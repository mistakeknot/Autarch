# Architecture Review: Unified TUI Navigation Design

**Reviewer:** Architecture Reviewer
**Plan:** `docs/plans/2026-02-05-unified-tui-navigation-design.md`
**Date:** 2026-02-06
**Focus:** Simplified Mode Architecture, Phase 2 (Gurgeh absorbs onboarding), Phase 3 (Signals overlay)

---

## Summary

The plan correctly identifies a real architectural problem -- two separate TUI implementations (`App` and `UnifiedApp`) with a confusing `ModeOnboarding`/`ModeDashboard` split -- and proposes a sound 3-phase strategy to eliminate it. Phase 1 (done) was low-risk and is well-implemented. Phase 2 is the crux: moving ~400 lines of onboarding orchestration from `UnifiedApp` into the Gurgeh view. The estimate (800-1200 lines, 5-8 files) is realistic given the code I reviewed, but the plan lacks critical detail on three points: (1) how the message routing contract changes when `UnifiedApp` no longer owns onboarding messages, (2) what happens to the 6 view factories currently injected from `cmd/autarch/main.go`, and (3) how the `App`/`UnifiedApp` merge actually proceeds without breaking the `--skip-onboard` path during transition. Phase 3 is a stub with insufficient design detail for the overlay rendering model.

---

## Architecture Assessment

### Components Affected

| Component | File(s) | Impact |
|-----------|---------|--------|
| **UnifiedApp** (onboarding host) | `/root/projects/Autarch/internal/tui/unified_app.go` (~2195 lines) | Phase 2 removes ~400 lines of transition handlers, onboarding state, breadcrumb |
| **App** (skip-onboard host) | `/root/projects/Autarch/internal/tui/app.go` (~466 lines) | Phase 2 merges into UnifiedApp or vice-versa |
| **OnboardingOrchestrator** | `/root/projects/Autarch/internal/tui/onboarding.go` (~248 lines) | Phase 2 restructures or removes |
| **Breadcrumb** | `/root/projects/Autarch/internal/tui/breadcrumb.go` (~192 lines) | Phase 2 moves into GurgehView |
| **GurgehView** (dashboard tab) | `/root/projects/Autarch/internal/tui/views/gurgeh.go` (~313 lines) | Phase 2 absorbs onboarding flow -- becomes largest view |
| **SignalsView** | `/root/projects/Autarch/internal/tui/views/signals.go` (~538 lines) | Phase 3 converts to overlay |
| **Messages** | `/root/projects/Autarch/internal/tui/messages.go` (~273 lines) | Phase 2 changes routing contract for 12+ message types |
| **Types** | `/root/projects/Autarch/internal/tui/types.go` (~227 lines) | Phase 2 moves interfaces; some become Gurgeh-internal |
| **Main entry point** | `/root/projects/Autarch/cmd/autarch/main.go` (~610 lines) | Phase 2 rewires factory injection |
| **Command picker** | `/root/projects/Autarch/pkg/tui/command_picker.go` | Phase 1 (done) added navigation commands |

### Boundary Compliance

The plan respects the project's established boundaries:

- **`internal/tui/`** for app-level orchestration (UnifiedApp, App, onboarding)
- **`internal/tui/views/`** for individual tool views (GurgehView, SignalsView, etc.)
- **`pkg/tui/`** for shared components (ShellLayout, ChatPanel, CommandPicker)
- **`cmd/`** for wiring and factory injection

Phase 2 moves code from `internal/tui/unified_app.go` into `internal/tui/views/gurgeh.go`. This is architecturally sound: onboarding IS the Gurgeh workflow, so it belongs in the Gurgeh view rather than the app shell.

### Coupling Assessment

**Current coupling (problematic):** `UnifiedApp` is tightly coupled to Gurgeh's internal workflow through 12+ message types (`ProjectCreatedMsg`, `InterviewCompleteMsg`, `SpecAcceptedMsg`, `EpicsGeneratedMsg`, `TasksAcceptedMsg`, `SprintCompleteMsg`, etc.) and 6 view factories for onboarding-specific views. The breadcrumb is also Gurgeh-specific but lives at the app level.

