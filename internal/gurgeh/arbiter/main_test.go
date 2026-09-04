package arbiter

import (
	"os"
	"path/filepath"
	"testing"
)

// Ordinary sprint tests exercise the offline fallback. Having a real agent
// installed must not turn go test into a paid/network integration run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "autarch-offline-agents-")
	if err != nil {
		panic(err)
	}
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho 'agent disabled in unit tests' >&2\nexit 1\n"), 0700); err != nil {
			panic(err)
		}
	}
	oldPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	code := m.Run()
	_ = os.Setenv("PATH", oldPath)
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
