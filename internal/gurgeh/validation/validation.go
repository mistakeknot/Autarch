// Package validation provides stakeholder perspective validation for specs.
// The "broker" subagent negotiates alignment between Product, Design, and
// Engineering perspectives before specs move to implementation.
package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// Perspective represents a stakeholder perspective
type Perspective string

const (
	PerspectiveProduct     Perspective = "product"
	PerspectiveDesign      Perspective = "design"
	PerspectiveEngineering Perspective = "engineering"
)

// Severity indicates the severity of a validation concern
type Severity string

const (
	SeverityCritical Severity = "critical" // Blocks approval
	SeverityHigh     Severity = "high"     // Strongly recommended
	SeverityMedium   Severity = "medium"   // Should address
	SeverityLow      Severity = "low"      // Nice to have
)

// ValidationConcern represents a specific concern raised during validation
type ValidationConcern struct {
	Perspective Perspective `yaml:"perspective" json:"perspective"`
	Severity    Severity    `yaml:"severity" json:"severity"`
	Category    string      `yaml:"category" json:"category"`     // e.g., "scope", "feasibility", "a11y"
	Title       string      `yaml:"title" json:"title"`           // Short description
	Description string      `yaml:"description" json:"description"`
	Section     string      `yaml:"section,omitempty" json:"section,omitempty"` // Which part of spec
	Suggestion  string      `yaml:"suggestion,omitempty" json:"suggestion,omitempty"`
}

// ValidationResult represents the result of validating a spec from one perspective
type ValidationResult struct {
	Perspective Perspective         `yaml:"perspective" json:"perspective"`
	Approved    bool                `yaml:"approved" json:"approved"`
	Concerns    []ValidationConcern `yaml:"concerns" json:"concerns"`
	Suggestions []string            `yaml:"suggestions,omitempty" json:"suggestions,omitempty"`
}

// AlignmentReport aggregates results from all perspectives
type AlignmentReport struct {
	SpecID          string             `yaml:"spec_id" json:"spec_id"`
	OverallApproved bool               `yaml:"overall_approved" json:"overall_approved"`
	Results         []ValidationResult `yaml:"results" json:"results"`
	Conflicts       []Conflict         `yaml:"conflicts,omitempty" json:"conflicts,omitempty"`
	Summary         string             `yaml:"summary" json:"summary"`
}

// Conflict represents a disagreement between perspectives
type Conflict struct {
	Perspectives []Perspective `yaml:"perspectives" json:"perspectives"`
	Topic        string        `yaml:"topic" json:"topic"`
	Description  string        `yaml:"description" json:"description"`
	Resolution   string        `yaml:"resolution,omitempty" json:"resolution,omitempty"`
}

// Validator interface for perspective validators
type Validator interface {
	Perspective() Perspective
	Validate(spec *specs.Spec) *ValidationResult
}

// Broker orchestrates multi-perspective validation
type Broker struct {
	validators []Validator
}

// NewBroker creates a broker with all three perspective validators
func NewBroker() *Broker {
	return &Broker{
		validators: []Validator{
			&ProductValidator{},
			&DesignValidator{},
			&EngineeringValidator{},
		},
	}
}

// ValidateSpec runs all validators and produces an alignment report
func (b *Broker) ValidateSpec(spec *specs.Spec) *AlignmentReport {
	report := &AlignmentReport{
		SpecID:          spec.ID,
		OverallApproved: true,
	}

	// Run all validators
	for _, v := range b.validators {
		result := v.Validate(spec)
		report.Results = append(report.Results, *result)
		if !result.Approved {
			report.OverallApproved = false
		}
	}

	// Detect conflicts between perspectives
	report.Conflicts = b.detectConflicts(report.Results)

	// Generate summary
	report.Summary = b.generateSummary(report)

	return report
}

// ValidatePRD validates an entire PRD by validating each feature
func (b *Broker) ValidatePRD(prd *specs.PRD) *AlignmentReport {
	// Create a synthetic spec from the PRD
	syntheticSpec := &specs.Spec{
		ID:      prd.ID,
		Title:   prd.Title,
		Summary: prd.Title, // PRD doesn't have Description, use Title
		Status:  string(prd.Status),
	}

	// Aggregate requirements from all features
	for _, feature := range prd.Features {
		syntheticSpec.Requirements = append(syntheticSpec.Requirements, feature.Requirements...)
		syntheticSpec.CriticalUserJourneys = append(syntheticSpec.CriticalUserJourneys, feature.CriticalUserJourneys...)
	}

	return b.ValidateSpec(syntheticSpec)
}

