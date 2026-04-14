package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuItem struct {
	title string
	desc  string
}

type MenuModel struct {
	items  []menuItem
	cursor int
	bound  bool
}

func NewMenu(bound bool) MenuModel {
	items := []menuItem{
		{title: "Bind Account", desc: "Configure Asana & OpenRouter API keys"},
		{title: "Show Week Hours", desc: "View actual hours for a given week"},
		{title: "Plan Day/Week", desc: "Plan and create tasks with LLM assistance"},
		{title: "Move Tasks", desc: "Move tasks between projects by date range"},
	}
	return MenuModel{items: items, bound: bound}
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0:
				return m, func() tea.Msg { return ScreenMsg{Screen: screenBind} }
			case 1:
				if !m.bound {
					return m, func() tea.Msg { return ErrorMsg{Message: "please bind your account first"} }
				}
				return m, func() tea.Msg { return ScreenMsg{Screen: screenHours} }
			case 2:
				if !m.bound {
					return m, func() tea.Msg { return ErrorMsg{Message: "please bind your account first"} }
				}
				return m, func() tea.Msg { return ScreenMsg{Screen: screenPlanner} }
			case 3:
				if !m.bound {
					return m, func() tea.Msg { return ErrorMsg{Message: "please bind your account first"} }
				}
				return m, func() tea.Msg { return ScreenMsg{Screen: screenMigrate} }
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m MenuModel) View() string {
	s := WindowTitle.Render("lazy-asana")
	s += Subtitle.Render("Asana task planner with LLM assistance in TUI")
	s += "\n\n"

	for i, item := range m.items {
		cursor := "  "
		style := MenuItem
		if i == m.cursor {
			cursor = "> "
			style = MenuItemSelected
		}
		s += style.Render(fmt.Sprintf("%s%s", cursor, item.title))
		s += "\n"
		s += DimStyle.Render(fmt.Sprintf("    %s", item.desc))
		s += "\n\n"
	}

	s += "\n"
	s += DimStyle.Render("↑/k up • ↓/j down • enter select • q quit")
	return lipgloss.NewStyle().Padding(2, 4).Render(s)
}
