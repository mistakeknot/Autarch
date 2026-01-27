package spec

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestSpecFlowAnalyzer_WellFormedSpec(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-001",
		Title:   "User Login",
		Summary: "Allow users to log in with email and password",
		Requirements: []string{
			"User must be able to enter email and password",
			"System must validate credentials against database",
			"User must see error message for invalid credentials",
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "When user enters valid email and password, they are redirected to dashboard"},
			{ID: "AC-2", Description: "When user enters invalid email format, error message 'Invalid email' displays"},
			{ID: "AC-3", Description: "When user enters wrong password, error message 'Invalid credentials' displays"},
		},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:       "CUJ-1",
				Title:    "Successful Login",
				Priority: "p0",
				Steps: []string{
					"User navigates to login page",
					"User enters email address",
					"User enters password",
					"User clicks submit button",
					"System validates credentials",
					"System redirects to dashboard",
				},
				SuccessCriteria: []string{
					"User sees dashboard within 2 seconds",
					"User session is created",
				},
			},
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	if result.SpecID != "PRD-001" {
		t.Errorf("SpecID = %q, want %q", result.SpecID, "PRD-001")
	}

	if result.Blockers > 0 {
		t.Errorf("expected no blockers for well-formed spec, got %d", result.Blockers)
		for _, gap := range result.Gaps {
			if gap.Severity == SeverityBlocker {
				t.Logf("  Blocker: %s", gap.Description)
			}
		}
	}

	if !result.ReadyForImplementation {
		t.Error("expected spec to be ready for implementation")
	}

	if result.Coverage < 0.7 {
		t.Errorf("coverage %.2f should be >= 0.7 for well-formed spec", result.Coverage)
	}
}

func TestSpecFlowAnalyzer_NoAcceptanceCriteria(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-002",
		Title: "Test",
		Requirements: []string{
			"User must log in",
			"User must log out",
		},
		Acceptance: []specs.AcceptanceCriterion{}, // Empty
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should have a blocker for missing acceptance criteria
	hasBlocker := false
	for _, gap := range result.Gaps {
		if gap.Severity == SeverityBlocker && gap.Category == CategoryUnclearCriteria {
			hasBlocker = true
			break
		}
	}

	if !hasBlocker {
		t.Error("expected blocker for missing acceptance criteria")
	}

	if result.ReadyForImplementation {
		t.Error("spec without acceptance criteria should not be ready for implementation")
	}
}

func TestSpecFlowAnalyzer_IncompleteCUJ(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-003",
		Title: "Test",
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:    "CUJ-1",
				Title: "Too Short",
				Steps: []string{
					"Click button", // No actor, too short
					"Done",         // No actor
				},
				SuccessCriteria: []string{
					"It works", // Not measurable
				},
			},
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should flag incomplete CUJ
	hasMissingFlow := false
	hasUnclearCriteria := false

	for _, gap := range result.Gaps {
		if gap.Category == CategoryMissingFlow {
			hasMissingFlow = true
		}
		if gap.Category == CategoryUnclearCriteria {
			hasUnclearCriteria = true
		}
	}

	if !hasMissingFlow {
		t.Error("expected warning about incomplete CUJ flow")
	}

	if !hasUnclearCriteria {
		t.Error("expected warning about unmeasurable success criteria")
	}
}

func TestSpecFlowAnalyzer_MissingErrorHandling(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-004",
		Title:   "User Registration",
		Summary: "Allow users to create accounts",
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{
				ID:    "CUJ-1",
				Title: "Happy Path Registration",
				Steps: []string{
					"User enters email",
					"User enters password",
					"User clicks register",
					"System creates account",
				},
				SuccessCriteria: []string{
					"Account is created",
				},
			},
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "User can register with valid email"},
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should flag missing error handling
	hasErrorGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryErrorHandling {
			hasErrorGap = true
			break
		}
	}

	if !hasErrorGap {
		t.Error("expected warning about missing error handling")
	}
}

func TestSpecFlowAnalyzer_EdgeCases(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-005",
		Title:   "Search Results",
		Summary: "Display search results to user",
		Requirements: []string{
			"User can search for products",
			"System displays list of matching products",
		},
		// No mention of empty results
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should suggest edge case for empty search results
	hasEdgeCaseGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryEdgeCase {
			hasEdgeCaseGap = true
			break
		}
	}

	if !hasEdgeCaseGap {
		t.Error("expected suggestion about search edge cases (no results)")
	}
}

func TestSpecFlowAnalyzer_IntegrationPoints(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-006",
		Title:   "Payment Integration",
		Summary: "Integrate with Stripe API for payments",
		Requirements: []string{
			"System must connect to external payment API",
			"System must process credit card payments",
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should flag missing integration details
	hasIntegrationGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryIntegrationPoint {
			hasIntegrationGap = true
			break
		}
	}

	if !hasIntegrationGap {
		t.Error("expected warning about missing integration details")
	}
}

func TestSpecFlowAnalyzer_StateTransitions(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-007",
		Title:   "Order Status",
		Summary: "Track order status through various stages",
		Requirements: []string{
			"Order can be in pending, processing, shipped, or delivered status",
			"Admin can update order status",
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should suggest state transition diagram
	hasStateGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryStateTransition {
			hasStateGap = true
			break
		}
	}

	if !hasStateGap {
		t.Error("expected suggestion about state transitions")
	}
}

func TestSpecFlowAnalyzer_DataValidation(t *testing.T) {
	spec := &specs.Spec{
		ID:      "PRD-008",
		Title:   "User Profile",
		Summary: "Allow users to update their profile",
		Requirements: []string{
			"User can update their email address",
			"User can update their phone number",
		},
		// No validation rules specified
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should flag missing validation rules
	hasValidationGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryDataValidation {
			hasValidationGap = true
			break
		}
	}

	if !hasValidationGap {
		t.Error("expected suggestion about data validation rules")
	}
}

func TestSpecFlowAnalyzer_LowACRatio(t *testing.T) {
	spec := &specs.Spec{
		ID:    "PRD-009",
		Title: "Test",
		Requirements: []string{
			"Req 1", "Req 2", "Req 3", "Req 4", "Req 5",
			"Req 6", "Req 7", "Req 8", "Req 9", "Req 10",
		},
		Acceptance: []specs.AcceptanceCriterion{
			{ID: "AC-1", Description: "Something works"},
			{ID: "AC-2", Description: "Something else works"},
		},
	}

	analyzer := NewSpecFlowAnalyzer()
	result := analyzer.Analyze(spec)

	// Should flag low AC ratio
	hasLowRatioGap := false
	for _, gap := range result.Gaps {
		if gap.Category == CategoryUnclearCriteria && gap.Location == "acceptance_criteria" {
			hasLowRatioGap = true
			break
		}
	}

	if !hasLowRatioGap {
		t.Error("expected warning about low acceptance criteria ratio")
	}
}
