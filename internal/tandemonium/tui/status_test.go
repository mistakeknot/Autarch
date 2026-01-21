package tui

import (
	"strings"
	"testing"
)

func TestViewIncludesStatusLine(t *testing.T) {
	m := NewModel()
	m.Status = "ready"
	m.StatusLevel = StatusInfo
	view := m.View()
	if !strings.Contains(view, "STATUS: ready") {
		t.Fatalf("expected status line, got %q", view)
	}
}
