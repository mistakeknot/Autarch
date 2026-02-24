---
agent: fd-code-quality
tier: 1
issues:
  - id: P0-1
    severity: P0
    section: "Concurrency Safety"
    title: "SprintState.Clone() does not deep-copy ExplorationResult map"
  - id: P0-2
    severity: P0
    section: "Concurrency Safety"
    title: "Subscription.Stream() can deadlock on full output channel"
  - id: P1-1
    severity: P1
    section: "Test Coverage"
    title: "Pollard has 9 test files for 70 source files (13% file coverage)"
  - id: P1-2
    severity: P1
    section: "Test Coverage"
    title: "Zero test files for all 13 Pollard hunter implementations"
  - id: P1-3
    severity: P1
    section: "Error Handling"
    title: "Mixed logging patterns: fmt.Fprintf(os.Stderr) vs slog vs log.Print"
  - id: P2-1
    severity: P2
    section: "Naming Consistency"
    title: "93 files still reference legacy tool names (Praude/Vauxhall/Tandemonium)"
  - id: P2-2
    severity: P2
    section: "Code Complexity"
    title: "advanceInternal has 4 levels of nested conditionals (120+ lines)"
  - id: P2-3
    severity: P2
    section: "Package Organization"
    title: "Untested shared packages in pkg/ (netguard, httpapi, contract, etc.)"
improvements:
  - id: IMP-1
    title: "Add ExplorationResult deep-copy to SprintState.Clone()"
    section: "Concurrency Safety"
  - id: IMP-2
    title: "Make Subscription.Stream() respect context on send"
    section: "Concurrency Safety"
  - id: IMP-3
    title: "Add unit tests for Pollard hunters (at minimum: GitHubScout, HackerNews, Arxiv)"
    section: "Test Coverage"
  - id: IMP-4
    title: "Standardize on slog for all non-main package logging"
    section: "Error Handling"
  - id: IMP-5
    title: "Extract advanceInternal draft-generation into a dedicated method"
    section: "Code Complexity"
verdict: needs-changes
---

## Summary

