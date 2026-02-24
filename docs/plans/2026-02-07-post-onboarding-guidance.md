# Post-Onboarding Next-Step Guidance Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Persist generated spec/epics/tasks when onboarding completes and show the user their freshly created spec instead of "No specs found."

**Architecture:** Thread the `autarch.Client` into `GurgehOnboardingView` via `GurgehConfig`, persist the spec at the moment tasks are accepted (when all artifacts exist), enrich `OnboardingCompleteMsg` with the created spec ID, and have the spec browser pre-select it on transition.

**Tech Stack:** Go, Bubble Tea, `pkg/autarch.Client` for HTTP persistence

---

## Context

After onboarding completes (spec → epics → tasks reviewed and accepted), the user sees "No specs found" because:
- Generated artifacts live only in `GurgehOnboardingView` memory fields
- `OnboardingCompleteMsg` only carries `ProjectID` and `ProjectName`
- `GurgehView` calls `client.ListSpecs("")` which returns empty (nothing was persisted)
- The onboarding view has no access to `autarch.Client` for persistence

The fix has 3 tasks:
1. **Thread the client** into onboarding + persist spec on completion
2. **Enrich the completion message** with spec ID
3. **Pre-select the spec** in the browser on transition

### Task 1: Thread autarch.Client into GurgehOnboardingView and persist spec

**Files:**
- Modify: `internal/tui/gurgeh_config.go` — add `Client *autarch.Client` field
- Modify: `internal/tui/views/gurgeh_onboarding.go` — store client, call `CreateSpec` in `handleTasksAccepted`
- Modify: `internal/tui/views/gurgeh.go:54-55` — pass client through config
- Test: `internal/tui/views/gurgeh_onboarding_test.go` — add test for spec persistence

**Step 1: Add Client field to GurgehConfig**

In `internal/tui/gurgeh_config.go`, add `Client *autarch.Client` to `GurgehConfig`:

```go
type GurgehConfig struct {
	Client        *autarch.Client       // For persisting spec on completion
	ResearchCoord *research.Coordinator
	CodingAgent   *agent.Agent
	// ... rest unchanged
}
```

**Step 2: Store client in GurgehOnboardingView**

In `internal/tui/views/gurgeh_onboarding.go`, add `client *autarch.Client` field to the struct and populate it in `NewGurgehOnboardingView`:

```go
type GurgehOnboardingView struct {
	// Dependencies (from GurgehConfig)
	client        *autarch.Client
	researchCoord *research.Coordinator
	// ... rest unchanged
}

func NewGurgehOnboardingView(cfg tui.GurgehConfig) *GurgehOnboardingView {
	// ...
	return &GurgehOnboardingView{
		client:        cfg.Client,
		researchCoord: cfg.ResearchCoord,
		// ... rest unchanged
	}
}
```

**Step 3: Persist spec in handleTasksAccepted**

In `handleTasksAccepted` (line ~581), before emitting `OnboardingCompleteMsg`, persist the spec:

```go
func (v *GurgehOnboardingView) handleTasksAccepted(msg tui.TasksAcceptedMsg) tea.Cmd {
	v.generatedTasks = msg.Tasks
	v.state = tui.OnboardingComplete
	v.breadcrumb.SetCurrent(tui.OnboardingComplete)

	// Persist spec if client available
	return v.persistAndComplete()
}

func (v *GurgehOnboardingView) persistAndComplete() tea.Cmd {
	return func() tea.Msg {
		specID := ""
		if v.client != nil {
			spec := autarch.Spec{
				Title:   v.projectName,
				Project: v.projectID,
				Vision:  v.specVision(),
				Users:   v.specUsers(),
				Problem: v.specProblem(),
				Status:  autarch.SpecStatusDraft,
			}
			created, err := v.client.CreateSpec(spec)
			if err == nil {
				specID = created.ID
			}
			// Non-fatal: if persistence fails, still transition to dashboard
		}
		return tui.OnboardingCompleteMsg{
			ProjectID:   v.projectID,
			ProjectName: v.projectName,
			SpecID:      specID,
		}
	}
}
```

The `specVision()`, `specUsers()`, `specProblem()` helpers extract the relevant fields from whichever source produced the spec (interview answers or sprint state). These values already exist on the view — check `v.generatedEpics` source or the last `SpecAcceptedMsg` fields. The simplest approach: store the accepted spec fields when `handleSpecAccepted` is called.

**Step 4: Store accepted spec fields**

Add fields to `GurgehOnboardingView`:
```go
acceptedVision  string
acceptedUsers   string
acceptedProblem string
```

