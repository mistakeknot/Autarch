package tui

import (
	"strings"
	"testing"
)

func TestLayoutIncludesHeaderFooter(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "PRAUDE") {
		t.Fatalf("expected header")
	}
	if !strings.Contains(out, "KEYS:") {
		t.Fatalf("expected footer")
	}
}

func TestSplitViewFallback(t *testing.T) {
	out := renderSplitView(60, []string{"L"}, []string{"R"})
	if strings.Contains(out, "|") {
		t.Fatalf("expected single column on narrow width")
	}
	wide := renderSplitView(140, []string{"L"}, []string{"R"})
	if !strings.Contains(wide, "|") {
		t.Fatalf("expected split view on wide width")
	}
}
