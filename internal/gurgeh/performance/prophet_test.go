package performance

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestProphet_Predict_ClassifiesRealtime(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Live Chat",
		Summary:      "Real-time chat with WebSocket",
		Requirements: []string{"Send messages instantly", "Show typing indicators"},
	}

	profile := prophet.Predict(spec)

	if profile.Class != ClassRealtime {
		t.Errorf("class = %v, want realtime", profile.Class)
	}
}

func TestProphet_Predict_ClassifiesInteractive(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Dashboard",
		Summary:      "Display user statistics and activity",
		Requirements: []string{"Show recent activity", "Display charts"},
	}

	profile := prophet.Predict(spec)

	if profile.Class != ClassInteractive {
		t.Errorf("class = %v, want interactive", profile.Class)
	}
}

func TestProphet_Predict_ClassifiesBatch(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Report Generator",
		Summary:      "Generate nightly reports in batch",
		Requirements: []string{"Run scheduled jobs", "Export data to CSV"},
	}

	profile := prophet.Predict(spec)

	if profile.Class != ClassBatch {
		t.Errorf("class = %v, want batch", profile.Class)
	}
}

func TestProphet_Predict_GeneratesBudgets(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Service",
		Summary:      "REST API for users",
		Requirements: []string{"Store in database", "Return user data"},
	}

	profile := prophet.Predict(spec)

	if len(profile.Budgets) == 0 {
		t.Fatal("expected budgets")
	}

	// Should have API response time budget
	foundAPIBudget := false
	for _, budget := range profile.Budgets {
		if strings.Contains(budget.Metric, "API Response") {
			foundAPIBudget = true
			break
		}
	}
	if !foundAPIBudget {
		t.Error("expected API response time budget")
	}

	// Should have error rate budget
	foundErrorBudget := false
	for _, budget := range profile.Budgets {
		if strings.Contains(budget.Metric, "Error") {
			foundErrorBudget = true
			break
		}
	}
	if !foundErrorBudget {
		t.Error("expected error rate budget")
	}
}

func TestProphet_Predict_RealtimeBudgetsStricter(t *testing.T) {
	prophet := NewProphet()

	realtimeSpec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Live Updates",
		Summary:      "Real-time data streaming",
		Requirements: []string{"WebSocket connections"},
	}
	realtimeProfile := prophet.Predict(realtimeSpec)

	interactiveSpec := &specs.Spec{
		ID:           "SPEC-002",
		Title:        "Dashboard",
		Summary:      "User dashboard",
		Requirements: []string{"Show user data"},
	}
	interactiveProfile := prophet.Predict(interactiveSpec)

	// Realtime should have WebSocket latency budget
	hasWebSocketBudget := false
	for _, b := range realtimeProfile.Budgets {
		if strings.Contains(b.Metric, "WebSocket") {
			hasWebSocketBudget = true
			break
		}
	}
	if !hasWebSocketBudget {
		t.Error("realtime should have WebSocket latency budget")
	}

	// Interactive should not have WebSocket budget
	for _, b := range interactiveProfile.Budgets {
		if strings.Contains(b.Metric, "WebSocket") {
			t.Error("interactive should not have WebSocket budget")
		}
	}
}

func TestProphet_Predict_MakesPredictions(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "File Upload",
		Summary:      "Allow users to upload files",
		Requirements: []string{"Upload images", "Store files"},
	}

	profile := prophet.Predict(spec)

	if len(profile.Predictions) == 0 {
		t.Error("expected predictions")
	}

	// Should predict file upload concerns
	foundUploadPrediction := false
	for _, pred := range profile.Predictions {
		if strings.Contains(strings.ToLower(pred.Concern), "upload") ||
			strings.Contains(strings.ToLower(pred.Concern), "file") {
			foundUploadPrediction = true
			break
		}
	}
	if !foundUploadPrediction {
		t.Error("expected file upload prediction")
	}
}

func TestProphet_Predict_N1QueryPrediction(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User List",
		Summary:      "List all users with their orders",
		Requirements: []string{"Show each user's order history"},
	}

	profile := prophet.Predict(spec)

	foundN1 := false
	for _, pred := range profile.Predictions {
		if strings.Contains(pred.Concern, "N+1") {
			foundN1 = true
			break
		}
	}
	if !foundN1 {
		t.Error("expected N+1 query prediction for list operations")
	}
}

