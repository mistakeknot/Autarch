package arbiter

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestMigrateFromSpec(t *testing.T) {
	spec := &specs.Spec{
		Summary: "Test problem statement",
		UserStory: specs.UserStory{
			Text: "As a user, I want to test this feature",
		},
		Title: "Test Feature",
		Goals: []specs.Goal{
			{
				ID:          "GOAL-001",
				Description: "Improve performance",
				Metric:      "Load time",
				Target:      "< 1s",
			},
		},
		Requirements: []string{
			"Must work on mobile",
			"Must support offline mode",
		},
		NonGoals: []specs.NonGoal{
			{
				ID:          "NG-001",
				Description: "Desktop optimization",
				Rationale:   "Out of scope for MVP",
			},
		},
		Assumptions: []specs.Assumption{
			{
				ID:            "ASSM-001",
				Description:   "Users have modern browsers",
				ImpactIfFalse: "Feature may not work",
				Confidence:    "high",
			},
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:    "CUJ-001",
				Title: "First-time setup",
				Priority: "high",
				Steps: []string{
					"Open app",
					"Enter credentials",
					"See dashboard",
				},
				SuccessCriteria: []string{
					"User sees dashboard within 2 seconds",
					"No errors shown",
				},
			},
		},
		Acceptance: []specs.AcceptanceCriterion{
			{
				ID:          "AC-001",
				Description: "Feature works on Chrome",
			},
			{
				ID:          "AC-002",
				Description: "No console errors",
			},
		},
	}

	state := MigrateFromSpec(spec, "/tmp/test")

	// Check Problem section
	if state.Sections[PhaseProblem].Content != "Test problem statement" {
		t.Errorf("expected problem content from spec summary, got: %s",
			state.Sections[PhaseProblem].Content)
	}
	if state.Sections[PhaseProblem].Status != DraftAccepted {
		t.Error("expected accepted status for migrated problem content")
	}

	// Check Users section
	if state.Sections[PhaseUsers].Content != "As a user, I want to test this feature" {
		t.Errorf("expected user story content, got: %s",
			state.Sections[PhaseUsers].Content)
	}
	if state.Sections[PhaseUsers].Status != DraftAccepted {
		t.Error("expected accepted status for migrated user content")
	}

	// Check Features+Goals section
	featuresContent := state.Sections[PhaseFeaturesGoals].Content
	if !strings.Contains(featuresContent, "Test Feature") {
		t.Error("expected title in features+goals section")
	}
	if !strings.Contains(featuresContent, "Improve performance") {
		t.Error("expected goal in features+goals section")
	}
	if !strings.Contains(featuresContent, "Must work on mobile") {
		t.Error("expected requirement in features+goals section")
	}

	// Check Scope+Assumptions section
	scopeContent := state.Sections[PhaseScopeAssumptions].Content
	if !strings.Contains(scopeContent, "Desktop optimization") {
		t.Error("expected non-goal in scope+assumptions section")
	}
	if !strings.Contains(scopeContent, "Users have modern browsers") {
		t.Error("expected assumption in scope+assumptions section")
	}

	// Check CUJs section
	cujContent := state.Sections[PhaseCUJs].Content
	if !strings.Contains(cujContent, "First-time setup") {
		t.Error("expected CUJ title in CUJs section")
	}
	if !strings.Contains(cujContent, "Open app") {
		t.Error("expected CUJ steps in CUJs section")
	}

	// Check Acceptance Criteria section
	acceptanceContent := state.Sections[PhaseAcceptanceCriteria].Content
	if !strings.Contains(acceptanceContent, "AC-001") {
		t.Error("expected AC-001 in acceptance section")
	}
	if !strings.Contains(acceptanceContent, "Feature works on Chrome") {
		t.Error("expected AC description in acceptance section")
	}
}

func TestMigrateFromNilSpec(t *testing.T) {
	state := MigrateFromSpec(nil, "/tmp/test")
	if state == nil {
		t.Error("expected non-nil state for nil spec")
	}

	// All sections should be initialized with DraftPending status
	for _, phase := range AllPhases() {
		if state.Sections[phase] == nil {
			t.Errorf("expected initialized section for phase %s", phase.String())
		}
		if state.Sections[phase].Status != DraftPending {
			t.Errorf("expected DraftPending status for phase %s, got %v",
				phase.String(), state.Sections[phase].Status)
		}
	}
}

func TestMigrateFromEmptySpec(t *testing.T) {
	spec := &specs.Spec{}
	state := MigrateFromSpec(spec, "/tmp/test")

	// All sections should remain in DraftPending status
	for _, phase := range AllPhases() {
		if state.Sections[phase].Status != DraftPending {
			t.Errorf("expected DraftPending for empty spec phase %s",
				phase.String())
		}
	}
}

func TestMigratePreservesProjectPath(t *testing.T) {
	spec := &specs.Spec{Summary: "Test"}
	projectPath := "/my/project/path"

	state := MigrateFromSpec(spec, projectPath)
	if state.ProjectPath != projectPath {
		t.Errorf("expected project path %s, got %s",
			projectPath, state.ProjectPath)
	}
}

func TestMigratePartialSpec(t *testing.T) {
	spec := &specs.Spec{
		Summary: "Only summary",
		Goals: []specs.Goal{
			{Description: "Single goal"},
		},
	}

	state := MigrateFromSpec(spec, "/tmp/test")

	// Problem should be accepted
	if state.Sections[PhaseProblem].Status != DraftAccepted {
		t.Error("expected accepted problem section")
	}

	// Users should remain pending
	if state.Sections[PhaseUsers].Status != DraftPending {
		t.Error("expected pending user section")
	}

	// Features+Goals should be accepted
	if state.Sections[PhaseFeaturesGoals].Status != DraftAccepted {
		t.Error("expected accepted features+goals section")
	}

	// Others should remain pending
	if state.Sections[PhaseScopeAssumptions].Status != DraftPending {
		t.Error("expected pending scope+assumptions section")
	}
}
