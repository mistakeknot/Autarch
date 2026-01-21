package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReviewRejectRequiresFeedback(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewQueue = []string{"T1"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	updated := next.(Model)
	if updated.ReviewInputMode != ReviewInputFeedback {
		t.Fatalf("expected feedback mode")
	}
	if !updated.ReviewPendingReject {
		t.Fatalf("expected pending reject")
	}
}
