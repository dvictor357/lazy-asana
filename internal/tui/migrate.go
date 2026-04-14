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

type migrateStep int

const (
	migrateStepWeek         migrateStep = iota
	migrateStepFetching
	migrateStepTasks
	migrateStepLoadProjects
	migrateStepPickTarget
	migrateStepLoadSections
	migrateStepPickSection
	migrateStepPickSource
	migrateStepMoving
	migrateStepDone
)

// ── Messages ──────────────────────────────────────────────────────────────────

type migrateTasksMsg struct {
	tasks        []asana.Task
	fetchedStart string
	fetchedEnd   string
	weekNum      int
	err          error
}

type migrateProjsMsg struct {
	projects []asana.Project
	err      error
}

type migrateSectionsMsg struct {
	sections []asana.Section
	err      error
}

type migrateExecMsg struct {
	nextIdx int
	moved   int
	failed  int
	errLine string
	done    bool
}

// ── Model ─────────────────────────────────────────────────────────────────────

type MigrateModel struct {
	cfg          *config.Config
	client       *asana.Client
	step         migrateStep
	width        int
	weekInput    textinput.Model
	weekNum      int
	fetchedStart string
	fetchedEnd   string
	// task list
	tasks        []asana.Task
	taskCursor   int
	selected     map[int]bool  // checkbox state (index into tasks)
	// execution targets (derived on Enter)
	selectedTasks []asana.Task // tasks to move (one or many)
	selectedTask  asana.Task   // first selected (used for section auto-detect)
	// project picker
	projects   []asana.Project
	projSearch textinput.Model
	projCursor int
	targetProj asana.Project
	// section picker
	sections      []asana.Section
	sectionCursor int
	// source picker
	sourceOpts   []asana.Project
	sourceCursor int
	// progress / result
	moveMoved  int
	moveFailed int
	moveErrors []string
	err        string
	spinner    spinner.Model
}

func NewMigrateModel(cfg *config.Config, client *asana.Client) MigrateModel {
	wi := textinput.New()
	wi.Placeholder = fmt.Sprintf("%d", asana.CurrentWeek())
	wi.CharLimit = 2
	wi.Width = 6
	wi.Focus()

	ps := textinput.New()
	ps.Placeholder = "type to filter..."
	ps.CharLimit = 100
	ps.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return MigrateModel{
		cfg:        cfg,
		client:     client,
		step:       migrateStepWeek,
		weekInput:  wi,
		projSearch: ps,
		spinner:    sp,
		selected:   make(map[int]bool),
	}
}

func (m MigrateModel) Init() tea.Cmd { return nil }

// ── Update ────────────────────────────────────────────────────────────────────

func (m MigrateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m.handleMsg(msg)
}

func (m *MigrateModel) handleMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case migrateTasksMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.step = migrateStepWeek
			m.weekInput.Focus()
			return *m, nil
		}
		m.tasks = msg.tasks
		m.fetchedStart = msg.fetchedStart
		m.fetchedEnd = msg.fetchedEnd
		m.weekNum = msg.weekNum
		m.taskCursor = 0
		m.selected = make(map[int]bool)
		m.step = migrateStepTasks
		return *m, nil

	case migrateProjsMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			m.step = migrateStepTasks
			return *m, nil
		}
		m.projects = msg.projects
		m.projCursor = 0
		m.projSearch.SetValue("")
		m.projSearch.Focus()
		m.step = migrateStepPickTarget
		return *m, nil

	case migrateSectionsMsg:
		if msg.err != nil {
			m.sections = nil
			m.step = migrateStepPickSource
			return *m, nil
		}
		m.sections = msg.sections
		// Auto-select section matching the first selected task's due-date month
		monthName := time.Now().Format("January")
		if m.selectedTask.DueOn != "" {
			if t, err := time.Parse("2006-01-02", m.selectedTask.DueOn); err == nil {
				monthName = t.Format("January")
			}
		}
		m.sectionCursor = len(m.sections) // default: no section
		for i, s := range m.sections {
			if strings.EqualFold(s.Name, monthName) {
				m.sectionCursor = i
				break
			}
		}
		m.step = migrateStepPickSection
		return *m, nil

	case migrateExecMsg:
		if msg.errLine != "" {
			m.moveErrors = append(m.moveErrors, msg.errLine)
		}
		m.moveMoved = msg.moved
		m.moveFailed = msg.failed
		if msg.done {
			m.step = migrateStepDone
			return *m, nil
		}
		return *m, m.execMoveNext(msg.nextIdx, msg.moved, msg.failed)
	}

	return *m, nil
}

