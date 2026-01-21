package tui

import (
	"strings"
	"testing"
)

func TestDetailPaneShowsHeaderGrid(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "ID:") || !strings.Contains(out, "Status:") {
		t.Fatalf("expected header grid")
	}
}
