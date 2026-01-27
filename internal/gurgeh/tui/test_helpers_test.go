package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/project"
)

func pressKey(m Model, key string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	newM, _ := m.Update(msg)
	return newM.(Model)
}

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
	entries, err := os.ReadDir(filepath.Join(root, ".gurgeh", "specs"))
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
