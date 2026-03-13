package mycroft

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config maps .autarch/mycroft/config.yaml.
type Config struct {
	Tier                Tier                    `yaml:"tier"`
	DispatchPreferences DispatchPreferences     `yaml:"dispatch_preferences"`
	T2DispatchAllowlist []AllowlistEntry        `yaml:"tier2_dispatch_allowlist"`
	DemotionTriggers    DemotionTriggers        `yaml:"demotion_triggers"`
	AgentOverrides      map[string]AgentOverride `yaml:"agent_overrides"`
	PriorityBoosts      []PriorityBoost         `yaml:"priority_boosts"`
}

// PriorityBoost adjusts a bead's effective priority during ranking.
// Boost is subtracted from priority (lower = more urgent), so a positive
// boost makes matching beads more urgent. Priority is clamped to [0, 4].
type PriorityBoost struct {
	Type  string `yaml:"type"`  // match bead type (bug, task, docs, feature)
	Boost int    `yaml:"boost"` // amount to subtract from priority (1 = one level more urgent)
}

// DispatchPreferences controls dispatch limits.
type DispatchPreferences struct {
	MaxConcurrentAgents int     `yaml:"max_concurrent_agents"`
	DailyBudget         float64 `yaml:"daily_budget"` // USD
}

// AllowlistEntry defines what work T2 can auto-dispatch.
type AllowlistEntry struct {
	Type          string `yaml:"type"`           // task, bug, docs
	MaxPriority   int    `yaml:"max_priority"`   // 0-4
	MaxComplexity string `yaml:"max_complexity"` // simple, medium, any
}

// DemotionTriggers controls when Mycroft loses autonomy.
type DemotionTriggers struct {
	FailureRateWindow        Duration `yaml:"failure_rate_window"`
	T3FailureRateThreshold   float64  `yaml:"t3_failure_rate_threshold"`
	T2FailureRateThreshold   float64  `yaml:"t2_failure_rate_threshold"`
	ConsecutiveFailureLimit  int      `yaml:"consecutive_failure_limit"`
	BudgetOvershootThreshold float64  `yaml:"budget_overshoot_threshold"`
	MinSampleSize            int      `yaml:"min_sample_size"`
}

// AgentOverride allows per-agent dispatch tuning.
type AgentOverride struct {
	MaxConcurrent int      `yaml:"max_concurrent"`
	PriorityBias  []string `yaml:"priority_bias"`
}

// Duration wraps time.Duration for YAML serialization (e.g., "24h").
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Tier: T0,
		DispatchPreferences: DispatchPreferences{
			MaxConcurrentAgents: 5,
			DailyBudget:         50.0,
		},
		DemotionTriggers: DemotionTriggers{
			FailureRateWindow:        Duration(24 * time.Hour),
			T3FailureRateThreshold:   0.25,
			T2FailureRateThreshold:   0.15,
			ConsecutiveFailureLimit:  3,
			BudgetOvershootThreshold: 1.2,
			MinSampleSize:            20,
		},
	}
}

// LoadConfig reads a config file, falling back to defaults for missing fields.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // missing file → defaults
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg, nil
}

// SaveConfig writes the config to disk.
func SaveConfig(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
