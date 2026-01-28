title: fix: guardrails follow-through
status: draft
date: 2026-01-28

# fix: guardrails follow-through

## Summary
Finish the remaining pieces of the performance/reliability guardrails. Many items in the 2026-01-27 plan are already implemented (db helper, timeout constants, Intermute client offline/timeout), but a few gaps still cause noisy failures or unbounded work. This plan focuses on the minimum set of changes that make Intermute optional without error logs, enforce bounded heartbeats, and remove the last unbounded context.Background() in production work paths.

## Current State (Already Implemented)
- Unified SQLite helper exists at `pkg/db/open.go` and is used by:
  - `pkg/events/store.go`
  - `internal/coldwine/storage/db.go` (plus foreign_keys pragma)
  - `internal/pollard/state/db.go`
- Timeout constants exist at `pkg/timeout/timeout.go`.
- Intermute client is already offline-safe and has default timeout:
  - `pkg/intermute/client.go` provides ErrOffline, `Available()`, and request timeout via `withTimeout()`.
- Event bridge forwarding already uses a bounded timeout in `pkg/events/writer.go`.

## Gaps to Close
1. **Intermute registration still hard-fails** when `INTERMUTE_URL` is missing (`pkg/intermute/register.go`, `internal/intermute/intermute.go`). This causes noisy logs and violates the “graceful degradation” goal.
2. **Heartbeat uses context.Background()** in register loops, with no timeout.
3. **One production path still uses context.Background()** for LLM summary calls in Coldwine coordination.

## Scope
- Make Intermute registration no-op when URL is missing (return `func(){}` stop + nil error).
- Apply bounded timeouts to heartbeat calls.
- Add explicit timeout for Coldwine LLM summary execution.
- Update affected tests and help text where applicable.

Out of scope:
- Coordination API foundation (handled by its own plan).
- New retry/backoff policy for all HTTP calls (can be a later hardening pass).

## Plan

### Task 1: Graceful Intermute registration (no URL)
**Files:**
- `pkg/intermute/register.go`
- `internal/intermute/intermute.go`
- Add tests in `pkg/intermute/register_test.go`

**Steps:**
- [x] Change `Register()` to return a no-op stop (`func(){}`) and nil error when `INTERMUTE_URL` is empty.
- [x] Ensure `RegisterTool()` inherits the same behavior.
- [x] Update `internal/intermute/intermute.go` (deprecated) to match the no-op behavior.
- [x] Add tests:
  - [x] `TestRegisterNoURLNoop` (missing URL returns nil error and non-nil stop).
  - [x] `TestRegisterNoURLSkipsClient` (`newClient` is not called when URL is empty; use test hooks).
  - [x] `TestRegisterHeartbeatHasDeadline` (heartbeat context has a deadline).

**Acceptance:**
- Tools no longer log an “INTERMUTE_URL required” error on startup when unset.
- Callers do not need code changes to handle nil stop.

### Task 2: Heartbeat timeouts
**Files:**
- `pkg/intermute/register.go`
- `internal/intermute/intermute.go`
- `pkg/timeout/timeout.go` (if a new constant is needed)

**Steps:**
- [x] Wrap heartbeat calls with `context.WithTimeout` using `timeout.HTTPDefault`.
- [x] Ensure ticker loop uses the bounded context on every heartbeat.
- [x] Add tests that assert heartbeat receives a context with a deadline (via test hook).

**Acceptance:**
- Heartbeats can’t block indefinitely.

### Task 3: Coldwine LLM summary timeout
**Files:**
- `internal/coldwine/coordination/compat.go`
- `internal/coldwine/coordination/llm_summary_test.go`

**Steps:**
- [x] Update `SummarizeThread` to use a bounded context for `RunLLMSummaryCommand`.
- [x] Use `timeout.HTTPDefault` for the LLM summary call.
- [x] Update tests to reflect the new timeout behavior.

**Acceptance:**
- LLM summary work uses a timeout instead of `context.Background()`.

### Task 4: Verification sweep (bounded)
**Files:**
- Audit **only** the specific production paths already identified:
  - `internal/coldwine/coordination/compat.go` (LLM summary)
  - `pkg/intermute/register.go` (heartbeat)
  - `internal/intermute/intermute.go` (heartbeat)

**Steps:**
- [x] Re-run `rg "context.Background()"` and confirm remaining matches are in tests or CLI startup.
- [x] If any non-test IO path remains, convert it to a bounded context (no changes required for this plan’s scoped paths).

**Acceptance:**
- No unbounded `context.Background()` in production IO paths.

## Testing
- `go test ./pkg/intermute -run TestRegisterNoURL`
- `go test ./pkg/intermute -run TestRegisterNoURLNoop`
- `go test ./pkg/intermute -run TestRegisterNoURLSkipsClient`
- `go test ./internal/coldwine/coordination -run Summary`
- `go test ./internal/intermute`
- `go test ./...` (optional full sweep)

## Risks
- Changing Register() behavior could mask real configuration errors. Mitigation: emit a one-time warning **only when** `INTERMUTE_URL` is empty **and** either `INTERMUTE_API_KEY` or `INTERMUTE_PROJECT` is set (suspicious config). Otherwise remain silent and rely on `Available()` checks for call sites.

## Open Questions
- Should we add a dedicated `timeout.IntermuteHeartbeat` constant, or reuse `timeout.HTTPDefault`?
- Do we want a warning log when `INTERMUTE_URL` is missing **only** if other Intermute env vars are set? (recommended)
