# Brainstorm: Parallel Codex Mechanical Batch

**Date:** 2026-02-11
**Status:** Ready for planning

## What We're Building

Dispatch 3 independent Codex agents in parallel to tackle mechanical, well-scoped beads from the Autarch backlog. Each agent handles one bead with clear file paths, fix patterns, and test requirements.

## The 3 Tasks

### 1. Autarch-i0u: Context cancellation guardrails
- **What:** Replace `context.Background()` with proper `context.WithCancel` in 7 HIGH RISK TUI goroutine sites
- **Files:** `sprint_view.go:90,112,476`, `gurgeh_onboarding.go:89`, `arbiter_view.go:148,156,264`
- **Pattern:** Follow existing correct pattern at `sprint_view.go:436` — `context.WithCancel` + store cancel func + call on cleanup
- **Tests:** Add cancellation tests verifying goroutines clean up after context cancel
- **Scope:** `go test ./internal/tui/... -run TestCancel -v` and `go test ./internal/gurgeh/... -run TestCancel -v`

### 2. Autarch-6eg: Pollard test coverage
- **What:** Write tests for YAML persistence layer (currently zero tests, 0.14 test ratio)
- **Files:** `internal/pollard/` — focus on YAML read/write/round-trip
- **Tests:** Table-driven tests for YAML serialization, edge cases (empty fields, unicode, large files)
- **Scope:** `go test ./internal/pollard/... -v -short`

### 3. Autarch-wq8: Intermute background startup
- **What:** Move `EnsureRunning` from synchronous call to background `tea.Cmd`
- **Files:** `pkg/intermute/` and TUI integration points
- **Pattern:** Return a `tea.Cmd` that runs `EnsureRunning` asynchronously, send result message back to update model
- **Tests:** Test that startup doesn't block TUI init, test message handling for startup success/failure
- **Scope:** `go test ./pkg/intermute/... -v -short`

## Why This Approach

- **All mechanical:** Grep-find-fix or write-tests — no design decisions needed mid-task
- **All independent:** Different packages, no shared files, no ordering dependencies
- **All testable:** Each has a clear `go test` command to verify success
- **Known pattern:** Matches successful prior Codex dispatches (e.g., Phase 3 signals overlay)

## Key Decisions

- **3 agents, not more:** Keep batch small for reliable verification
- **Fix + test:** Each agent writes tests for its changes (not just code fixes)
- **Scoped test commands:** Always use `-run TestPattern` or `-short` to avoid hanging integration tests
- **GOCACHE=/tmp/go-build-cache:** Required for Codex agent Go builds
- **No commits:** Agents must NOT commit or push (we verify and commit ourselves)

## Constraints for All Agents

- `GOCACHE=/tmp/go-build-cache` in environment
- Do NOT run `go test ./... -v` (hangs on arbiter integration tests)
- Do NOT commit, push, or modify git state
- Do NOT touch files outside the scoped package
- Verify with scoped `go test` before declaring success

## Open Questions

None — all three tasks are well-understood from prior flux-drive analysis.

## Next

Run `/clavain:write-plan` to create detailed implementation plan with Codex dispatch prompts.
