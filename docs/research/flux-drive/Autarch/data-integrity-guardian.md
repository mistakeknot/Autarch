---
agent: data-integrity-guardian
tier: 3
issues:
  - id: P0-1
    severity: P0
    section: "Spec ID Generation Race Condition"
    title: "NextID has a TOCTOU race allowing duplicate spec IDs"
  - id: P0-2
    severity: P0
    section: "Non-Atomic File Writes Across Codebase"
    title: "Most file writes use os.WriteFile without atomic rename or locking"
  - id: P1-1
    severity: P1
    section: "Inconsistent SQLite Configuration"
    title: "Signal store bypasses pkg/db/Open, creates separate connection pool without MaxOpenConns(1)"
  - id: P1-2
    severity: P1
    section: "No Schema Versioning"
    title: "No migration version tracking -- all migrations rely on CREATE IF NOT EXISTS"
  - id: P1-3
    severity: P1
    section: "Foreign Keys Not Enabled in Pollard or Events Databases"
    title: "Only Coldwine enables PRAGMA foreign_keys; Pollard and Events do not"
  - id: P1-4
    severity: P1
    section: "Archive Engine Partial Move Failure"
    title: "Multi-file archive/delete has no rollback on partial failure"
  - id: P1-5
    severity: P1
    section: "EmitAll Signal Batch Not Transactional"
    title: "EmitAll loops individual INSERTs without transaction wrapping"
  - id: P2-1
    severity: P2
    section: "Windows File Locking Is a No-Op"
    title: "lock_windows.go acquires no actual lock, AtomicWriteFile provides no safety on Windows"
  - id: P2-2
    severity: P2
    section: "Silent Time Parse Failures"
    title: "time.Parse errors discarded with _ across Pollard state, events, and signal stores"
  - id: P2-3
    severity: P2
    section: "No WAL Checkpoint Strategy"
    title: "No explicit WAL checkpointing -- relies on SQLite auto-checkpoint only"
  - id: P2-4
    severity: P2
    section: "OpenShared Connection Pool Never Closed"
    title: "Coldwine OpenShared caches *sql.DB globally with no Close path"
  - id: P2-5
    severity: P2
    section: "Sprint Persistence Uses os.WriteFile"
    title: "SaveSprintState writes YAML directly without atomic rename"
  - id: P2-6
    severity: P2
    section: "Insight LoadAll Silently Skips Corrupt Files"
    title: "Pollard LoadAll and LoadAllPRDs swallow parse errors with continue"
improvements:
  - id: IMP-1
    title: "Introduce a centralized atomic-write helper for all YAML/JSON writes"
    section: "Non-Atomic File Writes Across Codebase"
  - id: IMP-2
    title: "Add PRAGMA user_version-based schema migration tracking"
    section: "No Schema Versioning"
  - id: IMP-3
    title: "Route all SQLite opens through pkg/db.Open to enforce consistent pragmas"
    section: "Inconsistent SQLite Configuration"
  - id: IMP-4
    title: "Add file-lock-protected NextID or switch to UUID-based spec IDs"
    section: "Spec ID Generation Race Condition"
  - id: IMP-5
    title: "Add WAL checkpoint on graceful shutdown"
    section: "No WAL Checkpoint Strategy"
  - id: IMP-6
    title: "Enable foreign_keys pragma in pkg/db.Open for all consumers"
    section: "Foreign Keys Not Enabled in Pollard or Events Databases"
  - id: IMP-7
    title: "Wrap multi-file archive operations in a journal-based rollback mechanism"
    section: "Archive Engine Partial Move Failure"
  - id: IMP-8
    title: "Return structured warnings from LoadAll instead of silently skipping"
    section: "Insight LoadAll Silently Skips Corrupt Files"
verdict: needs-changes
---

## Summary

