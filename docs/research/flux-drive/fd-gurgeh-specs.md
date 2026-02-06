# Gurgeh Spec System Review: Phase 2 Impact Analysis

**Reviewer:** Gurgeh Spec System Specialist
**Date:** 2026-02-06
**Focus:** How Phase 2 (Gurgeh absorbs onboarding) affects sprint orchestration, spec persistence, and the 8-phase model
**Plan reviewed:** `docs/plans/2026-02-05-unified-tui-navigation-design.md` (Phase 2 section)

---

## Summary

Phase 2 moves the onboarding state machine (kickoff -> sprint -> spec summary -> epics -> tasks) from `UnifiedApp` into the Gurgeh tab view. This refactor touches the full spec lifecycle but does NOT change the arbiter Orchestrator, persistence layer, or consistency engine themselves. The primary risk is severing the message-routing chains that connect KickoffView -> SprintView -> SpecSummaryView -> EpicReview -> TaskReview, because `UnifiedApp.Update()` currently acts as the central switchboard for 12+ message types that drive spec creation. If the new GurgehView host doesn't replicate every transition handler with identical semantics, sprints will stall, specs will fail to export, and the confidence/consistency pipeline will break silently.

---

## Section-by-Section Review

### Phase 2 Overview: "Move onboarding state machine from UnifiedApp into GurgehView"

The plan correctly identifies the scope: ~400 lines of transition handlers, 12+ message types, 6 view factories, and 7 key struct fields (`projectID`, `projectName`, `interviewAnswers`, `generatedEpics`, `generatedTasks`, `onboardingState`, `breadcrumb`). The 800-1200 line estimate is reasonable.

However, the plan does not enumerate the **spec-specific data flows** that must be preserved. These are:

1. **Exploration -> Orchestrator seeding:** `handleProjectCreated` converts `CodebaseScanResultMsg.PhaseArtifacts` into `scan.Artifacts` via `scanResultToArtifacts()`, then passes them to `SprintView.StartSprintWithExploration()`. This is the mechanism that pre-populates phases 0-2 (Vision, Problem, Users) with evidence-grounded drafts.

2. **Sprint completion -> SpecSummary extraction:** `SprintCompleteMsg` handler calls `createSpecSummaryFromSprintState(&state)` which reads `state.Sections[PhaseVision]`, `[PhaseProblem]`, `[PhaseUsers]`, `[PhaseRequirements]` and extracts display fields. This feeds the `SpecSummaryView`.

3. **SpecSummary -> Epic generation:** The `SpecAcceptedMsg` handler passes `{Vision, Users, Problem, Platform, Language, Requirements}` to `agent.GenerateEpicsWithOutput()`. The agent generation depends on `codingAgent` which is currently owned by `UnifiedApp`, not any view.

4. **Research coordination:** `researchCoord` is created in `NewUnifiedApp` and threaded through `createArbiterView(coord)` and `createSpecSummaryView(spec, coord)`. The new host must own or receive this coordinator.

### "Move breadcrumb into Gurgeh's content area"

The `Breadcrumb` component (`internal/tui/breadcrumb.go`) tracks `OnboardingState` and emits `NavigateToStepMsg`. Currently, `UnifiedApp.Update()` handles `NavigateToStepMsg` by calling `navigateToStep(msg.State)` which can recreate any onboarding sub-view.

In Phase 2, the breadcrumb moves inside Gurgeh. This means the Gurgeh view must:
- Own the `OnboardingState` enum tracking
- Handle `NavigateToStepMsg` internally
- Manage view factory calls for spec summary, epic review, and task review

This is safe for the spec system as long as the Gurgeh view replicates the guard logic in `navigateToStep()` -- e.g., only allowing navigation to `OnboardingEpicReview` if `len(generatedEpics) > 0`.

### "Move all onboarding message handlers out of UnifiedApp.Update()"

