package clavain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// SprintOption configures a SprintCreate call.
type SprintOption func(*sprintOpts)

type sprintOpts struct {
	complexity int
	lane       string
}

// WithSprintComplexity sets the complexity (1-5) for budget calculation.
func WithSprintComplexity(n int) SprintOption {
	return func(o *sprintOpts) { o.complexity = n }
}

// WithSprintLane sets the thematic lane label.
func WithSprintLane(lane string) SprintOption {
	return func(o *sprintOpts) { o.lane = lane }
}

// SprintCreate creates a sprint via clavain-cli (bead + ic run + budget + phases).
// Returns the bead ID (plain text output from clavain-cli sprint-create).
func (c *Client) SprintCreate(ctx context.Context, goal string, opts ...SprintOption) (string, error) {
	var o sprintOpts
	for _, fn := range opts {
		fn(&o)
	}

	args := []string{"sprint-create", goal}
	if o.complexity > 0 {
		args = append(args, strconv.Itoa(o.complexity))
	}
	if o.lane != "" {
		// Lane is the 3rd positional arg
		if o.complexity == 0 {
			args = append(args, "3") // default complexity
		}
		args = append(args, o.lane)
	}

	beadID, err := c.execText(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("sprint-create: %w", err)
	}
	return beadID, nil
}

// SprintAdvance advances a sprint to the next phase via clavain-cli.
// beadID is the sprint bead, currentPhase is the current phase name.
// Returns the pause reason (empty string means advanced successfully).
func (c *Client) SprintAdvance(ctx context.Context, beadID, currentPhase string, artifactPath ...string) (string, error) {
	args := []string{"sprint-advance", beadID, currentPhase}
	if len(artifactPath) > 0 && artifactPath[0] != "" {
		args = append(args, artifactPath[0])
	}

	// Use execRaw instead of execText: sprint-advance writes pause reasons
	// to stdout even on non-zero exit. execText discards stdout on error.
	out, err := c.execRaw(ctx, args...)
	if err != nil {
		reason := strings.TrimSpace(string(out))
		if reason != "" {
			return reason, nil // pause reason, not an error
		}
		return "", fmt.Errorf("sprint-advance: %w", err)
	}
	return "", nil // empty = advanced successfully
}

// SprintCancel cancels a sprint's ic run and marks the bead cancelled.
// This delegates to ic directly since clavain-cli doesn't have cancel yet.
// The cancel operation is not policy-governed (user explicitly cancels).
func (c *Client) SprintCancel(ctx context.Context, runID string) error {
	return fmt.Errorf("sprint cancel not yet implemented in clavain-cli — use ic.RunCancel()")
}

// SprintReadState reads the full state of a sprint.
func (c *Client) SprintReadState(ctx context.Context, beadID string) (string, error) {
	return c.execText(ctx, "sprint-read-state", beadID)
}

// resolveRunID resolves a bead ID to an ic run ID using clavain-cli's internal cache.
// This is a convenience for callers that need the underlying run ID.
func (c *Client) resolveRunID(ctx context.Context, beadID string) (string, error) {
	out, err := c.execRaw(ctx, "sprint-read-state", beadID)
	if err != nil {
		return "", err
	}
	// sprint-read-state returns JSON — parse properly, don't line-scan.
	var state struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &state); err != nil {
		return "", fmt.Errorf("resolveRunID: parse sprint state for %s: %w", beadID, err)
	}
	if state.ID == "" {
		return "", fmt.Errorf("resolveRunID: no run ID found for bead %s", beadID)
	}
	return state.ID, nil
}
