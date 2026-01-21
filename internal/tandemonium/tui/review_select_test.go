package tui

import "testing"

func TestClampSelectionAfterRefresh(t *testing.T) {
	m := NewModel()
	m.ReviewQueue = []string{"T1", "T2"}
	m.SelectedReview = 1
	m.ReviewQueue = []string{"T1"}
	m.ClampReviewSelection()
	if m.SelectedReview != 0 {
		t.Fatalf("expected selection 0, got %d", m.SelectedReview)
	}
}
