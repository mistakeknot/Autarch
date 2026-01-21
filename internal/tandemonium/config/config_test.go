package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".tandemonium")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`
[general]
max_agents = 3
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.General.MaxAgents != 3 {
		t.Fatalf("expected max_agents=3, got %d", cfg.General.MaxAgents)
	}
}

func TestLoadProjectConfigConfirmApproveDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.TUI.ConfirmApprove != true {
		t.Fatalf("expected confirm approve default true")
	}
}

func TestLoadProjectConfigReviewTargetBranch(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".tandemonium")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`
[review]
target_branch = "develop"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Review.TargetBranch != "develop" {
		t.Fatalf("expected target branch develop, got %q", cfg.Review.TargetBranch)
	}
}

func TestLoadProjectConfigCodingDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Coding.HealthCheckInterval != 0 {
		t.Fatalf("expected health interval default 0, got %d", cfg.Coding.HealthCheckInterval)
	}
	if cfg.Coding.RestartOnFailure != false {
		t.Fatalf("expected restart default false")
	}
}

func TestLoadProjectConfigLLMSummary(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".tandemonium")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`
[llm_summary]
command = "claude"
timeout_seconds = 15
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.LLMSummary.Command != "claude" {
		t.Fatalf("expected llm command claude, got %q", cfg.LLMSummary.Command)
	}
	if cfg.LLMSummary.TimeoutSeconds != 15 {
		t.Fatalf("expected timeout 15, got %d", cfg.LLMSummary.TimeoutSeconds)
	}
}
