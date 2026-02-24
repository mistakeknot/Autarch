---
agent: fd-architecture
tier: 1
issues:
  - id: P0-1
    severity: P0
    title: "internal/tui is a God Package that violates module boundaries"
    section: "Module Boundaries and Dependency Direction"
  - id: P1-1
    severity: P1
    title: "Pollard imports Gurgeh arbiter types -- inverted dependency"
    section: "Cross-Tool Dependency Analysis"
  - id: P1-2
    severity: P1
    title: "Dual TUI implementations (App vs UnifiedApp) with divergent state models"
    section: "Dual TUI Implementations"
  - id: P1-3
    severity: P1
    title: "pkg/contract is nearly unused -- phantom shared layer"
    section: "Shared Infrastructure Utilization"
  - id: P2-1
    severity: P2
    title: "4x duplicated generateID() function across signal emitters"
    section: "Code Duplication"
  - id: P2-2
    severity: P2
    title: "internal/intermute deprecated but still wired in cmd/autarch"
    section: "Intermute Integration Layers"
  - id: P2-3
    severity: P2
    title: "spec vs specs naming ambiguity in internal/gurgeh"
    section: "Naming Consistency"
  - id: P2-4
    severity: P2
    title: "PhaseArtifacts type defined in internal/tui shadows arbiter domain"
    section: "Module Boundaries and Dependency Direction"
improvements:
  - id: IMP-1
    title: "Extract arbiter shared types to pkg/arbiter or pkg/sprint"
    section: "Cross-Tool Dependency Analysis"
  - id: IMP-2
    title: "Introduce adapter interfaces in internal/tui to decouple from tool internals"
    section: "Module Boundaries and Dependency Direction"
  - id: IMP-3
    title: "Consolidate App and UnifiedApp into a single implementation"
    section: "Dual TUI Implementations"
  - id: IMP-4
    title: "Either wire pkg/contract into tool storage layers or remove it"
    section: "Shared Infrastructure Utilization"
  - id: IMP-5
    title: "Extract generateID to a shared utility package"
    section: "Code Duplication"
verdict: needs-changes
---

# Architecture Review: Autarch Monorepo

## Summary

Autarch is an 823-file Go 1.24 monorepo containing four tools (Bigend, Gurgeh, Coldwine, Pollard) with a well-documented `cmd/` + `internal/` + `pkg/` structure and clear architectural intent. The project follows idiomatic Go module conventions and has thoughtful shared infrastructure (`pkg/signals`, `pkg/events`, `pkg/contract`, `pkg/tui`). However, the implementation has drifted from the documented architecture in two critical areas: (1) `internal/tui` has become a coupling hub that imports concrete types from all four tools, and (2) `internal/pollard/quick` imports `internal/gurgeh/arbiter` types, creating an inverted dependency between tools that should be independent. The dual TUI implementation (`App` vs `UnifiedApp`) adds maintenance burden and the shared contract types in `pkg/contract` are nearly unused, suggesting a phantom abstraction layer.

---

## Section-by-Section Review

### 1. cmd/ Layer -- Entry Points

**Files examined**: `cmd/autarch/main.go`, `cmd/bigend/main.go`, `cmd/gurgeh/main.go`, `cmd/coldwine/main.go`, `cmd/pollard/main.go`, `cmd/signals/`, `cmd/archviz/`, `cmd/autarch-mcp/`, `cmd/testui/`

The `cmd/` layer correctly serves as thin wiring: each entry point imports from its corresponding `internal/` package and uses `cobra` for CLI scaffolding. The unified `cmd/autarch/main.go` is the most complex, importing from all four tools' internal packages to assemble the tabbed TUI. This is acceptable for a unified entry point.

There are 9 entry points in `cmd/`. The project documents 5 primary ones (autarch, bigend, coldwine, gurgeh, pollard) plus a signals server. The undocumented `archviz`, `autarch-mcp`, and `testui` appear to be development/auxiliary tools. This is fine but should be noted in ARCHITECTURE.md for completeness.

**Verdict**: Clean. Follows Go conventions correctly.

### 2. Module Boundaries and Dependency Direction

**The documented rule** (from AGENTS.md): "Use `internal/` for tool-specific, `pkg/` for shared code."

