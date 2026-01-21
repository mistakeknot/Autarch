package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterReviewView(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	updated := next.(Model)
	if updated.ViewMode != ViewReview {
		t.Fatalf("expected review view")
	}
}