In `handleSpecAccepted`:
```go
func (v *GurgehOnboardingView) handleSpecAccepted(msg tui.SpecAcceptedMsg) tea.Cmd {
	v.acceptedVision = msg.Vision
	v.acceptedUsers = msg.Users
	v.acceptedProblem = msg.Problem
	v.state = tui.OnboardingEpicReview
	// ... rest unchanged
}
```

Then `persistAndComplete` uses these fields directly.

**Step 5: Pass client through GurgehView to config**

In `internal/tui/views/gurgeh.go`, `NewGurgehView` already receives `client *autarch.Client`. Pass it into the config:

```go
if cfg != nil {
	cfg.Client = client  // Thread client for spec persistence
	v.onboarding = NewGurgehOnboardingView(*cfg)
}
```

**Step 6: Write test for persistence**

In `internal/tui/views/gurgeh_onboarding_test.go`, add a test that verifies `OnboardingCompleteMsg` carries a spec ID when client is available.

**Step 7: Run tests**

```bash
go test ./internal/tui/... -v -run TestGurgeh
```

**Step 8: Commit**

```bash
git add internal/tui/gurgeh_config.go internal/tui/views/gurgeh_onboarding.go internal/tui/views/gurgeh.go internal/tui/views/gurgeh_onboarding_test.go
git commit -m "feat(tui): persist spec on onboarding completion"
```

### Task 2: Enrich OnboardingCompleteMsg with SpecID

**Files:**
- Modify: `internal/tui/messages.go:46` — add `SpecID` field
- Test: existing tests still pass (field is additive)

**Step 1: Add SpecID to OnboardingCompleteMsg**

```go
type OnboardingCompleteMsg struct {
	ProjectID   string
	ProjectName string
	SpecID      string // ID of the persisted spec (empty if persistence failed)
}
```

**Step 2: Run tests**

```bash
go test ./internal/tui/... -v
```

**Step 3: Commit**

```bash
git add internal/tui/messages.go
git commit -m "feat(tui): add SpecID to OnboardingCompleteMsg"
```

### Task 3: Pre-select spec in browser on transition

**Files:**
- Modify: `internal/tui/views/gurgeh.go:126-130` — extract spec ID from completion msg, pass to browser
- Test: `internal/tui/views/gurgeh_test.go` — add test for spec pre-selection

**Step 1: Handle SpecID in GurgehView**

In `gurgeh.go`, the `OnboardingCompleteMsg` handler (line 126-130):

```go
case tui.OnboardingCompleteMsg:
	v.showBrowser = true
	v.pendingSpecID = msg.SpecID  // Store for selection after specs load
	return v, tea.Batch(
		v.loadSpecs(),
		func() tea.Msg { return msg }, // Still propagate upstream
	)
```

Add `pendingSpecID string` field to `GurgehView`.

In `specsLoadedMsg` handler, after loading specs, select the pending spec:

```go
case specsLoadedMsg:
	v.loading = false
	if msg.err != nil {
		v.err = msg.err
	} else {
		v.specs = msg.specs
		// Pre-select the spec created during onboarding
		if v.pendingSpecID != "" {
			for i, s := range v.specs {
				if s.ID == v.pendingSpecID {
					v.selected = i
					break
				}
			}
			v.pendingSpecID = ""
		}
	}
	return v, nil
```

**Step 2: Write test**

Test that after `OnboardingCompleteMsg` with a `SpecID`, the `specsLoadedMsg` handler selects the matching spec.

**Step 3: Run tests**

```bash
go test ./internal/tui/... -v -run TestGurgeh
```

**Step 4: Commit**

```bash
git add internal/tui/views/gurgeh.go internal/tui/views/gurgeh_test.go
git commit -m "feat(tui): pre-select spec in browser after onboarding"
```

## Notes

- **Persistence is non-fatal**: If the Gurgeh server isn't running or `CreateSpec` fails, onboarding still completes — the user just won't see their spec in the browser. This is the same behavior as today.
- **Fallback emission points**: The other two `OnboardingCompleteMsg` emission points (fallback at line 636, state handler at line 744) should also be updated to attempt persistence if possible. However, these are edge cases (view transition failures, direct state navigation) — address in a follow-up if needed.
- **Epic/task persistence**: The `autarch.Client` also has `CreateEpic` and `CreateTask` methods. A follow-up bead should persist epics and tasks alongside the spec. For this bead, persisting just the spec is sufficient to show "what you created" in the browser.
- **GOCACHE fix**: If using Codex agents, add `GOCACHE=/tmp/go-build-cache` to prompts.
