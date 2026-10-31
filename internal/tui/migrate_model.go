package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type migrateStep int

// Move Tasks starts from tasks picked in Insights, then walks through the
// target project, section and optional source project.
const (
	migrateStepLoadProjects migrateStep = iota
	migrateStepPickTarget
	migrateStepLoadSections
	migrateStepPickSection
	migrateStepPickSource
	migrateStepMoving
	migrateStepDone
)

// ── Messages ──────────────────────────────────────────────────────────────────

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
	cfg    *config.Config
	client asana.AsanaClient
	step   migrateStep
	width  int
	// execution targets, handed over from Insights
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
	spinner    spinner.Model
}

// NewMigrateForTasks starts Move Tasks for tasks picked in Insights. Call
// Init to load the project list.
func NewMigrateForTasks(cfg *config.Config, client asana.AsanaClient, tasks []asana.Task) MigrateModel {
	ps := textinput.New()
	ps.Placeholder = "type to filter..."
	ps.CharLimit = 100
	ps.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := MigrateModel{
		cfg:           cfg,
		client:        client,
		step:          migrateStepLoadProjects,
		projSearch:    ps,
		spinner:       sp,
		selectedTasks: tasks,
	}
	if len(tasks) > 0 {
		m.selectedTask = tasks[0]
	}
	return m
}

func (m MigrateModel) Init() tea.Cmd {
	return tea.Batch(m.fetchProjects(), m.spinner.Tick)
}

// backToInsights leaves Move Tasks; refresh reloads the week so moved tasks
// show their new place.
func backToInsights(refresh bool, errText string) tea.Cmd {
	return func() tea.Msg { return BackToInsightsMsg{Refresh: refresh, Err: errText} }
}
