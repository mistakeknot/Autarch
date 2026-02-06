# Bubble Tea v1 Specialist Review: Unified TUI Navigation Design

**Reviewer:** BT v1 Specialist (Opus 4.6)
**Plan:** `docs/plans/2026-02-05-unified-tui-navigation-design.md`
**Date:** 2026-02-06
**Focus:** Keybinding portability, Phase 2 message routing, Phase 3 overlay rendering, BT v1 compatibility

---

## Summary

The plan is well-structured and shows clear evidence of learning from previous BT v1 pitfalls (Ctrl+number dropped, single-letter alias collisions caught). Phase 1 is already shipped and looks clean. Phase 2 is the high-risk work: moving ~400 lines of onboarding state machine from `UnifiedApp` into Gurgeh requires careful message routing redesign. Phase 3's Signals overlay is feasible but Ctrl+Shift+S has terminal portability concerns that should be addressed now. The plan correctly identifies the two-App problem but underestimates the message forwarding complexity in Phase 2.

---

## Section-by-Section Review

### Tab Bar Always Visible (Phase 1 -- SHIPPED)

The tab bar renders in both `ModeOnboarding` and `ModeDashboard` via `unified_app.go:View()` lines 1917-1921. The header height calculation correctly accounts for the breadcrumb line in onboarding mode (`headerHeight = 4` vs `headerHeight = 3`). This is the pattern documented in MEMORY.md about lipgloss `Height()` being a floor, not a ceiling.

Phase 1 delivery is solid. Tab switching during onboarding exits onboarding and enters dashboard (`switchToTab` at line 1876-1886). The `enterDashboard()` call creates fresh dashboard views, which is correct -- it avoids stale state from a partially-completed onboarding.

### Keybindings

Slash commands (`/big`, `/gur`, `/cold`, `/pol`) are registered in `GlobalCommands()` at `pkg/tui/command_picker.go:320-324` and handled in `UnifiedApp.Update()` at lines 401-412. The 3-letter alias choice is correct -- verified no collisions with existing aliases.

`Ctrl+Left/Right` cycling works in both modes (lines 511-517). Uses modular arithmetic for wrapping, which is correct.

The decision to drop `Ctrl+N` and `Alt+N` direct keybindings is correct for BT v1. This is a well-documented limitation.

### Simplified Mode Architecture (Phase 2)

This is the critical section. The plan says "800-1200 lines changed, 5-8 files" which is realistic but the complexity lies not in line count but in message routing correctness.

**Current State (what must move):**

1. **12+ message types** routed through `UnifiedApp.Update()` (lines 536-699):
   - `ProjectCreatedMsg`, `InterviewCompleteMsg`, `SuggestionsReadyMsg`, `SpecAcceptedMsg`
   - `EpicsGeneratedMsg`, `EpicsAcceptedMsg`, `TasksGeneratedMsg`, `TasksAcceptedMsg`
   - `SprintCompleteMsg`, `OnboardingCompleteMsg`, `ScanCodebaseMsg`, `CodebaseScanResultMsg`
   - `NavigateToTaskDetailMsg`, `NavigateBackMsg`, `NavigateToKickoffMsg`, `NavigateToStepMsg`
   - `GeneratingMsg`, `GenerationErrorMsg`, `AgentNotFoundMsg`
   - `scanProgressWithContinuation`, `agentStreamWithContinuation`

2. **6 view factories** injected via `cmd/autarch/main.go` (lines 184-272):
   - `createKickoffView`, `createArbiterView`, `createSpecSummaryView`
   - `createEpicReviewView`, `createTaskReviewView`, `createTaskDetailView`
   - Plus `createDashboardViews` and `createSprintView`

3. **Onboarding-specific struct fields** on `UnifiedApp` (lines 42-53):
   - `onboardingState`, `breadcrumb`, `currentView`
   - `projectID`, `projectName`, `projectDesc`
   - `interviewAnswers`, `generatedEpics`, `generatedTasks`
   - `researchCoord`, `codingAgent`

### Signals Overlay (Phase 3)

The plan proposes `Ctrl+Shift+S` as the toggle keybinding and a floating overlay rendered on top of the active tab content.

---

## Issues Found

### P1-1: Ctrl+Shift+S is not portable in BT v1

**Section:** Signals Overlay (Phase 3), Keybindings
**File:** Plan section "Keybindings" and "Signals Overlay"

