# Vauxhall Focused Pane + Default Projects Focus Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-rn9` (Task reference)

**Goal:** Make Vauxhall start focused on the Projects pane and visibly indicate focus with a border around the active pane.

**Architecture:** Add focused/unfocused pane border styles in the TUI styles, apply them in `renderTwoPane`, and adjust list sizes to account for border width/height. Default the initial active pane to Projects in `New`.

**Tech Stack:** Go, Bubble Tea, lipgloss

---

### Task 1: Default focus to Projects on launch

**Files:**
- Modify: `internal/vauxhall/tui/model_layout_test.go`
- Modify: `internal/vauxhall/tui/model.go`

**Step 1: Write the failing test**

Add to `internal/vauxhall/tui/model_layout_test.go`:
```go
func TestDefaultFocusIsProjects(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	if m.activePane != PaneProjects {
		t.Fatalf("expected default focus on projects")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestDefaultFocusIsProjects -v`
Expected: FAIL with "expected default focus on projects"

**Step 3: Write minimal implementation**

In `internal/vauxhall/tui/model.go`, set `activePane` to `PaneProjects` in `New`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestDefaultFocusIsProjects -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go
git commit -m "feat(vauxhall): default focus to projects pane"
```

---

### Task 2: Focused pane border styling

**Files:**
- Modify: `internal/vauxhall/tui/model_layout_test.go`
- Modify: `internal/vauxhall/tui/styles.go`
- Modify: `internal/vauxhall/tui/model.go`

**Step 1: Write failing tests**

Add to `internal/vauxhall/tui/model_layout_test.go`:
```go
func TestFocusedPaneBorderRendered(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m.activePane = PaneProjects
	view := m.renderTwoPane("left", "right")
	border := lipgloss.RoundedBorder()
	if !strings.Contains(view, border.TopLeft) {
		t.Fatalf("expected border to render")
	}
}

func TestFocusedPaneChangesRendering(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m.activePane = PaneProjects
	leftFocus := m.renderTwoPane("left", "right")
	m.activePane = PaneMain
	rightFocus := m.renderTwoPane("left", "right")
	if leftFocus == rightFocus {
		t.Fatalf("expected different rendering when focus changes")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/vauxhall/tui -run TestFocusedPane -v`
Expected: FAIL (no border / same output)

**Step 3: Write minimal implementation**

In `internal/vauxhall/tui/styles.go`, add styles:
```go
PaneFocusedStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorPrimary)

PaneUnfocusedStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorMuted)
```

In `internal/vauxhall/tui/model.go`:
- Update `renderTwoPane` to wrap left/right views with the focused/unfocused styles based on `activePane`.
- Keep single-pane mode unchanged.
- Adjust list sizes in the `tea.WindowSizeMsg` handler to account for border width/height when showing two panes:
  - `leftW-2`, `rightW-2`, and `h-2` (clamp to >= 1).

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/vauxhall/tui -run TestFocusedPane -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go internal/vauxhall/tui/styles.go
git commit -m "feat(vauxhall): highlight focused pane"
```

---

### Task 3: Full TUI test run

**Files:**
- Test: `internal/vauxhall/tui/...`

**Step 1: Run full tests**

Run: `go test ./internal/vauxhall/tui -v`
Expected: PASS

**Step 2: Commit plan completion note (optional)**

```bash
git status -sb
```
