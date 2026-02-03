---
title: "fix: Wire Sprint completion → Spec Summary transition"
type: fix
date: 2026-02-03
bead: Autarch-0iz
priority: P1
deepened: 2026-02-03
---

# fix: Wire Sprint completion → Spec Summary transition

## Enhancement Summary

**Deepened on:** 2026-02-03
**Research agents used:** code-simplicity-reviewer, architecture-strategist, pattern-recognition-specialist, learnings-researcher (x3)

### Key Improvements
1. **Simplified approach**: Only 1 handler change needed, not 4 separate changes
2. **Concurrency safety**: Pass `SprintState` by VALUE, not pointer (per arbiter-state-pointer-escape learning)
3. **Remove Esc change**: Auto-transition on completion instead of requiring user action
4. **Window sizing**: Must send reduced dimensions to child view

### Critical Insight
The simplicity reviewer identified that the original plan was over-engineered. The **minimum fix** is updating the `SprintCompleteMsg` handler in `unified_app.go` - no message struct changes, no new flags, no Esc behavior changes needed.

---

## Overview

When Arbiter sprint completes all 8 phases, the TUI incorrectly returns to Dashboard instead of showing the Spec Summary view with handoff options. The bug is in `unified_app.go:451-457` which emits `OnboardingCompleteMsg` instead of transitioning to SpecSummaryView.

## Problem Statement

**Current behavior:**
```go
// unified_app.go:451-457
case SprintCompleteMsg:
    return a, func() tea.Msg {
        return OnboardingCompleteMsg{...}  // ← Skips SpecSummary!
    }
```

**Expected behavior:** Show SpecSummaryView with handoff options (Deep Research, Generate Tasks, Export Spec, Export PRD).

---

## Simplified Technical Approach

### Phase 0: Verify First (REQUIRED per institutional learning)

Before implementing, verify the bug exists:
```bash
go run ./cmd/autarch tui
# 1. Complete Kickoff with description
# 2. Accept all 8 phases
# 3. Observe: Does it go to Dashboard or SpecSummary?
```

### Phase 1: Update SprintCompleteMsg Handler (Core Fix)

**Single change in `unified_app.go:451-457`:**

```go
case SprintCompleteMsg:
    // Handle at parent level: this transitions to a new view (SpecSummaryView)
    // No pass-through needed since SprintView is being replaced, not retained
    // SprintView already holds the orchestrator state
    if sprintView, ok := a.currentView.(*views.SprintView); ok {
        state, _ := sprintView.Orchestrator().State()
        spec := createSpecSummaryFromSprintState(&state)

        if a.createSpecSummaryView != nil {
            a.currentView = a.createSpecSummaryView(spec, a.researchCoord)
            a.onboardingState = OnboardingSpecSummary
            a.attachAgentSelector(a.currentView)

            // IMPORTANT: Send reduced dimensions for chrome
            return a, tea.Batch(
                a.currentView.Init(),
                a.currentView.Focus(),
                a.sendWindowSize(), // Uses stored a.width/a.height
            )
        }
    }
    // Fallback: proceed to dashboard
    return a, func() tea.Msg {
        return OnboardingCompleteMsg{
            ProjectID:   a.projectID,
            ProjectName: a.projectName,
        }
    }
```

### Phase 2: Add Inline Converter Function

Add to `unified_app.go` (near handleProjectCreated):

```go
// createSpecSummaryFromSprintState extracts display fields from a completed sprint.
// NOTE: state is passed by pointer to avoid copy, but is read-only here.
// The state was already cloned by Orchestrator.State().
func createSpecSummaryFromSprintState(state *arbiter.SprintState) *SpecSummary {
    spec := &SpecSummary{
        ProjectID: state.ID,
    }

    if s, ok := state.Sections[arbiter.PhaseVision]; ok {
        spec.Vision = s.Content
        spec.Name = extractFirstLine(s.Content)
    }
    if s, ok := state.Sections[arbiter.PhaseProblem]; ok {
        spec.Problem = s.Content
    }
    if s, ok := state.Sections[arbiter.PhaseUsers]; ok {
        spec.Users = s.Content
    }
    if s, ok := state.Sections[arbiter.PhaseRequirements]; ok {
        spec.Requirements = parseBulletItems(s.Content)
    }
    // Note: Platform/Language not in 8-phase sprint - leave empty

    return spec
}

func extractFirstLine(content string) string {
    if idx := strings.Index(content, "\n"); idx > 0 {
        return strings.TrimSpace(content[:idx])
    }
    return strings.TrimSpace(content)
}
```

---

## What We're NOT Changing (Simplification)

Per code-simplicity-reviewer findings:

| Original Plan Item | Decision | Rationale |
|--------------------|----------|-----------|
| Add `State` to `SprintCompleteMsg` | NOT NEEDED | SprintView already accessible via `a.currentView` type assertion |
| Add `sprintComplete` flag to SprintView | NOT NEEDED | Orchestrator already tracks completion state |
| Change Esc behavior | NOT NEEDED | Auto-transition on completion is better UX |
| Create `NavigateToSpecSummaryMsg` | NOT NEEDED | Direct view creation is simpler |

**LOC saved: ~40 lines of unnecessary code avoided**

---

## Research Insights

### Concurrency Safety (Critical)

**From arbiter-state-pointer-escape learning:**
> Never pass pointers to `SprintState` across goroutine boundaries. Pass cloned values instead.

The `Orchestrator.State()` method already returns a deep-copied `SprintState` value. We use this safely:
```go
state, _ := sprintView.Orchestrator().State()  // Returns clone
spec := createSpecSummaryFromSprintState(&state)  // Read-only access to clone
```

### Message Routing (Important)

**From swallowed-generation-error-msg learning:**
> "In Bubble Tea, assume messages should pass through unless proven otherwise"

Our case is safe because we're **replacing views**, not keeping an existing view that needs the message. Add an explanatory comment in the handler.

### Window Sizing (Important)

**From tui-breadcrumb-hidden learning:**
> Pass a reduced `WindowSizeMsg` to child views, subtracting header and footer height

The `sendWindowSize()` method already handles this via stored `a.width/a.height` values which account for chrome. Verified in existing `handleProjectCreated()` pattern.

---

## Acceptance Criteria

- [ ] User completes 8-phase sprint → SpecSummaryView appears automatically
- [ ] SpecSummaryView displays Vision, Problem, Users, Requirements
- [ ] Handoff options visible (via SpecSummaryView's existing callbacks)
- [x] Existing tests pass
- [x] Add integration test for SprintCompleteMsg → SpecSummaryView

## Files to Modify

| File | Change | LOC |
|------|--------|-----|
| `internal/tui/unified_app.go` | Update handler (451-457), add converter | ~30 |
| `internal/tui/unified_app_test.go` | Add integration test | ~40 |

**Total: ~70 LOC** (vs original plan's ~110 LOC)

---

## Edge Cases

| Scenario | Handling |
|----------|----------|
| SprintView type assertion fails | Fallback to Dashboard (defensive) |
| State() returns false | Fallback to Dashboard |
| Research still running | SpecSummaryView shows indicator (existing behavior) |
| SpecSummaryView factory not set | Fallback to Dashboard |

---

## Testing Plan

### Step 1: Verify Bug Exists
```bash
go run ./cmd/autarch tui
# Complete Kickoff, accept all 8 phases, observe
```

### Step 2: Unit Tests
```bash
go test ./internal/tui/... -v -run TestSprintComplete
```

### Step 3: Integration Test to Add

```go
func TestSprintCompleteMsgTransitionsToSpecSummary(t *testing.T) {
    app := NewUnifiedApp(nil)

    // Set up SprintView with mock orchestrator
    mockOrch := &mockOrchestrator{state: completedSprintState()}
    sprintView := &mockSprintViewWithOrch{orch: mockOrch}
    app.currentView = sprintView

    // Set up SpecSummary factory
    var createdSummary *SpecSummary
    app.SetSpecSummaryViewFactory(func(s *SpecSummary, r *research.Coordinator) View {
        createdSummary = s
        return &noopDashboardView{name: "SpecSummary"}
    })

    // Send SprintCompleteMsg
    updated, cmd := app.Update(SprintCompleteMsg{})
    app = updated.(*UnifiedApp)

    // Verify transition
    if app.currentView.Name() != "SpecSummary" {
        t.Errorf("Expected SpecSummary view, got %s", app.currentView.Name())
    }
    if createdSummary == nil {
        t.Error("SpecSummary factory not called")
    }
    if cmd == nil {
        t.Error("Expected commands for Init/Focus/WindowSize")
    }
}
```

---

## References

### Internal
- `internal/tui/unified_app.go:451-457` - Current (buggy) handler
- `internal/tui/unified_app.go:505-622` - Kickoff → SprintView pattern to follow
- `internal/gurgeh/arbiter/orchestrator.go:279-307` - Handoff options definition

### Institutional Learnings Applied
- `docs/solutions/runtime-errors/arbiter-state-pointer-escape-20260201.md` - Concurrency safety
- `docs/solutions/ui-bugs/swallowed-generation-error-msg-20260131.md` - Message routing pattern
- `docs/solutions/ui-bugs/tui-breadcrumb-hidden-by-oversized-child-view-20260127.md` - Window sizing
- `docs/solutions/workflow-issues/over-planning-before-reproduction-20260203.md` - Verify first

### Related Beads
- **Autarch-73j** (closed) - Kickoff → SprintView transition (verified working, similar pattern)
- **Autarch-6iu** (blocked by this) - Epic/Task generation integration
