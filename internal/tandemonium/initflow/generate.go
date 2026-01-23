package initflow

import (
	"context"
	"fmt"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/epics"
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

func GenerateEpics(gen Generator, input Input) (Result, error) {
	out, err := gen.Generate(context.Background(), input)
	if err == nil && len(out.Epics) > 0 {
		return out, nil
	}
	fallback := epics.Epic{
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
	}
	return Result{Epics: []epics.Epic{fallback}}, nil
}