BT v1 parses `Ctrl+Shift+S` inconsistently across terminals. Some terminals send `\x13` (same as `Ctrl+S`), others send nothing or a CSI sequence that BT v1 doesn't understand. `Ctrl+S` itself is traditionally XOFF (terminal flow control freeze) -- many terminals will freeze input entirely unless `stty -ixon` is set.

The plan already uses `/signals` (`/sig`) as the slash command alternative, which is correct. But the plan should explicitly note that `Ctrl+Shift+S` is a best-effort binding that will fail silently on many terminals, and `/sig` is the reliable path. The implementation should parse `Ctrl+Shift+S` if BT v1 delivers it but not rely on it.

**Recommendation:** Replace `Ctrl+Shift+S` with a safe binding like `Ctrl+\\` (which BT v1 receives as `\x1c` reliably) or drop the direct keybinding entirely and rely solely on `/sig`. The plan already has the slash command, so the keybinding is a nice-to-have.

### P1-2: Phase 2 message routing requires a forwarding boundary

**Section:** Phase 2: Gurgeh Absorbs Onboarding
**Files:** `internal/tui/unified_app.go`, future `internal/tui/views/gurgeh.go`

The plan says "move onboarding message handlers out of `UnifiedApp.Update()`" but doesn't define the message forwarding boundary. Currently, messages like `ProjectCreatedMsg`, `EpicsGeneratedMsg`, etc. bubble up to `UnifiedApp.Update()` because child views emit them as `tea.Cmd` returns. After Phase 2, these messages need to be caught and handled **within** the expanded `GurgehView`.

The problem: BT v1's `Update()` returns `(tea.Model, tea.Cmd)`. If `GurgehView.Update()` returns a `tea.Cmd` that produces `ProjectCreatedMsg`, that message will be delivered to `UnifiedApp.Update()`, not back to `GurgehView.Update()`. This means either:

(a) `UnifiedApp` must still know about all onboarding message types and forward them to the active Gurgeh view, or
(b) `GurgehView` must handle all onboarding messages internally without ever emitting them as `tea.Cmd` returns -- which means restructuring the view factory callbacks.

Option (a) defeats the purpose of the refactor. Option (b) is correct but means `GurgehView` needs to own its own goroutine coordination for scanning, epic generation, task generation, etc., rather than delegating to `UnifiedApp`.

**Recommendation:** The plan should specify that Phase 2 uses option (b): `GurgehView` owns all onboarding tea.Cmd/tea.Msg flows internally. The only message that should escape to `UnifiedApp` is a hypothetical `GurgehOnboardingDoneMsg` that signals dashboard transition. This is a design decision that should be documented before implementation begins, as it determines the entire refactor shape.

### P1-3: Two App structs create divergent behavior

**Section:** Phase 2: Gurgeh Absorbs Onboarding
**Files:** `internal/tui/app.go` (App), `internal/tui/unified_app.go` (UnifiedApp)

The plan says "Merge `App` and `UnifiedApp` into a single implementation" but this is more complex than it appears. The two structs have different:

- **Key handling:** `App` delegates to `pkgtui.HandleCommon()` (which calls `tea.Quit` on first Ctrl+C); `UnifiedApp` implements double-Ctrl+C quit with a 500ms timer. After merge, the double-Ctrl+C behavior must be preserved.
- **View lifecycle:** `App.Init()` initializes ALL views immediately; `UnifiedApp.Init()` only initializes the kickoff view. After merge, the behavior should match `UnifiedApp` (lazy init) to avoid unnecessary API calls on startup.
- **Message passthrough:** `App` forwards ALL unhandled messages to the active view only; `UnifiedApp` has a complex routing table. The merged version needs the routing table, but `App`'s simpler "pass to active view" pattern is actually what Phase 2 should converge toward.
- **Slash command handling:** `App` doesn't handle `SlashCommandMsg` at all -- it relies on Ctrl+key bindings. `UnifiedApp` has full slash command routing. The merged version needs slash commands.

**Recommendation:** The plan should note that "merge" means `App` is deleted and `UnifiedApp` is simplified. It is NOT a two-way merge. The `--skip-onboard` flag should create the same `UnifiedApp` but skip to dashboard mode immediately. This is clearer than "merge."

### P2-1: `sendToCurrentView` drops returned tea.Cmd

**Section:** Not explicitly in the plan, but relevant to Phase 2 refactoring
**File:** `internal/tui/unified_app.go:1280-1287`

