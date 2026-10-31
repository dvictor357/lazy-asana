package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dvictor357/lazy-asana/internal/config"
)

// tempHome keeps Save() away from the real ~/.config/lazy-asana.
func tempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func sKey(t *testing.T, m SettingsModel, keys ...string) (SettingsModel, []tea.Msg) {
	t.Helper()
	var msgs []tea.Msg
	for _, k := range keys {
		var cmd tea.Cmd
		r, cmd := m.Update(keyMsg(k))
		m = r.(SettingsModel)
		msgs = append(msgs, collect(cmd)...)
	}
	return m, msgs
}

// collect runs a command (expanding batches) and returns every message.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, collect(c)...)
		}
		return out
	}
	return []tea.Msg{cmd()}
}

func lastError(msgs []tea.Msg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if e, ok := msgs[i].(ErrorMsg); ok && e.Message != "" {
			return e.Message
		}
	}
	return ""
}

func hasConfigChanged(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ConfigChangedMsg); ok {
			return true
		}
	}
	return false
}

func TestMenuHasSettingsNotChat(t *testing.T) {
	for _, it := range NewMenu(true, true).items {
		if it.title == "Chat with AI" || it.title == "Bind Account" {
			t.Fatalf("menu still lists %q", it.title)
		}
	}
	last := NewMenu(false, false).items[len(NewMenu(false, false).items)-1]
	if last.screen != screenSettings || last.needsBind {
		t.Fatalf("last item = %+v, want Settings reachable without an Asana account", last)
	}
}

func TestAddGitLabDotComSource(t *testing.T) {
	tempHome(t)
	cfg := &config.Config{}
	m := NewSettingsModel(cfg)
	m, _ = sKey(t, m, "j", "j", "enter", "a", "enter") // hub → Git sources → add → GitLab.com
	if m.step != settingsGitForm || len(m.formFields()) != 2 {
		t.Fatalf("step=%v fields=%v, want GitLab.com form with name+token", m.step, m.formFields())
	}
	m, msgs := sKey(t, m, "enter")
	if len(cfg.GitSources) != 0 || lastError(msgs) == "" {
		t.Fatal("saved a GitLab source without a token")
	}
	m.inputs[gfToken].SetValue("glpat-abcd1234")
	m, msgs = sKey(t, m, "enter")
	if len(cfg.GitSources) != 1 || !hasConfigChanged(msgs) {
		t.Fatalf("sources=%v, want one saved and ConfigChangedMsg", cfg.GitSources)
	}
	if got := cfg.GitSources[0]; got.Kind != config.GitLab || got.BaseURL != config.GitLabDotCom || got.Label() != "GitLab.com" {
		t.Fatalf("saved %+v", got)
	}
	loaded, err := config.Load()
	if err != nil || len(loaded.GitSources) != 1 || loaded.GitSources[0].Token != "glpat-abcd1234" {
		t.Fatalf("config on disk = %+v, %v", loaded, err)
	}
	_ = m.View()
}

func TestSelfHostedGitLabNeedsValidURL(t *testing.T) {
	tempHome(t)
	cfg := &config.Config{}
	m := NewSettingsModel(cfg)
	m.step = settingsGitKind
	m, _ = sKey(t, m, "j", "enter") // GitLab self-hosted
	m.inputs[gfToken].SetValue("tok")
	for _, bad := range []string{"", "gitlab.kantor.co.id", "ftp://x"} {
		m.inputs[gfURL].SetValue(bad)
		var msgs []tea.Msg
		m, msgs = sKey(t, m, "enter")
		if len(cfg.GitSources) != 0 || lastError(msgs) == "" {
			t.Fatalf("URL %q accepted", bad)
		}
	}
	m.inputs[gfURL].SetValue("https://gitlab.kantor.co.id/")
	m.inputs[gfName].SetValue("Kantor")
	m, _ = sKey(t, m, "enter")
	if len(cfg.GitSources) != 1 || cfg.GitSources[0].BaseURL != "https://gitlab.kantor.co.id" || kindIndexOf(cfg.GitSources[0]) != 1 {
		t.Fatalf("saved %+v, want trimmed self-hosted URL", cfg.GitSources)
	}
	_ = m.View()
}

