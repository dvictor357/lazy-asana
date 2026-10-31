package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

type llmDescResult struct {
	desc string
	err  string
}

func (m PlannerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == plannerStepSubmitting {
				// Leaving mid-submit would silently stop the chain halfway.
				return m, nil
			}
			if m.step == plannerStepMenu {
				return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
			}
			m.step = plannerStepMenu
			return m, nil
		}
	}

	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if r, ok := msg.(sectionsResult); ok {
		m.sections = r.sections
		m.sectionsErr = r.err
		m.sectionsLoaded = true
		return m, nil
	}

	switch m.step {
	case plannerStepMenu:
		return m.updateMenu(msg)
	case plannerStepAddTask, plannerStepAddSubtask:
		return m.updateAddTask(msg)
	case plannerStepReview:
		return m.updateReview(msg)
	case plannerStepTagging:
		return m.updateTagging(msg)
	case plannerStepConfirm, plannerStepSubmitting:
		return m.updateConfirm(msg)
	case plannerStepLLMPlan:
		return m.updateLLMPlan(msg)
	case plannerStepDone:
		return m.updateDone(msg)
	}

	return m, nil
}

func (m *PlannerModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j", "ctrl+n":
		if m.menuIdx < 3 {
			m.menuIdx++
		}
	case "enter":
		switch m.menuIdx {
		case 0:
			m.step = plannerStepAddTask
			m.fieldCursor = 0
			m.titleInput.SetValue("")
			m.descInput.SetValue("")
			m.hoursInput.SetValue("")
			m.dateInput.SetValue("")
			m.descInput.Blur()
			m.hoursInput.Blur()
			m.dateInput.Blur()
			m.titleInput.Focus()
			return *m, func() tea.Msg { return ErrorMsg{Message: ""} }
		case 1:
			if len(m.tasks) == 0 {
				return *m, nil
			}
			m.clampCursors()
			m.step = plannerStepAddSubtask
			m.subParent = m.cursor
			m.fieldCursor = 0
			m.titleInput.SetValue("")
			m.descInput.SetValue("")
			m.hoursInput.SetValue("")
			m.dateInput.SetValue("")
			m.descInput.Blur()
			m.hoursInput.Blur()
			m.dateInput.Blur()
			m.titleInput.Focus()
			return *m, func() tea.Msg { return ErrorMsg{Message: ""} }
		case 2:
			if m.llmCli == nil {
				return *m, func() tea.Msg { return ErrorMsg{Message: "no LLM key configured"} }
			}
			m.step = plannerStepLLMPlan
			m.llmContext.SetValue("")
			m.llmContext.Blur()
			m.llmStartDate.SetValue("")
			m.llmStartDate.Focus()
			m.llmFieldCursor = 0
			return *m, func() tea.Msg { return ErrorMsg{Message: ""} }
		case 3:
			if len(m.tasks) == 0 {
				return *m, nil
			}
			m.step = plannerStepReview
			return *m, nil
		}
	case "tab", "ctrl+f":
		if len(m.tasks) > 0 {
			m.cursor = (m.cursor + 1) % len(m.tasks)
		}
	case "shift+tab", "ctrl+b":
		if len(m.tasks) > 0 {
			m.cursor = (m.cursor - 1 + len(m.tasks)) % len(m.tasks)
		}
	}
	return *m, nil
}

func (m *PlannerModel) updateAddTask(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		if t, ok2 := msg.(llmDescResult); ok2 {
			m.llmDescGenerating = false
			if t.err == "" && t.desc != "" {
				m.descInput.SetValue(t.desc)
			} else if t.err != "" {
				return *m, func() tea.Msg { return ErrorMsg{Message: "LLM error: " + t.err} }
			}
			return *m, nil
		}
		return m.updateFocusedField(msg)
	}

	switch k.String() {
	case "tab", "ctrl+f":
		m.cycleField(1)
		return *m, nil
	case "shift+tab", "ctrl+b":
		m.cycleField(-1)
		return *m, nil
	case "ctrl+g":
		cmd := m.handleGenerateDescription()
		if cmd != nil {
			m.llmDescGenerating = true
		}
		return *m, cmd
	case "ctrl+s":
		return *m, m.handleSaveTask()
	case "enter":
		if m.fieldCursor == 1 {
			// description textarea: pass enter through for newline
			var cmd tea.Cmd
			m.descInput, cmd = m.descInput.Update(msg)
			return *m, cmd
		}
		return *m, m.handleSaveTask()
	}

	return m.updateFocusedField(msg)
}

func (m *PlannerModel) cycleField(dir int) {
	m.titleInput.Blur()
	m.descInput.Blur()
	m.hoursInput.Blur()
	m.dateInput.Blur()
	m.fieldCursor = ((m.fieldCursor+dir)%4 + 4) % 4
	switch m.fieldCursor {
	case 0:
		m.titleInput.Focus()
	case 1:
		m.descInput.Focus()
	case 2:
		m.hoursInput.Focus()
	case 3:
		m.dateInput.Focus()
	}
}

