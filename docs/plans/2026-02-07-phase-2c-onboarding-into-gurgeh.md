# Phase 2c: Move Onboarding Into Gurgeh — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Move the entire onboarding state machine out of `UnifiedApp` into a new `GurgehOnboardingView`, making `GurgehView` a container that switches between onboarding and spec browsing.

**Architecture:** `UnifiedApp` delegates all onboarding messages to `GurgehView`, which internally routes them to `GurgehOnboardingView`. The `GurgehOnboardingView` absorbs all transition handlers, view factories, agent generation goroutines, and conversion helpers. Types (`OnboardingState`, `Breadcrumb`) stay in `internal/tui/` to avoid import cycles. Only one new message escapes to the shell: when onboarding completes, `GurgehView` emits `OnboardingCompleteMsg`.

**Tech Stack:** Go, Bubble Tea v1, lipgloss

---

## Design Spike Decisions

### 1. Message Routing Table

**Shell-level** (stay in `UnifiedApp.Update()`):
- `tea.WindowSizeMsg` — resize propagation to all views
- `tea.KeyMsg` — global keybindings (Ctrl+C, Ctrl+P, Ctrl+L, Ctrl+Left/Right, ?, etc.)
- `pkgtui.SlashCommandMsg` — global slash commands (/help, /quit, /big, /gur, etc.)
- `pkgtui.AgentSelectedMsg` — agent selector events (shared across all tabs)
- `pkgtui.LogBatchMsg` — log pane updates
- `OnboardingCompleteMsg` — transition to dashboard mode
- `logPaneAutoHideMsg` — log pane auto-hide timer

**Gurgeh-internal** (move into `GurgehOnboardingView.Update()`):
- `ProjectCreatedMsg` — start interview/sprint
- `InterviewCompleteMsg` — create spec summary
- `SuggestionsReadyMsg` — pass AI suggestions to interview
- `SpecAcceptedMsg` — generate epics
- `EpicsGeneratedMsg` — show epic review
- `EpicsAcceptedMsg` — generate tasks
- `TasksGeneratedMsg` — show task review
- `TasksAcceptedMsg` — complete onboarding
- `GeneratingMsg` — loading indicator
- `GenerationErrorMsg` — generation failure
- `AgentNotFoundMsg` — no coding agent
- `NavigateToTaskDetailMsg` — task detail navigation
- `NavigateBackMsg` — back navigation within onboarding
- `NavigateToKickoffMsg` — return to kickoff
- `NavigateToStepMsg` — breadcrumb step navigation
- `SprintCompleteMsg` — sprint finished → spec summary
- `SprintExitRequestedMsg` — exit sprint (same as NavigateBackMsg)
- `CodebaseScanResultMsg` — scan results update onboarding state
- `ScanCodebaseMsg` — trigger codebase scan
- `scanProgressWithContinuation` — progress + continuation scheduling
- `agentStreamWithContinuation` — agent stream + continuation
- `AgentStreamMsg` — individual stream line
- `AgentRunStartedMsg` — agent run lifecycle
- `AgentRunFinishedMsg` — agent run lifecycle
- `AgentEditSummaryMsg` — post-run summary
- `RevertLastRunMsg` — revert snapshot

**Pass-through to active view** (forwarded by both shell and onboarding):
- `SprintStreamLineMsg`, `SprintStreamDoneMsg`, `SprintDraftUpdatedMsg`, `SprintPhaseAdvancedMsg`, `SprintConflictMsg`, `SprintPhaseRevertedMsg` — these are forwarded by the default path at the bottom of Update()

### 2. GurgehConfig Struct