**Actual dependency graph** (imports verified via grep):

```
pkg/ ---> (no internal imports)                          [CORRECT]
cmd/ ---> internal/, pkg/                                [CORRECT]
internal/bigend ---> internal/gurgeh/specs,
                     internal/gurgeh/config              [ACCEPTABLE - read-only aggregation]
internal/coldwine ---> internal/gurgeh/specs,
                       internal/gurgeh/project,
                       internal/pollard/insights         [ACCEPTABLE - documented integration]
internal/gurgeh ---> internal/pollard/quick,
                     internal/pollard/insights,
                     internal/pollard/research,
                     internal/tui                        [MIXED - see issues]
internal/pollard ---> internal/gurgeh/arbiter            [PROBLEMATIC - inverted]
internal/tui ---> internal/gurgeh/arbiter,
                  internal/gurgeh/arbiter/scan,
                  internal/gurgeh/exploration,
                  internal/gurgeh/specs,
                  internal/coldwine/epics,
                  internal/coldwine/tasks,
                  internal/pollard/research,
                  internal/pollard/quick                 [PROBLEMATIC - hub coupling]
```

The `pkg/` boundary is clean: no `pkg/` package imports from `internal/`. This is the most important invariant in Go monorepo architecture and it holds.

However, `internal/tui` has become a coupling nexus. It imports concrete types from all four tools' internal packages across 18 import statements in 12 files. This means any change to Gurgeh's arbiter types, Coldwine's task types, or Pollard's research types can force changes in the shared TUI layer. The documented architecture shows `internal/tui` as a shared _application shell_, but in practice it has become a de-facto integration layer with deep knowledge of each tool's domain types.

The `PhaseArtifacts` type (and related `VisionArtifact`, `ProblemArtifact`, `UsersArtifact`) defined in `/root/projects/Autarch/internal/tui/messages.go` lines 171-229 is particularly concerning. This is a domain type that duplicates concepts from `internal/gurgeh/arbiter/types.go` (`SectionDraft`, `SprintState`) but is defined in the TUI layer. It creates a shadow domain model that must be kept in sync with the arbiter's types.

### 3. Cross-Tool Dependency Analysis

**Bigend -> Gurgeh/Coldwine**: `internal/bigend/daemon/projects.go` and `internal/bigend/aggregator/aggregator.go` import `internal/gurgeh/specs` to read spec summaries. This matches the documented "read-only aggregation" pattern and is architecturally sound.

**Coldwine -> Gurgeh**: `internal/coldwine/schema/schema.go`, `internal/coldwine/readiness/readiness.go`, `internal/coldwine/cli/init_flow.go`, and `internal/coldwine/prd/import.go` all import `internal/gurgeh/specs`. This matches the documented data flow: Gurgeh produces specs, Coldwine consumes them for task generation.

**Gurgeh -> Pollard**: `internal/gurgeh/tui/briefs.go` imports `internal/pollard/insights`, `internal/gurgeh/tui/model.go` and `internal/gurgeh/arbiter/tui/arbiter_view.go` import `internal/pollard/quick` and `internal/pollard/research`. This matches the documented "research enriches PRDs" pattern.

**Pollard -> Gurgeh (INVERTED)**: `/root/projects/Autarch/internal/pollard/quick/scan.go` imports `internal/gurgeh/arbiter` for the `QuickScanResult`, `GitHubFinding`, and `HNFinding` types. This is architecturally inverted: Pollard (a research tool) should not depend on Gurgeh (a PRD tool). The types `QuickScanResult`, `GitHubFinding`, and `HNFinding` are generic research result types that happen to be defined in the arbiter package because the arbiter uses them. The dependency should flow the other direction: the types should live in Pollard (or in `pkg/`) and the arbiter should import them.

### 4. Shared Infrastructure Utilization

**pkg/signals** (6 files): Well-used across the codebase -- 16 files import it from `internal/gurgeh/signals`, `internal/coldwine/signals`, `internal/pollard/signals`, `internal/signals/cli`, `internal/bigend/tui`, `internal/tui/views`, and more. The `Broker` + `Subscription` pattern with WebSocket support in `pkg/signals/broker.go` is clean and well-tested.