// detectConflicts identifies conflicts between perspective results
func (b *Broker) detectConflicts(results []ValidationResult) []Conflict {
	var conflicts []Conflict

	// Build category -> perspective -> concerns map
	categoryMap := make(map[string]map[Perspective][]ValidationConcern)
	for _, result := range results {
		for _, concern := range result.Concerns {
			if categoryMap[concern.Category] == nil {
				categoryMap[concern.Category] = make(map[Perspective][]ValidationConcern)
			}
			categoryMap[concern.Category][concern.Perspective] = append(
				categoryMap[concern.Category][concern.Perspective],
				concern,
			)
		}
	}

	// Find categories where multiple perspectives disagree
	for category, perspectiveMap := range categoryMap {
		if len(perspectiveMap) > 1 {
			var perspectives []Perspective
			var descriptions []string
			for p, concerns := range perspectiveMap {
				perspectives = append(perspectives, p)
				for _, c := range concerns {
					descriptions = append(descriptions, fmt.Sprintf("[%s] %s", p, c.Title))
				}
			}
			conflicts = append(conflicts, Conflict{
				Perspectives: perspectives,
				Topic:        category,
				Description:  strings.Join(descriptions, "; "),
			})
		}
	}

	return conflicts
}

// generateSummary creates a human-readable summary
func (b *Broker) generateSummary(report *AlignmentReport) string {
	var parts []string

	// Count concerns by severity
	criticalCount := 0
	highCount := 0
	for _, result := range report.Results {
		for _, concern := range result.Concerns {
			switch concern.Severity {
			case SeverityCritical:
				criticalCount++
			case SeverityHigh:
				highCount++
			}
		}
	}

	if report.OverallApproved {
		parts = append(parts, "✓ Spec approved by all perspectives")
	} else {
		parts = append(parts, "✗ Spec requires attention before approval")
	}

	if criticalCount > 0 {
		parts = append(parts, fmt.Sprintf("⚠️ %d critical concerns", criticalCount))
	}
	if highCount > 0 {
		parts = append(parts, fmt.Sprintf("! %d high-priority concerns", highCount))
	}
	if len(report.Conflicts) > 0 {
		parts = append(parts, fmt.Sprintf("⚡ %d cross-perspective conflicts", len(report.Conflicts)))
	}

	return strings.Join(parts, " | ")
}

// --- Product Validator ---

// ProductValidator validates from product perspective
type ProductValidator struct{}

func (v *ProductValidator) Perspective() Perspective {
	return PerspectiveProduct
}

func (v *ProductValidator) Validate(spec *specs.Spec) *ValidationResult {
	result := &ValidationResult{
		Perspective: PerspectiveProduct,
		Approved:    true,
	}

	// Check for value proposition clarity
	if spec.Summary == "" || len(spec.Summary) < 20 {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveProduct,
			Severity:    SeverityCritical,
			Category:    "value_prop",
			Title:       "Missing or unclear value proposition",
			Description: "Spec lacks a clear summary explaining the value to users",
			Section:     "summary",
			Suggestion:  "Add a 2-3 sentence summary explaining what problem this solves and for whom",
		})
		result.Approved = false
	}

	// Check for success metrics
	hasMetrics := false
	for _, req := range spec.Requirements {
		lower := strings.ToLower(req)
		if strings.Contains(lower, "metric") || strings.Contains(lower, "measure") ||
			strings.Contains(lower, "track") || strings.Contains(lower, "%") ||
			strings.Contains(lower, "kpi") {
			hasMetrics = true
			break
		}
	}
	if !hasMetrics {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveProduct,
			Severity:    SeverityHigh,
			Category:    "metrics",
			Title:       "No success metrics defined",
			Description: "Spec doesn't define how success will be measured",
			Section:     "requirements",
			Suggestion:  "Add measurable success criteria (e.g., 'Reduce load time by 50%')",
		})
	}

	// Check MVP scope
	if len(spec.Requirements) > 15 {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveProduct,
			Severity:    SeverityHigh,
			Category:    "scope",
			Title:       "Large scope - consider phasing",
			Description: fmt.Sprintf("Spec has %d requirements, which may indicate scope creep", len(spec.Requirements)),
			Section:     "requirements",
			Suggestion:  "Consider splitting into multiple phases or marking P0/P1/P2 priorities",
		})
	}

	// Check for user persona
	hasPersona := false
	// CUJ doesn't have Persona field in this schema, check requirements/summary
	_ = spec.CriticalUserJourneys // CUJs are checked separately
	lower := strings.ToLower(spec.Summary + " " + strings.Join(spec.Requirements, " "))
	if strings.Contains(lower, "user") || strings.Contains(lower, "customer") ||
		strings.Contains(lower, "admin") {
		hasPersona = true
	}
	if !hasPersona && len(spec.CriticalUserJourneys) > 0 {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveProduct,
			Severity:    SeverityMedium,
			Category:    "persona",
			Title:       "User persona not defined",
			Description: "CUJs exist but target user persona is not specified",
			Section:     "cujs",
			Suggestion:  "Define who the target users are (e.g., 'new user', 'admin', 'power user')",
		})
	}

	return result
}

