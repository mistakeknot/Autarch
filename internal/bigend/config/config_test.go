package config

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadDropsBlankScanRoots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[discovery]\nscan_roots = [\"\", \" \\t\\n\", \"~/projects\", \" project \"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(os.Getenv("HOME"), "projects"), " project "}
	if !reflect.DeepEqual(cfg.Discovery.ScanRoots, want) {
		t.Fatalf("scan roots = %q, want %q", cfg.Discovery.ScanRoots, want)
	}
	warnings := 0
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record struct {
			Level string
			Msg   string
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Level == "WARN" && record.Msg == "empty scan root in config ignored" {
			warnings++
		}
	}
	if warnings != 2 {
		t.Fatalf("got %d blank-root warnings, want 2", warnings)
	}
}

// An explicit list overrides the initialized default, even if filtering empties it.
func TestLoadAllBlankScanRoots(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[discovery]\nscan_roots = [\"\", \" \\t\\n\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Discovery.ScanRoots) != 0 {
		t.Fatalf("scan roots = %q, want empty explicit override", cfg.Discovery.ScanRoots)
	}
}
