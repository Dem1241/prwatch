package main

import "time"

// demoSnapshot is made-up data for screenshots and trying prwatch
// without a github login.
func demoSnapshot() *snapshot {
	ago := func(h float64) time.Time { return time.Now().Add(-time.Duration(h * float64(time.Hour))) }
	pr := func(repo string, n int, ci, review, status string, mine bool, h float64, title string) row {
		return row{Kind: "pr", Repo: repo, Number: n, CI: ci, Review: review, Status: status, YourMove: mine, Since: ago(h), Title: title}
	}
	rows := []row{
		pr("lattice-db/lattice", 2214, "failure", "none", "CI failing", true, 3, "fix: keep NULLs out of the bloom filter on merge"),
		pr("tidewave/tidewave", 981, "success", "changes_requested", "changes requested", true, 5, "sink/s3: retry multipart uploads on 503"),
		pr("quill-sh/quill", 412, "success", "none", "reply waiting", true, 26, "fix: respect NO_COLOR when stdout is a pipe"),
		{Kind: "disc", Repo: "lattice-db/lattice", Number: 1877, Status: "reply waiting", YourMove: true, Since: ago(1),
			Title: "Compaction stalls with many small parquet files"},
		pr("ferrous-io/migrate", 77, "success", "none", "no review yet", false, 24*19, "fix: ignore a byte order mark at the start of a migration"),
		pr("tidewave/tidewave", 990, "success", "none", "waiting on them", false, 24*4, "kafka: don't forward internal headers as record headers"),
		pr("pgkit/pgkit", 1503, "success", "approved", "approved, awaiting merge", false, 30, "pool: close idle conns on context cancel"),
		pr("lattice-db/lattice", 2219, "pending", "none", "no review yet", false, 2, "planner: push LIMIT below UNION ALL"),
		{Kind: "issue", Repo: "ferrous-io/migrate", Number: 64, Status: "waiting on them", Since: ago(52),
			Title: "dirty flag stays set after a failed down migration"},
	}
	rows[1].LastBy, rows[1].Last = "maintainer", "looks good overall, but can we cap the retries at 5 and make it configurable?"
	rows[2].LastBy, rows[2].Last = "maintainer", "thanks! could you add a test for the pipe case too?"
	rows[3].LastBy, rows[3].Last = "maintainer", "tried your repro on main and it's fixed there, can you confirm on your side?"
	sortRows(rows)
	return &snapshot{Me: "you", Merged: 12, Closed: 3, Rows: rows}
}
