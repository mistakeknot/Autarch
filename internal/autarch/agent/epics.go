package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/epics"
)

// SpecInput contains the spec information for epic generation
type SpecInput struct {
	Vision       string
	Users        string
	Problem      string
	Platform     string
	Language     string
	Requirements []string
}

// GenerateEpics uses the coding agent to generate epic proposals from a spec.
func GenerateEpics(ctx context.Context, agent *Agent, spec SpecInput) ([]epics.EpicProposal, error) {
	return GenerateEpicsWithOutput(ctx, agent, spec, nil)
}

// GenerateEpicsWithOutput streams agent output while generating epic proposals.
func GenerateEpicsWithOutput(ctx context.Context, agent *Agent, spec SpecInput, onOutput OutputCallback) ([]epics.EpicProposal, error) {
	prompt := buildEpicPrompt(spec)

	// Set a reasonable timeout for LLM generation
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resp, err := agent.GenerateWithOutput(ctx, GenerateRequest{
		Prompt: prompt,
	}, onOutput)
	if err != nil {
		return nil, fmt.Errorf("agent generation failed: %w", err)
	}

	// Parse the response into epic proposals
	proposals, err := parseEpicResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse epic response: %w", err)
	}

	return proposals, nil
}

func buildEpicPrompt(spec SpecInput) string {
	var sb strings.Builder

	sb.WriteString(`You are a software architect helping break down a project into epics.

Given the following project specification, generate a list of epics that cover all the necessary work.

PROJECT SPECIFICATION:
`)

	sb.WriteString(fmt.Sprintf("Vision: %s\n", spec.Vision))
	sb.WriteString(fmt.Sprintf("Target Users: %s\n", spec.Users))
	sb.WriteString(fmt.Sprintf("Problem Being Solved: %s\n", spec.Problem))
	sb.WriteString(fmt.Sprintf("Platform: %s\n", spec.Platform))
	sb.WriteString(fmt.Sprintf("Language: %s\n", spec.Language))

	sb.WriteString("\nRequirements:\n")
	for i, req := range spec.Requirements {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, req))
	}

	sb.WriteString(`
Generate 3-7 epics that logically group this work. Each epic should be:
- Cohesive: related functionality grouped together
- Sized appropriately: S (1-2 weeks), M (2-4 weeks), L (4-8 weeks)
- Ordered by dependencies: foundational work first

Output ONLY valid JSON in this exact format (no markdown, no explanation):
{
  "epics": [
    {
      "id": "EPIC-001",
      "title": "Epic Title",
      "description": "What this epic accomplishes",
      "size": "M",
      "priority": "P1",
      "dependencies": [],
      "task_count": 5,
      "stories": [
        {
          "id": "EPIC-001-STORY-01",
          "title": "Story title",
          "description": "User story description",
          "size": "S"
        }
      ]
    }
  ]
}

Priority levels: P0 (critical), P1 (high), P2 (medium), P3 (low)
Size levels: S, M, L, XL

Generate the JSON now:`)

	return sb.String()
}

func parseEpicResponse(content string) ([]epics.EpicProposal, error) {
	// Clean up the response - remove markdown code blocks if present
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Try to find JSON in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	var response struct {
		Epics []struct {
			ID           string   `json:"id"`
			Title        string   `json:"title"`
			Description  string   `json:"description"`
			Size         string   `json:"size"`
			Priority     string   `json:"priority"`
			Dependencies []string `json:"dependencies"`
			TaskCount    int      `json:"task_count"`
			Stories      []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Size        string `json:"size"`
			} `json:"stories"`
		} `json:"epics"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w\nContent: %s", err, content[:min(500, len(content))])
	}

	var proposals []epics.EpicProposal
	for _, e := range response.Epics {
		var stories []epics.StoryProposal
		for _, s := range e.Stories {
			stories = append(stories, epics.StoryProposal{
				ID:          s.ID,
				Title:       s.Title,
				Description: s.Description,
				Size:        epics.Size(s.Size),
			})
		}

		proposals = append(proposals, epics.EpicProposal{
			ID:           e.ID,
			Title:        e.Title,
			Description:  e.Description,
			Size:         epics.Size(e.Size),
			Priority:     epics.Priority(e.Priority),
			Dependencies: e.Dependencies,
			TaskCount:    e.TaskCount,
			Stories:      stories,
		})
	}

	if len(proposals) == 0 {
		return nil, fmt.Errorf("no epics generated")
	}

	return proposals, nil
}
