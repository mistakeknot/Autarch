package clavain

import (
	"context"
)

// DispatchOption configures a DispatchTask call.
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

// WithDispatchAgent sets the agent name.
func WithDispatchAgent(name string) DispatchOption {
	return func(o *dispatchOpts) { o.agent = name }
}

// WithDispatchName sets the dispatch display name.
func WithDispatchName(name string) DispatchOption {
	return func(o *dispatchOpts) { o.name = name }
}

// TrackAgent registers an agent dispatch with the OS layer for tracking.
// Call this after ic.DispatchSpawn() to keep the OS layer informed.
func (c *Client) TrackAgent(ctx context.Context, beadID, agentName string, agentType, dispatchID string) error {
	args := []string{"sprint-track-agent", beadID, agentName}
	if agentType != "" {
		args = append(args, agentType)
	}
	if dispatchID != "" {
		args = append(args, dispatchID)
	}
	_, err := c.execText(ctx, args...)
	return err
}

// CompleteAgent marks an agent as complete in the OS layer.
func (c *Client) CompleteAgent(ctx context.Context, agentID, status string) error {
	args := []string{"sprint-complete-agent", agentID}
	if status != "" {
		args = append(args, status)
	}
	_, err := c.execText(ctx, args...)
	return err
}
