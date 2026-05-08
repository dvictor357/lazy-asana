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
	if m.asanaCli == nil {
		return func() tea.Msg { return submitResult{idx: idx, err: "asana client not configured — please bind your account first"} }
	}
	if idx >= len(m.tasks) {
		return func() tea.Msg { return submitResult{idx: idx, done: true} }
	}
	task := m.tasks[idx]
	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID
	hoursFieldGID := m.cfg.ActualHoursFieldGID
	userGID := m.cfg.UserGID
	entityFieldGID := m.cfg.EntityFieldGID
	entityFieldType := m.cfg.EntityFieldType
	productFieldGID := m.cfg.ProductFieldGID
	productFieldType := m.cfg.ProductFieldType
	selectedEntities := m.selectedEntities
	selectedProducts := m.selectedProducts

	return func() tea.Msg {
		input := map[string]interface{}{
			"name":      task.Title,
			"notes":     task.Description,
			"projects":  []string{projGID},
			"workspace": wsGID,
			"completed": true,
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
		applyEnumField(customFields, entityFieldGID, entityFieldType, selectedEntities)
		applyEnumField(customFields, productFieldGID, productFieldType, selectedProducts)
		if len(customFields) > 0 {
			input["custom_fields"] = customFields
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
	m.step = plannerStepReview
	return nil
}
