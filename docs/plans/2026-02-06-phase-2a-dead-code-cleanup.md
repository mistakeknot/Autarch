# Phase 2a: Dead Code Cleanup — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Remove dead code and fix a dropped-cmd bug in the TUI layer, preparing a clean foundation for Phase 2b (App/UnifiedApp merge) and 2c (onboarding relocation). Zero behavior change — only code that is provably unused or broken is touched.

**Architecture:** This is a pure cleanup pass. We delete `OnboardingOrchestrator` (unused type), remove deprecated wrappers, replace hand-rolled string helpers with `strings.*`, and fix `sendToCurrentView` which silently discards `tea.Cmd` values. Every change is independently committable and testable.

**Tech Stack:** Go, Bubble Tea v1, lipgloss

**Baseline:** All `./internal/tui/...` tests pass as of this writing (`go test ./internal/tui/...`).

---

## Task 1: Delete OnboardingOrchestrator

The `OnboardingOrchestrator` type in `onboarding.go:111-247` is completely unused by `UnifiedApp`. The `UnifiedApp` manages onboarding directly via its own state machine. The only reference to `OnboardingOrchestrator` outside its definition is in `onboarding_test.go`, which tests the orchestrator in isolation — also dead.

**Files:**
- Modify: `internal/tui/onboarding.go` — delete lines 111-247 (the type, constructor, and all methods)
- Delete: `internal/tui/onboarding_test.go` — the entire file (only tests `OnboardingOrchestrator`)

**Step 1: Verify no other references exist**

Run:
```bash
grep -r "OnboardingOrchestrator\|NewOnboardingOrchestrator" internal/ cmd/ pkg/ --include='*.go' | grep -v '_test.go' | grep -v 'onboarding.go'
```
Expected: No output (zero references outside the definition and its test).

**Step 2: Delete OnboardingOrchestrator from onboarding.go**

Remove `onboarding.go:111-247` — everything from the `// OnboardingOrchestrator manages...` comment through the `Cancel()` method. Keep lines 1-109 (the `OnboardingState` type, constants, `AllOnboardingStates()`, `ID()`, `Label()`, `InterviewStep`, and `InterviewSteps()`) — these are actively used.

After deletion, also remove the now-unused imports. The following imports are only used by `OnboardingOrchestrator`:
- `"context"` (used in `OnboardingOrchestrator.ctx/cancel`)
- `"github.com/charmbracelet/bubbles/key"` (used in `Update()` key matching)
- `"github.com/charmbracelet/lipgloss"` (used in `View()` fallback)
- `pkgtui "github.com/mistakeknot/autarch/pkg/tui"` (used for `CommonKeys` and `ColorMuted`)

Verify which imports remain needed by the surviving code (lines 1-109). The surviving code uses only the `OnboardingState` type with no external dependencies, so remove all imports. The file becomes import-free.

Wait — `tea` is not imported at the top level in the surviving code either. The `OnboardingState` type and `InterviewStep` are pure Go types. Confirm by checking: the surviving functions (`AllOnboardingStates`, `ID`, `Label`, `InterviewSteps`) use no external packages.

**Step 3: Delete onboarding_test.go**

Delete the entire file `internal/tui/onboarding_test.go`. Its only test (`TestOnboardingQuitKeyCancels`) tests the deleted `OnboardingOrchestrator`.

**Step 4: Run tests to verify nothing breaks**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass. Build succeeds.

**Step 5: Commit**

```bash
git add internal/tui/onboarding.go internal/tui/onboarding_test.go
git commit -m "refactor(tui): delete unused OnboardingOrchestrator

OnboardingOrchestrator was never wired into UnifiedApp — the unified app
manages onboarding state directly. Remove the type and its test.

Part of Phase 2a dead code cleanup (Autarch-ant)."
```

---

## Task 2: Delete unused onboardingHeader method

`UnifiedApp.onboardingHeader()` at `unified_app.go:1980-1995` is defined but never called. It was superseded by the breadcrumb + tab bar rendering. The only occurrence of `onboardingHeader` in the entire codebase is its definition.

**Files:**
- Modify: `internal/tui/unified_app.go` — delete lines 1980-1995

**Step 1: Verify no callers**

Run:
```bash
grep -r 'onboardingHeader' internal/ cmd/ pkg/ --include='*.go'
```
Expected: Only one match — the definition at `unified_app.go:1980`.

**Step 2: Delete the method**

Remove `unified_app.go:1980-1995` (the `func (a *UnifiedApp) onboardingHeader() string` method and its entire body).