func (m *MigrateModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {

	// ── week input ────────────────────────────────────────────────────────────
	case migrateStepWeek:
		switch msg.String() {
		case "esc":
			return *m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
		case "enter":
			weekStr := m.weekInput.Value()
			var weekNum int
			if weekStr == "" {
				weekNum = asana.CurrentWeek()
			} else {
				fmt.Sscanf(weekStr, "%d", &weekNum)
			}
			if err := asana.ValidateWeek(asana.CurrentYear(), weekNum); err != nil {
				m.err = fmt.Sprintf("invalid week: %s", err)
				return *m, nil
			}
			m.err = ""
			m.weekNum = weekNum
			m.step = migrateStepFetching
			m.weekInput.Blur()
			return *m, tea.Batch(m.fetchTasks(weekNum), m.spinner.Tick)
		default:
			var cmd tea.Cmd
			m.weekInput, cmd = m.weekInput.Update(msg)
			return *m, cmd
		}

	// ── task list ─────────────────────────────────────────────────────────────
	case migrateStepTasks:
		switch msg.String() {
		case "esc":
			m.step = migrateStepWeek
			m.tasks = nil
			m.selected = make(map[int]bool)
			m.weekInput.Focus()
			return *m, nil
		case "up", "k":
			if m.taskCursor > 0 {
				m.taskCursor--
			}
			return *m, nil
		case "down", "j":
			if m.taskCursor < len(m.tasks)-1 {
				m.taskCursor++
			}
			return *m, nil
		case " ":
			// Toggle checkbox on current task
			m.selected[m.taskCursor] = !m.selected[m.taskCursor]
			return *m, nil
		case "a":
			// Select all / deselect all
			if len(m.selected) == len(m.tasks) {
				m.selected = make(map[int]bool)
			} else {
				for i := range m.tasks {
					m.selected[i] = true
				}
			}
			return *m, nil
		case "enter":
			if len(m.tasks) == 0 {
				return *m, nil
			}
			// Build selectedTasks: checked ones, or just the cursor task
			m.selectedTasks = nil
			for i, t := range m.tasks {
				if m.selected[i] {
					m.selectedTasks = append(m.selectedTasks, t)
				}
			}
			if len(m.selectedTasks) == 0 {
				m.selectedTasks = []asana.Task{m.tasks[m.taskCursor]}
			}
			m.selectedTask = m.selectedTasks[0]
			// Load projects (or reuse cache)
			if m.projects != nil {
				m.projCursor = 0
				m.projSearch.SetValue("")
				m.projSearch.Focus()
				m.step = migrateStepPickTarget
				return *m, nil
			}
			m.step = migrateStepLoadProjects
			return *m, tea.Batch(m.fetchProjects(), m.spinner.Tick)
		}
		return *m, nil

	// ── target project picker ─────────────────────────────────────────────────
	case migrateStepPickTarget:
		filtered := m.filteredProjects()
		switch msg.String() {
		case "esc":
			m.step = migrateStepTasks
			m.projSearch.Blur()
			return *m, nil
		case "up", "k":
			if m.projCursor > 0 {
				m.projCursor--
			}
			return *m, nil
		case "down", "j":
			if m.projCursor < len(filtered)-1 {
				m.projCursor++
			}
			return *m, nil
		case "enter":
			if len(filtered) == 0 || m.projCursor >= len(filtered) {
				return *m, nil
			}
			m.targetProj = filtered[m.projCursor]
			m.projSearch.Blur()
			// Build source options: union of all selected tasks' memberships, minus target
			seen := map[string]bool{}
			m.sourceOpts = nil
			for _, task := range m.selectedTasks {
				for _, mem := range task.Memberships {
					if mem.Project.GID != m.targetProj.GID && !seen[mem.Project.GID] {
						seen[mem.Project.GID] = true
						m.sourceOpts = append(m.sourceOpts, asana.Project{GID: mem.Project.GID, Name: mem.Project.Name})
					}
				}
			}
			m.sourceCursor = 0
			m.step = migrateStepLoadSections
			return *m, tea.Batch(m.fetchSections(m.targetProj.GID), m.spinner.Tick)
		default:
			prev := m.projSearch.Value()
			var cmd tea.Cmd
			m.projSearch, cmd = m.projSearch.Update(msg)
			if m.projSearch.Value() != prev {
				m.projCursor = 0
			}
			return *m, cmd
		}

	// ── section picker ────────────────────────────────────────────────────────
	case migrateStepPickSection:
		total := len(m.sections) + 1
		switch msg.String() {
		case "esc":
			m.step = migrateStepPickTarget
			m.projSearch.Focus()
			return *m, nil
		case "up", "k":
			if m.sectionCursor > 0 {
				m.sectionCursor--
			}
			return *m, nil
		case "down", "j":
			if m.sectionCursor < total-1 {
				m.sectionCursor++
			}
			return *m, nil
		case "enter":
			m.step = migrateStepPickSource
			return *m, nil
		}
		return *m, nil

	// ── source project picker ─────────────────────────────────────────────────
	case migrateStepPickSource:
		total := len(m.sourceOpts) + 1
		switch msg.String() {
		case "esc":
			if len(m.sections) > 0 {
				m.step = migrateStepPickSection
			} else {
				m.step = migrateStepPickTarget
				m.projSearch.Focus()
			}
			return *m, nil
		case "up", "k":
			if m.sourceCursor > 0 {
				m.sourceCursor--
			}
			return *m, nil
		case "down", "j":
			if m.sourceCursor < total-1 {
				m.sourceCursor++
			}
			return *m, nil
		case "enter":
			m.step = migrateStepMoving
			m.moveMoved = 0
			m.moveFailed = 0
			m.moveErrors = nil
			return *m, tea.Batch(m.execMoveNext(0, 0, 0), m.spinner.Tick)
		}
		return *m, nil

	// ── done ─────────────────────────────────────────────────────────────────
	case migrateStepDone:
		switch msg.String() {
		case "r", "enter":
			m.step = migrateStepFetching
			m.moveMoved = 0
			m.moveFailed = 0
			m.moveErrors = nil
			m.selected = make(map[int]bool)
			return *m, tea.Batch(m.fetchTasks(m.weekNum), m.spinner.Tick)
		case "esc", "q":
			return *m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
		}
		return *m, nil
	}

	return *m, nil
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (m MigrateModel) fetchTasks(weekNum int) tea.Cmd {
	wsGID := m.cfg.DefaultWorkspaceGID
	userGID := m.cfg.UserGID
	client := m.client
	return func() tea.Msg {
		dateStart, dateEnd := asana.WeekToDateRange(asana.CurrentYear(), weekNum)
		filters := url.Values{
			"due_on.after":  {shiftDate(dateStart, -1)},
			"due_on.before": {shiftDate(dateEnd, 1)},
		}
		if userGID != "" {
			filters["assignee.any"] = []string{userGID}
		}
		tasks, err := client.SearchTasks(wsGID, filters)
		if err != nil {
			return migrateTasksMsg{err: err}
		}
		return migrateTasksMsg{tasks: tasks, fetchedStart: dateStart, fetchedEnd: dateEnd, weekNum: weekNum}
	}
}

func (m MigrateModel) fetchProjects() tea.Cmd {
	wsGID := m.cfg.DefaultWorkspaceGID
	client := m.client
	return func() tea.Msg {
		projects, err := client.ListProjects(wsGID)
		if err != nil {
			return migrateProjsMsg{err: err}
		}
		return migrateProjsMsg{projects: projects}
	}
}

func (m MigrateModel) fetchSections(projectGID string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		sections, err := client.ListSections(projectGID)
		if err != nil {
			return migrateSectionsMsg{err: err}
		}
		return migrateSectionsMsg{sections: sections}
	}
}

