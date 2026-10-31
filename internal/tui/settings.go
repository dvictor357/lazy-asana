package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/config"
)

// ConfigChangedMsg tells the app to rebuild clients from the saved config.
type ConfigChangedMsg struct{}

type settingsStep int

const (
	settingsHub settingsStep = iota
	settingsLLM
	settingsGitList
	settingsGitKind
	settingsGitForm
)

// gitKind is one choice in the "add source" picker.
type gitKind struct {
	label    string
	kind     string
	fixedURL string // set when the URL is not asked for
	hint     string
}

var gitKinds = []gitKind{
	{"GitLab.com", config.GitLab, config.GitLabDotCom, "personal access token with read_api scope"},
	{"GitLab self-hosted", config.GitLab, "", "your instance URL and a token with read_api scope"},
	{"GitHub", config.GitHub, "", "token with repo scope (classic) or Contents: read (fine-grained)"},
	{"Local folder", config.Local, "", "a folder holding your clones, scanned without a token"},
}

// git form fields
const (
	gfName = iota
	gfURL
	gfToken
	gfPath
	gfDepth
)

type SettingsModel struct {
	cfg    *config.Config
	step   settingsStep
	cursor int // hub row

	// LLM form
	provider string
	orKey    textinput.Model
	dsKey    textinput.Model
	llmField int // 0 provider, 1 OpenRouter, 2 DeepSeek

	// git sources
	gitCursor     int
	confirmDelete bool
	kindCursor    int
	editIdx       int // source being edited, -1 when adding
	formKind      int // index into gitKinds
	inputs        [5]textinput.Model
	formField     int // index into m.formFields()
}

func secretInput(placeholder, value string) textinput.Model {
	t := textinput.New()
	t.Placeholder = placeholder
	t.EchoMode = textinput.EchoPassword
	t.EchoCharacter = '•'
	t.CharLimit = 300
	t.Width = 50
	t.SetValue(value)
	return t
}

func plainInput(placeholder string, limit int) textinput.Model {
	t := textinput.New()
	t.Placeholder = placeholder
	t.CharLimit = limit
	t.Width = 50
	return t
}

func NewSettingsModel(cfg *config.Config) SettingsModel {
	provider := cfg.LLMProvider
	if provider == "" {
		provider = "openrouter"
	}
	m := SettingsModel{
		cfg:      cfg,
		provider: provider,
		orKey:    secretInput("sk-or-...", cfg.OpenRouterKey),
		dsKey:    secretInput("sk-...", cfg.DeepSeekKey),
		editIdx:  -1,
	}
	m.inputs[gfName] = plainInput("e.g. Office GitLab", 60)
	m.inputs[gfURL] = plainInput("https://gitlab.example.com", 200)
	m.inputs[gfToken] = secretInput("token", "")
	m.inputs[gfPath] = plainInput("~/Projects/company", 300)
	m.inputs[gfDepth] = plainInput("4", 2)
	m.inputs[gfDepth].Width = 4
	return m
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	switch m.step {
	case settingsHub:
		if isKey {
			return m.updateHub(k)
		}
	case settingsLLM:
		return m.updateLLM(msg)
	case settingsGitList:
		if isKey {
			return m.updateGitList(k)
		}
	case settingsGitKind:
		if isKey {
			return m.updateGitKind(k)
		}
	case settingsGitForm:
		return m.updateGitForm(msg)
	}
	return m, nil
}

// ── hub ──────────────────────────────────────────────────────────────────────

const hubRows = 3

func (m SettingsModel) updateHub(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "ctrl+n":
		if m.cursor < hubRows-1 {
			m.cursor++
		}
	case "enter":
		switch m.cursor {
		case 0:
			return m, func() tea.Msg { return ScreenMsg{Screen: screenBind} }
		case 1:
			m.step = settingsLLM
			m.llmField = 0
			m.orKey.Blur()
			m.dsKey.Blur()
			return m, clearError
		case 2:
			m.step = settingsGitList
			m.confirmDelete = false
			return m, clearError
		}
	case "esc", "q":
		return m, func() tea.Msg { return ScreenMsg{Screen: screenMenu} }
	}
	return m, nil
}

// ── LLM ──────────────────────────────────────────────────────────────────────

