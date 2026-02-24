# Autarch Cross-Module Code Quality Review

**Reviewer:** fd-code-quality (Code Quality Reviewer)
**Date:** 2026-02-07
**Scope:** Cross-module patterns affecting workflow reliability and maintainability
**Files examined:** ~50 source files across all 4 tools, pkg/, cmd/, event spine, intermute bridges, signals, storage

---

## 1. Executive Summary

The Autarch monorepo has strong foundational patterns: consistent graceful degradation for Intermute bridges (nil-client becomes no-op), well-structured event spine with SQLite + WAL, and good separation between `internal/` and `pkg/`. However, three systemic issues undermine workflow reliability:

1. **Legacy name entrenchment** -- `tandemonium`, `praude`, and `vauxhall` are not just aliases; they are load-bearing identifiers in production structs, JSON APIs, file-path lookups, and env vars. The migration command exists but the codebase itself was never migrated.
2. **context.TODO() in TUI goroutines** -- 24 instances in production code, with the most dangerous ones spawning cancellation-unaware goroutines from Bubble Tea `tea.Cmd` closures. These represent missing shutdown plumbing for the unified TUI.
3. **Silent error swallowing** -- `time.Parse` errors discarded in the event store (4 locations), `PublishFindings` silently continues on per-finding errors with no logging, and the event subscriber drops messages when the channel is full with no backpressure signal.

Overall code quality alignment: **acceptable with specific areas needing work**.

---

## 2. Error Handling Analysis

### 2.1 Project Convention

AGENTS.md declares: `Error handling: fmt.Errorf("context: %w", err)` with `log/slog` for structured logging. The intermute bridges follow this consistently. The event spine does not.

### 2.2 Silent Time-Parse Errors in Event Store

**Location:** `/root/projects/Autarch/pkg/events/store.go` lines 219, 242, 316; `/root/projects/Autarch/pkg/events/reconcile_store.go` line 55, 131

```go
e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
```

This appears in `Query()`, `GetByID()`, `Replay()`, and `GetCursor()`. If the stored timestamp format ever deviates from RFC3339Nano (e.g., from a manual insert, schema migration, or clock issue), `CreatedAt` silently becomes the zero time (January 1, year 1). Downstream consumers have no way to distinguish "event has no timestamp" from "timestamp was unparseable." This is a data integrity concern for the event spine, which is the cross-tool communication backbone.

**Severity:** Medium. Low probability (SQLite stores what the writer emits), but high impact if triggered -- events would sort incorrectly and time-based filters would silently exclude them.

### 2.3 PublishFindings Error Swallowing

**Location:** `/root/projects/Autarch/internal/pollard/intermute/publisher.go` lines 52-67

```go
func (p *Publisher) PublishFindings(ctx context.Context, findings []research.Finding) ([]intermute.Insight, error) {
    // ...
    for _, finding := range findings {
        insight, err := p.PublishFinding(ctx, finding)
        if err != nil {
            // Log but continue with other findings
            continue
        }
        results = append(results, insight)
    }
    return results, nil
}
```

The comment says "Log but continue" but there is no logging. The error is silently discarded. The function always returns `nil` error even if every single finding fails to publish. The caller has no way to know that 0 of N findings were published.

Compare with the Coldwine broadcaster pattern (`/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go`), which propagates individual errors to the caller. The inconsistency means Pollard integration failures are invisible while Coldwine failures are reported.

**Severity:** Medium. Research findings could fail to reach Intermute silently, breaking the Pollard-to-Gurgeh insight pipeline without any visible error.

### 2.4 Event Channel Backpressure

**Location:** `/root/projects/Autarch/pkg/events/writer.go` lines 122-126

```go
select {
case sub.Channel <- event:
default:
    // Channel full, skip
}
```

This is a defensible choice to prevent blocking the writer, but the skipped events are lost with no counter, log, or notification. A slow subscriber silently misses events. Combined with the `Subscription.closed` flag not being thread-safe (no mutex on the boolean), there is a potential race between `IsClosed()` and `Close()`.

**Severity:** Low for the dropped events (acceptable trade-off). Medium for the race condition on `Subscription.closed`.

### 2.5 Logging Convention Split

The project declares `log/slog` as the logging standard, and 70 call sites use it correctly. However, 12 call sites use `log.Printf` from the standard `log` package:

