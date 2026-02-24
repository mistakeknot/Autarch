# Phase 2b: Merge App and UnifiedApp — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the duplicate `App` type so `--skip-onboard` uses `UnifiedApp` (the full-featured path), then deprecate the flag. One TUI implementation for all modes.

**Architecture:** `UnifiedApp` is the survivor — it has slash commands, chat settings, double-ctrl-c, agent detection, and the onboarding state machine that `App` lacks. When `--skip-onboard` is used, `UnifiedApp.Init()` will call `enterDashboard()` immediately instead of starting onboarding. `App`, `Run()`, and `RunWithOpts()` are deleted. Tests for `App` are migrated to test the equivalent `UnifiedApp` behavior.

**Tech Stack:** Go, Bubble Tea v1, Cobra (CLI flags)

**Baseline:** All `./internal/tui/...` tests pass. `go build ./cmd/...` succeeds.

---

## Task 1: Add skipOnboarding flag to UnifiedApp

Make `UnifiedApp` support skipping onboarding by going directly to dashboard mode in `Init()`. This is the core behavior change that enables the merge.

**Files:**
- Modify: `internal/tui/unified_app.go` — add `skipOnboarding` field and setter, modify `Init()`

**Step 1: Write the failing test**

Add to `internal/tui/unified_app_test.go`:

```go
func TestUnifiedAppSkipOnboardingGoesToDashboard(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.SetSkipOnboarding(true)
	app.SetViewFactories(
		func() View { return &noopDashboardView{name: "Kickoff"} },
		nil, nil, nil, nil,
		func(c *autarch.Client) []View {
			return []View{
				&noopDashboardView{name: "Bigend"},
				&noopDashboardView{name: "Gurgeh"},
			}
		},
	)

	app.Init()

	if app.mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard, got %v", app.mode)
	}
	if len(app.dashViews) != 2 {
		t.Fatalf("expected 2 dashboard views, got %d", len(app.dashViews))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestUnifiedAppSkipOnboardingGoesToDashboard -v`
Expected: FAIL — `SetSkipOnboarding` undefined.

**Step 3: Implement**

Add a `skipOnboarding` field to `UnifiedApp` struct (after the `initialTab` field):

```go
	// Skip onboarding and go directly to dashboard
	skipOnboarding bool
```

Add the setter method (after `SetInitialTab`):

```go
// SetSkipOnboarding skips onboarding and enters dashboard mode immediately.
func (a *UnifiedApp) SetSkipOnboarding(skip bool) {
	a.skipOnboarding = skip
}
```

Modify `Init()` — after `a.initPaletteCommands()` (line ~310), add a check before the kickoff view creation:

```go
	// Skip onboarding if requested — go directly to dashboard
	if a.skipOnboarding {
		return a.enterDashboard()
	}
```

This goes right before the existing `if a.createKickoffView != nil {` block.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestUnifiedAppSkipOnboardingGoesToDashboard -v`
Expected: PASS.

**Step 5: Run full suite**

Run: `go build ./internal/tui/... && go test ./internal/tui/...`
Expected: All tests pass.

**Step 6: Commit**

```bash
git add internal/tui/unified_app.go internal/tui/unified_app_test.go
git commit -m "feat(tui): add SetSkipOnboarding to UnifiedApp

When enabled, Init() calls enterDashboard() immediately instead of
starting the onboarding flow. This enables --skip-onboard to use
UnifiedApp instead of the separate App type.

