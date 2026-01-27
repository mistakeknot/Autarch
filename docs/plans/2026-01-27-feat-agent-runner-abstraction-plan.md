# Agent Runner Abstraction with Safety Policies

## Problem

Coldwine dispatches tasks to AI agents (Claude, Codex, Aider, Cursor) but has no unified abstraction for launching and managing them. Agent launching is ad-hoc via `internal/coldwine/tmux/exec.go` (a bare `os/exec` wrapper with no timeouts, output caps, or sandboxing). The `pkg/agenttargets/` package handles target resolution and config but stops at producing a `ResolvedTarget` -- nothing consumes it to actually run an agent with safety guarantees. The `pkg/contract/` types define `Run` and `Outcome` but nothing bridges from target resolution to run lifecycle management.

## Proposed Solution

Add an `AgentRunner` interface in `pkg/agenttargets/` that takes a `ResolvedTarget` and a `SafetyPolicy`, launches the agent process, enforces limits, and returns a `RunHandle` for lifecycle management.

### Core types

```go
// SafetyPolicy configures limits for an agent run.
type SafetyPolicy struct {
    Timeout        time.Duration // Max wall-clock time (0 = no limit)
    MaxOutputBytes int64         // Max stdout+stderr capture (0 = no limit)
    Sandbox        bool          // Whether to enable sandboxing flags (e.g. --sandbox for Claude)
    ReadOnly       bool          // Restrict to read-only operations where supported
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
    ExitCode   int
    Output     []byte // Capped to MaxOutputBytes
    Truncated  bool   // True if output was capped
    TimedOut   bool
    Duration   time.Duration
}

// AgentRunner launches agent processes with safety enforcement.
type AgentRunner interface {
    Run(ctx context.Context, target ResolvedTarget, policy SafetyPolicy, workDir string, prompt string) (*RunHandle, error)
}
```

### Implementation approach

1. `ExecAgentRunner` -- default implementation using `os/exec.CommandContext` with timeout via context deadline, `io.LimitReader` on output pipes, and agent-specific sandbox flags injected into args.
2. Agent-specific arg injection: a small map of agent name to flag transforms (e.g., Claude gets `--sandbox`, Codex gets `--sandbox`).
3. `DefaultSafetyPolicy()` returns conservative defaults (30m timeout, 10MB output cap, sandbox=true).
4. Coldwine's spawn/dispatch code calls `Resolver.Resolve()` then `AgentRunner.Run()`, converting the `RunResult` into a `contract.Outcome`.

## Acceptance Criteria

- [ ] `AgentRunner` interface defined in `pkg/agenttargets/runner.go`
- [ ] `SafetyPolicy`, `RunHandle`, `RunResult` types in `pkg/agenttargets/runner.go`
- [ ] `ExecAgentRunner` implementation in `pkg/agenttargets/runner_exec.go`
- [ ] `DefaultSafetyPolicy()` function with 30m timeout, 10MB output, sandbox=true
- [ ] Timeout enforcement via `context.WithTimeout`
- [ ] Output capping via `io.LimitReader` on stdout/stderr pipes
- [ ] Sandbox flag injection for known agents (claude, codex)
- [ ] Unit tests: timeout fires, output truncation works, sandbox args appended
- [ ] Integration point: Coldwine spawn code updated to use `AgentRunner` instead of bare `tmux.ExecRunner`

## Key Files

| File | Role |
|------|------|
| `pkg/agenttargets/types.go` | Existing `Target`, `Registry` types |
| `pkg/agenttargets/resolver.go` | Existing `ResolvedTarget`, `Resolver` |
| `pkg/agenttargets/runner.go` | NEW: `AgentRunner` interface, `SafetyPolicy`, `RunHandle`, `RunResult` |
| `pkg/agenttargets/runner_exec.go` | NEW: `ExecAgentRunner` implementation |
| `pkg/agenttargets/runner_test.go` | NEW: Unit tests |
| `pkg/contract/types.go` | Existing `Run`, `Outcome` types (consumed, not modified) |
| `internal/coldwine/tmux/exec.go` | Existing bare exec wrapper (to be replaced at call sites) |
| `internal/coldwine/storage/agent_session.go` | Session tracking (unchanged, used by caller) |