- `/root/projects/Autarch/pkg/events/writer.go:77`
- `/root/projects/Autarch/internal/bigend/daemon/server.go:84` (also says "Vauxhall" in the log message)
- `/root/projects/Autarch/cmd/coldwine/main.go:14`
- `/root/projects/Autarch/cmd/pollard/main.go:13`
- `/root/projects/Autarch/cmd/gurgeh/main.go:13`
- `/root/projects/Autarch/internal/pollard/research/coordinator.go:269`
- `/root/projects/Autarch/internal/coldwine/agent/loop.go:39`
- And 5 more

This makes log aggregation inconsistent -- `slog` output is structured JSON/text, `log.Printf` is unstructured. The `cmd/*/main.go` files and the event bridge are the most impactful because they run in every session.

---

## 3. context.TODO() Audit

24 total `context.TODO()` instances found in production code. Categorized by risk:

### 3.1 HIGH RISK -- TUI goroutines with no cancellation path (7 instances)

These create contexts inside `tea.Cmd` closures that launch blocking operations. When the user quits (`Ctrl+C`), Bubble Tea calls `p.Kill()` but these goroutines have no way to receive the cancellation signal.

| File | Line | Operation | Risk |
|------|------|-----------|------|
| `/root/projects/Autarch/internal/tui/views/sprint_view.go` | 90 | `orch.Start(context.TODO(), ...)` | Blocks on Claude CLI subprocess |
| `/root/projects/Autarch/internal/tui/views/sprint_view.go` | 112 | `orch.StartWithScan(context.TODO(), ...)` | Same |
| `/root/projects/Autarch/internal/tui/views/sprint_view.go` | 476 | `orch.ChatAcceptDraft(context.TODO())` | Same |
| `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go` | 89 | `context.WithCancel(context.TODO())` | View lifecycle context -- partially mitigated because it wraps with `WithCancel`, but the parent TODO means cleanup only happens if the cancel func is called explicitly |
| `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go` | 148, 156, 264 | Orchestrator Start/AcceptAndAdvance | Blocks on Claude CLI |

The sprint_view line 436 is a partial exception: it uses `context.WithCancel(context.TODO())` and stores the cancel func in `v.cancelChat`, which is called by `cancelStreaming()`. This is the correct pattern -- the others should follow it.

### 3.2 MEDIUM RISK -- HTTP/network calls without parent context (6 instances)

These use `context.WithTimeout(context.TODO(), ...)` which provides a deadline but no way to cancel from outside (e.g., on app shutdown).

| File | Line | Operation |
|------|------|-----------|
| `/root/projects/Autarch/internal/bigend/tui/pane.go` | 131 | MCP StartMCP call |
| `/root/projects/Autarch/internal/bigend/tui/pane.go` | 244 | Aggregator refresh |
| `/root/projects/Autarch/internal/bigend/tui/model.go` | 673 | Aggregator refresh |
| `/root/projects/Autarch/internal/bigend/tui/model.go` | 830 | Context-dependent refresh |
| `/root/projects/Autarch/internal/tui/views/signals.go` | 388 | Intermute WebSocket connect |
| `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` | 286 | Agent list refresh in goroutine |

These are mitigated by the timeout but represent leaked goroutines if the TUI exits before the timeout fires.

### 3.3 LOW RISK -- Defensive fallback or test-only (11 instances)

| File | Line | Why Low Risk |
|------|------|--------------|
| `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` | 783 | Nil-parent fallback in `withTimeoutOrCancel()` -- correct defensive pattern |
| `/root/projects/Autarch/pkg/events/writer.go` | 64 | Bridge forward when no forwardCtx set -- bounded by timeout |
| `/root/projects/Autarch/internal/gurgeh/cli/commands/interview.go` | 124, 198 | CLI command -- nil context guard, process exits after command |
| `/root/projects/Autarch/internal/pollard/cli/watch.go` | 64 | Same -- CLI fallback |
| `/root/projects/Autarch/internal/gurgeh/tui/model.go` | 616 | Legacy standalone TUI (deprecated) |
| `/root/projects/Autarch/pkg/events/reconcile_test.go` | 32, 44, 63, 96, 115, 127 | Test code |

### 3.4 Recommendation

The HIGH RISK instances all follow the same anti-pattern: a `tea.Cmd` closure captures no parent context. The fix is:

