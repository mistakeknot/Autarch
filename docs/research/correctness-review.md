# Correctness Review: Thematic Work Lanes (v13 Schema)

**Reviewer:** Julik (Flux-drive Correctness Reviewer)
**Date:** 2026-02-21
**Project:** infra/intercore (lanes feature, diff: `/tmp/qg-diff-1771665590.txt`)
**Files reviewed:**
- `/root/projects/Interverse/infra/intercore/internal/db/schema.sql`
- `/root/projects/Interverse/infra/intercore/internal/db/db.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/store.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/velocity.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/store_test.go`
- `/root/projects/Interverse/infra/intercore/internal/lane/velocity_test.go`
- `test-integration.sh` (lanes section)

---

## Invariants Established

Before scoring, these are the invariants that must hold for correctness:

1. A lane row and its "created" event are always both present or both absent (atomicity guaranteed by transaction).
2. A closed lane has `status = 'closed'` AND a non-NULL `closed_at` AND a "closed" lane_event — all three written atomically.
3. `lane_members(lane_id, bead_id)` is the exclusive source of truth for membership; no partial snapshot ever leaves the table in an inconsistent intermediate state visible to concurrent readers.
4. The schema version in `PRAGMA user_version` matches the DDL that was committed.
5. Upgrading from any prior version (v1–v12) leaves the database in valid state: no lost rows, no violated NOT NULL constraints, no orphan FK references.
6. Velocity starvation scores are bounded and free of divide-by-zero under all inputs.

---

## Overall Assessment

The implementation is largely solid. The v13 schema addition is the cleanest possible migration: three new tables using `CREATE TABLE IF NOT EXISTS` with no `ALTER TABLE` on existing tables, making the upgrade non-destructive for any database at v1–v12. The two critical write paths (Create, Close) are correctly wrapped in transactions with `defer tx.Rollback()`. Velocity math avoids divide-by-zero via `math.Max(throughput, 0.1)`.

Four issues are worth addressing before production use. None are catastrophic data-loss scenarios, but two of them (C-03, C-02) can silently produce wrong behavior under context cancellation and on perpetually-upgraded databases respectively.

---

## Issue C-01: LOW — Early-return inside Migrate skips explicit cleanup, relies silently on deferred rollback

**File:** `/root/projects/Interverse/infra/intercore/internal/db/db.go` lines 132–134

```go
if currentVersion >= currentSchemaVersion {
    return nil // already migrated
}
```

At this point the transaction `tx` has already executed a write (`CREATE TABLE IF NOT EXISTS _migrate_lock`). The `defer tx.Rollback()` at line 117 fires on this return path and rolls back that write. In WAL mode with `SetMaxOpenConns(1)` this is benign — the lock is released correctly. However:

- A future author adding work between lines 122 and 132 that should persist even when version is current will have that work silently discarded.
- The implicit reliance on deferred rollback for an intentional early-exit path obscures intent. Any reader has to reason through the defer chain to confirm the lock is released.

**Fix:** Call `tx.Rollback()` explicitly before the early `return nil`:

```go
if currentVersion >= currentSchemaVersion {
    tx.Rollback() // intentional no-op: nothing to commit
    return nil
}
```

---

## Issue C-02: LOW — v6 migration block has no upper-bound guard; fires on already-migrated databases

**File:** `/root/projects/Interverse/infra/intercore/internal/db/db.go` line 137

```go
if currentVersion >= 5 {   // <-- no upper bound
```

All other migration blocks use a bounded guard: `currentVersion >= X && currentVersion < Y`. The v6 block is the only one that fires on *any* database at or above v5, including a v13 database calling Migrate a second time. When it fires on a v13 database, it attempts `ALTER TABLE runs ADD COLUMN phases TEXT` which fails with "duplicate column name" — caught by `isDuplicateColumnError`. The behavior is safe because of that guard, but it means every call to Migrate on a current-version database burns through 6 redundant ALTER TABLE error paths. More importantly: if `isDuplicateColumnError` ever produces a false negative (e.g., after a SQLite driver version change that changes the error message text), the Migrate call fails with a spurious error on an otherwise healthy database.

**Fix:** Add the missing upper bound:

```go
if currentVersion >= 5 && currentVersion < 6 {
```

---

## Issue C-03: LOW — `rows.Err()` not checked after inner scan loop in SnapshotMembers

**File:** `/root/projects/Interverse/infra/intercore/internal/lane/store.go` lines 237–249

