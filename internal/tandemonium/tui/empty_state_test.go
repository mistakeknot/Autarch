package tui

import (
	"strings"
	"testing"
)

func TestEmptyStateShowsQuickStart(t *testing.T) {
	m := NewModel()
	m.TaskList = nil
	out := m.View()
	if !strings.Contains(out, "Quick start") {
		t.Fatalf("expected quick start block")
	}
	if !strings.Contains(out, "1) init") {
		t.Fatalf("expected init step")
	}
}
