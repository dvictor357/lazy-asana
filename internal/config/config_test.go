package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// First load should return empty config (no file yet)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() on empty dir: %v", err)
	}
	if cfg.AsanaToken != "" {
		t.Error("expected empty token on fresh load")
	}

	// Populate and save
	cfg.AsanaToken = "pat-test-token"
	cfg.OpenRouterKey = "sk-or-test"
	cfg.DeepSeekKey = "sk-ds-test"
	cfg.LLMProvider = "deepseek"
	cfg.DefaultWorkspaceGID = "ws-123"
	cfg.DefaultProjectGID = "proj-456"
	cfg.ActualHoursFieldGID = "cf-hours"
	cfg.EntityFieldGID = "cf-entity"
	cfg.EntityFieldType = "enum"
	cfg.ProductFieldGID = "cf-product"
	cfg.ProductFieldType = "multi_enum"
	cfg.UserGID = "user-789"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	// Verify file was created
	path := filepath.Join(tmp, ".config", "lazy-asana", "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", path)
	}

	// Load it back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() after save: %v", err)
	}

	if loaded.AsanaToken != "pat-test-token" {
		t.Errorf("AsanaToken = %q, want %q", loaded.AsanaToken, "pat-test-token")
	}
	if loaded.OpenRouterKey != "sk-or-test" {
		t.Errorf("OpenRouterKey = %q, want %q", loaded.OpenRouterKey, "sk-or-test")
	}
	if loaded.DeepSeekKey != "sk-ds-test" {
		t.Errorf("DeepSeekKey = %q, want %q", loaded.DeepSeekKey, "sk-ds-test")
	}
	if loaded.LLMProvider != "deepseek" {
		t.Errorf("LLMProvider = %q, want %q", loaded.LLMProvider, "deepseek")
	}
	if loaded.DefaultWorkspaceGID != "ws-123" {
		t.Errorf("DefaultWorkspaceGID = %q, want %q", loaded.DefaultWorkspaceGID, "ws-123")
	}
	if loaded.DefaultProjectGID != "proj-456" {
		t.Errorf("DefaultProjectGID = %q, want %q", loaded.DefaultProjectGID, "proj-456")
	}
	if loaded.ActualHoursFieldGID != "cf-hours" {
		t.Errorf("ActualHoursFieldGID = %q, want %q", loaded.ActualHoursFieldGID, "cf-hours")
	}
	if loaded.EntityFieldGID != "cf-entity" {
		t.Errorf("EntityFieldGID = %q, want %q", loaded.EntityFieldGID, "cf-entity")
	}
	if loaded.EntityFieldType != "enum" {
		t.Errorf("EntityFieldType = %q, want %q", loaded.EntityFieldType, "enum")
	}
	if loaded.ProductFieldGID != "cf-product" {
		t.Errorf("ProductFieldGID = %q, want %q", loaded.ProductFieldGID, "cf-product")
	}
	if loaded.ProductFieldType != "multi_enum" {
		t.Errorf("ProductFieldType = %q, want %q", loaded.ProductFieldType, "multi_enum")
	}
	if loaded.UserGID != "user-789" {
		t.Errorf("UserGID = %q, want %q", loaded.UserGID, "user-789")
	}
}

func TestIsBound(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"token only", Config{AsanaToken: "pat-xxx"}, true},
		{"full config", Config{AsanaToken: "pat-xxx", DefaultWorkspaceGID: "ws-1", DefaultProjectGID: "proj-1"}, true},
		{"no token but other fields", Config{DefaultWorkspaceGID: "ws-1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsBound(); got != tt.want {
				t.Errorf("IsBound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsComplete(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"token only", Config{AsanaToken: "pat-xxx"}, false},
		{"token and workspace", Config{AsanaToken: "pat-xxx", DefaultWorkspaceGID: "ws-1"}, false},
		{"token and project", Config{AsanaToken: "pat-xxx", DefaultProjectGID: "proj-1"}, false},
		{"all three", Config{AsanaToken: "pat-xxx", DefaultWorkspaceGID: "ws-1", DefaultProjectGID: "proj-1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"empty", Config{}, true},
		{"token set", Config{AsanaToken: "pat-xxx"}, false},
		{"full config", Config{
			AsanaToken: "pat-xxx", DefaultWorkspaceGID: "ws-1", DefaultProjectGID: "proj-1",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadNonexistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	if cfg.AsanaToken != "" {
		t.Error("expected empty config from nonexistent file")
	}
}
