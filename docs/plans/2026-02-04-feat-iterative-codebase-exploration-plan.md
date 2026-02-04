---
title: "feat: Claude Code-style iterative codebase exploration"
type: feat
date: 2026-02-04
deepened: 2026-02-04
reviewed: 2026-02-04
brainstorm: docs/brainstorms/2026-02-04-iterative-codebase-exploration-brainstorm.md
---

# Iterative Codebase Exploration for Gurgeh

## Review Summary (Post-Deepening)

**Reviewed by:** DHH, Code Simplicity, Pattern Recognition
**Consensus:** Simplify further. Ship 25 lines, see what Claude outputs, then add structure.

### Key Decisions from Review

1. **v1 is now 25 lines** - Return `map[string]any`, not typed structs
2. **No Explorer interface** - Premature abstraction deleted
3. **No AgentRunner** - Use simple `exec.Command` (matches existing `detect.go` pattern)
4. **Security sanitization deferred** - Developer exploring their own codebase
5. **Delete speculative code** - Phase 2/3/4 moved to "Future Ideas" with no code samples

---

## Overview

Replace single-shot LLM scanning with Claude Code subprocess invocation for iterative, tool-assisted codebase exploration.

**Goal**: PRD evidence quality that matches interactive Claude Code exploration.

---

## Implementation (25 lines)

### Phase 0: Verify Subprocess Works (MANDATORY)

```bash
claude -p "What is 2+2? Reply with just the number." --output-format json --print
```

**Do NOT proceed until this works.**

### Phase 1: Ship It

Create `internal/gurgeh/exploration/explore.go`:

```go
package exploration

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "time"
)

// Explore runs Claude Code and returns parsed output.
// Returns map[string]any - don't define types until we see real output.
func Explore(ctx context.Context, cwd string) (map[string]any, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
    defer cancel()

    cmd := exec.CommandContext(ctx, "claude",
        "-p", prompt,
        "--output-format", "json",
        "--print",
    )
    cmd.Dir = cwd

    out, err := cmd.Output()
    if err != nil {
        return nil, fmt.Errorf("claude failed: %w", err)
    }

    var result map[string]any
    if err := json.Unmarshal(out, &result); err != nil {
        return nil, fmt.Errorf("parse failed: %w", err)
    }
    return result, nil
}

const prompt = `Explore this codebase for PRD generation.

Find:
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this?

Extract VERBATIM QUOTES as evidence. Skip .env files.

Return JSON: {"vision": {...}, "problem": {...}, "users": {...}}`
```

### Phase 2: Wire It Up

~~Modify `internal/autarch/agent/scan.go` to call `exploration.Explore()` and merge results.~~

**v1 Update:** Phase 2 deferred. The `exploration.Explore()` function is available in `internal/gurgeh/exploration/` and can be called from Gurgeh-specific code (TUI layer, arbiter, or CLI). Integration into the scan pipeline will be done when we see what Claude outputs and decide how to merge it with existing scan results.

---

## Acceptance Criteria (v1)

- [x] Run Claude Code subprocess
- [x] Get JSON back
- [x] Parse it into map[string]any
- [x] 10-minute timeout
- [x] Error message if Claude Code not installed

**That's it.** Everything else is v2.

---

## Original Intent (Preserved for Future Iterations)

This section captures the full vision from brainstorming and research, intentionally deferred to keep v1 minimal.

### Target State (Claude Code Parity)

The ultimate goal is PRD evidence quality that matches interactive Claude Code exploration:
- **Multi-round autonomous exploration** - Agent decides what to investigate next
- **Parallel Task subagents** - Docs Analyst, Structure Mapper, Evidence Hunter running concurrently
- **No turn limits** - Explore until satisfied (like `/init`), 10 min emergency timeout only
- **Rich mental model** - Architecture understanding + PRD evidence interleaved

### Architecture Vision (Deferred)

