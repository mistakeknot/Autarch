package tui

import (
	"strings"
	"testing"
)

func TestHelpOverlayToggle(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "?")
		if !strings.Contains(stripANSI(m.View()), "Help") {
			t.Fatalf("expected help overlay")
		}
	})
}

func TestTutorialOverlayToggle(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "`")
		if !strings.Contains(stripANSI(m.View()), "Tutorial") {
			t.Fatalf("expected tutorial overlay")
		}
	})
}
