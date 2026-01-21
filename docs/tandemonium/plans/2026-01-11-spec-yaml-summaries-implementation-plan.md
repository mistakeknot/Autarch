# Spec YAML Summaries Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** TBD (no Task Master item provided)

**Goal:** Add hybrid YAML spec parsing (id/title/status) and integrate it into CLI diagnostics for better reporting.

**Architecture:** Introduce a small `internal/specs` helper that reads `.tandemonium/specs/*.yaml`, extracts `id`, `title`, `status` (hybrid ID resolution), and returns summaries plus warnings. CLI commands use this data for counts and diagnostics without mutating state.

**Tech Stack:** Go 1.22+, gopkg.in/yaml.v3.

**Constraint:** User requested no git worktrees for implementation (work in-place).

---

### Task 1: Spec summary parser (hybrid ID)

**Files:**
- Create: `internal/specs/specs.go`
- Create: `internal/specs/specs_test.go`

**Step 1: Write the failing test**

```go
package specs

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadSummariesHybridID(t *testing.T) {
    dir := t.TempDir()
    spec1 := []byte("id: TAND-001\ntitle: Test One\nstatus: ready\n")
    spec2 := []byte("title: No ID\nstatus: draft\n")

    if err := os.WriteFile(filepath.Join(dir, "TAND-001.yaml"), spec1, 0o644); err != nil {
        t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(dir, "TAND-002.yaml"), spec2, 0o644); err != nil {
        t.Fatal(err)
    }

    summaries, warnings := LoadSummaries(dir)
    if len(summaries) != 2 {
        t.Fatalf("expected 2 summaries, got %d", len(summaries))
    }
    if summaries[1].ID != "TAND-002" {
        t.Fatalf("expected filename fallback ID, got %s", summaries[1].ID)
    }
    if len(warnings) == 0 {
        t.Fatal("expected warnings for missing id")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`
Expected: FAIL with "undefined: LoadSummaries"

**Step 3: Write minimal implementation**

```go
package specs

import (
    "path/filepath"
    "strings"

    "gopkg.in/yaml.v3"
    "os"
)

type SpecSummary struct {
    ID     string
    Title  string
    Status string
    Path   string
}

type specDoc struct {
    ID     string `yaml:"id"`
    Title  string `yaml:"title"`
    Status string `yaml:"status"`
}

func LoadSummaries(dir string) ([]SpecSummary, []string) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return []SpecSummary{}, []string{}
    }
    var summaries []SpecSummary
    var warnings []string
    for _, e := range entries {
        if e.IsDir() {
            continue
        }
        name := e.Name()
        if !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
            continue
        }
        path := filepath.Join(dir, name)
        raw, err := os.ReadFile(path)
        if err != nil {
            warnings = append(warnings, "read failed: "+path)
            continue
        }
        var doc specDoc
        if err := yaml.Unmarshal(raw, &doc); err != nil {
            warnings = append(warnings, "parse failed: "+path)
            continue
        }
        id := doc.ID
        if id == "" {
            id = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
            warnings = append(warnings, "missing id: "+path)
        }
        if doc.Title == "" {
            warnings = append(warnings, "missing title: "+path)
        }
        if doc.Status == "" {
            warnings = append(warnings, "missing status: "+path)
        }
        summaries = append(summaries, SpecSummary{ID: id, Title: doc.Title, Status: doc.Status, Path: path})
    }
    return summaries, warnings
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/specs.go internal/specs/specs_test.go
git commit -m "feat: add spec summary parser"
```

---

### Task 2: Integrate spec summaries into `status`

**Files:**
- Modify: `internal/cli/commands/status.go`
- Create: `internal/cli/commands/status_specs_test.go`

**Step 1: Write the failing test**

```go
package commands

import "testing"

func TestSummariesToCounts(t *testing.T) {
    counts := summariesToCounts([]specSummary{
        {Status: "ready"},
        {Status: "draft"},
        {Status: ""},
    })
    if counts["ready"] != 1 || counts["draft"] != 1 || counts["unknown"] != 1 {
        t.Fatalf("unexpected counts: %v", counts)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL with "undefined: summariesToCounts"

**Step 3: Implement fallback to specs**

Add helper to convert summaries to status counts when DB has no tasks table. Use `internal/specs` to load summaries from `.tandemonium/specs` and add a line like `specs: N (warnings: M)`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/status.go internal/cli/commands/status_specs_test.go
git commit -m "feat: add spec-based status fallback"
```

---

### Task 3: Surface spec warnings in `doctor`

**Files:**
- Modify: `internal/cli/commands/doctor.go`

**Step 1: Write the failing test**

```go
package commands

import "testing"

func TestDoctorIncludesWarnings(t *testing.T) {
    lines := formatDoctorLines(doctorSummary{Initialized: true, SpecWarnings: []string{"missing id"}})
    found := false
    for _, l := range lines {
        if l == "spec warnings: 1" {
            found = true
        }
    }
    if !found {
        t.Fatal("expected spec warnings line")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`
Expected: FAIL with "unknown field SpecWarnings"

**Step 3: Implement summary + output**

Extend `doctorSummary` with `SpecWarnings []string` and load summaries from `.tandemonium/specs`, appending warnings line in output.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/doctor.go internal/cli/commands/doctor_output_test.go

git commit -m "feat: add spec warnings to doctor"
```

---

## Verification

Run:

```bash
go test ./...
```

Expected: PASS
