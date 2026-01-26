// Package nfr provides non-functional requirements extraction from specs.
// The "sentinel" subagent guards against security vulnerabilities through
// proactive threat analysis and generates NFRs for security, performance, and accessibility.
package nfr

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// ThreatCategory represents a STRIDE threat category
type ThreatCategory string

const (
	ThreatSpoofing            ThreatCategory = "spoofing"             // Identity spoofing
	ThreatTampering           ThreatCategory = "tampering"            // Data tampering
	ThreatRepudiation         ThreatCategory = "repudiation"          // Denial of actions
	ThreatInfoDisclosure      ThreatCategory = "info_disclosure"      // Information disclosure
	ThreatDenialOfService     ThreatCategory = "denial_of_service"    // DoS attacks
	ThreatElevationPrivilege  ThreatCategory = "elevation_privilege"  // Privilege escalation
)

// Severity indicates the severity of a threat
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// DataSensitivity indicates the type of sensitive data
type DataSensitivity string

const (
	DataSensitivityPII     DataSensitivity = "pii"     // Personally Identifiable Information
	DataSensitivityPCI     DataSensitivity = "pci"     // Payment Card Industry
	DataSensitivityPHI     DataSensitivity = "phi"     // Protected Health Information
	DataSensitivityGeneral DataSensitivity = "general" // General sensitive data
)

// Threat represents a security threat identified in the spec
type Threat struct {
	ID          string         `yaml:"id" json:"id"`
	Category    ThreatCategory `yaml:"category" json:"category"`
	Title       string         `yaml:"title" json:"title"`
	Description string         `yaml:"description" json:"description"`
	Severity    Severity       `yaml:"severity" json:"severity"`
	Trigger     string         `yaml:"trigger" json:"trigger"` // What in the spec triggered this
	Assets      []string       `yaml:"assets,omitempty" json:"assets,omitempty"`
}

// Mitigation represents an OWASP-aligned mitigation strategy
type Mitigation struct {
	ThreatID     string   `yaml:"threat_id" json:"threat_id"`
	Strategy     string   `yaml:"strategy" json:"strategy"`
	OWASPRef     string   `yaml:"owasp_ref,omitempty" json:"owasp_ref,omitempty"` // OWASP reference ID
	Implementation string `yaml:"implementation" json:"implementation"`
	Priority     Severity `yaml:"priority" json:"priority"`
}

// SecurityNFRs contains security-related non-functional requirements
type SecurityNFRs struct {
	Threats         []Threat          `yaml:"threats" json:"threats"`
	Mitigations     []Mitigation      `yaml:"mitigations" json:"mitigations"`
	DataSensitivity []DataSensitivity `yaml:"data_sensitivity" json:"data_sensitivity"`
	Requirements    []string          `yaml:"requirements" json:"requirements"`
}

// PerformanceNFRs contains performance-related non-functional requirements
type PerformanceNFRs struct {
	PageLoadBudgetMs   int    `yaml:"page_load_budget_ms" json:"page_load_budget_ms"`
	APIResponseP95Ms   int    `yaml:"api_response_p95_ms" json:"api_response_p95_ms"`
	BundleSizeBudgetKB int    `yaml:"bundle_size_budget_kb" json:"bundle_size_budget_kb"`
	ConcurrentUsers    int    `yaml:"concurrent_users" json:"concurrent_users"`
	Requirements       []string `yaml:"requirements" json:"requirements"`
}

// AccessibilityNFRs contains accessibility-related non-functional requirements
type AccessibilityNFRs struct {
	WCAGLevel    string   `yaml:"wcag_level" json:"wcag_level"` // A, AA, AAA
	Requirements []string `yaml:"requirements" json:"requirements"`
}

// NFRExtraction contains all extracted non-functional requirements
type NFRExtraction struct {
	SpecID        string            `yaml:"spec_id" json:"spec_id"`
	Security      SecurityNFRs      `yaml:"security" json:"security"`
	Performance   PerformanceNFRs   `yaml:"performance" json:"performance"`
	Accessibility AccessibilityNFRs `yaml:"accessibility" json:"accessibility"`
}

