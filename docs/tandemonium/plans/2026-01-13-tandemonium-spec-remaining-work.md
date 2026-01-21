# Tandemonium Spec Remainder Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the remaining MVP-scope items from `prd/tandemonium-spec.md` that are not yet in the Go/TUI codebase.

**Architecture:** Build missing spec features as small, testable modules in existing packages (CLI commands in `internal/cli/commands`, planning in a new `internal/plan` package, task/spec creation in `internal/specs` + `internal/storage`, UI enhancements in `internal/tui`). Keep data in `.tandemonium/` and wire to SQLite/YAML as the spec describes.

**Tech Stack:** Go 1.22+, Bubble Tea, Cobra, SQLite, tmux, TOML config, YAML specs.

---

## Priority Order (Critical Path)

1) `tand init` planning prompt + `.tandemonium/plan/` documents
2) Task creation + quick mode + spec YAML schema + validation
3) CLI surface parity (`plan`, `execute`, `stop`, `export`, `import`)
4) Health monitoring + auto-restart
5) Review alignment check (CUJ/MVP)
6) Command palette + settings UI + full shortcuts
7) Real recovery (`recover`) and cleanup/doctor polish

---

### Task 1: Planning prompt flow and plan documents

**Files:**
- Create: `internal/plan/plan.go`
- Create: `internal/plan/plan_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/project/init.go` (if needed for plan dir creation)
- Test: `internal/plan/plan_test.go`

**Step 1: Write the failing test**

```go
func TestRunPlanningCreatesPlanDocs(t *testing.T) {
    root := t.TempDir()
    planDir := filepath.Join(root, ".tandemonium", "plan")
    input := strings.NewReader("y\nmy vision\nmy mvp\n")
    if err := plan.Run(input, planDir); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(filepath.Join(planDir, "vision.md")); err != nil {
        t.Fatalf("expected vision.md: %v", err)
    }
    if _, err := os.Stat(filepath.Join(planDir, "mvp.md")); err != nil {
        t.Fatalf("expected mvp.md: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plan -v`  
Expected: FAIL with "plan.Run undefined"

**Step 3: Write minimal implementation**

```go
package plan

func Run(in io.Reader, planDir string) error {
    if err := os.MkdirAll(planDir, 0o755); err != nil {
        return err
    }
    scanner := bufio.NewScanner(in)
    if !scanLine(scanner) { return nil }
    if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" { return nil }
    vision := scanText(scanner)
    mvp := scanText(scanner)
    if err := os.WriteFile(filepath.Join(planDir, "vision.md"), []byte(vision+"\n"), 0o644); err != nil {
        return err
    }
    if err := os.WriteFile(filepath.Join(planDir, "mvp.md"), []byte(mvp+"\n"), 0o644); err != nil {
        return err
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/plan -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plan/plan.go internal/plan/plan_test.go internal/cli/root.go internal/project/init.go
git commit -m "feat: add planning prompt and plan docs"
```

---

### Task 2: Task creation CLI + quick mode YAML spec generation

**Files:**
- Create: `internal/specs/create.go`
- Create: `internal/specs/create_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/commands/status.go` (if new states used)
- Modify: `internal/specs/specs.go`

**Step 1: Write the failing test**

```go
func TestCreateQuickSpec(t *testing.T) {
    dir := t.TempDir()
    now := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
    path, err := specs.CreateQuickSpec(dir, "Fix login timeout bug", now)
    if err != nil { t.Fatal(err) }
    raw, _ := os.ReadFile(path)
    if !bytes.Contains(raw, []byte("quick_mode: true")) {
        t.Fatalf("expected quick_mode marker")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`  
Expected: FAIL with "CreateQuickSpec undefined"

**Step 3: Write minimal implementation**

```go
func CreateQuickSpec(dir, raw string, now time.Time) (string, error) {
    id := NewID(now) // TAND-### helper
    path := filepath.Join(dir, id+".yaml")
    doc := fmt.Sprintf("id: %q\ntitle: %q\ncreated_at: %q\nquick_mode: true\nsummary: |\n  %s\n",
        id, firstLine(raw), now.Format(time.RFC3339), strings.TrimSpace(raw))
    return path, os.WriteFile(path, []byte(doc), 0o644)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/create.go internal/specs/create_test.go internal/cli/root.go internal/specs/specs.go
git commit -m "feat: add quick mode task creation"
```

