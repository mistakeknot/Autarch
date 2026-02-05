---
title: "feat: Gurgeh Self-Hosting via Claude Code Integration"
type: feat
date: 2026-02-04
status: reviewed
deepened: 2026-02-04
reviewed: 2026-02-04
reviewers: [dhh-rails-reviewer, kieran-rails-reviewer, code-simplicity-reviewer]
brainstorm: docs/brainstorms/2026-02-04-gurgeh-self-hosting-claude-integration-brainstorm.md
---

# Gurgeh Self-Hosting via Claude Code Integration

## Overview

Make Gurgeh capable of generating specs/PRDs good enough that **Claude Code succeeds on the first try**. Gurgeh formats prompts → Claude thinks → Gurgeh persists results.

**MVP:** Brief Decomposition only. ~80 lines of code. One file.

---

## Problem Statement

Gurgeh generates excellent *specs*, but specs aren't agent-consumable. The critical gap: specs require human translation to actionable tasks.

---

## Proposed Solution

### The Simplest Thing That Works

```go
// internal/gurgeh/brief/decompose.go (~80 lines total)

// Brief describes an outcome for Claude Code to achieve
type Brief struct {
    Title    string   `json:"title"`    // What to build
    Outcome  string   `json:"outcome"`  // What success looks like
    Criteria []string `json:"criteria"` // How to verify it's done
}

// Decompose calls Claude Code to break a spec into briefs
func Decompose(spec *specs.Spec, workingDir string) ([]Brief, error) {
    prompt := buildPrompt(spec)

    cmd := exec.Command("claude", "-p", prompt, "--output-format", "json", "--print")
    cmd.Dir = workingDir

    out, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("claude failed: %w", err)
    }

    var result struct {
        Briefs []Brief `json:"briefs"`
    }
    if err := json.Unmarshal(out, &result); err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }

    return result.Briefs, nil
}

// SaveBriefs writes each brief as markdown
func SaveBriefs(specID string, briefs []Brief) error {
    dir := filepath.Join(".gurgeh", "briefs", specID)
    os.MkdirAll(dir, 0755)

    for i, b := range briefs {
        filename := fmt.Sprintf("BRIEF-%03d-%s.md", i+1, slug(b.Title))
        content := formatBrief(b)
        os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
    }
    return nil
}
```

**That's the entire implementation.** No abstractions. No shared packages. No layers.

### Files

| File | Purpose | Lines |
|------|---------|-------|
| `internal/gurgeh/brief/decompose.go` | Everything | ~80 |

### CLI

```bash
gurgeh export <spec-id> --format briefs
```

---

## Acceptance Criteria

- [x] `gurgeh export <spec-id> --format briefs` generates briefs
- [x] Claude subprocess returns JSON
- [x] Briefs saved to `.gurgeh/briefs/<spec-id>/BRIEF-XXX-<slug>.md`
- [x] Brief markdown can be piped to Claude: `claude -p "$(cat brief.md)"`
- [x] `go test ./internal/gurgeh/brief/...` passes

---

## Verification

```bash
# 1. Generate spec
gurgeh sprint new --input "Add semantic consistency checking"

# 2. Complete sprint

# 3. Decompose
gurgeh export <spec-id> --format briefs

# 4. Execute first brief
claude -p "$(cat .gurgeh/briefs/<spec-id>/BRIEF-001-*.md)"

# 5. Verify
go test ./internal/gurgeh/...
```

---

## Success Metrics

| Metric | Target |
|--------|--------|
| First-try success | 80%+ briefs execute without clarification |
| Test pass rate | 90%+ code passes tests first run |

---

## What to Add Later (Only When Triggered)

| Trigger | Add |
|---------|-----|
| Second backend (Codex) | Extract `pkg/agentcall/` |
| Brief quality < 70% | Quality evaluation prompt |
| Consistency issues | Semantic checking |
| Templates rejected | Claude generation |

**Don't add these yet.** Ship the 80 lines, measure what's missing.

---

## Reviewer Consensus (2026-02-04)

All three reviewers independently recommended the same simplifications:

| Reviewer | Verdict | Key Point |
|----------|---------|-----------|
| **DHH** | "Stop planning. Start shipping." | 9 fields → 3 fields, ~100 lines |
| **Kieran** | "85% there" | TaskBrief too prescriptive, subprocess over-engineered |
| **Simplicity** | "30 lines of plan, 80 lines of code" | 4 files → 1 file |

---

---

# Original Intent (Preserved for Future Iterations)

> *Per [preserve-original-intent-after-simplification pattern](../solutions/patterns/preserve-original-intent-after-simplification.md): When reviewers recommend simplifying, don't delete the research. Preserve it for when triggers fire.*

