package tui

import (
	"strings"
	"testing"
)

func TestReviewBranchesShownInView(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "TAND-001", Title: "Test", Status: "review"}}
	view := m.View()
	if !strings.Contains(view, "TAND-001") {
		t.Fatalf("expected task label in view, got %q", view)
	}
}
