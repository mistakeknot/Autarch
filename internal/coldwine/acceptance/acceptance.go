// Package acceptance provides Gherkin-style acceptance criteria generation
// for Coldwine stories. The "smith" subagent uses this to forge testable
// criteria from narrative story text.
package acceptance

import (
	"fmt"
	"strings"
	"time"
)

// Criterion represents a Gherkin-style acceptance criterion
type Criterion struct {
	ID          string    `yaml:"id" json:"id"`                                         // AC-001
	StoryID     string    `yaml:"story_id" json:"story_id"`                             // STORY-001
	Given       string    `yaml:"given" json:"given"`                                   // User is logged in
	When        string    `yaml:"when" json:"when"`                                     // User clicks share button
	Then        string    `yaml:"then" json:"then"`                                     // Tweet compose dialog opens
	And         []string  `yaml:"and,omitempty" json:"and,omitempty"`                   // Additional conditions
	EdgeCases   []string  `yaml:"edge_cases,omitempty" json:"edge_cases,omitempty"`     // Revoked token, rate limit
	TestCommand string    `yaml:"test_command,omitempty" json:"test_command,omitempty"` // Optional: test to run
	CreatedAt   time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt   time.Time `yaml:"updated_at" json:"updated_at"`
}

// ValidationResult contains the result of validating acceptance criteria
type ValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// Generator generates acceptance criteria from story text
type Generator struct{}

