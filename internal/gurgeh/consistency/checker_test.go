package consistency

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

func TestCheckersReturnEmptyForEmptyState(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	engine := NewEngine()

	conflicts := engine.Check(state)

	// Empty state should have no conflicts (nothing to conflict with)
	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts for empty state, got %d", len(conflicts))
	}
}

func TestUserFeatureMismatch(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")

	// Set up a mismatch: users are "solo developers" but feature requires "enterprise admin"
	state.Sections[arbiter.PhaseUsers].Content = "Solo developers building side projects"
	state.Sections[arbiter.PhaseFeaturesGoals].Content = `
Features:
- Enterprise admin dashboard for managing 100+ users
- Role-based access control with SSO integration
`

	engine := NewEngine()
	conflicts := engine.Check(state)

	// Should detect user-feature mismatch
	found := false
	for _, c := range conflicts {
		if c.Type == arbiter.ConflictUserFeature {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected UserFeature conflict, none found")
	}
}

func TestScopeCreepDetection(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")

	// Set up scope creep: non-goal says "no AI" but feature includes AI
	state.Sections[arbiter.PhaseScopeAssumptions].Content = `
Non-Goals:
- no AI features
- no machine learning
`
	state.Sections[arbiter.PhaseFeaturesGoals].Content = `
Features:
- AI-powered recommendations
- GPT integration for content generation
`

	engine := NewEngine()
	conflicts := engine.Check(state)

	// Should detect scope creep
	found := false
	for _, c := range conflicts {
		if c.Type == arbiter.ConflictScopeCreep && c.Severity == arbiter.SeverityBlocker {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected ScopeCreep blocker conflict, none found")
	}
}

func TestNilStateReturnsNil(t *testing.T) {
	engine := NewEngine()
	conflicts := engine.Check(nil)

	if conflicts != nil {
		t.Errorf("expected nil for nil state, got %v", conflicts)
	}
}

func TestGoalFeatureGap(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseFeaturesGoals].Content = `
Goals:
- Quick start under 5 minutes

Features:
- Dashboard for viewing data
- Export reports
`

	engine := NewEngine()
	conflicts := engine.Check(state)

	found := false
	for _, c := range conflicts {
		if c.Type == arbiter.ConflictGoalFeature {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected GoalFeature conflict, none found")
	}
}

func TestAssumptionConflict(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseScopeAssumptions].Content = `
Assumptions:
- users have accounts
`
	state.Sections[arbiter.PhaseFeaturesGoals].Content = `
Features:
- Public dashboard (no auth required)
- Anonymous data viewing
`

	engine := NewEngine()
	conflicts := engine.Check(state)

	found := false
	for _, c := range conflicts {
		if c.Type == arbiter.ConflictAssumption {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected Assumption conflict, none found")
	}
}

func TestNoConflictsForConsistentState(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseUsers].Content = "Solo developers building side projects"
	state.Sections[arbiter.PhaseFeaturesGoals].Content = `
Features:
- Simple CLI interface
- Local file storage
Goals:
- Easy to use for individual developers
`
	state.Sections[arbiter.PhaseScopeAssumptions].Content = `
In scope: CLI tool
Out of scope: Web UI
`

	engine := NewEngine()
	conflicts := engine.Check(state)

	if len(conflicts) != 0 {
		t.Errorf("expected 0 conflicts for consistent state, got %d: %v", len(conflicts), conflicts)
	}
}
