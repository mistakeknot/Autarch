# Drift Detection Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Detect file drift by comparing git‑changed files to spec `files_to_modify` and surface warnings.

**Architecture:** Parse minimal `files_to_modify` lists from spec YAMLs, use git diff name‑only to identify changed files, and compute drift warnings when changed files fall outside spec. This will be used in CLI review/doctor output (report‑only).

**Tech Stack:** Go 1.24+, git CLI, YAML.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Parse files_to_modify from specs

**Files:**
- Modify: `internal/specs/specs.go`
- Create: `internal/specs/files_test.go`

**Step 1: Write the failing test**

```go
package specs

import (
    "os"
    "path/filepath"
    "testing"
)

func TestParseFilesToModify(t *testing.T) {
    dir := t.TempDir()
    spec := []byte("id: TAND-001\nfiles_to_modify:\n  - a.txt\n  - b/c.txt\n")
    if err := os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), spec, 0o644); err != nil {
        t.Fatal(err)
    }
    summaries, _ := LoadSummaries(dir)
    if len(summaries[0].FilesToModify) != 2 {
        t.Fatal("expected files_to_modify")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`
Expected: FAIL (missing FilesToModify)

**Step 3: Extend SpecSummary + parsing**

Add `FilesToModify []string` to `SpecSummary` and parse from YAML.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/specs.go internal/specs/files_test.go
git commit -m "feat: parse files_to_modify"
```

---

### Task 2: Drift detection helper

**Files:**
- Create: `internal/drift/drift.go`
- Create: `internal/drift/drift_test.go`

**Step 1: Write failing test**

```go
package drift

import "testing"

func TestDetectDrift(t *testing.T) {
    spec := []string{"a.txt"}
    changed := []string{"a.txt", "b.txt"}
    drift := DetectDrift(spec, changed)
    if len(drift) != 1 || drift[0] != "b.txt" {
        t.Fatal("expected drift for b.txt")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/drift -v`
Expected: FAIL (missing DetectDrift)

**Step 3: Implement helper**

```go
func DetectDrift(allowed, changed []string) []string {
    // return changed files not in allowed list
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/drift -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/drift/drift.go internal/drift/drift_test.go
git commit -m "feat: add drift detection helper"
```

---

### Task 3: Surface drift warnings in doctor

**Files:**
- Modify: `internal/cli/commands/doctor.go`

**Step 1: Write failing test**

```go
package commands

import "testing"

func TestDoctorIncludesDriftLine(t *testing.T) {
    lines := formatDoctorLines(doctorSummary{Initialized: true, DriftFiles: []string{"b.txt"}})
    found := false
    for _, l := range lines {
        if l == "drift warnings: 1" { found = true }
    }
    if !found { t.Fatal("expected drift warnings line") }
}
```

**Step 2: Run test**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL

**Step 3: Implement output**

Add `DriftFiles []string` to `doctorSummary` and append a line with count if non‑empty.

**Step 4: Run test**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/doctor.go internal/cli/commands/doctor_output_test.go

git commit -m "feat: add drift warnings to doctor"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
