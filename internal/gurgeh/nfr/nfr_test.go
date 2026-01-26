package nfr

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func TestSentinel_AnalyzeSpec_AuthThreats(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Authentication",
		Summary:      "Allow users to login with email and password",
		Requirements: []string{"User can sign up", "User can login", "Password reset via email"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect spoofing threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatSpoofing {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected spoofing threat for auth-related spec")
	}

	// Should have related mitigation
	foundMitigation := false
	for _, mit := range extraction.Security.Mitigations {
		if strings.Contains(mit.Strategy, "Authentication") {
			foundMitigation = true
			break
		}
	}
	if !foundMitigation {
		t.Error("expected authentication mitigation")
	}
}

func TestSentinel_AnalyzeSpec_DataModification(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Profile",
		Summary:      "Allow users to update their profile",
		Requirements: []string{"Create profile", "Edit profile", "Delete account"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect tampering threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatTampering {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected tampering threat for data modification spec")
	}
}

func TestSentinel_AnalyzeSpec_Transactions(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Checkout",
		Summary:      "Process payments and orders",
		Requirements: []string{"Create order", "Process payment transaction", "Generate invoice"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect repudiation threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatRepudiation {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected repudiation threat for transaction spec")
	}
}

func TestSentinel_AnalyzeSpec_SensitiveData(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Medical Records",
		Summary:      "Store patient health information",
		Requirements: []string{"Store patient diagnosis", "Access medical history"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect PHI sensitivity
	if !contains(extraction.Security.DataSensitivity, DataSensitivityPHI) {
		t.Error("expected PHI data sensitivity detected")
	}

	// Should detect info disclosure threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatInfoDisclosure {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected info disclosure threat for medical data spec")
	}
}

func TestSentinel_AnalyzeSpec_PrivilegeEscalation(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Admin Panel",
		Summary:      "Manage user roles and permissions",
		Requirements: []string{"Admin can manage roles", "Role-based access control", "Permission management"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect elevation of privilege threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatElevationPrivilege {
			found = true
			if threat.Severity != SeverityCritical {
				t.Errorf("elevation of privilege should be critical, got %s", threat.Severity)
			}
			break
		}
	}
	if !found {
		t.Error("expected elevation of privilege threat for admin spec")
	}
}

func TestSentinel_AnalyzeSpec_DoS(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Bulk Import",
		Summary:      "Handle high volume data imports",
		Requirements: []string{"Bulk import millions of records", "High throughput processing"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should detect DoS threat
	found := false
	for _, threat := range extraction.Security.Threats {
		if threat.Category == ThreatDenialOfService {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected DoS threat for high-volume spec")
	}
}

func TestSentinel_DetectDataSensitivity_PII(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Profile",
		Summary:      "Store user name, email, and phone number",
		Requirements: []string{"Store user address", "Collect date of birth"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	if !contains(extraction.Security.DataSensitivity, DataSensitivityPII) {
		t.Error("expected PII data sensitivity")
	}
}

func TestSentinel_DetectDataSensitivity_PCI(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Payment",
		Summary:      "Process credit card payments",
		Requirements: []string{"Store credit card number", "Validate CVV"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	if !contains(extraction.Security.DataSensitivity, DataSensitivityPCI) {
		t.Error("expected PCI data sensitivity")
	}
}

func TestSentinel_AnalyzePerformance_RealTime(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Chat",
		Summary:      "Real-time messaging",
		Requirements: []string{"Real-time message delivery", "Live typing indicators"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	if extraction.Performance.APIResponseP95Ms >= 200 {
		t.Errorf("expected tighter API budget for real-time, got %dms", extraction.Performance.APIResponseP95Ms)
	}

	foundReq := false
	for _, req := range extraction.Performance.Requirements {
		if strings.Contains(req, "REALTIME") {
			foundReq = true
			break
		}
	}
	if !foundReq {
		t.Error("expected real-time performance requirement")
	}
}

func TestSentinel_AnalyzePerformance_Mobile(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Mobile App",
		Summary:      "Responsive mobile interface",
		Requirements: []string{"Mobile-first design", "Support iOS and Android"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	if extraction.Performance.BundleSizeBudgetKB >= 500 {
		t.Errorf("expected smaller bundle for mobile, got %dKB", extraction.Performance.BundleSizeBudgetKB)
	}

	foundReq := false
	for _, req := range extraction.Performance.Requirements {
		if strings.Contains(req, "MOBILE") {
			foundReq = true
			break
		}
	}
	if !foundReq {
		t.Error("expected mobile performance requirement")
	}
}

func TestSentinel_AnalyzeAccessibility_UI(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "User Form",
		Summary:      "Registration form",
		Requirements: []string{"Form with input fields", "Submit button", "Dropdown menu"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should have keyboard accessibility requirement
	foundKeyboard := false
	for _, req := range extraction.Accessibility.Requirements {
		if strings.Contains(req, "KEYBOARD") {
			foundKeyboard = true
			break
		}
	}
	if !foundKeyboard {
		t.Error("expected keyboard accessibility requirement for UI spec")
	}
}

func TestSentinel_AnalyzeAccessibility_Media(t *testing.T) {
	sentinel := NewSentinel()

	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Video Player",
		Summary:      "Stream video content",
		Requirements: []string{"Video playback", "Audio controls", "Image thumbnails"},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Should have alt text requirement
	foundAlt := false
	foundCaptions := false
	for _, req := range extraction.Accessibility.Requirements {
		if strings.Contains(req, "ALT") {
			foundAlt = true
		}
		if strings.Contains(req, "CAPTIONS") {
			foundCaptions = true
		}
	}
	if !foundAlt {
		t.Error("expected alt text requirement for media spec")
	}
	if !foundCaptions {
		t.Error("expected captions requirement for video spec")
	}
}

func TestSentinel_AnalyzePRD(t *testing.T) {
	sentinel := NewSentinel()

	prd := &specs.PRD{
		ID:      "MVP",
		Title:   "MVP Release",
		Version: "mvp",
		Features: []specs.Feature{
			{
				ID:           "FEAT-001",
				Title:        "User Auth",
				Summary:      "Authentication system",
				Requirements: []string{"User can login with password", "Admin role management"},
			},
			{
				ID:           "FEAT-002",
				Title:        "Payments",
				Summary:      "Payment processing",
				Requirements: []string{"Credit card payments", "Store billing info"},
			},
		},
	}

	extraction := sentinel.AnalyzePRD(prd)

	if extraction.SpecID != "MVP" {
		t.Errorf("SpecID = %s, want MVP", extraction.SpecID)
	}

	// Should detect multiple threats from combined features
	if len(extraction.Security.Threats) < 2 {
		t.Errorf("expected multiple threats, got %d", len(extraction.Security.Threats))
	}

	// Should detect PCI data sensitivity from payment feature
	if !contains(extraction.Security.DataSensitivity, DataSensitivityPCI) {
		t.Error("expected PCI data sensitivity from payment feature")
	}
}

func TestFormatNFRReport(t *testing.T) {
	extraction := &NFRExtraction{
		SpecID: "SPEC-001",
		Security: SecurityNFRs{
			DataSensitivity: []DataSensitivity{DataSensitivityPII},
			Threats: []Threat{
				{
					ID:          "THREAT-001",
					Category:    ThreatSpoofing,
					Title:       "Test Threat",
					Description: "Test description",
					Severity:    SeverityHigh,
					Trigger:     "test trigger",
				},
			},
			Mitigations: []Mitigation{
				{
					ThreatID:       "THREAT-001",
					Strategy:       "Test Strategy",
					OWASPRef:       "A01:2021",
					Implementation: "Test implementation",
					Priority:       SeverityHigh,
				},
			},
			Requirements: []string{"REQ-SEC-TEST: Test requirement"},
		},
		Performance: PerformanceNFRs{
			PageLoadBudgetMs:   3000,
			APIResponseP95Ms:   200,
			BundleSizeBudgetKB: 500,
			ConcurrentUsers:    100,
			Requirements:       []string{"REQ-PERF-TEST: Test performance req"},
		},
		Accessibility: AccessibilityNFRs{
			WCAGLevel:    "AA",
			Requirements: []string{"REQ-A11Y-TEST: Test a11y req"},
		},
	}

	output := FormatNFRReport(extraction)

	if !strings.Contains(output, "Non-Functional Requirements Report") {
		t.Error("should contain report header")
	}
	if !strings.Contains(output, "SPEC-001") {
		t.Error("should contain spec ID")
	}
	if !strings.Contains(output, "STRIDE Analysis") {
		t.Error("should contain STRIDE section")
	}
	if !strings.Contains(output, "PII") {
		t.Error("should contain data sensitivity")
	}
	if !strings.Contains(output, "Test Threat") {
		t.Error("should contain threat title")
	}
	if !strings.Contains(output, "Test Strategy") {
		t.Error("should contain mitigation")
	}
	if !strings.Contains(output, "Performance") {
		t.Error("should contain performance section")
	}
	if !strings.Contains(output, "Accessibility") {
		t.Error("should contain accessibility section")
	}
}

func TestBuildSentinelBrief(t *testing.T) {
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Test Feature",
		Summary:      "A test feature summary",
		Requirements: []string{"Req 1", "Req 2"},
	}

	brief := BuildSentinelBrief(spec)

	if !strings.Contains(brief, "Sentinel Agent Brief") {
		t.Error("should contain brief header")
	}
	if !strings.Contains(brief, "STRIDE") {
		t.Error("should contain STRIDE reference")
	}
	if !strings.Contains(brief, "poofing") { // **S**poofing contains "poofing"
		t.Error("should contain STRIDE categories")
	}
	if !strings.Contains(brief, "Req 1") {
		t.Error("should contain requirements")
	}
}

func TestGenerateSecurityRequirements_Dedupe(t *testing.T) {
	sentinel := NewSentinel()

	// Create a spec that would trigger multiple similar threats
	spec := &specs.Spec{
		ID:           "SPEC-001",
		Title:        "Auth System",
		Summary:      "Complete auth with login, signup, and admin roles",
		Requirements: []string{
			"User can login",
			"Admin can manage users",
			"Role-based permissions",
		},
	}

	extraction := sentinel.AnalyzeSpec(spec)

	// Check for deduplication
	seen := make(map[string]bool)
	for _, req := range extraction.Security.Requirements {
		if seen[req] {
			t.Errorf("duplicate requirement found: %s", req)
		}
		seen[req] = true
	}
}
