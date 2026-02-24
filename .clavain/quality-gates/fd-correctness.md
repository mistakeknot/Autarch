# Correctness Review: iv-gax Dashboard File Fallback

**Reviewer:** Julik (Flux-drive Correctness Reviewer)
**Date:** 2026-02-23
**Files reviewed:**
- `internal/autarch/local/source.go` (NEW)
- `pkg/autarch/client.go` (MODIFIED)
- `internal/autarch/local/source_test.go` (NEW)
- `pkg/autarch/client_test.go` (NEW)

---

## Invariants That Must Hold

1. **Fallback is session-sticky**: once activated, all subsequent reads go to local data without re-attempting HTTP. Prevents oscillation between stale local and live data.
2. **Fallback activates only on ECONNREFUSED, never on timeout or server errors**: switching to stale local data when the server is merely slow would cause data loss for the user.
3. **Local reads are read-only**: `LocalSource` never writes to any file or DB.
4. **Type mappings are consistent**: `FeatureRef → SpecID`, `work_tasks → Task`, `PRDStatus → SpecStatus` all use stable, documented mappings.
5. **Empty results return `[]T{}`, not `nil`**: callers may range over slices without nil checks.
6. **Timestamps are parsed from stored RFC3339 strings, not synthesized**: zero-value timestamps should not silently pass as "now".
7. **PRAGMA guard is safe against injection**: `hasTable` builds its SQL by string concatenation of caller-supplied table names.
8. **`rows.Err()` is checked after iteration**: rows.Next() returning false does not always mean "clean end of data".

---

## Findings Index

| SEVERITY | ID | Section | Title |
|---|---|---|---|
| HIGH | C-01 | Concurrency | `fallbackActive` is an unsynchronized shared flag |
| HIGH | C-02 | SQLite PRAGMA guard | SQL injection via table name in `hasTable()` |
| MEDIUM | C-03 | Error handling | `rows.Err()` not checked after scan loops |
| MEDIUM | C-04 | Type mapping | `mapPRDStatusToSpecStatus`: `PRDStatusApproved → SpecStatusResearch` is semantically wrong for the fallback use-case |
| LOW | C-05 | Error classification | `isDialError` returns false for `net.OpError{Op:"dial"}` with no `*os.SyscallError` wrapper — silently misses some dial failures |
| LOW | C-06 | Timestamp parsing | `time.Parse` errors are silently discarded with `_`; zero-time values propagate without warning |
| INFO | C-07 | Test coverage | `TestClient_FallbackOnDialError` relies on a real dial to localhost:19999 — not portable; may false-pass if something is listening |
| INFO | C-08 | Type mapping | `LinkedFeatures[0] → SpecID` takes first element without documentation that Intermute's `spec_id` is singular |

**Verdict: needs-changes**

---

## Summary

The fallback implementation is structurally sound and the intent is clear. Two issues require fixes before this can safely ship: an unsynchronized `fallbackActive` flag that will cause data races in Bubble Tea's concurrent model (HIGH), and a SQL injection vector in `hasTable()` via unquoted table-name concatenation (HIGH). Three medium/low issues involve silent error swallowing and an arguable status mapping mismatch. The test suite is comprehensive for the happy path but misses the race and the injection vector entirely.

---

## Issues Found

**C-01. HIGH: `fallbackActive` is an unsynchronized shared flag — data race under Bubble Tea concurrency**

`Client.fallbackActive` is a plain `bool` field. `tryFallback()` writes it without any lock; `InFallbackMode()` and the fallback-active guards in `ListSpecs`/`ListEpics`/`ListTasks`/`ListInsights` all read it without a lock. Bubble Tea issues commands from goroutines spawned by `tea.Cmd`; multiple concurrent dashboard tab refreshes can call different `List*` methods simultaneously.

Concrete interleaving that triggers the race:

