package tui

import (
	"net/url"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

// searchFake records the filters of every search and answers with tasks.
type searchFake struct {
	asana.AsanaClient
	filters []url.Values
	tasks   []asana.Task
}

func (f *searchFake) SearchTasks(_ string, v url.Values) ([]asana.Task, error) {
	f.filters = append(f.filters, v)
	return f.tasks, nil
}

// fetchResult runs the fetch part of cmd and returns its insightsResult.
func fetchResult(t *testing.T, cmd tea.Cmd) insightsResult {
	t.Helper()
	for _, msg := range runApp(cmd) {
		if r, ok := msg.(insightsResult); ok {
			return r
		}
	}
	t.Fatal("command did not fetch")
	return insightsResult{}
}

func scopedInsights(f *searchFake, userGID string) InsightsModel {
	return NewInsightsModel(&config.Config{DefaultProjectGID: "proj", DefaultWorkspaceGID: "ws", UserGID: userGID}, f)
}

func TestScopeToggleChangesSearchFilter(t *testing.T) {
	f := &searchFake{}
	m := scopedInsights(f, "me")
	m, cmd := m.startFetch()
	r, _ := m.Update(fetchResult(t, cmd))
	m = r.(InsightsModel)
	if got := f.filters[0].Get("projects.any"); got != "proj" {
		t.Fatalf("default scope projects.any = %q, want proj", got)
	}

	m, cmd = ikey(m, keyMsg("p"))
	if !m.allTasks {
		t.Fatal("p should switch to All my tasks")
	}
	r, _ = m.Update(fetchResult(t, cmd))
	m = r.(InsightsModel)
	last := f.filters[len(f.filters)-1]
	if last.Has("projects.any") || last.Get("assignee.any") != "me" {
		t.Fatalf("all-tasks filter = %v, want no project filter and assignee=me", last)
	}

	// scope survives week navigation
	_, cmd = ikey(m, tea.KeyMsg{Type: tea.KeyRight})
	fetchResult(t, cmd)
	if f.filters[len(f.filters)-1].Has("projects.any") {
		t.Fatal("next week went back to the default project")
	}
	_ = m.View()
}

func TestAllTasksNeedsUser(t *testing.T) {
	m := scopedInsights(&searchFake{}, "")
	m, cmd := ikey(m, keyMsg("p"))
	if m.allTasks {
		t.Fatal("All my tasks without a user would list everyone's tasks")
	}
	if em, ok := cmd().(ErrorMsg); !ok || em.Message == "" {
		t.Fatalf("want an error, got %#v", cmd())
	}
}

func TestStaleInsightsResultIsDropped(t *testing.T) {
	f := &searchFake{}
	m := scopedInsights(f, "me")
	m, first := m.startFetch()
	m, second := ikey(m, tea.KeyMsg{Type: tea.KeyRight}) // newer fetch

	old := fetchResult(t, first)
	old.tasks = []asana.Task{{GID: "old"}}
	r, _ := m.Update(old)
	m = r.(InsightsModel)
	if !m.loading || len(m.tasks) != 0 {
		t.Fatalf("older result was applied: loading=%v tasks=%v", m.loading, m.tasks)
	}

	cur := fetchResult(t, second)
	cur.tasks = []asana.Task{{GID: "new"}}
	r, _ = m.Update(cur)
	m = r.(InsightsModel)
	if m.loading || len(m.tasks) != 1 || m.tasks[0].GID != "new" {
		t.Fatalf("latest result not applied: loading=%v tasks=%v", m.loading, m.tasks)
	}
}

func TestOpeningInsightsShowsFirstResult(t *testing.T) {
	f := &searchFake{tasks: []asana.Task{{GID: "t", Name: "x", DueOn: "2026-10-19"}}}
	app := NewApp(&config.Config{AsanaToken: "x", DefaultWorkspaceGID: "w", DefaultProjectGID: "p", UserGID: "me"})
	app.asana = f
	_, cmd := app.Update(ScreenMsg{Screen: screenInsights})
	for _, msg := range runApp(cmd) {
		app.Update(msg)
	}
	if app.insights.loading || len(app.insights.tasks) != 1 {
		t.Fatalf("first fetch result dropped: loading=%v tasks=%d", app.insights.loading, len(app.insights.tasks))
	}
}

func TestTaskProjects(t *testing.T) {
	task := asana.Task{Memberships: []asana.TaskMembership{
		{Project: asana.ProjectRef{Name: "Product Development Team Planner 2026"}},
		{Project: asana.ProjectRef{Name: "elfo-Omnichannel"}},
	}}
	if got := taskProjects(task); got != "Product Development Team Planner 2026, elfo-Omnichannel" {
		t.Fatalf("taskProjects = %q", got)
	}
	if got := taskProjects(asana.Task{}); got != "no project" {
		t.Fatalf("taskProjects(no memberships) = %q", got)
	}
}
