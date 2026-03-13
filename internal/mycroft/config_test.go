package mycroft

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Tier != T0 {
		t.Errorf("default tier: got %v, want T0", cfg.Tier)
	}
	if cfg.DispatchPreferences.MaxConcurrentAgents != 5 {
		t.Errorf("default max_concurrent: got %d, want 5", cfg.DispatchPreferences.MaxConcurrentAgents)
	}
	if cfg.DemotionTriggers.MinSampleSize != 20 {
		t.Errorf("default min_sample_size: got %d, want 20", cfg.DemotionTriggers.MinSampleSize)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path.yaml")
	if err != nil {
		t.Fatalf("missing file should return defaults, got error: %v", err)
	}
	if cfg.Tier != T0 {
		t.Errorf("missing file tier: got %v, want T0", cfg.Tier)
	}
}

func TestLoadConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := Config{
		Tier: T1,
		DispatchPreferences: DispatchPreferences{
			MaxConcurrentAgents: 3,
			DailyBudget:         25.0,
		},
		T2DispatchAllowlist: []AllowlistEntry{
			{Type: "task", MaxPriority: 3, MaxComplexity: "medium"},
		},
		DemotionTriggers: DemotionTriggers{
			FailureRateWindow:        Duration(12 * time.Hour),
			T3FailureRateThreshold:   0.25,
			T2FailureRateThreshold:   0.15,
			ConsecutiveFailureLimit:  3,
			BudgetOvershootThreshold: 1.5,
			MinSampleSize:            10,
		},
		AgentOverrides: map[string]AgentOverride{
			"grey-area": {MaxConcurrent: 2, PriorityBias: []string{"go"}},
		},
	}

	if err := SaveConfig(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Tier != T1 {
		t.Errorf("tier: got %v, want T1", loaded.Tier)
	}
	if loaded.DispatchPreferences.MaxConcurrentAgents != 3 {
		t.Errorf("max_concurrent: got %d, want 3", loaded.DispatchPreferences.MaxConcurrentAgents)
	}
	if loaded.DispatchPreferences.DailyBudget != 25.0 {
		t.Errorf("daily_budget: got %f, want 25.0", loaded.DispatchPreferences.DailyBudget)
	}
	if len(loaded.T2DispatchAllowlist) != 1 {
		t.Fatalf("allowlist: got %d entries, want 1", len(loaded.T2DispatchAllowlist))
	}
	if loaded.T2DispatchAllowlist[0].Type != "task" {
		t.Errorf("allowlist type: got %q, want %q", loaded.T2DispatchAllowlist[0].Type, "task")
	}
	if loaded.DemotionTriggers.MinSampleSize != 10 {
		t.Errorf("min_sample_size: got %d, want 10", loaded.DemotionTriggers.MinSampleSize)
	}
	if time.Duration(loaded.DemotionTriggers.FailureRateWindow) != 12*time.Hour {
		t.Errorf("failure_rate_window: got %v, want 12h", time.Duration(loaded.DemotionTriggers.FailureRateWindow))
	}
	if override, ok := loaded.AgentOverrides["grey-area"]; !ok {
		t.Error("missing agent override for grey-area")
	} else if override.MaxConcurrent != 2 {
		t.Errorf("grey-area max_concurrent: got %d, want 2", override.MaxConcurrent)
	}
}

func TestLoadConfigPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.yaml")

	// Only set tier — everything else should be defaults.
	os.WriteFile(path, []byte("tier: 2\n"), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Tier != T2 {
		t.Errorf("tier: got %v, want T2", cfg.Tier)
	}
	// Defaults should fill in.
	if cfg.DispatchPreferences.MaxConcurrentAgents != 5 {
		t.Errorf("default max_concurrent preserved: got %d, want 5", cfg.DispatchPreferences.MaxConcurrentAgents)
	}
}
