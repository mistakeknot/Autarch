package acceptance

import (
	"strings"
	"testing"
)

func TestGenerateFromNarrative_UserStoryPattern(t *testing.T) {
	gen := NewGenerator()

	title := "User Registration"
	description := "As a new visitor, I want to create an account, so that I can access member features."

	criteria := gen.GenerateFromNarrative("STORY-001", title, description)

	if len(criteria) == 0 {
		t.Error("expected at least one criterion from user story pattern")
	}

	first := criteria[0]
	if first.StoryID != "STORY-001" {
		t.Errorf("StoryID = %v, want STORY-001", first.StoryID)
	}
	if !strings.Contains(first.Given, "new visitor") {
		t.Errorf("Given should contain user type, got: %v", first.Given)
	}
	if !strings.Contains(first.When, "create an account") {
		t.Errorf("When should contain action, got: %v", first.When)
	}
	if !strings.Contains(first.Then, "access member features") {
		t.Errorf("Then should contain outcome, got: %v", first.Then)
	}
}

func TestGenerateFromNarrative_ShouldStatements(t *testing.T) {
	gen := NewGenerator()

	title := "Error Handling"
	description := `
The system should display an error message when validation fails.
The form should preserve user input on error.
The submit button should be disabled during submission.
`

	criteria := gen.GenerateFromNarrative("STORY-002", title, description)

	// Should extract "should" statements
	foundShouldCriteria := 0
	for _, c := range criteria {
		if strings.Contains(c.Then, "should") {
			foundShouldCriteria++
		}
	}

	if foundShouldCriteria < 2 {
		t.Errorf("expected at least 2 'should' criteria, got %d", foundShouldCriteria)
	}
}

func TestValidate(t *testing.T) {
	gen := NewGenerator()

	tests := []struct {
		name       string
		criterion  Criterion
		wantValid  bool
		wantErrors int
	}{
		{
			name: "valid complete criterion",
			criterion: Criterion{
				StoryID:   "STORY-001",
				Given:     "user is logged in",
				When:      "user clicks logout",
				Then:      "user is redirected to login",
				EdgeCases: []string{"session expired"},
			},
			wantValid:  true,
			wantErrors: 0,
		},
		{
			name: "missing given",
			criterion: Criterion{
				StoryID: "STORY-001",
				When:    "user clicks",
				Then:    "something happens",
			},
			wantValid:  false,
			wantErrors: 1,
		},
		{
			name: "missing all clauses",
			criterion: Criterion{
				StoryID: "STORY-001",
			},
			wantValid:  false,
			wantErrors: 3, // given, when, then
		},
		{
			name: "missing story ID",
			criterion: Criterion{
				Given: "user is logged in",
				When:  "user clicks",
				Then:  "something happens",
			},
			wantValid:  false,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.Validate(&tt.criterion)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if len(result.Errors) != tt.wantErrors {
				t.Errorf("Errors = %v, want %d errors", result.Errors, tt.wantErrors)
			}
		})
	}
}

func TestFormatGherkin(t *testing.T) {
	c := &Criterion{
		ID:      "AC-001",
		StoryID: "STORY-001",
		Given:   "user is logged in",
		When:    "user clicks the share button",
		Then:    "share dialog appears",
		And:     []string{"dialog shows social options"},
	}

	result := FormatGherkin(c)

	if !strings.Contains(result, "Scenario: AC-001") {
		t.Error("should contain scenario ID")
	}
	if !strings.Contains(result, "Given user is logged in") {
		t.Error("should contain given clause")
	}
	if !strings.Contains(result, "When user clicks the share button") {
		t.Error("should contain when clause")
	}
	if !strings.Contains(result, "Then share dialog appears") {
		t.Error("should contain then clause")
	}
	if !strings.Contains(result, "And dialog shows social options") {
		t.Error("should contain and clause")
	}
}

func TestFormatGherkinAll(t *testing.T) {
	criteria := []Criterion{
		{ID: "AC-001", Given: "user is logged in", When: "clicks share", Then: "dialog opens"},
		{ID: "AC-002", Given: "dialog is open", When: "clicks twitter", Then: "twitter auth starts"},
	}

	result := FormatGherkinAll("Social Sharing", criteria)

	if !strings.Contains(result, "Feature: Social Sharing") {
		t.Error("should contain feature title")
	}
	if !strings.Contains(result, "Scenario: AC-001") {
		t.Error("should contain first scenario")
	}
	if !strings.Contains(result, "Scenario: AC-002") {
		t.Error("should contain second scenario")
	}
}

func TestFormatGherkinAll_Empty(t *testing.T) {
	result := FormatGherkinAll("Empty Story", nil)
	if result != "" {
		t.Errorf("expected empty string for no criteria, got: %v", result)
	}
}

func TestBuildSmithBrief(t *testing.T) {
	req := SmithRequest{
		StoryID:     "STORY-001",
		StoryTitle:  "User Login",
		Description: "User should be able to log in with email and password",
		Context:     "Part of authentication epic",
	}

	brief := BuildSmithBrief(req)

	if !strings.Contains(brief, "STORY-001") {
		t.Error("brief should contain story ID")
	}
	if !strings.Contains(brief, "User Login") {
		t.Error("brief should contain story title")
	}
	if !strings.Contains(brief, "authentication epic") {
		t.Error("brief should contain context")
	}
	if !strings.Contains(brief, "Given/When/Then") {
		t.Error("brief should mention Gherkin format")
	}
}
