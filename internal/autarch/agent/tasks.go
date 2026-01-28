package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
)

// GenerateTasks uses the coding agent to generate task proposals from epics.
func GenerateTasks(ctx context.Context, agent *Agent, epicList []epics.EpicProposal) ([]tasks.TaskProposal, error) {
	return GenerateTasksWithOutput(ctx, agent, epicList, nil)
}

// GenerateTasksWithOutput streams agent output while generating task proposals.
func GenerateTasksWithOutput(ctx context.Context, agent *Agent, epicList []epics.EpicProposal, onOutput OutputCallback) ([]tasks.TaskProposal, error) {
	prompt := buildTaskPrompt(epicList)

	// Set a reasonable timeout for LLM generation
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	resp, err := agent.GenerateWithOutput(ctx, GenerateRequest{
		Prompt: prompt,
	}, onOutput)
	if err != nil {
		return nil, fmt.Errorf("agent generation failed: %w", err)
	}

	// Parse the response into task proposals
	taskList, err := parseTaskResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse task response: %w", err)
	}

	return taskList, nil
}

func buildTaskPrompt(epicList []epics.EpicProposal) string {
	var sb strings.Builder

	sb.WriteString(`You are a software architect helping break down epics into concrete tasks.

Given the following epics, generate specific implementation tasks for each.

EPICS:
`)

	for _, epic := range epicList {
		sb.WriteString(fmt.Sprintf("\n## %s: %s\n", epic.ID, epic.Title))
		sb.WriteString(fmt.Sprintf("Description: %s\n", epic.Description))
		sb.WriteString(fmt.Sprintf("Size: %s, Priority: %s\n", epic.Size, epic.Priority))
		if len(epic.Dependencies) > 0 {
			sb.WriteString(fmt.Sprintf("Dependencies: %s\n", strings.Join(epic.Dependencies, ", ")))
		}
		if len(epic.Stories) > 0 {
			sb.WriteString("Stories:\n")
			for _, story := range epic.Stories {
				sb.WriteString(fmt.Sprintf("  - %s: %s\n", story.ID, story.Title))
			}
		}
	}

	sb.WriteString(`
Generate concrete implementation tasks for each epic. Each task should be:
- Specific and actionable (a developer should know exactly what to do)
- Small enough to complete in 1-4 hours
- Typed appropriately: implementation, test, documentation, review, setup, research

Output ONLY valid JSON in this exact format (no markdown, no explanation):
{
  "tasks": [
    {
      "id": "TASK-001",
      "epic_id": "EPIC-001",
      "title": "Task title",
      "description": "Detailed description of what to implement",
      "type": "implementation",
      "dependencies": []
    }
  ]
}

Task types: implementation, test, documentation, review, setup, research

Generate tasks for ALL epics. Include:
- Setup/infrastructure tasks first
- Implementation tasks for core functionality
- Test tasks for each implementation
- Documentation tasks where needed

Generate the JSON now:`)

	return sb.String()
}

func parseTaskResponse(content string) ([]tasks.TaskProposal, error) {
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
		Tasks []struct {
			ID           string   `json:"id"`
			EpicID       string   `json:"epic_id"`
			Title        string   `json:"title"`
			Description  string   `json:"description"`
			Type         string   `json:"type"`
			Dependencies []string `json:"dependencies"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w\nContent: %s", err, content[:min(500, len(content))])
	}

	var taskList []tasks.TaskProposal
	for _, t := range response.Tasks {
		taskType := tasks.TaskType(t.Type)
		// Validate and default task type
		switch taskType {
		case tasks.TaskTypeImplementation, tasks.TaskTypeTest, tasks.TaskTypeDocumentation,
			tasks.TaskTypeReview, tasks.TaskTypeSetup, tasks.TaskTypeResearch:
			// Valid
		default:
			taskType = tasks.TaskTypeImplementation
		}

		taskList = append(taskList, tasks.TaskProposal{
			ID:           t.ID,
			EpicID:       t.EpicID,
			Title:        t.Title,
			Description:  t.Description,
			Type:         taskType,
			Dependencies: t.Dependencies,
		})
	}

	if len(taskList) == 0 {
		return nil, fmt.Errorf("no tasks generated")
	}

	// Resolve dependencies to mark ready tasks
	tasks.ResolveCrossEpicDependencies(taskList)

	return taskList, nil
}
