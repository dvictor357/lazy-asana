package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m PlannerModel) View() string {
	s := WindowTitle.Render("Plan Day/Week")
	s += "\n\n"

	switch m.step {
	case plannerStepMenu:
		s += m.viewMenu()
	case plannerStepAddTask:
		s += m.viewAddTask("Add Task")
	case plannerStepAddSubtask:
		s += m.viewAddTask(fmt.Sprintf("Add Subtask to: %s", m.tasks[m.subParent].Title))
	case plannerStepReview:
		s += m.viewReview()
	case plannerStepConfirm:
		s += m.viewConfirm()
	case plannerStepSubmitting:
		s += m.spinner.View() + fmt.Sprintf(" Submitting task %d of %d...\n", m.submitIdx+1, len(m.tasks))
	case plannerStepDone:
		s += SuccessStyle.Render("All tasks created successfully!")
		s += "\n\n" + DimStyle.Render("press enter to return")
	case plannerStepLLMPlan:
		s += m.viewLLMPlan()
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m PlannerModel) viewMenu() string {
	var b strings.Builder

	menuItems := []struct {
		label string
		desc  string
	}{
		{"Add Task", "Manually add a new task"},
		{"Add Subtask", fmt.Sprintf("Add subtask to: %s", m.selectedTaskName())},
		{"LLM Auto-Plan", "Let AI plan your day/week"},
		{"Review & Submit", fmt.Sprintf("Submit %d tasks to Asana", len(m.tasks))},
	}

	for i, item := range menuItems {
		cursor := "  "
		style := MenuItem
		if i == m.menuIdx {
			cursor = "> "
			style = MenuItemSelected
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, item.label)))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(fmt.Sprintf("    %s", item.desc)))
		b.WriteString("\n\n")
	}

	if len(m.tasks) > 0 {
		b.WriteString(HeaderStyle.Render("Tasks:"))
		b.WriteString("\n")
		for i, t := range m.tasks {
			cursor := "  "
			style := ValueStyle
			if i == m.cursor {
				cursor = "> "
				style = HighlightStyle
			}
			hoursStr := ""
			if t.Hours > 0 {
				hoursStr = fmt.Sprintf(" (%.1fh)", t.Hours)
			}
			b.WriteString(style.Render(fmt.Sprintf("%s%s%s", cursor, t.Title, hoursStr)))
			b.WriteString("\n")
			for _, sub := range t.Subtasks {
				subHours := ""
				if sub.Hours > 0 {
					subHours = fmt.Sprintf(" (%.1fh)", sub.Hours)
				}
				b.WriteString(DimStyle.Render(fmt.Sprintf("    └ %s%s", sub.Title, subHours)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("↑/k↓/j navigate • tab cycle tasks • enter select • esc back"))
	return b.String()
}

func (m PlannerModel) selectedTaskName() string {
	if len(m.tasks) == 0 {
		return "(none)"
	}
	return m.tasks[m.cursor].Title
}

func (m PlannerModel) viewAddTask(header string) string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render(header))
	b.WriteString("\n\n")

	b.WriteString(LabelStyle.Render("Title"))
	b.WriteString("\n")
	b.WriteString(m.titleInput.View())
	b.WriteString("\n\n")

	b.WriteString(LabelStyle.Render("Description"))
	b.WriteString("\n")
	b.WriteString(m.descInput.View())
	b.WriteString("\n\n")

	b.WriteString(LabelStyle.Render("Actual Hours"))
	b.WriteString("\n")
	b.WriteString(m.hoursInput.View())
	b.WriteString("\n\n")

	b.WriteString(LabelStyle.Render("Due Date (YYYY-MM-DD)"))
	b.WriteString("\n")
	b.WriteString(m.dateInput.View())
	b.WriteString("\n\n")

	b.WriteString(DimStyle.Render("enter save • tab generate description with LLM • esc cancel"))

	if m.validationErr != "" {
		b.WriteString("\n" + ErrorStyle.Render(m.validationErr))
	}

	return b.String()
}

func (m PlannerModel) viewReview() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Review & Submit"))
	b.WriteString("\n\n")

	totalHours := 0.0
	for i, t := range m.tasks {
		hoursStr := ""
		if t.Hours > 0 {
			hoursStr = fmt.Sprintf(" (%.1fh)", t.Hours)
		}
		totalHours += t.Hours

		cursor := "  "
		style := ValueStyle
		if i == m.reviewCursor {
			cursor = "> "
			style = HighlightStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%d. %s%s", cursor, i+1, t.Title, hoursStr)))
		b.WriteString("\n")
		if t.Description != "" {
			b.WriteString(DimStyle.Render(fmt.Sprintf("   %s", t.Description)))
			b.WriteString("\n")
		}
		if t.DueDate != "" {
			b.WriteString(DimStyle.Render(fmt.Sprintf("   Due: %s", t.DueDate)))
			b.WriteString("\n")
		}
		for _, sub := range t.Subtasks {
			subH := ""
			if sub.Hours > 0 {
				subH = fmt.Sprintf(" (%.1fh)", sub.Hours)
			}
			totalHours += sub.Hours
			b.WriteString(DimStyle.Render(fmt.Sprintf("   └ %s%s", sub.Title, subH)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(HighlightStyle.Render(fmt.Sprintf("Total: %.1f hours across %d tasks", totalHours, len(m.tasks))))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("↑/k↓/j navigate • d delete task • enter submit • esc go back"))

	if m.submitErr != "" {
		b.WriteString("\n" + ErrorStyle.Render(m.submitErr))
	}

	return b.String()
}

func (m PlannerModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Confirm Submission"))
	b.WriteString("\n\n")
	b.WriteString(ValueStyle.Render(fmt.Sprintf("Submit %d task(s) to Asana?", len(m.tasks))))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("y/enter confirm • n/esc cancel"))
	return b.String()
}

func (m PlannerModel) viewLLMPlan() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("LLM Auto-Plan"))
	b.WriteString("\n\n")

	if m.llmGenerating {
		b.WriteString(m.spinner.View() + " Generating plan with AI...\n")
		return b.String()
	}

	b.WriteString(LabelStyle.Render("Describe what you want to plan:"))
	b.WriteString("\n")
	b.WriteString(m.llmContext.View())
	b.WriteString("\n\n")

	if m.llmErr != "" {
		b.WriteString(ErrorStyle.Render(m.llmErr))
		b.WriteString("\n\n")
	}

	b.WriteString(DimStyle.Render("enter generate plan • esc cancel"))
	return b.String()
}
