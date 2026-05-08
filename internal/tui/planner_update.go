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
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
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
			m.validationErr = ""
			return *m, nil
		case 1:
			if len(m.tasks) == 0 {
				return *m, nil
			}
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
			m.validationErr = ""
			return *m, nil
		case 2:
			if m.llmCli == nil {
				return *m, func() tea.Msg { return ErrorMsg{Message: "no LLM key configured"} }
			}
			m.step = plannerStepLLMPlan
			m.llmContext.SetValue("")
			m.llmContext.Blur()
			m.llmStartDate.SetValue("")
			m.llmStartDate.Focus()
			m.llmErr = ""
			m.llmFieldCursor = 0
			return *m, nil
		case 3:
			if len(m.tasks) == 0 {
				return *m, nil
			}
			m.step = plannerStepReview
			return *m, nil
		}
	case "tab":
		if len(m.tasks) > 0 {
			m.cursor = (m.cursor + 1) % len(m.tasks)
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
				m.validationErr = "LLM error: " + t.err
			}
			return *m, nil
		}
		return m.updateFocusedField(msg)
	}

	switch k.String() {
	case "tab":
		m.cycleField(1)
		return *m, nil
	case "shift+tab":
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
	m.fieldCursor = ((m.fieldCursor + dir) % 4 + 4) % 4
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
	m.validationErr = ""

	title := m.titleInput.Value()
	if title == "" {
		return nil
	}

	var hours float64
	hoursStr := m.hoursInput.Value()
	if hoursStr != "" {
		if _, err := fmt.Sscanf(hoursStr, "%f", &hours); err != nil || hours == 0 {
			m.validationErr = "invalid hours: enter a number (e.g. 1.5)"
			return nil
		}
	}

	if dateStr := m.dateInput.Value(); dateStr != "" {
		if err := asana.ValidateDate(dateStr); err != nil {
			m.validationErr = "invalid date: use YYYY-MM-DD format"
			return nil
		}
	}

	task := PlanTask{
		Title:       title,
		Description: m.descInput.Value(),
		Hours:       hours,
		DueDate:     m.dateInput.Value(),
	}

	if m.step == plannerStepAddSubtask && m.subParent < len(m.tasks) {
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
	case "up", "k":
		if m.reviewCursor > 0 {
			m.reviewCursor--
		}
	case "down", "j":
		if m.reviewCursor < len(m.tasks)-1 {
			m.reviewCursor++
		}
	case "d", "delete":
		if len(m.tasks) > 0 {
			m.tasks = append(m.tasks[:m.reviewCursor], m.tasks[m.reviewCursor+1:]...)
			if m.reviewCursor >= len(m.tasks) && m.reviewCursor > 0 {
				m.reviewCursor--
			}
		}
	case "enter":
		if len(m.tasks) > 0 {
			if m.cfg.EntityFieldGID != "" || m.cfg.ProductFieldGID != "" {
				m.step = plannerStepTagging
				m.taggingLoading = true
				m.taggingSection = 0
				m.taggingCursor = 0
				m.selectedEntities = map[string]bool{}
				m.selectedProducts = map[string]bool{}
				return *m, tea.Batch(m.fetchTagOptions(), m.spinner.Tick)
			}
			m.step = plannerStepConfirm
		}
	}
	return *m, nil
}

func (m *PlannerModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		switch t := msg.(type) {
		case submitResult:
			m.submitIdx = t.idx
			if t.err != "" {
				m.submitErr = t.err
				m.step = plannerStepMenu
				return *m, nil
			}
			if t.done {
				m.step = plannerStepDone
				return *m, nil
			}
			return *m, m.submitNext(t.idx)
		}
		return *m, nil
	}

	switch k.String() {
	case "y", "enter":
		m.submitIdx = 0
		m.submitErr = ""
		m.step = plannerStepSubmitting
		return *m, tea.Batch(m.submitNext(0), m.spinner.Tick)
	case "n", "esc":
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
		m.step = plannerStepMenu
	}
	return *m, nil
}