```go
func (a *UnifiedApp) sendToCurrentView(msg tea.Msg) {
    if a.currentView == nil {
        return
    }
    var cmd tea.Cmd
    a.currentView, cmd = a.currentView.Update(msg)
    _ = cmd  // <-- BUG: discarded
}
```

This helper is called from `finalizeAgentRun` (line 1246, 1254) and `revertLastRun` (line 1849). Any `tea.Cmd` returned by the view in response to `AgentRunFinishedMsg` or `AgentEditSummaryMsg` or `RevertLastRunMsg` is silently dropped. If the view wants to trigger a follow-up action (e.g., auto-scroll, refresh sidebar, emit a status message), it won't work.

This is a pre-existing bug, not introduced by the plan, but Phase 2 refactoring should fix it since this code will be touched anyway.

**Recommendation:** Change `sendToCurrentView` to return `tea.Cmd`, or refactor callers to use the standard `Update()` pattern. At minimum, `_ = cmd` should become a logged warning so dropped commands are visible during development.

### P2-2: Ctrl+Right collision between tab cycling and sprint accept

**Section:** Keybindings
**Files:** `internal/tui/unified_app.go:511-517`, `internal/tui/views/sprint_view.go:303`

`Ctrl+Right` is used for two different things:
- In `UnifiedApp.Update()` (line 515): cycle to next tab
- In `SprintView.Update()` (line 303): accept current draft

Currently this works by accident: `UnifiedApp` processes `Ctrl+Right` before passing to the current view. But this means a user in the sprint flow who presses `Ctrl+Right` intending to accept the draft will instead switch tabs.

Phase 1 already shipped this behavior. The sprint view's `Ctrl+Right` accept binding is dead when accessed through the unified TUI.

**Recommendation:** The plan should explicitly address this conflict. Options:
- (a) Remove `Ctrl+Right` from tab cycling (use only slash commands for tab switching)
- (b) Remove `Ctrl+Right` from sprint accept (use only `/accept` or natural language "accept")
- (c) Make tab cycling conditional: only cycle when the current view doesn't consume the key

Option (c) is the BT v1-idiomatic approach: let the current view's `Update()` run first, and only handle `Ctrl+Left/Right` at the app level if the view doesn't consume it. But this requires the View interface to signal "I consumed this key," which the current `View.Update()` signature doesn't support. The pragmatic fix is option (a) or (b).

### P2-3: WindowSizeMsg reduction math differs between App and UnifiedApp

**Section:** Phase 2
**Files:** `internal/tui/app.go:166-173`, `internal/tui/unified_app.go:333-361`

`App` passes `WindowSizeMsg` directly to the active view with no size reduction. `UnifiedApp` subtracts `headerHeight + footerHeight + logPaneHeight`. After the merge, all views will receive the reduced size. Views like `GurgehView` (line 70-72) and `SprintView` (line 160-161) already do their own subtraction:

```go
// GurgehView
v.width = msg.Width - 6
v.height = msg.Height - 4

// SprintView
v.width = msg.Width - 6
v.height = msg.Height - 4 - 2
```

If `UnifiedApp` is already subtracting header/footer height, and then views subtract more, the content area will be too small. This double-subtraction is currently masked because `App` doesn't subtract, so views do. After merge, this will break.

**Recommendation:** Phase 2 should audit all view `WindowSizeMsg` handlers and remove the manual subtraction, since `UnifiedApp` will provide the content-area dimensions. This is the kind of bug that manifests as black lines at the bottom of the terminal.

### P2-4: goroutine ownership during onboarding move

**Section:** Phase 2: Gurgeh Absorbs Onboarding
**File:** `internal/tui/unified_app.go:882-930` (scanCodebase), lines 1473-1498 (generateEpicsWithAgent)

Currently, `scanCodebase` and `generateEpicsWithAgent` spawn goroutines that write to channels, and `UnifiedApp` reads from those channels via `waitForScanProgress` / `waitForAgentStream`. These goroutines capture `a` (the `UnifiedApp` pointer) in their closures.

When these move to `GurgehView`, the goroutines must capture the `GurgehView` pointer instead. But there is a subtlety: `View.Update()` returns a `View` interface, and BT v1 allows the returned value to be a different struct (for immutable update patterns). If the goroutine captures the old pointer and `Update` returns a new struct, the goroutine writes to a stale view.

In this codebase, views mutate in place (they return `v, cmd` not `newV, cmd`), so this is safe **as long as that pattern is maintained**. But it's a footgun during refactoring.

