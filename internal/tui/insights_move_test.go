package tui

import (
	"net/url"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type moveFake struct {
	asana.AsanaClient
	moved []string
}

func (f *moveFake) ListProjects(string) ([]asana.Project, error) {
	return []asana.Project{{GID: "dst", Name: "Product Development Team Planner 2026"}}, nil
}
func (f *moveFake) ListSections(string) ([]asana.Section, error) {
	return []asana.Section{{GID: "oct", Name: "October"}}, nil
}
func (f *moveFake) AddTaskToProject(task, proj, sec string) error {
	f.moved = append(f.moved, task+"→"+proj+"/"+sec)
	return nil
}
func (f *moveFake) SearchTasks(string, url.Values) ([]asana.Task, error) {
	return nil, nil
}

func weekTasks() []asana.Task {
	return []asana.Task{
		{GID: "t1", Name: "one", DueOn: "2026-10-19"},
		{GID: "t2", Name: "two", DueOn: "2026-10-20"},
		{GID: "t3", Name: "three", DueOn: "2026-10-21"},
	}
}

func insightsWith(tasks []asana.Task) InsightsModel {
	m := NewInsightsModel(&config.Config{}, nil)
	r, _ := m.Update(insightsResult{tasks: tasks})
	return r.(InsightsModel)
}

func ikey(m InsightsModel, k tea.KeyMsg) (InsightsModel, tea.Cmd) {
	r, cmd := m.Update(k)
	return r.(InsightsModel), cmd
}

func TestMenuHasNoHoursOrMoveTasks(t *testing.T) {
	m := NewMenu(true, true)
	if m.items[0].screen != screenInsights {
		t.Fatalf("first menu item = %q, want Insights", m.items[0].title)
	}
	for _, it := range m.items {
		if it.title == "Show Week Hours" || it.title == "Move Tasks" {
			t.Fatalf("menu still lists %q", it.title)
		}
	}
	r, cmd := m.Update(keyMsg("enter"))
	_ = r
	if sm, ok := cmd().(ScreenMsg); !ok || sm.Screen != screenInsights {
		t.Fatalf("enter on first item = %#v, want ScreenMsg{Insights}", cmd())
	}
}

func TestMenuRequiresBind(t *testing.T) {
	m := NewMenu(false, false)
	_, cmd := m.Update(keyMsg("enter"))
	if em, ok := cmd().(ErrorMsg); !ok || em.Message == "" {
		t.Fatalf("unbound Insights = %#v, want an error", cmd())
	}
}

func TestInsightsMoveSelectedTasks(t *testing.T) {
	m := insightsWith(weekTasks())
	m, _ = ikey(m, keyMsg(" ")) // pick t1, cursor → t2
	m, _ = ikey(m, keyMsg("j")) // cursor → t3
	m, _ = ikey(m, keyMsg(" ")) // pick t3
	_ = m.View()
	_, cmd := ikey(m, keyMsg("M"))
	msg, ok := cmd().(MoveTasksMsg)
	if !ok || len(msg.Tasks) != 2 || msg.Tasks[0].GID != "t1" || msg.Tasks[1].GID != "t3" {
		t.Fatalf("M sent %#v, want MoveTasksMsg with t1 and t3", cmd())
	}
}

func TestInsightsMoveHighlightedWhenNothingPicked(t *testing.T) {
	m := insightsWith(weekTasks())
	m, _ = ikey(m, keyMsg("j"))
	_, cmd := ikey(m, keyMsg("M"))
	if msg := cmd().(MoveTasksMsg); len(msg.Tasks) != 1 || msg.Tasks[0].GID != "t2" {
		t.Fatalf("M moved %v, want just t2", msg.Tasks)
	}
}

func TestInsightsSelectAllAndClearOnNavigate(t *testing.T) {
	m := insightsWith(weekTasks())
	m, _ = ikey(m, keyMsg("a"))
	if len(m.selected) != 3 {
		t.Fatalf("a selected %d, want 3", len(m.selected))
	}
	m, _ = ikey(m, keyMsg("a"))
	if len(m.selected) != 0 {
		t.Fatal("a again should clear the selection")
	}
	m, _ = ikey(m, keyMsg(" "))
	m, _ = ikey(m, tea.KeyMsg{Type: tea.KeyRight})
	if len(m.selected) != 0 {
		t.Fatal("changing week should drop the selection")
	}
}

func TestMoveTasksRoundTripKeepsWeek(t *testing.T) {
	f := &moveFake{}
	app := NewApp(&config.Config{AsanaToken: "x", DefaultWorkspaceGID: "w", DefaultProjectGID: "p"})
	app.asana = f
	week := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	app.insights.refDate = week
	app.insights.client = f
	app.screen = screenInsights

	// send delivers msg and every follow-up message until the app is idle.
	send := func(msg tea.Msg) {
		t.Helper()
		queue := []tea.Msg{msg}
		for i := 0; len(queue) > 0; i++ {
			if i > 100 {
				t.Fatal("app did not settle")
			}
			_, cmd := app.Update(queue[0])
			queue = append(queue[1:], runApp(cmd)...)
		}
	}
	send(MoveTasksMsg{Tasks: []asana.Task{{GID: "t1", Name: "one", DueOn: "2026-10-19"}}})
	if app.screen != screenMigrate || app.migrate.step != migrateStepPickTarget {
		t.Fatalf("screen=%v step=%v, want Move Tasks project picker", app.screen, app.migrate.step)
	}
	send(keyMsg("enter")) // pick the target → sections load
	if app.migrate.step != migrateStepPickSection || app.migrate.sections[app.migrate.sectionCursor].GID != "oct" {
		t.Fatalf("step=%v, want section picker on October", app.migrate.step)
	}
	send(keyMsg("enter")) // section
	send(keyMsg("enter")) // source: none → move
	if len(f.moved) != 1 || f.moved[0] != "t1→dst/oct" {
		t.Fatalf("moved %v, want t1→dst/oct", f.moved)
	}
	if app.migrate.step != migrateStepDone {
		t.Fatalf("step=%v, want done", app.migrate.step)
	}
	send(keyMsg("enter"))
	if app.screen != screenInsights || !app.insights.refDate.Equal(week) {
		t.Fatalf("screen=%v refDate=%v, want back on Insights at the same week", app.screen, app.insights.refDate)
	}
}

func TestMoveTasksEscReturnsToInsights(t *testing.T) {
	m := NewMigrateForTasks(&config.Config{}, &moveFake{}, []asana.Task{{GID: "t"}})
	r, _ := m.Update(migrateProjsMsg{projects: []asana.Project{{GID: "dst"}}})
	m = r.(MigrateModel)
	_, cmd := m.Update(keyMsg("esc"))
	if back, ok := cmd().(BackToInsightsMsg); !ok || back.Refresh {
		t.Fatalf("esc = %#v, want BackToInsightsMsg without refresh", cmd())
	}
}

// runApp runs a command and returns the messages the app needs back,
// skipping spinner ticks and error banners.
func runApp(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, runApp(c)...)
		}
		return out
	case migrateProjsMsg, migrateSectionsMsg, migrateExecMsg, BackToInsightsMsg, insightsResult:
		return []tea.Msg{msg}
	}
	return nil
}
