package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

func (m PlannerModel) View() string {
	if m.step == plannerStepComposer {
		// The composer draws its own frame and uses the full width.
		return lipgloss.NewStyle().Padding(1, 2).Render(m.viewComposer())
	}

	s := WindowTitle.Render("Task Composer")
	s += "\n\n"

	switch m.step {
	case plannerStepAddTask:
		header := "Add Task"
		if m.editIdx >= 0 {
			header = "Edit Task"
		}
		s += m.viewAddTask(header)
	case plannerStepAddSubtask:
		parent := "(missing)"
		if m.subParent >= 0 && m.subParent < len(m.tasks) {
			parent = m.tasks[m.subParent].Title
		}
		s += m.viewAddTask(fmt.Sprintf("Add Subtask to: %s", parent))
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

	b.WriteString(fieldLabel("Status", formFieldDone))
	b.WriteString("\n")
	if m.formDone {
		b.WriteString(SuccessStyle.Render("  [x] Completed"))
	} else {
		b.WriteString(ValueStyle.Render("  [ ] Not completed yet"))
	}
	b.WriteString("\n\n")

	hint := "tab/shift+tab switch fields • enter save"
	if m.fieldCursor == formFieldDone {
		hint = "space toggle completed • tab/shift+tab switch fields • enter save"
	}
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

func (m PlannerModel) viewTagging() string {
	var b strings.Builder
	if m.tagTarget >= 0 && m.tagTarget < len(m.tasks) {
		b.WriteString(HeaderStyle.Render("Tag Task: " + m.tasks[m.tagTarget].Title))
	} else {
		b.WriteString(HeaderStyle.Render("Default Tags"))
		b.WriteString("\n" + DimStyle.Render("Applies to every task without its own tags (set one per task with t in the composer)."))
	}
	b.WriteString("\n\n")

	if m.taggingLoading {
		b.WriteString(m.spinner.View() + " Loading options...\n")
		return b.String()
	}

	label := func(name, fieldType string) string {
		if fieldType == "multi_enum" {
			return name + " (pick any)"
		}
		return name + " (pick one)"
	}
	selectedNames := func(opts []asana.EnumOption, selected map[string]bool) string {
		var names []string
		for _, o := range opts {
			if selected[o.GID] {
				names = append(names, o.Name)
			}
		}
		if len(names) == 0 {
			return "none"
		}
		return strings.Join(names, ", ")
	}

	fields := []struct {
		name     string
		ftype    string
		opts     []asana.EnumOption
		selected map[string]bool
	}{
		{"Company/Entity", m.cfg.EntityFieldType, m.entityOptions, m.selectedEntities},
		{"Product/Project", m.cfg.ProductFieldType, m.productOptions, m.selectedProducts},
	}

	for i, f := range fields {
		if i != m.taggingSection {
			// Inactive field: just its current choice, to keep the list short.
			b.WriteString(LabelStyle.Render("  "+label(f.name, f.ftype)) + "  " +
				ValueStyle.Render(selectedNames(f.opts, f.selected)) + "\n\n")
			continue
		}
		b.WriteString(HighlightStyle.Render("> "+label(f.name, f.ftype)) + "  " +
			DimStyle.Render("selected: ") + ValueStyle.Render(selectedNames(f.opts, f.selected)) + "\n")

		shown := filterOptions(f.opts, m.tagSearch.Value())
		if m.tagSearching || m.tagSearch.Value() != "" {
			b.WriteString("  " + m.tagSearch.View() + DimStyle.Render(fmt.Sprintf("  %d of %d", len(shown), len(f.opts))) + "\n")
		}

		const pageSize = 10
		switch {
		case len(f.opts) == 0:
			b.WriteString(DimStyle.Render("  (no options)") + "\n")
		case len(shown) == 0:
			b.WriteString(DimStyle.Render("  no match — esc clears the filter") + "\n")
		default:
			start, end := windowAround(m.taggingCursor, len(shown), pageSize)
			if start > 0 {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
			}
			for j := start; j < end; j++ {
				opt := shown[j]
				check := "[ ]"
				if f.selected[opt.GID] {
					check = "[x]"
				}
				style := ValueStyle
				if j == m.taggingCursor {
					style = HighlightStyle
				}
				b.WriteString(style.Render(fmt.Sprintf("  %s %s", check, opt.Name)) + "\n")
			}
			if end < len(shown) {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(shown)-end)) + "\n")
			}
		}
		b.WriteString("\n")
	}

	if m.tagSearching {
		b.WriteString(DimStyle.Render("type to filter • ↑↓ move • enter done filtering • esc clear filter"))
	} else {
		b.WriteString(DimStyle.Render("/ search • ↑/k↓/j move • space select • tab switch field • enter save • esc back"))
	}
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
		b.WriteString("\n\n" + DimStyle.Render("n/esc back to composer (enter there retries)"))
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
		b.WriteString(DimStyle.Render("n/esc back to composer"))
		return b.String()
	}
	b.WriteString(DimStyle.Render("y/enter confirm • n/esc back to composer"))
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
