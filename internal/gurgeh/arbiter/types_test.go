package arbiter

import (
	"testing"
)

func TestNewSprintState(t *testing.T) {
	state := NewSprintState("test-project")

	if state.Phase != PhaseVision {
		t.Errorf("expected initial phase %v, got %v", PhaseVision, state.Phase)
	}
	if len(state.Sections) != PhaseCount {
		t.Errorf("expected %d sections, got %d", PhaseCount, len(state.Sections))
	}
	if state.Confidence.Total() != 0 {
		t.Errorf("expected initial confidence 0, got %f", state.Confidence.Total())
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase    Phase
		expected string
	}{
		{PhaseProblem, "Problem"},
		{PhaseAcceptanceCriteria, "Acceptance Criteria"},
		{Phase(-1), "Unknown"},
		{Phase(100), "Unknown"},
	}

	for _, tt := range tests {
		got := tt.phase.String()
		if got != tt.expected {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.expected)
		}
	}
}

func TestPhaseOrder(t *testing.T) {
	phases := AllPhases()
	expected := []Phase{
		PhaseVision,
		PhaseProblem,
		PhaseUsers,
		PhaseFeaturesGoals,
		PhaseRequirements,
		PhaseScopeAssumptions,
		PhaseCUJs,
		PhaseAcceptanceCriteria,
	}

	if len(phases) != len(expected) {
		t.Fatalf("expected %d phases, got %d", len(expected), len(phases))
	}

	for i, p := range phases {
		if p != expected[i] {
			t.Errorf("phase %d: expected %v, got %v", i, expected[i], p)
		}
	}
}
