# Chat Panel Mouse Scroll Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `[none] (no bead provided)`

**Goal:** Enable trackpad/mouse wheel scrolling for the chat panel without affecting other panes.

**Architecture:** Enable Bubble Tea mouse events for the unified app and route wheel events to the chat panel when it has focus. Add a small test-only accessor to verify scroll state changes.

**Tech Stack:** Go, Bubble Tea, Autarch TUI.

**Skills:** @superpowers:test-driven-development

---

### Task 1: Add test for mouse wheel scrolling in kickoff chat

**Files:**
- Modify: `internal/tui/views/kickoff_chat_test.go`
- Modify: `pkg/tui/chatpanel.go`

**Step 1: Write the failing test**

```go
func TestKickoffMouseWheelScrollsChatWhenFocused(t *testing.T) {
	v := NewKickoffView()
	v.focusInput = true
	v.chatPanel.SetSize(60, 20)
	v.chatPanel.AddMessage("user", "One")
	v.chatPanel.AddMessage("user", "Two")

	before := v.chatPanel.ScrollOffsetForTest()
	_, _ = v.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	after := v.chatPanel.ScrollOffsetForTest()
	if after <= before {
		t.Fatalf("expected chat scroll offset to increase")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffMouseWheelScrollsChatWhenFocused -v`

Expected: FAIL (missing ScrollOffsetForTest / mouse handling)

**Step 3: Write minimal implementation**

- Add `ScrollOffsetForTest()` to `pkg/tui/chatpanel.go` returning `p.scroll`.
- Handle `tea.MouseMsg` in `internal/tui/views/kickoff.go`:
  - If `msg.Type == tea.MouseWheelUp` and `focusInput` is true, call `chatPanel.ScrollUp()`.
  - If `msg.Type == tea.MouseWheelDown` and `focusInput` is true, call `chatPanel.ScrollDown()`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffMouseWheelScrollsChatWhenFocused -v`

Expected: PASS

**Step 5: Commit**

```bash
git add pkg/tui/chatpanel.go internal/tui/views/kickoff_chat_test.go internal/tui/views/kickoff.go
git commit -m "feat(tui): scroll chat panel with mouse wheel"
```

---

### Task 2: Enable mouse input for unified app

**Files:**
- Modify: `internal/tui/unified_app.go`

**Step 1: Write the failing test**

```go
func TestRunUnifiedEnablesMouse(t *testing.T) {
	// No direct hook to assert options; validate manually by running the app
	// and confirming wheel events reach the view. Document in plan notes.
}
```

**Step 2: Manual verification (expected failure before change)**

Run the app and scroll trackpad in chat: it should not scroll before the change.

**Step 3: Write minimal implementation**

- Update `RunUnified` to create the program with mouse enabled:

```go
p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
```

**Step 4: Manual verification (expected pass)**

- Run unified app and scroll the chat panel with trackpad; it should scroll.

**Step 5: Commit**

```bash
git add internal/tui/unified_app.go
git commit -m "feat(tui): enable mouse input in unified app"
```

---

## Notes

- Scrolling applies to the chat pane only when it has focus, so document pane remains unchanged.
- If you want hover‑based targeting later, compute pane bounds from `ShellLayout` and route based on cursor position.

