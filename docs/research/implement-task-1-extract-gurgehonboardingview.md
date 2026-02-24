# Task 1: Extract GurgehOnboardingView - Implementation Analysis

## Summary

Successfully extracted the Gurgeh onboarding state machine from `UnifiedApp` (~2200 lines) into a new `GurgehOnboardingView` in `internal/tui/views/`. This is the Extract step of Phase 2c -- both old and new code coexist; no routing changes yet.

## Files Created

### 1. `internal/tui/gurgeh_config.go` (28 lines)
- Defines `GurgehConfig` struct with all dependencies needed by the onboarding view
- Fields: `ResearchCoord`, `CodingAgent`, `AgentSelector`, `SelectedAgent`
- View factory functions for all 7 onboarding steps (Kickoff, Arbiter, SpecSummary, EpicReview, TaskReview, TaskDetail, Sprint)

### 2. `internal/tui/views/gurgeh_helpers.go` (367 lines)
Extracted pure functions (no method receiver) from `unified_app.go`:

**Helper functions (exported for use by both old and new code):**
- `ExtractString`, `ExtractVisionSummary`, `ExtractProblemSummary`, `ExtractUsersSummary` -- extraction from exploration results
- `ToPhaseArtifacts`, `ToVisionArtifact`, `ToProblemArtifact`, `ToUsersArtifact`, `ToEvidenceItems`, `ToPersonas`, `ToQualityScores` -- agent-to-tui type conversion
- `ScanResultToArtifacts`, `ToScanEvidence`, `ToScanResolved` -- tui-to-scan type conversion
- `SafeIndex`, `CreateSpecSummaryFromSprintState`, `ExtractFirstLine`, `ParseBulletItems`, `SummarizeDiff` -- text helpers
- `ToValidationErrors` -- error type conversion

**Internal types (unexported, package-scoped):**
- `scanProgressWithContinuation` -- wraps `ScanProgressMsg` + `nextCmd`
- `agentStreamEvent` -- stream event with line/epics/tasks/err
- `agentStreamWithContinuation` -- wraps `AgentStreamMsg` + `nextCmd`

**Design decision:** Functions are exported (capitalized) because they will be called from the `views` package. The original unexported versions in `unified_app.go` remain unchanged -- both copies coexist during Extract step.

### 3. `internal/tui/views/gurgeh_onboarding.go` (670 lines)
The core extraction -- `GurgehOnboardingView` struct with all handler methods:

**Struct fields:** Mirror the onboarding-related fields from `UnifiedApp` (state, breadcrumb, currentView, project info, generated epics/tasks, loading state, agent run state, view factories, chat settings)

**Handler methods (moved from UnifiedApp with receiver change):**
- `handleProjectCreated` -- transitions Kickoff -> Interview/Sprint
- `handleInterviewComplete` -- transitions Interview -> SpecSummary
- `handleSuggestionsReady` -- passes AI suggestions to interview view
- `handleSpecAccepted` -- transitions SpecSummary -> EpicReview + generation
- `handleEpicsGenerated/Accepted` -- transitions through epic review
- `handleTasksGenerated/Accepted` -- transitions through task review
- `handleSprintComplete` -- sprint -> SpecSummary with sprint state
- `showTaskDetail` -- navigates to task detail view
- `navigateBack`, `navigateToKickoff`, `navigateToStep` -- backward navigation

**Generation methods:**
- `generateSuggestions` -- AI suggestion generation
- `generateEpicsWithAgent` -- epic generation with streaming
- `generateTasksWithAgent` -- task generation with streaming
- `scanCodebase` -- codebase exploration via Claude Code
- `waitForScanProgress` -- progressive scan result reading
- `waitForAgentStream` -- progressive agent stream reading

**Snapshot methods:**
- `captureRunSnapshot`, `finalizeAgentRun` -- diff tracking
- `sendToCurrentView` -- message forwarding (keeps BUG comment)

**Attach helpers:**
- `attachAgentSelector`, `attachAgentName`, `attachChatSettings`

**Local interface copies:**
- `onboardingAgentSelectorSetter`, `onboardingAgentNameSetter`, `onboardingChatSettingsSetter` -- local versions of unexported interfaces from `internal/tui/`

**View interface:**
- `Init()`, `Update()`, `View()`, `Focus()`, `Blur()`, `Name()`, `ShortHelp()` -- full View implementation
- `Breadcrumb()`, `State()`, `SetChatSettings()` -- additional accessors

### 4. `internal/tui/views/gurgeh_onboarding_test.go` (128 lines)
- `TestGurgehOnboardingViewInit` -- verifies factory is called, state is Kickoff, View delegates
- `TestGurgehOnboardingViewProjectCreated` -- verifies ProjectCreatedMsg transitions to Interview state and calls sprint factory
- `TestGurgehOnboardingViewInitNoFactory` -- verifies nil return when no factory set
- `TestGurgehOnboardingViewViewDelegates` -- verifies View() delegates to currentView
- `stubView` -- minimal View implementation for testing

## Import Cycle Resolution

The key constraint: `internal/tui/views/` imports `internal/tui/`, so types like `OnboardingState`, `Breadcrumb`, and all message types MUST stay in `internal/tui/`. Only the **usage** (handler methods, helper functions) moves to `views/`.

All types from `internal/tui/` are prefixed with `tui.` (e.g., `tui.View`, `tui.OnboardingState`, `tui.ProjectCreatedMsg`). The unexported interfaces (`agentSelectorSetter`, etc.) are duplicated locally with `onboarding` prefix.

## Build Verification

- `go build ./internal/tui/...` -- PASS (clean)
- `go build ./internal/tui/views/...` -- PASS (clean)
- `go build ./...` -- PASS (only pre-existing `docs/solutions` type assertion failure, unrelated)
- `go test ./internal/tui/views/...` -- 28/28 PASS (including 4 new tests)
- `go test ./internal/tui/...` -- all PASS (no regressions)

## What Was NOT Changed

- `unified_app.go` -- untouched, both old and new code coexist
- No routing changes -- `UnifiedApp` still handles all messages
- No behavioral changes -- this is Extract-only

## Next Steps (Task 2)

- Make `GurgehView` a container that embeds `GurgehOnboardingView`
- Reroute onboarding messages from `UnifiedApp.Update()` to `GurgehOnboardingView.Update()`
- Remove duplicate code from `unified_app.go`
