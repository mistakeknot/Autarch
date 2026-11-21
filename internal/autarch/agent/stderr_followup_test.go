//go:build !windows

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailureIncludesStderrTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho diagnostic-tail >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AUTARCH_AGENT", "claude")
	a := &Agent{Type: "claude", explicitSelection: true}
	_, err := a.GenerateWithOutput(context.Background(), GenerateRequest{Prompt: "hello"}, nil)
	if err == nil || !strings.Contains(err.Error(), "diagnostic-tail") {
		t.Fatalf("missing stderr tail: %v", err)
	}
}

func TestErrorResultAndStderrTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho '{\"type\":\"result\",\"is_error\":true,\"result\":\"please authenticate\"}'\necho diagnostic-tail >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AUTARCH_AGENT", "claude")
	a := &Agent{Type: "claude", explicitSelection: true}
	_, err := a.GenerateWithOutput(context.Background(), GenerateRequest{Prompt: "hello"}, nil)
	if err == nil || (!strings.Contains(err.Error(), "diagnostic-tail") || !strings.Contains(err.Error(), "please authenticate")) {
		t.Fatalf("missing stderr tail: %v", err)
	}
}
