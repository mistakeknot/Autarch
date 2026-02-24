# Data Integrity Review: Acceptance Criteria Plan

**Reviewer:** Data Integrity Guardian
**Date:** 2026-02-06
**Plan reviewed:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Verdict:** The plan's "Data Integrity Risks" section (lines 243-252) identifies the right categories but materially underestimates the severity and incompleteness of several issues. Six additional risks are missing entirely. Fourteen acceptance criteria need additions or amendments.

---

## 1. Assessment of Existing "Data Integrity Risks" Section

The plan identifies four risks at lines 243-252. I evaluate each against the actual codebase.

### Risk 1: "SaveRevision has non-atomic writes" -- PARTIALLY RESOLVED, PLAN IS STALE

**Plan claim:** "Two files written sequentially with no write-to-temp-then-rename. The function mutates spec.Version as a side effect even if file writes fail. Concurrent phase completions can produce duplicate version numbers."

**Actual code state** (from `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`):

- **Input mutation: FIXED.** The current code creates `snapshot := *spec` at line 79 and sets `snapshot.Version = version` at line 80. The input `*Spec` is never mutated. The test at `evolution_test.go:11-49` explicitly verifies this: `TestSaveRevisionUsesFilesystemVersionAndDoesNotMutateInput` asserts `spec.Version` remains 0 after two saves.

- **Two-file write: MITIGATED but not eliminated.** Both files now use `fileutil.AtomicWriteFile` (lines 88, 98), which performs write-to-temp-then-fsync-then-rename for each individual file. If the process crashes between the snapshot rename (line 88) and the revision metadata rename (line 98), an orphaned snapshot still exists. However, there is now a best-effort rollback at line 100: `os.Remove(snapPath)` if the revision write fails. This covers application-level failures but NOT mid-operation crashes (kill -9, power loss).

- **Concurrent version numbers: FIXED via file locking.** Lines 52-62 acquire a filesystem lock via `fileutil.LockFile` using `syscall.Flock(LOCK_EX)` (see `/root/projects/Autarch/internal/file/lock_unix.go`). The `nextHistoryVersion` function (line 107) scans the directory under this lock to determine the next version. The test at `evolution_test.go:69-133` verifies 8 concurrent workers produce 8 unique sequential versions. This is correct.

**Assessment:** The plan's description of SaveRevision is outdated. Two of three problems are already fixed. The remaining two-file atomicity gap (crash between first and second rename) is a genuine risk but lower severity than stated. The plan should update its description and acknowledge the existing mitigations while noting the residual crash-window risk.

**Missing acceptance criterion:** There is no AC that tests the crash-recovery scenario. A test should verify that if only the snapshot file exists (no `_rev.yaml`), `LoadHistory` either (a) detects and reports the orphan, or (b) ignores it cleanly without returning corrupted data. Currently `LoadHistory` (line 153) only scans for `_rev.yaml` suffixes, so orphaned snapshots are invisible. This is safe but wasteful -- the orphan version number is consumed forever.

### Risk 2: "Feedback rolling window lacks crash recovery" -- ACCURATE, UNDERSPECIFIED

**Plan claim:** "Archive-then-truncate is not atomic. Process crash between archive write and truncation causes duplicate entries. Concurrent triage sessions can interleave YAML writes."

**Assessment:** This risk is accurately identified but the plan provides no acceptance criteria that test the crash-recovery path. AC-3.9 only tests "add 60 decisions, verify window size and archive presence" -- a happy-path test.

**Missing acceptance criteria:**

- AC-3.9a: Simulate crash after archive write but before truncation (e.g., inject fault). On restart, verify no duplicate decisions exist and no decisions are lost.
- AC-3.9b: Two concurrent triage sessions writing to `feedback.yaml` simultaneously must not produce corrupt YAML. Verify with `go test -race` and 10+ concurrent writers.
- AC-3.9c: If `feedback.yaml` is corrupt (e.g., truncated mid-write), the agent should start with empty preferences and log a warning without overwriting the corrupt file (preserving it for forensic recovery).

**Code note:** The feedback YAML code does not appear to exist yet in the codebase (no `.pollard/feedback.yaml` handler found). This means the risk is entirely prospective -- there is no code to review, only a specification. The acceptance criteria must be written defensively to prevent the described failure modes from ever being introduced.

