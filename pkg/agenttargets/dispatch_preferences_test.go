package agenttargets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDispatchRejectsUnsupportedBackend(t *testing.T) {
	for _, backend := range []BackendType{BackendAPI, "unknown"} {
		_, err := Dispatch(context.Background(), DispatchConfig{PreferredBackend: backend, PreferredAgent: "codex"}, "", "")
		if err == nil || !strings.Contains(err.Error(), "unsupported dispatch backend") {
			t.Fatalf("backend %q: %v", backend, err)
		}
	}
}

func TestDispatchPreferencesOverCallerDefaults(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	root := t.TempDir()
	defaults := DispatchConfig{PreferredAgent: "claude", Timeout: time.Minute}
	if got := LoadDispatchConfigWithDefaults(root, defaults); got.PreferredAgent != "claude" || got.Timeout != time.Minute {
		t.Fatalf("defaults changed: %+v", got)
	}
	if err := os.Mkdir(filepath.Join(root, ".gurgeh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gurgeh", "agents.toml"), []byte("[dispatch]\npreferred_agent = 'codex'\ntimeout = '2m'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := LoadDispatchConfigWithDefaults(root, defaults)
	if got.PreferredAgent != "codex" || got.Timeout != 2*time.Minute {
		t.Fatalf("project preferences missing: %+v", got)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "claude")
	if got := LoadDispatchConfigWithDefaults(root, defaults); got.PreferredAgent != "claude" {
		t.Fatalf("env override missing: %+v", got)
	}
}