```go
rows, err := tx.QueryContext(ctx, `SELECT bead_id FROM lane_members WHERE lane_id = ?`, laneID)
if err != nil {
    return fmt.Errorf("snapshot members: query: %w", err)
}
for rows.Next() {
    var bid string
    if err := rows.Scan(&bid); err != nil {
        rows.Close()
        return fmt.Errorf("snapshot members: scan: %w", err)
    }
    existing[bid] = true
}
rows.Close()
// rows.Err() is never checked here
```

The Go `database/sql` contract requires checking `rows.Err()` after the iteration loop ends. If `rows.Next()` returns false due to a context cancellation mid-read, `rows.Err()` carries `context.Canceled` or `context.DeadlineExceeded`. The code silently treats a partial read of `existing` as complete and proceeds to diff it against `desired`.

**Concrete failure interleaving:**
1. Lane has members `[iv-a, iv-b, iv-c]` in DB.
2. `SnapshotMembers(ctx, laneID, ["iv-a", "iv-b", "iv-c"])` is called.
3. Context deadline fires after cursor reads only `iv-a`. `rows.Next()` returns false. `existing = {"iv-a": true}`.
4. Code builds `desired = {iv-a, iv-b, iv-c}`. Since `iv-b` is not in `existing`, it attempts `INSERT INTO lane_members ... iv-b`. This hits the PRIMARY KEY constraint because iv-b already exists.
5. The snapshot function returns an error: `"snapshot members: insert iv-b: UNIQUE constraint failed"` — which misattributes the cause to a logic error rather than a context cancellation.

In a system calling SnapshotMembers on context timeout (e.g., a cron job with a 5s deadline), this produces confusing error logs and may cause the caller to believe the DB is in a worse state than it is. The transaction rollback guarantees invariant 3 is preserved, but the error message is misleading.

**Fix:** Add `rows.Err()` check after `rows.Close()`:

```go
rows.Close()
if err := rows.Err(); err != nil {
    return fmt.Errorf("snapshot members: read existing: %w", err)
}
```

---

## Issue C-04: LOW — `Close()` returns tx.Commit() error bare, breaking the project error-chain convention

**File:** `/root/projects/Interverse/infra/intercore/internal/lane/store.go` line 189

```go
return tx.Commit()
```

Every other error path in this file wraps with `fmt.Errorf("lane ...: %w", err)`. The bare `return tx.Commit()` produces an unwrapped error, making it harder to locate the origin in log output. This is a convention violation, not a correctness failure.

**Fix:**

```go
if err := tx.Commit(); err != nil {
    return fmt.Errorf("lane close: commit: %w", err)
}
return nil
```

---

## Schema Migration Safety: v12→v13

The v13 tables (`lanes`, `lane_events`, `lane_members`) are added exclusively via `CREATE TABLE IF NOT EXISTS` in `schema.sql`, which is applied at the end of every migration run (`tx.ExecContext(ctx, schemaDDL)`). There is no v12→v13 `ALTER TABLE` block needed, and none exists — this is correct.

**Upgrade path correctness:**
- Fresh install: all three tables created by DDL. Version bumped to 13. Correct.
- v12 DB: ALTER TABLE blocks for v12 run (budget_enforce, max_agents) and dispatch (spawn_depth, parent_dispatch_id) run first (since `currentVersion < 12`), then DDL adds the three lane tables. Version bumped to 13. Correct.
- v3–v11 DB: all intermediate ALTER TABLE blocks run (guarded by version checks), then DDL adds all missing tables including lanes. Version bumped to 13. Correct.
- v13 DB calling Migrate again: `currentVersion >= currentSchemaVersion` returns early (with the implicit rollback — see C-01). The DDL `CREATE TABLE IF NOT EXISTS` is never re-applied. Correct, but C-02 means the v6 block still fires redundantly.

**Rollback safety:** There is no DDL rollback possible in SQLite (DDL is not transactional in terms of schema changes being rolled back cleanly — SQLite will commit schema changes up to a crash point and can leave the DB in partial state). However, the backup mechanism (`d.path.backup-<timestamp>`) created before migration provides a recovery path.

---

## Velocity Calculation Safety

**Division safety:** `math.Max(throughput, 0.1)` at lines 85 and 127 prevents divide-by-zero when throughput is 0. The epsilon of 0.1 means starvation for a lane with 0 throughput and N open beads is `N * priority_weight / 0.1 = N * priority_weight * 10`. This produces large scores for starved lanes, which is the desired behavior.

**Priority range assumption:** `float64(5 - bs.Priority)` at line 80 assumes `bs.Priority` is in `[0, 4]`. Values outside this range produce weights outside `[1, 5]`:
- Priority = -1: weight = 6 (overestimates urgency)
- Priority = 5: weight = 0 (bead contributes nothing to starvation score — treated as if already done)
- Priority = 6+: weight negative (bead reduces starvation score)

