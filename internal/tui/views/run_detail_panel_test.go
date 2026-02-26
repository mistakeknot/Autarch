package views

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/intercore"
)

func TestRunDetailPanel_RenderEmpty(t *testing.T) {
	p := NewRunDetailPanel()
	got := p.Render()
	if !strings.Contains(got, "Select a sprint run") {
		t.Errorf("empty panel should show 'Select a sprint run', got: %q", got)
	}
}

func TestRunDetailPanel_RenderLoadingState(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetData(&intercore.Run{ID: "abc123", Goal: "Test goal", Status: "active"}, nil, nil, nil, nil)
	got := p.Render()
	if !strings.Contains(got, "Loading sprint detail") {
		t.Errorf("loading state should show 'Loading sprint detail...', got: %q", got)
	}
	if !strings.Contains(got, "abc123") {
		t.Errorf("loading state should show run ID, got: %q", got)
	}
}

func TestRunDetailPanel_RenderWithRun(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetSize(80, 40)

	exitCode := 0
	p.SetData(
		&intercore.Run{
			ID: "test123", Goal: "Build feature", Status: "active",
			Phase: "plan", Phases: []string{"brainstorm", "plan", "execute"},
			AutoAdvance: true,
		},
		[]intercore.Dispatch{
			{ID: "d1", RunID: "test123", Status: "completed", Agent: "review", ExitCode: &exitCode},
		},
		&intercore.BudgetResult{RunID: "test123", TokenBudget: 100000, TokensUsed: 50000},
		[]intercore.Event{
			{RunID: "test123", Source: "gate", Type: "advance", Timestamp: 1000000},
		},
		&intercore.GateResult{RunID: "test123", FromPhase: "plan", ToPhase: "execute", Result: "pass", Tier: "basic"},
	)

	got := p.Render()

	// Check header
	if !strings.Contains(got, "test123") {
		t.Errorf("should contain run ID, got: %q", got)
	}
	if !strings.Contains(got, "Build feature") {
		t.Errorf("should contain goal, got: %q", got)
	}

	// Check phase timeline
	if !strings.Contains(got, "Phase Timeline") {
		t.Errorf("should contain phase timeline section, got: %q", got)
	}
	if !strings.Contains(got, "plan") {
		t.Errorf("should contain current phase, got: %q", got)
	}

	// Check budget
	if !strings.Contains(got, "Token Budget") {
		t.Errorf("should contain budget section, got: %q", got)
	}

	// Check gate
	if !strings.Contains(got, "Gate Status") {
		t.Errorf("should contain gate section, got: %q", got)
	}
	if !strings.Contains(got, "PASS") {
		t.Errorf("should show PASS for passing gate, got: %q", got)
	}

	// Check dispatches
	if !strings.Contains(got, "Dispatches") {
		t.Errorf("should contain dispatches section, got: %q", got)
	}

	// Check events
	if !strings.Contains(got, "Recent Events") {
		t.Errorf("should contain events section, got: %q", got)
	}
}

func TestRunDetailPanel_CompactRender(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetSize(80, 40)

	p.SetData(
		&intercore.Run{
			ID: "test123", Goal: "Build feature", Status: "active",
			Phase: "plan", Phases: []string{"brainstorm", "plan"},
		},
		[]intercore.Dispatch{{ID: "d1", RunID: "test123", Status: "running", Agent: "build"}},
		&intercore.BudgetResult{RunID: "test123", TokenBudget: 100000, TokensUsed: 50000},
		nil, // no events
		&intercore.GateResult{RunID: "test123", FromPhase: "plan", ToPhase: "execute", Result: "pass", Tier: "basic"},
	)

	got := p.CompactRender()

	// Compact should have header, phase timeline, budget, gate
	if !strings.Contains(got, "test123") {
		t.Errorf("compact should contain run ID")
	}
	if !strings.Contains(got, "Phase Timeline") {
		t.Errorf("compact should contain phase timeline")
	}
	if !strings.Contains(got, "Token Budget") {
		t.Errorf("compact should contain budget")
	}
	if !strings.Contains(got, "Gate Status") {
		t.Errorf("compact should contain gate status")
	}

	// Compact should NOT have dispatches or events
	if strings.Contains(got, "Dispatches") {
		t.Errorf("compact should NOT contain dispatches section")
	}
	if strings.Contains(got, "Recent Events") {
		t.Errorf("compact should NOT contain events section")
	}
}

