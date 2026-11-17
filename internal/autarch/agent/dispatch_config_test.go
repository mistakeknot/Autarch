package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateWithOutputRejectsConfiguredBackend(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "api")
	_, err := (&Agent{Type: "test-no-launch", explicitSelection: true}).GenerateWithOutput(context.Background(), GenerateRequest{Prompt: "hello"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported dispatch backend") {
		t.Fatalf("dispatch did not apply env: %v", err)
	}
}

func TestExplicitAgentBeatsProjectAndEnv(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".gurgeh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gurgeh", "agents.toml"), []byte("[dispatch]\npreferred_agent = 'codex'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	if cfg := (&Agent{}).dispatchConfig(root); cfg.PreferredAgent != "codex" {
		t.Fatalf("project preference lost: %+v", cfg)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "claude")
	if cfg := (&Agent{}).dispatchConfig(root); cfg.PreferredAgent != "claude" {
		t.Fatalf("env preference lost: %+v", cfg)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	cfg := (&Agent{Type: TypeClaude, explicitSelection: true}).dispatchConfig(root)
	if cfg.PreferredAgent != "claude" {
		t.Fatalf("explicit selection lost: %+v", cfg)
	}
}

func TestAutoDetectedAgentAllowsPreferences(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	// DetectAgent returns an agent type without an explicit selector choice.
	auto := &Agent{Type: TypeClaude}
	if cfg := auto.dispatchConfig(t.TempDir()); cfg.PreferredAgent != "codex" {
		t.Fatalf("auto-detected default masked environment preference: %+v", cfg)
	}
}

func TestAutoDetectedAgentUsesProjectThenDefault(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	root := t.TempDir()
	auto := &Agent{Type: TypeClaude}
	if got := auto.dispatchConfig(root).PreferredAgent; got != "claude" {
		t.Fatalf("default = %q", got)
	}
	if err := os.Mkdir(filepath.Join(root, ".gurgeh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gurgeh", "agents.toml"), []byte("[dispatch]\npreferred_agent = 'codex'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := auto.dispatchConfig(root).PreferredAgent; got != "codex" {
		t.Fatalf("project = %q", got)
	}
}

func TestAutoDetectedAgentErrorNamesConfiguredAgent(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "test-no-launch")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	_, err := (&Agent{Type: TypeClaude}).GenerateWithOutput(context.Background(), GenerateRequest{}, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "test-no-launch execution failed:") {
		t.Fatalf("wrong agent in error: %v", err)
	}
}

func TestDetectionDistinguishesNamedSelection(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nprintf 'test-version\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	auto, err := DetectAgent()
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := DetectAgentByName("claude", nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if got := auto.dispatchConfig(root).PreferredAgent; got != "codex" {
		t.Fatalf("auto = %q", got)
	}
	if got := explicit.dispatchConfig(root).PreferredAgent; got != "claude" {
		t.Fatalf("explicit = %q", got)
	}
}