**Post-Phase-2 coupling (improved):** `UnifiedApp` (merged with `App`) would only know about tabs, each tab being a `View`. The Gurgeh tab internally manages its own state machine. This is a clear improvement in separation of concerns.

**New coupling risk:** The Gurgeh view will need access to the `*autarch.Client`, `*research.Coordinator`, `*agent.Agent`, and `*pkgtui.AgentSelector` that `UnifiedApp` currently owns. The plan does not specify how these dependencies flow into the expanded Gurgeh view.

---

## Section-by-Section Review

### Phase 1: Tabs Always Visible + Navigation (DONE)

Phase 1 is implemented and committed (4c62720). I confirmed the code: slash commands (`/big`, `/gur`, `/cold`, `/pol`) and `Ctrl+Left/Right` cycling work in both modes. The `switchToTab()` method in `unified_app.go` lines 1876-1886 correctly handles mode transitions -- switching tabs during onboarding enters dashboard mode for the selected tab.

No issues.

### Phase 2: Gurgeh Absorbs Onboarding

This is where the architectural risk concentrates.

#### What the plan says

- Move onboarding state machine from `UnifiedApp` into `GurgehView`
- Move all onboarding message handlers out of `UnifiedApp.Update()`
- Move breadcrumb into Gurgeh's content area
- Merge `App` and `UnifiedApp` into a single implementation
- Remove `--skip-onboard` flag
- ~800-1200 lines changed, 5-8 files

#### What the plan does NOT say

1. **Message routing contract change.** Currently, `UnifiedApp.Update()` intercepts 12+ message types (lines 536-698 of `unified_app.go`) before they reach any view. After Phase 2, these messages must either:
   - Route to GurgehView specifically (requiring the shell to know which messages are Gurgeh-specific), or
   - Pass through generically to the active view (requiring GurgehView to handle them and other views to ignore them).

   The second approach follows the existing pattern -- `UnifiedApp` already passes unhandled messages to `a.currentView.Update(msg)` at line 702. But currently, many onboarding messages are intercepted BEFORE reaching the view. The plan must specify which messages become view-internal vs. which remain at the shell level.

2. **View factory injection.** `cmd/autarch/main.go` (lines 184-284) injects 6 view factories into `UnifiedApp` via `SetViewFactories()` and `SetSprintViewFactory()`:
   - `createKickoffView`
   - `createSpecSummaryView`
   - `createEpicReviewView`
   - `createTaskReviewView`
   - `createTaskDetailView`
   - `createSprintView`

   After Phase 2, these factories must be injected into the Gurgeh view instead. The plan mentions "rewire view factories in `cmd/autarch/main.go`" but does not detail the new injection API.

3. **Dependency injection for shared state.** `UnifiedApp` currently owns:
   - `codingAgent *agent.Agent` and `agentSelector *pkgtui.AgentSelector` (lines 56-58)
   - `researchCoord *research.Coordinator` (line 51)
   - `chatSettings pkgtui.ChatSettings` (line 81)
   - `selectedAgent string` (line 58)

   These are passed to views via `attachAgentSelector()` (line 248). After Phase 2, the merged shell still needs to own these and pass them to all views (not just Gurgeh). The plan does not address this.

4. **App/UnifiedApp merge strategy.** The plan says "merge App and UnifiedApp" but does not specify direction. The two implementations differ significantly:
   - `App` is stateless (just tabs + views) -- 466 lines
   - `UnifiedApp` has onboarding state, log pane auto-show/hide, agent lifecycle, chat settings, breadcrumb, overlay rendering -- 2195 lines

   The merge should preserve `UnifiedApp`'s infrastructure (log pane, palette, overlay, chat settings) while removing its onboarding code. The result should look like `App`'s simplicity with `UnifiedApp`'s infrastructure.

5. **The `--skip-onboard` transition.** During Phase 2 development, both `RunWithOpts()` and `RunUnifiedWithOpts()` must continue working. The plan should specify whether the merge happens atomically or progressively (keeping backward compatibility during the refactor).

### Phase 3: Signals Overlay

The plan says:
- Create `SignalsOverlay` component
- Add toggle state and rendering in `UnifiedApp`
- Wire `Ctrl+Shift+S` and `/signals`
- ~200-300 lines, 2-3 files

