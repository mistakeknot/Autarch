// Package patterns provides the recognizer subagent for detecting patterns and anti-patterns in specs.
package patterns

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// PatternType categorizes detected patterns
type PatternType string

const (
	PatternGood        PatternType = "good"        // Best practice
	PatternAntiPattern PatternType = "anti-pattern" // Should be avoided
	PatternWarning     PatternType = "warning"     // Potential issue
	PatternSuggestion  PatternType = "suggestion"  // Could be improved
)

// Severity indicates how important addressing the pattern is
type Severity string

const (
	SeverityCritical Severity = "critical" // Must fix before implementation
	SeverityHigh     Severity = "high"     // Should fix before implementation
	SeverityMedium   Severity = "medium"   // Consider fixing
	SeverityLow      Severity = "low"      // Nice to fix
	SeverityInfo     Severity = "info"     // Informational
)

// DetectedPattern represents a pattern found in a spec
type DetectedPattern struct {
	Name        string      `yaml:"name" json:"name"`
	Type        PatternType `yaml:"type" json:"type"`
	Severity    Severity    `yaml:"severity" json:"severity"`
	Description string      `yaml:"description" json:"description"`
	Location    string      `yaml:"location" json:"location"`   // Where in spec it was detected
	Suggestion  string      `yaml:"suggestion" json:"suggestion"` // How to address it
	Examples    []string    `yaml:"examples,omitempty" json:"examples,omitempty"`
}

// SpecQuality represents overall spec quality assessment
type SpecQuality string

const (
	QualityExcellent SpecQuality = "excellent"
	QualityGood      SpecQuality = "good"
	QualityFair      SpecQuality = "fair"
	QualityPoor      SpecQuality = "poor"
)

// SimilarFeature represents a potentially similar existing feature
type SimilarFeature struct {
	Name        string   `yaml:"name" json:"name"`
	Similarity  string   `yaml:"similarity" json:"similarity"` // high, medium, low
	Overlaps    []string `yaml:"overlaps" json:"overlaps"`     // What overlaps
	Suggestion  string   `yaml:"suggestion" json:"suggestion"` // Reuse recommendation
}

// ReusableComponent represents a component that could be reused
type ReusableComponent struct {
	Name        string `yaml:"name" json:"name"`
	Type        string `yaml:"type" json:"type"` // UI, API, service, library
	Description string `yaml:"description" json:"description"`
	Confidence  string `yaml:"confidence" json:"confidence"` // high, medium, low
}

// QualityMetric represents a specific quality measurement
type QualityMetric struct {
	Metric string `yaml:"metric" json:"metric"`
	Score  int    `yaml:"score" json:"score"` // 0-100
	Notes  string `yaml:"notes" json:"notes"`
}

// PatternReport represents the full pattern analysis
type PatternReport struct {
	SpecID           string              `yaml:"spec_id" json:"spec_id"`
	Quality          SpecQuality         `yaml:"quality" json:"quality"`
	QualityScore     int                 `yaml:"quality_score" json:"quality_score"` // 0-100
	Patterns         []DetectedPattern   `yaml:"patterns" json:"patterns"`
	SimilarFeatures  []SimilarFeature    `yaml:"similar_features" json:"similar_features"`
	ReusableComponents []ReusableComponent `yaml:"reusable_components" json:"reusable_components"`
	Metrics          []QualityMetric     `yaml:"metrics" json:"metrics"`
	Recommendations  []string            `yaml:"recommendations" json:"recommendations"`
}

// Recognizer analyzes specs for patterns and anti-patterns
type Recognizer struct{}

// NewRecognizer creates a new Recognizer instance
func NewRecognizer() *Recognizer {
	return &Recognizer{}
}

// Recognize analyzes a spec and returns its pattern report
func (r *Recognizer) Recognize(spec *specs.Spec) *PatternReport {
	report := &PatternReport{
		SpecID: spec.ID,
	}

	report.Patterns = r.detectPatterns(spec)
	report.SimilarFeatures = r.findSimilarFeatures(spec)
	report.ReusableComponents = r.identifyReusableComponents(spec)
	report.Metrics = r.calculateQualityMetrics(spec)
	report.QualityScore = r.calculateOverallScore(report.Metrics, report.Patterns)
	report.Quality = r.qualityFromScore(report.QualityScore)
	report.Recommendations = r.generateRecommendations(report)

	return report
}

