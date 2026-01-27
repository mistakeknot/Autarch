package arbiter

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ProjectContext holds detected project metadata for draft generation.
type ProjectContext struct {
	HasReadme      bool
	ReadmeSnippet  string
	HasPackageJSON bool
	PackageName    string
	Dependencies   []string
	MainFiles      []string
}

// Generator produces section drafts for the propose-first flow.
type Generator struct{}

// NewGenerator creates a new Generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateDraft produces a SectionDraft for the given phase using available context.
func (g *Generator) GenerateDraft(_ context.Context, phase Phase, projectCtx *ProjectContext, userInput string) (*SectionDraft, error) {
	var content string
	var options []string

	switch phase {
	case PhaseVision:
		content, options = g.generateVision(projectCtx, userInput)
	case PhaseProblem:
		content, options = g.generateProblem(projectCtx, userInput)
	case PhaseUsers:
		content, options = g.generateUsers(projectCtx, userInput)
	case PhaseFeaturesGoals:
		content, options = g.generateFeaturesGoals(projectCtx, userInput)
	case PhaseRequirements:
		content, options = g.generateRequirements(projectCtx, userInput)
	case PhaseScopeAssumptions:
		content, options = g.generateScopeAssumptions(projectCtx, userInput)
	case PhaseCUJs:
		content, options = g.generateCUJs(projectCtx, userInput)
	case PhaseAcceptanceCriteria:
		content, options = g.generateAcceptanceCriteria(projectCtx, userInput)
	default:
		return nil, fmt.Errorf("unknown phase: %d", phase)
	}

	return &SectionDraft{
		Content:   content,
		Options:   options,
		Status:    DraftProposed,
		UpdatedAt: time.Now(),
	}, nil
}

func (g *Generator) generateProblem(projectCtx *ProjectContext, userInput string) (string, []string) {
	if userInput != "" {
		base := g.draftFromInput(userInput, "Problem")
		return base, g.problemOptions(base)
	}
	if projectCtx != nil {
		base := g.draftFromContext(projectCtx, "Problem")
		return base, g.problemOptions(base)
	}
	base := "## Problem\n\n[Describe the core problem your product solves. Who experiences it? How often? What's the cost of not solving it?]"
	return base, g.problemOptions(base)
}

func (g *Generator) generateUsers(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Target Users\n\n**Primary:** %s\n\n**Demographics:** [Age range, technical skill level, domain]\n\n**Workflow:** [How they currently solve this problem]", userInput)
	} else {
		base = "## Target Users\n\n**Primary:** [Who is the main user?]\n\n**Demographics:** [Age range, technical skill level, domain]\n\n**Workflow:** [How they currently solve this problem]"
	}
	return base, []string{
		"Focus on demographics and psychographics",
		"Focus on skill level and technical background",
		"Focus on current workflow and pain points",
	}
}

func (g *Generator) generateFeaturesGoals(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Features\n\n1. %s\n2. [Feature 2]\n3. [Feature 3]\n\n## Goals\n\n- [Measurable outcome 1]\n- [Measurable outcome 2]", userInput)
	} else {
		base = "## Features\n\n1. [Core feature]\n2. [Supporting feature]\n3. [Nice-to-have feature]\n\n## Goals\n\n- [Measurable outcome 1]\n- [Measurable outcome 2]"
	}
	return base, []string{
		"Prioritize by user impact",
		"Prioritize by implementation effort",
		"Prioritize by business value",
	}
}

func (g *Generator) generateScopeAssumptions(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## In Scope\n\n- %s\n\n## Out of Scope\n\n- [Explicitly excluded]\n\n## Assumptions\n\n- [Key assumption 1]\n- [Key assumption 2]", userInput)
	} else {
		base = "## In Scope\n\n- [What's included in v1]\n\n## Out of Scope\n\n- [Explicitly excluded from v1]\n\n## Assumptions\n\n- [Key assumption 1]\n- [Key assumption 2]"
	}
	return base, []string{
		"Aggressive scope (MVP only)",
		"Moderate scope (core + key differentiator)",
	}
}

