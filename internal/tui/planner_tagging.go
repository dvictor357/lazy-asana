package tui

import (
	"strings"

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

// openTagging shows the tag picker for one task (target >= 0) or for the
// batch default (target == -1), loading the field options once.
func (m *PlannerModel) openTagging(target int) tea.Cmd {
	m.tagTarget = target
	m.step = plannerStepTagging
	m.taggingSection = 0
	m.taggingCursor = 0
	if target >= 0 {
		t := m.tasks[target]
		if t.HasTags {
			m.selectedEntities = copySet(t.Entities)
			m.selectedProducts = copySet(t.Products)
		} else {
			m.selectedEntities = copySet(m.batchEntities)
			m.selectedProducts = copySet(m.batchProducts)
		}
	} else {
		m.selectedEntities = copySet(m.batchEntities)
		m.selectedProducts = copySet(m.batchProducts)
	}
	if m.tagOptionsLoaded {
		return nil
	}
	m.taggingLoading = true
	return m.fetchTagOptions() // callers start the spinner
}

// closeTagging stores the picked tags and moves on: back to Review for a
// single task, on to Confirm for the batch default.
func (m *PlannerModel) closeTagging() {
	if m.tagTarget >= 0 && m.tagTarget < len(m.tasks) {
		t := &m.tasks[m.tagTarget]
		t.Entities = copySet(m.selectedEntities)
		t.Products = copySet(m.selectedProducts)
		t.HasTags = true
		m.tagTarget = -1
		m.step = plannerStepReview
		return
	}
	m.batchEntities = copySet(m.selectedEntities)
	m.batchProducts = copySet(m.selectedProducts)
	m.tagTarget = -1
	m.step = plannerStepConfirm
}

func (m *PlannerModel) cancelTagging() {
	m.tagTarget = -1
	m.taggingLoading = false
	m.step = plannerStepReview
}

// needsBatchTags reports whether any unsent task falls back to the batch
// default, i.e. has no tags of its own.
func (m PlannerModel) needsBatchTags() bool {
	for _, t := range m.tasks {
		if !t.submitted() && !t.HasTags {
			return true
		}
	}
	return false
}

// tagsFor returns the entity and product selections that apply to a task.
func (m PlannerModel) tagsFor(t PlanTask) (entities, products map[string]bool) {
	if t.HasTags {
		return t.Entities, t.Products
	}
	return m.batchEntities, m.batchProducts
}

// tagLabel renders a task's tags as option names, e.g. "elfo · elfo A2P".
func (m PlannerModel) tagLabel(t PlanTask) string {
	ents, prods := m.tagsFor(t)
	var names []string
	for _, opts := range []struct {
		sel  map[string]bool
		opts []asana.EnumOption
	}{{ents, m.entityOptions}, {prods, m.productOptions}} {
		for _, o := range opts.opts {
			if opts.sel[o.GID] {
				names = append(names, o.Name)
			}
		}
	}
	if len(names) == 0 {
		return "no tags"
	}
	return strings.Join(names, " · ")
}

func (m PlannerModel) hasTagFields() bool {
	return m.cfg.EntityFieldGID != "" || m.cfg.ProductFieldGID != ""
}

func copySet(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		if v {
			out[k] = true
		}
	}
	return out
}

func (m *PlannerModel) updateTagging(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.taggingLoading {
		switch t := msg.(type) {
		case tagOptionsResult:
			m.taggingLoading = false
			m.entityOptions = t.entityOptions
			m.productOptions = t.productOptions
			if t.err != "" && len(t.entityOptions) == 0 && len(t.productOptions) == 0 {
				errMsg := "cannot load tag options: " + t.err
				if m.tagTarget >= 0 {
					m.cancelTagging()
				} else {
					// batch: proceed — user can still submit without tags
					m.step = plannerStepConfirm
				}
				return *m, func() tea.Msg { return ErrorMsg{Message: errMsg} }
			}
			m.tagOptionsLoaded = true
		case tea.KeyMsg:
			if t.String() == "esc" {
				m.cancelTagging()
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
	case "tab", "shift+tab", "ctrl+f", "ctrl+b":
		m.taggingSection = 1 - m.taggingSection
		m.taggingCursor = 0

	case "up", "k", "ctrl+p":
		if m.taggingCursor > 0 {
			m.taggingCursor--
		}

	case "down", "j", "ctrl+n":
		if m.taggingCursor < len(currentOptions)-1 {
			m.taggingCursor++
		}

	case " ":
		if m.taggingCursor < len(currentOptions) {
			m.toggleTag(currentOptions[m.taggingCursor].GID)
		}

	case "enter":
		m.closeTagging()

	case "esc":
		m.cancelTagging()
	}

	return *m, nil
}

func (m *PlannerModel) currentTagOptions() []asana.EnumOption {
	if m.taggingSection == 1 {
		return m.productOptions
	}
	return m.entityOptions
}

// toggleTag flips an option. Single-select ("enum") fields keep at most one
// option, because Asana accepts only one value for them.
func (m *PlannerModel) toggleTag(gid string) {
	sel, fieldType := &m.selectedEntities, m.cfg.EntityFieldType
	if m.taggingSection == 1 {
		sel, fieldType = &m.selectedProducts, m.cfg.ProductFieldType
	}
	if *sel == nil {
		*sel = map[string]bool{}
	}
	if (*sel)[gid] {
		delete(*sel, gid)
		return
	}
	if fieldType != "multi_enum" {
		*sel = map[string]bool{}
	}
	(*sel)[gid] = true
}
