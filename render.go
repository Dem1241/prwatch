package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type palette struct{ red, green, yellow, dim, bold, reset string }

func colors(on bool) palette {
	if !on {
		return palette{}
	}
	return palette{"\033[31m", "\033[32m", "\033[33m", "\033[2m", "\033[1m", "\033[0m"}
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func render(w io.Writer, s *snapshot, c palette, stale time.Duration) {
	yours, prs := 0, 0
	repoW := 4
	for _, x := range s.Rows {
		if x.YourMove {
			yours++
		}
		if x.Kind == "pr" {
			prs++
		}
		repoW = max(repoW, len(x.Repo)+len(fmt.Sprint(x.Number))+1)
	}
	fmt.Fprintf(w, "%s%s%s · %d open PRs · %d threads · %s%d your move%s · %d merged · %d closed\n\n",
		c.bold, s.Me, c.reset, prs, len(s.Rows)-prs, c.yellow, yours, c.reset, s.Merged, s.Closed)

	for i, x := range s.Rows {
		if i == 0 || x.YourMove != s.Rows[i-1].YourMove {
			label := "YOUR MOVE"
			if !x.YourMove {
				label = "WAITING ON THEM"
			}
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s%s%s\n", c.bold, label, c.reset)
		}

		status, col := x.Status, c.dim
		switch {
		case x.YourMove:
			col = c.yellow
			if x.Status == "CI failing" {
				col = c.red
			}
		case x.Review == "approved" || x.Status == "answered":
			col = c.green
		case x.Status == "closed":
		case time.Since(x.Since) >= stale:
			col, status = c.red, status+" (stale)"
		}

		mark := c.dim + "·"
		switch x.Kind {
		case "issue":
			mark = c.dim + "i"
		case "disc":
			mark = c.dim + "d"
		default:
			switch x.CI {
			case "success":
				mark = c.green + "✓"
			case "failure", "error":
				mark = c.red + "✗"
			case "pending", "expected":
				mark = c.yellow + "…"
			}
		}
		fmt.Fprintf(w, "  %s%s %-*s %s%5s%s  %s%-26s%s %s\n",
			mark, c.reset, repoW, fmt.Sprintf("%s#%d", x.Repo, x.Number),
			c.dim, ago(x.Since), c.reset, col, status, c.reset, trunc(x.Title, 50))
		if x.Last != "" {
			fmt.Fprintf(w, "    %s↳ %s: %s%s\n", c.dim, x.LastBy, trunc(x.Last, 100), c.reset)
		}
	}
}
