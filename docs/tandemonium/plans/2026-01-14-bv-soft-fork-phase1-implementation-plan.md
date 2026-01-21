# Beads Viewer Soft-Fork Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `[none] (No task id provided)`

**Goal:** Deliver beads_viewer-style TUI parity: full-screen two-pane layout, markdown detail rendering, search + filters, and live reload.

**Architecture:** Extend the existing Bubble Tea model with window sizing, search/filter state, and markdown rendering. Add a small adapter layer for reusable UI helpers. Use a file watcher (specs + state.db) to trigger refresh events in addition to the existing timer.

**Tech Stack:** Go 1.22+, Bubble Tea, SQLite, YAML specs, fsnotify (watcher), glamour (markdown rendering).

---

### Task 1: Full-screen layout using WindowSizeMsg

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/fullscreen_test.go`

**Step 1: Write the failing test**

```go
func TestViewUsesWindowHeight(t *testing.T) {
    m := NewModel()
    m.Width = 80
    m.Height = 5
    out := m.View()
    lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
    if len(lines) != 5 {
        t.Fatalf("expected 5 lines, got %d", len(lines))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestViewUsesWindowHeight -v`
Expected: FAIL with "Width undefined" or "Height undefined" or line count mismatch.

**Step 3: Write minimal implementation**

- Add `Width` and `Height` fields to `Model`.
- In `Update`, handle `tea.WindowSizeMsg` and store size.
- In `View`, pad/trim to `Height` (if > 0) using helper `fitToHeight`.

```go
func fitToHeight(out string, height int) string {
    if height <= 0 {
        return out
    }
    lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
    if len(lines) > height {
        lines = lines[:height]
    }
    for len(lines) < height {
        lines = append(lines, "")
    }
    return strings.Join(lines, "\n") + "\n"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestViewUsesWindowHeight -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/fullscreen_test.go
git commit -m "feat: render TUI to full terminal height"
```

---

### Task 2: Search prompt + filter toggles

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/search_test.go`

**Step 1: Write the failing test**

```go
func TestSearchFiltersTasks(t *testing.T) {
    m := NewModel()
    m.TaskList = []TaskItem{{ID: "T1", Title: "Alpha"}, {ID: "T2", Title: "Beta"}}
    m.SearchQuery = "alp"
    filtered := m.filteredTasks()
    if len(filtered) != 1 || filtered[0].ID != "T1" {
        t.Fatalf("expected Alpha only")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestSearchFiltersTasks -v`
Expected: FAIL with "filteredTasks undefined".

**Step 3: Write minimal implementation**

- Add fields: `SearchMode bool`, `SearchQuery string`, `FilterMode string` (e.g., `all|open|review|done`).
- Add `filteredTasks()` to filter by query + status.
- Key handling: `/` opens search, `enter` exits search, `esc` clears search, `backspace` edits.
- Filter keys: `a` (all), `o` (open = assigned/todo/in_progress), `v` (review), `d` (done).
- Use `filteredTasks()` when rendering list and when selection bounds are checked.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestSearchFiltersTasks -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/search_test.go
git commit -m "feat: add search prompt and status filters"
```

---

### Task 3: Markdown rendering in detail pane

**Files:**
- Modify: `go.mod` `go.sum`
- Create: `internal/tui/markdown.go`
- Create: `internal/tui/markdown_test.go`
- Modify: `internal/tui/model.go`

**Step 1: Write the failing test**

```go
func TestRenderMarkdown(t *testing.T) {
    out, err := renderMarkdown("# Title\n- item", 40)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !strings.Contains(out, "Title") {
        t.Fatalf("expected rendered output")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestRenderMarkdown -v`
Expected: FAIL with "renderMarkdown undefined".

**Step 3: Write minimal implementation**

- Add dependency: `github.com/charmbracelet/glamour`.
- Implement `renderMarkdown` using glamour with word-wrap to pane width.
- In right pane, build a markdown string from detail sections (Summary, Acceptance Criteria, Review Notes) and render via `renderMarkdown`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestRenderMarkdown -v`
Expected: PASS

**Step 5: Commit**

```bash
git add go.mod go.sum internal/tui/markdown.go internal/tui/markdown_test.go internal/tui/model.go
git commit -m "feat: render task detail as markdown"
```

---

### Task 4: File watcher-driven live reload

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/watch.go`
- Create: `internal/tui/watch_test.go`

**Step 1: Write the failing test**

```go
func TestWatchFiltersPaths(t *testing.T) {
    if !shouldReloadPath(".tandemonium/specs/T1.yaml") {
        t.Fatalf("expected spec path to reload")
    }
    if shouldReloadPath("README.md") {
        t.Fatalf("expected README to be ignored")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestWatchFiltersPaths -v`
Expected: FAIL with "shouldReloadPath undefined".

**Step 3: Write minimal implementation**

- Add `watchCmd()` that uses `fsnotify.NewWatcher()` and watches `.tandemonium/specs` and `.tandemonium/state.db` parent dir.
- Emit a `watchMsg` on relevant events (`Write`, `Create`, `Rename`) filtered by `shouldReloadPath`.
- In `Init`, return a batch of `tickCmd()` + `watchCmd()`.
- On `watchMsg`, call `RefreshTasks()` and `RefreshTaskDetail()`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestWatchFiltersPaths -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/watch.go internal/tui/watch_test.go
git commit -m "feat: live reload on spec/state changes"
```

---

### Task 5: UX polish + key hints

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/filter_hint_test.go`

**Step 1: Write the failing test**

```go
func TestFilterHintRenders(t *testing.T) {
    m := NewModel()
    m.FilterMode = "review"
    out := m.View()
    if !strings.Contains(out, "filter: review") {
        t.Fatalf("expected filter hint")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestFilterHintRenders -v`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add a line in the summary/header with `filter: <mode>` and `search: <query>`.
- Update footer key hints to include `/ search`, `a/o/v/d filter`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestFilterHintRenders -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/filter_hint_test.go
git commit -m "chore: surface search and filter hints"
```

