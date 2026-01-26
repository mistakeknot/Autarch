package patterns

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestRecognizer_Recognize_DetectsVagueLanguage(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Feature",
		Summary:      "Do some stuff",
		Requirements: []string{"Somehow handle user input", "Show things etc"},
	}

	report := recognizer.Recognize(spec)

	foundVague := false
	for _, p := range report.Patterns {
		if p.Name == "Vague Language" && p.Type == PatternAntiPattern {
			foundVague = true
			break
		}
	}
	if !foundVague {
		t.Error("expected vague language anti-pattern detection")
	}
}

func TestRecognizer_Recognize_DetectsScopeCreep(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Feature",
		Summary:      "Main feature plus extras",
		Requirements: []string{
			"Do A, also do B",
			"Additionally handle C",
			"Furthermore support D",
			"Moreover include E",
		},
	}

	report := recognizer.Recognize(spec)

	foundCreep := false
	for _, p := range report.Patterns {
		if p.Name == "Potential Scope Creep" {
			foundCreep = true
			break
		}
	}
	if !foundCreep {
		t.Error("expected scope creep warning")
	}
}

func TestRecognizer_Recognize_DetectsGoldPlating(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Complete System",
		Summary:      "Comprehensive solution with all possible features",
		Requirements: []string{"Support every possible use case"},
	}

	report := recognizer.Recognize(spec)

	foundGold := false
	for _, p := range report.Patterns {
		if p.Name == "Gold Plating Risk" {
			foundGold = true
			break
		}
	}
	if !foundGold {
		t.Error("expected gold plating warning")
	}
}

func TestRecognizer_Recognize_DetectsUserStoryFormat(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Profile",
		Summary:      "As a user I want to view my profile",
		Requirements: []string{"Display user information"},
	}

	report := recognizer.Recognize(spec)

	foundStory := false
	for _, p := range report.Patterns {
		if p.Name == "User Story Format" && p.Type == PatternGood {
			foundStory = true
			break
		}
	}
	if !foundStory {
		t.Error("expected user story format good pattern")
	}
}

func TestRecognizer_Recognize_DetectsMeasurableOutcomes(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Performance Feature",
		Summary:      "Improve page load with measurable metrics",
		Requirements: []string{"Reduce load time by 50%", "Track success rate"},
	}

	report := recognizer.Recognize(spec)

	foundMeasurable := false
	for _, p := range report.Patterns {
		if p.Name == "Measurable Outcomes" && p.Type == PatternGood {
			foundMeasurable = true
			break
		}
	}
	if !foundMeasurable {
		t.Error("expected measurable outcomes good pattern")
	}
}

func TestRecognizer_Recognize_DetectsEdgeCases(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Form Handler",
		Summary:      "Handle form submission with error cases",
		Requirements: []string{
			"Validate input fields",
			"Handle empty form submission",
			"Show error for invalid data",
		},
	}

	report := recognizer.Recognize(spec)

	foundEdge := false
	for _, p := range report.Patterns {
		if p.Name == "Edge Cases Considered" && p.Type == PatternGood {
			foundEdge = true
			break
		}
	}
	if !foundEdge {
		t.Error("expected edge cases good pattern")
	}
}

func TestRecognizer_Recognize_SuggestsSecurityForUserData(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Data Handler",
		Summary:      "Store and manage user data",
		Requirements: []string{"Save user profiles", "Load user preferences"},
	}

	report := recognizer.Recognize(spec)

	foundSecurity := false
	for _, p := range report.Patterns {
		if p.Name == "Security Not Addressed" && p.Type == PatternSuggestion {
			foundSecurity = true
			break
		}
	}
	if !foundSecurity {
		t.Error("expected security suggestion for user data")
	}
}

func TestRecognizer_Recognize_FindsSimilarFeatures(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Authentication",
		Summary:      "Handle user login and authentication",
		Requirements: []string{"Login with email", "Password reset"},
	}

	report := recognizer.Recognize(spec)

	if len(report.SimilarFeatures) == 0 {
		t.Error("expected similar features for common auth functionality")
	}

	foundAuth := false
	for _, sf := range report.SimilarFeatures {
		if sf.Name == "authentication" || sf.Name == "login" {
			foundAuth = true
			break
		}
	}
	if !foundAuth {
		t.Error("expected authentication similar feature")
	}
}

func TestRecognizer_Recognize_IdentifiesReusableComponents(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Data Management",
		Summary:      "Manage data with forms and tables",
		Requirements: []string{"Display data in table", "Edit with form modal"},
	}

	report := recognizer.Recognize(spec)

	if len(report.ReusableComponents) == 0 {
		t.Error("expected reusable UI components")
	}

	foundTable := false
	foundForm := false
	foundModal := false
	for _, c := range report.ReusableComponents {
		if strings.Contains(c.Name, "table") {
			foundTable = true
		}
		if strings.Contains(c.Name, "form") {
			foundForm = true
		}
		if strings.Contains(c.Name, "modal") {
			foundModal = true
		}
	}
	if !foundTable {
		t.Error("expected table component")
	}
	if !foundForm {
		t.Error("expected form component")
	}
	if !foundModal {
		t.Error("expected modal component")
	}
}

