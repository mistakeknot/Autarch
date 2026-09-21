package epics

import (
	"fmt"
	"strings"
)

// GeneratorInput contains the information needed to generate epics.
type GeneratorInput struct {
	Vision       string
	Problem      string
	Users        string
	Platform     string
	Language     string
	Requirements []string
}

// EpicProposal represents a proposed epic with estimates.
type EpicProposal struct {
	ID           string          `yaml:"id"`
	Title        string          `yaml:"title"`
	Description  string          `yaml:"description"`
	Size         Size            `yaml:"size"` // S, M, L
	Priority     Priority        `yaml:"priority"`
	Dependencies []string        `yaml:"dependencies,omitempty"`
	TaskCount    int             `yaml:"task_count"`
	Stories      []StoryProposal `yaml:"stories,omitempty"`
	Edited       bool            `yaml:"-"` // User has modified
}

// StoryProposal represents a proposed story within an epic.
type StoryProposal struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Size        Size   `yaml:"size"`
}

// Size represents epic/story size estimates.
type Size string

const (
	SizeSmall  Size = "S"
	SizeMedium Size = "M"
	SizeLarge  Size = "L"
	SizeXLarge Size = "XL"
)

// Generator creates epic proposals from spec input.
type Generator struct{}

// NewGenerator creates a new epic generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate creates epic proposals from the input specification.
func (g *Generator) Generate(input GeneratorInput) ([]EpicProposal, error) {
	if len(input.Requirements) == 0 {
		return nil, fmt.Errorf("no requirements provided")
	}

	var proposals []EpicProposal

	// Group requirements into logical epics
	groups := g.groupRequirements(input.Requirements)

	epicNum := 1
	for groupName, reqs := range groups {
		epic := g.createEpicFromGroup(epicNum, groupName, reqs, input)
		proposals = append(proposals, epic)
		epicNum++
	}

	// Add dependencies based on platform and architecture
	g.addDependencies(proposals, input)

	return proposals, nil
}

// groupRequirements clusters requirements into logical groups.
func (g *Generator) groupRequirements(requirements []string) map[string][]string {
	groups := make(map[string][]string)

	// Simple keyword-based grouping
	for _, req := range requirements {
		reqLower := strings.ToLower(req)
		group := g.categorizeRequirement(reqLower)
		groups[group] = append(groups[group], req)
	}

	// If all in one group, try to split by count
	if len(groups) == 1 && len(requirements) > 4 {
		groups = g.splitByCount(requirements)
	}

	return groups
}

// categorizeRequirement assigns a requirement to a category.
func (g *Generator) categorizeRequirement(req string) string {
	categories := map[string][]string{
		"Authentication & Security": {"auth", "login", "password", "security", "permission", "role", "user"},
		"Data Management":           {"database", "storage", "data", "persist", "save", "load", "import", "export"},
		"User Interface":            {"ui", "interface", "display", "view", "screen", "layout", "design", "style"},
		"API & Integration":         {"api", "endpoint", "rest", "graphql", "integrate", "connect", "webhook"},
		"Core Features":             {"feature", "function", "ability", "support", "enable", "process"},
		"Performance & Scaling":     {"performance", "scale", "cache", "optimize", "speed", "fast"},
		"Testing & Quality":         {"test", "quality", "validate", "verify", "check"},
		"Infrastructure":            {"deploy", "server", "cloud", "infrastructure", "ci", "cd"},
	}

	for category, keywords := range categories {
		for _, kw := range keywords {
			if strings.Contains(req, kw) {
				return category
			}
		}
	}

	return "Core Features"
}

// splitByCount divides requirements into roughly equal groups.
func (g *Generator) splitByCount(requirements []string) map[string][]string {
	groups := make(map[string][]string)
	groupNames := []string{"Foundation", "Core Features", "Enhancement"}

	perGroup := (len(requirements) + len(groupNames) - 1) / len(groupNames)

	for i, req := range requirements {
		groupIdx := i / perGroup
		if groupIdx >= len(groupNames) {
			groupIdx = len(groupNames) - 1
		}
		groupName := groupNames[groupIdx]
		groups[groupName] = append(groups[groupName], req)
	}

	return groups
}

// createEpicFromGroup builds an epic proposal from a group of requirements.
func (g *Generator) createEpicFromGroup(num int, groupName string, reqs []string, input GeneratorInput) EpicProposal {
	epicID := fmt.Sprintf("EPIC-%03d", num)

	// Estimate size based on requirement count
	var size Size
	switch {
	case len(reqs) <= 2:
		size = SizeSmall
	case len(reqs) <= 4:
		size = SizeMedium
	case len(reqs) <= 6:
		size = SizeLarge
	default:
		size = SizeXLarge
	}

	// Estimate task count
	taskCount := len(reqs) * 3 // Rough estimate: 3 tasks per requirement

	// Create stories for each requirement
	var stories []StoryProposal
	for i, req := range reqs {
		storyID := fmt.Sprintf("%s-STORY-%02d", epicID, i+1)
		stories = append(stories, StoryProposal{
			ID:          storyID,
			Title:       g.extractTitle(req),
			Description: req,
			Size:        g.estimateStorySize(req),
		})
	}

	// Determine priority based on group type
	priority := g.determinePriority(groupName)

	// Build description
	description := fmt.Sprintf("Epic for %s functionality.\n\nRequirements:\n", groupName)
	for _, req := range reqs {
		description += fmt.Sprintf("- %s\n", req)
	}
	if input.Platform != "" {
		description += fmt.Sprintf("\nTarget platform: %s", input.Platform)
	}

	return EpicProposal{
		ID:          epicID,
		Title:       groupName,
		Description: description,
		Size:        size,
		Priority:    priority,
		TaskCount:   taskCount,
		Stories:     stories,
	}
}