1. Store a `context.Context` (derived from a root cancellable context) on the view struct
2. Cancel it on view cleanup / `Ctrl+C` / app shutdown
3. Use it as the parent for all `tea.Cmd` closures

Sprint view line 436 already demonstrates the correct pattern. The others should be aligned.

---

## 4. Legacy/Dead Code Inventory

### 4.1 Load-Bearing Legacy Names

These are not just comments -- they are in struct fields, JSON tags, file system paths, and API responses.

**"Tandemonium" (old name for Coldwine):**

| Location | Type | Impact |
|----------|------|--------|
| `/root/projects/Autarch/internal/bigend/daemon/server.go:248` | Struct field + JSON tag | `HasTandemonium bool json:"has_tandemonium"` -- **API contract** |
| `/root/projects/Autarch/internal/bigend/daemon/projects.go:69,117-118` | File system scan | Scans for `.tandemonium` directory |
| `/root/projects/Autarch/internal/bigend/daemon/projects.go:73,133` | TODO comments | References `.tandemonium/state.db` |
| `/root/projects/Autarch/internal/coldwine/config/config.go:56` | File system fallback | Falls back to `.tandemonium/config.toml` |
| `/root/projects/Autarch/internal/coldwine/cli/root_test.go:51,54,82,89` | Tests | Create `.tandemonium` directories in tests |
| `/root/projects/Autarch/internal/coldwine/config/config_test.go:11,45,81` | Tests | Use `.tandemonium` paths |
| `/root/projects/Autarch/pkg/shell/projects.go:17` | Struct field | `HasTandemonium bool` |
| `/root/projects/Autarch/pkg/shell/projects.go:151` | TODO comment | References `.tandemonium` |

**"Praude" (old name for Gurgeh):**

| Location | Type | Impact |
|----------|------|--------|
| `/root/projects/Autarch/pkg/agenttargets/config.go:55` | File system fallback | Falls back to `.praude` for config |
| `/root/projects/Autarch/pkg/agenttargets/config_test.go:13,20` | Tests | Create `.praude` directories |
| `/root/projects/Autarch/pkg/plan/plan.go:49` | Comment | `Tool string // praude, pollard, tandemonium` |
| `/root/projects/Autarch/pkg/plan/plan_test.go:10,12,13,27,99,112,128,163,175,185` | Tests | Use "praude" as tool name |
| `/root/projects/Autarch/internal/gurgeh/specs/praudemap.go` | **Entire file** | Types `Praudemap`, `PraudemapFeature`, `PraudemapVersion` -- 156 lines |

**"Vauxhall" (old name for Bigend):**

| Location | Type | Impact |
|----------|------|--------|
| `/root/projects/Autarch/internal/bigend/daemon/server.go:84` | Log message | `"Vauxhall daemon starting on %s"` |
| `/root/projects/Autarch/AGENTS.md:404-405` | Documentation | `VAUXHALL_PORT`, `VAUXHALL_SCAN_ROOTS` env vars |
| Multiple docs files | Documentation | Reference VAUXHALL_* env vars |

### 4.2 Migration Command Exists But Is Incomplete

`/root/projects/Autarch/cmd/autarch/migrate.go` handles `.praude` -> `.gurgeh` and `.tandemonium` -> `.coldwine` directory renames, and has good test coverage. But the code that *reads* those directories was never updated to use the new names primarily -- it still scans for the old names (e.g., `projects.go:117` scans for `.tandemonium`, not `.coldwine`).

### 4.3 The `praudemap.go` File

`/root/projects/Autarch/internal/gurgeh/specs/praudemap.go` is 156 lines of production code with types named `Praudemap`, `PraudemapFeature`, `PraudemapVersion`. This is not backward compatibility -- the types and functions use the legacy name throughout. There is no `roadmap.go` or equivalent with modern names.

### 4.4 Stale TODO Comments

| File | Line | Content | Assessment |
|------|------|---------|------------|
| `/root/projects/Autarch/internal/coldwine/cli/commands/import.go` | 84 | `TODO: Actually persist to database` | The import command claims to import but prints a note saying "Database persistence coming soon" -- this is a **broken user-facing feature** |
| `/root/projects/Autarch/internal/bigend/daemon/projects.go` | 73 | `TODO: Load tasks from .tandemonium/state.db` | Returns empty array -- Bigend task view is always empty |
| `/root/projects/Autarch/internal/bigend/daemon/projects.go` | 133 | `TODO: Query .tandemonium/state.db for actual stats` | Returns zero stats |
| `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` | 390 | `TODO: Load recent activities` | Activities section is dead |
| `/root/projects/Autarch/internal/bigend/daemon/server.go` | 129 | `AgentCount: 0, // TODO: integrate with agent registry` | Agent count always zero |
| `/root/projects/Autarch/internal/bigend/daemon/sessions.go` | 174 | `_ = output // TODO: Parse and add existing sessions` | tmux sessions silently discarded |

