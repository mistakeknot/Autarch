package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrate_PraudeToGurgeh(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create .praude with some files
	praudePath := filepath.Join(tmpDir, ".praude")
	if err := os.MkdirAll(filepath.Join(praudePath, "specs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(praudePath, "config.yaml"), []byte("test: true"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(praudePath, "specs", "spec1.yaml"), []byte("spec: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock command for output capture
	cmd := migrateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Run migration
	err := runMigrate(tmpDir, false, false, cmd)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify .praude no longer exists
	if _, err := os.Stat(praudePath); !os.IsNotExist(err) {
		t.Error(".praude should not exist after migration")
	}

	// Verify .gurgeh exists with files
	gurgehPath := filepath.Join(tmpDir, ".gurgeh")
	if _, err := os.Stat(gurgehPath); os.IsNotExist(err) {
		t.Error(".gurgeh should exist after migration")
	}
	if _, err := os.Stat(filepath.Join(gurgehPath, "config.yaml")); os.IsNotExist(err) {
		t.Error(".gurgeh/config.yaml should exist")
	}
	if _, err := os.Stat(filepath.Join(gurgehPath, "specs", "spec1.yaml")); os.IsNotExist(err) {
		t.Error(".gurgeh/specs/spec1.yaml should exist")
	}

	// Verify marker file exists
	markerPath := filepath.Join(tmpDir, ".praude.migrated")
	content, err := os.ReadFile(markerPath)
	if err != nil {
		t.Errorf("marker file should exist: %v", err)
	}
	if !strings.Contains(string(content), ".gurgeh") {
		t.Error("marker file should reference .gurgeh")
	}

	// Check output mentions migration
	output := stdout.String()
	if !strings.Contains(output, ".praude") || !strings.Contains(output, ".gurgeh") {
		t.Errorf("output should mention migration: %s", output)
	}
}

func TestMigrate_TandemoniumToColdwine(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .tandemonium
	tandemoniumPath := filepath.Join(tmpDir, ".tandemonium")
	if err := os.MkdirAll(filepath.Join(tandemoniumPath, "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tandemoniumPath, "tasks", "task1.yaml"), []byte("task: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := runMigrate(tmpDir, false, false, cmd)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify migration
	if _, err := os.Stat(tandemoniumPath); !os.IsNotExist(err) {
		t.Error(".tandemonium should not exist after migration")
	}
	coldwinePath := filepath.Join(tmpDir, ".coldwine")
	if _, err := os.Stat(filepath.Join(coldwinePath, "tasks", "task1.yaml")); os.IsNotExist(err) {
		t.Error(".coldwine/tasks/task1.yaml should exist")
	}
}

func TestMigrate_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .praude
	praudePath := filepath.Join(tmpDir, ".praude")
	if err := os.MkdirAll(praudePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(praudePath, "test.yaml"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Run with dry-run
	err := runMigrate(tmpDir, true, false, cmd)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	// Verify .praude still exists (no actual migration)
	if _, err := os.Stat(praudePath); os.IsNotExist(err) {
		t.Error(".praude should still exist after dry-run")
	}

	// Verify .gurgeh does NOT exist
	gurgehPath := filepath.Join(tmpDir, ".gurgeh")
	if _, err := os.Stat(gurgehPath); !os.IsNotExist(err) {
		t.Error(".gurgeh should not exist after dry-run")
	}

	// Output should indicate dry-run
	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("output should indicate dry-run: %s", output)
	}
	if !strings.Contains(output, "Would migrate") {
		t.Errorf("output should say 'Would migrate': %s", output)
	}
}

func TestMigrate_BothExist_Error(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both .praude and .gurgeh
	if err := os.MkdirAll(filepath.Join(tmpDir, ".praude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".gurgeh"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := runMigrate(tmpDir, false, false, cmd)
	if err == nil {
		t.Error("migration should fail when both directories exist")
	}

	// Error output goes to stderr via cmd.OutOrStderr()
	// Check both stdout and stderr for the guidance message
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "resolve manually") {
		t.Errorf("output should contain guidance: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestMigrate_NeitherExists(t *testing.T) {
	tmpDir := t.TempDir()

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := runMigrate(tmpDir, false, false, cmd)
	if err != nil {
		t.Fatalf("migration should succeed when nothing to migrate: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Nothing to migrate") {
		t.Errorf("output should say nothing to migrate: %s", output)
	}
}

func TestMigrate_AlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()

	// Only .gurgeh exists (already migrated)
	if err := os.MkdirAll(filepath.Join(tmpDir, ".gurgeh"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := runMigrate(tmpDir, false, false, cmd)
	if err != nil {
		t.Fatalf("migration should succeed for already migrated: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Skipped") || !strings.Contains(output, "already migrated") {
		t.Errorf("output should indicate already migrated: %s", output)
	}
}

func TestMigrate_RemoveLegacy(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .praude
	praudePath := filepath.Join(tmpDir, ".praude")
	if err := os.MkdirAll(praudePath, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Run with --remove-legacy
	err := runMigrate(tmpDir, false, true, cmd)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify no marker file created
	markerPath := filepath.Join(tmpDir, ".praude.migrated")
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("marker file should not exist with --remove-legacy")
	}
}

func TestMigrate_BothLegacyDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create both legacy directories
	if err := os.MkdirAll(filepath.Join(tmpDir, ".praude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".praude", "p.yaml"), []byte("p"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".tandemonium"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".tandemonium", "t.yaml"), []byte("t"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := migrateCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	err := runMigrate(tmpDir, false, false, cmd)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Both should be migrated
	if _, err := os.Stat(filepath.Join(tmpDir, ".gurgeh", "p.yaml")); os.IsNotExist(err) {
		t.Error(".gurgeh/p.yaml should exist")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".coldwine", "t.yaml")); os.IsNotExist(err) {
		t.Error(".coldwine/t.yaml should exist")
	}

	output := stdout.String()
	if !strings.Contains(output, ".praude") || !strings.Contains(output, ".tandemonium") {
		t.Errorf("output should mention both migrations: %s", output)
	}
}
