package tui

import (
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

// fakeAsana records submit calls. failTask / failSub make the n-th call
// (1-based, counted over the client's lifetime) fail once.
type fakeAsana struct {
	asana.AsanaClient
	sections []asana.Section

	taskCalls, subCalls, placeCalls int
	failTask, failSub, failPlace    int
	created                         []string // task names created
	placed                          map[string]string
}

func (f *fakeAsana) CreateTask(in map[string]interface{}) (*asana.Task, error) {
	f.taskCalls++
	if f.taskCalls == f.failTask {
		return nil, errors.New("create failed")
	}
	name := in["name"].(string)
	f.created = append(f.created, name)
	return &asana.Task{GID: "gid-" + name}, nil
}

func (f *fakeAsana) CreateSubtask(parent string, in map[string]interface{}) (*asana.Task, error) {
	f.subCalls++
	if f.subCalls == f.failSub {
		return nil, errors.New("subtask failed")
	}
	name := in["name"].(string)
	f.created = append(f.created, name)
	return &asana.Task{GID: "gid-" + name}, nil
}

func (f *fakeAsana) AddTaskToProject(taskGID, projectGID, sectionGID string) error {
	f.placeCalls++
	if f.placeCalls == f.failPlace {
		return errors.New("addProject failed")
	}
	if f.placed == nil {
		f.placed = map[string]string{}
	}
	f.placed[taskGID] = sectionGID
	return nil
}

func (f *fakeAsana) ListSections(string) ([]asana.Section, error) { return f.sections, nil }

var monthSections = []asana.Section{
	{GID: "sec-sep", Name: "September"},
	{GID: "sec-oct", Name: "October"},
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// step sends msg and then runs every command it produces until the model
// goes quiet, feeding results (submit, sections) back like the runtime would.
func step(t *testing.T, m PlannerModel, msg tea.Msg) PlannerModel {
	t.Helper()
	queue := []tea.Msg{msg}
	for i := 0; len(queue) > 0; i++ {
		if i > 200 {
			t.Fatal("command loop did not settle")
		}
		next := queue[0]
		queue = queue[1:]
		r, cmd := m.Update(next)
		m = r.(PlannerModel)
		queue = append(queue, runAll(cmd)...)
	}
	return m
}

// runAll executes a command (expanding batches) and keeps only messages the
// planner reacts to; spinner ticks and error banners are dropped.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, runAll(c)...)
		}
		return out
	case submitResult, sectionsResult:
		return []tea.Msg{msg}
	}
	return nil
}

func newPlanner(f *fakeAsana, tasks ...PlanTask) PlannerModel {
	m := NewPlannerModel(&config.Config{DefaultProjectGID: "proj", DefaultWorkspaceGID: "ws"}, f, nil)
	m.tasks = tasks
	return m
}

// submitFromReview goes review → confirm (sections load) → enter.
func submitFromReview(t *testing.T, m PlannerModel) PlannerModel {
	t.Helper()
	m.step = plannerStepReview
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepConfirm || !m.sectionsLoaded {
		t.Fatalf("after review enter: step=%v sectionsLoaded=%v, want confirm with sections", m.step, m.sectionsLoaded)
	}
	return step(t, m, keyMsg("enter"))
}

func TestSecondEnterWhileSubmittingIsIgnored(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f, PlanTask{Title: "a", DueDate: "2026-10-20"})
	m.step = plannerStepReview
	m = step(t, m, keyMsg("enter")) // to confirm, sections loaded

	r, first := m.Update(keyMsg("enter"))
	m = r.(PlannerModel)
	if m.step != plannerStepSubmitting {
		t.Fatalf("step=%v, want submitting", m.step)
	}
	r, second := m.Update(keyMsg("enter"))
	m = r.(PlannerModel)
	if second != nil {
		t.Fatal("second enter while submitting returned a command; it would start a duplicate submit chain")
	}
	for _, msg := range runAll(first) {
		m = step(t, m, msg)
	}
	if len(f.created) != 1 {
		t.Fatalf("created %v, want exactly one task", f.created)
	}
}

func TestEscWhileSubmittingIsIgnored(t *testing.T) {
	m := newPlanner(&fakeAsana{})
	m.step = plannerStepSubmitting
	r, cmd := m.Update(keyMsg("esc"))
	if got := r.(PlannerModel).step; got != plannerStepSubmitting || cmd != nil {
		t.Fatalf("esc while submitting: step=%v cmd=%v, want to stay submitting", got, cmd != nil)
	}
}

func TestResubmitAfterTaskFailureSkipsCreated(t *testing.T) {
	f := &fakeAsana{sections: monthSections, failTask: 2}
	m := newPlanner(f,
		PlanTask{Title: "a", DueDate: "2026-10-20"},
		PlanTask{Title: "b", DueDate: "2026-10-21"},
	)
	m = submitFromReview(t, m)
	if m.step != plannerStepReview {
		t.Fatalf("after failure step=%v, want review", m.step)
	}
	if !m.tasks[0].submitted() || m.tasks[1].GID != "" {
		t.Fatalf("progress not recorded: %+v", m.tasks)
	}

	m = submitFromReview(t, m)
	if m.step != plannerStepDone {
		t.Fatalf("after resubmit step=%v, want done", m.step)
	}
	if fmt.Sprint(f.created) != "[a b]" {
		t.Fatalf("created %v, want [a b] (no duplicates)", f.created)
	}
}

