package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type actor struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
}

// comment covers issue comments, discussion comments and PR reviews.
type comment struct {
	Author      *actor    `json:"author"`
	CreatedAt   time.Time `json:"createdAt"`
	SubmittedAt time.Time `json:"submittedAt"`
	BodyText    string    `json:"bodyText"`
	State       string    `json:"state"`
	Replies     *struct {
		Nodes []comment `json:"nodes"`
	} `json:"replies"`
}

func (c comment) event() event {
	at, body := c.CreatedAt, c.BodyText
	if !c.SubmittedAt.IsZero() {
		at = c.SubmittedAt
	}
	if body == "" && c.State != "" {
		body = strings.ToLower(strings.ReplaceAll(c.State, "_", " "))
	}
	return event{Who: c.Author, At: at, Body: body}
}

type node struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"createdAt"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	State          string    `json:"state"`
	IsAnswered     bool      `json:"isAnswered"`
	Repository     struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				CommittedDate     time.Time `json:"committedDate"`
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	Comments struct {
		Nodes []comment `json:"nodes"`
	} `json:"comments"`
	Reviews struct {
		Nodes []comment `json:"nodes"`
	} `json:"reviews"`
}

func (n node) events() []event {
	var out []event
	for _, c := range n.Comments.Nodes {
		out = append(out, c.event())
		if c.Replies != nil {
			for _, r := range c.Replies.Nodes {
				out = append(out, r.event())
			}
		}
	}
	for _, r := range n.Reviews.Nodes {
		out = append(out, r.event())
	}
	return out
}

const prFields = `
  number title url createdAt isDraft reviewDecision
  repository { nameWithOwner }
  commits(last:1) { nodes { commit { committedDate statusCheckRollup { state } } } }
  comments(last:30) { nodes { author { login __typename } createdAt bodyText } }
  reviews(last:30) { nodes { author { login __typename } submittedAt bodyText state } }`

const issueFields = `
  number title url createdAt state
  repository { nameWithOwner }
  comments(last:30) { nodes { author { login __typename } createdAt bodyText } }`

const discFields = `
  number title url createdAt isAnswered
  repository { nameWithOwner }
  comments(last:20) { nodes { author { login __typename } createdAt bodyText
    replies(last:20) { nodes { author { login __typename } createdAt bodyText } } } }`

type snapshot struct {
	Me     string
	Merged int
	Closed int
	Rows   []row
}

func token() (string, error) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t, nil
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("no GITHUB_TOKEN and `gh auth token` failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func graphql(q string, vars map[string]any) (map[string]json.RawMessage, error) {
	tok, err := token()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{"query": q, "variables": vars})
	req, _ := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader(body))
	req.Header.Set("Authorization", "bearer "+tok)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Data   map[string]json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("github returned %s: %w", resp.Status, err)
	}
	if len(r.Errors) > 0 {
		return nil, fmt.Errorf("graphql: %s", r.Errors[0].Message)
	}
	return r.Data, nil
}

// fetch gets open PRs, merge counts and every tracked thread in one request.
func fetch(user string, tracked []trackedThread) (*snapshot, error) {
	var q strings.Builder
	q.WriteString(`query($open:String!, $merged:String!, $closed:String!) {
  viewer { login }
  merged: search(query:$merged, type:ISSUE, first:0) { issueCount }
  closed: search(query:$closed, type:ISSUE, first:0) { issueCount }
  open: search(query:$open, type:ISSUE, first:100) { nodes { ... on PullRequest {` + prFields + `} } }
`)
	for i, t := range tracked {
		field, fields := "issue", issueFields
		if t.Kind == "disc" {
			field, fields = "discussion", discFields
		}
		fmt.Fprintf(&q, "  t%d: repository(owner:%q, name:%q) { %s(number:%d) {%s} }\n",
			i, t.Owner(), t.Name(), field, t.Number, fields)
	}
	q.WriteString("}")

	base := "is:pr author:" + user
	data, err := graphql(q.String(), map[string]any{
		"open":   base + " is:open",
		"merged": base + " is:merged",
		"closed": base + " is:closed is:unmerged",
	})
	if err != nil {
		return nil, err
	}

	var viewer struct{ Login string }
	var merged, closed struct{ IssueCount int }
	var open struct{ Nodes []node }
	for k, v := range map[string]any{"viewer": &viewer, "merged": &merged, "closed": &closed, "open": &open} {
		if err := json.Unmarshal(data[k], v); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", k, err)
		}
	}
	s := &snapshot{Me: viewer.Login, Merged: merged.IssueCount, Closed: closed.IssueCount}
	if user != "@me" {
		s.Me = user
	}

	for _, n := range open.Nodes {
		if n.Number == 0 {
			continue
		}
		s.Rows = append(s.Rows, classifyPR(n, s.Me))
	}
	for i, t := range tracked {
		var repo map[string]*node
		if err := json.Unmarshal(data[fmt.Sprintf("t%d", i)], &repo); err != nil || repo == nil {
			return nil, fmt.Errorf("tracked %s#%d: not found", t.Repo, t.Number)
		}
		for _, n := range repo {
			if n == nil {
				return nil, fmt.Errorf("tracked %s %s#%d: not found", t.Kind, t.Repo, t.Number)
			}
			s.Rows = append(s.Rows, classifyThread(*n, t, s.Me))
		}
	}
	sortRows(s.Rows)
	return s, nil
}