Autarch has a thoughtfully designed data layer with several strong foundations: WAL mode with NORMAL synchronous, a single-writer connection pool, an `AtomicWriteFile` utility with `flock`-based locking and `fsync`, and file-lock-serialized spec versioning. However, these good patterns are applied inconsistently. The `AtomicWriteFile` and file-locking mechanisms are only used in the spec evolution path while dozens of other write sites across Gurgeh, Pollard, and Coldwine use raw `os.WriteFile`. The signal store opens SQLite outside the standard `pkg/db.Open` helper, bypassing pragma enforcement. There is no schema migration versioning, no WAL checkpoint strategy, and foreign keys are only enabled in one of three database consumers. The spec ID generator has a classic TOCTOU race condition, and the multi-file archive engine has no rollback on partial failure.

## Section-by-Section Review

### 1. pkg/db/Open -- SQLite Connection Configuration

**File:** `/root/projects/Autarch/pkg/db/open.go`

The central SQLite helper is well-designed:
- WAL journal mode for concurrent reads during writes
- NORMAL synchronous (good balance of safety and performance for local-only tool)
- 5-second busy timeout (adequate for single-user local tool)
- `SetMaxOpenConns(1)` prevents writer contention
- `SetConnMaxLifetime(0)` prevents connection churn

**What is missing:**
- `PRAGMA foreign_keys = ON` is not set here. Coldwine's `storage.Open()` adds it separately at `/root/projects/Autarch/internal/coldwine/storage/db.go:18`, but Pollard (`state.Open()`) and the events store (`events.OpenStore()`) do not enable foreign keys at all. Foreign key constraints in those schemas are decorative only.
- No WAL checkpoint pragma or explicit checkpoint-on-close. For a local tool this is low risk but the WAL file can grow unbounded during long-running operations like Pollard scan cycles.

### 2. internal/gurgeh/signals/store.go -- Inconsistent DB Opening

**File:** `/root/projects/Autarch/internal/gurgeh/signals/store.go`

The signal store opens SQLite directly via `sql.Open("sqlite", path+"?_pragma=...")` at line 49 instead of using `pkg/db.Open()`. This creates several inconsistencies:

- No `SetMaxOpenConns(1)` -- the default `database/sql` pool allows multiple connections, which with SQLite can cause `SQLITE_BUSY` errors under write contention.
- The pragma DSN syntax (`?_pragma=journal_mode(wal)`) is driver-specific and fragile. The `modernc.org/sqlite` driver's DSN pragma support has had bugs historically. The rest of the codebase uses direct `db.Exec("PRAGMA ...")` which is more reliable.
- No busy timeout pragma is set via DSN (only `journal_mode` and `busy_timeout` are attempted, but the DSN format may not parse correctly for all driver versions).
- `PRAGMA synchronous=NORMAL` is not set at all.

### 3. internal/gurgeh/specs/id.go -- Spec ID Race Condition

**File:** `/root/projects/Autarch/internal/gurgeh/specs/id.go`

`NextID()` reads the directory, finds the highest PRD number, and returns `PRD-{N+1}`. But there is no lock between reading the directory and the caller writing the file. If two concurrent processes (or TUI instances, or an arbiter agent and a CLI command) both call `NextID()` before either writes, they get the same ID and one overwrites the other.

This is a TOCTOU (time-of-check-to-time-of-use) race condition. The callers at `/root/projects/Autarch/internal/gurgeh/specs/create.go:11` (`CreateTemplate`) and line 89 (`CreateBlank`) both call `NextID()` then `os.WriteFile()` without any locking.

Contrast with the spec evolution path at `/root/projects/Autarch/internal/gurgeh/specs/evolution.go:53-57` which correctly uses `fileutil.LockFile()` to serialize version assignment. The same pattern should be applied to spec creation.

### 4. internal/file/atomic.go and lock_unix.go -- Atomic Write Infrastructure

**Files:**
- `/root/projects/Autarch/internal/file/atomic.go`
- `/root/projects/Autarch/internal/file/lock_unix.go`
- `/root/projects/Autarch/internal/file/lock_windows.go`

The `AtomicWriteFile` implementation is solid on Unix: temp file, write, fsync, rename, directory fsync. It also acquires an flock before writing, preventing concurrent writes to the same target. The `SaveRevision` path in evolution.go correctly uses this.

