package discovery

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

func TestScanWarnsForEmptyRoots(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(projectPath, ".gurgeh"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	projects, err := NewScanner(config.DiscoveryConfig{
		ScanRoots: []string{"", " \t\n", root, missing},
	}).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Path != projectPath {
		t.Fatalf("expected temp project %q, got %+v", projectPath, projects)
	}
	var emptyWarnings, missingWarnings int
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var record struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
			Path  string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Level != "WARN" {
			t.Errorf("expected warning, got %s", line)
		}
		switch record.Msg {
		case "empty scan root in config ignored":
			emptyWarnings++
		case "scan root does not exist":
			missingWarnings++
			if record.Path != missing {
				t.Errorf("unexpected missing-root path %q", record.Path)
			}
		default:
			t.Errorf("unexpected log: %s", line)
		}
	}
	if emptyWarnings != 2 || missingWarnings != 1 {
		t.Errorf("expected 2 empty-root warnings and 1 missing-root warning, got %d and %d", emptyWarnings, missingWarnings)
	}
}

func TestScanFindsIntercoreProjects(t *testing.T) {
	// Create temp directory structure
	root := t.TempDir()
	projectPath := filepath.Join(root, "myproject")
	clavainDir := filepath.Join(projectPath, ".clavain")
	if err := os.MkdirAll(clavainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create intercore.db so HasIntercore is set
	dbPath := filepath.Join(clavainDir, "intercore.db")
	if err := os.WriteFile(dbPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(config.DiscoveryConfig{
		ScanRoots: []string{root},
	})
	projects, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if !projects[0].HasIntercore {
		t.Error("expected HasIntercore=true")
	}
	if projects[0].HasGurgeh || projects[0].HasColdwine || projects[0].HasPollard {
		t.Error("expected only HasIntercore to be set")
	}
}

func TestScanSymlinkDedup(t *testing.T) {
	root := t.TempDir()
	// Real project with .clavain
	realPath := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(realPath, ".clavain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realPath, ".clavain", "intercore.db"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Symlink pointing to the same project
	linkPath := filepath.Join(root, "link")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	scanner := NewScanner(config.DiscoveryConfig{
		ScanRoots: []string{root},
	})
	projects, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 1 {
		t.Errorf("expected 1 project (deduped), got %d", len(projects))
	}
}
