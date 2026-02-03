---
title: "fix: Kickoff → Arbiter Interview Transition"
type: fix
date: 2026-02-02
bead: Autarch-73j
priority: P1
deepened: 2026-02-02
---

# fix: Kickoff → Arbiter Interview Transition

## Enhancement Summary

**Deepened on:** 2026-02-02
**Research agents used:** best-practices-researcher, code-simplicity-reviewer, julik-frontend-races-reviewer, learnings-researcher (x2), architecture-strategist, repo-research-analyst

### Key Improvements
1. **Simplified approach**: Diagnose first, then fix only what's broken
2. **Additional race conditions identified**: Orphaned goroutines, stale messages, negative dimensions
3. **Codebase patterns documented**: Existing `tea.Batch` usage, WindowSizeMsg handling, compile-time checks

### Critical Insight
The simplicity reviewer found this plan may be over-engineered. The code chain IS implemented. **Step 1 should be reproduction**, not planning 5 phases of speculative fixes.

---

## Overview

The transition from Kickoff view to SprintView (Arbiter interview) **has code implemented** but may fail silently. Before implementing fixes, we must reproduce and diagnose the actual failure.

## Problem Statement

Users report that after completing the Kickoff form, the TUI doesn't reliably start the 8-phase interview. Investigation reveals the code chain exists:

```
KickoffView.createProject() → projectCreatedMsg → onProjectStart callback
→ ProjectCreatedMsg → handleProjectCreated() → SprintView creation
```

**Potential failure points (unconfirmed):**
- Race condition in `tea.Batch` initialization order
- Type assertions without logging in `else` branches
- Missing error handling for `StartSprint` failures
- Potential 0x0 or negative window dimensions

---

## Simplified Technical Approach

### Phase 0: Reproduce First (NEW - REQUIRED)

**Goal:** Confirm the bug exists before writing any fixes

**Steps:**
1. Run `go run ./cmd/autarch tui`
2. Type a project description in Kickoff
3. Press Enter
4. **Observe:** Does SprintView appear? Does Phase 0 (Vision) show?

**If it works:** Close this bead. The bug may have been fixed or was intermittent.

**If it fails:** Note exactly what happens:
- Blank screen?
- Returns to Kickoff?
- Panic with stack trace?
- Hangs indefinitely?

### Phase 1: Diagnostic Logging (Only if Phase 0 fails)

**Goal:** Identify exactly where the transition fails

**Add minimal logging to `handleProjectCreated()` (unified_app.go:505):**
```go
slog.Debug("handleProjectCreated",
    "projectID", msg.ProjectID,
    "hasSprintFactory", a.createSprintView != nil)
```

**Run with debug logging:**
```bash
SLOG_LEVEL=debug go run ./cmd/autarch tui
```

**Look for:**
- Is `handleProjectCreated` called?
- Is `createSprintView` factory set?
- Is SprintView created?
- Does `StartSprint` return a command?

### Phase 2: Fix Specific Issue Found

Based on diagnosis, apply the minimal fix. Options:

#### Option A: If WindowSizeMsg ordering is the issue

**Research finding:** The codebase uses `tea.Batch` everywhere. However, `tea.Sequence` guarantees ordering.

```go
// Current (may race)
cmds := []tea.Cmd{
    a.currentView.Init(),
    a.currentView.Focus(),
    a.sendWindowSize(),
}
return tea.Batch(cmds...)

// Fixed (ordered)
return tea.Sequence(
    a.sendWindowSize(),     // 1. Dimensions first
    a.currentView.Init(),   // 2. Then initialize
    a.currentView.Focus(),  // 3. Finally focus
    startCmd,               // 4. Start sprint last
)
```

**Research Insight (Bubble Tea best practices):**
> `sendWindowSize()` must complete BEFORE `Init()` because many components (viewport, list, table) need dimensions during initialization.

#### Option B: If SprintStarter interface assertion fails silently

**Add logging to the else branch:**
```go
if starter, ok := a.currentView.(SprintStarter); ok {
    startCmd = starter.StartSprint(msg.Description)
} else {
    slog.Warn("SprintView does not implement SprintStarter",
        "viewType", fmt.Sprintf("%T", a.currentView))
}
```

#### Option C: If dimensions cause panic/blank screen

**Add dimension guards (from institutional learnings):**
```go
func (v *SprintView) View() string {
    if v.width <= 0 || v.height <= 0 {
        return "Initializing..." // Match UnifiedApp pattern
    }
    // ... rest of render
}

case tea.WindowSizeMsg:
    // Guard against negative dimensions (terminal < padding)
    v.width = max(0, msg.Width - 6)
    v.height = max(0, msg.Height - 6)
```