**Critical gap:** The Windows implementation of `LockFile` at `/root/projects/Autarch/internal/file/lock_windows.go` acquires no lock at all -- it just opens the lockfile and returns. `AtomicWriteFile` on Windows is therefore not safe against concurrent writers. Since the CLAUDE.md states this is a local tool, Windows may not be a primary target, but the code compiles and ships for Windows, so this is a latent integrity risk.

### 5. Non-Atomic File Writes Across Codebase

The `AtomicWriteFile` utility exists but is only used in the spec evolution path. Every other write site uses raw `os.WriteFile`, which is not atomic (partial writes visible on crash, no fsync). Key locations:

| File | Function | Risk |
|------|----------|------|
| `internal/gurgeh/specs/create.go:86,102` | `CreateTemplate`, `CreateBlank` | Spec file corruption on crash |
| `internal/gurgeh/specs/load.go:94` | `UpdateStatus` | Spec status corruption |
| `internal/gurgeh/specs/metadata.go:28` | `StoreValidationWarnings` | Metadata corruption |
| `internal/gurgeh/specs/prd.go:110` | `PRD.Save` | PRD corruption |
| `internal/gurgeh/specs/praudemap.go:59` | `Praudemap.Save` | Roadmap corruption |
| `internal/gurgeh/tui/state_persist.go:43` | `SaveUIState` | UI state loss |
| `internal/gurgeh/arbiter/persistence.go:33` | `SaveSprintState` | Sprint recovery data loss |
| `internal/pollard/sources/source.go:255` | `SourceCollection.Save` | Research data loss |
| `internal/pollard/insights/insight.go:101` | `Insight.Save` | Insight data loss |
| `internal/pollard/watch/watcher.go:213` | `saveSnapshot` | Watch baseline corruption |
| `internal/gurgeh/cli/commands/approve.go:93` | Approve handler | Spec approval corruption |
| `internal/gurgeh/cli/commands/create.go:80` | Create handler | Spec creation corruption |
| `internal/gurgeh/cli/commands/apply.go:148` | Apply handler | Spec update corruption |
| `internal/gurgeh/cli/commands/interview.go:267` | Interview handler | Interview data loss |

For a local-only tool the probability of hitting these during normal operation is low, but during long Pollard scan cycles that write many files, or if the user's machine loses power, partial writes are possible.

### 6. internal/gurgeh/specs/evolution.go -- Spec Version Integrity

**File:** `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`

This is the strongest data integrity code in the repository. It demonstrates:
- File-lock serialization of version assignment (line 53-57)
- Filesystem-based version computation (not in-memory counter)
- Atomic writes for both snapshot and revision metadata
- Best-effort rollback: if the revision metadata write fails, the snapshot is removed (line 99-101)
- Input spec is not mutated (`snapshot := *spec` at line 79)
- Concurrent safety tested with 8 goroutines (`evolution_test.go:69`)

**Minor gap:** The rollback at line 100 (`os.Remove(snapPath)`) is best-effort. If the snapshot was written but the revision metadata failed AND the snapshot removal also fails, you end up with an orphaned snapshot file that has no corresponding revision metadata. `LoadHistory` only reads `_rev.yaml` files so the orphan would be invisible but would affect `nextHistoryVersion` since it scans all files with the version prefix.

### 7. internal/gurgeh/archive/engine.go -- Non-Atomic Multi-File Moves

**File:** `/root/projects/Autarch/internal/gurgeh/archive/engine.go`

The archive operation moves a spec file and its related artifacts (research, suggestions, briefs) across directories. Each move is done with `os.Rename`. If the spec move succeeds but an artifact move fails, the system is left in an inconsistent state: spec is archived but some artifacts remain in the active directory.

The `Undo` function at line 26 exists but is only callable externally -- there is no automatic rollback on partial failure. The caller would need to track which moves succeeded and call Undo, but the error return from `movePRD` does not include partial success information (the `Result` is empty on error).

### 8. internal/pollard/state/db.go -- Pollard State Database

**File:** `/root/projects/Autarch/internal/pollard/state/db.go`

Uses `pkg/db.Open` correctly. The schema uses `CREATE TABLE IF NOT EXISTS` which is idempotent. However:

- No foreign key relationships defined despite having a `hunter_name` column that could reference a hunters table.
- `StartRun` and `CompleteRun` are not transactional as a pair. If a process crashes between `StartRun` and `CompleteRun`, the run stays in "running" status forever. `ShouldRun` at line 225 checks for this and returns `false`, which means a crashed hunter blocks future runs of the same hunter indefinitely until manual intervention.
- Time parsing errors are silently discarded with `_` at lines 145, 148, 176, 179, 203, 277. A corrupt or empty timestamp in the database would silently produce a zero-valued `time.Time`.

### 9. internal/coldwine/storage/ -- Coldwine Database Layer

**Files:**
- `/root/projects/Autarch/internal/coldwine/storage/db.go`
- `/root/projects/Autarch/internal/coldwine/storage/schema.go`
- `/root/projects/Autarch/internal/coldwine/storage/coordination.go`
- `/root/projects/Autarch/internal/coldwine/storage/review.go`

This is the most mature database layer in the codebase:
- Foreign keys enabled via pragma (line 18 of db.go)
- Proper transaction usage in `SendMessage`, `ReservePaths`, `RenewReservations`, `ApproveTask`, `RejectTask`, `ApplyDetectionAtomic`, `UpsertAgent`
- ON DELETE CASCADE on foreign keys
- Appropriate indexes

**Issues:**
- `OpenShared` at line 36-48 of db.go caches `*sql.DB` instances in a global map with no Close path. The map grows monotonically and connections are never released. For a local tool with one or two databases this is acceptable but it is a resource leak pattern.
- The V1 and V2 migrations both use `CREATE TABLE IF NOT EXISTS` which means schema changes to existing tables require separate ALTER TABLE statements (like `addAttachmentColumns`). The ALTER TABLE approach at line 151-168 catches "duplicate column name" errors by string matching, which is fragile across SQLite driver versions and locales.
- No formal migration version tracking (`PRAGMA user_version` or a migrations table). If a V3 migration is needed that modifies column types or constraints, there is no way to know which version is currently applied.

### 10. pkg/events/store.go -- Event Store

**File:** `/root/projects/Autarch/pkg/events/store.go`

Uses `pkg/db.Open` correctly. The event store is append-only which is inherently safe. However:
- No foreign keys pragma enabled, so the `reconcile_cursors` and `reconcile_conflicts` tables have no referential integrity enforcement.
- The `Append` method at line 118 modifies `event.CreatedAt` and `event.ID` as side effects. If the caller retries on error, the timestamp will differ from the first attempt.
- Time parsing errors silently discarded at lines 219, 244, 316.

### 11. pkg/signals/broker.go -- In-Memory Signal Broker

**File:** `/root/projects/Autarch/pkg/signals/broker.go`

The broker is purely in-memory with no persistence, which means all in-flight signals are lost on process restart. This is documented behavior (the `Dropped` counter tracks overflow drops). The implementation is correct:
- Mutex-protected subscriber map
- Bounded channel (64) with evict-oldest overflow strategy
- Drop counter for observability
- Proper cleanup on `Close`

**Concern:** The `Publish` method holds the mutex for the entire fan-out loop (line 46-64). With many subscribers and a slow consumer, this blocks all other publishes and subscribes. For the current use case (single TUI process with a handful of subscribers) this is fine, but it would not scale.

### 12. Dual State Systems -- File + SQLite

The codebase has a split data model:
- **Gurgeh** uses YAML files on disk for specs, research, suggestions, briefs, sprint state, and UI state. SQLite is only used for the signals store.
- **Pollard** uses YAML files for sources, insights, reports, and watch snapshots. SQLite is only used for hunter run tracking and rate limits.
- **Coldwine** uses SQLite for everything (tasks, messages, reservations, agents, worktrees, sessions).
- **Events** (cross-tool) uses SQLite for the event log and reconciliation state.

There is no consistency mechanism between the file-based state and SQLite state. For example, if a Pollard hunter run is recorded as "success" in SQLite but the source YAML file write fails, the database says data was collected but the data is not actually on disk.

## Issues Found

### P0-1: NextID TOCTOU Race Condition

**File:** `/root/projects/Autarch/internal/gurgeh/specs/id.go:13-33`

