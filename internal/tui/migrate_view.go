package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ── Views ─────────────────────────────────────────────────────────────────────

func (m MigrateModel) View() string {
	s := WindowTitle.Render("Move Tasks")
	s += "\n\n"

	switch m.step {
	case migrateStepLoadProjects, migrateStepLoadSections:
		s += m.spinner.View() + " Loading...\n"
	case migrateStepPickTarget:
		s += m.viewPickTarget()
	case migrateStepPickSection:
		s += m.viewPickSection()
	case migrateStepPickSource:
		s += m.viewPickSource()
	case migrateStepMoving:
		s += m.viewMoving()
	case migrateStepDone:
		s += m.viewDone()
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m MigrateModel) viewPickTarget() string {
	var b strings.Builder
	if len(m.selectedTasks) == 1 {
		b.WriteString(HighlightStyle.Render("Task: "+m.selectedTask.Name) + "\n\n")
	} else {
		b.WriteString(HighlightStyle.Render(fmt.Sprintf("%d tasks selected", len(m.selectedTasks))) + "\n\n")
	}
	b.WriteString(colHeader.Render("Move TO — Select Target Project") + "\n\n")

	filtered := m.filteredProjects()
	matchInfo := fmt.Sprintf("%d total", len(m.projects))
	if m.projSearch.Value() != "" {
		matchInfo = fmt.Sprintf("%d match / %d total", len(filtered), len(m.projects))
	}
	b.WriteString(LabelStyle.Render(fmt.Sprintf("Search (%s): ", matchInfo)))
	b.WriteString(m.projSearch.View() + "\n\n")

	if len(filtered) == 0 {
		b.WriteString(DimStyle.Render("  no projects match") + "\n")
	} else {
		cur := m.projCursor
		if cur >= len(filtered) {
			cur = len(filtered) - 1
		}
		const pageSize = 10
		start := cur - pageSize/2
		if start < 0 {
			start = 0
		}
		end := start + pageSize
		if end > len(filtered) {
			end = len(filtered)
			start = end - pageSize
			if start < 0 {
				start = 0
			}
		}
		if start > 0 {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
		}
		for i := start; i < end; i++ {
			p := filtered[i]
			pfx, style := "  ", MenuItem
			if i == cur {
				pfx, style = "> ", MenuItemSelected
			}
			b.WriteString(style.Render(pfx+p.Name) + "\n")
		}
		if end < len(filtered) {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(filtered)-end)) + "\n")
		}
	}
	b.WriteString("\n" + DimStyle.Render("type to search • ↑↓ navigate • enter select • esc back"))
	return b.String()
}

func (m MigrateModel) viewPickSection() string {
	var b strings.Builder
	if len(m.selectedTasks) == 1 {
		b.WriteString(HighlightStyle.Render("Task: "+m.selectedTask.Name) + "\n")
	} else {
		b.WriteString(HighlightStyle.Render(fmt.Sprintf("%d tasks", len(m.selectedTasks))) + "\n")
	}
	b.WriteString(SuccessStyle.Render("→ Project: "+m.targetProj.Name) + "\n\n")
	b.WriteString(colHeader.Render("Select Section (month)") + "\n\n")

	autoMonth := time.Now().Format("January")
	if m.selectedTask.DueOn != "" {
		if t, err := time.Parse("2006-01-02", m.selectedTask.DueOn); err == nil {
			autoMonth = t.Format("January")
		}
	}
	for i, s := range m.sections {
		pfx, style := "  ", MenuItem
		if i == m.sectionCursor {
			pfx, style = "> ", MenuItemSelected
		}
		label := s.Name
		if strings.EqualFold(s.Name, autoMonth) {
			label += DimStyle.Render("  ← task due date")
		}
		b.WriteString(style.Render(pfx+label) + "\n")
	}
	pfx, style := "  ", DimStyle
	if m.sectionCursor == len(m.sections) {
		pfx, style = "> ", MenuItemSelected
	}
	b.WriteString(style.Render(pfx+"No section (default)") + "\n")
	b.WriteString("\n" + DimStyle.Render("↑/k ↓/j navigate • enter confirm • esc back"))
	return b.String()
}

func (m MigrateModel) viewPickSource() string {
	var b strings.Builder
	if len(m.selectedTasks) == 1 {
		b.WriteString(HighlightStyle.Render("Task: "+m.selectedTask.Name) + "\n")
	} else {
		b.WriteString(HighlightStyle.Render(fmt.Sprintf("%d tasks", len(m.selectedTasks))) + "\n")
	}
	b.WriteString(SuccessStyle.Render("→ Project: "+m.targetProj.Name) + "\n")
	if m.sectionCursor < len(m.sections) {
		b.WriteString(SuccessStyle.Render("→ Section: "+m.sections[m.sectionCursor].Name) + "\n")
	}
	b.WriteString("\n" + colHeader.Render("Remove FROM — Select Source Project (optional)") + "\n\n")

	if len(m.sourceOpts) == 0 {
		b.WriteString(DimStyle.Render("No other projects found across selected tasks.") + "\n\n")
		b.WriteString(MenuItemSelected.Render("> Skip (just add to target)") + "\n")
	} else {
		for i, p := range m.sourceOpts {
			pfx, style := "  ", MenuItem
			if i == m.sourceCursor {
				pfx, style = "> ", MenuItemSelected
			}
			b.WriteString(style.Render(pfx+p.Name) + "\n")
		}
		pfx, style := "  ", DimStyle
		if m.sourceCursor == len(m.sourceOpts) {
			pfx, style = "> ", MenuItemSelected
		}
		b.WriteString(style.Render(pfx+"Skip (just add, don't remove from any project)") + "\n")
	}
	b.WriteString("\n" + DimStyle.Render("↑/k ↓/j navigate • enter confirm • esc back"))
	return b.String()
}

func (m MigrateModel) viewMoving() string {
	total := len(m.selectedTasks)
	done := m.moveMoved + m.moveFailed
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s Moving tasks... %d / %d\n\n", m.spinner.View(), done, total))
	b.WriteString(SuccessStyle.Render(fmt.Sprintf("Moved:  %d", m.moveMoved)) + "\n")
	if m.moveFailed > 0 {
		b.WriteString(ErrorStyle.Render(fmt.Sprintf("Failed: %d", m.moveFailed)) + "\n")
		for _, e := range m.moveErrors {
			b.WriteString(ErrorStyle.Render("  "+e) + "\n")
		}
	}
	return b.String()
}

func (m MigrateModel) viewDone() string {
	var b strings.Builder
	b.WriteString(SuccessStyle.Render(fmt.Sprintf("Done! Moved %d task(s).", m.moveMoved)) + "\n")
	b.WriteString(DimStyle.Render("→ Project: "+m.targetProj.Name) + "\n")
	if m.sectionCursor < len(m.sections) {
		b.WriteString(DimStyle.Render("→ Section: "+m.sections[m.sectionCursor].Name) + "\n")
	}
	if m.sourceCursor < len(m.sourceOpts) {
		b.WriteString(DimStyle.Render("Removed from: "+m.sourceOpts[m.sourceCursor].Name) + "\n")
	}
	if m.moveFailed > 0 {
		b.WriteString("\n" + ErrorStyle.Render(fmt.Sprintf("%d task(s) had errors:", m.moveFailed)) + "\n")
		for _, e := range m.moveErrors {
			b.WriteString(DimStyle.Render("  "+e) + "\n")
		}
	}
	b.WriteString("\n" + DimStyle.Render("enter/esc back to Insights"))
	return b.String()
}