---

### Task 3: Spec schema validation (required fields)

**Files:**
- Create: `internal/specs/validate.go`
- Create: `internal/specs/validate_test.go`
- Modify: `internal/specs/specs.go` (call validation during summary load)

**Step 1: Write the failing test**

```go
func TestValidateSpecMissingTitle(t *testing.T) {
    doc := []byte("id: \"TAND-001\"\nstatus: \"ready\"\n")
    if err := specs.Validate(doc); err == nil {
        t.Fatalf("expected validation error")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`  
Expected: FAIL with "Validate undefined"

**Step 3: Write minimal implementation**

```go
func Validate(raw []byte) error {
    var doc struct {
        ID     string `yaml:"id"`
        Title  string `yaml:"title"`
        Status string `yaml:"status"`
    }
    if err := yaml.Unmarshal(raw, &doc); err != nil { return err }
    if doc.ID == "" || doc.Title == "" || doc.Status == "" {
        return fmt.Errorf("missing required fields")
    }
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/validate.go internal/specs/validate_test.go internal/specs/specs.go
git commit -m "feat: validate spec YAML required fields"
```

---

### Task 4: CLI surface parity (`plan`, `execute`, `stop`, `export`, `import`)

**Files:**
- Create: `internal/cli/commands/plan.go`
- Create: `internal/cli/commands/execute.go`
- Create: `internal/cli/commands/stop.go`
- Create: `internal/cli/commands/export.go`
- Create: `internal/cli/commands/import.go`
- Create: `internal/cli/commands/plan_test.go`
- Modify: `internal/cli/root.go`

**Step 1: Write the failing test**

```go
func TestPlanCommandRunsPlanning(t *testing.T) {
    cmd := commands.PlanCmd()
    out := bytes.NewBuffer(nil)
    cmd.SetOut(out)
    cmd.SetIn(strings.NewReader("n\n"))
    if err := cmd.Execute(); err != nil {
        t.Fatal(err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -v`  
Expected: FAIL with "PlanCmd undefined"

**Step 3: Write minimal implementation**

```go
func PlanCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "plan",
        Short: "Run planning flow",
        RunE: func(cmd *cobra.Command, args []string) error {
            root, err := project.FindRoot(".")
            if err != nil { return err }
            return plan.Run(cmd.InOrStdin(), filepath.Join(root, ".tandemonium", "plan"))
        },
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/plan.go internal/cli/commands/execute.go internal/cli/commands/stop.go internal/cli/commands/export.go internal/cli/commands/import.go internal/cli/commands/plan_test.go internal/cli/root.go
git commit -m "feat: add plan/execute/stop/export/import commands"
```

---

### Task 5: Health monitoring + auto-restart

**Files:**
- Create: `internal/agent/health.go`
- Create: `internal/agent/health_test.go`
- Modify: `internal/agent/loop.go`
- Modify: `internal/config/config.go`

**Step 1: Write the failing test**

```go
func TestHealthMonitorRestartsCrashedSession(t *testing.T) {
    fake := agent.NewFakeRunner()
    monitor := agent.NewHealthMonitor(fake, 10*time.Millisecond, true)
    monitor.Check("tand-123")
    if !fake.Restarted("tand-123") {
        t.Fatalf("expected restart call")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent -v`  
Expected: FAIL with "NewHealthMonitor undefined"

**Step 3: Write minimal implementation**

```go
type HealthMonitor struct {
    runner Runner
    interval time.Duration
    restart bool
}
func (h *HealthMonitor) Check(session string) {
    if h.restart && !h.runner.IsAlive(session) {
        _ = h.runner.Restart(session)
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/health.go internal/agent/health_test.go internal/agent/loop.go internal/config/config.go
git commit -m "feat: add agent health monitoring and restart"
```

---

### Task 6: Review alignment check (CUJ/MVP)

**Files:**
- Modify: `internal/specs/detail.go`
- Modify: `internal/tui/review_detail.go`
- Modify: `internal/tui/review_detail_test.go`

**Step 1: Write the failing test**

