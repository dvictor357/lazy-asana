package tui

import (
	"errors"
	"fmt"
	"strings"
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
	fields                          map[string]map[string]interface{} // custom_fields per created task
	inputs                          map[string]map[string]interface{} // full create payload per name
}

func (f *fakeAsana) CreateTask(in map[string]interface{}) (*asana.Task, error) {
	f.taskCalls++
	if f.taskCalls == f.failTask {
		return nil, errors.New("create failed")
	}
	name := in["name"].(string)
	f.created = append(f.created, name)
	f.record(name, in)
	if cf, ok := in["custom_fields"].(map[string]interface{}); ok {
		if f.fields == nil {
			f.fields = map[string]map[string]interface{}{}
		}
		f.fields[name] = cf
	}
	return &asana.Task{GID: "gid-" + name}, nil
}

func (f *fakeAsana) record(name string, in map[string]interface{}) {
	if f.inputs == nil {
		f.inputs = map[string]map[string]interface{}{}
	}
	f.inputs[name] = in
}

func (f *fakeAsana) GetCustomFieldSettings(string) ([]asana.CustomFieldSetting, error) {
	return []asana.CustomFieldSetting{
		{CustomField: asana.CustomField{GID: "ent", EnumOptions: []asana.EnumOption{{GID: "e-elfo", Name: "elfo"}, {GID: "e-x", Name: "X"}}}},
		{CustomField: asana.CustomField{GID: "prod", EnumOptions: []asana.EnumOption{{GID: "p-a2p", Name: "elfo A2P"}, {GID: "p-y", Name: "Y"}}}},
	}, nil
}

func (f *fakeAsana) CreateSubtask(parent string, in map[string]interface{}) (*asana.Task, error) {
	f.subCalls++
	if f.subCalls == f.failSub {
		return nil, errors.New("subtask failed")
	}
	name := in["name"].(string)
	f.created = append(f.created, name)
	f.record(name, in)
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
	case submitResult, sectionsResult, tagOptionsResult:
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
	m.step = plannerStepComposer
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepConfirm || !m.sectionsLoaded {
		t.Fatalf("after review enter: step=%v sectionsLoaded=%v, want confirm with sections", m.step, m.sectionsLoaded)
	}
	return step(t, m, keyMsg("enter"))
}

func TestSecondEnterWhileSubmittingIsIgnored(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f, PlanTask{Title: "a", DueDate: "2026-10-20"})
	m.step = plannerStepComposer
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
	if m.step != plannerStepComposer {
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
	m = step(t, m, keyMsg("j")) // select task 2
	m = step(t, m, keyMsg("d")) // delete it
	if m.reviewCursor != 0 {
		t.Fatalf("cursor=%d, want 0 after deleting the last row", m.reviewCursor)
	}
	_ = m.View()

	m = step(t, m, keyMsg("s")) // add subtask to the remaining task
	_ = m.View()
	if m.step != plannerStepAddSubtask || m.subParent != 0 {
		t.Fatalf("step=%v subParent=%d, want subtask form for task 0", m.step, m.subParent)
	}
}

func TestEditTaskKeepsSubtasksAndTags(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{
		Title: "old", Hours: 1, DueDate: "2026-10-19",
		Subtasks: []PlanTask{{Title: "s"}}, HasTags: true, Entities: map[string]bool{"e": true},
	})
	m = step(t, m, keyMsg("e"))
	if m.step != plannerStepAddTask || m.titleInput.Value() != "old" || m.hoursInput.Value() != "1" {
		t.Fatalf("edit form not prefilled: step=%v title=%q hours=%q", m.step, m.titleInput.Value(), m.hoursInput.Value())
	}
	m.titleInput.SetValue("new")
	m.hoursInput.SetValue("2.5")
	m = step(t, m, keyMsg("enter"))
	got := m.tasks[0]
	if len(m.tasks) != 1 || got.Title != "new" || got.Hours != 2.5 || len(got.Subtasks) != 1 || !got.HasTags {
		t.Fatalf("after edit: %d tasks, %+v", len(m.tasks), got)
	}
}

func TestSentTaskCannotBeEdited(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a", GID: "g", Placed: true})
	m = step(t, m, keyMsg("e"))
	if m.step != plannerStepComposer {
		t.Fatalf("step=%v, want to stay in composer", m.step)
	}
}

func TestComposerViewRendersAtSmallAndLargeWidths(t *testing.T) {
	m := newPlanner(&fakeAsana{},
		PlanTask{Title: "a very long task title that will not fit in the column", DueDate: "2026-10-19", Hours: 9,
			Subtasks: []PlanTask{{Title: "sub", Hours: 1}}},
		PlanTask{Title: "no date"},
	)
	for _, w := range []int{0, 40, 80, 200} {
		m.width = w
		if out := m.View(); !strings.Contains(out, "Task Composer") {
			t.Fatalf("width %d: composer title missing", w)
		}
	}
	m.tasks = nil
	_ = m.View()
}

