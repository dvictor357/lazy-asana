package tui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type hoursStep int

const (
	hoursStepInput hoursStep = iota
	hoursStepLoading
	hoursStepView
)

type hoursResult struct {
	tasks     []asana.Task
	dateStart string
	dateEnd   string
	weekNum   int
	err       error
}

type HoursModel struct {
	cfg          *config.Config
	client       asana.AsanaClient
	step         hoursStep
	mode         int
	width        int // terminal width from WindowSizeMsg
	weekInput    textinput.Model
	dateStart    textinput.Model
	dateEnd      textinput.Model
	tasks        []asana.Task
	total        float64
	fetchedStart string
	fetchedEnd   string
	weekNum      int
	cursor       int
	spinner      spinner.Model
}

func NewHoursModel(cfg *config.Config, client asana.AsanaClient) HoursModel {
	wi := textinput.New()
	wi.Placeholder = fmt.Sprintf("%d", asana.CurrentWeek())
	wi.CharLimit = 2
	wi.Width = 6
	wi.Focus()

	ds := textinput.New()
	ds.Placeholder = "2026-01-06"
	ds.CharLimit = 10
	ds.Width = 12

	de := textinput.New()
	de.Placeholder = "2026-01-12"
	de.CharLimit = 10
	de.Width = 12

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return HoursModel{
		cfg:       cfg,
		client:    client,
		step:      hoursStepInput,
		mode:      0,
		weekInput: wi,
		dateStart: ds,
		dateEnd:   de,
		spinner:   sp,
	}
}

func (m HoursModel) Init() tea.Cmd {
	return nil
}

func (m HoursModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == hoursStepView {
				m.step = hoursStepInput
				m.tasks = nil
				return m, func() tea.Msg { return ErrorMsg{Message: ""} }
			}
			return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }

		case "q":
			if m.step == hoursStepView {
				return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
			}

		case "up", "k", "ctrl+p":
			if m.step == hoursStepView && m.cursor > 0 {
				m.cursor--
				return m, nil
			}

		case "down", "j", "ctrl+n":
			if m.step == hoursStepView && m.cursor < len(m.tasks)-1 {
				m.cursor++
				return m, nil
			}

		case "tab", "ctrl+f":
			fallthrough
		case "shift+tab", "ctrl+b":
			if m.step == hoursStepInput {
				forward := msg.String() == "tab" || msg.String() == "ctrl+f"
				if m.mode == 0 {
					if forward {
						// week → dateStart
						m.mode = 1
						m.weekInput.Blur()
						m.dateStart.Focus()
						m.dateEnd.Blur()
					} else {
						// week ← dateEnd (wrap)
						m.mode = 1
						m.weekInput.Blur()
						m.dateStart.Blur()
						m.dateEnd.Focus()
					}
				} else {
					if m.dateStart.Focused() {
						if forward {
							// dateStart → dateEnd
							m.dateStart.Blur()
							m.dateEnd.Focus()
						} else {
							// dateStart → week
							m.mode = 0
							m.dateStart.Blur()
							m.dateEnd.Blur()
							m.weekInput.Focus()
						}
					} else {
						if forward {
							// dateEnd → week (wrap)
							m.mode = 0
							m.dateStart.Blur()
							m.dateEnd.Blur()
							m.weekInput.Focus()
						} else {
							// dateEnd → dateStart
							m.dateEnd.Blur()
							m.dateStart.Focus()
						}
					}
				}
				return m, nil
			}

		case "enter":
			if m.step == hoursStepInput {
				return m.handleFetch()
			}
		}

	case hoursResult:
		m.step = hoursStepView
		if msg.err != nil {
			return m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
		}
		m.tasks = msg.tasks
		m.fetchedStart = msg.dateStart
		m.fetchedEnd = msg.dateEnd
		m.weekNum = msg.weekNum
		m.cursor = 0
		m.total = 0
		for _, t := range m.tasks {
			m.total += asana.ParseActualHours(t.CustomFields, m.cfg.ActualHoursFieldGID)
		}
		return m, nil
	}

	if m.step == hoursStepInput {
		var cmd tea.Cmd
		if m.mode == 0 {
			m.weekInput, cmd = m.weekInput.Update(msg)
		} else {
			if m.dateStart.Focused() {
				m.dateStart, cmd = m.dateStart.Update(msg)
			} else {
				m.dateEnd, cmd = m.dateEnd.Update(msg)
			}
		}
		return m, cmd
	}

	return m, nil
}

