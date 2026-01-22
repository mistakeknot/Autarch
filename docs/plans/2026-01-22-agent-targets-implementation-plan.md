# Shared Run-Target Registry Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `none (no bead in use)`

**Goal:** Implement a shared run-target registry/resolver used by Praude, Tandemonium, and Vauxhall with global defaults and per-project overrides.

**Architecture:** Add `pkg/agenttargets` to centralize target definitions, config loading, merging, and resolution. Use a global config file at `~/.config/vauxpraudemonium/agents.toml` and a project override at `.praude/agents.toml`, with compatibility support for `[agents]` in `.praude/config.toml`. Update Praude and Vauxhall to resolve through the shared package; Tandemonium resolves via project path.

**Tech Stack:** Go, TOML (BurntSushi), standard library.

---

### Task 1: Define shared target models + merge logic

**Files:**
- Create: `pkg/agenttargets/types.go`
- Create: `pkg/agenttargets/merge.go`
- Test: `pkg/agenttargets/merge_test.go`

**Step 1: Write failing test (project override beats global)**

```go
func TestMergeTargetsProjectOverridesGlobal(t *testing.T) {
    global := Registry{
        Targets: map[string]Target{
            "codex": {Name: "codex", Type: TargetDetected, Command: "codex"},
            "custom": {Name: "custom", Type: TargetCommand, Command: "/bin/custom"},
        },
    }
    project := Registry{
        Targets: map[string]Target{
            "custom": {Name: "custom", Type: TargetCommand, Command: "/bin/project-custom"},
        },
    }
    merged := Merge(global, project)
    if merged.Targets["custom"].Command != "/bin/project-custom" {
        t.Fatalf("expected project override")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/agenttargets -run TestMergeTargetsProjectOverridesGlobal`

Expected: FAIL (Merge not implemented).

**Step 3: Implement minimal types + Merge**

