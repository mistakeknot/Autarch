package intercore

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// --- Dispatch Operations ---

// DispatchOption configures a DispatchSpawn call.
type DispatchOption func(*dispatchOpts)

type dispatchOpts struct {
	dispatchType string
	agent        string
	name         string
}

// WithDispatchType sets the dispatch type (e.g., "task", "review").
func WithDispatchType(t string) DispatchOption {
	return func(o *dispatchOpts) { o.dispatchType = t }
}

// WithAgent sets the agent name for the dispatch.
func WithAgent(name string) DispatchOption {
	return func(o *dispatchOpts) { o.agent = name }
}

// WithDispatchName sets the dispatch name.
func WithDispatchName(name string) DispatchOption {
	return func(o *dispatchOpts) { o.name = name }
}

// DispatchSpawn spawns an agent dispatch. Returns the dispatch ID.
func (c *Client) DispatchSpawn(ctx context.Context, runID string, opts ...DispatchOption) (string, error) {
	var o dispatchOpts
	for _, fn := range opts {
		fn(&o)
	}

	args := []string{"dispatch", "spawn", "--run-id=" + runID}
	if o.dispatchType != "" {
		args = append(args, "--type="+o.dispatchType)
	}
	if o.agent != "" {
		args = append(args, "--agent="+o.agent)
	}
	if o.name != "" {
		args = append(args, "--name="+o.name)
	}

	return c.execText(ctx, args...)
}

// DispatchStatus returns the status of a dispatch.
func (c *Client) DispatchStatus(ctx context.Context, dispatchID string) (*Dispatch, error) {
	data, err := c.execJSON(ctx, "dispatch", "status", dispatchID)
	if err != nil {
		return nil, err
	}
	d, err := unmarshal[Dispatch](data)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// DispatchList returns dispatches, optionally filtered to active only.
func (c *Client) DispatchList(ctx context.Context, active bool) ([]Dispatch, error) {
	args := []string{"dispatch", "list"}
	if active {
		args = append(args, "--active")
	}
	data, err := c.execJSON(ctx, args...)
	if err != nil {
		return nil, err
	}
	return unmarshal[[]Dispatch](data)
}

// DispatchWait blocks until a dispatch completes or timeout.
func (c *Client) DispatchWait(ctx context.Context, dispatchID string, timeout time.Duration) error {
	args := []string{"dispatch", "wait", dispatchID}
	if timeout > 0 {
		args = append(args, "--timeout="+timeout.String())
	}

	// Use a longer subprocess timeout for wait operations.
	waitCtx := ctx
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout+5*time.Second)
		defer cancel()
	}

	fullArgs := append(c.baseArgs(false), args...)
	_, err := c.execRaw(waitCtx, fullArgs...)
	return err
}

// DispatchKill kills a running dispatch.
func (c *Client) DispatchKill(ctx context.Context, dispatchID string) error {
	_, err := c.execText(ctx, "dispatch", "kill", dispatchID)
	return err
}

// --- Gate Operations ---

// GateCheck performs a dry-run gate evaluation.
func (c *Client) GateCheck(ctx context.Context, runID string) (*GateResult, error) {
	data, err := c.execJSON(ctx, "gate", "check", runID)
	if err != nil {
		// Gate check returns exit code 1 for "fail" — still has valid JSON.
		// Try to parse even on error.
		if data == nil || len(data) == 0 {
			return nil, err
		}
	}
	r, err2 := unmarshal[GateResult](data)
	if err2 != nil {
		if err != nil {
			return nil, err // original error
		}
		return nil, err2
	}
	return &r, nil
}

// GateOverride forces advancement past a failing gate.
func (c *Client) GateOverride(ctx context.Context, runID, reason string) error {
	_, err := c.execText(ctx, "gate", "override", runID, "--reason="+reason)
	return err
}

// GateRules returns the configured gate rules.
func (c *Client) GateRules(ctx context.Context) ([]GateRule, error) {
	data, err := c.execJSON(ctx, "gate", "rules")
	if err != nil {
		return nil, err
	}
	return unmarshal[[]GateRule](data)
}

// --- Artifact Operations ---

