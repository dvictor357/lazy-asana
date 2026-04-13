package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

type bindStep int

const (
	bindStepTokens bindStep = iota
	bindStepWorkspace
	bindStepDone
)

type bindResult struct {
	workspaces []asana.Workspace
	projects   []asana.Project
	settings   []asana.CustomFieldSetting
	err        error
}

type BindModel struct {
	cfg           *config.Config
	step          bindStep
	asanaToken    textinput.Model
	openrouter    textinput.Model
	cursor        int
	workspaces    []asana.Workspace
	projects      []asana.Project
	wsCursor      int
	projCursor    int
	loading       bool
	status        string
	err           string
	hoursFieldGID string
}

func NewBindModel(cfg *config.Config) BindModel {
	at := textinput.New()
	at.Placeholder = "pat/..."
	at.EchoMode = textinput.EchoPassword
	at.EchoCharacter = '•'
	at.Focus()
	at.CharLimit = 200
	at.Width = 50

	or := textinput.New()
	or.Placeholder = "sk-or-..."
	or.EchoMode = textinput.EchoPassword
	or.EchoCharacter = '•'
	or.CharLimit = 200
	or.Width = 50

	return BindModel{
		cfg:        cfg,
		step:       bindStepTokens,
		asanaToken: at,
		openrouter: or,
	}
}

func (m BindModel) Init() tea.Cmd {
	return nil
}

func (m BindModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == bindStepWorkspace {
				m.step = bindStepTokens
				m.asanaToken.Focus()
				return m, nil
			}
			return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
		case "tab", "shift+tab":
			if m.step == bindStepTokens {
				if m.cursor == 0 {
					m.cursor = 1
					m.asanaToken.Blur()
					m.openrouter.Focus()
				} else {
					m.cursor = 0
					m.openrouter.Blur()
					m.asanaToken.Focus()
				}
			}
			return m, nil
		case "enter":
			return m.handleEnter()
		case "up", "k":
			if m.step == bindStepWorkspace && m.wsCursor > 0 {
				m.wsCursor--
			}
		case "down", "j":
			if m.step == bindStepWorkspace && m.wsCursor < len(m.workspaces)-1 {
				m.wsCursor++
			}
		}
	}

	var cmd tea.Cmd
	if m.step == bindStepTokens {
		if m.cursor == 0 {
			m.asanaToken, cmd = m.asanaToken.Update(msg)
		} else {
			m.openrouter, cmd = m.openrouter.Update(msg)
		}
	}

	if m.step == bindStepWorkspace {
		switch t := msg.(type) {
		case bindResult:
			m.loading = false
			if t.err != nil {
				m.err = t.err.Error()
				return m, nil
			}
			if m.workspaces == nil {
				m.workspaces = t.workspaces
				if len(m.workspaces) == 1 {
					m.wsCursor = 0
					return m, m.fetchProjects(m.workspaces[0].GID)
				}
			} else if m.projects == nil {
				m.projects = t.projects
				if len(m.projects) > 0 {
					m.projCursor = 0
				}
			}
			if t.settings != nil {
				m.hoursFieldGID = findActualHoursField(t.settings)
			}
		}
	}

	return m, cmd
}

func (m *BindModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case bindStepTokens:
		token := m.asanaToken.Value()
		if token == "" {
			m.err = "asana token is required"
			return m, nil
		}
		m.err = ""
		m.loading = true
		m.cfg.AsanaToken = token
		m.cfg.OpenRouterKey = m.openrouter.Value()
		return m, m.validateAndFetch(token)
	case bindStepWorkspace:
		if len(m.projects) == 0 {
			if m.wsCursor < len(m.workspaces) {
				return m, m.fetchProjects(m.workspaces[m.wsCursor].GID)
			}
		}
		if m.projCursor < len(m.projects) {
			m.cfg.DefaultWorkspaceGID = m.workspaces[m.wsCursor].GID
			m.cfg.DefaultProjectGID = m.projects[m.projCursor].GID
			m.cfg.ActualHoursFieldGID = m.hoursFieldGID
			if err := m.cfg.Save(); err != nil {
				m.err = err.Error()
				return m, nil
			}
			return m, func() tea.Msg { return BindCompleteMsg{Config: m.cfg} }
		}
	}
	return m, nil
}

func (m BindModel) validateAndFetch(token string) tea.Cmd {
	return func() tea.Msg {
		client := asana.NewClient(token)
		if _, err := client.GetMe(); err != nil {
			return bindResult{err: fmt.Errorf("invalid asana token: %w", err)}
		}
		ws, err := client.ListWorkspaces()
		if err != nil {
			return bindResult{err: fmt.Errorf("failed to fetch workspaces: %w", err)}
		}
		return bindResult{workspaces: ws}
	}
}

func (m BindModel) fetchProjects(wsGID string) tea.Cmd {
	return func() tea.Msg {
		client := asana.NewClient(m.cfg.AsanaToken)
		projects, err := client.ListProjects(wsGID)
		if err != nil {
			return bindResult{err: fmt.Errorf("failed to fetch projects: %w", err)}
		}
		settings, _ := client.GetCustomFieldSettings(projects[0].GID)
		return bindResult{projects: projects, settings: settings}
	}
}

func findActualHoursField(settings []asana.CustomFieldSetting) string {
	for _, s := range settings {
		name := s.CustomField.Name
		if name == "Actual Hours" || name == "actual_hours" || name == "ActualHours" {
			return s.CustomField.GID
		}
		if s.CustomField.Type == "number" && (name == "Hours" || name == "hours" || name == "Actual" || name == "Effort") {
			return s.CustomField.GID
		}
	}
	return ""
}

func (m BindModel) View() string {
	s := WindowTitle.Render("Bind Account")
	s += "\n\n"

	if m.step == bindStepTokens {
		s += LabelStyle.Render("Asana Personal Access Token")
		s += "\n"
		s += m.asanaToken.View()
		s += "\n\n"
		s += LabelStyle.Render("OpenRouter API Key (optional, for LLM features)")
		s += "\n"
		s += m.openrouter.View()
		s += "\n\n"
		s += DimStyle.Render("tab to switch fields • enter to continue • esc to go back")

		if m.err != "" {
			s += "\n" + ErrorStyle.Render(m.err)
		}
	}

	if m.step == bindStepWorkspace {
		if m.loading {
			s += "Loading...\n"
			return lipgloss.NewStyle().Padding(2, 4).Render(s)
		}

		if len(m.workspaces) > 0 && m.projects == nil {
			s += HeaderStyle.Render("Select Workspace")
			for i, ws := range m.workspaces {
				cursor := "  "
				style := MenuItem
				if i == m.wsCursor {
					cursor = "> "
					style = MenuItemSelected
				}
				s += style.Render(fmt.Sprintf("%s%s", cursor, ws.Name))
				s += "\n"
			}
			s += "\n" + DimStyle.Render("enter to select")
		}

		if len(m.projects) > 0 {
			s += HeaderStyle.Render(fmt.Sprintf("Workspace: %s", m.workspaces[m.wsCursor].Name))
			s += "\n"
			s += HeaderStyle.Render("Select Default Project")
			for i, p := range m.projects {
				cursor := "  "
				style := MenuItem
				if i == m.projCursor {
					cursor = "> "
					style = MenuItemSelected
				}
				s += style.Render(fmt.Sprintf("%s%s", cursor, p.Name))
				s += "\n"
			}
			s += "\n" + DimStyle.Render("enter to confirm • esc to go back")
		}

		if m.err != "" {
			s += "\n" + ErrorStyle.Render(m.err)
		}
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}
