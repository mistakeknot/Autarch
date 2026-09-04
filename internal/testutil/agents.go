package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// WithoutAgents keeps ordinary tests on the offline fallback even on machines
// with authenticated agent CLIs installed.
func WithoutAgents(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho 'agent disabled in unit tests' >&2\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
