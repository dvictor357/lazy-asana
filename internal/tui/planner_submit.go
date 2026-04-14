package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type submitResult struct {
	idx  int
	err  string
	done bool
}

func (m *PlannerModel) submitNext(idx int) tea.Cmd {
	if idx >= len(m.tasks) {
		return func() tea.Msg { return submitResult{idx: idx, done: true} }
	}
	task := m.tasks[idx]
	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID
	hoursFieldGID := m.cfg.ActualHoursFieldGID

	return func() tea.Msg {
		input := map[string]interface{}{
			"name":      task.Title,
			"notes":     task.Description,
			"projects":  []string{projGID},
			"workspace": wsGID,
		}
		if task.DueDate != "" {
			input["due_on"] = task.DueDate
		}
		if hoursFieldGID != "" && task.Hours > 0 {
			input["custom_fields"] = map[string]interface{}{
				hoursFieldGID: task.Hours,
			}
		}

		created, err := m.asanaCli.CreateTask(input)
		if err != nil {
			return submitResult{idx: idx, err: err.Error()}
		}

		for _, sub := range task.Subtasks {
			subInput := map[string]interface{}{
				"name":  sub.Title,
				"notes": sub.Description,
			}
			if sub.DueDate != "" {
				subInput["due_on"] = sub.DueDate
			}
			if hoursFieldGID != "" && sub.Hours > 0 {
				subInput["custom_fields"] = map[string]interface{}{
					hoursFieldGID: sub.Hours,
				}
			}
			if _, err := m.asanaCli.CreateSubtask(created.GID, subInput); err != nil {
				return submitResult{idx: idx, err: fmt.Sprintf("subtask %q: %s", sub.Title, err.Error())}
			}
		}

		return submitResult{idx: idx + 1}
	}
}

func (m *PlannerModel) handleLLMSubmit() tea.Cmd {
	if len(m.tasks) == 0 {
		return nil
	}
	m.step = plannerStepReview
	return nil
}
