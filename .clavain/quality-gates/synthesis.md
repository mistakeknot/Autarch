## Synthesis Report

**Context:** 7 files changed across Go. Risk domains: data-access (SQLite), fallback-logic (network error detection), TUI rendering (offline badge). Changes implement a DataSource interface with HTTPSource/LocalSource fallback pattern for Autarch dashboard views.

**Review Date:** 2026-02-23

**Agents Launched:** 4
**Agents Completed:** 4
**Agents Failed:** 0

**Overall Verdict:** `needs-changes` (P0: 4, P1: 5, P2: 5, IMP: 5)

---

## Verdict Summary

| Agent | Status | Summary |
|-------|--------|---------|
| fd-architecture | NEEDS_ATTENTION | 7 findings: 3 MUST-FIX (coupling, concurrency, duplication), 3 CAUTION, 1 INFO. Fallback pattern sound but load-bearing fallback adapter violates internal/ contract. |
| fd-correctness | NEEDS_ATTENTION | 8 findings: 2 HIGH (data race, SQL injection), 3 MEDIUM (rows.Err, status mapping, error classification), 3 LOW/INFO. Fallback logic correct but unsafe error handling. |
| fd-quality | NEEDS_ATTENTION | 8 findings: 2 MEDIUM (rows.Err, PRAGMA concatenation), 3 LOW (port race, status mapping, DB reuse), 2 INFO (redundant guards, octal literals). |
| fd-user-product | NEEDS_ATTENTION | 6 findings: 2 HIGH (badge ambiguity, silent write failures), 2 MEDIUM (silent activation, no escape hatch), 2 LOW (badge styling, staleness). Users cannot understand offline state or recover from it. |

---

## Findings by Severity

### P0/CRITICAL (Blocks Merge)

**C-02: SQL injection via table name in hasTable() — PRAGMA table_info concatenation**
- **Agents:** fd-correctness (HIGH), fd-quality (MEDIUM) — convergence 2/4
- **File:** `internal/autarch/local/source.go:240-247`
- **Summary:** `hasTable()` builds SQL by direct string concatenation of caller-supplied table names. While hardcoded today ("epics", "stories", "work_tasks"), future refactors with variable table names will introduce a SQL injection vector. SQLite PRAGMA does not support parameterized arguments, so mitigation must be an allowlist.
- **Fix:** Validate table names against closed allowlist before use or quote identifiers.
- **Severity:** HIGH — potential data corruption path, violates safe-by-default SQL pattern.

**A2: fallbackActive write is not protected against concurrent callers — Data race**
- **Agents:** fd-architecture (MUST-FIX), fd-correctness (HIGH) — convergence 2/4
- **File:** `pkg/autarch/client.go:81-90`
- **Summary:** `Client.fallbackActive` is a plain `bool` field. `tryFallback()` sets it without a mutex. Dashboard views issue List* calls concurrently via Bubble Tea commands, causing unsynchronized reads and writes. `go test -race` will flag this. CLAUDE.md explicitly requires `-race` compliance.
- **Fix:** Use `sync/atomic.Bool` or guard check-and-set with `sync.Mutex`.
- **Severity:** HIGH — violates project concurrency rules, will fail CI.

**U2: Mutating operations in fallback mode fail silently with generic network error**
- **Agents:** fd-user-product (HIGH) — convergence 1/4
- **File:** `pkg/autarch/client.go:218-265`
- **Summary:** `CreateSpec`, `UpdateSpec`, `DeleteSpec`, `UpdateTask`, and all write methods have no fallback guard. When `fallbackActive=true`, they attempt HTTP POST/PUT/DELETE against unreachable Intermute, receive ECONNREFUSED, and return raw `fmt.Errorf()`. Users receive no message like "Cannot create specs in offline mode". Write methods need an `ErrOffline` guard.
- **Fix:** Add early-return `checkWritable()` guard to all Create/Update/Delete/Link/Assign methods returning a typed `ErrFallbackReadOnly` when offline.
- **Severity:** HIGH — UX failure: users attempt actions, get opaque errors, have no recovery path.

