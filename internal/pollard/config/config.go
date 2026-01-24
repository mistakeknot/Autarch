// Package config handles Pollard configuration for research agents and sources.
package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the Pollard configuration
type Config struct {
	Speed    string                  `yaml:"speed,omitempty"` // slow, medium, fast
	Hunters  map[string]HunterConfig `yaml:"hunters"`
	Linking  LinkingConfig           `yaml:"linking,omitempty"`
	Defaults DefaultsConfig          `yaml:"defaults,omitempty"`
}

// HunterConfig defines a research hunter
type HunterConfig struct {
	Enabled    bool           `yaml:"enabled"`
	Interval   string         `yaml:"interval,omitempty"`  // e.g., "6h", "2h", "15m"
	Schedule   string         `yaml:"schedule,omitempty"`  // legacy: daily, weekly
	Queries    []string       `yaml:"queries,omitempty"`
	Categories []string       `yaml:"categories,omitempty"` // for arXiv
	MinStars   int            `yaml:"min_stars,omitempty"`  // for GitHub
	MinPoints  int            `yaml:"min_points,omitempty"` // for HackerNews
	MaxResults int            `yaml:"max_results,omitempty"`
	Targets    []TargetConfig `yaml:"targets,omitempty"` // for competitor tracker
	Output     string         `yaml:"output"`
}

// TargetConfig defines a target for competitor tracking
type TargetConfig struct {
	Name      string `yaml:"name"`
	Changelog string `yaml:"changelog,omitempty"`
	Docs      string `yaml:"docs,omitempty"`
	GitHub    string `yaml:"github,omitempty"`
}

// LinkingConfig controls how insights are linked to features/epics
type LinkingConfig struct {
	Mode                string  `yaml:"mode"`                 // manual, suggest, auto
	ConfidenceThreshold float64 `yaml:"confidence_threshold"` // for auto mode
}

// DefaultsConfig holds default values
type DefaultsConfig struct {
	MaxResults int    `yaml:"max_results,omitempty"`
	Interval   string `yaml:"interval,omitempty"`
}

// Load reads the config from a project's .pollard/config.yaml
func Load(projectPath string) (*Config, error) {
	configPath := filepath.Join(projectPath, ".pollard", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Apply defaults for missing values
	cfg.applyDefaults()
	return &cfg, nil
}

// applyDefaults fills in missing configuration values.
func (c *Config) applyDefaults() {
	if c.Speed == "" {
		c.Speed = "slow"
	}
	if c.Defaults.MaxResults == 0 {
		c.Defaults.MaxResults = 50
	}
	if c.Defaults.Interval == "" {
		c.Defaults.Interval = "6h"
	}
	if c.Linking.Mode == "" {
		c.Linking.Mode = "suggest"
	}
	if c.Linking.ConfidenceThreshold == 0 {
		c.Linking.ConfidenceThreshold = 0.8
	}

	for name, hunter := range c.Hunters {
		if hunter.MaxResults == 0 {
			hunter.MaxResults = c.Defaults.MaxResults
		}
		if hunter.Interval == "" {
			hunter.Interval = c.Defaults.Interval
		}
		c.Hunters[name] = hunter
	}
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

// GetInterval parses the interval string for a hunter.
func (c *Config) GetInterval(hunterName string) time.Duration {
	hunter, ok := c.Hunters[hunterName]
	if !ok {
		return parseInterval(c.Defaults.Interval)
	}
	if hunter.Interval != "" {
		return parseInterval(hunter.Interval)
	}
	// Legacy schedule support
	switch hunter.Schedule {
	case "daily":
		return 24 * time.Hour
	case "weekly":
		return 7 * 24 * time.Hour
	case "hourly":
		return time.Hour
	default:
		return parseInterval(c.Defaults.Interval)
	}
}

func parseInterval(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 6 * time.Hour // default fallback
	}
	return d
}

// HunterEnabled checks if a hunter is enabled.
func (c *Config) HunterEnabled(name string) bool {
	hunter, ok := c.Hunters[name]
	if !ok {
		return false
	}
	return hunter.Enabled
}

// DefaultConfig returns a default Pollard configuration
func DefaultConfig() *Config {
	return &Config{
		Speed: "slow",
		Hunters: map[string]HunterConfig{
			"github-scout": {
				Enabled:  true,
				Interval: "6h",
				Queries: []string{
					"topic:cli topic:tui language:go stars:>100",
					"topic:agent-orchestration created:>2025-01-01",
					"topic:mcp-server language:typescript",
				},
				MaxResults: 50,
				MinStars:   100,
				Output:     "sources/github/",
			},
			"trend-watcher": {
				Enabled:  true,
				Interval: "2h",
				Queries: []string{
					"AI agents",
					"LLM tools",
					"developer experience",
				},
				MinPoints:  50,
				MaxResults: 50,
				Output:     "insights/trends/",
			},
			"research-scout": {
				Enabled:  true,
				Interval: "4h",
				Categories: []string{
					"cs.AI",
					"cs.CL",
					"cs.SE",
					"cs.HC",
				},
				Queries: []string{
					"LLM agents",
					"code generation",
					"developer tools",
				},
				MaxResults: 50,
				Output:     "sources/research/",
			},
			"competitor-tracker": {
				Enabled:  true,
				Interval: "24h",
				Targets: []TargetConfig{
					{
						Name:      "Cursor",
						Changelog: "https://changelog.cursor.sh/",
						Docs:      "https://docs.cursor.com/",
					},
					{
						Name:      "Windsurf",
						Changelog: "https://codeium.com/changelog",
					},
					{
						Name:      "Aider",
						Changelog: "https://aider.chat/HISTORY.html",
						GitHub:    "paul-gauthier/aider",
					},
				},
				Output: "insights/competitive/",
			},
		},
		Linking: LinkingConfig{
			Mode:                "suggest",
			ConfidenceThreshold: 0.8,
		},
		Defaults: DefaultsConfig{
			MaxResults: 50,
			Interval:   "6h",
		},
	}
}

// GetHunterConfig returns config for a specific hunter.
func (c *Config) GetHunterConfig(name string) (HunterConfig, bool) {
	h, ok := c.Hunters[name]
	return h, ok
}

// EnabledHunters returns names of all enabled hunters.
func (c *Config) EnabledHunters() []string {
	var result []string
	for name, h := range c.Hunters {
		if h.Enabled {
			result = append(result, name)
		}
	}
	return result
}
