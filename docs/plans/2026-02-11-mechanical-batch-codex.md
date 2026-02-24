# Mechanical Batch — Codex Parallel Dispatch Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Fix 3 independent technical debt items via parallel Codex agents: context cancellation guardrails, Pollard YAML persistence tests, and Intermute background startup.

**Architecture:** 3 Codex agents run in parallel, each scoped to a single package with no shared files. All agents write tests for their changes. Results verified independently after all agents complete.

**Tech Stack:** Go 1.24, Bubble Tea v1, gopkg.in/yaml.v3, pkg/yamlsafe wrapper

**Execution:** Codex Delegation — 3 parallel agents dispatched via interclode

---

## Task 1: Context Cancellation Guardrails (Autarch-i0u)

**Bead:** Autarch-i0u
**Files:**
- Modify: `internal/gurgeh/arbiter/tui/arbiter_view.go` (6 sites: lines 161, 169, 323, 356, 365, 412)
- Modify: `internal/tui/views/sprint_view.go` (3 sites: lines 90, 112, 476)
- Test: `internal/gurgeh/arbiter/tui/arbiter_view_test.go` (create if needed)
- Test: `internal/tui/views/sprint_view_test.go` (create if needed)

**Reference pattern** (already correct at `internal/tui/views/sprint_view.go:436`):
```go
ctx, cancel := context.WithCancel(context.TODO())
v.cancelChat = cancel
v.responseCh = v.orch.ProcessChatMessage(ctx, msg)
```

**Reference pattern** (already correct at `internal/tui/views/gurgeh_onboarding.go:94`):
```go
ctx, cancel := context.WithCancel(context.TODO())
return &GurgehOnboardingView{
    ctx:    ctx,
    cancel: cancel,
}
// cancel() called in Blur()
```

**Step 1: Add context + cancel fields to arbiter_view.go**

Add a `ctx` and `cancel` field to the arbiter view struct. Initialize with `context.WithCancel(context.TODO())` in the constructor. Call `cancel()` in the cleanup/blur method.

**Step 2: Replace all 6 context.TODO() sites in arbiter_view.go**

Replace each bare `context.TODO()` with `v.ctx` (the stored cancellable context):

- Line 161: `v.orchestrator.Start(v.ctx, "")`
- Line 169: `v.orchestrator.Start(v.ctx, input)`
- Line 323: `brief.Decompose(v.ctx, spec, v.projectPath)`
- Line 356: `v.orchestrator.StartDeepScan(v.ctx, v.state)`
- Line 365: `v.coordinator.StartRun(v.ctx, v.state.ID, ...)`
- Line 412: `v.orchestrator.AcceptAndAdvance(v.ctx)`

**Step 3: Replace 3 context.TODO() sites in sprint_view.go**

