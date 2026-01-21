# Drift Detection + Git Diff Wiring Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Wire drift detection to real git diffs per task and surface results in CLI review/doctor output.

**Architecture:** Add a git helper to run `git diff --name-only` for a task branch, parse output, and compare against spec `files_to_modify` for that task ID. Update CLI to include drift counts using this real data (still report-only).

**Tech Stack:** Go 1.24+, git CLI, YAML.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Git diff name-only runner

**Files:**
- Create: `internal/git/diff_runner.go`
- Create: `internal/git/diff_runner_test.go`

**Step 1: Write the failing test**

```go
package git

import "testing"

type fakeRunner struct{ output string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
    return f.output, nil
}

func TestDiffNameOnly(t *testing.T) {
    r := &fakeRunner{output: "a.txt\nb.txt\n"}
    files, err := DiffNameOnly(r, "HEAD")
    if err != nil || len(files) != 2 {
        t.Fatal("expected 2 files")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git -v`
Expected: FAIL with "undefined: DiffNameOnly"

**Step 3: Implement runner**

```go
package git

import "os/exec"

type Runner interface { Run(name string, args ...string) (string, error) }

type ExecRunner struct{}

func (e *ExecRunner) Run(name string, args ...string) (string, error) {
    out, err := exec.Command(name, args...).CombinedOutput()
    return string(out), err
}

func DiffNameOnly(r Runner, rev string) ([]string, error) {
    out, err := r.Run("git", "diff", "--name-only", rev)
    if err != nil {
        return nil, err
    }
    return ParseNameOnly(out), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/diff_runner.go internal/git/diff_runner_test.go
git commit -m "feat: add git diff name-only runner"
```

---

### Task 2: Spec lookup by task ID

**Files:**
- Modify: `internal/specs/specs.go`
- Create: `internal/specs/lookup_test.go`

**Step 1: Write failing test**

```go
package specs

import (
    "os"
    "path/filepath"
    "testing"
)

func TestFindByID(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), []byte("id: TAND-001\nfiles_to_modify:\n  - a.txt\n"), 0o644)
    summaries, _ := LoadSummaries(dir)
    s, ok := FindByID(summaries, "TAND-001")
    if !ok || len(s.FilesToModify) != 1 {
        t.Fatal("expected match")
    }
}
```

**Step 2: Run test**

Run: `go test ./internal/specs -v`
Expected: FAIL

**Step 3: Implement helper**

```go
func FindByID(list []SpecSummary, id string) (SpecSummary, bool) {
    for _, s := range list { if s.ID == id { return s, true } }
    return SpecSummary{}, false
}
```

**Step 4: Run test**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/specs.go internal/specs/lookup_test.go
git commit -m "feat: add spec lookup by id"
```

---

### Task 3: Drift check using real git diff in doctor

**Files:**
- Modify: `internal/cli/commands/doctor.go`
- Create: `internal/cli/commands/doctor_drift_test.go`

**Step 1: Write failing test**

```go
package commands

import "testing"

func TestDoctorAddsDriftCount(t *testing.T) {
    lines := formatDoctorLines(doctorSummary{Initialized: true, DriftFiles: []string{"x.go"}})
    found := false
    for _, l := range lines {
        if l == "drift warnings: 1" { found = true }
    }
    if !found { t.Fatal("expected drift warnings line") }
}
```

**Step 2: Run test**

Run: `go test ./internal/cli/commands -v`
Expected: PASS (already exists) – if already covered, skip this test.

**Step 3: Add real drift calculation path**

In `doctorSummaryFromCwd`, after loading spec summaries, run `git diff --name-only` and compare with `files_to_modify` for each spec’s ID; collect drift files and attach to summary.

**Step 4: Run tests**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/doctor.go

git commit -m "feat: wire drift checks to git diff"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
