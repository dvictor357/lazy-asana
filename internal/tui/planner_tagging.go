package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
)

type tagOptionsResult struct {
	entityOptions  []asana.EnumOption
	productOptions []asana.EnumOption
	err            string
}

func (m *PlannerModel) fetchTagOptions() tea.Cmd {
	asanaCli := m.asanaCli
	cfg := m.cfg
	return func() tea.Msg {
		if asanaCli == nil || cfg.DefaultProjectGID == "" {
			return tagOptionsResult{}
		}
		settings, err := asanaCli.GetCustomFieldSettings(cfg.DefaultProjectGID)
		if err != nil {
			return tagOptionsResult{err: err.Error()}
		}
		var entityOpts, productOpts []asana.EnumOption
		for _, s := range settings {
			switch s.CustomField.GID {
			case cfg.EntityFieldGID:
				entityOpts = s.CustomField.EnumOptions
			case cfg.ProductFieldGID:
				productOpts = s.CustomField.EnumOptions
			}
		}
		return tagOptionsResult{entityOptions: entityOpts, productOptions: productOpts}
	}
}

func (m *PlannerModel) updateTagging(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.taggingLoading {
		switch t := msg.(type) {
		case tagOptionsResult:
			m.taggingLoading = false
			m.entityOptions = t.entityOptions
			m.productOptions = t.productOptions
			// on error just proceed — user can still submit without tags
			if t.err != "" && len(t.entityOptions) == 0 && len(t.productOptions) == 0 {
				m.step = plannerStepConfirm
			}
		}
		return *m, nil
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	currentOptions := m.currentTagOptions()

	switch k.String() {
	case "tab", "shift+tab":
		m.taggingSection = 1 - m.taggingSection
		m.taggingCursor = 0

	case "up", "k":
		if m.taggingCursor > 0 {
			m.taggingCursor--
		}

	case "down", "j":
		if m.taggingCursor < len(currentOptions)-1 {
			m.taggingCursor++
		}

	case " ":
		if m.taggingCursor < len(currentOptions) {
			m.toggleTag(currentOptions[m.taggingCursor].GID)
		}

	case "enter":
		m.step = plannerStepConfirm

	case "esc":
		m.step = plannerStepReview
	}

	return *m, nil
}

func (m *PlannerModel) currentTagOptions() []asana.EnumOption {
	if m.taggingSection == 1 {
		return m.productOptions
	}
	return m.entityOptions
}

func (m *PlannerModel) toggleTag(gid string) {
	if m.taggingSection == 0 {
		if m.selectedEntities == nil {
			m.selectedEntities = map[string]bool{}
		}
		m.selectedEntities[gid] = !m.selectedEntities[gid]
	} else {
		if m.selectedProducts == nil {
			m.selectedProducts = map[string]bool{}
		}
		m.selectedProducts[gid] = !m.selectedProducts[gid]
	}
}
