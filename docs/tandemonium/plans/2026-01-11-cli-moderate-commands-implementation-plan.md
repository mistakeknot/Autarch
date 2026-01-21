# CLI Moderate Commands Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Implement "moderate" CLI behaviors for `status`, `doctor`, `recover`, and `cleanup` that report useful diagnostics without mutating state.

**Architecture:** Add small helper packages for project path discovery, SQLite diagnostics, and tmux session listing. CLI commands assemble these checks and print human-readable summaries; no modifications or destructive actions.

**Tech Stack:** Go 1.22+, SQLite (modernc.org/sqlite), tmux, git.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Project path discovery helpers

**Files:**
- Create: `internal/project/paths.go`
- Create: `internal/project/paths_test.go`

**Step 1: Write the failing test**

```go
package project

import (
    "os"
    "path/filepath"
    "testing"
)

func TestFindRoot(t *testing.T) {
    dir := t.TempDir()
    if err := os.MkdirAll(filepath.Join(dir, ".tandemonium"), 0o755); err != nil {
        t.Fatal(err)
    }
    got, err := FindRoot(dir)
    if err != nil {
        t.Fatalf("expected root, got error: %v", err)
    }
    if got != dir {
        t.Fatalf("expected %s, got %s", dir, got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/project -v`
Expected: FAIL with "undefined: FindRoot"

**Step 3: Write minimal implementation**

```go
package project

import (
    "errors"
    "os"
    "path/filepath"
)

var ErrNotInitialized = errors.New("not a Tandemonium project")

func FindRoot(start string) (string, error) {
    cur := start
    for {
        cand := filepath.Join(cur, ".tandemonium")
        if st, err := os.Stat(cand); err == nil && st.IsDir() {
            return cur, nil
        }
        parent := filepath.Dir(cur)
        if parent == cur {
            return "", ErrNotInitialized
        }
        cur = parent
    }
}

func StateDBPath(root string) string {
    return filepath.Join(root, ".tandemonium", "state.db")
}

func SpecsDir(root string) string {
    return filepath.Join(root, ".tandemonium", "specs")
}

func SessionsDir(root string) string {
    return filepath.Join(root, ".tandemonium", "sessions")
}

func WorktreesDir(root string) string {
    return filepath.Join(root, ".tandemonium", "worktrees")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/project -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/project/paths.go internal/project/paths_test.go
git commit -m "feat: add project path helpers"
```

---

### Task 2: SQLite diagnostics helpers

**Files:**
- Create: `internal/storage/diagnostics.go`
- Create: `internal/storage/diagnostics_test.go`

**Step 1: Write the failing test**

```go
package storage

import "testing"

func TestHasTasksTable(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }
    ok, err := HasTasksTable(db)
    if err != nil {
        t.Fatal(err)
    }
    if !ok {
        t.Fatal("expected tasks table")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`
Expected: FAIL with "undefined: HasTasksTable"

**Step 3: Write minimal implementation**

```go
package storage

import "database/sql"

func HasTasksTable(db *sql.DB) (bool, error) {
    row := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'`)
    var name string
    if err := row.Scan(&name); err != nil {
        if err == sql.ErrNoRows {
            return false, nil
        }
        return false, err
    }
    return name == "tasks", nil
}

func QuickCheck(db *sql.DB) (string, error) {
    row := db.QueryRow(`PRAGMA quick_check;`)
    var res string
    if err := row.Scan(&res); err != nil {
        return "", err
    }
    return res, nil
}

