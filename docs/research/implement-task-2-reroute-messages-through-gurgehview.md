# Task 2: Make GurgehView a Container and Reroute Messages (Reroute)

## Summary

Task 2 of Phase 2c converted `GurgehView` from a simple spec browser into a container
that delegates to `GurgehOnboardingView` until onboarding completes, then switches to
the spec browser. Approximately 900 lines were removed from `UnifiedApp`, including
20+ message handlers, 15+ helper methods, all onboarding-related struct fields, and
several internal types.

## Changes Made

### Step 1: GurgehView Container (`internal/tui/views/gurgeh.go`)

- Added `onboarding *GurgehOnboardingView` and `showBrowser bool` fields to struct
- Changed signature: `NewGurgehView(client, cfg)` — if `cfg` is non-nil, onboarding
  is created; otherwise `showBrowser=true` skips straight to spec browser
- `Init()` delegates to `onboarding.Init()` when in onboarding mode
- `Update()` adds onboarding delegation block at top:
  - `tea.WindowSizeMsg` passes to both browser sizing and onboarding
  - `OnboardingCompleteMsg` sets `showBrowser=true` and re-emits for UnifiedApp
  - All other messages pass through to `onboarding.Update(msg)`
- `View()`, `Focus()`, `Blur()`, `ShortHelp()` all delegate to onboarding when active
- Added pass-through methods: `SetAgentSelector()`, `SetAgentName()`, `SetChatSettings()`

### Step 2: Log Pane Bridge Messages (`internal/tui/messages.go`)

- Added `LogPaneAutoShowMsg` and `LogPaneScheduleAutoHideMsg` message types
- These allow `GurgehOnboardingView` to signal `UnifiedApp` to toggle the log pane
  without having direct access to log pane state

### Step 3: Remove Onboarding Handlers from UnifiedApp.Update()

Removed explicit `case` handlers for all Gurgeh-internal messages:
- `ProjectCreatedMsg`, `InterviewCompleteMsg`, `SuggestionsReadyMsg`
- `SpecAcceptedMsg`, `EpicsGeneratedMsg`, `EpicsAcceptedMsg`
- `TasksGeneratedMsg`, `TasksAcceptedMsg`, `GeneratingMsg`, `GenerationErrorMsg`
- `AgentNotFoundMsg`, `NavigateToTaskDetailMsg`, `NavigateBackMsg`
- `NavigateToKickoffMsg`, `ScanCodebaseMsg`, `CodebaseScanResultMsg`
- `SprintCompleteMsg`, `ScanProgressMsg`, `AgentRunStartedMsg`
- `scanProgressWithContinuation`, `agentStreamWithContinuation`, `AgentStreamMsg`
- `AgentRunFinishedMsg`, `AgentEditSummaryMsg`, `RevertLastRunMsg`

These now fall through to `currentView.Update(msg)` (the default path at the bottom
of Update), where GurgehView's delegation routes them to GurgehOnboardingView.

Added new handlers:
- `LogPaneAutoShowMsg` — auto-shows log pane during scan
- `LogPaneScheduleAutoHideMsg` — schedules 3s auto-hide after scan completes
- `OnboardingCompleteMsg` — changed to no-op (GurgehView handles internally)

### Step 4: Remove Onboarding State Fields from UnifiedApp

Removed from `UnifiedApp` struct:
- `projectID`, `projectName`, `projectDesc`, `interviewAnswers`
- `generatedEpics`, `generatedTasks`
- `generating`, `generatingWhat`
- `lastRunLabel`, `lastRunSnapshot`

Kept: `onboardingState`, `breadcrumb` (still used for header rendering), `researchCoord`,
`codingAgent`, `agentSelector`, `selectedAgent`, `ctx`, `cancel`

### Step 5: Delete Handler Methods from UnifiedApp