**U1: [offline] badge is ambiguous — does not explain what is degraded or how to fix it**
- **Agents:** fd-user-product (HIGH) — convergence 1/4
- **File:** `internal/tui/unified_app.go:791-794`
- **Summary:** Badge text "[offline]" carries no context about which service is unavailable, what data is shown instead, or how to resolve it. No help entry exists. Developers unfamiliar with the fallback mechanism will not understand that read operations work from files or that they can start Intermute to fix it.
- **Fix:** Replace with "[offline: local files]", add `/help` entry explaining the state and recovery path, apply warning color styling.
- **Severity:** HIGH — users cannot understand the system state or take corrective action.

### P1/IMPORTANT (Should Fix)

**C-03: rows.Err() is not checked after all scan loops**
- **Agents:** fd-correctness (MEDIUM), fd-quality (MEDIUM) — convergence 2/4
- **File:** `internal/autarch/local/source.go:80-95, 126-141, 183-200`
- **Summary:** All five SQL iterator loops in `source.go` skip `rows.Err()`, which is required by the `database/sql` contract. When the SQLite driver encounters an I/O error mid-scan, `rows.Next()` returns false and the error is stored in `rows.Err()`. Without the check, code silently returns partial results with `nil` error, which display as correct data in the dashboard with no indication of truncation.
- **Fix:** Add after each loop: `if err := rows.Err(); err != nil { return nil, fmt.Errorf("scan error: %w", err) }`
- **Severity:** MEDIUM/HIGH — silent data loss in fallback mode is worse than no data.
- **Note:** This is duplicated as F01 in fd-quality; both reviewers flagged it independently.

**A1: internal/ fallback implementation couples three previously independent internal/ subtrees**
- **Agents:** fd-architecture (MUST-FIX) — convergence 1/4
- **File:** `internal/autarch/local/source.go:11-16`
- **Summary:** `LocalSource` directly imports `internal/gurgeh/specs` and `internal/pollard/insights`, violating Go's internal/ contract. Per project convention, internal packages are opaque to one another. Fallback adapter now load-bearing for two domain packages that were previously independent. Any signature change in those packages breaks the fallback adapter.
- **Fix:** Move reading logic into thin `Adapter` types owned by each domain package, or expose domain-owned factory functions that return `autarch.DataSource`. Smallest change: add `specs.DataSourceAdapter(projectPath) autarch.DataSource` functions in each domain package.
- **Severity:** MEDIUM/HIGH — architectural violation that couples previously independent layers.

**A3: mapPRDStatusToSpecStatus is duplicated and acknowledged to drift**
- **Agents:** fd-architecture (MUST-FIX), fd-correctness (MEDIUM), fd-quality (LOW) — convergence 3/4
- **File:** `internal/autarch/local/source.go:269-282` vs `internal/gurgeh/intermute/sync.go:76-94`
- **Summary:** Status mapping exists in two places. Comment acknowledges drift risk: "Mirrors the mapping in internal/gurgeh/intermute/sync.go". When a new PRD status is added to `specs.PRDStatus`, both copies must be updated. No enforcement mechanism.
- **Fix:** Consolidate into `pkg/autarch` (shared boundary) and have sync.go import from there. Or expose a single mapping function in `specs` package itself.
- **Severity:** MEDIUM — drift risk increases with codebase age; current mapping matches but has no enforcement.

**U4: Session-sticky fallback has no escape hatch — user cannot reconnect**
- **Agents:** fd-user-product (MEDIUM) — convergence 1/4
- **File:** `pkg/autarch/client.go:81-83`
- **Summary:** Once `fallbackActive=true`, `tryFallback()` returns early and every List call short-circuits to local source. User cannot trigger re-probe within session. If Intermute starts after TUI launch, user has no way to reconnect — must restart TUI. Expected recovery (start Intermute, use normally) is broken without restart.
- **Fix:** Add `/reconnect` command (or honor `ctrl+r`) that resets `fallbackActive` to false, attempts one List call, re-activates fallback only if it fails.
- **Severity:** MEDIUM — users can recover but must know to restart TUI; poor discoverability.

### P2/IMPORTANT (Should Fix)

**C-04: PRDStatusApproved → SpecStatusResearch is semantically wrong mapping**
- **Agents:** fd-correctness (MEDIUM), fd-quality (LOW) — convergence 2/4
- **File:** `internal/autarch/local/source.go:270-284`
- **Summary:** Status mapping: `PRDStatusApproved → SpecStatusResearch`, `InProgress → Validated`, `Done → Archived`. An "approved" PRD in Gurgeh means user-accepted and ready; mapping to "research" (still gathering data) misrepresents state to views and any downstream status-filter logic.
- **Fix:** Verify mapping against `internal/gurgeh/intermute/sync.go` intent. If mapping is intentional (Intermute uses different semantics), document the design decision with a link.
- **Severity:** MEDIUM — UX degradation in fallback mode; not data corruption.

