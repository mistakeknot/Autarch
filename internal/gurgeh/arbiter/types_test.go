package arbiter

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter/scan"
	"github.com/mistakeknot/autarch/pkg/thinking"
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
		PhaseCUJs,             // User journeys flow from users + features
		PhaseRequirements,     // Requirements derived from CUJs
		PhaseScopeAssumptions,
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

func TestDefaultModelTiers(t *testing.T) {
	tiers := DefaultModelTiers()

	if len(tiers) != PhaseCount {
		t.Errorf("DefaultModelTiers has %d entries, want %d", len(tiers), PhaseCount)
	}

	// Early phases should use cheaper models, later phases more capable.
	earlyPhases := []Phase{PhaseVision, PhaseProblem, PhaseUsers}
	for _, p := range earlyPhases {
		model, ok := tiers[p]
		if !ok {
			t.Errorf("no tier for %v", p)
			continue
		}
		if model == "" {
			t.Errorf("empty model for early phase %v", p)
		}
		// Haiku for early phases
		if !contains(model, "haiku") {
			t.Errorf("early phase %v model = %q, expected haiku tier", p, model)
		}
	}

	// Late phases should use the most capable model.
	latePhases := []Phase{PhaseScopeAssumptions, PhaseAcceptanceCriteria}
	for _, p := range latePhases {
		model := tiers[p]
		if !contains(model, "opus") {
			t.Errorf("late phase %v model = %q, expected opus tier", p, model)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestModelForPhase(t *testing.T) {
	overrides := map[Phase]string{
		PhaseVision:  "claude-haiku-4-5-20251001",
		PhaseProblem: "claude-sonnet-4-5-20250929",
	}

	// Should return override when present.
	if got := ModelForPhase(overrides, PhaseVision); got != "claude-haiku-4-5-20251001" {
		t.Errorf("ModelForPhase(Vision) = %q, want haiku", got)
	}
	if got := ModelForPhase(overrides, PhaseProblem); got != "claude-sonnet-4-5-20250929" {
		t.Errorf("ModelForPhase(Problem) = %q, want sonnet", got)
	}

	// Should return empty string when no override for this phase.
	if got := ModelForPhase(overrides, PhaseUsers); got != "" {
		t.Errorf("ModelForPhase(Users) = %q, want empty", got)
	}

	// Should return empty string when overrides is nil.
	if got := ModelForPhase(nil, PhaseVision); got != "" {
		t.Errorf("ModelForPhase(nil, Vision) = %q, want empty", got)
	}
}

func TestSprintStateClone(t *testing.T) {
	now := time.Now()
	original := &SprintState{
		ID:          "test-id",
		SpecID:      "spec-1",
		ProjectPath: "/tmp/test",
		Phase:       PhaseProblem,
		Sections: map[Phase]*SectionDraft{
			PhaseVision: {
				Content:       "vision content",
				Options:       []string{"opt1", "opt2"},
				Status:        DraftAccepted,
				ActiveSignals: []string{"sig1"},
				UserEdits: []Edit{
					{Before: "a", After: "b", Reason: "test", Timestamp: now},
				},
				UpdatedAt: now,
			},
			PhaseProblem: {
				Content: "problem content",
				Status:  DraftPending,
			},
		},
		Conflicts: []Conflict{
			{Type: ConflictScopeCreep, Severity: SeverityWarning, Message: "scope", Sections: []Phase{PhaseProblem, PhaseVision}},
		},
		Findings: []ResearchFinding{
			{ID: "f1", Title: "finding", Tags: []string{"tag1", "tag2"}},
		},
		ResearchCtx: &QuickScanResult{
			Topic:      "test topic",
			GitHubHits: []GitHubFinding{{Name: "repo1", Stars: 100}},
			HNHits:     []HNFinding{{Title: "hn1", Points: 50}},
			Summary:    "summary",
		},
		VisionContext: &VisionContext{
			SpecID:      "vision-1",
			Goals:       []string{"goal1"},
			Assumptions: []string{"assumption1"},
			CUJs:        []string{"cuj1"},
			Hypotheses:  []string{"hyp1"},
		},
		ShapeOverrides: map[Phase]thinking.Shape{
			PhaseVision: thinking.ShapeDeductive,
		},
		ModelOverrides: map[Phase]string{
			PhaseVision: "claude-haiku-4-5-20251001",
		},
		ScanArtifacts: &scan.Artifacts{},
		StartedAt:     now,
		UpdatedAt:     now,
	}

	clone := original.Clone()

	// Mutate original
	original.Sections[PhaseVision].Content = "MUTATED"
	original.Sections[PhaseVision].Options = append(original.Sections[PhaseVision].Options, "opt3")
	original.Sections[PhaseVision].UserEdits = append(original.Sections[PhaseVision].UserEdits, Edit{Before: "x", After: "y"})
	original.Conflicts[0].Sections = append(original.Conflicts[0].Sections, PhaseUsers)
	original.Findings[0].Tags = append(original.Findings[0].Tags, "tag3")
	original.ResearchCtx.GitHubHits = append(original.ResearchCtx.GitHubHits, GitHubFinding{Name: "repo2"})
	original.VisionContext.Goals = append(original.VisionContext.Goals, "goal2")
	original.ShapeOverrides[PhaseProblem] = thinking.ShapeInductive
	original.ModelOverrides[PhaseProblem] = "claude-opus-4-6"

	// Assert clone is unchanged
	if clone.Sections[PhaseVision].Content != "vision content" {
		t.Error("clone section content was mutated")
	}
	if len(clone.Sections[PhaseVision].Options) != 2 {
		t.Errorf("clone options mutated: got %d, want 2", len(clone.Sections[PhaseVision].Options))
	}
	if len(clone.Sections[PhaseVision].UserEdits) != 1 {
		t.Errorf("clone edits mutated: got %d, want 1", len(clone.Sections[PhaseVision].UserEdits))
	}
	if len(clone.Conflicts[0].Sections) != 2 {
		t.Errorf("clone conflict sections mutated: got %d, want 2", len(clone.Conflicts[0].Sections))
	}
	if len(clone.Findings[0].Tags) != 2 {
		t.Errorf("clone finding tags mutated: got %d, want 2", len(clone.Findings[0].Tags))
	}
	if len(clone.ResearchCtx.GitHubHits) != 1 {
		t.Errorf("clone github hits mutated: got %d, want 1", len(clone.ResearchCtx.GitHubHits))
	}
	if len(clone.VisionContext.Goals) != 1 {
		t.Errorf("clone vision goals mutated: got %d, want 1", len(clone.VisionContext.Goals))
	}
	if len(clone.ShapeOverrides) != 1 {
		t.Errorf("clone shape overrides mutated: got %d, want 1", len(clone.ShapeOverrides))
	}
	if len(clone.ModelOverrides) != 1 {
		t.Errorf("clone model overrides mutated: got %d, want 1", len(clone.ModelOverrides))
	}

	// ScanArtifacts should be the same pointer (shared, immutable)
	if clone.ScanArtifacts != original.ScanArtifacts {
		t.Error("ScanArtifacts should be shared pointer, not deep-copied")
	}
}
