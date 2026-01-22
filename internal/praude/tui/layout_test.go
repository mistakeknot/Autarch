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
	found := false
	for _, line := range lines {
		if strings.Contains(line, "PRDs") && strings.Contains(line, "DETAILS") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected headers on same line")
	}
}

func TestPanelStyleAddsBorders(t *testing.T) {
	out := renderDualColumnLayout("PRDs", "left", "DETAILS", "right", 100, 6)
	if !strings.Contains(out, "╭") && !strings.Contains(out, "┌") {
		t.Fatalf("expected bordered panels")
	}
}