// Sentinel extracts NFRs from specs using STRIDE analysis
type Sentinel struct {
	threatCounter int
}

// NewSentinel creates a new sentinel analyzer
func NewSentinel() *Sentinel {
	return &Sentinel{}
}

// AnalyzeSpec extracts NFRs from a spec
func (s *Sentinel) AnalyzeSpec(spec *specs.Spec) *NFRExtraction {
	extraction := &NFRExtraction{
		SpecID: spec.ID,
	}

	// Aggregate all text for analysis
	allText := s.aggregateText(spec)

	// Security analysis (STRIDE)
	extraction.Security = s.analyzeSecurityThreats(allText, spec)

	// Performance analysis
	extraction.Performance = s.analyzePerformance(allText)

	// Accessibility analysis
	extraction.Accessibility = s.analyzeAccessibility(allText)

	return extraction
}

// AnalyzePRD extracts NFRs from a PRD
func (s *Sentinel) AnalyzePRD(prd *specs.PRD) *NFRExtraction {
	extraction := &NFRExtraction{
		SpecID: prd.ID,
	}

	// Aggregate all text from PRD features
	var allText []string
	allText = append(allText, prd.Title)
	for _, feature := range prd.Features {
		allText = append(allText, feature.Title, feature.Summary)
		allText = append(allText, feature.Requirements...)
	}
	text := strings.ToLower(strings.Join(allText, " "))

	// Create synthetic spec for analysis
	syntheticSpec := &specs.Spec{ID: prd.ID, Title: prd.Title}
	for _, feature := range prd.Features {
		syntheticSpec.Requirements = append(syntheticSpec.Requirements, feature.Requirements...)
	}

	extraction.Security = s.analyzeSecurityThreats(text, syntheticSpec)
	extraction.Performance = s.analyzePerformance(text)
	extraction.Accessibility = s.analyzeAccessibility(text)

	return extraction
}

