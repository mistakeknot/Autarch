// Package config handles Pollard configuration for research agents and sources.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the Pollard configuration
type Config struct {
	Agents   []AgentConfig   `yaml:"agents"`
	Overlays []string        `yaml:"overlays,omitempty"`
	Defaults DefaultsConfig  `yaml:"defaults,omitempty"`
}

// AgentConfig defines a research agent
type AgentConfig struct {
	Name     string         `yaml:"name"`
	Schedule string         `yaml:"schedule"` // daily, weekly, monthly
	Sources  []SourceConfig `yaml:"sources,omitempty"`
	Targets  []TargetConfig `yaml:"targets,omitempty"`
	Output   string         `yaml:"output"`
}

// SourceConfig defines a source for research
type SourceConfig struct {
	Type  string `yaml:"type,omitempty"`  // github, hackernews, producthunt
	Query string `yaml:"query,omitempty"` // search query
	URL   string `yaml:"url,omitempty"`   // for specific URLs
	Limit int    `yaml:"limit,omitempty"` // max results
}

// TargetConfig defines a target for tracking
type TargetConfig struct {
	URL  string `yaml:"url"`
	Type string `yaml:"type"` // changelog, blog, docs
}

// DefaultsConfig holds default values
type DefaultsConfig struct {
	SourceLimit int `yaml:"source_limit,omitempty"`
}

// Load reads the config from a project's .pollard/config.yaml
func Load(projectPath string) (*Config, error) {
	configPath := filepath.Join(projectPath, ".pollard", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes the config to a project's .pollard/config.yaml
func (c *Config) Save(projectPath string) error {
	pollardDir := filepath.Join(projectPath, ".pollard")
	if err := os.MkdirAll(pollardDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	configPath := filepath.Join(pollardDir, "config.yaml")
	return os.WriteFile(configPath, data, 0644)
}

// DefaultConfig returns a default Pollard configuration
func DefaultConfig() *Config {
	return &Config{
		Agents: []AgentConfig{
			{
				Name:     "github-scout",
				Schedule: "daily",
				Sources: []SourceConfig{
					{Type: "github", Query: "topic:cli topic:tui language:go", Limit: 50},
					{Type: "github", Query: "topic:agent-orchestration", Limit: 20},
				},
				Output: "sources/github/",
			},
			{
				Name:     "trend-watcher",
				Schedule: "daily",
				Sources: []SourceConfig{
					{Type: "hackernews", Query: "AI agents OR LLM tools"},
					{Type: "producthunt", Query: "developer-tools"},
				},
				Output: "insights/trends.yaml",
			},
		},
		Defaults: DefaultsConfig{
			SourceLimit: 50,
		},
	}
}
