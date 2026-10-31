package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
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
		parent := "(missing)"
		if m.subParent >= 0 && m.subParent < len(m.tasks) {
			parent = m.tasks[m.subParent].Title
		}
		s += m.viewAddTask(fmt.Sprintf("Add Subtask to: %s", parent))
	case plannerStepReview:
		s += m.viewReview()
	case plannerStepTagging:
		s += m.viewTagging()
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
	if m.cursor < 0 || m.cursor >= len(m.tasks) {
		return "(none)"
	}
	return m.tasks[m.cursor].Title
}

func (m PlannerModel) viewAddTask(header string) string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render(header))
	b.WriteString("\n\n")

	fieldLabel := func(label string, idx int) string {
		if m.fieldCursor == idx {
			return HighlightStyle.Render("> " + label)
		}
		return LabelStyle.Render("  " + label)
	}

	b.WriteString(fieldLabel("Title", 0))
	b.WriteString("\n")
	b.WriteString(m.titleInput.View())
	b.WriteString("\n\n")

	descLabel := "Description"
	if m.llmDescGenerating {
		descLabel = "Description (generating...)"
	}
	b.WriteString(fieldLabel(descLabel, 1))
	b.WriteString("\n")
	b.WriteString(m.descInput.View())
	b.WriteString("\n\n")

	b.WriteString(fieldLabel("Actual Hours", 2))
	b.WriteString("\n")
	b.WriteString(m.hoursInput.View())
	b.WriteString("\n\n")

	b.WriteString(fieldLabel("Due Date (YYYY-MM-DD)", 3))
	b.WriteString("\n")
	b.WriteString(m.dateInput.View())
	b.WriteString("\n\n")

	hint := "tab/shift+tab switch fields • enter save"
	if m.fieldCursor == 1 {
		hint = "tab/shift+tab switch fields • ctrl+s save • enter newline"
	}
	if m.llmCli != nil && m.titleInput.Value() != "" {
		hint += " • ctrl+g generate description"
	}
	hint += " • esc cancel"
	b.WriteString(DimStyle.Render(hint))

	return b.String()
}

