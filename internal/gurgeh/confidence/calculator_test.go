package confidence

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
)

func TestEmptyStateHasZeroConfidence(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	calc := NewCalculator()
	score := calc.Calculate(state)
	if score.Total() != 0 {
		t.Errorf("expected 0 confidence for empty state, got %f", score.Total())
	}
}

func TestCompleteStateHasHighConfidence(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseVision].Content = "A world where automation handles the boring parts of software development"
	state.Sections[arbiter.PhaseVision].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseProblem].Content = "Users waste time on repetitive tasks"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseUsers].Content = "Software developers, 25-45, building side projects"
	state.Sections[arbiter.PhaseUsers].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseFeaturesGoals].Content = "Features:\n- Automated task scheduling\n- Integration with GitHub\nGoals:\n- Reduce manual work by 50%"
	state.Sections[arbiter.PhaseFeaturesGoals].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseRequirements].Content = "- Must support GitHub Actions\n- Must handle 100 concurrent users"
	state.Sections[arbiter.PhaseRequirements].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseScopeAssumptions].Content = "In scope: Core automation\nOut of scope: Mobile app\nAssumptions: Users have GitHub accounts"
	state.Sections[arbiter.PhaseScopeAssumptions].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseCUJs].Content = "CUJ 1: User connects GitHub → creates first automation → sees results"
	state.Sections[arbiter.PhaseCUJs].Status = arbiter.DraftAccepted
	state.Sections[arbiter.PhaseAcceptanceCriteria].Content = "- Automation runs within 5 seconds of trigger\n- Errors are logged with actionable messages"
	state.Sections[arbiter.PhaseAcceptanceCriteria].Status = arbiter.DraftAccepted

	calc := NewCalculator()
	score := calc.Calculate(state)
	if score.Completeness < 0.8 {
		t.Errorf("expected high completeness, got %f", score.Completeness)
	}
	if score.Total() < 0.5 {
		t.Errorf("expected total > 0.5 for complete state, got %f", score.Total())
	}
}

func TestNilStateHasZeroConfidence(t *testing.T) {
	calc := NewCalculator()
	score := calc.Calculate(nil)
	if score.Total() != 0 {
		t.Errorf("expected 0 for nil state, got %f", score.Total())
	}
}

func TestResearchScoring(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	calc := NewCalculator()

	// No research context = 0
	score := calc.Calculate(state)
	if score.Research != 0 {
		t.Errorf("expected 0 research without context, got %f", score.Research)
	}

	// With research context
	state.ResearchCtx = &arbiter.QuickScanResult{
		GitHubHits: []arbiter.GitHubFinding{{Name: "test", Stars: 100}},
		HNHits:     []arbiter.HNFinding{{Title: "test", Points: 50}},
		Summary:    "Found relevant projects",
	}
	score = calc.Calculate(state)
	if score.Research != 1.0 {
		t.Errorf("expected 1.0 research with full context, got %f", score.Research)
	}
}

func TestSpecificityScoring(t *testing.T) {
	state := arbiter.NewSprintState("/tmp/test")
	state.Sections[arbiter.PhaseProblem].Content = "Things are hard"
	state.Sections[arbiter.PhaseProblem].Status = arbiter.DraftAccepted

	calc := NewCalculator()
	score1 := calc.Calculate(state)

	state.Sections[arbiter.PhaseProblem].Content = "Developers spend 2 hours daily on manual deployments, costing $50k/year in lost productivity"
	score2 := calc.Calculate(state)

	if score2.Specificity <= score1.Specificity {
		t.Errorf("expected specific content to score higher: vague=%f, specific=%f", score1.Specificity, score2.Specificity)
	}
}