**Risk:** Two concurrent spec creation operations (arbiter + CLI, two TUI instances) read the same max ID and generate the same `PRD-NNN`. The second writer silently overwrites the first spec.

**Example scenario:**
1. Process A calls `NextID()`, reads directory, finds PRD-005 as highest, returns PRD-006
2. Process B calls `NextID()`, reads directory, finds PRD-005 as highest, returns PRD-006
3. Process A writes PRD-006.yaml with spec A content
4. Process B writes PRD-006.yaml with spec B content, destroying spec A

**Fix:** Wrap NextID + file creation in a single locked region using the existing `fileutil.LockFile` mechanism, or use `os.OpenFile` with `O_CREATE|O_EXCL` to fail on existing file, then retry with the next ID.

### P0-2: Non-Atomic File Writes

**Risk:** Power loss or crash during `os.WriteFile` produces a truncated or empty file. On next load, the YAML parser fails and the data is lost.

**Fix:** Replace all `os.WriteFile` calls for data files with `fileutil.AtomicWriteFile`. This is a mechanical change. The function already exists and is tested.

### P1-1: Signal Store Bypasses pkg/db.Open

**File:** `/root/projects/Autarch/internal/gurgeh/signals/store.go:49`

**Risk:** No MaxOpenConns limit means multiple goroutines can open parallel write connections, causing SQLITE_BUSY errors or silent data loss under contention.

### P1-2: No Schema Versioning

**Risk:** No mechanism to determine which schema version a database is at. If a future migration needs to ALTER a table that already has `CREATE TABLE IF NOT EXISTS` applied, there is no safe way to know whether the ALTER has run.

### P1-3: Foreign Keys Not Enabled in Pollard or Events

**Files:**
- `/root/projects/Autarch/internal/pollard/state/db.go` (no FK pragma)
- `/root/projects/Autarch/pkg/events/store.go` (no FK pragma)

**Risk:** Foreign key constraints defined in schema DDL are not enforced. Orphaned records or referential integrity violations possible.

### P1-4: Archive Partial Failure

**File:** `/root/projects/Autarch/internal/gurgeh/archive/engine.go:38-59`

**Risk:** Spec is moved to archive/trash but related artifacts fail to move (permissions, disk full). No automatic rollback. Result struct is empty on error so caller cannot undo.

### P1-5: EmitAll Not Transactional

**File:** `/root/projects/Autarch/internal/gurgeh/signals/store.go:162-172`

**Risk:** A batch of signals is written one at a time. If the process crashes mid-batch, some signals are persisted and others are lost. For signals this is arguably acceptable, but the function also returns an error on non-UNIQUE failures, leaving the batch in an unknown partial state.

### P2-1: Windows File Locking No-Op

**File:** `/root/projects/Autarch/internal/file/lock_windows.go`

`LockFile` on Windows does not call `LockFileEx`. It just opens the file and returns. All AtomicWriteFile and evolution lock safety is void on Windows.

### P2-2: Silent Time Parse Failures

Multiple locations use `time.Parse` and discard the error with `_`. If stored timestamps are corrupt or in an unexpected format, the code silently uses `time.Time{}` (zero value, year 0001). This affects:
- `/root/projects/Autarch/internal/pollard/state/db.go:145,148,176,179,203,277`
- `/root/projects/Autarch/pkg/events/store.go:219,244,316`

### P2-3: No WAL Checkpoint Strategy

No code calls `PRAGMA wal_checkpoint`. SQLite auto-checkpoints at 1000 pages by default, but long-running Pollard scans or Coldwine coordination sessions could accumulate a large WAL file.

### P2-4: OpenShared Never Closes

**File:** `/root/projects/Autarch/internal/coldwine/storage/db.go:33-48`

The `sharedDBs` map caches connections indefinitely with no eviction or close mechanism.

### P2-5: Sprint Persistence Non-Atomic

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/persistence.go:33`

Uses `os.WriteFile` directly. A crash during sprint save loses the recovery checkpoint that enables interrupted sprint resumption, which is the entire point of sprint persistence.

### P2-6: Silent Error Swallowing in LoadAll

**Files:**
- `/root/projects/Autarch/internal/pollard/insights/insight.go:122` (line: `continue // Skip invalid files`)
- `/root/projects/Autarch/internal/gurgeh/specs/prd.go:136` (line: `continue // Skip invalid files`)

