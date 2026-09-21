// Package plan provides plan generation for Praude's plan/apply pattern.
package plan

import (
	"time"

	"github.com/mistakeknot/autarch/pkg/discovery"
	"github.com/mistakeknot/autarch/pkg/plan"
)

// InterviewPlanItems contains the items for an interview plan.
type InterviewPlanItems struct {
	PRD        PRDSummary `json:"prd"`
	Validation Validation `json:"validation"`
}

// PRDSummary contains the PRD that will be created.
type PRDSummary struct {
	ID                      string   `json:"id"`
	Title                   string   `json:"title"`
	Vision                  string   `json:"vision"`
	Users                   string   `json:"users"`
	Problem                 string   `json:"problem"`
	Requirements            []string `json:"requirements"`
	AcceptanceCriteriaCount int      `json:"acceptance_criteria_count"`
}

// Validation contains validation results.
type Validation struct {
	Passed   bool     `json:"passed"`
	Warnings []string `json:"warnings,omitempty"`
}

// InterviewPlanOptions contains inputs for generating an interview plan.
type InterviewPlanOptions struct {
	Root         string
	NextID       string
	Vision       string
	Users        string
	Problem      string
	Requirements []string
}

// GenerateInterviewPlan creates a plan for the interview command.
func GenerateInterviewPlan(opts InterviewPlanOptions) (*plan.Plan, error) {
	p := plan.NewPlan("praude", "interview")

	// Build PRD summary
	reqCount := len(opts.Requirements)
	if reqCount == 0 {
		reqCount = 1 // Default REQ-001: TBD
	}

	title := opts.Vision
	if title == "" {
		title = opts.Problem
	}
	if title == "" {
		title = "New PRD"
	}

	prdSummary := PRDSummary{
		ID:                      opts.NextID,
		Title:                   title,
		Vision:                  opts.Vision,
		Users:                   opts.Users,
		Problem:                 opts.Problem,
		Requirements:            opts.Requirements,
		AcceptanceCriteriaCount: reqCount, // Initial estimate
	}

	// Validate inputs
	validation := Validation{Passed: true}
	if opts.Vision == "" {
		validation.Warnings = append(validation.Warnings, "Vision statement is empty")
	}
	if opts.Users == "" {
		validation.Warnings = append(validation.Warnings, "Target users not specified")
	}
	if opts.Problem == "" {
		validation.Warnings = append(validation.Warnings, "Problem statement is empty")
	}
	if len(opts.Requirements) == 0 {
		validation.Warnings = append(validation.Warnings, "No requirements specified, will use placeholder")
	}
	if len(validation.Warnings) > 0 {
		validation.Passed = false
	}

	items := InterviewPlanItems{
		PRD:        prdSummary,
		Validation: validation,
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	// Generate summary
	p.Summary = "Create " + opts.NextID + ": " + title
	if reqCount > 0 {
		p.Summary += " with " + itoa(reqCount) + " requirement(s)"
	}

	// Add validation recommendations
	for _, w := range validation.Warnings {
		p.AddRecommendation(plan.Recommendation{
			Type:     plan.TypeValidation,
			Severity: plan.SeverityWarning,
			Message:  w,
		})
	}

	// Check for Pollard integration
	addPollardRecommendations(p, opts.Root)

	return p, nil
}

// addPollardRecommendations adds cross-tool recommendations from Pollard.
func addPollardRecommendations(p *plan.Plan, root string) {
	if !discovery.ToolExists(root, "pollard") {
		// Suggest running Pollard if not initialized
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeIntegration,
			Severity:   plan.SeverityInfo,
			SourceTool: "pollard",
			Message:    "Pollard not initialized - consider running research first",
			Suggestion: "pollard init && pollard scan",
		})
		return
	}

	// Check for available insights
	insightCount := discovery.CountPollardInsights(root)
	if insightCount > 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeIntegration,
			Severity:   plan.SeverityInfo,
			SourceTool: "pollard",
			Field:      "market_research",
			Message:    itoa(insightCount) + " insight(s) available from Pollard",
			Suggestion: "praude import-research --from-pollard",
			Context: map[string]interface{}{
				"insights_count": insightCount,
			},
			AutoFixable: true,
		})
	} else if discovery.PollardHasData(root) {
		// Has sources but no insights
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeIntegration,
			Severity:   plan.SeverityInfo,
			SourceTool: "pollard",
			Message:    "Pollard has sources but no synthesized insights",
			Suggestion: "pollard report to generate insights",
		})
	} else {
		// Pollard exists but empty
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypePrereq,
			Severity:   plan.SeverityWarning,
			SourceTool: "pollard",
			Message:    "No market research - consider running Pollard first",
			Suggestion: "pollard scan",
		})
	}

	// Check for recent insights (last 7 days)
	recent, _ := discovery.RecentPollardInsights(root, 7)
	if len(recent) == 0 && insightCount > 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeQuality,
			Severity:   plan.SeverityInfo,
			Message:    "No insights from the last 7 days - research may be stale",
			Suggestion: "pollard scan to refresh",
		})
	}
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

// ApplyInterviewPlan applies a saved interview plan.
func ApplyInterviewPlan(p *plan.Plan, root string) (path string, id string, warnings []string, err error) {
	var items InterviewPlanItems
	if err := p.GetItems(&items); err != nil {
		return "", "", nil, err
	}

	// The actual PRD creation is delegated back to the interview command
	// This function is called by the apply command to extract the plan data
	return "", items.PRD.ID, items.Validation.Warnings, nil
}

// ResearchPlanItems contains the items for a research plan.
type ResearchPlanItems struct {
	SpecID       string    `json:"spec_id"`
	ResearchPath string    `json:"research_path"`
	BriefPath    string    `json:"brief_path"`
	CreatedAt    time.Time `json:"created_at"`
}

// GenerateResearchPlan creates a plan for the research command.
func GenerateResearchPlan(root, specID string) (*plan.Plan, error) {
	p := plan.NewPlan("praude", "research")

	items := ResearchPlanItems{
		SpecID:    specID,
		CreatedAt: time.Now(),
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	p.Summary = "Run research for " + specID

	// Check if spec exists
	spec, err := discovery.FindSpec(root, specID)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		p.AddRecommendation(plan.Recommendation{
			Type:     plan.TypeValidation,
			Severity: plan.SeverityError,
			Message:  "Spec " + specID + " not found",
		})
	}

	return p, nil
}
