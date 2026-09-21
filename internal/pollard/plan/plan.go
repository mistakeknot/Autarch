// Package plan provides plan generation for Pollard's plan/apply pattern.
package plan

import (
	"os"

	"github.com/mistakeknot/autarch/pkg/discovery"
	"github.com/mistakeknot/autarch/pkg/plan"
)

// ScanPlanItems contains the items for a scan plan.
type ScanPlanItems struct {
	Hunters []HunterPlan `json:"hunters"`
}

// HunterPlan describes what a hunter will do.
type HunterPlan struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Queries          []string `json:"queries,omitempty"`
	EstimatedResults string   `json:"estimated_results"`
	Enabled          bool     `json:"enabled"`
}

// ScanPlanOptions contains inputs for generating a scan plan.
type ScanPlanOptions struct {
	Root          string
	HunterNames   []string
	HunterConfigs map[string]HunterConfig
}

// HunterConfig contains hunter-specific configuration.
type HunterConfig struct {
	Queries    []string
	MaxResults int
	Interval   string
	Output     string
}

// GenerateScanPlan creates a plan for the scan command.
func GenerateScanPlan(opts ScanPlanOptions) (*plan.Plan, error) {
	p := plan.NewPlan("pollard", "scan")

	var hunters []HunterPlan
	for _, name := range opts.HunterNames {
		cfg := opts.HunterConfigs[name]
		hp := HunterPlan{
			ID:               name,
			Name:             name,
			Queries:          cfg.Queries,
			Enabled:          true,
			EstimatedResults: estimateResults(name, cfg),
		}
		hunters = append(hunters, hp)
	}

	items := ScanPlanItems{
		Hunters: hunters,
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	// Generate summary
	p.Summary = "Scan " + itoa(len(hunters)) + " hunter(s)"
	if len(hunters) > 0 {
		p.Summary += ": " + hunters[0].Name
		if len(hunters) > 1 {
			p.Summary += " (+" + itoa(len(hunters)-1) + " more)"
		}
	}

	// Check for environment recommendations
	addEnvRecommendations(p)

	// Check for Praude integration
	addPraudeRecommendations(p, opts.Root)

	return p, nil
}

func estimateResults(hunterName string, cfg HunterConfig) string {
	switch hunterName {
	case "github-scout":
		return "10-50 repos"
	case "openalex":
		return "100+ papers"
	case "hackernews":
		return "20-100 posts"
	case "arxiv":
		return "50-200 papers"
	case "pubmed":
		return "10-100 articles"
	default:
		return "varies"
	}
}

func addEnvRecommendations(p *plan.Plan) {
	// Check for GITHUB_TOKEN
	if os.Getenv("GITHUB_TOKEN") == "" {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypePrereq,
			Severity:   plan.SeverityInfo,
			Message:    "GITHUB_TOKEN not set - rate limited to 60 req/hour",
			Suggestion: "export GITHUB_TOKEN=your_token",
		})
	}
}

func addPraudeRecommendations(p *plan.Plan, root string) {
	if !discovery.ToolExists(root, "praude") {
		return
	}

	specCount := discovery.CountGurgSpecs(root)
	prdCount := discovery.CountGurgPRDs(root)

	if specCount > 0 || prdCount > 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeIntegration,
			Severity:   plan.SeverityInfo,
			SourceTool: "praude",
			Message:    itoa(specCount) + " spec(s) and " + itoa(prdCount) + " PRD(s) available",
			Suggestion: "Research can inform PRD market research sections",
		})
	}
}

// ReportPlanItems contains the items for a report plan.
type ReportPlanItems struct {
	ReportType      string         `json:"report_type"`
	SourcesByHunter map[string]int `json:"sources_by_hunter"`
	TotalSources    int            `json:"total_sources"`
}

// ReportPlanOptions contains inputs for generating a report plan.
type ReportPlanOptions struct {
	Root       string
	ReportType string
}

