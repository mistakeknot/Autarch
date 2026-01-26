// Package dependency provides the tracer subagent for mapping technical dependencies and risk assessment.
package dependency

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// RiskLevel represents the severity of a dependency risk
type RiskLevel string

const (
	RiskCritical RiskLevel = "critical"
	RiskHigh     RiskLevel = "high"
	RiskMedium   RiskLevel = "medium"
	RiskLow      RiskLevel = "low"
)

// DependencyType categorizes the kind of dependency
type DependencyType string

const (
	DepTypeExternal DependencyType = "external"  // Third-party APIs
	DepTypeInternal DependencyType = "internal"  // Internal services
	DepTypeLibrary  DependencyType = "library"   // Package dependencies
	DepTypeData     DependencyType = "data"      // Database/storage
	DepTypeNetwork  DependencyType = "network"   // Network/infra
)

// Dependency represents a single technical dependency
type Dependency struct {
	Name        string         `yaml:"name" json:"name"`
	Type        DependencyType `yaml:"type" json:"type"`
	Description string         `yaml:"description" json:"description"`
	Critical    bool           `yaml:"critical" json:"critical"`    // Is this on the critical path?
	SLA         string         `yaml:"sla,omitempty" json:"sla,omitempty"` // Expected availability
}

// DependencyRisk represents a risk associated with a dependency
type DependencyRisk struct {
	DependencyName string    `yaml:"dependency" json:"dependency"`
	Risk           string    `yaml:"risk" json:"risk"`
	Level          RiskLevel `yaml:"level" json:"level"`
	Mitigation     string    `yaml:"mitigation" json:"mitigation"`
}

// LicenseIssue represents a potential license compatibility problem
type LicenseIssue struct {
	Package   string `yaml:"package" json:"package"`
	License   string `yaml:"license" json:"license"`
	Concern   string `yaml:"concern" json:"concern"`
}

// DependencyMap represents the full dependency analysis
type DependencyMap struct {
	SpecID         string            `yaml:"spec_id" json:"spec_id"`
	Dependencies   []Dependency      `yaml:"dependencies" json:"dependencies"`
	Risks          []DependencyRisk  `yaml:"risks" json:"risks"`
	LicenseIssues  []LicenseIssue    `yaml:"license_issues" json:"license_issues"`
	CriticalPath   []string          `yaml:"critical_path" json:"critical_path"`
	OverallRisk    RiskLevel         `yaml:"overall_risk" json:"overall_risk"`
	Recommendations []string         `yaml:"recommendations" json:"recommendations"`
}

// Tracer analyzes specs for technical dependencies and risks
type Tracer struct{}

// NewTracer creates a new Tracer instance
func NewTracer() *Tracer {
	return &Tracer{}
}

// Trace analyzes a spec and returns its dependency map
func (t *Tracer) Trace(spec *specs.Spec) *DependencyMap {
	dm := &DependencyMap{
		SpecID:       spec.ID,
		Dependencies: t.extractDependencies(spec),
	}

	dm.Risks = t.assessRisks(dm.Dependencies, spec)
	dm.LicenseIssues = t.checkLicenses(spec)
	dm.CriticalPath = t.identifyCriticalPath(dm.Dependencies)
	dm.OverallRisk = t.calculateOverallRisk(dm.Risks)
	dm.Recommendations = t.generateRecommendations(dm)

	return dm
}

// TracePRD traces dependencies across all features in a PRD
func (t *Tracer) TracePRD(prd *specs.PRD) *DependencyMap {
	dm := &DependencyMap{
		SpecID: prd.ID,
	}

	// Aggregate dependencies from all features
	seen := make(map[string]bool)
	for _, feature := range prd.Features {
		spec := &specs.Spec{
			ID:           feature.ID,
			Title:        feature.Title,
			Summary:      feature.Summary,
			Requirements: feature.Requirements,
		}
		featureDeps := t.extractDependencies(spec)
		for _, dep := range featureDeps {
			if !seen[dep.Name] {
				seen[dep.Name] = true
				dm.Dependencies = append(dm.Dependencies, dep)
			}
		}
	}

	dm.Risks = t.assessRisks(dm.Dependencies, nil)
	dm.CriticalPath = t.identifyCriticalPath(dm.Dependencies)
	dm.OverallRisk = t.calculateOverallRisk(dm.Risks)
	dm.Recommendations = t.generateRecommendations(dm)

	return dm
}

