package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func at(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }

func user(login string) *actor { return &actor{Login: login, Typename: "User"} }

func prNode(ci, review string, comments ...comment) node {
	var n node
	n.Number, n.Title, n.CreatedAt, n.ReviewDecision = 1, "fix a thing", t0, review
	n.Repository.NameWithOwner = "acme/widget"
	n.Commits.Nodes = make([]struct {
		Commit struct {
			CommittedDate     time.Time `json:"committedDate"`
			StatusCheckRollup *struct {
				State string `json:"state"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}, 1)
	n.Commits.Nodes[0].Commit.CommittedDate = t0
	if ci != "" {
		n.Commits.Nodes[0].Commit.StatusCheckRollup = &struct {
			State string `json:"state"`
		}{ci}
	}
	n.Comments.Nodes = comments
	return n
}

func TestClassifyPR(t *testing.T) {
	tests := []struct {
		name     string
		n        node
		yourMove bool
		status   string
	}{
		{"fresh", prNode("SUCCESS", ""), false, "no review yet"},
		{"ci failing", prNode("FAILURE", ""), true, "CI failing"},
		{"maintainer replied",
			prNode("SUCCESS", "", comment{Author: user("maint"), CreatedAt: at(2), BodyText: "can you add a test?"}),
			true, "reply waiting"},
		{"i answered",
			prNode("SUCCESS", "",
				comment{Author: user("maint"), CreatedAt: at(2)},
				comment{Author: user("me"), CreatedAt: at(3)}),
			false, "waiting on them"},
		{"my login is case-insensitive",
			prNode("SUCCESS", "",
				comment{Author: user("maint"), CreatedAt: at(2)},
				comment{Author: user("ME"), CreatedAt: at(3)}),
			false, "waiting on them"},
		{"bots don't count",
			prNode("SUCCESS", "",
				comment{Author: &actor{Login: "codecov", Typename: "Bot"}, CreatedAt: at(2)},
				comment{Author: user("dependabot[bot]"), CreatedAt: at(2)}),
			false, "no review yet"},
		{"changes requested",
			prNode("SUCCESS", "CHANGES_REQUESTED", comment{Author: user("maint"), CreatedAt: at(2)}),
			true, "changes requested"},
		{"approved", prNode("SUCCESS", "APPROVED"), false, "approved, awaiting merge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := classifyPR(tt.n, "me")
			if r.YourMove != tt.yourMove || r.Status != tt.status {
				t.Errorf("got yourMove=%v status=%q, want %v %q", r.YourMove, r.Status, tt.yourMove, tt.status)
			}
		})
	}
}

func TestReviewCountsAsActivity(t *testing.T) {
	n := prNode("SUCCESS", "")
	n.Reviews.Nodes = []comment{{Author: user("maint"), SubmittedAt: at(5), State: "COMMENTED"}}
	r := classifyPR(n, "me")
	if !r.YourMove || r.Last != "commented" || r.LastBy != "maint" {
		t.Errorf("got %+v", r)
	}
}

func TestClassifyThreadSeen(t *testing.T) {
	var n node
	n.Number, n.CreatedAt = 7, t0
	n.Comments.Nodes = []comment{{
		Author: user("me"), CreatedAt: at(1),
		Replies: &struct {
			Nodes []comment `json:"nodes"`
		}{[]comment{{Author: user("maint"), CreatedAt: at(4)}}},
	}}
	tr := trackedThread{Kind: "disc", Repo: "acme/widget", Number: 7}

	if r := classifyThread(n, tr, "me"); !r.YourMove {
		t.Errorf("nested reply should be your move, got %+v", r)
	}
	tr.Seen = at(4)
	if r := classifyThread(n, tr, "me"); r.YourMove {
		t.Errorf("reply at seen time should be handled, got %+v", r)
	}
}

func TestSortRows(t *testing.T) {
	rows := []row{
		{Number: 1, Since: at(1)},
		{Number: 2, Since: at(5), YourMove: true},
		{Number: 3, Since: at(0)},
		{Number: 4, Since: at(2), YourMove: true},
	}
	sortRows(rows)
	var got []int
	for _, r := range rows {
		got = append(got, r.Number)
	}
	want := []int{4, 2, 3, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

func TestNewMoves(t *testing.T) {
	seen := map[string]string{}
	a := row{Repo: "acme/w", Number: 1, YourMove: true, Status: "reply waiting", Since: at(1)}
	b := row{Repo: "acme/w", Number: 2, Status: "waiting on them"}

	if got := newMoves([]row{a, b}, seen); len(got) != 1 {
		t.Fatalf("first pass: got %d, want 1", len(got))
	}
	if got := newMoves([]row{a, b}, seen); len(got) != 0 {
		t.Fatalf("unchanged: got %d, want 0", len(got))
	}
	a.Since = at(3) // another reply
	if got := newMoves([]row{a, b}, seen); len(got) != 1 {
		t.Fatalf("new reply: got %d, want 1", len(got))
	}
}

func TestLoadTracked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracked.txt")
	os.WriteFile(path, []byte(`# comment
disc acme/widget 12
issue acme/widget 34 2026-10-24T18:38:51Z  # handled already

`), 0o644)
	got, err := loadTracked(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Kind != "disc" || got[1].Number != 34 || got[1].Seen.IsZero() {
		t.Errorf("got %+v", got)
	}

	os.WriteFile(path, []byte("pr acme/widget 1\n"), 0o644)
	if _, err := loadTracked(path); err == nil {
		t.Error("want error for unknown kind")
	}
	if got, err := loadTracked(filepath.Join(t.TempDir(), "missing")); err != nil || got != nil {
		t.Errorf("missing file: got %v, %v", got, err)
	}
}

func TestGraphqlHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()
	defer func(u string) { graphqlURL = u }(graphqlURL)
	graphqlURL = srv.URL
	t.Setenv("GITHUB_TOKEN", "nope")

	_, err := graphql("query { viewer { login } }", nil)
	if err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("got %v, want the message from github", err)
	}
}
