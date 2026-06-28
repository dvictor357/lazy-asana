package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
	bindStepRescanFields
	bindStepDone
)

type bindResult struct {
	workspaces []asana.Workspace
	projects   []asana.Project
	userGID    string
	err        error
}

type customFieldsResult struct {
	settings []asana.CustomFieldSetting
	err      error
}

type BindModel struct {
	cfg           *config.Config
	step          bindStep
	asanaToken    textinput.Model
	openrouter    textinput.Model
	deepseek      textinput.Model
	provider      string // "openrouter" or "deepseek"
	cursor        int
	workspaces    []asana.Workspace
	projects      []asana.Project
	wsCursor      int
	projCursor    int
	projSearch    textinput.Model
	loading       bool
	status        string
	hoursFieldGID string
	spinner       spinner.Model
}

func NewBindModel(cfg *config.Config) BindModel {
	at := textinput.New()
	at.Placeholder = "pat/..."
	at.EchoMode = textinput.EchoPassword
	at.EchoCharacter = '•'
	at.CharLimit = 200
	at.Width = 50
	at.SetValue(cfg.AsanaToken)
	at.Focus()

	or := textinput.New()
	or.Placeholder = "sk-or-..."
	or.EchoMode = textinput.EchoPassword
	or.EchoCharacter = '•'
	or.CharLimit = 200
	or.Width = 50
	or.SetValue(cfg.OpenRouterKey)

	ds := textinput.New()
	ds.Placeholder = "sk-..."
	ds.EchoMode = textinput.EchoPassword
	ds.EchoCharacter = '•'
	ds.CharLimit = 200
	ds.Width = 50
	ds.SetValue(cfg.DeepSeekKey)

	ps := textinput.New()
	ps.Placeholder = "type to filter..."
	ps.CharLimit = 100
	ps.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	provider := cfg.LLMProvider
	if provider == "" {
		provider = "openrouter"
	}

	return BindModel{
		cfg:        cfg,
		step:       bindStepTokens,
		asanaToken: at,
		openrouter: or,
		deepseek:   ds,
		provider:   provider,
		projSearch: ps,
		spinner:    sp,
	}
}

func (m BindModel) Init() tea.Cmd {
	return nil
}

func (m BindModel) filteredProjects() []asana.Project {
	query := strings.ToLower(strings.TrimSpace(m.projSearch.Value()))
	if query == "" {
		return m.projects
	}
	var result []asana.Project
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			result = append(result, p)
		}
	}
	return result
}

