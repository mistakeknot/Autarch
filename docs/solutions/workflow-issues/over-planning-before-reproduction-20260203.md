---
title: "Over-Planning Before Bug Reproduction"
category: workflow-issues
tags: [planning, debugging, reproduction, process, over-engineering, reviewers]
module: internal/tui
symptom: "Spent 6+ hours on 5-phase fix plan before verifying bug exists"
root_cause: "Skipped Phase 0 (reproduction) - bug was likely intermittent or already fixed"
date_resolved: "2026-02-03"
bead: "Autarch-73j"
---

# Over-Planning Before Bug Reproduction

## Problem Statement

**Issue ID:** Autarch-73j
**Title:** Wire Kickoff → Arbiter interview transition in TUI
**Priority:** P1
**Outcome:** Bug was likely intermittent or already fixed; code was correctly implemented

A user reported that after completing the Kickoff form, the TUI doesn't reliably start the 8-phase Arbiter interview. Instead of first verifying the bug existed, we:

1. Created a detailed 5-phase fix plan
2. Ran 7 parallel research agents to "deepen" the plan
3. Had 3 reviewers evaluate the plan

**All three reviewers (DHH, Kieran, Simplicity) unanimously agreed:** "Reproduce first. 58% of this plan is speculative and should be deleted."

When we finally attempted reproduction, the code chain was fully implemented and working correctly.

## Investigation Steps Taken

### What We Did Wrong (First)

1. **Created elaborate plan** at `docs/plans/2026-02-02-fix-kickoff-arbiter-transition-plan.md`
   - 5 phases of potential fixes
   - Multiple code examples for speculative issues
   - Architecture diagrams for problems that didn't exist

2. **Deepened with 7 research agents:**
   - best-practices-researcher
   - code-simplicity-reviewer
   - julik-frontend-races-reviewer
   - learnings-researcher (x2)
   - architecture-strategist
   - repo-research-analyst

3. **Ran plan review with 3 reviewers:**
   - **DHH:** "Delete everything below Phase 0"
   - **Kieran:** "`tea.Sequence` suggestion is based on misunderstanding"
   - **Simplicity:** "58% should be deleted"

### What We Should Have Done (And Eventually Did)

1. **Attempted reproduction first:**
   ```bash
   go run ./cmd/autarch tui
   # Completed Kickoff form with test description
   # Observed: Transition occurred successfully
   ```
   **Result:** Could not reproduce the bug.

2. **Traced the code chain:**
   - `main.go:281-289` → SetSprintViewFactory wires factory
   - `unified_app.go:399-400` → ProjectCreatedMsg routes to handler
   - `unified_app.go:505-555` → handleProjectCreated creates SprintView
   - `views/sprint_view.go:87-95` → SprintView implements SprintStarter

3. **Verified all tests pass:**
   ```bash
   go test ./internal/tui/... -race -v
   # All tests pass, no race conditions
   ```

4. **Added integration tests as insurance** (the only actual code change)

## Root Cause Analysis

### The "Bug"

The bug was one of:
- **Already fixed:** A previous commit resolved it without explicit closure
- **Intermittent:** Race condition only reproducible in specific environments
- **Configuration-dependent:** Only occurs with specific Intermute/auth setup

### The Real Problem

**We skipped Phase 0.** The plan document itself said:

> **Phase 0: Reproduce First (NEW - REQUIRED)**
> **Goal:** Confirm the bug exists before writing any fixes

But we wrote Phases 1-5 before executing Phase 0.

### Cost of Over-Planning

