package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type migrateStep int

const (
	migrateStepWeek migrateStep = iota
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
	client       asana.AsanaClient
	step         migrateStep
	width        int
	weekInput    textinput.Model
	weekNum      int
	fetchedStart string
	fetchedEnd   string
	// task list
	tasks      []asana.Task
	taskCursor int
	selected   map[int]bool // checkbox state (index into tasks)
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
	spinner    spinner.Model
}

func NewMigrateModel(cfg *config.Config, client asana.AsanaClient) MigrateModel {
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
