package tui

import "testing"

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseCommand, "command"},
		{PhaseTarget, "target"},
		{PhaseConfirm, "confirm"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestTargetLabel(t *testing.T) {
	tests := []struct {
		target Target
		want   string
	}{
		{TargetAll, "All agents"},
		{TargetClaude, "Claude"},
		{TargetCodex, "Codex"},
		{TargetGemini, "Gemini"},
	}
	for _, tt := range tests {
		if got := tt.target.Label(); got != tt.want {
			t.Errorf("Target(%d).Label() = %q, want %q", tt.target, got, tt.want)
		}
	}
}

func TestPaneCountsTotal(t *testing.T) {
	pc := PaneCounts{Claude: 2, Codex: 1, Gemini: 0}
	if got := pc.Total(); got != 3 {
		t.Errorf("Total() = %d, want 3", got)
	}
}

func TestPaneCountsForTarget(t *testing.T) {
	pc := PaneCounts{Claude: 2, Codex: 1, Gemini: 3}
	tests := []struct {
		target Target
		want   int
	}{
		{TargetAll, 6},
		{TargetClaude, 2},
		{TargetCodex, 1},
		{TargetGemini, 3},
	}
	for _, tt := range tests {
		if got := pc.ForTarget(tt.target); got != tt.want {
			t.Errorf("ForTarget(%v) = %d, want %d", tt.target, got, tt.want)
		}
	}
}