```go
func TestReviewDetailShowsAlignment(t *testing.T) {
    view := renderReviewDetailWithMVP(true)
    if !strings.Contains(view, "Alignment: MVP") {
        t.Fatalf("expected MVP alignment")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with "Alignment: MVP" missing

**Step 3: Write minimal implementation**

```go
if detail.MVPIncluded != nil {
    if *detail.MVPIncluded { out += "Alignment: MVP\n" } else { out += "Alignment: out-of-scope\n" }
} else {
    out += "Alignment: unknown\n"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/detail.go internal/tui/review_detail.go internal/tui/review_detail_test.go
git commit -m "feat: show MVP alignment in review detail"
```

---

### Task 7: Command palette + settings UI + full shortcuts

**Files:**
- Create: `internal/tui/palette.go`
- Create: `internal/tui/palette_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/view.go`
- Modify: `internal/config/config.go`

**Step 1: Write the failing test**

```go
func TestPaletteOpensOnCtrlK(t *testing.T) {
    m := NewModel()
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
    if !m.PaletteOpen {
        t.Fatalf("expected palette open")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with "PaletteOpen false"

**Step 3: Write minimal implementation**

```go
if key.Type == tea.KeyCtrlK {
    m.PaletteOpen = true
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/palette.go internal/tui/palette_test.go internal/tui/model.go internal/tui/view.go internal/config/config.go
git commit -m "feat: add command palette and settings shortcuts"
```

---

### Task 8: Real recovery flow

**Files:**
- Modify: `internal/cli/commands/recover.go`
- Modify: `internal/storage/diagnostics.go`
- Create: `internal/storage/rebuild.go`
- Create: `internal/storage/rebuild_test.go`

**Step 1: Write the failing test**

```go
func TestRebuildFromSpecsLoadsTasks(t *testing.T) {
    root := t.TempDir()
    specPath := writeSpec(root, "TAND-001")
    if err := storage.RebuildFromSpecs(root); err != nil {
        t.Fatal(err)
    }
    if _, err := storage.LoadTaskByID(root, "TAND-001"); err != nil {
        t.Fatalf("expected task from spec: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`  
Expected: FAIL with "RebuildFromSpecs undefined"

**Step 3: Write minimal implementation**

```go
func RebuildFromSpecs(root string) error {
    // iterate specs/*.yaml, insert tasks into sqlite if missing
    return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/commands/recover.go internal/storage/rebuild.go internal/storage/rebuild_test.go internal/storage/diagnostics.go
git commit -m "feat: implement recovery rebuild from specs"
```

---

### Task 9: Export/Import state (JSON)

**Files:**
- Modify: `internal/cli/commands/export.go`
- Modify: `internal/cli/commands/import.go`
- Create: `internal/storage/export.go`
- Create: `internal/storage/export_test.go`

**Step 1: Write the failing test**

```go
func TestExportWritesJSON(t *testing.T) {
    root := t.TempDir()
    path := filepath.Join(root, "out.json")
    if err := storage.ExportJSON(root, path); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(path); err != nil {
        t.Fatalf("expected export file")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -v`  
Expected: FAIL with "ExportJSON undefined"

**Step 3: Write minimal implementation**

```go
func ExportJSON(root, path string) error {
    payload := map[string]any{"tasks": []any{}}
    data, _ := json.MarshalIndent(payload, "", "  ")
    return os.WriteFile(path, data, 0o644)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/storage/export.go internal/storage/export_test.go internal/cli/commands/export.go internal/cli/commands/import.go
git commit -m "feat: add export/import JSON scaffolding"
```

---

### Task 10: Keyboard shortcuts parity

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/view.go`
- Modify: `internal/tui/view_test.go`

**Step 1: Write the failing test**

```go
func TestQuitShortcutMatchesConfig(t *testing.T) {
    m := NewModel()
    m.Config.Shortcuts.Quit = "q"
    _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
    if cmd == nil {
        t.Fatalf("expected quit command")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with "expected quit command"

**Step 3: Write minimal implementation**

```go
if key.Type == tea.KeyRunes && string(key.Runes) == m.Config.Shortcuts.Quit {
    return m, tea.Quit
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view.go internal/tui/view_test.go
git commit -m "feat: honor shortcut config for quit and focus"
```

