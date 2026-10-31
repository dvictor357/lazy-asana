package gitimport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func mustRange(t *testing.T, a, b string) Range {
	t.Helper()
	r, err := NewRange(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewRange(t *testing.T) {
	cases := []struct {
		start, end string
		ok         bool
	}{
		{"2026-10-26", "2026-10-31", true},
		{"2026-10-31", "2026-10-31", true},
		{"2026-10-31", "2026-10-30", false},
		{"2026-10-x", "2026-10-31", false},
		{"2026-01-01", "2026-06-01", false}, // longer than 3 months
	}
	for _, c := range cases {
		_, err := NewRange(c.start, c.end)
		if (err == nil) != c.ok {
			t.Errorf("NewRange(%s, %s) err = %v, want ok=%v", c.start, c.end, err, c.ok)
		}
	}
	r := mustRange(t, "2026-10-26", "2026-10-27")
	for s, want := range map[string]bool{
		"2026-10-25 23:59": false, "2026-10-26 00:00": true,
		"2026-10-27 23:59": true, "2026-10-28 00:00": false,
	} {
		if got := r.Contains(at(s)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestParseShortstat(t *testing.T) {
	cases := map[string][3]int{
		" 3 files changed, 10 insertions(+), 2 deletions(-)": {3, 10, 2},
		" 1 file changed, 1 insertion(+)":                    {1, 1, 0},
		" 2 files changed, 5 deletions(-)":                   {2, 0, 5},
	}
	for line, want := range cases {
		f, i, d := parseShortstat(line)
		if [3]int{f, i, d} != want {
			t.Errorf("parseShortstat(%q) = %d %d %d, want %v", line, f, i, d, want)
		}
	}
}

func TestGroupSplitsPerDayPerRepoAndCapsTheDay(t *testing.T) {
	commits := []Commit{
		{SHA: "a1", Repo: "web", When: at("2026-10-26 15:00"), Subject: "big change", Additions: 3000},
		{SHA: "a2", Repo: "api", When: at("2026-10-26 10:00"), Subject: "fix retry", Additions: 30},
		{SHA: "a3", Repo: "web", When: at("2026-10-26 09:00"), Subject: "small", Additions: 5},
		{SHA: "a4", Repo: "web", When: at("2026-10-27 09:00"), Subject: "next day", Additions: 10},
	}
	entries := Group(commits)
	if len(entries) != 3 {
		t.Fatalf("want 3 entries (api 26, web 26, web 27), got %d: %+v", len(entries), entries)
	}
	if entries[0].Repo != "api" || entries[1].Repo != "web" || entries[2].Date != "2026-10-27" {
		t.Fatalf("wrong order: %+v", entries)
	}
	web := entries[1]
	if web.Commits[0].SHA != "a3" {
		t.Errorf("commits not oldest first: %v", web.SHAs())
	}
	if day := entries[0].Hours + entries[1].Hours; day > MaxDayHours+0.5 {
		t.Errorf("day total %.1fh exceeds the %vh cap", day, MaxDayHours)
	}
	if entries[1].Hours <= entries[0].Hours {
		t.Errorf("bigger entry should get more hours: api %.1f web %.1f", entries[0].Hours, entries[1].Hours)
	}
	if entries[2].Hours != 1 {
		t.Errorf("one small commit = 1h, got %.1f", entries[2].Hours)
	}
	for _, e := range entries {
		if e.Hours < 0.5 || e.Hours*2 != float64(int(e.Hours*2)) {
			t.Errorf("hours %.2f not a half-hour step >= 0.5", e.Hours)
		}
	}
	if got := web.Title(); got != "Working on web: big change" {
		t.Errorf("Title = %q", got)
	}
	if d := web.Description(); !strings.Contains(d, "Project: web") || !strings.Contains(d, "- small (a3)") {
		t.Errorf("Description = %q", d)
	}
}

func TestCollectDedupesBySHAAndKeepsStats(t *testing.T) {
	r := mustRange(t, "2026-10-26", "2026-10-31")
	orig := fetchers
	defer func() { fetchers = orig }()
	fetchers = map[string]func(context.Context, config.GitSource, Range) ([]Commit, error){
		config.GitHub: func(context.Context, config.GitSource, Range) ([]Commit, error) {
			return []Commit{{SHA: "s1", Repo: "api", When: at("2026-10-27 10:00")}}, nil
		},
		config.Local: func(context.Context, config.GitSource, Range) ([]Commit, error) {
			return []Commit{
				{SHA: "s1", Repo: "api", When: at("2026-10-27 10:00"), Additions: 50, HasStats: true},
				{SHA: "s2", Repo: "api", When: at("2026-10-20 10:00")}, // outside range
			}, nil
		},
		config.GitLab: func(context.Context, config.GitSource, Range) ([]Commit, error) {
			return nil, errors.New("HTTP 401")
		},
	}
	res := Collect(context.Background(), []config.GitSource{
		{Kind: config.GitHub, Token: "x"},
		{Kind: config.Local, Path: "/tmp"},
		{Kind: config.GitLab, Name: "Kantor", Token: "y"},
	}, r)
	if len(res.Commits) != 1 || !res.Commits[0].HasStats || res.Commits[0].Additions != 50 {
		t.Fatalf("want one deduped commit with stats, got %+v", res.Commits)
	}
	if len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], "Kantor:") {
		t.Fatalf("want one Kantor warning, got %v", res.Warnings)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitAt(t *testing.T, dir, file, author, when string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+when), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	cmd := exec.Command("git", "-C", dir, "commit", "-q", "-m", "add "+file, "--author", author, "--date", when)
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+when)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
}

func TestFetchLocalFindsReposAndFiltersAuthor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"),
		[]byte("[user]\n\tname = Me\n\temail = me@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	api := filepath.Join(root, "clients", "api")
	ignored := filepath.Join(root, "node_modules", "dep")
	for _, d := range []string{api, ignored} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, d, "init", "-q")
	}
	commitAt(t, api, "c.go", "Me <me@example.com>", "2026-10-10T10:00:00")
	commitAt(t, api, "a.go", "Me <me@example.com>", "2026-10-27T10:00:00")
	commitAt(t, api, "b.go", "Other <other@example.com>", "2026-10-27T11:00:00")
	git(t, api, "checkout", "-q", "-b", "feature")
	commitAt(t, api, "d.go", "Me <me@example.com>", "2026-10-28T10:00:00")
	git(t, api, "checkout", "-q", "main")
	commitAt(t, ignored, "x.go", "Me <me@example.com>", "2026-10-27T10:00:00")

	res := Collect(context.Background(), []config.GitSource{{Kind: config.Local, Path: root}},
		mustRange(t, "2026-10-26", "2026-10-31"))
	if len(res.Warnings) > 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	var subjects []string
	for _, c := range res.Commits {
		subjects = append(subjects, c.Repo+":"+c.Subject)
		if !c.HasStats || c.Files != 1 || c.Additions != 1 {
			t.Errorf("missing stats on %+v", c)
		}
	}
	got := strings.Join(subjects, ",")
	if !strings.Contains(got, "api:add a.go") || !strings.Contains(got, "api:add d.go") || len(subjects) != 2 {
		t.Fatalf("want my two in-range commits on all branches, got %s", got)
	}
}

func TestFetchLocalReportsMissingRepos(t *testing.T) {
	res := Collect(context.Background(), []config.GitSource{{Kind: config.Local, Name: "Empty", Path: t.TempDir()}},
		mustRange(t, "2026-10-26", "2026-10-31"))
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "no git repos") {
		t.Fatalf("want a no-repos warning, got %v", res.Warnings)
	}
}

