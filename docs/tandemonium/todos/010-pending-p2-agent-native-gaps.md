---
status: pending
priority: p2
issue_id: "010"
tags: [agent-native, cli, code-review]
dependencies: []
---

# Agent-Native CLI Gaps - TUI-Only Features

## Problem Statement

12 of 18 TUI capabilities have no CLI equivalent, preventing AI agents from using the review workflow. AGENTS.md documents commands that don't exist.

## Findings

**Missing CLI Commands (TUI-only features):**

| TUI Feature | TUI Location | CLI Status |
|-------------|--------------|------------|
| View review queue | `model.go` (R key) | MISSING |
| Reject with feedback | `model.go` (r key) | MISSING |
| Add feedback | `model.go` (f key) | MISSING |
| Edit story | `model.go` (e key) | MISSING |
| View diff | `review_diff.go` (d key) | MISSING |
| List tasks | AGENTS.md documented | MISSING |
| Add task | AGENTS.md documented | MISSING |
| Start task | AGENTS.md documented | MISSING |
| Complete task | AGENTS.md documented | MISSING |
| Show task | AGENTS.md documented | MISSING |
| JSON output | AGENTS.md documented | MISSING |

**Documentation Drift:**
AGENTS.md (lines 54-91) documents `list`, `add`, `start`, `complete`, `show`, `update`, `branch`, `--json` that don't exist in the Go CLI.

## Proposed Solutions

### Option 1: Implement Core CLI Commands (Recommended)
- **Pros:** Enables agent automation
- **Cons:** Significant work
- **Effort:** Large
- **Risk:** Low

Implement in priority order:
1. `tandemonium review list [--json]`
2. `tandemonium review reject <id> --feedback "msg"`
3. `tandemonium review feedback <id> "msg"`
4. `tandemonium show <id> [--json]`
5. `tandemonium diff <id> [--file <path>]`

### Option 2: Update Documentation to Match Reality
- **Pros:** Reduces confusion
- **Cons:** Agents still can't automate
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Implement Option 1 incrementally. Start with `review list --json` which unblocks basic agent workflows.

## Technical Details

- **Affected files:**
  - New: `internal/cli/commands/review.go`
  - New: `internal/cli/commands/show.go`
  - Update: `AGENTS.md`
- **Components:** CLI
- **Database changes:** None

## Acceptance Criteria

- [ ] `tandemonium review list` command implemented
- [ ] `--json` flag added to all list commands
- [ ] `tandemonium review reject` command implemented
- [ ] AGENTS.md updated to reflect actual CLI
- [ ] Agent can complete basic review workflow via CLI

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during agent-native review | 12/18 features TUI-only |

## Resources

- N/A
