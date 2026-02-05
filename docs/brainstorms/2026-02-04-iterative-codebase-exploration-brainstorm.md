# Brainstorm: Iterative Codebase Exploration for Gurgeh

**Date:** 2026-02-04
**Bead:** (new - to be created)
**Status:** Ready for planning

## What We're Building

Claude Code-style iterative codebase exploration that builds a mental model through multiple rounds of tool-assisted investigation. Instead of a single-shot LLM call, the agent explores autonomously using Read/Grep/Glob tools until it understands the codebase well enough to ground PRD generation.

### Core Insight

**Don't reimplement tool-calling** - use Claude Code itself as the exploration agent. It already has:
- Full tool access (Read, Grep, Glob, Bash)
- Iterative exploration capability
- Mental model building
- Context management

Gurgeh invokes Claude Code via subprocess CLI, passes an exploration prompt, and imports the structured results.

## Why This Approach

### Current State (30-40% of Claude Code parity)
- Single-shot LLM call with ~10 files
- Deterministic tech stack detection
- Improved prompts for verbatim evidence
- Missing: iterative exploration, tool use, mental model building

### Target State (Claude Code parity)
- Multi-round autonomous exploration
- Agent decides what to investigate next
- Builds understanding progressively
- Rich evidence for PRD phases

### Why Subprocess CLI
| Considered | Verdict | Reason |
|------------|---------|--------|
| Subprocess CLI | **Chosen** | Works today, validates concept, easy upgrade path |
| SDK integration | Deferred | May not exist, tighter coupling |
| MCP server | Deferred | More complex, less mature |
| Reimplement tools | Rejected | Reinventing the wheel |

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Loop control | Fully autonomous | User sees final mental model, not each round |
| Tool access | Claude Code subprocess | Already has tool-calling, no reimplementation |
| Exploration strategy | Interleaved | Build architecture understanding AND gather PRD evidence together |
| Output target | Rich PhaseArtifacts | Vision, Problem, Users, Scope with deep evidence |
| Integration method | Subprocess CLI | Works today, validates concept |
| Timeout strategy | /init-style | No turn limits - explore until satisfied. 10 min emergency timeout only. |
| Cost control | Trust the agent | No artificial limits. Agent scales exploration naturally like /init does. |
| Caching | Git hash keyed | Invalidate on any commit, cheap for repeated iterations |
| Progress display | Verbose streaming | Show tool calls AND findings via stream-json |
| Error handling | Save partial + warn | Keep gathered evidence, warn if incomplete |
| Parallelism | Task subagents | Claude Code spawns parallel Explore agents, synthesizes results |

## Exploration Flow (Inside Claude Code)

### Parallel Subagent Architecture

Claude Code orchestrator spawns parallel `Task` subagents for speed:

```
┌─────────────────────────────────────────────────────────────────┐
│ Claude Code Orchestrator                                         │
│                                                                 │
│ Turn 1: Launch parallel subagents                               │
│   [Task: Explore] "Analyze docs for Vision/Problem/Users"       │
│   [Task: Explore] "Map directory structure and entry points"    │
│   [Task: Explore] "Search for test descriptions and UI strings" │
│                                                                 │
│ Turn 2: Synthesize all findings into PhaseArtifacts             │
└─────────────────────────────────────────────────────────────────┘
```

### Subagent Responsibilities

| Subagent | Focus | Files/Patterns |
|----------|-------|----------------|
| **Docs Analyst** | Vision, Problem, Users evidence | README, CLAUDE.md, AGENTS.md, docs/ |
| **Structure Mapper** | Architecture, entry points, patterns | Directory tree, main.*, index.*, cmd/ |
| **Evidence Hunter** | User scenarios, test coverage, UI strings | *_test.*, Grep for errors/help text |

### Why Parallel Subagents (vs /init's simpler approach)

Note: Claude Code's `/init` doesn't use subagents - it just makes parallel tool calls. We chose subagents because:

| Aspect | /init Approach | Our Subagent Approach |
|--------|---------------|----------------------|
| **Output** | CLAUDE.md (instructions) | PhaseArtifacts (structured PRD evidence) |
| **Depth** | Surface-level (commands, structure) | Deep (verbatim quotes, user evidence) |
| **Focus** | Single purpose | 3 specialized hunters |
| **Thoroughness** | Good for setup | Better for PRD grounding |

The PRD evidence task is more complex than generating CLAUDE.md. Subagents let us:
- **Docs Analyst**: Deep-read README/docs for Vision/Problem/Users
- **Structure Mapper**: Trace architecture patterns thoroughly
- **Evidence Hunter**: Exhaustively search for user scenarios

### No Sequential Fallback

Unlike the original design with complexity tiers, we now let Claude decide:
- Agent uses subagents when beneficial
- Agent uses direct tool calls when simpler
- No artificial constraints - explore until satisfied

## Integration Points

### Invocation (Gurgeh → Claude Code)
```go
cmd := exec.Command("claude",
    "-p", explorationPrompt,
    "--output-format", "stream-json",
    "--print",
    "--verbose",
)
cmd.Dir = targetCodebase  // Set working directory

// No --max-turns: let Claude explore until satisfied (like /init)
// 10 min context timeout as emergency stop
ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
defer cancel()
```

### Prompt Structure
```
You are exploring a codebase to gather evidence for PRD generation.

TARGET: Build a mental model and extract evidence for:
- Vision: What does this project do? Why does it exist?
- Problem: What pain points does it solve?
- Users: Who uses this? What are their needs?
- Scope: What's in vs out of scope?

APPROACH: Use parallel Task subagents for speed.

Launch these THREE Task agents IN PARALLEL (same message):

1. Task(Explore): "Analyze documentation files (README.md, CLAUDE.md, AGENTS.md,
   docs/*.md) for project Vision, Problem statements, and Users. Extract VERBATIM
   QUOTES as evidence. Return JSON: {vision: {...}, problem: {...}, users: {...}}"

2. Task(Explore): "Map directory structure and architecture. Find entry points
   (main.go, index.ts, app.py). Detect patterns (monorepo, cmd-internal, MVC).
   Return JSON: {architecture: '...', entry_points: [...], patterns: [...]}"

3. Task(Explore): "Search for user-facing content: test descriptions, error messages,
   CLI help text, UI strings. Return JSON: {test_evidence: [...], ui_strings: [...],
   user_scenarios: [...]}"

After ALL subagents complete, SYNTHESIZE their findings into final output.

OUTPUT FORMAT:
{
  "mental_model": {
    "architecture": "...",
    "key_patterns": [...],
    "entry_points": [...]
  },
  "phase_artifacts": {
    "vision": { "summary": "...", "evidence": [...] },
    "problem": { "summary": "...", "evidence": [...] },
    "users": { "personas": [...], "evidence": [...] }
  }
}
```

### Result Import (Claude Code → Gurgeh)
- Parse JSON output
- Map to existing `PhaseArtifacts` structure
- Merge with deterministic tech stack evidence
- Inject into sprint state

## Timeout and Cost Control (Refined)

### Match /init Approach: No Turn Limits

After investigating Claude Code's `/init` command, we learned:
- `/init` has **no turn limits** - it explores until satisfied
- For a 3-file project: 7 tool calls
- For larger projects: scales up naturally (20-50+ calls)
- Uses **parallel tool calls** when tasks are independent

**Our approach matches this:**

| Aspect | /init | Our Design |
|--------|-------|------------|
| Turn limits | None | **None** |
| Safety timeout | Implicit | 10 min max (emergency stop) |
| Parallel calls | Yes (3 Globs at once) | Yes (via subagents) |
| Scaling | Adapts to codebase | Same |

**Cost control**: No artificial limits. Claude will naturally use fewer turns for simple codebases. Emergency timeout prevents runaway costs.

