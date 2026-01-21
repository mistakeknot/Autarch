---
status: pending
priority: p3
issue_id: "019"
tags: [data-integrity, cli, code-review]
dependencies: []
---

# Implement Actual Recovery Logic (Not Dry-Run)

## Problem Statement

The recovery command is a dry-run stub with no actual recovery logic. After a crash, users have no automated way to recover consistent state.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/cli/commands/recover.go` lines 29-38

```go
fmt.Fprintln(cmd.OutOrStdout(), "Recovery (dry-run):")
fmt.Fprintf(cmd.OutOrStdout(), "  Would rebuild from %d spec file(s)\n", len(specs))
// ...
fmt.Fprintln(cmd.OutOrStdout(), "  No changes applied.")
```

**Impact:**
- No crash recovery
- User must manually fix inconsistencies
- Orphaned worktrees persist

## Proposed Solutions

### Option 1: Implement Full Recovery (Recommended)
- **Pros:** Actual crash recovery
- **Cons:** Complex logic
- **Effort:** Medium
- **Risk:** Medium

Recovery should:
1. Scan spec files as source of truth
2. Reconcile DB state with spec files
3. Detect and repair orphaned worktrees/sessions
4. Generate recovery report

Add `--apply` flag to execute vs dry-run.

## Recommended Action

Implement recovery with --apply flag, keeping dry-run as default.

## Technical Details

- **Affected files:** `internal/cli/commands/recover.go`
- **Components:** CLI
- **Database changes:** May fix inconsistencies

## Acceptance Criteria

- [ ] Spec files scanned for truth
- [ ] DB reconciled with specs
- [ ] Orphaned resources detected
- [ ] --apply flag executes recovery
- [ ] Report generated

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during data integrity review | Recovery is critical for data safety |

## Resources

- N/A