func (g *Generator) generateCUJs(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Critical User Journeys\n\n### Journey 1: %s\n\n1. User opens the app\n2. [Step 2]\n3. [Step 3]\n4. User achieves their goal", userInput)
	} else {
		base = "## Critical User Journeys\n\n### Journey 1: [Primary task]\n\n1. User opens the app\n2. [Step 2]\n3. [Step 3]\n4. User achieves their goal"
	}
	return base, []string{
		"Happy path focus",
		"Include error/edge cases",
		"Include onboarding journey",
	}
}

func (g *Generator) generateAcceptanceCriteria(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Acceptance Criteria\n\n- [ ] %s\n- [ ] [Testable criterion 2]\n- [ ] [Testable criterion 3]\n- [ ] Performance: [metric] under [threshold]", userInput)
	} else {
		base = "## Acceptance Criteria\n\n- [ ] [Testable criterion 1]\n- [ ] [Testable criterion 2]\n- [ ] [Testable criterion 3]\n- [ ] Performance: [metric] under [threshold]"
	}
	return base, []string{
		"Functional criteria only",
		"Include non-functional (performance, accessibility)",
	}
}

func (g *Generator) generateVision(projectCtx *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Vision\n\n%s", userInput)
	} else if projectCtx != nil && projectCtx.HasReadme && projectCtx.ReadmeSnippet != "" {
		base = fmt.Sprintf("## Vision\n\n%s", projectCtx.ReadmeSnippet)
	} else {
		base = "## Vision\n\n[What does the world look like when this product succeeds? One paragraph describing the future state.]"
	}
	return base, []string{
		"User-centric vision (focus on user outcomes)",
		"Market-centric vision (focus on market position)",
		"Technical vision (focus on capabilities enabled)",
	}
}

func (g *Generator) generateRequirements(_ *ProjectContext, userInput string) (string, []string) {
	var base string
	if userInput != "" {
		base = fmt.Sprintf("## Requirements\n\n- %s\n- [Requirement 2]\n- [Requirement 3]", userInput)
	} else {
		base = "## Requirements\n\n- [Functional requirement 1]\n- [Functional requirement 2]\n- [Non-functional requirement 1]"
	}
	return base, []string{
		"Functional requirements only",
		"Include non-functional (performance, security, scale)",
		"Prioritized MoSCoW style (must/should/could/won't)",
	}
}

func (g *Generator) draftFromInput(input, section string) string {
	// Extract reason after "because" if present
	if idx := strings.Index(strings.ToLower(input), "because"); idx != -1 {
		reason := strings.TrimSpace(input[idx+len("because"):])
		return fmt.Sprintf("## %s\n\n%s\n\n**Root cause:** %s", section, input, reason)
	}
	return fmt.Sprintf("## %s\n\n%s", section, input)
}

func (g *Generator) draftFromContext(projectCtx *ProjectContext, section string) string {
	if projectCtx == nil {
		return "[Could not infer from context]"
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("## %s", section))
	if projectCtx.HasReadme && projectCtx.ReadmeSnippet != "" {
		parts = append(parts, fmt.Sprintf("Based on project description: %s", projectCtx.ReadmeSnippet))
	}
	if projectCtx.HasPackageJSON && projectCtx.PackageName != "" {
		parts = append(parts, fmt.Sprintf("Package: %s", projectCtx.PackageName))
	}
	parts = append(parts, "\n[Refine the problem statement above]")
	return strings.Join(parts, "\n\n")
}

func (g *Generator) problemOptions(base string) []string {
	return []string{
		base + "\n\n_Emphasis: user pain point_",
		base + "\n\n_Emphasis: market opportunity_",
		base + "\n\n_Emphasis: technical gap_",
	}
}
