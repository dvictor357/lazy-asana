package tui

import (
	"fmt"
	"net/url"
	"strings"

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
	tasks []asana.Task
	err   error
}

type HoursModel struct {
	cfg    *config.Config
	client *asana.Client
	step   hoursStep
	mode   int
	inputs []textinput.Model
	cursor int
	tasks  []asana.Task
	err    string
	total  float64
}

func NewHoursModel(cfg *config.Config, client *asana.Client) HoursModel {
	weekInput := textinput.New()
	weekInput.Placeholder = fmt.Sprintf("%d", asana.CurrentWeek())
	weekInput.CharLimit = 2
	weekInput.Width = 10
	weekInput.Focus()

	dateStart := textinput.New()
	dateStart.Placeholder = "2026-01-06"
	dateStart.CharLimit = 10
	dateStart.Width = 12

	dateEnd := textinput.New()
	dateEnd.Placeholder = "2026-01-12"
	dateEnd.CharLimit = 10
	dateEnd.Width = 12

	return HoursModel{
		cfg:    cfg,
		client: client,
		step:   hoursStepInput,
		mode:   0,
		inputs: []textinput.Model{weekInput, dateStart, dateEnd},
	}
}

func (m HoursModel) Init() tea.Cmd {
	return nil
}

func (m HoursModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == hoursStepView {
				m.step = hoursStepInput
				m.tasks = nil
				return m, nil
			}
			return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
		case "tab", "shift+tab":
			if m.step == hoursStepInput {
				if msg.String() == "tab" {
					m.cursor = (m.cursor + 1) % len(m.inputs)
				} else {
					m.cursor = (m.cursor - 1 + len(m.inputs)) % len(m.inputs)
				}
				for i := range m.inputs {
					if i == m.cursor {
						m.inputs[i].Focus()
					} else {
						m.inputs[i].Blur()
					}
				}
			}
			return m, nil
		case "1":
			if m.step == hoursStepInput && m.cursor != 0 {
				m.mode = 0
				return m, nil
			}
		case "2":
			if m.step == hoursStepInput && m.cursor != 0 {
				m.mode = 1
				return m, nil
			}
		case "enter":
			if m.step == hoursStepInput {
				return m.handleFetch()
			}
		case "q":
			if m.step == hoursStepView {
				return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
			}
		}
	case hoursResult:
		m.step = hoursStepView
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.tasks = msg.tasks
		m.total = 0
		for _, t := range m.tasks {
			m.total += asana.ParseActualHours(t.CustomFields, m.cfg.ActualHoursFieldGID)
		}
		return m, nil
	}

	if m.step == hoursStepInput {
		var cmd tea.Cmd
		m.inputs[m.cursor], cmd = m.inputs[m.cursor].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *HoursModel) handleFetch() (tea.Model, tea.Cmd) {
	var dateStart, dateEnd string

	if m.mode == 0 {
		weekStr := m.inputs[0].Value()
		if weekStr == "" {
			weekStr = fmt.Sprintf("%d", asana.CurrentWeek())
		}
		var week int
		fmt.Sscanf(weekStr, "%d", &week)
		dateStart, dateEnd = asana.WeekToDateRange(asana.CurrentYear(), week)
	} else {
		dateStart = m.inputs[1].Value()
		dateEnd = m.inputs[2].Value()
		if err := asana.ValidateDate(dateStart); err != nil {
			m.err = "invalid start date"
			return m, nil
		}
		if err := asana.ValidateDate(dateEnd); err != nil {
			m.err = "invalid end date"
			return m, nil
		}
	}

	m.step = hoursStepLoading
	m.err = ""

	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID

	return m, func() tea.Msg {
		filters := url.Values{
			"projects.any":  {projGID},
			"due_on.after":  {dateStart},
			"due_on.before": {dateEnd},
		}
		tasks, err := m.client.SearchTasks(wsGID, filters)
		if err != nil {
			return hoursResult{err: err}
		}
		return hoursResult{tasks: tasks}
	}
}

func (m HoursModel) View() string {
	s := WindowTitle.Render("Show Week Hours")
	s += "\n\n"

	if m.step == hoursStepInput {
		modeStyle := DimStyle
		activeStyle := HighlightStyle
		ws := modeStyle.Render("[1] Week Number")
		ds := modeStyle.Render("[2] Date Range")
		if m.mode == 0 {
			ws = activeStyle.Render("[1] Week Number")
		} else {
			ds = activeStyle.Render("[2] Date Range")
		}
		s += fmt.Sprintf("Mode: %s  %s\n\n", ws, ds)

		s += LabelStyle.Render("Week Number")
		s += "\n"
		s += m.inputs[0].View()
		s += "\n\n"
		s += LabelStyle.Render("Date Start")
		s += "\n"
		s += m.inputs[1].View()
		s += "\n\n"
		s += LabelStyle.Render("Date End")
		s += "\n"
		s += m.inputs[2].View()
		s += "\n\n"
		s += DimStyle.Render("tab switch fields • 1/2 switch mode • enter fetch • esc back")

		if m.err != "" {
			s += "\n" + ErrorStyle.Render(m.err)
		}
	}

	if m.step == hoursStepLoading {
		s += "Loading tasks...\n"
	}

	if m.step == hoursStepView {
		if m.err != "" {
			s += ErrorStyle.Render(m.err) + "\n\n"
		}

		if len(m.tasks) == 0 {
			s += "No tasks found for this period.\n"
		} else {
			tableStr := renderHoursTable(m.tasks, m.cfg.ActualHoursFieldGID)
			s += tableStr
			s += "\n\n"
			s += HighlightStyle.Render(fmt.Sprintf("Total Actual Hours: %.1f", m.total))
		}
		s += "\n\n" + DimStyle.Render("esc back • q return to menu")
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func renderHoursTable(tasks []asana.Task, fieldGID string) string {
	var b strings.Builder

	nameWidth := 40
	dateWidth := 12
	hoursWidth := 8
	statusWidth := 10

	fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %-*s\n",
		nameWidth, HeaderStyle.Render("Task"),
		dateWidth, HeaderStyle.Render("Due Date"),
		hoursWidth, HeaderStyle.Render("Hours"),
		statusWidth, HeaderStyle.Render("Status"),
	)

	sep := strings.Repeat("─", nameWidth+dateWidth+hoursWidth+statusWidth+6)
	b.WriteString(DimStyle.Render(sep) + "\n")

	for _, t := range tasks {
		hours := asana.ParseActualHours(t.CustomFields, fieldGID)
		status := "Open"
		statusStyle := ValueStyle
		if t.Completed {
			status = "Done"
			statusStyle = SuccessStyle
		}

		name := t.Name
		if len(name) > nameWidth {
			name = name[:nameWidth-2] + ".."
		}

		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %-*s\n",
			nameWidth, ValueStyle.Render(name),
			dateWidth, DimStyle.Render(t.DueOn),
			hoursWidth, HighlightStyle.Render(fmt.Sprintf("%.1f", hours)),
			statusWidth, statusStyle.Render(status),
		)
	}

	return b.String()
}
