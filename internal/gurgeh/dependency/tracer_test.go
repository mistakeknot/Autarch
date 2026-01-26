package dependency

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestTracer_Trace_ExtractsExternalDependencies(t *testing.T) {
	tracer := NewTracer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Payment System",
		Summary:      "Handle payments with Stripe integration",
		Requirements: []string{"Accept credit cards via Stripe", "Send receipts via SendGrid"},
	}

	dm := tracer.Trace(spec)

	// Should detect Stripe
	foundStripe := false
	foundSendGrid := false
	for _, dep := range dm.Dependencies {
		if dep.Name == "stripe" {
			foundStripe = true
			if dep.Type != DepTypeExternal {
				t.Errorf("stripe type = %v, want external", dep.Type)
			}
			if !dep.Critical {
				t.Error("stripe should be marked as critical")
			}
		}
		if dep.Name == "sendgrid" {
			foundSendGrid = true
		}
	}
	if !foundStripe {
		t.Error("expected stripe dependency")
	}
	if !foundSendGrid {
		t.Error("expected sendgrid dependency")
	}
}

func TestTracer_Trace_ExtractsDataDependencies(t *testing.T) {
	tracer := NewTracer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Service",
		Summary:      "Store user data in PostgreSQL with Redis caching",
		Requirements: []string{"Persist user profiles", "Cache sessions in Redis"},
	}

	dm := tracer.Trace(spec)

	foundPostgres := false
	foundRedis := false
	for _, dep := range dm.Dependencies {
		if dep.Name == "postgres" || dep.Name == "postgresql" {
			foundPostgres = true
			if dep.Type != DepTypeData {
				t.Errorf("postgres type = %v, want data", dep.Type)
			}
		}
		if dep.Name == "redis" {
			foundRedis = true
		}
	}
	if !foundPostgres {
		t.Error("expected postgres dependency")
	}
	if !foundRedis {
		t.Error("expected redis dependency")
	}
}

func TestTracer_Trace_AssessesRisks(t *testing.T) {
	tracer := NewTracer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Integration Heavy Feature",
		Summary:      "Integrate with multiple external services",
		Requirements: []string{"Use Stripe for payments", "Use Twilio for SMS", "Store in PostgreSQL"},
	}

	dm := tracer.Trace(spec)

	if len(dm.Risks) == 0 {
		t.Error("expected risk assessment")
	}

	// Should have external API risk
	foundExternalRisk := false
	for _, risk := range dm.Risks {
		if strings.Contains(risk.Risk, "Third-party") || strings.Contains(risk.Risk, "external") {
			foundExternalRisk = true
			break
		}
	}
	if !foundExternalRisk {
		t.Error("expected external API risk")
	}
}

func TestTracer_Trace_IdentifiesCriticalPath(t *testing.T) {
	tracer := NewTracer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth System",
		Summary:      "OAuth authentication with PostgreSQL",
		Requirements: []string{"Authenticate via OAuth", "Store sessions in database"},
	}

	dm := tracer.Trace(spec)

	// OAuth and database should be on critical path
	if len(dm.CriticalPath) == 0 {
		t.Error("expected critical path dependencies")
	}
}

func TestTracer_Trace_CalculatesOverallRisk(t *testing.T) {
	tracer := NewTracer()

	// Low risk spec
	lowRiskSpec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Simple Feature",
		Summary:      "A simple feature with no external dependencies",
		Requirements: []string{"Display a form", "Show confirmation"},
	}
	dmLow := tracer.Trace(lowRiskSpec)
	if dmLow.OverallRisk == RiskCritical || dmLow.OverallRisk == RiskHigh {
		t.Errorf("simple spec should have low/medium risk, got %v", dmLow.OverallRisk)
	}

	// High risk spec
	highRiskSpec := &specs.Spec{
		ID:           "SPEC-002",
		Title:        "Complex Integration",
		Summary:      "Multiple critical integrations",
		Requirements: []string{
			"Process payments via Stripe",
			"Authenticate via OAuth",
			"Store in PostgreSQL",
			"Send SMS via Twilio",
		},
	}
	dmHigh := tracer.Trace(highRiskSpec)
	if dmHigh.OverallRisk == RiskLow {
		t.Errorf("complex spec should have higher risk, got %v", dmHigh.OverallRisk)
	}
}