1. Goroutine A calls `ListSpecs("")`. `fallbackActive == false`. Enters the HTTP path, gets ECONNREFUSED.
2. Goroutine B calls `ListEpics("")`. `fallbackActive == false` (not yet updated). Also enters the HTTP path, also gets ECONNREFUSED.
3. Goroutine A's `tryFallback()` sets `fallbackActive = true` and calls `c.fallback.ListSpecs(...)`.
4. Goroutine B's `tryFallback()` reads `fallbackActive` — value is undefined (torn read on some architectures). `isDialError` returns true again; `tryFallback` sets `fallbackActive = true` a second time, double-invoking the fallback. No practical harm here, but go test -race will flag this.
5. On any subsequent call the flag may or may not be visible to the reading goroutine, causing intermittent HTTP attempts after the session should be sticky-fallback.

This is a 3 AM wake-up: the race is benign in outcome most of the time (worst case: one extra HTTP attempt) but will fail `go test -race` and violates the project's explicit concurrency rule ("Run tests with `-race` flag", CLAUDE.md).

Fix: use `sync/atomic` for the flag:
```go
import "sync/atomic"

type Client struct {
    ...
    fallbackActive atomic.Bool
}

func (c *Client) InFallbackMode() bool { return c.fallbackActive.Load() }

func (c *Client) tryFallback(err error) bool {
    if c.fallback == nil || c.fallbackActive.Load() {
        return false
    }
    if isDialError(err) {
        c.fallbackActive.Store(true)
        return true
    }
    return false
}
```
The `List*` guards become `if c.fallbackActive.Load() && c.fallback != nil`. Note that `tryFallback` as written is also a check-then-act pattern: two goroutines can both observe `fallbackActive==false` and both proceed to the fallback. With `atomic.Bool` this race still exists but is race-detector-clean; if double-activation is a real concern (e.g., fallback has side effects on first call), use a `sync.Once` instead.

Files: `pkg/autarch/client.go` lines 26-27, 56-58, 81-90, and all `if c.fallbackActive &&` guards.

---

**C-02. HIGH: SQL injection via table name in `hasTable()` — PRAGMA table_info concatenation**

`hasTable` builds its query by direct string concatenation:
```go
rows, err := db.Query("PRAGMA table_info(" + table + ")")
```
`table` is caller-supplied. In the current call sites it is always a hardcoded literal (`"epics"`, `"stories"`, `"work_tasks"`), so there is no immediate exploit path. However:

- Any future refactor that passes a variable `tableName` through this function will introduce a SQL injection vector. SQLite's PRAGMA does not support parameterized arguments (`?`), so the mitigation must be an allowlist.
- The DB is opened with `PRAGMA journal_mode=WAL` and a busy timeout; an injected pragma (e.g., `"epics); DROP TABLE epics; --"`) can corrupt the database.

Fix: validate the table name against a closed allowlist before use:
```go
var allowedTables = map[string]bool{
    "epics": true, "stories": true, "work_tasks": true,
}

func (s *LocalSource) hasTable(db *sql.DB, table string) bool {
    if !allowedTables[table] {
        return false
    }
    rows, err := db.Query("PRAGMA table_info(" + table + ")")
    ...
}
```

File: `internal/autarch/local/source.go` lines 240-247.

---

**C-03. MEDIUM: `rows.Err()` is not checked after all scan loops**

All five scan loops in `source.go` follow the pattern:
```go
rows, err := db.Query(...)
if err != nil { return nil, err }
defer rows.Close()
for rows.Next() {
    if err := rows.Scan(...); err != nil { return nil, err }
    ...
}
// rows.Err() never checked
```

`rows.Next()` returns `false` both on end-of-data and on an iteration error. When the SQLite driver encounters an I/O error mid-scan, `rows.Next()` returns false and the error is stored in `rows.Err()`. Without the check, the code silently returns a partial (possibly truncated) result set with `nil` error, which will display as correct data in the dashboard with no visible indication of the problem.

Fix: add after each loop:
```go
if err := rows.Err(); err != nil {
    return nil, fmt.Errorf("scan rows: %w", err)
}
```

Affected: `ListEpics` (line ~91), `ListStories` (~137), `ListTasks` (~197).
File: `internal/autarch/local/source.go`.

---

**C-04. MEDIUM: `PRDStatusApproved → SpecStatusResearch` is a lossy and arguably wrong status mapping**

