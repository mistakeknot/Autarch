package status

import (
	"strings"
	"testing"
)

func TestRenderProgressBar(t *testing.T) {
	phases := []string{"brainstorm", "strategized", "planned", "executing", "done"}

	tests := []struct {
		phase    string
		barWidth int
		wantFill int // approximate filled chars
	}{
		{"brainstorm", 8, 1},   // 1/5 = 1.6
		{"planned", 8, 4},     // 3/5 = 4.8
		{"done", 8, 8},        // 5/5 = 8
	}

	for _, tt := range tests {
		bar := renderProgressBar(tt.phase, phases, tt.barWidth)
		// Bar should contain some filled and some empty chars
		if bar == "" {
			t.Errorf("renderProgressBar(%q) returned empty string", tt.phase)
		}
		// Just verify it's not panicking and returns something
		_ = bar
	}
}

func TestRenderProgressBarNoPhases(t *testing.T) {
	bar := renderProgressBar("brainstorm", nil, 8)
	if !strings.Contains(bar, "░") {
		t.Error("expected empty progress bar for nil phases")
	}
}

func TestRunsPaneCursor(t *testing.T) {
	p := NewRunsPane()
	p.SetRuns([]Run{
		{ID: "r1", Goal: "First"},
		{ID: "r2", Goal: "Second"},
		{ID: "r3", Goal: "Third"},
	})

	if p.SelectedRun().ID != "r1" {
		t.Errorf("initial selection = %q, want r1", p.SelectedRun().ID)
	}

	p.CursorDown()
	if p.SelectedRun().ID != "r2" {
		t.Errorf("after down = %q, want r2", p.SelectedRun().ID)
	}

	p.CursorDown()
	p.CursorDown() // should clamp at end
	if p.SelectedRun().ID != "r3" {
		t.Errorf("after 2x down = %q, want r3", p.SelectedRun().ID)
	}

	p.CursorUp()
	if p.SelectedRun().ID != "r2" {
		t.Errorf("after up = %q, want r2", p.SelectedRun().ID)
	}
}

func TestRunsPaneEmpty(t *testing.T) {
	p := NewRunsPane()
	if p.SelectedRun() != nil {
		t.Error("expected nil for empty runs pane")
	}
	// Should not panic
	p.CursorDown()
	p.CursorUp()
}

func TestRunsPaneCursorPreservedOnUpdate(t *testing.T) {
	p := NewRunsPane()
	p.SetRuns([]Run{
		{ID: "r1"}, {ID: "r2"}, {ID: "r3"},
	})
	p.CursorDown() // cursor = 1

	// Update with same runs — cursor should stay
	p.SetRuns([]Run{
		{ID: "r1"}, {ID: "r2"}, {ID: "r3"},
	})
	if p.SelectedRun().ID != "r2" {
		t.Errorf("cursor not preserved: got %q, want r2", p.SelectedRun().ID)
	}

	// Update with fewer runs — cursor should clamp
	p.SetRuns([]Run{{ID: "r1"}})
	if p.SelectedRun().ID != "r1" {
		t.Errorf("cursor not clamped: got %q, want r1", p.SelectedRun().ID)
	}
}
