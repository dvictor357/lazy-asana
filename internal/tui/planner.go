package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
	"github.com/dvictor357/lazy-asana/internal/llm"
)

type plannerStep int

const (
	plannerStepMenu plannerStep = iota
	plannerStepAddTask
	plannerStepAddSubtask
	plannerStepReview
	plannerStepSubmitting
	plannerStepDone
	plannerStepLLMPlan
)

type PlanTask struct {
	Title       string
	Description string
	Hours       float64
	DueDate     string
	Subtasks    []PlanTask
}

type PlannerModel struct {
	cfg       *config.Config
	asanaCli  *asana.Client
	llmCli    *llm.Client
	step      plannerStep
	menuIdx   int
	tasks     []PlanTask
	cursor    int
	subParent int

	titleInput textinput.Model
	descInput  textarea.Model
	hoursInput textinput.Model
	dateInput  textinput.Model

	llmGenerating bool
	llmResult     string
	llmErr        string
	llmContext    textarea.Model

	submitIdx int
	submitErr string
}

func NewPlannerModel(cfg *config.Config, asanaCli *asana.Client, llmCli *llm.Client) PlannerModel {
	ti := textinput.New()
	ti.Placeholder = "Task title"
	ti.CharLimit = 200
	ti.Width = 50

	di := textarea.New()
	di.Placeholder = "Task description..."
	di.SetWidth(50)
	di.SetHeight(4)

	hi := textinput.New()
	hi.Placeholder = "1.0"
	hi.CharLimit = 10
	hi.Width = 10

	dt := textinput.New()
	dt.Placeholder = "2026-04-15"
	dt.CharLimit = 10
	dt.Width = 12

	lc := textarea.New()
	lc.Placeholder = "Describe what you want to plan (e.g., 'Plan my week: API integration, code review, deploy to staging')"
	lc.SetWidth(60)
	lc.SetHeight(4)

	return PlannerModel{
		cfg:        cfg,
		asanaCli:   asanaCli,
		llmCli:     llmCli,
		step:       plannerStepMenu,
		titleInput: ti,
		descInput:  di,
		hoursInput: hi,
		dateInput:  dt,
		llmContext: lc,
	}
}

func (m PlannerModel) Init() tea.Cmd {
	return nil
}

func (m PlannerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == plannerStepMenu {
				return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
			}
			m.step = plannerStepMenu
			return m, nil
		}
	}

	switch m.step {
	case plannerStepMenu:
		return m.updateMenu(msg)
	case plannerStepAddTask, plannerStepAddSubtask:
		return m.updateAddTask(msg)
	case plannerStepReview:
		return m.updateReview(msg)
	case plannerStepLLMPlan:
		return m.updateLLMPlan(msg)
	case plannerStepDone:
		return m.updateDone(msg)
	}

	return m, nil
}

func (m *PlannerModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch k.String() {
	case "up", "k":
		if m.menuIdx > 0 {
			m.menuIdx--
		}
	case "down", "j":
		if m.menuIdx < 3 {
			m.menuIdx++
		}
	case "enter":
		switch m.menuIdx {
		case 0:
			m.step = plannerStepAddTask
			m.titleInput.SetValue("")
			m.descInput.SetValue("")
			m.hoursInput.SetValue("")
			m.dateInput.SetValue("")
			m.titleInput.Focus()
			return m, nil
		case 1:
			if len(m.tasks) == 0 {
				return m, nil
			}
			m.step = plannerStepAddSubtask
			m.subParent = m.cursor
			m.titleInput.SetValue("")
			m.descInput.SetValue("")
			m.hoursInput.SetValue("")
			m.titleInput.Focus()
			return m, nil
		case 2:
			if m.llmCli == nil {
				return m, func() tea.Msg { return ErrorMsg{Message: "no OpenRouter key configured"} }
			}
			m.step = plannerStepLLMPlan
			m.llmContext.SetValue("")
			m.llmContext.Focus()
			m.llmErr = ""
			return m, nil
		case 3:
			if len(m.tasks) == 0 {
				return m, nil
			}
			m.step = plannerStepReview
			return m, nil
		}
	case "tab":
		if len(m.tasks) > 0 {
			m.cursor = (m.cursor + 1) % len(m.tasks)
		}
	}
	return m, nil
}

func (m *PlannerModel) updateAddTask(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmds []tea.Cmd
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		cmds = append(cmds, cmd)
		m.descInput, cmd = m.descInput.Update(msg)
		cmds = append(cmds, cmd)
		m.hoursInput, cmd = m.hoursInput.Update(msg)
		cmds = append(cmds, cmd)
		m.dateInput, cmd = m.dateInput.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	switch k.String() {
	case "tab":
		return m, m.handleGenerateDescription()
	case "enter":
		return m, m.handleSaveTask()
	}

	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	m.descInput, _ = m.descInput.Update(msg)
	m.hoursInput, _ = m.hoursInput.Update(msg)
	m.dateInput, _ = m.dateInput.Update(msg)
	return m, cmd
}

func (m *PlannerModel) handleGenerateDescription() tea.Cmd {
	if m.llmCli == nil || m.titleInput.Value() == "" {
		return nil
	}
	title := m.titleInput.Value()
	return func() tea.Msg {
		desc, err := m.llmCli.GenerateTaskDescription(title)
		if err != nil {
			return llmDescResult{err: err.Error()}
		}
		return llmDescResult{desc: desc}
	}
}

