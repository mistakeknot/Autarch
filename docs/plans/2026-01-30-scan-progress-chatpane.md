# Scan Progress In Chat Pane Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `[none] (no bead provided)`

**Goal:** Keep the main doc pane steady during scanning, remove “Found X files / path” copy, and route agent activity + scan progress to the chat pane.

**Architecture:** Treat scanning as chat-panel activity only. The doc pane should remain in its normal state (Autarch copy or step content). Scan progress becomes system messages and/or agent stream lines in the chat panel; remove the scanning overlay in the doc panel.

**Tech Stack:** Go, Bubble Tea, Autarch TUI.

**Skills:** @superpowers:test-driven-development

---

### Task 1: Remove scan overlay from doc panel

**Files:**
- Modify: `internal/tui/views/kickoff.go`
- Test: `internal/tui/views/kickoff_chat_test.go`

**Step 1: Write the failing test**

```go
func TestKickoffScanDoesNotOverrideDocPane(t *testing.T) {
	v := NewKickoffView()
	v.docPanel.SetSize(80, 20)
	v.loading = true
	v.scanning = true
	v.loadingMsg = "Found 5 files to analyze"
	v.scanPath = "/tmp/project"

	view := v.View()
	if strings.Contains(view, "Found 5 files") {
		t.Fatalf("expected scan status not to render in doc pane")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffScanDoesNotOverrideDocPane -v`

Expected: FAIL (scan overlay visible)

**Step 3: Write minimal implementation**

- In `KickoffView.View()`, remove the `if v.loading { leftContent = v.renderScanProgressPane() }` override.
- Keep `docPanel.View()` as the left content always.
- Optionally delete `renderScanProgressPane()` if unused.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffScanDoesNotOverrideDocPane -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/views/kickoff.go internal/tui/views/kickoff_chat_test.go
git commit -m "feat(tui): keep doc pane stable during scan"
```

---

### Task 2: Route scan progress + tool activity to chat pane only

**Files:**
- Modify: `internal/tui/views/kickoff.go`
- Test: `internal/tui/views/kickoff_chat_test.go`

**Step 1: Write the failing test**

```go
func TestKickoffScanProgressRendersInChatOnly(t *testing.T) {
	v := NewKickoffView()
	v.loading = true
	v.scanning = true
	v.loadingMsg = "Found 5 files to analyze"

	_, _ = v.Update(tui.ScanProgressMsg{Step: "Found files", Details: "Found 5 files to analyze", Files: []string{"README.md"}})

	view := v.View()
	if strings.Contains(view, "Found 5 files") {
		t.Fatalf("expected scan progress not to render in doc pane")
	}
	msgs := v.ChatMessagesForTest()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Found 5 files") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scan progress in chat messages")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffScanProgressRendersInChatOnly -v`

Expected: FAIL (progress not guaranteed in chat or still in doc pane)

**Step 3: Write minimal implementation**

- In `KickoffView.Update` handling `ScanProgressMsg`:
  - For all non-empty `Details`, add a `system` message in the chat panel.
  - Stop using `loadingMsg` to drive the doc pane.
  - Keep agent output (`AgentLine`) as chat messages.
- Remove any “Found files / Path / etc” text from doc pane rendering. (Doc pane should never show scan progress.)

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffScanProgressRendersInChatOnly -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/views/kickoff.go internal/tui/views/kickoff_chat_test.go
git commit -m "feat(tui): send scan activity to chat pane"
```

---

## Notes

- This mirrors Cursor behavior: scanning/agent activity appears in chat pane; doc pane remains steady.
- If we want a subtle scan indicator, we can add a short system line like “Scanning…” in chat only.

