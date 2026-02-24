# Deployment verification review

**Source:** Agent result from session b817bfb0
**Task ID:** a69a693
**Date:** 2026-02-05

---

I now have a thorough understanding of the entire data layer. Let me compile the comprehensive deployment verification checklist.

---

# Deployment Checklist: Autarch Acceptance Criteria Plan

This checklist covers the deployment and migration risks identified from analyzing the acceptance criteria plan at `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`, cross-referenced against the actual codebase: database schemas, migration logic, file-based persistence formats, and coordination layer code.

---

## SECTION 1: Data Inventory and Invariants

Autarch has **five separate SQLite databases** and **three YAML/file persistence layers**. Each has distinct migration and rollback characteristics.

### SQLite Databases

| Database | Location | Schema Source | Has Schema Versioning |
|----------|----------|--------------|----------------------|
| Coldwine state | `.coldwine/state.db` | `internal/coldwine/storage/db.go:Migrate()` | NO |
| Coldwine V2 (epics/stories/tasks) | `.coldwine/state.db` | `internal/coldwine/storage/schema.go:MigrateV2()` | NO |
| Pollard state | `.pollard/state.db` | `internal/pollard/state/db.go:migrate()` | NO |
| Gurgeh signals | `.gurgeh/signals/signals.db` | `internal/gurgeh/signals/store.go:createTableSQL` | NO |
| Event spine | `~/.autarch/events.db` | `pkg/events/store.go:migrate()` | NO |

### File-Based Persistence

| Data | Location | Format | Git-Tracked |
|------|----------|--------|-------------|
| Specs | `.gurgeh/specs/*.yaml` | YAML | Yes |
| Spec history | `.gurgeh/specs/history/*_v*.yaml` | YAML (snapshot + rev metadata) | Yes |
| Sprint state | `.gurgeh/sprints/*.json` | JSON | Yes |
| Feedback | `.pollard/feedback.yaml` | YAML | Yes |
| Global prefs | `~/.autarch/pollard-preferences.yaml` | YAML | No |
| Coldwine tasks (legacy) | `.coldwine/*.yaml` or `.tandemonium/*.yaml` | YAML | Yes |

### Data Invariants That Must Remain True

```
- [ ] INV-1: All Coldwine foreign keys remain valid (stories->epics, work_tasks->stories, agent_sessions->work_tasks, worktrees->work_tasks)
- [ ] INV-2: Gurgeh signal dedup constraint UNIQUE(spec_id, type, affected_field) is present and enforced in .gurgeh/signals/signals.db
- [ ] INV-3: No orphaned spec history files exist (every *_v*_rev.yaml has a matching *_v*.yaml snapshot)
- [ ] INV-4: Reservation expiry timestamps are in RFC3339Nano format in both Coldwine local storage and Intermute server
- [ ] INV-5: Event spine events are monotonically increasing by ID (no gaps break Replay())
- [ ] INV-6: Legacy directory names (.praude/, .tandemonium/) have been migrated to .gurgeh/ and .coldwine/ respectively
- [ ] INV-7: Spec version numbers are sequential per spec_id (no duplicates from concurrent SaveRevision)
- [ ] INV-8: Coldwine task status values are constrained to: todo, in_progress, blocked, done
- [ ] INV-9: All SQLite databases are in WAL mode (not journal/delete)
- [ ] INV-10: .gurgeh/ and .coldwine/ contain no binary blobs (YAML/text only for git tracking)
```

---

## SECTION 2: Pre-Deploy Audits (Read-Only SQL Queries)

Run these queries against each SQLite database BEFORE deploying any changes. Save all output as baseline values.

### 2.1 Coldwine State Database (`.coldwine/state.db`)

