package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/llm"
)

type llmPlanResult struct {
	tasks []llm.PlannedTask
	err   string
}

func (m *PlannerModel) updateLLMPlan(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.llmGenerating {
		switch t := msg.(type) {
		case llmPlanResult:
			m.llmGenerating = false
			if t.err != "" {
				m.llmErr = t.err
				return m, nil
			}
			for _, pt := range t.tasks {
				m.tasks = append(m.tasks, PlanTask{
					Title:       pt.Title,
					Description: pt.Description,
					Hours:       pt.Hours,
					DueDate:     pt.DueDate,
					Subtasks:    convertSubtasks(pt.Subtasks),
				})
			}
			m.step = plannerStepMenu
			return m, nil
		}
		return m, nil
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.llmContext, cmd = m.llmContext.Update(msg)
		return m, cmd
	}

	switch k.String() {
	case "enter":
		if m.llmContext.Value() == "" {
			return m, nil
		}
		m.llmGenerating = true
		context := m.llmContext.Value()
		return m, tea.Batch(func() tea.Msg {
			resp, err := m.llmCli.GenerateWeekPlan(context)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}
			tasks, err := llm.ParsePlan(resp)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}
			return llmPlanResult{tasks: tasks}
		}, m.spinner.Tick)
	case "ctrl+s":
		return m, m.handleLLMSubmit()
	}

	var cmd tea.Cmd
	m.llmContext, cmd = m.llmContext.Update(msg)
	return m, cmd
}

func convertSubtasks(subs []struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Hours       float64 `json:"hours"`
}) []PlanTask {
	result := make([]PlanTask, len(subs))
	for i, s := range subs {
		result[i] = PlanTask{Title: s.Title, Description: s.Description, Hours: s.Hours}
	}
	return result
}
