package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuItem struct {
	title     string
	desc      string
	screen    screen
	needsBind bool
	needsLLM  bool
}

type MenuModel struct {
	items  []menuItem
	cursor int
	bound  bool
	hasLLM bool
}

func NewMenu(bound, hasLLM bool) MenuModel {
	items := []menuItem{
		{title: "Insights", desc: "Hours by day and week, and move tasks to another project", screen: screenInsights, needsBind: true},
		{title: "Task Composer", desc: "Compose, tag and submit tasks to Asana (LLM planning optional)", screen: screenPlanner, needsBind: true},
		{title: "Settings", desc: "Asana account, LLM keys and git sources", screen: screenSettings},
	}
	return MenuModel{items: items, bound: bound, hasLLM: hasLLM}
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up", "k", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "ctrl+n":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		item := m.items[m.cursor]
		if item.needsBind && !m.bound {
			return m, errorCmd("set up your Asana account in Settings first")
		}
		if item.needsLLM && !m.hasLLM {
			return m, errorCmd("no LLM key configured — add one in Settings → LLM")
		}
		return m, func() tea.Msg { return ScreenMsg{Screen: item.screen} }
	case "q":
		return m, tea.Quit
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