```sql
-- BASELINE-C1: Task status distribution
SELECT status, COUNT(*) as cnt FROM tasks GROUP BY status;

-- BASELINE-C2: Epic/Story/Task hierarchy counts (V2 schema)
SELECT 'epics' as entity, COUNT(*) as cnt FROM epics
UNION ALL SELECT 'stories', COUNT(*) FROM stories
UNION ALL SELECT 'work_tasks', COUNT(*) FROM work_tasks
UNION ALL SELECT 'agent_sessions', COUNT(*) FROM agent_sessions
UNION ALL SELECT 'worktrees', COUNT(*) FROM worktrees;

-- BASELINE-C3: Foreign key integrity check
PRAGMA foreign_key_check;
-- Expected: empty result (no violations)

-- BASELINE-C4: Orphaned stories (epic_id not in epics)
SELECT s.id, s.epic_id FROM stories s
LEFT JOIN epics e ON s.epic_id = e.id
WHERE e.id IS NULL;
-- Expected: 0 rows

-- BASELINE-C5: Orphaned work_tasks (story_id not in stories)
SELECT wt.id, wt.story_id FROM work_tasks wt
LEFT JOIN stories s ON wt.story_id = s.id
WHERE s.id IS NULL;
-- Expected: 0 rows

-- BASELINE-C6: Active reservations (not released, not expired)
SELECT COUNT(*) as active_reservations FROM reservations
WHERE released_ts IS NULL AND expires_ts > datetime('now');

-- BASELINE-C7: WAL mode check
PRAGMA journal_mode;
-- Expected: wal

-- BASELINE-C8: Foreign keys enabled check
PRAGMA foreign_keys;
-- Expected: 1

-- BASELINE-C9: Check for schema_version table (should NOT exist yet)
SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version';
-- Expected: 0 rows (schema versioning is a known TODO: docs/coldwine/todos/020-pending-p3-add-schema-versioning.md)
```

### 2.2 Pollard State Database (`.pollard/state.db`)

```sql
-- BASELINE-P1: Hunter run statistics
SELECT hunter_name, status, COUNT(*) as cnt
FROM hunter_runs GROUP BY hunter_name, status;

-- BASELINE-P2: Active rate limits
SELECT api_name, requests_remaining, reset_at FROM rate_limits;

-- BASELINE-P3: Running hunts (should be 0 at deploy time)
SELECT COUNT(*) as running FROM hunter_runs WHERE status = 'running';
-- Expected: 0 (no hunts should be running during deploy)

-- BASELINE-P4: WAL mode check
PRAGMA journal_mode;
-- Expected: wal
```

### 2.3 Gurgeh Signals Database (`.gurgeh/signals/signals.db`)

```sql
-- BASELINE-G1: Active (non-dismissed) signals by spec
SELECT spec_id, type, COUNT(*) as cnt FROM signals
WHERE dismissed_at IS NULL
GROUP BY spec_id, type;

-- BASELINE-G2: Dedup constraint verification
SELECT sql FROM sqlite_master
WHERE type='table' AND name='signals';
-- Expected: Contains UNIQUE(spec_id, type, affected_field)

-- BASELINE-G3: Total signals
SELECT COUNT(*) as total, COUNT(dismissed_at) as dismissed FROM signals;
```

### 2.4 Event Spine (`~/.autarch/events.db`)

```sql
-- BASELINE-E1: Event counts by type and source
SELECT source_tool, event_type, COUNT(*) as cnt
FROM events GROUP BY source_tool, event_type;

-- BASELINE-E2: Latest event ID (for replay cursor)
SELECT COALESCE(MAX(id), 0) as last_id FROM events;

-- BASELINE-E3: Reconcile conflicts (should be 0 ideally)
SELECT COUNT(*) as conflicts FROM reconcile_conflicts;

-- BASELINE-E4: Reconcile cursor state
SELECT project_path, entity_type, COUNT(*) as cnt
FROM reconcile_cursors GROUP BY project_path, entity_type;
```

### 2.5 File-Based Persistence Checks