**C-05: isDialError misses dial errors not wrapped in os.SyscallError**
- **Agents:** fd-correctness (LOW) — convergence 1/4
- **File:** `pkg/autarch/client.go:63-77`
- **Summary:** Function checks for `*net.OpError{Op:"dial"}` wrapping `*os.SyscallError{ECONNREFUSED}`. If error chain differs (custom dialer, unusual platform), function returns false and fallback is never activated. Silently returns false when `sysErr` is absent without comment.
- **Fix:** Explicitly document fall-through case in comment: "Returns false if Op is 'dial' but error is not wrapped in SyscallError".
- **Severity:** LOW — unlikely in practice; defensive documentation helps maintainers.

**C-06: RFC3339 parse errors silently produce zero-time values**
- **Agents:** fd-correctness (LOW) — convergence 1/4
- **File:** `internal/autarch/local/source.go:88-89, 134-135, 194-195, 260-263`
- **Summary:** Timestamp parsing done as `t, _ = time.Parse(time.RFC3339, str)`. Malformed or missing timestamps silently yield zero time (Unix epoch 1970-01-01). Dashboard displays epoch as creation date, misleading but not catastrophic. Test covers happy path but not failure path.
- **Fix:** Log warning via `slog` when parse fails rather than silently discarding error. Per project convention: non-fatal errors log to stderr with `warning:` prefix.
- **Severity:** LOW — rare edge case; better error visibility helps debugging.

**A4: probeClient field is allocated but never used**
- **Agents:** fd-architecture (CAUTION), fd-quality (LOW) — convergence 2/4
- **File:** `pkg/autarch/client.go:27, 50`
- **Summary:** `c.probeClient = &http.Client{Timeout: 2*time.Second}` inside `WithFallback`, but no code path calls it. Field comment says "Short-timeout client for initial connection probes" but current strategy uses lazy activation on first request error. Dead struct field misleads future readers about activation model.
- **Fix:** Delete field or add TODO documenting deferred proactive probing. Dead fields should not accumulate.
- **Severity:** MEDIUM — code clarity; prevents future confusion about activation strategy.

**F03: All SQL-backed list methods open and immediately close fresh sql.DB on every call**
- **Agents:** fd-quality (LOW) — convergence 1/4
- **File:** `internal/autarch/local/source.go` (ListEpics, ListStories, ListTasks, ListInsights)
- **Summary:** Each list call creates new connection pool, opens WAL file, closes immediately. `openDB()` calls `autarchdb.Open()` and caller defers `db.Close()`. TUI refresh ticker (few seconds) will create connection churn. Codebase uses `OpenShared` for read-heavy paths (comment in `openDB` acknowledges). Shared read-only connection in `NewLocalSource` would reduce FD churn.
- **Fix:** Initialize shared read-only connection in `NewLocalSource`, reuse across all list calls.
- **Severity:** MEDIUM/LOW — not a bug; improvement before refresh frequency increases.

### IMP/Nice-to-Have (Optional Improvements)

**A5: Get* and mutation methods fail silently in fallback mode, creating split-brain UX**
- **Agents:** fd-architecture (CAUTION) — convergence 1/4
- **File:** `pkg/autarch/client.go:229-253`
- **Summary:** `DataSource` only covers List* operations. `GetSpec`, `GetEpic`, `GetStory`, `GetTask`, `GetInsight`, and mutations have no fallback path. When offline, these methods issue HTTP calls against unreachable server, receive network errors, return to caller while `InFallbackMode()` returns true and footer shows `[offline]`. Dashboard may only call List* today, but detail views and export paths will silently regress.
- **Fix:** Guard Get* methods with early-return `ErrOffline` when offline, rather than attempting network call that will time out. Minimum viable: gives callers clean signal to disable write UI affordances.
- **Severity:** LOW/MEDIUM — guards Get* from silent failure, improves UX consistency.

