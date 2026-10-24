// prwatch shows your open GitHub PRs and tracked threads, and whose move it is on each.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	user := flag.String("user", "", "GitHub login (default: the authenticated user)")
	asJSON := flag.Bool("json", false, "print rows as JSON")
	staleDays := flag.Int("stale", 14, "flag rows waiting on others for at least this many days")
	trackedPath := flag.String("tracked", defaultTrackedPath(), "file of issues/discussions to watch")
	watch := flag.Duration("watch", 0, "refresh on this interval, e.g. 5m (0 runs once)")
	noNotify := flag.Bool("no-notify", false, "in watch mode, don't send desktop notifications")
	flag.Parse()

	who := *user
	if who == "" {
		who = "@me"
	}
	tracked, err := loadTracked(*trackedPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prwatch:", err)
		os.Exit(1)
	}

	fi, _ := os.Stdout.Stat()
	tty := fi.Mode()&os.ModeCharDevice != 0
	c := colors(tty && os.Getenv("NO_COLOR") == "")
	stale := time.Duration(*staleDays) * 24 * time.Hour

	if *watch > 0 {
		if *watch < time.Minute {
			fmt.Fprintln(os.Stderr, "prwatch: -watch must be at least 1m")
			os.Exit(2)
		}
		watchLoop(who, tracked, *watch, c, stale, !*noNotify)
		return
	}

	s, err := fetch(who, tracked)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prwatch:", err)
		os.Exit(1)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(s.Rows)
		return
	}
	render(os.Stdout, s, c, stale)
}

// watchLoop redraws on every tick and notifies when a row becomes your
// move or something new lands on a row that already was.
func watchLoop(who string, tracked []trackedThread, every time.Duration, c palette, stale time.Duration, notify bool) {
	notifier, _ := exec.LookPath("notify-send")
	seen := map[string]string{}
	first := true
	for {
		var buf bytes.Buffer
		s, err := fetch(who, tracked)
		if err != nil {
			fmt.Fprintf(&buf, "%sprwatch: %v%s\n", c.red, err, c.reset)
		} else {
			render(&buf, s, c, stale)
			for _, r := range newMoves(s.Rows, seen) {
				if !first && notify && notifier != "" {
					exec.Command(notifier, "-a", "prwatch", "prwatch: your move",
						fmt.Sprintf("%s#%d · %s\n%s", r.Repo, r.Number, r.Status, trunc(r.Title, 80))).Run()
				}
			}
			first = false
		}
		fmt.Printf("\033[H\033[2J%s\n%supdated %s · next in %s · ctrl-c to quit%s\n",
			buf.String(), c.dim, time.Now().Format("15:04"), every, c.reset)
		time.Sleep(every)
	}
}

// newMoves returns your-move rows whose state changed since the last
// call, and updates seen to the current state.
func newMoves(rows []row, seen map[string]string) []row {
	var out []row
	now := map[string]string{}
	for _, r := range rows {
		if !r.YourMove {
			continue
		}
		id := fmt.Sprintf("%s#%d", r.Repo, r.Number)
		now[id] = r.key()
		if seen[id] != r.key() {
			out = append(out, r)
		}
	}
	clear(seen)
	for k, v := range now {
		seen[k] = v
	}
	return out
}