#### Missing details

1. **Overlay rendering model.** `UnifiedApp.overlay()` (line 2103) currently renders overlays by overwriting base content character-by-character using ANSI-aware operations. This works for the command palette (a small centered box) but a signals overlay would occupy a significant portion of the screen. The plan does not specify:
   - Overlay size (full width? half height? floating panel?)
   - Whether the overlay captures input exclusively (like palette) or allows interaction with the underlying view
   - How the overlay interacts with the existing log pane (which is already a bottom panel)

2. **Ctrl+Shift+S portability.** The plan specifies `Ctrl+Shift+S` as the keybinding, but the codebase memory notes that Bubble Tea v1 has keybinding limitations (see `MEMORY.md`: "Ctrl+number keybindings don't work in Bubble Tea v1"). `Ctrl+Shift+S` sends different escape sequences depending on the terminal. The `/signals` slash command is the safe fallback, but the keybinding may be unreliable.

3. **SignalsView reuse.** The current `SignalsView` (538 lines) is a full `ShellLayout`-based 3-pane view. Converting it to an overlay means either:
   - Creating a simplified `SignalsOverlay` from scratch (ignoring existing implementation)
   - Stripping `SignalsView` down to a compact list renderer

   The plan says "simplified rendering, no 3-pane layout" but does not specify what the simplified version shows or how it relates to the existing filtering/sorting logic (source filter, type filter, severity filter, Intermute live connection).

4. **State persistence.** `SignalsView` currently connects to Intermute via WebSocket (`connectIntermute()`, line 372) for live event streaming. An overlay that is toggled on/off needs to decide whether the WebSocket stays connected while hidden or reconnects each toggle.

---

## Issues Found

### P1-1: Phase 2 message routing contract is unspecified (P1)

**Location:** Phase 2 design, "Move all onboarding message handlers out of UnifiedApp.Update()"

**Problem:** 12+ message types currently intercepted at `UnifiedApp.Update()` (lines 536-698 of `/root/projects/Autarch/internal/tui/unified_app.go`) need a defined routing strategy after Phase 2. Some messages (`ProjectCreatedMsg`, `SprintCompleteMsg`) drive state transitions that affect which view is displayed. If GurgehView handles these internally, it needs a way to tell the shell "I've changed my sub-view" without the shell caring about Gurgeh's internal state.

**Suggestion:** Define an explicit routing contract before implementation:
- Messages that remain at the shell level: `OnboardingCompleteMsg` (becomes a no-op or triggers initial dashboard setup), `AgentSelectedMsg`, `AgentNotFoundMsg`
- Messages that become Gurgeh-internal: `ProjectCreatedMsg`, `InterviewCompleteMsg`, `SpecAcceptedMsg`, `EpicsGeneratedMsg`, `EpicsAcceptedMsg`, `TasksGeneratedMsg`, `TasksAcceptedMsg`, `SprintCompleteMsg`, `ScanCodebaseMsg`, `CodebaseScanResultMsg`, `NavigateToTaskDetailMsg`, `NavigateBackMsg`, `NavigateToKickoffMsg`, `NavigateToStepMsg`
- The shell routes ALL messages to the active view first; the view returns `tea.Cmd` to signal shell-level transitions (like entering dashboard). This matches the existing `a.currentView.Update(msg)` fallthrough pattern already used at line 702.

### P1-2: View factory injection API not designed (P1)

**Location:** Phase 2 design, "Rewire view factories in cmd/autarch/main.go"

**Problem:** The 6 view factories in `SetViewFactories()` and `SetSprintViewFactory()` at `/root/projects/Autarch/internal/tui/unified_app.go` lines 163-177 and 158-160 currently create onboarding-specific views (KickoffView, SpecSummaryView, EpicReviewView, TaskReviewView, TaskDetailView, SprintView). After Phase 2, these must be injected into the Gurgeh view. The plan mentions this but does not design the injection API.

**Suggestion:** The expanded GurgehView should accept a `GurgehConfig` struct containing all factories:
```go
type GurgehConfig struct {
    Client          *autarch.Client
    CreateKickoff   func() View
    CreateSprint    func(string) View
    CreateSpecSummary func(*SpecSummary, *research.Coordinator) View
    // ... etc
}
```
This mirrors how `cmd/autarch/main.go` already injects factories, but targets GurgehView instead of UnifiedApp. Wire it in `main.go`'s dashboard views factory function.