**A6: Synthetic ID from prd.Version breaks cross-boundary identity assumptions**
- **Agents:** fd-architecture (CAUTION) — convergence 1/4
- **File:** `internal/autarch/local/source.go:252-256`
- **Summary:** `mapPRDToSpec` sets `spec.ID = prd.Version` (comment: "Synthetic ID from version slug e.g. 'mvp', 'v1'"). Rest of codebase treats `Spec.ID` as UUID assigned by Intermute. Callers that store or compare IDs obtained during fallback against IDs from Intermute will find mismatches. `GetSpec(id)` will not find fallback-sourced IDs (no fallback path). Latent correctness bug if any caller captures and reuses IDs across mode transitions.
- **Fix:** Document at `autarch.Spec` type level (not only in local mapper comment) that ID stability is not guaranteed across sources. Or generate stable synthetic IDs from PRD file path + version.
- **Severity:** LOW — latent bug; no current caller reuses IDs across modes.

**A7: mapInsightToAutarch lossy reduction is not surfaced to callers**
- **Agents:** fd-architecture (INFO) — convergence 1/4
- **File:** `internal/autarch/local/source.go:211-232`
- **Summary:** `ListInsights` fallback returns `autarch.Insight` with `Body` always empty and `SpecID` set only to first element of `LinkedFeatures`. Documented in comment but callers receive same return type as Intermute and cannot distinguish "Body not available" from "Body is empty". `DataSource` interface carries no data-completeness signal. Acceptable for current read-only display; becomes problem if caller branches on `Insight.Body != ""`.
- **Fix:** No change required now. Document on `DataSource` interface as contract note: what data completeness guarantees apply per source.
- **Severity:** INFO — documentation only; current usage safe.

**U3: Fallback activation is silent — no in-session notification**
- **Agents:** fd-user-product (MEDIUM) — convergence 1/4
- **File:** `pkg/autarch/client.go:79-90`
- **Summary:** Fallback activates on first List ECONNREFUSED with no toast, status message, log pane event, or modal. Badge `[offline]` appears in low-attention footer zone. Active user scrolling may not notice. First signal of problem is failed write (U2), after user invested time in session.
- **Fix:** Emit `slog.Warn("intermute unreachable — switched to local files")` to log pane when activation occurs. Optionally emit `tea.Msg` for transient status display.
- **Severity:** MEDIUM — improves user awareness of state change.

**U5: [offline] badge is visually indistinguishable from surrounding footer text**
- **Agents:** fd-user-product (LOW) — convergence 1/4
- **File:** `internal/tui/unified_app.go:792-794`, `pkg/tui/styles.go:47-50`
- **Summary:** Badge appended as plain string to help text, rendered via `FooterStyle` (ColorMuted). No color differentiation, no bold. Tokyo Night palette has warning colors (amber, red) used elsewhere. Badge easy to miss in dense footer. Codebase conventions (confidence warnings in Arbiter view) use distinct colors for degraded states.
- **Fix:** Apply inline lipgloss style using existing warning/amber color before appending badge.
- **Severity:** LOW — improves visibility; non-functional workaround is scrolling right.

**U6: Data staleness is unquantified in the badge**
- **Agents:** fd-user-product (LOW) — convergence 1/4
- **File:** `internal/autarch/local/source.go:252-267`, `internal/tui/unified_app.go:793`
- **Summary:** Badge "[offline]" gives no indication of data age. Local `.gurgeh/specs/` and `.tandemonium/state.db` could be seconds or months old. Users have no idea when local data was last synchronized. Staleness critical for spec browser (decisions based on outdated data). `mapPRDToSpec()` parses `UpdatedAt` from PRD files — information available but not surfaced.
- **Fix:** Include timestamp in badge: "[offline — data from HH:MM]" or "[offline — specs from 2h ago]".
- **Severity:** LOW — informational; current read-only usage acceptable without it.

**F04: probeClient allocated but never read** (duplicate of A4)
- Covered under A4 above.

**F05: mapPRDStatusToSpecStatus duplication** (duplicate of A3)
- Covered under A3 above.

**F06: TestClient_FallbackOnDialError uses port 19999 without binding first** (related to C-07)
- **Agents:** fd-quality (LOW) — convergence 1/4
- **File:** `pkg/autarch/client_test.go:88, 118`
- **Summary:** Test assumes port 19999 unused. True in CI but can fail on dev machines running other services. Test specifically requires ECONNREFUSED (not EADDRINUSE). Standard pattern in codebase (bigend/web tests) is to use `:0` and obtain actual port from listener.
- **Fix:** Bind listener, capture address, close listener, pass address to client. Guarantees port free at moment it closed, returns ECONNREFUSED for new connections.
- **Severity:** LOW/MEDIUM — test brittleness; CI will pass but dev-machine failures possible.

