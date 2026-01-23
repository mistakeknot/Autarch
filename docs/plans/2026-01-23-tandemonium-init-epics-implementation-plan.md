# Tandemonium Init Epic/Story Generation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `Vauxpraudemonium-i6o (Task reference)`

**Goal:** Replace `tandemonium init` vision/MVP prompts with an agent-assisted repo scan that generates epic + story specs and keeps them updated via on-demand and background scans.

**Architecture:** Add an exploration pipeline that scans repo docs/code/tests and writes `.tandemonium/plan/exploration.md`, then a generator layer that uses the shared run-target registry (default `claude`) to produce epics/stories. Emit specs into `.tandemonium/specs/EPIC-###.yaml` with progress indicators, and add CLI + TUI hooks for scan intervals and commit detection. Prompt for exploration depth (1/2/3), and respect rerun behavior flags with a non-interactive default of `skip-existing`.

**Tech Stack:** Go, TOML, YAML (`gopkg.in/yaml.v3`), Bubble Tea.

**Worktree:** User requested no worktree; execute on the current branch.

---

### Task 1: Epic/Story spec types + writer

**Files:**
- Create: `internal/tandemonium/epics/types.go`
- Create: `internal/tandemonium/epics/write.go`
- Test: `internal/tandemonium/epics/write_test.go`

