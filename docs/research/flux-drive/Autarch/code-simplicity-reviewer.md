---
agent: code-simplicity-reviewer
tier: 3
issues:
  - id: P0-1
    severity: P0
    title: "Dual TUI implementations duplicate ~400 LOC of overlay/layout/run logic"
    section: "Dual TUI Implementations"
  - id: P0-2
    severity: P0
    title: "OnboardingOrchestrator is dead code -- never instantiated outside its own file"
    section: "Dead Code"
  - id: P1-1
    severity: P1
    title: "pkg/compound has zero importers -- 821 LOC of unreachable code"
    section: "Dead Code"
  - id: P1-2
    severity: P1
    title: "Custom string utilities reimplement stdlib (splitLines, trimSpace, etc.)"
    section: "Reinvented Wheels"
  - id: P1-3
    severity: P1
    title: "Triple type system for entities: contract, intermute, and events packages all define Status/Task/Epic"
    section: "Type Duplication"
  - id: P1-4
    severity: P1
    title: "Deprecated renderFooter method left in unified_app.go"
    section: "Dead Code"
  - id: P1-5
    severity: P1
    title: "38 files carry legacy Vauxhall/Praude/Tandemonium aliases"
    section: "Legacy Aliases"
  - id: P2-1
    severity: P2
    title: "cmd/testui and cmd/archviz are developer-only tools shipped as top-level binaries"
    section: "cmd/ Entry Points"
  - id: P2-2
    severity: P2
    title: "pkg/timeout is 3 constants that could live inline or in a single const block"
    section: "Over-Granular Packages"
  - id: P2-3
    severity: P2
    title: "internal/tui/view.go is 22 lines of type aliases for backward compatibility"
    section: "Unnecessary Abstraction"
  - id: P2-4
    severity: P2
    title: "OnboardingScanVision/ScanProblem/ScanUsers states declared but only referenced in two switch arms"
    section: "Dead Code"
  - id: P2-5
    severity: P2
    title: "pkg/intermute/types.go has ~400 LOC of to/from conversion boilerplate"
    section: "Type Duplication"
improvements:
  - id: IMP-1
    title: "Merge App into UnifiedApp -- eliminate ~465 LOC"
    section: "Dual TUI Implementations"
  - id: IMP-2
    title: "Delete OnboardingOrchestrator + tests -- 248+ LOC of dead code"
    section: "Dead Code"
  - id: IMP-3
    title: "Delete pkg/compound or move to docs/solutions -- 821 LOC with no callers"
    section: "Dead Code"
  - id: IMP-4
    title: "Replace custom splitLines/trimSpace/splitComma/containsComma with strings stdlib"
    section: "Reinvented Wheels"
  - id: IMP-5
    title: "Unify entity types: pick one canonical source (contract or intermute), alias the rest"
    section: "Type Duplication"
  - id: IMP-6
    title: "Move archviz and testui to internal/devtools or scripts"
    section: "cmd/ Entry Points"
  - id: IMP-7
    title: "Inline pkg/timeout constants at call sites"
    section: "Over-Granular Packages"
  - id: IMP-8
    title: "Extract shared overlay function to pkg/tui, remove two private copies"
    section: "Dual TUI Implementations"
  - id: IMP-9
    title: "Audit and remove legacy aliases from 38 files, replacing with canonical names"
    section: "Legacy Aliases"
  - id: IMP-10
    title: "Delete three unused OnboardingScan* states"
    section: "Dead Code"
verdict: needs-changes
---

## Summary

Autarch is a 144K LOC Go monorepo with genuine architectural substance -- four distinct tools, a shared TUI framework, cross-tool signaling, and an MCP server. However, it carries significant accumulated complexity that undermines maintainability. The most impactful issues are: (1) two parallel TUI implementations (`App` and `UnifiedApp`) that duplicate overlay logic, run scaffolding, and layout code; (2) triple entity type definitions across `pkg/contract`, `pkg/intermute`, and `pkg/events` with ~400 lines of mechanical conversion functions; (3) approximately 1,300 lines of confirmed dead code across `OnboardingOrchestrator`, `pkg/compound`, deprecated methods, and unused onboarding states. Fixing these would reduce cognitive load substantially while removing zero functionality.

## Section-by-Section Review

### Dual TUI Implementations

**Files:** `/root/projects/Autarch/internal/tui/app.go` (465 LOC), `/root/projects/Autarch/internal/tui/unified_app.go` (2,194 LOC)

`App` is the "skip-onboard" path; `UnifiedApp` handles onboarding and then transitions to a dashboard that does the same thing `App` does. Both contain:

- Identical `overlay()` and `insertAt()` methods (lines 353-406 in app.go, lines 2103-2155 in unified_app.go) -- 53+53 = 106 lines duplicated verbatim, differing only in receiver type vs. package-level function.
- Identical `SetInlineMode()`, `LogPane()`, `SetInitialTab()` methods.
- Parallel `Run()`/`RunWithOpts()` and `RunUnified()`/`RunUnifiedWithOpts()` functions (lines 415-452 in app.go, lines 2158-2194 in unified_app.go) with identical log handler setup, program creation, and inline-mode log dump.
- Separate `updateCommands()` implementations that build the same palette commands.

This is the project memory's known debt item "Two TUI implementations that need merging." The `App` type is only used from `cmd/autarch/main.go` when `--skip-onboard` is passed. `UnifiedApp` already has a dashboard mode that does exactly what `App` does. `App` could be eliminated by having `UnifiedApp` start in `ModeDashboard` when onboarding is skipped.

### Dead Code

1. **`OnboardingOrchestrator`** (`/root/projects/Autarch/internal/tui/onboarding.go` lines 111-247): Defined as a tea.Model with Init/Update/View/SetCallbacks/Complete. `NewOnboardingOrchestrator()` is never called outside `onboarding.go` and `onboarding_test.go`. The actual onboarding flow is handled directly by `UnifiedApp`. This is 137 lines of unreachable production code.

2. **`pkg/compound`** (`/root/projects/Autarch/pkg/compound/`): 821 LOC across 5 files (types.go, capture.go, search.go, and tests). Zero importers anywhere in the codebase. The `Solution` type and search infrastructure exist but nothing creates, stores, or queries solutions. This entire package is dead.

3. **Deprecated `renderFooter`** (`/root/projects/Autarch/internal/tui/unified_app.go` lines 2012-2015): Explicitly marked as deprecated, delegates to `renderFooterContent()`. Should be deleted.

4. **`OnboardingScanVision`, `OnboardingScanProblem`, `OnboardingScanUsers`** states (`/root/projects/Autarch/internal/tui/onboarding.go` lines 17-19): Declared in the enum, given `ID()` and `Label()` cases, but only appear in two switch arms in `unified_app.go` -- they are set to the current state once in lines 1673-1676 and then nothing happens. They are phantom states from a planned but unimplemented scan decomposition.

5. **`AllOnboardingStates()`** (`/root/projects/Autarch/internal/tui/onboarding.go` lines 28-37): Returns a curated list that deliberately excludes the three Scan states, confirming they are orphaned.

### Type Duplication

Three packages define overlapping entity models:

| Concept | `pkg/contract` | `pkg/intermute` | `pkg/events` |
|---------|---------------|-----------------|--------------|
| Task Status | `TaskStatus` (todo/in_progress/blocked/done) | `TaskStatus` (pending/running/blocked/done) | Event types only |
| Epic | `Epic` struct | `Epic` struct | `EntityEpic` type const |
| Story | `Story` struct | `Story` struct | `EntityStory` type const |
| Source Tool | `SourceTool` type + consts | N/A | Re-exported from contract |

`pkg/intermute/types.go` (575 LOC) defines its own `Spec`, `Epic`, `Story`, `Task`, `Session`, `Insight`, `CriticalUserJourney` types AND provides ~400 lines of `toIntermute*`/`fromIntermute*` conversion functions to translate between Autarch's types and the external intermute client library's types. These conversions are pure mechanical field copying -- every single function just maps identically-named fields between structs with slightly different module paths.

This means the same conceptual entity (e.g., "Epic") has at minimum three struct definitions: one in `pkg/contract`, one in `pkg/intermute`, and one in the external `intermute/client` package. Keeping these synchronized is a maintenance burden.

### Reinvented Wheels

`/root/projects/Autarch/internal/tui/types.go` lines 136-185 implement `splitLines()`, `splitComma()`, `containsComma()`, and `trimSpace()` -- manual reimplementations of `strings.Split(s, "\n")`, `strings.Split(s, ",")`, `strings.Contains(s, ",")`, and `strings.TrimSpace(s)`. The file comment says "Helper functions to avoid importing strings package in views" but the `strings` package is already imported in every other file in this package (`unified_app.go` uses it extensively). These 50 lines serve no purpose.

### Legacy Aliases

38 `.go` files reference the legacy tool names Vauxhall (Bigend), Praude (Gurgeh), and Tandemonium (Coldwine). Key locations:

- `internal/gurgeh/specs/praudemap.go` -- an entire file for the alias
- Config path resolution in `internal/gurgeh/config/config.go`, `internal/coldwine/project/paths.go`
- TUI layout and styles: `internal/gurgeh/tui/layout.go`, `internal/gurgeh/tui/styles.go`, `internal/coldwine/tui/model.go`, `internal/coldwine/tui/styles.go`, `internal/bigend/tui/model.go`, `internal/bigend/tui/pane.go`
- CLI root commands: `internal/coldwine/cli/root.go`, `internal/pollard/cli/root.go`
- Server and daemon: `internal/bigend/web/server.go`, `internal/bigend/daemon/server.go`