**I01: Redundant double-guard in List* methods**
- **Agents:** fd-quality (INFO) — convergence 1/4
- **File:** `pkg/autarch/client.go` fast-path check vs error-path `tryFallback`
- **Summary:** Fast-path check `if c.fallbackActive && c.fallback != nil` and error-path `c.tryFallback(err)` (which internally checks same invariant) both guard same condition. `tryFallback` already handles all cases. Could unify into single pattern, reducing surface area for mismatches.
- **Fix:** Simplify to single pattern (cosmetic; behavior already correct).
- **Severity:** INFO — code clarity only.

**I02: Test helpers use bare octal literals**
- **Agents:** fd-quality (INFO) — convergence 1/4
- **File:** `internal/autarch/local/source_test.go:17, 29, 65`
- **Summary:** Use `0755` and `0644` instead of Go 1.13+ `0o755` / `0o644`. Rest of repo consistently uses 0o-prefixed form. Not a correctness issue, but inconsistent with local style.
- **Fix:** Use `0o755`, `0o644` for consistency.
- **Severity:** INFO — style only.

**C-07: TestClient_FallbackOnDialError relies on real dial to localhost:19999**
- **Agents:** fd-correctness (INFO) — convergence 1/4
- **File:** `pkg/autarch/client_test.go:82-108`
- **Summary:** Test connects to `127.0.0.1:19999` expecting nothing listening. If something is listening (CI collision, another dev, parallel test), test false-passes (server returns 404 which is not dial error, so fallback not triggered, test fails with "not in fallback mode") or false-fails depending on response. Replace with `http.RoundTripper` mock that returns `*net.OpError{Op:"dial"}` directly — hermetic and fast.
- **Fix:** Use mock RoundTripper instead of real port dial.
- **Severity:** LOW/MEDIUM — test brittleness; already covered under F06.

**C-08: Document LinkedFeatures[0] → SpecID as known-lossy mapping**
- **Agents:** fd-correctness (INFO) — convergence 1/4
- **File:** `internal/autarch/local/source.go:304-307`
- **Summary:** `mapInsightToAutarch` takes `li.LinkedFeatures[0]` as SpecID. LinkedFeatures is slice but SpecID is single string — lossy. Insight linked to multiple features only surfaces under one spec in fallback view. Comment acknowledges lossiness for `Sources[]` but not for `LinkedFeatures`.
- **Fix:** Add comment documenting lossy multi-feature mapping and how Intermute API differs.
- **Severity:** INFO — documentation only.

---

## Conflict Matrix

**Convergence on High-Risk Issues (2+ agents flagged):**
- `fallbackActive` data race: fd-architecture, fd-correctness agree (both MUST-FIX/HIGH)
- SQL injection in hasTable: fd-correctness, fd-quality agree (both HIGH/MEDIUM)
- rows.Err() missing: fd-correctness, fd-quality agree (both MEDIUM)
- mapPRDStatusToSpecStatus duplication: fd-architecture, fd-correctness, fd-quality agree (MUST-FIX/MEDIUM/LOW)
- probeClient dead field: fd-architecture, fd-quality agree (CAUTION/LOW)

**No significant conflicts detected.** All agents agree on the existence and severity of top-tier issues. Minor disagreements are severity level (e.g., probeClient cleanup is CAUTION vs LOW) and implementation details (e.g., atomic.Bool vs sync.Mutex for fallbackActive), which is expected across independent reviewers.

---

## Files

- **Agent Reports:**
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/fd-architecture.md` — Coupling, pattern, and API surface analysis
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/fd-correctness.md` — Safety, concurrency, SQL, type mapping verification
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/fd-quality.md` — Style, maintainability, resource usage
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/fd-user-product.md` — UX, error messaging, user recovery paths

- **This Synthesis:**
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/synthesis.md` — Full deduplication and verdict report (this file)
  - `/home/mk/projects/Demarch/apps/autarch/.clavain/quality-gates/findings.json` — Structured findings export