func (m SettingsModel) updateLLM(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m.updateLLMInput(msg)
	}
	switch k.String() {
	case "esc":
		m.step = settingsHub
		m.orKey.SetValue(m.cfg.OpenRouterKey) // drop unsaved edits
		m.dsKey.SetValue(m.cfg.DeepSeekKey)
		return m, clearError
	case "tab", "down", "ctrl+n":
		m.focusLLM((m.llmField + 1) % 3)
		return m, nil
	case "shift+tab", "up", "ctrl+p":
		m.focusLLM((m.llmField + 2) % 3)
		return m, nil
	case " ", "left", "right":
		if m.llmField == 0 {
			if m.provider == "deepseek" {
				m.provider = "openrouter"
			} else {
				m.provider = "deepseek"
			}
			return m, nil
		}
	case "enter", "ctrl+s":
		m.cfg.LLMProvider = m.provider
		m.cfg.OpenRouterKey = strings.TrimSpace(m.orKey.Value())
		m.cfg.DeepSeekKey = strings.TrimSpace(m.dsKey.Value())
		if err := m.cfg.Save(); err != nil {
			return m, errorCmd("cannot save config: " + err.Error())
		}
		m.step = settingsHub
		return m, tea.Batch(func() tea.Msg { return ConfigChangedMsg{} }, errorCmd(""))
	}
	return m.updateLLMInput(msg)
}

func (m *SettingsModel) focusLLM(field int) {
	m.llmField = field
	m.orKey.Blur()
	m.dsKey.Blur()
	switch field {
	case 1:
		m.orKey.Focus()
	case 2:
		m.dsKey.Focus()
	}
}

func (m SettingsModel) updateLLMInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.llmField {
	case 1:
		m.orKey, cmd = m.orKey.Update(msg)
	case 2:
		m.dsKey, cmd = m.dsKey.Update(msg)
	}
	return m, cmd
}

// ── git sources ──────────────────────────────────────────────────────────────

func (m SettingsModel) updateGitList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.cfg.GitSources)
	if m.confirmDelete {
		m.confirmDelete = false
		if k.String() == "y" && m.gitCursor < n {
			removed := m.cfg.GitSources[m.gitCursor].Label()
			m.cfg.GitSources = append(m.cfg.GitSources[:m.gitCursor:m.gitCursor], m.cfg.GitSources[m.gitCursor+1:]...)
			if m.gitCursor >= len(m.cfg.GitSources) && m.gitCursor > 0 {
				m.gitCursor--
			}
			if err := m.cfg.Save(); err != nil {
				return m, errorCmd("cannot save config: " + err.Error())
			}
			return m, errorCmd("removed " + removed)
		}
		return m, clearError
	}
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.gitCursor > 0 {
			m.gitCursor--
		}
	case "down", "j", "ctrl+n":
		if m.gitCursor < n-1 {
			m.gitCursor++
		}
	case "a":
		m.step = settingsGitKind
		m.kindCursor = 0
	case "e", "enter":
		if m.gitCursor < n {
			src := m.cfg.GitSources[m.gitCursor]
			m.openGitForm(kindIndexOf(src), m.gitCursor, src)
		}
	case "d":
		if m.gitCursor < n {
			m.confirmDelete = true
			return m, errorCmd(fmt.Sprintf("remove %s? y to confirm, any other key cancels", m.cfg.GitSources[m.gitCursor].Label()))
		}
	case "esc", "q":
		m.step = settingsHub
		return m, clearError
	}
	return m, nil
}

func kindIndexOf(src config.GitSource) int {
	switch src.Kind {
	case config.GitLab:
		if src.BaseURL == "" || strings.TrimRight(src.BaseURL, "/") == config.GitLabDotCom {
			return 0
		}
		return 1
	case config.GitHub:
		return 2
	}
	return 3
}

func (m SettingsModel) updateGitKind(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.kindCursor > 0 {
			m.kindCursor--
		}
	case "down", "j", "ctrl+n":
		if m.kindCursor < len(gitKinds)-1 {
			m.kindCursor++
		}
	case "enter":
		m.openGitForm(m.kindCursor, -1, config.GitSource{})
	case "esc", "q":
		m.step = settingsGitList
	}
	return m, nil
}

// formFields lists the inputs the chosen kind asks for, in order.
func (m SettingsModel) formFields() []int {
	switch {
	case gitKinds[m.formKind].kind == config.Local:
		return []int{gfName, gfPath, gfDepth}
	case gitKinds[m.formKind].fixedURL != "":
		return []int{gfName, gfToken}
	default:
		return []int{gfName, gfURL, gfToken}
	}
}

func (m *SettingsModel) openGitForm(kind, editIdx int, src config.GitSource) {
	m.step = settingsGitForm
	m.formKind = kind
	m.editIdx = editIdx
	m.inputs[gfName].SetValue(src.Name)
	m.inputs[gfURL].SetValue(src.BaseURL)
	m.inputs[gfToken].SetValue(src.Token)
	m.inputs[gfPath].SetValue(src.Path)
	depth := ""
	if src.Depth > 0 {
		depth = strconv.Itoa(src.Depth)
	}
	m.inputs[gfDepth].SetValue(depth)
	switch {
	case gitKinds[kind].kind == config.GitHub:
		m.inputs[gfURL].Placeholder = "optional, GitHub Enterprise API URL (default " + config.GitHubAPI + ")"
	default:
		m.inputs[gfURL].Placeholder = "https://gitlab.example.com"
	}
	m.focusGitField(0)
}

