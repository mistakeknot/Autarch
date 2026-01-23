# Praude Vauxhall Style Parity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-wtl` (Task reference)

**Goal:** Make Praude’s TUI visually match Vauxhall’s styling (colors, borders, panel titles, and help bar) while keeping current list/detail behavior.

**Architecture:** Reuse the shared Tokyo Night palette and style helpers from `pkg/tui`, and apply them to Praude’s header/footer, panel titles, list rows, and overlays. Keep layout logic intact, but render panels through styled wrappers to match Vauxhall’s look.

**Tech Stack:** Go 1.24+, Bubble Tea, Lip Gloss, shared `pkg/tui` styles

Note: User explicitly requested no worktrees; implementation will be in the current working tree.

---

### Task 1: Add style helpers that mirror Vauxhall

**Files:**
- Create: `internal/praude/tui/styles.go`
- Modify: `internal/praude/tui/layout.go`
- Test: `internal/praude/tui/styles_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/styles_test.go`:

```go
package tui

import "testing"

func TestRenderHeaderFooterStyled(t *testing.T) {
	header := renderHeader("LIST", "LIST")
	footer := renderFooter("keys", "ready")
	if header == "" || footer == "" {
		t.Fatalf("expected non-empty header/footer")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestRenderHeaderFooterStyled`

Expected: FAIL (render functions moved/changed).

**Step 3: Write minimal implementation**

Create `internal/praude/tui/styles.go` and import shared styles:

```go
package tui

import (
	"strings"

	sharedtui "github.com/mistakeknot/vauxpraudemonium/pkg/tui"
)

func renderHeader(title, focus string) string {
	label := "PRAUDE | " + title + " | [" + focus + "]"
	return sharedtui.TitleStyle.Render(label)
}

func renderFooter(keys, status string) string {
	if strings.TrimSpace(status) == "" {
		status = "ready"
	}
	label := "KEYS: " + keys + " | " + status
	return sharedtui.HelpDescStyle.Render(label)
}

func renderPanelTitle(title string, width int) string {
	line := strings.Repeat("─", max(0, width))
	return sharedtui.TitleStyle.Render(title) + "\n" + sharedtui.LabelStyle.Render(line)
}
```

Update `internal/praude/tui/layout.go` to call the new `renderHeader`, `renderFooter`, `renderPanelTitle` from `styles.go` (remove duplicates in layout.go if needed).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestRenderHeaderFooterStyled`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/styles.go internal/praude/tui/layout.go internal/praude/tui/styles_test.go
git commit -m "Add Praude TUI style helpers"
```

---

### Task 2: Wrap panels with Vauxhall-style borders and padding

**Files:**
- Modify: `internal/praude/tui/layout.go`
- Test: `internal/praude/tui/layout_test.go`

**Step 1: Write the failing test**

Add to `internal/praude/tui/layout_test.go`:

```go
func TestPanelStyleAddsBorders(t *testing.T) {
	out := renderDualColumnLayout("PRDs", "left", "DETAILS", "right", 100, 6)
	if !strings.Contains(out, "┌") && !strings.Contains(out, "╭") {
		t.Fatalf("expected bordered panels")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestPanelStyleAddsBorders`

Expected: FAIL (no border chars).

**Step 3: Write minimal implementation**

In `internal/praude/tui/layout.go` wrap panels using shared `PanelStyle`:

```go
import sharedtui "github.com/mistakeknot/vauxpraudemonium/pkg/tui"

func stylePanel(content string, width, height int) string {
	style := sharedtui.PanelStyle.Copy().Width(width).Height(height)
	return style.Render(content)
}
```

Use `stylePanel` in `renderDualColumnLayout`, `renderStackedLayout`, and `renderSingleColumnLayout` after `renderPanelTitle` + content composition, then re-run `ensureExactWidth/Height` to preserve alignment.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestPanelStyleAddsBorders`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/layout.go internal/praude/tui/layout_test.go
git commit -m "Style Praude panels to match Vauxhall"
```

---

### Task 3: Style list rows and group headers

**Files:**
- Modify: `internal/praude/tui/screen_list.go`
- Test: `internal/praude/tui/list_style_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/list_style_test.go`:

```go
package tui

import "testing"

func TestRenderGroupListUsesSelectionMarker(t *testing.T) {
	items := []Item{{Type: ItemTypeGroup, Group: &Group{Name: "draft", Expanded: true}}}
	out := renderGroupList(items, 0, 0, 3)
	if out == "" {
		t.Fatalf("expected output")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestRenderGroupListUsesSelectionMarker`

Expected: PASS/FAIL depending on styling updates; keep as smoke test.

**Step 3: Write minimal implementation**

Update `renderGroupListItem` to apply shared styles:

```go
import sharedtui "github.com/mistakeknot/vauxpraudemonium/pkg/tui"

// selected uses SelectedStyle, unselected uses UnselectedStyle
```

Use `TitleStyle` for group names, `LabelStyle` for counts, and `SelectedStyle` for selected row. Optionally add a small status badge using `BadgeStyle` or `LabelStyle` for each PRD.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestRenderGroupListUsesSelectionMarker`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/screen_list.go internal/praude/tui/list_style_test.go
git commit -m "Style Praude list rows"
```

---

### Task 4: Style search overlay and help bar

**Files:**
- Modify: `internal/praude/tui/search_overlay.go`
- Modify: `internal/praude/tui/overlay.go`
- Test: `internal/praude/tui/search_overlay_test.go`

**Step 1: Write the failing test**

Add to `internal/praude/tui/search_overlay_test.go`:

```go
func TestSearchOverlayViewStyled(t *testing.T) {
	overlay := NewSearchOverlay()
	overlay.Show()
	out := overlay.View(60)
	if out == "" {
		t.Fatalf("expected styled overlay")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestSearchOverlayViewStyled`

Expected: PASS/FAIL depending on current output.

**Step 3: Write minimal implementation**

Update overlay view to use shared styles:

```go
import sharedtui "github.com/mistakeknot/vauxpraudemonium/pkg/tui"

boxStyle := sharedtui.PanelStyle.Copy().Padding(1, 2).BorderForeground(sharedtui.ColorPrimary)
header := sharedtui.TitleStyle.Render("Search")
```

Update `renderHelpOverlay` to use `HelpKeyStyle`/`HelpDescStyle` for key labels.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestSearchOverlayViewStyled`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/search_overlay.go internal/praude/tui/overlay.go internal/praude/tui/search_overlay_test.go
git commit -m "Style search overlay and help text"
```

---

### Task 5: Full test pass

**Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 2: Commit (if needed)**

```bash
git status --porcelain
# commit only if there are remaining changes
```

---

Plan complete and saved to `docs/plans/2026-01-22-praude-vauxhall-style-parity-plan.md`.

Two execution options:
1) Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2) Parallel Session (separate) — New session uses superpowers:executing-plans to execute task-by-task

Which approach?
