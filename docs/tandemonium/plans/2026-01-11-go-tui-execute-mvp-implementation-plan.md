# Go TUI Execute-Only MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Replace the Rust/Tauri codebase with a Go TUI-first, execute-only MVP that matches the Go/TOML spec.

**Architecture:** A single Go module with a Bubble Tea TUI, SQLite (WAL) runtime state, YAML specs in git, tmux-backed agent sessions, and minimal CLI bootstrap/diagnostics. All state lives in `.tandemonium/` and recovery rebuilds from specs and tmux sessions.

**Tech Stack:** Go 1.22+, Bubble Tea + Lip Gloss, modernc.org/sqlite, BurntSushi/toml, gopkg.in/yaml.v3, git CLI, tmux.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Create Go module and base entrypoint

**Files:**
- Create: `go.mod`
- Create: `cmd/tandemonium/main.go`
- Create: `internal/cli/root.go`

**Step 1: Create go.mod**

```go
module github.com/gensysven/tandemonium

go 1.22

require (
    github.com/BurntSushi/toml v1.4.0
    github.com/charmbracelet/bubbletea v0.26.6
    github.com/charmbracelet/lipgloss v0.11.0
    github.com/spf13/cobra v1.8.1
    gopkg.in/yaml.v3 v3.0.1
    modernc.org/sqlite v1.30.0
)
```

**Step 2: Create CLI entrypoint**

```go
package main

import (
    "os"

    "github.com/gensysven/tandemonium/internal/cli"
)

func main() {
    if err := cli.Execute(); err != nil {
        // cobra already prints; just exit non-zero
        os.Exit(1)
    }
}
```

**Step 3: Create CLI root command**

```go
package cli

import "github.com/spf13/cobra"

func Execute() error {
    root := &cobra.Command{
        Use:   "tandemonium",
        Short: "Task orchestration for human-AI collaboration",
        RunE: func(cmd *cobra.Command, args []string) error {
            // TUI launch stub for now
            return nil
        },
    }
    return root.Execute()
}
```

**Step 4: Run format**

Run: `gofmt -w cmd/tandemonium/main.go internal/cli/root.go`

Expected: files formatted

**Step 5: Commit**

```bash
git add go.mod cmd/tandemonium/main.go internal/cli/root.go
git commit -m "chore: scaffold Go module and CLI entrypoint"
```

---

### Task 2: Config loading (TOML) with layered overrides

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write failing test**

```go
package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadProjectConfig(t *testing.T) {
    dir := t.TempDir()
    cfgDir := filepath.Join(dir, ".tandemonium")
    if err := os.MkdirAll(cfgDir, 0o755); err != nil {
        t.Fatal(err)
    }
    cfgPath := filepath.Join(cfgDir, "config.toml")
    if err := os.WriteFile(cfgPath, []byte(`
[general]
max_agents = 3
`), 0o644); err != nil {
        t.Fatal(err)
    }

    cfg, err := LoadFromProject(dir)
    if err != nil {
        t.Fatalf("load failed: %v", err)
    }
    if cfg.General.MaxAgents != 3 {
        t.Fatalf("expected max_agents=3, got %d", cfg.General.MaxAgents)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -v`

Expected: FAIL with "undefined: LoadFromProject"

**Step 3: Write minimal implementation**

```go
package config

import (
    "errors"
    "os"
    "path/filepath"

    "github.com/BurntSushi/toml"
)

type GeneralConfig struct {
    MaxAgents int `toml:"max_agents"`
}

type Config struct {
    General GeneralConfig `toml:"general"`
}

func defaultConfig() Config {
    return Config{General: GeneralConfig{MaxAgents: 4}}
}

func LoadFromProject(projectDir string) (Config, error) {
    cfg := defaultConfig()
    path := filepath.Join(projectDir, ".tandemonium", "config.toml")
    if _, err := os.Stat(path); err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return cfg, nil
        }
        return Config{}, err
    }
    if _, err := toml.DecodeFile(path, &cfg); err != nil {
        return Config{}, err
    }
    return cfg, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add TOML config loader"
```

---