**Recommendation:** Phase 2 should document that `GurgehView.Update()` must always return `v` (the same pointer), not a copy. Add a comment in the struct definition. Consider a compile-time assertion that the view is pointer-receiver-only.

---

## Improvements Suggested

### S1: Define a GurgehSubView interface for Phase 2

Rather than making `GurgehView` a monolithic 800-line file, define a sub-view interface:

```go
type GurgehSubView interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (GurgehSubView, tea.Cmd)
    View() string
    Name() string
}
```

Then `GurgehView` manages a stack of sub-views (kickoff, sprint, spec summary, epic review, task review) with its own internal routing. This mirrors `UnifiedApp`'s current `currentView` pattern but scoped to Gurgeh. The key difference: messages that currently bubble to `UnifiedApp` are now caught in `GurgehView.Update()` and used to swap sub-views.

**Rationale:** This keeps the refactor mechanical -- each current view factory maps to a sub-view constructor. The routing logic moves from `UnifiedApp` to `GurgehView` with minimal transformation.

### S2: Add a "consumed" signal to key handling

Currently, `Ctrl+Left/Right` is processed at the `UnifiedApp` level before being forwarded to views. This creates the `Ctrl+Right` collision (P2-2). A better pattern:

```go
// In UnifiedApp.Update, for KeyMsg:
// 1. Let the current view try first
if a.currentView != nil {
    a.currentView, cmd = a.currentView.Update(msg)
    if cmd != nil {
        return a, cmd
    }
}
// 2. Only then try app-level keybindings
switch msg.String() {
case "ctrl+left": ...
case "ctrl+right": ...
}
```

This is the "bubbling" pattern. If the view consumed the key (returned a non-nil `cmd`), the app doesn't handle it. This is imperfect (a view might return a `cmd` for other reasons), but it's a reasonable heuristic for BT v1 where there is no explicit "consumed" concept.

**Rationale:** Prevents key conflicts without needing a new interface. Works with the existing `View` interface unchanged. The only risk is views that return commands from key events they don't actually consume (e.g., a timer tick queued alongside a key press), but this is rare in practice.

### S3: Phase 3 overlay should use the existing `overlay()` method

The plan mentions "Create a new `SignalsOverlay` component" but `UnifiedApp` already has a working `overlay()` method (line 2103) used for palette, chat settings, and help. The Signals overlay should reuse this exact pattern: render the base view, then call `overlay()` with the signals content.

The current `SignalsView` uses `ShellLayout` with sidebar + document + chat (3-pane). For the overlay, strip it down to a single-pane list with a fixed height (say, 60% of terminal height) and use the existing `overlay()` positioning.

**Rationale:** Zero new rendering infrastructure needed. The overlay method handles ANSI-aware string splicing, which is the hard part.

### S4: Preserve `--skip-onboard` as `--skip-onboard` during Phase 2

The plan says "Remove `--skip-onboard` flag (no-op with deprecation warning)". Instead, keep it working but make it equivalent to `autarch tui --tool=gurgeh` with Gurgeh's internal state set to "dashboard" (past onboarding). This avoids breaking existing scripts or muscle memory while achieving the architectural goal.

**Rationale:** Deprecation warnings are fine, but removing functionality during a refactor introduces two kinds of breakage simultaneously. Ship the architectural change, then deprecate in a follow-up.

---

## Overall Assessment

**Phase 1:** Shipped and clean. The keybinding decisions are correct for BT v1. No issues found.

**Phase 2:** This is the risky phase. The plan correctly identifies the scope (800-1200 lines) but should add a design spike section addressing:
1. The message forwarding boundary (P1-2) -- this is the single most important architectural decision
2. The `Ctrl+Right` collision (P2-2) -- must be resolved before or during Phase 2
3. The `WindowSizeMsg` double-subtraction (P2-3) -- will cause visible rendering bugs if not caught
4. Goroutine ownership (P2-4) -- must be documented as a constraint

The suggested `GurgehSubView` interface (S1) would make Phase 2 more mechanical and less risky. I recommend a design spike that writes just the interface + routing skeleton before moving any handler code.

**Phase 3:** Low risk, mostly UI work. The `Ctrl+Shift+S` portability issue (P1-1) should be addressed in the plan text now, even though Phase 3 is deferred. The existing `overlay()` infrastructure (S3) makes this straightforward.

The plan is approved with the P1-severity items as conditions for Phase 2 implementation. Phase 1 is already shipped successfully.
