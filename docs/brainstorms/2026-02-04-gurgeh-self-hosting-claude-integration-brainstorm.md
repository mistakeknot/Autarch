# Brainstorm: Gurgeh Self-Hosting via Claude Code Integration

**Date:** 2026-02-04
**Status:** Complete
**Next Step:** `/workflows:plan` to formalize implementation phases

---

## What We're Building

Make Gurgeh capable of generating specs/PRDs good enough that **Claude Code succeeds on the first try**. Gurgeh is the orchestrator/context-provider; Claude Code (and potentially other AI backends) is the cognitive engine.

### The Vision

```
User Intent → Gurgeh Sprint → Spec → Brief Decomposition → Claude Code Execution
                  ↑                        ↑                      ↑
            Claude generates          Claude decomposes      Claude implements
            phase content             into tasks             the code
```

**Key Insight:** Gurgeh doesn't execute anything. It produces:
1. **Prompts** - Structured instructions for AI backends
2. **Context** - Relevant code/docs excerpts
3. **Acceptance criteria** - How the AI knows it's done

---

## Why This Approach: Layered Integration

We evaluated three approaches:

| Approach | Description | Verdict |
|----------|-------------|---------|
| **Layered Integration** | Progressive layers, ship incrementally | ✅ **Selected** |
| Unified Cognitive Core | Single abstraction, all-or-nothing | Clean but high upfront cost |
| Phase-Centric Pipeline | Per-phase hooks, 32 potential calls | Over-engineered for v1 |

### Why Layered Integration Wins

1. **Ships incrementally** - Each layer delivers standalone value
2. **Proves value first** - Measure impact before adding complexity
3. **Failure isolation** - One layer failing doesn't break others
4. **Multi-backend ready** - Shared `pkg/agentcall/` foundation

### The Four Layers

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

**Implementation Order:** Layer 1 → 2 → 3 → 4 (each builds on foundation)

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Architecture** | Layered Integration | Ship fast, prove value incrementally |
| **AI Backend** | Multi-backend via CLI subprocess | Leverage existing agent infrastructure |
| **Primary Backend** | Claude Code first | Best reasoning, most capable |
| **Interface Boundary** | Autarch handles abstraction | `pkg/agentcall/` shared across modules |
| **Thinking Shapes** | Configurable per phase | Allow enabling/disabling shape preambles |
| **Success Metric** | Claude Code evaluates brief quality | Recursive: AI judges AI output |
| **Consistency Checking** | High priority, semantic | Replace keyword matching with Claude reasoning |

---

## Current State vs. Target State

### What Exists Today

| Component | Status | Location |
|-----------|--------|----------|
| 8-phase Arbiter sprint | ✅ Complete | `internal/gurgeh/arbiter/` |
| Template-based generation | ✅ Works but basic | `internal/gurgeh/arbiter/generator.go` |
| Thinking shapes | ✅ Complete | `pkg/thinking/` |
| Consistency checkers | ⚠️ Keyword-based only | `internal/gurgeh/arbiter/consistency/` |
| Claude subprocess | ✅ Exists (exploration only) | `internal/gurgeh/exploration/explore.go` |
| Subagent profiles | ✅ Defined | `internal/gurgeh/agents/agents.go` |
| Spec schema | ✅ Complete | `internal/gurgeh/specs/schema.go` |
| Brief types | ✅ Exist | `internal/gurgeh/brief/brief.go` |

### What's Missing (The Gaps)

| Gap | Impact | Layer |
|-----|--------|-------|
| **Brief Decomposition** | Can't self-host without it | Layer 1 |
| **Sprint AI generation** | Quality limited by templates | Layer 2 |
| **Semantic consistency** | Misses subtle conflicts | Layer 3 |
| **Quality evaluation loop** | No feedback before export | Layer 4 |
| **Multi-backend abstraction** | Claude-only currently | Foundation |

---

## Opportunities Discovered

### Beyond the Original Plan

The research revealed these additional opportunities:

1. **Sprint Phase Enhancement** (Layer 2)
   - Current: Templates + user input
   - Target: Claude generates draft content per phase
   - Benefit: Higher quality specs from the start

2. **Semantic Consistency** (Layer 3)
   - Current: Keyword overlap + regex patterns
   - Target: Claude evaluates phase-to-phase coherence
   - Benefit: Catches subtle contradictions

3. **Quality Evaluation Loop** (Layer 4)
   - Current: No quality gate before export
   - Target: Claude scores briefs, iterates if below threshold
   - Benefit: Self-improving output quality

4. **Thinking Shape Integration**
   - Current: Static assignment per phase
   - Target: Configurable, can enable/disable for Claude calls
   - Benefit: Experiment with Claude + shapes vs. Claude alone

5. **Evidence Gathering Enhancement**
   - Current: Only first 3 phases get evidence
   - Target: Claude explores codebase for all 8 phases
   - Benefit: Grounded specs throughout

---

## Multi-Backend Design

### CLI Subprocess Pattern

```go
// pkg/agentcall/call.go

type Backend string

const (
    BackendClaude   Backend = "claude"
    BackendCodex    Backend = "codex"
    BackendOpenCode Backend = "opencode"
)

type CallOptions struct {
    Backend      Backend
    Prompt       string
    OutputFormat string        // "json" or "text"
    Timeout      time.Duration
    WorkingDir   string
    AllowedTools []string      // For backends that support tool use
}

type CallResult struct {
    Output   []byte
    IsJSON   bool
    Duration time.Duration
    Backend  Backend
}

func Call(ctx context.Context, opts CallOptions) (*CallResult, error)
```

### Backend CLI Commands

| Backend | CLI | Output Format Flag |
|---------|-----|-------------------|
| Claude Code | `claude -p <prompt> --output-format json --print` | `--output-format` |
| Codex | `codex -p <prompt> --json` | `--json` |
| OpenCode | `opencode run <prompt>` | TBD |

---

## Success Criteria

### Self-Hosting Round Trip Test

```bash
# 1. Generate spec for a Gurgeh feature
gurgeh sprint new --input "Add semantic consistency checking to Gurgeh"

# 2. Complete sprint (Claude generates phase content)

# 3. Evaluate spec quality (Layer 4)
gurgeh evaluate <spec-id>
# → Score: 0.87, ready_for_decomposition: true

# 4. Decompose into briefs (Layer 1)
gurgeh export <spec-id> --format briefs
# → .gurgeh/briefs/<spec-id>/BRIEF-001-*.md

# 5. Execute brief with Claude Code
claude -p "$(cat .gurgeh/briefs/<spec-id>/BRIEF-001-*.md)"

# 6. Verify
go test ./internal/gurgeh/...
# → PASS
```

### Quality Signals

| Signal | How Measured | Target |
|--------|--------------|--------|
| First-try success | Claude executes brief without clarifying questions | 80%+ |
| Test pass rate | Code from brief passes tests first run | 90%+ |
| Brief quality score | Claude evaluates brief completeness | 0.85+ |
| Spec-to-brief fidelity | Requirements map 1:1 to briefs | 100% |

---

## Open Questions

1. **Caching strategy** - Should Claude responses be cached? Git-hash keyed?
2. **Cost management** - 8 phases × multiple calls = expensive. Rate limiting?
3. **Fallback behavior** - What if Claude is unavailable? Template fallback?
4. **Parallel calls** - Can layers 2-4 run in parallel for independent phases?
5. **Human override** - How does user edit Claude-generated content mid-sprint?

---

## References

- Original plan: This brainstorm reviews the plan provided at session start
- Exploration plan: `docs/plans/2026-02-04-feat-iterative-codebase-exploration-plan.md`
- Existing subprocess: `internal/gurgeh/exploration/explore.go`
- Agent profiles: `internal/gurgeh/agents/agents.go`
- Thinking shapes: `pkg/thinking/shape.go`
