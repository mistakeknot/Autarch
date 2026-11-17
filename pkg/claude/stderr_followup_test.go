//go:build !windows

package claude

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
	_, err := Run(context.Background(), t.TempDir(), []string{"-p", "hello"})
	if err == nil || !strings.Contains(err.Error(), "diagnostic-tail") {
		t.Fatalf("missing stderr tail: %v", err)
	}
}
