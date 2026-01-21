# TUI Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `[none] (No task id provided)`

**Goal:** Make the TUI feel less barebones by improving empty state, task table styling, and detail pane framing.

**Architecture:** Extend `internal/tui/model.go` view rendering with new helper functions for empty-state blocks, table formatting, and detail framing. Use tests to lock in key strings and layout cues without hard-coding full screens.

**Tech Stack:** Go 1.22+, Bubble Tea, ANSI styling (existing helpers), Glamour for markdown.

---

### Task 1: Empty-state polish

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/empty_state_test.go`

**Step 1: Write the failing test**

```go
func TestEmptyStateShowsQuickStart(t *testing.T) {
    m := NewModel()
    m.TaskList = nil
    out := m.View()
    if !strings.Contains(out, "Quick start") {
        t.Fatalf("expected quick start block")
    }
    if !strings.Contains(out, "1) init") {
        t.Fatalf("expected init step")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestEmptyStateShowsQuickStart -v`
Expected: FAIL with "expected quick start block".

**Step 3: Write minimal implementation**

- Add helper `renderEmptyState()` returning a short block with numbered steps.
- Inject into left pane when `len(TaskList)==0`.
- Add placeholder metadata in right pane when no task selected (e.g., `ID: -`, `Status: -`).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestEmptyStateShowsQuickStart -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/empty_state_test.go
git commit -m "feat: add empty-state quick start"
```

---

### Task 2: Table/list styling

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/table_style_test.go`

**Step 1: Write the failing test**

```go
func TestTableHeaderIncludesColumns(t *testing.T) {
    m := NewModel()
    m.TaskList = []TaskItem{{ID: "T1", Title: "Alpha", Status: "todo"}}
    out := m.View()
    if !strings.Contains(out, "TYPE") || !strings.Contains(out, "PRI") {
        t.Fatalf("expected extended column headers")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestTableHeaderIncludesColumns -v`
Expected: FAIL with "expected extended column headers".

**Step 3: Write minimal implementation**

- Update left pane header row to include extra columns (e.g., `TYPE PRI ST ID TITLE AGE ASG`).
- Add `formatTaskRow` helper for fixed-width columns.
- Apply subtle dim styling to non-selected rows using existing `colorize` helper (e.g., gray for idle rows).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestTableHeaderIncludesColumns -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/table_style_test.go
git commit -m "feat: improve task table styling"
```

---

### Task 3: Detail pane framing

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/detail_frame_test.go`

**Step 1: Write the failing test**

```go
func TestDetailPaneShowsHeaderGrid(t *testing.T) {
    m := NewModel()
    out := m.View()
    if !strings.Contains(out, "ID:") || !strings.Contains(out, "Status:") {
        t.Fatalf("expected header grid")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestDetailPaneShowsHeaderGrid -v`
Expected: FAIL with "expected header grid".

**Step 3: Write minimal implementation**

- Add a small header grid to the right pane even when no task is selected, using placeholders (`-`).
- Add section titles for markdown (`Summary`, `Acceptance Criteria`, `Recent Activity`).
- Keep markdown rendering for the body when content exists; otherwise show placeholders.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestDetailPaneShowsHeaderGrid -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/detail_frame_test.go
git commit -m "feat: add detail pane framing"
```

