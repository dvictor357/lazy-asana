package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

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
			m.step = migrateStepWeek
			m.weekInput.Focus()
			return *m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
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
			m.step = migrateStepTasks
			return *m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
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
			return *m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
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
				return *m, func() tea.Msg { return ErrorMsg{Message: fmt.Sprintf("invalid week: %s", err)} }
			}
			m.weekNum = weekNum
			m.step = migrateStepFetching
			m.weekInput.Blur()
			return *m, tea.Batch(m.fetchTasks(weekNum), m.spinner.Tick, func() tea.Msg { return ErrorMsg{Message: ""} })
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
			return *m, func() tea.Msg { return ErrorMsg{Message: ""} }
		case "up", "k", "ctrl+p":
			if m.taskCursor > 0 {
				m.taskCursor--
			}
			return *m, nil
		case "down", "j", "ctrl+n":
			if m.taskCursor < len(m.tasks)-1 {
				m.taskCursor++
			}
			return *m, nil
		case " ":
			// Toggle checkbox on current task
			if m.selected[m.taskCursor] {
				delete(m.selected, m.taskCursor)
			} else {
				m.selected[m.taskCursor] = true
			}
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
		case "up", "k", "ctrl+p":
			if m.projCursor > 0 {
				m.projCursor--
			}
			return *m, nil
		case "down", "j", "ctrl+n":
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
		case "up", "k", "ctrl+p":
			if m.sectionCursor > 0 {
				m.sectionCursor--
			}
			return *m, nil
		case "down", "j", "ctrl+n":
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
		case "up", "k", "ctrl+p":
			if m.sourceCursor > 0 {
				m.sourceCursor--
			}
			return *m, nil
		case "down", "j", "ctrl+n":
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
