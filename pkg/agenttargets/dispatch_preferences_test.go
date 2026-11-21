package agenttargets

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDispatchRejectsUnsupportedBackend(t *testing.T) {
	for _, backend := range []BackendType{BackendAPI, "unknown"} {
		_, err := Dispatch(context.Background(), DispatchConfig{PreferredBackend: backend, PreferredAgent: "test-no-launch"}, "", "")
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

func TestUnknownAgentNamesItsSource(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".gurgeh"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".gurgeh", "agents.toml")
	if err := os.WriteFile(path, []byte("[dispatch]\npreferred_agent = 'gemini'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	for _, source := range []string{path, "AUTARCH_DISPATCH_AGENT"} {
		if source == "AUTARCH_DISPATCH_AGENT" {
			t.Setenv(source, "gemini")
		}
		_, err := Dispatch(context.Background(), LoadDispatchConfig(root), "", "")
		if err == nil || !strings.Contains(err.Error(), "gemini") || !strings.Contains(err.Error(), source) {
			t.Fatalf("missing value/source: %v", err)
		}
	}
}

func TestBrokenDispatchFileWarnsAndFallsBack(t *testing.T) {
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	for _, unreadable := range []bool{false, true} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".gurgeh"), 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".gurgeh", "agents.toml")
		if unreadable {
			// A directory fails reads even for privileged test users.
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("[dispatch\n"), 0600); err != nil {
			t.Fatal(err)
		}
		var log bytes.Buffer
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
		cfg := LoadDispatchConfig(root)
		slog.SetDefault(old)
		if cfg.PreferredAgent != "" || cfg.PreferredBackend != BackendSubscriptionCLI || cfg.Timeout != 30*time.Minute {
			t.Fatalf("defaults changed: %+v", cfg)
		}
		if !strings.Contains(log.String(), path) || !strings.Contains(log.String(), "WARN") {
			t.Fatalf("missing warning: %s", log.String())
		}
	}
}
