package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ── Views ─────────────────────────────────────────────────────────────────────

func (m MigrateModel) View() string {
	s := WindowTitle.Render("Move Task to Project")
	s += "\n\n"

	switch m.step {
	case migrateStepWeek:
		s += m.viewWeek()
	case migrateStepFetching, migrateStepLoadProjects, migrateStepLoadSections:
		s += m.spinner.View() + " Loading...\n"
	case migrateStepTasks:
		s += m.viewTasks()
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

func (m MigrateModel) viewWeek() string {
	var b strings.Builder
	b.WriteString(LabelStyle.Render("Week Number") + "\n")
	b.WriteString(m.weekInput.View() + "\n\n")
	if m.cfg.UserGID == "" {
		b.WriteString(ErrorStyle.Render("warning: no user ID saved — results show all assignees. Re-bind to fix.") + "\n\n")
	}
	b.WriteString(DimStyle.Render("enter fetch tasks • esc back"))
	return b.String()
}

func (m MigrateModel) viewTasks() string {
	var b strings.Builder
	rangeStr := fmt.Sprintf("Week %d  ·  %s → %s", m.weekNum, m.fetchedStart, m.fetchedEnd)
	b.WriteString(colHeader.Render(rangeStr) + "\n\n")

	if len(m.tasks) == 0 {
		b.WriteString(DimStyle.Render("No tasks found. (Tasks without a due date are excluded.)") + "\n\n")
		b.WriteString(DimStyle.Render("esc back"))
		return b.String()
	}

	selCount := len(m.selected)
	header := fmt.Sprintf("Select Tasks (%d found)", len(m.tasks))
	if selCount > 0 {
		header += fmt.Sprintf("  —  %d selected", selCount)
	}
	b.WriteString(colHeader.Render(header) + "\n\n")

	w := m.width
	if w < 60 {
		w = 100
	}
	nameW := w - 8 - 4 - 12 - 2 // padding(8) + checkbox+cursor(4) + date(12) + gap(2)
	if nameW < 30 {
		nameW = 30
	}
	if nameW > 70 {
		nameW = 70
	}

	const pageSize = 8
	cur := m.taskCursor
	if cur >= len(m.tasks) {
		cur = len(m.tasks) - 1
	}
	start := cur - pageSize/2
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > len(m.tasks) {
		end = len(m.tasks)
		start = end - pageSize
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for i := start; i < end; i++ {
		t := m.tasks[i]

		// Cursor + checkbox
		cursor := " "
		if i == cur {
			cursor = ">"
		}
		check := "[ ]"
		checkStyle := DimStyle
		if m.selected[i] {
			check = "[✓]"
			checkStyle = SuccessStyle
		}
		nameStyle := ValueStyle
		if i == cur {
			nameStyle = MenuItemSelected
		}

		due := t.DueOn
		if due == "" {
			due = "no date"
		}
		b.WriteString(fmt.Sprintf("%s %s %s  %s\n",
			cursor,
			checkStyle.Render(check),
			nameStyle.Render(padRight(t.Name, nameW)),
			DimStyle.Render(due),
		))
		for _, mem := range t.Memberships {
			label := "· " + mem.Project.Name
			if mem.Section != nil && mem.Section.Name != "" {
				label += " / " + mem.Section.Name
			}
			b.WriteString("      " + DimStyle.Render(label) + "\n")
		}
	}
	if end < len(m.tasks) {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.tasks)-end)) + "\n")
	}

	b.WriteString("\n" + DimStyle.Render("↑/k ↓/j navigate • space toggle • a select all • enter confirm • esc back"))
	return b.String()
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
	b.WriteString("\n" + DimStyle.Render("type to search • ↑/k ↓/j navigate • enter select • esc back"))
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
	b.WriteString("\n" + DimStyle.Render("r/enter move more tasks • q/esc menu"))
	return b.String()
}
