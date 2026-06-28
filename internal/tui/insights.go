package tui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type insightMode int

const (
	insightModeWeek insightMode = iota
	insightModeMonth
)

type insightsResult struct {
	tasks []asana.Task
	err   error
}

type daySummary struct {
	date  string
	label string
	hours float64
	count int
}

type insightWeekSummary struct {
	start   string
	end     string
	weekNum int
	hours   float64
	count   int
}

type InsightsModel struct {
	cfg       *config.Config
	client    asana.AsanaClient
	mode      insightMode
	refDate   time.Time
	loading   bool
	firstLoad bool
	tasks     []asana.Task
	cursor    int
	total     float64
	spinner   spinner.Model
	width     int
}

func NewInsightsModel(cfg *config.Config, client asana.AsanaClient) InsightsModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return InsightsModel{
		cfg:       cfg,
		client:    client,
		mode:      insightModeWeek,
		refDate:   time.Now(),
		spinner:   sp,
		loading:   true,
		firstLoad: true,
	}
}

func (m InsightsModel) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.spinner.Tick)
}

func (m InsightsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		return m, nil
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case insightsResult:
		m.loading = false
		m.firstLoad = false
		if msg.err != nil {
			return m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
		}
		m.tasks = msg.tasks
		m.total = 0
		for _, t := range m.tasks {
			m.total += asana.ParseActualHours(t.CustomFields, m.cfg.ActualHoursFieldGID)
		}
		m.cursor = 0
		return m, func() tea.Msg { return ErrorMsg{Message: ""} }

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }

		case "left":
			return m.navigate(-1)
		case "right":
			return m.navigate(1)

		case "up", "k", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j", "ctrl+n":
			max := len(m.tasks) - 1
			if m.mode == insightModeMonth {
				max = len(m.weekSummaries()) - 1
			}
			if m.cursor < max {
				m.cursor++
			}
			return m, nil

		case "w":
			if m.mode != insightModeWeek {
				m.mode = insightModeWeek
				m.cursor = 0
				m.loading = true
				return m, tea.Batch(m.fetchCmd(), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
			}

		case "m":
			if m.mode != insightModeMonth {
				m.mode = insightModeMonth
				m.cursor = 0
				m.loading = true
				return m, tea.Batch(m.fetchCmd(), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
			}

		case "enter":
			if m.mode == insightModeMonth {
				weeks := m.weekSummaries()
				if m.cursor < len(weeks) {
					t, err := time.Parse("2006-01-02", weeks[m.cursor].start)
					if err == nil {
						m.refDate = t
					}
					m.mode = insightModeWeek
					m.cursor = 0
					m.loading = true
					return m, tea.Batch(m.fetchCmd(), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
				}
			}

		case "r":
			m.loading = true
			return m, tea.Batch(m.fetchCmd(), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
		}
	}

	return m, nil
}

func (m InsightsModel) navigate(dir int) (tea.Model, tea.Cmd) {
	if m.mode == insightModeWeek {
		m.refDate = m.refDate.AddDate(0, 0, 7*dir)
	} else {
		m.refDate = m.refDate.AddDate(0, dir, 0)
	}
	m.loading = true
	m.cursor = 0
	return m, tea.Batch(m.fetchCmd(), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
}

func (m InsightsModel) fetchCmd() tea.Cmd {
	client := m.client
	cfg := m.cfg
	mode := m.mode
	refDate := m.refDate

	return func() tea.Msg {
		if client == nil {
			return insightsResult{err: fmt.Errorf("asana client not configured")}
		}

		var start, end string
		if mode == insightModeWeek {
			start, end = insightWeekRange(refDate)
		} else {
			start, end = insightMonthRange(refDate)
		}

		filters := url.Values{
			"projects.any":  {cfg.DefaultProjectGID},
			"due_on.after":  {shiftDate(start, -1)},
			"due_on.before": {shiftDate(end, 1)},
		}
		if cfg.UserGID != "" {
			filters["assignee.any"] = []string{cfg.UserGID}
		}

		tasks, err := client.SearchTasks(cfg.DefaultWorkspaceGID, filters)
		if err != nil {
			return insightsResult{err: err}
		}
		return insightsResult{tasks: tasks}
	}
}

func insightWeekRange(t time.Time) (start, end string) {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := t.AddDate(0, 0, 1-wd)
	sunday := monday.AddDate(0, 0, 6)
	return monday.Format("2006-01-02"), sunday.Format("2006-01-02")
}

func insightMonthRange(t time.Time) (start, end string) {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

func (m InsightsModel) daySummaries() []daySummary {
	start, _ := insightWeekRange(m.refDate)
	t, _ := time.Parse("2006-01-02", start)

	days := make([]daySummary, 7)
	for i := range days {
		d := t.AddDate(0, 0, i)
		days[i] = daySummary{
			date:  d.Format("2006-01-02"),
			label: d.Format("Mon 01/02"),
		}
	}
	for _, task := range m.tasks {
		if task.DueOn == "" {
			continue
		}
		h := asana.ParseActualHours(task.CustomFields, m.cfg.ActualHoursFieldGID)
		for i := range days {
			if task.DueOn == days[i].date {
				days[i].hours += h
				days[i].count++
				break
			}
		}
	}
	return days
}

func (m InsightsModel) weekSummaries() []insightWeekSummary {
	start, end := insightMonthRange(m.refDate)
	startDate, _ := time.Parse("2006-01-02", start)
	endDate, _ := time.Parse("2006-01-02", end)

	wd := int(startDate.Weekday())
	if wd == 0 {
		wd = 7
	}
	cur := startDate.AddDate(0, 0, 1-wd)

	var weeks []insightWeekSummary
	for !cur.After(endDate) {
		weekEnd := cur.AddDate(0, 0, 6)
		_, wn := cur.ISOWeek()
		ws := insightWeekSummary{
			start:   cur.Format("2006-01-02"),
			end:     weekEnd.Format("2006-01-02"),
			weekNum: wn,
		}
		for _, task := range m.tasks {
			if task.DueOn == "" {
				continue
			}
			td, err := time.Parse("2006-01-02", task.DueOn)
			if err != nil {
				continue
			}
			if !td.Before(cur) && !td.After(weekEnd) {
				ws.hours += asana.ParseActualHours(task.CustomFields, m.cfg.ActualHoursFieldGID)
				ws.count++
			}
		}
		weeks = append(weeks, ws)
		cur = cur.AddDate(0, 0, 7)
	}
	return weeks
}

func renderInsightBar(value, max float64, width int) string {
	if max <= 0 {
		return DimStyle.Render(strings.Repeat("░", width))
	}
	filled := int(value / max * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if value >= max {
		return SuccessStyle.Render(bar)
	}
	if value > 0 {
		return HighlightStyle.Render(bar)
	}
	return DimStyle.Render(bar)
}

func (m InsightsModel) View() string {
	s := WindowTitle.Render("Insights")
	s += "\n\n"

	if m.loading {
		s += m.spinner.View() + " Loading insights...\n"
		if m.firstLoad {
			s += DimStyle.Render("Break down Asana tasks by day and week.\n")
		}
		s += DimStyle.Render("Use ← → to navigate, w/m to switch views")
		return lipgloss.NewStyle().Padding(2, 4).Render(s)
	}

	if m.mode == insightModeWeek {
		s += m.viewWeek()
	} else {
		s += m.viewMonth()
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m InsightsModel) viewWeek() string {
	var b strings.Builder

	start, end := insightWeekRange(m.refDate)
	_, wn := m.refDate.ISOWeek()

	modeBar := DimStyle.Render("[w]eek") + "  " + DimStyle.Render("[m]onth")
	header := fmt.Sprintf("Week %d  ·  %s → %s", wn, start, end)
	b.WriteString(HeaderStyle.Render(header) + "  " + modeBar)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("← prev week") + strings.Repeat(" ", 30) + DimStyle.Render("next week →"))
	b.WriteString("\n\n")

	days := m.daySummaries()
	const barW = 20
	for _, d := range days {
		bar := renderInsightBar(d.hours, 8.0, barW)
		countStr := ""
		if d.count > 0 {
			countStr = fmt.Sprintf("  (%d task", d.count)
			if d.count != 1 {
				countStr += "s"
			}
			countStr += ")"
		}
		b.WriteString(fmt.Sprintf("  %s  %s  %4.1fh%s\n",
			DimStyle.Render(d.label), bar, d.hours, DimStyle.Render(countStr)))
	}

	b.WriteString("  " + DimStyle.Render(strings.Repeat("─", 10+barW+12)))
	b.WriteString("\n")
	totalBar := renderInsightBar(m.total, 40.0, barW)
	b.WriteString(fmt.Sprintf("  %-10s  %s  %4.1fh / 40h\n",
		HighlightStyle.Render("Total"), totalBar, m.total))
	b.WriteString("\n")

	// task list
	if len(m.tasks) == 0 {
		b.WriteString(DimStyle.Render("No tasks with due dates found for this week."))
		b.WriteString("\n")
	} else {
		const pageSize = 8
		total := len(m.tasks)
		start2 := m.cursor - pageSize/2
		if start2 < 0 {
			start2 = 0
		}
		end2 := start2 + pageSize
		if end2 > total {
			end2 = total
			start2 = end2 - pageSize
			if start2 < 0 {
				start2 = 0
			}
		}

		b.WriteString(LabelStyle.Render(fmt.Sprintf("Tasks (%d):", total)))
		b.WriteString("\n")

		if start2 > 0 {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ↑ %d more\n", start2)))
		}
		for i := start2; i < end2; i++ {
			t := m.tasks[i]
			h := asana.ParseActualHours(t.CustomFields, m.cfg.ActualHoursFieldGID)
			cur := "  "
			style := ValueStyle
			if i == m.cursor {
				cur = "> "
				style = HighlightStyle
			}
			status := DimStyle.Render("open")
			if t.Completed {
				status = SuccessStyle.Render("done")
			}
			name := padRight(t.Name, 45)
			b.WriteString(fmt.Sprintf("%s%s  %s  %4.1fh  %s\n",
				cur, style.Render(name), DimStyle.Render(t.DueOn), h, status))
		}
		if end2 < total {
			b.WriteString(DimStyle.Render(fmt.Sprintf("  ↓ %d more\n", total-end2)))
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("← → navigate weeks • ↑/k↓/j scroll tasks • m month view • r refresh • esc back"))
	return b.String()
}

func (m InsightsModel) viewMonth() string {
	var b strings.Builder

	modeBar := DimStyle.Render("[w]eek") + "  " + DimStyle.Render("[m]onth")
	header := m.refDate.Format("January 2006")
	b.WriteString(HeaderStyle.Render(header) + "  " + modeBar)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("← prev month") + strings.Repeat(" ", 28) + DimStyle.Render("next month →"))
	b.WriteString("\n\n")

	weeks := m.weekSummaries()

	// find max hours for bar scaling
	maxH := 0.0
	for _, w := range weeks {
		if w.hours > maxH {
			maxH = w.hours
		}
	}
	if maxH < 40.0 {
		maxH = 40.0
	}

	const barW = 20
	for i, w := range weeks {
		cur := "  "
		style := ValueStyle
		if i == m.cursor {
			cur = "> "
			style = HighlightStyle
		}
		bar := renderInsightBar(w.hours, maxH, barW)
		countStr := fmt.Sprintf("(%d task", w.count)
		if w.count != 1 {
			countStr += "s"
		}
		countStr += ")"
		label := fmt.Sprintf("W%-2d  %s–%s", w.weekNum, w.start[5:], w.end[5:])
		b.WriteString(fmt.Sprintf("%s%s  %s  %5.1fh  %s\n",
			cur, style.Render(padRight(label, 22)), bar, w.hours, DimStyle.Render(countStr)))
	}

	// summary
	b.WriteString("\n  " + DimStyle.Render(strings.Repeat("─", 22+barW+16)))
	b.WriteString("\n")

	weekCount := 0
	bestH := 0.0
	bestW := 0
	for _, w := range weeks {
		if w.hours > 0 {
			weekCount++
			if w.hours > bestH {
				bestH = w.hours
				bestW = w.weekNum
			}
		}
	}
	avg := 0.0
	if weekCount > 0 {
		avg = m.total / float64(weekCount)
	}
	b.WriteString(fmt.Sprintf("  Total: %.1fh", m.total))
	if weekCount > 0 {
		b.WriteString(fmt.Sprintf("   Avg: %.1fh/week", avg))
		b.WriteString(fmt.Sprintf("   Best: W%d (%.1fh)", bestW, bestH))
	}
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("← → navigate months • ↑/k↓/j select week • enter drill into week • w week view • r refresh • esc back"))
	return b.String()
}
