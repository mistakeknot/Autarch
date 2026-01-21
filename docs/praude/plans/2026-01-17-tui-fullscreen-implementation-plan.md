# Fullscreen TUI Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `praude-ur1` (Task reference)

**Goal:** Refactor the Praude TUI into a full-screen, beadsviewer-inspired split-view interface with Markdown detail rendering, help/tutorial overlays, and a consistent keymap.

**Architecture:** Introduce a view router with discrete screens and shared state. Render a fixed header/body/footer layout, with Markdown detail rendering and caching. Add help and tutorial overlays and a beadsviewer-style keymap.

**Tech Stack:** Go, Bubble Tea (tea), Lip Gloss, Glamour (Markdown rendering), existing `internal/tui` state model.

---

### Task 1: Add view router scaffold

**Files:**
- Create: `internal/tui/router.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/router_test.go`

**Step 1: Write the failing test**
```go
func TestRouterSwitchesScreens(t *testing.T) {
	m := NewModel()
	if m.router.active != "list" {
		t.Fatalf("expected list screen")
	}
	m.router.Switch("help")
	if m.router.active != "help" {
		t.Fatalf("expected help screen")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestRouterSwitchesScreens -v`
Expected: FAIL (router not implemented)

**Step 3: Write minimal implementation**
```go
type Router struct {
	active string
}

func (r *Router) Switch(name string) { r.active = name }
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestRouterSwitchesScreens -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/router.go internal/tui/router_test.go internal/tui/model.go
git commit -m "feat(tui): add router scaffold"
```

---

### Task 2: Add full-screen layout frame (header/body/footer)

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/layout.go`
- Test: `internal/tui/layout_test.go`

**Step 1: Write the failing test**
```go
func TestLayoutIncludesHeaderFooter(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "PRAUDE") {
		t.Fatalf("expected header")
	}
	if !strings.Contains(out, "KEYS:") {
		t.Fatalf("expected footer")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestLayoutIncludesHeaderFooter -v`
Expected: FAIL (no header/footer)

**Step 3: Write minimal implementation**
```go
func renderHeader(title, focus string) string {
	return "PRAUDE | " + title + " | [" + focus + "]"
}

func renderFooter(keys, status string) string {
	return "KEYS: " + keys + " | " + status
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestLayoutIncludesHeaderFooter -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/layout.go internal/tui/layout_test.go internal/tui/model.go
git commit -m "feat(tui): add header/footer layout"
```

---

### Task 3: Implement split view + single-column fallback

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/layout.go`
- Test: `internal/tui/layout_test.go`

**Step 1: Write the failing test**
```go
func TestSplitViewFallback(t *testing.T) {
	out := renderSplitView(60, []string{"L"}, []string{"R"})
	if strings.Contains(out, "|") {
		t.Fatalf("expected single column on narrow width")
	}
	wide := renderSplitView(140, []string{"L"}, []string{"R"})
	if !strings.Contains(wide, "|") {
		t.Fatalf("expected split view on wide width")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestSplitViewFallback -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
func renderSplitView(width int, left, right []string) string {
	if width < 100 {
		return strings.Join(left, "\n")
	}
	return joinColumns(left, right, 42)
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestSplitViewFallback -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/layout.go internal/tui/layout_test.go internal/tui/model.go
git commit -m "feat(tui): add split view fallback"
```

---

### Task 4: Add Markdown detail rendering with cache

**Files:**
- Create: `internal/tui/markdown.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/markdown_test.go`

**Step 1: Write the failing test**
```go
func TestMarkdownCacheHits(t *testing.T) {
	cache := NewMarkdownCache()
	cache.Set("PRD-001", "hash", "rendered")
	if got, ok := cache.Get("PRD-001", "hash"); !ok || got != "rendered" {
		t.Fatalf("expected cached render")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestMarkdownCacheHits -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
type MarkdownCache struct {
	items map[string]string
}

func (c *MarkdownCache) key(id, hash string) string { return id + ":" + hash }
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestMarkdownCacheHits -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/markdown.go internal/tui/markdown_test.go internal/tui/model.go
git commit -m "feat(tui): add markdown cache"
```

---

### Task 5: Add help overlay and tutorial overlay

**Files:**
- Create: `internal/tui/overlay.go`
- Modify: `internal/tui/model.go`
- Test: `internal/tui/overlay_test.go`

**Step 1: Write the failing test**
```go
func TestHelpOverlayToggle(t *testing.T) {
	m := NewModel()
	m = pressKey(m, "?")
	if !strings.Contains(m.View(), "Help") {
		t.Fatalf("expected help overlay")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestHelpOverlayToggle -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
func renderHelp() string { return "Help\n j/k: move  /: search  g: interview" }
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestHelpOverlayToggle -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/overlay.go internal/tui/overlay_test.go internal/tui/model.go
git commit -m "feat(tui): add help and tutorial overlays"
```

---

### Task 6: Adopt beadsviewer-style keymap + focus indicator

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/layout.go`
- Test: `internal/tui/model_test.go`

**Step 1: Write the failing test**
```go
func TestFocusIndicatorShown(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "[LIST]") {
		t.Fatalf("expected focus indicator")
	}
}
```

**Step 2: Run test to verify it fails**
Run: `go test ./internal/tui -run TestFocusIndicatorShown -v`
Expected: FAIL

**Step 3: Write minimal implementation**
```go
func renderHeader(title, focus string) string {
	return "PRAUDE | " + title + " | [" + focus + "]"
}
```

**Step 4: Run test to verify it passes**
Run: `go test ./internal/tui -run TestFocusIndicatorShown -v`
Expected: PASS

**Step 5: Commit**
```bash
git add internal/tui/layout.go internal/tui/model.go internal/tui/model_test.go
git commit -m "feat(tui): add beadsviewer keymap and focus indicator"
```

---

### Task 7: Update docs and polish

**Files:**
- Modify: `docs/plans/2026-01-17-tui-fullscreen-design.md`
- Modify: `docs/plans/2026-01-14-praude-design.md`

**Step 1: Update docs for new UI**
- Add keymap table
- Document overlays and markdown detail rendering

**Step 2: Commit**
```bash
git add docs/plans/2026-01-17-tui-fullscreen-design.md docs/plans/2026-01-14-praude-design.md
git commit -m "docs(praude): document fullscreen TUI"
```

---

## Verification
- `go test ./internal/tui -v`
- `go test ./...`

---

Plan complete and saved to `docs/plans/2026-01-17-tui-fullscreen-implementation-plan.md`. Two execution options:

1) Subagent-Driven (this session) - I dispatch fresh subagent per task, review between tasks, fast iteration

2) Parallel Session (separate) - Open new session with executing-plans, batch execution with checkpoints

Which approach?
