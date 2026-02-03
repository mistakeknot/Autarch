---
title: "Oracle Architecture Review: Issues 3-6 Resolution"
category: patterns
tags: [architecture-review, oracle, code-cleanup, constants, error-surfacing, focus-routing]
module: internal/gurgeh/arbiter
symptom: "Orphaned legacy code, brittle hard-coded indices, silent error handling, missing keyboard focus routing, and misaligned quick scan timing"
root_cause: "Iterative refactoring left duplicate implementations; phase-based logic used array indices instead of semantic constants; error handling optimized for 'happy path' continuity without operator visibility; TUI focus state not wired into keyboard routing; quick scan timing not aligned with phase dependencies"
date_resolved: "2026-02-01"
---

# Oracle Architecture Review: Issues 3-6 Resolution

**Review Date:** 2026-02-01
**Model:** GPT-5.2 Pro via Oracle browser automation
**Context:** 173 files (~258K tokens)
**Commit:** f406a0b

## Problem Statement

An Oracle architecture review of Autarch/Gurgeh identified 6 critical issues. This document covers the resolution of issues 3-6:

### Issue 3: Orphaned Legacy Code

**Symptom:** Two orphaned directories existed:
- `internal/gurgeh/confidence/` (155 lines calculator + 91 lines tests)
- `internal/gurgeh/consistency/` (212 lines checker + 166 lines tests)

**Context:** These were superseded by the arbiter refactoring:
- `internal/gurgeh/arbiter/confidence/` — canonical confidence scoring
- `internal/gurgeh/arbiter/consistency/` — canonical consistency checking

**Impact:** 621 lines of dead code; potential confusion about which implementation to use; risk of accidental imports leading to logic drift.

### Issue 4a: Hard-coded Section Indices

**Symptom:** Consistency engine used array indices instead of Phase constants:

```go
// BEFORE (arbiter/consistency/engine.go:51-109)
problem := sections[1]    // PhaseProblem (after PhaseVision=0)
features := sections[3]   // PhaseFeaturesGoals (after PhaseVision=0, PhaseProblem=1, PhaseUsers=2)
Sections: []int{1, 3},
```

**Risk:** If phase ordering changes (e.g., inserting a new phase between Problem and Users), indices silently drift. Comments like `// PhaseProblem (after PhaseVision=0)` indicate awareness of fragility but don't prevent the failure mode.

**Root Cause:** Import cycles prevent importing `arbiter.Phase` constants directly into `consistency/` package.

### Issue 4b: UserStory.Text Sharing (TODO)

**Symptom:** Both Problem and Users phases map to the same `spec.UserStory.Text` field in review mode:

```go
// orchestrator.go:320-415 (extractSectionFromSpec)
case PhaseProblem:
    content = spec.UserStory.Text
case PhaseUsers:
    content = spec.UserStory.Text  // Same field!
```

**Context:** Problem phase describes the core user pain point; Users phase should describe actors, personas, and demographics. Sharing `UserStory.Text` causes semantic collision.

**Status:** TODO — flagged for future fix. Requires expanding `specs.Spec` with distinct `Actors` or `Personas` field.

### Issue 4c: Quick Scan Trigger Timing

**Symptom:** Quick scan triggered when advancing to **FeaturesGoals** phase:

```go
// BEFORE (orchestrator.go:205-214)
if state.Phase == PhaseFeaturesGoals {
    o.runQuickScan(ctx, state)
}
```

**Context:** Quick scan gathers competitive intelligence (GitHub projects, HN discussions) to inform product positioning. Triggering at FeaturesGoals means **Users** phase drafts are generated without scan evidence.

**Expected Flow:** Scan should run early (after Problem) so insights inform Users and Features phases.

### Issue 5: Focus Routing in SprintView

**Symptom:** `SprintView.Update()` didn't consult `shell.Focus()` state. Tab key visually toggled focus in ShellLayout (document ↔ chat), but keyboard handlers always routed to chat input.

