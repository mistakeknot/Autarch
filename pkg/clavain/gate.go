package clavain

import "context"

// EnforceGate checks whether a phase transition is allowed.
// Returns nil if the gate passes, error with reason if blocked.
func (c *Client) EnforceGate(ctx context.Context, beadID, targetPhase, artifactPath string) error {
	args := []string{"enforce-gate", beadID, targetPhase}
	if artifactPath != "" {
		args = append(args, artifactPath)
	}
	_, err := c.execText(ctx, args...)
	return err
}

// GateOverride forces advancement past a gate.
// This records the override with a reason for audit purposes.
// NOTE: Delegates to ic gate override since clavain-cli doesn't wrap it yet.
func (c *Client) GateOverride(ctx context.Context, beadID, reason string) error {
	// clavain-cli doesn't have gate-override yet
	// TODO(iv-gyq9l): Add gate-override to clavain-cli
	return ErrUnavailable
}