// --- Design Validator ---

// DesignValidator validates from design perspective
type DesignValidator struct{}

func (v *DesignValidator) Perspective() Perspective {
	return PerspectiveDesign
}

func (v *DesignValidator) Validate(spec *specs.Spec) *ValidationResult {
	result := &ValidationResult{
		Perspective: PerspectiveDesign,
		Approved:    true,
	}

	lower := strings.ToLower(strings.Join(spec.Requirements, " "))

	// Check for accessibility considerations
	a11yKeywords := []string{"accessibility", "a11y", "screen reader", "keyboard", "aria", "wcag"}
	hasA11y := false
	for _, kw := range a11yKeywords {
		if strings.Contains(lower, kw) {
			hasA11y = true
			break
		}
	}
	if !hasA11y && containsUIElements(spec) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveDesign,
			Severity:    SeverityHigh,
			Category:    "a11y",
			Title:       "No accessibility requirements",
			Description: "Spec includes UI elements but doesn't mention accessibility",
			Section:     "requirements",
			Suggestion:  "Add WCAG 2.1 AA compliance requirements for all interactive elements",
		})
	}

	// Check for responsive design considerations
	responsiveKeywords := []string{"responsive", "mobile", "tablet", "breakpoint", "viewport"}
	hasResponsive := false
	for _, kw := range responsiveKeywords {
		if strings.Contains(lower, kw) {
			hasResponsive = true
			break
		}
	}
	if !hasResponsive && containsUIElements(spec) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveDesign,
			Severity:    SeverityMedium,
			Category:    "responsive",
			Title:       "No responsive design requirements",
			Description: "Spec includes UI but doesn't specify responsive behavior",
			Section:     "requirements",
			Suggestion:  "Define behavior for mobile, tablet, and desktop viewports",
		})
	}

	// Check for visual complexity
	visuallyComplexPatterns := regexp.MustCompile(`(?i)(animation|transition|drag|drop|gesture|swipe|pinch|real.?time|live|stream)`)
	if visuallyComplexPatterns.MatchString(lower) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveDesign,
			Severity:    SeverityMedium,
			Category:    "complexity",
			Title:       "Complex interactions detected",
			Description: "Spec includes animations, gestures, or real-time updates that need careful design",
			Section:     "requirements",
			Suggestion:  "Consider design prototypes for complex interactions before implementation",
		})
	}

	// Check for error states
	errorKeywords := []string{"error", "fail", "invalid", "empty state", "loading", "skeleton"}
	hasErrorStates := false
	for _, kw := range errorKeywords {
		if strings.Contains(lower, kw) {
			hasErrorStates = true
			break
		}
	}
	if !hasErrorStates && containsUIElements(spec) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveDesign,
			Severity:    SeverityMedium,
			Category:    "states",
			Title:       "UI states not defined",
			Description: "Spec doesn't define error, loading, or empty states",
			Section:     "requirements",
			Suggestion:  "Define loading, empty, error, and success states for all interactive components",
		})
	}

	return result
}