// GenerateReportPlan creates a plan for the report command.
func GenerateReportPlan(opts ReportPlanOptions) (*plan.Plan, error) {
	p := plan.NewPlan("pollard", "report")

	// Count sources by type
	sources, _ := discovery.PollardSources(opts.Root)
	byHunter := make(map[string]int)
	for _, s := range sources {
		byHunter[s.AgentName]++
	}

	total := 0
	for _, count := range byHunter {
		total += count
	}

	reportType := opts.ReportType
	if reportType == "" {
		reportType = "landscape"
	}

	items := ReportPlanItems{
		ReportType:      reportType,
		SourcesByHunter: byHunter,
		TotalSources:    total,
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	p.Summary = "Generate " + reportType + " report from " + itoa(total) + " source(s)"

	// Add recommendations
	if total == 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypePrereq,
			Severity:   plan.SeverityWarning,
			Message:    "No sources collected - run scan first",
			Suggestion: "pollard scan",
		})
	}

	// Check for stale data
	recent, _ := discovery.RecentPollardInsights(opts.Root, 7)
	if len(recent) == 0 && total > 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeQuality,
			Severity:   plan.SeverityWarning,
			Message:    "No sources from last 7 days - data may be stale",
			Suggestion: "pollard scan to refresh",
		})
	}

	return p, nil
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// =============================================================================
// Propose Plan Types
// =============================================================================

// ProposePlanItems contains the items for a propose plan.
type ProposePlanItems struct {
	ProjectName  string   `json:"project_name"`
	Technologies []string `json:"technologies,omitempty"`
	DetectedType string   `json:"detected_type,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	FilesFound   []string `json:"files_found"`
	MaxAgendas   int      `json:"max_agendas"`
	IncludeSrc   bool     `json:"include_src"`
}

// ProposePlanOptions contains inputs for generating a propose plan.
type ProposePlanOptions struct {
	Root         string
	ProjectName  string
	Technologies []string
	DetectedType string
	Domain       string
	FilesFound   []string
	MaxAgendas   int
	IncludeSrc   bool
}

// GenerateProposePlan creates a plan for the propose command.
func GenerateProposePlan(opts ProposePlanOptions) (*plan.Plan, error) {
	p := plan.NewPlan("pollard", "propose")

	items := ProposePlanItems{
		ProjectName:  opts.ProjectName,
		Technologies: opts.Technologies,
		DetectedType: opts.DetectedType,
		Domain:       opts.Domain,
		FilesFound:   opts.FilesFound,
		MaxAgendas:   opts.MaxAgendas,
		IncludeSrc:   opts.IncludeSrc,
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	// Generate summary
	p.Summary = "Propose " + itoa(opts.MaxAgendas) + " research agenda(s) for " + opts.ProjectName

	// Check for documentation
	if len(opts.FilesFound) == 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypePrereq,
			Severity:   plan.SeverityWarning,
			Message:    "No documentation files found (CLAUDE.md, AGENTS.md, README.md)",
			Suggestion: "Create at least a README.md to provide project context",
		})
	}

	// Check for agent configuration
	if os.Getenv("POLLARD_AGENT_COMMAND") == "" {
		p.AddRecommendation(plan.Recommendation{
			Type:     plan.TypePrereq,
			Severity: plan.SeverityInfo,
			Message:  "Using default agent 'claude' (set POLLARD_AGENT_COMMAND to override)",
		})
	}

	// Add technology-based recommendations
	addTechnologyRecommendations(p, opts.Technologies, opts.DetectedType)

	return p, nil
}

// addTechnologyRecommendations suggests hunters based on detected tech.
func addTechnologyRecommendations(p *plan.Plan, techs []string, projectType string) {
	techSet := make(map[string]bool)
	for _, t := range techs {
		techSet[t] = true
	}

	// AI/ML technologies suggest arxiv
	if techSet["PyTorch"] || techSet["TensorFlow"] || techSet["OpenAI"] {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeEnhancement,
			Severity:   plan.SeverityInfo,
			Message:    "AI/ML project detected - arxiv hunter recommended",
			Suggestion: "Research agendas may include academic paper review",
		})
	}

	// Web frameworks suggest competitor tracking
	if techSet["Next.js"] || techSet["React"] || techSet["Rails"] {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeEnhancement,
			Severity:   plan.SeverityInfo,
			Message:    "Web framework detected - competitor-tracker recommended",
			Suggestion: "Research agendas may include competitor analysis",
		})
	}

	// CLI tools suggest github-scout
	if projectType == "cli" || techSet["Cobra CLI"] {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeEnhancement,
			Severity:   plan.SeverityInfo,
			Message:    "CLI tool detected - github-scout recommended",
			Suggestion: "Research agendas may include open source implementations",
		})
	}
}
