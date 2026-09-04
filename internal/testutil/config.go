// Package testutil isolates machine-local state used by integration tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// ConfigHome isolates os.UserConfigDir on macOS, Linux, and Windows.
// XDG_CONFIG_HOME alone does not isolate macOS or Windows.
func ConfigHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("AppData", filepath.Join(root, "config"))
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
