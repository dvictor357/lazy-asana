package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/lazy-asana/internal/config"
	"github.com/dvictor357/lazy-asana/internal/gitimport"
)

func localAt(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// fakeCollect replaces the git reader for one test and counts calls.
func fakeCollect(t *testing.T, res gitimport.Result) *int {
	t.Helper()
	calls := 0
	orig := collectCommits
	collectCommits = func(context.Context, []config.GitSource, gitimport.Range) gitimport.Result {
		calls++
		return res
	}
	t.Cleanup(func() { collectCommits = orig })
	return &calls
}

func importPlanner() PlannerModel {
	m := newPlanner(&fakeAsana{})
	m.cfg.GitSources = []config.GitSource{{Kind: config.Local, Path: "/src"}}
	return m
}

func setRange(m PlannerModel, start, end string) PlannerModel {
	m.imp.start.SetValue(start)
	m.imp.end.SetValue(end)
	return m
}

var sampleCommits = gitimport.Result{
	Commits: []gitimport.Commit{
		{SHA: "a1", Repo: "api", When: localAt("2026-10-27 10:00"), Subject: "add retry", Additions: 120},
		{SHA: "w1", Repo: "web", When: localAt("2026-10-27 14:00"), Subject: "rename ads", Additions: 40},
		{SHA: "w2", Repo: "web", When: localAt("2026-10-28 09:00"), Subject: "fix filter"},
	},
	Warnings: []string{"GitHub: HTTP 401 — token invalid or expired"},
}

func TestImportNeedsAGitSource(t *testing.T) {
	m := newPlanner(&fakeAsana{})
	r, cmd := m.Update(keyMsg("i"))
	m = r.(PlannerModel)
	if m.step != plannerStepComposer {
		t.Fatalf("step = %v, want composer", m.step)
	}
	if got := lastError(collect(cmd)); !strings.Contains(got, "Settings → Git sources") {
		t.Fatalf("error = %q", got)
	}
}

func TestImportDefaultsToThisWeek(t *testing.T) {
	s, e := weekSoFar(localAt("2026-10-29 12:00")) // a Thursday
	if s != "2026-10-26" || e != "2026-10-29" {
		t.Fatalf("weekSoFar = %s..%s", s, e)
	}
	s, _ = weekSoFar(localAt("2026-11-01 12:00")) // a Sunday
	if s != "2026-10-26" {
		t.Fatalf("Sunday belongs to the week starting %s, got %s", "2026-10-26", s)
	}
}

func TestImportAddsPickedEntriesAsDraftTasks(t *testing.T) {
	calls := fakeCollect(t, sampleCommits)
	m := step(t, importPlanner(), keyMsg("i"))
	if m.step != plannerStepImport {
		t.Fatalf("step = %v, want import", m.step)
	}
	m = setRange(m, "2026-10-26", "2026-10-31")
	m = step(t, m, keyMsg("enter"))
	if *calls != 1 || !m.imp.loaded {
		t.Fatalf("fetch not done: calls=%d loaded=%v", *calls, m.imp.loaded)
	}
	if len(m.imp.entries) != 3 {
		t.Fatalf("want 3 entries (api 27, web 27, web 28), got %d", len(m.imp.entries))
	}
	view := m.View()
	for _, want := range []string{"api", "web", "token invalid", "3 picked"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	// Untick web on the 27th, add the rest.
	m = step(t, m, keyMsg("j"))
	m = step(t, m, keyMsg(" "))
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepComposer {
		t.Fatalf("step = %v, want composer", m.step)
	}
	if len(m.tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d", len(m.tasks))
	}
	api := m.tasks[0]
	if api.DueDate != "2026-10-27" || api.Title != "Working on api: add retry" || api.Hours < 0.5 {
		t.Errorf("api task = %+v", api)
	}
	if !strings.Contains(api.Description, "- add retry (a1)") || api.GID != "" || api.Completed {
		t.Errorf("api task = %+v", api)
	}
	if m.tasks[1].DueDate != "2026-10-28" {
		t.Errorf("second task due %s, want 2026-10-28", m.tasks[1].DueDate)
	}
}

func TestReimportSkipsWhatIsAlreadyAdded(t *testing.T) {
	fakeCollect(t, sampleCommits)
	m := step(t, importPlanner(), keyMsg("i"))
	m = setRange(m, "2026-10-26", "2026-10-31")
	m = step(t, m, keyMsg("enter"))
	m = step(t, m, keyMsg("enter")) // add all three
	if len(m.tasks) != 3 {
		t.Fatalf("want 3 tasks, got %d", len(m.tasks))
	}

	m = step(t, m, keyMsg("i")) // keeps the last range
	m = step(t, m, keyMsg("enter"))
	for i, p := range m.imp.picked {
		if p {
			t.Errorf("entry %d already added but picked again", i)
		}
	}
	if !strings.Contains(m.View(), "(added)") {
		t.Error("view should mark added entries")
	}
	r, cmd := m.Update(keyMsg("enter"))
	m = r.(PlannerModel)
	if len(m.tasks) != 3 || m.step != plannerStepImport {
		t.Fatalf("enter with nothing picked must not add or leave: tasks=%d step=%v", len(m.tasks), m.step)
	}
	if got := lastError(collect(cmd)); !strings.Contains(got, "nothing picked") {
		t.Fatalf("error = %q", got)
	}
}

func TestImportRejectsBadRange(t *testing.T) {
	calls := fakeCollect(t, sampleCommits)
	m := step(t, importPlanner(), keyMsg("i"))
	m = setRange(m, "2026-10-31", "2026-10-01")
	r, cmd := m.Update(keyMsg("enter"))
	m = r.(PlannerModel)
	if *calls != 0 || m.imp.loading {
		t.Fatal("bad range must not fetch")
	}
	if got := lastError(collect(cmd)); !strings.Contains(got, "before start date") {
		t.Fatalf("error = %q", got)
	}
}

func TestEscDuringImportDropsTheLateResult(t *testing.T) {
	fakeCollect(t, sampleCommits)
	m := step(t, importPlanner(), keyMsg("i"))
	m = setRange(m, "2026-10-26", "2026-10-31")
	r, cmd := m.Update(keyMsg("enter")) // fetch started, result not delivered yet
	m = r.(PlannerModel)
	late := runAll(cmd)
	m = step(t, m, keyMsg("esc"))
	if m.step != plannerStepComposer || m.imp.loading {
		t.Fatalf("esc should leave import: step=%v loading=%v", m.step, m.imp.loading)
	}
	for _, msg := range late {
		m = step(t, m, msg)
	}
	m = step(t, m, keyMsg("i"))
	if m.imp.loaded || len(m.tasks) != 0 {
		t.Fatalf("late result leaked into the new import: loaded=%v tasks=%d", m.imp.loaded, len(m.tasks))
	}
}