```bash
# BASELINE-F1: Count spec files
find .gurgeh/specs/ -name "*.yaml" -not -path "*/history/*" 2>/dev/null | wc -l

# BASELINE-F2: Count spec history files (should be pairs: snapshot + rev)
find .gurgeh/specs/history/ -name "*_v*.yaml" 2>/dev/null | wc -l

# BASELINE-F3: Verify history pairs are complete
for rev in .gurgeh/specs/history/*_rev.yaml; do
  snap="${rev/_rev.yaml/.yaml}"
  [ ! -f "$snap" ] && echo "MISSING SNAPSHOT: $snap"
done

# BASELINE-F4: Count sprint state files
find .gurgeh/sprints/ -name "*.json" 2>/dev/null | wc -l

# BASELINE-F5: Validate YAML files parse correctly
for f in .gurgeh/specs/*.yaml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))" 2>/dev/null || echo "INVALID YAML: $f"
done

# BASELINE-F6: Check feedback.yaml size (rolling window should be <=50 entries)
wc -l .pollard/feedback.yaml 2>/dev/null || echo "No feedback.yaml"

# BASELINE-F7: Verify legacy directories do not coexist with new
[ -d ".praude" ] && [ -d ".gurgeh" ] && echo "CONFLICT: Both .praude/ and .gurgeh/ exist"
[ -d ".tandemonium" ] && [ -d ".coldwine" ] && echo "CONFLICT: Both .tandemonium/ and .coldwine/ exist"

# BASELINE-F8: Verify no binary blobs in tracked directories
find .gurgeh/ .coldwine/ .pollard/ -type f ! -name "*.yaml" ! -name "*.json" ! -name "*.md" ! -name "*.txt" ! -name "*.db" ! -name "*.db-wal" ! -name "*.db-shm" 2>/dev/null
# Expected: empty (no unexpected file types)
```

---

## SECTION 3: Critical Migration Risks and Destructive Steps

### 3.1 Risk Matrix

| Risk | Severity | Component | Code Location | Mitigation |
|------|----------|-----------|---------------|------------|
| No schema versioning in any SQLite DB | HIGH | All | `pkg/db/open.go`, all `Migrate()` functions | `CREATE TABLE IF NOT EXISTS` provides idempotency but no downgrade path |
| Non-atomic SaveRevision (two sequential file writes) | HIGH | Gurgeh | `/root/projects/Autarch/internal/gurgeh/specs/evolution.go:41-81` | Crash between snapshot write (line 66) and rev metadata write (line 76) creates orphaned snapshot |
| SaveRevision mutates input spec.Version as side effect | MEDIUM | Gurgeh | `evolution.go:47-48` (`spec.Version = version`) | If file writes fail, spec struct has wrong version number in memory |
| Concurrent SaveRevision can duplicate version numbers | HIGH | Gurgeh | `evolution.go:47` (no locking) | Two goroutines read same `spec.Version`, both compute `version+1`, both write |
| Glob overlap not detected in reservations | CRITICAL | Intermute | `/root/projects/Intermute/internal/storage/sqlite/sqlite.go:813-839` | Simple INSERT with no overlap check -- `internal/auth/**/*.go` and `internal/**/*.go` both succeed |
| Coldwine local ReservePaths uses exact string matching | HIGH | Coldwine | `/root/projects/Autarch/internal/coldwine/storage/coordination.go:258` | Only checks `path = ?`, not glob expansion |
| MaxOpenConns(1) bottleneck with concurrent agents | MEDIUM | All SQLite | `/root/projects/Autarch/pkg/db/open.go:22` | Under 3+ concurrent agent sessions, SQLITE_BUSY likely despite 5s timeout |
| Feedback rolling window has no crash recovery | MEDIUM | Pollard | Plan describes archive-then-truncate (not yet implemented) | Process crash between archive and truncation causes duplicates |
| Coldwine V1 and V2 schemas coexist in same DB | MEDIUM | Coldwine | `db.go:Migrate()` creates `tasks`, `schema.go:MigrateV2()` creates `epics/stories/work_tasks` | Both `tasks` and `work_tasks` tables exist -- which is authoritative? |
| Legacy directory migration is one-way rename | LOW | All | `/root/projects/Autarch/cmd/autarch/migrate.go:161` | `os.Rename()` is atomic on same filesystem but leaves `.migrated` marker |

### 3.2 Migration Step Table

