package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

// composerRowSelected highlights the selected row without MenuItemSelected's
// left padding, which would shift the columns.
var composerRowSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)

var composerFrame = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#7D56F4")).
	Padding(0, 1)

// composerWidth is the frame width passed to lipgloss (it includes the
// frame's horizontal padding); content gets composerWidth()-2 columns.
func (m PlannerModel) composerWidth() int {
	w := m.width
	switch {
	case w == 0:
		w = 110 // before the first WindowSizeMsg
	case w < 60:
		w = 60
	}
	w -= 4 + 2 // outer padding + frame border
	if w > 140 {
		w = 140
	}
	return w
}

func (m PlannerModel) viewComposer() string {
	w := m.composerWidth()
	var b strings.Builder

	b.WriteString(WindowTitle.Render("Task Composer"))
	b.WriteString("\n")

	if len(m.tasks) == 0 {
		empty := DimStyle.Render("No tasks yet.\n\n") +
			ValueStyle.Render("a") + DimStyle.Render(" add a task") +
			m.llmHint() + "\n"
		b.WriteString(composerFrame.Width(w).Render(empty))
		b.WriteString("\n" + DimStyle.Render("a add • esc back"))
		return b.String()
	}

	inner := w - 2
	table := m.composerTable(inner)
	days := m.dayBars(inner)
	detail := m.composerDetail(inner)
	b.WriteString(composerFrame.Width(w).Render(table + "\n" + DimStyle.Render(strings.Repeat("─", inner)) + "\n" + days))
	b.WriteString("\n")
	b.WriteString(composerFrame.Width(w).Render(detail))
	b.WriteString("\n")

	keys := "↑↓ move • a add • e edit • s subtask • d delete"
	if m.hasTagFields() {
		keys += " • t tags • u batch tags"
	}
	if m.llmCli != nil {
		keys += " • L LLM plan"
	}
	keys += " • enter submit • esc back"
	b.WriteString(DimStyle.Width(w + 2).Render(keys))
	return b.String()
}

func (m PlannerModel) llmHint() string {
	if m.llmCli == nil {
		return ""
	}
	return DimStyle.Render(" • ") + ValueStyle.Render("L") + DimStyle.Render(" let the LLM plan your week")
}

// tableCols holds the composer column widths; the task column takes the rest.
type tableCols struct {
	due, task, hours, entity, product, section, status int
	gap                                                string
}

const colCursor = 2

// composerCols fits the columns into w, narrowing tags and section on small
// terminals so rows never wrap.
func composerCols(w int) tableCols {
	c := tableCols{due: 7, hours: 5, entity: 12, product: 14, section: 10, status: 2, gap: "  "}
	fixed := func() int {
		return colCursor + c.due + c.hours + c.entity + c.product + c.section + c.status + 6*len(c.gap)
	}
	if w-fixed() < 24 {
		c.entity, c.product, c.section, c.gap = 8, 10, 9, " "
	}
	c.task = w - fixed()
	if c.task < 8 {
		c.task = 8
	}
	return c
}

func (m PlannerModel) composerTable(w int) string {
	c := composerCols(w)
	taskW := c.task
	colDue, colHours, colEntity, colProduct, colSection, colStatus, gap := c.due, c.hours, c.entity, c.product, c.section, c.status, c.gap
	cell := func(s string, n int) string { return padRight(s, n) }

	var b strings.Builder
	header := strings.Repeat(" ", colCursor) + strings.Join([]string{
		cell("Due", colDue), cell("Task", taskW), cell("Hrs", colHours),
		cell("Entity", colEntity), cell("Product", colProduct), cell("Section", colSection), cell("St", colStatus),
	}, gap)
	b.WriteString(colHeader.Render(header))
	b.WriteString("\n")

	// Keep the selected row visible: show a window of rows around it.
	const maxRows = 12
	start, end := windowAround(m.reviewCursor, len(m.tasks), maxRows)
	if start > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n")
	}
	for i := start; i < end; i++ {
		t := m.tasks[i]
		ent, prod := m.tagNames(t)
		tagStyle := ValueStyle
		if !t.HasTags {
			tagStyle = DimStyle // batch default
		}
		cursor, rowStyle := "  ", ValueStyle
		if i == m.reviewCursor {
			cursor, rowStyle = "> ", composerRowSelected
		}
		status, statusStyle := "·", DimStyle
		switch {
		case t.submitted():
			status, statusStyle = "✓", SuccessStyle
		case t.GID != "":
			status, statusStyle = "◐", ErrorStyle
		}
		section, sectionStyle := m.sectionLabel(t)

		b.WriteString(rowStyle.Render(cursor + cell(shortDate(t.DueDate), colDue) + gap + cell(t.Title, taskW) + gap + cell(hoursCell(t.Hours), colHours)))
		b.WriteString(gap + tagStyle.Render(cell(ent, colEntity)))
		b.WriteString(gap + tagStyle.Render(cell(prod, colProduct)))
		b.WriteString(gap + sectionStyle.Render(cell(section, colSection)))
		b.WriteString(gap + statusStyle.Render(cell(status, colStatus)))
		b.WriteString("\n")
		for _, sub := range t.Subtasks {
			line := strings.Repeat(" ", colCursor+colDue+len(gap)) + cell("└ "+sub.Title, taskW) + gap + cell(hoursCell(sub.Hours), colHours)
			b.WriteString(DimStyle.Render(line) + "\n")
		}
	}
	if end < len(m.tasks) {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.tasks)-end)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// dayBars sums planned hours per due date (subtasks included) against an
