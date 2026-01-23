# Praude Archive/Delete/Undo Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-bwe` (Task reference)

**Goal:** Add archive/delete/undo for PRDs and artifacts with TUI and CLI support, plus a show-archived toggle.

**Architecture:** Implement a shared archive/delete engine that moves files between `.praude/` folders, updates PRD status, and records last action in `.praude/state.json`. Wire it into TUI keybindings with confirmation overlays and into CLI commands. Archived items are hidden by default but can be included via toggle/flag.

**Tech Stack:** Go 1.24+, Bubble Tea, YAML, JSON state file

Note: User explicitly requested no worktrees; implementation will be in the current working tree.

---

### Task 1: Add archive/trash paths and state fields

**Files:**
- Modify: `internal/praude/project/project.go`
- Modify: `internal/praude/tui/state_persist.go`
- Modify: `internal/praude/tui/state_persist_test.go`

**Step 1: Write the failing test**

Update `internal/praude/tui/state_persist_test.go`:

```go
func TestLoadSaveUIStateIncludesArchiveFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := UIState{
		Expanded:     map[string]bool{"draft": true},
		SelectedID:   "PRD-123",
		ShowArchived: true,
		LastAction: &LastAction{Type: "archive", ID: "PRD-123"},
	}
	if err := SaveUIState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadUIState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ShowArchived || loaded.LastAction == nil {
		t.Fatalf("expected archive fields persisted")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestLoadSaveUIStateIncludesArchiveFields`

Expected: FAIL (missing fields).

**Step 3: Write minimal implementation**

Update `internal/praude/project/project.go`:

```go
func ArchivedDir(root string) string { return filepath.Join(RootDir(root), "archived") }
func ArchivedSpecsDir(root string) string { return filepath.Join(ArchivedDir(root), "specs") }
func ArchivedResearchDir(root string) string { return filepath.Join(ArchivedDir(root), "research") }
func ArchivedSuggestionsDir(root string) string { return filepath.Join(ArchivedDir(root), "suggestions") }
func ArchivedBriefsDir(root string) string { return filepath.Join(ArchivedDir(root), "briefs") }

func TrashDir(root string) string { return filepath.Join(RootDir(root), "trash") }
func TrashSpecsDir(root string) string { return filepath.Join(TrashDir(root), "specs") }
func TrashResearchDir(root string) string { return filepath.Join(TrashDir(root), "research") }
func TrashSuggestionsDir(root string) string { return filepath.Join(TrashDir(root), "suggestions") }
func TrashBriefsDir(root string) string { return filepath.Join(TrashDir(root), "briefs") }
```

Update `internal/praude/tui/state_persist.go`:

```go
type LastAction struct {
	Type string   `json:"type"`
	ID   string   `json:"id"`
	From []string `json:"from"`
	To   []string `json:"to"`
}

type UIState struct {
	Expanded     map[string]bool `json:"expanded"`
	SelectedID   string          `json:"selected_id"`
	ShowArchived bool            `json:"show_archived"`
	LastAction   *LastAction     `json:"last_action"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestLoadSaveUIStateIncludesArchiveFields`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/project/project.go internal/praude/tui/state_persist.go internal/praude/tui/state_persist_test.go
git commit -m "Add archive/trash paths and UI state fields"
```

---

### Task 2: Implement archive/delete/undo engine

**Files:**
- Create: `internal/praude/archive/engine.go`
- Create: `internal/praude/archive/engine_test.go`
- Modify: `internal/praude/specs/load.go`

**Step 1: Write the failing test**

Create `internal/praude/archive/engine_test.go`:

```go
package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func TestArchiveMovesSpec(t *testing.T) {
	root := t.TempDir()
	_ = project.Init(root)
	src := filepath.Join(project.SpecsDir(root), "PRD-001.yaml")
	if err := os.WriteFile(src, []byte("id: \"PRD-001\"\nstatus: \"draft\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Archive(root, "PRD-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.To) == 0 {
		t.Fatalf("expected move paths")
	}
	if _, err := os.Stat(filepath.Join(project.ArchivedSpecsDir(root), "PRD-001.yaml")); err != nil {
		t.Fatalf("expected archived spec")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/archive -run TestArchiveMovesSpec`

Expected: FAIL (missing package/functions).

**Step 3: Write minimal implementation**

Create `internal/praude/archive/engine.go`:

```go
package archive

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

type Result struct {
	From []string
	To   []string
}

func Archive(root, id string) (Result, error) {
	return movePRD(root, id, true)
}

func Delete(root, id string) (Result, error) {
	return movePRD(root, id, false)
}

func Undo(root string, actionType string, from, to []string) error {
	for i := range to {
		if err := moveFile(to[i], from[i]); err != nil {
			return err
		}
	}
	return nil
}

func movePRD(root, id string, archived bool) (Result, error) {
	if id == "" {
		return Result{}, fmt.Errorf("missing id")
	}
	srcSpec := filepath.Join(project.SpecsDir(root), id+".yaml")
	dstSpec := filepath.Join(project.ArchivedSpecsDir(root), id+".yaml")
	if !archived {
		dstSpec = filepath.Join(project.TrashSpecsDir(root), id+".yaml")
	}
	if err := os.MkdirAll(filepath.Dir(dstSpec), 0o755); err != nil {
		return Result{}, err
	}
	if err := moveFile(srcSpec, dstSpec); err != nil {
		return Result{}, err
	}
	if archived {
		_ = specs.UpdateStatus(dstSpec, "archived")
	}
	return Result{From: []string{srcSpec}, To: []string{dstSpec}}, nil
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}
```

Update `internal/praude/specs/load.go` to allow loading archived specs when asked (add helper `LoadSummariesWithArchived` returning active + archived when flag true).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/archive -run TestArchiveMovesSpec`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/archive/engine.go internal/praude/archive/engine_test.go internal/praude/specs/load.go
git commit -m "Add archive/delete engine"
```

---

### Task 3: Wire archive/delete/undo into TUI

**Files:**
- Modify: `internal/praude/tui/model.go`
- Modify: `internal/praude/tui/overlay.go`
- Modify: `internal/praude/tui/state_persist.go`
- Test: `internal/praude/tui/archive_actions_test.go`

**Step 1: Write the failing test**

Create `internal/praude/tui/archive_actions_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func TestArchiveKeyMovesSpec(t *testing.T) {
	root := t.TempDir()
	_ = project.Init(root)
	src := filepath.Join(project.SpecsDir(root), "PRD-001.yaml")
	_ = os.WriteFile(src, []byte("id: \"PRD-001\"\nsummary: \"S\"\n"), 0o644)
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(root)
	m := NewModel()
	m = pressKey(m, "a")
	if _, err := os.Stat(filepath.Join(project.ArchivedSpecsDir(root), "PRD-001.yaml")); err != nil {
		t.Fatalf("expected archived spec")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/tui -run TestArchiveKeyMovesSpec`

Expected: FAIL.

**Step 3: Write minimal implementation**

Update `Model` to include:
- `showArchived bool`
- `confirmAction string` + `confirmMessage string`
- `lastAction *LastAction`

Add key handling:
- `a`: show confirm overlay for archive
- `d`: show confirm overlay for delete
- `u`: undo last action (confirm)
- `h`: toggle showArchived
- `enter`: confirm overlay
- `esc`: cancel overlay

On confirm, call `archive.Archive` or `archive.Delete`, update UI state + persist. For undo, call `archive.Undo` with recorded paths.

Update rendering to include archived items only when `showArchived` is true (use new summary loader).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/tui -run TestArchiveKeyMovesSpec`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/tui/model.go internal/praude/tui/overlay.go internal/praude/tui/state_persist.go internal/praude/tui/archive_actions_test.go
git commit -m "Add archive/delete/undo to Praude TUI"
```

---

### Task 4: Add CLI commands

**Files:**
- Create: `internal/praude/cli/commands/archive.go`
- Create: `internal/praude/cli/commands/delete.go`
- Create: `internal/praude/cli/commands/undo.go`
- Modify: `internal/praude/cli/root.go`
- Modify: `internal/praude/cli/commands/list.go`
- Test: `internal/praude/cli/commands/archive_test.go`

**Step 1: Write the failing test**

Create `internal/praude/cli/commands/archive_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
)

func TestArchiveCommandMovesSpec(t *testing.T) {
	root := t.TempDir()
	_ = project.Init(root)
	src := filepath.Join(project.SpecsDir(root), "PRD-001.yaml")
	_ = os.WriteFile(src, []byte("id: \"PRD-001\"\nsummary: \"S\"\n"), 0o644)
	err := archiveCmdRun(root, "PRD-001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(project.ArchivedSpecsDir(root), "PRD-001.yaml")); err != nil {
		t.Fatalf("expected archived spec")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/praude/cli/commands -run TestArchiveCommandMovesSpec`

Expected: FAIL.

**Step 3: Write minimal implementation**

Implement archive/delete/undo commands calling archive engine, and add `--include-archived` to list (load archived summaries when flag set).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/praude/cli/commands -run TestArchiveCommandMovesSpec`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude/cli/commands/archive.go internal/praude/cli/commands/delete.go internal/praude/cli/commands/undo.go internal/praude/cli/root.go internal/praude/cli/commands/list.go internal/praude/cli/commands/archive_test.go
git commit -m "Add Praude archive/delete/undo CLI"
```

---

### Task 5: Full test pass

**Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 2: Commit (if needed)**

```bash
git status --porcelain
# commit only if there are remaining changes
```

---

Plan complete and saved to `docs/plans/2026-01-22-praude-archive-delete-plan.md`.

Two execution options:
1) Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2) Parallel Session (separate) — New session uses superpowers:executing-plans to execute task-by-task

Which approach?