**Rationale**: Arbitrary turn limits (5, 10, 20) were guesswork. Better to trust the agent's judgment, same as `/init` does.

## Progress Visibility (Refined)

### Streaming via `--output-format stream-json`

Claude Code CLI supports realtime JSON event streaming. Gurgeh parses these events and displays:

**Tool calls:**
```
[Exploring] Reading README.md...
[Exploring] Grepping for "test" patterns...
[Exploring] Checking directory structure...
```

**Key findings as they emerge:**
```
[Exploring] Vision: "AI agent development tools"
[Exploring] Problem: Found 3 pain points
[Exploring] Users: Detected 2 personas
```

### Implementation

```go
cmd := exec.Command("claude",
    "-p", prompt,
    "--output-format", "stream-json",  // Realtime events
    "--print",                          // Non-interactive
    "--verbose",                        // Required for stream-json
)
cmd.Dir = targetCodebase
stdout, _ := cmd.StdoutPipe()

// Parse JSON events as they arrive
scanner := bufio.NewScanner(stdout)
for scanner.Scan() {
    event := parseStreamEvent(scanner.Text())
    switch event.Type {
    case "tool_use":
        progress(fmt.Sprintf("Reading %s...", event.ToolInput.Path))
    case "text":
        if finding := extractFinding(event.Text); finding != nil {
            progress(fmt.Sprintf("%s: %s", finding.Phase, finding.Summary))
        }
    }
}
```

## Error Handling (Refined)

### Strategy: Save Partial Results + Warn

If exploration fails or times out, keep whatever evidence was gathered and warn the user.

| Failure Mode | Handling |
|--------------|----------|
| **Claude Code not installed** | Fatal error: "Claude Code CLI required. Install with: npm i -g @anthropic-ai/claude-code" |
| **Timeout exceeded** | Save partial results, warn: "Exploration incomplete (timeout). Found N evidence items." |
| **Invalid JSON output** | Try to extract partial JSON, fall back to text parsing, warn if incomplete |
| **Empty results** | Warn: "Exploration found no evidence. Try manual input." |
| **Process crash** | Check for any stdout captured, save if parseable, warn about incomplete exploration |

### Implementation

```go
result, err := runExploration(ctx, cwd, budget)
if err != nil {
    // Check if we have partial results
    if result != nil && len(result.Evidence) > 0 {
        result.Warnings = append(result.Warnings,
            fmt.Sprintf("Exploration incomplete: %v. Found %d evidence items.", err, len(result.Evidence)))
        return result, nil  // Return partial results
    }
    return nil, fmt.Errorf("exploration failed: %w", err)
}
```

## Caching Strategy (Refined)

### Git Hash Caching

Cache exploration results keyed by git HEAD. Invalidate automatically when any commit is made.

**Cache key:** `sha256(git HEAD + absolute cwd path)`

**Benefits:**
- Fresh results when code changes
- Cheap for repeated PRD iterations on same commit
- Natural invalidation - no stale data concerns
- User can force refresh by making any commit

**Storage:** `.gurgeh/exploration_cache/` directory

```go
// Cache location: .gurgeh/exploration_cache/{hash}.json
func explorationCachePath(cwd string) string {
    head := gitHead(cwd)
    hash := sha256(head + cwd)
    return filepath.Join(cwd, ".gurgeh", "exploration_cache", hash[:16]+".json")
}
```

## Open Questions

None - all questions resolved!

## Success Criteria

- [ ] Claude Code subprocess invocation works from Gurgeh
- [ ] Exploration prompt produces structured JSON output
- [ ] PhaseArtifacts populated with rich evidence from exploration
- [ ] Evidence quality matches or approaches interactive Claude Code exploration
- [ ] Verbatim quotes extracted from codebase (not paraphrased)
- [ ] Works on codebases of varying sizes (small to monorepo)

## Next Steps

→ Proceed to `/workflows:plan` for implementation details