## Target State (Vision)

When all triggers fire, Gurgeh becomes a full cognitive orchestrator:

```
┌─────────────────────────────────────────────┐
│              Gurgeh Orchestrator            │
├─────────────────────────────────────────────┤
│  Layer 4: Quality Evaluation Loop           │  ← Claude scores briefs, iterates
│  Layer 3: Semantic Consistency              │  ← Claude checks phase coherence
│  Layer 2: Sprint Phase Generation           │  ← Claude generates draft content
│  Layer 1: Brief Decomposition               │  ← Claude breaks specs into tasks
├─────────────────────────────────────────────┤
│         pkg/agentcall/ (CLI Abstraction)    │  ← Supports claude/codex/etc
└─────────────────────────────────────────────┘
```

## Architecture Vision (Deferred)

### Multi-Backend Interface

```go
// pkg/agentcall/backend.go
type Backend interface {
    Name() string
    Available() error
    BuildCommand(req Request) (Command, error)
    ParseOutput(r io.Reader) (<-chan Message, <-chan error)
}

type Request struct {
    Prompt       string
    OutputFormat string // "json" or "text"
    Timeout      time.Duration
    WorkingDir   string
    AllowedTools []string
}
```

### Rich TaskBrief (Agent-Native)

Per agent-native architecture review, outcome-based briefs with judgment criteria:

```go
type TaskBrief struct {
    ID                 string   `json:"id"`
    Title              string   `json:"title"`
    Outcome            string   `json:"outcome"`            // What success looks like
    Constraints        string   `json:"constraints"`        // Boundaries, what to avoid
    Judgment           string   `json:"judgment"`           // Edge case guidance
    AcceptanceCriteria []string `json:"acceptance_criteria"`
    StartingPoints     []string `json:"starting_points"`    // Suggested files
    SourceCUJ          string   `json:"source_cuj"`         // Traceability
    Dependencies       []string `json:"dependencies"`
}
```

**Reviewer feedback:** Too prescriptive for v1. Start with 3 fields, add when needed.

## Key Decisions from Brainstorm

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Architecture | Layered Integration | Ship incrementally, prove value first |
| AI Backend | Multi-backend via CLI subprocess | Leverage existing infrastructure |
| Primary Backend | Claude Code first | Best reasoning, most capable |
| Interface Boundary | CLI subprocess | Autarch handles abstraction |
| Thinking Shapes | Configurable per phase | Experiment with Claude + shapes |
| Success Metric | Claude evaluates brief quality | Recursive: AI judges AI output |
| Consistency | Semantic (high priority) | Replace keyword matching with reasoning |

## Research Insights (Preserved)

### Subprocess Best Practices

**Process Group Isolation (kill child processes on timeout):**
```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,  // Create new process group
}

cmd.Cancel = func() error {
    if cmd.Process == nil {
        return nil
    }
    pgid := -cmd.Process.Pid
    return syscall.Kill(pgid, syscall.SIGTERM)
}
cmd.WaitDelay = 30 * time.Second  // Grace period before SIGKILL
```

**Large buffer for AI output:**
```go
scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)  // 1MB initial, 10MB max
```

**Reviewer feedback:** Overkill for MVP. `cmd.Output()` is fine.

### Concurrency Safety

Per state-pointer-escape learning, **clone SprintState before Claude calls**:

```go
func (o *Orchestrator) prepareDecomposition() (*DecomposeInput, error) {
    o.mu.Lock()
    if o.state == nil {
        o.mu.Unlock()
        return nil, ErrNoState
    }
    state := o.state.Clone()  // Deep copy
    o.mu.Unlock()

    return &DecomposeInput{
        Spec:     state.ExportSpec(),
        Evidence: state.Evidence,
    }, nil
}
```

**Test requirement:** `go test -race ./internal/gurgeh/...` must pass.

### Error Handling

Per non-fatal-error-logging learning, use **visible continuation**:

```go
result, err := decompose.Decompose(ctx, spec)
if err != nil {
    fmt.Fprintf(os.Stderr, "warning: decomposition failed: %v\n", err)
    return promptManualBriefCreation()
}
```

### Security Hardening (Deferred)

Per security review, address these before multi-user/production:

**Sensitive File Exclusion:**
```go
var sensitivePatterns = []string{
    ".env*", "*.pem", "*.key", "*.crt",
    "*credentials*", "*secrets*",
    "*_rsa", "*_dsa", "*_ecdsa",
    ".aws/*", ".netrc", ".npmrc",
}
```