```go
// types.go
package agenttargets

type TargetType string
const (
    TargetDetected  TargetType = "detected"
    TargetPromptable TargetType = "promptable"
    TargetCommand   TargetType = "command"
)

type Target struct {
    Name    string
    Type    TargetType
    Command string
    Args    []string
    Env     map[string]string
}

type Registry struct {
    Targets map[string]Target
}

// merge.go
package agenttargets

func Merge(global, project Registry) Registry {
    merged := Registry{Targets: map[string]Target{}}
    for k, v := range global.Targets { merged.Targets[k] = v }
    for k, v := range project.Targets { merged.Targets[k] = v }
    return merged
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/agenttargets -run TestMergeTargetsProjectOverridesGlobal`

Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/agenttargets/types.go pkg/agenttargets/merge.go pkg/agenttargets/merge_test.go
git commit -m "feat(agenttargets): add registry types and merge"
```

---

### Task 2: Add config loading (global + project + compat)

**Files:**
- Create: `pkg/agenttargets/config.go`
- Test: `pkg/agenttargets/config_test.go`

**Step 1: Write failing test (global + project + compat)**

```go
func TestLoadRegistriesWithCompat(t *testing.T) {
    dir := t.TempDir()
    globalPath := filepath.Join(dir, "agents.toml")
    projectDir := filepath.Join(dir, "proj")
    if err := os.MkdirAll(filepath.Join(projectDir, ".praude"), 0o755); err != nil { t.Fatal(err) }

    if err := os.WriteFile(globalPath, []byte("[targets.codex]\ncommand=\"codex\"\n"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(projectDir, ".praude", "config.toml"), []byte("[agents.custom]\ncommand=\"/bin/custom\"\n"), 0o644); err != nil { t.Fatal(err) }

    global, project, err := Load(globalPath, projectDir)
    if err != nil { t.Fatal(err) }
    if _, ok := global.Targets["codex"]; !ok { t.Fatalf("expected global codex") }
    if _, ok := project.Targets["custom"]; !ok { t.Fatalf("expected project custom") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/agenttargets -run TestLoadRegistriesWithCompat`

Expected: FAIL (Load not implemented).

**Step 3: Implement loader**

- `Load(globalPath, projectRoot)` loads:
  - `globalPath` if present (TOML format `[targets.<name>]`)
  - `.praude/agents.toml` if present
  - `.praude/config.toml` `[agents]` compat if agents.toml missing
- Return `(global Registry, project Registry, error)`

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/agenttargets -run TestLoadRegistriesWithCompat`

Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/agenttargets/config.go pkg/agenttargets/config_test.go
git commit -m "feat(agenttargets): load global and project configs"
```

---

### Task 3: Add resolver API (context-aware)

**Files:**
- Create: `pkg/agenttargets/resolver.go`
- Test: `pkg/agenttargets/resolver_test.go`

**Step 1: Write failing test (context rules)**

```go
func TestResolveUsesProjectInProjectContext(t *testing.T) {
    global := Registry{Targets: map[string]Target{"custom": {Name: "custom", Type: TargetCommand, Command: "/bin/global"}}}
    project := Registry{Targets: map[string]Target{"custom": {Name: "custom", Type: TargetCommand, Command: "/bin/project"}}}
    r := NewResolver(global, project)
    got, err := r.Resolve(ProjectContext, "custom")
    if err != nil { t.Fatal(err) }
    if got.Command != "/bin/project" { t.Fatalf("expected project override") }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/agenttargets -run TestResolveUsesProjectInProjectContext`

Expected: FAIL.

**Step 3: Implement resolver**

- Context enum: `GlobalContext`, `ProjectContext`, `SpawnContext`.
- `Resolve(ctx, name)` picks:
  - Project registry for Project/Spawn contexts, fallback to global
  - Global only for Global context
- Return `ResolvedTarget{Command, Args, Env, Source}`

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/agenttargets -run TestResolveUsesProjectInProjectContext`

Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/agenttargets/resolver.go pkg/agenttargets/resolver_test.go
git commit -m "feat(agenttargets): add context-aware resolver"
```

---

### Task 4: Integrate Praude + Vauxhall + Tandemonium

**Files:**
- Modify: `internal/praude/agents/agents.go`
- Modify: `internal/praude/config/config.go`
- Modify: `internal/vauxhall/agentcmd/resolver.go`
- Modify: `internal/tandemonium/agent` (exact file(s) discovered during implementation)
- Test: update existing tests in `internal/praude/*` and `internal/vauxhall/agentcmd/*`

**Step 1: Add failing tests for shared resolution in Praude/Vauxhall**

Example (Vauxhall):
```go
func TestResolverUsesSharedTargets(t *testing.T) {
    // set up temp global config with custom target
    // ensure resolver returns it
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/vauxhall/agentcmd -run TestResolverUsesSharedTargets`

Expected: FAIL.

**Step 3: Implement integration**

- Praude: replace direct map resolution with `agenttargets.Load(...)` + resolver.
- Vauxhall: use `agenttargets` resolver; in `Resolve`, if projectPath provided use Project/Spawn context.
- Tandemonium: resolve through shared package using the project root (likely from `.praude`).

**Step 4: Run targeted tests**

Run: `go test ./internal/praude/... ./internal/vauxhall/agentcmd`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/praude internal/vauxhall/agentcmd internal/tandemonium
 git commit -m "refactor: use shared agenttargets resolver"
```

---

### Task 5: Update docs + full verification

**Files:**
- Modify: `docs/plans/2026-01-22-agent-targets-design.md` (if needed)
- Modify: `AGENTS.md` (document new config paths)

**Step 1: Update docs**

- Document global config: `~/.config/vauxpraudemonium/agents.toml`
- Document per-project overrides: `.praude/agents.toml`
- Note compatibility with `.praude/config.toml` `[agents]`

**Step 2: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 3: Commit**

```bash
git add AGENTS.md docs/plans/2026-01-22-agent-targets-design.md
 git commit -m "docs: document shared run-target config"
```

---

Plan complete and saved to `docs/plans/2026-01-22-agent-targets-implementation-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