func (t *Tracer) extractDependencies(spec *specs.Spec) []Dependency {
	var deps []Dependency
	text := strings.ToLower(spec.Title + " " + spec.Summary + " " + strings.Join(spec.Requirements, " "))

	// External API patterns
	apiPatterns := map[string]string{
		"stripe":       "Payment processing via Stripe API",
		"twilio":       "SMS/communication via Twilio",
		"sendgrid":     "Email delivery via SendGrid",
		"aws":          "AWS cloud services",
		"gcp":          "Google Cloud Platform services",
		"azure":        "Microsoft Azure services",
		"oauth":        "OAuth authentication provider",
		"github api":   "GitHub API integration",
		"slack":        "Slack integration",
		"firebase":     "Firebase services",
		"openai":       "OpenAI API integration",
		"anthropic":    "Anthropic API integration",
	}

	for pattern, desc := range apiPatterns {
		if strings.Contains(text, pattern) {
			deps = append(deps, Dependency{
				Name:        pattern,
				Type:        DepTypeExternal,
				Description: desc,
				Critical:    pattern == "stripe" || pattern == "oauth", // Payment and auth are critical
			})
		}
	}

	// Database patterns
	dbPatterns := map[string]string{
		"postgres":    "PostgreSQL database",
		"postgresql":  "PostgreSQL database",
		"mysql":       "MySQL database",
		"mongodb":     "MongoDB database",
		"redis":       "Redis cache/database",
		"elasticsearch": "Elasticsearch search engine",
		"sqlite":      "SQLite database",
	}

	for pattern, desc := range dbPatterns {
		if strings.Contains(text, pattern) {
			deps = append(deps, Dependency{
				Name:        pattern,
				Type:        DepTypeData,
				Description: desc,
				Critical:    true, // Databases are usually critical
			})
		}
	}

	// Generic patterns
	if strings.Contains(text, "api") || strings.Contains(text, "integration") {
		if !hasDependencyType(deps, DepTypeExternal) {
			deps = append(deps, Dependency{
				Name:        "external-api",
				Type:        DepTypeExternal,
				Description: "External API integration detected",
				Critical:    false,
			})
		}
	}

	if strings.Contains(text, "database") || strings.Contains(text, "store") || strings.Contains(text, "persist") {
		if !hasDependencyType(deps, DepTypeData) {
			deps = append(deps, Dependency{
				Name:        "database",
				Type:        DepTypeData,
				Description: "Database/storage dependency",
				Critical:    true,
			})
		}
	}

	if strings.Contains(text, "message queue") || strings.Contains(text, "kafka") || strings.Contains(text, "rabbitmq") {
		deps = append(deps, Dependency{
			Name:        "message-queue",
			Type:        DepTypeNetwork,
			Description: "Message queue for async processing",
			Critical:    false,
		})
	}

	return deps
}

func (t *Tracer) assessRisks(deps []Dependency, spec *specs.Spec) []DependencyRisk {
	var risks []DependencyRisk

	for _, dep := range deps {
		switch dep.Type {
		case DepTypeExternal:
			risks = append(risks, DependencyRisk{
				DependencyName: dep.Name,
				Risk:           "Third-party API availability",
				Level:          RiskMedium,
				Mitigation:     "Implement circuit breaker and fallback mechanisms",
			})
			if dep.Critical {
				risks = append(risks, DependencyRisk{
					DependencyName: dep.Name,
					Risk:           "Critical path dependency on external service",
					Level:          RiskHigh,
					Mitigation:     "Consider caching, retry logic, and graceful degradation",
				})
			}
		case DepTypeData:
			risks = append(risks, DependencyRisk{
				DependencyName: dep.Name,
				Risk:           "Data layer availability and performance",
				Level:          RiskHigh,
				Mitigation:     "Implement connection pooling, read replicas, and backup strategy",
			})
		case DepTypeNetwork:
			risks = append(risks, DependencyRisk{
				DependencyName: dep.Name,
				Risk:           "Network reliability",
				Level:          RiskMedium,
				Mitigation:     "Implement retry logic and dead letter queues",
			})
		}
	}

	// Check for single points of failure
	criticalCount := 0
	for _, dep := range deps {
		if dep.Critical {
			criticalCount++
		}
	}
	if criticalCount > 2 {
		risks = append(risks, DependencyRisk{
			Risk:       "Multiple critical dependencies increase failure risk",
			Level:      RiskHigh,
			Mitigation: "Consider reducing critical path dependencies or implementing redundancy",
		})
	}

	return risks
}

func (t *Tracer) checkLicenses(spec *specs.Spec) []LicenseIssue {
	var issues []LicenseIssue
	text := strings.ToLower(strings.Join(spec.Requirements, " "))

	// Known problematic license patterns (simplified)
	if strings.Contains(text, "gpl") && !strings.Contains(text, "lgpl") {
		issues = append(issues, LicenseIssue{
			Package: "GPL-licensed dependency",
			License: "GPL",
			Concern: "GPL requires derivative works to be open source",
		})
	}

	return issues
}

func (t *Tracer) identifyCriticalPath(deps []Dependency) []string {
	var critical []string
	for _, dep := range deps {
		if dep.Critical {
			critical = append(critical, dep.Name)
		}
	}
	return critical
}

func (t *Tracer) calculateOverallRisk(risks []DependencyRisk) RiskLevel {
	criticalCount := 0
	highCount := 0

	for _, r := range risks {
		switch r.Level {
		case RiskCritical:
			criticalCount++
		case RiskHigh:
			highCount++
		}
	}

	if criticalCount > 0 {
		return RiskCritical
	}
	if highCount > 2 {
		return RiskHigh
	}
	if highCount > 0 {
		return RiskMedium
	}
	return RiskLow
}

