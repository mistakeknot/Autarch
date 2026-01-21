# TUI Router + Search Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `praude-zcz` (Task reference)

**Goal:** Refactor the Praude TUI into real screen structs with a router and add a beadsviewer-style search/filter UI (`/`) with focus-aware input handling.

**Architecture:** Introduce a `Screen` interface with a router that owns shared state and delegates `Update/View` to active screens. Add a search input state that is modal, isolates key handling, and filters list entries. Keep overlays (help/tutorial) as screens layered by the router.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, Glamour, existing `internal/tui`.

---

### Task 1: Define Screen interface + router dispatch

**Files:**
- Create: `internal/tui/screen.go`
- Modify: `internal/tui/router.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/router_test.go`

**Step 1: Write the failing test**
```go
func TestRouterDispatchesToActiveScreen(t *testing.T) {
	list := &ListScreen{}
	help := &HelpScreen{}
	r := NewRouter(map[string]Screen{"list": list, "help": help}, "list")
	if r.ActiveName() != "list" {
		t.Fatalf("expected list")
	}
	r.Switch("help")
	if r.ActiveName() != "help" {
		t.Fatalf("expected help")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestRouterDispatchesToActiveScreen -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
type Screen interface {
	Update(tea.Msg, *SharedState) (Screen, Intent)
	View(*SharedState) string
	Title() string
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestRouterDispatchesToActiveScreen -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/screen.go internal/tui/router.go internal/tui/router_test.go internal/tui/model.go
git commit -m "feat(tui): add screen interface and router dispatch"
```

---

### Task 2: Extract List/Detail screen

**Files:**
- Create: `internal/tui/screen_list.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/list_screen_test.go`

**Step 1: Write the failing test**
```go
func TestListScreenRendersSelection(t *testing.T) {
	state := NewSharedState()
	state.Summaries = []specs.Summary{{ID: "PRD-001", Title: "Alpha"}}
	out := (&ListScreen{}).View(state)
	if !strings.Contains(out, "PRD-001") {
		t.Fatalf("expected list item")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestListScreenRendersSelection -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
func (s *ListScreen) View(state *SharedState) string {
	return renderList(state)
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestListScreenRendersSelection -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/screen_list.go internal/tui/list_screen_test.go internal/tui/model.go
git commit -m "feat(tui): extract list/detail screen"
```

---

### Task 3: Add shared state model

**Files:**
- Create: `internal/tui/state.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/state_test.go`

**Step 1: Write the failing test**
```go
func TestSharedStateDefaults(t *testing.T) {
	state := NewSharedState()
	if state.Focus != "LIST" {
		t.Fatalf("expected LIST focus")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestSharedStateDefaults -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
type SharedState struct {
	Summaries []specs.Summary
	Selected  int
	Focus     string
	Filter    string
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestSharedStateDefaults -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/state.go internal/tui/state_test.go internal/tui/model.go
git commit -m "feat(tui): add shared state"
```

---

### Task 4: Implement search/filter input (`/`)

**Files:**
- Create: `internal/tui/search.go`
- Modify: `internal/tui/screen_list.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/search_test.go`

**Step 1: Write the failing test**
```go
func TestSearchFiltersList(t *testing.T) {
	state := NewSharedState()
	state.Summaries = []specs.Summary{
		{ID: "PRD-001", Title: "Alpha"},
		{ID: "PRD-002", Title: "Beta"},
	}
	state.Filter = "Alpha"
	items := filterSummaries(state.Summaries, state.Filter)
	if len(items) != 1 {
		t.Fatalf("expected filtered list")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestSearchFiltersList -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
func filterSummaries(items []specs.Summary, filter string) []specs.Summary { ... }
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestSearchFiltersList -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/search.go internal/tui/search_test.go internal/tui/screen_list.go internal/tui/model.go
git commit -m "feat(tui): add search filter UI"
```

---

### Task 5: Search input modal and key isolation

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/search.go`
- Test: `internal/tui/search_test.go`

**Step 1: Write the failing test**
```go
func TestSearchModalConsumesKeys(t *testing.T) {
	m := NewModel()
	m = pressKey(m, "/")
	m = pressKey(m, "a")
	if m.search.Query != "a" {
		t.Fatalf("expected search query updated")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestSearchModalConsumesKeys -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
if state.SearchActive { handleSearchInput(msg) }
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestSearchModalConsumesKeys -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/model.go internal/tui/search.go internal/tui/search_test.go
git commit -m "feat(tui): add modal search input"
```

---

### Task 6: Update help/tutorial to include search

**Files:**
- Modify: `internal/tui/overlay.go`
- Test: `internal/tui/overlay_test.go`

**Step 1: Update overlay copy**
- Add `/` search line to help and tutorial.

**Step 2: Commit**
```bash
git add internal/tui/overlay.go internal/tui/overlay_test.go
git commit -m "docs(tui): add search to overlays"
```

---

## Verification
- `go test ./internal/tui -v`
- `go test ./...`

---

Plan complete and saved to `docs/plans/2026-01-17-tui-router-search-implementation-plan.md`. Two execution options:

1) Subagent-Driven (this session) - I dispatch fresh subagent per task, review between tasks, fast iteration

2) Parallel Session (separate) - Open new session with executing-plans, batch execution with checkpoints

Which approach?