// RecognizePRD analyzes a full PRD for patterns
func (r *Recognizer) RecognizePRD(prd *specs.PRD) *PatternReport {
	report := &PatternReport{
		SpecID: prd.ID,
	}

	// Analyze each feature and aggregate
	for _, feature := range prd.Features {
		spec := &specs.Spec{
			ID:           feature.ID,
			Title:        feature.Title,
			Summary:      feature.Summary,
			Requirements: feature.Requirements,
		}
		featurePatterns := r.detectPatterns(spec)
		report.Patterns = append(report.Patterns, featurePatterns...)
	}

	// PRD-level analysis
	report.Patterns = append(report.Patterns, r.detectPRDPatterns(prd)...)
	report.Metrics = r.calculatePRDMetrics(prd)
	report.QualityScore = r.calculateOverallScore(report.Metrics, report.Patterns)
	report.Quality = r.qualityFromScore(report.QualityScore)
	report.Recommendations = r.generateRecommendations(report)

	return report
}

func (r *Recognizer) detectPatterns(spec *specs.Spec) []DetectedPattern {
	var patterns []DetectedPattern

	text := spec.Title + " " + spec.Summary + " " + strings.Join(spec.Requirements, " ")
	textLower := strings.ToLower(text)

	// === Anti-patterns ===

	// Vague requirements
	vagueWords := []string{"somehow", "maybe", "possibly", "etc", "and so on", "things like", "stuff"}
	for _, word := range vagueWords {
		if strings.Contains(textLower, word) {
			patterns = append(patterns, DetectedPattern{
				Name:        "Vague Language",
				Type:        PatternAntiPattern,
				Severity:    SeverityMedium,
				Description: fmt.Sprintf("Found vague term '%s' which reduces spec clarity", word),
				Location:    "Requirements",
				Suggestion:  "Replace vague terms with specific, measurable requirements",
			})
			break
		}
	}

	// Scope creep indicators
	scopeCreepPatterns := []string{"also", "additionally", "as well as", "plus", "furthermore", "moreover"}
	scopeCreepCount := 0
	for _, pattern := range scopeCreepPatterns {
		scopeCreepCount += strings.Count(textLower, pattern)
	}
	if scopeCreepCount > 3 {
		patterns = append(patterns, DetectedPattern{
			Name:        "Potential Scope Creep",
			Type:        PatternWarning,
			Severity:    SeverityMedium,
			Description: fmt.Sprintf("Found %d additive phrases suggesting expanding scope", scopeCreepCount),
			Location:    "Requirements",
			Suggestion:  "Consider splitting into multiple specs or prioritizing requirements",
		})
	}

	// Gold plating (over-engineering)
	goldPlatingPatterns := []string{"all possible", "every possible", "complete", "comprehensive", "fully featured"}
	for _, pattern := range goldPlatingPatterns {
		if strings.Contains(textLower, pattern) {
			patterns = append(patterns, DetectedPattern{
				Name:        "Gold Plating Risk",
				Type:        PatternWarning,
				Severity:    SeverityMedium,
				Description: "Spec may be over-scoped with completeness language",
				Location:    "Requirements",
				Suggestion:  "Define MVP scope first, defer nice-to-haves",
			})
			break
		}
	}

	// Missing acceptance criteria indicators
	if !strings.Contains(textLower, "when") && !strings.Contains(textLower, "should") &&
		!strings.Contains(textLower, "must") && !strings.Contains(textLower, "will") {
		patterns = append(patterns, DetectedPattern{
			Name:        "Missing Acceptance Language",
			Type:        PatternWarning,
			Severity:    SeverityLow,
			Description: "No clear acceptance criteria language found",
			Location:    "Requirements",
			Suggestion:  "Add testable acceptance criteria using 'should', 'must', or 'when' statements",
		})
	}

	// Conflicting requirements
	conflicts := r.detectConflicts(spec.Requirements)
	for _, conflict := range conflicts {
		patterns = append(patterns, DetectedPattern{
			Name:        "Conflicting Requirements",
			Type:        PatternAntiPattern,
			Severity:    SeverityHigh,
			Description: conflict,
			Location:    "Requirements",
			Suggestion:  "Clarify which requirement takes precedence or resolve the conflict",
		})
	}

	// === Good patterns ===

	// Clear user stories
	if strings.Contains(textLower, "as a") && strings.Contains(textLower, "i want") {
		patterns = append(patterns, DetectedPattern{
			Name:        "User Story Format",
			Type:        PatternGood,
			Severity:    SeverityInfo,
			Description: "Spec uses user story format for requirements",
			Location:    "Requirements",
			Suggestion:  "Continue using this format for consistency",
		})
	}

	// Measurable outcomes
	measurePatterns := []string{"measure", "metric", "kpi", "percentage", "rate", "count"}
	for _, pattern := range measurePatterns {
		if strings.Contains(textLower, pattern) {
			patterns = append(patterns, DetectedPattern{
				Name:        "Measurable Outcomes",
				Type:        PatternGood,
				Severity:    SeverityInfo,
				Description: "Spec includes measurable success criteria",
				Location:    "Requirements",
				Suggestion:  "Ensure all key features have measurable outcomes",
			})
			break
		}
	}

	// Edge cases mentioned
	edgeCasePatterns := []string{"edge case", "error", "fail", "invalid", "empty", "null", "timeout"}
	edgeCaseCount := 0
	for _, pattern := range edgeCasePatterns {
		if strings.Contains(textLower, pattern) {
			edgeCaseCount++
		}
	}
	if edgeCaseCount >= 2 {
		patterns = append(patterns, DetectedPattern{
			Name:        "Edge Cases Considered",
			Type:        PatternGood,
			Severity:    SeverityInfo,
			Description: "Spec addresses error conditions and edge cases",
			Location:    "Requirements",
			Suggestion:  "Continue documenting edge cases for completeness",
		})
	}

	// === Suggestions ===

	// Missing security consideration
	if (strings.Contains(textLower, "user") || strings.Contains(textLower, "login") ||
		strings.Contains(textLower, "data")) && !strings.Contains(textLower, "security") &&
		!strings.Contains(textLower, "auth") {
		patterns = append(patterns, DetectedPattern{
			Name:        "Security Not Addressed",
			Type:        PatternSuggestion,
			Severity:    SeverityMedium,
			Description: "Spec involves user data but doesn't mention security",
			Location:    "Requirements",
			Suggestion:  "Add security requirements for authentication and data protection",
		})
	}

	// Missing performance consideration
	if (strings.Contains(textLower, "list") || strings.Contains(textLower, "search") ||
		strings.Contains(textLower, "load")) && !strings.Contains(textLower, "performance") &&
		!strings.Contains(textLower, "fast") && !strings.Contains(textLower, "quick") {
		patterns = append(patterns, DetectedPattern{
			Name:        "Performance Not Addressed",
			Type:        PatternSuggestion,
			Severity:    SeverityLow,
			Description: "Spec involves data operations but doesn't mention performance",
			Location:    "Requirements",
			Suggestion:  "Consider adding performance requirements for data-heavy operations",
		})
	}

	return patterns
}