func TestTracer_Trace_GeneratesRecommendations(t *testing.T) {
	tracer := NewTracer()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Multi-Integration",
		Summary:      "Many external APIs",
		Requirements: []string{
			"Use Stripe",
			"Use Twilio",
			"Use SendGrid",
			"Use Firebase",
		},
	}

	dm := tracer.Trace(spec)

	// Should recommend unified error handling for multiple external deps
	foundRec := false
	for _, rec := range dm.Recommendations {
		if strings.Contains(rec, "external") || strings.Contains(rec, "error handling") {
			foundRec = true
			break
		}
	}
	if !foundRec {
		t.Error("expected recommendation for multiple external dependencies")
	}
}

func TestTracer_TracePRD_AggregatesDependencies(t *testing.T) {
	tracer := NewTracer()

	prd := &specs.PRD{
		ID:    "MVP",
		Title: "MVP Release",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "Payments",
				Requirements: []string{"Use Stripe"},
			},
			{
				ID:           "FEAT-002",
				Title:        "Notifications",
				Requirements: []string{"Use Twilio"},
			},
		},
	}

	dm := tracer.TracePRD(prd)

	// Should have dependencies from both features
	if len(dm.Dependencies) < 2 {
		t.Errorf("expected at least 2 dependencies, got %d", len(dm.Dependencies))
	}

	foundStripe := false
	foundTwilio := false
	for _, dep := range dm.Dependencies {
		if dep.Name == "stripe" {
			foundStripe = true
		}
		if dep.Name == "twilio" {
			foundTwilio = true
		}
	}
	if !foundStripe {
		t.Error("expected stripe from payments feature")
	}
	if !foundTwilio {
		t.Error("expected twilio from notifications feature")
	}
}

func TestFormatDependencyMap(t *testing.T) {
	dm := &DependencyMap{
		SpecID: "SPEC-001",
		Dependencies: []Dependency{
			{Name: "stripe", Type: DepTypeExternal, Description: "Payments", Critical: true},
		},
		CriticalPath: []string{"stripe"},
		Risks: []DependencyRisk{
			{DependencyName: "stripe", Risk: "API availability", Level: RiskMedium, Mitigation: "Circuit breaker"},
		},
		OverallRisk: RiskMedium,
		Recommendations: []string{"Monitor API health"},
	}

	output := FormatDependencyMap(dm)

	if !strings.Contains(output, "Dependency Analysis Report") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "stripe") {
		t.Error("should contain dependency name")
	}
	if !strings.Contains(output, "Critical Path") {
		t.Error("should contain critical path section")
	}
	if !strings.Contains(output, "Risks") {
		t.Error("should contain risks section")
	}
	if !strings.Contains(output, "Recommendations") {
		t.Error("should contain recommendations section")
	}
}

func TestBuildTracerBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:      "SPEC-001",
		Title:   "Test Feature",
		Summary: "A test feature",
	}

	brief := BuildTracerBrief(spec)

	if !strings.Contains(brief, "Tracer Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "External Dependencies") {
		t.Error("should reference external dependencies")
	}
	if !strings.Contains(brief, "Risk Assessment") {
		t.Error("should reference risk assessment")
	}
}

func TestRiskLevels(t *testing.T) {
	// Test all risk icons exist
	levels := []RiskLevel{RiskCritical, RiskHigh, RiskMedium, RiskLow}
	for _, level := range levels {
		icon := riskIcon(level)
		if icon == "⚪ Unknown" {
			t.Errorf("risk level %s should have specific icon", level)
		}
	}
}
