// Package gitimport reads the user's commits from GitLab, GitHub and local
// clones and groups them into one draft entry per day per repo, the unit
// Task Composer imports as a task.
package gitimport

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
)

// Commit is one commit authored by the user.
type Commit struct {
	SHA       string // full SHA; the dedupe key across sources
	Repo      string // short repo name, e.g. "api"
	When      time.Time
	Subject   string
	Additions int
	Deletions int
	Files     int
	HasStats  bool // false when the source gave no diff stats
}

// Entry is one day of work in one repo: one draft task.
type Entry struct {
	Date    string // YYYY-MM-DD, local time
	Repo    string
	Commits []Commit // oldest first
	Hours   float64  // estimate, see Estimate
}

// SHAs lists the entry's commit SHAs.
func (e Entry) SHAs() []string {
	out := make([]string, len(e.Commits))
	for i, c := range e.Commits {
		out[i] = c.SHA
	}
	return out
}

// Result is what Collect returns: the commits found and one warning per
// source that failed, so one broken token does not hide the other sources.
type Result struct {
	Commits  []Commit
	Warnings []string
}

// Range is an inclusive range of local dates.
type Range struct {
	Start, End time.Time // midnight local time of the first and last day
}

// NewRange parses two YYYY-MM-DD dates in local time.
func NewRange(start, end string) (Range, error) {
	s, err := time.ParseInLocation("2006-01-02", start, time.Local)
	if err != nil {
		return Range{}, fmt.Errorf("start date %q must be YYYY-MM-DD", start)
	}
	e, err := time.ParseInLocation("2006-01-02", end, time.Local)
	if err != nil {
		return Range{}, fmt.Errorf("end date %q must be YYYY-MM-DD", end)
	}
	if e.Before(s) {
		return Range{}, fmt.Errorf("end date %s is before start date %s", end, start)
	}
	if e.Sub(s) > 92*24*time.Hour {
		return Range{}, fmt.Errorf("range %s..%s is longer than 3 months", start, end)
	}
	return Range{Start: s, End: e}, nil
}

// Contains reports whether t falls on one of the range's days (local time).
func (r Range) Contains(t time.Time) bool {
	d := t.In(time.Local)
	return !d.Before(r.Start) && d.Before(r.End.AddDate(0, 0, 1))
}

// httpClient is shared by the API sources; tests swap it.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// fetchers per source kind; tests swap them.
var fetchers = map[string]func(context.Context, config.GitSource, Range) ([]Commit, error){
	config.Local:  fetchLocal,
	config.GitLab: fetchGitLab,
	config.GitHub: fetchGitHub,
}

// Collect reads every source in parallel, keeps commits inside r and drops
// duplicates (the same SHA seen locally and on GitLab counts once).
func Collect(ctx context.Context, sources []config.GitSource, r Range) Result {
	type out struct {
		commits []Commit
		err     error
	}
	results := make([]out, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		fetch, ok := fetchers[src.Kind]
		if !ok {
			results[i].err = fmt.Errorf("unknown source kind %q", src.Kind)
			continue
		}
		wg.Add(1)
		go func(i int, src config.GitSource) {
			defer wg.Done()
			c, err := fetch(ctx, src, r)
			results[i] = out{c, err}
		}(i, src)
	}
	wg.Wait()

	var res Result
	seen := map[string]int{} // SHA -> index in res.Commits
	for i, o := range results {
		if o.err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: %v", sources[i].Label(), o.err))
		}
		for _, c := range o.commits {
			if !r.Contains(c.When) {
				continue
			}
			if j, dup := seen[c.SHA]; dup {
				// Keep the copy that has diff stats.
				if !res.Commits[j].HasStats && c.HasStats {
					res.Commits[j] = c
				}
				continue
			}
			seen[c.SHA] = len(res.Commits)
			res.Commits = append(res.Commits, c)
		}
	}
	return res
}

// Group makes one entry per (day, repo), sorted by date then repo, with
// hours estimated by Estimate.
func Group(commits []Commit) []Entry {
	idx := map[string]int{}
	var entries []Entry
	for _, c := range commits {
		date := c.When.In(time.Local).Format("2006-01-02")
		key := date + "\x00" + c.Repo
		i, ok := idx[key]
		if !ok {
			i = len(entries)
			idx[key] = i
			entries = append(entries, Entry{Date: date, Repo: c.Repo})
		}
		entries[i].Commits = append(entries[i].Commits, c)
	}
	for i := range entries {
		sort.SliceStable(entries[i].Commits, func(a, b int) bool {
			return entries[i].Commits[a].When.Before(entries[i].Commits[b].When)
		})
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].Date != entries[b].Date {
			return entries[a].Date < entries[b].Date
		}
		return entries[a].Repo < entries[b].Repo
	})
	Estimate(entries)
	return entries
}

// MaxDayHours caps the estimated hours of one day across all repos.
const MaxDayHours = 8.0

// Estimate sets Hours on each entry. An entry's raw size is 1h plus half an
// hour per extra commit plus an hour per 300 changed lines; when a day's raw
// sizes add up to more than MaxDayHours they are scaled down to fit. Hours
// round to the nearest half hour, at least 0.5. These are guesses for the
// user to adjust, not tracked time.
func Estimate(entries []Entry) {
	byDay := map[string][]int{}
	for i, e := range entries {
		byDay[e.Date] = append(byDay[e.Date], i)
	}
	for _, idxs := range byDay {
		raw := make([]float64, len(idxs))
		total := 0.0
		for k, i := range idxs {
			lines := 0
			for _, c := range entries[i].Commits {
				lines += c.Additions + c.Deletions
			}
			raw[k] = 1 + 0.5*float64(len(entries[i].Commits)-1) + float64(lines)/300
			total += raw[k]
		}
		scale := 1.0
		if total > MaxDayHours {
			scale = MaxDayHours / total
		}
		for k, i := range idxs {
			h := math.Round(raw[k]*scale*2) / 2
			if h < 0.5 {
				h = 0.5
			}
			entries[i].Hours = h
		}
	}
}

// Title is the draft task title for an entry.
func (e Entry) Title() string {
	main := e.Commits[0]
	for _, c := range e.Commits[1:] {
		if c.Additions+c.Deletions > main.Additions+main.Deletions {
			main = c
		}
	}
	t := "Working on " + e.Repo + ": " + main.Subject
	if r := []rune(t); len(r) > 150 {
		t = string(r[:147]) + "..."
	}
	return t
}

// Description is the draft task description: the repo and its commits.
func (e Entry) Description() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\nCommits:\n", e.Repo)
	for _, c := range e.Commits {
		sha := c.SHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		fmt.Fprintf(&b, "- %s (%s)\n", c.Subject, sha)
	}
	return strings.TrimRight(b.String(), "\n")
}

// firstLine is a commit message's subject.
func firstLine(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return strings.TrimSpace(msg)
}