func (r *Recognizer) detectConflicts(requirements []string) []string {
	var conflicts []string

	// Simple conflict detection based on opposing terms
	opposites := map[string]string{
		"simple":     "complex",
		"fast":       "comprehensive",
		"minimal":    "complete",
		"real-time":  "batch",
		"synchronous": "asynchronous",
	}

	reqText := strings.ToLower(strings.Join(requirements, " "))
	for term1, term2 := range opposites {
		if strings.Contains(reqText, term1) && strings.Contains(reqText, term2) {
			conflicts = append(conflicts, fmt.Sprintf("Potential conflict: '%s' vs '%s' - clarify priority", term1, term2))
		}
	}

	return conflicts
}

func (r *Recognizer) detectPRDPatterns(prd *specs.PRD) []DetectedPattern {
	var patterns []DetectedPattern

	// Too many features
	if len(prd.Features) > 10 {
		patterns = append(patterns, DetectedPattern{
			Name:        "Feature Overload",
			Type:        PatternWarning,
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("PRD has %d features - may be too ambitious for single release", len(prd.Features)),
			Location:    "PRD Structure",
			Suggestion:  "Consider splitting into multiple releases or phases",
		})
	}

	// No features
	if len(prd.Features) == 0 {
		patterns = append(patterns, DetectedPattern{
			Name:        "Empty PRD",
			Type:        PatternAntiPattern,
			Severity:    SeverityCritical,
			Description: "PRD has no features defined",
			Location:    "PRD Structure",
			Suggestion:  "Add at least one feature with clear requirements",
		})
	}

	// Check for dependencies between features
	// (Simplified - in production would analyze actual dependencies)
	featureTitles := make([]string, len(prd.Features))
	for i, f := range prd.Features {
		featureTitles[i] = strings.ToLower(f.Title)
	}

	return patterns
}

