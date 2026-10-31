package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

type submitResult struct {
	idx     int
	gid     string   // parent task GID, "" if creation failed
	placed  bool     // month section applied
	subGIDs []string // per subtask, "" for those not created
	err     string
	done    bool
}

type sectionsResult struct {
	sections []asana.Section
	err      string
}

func (m *PlannerModel) fetchSections() tea.Cmd {
	asanaCli := m.asanaCli
	projGID := m.cfg.DefaultProjectGID
	return func() tea.Msg {
		if asanaCli == nil || projGID == "" {
			return sectionsResult{err: "no default project bound"}
		}
		secs, err := asanaCli.ListSections(projGID)
		if err != nil {
			return sectionsResult{err: err.Error()}
		}
		return sectionsResult{sections: secs}
	}
}

// monthSection finds the section named after dueDate's month, e.g.
// "October", "October 2026", "Oct" or "Oct 2026" (case-insensitive).
// A name carrying the matching year wins over a bare month name.
func monthSection(sections []asana.Section, dueDate string) (asana.Section, bool) {
	d, err := time.Parse("2006-01-02", dueDate)
	if err != nil {
		return asana.Section{}, false
	}
	full := strings.ToLower(d.Format("January"))
	short := strings.ToLower(d.Format("Jan"))
	year := d.Format("2006")
	candidates := []string{full + " " + year, full, short + " " + year, short}
	for _, want := range candidates {
		for _, sec := range sections {
			if strings.ToLower(strings.Join(strings.Fields(sec.Name), " ")) == want {
				return sec, true
			}
		}
	}
	return asana.Section{}, false
}

// placementProblems lists tasks whose month section cannot be found, which
// blocks submission so no task lands in the wrong month column.
func (m PlannerModel) placementProblems() []string {
	var probs []string
	for i, t := range m.tasks {
		if t.Placed || t.DueDate == "" {
			continue
		}
		if _, ok := monthSection(m.sections, t.DueDate); !ok {
			d, _ := time.Parse("2006-01-02", t.DueDate)
			probs = append(probs, fmt.Sprintf("task %d %q: no %q section in the default project", i+1, t.Title, d.Format("January")))
		}
	}
	return probs
}

func (m *PlannerModel) submitNext(idx int) tea.Cmd {
	if m.asanaCli == nil {
		return func() tea.Msg {
			return submitResult{idx: idx, err: "asana client not configured — set up Asana in Settings first"}
		}
	}
	// Skip tasks already fully in Asana (resubmit after a failure).
	for idx < len(m.tasks) && m.tasks[idx].submitted() {
		idx++
	}
	if idx >= len(m.tasks) {
		return func() tea.Msg { return submitResult{idx: idx, done: true} }
	}
	task := m.tasks[idx]
	var sectionGID string
	if task.DueDate != "" {
		sec, ok := monthSection(m.sections, task.DueDate)
		if !ok && !task.Placed {
			return func() tea.Msg {
				return submitResult{idx: idx, err: fmt.Sprintf("no month section for %q (due %s)", task.Title, task.DueDate)}
			}
		}
		sectionGID = sec.GID
	}
	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID
	hoursFieldGID := m.cfg.ActualHoursFieldGID
	userGID := m.cfg.UserGID
	entityFieldGID := m.cfg.EntityFieldGID
	entityFieldType := m.cfg.EntityFieldType
	productFieldGID := m.cfg.ProductFieldGID
	productFieldType := m.cfg.ProductFieldType
	entities, products := m.tagsFor(task)
	asanaCli := m.asanaCli

	return func() tea.Msg {
		res := submitResult{idx: idx, gid: task.GID, placed: task.Placed, subGIDs: make([]string, len(task.Subtasks))}
		for i, sub := range task.Subtasks {
			res.subGIDs[i] = sub.GID
		}

		if res.gid == "" {
			input := map[string]interface{}{
				"name":      task.Title,
				"notes":     task.Description,
				"projects":  []string{projGID},
				"workspace": wsGID,
				"completed": task.Completed,
			}
			if userGID != "" {
				input["assignee"] = userGID
			}
			if task.DueDate != "" {
				input["due_on"] = task.DueDate
			}
			customFields := map[string]interface{}{}
			if hoursFieldGID != "" && task.Hours > 0 {
				customFields[hoursFieldGID] = task.Hours
			}
			applyEnumField(customFields, entityFieldGID, entityFieldType, entities)
			applyEnumField(customFields, productFieldGID, productFieldType, products)
			if len(customFields) > 0 {
				input["custom_fields"] = customFields
			}
			created, err := asanaCli.CreateTask(input)
			if err != nil {
				res.err = err.Error()
				return res
			}
			res.gid = created.GID
		}

		// Move the task into its month section. addProject is idempotent, so a
		// retry after a failure here is safe.
		if !res.placed {
			if sectionGID != "" {
				if err := asanaCli.AddTaskToProject(res.gid, projGID, sectionGID); err != nil {
					res.err = fmt.Sprintf("%q created but not moved to its month section: %s", task.Title, err)
					return res
				}
			}
			res.placed = true
		}

		for i, sub := range task.Subtasks {
			if res.subGIDs[i] != "" {
				continue
			}
			subInput := map[string]interface{}{
				"name":      sub.Title,
				"notes":     sub.Description,
				"completed": sub.Completed,
			}
			if sub.DueDate != "" {
				subInput["due_on"] = sub.DueDate
			}
			if hoursFieldGID != "" && sub.Hours > 0 {
				subInput["custom_fields"] = map[string]interface{}{
					hoursFieldGID: sub.Hours,
				}
			}
			created, err := asanaCli.CreateSubtask(res.gid, subInput)
			if err != nil {
				res.err = fmt.Sprintf("subtask %q: %s", sub.Title, err.Error())
				return res
			}
			res.subGIDs[i] = created.GID
		}

		res.idx = idx + 1
		return res
	}
}

// applySubmitResult records what a submit step created, so it is never
// created twice.
func (m *PlannerModel) applySubmitResult(r submitResult) {
	i := r.idx
	if r.err == "" {
		i = r.idx - 1
	}
	if r.done || i < 0 || i >= len(m.tasks) {
		return
	}
	t := &m.tasks[i]
	if r.gid != "" {
		t.GID = r.gid
	}
	t.Placed = t.Placed || r.placed
	for j := range t.Subtasks {
		if j < len(r.subGIDs) && r.subGIDs[j] != "" {
			t.Subtasks[j].GID = r.subGIDs[j]
		}
	}
}

// applyEnumField sets enum or multi_enum custom field values into the map.
func applyEnumField(customFields map[string]interface{}, fieldGID, fieldType string, selected map[string]bool) {
	if fieldGID == "" || len(selected) == 0 {
		return
	}
	var gids []string
	for gid, on := range selected {
		if on {
			gids = append(gids, gid)
		}
	}
	if len(gids) == 0 {
		return
	}
	sort.Strings(gids) // map order is random; keep the payload deterministic
	if fieldType == "multi_enum" {
		customFields[fieldGID] = gids
	} else {
		// enum: single value only
		customFields[fieldGID] = gids[0]
	}
}

func (m *PlannerModel) handleLLMSubmit() tea.Cmd {
	if len(m.tasks) == 0 {
		return nil
	}
	m.step = plannerStepComposer
	return nil
}
