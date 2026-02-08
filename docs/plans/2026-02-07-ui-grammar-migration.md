# UI Grammar Migration Plan

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Goal:** Complete the CommonKeys + HelpOverlay migration for Gurgeh and Coldwine standalone TUIs. Bigend and UnifiedApp are already migrated.

**Architecture:** `pkg/tui/keys.go` defines `CommonKeys` (shared keybindings) and `pkg/tui/help.go` defines `HelpOverlay` (renders common bindings + tool-specific extras). Bigend and UnifiedApp already use these. Gurgeh and Coldwine standalone TUIs use `CommonKeys` for key handling but have hand-rolled `renderHelpOverlay()` functions instead of `pkgtui.HelpOverlay.Render()`.

**Tech Stack:** Go, Bubble Tea, lipgloss, `pkg/tui`

---

### Task 1: Migrate Gurgeh standalone TUI help overlay

**Files:**
- Modify: `internal/gurgeh/tui/overlay.go` — replace `renderHelpOverlay()` with `pkgtui.HelpOverlay`
- Modify: `internal/gurgeh/tui/model.go` — add `helpOverlay pkgtui.HelpOverlay` field, wire `Toggle()` and `Render()`

**Step 1: Update model.go**

The Gurgeh TUI model (at `internal/gurgeh/tui/model.go`) already has `helpOverlay pkgtui.HelpOverlay` (line 52) and `keys pkgtui.CommonKeys` (line 51). This means it already has the infrastructure — verify that `helpOverlay.Render(m.keys, extras, width)` is called in the View method instead of the hand-rolled `renderHelpOverlay()`.

If `renderHelpOverlay()` is still called in View, replace:
```go
// Before:
renderHelpOverlay()

// After:
m.helpOverlay.Render(m.keys, gurgehExtras(), m.width)
```

Create a `gurgehExtras()` function that returns `[]pkgtui.HelpBinding` for Gurgeh-specific keys:
- j/k move, enter toggle group, / search
- n new sprint, g sprint from PRD
- r research, p suggestions, s review
- a archive, d delete, u undo, h archived

**Step 2: Update overlay.go**

Remove or simplify `renderHelpOverlay()` if it's now unused. Keep `renderTutorialOverlay()` and `renderConfirmOverlay()` as they serve different purposes.

**Step 3: Verify**
```bash
go build ./internal/gurgeh/...
go test ./internal/gurgeh/tui/... -count=1
```

**Step 4: Commit**
```bash
git add internal/gurgeh/tui/overlay.go internal/gurgeh/tui/model.go
git commit -m "refactor(gurgeh): migrate help overlay to pkgtui.HelpOverlay"
```

---

### Task 2: Migrate Coldwine standalone TUI help overlay

**Files:**
- Modify: `internal/coldwine/tui/styles.go` — replace `renderHelpOverlay()` with `pkgtui.HelpOverlay`
- Modify: `internal/coldwine/tui/model.go` — verify `helpOverlay pkgtui.HelpOverlay` is used

**Step 1: Update model.go**

The Coldwine TUI model already has `helpOverlay pkgtui.HelpOverlay` (line 72) and `keys pkgtui.CommonKeys` (line 71). Verify that `helpOverlay.Render(m.keys, extras, width)` is called in the View method.

If `renderHelpOverlay()` is still called, replace with:
```go
m.helpOverlay.Render(m.keys, coldwineExtras(), m.width)
```

Create a `coldwineExtras()` function with Coldwine-specific keys:
- n new task, s start, x stop
- r review, R review view, c coord
- a/o/v/d filter, i init
- ctrl+k palette, , settings

**Step 2: Update styles.go**

Remove `renderHelpOverlay()` from styles.go if now unused.

**Step 3: Verify**
```bash
go build ./internal/coldwine/...
go test ./internal/coldwine/tui/... -count=1
```

**Step 4: Commit**
```bash
git add internal/coldwine/tui/styles.go internal/coldwine/tui/model.go
git commit -m "refactor(coldwine): migrate help overlay to pkgtui.HelpOverlay"
```

---

### Task 3: Update SHORTCUTS.md with canonical bindings

**Files:**
- Modify: `docs/tui/SHORTCUTS.md`

**Step 1:** Add a "Canonical CommonKeys Bindings" section listing all bindings from `pkg/tui/keys.go`:
- ctrl+c×2 quit, F1 help, ctrl+f search, esc back
- up/down navigate, home/end top/bottom, pgup/pgdn prev/next
- ctrl+r refresh, tab cycle panes, enter select

**Step 2:** Add a "Per-Tool Extras" section listing tool-specific bindings:
- Bigend: [/] focus, n/e/f/k/a/m/p
- Gurgeh: j/k/n/g/r/p/s/a/d/u/h
- Coldwine: n/s/x/r/R/c/a/o/v/d/i

**Step 3: Commit**
```bash
git add docs/tui/SHORTCUTS.md
git commit -m "docs: update SHORTCUTS.md with canonical CommonKeys bindings"
```