| Step | Command | Estimated Runtime | Batching | Rollback Possible | Notes |
|------|---------|-------------------|----------|-------------------|-------|
| 1. Back up all SQLite databases | `cp .coldwine/state.db .coldwine/state.db.bak` (repeat for each) | <5s | N/A | N/A | MANDATORY before any schema change |
| 2. Back up file-based persistence | `tar czf autarch-data-backup.tar.gz .gurgeh/ .coldwine/ .pollard/` | <30s | N/A | Extract from tar | Include in git commit |
| 3. Run legacy directory migration | `autarch migrate --dry-run` then `autarch migrate` | <1s | N/A | Rename back + remove `.migrated` marker | Check for conflicts first |
| 4. Build all binaries | `go build ./cmd/...` | <60s | N/A | Use previous binary | Verify with `go test ./...` |
| 5. Run all tests with race detector | `go test -race ./internal/gurgeh/arbiter/...` | 30-120s | N/A | N/A | MANDATORY per institutional learnings |
| 6. Start Intermute | Verify Intermute is running at `$INTERMUTE_URL` | <5s | N/A | N/A | All Coldwine coordination depends on this |
| 7. Verify Coldwine schema migration | Open `.coldwine/state.db` and run `SELECT name FROM sqlite_master WHERE type='table'` | <1s | N/A | Restore from backup | Should show both V1 and V2 tables |
| 8. Enable feature flags | Set `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` if testing CUJ-4 | Instant | N/A | Unset env var | Additive -- all features work without it |

---

## SECTION 4: Post-Deploy Verification (Within 5 Minutes)

### 4.1 SQLite Schema Integrity

```sql
-- POST-C1: Verify Coldwine tables exist (both V1 and V2)
SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;
-- Expected: agent_sessions, agents, attachments, contact_policies, contact_requests,
--   epics, events, mailboxes, messages, reservations, review_queue, sessions,
--   stories, tasks, work_tasks, worktrees

-- POST-C2: Verify all indexes exist
SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%' ORDER BY name;
-- Expected: All idx_* indexes from both Migrate() and MigrateV2()

-- POST-C3: Foreign key check (Coldwine)
PRAGMA foreign_keys;
-- Expected: 1
PRAGMA foreign_key_check;
-- Expected: empty

-- POST-C4: Verify WAL mode on all databases
-- Run on each: .coldwine/state.db, .pollard/state.db, .gurgeh/signals/signals.db, ~/.autarch/events.db
PRAGMA journal_mode;
-- Expected: wal
```

### 4.2 Gurgeh Signal Deduplication

```sql
-- POST-G1: Verify UNIQUE constraint exists
SELECT sql FROM sqlite_master WHERE type='table' AND name='signals';
-- Expected output must contain: UNIQUE(spec_id, type, affected_field)

-- POST-G2: Verify dedup works (attempt duplicate insert)
-- This is a READ test -- do NOT run in production
-- INSERT OR IGNORE INTO signals (...) VALUES (same spec_id, type, affected_field)
-- Verify rowcount = 0
```

### 4.3 Data Counts Match Baseline

```sql
-- POST-MATCH-1: Coldwine task counts unchanged
SELECT status, COUNT(*) as cnt FROM tasks GROUP BY status;
-- Compare with BASELINE-C1

-- POST-MATCH-2: Event spine last ID unchanged or higher
SELECT COALESCE(MAX(id), 0) as last_id FROM events;
-- Must be >= BASELINE-E2 value

-- POST-MATCH-3: Pollard hunter runs unchanged
SELECT hunter_name, status, COUNT(*) as cnt
FROM hunter_runs GROUP BY hunter_name, status;
-- Compare with BASELINE-P1

-- POST-MATCH-4: Active signal counts unchanged
SELECT spec_id, type, COUNT(*) as cnt FROM signals
WHERE dismissed_at IS NULL GROUP BY spec_id, type;
-- Compare with BASELINE-G1
```

### 4.4 Service Health Checks

