package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigHasAgents(t *testing.T) {
	if !strings.Contains(DefaultConfigToml, "[agents.codex]") {
		t.Fatalf("expected codex agent profile")
	}
}

func TestDefaultConfigHasValidationMode(t *testing.T) {
	if !strings.Contains(DefaultConfigToml, "validation_mode = \"soft\"") {
		t.Fatalf("expected validation_mode default")
	}
}

func TestLoadConfigReadsValidationMode(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".praude")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(path, []byte("validation_mode = \"hard\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ValidationMode != "hard" {
		t.Fatalf("expected hard mode, got %q", cfg.ValidationMode)
	}
}

func TestLoadConfigMergesGlobalAgents(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, ".praude")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("validation_mode = \"soft\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalDir := filepath.Join(configHome, "vauxpraudemonium")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "agents.toml"), []byte("[targets.codex]\ncommand=\"codex\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["codex"].Command != "codex" {
		t.Fatalf("expected codex from global config")
	}
}