Autarch is an 823-file Go 1.24 monorepo with strong conventions documented in AGENTS.md: `fmt.Errorf("context: %w", err)` for errors, `log/slog` for logging, TDD for behavior changes, and `internal/` vs `pkg/` separation. The codebase broadly follows these conventions, with Coldwine being the standout for test discipline (138 test files for 105 source files). However, there are two P0 concurrency bugs (one in the arbiter's Clone method, one in the signals broker), a severe test coverage gap in Pollard (13% file coverage, zero hunter tests), inconsistent logging across packages, and significant legacy-name debt. The core architecture is sound -- the code reads like idiomatic Go with well-designed interfaces -- but the issues identified represent real correctness risks in production-path code.

## Section-by-Section Review

### 1. Concurrency Safety

The project shows concurrency awareness: `SprintState.Clone()` carefully deep-copies Sections, Conflicts, Findings, ResearchCtx, VisionContext, and ShapeOverrides. The orchestrator consistently uses Clone() for safe snapshots. The Broker uses `sync.Mutex` correctly.

However, two issues break this discipline:

**P0-1: ExplorationResult not deep-copied in Clone()**

In `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` lines 315-349, `SprintState.Clone()` copies `*s` by value, which gives the clone a shared reference to `ExplorationResult map[string]any`. The `SetExplorationResult()` method in orchestrator.go (line 252) mutates this map under the lock, but any previously-cloned state still shares the same map reference. This violates the documented contract: "The returned value is safe to read without locks" (orchestrator.go line 77).

The comment on line 347 explicitly marks `ScanArtifacts` as shared-immutable, but no such annotation exists for `ExplorationResult`, and it IS mutated via `SetExplorationResult()`.

**P0-2: Subscription.Stream() deadlock potential**

In `/root/projects/Autarch/pkg/signals/broker.go` lines 114-123:
```go
func (s *Subscription) Stream(ctx context.Context, out chan<- Signal) {
    for {
        select {
        case <-ctx.Done():
            return
        case sig := <-s.sub.ch:
            out <- sig  // <-- blocks if out is full, ignoring ctx
        }
    }
}
```
The `select` correctly checks context OR reads from the subscription channel. But the subsequent `out <- sig` is an unconditional send. If the `out` channel is full (or the consumer has stopped reading), this goroutine blocks forever -- even after the context is cancelled. The send should be wrapped in its own select with ctx.Done().

### 2. Test Coverage

The project's AGENTS.md mandates "TDD for behavior changes" and "Small unit tests over broad integration tests." Coverage varies dramatically by tool:

| Tool | Source Files | Test Files | File Ratio |
|------|-------------|------------|------------|
| Coldwine | 105 | 138 | 131% (excellent) |
| Gurgeh | 115 | 81 | 70% (good) |
| Bigend | 28 | 17 | 61% (acceptable) |
| Pollard | 70 | 9 | 13% (poor) |

**P1-1 and P1-2: Pollard test gap**

Pollard has 13 hunter implementations (`/root/projects/Autarch/internal/pollard/hunters/`) -- GitHubScout, HackerNews, Arxiv, CompetitorTracker, OpenAlex, PubMed, USDA, Legal, Economics, Wiki, Agent, Context7, Custom -- with zero test files. The hunters make real HTTP calls and parse real API responses. Without tests, regressions in API parsing go undetected.

Additionally, 61 of 70 Pollard source files have no corresponding test file. Major untested packages include: `hunters/` (13 files), `pipeline/` (3 files), `server/` (3 files), `scoring/` (2 files), `selector/` (3 files), `proposal/` (4 files), `cli/` (14 files).

**pkg/ test gaps**

Several shared packages lack test files entirely:
- `/root/projects/Autarch/pkg/netguard/bind.go` -- no tests for the security-critical local-only enforcement
- `/root/projects/Autarch/pkg/httpapi/envelope.go` -- no tests for the API response wrapper
- `/root/projects/Autarch/pkg/contract/` (3 files) -- no tests for cross-tool entity types
- `/root/projects/Autarch/pkg/signals/client.go` -- no tests for the signals client

The existing tests are well-written. The signals broker tests (`/root/projects/Autarch/pkg/signals/broker_test.go`) use helper functions (`testSignal`, `recvSignal` with `t.Helper()`) and test both happy paths and edge cases (overflow, drop counter). The arbiter tests (`/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator_test.go`) follow the project's pattern of test doubles (testResearchProvider) rather than mocks. The TUI tests (`/root/projects/Autarch/pkg/tui/shelllayout_test.go`) test both behavior (focus cycling) and rendering (sidebar visibility). These are the patterns that should be replicated in undertested areas.

### 3. Error Handling

AGENTS.md specifies: `fmt.Errorf("context: %w", err)` for errors, `log/slog` for logging.

**Consistent error wrapping** -- The codebase overwhelmingly follows the `fmt.Errorf("context: %w", err)` convention. Examples from persistence.go:
```go
return nil, fmt.Errorf("read state: %w", err)
return nil, fmt.Errorf("unmarshal state: %w", err)
```
This is correct and consistent throughout the arbiter, persistence, coldwine storage, and signals packages.

**Sentinel errors used correctly** -- `ErrBlocker` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` line 22 follows Go convention with `errors.New()` + `IsBlockerError()` using `errors.Is()`.

**P1-3: Mixed logging patterns**

Despite AGENTS.md specifying `log/slog`, three different logging patterns coexist:

1. **`fmt.Fprintf(os.Stderr, ...)`** -- 29 occurrences in 9 files. Notably, the orchestrator uses this for all warning output (orchestrator.go lines 162, 182, 996, 1023, 1070). This bypasses slog entirely.

2. **`slog.Info/Warn/Error/Debug`** -- 70 occurrences in 10 files. Concentrated in bigend and exploration packages. This is the documented convention.

3. **`log.Print/Fatal`** -- 12 occurrences in 11 files. Scattered across CLI entry points and some internal packages.

The `fmt.Fprintf(os.Stderr)` pattern in orchestrator.go is particularly inconsistent because the same file imports no logging package at all -- it manually formats warnings to stderr instead of using `slog.Warn()` with structured fields.

### 4. Naming Consistency

**Package naming** follows Go conventions well: lowercase, short, no underscores. The `internal/{tool}/` vs `pkg/` split is respected. Interface naming uses Go-standard patterns (`Hunter`, `QuickScanner`, `ResearchProvider`).

**Type naming** is consistent: `PascalCase` for exports, `camelCase` for unexported. Struct fields follow the same pattern. JSON/YAML tags use `snake_case` throughout.

**P2-1: Legacy tool name debt**

93 files reference legacy tool names:
- "Praude" (old name for Gurgeh): `PraudemapFeature`, `PraudemapVersion`, `Praudemap`, `praudemap.yaml`, `LoadPraudemap()` in `/root/projects/Autarch/internal/gurgeh/specs/praudemap.go`
- "Tandemonium" (old name for Coldwine): `tandemonium-db-` in `/root/projects/Autarch/internal/coldwine/storage/db.go` line 26
- "Vauxhall" (old name for Bigend): `VAUXHALL_PORT`, `VAUXHALL_SCAN_ROOTS` in env vars

CLAUDE.md states "Legacy tool names (Vauxhall/Praude/Tandemonium) still work via aliases," so these are intentionally preserved for backward compatibility. However, the internal type names like `PraudemapFeature` and the temp dir prefix `tandemonium-db-` serve no compatibility purpose -- they are purely internal naming that could confuse new contributors. The legacy names should be limited to CLI aliases and environment variable names, not internal types.

### 5. Code Complexity

**P2-2: advanceInternal complexity**

The `advanceInternal` method in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (lines 307-426) contains a deeply nested conditional tree for draft generation:

```
if content != "" {
    // use cached
} else if state.ExplorationResult != nil {
    // try context-aware generation
    if err != nil {
        // try full exploration
        if err != nil {
            // try template fallback
            if fallbackErr != nil {
                return error
            }
        }
    }
} else {
    if shouldUseDynamicGeneration {
        // try generation
        if err != nil {
            // try template
        }
    } else {
        // use template directly
    }
}
```

This is 4 levels deep with 6 assignment targets for `state.Sections[state.Phase]`. The repeated pattern of `SectionDraft{Content: ..., Status: DraftProposed, UpdatedAt: time.Now()}` appears 5 times. A `generateDraftForPhase(ctx, state)` method extracting this logic would flatten the nesting and make the strategy pattern explicit.

### 6. API Design and Interfaces

The interface design is strong:
- `Hunter` interface (`Name() + Hunt()`) is minimal and correct
- `QuickScanner` interface is a single method -- easy to test with stubs
- `ResearchProvider` interface allows swapping Intermute for test doubles
- The `httpapi.Envelope` provides a consistent API response wrapper

The `Orchestrator` uses constructor injection (`NewOrchestratorWithResearch`) and setter injection (`SetScanner`), which is slightly inconsistent but pragmatically reasonable -- scanner is optional, research is a mode switch.

### 7. Package Organization

The `internal/` vs `pkg/` split is clear and well-maintained. Each tool has its own subpackage tree: `cli/`, `tui/`, `storage/` (where applicable). The shared `pkg/tui/` package correctly abstracts layout, styles, and components.

**P2-3: Several pkg/ packages lack tests despite being shared dependencies:**

- `pkg/netguard/bind.go` -- security-critical (enforces local-only binding), zero tests
- `pkg/httpapi/envelope.go` -- used by every API server, zero tests
- `pkg/contract/` -- cross-tool entity types, zero tests
- `pkg/discovery/` -- project discovery (3 files), zero tests

These packages are dependencies of multiple tools and should have proportional test coverage.

### 8. Known Issues Verification

The documented pre-existing test failures are confirmed:
- `internal/coldwine/cli/commands` tests FAIL with mail-related errors (verified by running `go test ./internal/coldwine/cli/...`)
- The `docs/solutions` build failure (type assertion) was not tested as it is a documentation package

`go vet` passes cleanly on tested packages (`pkg/signals/`, `internal/gurgeh/arbiter/`).

## Issues Found

| ID | Severity | Section | Description |
|----|----------|---------|-------------|
| P0-1 | P0 | Concurrency Safety | `SprintState.Clone()` in `types.go:315` does not deep-copy `ExplorationResult map[string]any`. The orchestrator mutates this map via `SetExplorationResult()` while cloned states may hold a shared reference, violating the documented thread-safety contract. |
| P0-2 | P0 | Concurrency Safety | `Subscription.Stream()` in `broker.go:114` blocks on `out <- sig` without context check. If the consumer stops reading, this goroutine leaks indefinitely even after context cancellation. |
| P1-1 | P1 | Test Coverage | Pollard has 9 test files for 70 source files (13% coverage by file count). Compare to Coldwine at 131%. |
| P1-2 | P1 | Test Coverage | All 13 hunter implementations in `internal/pollard/hunters/` have zero test files. These make HTTP API calls and parse responses -- the most error-prone code path in the system. |
| P1-3 | P1 | Error Handling | Three competing logging patterns: `fmt.Fprintf(os.Stderr)` (29 uses in 9 files), `slog` (70 uses in 10 files), `log.Print` (12 uses in 11 files). AGENTS.md specifies slog. |
| P2-1 | P2 | Naming Consistency | 93 files reference legacy names (Praude/Vauxhall/Tandemonium). Internal types like `PraudemapFeature` and temp dir prefixes like `tandemonium-db-` serve no compatibility purpose. |
| P2-2 | P2 | Code Complexity | `advanceInternal()` in `orchestrator.go:307` has 120+ lines with 4-level nesting and 5 repeated `SectionDraft` construction sites. |
| P2-3 | P2 | Package Organization | Security-critical `pkg/netguard/` and API-critical `pkg/httpapi/` have zero tests despite being shared dependencies. |

## Improvements Suggested

| ID | Title | Section | Detail |
|----|-------|---------|--------|
| IMP-1 | Deep-copy ExplorationResult in Clone() | Concurrency Safety | Add `encoding/json` round-trip or recursive map copy for `ExplorationResult map[string]any` in `SprintState.Clone()`. The nested `map[string]any` values (phase data, summary fields) make shallow copy insufficient. This is a one-line category fix but critical for the thread-safety contract. |
| IMP-2 | Context-aware send in Subscription.Stream() | Concurrency Safety | Replace `out <- sig` with `select { case out <- sig: case <-ctx.Done(): return }` in `broker.go:120`. This prevents goroutine leaks when consumers abandon their channel. |
| IMP-3 | Hunter unit tests with httptest | Test Coverage | Add `_test.go` files for at minimum `GitHubScout`, `HackerNews`, and `ArxivHunter`. Use `httptest.NewServer` to mock API responses. Test: valid response parsing, error responses, rate limiting, empty results. This follows the project's existing pattern in `server_test.go`. |
| IMP-4 | Standardize on slog | Error Handling | Replace `fmt.Fprintf(os.Stderr, "warning: ...")` in orchestrator.go with `slog.Warn("message", "error", err)`. Replace `log.Print*` calls in non-main packages with slog equivalents. Keep `log.Fatal` only in `main()` functions. |
| IMP-5 | Extract draft generation from advanceInternal | Code Complexity | Move lines 357-422 of `orchestrator.go` into a `generateDraftForPhase(ctx, state) (*SectionDraft, error)` method. The three strategies (cached extraction, context-aware generation, full exploration) become a clear fallback chain instead of nested conditionals. |

## Overall Assessment

**Verdict: needs-changes**

The codebase demonstrates strong Go fundamentals: clean interfaces, proper error wrapping, careful concurrency handling (Clone methods, mutex discipline), and excellent test coverage in Coldwine and the arbiter. The shared TUI package and httpapi envelope show thoughtful design for reuse.

The two P0 issues are real correctness bugs. P0-1 (`ExplorationResult` shallow copy) could cause data corruption during concurrent sprint operations -- the exact class of bug the Clone pattern was built to prevent. P0-2 (`Stream` deadlock) is a goroutine leak that could accumulate over long-running server sessions.

**Top 3 changes for better consistency:**

1. **Fix the two P0 concurrency bugs** (IMP-1, IMP-2) -- both are small, targeted fixes that restore the safety contract the codebase already intends to uphold.

2. **Add Pollard hunter tests** (IMP-3) -- the 13% test coverage is an outlier that undermines the project's TDD discipline. HTTP API parsing is inherently fragile and the most valuable place for regression tests.

3. **Standardize logging on slog** (IMP-4) -- the three-way split between `fmt.Fprintf(os.Stderr)`, `slog`, and `log.Print` creates confusion about which to use when extending the codebase. The orchestrator alone has 5 instances of the wrong pattern.
