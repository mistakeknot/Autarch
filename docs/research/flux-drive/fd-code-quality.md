# Code Quality Review: Phase 2 -- Gurgeh Absorbs Onboarding

**Reviewer:** Code Quality Reviewer (conventions alignment)
**Plan:** `docs/plans/2026-02-05-unified-tui-navigation-design.md`, Phase 2
**Date:** 2026-02-06

---

## Summary

Phase 2 is the most architecturally consequential change in Autarch's TUI layer since the UnifiedApp was introduced. The plan correctly identifies the scope (800-1200 lines) and the files affected, but leaves concrete code organization underspecified. After reviewing the actual source code in `internal/tui/unified_app.go` (2195 lines), `internal/tui/app.go` (466 lines), `internal/tui/views/gurgeh.go` (313 lines), `internal/tui/onboarding.go` (248 lines), `internal/tui/breadcrumb.go` (192 lines), `internal/tui/messages.go` (273 lines), `internal/tui/types.go` (227 lines), and `cmd/autarch/main.go` (610 lines), this review identifies 7 specific issues: 2 at P1 severity and 5 at P2. The plan is broadly aligned with project conventions but needs concrete structural decisions before implementation begins.

---

## Conventions Check

### Conventions the project follows (from CLAUDE.md, AGENTS.md, existing code)

1. **Package layout:** `internal/{tool}/` for tool-specific code, `pkg/` for shared. TUI views live in `internal/tui/views/`, shared TUI components in `pkg/tui/`.
2. **View interface:** All TUI views implement `pkgtui.View` (defined in `pkg/tui/`): `Init()`, `Update()`, `View()`, `Focus()`, `Blur()`, `Name()`, `ShortHelp()`.
3. **Message types:** Defined in `internal/tui/messages.go` as exported structs. Named `{Noun}{Verb}Msg` (e.g., `ProjectCreatedMsg`, `SprintCompleteMsg`, `EpicsGeneratedMsg`).
4. **Interface-based composition:** Small setter interfaces for optional capabilities: `agentSelectorSetter`, `chatSettingsSetter`, `inputClearer`, `slashCommandHandler`, `ChatStreamSetter`, `DocumentSnapshotter`, `SprintStateProvider`, `SprintStarter`. Checked at runtime via type assertion.
5. **View factory pattern:** `UnifiedApp` receives factory functions (`createKickoffView`, `createSprintView`, etc.) injected from `cmd/autarch/main.go` to avoid import cycles between `internal/tui` and `internal/tui/views`.
6. **Error handling:** `fmt.Errorf("context: %w", err)` -- no panics in TUI code.
7. **Test patterns:** Mock views implementing `pkgtui.View` with minimal stubs (e.g., `noopDashboardView`, `mockSprintView`). Tests focus on message routing and state transitions, not rendering. No table-driven tests in TUI layer.
8. **Naming:** PascalCase for types/exports, camelCase for locals. View types named `{Tool}View` (e.g., `GurgehView`, `BigendView`, `SprintView`, `KickoffView`). Constructor: `New{Type}()`.
9. **Callbacks via struct methods:** Views expose `SetCallbacks()` or `Set{Name}Callback()` for parent-child communication. Callbacks return `tea.Cmd`.
10. **Commit messages:** `type(scope): description` -- e.g., `feat(tui):`, `refactor(tui):`.
11. **Bubble Tea patterns:** `tea.Batch()` for combining commands. `func() tea.Msg { return ... }` for deferred messages. Method receivers on the app struct, not free functions.

### Conventions the plan adheres to

- Correctly identifies `internal/tui/views/gurgeh.go` as the target location for absorbed onboarding logic.
- Correctly identifies the view factory wiring in `cmd/autarch/main.go` as needing changes.
- Correctly scopes the breadcrumb move into Gurgeh's content area (matching the convention that tool-specific UI lives inside tool views).
- Correctly identifies `App` and `UnifiedApp` as needing merger.