func TestRecognizer_Recognize_CalculatesQualityMetrics(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Well-Specified Feature",
		Summary:      "A user should be able to view their dashboard",
		Requirements: []string{
			"User must see recent activity",
			"Dashboard should load within 2 seconds",
			"Display 10 most recent items",
		},
	}

	report := recognizer.Recognize(spec)

	if len(report.Metrics) == 0 {
		t.Error("expected quality metrics")
	}

	foundClarity := false
	foundCompleteness := false
	for _, m := range report.Metrics {
		if m.Metric == "Clarity" {
			foundClarity = true
			if m.Score <= 0 || m.Score > 100 {
				t.Errorf("clarity score %d out of range", m.Score)
			}
		}
		if m.Metric == "Completeness" {
			foundCompleteness = true
		}
	}
	if !foundClarity {
		t.Error("expected clarity metric")
	}
	if !foundCompleteness {
		t.Error("expected completeness metric")
	}
}

func TestRecognizer_Recognize_CalculatesOverallQuality(t *testing.T) {
	recognizer := NewRecognizer()

	// Good spec
	goodSpec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Clear Feature",
		Summary:      "User should be able to update their profile",
		Requirements: []string{
			"Must save profile changes",
			"Should validate email format",
			"Must handle empty name error",
		},
	}
	goodReport := recognizer.Recognize(goodSpec)
	if goodReport.Quality == QualityPoor {
		t.Errorf("well-specified spec quality = %v, expected better", goodReport.Quality)
	}

	// Poor spec
	poorSpec := &specs.Spec{
		ID:      "SPEC-002",
		Title:   "Stuff",
		Summary: "Do things somehow",
	}
	poorReport := recognizer.Recognize(poorSpec)
	if poorReport.Quality == QualityExcellent {
		t.Errorf("vague spec quality = %v, expected worse", poorReport.Quality)
	}
}

func TestRecognizer_RecognizePRD_DetectsFeatureOverload(t *testing.T) {
	recognizer := NewRecognizer()

	// Create PRD with too many features
	features := make([]specs.Feature, 15)
	for i := 0; i < 15; i++ {
		features[i] = specs.Feature{
			ID:           fmt.Sprintf("FEAT-%03d", i),
			Title:        fmt.Sprintf("Feature %d", i),
			Requirements: []string{"Some requirement"},
		}
	}

	prd := &specs.PRD{
		ID:       "MVP",
		Title:    "MVP",
		Features: features,
	}

	report := recognizer.RecognizePRD(prd)

	foundOverload := false
	for _, p := range report.Patterns {
		if p.Name == "Feature Overload" {
			foundOverload = true
			break
		}
	}
	if !foundOverload {
		t.Error("expected feature overload warning for 15 features")
	}
}

func TestRecognizer_RecognizePRD_DetectsEmptyPRD(t *testing.T) {
	recognizer := NewRecognizer()

	prd := &specs.PRD{
		ID:       "Empty",
		Title:    "Empty PRD",
		Features: []specs.Feature{},
	}

	report := recognizer.RecognizePRD(prd)

	foundEmpty := false
	for _, p := range report.Patterns {
		if p.Name == "Empty PRD" && p.Severity == SeverityCritical {
			foundEmpty = true
			break
		}
	}
	if !foundEmpty {
		t.Error("expected empty PRD anti-pattern")
	}
}

func TestRecognizer_Recognize_GeneratesRecommendations(t *testing.T) {
	recognizer := NewRecognizer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth Feature",
		Summary:      "Authentication somehow",
		Requirements: []string{"Handle login etc"},
	}

	report := recognizer.Recognize(spec)

	// Should have recommendations given the poor quality
	if len(report.Recommendations) == 0 {
		t.Error("expected recommendations for spec with issues")
	}
}

func TestFormatPatternReport(t *testing.T) {
	report := &PatternReport{
		SpecID:       "SPEC-001",
		Quality:      QualityGood,
		QualityScore: 75,
		Patterns: []DetectedPattern{
			{Name: "Vague Language", Type: PatternAntiPattern, Severity: SeverityMedium, Description: "Found vague terms"},
			{Name: "User Story Format", Type: PatternGood, Severity: SeverityInfo, Description: "Uses user story format"},
		},
		Metrics: []QualityMetric{
			{Metric: "Clarity", Score: 70, Notes: "Based on language"},
		},
		SimilarFeatures: []SimilarFeature{
			{Name: "auth", Similarity: "medium", Suggestion: "Check existing auth"},
		},
		ReusableComponents: []ReusableComponent{
			{Name: "form", Type: "UI", Description: "Form component"},
		},
		Recommendations: []string{"Address vague language"},
	}

	output := FormatPatternReport(report)

	if !strings.Contains(output, "Pattern Analysis Report") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "Quality Metrics") {
		t.Error("should contain quality metrics section")
	}
	if !strings.Contains(output, "Anti-Patterns") {
		t.Error("should contain anti-patterns section")
	}
	if !strings.Contains(output, "Good Patterns") {
		t.Error("should contain good patterns section")
	}
	if !strings.Contains(output, "Similar Features") {
		t.Error("should contain similar features section")
	}
}

func TestBuildRecognizerBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Feature",
		Summary: "A test feature",
	}

	brief := BuildRecognizerBrief(spec)

	if !strings.Contains(brief, "Recognizer Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "Anti-Patterns") {
		t.Error("should reference anti-patterns")
	}
	if !strings.Contains(brief, "Quality Metrics") {
		t.Error("should reference quality metrics")
	}
}

func TestQualityIcons(t *testing.T) {
	qualities := []SpecQuality{QualityExcellent, QualityGood, QualityFair, QualityPoor}
	for _, q := range qualities {
		icon := qualityIcon(q)
		if strings.Contains(icon, "Unknown") {
			t.Errorf("quality %s should have specific icon", q)
		}
	}
}

func TestSeverityIcons(t *testing.T) {
	severities := []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo}
	for _, s := range severities {
		icon := severityIcon(s)
		if icon == "⚪" {
			t.Errorf("severity %s should have specific icon", s)
		}
	}
}
