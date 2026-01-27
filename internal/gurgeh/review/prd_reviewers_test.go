package review

import (
	"context"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestRunParallelReview(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-001",
		Title:   "Test PRD",
		Summary: "This is a test PRD with sufficient detail for review",
		Requirements: []string{
			"User must be able to log in",
			"System must validate credentials",
		},
		UserStory: specs.UserStory{
			Text: "As a user, I want to log in so that I can access my account",
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "When user enters valid credentials, they are redirected to dashboard"},
			{ID: "AC-2", Description: "When user enters invalid credentials, error message is displayed"},
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:              "CUJ-1",
				Title:           "Login Flow",
				Priority:        "p0",
				Steps:           []string{"User navigates to login page", "User enters credentials", "User clicks submit"},
				SuccessCriteria: []string{"User sees dashboard"},
			},
		},
		Goals: []specs.Goal{
			{ID: "GOAL-1", Description: "Reduce login time to under 2 seconds", Metric: "login_time", Target: "< 2s"},
		},
		NonGoals: []specs.NonGoal{
			{ID: "NG-1", Description: "Social login integration", Rationale: "Phase 2 feature"},
		},
		Assumptions: []specs.Assumption{
			{ID: "ASSM-1", Description: "Users have email accounts", ImpactIfFalse: "Need alternative auth", Confidence: "high"},
		},
	}

	reviewers := DefaultReviewers()
	result, err := RunParallelReview(context.Background(), spec, reviewers)
	if err != nil {
		t.Fatalf("RunParallelReview failed: %v", err)
	}

	if result.SpecID != "PRD-001" {
		t.Errorf("SpecID = %q, want %q", result.SpecID, "PRD-001")
	}

	if len(result.Results) != 4 {
		t.Errorf("got %d results, want 4 (one per reviewer)", len(result.Results))
	}

	// A well-formed PRD should pass
	if !result.ReadyForImplementation {
		t.Errorf("expected PRD to be ready for implementation, but it wasn't (score %.2f)", result.OverallScore)
		for _, r := range result.Results {
			t.Logf("  %s: score=%.2f, issues=%d", r.Reviewer, r.Score, len(r.Issues))
			for _, issue := range r.Issues {
				t.Logf("    - [%s] %s: %s", issue.Severity, issue.Category, issue.Description)
			}
		}
	}
}

func TestCompletenessReviewer_MissingFields(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-002",
		Title: "", // Missing
		// Summary missing
		// Requirements missing
	}

	reviewer := NewCompletenessReviewer()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should have errors for missing title, summary, requirements
	errorCount := 0
	for _, issue := range result.Issues {
		if issue.Severity == SeverityError {
			errorCount++
		}
	}

	if errorCount < 3 {
		t.Errorf("expected at least 3 errors for missing fields, got %d", errorCount)
	}

	if result.Score > 0.5 {
		t.Errorf("score %.2f should be <= 0.5 for PRD missing required fields", result.Score)
	}
}

func TestCUJConsistencyReviewer_VagueSteps(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-003",
		Title: "Test",
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:       "CUJ-1",
				Title:    "Vague Journey",
				Priority: "p1",
				Steps: []string{
					"click",
					"enter",
					"proceed",
				},
				// No success criteria
			},
		},
	}

	reviewer := NewCUJConsistencyReviewer()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should flag vague steps and missing success criteria
	hasVagueSteps := false
	hasMissingCriteria := false

	for _, issue := range result.Issues {
		if issue.Category == "vague-cuj-steps" {
			hasVagueSteps = true
		}
		if issue.Category == "missing-success-criteria" {
			hasMissingCriteria = true
		}
	}

	if !hasVagueSteps {
		t.Error("expected warning about vague CUJ steps")
	}

	if !hasMissingCriteria {
		t.Error("expected warning about missing success criteria")
	}
}

