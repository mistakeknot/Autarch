package tui

import (
	"strings"
	"testing"
)

func TestTaskDetailShowsStatusBadges(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "in_progress", SessionState: "working"}}
	m.TaskDetail = TaskDetail{
		ID:           "T1",
		Title:        "One",
		Status:       "in_progress",
		SessionState: "working",
		Summary:      "Did the thing.",
	}
	out := m.View()
	clean := stripANSI(out)
	if !strings.Contains(clean, "Status: [RUN]") {
		t.Fatalf("expected status badge, got %q", clean)
	}
	if !strings.Contains(clean, "Session: [RUN]") {
		t.Fatalf("expected session badge")
	}
	if !strings.Contains(clean, "Did the thing.") {
		t.Fatalf("expected summary")
	}
}