func (m *PlannerModel) updateFocusedField(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.fieldCursor {
	case 0:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case 1:
		m.descInput, cmd = m.descInput.Update(msg)
	case 2:
		m.hoursInput, cmd = m.hoursInput.Update(msg)
	case 3:
		m.dateInput, cmd = m.dateInput.Update(msg)
	}
	return *m, cmd
}

func (m *PlannerModel) handleGenerateDescription() tea.Cmd {
	if m.llmCli == nil || m.titleInput.Value() == "" {
		return nil
	}
	title := m.titleInput.Value()
	return func() tea.Msg {
		desc, err := m.llmCli.GenerateTaskDescription(title)
		if err != nil {
			return llmDescResult{err: err.Error()}
		}
		return llmDescResult{desc: desc}
	}
}

func (m *PlannerModel) handleSaveTask() tea.Cmd {
	title := m.titleInput.Value()
	if title == "" {
		return nil
	}

	var hours float64
	hoursStr := m.hoursInput.Value()
	if hoursStr != "" {
		if _, err := fmt.Sscanf(hoursStr, "%f", &hours); err != nil || hours == 0 {
			return func() tea.Msg { return ErrorMsg{Message: "invalid hours: enter a number (e.g. 1.5)"} }
		}
	}

	if dateStr := m.dateInput.Value(); dateStr != "" {
		if err := asana.ValidateDate(dateStr); err != nil {
			return func() tea.Msg { return ErrorMsg{Message: "invalid date: use YYYY-MM-DD format"} }
		}
	}

	task := PlanTask{
		Title:       title,
		Description: m.descInput.Value(),
		Hours:       hours,
		DueDate:     m.dateInput.Value(),
	}

	if m.step == plannerStepAddSubtask {
		if m.subParent < 0 || m.subParent >= len(m.tasks) {
			m.step = plannerStepMenu
			return func() tea.Msg { return ErrorMsg{Message: "parent task no longer exists — subtask not added"} }
		}
		m.tasks[m.subParent].Subtasks = append(m.tasks[m.subParent].Subtasks, task)
	} else {
		m.tasks = append(m.tasks, task)
	}

	m.step = plannerStepMenu
	return nil
}

func (m *PlannerModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.reviewCursor > 0 {
			m.reviewCursor--
		}
	case "down", "j", "ctrl+n":
		if m.reviewCursor < len(m.tasks)-1 {
			m.reviewCursor++
		}
	case "d", "delete":
		if len(m.tasks) > 0 {
			m.tasks = append(m.tasks[:m.reviewCursor], m.tasks[m.reviewCursor+1:]...)
			m.clampCursors()
			if len(m.tasks) == 0 {
				m.step = plannerStepMenu
			}
		}
	case "enter":
		if len(m.tasks) > 0 {
			// Load the default project's sections now, so confirm can show and
			// verify each task's month column before anything is sent.
			m.sections = nil
			m.sectionsErr = ""
			m.sectionsLoaded = false
			if m.cfg.EntityFieldGID != "" || m.cfg.ProductFieldGID != "" {
				m.step = plannerStepTagging
				m.taggingLoading = true
				m.taggingSection = 0
				m.taggingCursor = 0
				m.selectedEntities = map[string]bool{}
				m.selectedProducts = map[string]bool{}
				return *m, tea.Batch(m.fetchTagOptions(), m.fetchSections(), m.spinner.Tick)
			}
			m.step = plannerStepConfirm
			return *m, tea.Batch(m.fetchSections(), m.spinner.Tick)
		}
	}
	return *m, nil
}

func (m *PlannerModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		switch t := msg.(type) {
		case submitResult:
			m.applySubmitResult(t)
			m.submitIdx = t.idx
			if t.err != "" {
				m.step = plannerStepReview
				msg := t.err + " — tasks already in Asana are marked ✓ and skipped on resubmit"
				return *m, func() tea.Msg { return ErrorMsg{Message: msg} }
			}
			if t.done {
				m.step = plannerStepDone
				return *m, nil
			}
			return *m, m.submitNext(t.idx)
		}
		return *m, nil
	}

	// Keys are ignored while submitting: a second enter must not start a
	// second submit chain.
	if m.step != plannerStepConfirm {
		return *m, nil
	}

	switch k.String() {
	case "y", "enter":
		if !m.sectionsLoaded {
			return *m, nil
		}
		if m.sectionsErr != "" {
			msg := "cannot load project sections: " + m.sectionsErr + " — press n, then enter to retry"
			return *m, func() tea.Msg { return ErrorMsg{Message: msg} }
		}
		if probs := m.placementProblems(); len(probs) > 0 {
			return *m, func() tea.Msg {
				return ErrorMsg{Message: "create the missing month section in Asana first: " + probs[0]}
			}
		}
		m.submitIdx = 0
		m.step = plannerStepSubmitting
		return *m, tea.Batch(m.submitNext(0), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
	case "n":
		m.step = plannerStepReview
	}
	return *m, nil
}

func (m *PlannerModel) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}
	if k.String() == "enter" || k.String() == "esc" {
		m.tasks = nil
		m.clampCursors()
		m.step = plannerStepMenu
	}
	return *m, nil
}