### Risk 3: "Agent Teams <-> Coldwine task sync has no reconciliation" -- ACCURATE, CRITICAL

**Plan claim:** "If a teammate marks done in Agent Teams while Coldwine is unreachable, states diverge. No conflict detection or resolution mechanism defined."

**Assessment:** This is the most dangerous data integrity risk in the entire plan. The Coldwine storage layer (`/root/projects/Autarch/internal/coldwine/storage/`) has comprehensive SQLite tables for tasks, work_tasks, epics, stories, and agent_sessions -- but no reconciliation logic. The task status (`UpdateWorkTaskStatus` in `work_task.go:127`) does a bare UPDATE with no optimistic concurrency control (no version column, no updated_at check). Two writers can set conflicting statuses without either detecting the conflict.

**Missing acceptance criteria:**

- AC-4.11: Define and test reconciliation behavior when Agent Teams reports a task as "done" but Coldwine has it as "in_progress". Expected: Coldwine detects divergence within one poll cycle, surfaces it as a warning in the TUI, and does NOT silently overwrite either state.
- AC-4.12: Define and test reconciliation behavior when Coldwine is restarted after being offline while Agent Teams made progress. Expected: on startup, Coldwine performs a full sync, comparing its state.db against Agent Teams config, and generates a reconciliation report listing all divergent states.
- AC-4.13: `work_tasks` table should have a `version` or `updated_at` column used for optimistic locking. `UpdateWorkTaskStatus` should fail if the row's `updated_at` has changed since the caller last read it.

### Risk 4: "SQLite single-connection bottleneck under Agent Teams" -- ACCURATE, NUANCED

**Plan claim:** "MaxOpenConns(1) serializes all reads and writes. With 3+ teammates writing to .coldwine/state.db, SQLITE_BUSY errors become likely despite 5s timeout."

**Actual code** (from `/root/projects/Autarch/pkg/db/open.go`):

```go
db.SetMaxOpenConns(1)
// ...
"PRAGMA busy_timeout=5000",
```

**Assessment:** The plan correctly identifies the constraint but overstates the risk for the intended deployment model. Key observations:

1. **MaxOpenConns(1) is the recommended pattern for SQLite writers.** SQLite supports only one writer at a time. `MaxOpenConns(1)` serializes at the Go level, avoiding the overhead of SQLite-level locking. The `busy_timeout=5000` serves as a safety net if multiple processes access the same database file.

2. **WAL mode helps.** With WAL mode, readers do not block writers and writers do not block readers. Only writer-writer contention is serialized. Since most agent operations are reads (status checks, listing tasks), the single writer connection is unlikely to be a bottleneck for 3 teammates.

3. **The real risk is multi-process, not multi-goroutine.** If Agent Teams spawns each teammate as a separate OS process, each process opens its own Go `sql.DB` with `MaxOpenConns(1)`. Now there are 3+ separate writers hitting the same SQLite file. The busy_timeout of 5 seconds should handle this, but under sustained load (e.g., all three teammates completing tasks simultaneously), `SQLITE_BUSY` errors become possible.

4. **Coldwine's `OpenShared` pattern** (`/root/projects/Autarch/internal/coldwine/storage/db.go:36-48`) caches DB handles per path, preventing multiple connections from the same process. This is correct for the single-process case.

**Missing acceptance criteria:**

- AC-X.11: Under simulated Agent Teams load (3 concurrent processes writing to the same `state.db`), verify zero `SQLITE_BUSY` errors over 100 write operations with 5-second busy_timeout. If this test fails, increase busy_timeout or implement retry logic.
- AC-X.12: Add a test measuring p99 write latency under 3-concurrent-writer load. The plan's timing threshold table lists "SQLite write p99 <100ms under 3 concurrent agent sessions" but no AC references this target.

---

## 2. Missing Data Integrity Risks Not Covered by the Plan