**Step 3: Run tests**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "refactor(tui): delete unused onboardingHeader method

Superseded by breadcrumb + tab bar rendering. Never called.

Part of Phase 2a dead code cleanup (Autarch-ant)."
```

---

## Task 3: Delete deprecated renderFooter wrapper

`UnifiedApp.renderFooter()` at `unified_app.go:2012-2015` is a deprecated wrapper that just calls `renderFooterContent()`. It has zero callers in `UnifiedApp` — the `View()` method calls `renderFooterContent()` directly at line 1953. (Note: `App.renderFooter()` in `app.go:333` is a different method on a different type — do NOT touch it.)

**Files:**
- Modify: `internal/tui/unified_app.go` — delete lines 2012-2015

**Step 1: Verify no callers on UnifiedApp**

Run:
```bash
grep -n 'renderFooter()' internal/tui/unified_app.go
```
Expected: Only the definition at line 2013. No calls.

**Step 2: Delete the method**

Remove `unified_app.go:2012-2015` (the comment + one-line wrapper method).

**Step 3: Run tests**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "refactor(tui): delete deprecated renderFooter wrapper

renderFooterContent() is called directly. The wrapper had no callers.

Part of Phase 2a dead code cleanup (Autarch-ant)."
```

---

## Task 4: Replace hand-rolled string helpers with strings.*

`types.go:136-185` has four hand-rolled string functions (`splitLines`, `splitComma`, `containsComma`, `trimSpace`) with the comment "avoid importing strings package in views". This avoidance has no technical justification — `strings` is a stdlib package with zero cost. Replace with `strings.Split`, `strings.Contains`, and `strings.TrimSpace`.

All four helpers are only used in `SpecFromAnswers()` in `types.go:111-128`.

**Files:**
- Modify: `internal/tui/types.go` — replace helper calls in lines 111-128, delete lines 136-185, add `"strings"` import

**Step 1: Write a test for SpecFromAnswers to lock behavior**

Create `internal/tui/types_test.go`:

```go
package tui

import "testing"

func TestSpecFromAnswers(t *testing.T) {
	answers := map[string]string{
		"vision":       "Build a great app",
		"users":        "Developers",
		"problem":      "Too complex",
		"platform":     "Web",
		"language":     "Go",
		"requirements": "Fast startup\nLow memory\nGood UX",
	}

	spec := SpecFromAnswers("proj-1", answers)

	if spec.Name != "Build a great app" {
		t.Errorf("Name = %q, want %q", spec.Name, "Build a great app")
	}
	if len(spec.Requirements) != 3 {
		t.Fatalf("Requirements count = %d, want 3", len(spec.Requirements))
	}
	if spec.Requirements[0] != "Fast startup" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "Fast startup")
	}
}

func TestSpecFromAnswersCommaSeparated(t *testing.T) {
	answers := map[string]string{
		"vision":       "App",
		"requirements": "fast, reliable, secure",
	}

	spec := SpecFromAnswers("proj-2", answers)

	if len(spec.Requirements) != 3 {
		t.Fatalf("Requirements count = %d, want 3", len(spec.Requirements))
	}
	if spec.Requirements[0] != "fast" {
		t.Errorf("Requirements[0] = %q, want %q", spec.Requirements[0], "fast")
	}
	if spec.Requirements[2] != "secure" {
		t.Errorf("Requirements[2] = %q, want %q", spec.Requirements[2], "secure")
	}
}

func TestSpecFromAnswersEmpty(t *testing.T) {
	spec := SpecFromAnswers("proj-3", map[string]string{})

	if spec.ProjectID != "proj-3" {
		t.Errorf("ProjectID = %q, want %q", spec.ProjectID, "proj-3")
	}
	if len(spec.Requirements) != 0 {
		t.Errorf("Requirements count = %d, want 0", len(spec.Requirements))
	}
}
```

**Step 2: Run tests to verify they pass with current code**

Run: `go test ./internal/tui/ -run TestSpecFromAnswers -v`
Expected: 3 tests PASS (locking existing behavior).

**Step 3: Replace helpers with strings.* calls**

In `types.go`, replace the body of the requirements parsing (lines 112-129) with:

```go
	// Parse requirements
	if reqs := answers["requirements"]; reqs != "" {
		lines := strings.Split(reqs, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				spec.Requirements = append(spec.Requirements, line)
			}
		}
		// Also handle comma-separated
		if len(spec.Requirements) == 1 && strings.Contains(spec.Requirements[0], ",") {
			spec.Requirements = nil
			for _, req := range strings.Split(answers["requirements"], ",") {
				req = strings.TrimSpace(req)
				if req != "" {
					spec.Requirements = append(spec.Requirements, req)
				}
			}
		}
	}
```

