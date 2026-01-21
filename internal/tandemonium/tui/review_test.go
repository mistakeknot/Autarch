package tui

import "testing"

func TestModelHasReviewQueue(t *testing.T) {
	m := NewModel()
	if m.ReviewQueue == nil {
		t.Fatal("expected review queue")
	}
}