// NewGenerator creates a new acceptance criteria generator
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateFromNarrative extracts potential acceptance criteria from narrative text.
// This is a heuristic approach - the smith subagent provides AI-assisted extraction.
func (g *Generator) GenerateFromNarrative(storyID, title, description string) []Criterion {
	var criteria []Criterion

	// Extract user story pattern: "As a X, I want Y, so that Z"
	if given, when, then := extractUserStoryPattern(title, description); given != "" {
		criteria = append(criteria, Criterion{
			ID:        fmt.Sprintf("%s-AC-001", storyID),
			StoryID:   storyID,
			Given:     given,
			When:      when,
			Then:      then,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}

	// Look for "should" statements
	shouldCriteria := extractShouldStatements(storyID, description)
	criteria = append(criteria, shouldCriteria...)

	// Look for "when... then" patterns
	whenThenCriteria := extractWhenThenPatterns(storyID, description)
	criteria = append(criteria, whenThenCriteria...)

	return criteria
}

// Validate checks if an acceptance criterion is complete
func (g *Generator) Validate(c *Criterion) ValidationResult {
	result := ValidationResult{Valid: true}

	if strings.TrimSpace(c.Given) == "" {
		result.Errors = append(result.Errors, "Given clause is required")
		result.Valid = false
	}
	if strings.TrimSpace(c.When) == "" {
		result.Errors = append(result.Errors, "When clause is required")
		result.Valid = false
	}
	if strings.TrimSpace(c.Then) == "" {
		result.Errors = append(result.Errors, "Then clause is required")
		result.Valid = false
	}
	if strings.TrimSpace(c.StoryID) == "" {
		result.Errors = append(result.Errors, "StoryID is required")
		result.Valid = false
	}

	// Warnings
	if len(c.EdgeCases) == 0 {
		result.Warnings = append(result.Warnings, "No edge cases defined")
	}
	if c.TestCommand == "" {
		result.Warnings = append(result.Warnings, "No test command specified")
	}

	return result
}

// FormatGherkin formats acceptance criteria as Gherkin syntax
func FormatGherkin(c *Criterion) string {
	var sb strings.Builder

	sb.WriteString("Scenario: ")
	sb.WriteString(c.ID)
	sb.WriteString("\n")

	sb.WriteString("  Given ")
	sb.WriteString(c.Given)
	sb.WriteString("\n")

	sb.WriteString("  When ")
	sb.WriteString(c.When)
	sb.WriteString("\n")

	sb.WriteString("  Then ")
	sb.WriteString(c.Then)
	sb.WriteString("\n")

	for _, and := range c.And {
		sb.WriteString("  And ")
		sb.WriteString(and)
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatGherkinAll formats multiple criteria as a Gherkin feature
func FormatGherkinAll(storyTitle string, criteria []Criterion) string {
	if len(criteria) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("Feature: ")
	sb.WriteString(storyTitle)
	sb.WriteString("\n\n")

	for _, c := range criteria {
		sb.WriteString(FormatGherkin(&c))
		sb.WriteString("\n")
	}

	return sb.String()
}

// --- Pattern extraction helpers ---

// extractUserStoryPattern looks for "As a X, I want Y, so that Z" pattern
func extractUserStoryPattern(title, description string) (given, when, then string) {
	combined := title + " " + description
	lower := strings.ToLower(combined)

	// Look for "as a" ... "I want" ... "so that" pattern
	asAIdx := strings.Index(lower, "as a ")
	iWantIdx := strings.Index(lower, "i want ")
	soThatIdx := strings.Index(lower, "so that ")

	if asAIdx >= 0 && iWantIdx > asAIdx && soThatIdx > iWantIdx {
		// Extract user type
		userEnd := iWantIdx
		userType := strings.TrimSpace(combined[asAIdx+5 : userEnd])
		if len(userType) > 50 {
			userType = userType[:50]
		}
		given = "the user is a " + userType

		// Extract action
		actionEnd := soThatIdx
		if actionEnd < 0 {
			actionEnd = len(combined)
		}
		action := strings.TrimSpace(combined[iWantIdx+7 : actionEnd])
		if len(action) > 100 {
			action = action[:100]
		}
		when = "the user " + action

		// Extract outcome
		if soThatIdx >= 0 {
			outcome := strings.TrimSpace(combined[soThatIdx+8:])
			if len(outcome) > 100 {
				outcome = outcome[:100]
			}
			then = outcome
		} else {
			then = "the system responds appropriately"
		}
	}

	return
}

// extractShouldStatements looks for "should" statements in description
func extractShouldStatements(storyID, description string) []Criterion {
	var criteria []Criterion

	lines := strings.Split(description, "\n")
	counter := 2 // Start at 2 since AC-001 is typically from user story

	for _, line := range lines {
		lower := strings.ToLower(line)
		shouldIdx := strings.Index(lower, " should ")

		if shouldIdx > 0 {
			subject := strings.TrimSpace(line[:shouldIdx])
			predicate := strings.TrimSpace(line[shouldIdx+8:])

			if len(subject) > 0 && len(predicate) > 0 {
				c := Criterion{
					ID:        fmt.Sprintf("%s-AC-%03d", storyID, counter),
					StoryID:   storyID,
					Given:     "the system is in normal state",
					When:      fmt.Sprintf("%s is triggered", subject),
					Then:      fmt.Sprintf("it should %s", predicate),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				criteria = append(criteria, c)
				counter++
			}
		}
	}

	return criteria
}

// extractWhenThenPatterns looks for explicit when/then patterns
func extractWhenThenPatterns(storyID, description string) []Criterion {
	var criteria []Criterion

	lower := strings.ToLower(description)
	counter := 10 // Start higher to avoid conflicts

	// Look for "when X, then Y" or "when X: Y"
	whenIdx := 0
	for {
		idx := strings.Index(lower[whenIdx:], "when ")
		if idx < 0 {
			break
		}
		whenIdx += idx

		// Find the end of the when clause
		thenIdx := strings.Index(lower[whenIdx:], " then ")
		colonIdx := strings.Index(lower[whenIdx:], ":")

		var whenEnd int
		if thenIdx > 0 && (colonIdx < 0 || thenIdx < colonIdx) {
			whenEnd = whenIdx + thenIdx
		} else if colonIdx > 0 {
			whenEnd = whenIdx + colonIdx
		} else {
			whenIdx += 5
			continue
		}

		// Extract when clause
		whenClause := strings.TrimSpace(description[whenIdx+5 : whenEnd])

		// Find then clause
		var thenClause string
		if thenIdx > 0 {
			thenStart := whenIdx + thenIdx + 6
			thenEnd := strings.Index(description[thenStart:], "\n")
			if thenEnd < 0 {
				thenEnd = len(description) - thenStart
			}
			thenClause = strings.TrimSpace(description[thenStart : thenStart+thenEnd])
		}

		if len(whenClause) > 0 && len(thenClause) > 0 {
			c := Criterion{
				ID:        fmt.Sprintf("%s-AC-%03d", storyID, counter),
				StoryID:   storyID,
				Given:     "the system is running",
				When:      whenClause,
				Then:      thenClause,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			criteria = append(criteria, c)
			counter++
		}

		whenIdx += 5
	}

	return criteria
}

// SmithRequest represents a request for the smith subagent
type SmithRequest struct {
	StoryID     string `json:"story_id"`
	StoryTitle  string `json:"story_title"`
	Description string `json:"description"`
	Context     string `json:"context,omitempty"` // Additional context from epic/feature
}

// SmithResponse represents the response from the smith subagent
type SmithResponse struct {
	StoryID  string      `json:"story_id"`
	Criteria []Criterion `json:"criteria"`
	Errors   []string    `json:"errors,omitempty"`
}

// BuildSmithBrief creates a brief for the smith subagent to process
func BuildSmithBrief(req SmithRequest) string {
	var sb strings.Builder

	sb.WriteString("# Smith: Acceptance Criteria Generation\n\n")
	sb.WriteString("## Story\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", req.StoryID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n\n", req.StoryTitle))
	sb.WriteString("**Description:**\n")
	sb.WriteString(req.Description)
	sb.WriteString("\n\n")

	if req.Context != "" {
		sb.WriteString("## Context\n")
		sb.WriteString(req.Context)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Instructions\n")
	sb.WriteString("Generate acceptance criteria in Gherkin format (Given/When/Then).\n")
	sb.WriteString("Include:\n")
	sb.WriteString("- Happy path scenarios\n")
	sb.WriteString("- Error handling scenarios\n")
	sb.WriteString("- Edge cases\n\n")
	sb.WriteString("Return YAML only:\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("criteria:\n")
	sb.WriteString("  - id: STORY-001-AC-001\n")
	sb.WriteString("    given: \"user is logged in\"\n")
	sb.WriteString("    when: \"user clicks submit\"\n")
	sb.WriteString("    then: \"form is submitted\"\n")
	sb.WriteString("    edge_cases:\n")
	sb.WriteString("      - \"network timeout\"\n")
	sb.WriteString("```\n")

	return sb.String()
}