// 8h day, several days per line.
func (m PlannerModel) dayBars(w int) string {
	perDay := map[string]float64{}
	total, undated := 0.0, 0.0
	for _, t := range m.tasks {
		h := t.Hours
		for _, s := range t.Subtasks {
			h += s.Hours
		}
		total += h
		if t.DueDate == "" {
			undated += h
			continue
		}
		perDay[t.DueDate] += h
	}
	dates := make([]string, 0, len(perDay))
	for d := range perDay {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	const barW = 10
	const itemW = 10 + 1 + 6 + 1 + barW + 3 // "Mon 19 Oct" h bar gap
	perLine := w / itemW
	if perLine < 1 {
		perLine = 1
	}
	var lines []string
	var cur []string
	for _, d := range dates {
		label := d
		if t, err := time.Parse("2006-01-02", d); err == nil {
			label = t.Format("Mon 02 Jan")
		}
		h := perDay[d]
		style := ValueStyle
		if h > 8 {
			style = ErrorStyle
		}
		cur = append(cur, DimStyle.Render(label)+" "+style.Render(fmt.Sprintf("%5.1fh", h))+" "+renderInsightBar(h, 8, barW))
		if len(cur) == perLine {
			lines = append(lines, strings.Join(cur, "   "))
			cur = nil
		}
	}
	if len(cur) > 0 {
		lines = append(lines, strings.Join(cur, "   "))
	}
	summary := HighlightStyle.Render(fmt.Sprintf("Total %.1fh across %d tasks", total, len(m.tasks)))
	if undated > 0 {
		summary += DimStyle.Render(fmt.Sprintf("  (%.1fh without due date)", undated))
	}
	lines = append(lines, summary)
	return strings.Join(lines, "\n")
}

func (m PlannerModel) composerDetail(w int) string {
	if m.reviewCursor < 0 || m.reviewCursor >= len(m.tasks) {
		return ""
	}
	t := m.tasks[m.reviewCursor]
	label := func(s string) string { return LabelStyle.Render(padRight(s, 8)) }
	valW := w - 8
	if valW < 10 {
		valW = 10
	}

	var b strings.Builder
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, label("Title"), ValueStyle.Width(valW).Render(t.Title)) + "\n")
	desc := strings.Join(strings.Fields(t.Description), " ")
	if desc == "" {
		desc = DimStyle.Render("—")
	} else if len([]rune(desc)) > valW*2 {
		desc = string([]rune(desc)[:valW*2-1]) + "…"
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, label("Desc"), lipgloss.NewStyle().Width(valW).Render(desc)) + "\n")

	due := t.DueDate
	if due == "" {
		due = "—"
	}
	section, sectionStyle := m.sectionLabel(t)
	b.WriteString(label("Hours") + ValueStyle.Render(padRight(hoursCell(t.Hours), 8)) +
		LabelStyle.Render("Due ") + ValueStyle.Render(due) + "  " + sectionStyle.Render("→ "+section) + "\n")

	tags := m.tagLabel(t)
	if m.hasTagFields() {
		if !t.HasTags {
			tags += DimStyle.Render("  (batch default — t to set for this task)")
		}
		b.WriteString(label("Tags") + ValueStyle.Render(tags) + "\n")
	}
	if n := len(t.Subtasks); n > 0 {
		b.WriteString(label("Subtasks") + ValueStyle.Render(fmt.Sprintf("%d", n)) + "\n")
	}
	switch {
	case t.submitted():
		b.WriteString(label("Status") + SuccessStyle.Render("✓ in Asana"))
	case t.GID != "":
		b.WriteString(label("Status") + ErrorStyle.Render("◐ partly sent — submit again to finish"))
	default:
		b.WriteString(label("Status") + DimStyle.Render("· not sent"))
	}
	return b.String()
}

// tagNames returns the entity and product option names that apply to a task.
func (m PlannerModel) tagNames(t PlanTask) (entity, product string) {
	ents, prods := m.tagsFor(t)
	names := func(sel map[string]bool, opts []asana.EnumOption) string {
		var out []string
		for _, o := range opts {
			if sel[o.GID] {
				out = append(out, o.Name)
			}
		}
		if len(out) == 0 {
			return "—"
		}
		return strings.Join(out, ", ")
	}
	return names(ents, m.entityOptions), names(prods, m.productOptions)
}

// sectionLabel names the month column a task lands in. Before sections are
// loaded it shows the expected month; after, it flags a missing section.
func (m PlannerModel) sectionLabel(t PlanTask) (string, lipgloss.Style) {
	if t.DueDate == "" {
		return "default", DimStyle
	}
	d, err := time.Parse("2006-01-02", t.DueDate)
	if err != nil {
		return "?", ErrorStyle
	}
	if m.sectionsLoaded && m.sectionsErr == "" {
		if sec, ok := monthSection(m.sections, t.DueDate); ok {
			return sec.Name, SuccessStyle
		}
		return d.Format("January") + "?", ErrorStyle
	}
	return d.Format("January"), DimStyle
}

func shortDate(s string) string {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("02 Jan")
	}
	if s == "" {
		return "—"
	}
	return s
}

func hoursCell(h float64) string {
	if h <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f", h)
}

// windowAround returns [start, end) of at most size rows that contains cur.
func windowAround(cur, n, size int) (int, int) {
	start := cur - size/2
	if start < 0 {
		start = 0
	}
	end := start + size
	if end > n {
		end = n
		start = end - size
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