This is a caller-contract bug rather than a library bug. The `BeadStatus` struct documents `Priority int // 0-4` but there is no enforcement. A clamped calculation `math.Max(float64(5-bs.Priority), 1.0)` would make the score monotonically bounded.

**Zero lanes:** If no active lanes exist, `List(ctx, "active")` returns nil/empty. The loop body never executes. `scores` is an empty map. `SortedByStarvation` returns an empty slice. No panic or divide-by-zero. Correct.

**Zero members:** A lane with 0 members produces `priorityWeightedOpen = 0`, `throughput` from closures. Starvation = 0 / max(throughput, 0.1). This is 0 for any throughput, or 0 for no throughput. A lane with no members has zero starvation. Correct — there is nothing to be starved of.

---

## Transaction Correctness in SnapshotMembers

The read-then-write pattern in `SnapshotMembers` is correctly protected inside a single transaction:

```
tx.BeginTx → SELECT members (read inside tx) → INSERT/DELETE (writes inside tx) → tx.Commit
```

With SQLite in WAL mode and `SetMaxOpenConns(1)`, the single connection means no concurrent writer can interleave between the read and the writes within the same process. Cross-process concurrency is handled by SQLite's busy_timeout (5s default). This is sufficient for the expected usage pattern of a CLI tool.

However: the transaction does not verify the lane exists before reading members. If `laneID` is invalid, the member query returns 0 rows (no error), the loop iterates zero times, and `desired` contains the caller's beadIDs. The INSERT loop then fires for each bead, hitting the `lane_id REFERENCES lanes(id)` foreign key constraint (if PRAGMA foreign_keys = ON, which db.go sets). This returns a clear FK error. So correctness is preserved — but a pre-flight check would give a cleaner error message.

---

## Foreign Key Enforcement

The schema defines `REFERENCES lanes(id)` on both `lane_events.lane_id` and `lane_members.lane_id`. SQLite foreign keys are not enforced by default; they require `PRAGMA foreign_keys = ON`. The `db.go:Open` function sets this pragma explicitly on every connection (line 68). The test helper `setupTestStore` calls `db.Migrate` which calls `db.Open` under the hood — so tests also have FK enforcement. This is correct.

**Risk:** If any code path creates a `*sql.DB` directly (bypassing `db.Open`), FK enforcement will be absent and orphan lane_events/lane_members records can be inserted for non-existent lanes without error. A grep of the codebase shows only `db.Open` is used — no direct `sql.Open("sqlite", ...)` calls in non-test code — so this risk is theoretical but worth noting.

---

## Test Coverage Assessment

**Covered:**
- Create + Get (ID and Name)
- List with status filter
- Close (including double-close guard)
- Duplicate name constraint
- RecordEvent + Events query
- SnapshotMembers (add, replace, multi-lane)
- GetLanesForBead
- Velocity relative starvation
- Velocity throughput effect
- SortedByStarvation
- FromDB velocity path
- Schema migration test (TestMigrate_V12ToV13_LaneTables)

**Not covered:**
- SnapshotMembers with context cancellation mid-read (the C-03 failure mode)
- SnapshotMembers on a closed or non-existent lane
- RecordEvent with an invalid lane_id (FK enforcement test)
- Velocity with priority values outside [0,4]
- Concurrent SnapshotMembers calls (stress test)
- GetMembers ordering stability (ORDER BY added_at — verified by schema but not by test)

---

## Improvements

**I-01. SnapshotMembers: pre-validate lane exists inside transaction** — A `SELECT 1 FROM lanes WHERE id = ?` before the member read would return a clean "lane not found" error instead of an FK violation error after partial work.

**I-02. velocity.go: clamp priority to [0,4]** — Replace `float64(5 - bs.Priority)` with `math.Max(float64(5-bs.Priority), 1.0)` and add a doc comment: `// Priority must be in [0,4]; values outside this range are clamped to weight [1,5]`.

**I-03. schema.sql: add CHECK constraints on status and lane_type** — `CHECK (status IN ('active','closed','archived'))` and `CHECK (lane_type IN ('standing','arc'))` enforce domain invariants at the DB layer. This prevents a stale binary or a direct SQL insert from writing an unrecognized value that confuses routing logic in future versions.

**I-04. store_test.go: add context-cancellation test for SnapshotMembers** — A test that cancels the context after query but before commit verifies that the DB state is unchanged after the error. This is the regression guard for C-03 once it is fixed.

**I-05. store_test.go: add SnapshotMembers on closed lane test** — Clarifies and tests whether closed lanes can have their membership updated (currently allowed by schema; unclear if this is intended).
