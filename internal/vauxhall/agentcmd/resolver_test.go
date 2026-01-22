package agentcmd

import (
	"os"
	"path/filepath"
	"testing"

	vconfig "github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
)

func TestResolveCommandFallback(t *testing.T) {
	cfg := &vconfig.Config{}
	r := NewResolver(cfg)
	cmd, args := r.Resolve("claude", "/root/projects/demo")
	if cmd != "claude" {
		t.Fatalf("expected claude fallback, got %q", cmd)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}

func TestResolveUsesSharedTargets(t *testing.T) {
	cfg := &vconfig.Config{}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	configDir := filepath.Join(dir, "vauxpraudemonium")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "agents.toml")
	if err := os.WriteFile(path, []byte("[targets.custom]\ncommand=\"/bin/custom\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(cfg)
	cmd, _ := r.Resolve("custom", "")
	if cmd != "/bin/custom" {
		t.Fatalf("expected shared target, got %q", cmd)
	}
}
