# Praude Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `N/A (no task system)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Build Praude as a TUI-first PM/specs CLI that stores full PRD specs, orchestrates agent research/PRD generation, and enforces drift control.

**Architecture:** A Go CLI + TUI app that writes YAML specs to `.praude/specs/`, manages research and brief artifacts, and spawns configured agent CLIs. Core logic lives under `internal/` with focused packages: `project`, `specs`, `brief`, `agents`, `research`, `git`, `cli`, `tui`.

**Tech Stack:** Go 1.22, Cobra, Bubble Tea, Lip Gloss, `gopkg.in/yaml.v3`, `github.com/BurntSushi/toml`.

---

### Task 1: Project init + layout + config

**Files:**
- Create: `cmd/praude/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/project/project.go`
- Create: `internal/project/project_test.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
func TestInitCreatesPraudeLayout(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	mustDir := []string{
		filepath.Join(root, ".praude"),
		filepath.Join(root, ".praude", "specs"),
		filepath.Join(root, ".praude", "research"),
		filepath.Join(root, ".praude", "briefs"),
	}
	for _, dir := range mustDir {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Fatalf("missing dir %s", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".praude", "config.toml")); err != nil {
		t.Fatalf("expected config.toml")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/project -v`  
Expected: FAIL with "project.Init undefined"

**Step 3: Write minimal implementation**

```go
// internal/project/project.go
package project

import (
	"os"
	"path/filepath"
)

const PraudeDir = ".praude"

func RootDir(root string) string {
	return filepath.Join(root, PraudeDir)
}

func SpecsDir(root string) string {
	return filepath.Join(RootDir(root), "specs")
}

func ResearchDir(root string) string {
	return filepath.Join(RootDir(root), "research")
}

func BriefsDir(root string) string {
	return filepath.Join(RootDir(root), "briefs")
}

func ConfigPath(root string) string {
	return filepath.Join(RootDir(root), "config.toml")
}

func Init(root string) error {
	dirs := []string{RootDir(root), SpecsDir(root), ResearchDir(root), BriefsDir(root)}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Stat(ConfigPath(root)); os.IsNotExist(err) {
		if err := os.WriteFile(ConfigPath(root), []byte(DefaultConfigToml), 0o644); err != nil {
			return err
		}
	}
	return nil
}
```

```go
// internal/config/config.go
package config

type Config struct {
	Agents map[string]AgentProfile `toml:"agents"`
}

type AgentProfile struct {
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
}

const DefaultConfigToml = ` + "`" + `# Praude configuration

[agents.codex]
command = "codex"
args = []
# args = ["--profile", "pm", "--prompt-file", "{brief}"]

[agents.claude]
command = "claude"
args = []
# args = ["--profile", "pm", "--prompt-file", "{brief}"]

[agents.opencode]
command = "opencode"
args = []

[agents.droid]
command = "droid"
args = []
` + "`" + `
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/project -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/praude/main.go internal/cli/root.go internal/project/project.go internal/project/project_test.go internal/config/config.go internal/config/config_test.go
git commit -m "feat: add praude init layout and config defaults"
```

---

### Task 2: Spec schema + ID generation + template spec

**Files:**
- Create: `internal/specs/schema.go`
- Create: `internal/specs/id.go`
- Create: `internal/specs/create.go`
- Create: `internal/specs/create_test.go`

**Step 1: Write the failing test**

```go
func TestCreateTemplateSpec(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := specs.CreateTemplate(specsDir, time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Contains(raw, []byte("id: \"PRD-001\"")) {
		t.Fatalf("expected PRD-001 id")
	}
	if !bytes.Contains(raw, []byte("strategic_context:")) {
		t.Fatalf("expected full schema")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`  
Expected: FAIL with "specs.CreateTemplate undefined"

**Step 3: Write minimal implementation**

```go
// internal/specs/id.go
package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var idPattern = regexp.MustCompile(` + "`" + `^PRD-(\d+)\.ya?ml$` + "`" + `)

func NextID(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "PRD-001", nil
	}
	var nums []int
	for _, e := range entries {
		m := idPattern.FindStringSubmatch(e.Name())
		if len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				nums = append(nums, n)
			}
		}
	}
	sort.Ints(nums)
	next := 1
	if len(nums) > 0 {
		next = nums[len(nums)-1] + 1
	}
	return fmt.Sprintf("PRD-%03d", next), nil
}
```

