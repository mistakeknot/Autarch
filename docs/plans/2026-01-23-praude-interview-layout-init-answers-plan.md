# Praude Interview Layout + Init Answers Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-bgq` (Task reference)

**Goal:** Modernize the Praude interview UI (header nav + top-left selector + top-right read-only section + bottom chat/composer with swap + open file), auto-init .praude on first run, and optionally seed PRD answers with a coding-agent scan when creating a new PRD.

**Architecture:** Introduce an interview-specific layout renderer that composes three zones (header, top split, bottom split), plus a lightweight chat transcript model that records user/agent turns. Add a small auto-init helper used by CLI and TUI. Add an optional “agent bootstrap” step when creating a new PRD that scans the repo and fills initial answers/drafts.

**Tech Stack:** Go, Bubble Tea, lipgloss styles in `pkg/tui`, YAML parsing, Cobra CLI.

---

### Task 1: Auto-init `.praude` on first run (CLI + TUI)

**Files:**
- Modify: `internal/praude/cli/root.go`
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/cli/root_test.go`
- Modify: `internal/praude/tui/model_test.go`

**Step 1: Write the failing test**

Update CLI test to expect auto-init (no “praude init” prompt), and ensure `.praude/config.toml` exists after running root.

```go
func TestRootRunAutoInitCreatesPraudeDir(t *testing.T) {
    root := t.TempDir()
    origRun := runTUI
    runTUI = func() error { return nil }
    defer func() { runTUI = origRun }()

    cwd, _ := os.Getwd()
    defer func() { _ = os.Chdir(cwd) }()
    if err := os.Chdir(root); err != nil { t.Fatal(err) }

    cmd := NewRoot()
    buf := bytes.NewBuffer(nil)
    cmd.SetOut(buf)
    cmd.SetErr(bytes.NewBuffer(nil))
    if err := cmd.Execute(); err != nil { t.Fatalf("unexpected error: %v", err) }

    if _, err := os.Stat(filepath.Join(root, ".praude", "config.toml")); err != nil {
        t.Fatalf("expected auto-init config, got %v", err)
    }
    if strings.Contains(buf.String(), "praude init") {
        t.Fatalf("did not expect init prompt")
    }
}
```

Update TUI tests that call `NewModel()` without chdir to a temp dir (to avoid creating `.praude` in repo during tests).

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/cli -run TestRootRunAutoInitCreatesPraudeDir`
Expected: FAIL (auto-init not yet implemented).

**Step 3: Write minimal implementation**

Add a small helper in `internal/praude/cli/root.go` or shared `tui/model.go`:

```go
func ensurePraudeInitialized(root string) error {
    if _, err := os.Stat(project.RootDir(root)); err == nil {
        return nil
    }
    if err := project.Init(root); err != nil { return err }
    _, err := specs.CreateTemplate(project.SpecsDir(root), time.Now())
    return err
}
```

- In `NewRoot`, call `ensurePraudeInitialized(cwd)` instead of printing “Not initialized”.
- In `NewModel`, if `.praude` missing, call the same helper and proceed.
- Update tests to chdir into temp dirs before calling `NewModel()`.

**Step 4: Run tests to verify it passes**

Run: `go test ./internal/praude/cli -run TestRootRunAutoInitCreatesPraudeDir`
Expected: PASS.

Run: `go test ./internal/praude/tui -run TestViewIncludesHeaders`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/cli/root.go internal/praude/tui/model.go internal/praude/cli/root_test.go internal/praude/tui/model_test.go
git commit -m "feat(praude): auto-init on first run"
```

---

### Task 2: Interview chat transcript model + composer rendering

**Files:**
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/input_buffer.go`
- Modify: `internal/praude/tui/interview_test.go`
- (Optional) Add: `internal/praude/tui/interview_chat.go`

**Step 1: Write the failing test**

Add a test that ensures the interview view renders a chat transcript block with user/agent labels and a full-width composer box label.

```go
func TestInterviewChatRendersTranscript(t *testing.T) {
    m := NewModel()
    m.mode = "interview"
    m.interview = startInterview(m.root, specs.Spec{}, "")
    m.interview.chat = []interviewMessage{
        {Role: "user", Text: "User line"},
        {Role: "agent", Text: "Agent line"},
    }
    out := m.View()
    if !strings.Contains(out, "User") || !strings.Contains(out, "Agent") {
        t.Fatalf("expected chat transcript")
    }
    if !strings.Contains(out, "Compose") {
        t.Fatalf("expected composer label")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewChatRendersTranscript`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add `interviewMessage` struct and `chat []interviewMessage` in `interviewState`.
- Append to chat on user submit (`Enter`) and after agent draft update.
- Add `renderInterviewChat()` to format transcript blocks (User/Agent labels) and a composer area with a header line (e.g., `Compose`), bordered input area, and a status/hint line.