func (r *Recognizer) findSimilarFeatures(spec *specs.Spec) []SimilarFeature {
	var similar []SimilarFeature
	textLower := strings.ToLower(spec.Title + " " + spec.Summary)

	// Common feature patterns that might already exist
	commonPatterns := map[string]string{
		"authentication": "User authentication is a common feature - check for existing auth modules",
		"login":          "Login functionality - check for existing auth systems",
		"dashboard":      "Dashboards are common - check for existing dashboard components",
		"settings":       "Settings pages - check for existing settings infrastructure",
		"notifications":  "Notification systems - check for existing notification services",
		"search":         "Search functionality - check for existing search implementations",
		"file upload":    "File uploads - check for existing upload handling",
		"email":          "Email functionality - check for existing email services",
		"payment":        "Payment processing - check for existing payment integrations",
	}

	for pattern, suggestion := range commonPatterns {
		if strings.Contains(textLower, pattern) {
			similar = append(similar, SimilarFeature{
				Name:       pattern,
				Similarity: "medium",
				Overlaps:   []string{pattern},
				Suggestion: suggestion,
			})
		}
	}

	return similar
}

func (r *Recognizer) identifyReusableComponents(spec *specs.Spec) []ReusableComponent {
	var components []ReusableComponent
	textLower := strings.ToLower(strings.Join(spec.Requirements, " ") + " " + spec.Summary)

	// UI components
	uiPatterns := map[string]string{
		"form":    "Form component for data entry",
		"table":   "Data table component",
		"list":    "List view component",
		"modal":   "Modal/dialog component",
		"button":  "Button components",
		"card":    "Card layout component",
	}
	for pattern, desc := range uiPatterns {
		if strings.Contains(textLower, pattern) {
			components = append(components, ReusableComponent{
				Name:        pattern + " component",
				Type:        "UI",
				Description: desc,
				Confidence:  "medium",
			})
		}
	}

	// API/Service patterns
	servicePatterns := map[string]string{
		"crud":     "CRUD API endpoints",
		"api":      "REST API service",
		"webhook":  "Webhook handler service",
		"queue":    "Message queue handler",
	}
	for pattern, desc := range servicePatterns {
		if strings.Contains(textLower, pattern) {
			components = append(components, ReusableComponent{
				Name:        pattern + " service",
				Type:        "API",
				Description: desc,
				Confidence:  "medium",
			})
		}
	}

	return components
}

func (r *Recognizer) calculateQualityMetrics(spec *specs.Spec) []QualityMetric {
	var metrics []QualityMetric

	// Clarity score - based on specificity
	clarityScore := 70 // Base score
	text := spec.Summary + " " + strings.Join(spec.Requirements, " ")

	// Deduct for vague words
	vagueWords := regexp.MustCompile(`(?i)\b(somehow|maybe|possibly|etc|stuff|things)\b`)
	vagueCount := len(vagueWords.FindAllString(text, -1))
	clarityScore -= vagueCount * 10

	// Add for specific quantities
	quantities := regexp.MustCompile(`\d+`)
	quantityCount := len(quantities.FindAllString(text, -1))
	clarityScore += min(quantityCount*5, 20)

	metrics = append(metrics, QualityMetric{
		Metric: "Clarity",
		Score:  max(0, min(100, clarityScore)),
		Notes:  fmt.Sprintf("Based on language specificity (%d vague terms, %d quantities)", vagueCount, quantityCount),
	})

	// Completeness score - based on required elements
	completenessScore := 0
	if spec.Title != "" {
		completenessScore += 20
	}
	if spec.Summary != "" {
		completenessScore += 20
	}
	if len(spec.Requirements) > 0 {
		completenessScore += 20
	}
	if len(spec.Requirements) >= 3 {
		completenessScore += 20
	}
	if strings.Contains(strings.ToLower(text), "user") {
		completenessScore += 10
	}
	if strings.Contains(strings.ToLower(text), "should") || strings.Contains(strings.ToLower(text), "must") {
		completenessScore += 10
	}

	metrics = append(metrics, QualityMetric{
		Metric: "Completeness",
		Score:  min(100, completenessScore),
		Notes:  fmt.Sprintf("Based on presence of required elements (%d requirements)", len(spec.Requirements)),
	})

	// Testability score
	testabilityScore := 50 // Base score
	testablePatterns := []string{"should", "must", "when", "given", "then", "expect"}
	for _, pattern := range testablePatterns {
		if strings.Contains(strings.ToLower(text), pattern) {
			testabilityScore += 10
		}
	}
	metrics = append(metrics, QualityMetric{
		Metric: "Testability",
		Score:  min(100, testabilityScore),
		Notes:  "Based on presence of testable language",
	})

	return metrics
}

