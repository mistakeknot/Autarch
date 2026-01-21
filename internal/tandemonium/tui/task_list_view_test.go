package tui

import (
	"strings"
	"testing"
)

func TestFleetViewShowsTaskList(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "todo"}}
	out := m.View()
	if !strings.Contains(out, "T1") || !strings.Contains(out, "One") {
		t.Fatalf("expected task list in view")
	}
}
