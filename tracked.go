package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// trackedThread is an issue or discussion to watch for replies.
// Open PRs are found automatically and don't need tracking.
type trackedThread struct {
	Kind   string // issue or disc
	Repo   string // owner/name
	Number int
	Seen   time.Time // replies at or before this are already handled
}

func (t trackedThread) Owner() string { o, _, _ := strings.Cut(t.Repo, "/"); return o }
func (t trackedThread) Name() string  { _, n, _ := strings.Cut(t.Repo, "/"); return n }

func defaultTrackedPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "prwatch", "tracked.txt")
}

// loadTracked reads lines of `kind owner/repo number [seen]`.
// A missing file just means nothing is tracked.
func loadTracked(path string) ([]trackedThread, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []trackedThread
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		bad := func(why string) error { return fmt.Errorf("%s:%d: %s", path, line, why) }
		if len(fields) < 3 || len(fields) > 4 {
			return nil, bad("want `kind owner/repo number [seen]`")
		}
		t := trackedThread{Kind: fields[0], Repo: fields[1]}
		if t.Kind != "issue" && t.Kind != "disc" {
			return nil, bad("kind must be issue or disc")
		}
		if !strings.Contains(t.Repo, "/") {
			return nil, bad("repo must be owner/name")
		}
		if t.Number, err = strconv.Atoi(fields[2]); err != nil {
			return nil, bad("bad number")
		}
		if len(fields) == 4 {
			if t.Seen, err = time.Parse(time.RFC3339, fields[3]); err != nil {
				return nil, bad("seen must be an RFC 3339 time")
			}
		}
		out = append(out, t)
	}
	return out, sc.Err()
}
