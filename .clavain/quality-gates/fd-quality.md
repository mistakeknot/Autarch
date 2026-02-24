# Quality Review: iv-gax Dashboard File Fallback

Reviewer: Flux-drive Quality & Style Reviewer
Date: 2026-02-22
Scope: pkg/autarch/source.go, internal/autarch/local/source.go, internal/autarch/local/source_test.go, pkg/autarch/client.go, pkg/autarch/client_test.go, cmd/autarch/main.go

---

## Findings Index

| SEVERITY | ID | Section | Title |
|---|---|---|---|
| MEDIUM | F01 | local/source.go | rows.Err() never checked after iteration in all four SQL list methods |
| MEDIUM | F02 | local/source.go | SQL string built with unquoted table name in hasTable — SQL injection risk |
| LOW | F03 | local/source.go | Repeated DB open/close on every call — no reuse across a single request burst |
| LOW | F04 | pkg/autarch/client.go | probeClient field allocated but never used |
| LOW | F05 | local/source.go | mapPRDStatusToSpecStatus maps PRDStatusApproved to SpecStatusResearch, contradicting comment |
| LOW | F06 | pkg/autarch/client_test.go | TestClient_FallbackOnDialError has a port race — port 19999 may already be in use |
| INFO | I01 | pkg/autarch/client.go | fallbackActive/fallback guard is redundant: tryFallback already checks both |
| INFO | I02 | internal/autarch/local/source_test.go | Test helpers use 0755/0644 literal octal — project uses 0o-prefixed form elsewhere |

Verdict: needs-changes

---

## Summary

The fallback architecture is sound: the `isDialError` sentinel (ECONNREFUSED-only, not timeouts), session-sticky activation, and the five-method `DataSource` interface are all well-designed and clearly documented. The critical gap is that every SQL iterator in `local/source.go` skips `rows.Err()`, which is required by the `database/sql` contract and consistently applied everywhere else in this codebase. The `hasTable` helper concatenates an unquoted table name into a PRAGMA query; although PRAGMA is not vulnerable to classic injection, the pattern is unsafe to generalize and diverges from the parameterized style used in all other queries. Two supporting issues (unused `probeClient`, redundant guard checks) are minor cleanup items.

---

## Issues Found

**F01. MEDIUM: rows.Err() never checked after iteration in ListEpics, ListStories, ListTasks, and the implicit loop in ListInsights.**

`database/sql` explicitly requires checking `rows.Err()` after the `for rows.Next()` loop because scanner errors (network partition, WAL checkpoint failure, partial reads) are surfaced there, not inside `rows.Next()`. Every other SQL query in this codebase checks it: `internal/coldwine/storage/coordination.go`, `internal/coldwine/tui/task_loader.go`, `internal/gurgeh/signals/store.go`, etc. The omission means silently truncated results are returned as successful responses to the TUI.

Evidence — `internal/autarch/local/source.go:80–95` (ListEpics):
```go
for rows.Next() {
    // ...scan...
    result = append(result, e)
}
if result == nil {
    result = []autarch.Epic{}
}
return result, nil   // rows.Err() never consulted
```

Fix — add after each loop and before the nil-guard:
```go
if err := rows.Err(); err != nil {
    return nil, fmt.Errorf("list epics: %w", err)
}
```
Apply identically to ListStories (line 126–141) and ListTasks (line 183–200).

---

**F02. MEDIUM: hasTable builds a PRAGMA query by direct string concatenation of an unquoted table name.**

`internal/autarch/local/source.go:241`:
```go
rows, err := db.Query("PRAGMA table_info(" + table + ")")
```

The `table` argument comes from call sites that pass literal strings ("epics", "stories", "work_tasks"), so there is no live injection surface today. However PRAGMA does not support parameter placeholders, and the pattern introduces fragility: any future caller that passes a runtime-constructed name (e.g., from config) would be vulnerable. The Go sql driver does not quote identifiers automatically. SQLite itself will interpret `PRAGMA table_info(evil; DROP TABLE users)` as two statements in some driver configurations.