func (r *Recognizer) calculatePRDMetrics(prd *specs.PRD) []QualityMetric {
	var metrics []QualityMetric

	// Feature count metric
	featureScore := 100
	if len(prd.Features) == 0 {
		featureScore = 0
	} else if len(prd.Features) > 10 {
		featureScore = 60 // Too many
	} else if len(prd.Features) < 2 {
		featureScore = 70 // Too few
	}
	metrics = append(metrics, QualityMetric{
		Metric: "Scope Balance",
		Score:  featureScore,
		Notes:  fmt.Sprintf("Based on feature count (%d features)", len(prd.Features)),
	})

	// Calculate average requirement quality
	totalReqs := 0
	for _, f := range prd.Features {
		totalReqs += len(f.Requirements)
	}
	avgReqs := 0
	if len(prd.Features) > 0 {
		avgReqs = totalReqs / len(prd.Features)
	}

	reqScore := 100
	if avgReqs < 2 {
		reqScore = 50 // Too few requirements per feature
	} else if avgReqs > 10 {
		reqScore = 70 // Too many per feature
	}
	featureCount := len(prd.Features)
	if featureCount == 0 {
		featureCount = 1
	}
	metrics = append(metrics, QualityMetric{
		Metric: "Requirement Depth",
		Score:  reqScore,
		Notes:  fmt.Sprintf("Average %.1f requirements per feature", float64(totalReqs)/float64(featureCount)),
	})

	return metrics
}

func (r *Recognizer) calculateOverallScore(metrics []QualityMetric, patterns []DetectedPattern) int {
	if len(metrics) == 0 {
		return 50
	}

	// Average of metrics
	total := 0
	for _, m := range metrics {
		total += m.Score
	}
	score := total / len(metrics)

	// Adjust based on patterns
	for _, p := range patterns {
		switch p.Severity {
		case SeverityCritical:
			score -= 20
		case SeverityHigh:
			score -= 10
		case SeverityMedium:
			if p.Type == PatternAntiPattern {
				score -= 5
			}
		}
		if p.Type == PatternGood {
			score += 3
		}
	}

	return max(0, min(100, score))
}

func (r *Recognizer) qualityFromScore(score int) SpecQuality {
	switch {
	case score >= 80:
		return QualityExcellent
	case score >= 60:
		return QualityGood
	case score >= 40:
		return QualityFair
	default:
		return QualityPoor
	}
}

func (r *Recognizer) generateRecommendations(report *PatternReport) []string {
	var recs []string

	// Based on quality
	if report.Quality == QualityPoor {
		recs = append(recs, "Significant spec improvements needed before implementation")
	}

	// Based on patterns
	criticalCount := 0
	highCount := 0
	for _, p := range report.Patterns {
		if p.Severity == SeverityCritical {
			criticalCount++
		}
		if p.Severity == SeverityHigh {
			highCount++
		}
	}

	if criticalCount > 0 {
		recs = append(recs, fmt.Sprintf("Address %d critical issues before proceeding", criticalCount))
	}
	if highCount > 0 {
		recs = append(recs, fmt.Sprintf("Review and fix %d high-severity issues", highCount))
	}

	// Based on similar features
	if len(report.SimilarFeatures) > 0 {
		recs = append(recs, "Check codebase for existing implementations of similar features")
	}

	// Based on reusable components
	if len(report.ReusableComponents) > 0 {
		recs = append(recs, fmt.Sprintf("Consider reusing %d existing components", len(report.ReusableComponents)))
	}

	return recs
}

