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
	Pipeline PipelineConfig          `yaml:"pipeline,omitempty"`
	Scoring  ScoringConfig           `yaml:"scoring,omitempty"`
}

// PipelineConfig controls the 4-stage research pipeline.
type PipelineConfig struct {
	Synthesizer SynthesizerConfig `yaml:"synthesizer,omitempty"`
	Modes       ModeConfigs       `yaml:"modes,omitempty"`
}

// SynthesizerConfig controls agent-spawned synthesis.
type SynthesizerConfig struct {
	Agent       string `yaml:"agent,omitempty"`       // Agent command (e.g., "claude", "cursor --ask")
	Parallelism int    `yaml:"parallelism,omitempty"` // Max concurrent agent instances
	Timeout     string `yaml:"timeout,omitempty"`     // Per-item timeout (e.g., "2m")
}

// ModeConfigs defines behavior for different pipeline modes.
type ModeConfigs struct {
	Quick    ModeConfig `yaml:"quick,omitempty"`
	Balanced ModeConfig `yaml:"balanced,omitempty"`
	Deep     ModeConfig `yaml:"deep,omitempty"`
}

// ModeConfig controls a single pipeline mode.
type ModeConfig struct {
	Synthesize      bool   `yaml:"synthesize"`
	SynthesizeLimit int    `yaml:"synthesize_limit,omitempty"` // 0 = all items
	FetchDepth      string `yaml:"fetch_depth,omitempty"`      // basic, standard, full
}

// ScoringConfig controls unified quality scoring.
type ScoringConfig struct {
	Weights    ScoreWeightsConfig    `yaml:"weights,omitempty"`
	HalfLives  HalfLivesConfig       `yaml:"half_lives,omitempty"`
	Thresholds ScoreThresholdsConfig `yaml:"thresholds,omitempty"`
}

// ScoreWeightsConfig defines the relative importance of scoring factors.
type ScoreWeightsConfig struct {
	Engagement float64 `yaml:"engagement,omitempty"` // points, comments, stars
	Citations  float64 `yaml:"citations,omitempty"`  // academic citations
	Recency    float64 `yaml:"recency,omitempty"`    // temporal decay
	QueryMatch float64 `yaml:"query_match,omitempty"` // title/content match
	Synthesis  float64 `yaml:"synthesis,omitempty"`  // agent analysis confidence
}

// HalfLivesConfig defines temporal decay rates.
type HalfLivesConfig struct {
	Trends   string `yaml:"trends,omitempty"`   // e.g., "168h" (7 days)
	Research string `yaml:"research,omitempty"` // e.g., "8760h" (365 days)
	Repos    string `yaml:"repos,omitempty"`    // e.g., "2160h" (90 days)
}