Add `"strings"` to the import block. Delete the four helper functions (lines 135-185 approximately, from `// Helper functions...` through end of `trimSpace`).

**Step 4: Run tests to verify behavior is preserved**

Run: `go test ./internal/tui/ -run TestSpecFromAnswers -v`
Expected: All 3 tests PASS (identical behavior).

**Step 5: Run full test suite**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass.

**Step 6: Commit**

```bash
git add internal/tui/types.go internal/tui/types_test.go
git commit -m "refactor(tui): replace hand-rolled string helpers with strings.*

splitLines, splitComma, containsComma, trimSpace were reinventing
strings.Split, strings.Contains, strings.TrimSpace. Added tests for
SpecFromAnswers to lock behavior before replacement.

Part of Phase 2a dead code cleanup (Autarch-ant)."
```

---

## Task 5: Fix sendToCurrentView dropping tea.Cmd

`sendToCurrentView` at `unified_app.go:1280-1287` calls `a.currentView.Update(msg)` but discards the returned `tea.Cmd` with `_ = cmd`. This means any commands the view returns (timers, IO, focus requests) are silently lost. There are 3 call sites: lines 1246, 1254, and 1849.

The fix: return `tea.Cmd` from the function and batch the returned commands at call sites. However, all 3 call sites are inside goroutines or `handleAgentRun` which doesn't return `tea.Cmd` to the Bubble Tea runtime. The calls happen outside the normal `Update()` cycle, so we can't just return the cmd.

The correct fix is to use `tea.Program.Send()` to re-inject the cmd's resulting messages. But `sendToCurrentView` doesn't have access to `tea.Program`. A simpler approach: change the call sites to use the `Update()` path properly. However, that's a larger refactor.

For Phase 2a, the minimal safe fix is:
1. Change `sendToCurrentView` to return `tea.Cmd`
2. At each call site, if a cmd is returned, schedule it via the program (if available) or log a warning

Actually, looking more carefully — these calls are inside `handleAgentRun()` which runs in a goroutine. The proper pattern in Bubble Tea is to send messages via `p.Send()`, not to call `Update()` directly from goroutines. This is a deeper architectural issue.

**Revised scope for Phase 2a:** Document the bug with a `// BUG:` comment explaining the cmd drop, but defer the actual fix to Phase 2c where the goroutine architecture gets restructured. This avoids introducing new bugs in Phase 2a.

**Files:**
- Modify: `internal/tui/unified_app.go` — add BUG comment to `sendToCurrentView`

**Step 1: Add BUG comment**

Replace `sendToCurrentView` (lines 1280-1287) with:

```go
// BUG(phase2c): sendToCurrentView discards the tea.Cmd returned by Update().
// Any commands the view returns (timers, IO, focus requests) are silently lost.
// This is called from goroutines (handleAgentRun) which cannot return commands
// to the Bubble Tea runtime. Fix in Phase 2c by converting to p.Send() pattern.
func (a *UnifiedApp) sendToCurrentView(msg tea.Msg) {
	if a.currentView == nil {
		return
	}
	a.currentView, _ = a.currentView.Update(msg)
}
```

**Step 2: Run tests**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass (no behavior change).

**Step 3: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "docs(tui): document sendToCurrentView cmd-drop bug

sendToCurrentView discards tea.Cmd from Update() calls made inside
goroutines. Documented as BUG(phase2c) — proper fix requires switching
to p.Send() pattern during onboarding relocation.

Part of Phase 2a dead code cleanup (Autarch-ant)."
```

---

## Summary

| Task | Files | Lines changed | Risk |
|------|-------|--------------|------|
| 1. Delete OnboardingOrchestrator | onboarding.go, onboarding_test.go | -168 | None (zero references) |
| 2. Delete onboardingHeader | unified_app.go | -16 | None (zero callers) |
| 3. Delete renderFooter wrapper | unified_app.go | -4 | None (zero callers) |
| 4. Replace string helpers | types.go, types_test.go (new) | -50, +55 test | Low (locked by tests) |
| 5. Document cmd-drop bug | unified_app.go | +4 comment | None (no behavior change) |

**Total: ~238 lines removed, ~55 lines of test added, 5 independent commits.**

Each task is independently shippable and revertible. Tasks 1-3 are pure deletions. Task 4 is a refactor locked by new tests. Task 5 is documentation only.