// aggregateText combines all spec text for analysis
func (s *Sentinel) aggregateText(spec *specs.Spec) string {
	var parts []string
	parts = append(parts, spec.Title, spec.Summary)
	parts = append(parts, spec.Requirements...)
	for _, cuj := range spec.CriticalUserJourneys {
		parts = append(parts, cuj.Title)
		parts = append(parts, cuj.Steps...)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// analyzeSecurityThreats performs STRIDE analysis
func (s *Sentinel) analyzeSecurityThreats(text string, spec *specs.Spec) SecurityNFRs {
	nfrs := SecurityNFRs{}

	// Detect data sensitivity
	nfrs.DataSensitivity = s.detectDataSensitivity(text)

	// STRIDE threat analysis
	s.threatCounter = 0

	// Spoofing threats
	if s.detectAuthPatterns(text) {
		threat := s.createThreat(ThreatSpoofing, "Identity Spoofing Risk",
			"Spec involves authentication - attackers may attempt to impersonate legitimate users",
			SeverityHigh, "authentication patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Strong Authentication",
			OWASPRef:       "A07:2021",
			Implementation: "Implement MFA, secure session management, and rate limiting on auth endpoints",
			Priority:       SeverityHigh,
		})
	}

	// Tampering threats
	if s.detectDataModificationPatterns(text) {
		threat := s.createThreat(ThreatTampering, "Data Tampering Risk",
			"Spec involves data modification - inputs must be validated and integrity protected",
			SeverityHigh, "data modification patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Input Validation & Integrity",
			OWASPRef:       "A03:2021",
			Implementation: "Validate all inputs server-side, use parameterized queries, implement CSRF protection",
			Priority:       SeverityHigh,
		})
	}

	// Repudiation threats
	if s.detectTransactionPatterns(text) {
		threat := s.createThreat(ThreatRepudiation, "Audit Trail Requirement",
			"Spec involves transactions - users may deny actions without proper logging",
			SeverityMedium, "transaction patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Comprehensive Logging",
			OWASPRef:       "A09:2021",
			Implementation: "Log all security-relevant events with timestamps, user IDs, and action details",
			Priority:       SeverityMedium,
		})
	}

	// Information Disclosure threats
	if s.detectSensitiveDataPatterns(text) || len(nfrs.DataSensitivity) > 0 {
		severity := SeverityHigh
		if contains(nfrs.DataSensitivity, DataSensitivityPCI) ||
			contains(nfrs.DataSensitivity, DataSensitivityPHI) {
			severity = SeverityCritical
		}
		threat := s.createThreat(ThreatInfoDisclosure, "Sensitive Data Exposure Risk",
			"Spec handles sensitive data that must be protected from unauthorized disclosure",
			severity, "sensitive data patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Data Protection",
			OWASPRef:       "A02:2021",
			Implementation: "Encrypt data at rest and in transit, minimize data exposure, implement access controls",
			Priority:       severity,
		})
	}

	// Denial of Service threats
	if s.detectScalePatterns(text) {
		threat := s.createThreat(ThreatDenialOfService, "Denial of Service Risk",
			"Spec involves scalable operations that could be exploited for DoS",
			SeverityMedium, "scale/throughput patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Rate Limiting & Resource Management",
			OWASPRef:       "A04:2021",
			Implementation: "Implement rate limiting, request timeouts, and resource quotas",
			Priority:       SeverityMedium,
		})
	}

	// Elevation of Privilege threats
	if s.detectPrivilegePatterns(text) {
		threat := s.createThreat(ThreatElevationPrivilege, "Privilege Escalation Risk",
			"Spec involves role-based access - improper checks could lead to privilege escalation",
			SeverityCritical, "role/permission patterns detected")
		nfrs.Threats = append(nfrs.Threats, threat)
		nfrs.Mitigations = append(nfrs.Mitigations, Mitigation{
			ThreatID:       threat.ID,
			Strategy:       "Access Control",
			OWASPRef:       "A01:2021",
			Implementation: "Implement RBAC, verify permissions on every request, deny by default",
			Priority:       SeverityCritical,
		})
	}

	// Generate security requirements
	nfrs.Requirements = s.generateSecurityRequirements(nfrs)

	return nfrs
}

// createThreat creates a new threat with auto-incrementing ID
func (s *Sentinel) createThreat(category ThreatCategory, title, description string, severity Severity, trigger string) Threat {
	s.threatCounter++
	return Threat{
		ID:          fmt.Sprintf("THREAT-%03d", s.threatCounter),
		Category:    category,
		Title:       title,
		Description: description,
		Severity:    severity,
		Trigger:     trigger,
	}
}

// Pattern detection functions
func (s *Sentinel) detectAuthPatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(auth|login|password|credential|session|token|jwt|oauth|sso|sign.?in|sign.?up)`)
	return patterns.MatchString(text)
}

func (s *Sentinel) detectDataModificationPatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(create|update|delete|modify|edit|change|submit|save|write|insert)`)
	return patterns.MatchString(text)
}

func (s *Sentinel) detectTransactionPatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(transaction|payment|purchase|order|transfer|checkout|invoice|billing)`)
	return patterns.MatchString(text)
}

func (s *Sentinel) detectSensitiveDataPatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(personal|private|sensitive|confidential|secret|ssn|social.?security|credit.?card|bank|medical|health)`)
	return patterns.MatchString(text)
}