### Conventions the plan may violate (see Issues below)

- Does not specify whether the new Gurgeh onboarding component will be a new type or embedded in `GurgehView`.
- Does not address what happens to the 12+ message types currently defined in `internal/tui/messages.go`.
- Does not address what happens to the 8 setter interfaces in `internal/tui/types.go`.
- Does not specify where `scanCodebase()` and its helper functions (260 lines) should land.

---

## Section-by-Section Review

### "Move onboarding state machine from UnifiedApp into GurgehView or new GurgehOnboardingView"

The plan is ambiguous ("GurgehView or new GurgehOnboardingView"). This is the most critical architectural decision in the refactor and should not be left to implementation time.

**Current `GurgehView`** (`internal/tui/views/gurgeh.go`, 313 lines) is a lightweight spec browser: it lists specs, shows details, and has a stub chat pane. It implements `pkgtui.View` and `pkgtui.SidebarProvider`.

**Current onboarding flow** (`unified_app.go`, ~800 lines of logic) is a full state machine with codebase scanning, sprint orchestration, epic generation, task generation, agent streaming, and multi-view transitions. It holds 11 onboarding-specific struct fields and handles 12+ message types.

Cramming this into `GurgehView` would turn a 313-line spec browser into a 1100+ line monster. A separate `GurgehOnboardingView` is the right call, but then `GurgehView` needs to become a container that switches between `GurgehOnboardingView` and the spec browser based on internal state.

### "Move all onboarding message handlers out of UnifiedApp.Update()"

The plan does not specify which message types remain app-level and which become Gurgeh-internal. This matters because some messages (`AgentStreamMsg`, `GenerationErrorMsg`, `AgentRunStartedMsg`) are currently used by both onboarding flows AND dashboard views. Moving them wholesale into Gurgeh would break the SprintView when accessed from the dashboard.

### "Move breadcrumb into Gurgeh's content area"

Straightforward. The `Breadcrumb` type (`internal/tui/breadcrumb.go`, 192 lines) is self-contained with its own `Update()` and `View()`. It depends only on `OnboardingState` (defined in `internal/tui/onboarding.go`) and `pkgtui.CommonKeys`. Moving it is a clean lift.

However, the plan does not address the header height math. Currently `UnifiedApp.View()` uses `headerHeight := 4` in onboarding mode (tabs + breadcrumb) and `headerHeight := 3` in dashboard mode. If breadcrumb moves into Gurgeh's content area, the header is always 3 -- but `GurgehView` must account for the breadcrumb height in its own content height calculation.

### "Merge App and UnifiedApp into a single implementation"

`App` (466 lines) and `UnifiedApp` (2195 lines) share significant overlapping logic: overlay rendering, tab switching, palette management, log pane handling, footer rendering. The plan says "merge" but does not specify the direction. The answer is obvious (keep `UnifiedApp`, delete `App`) but should be stated explicitly because `App` has some simpler patterns (e.g., `doSwitchTab` is cleaner than `switchDashboardTab`/`switchToTab`) that may be worth adopting.

### "Remove --skip-onboard flag (deprecation warning)"

This also means removing the entire `skipOnboard` code path in `cmd/autarch/main.go` (lines 137-177) which constructs views directly without factories and calls `tui.RunWithOpts()` instead of `tui.RunUnifiedWithOpts()`. The plan should note this is a secondary benefit of the App/UnifiedApp merge.

### "Rewire view factories in cmd/autarch/main.go"

The plan lists this as a single bullet but it is the most error-prone part. Currently `main.go` wires 7 factory functions with complex callback chains. After the refactor, most of these should move into Gurgeh's constructor or an options struct, reducing `main.go` to creating dashboard views only. The plan should be explicit about this.

---

## Issues Found

### Issue 1 (P1): No concrete type design for GurgehView's internal state machine