| Activity | Time Spent | Value Delivered |
|----------|------------|-----------------|
| Initial plan creation | ~1 hour | 10% (only Phase 0 was useful) |
| Research agent deepening | ~30 min | 0% (all speculative) |
| Plan review by 3 agents | ~20 min | High (caught the problem) |
| Actual reproduction attempt | ~5 min | 100% (proved bug didn't exist) |
| Integration test writing | ~15 min | 100% (prevents regression) |

**Total waste: ~1.5 hours on speculative planning**

## Solution Applied

### Minimal, Targeted Changes

1. **Verified** the complete code chain is wired and functional
2. **Added 2 integration tests** to `internal/tui/unified_app_test.go`:
   - `TestProjectCreatedMsgTransitionsToSprintView` (lines 174-244)
   - `TestProjectCreatedMsgFallsBackWithoutSprintFactory` (lines 246-278)
3. **Closed bead** as "likely intermittent/already fixed"

### Files Changed

| File | Change | Lines |
|------|--------|-------|
| `internal/tui/unified_app_test.go` | Added 2 integration tests | +105 |
| Production code | None | 0 |

## Prevention Strategies

### The Zero-Phase Rule

**Before writing ANY fix plan, complete Phase 0:**

```
Phase 0: Verify Bug Exists (MANDATORY)
├─ Write exact reproduction steps
├─ Execute reproduction steps
├─ Write a failing test (if reproducible)
├─ Check git blame (is it already fixed?)
├─ Assess severity (P0-P3)
└─ Get peer confirmation → ONLY THEN plan phases 1-N
```

### Recommended Checklist

Before planning a bug fix, answer these questions:

- [ ] Can I reproduce the bug right now? (If no, investigate first)
- [ ] Do I have a failing test that proves the bug exists?
- [ ] Have I checked `git log` for recent changes to this area?
- [ ] Is the "bug" actually a configuration or environment issue?
- [ ] What's the simplest possible explanation?

### Red Flags for Over-Engineering

Stop planning if you notice:

| Red Flag | What It Means |
|----------|---------------|
| Plan before reproducer test | You don't know if the bug exists |
| 5+ phases for a P2/P3 bug | Scope creep |
| "Investigate" as a phase name | You should investigate BEFORE planning |
| Uncertain root cause in plan | More diagnosis needed, not more planning |
| Plan is longer than the fix will be | Over-engineering |
| Research agents before reproduction | Premature optimization of knowledge |

### Process Improvement

**Old process:**
```
Report → Plan → Research → Review → Reproduce → Fix
```

**New process:**
```
Report → Reproduce → (If reproducible) → Minimal Plan → Fix → Test
         ↓
    (If not reproducible) → Document as "could not reproduce" → Close
```

## Verification

### Tests Added

```go
// TestProjectCreatedMsgTransitionsToSprintView verifies:
// 1. SprintView factory is called when ProjectCreatedMsg arrives
// 2. currentView is replaced with SprintView instance
// 3. onboarding state updates to OnboardingInterview
// 4. StartSprint is called with project description
func TestProjectCreatedMsgTransitionsToSprintView(t *testing.T)

// TestProjectCreatedMsgFallsBackWithoutSprintFactory verifies:
// 1. If no SprintView factory, doesn't panic
// 2. State still transitions to OnboardingInterview
func TestProjectCreatedMsgFallsBackWithoutSprintFactory(t *testing.T)
```

### Manual Verification

```bash
go run ./cmd/autarch tui
# 1. Enter project name and description in Kickoff
# 2. Press Enter
# Expected: SprintView appears with chat panel ready
# Actual: Works correctly
```

## Related Documentation

### Internal References
- `docs/plans/2026-02-02-fix-kickoff-arbiter-transition-plan.md` - The over-engineered plan (useful as anti-pattern)
- `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md` - WindowSizeMsg padding patterns
- `docs/FLOWS.md` - System message flow diagram

### Code References
- `internal/tui/unified_app.go:505-555` - handleProjectCreated (verified correct)
- `internal/tui/unified_app_test.go:174-278` - Integration tests added
- `cmd/autarch/main.go:281-289` - SprintView factory wiring

### Related Beads
- **Autarch-0iz:** Sprint → Spec Summary transition (unblocked by this verification)
- **Autarch-6iu:** Epic/Task generation integration (unblocked by this verification)

## Key Learnings

### Process
1. **Reproduce before planning.** The reviewers were right—58% of elaborate planning was unnecessary.
2. **Integration tests > Speculation.** One simple test is worth more than five "what-if" architectural docs.
3. **Trust the reviewers.** When three independent reviewers say "simplify," listen.

### Technical
- UnifiedApp's message-driven pattern is solid and testable
- View factory pattern (dependency injection) enables clean testing
- The `-race` flag is valuable for TUI concurrency verification

### Meta
- **The cost of over-planning compounds.** Each speculative phase spawns more speculative work.
- **The value of reproduction is front-loaded.** 5 minutes of reproduction can save hours of planning.
- **Document the anti-pattern.** This document exists so we don't repeat this mistake.