**From `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md`:**
> Account for unified_app's content padding: `Padding(1, 3)` = 6 horizontal, 2 vertical

---

## Additional Race Conditions Identified

**From races reviewer (julik-frontend-races-reviewer):**

### 1. StartSprint Goroutine Orphaning

**Problem:** `StartSprint` uses `context.Background()`. If user presses Escape immediately, the goroutine continues running orphaned.

**Fix (if needed):**
```go
func (v *SprintView) StartSprint(ctx context.Context, userInput string) tea.Cmd {
    return func() tea.Msg {
        _, err := v.orch.Start(ctx, userInput)
        // ...
    }
}
```

### 2. Stale Messages After Cancel

**Problem:** If user submits chat message, then quickly submits another, the first response may still arrive and display.

**Fix (if needed):** Add generation ID to correlate messages with conversations.

### 3. No Timeout on Network Operations

**Problem:** If Intermute hangs, TUI freezes with no feedback.

**Fix (if needed):**
```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
```

**Note:** These are **potential** issues identified by research. Only fix if observed during diagnosis.

---

## Codebase Patterns to Follow

**From repo-research-analyst:**

### WindowSizeMsg Pattern
```go
case tea.WindowSizeMsg:
    v.width = msg.Width - 6       // Account for padding
    v.height = msg.Height - 4 - 2 // Account for header/footer
    v.shell.SetSize(v.width, v.height)
    // Propagate to children
    return v, nil
```

### Compile-Time Interface Checks
```go
// Add to sprint_view.go (production code, not just tests)
var _ pkgtui.View = (*SprintView)(nil)
var _ tui.SprintStarter = (*SprintView)(nil)
```

### Error Handling Pattern
```go
// Errors are messages, not panics
case tui.GenerationErrorMsg:
    v.chatPanel.AddMessage("system", "Error: "+msg.Error.Error())
    return v, nil
```

---

## Acceptance Criteria

### Functional Requirements

- [ ] User completes Kickoff form → SprintView appears reliably
- [ ] SprintView shows Phase 0 (Vision) prompt
- [ ] User can type and see response in chat panel

### Quality Gates

- [ ] Bug reproduced and documented (or confirmed not reproducible)
- [ ] Fix targets the specific failure point identified
- [ ] No new race conditions introduced

---

## Testing Plan

### Step 1: Reproduce
```bash
go run ./cmd/autarch tui
# Type description, press Enter, observe
```

### Step 2: Debug (if needed)
```bash
SLOG_LEVEL=debug go run ./cmd/autarch tui 2>&1 | tee debug.log
# Reproduce issue, check debug.log
```

### Edge Cases (only test if relevant to diagnosed issue)
- [ ] Transition with Intermute not running
- [ ] Transition with very small terminal (40x24)
- [ ] Window resize during transition

---

## References

### Internal References
- `internal/tui/unified_app.go:505-622` - handleProjectCreated
- `internal/tui/views/sprint_view.go:52-78` - SprintView constructor
- `cmd/autarch/main.go:281-289` - SprintView factory wiring

### Institutional Learnings Applied
- `docs/solutions/ui-bugs/tui-breadcrumb-hidden-by-oversized-child-view-20260127.md` - WindowSizeMsg must subtract chrome
- `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md` - Padding arithmetic (Width-6, Height-6)

### External References
- [Bubble Tea Package Documentation](https://pkg.go.dev/github.com/charmbracelet/bubbletea)
- [Commands in Bubble Tea (Charm Blog)](https://charm.land/blog/commands-in-bubbletea/)
- [Building Bubble Tea Programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)

### Related Work
- Bead: Autarch-73j (this issue)
- Blocks: Autarch-0iz (Sprint → Spec Summary transition)

---

## Architecture Notes

**From architecture-strategist:**

### ViewTransition Abstraction (Future)
If similar bugs recur, consider extracting a `ViewTransition` struct:
```go
type ViewTransition struct {
    FromView View
    ToView   View
    Init     bool
    Focus    bool
}

func (a *UnifiedApp) executeTransition(t ViewTransition) tea.Cmd
```

**Defer this until immediate fix is stable.**

### Interface Pattern
`tui.View` is a type alias for `pkgtui.View` (backward compatibility). Both work. New code should import `pkgtui` directly.
