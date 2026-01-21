# TUI Approve Confirmation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add a confirmation step before approving a review item (default on), with a config flag to disable.

**Architecture:** Extend the TUI model with confirmation state (pending task ID) and handle `y/n` keys. Wire config loading in the CLI entrypoint to set a default `ConfirmApprove` flag on the model.

**Tech Stack:** Go 1.24+, Bubble Tea, TOML config.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Add confirmation flow to approve path

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/approve_key_test.go`

**Step 1: Write failing tests**

```go
func TestApproveEnterRequiresConfirmation(t *testing.T) {
	m := NewModel()
	m.ConfirmApprove = true
	m.ReviewQueue = []string{"T1"}
	m.BranchLookup = func(taskID string) (string, error) {
		return "feature/" + taskID, nil
	}
	m.ReviewLoader = func() ([]string, error) {
		return []string{}, nil
	}
	fake := &fakeKeyApprover{}
	m.Approver = fake

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if fake.called {
		t.Fatal("expected approve to be deferred")
	}
	if m.PendingApproveTask != "T1" {
		t.Fatalf("expected pending task, got %q", m.PendingApproveTask)
	}
}

func TestApproveConfirmationYRunsApprove(t *testing.T) {
	m := NewModel()
	m.ConfirmApprove = true
	m.PendingApproveTask = "T1"
	m.BranchLookup = func(taskID string) (string, error) {
		return "feature/" + taskID, nil
	}
	m.ReviewLoader = func() ([]string, error) {
		return []string{}, nil
	}
	fake := &fakeKeyApprover{}
	m.Approver = fake

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if !fake.called {
		t.Fatal("expected approve call on confirmation")
	}
	if m.PendingApproveTask != "" {
		t.Fatalf("expected pending cleared, got %q", m.PendingApproveTask)
	}
}

func TestApproveConfirmationNCancels(t *testing.T) {
	m := NewModel()
	m.ConfirmApprove = true
	m.PendingApproveTask = "T1"

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if m.PendingApproveTask != "" {
		t.Fatalf("expected pending cleared, got %q", m.PendingApproveTask)
	}
	if m.StatusLevel != StatusInfo {
		t.Fatalf("expected status info, got %v", m.StatusLevel)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/tui -v`
Expected: FAIL with "m.ConfirmApprove undefined"

**Step 3: Implement confirmation state + key handling**

```go
type Model struct {
	// ...
	ConfirmApprove    bool
	PendingApproveTask string
}

func NewModel() Model {
	return Model{
		// ...
		ConfirmApprove: true,
	}
}

// In Update key handling:
case "enter", "a":
	if len(m.ReviewQueue) > 0 {
		idx := m.SelectedReview
		if idx < 0 || idx >= len(m.ReviewQueue) {
			idx = 0
		}
		taskID := m.ReviewQueue[idx]
		if m.ConfirmApprove {
			m.PendingApproveTask = taskID
			m.SetStatusInfo("confirm approve " + taskID + " (y/n)")
			return m, nil
		}
		// fall through to approve logic
	}
case "y":
	if m.PendingApproveTask != "" {
		taskID := m.PendingApproveTask
		m.PendingApproveTask = ""
		// run approve logic for taskID
	}
case "n":
	if m.PendingApproveTask != "" {
		m.PendingApproveTask = ""
		m.SetStatusInfo("approve cancelled")
	}
```

Factor the approval logic into a helper (e.g., `approveTaskByID(taskID string)`) so `enter` and `y` can call the same code without duplication.

**Step 4: Run tests**

Run: `go test ./internal/tui -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/approve_key_test.go
git commit -m "feat: add approve confirmation flow"
```

---

### Task 2: Add config flag to disable confirmation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/tui/model.go`

**Step 1: Write failing tests**

```go
func TestLoadProjectConfigConfirmApproveDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadFromProject(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TUI.ConfirmApprove != true {
		t.Fatalf("expected confirm approve default true")
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/config -v`
Expected: FAIL with "cfg.TUI undefined"

**Step 3: Implement config + wiring**

```go
type TUIConfig struct {
	ConfirmApprove bool `toml:"confirm_approve"`
}

type Config struct {
	General GeneralConfig `toml:"general"`
	TUI     TUIConfig     `toml:"tui"`
}

func defaultConfig() Config {
	return Config{
		General: GeneralConfig{MaxAgents: 4},
		TUI:     TUIConfig{ConfirmApprove: true},
	}
}
```

In `internal/cli/root.go`, load config and pass to the model:

```go
cfg, err := config.LoadFromProject(".")
if err != nil { return err }
m := tui.NewModel()
m.ConfirmApprove = cfg.TUI.ConfirmApprove
p := tea.NewProgram(m)
```

**Step 4: Run tests**

Run: `go test ./internal/config -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/cli/root.go internal/tui/model.go
git commit -m "feat: add config flag for approve confirmation"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
