package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

type event struct {
	Who  *actor
	At   time.Time
	Body string
}

// row is the computed view of one PR, issue or discussion.
type row struct {
	Kind     string    `json:"kind"` // pr, issue or disc
	Repo     string    `json:"repo"`
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	CI       string    `json:"ci,omitempty"`
	Review   string    `json:"review,omitempty"`
	YourMove bool      `json:"yourMove"`
	Status   string    `json:"status"`
	Since    time.Time `json:"since"`
	LastBy   string    `json:"lastBy,omitempty"`
	Last     string    `json:"last,omitempty"`
}

// key changes whenever something new happens on a row, so watch mode
// can tell a fresh reply from one it already notified about.
func (r row) key() string {
	return r.Status + "@" + r.Since.Format(time.RFC3339)
}

var botName = regexp.MustCompile(`(?i)bot\b|\[bot\]|^(github-actions|google-cla|CLAassistant|codecov|netlify|vercel|gemini-code-assist|coderabbitai|copilot)`)

func isBot(a *actor) bool {
	return a == nil || a.Typename == "Bot" || botName.MatchString(a.Login)
}

// latest splits events into my newest activity and the newest human
// activity from anyone else, starting mine at floor.
func latest(events []event, me string, floor time.Time) (mine time.Time, theirs *event) {
	mine = floor
	for i, e := range events {
		switch {
		case e.Who != nil && strings.EqualFold(e.Who.Login, me):
			if e.At.After(mine) {
				mine = e.At
			}
		case !isBot(e.Who):
			if theirs == nil || e.At.After(theirs.At) {
				theirs = &events[i]
			}
		}
	}
	return mine, theirs
}

func classifyPR(n node, me string) row {
	r := row{
		Kind: "pr", Repo: n.Repository.NameWithOwner, Number: n.Number, Title: n.Title,
		URL: n.URL, CI: "none", Review: strings.ToLower(n.ReviewDecision),
	}
	if r.Review == "" {
		r.Review = "none"
	}
	floor := n.CreatedAt
	if len(n.Commits.Nodes) > 0 {
		c := n.Commits.Nodes[0].Commit
		if c.CommittedDate.After(floor) {
			floor = c.CommittedDate
		}
		if c.StatusCheckRollup != nil {
			r.CI = strings.ToLower(c.StatusCheckRollup.State)
		}
	}
	mine, theirs := latest(n.events(), me, floor)
	replied := theirs != nil && theirs.At.After(mine)
	if replied {
		r.LastBy, r.Last = theirs.Who.Login, theirs.Body
	}

	switch {
	case r.CI == "failure" || r.CI == "error":
		r.YourMove, r.Status, r.Since = true, "CI failing", mine
	case replied && r.Review == "changes_requested":
		r.YourMove, r.Status, r.Since = true, "changes requested", theirs.At
	case replied:
		r.YourMove, r.Status, r.Since = true, "reply waiting", theirs.At
	case n.IsDraft:
		r.YourMove, r.Status, r.Since = true, "still a draft", mine
	case r.Review == "approved":
		r.Status, r.Since = "approved, awaiting merge", mine
	case theirs == nil:
		r.Status, r.Since = "no review yet", mine
	default:
		r.Status, r.Since = "waiting on them", mine
	}
	return r
}

func classifyThread(n node, t trackedThread, me string) row {
	r := row{Kind: t.Kind, Repo: t.Repo, Number: n.Number, Title: n.Title, URL: n.URL}
	mine, theirs := latest(n.events(), me, t.Seen)
	switch {
	case theirs != nil && theirs.At.After(mine):
		r.YourMove, r.Status, r.Since = true, "reply waiting", theirs.At
		r.LastBy, r.Last = theirs.Who.Login, theirs.Body
	case n.State == "CLOSED":
		r.Status, r.Since = "closed", mine
	case n.IsAnswered:
		r.Status, r.Since = "answered", mine
	default:
		r.Status, r.Since = "waiting on them", mine
	}
	if r.Since.IsZero() {
		r.Since = n.CreatedAt
	}
	return r
}

// sortRows puts your-move rows first, then whatever has waited longest.
func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].YourMove != rows[j].YourMove {
			return rows[i].YourMove
		}
		return rows[i].Since.Before(rows[j].Since)
	})
}
