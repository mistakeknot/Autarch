package validation

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestProductValidator_NoValueProp(t *testing.T) {
	v := &ProductValidator{}

	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Spec",
		Summary: "Short", // Too short
	}

	result := v.Validate(spec)

	if result.Approved {
		t.Error("expected not approved when summary is too short")
	}

	found := false
	for _, c := range result.Concerns {
		if c.Category == "value_prop" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected value_prop concern")
	}
}

func TestProductValidator_NoMetrics(t *testing.T) {
	v := &ProductValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Summary:      "This is a sufficiently long summary for the value proposition",
		Requirements: []string{"Add a button", "Show a list"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "metrics" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected metrics concern when no success metrics defined")
	}
}

func TestProductValidator_LargeScope(t *testing.T) {
	v := &ProductValidator{}

	reqs := make([]string, 20)
	for i := range reqs {
		reqs[i] = "Requirement " + string(rune('A'+i))
	}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Summary:      "This is a sufficiently long summary for the value proposition",
		Requirements: reqs,
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "scope" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected scope concern for large requirement count")
	}
}

func TestProductValidator_WithMetrics(t *testing.T) {
	v := &ProductValidator{}

	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Spec",
		Summary: "This is a sufficiently long summary for the value proposition",
		Requirements: []string{
			"Track user engagement metrics",
			"Measure conversion rate",
		},
	}

	result := v.Validate(spec)

	for _, c := range result.Concerns {
		if c.Category == "metrics" {
			t.Error("should not have metrics concern when metrics are defined")
		}
	}
}

func TestDesignValidator_NoA11y(t *testing.T) {
	v := &DesignValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Summary:      "A UI feature",
		Requirements: []string{"Display a form with buttons", "Show error messages"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "a11y" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a11y concern when UI elements present without accessibility")
	}
}

func TestDesignValidator_WithA11y(t *testing.T) {
	v := &DesignValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Summary:      "A UI feature",
		Requirements: []string{"Display a form with buttons", "Ensure WCAG 2.1 AA compliance"},
	}

	result := v.Validate(spec)

	for _, c := range result.Concerns {
		if c.Category == "a11y" {
			t.Error("should not have a11y concern when accessibility is mentioned")
		}
	}
}

func TestDesignValidator_ComplexInteractions(t *testing.T) {
	v := &DesignValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Summary:      "Drag and drop interface",
		Requirements: []string{"Support drag and drop reordering", "Animate transitions"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "complexity" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected complexity concern for complex interactions")
	}
}

func TestEngineeringValidator_ExternalDependencies(t *testing.T) {
	v := &EngineeringValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Requirements: []string{"Integrate with Stripe API", "Connect to third-party auth"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "dependencies" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected dependencies concern for external integrations")
	}
}

func TestEngineeringValidator_DataStorage(t *testing.T) {
	v := &EngineeringValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Requirements: []string{"Store user preferences", "Persist session data"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "data" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected data concern when storage mentioned without schema")
	}
}

func TestEngineeringValidator_Security(t *testing.T) {
	v := &EngineeringValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Requirements: []string{"User authentication via password", "Admin role management"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "security" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected security concern for auth-related requirements")
	}
}

func TestEngineeringValidator_Performance(t *testing.T) {
	v := &EngineeringValidator{}

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Spec",
		Requirements: []string{"Real-time updates", "Handle millions of records"},
	}

	result := v.Validate(spec)

	found := false
	for _, c := range result.Concerns {
		if c.Category == "performance" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected performance concern for real-time/scale requirements")
	}
}

func TestBroker_ValidateSpec(t *testing.T) {
	broker := NewBroker()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Feature",
		Summary:      "This is a comprehensive summary explaining the feature value",
		Requirements: []string{"Display user dashboard", "Track metrics for engagement"},
	}

	report := broker.ValidateSpec(spec)

	if report.SpecID != "SPEC-001" {
		t.Errorf("SpecID = %s, want SPEC-001", report.SpecID)
	}
	if len(report.Results) != 3 {
		t.Errorf("Results = %d, want 3 perspectives", len(report.Results))
	}
}

