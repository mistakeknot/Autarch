package agenttargets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRegistriesWithCompat(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "agents.toml")
	projectDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(filepath.Join(projectDir, ".praude"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(globalPath, []byte("[targets.codex]\ncommand=\"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".praude", "config.toml"), []byte("[agents.custom]\ncommand=\"/bin/custom\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	global, project, err := Load(globalPath, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := global.Targets["codex"]; !ok {
		t.Fatalf("expected global codex")
	}
	if _, ok := project.Targets["custom"]; !ok {
		t.Fatalf("expected project custom")
	}
}

func TestLoadDispatchConfig_NoProject(t *testing.T) {
	cfg := LoadDispatchConfig("")
	if cfg.PreferredBackend != BackendSubscriptionCLI {
		t.Errorf("preferred backend = %q, want %q", cfg.PreferredBackend, BackendSubscriptionCLI)
	}
	if cfg.Timeout != 30*time.Minute {
		t.Errorf("timeout = %v, want 30m", cfg.Timeout)
	}
}

func TestLoadDispatchConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gurgeh"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `[dispatch]
preferred_backend = "api"
preferred_agent = "codex"
timeout = "15m"
`
	if err := os.WriteFile(filepath.Join(dir, ".gurgeh", "agents.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadDispatchConfig(dir)
	if cfg.PreferredBackend != BackendAPI {
		t.Errorf("preferred backend = %q, want %q", cfg.PreferredBackend, BackendAPI)
	}
	if cfg.PreferredAgent != "codex" {
		t.Errorf("preferred agent = %q, want %q", cfg.PreferredAgent, "codex")
	}
	if cfg.Timeout != 15*time.Minute {
		t.Errorf("timeout = %v, want 15m", cfg.Timeout)
	}
}

func TestLoadDispatchConfig_EnvOverride(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")

	cfg := LoadDispatchConfig("")
	if cfg.PreferredBackend != BackendAPI {
		t.Errorf("preferred backend = %q, want %q", cfg.PreferredBackend, BackendAPI)
	}
	if cfg.PreferredAgent != "codex" {
		t.Errorf("preferred agent = %q, want %q", cfg.PreferredAgent, "codex")
	}
}

func TestLoadDispatchConfig_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".gurgeh"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `[dispatch]
preferred_backend = "subscription-cli"
preferred_agent = "claude"
`
	if err := os.WriteFile(filepath.Join(dir, ".gurgeh", "agents.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Env overrides file
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")

	cfg := LoadDispatchConfig(dir)
	if cfg.PreferredBackend != BackendAPI {
		t.Errorf("preferred backend = %q, want %q (env should override file)", cfg.PreferredBackend, BackendAPI)
	}
	if cfg.PreferredAgent != "codex" {
		t.Errorf("preferred agent = %q, want %q (env should override file)", cfg.PreferredAgent, "codex")
	}
}

func TestLoadDispatchConfig_MissingConfigDir(t *testing.T) {
	dir := t.TempDir()
	// No .gurgeh or .praude directory
	cfg := LoadDispatchConfig(dir)
	if cfg.PreferredBackend != BackendSubscriptionCLI {
		t.Errorf("expected default backend, got %q", cfg.PreferredBackend)
	}
}