// ScoreThresholdsConfig defines quality level cutoffs.
type ScoreThresholdsConfig struct {
	High   float64 `yaml:"high,omitempty"`   // 0.7
	Medium float64 `yaml:"medium,omitempty"` // 0.4
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

	// New hunter-specific config fields
	Email           string   `yaml:"email,omitempty"`            // for OpenAlex polite pool
	MeSHTerms       []string `yaml:"mesh_terms,omitempty"`       // for PubMed
	DataTypes       []string `yaml:"data_types,omitempty"`       // for USDA (Foundation, SR Legacy)
	IncludeAllergens bool    `yaml:"include_allergens,omitempty"` // for USDA
	Courts          []string `yaml:"courts,omitempty"`           // for CourtListener
	DateFiledAfter  string   `yaml:"date_filed_after,omitempty"` // for CourtListener
	Indicators      []string `yaml:"indicators,omitempty"`       // for economics
	Countries       []string `yaml:"countries,omitempty"`        // for economics
	IncludeWikipedia bool    `yaml:"include_wikipedia,omitempty"` // for wiki
	IncludeWikidata  bool    `yaml:"include_wikidata,omitempty"`  // for wiki
	Languages       []string `yaml:"languages,omitempty"`        // for wiki
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

	// Pipeline defaults
	if c.Pipeline.Synthesizer.Parallelism == 0 {
		c.Pipeline.Synthesizer.Parallelism = 3
	}
	if c.Pipeline.Synthesizer.Timeout == "" {
		c.Pipeline.Synthesizer.Timeout = "2m"
	}
	// Mode defaults
	if c.Pipeline.Modes.Quick.FetchDepth == "" {
		c.Pipeline.Modes.Quick.FetchDepth = "basic"
		c.Pipeline.Modes.Quick.Synthesize = false
	}
	if c.Pipeline.Modes.Balanced.FetchDepth == "" {
		c.Pipeline.Modes.Balanced.FetchDepth = "standard"
		c.Pipeline.Modes.Balanced.Synthesize = true
		c.Pipeline.Modes.Balanced.SynthesizeLimit = 10
	}
	if c.Pipeline.Modes.Deep.FetchDepth == "" {
		c.Pipeline.Modes.Deep.FetchDepth = "full"
		c.Pipeline.Modes.Deep.Synthesize = true
		c.Pipeline.Modes.Deep.SynthesizeLimit = 0 // All items
	}

	// Scoring defaults
	if c.Scoring.Weights.Engagement == 0 {
		c.Scoring.Weights.Engagement = 0.25
	}
	if c.Scoring.Weights.Citations == 0 {
		c.Scoring.Weights.Citations = 0.20
	}
	if c.Scoring.Weights.Recency == 0 {
		c.Scoring.Weights.Recency = 0.25
	}
	if c.Scoring.Weights.QueryMatch == 0 {
		c.Scoring.Weights.QueryMatch = 0.15
	}
	if c.Scoring.Weights.Synthesis == 0 {
		c.Scoring.Weights.Synthesis = 0.15
	}
	if c.Scoring.HalfLives.Trends == "" {
		c.Scoring.HalfLives.Trends = "168h" // 7 days
	}
	if c.Scoring.HalfLives.Research == "" {
		c.Scoring.HalfLives.Research = "8760h" // 365 days
	}
	if c.Scoring.HalfLives.Repos == "" {
		c.Scoring.HalfLives.Repos = "2160h" // 90 days
	}
	if c.Scoring.Thresholds.High == 0 {
		c.Scoring.Thresholds.High = 0.7
	}
	if c.Scoring.Thresholds.Medium == 0 {
		c.Scoring.Thresholds.Medium = 0.4
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
				// Output empty - uses hunter's default: .pollard/sources/github/
			},
			"hackernews-trendwatcher": {
				Enabled:  true,
				Interval: "2h",
				Queries: []string{
					"AI agents",
					"LLM tools",
					"developer experience",
				},
				MinPoints:  50,
				MaxResults: 50,
				// Output empty - uses hunter's default: .pollard/insights/trends/
			},
			"arxiv-scout": {
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
				// Output empty - uses hunter's default: .pollard/sources/research/
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
				// Output empty - uses hunter's default: .pollard/insights/competitive/
			},
			// New general-purpose hunters (disabled by default - enable as needed)
			"openalex": {
				Enabled:  false,
				Interval: "6h",
				Queries: []string{
					"artificial intelligence",
					"machine learning",
				},
				MaxResults: 100,
			},
			"pubmed": {
				Enabled:  false,
				Interval: "6h",
				Queries: []string{
					"food allergy treatment",
					"celiac disease",
				},
				MeSHTerms: []string{
					"Food Hypersensitivity",
				},
				MaxResults: 50,
			},
			"usda-nutrition": {
				Enabled:  false,
				Interval: "24h",
				Queries: []string{
					"peanut",
					"gluten",
				},
				DataTypes: []string{
					"Foundation",
					"SR Legacy",
				},
				IncludeAllergens: true,
				MaxResults:       50,
			},
			"legal": {
				Enabled:  false,
				Interval: "24h",
				Queries: []string{
					"first amendment",
					"patent infringement",
				},
				Courts: []string{
					"scotus",
					"ca9",
				},
				DateFiledAfter: "2020-01-01",
				MaxResults:     50,
			},
			"economics": {
				Enabled:  false,
				Interval: "24h",
				Indicators: []string{
					"GDP",
					"CPI",
					"UNEMP",
				},
				Countries: []string{
					"USA",
					"GBR",
					"DEU",
				},
				MaxResults: 50,
			},
			"wiki": {
				Enabled:  false,
				Interval: "24h",
				Queries: []string{
					"democratic institutions",
					"number theory",
				},
				IncludeWikipedia: true,
				IncludeWikidata:  true,
				Languages:        []string{"en"},
				MaxResults:       20,
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

// GetSynthesizerTimeout parses the synthesizer timeout.
func (c *Config) GetSynthesizerTimeout() time.Duration {
	return parseInterval(c.Pipeline.Synthesizer.Timeout)
}

// GetHalfLife parses a half-life duration by type.
func (c *Config) GetHalfLife(halfLifeType string) time.Duration {
	switch halfLifeType {
	case "trends":
		return parseInterval(c.Scoring.HalfLives.Trends)
	case "research":
		return parseInterval(c.Scoring.HalfLives.Research)
	case "repos":
		return parseInterval(c.Scoring.HalfLives.Repos)
	default:
		return 90 * 24 * time.Hour // Default to repos
	}
}

// GetModeConfig returns configuration for a specific pipeline mode.
func (c *Config) GetModeConfig(mode string) ModeConfig {
	switch mode {
	case "quick":
		return c.Pipeline.Modes.Quick
	case "deep":
		return c.Pipeline.Modes.Deep
	default:
		return c.Pipeline.Modes.Balanced
	}
}
