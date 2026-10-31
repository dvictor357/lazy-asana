package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/asana"
	"github.com/dvictor357/lazy-asana/internal/config"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestBindProjectSearchAcceptsJK(t *testing.T) {
	m := NewBindModel(&config.Config{})
	m.step = bindStepWorkspace
	m.workspaces = []asana.Workspace{{GID: "w"}}
	m.projects = []asana.Project{{GID: "1", Name: "Kopi Kenangan"}, {GID: "2", Name: "Jakarta Ops"}}
	m.projSearch.Focus()
	for _, k := range []string{"j", "k"} {
		r, _ := m.Update(runes(k))
		m = r.(BindModel)
	}
	if got := m.projSearch.Value(); got != "jk" {
		t.Fatalf("project search = %q, want %q", got, "jk")
	}
	r, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if r.(BindModel).projCursor != 0 { // filter "jk" matches nothing; down must not panic
		t.Fatal("cursor moved past an empty filtered list")
	}
}

func TestBindTokenAcceptsJK(t *testing.T) {
	m := NewBindModel(&config.Config{})
	for _, k := range []string{"k", "j"} {
		r, _ := m.Update(runes(k))
		m = r.(BindModel)
	}
	if got := m.asanaToken.Value(); got != "kj" {
		t.Fatalf("token = %q, want %q", got, "kj")
	}
}

func TestMigrateProjectSearchAcceptsJK(t *testing.T) {
	m := NewMigrateForTasks(&config.Config{}, nil, nil)
	m.step = migrateStepPickTarget
	m.projects = []asana.Project{{GID: "1", Name: "Jakarta"}}
	m.projSearch.Focus()
	for _, k := range []string{"j", "k"} {
		r, _ := m.Update(runes(k))
		m = r.(MigrateModel)
	}
	if got := m.projSearch.Value(); got != "jk" {
		t.Fatalf("project search = %q, want %q", got, "jk")
	}
}