### Task 3: Project initialization (.tandemonium/ layout)

**Files:**
- Create: `internal/project/init.go`
- Create: `internal/project/init_test.go`
- Modify: `internal/cli/root.go`

**Step 1: Write failing test**

```go
package project

import (
    "os"
    "path/filepath"
    "testing"
)

func TestInitProjectCreatesLayout(t *testing.T) {
    dir := t.TempDir()
    if err := Init(dir); err != nil {
        t.Fatalf("init failed: %v", err)
    }
    want := []string{
        ".tandemonium",
        ".tandemonium/specs",
        ".tandemonium/sessions",
    }
    for _, p := range want {
        if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
            t.Fatalf("missing %s: %v", p, err)
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/project -v`

Expected: FAIL with "undefined: Init"

**Step 3: Write minimal implementation**

```go
package project

import (
    "os"
    "path/filepath"
)

func Init(projectDir string) error {
    dirs := []string{
        filepath.Join(projectDir, ".tandemonium"),
        filepath.Join(projectDir, ".tandemonium", "specs"),
        filepath.Join(projectDir, ".tandemonium", "sessions"),
    }
    for _, d := range dirs {
        if err := os.MkdirAll(d, 0o755); err != nil {
            return err
        }
    }
    return nil
}
```

**Step 4: Wire CLI init command**

```go
root.AddCommand(&cobra.Command{
    Use:   "init",
    Short: "Initialize .tandemonium in current directory",
    RunE: func(cmd *cobra.Command, args []string) error {
        return project.Init(".")
    },
})
```

**Step 5: Run tests**

Run: `go test ./internal/project -v`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/project/init.go internal/project/init_test.go internal/cli/root.go
git commit -m "feat: initialize .tandemonium layout"
```

---

### Task 4: SQLite schema + basic task storage

**Files:**
- Create: `internal/storage/db.go`
- Create: `internal/storage/task.go`
- Create: `internal/storage/db_test.go`

**Step 1: Write failing test**

```go
package storage

import "testing"