// --- Engineering Validator ---

// EngineeringValidator validates from engineering perspective
type EngineeringValidator struct{}

func (v *EngineeringValidator) Perspective() Perspective {
	return PerspectiveEngineering
}

func (v *EngineeringValidator) Validate(spec *specs.Spec) *ValidationResult {
	result := &ValidationResult{
		Perspective: PerspectiveEngineering,
		Approved:    true,
	}

	lower := strings.ToLower(strings.Join(spec.Requirements, " "))

	// Check for technical dependencies
	dependencyPatterns := regexp.MustCompile(`(?i)(integrate|connect|sync|import|export|api|webhook|third.?party|external)`)
	if dependencyPatterns.MatchString(lower) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveEngineering,
			Severity:    SeverityHigh,
			Category:    "dependencies",
			Title:       "External dependencies detected",
			Description: "Spec mentions integrations or external services",
			Section:     "requirements",
			Suggestion:  "Document specific APIs, versions, and fallback behavior for external dependencies",
		})
	}

	// Check for data storage implications
	dataPatterns := regexp.MustCompile(`(?i)(store|persist|save|database|cache|remember|history)`)
	if dataPatterns.MatchString(lower) {
		migrationKeywords := []string{"migration", "schema", "model", "table", "column"}
		hasMigration := false
		for _, kw := range migrationKeywords {
			if strings.Contains(lower, kw) {
				hasMigration = true
				break
			}
		}
		if !hasMigration {
			result.Concerns = append(result.Concerns, ValidationConcern{
				Perspective: PerspectiveEngineering,
				Severity:    SeverityMedium,
				Category:    "data",
				Title:       "Data storage without schema definition",
				Description: "Spec mentions storing data but doesn't define data model",
				Section:     "requirements",
				Suggestion:  "Define data schema and migration strategy",
			})
		}
	}

	// Check for performance implications
	perfPatterns := regexp.MustCompile(`(?i)(real.?time|instant|live|stream|large|bulk|batch|million|scale)`)
	if perfPatterns.MatchString(lower) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveEngineering,
			Severity:    SeverityHigh,
			Category:    "performance",
			Title:       "Performance-sensitive requirements detected",
			Description: "Spec includes real-time, large-scale, or streaming requirements",
			Section:     "requirements",
			Suggestion:  "Define performance budgets (latency, throughput) and load testing approach",
		})
	}

	// Check for security implications
	securityPatterns := regexp.MustCompile(`(?i)(auth|login|password|token|permission|role|admin|sensitive|private|encrypt|secret)`)
	if securityPatterns.MatchString(lower) {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveEngineering,
			Severity:    SeverityHigh,
			Category:    "security",
			Title:       "Security-sensitive requirements detected",
			Description: "Spec mentions authentication, authorization, or sensitive data",
			Section:     "requirements",
			Suggestion:  "Conduct threat modeling and define security requirements explicitly",
		})
	}

	// Check for vague technical terms
	vaguePatterns := regexp.MustCompile(`(?i)(fast|quick|instant|simple|easy|seamless|smooth|smart|intelligent|advanced)`)
	matches := vaguePatterns.FindAllString(lower, -1)
	if len(matches) > 3 {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveEngineering,
			Severity:    SeverityMedium,
			Category:    "clarity",
			Title:       "Vague technical language",
			Description: fmt.Sprintf("Spec uses ambiguous terms: %s", strings.Join(matches[:min(3, len(matches))], ", ")),
			Section:     "requirements",
			Suggestion:  "Replace vague terms with measurable criteria (e.g., 'fast' → 'under 200ms')",
		})
	}

	// Check for complexity estimate
	if len(spec.Requirements) > 10 && spec.Complexity == "" {
		result.Concerns = append(result.Concerns, ValidationConcern{
			Perspective: PerspectiveEngineering,
			Severity:    SeverityMedium,
			Category:    "complexity",
			Title:       "No complexity estimate",
			Description: "Large spec without complexity assessment",
			Section:     "metadata",
			Suggestion:  "Add complexity estimate (simple/medium/complex) to set expectations",
		})
	}

	return result
}

// --- Helper functions ---

