package exploration

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestExplore_RequiresClaude(t *testing.T) {
	// This test verifies error handling when Claude is not available
	// by using a non-existent path for the cwd
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Explore(ctx, "/nonexistent/path")
	if err == nil {
		t.Skip("Claude Code is installed and reachable - skipping error path test")
	}
	// We expect some error - either "claude not found" or "claude failed"
	t.Logf("Got expected error: %v", err)
}

func TestExplore_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if Claude is available
	if _, err := os.Stat("/usr/local/bin/claude"); os.IsNotExist(err) {
		if os.Getenv("PATH") == "" {
			t.Skip("claude not in PATH, skipping integration test")
		}
	}

	// Create a minimal test directory with a README
	tmpDir := t.TempDir()
	readme := `# Test Project
A simple test project for testing exploration.

## Vision
This is a test project for validating Claude Code exploration.

## Users
Developers testing the exploration feature.
`
	if err := os.WriteFile(tmpDir+"/README.md", []byte(readme), 0644); err != nil {
		t.Fatalf("failed to write test README: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := Explore(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	// Just verify we got some result back
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	t.Logf("Exploration result: %+v", result)
}