This is the highest-risk change for the spec system. The following message handlers contain spec-system logic:

| Handler | Spec-System Logic |
|---------|-------------------|
| `ProjectCreatedMsg` | Converts scan artifacts to `scan.Artifacts`, starts sprint, resumes existing sprints |
| `InterviewCompleteMsg` | Creates `SpecSummary` from answers, wires `SpecSummaryViewSetter` callbacks |
| `SprintCompleteMsg` | Extracts `SpecSummary` from `SprintState` via `createSpecSummaryFromSprintState()` |
| `SpecAcceptedMsg` | Triggers agent-powered epic generation via `generateEpicsWithAgent()` |
| `EpicsGeneratedMsg` | Stores epics, calls `finalizeAgentRun("epics")` for diff tracking |
| `EpicsAcceptedMsg` | Triggers agent-powered task generation via `generateTasksWithAgent()` |
| `TasksGeneratedMsg` | Stores tasks, calls `finalizeAgentRun("tasks")` for diff tracking |
| `TasksAcceptedMsg` | Transitions to `OnboardingComplete`, triggers `enterDashboard()` |
| `ScanCodebaseMsg` | Runs Claude Code exploration, streams progress to log pane |
| `CodebaseScanResultMsg` | Updates onboarding state, passes scan result to kickoff view |

All of these must be moved as a unit. Partial movement -- e.g., moving `SprintCompleteMsg` but not `SpecAcceptedMsg` -- would break the spec -> epic -> task pipeline.

### "Merge App and UnifiedApp into a single implementation"

The `App` struct (`internal/tui/app.go`) is used by `--skip-onboard` and has NO onboarding or spec-creation logic. It just renders dashboard tabs. Merging these two is safe for the spec system because:
- `App` never touches `Orchestrator`, `SprintState`, or spec persistence
- The merge only affects tab management and key handling, not data flow
- After merge, `--skip-onboard` becomes a no-op (goes straight to dashboard with Gurgeh tab managing its own state)

### "Rewire view factories in cmd/autarch/main.go"

Currently `main.go` injects 6 view factories into `UnifiedApp.SetViewFactories()` plus 2 more via `SetArbiterViewFactory()` and `SetSprintViewFactory()`. After Phase 2, these factories must be injected into the Gurgeh view instead.

The `SprintView` factory is the most critical -- it creates the `Orchestrator` with optional Intermute research integration (`arbiter.NewOrchestratorWithResearch`). The new Gurgeh host must accept this factory and thread the Intermute URL through.

---

## Issues Found

### P0-1: Agent ownership transfer is unspecified

**Location:** Phase 2 plan, "Changes" section

`UnifiedApp` owns `codingAgent *agent.Agent`, `agentSelector *pkgtui.AgentSelector`, and `selectedAgent string`. These are used by `generateEpicsWithAgent()` and `generateTasksWithAgent()` to run Claude Code / Codex for spec-to-epic and epic-to-task generation. The plan lists `interviewAnswers, generatedEpics, generatedTasks` as fields to move but does not mention `codingAgent` or `agentSelector`.

If the Gurgeh view doesn't own or receive the agent reference, epic and task generation will fail with `AgentNotFoundMsg`. This is a hard blocker for the spec flow.

**Recommendation:** Explicitly add `codingAgent`, `agentSelector`, and `selectedAgent` to the list of fields moving into Gurgeh. Alternatively, keep them in the parent app and expose an interface (`AgentProvider`) that Gurgeh calls when it needs to generate epics/tasks. The latter is cleaner since it avoids duplicating agent detection logic.

### P0-2: Sprint state extraction on completion depends on parent view access pattern

**Location:** `unified_app.go:591-635` (`SprintCompleteMsg` handler)

The `SprintCompleteMsg` handler does:
```go
if provider, ok := a.currentView.(SprintStateProvider); ok {
    state, stateOK := provider.Orchestrator().State()
    // ... creates SpecSummary from state
}
```