func (t *Tracer) generateRecommendations(dm *DependencyMap) []string {
	var recs []string

	if len(dm.CriticalPath) > 3 {
		recs = append(recs, "Consider reducing the number of critical path dependencies")
	}

	externalCount := 0
	for _, dep := range dm.Dependencies {
		if dep.Type == DepTypeExternal {
			externalCount++
		}
	}
	if externalCount > 2 {
		recs = append(recs, "Multiple external dependencies detected - implement unified error handling")
	}

	if dm.OverallRisk == RiskHigh || dm.OverallRisk == RiskCritical {
		recs = append(recs, "High risk profile - prioritize reliability testing and monitoring")
	}

	if len(dm.LicenseIssues) > 0 {
		recs = append(recs, "Review license compatibility before production deployment")
	}

	return recs
}

func hasDependencyType(deps []Dependency, depType DependencyType) bool {
	for _, d := range deps {
		if d.Type == depType {
			return true
		}
	}
	return false
}

// FormatDependencyMap formats a dependency map as readable text
func FormatDependencyMap(dm *DependencyMap) string {
	var sb strings.Builder

	sb.WriteString("# Dependency Analysis Report\n\n")
	sb.WriteString(fmt.Sprintf("**Spec:** %s\n", dm.SpecID))
	sb.WriteString(fmt.Sprintf("**Overall Risk Level:** %s\n\n", riskIcon(dm.OverallRisk)))

	if len(dm.Dependencies) > 0 {
		sb.WriteString("## Dependencies\n\n")
		for _, dep := range dm.Dependencies {
			criticalMark := ""
			if dep.Critical {
				criticalMark = " ⚠️"
			}
			sb.WriteString(fmt.Sprintf("- **%s** (%s)%s\n", dep.Name, dep.Type, criticalMark))
			sb.WriteString(fmt.Sprintf("  %s\n", dep.Description))
		}
		sb.WriteString("\n")
	}

	if len(dm.CriticalPath) > 0 {
		sb.WriteString("## Critical Path\n\n")
		for _, cp := range dm.CriticalPath {
			sb.WriteString(fmt.Sprintf("- %s\n", cp))
		}
		sb.WriteString("\n")
	}

	if len(dm.Risks) > 0 {
		sb.WriteString("## Risks\n\n")
		for _, risk := range dm.Risks {
			sb.WriteString(fmt.Sprintf("### %s %s\n", riskIcon(risk.Level), risk.Risk))
			if risk.DependencyName != "" {
				sb.WriteString(fmt.Sprintf("*Dependency:* %s\n", risk.DependencyName))
			}
			sb.WriteString(fmt.Sprintf("*Mitigation:* %s\n\n", risk.Mitigation))
		}
	}

	if len(dm.LicenseIssues) > 0 {
		sb.WriteString("## License Issues\n\n")
		for _, issue := range dm.LicenseIssues {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): %s\n", issue.Package, issue.License, issue.Concern))
		}
		sb.WriteString("\n")
	}

	if len(dm.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, rec := range dm.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", rec))
		}
	}

	return sb.String()
}

// BuildTracerBrief generates an agent brief for dependency analysis
func BuildTracerBrief(spec *specs.Spec) string {
	var sb strings.Builder

	sb.WriteString("# Tracer Agent Brief\n\n")
	sb.WriteString("## Mission\n")
	sb.WriteString("Map technical dependencies and assess risk terrain for the given specification.\n\n")

	sb.WriteString("## Spec Context\n")
	sb.WriteString(fmt.Sprintf("- **ID:** %s\n", spec.ID))
	sb.WriteString(fmt.Sprintf("- **Title:** %s\n", spec.Title))
	sb.WriteString(fmt.Sprintf("- **Summary:** %s\n\n", spec.Summary))

	sb.WriteString("## Analysis Scope\n")
	sb.WriteString("1. **External Dependencies** - Third-party APIs, SaaS integrations\n")
	sb.WriteString("2. **Data Dependencies** - Databases, storage, caching\n")
	sb.WriteString("3. **Network Dependencies** - Message queues, service mesh\n")
	sb.WriteString("4. **Library Dependencies** - Package/module dependencies\n\n")

	sb.WriteString("## Risk Assessment Criteria\n")
	sb.WriteString("- Critical path analysis\n")
	sb.WriteString("- Single points of failure\n")
	sb.WriteString("- License compatibility\n")
	sb.WriteString("- SLA and availability concerns\n")

	return sb.String()
}

func riskIcon(level RiskLevel) string {
	switch level {
	case RiskCritical:
		return "🔴 Critical"
	case RiskHigh:
		return "🟠 High"
	case RiskMedium:
		return "🟡 Medium"
	case RiskLow:
		return "🟢 Low"
	default:
		return "⚪ Unknown"
	}
}

// Regular expression for API patterns
var apiEndpointPattern = regexp.MustCompile(`(?i)(GET|POST|PUT|DELETE|PATCH)\s+/api/`)
