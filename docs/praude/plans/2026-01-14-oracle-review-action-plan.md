# Oracle Review Action Plan

Date: 2026-01-14
Source: GPT-5.2 Pro architectural review via Oracle

## Overview

This plan addresses issues identified in the Oracle architectural review of Praude.
Items are prioritized by severity and dependency order.

---

## Priority 1: Critical Fixes (Blocking)

These issues prevent the application from functioning correctly.

### 1.1 Fix Invalid YAML Template

**File:** `internal/specs/create.go`

**Problem:** Template YAML is built with `fmt.Sprintf`, likely producing invalid YAML.
Tests only check substring presence, not actual parseability.

**Fix:**
- Construct a `Spec` struct with example values
- Use `yaml.Marshal` to produce valid YAML
- For human-friendly formatting (block scalars), use `yaml.Node` if needed

**Test change:** Add test that `yaml.Unmarshal`s the template output.

---

### 1.2 Fix Invalid TOML Config

**File:** `internal/config/config.go`

**Problem:** `DefaultConfigToml` has keys on same line, which is invalid TOML.

**Fix:** Reformat to proper TOML with one key/value per line:

```toml
[agents.codex]
command = "codex"
args = []
# args = ["--profile", "pm", "--prompt-file", "{brief}"]
```

**Test change:** Add test that `toml.Decode`s the default config.

---

### 1.3 Wire CLI Subcommands

**File:** `internal/cli/root.go`

**Problem:** Root command has no subcommands registered. Binary is inert.

**Fix:**
- Import `internal/cli/commands`
- Register `ListCmd()`, `RunCmd()`, `ShowCmd()`, etc. on root
- Add `init` and `new` commands

---

### 1.4 Fix Run Command Empty Map

**File:** `internal/cli/commands/run.go`

**Problem:** `agents.Resolve(map[string]agents.Profile{}, agent)` passes empty map,
so resolution always fails.

**Fix:**
- Load config from `.praude/config.toml`
- Convert `config.AgentProfile` to `agents.Profile`
- Pass loaded profiles to `Resolve()`

---

### 1.5 Fix NextID Error Handling

**File:** `internal/specs/id.go`

**Problem:** All `ReadDir` errors return `"PRD-001", nil`, risking overwrites.

**Fix:**
- Only treat `os.IsNotExist(err)` as default case
- Propagate other errors (permissions, IO)

---

### 1.6 Handle Execute Error

**File:** `cmd/praude/main.go`

**Problem:** Error from `cli.Execute()` is ignored; loses exit codes.

**Fix:**
```go
func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

---

## Priority 2: Architecture Improvements

These changes improve maintainability and enable future features.

### 2.1 Add Application Layer

**New package:** `internal/app/`

**Problem:** CLI and TUI call leaf packages directly. Cross-cutting behaviors
(auto-commit, brief writing, drift control) will be duplicated.

**Create functions:**
- `InitProject(root string) error`
- `CreateSpec(root string) (string, error)` - creates template, auto-commits
- `ListSpecs(root string) ([]specs.Summary, error)`
- `ShowSpec(root, id string) (*specs.Spec, error)`
- `ValidateSpec(root, id string) error`
- `ValidateAll(root string) []error`
- `ComposeBrief(root, id, agentProfile string) (string, error)` - writes brief file
- `RunAgent(root, id, agentProfile string) error`
- `ApproveSpec(root, id string) error`
- `DriftCheck(root, id string) (*DriftResult, error)`

**Benefit:** CLI and TUI share identical behavior.

---

### 2.2 Add Config Loading

**File:** `internal/config/config.go`

**Add:**
```go
func Load(path string) (*Config, error)
func (c *Config) ResolveProfile(name string) (agents.Profile, error)
func SubstitutePlaceholders(args []string, vars map[string]string) []string
```

**Placeholders to support:**
- `{brief}` - path to brief file
- `{root}` - project root
- `{id}` - spec ID

---

### 2.3 Add Project Root Detection

**File:** `internal/project/project.go`

**Add:**
```go
func FindRoot(start string) (string, error)
```

**Behavior:** Walk upward from `start` looking for `.praude/` directory.
Fall back to looking for `.git/` if no `.praude/` found.

**Update all commands** to use `FindRoot(os.Getwd())` instead of raw `os.Getwd()`.

---

### 2.4 Improve Agent Launching

**File:** `internal/agents/agents.go`

**Problems:**
- No `Wait()` to reap processes
- No stdout/stderr handling
- No TTY support for interactive agents
- No context cancellation

**Add two modes:**

```go
// Interactive mode - for TTY agents like Claude Code
func LaunchInteractive(p Profile, briefPath string) error