func TestBroker_ValidatePRD(t *testing.T) {
	broker := NewBroker()

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "MVP Release - First release with core features for user management",
		Version: "mvp",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "User Dashboard",
				Requirements: []string{"Display user stats", "Show recent activity"},
			},
		},
	}

	report := broker.ValidatePRD(prd)

	if report.SpecID != "MVP" {
		t.Errorf("SpecID = %s, want MVP", report.SpecID)
	}
}

func TestBroker_DetectConflicts(t *testing.T) {
	broker := NewBroker()

	// Spec that triggers concerns from multiple perspectives on same category
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Complex Feature",
		Summary:      "This is a comprehensive summary explaining the feature value",
		Requirements: []string{
			"Real-time collaboration interface",      // Engineering: performance
			"Handle millions of concurrent users",    // Engineering: performance
			"Display instant updates",                // Engineering: performance, Design: complexity
			"Animate all transitions smoothly",       // Design: complexity
			"Drag and drop with live preview",        // Design: complexity
		},
	}

	report := broker.ValidateSpec(spec)

	// Should have some concerns (at minimum about complexity and performance)
	totalConcerns := 0
	for _, result := range report.Results {
		totalConcerns += len(result.Concerns)
	}

	if totalConcerns == 0 {
		t.Error("expected at least some concerns for complex spec")
	}
}

func TestFormatAlignmentReport(t *testing.T) {
	report := &AlignmentReport{
		SpecID:          "SPEC-001",
		OverallApproved: false,
		Results: []ValidationResult{
			{
				Perspective: PerspectiveProduct,
				Approved:    false,
				Concerns: []ValidationConcern{
					{
						Perspective: PerspectiveProduct,
						Severity:    SeverityCritical,
						Category:    "value_prop",
						Title:       "Missing value proposition",
						Description: "No clear value",
						Suggestion:  "Add summary",
					},
				},
			},
			{
				Perspective: PerspectiveDesign,
				Approved:    true,
			},
			{
				Perspective: PerspectiveEngineering,
				Approved:    true,
			},
		},
		Conflicts: []Conflict{
			{
				Perspectives: []Perspective{PerspectiveProduct, PerspectiveEngineering},
				Topic:        "scope",
				Description:  "Product wants more, eng says too complex",
			},
		},
		Summary: "✗ Spec requires attention",
	}

	output := FormatAlignmentReport(report)

	if !strings.Contains(output, "Stakeholder Alignment Report") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "SPEC-001") {
		t.Error("should contain spec ID")
	}
	if !strings.Contains(output, "Product Perspective") {
		t.Error("should contain product perspective")
	}
	if !strings.Contains(output, "Missing value proposition") {
		t.Error("should contain concern title")
	}
	if !strings.Contains(output, "Cross-Perspective Conflicts") {
		t.Error("should contain conflicts section")
	}
}

func TestBuildBrokerBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Feature",
		Summary:      "A test feature summary",
		Requirements: []string{"Req 1", "Req 2"},
		CriticalUserJourneys: []specs.CriticalUserJourney{
			{ID: "CUJ-001", Title: "User Onboarding"},
		},
	}

	brief := BuildBrokerBrief(spec)

	if !strings.Contains(brief, "Broker Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "SPEC-001") {
		t.Error("should contain spec ID")
	}
	if !strings.Contains(brief, "Req 1") {
		t.Error("should contain requirements")
	}
	if !strings.Contains(brief, "CUJ-001") {
		t.Error("should contain CUJs")
	}
	if !strings.Contains(brief, "Product") {
		t.Error("should contain validation perspectives")
	}
}

func TestContainsUIElements(t *testing.T) {
	tests := []struct {
		name     string
		spec     *specs.Spec
		expected bool
	}{
		{
			name:     "has UI elements",
			spec:     &specs.Spec{Requirements: []string{"Display a form"}},
			expected: true,
		},
		{
			name:     "no UI elements",
			spec:     &specs.Spec{Requirements: []string{"Process data"}},
			expected: false,
		},
		{
			name:     "UI in summary",
			spec:     &specs.Spec{Summary: "A page with buttons"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsUIElements(tt.spec)
			if result != tt.expected {
				t.Errorf("containsUIElements() = %v, want %v", result, tt.expected)
			}
		})
	}
}