func (s *Sentinel) detectScalePatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(scale|bulk|batch|concurrent|parallel|thousand|million|high.?volume|throughput)`)
	return patterns.MatchString(text)
}

func (s *Sentinel) detectPrivilegePatterns(text string) bool {
	patterns := regexp.MustCompile(`(?i)(admin|role|permission|privilege|access.?control|authorize|rbac|acl|owner|manager)`)
	return patterns.MatchString(text)
}

// detectDataSensitivity identifies types of sensitive data
func (s *Sentinel) detectDataSensitivity(text string) []DataSensitivity {
	var sensitivities []DataSensitivity

	piiPatterns := regexp.MustCompile(`(?i)(name|email|phone|address|birth|age|ssn|social.?security|driver.?license|passport)`)
	if piiPatterns.MatchString(text) {
		sensitivities = append(sensitivities, DataSensitivityPII)
	}

	pciPatterns := regexp.MustCompile(`(?i)(credit.?card|debit|payment|cvv|card.?number|expir|billing)`)
	if pciPatterns.MatchString(text) {
		sensitivities = append(sensitivities, DataSensitivityPCI)
	}

	phiPatterns := regexp.MustCompile(`(?i)(medical|health|patient|diagnosis|prescription|hipaa|treatment|doctor|hospital)`)
	if phiPatterns.MatchString(text) {
		sensitivities = append(sensitivities, DataSensitivityPHI)
	}

	if len(sensitivities) == 0 {
		// Check for general sensitive patterns
		generalPatterns := regexp.MustCompile(`(?i)(sensitive|private|confidential|secret|encrypt)`)
		if generalPatterns.MatchString(text) {
			sensitivities = append(sensitivities, DataSensitivityGeneral)
		}
	}

	return sensitivities
}

// generateSecurityRequirements creates actionable security requirements
func (s *Sentinel) generateSecurityRequirements(nfrs SecurityNFRs) []string {
	var reqs []string

	// Based on threats
	for _, threat := range nfrs.Threats {
		switch threat.Category {
		case ThreatSpoofing:
			reqs = append(reqs, "REQ-SEC-AUTH: Implement secure authentication with MFA support")
		case ThreatTampering:
			reqs = append(reqs, "REQ-SEC-INPUT: Validate and sanitize all user inputs server-side")
		case ThreatRepudiation:
			reqs = append(reqs, "REQ-SEC-AUDIT: Maintain comprehensive audit logs for all sensitive operations")
		case ThreatInfoDisclosure:
			reqs = append(reqs, "REQ-SEC-DATA: Encrypt sensitive data at rest and in transit")
		case ThreatDenialOfService:
			reqs = append(reqs, "REQ-SEC-RATELIMIT: Implement rate limiting on all public endpoints")
		case ThreatElevationPrivilege:
			reqs = append(reqs, "REQ-SEC-AUTHZ: Implement role-based access control with principle of least privilege")
		}
	}

	// Based on data sensitivity
	for _, ds := range nfrs.DataSensitivity {
		switch ds {
		case DataSensitivityPII:
			reqs = append(reqs, "REQ-SEC-PII: Implement GDPR/CCPA compliance measures for PII handling")
		case DataSensitivityPCI:
			reqs = append(reqs, "REQ-SEC-PCI: Ensure PCI-DSS compliance for payment data")
		case DataSensitivityPHI:
			reqs = append(reqs, "REQ-SEC-PHI: Implement HIPAA-compliant data handling for health information")
		}
	}

	return dedupe(reqs)
}

// analyzePerformance extracts performance NFRs
func (s *Sentinel) analyzePerformance(text string) PerformanceNFRs {
	nfrs := PerformanceNFRs{
		PageLoadBudgetMs:   3000, // Default 3s
		APIResponseP95Ms:   200,  // Default 200ms
		BundleSizeBudgetKB: 500,  // Default 500KB
		ConcurrentUsers:    100,  // Default 100
	}

	// Detect real-time requirements
	realTimePatterns := regexp.MustCompile(`(?i)(real.?time|instant|live|stream|websocket)`)
	if realTimePatterns.MatchString(text) {
		nfrs.APIResponseP95Ms = 100 // Tighter budget for real-time
		nfrs.Requirements = append(nfrs.Requirements, "REQ-PERF-REALTIME: Sub-100ms response time for real-time features")
	}

	// Detect scale requirements
	scalePatterns := regexp.MustCompile(`(?i)(million|thousand|scale|high.?volume|enterprise)`)
	if scalePatterns.MatchString(text) {
		nfrs.ConcurrentUsers = 1000
		nfrs.Requirements = append(nfrs.Requirements, "REQ-PERF-SCALE: Support 1000+ concurrent users")
	}

	// Detect mobile requirements
	mobilePatterns := regexp.MustCompile(`(?i)(mobile|responsive|app|ios|android)`)
	if mobilePatterns.MatchString(text) {
		nfrs.BundleSizeBudgetKB = 300 // Tighter for mobile
		nfrs.Requirements = append(nfrs.Requirements, "REQ-PERF-MOBILE: Optimize for mobile networks with <300KB initial bundle")
	}

	// Default requirements if none specific
	if len(nfrs.Requirements) == 0 {
		nfrs.Requirements = []string{
			"REQ-PERF-LOAD: Page load under 3 seconds on 3G connections",
			"REQ-PERF-API: API response p95 under 200ms",
		}
	}

	return nfrs
}

// analyzeAccessibility extracts accessibility NFRs
func (s *Sentinel) analyzeAccessibility(text string) AccessibilityNFRs {
	nfrs := AccessibilityNFRs{
		WCAGLevel: "AA", // Default to AA
	}

	// Check for explicit accessibility mentions
	explicitA11y := regexp.MustCompile(`(?i)(accessibility|a11y|wcag|aria|screen.?reader)`)
	if explicitA11y.MatchString(text) {
		nfrs.Requirements = append(nfrs.Requirements, "REQ-A11Y-EXPLICIT: Accessibility explicitly mentioned - ensure full compliance")
	}

	// Check for UI elements that need accessibility
	uiPatterns := regexp.MustCompile(`(?i)(form|button|input|modal|dialog|menu|dropdown|table|list)`)
	if uiPatterns.MatchString(text) {
		nfrs.Requirements = append(nfrs.Requirements,
			"REQ-A11Y-KEYBOARD: All interactive elements must be keyboard accessible",
			"REQ-A11Y-LABELS: All form inputs must have associated labels",
			"REQ-A11Y-FOCUS: Visible focus indicators for all focusable elements",
		)
	}

	// Check for media
	mediaPatterns := regexp.MustCompile(`(?i)(video|audio|image|media|player)`)
	if mediaPatterns.MatchString(text) {
		nfrs.Requirements = append(nfrs.Requirements,
			"REQ-A11Y-ALT: All images must have alt text",
			"REQ-A11Y-CAPTIONS: Videos must have captions/transcripts",
		)
	}

	// Default requirements if none specific
	if len(nfrs.Requirements) == 0 {
		nfrs.Requirements = []string{
			"REQ-A11Y-WCAG: Meet WCAG 2.1 Level AA compliance",
			"REQ-A11Y-CONTRAST: Ensure sufficient color contrast ratios",
		}
	}

	return nfrs
}

// FormatNFRReport formats the extraction as markdown
func FormatNFRReport(extraction *NFRExtraction) string {
	var sb strings.Builder

	sb.WriteString("# Non-Functional Requirements Report\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n\n", extraction.SpecID))

	// Security section
	sb.WriteString("## Security (STRIDE Analysis)\n\n")
	if len(extraction.Security.DataSensitivity) > 0 {
		sb.WriteString("### Data Sensitivity\n")
		for _, ds := range extraction.Security.DataSensitivity {
			sb.WriteString(fmt.Sprintf("- %s\n", strings.ToUpper(string(ds))))
		}
		sb.WriteString("\n")
	}

	if len(extraction.Security.Threats) > 0 {
		sb.WriteString("### Identified Threats\n\n")
		for _, threat := range extraction.Security.Threats {
			icon := severityIcon(threat.Severity)
			sb.WriteString(fmt.Sprintf("#### %s %s [%s]\n\n", icon, threat.Title, threat.Category))
			sb.WriteString(fmt.Sprintf("**Severity:** %s | **Trigger:** %s\n\n", threat.Severity, threat.Trigger))
			sb.WriteString(fmt.Sprintf("%s\n\n", threat.Description))
		}
	}

	if len(extraction.Security.Mitigations) > 0 {
		sb.WriteString("### Mitigations\n\n")
		for _, mit := range extraction.Security.Mitigations {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", mit.Strategy, mit.ThreatID, mit.Implementation))
			if mit.OWASPRef != "" {
				sb.WriteString(fmt.Sprintf("  - OWASP Reference: %s\n", mit.OWASPRef))
			}
		}
		sb.WriteString("\n")
	}

	if len(extraction.Security.Requirements) > 0 {
		sb.WriteString("### Security Requirements\n")
		for _, req := range extraction.Security.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
		sb.WriteString("\n")
	}

	// Performance section
	sb.WriteString("## Performance\n\n")
	sb.WriteString(fmt.Sprintf("- **Page Load Budget:** %dms\n", extraction.Performance.PageLoadBudgetMs))
	sb.WriteString(fmt.Sprintf("- **API Response (p95):** %dms\n", extraction.Performance.APIResponseP95Ms))
	sb.WriteString(fmt.Sprintf("- **Bundle Size Budget:** %dKB\n", extraction.Performance.BundleSizeBudgetKB))
	sb.WriteString(fmt.Sprintf("- **Concurrent Users:** %d\n\n", extraction.Performance.ConcurrentUsers))
	if len(extraction.Performance.Requirements) > 0 {
		sb.WriteString("### Performance Requirements\n")
		for _, req := range extraction.Performance.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
		sb.WriteString("\n")
	}

	// Accessibility section
	sb.WriteString("## Accessibility\n\n")
	sb.WriteString(fmt.Sprintf("- **WCAG Level:** %s\n\n", extraction.Accessibility.WCAGLevel))
	if len(extraction.Accessibility.Requirements) > 0 {
		sb.WriteString("### Accessibility Requirements\n")
		for _, req := range extraction.Accessibility.Requirements {
			sb.WriteString(fmt.Sprintf("- %s\n", req))
		}
	}

	return sb.String()
}

// BuildSentinelBrief creates an agent brief for NFR analysis
func BuildSentinelBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Sentinel Agent Brief: NFR Analysis\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Perform STRIDE threat analysis and extract non-functional requirements.\n\n")

	sb.WriteString("## Spec to Analyze\n")
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

	sb.WriteString("## Analysis Areas\n\n")
	sb.WriteString("### Security (STRIDE)\n")
	sb.WriteString("- **S**poofing: Identity authentication threats\n")
	sb.WriteString("- **T**ampering: Data integrity threats\n")
	sb.WriteString("- **R**epudiation: Audit and logging requirements\n")
	sb.WriteString("- **I**nformation Disclosure: Data privacy threats\n")
	sb.WriteString("- **D**enial of Service: Availability threats\n")
	sb.WriteString("- **E**levation of Privilege: Authorization threats\n\n")

	sb.WriteString("### Performance\n")
	sb.WriteString("- Page load budgets\n")
	sb.WriteString("- API response times\n")
	sb.WriteString("- Scalability requirements\n\n")

	sb.WriteString("### Accessibility\n")
	sb.WriteString("- WCAG compliance level\n")
	sb.WriteString("- Keyboard navigation\n")
	sb.WriteString("- Screen reader support\n")

	return sb.String()
}

// --- Helper functions ---

func severityIcon(s Severity) string {
	switch s {
	case SeverityCritical:
		return "🔴"
	case SeverityHigh:
		return "🟠"
	case SeverityMedium:
		return "🟡"
	case SeverityLow:
		return "🟢"
	default:
		return "⚪"
	}
}

func contains(slice []DataSensitivity, item DataSensitivity) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func dedupe(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
