package readiness

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestGenerator_GenerateFromSpec_FeatureFlag(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "User Authentication",
	}

	checklist := gen.GenerateFromSpec(spec)

	if checklist.FeatureFlag == nil {
		t.Fatal("expected feature flag config")
	}
	if checklist.FeatureFlag.Name == "" {
		t.Error("expected feature flag name")
	}
	if !checklist.FeatureFlag.DefaultOff {
		t.Error("expected feature flag to default to off")
	}
	if !checklist.FeatureFlag.Gradual {
		t.Error("expected feature flag to support gradual rollout")
	}
}

func TestGenerator_GenerateFromSpec_Monitoring(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.Monitoring) == 0 {
		t.Fatal("expected monitoring configs")
	}

	// Should have metrics and alerts
	hasMetric := false
	hasAlert := false
	for _, m := range checklist.Monitoring {
		if m.Type == "metric" {
			hasMetric = true
		}
		if m.Type == "alert" {
			hasAlert = true
		}
	}
	if !hasMetric {
		t.Error("expected metric monitoring")
	}
	if !hasAlert {
		t.Error("expected alert monitoring")
	}
}

func TestGenerator_GenerateFromSpec_PaymentMonitoring(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Checkout",
		Requirements: []string{"Process payment transactions"},
	}

	checklist := gen.GenerateFromSpec(spec)

	// Should have payment-specific alert
	foundPaymentAlert := false
	for _, m := range checklist.Monitoring {
		if strings.Contains(m.Name, "payment") {
			foundPaymentAlert = true
			break
		}
	}
	if !foundPaymentAlert {
		t.Error("expected payment failure alert for payment spec")
	}
}

func TestGenerator_GenerateFromSpec_Logging(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if checklist.Logging.Level == "" {
		t.Error("expected logging level")
	}
	if !checklist.Logging.Structured {
		t.Error("expected structured logging")
	}
	if len(checklist.Logging.Events) == 0 {
		t.Error("expected logging events")
	}
}

func TestGenerator_GenerateFromSpec_LoggingRedactions(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth",
		Requirements: []string{"User login with password"},
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.Logging.Redactions) == 0 {
		t.Error("expected redactions for auth-related spec")
	}

	// Should redact password
	foundPassword := false
	for _, r := range checklist.Logging.Redactions {
		if r == "password" {
			foundPassword = true
			break
		}
	}
	if !foundPassword {
		t.Error("expected password in redactions")
	}
}

func TestGenerator_GenerateFromSpec_Rollback(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if checklist.Rollback.Strategy == "" {
		t.Error("expected rollback strategy")
	}
	if checklist.Rollback.Timeframe == "" {
		t.Error("expected rollback timeframe")
	}
	if len(checklist.Rollback.Steps) == 0 {
		t.Error("expected rollback steps")
	}
}

func TestGenerator_GenerateFromSpec_RollbackWithDataBackup(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Data Migration",
		Requirements: []string{"Migrate existing user data", "Delete old records"},
	}

	checklist := gen.GenerateFromSpec(spec)

	if !checklist.Rollback.DataBackup {
		t.Error("expected data backup for data modification spec")
	}
}

func TestGenerator_GenerateFromSpec_Migrations(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "New Entity",
		Requirements: []string{"Store new entity in database"},
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.Migrations) == 0 {
		t.Error("expected migration steps for database spec")
	}
}

func TestGenerator_GenerateFromSpec_EnvVars(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.EnvVars) == 0 {
		t.Error("expected environment variables")
	}
}

func TestGenerator_GenerateFromSpec_EnvVarsAPI(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Integration",
		Requirements: []string{"Integrate with external API"},
	}

	checklist := gen.GenerateFromSpec(spec)

	foundAPIKey := false
	for _, env := range checklist.EnvVars {
		if strings.Contains(env.Name, "API") && env.Sensitive {
			foundAPIKey = true
			break
		}
	}
	if !foundAPIKey {
		t.Error("expected sensitive API key env var for API spec")
	}
}

func TestGenerator_GenerateFromSpec_Documentation(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.Documentation) == 0 {
		t.Error("expected documentation requirements")
	}

	// Should always have runbook
	foundRunbook := false
	for _, doc := range checklist.Documentation {
		if doc.Type == "runbook" {
			foundRunbook = true
			break
		}
	}
	if !foundRunbook {
		t.Error("expected runbook documentation")
	}
}

func TestGenerator_GenerateFromSpec_APIDocumentation(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "API Feature",
		Requirements: []string{"New API endpoint for data"},
	}

	checklist := gen.GenerateFromSpec(spec)

	foundAPIDocs := false
	for _, doc := range checklist.Documentation {
		if doc.Type == "api" {
			foundAPIDocs = true
			break
		}
	}
	if !foundAPIDocs {
		t.Error("expected API documentation for API spec")
	}
}

