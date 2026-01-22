package tui

import (
	"strings"
	"testing"
)

func TestLayoutModeSelection(t *testing.T) {
	if layoutMode(40) != LayoutModeSingle {
		t.Fatalf("expected single")
	}
	if layoutMode(60) != LayoutModeStacked {
		t.Fatalf("expected stacked")
	}
	if layoutMode(90) != LayoutModeDual {
		t.Fatalf("expected dual")
	}
}

func TestRenderDualColumnLayoutJoinsHorizontally(t *testing.T) {
	out := renderDualColumnLayout("PRDs", "left", "DETAILS", "right", 100, 6)
	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected output")
	}
	if !strings.Contains(lines[0], "PRDs") || !strings.Contains(lines[0], "DETAILS") {
		t.Fatalf("expected headers on same line")
	}
}