func TestResubmitAfterSubtaskFailureKeepsParent(t *testing.T) {
	f := &fakeAsana{sections: monthSections, failSub: 2}
	m := newPlanner(f, PlanTask{
		Title: "parent", DueDate: "2026-10-20",
		Subtasks: []PlanTask{{Title: "s1"}, {Title: "s2"}},
	})
	m = submitFromReview(t, m)
	m = submitFromReview(t, m)
	if m.step != plannerStepDone {
		t.Fatalf("step=%v, want done", m.step)
	}
	if fmt.Sprint(f.created) != "[parent s1 s2]" {
		t.Fatalf("created %v, want [parent s1 s2] (parent and s1 once)", f.created)
	}
}

func TestResubmitAfterPlacementFailureOnlyMovesTask(t *testing.T) {
	f := &fakeAsana{sections: monthSections, failPlace: 1}
	m := newPlanner(f, PlanTask{Title: "a", DueDate: "2026-10-20"})
	m = submitFromReview(t, m)
	if m.tasks[0].GID == "" || m.tasks[0].Placed {
		t.Fatalf("want task created but not placed, got %+v", m.tasks[0])
	}
	m = submitFromReview(t, m)
	if m.step != plannerStepDone || f.taskCalls != 1 || f.placed["gid-a"] != "sec-oct" {
		t.Fatalf("step=%v taskCalls=%d placed=%v, want done, 1 create, a→sec-oct", m.step, f.taskCalls, f.placed)
	}
}

func TestTaskGoesToItsMonthSection(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f,
		PlanTask{Title: "oct", DueDate: "2026-10-20"},
		PlanTask{Title: "sep", DueDate: "2026-09-30"},
		PlanTask{Title: "nodate"},
	)
	m = submitFromReview(t, m)
	if m.step != plannerStepDone {
		t.Fatalf("step=%v, want done", m.step)
	}
	if f.placed["gid-oct"] != "sec-oct" || f.placed["gid-sep"] != "sec-sep" {
		t.Fatalf("placement %v, want oct→sec-oct, sep→sec-sep", f.placed)
	}
	if _, moved := f.placed["gid-nodate"]; moved {
		t.Fatal("task without due date should stay in the project's default section")
	}
}

func TestMissingMonthSectionBlocksSubmit(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f, PlanTask{Title: "nov", DueDate: "2026-11-02"})
	m = submitFromReview(t, m)
	if m.step != plannerStepConfirm {
		t.Fatalf("step=%v, want to stay on confirm", m.step)
	}
	if f.taskCalls != 0 {
		t.Fatal("task was created although its month section does not exist")
	}
}

func TestStaleCursorAfterDeleteDoesNotPanic(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a"}, PlanTask{Title: "b"})
	m = step(t, m, keyMsg("tab")) // planner cursor → task 2
	m.step = plannerStepReview
	m = step(t, m, keyMsg("d")) // delete task 1
	m = step(t, m, keyMsg("esc"))
	_ = m.View()

	m.step = plannerStepMenu
	m.menuIdx = 1 // Add Subtask
	m = step(t, m, keyMsg("enter"))
	_ = m.View()
	if m.subParent != 0 {
		t.Fatalf("subParent=%d, want 0 (the only task left)", m.subParent)
	}
}

func TestDeletingLastTaskLeavesReview(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a"})
	m.step = plannerStepReview
	m = step(t, m, keyMsg("d"))
	if m.step != plannerStepMenu {
		t.Fatalf("step=%v, want menu after deleting the last task", m.step)
	}
	_ = m.View()
}

func TestMonthSection(t *testing.T) {
	secs := []asana.Section{
		{GID: "1", Name: "Oct"},
		{GID: "2", Name: " october "},
		{GID: "3", Name: "October 2026"},
		{GID: "4", Name: "October 2025"},
		{GID: "5", Name: "Backlog"},
	}
	tests := []struct {
		name     string
		sections []asana.Section
		due      string
		want     string
	}{
		{"full name with matching year wins", secs, "2026-10-20", "3"},
		{"bare month beats short name", secs[:2], "2026-10-20", "2"},
		{"short name accepted", secs[:1], "2026-10-01", "1"},
		{"other year only is no match", secs[3:], "2026-10-20", ""},
		{"no month section", secs[4:], "2026-10-20", ""},
		{"invalid date", secs, "20-10-2026", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := monthSection(tt.sections, tt.due)
			if got.GID != tt.want || ok != (tt.want != "") {
				t.Fatalf("monthSection(%q) = %q,%v want %q", tt.due, got.GID, ok, tt.want)
			}
		})
	}
}