func TestRunDetailPanel_BudgetExceeded(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetData(
		&intercore.Run{ID: "x", Status: "active"},
		[]intercore.Dispatch{}, // non-nil so we don't trigger loading state
		&intercore.BudgetResult{RunID: "x", TokenBudget: 100000, TokensUsed: 150000, Exceeded: true},
		[]intercore.Event{},
		nil,
	)

	got := p.Render()
	if !strings.Contains(got, "BUDGET EXCEEDED") {
		t.Errorf("should show BUDGET EXCEEDED when exceeded, got: %q", got)
	}
}

func TestRunDetailPanel_GateBlocked(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetData(
		&intercore.Run{ID: "x", Status: "active"},
		[]intercore.Dispatch{},
		nil,
		[]intercore.Event{},
		&intercore.GateResult{RunID: "x", FromPhase: "plan", ToPhase: "execute", Result: "blocked", Tier: "basic"},
	)

	got := p.Render()
	if !strings.Contains(got, "BLOCKED") {
		t.Errorf("should show BLOCKED for failing gate, got: %q", got)
	}
}

func TestRenderRunSidebarItems(t *testing.T) {
	runs := []intercore.Run{
		{ID: "run001", Status: "active", Phase: "execute"},
		{ID: "run002", Status: "completed", Phase: "done"},
		{ID: "run003", Status: "cancelled", Phase: "plan"},
	}

	items := renderRunSidebarItems(runs, 0)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// First should have active icon and selection marker
	if items[0].Icon != "●" {
		t.Errorf("active run should have ● icon, got %q", items[0].Icon)
	}
	if !strings.Contains(items[0].Label, "▸") {
		t.Errorf("selected run should have ▸ marker, got %q", items[0].Label)
	}

	// Second should have completed icon
	if items[1].Icon != "✓" {
		t.Errorf("completed run should have ✓ icon, got %q", items[1].Icon)
	}

	// Third should have cancelled icon
	if items[2].Icon != "✗" {
		t.Errorf("cancelled run should have ✗ icon, got %q", items[2].Icon)
	}
}

func TestRenderRunSidebarItems_Empty(t *testing.T) {
	items := renderRunSidebarItems(nil, 0)
	if len(items) != 1 {
		t.Fatalf("expected 1 placeholder item, got %d", len(items))
	}
	if !strings.Contains(items[0].Label, "No sprints") {
		t.Errorf("empty list should show 'No sprints', got %q", items[0].Label)
	}
}

func TestShouldAutoAdvance(t *testing.T) {
	exitZero := 0
	exitOne := 1

	tests := []struct {
		name string
		run  *intercore.Run
		d    intercore.Dispatch
		want bool
	}{
		{
			name: "nil run",
			run:  nil,
			d:    intercore.Dispatch{Status: "completed", ExitCode: &exitZero},
			want: false,
		},
		{
			name: "auto-advance disabled",
			run:  &intercore.Run{AutoAdvance: false, Status: "active"},
			d:    intercore.Dispatch{Status: "completed", ExitCode: &exitZero},
			want: false,
		},
		{
			name: "run not active",
			run:  &intercore.Run{AutoAdvance: true, Status: "completed"},
			d:    intercore.Dispatch{Status: "completed", ExitCode: &exitZero},
			want: false,
		},
		{
			name: "dispatch not completed",
			run:  &intercore.Run{AutoAdvance: true, Status: "active"},
			d:    intercore.Dispatch{Status: "running"},
			want: false,
		},
		{
			name: "dispatch failed (exit 1)",
			run:  &intercore.Run{AutoAdvance: true, Status: "active"},
			d:    intercore.Dispatch{Status: "completed", ExitCode: &exitOne},
			want: false,
		},
		{
			name: "all conditions met",
			run:  &intercore.Run{AutoAdvance: true, Status: "active"},
			d:    intercore.Dispatch{Status: "completed", ExitCode: &exitZero},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldAutoAdvance(tt.run, tt.d)
			if got != tt.want {
				t.Errorf("ShouldAutoAdvance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunDetailPanel_MaxEvents(t *testing.T) {
	p := NewRunDetailPanel()
	p.SetMaxEvents(3)

	events := make([]intercore.Event, 10)
	for i := range events {
		events[i] = intercore.Event{RunID: "x", Source: "test", Type: "phase_change", Timestamp: int64(1000000 + i)}
	}

	p.SetData(
		&intercore.Run{ID: "x", Status: "active"},
		[]intercore.Dispatch{},
		nil,
		events,
		nil,
	)

	got := p.Render()
	// Count event lines (lines containing a timestamp pattern HH:MM:SS)
	lines := strings.Split(got, "\n")
	eventLines := 0
	for _, line := range lines {
		if strings.Contains(line, "test") && strings.Contains(line, "phase_change") {
			eventLines++
		}
	}
	if eventLines != 3 {
		t.Errorf("expected 3 event lines with maxEvents=3, got %d", eventLines)
	}
}
