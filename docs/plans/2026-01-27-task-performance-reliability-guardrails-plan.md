# Performance/Reliability Guardrails

**Date:** 2026-01-27
**Status:** Draft

## Problem

Autarch tools lack production-hardening in three areas:

1. **Intermute client has no graceful degradation.** `NewClient` fails hard when `INTERMUTE_URL` is unset. Every REST method passes errors straight through with no timeouts, retries, or fallback. If the Intermute server is down, all tools that depend on it break completely.

2. **SQLite configuration is inconsistent.** `pkg/events/store.go` correctly sets WAL mode, `NORMAL` synchronous, and `_busy_timeout=5000`. But `internal/coldwine/storage/db.go` opens SQLite with zero pragmas (no WAL, no busy timeout), and `internal/pollard/state/db.go` sets WAL but omits `_busy_timeout` and `_synchronous`. This means Coldwine is vulnerable to write contention errors and Pollard may stall under concurrent access.

3. **No unified timeout/cancellation policy.** Timeouts are ad-hoc: 5s shutdown contexts in `cmd/bigend/main.go`, 30s HTTP client in `pkg/autarch/client.go`, 500ms health check in `internal/intermute/manager.go`, configurable synthesizer timeout in Pollard. Some code paths use `context.Background()` with no timeout at all (e.g., the Intermute client REST methods inherit whatever context the caller passes, but callers are inconsistent). There is no resource cleanup pattern -- `Close()` exists on the Intermute client but no `defer` enforcement or connection pool limits on SQLite.

## Proposed Solution

### 1. Intermute client: graceful degradation

- Add a `WithTimeout(d time.Duration)` client option that wraps every REST call context with a deadline (default: 10s).
- Add `WithRetry(maxAttempts int, backoff time.Duration)` for idempotent GET operations.
- Make `NewClient` succeed even when `INTERMUTE_URL` is empty -- return a no-op client that returns `ErrOffline` from all methods. Callers can check `client.Available()` before making optional calls.
- Add a `Ping(ctx) error` method for health checking.

### 2. SQLite: unified open helper

- Create `pkg/db/open.go` with `Open(path string) (*sql.DB, error)` that enforces:
  - `_journal_mode=WAL`
  - `_synchronous=NORMAL`
  - `_busy_timeout=5000`
  - `SetMaxOpenConns(1)` for writers (SQLite best practice)
  - `SetConnMaxLifetime(0)` (reuse forever)
- Migrate `pkg/events/store.go`, `internal/coldwine/storage/db.go`, and `internal/pollard/state/db.go` to use this helper.

### 3. Timeout/cancellation policy

- Document a policy: CLI commands get a top-level context from `signal.NotifyContext` (already done in `cmd/` mains). All internal operations must accept and respect `context.Context`.
- Add a `pkg/timeout` package with named duration constants: `HTTPDefault = 10s`, `DBWrite = 5s`, `Shutdown = 5s`, `WSReconnect = 30s`.
- Audit and fix callers that use `context.Background()` without a deadline outside of `main()` -- specifically `pkg/events/writer.go:52` and any Intermute bridge paths.
- Ensure all `*sql.DB` handles are closed on shutdown via `defer` in the owning component.

## Acceptance Criteria

- [ ] Autarch tools start and run basic operations when `INTERMUTE_URL` is unset (graceful degradation, no panic/fatal).
- [ ] All SQLite databases open with WAL mode, `_synchronous=NORMAL`, and `_busy_timeout=5000`.
- [ ] `pkg/db/open.go` exists and all three current `sql.Open` call sites use it.
- [ ] Intermute REST calls have a default 10s timeout that can be overridden.
- [ ] No `context.Background()` without timeout outside of `main()` entry points.
- [ ] `go vet ./...` and `go test ./...` pass.

## Key Files

| File | Change |
|------|--------|
| `pkg/intermute/client.go` | Add timeout wrapping, no-op mode, `Ping()`, retry for GETs |
| `pkg/db/open.go` | **New** -- unified SQLite open helper |
| `pkg/events/store.go` | Use `pkg/db` helper |
| `internal/coldwine/storage/db.go` | Use `pkg/db` helper |
| `internal/pollard/state/db.go` | Use `pkg/db` helper |
| `pkg/events/writer.go` | Replace bare `context.Background()` with caller-provided context |
| `pkg/timeout/timeout.go` | **New** -- named timeout constants |