func TestLocalSourceChecksFolderAndDepth(t *testing.T) {
	tempHome(t)
	cfg := &config.Config{}
	m := NewSettingsModel(cfg)
	m.step = settingsGitKind
	m, _ = sKey(t, m, "j", "j", "j", "enter") // Local folder
	m.inputs[gfPath].SetValue("/does/not/exist")
	m, msgs := sKey(t, m, "enter")
	if len(cfg.GitSources) != 0 || lastError(msgs) == "" {
		t.Fatal("missing folder accepted")
	}
	m.inputs[gfPath].SetValue(t.TempDir())
	m.inputs[gfDepth].SetValue("x")
	m, msgs = sKey(t, m, "enter")
	if len(cfg.GitSources) != 0 || lastError(msgs) == "" {
		t.Fatal("non-numeric depth accepted")
	}
	m.inputs[gfDepth].SetValue("2")
	m, _ = sKey(t, m, "enter")
	if len(cfg.GitSources) != 1 || cfg.GitSources[0].Depth != 2 || cfg.GitSources[0].Kind != config.Local {
		t.Fatalf("saved %+v", cfg.GitSources)
	}
}

func TestEditAndDeleteGitSource(t *testing.T) {
	tempHome(t)
	cfg := &config.Config{GitSources: []config.GitSource{
		{Kind: config.GitHub, Token: "ghp_1"},
		{Kind: config.GitLab, BaseURL: config.GitLabDotCom, Token: "glpat"},
	}}
	m := NewSettingsModel(cfg)
	m.step = settingsGitList
	m, _ = sKey(t, m, "enter") // edit GitHub
	if m.inputs[gfToken].Value() != "ghp_1" || gitKinds[m.formKind].kind != config.GitHub {
		t.Fatal("edit form not prefilled")
	}
	m.inputs[gfToken].SetValue("ghp_2")
	m, _ = sKey(t, m, "enter")
	if len(cfg.GitSources) != 2 || cfg.GitSources[0].Token != "ghp_2" {
		t.Fatalf("edit result %+v", cfg.GitSources)
	}

	m, _ = sKey(t, m, "d", "n") // cancelled
	if len(cfg.GitSources) != 2 {
		t.Fatal("d then n removed a source")
	}
	m, _ = sKey(t, m, "d", "y")
	if len(cfg.GitSources) != 1 || cfg.GitSources[0].Kind != config.GitLab {
		t.Fatalf("after delete %+v, want only GitLab", cfg.GitSources)
	}
	_ = m.View()
}

func TestLLMSettingsSaveAndCancel(t *testing.T) {
	tempHome(t)
	cfg := &config.Config{OpenRouterKey: "old"}
	m := NewSettingsModel(cfg)
	m, _ = sKey(t, m, "j", "enter") // LLM
	m, _ = sKey(t, m, " ")          // provider → deepseek
	m.dsKey.SetValue("sk-new")
	m, msgs := sKey(t, m, "enter")
	if cfg.LLMProvider != "deepseek" || cfg.DeepSeekKey != "sk-new" || cfg.OpenRouterKey != "old" || !hasConfigChanged(msgs) {
		t.Fatalf("cfg=%+v, want deepseek with new key", cfg)
	}

	m, _ = sKey(t, m, "enter") // LLM again
	m.orKey.SetValue("typo")
	m, _ = sKey(t, m, "esc")
	if cfg.OpenRouterKey != "old" || m.orKey.Value() != "old" {
		t.Fatal("esc should discard unsaved key edits")
	}
	_ = m.View()
}
