---
title: "Architecture Review: iv-gax dashboard file fallback implementation"
date: 2026-02-23
reviewer: flux-drive fd-architecture
commit_range: diff at /tmp/qg-diff-1771832098.txt
---

### Findings Index

| SEVERITY | ID | Section | Title |
|----------|----|---------|-------|
| MUST-FIX | A1 | Boundaries & Coupling | internal/ implementation imports two sibling internal/ packages, violating the pkg/internal contract |
| MUST-FIX | A2 | Boundaries & Coupling | Fallback state mutation is not concurrency-safe |
| MUST-FIX | A3 | Pattern Analysis | Duplicated status-mapping function with acknowledged drift risk |
| CAUTION   | A4 | Simplicity & YAGNI | probeClient field is allocated but never used |
| CAUTION   | A5 | Boundaries & Coupling | Get* and mutation methods fail silently in fallback mode, creating a split-brain UX |
| CAUTION   | A6 | Pattern Analysis | Synthetic ID from prd.Version breaks cross-boundary identity assumptions |
| INFO      | A7 | Pattern Analysis | mapInsightToAutarch is lossy by design but the lossy fields are not surfaced to the caller |

Verdict: needs-changes

---

### Summary

The fallback pattern is architecturally sound in intent and fits the project's documented "graceful degradation" principle. The DataSource interface placement in pkg/autarch is correct — it keeps the abstraction in the shared layer where the Client lives. The fatal coupling problem is that the implementation in internal/autarch/local/ directly imports two other internal/ subtrees (internal/gurgeh/specs and internal/pollard/insights), which means a presentation-layer fallback adapter is now load-bearing for two domain packages that were previously independent of it. The implementation also carries a data race in the state-transition boolean, an unused allocated field, and a duplicated status mapping that is documented as a drift risk with no enforcement mechanism. These are fixable without a structural rewrite.

---

### Issues Found

**A1. MUST-FIX: internal/ fallback implementation couples three previously independent internal/ subtrees.**

`internal/autarch/local/source.go` imports `internal/gurgeh/specs` and `internal/pollard/insights` directly. Per Go conventions and this project's own documented convention ("Use `internal/` for tool-specific, `pkg/` for shared code"), internal packages are opaque to one another. This creates a new dependency arc: `internal/tui` -> (via cmd) `internal/autarch/local` -> `internal/gurgeh/specs` + `internal/pollard/insights`. The Coldwine/Tandemonium DB path is hardcoded inside the same file. If `specs.LoadAllPRDs` or `insights.LoadAll` change their signatures or loading contract, the fallback adapter breaks. The correct boundary is to move the reading logic into thin `Adapter` types owned by each domain package (or into pkg/) and have LocalSource call those. Smallest viable change: expose a `specs.DataSourceAdapter(projectPath) autarch.DataSource` function from each domain package, keeping the import direction domain -> shared instead of shared-fallback -> domain.

Evidence: `/home/mk/projects/Demarch/apps/autarch/internal/autarch/local/source.go` lines 11-16:
```go
import (
    "github.com/mistakeknot/autarch/internal/gurgeh/specs"
    "github.com/mistakeknot/autarch/internal/pollard/insights"
    "github.com/mistakeknot/autarch/pkg/autarch"
    autarchdb "github.com/mistakeknot/autarch/pkg/db"
)
```

**A2. MUST-FIX: fallbackActive write is not protected against concurrent callers.**

`Client.tryFallback` sets `c.fallbackActive = true` without a mutex. The Client is distributed to multiple views (`internal/tui/views/bigend.go`, `coldwine.go`, `gurgeh.go`, `pollard.go`, `signals.go`) which issue List* calls. If two views call a List* method simultaneously at first-dial-error, both enter `tryFallback`, both read `c.fallbackActive == false`, both set it to `true`, and both proceed to call the fallback — a harmless double-read in this case, but the unsynchronized write-then-read on `fallbackActive` is a data race that `go test -race` will flag. CLAUDE.md explicitly requires `-race` compliance. Smallest fix: replace `fallbackActive bool` with `sync/atomic.Bool` or guard the check-and-set in `tryFallback` with a `sync.Mutex`.

Evidence: `/home/mk/projects/Demarch/apps/autarch/pkg/autarch/client.go` lines 81-90, combined with the fact that all four dashboard views hold the same `*Client` pointer (confirmed at `internal/tui/unified_app.go:548`).

**A3. MUST-FIX: mapPRDStatusToSpecStatus is duplicated and acknowledged to drift.**

`internal/autarch/local/source.go:271` and `internal/gurgeh/intermute/sync.go:76` both define `mapPRDStatusToSpecStatus` mapping the same `specs.PRDStatus` enum to two different but structurally identical status types (`autarch.SpecStatus` and `intermute.SpecStatus`). The comment in local/source.go says "Mirrors the mapping in internal/gurgeh/intermute/sync.go" — acknowledging the duplication but providing no enforcement. When a new PRD status is added to `specs.PRDStatus`, it must be updated in both places. The duplication exists because `autarch.SpecStatus` and `intermute.SpecStatus` are different types even though they carry the same string values. The fix is to consolidate status values into `pkg/autarch` (the shared boundary already defines `SpecStatus`) and have `internal/gurgeh/intermute/sync.go` import from there, eliminating the second copy. Alternatively, expose a single mapping function in the `specs` package itself that targets `autarch.SpecStatus`.