func (m *MigrateModel) execMoveNext(idx, moved, failed int) tea.Cmd {
	if idx >= len(m.selectedTasks) {
		return func() tea.Msg {
			return migrateExecMsg{done: true, moved: moved, failed: failed}
		}
	}
	task := m.selectedTasks[idx]
	tgtGID := m.targetProj.GID
	var sectionGID string
	if m.sectionCursor < len(m.sections) {
		sectionGID = m.sections[m.sectionCursor].GID
	}
	var srcGID string
	if m.sourceCursor < len(m.sourceOpts) {
		srcGID = m.sourceOpts[m.sourceCursor].GID
	}
	client := m.client
	return func() tea.Msg {
		if err := client.AddTaskToProject(task.GID, tgtGID, sectionGID); err != nil {
			return migrateExecMsg{nextIdx: idx + 1, moved: moved, failed: failed + 1,
				errLine: fmt.Sprintf("%s: add failed: %s", task.Name, err)}
		}
		// Only remove from source if this task is actually in that source project
		if srcGID != "" {
			inSrc := false
			for _, mem := range task.Memberships {
				if mem.Project.GID == srcGID {
					inSrc = true
					break
				}
			}
			if inSrc {
				if err := client.RemoveTaskFromProject(task.GID, srcGID); err != nil {
					return migrateExecMsg{nextIdx: idx + 1, moved: moved + 1, failed: failed,
						errLine: fmt.Sprintf("%s: remove from source failed: %s", task.Name, err)}
				}
			}
		}
		return migrateExecMsg{nextIdx: idx + 1, moved: moved + 1, failed: failed}
	}
}

func (m MigrateModel) filteredProjects() []asana.Project {
	query := strings.ToLower(strings.TrimSpace(m.projSearch.Value()))
	if query == "" {
		return m.projects
	}
	var out []asana.Project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			out = append(out, p)
		}
	}
	return out
}

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
	if m.err != "" {
		b.WriteString("\n" + ErrorStyle.Render(m.err))
	}
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