**Output Sanitization:**
```go
var secretPatterns = []*regexp.Regexp{
    regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),           // GitHub PAT
    regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),            // OpenAI key
    regexp.MustCompile(`AKIA[A-Z0-9]{16}`),              // AWS access key
}

func sanitizeOutput(content string) string {
    for _, pattern := range secretPatterns {
        content = pattern.ReplaceAllString(content, "[REDACTED]")
    }
    return content
}
```

**Reviewer feedback:** Fear-driven development for a single-user CLI. Defer.

### Performance Batching (Deferred)

For Layer 3, batch all consistency checks into single Claude call:

```go
const consistencyBatchPrompt = `Evaluate semantic consistency for these phase pairs.

{{range .Pairs}}
## Pair: {{.A.Name}} ↔ {{.B.Name}}
<section_a>{{.A.Content}}</section_a>
<section_b>{{.B.Content}}</section_b>

{{end}}

Return JSON with conflicts for ALL pairs in a single response.`
```

**Token Budget (Full Vision):**

| Operation | Input Tokens | Output Tokens | Calls | Total |
|-----------|-------------|---------------|-------|-------|
| Brief Decomposition | 5,000 | 3,000 | 1 | 8,000 |
| Consistency (batched) | 10,000 | 2,000 | 1 | 12,000 |
| Quality Evaluation | 3,000 | 1,000 | 1 | 4,000 |
| **Full Sprint Total** | | | | **~24,000** |

### Import Cycle Prevention

Per arbiter-architecture learning, use adapter sub-packages if needed:

```
internal/gurgeh/arbiter/
├── types.go            # Core types
├── orchestrator.go     # Uses local adapters
├── decompose/          # Local adapter (if needed)
└── consistency/        # Already exists as local adapter
```

## Expansion Path

| Trigger | Add | Complexity |
|---------|-----|------------|
| Second backend requested | Extract `pkg/agentcall/` abstraction | +100 lines |
| Brief quality < 70% first-try success | Add quality evaluation prompt | +50 lines |
| Consistency issues in decomposed briefs | Add semantic consistency checking | +80 lines |
| Template drafts consistently rejected | Add Claude generation path | +100 lines |
| Exploration cache needed | Add git-hash keyed caching | +60 lines |
| Multi-tenant use | Add request queuing, rate limiting | +200 lines |

## Full File List (Deferred)

When all triggers fire:

| File | Purpose | Lines |
|------|---------|-------|
| `pkg/agentcall/backend.go` | Backend interface | ~50 |
| `pkg/agentcall/claude.go` | Claude implementation | ~100 |
| `pkg/agentcall/options.go` | CallOptions, CallResult | ~30 |
| `internal/gurgeh/decompose/decompose.go` | Decomposition engine | ~100 |
| `internal/gurgeh/decompose/types.go` | TaskBrief struct | ~40 |
| `internal/gurgeh/decompose/prompt.go` | Prompt template | ~30 |
| `internal/gurgeh/consistency/semantic.go` | Claude consistency | ~80 |
| `internal/gurgeh/evaluate/quality.go` | Quality scoring | ~60 |
| `internal/gurgeh/generate/claude.go` | Claude draft generation | ~80 |
| `internal/gurgeh/cli/commands/export_briefs.go` | CLI command | ~60 |

**Total when fully expanded:** ~630 lines (vs 80 for MVP)

## Risk Analysis

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Claude subprocess hangs | Medium | High | Timeout (use context.WithTimeout) |
| JSON parse failures | Medium | Medium | Log warning, return error |
| State race during Claude calls | High | High | Clone SprintState before call |
| Secrets in output | Medium | High | Defer (single-user CLI) |
| Over-engineering | High | Medium | **Ship 80 lines first** |

## References

### Internal
- Brainstorm: `docs/brainstorms/2026-02-04-gurgeh-self-hosting-claude-integration-brainstorm.md`
- Existing subprocess: `internal/gurgeh/exploration/explore.go:18-104`
- Brief types: `internal/gurgeh/brief/brief.go:5-35`
- Consistency engine: `internal/gurgeh/arbiter/consistency/engine.go:48-66`

### Institutional Learnings Applied
- Import cycle prevention: `docs/solutions/patterns/arbiter-spec-sprint-architecture.md`
- State pointer escape: `docs/solutions/runtime-errors/arbiter-state-pointer-escape-20260201.md`
- Non-fatal error logging: `docs/solutions/patterns/oracle-review-issues-3-6-20260201.md`

### Research Insights Applied
- Subprocess process groups: Go 1.20+ `cmd.Cancel` + `cmd.WaitDelay`
- Agent-native briefs: Outcomes with judgment, not step-by-step instructions
- Performance batching: Single Claude call for consistency checks
- Security: Secret exclusion, output sanitization (deferred)
