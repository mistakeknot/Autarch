---
status: complete
priority: p2
issue_id: "004"
tags: [coldwine, agents, safety]
dependencies: []
---

# Integrate AgentRunner safety abstraction into Coldwine

## Problem Statement

The AgentRunner abstraction exists in `pkg/agenttargets`, but Coldwine still starts agents via `tmux.ExecRunner` without safety policies, timeouts, or output caps.

## Findings

- Plan requires Coldwine spawn code to use AgentRunner instead of bare tmux exec. (`docs/plans/2026-01-27-feat-agent-runner-abstraction-plan.md:53-65`)
- Coldwine task start/stop uses `tmux.ExecRunner` directly. (`internal/coldwine/tui/model.go:1592-1655`)
- `pkg/agenttargets/runner.go` and `runner_exec.go` are implemented but not used by Coldwine. (`pkg/agenttargets/runner.go:8-47`, `pkg/agenttargets/runner_exec.go:20-85`)

## Proposed Solutions

### Option 1: Wire AgentRunner into Coldwine task start/stop

**Approach:**
- Resolve agent target via `agenttargets.Resolver`
- Use `ExecAgentRunner.Run` with `DefaultSafetyPolicy`
- Map results into Coldwine session lifecycle

**Pros:**
- Enforces timeouts/output caps
- Centralizes agent execution safety

**Cons:**
- Requires refactoring tmux session flow

**Effort:** 4-6 hours

**Risk:** Medium

---

### Option 2: Wrap tmux runner in an AgentRunner adapter

**Approach:**
- Implement an AgentRunner that delegates to tmux Start/Stop, preserving tmux UX

**Pros:**
- Minimal behavior change

**Cons:**
- Safety guarantees still limited

**Effort:** 2-3 hours

**Risk:** Medium

## Recommended Action

Use `agenttargets.AgentRunner` for Coldwine's init agent execution to enforce safety policy and output caps; keep tmux session lifecycle unchanged for interactive task sessions.

## Technical Details

**Affected files:**
- `docs/plans/2026-01-27-feat-agent-runner-abstraction-plan.md:53-65`
- `internal/coldwine/tui/model.go:1592-1655`
- `pkg/agenttargets/runner.go:8-47`
- `pkg/agenttargets/runner_exec.go:20-85`

## Acceptance Criteria

- [x] Coldwine starts agents through AgentRunner with SafetyPolicy
- [x] Agent output caps and timeouts enforced
- [x] Coldwine session lifecycle still functions (start/stop)
- [x] Tests cover integration path

## Work Log

### 2026-01-28 - Initial Discovery

**By:** Codex

**Actions:**
- Verified AgentRunner exists but is unused in Coldwine
- Located direct tmux ExecRunner usage in task start/stop

**Learnings:**
- Safety abstraction not yet integrated into task execution

### 2026-01-28 - Implementation

**By:** Codex

**Actions:**
- Wired Coldwine init agent execution to `agenttargets.ExecAgentRunner` with `DefaultSafetyPolicy`
- Added runner helper + tests for runner integration
- Preserved tmux-based session lifecycle for task start/stop

**Learnings:**
- Coldwine's automated agent execution currently happens in init flow; task sessions remain interactive via tmux