### Missing Risk A: SaveSprintState Uses Non-Atomic os.WriteFile

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/persistence.go:14-38`

```go
func SaveSprintState(state *SprintState) error {
    // ...
    path := filepath.Join(dir, state.ID+".yaml")
    if err := os.WriteFile(path, data, 0644); err != nil {
        return fmt.Errorf("write state: %w", err)
    }
    return nil
}
```

Unlike `SaveRevision` which uses `fileutil.AtomicWriteFile` (write-to-temp, fsync, rename), `SaveSprintState` uses bare `os.WriteFile`. This means:

1. **Partial writes on crash.** If the process crashes mid-write, the YAML file is truncated or corrupted. On next startup, `LoadSprintState` will fail with a YAML parse error, and the sprint is unrecoverable.

2. **Concurrent writes corrupt data.** The orchestrator calls `saveLocked()` from multiple code paths (SetState, Revert, Advance, ProcessChatMessage, etc.). While the orchestrator's `mu` lock prevents concurrent access within a single process, if two processes ever access the same sprint file (unlikely today but possible under Agent Teams), the file will be corrupted.

3. **High frequency of writes.** `saveLocked()` is called on every state mutation, including phase transitions, draft revisions, confidence updates, and research result arrivals. A crash during any of these frequent writes destroys the entire sprint history.

**Severity:** HIGH. Sprint state loss means the user must restart a 25-45 minute workflow.

**Recommendation:** Add AC-1.15a: "Sprint state persistence uses atomic write (write-to-temp-then-rename). Verify: kill -9 during SaveSprintState results in either the old state or the new state on restart, never a corrupt file." Alternatively, `SaveSprintState` should use the existing `fileutil.AtomicWriteFile` helper.

### Missing Risk B: Signal Store DSN Pragma Inconsistency

**File:** `/root/projects/Autarch/internal/gurgeh/signals/store.go:49`

```go
db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
```

**File:** `/root/projects/Autarch/pkg/db/open.go:25-26` (comment)

```go
// Execute pragmas directly -- modernc.org/sqlite does not support DSN params.
```

The signals store uses DSN params (`?_pragma=...`) to set WAL mode and busy_timeout, but the shared `pkg/db/open.go` explicitly states that the modernc.org/sqlite driver does not support DSN params. This means:

- If the comment is accurate, the signals store's WAL mode and busy_timeout are silently NOT set. The database runs in default journal mode (DELETE) with no busy timeout.
- If the DSN params actually work (the comment is wrong), there is an inconsistency in approach that should be unified.

**Severity:** MEDIUM. Without WAL mode, the signals database has worse concurrent read/write performance. Without busy_timeout, any contention results in immediate `SQLITE_BUSY` errors instead of retrying.

**Recommendation:** Add AC-3.4e: "Signal store database runs in WAL mode with busy_timeout=5000. Verify by querying `PRAGMA journal_mode` and `PRAGMA busy_timeout` after store initialization."

### Missing Risk C: Gurgeh Signal Store Lacks foreign_keys PRAGMA

**File:** `/root/projects/Autarch/internal/gurgeh/signals/store.go`

The signals store opens its own database connection directly via `sql.Open("sqlite", ...)` without going through `pkg/db/Open()` or Coldwine's `storage.Open()`. It does NOT set `PRAGMA foreign_keys = ON`. While the signals table currently has no foreign keys, this divergence means:

- If foreign keys are added to the signals schema in the future, they will not be enforced.
- There is no `MaxOpenConns` limit, meaning the Go connection pool could open multiple writers, defeating SQLite's single-writer model.

**Severity:** LOW (currently). But a latent correctness hole.

**Recommendation:** Signals store should use `pkg/db/Open()` for consistency. Add this to the existing "framework unification" recommendations.

### Missing Risk D: ReservePaths Partial-Grant Behavior

**File:** `/root/projects/Autarch/internal/coldwine/storage/coordination.go:239-306`

The `ReservePaths` function processes paths in a loop within a single transaction. If path A is granted but path B conflicts, the function does NOT rollback path A's grant. It continues, accumulating both `Granted` and `Conflicts` in the result. The transaction is then committed with the partial grants.

This creates a data integrity issue:

1. A task requires reservations on BOTH `internal/auth/**/*.go` AND `internal/middleware/**/*.go`.
2. Path A is granted, path B conflicts.
3. The function returns with one grant and one conflict.
4. The caller must decide whether to release path A or proceed with partial coverage.
5. If the caller crashes before releasing path A, the reservation is leaked until TTL expiry.

**The current code has no rollback-on-partial-conflict logic.** The caller gets a mixed result and must handle cleanup.

**Severity:** MEDIUM. Leaked reservations block other agents until TTL expiry.

**Missing acceptance criteria:**

- AC-4.2d: When a multi-path reservation request has partial conflicts, verify that EITHER all paths are granted atomically OR no paths are granted (all-or-nothing semantics). The current partial-grant behavior should be an explicit design decision documented in the plan.
- AC-4.4b: Verify that reservation cleanup occurs even if the requesting process crashes between receiving a partial grant and deciding to release. (Currently relies on TTL expiry as the only safety net.)

### Missing Risk E: Coldwine Task Status Updates Have No Optimistic Concurrency Control

**File:** `/root/projects/Autarch/internal/coldwine/storage/work_task.go:127-131`

```go
func UpdateWorkTaskStatus(db *sql.DB, id string, status TaskStatus) error {
    now := time.Now().Format(time.RFC3339)
    _, err := db.Exec(`UPDATE work_tasks SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
    return err
}
```

This function performs a blind UPDATE with no WHERE clause on the current status or updated_at. Two concurrent callers can:

1. Both read task as "in_progress"
2. Caller A sets status to "done"
3. Caller B sets status to "blocked" (overwriting "done")

The final state is "blocked" even though the task was completed. No error is raised. No audit trail shows the lost "done" transition.

This is particularly dangerous in the Agent Teams context where the lead and a teammate might simultaneously update the same task's status.

**Severity:** HIGH under Agent Teams. A completed task reverting to "blocked" could cause the team to re-do work.

**Missing acceptance criteria:**

- AC-2.7a: Task state transitions enforce valid state machine transitions (e.g., "done" -> "blocked" is invalid). The UPDATE should include a WHERE clause validating the current status.
- AC-2.7b: Concurrent status updates to the same task must not result in a lost update. Implement optimistic locking: `UPDATE ... SET status = ?, updated_at = ? WHERE id = ? AND updated_at = ?` with retry on zero rows affected.

### Missing Risk F: Broker Signal Drop Without Notification

**File:** `/root/projects/Autarch/pkg/signals/broker.go:51-54`

```go
select {
case sub.ch <- sig:
default:
    // Drop if subscriber is slow.
}
```

The broker silently drops signals when a subscriber's 64-element buffer is full. This means:

1. During a burst of research findings, Bigend's dashboard subscription could miss signals.
2. There is no counter, log, or metric for dropped signals.
3. The plan's AC-3.4a (signal deduplication) could be undermined: if a signal is dropped and then re-emitted, the subscriber sees it for the "first time" even though it was previously sent.

**Severity:** MEDIUM. Silent data loss in an observation system.

**Missing acceptance criteria:**

- AC-3.4f: Signal broker tracks a dropped-signal counter per subscriber. When drops exceed a threshold (e.g., 10), emit a `SignalBackpressure` meta-signal to the subscriber indicating missed data.
- AC-3.4g: Alternatively, switch to a blocking send with timeout, or use a ring buffer that overwrites oldest signals and includes a "missed N signals" indicator.

---

## 3. SQLite Concurrent Access Under Agent Teams Load

### Architecture Assessment

The Coldwine storage layer uses three different database access patterns:

| Component | DB Path | Open Method | foreign_keys | MaxOpenConns |
|-----------|---------|-------------|-------------|--------------|
| Coldwine tasks | `.coldwine/state.db` | `storage.Open()` -> `pkg/db.Open()` + FK pragma | ON | 1 |
| Coldwine shared | `.coldwine/state.db` | `storage.OpenShared()` (cached) | ON | 1 |
| Gurgeh signals | `.gurgeh/signals/signals.db` | Direct `sql.Open()` | OFF (not set) | default (unlimited) |
| Events spine | `~/.autarch/events.db` | `pkg/db.Open()` (presumed) | OFF (not set) | 1 |

### Concurrent Access Scenarios

**Scenario 1: Single Process, Multiple Goroutines (Current Architecture)**

With `MaxOpenConns(1)`, all goroutines within the same process serialize on the same connection. This is safe. WAL mode means read-only queries (list tasks, check status) execute concurrently via separate read connections... except `MaxOpenConns(1)` prevents this. ALL operations, reads and writes, are serialized through a single connection.

**Impact:** Under Agent Teams with a background poller + TUI refresh + state saves, the single connection becomes a bottleneck. Reads block behind writes. The orchestrator's `saveLocked()` (called frequently) will block TUI reads.

**Recommendation for AC plan:** Add AC-X.13: "Separate read and write connection pools for Coldwine state.db. Write pool: MaxOpenConns(1). Read pool: MaxOpenConns(4) with `PRAGMA query_only=ON`. Verify TUI refresh latency is unaffected by concurrent writes."

**Scenario 2: Multiple Processes (Agent Teams Spawned Teammates)**

Each teammate process opens its own `sql.DB`. With WAL mode and `busy_timeout=5000`, this should work for low-to-moderate write rates. However:

- The Coldwine `OpenShared()` cache is process-local. It provides no benefit across processes.
- Each process sets `PRAGMA foreign_keys = ON` independently. This is correct (per-connection pragma).
- Each process sets `PRAGMA journal_mode=WAL` independently. The second process's WAL pragma is a no-op (WAL mode is a database property, not connection property), which is fine.

**Risk:** If teammates write at high frequency (e.g., rapid task status updates during a sprint), the 5-second busy_timeout may be exceeded. The plan's timing table says "SQLite write p99 <100ms under 3 concurrent agent sessions" but this is untested.

### Recommendation

The SQLite configuration is fundamentally sound for the single-developer, local-first model. The risks are:

1. **Read starvation under write load** (single connection pool) -- solvable with read/write connection separation.
2. **Multi-process busy contention** (Agent Teams) -- solvable with increased busy_timeout (15s) and retry logic with exponential backoff.
3. **No connection init hooks** -- if the connection pool ever grows, `PRAGMA foreign_keys = ON` must be set on every new connection, not just the first. This is noted in the deep-dive research but has no AC.

---

## 4. YAML Write Pattern Safety

### Concurrent Triage (feedback.yaml)

The feedback YAML system does not exist in code yet. The plan describes it as a rolling window of 50 decisions. Key risks for the implementation:

**Risk 1: YAML is not append-safe.** Unlike line-oriented formats (JSONL, CSV), YAML requires rewriting the entire file to add or remove entries. Two concurrent writers both reading the file, appending a decision, and writing back will result in one writer's decision being lost (last-writer-wins) or the file being corrupted (interleaved writes).

**Mitigation required:** Use file locking (the existing `fileutil.LockFile` pattern) around all feedback.yaml read-modify-write cycles. Or switch to JSONL (append-only, one JSON object per line) which avoids the read-modify-write pattern entirely.

**Risk 2: Rolling window archive is a multi-step operation.** The plan describes "archive-then-truncate" but does not specify atomicity. The correct sequence is:

1. Acquire lock on feedback.yaml
2. Read feedback.yaml (50+ entries)
3. Write entries 1-N to feedback-archive/YYYY-MM-DD.yaml (atomic write)
4. Write entries N+1 through 50 back to feedback.yaml (atomic write)
5. Release lock

If step 3 succeeds but step 4 crashes, the archived entries are duplicated on restart. If step 4 succeeds but step 3 crashes, the archived entries are lost.

**Recommendation:** Make the archive operation idempotent. Include a sequence number or hash in the archive filename. On startup, check for incomplete archives and complete the operation. Add AC-3.9d: "Rolling window archive is idempotent: re-running the archive operation after a crash produces the same result as if the first operation completed."

### Sprint State YAML (SaveSprintState)

As detailed in Missing Risk A, `SaveSprintState` uses non-atomic `os.WriteFile`. This is the most frequently written YAML file in the system (called on every state mutation during an active sprint). The fix is straightforward: replace `os.WriteFile` with `fileutil.AtomicWriteFile`.

### Spec History YAML (SaveRevision)

Already uses `fileutil.AtomicWriteFile` for individual files. The two-file pair atomicity gap is documented above. Severity is low because the lock prevents concurrent version conflicts, and the rollback logic handles application-level failures.

---

## 5. Summary of Recommended AC Additions

### High Priority (Data Loss or Corruption Risk)

| New AC ID | Description | Addresses |
|-----------|-------------|-----------|
| AC-1.15a | Sprint state persistence uses atomic writes; verify crash resilience | Missing Risk A |
| AC-2.7a | Task state transitions enforce valid state machine (reject invalid transitions) | Missing Risk E |
| AC-2.7b | Concurrent task status updates use optimistic locking | Missing Risk E |
| AC-3.9a | Feedback rolling window crash recovery: no duplicates, no data loss | Risk 2 underspecification |
| AC-3.9b | Concurrent feedback writes produce valid YAML (test with -race) | Risk 2 underspecification |
| AC-4.2d | Multi-path reservations are all-or-nothing (no partial grants) | Missing Risk D |
| AC-4.11 | Agent Teams/Coldwine state divergence detection and warning | Risk 3 underspecification |
| AC-4.12 | Coldwine startup reconciliation with Agent Teams | Risk 3 underspecification |

### Medium Priority (Correctness or Consistency)

| New AC ID | Description | Addresses |
|-----------|-------------|-----------|
| AC-3.4e | Signal store runs in WAL mode with busy_timeout (verify pragmas) | Missing Risk B |
| AC-3.4f | Signal broker tracks and reports dropped signals | Missing Risk F |
| AC-3.9c | Corrupt feedback.yaml triggers graceful degradation, not crash | Risk 2 underspecification |
| AC-3.9d | Rolling window archive is idempotent across crashes | Risk 2 underspecification |
| AC-X.11 | Zero SQLITE_BUSY errors under 3-process concurrent write load | Risk 4 underspecification |
| AC-X.12 | SQLite write p99 <100ms under 3-concurrent-writer benchmark | Risk 4 underspecification |

### Low Priority (Defense in Depth)

| New AC ID | Description | Addresses |
|-----------|-------------|-----------|
| AC-4.13 | work_tasks version column for optimistic concurrency control | Risk 3 |
| AC-X.13 | Separate read/write connection pools for Coldwine state.db | SQLite assessment |

---

## 6. Plan Corrections Needed

1. **Lines 243-244 (SaveRevision description):** Update to reflect that input mutation is fixed, filesystem locking prevents duplicate versions, and both files use AtomicWriteFile. The residual risk is the two-file crash window, which is lower severity.

2. **Lines 245-246 (Feedback rolling window):** Add specification for locking strategy and idempotent archive operations.

3. **Line 249 (Agent Teams sync):** Elevate from a description to a concrete gap with required reconciliation mechanism and acceptance criteria.

4. **Lines 250-252 (SQLite bottleneck):** Clarify that `MaxOpenConns(1)` is correct for writers, but readers should have a separate pool. The multi-process case (Agent Teams) needs specific busy_timeout tuning and retry logic.

5. **Missing entirely:** SaveSprintState non-atomic writes, signal store pragma inconsistency, ReservePaths partial-grant semantics, task status lost-update vulnerability, broker signal drops.

---

## 7. Code Files Referenced

| File | Relevance |
|------|-----------|
| `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` | SaveRevision atomicity, file locking |
| `/root/projects/Autarch/internal/gurgeh/specs/evolution_test.go` | Concurrency and mutation tests |
| `/root/projects/Autarch/internal/gurgeh/arbiter/persistence.go` | Non-atomic SaveSprintState |
| `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` | saveLocked() call sites, concurrent state access |
| `/root/projects/Autarch/internal/gurgeh/signals/store.go` | DSN pragma inconsistency, missing foreign_keys |
| `/root/projects/Autarch/pkg/signals/broker.go` | Silent signal drops |
| `/root/projects/Autarch/pkg/db/open.go` | MaxOpenConns(1), WAL mode, busy_timeout |
| `/root/projects/Autarch/internal/coldwine/storage/db.go` | foreign_keys pragma, OpenShared caching |
| `/root/projects/Autarch/internal/coldwine/storage/schema.go` | MigrateV2 schema, CASCADE definitions |
| `/root/projects/Autarch/internal/coldwine/storage/coordination.go` | ReservePaths partial-grant behavior |
| `/root/projects/Autarch/internal/coldwine/storage/work_task.go` | UpdateWorkTaskStatus blind update |
| `/root/projects/Autarch/internal/coldwine/storage/task.go` | Basic task CRUD, no concurrency control |
| `/root/projects/Autarch/internal/file/atomic.go` | AtomicWriteFile implementation |
| `/root/projects/Autarch/internal/file/lock_unix.go` | flock-based file locking |