func containsUIElements(spec *specs.Spec) bool {
	lower := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)
	uiKeywords := []string{"display", "show", "render", "ui", "interface", "screen", "page", "form",
		"button", "input", "click", "tap", "view", "modal", "dialog", "menu", "list", "table"}
	for _, kw := range uiKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FormatAlignmentReport formats the report as markdown
func FormatAlignmentReport(report *AlignmentReport) string {
	var sb strings.Builder

	sb.WriteString("# Stakeholder Alignment Report\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n\n", report.SpecID))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", report.Summary))

	// Results by perspective
	for _, result := range report.Results {
		status := "✓ Approved"
		if !result.Approved {
			status = "✗ Needs Attention"
		}
		sb.WriteString(fmt.Sprintf("## %s Perspective %s\n\n", strings.Title(string(result.Perspective)), status))

		if len(result.Concerns) == 0 {
			sb.WriteString("No concerns raised.\n\n")
		} else {
			for _, concern := range result.Concerns {
				icon := ""
				switch concern.Severity {
				case SeverityCritical:
					icon = "🔴"
				case SeverityHigh:
					icon = "🟠"
				case SeverityMedium:
					icon = "🟡"
				case SeverityLow:
					icon = "🟢"
				}
				sb.WriteString(fmt.Sprintf("### %s %s\n\n", icon, concern.Title))
				sb.WriteString(fmt.Sprintf("**Severity:** %s | **Category:** %s\n\n", concern.Severity, concern.Category))
				sb.WriteString(fmt.Sprintf("%s\n\n", concern.Description))
				if concern.Suggestion != "" {
					sb.WriteString(fmt.Sprintf("**Suggestion:** %s\n\n", concern.Suggestion))
				}
			}
		}
	}

	// Conflicts
	if len(report.Conflicts) > 0 {
		sb.WriteString("## Cross-Perspective Conflicts\n\n")
		for _, conflict := range report.Conflicts {
			perspectives := make([]string, len(conflict.Perspectives))
			for i, p := range conflict.Perspectives {
				perspectives[i] = string(p)
			}
			sb.WriteString(fmt.Sprintf("### %s\n\n", conflict.Topic))
			sb.WriteString(fmt.Sprintf("**Between:** %s\n\n", strings.Join(perspectives, ", ")))
			sb.WriteString(fmt.Sprintf("%s\n\n", conflict.Description))
			if conflict.Resolution != "" {
				sb.WriteString(fmt.Sprintf("**Resolution:** %s\n\n", conflict.Resolution))
			}
		}
	}

	return sb.String()
}

// BuildBrokerBrief creates an agent brief for validation
func BuildBrokerBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Broker Agent Brief: Stakeholder Validation\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Validate the following spec from Product, Design, and Engineering perspectives.\n\n")

	sb.WriteString("## Spec to Validate\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("**Summary:** %s\n\n", spec.Summary))

	if len(spec.Requirements) > 0 {
		sb.WriteString("### Requirements\n")
		for _, req := range spec.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
		sb.WriteString("\n")
	}

	if len(spec.CriticalUserJourneys) > 0 {
		sb.WriteString("### Critical User Journeys\n")
		for _, cuj := range spec.CriticalUserJourneys {
			sb.WriteString(fmt.Sprintf("- **%s:** %s\n", cuj.ID, cuj.Title))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Validation Perspectives\n\n")
	sb.WriteString("### Product\n")
	sb.WriteString("- Is the value proposition clear?\n")
	sb.WriteString("- Are success metrics defined?\n")
	sb.WriteString("- Is the scope appropriate for MVP?\n")
	sb.WriteString("- Are target users identified?\n\n")

	sb.WriteString("### Design\n")
	sb.WriteString("- Are accessibility requirements addressed?\n")
	sb.WriteString("- Is responsive behavior defined?\n")
	sb.WriteString("- Are UI states (loading, error, empty) specified?\n")
	sb.WriteString("- Are complex interactions properly scoped?\n\n")

	sb.WriteString("### Engineering\n")
	sb.WriteString("- Are external dependencies documented?\n")
	sb.WriteString("- Is the data model defined?\n")
	sb.WriteString("- Are performance requirements specified?\n")
	sb.WriteString("- Are security implications addressed?\n")

	return sb.String()
}