Deleted methods (~700 lines):
- `handleProjectCreated`, `handleInterviewComplete`, `handleSuggestionsReady`
- `handleSpecAccepted`, `handleEpicsGenerated`, `handleEpicsAccepted`
- `handleTasksGenerated`, `handleTasksAccepted`
- `showTaskDetail`, `navigateBack`, `navigateToKickoff`, `navigateToStep`
- `generateSuggestions`, `generateEpicsWithAgent`, `generateTasksWithAgent`
- `scanCodebase`, `waitForScanProgress`, `waitForAgentStream`
- `captureRunSnapshot`, `finalizeAgentRun`, `summarizeDiff`, `sendToCurrentView`

Deleted types:
- `scanProgressWithContinuation`, `agentStreamEvent`, `agentStreamWithContinuation`

Deleted helper functions (~200 lines):
- `extractString`, `extractVisionSummary`, `extractProblemSummary`, `extractUsersSummary`
- `safeIndex`, `toValidationErrors`, `toPhaseArtifacts`, `toVisionArtifact`
- `toProblemArtifact`, `toUsersArtifact`, `toEvidenceItems`, `toPersonas`
- `toQualityScores`, `scanResultToArtifacts`, `toScanEvidence`, `toScanResolved`
- `createSpecSummaryFromSprintState`, `extractFirstLine`, `parseBulletItems`

### Step 6: Log Pane Bridge in GurgehOnboardingView

Updated `gurgeh_onboarding.go`:
- `ScanCodebaseMsg` handler now emits `LogPaneAutoShowMsg` alongside the scan command
  via `tea.Batch()`, so the log pane auto-shows when a scan starts
- `CodebaseScanResultMsg` handler now emits `LogPaneScheduleAutoHideMsg` alongside
  passing the result to the current view, so the log pane auto-hides after scan completes

### Step 7: Init() Flow

`UnifiedApp.Init()` now always calls `enterDashboard()`. The onboarding flow starts
inside `GurgehView.Init()` when a `GurgehConfig` is provided. This removes the
`skipOnboarding` branching from `Init()` — both paths converge at dashboard entry.

### Step 8: Tests

Updated `unified_app_test.go`:
- Removed tests for deleted handlers (7 tests)
- Added tests: `TestAgentStreamMessagesPassThroughToView`, `TestInitAlwaysEntersDashboard`,
  `TestSkipOnboardingWithInitAlwaysEntersDashboard`, `TestOnboardingCompleteMsgIsNoOp`,
  `TestLogPaneAutoShowMsg`, `TestLogPaneScheduleAutoHideMsg`,
  `TestLogPaneScheduleAutoHideMsgNoOpWhenNotAutoShown`, `TestTabSwitchingWorksInDashboardMode`

Created `gurgeh_test.go`:
- `TestGurgehViewNilConfigSkipsOnboarding`
- `TestGurgehViewWithConfigStartsOnboarding`
- `TestGurgehViewInitDelegatesToOnboarding`
- `TestGurgehViewViewDelegatesToOnboarding`
- `TestGurgehViewOnboardingCompleteSwitchesToBrowser`
- `TestGurgehViewPassesThroughMessagesToOnboarding`
- `TestGurgehViewFocusDelegatesToOnboarding`
- `TestGurgehViewShortHelpDelegatesToOnboarding`

### Step 9: Import Cleanup

Removed from `unified_app.go`: `path/filepath`, `arbiter`, `scan`, `exploration`
Kept `epics`/`tasks` for `SetViewFactories` backward-compatible signature.

### Callers Updated

- `cmd/autarch/main.go`: `NewGurgehView(c)` -> `NewGurgehView(c, nil)`, removed
  `app.SetSprintViewFactory()` call
- `cmd/testui/main.go`: `NewGurgehView(c)` -> `NewGurgehView(c, nil)`

## Metrics

- `unified_app.go`: ~2200 lines -> ~1017 lines (~1183 lines removed)
- All packages build cleanly
- All tests pass (including new container delegation tests)

## Remaining for Task 3

- Wire `GurgehConfig` in `cmd/autarch/main.go` with actual factory functions
  (currently `NewGurgehView(c, nil)` — no onboarding in production yet)
- The `SetViewFactories` backward-compat signature ignores onboarding params;
  Task 3 replaces it with `SetDashboardViewsFactory`