This uses a type assertion on `currentView` to access the Orchestrator. After Phase 2, the Gurgeh view will be `currentView` from the parent's perspective, and the SprintView will be a sub-view inside Gurgeh. The type assertion `a.currentView.(SprintStateProvider)` will fail because `GurgehView` (the outer container) won't implement `SprintStateProvider` -- `SprintView` (the inner view) does.

**Recommendation:** Either (a) have the expanded GurgehView implement `SprintStateProvider` by delegating to its internal SprintView, or (b) handle `SprintCompleteMsg` entirely inside the Gurgeh view (preferred, since the whole point is to internalize the flow). The plan should call this out since it's a non-obvious interface contract.

### P1-1: Orchestrator `projectPath` may diverge from GurgehView context

**Location:** `internal/tui/views/sprint_view.go:49-75` (SprintView construction)

The `SprintView` constructor receives `projectPath` from `handleProjectCreated`:
```go
projectPath := ""
if cwd, err := os.Getwd(); err == nil {
    projectPath = cwd
}
a.currentView = a.createSprintView(projectPath)
```

This uses the process CWD at the time of project creation. After Phase 2, if the user switches to a different tab (e.g., Pollard) and then comes back to Gurgeh, the CWD might have changed (though unlikely in practice since Autarch doesn't `chdir`). More importantly, the `Orchestrator` stores `projectPath` and uses it for:
- `SaveSprintState()` writes to `{projectPath}/.gurgeh/sprints/`
- `LoadVisionContext()` reads from `{projectPath}/.gurgeh/specs/`
- `ListSprints()` scans `{projectPath}/.gurgeh/sprints/`

The Gurgeh view must ensure the `projectPath` is captured at sprint start time and doesn't change if the user navigates away and back.

**Recommendation:** The Gurgeh view should store `projectPath` as a field set during kickoff (when `ProjectCreatedMsg` fires or scan completes) and pass it to the SprintView factory. Do not re-read CWD when resuming.

### P1-2: Consistency engine is invoked during `Advance()` but state flows through multiple owners

**Location:** `internal/gurgeh/arbiter/orchestrator.go:297-426` (`advanceInternal`)

The consistency engine checks happen inside `Orchestrator.Advance()` which is called from `Orchestrator.ChatAcceptDraft()`. The Orchestrator is fully self-contained (owns its own mutex, state, and persistence). This means Phase 2 does NOT affect the consistency engine directly.

However, there is an indirect risk: the `SprintView` calls `v.orch.ChatAcceptDraft()` which internally calls `Advance()`, which can return `ErrBlocker`. The `SprintView.handleAccept()` converts this to `SprintConflictMsg`. Currently, `UnifiedApp.Update()` does NOT handle `SprintConflictMsg` -- it falls through to `a.currentView.Update(msg)`. This is fine because `SprintView.Update()` handles it directly.

After Phase 2, this pattern must be preserved: `SprintConflictMsg` must reach the SprintView inside Gurgeh. If the Gurgeh view intercepts messages and doesn't forward them, consistency conflicts will be silently swallowed.

**Recommendation:** The Gurgeh view's `Update()` must have a default pass-through that forwards unhandled messages to its active sub-view. Add a test that verifies `SprintConflictMsg` reaches the SprintView after Phase 2.

### P1-3: Spec persistence path unchanged but export trigger is in UnifiedApp

**Location:** `internal/gurgeh/arbiter/export.go`, `internal/gurgeh/arbiter/persistence.go`

The persistence layer (`SaveSprintState`, `LoadSprintState`, `ListSprints`) is self-contained within the Orchestrator and untouched by Phase 2. Good.

However, the `ExportSpec()` method on the Orchestrator is currently only called via the handoff flow (post-sprint completion). The handoff options are returned by `GetHandoffOptions(state)` which includes "Export Spec". This handoff UI is not yet implemented in the current TUI -- the current flow goes `SprintComplete -> SpecSummary -> EpicReview -> TaskReview -> Dashboard`.

Phase 2 should preserve the `ExportToSpec()` call path. Currently the only export happens implicitly when `SprintCompleteMsg` triggers `createSpecSummaryFromSprintState()`, which does NOT call `ExportToSpec()` -- it creates a display-only `SpecSummary` struct. The actual structured export is deferred. Phase 2 doesn't change this, but should document that the export path remains accessible from the GurgehView post-sprint.

### P1-4: `NavigateBackMsg` behavior changes when Gurgeh owns the flow

**Location:** `unified_app.go:1613-1636` (`navigateBack`)

Currently, `navigateBack()` uses `a.onboardingState` to determine where to go:
- `OnboardingEpicReview` -> back to kickoff
- `OnboardingTaskReview` -> back to epic review
- `OnboardingComplete` -> back to Bigend (first dashboard view)

After Phase 2, `NavigateBackMsg` from within Gurgeh should NOT exit to Bigend. It should navigate backward within Gurgeh's internal state machine. Only an explicit tab switch or `SprintExitRequestedMsg` should leave Gurgeh.

**Recommendation:** The Gurgeh view must intercept `NavigateBackMsg` and handle it internally. When the user is on the kickoff screen inside Gurgeh and presses Esc (which currently emits `SprintExitRequestedMsg`), Gurgeh should either show an empty/welcome state or do nothing -- it should NOT exit to another tab.

### P2-1: `NavigateToKickoffMsg` clears spec-related state

**Location:** `unified_app.go:1644-1665` (`navigateToKickoff`)

`navigateToKickoff()` clears `generatedEpics`, `generatedTasks`, `projectID`, `projectName`, `projectDesc`. This is correct behavior (starting over clears the slate). After Phase 2, the Gurgeh view's equivalent must also clear any active sprint in the Orchestrator, or the SprintView will attempt to resume a stale sprint on the next kickoff.

Currently there's no `Orchestrator.Clear()` method. The Orchestrator is simply recreated via the factory. Phase 2 must ensure that returning to kickoff creates a fresh SprintView with a fresh Orchestrator, not reuse the old one.

**Recommendation:** When GurgehView navigates back to kickoff, it should set its SprintView reference to nil. The next project creation should invoke the factory to create a fresh SprintView (and thus a fresh Orchestrator).

### P2-2: `OnboardingOrchestrator` in `onboarding.go` is partially redundant

**Location:** `internal/tui/onboarding.go:111-247`

The `OnboardingOrchestrator` struct was designed as an intermediate controller but is currently unused by `UnifiedApp` (which handles everything directly). Phase 2 could either:
1. Use `OnboardingOrchestrator` as the internal state machine for Gurgeh
2. Inline the state machine directly in the expanded GurgehView

Option 1 is cleaner but requires wiring the 12+ message handlers into `OnboardingOrchestrator.Update()`. Option 2 is what the plan implies. Either way, the `OnboardingOrchestrator` should be deleted or refactored -- leaving it as dead code increases confusion.

### P2-3: Window size calculation changes when breadcrumb moves inside content area

**Location:** `unified_app.go:333-361` (WindowSizeMsg handler)

Currently, `headerHeight` is 4 in `ModeOnboarding` (tabs + breadcrumb) and 3 in `ModeDashboard` (tabs only). After Phase 2, there is no onboarding mode -- it's always dashboard mode with `headerHeight = 3`. The breadcrumb moves inside Gurgeh's content area.

This means Gurgeh's content area gets 1 extra row compared to the current onboarding mode. The Gurgeh view must account for this internally -- if it renders its own breadcrumb, it needs to subtract that from the available height for its sub-views.

**Recommendation:** The expanded GurgehView should track its own `breadcrumbHeight` (1 when in onboarding sub-flow, 0 when showing spec list) and subtract it from the `WindowSizeMsg.Height` before forwarding to sub-views.

---

## Improvements Suggested

### 1. Define a `GurgehInternalState` enum parallel to `OnboardingState`

**Rationale:** After Phase 2, Gurgeh needs its own state machine. Rather than reusing `OnboardingState` (which includes concepts like `OnboardingComplete` that map to "exit Gurgeh"), create a new enum:

```go
type GurgehState int
const (
    GurgehKickoff GurgehState = iota  // Project selection/creation
    GurgehSprint                       // 8-phase sprint
    GurgehSpecReview                   // Post-sprint spec summary
    GurgehEpicReview                   // Epic proposals
    GurgehTaskReview                   // Task proposals
    GurgehSpecList                     // Dashboard: browsing existing specs
)
```

This clearly separates Gurgeh's internal navigation from the app-level tab system.

### 2. Extract `AgentProvider` interface from `UnifiedApp`

**Rationale:** Agent management (detection, selection, generation) is currently embedded in `UnifiedApp`. After Phase 2, Gurgeh needs agent access for epic/task generation. Rather than moving all agent state into Gurgeh, define:

```go
type AgentProvider interface {
    Agent() *agent.Agent
    SelectedAgentName() string
    AgentSelector() *pkgtui.AgentSelector
}
```

The parent app implements this and passes it to Gurgeh. This keeps agent lifecycle in one place.

### 3. Add integration test: full spec lifecycle through Gurgeh view

**Rationale:** The spec flow is currently tested indirectly through `unified_app_test.go`. After Phase 2, the same flow runs through GurgehView. Add a test that:
1. Creates a GurgehView with mock factories
2. Sends `ProjectCreatedMsg` with scan artifacts
3. Verifies SprintView receives artifacts
4. Simulates `SprintCompleteMsg`
5. Verifies SpecSummary is created with correct fields
6. Simulates `SpecAcceptedMsg`
7. Verifies epic generation triggers

This test should run with `-race` per the institutional learnings from `arbiter-state-pointer-escape`.

### 4. Preserve `createSpecSummaryFromSprintState()` in a shared location

**Rationale:** This function converts `SprintState` sections into a display-friendly `SpecSummary`. It currently lives in `unified_app.go`. After Phase 2, it should live in the `tui` package (near `types.go` where `SpecSummary` is defined) or in the `views` package, so both the GurgehView and any future consumers can use it.

### 5. Add `scanResultToArtifacts()` to a shared location

**Rationale:** Same as above -- this conversion function bridges TUI types (`CodebaseScanResultMsg.PhaseArtifacts`) to arbiter types (`scan.Artifacts`). It's pure data transformation and should not be tied to any specific view host.

---

## Overall Assessment

Phase 2 is architecturally sound but under-specified in its spec-system impact. The plan correctly identifies the structural changes (move state, handlers, breadcrumb, view factories) but omits critical data-flow details:

1. **Agent ownership** is a P0 gap -- epic/task generation will fail without it.
2. **SprintStateProvider type assertion** is a P0 gap -- sprint completion will silently fail to create SpecSummary.
3. **Message forwarding** must be explicit -- the Gurgeh view needs a pass-through for messages it doesn't handle.
4. **Shared utility functions** (`createSpecSummaryFromSprintState`, `scanResultToArtifacts`) should be extracted before the move to avoid duplication.

The 8-phase sprint model, Orchestrator concurrency model, consistency engine, confidence scoring, spec persistence, and spec export are all encapsulated within the `arbiter` package and are NOT directly affected by Phase 2. The risk is entirely in the TUI wiring layer that connects these components.

**Verdict:** Phase 2 is safe to proceed with a design spike, provided the spike addresses the 2 P0 issues (agent ownership and SprintStateProvider) and adds the integration test (Improvement 3) as a pre-merge gate.