// extractTitle creates a short title from a requirement.
func (g *Generator) extractTitle(req string) string {
	// Remove common prefixes
	req = strings.TrimPrefix(req, "REQ-")
	for i := 0; i <= 9; i++ {
		req = strings.TrimPrefix(req, fmt.Sprintf("%d:", i))
		req = strings.TrimPrefix(req, fmt.Sprintf("%d.", i))
		req = strings.TrimPrefix(req, fmt.Sprintf("%d)", i))
	}
	req = strings.TrimSpace(req)

	// Truncate if too long
	if len(req) > 50 {
		req = req[:47] + "..."
	}

	return req
}

// estimateStorySize estimates size from requirement complexity.
func (g *Generator) estimateStorySize(req string) Size {
	words := len(strings.Fields(req))
	switch {
	case words <= 5:
		return SizeSmall
	case words <= 10:
		return SizeMedium
	default:
		return SizeLarge
	}
}

// determinePriority maps group names to priorities.
func (g *Generator) determinePriority(groupName string) Priority {
	highPriority := []string{"Authentication", "Security", "Core", "Foundation"}
	mediumPriority := []string{"Data", "API", "Interface"}

	for _, hp := range highPriority {
		if strings.Contains(groupName, hp) {
			return PriorityP1
		}
	}
	for _, mp := range mediumPriority {
		if strings.Contains(groupName, mp) {
			return PriorityP2
		}
	}
	return PriorityP3
}

// addDependencies establishes dependencies between epics.
func (g *Generator) addDependencies(proposals []EpicProposal, input GeneratorInput) {
	// Find foundation/auth epics - these should come first
	var foundationIdx, authIdx int = -1, -1
	for i, p := range proposals {
		titleLower := strings.ToLower(p.Title)
		if strings.Contains(titleLower, "foundation") || strings.Contains(titleLower, "infrastructure") {
			foundationIdx = i
		}
		if strings.Contains(titleLower, "auth") || strings.Contains(titleLower, "security") {
			authIdx = i
		}
	}

	// Make other epics depend on foundation/auth
	for i := range proposals {
		if i == foundationIdx || i == authIdx {
			continue
		}

		if foundationIdx >= 0 {
			proposals[i].Dependencies = append(proposals[i].Dependencies, proposals[foundationIdx].ID)
		}
		if authIdx >= 0 && authIdx != foundationIdx {
			proposals[i].Dependencies = append(proposals[i].Dependencies, proposals[authIdx].ID)
		}
	}
}

// ConvertToEpics converts proposals to actual Epic structs.
func ConvertToEpics(proposals []EpicProposal) []Epic {
	var epics []Epic
	for _, p := range proposals {
		var stories []Story
		for _, sp := range p.Stories {
			stories = append(stories, Story{
				ID:       sp.ID,
				Title:    sp.Title,
				Summary:  sp.Description,
				Status:   StatusTodo,
				Priority: p.Priority,
			})
		}

		epics = append(epics, Epic{
			ID:       p.ID,
			Title:    p.Title,
			Summary:  p.Description,
			Status:   StatusTodo,
			Priority: p.Priority,
			Stories:  stories,
		})
	}
	return epics
}

// EstimateTotalTasks returns the sum of all task counts.
func EstimateTotalTasks(proposals []EpicProposal) int {
	total := 0
	for _, p := range proposals {
		total += p.TaskCount
	}
	return total
}

// GenerateFromDescription creates epics from a simple project description.
// This is used during onboarding when we only have a description, not a full spec.
func (g *Generator) GenerateFromDescription(description string) []EpicProposal {
	// Parse the description to extract potential requirements
	requirements := g.extractRequirementsFromDescription(description)

	if len(requirements) == 0 {
		// Create a single default epic from the description
		return []EpicProposal{
			{
				ID:          "EPIC-001",
				Title:       "Core Implementation",
				Description: description,
				Size:        SizeMedium,
				Priority:    PriorityP1,
				TaskCount:   5,
			},
		}
	}

	input := GeneratorInput{
		Vision:       description,
		Requirements: requirements,
	}

	proposals, err := g.Generate(input)
	if err != nil || len(proposals) == 0 {
		// Fallback to single epic
		return []EpicProposal{
			{
				ID:          "EPIC-001",
				Title:       "Core Implementation",
				Description: description,
				Size:        SizeMedium,
				Priority:    PriorityP1,
				TaskCount:   5,
			},
		}
	}

	return proposals
}

// extractRequirementsFromDescription parses a description for requirements.
func (g *Generator) extractRequirementsFromDescription(description string) []string {
	var requirements []string

	// Look for bullet points, numbered lists, or "and" conjunctions
	lines := strings.Split(description, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check for bullet points
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
			req := strings.TrimPrefix(line, "-")
			req = strings.TrimPrefix(req, "*")
			req = strings.TrimPrefix(req, "•")
			req = strings.TrimSpace(req)
			if req != "" {
				requirements = append(requirements, req)
			}
			continue
		}

		// Check for numbered lists
		for i := 1; i <= 9; i++ {
			prefixes := []string{
				fmt.Sprintf("%d.", i),
				fmt.Sprintf("%d)", i),
				fmt.Sprintf("%d:", i),
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(line, prefix) {
					req := strings.TrimPrefix(line, prefix)
					req = strings.TrimSpace(req)
					if req != "" {
						requirements = append(requirements, req)
					}
					break
				}
			}
		}
	}

	// If no structured requirements found, try splitting by "and" or commas
	if len(requirements) == 0 && len(description) > 20 {
		// Split on common conjunctions
		parts := strings.Split(description, " and ")
		if len(parts) > 1 {
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if len(part) > 5 {
					requirements = append(requirements, part)
				}
			}
		}
	}

	return requirements
}