func (m PlannerModel) viewReview() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Review & Submit"))
	b.WriteString("\n\n")

	const pageSize = 5
	total := len(m.tasks)
	start := m.reviewCursor - pageSize/2
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > total {
		end = total
		start = end - pageSize
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
		b.WriteString("\n")
	}

	totalHours := 0.0
	for _, t := range m.tasks {
		totalHours += t.Hours
		for _, sub := range t.Subtasks {
			totalHours += sub.Hours
		}
	}

	for i := start; i < end; i++ {
		t := m.tasks[i]
		hoursStr := ""
		if t.Hours > 0 {
			hoursStr = fmt.Sprintf(" (%.1fh)", t.Hours)
		}

		cursor := "  "
		style := ValueStyle
		if i == m.reviewCursor {
			cursor = "> "
			style = HighlightStyle
		}
		status := ""
		switch {
		case t.submitted():
			status = SuccessStyle.Render("  ✓ in Asana")
		case t.GID != "":
			status = ErrorStyle.Render("  ◐ partly sent — resubmit finishes it")
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%d. %s%s", cursor, i+1, t.Title, hoursStr)) + status)
		b.WriteString("\n")
		if t.Description != "" {
			b.WriteString(DimStyle.Render(fmt.Sprintf("   %s", t.Description)))
			b.WriteString("\n")
		}
		if t.DueDate != "" {
			b.WriteString(DimStyle.Render(fmt.Sprintf("   Due: %s", t.DueDate)))
			b.WriteString("\n")
		}
		if m.hasTagFields() {
			if t.HasTags {
				b.WriteString(HighlightStyle.Render("   Tags: " + m.tagLabel(t)))
			} else {
				b.WriteString(DimStyle.Render("   Tags: batch default"))
			}
			b.WriteString("\n")
		}
		for _, sub := range t.Subtasks {
			subH := ""
			if sub.Hours > 0 {
				subH = fmt.Sprintf(" (%.1fh)", sub.Hours)
			}
			b.WriteString(DimStyle.Render(fmt.Sprintf("   └ %s%s", sub.Title, subH)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if end < total {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", total-end)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(HighlightStyle.Render(fmt.Sprintf("Total: %.1f hours across %d tasks", totalHours, total)))
	b.WriteString("\n\n")
	help := "↑/k↓/j navigate • d delete task • enter submit • esc go back"
	if m.hasTagFields() {
		help = "↑/k↓/j navigate • t tag task • u use batch tags • d delete • enter submit • esc back"
	}
	b.WriteString(DimStyle.Render(help))

	return b.String()
}

func (m PlannerModel) viewTagging() string {
	var b strings.Builder
	if m.tagTarget >= 0 && m.tagTarget < len(m.tasks) {
		b.WriteString(HeaderStyle.Render("Tag Task: " + m.tasks[m.tagTarget].Title))
	} else {
		b.WriteString(HeaderStyle.Render("Default Tags"))
		b.WriteString("\n" + DimStyle.Render("Applies to every task without its own tags (set them with t in Review)."))
	}
	b.WriteString("\n\n")

	if m.taggingLoading {
		b.WriteString(m.spinner.View() + " Loading options...\n")
		return b.String()
	}

	const pageSize = 10
	renderSection := func(label string, opts []asana.EnumOption, selected map[string]bool, active bool, cursor int) {
		sectionStyle := LabelStyle
		prefix := "  "
		if active {
			sectionStyle = HighlightStyle
			prefix = "> "
		}
		b.WriteString(sectionStyle.Render(prefix + label))
		b.WriteString("\n")
		if len(opts) == 0 {
			b.WriteString(DimStyle.Render("  (no options)"))
			b.WriteString("\n")
		} else {
			start := cursor - pageSize/2
			if start < 0 {
				start = 0
			}
			end := start + pageSize
			if end > len(opts) {
				end = len(opts)
				start = end - pageSize
				if start < 0 {
					start = 0
				}
			}
			if start > 0 {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
				b.WriteString("\n")
			}
			for i := start; i < end; i++ {
				opt := opts[i]
				check := "[ ]"
				if selected[opt.GID] {
					check = "[x]"
				}
				style := ValueStyle
				if active && i == cursor {
					style = HighlightStyle
				}
				b.WriteString(style.Render(fmt.Sprintf("  %s %s", check, opt.Name)))
				b.WriteString("\n")
			}
			if end < len(opts) {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(opts)-end)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	label := func(name, fieldType string) string {
		if fieldType == "multi_enum" {
			return name + " (pick any)"
		}
		return name + " (pick one)"
	}
	renderSection(label("Company/Entity", m.cfg.EntityFieldType), m.entityOptions, m.selectedEntities, m.taggingSection == 0, m.taggingCursor)
	renderSection(label("Product/Project", m.cfg.ProductFieldType), m.productOptions, m.selectedProducts, m.taggingSection == 1, m.taggingCursor)

	b.WriteString(DimStyle.Render("↑/k↓/j navigate • space select • tab switch field • enter save • esc back to review"))
	return b.String()
}

func (m PlannerModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Confirm Submission"))
	b.WriteString("\n\n")
	if !m.sectionsLoaded {
		b.WriteString(m.spinner.View() + " Loading month sections...\n")
		return b.String()
	}
	if m.sectionsErr != "" {
		b.WriteString(ErrorStyle.Render("Cannot load project sections: " + m.sectionsErr))
		b.WriteString("\n\n" + DimStyle.Render("n back to review (enter there retries) • esc menu"))
		return b.String()
	}

	pending := 0
	for _, t := range m.tasks {
		if !t.submitted() {
			pending++
		}
	}
	b.WriteString(ValueStyle.Render(fmt.Sprintf("Submit %d task(s) to Asana?", pending)))
	if done := len(m.tasks) - pending; done > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  (%d already in Asana, skipped)", done)))
	}
	b.WriteString("\n\n")

	for i, t := range m.tasks {
		if t.submitted() {
			continue
		}
		var where string
		switch {
		case t.DueDate == "":
			where = DimStyle.Render("no due date → project's default section")
		default:
			if sec, ok := monthSection(m.sections, t.DueDate); ok {
				where = SuccessStyle.Render("→ " + sec.Name)
			} else {
				where = ErrorStyle.Render("→ month section missing")
			}
		}
		b.WriteString(fmt.Sprintf("  %d. %s  %s  %s\n", i+1, padRight(t.Title, 40), DimStyle.Render(t.DueDate), where))
		if m.hasTagFields() {
			b.WriteString(DimStyle.Render("     tags: "+m.tagLabel(t)) + "\n")
		}
	}
	b.WriteString("\n")

	if probs := m.placementProblems(); len(probs) > 0 {
		for _, p := range probs {
			b.WriteString(ErrorStyle.Render("✗ "+p) + "\n")
		}
		b.WriteString(ErrorStyle.Render("Create the missing section(s) in Asana, then retry.") + "\n\n")
		b.WriteString(DimStyle.Render("n back to review • esc menu"))
		return b.String()
	}
	b.WriteString(DimStyle.Render("y/enter confirm • n back to review • esc menu"))
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

	llmFieldLabel := func(label string, idx int) string {
		if m.llmFieldCursor == idx {
			return HighlightStyle.Render("> " + label)
		}
		return LabelStyle.Render("  " + label)
	}

	// Start date
	b.WriteString(llmFieldLabel("Start Date (YYYY-MM-DD, leave empty for today)", 0))
	b.WriteString("\n")
	b.WriteString(m.llmStartDate.View())
	b.WriteString("\n\n")

	// Days selector
	b.WriteString(llmFieldLabel("Days to plan", 1))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ← %s →", HighlightStyle.Render(fmt.Sprintf(" %d ", m.planDays))))
	b.WriteString("\n\n")

	// Context textarea
	b.WriteString(llmFieldLabel("Describe what you want to plan", 2))
	b.WriteString("\n")
	b.WriteString(m.llmContext.View())
	b.WriteString("\n\n")

	switch m.llmFieldCursor {
	case 0:
		b.WriteString(DimStyle.Render("tab/enter next field • esc cancel"))
	case 1:
		b.WriteString(DimStyle.Render("← → change days • tab/enter next field • shift+tab back • esc cancel"))
	case 2:
		b.WriteString(DimStyle.Render("enter generate • shift+tab back • ctrl+s submit existing • esc cancel"))
	}
	return b.String()
}