func (m *HoursModel) handleFetch() (tea.Model, tea.Cmd) {
	if m.client == nil {
		return *m, func() tea.Msg {
			return ErrorMsg{Message: "asana client not configured — please bind your account first"}
		}
	}

	var dateStart, dateEnd string
	var weekNum int

	if m.mode == 0 {
		weekStr := m.weekInput.Value()
		if weekStr == "" {
			weekNum = asana.CurrentWeek()
		} else {
			fmt.Sscanf(weekStr, "%d", &weekNum)
		}
		if err := asana.ValidateWeek(asana.CurrentYear(), weekNum); err != nil {
			return *m, func() tea.Msg { return ErrorMsg{Message: fmt.Sprintf("invalid week: %s", err)} }
		}
		dateStart, dateEnd = asana.WeekToDateRange(asana.CurrentYear(), weekNum)
	} else {
		dateStart = m.dateStart.Value()
		dateEnd = m.dateEnd.Value()
		if err := asana.ValidateDate(dateStart); err != nil {
			return *m, func() tea.Msg { return ErrorMsg{Message: "invalid start date — use YYYY-MM-DD"} }
		}
		if err := asana.ValidateDate(dateEnd); err != nil {
			return *m, func() tea.Msg { return ErrorMsg{Message: "invalid end date — use YYYY-MM-DD"} }
		}
	}

	m.step = hoursStepLoading

	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID
	userGID := m.cfg.UserGID

	return *m, tea.Batch(func() tea.Msg {
		// due_on.after and due_on.before are exclusive, so shift by 1 day
		// to get an inclusive range [dateStart, dateEnd]
		filters := url.Values{
			"projects.any":  {projGID},
			"due_on.after":  {shiftDate(dateStart, -1)},
			"due_on.before": {shiftDate(dateEnd, 1)},
		}
		if userGID != "" {
			filters["assignee.any"] = []string{userGID}
		}
		tasks, err := m.client.SearchTasks(wsGID, filters)
		if err != nil {
			return hoursResult{err: err}
		}
		return hoursResult{tasks: tasks, dateStart: dateStart, dateEnd: dateEnd, weekNum: weekNum}
	}, m.spinner.Tick)
}

func (m HoursModel) View() string {
	s := WindowTitle.Render("Show Week Hours")
	s += "\n\n"

	switch m.step {
	case hoursStepInput:
		s += m.viewInput()
	case hoursStepLoading:
		s += m.spinner.View() + " Fetching tasks...\n"
	case hoursStepView:
		s += m.viewResults()
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m HoursModel) viewInput() string {
	var b strings.Builder

	weekBtn := DimStyle.Render("Week Number")
	dateBtn := DimStyle.Render("Date Range")
	if m.mode == 0 {
		weekBtn = MenuItemSelected.Render("Week Number")
	} else {
		dateBtn = MenuItemSelected.Render("Date Range")
	}
	b.WriteString(fmt.Sprintf("Mode:  %s   %s\n\n", weekBtn, dateBtn))

	if m.mode == 0 {
		b.WriteString(LabelStyle.Render("Week Number"))
		b.WriteString("\n")
		b.WriteString(m.weekInput.View())
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("tab switch mode • enter fetch • esc back"))
	} else {
		b.WriteString(LabelStyle.Render("Date Start (YYYY-MM-DD)"))
		b.WriteString("\n")
		b.WriteString(m.dateStart.View())
		b.WriteString("\n\n")
		b.WriteString(LabelStyle.Render("Date End (YYYY-MM-DD)"))
		b.WriteString("\n")
		b.WriteString(m.dateEnd.View())
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("tab switch fields/mode • enter fetch • esc back"))
	}

	if m.cfg.UserGID == "" {
		b.WriteString("\n\n" + ErrorStyle.Render("warning: no user ID saved — results will show all assignees. Re-bind to fix."))
	}

	return b.String()
}

