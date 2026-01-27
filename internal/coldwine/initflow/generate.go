package initflow

import (
	"context"
	"fmt"

	"github.com/mistakeknot/autarch/internal/coldwine/epics"
)

type Input struct {
	Summary string
	Depth   int
	Repo    string
}

type Result struct {
	Epics []epics.Epic
}

type Generator interface {
	Generate(ctx context.Context, input Input) (Result, error)
}

type Prompt struct {
	Text string
}

func BuildPrompt(input Input) Prompt {
	return Prompt{
		Text: fmt.Sprintf("Summary:\n%s\nDepth: %d\nRepo: %s\n", input.Summary, input.Depth, input.Repo),
	}
}

// GenerateEpics runs the generator and returns epics.
// If the generator fails or returns empty, it returns the error
// rather than silently falling back — callers should handle the error
// and decide whether to retry or use FallbackEpics.
func GenerateEpics(gen Generator, input Input) (Result, error) {
	out, err := gen.Generate(context.Background(), input)
	if err != nil {
		return Result{}, fmt.Errorf("epic generation: %w", err)
	}
	if len(out.Epics) == 0 {
		return Result{}, fmt.Errorf("epic generation returned no epics")
	}
	return out, nil
}

// FallbackEpics returns a minimal starter backlog when generation fails.
func FallbackEpics() []epics.Epic {
	return []epics.Epic{
		{
			ID:       "EPIC-001",
			Title:    "Initial backlog",
			Status:   epics.StatusTodo,
			Priority: epics.PriorityP2,
			Stories: []epics.Story{
				{
					ID:       "EPIC-001-S01",
					Title:    "Inventory existing tasks",
					Status:   epics.StatusTodo,
					Priority: epics.PriorityP2,
				},
			},
		},
	}
}