### P1-3: App/UnifiedApp merge direction and strategy not specified (P1)

**Location:** Phase 2 design, "Merge App and UnifiedApp into a single implementation"

**Problem:** Two separate code paths exist: `RunWithOpts()` (using `App`, 466 lines) in `/root/projects/Autarch/internal/tui/app.go` and `RunUnifiedWithOpts()` (using `UnifiedApp`, 2195 lines) in `/root/projects/Autarch/internal/tui/unified_app.go`. They share duplicated logic for tab switching, overlay rendering, log pane management, and keybinding handling. The plan does not specify the merge direction or whether backward compatibility is maintained during the transition.

**Suggestion:** The merge should:
1. Keep `UnifiedApp` as the survivor (it has log pane auto-show/hide, chat settings, agent lifecycle, palette commands -- features `App` lacks)
2. Remove all onboarding-specific code from `UnifiedApp` (Phase 2's main work)
3. Delete `App` entirely after the merge
4. `--skip-onboard` becomes a no-op with deprecation warning (already specified in the plan)
5. Both `Run()` and `RunUnified()` delegate to the same implementation
6. Do this atomically in one commit to avoid a broken intermediate state where `App` is removed but `UnifiedApp` still has onboarding code

### P2-1: Ctrl+Shift+S keybinding may not work in Bubble Tea v1 (P2)

**Location:** Phase 3 design, "Wire Ctrl+Shift+S keybinding"

**Problem:** The project's own memory (`MEMORY.md`) documents that Bubble Tea v1 has keybinding limitations: "Ctrl+number keybindings don't work in Bubble Tea v1: BT v1 doesn't negotiate the Kitty keyboard protocol." `Ctrl+Shift+S` similarly relies on the Kitty protocol or CSI u encoding to distinguish from `Ctrl+S` (which is `XOFF` flow control and often intercepted by the terminal). In most terminals, `Ctrl+Shift+S` either sends the same byte as `Ctrl+S` (0x13) or is intercepted by the terminal itself.

**Suggestion:** Drop `Ctrl+Shift+S` and rely solely on `/signals` (slash command) and the command palette. This is consistent with the Phase 1 decision to drop `Ctrl+N` / `Alt+N` for the same reason. If a keybinding is desired, consider an unused `Ctrl+` combination that Bubble Tea v1 can parse, though the slash command approach already works.

### P2-2: Phase 3 overlay rendering model not designed (P2)

**Location:** Phase 3 design, "Create a new SignalsOverlay component"

**Problem:** The existing overlay mechanism (`UnifiedApp.overlay()` at line 2103) uses character-level ANSI-aware overwriting. This works for small centered boxes (palette, help) but a signals overlay covering half the screen would be visually jarring with this approach. Additionally, the overlay currently blocks all interaction with underlying content (palette captures all key events).

**Suggestion:** Phase 3 needs a mini-design spike before implementation addressing: (1) overlay size and position (suggest: bottom panel replacing log pane when visible, ~10-15 lines), (2) input capture model (suggest: non-exclusive, signals are read-only so underlying view stays interactive), (3) content model (suggest: simplified list of recent signals with severity icons, no 3-pane layout). The existing `SignalsView` filtering logic can be extracted into a shared `signalFilter` function used by both the full view and the overlay.

### P2-3: OnboardingOrchestrator relationship unclear (P2)

**Location:** Phase 2, "Move onboarding state machine from UnifiedApp into GurgehView"

**Problem:** `/root/projects/Autarch/internal/tui/onboarding.go` defines both `OnboardingState` (used by `UnifiedApp` for breadcrumb/state tracking) and `OnboardingOrchestrator` (a separate struct that wraps a `View` with context/cancel support). The plan mentions `onboarding.go` as "Move into gurgeh or restructure" but does not specify what happens to `OnboardingOrchestrator`. It is unclear whether `OnboardingOrchestrator` is currently used by `UnifiedApp` (it appears to be a dead or partially-used abstraction -- `UnifiedApp` does its own orchestration directly).

**Suggestion:** During the Phase 2 design spike, audit whether `OnboardingOrchestrator` is actively used. If it is dead code, delete it. If it wraps useful patterns (context cancellation, completion callbacks), extract those patterns into the new Gurgeh internal state machine. Either way, `OnboardingState` and its constants should move into the Gurgeh package since they represent Gurgeh-specific workflow stages.

### P2-4: headerHeight calculation hardcoded to mode (P2)

**Location:** Phase 2, rendering changes

**Problem:** `UnifiedApp.View()` at line 1904 and `UnifiedApp.Update()` at line 349 both hardcode header height based on mode:
```go
headerHeight := 3
if a.mode == ModeOnboarding {
    headerHeight = 4
}
```
After Phase 2 removes `ModeOnboarding`, this conditional becomes dead code. But the breadcrumb (which added the extra line) moves into GurgehView's content area. The shell's header height becomes a constant 3 (tabs only), and the breadcrumb height is accounted for within Gurgeh's own layout math.

**Suggestion:** This is a mechanical fix during Phase 2. Flag it in the implementation checklist to avoid a 1-line layout overflow (the kind of bug documented in `MEMORY.md`: "lipgloss Height() is a floor, not a ceiling").

---

## Improvements Suggested

### 1. Add a Phase 2 design spike document (HIGH VALUE)

The plan correctly calls for a "design spike recommended before implementation" but does not define what the spike should produce. Before Phase 2 coding begins, write a 1-2 page document covering:
- Message routing table (which messages stay at shell, which become Gurgeh-internal)
- GurgehView expanded API (config struct, factory injection, sub-view management)
- Merge strategy (which struct survives, deletion order, commit strategy)
- Migration of `OnboardingState` and `Breadcrumb` into Gurgeh's package

This spike pays for itself by preventing the "start refactoring, discover entanglement, partial revert" cycle that commonly kills large refactors in Bubble Tea apps.

### 2. Consider an intermediate sub-view pattern for Gurgeh (MEDIUM VALUE)

The current `GurgehView` (313 lines, simple list-and-detail) will balloon to 1000+ lines when it absorbs the onboarding flow. Instead of a monolithic `GurgehView.Update()`, consider a sub-view pattern already used by the project:
- `GurgehView` manages a `currentSubView View` (kickoff, sprint, spec summary, epic review, task review)
- Sub-views are the same types that `UnifiedApp` currently creates via factories
- `GurgehView.Update()` delegates to the current sub-view, only handling transitions between sub-views

This mirrors how `UnifiedApp` already works (it has `a.currentView` that changes during onboarding) but scoped to the Gurgeh tab. The code movement is then literally: extract the state machine from `UnifiedApp` into `GurgehView`, keep the sub-views unchanged.

### 3. Simplify Phase 3 to "bottom panel toggle" instead of "overlay" (MEDIUM VALUE)

The overlay approach introduces rendering complexity. The project already has a bottom-panel toggle: the log pane (`Ctrl+L`). A signals panel could work identically:
- `Ctrl+L` toggles log pane (existing)
- `/signals` toggles signals panel (replaces log pane when active, or appears alongside it)
- Both are bottom panels with fixed height

This reuses the existing log pane infrastructure (sizing, toggle, auto-show/hide) and avoids the character-level overlay rendering that is fragile with ANSI escape sequences.

---

## Overall Assessment

**Architecture fit:** Good. The plan simplifies a real mess (two app structs, onboarding entangled in the shell) into a clean tabs-only shell. The phased approach is appropriate.

**Top changes needed before Phase 2 implementation:**

1. **Write the design spike document** with explicit message routing table, factory injection API, and merge strategy. This is the difference between an 800-line clean refactor and a 1200-line refactor with rework.

2. **Adopt the sub-view delegation pattern** for the expanded GurgehView. This keeps each sub-view (kickoff, sprint, spec, etc.) as an independent `View` implementation -- the same types that exist today -- and makes GurgehView a thin orchestrator rather than a monolith.

3. **Drop Ctrl+Shift+S** for Phase 3 and either simplify the overlay to a bottom panel or defer the design until the rendering model is specified. The `/signals` slash command already provides the access mechanism.