func TestDeletingLastTaskKeepsComposerUsable(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a"})
	m.step = plannerStepComposer
	m = step(t, m, keyMsg("d"))
	if m.step != plannerStepComposer {
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

func newTaggedPlanner(f *fakeAsana, prodType string, tasks ...PlanTask) PlannerModel {
	m := NewPlannerModel(&config.Config{
		DefaultProjectGID: "proj", DefaultWorkspaceGID: "ws",
		EntityFieldGID: "ent", EntityFieldType: "enum",
		ProductFieldGID: "prod", ProductFieldType: prodType,
	}, f, nil)
	m.tasks = tasks
	m.step = plannerStepComposer
	return m
}

// pick moves to option i of the current field and selects it.
func pick(t *testing.T, m PlannerModel, i int) PlannerModel {
	t.Helper()
	m.taggingCursor = i
	return step(t, m, keyMsg(" "))
}

func TestPerTaskTagsAndBatchDefault(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newTaggedPlanner(f, "enum",
		PlanTask{Title: "a", DueDate: "2026-10-19"},
		PlanTask{Title: "b", DueDate: "2026-10-19"},
	)
	// task b gets its own tags: X / Y
	m.reviewCursor = 1
	m = step(t, m, keyMsg("t"))
	if m.step != plannerStepTagging || m.tagTarget != 1 {
		t.Fatalf("t: step=%v target=%d, want tagging task 1", m.step, m.tagTarget)
	}
	m = pick(t, m, 1)             // entity X
	m = step(t, m, keyMsg("tab")) // product field
	m = pick(t, m, 1)             // product Y
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepComposer || !m.tasks[1].HasTags {
		t.Fatalf("after save: step=%v hasTags=%v, want back on review with tags", m.step, m.tasks[1].HasTags)
	}

	// submit: batch default for task a is elfo / elfo A2P
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepTagging || m.tagTarget != -1 {
		t.Fatalf("submit: step=%v target=%d, want batch default picker", m.step, m.tagTarget)
	}
	m = pick(t, m, 0)
	m = step(t, m, keyMsg("tab"))
	m = pick(t, m, 0)
	m = step(t, m, keyMsg("enter"))
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepDone {
		t.Fatalf("step=%v, want done", m.step)
	}
	want := map[string]string{"a": "e-elfo p-a2p", "b": "e-x p-y"}
	for task, w := range want {
		cf := f.fields[task]
		if got := fmt.Sprint(cf["ent"], " ", cf["prod"]); got != w {
			t.Errorf("task %s tags = %s, want %s", task, got, w)
		}
	}
}

func TestBatchPickerSkippedWhenAllTasksTagged(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newTaggedPlanner(f, "enum", PlanTask{Title: "a", DueDate: "2026-10-19"})
	m = step(t, m, keyMsg("t"))
	m = pick(t, m, 0)
	m = step(t, m, keyMsg("enter"))
	m = step(t, m, keyMsg("enter"))
	if m.step != plannerStepConfirm {
		t.Fatalf("step=%v, want confirm (no batch picker needed)", m.step)
	}
}

func TestEnumFieldKeepsOneOption(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = pick(t, m, 0)
	m = pick(t, m, 1)
	if len(m.selectedEntities) != 1 || !m.selectedEntities["e-x"] {
		t.Fatalf("enum selection = %v, want only e-x", m.selectedEntities)
	}
	m = pick(t, m, 1) // toggle off
	if len(m.selectedEntities) != 0 {
		t.Fatalf("enum selection = %v, want empty after toggling off", m.selectedEntities)
	}
}

func TestMultiEnumFieldKeepsSeveral(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "multi_enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = step(t, m, keyMsg("tab"))
	m = pick(t, m, 0)
	m = pick(t, m, 1)
	if len(m.selectedProducts) != 2 {
		t.Fatalf("multi_enum selection = %v, want both", m.selectedProducts)
	}
}

func TestEscInTagPickerReturnsToReview(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = pick(t, m, 0)
	m = step(t, m, keyMsg("esc"))
	if m.step != plannerStepComposer || m.tasks[0].HasTags {
		t.Fatalf("esc: step=%v hasTags=%v, want review without saving", m.step, m.tasks[0].HasTags)
	}
}

func TestClearTaskTags(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = pick(t, m, 0)
	m = step(t, m, keyMsg("enter"))
	m = step(t, m, keyMsg("u"))
	if m.tasks[0].HasTags {
		t.Fatal("u should drop the task's own tags")
	}
	_ = m.View()
}

func TestCompletedToggleInFormIsSubmitted(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f)
	m = step(t, m, keyMsg("a"))
	m.titleInput.SetValue("done task")
	m.dateInput.SetValue("2026-10-19")
	for i := 0; i < formFieldDone; i++ {
		m = step(t, m, keyMsg("tab"))
	}
	m = step(t, m, keyMsg(" "))
	if !m.formDone {
		t.Fatal("space on the Status field should toggle completed")
	}
	m = step(t, m, keyMsg("enter"))
	if !m.tasks[0].Completed {
		t.Fatal("saved task is not completed")
	}
	m = step(t, m, keyMsg("enter")) // confirm
	m = step(t, m, keyMsg("enter")) // submit
	if got := f.inputs["done task"]["completed"]; got != true {
		t.Fatalf("CreateTask completed = %v, want true", got)
	}
}

func TestCompletedDefaultsToFalse(t *testing.T) {
	f := &fakeAsana{sections: monthSections}
	m := newPlanner(f, PlanTask{Title: "open", DueDate: "2026-10-19", Subtasks: []PlanTask{{Title: "sub"}}})
	m = submitFromReview(t, m)
	if f.inputs["open"]["completed"] != false || f.inputs["sub"]["completed"] != false {
		t.Fatalf("completed = %v / %v, want false for task and subtask", f.inputs["open"]["completed"], f.inputs["sub"]["completed"])
	}
}

func TestComposerCTogglesCompleted(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a"}, PlanTask{Title: "b", GID: "g", Placed: true})
	m = step(t, m, keyMsg("c"))
	if !m.tasks[0].Completed {
		t.Fatal("c should mark the selected task completed")
	}
	m = step(t, m, keyMsg("c"))
	if m.tasks[0].Completed {
		t.Fatal("c again should clear it")
	}
	m = step(t, m, keyMsg("j"))
	m = step(t, m, keyMsg("c"))
	if m.tasks[1].Completed {
		t.Fatal("a task already in Asana must not change locally")
	}
	_ = m.View()
}

func TestEditPrefillsCompleted(t *testing.T) {
	m := newPlanner(&fakeAsana{}, PlanTask{Title: "a", Completed: true})
	m = step(t, m, keyMsg("e"))
	if !m.formDone {
		t.Fatal("edit form should show the task as completed")
	}
	m = step(t, m, keyMsg("a"))
	_ = m
}

func typeText(t *testing.T, m PlannerModel, text string) PlannerModel {
	t.Helper()
	for _, r := range text {
		m = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestTagSearchFiltersAndSelects(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = step(t, m, keyMsg("tab")) // Product/Project
	m = step(t, m, keyMsg("/"))
	m = typeText(t, m, "a2p j") // j is typed, not a move
	if got := m.tagSearch.Value(); got != "a2p j" {
		t.Fatalf("filter = %q, want %q", got, "a2p j")
	}
	if n := len(m.currentTagOptions()); n != 0 {
		t.Fatalf("%d matches for %q, want 0", n, "a2p j")
	}
	m = step(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = step(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	opts := m.currentTagOptions()
	if len(opts) != 1 || opts[0].GID != "p-a2p" {
		t.Fatalf("filter %q matched %v, want only elfo A2P", m.tagSearch.Value(), opts)
	}
	m = step(t, m, keyMsg("enter")) // leave filter mode, keep filter
	m = step(t, m, keyMsg(" "))
	if !m.selectedProducts["p-a2p"] {
		t.Fatalf("selected %v, want p-a2p", m.selectedProducts)
	}
	m = step(t, m, keyMsg("esc")) // clears filter
	if m.tagSearch.Value() != "" || m.step != plannerStepTagging {
		t.Fatalf("first esc: filter=%q step=%v, want cleared filter, still tagging", m.tagSearch.Value(), m.step)
	}
	_ = m.View()
	m = step(t, m, keyMsg("enter"))
	if !m.tasks[0].Products["p-a2p"] {
		t.Fatal("tag picked through the filter was not saved")
	}
}

func TestTagSearchResetsOnFieldSwitch(t *testing.T) {
	m := newTaggedPlanner(&fakeAsana{}, "enum", PlanTask{Title: "a"})
	m = step(t, m, keyMsg("t"))
	m = step(t, m, keyMsg("/"))
	m = typeText(t, m, "elfo")
	m = step(t, m, keyMsg("enter"))
	m = step(t, m, keyMsg("tab"))
	if m.tagSearch.Value() != "" || len(m.currentTagOptions()) != 2 {
		t.Fatalf("after tab: filter=%q options=%d, want empty filter and all options", m.tagSearch.Value(), len(m.currentTagOptions()))
	}
}

func TestFilterOptionsMatchesAllWords(t *testing.T) {
	opts := []asana.EnumOption{{GID: "1", Name: "elfo A2P"}, {GID: "2", Name: "elfo Omnichannel"}, {GID: "3", Name: "Kopi Kenangan"}}
	cases := map[string]string{"": "[1 2 3]", "ELFO": "[1 2]", "a2p elfo": "[1]", "omni ch": "[2]", "nope": "[]"}
	for q, want := range cases {
		var got []string
		for _, o := range filterOptions(opts, q) {
			got = append(got, o.GID)
		}
		if g := fmt.Sprint(got); g != want && !(want == "[]" && len(got) == 0) {
			t.Errorf("filterOptions(%q) = %s, want %s", q, g, want)
		}
	}
}