**Step 4: Run tests to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewChatRendersTranscript`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/interview.go internal/praude/tui/interview_test.go internal/praude/tui/input_buffer.go
git commit -m "feat(praude): add interview chat transcript"
```

---

### Task 3: New interview layout + header nav + swap key + open file

**Files:**
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/tui/layout.go`
- Modify: `internal/praude/tui/styles.go`
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/interview_test.go`

**Step 1: Write the failing test**

Add a layout test for the interview mode that asserts:
- Header nav includes step labels.
- Top-left PRD selector is present.
- Section panel is read-only with “Open file: Ctrl+O”.
- Bottom chat/composer exists.

```go
func TestInterviewLayoutShowsHeaderAndPanels(t *testing.T) {
    m := NewModel()
    m.mode = "interview"
    m.interview = startInterview(m.root, specs.Spec{}, "")
    out := m.View()
    if !strings.Contains(out, "Scan") || !strings.Contains(out, "Vision") {
        t.Fatalf("expected header nav steps")
    }
    if !strings.Contains(out, "PRDs") || !strings.Contains(out, "Section") {
        t.Fatalf("expected top panels")
    }
    if !strings.Contains(out, "Open file: Ctrl+O") {
        t.Fatalf("expected open file hint")
    }
    if !strings.Contains(out, "Compose") {
        t.Fatalf("expected composer")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewLayoutShowsHeaderAndPanels`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add `interviewLayoutSwap bool` to `Model`.
- In `Update`, bind `Ctrl+`` to toggle swap; `\` as fallback.
- Add `Ctrl+O` handler to open the current spec file in `$EDITOR` (fallback `vi`), and report errors in status.
- Add new layout helper in `layout.go`:
  - `renderInterviewLayout(width,height,header,topLeft,topRight,bottom string, swap bool)`
  - Handles stacked mode when narrow.
- Add a `renderInterviewHeaderNav()` in `interview.go` to build the step nav.
- Replace `renderInterviewPanel` usage in `model.View()` with the new layout.

**Step 4: Run tests to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewLayoutShowsHeaderAndPanels`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/model.go internal/praude/tui/layout.go internal/praude/tui/styles.go internal/praude/tui/interview.go internal/praude/tui/interview_test.go
git commit -m "feat(praude): agent-style interview layout"
```

---

### Task 4: Agent-generated initial answers for new PRD (optional, skippable)

**Files:**
- Modify: `internal/praude/tui/interview.go`
- Modify: `internal/praude/tui/interview_agent.go`
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/tui/interview_test.go`
- Add: `internal/praude/tui/interview_bootstrap.go`

**Step 1: Write the failing test**

Add a test that verifies when a new PRD is created, the interview flow includes a “Generate initial answers?” step and that skipping it proceeds to Vision.

```go
func TestInterviewIncludesBootstrapStep(t *testing.T) {
    m := NewModel()
    m.startNewInterview()
    if m.interview.step != stepScanPrompt {
        t.Fatalf("expected scan step")
    }
    m.handleInterviewInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}) // skip scan
    if m.interview.step != stepBootstrapPrompt {
        t.Fatalf("expected bootstrap step")
    }
    m.handleInterviewInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}) // skip bootstrap
    if m.interview.step != stepVision {
        t.Fatalf("expected vision step")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestInterviewIncludesBootstrapStep`
Expected: FAIL.

**Step 3: Write minimal implementation**

- Add a new interview step `stepBootstrapPrompt` after scan/confirm.
- Add a new brief builder for bootstrap in `interview_bootstrap.go` that asks the agent to infer initial answers for Vision/Users/Problem/Requirements using repo scan summary + file list.
- When the user selects “Yes”, run scan + agent; parse YAML response with fields:

```yaml
vision: |
  ...
users: |
  ...
problem: |
  ...
requirements: |
  ...
```

- Seed `interview.answers` and `interview.drafts` with returned values, then proceed to Vision.
- If user selects “No”, proceed directly to Vision.

**Step 4: Run tests to verify it passes**

Run: `go test ./internal/praude/tui -run TestInterviewIncludesBootstrapStep`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/interview.go internal/praude/tui/interview_agent.go internal/praude/tui/model.go internal/praude/tui/interview_test.go internal/praude/tui/interview_bootstrap.go
git commit -m "feat(praude): bootstrap initial answers via agent"
```

---

## Execution Options

Plan complete and saved to `docs/plans/2026-01-23-praude-interview-layout-init-answers-plan.md`. Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks, fast iteration
2. Parallel Session (separate) — Open a new session with executing-plans, batch execution with checkpoints

Which approach?