```go
// internal/specs/create.go
package specs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func CreateTemplate(dir string, now time.Time) (string, error) {
	id, err := NextID(dir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".yaml")
	doc := fmt.Sprintf(` + "`" + `id: "%s"
title: "Example PRD Title"
created_at: "%s"
strategic_context:
  cuj_id: "CUJ-1"
  cuj_name: "Example Journey"
  feature_id: "example-feature"
  mvp_included: true
user_story:
  text: "As a user, I want X so that Y."
  hash: "pending"
summary: |
  One paragraph describing what to build and why.
requirements:
  - "Requirement one"
acceptance_criteria:
  - id: "ac-1"
    description: "Acceptance criterion one"
files_to_modify:
  - action: "create"
    path: "path/to/file"
    description: "Why this file"
complexity: "medium"
estimated_minutes: 25
priority: 1
` + "`" + `, id, now.UTC().Format(time.RFC3339))
	return path, os.WriteFile(path, []byte(doc), 0o644)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/schema.go internal/specs/id.go internal/specs/create.go internal/specs/create_test.go
git commit -m "feat: add spec schema and template creation"
```

---

### Task 3: Spec validation

**Files:**
- Create: `internal/specs/validate.go`
- Create: `internal/specs/validate_test.go`

**Step 1: Write the failing test**

```go
func TestValidateMissingTitle(t *testing.T) {
	raw := []byte("id: \"PRD-001\"\n")
	if err := specs.Validate(raw); err == nil {
		t.Fatalf("expected error")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`  
Expected: FAIL with "Validate undefined"

**Step 3: Write minimal implementation**