These aliases exist so that users with old `.vauxhall/`, `.praude/`, or `.tandemonium/` directories can still use the tools. The project CLAUDE.md confirms this is intentional. However, 38 files of alias maintenance is a real burden. A migration script + deprecation warning at startup would be more appropriate than permanent dual-path support spread across the codebase.

### cmd/ Entry Points

9 binaries in `cmd/`:

| Binary | Purpose | Justified? |
|--------|---------|-----------|
| `autarch` | Main unified TUI | Yes |
| `bigend` | Standalone Bigend server | Yes |
| `gurgeh` | Standalone Gurgeh CLI | Yes |
| `coldwine` | Standalone Coldwine CLI | Yes |
| `pollard` | Standalone Pollard CLI | Yes |
| `signals` | Signals WebSocket server | Yes (15 LOC, delegates to cli) |
| `autarch-mcp` | MCP server for AI agents | Yes |
| `archviz` | Regenerates architecture HTML | Dev tool -- should not be a top-level binary |
| `testui` | Test harness for onboarding views | Dev tool -- should not be a top-level binary |

`archviz` (269 LOC) and `testui` (321 LOC) are developer utilities. They import internal packages and serve no user-facing purpose. Shipping them as `cmd/` binaries means `go build ./cmd/...` includes them, `go install` exposes them, and they appear alongside real tools. They belong in `internal/devtools/` or `scripts/`.

### Over-Granular Packages

Several `pkg/` packages are tiny and have very few importers:

- **`pkg/timeout`** (15 LOC, 3 constants): Named timeout values. Could be inlined at each call site or collected into a `pkg/config` package. Having a whole package for 3 constants creates import overhead for minimal value.

- **`pkg/netguard`** (27 LOC, 1 function): `EnsureLocalOnly()` validates a loopback bind address. Used by 3 servers. A reasonable micro-package, but borderline -- could be a function in `pkg/httpapi`.

- **`pkg/compound`** (821 LOC, 0 importers): Dead code. Discussed above.

- **`pkg/contract`** (162 LOC): Shared entity types. Legitimate package but overlaps with `pkg/intermute` types.

### Communication Systems

Three cross-tool communication mechanisms:

1. **`pkg/signals`** (6 files, broker + server + client): Real-time typed alerts (competitor shipped, spec health low, etc.). Used by 3 internal emitters (`internal/{gurgeh,coldwine,pollard}/signals/`).

2. **`pkg/events`** (11 files, SQLite event store + subscriptions + reconciliation): Persistent event log for lifecycle tracking (initiative created, task completed, etc.). Has an `intermute_bridge.go` that bridges to intermute.

3. **`pkg/intermute`** (4 files, REST+WS client wrapper): Wraps external Intermute service for entity CRUD. Has per-tool adapters in `internal/{gurgeh,coldwine,pollard}/intermute/`.

These are not redundant -- they serve different purposes (real-time alerts vs. persistent audit log vs. entity CRUD). However, the layering creates a translation tax: data flows from tool internals -> intermute adapter -> intermute types -> intermute client -> server, and separately from tool internals -> events store -> events bridge -> intermute. The `pkg/events/intermute_bridge.go` exists solely to reconcile these two paths.

### internal/tui/view.go -- Pure Alias File

`/root/projects/Autarch/internal/tui/view.go` (22 LOC) is nothing but type aliases:

```go
type View = pkgtui.View
type HelpBinding = pkgtui.HelpBinding
type FullHelpProvider = pkgtui.FullHelpProvider
type Command = pkgtui.Command
type CommandProvider = pkgtui.CommandProvider
```

This exists for "backward compatibility" per the comment. It has 17 importers. While removing it would require updating 17 import paths, that is a mechanical find-and-replace. Each importer would change from `internal/tui.View` to `pkg/tui.View` -- a net simplification since it removes an indirection layer.

## Issues Found