**pkg/events** (11 files): Used by 7 files, mostly within `pkg/events` itself, plus `cmd/autarch/events.go`, `cmd/autarch/reconcile.go`, `internal/coldwine/tui/artifacts.go`, and `internal/tui/views/signals.go`. Moderate utilization. The event spine is comprehensive (19 event types, 8 entity types) but adoption within the tool internals is thin -- most tools still use file-based integration rather than the event system.

**pkg/contract** (3 files): Nearly unused. Only 1 internal file (`internal/coldwine/tui/artifacts.go`) imports it directly. The `pkg/events/types.go` re-exports its constants. The types (`Initiative`, `Epic`, `Story`, `Task`, `Run`, `Outcome`) are defined but the tools use their own internal types (`coldwine/epics.EpicProposal`, `coldwine/tasks.TaskProposal`, `gurgeh/specs.Spec`, etc.) instead of these shared contract types. This is a phantom shared layer -- it was designed for cross-tool communication but the tools evolved their own type systems independently.

**pkg/intermute** (4 files): Clean wrapper around the external Intermute client. Correctly placed in `pkg/` since multiple tools use it.

**pkg/tui** (29 files): Heavily used as the shared component library (styles, layouts, ChatPanel, DocPanel, ShellLayout, Composer, CommandPicker, AgentSelector, LogPane). This is the most successful `pkg/` package in terms of adoption across the codebase.

### 5. Dual TUI Implementations

**`App`** (`/root/projects/Autarch/internal/tui/app.go`): Used when `--skip-onboard` is passed. Has `client`, `tabs`, `views`, `palette`, `keys`, `help`, `logPane` state. Imports only `pkg/autarch` and `pkg/tui` -- clean dependency profile.

**`UnifiedApp`** (`/root/projects/Autarch/internal/tui/unified_app.go`): Used for the normal onboarding flow. Has all of `App`'s state PLUS onboarding state (`onboardingState`, `breadcrumb`, `interviewAnswers`, `generatedEpics`, `generatedTasks`, `researchCoord`, `codingAgent`, etc.). Imports from `internal/gurgeh/arbiter`, `internal/gurgeh/arbiter/scan`, `internal/gurgeh/exploration`, `internal/coldwine/epics`, `internal/coldwine/tasks`, `internal/pollard/research` -- heavy cross-tool coupling.

The project's own MEMORY.md acknowledges this: "Two TUI implementations: App (used by --skip-onboard) and UnifiedApp (normal flow) -- need merging." The unified navigation plan (Phase 2) targets absorbing onboarding into Gurgeh, which would eliminate this split.

The divergence is not just cosmetic. `UnifiedApp` has ~1300+ lines with complex state machine logic for onboarding transitions, while `App` is a simpler ~200-line tab container. The onboarding state in `UnifiedApp` (12+ message types per MEMORY.md) creates a gravitational pull that draws tool-specific types into the shared TUI layer.

### 6. Intermute Integration Layers

There are three distinct Intermute integration points:

1. **`pkg/intermute`** (4 files): Shared client wrapper. Clean.
2. **`internal/intermute`** (2 files): Server lifecycle management (`Manager`) and deprecated agent registration (`Start`). The `Start` function is marked deprecated in favor of `pkg/intermute.Register()`.
3. **`internal/{tool}/intermute`** (per-tool bridges): Gurgeh sync, Coldwine broadcaster, Pollard publisher. Each correctly adapts tool-specific types to Intermute's domain model.

The deprecated `internal/intermute/intermute.go:Start()` function is still wired in the codebase (used by `cmd/autarch/main.go` through `internalIntermute.NewManager`). The `Manager` in the same package is not deprecated and provides server lifecycle management, but sharing a package with a deprecated function creates confusion about the package's status.

### 7. Signal System Architecture

The signal system is well-designed with clean separation:

- **`pkg/signals`**: Shared types (`Signal`, `SignalType`, `Severity`) + infrastructure (`Broker`, `Subscription`, WebSocket server)
- **`internal/{tool}/signals`**: Per-tool emitters that produce tool-specific signal types using `pkg/signals.Signal`
- **`internal/gurgeh/signals`**: Also includes a `Store` for signal persistence and a `Review` system