- **Location:** Plan section "Move onboarding state machine from UnifiedApp into GurgehView or new GurgehOnboardingView"
- **Convention:** The project uses focused types -- `SprintView` (sprint orchestration), `KickoffView` (project creation), `GurgehView` (spec browser). Each implements `pkgtui.View`.
- **Violation:** The plan leaves the core structural decision ambiguous. "GurgehView or new GurgehOnboardingView" is not a plan -- it is two incompatible designs.
- **Fix:** Choose the container pattern. `GurgehView` becomes a container with an `activeSubView View` field. In onboarding mode, `activeSubView` is a new `GurgehFlowView` that owns the state machine, breadcrumb, codebase scanner, view factories, and all onboarding message types. In browser mode, `activeSubView` is the current spec browser (renamed to `GurgehBrowserView` or kept as an internal struct). This matches the existing pattern: `SprintView` already contains a `PhaseSidebar`, `SprintDocPanel`, and `ChatPanel` as internal sub-components.

  Concrete struct sketch:
  ```go
  // internal/tui/views/gurgeh.go -- container
  type GurgehView struct {
      client      *autarch.Client
      activeMode  gurgehMode // modeBrowser | modeFlow
      browserView *gurgehBrowserView
      flowView    *GurgehFlowView
      width, height int
  }

  // internal/tui/views/gurgeh_flow.go -- absorbed from unified_app.go
  type GurgehFlowView struct {
      // Onboarding state (moved from UnifiedApp)
      onboardingState  OnboardingState
      breadcrumb       *Breadcrumb
      currentView      View
      projectID        string
      ...
      // View factories (moved from UnifiedApp)
      createKickoffView func() View
      createSprintView  func(string) View
      ...
  }
  ```

  This keeps files under 500 lines each and maintains the single-responsibility principle.

### Issue 2 (P1): Message ownership boundary not defined

- **Location:** Plan section "Move all onboarding message handlers out of UnifiedApp.Update()"
- **Convention:** Messages in `internal/tui/messages.go` are the contract between `UnifiedApp` and `internal/tui/views/`. The `Update()` method of `UnifiedApp` is the central message router.
- **Violation:** The plan does not specify which messages stay in the app-level router and which become Gurgeh-internal. This is not a detail -- it is the refactor.
- **Fix:** Partition messages into three categories:

  **App-level (remain in unified app):** `AgentSelectedMsg`, `LogBatchMsg`, `SlashCommandMsg`, `logPaneAutoHideMsg`, `tea.WindowSizeMsg`, `tea.KeyMsg`. These are cross-cutting concerns.

  **Gurgeh-internal (move into GurgehFlowView):** `ProjectCreatedMsg`, `InterviewCompleteMsg`, `SuggestionsReadyMsg`, `SpecAcceptedMsg`, `EpicsGeneratedMsg`, `EpicsAcceptedMsg`, `TasksGeneratedMsg`, `TasksAcceptedMsg`, `OnboardingCompleteMsg`, `ScanCodebaseMsg`, `CodebaseScanResultMsg`, `ScanProgressMsg`, `scanProgressWithContinuation`, `NavigateToTaskDetailMsg`, `NavigateBackMsg`, `NavigateToKickoffMsg`, `NavigateToStepMsg`, `SprintCompleteMsg`.

  **Shared (used by both onboarding and dashboard):** `GeneratingMsg`, `GenerationErrorMsg`, `AgentNotFoundMsg`, `AgentStreamMsg`, `agentStreamWithContinuation`, `AgentRunStartedMsg`, `AgentRunFinishedMsg`, `AgentEditSummaryMsg`, `RevertLastRunMsg`. These should remain in `internal/tui/messages.go` and be forwarded to the active view by the app-level router.

  The "Gurgeh-internal" messages can move to a new file `internal/tui/views/gurgeh_messages.go` if they need to be in the `views` package, or they can stay in `internal/tui/messages.go` and be consumed by `GurgehFlowView` via its `Update()` method. The latter avoids import cycle issues since `views` already imports `tui`.