The Bigend daemon has 5 separate TODO stubs that mean it returns hardcoded empty/zero data for tasks, activities, agents, and sessions. This makes the daemon essentially non-functional for its core purpose.

---

## 5. Pattern Consistency Matrix

### 5.1 Intermute Bridge Pattern

| Aspect | Gurgeh | Coldwine | Pollard | Bigend |
|--------|--------|----------|---------|--------|
| Has `internal/*/intermute/` package | Yes | Yes | Yes | **No** |
| Interface defined for Intermute dependency | `SpecManager` | `MessageSender`, `SessionManager` | `InsightCreator` | N/A |
| Nil-client graceful degradation | Yes | Yes | Yes | N/A |
| Error wrapping with `fmt.Errorf("context: %w")` | Yes | Yes | **Partial** (PublishFindings swallows) | N/A |
| Test coverage for bridge | Yes (`sync_test.go`) | Yes (3 test files) | Yes (`publisher_test.go`) | N/A |
| Intermute client import | `pkg/intermute` | `intermute/client` (external) | `pkg/intermute` | N/A |

**Notable inconsistency:** Coldwine imports the external `github.com/mistakeknot/intermute/client` directly (`ic "github.com/mistakeknot/intermute/client"`), while Gurgeh and Pollard go through the internal wrapper `pkg/intermute`. This means Coldwine's Intermute types are coupled to the external library while the others are decoupled.

**Bigend gap:** Bigend has no `internal/bigend/intermute/` package despite being the "mission control" that should aggregate data from Intermute. It uses the event spine (`pkg/events`) and the aggregator talks to Intermute indirectly through the event bridge, but there is no structured bridge like the other tools have.

### 5.2 Signal Emitter Pattern

| Aspect | Gurgeh | Coldwine | Pollard | Bigend |
|--------|--------|----------|---------|--------|
| Has `internal/*/signals/` package | Yes | Yes | Yes | **No** |
| Emitter struct | `Emitter{}` (stateless) | `Emitter{}` (stateless) | `Emitter{}` (stateless) | N/A |
| `NewEmitter()` constructor | Yes | Yes | Yes | N/A |
| Return type for single signal | `[]signals.Signal` | `*signals.Signal` (nullable) | `signals.Signal` (value) | N/A |
| `generateID()` function | Yes (duplicated) | Yes (duplicated) | Yes (duplicated) | N/A |

**Inconsistency:** The return type for individual signal generation varies across all three tools:
- Gurgeh: Returns `[]signals.Signal` (always a slice, possibly empty)
- Coldwine: Returns `*signals.Signal` (nil means no signal)
- Pollard: Returns `signals.Signal` (value type, always present when called)

This forces callers to handle three different patterns for the same conceptual operation.

**Duplication:** `generateID()` is copy-pasted identically in all three emitter packages:
- `/root/projects/Autarch/internal/gurgeh/signals/emitter.go:117-121`
- `/root/projects/Autarch/internal/coldwine/signals/emitter.go:59-63`
- `/root/projects/Autarch/internal/pollard/signals/emitter.go:50-54`

This belongs in `pkg/signals/` as a shared function.

### 5.3 Storage Pattern

| Aspect | Gurgeh | Coldwine | Pollard | Bigend |
|--------|--------|----------|---------|--------|
| Storage location | `.gurgeh/specs/` (YAML) | `.coldwine/` (SQLite) | `.pollard/` (YAML files) | `~/.autarch/events.db` (SQLite) |
| Storage package | `internal/gurgeh/specs/` | `internal/coldwine/storage/` (32 files) | Scattered: `insights/`, `sources/`, `patterns/`, `config/` | `pkg/events/` |
| Test coverage | 6 test files | 14 test files | 0 test files for YAML persistence | 2 test files |
| Error handling | Returns errors | Returns errors | Returns errors | Swallows time-parse errors |

