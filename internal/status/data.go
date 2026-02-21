package status

import (
	"context"

	"github.com/mistakeknot/autarch/internal/icdata"
)

// Re-export types from icdata for backward compatibility within this package.
// TUI pane files (runs.go, dispatches.go, events.go) use these type aliases
// so they can reference the types without a package qualifier.
type Run = icdata.Run
type Dispatch = icdata.Dispatch
type Event = icdata.Event
type TokenSummary = icdata.TokenSummary

// Re-export fetch functions — the TUI model calls these in fetchData().
func FetchRuns(ctx context.Context, projectDir string) ([]Run, error) {
	return icdata.FetchRuns(ctx, projectDir)
}

func FetchAllRuns(ctx context.Context, projectDir string) ([]Run, error) {
	return icdata.FetchAllRuns(ctx, projectDir)
}

func FetchDispatches(ctx context.Context, projectDir string, activeOnly bool) ([]Dispatch, error) {
	return icdata.FetchDispatches(ctx, projectDir, activeOnly)
}

func FetchEvents(ctx context.Context, projectDir string, runID string, limit int) ([]Event, error) {
	return icdata.FetchEvents(ctx, projectDir, runID, limit)
}

func FetchTokens(ctx context.Context, projectDir string, runID string) (TokenSummary, error) {
	return icdata.FetchTokens(ctx, projectDir, runID)
}
