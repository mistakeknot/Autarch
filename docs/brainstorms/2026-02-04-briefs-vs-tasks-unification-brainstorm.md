# Brainstorm: Should Gurgeh Briefs and Coldwine Tasks Be Unified?

**Date:** 2026-02-04
**Status:** Complete - decision made

## What We're Building

A clear integration strategy between Gurgeh Briefs and Coldwine Tasks that **reduces perceived duplication while preserving tool modularity**.

### Core Constraint

Each tool must be usable standalone:
- `gurgeh` CLI works without Coldwine
- `coldwine` CLI works without Gurgeh
- Together via `autarch tui`, they're better - but not required

## Why This Approach

### The Perceived Duplication

User identified all four types of duplication:
1. **Data structure overlap** - Both have title, description/outcome, criteria
2. **Workflow redundancy** - Creating Brief then importing to Task feels like extra steps
3. **Mental model confusion** - Hard to remember when to use Brief vs Task
4. **Code maintenance** - Two similar implementations to maintain

### The Reality: Intentional Separation

After exploration, we determined these are **different concerns**:

| Aspect | Brief | Task |
|--------|-------|------|
| **Purpose** | Agent instruction | Orchestration tracking |
| **Fields** | 3 (title, outcome, criteria) | 12+ (status, assignee, worktree, session...) |
| **Storage** | Markdown files | SQLite database |
| **Lifecycle** | Stateless (create → execute → discard) | Stateful (pending → in_progress → completed) |
| **Consumer** | Claude Code (`claude -p "$(cat brief.md)"`) | Coldwine TUI/orchestrator |

The overlap in fields (title, outcome, criteria) is not redundancy - it's the **handoff contract**.

## Key Decisions

### Decision 1: Import/Export Boundary (No Shared Code)

```
Gurgeh                          Coldwine
┌─────────────┐                ┌──────────────┐
│ Spec → Brief │  ─[.md]───▶  │ Brief → Task │
│ (.gurgeh/)  │                │ (.coldwine/) │
└─────────────┘                └──────────────┘
```

- **Brief stays in Gurgeh**, stored as markdown in `.gurgeh/briefs/`
- **Task stays in Coldwine**, stored in SQLite
- **Markdown is the contract** - human-readable, versionable, inspectable
- **No compile-time coupling** - tools are genuinely standalone

### Decision 2: Reject Shared Types Package

We considered `pkg/brief.Brief` shared by both tools but rejected it:
- Creates compile-time dependency
- Breaks "just use Gurgeh" standalone promise
- Couples versioning across tools

### Decision 3: Reject Signal-Based Handoff

We considered event-based coordination but rejected it:
- Requires signal infrastructure running
- Adds complexity for simple file-based handoff
- Overkill for two-tool integration

### Decision 4: Improve Existing Integration

Coldwine already has `prd.ImportFromPRD()`. The path forward:
1. Standardize Brief markdown format
2. Add `coldwine import --from-briefs .gurgeh/briefs/<spec-id>/`
3. Keep the separation - it's a feature, not a bug

## Mental Model Clarification

For the user's reference:

> **Brief** = "What Claude Code should do" (instruction for agent execution)
> **Task** = "How to track what Claude Code is doing" (orchestration state)

These are different purposes that happen to share some fields. The shared fields (title, outcome, criteria) form the **handoff contract** - they must match so Task can track Brief execution.

## Open Questions

None - decision is clear. Implementation path:
1. Standardize Brief markdown format (already done in `formatBrief()`)
2. Add Coldwine import command for Briefs
3. Document the mental model distinction in AGENTS.md

## Approaches Considered

| Approach | Verdict | Reason |
|----------|---------|--------|
| **A: Import/Export Boundary** | ✅ Selected | Preserves modularity, zero coupling |
| **B: Shared Types Package** | ❌ Rejected | Compile-time coupling breaks standalone use |
| **C: Signal-Based Handoff** | ❌ Rejected | Overkill, adds infrastructure dependency |

## Next Steps

1. No code changes needed - the current design is correct
2. Improve documentation to clarify Brief vs Task distinction
3. Optionally add `coldwine import --from-briefs` convenience command
