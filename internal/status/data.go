package status

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Run represents an Intercore run from `ic run list --json`.
type Run struct {
	ID         string   `json:"id"`
	Goal       string   `json:"goal"`
	Phase      string   `json:"phase"`
	Phases     []string `json:"phases"`
	Status     string   `json:"status"`
	ScopeID    string   `json:"scope_id"`
	Complexity int      `json:"complexity"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
	ProjectDir string   `json:"project_dir"`
}

// Dispatch represents an Intercore dispatch from `ic dispatch list --json`.
type Dispatch struct {
	ID          string  `json:"id"`
	AgentType   string  `json:"agent_type"`
	Status      string  `json:"status"`
	Name        *string `json:"name"`
	Model       *string `json:"model"`
	InTokens    int     `json:"in_tokens"`
	OutTokens   int     `json:"out_tokens"`
	CreatedAt   int64   `json:"created_at"`
	StartedAt   *int64  `json:"started_at"`
	CompletedAt *int64  `json:"completed_at"`
	ScopeID     *string `json:"scope_id"`
	ProjectDir  string  `json:"project_dir"`
}

// DisplayName returns the dispatch name, falling back to agent type.
func (d Dispatch) DisplayName() string {
	if d.Name != nil && *d.Name != "" {
		return *d.Name
	}
	return d.AgentType
}

// DisplayModel returns the model name or empty string.
func (d Dispatch) DisplayModel() string {
	if d.Model != nil {
		return *d.Model
	}
	return ""
}

// Event represents an Intercore event from `ic events tail`.
type Event struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// TokenSummary represents token usage from `ic run tokens --json`.
type TokenSummary struct {
	RunID        string `json:"run_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CacheHits    int64  `json:"cache_hits"`
}

// FetchRuns calls `ic run list --active --json` and parses the result.
func FetchRuns(ctx context.Context, projectDir string) ([]Run, error) {
	out, err := runIC(ctx, projectDir, "run", "list", "--active", "--json")
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
	out, err := runIC(ctx, projectDir, "run", "list", "--json")
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
	out, err := runIC(ctx, projectDir, args...)
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
	out, err := runIC(ctx, projectDir, args...)
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
	out, err := runIC(ctx, projectDir, "run", "tokens", runID, "--json")
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

// runIC executes an `ic` command and returns its stdout.
func runIC(ctx context.Context, projectDir string, args ...string) (string, error) {
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
