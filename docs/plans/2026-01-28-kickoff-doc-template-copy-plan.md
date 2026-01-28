# Kickoff Doc Template Copy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** n/a (no bead provided)

**Goal:** Show fixed Autarch kickoff guidance in the left doc panel, directing users to use the chat panel to start a PRD.

**Architecture:** Update the Kickoff view’s doc panel sections to include a fixed “Autarch” guidance block above tips/shortcuts, and add a lightweight view test that asserts the rendered doc panel includes the new copy.

**Tech Stack:** Go, Bubble Tea, lipgloss

### Task 1: Add fixed doc panel guidance + test

**Files:**
- Modify: `internal/tui/views/kickoff.go`
- Create: `internal/tui/views/kickoff_doc_test.go`

**Step 1: Write the failing test**

Create `internal/tui/views/kickoff_doc_test.go`:

```go
package views

import (
    "strings"
    "testing"
)

func TestKickoffDocPanelIncludesAutarchCopy(t *testing.T) {
    v := NewKickoffView()
    v.docPanel.SetSize(80, 20)

    view := v.docPanel.View()
    expected := "Autarch is a platform for a suite of agentic tools"

    if !strings.Contains(view, expected) {
        t.Fatalf("expected doc panel to include kickoff copy, got %q", view)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/views -run TestKickoffDocPanelIncludesAutarchCopy -v`

Expected: FAIL (copy not found).

**Step 3: Write minimal implementation**

In `internal/tui/views/kickoff.go`, update `updateDocPanel()` to add a new first section with the fixed copy:

```go
v.docPanel.AddSection(pkgtui.DocSection{
    Title:   "Autarch",
    Content: "Autarch is a platform for a suite of agentic tools to help you build better products. Use the chat panel on the right to start creating a PRD that will provide a solid foundation to build upon.",
    Style:   lipgloss.NewStyle().Foreground(pkgtui.ColorFg),
})
```

(Keep Tips/Shortcuts below this section.)

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/views -run TestKickoffDocPanelIncludesAutarchCopy -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/views/kickoff.go internal/tui/views/kickoff_doc_test.go
git commit -m "feat(tui): add kickoff doc panel intro copy"
```
