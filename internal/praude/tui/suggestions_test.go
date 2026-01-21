package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/suggestions"
)

func TestSuggestionAcceptAppliesUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(root, ".praude", "specs", "PRD-001.yaml")
	specRaw := []byte("id: \"PRD-001\"\ntitle: \"Title\"\nsummary: \"Old\"\n")
	if err := os.WriteFile(specPath, specRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	suggPath, err := suggestions.Create(filepath.Join(root, ".praude", "suggestions"), "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(suggPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedBody := strings.Replace(string(raw), "suggestion: \"\"", "suggestion: \"Updated summary\"", 1)
	if err := os.WriteFile(suggPath, []byte(updatedBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m = pressKeySuggestion(m, "s")
	m = pressKeySuggestion(m, "a")
	updatedSpec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updatedSpec), "summary: \"Old\"") {
		t.Fatalf("expected summary to change")
	}
}

func TestSuggestionToggleRejectSkipsUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(root, ".praude", "specs", "PRD-001.yaml")
	specRaw := []byte("id: \"PRD-001\"\ntitle: \"Title\"\nsummary: \"Old\"\n")
	if err := os.WriteFile(specPath, specRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	suggPath, err := suggestions.Create(filepath.Join(root, ".praude", "suggestions"), "PRD-001", time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(suggPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedBody := strings.Replace(string(raw), "suggestion: \"\"", "suggestion: \"Updated summary\"", 1)
	if err := os.WriteFile(suggPath, []byte(updatedBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m = pressKeySuggestion(m, "s")
	m = pressKeySuggestion(m, "1")
	m = pressKeySuggestion(m, "a")
	updatedSpec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updatedSpec), "Updated summary") {
		t.Fatalf("expected summary to remain unchanged")
	}
}

func pressKeySuggestion(m Model, key string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}