```
┌─────────────────────────────────────────────────────────────────┐
│ Gurgeh TUI/CLI                                                  │
│                                                                 │
│  1. Check cache (git hash + dirty state)                        │
│  2. Invoke Claude Code subprocess                               │
│  3. Stream progress events to TUI                               │
│  4. Parse final JSON → PhaseArtifacts                           │
│  5. Sanitize output (filter secrets)                            │
│  6. Merge with deterministic tech stack                         │
│  7. Inject into SprintState                                     │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Claude Code Subprocess                                          │
│                                                                 │
│  claude -p <prompt> --output-format stream-json --verbose       │
│                                                                 │
│  Spawns parallel Task subagents:                                │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐               │
│  │ Docs Analyst│ │Structure Map│ │Evidence Hunt│               │
│  │ README, docs│ │ dir tree    │ │ tests, UI   │               │
│  └─────────────┘ └─────────────┘ └─────────────┘               │
│                                                                 │
│  Synthesizes → JSON output                                      │
└─────────────────────────────────────────────────────────────────┘
```

### Key Decisions from Brainstorm

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Loop control | Fully autonomous | User sees final mental model, not each round |
| Tool access | Claude Code subprocess | Already has tool-calling, no reimplementation |
| Exploration strategy | Interleaved | Build architecture understanding AND gather PRD evidence together |
| Output target | Rich PhaseArtifacts | Vision, Problem, Users with deep evidence |
| Timeout strategy | /init-style | No turn limits - explore until satisfied. 10 min emergency timeout only |
| Caching | Git hash keyed | Invalidate on any commit, cheap for repeated iterations |
| Progress display | Verbose streaming | Show tool calls AND findings via stream-json |
| Error handling | Save partial + warn | Keep gathered evidence, warn if incomplete |
| Parallelism | Task subagents | Claude Code spawns parallel Explore agents, synthesizes results |

### Research Insights (Preserved)

**Agent-Native Architecture:**
- Features are outcomes achieved by agents operating in a loop
- The exploration prompt should describe the outcome, not the steps
- Completion signal: Agent writes output file or emits "result" event—not timeout detection

**Security (for when caching is added):**
- Prompt instructions aren't a security control - need post-processing filter
- Pattern: redact API keys, passwords, private keys before caching
- Patterns discovered: `ghp_*`, `sk-*`, `AKIA*`, `-----BEGIN PRIVATE KEY-----`

**Performance (for large codebases):**
- Go 1.20+ `cmd.Cancel` for SIGTERM, `cmd.WaitDelay` for grace period
- Copy `scanner.Bytes()` before channel send (buffer reuse)
- Consider compression for cached results (70-90% size reduction)

**Silent Failure Prevention (for streaming):**
- Count malformed JSON lines, fail if > 10
- Always check `scanner.Err()` after loop
- Handle "error" event type from Claude Code
- Mark results as `IsPartial` when using fallback parsing

### Expansion Path

When v1 limitations emerge:

| Trigger | Add |
|---------|-----|
| Need typed access to evidence | Define `ExplorationResult` struct based on real output |
| Adding caching | Security sanitization (post-processing filter) |
| Exploration takes > 30s | Streaming with progress callbacks |
| Repeated scans on same commit | Git-hash caching |
| Multiple agent types | AgentRunner abstraction from `pkg/agenttargets` |

---

## Future Ideas (No Code Until Needed)

Brief list (details above in "Original Intent"):

1. **Typed structs** - Once we see real Claude output patterns
2. **Security sanitization** - When caching is added
3. **Streaming** - When explorations are slow
4. **Caching** - When repeated scans become painful
5. **AgentRunner** - When we add more agent types

**Don't plan code you're not writing yet.**

---

## References

- Brainstorm: `docs/brainstorms/2026-02-04-iterative-codebase-exploration-brainstorm.md`
- Existing subprocess pattern: `internal/autarch/agent/detect.go:209-286`
- Claude Code CLI: https://code.claude.com/docs/en/cli-reference

---

## Review History

| Date | Reviewers | Outcome |
|------|-----------|---------|
| 2026-02-04 | DHH, Simplicity, Pattern | Simplified from 70 lines to 25 lines |

**DHH's verdict:** "Ship this. See what Claude outputs. Adjust."