Corrupt or partially-written files are silently skipped. The user sees fewer items than expected with no indication of data loss.

## Improvements Suggested

### IMP-1: Centralize Atomic Writes

Create a project-wide lint or convention that all data file writes go through `fileutil.AtomicWriteFile`. Consider a wrapper like `fileutil.WriteYAML(path, v)` that marshals and atomically writes in one call.

### IMP-2: Schema Migration Versioning

Use `PRAGMA user_version` to track the current schema version in each database. Check the version on open and apply migrations sequentially. This is a minimal change to `pkg/db.Open` or each consumer's migrate function:

```go
var version int
_ = db.QueryRow("PRAGMA user_version").Scan(&version)
if version < 1 { /* apply v1 schema */ }
if version < 2 { /* apply v2 schema */ }
db.Exec(fmt.Sprintf("PRAGMA user_version = %d", currentVersion))
```

### IMP-3: Route All SQLite Through pkg/db.Open

Refactor the signal store at `/root/projects/Autarch/internal/gurgeh/signals/store.go` to use `autarchdb.Open(dbPath)` instead of `sql.Open` with DSN pragmas. This ensures consistent WAL, synchronous, busy timeout, and connection pool settings.

### IMP-4: Lock-Protected Spec ID Generation

Wrap the NextID + WriteFile sequence in the existing `fileutil.LockFile` mechanism, or use `O_CREATE|O_EXCL` to prevent overwrites:

```go
func CreateBlankSafe(dir string, now time.Time) (string, string, error) {
    lock, _ := fileutil.LockFile(filepath.Join(dir, ".nextid"))
    defer lock.Unlock()
    id, _ := NextID(dir)
    path := filepath.Join(dir, id+".yaml")
    // ... write file ...
}
```

### IMP-5: WAL Checkpoint on Shutdown

Add a `Close()` method or shutdown hook that runs `PRAGMA wal_checkpoint(TRUNCATE)` before closing the database. This ensures the WAL file is cleaned up on graceful exit.

### IMP-6: Enable Foreign Keys in pkg/db.Open

Move `PRAGMA foreign_keys = ON` into the central `pkg/db.Open` pragma list so all consumers get referential integrity enforcement by default.

### IMP-7: Journal-Based Archive Rollback

Record the planned moves before executing them, then execute, then remove the journal. On startup, if a journal exists, replay it in reverse for rollback. Or simpler: collect all moves, execute them, and if any fails, reverse all completed moves before returning the error.

### IMP-8: Structured Warnings from LoadAll

Return a `([]T, []Warning, error)` tuple from LoadAll-style functions so callers can display which files were skipped and why. The `LoadSummaries` function in `/root/projects/Autarch/internal/gurgeh/specs/load.go` already returns warnings as a separate slice -- extend this pattern to Pollard's `LoadAll` functions.

## Overall Assessment

The Autarch data layer has strong foundations in its core primitives (`AtomicWriteFile`, `flock`-based locking, single-writer SQLite pools, WAL mode). The spec evolution system is particularly well-engineered with concurrent-safe versioning backed by tests. However, these patterns are only applied in the newest code paths. The majority of file writes (roughly 30+ sites) use raw `os.WriteFile`, the signal store bypasses the central SQLite helper, there is no schema versioning, and foreign keys are inconsistently enabled.

The most critical risks are the spec ID generation race (P0-1, data loss via silent overwrite) and the widespread non-atomic writes (P0-2, data corruption on crash). Both have straightforward fixes using infrastructure that already exists in the codebase. The inconsistent SQLite configuration (P1-1) and missing foreign key enforcement (P1-3) are structural issues that should be addressed before the database schema grows further.

For a local-only development tool, the current state is functional and the probability of hitting these issues in normal use is low. However, as the tool moves toward multi-agent concurrent operation (arbiter + TUI + CLI), the race conditions and non-atomic writes become increasingly likely to manifest. The verdict is **needs-changes** -- the existing primitives are solid but need to be applied consistently across the codebase.