func TestProphet_Predict_AssessesScaling(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Service",
		Summary:      "High-traffic API with database",
		Requirements: []string{"Handle API requests", "Store in database", "Cache results"},
	}

	profile := prophet.Predict(spec)

	if len(profile.Scaling) == 0 {
		t.Error("expected scaling considerations")
	}

	// Should have database scaling consideration
	foundDB := false
	for _, scale := range profile.Scaling {
		if scale.Component == "Database" {
			foundDB = true
			break
		}
	}
	if !foundDB {
		t.Error("expected database scaling consideration")
	}
}

func TestProphet_Predict_RecommendsMonitoring(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Service",
		Summary:      "A service",
		Requirements: []string{"Handle requests"},
	}

	profile := prophet.Predict(spec)

	if len(profile.Monitoring) == 0 {
		t.Error("expected monitoring recommendations")
	}

	// Should always recommend response time monitoring
	foundResponseTime := false
	for _, m := range profile.Monitoring {
		if strings.Contains(strings.ToLower(m), "response time") {
			foundResponseTime = true
			break
		}
	}
	if !foundResponseTime {
		t.Error("expected response time monitoring")
	}
}

func TestProphet_Predict_RealtimeMonitoring(t *testing.T) {
	prophet := NewProphet()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Live Chat",
		Summary:      "Real-time chat",
		Requirements: []string{"WebSocket connections"},
	}

	profile := prophet.Predict(spec)

	// Should recommend WebSocket monitoring
	foundWSMonitoring := false
	for _, m := range profile.Monitoring {
		if strings.Contains(strings.ToLower(m), "websocket") || strings.Contains(strings.ToLower(m), "connection") {
			foundWSMonitoring = true
			break
		}
	}
	if !foundWSMonitoring {
		t.Error("expected WebSocket monitoring for realtime")
	}
}

func TestProphet_PredictPRD_AggregatesProfiles(t *testing.T) {
	prophet := NewProphet()

	prd := &specs.PRD{
		ID:    "MVP",
		Title: "MVP Release",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "Live Chat",
				Summary:      "Real-time chat",
				Requirements: []string{"WebSocket chat"},
			},
			{
				ID:           "FEAT-002",
				Title:        "Reports",
				Summary:      "Batch reports",
				Requirements: []string{"Generate nightly reports"},
			},
		},
	}

	profile := prophet.PredictPRD(prd)

	// Should use most demanding class (realtime)
	if profile.Class != ClassRealtime {
		t.Errorf("class = %v, want realtime (most demanding)", profile.Class)
	}
}

func TestFormatPerformanceProfile(t *testing.T) {
	profile := &PerformanceProfile{
		SpecID: "SPEC-001",
		Class:  ClassInteractive,
		Budgets: []Budget{
			{Metric: "API Response Time", Target: "300ms", Measurement: "APM"},
		},
		Predictions: []Prediction{
			{Area: "Database", Concern: "N+1 queries", Impact: "Slow", Confidence: ConfidenceMedium, Suggestion: "Eager load"},
		},
		Scaling: []ScalingConsideration{
			{Component: "API", Bottleneck: "Concurrency", ScaleMethod: "Horizontal"},
		},
		Monitoring: []string{"Response time"},
	}

	output := FormatPerformanceProfile(profile)

	if !strings.Contains(output, "Performance Profile") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "Performance Budgets") {
		t.Error("should contain budgets section")
	}
	if !strings.Contains(output, "Performance Predictions") {
		t.Error("should contain predictions section")
	}
	if !strings.Contains(output, "Scaling Considerations") {
		t.Error("should contain scaling section")
	}
	if !strings.Contains(output, "Monitoring") {
		t.Error("should contain monitoring section")
	}
}

func TestBuildProphetBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Feature",
		Summary: "A test feature",
	}

	brief := BuildProphetBrief(spec)

	if !strings.Contains(brief, "Prophet Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "Performance Classification") {
		t.Error("should reference classification")
	}
	if !strings.Contains(brief, "Budget") {
		t.Error("should reference budgets")
	}
}

func TestClassIcons(t *testing.T) {
	classes := []PerformanceClass{ClassRealtime, ClassInteractive, ClassBatch}
	for _, class := range classes {
		icon := classIcon(class)
		if icon == "❓ Unknown" {
			t.Errorf("class %s should have specific icon", class)
		}
	}
}