// ArtifactAdd registers an artifact on a run.
func (c *Client) ArtifactAdd(ctx context.Context, runID, phase, path, artifactType string) error {
	args := []string{"run", "artifact", "add", runID, "--phase=" + phase, "--path=" + path}
	if artifactType != "" {
		args = append(args, "--type="+artifactType)
	}
	_, err := c.execText(ctx, args...)
	return err
}

// ArtifactList returns artifacts for a run, optionally filtered by phase.
func (c *Client) ArtifactList(ctx context.Context, runID, phase string) ([]Artifact, error) {
	args := []string{"run", "artifact", "list", runID}
	if phase != "" {
		args = append(args, "--phase="+phase)
	}
	data, err := c.execJSON(ctx, args...)
	if err != nil {
		return nil, err
	}
	return unmarshal[[]Artifact](data)
}

// --- State Operations ---

// StateSet sets a state value (JSON string piped via stdin).
func (c *Client) StateSet(ctx context.Context, key, scope, jsonValue string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.timeout
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := append(c.baseArgs(false), "state", "set", key, scope)
	cmd := exec.CommandContext(ctx, c.binPath, args...)
	cmd.Stdin = stringReader(jsonValue)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ic state set %s %s: %s", key, scope, trimOutput(out, err))
	}
	return nil
}

// StateGet retrieves a state value. Returns empty string if not found.
func (c *Client) StateGet(ctx context.Context, key, scope string) (string, error) {
	result, err := c.execText(ctx, "state", "get", key, scope)
	if err != nil {
		// "not found" is not an error — return empty.
		if isNotFoundError(err) {
			return "", nil
		}
		return "", err
	}
	return result, nil
}

// StateDelete removes a state value.
func (c *Client) StateDelete(ctx context.Context, key, scope string) error {
	_, err := c.execText(ctx, "state", "delete", key, scope)
	return err
}

// --- Lock Operations ---

// LockAcquire acquires a named lock.
func (c *Client) LockAcquire(ctx context.Context, name, scope string, timeout time.Duration) error {
	args := []string{"lock", "acquire", name, scope}
	if timeout > 0 {
		args = append(args, "--timeout="+timeout.String())
	}
	_, err := c.execText(ctx, args...)
	return err
}

// LockRelease releases a named lock.
func (c *Client) LockRelease(ctx context.Context, name, scope string) error {
	_, err := c.execText(ctx, "lock", "release", name, scope)
	return err
}

// --- Run Agent Operations ---

// RunAgentAdd registers an agent on a run.
func (c *Client) RunAgentAdd(ctx context.Context, runID, agentType string, name, dispatchID string) (string, error) {
	args := []string{"run", "agent", "add", runID, "--type=" + agentType}
	if name != "" {
		args = append(args, "--name="+name)
	}
	if dispatchID != "" {
		args = append(args, "--dispatch-id="+dispatchID)
	}
	return c.execText(ctx, args...)
}

// RunAgentList lists agents for a run.
func (c *Client) RunAgentList(ctx context.Context, runID string) ([]RunAgent, error) {
	data, err := c.execJSON(ctx, "run", "agent", "list", runID)
	if err != nil {
		return nil, err
	}
	return unmarshal[[]RunAgent](data)
}

// RunAgentUpdate updates an agent's status.
func (c *Client) RunAgentUpdate(ctx context.Context, agentID, status string) error {
	_, err := c.execText(ctx, "run", "agent", "update", agentID, "--status="+status)
	return err
}

// --- Sentinel Operations ---

// SentinelCheck checks a named sentinel with a cooldown interval.
// Returns true if allowed (proceed), false if throttled.
func (c *Client) SentinelCheck(ctx context.Context, name, scope string, interval time.Duration) (bool, error) {
	args := []string{"sentinel", "check", name, scope, "--interval=" + strconv.Itoa(int(interval.Seconds())) + "s"}
	_, err := c.execText(ctx, args...)
	if err != nil {
		// Exit code 1 = throttled (not an error, just "skip").
		return false, nil
	}
	return true, nil
}

// --- Helpers ---

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "not found") || contains(msg, "no rows")
}

func trimOutput(out []byte, fallback error) string {
	s := string(out)
	if s = stringTrimSpace(s); s != "" {
		return s
	}
	return fallback.Error()
}

func stringTrimSpace(s string) string {
	return strings.TrimSpace(s)
}

func stringReader(s string) io.Reader {
	return strings.NewReader(s + "\n")
}