**Step 1: Write failing test (writes EPIC-###.yaml and story IDs)**

```go
func TestWriteEpicsCreatesFiles(t *testing.T) {
    dir := t.TempDir()
    epics := []Epic{
        {
            ID:       "EPIC-001",
            Title:    "Auth",
            Summary:  "User auth and sessions",
            Status:   StatusTodo,
            Priority: PriorityP1,
            AcceptanceCriteria: []string{"Login works"},
            Risks:    []string{"OAuth latency"},
            Estimate: "M",
            Stories: []Story{
                {
                    ID:       "EPIC-001-S01",
                    Title:    "Login form",
                    Summary:  "Email/password flow",
                    Status:   StatusTodo,
                    Priority: PriorityP1,
                    Estimate: "S",
                },
            },
        },
    }

    if err := WriteEpics(dir, epics, WriteOptions{Existing: ExistingOverwrite}); err != nil {
        t.Fatal(err)
    }

    if _, err := os.Stat(filepath.Join(dir, "EPIC-001.yaml")); err != nil {
        t.Fatalf("expected epic file: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/epics -run TestWriteEpicsCreatesFiles`

Expected: FAIL (WriteEpics undefined).

**Step 3: Implement minimal types + writer**

```go
// types.go
package epics

type Status string
const (
    StatusTodo       Status = "todo"
    StatusInProgress Status = "in_progress"
    StatusReview     Status = "review"
    StatusBlocked    Status = "blocked"
    StatusDone       Status = "done"
)

type Priority string
const (
    PriorityP0 Priority = "p0"
    PriorityP1 Priority = "p1"
    PriorityP2 Priority = "p2"
    PriorityP3 Priority = "p3"
)

type Story struct {
    ID       string   `yaml:"id"`
    Title    string   `yaml:"title"`
    Summary  string   `yaml:"summary,omitempty"`
    Status   Status   `yaml:"status"`
    Priority Priority `yaml:"priority"`
    AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
    Risks    []string `yaml:"risks,omitempty"`
    Estimate string   `yaml:"estimate,omitempty"`
}

type Epic struct {
    ID       string   `yaml:"id"`
    Title    string   `yaml:"title"`
    Summary  string   `yaml:"summary,omitempty"`
    Status   Status   `yaml:"status"`
    Priority Priority `yaml:"priority"`
    AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
    Risks    []string `yaml:"risks,omitempty"`
    Estimate string   `yaml:"estimate,omitempty"`
    Stories  []Story  `yaml:"stories,omitempty"`
}

// write.go
package epics

type ExistingMode string
const (
    ExistingSkip      ExistingMode = "skip"
    ExistingOverwrite ExistingMode = "overwrite"
)

type WriteOptions struct {
    Existing ExistingMode
}
```

```go
func WriteEpics(dir string, epics []Epic, opts WriteOptions) error {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return fmt.Errorf("mkdir epics dir: %w", err)
    }
    for _, epic := range epics {
        path := filepath.Join(dir, fmt.Sprintf("%s.yaml", epic.ID))
        if _, err := os.Stat(path); err == nil && opts.Existing == ExistingSkip {
            continue
        }
        data, err := yaml.Marshal(epic)
        if err != nil {
            return fmt.Errorf("marshal epic %s: %w", epic.ID, err)
        }
        if err := os.WriteFile(path, data, 0o644); err != nil {
            return fmt.Errorf("write epic %s: %w", epic.ID, err)
        }
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/epics -run TestWriteEpicsCreatesFiles`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/epics/*
git commit -m "feat(tandemonium): add epic spec writer"
```

---

### Task 2: Exploration pipeline + summary

**Files:**
- Create: `internal/tandemonium/explore/explore.go`
- Create: `internal/tandemonium/explore/scanners.go`
- Test: `internal/tandemonium/explore/explore_test.go`

**Step 1: Write failing test (writes exploration summary)**

```go
func TestExploreWritesSummary(t *testing.T) {
    root := t.TempDir()
    planDir := filepath.Join(root, ".tandemonium", "plan")
    if err := os.MkdirAll(planDir, 0o755); err != nil {
        t.Fatal(err)
    }

    out, err := Run(root, planDir, Options{EmitProgress: func(string){}})
    if err != nil {
        t.Fatal(err)
    }
    if out.SummaryPath == "" {
        t.Fatalf("expected summary path")
    }
    if _, err := os.Stat(out.SummaryPath); err != nil {
        t.Fatalf("expected summary file: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/explore -run TestExploreWritesSummary`

Expected: FAIL (Run undefined).

**Step 3: Implement minimal exploration**

```go
// explore.go
package explore

type Options struct {
    EmitProgress func(string)
    Depth        int // 1, 2, or 3
}

type Output struct {
    SummaryPath string
    Summary     string
}

func Run(root, planDir string, opts Options) (Output, error) {
    emit := opts.EmitProgress
    if emit == nil {
        emit = func(string) {}
    }
    emit("Scanning docs")
    docs := scanDocs(root)
    emit("Scanning code")
    code := scanCode(root)
    emit("Scanning tests")
    tests := scanTests(root)

    summary := buildSummary(docs, code, tests, opts.Depth)
    path := filepath.Join(planDir, "exploration.md")
    if err := os.MkdirAll(planDir, 0o755); err != nil {
        return Output{}, fmt.Errorf("mkdir plan: %w", err)
    }
    if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
        return Output{}, fmt.Errorf("write summary: %w", err)
    }
    return Output{SummaryPath: path, Summary: summary}, nil
}
```

```go
// scanners.go
func scanDocs(root string) []string { return findByExt(root, []string{".md", ".mdx", ".rst"}) }
func scanCode(root string) []string { return findByExt(root, []string{".go", ".ts", ".tsx", ".js"}) }
func scanTests(root string) []string { return findBySuffix(root, []string{"_test.go", ".spec.ts", ".test.ts"}) }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/explore -run TestExploreWritesSummary`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/explore/*
git commit -m "feat(tandemonium): add exploration pipeline"
```

---

### Task 3: Agent generator + prompt builder

**Files:**
- Create: `internal/tandemonium/initflow/generate.go`
- Test: `internal/tandemonium/initflow/generate_test.go`

**Step 1: Write failing test (fallback on agent error)**

```go
func TestGenerateEpicsFallsBackOnError(t *testing.T) {
    gen := &FakeGenerator{Err: errors.New("boom")}
    out, err := GenerateEpics(gen, Input{Summary: "summary"})
    if err != nil {
        t.Fatal(err)
    }
    if len(out.Epics) == 0 {
        t.Fatalf("expected fallback epics")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/initflow -run TestGenerateEpicsFallsBackOnError`

Expected: FAIL (GenerateEpics undefined).

**Step 3: Implement generator abstraction + fallback**

```go
// generate.go
package initflow

type Input struct {
    Summary string
    Depth   int
    Repo    string
}

type Result struct {
    Epics []epics.Epic
}

type Generator interface {
    Generate(ctx context.Context, input Input) (Result, error)
}

type Prompt struct {
    Text string
}

func BuildPrompt(input Input) Prompt {
    return Prompt{Text: fmt.Sprintf("Summary:\n%s\nDepth:%d\nRepo:%s\n", input.Summary, input.Depth, input.Repo)}
}

func GenerateEpics(gen Generator, input Input) (Result, error) {
    out, err := gen.Generate(context.Background(), input)
    if err == nil && len(out.Epics) > 0 {
        return out, nil
    }
    fallback := epics.Epic{
        ID:       "EPIC-001",
        Title:    "Initial backlog",
        Status:   epics.StatusTodo,
        Priority: epics.PriorityP2,
        Stories: []epics.Story{
            {ID: "EPIC-001-S01", Title: "Inventory existing tasks", Status: epics.StatusTodo, Priority: epics.PriorityP2},
        },
    }
    return Result{Epics: []epics.Epic{fallback}}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/initflow -run TestGenerateEpicsFallsBackOnError`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/initflow/*
git commit -m "feat(tandemonium): add epic generator abstraction"
```

---

### Task 4: CLI init + scan command

**Files:**
- Modify: `internal/tandemonium/cli/root.go`
- Create: `internal/tandemonium/cli/commands/scan.go`
- Test: `internal/tandemonium/cli/commands/scan_test.go`

**Step 1: Write failing test (scan command writes summary)**

```go
func TestScanCommandWritesSummary(t *testing.T) {
    root := t.TempDir()
    _ = project.Init(root)

    cmd := ScanCmd()
    cmd.SetArgs([]string{root})
    var out bytes.Buffer
    cmd.SetOut(&out)
    cmd.SetErr(&out)

    if err := cmd.Execute(); err != nil {
        t.Fatal(err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/cli/commands -run TestScanCommandWritesSummary`

Expected: FAIL (ScanCmd undefined).

**Step 3: Implement scan command**

```go
func ScanCmd() *cobra.Command {
    var depth int
    cmd := &cobra.Command{
        Use:   "scan [path]",
        Short: "Scan repo for new epics",
        Args:  cobra.MaximumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            root := "."
            if len(args) == 1 {
                root = args[0]
            }
            planDir := filepath.Join(root, ".tandemonium", "plan")
            _, err := explore.Run(root, planDir, explore.Options{Depth: depth})
            return err
        },
    }
    cmd.Flags().IntVar(&depth, "depth", 2, "scan depth (1-3)")
    return cmd
}
```

**Step 4: Implement init flow (use scan + generator + writer)**

```go
// root.go (init command)
// - remove Vision/MVP prompt
// - prompt for depth (1/2/3) and existing handling when interactive
// - use agenttargets registry to resolve run target (default claude)
// - run explore.Run, build prompt, call generator, write specs
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/tandemonium/cli/commands -run TestScanCommandWritesSummary`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/tandemonium/cli
git commit -m "feat(tandemonium): add init scan flow"
```

---

### Task 5: TUI background scanning

**Files:**
- Modify: `internal/tandemonium/tui/model.go`
- Test: `internal/tandemonium/tui/model_test.go`

**Step 1: Write failing test (scan tick triggers)**

```go
func TestBackgroundScanTick(t *testing.T) {
    m := NewModel()
    m.ScanInterval = time.Minute
    msg := scanTickMsg{}
    cmd := m.Update(msg)
    if cmd == nil {
        t.Fatalf("expected scan cmd")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tandemonium/tui -run TestBackgroundScanTick`

Expected: FAIL.

**Step 3: Implement background scan loop**

```go
// model.go
// - add scanTickMsg + scanCmd
// - schedule time.After(m.ScanInterval)
// - on tick: call explore.Run and update model status
// - additionally poll `git rev-parse HEAD` and trigger scan when it changes
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tandemonium/tui -run TestBackgroundScanTick`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tandemonium/tui
git commit -m "feat(tandemonium): add background scan loop"
```

---

### Task 6: Docs + full verification

**Files:**
- Modify: `docs/plans/2026-01-23-tandemonium-init-epics-design.md`
- Modify: `AGENTS.md`

**Step 1: Update docs**

- Document new init behavior, depth prompt, and scan command.
- Document epic spec schema, status/priority enums, and file layout.

**Step 2: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 3: Commit**

```bash
git add docs/plans/2026-01-23-tandemonium-init-epics-design.md AGENTS.md
git commit -m "docs(tandemonium): document init epic generation"
```

---

Plan complete and saved to `docs/plans/2026-01-23-tandemonium-init-epics-implementation-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
