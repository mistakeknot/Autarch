package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

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