```go
// GurgehConfig configures the GurgehView with all onboarding dependencies.
type GurgehConfig struct {
    Client        *autarch.Client
    ResearchCoord *research.Coordinator
    CodingAgent   *agent.Agent      // nil if no agent detected
    AgentSelector *pkgtui.AgentSelector

    // View factories for onboarding steps
    CreateKickoffView     func() View
    CreateArbiterView     func(*research.Coordinator) View
    CreateSpecSummaryView func(*SpecSummary, *research.Coordinator) View
    CreateEpicReviewView  func([]epics.EpicProposal) View
    CreateTaskReviewView  func([]tasks.TaskProposal) View
    CreateTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) View
    CreateSprintView      func(string) View
}
```

Replaces the 6-positional-argument `SetViewFactories()` call plus the separate `SetArbiterViewFactory()` and `SetSprintViewFactory()` calls.

### 3. Import Cycle Strategy

- `OnboardingState`, `Breadcrumb`, `AllOnboardingStates()` — **stay in `internal/tui/`** (in existing `onboarding.go` and `breadcrumb.go`)
- All message types — **stay in `internal/tui/messages.go`** (views/ imports tui/)
- `GurgehOnboardingView` goes in `internal/tui/views/gurgeh_onboarding.go` — it imports `tui.OnboardingState`, `tui.Breadcrumb`, etc.
- `GurgehConfig` goes in `internal/tui/gurgeh_config.go` — both `internal/tui/` and `views/` can use it

### 4. Agent Dependency Injection

Direct ownership, not interface. `GurgehOnboardingView` holds `codingAgent *agent.Agent`, `agentSelector *pkgtui.AgentSelector`, `selectedAgent string`. The shell notifies Gurgeh of agent changes via `pkgtui.AgentSelectedMsg` pass-through.

`UnifiedApp` still owns the agent selector for global F2/Ctrl+G, but passes it to `GurgehView` via `GurgehConfig`. When the user switches agents, `UnifiedApp` sends the message down to the active view, and `GurgehView` forwards it to `GurgehOnboardingView`.

### 5. Helper Placement

All scan/conversion helpers move to a new file `internal/tui/views/gurgeh_helpers.go`:
- `extractString`, `extractVisionSummary`, `extractProblemSummary`, `extractUsersSummary` (~40 lines)
- `toPhaseArtifacts`, `toVisionArtifact`, `toProblemArtifact`, `toUsersArtifact`, `toEvidenceItems`, `toPersonas`, `toQualityScores` (~100 lines)
- `scanResultToArtifacts`, `toScanEvidence`, `toScanResolved` (~65 lines)
- `safeIndex`, `createSpecSummaryFromSprintState`, `extractFirstLine`, `parseBulletItems`, `summarizeDiff` (~50 lines)
- `waitForScanProgress`, `waitForAgentStream`, `generateSuggestions`, `generateEpicsWithAgent`, `generateTasksWithAgent`, `scanCodebase` (~160 lines)
- `captureRunSnapshot`, `finalizeAgentRun` (~35 lines)
- Internal types: `scanProgressWithContinuation`, `agentStreamEvent`, `agentStreamWithContinuation` (~15 lines)
- `toValidationErrors` (~15 lines)

Total: ~480 lines moving out of `unified_app.go`.

### 6. Commit Boundaries

4 commits, each independently compilable and testable:

1. **Extract** — Create `GurgehConfig`, `gurgeh_onboarding.go`, `gurgeh_helpers.go` with all moved code. `UnifiedApp` still routes messages (compiles, tests pass, behavior identical).
2. **Reroute** — `UnifiedApp.Update()` forwards onboarding messages to `GurgehView`. Remove handler methods from `UnifiedApp`. Remove onboarding state fields.
3. **Wire** — Update `cmd/autarch/main.go` to use `GurgehConfig` instead of `SetViewFactories()`. Delete `SetViewFactories`, `SetArbiterViewFactory`, `SetSprintViewFactory` from `UnifiedApp`.
4. **Cleanup** — Remove `ModeOnboarding`/`ModeDashboard` distinction from `UnifiedApp` (it's always in dashboard mode now). Remove breadcrumb rendering from `UnifiedApp.View()`. Remove onboarding-specific help text. Fix header height (always 3, no mode check). Update tests.

---

## Tasks

### Task 1: Create GurgehOnboardingView and helpers (Extract)

**Files:**
- Create: `internal/tui/views/gurgeh_onboarding.go`
- Create: `internal/tui/views/gurgeh_helpers.go`
- Create: `internal/tui/gurgeh_config.go`
- Test: `internal/tui/views/gurgeh_onboarding_test.go`

**Step 1: Create `internal/tui/gurgeh_config.go`**

```go
package tui

import (
    "github.com/mistakeknot/autarch/internal/autarch/agent"
    "github.com/mistakeknot/autarch/internal/coldwine/epics"
    "github.com/mistakeknot/autarch/internal/coldwine/tasks"
    "github.com/mistakeknot/autarch/internal/pollard/research"
    pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// GurgehConfig configures the GurgehView with onboarding dependencies.
type GurgehConfig struct {
    ResearchCoord *research.Coordinator
    CodingAgent   *agent.Agent
    AgentSelector *pkgtui.AgentSelector
    SelectedAgent string

    // View factories for onboarding steps
    CreateKickoffView     func() View
    CreateArbiterView     func(*research.Coordinator) View
    CreateSpecSummaryView func(*SpecSummary, *research.Coordinator) View
    CreateEpicReviewView  func([]epics.EpicProposal) View
    CreateTaskReviewView  func([]tasks.TaskProposal) View
    CreateTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) View
    CreateSprintView      func(string) View
}
```

**Step 2: Create `internal/tui/views/gurgeh_helpers.go`**

Move all these functions from `unified_app.go` (lines ~946-1396) into this file with `package views`:
- `extractString`, `extractVisionSummary`, `extractProblemSummary`, `extractUsersSummary`
- All `to*` conversion functions
- `scanResultToArtifacts`, `toScanEvidence`, `toScanResolved`
- `safeIndex`, `createSpecSummaryFromSprintState`, `extractFirstLine`, `parseBulletItems`, `summarizeDiff`
- `toValidationErrors`
- Types: `scanProgressWithContinuation`, `agentStreamEvent`, `agentStreamWithContinuation`

These are pure functions — no method receivers on `UnifiedApp`. They need import adjustments (e.g., `tui.CodebaseScanResultMsg` → just `tui.CodebaseScanResultMsg` since views/ imports tui/).

Note: Several of these reference types from `internal/tui/` (like `CodebaseScanResultMsg`, `SpecSummary`, `PhaseArtifacts`). Since `views/` already imports `tui/`, this works.

**Step 3: Create `internal/tui/views/gurgeh_onboarding.go`**

This is the core extraction. Create the `GurgehOnboardingView` struct with:

```go
type GurgehOnboardingView struct {
    // Dependencies (from GurgehConfig)
    researchCoord *research.Coordinator
    codingAgent   *agent.Agent
    agentSelector *pkgtui.AgentSelector
    selectedAgent string

    // Onboarding state
    state         tui.OnboardingState
    breadcrumb    *tui.Breadcrumb
    currentView   tui.View
    projectID     string
    projectName   string
    projectDesc   string
    interviewAnswers map[string]string
    generatedEpics   []epics.EpicProposal
    generatedTasks   []tasks.TaskProposal

    // Loading state
    generating     bool
    generatingWhat string

    // Agent run state
    lastRunLabel    string
    lastRunSnapshot string

    // Context
    ctx    context.Context
    cancel context.CancelFunc

    // Layout
    width  int
    height int

    // View factories
    createKickoffView     func() tui.View
    createArbiterView     func(*research.Coordinator) tui.View
    createSpecSummaryView func(*tui.SpecSummary, *research.Coordinator) tui.View
    createEpicReviewView  func([]epics.EpicProposal) tui.View
    createTaskReviewView  func([]tasks.TaskProposal) tui.View
    createTaskDetailView  func(tasks.TaskProposal, *research.Coordinator) tui.View
    createSprintView      func(string) tui.View

    // Chat settings (received from parent)
    chatSettings pkgtui.ChatSettings
}
```

Move all handler methods from `UnifiedApp` into methods on `GurgehOnboardingView`:
- `handleProjectCreated` (~140 lines)
- `handleInterviewComplete` (~45 lines)
- `handleSuggestionsReady` (~12 lines)
- `handleSpecAccepted` (~8 lines)
- `handleEpicsGenerated` (~17 lines)
- `handleEpicsAccepted` (~10 lines)
- `handleTasksGenerated` (~17 lines)
- `handleTasksAccepted` (~12 lines)
- `showTaskDetail` (~12 lines)
- `navigateBack` (~25 lines)
- `navigateToKickoff` (~20 lines)
- `navigateToStep` (~45 lines)
- `generateSuggestions` (~20 lines)
- `generateEpicsWithAgent` (~50 lines)
- `generateTasksWithAgent` (~40 lines)
- `scanCodebase` (~55 lines)
- `waitForScanProgress` (~45 lines)
- `waitForAgentStream` (~30 lines)
- `captureRunSnapshot` (~10 lines)
- `finalizeAgentRun` (~25 lines)
- `sendToCurrentView` (BUG(phase2c) — keep as-is for now)

Implement `Update()` with the Gurgeh-internal message routing from the design spike.

Implement `View()` — delegates to `currentView.View()`, adds breadcrumb above content.

Implement `Init()` — starts with kickoff view, same as current `UnifiedApp.Init()` onboarding path.

**Step 4: Write initial test**

```go
func TestGurgehOnboardingViewInit(t *testing.T) {
    // Verify GurgehOnboardingView starts in Kickoff state
    // and creates the kickoff view via factory
}
```

**Step 5: Verify compilation**

Run: `go build ./...`
Expected: PASS (both old and new code exist; no behavior change yet)

**Step 6: Commit**

```bash
git add internal/tui/gurgeh_config.go internal/tui/views/gurgeh_onboarding.go internal/tui/views/gurgeh_helpers.go internal/tui/views/gurgeh_onboarding_test.go
git commit -m "feat(tui): extract GurgehOnboardingView and helpers from UnifiedApp

Create GurgehOnboardingView in views/ with all onboarding transition
handlers, scan/conversion helpers, and agent generation goroutines.
No routing changes yet — UnifiedApp still handles messages."
```

---

### Task 2: Make GurgehView a container and reroute messages (Reroute)

**Files:**
- Modify: `internal/tui/views/gurgeh.go`
- Modify: `internal/tui/unified_app.go`
- Test: `internal/tui/views/gurgeh_test.go` (new)
- Modify: `internal/tui/unified_app_test.go`

**Step 1: Modify `GurgehView` to be a container**

Add fields to `GurgehView`:
```go
type GurgehView struct {
    client   *autarch.Client
    // ... existing spec browser fields ...

    // Onboarding sub-view (nil after onboarding completes)
    onboarding *GurgehOnboardingView
    // If true, show spec browser; else show onboarding
    showBrowser bool
}
```

Modify `NewGurgehView` to accept `*tui.GurgehConfig`:
```go
func NewGurgehView(client *autarch.Client, cfg *tui.GurgehConfig) *GurgehView {
    v := &GurgehView{
        client:  client,
        shell:   pkgtui.NewShellLayout(),
    }
    if cfg != nil {
        v.onboarding = NewGurgehOnboardingView(cfg)
    }
    return v
}
```

Modify `GurgehView.Update()` to route messages:
- If `showBrowser` is false and `onboarding != nil`, delegate to `onboarding.Update()`
- When onboarding emits `OnboardingCompleteMsg`, set `showBrowser = true` and return the message (so `UnifiedApp` transitions to dashboard)
- Pass `tea.WindowSizeMsg`, `pkgtui.AgentSelectedMsg` to both browser and onboarding

Modify `GurgehView.View()`:
- If `!showBrowser && onboarding != nil`, render onboarding view
- Otherwise render spec browser (existing View code)

**Step 2: Modify `UnifiedApp.Update()` to delegate onboarding messages**

Remove all Gurgeh-internal message handlers from `UnifiedApp.Update()` (lines ~549-711 of the current file). The messages now flow through the default path at the bottom of `Update()`:

```go
// Pass to current view (this forwards to GurgehView, which forwards to GurgehOnboardingView)
if a.currentView != nil {
    var cmd tea.Cmd
    a.currentView, cmd = a.currentView.Update(msg)
    return a, cmd
}
```

Keep only these in `Update()`:
- `LogBatchMsg` routing
- `tea.WindowSizeMsg` handling
- `pkgtui.SlashCommandMsg` handling
- `tea.KeyMsg` global handling
- `pkgtui.AgentSelectedMsg` — still update shell-level agent state, then forward to view
- `OnboardingCompleteMsg` — call `enterDashboard()`
- `logPaneAutoHideMsg` — log pane timer
- Default pass-through to `currentView`

**Step 3: Remove onboarding state fields from `UnifiedApp`**

Delete these fields from the `UnifiedApp` struct:
- `onboardingState`, `projectID`, `projectName`, `projectDesc`
- `interviewAnswers`, `generatedEpics`, `generatedTasks`
- `generating`, `generatingWhat`
- `lastRunLabel`, `lastRunSnapshot`
- `codingAgent` (keep `agentSelector` and `selectedAgent` for global selector)
- All `createXxxView` factory fields (except `createDashboardViews`)

Delete these methods from `UnifiedApp`:
- All `handle*` methods
- `generateSuggestions`, `generateEpicsWithAgent`, `generateTasksWithAgent`
- `scanCodebase`, `waitForScanProgress`, `waitForAgentStream`
- `captureRunSnapshot`, `finalizeAgentRun`, `sendToCurrentView`
- `showTaskDetail`, `navigateBack`, `navigateToKickoff`, `navigateToStep`
- `blurCurrentView` (if only used by onboarding navigation)

Delete all helper functions that were moved to `gurgeh_helpers.go`.

**Step 4: Handle the `ScanCodebaseMsg` → log pane interaction**

`ScanCodebaseMsg` currently triggers `scanCodebase()` in `UnifiedApp`, which auto-shows the log pane. After the move, `GurgehOnboardingView` handles the scan, but auto-show/auto-hide of the log pane is a shell concern.

Solution: `GurgehOnboardingView.Update()` for `scanProgressWithContinuation` emits a new `tui.LogPaneAutoShowMsg` message. `UnifiedApp` handles this to show the log pane. Similarly, `CodebaseScanResultMsg` handling in `GurgehOnboardingView` emits `tui.LogPaneAutoHideMsg` (with timer). Add these two tiny message types to `messages.go`.

**Step 5: Run tests and fix breakage**

Run: `go test ./internal/tui/... ./cmd/...`
Fix any compilation errors from removed fields/methods.

Update `unified_app_test.go`:
- Tests that send onboarding messages (e.g., `TestScanResultUpdatesOnboardingState`) need to verify the message reaches GurgehView
- Tests that check `UnifiedApp` fields (like `onboardingState`) need to check GurgehView's internal state instead, or be moved to `gurgeh_test.go`

**Step 6: Commit**

```bash
git add internal/tui/views/gurgeh.go internal/tui/unified_app.go internal/tui/messages.go internal/tui/unified_app_test.go internal/tui/views/gurgeh_test.go
git commit -m "refactor(tui): reroute onboarding messages through GurgehView

GurgehView now delegates onboarding messages to GurgehOnboardingView.
UnifiedApp.Update() no longer handles onboarding transitions — messages
flow through the default view pass-through path. Remove ~500 lines of
handlers and state fields from UnifiedApp."
```

---

### Task 3: Rewire main.go factory injection (Wire)

**Files:**
- Modify: `cmd/autarch/main.go`
- Modify: `internal/tui/unified_app.go`
- Modify: `cmd/testui/main.go`

**Step 1: Create `GurgehConfig` in main.go and pass to dashboard factory**

Replace the `SetViewFactories()` + `SetArbiterViewFactory()` + `SetSprintViewFactory()` calls with a single `GurgehConfig` constructed in `main.go`:

```go
gurgehCfg := &tui.GurgehConfig{
    ResearchCoord: research.NewCoordinator(nil),
    // CodingAgent and AgentSelector set by UnifiedApp after detection
    CreateKickoffView: func() tui.View { /* ... existing factory ... */ },
    // ... rest of factories ...
    CreateSprintView: func(projectPath string) tui.View { /* ... existing factory ... */ },
}

app.SetGurgehConfig(gurgehCfg)
```

Modify the dashboard views factory to use the config:
```go
func(c *autarch.Client) []tui.View {
    return []tui.View{
        views.NewBigendView(c),
        views.NewGurgehView(c, gurgehCfg), // Pass config here
        views.NewColdwineView(c),
        views.NewPollardView(c),
    }
}
```

**Step 2: Delete old factory setters from UnifiedApp**

Remove `SetViewFactories()`, `SetArbiterViewFactory()`, `SetSprintViewFactory()`.

Add `SetGurgehConfig()` which stores the config and passes agent info after `Init()` detects agents:
```go
func (a *UnifiedApp) SetGurgehConfig(cfg *tui.GurgehConfig) {
    a.gurgehConfig = cfg
}
```

In `Init()`, after agent detection, update the config:
```go
if a.gurgehConfig != nil {
    a.gurgehConfig.CodingAgent = a.codingAgent
    a.gurgehConfig.AgentSelector = a.agentSelector
    a.gurgehConfig.SelectedAgent = a.selectedAgent
}
```

**Step 3: Update `cmd/testui/main.go`**

Adjust to use the new factory wiring pattern.

**Step 4: Verify end-to-end**

Run: `go build ./cmd/...`
Run: `go test ./...`

**Step 5: Commit**

```bash
git add cmd/autarch/main.go cmd/testui/main.go internal/tui/unified_app.go
git commit -m "refactor(tui): replace SetViewFactories with GurgehConfig injection

Single GurgehConfig struct replaces 3 separate factory setter calls.
Config flows through dashboard factory to GurgehView constructor."
```

---

### Task 4: Remove mode distinction and clean up UnifiedApp (Cleanup)

**Files:**
- Modify: `internal/tui/unified_app.go`
- Modify: `internal/tui/unified_app_test.go`
- Delete: (nothing — keep `onboarding.go` and `breadcrumb.go` in tui/ for type definitions)

**Step 1: Remove `ModeOnboarding`/`ModeDashboard` from UnifiedApp**

After Phase 2c, `UnifiedApp` is always in "dashboard mode" — even during onboarding (since GurgehView handles it internally). However, `UnifiedApp.Init()` still needs to know whether to start with the kickoff view or skip to dashboard.

The `skipOnboarding` flag handles this already. Remove the `mode` field, `AppMode` type, and `ModeOnboarding`/`ModeDashboard` constants from `UnifiedApp`.

Note: `GurgehOnboardingView` has its own internal mode tracking. `UnifiedApp` just needs to know "has dashboard views been created yet" which is implicit from `len(a.dashViews) > 0`.

Wait — `mode` is checked in:
- `View()` — for header height (3 vs 4) and breadcrumb rendering
- `renderFooterContent()` — for onboarding-specific help text
- `switchToTab()` — to exit onboarding and enter dashboard
- `Update()` key handling — `Ctrl+B` breadcrumb navigation in onboarding mode

After Phase 2c, breadcrumb and onboarding help text are rendered by `GurgehOnboardingView.View()`, not by `UnifiedApp.View()`. The header is always 3 lines (tabs only). So `mode` can be removed.

For `switchToTab()` during onboarding: when the user is on the Gurgeh tab in onboarding mode and clicks another tab, `switchDashboardTab()` handles it since all views are already dashboard views.

**Step 2: Simplify `UnifiedApp.View()`**

- Remove `headerHeight` mode check — always 3
- Remove breadcrumb rendering from header
- Remove `ModeOnboarding` check in footer

**Step 3: Simplify `UnifiedApp.Init()`**

- When `!skipOnboarding`, call `enterDashboard()` (which creates all dashboard views including Gurgeh with onboarding)
- When `skipOnboarding`, call `enterDashboard()` with a flag that tells GurgehView to skip onboarding

This means `Init()` always calls `enterDashboard()`. The `createKickoffView` / `createArbiterView` calls that were in `Init()` are now inside `GurgehOnboardingView.Init()`.

**Step 4: Remove dead fields**

Remove from `UnifiedApp` struct:
- `mode AppMode`
- `breadcrumb *Breadcrumb` (owned by GurgehOnboardingView now)
- `researchCoord` (owned by GurgehConfig)
- `ctx`, `cancel` (owned by GurgehOnboardingView)

**Step 5: Update tests**

- Remove tests that check `a.mode == ModeOnboarding`
- Verify tab switching works without mode check
- Verify `skipOnboarding` still works

**Step 6: Run full test suite**

Run: `go test ./...`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/tui/unified_app.go internal/tui/unified_app_test.go
git commit -m "refactor(tui): remove AppMode from UnifiedApp

UnifiedApp is now always in dashboard mode. Onboarding state is fully
owned by GurgehOnboardingView. Remove mode field, breadcrumb rendering,
onboarding-specific header/footer logic from shell."
```

---

## Flux Drive Warnings Checklist

- [ ] `codingAgent` and `agentSelector` explicitly transferred to `GurgehOnboardingView` via `GurgehConfig` (fd-gurgeh-specs P0)
- [ ] Default pass-through in `GurgehView.Update()` forwards unhandled messages to active sub-view, so `SprintConflictMsg` reaches SprintView (fd-bubble-tea)
- [ ] `SprintStateProvider` type assertion: `GurgehView` does NOT implement it — `SprintCompleteMsg` is handled entirely inside `GurgehOnboardingView` (fd-gurgeh-specs)
- [ ] `projectPath` captured at kickoff time via `os.Getwd()`, stored as field in `GurgehOnboardingView` (fd-gurgeh-specs)
- [ ] `WindowSizeMsg` double-subtraction: `UnifiedApp` subtracts header(3)+footer(3)+logpane, passes to views. `GurgehOnboardingView` adds breadcrumb (1 line) within its content area, so it needs to subtract that from the height it gives to its child view. Verify empirically. (fd-bubble-tea)
- [ ] Goroutines (`scanCodebase`, `generateEpicsWithAgent`) capture `GurgehOnboardingView` pointer, not `UnifiedApp`. Ensure `Update()` returns same pointer (not copy). (fd-bubble-tea)
- [ ] Log pane auto-show/hide: new `LogPaneAutoShowMsg`/`LogPaneAutoHideMsg` messages bridge between onboarding scan and shell log pane. (fd-architecture)

## Expected Line Count Changes

| File | Before | After | Delta |
|------|--------|-------|-------|
| `unified_app.go` | ~2200 | ~800 | -1400 |
| `views/gurgeh.go` | 313 | ~400 | +87 |
| `views/gurgeh_onboarding.go` | 0 | ~700 | +700 |
| `views/gurgeh_helpers.go` | 0 | ~480 | +480 |
| `gurgeh_config.go` | 0 | ~30 | +30 |
| **Net** | | | **-103** |

The net reduction is modest because this is primarily a relocation. The real win is in separation of concerns: `UnifiedApp` drops from ~2200 to ~800 lines and becomes a pure shell/chrome manager.