// Captured mode - for agents that return structured output
func LaunchCaptured(p Profile, briefPath string) ([]byte, error)
```

**Interactive mode:**
- Attach stdin/stdout/stderr
- Use `cmd.Run()` (blocking)

**Captured mode:**
- Use `cmd.CombinedOutput()`
- Return output for parsing

---

## Priority 3: Data Model Enhancements

### 3.1 Add Governance Fields to Spec Schema

**File:** `internal/specs/schema.go`

**Add fields:**
```go
type Spec struct {
    // ... existing fields ...

    // Governance
    Status      string `yaml:"status"`       // draft, approved, locked
    UpdatedAt   string `yaml:"updated_at"`
    ApprovedAt  string `yaml:"approved_at,omitempty"`
    ApprovedBy  string `yaml:"approved_by,omitempty"`

    // Drift control
    ApprovedHash string `yaml:"approved_hash,omitempty"`
}
```

**Status lifecycle:** `draft` → `approved` → `locked`

---

### 3.2 Enhance Requirements Structure

**Current:** `Requirements []string`

**Change to:**
```go
type Requirement struct {
    ID       string `yaml:"id"`
    Text     string `yaml:"text"`
    Priority string `yaml:"priority"` // must, should, could
}

Requirements []Requirement `yaml:"requirements"`
```

---

### 3.3 Enhance Acceptance Criteria

**Add fields:**
```go
type AcceptanceCriterion struct {
    ID           string `yaml:"id"`
    Description  string `yaml:"description"`
    Type         string `yaml:"type,omitempty"`         // functional, non-functional
    Verification string `yaml:"verification,omitempty"` // how to verify
    RequirementRefs []string `yaml:"requirement_refs,omitempty"` // links to R-###
}
```

---

### 3.4 Fix UserStory Hash Semantics

**Current:** `Hash` stored alongside `Text` is redundant (can compute).

**Change to:**
```go
type UserStory struct {
    Text         string `yaml:"text"`
    ApprovedHash string `yaml:"approved_hash,omitempty"` // set on approve
}
```

The current hash is computed on-demand; `ApprovedHash` is the drift baseline.

---

## Priority 4: Missing Commands

### 4.1 Add `praude init` Command

**File:** `internal/cli/commands/init.go`

**Behavior:**
- Call `project.Init(root)`
- Print success message with created paths

---

### 4.2 Add `praude new` Command

**File:** `internal/cli/commands/new.go`

**Behavior:**
- Call `app.CreateSpec(root)`
- Auto-commit with `chore(praude): add PRD-###`
- Print path to new spec

---

### 4.3 Add `praude show <id>` Command

**File:** `internal/cli/commands/show.go`

**Behavior:**
- Load full spec by ID
- Print YAML to stdout (or formatted view with `--format`)

---

### 4.4 Add `praude validate` Command

**File:** `internal/cli/commands/validate.go`

**Behavior:**
- `praude validate <id>` - validate single spec
- `praude validate --all` - validate all specs
- Return actionable per-field errors

---

### 4.5 Add `praude research <id>` Command

**File:** `internal/cli/commands/research.go`

**Behavior:**
- Create research artifact via `research.Create()`
- Update spec's `research` field
- Auto-commit

---

## Priority 5: Brief Composition Improvements

### 5.1 Improve Brief Format

**File:** `internal/brief/brief.go`

**Current:** Simple `fmt.Sprintf` with `%v` for lists.

**Change to:** Structured Markdown with:
- Overview section
- Requirements (numbered)
- Acceptance criteria (numbered)
- Files to modify (checklist)
- Constraints / non-goals
- Output contract (what format agent should return)
- Spec metadata (ID, path, baseline hash)

**Example output contract section:**
```markdown
## Output Contract

Return your changes as a YAML fragment with only the fields you're updating:

```yaml
summary: |
  Updated summary text...
requirements:
  - id: R-1
    text: ...
```
```

---

## Priority 6: TUI Implementation

### 6.1 Add TUI State Model

**File:** `internal/tui/model.go`

