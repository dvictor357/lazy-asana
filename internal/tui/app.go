package tui

import (
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
	"github.com/dvictor357/lazy-asana/internal/llm"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenMenu screen = iota
	screenBind
	screenPlanner
	screenMigrate
	screenSettings
	screenInsights
)

type App struct {
	cfg      *config.Config
	asana    asana.AsanaClient
	llm      llm.LLMClient
	screen   screen
	menu     MenuModel
	bind     BindModel
	settings SettingsModel
	planner  PlannerModel
	migrate  MigrateModel
	insights InsightsModel
	width    int
	height   int
	err      string
}

func NewApp(cfg *config.Config) *App {
	var ac asana.AsanaClient
	if cfg.AsanaToken != "" {
		ac = asana.NewClient(cfg.AsanaToken)
	}

	lc := newLLMClient(cfg)

	app := &App{
		cfg:    cfg,
		asana:  ac,
		llm:    lc,
		screen: screenMenu,
		menu:   NewMenu(cfg.IsBound(), lc != nil),
	}

	if !cfg.IsBound() {
		app.screen = screenBind
		app.bind = NewBindModel(cfg)
	} else {
		app.planner = NewPlannerModel(cfg, ac, lc)
		app.insights = NewInsightsModel(cfg, ac)
	}

	return app
}

func (a *App) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		}
	case ScreenMsg:
		a.screen = msg.Screen
		a.err = ""
		switch msg.Screen {
		case screenBind:
			a.bind = NewBindModel(a.cfg)
		case screenSettings:
			a.settings = NewSettingsModel(a.cfg)
		case screenPlanner:
			a.planner = NewPlannerModel(a.cfg, a.asana, a.llm)
			a.planner.width = a.width
		case screenMenu:
			a.menu = NewMenu(a.cfg.IsBound(), a.llm != nil)
		case screenInsights:
			a.insights = NewInsightsModel(a.cfg, a.asana)
			var cmd tea.Cmd
			a.insights, cmd = a.insights.startFetch()
			return a, cmd
		}
		return a, nil
	case ErrorMsg:
		a.err = msg.Message
		return a, nil
	case ConfigChangedMsg:
		a.llm = newLLMClient(a.cfg)
		a.menu = NewMenu(a.cfg.IsBound(), a.llm != nil)
		return a, nil
	case MoveTasksMsg:
		a.migrate = NewMigrateForTasks(a.cfg, a.asana, msg.Tasks)
		a.migrate.width = a.width
		a.screen = screenMigrate
		a.err = ""
		return a, a.migrate.Init()
	case BackToInsightsMsg:
		a.screen = screenInsights
		a.err = msg.Err
		if msg.Refresh {
			var cmd tea.Cmd
			a.insights, cmd = a.insights.reload()
			return a, cmd
		}
		return a, nil
	case BindCompleteMsg:
		a.cfg = msg.Config
		a.asana = asana.NewClient(a.cfg.AsanaToken)
		a.llm = newLLMClient(a.cfg)
		a.planner = NewPlannerModel(a.cfg, a.asana, a.llm)
		a.insights = NewInsightsModel(a.cfg, a.asana)
		a.menu = NewMenu(true, a.llm != nil)
		// Back to Settings, so LLM keys and git sources can be set next.
		a.settings = NewSettingsModel(a.cfg)
		a.screen = screenSettings
		return a, nil
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenMenu:
		m, c := a.menu.Update(msg)
		a.menu = m.(MenuModel)
		cmd = c
	case screenBind:
		m, c := a.bind.Update(msg)
		a.bind = m.(BindModel)
		cmd = c
	case screenSettings:
		m, c := a.settings.Update(msg)
		a.settings = m.(SettingsModel)
		cmd = c
	case screenPlanner:
		m, c := a.planner.Update(msg)
		a.planner = m.(PlannerModel)
		cmd = c
	case screenMigrate:
		m, c := a.migrate.Update(msg)
		a.migrate = m.(MigrateModel)
		cmd = c
	case screenInsights:
		m, c := a.insights.Update(msg)
		a.insights = m.(InsightsModel)
		cmd = c
	}
	return a, cmd
}

func (a *App) View() string {
	var content string
	switch a.screen {
	case screenMenu:
		content = a.menu.View()
	case screenBind:
		content = a.bind.View()
	case screenSettings:
		content = a.settings.View()
	case screenPlanner:
		content = a.planner.View()
	case screenMigrate:
		content = a.migrate.View()
	case screenInsights:
		content = a.insights.View()
	}

	if a.err != "" {
		content += "\n" + ErrorStyle.Render("Error: "+a.err)
	}
	return content
}

type ScreenMsg struct {
	Screen screen
}

type ErrorMsg struct {
	Message string
}

// MoveTasksMsg opens Move Tasks for tasks picked in Insights.
type MoveTasksMsg struct {
	Tasks []asana.Task
}

// BackToInsightsMsg returns from Move Tasks to Insights, keeping the week
// that was open; Refresh reloads it.
type BackToInsightsMsg struct {
	Refresh bool
	Err     string
}

type BindCompleteMsg struct {
	Config *config.Config
}

func newLLMClient(cfg *config.Config) llm.LLMClient {
	switch cfg.LLMProvider {
	case "deepseek":
		if cfg.DeepSeekKey != "" {
			return llm.NewDeepSeekClient(cfg.DeepSeekKey, "")
		}
	default:
		if cfg.OpenRouterKey != "" {
			return llm.NewClient(cfg.OpenRouterKey, "")
		}
	}
	// fallback: use whichever key is available
	if cfg.DeepSeekKey != "" {
		return llm.NewDeepSeekClient(cfg.DeepSeekKey, "")
	}
	if cfg.OpenRouterKey != "" {
		return llm.NewClient(cfg.OpenRouterKey, "")
	}
	return nil
}
