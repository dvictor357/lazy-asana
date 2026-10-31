package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

	GitSources []GitSource `json:"git_sources,omitempty"`
}

// Git source kinds.
const (
	GitLab = "gitlab" // gitlab.com or a self-hosted instance
	GitHub = "github" // github.com or GitHub Enterprise
	Local  = "local"  // a folder of local clones, scanned like asana-recap
)

// Default API base URLs per kind.
const (
	GitLabDotCom = "https://gitlab.com"
	GitHubAPI    = "https://api.github.com"
)

// GitSource is one place to read your commits from for Task Composer import.
type GitSource struct {
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
	BaseURL string `json:"base_url,omitempty"` // gitlab/github: instance URL
	Token   string `json:"token,omitempty"`    // gitlab/github: personal access token
	Path    string `json:"path,omitempty"`     // local: root folder holding the repos
	Depth   int    `json:"depth,omitempty"`    // local: how deep to look for repos (default 4)
}

// Label is the name shown in lists, falling back to something descriptive.
func (g GitSource) Label() string {
	if g.Name != "" {
		return g.Name
	}
	switch g.Kind {
	case GitLab:
		if g.BaseURL == "" || g.BaseURL == GitLabDotCom {
			return "GitLab.com"
		}
		return "GitLab " + g.BaseURL
	case GitHub:
		return "GitHub"
	case Local:
		return "Local " + g.Path
	}
	return g.Kind
}

// Validate reports the first missing or invalid field, phrased for the form.
func (g GitSource) Validate() error {
	switch g.Kind {
	case GitLab, GitHub:
		if g.Token == "" {
			return fmt.Errorf("token is required for %s", g.Kind)
		}
		if g.BaseURL != "" {
			u, err := url.Parse(g.BaseURL)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return fmt.Errorf("base URL %q must look like https://gitlab.example.com", g.BaseURL)
			}
		}
	case Local:
		if g.Path == "" {
			return fmt.Errorf("folder path is required")
		}
		info, err := os.Stat(ExpandHome(g.Path))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("folder %q does not exist", g.Path)
		}
		if g.Depth < 0 || g.Depth > 10 {
			return fmt.Errorf("depth must be between 1 and 10")
		}
	default:
		return fmt.Errorf("unknown source kind %q", g.Kind)
	}
	return nil
}

// ExpandHome turns a leading ~ into the home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
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
