package clavain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mistakeknot/intercore/pkg/contract"
)

// SubmitIntent sends a typed intent through clavain-cli via stdin and returns the structured result.
// Params are passed via stdin JSON (not CLI flags) to avoid /proc/cmdline exposure.
func (c *Client) SubmitIntent(ctx context.Context, intent *contract.Intent) (*contract.IntentResult, error) {
	if err := intent.Validate(); err != nil {
		return nil, fmt.Errorf("invalid intent: %w", err)
	}

	payload, err := json.Marshal(intent)
	if err != nil {
		return nil, fmt.Errorf("marshal intent: %w", err)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// Pipe JSON via stdin — never pass params as CLI flags (visible in /proc).
	cmd := exec.CommandContext(ctx, c.binPath, "intent", "submit")
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("intent submit: %s", errMsg)
	}

	var result contract.IntentResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return nil, fmt.Errorf("unmarshal intent result: %w", err)
	}
	return &result, nil
}

// SprintAdvanceIntent submits a typed sprint.advance intent.
// Idempotency key is deterministic: session+type+phase+bead — safe for retries.
func (c *Client) SprintAdvanceIntent(ctx context.Context, beadID, phase, sessionID string) (*contract.IntentResult, error) {
	return c.SubmitIntent(ctx, &contract.Intent{
		Type:           contract.IntentSprintAdvance,
		BeadID:         beadID,
		IdempotencyKey: fmt.Sprintf("%s-sprint.advance-%s-%s", sessionID, phase, beadID),
		SessionID:      sessionID,
		Timestamp:      time.Now().Unix(),
		Params:         map[string]any{"phase": phase},
	})
}

// GateEnforceIntent submits a typed gate.enforce intent.
// Idempotency key is deterministic: session+gate+phase+bead — safe for retries.
func (c *Client) GateEnforceIntent(ctx context.Context, beadID, targetPhase, artifactPath, sessionID string) (*contract.IntentResult, error) {
	return c.SubmitIntent(ctx, &contract.Intent{
		Type:           contract.IntentGateEnforce,
		BeadID:         beadID,
		IdempotencyKey: fmt.Sprintf("%s-gate-%s-%s", sessionID, targetPhase, beadID),
		SessionID:      sessionID,
		Timestamp:      time.Now().Unix(),
		Params:         map[string]any{"target_phase": targetPhase, "artifact_path": artifactPath},
	})
}