```bash
# POST-SVC-1: Verify all servers start and bind to loopback
go run ./cmd/pollard serve --addr 127.0.0.1:8090 &
sleep 2
curl -sf http://127.0.0.1:8090/healthz && echo "Pollard OK" || echo "Pollard FAIL"

go run ./cmd/gurgeh serve --addr 127.0.0.1:8091 &
sleep 2
curl -sf http://127.0.0.1:8091/healthz && echo "Gurgeh OK" || echo "Gurgeh FAIL"

go run ./cmd/signals serve --addr 127.0.0.1:8092 &
sleep 2
curl -sf http://127.0.0.1:8092/healthz && echo "Signals OK" || echo "Signals FAIL"

# POST-SVC-2: Verify Intermute connectivity
curl -sf ${INTERMUTE_URL}/healthz && echo "Intermute OK" || echo "Intermute FAIL"

# POST-SVC-3: Verify servers reject non-loopback without explicit flag
# This should fail:
go run ./cmd/pollard serve --addr 0.0.0.0:8090 2>&1 | grep -q "error\|denied\|rejected" && echo "Non-loopback correctly rejected"

# POST-SVC-4: TUI launches successfully
timeout 10 ./dev autarch tui --skip-onboard 2>&1 | head -5
# Expected: no crash, no panic
```

### 4.5 Reservation System Verification

```bash
# POST-RES-1: Test Coldwine local reservation path conflict detection
# Using sqlite3 against .coldwine/state.db:
sqlite3 .coldwine/state.db "
  INSERT INTO reservations (path, owner, exclusive, reason, created_ts, expires_ts)
  VALUES ('test/path.go', 'test-agent', 1, 'test', datetime('now'), datetime('now', '+1 hour'));
  SELECT COUNT(*) FROM reservations WHERE path='test/path.go' AND released_ts IS NULL;
"
# Expected: 1
# IMPORTANT: Clean up test data:
sqlite3 .coldwine/state.db "
  UPDATE reservations SET released_ts = datetime('now') WHERE owner='test-agent';
"
```

---

## SECTION 5: Rollback Plan

### 5.1 Rollback Classification

| Component | Rollback Type | Procedure | Data Loss Risk |
|-----------|--------------|-----------|----------------|
| Go binaries | Full rollback | `git checkout <previous-commit> && go build ./cmd/...` | None |
| Coldwine SQLite | Full rollback | Restore `.coldwine/state.db.bak` | Loses data written since backup |
| Pollard SQLite | Full rollback | Restore `.pollard/state.db.bak` | Loses hunter runs since backup |
| Gurgeh signals SQLite | Full rollback | Restore `.gurgeh/signals/signals.db.bak` | Loses signals since backup |
| Event spine | Full rollback | Restore `~/.autarch/events.db.bak` | Loses events since backup (not git-tracked, acceptable) |
| Spec YAML files | Full rollback | `git checkout <previous-commit> -- .gurgeh/specs/` | None (git-tracked) |
| Spec history | Partial rollback | Remove newly added `*_v*.yaml` files | Version numbers may have gaps |
| Sprint state JSON | Full rollback | `git checkout <previous-commit> -- .gurgeh/sprints/` | Loses in-progress sprint state |
| Legacy directory migration | Full rollback | `mv .gurgeh .praude && mv .coldwine .tandemonium && rm *.migrated` | None (atomic rename) |
| Intermute reservations | Manual cleanup | Release via API: `curl -X DELETE $INTERMUTE_URL/api/reservations/<id>` | None (reservations are transient) |
| Feedback YAML | Full rollback | `git checkout <previous-commit> -- .pollard/feedback.yaml` | Loses triage decisions since backup |

### 5.2 Rollback Steps (Execute in Order)

```bash
# Step 1: Stop all running servers
pkill -f "go run ./cmd/pollard"
pkill -f "go run ./cmd/gurgeh"
pkill -f "go run ./cmd/signals"
pkill -f "go run ./cmd/bigend"

# Step 2: Restore SQLite databases from backups
cp .coldwine/state.db.bak .coldwine/state.db 2>/dev/null
cp .pollard/state.db.bak .pollard/state.db 2>/dev/null
cp .gurgeh/signals/signals.db.bak .gurgeh/signals/signals.db 2>/dev/null
cp ~/.autarch/events.db.bak ~/.autarch/events.db 2>/dev/null

# Step 3: Restore git-tracked files
git checkout <previous-commit> -- .gurgeh/ .coldwine/ .pollard/

# Step 4: Deploy previous binary
git checkout <previous-commit>
go build ./cmd/...

# Step 5: Restart servers
# (use your normal startup procedure)

# Step 6: Verify with post-rollback queries
# Re-run all BASELINE queries and compare with saved baseline values
```

