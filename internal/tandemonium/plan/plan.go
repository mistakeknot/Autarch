package plan

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/pkg/discovery"
	"github.com/mistakeknot/vauxpraudemonium/pkg/plan"
)

func Run(in io.Reader, out io.Writer, planDir string) error {
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return err
	}
	if out == nil {
		out = io.Discard
	}
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
		return nil
	}
	vision := ""
	mvp := ""
	fmt.Fprintln(out, "Vision (leave blank to skip):")
	if scanner.Scan() {
		vision = scanner.Text()
	}
	fmt.Fprintln(out, "MVP (leave blank to skip):")
	if scanner.Scan() {
		mvp = scanner.Text()
	}
	if err := os.WriteFile(filepath.Join(planDir, "vision.md"), []byte(vision+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(planDir, "mvp.md"), []byte(mvp+"\n"), 0o644); err != nil {
		return err
	}
	return nil
}

// InitPlanItems contains the items for an init --from-prd plan.
type InitPlanItems struct {
	SourcePRD string      `json:"source_prd"`
	Epics     []EpicPlan  `json:"epics"`
}

// EpicPlan describes an epic that will be created.
type EpicPlan struct {
	ID      string      `json:"id"`
	Title   string      `json:"title"`
	Status  string      `json:"status"`
	Stories []StoryPlan `json:"stories"`
}

// StoryPlan describes a story within an epic.
type StoryPlan struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
}

// InitPlanOptions contains inputs for generating an init plan.
type InitPlanOptions struct {
	Root     string
	PRDID    string
	Epics    []EpicPlan
	Warnings []string
}

// GenerateInitPlan creates a plan for the init --from-prd command.
func GenerateInitPlan(opts InitPlanOptions) (*plan.Plan, error) {
	p := plan.NewPlan("tandemonium", "init")

	items := InitPlanItems{
		SourcePRD: opts.PRDID,
		Epics:     opts.Epics,
	}

	if err := p.SetItems(items); err != nil {
		return nil, err
	}

	// Count totals
	storyCount := 0
	for _, e := range opts.Epics {
		storyCount += len(e.Stories)
	}

	p.Summary = "Generate " + itoa(len(opts.Epics)) + " epic(s) with " + itoa(storyCount) + " stories from " + opts.PRDID

	// Add warnings from import
	for _, w := range opts.Warnings {
		p.AddRecommendation(plan.Recommendation{
			Type:     plan.TypeValidation,
			Severity: plan.SeverityWarning,
			Message:  w,
		})
	}

	// Check for quality issues
	for _, e := range opts.Epics {
		if len(e.Stories) == 0 {
			p.AddRecommendation(plan.Recommendation{
				Type:     plan.TypeQuality,
				Severity: plan.SeverityWarning,
				Field:    e.ID,
				Message:  e.ID + " has no stories",
			})
		}
		if len(e.Stories) > 5 {
			p.AddRecommendation(plan.Recommendation{
				Type:     plan.TypeEnhancement,
				Severity: plan.SeverityInfo,
				Field:    e.ID,
				Message:  "Consider splitting " + e.ID + " - " + itoa(len(e.Stories)) + " stories is large",
			})
		}
	}

	// Check for Praude/Pollard integration
	addCrossToolRecommendations(p, opts.Root)

	return p, nil
}

func addCrossToolRecommendations(p *plan.Plan, root string) {
	// Check for Pollard insights
	if discovery.ToolExists(root, "pollard") {
		insightCount := discovery.CountPollardInsights(root)
		if insightCount > 0 {
			p.AddRecommendation(plan.Recommendation{
				Type:       plan.TypeIntegration,
				Severity:   plan.SeverityInfo,
				SourceTool: "pollard",
				Message:    itoa(insightCount) + " research insight(s) available",
				Suggestion: "Consider linking insights to epics for context",
			})
		}

		// Check for Pollard patterns
		addPollardPatternRecommendations(p, root)
	}

	// Check for blocked epics from existing data
	if discovery.TandemoniumHasData(root) {
		blocked, _ := discovery.BlockedEpics(root)
		if len(blocked) > 0 {
			p.AddRecommendation(plan.Recommendation{
				Type:     plan.TypePrereq,
				Severity: plan.SeverityWarning,
				Message:  itoa(len(blocked)) + " existing epic(s) are blocked",
			})
		}
	}
}

// addPollardPatternRecommendations adds recommendations based on Pollard patterns.
func addPollardPatternRecommendations(p *plan.Plan, root string) {
	patternCount := discovery.CountPollardPatterns(root)
	if patternCount == 0 {
		return
	}

	// Check for anti-patterns specifically
	antiPatterns, err := discovery.PollardAntiPatterns(root)
	if err != nil {
		return
	}

	if len(antiPatterns) > 0 {
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeQuality,
			Severity:   plan.SeverityWarning,
			SourceTool: "pollard",
			Message:    itoa(len(antiPatterns)) + " anti-pattern(s) may apply to this implementation",
			Suggestion: "Review patterns before implementation to avoid known pitfalls",
		})
	}

	// Add general pattern info
	if patternCount > len(antiPatterns) {
		implPatterns := patternCount - len(antiPatterns)
		p.AddRecommendation(plan.Recommendation{
			Type:       plan.TypeIntegration,
			Severity:   plan.SeverityInfo,
			SourceTool: "pollard",
			Message:    itoa(implPatterns) + " implementation pattern(s) available",
			Suggestion: "See .pollard/patterns/ for architecture and UX patterns",
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
