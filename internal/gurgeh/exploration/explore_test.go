package exploration

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestExplore_RequiresClaude(t *testing.T) {
	// This test verifies error handling when Claude is not available
	// by using a non-existent path for the cwd
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := Explore(ctx, "/nonexistent/path")
	if err == nil {
		t.Skip("Claude Code is installed and reachable - skipping error path test")
	}
	// We expect some error - either "claude not found" or "claude failed"
	t.Logf("Got expected error: %v", err)
}

func TestExplore_Integration(t *testing.T) {
	if os.Getenv("AUTARCH_AGENT_INTEGRATION") != "1" {
		t.Skip("set AUTARCH_AGENT_INTEGRATION=1 to run a real agent")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if any supported agent is available
	if _, err := exec.LookPath("claude"); err != nil {
		if _, err := exec.LookPath("codex"); err != nil {
			t.Skip("no supported agent (claude, codex) available, skipping integration test")
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

	result, _, err := Explore(ctx, tmpDir)
	if err != nil {
		t.Fatalf("Explore failed: %v", err)
	}

	// Just verify we got some result back
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	t.Logf("Exploration result: %+v", result)
}