sprint_view.go already has the correct pattern at line 436. Apply the same pattern to lines 90, 112, 476 — use `v.ctx` (the view's stored cancellable context, or add one if not present).

- Line 90: `v.orch.Start(v.ctx, userInput)`
- Line 112: `v.orch.StartWithScan(v.ctx, userInput, artifacts)`
- Line 476: `v.orch.ChatAcceptDraft(v.ctx)`

**Step 4: Write cancellation tests**

Write tests that:
1. Create a view with a cancellable context
2. Cancel the context
3. Verify the view handles cancellation gracefully (no panics, goroutines cleaned up)

**Step 5: Run scoped tests**

```bash
GOCACHE=/tmp/go-build-cache go test ./internal/gurgeh/arbiter/tui/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go test ./internal/tui/views/... -v -short -count=1 -run TestSprint
```

**Step 6: Verify no regressions**

```bash
GOCACHE=/tmp/go-build-cache go build ./cmd/autarch/...
```

---

## Task 2: Pollard YAML Persistence Tests (Autarch-6eg)

**Bead:** Autarch-6eg
**Files:**
- Test: `internal/pollard/sources/source_test.go` (create)
- Test: `internal/pollard/insights/insight_test.go` (create)
- Test: `internal/pollard/patterns/pattern_test.go` (create)
- Test: `internal/pollard/config/config_test.go` (create)
- Reference: `internal/pollard/sources/source.go` (Save at line 245, Load at line 260)
- Reference: `internal/pollard/insights/insight.go` (Save at line 85, Load at line 76, LoadAll at line 101)
- Reference: `internal/pollard/patterns/pattern.go` (Save at line 53, Load at line 44, LoadAll at line 69)
- Reference: `internal/pollard/config/config.go` (Load at line 136, Save at line 236)

**Step 1: Write source_test.go — Save/Load round-trip tests**

Table-driven tests for SourceCollection:
```go
func TestSourceCollectionSaveLoad(t *testing.T) {
    // Create a SourceCollection with various source types
    // Save to temp dir
    // Load from same path
    // Assert deep equality
}

func TestSourceCollectionLoadMissing(t *testing.T) {
    // Load from non-existent path → error
}

func TestSourceCollectionSaveLoadEmpty(t *testing.T) {
    // Empty collection round-trips correctly
}
```

**Step 2: Write insight_test.go — Save/Load/LoadAll tests**

```go
func TestInsightSaveLoad(t *testing.T) {
    // Create Insight with all fields populated
    // Save to temp .pollard/insights/
    // Load by path
    // Assert equality
}

func TestInsightLoadAll(t *testing.T) {
    // Save 3 insights
    // LoadAll
    // Assert count == 3
}

func TestInsightLoadMalformed(t *testing.T) {
    // Write invalid YAML to file
    // Load → expect error
}
```

**Step 3: Write pattern_test.go — Save/Load/LoadAll tests**

Same structure as insight_test.go but for Pattern type.

**Step 4: Write config_test.go — Load/Save with strict validation**

```go
func TestConfigSaveLoadRoundTrip(t *testing.T) {
    // Create Config with hunters, scoring, pipeline
    // Save, Load, compare
}

func TestConfigLoadStrict_RejectsUnknownFields(t *testing.T) {
    // Write YAML with extra field
    // Load → expect error (strict mode)
}

func TestConfigLoadDefaults(t *testing.T) {
    // Write minimal YAML
    // Load → verify defaults applied
}
```

**Step 5: Run all Pollard tests**

```bash
GOCACHE=/tmp/go-build-cache go test ./internal/pollard/... -v -short -count=1
```

---

## Task 3: Intermute Background Startup (Autarch-wq8)

**Bead:** Autarch-wq8
**Files:**
- Modify: `cmd/autarch/main.go` (lines ~120-140, the EnsureRunning call in tuiCmd)
- Modify: `internal/tui/unified_app.go` (add message handler for startup result)
- Create or modify: `internal/tui/messages.go` (add IntermuteStartedMsg, IntermuteStartFailedMsg)
- Test: `internal/intermute/manager_test.go` (create if needed)

**Current synchronous pattern** (in `cmd/autarch/main.go:125-135`):
```go
ensureCtx, ensureCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer ensureCancel()
cleanup, err := mgr.EnsureRunning(ensureCtx)
if err != nil {
    return fmt.Errorf("failed to ensure intermute running: %w", err)
}
defer cleanup()
```

**Step 1: Add message types**

In `internal/tui/messages.go`, add:
```go
// IntermuteStartedMsg is sent when Intermute server is confirmed running.
type IntermuteStartedMsg struct {
    Cleanup func()
}

// IntermuteStartFailedMsg is sent when Intermute server fails to start.
type IntermuteStartFailedMsg struct {
    Error error
}
```

**Step 2: Create a tea.Cmd for EnsureRunning**

In `cmd/autarch/main.go`, replace the synchronous EnsureRunning call with a tea.Cmd that the UnifiedApp dispatches on Init:

```go
// Instead of blocking on EnsureRunning, pass the manager to UnifiedApp
// and let it start asynchronously
app := tui.NewUnifiedApp(opts..., tui.WithIntermuteManager(mgr))
```

The UnifiedApp's Init() returns a Cmd that runs EnsureRunning in a goroutine:

```go
func (a *UnifiedApp) startIntermute() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        cleanup, err := a.intermuteMgr.EnsureRunning(ctx)
        if err != nil {
            return tui.IntermuteStartFailedMsg{Error: err}
        }
        return tui.IntermuteStartedMsg{Cleanup: cleanup}
    }
}
```

**Step 3: Handle messages in UnifiedApp.Update()**

```go
case tui.IntermuteStartedMsg:
    a.intermuteCleanup = msg.Cleanup
    // Intermute ready — views can now connect
    return a, nil

case tui.IntermuteStartFailedMsg:
    // Log error, continue in offline mode
    slog.Error("intermute startup failed", "error", msg.Error)
    return a, nil
```

**Step 4: Call cleanup on quit**

In the UnifiedApp's quit handler, call `a.intermuteCleanup()` if set.

**Step 5: Write tests**

Test that:
1. `startIntermute()` returns a valid tea.Cmd
2. The Cmd produces `IntermuteStartedMsg` when server is reachable
3. The Cmd produces `IntermuteStartFailedMsg` when server can't start
4. UnifiedApp handles both messages without panics

**Step 6: Run tests and build**

```bash
GOCACHE=/tmp/go-build-cache go test ./internal/tui/... -v -short -count=1 -run TestIntermute
GOCACHE=/tmp/go-build-cache go test ./internal/intermute/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go build ./cmd/autarch/...
```

---

## Global Constraints (All Agents)

- **Environment:** `GOCACHE=/tmp/go-build-cache`
- **Test scope:** Always use `-short -count=1` and `-run TestPattern` to avoid integration test hangs
- **Do NOT:** run `go test ./... -v` (hangs on arbiter integration tests)
- **Do NOT:** commit, push, or modify git state
- **Do NOT:** touch files outside the scoped packages
- **Do NOT:** add unnecessary dependencies
- **Verify:** Build must pass: `GOCACHE=/tmp/go-build-cache go build ./cmd/autarch/...`

## Verification After All Agents Complete

```bash
# Check what changed
git diff --stat

# Build all
GOCACHE=/tmp/go-build-cache go build ./cmd/...

# Run all affected tests
GOCACHE=/tmp/go-build-cache go test ./internal/gurgeh/arbiter/tui/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go test ./internal/tui/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go test ./internal/pollard/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go test ./internal/intermute/... -v -short -count=1
GOCACHE=/tmp/go-build-cache go test ./pkg/intermute/... -v -short -count=1

# Revert any unrelated cosmetic changes
git diff --stat  # review before committing
```
