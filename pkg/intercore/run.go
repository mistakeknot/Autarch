package intercore

import (
	"context"
	"fmt"
	"strconv"
)

// RunOption configures a RunCreate call.
type RunOption func(*runOpts)

type runOpts struct {
	scopeID     string
	complexity  int
	tokenBudget int64
	// Post-create settings (applied via RunSet after creation).
	autoAdvance *bool
	forceFull   *bool
}

// WithScopeID sets the bead/scope ID for the run.
func WithScopeID(id string) RunOption {
	return func(o *runOpts) { o.scopeID = id }
}

// WithComplexity sets the complexity (1-5).
func WithComplexity(n int) RunOption {
	return func(o *runOpts) { o.complexity = n }
}

// WithAutoAdvance sets auto-advance after creation (via RunSet).
func WithAutoAdvance(v bool) RunOption {
	return func(o *runOpts) { o.autoAdvance = &v }
}

// WithForceFull sets force-full after creation (via RunSet).
func WithForceFull(v bool) RunOption {
	return func(o *runOpts) { o.forceFull = &v }
}

// WithTokenBudget sets the token budget.
func WithTokenBudget(n int64) RunOption {
	return func(o *runOpts) { o.tokenBudget = n }
}

// RunCreate creates a new run. Returns the run ID (plain text, not JSON).
// AutoAdvance and ForceFull are applied via RunSet after creation.
func (c *Client) RunCreate(ctx context.Context, project, goal string, opts ...RunOption) (string, error) {
	var o runOpts
	for _, fn := range opts {
		fn(&o)
	}

	args := []string{"run", "create", "--project=" + project, "--goal=" + goal}
	if o.scopeID != "" {
		args = append(args, "--scope-id="+o.scopeID)
	}
	if o.complexity > 0 {
		args = append(args, "--complexity="+strconv.Itoa(o.complexity))
	}
	if o.tokenBudget > 0 {
		args = append(args, "--token-budget="+strconv.FormatInt(o.tokenBudget, 10))
	}

	// RunCreate returns a plain text run ID, not JSON.
	runID, err := c.execText(ctx, args...)
	if err != nil {
		return "", err
	}

	// Apply post-create settings if any.
	var setOpts []RunSetOption
	if o.autoAdvance != nil {
		setOpts = append(setOpts, SetAutoAdvance(*o.autoAdvance))
	}
	if o.forceFull != nil {
		setOpts = append(setOpts, SetForceFull(*o.forceFull))
	}
	if len(setOpts) > 0 {
		if err := c.RunSet(ctx, runID, setOpts...); err != nil {
			return runID, fmt.Errorf("created run %s but failed to apply settings: %w", runID, err)
		}
	}

	return runID, nil
}

// RunStatus returns the full status of a run.
func (c *Client) RunStatus(ctx context.Context, runID string) (*Run, error) {
	data, err := c.execJSON(ctx, "run", "status", runID)
	if err != nil {
		return nil, err
	}
	r, err := unmarshal[Run](data)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RunList returns all runs, optionally filtered to active only.
func (c *Client) RunList(ctx context.Context, active bool) ([]Run, error) {
	args := []string{"run", "list"}
	if active {
		args = append(args, "--active")
	}
	data, err := c.execJSON(ctx, args...)
	if err != nil {
		return nil, err
	}
	return unmarshal[[]Run](data)
}

// RunAdvance advances a run to the next phase.
func (c *Client) RunAdvance(ctx context.Context, runID string) (*AdvanceResult, error) {
	data, err := c.execJSON(ctx, "run", "advance", runID)
	if err != nil {
		return nil, err
	}
	r, err := unmarshal[AdvanceResult](data)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RunCancel cancels a run.
func (c *Client) RunCancel(ctx context.Context, runID string) error {
	_, err := c.execJSON(ctx, "run", "cancel", runID)
	return err
}

// RunPhase returns the current phase of a run (plain text).
func (c *Client) RunPhase(ctx context.Context, runID string) (string, error) {
	return c.execText(ctx, "run", "phase", runID)
}

// RunCurrent returns the active run ID for a project directory.
// Returns empty string and nil error if no active run.
func (c *Client) RunCurrent(ctx context.Context, projectDir string) (string, error) {
	id, err := c.execText(ctx, "run", "current", "--project="+projectDir)
	if err != nil {
		// "no active run" is not an error condition.
		if isNoRunError(err) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

// RunSetOption configures a RunSet call.
type RunSetOption func(args *[]string)

// SetComplexity sets complexity on an existing run.
func SetComplexity(n int) RunSetOption {
	return func(args *[]string) {
		*args = append(*args, "--complexity="+strconv.Itoa(n))
	}
}

// SetAutoAdvance sets auto-advance on an existing run.
func SetAutoAdvance(v bool) RunSetOption {
	return func(args *[]string) {
		*args = append(*args, "--auto-advance="+strconv.FormatBool(v))
	}
}

// SetForceFull sets force-full on an existing run.
func SetForceFull(v bool) RunSetOption {
	return func(args *[]string) {
		*args = append(*args, "--force-full="+strconv.FormatBool(v))
	}
}

// SetMaxDispatches sets max concurrent dispatches on a run.
func SetMaxDispatches(n int) RunSetOption {
	return func(args *[]string) {
		*args = append(*args, "--max-dispatches="+strconv.Itoa(n))
	}
}

// RunSet updates mutable fields on a run.
func (c *Client) RunSet(ctx context.Context, runID string, opts ...RunSetOption) error {
	args := []string{"run", "set", runID}
	for _, fn := range opts {
		fn(&args)
	}
	_, err := c.execText(ctx, args...)
	return err
}

// RunTokens returns aggregated token usage across dispatches for a run.
func (c *Client) RunTokens(ctx context.Context, runID string) (int64, error) {
	out, err := c.execText(ctx, "run", "tokens", runID)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("intercore: parse tokens: %w", err)
	}
	return n, nil
}

// RunBudget checks budget thresholds for a run.
func (c *Client) RunBudget(ctx context.Context, runID string) (*BudgetResult, error) {
	data, err := c.execJSON(ctx, "run", "budget", runID)
	if err != nil {
		return nil, err
	}
	r, err := unmarshal[BudgetResult](data)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RunEvents returns the phase event audit trail for a run.
func (c *Client) RunEvents(ctx context.Context, runID string) ([]Event, error) {
	data, err := c.execJSON(ctx, "run", "events", runID)
	if err != nil {
		return nil, err
	}
	return unmarshal[[]Event](data)
}

func isNoRunError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "no active run") || contains(msg, "not found")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