func (m *SettingsModel) focusGitField(i int) {
	fields := m.formFields()
	m.formField = (i + len(fields)) % len(fields)
	for j := range m.inputs {
		m.inputs[j].Blur()
	}
	m.inputs[fields[m.formField]].Focus()
}

func (m SettingsModel) updateGitForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.step = settingsGitList
			return m, clearError
		case "tab", "down", "ctrl+n":
			m.focusGitField(m.formField + 1)
			return m, nil
		case "shift+tab", "up", "ctrl+p":
			m.focusGitField(m.formField - 1)
			return m, nil
		case "enter", "ctrl+s":
			return m.saveGitSource()
		}
	}
	var cmd tea.Cmd
	f := m.formFields()[m.formField]
	m.inputs[f], cmd = m.inputs[f].Update(msg)
	return m, cmd
}

func (m SettingsModel) draftSource() (config.GitSource, error) {
	kind := gitKinds[m.formKind]
	src := config.GitSource{
		Kind: kind.kind,
		Name: strings.TrimSpace(m.inputs[gfName].Value()),
	}
	switch kind.kind {
	case config.Local:
		src.Path = strings.TrimSpace(m.inputs[gfPath].Value())
		if d := strings.TrimSpace(m.inputs[gfDepth].Value()); d != "" {
			n, err := strconv.Atoi(d)
			if err != nil || n < 1 {
				return src, fmt.Errorf("depth must be a number between 1 and 10")
			}
			src.Depth = n
		}
	default:
		src.Token = strings.TrimSpace(m.inputs[gfToken].Value())
		src.BaseURL = strings.TrimRight(strings.TrimSpace(m.inputs[gfURL].Value()), "/")
		if kind.fixedURL != "" {
			src.BaseURL = kind.fixedURL
		}
		if kind.kind == config.GitLab && kind.fixedURL == "" && src.BaseURL == "" {
			return src, fmt.Errorf("instance URL is required for self-hosted GitLab")
		}
	}
	return src, src.Validate()
}

func (m SettingsModel) saveGitSource() (tea.Model, tea.Cmd) {
	src, err := m.draftSource()
	if err != nil {
		return m, errorCmd(err.Error())
	}
	if m.editIdx >= 0 && m.editIdx < len(m.cfg.GitSources) {
		m.cfg.GitSources[m.editIdx] = src
	} else {
		m.cfg.GitSources = append(m.cfg.GitSources, src)
		m.gitCursor = len(m.cfg.GitSources) - 1
	}
	if err := m.cfg.Save(); err != nil {
		return m, errorCmd("cannot save config: " + err.Error())
	}
	m.step = settingsGitList
	return m, tea.Batch(func() tea.Msg { return ConfigChangedMsg{} }, errorCmd("saved "+src.Label()))
}

// ── views ────────────────────────────────────────────────────────────────────

func mask(secret string) string {
	if secret == "" {
		return "not set"
	}
	r := []rune(secret)
	if len(r) <= 4 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}

func (m SettingsModel) View() string {
	s := WindowTitle.Render("Settings") + "\n\n"
	switch m.step {
	case settingsHub:
		s += m.viewHub()
	case settingsLLM:
		s += m.viewLLM()
	case settingsGitList:
		s += m.viewGitList()
	case settingsGitKind:
		s += m.viewGitKind()
	case settingsGitForm:
		s += m.viewGitForm()
	}
	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m SettingsModel) viewHub() string {
	asanaState := ErrorStyle.Render("not bound")
	if m.cfg.IsComplete() {
		asanaState = SuccessStyle.Render("bound") + DimStyle.Render(fmt.Sprintf(" · token %s · project %s", mask(m.cfg.AsanaToken), m.cfg.DefaultProjectGID))
	} else if m.cfg.IsBound() {
		asanaState = HighlightStyle.Render("token set, pick a project")
	}

	llmKey := m.cfg.OpenRouterKey
	if m.provider == "deepseek" {
		llmKey = m.cfg.DeepSeekKey
	}
	llmState := DimStyle.Render(m.provider+" · key ") + ValueStyle.Render(mask(llmKey))

	var names []string
	for _, g := range m.cfg.GitSources {
		names = append(names, g.Label())
	}
	gitState := DimStyle.Render("none — add GitLab, GitHub or a local folder")
	if len(names) > 0 {
		gitState = ValueStyle.Render(fmt.Sprintf("%d: ", len(names))) + DimStyle.Render(strings.Join(names, ", "))
	}

	rows := []struct{ title, desc, state string }{
		{"Asana", "token, workspace, default project, custom fields", asanaState},
		{"LLM", "provider and API keys for planning and descriptions", llmState},
		{"Git sources", "where Task Composer imports your commits from", gitState},
	}
	var b strings.Builder
	for i, r := range rows {
		cur, style := "  ", MenuItem
		if i == m.cursor {
			cur, style = "> ", MenuItemSelected
		}
		b.WriteString(style.Render(cur+r.title) + "\n")
		b.WriteString("      " + DimStyle.Render(r.desc) + "\n")
		b.WriteString("      " + r.state + "\n\n")
	}
	b.WriteString(DimStyle.Render("↑/k ↓/j move • enter open • esc back"))
	return b.String()
}

