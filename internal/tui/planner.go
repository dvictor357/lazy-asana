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

	titleInput    textinput.Model
	descInput     textarea.Model
	hoursInput    textinput.Model
	dateInput     textinput.Model
	validationErr string

	llmGenerating bool
	llmResult     string
	llmErr        string
	llmContext    textarea.Model

	submitIdx    int
	submitErr    string
	reviewCursor int
	spinner      spinner.Model
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

	sp := spinner.New()
	sp.Spinner = spinner.Dot

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
		spinner:    sp,
	}
}

func (m PlannerModel) Init() tea.Cmd {
	return nil
}
