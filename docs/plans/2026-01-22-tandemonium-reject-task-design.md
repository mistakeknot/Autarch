# Tandemonium RejectTask Ready-State Design

## Goal
Ensure RejectTask only transitions tasks back to "ready" (no redundant "rejected" update), while keeping the review queue cleanup and transaction semantics unchanged.

## Context
`RejectTask` currently updates status to "rejected" and then immediately to "ready" in the same transaction, making the "rejected" state unobservable. Tests already assert the end state is "ready" and that rollback leaves the status as "review" if the transaction fails.

## Decision
Keep the business behavior "reject -> ready" and remove the redundant "rejected" update. This aligns with existing tests and avoids an unnecessary write.

## Approach
- Add a regression test that installs a temporary SQLite trigger on the `tasks` table to raise an error if any update sets `status = 'rejected'`. This makes the current bug observable (test fails now) and will pass once the redundant update is removed.
- Simplify `RejectTask` to perform only a single `UPDATE` to "ready" and then remove the task from `review_queue`, all in the existing transaction.
- Preserve existing rollback behavior by keeping operations within the same transaction and relying on the current rollback test.

## Testing
- New test in `internal/tandemonium/storage/review_test.go` that fails if `RejectTask` attempts to write "rejected".
- Existing tests:
  - `TestRejectTaskRequeues` should continue to pass (status becomes "ready", review queue cleared).
  - `TestRejectTaskTransactionRollsBack` should continue to pass (status remains "review" after a forced failure).

## Risks
Low. Behavior remains "ready" on rejection; only the redundant update is removed. The trigger-based test provides strong regression coverage without requiring new production code paths.
