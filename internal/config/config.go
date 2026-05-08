package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	AsanaToken          string `json:"asana_token,omitempty"`
	OpenRouterKey       string `json:"openrouter_key,omitempty"`
	DeepSeekKey         string `json:"deepseek_key,omitempty"`
	LLMProvider         string `json:"llm_provider,omitempty"`
	DefaultWorkspaceGID string `json:"default_workspace_gid,omitempty"`
	DefaultProjectGID   string `json:"default_project_gid,omitempty"`
	ActualHoursFieldGID string `json:"actual_hours_field_gid,omitempty"`
	EntityFieldGID      string `json:"entity_field_gid,omitempty"`
	EntityFieldType     string `json:"entity_field_type,omitempty"`
	ProductFieldGID     string `json:"product_field_gid,omitempty"`
	ProductFieldType    string `json:"product_field_type,omitempty"`
	UserGID             string `json:"user_gid,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "lazy-asana")
	return filepath.Join(dir, "config.json"), nil
}

func ensureDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "lazy-asana")
	return os.MkdirAll(dir, 0700)
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	if err := ensureDir(); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *Config) IsBound() bool {
	return c.AsanaToken != ""
}

func (c *Config) IsComplete() bool {
	return c.AsanaToken != "" &&
		c.DefaultWorkspaceGID != "" &&
		c.DefaultProjectGID != ""
}

func (c *Config) Validate() error {
	if c.AsanaToken == "" {
		return fmt.Errorf("asana token not set")
	}
	return nil
}