func TestCreateAndReadTask(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }

    task := Task{ID: "TAND-001", Title: "Test", Status: "todo"}
    if err := InsertTask(db, task); err != nil {
        t.Fatal(err)
    }

    got, err := GetTask(db, "TAND-001")
    if err != nil {
        t.Fatal(err)
    }
    if got.Title != "Test" {
        t.Fatalf("expected title Test, got %s", got.Title)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`

Expected: FAIL with "undefined: OpenTemp"

**Step 3: Implement storage**

```go
package storage

import (
    "database/sql"
    "os"
    "path/filepath"

    _ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
    return sql.Open("sqlite", path)
}

func OpenTemp() (*sql.DB, error) {
    dir, err := os.MkdirTemp("", "tandemonium-db-")
    if err != nil {
        return nil, err
    }
    return Open(filepath.Join(dir, "state.db"))
}

func Migrate(db *sql.DB) error {
    _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL
);
`)
    return err
}
```

```go
package storage

import "database/sql"

type Task struct {
    ID     string
    Title  string
    Status string
}

func InsertTask(db *sql.DB, t Task) error {
    _, err := db.Exec(`INSERT INTO tasks (id, title, status) VALUES (?, ?, ?)`, t.ID, t.Title, t.Status)
    return err
}

func GetTask(db *sql.DB, id string) (Task, error) {
    row := db.QueryRow(`SELECT id, title, status FROM tasks WHERE id = ?`, id)
    var t Task
    if err := row.Scan(&t.ID, &t.Title, &t.Status); err != nil {
        return Task{}, err
    }
    return t, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/db.go internal/storage/task.go internal/storage/db_test.go
git commit -m "feat: add SQLite task storage"
```

---

### Task 5: Git worktree operations (for task execution)

**Files:**
- Create: `internal/git/worktree.go`
- Create: `internal/git/worktree_test.go`

**Step 1: Write failing test**

```go
package git

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

func TestCreateWorktree(t *testing.T) {
    dir := t.TempDir()
    cmd := exec.Command("git", "init")
    cmd.Dir = dir
    if err := cmd.Run(); err != nil {
        t.Fatalf("git init failed: %v", err)
    }

    wtPath := filepath.Join(dir, "wt")
    if err := CreateWorktree(dir, wtPath, "feature/test"); err != nil {
        t.Fatalf("create worktree failed: %v", err)
    }
    if _, err := os.Stat(wtPath); err != nil {
        t.Fatalf("missing worktree: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git -v`

Expected: FAIL with "undefined: CreateWorktree"

**Step 3: Implement worktree creation**

```go
package git

import (
    "os/exec"
)

func CreateWorktree(repoDir, path, branch string) error {
    cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
    cmd.Dir = repoDir
    return cmd.Run()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/worktree.go internal/git/worktree_test.go
git commit -m "feat: add git worktree creation"
```

---

### Task 6: tmux session lifecycle + log streaming contract

**Files:**
- Create: `internal/tmux/session.go`
- Create: `internal/tmux/stream.go`
- Create: `internal/tmux/session_test.go`

**Step 1: Write failing test**

```go
package tmux

import "testing"

type fakeRunner struct{ cmds [][]string }

func (f *fakeRunner) Run(name string, args ...string) error {
    f.cmds = append(f.cmds, append([]string{name}, args...))
    return nil
}

func TestStartSessionBuildsCommands(t *testing.T) {
    r := &fakeRunner{}
    s := Session{ID: "tand-TAND-001", Workdir: "/tmp/x", LogPath: "/tmp/log"}
    if err := StartSession(r, s); err != nil {
        t.Fatal(err)
    }
    if len(r.cmds) < 2 {
        t.Fatalf("expected commands, got %d", len(r.cmds))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux -v`

Expected: FAIL with "undefined: Session"

**Step 3: Implement session control**

```go
package tmux

type Runner interface {
    Run(name string, args ...string) error
}

type Session struct {
    ID      string
    Workdir string
    LogPath string
}

func StartSession(r Runner, s Session) error {
    if err := r.Run("tmux", "new-session", "-d", "-s", s.ID, "-c", s.Workdir); err != nil {
        return err
    }
    return r.Run("tmux", "pipe-pane", "-t", s.ID, "-o", "cat >> "+s.LogPath)
}
```

**Step 4: Implement log streaming helper**

```go
package tmux

import (
    "bufio"
    "os"
)

func TailLines(path string, fn func(line string)) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        fn(scanner.Text())
    }
    return scanner.Err()
}
```

**Step 5: Run tests**

Run: `go test ./internal/tmux -v`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/tmux/session.go internal/tmux/stream.go internal/tmux/session_test.go
git commit -m "feat: add tmux session control and streaming"
```

---

### Task 7: Agent detection (completion + blocker)

**Files:**
- Create: `internal/agent/detect.go`
- Create: `internal/agent/detect_test.go`

**Step 1: Write failing test**

```go
package agent

import "testing"

func TestDetectCompletion(t *testing.T) {
    state := DetectState("Done. All tests pass.")
    if state != "done" {
        t.Fatalf("expected done, got %s", state)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`

Expected: FAIL with "undefined: DetectState"

**Step 3: Implement detection**

```go
package agent

import "strings"

func DetectState(line string) string {
    l := strings.ToLower(line)
    if strings.Contains(l, "done") || strings.Contains(l, "complete") {
        return "done"
    }
    if strings.Contains(l, "blocked") || strings.Contains(l, "waiting") {
        return "blocked"
    }
    return "working"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/detect.go internal/agent/detect_test.go
git commit -m "feat: add basic agent state detection"
```

---

### Task 8: TUI shell and Fleet view (Bubble Tea)

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/fleet_view.go`
- Modify: `internal/cli/root.go`

**Step 1: Write failing test**

```go
package tui

import "testing"

func TestInitialModelHasTitle(t *testing.T) {
    m := NewModel()
    if m.Title == "" {
        t.Fatal("expected title")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`

Expected: FAIL with "undefined: NewModel"

**Step 3: Implement minimal model + view**

```go
package tui

import (
    "github.com/charmbracelet/bubbletea"
)

type Model struct {
    Title string
}

func NewModel() Model {
    return Model{Title: "Tandemonium"}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    return m, nil
}

func (m Model) View() string {
    return m.Title + "\n\nFleet view (stub)"
}
```

**Step 4: Wire CLI to launch TUI**

```go
import (
    "github.com/charmbracelet/bubbletea"
    "github.com/gensysven/tandemonium/internal/tui"
)

root.RunE = func(cmd *cobra.Command, args []string) error {
    p := tea.NewProgram(tui.NewModel())
    _, err := p.Run()
    return err
}
```

**Step 5: Run tests**

Run: `go test ./internal/tui -v`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/fleet_view.go internal/cli/root.go
git commit -m "feat: add TUI shell and fleet view stub"
```

---

### Task 9: Review queue model (data only)

**Files:**
- Create: `internal/review/queue.go`
- Create: `internal/review/queue_test.go`

**Step 1: Write failing test**

```go
package review

import "testing"

func TestQueueAdd(t *testing.T) {
    q := NewQueue()
    q.Add("TAND-001")
    if q.Len() != 1 {
        t.Fatalf("expected len 1, got %d", q.Len())
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/review -v`

Expected: FAIL with "undefined: NewQueue"

**Step 3: Implement queue**

```go
package review

type Queue struct {
    ids []string
}

func NewQueue() *Queue { return &Queue{ids: []string{}} }

func (q *Queue) Add(id string) { q.ids = append(q.ids, id) }

func (q *Queue) Len() int { return len(q.ids) }
```

**Step 4: Run tests**

Run: `go test ./internal/review -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/review/queue.go internal/review/queue_test.go
git commit -m "feat: add review queue data model"
```

---

### Task 10: Minimal CLI commands (status/doctor/recover/cleanup)

**Files:**
- Modify: `internal/cli/root.go`
- Create: `internal/cli/commands/status.go`
- Create: `internal/cli/commands/doctor.go`
- Create: `internal/cli/commands/recover.go`
- Create: `internal/cli/commands/cleanup.go`

**Step 1: Write failing test**

```go
package commands

import "testing"

func TestStatusCommand(t *testing.T) {
    if StatusCmd().Use != "status" {
        t.Fatalf("unexpected Use")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`

Expected: FAIL with "undefined: StatusCmd"

**Step 3: Implement command factories**

```go
package commands

import "github.com/spf13/cobra"

func StatusCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show quick project status",
        RunE: func(cmd *cobra.Command, args []string) error { return nil },
    }
}
```

Repeat for `doctor`, `recover`, and `cleanup` with stub RunE returning nil.

**Step 4: Wire commands into root**

```go
root.AddCommand(
    commands.StatusCmd(),
    commands.DoctorCmd(),
    commands.RecoverCmd(),
    commands.CleanupCmd(),
)
```

**Step 5: Run tests**

Run: `go test ./internal/cli/commands -v`

Expected: PASS

**Step 6: Commit**

```bash
git add internal/cli/root.go internal/cli/commands/*.go internal/cli/commands/status_test.go
git commit -m "feat: add minimal CLI command stubs"
```

---

### Task 11: Remove Rust/Tauri code and update docs

**Files:**
- Delete: `Cargo.toml`
- Delete: `Cargo.lock`
- Delete: `crates/`
- Delete: `app/`
- Modify: `README.md`

**Step 1: Update README to Go/TUI**

Replace Rust/Tauri references with Go + Bubble Tea, TOML config path, and new CLI usage.

**Step 2: Remove Rust/Tauri artifacts**

Run:

```bash
rm -rf crates app Cargo.toml Cargo.lock
```

**Step 3: Run quick verification**

Run: `go test ./...`

Expected: PASS

**Step 4: Commit**

```bash
git add README.md
git rm -r crates app Cargo.toml Cargo.lock
git commit -m "chore: remove Rust/Tauri code and update docs"
```

---

## Verification (end-to-end)

Run:

```bash
go test ./...
```

Expected: PASS

Optionally:

```bash
go vet ./...
```

Expected: no issues