func (m HoursModel) viewResults() string {
	var b strings.Builder

	// Header: date range
	var rangeStr string
	if m.weekNum > 0 {
		rangeStr = fmt.Sprintf("Week %d  ·  %s → %s", m.weekNum, m.fetchedStart, m.fetchedEnd)
	} else {
		rangeStr = fmt.Sprintf("%s → %s", m.fetchedStart, m.fetchedEnd)
	}
	b.WriteString(HeaderStyle.Render(rangeStr))
	b.WriteString("\n")

	if m.cfg.UserGID == "" {
		b.WriteString(ErrorStyle.Render("⚠ no assignee filter — re-bind to filter by your account"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(m.tasks) == 0 {
		b.WriteString(DimStyle.Render("No tasks found for this period."))
		b.WriteString("\n" + DimStyle.Render("Note: tasks without a due date are not returned by the Asana search API."))
		b.WriteString("\n")
	} else {
		b.WriteString(renderHoursTable(m.tasks, m.cfg.ActualHoursFieldGID, m.cursor, m.width))
		b.WriteString("\n")
		b.WriteString(HighlightStyle.Render(fmt.Sprintf("Total: %.1fh across %d tasks", m.total, len(m.tasks))))
	}

	b.WriteString("\n\n" + DimStyle.Render("↑/k ↓/j navigate • esc back • q menu"))
	return b.String()
}

// padRight pads or truncates s to exactly width visible characters.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) > width {
		return string(runes[:width-2]) + ".."
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// colHeader renders a header cell without block padding (HeaderStyle has PaddingBottom
// which would cause each cell to wrap to its own line).
var colHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))

func renderHoursTable(tasks []asana.Task, fieldGID string, cursor, termWidth int) string {
	const (
		dateW   = 12
		hoursW  = 6
		statusW = 5
		// cursor(2) + 3 separators(2 each) + padding(8)
		fixedOverhead = 2 + 3*2 + 8
	)

	// Responsive name column
	w := termWidth
	if w < 60 {
		w = 100 // default before first WindowSizeMsg
	}
	nameW := w - fixedOverhead - dateW - hoursW - statusW
	if nameW < 30 {
		nameW = 30
	}
	if nameW > 70 {
		nameW = 70
	}

	totalW := 2 + nameW + 2 + dateW + 2 + hoursW + 2 + statusW
	sep := DimStyle.Render(strings.Repeat("─", totalW)) + "\n"

	var b strings.Builder

	// Header row
	b.WriteString("  ")
	b.WriteString(colHeader.Render(padRight("Task", nameW)))
	b.WriteString("  ")
	b.WriteString(colHeader.Render(padRight("Due Date", dateW)))
	b.WriteString("  ")
	b.WriteString(colHeader.Render(padRight("Hours", hoursW)))
	b.WriteString("  ")
	b.WriteString(colHeader.Render(padRight("Status", statusW)))
	b.WriteString("\n")
	b.WriteString(sep)

	detailW := nameW + 2 + dateW + 2 + hoursW + 2 + statusW

	for i, t := range tasks {
		hours := asana.ParseActualHours(t.CustomFields, fieldGID)
		hoursStr := fmt.Sprintf("%.1f", hours)
		if hours == 0 {
			hoursStr = DimStyle.Render(padRight("0.0", hoursW))
		} else {
			hoursStr = HighlightStyle.Render(padRight(hoursStr, hoursW))
		}

		status := "Open"
		statusStyle := DimStyle
		if t.Completed {
			status = "Done"
			statusStyle = SuccessStyle
		}

		selected := i == cursor
		nameStyle := ValueStyle
		cur := "  "
		if selected {
			nameStyle = MenuItemSelected
			cur = "> "
		}

		b.WriteString(cur)
		b.WriteString(nameStyle.Render(padRight(t.Name, nameW)))
		b.WriteString("  ")
		b.WriteString(DimStyle.Render(padRight(t.DueOn, dateW)))
		b.WriteString("  ")
		b.WriteString(hoursStr)
		b.WriteString("  ")
		b.WriteString(statusStyle.Render(padRight(status, statusW)))
		b.WriteString("\n")

		// Second line: assignee · project/section memberships
		var parts []string
		if t.Assignee != nil && t.Assignee.Name != "" {
			parts = append(parts, t.Assignee.Name)
		}
		for _, mem := range t.Memberships {
			label := mem.Project.Name
			if mem.Section != nil && mem.Section.Name != "" {
				label += " / " + mem.Section.Name
			}
			parts = append(parts, label)
		}
		if len(parts) > 0 {
			b.WriteString("  ")
			b.WriteString(DimStyle.Render(padRight(strings.Join(parts, "  ·  "), detailW)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func shiftDate(s string, days int) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}