func (m SettingsModel) viewLLM() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("LLM") + "\n\n")
	label := func(i int, s string) string {
		if m.llmField == i {
			return HighlightStyle.Render("> " + s)
		}
		return LabelStyle.Render("  " + s)
	}
	or, ds := "[ ] openrouter", "[ ] deepseek"
	if m.provider == "deepseek" {
		ds = "[x] deepseek"
	} else {
		or = "[x] openrouter"
	}
	b.WriteString(label(0, "Provider") + "\n  " + ValueStyle.Render(or+"   "+ds) + "\n\n")
	b.WriteString(label(1, "OpenRouter API key") + "\n" + m.orKey.View() + "\n\n")
	b.WriteString(label(2, "DeepSeek API key") + "\n" + m.dsKey.View() + "\n\n")
	b.WriteString(DimStyle.Render("tab/↑↓ switch field • space switch provider • enter save • esc cancel"))
	return b.String()
}

func (m SettingsModel) viewGitList() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Git sources") + "\n\n")
	if len(m.cfg.GitSources) == 0 {
		b.WriteString(DimStyle.Render("No sources yet. Task Composer imports your commits from these.") + "\n\n")
	}
	for i, g := range m.cfg.GitSources {
		cur, style := "  ", ValueStyle
		if i == m.gitCursor {
			cur, style = "> ", MenuItemSelected.PaddingLeft(0)
		}
		var detail string
		switch g.Kind {
		case config.Local:
			depth := g.Depth
			if depth == 0 {
				depth = 4
			}
			detail = fmt.Sprintf("local · %s · depth %d", g.Path, depth)
		case config.GitHub:
			api := g.BaseURL
			if api == "" {
				api = config.GitHubAPI
			}
			detail = fmt.Sprintf("github · %s · token %s", api, mask(g.Token))
		default:
			detail = fmt.Sprintf("gitlab · %s · token %s", g.BaseURL, mask(g.Token))
		}
		b.WriteString(style.Render(cur+padRight(g.Label(), 28)) + "  " + DimStyle.Render(detail) + "\n")
	}
	b.WriteString("\n" + DimStyle.Render("a add • e/enter edit • d remove • esc back"))
	return b.String()
}

func (m SettingsModel) viewGitKind() string {
	var b strings.Builder
	b.WriteString(HeaderStyle.Render("Add git source") + "\n\n")
	for i, k := range gitKinds {
		cur, style := "  ", MenuItem
		if i == m.kindCursor {
			cur, style = "> ", MenuItemSelected
		}
		b.WriteString(style.Render(cur+k.label) + "\n")
		b.WriteString("      " + DimStyle.Render(k.hint) + "\n\n")
	}
	b.WriteString(DimStyle.Render("↑/k ↓/j move • enter choose • esc back"))
	return b.String()
}

func (m SettingsModel) viewGitForm() string {
	kind := gitKinds[m.formKind]
	var b strings.Builder
	title := "Add " + kind.label
	if m.editIdx >= 0 {
		title = "Edit " + kind.label
	}
	b.WriteString(HeaderStyle.Render(title) + "\n")
	b.WriteString(DimStyle.Render(kind.hint) + "\n\n")
	if kind.fixedURL != "" {
		b.WriteString(LabelStyle.Render("  URL  ") + ValueStyle.Render(kind.fixedURL) + "\n\n")
	}
	names := map[int]string{
		gfName:  "Name (optional)",
		gfURL:   "Instance URL",
		gfToken: "Personal access token",
		gfPath:  "Folder",
		gfDepth: "Repo search depth (default 4)",
	}
	if kind.kind == config.GitHub {
		names[gfURL] = "API URL (optional, GitHub Enterprise)"
	}
	for i, f := range m.formFields() {
		label := LabelStyle.Render("  " + names[f])
		if i == m.formField {
			label = HighlightStyle.Render("> " + names[f])
		}
		b.WriteString(label + "\n" + m.inputs[f].View() + "\n\n")
	}
	b.WriteString(DimStyle.Render("tab/↑↓ switch field • enter save • esc cancel"))
	return b.String()
}