Evidence: `/home/mk/projects/Demarch/apps/autarch/internal/autarch/local/source.go:269-282` vs `/home/mk/projects/Demarch/apps/autarch/internal/gurgeh/intermute/sync.go:76-94`.

**A4. CAUTION: probeClient is allocated in WithFallback but never read.**

`Client.probeClient` is created at `pkg/autarch/client.go:50` (`c.probeClient = &http.Client{Timeout: 2 * time.Second}`) inside `WithFallback`, but grep finds no other reference to `probeClient` in any method body. It is struct dead weight that signals an incomplete design (presumably intended for an eager connectivity probe before the first List* call, or for recovery polling). Either implement the intended probe or remove the field. As-is it misleads future readers about what the client does.

Evidence: `/home/mk/projects/Demarch/apps/autarch/pkg/autarch/client.go:27` (declaration) and `:50` (only assignment). No other uses found.

**A5. CAUTION: Get* and mutation methods fail silently in fallback mode, creating a split-brain UX.**

`DataSource` only covers List* operations. `GetSpec`, `GetEpic`, `GetStory`, `GetTask`, `GetInsight`, `AssignTask`, `UpdateTask`, and all Create/Delete operations have no fallback path. When `fallbackActive == true`, any code that calls a Get* or mutating method will issue an HTTP request to a server that is known to be unreachable, receive a network error, and return that error to the caller — all while `InFallbackMode()` returns true and the footer shows `[offline]`. The dashboard views may only call List* today, but callers of `GetSpec` (e.g. detail views, export paths) will silently regress. The fix does not require extending DataSource — the minimum viable guard is: when `fallbackActive` is true, Get* methods should return a sentinel error (`ErrOffline`) rather than attempting a network call that will time out. This keeps failure modes visible.

Evidence: `/home/mk/projects/Demarch/apps/autarch/pkg/autarch/client.go:229-234` (`GetSpec` — no fallback guard) compared to `ListSpecs` at lines 237-253.

**A6. CAUTION: Synthetic ID from prd.Version breaks cross-boundary identity assumptions.**

`mapPRDToSpec` in `internal/autarch/local/source.go:253-265` sets `spec.ID = prd.Version` (comment: "Synthetic ID from version slug e.g. 'mvp', 'v1'"). The `autarch.Spec.ID` field is treated by the rest of the codebase as a UUID assigned by Intermute. Callers that store or compare Spec IDs obtained during fallback mode against IDs obtained from Intermute will find mismatches — the same PRD will have two different IDs depending on which mode the client is in. `GetSpec(id)` will not find fallback-sourced IDs because it routes to Intermute (see A5). This is a latent correctness bug if any caller captures and reuses Spec IDs across mode transitions. At minimum the discrepancy should be documented at the `autarch.Spec` type level, not only in the local mapper comment.

Evidence: `/home/mk/projects/Demarch/apps/autarch/internal/autarch/local/source.go:252-256` and the `pkg/autarch/source.go` interface definition which has no contract statement about ID stability.

**A7. INFO: mapInsightToAutarch lossy reduction is not surfaced to callers.**

`ListInsights` in fallback mode returns `autarch.Insight` structs with `Body` always empty and `SpecID` set only to the first element of `LinkedFeatures`. These are documented in the function comment but callers receive the same return type as from Intermute and cannot distinguish "Body not available" from "Body is empty string". The `DataSource` interface carries no signal about data completeness. This is acceptable for the current dashboard read-only display, but will become a problem if any caller branches on `Insight.Body != ""` or relies on `SpecID` filtering to be accurate. No change required now, but document it on the `DataSource` interface as a contract note.

Evidence: `/home/mk/projects/Demarch/apps/autarch/internal/autarch/local/source.go:211-232` (mapInsightToAutarch).

---

### Improvements

**I1. Add a DataSource contract note to pkg/autarch/source.go** — the interface currently has no doc comment beyond a single line; adding notes about ID stability guarantees, data completeness expectations, and what "status filtering" means in a read-only local context will prevent callers from making incorrect assumptions as the interface grows.

**I2. Consider an ErrOffline sentinel in pkg/autarch** — a typed error returned by the Client when `fallbackActive` is true and the called method is not supported by the fallback DataSource (Get*, mutations) gives callers a clean signal to disable write UI affordances rather than presenting a generic network error. This is the standard pattern for mode-aware clients in this codebase (cf. the nil-client no-op pattern in `pkg/intermute`).

**I3. Extract the Coldwine DB open+query logic into pkg/db or internal/coldwine/storage** — the raw SQL in `LocalSource.ListEpics/ListStories/ListTasks` duplicates schema knowledge that already lives in Coldwine's storage package. A `storage.ReadOnlyAdapter(dbPath) autarch.DataSource` function would keep schema ownership in the right module and reduce the amount of SQL that lives in the fallback adapter.