Each emitter correctly depends only on `pkg/signals` and its own tool's types. The one issue is that all three emitters plus `internal/gurgeh/arbiter/types.go` independently define a `generateID()` function with identical logic (16 random bytes, hex-encoded with a "sig-" prefix). This is a textbook case for extraction to a shared utility.

### 8. Naming Consistency

Inside `internal/gurgeh/`, there are two similarly-named packages:
- **`spec`** (2 files): Contains `SpecFlowAnalyzer` for gap analysis
- **`specs`** (20 files): Contains all spec types (`PRD`, `Spec`), loading, validation, evolution, hashing, etc.

This singular/plural distinction (`spec` vs `specs`) is confusing. The `spec` package has only 2 files (`specflow_analyzer.go` and its test) and could easily be merged into `specs` or renamed to something like `specflow` to clarify its purpose.

---

## Issues Found

### P0-1: internal/tui is a God Package that violates module boundaries

**Location**: `/root/projects/Autarch/internal/tui/` (18 files, 42 total including subdirectories)

**Problem**: `internal/tui` imports concrete types from all four tools' `internal/` packages -- `gurgeh/arbiter`, `gurgeh/arbiter/scan`, `gurgeh/exploration`, `gurgeh/specs`, `coldwine/epics`, `coldwine/tasks`, `pollard/research`, `pollard/quick`. This makes it impossible to change any tool's internal types without potentially breaking the shared TUI layer. It also means building the Bigend standalone binary (`cmd/bigend`) would transitively pull in all Gurgeh and Coldwine types even though Bigend does not use them.

The root cause is that `internal/tui/messages.go` defines Bubble Tea message types that carry tool-specific types (e.g., `SpecCompletedMsg{Spec *specs.PRD}`, `TasksGeneratedMsg{Tasks []tasks.TaskProposal}`), and `internal/tui/unified_app.go` orchestrates transitions that require understanding all tools' state.

**Suggestion**: Introduce adapter interfaces in `internal/tui` that tool views implement, rather than importing concrete types. For example, instead of `internal/tui` knowing about `arbiter.Orchestrator`, define a `SprintStateProvider` interface (already partially done in `types.go` line 209) and push all type-specific wiring into the view implementations in `internal/tui/views/`. The `cmd/autarch/main.go` wiring code is the appropriate place for concrete type knowledge, not the shared TUI layer.

### P1-1: Pollard imports Gurgeh arbiter types -- inverted dependency

**Location**: `/root/projects/Autarch/internal/pollard/quick/scan.go` line 13

