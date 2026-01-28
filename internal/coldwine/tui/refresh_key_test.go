package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRefreshKeyDoesNotEnterReview(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewFleet
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "review"}}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	updated := next.(Model)
	if updated.ViewMode != ViewFleet {
		t.Fatalf("expected to stay in fleet view")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command")
	}
}