**Impact:** Users couldn't scroll the document pane using arrow keys; all keystrokes went to chat composer even when document was visually focused.

**Pattern:** Other views (e.g., `signals.go:152-176`) correctly route based on `shell.Focus()`.

### Issue 6: Silent Errors

**Symptom:** Multiple critical failures were silently ignored:

1. **Intermute spec creation failure** (`orchestrator.go:111-118`):
   ```go
   specID, err := o.research.CreateSpec(ctx, state.ID, title)
   if err != nil {
       _ = err  // Non-fatal: sprint can proceed without research tracking
   }
   ```

2. **Quick scan failure** (`orchestrator.go:635-677`):
   ```go
   result, err := o.scanner.Scan(ctx, topic, o.projectPath)
   if err != nil {
       return  // Silent failure
   }
   ```

3. **Phase research failure** (`orchestrator.go:590-634`):
   ```go
   _ = o.research.RunTargetedScan(ctx, state.SpecID, cfg.Hunters, cfg.Mode, query)
   ```

**Impact:** Operators had no visibility into partial failures. Sprints appeared to succeed but lacked research evidence.

## Solutions Applied

### Solution 3: Delete Orphaned Directories

**Action:** Removed 621 lines of orphaned code:

```bash
git rm -r internal/gurgeh/confidence/
git rm -r internal/gurgeh/consistency/
```

**Files Deleted:**
- `internal/gurgeh/confidence/calculator.go` (155 lines)
- `internal/gurgeh/confidence/calculator_test.go` (91 lines)
- `internal/gurgeh/consistency/checker.go` (212 lines)
- `internal/gurgeh/consistency/checker_test.go` (166 lines)

**Verification:** All imports resolved to `internal/gurgeh/arbiter/{confidence,consistency}/`.

### Solution 4a: Use Phase Constants

**Action:** Added phase index constants to `consistency/engine.go` to mirror `arbiter.Phase` enum:

```go
// internal/gurgeh/arbiter/consistency/engine.go
const (
    PhaseVision        = 0
    PhaseProblem       = 1
    PhaseUsers         = 2
    PhaseFeaturesGoals = 3
)
```

**Refactored Code:**

```go
// BEFORE
problem := sections[1]
features := sections[3]
Sections: []int{1, 3},

// AFTER
problem := sections[int(PhaseProblem)]
features := sections[int(PhaseFeaturesGoals)]
Sections: []int{int(PhaseProblem), int(PhaseFeaturesGoals)},
```

**Rationale:** Constants are local to avoid import cycles but semantically tied to phase ordering. Comment at top of file documents the mirroring.

### Solution 4b: TODO for UserStory.Text Sharing

**Action:** Added inline TODO at the collision site:

```go
// orchestrator.go:347-348
case PhaseUsers:
    // TODO: UserStory only has Text — needs a distinct Actors/Personas field for Users phase
    content = spec.UserStory.Text
```

**Future Work:** Add `specs.Spec.Actors` or `specs.Spec.Personas` field; update `extractSectionFromSpec` and `fillSpecFromSprint` accordingly.

### Solution 4c: Move Quick Scan Trigger to Users Phase

**Action:** Moved trigger from FeaturesGoals to Users:

```go
// BEFORE (orchestrator.go:208)
if state.Phase == PhaseFeaturesGoals {
    o.runQuickScan(ctx, state)
}

// AFTER (orchestrator.go:208)
// Trigger quick scan when advancing to Users so scan evidence informs the Users phase
if state.Phase == PhaseUsers {
    o.runQuickScan(ctx, state)
}
```

**Flow Impact:**
- **Before:** Problem (draft) → Users (draft without scan) → FeaturesGoals (scan runs) → Requirements (scan available)
- **After:** Problem (draft) → Users (scan runs immediately) → Users (draft with scan evidence) → FeaturesGoals (scan available)

### Solution 5: Add ShellLayout Focus Routing

**Action:** Modified `SprintView.Update()` to route keyboard input based on `shell.Focus()`:

```go
// BEFORE (sprint_view.go:191-217)
if v.chatPanel.Focused() {
    switch {
    case msg.Type == tea.KeyEnter:
        return v, v.handleChatSubmit()
    // ... other chat-specific keys
    }
}

// AFTER (sprint_view.go:196-227)
switch v.shell.Focus() {
case pkgtui.FocusDocument:
    switch {
    case key.Matches(msg, v.keys.Back):
        if v.onBack != nil {
            v.cancelStreaming()
            return v, v.onBack()
        }
    case key.Matches(msg, v.keys.NavUp):
        v.docPanel.ScrollUp()
    case key.Matches(msg, v.keys.NavDown):
        v.docPanel.ScrollDown()
    }
    return v, nil

default: // FocusChat
    switch {
    case msg.Type == tea.KeyEnter:
        return v, v.handleChatSubmit()
    // ... other chat-specific keys
    }
}
```

**Behavior:**
- **Tab:** Toggles focus between document and chat (via ShellLayout)
- **Arrow keys (document focused):** Scroll document pane
- **Arrow keys (chat focused):** Navigate chat history
- **Enter (chat focused):** Submit message

**Pattern:** Matches `signals.go:152-176` focus routing implementation.

### Solution 6: Surface Silent Errors

**Action:** Added stderr warnings for all silent failures:

```go
// 1. Intermute spec creation (orchestrator.go:96)
specID, err := o.research.CreateSpec(ctx, state.ID, title)
if err != nil {
    // Non-fatal: sprint can proceed without research tracking
    fmt.Fprintf(os.Stderr, "warning: Intermute spec creation failed: %v\n", err)
} else {
    state.SpecID = specID
}

// 2. Phase research (orchestrator.go:640-642)
if err := o.research.RunTargetedScan(ctx, state.SpecID, cfg.Hunters, cfg.Mode, query); err != nil {
    fmt.Fprintf(os.Stderr, "warning: phase research failed for %s: %v\n", state.Phase, err)
}

// 3. Quick scan (orchestrator.go:667)
result, err := o.scanner.Scan(ctx, topic, o.projectPath)
if err != nil {
    fmt.Fprintf(os.Stderr, "warning: quick scan failed: %v\n", err)
    return
}
```

**Rationale:** Sprints are non-fatal continuations (users can proceed without research), but operators need visibility into partial failures for debugging and monitoring.

## Key Patterns

### Pattern 1: Phase Constants for Import-Cycle-Free Coupling

**Problem:** Consistency engine needs phase semantics but can't import `arbiter.Phase` enum (import cycle).

**Solution:** Mirror constants locally with documentation:

```go
// Phase indices mirror arbiter.Phase to avoid import cycles.
const (
    PhaseVision        = 0
    PhaseProblem       = 1
    PhaseUsers         = 2
    PhaseFeaturesGoals = 3
)
```

**When to Use:**
- Package A defines an enum
- Package B needs the enum values but A imports B
- Enum order is stable (changing order is a breaking change)

**Alternative Considered:** Extract Phase enum to `pkg/arbiter/phase.go` — rejected because Phase is tightly coupled to Gurgeh's domain model.

### Pattern 2: Focus-Aware Keyboard Routing

**Problem:** Multi-pane TUIs need context-sensitive keyboard handling.

**Solution:** Route keys through focus state machine:

```go
switch v.shell.Focus() {
case pkgtui.FocusDocument:
    // Handle document-specific keys (scroll, search, etc.)
case pkgtui.FocusChat:
    // Handle chat-specific keys (input, history, etc.)
case pkgtui.FocusSidebar:
    // Handle sidebar-specific keys (navigation, filtering, etc.)
}
```

**Requirements:**
- ShellLayout manages focus state
- Tab key cycles through focus targets
- Each focus target has distinct key bindings
- Non-focused panes ignore most keys (except global shortcuts like Esc)

**Reference Implementation:** `pkg/tui/signals.go:152-176`

