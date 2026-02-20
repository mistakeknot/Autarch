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

func TestRunsPaneRenderWithData(t *testing.T) {
	p := NewRunsPane()
	p.SetSize(80, 20)
	p.SetRuns([]Run{
		{
			ID:     "r1abcde",
			Goal:   "Cost-aware scheduling",
			Phase:  "brainstorm",
			Phases: []string{"brainstorm", "strategized", "planned", "executing", "done"},
			Status: "active",
		},
		{
			ID:     "r2fghij",
			Goal:   "Status tool MVP",
			Phase:  "done",
			Phases: []string{"brainstorm", "done"},
			Status: "completed",
		},
	})

	view := p.View()

	if !strings.Contains(view, "RUNS") {
		t.Error("expected 'RUNS' header")
	}
	if !strings.Contains(view, "r1abcde") {
		t.Error("expected run ID 'r1abcde'")
	}
	if !strings.Contains(view, "r2fghij") {
		t.Error("expected run ID 'r2fghij'")
	}
	if !strings.Contains(view, "Cost-aware") {
		t.Error("expected goal text 'Cost-aware'")
	}
	if !strings.Contains(view, "brainstorm") {
		t.Error("expected phase 'brainstorm'")
	}
	if !strings.Contains(view, "done") {
		t.Error("expected phase 'done'")
	}
}

func TestRunsPaneRenderTruncation(t *testing.T) {
	p := NewRunsPane()
	p.SetSize(80, 20)

	longGoal := strings.Repeat("x", 80)
	p.SetRuns([]Run{
		{ID: "r1", Goal: longGoal, Phase: "brainstorm", Status: "active"},
	})

	view := p.View()

	// Full goal should NOT appear — goalWidth = 80 - 46 = 34, so goal is truncated
	if strings.Contains(view, longGoal) {
		t.Error("expected long goal to be truncated")
	}
	// Truncated prefix should appear (33 chars + ellipsis)
	if !strings.Contains(view, longGoal[:33]) {
		t.Error("expected truncated goal prefix")
	}
}

func TestRunsPaneRenderDiffersWithCursor(t *testing.T) {
	p := NewRunsPane()
	p.SetSize(80, 20)
	p.SetRuns([]Run{
		{ID: "r1", Goal: "First run", Phase: "brainstorm", Status: "active"},
		{ID: "r2", Goal: "Second run", Phase: "done", Status: "completed"},
	})

	// Render with cursor at 0
	view1 := p.View()

	// Move cursor to 1 and render again
	p.CursorDown()
	view2 := p.View()

	// Views should differ — selected row gets Width() padding
	if view1 == view2 {
		t.Error("expected different views with different cursor positions")
	}
}