**Add state:**
```go
type Model struct {
    specs    []specs.Summary
    selected int
    mode     Mode // normal, confirm, command
    viewport viewport.Model
    err      error
}

type Mode int
const (
    ModeNormal Mode = iota
    ModeConfirm
    ModeCommand
)
```

---

### 6.2 Wire TUI as Default

**File:** `internal/cli/root.go`

**Behavior:** When `praude` is run with no args and stdout is a TTY, launch TUI.

```go
if len(os.Args) == 1 && term.IsTerminal(int(os.Stdout.Fd())) {
    return runTUI()
}
```

---

### 6.3 Implement Two-Pane Layout

**Left pane (list):**
- ID, title, status, completeness indicator
- Filter/search with `/`
- Sort by updated_at

**Right pane (details):**
- Tabs: PRD | YAML | Brief | Research | Drift
- Completeness indicators per section
- Visual lock indicator for approved specs

**Hotkeys:**
- `n` new spec
- `r` research
- `v` validate
- `a` approve
- `d` drift check
- `?` help
- `q` quit

---

## Priority 7: Testing Improvements

### 7.1 Add Format Validation Tests

**New tests:**
- `TestDefaultConfigIsValidTOML` - parse with `toml.Decode`
- `TestTemplateSpecIsValidYAML` - parse with `yaml.Unmarshal`
- `TestTemplateSpecHasAllRequiredFields` - validate after parse

---

### 7.2 Add Integration Tests

**New tests:**
- `TestInitCreatesValidProject` - run init, verify all paths
- `TestNewCreatesValidSpec` - run new, parse resulting YAML
- `TestListShowsCreatedSpecs` - create specs, verify list output
- `TestRunWithMissingAgentPrintsBriefPath` - verify graceful fallback

---

### 7.3 Add Failure Mode Tests

**New tests:**
- `TestNextIDWithPermissionError` - verify error propagation
- `TestCommitWithNoGitRepo` - verify clean error
- `TestCommitWithDirtyIndex` - verify behavior
- `TestLoadSpecsWithInvalidYAML` - verify warnings returned

---

## Priority 8: Security Hardening

### 8.1 Add Command Confirmation

**Location:** `internal/app/` or TUI

Before launching an agent for the first time in a repo, show:
- Resolved command and args
- Require confirmation unless `--yes` flag

---

### 8.2 Add Path Traversal Protection

**Location:** Brief writing, research creation

Before writing any file:
```go
func SafePath(root, subpath string) (string, error) {
    abs := filepath.Join(root, subpath)
    abs = filepath.Clean(abs)
    if !strings.HasPrefix(abs, filepath.Clean(root)) {
        return "", fmt.Errorf("path escapes project root")
    }
    return abs, nil
}
```

---

### 8.3 Git Add Safety

**File:** `internal/git/commit.go`

Change:
```go
args := append([]string{"-C", root, "add"}, files...)
```

To:
```go
args := append([]string{"-C", root, "add", "--"}, files...)
```

The `--` prevents filenames from being interpreted as options.

---

## Execution Order

Recommended implementation sequence:

1. **Week 1: Critical fixes (P1)**
   - 1.1 Fix YAML template
   - 1.2 Fix TOML config
   - 1.5 Fix NextID error handling
   - 1.6 Handle Execute error

2. **Week 2: Wiring (P1 + P4)**
   - 1.3 Wire CLI subcommands
   - 1.4 Fix run command
   - 4.1 Add init command
   - 4.2 Add new command
   - 4.3 Add show command

3. **Week 3: Architecture (P2)**
   - 2.1 Add application layer
   - 2.2 Add config loading
   - 2.3 Add root detection

4. **Week 4: Data model + validation (P3 + P4)**
   - 3.1 Add governance fields
   - 4.4 Add validate command
   - 4.5 Add research command

5. **Week 5: Agent improvements (P2 + P5)**
   - 2.4 Improve agent launching
   - 5.1 Improve brief format

6. **Week 6: TUI (P6)**
   - 6.1 Add TUI state model
   - 6.2 Wire TUI as default
   - 6.3 Implement two-pane layout

7. **Ongoing: Testing + Security (P7 + P8)**
   - Add tests alongside each feature
   - Security hardening before any "run agent" features go live

---

## Notes

- Each task should follow TDD: write failing test first
- Auto-commit after each logical change
- Update AGENTS.md if conventions change
- Keep CLI/TUI parity via shared app layer