### Issue 3 (P2): scanCodebase() and helpers are 260 lines with no clear home

- **Location:** Plan section "Remove onboarding state, breadcrumb, ~400 lines of transition handlers" from `unified_app.go`
- **Convention:** The project separates concerns: `internal/gurgeh/exploration/` handles codebase exploration, `internal/gurgeh/arbiter/scan/` handles scan artifacts. The TUI layer should not own scan orchestration.
- **Violation:** `scanCodebase()` in `unified_app.go` (lines 876-930) plus helper functions `extractString`, `extractVisionSummary`, `extractProblemSummary`, `extractUsersSummary`, `waitForScanProgress`, `scanResultToArtifacts`, and all the `to*` conversion functions (lines 932-1211) total ~280 lines. The plan says "move into gurgeh view" but does not address this blob specifically.
- **Fix:** Extract scan orchestration into a dedicated file `internal/tui/views/gurgeh_scan.go` (or better, move the conversion functions into `internal/gurgeh/exploration/` where they logically belong). The `scanCodebase` method itself should live on `GurgehFlowView` since it initiates the scan and routes progress messages. The `to*` conversion functions (`toPhaseArtifacts`, `toVisionArtifact`, etc.) are pure data mapping and should live near their source types, not in the TUI layer.

### Issue 4 (P2): No test strategy for the refactor