### Pattern 3: Visible Continuations for Non-Fatal Errors

**Problem:** Some failures are recoverable (sprint proceeds without research), but silent failures hide operational issues.

**Solution:** Log warnings to stderr while continuing execution:

```go
if err != nil {
    fmt.Fprintf(os.Stderr, "warning: %s failed: %v\n", operation, err)
    // Continue without research data
}
```

**Decision Criteria:**
- **Fatal error:** Return error, halt execution
- **Recoverable with degraded UX:** Log warning, continue
- **Expected no-op:** Silent (e.g., research provider returns nil for missing config)

**Observability:** Warnings appear in:
- Direct CLI invocation: stderr visible in terminal
- Systemd service: captured in journal (`journalctl -u gurgeh`)
- TUI mode: stderr redirected to log file

## Prevention Strategies

### Strategy 1: Orphaned Code Detection

**Root Cause:** Refactoring moved logic to new packages but didn't delete old implementations.

**Automated Detection:**
1. **Dead code analysis:** Run `deadcode` tool weekly:
   ```bash
   go install golang.org/x/tools/cmd/deadcode@latest
   deadcode -test ./...
   ```

2. **Dependency graph review:** Generate import graph on major refactors:
   ```bash
   go mod graph | grep 'gurgeh'
   ```

3. **Grep for duplicates:** Search for duplicate type/function names:
   ```bash
   rg 'type Calculator struct' --type go
   rg 'func.*Calculate.*Confidence' --type go
   ```

**Policy:** Before merging package refactors, explicitly document "Legacy cleanup" section in PR:
- [ ] Old package deleted or deprecated
- [ ] All imports redirected to new package
- [ ] Tests migrated or removed

### Strategy 2: Phase Constant Enforcement

**Root Cause:** Array indices are valid Go code but semantically fragile.

**Linting Rule:** Ban raw numeric indices into `Sections` map:

```yaml
# .golangci.yml
linters-settings:
  gocritic:
    enabled-checks:
      - indexAlloc  # Warn on map[int] access with literals
```

**Code Review Checklist:**
- [ ] Phase access uses constants (e.g., `sections[PhaseProblem]`)
- [ ] No numeric literals (e.g., `sections[1]`)
- [ ] Consistency checker imports mirrored constants

**Future:** Consider refactoring to `map[Phase]Section` (eliminates integer key entirely).

### Strategy 3: Error Observability Standards

**Root Cause:** "Non-fatal" became "invisible."

**Logging Policy:**
- **Fatal errors:** Return error to caller
- **Degraded operation:** Log to stderr with `warning:` prefix
- **Expected behavior:** No log (e.g., research provider returns nil when Pollard not configured)

**Template:**
```go
if err != nil {
    fmt.Fprintf(os.Stderr, "warning: %s failed: %v\n", operationName, err)
    // Proceed with degraded functionality
}
```

**Observability Tools:**
- **Local dev:** stderr visible in terminal
- **Production:** Structured logging via `slog` (future enhancement)
- **Monitoring:** Count warnings via log aggregation (Loki/Grafana)

### Strategy 4: Focus State Testing

**Root Cause:** Focus state and keyboard routing implemented separately; no integration tests.

**Test Coverage:**
1. **Unit tests** for focus transitions:
   ```go
   func TestShellLayoutFocusCycle(t *testing.T) {
       shell := NewShellLayout()
       assert.Equal(t, FocusChat, shell.Focus())
       shell.CycleFocus()
       assert.Equal(t, FocusDocument, shell.Focus())
       shell.CycleFocus()
       assert.Equal(t, FocusChat, shell.Focus())
   }
   ```

2. **Integration tests** for key routing:
   ```go
   func TestSprintViewDocumentScrollWhenFocused(t *testing.T) {
       view := NewSprintView(...)
       view.shell.SetFocus(FocusDocument)
       _, cmd := view.Update(tea.KeyMsg{Type: tea.KeyDown})
       // Assert: document scrolled
       // Assert: chat input unchanged
   }
   ```

