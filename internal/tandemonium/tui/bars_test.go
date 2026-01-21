package tui

import (
	"strings"
	"testing"
)

func TestViewIncludesTopBar(t *testing.T) {
	m := NewModel()
	m.Title = "Praude"
	m.CurrentPRD = "PRD-001"
	m.RepoPath = "/repo"
	m.StatusBadges = []string{"draft"}
	out := m.View()
	if !strings.Contains(out, "Praude | PRD: PRD-001") {
		t.Fatalf("expected top bar with title/prd")
	}
	if !strings.Contains(out, "repo: /repo") {
		t.Fatalf("expected repo in top bar")
	}
	if !strings.Contains(out, "status: draft") {
		t.Fatalf("expected status badges in top bar")
	}
}

func TestViewIncludesBottomBar(t *testing.T) {
	m := NewModel()
	m.Status = "ready"
	out := m.View()
	if !strings.Contains(out, "MODE: VIEW") {
		t.Fatalf("expected mode in bottom bar")
	}
	if !strings.Contains(out, "FOCUS: DOC") {
		t.Fatalf("expected focus in bottom bar")
	}
	if !strings.Contains(out, "STATUS: ready") {
		t.Fatalf("expected status in bottom bar")
	}
}