| ID | Severity | Description |
|----|----------|-------------|
| P0-1 | P0 | Dual TUI implementations (`App` + `UnifiedApp`) duplicate ~400 LOC of overlay, layout, and run scaffolding logic |
| P0-2 | P0 | `OnboardingOrchestrator` (137 LOC) is dead code -- defined, tested, but never instantiated |
| P1-1 | P1 | `pkg/compound` (821 LOC) has zero importers -- entirely unreachable |
| P1-2 | P1 | Custom `splitLines`/`trimSpace`/`splitComma`/`containsComma` (50 LOC) reimplement `strings` stdlib in a file that already imports `strings` elsewhere in the package |
| P1-3 | P1 | Entity types defined three times across `pkg/contract`, `pkg/intermute`, and `pkg/events` with ~400 LOC of mechanical conversion code |
| P1-4 | P1 | Deprecated `renderFooter()` method left in `unified_app.go` (3 LOC + comment) |
| P1-5 | P1 | Legacy Vauxhall/Praude/Tandemonium aliases spread across 38 files |
| P2-1 | P2 | `cmd/archviz` and `cmd/testui` are dev-only tools exposed as top-level binaries |
| P2-2 | P2 | `pkg/timeout` is 3 constants in their own package (15 LOC) |
| P2-3 | P2 | `internal/tui/view.go` is a 22 LOC alias file that could be eliminated by updating 17 import paths |
| P2-4 | P2 | Three `OnboardingScan*` states are declared but functionally unused |
| P2-5 | P2 | `pkg/intermute/types.go` has ~400 LOC of `toIntermute*/fromIntermute*` boilerplate |

## Improvements Suggested

| ID | Title | Estimated LOC Saved | Effort |
|----|-------|-------------------|--------|
| IMP-1 | Merge `App` into `UnifiedApp` (start in `ModeDashboard` for `--skip-onboard`) | ~465 | Medium (1-2 hours) |
| IMP-2 | Delete `OnboardingOrchestrator` + its test | ~300 | Trivial |
| IMP-3 | Delete `pkg/compound/` entirely (or move to `docs/solutions/` if preserving for future) | ~821 | Trivial |
| IMP-4 | Replace 4 custom string helpers with `strings` stdlib calls | ~50 (net -46 LOC) | Trivial |
| IMP-5 | Unify entity types: pick `pkg/contract` as canonical, type-alias in `pkg/intermute`, remove conversion boilerplate | ~300 | Medium-Large |
| IMP-6 | Move `archviz` and `testui` out of `cmd/` | 0 (reorganization) | Trivial |
| IMP-7 | Inline `pkg/timeout` constants at call sites, delete package | ~15 | Trivial |
| IMP-8 | Extract `overlay()`/`insertAt()` to `pkg/tui`, remove 2 private copies | ~100 | Small |
| IMP-9 | Create a one-time migration script for legacy tool directories, remove 38-file alias support | Varies (potentially hundreds) | Large |
| IMP-10 | Delete 3 unused `OnboardingScan*` states + their switch cases | ~20 | Trivial |

## YAGNI Violations

1. **`OnboardingOrchestrator`**: Built as a reusable tea.Model wrapper for the onboarding flow, but the actual flow is managed directly by `UnifiedApp`. The orchestrator was designed for a separation that never materialized. It should be deleted.

2. **`pkg/compound`**: A full solution-capture-and-search system (types, capture, search with scoring) with zero callers. This was built for an institutional learning feature that was never wired up. Delete it. If needed later, it can be rebuilt from git history.

3. **`OnboardingScanVision/ScanProblem/ScanUsers` states**: Three enum values reserved for a fine-grained scan decomposition that never shipped. The scan flow uses a single `OnboardingInterview` state instead. These phantom states add noise to every switch statement that handles `OnboardingState`.

4. **Triple entity type system**: `pkg/contract`, `pkg/intermute`, and `pkg/events` each define their own entity models for the same domain concepts. The conversion layer (`toIntermute*`/`fromIntermute*`) is ~400 LOC of field-by-field copying with no transformation. This anticipates a future where the types diverge, but they have not and likely will not -- they represent the same domain entities.

5. **Type alias bridge in `internal/tui/view.go`**: 22 lines that exist solely because imports were not updated when types moved from `internal/tui` to `pkg/tui`. "Backward compatibility" within a single repo is not a real constraint -- all callers can be updated atomically.

## Overall Assessment

**Total potential LOC reduction:** ~2,100 lines (1.5% of total), concentrated in high-traffic files that developers read frequently.

**Complexity score:** High -- not because of any single catastrophic issue, but because complexity is distributed: dual TUI paths, triple type systems, 38-file alias support, and dead code in core packages all compound to make the codebase harder to navigate than its functionality warrants.

**Recommended action:** Needs changes. Start with the zero-risk, high-value items:
1. Delete `OnboardingOrchestrator` (IMP-2) -- zero callers, zero risk
2. Delete `pkg/compound` (IMP-3) -- zero callers, zero risk
3. Replace custom string helpers with stdlib (IMP-4) -- trivial, improves clarity
4. Delete deprecated `renderFooter` (P1-4) -- 3 lines, obvious
5. Extract shared `overlay()`/`insertAt()` to `pkg/tui` (IMP-8) -- removes duplication across the two TUI implementations as a stepping stone to the full merge (IMP-1)

The App/UnifiedApp merge (IMP-1) and entity type unification (IMP-5) are higher effort but deliver the largest maintainability improvements. The legacy alias cleanup (IMP-9) is the highest effort and lowest urgency -- defer until a major version boundary.