**CI Gate:** Focus routing tests must pass before merging TUI changes.

### Strategy 5: Phase Timing Validation

**Root Cause:** Quick scan timing was documented in one place (comments) but implemented differently.

**Documentation Contract:**
- **Source of truth:** `docs/gurgeh/arbiter-phases.md` (to be created)
- **Implementation reference:** `orchestrator.go:Advance()` includes phase-by-phase comments

**Validation:**
1. **Automated test:** Assert scan happens at Users phase:
   ```go
   func TestQuickScanTriggeredAtUsersPhase(t *testing.T) {
       orch := NewOrchestrator(mockScanner, ...)
       orch.Start(ctx, "input")
       orch.AcceptDraft(ctx)  // Advance to Problem
       orch.AcceptDraft(ctx)  // Advance to Users → scan should trigger
       assert.True(t, mockScanner.ScanCalled)
   }
   ```

2. **Documentation sync check:** PR template includes:
   - [ ] If changing phase logic, update `docs/gurgeh/arbiter-phases.md`
   - [ ] If changing scan timing, update tests

## Related Issues

**Oracle Review Issues 1-2** (addressed in separate commit):
- **Issue 1:** Orchestrator state ownership + concurrency (State() pointer escape, ChatAcceptDraft manual unlock)
- **Issue 2:** Research integration incomplete (Intermute coordinator unused, RunTargetedScan no-op)

**Future Work (from Oracle review):**
- Implement real `RunTargetedScan` (currently no-op)
- Add tests for phase transitions, blocker gating, confidence scoring
- Resolve duplicate SprintView implementations (`views/sprint_view.go` vs `gurgeh/tui/sprint.go`)

## Verification

### Before (Issues Present)

```bash
# Check for orphaned code
$ rg 'internal/gurgeh/confidence' --type go
internal/gurgeh/consistency/checker_test.go:6:    "github.com/mistakeknot/autarch/internal/gurgeh/confidence"

# Check for hard-coded indices
$ rg 'sections\[1\]' internal/gurgeh/arbiter/consistency/
51:    problem := sections[1]    // PhaseProblem

# Check for silent errors
$ rg 'err != nil.*_ = err' internal/gurgeh/arbiter/
111:    if err != nil {
112:        _ = err
```

### After (Issues Resolved)

```bash
# Orphaned code removed
$ ls internal/gurgeh/confidence/
ls: cannot access 'internal/gurgeh/confidence/': No such file or directory

# Phase constants used
$ rg 'sections\[int\(Phase' internal/gurgeh/arbiter/consistency/
52:    problem := sections[int(PhaseProblem)]
53:    features := sections[int(PhaseFeaturesGoals)]

# Errors surfaced
$ rg 'fmt.Fprintf.*warning:' internal/gurgeh/arbiter/
orchestrator.go:96:        fmt.Fprintf(os.Stderr, "warning: Intermute spec creation failed: %v\n", err)
orchestrator.go:640:        fmt.Fprintf(os.Stderr, "warning: phase research failed for %s: %v\n", state.Phase, err)
orchestrator.go:667:        fmt.Fprintf(os.Stderr, "warning: quick scan failed: %v\n", err)

# Focus routing implemented
$ rg 'switch v.shell.Focus' internal/tui/views/sprint_view.go
199:    switch v.shell.Focus() {
```

### Test Coverage

Commit includes new tests (added in subsequent commit dfc2cc7):
- Phase transition tests
- Blocker gating tests
- Confidence scoring tests

## References

- **Commit:** f406a0b
- **Oracle Review:** `docs/oracle-architecture-review-2026-02-01.md`
- **Related ADR:** `docs/decisions/2026-02-01-spec-vs-prd-canonical-type.md`
- **Test Commit:** dfc2cc7 (adds arbiter tests)
- **Integration Commit:** 09ac111 (wires Pollard/Intermute research)