- **Location:** Plan section "Files affected" lists "Tests | Update for new structure" with no detail.
- **Convention:** AGENTS.md specifies "TDD for behavior changes" and "Small unit tests over broad integration tests." Existing tests in `internal/tui/unified_app_test.go` (381 lines) test specific message routing (e.g., `TestProjectCreatedMsgTransitionsToSprintView`, `TestSprintCompleteMsgTransitionsToSpecSummary`). These are the right kind of tests.
- **Violation:** The plan does not specify which existing tests break, which need migration, or which new tests to add.
- **Fix:** Define the test migration plan:

  **Tests that break and need rewriting** (currently test `UnifiedApp` message routing that will move to `GurgehFlowView`):
  - `TestProjectCreatedMsgTransitionsToSprintView` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestProjectCreatedMsgFallsBackWithoutSprintFactory` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestSprintCompleteMsgTransitionsToSpecSummary` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestSprintCompleteMsgFallsBackToOnboardingComplete` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestScanResultSetsInterviewBreadcrumb` in `/root/projects/Autarch/internal/tui/unified_app_test.go`

  **Tests that should survive unchanged** (test app-level behavior):
  - `TestUnifiedAppCtrlLeftCyclesBack` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestUnifiedAppDoubleCtrlCQuitsWithHelpVisible` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestChatSettingsTogglePersistsAndApplies` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestAgentStreamMessagesRouteToChat` in `/root/projects/Autarch/internal/tui/unified_app_test.go`
  - `TestCommaDoesNotOpenChatSettingsWhenInputFocused` in `/root/projects/Autarch/internal/tui/unified_app_test.go`

  **New tests to add** for `GurgehFlowView`:
  - `TestGurgehFlowProjectCreatedTransitionsToSprint` -- same as old test but on `GurgehFlowView`
  - `TestGurgehFlowSprintCompleteTransitionsToSpecSummary`
  - `TestGurgehViewSwitchesBetweenBrowserAndFlow`
  - `TestGurgehFlowOnboardingCompleteExitsToContainer`

  **Tests that should be deleted** (test `App` struct which gets merged away):
  - `TestAppHelpOverlayToggles` in `/root/projects/Autarch/internal/tui/app_test.go`
  - `TestAppCtrlCQuitsWithPaletteVisible` in `/root/projects/Autarch/internal/tui/app_test.go`

### Issue 5 (P2): OnboardingState and Breadcrumb package placement after move

- **Location:** Plan section "Move onboarding.go into gurgeh or restructure"
- **Convention:** Types shared across `internal/tui/` and `internal/tui/views/` are defined in `internal/tui/` to avoid import cycles (`views` imports `tui`, not vice versa). This is why `OnboardingState`, `SpecSummary`, `InterviewQuestion`, and the setter interfaces live in `internal/tui/`.
- **Violation:** The plan says "Move into gurgeh or restructure" without acknowledging the import cycle constraint. `OnboardingState` is used by `Breadcrumb` (which would move into `views` package) AND by `UnifiedApp` (which stays in `internal/tui`). If `OnboardingState` moves into `views`, `internal/tui` cannot import it.
- **Fix:** Keep `OnboardingState` and `Breadcrumb` in `internal/tui/` where they currently are. `GurgehFlowView` (in `internal/tui/views/`) already imports `internal/tui` -- it can use `tui.OnboardingState` and `tui.Breadcrumb` directly. The files do not need to move. What changes is who *uses* them: `GurgehFlowView` instead of `UnifiedApp`. The plan should say "Breadcrumb usage moves from UnifiedApp to GurgehFlowView" rather than "Move breadcrumb.go into gurgeh view."

### Issue 6 (P2): View factory injection needs simplification

- **Location:** Plan section "Rewire view factories in cmd/autarch/main.go"
- **Convention:** Currently `main.go` calls `app.SetViewFactories()` with 6 positional function arguments (lines 184-272) plus `app.SetSprintViewFactory()` separately. This is brittle and hard to read.
- **Violation:** The plan says "Simplify; remove separate --skip-onboard path" but does not address the factory injection pattern. If onboarding factories move into Gurgeh, the main.go wiring should also improve.
- **Fix:** Replace the positional `SetViewFactories()` call with an options struct:

  ```go
  // internal/tui/views/gurgeh_flow.go
  type GurgehFlowOpts struct {
      CreateKickoffView     func() tui.View
      CreateSprintView      func(string) tui.View
      CreateSpecSummaryView func(*tui.SpecSummary, *research.Coordinator) tui.View
      CreateEpicReviewView  func([]epics.EpicProposal) tui.View
      CreateTaskReviewView  func([]tasks.TaskProposal) tui.View
      CreateTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) tui.View
  }
  ```

  This matches the existing `SprintViewOpts` pattern in `internal/tui/views/sprint_view.go` (line 43) and eliminates the 6-positional-argument anti-pattern. The merged app no longer needs `SetViewFactories` at all -- it only creates dashboard views.

### Issue 7 (P2): Complexity budget -- the plan adds no new functionality

- **Location:** Overall plan assessment
- **Convention:** CLAUDE.md Workflow Discipline says "Pause after major refactoring" and "Verify end-to-end before completion."
- **Violation:** 800-1200 lines of refactoring with zero new user-visible behavior is a high-risk change. The plan does not define intermediate checkpoints or rollback criteria.
- **Fix:** Define checkpoints:

  1. **Checkpoint A:** Extract `GurgehFlowView` into `internal/tui/views/gurgeh_flow.go` with all onboarding logic. `UnifiedApp` delegates to it but still owns the message routing. Tests pass. Verify: `./dev autarch tui` and `./dev autarch tui --skip-onboard` both work. This is the most dangerous step; if it fails, the codebase is in an inconsistent state.

  2. **Checkpoint B:** Move message routing from `UnifiedApp.Update()` into `GurgehFlowView.Update()`. `UnifiedApp` forwards unhandled messages to the active dashboard view (which is `GurgehView` by default). Tests pass.

  3. **Checkpoint C:** Delete `App` struct and `--skip-onboard` path. `RunWithOpts` and `RunUnifiedWithOpts` merge into a single `RunWithOpts`. Tests pass.

  4. **Checkpoint D:** Clean up -- remove dead code, update CLAUDE.md, update `docs/plans/STATUS.md`.

  Each checkpoint should be a separate commit so rollback is possible. This matches the existing commit style (e.g., `3e2a7f1 fix(P1): TUI dimension overflow + signal broker silent drops` -- scoped, specific).

---

## Improvements Suggested

### 1. Add a file-by-file changelist with line estimates

**Rationale:** The plan lists 5 files but the actual change involves 8-10 files when accounting for new files and test files. A concrete changelist prevents scope creep during implementation.

Suggested changelist:

| File | Action | Estimated Lines |
|------|--------|----------------|
| `internal/tui/views/gurgeh.go` | Refactor into container with browser/flow modes | +80, -200 (net: lighter) |
| `internal/tui/views/gurgeh_flow.go` | **New file.** Absorb onboarding state machine from `unified_app.go` | +500 |
| `internal/tui/views/gurgeh_scan.go` | **New file.** Absorb `scanCodebase()` and conversion helpers | +280 |
| `internal/tui/unified_app.go` | Remove onboarding state, handlers, scan logic. Keep app-level concerns only. | -800 |
| `internal/tui/app.go` | **Delete.** Merge useful patterns into `unified_app.go`. | -466 |
| `cmd/autarch/main.go` | Remove `--skip-onboard` path, simplify factory wiring | -60 |
| `internal/tui/unified_app_test.go` | Remove migrated tests, add forwarding tests | -150, +50 |
| `internal/tui/views/gurgeh_flow_test.go` | **New file.** Migrated onboarding transition tests | +200 |
| `internal/tui/app_test.go` | **Delete.** | -50 |
| `internal/tui/onboarding.go` | Keep in place (see Issue 5) | ~0 |
| `internal/tui/breadcrumb.go` | Keep in place (see Issue 5) | ~0 |

### 2. Define the "onboarding complete" handoff from GurgehFlowView to its container

**Rationale:** Currently `OnboardingCompleteMsg` triggers `enterDashboard()` on `UnifiedApp`. After the refactor, `GurgehFlowView` finishes its flow and needs to signal its container (`GurgehView`) to switch to browser mode. This is a new message flow that does not exist today.

Suggested approach: `GurgehFlowView` emits a new `GurgehFlowCompleteMsg` (internal to the views package). `GurgehView` catches it in its `Update()` and switches `activeMode` to `modeBrowser`. This is analogous to how `SprintCompleteMsg` currently signals the parent.

### 3. Address the `--tool=gurgeh` startup path

**Rationale:** After the refactor, `autarch tui` should start with Gurgeh active and its flow view showing (the kickoff screen). `autarch tui --tool=gurgeh` should do the same thing. But `autarch tui --tool=bigend` should start with Bigend active and Gurgeh in browser mode (not starting the flow). The plan does not specify this behavior.

---

## Overall Assessment

**Alignment: Acceptable -- needs concrete structural decisions before implementation.**

The plan correctly identifies the problem (onboarding entangled in the app shell), the solution direction (Gurgeh absorbs onboarding), and the scope (800-1200 lines). It follows project conventions in its general approach. However, it defers the most critical architectural decisions (type design, message ownership, package placement) to implementation time, which for an 800+ line refactor across 8-10 files is risky.

**Top 3 changes for better consistency:**

1. **(P1) Commit to the container pattern** for `GurgehView` with a new `GurgehFlowView` type in a dedicated file (`gurgeh_flow.go`). Do not expand the existing `GurgehView` into a monolith. Sketch the struct fields.

2. **(P1) Define the message ownership partition** -- which messages route through the app shell vs. which are handled internally by `GurgehFlowView`. This determines the entire refactoring strategy.

3. **(P2) Add checkpoints** -- the refactor should be 3-4 commits, not a single 800-line commit. Each checkpoint should be independently testable and deployable.
