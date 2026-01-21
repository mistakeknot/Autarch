package tui

import (
	"strings"
	"testing"
)

func TestTableHeaderIncludesColumns(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "Alpha", Status: "todo"}}
	out := m.View()
	if !strings.Contains(out, "TYPE") || !strings.Contains(out, "PRI") {
		t.Fatalf("expected extended column headers")
	}
}