**Problem**: `internal/pollard/quick` imports `internal/gurgeh/arbiter` to use `QuickScanResult`, `GitHubFinding`, and `HNFinding` types. This means Pollard (research tool) depends on Gurgeh (PRD tool), but conceptually these are research result types that should belong to Pollard or to a shared package. The arbiter happened to define them because it consumes them, but the producer (Pollard's quick scanner) should own the types.

**Suggestion**: Move `QuickScanResult`, `GitHubFinding`, `HNFinding` to `internal/pollard/quick/types.go` and have `internal/gurgeh/arbiter` import from there. Alternatively, if these types are genuinely cross-tool, move them to `pkg/research/` or similar. This aligns dependency direction with data flow: Pollard produces research, arbiter consumes it.

### P1-2: Dual TUI implementations with divergent state models

**Location**: `/root/projects/Autarch/internal/tui/app.go` (App) and `/root/projects/Autarch/internal/tui/unified_app.go` (UnifiedApp)

**Problem**: Two separate Bubble Tea models serve as the application root depending on a CLI flag. `App` is a clean tab container (~200 lines, no tool-specific imports). `UnifiedApp` is a complex state machine (~1300+ lines) that manages onboarding flow with deep coupling to Gurgeh, Coldwine, and Pollard internals. Any TUI-wide feature (new keybinding, layout change, log pane behavior) must be implemented in both models and kept in sync.

**Suggestion**: The project already has a plan (Phase 2 of the unified navigation plan) to absorb onboarding into Gurgeh. Prioritize this. The target state should be a single `App`-style container where Gurgeh's tab handles its own onboarding internally, eliminating `UnifiedApp` entirely.

### P1-3: pkg/contract is nearly unused -- phantom shared layer

**Location**: `/root/projects/Autarch/pkg/contract/types.go`

**Problem**: `pkg/contract` defines 7 entity types (`Initiative`, `Epic`, `Story`, `Task`, `Run`, `Outcome`, `RunArtifact`) and 5 status enums intended as the unified cross-tool data contract. But only 1 file in `internal/` actually imports it (`internal/coldwine/tui/artifacts.go`). The tools use their own types: Gurgeh uses `specs.Spec`/`specs.PRD`, Coldwine uses `epics.EpicProposal`/`tasks.TaskProposal`, Pollard uses its own insight types. The contract types exist but are not adopted.

**Suggestion**: Either (a) wire `pkg/contract` types into the tool storage layers as the canonical representation, updating the per-tool types to embed or convert to contract types, or (b) acknowledge that file-based integration has made the contract layer unnecessary and remove it to reduce confusion. The current state -- defined but unused -- is the worst outcome because it misleads developers into thinking there is a shared type system when there is not.

### P2-1: 4x duplicated generateID() function

**Location**: `/root/projects/Autarch/internal/coldwine/signals/emitter.go:59`, `/root/projects/Autarch/internal/gurgeh/arbiter/types.go:165`, `/root/projects/Autarch/internal/gurgeh/signals/emitter.go:117`, `/root/projects/Autarch/internal/pollard/signals/emitter.go:50`

**Problem**: Four independent implementations of `generateID()` with identical logic: `crypto/rand.Read(16 bytes) -> hex.EncodeToString`. Three of them use 8 bytes with "sig-" prefix; one (arbiter) uses 16 bytes without prefix. The semantic difference (signal ID vs sprint ID) is buried in copy-pasted code.

**Suggestion**: Extract to a shared utility, e.g., `pkg/id.New(prefix string, bytes int) string`. This is a small change with clear benefit: consistent ID generation and a single place to add tracing/debugging if needed.

### P2-2: internal/intermute deprecated but still wired

**Location**: `/root/projects/Autarch/internal/intermute/intermute.go` (deprecated `Start` function), `/root/projects/Autarch/internal/intermute/manager.go` (active `Manager`)

**Problem**: The `Start` function in `internal/intermute/intermute.go` is explicitly deprecated ("Deprecated: Use pkg/intermute.Register() or pkg/intermute.RegisterTool() instead") but lives in the same package as the active `Manager` struct. The `cmd/autarch/main.go` imports the package as `internalIntermute` for the `Manager`, which means the deprecated code is still compiled and discoverable.

**Suggestion**: Either move `Manager` to its own package (e.g., `internal/intermute/lifecycle/`) or remove the deprecated `Start` function entirely if all callers have migrated to `pkg/intermute.Register()`.

### P2-3: spec vs specs naming ambiguity in internal/gurgeh

**Location**: `/root/projects/Autarch/internal/gurgeh/spec/` (2 files) and `/root/projects/Autarch/internal/gurgeh/specs/` (20 files)

**Problem**: Two packages with near-identical names. `spec` contains only `specflow_analyzer.go` and its test. `specs` contains all core spec types, loading, validation, and evolution. A developer looking for "spec-related code" will not know which package to check.

**Suggestion**: Rename `internal/gurgeh/spec` to `internal/gurgeh/specflow` or merge its `SpecFlowAnalyzer` into the `specs` package. The analyzer operates on spec types from the `specs` package anyway, so co-location would be natural.

### P2-4: PhaseArtifacts type in internal/tui shadows arbiter domain

**Location**: `/root/projects/Autarch/internal/tui/messages.go` lines 171-229

**Problem**: `internal/tui/messages.go` defines `PhaseArtifacts`, `VisionArtifact`, `ProblemArtifact`, `UsersArtifact`, `EvidenceItem`, `QualityScores`, `Persona`, and `ResolvedQuestion` types. These shadow the domain concepts in `internal/gurgeh/arbiter/types.go` (`SectionDraft`, `ConfidenceScore`). The TUI layer has its own parallel domain model that must be manually kept in sync with the arbiter's model. Data flows through a conversion step in `unified_app.go:createSpecSummaryFromSprintState()` that maps between the two representations.

**Suggestion**: Eliminate the shadow types by having the TUI views work directly with the arbiter's types (already partially done -- `views/sprint_view.go` uses `arbiter.Orchestrator` directly). The `PhaseArtifacts` types appear to be remnants of an earlier onboarding scan flow and could likely be replaced with the arbiter's `ScanArtifacts` or a thin wrapper interface.

---

## Improvements Suggested

### IMP-1: Extract arbiter shared types to pkg/arbiter or pkg/sprint

Move `QuickScanResult`, `GitHubFinding`, `HNFinding`, `ResearchFinding`, and `PriorArtResult` from `internal/gurgeh/arbiter/types.go` to a shared package. These types represent research results, not PRD-specific concepts, and are currently the source of the inverted Pollard->Gurgeh dependency. A `pkg/research/types.go` or even `internal/pollard/quick/types.go` would be more appropriate homes. This resolves P1-1 and reduces the arbiter's type surface area.

### IMP-2: Introduce adapter interfaces in internal/tui to decouple from tool internals

Replace direct imports of `internal/gurgeh/arbiter`, `internal/coldwine/epics`, etc. in `internal/tui` with interfaces defined in `internal/tui` itself or in `pkg/tui`. For example:

```go
// pkg/tui/view.go or internal/tui/adapters.go
type EpicProposal interface {
    Title() string
    Description() string
    StoryCount() int
}
```

Tool views would implement these interfaces. The TUI layer would work with interfaces, not concrete types. This is a large refactor but would dramatically reduce coupling and make the dual-TUI problem (P1-2) easier to resolve.

### IMP-3: Consolidate App and UnifiedApp into a single implementation

The Phase 2 plan (absorb onboarding into Gurgeh) is the right approach. After that, `UnifiedApp` can be deleted and `App` becomes the sole entry point. In the interim, extract any shared logic (tab management, keybinding handling, log pane toggling) from `UnifiedApp` into `App` to reduce drift between the two implementations.

### IMP-4: Either wire pkg/contract into tool storage layers or remove it

The current state where `pkg/contract` exists but is unused by 3 of 4 tools is worse than not having it at all. If the project intends to build cross-tool event-driven workflows (the event spine in `pkg/events` assumes these types), then each tool's storage layer should convert to/from contract types at its boundary. If file-based integration is the long-term strategy, remove `pkg/contract` and let each tool own its types entirely.

### IMP-5: Extract generateID to a shared utility package

Create `pkg/id/id.go` (or add to an existing utility package) with:

```go
func New() string        // 32-char hex (for sprint IDs, etc.)
func Signal() string     // "sig-" + 16-char hex (for signal IDs)
```

Replace the 4 duplicated implementations. This is a low-risk, high-clarity improvement.

---

## Overall Assessment

**Verdict: needs-changes**

The Autarch monorepo has a solid architectural foundation: clean `pkg/` boundary, well-documented integration patterns, thoughtful graceful degradation for Intermute, and a well-designed signal system. The `pkg/tui` component library is the standout success -- heavily adopted and cleanly decoupled.

The primary architectural concern is the coupling fan-out from `internal/tui`, which has evolved from a thin shell into a cross-tool integration hub. This is the single largest source of architectural fragility in the codebase. The inverted Pollard->Gurgeh dependency (P1-1) is a smaller but more cleanly fixable instance of the same problem: types are defined where they are consumed rather than where they are produced.

**Top 3 changes that would improve the architecture:**

1. **Fix the Pollard->Gurgeh dependency inversion** (P1-1): Move research result types to Pollard or `pkg/`. This is a small, mechanical change (move 3 type definitions, update imports) with clear architectural benefit.

2. **Complete Phase 2 of the unified navigation plan** (P1-2/IMP-3): Absorbing onboarding into Gurgeh eliminates `UnifiedApp`, which is the primary driver of `internal/tui`'s cross-tool coupling. This is already planned and is the single highest-leverage refactor.

3. **Decide the fate of pkg/contract** (P1-3/IMP-4): The phantom shared type layer creates false confidence that cross-tool type safety exists. Either adopt it fully or remove it -- the current state is misleading.
