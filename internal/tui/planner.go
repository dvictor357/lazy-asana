package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	plannerStepTagging
	plannerStepConfirm
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

	// Submission progress, so a resubmit after a failure skips what Asana
	// already has instead of creating duplicates.
	GID    string // set once the task exists in Asana
	Placed bool   // month section applied (or none needed)
}

// submitted reports whether the task and all its subtasks exist in Asana.
func (t PlanTask) submitted() bool {
	if t.GID == "" || !t.Placed {
		return false
	}
	for _, s := range t.Subtasks {
		if s.GID == "" {
			return false
		}
	}
	return true
}

type PlannerModel struct {
	cfg       *config.Config
	asanaCli  asana.AsanaClient
	llmCli    llm.LLMClient
	step      plannerStep
	menuIdx   int
	tasks     []PlanTask
	cursor    int
	subParent int

	titleInput textinput.Model
	descInput  textarea.Model
	hoursInput textinput.Model
	dateInput  textinput.Model

	llmGenerating  bool
	llmResult      string
	llmContext     textarea.Model
	llmStartDate   textinput.Model
	llmFieldCursor int // 0=startDate, 1=days, 2=context
	planDays       int

	fieldCursor       int
	llmDescGenerating bool
	submitIdx         int
	reviewCursor      int
	spinner           spinner.Model

	// tagging step
	entityOptions    []asana.EnumOption
	productOptions   []asana.EnumOption
	selectedEntities map[string]bool
	selectedProducts map[string]bool
	taggingSection   int // 0=entity, 1=product
	taggingCursor    int
	taggingLoading   bool

	// month sections of the default project, loaded before confirm
	sections       []asana.Section
	sectionsLoaded bool
	sectionsErr    string
}

// clampCursors keeps every task index valid after the task list shrinks.
func (m *PlannerModel) clampCursors() {
	n := len(m.tasks)
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.reviewCursor >= n {
		m.reviewCursor = n - 1
	}
	if m.reviewCursor < 0 {
		m.reviewCursor = 0
	}
}

func NewPlannerModel(cfg *config.Config, asanaCli asana.AsanaClient, llmCli llm.LLMClient) PlannerModel {
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

	sd := textinput.New()
	sd.Placeholder = "2026-05-08"
	sd.CharLimit = 10
	sd.Width = 12

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return PlannerModel{
		cfg:          cfg,
		asanaCli:     asanaCli,
		llmCli:       llmCli,
		step:         plannerStepMenu,
		titleInput:   ti,
		descInput:    di,
		hoursInput:   hi,
		dateInput:    dt,
		llmContext:   lc,
		llmStartDate: sd,
		spinner:      sp,
		planDays:     5,
	}
}

func (m PlannerModel) Init() tea.Cmd {
	return nil
}
