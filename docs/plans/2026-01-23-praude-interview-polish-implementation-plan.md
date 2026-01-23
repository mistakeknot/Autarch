# Praude Interview UI Polish Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-rte` (Task reference)

**Goal:** Polish the interview UI to match an Agent-Desk style by redesigning the composer, chat transcript, and header nav.

**Architecture:** Keep existing interview layout but refine rendering helpers in `internal/praude/tui/interview.go` and style utilities in `internal/praude/tui/styles.go`. Add targeted tests in `internal/praude/tui/interview_test.go` to lock visual structure and strings.

**Tech Stack:** Go, Bubble Tea, lipgloss styles in `pkg/tui`.

---

### Task 1: Composer redesign (title bar + input + compact status line)

**Files:**
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/styles.go`
- Test: `internal/praude/tui/interview_test.go`

**Step 1: Write the failing test**

Add a test asserting the composer shows `Compose · <Step>` and the compact status line in the interview view.

```go
func TestInterviewComposerShowsTitleAndHints(t *testing.T) {
    withTempRoot(t, func(root string) {
        m := NewModel()
        m.mode = "interview"
        m.interview = startInterview(m.root, specs.Spec{}, "")
        m.interview.step = stepVision
        out := stripANSI(m.View())
        if !strings.Contains(out, "Compose · Vision") {
            t.Fatalf("expected composer title with step")
        }
        if !strings.Contains(out, "Ctrl+O") || !strings.Contains(out, "\\") {
            t.Fatalf("expected compact composer hints")
        }
    })
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewComposerShowsTitleAndHints`
Expected: FAIL (title/hints not yet styled).

**Step 3: Write minimal implementation**

- Update `renderInterviewComposerLines()` in `internal/praude/tui/interview.go`:
  - Title line `Compose · <Step>` using a new style helper in `styles.go`.
  - Remove redundant `Input:` label.
  - Status line: `Enter: iterate · [ / ]: prev/next · Ctrl+O: open · \: swap · (line X, col Y)`.
- Add a `renderComposerTitle` style helper in `internal/praude/tui/styles.go` (reuse existing shared styles).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewComposerShowsTitleAndHints`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/interview.go internal/praude/tui/styles.go internal/praude/tui/interview_test.go
git commit -m "feat(praude): polish interview composer"
```

---

### Task 2: Chat transcript styling (role badges + spacing)

**Files:**
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/styles.go`
- Test: `internal/praude/tui/interview_test.go`

**Step 1: Write the failing test**

Add a test asserting transcript formatting with role badges on their own line and indented message text.

```go
func TestInterviewTranscriptUsesRoleBadges(t *testing.T) {
    withTempRoot(t, func(root string) {
        m := NewModel()
        m.mode = "interview"
        m.interview = startInterview(m.root, specs.Spec{}, "")
        m.interview.chat = []interviewMessage{{Role: "user", Text: "Hello"}}
        out := stripANSI(m.View())
        if !strings.Contains(out, "[User]") {
            t.Fatalf("expected user badge")
        }
        if !strings.Contains(out, "  Hello") {
            t.Fatalf("expected indented message")
        }
    })
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewTranscriptUsesRoleBadges`
Expected: FAIL (current transcript format is inline).

**Step 3: Write minimal implementation**

- Update `renderInterviewTranscriptLines()` in `internal/praude/tui/interview.go`:
  - Add badges `[User]` / `[Agent]` on their own line.
  - Add a single indented line for message content (`"  " + msg.Text`).
  - Insert a blank line between message blocks.
- Add optional light divider string if desired (ASCII safe).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewTranscriptUsesRoleBadges`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/interview.go internal/praude/tui/styles.go internal/praude/tui/interview_test.go
git commit -m "feat(praude): style interview transcript"
```

---

### Task 3: Header nav pill styling + responsive collapse

**Files:**
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/styles.go`
- Test: `internal/praude/tui/interview_test.go`

**Step 1: Write the failing test**

Add a test that verifies active step uses double brackets and collapsed nav appears for narrow width.

```go
func TestInterviewHeaderNavActiveAndCollapsed(t *testing.T) {
    withTempRoot(t, func(root string) {
        m := NewModel()
        m.mode = "interview"
        m.interview = startInterview(m.root, specs.Spec{}, "")
        m.interview.step = stepProblem
        m.width = 60
        out := stripANSI(m.View())
        if !strings.Contains(out, "[[Problem]]") {
            t.Fatalf("expected active step emphasis")
        }
        if !strings.Contains(out, "...") {
            t.Fatalf("expected collapsed nav")
        }
    })
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewHeaderNavActiveAndCollapsed`
Expected: FAIL (no pill emphasis/collapse yet).

**Step 3: Write minimal implementation**

- Update `renderInterviewHeaderNav()` in `internal/praude/tui/interview.go`:
  - Render inactive pills as `[Step]`.
  - Render active as `[[Step]]`.
  - If width < 80, show `...` and only include active + neighbors.
- Add header style helpers in `styles.go` if needed.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewHeaderNavActiveAndCollapsed`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/interview.go internal/praude/tui/styles.go internal/praude/tui/interview_test.go
git commit -m "feat(praude): polish interview header nav"
```

---

### Task 4: Full TUI test sweep

**Files:**
- Test: `internal/praude/tui/...`

**Step 1: Run full TUI test suite**

Run: `go test ./internal/praude/tui`
Expected: PASS.

**Step 2: Commit any fixes if needed**

```bash
git add internal/praude/tui/...
git commit -m "fix(praude): stabilize interview polish"
```

---

## Execution Options

Plan complete and saved to `docs/plans/2026-01-23-praude-interview-polish-implementation-plan.md`. Two execution options:

1. Subagent-Driven (this session) — I dispatch fresh subagent per task, review between tasks, fast iteration
2. Parallel Session (separate) — Open a new session with executing-plans, batch execution with checkpoints

Which approach?
