package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dvictor357/lazy-asana/internal/llm"
)

type chatResponse struct {
	content string
	err     error
}

type ChatModel struct {
	client   *llm.Client
	history  []llm.Message
	viewport viewport.Model
	input    textarea.Model
	loading  bool
	err      string
	width    int
	height   int
}

func NewChatModel(client *llm.Client) ChatModel {
	vp := viewport.New(60, 15)

	ta := textarea.New()
	ta.Placeholder = "Ask the AI assistant..."
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.Focus()

	return ChatModel{
		client:   client,
		viewport: vp,
		input:    ta,
	}
}

func (m ChatModel) Init() tea.Cmd {
	return nil
}

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 8
		m.viewport.Height = msg.Height - 14
		m.input.SetWidth(msg.Width - 8)
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ScreenMsg{Screen: screenPlanner} }
		}
	case chatResponse:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.history = append(m.history, llm.Message{Role: "assistant", Content: msg.content})
		m.refreshViewport()
		m.input.Focus()
		return m, nil
	}

	if !m.loading {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != "" {
			k, ok := msg.(tea.KeyMsg)
			if ok && k.String() == "enter" {
				text := m.input.Value()
				m.input.Reset()
				m.history = append(m.history, llm.Message{Role: "user", Content: text})
				m.refreshViewport()
				m.loading = true
				m.err = ""
				return m, m.sendChat(text)
			}
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m ChatModel) sendChat(text string) tea.Cmd {
	history := make([]llm.Message, len(m.history))
	copy(history, m.history)
	return func() tea.Msg {
		resp, err := m.client.Chat(history)
		if err != nil {
			return chatResponse{err: err}
		}
		return chatResponse{content: resp}
	}
}

func (m *ChatModel) refreshViewport() {
	var b strings.Builder
	for _, msg := range m.history {
		switch msg.Role {
		case "user":
			b.WriteString(ChatUserStyle.Render("You: "))
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString(ChatAssistantStyle.Render("AI: "))
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		}
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func (m ChatModel) View() string {
	s := WindowTitle.Render("LLM Chat Assistant")
	s += "\n\n"
	s += m.viewport.View()
	s += "\n\n"

	if m.loading {
		s += DimStyle.Render("Thinking...")
	} else {
		s += m.input.View()
		s += "\n" + DimStyle.Render("enter send • esc back to planner")
	}

	if m.err != "" {
		s += "\n" + ErrorStyle.Render(m.err)
	}

	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}

func (m *ChatModel) GetLastResponse() string {
	for i := len(m.history) - 1; i >= 0; i-- {
		if m.history[i].Role == "assistant" {
			return m.history[i].Content
		}
	}
	return ""
}

func (m *ChatModel) SetSystemPrompt(prompt string) {
	m.history = append([]llm.Message{{Role: "system", Content: prompt}}, m.history...)
}
