package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewSelectionMovesDown(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewQueue = []string{"T1", "T2"}
	m.SelectedReview = 0

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updated := next.(Model)
	if updated.SelectedReview != 1 {
		t.Fatalf("expected selection 1, got %d", updated.SelectedReview)
	}
}
