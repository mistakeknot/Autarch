package icdata

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// FetchRuns calls `ic run list --active --json` and parses the result.
func FetchRuns(ctx context.Context, projectDir string) ([]Run, error) {
	out, err := RunIC(ctx, projectDir, "run", "list", "--active", "--json")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	var runs []Run
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		return nil, fmt.Errorf("parse runs: %w", err)
	}
	return runs, nil
}

// FetchAllRuns calls `ic run list --json` (including completed/cancelled).
func FetchAllRuns(ctx context.Context, projectDir string) ([]Run, error) {
	out, err := RunIC(ctx, projectDir, "run", "list", "--json")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	var runs []Run
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		return nil, fmt.Errorf("parse runs: %w", err)
	}
	return runs, nil
}

// FetchDispatches calls `ic dispatch list --json` and optionally filters by scope.
func FetchDispatches(ctx context.Context, projectDir string, activeOnly bool) ([]Dispatch, error) {
	args := []string{"dispatch", "list", "--json"}
	if activeOnly {
		args = append(args, "--active")
	}
	out, err := RunIC(ctx, projectDir, args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	var dispatches []Dispatch
	if err := json.Unmarshal([]byte(out), &dispatches); err != nil {
		return nil, fmt.Errorf("parse dispatches: %w", err)
	}
	return dispatches, nil
}

// FetchEvents calls `ic events tail` and parses JSON lines.
func FetchEvents(ctx context.Context, projectDir string, runID string, limit int) ([]Event, error) {
	args := []string{"events", "tail"}
	if runID != "" {
		args = append(args, runID)
	} else {
		args = append(args, "--all")
	}
	if limit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", limit))
	}
	out, err := RunIC(ctx, projectDir, args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	// Events are JSON lines (one JSON object per line)
	var events []Event
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // skip malformed lines
		}
		events = append(events, ev)
	}
	return events, nil
}

// FetchTokens calls `ic run tokens <id> --json`.
func FetchTokens(ctx context.Context, projectDir string, runID string) (TokenSummary, error) {
	out, err := RunIC(ctx, projectDir, "run", "tokens", runID, "--json")
	if err != nil {
		return TokenSummary{}, err
	}
	if strings.TrimSpace(out) == "" {
		return TokenSummary{}, nil
	}
	var ts TokenSummary
	if err := json.Unmarshal([]byte(out), &ts); err != nil {
		return TokenSummary{}, fmt.Errorf("parse tokens: %w", err)
	}
	return ts, nil
}

// FetchLanes calls `ic lane list --json` to get all lanes.
func FetchLanes(ctx context.Context, projectDir string) ([]Lane, error) {
	out, err := RunIC(ctx, projectDir, "lane", "list", "--json")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	var lanes []Lane
	if err := json.Unmarshal([]byte(out), &lanes); err != nil {
		return nil, fmt.Errorf("parse lanes: %w", err)
	}
	return lanes, nil
}

// FetchLaneVelocity calls `ic lane velocity --json` to get starvation scores.
func FetchLaneVelocity(ctx context.Context, projectDir string) ([]LaneVelocity, error) {
	out, err := RunIC(ctx, projectDir, "lane", "velocity", "--json")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return nil, nil
	}
	var velocities []LaneVelocity
	if err := json.Unmarshal([]byte(out), &velocities); err != nil {
		return nil, fmt.Errorf("parse lane velocity: %w", err)
	}
	return velocities, nil
}

// RunIC executes an `ic` command and returns its stdout.
func RunIC(ctx context.Context, projectDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "ic", args...)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("ic %s: exit %d: %s", args[0], exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("ic %s: %w", args[0], err)
	}
	return string(out), nil
}