`mapPRDStatusToSpecStatus` maps:
```go
case specs.PRDStatusApproved:
    return autarch.SpecStatusResearch
case specs.PRDStatusInProgress:
    return autarch.SpecStatusValidated
case specs.PRDStatusDone:
    return autarch.SpecStatusArchived
```

The `autarch.SpecStatus` constants are `draft`, `research`, `validated`, `archived`. An "approved" PRD in Gurgeh means user-accepted and ready to execute — mapping it to `research` (which sounds like "we're still gathering data") misrepresents the state to any view that shows the status string. A user looking at the dashboard in fallback mode will see their approved PRD labeled as "research". This is not a data corruption risk but it is a correctness failure for UX and for any downstream status-filter logic that uses the `SpecStatus` enum.

The comment says "Mirrors the mapping in `internal/gurgeh/intermute/sync.go`" — that mapping should be verified to ensure `LocalSource` and the sync path stay in sync. If the mapping is intentional (Intermute uses different semantics), document why "approved in Gurgeh == research in Intermute" with a link to the design decision.

File: `internal/autarch/local/source.go` lines 270-284.

---

**C-05. LOW: `isDialError` misses dial errors not wrapped in `*os.SyscallError`**

```go
func isDialError(err error) bool {
    var opErr *net.OpError
    if !errors.As(err, &opErr) { return false }
    if opErr.Op != "dial" { return false }
    var sysErr *os.SyscallError
    if errors.As(err, &sysErr) {
        return errors.Is(sysErr.Err, syscall.ECONNREFUSED)
    }
    return false  // <-- falls through if no SyscallError
}
```

If a `*net.OpError` with `Op == "dial"` wraps an error that is not a `*os.SyscallError` (e.g., some custom dialer, or on an unusual platform where the error chain structure differs), the function returns `false` and fallback is never activated. The function is documented as "ECONNREFUSED specifically", so the intent is correct, but returning `false` when `sysErr` is absent is a silent no-op that could confuse future maintainers who add a custom transport. The comment should explicitly document the fall-through case.

File: `pkg/autarch/client.go` lines 63-77.

---

**C-06. LOW: RFC3339 parse errors silently produce zero-time values**

Throughout `source.go`, timestamp parsing is done as:
```go
e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
```
and in `mapPRDToSpec`:
```go
spec.CreatedAt, _ = time.Parse(time.RFC3339, prd.CreatedAt)
```

A malformed or missing timestamp silently yields a zero `time.Time`. The dashboard will show the Unix epoch (1970-01-01) as the creation date, which is misleading but not catastrophic. The test `TestLocalSource_ListSpecs` does verify the year is 2025, so the happy path is covered. The failure path (bad timestamp in a real file) is not tested. Consider logging a warning via `slog` when parse fails, rather than silently discarding the error, per the project convention "non-fatal errors log to stderr with `warning:` prefix".

Files: `internal/autarch/local/source.go` lines 88-89, 134-135, 194-195, 260-263.

---

## Improvements

**C-07. INFO: `TestClient_FallbackOnDialError` dials a real port — use a fake transport instead**

The test connects to `127.0.0.1:19999` expecting nothing to be listening. If something is in fact listening on that port (CI, another dev, another test in parallel), the test either false-passes (server returns 404 which is not a dial error, so fallback is not triggered, test fails with "not in fallback mode") or false-fails depending on the response. Replace with a `http.RoundTripper` mock that returns a `*net.OpError{Op:"dial", ...}` directly to make the test hermetic and fast.

File: `pkg/autarch/client_test.go` lines 82-108.

---

**C-08. INFO: Document `LinkedFeatures[0] → SpecID` as a known-lossy mapping**

`mapInsightToAutarch` takes `li.LinkedFeatures[0]` as `SpecID`. The `LinkedFeatures` field is `[]string` (a slice), but `autarch.Insight.SpecID` is a single string. This is lossy: an insight linked to multiple features will only surface under one spec in the fallback view. The existing comment acknowledges this for `Sources[]` but not for `LinkedFeatures`. Add a comment stating this is lossy and that the Intermute API resolves multi-feature links differently (or not, if unknown).

File: `internal/autarch/local/source.go` lines 304-307.
