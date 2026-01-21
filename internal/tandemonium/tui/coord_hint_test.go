package tui

import (
	"fmt"
	"strings"
	"testing"
)

func TestCoordTabShowsHints(t *testing.T) {
	m := NewModel()
	m.TaskList = make([]TaskItem, 15)
	for i := range m.TaskList {
		m.TaskList[i] = TaskItem{
			ID:     fmt.Sprintf("tsk-%02d", i),
			Title:  "Task",
			Status: "todo",
		}
	}
	m.RightTab = RightTabCoord
	out := m.View()
	if !strings.Contains(out, "u urgent") || !strings.Contains(out, "r recipient") || !strings.Contains(out, "tand mail") {
		t.Fatalf("expected coord hints")
	}
}