// FormatPatternReport formats a pattern report as readable text
func FormatPatternReport(report *PatternReport) string {
	var sb strings.Builder

	sb.WriteString("# Pattern Analysis Report\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n", report.SpecID))
	sb.WriteString(fmt.Sprintf("**Quality:** %s (Score: %d/100)\n\n", qualityIcon(report.Quality), report.QualityScore))

	if len(report.Metrics) > 0 {
		sb.WriteString("## Quality Metrics\n\n")
		sb.WriteString("| Metric | Score | Notes |\n")
		sb.WriteString("|--------|-------|-------|\n")
		for _, m := range report.Metrics {
			sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", m.Metric, m.Score, m.Notes))
		}
		sb.WriteString("\n")
	}

	if len(report.Patterns) > 0 {
		// Group by type
		antiPatterns := filterByType(report.Patterns, PatternAntiPattern)
		warnings := filterByType(report.Patterns, PatternWarning)
		suggestions := filterByType(report.Patterns, PatternSuggestion)
		goodPatterns := filterByType(report.Patterns, PatternGood)

		if len(antiPatterns) > 0 {
			sb.WriteString("## Anti-Patterns Detected\n\n")
			for _, p := range antiPatterns {
				sb.WriteString(fmt.Sprintf("### %s %s\n", severityIcon(p.Severity), p.Name))
				sb.WriteString(fmt.Sprintf("%s\n", p.Description))
				sb.WriteString(fmt.Sprintf("*Suggestion:* %s\n\n", p.Suggestion))
			}
		}

		if len(warnings) > 0 {
			sb.WriteString("## Warnings\n\n")
			for _, p := range warnings {
				sb.WriteString(fmt.Sprintf("- **%s:** %s\n", p.Name, p.Description))
			}
			sb.WriteString("\n")
		}

		if len(suggestions) > 0 {
			sb.WriteString("## Suggestions\n\n")
			for _, p := range suggestions {
				sb.WriteString(fmt.Sprintf("- **%s:** %s\n", p.Name, p.Suggestion))
			}
			sb.WriteString("\n")
		}

		if len(goodPatterns) > 0 {
			sb.WriteString("## Good Patterns Found\n\n")
			for _, p := range goodPatterns {
				sb.WriteString(fmt.Sprintf("- **%s:** %s\n", p.Name, p.Description))
			}
			sb.WriteString("\n")
		}
	}

	if len(report.SimilarFeatures) > 0 {
		sb.WriteString("## Similar Features\n\n")
		for _, sf := range report.SimilarFeatures {
			sb.WriteString(fmt.Sprintf("- **%s** (%s similarity): %s\n", sf.Name, sf.Similarity, sf.Suggestion))
		}
		sb.WriteString("\n")
	}

	if len(report.ReusableComponents) > 0 {
		sb.WriteString("## Potentially Reusable Components\n\n")
		for _, c := range report.ReusableComponents {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", c.Name, c.Type, c.Description))
		}
		sb.WriteString("\n")
	}

	if len(report.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, r := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", r))
		}
	}

	return sb.String()
}

// BuildRecognizerBrief generates an agent brief for pattern recognition
func BuildRecognizerBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Recognizer Agent Brief\n\n")
	sb.WriteString("## Mission\n")
	sb.WriteString("Detect patterns and anti-patterns in the specification to improve quality.\n\n")

	sb.WriteString("## Spec Context\n")
	sb.WriteString(fmt.Sprintf("- **ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("- **Summary:** %s\n\n", spec.Summary))

	sb.WriteString("## Analysis Scope\n")
	sb.WriteString("1. **Anti-Patterns** - Vague requirements, scope creep, conflicting constraints\n")
	sb.WriteString("2. **Good Patterns** - User stories, measurable outcomes, edge case coverage\n")
	sb.WriteString("3. **Similar Features** - Existing functionality that overlaps\n")
	sb.WriteString("4. **Reusable Components** - UI, API, service components to reuse\n")
	sb.WriteString("5. **Quality Metrics** - Clarity, completeness, testability\n")

	return sb.String()
}

func filterByType(patterns []DetectedPattern, pType PatternType) []DetectedPattern {
	var result []DetectedPattern
	for _, p := range patterns {
		if p.Type == pType {
			result = append(result, p)
		}
	}
	return result
}

func qualityIcon(quality SpecQuality) string {
	switch quality {
	case QualityExcellent:
		return "🟢 Excellent"
	case QualityGood:
		return "🟡 Good"
	case QualityFair:
		return "🟠 Fair"
	case QualityPoor:
		return "🔴 Poor"
	default:
		return "⚪ Unknown"
	}
}

func severityIcon(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "🔴"
	case SeverityHigh:
		return "🟠"
	case SeverityMedium:
		return "🟡"
	case SeverityLow:
		return "🟢"
	case SeverityInfo:
		return "ℹ️"
	default:
		return "⚪"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