func (m BindModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	showingProjects := m.step == bindStepWorkspace && len(m.projects) > 0

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.step == bindStepWorkspace {
				m.step = bindStepTokens
				m.projects = nil
				m.workspaces = nil
				m.projSearch.SetValue("")
				m.asanaToken.Focus()
				return m, nil
			}
			return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }

		case "tab", "ctrl+f":
			fallthrough
		case "shift+tab", "ctrl+b":
			if m.step == bindStepTokens {
				m.asanaToken.Blur()
				m.openrouter.Blur()
				m.deepseek.Blur()
				const numFields = 3
				if msg.String() == "tab" || msg.String() == "ctrl+f" {
					m.cursor = (m.cursor + 1) % numFields
				} else {
					m.cursor = (m.cursor + numFields - 1) % numFields
				}
				switch m.cursor {
				case 0:
					m.asanaToken.Focus()
				case 1:
					m.openrouter.Focus()
				case 2:
					m.deepseek.Focus()
				}
			}
			return m, nil

		case "p":
			if m.step == bindStepTokens {
				if m.provider == "openrouter" {
					m.provider = "deepseek"
				} else {
					m.provider = "openrouter"
				}
			}
			return m, nil

		case "ctrl+r":
			if m.step == bindStepTokens && m.cfg.IsBound() && m.cfg.DefaultProjectGID != "" {
				m.step = bindStepRescanFields
				m.loading = true
				return m, tea.Batch(m.fetchCustomFields(m.cfg.DefaultProjectGID), m.spinner.Tick)
			}
			return m, nil

		case "up", "k", "ctrl+p":
			if showingProjects {
				if m.projCursor > 0 {
					m.projCursor--
				}
				return m, nil
			}
			if m.step == bindStepWorkspace && m.wsCursor > 0 {
				m.wsCursor--
			}
			return m, nil

		case "down", "j", "ctrl+n":
			if showingProjects {
				filtered := m.filteredProjects()
				if m.projCursor < len(filtered)-1 {
					m.projCursor++
				}
				return m, nil
			}
			if m.step == bindStepWorkspace && m.wsCursor < len(m.workspaces)-1 {
				m.wsCursor++
			}
			return m, nil

		case "enter":
			return m.handleEnter()
		}

		// Typing goes to search input when showing projects
		if showingProjects {
			prev := m.projSearch.Value()
			var cmd tea.Cmd
			m.projSearch, cmd = m.projSearch.Update(msg)
			if m.projSearch.Value() != prev {
				m.projCursor = 0
			}
			return m, cmd
		}

	case bindResult:
		if m.step == bindStepWorkspace {
			m.loading = false
			if msg.err != nil {
				return m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
			}
			if msg.userGID != "" {
				m.cfg.UserGID = msg.userGID
			}
			if m.workspaces == nil {
				m.workspaces = msg.workspaces
				if len(m.workspaces) == 1 {
					m.wsCursor = 0
					m.loading = true
					return m, tea.Batch(m.fetchProjects(m.workspaces[0].GID), m.spinner.Tick)
				}
			} else if m.projects == nil {
				m.projects = msg.projects
				m.projCursor = 0
				m.projSearch.SetValue("")
				m.projSearch.Focus()
			}
			return m, nil
		}

	case customFieldsResult:
		m.loading = false
		if msg.err == nil {
			m.hoursFieldGID = findActualHoursField(msg.settings)
			entityGID, entityType := findEnumField(msg.settings, "company", "entity")
			productGID, productType := findEnumField(msg.settings, "product", "project")
			m.cfg.EntityFieldGID = entityGID
			m.cfg.EntityFieldType = entityType
			m.cfg.ProductFieldGID = productGID
			m.cfg.ProductFieldType = productType
		} else {
			if m.step == bindStepRescanFields {
				m.step = bindStepTokens
				m.asanaToken.Focus()
			}
			return m, func() tea.Msg { return ErrorMsg{Message: msg.err.Error()} }
		}
		m.cfg.ActualHoursFieldGID = m.hoursFieldGID
		if err := m.cfg.Save(); err != nil {
			if m.step == bindStepRescanFields {
				m.step = bindStepTokens
				m.asanaToken.Focus()
			}
			return m, func() tea.Msg { return ErrorMsg{Message: err.Error()} }
		}
		return m, func() tea.Msg { return BindCompleteMsg{Config: m.cfg} }
	}

	var cmd tea.Cmd
	if m.step == bindStepTokens {
		switch m.cursor {
		case 0:
			m.asanaToken, cmd = m.asanaToken.Update(msg)
		case 1:
			m.openrouter, cmd = m.openrouter.Update(msg)
		case 2:
			m.deepseek, cmd = m.deepseek.Update(msg)
		}
	}
	return m, cmd
}

func (m *BindModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case bindStepTokens:
		token := m.asanaToken.Value()
		if token == "" {
			return *m, func() tea.Msg { return ErrorMsg{Message: "asana token is required"} }
		}
		m.loading = true
		m.step = bindStepWorkspace
		m.cfg.AsanaToken = token
		m.cfg.OpenRouterKey = m.openrouter.Value()
		m.cfg.DeepSeekKey = m.deepseek.Value()
		m.cfg.LLMProvider = m.provider
		return *m, tea.Batch(
			func() tea.Msg { return ErrorMsg{Message: ""} },
			m.validateAndFetch(token),
			m.spinner.Tick,
		)

	case bindStepWorkspace:
		if len(m.projects) == 0 {
			if m.wsCursor < len(m.workspaces) {
				m.loading = true
				return *m, tea.Batch(m.fetchProjects(m.workspaces[m.wsCursor].GID), m.spinner.Tick)
			}
		}
		filtered := m.filteredProjects()
		if m.projCursor < len(filtered) {
			proj := filtered[m.projCursor]
			m.cfg.DefaultWorkspaceGID = m.workspaces[m.wsCursor].GID
			m.cfg.DefaultProjectGID = proj.GID
			m.loading = true
			return *m, tea.Batch(m.fetchCustomFields(proj.GID), m.spinner.Tick)
		}
	}
	return *m, nil
}

func (m BindModel) validateAndFetch(token string) tea.Cmd {
	return func() tea.Msg {
		client := asana.NewClient(token)
		me, err := client.GetMe()
		if err != nil {
			return bindResult{err: fmt.Errorf("invalid asana token: %w", err)}
		}
		ws, err := client.ListWorkspaces()
		if err != nil {
			return bindResult{err: fmt.Errorf("failed to fetch workspaces: %w", err)}
		}
		return bindResult{workspaces: ws, userGID: me.GID}
	}
}

