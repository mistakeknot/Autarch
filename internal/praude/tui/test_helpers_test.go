package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func withTempRoot(t *testing.T, fn func(root string)) {
	t.Helper()
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	fn(root)
}

func withTempRootInitialized(t *testing.T, fn func(root string)) {
	t.Helper()
	withTempRoot(t, func(root string) {
		if err := project.Init(root); err != nil {
			t.Fatal(err)
		}
		fn(root)
	})
}

func praudeSpecFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	return files
}
