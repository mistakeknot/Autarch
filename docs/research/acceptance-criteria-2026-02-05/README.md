# Acceptance Criteria Plan: Research Deep-Dive Results

**Date:** 2026-02-05
**Plan:** [docs/plans/2026-02-05-acceptance-criteria-plan.md](../../plans/2026-02-05-acceptance-criteria-plan.md)
**Status:** Collected, pending synthesis into plan

## Overview

15 specialized research agents analyzed the acceptance criteria plan from different angles. Their findings are saved here for reference and synthesis.

## Reports

### Deep-Dive analyses (Wave 1)

| File | Focus | Key Findings |
|------|-------|-------------|
| [deep-dive-mcp-tool-surface.md](deep-dive-mcp-tool-surface.md) | MCP server tool catalog | 38-tool design for Autarch MCP API |
| [deep-dive-security-remediation.md](deep-dive-security-remediation.md) | Security findings F1-F6 | WebSocket origin wildcard, glob overlap, YAML poisoning |
| [deep-dive-data-integrity-fixes.md](deep-dive-data-integrity-fixes.md) | Data integrity risks | SaveRevision atomicity, SQLite bottleneck, signal dedup races |

### Institutional learnings (Wave 2)

| File | Focus | Key Findings |
|------|-------|-------------|
| [learnings-arbiter-architecture.md](learnings-arbiter-architecture.md) | Adapter patterns, import cycles | AC-1.14a needed for GoalFeature auto-reeval |
| [learnings-phase-reordering.md](learnings-phase-reordering.md) | Phase ordering dependencies | AC-1.12 overclaims 8 phases, only 4 exist |
| [learnings-over-planning-pattern.md](learnings-over-planning-pattern.md) | Reproduce-before-plan anti-pattern | Plan itself may be over-planning before reproduction |
| [learnings-tui-layout-patterns.md](learnings-tui-layout-patterns.md) | Dimension propagation bugs | Live bug in PollardView; proposed 14 new ACs |
| [learnings-ansi-overlay-rendering.md](learnings-ansi-overlay-rendering.md) | ANSI-aware string ops | Live bug in wordWrap using byte length for emoji |

### Specialized reviews (Wave 2)

| File | Focus | Key Findings |
|------|-------|-------------|
| [tui-race-conditions-review.md](tui-race-conditions-review.md) | Bubble Tea race scenarios | Concurrent state access, ticker cleanup, cmd ordering |
| [deployment-verification-review.md](deployment-verification-review.md) | Migration/rollback checklists | Pre/post-deploy SQL verification queries |
| [agent-native-architecture-skill.md](agent-native-architecture-skill.md) | Agent parity audit | 15/17 TUI actions lack agent equivalents |
| [orchestrating-swarms-skill.md](orchestrating-swarms-skill.md) | Agent Teams integration | Structural gaps in swarm orchestration patterns |
| [repo-patterns-analysis.md](repo-patterns-analysis.md) | Codebase conventions | 1077 tests, stdlib-only testing, no testify |
| [git-history-evolution-analysis.md](git-history-evolution-analysis.md) | Code evolution patterns | 399 commits in 16 days, burst development |
| [framework-docs-research.md](framework-docs-research.md) | BT, SQLite, WS, YAML, Agent Teams | Per-framework best practices with code patterns |

## Summary statistics

- **Total research output:** ~380K characters across 15 reports
- **Live bugs found:** 2 (PollardView dimension mismatch, wordWrap byte-length for emoji)
- **New ACs proposed:** ~40+ across all agents
- **Security findings:** 6 (F1-F6), prioritized P0-P3
- **Agent-native gaps:** 15/17 TUI actions missing agent equivalents

## Next step

Synthesize these findings into the acceptance criteria plan: update Enhancement Summary, add new ACs, correct overclaimed ACs, and integrate framework-specific patterns.