func TestGenerator_GenerateFromSpec_Checklist(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	if len(checklist.Checklist) == 0 {
		t.Error("expected checklist items")
	}

	// Check that required categories exist
	categories := make(map[string]bool)
	for _, item := range checklist.Checklist {
		categories[item.Category] = true
	}

	requiredCategories := []string{"feature_flag", "monitoring", "logging", "rollback", "testing", "review"}
	for _, cat := range requiredCategories {
		if !categories[cat] {
			t.Errorf("expected checklist category: %s", cat)
		}
	}
}

func TestGenerator_GenerateFromSpec_SecurityReview(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth Feature",
		Requirements: []string{"User authentication with password"},
	}

	checklist := gen.GenerateFromSpec(spec)

	foundSecurityReview := false
	for _, item := range checklist.Checklist {
		if item.Category == "review" && strings.Contains(item.Description, "security") {
			foundSecurityReview = true
			break
		}
	}
	if !foundSecurityReview {
		t.Error("expected security review for auth spec")
	}
}

func TestGenerator_GenerateFromPRD(t *testing.T) {
	gen := NewGenerator()

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "MVP Release",
		Version: "mvp",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "User Auth",
				Requirements: []string{"User login"},
			},
		},
	}

	checklist := gen.GenerateFromPRD(prd)

	if checklist.SpecID != "MVP" {
		t.Errorf("SpecID = %s, want MVP", checklist.SpecID)
	}
	if len(checklist.Checklist) == 0 {
		t.Error("expected checklist items")
	}
}

func TestFormatReadinessChecklist(t *testing.T) {
	checklist := &ReadinessChecklist{
		SpecID: "SPEC-001",
		FeatureFlag: &FeatureFlagConfig{
			Name:        "feature_test",
			Description: "Test feature",
			DefaultOff:  true,
			Gradual:     true,
		},
		Monitoring: []MonitoringConfig{
			{Type: "metric", Name: "test_metric", Description: "Test metric"},
			{Type: "alert", Name: "test_alert", Description: "Test alert", Threshold: "> 1%"},
		},
		Logging: LoggingStrategy{
			Level:      "info",
			Structured: true,
			Events:     []string{"test_event"},
			Redactions: []string{"password"},
		},
		Rollback: RollbackPlan{
			Strategy:   "feature_flag",
			Timeframe:  "5 minutes",
			Steps:      []string{"Disable flag"},
			DataBackup: false,
		},
		EnvVars: []EnvVarRequirement{
			{Name: "TEST_VAR", Description: "Test variable", Required: true, Sensitive: false},
		},
		Checklist: []ChecklistItem{
			{ID: "CHK-001", Category: "testing", Description: "Test item", Priority: PriorityRequired, Status: StatusPending},
		},
	}

	output := FormatReadinessChecklist(checklist)

	if !strings.Contains(output, "Implementation Readiness Checklist") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "SPEC-001") {
		t.Error("should contain spec ID")
	}
	if !strings.Contains(output, "Feature Flag") {
		t.Error("should contain feature flag section")
	}
	if !strings.Contains(output, "Monitoring") {
		t.Error("should contain monitoring section")
	}
	if !strings.Contains(output, "Logging") {
		t.Error("should contain logging section")
	}
	if !strings.Contains(output, "Rollback") {
		t.Error("should contain rollback section")
	}
	if !strings.Contains(output, "Environment Variables") {
		t.Error("should contain env vars section")
	}
	if !strings.Contains(output, "Checklist") {
		t.Error("should contain checklist section")
	}
}

func TestBuildReadinessBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Feature",
		Summary:      "A test feature",
		Requirements: []string{"Req 1"},
	}

	brief := BuildReadinessBrief(spec)

	if !strings.Contains(brief, "Readiness Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "Feature Flags") {
		t.Error("should contain feature flags reference")
	}
	if !strings.Contains(brief, "Monitoring") {
		t.Error("should contain monitoring reference")
	}
	if !strings.Contains(brief, "Rollback") {
		t.Error("should contain rollback reference")
	}
}

func TestChecklistItemPriorities(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	hasRequired := false
	for _, item := range checklist.Checklist {
		if item.Priority == PriorityRequired {
			hasRequired = true
			break
		}
	}
	if !hasRequired {
		t.Error("expected some required priority items")
	}
}

func TestChecklistItemStatuses(t *testing.T) {
	gen := NewGenerator()

	spec := &specs.Spec{
		ID:    "SPEC-001",
		Title: "Test Feature",
	}

	checklist := gen.GenerateFromSpec(spec)

	// All items should start as pending
	for _, item := range checklist.Checklist {
		if item.Status != StatusPending {
			t.Errorf("expected all items to start as pending, got %s for %s", item.Status, item.ID)
		}
	}
}