```go
// internal/specs/validate.go
package specs

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func Validate(raw []byte) error {
	var doc struct {
		ID     string `yaml:"id"`
		Title  string `yaml:"title"`
		Summary string `yaml:"summary"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.ID == "" || doc.Title == "" || doc.Summary == "" {
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
git add internal/specs/validate.go internal/specs/validate_test.go
git commit -m "feat: validate required spec fields"
```

---

### Task 4: Load specs + list/show CLI

**Files:**
- Create: `internal/specs/load.go`
- Create: `internal/specs/load_test.go`
- Create: `internal/cli/commands/list.go`
- Create: `internal/cli/commands/show.go`
- Create: `internal/cli/commands/list_test.go`

**Step 1: Write the failing test**

```go
func TestLoadSummaries(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "PRD-001.yaml"), []byte("id: \"PRD-001\"\ntitle: \"A\"\nsummary: \"S\"\n"), 0o644)
	list, _ := specs.LoadSummaries(dir)
	if len(list) != 1 || list[0].ID != "PRD-001" {
		t.Fatalf("expected summary")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/specs -v`  
Expected: FAIL with "LoadSummaries undefined"

**Step 3: Write minimal implementation**

```go
// internal/specs/load.go
package specs

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Summary struct {
	ID     string
	Title  string
	Summary string
	Path   string
}

func LoadSummaries(dir string) ([]Summary, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Summary{}, []string{}
	}
	var out []Summary
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
		var doc struct {
			ID string `yaml:"id"`
			Title string `yaml:"title"`
			Summary string `yaml:"summary"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			warnings = append(warnings, "parse failed: "+path)
			continue
		}
		out = append(out, Summary{ID: doc.ID, Title: doc.Title, Summary: doc.Summary, Path: path})
	}
	return out, warnings
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/specs -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/specs/load.go internal/specs/load_test.go internal/cli/commands/list.go internal/cli/commands/show.go internal/cli/commands/list_test.go
git commit -m "feat: add spec list/show"
```

---

### Task 5: Auto-commit spec writes

**Files:**
- Create: `internal/git/commit.go`
- Create: `internal/git/commit_test.go`
- Modify: `internal/specs/create.go`

**Step 1: Write the failing test**

```go
func TestCommitFilesNoGit(t *testing.T) {
	dir := t.TempDir()
	if err := git.CommitFiles(dir, []string{"x.txt"}, "msg"); err == nil {
		t.Fatalf("expected error without git")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git -v`  
Expected: FAIL with "CommitFiles undefined"

**Step 3: Write minimal implementation**

```go
// internal/git/commit.go
package git

import (
	"fmt"
	"os/exec"
)

func CommitFiles(root string, files []string, message string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return err
	}
	args := append([]string{"-C", root, "add"}, files...)
	if err := exec.Command("git", args...).Run(); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", root, "commit", "-m", message)
	return cmd.Run()
}

func EnsureRepo(root string) error {
	if err := exec.Command("git", "-C", root, "rev-parse", "--git-dir").Run(); err != nil {
		return fmt.Errorf("not a git repo")
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/git/commit.go internal/git/commit_test.go internal/specs/create.go
git commit -m "feat: add auto-commit helper"
```

---

### Task 6: Brief generation

**Files:**
- Create: `internal/brief/brief.go`
- Create: `internal/brief/brief_test.go`

**Step 1: Write the failing test**

```go
func TestBriefIncludesSummary(t *testing.T) {
	b := brief.Compose(brief.Input{ID: "PRD-001", Title: "T", Summary: "S"})
	if !strings.Contains(b, "Summary:") || !strings.Contains(b, "S") {
		t.Fatalf("expected summary in brief")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/brief -v`  
Expected: FAIL with "Compose undefined"

**Step 3: Write minimal implementation**

```go
// internal/brief/brief.go
package brief

import "fmt"

type Input struct {
	ID string
	Title string
	Summary string
	Requirements []string
	Acceptance []string
	ResearchFiles []string
}

func Compose(in Input) string {
	return fmt.Sprintf(` + "`" + `PRD: %s
Title: %s

Summary:
%s

Requirements:
%v

Acceptance Criteria:
%v

Research:
%v
` + "`" + `, in.ID, in.Title, in.Summary, in.Requirements, in.Acceptance, in.ResearchFiles)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/brief -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/brief/brief.go internal/brief/brief_test.go
git commit -m "feat: add brief composer"
```

---

### Task 7: Agent profiles + run command

**Files:**
- Create: `internal/agents/agents.go`
- Create: `internal/agents/agents_test.go`
- Create: `internal/cli/commands/run.go`
- Create: `internal/cli/commands/run_test.go`

**Step 1: Write the failing test**

```go
func TestResolveProfileMissing(t *testing.T) {
	cfg := config.Config{Agents: map[string]config.AgentProfile{}}
	if _, err := agents.Resolve(cfg, "codex"); err == nil {
		t.Fatalf("expected error")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agents -v`  
Expected: FAIL with "agents.Resolve undefined"

**Step 3: Write minimal implementation**

```go
// internal/agents/agents.go
package agents

import (
	"fmt"
	"os/exec"
)

type Profile struct {
	Command string
	Args []string
}

func Resolve(cfg map[string]Profile, name string) (Profile, error) {
	p, ok := cfg[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown agent %s", name)
	}
	return p, nil
}

func Launch(p Profile, briefPath string) error {
	if _, err := exec.LookPath(p.Command); err != nil {
		return err
	}
	args := append([]string{}, p.Args...)
	args = append(args, briefPath)
	cmd := exec.Command(p.Command, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agents -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agents/agents.go internal/agents/agents_test.go internal/cli/commands/run.go internal/cli/commands/run_test.go
git commit -m "feat: add agent profiles and run command"
```

---

### Task 8: Research output + spec reference

**Files:**
- Create: `internal/research/research.go`
- Create: `internal/research/research_test.go`
- Modify: `internal/specs/schema.go`
- Modify: `internal/specs/create.go`
- Create: `internal/cli/commands/research.go`

**Step 1: Write the failing test**

```go
func TestCreateResearchFile(t *testing.T) {
	dir := t.TempDir()
	path, err := research.Create(dir, "PRD-001", time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(path), "PRD-001") {
		t.Fatalf("expected prd in filename")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/research -v`  
Expected: FAIL with "research.Create undefined"

**Step 3: Write minimal implementation**

```go
// internal/research/research.go
package research

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func Create(dir, id string, now time.Time) (string, error) {
	name := fmt.Sprintf("%s-%s.md", id, now.UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	body := fmt.Sprintf("# Research for %s\n\n- Competitive analysis:\n- Market summary:\n", id)
	return path, os.WriteFile(path, []byte(body), 0o644)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/research -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/research/research.go internal/research/research_test.go internal/specs/schema.go internal/specs/create.go internal/cli/commands/research.go
git commit -m "feat: add research artifacts"
```

---

### Task 9: TUI skeleton (list + detail + hotkeys)

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/view.go`
- Create: `internal/tui/model_test.go`

**Step 1: Write the failing test**

```go
func TestViewIncludesHeaders(t *testing.T) {
	m := tui.NewModel()
	out := m.View()
	if !strings.Contains(out, "PRDs") || !strings.Contains(out, "DETAILS") {
		t.Fatalf("expected headers")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with "NewModel undefined"

**Step 3: Write minimal implementation**

```go
// internal/tui/model.go
package tui

import tea "github.com/charmbracelet/bubbletea"

type Model struct {
}

func NewModel() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) View() string {
	return "PRDs\n\nDETAILS"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view.go internal/tui/model_test.go
git commit -m "feat: add tui skeleton"
```

---

### Task 10: CLI wiring + main entrypoint

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `cmd/praude/main.go`

**Step 1: Write the failing test**

```go
func TestRootCommandHasInit(t *testing.T) {
	cmd := cli.NewRoot()
	if cmd == nil || cmd.Use != "praude" {
		t.Fatalf("expected root command")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli -v`  
Expected: FAIL with "NewRoot undefined"

**Step 3: Write minimal implementation**

```go
// internal/cli/root.go
package cli

import (
	"github.com/spf13/cobra"
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use: "praude",
		Short: "PM-focused PRD CLI",
	}
	return root
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cli/root.go cmd/praude/main.go
git commit -m "feat: wire cli entrypoint"
```