### 5.3 Rollback Decision Matrix

| Condition | Action |
|-----------|--------|
| Any BASELINE query returns different counts post-deploy | INVESTIGATE -- compare row-by-row |
| Foreign key violations detected (PRAGMA foreign_key_check returns rows) | ROLLBACK IMMEDIATELY |
| WAL mode not set on any database | ROLLBACK -- data corruption risk |
| Server fails to bind to loopback | INVESTIGATE -- likely port conflict, not data issue |
| Intermute unreachable | DEGRADED MODE OK -- tools continue without coordination (AC-X.5) |
| Spec history has orphaned files (snapshot without rev or vice versa) | ROLLBACK -- SaveRevision atomicity failure confirmed |
| Duplicate spec version numbers detected | ROLLBACK -- concurrent SaveRevision race condition |
| SQLITE_BUSY errors in logs | MONITOR -- may resolve with reduced concurrency; escalate if persistent >5 min |

---

## SECTION 6: Post-Deploy Monitoring (First 24 Hours)

### 6.1 Monitoring Metrics

| Metric | How to Check | Alert Condition | Frequency |
|--------|-------------|-----------------|-----------|
| SQLite BUSY errors | `grep -i "SQLITE_BUSY\|database is locked" *.log` | Any occurrence with 3+ agents | Every 15 min for first 4 hours |
| Reservation conflict false negatives | Query Intermute for overlapping active reservations on same project | Two exclusive reservations with overlapping glob patterns | Every 30 min |
| Spec version gaps | `ls .gurgeh/specs/history/ \| sort -V` and check for gaps | Non-sequential version numbers | After each sprint completion |
| Signal dedup failures | `SELECT spec_id, type, affected_field, COUNT(*) FROM signals GROUP BY 1,2,3 HAVING COUNT(*) > 1` | Any row with count > 1 | Every hour |
| Event spine growth | `SELECT COUNT(*) FROM events` on `~/.autarch/events.db` | >10000 events/day (abnormal) | Every 4 hours |
| Feedback YAML size | `wc -l .pollard/feedback.yaml` | >50 entries (rolling window exceeded) | Every 4 hours |
| WAL checkpoint | `PRAGMA wal_checkpoint(PASSIVE)` on each database | WAL file >10MB | Every 4 hours |
| Orphaned spec history | `for rev in .gurgeh/specs/history/*_rev.yaml; do snap="${rev/_rev.yaml/.yaml}"; [ ! -f "$snap" ] && echo "ORPHAN: $rev"; done` | Any orphan found | After each deploy |

### 6.2 Console Spot Checks

Run these at +1h, +4h, and +24h after deploy:

```bash
# CHECK-1: Verify all databases are accessible
for db in .coldwine/state.db .pollard/state.db .gurgeh/signals/signals.db ~/.autarch/events.db; do
  if [ -f "$db" ]; then
    sqlite3 "$db" "PRAGMA integrity_check;" 2>/dev/null | head -1
    echo "  $db: $(sqlite3 "$db" 'PRAGMA integrity_check;' 2>/dev/null | head -1)"
  else
    echo "  $db: NOT FOUND"
  fi
done
# Expected: "ok" for each

# CHECK-2: WAL file sizes (should not grow unbounded)
ls -lh .coldwine/state.db-wal .pollard/state.db-wal .gurgeh/signals/signals.db-wal ~/.autarch/events.db-wal 2>/dev/null

# CHECK-3: Test round-trip: create task, verify in DB
# (Only run in staging/dev, not production)
go test ./internal/coldwine/storage/... -run TestMigrate -v

# CHECK-4: Verify go test -race still passes
go test -race ./internal/gurgeh/arbiter/... -timeout 120s
```

