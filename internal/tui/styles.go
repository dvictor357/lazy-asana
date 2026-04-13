package tui

import "github.com/charmbracelet/lipgloss"

var (
	WindowTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	Subtitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	MenuItem = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#E5E7EB"))

	MenuItemSelected = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("#7D56F4")).
				Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			MarginTop(1)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#10B981"))

	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	HighlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B"))

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Padding(1, 2)

	LabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF"))

	ValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			PaddingBottom(1)

	ChatUserStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4"))

	ChatAssistantStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981"))
)