func (m BindModel) fetchProjects(wsGID string) tea.Cmd {
	return func() tea.Msg {
		client := asana.NewClient(m.cfg.AsanaToken)
		projects, err := client.ListProjects(wsGID)
		if err != nil {
			return bindResult{err: fmt.Errorf("failed to fetch projects: %w", err)}
		}
		return bindResult{projects: projects}
	}
}

func (m BindModel) fetchCustomFields(projectGID string) tea.Cmd {
	return func() tea.Msg {
		client := asana.NewClient(m.cfg.AsanaToken)
		settings, err := client.GetCustomFieldSettings(projectGID)
		if err != nil {
			return customFieldsResult{err: err}
		}
		return customFieldsResult{settings: settings}
	}
}

func findEnumField(settings []asana.CustomFieldSetting, keywords ...string) (gid, fieldType string) {
	for _, s := range settings {
		name := strings.ToLower(s.CustomField.Name)
		for _, kw := range keywords {
			if strings.Contains(name, kw) {
				t := s.CustomField.Type
				if t == "enum" || t == "multi_enum" {
					return s.CustomField.GID, t
				}
			}
		}
	}
	return "", ""
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

	if m.step == bindStepRescanFields {
		s += m.spinner.View() + " Re-scanning custom fields...\n"
	}

	if m.step == bindStepTokens {
		s += LabelStyle.Render("Asana Personal Access Token")
		s += "\n"
		s += m.asanaToken.View()
		s += "\n\n"
		s += LabelStyle.Render("OpenRouter API Key (optional)")
		s += "\n"
		s += m.openrouter.View()
		s += "\n\n"
		s += LabelStyle.Render("DeepSeek API Key (optional)")
		s += "\n"
		s += m.deepseek.View()
		s += "\n\n"

		orMark := "[ ]"
		dsMark := "[ ]"
		if m.provider == "deepseek" {
			dsMark = "[x]"
		} else {
			orMark = "[x]"
		}
		s += LabelStyle.Render("Active LLM Provider:")
		s += " " + DimStyle.Render(orMark+" openrouter")
		s += "  " + DimStyle.Render(dsMark+" deepseek")
		s += "\n\n"
		s += DimStyle.Render("tab/shift+tab switch fields • p toggle provider • ctrl+r re-scan fields • enter continue • esc back")
	}

	if m.step == bindStepWorkspace {
		if m.loading {
			s += m.spinner.View() + " Loading...\n"
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
			s += "\n" + DimStyle.Render("↑/k↓/j navigate • enter to select")
		}

		if len(m.projects) > 0 {
			filtered := m.filteredProjects()

			wsName := ""
			if m.wsCursor < len(m.workspaces) {
				wsName = m.workspaces[m.wsCursor].Name
			}
			s += HeaderStyle.Render(fmt.Sprintf("Workspace: %s", wsName))
			s += "\n"

			matchInfo := fmt.Sprintf("%d/%d", len(filtered), len(m.projects))
			if m.projSearch.Value() != "" {
				matchInfo = fmt.Sprintf("%d match • %d total", len(filtered), len(m.projects))
			}
			s += HeaderStyle.Render(fmt.Sprintf("Select Default Project (%s)", matchInfo))
			s += "\n"

			s += LabelStyle.Render("Search: ")
			s += m.projSearch.View()
			s += "\n\n"

			if len(filtered) == 0 {
				s += DimStyle.Render("  no projects match your search")
				s += "\n"
			} else {
				const pageSize = 10
				cursor := m.projCursor
				if cursor >= len(filtered) {
					cursor = len(filtered) - 1
				}

				start := cursor - pageSize/2
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
					s += DimStyle.Render(fmt.Sprintf("  ↑ %d more", start)) + "\n"
				}
				for i := start; i < end; i++ {
					p := filtered[i]
					cur := "  "
					style := MenuItem
					if i == cursor {
						cur = "> "
						style = MenuItemSelected
					}
					s += style.Render(fmt.Sprintf("%s%s", cur, p.Name))
					s += "\n"
				}
				if end < len(filtered) {
					s += DimStyle.Render(fmt.Sprintf("  ↓ %d more", len(filtered)-end)) + "\n"
				}
			}

			s += "\n" + DimStyle.Render("type to search • ↑/k↓/j navigate • enter confirm • esc back")
		}
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}
