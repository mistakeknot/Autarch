# Vauxhall TUI Two-Pane Layout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-x7y` (Task reference)

**Goal:** Pin Projects on the left and filter Sessions/Agents by selected project in the TUI.

**Architecture:** Refactor the TUI into a two-pane layout with projects list on the left and the active view on the right. Introduce a selected-project filter that scopes sessions/agents and dashboard summaries. Add focus switching between panes.

**Tech Stack:** Go 1.24+, Bubble Tea + lipgloss.

---

### Task 1: Filtering Logic + Tests

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_layout_test.go`

**Step 1: Write the failing test for project filtering (sessions)**

```go
package tui

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
)

type fakeAgg struct { state aggregator.State }
func (f *fakeAgg) GetState() aggregator.State { return f.state }
func (f *fakeAgg) Refresh(context.Context) error { return nil }
func (f *fakeAgg) NewSession(string,string,string) error { return nil }
func (f *fakeAgg) RestartSession(string,string,string) error { return nil }
func (f *fakeAgg) RenameSession(string,string) error { return nil }
func (f *fakeAgg) ForkSession(string,string,string) error { return nil }
func (f *fakeAgg) AttachSession(string) error { return nil }
func (f *fakeAgg) StartMCP(context.Context,string,string) error { return nil }
func (f *fakeAgg) StopMCP(string,string) error { return nil }

func TestFilterSessionsByProject(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{Sessions: []aggregator.TmuxSession{
		{Name: "a", ProjectPath: "/p/one"},
		{Name: "b", ProjectPath: "/p/two"},
	}}}
	m := New(agg)
	m.projectsList.SetItems([]list.Item{ProjectItem{Path: "", Name: "All"}, ProjectItem{Path: "/p/one", Name: "one"}})
	m.projectsList.Select(1)
	m.updateLists()

	if len(m.sessionList.Items()) != 1 {
		t.Fatalf("expected 1 session, got %d", len(m.sessionList.Items()))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestFilterSessionsByProject -v`
Expected: FAIL because filtering isn’t implemented / projectsList doesn’t exist.

**Step 3: Implement minimal filtering in `updateLists()`**

- Add a pinned `projectsList` in the model.
- Add `selectedProjectPath()` helper.
- Filter sessions/agents when selected project is not empty.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestFilterSessionsByProject -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go
git commit -m "feat(vauxhall): filter TUI sessions by selected project"
```

---

### Task 2: Two-Pane Layout + Focus Switching

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_layout_test.go`

**Step 1: Write failing test for focus switching**

```go
func TestFocusSwitching(t *testing.T) {
	agg := &fakeAgg{}
	m := New(agg)
	m.activePane = PaneMain
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	updated := mm.(Model)
	if updated.activePane != PaneProjects {
		t.Fatalf("expected projects pane")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestFocusSwitching -v`
Expected: FAIL (no panes implemented).

**Step 3: Implement panes + key routing**

- Add `PaneProjects` and `PaneMain`.
- Route Up/Down and `/` to the focused list.
- Add `[` and `]` focus toggles.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestFocusSwitching -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go
git commit -m "feat(vauxhall): add TUI two-pane focus switching"
```

---

### Task 3: Right-Pane Tabs (Dashboard/Sessions/Agents)

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_layout_test.go`

**Step 1: Write failing test for tab count**

```go
func TestRightPaneTabs(t *testing.T) {
	m := New(&fakeAgg{})
	if m.maxTab != TabAgents {
		t.Fatalf("expected Dashboard/Sessions/Agents tabs only")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestRightPaneTabs -v`
Expected: FAIL.

**Step 3: Implement right-pane tabs only**

- Remove Projects tab; keep Dashboard, Sessions, Agents.
- Update tab labels + key mappings (1–3).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestRightPaneTabs -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go
git commit -m "feat(vauxhall): restrict right-pane tabs to dashboard/sessions/agents"
```

---

### Task 4: Render Two-Pane Layout

**Files:**
- Modify: `internal/vauxhall/tui/model.go`
- Test: `internal/vauxhall/tui/model_layout_test.go`

**Step 1: Write failing test for width clamp**

```go
func TestTwoPaneLayoutClamp(t *testing.T) {
	m := New(&fakeAgg{})
	m.width = 40
	_ = m.renderTwoPane("left", "right") // should not panic
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/vauxhall/tui -run TestTwoPaneLayoutClamp -v`
Expected: FAIL (helper missing).

**Step 3: Implement layout helper**

- Add `renderTwoPane(left, right string) string`.
- Compute left width as 30% with min/max and clamp.
- Fallback to single-column if width too small.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/vauxhall/tui -run TestTwoPaneLayoutClamp -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/vauxhall/tui/model.go internal/vauxhall/tui/model_layout_test.go
git commit -m "feat(vauxhall): render TUI two-pane layout"
```

---

### Task 5: Manual Verification Checklist

- Run `./dev vauxhall --tui` in a narrow terminal and confirm no panic.
- Verify projects pinned left and sessions/agents filter as expected.
- Confirm dashboard shows project-scoped summary.
- Validate focus switching with `[` and `]` and filtering with `/`.

---

Plan complete and saved to `docs/plans/2026-01-22-vauxhall-tui-two-pane-layout-implementation-plan.md`.

Two execution options:
1. Subagent-Driven (this session) - use @superpowers:subagent-driven-development
2. Parallel Session (separate) - new session uses @superpowers:executing-plans

Which approach?
