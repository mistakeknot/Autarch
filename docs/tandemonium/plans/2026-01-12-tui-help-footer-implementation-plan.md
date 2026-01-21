# TUI Help Footer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add an inline help footer that lists key bindings for review navigation and approval.

**Architecture:** Render a simple, static footer string at the end of the TUI view to show the most important keys. Keep it text-only and always visible.

**Tech Stack:** Go 1.24+, Bubble Tea.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Add help footer to TUI view

**Files:**
- Modify: `internal/tui/view_test.go`
- Modify: `internal/tui/model.go`

**Step 1: Write failing test**

```go
func TestViewIncludesHelpFooter(t *testing.T) {
	m := NewModel()
	out := m.View()
	if !strings.Contains(out, "KEYS: j/k") {
		t.Fatalf("expected help footer, got %q", out)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/tui -v`
Expected: FAIL with "expected help footer"

**Step 3: Implement footer**

Add a footer line in `View()`:

```go
out += "\nKEYS: j/k up/down, enter approve, a approve, y/n confirm\n"
```

**Step 4: Run test**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view_test.go
git commit -m "feat: add TUI help footer"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
