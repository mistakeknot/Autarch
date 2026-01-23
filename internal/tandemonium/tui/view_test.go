package tui

import (
	"strings"
	"testing"
)

func TestViewIncludesReviewHeader(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "TASKS") {
		t.Fatal("expected tasks header")
	}
	if !strings.Contains(out, "DETAILS") {
		t.Fatal("expected details header")
	}
	if !strings.Contains(out, " | ") {
		t.Fatal("expected pane separator")
	}
}

func TestViewIncludesHelpFooter(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "KEYS: n new task") {
		t.Fatalf("expected help footer, got %q", out)
	}
}

func TestReviewViewIncludesActions(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	out := m.View()
	if !strings.Contains(out, "[d]iff") || !strings.Contains(out, "[a]pprove") {
		t.Fatalf("expected review actions, got %q", out)
	}
}

func TestTabBarRenders(t *testing.T) {
	m := NewModel()
	m.Width = 120
	out := stripANSI(m.View())
	if !strings.Contains(out, "Fleet") || !strings.Contains(out, "Review") {
		t.Fatalf("expected tab bar")
	}
}

func TestTwoPaneLayoutRenders(t *testing.T) {
	m := NewModel()
	m.Width = 120
	m.Height = 40
	out := stripANSI(m.View())
	if !strings.Contains(out, "│") && !strings.Contains(out, "┐") {
		t.Fatalf("expected pane borders")
	}
}
