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
	_, err := (&Agent{Type: "test-no-launch"}).GenerateWithOutput(context.Background(), GenerateRequest{Prompt: "hello"}, nil)
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
	cfg := (&Agent{Type: TypeClaude}).dispatchConfig(root)
	if cfg.PreferredAgent != "claude" {
		t.Fatalf("explicit selection lost: %+v", cfg)
	}
}
