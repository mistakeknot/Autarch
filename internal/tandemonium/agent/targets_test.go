package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTargetUsesGlobalConfig(t *testing.T) {
	root := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalDir := filepath.Join(configHome, "vauxpraudemonium")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "agents.toml"), []byte("[targets.custom]\ncommand=\"/bin/custom\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveTarget(root, "custom")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Command != "/bin/custom" {
		t.Fatalf("expected custom target")
	}
}