func CountTasksByStatus(db *sql.DB) (map[string]int, error) {
    rows, err := db.Query(`SELECT status, COUNT(*) FROM tasks GROUP BY status`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    counts := make(map[string]int)
    for rows.Next() {
        var status string
        var cnt int
        if err := rows.Scan(&status, &cnt); err != nil {
            return nil, err
        }
        counts[status] = cnt
    }
    return counts, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/diagnostics.go internal/storage/diagnostics_test.go
git commit -m "feat: add SQLite diagnostics helpers"
```

---

### Task 3: tmux session discovery helpers

**Files:**
- Create: `internal/tmux/list.go`
- Create: `internal/tmux/list_test.go`

**Step 1: Write the failing test**

```go
package tmux

import "testing"

func TestParseSessions(t *testing.T) {
    out := "tand-TAND-001: 1 windows (created Fri)\nother: 1 windows\n"
    got := ParseSessions(out, "tand-")
    if len(got) != 1 || got[0] != "tand-TAND-001" {
        t.Fatalf("unexpected sessions: %v", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux -v`
Expected: FAIL with "undefined: ParseSessions"

**Step 3: Write minimal implementation**

```go
package tmux

import (
    "os/exec"
    "strings"
)

func ListSessions(prefix string) ([]string, error) {
    out, err := exec.Command("tmux", "ls").CombinedOutput()
    if err != nil {
        // tmux ls exits non-zero when no server; return empty slice
        return []string{}, nil
    }
    return ParseSessions(string(out), prefix), nil
}

func ParseSessions(output, prefix string) []string {
    lines := strings.Split(output, "\n")
    var sessions []string
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        parts := strings.SplitN(line, ":", 2)
        name := strings.TrimSpace(parts[0])
        if strings.HasPrefix(name, prefix) {
            sessions = append(sessions, name)
        }
    }
    return sessions
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmux/list.go internal/tmux/list_test.go
git commit -m "feat: add tmux session discovery"
```

---

### Task 4: Implement `status` command output

**Files:**
- Modify: `internal/cli/commands/status.go`
- Create: `internal/cli/commands/status_output_test.go`

**Step 1: Write the failing test**

```go
package commands

import "testing"

func TestFormatStatusLines(t *testing.T) {
    lines := formatStatusLines(statusSummary{
        ProjectRoot: "/tmp/project",
        Initialized: true,
    })
    if len(lines) == 0 {
        t.Fatal("expected status lines")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL with "undefined: formatStatusLines"

**Step 3: Implement summary + formatter**

```go
package commands

import (
    "fmt"
    "os"

    "github.com/gensysven/tandemonium/internal/project"
    "github.com/gensysven/tandemonium/internal/storage"
    "github.com/gensysven/tandemonium/internal/tmux"
    "github.com/spf13/cobra"
)

type statusSummary struct {
    ProjectRoot string
    Initialized bool
    DBExists    bool
    TaskCounts  map[string]int
    Sessions    []string
}

func statusSummaryFromCwd() statusSummary {
    cwd, _ := os.Getwd()
    root, err := project.FindRoot(cwd)
    if err != nil {
        return statusSummary{Initialized: false}
    }
    sum := statusSummary{ProjectRoot: root, Initialized: true}
    if st, err := os.Stat(project.StateDBPath(root)); err == nil && !st.IsDir() {
        sum.DBExists = true
        if db, err := storage.Open(project.StateDBPath(root)); err == nil {
            if ok, _ := storage.HasTasksTable(db); ok {
                if counts, err := storage.CountTasksByStatus(db); err == nil {
                    sum.TaskCounts = counts
                }
            }
            db.Close()
        }
    }
    sum.Sessions, _ = tmux.ListSessions("tand-")
    return sum
}

func formatStatusLines(sum statusSummary) []string {
    if !sum.Initialized {
        return []string{"Not initialized (.tandemonium not found)"}
    }
    lines := []string{fmt.Sprintf("Project: %s", sum.ProjectRoot)}
    lines = append(lines, fmt.Sprintf("state.db: %v", sum.DBExists))
    if len(sum.TaskCounts) > 0 {
        lines = append(lines, fmt.Sprintf("tasks: %v", sum.TaskCounts))
    } else {
        lines = append(lines, "tasks: (none)")
    }
    lines = append(lines, fmt.Sprintf("tmux sessions: %d", len(sum.Sessions)))
    return lines
}

func StatusCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "status",
        Short: "Show quick project status",
        RunE: func(cmd *cobra.Command, args []string) error {
            sum := statusSummaryFromCwd()
            for _, line := range formatStatusLines(sum) {
                fmt.Fprintln(cmd.OutOrStdout(), line)
            }
            return nil
        },
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/status.go internal/cli/commands/status_output_test.go
git commit -m "feat: add status command output"
```

---

### Task 5: Implement `doctor`, `recover`, `cleanup` report-only outputs

**Files:**
- Modify: `internal/cli/commands/doctor.go`
- Modify: `internal/cli/commands/recover.go`
- Modify: `internal/cli/commands/cleanup.go`
- Create: `internal/cli/commands/doctor_output_test.go`

**Step 1: Write failing test**

```go
package commands

import "testing"

func TestFormatDoctorLines(t *testing.T) {
    lines := formatDoctorLines(doctorSummary{Initialized: false})
    if len(lines) == 0 {
        t.Fatal("expected doctor lines")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL with "undefined: formatDoctorLines"

**Step 3: Implement summaries + outputs**

```go
package commands

import (
    "fmt"
    "os"

    "github.com/gensysven/tandemonium/internal/project"
    "github.com/gensysven/tandemonium/internal/storage"
    "github.com/gensysven/tandemonium/internal/tmux"
    "github.com/spf13/cobra"
)

type doctorSummary struct {
    Initialized bool
    DBQuickCheck string
    Sessions []string
}

func doctorSummaryFromCwd() doctorSummary {
    cwd, _ := os.Getwd()
    root, err := project.FindRoot(cwd)
    if err != nil {
        return doctorSummary{Initialized: false}
    }
    sum := doctorSummary{Initialized: true}
    if db, err := storage.Open(project.StateDBPath(root)); err == nil {
        if res, err := storage.QuickCheck(db); err == nil {
            sum.DBQuickCheck = res
        }
        db.Close()
    }
    sum.Sessions, _ = tmux.ListSessions("tand-")
    return sum
}

func formatDoctorLines(sum doctorSummary) []string {
    if !sum.Initialized {
        return []string{"Not initialized (.tandemonium not found)"}
    }
    lines := []string{"Doctor checks:"}
    if sum.DBQuickCheck != "" {
        lines = append(lines, fmt.Sprintf("sqlite quick_check: %s", sum.DBQuickCheck))
    } else {
        lines = append(lines, "sqlite quick_check: (skipped)")
    }
    lines = append(lines, fmt.Sprintf("tmux sessions: %d", len(sum.Sessions)))
    return lines
}

func DoctorCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "doctor",
        Short: "Run integrity checks",
        RunE: func(cmd *cobra.Command, args []string) error {
            sum := doctorSummaryFromCwd()
            for _, line := range formatDoctorLines(sum) {
                fmt.Fprintln(cmd.OutOrStdout(), line)
            }
            return nil
        },
    }
}
```

`recover` and `cleanup` should print “would do” messages only, based on the same discovery helpers. Provide minimal lines for now (no mutations).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/doctor.go internal/cli/commands/recover.go internal/cli/commands/cleanup.go internal/cli/commands/doctor_output_test.go
git commit -m "feat: add doctor/recover/cleanup report outputs"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
