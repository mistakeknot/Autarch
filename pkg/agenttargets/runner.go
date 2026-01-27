package agenttargets

import (
	"context"
	"time"
)

// SafetyPolicy configures limits for an agent run.
type SafetyPolicy struct {
	Timeout        time.Duration // Max wall-clock time (0 = no limit)
	MaxOutputBytes int64         // Max stdout+stderr capture (0 = no limit)
	Sandbox        bool          // Whether to enable sandboxing flags (e.g. --sandbox for Claude)
	ReadOnly       bool          // Restrict to read-only operations where supported
}

// DefaultSafetyPolicy returns conservative defaults: 30m timeout, 10MB output, sandbox enabled.
func DefaultSafetyPolicy() SafetyPolicy {
	return SafetyPolicy{
		Timeout:        30 * time.Minute,
		MaxOutputBytes: 10 * 1024 * 1024, // 10MB
		Sandbox:        true,
	}
}

// RunHandle provides control over a running agent process.
type RunHandle struct {
	ID        string
	Target    ResolvedTarget
	Policy    SafetyPolicy
	StartedAt time.Time
	Done      <-chan struct{}     // Closed when process exits
	Cancel    context.CancelFunc // Kill the process
	Wait      func() (RunResult, error)
}

// RunResult captures the outcome of a completed run.
type RunResult struct {
	ExitCode  int
	Output    []byte // Capped to MaxOutputBytes
	Truncated bool   // True if output was capped
	TimedOut  bool
	Duration  time.Duration
}

// AgentRunner launches agent processes with safety enforcement.
type AgentRunner interface {
	Run(ctx context.Context, target ResolvedTarget, policy SafetyPolicy, workDir string, prompt string) (*RunHandle, error)
}
