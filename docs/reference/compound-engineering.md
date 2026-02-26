# Compound Engineering Integration

Extracted from AGENTS.md. Patterns for multi-agent review, knowledge compounding, and spec analysis.

## Multi-Agent Review

PRDs and research are validated by parallel review agents:

```bash
# PRD review with multi-agent validation
gurgeh review PRD-001 --gaps

# Reviewers: Completeness, CUJ Consistency, Acceptance Criteria, Scope Creep
```

## Knowledge Compounding

Solved problems are captured in `docs/solutions/` for future reference.

**Current categories:** `runtime-errors/`, `ui-bugs/`, `patterns/`, `workflow-issues/`, `integration/`

```bash
# Before debugging - search existing solutions
grep -r "error message" docs/solutions/
grep -l "tags:.*bubble-tea" docs/solutions/**/*.md

# After fixing - capture the learning
/clavain:compound
```

**Key solutions documented:**
- Concurrency: State pointer escape, deep-copy patterns
- TUI: Message swallowing, ANSI-aware string operations, dimension mismatches
- Process: Reproduce before planning (over-engineering anti-pattern)
- Architecture: Oracle review findings, arbiter sprint patterns

## SpecFlow Gap Analysis

Detect specification gaps before implementation:

```go
analyzer := spec.NewSpecFlowAnalyzer()
result := analyzer.Analyze(spec)
// Gaps: missing_flow, unclear_criteria, edge_case, error_handling, etc.
```

## Claude Code Plugin

The `autarch-plugin/` directory provides Claude Code integration:

| Component | Purpose |
|-----------|---------|
| `/autarch:prd` | Create PRD (now uses Spec Sprint) |
| `/autarch:research` | Run Pollard research |
| `/autarch:tasks` | Generate epics from PRD |
| `/autarch:status` | Show project status |
| `autarch-mcp` | MCP server for AI agents |

See [docs/COMPOUND_INTEGRATION.md](../COMPOUND_INTEGRATION.md) for full details.