### 6.3 Known Gotchas to Watch For

**1. SaveRevision non-atomicity** (`/root/projects/Autarch/internal/gurgeh/specs/evolution.go:41-81`):
The function writes two files sequentially: snapshot at line 66, then revision metadata at line 76. A crash or power loss between these writes produces an orphaned snapshot file. The function also mutates `spec.Version` at line 48 before any writes -- if either write fails, the in-memory spec has an incorrect version number. Monitor for orphaned files in `.gurgeh/specs/history/`.

**2. Coldwine dual schema** (`/root/projects/Autarch/internal/coldwine/storage/db.go` + `schema.go`):
Two migration functions exist: `Migrate()` creates V1 tables (`tasks`, `sessions`, `reservations`, etc.) and `MigrateV2()` creates V2 tables (`epics`, `stories`, `work_tasks`, `agent_sessions`, `worktrees`). Both coexist in the same database. The V1 `tasks` table and V2 `work_tasks` table serve different purposes but this is confusing. The `sessions` (V1) and `agent_sessions` (V2) tables are also distinct. Verify no code path accidentally writes to the wrong table.

**3. Intermute reservation blind spot** (`/root/projects/Intermute/internal/storage/sqlite/sqlite.go:813-839`):
The `Reserve()` function performs a simple INSERT with zero overlap detection. Two agents can simultaneously hold exclusive reservations on `internal/auth/**/*.go` and `internal/**/*.go`. The Coldwine local `ReservePaths` (`/root/projects/Autarch/internal/coldwine/storage/coordination.go:258`) checks exact string match only (`WHERE path = ?`). This means CUJ-4 (Parallel Agent Development) isolation guarantees are **not enforceable** until glob overlap detection is implemented. This is documented as "Gap 1" in the plan and is a **blocking dependency** for CUJ-4 acceptance criteria.

**4. No schema versioning** (`/root/projects/Autarch/docs/coldwine/todos/020-pending-p3-add-schema-versioning.md`):
None of the five SQLite databases have a `schema_version` table. All migrations use `CREATE TABLE IF NOT EXISTS` which is idempotent for creation but provides no mechanism to detect schema drift, apply incremental changes, or roll back specific migrations. The `addAttachmentColumns` in `db.go:151-168` adds columns by catching "duplicate column name" errors -- this works but is fragile and not generalizable.

**5. .gitignore gaps**:
The `.gitignore` at `/root/projects/Autarch/.gitignore` lists `.autarch/`, `.praude/`, `.pollard/`, `.tandemonium/` but does NOT list `.gurgeh/` or `.coldwine/`. This means `.gurgeh/` and `.coldwine/` directories ARE git-tracked, which is intentional for YAML specs and tasks but inadvertently includes SQLite database files (`.coldwine/state.db`, `.gurgeh/signals/signals.db`). These `.db` files should be excluded via a more specific gitignore pattern. Check with `git ls-files .gurgeh/ .coldwine/` to verify what is actually tracked.

---

## SECTION 7: Go/No-Go Summary Checklist

### RED: Pre-Deploy (Required -- All Must Pass)

```
- [ ] Run baseline SQL queries (Section 2) on all 4 SQLite databases and save output
- [ ] Run file-based persistence checks (BASELINE-F1 through F8)
- [ ] Verify no legacy directory conflicts (both .praude/ and .gurgeh/ existing)
- [ ] Verify go build ./cmd/... succeeds with zero errors
- [ ] Verify go test -race ./internal/gurgeh/arbiter/... passes (institutional requirement)
- [ ] Verify go test ./... passes (full test suite)
- [ ] Back up all SQLite databases (.bak copies)
- [ ] Back up file-based persistence (tar archive)
- [ ] Verify Intermute is running and healthy (curl $INTERMUTE_URL/healthz)
- [ ] Confirm rollback plan is reviewed and understood by deploying engineer
- [ ] Confirm BLOCKING dependency: Intermute glob overlap detection status
      (If not implemented, CUJ-4 AC-4.2 is untestable -- document acceptance of this risk)
- [ ] Verify .gitignore correctly excludes .db files from .gurgeh/ and .coldwine/
```