func TestAcceptanceCriteriaReviewer_VagueCriteria(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-004",
		Title: "Test",
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "System should work properly and be user-friendly"},
			{ID: "AC-2", Description: "Performance should be acceptable and efficient"},
		},
	}

	reviewer := NewAcceptanceCriteriaReviewer()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should flag vague criteria
	vagueCount := 0
	for _, issue := range result.Issues {
		if issue.Category == "vague-criterion" {
			vagueCount++
		}
	}

	if vagueCount < 2 {
		t.Errorf("expected at least 2 vague criterion warnings, got %d", vagueCount)
	}
}

func TestScopeCreepDetector_TooManyRequirements(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-005",
		Title: "Test",
		Requirements: []string{
			"Req 1", "Req 2", "Req 3", "Req 4", "Req 5",
			"Req 6", "Req 7", "Req 8", "Req 9", "Req 10",
			"Req 11", "Req 12", "Req 13", "Req 14", "Req 15",
			"Req 16", "Req 17", "Req 18",
		},
	}

	reviewer := NewScopeCreepDetector()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should warn about too many requirements
	hasTooManyWarning := false
	for _, issue := range result.Issues {
		if issue.Category == "too-many-requirements" {
			hasTooManyWarning = true
			break
		}
	}

	if !hasTooManyWarning {
		t.Error("expected warning about too many requirements")
	}
}

func TestScopeCreepDetector_GoalNonGoalConflict(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-006",
		Title: "Test",
		Goals: []specs.Goal{
			{ID: "GOAL-1", Description: "Implement social login with OAuth providers for easy authentication"},
		},
		NonGoals: []specs.NonGoal{
			{ID: "NG-1", Description: "Social login with OAuth providers is out of scope for this release"},
		},
	}

	reviewer := NewScopeCreepDetector()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should detect goal/non-goal conflict
	hasConflict := false
	for _, issue := range result.Issues {
		if issue.Category == "goal-nongoal-conflict" {
			hasConflict = true
			break
		}
	}

	if !hasConflict {
		t.Error("expected error about goal/non-goal conflict")
	}
}

func TestScopeCreepDetector_RequirementMatchesNonGoal(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-007",
		Title: "Test",
		Requirements: []string{
			"System must support social login with OAuth providers like Google and Facebook",
		},
		NonGoals: []specs.NonGoal{
			{ID: "NG-1", Description: "Social login with OAuth providers is out of scope"},
		},
	}

	reviewer := NewScopeCreepDetector()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should detect requirement matching non-goal
	hasConflict := false
	for _, issue := range result.Issues {
		if issue.Category == "requirement-matches-nongoal" {
			hasConflict = true
			break
		}
	}

	if !hasConflict {
		t.Error("expected error about requirement matching non-goal")
	}
}

func TestAcceptanceCriteriaReviewer_GoodCriteria(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-008",
		Title: "Test",
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "Given a user is on the login page, when they enter valid credentials and click submit, then they are redirected to the dashboard within 2 seconds"},
			{ID: "AC-2", Description: "When user enters an invalid password, error message 'Invalid credentials' is displayed"},
		},
	}

	reviewer := NewAcceptanceCriteriaReviewer()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should have high score for good criteria
	if result.Score < 0.9 {
		t.Errorf("score %.2f should be >= 0.9 for well-written acceptance criteria", result.Score)
		for _, issue := range result.Issues {
			t.Logf("  - [%s] %s: %s", issue.Severity, issue.Category, issue.Description)
		}
	}
}

func TestCUJConsistencyReviewer_BrokenRequirementLink(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-009",
		Title: "Test",
		Requirements: []string{
			"User must log in",
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:                 "CUJ-1",
				Title:              "Login",
				Steps:              []string{"User enters credentials and submits the login form"},
				SuccessCriteria:    []string{"User is logged in"},
				LinkedRequirements: []string{"REQ-999"}, // Non-existent
			},
		},
	}

	reviewer := NewCUJConsistencyReviewer()
	result, err := reviewer.Review(context.Background(), spec)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Should flag broken requirement link
	hasBrokenLink := false
	for _, issue := range result.Issues {
		if issue.Category == "broken-requirement-link" {
			hasBrokenLink = true
			break
		}
	}

	if !hasBrokenLink {
		t.Error("expected warning about broken requirement link")
	}
}