Minimal safe fix — use a quoted identifier:
```go
rows, err := db.Query("PRAGMA table_info(\"" + table + "\")")
```

Or validate against an allowlist:
```go
allowed := map[string]struct{}{"epics": {}, "stories": {}, "work_tasks": {}}
if _, ok := allowed[table]; !ok {
    return false
}
```

---

**F03. LOW: All four SQL-backed list methods open and immediately close a fresh sql.DB connection on every call.**

`openDB()` calls `autarchdb.Open()` and the caller defers `db.Close()`. In a TUI that refreshes dashboards every few seconds (all four views at startup), this creates a new connection pool, opens the WAL file, and tears it down for each list call. The existing codebase uses `autarchdb.OpenShared` for read-heavy paths (see the comment in `openDB` itself acknowledging this choice). A shared read-only connection initialized once in `NewLocalSource` would reduce file descriptor churn without complicating lifecycle, since `LocalSource` is already session-scoped (created once in `main.go`).

This is an improvement rather than a bug because SQLite in WAL mode handles concurrent opens, but it is worth addressing before the TUI's refresh ticker runs at high frequency.

---

**F04. LOW: probeClient is allocated by WithFallback but never read anywhere in the codebase.**

`pkg/autarch/client.go:50`:
```go
c.probeClient = &http.Client{Timeout: 2 * time.Second}
```

The field comment says "Short-timeout client for initial connection probes" but no code path calls `probeClient.Get(...)`. The current fallback detection happens lazily on the first real request error, which is a valid and simpler strategy. The field should either be deleted (if proactive probing was deferred) or the intent documented in a TODO. Dead struct fields mislead future readers about the activation model.

---

**F05. LOW: mapPRDStatusToSpecStatus maps PRDStatusApproved to SpecStatusResearch, which is semantically surprising and inconsistently commented.**

`internal/autarch/local/source.go:272–283`:
```go
case specs.PRDStatusApproved:
    return autarch.SpecStatusResearch
case specs.PRDStatusInProgress:
    return autarch.SpecStatusValidated
```

"Approved → Research" and "InProgress → Validated" are non-obvious mappings. The comment says "Mirrors the mapping in internal/gurgeh/intermute/sync.go", which justifies the values, but there is no compile-time link ensuring they stay in sync. If the sync.go mapping drifts, the fallback data will show different statuses than the online data. A shared constant or function in a common package (rather than two separate switch statements) would eliminate the divergence risk.

---

**F06. LOW: TestClient_FallbackOnDialError and TestClient_SessionStickyFallback use port 19999 without binding a listener first.**

`pkg/autarch/client_test.go:88, 118`:
```go
client := NewClient("http://127.0.0.1:19999")
```

The test assumes port 19999 is not in use, which is true in CI but can fail on developer machines running other services. The standard pattern in this codebase (e.g., `bigend/web` tests) is to use `:0` and obtain the actual port from the listener. Since the test specifically requires ECONNREFUSED (not EADDRINUSE), the correct approach is to bind a listener, capture the address, close the listener, then pass the address to the client. This guarantees the port was free at the moment it closed and will return ECONNREFUSED for new connections.

---

## Improvements

**I01. Redundant double-guard in List* methods — simplify to a single tryFallback check.**

In `pkg/autarch/client.go`, the fast-path check `if c.fallbackActive && c.fallback != nil` and the error-path `c.tryFallback(err)` (which internally checks `c.fallback == nil || c.fallbackActive`) both guard the same invariant. `tryFallback` already handles all cases. The fast path could be unified into a single pattern, reducing the surface area for future mismatches. This is cosmetic given correct behavior today.

---

**I02. Test helpers use bare octal literals (0755, 0644) — project uses 0o-prefixed form elsewhere.**

`internal/autarch/local/source_test.go:17, 29, 65, etc.` use `0755` and `0644`. The rest of the test files in this repo (e.g., `internal/coldwine/storage/`) consistently use Go 1.13+ `0o755` / `0o644` syntax. Not a correctness issue, but inconsistent with prevailing local style.