**Pollard gap:** Pollard has 70 source files and only 10 test files total (vs Coldwine's 140 test files for 107 source files). The YAML Load/Save functions across `insights/`, `sources/`, `patterns/`, and `config/` have **zero test files**. These are the data persistence functions for research findings.

### 5.4 TUI View Pattern

| Aspect | Unified Views | Gurgeh Standalone | Bigend Standalone |
|--------|--------------|-------------------|-------------------|
| Uses `context.TODO()` | Yes (sprint_view, gurgeh_onboarding, signals) | Yes (model.go:616) | Yes (model.go:673, pane.go:131,244) |
| Follows `pkgtui.View` interface | Yes | Partially (ArbiterView does) | No (standalone `tea.Model`) |
| Chat panel integration | Yes | Yes (ArbiterView) | No |
| Test coverage | 16 test files for 32 source files | Varies | 6 test files for 28 source files |

---

## 6. Test Coverage Gaps

### 6.1 Critical Paths With No Tests

1. **Event store time parsing** -- No test verifies behavior when `time.Parse` fails. Given the silent `_ =` discard, corrupt timestamps would propagate silently.

2. **Pollard YAML persistence** -- `Load()`, `Save()`, `LoadAll()` functions in `insights/insight.go`, `sources/source.go`, `patterns/pattern.go` have zero test coverage.

3. **Bigend daemon task loading** -- `loadTaskStats()` and `GetTasks()` always return empty/zero. Not testable because the implementation is a stub, but there are no tests verifying even the stub behavior.

4. **Cross-tool event flow** -- No integration test verifies that a Gurgeh spec change -> event spine -> Bigend aggregator refresh works end-to-end. The event store tests and intermute bridge tests exist independently but the pipeline is untested.

5. **Coldwine import persistence** -- The `import.go` command has a TODO for database persistence. The dry-run path is tested but the actual import path is a no-op, and there is no test asserting that it actually fails or succeeds.

### 6.2 Test Ratio by Tool

| Tool | Source Files | Test Files | Ratio |
|------|-------------|------------|-------|
| Coldwine | 107 | 140 | 1.31 (excellent) |
| Gurgeh | 115 | 82 | 0.71 (good) |
| Bigend | 28 | 19 | 0.68 (adequate) |
| Pollard | 70 | 10 | **0.14 (poor)** |
| Unified TUI | 32 | 16 | 0.50 (adequate) |
| pkg/ | 75 | 32 | 0.43 (adequate) |

Pollard is a significant outlier. With 70 source files and 10 test files, most of the research pipeline (hunters, pipeline, scoring, insights persistence, proposals) is untested.

### 6.3 Arbiter Phase Tests

Per project memory: "Every test in `orchestrator_phase_test.go` calls `Advance()` -> `GeneratePhase()` -> `runClaude()`. They hang without a live Claude CLI." This means the arbiter sprint flow -- the primary user workflow -- has no tests that run in CI. Only `TestConfidenceTotalWeightedCorrectly` and `TestConfidenceConflictsReduceConsistency` are true unit tests.

---

## 7. Recommendations (Prioritized)

### P0 -- Workflow-Breaking

1. **Fix Coldwine import command** (`/root/projects/Autarch/internal/coldwine/cli/commands/import.go:84`). The non-dry-run path prints "Imported N tasks" but does not actually persist them. This is a lie to the user. Either wire up the database persistence or make the command clearly fail with "not yet implemented" instead of claiming success.

2. **Fix `PublishFindings` error handling** (`/root/projects/Autarch/internal/pollard/intermute/publisher.go:58-63`). Add `slog.Warn` for failed findings and return an error (or at minimum, the count of failures) so callers know the pipeline is broken.

### P1 -- Data Integrity

3. **Handle time-parse errors in event store** (`/root/projects/Autarch/pkg/events/store.go` lines 219, 242, 316; `reconcile_store.go` lines 55, 131). Either log a warning with `slog.Warn` or return the parse error. These are the event spine's data integrity layer.

4. **Add Pollard persistence tests**. At minimum, roundtrip tests for `insights.Load/Save`, `sources.Load/Save`, `patterns.Load/Save`. These are the research data pipeline's storage layer.

### P2 -- Reliability

5. **Thread context through TUI views**. The 7 HIGH RISK `context.TODO()` instances in sprint_view, gurgeh_onboarding, and arbiter_view should follow the pattern already established at sprint_view:436 -- store a cancellable context on the view struct, cancel it on cleanup.

6. **Extract `generateID()` to `pkg/signals/`**. Three identical copy-paste implementations is a maintenance risk. A single `signals.GenerateID()` function eliminates the duplication.

### P3 -- Technical Debt

7. **Complete the legacy name migration in code**. Priority order:
   - `internal/bigend/daemon/server.go:248` -- `HasTandemonium` JSON field (API contract, hardest to change)
   - `internal/bigend/daemon/projects.go` -- scan for `.coldwine` first, `.tandemonium` as fallback (like config.go already does)
   - `internal/gurgeh/specs/praudemap.go` -- rename types to `Roadmap`/`RoadmapFeature`/`RoadmapVersion`
   - `pkg/plan/plan.go:49` -- update comment
   - `internal/bigend/daemon/server.go:84` -- change "Vauxhall" to "Bigend" in log message
   - Tests using legacy names (lower priority, but misleading)

8. **Standardize logging to slog**. Replace the 12 `log.Printf` call sites with `slog.Info`/`slog.Warn`. Most impactful: `cmd/*/main.go` (3 files), `pkg/events/writer.go`, `internal/bigend/daemon/server.go`.

9. **Normalize Coldwine's Intermute import**. Switch from `ic "github.com/mistakeknot/intermute/client"` to the `pkg/intermute` wrapper for consistency with Gurgeh and Pollard.

10. **Add build tags for arbiter integration tests**. Separate tests that require a live Claude CLI from true unit tests, so CI can run the unit tests and skip the integration tests.

---

## Appendix: File Reference

Key files examined during this review:

| File | Relevance |
|------|-----------|
| `/root/projects/Autarch/internal/gurgeh/intermute/sync.go` | Gurgeh Intermute bridge |
| `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go` | Coldwine Intermute bridge |
| `/root/projects/Autarch/internal/coldwine/intermute/mapper.go` | Coldwine type mapper |
| `/root/projects/Autarch/internal/coldwine/intermute/sessions.go` | Coldwine session tracker |
| `/root/projects/Autarch/internal/pollard/intermute/publisher.go` | Pollard Intermute bridge |
| `/root/projects/Autarch/internal/gurgeh/signals/emitter.go` | Gurgeh signal emitter |
| `/root/projects/Autarch/internal/coldwine/signals/emitter.go` | Coldwine signal emitter |
| `/root/projects/Autarch/internal/pollard/signals/emitter.go` | Pollard signal emitter |
| `/root/projects/Autarch/pkg/events/store.go` | Event spine storage |
| `/root/projects/Autarch/pkg/events/writer.go` | Event spine writer |
| `/root/projects/Autarch/pkg/events/types.go` | Event type definitions |
| `/root/projects/Autarch/internal/bigend/daemon/projects.go` | Bigend project scanner |
| `/root/projects/Autarch/internal/bigend/daemon/server.go` | Bigend daemon HTTP server |
| `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` | Bigend state aggregator |
| `/root/projects/Autarch/internal/bigend/tui/model.go` | Bigend standalone TUI |
| `/root/projects/Autarch/internal/bigend/tui/pane.go` | Bigend TUI pane |
| `/root/projects/Autarch/internal/tui/views/sprint_view.go` | Unified sprint view |
| `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go` | Gurgeh onboarding view |
| `/root/projects/Autarch/internal/tui/views/signals.go` | Signals view |
| `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go` | Arbiter TUI view |
| `/root/projects/Autarch/internal/gurgeh/cli/commands/interview.go` | Gurgeh CLI interview |
| `/root/projects/Autarch/internal/gurgeh/tui/model.go` | Gurgeh standalone TUI (deprecated) |
| `/root/projects/Autarch/internal/pollard/cli/watch.go` | Pollard watch command |
| `/root/projects/Autarch/internal/coldwine/config/config.go` | Coldwine config with legacy fallback |
| `/root/projects/Autarch/internal/coldwine/cli/commands/import.go` | Coldwine import (broken) |
| `/root/projects/Autarch/internal/gurgeh/specs/praudemap.go` | Legacy "Praudemap" types |
| `/root/projects/Autarch/pkg/plan/plan.go` | Plan types with legacy comments |
| `/root/projects/Autarch/pkg/contract/types.go` | Shared contract types |