Part of Phase 2b App/UnifiedApp merge (Autarch-ant)."
```

---

## Task 2: Rewire --skip-onboard to use UnifiedApp

Change `cmd/autarch/main.go` so `--skip-onboard` creates a `UnifiedApp` with `SetSkipOnboarding(true)` instead of creating an `App` via `RunWithOpts`. This eliminates the only production caller of `App`.

**Files:**
- Modify: `cmd/autarch/main.go` — replace the `skipOnboard` branch

**Step 1: Write the test (manual verification)**

There's no unit test for the CLI wiring — this is integration-level. We verify by building and running:

```bash
go build -o /tmp/autarch ./cmd/autarch
/tmp/autarch tui --skip-onboard --help 2>&1  # Should still show help
```

The structural correctness is verified by compilation (type-checks the wiring).

**Step 2: Replace the skipOnboard branch**

In `cmd/autarch/main.go`, replace the entire `if skipOnboard {` block (lines 137-177) with:

```go
			// Create unified app
			app := tui.NewUnifiedApp(client)

			if skipOnboard {
				app.SetSkipOnboarding(true)
				// Print deprecation warning
				fmt.Fprintln(os.Stderr, "Warning: --skip-onboard is deprecated. Use --tool=gurgeh or omit the flag.")
			}
```

Then move the view factories setup (lines 183-284) to be BEFORE this block — it needs to run for both skip and normal paths. Actually, the view factories are already set up after the `if skipOnboard` block, so we just need to move the `if skipOnboard` logic into the existing flow.

More precisely, the new structure is:

```go
			// Create unified app with onboarding flow
			app := tui.NewUnifiedApp(client)

			if skipOnboard {
				app.SetSkipOnboarding(true)
				fmt.Fprintln(os.Stderr, "Warning: --skip-onboard is deprecated. Use --tool=gurgeh or omit the flag.")
			}

			// Set up view factories for state transitions
			app.SetViewFactories(
				// ... (existing factory code, unchanged)
			)

			// Wire unified sprint view
			// ... (existing sprint factory code, unchanged)

			return tui.RunUnifiedWithOpts(client, app, tui.RunOpts{InlineMode: inlineMode, InitialTool: toolFlag})
```

This removes the entire `RunWithOpts` call path and the duplicate view creation for `--skip-onboard`.

**Step 3: Build and verify**

Run: `go build ./cmd/autarch`
Expected: Compiles successfully.

**Step 4: Commit**

```bash
git add cmd/autarch/main.go
git commit -m "refactor(cli): rewire --skip-onboard to use UnifiedApp

--skip-onboard now uses UnifiedApp with SetSkipOnboarding(true) instead
of the separate App type. Adds deprecation warning to stderr.

Part of Phase 2b App/UnifiedApp merge (Autarch-ant)."
```

---

## Task 3: Delete App type and related code

With no production callers remaining, delete `App`, `Run()`, `RunWithOpts()`, and `RunUnified()` (the zero-opts wrapper). Keep `RunUnifiedWithOpts()` as the sole entry point (rename it to `Run()` for simplicity).

**Files:**
- Delete: `internal/tui/app.go` — entire file
- Delete: `internal/tui/app_test.go` — entire file
- Modify: `internal/tui/unified_app.go` — delete `RunUnified()` wrapper, rename `RunUnifiedWithOpts` → `Run`
- Modify: `cmd/autarch/main.go` — update call from `RunUnifiedWithOpts` → `Run`

**Step 1: Verify no remaining references to App or RunWithOpts**

Run:
```bash
grep -rn 'tui\.Run\b\|tui\.RunWithOpts\|NewApp(' cmd/ internal/ pkg/ --include='*.go' | grep -v '_test.go' | grep -v 'app.go'
```
Expected: No matches (all callers switched in Task 2).

Also verify `RunUnified` (the zero-opts wrapper) has no callers:
```bash
grep -rn 'RunUnified(' cmd/ internal/ pkg/ --include='*.go' | grep -v '_test.go' | grep -v 'unified_app.go'
```
Expected: No matches.

**Step 2: Delete app.go**

Delete the entire file `internal/tui/app.go`.

**Step 3: Delete app_test.go**

Delete `internal/tui/app_test.go`. The two tests it contains (`TestAppHelpOverlayToggles`, `TestAppCtrlCQuitsWithPaletteVisible`) test `App`-specific behavior. The equivalent behaviors on `UnifiedApp` are already tested:
- Help overlay: `TestUnifiedAppDoubleCtrlCQuitsWithHelpVisible` tests the help visible path
- Ctrl+C quit: `TestUnifiedAppDoubleCtrlCQuitsWithHelpVisible` tests double-ctrl-c

Note: `noopView` (defined in `app_test.go`) is NOT used in `unified_app_test.go` — that file has its own `noopDashboardView`. Verify before deleting:
```bash
grep -n 'noopView\b' internal/tui/unified_app_test.go
```
Expected: No matches.

**Step 4: Rename RunUnifiedWithOpts → Run, delete RunUnified wrapper**

In `unified_app.go`:

1. Delete the `RunUnified` function (the 2-line wrapper)
2. Rename `RunUnifiedWithOpts` to `Run`

The function signature changes from:
```go
func RunUnifiedWithOpts(client *autarch.Client, app *UnifiedApp, opts RunOpts) error {
```
to:
```go
func Run(client *autarch.Client, app *UnifiedApp, opts RunOpts) error {
```

**Step 5: Update caller in main.go**

In `cmd/autarch/main.go`, change:
```go
return tui.RunUnifiedWithOpts(client, app, tui.RunOpts{...})
```
to:
```go
return tui.Run(client, app, tui.RunOpts{...})
```

**Step 6: Update test reference**

In `unified_app_test.go`, `TestRunUnifiedEnablesMouse` references the old name in its comment. Update the test name and any internal references:
```go
func TestRunEnablesMouse(t *testing.T) {
```

**Step 7: Verify ErrorView and EmptyView are preserved**

`app.go` contains `ErrorView()` and `EmptyView()` helper functions. Check if they're used:
```bash
grep -rn 'ErrorView\|EmptyView' internal/ cmd/ pkg/ --include='*.go' | grep -v 'app.go'
```

If they have callers, move them to a shared location (e.g., `helpers.go` or `unified_app.go`) before deleting `app.go`. If unused, delete them with `app.go`.

**Step 8: Build and test**

Run: `go build ./cmd/... && go test ./internal/tui/...`
Expected: All tests pass. Build succeeds.

**Step 9: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go internal/tui/unified_app.go internal/tui/unified_app_test.go cmd/autarch/main.go
git commit -m "refactor(tui): delete App type, unify on UnifiedApp

Delete App, Run(), RunWithOpts(), RunUnified(). Rename
RunUnifiedWithOpts → Run. UnifiedApp is now the sole TUI entry point.

Part of Phase 2b App/UnifiedApp merge (Autarch-ant)."
```

---

## Task 4: Move RunOpts to its own file (optional cleanup)

`RunOpts` was defined in `app.go` (now deleted). Verify it still has a home after the deletion — it may already be in `unified_app.go` or need to be extracted. Also check that the `switchTabMsg` type (previously in `app.go`) is either duplicated in `unified_app.go` or dead.

**Files:**
- Possibly modify: `internal/tui/unified_app.go` — verify `RunOpts` and `switchTabMsg` are defined

**Step 1: Check where RunOpts is defined after deletion**

Run:
```bash
grep -rn 'type RunOpts' internal/tui/ --include='*.go'
```

If only in `app.go` (now deleted), it needs to be moved. `RunOpts` is used by the renamed `Run()` function, so it MUST exist somewhere.

Also check `switchTabMsg`:
```bash
grep -rn 'switchTabMsg' internal/tui/ --include='*.go'
```

If `switchTabMsg` was only in `app.go` and is also used in `unified_app.go`, move it.

**Step 2: Move types if needed**

If `RunOpts` only exists in `app.go`, add it to `unified_app.go` before the `Run` function:

```go
// RunOpts configures TUI execution options.
type RunOpts struct {
	InlineMode  bool   // Inline mode preserves scrollback and shows log pane
	InitialTool string // Jump directly to this tool tab
}
```

If `switchTabMsg` is only in `app.go` but referenced in `unified_app.go`, move it similarly. If it's NOT referenced in `unified_app.go` (the unified app uses `switchToTab` directly), don't add it.

**Step 3: Build and test**

Run: `go build ./cmd/... && go test ./internal/tui/...`
Expected: All tests pass.

**Step 4: Commit (only if changes were needed)**

```bash
git add internal/tui/unified_app.go
git commit -m "refactor(tui): move RunOpts after App deletion

RunOpts was defined in the now-deleted app.go. Moved to unified_app.go.

Part of Phase 2b App/UnifiedApp merge (Autarch-ant)."
```

---

## Summary

| Task | Files | Risk | Description |
|------|-------|------|-------------|
| 1. Add skipOnboarding | unified_app.go, unified_app_test.go | Low | New field + 3-line Init() check |
| 2. Rewire CLI | cmd/autarch/main.go | Medium | Removes App creation path, adds deprecation |
| 3. Delete App | app.go, app_test.go, unified_app.go, main.go | Medium | Core deletion + rename |
| 4. Move RunOpts | unified_app.go (maybe) | Low | Type relocation if needed |

**Total: ~460 lines deleted (app.go), ~50 lines of test deleted (app_test.go), ~30 lines added/modified. 3-4 commits.**

Tasks 1-2 are independently shippable. Task 3 depends on Task 2. Task 4 depends on Task 3 (cleanup after deletion). Tasks 1 and 2 could run in parallel if they didn't share main.go, but sequencing is safer since Task 2 depends on Task 1's `SetSkipOnboarding` method.