func serve(t *testing.T, routes map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if s, isStatus := body.(int); isStatus {
			w.WriteHeader(s)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchGitLab(t *testing.T) {
	srv := serve(t, map[string]any{
		"/api/v4/user":           map[string]any{"id": 7, "name": "Me", "email": "me@corp.com"},
		"/api/v4/user/emails":    []map[string]any{{"email": "me@old.com"}},
		"/api/v4/users/7/events": []map[string]any{{"project_id": 3}, {"project_id": 3}},
		"/api/v4/projects/3":     map[string]any{"path": "billing"},
		"/api/v4/projects/3/repository/commits": []map[string]any{
			{"id": "c1", "title": "mine", "author_email": "ME@corp.com", "authored_date": "2026-10-27T10:00:00+07:00",
				"parent_ids": []string{"p"}, "stats": map[string]int{"additions": 12, "deletions": 3}},
			{"id": "c2", "title": "old email", "author_email": "me@old.com", "authored_date": "2026-10-27T11:00:00+07:00", "parent_ids": []string{"p"}},
			{"id": "c3", "title": "theirs", "author_email": "x@corp.com", "author_name": "X", "authored_date": "2026-10-27T11:00:00+07:00", "parent_ids": []string{"p"}},
			{"id": "c4", "title": "merge", "author_email": "me@corp.com", "authored_date": "2026-10-27T12:00:00+07:00", "parent_ids": []string{"p", "q"}},
		},
	})
	got, err := fetchGitLab(context.Background(), config.GitSource{Kind: config.GitLab, BaseURL: srv.URL, Token: "t"},
		mustRange(t, "2026-10-26", "2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SHA != "c1" || got[1].SHA != "c2" || got[0].Repo != "billing" || got[0].Additions != 12 {
		t.Fatalf("got %+v", got)
	}
}

func TestFetchGitLabBadTokenNamesTheProblem(t *testing.T) {
	srv := serve(t, map[string]any{"/api/v4/user": http.StatusUnauthorized})
	_, err := fetchGitLab(context.Background(), config.GitSource{Kind: config.GitLab, BaseURL: srv.URL, Token: "secret-token"},
		mustRange(t, "2026-10-26", "2026-10-31"))
	if err == nil || !strings.Contains(err.Error(), "token invalid") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchGitHub(t *testing.T) {
	srv := serve(t, map[string]any{
		"/user": map[string]any{"login": "me"},
		"/search/commits": map[string]any{"items": []map[string]any{
			{"sha": "g1", "commit": map[string]any{"message": "feat: x\n\nbody", "author": map[string]any{"date": "2026-10-28T09:00:00Z"}},
				"parents": []any{map[string]any{}}, "repository": map[string]any{"name": "site", "full_name": "me/site"}},
			{"sha": "g2", "commit": map[string]any{"message": "Merge", "author": map[string]any{"date": "2026-10-28T09:00:00Z"}},
				"parents": []any{map[string]any{}, map[string]any{}}, "repository": map[string]any{"name": "site", "full_name": "me/site"}},
		}},
		"/repos/me/site/commits/g1": map[string]any{"stats": map[string]int{"additions": 40, "deletions": 2}, "files": []any{map[string]any{}, map[string]any{}}},
	})
	got, err := fetchGitHub(context.Background(), config.GitSource{Kind: config.GitHub, BaseURL: srv.URL, Token: "t"},
		mustRange(t, "2026-10-26", "2026-10-31"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Subject != "feat: x" || got[0].Repo != "site" || got[0].Additions != 40 || got[0].Files != 2 {
		t.Fatalf("got %+v", got)
	}
}
