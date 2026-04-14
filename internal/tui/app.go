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
	screenHours
	screenPlanner
	screenChat
	screenMigrate
)

type App struct {
	cfg     *config.Config
	asana   *asana.Client
	llm     *llm.Client
	screen  screen
	menu    MenuModel
	bind    BindModel
	hours   HoursModel
	planner PlannerModel
	chat    ChatModel
	migrate MigrateModel
	width   int
	height  int
	err     string
}

func NewApp(cfg *config.Config) *App {
	var ac *asana.Client
	if cfg.AsanaToken != "" {
		ac = asana.NewClient(cfg.AsanaToken)
	}

	var lc *llm.Client
	if cfg.OpenRouterKey != "" {
		lc = llm.NewClient(cfg.OpenRouterKey, "")
	}

	app := &App{
		cfg:    cfg,
		asana:  ac,
		llm:    lc,
		screen: screenMenu,
		menu:   NewMenu(cfg.IsBound()),
	}

	if !cfg.IsBound() {
		app.screen = screenBind
		app.bind = NewBindModel(cfg)
	} else {
		app.hours = NewHoursModel(cfg, ac)
		app.planner = NewPlannerModel(cfg, ac, lc)
		if lc != nil {
			app.chat = NewChatModel(lc)
		}
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
		case screenHours:
			a.hours = NewHoursModel(a.cfg, a.asana)
		case screenPlanner:
			a.planner = NewPlannerModel(a.cfg, a.asana, a.llm)
			if a.llm != nil {
				a.chat = NewChatModel(a.llm)
			}
		case screenMenu:
			a.menu = NewMenu(a.cfg.IsBound())
		case screenMigrate:
			a.migrate = NewMigrateModel(a.cfg, a.asana)
		}
		return a, nil
	case ErrorMsg:
		a.err = msg.Message
		return a, nil
	case BindCompleteMsg:
		a.cfg = msg.Config
		a.asana = asana.NewClient(a.cfg.AsanaToken)
		a.llm = llm.NewClient(a.cfg.OpenRouterKey, "")
		a.hours = NewHoursModel(a.cfg, a.asana)
		a.planner = NewPlannerModel(a.cfg, a.asana, a.llm)
		a.chat = NewChatModel(a.llm)
		a.screen = screenMenu
		a.menu = NewMenu(true)
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
	case screenHours:
		m, c := a.hours.Update(msg)
		a.hours = m.(HoursModel)
		cmd = c
	case screenPlanner:
		m, c := a.planner.Update(msg)
		a.planner = m.(PlannerModel)
		cmd = c
	case screenChat:
		m, c := a.chat.Update(msg)
		a.chat = m.(ChatModel)
		cmd = c
	case screenMigrate:
		m, c := a.migrate.Update(msg)
		a.migrate = m.(MigrateModel)
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
	case screenHours:
		content = a.hours.View()
	case screenPlanner:
		content = a.planner.View()
	case screenChat:
		content = a.chat.View()
	case screenMigrate:
		content = a.migrate.View()
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

type BindCompleteMsg struct {
	Config *config.Config
}