type llmDescResult struct {
	desc string
	err  string
}

func (m *PlannerModel) handleSaveTask() tea.Cmd {
	title := m.titleInput.Value()
	if title == "" {
		return nil
	}

	var hours float64
	fmt.Sscanf(m.hoursInput.Value(), "%f", &hours)

	task := PlanTask{
		Title:       title,
		Description: m.descInput.Value(),
		Hours:       hours,
		DueDate:     m.dateInput.Value(),
	}

	if m.step == plannerStepAddSubtask && m.subParent < len(m.tasks) {
		m.tasks[m.subParent].Subtasks = append(m.tasks[m.subParent].Subtasks, task)
	} else {
		m.tasks = append(m.tasks, task)
	}

	m.step = plannerStepMenu
	return nil
}

func (m *PlannerModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		switch t := msg.(type) {
		case submitResult:
			m.submitIdx = t.idx
			if t.err != "" {
				m.submitErr = t.err
				m.step = plannerStepMenu
				return m, nil
			}
			if t.done {
				m.step = plannerStepDone
				return m, nil
			}
			return m, m.submitNext(t.idx)
		}
		return m, nil
	}

	switch k.String() {
	case "enter":
		m.submitIdx = 0
		m.submitErr = ""
		m.step = plannerStepSubmitting
		return m, m.submitNext(0)
	}
	return m, nil
}

type submitResult struct {
	idx  int
	err  string
	done bool
}

func (m *PlannerModel) submitNext(idx int) tea.Cmd {
	if idx >= len(m.tasks) {
		return func() tea.Msg { return submitResult{idx: idx, done: true} }
	}
	task := m.tasks[idx]
	wsGID := m.cfg.DefaultWorkspaceGID
	projGID := m.cfg.DefaultProjectGID
	hoursFieldGID := m.cfg.ActualHoursFieldGID

	return func() tea.Msg {
		input := map[string]interface{}{
			"name":      task.Title,
			"notes":     task.Description,
			"projects":  []string{projGID},
			"workspace": wsGID,
		}
		if task.DueDate != "" {
			input["due_on"] = task.DueDate
		}
		if hoursFieldGID != "" && task.Hours > 0 {
			input["custom_fields"] = map[string]interface{}{
				hoursFieldGID: task.Hours,
			}
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

func (m *PlannerModel) updateLLMPlan(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.llmGenerating {
		switch t := msg.(type) {
		case llmPlanResult:
			m.llmGenerating = false
			if t.err != "" {
				m.llmErr = t.err
				return m, nil
			}
			for _, pt := range t.tasks {
				m.tasks = append(m.tasks, PlanTask{
					Title:       pt.Title,
					Description: pt.Description,
					Hours:       pt.Hours,
					DueDate:     pt.DueDate,
					Subtasks:    convertSubtasks(pt.Subtasks),
				})
			}
			m.step = plannerStepMenu
			return m, nil
		}
		return m, nil
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.llmContext, cmd = m.llmContext.Update(msg)
		return m, cmd
	}

	switch k.String() {
	case "enter":
		if m.llmContext.Value() == "" {
			return m, nil
		}
		m.llmGenerating = true
		context := m.llmContext.Value()
		return m, func() tea.Msg {
			resp, err := m.llmCli.GenerateWeekPlan(context)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}
			tasks, err := llm.ParsePlan(resp)
			if err != nil {
				return llmPlanResult{err: err.Error()}
			}
			return llmPlanResult{tasks: tasks}
		}
	case "ctrl+s":
		return m, m.handleLLMSubmit()
	}

	var cmd tea.Cmd
	m.llmContext, cmd = m.llmContext.Update(msg)
	return m, cmd
}

type llmPlanResult struct {
	tasks []llm.PlannedTask
	err   string
}

func convertSubtasks(subs []struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Hours       float64 `json:"hours"`
}) []PlanTask {
	result := make([]PlanTask, len(subs))
	for i, s := range subs {
		result[i] = PlanTask{Title: s.Title, Description: s.Description, Hours: s.Hours}
	}
	return result
}

func (m *PlannerModel) handleLLMSubmit() tea.Cmd {
	return nil
}

func (m *PlannerModel) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if k.String() == "enter" || k.String() == "esc" {
		m.tasks = nil
		m.step = plannerStepMenu
	}
	return m, nil
}

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
	case plannerStepSubmitting:
		s += fmt.Sprintf("Submitting task %d of %d...\n", m.submitIdx+1, len(m.tasks))
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

		b.WriteString(ValueStyle.Render(fmt.Sprintf("%d. %s%s", i+1, t.Title, hoursStr)))
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
	b.WriteString(DimStyle.Render("enter submit to Asana • esc go back"))

	if m.submitErr != "" {
		b.WriteString("\n" + ErrorStyle.Render(m.submitErr))
	}

	return b.String()
}

func (m PlannerModel) viewLLMPlan() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("LLM Auto-Plan"))
	b.WriteString("\n\n")

	if m.llmGenerating {
		b.WriteString("Generating plan with AI...\n")
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