### YELLOW: Deploy Steps

```
1. [ ] Run legacy migration if needed: autarch migrate --dry-run, then autarch migrate
2. [ ] Build new binaries: go build ./cmd/...
3. [ ] Start services in order: Intermute first, then Pollard, Gurgeh, Signals, Bigend
4. [ ] Verify each service binds to 127.0.0.1 (check with ss -tlnp or lsof -i)
5. [ ] Set CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS if testing CUJ-4 features
```

### GREEN: Post-Deploy (Within 5 Minutes)

```
- [ ] Run post-deploy SQL queries (Section 4.1-4.3) on all databases
- [ ] Compare post-deploy counts with saved baseline values (Section 4.3)
- [ ] Run service health checks (Section 4.4: POST-SVC-1 through POST-SVC-4)
- [ ] Run PRAGMA integrity_check on all databases
- [ ] Run PRAGMA foreign_key_check on Coldwine state.db
- [ ] Verify TUI launches without crash: ./dev autarch tui --skip-onboard
- [ ] Spot check: open Gurgeh tab, verify specs list loads
- [ ] Spot check: open Pollard tab, verify hunter status displays
```

### BLUE: Monitoring (24 Hours)

```
- [ ] Set up monitoring for SQLITE_BUSY errors (grep logs every 15 min)
- [ ] Check spec history for orphaned files at +1h
- [ ] Run signal dedup verification query at +1h, +4h, +24h
- [ ] Check WAL file sizes at +4h (should not exceed 10MB)
- [ ] Run PRAGMA integrity_check on all databases at +24h
- [ ] Verify feedback.yaml rolling window size at +4h
- [ ] Review event spine growth rate at +24h
- [ ] Run go test -race ./internal/gurgeh/arbiter/... at +24h to confirm no regressions
```

### ROLLBACK: If Needed (Section 5.2)

```
1. [ ] Stop all running Autarch services
2. [ ] Restore SQLite databases from .bak files
3. [ ] Restore git-tracked files from previous commit
4. [ ] Rebuild previous binaries: git checkout <sha> && go build ./cmd/...
5. [ ] Restart services
6. [ ] Re-run all BASELINE queries and verify counts match original baseline
7. [ ] Verify PRAGMA integrity_check passes on all restored databases
8. [ ] Document the failure reason and file an issue
```

---

## SECTION 8: Blocking Issues for Full Acceptance Criteria

These issues from the plan MUST be resolved before the corresponding acceptance criteria can be verified in production:

| Issue | Blocks | Status | Reference |
|-------|--------|--------|-----------|
| Intermute `Reserve()` has no glob overlap detection | AC-4.2, AC-4.3, AC-4.8 (all of CUJ-4 isolation) | UNRESOLVED | `/root/projects/Intermute/internal/storage/sqlite/sqlite.go:813-839` |
| Coldwine `ReservePaths` uses exact string match | AC-4.2 local fallback | UNRESOLVED | `/root/projects/Autarch/internal/coldwine/storage/coordination.go:258` |
| No schema versioning in any database | All future schema migrations | TODO (priority P3) | `/root/projects/Autarch/docs/coldwine/todos/020-pending-p3-add-schema-versioning.md` |
| SaveRevision non-atomic writes | AC-1.15 (spec versioning correctness) | UNRESOLVED | `/root/projects/Autarch/internal/gurgeh/specs/evolution.go:41-81` |
| Signal broker has no server-side dedup persistence | AC-3.4a (signal deduplication across restarts) | UNRESOLVED | Plan notes `pkg/signals/broker.go` is pure fan-out |
| Coldwine-to-Agent Teams bridge mechanism undefined | AC-4.8, AC-4.11 (automatic reservation on task claim) | UNRESOLVED | Plan Gap 2 |
| Signal transport path ambiguity (Signals server vs Intermute) | AC-3.4 (SignalResearchInvalidation reaching Bigend) | UNRESOLVED | Plan Gap 4 |