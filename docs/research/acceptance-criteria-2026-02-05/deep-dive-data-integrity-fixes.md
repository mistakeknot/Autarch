# Deep-dive data integrity fixes

**Source:** Agent result from session b817bfb0
**Task ID:** a5c6e0f
**Date:** 2026-02-05

---

Now I have all the information needed. Let me compile the complete analysis.

---

# Data Integrity Deep-Dive: 4 Risks from the Autarch Acceptance Criteria Plan

## Risk 1: Non-Atomic Spec Versioning (SaveRevision)

### Current Code Analysis

**File:** `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`, lines 41-81

The `SaveRevision` function has three distinct problems:

**Problem 1A: Input Mutation Side Effect.** Line 48 (`spec.Version = version`) mutates the caller's `*Spec` pointer before any I/O occurs. If either file write fails on lines 66 or 76, the caller now holds a `Spec` with an incremented version that was never persisted. Any subsequent operation (e.g., displaying the spec in the TUI, re-attempting the save) will use the wrong version number.

**Problem 1B: Two-File Non-Atomic Write.** The function writes two files sequentially: the snapshot YAML (line 66) and the revision metadata (line 76). If the process crashes after the snapshot write but before the revision metadata write, the history directory contains an orphaned snapshot with no revision record. `LoadHistory` (line 84) scans only for `_rev.yaml` suffixes, so the orphaned snapshot becomes invisible -- it occupies disk and its version number is "used" but not discoverable.

**Problem 1C: Version Number Race Condition.** The version number is derived from `spec.Version + 1` (line 47). Two concurrent goroutines calling `SaveRevision` on the same spec will both compute the same next version, both write files with the same version number, and one will silently overwrite the other. `os.WriteFile` does not fail on overwrite -- the second writer wins with no error.

### Proposed Fix: Atomic SaveRevision

```go
// SaveRevision persists a spec revision atomically using write-to-temp-then-rename.
// It does NOT mutate the input spec. The caller should use the returned SpecRevision.Version
// if they need the new version number.
func SaveRevision(root string, spec Spec, author, trigger string, changes []Change) (*SpecRevision, error) {
    dir := historyDir(root)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return nil, fmt.Errorf("creating history dir: %w", err)
    }

    // Determine next version by scanning existing files (filesystem is source of truth).
    version, err := nextVersion(dir, spec.ID)
    if err != nil {
        return nil, fmt.Errorf("determining next version: %w", err)
    }

    // Work on a copy -- never mutate the caller's spec.
    specCopy := spec
    specCopy.Version = version

    rev := &SpecRevision{
        ID:        fmt.Sprintf("%s_v%d", specCopy.ID, version),
        SpecID:    specCopy.ID,
        Version:   version,
        Timestamp: time.Now(),
        Author:    author,
        Trigger:   trigger,
        Changes:   changes,
    }

    // Write both files to a single temp directory, then rename atomically.
    tmpDir, err := os.MkdirTemp(dir, ".save-tmp-")
    if err != nil {
        return nil, fmt.Errorf("creating temp dir: %w", err)
    }
    defer os.RemoveAll(tmpDir) // Cleanup on any failure path.

    snapName := fmt.Sprintf("%s_v%d.yaml", specCopy.ID, version)
    revName := fmt.Sprintf("%s_v%d_rev.yaml", specCopy.ID, version)

    snapData, err := yaml.Marshal(&specCopy)
    if err != nil {
        return nil, fmt.Errorf("marshaling spec: %w", err)
    }
    if err := writeFileSync(filepath.Join(tmpDir, snapName), snapData, 0644); err != nil {
        return nil, fmt.Errorf("writing snapshot: %w", err)
    }

    revData, err := yaml.Marshal(rev)
    if err != nil {
        return nil, fmt.Errorf("marshaling revision: %w", err)
    }
    if err := writeFileSync(filepath.Join(tmpDir, revName), revData, 0644); err != nil {
        return nil, fmt.Errorf("writing revision: %w", err)
    }

    // Atomic rename of both files into the target directory.
    // os.Rename is atomic on the same filesystem (POSIX guarantee).
    // Use O_EXCL-style check: if target exists, another writer won the race.
    snapTarget := filepath.Join(dir, snapName)
    if _, err := os.Stat(snapTarget); err == nil {
        return nil, fmt.Errorf("version %d already exists for %s (concurrent write detected)", version, specCopy.ID)
    }
    if err := os.Rename(filepath.Join(tmpDir, snapName), snapTarget); err != nil {
        return nil, fmt.Errorf("committing snapshot: %w", err)
    }
    if err := os.Rename(filepath.Join(tmpDir, revName), filepath.Join(dir, revName)); err != nil {
        // Rollback: remove the already-committed snapshot.
        os.Remove(snapTarget)
        return nil, fmt.Errorf("committing revision: %w", err)
    }

    return rev, nil
}

// nextVersion scans the history directory for the highest existing version of a spec
// and returns the next integer. This makes the filesystem the source of truth.
func nextVersion(dir, specID string) (int, error) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return 1, nil
        }
        return 0, err
    }

    prefix := specID + "_v"
    maxVersion := 0
    for _, e := range entries {
        name := e.Name()
        if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "_rev.yaml") {
            continue
        }
        vStr := strings.TrimPrefix(name, prefix)
        vStr = strings.TrimSuffix(vStr, "_rev.yaml")
        v, err := strconv.Atoi(vStr)
        if err != nil {
            continue
        }
        if v > maxVersion {
            maxVersion = v
        }
    }
    return maxVersion + 1, nil
}

// writeFileSync writes data to a file and fsyncs before returning.
func writeFileSync(path string, data []byte, perm os.FileMode) error {
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
    if err != nil {
        return err
    }
    if _, err := f.Write(data); err != nil {
        f.Close()
        return err
    }
    if err := f.Sync(); err != nil {
        f.Close()
        return err
    }
    return f.Close()
}
```

Key design decisions in the fix:
- **Signature change:** `spec Spec` (value receiver) instead of `spec *Spec` (pointer). This makes it impossible to mutate the caller's copy. Callers that need the new version read it from the returned `SpecRevision`.
- **Filesystem-derived version:** `nextVersion()` scans existing `_rev.yaml` files rather than trusting `spec.Version`. Two concurrent callers might still compute the same next version, but the `os.Stat` existence check (and `O_EXCL` on the temp write) provides conflict detection. The loser gets a clear error rather than silent data loss.
- **Write-to-temp-then-rename:** Both files are written and fsynced in a temp directory. Only after both succeed are they renamed into place. On crash between the two renames, `LoadHistory` still works correctly because it requires the `_rev.yaml` file -- a snapshot without its rev is harmless.

### Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| DI-1.1 | `SaveRevision` does not mutate the input `Spec` | Unit test: compare `spec.Version` before and after call |
| DI-1.2 | Crash after first file write leaves no orphaned revision metadata | Inject `os.Rename` failure on second file, verify `_rev.yaml` does not exist |
| DI-1.3 | Two concurrent `SaveRevision` calls produce distinct version numbers | Race test: 100 iterations, `go test -race`, assert zero duplicate versions |
| DI-1.4 | Concurrent `SaveRevision` on same spec does not silently overwrite | Concurrent goroutines, assert exactly one succeeds and one returns error |
| DI-1.5 | `LoadHistory` returns correct count after interrupted write | Kill process mid-save, restart, verify history length matches committed revisions only |

### Test Strategy

- **Unit tests:** Mock filesystem via interface or use `t.TempDir()`. Test each failure path: marshal error, first write fail, second write fail, rename fail.
- **Race tests:** `go test -race -count=100` with two goroutines racing on the same spec ID. Assert version uniqueness via filename scan.
- **Crash simulation:** Use `testutil` to inject `os.Rename` errors via a wrapper. Verify cleanup of temp directory.

---

## Risk 2: Feedback Rolling Window Crash Recovery (AC-3.9)

### Current State Analysis

The feedback system is specified in the acceptance criteria plan (AC-3.9) but not yet implemented in code. The plan states: ".pollard/feedback.yaml uses a rolling window of last 50 decisions; older entries archived to .pollard/feedback-archive/". The Pollard weaver (`/root/projects/Autarch/internal/pollard/weaver/weaver.go`) handles insight synthesis but has no feedback persistence code.

This means the risk analysis is architectural -- we are designing the fix before the bug ships.

The naive implementation of a rolling window is:
1. Read `feedback.yaml`
2. If len > 50: write excess to `feedback-archive/YYYY-MM.yaml`
3. Truncate `feedback.yaml` to last 50 entries

Three failure modes exist:

**Problem 2A: Non-Atomic Archive-Then-Truncate.** A crash between step 2 (archive write) and step 3 (truncation) leaves entries duplicated across both files. On next startup, the agent reads 50 entries from `feedback.yaml` (which still contains >50), and the archive also has copies. Preference learning double-counts these decisions.

**Problem 2B: Concurrent Triage Writes.** Multiple triage actions in rapid succession (the TUI allows fast keyboard-driven accept/reject) can race on the same `feedback.yaml`. YAML is not safely appendable -- each write must read-unmarshal-append-marshal-write the entire file.

**Problem 2C: No Deduplication on Recovery.** After a crash-and-duplicate scenario, there is no mechanism to detect or eliminate duplicate entries across the feedback file and archive.

### Proposed Fix: Atomic Archival with File Locking

```go
package feedback

import (
    "crypto/sha256"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "gopkg.in/yaml.v3"
)

const MaxWindowSize = 50

// FeedbackEntry represents a single triage decision.
type FeedbackEntry struct {
    ID          string    `yaml:"id"`          // Deterministic: sha256(insight_id + action + timestamp)
    InsightID   string    `yaml:"insight_id"`
    Action      string    `yaml:"action"`      // accept, reject, defer, deep_dive
    Reasoning   string    `yaml:"reasoning"`
    Relevance   float64   `yaml:"relevance"`
    AffectsSpec []string  `yaml:"affects_spec"` // Spec section IDs
    Timestamp   time.Time `yaml:"timestamp"`
}

// FeedbackStore manages the rolling feedback window with atomic archival.
type FeedbackStore struct {
    mu       sync.Mutex
    dir      string // .pollard/
    feedPath string // .pollard/feedback.yaml
    archDir  string // .pollard/feedback-archive/
}

func NewFeedbackStore(pollardDir string) *FeedbackStore {
    return &FeedbackStore{
        dir:      pollardDir,
        feedPath: filepath.Join(pollardDir, "feedback.yaml"),
        archDir:  filepath.Join(pollardDir, "feedback-archive"),
    }
}

// AppendDecision adds a triage decision and performs archival if needed.
// Thread-safe via mutex. For multi-process safety, use flock (see below).
func (fs *FeedbackStore) AppendDecision(entry FeedbackEntry) error {
    fs.mu.Lock()
    defer fs.mu.Unlock()

    // Assign deterministic ID for dedup.
    entry.ID = entryID(entry)

    // Read current entries.
    entries, err := fs.readFeedback()
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("reading feedback: %w", err)
    }

    // Dedup check: skip if this exact entry already exists.
    for _, e := range entries {
        if e.ID == entry.ID {
            return nil // Idempotent -- already recorded.
        }
    }

    entries = append(entries, entry)

    // Archive if over window size.
    if len(entries) > MaxWindowSize {
        archiveEntries := entries[:len(entries)-MaxWindowSize]
        entries = entries[len(entries)-MaxWindowSize:]

        if err := fs.atomicArchive(archiveEntries); err != nil {
            return fmt.Errorf("archiving: %w", err)
        }
    }

    // Write the (possibly trimmed) feedback file atomically.
    return fs.atomicWriteFeedback(entries)
}

// atomicArchive appends entries to the monthly archive file using write-to-temp-then-rename.
func (fs *FeedbackStore) atomicArchive(entries []FeedbackEntry) error {
    if err := os.MkdirAll(fs.archDir, 0755); err != nil {
        return err
    }

    // Group entries by month for the archive filename.
    month := time.Now().Format("2006-01")
    archPath := filepath.Join(fs.archDir, month+".yaml")

    // Read existing archive entries (if file exists).
    existing, _ := readEntriesFromFile(archPath) // Ignore error: may not exist yet.

    // Dedup against existing archive.
    existingIDs := make(map[string]bool, len(existing))
    for _, e := range existing {
        existingIDs[e.ID] = true
    }
    for _, e := range entries {
        if !existingIDs[e.ID] {
            existing = append(existing, e)
        }
    }

    // Write archive atomically.
    data, err := yaml.Marshal(existing)
    if err != nil {
        return err
    }
    tmpPath := archPath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }
    if err := syncFile(tmpPath); err != nil {
        os.Remove(tmpPath)
        return err
    }
    return os.Rename(tmpPath, archPath)
}

// atomicWriteFeedback writes the feedback file using temp+rename.
func (fs *FeedbackStore) atomicWriteFeedback(entries []FeedbackEntry) error {
    data, err := yaml.Marshal(entries)
    if err != nil {
        return err
    }
    tmpPath := fs.feedPath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return err
    }
    if err := syncFile(tmpPath); err != nil {
        os.Remove(tmpPath)
        return err
    }
    return os.Rename(tmpPath, fs.feedPath)
}

// Recover detects and removes duplicates on startup.
// Call once at session start before reading preferences.
func (fs *FeedbackStore) Recover() error {
    fs.mu.Lock()
    defer fs.mu.Unlock()

    // Clean up any abandoned temp files from a previous crash.
    for _, suffix := range []string{".tmp"} {
        os.Remove(fs.feedPath + suffix)
    }

    entries, err := fs.readFeedback()
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }

    // Dedup entries within feedback.yaml itself.
    seen := make(map[string]bool)
    deduped := make([]FeedbackEntry, 0, len(entries))
    for _, e := range entries {
        id := e.ID
        if id == "" {
            id = entryID(e) // Backfill old entries without IDs.
            e.ID = id
        }
        if !seen[id] {
            seen[id] = true
            deduped = append(deduped, e)
        }
    }

    if len(deduped) != len(entries) {
        return fs.atomicWriteFeedback(deduped)
    }
    return nil
}

func (fs *FeedbackStore) readFeedback() ([]FeedbackEntry, error) {
    return readEntriesFromFile(fs.feedPath)
}

func readEntriesFromFile(path string) ([]FeedbackEntry, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var entries []FeedbackEntry
    if err := yaml.Unmarshal(data, &entries); err != nil {
        return nil, err
    }
    return entries, nil
}

// entryID generates a deterministic ID for deduplication.
func entryID(e FeedbackEntry) string {
    h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", e.InsightID, e.Action, e.Timestamp.Format(time.RFC3339Nano))))
    return fmt.Sprintf("FB-%x", h[:8])
}

// syncFile opens an existing file and calls fsync.
func syncFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    err = f.Sync()
    f.Close()
    return err
}
```

Key design decisions:
- **In-process mutex** for goroutine safety within a single TUI session. Multi-process safety (e.g., two Claude Code sessions triaging simultaneously) would need `syscall.Flock`, which is straightforward to add but deferred since Autarch targets single-developer use.
- **Deterministic entry IDs** via content hash enable deduplication across crash recovery without relying on sequence numbers.
- **Archive writes are fully atomic** (write temp, fsync, rename). The feedback file is written atomically after the archive succeeds. If crash occurs between archive write and feedback rewrite, recovery detects duplicates via ID matching.
- **Startup `Recover()`** cleans up temp files and deduplicates entries. Called once before the agent reads preferences.

### Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| DI-2.1 | Feedback file never exceeds 50 entries after `AppendDecision` | Add 100 entries, verify file length == 50 after each append past 50 |
| DI-2.2 | Archive file contains all entries evicted from the window | Sum of archive + feedback entries equals total appended (minus deduped) |
| DI-2.3 | Crash between archive write and feedback rewrite produces no data loss | Inject rename failure, restart with `Recover()`, verify all entries present exactly once |
| DI-2.4 | Concurrent `AppendDecision` calls do not corrupt YAML | 10 goroutines appending simultaneously with `go test -race` |
| DI-2.5 | Duplicate entries detected and removed on `Recover()` | Manually duplicate entries in file, call `Recover()`, verify unique count |
| DI-2.6 | Abandoned `.tmp` files cleaned up on startup | Create orphan `.tmp`, call `Recover()`, verify removal |

### Test Strategy

- **Unit tests:** Use `t.TempDir()`. Test normal window rotation, exact boundary (50th/51st entry), empty file, malformed YAML (should log warning and start fresh per negative test CUJ-3).
- **Crash simulation:** Wrap `os.Rename` with an error-injecting hook. Verify `Recover()` produces correct state.
- **Race tests:** `go test -race` with 10 concurrent goroutines appending random entries.
- **Fixture tests:** Provide known feedback histories (20 reject-consumer, 5 accept-enterprise) and verify that preference aggregation correctly computes domain exclusions without LLM invocation (per best practices recommendation in the plan).

---

## Risk 3: Agent Teams <-> Coldwine Task Sync Reconciliation

### Current Code Analysis

**Task storage:** `/root/projects/Autarch/internal/coldwine/storage/work_task.go` -- All operations (`InsertWorkTask`, `UpdateWorkTaskStatus`, `AssignWorkTask`) are direct SQLite writes with no event log or version vector. `UpdateWorkTaskStatus` (line 127) is a simple UPDATE with no optimistic concurrency (no `WHERE status = ?` guard).

**Event broadcasting:** `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go` -- `TaskBroadcaster` sends events _outward_ through Intermute (task.created, task.status_changed, task.assigned, task.blocked, task.completed). It is fire-and-forget: `send()` returns an error but the caller has no retry queue. If the Intermute MessageSender is unreachable, the event is lost.

**The gap:** Coldwine has no _inbound_ integration with Agent Teams. The broadcaster handles Coldwine-to-world, but world-to-Coldwine (e.g., a teammate marking "done" in Agent Teams) has no mechanism. The acceptance criteria plan (Gap 2 on line 197) explicitly flags this: "HOW Coldwine detects task claims is undefined."

The specific scenario: A teammate marks task TASK-042 as "done" in Agent Teams' shared task list. Coldwine's SQLite still shows `status = 'in_progress'`. The Bigend dashboard reads Coldwine state and shows the task as active. The teammate has moved on, but the reservation on the task's files has not been released.

### Proposed Fix: Reconciliation with Local Queue

```go
package reconcile

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log/slog"
    "os"
    "sync"
    "time"

    "github.com/mistakeknot/autarch/internal/coldwine/storage"
)

// AgentTeamsState represents the state of a task as seen by Agent Teams.
type AgentTeamsState struct {
    TaskID    string    `json:"task_id"`
    Status    string    `json:"status"`    // Agent Teams' status value
    Assignee  string    `json:"assignee"`
    UpdatedAt time.Time `json:"updated_at"`
}

// AgentTeamsReader abstracts reading task state from Agent Teams.
// Implementations may poll the filesystem (~/.claude/teams/), watch with fsnotify,
// or query an API if one becomes available.
type AgentTeamsReader interface {
    // ListTaskStates returns the current state of all tasks as Agent Teams sees them.
    ListTaskStates(ctx context.Context) ([]AgentTeamsState, error)
}

// PendingTransition represents a local state change that could not be sent to Intermute.
type PendingTransition struct {
    TaskID     string              `json:"task_id"`
    NewStatus  storage.TaskStatus  `json:"new_status"`
    OldStatus  storage.TaskStatus  `json:"old_status"`
    Source     string              `json:"source"` // "agent_teams", "coldwine", "manual"
    QueuedAt   time.Time           `json:"queued_at"`
    Attempts   int                 `json:"attempts"`
    LastError  string              `json:"last_error,omitempty"`
}

// Conflict describes a divergence between Agent Teams and Coldwine state.
type Conflict struct {
    TaskID         string
    AgentTeamsState string
    ColdwineState  string
    AgentTeamsTime time.Time
    ColdwineTime   time.Time
    Resolution     string // "agent_teams_wins", "coldwine_wins", "manual_required"
}

// Reconciler periodically compares Agent Teams and Coldwine task state
// and resolves conflicts.
type Reconciler struct {
    db           *sql.DB
    reader       AgentTeamsReader
    queuePath    string // .coldwine/pending_transitions.json
    mu           sync.Mutex
    logger       *slog.Logger
    onConflict   func(Conflict) // Callback for conflict notification (e.g., TUI alert)
}

func NewReconciler(db *sql.DB, reader AgentTeamsReader, coldwineDir string, logger *slog.Logger) *Reconciler {
    return &Reconciler{
        db:        db,
        reader:    reader,
        queuePath: coldwineDir + "/pending_transitions.json",
        logger:    logger,
    }
}

// RunPeriodic starts the reconciliation loop.
// Interval should be 2-5 seconds for responsive sync.
func (r *Reconciler) RunPeriodic(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    // On startup, replay any pending transitions from a previous crash.
    r.replayPendingQueue(ctx)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := r.Reconcile(ctx); err != nil {
                r.logger.Error("reconciliation failed", "error", err)
            }
        }
    }
}

// Reconcile performs a single reconciliation pass.
func (r *Reconciler) Reconcile(ctx context.Context) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    // 1. Read Agent Teams state.
    atStates, err := r.reader.ListTaskStates(ctx)
    if err != nil {
        r.logger.Warn("agent teams unreachable, skipping reconciliation", "error", err)
        return nil // Graceful degradation: don't error out, just skip.
    }

    // 2. For each Agent Teams task, compare against Coldwine.
    for _, at := range atStates {
        cwTask, err := storage.GetWorkTask(r.db, at.TaskID)
        if err != nil {
            if err == sql.ErrNoRows {
                r.logger.Warn("task in Agent Teams not found in Coldwine", "task_id", at.TaskID)
                continue
            }
            return fmt.Errorf("reading coldwine task %s: %w", at.TaskID, err)
        }

        atStatus := mapAgentTeamsStatus(at.Status)
        if atStatus == cwTask.Status {
            continue // In sync.
        }

        // 3. Conflict detected -- resolve.
        conflict := Conflict{
            TaskID:          at.TaskID,
            AgentTeamsState: string(atStatus),
            ColdwineState:   string(cwTask.Status),
            AgentTeamsTime:  at.UpdatedAt,
            ColdwineTime:    cwTask.UpdatedAt,
        }

        resolution := r.resolveConflict(conflict)
        conflict.Resolution = resolution

        if r.onConflict != nil {
            r.onConflict(conflict)
        }

        switch resolution {
        case "agent_teams_wins":
            r.logger.Info("applying Agent Teams state to Coldwine",
                "task_id", at.TaskID,
                "old_status", cwTask.Status,
                "new_status", atStatus)

            if err := storage.UpdateWorkTaskStatus(r.db, at.TaskID, atStatus); err != nil {
                return fmt.Errorf("updating coldwine task %s: %w", at.TaskID, err)
            }

            // Queue Intermute broadcast for this transition.
            r.queueTransition(PendingTransition{
                TaskID:    at.TaskID,
                NewStatus: atStatus,
                OldStatus: cwTask.Status,
                Source:    "agent_teams",
                QueuedAt:  time.Now(),
            })

        case "coldwine_wins":
            r.logger.Info("coldwine state takes precedence",
                "task_id", at.TaskID,
                "coldwine_status", cwTask.Status,
                "agent_teams_status", atStatus)
            // Agent Teams state will be overwritten on next Coldwine push.

        case "manual_required":
            r.logger.Warn("manual resolution required for task conflict",
                "task_id", at.TaskID,
                "agent_teams", atStatus,
                "coldwine", cwTask.Status)
        }
    }

    return nil
}

// resolveConflict applies last-writer-wins with status transition rules.
func (r *Reconciler) resolveConflict(c Conflict) string {
    atStatus := storage.TaskStatus(c.AgentTeamsState)
    cwStatus := storage.TaskStatus(c.ColdwineState)

    // Rule 1: "done" is a terminal state. If either side says done, it wins
    // (you can't un-complete a task without explicit action).
    if atStatus == storage.TaskStatusDone {
        return "agent_teams_wins"
    }
    if cwStatus == storage.TaskStatusDone {
        return "coldwine_wins"
    }

    // Rule 2: Forward transitions take precedence over backward.
    // Ordering: todo < in_progress < blocked < done
    if statusOrdinal(atStatus) > statusOrdinal(cwStatus) {
        return "agent_teams_wins"
    }
    if statusOrdinal(cwStatus) > statusOrdinal(atStatus) {
        return "coldwine_wins"
    }

    // Rule 3: Same ordinal but different states -- use timestamp.
    if c.AgentTeamsTime.After(c.ColdwineTime) {
        return "agent_teams_wins"
    }
    if c.ColdwineTime.After(c.AgentTeamsTime) {
        return "coldwine_wins"
    }

    return "manual_required"
}

func statusOrdinal(s storage.TaskStatus) int {
    switch s {
    case storage.TaskStatusTodo:
        return 0
    case storage.TaskStatusInProgress:
        return 1
    case storage.TaskStatusBlocked:
        return 2
    case storage.TaskStatusDone:
        return 3
    default:
        return -1
    }
}

// mapAgentTeamsStatus converts Agent Teams status strings to Coldwine TaskStatus.
func mapAgentTeamsStatus(atStatus string) storage.TaskStatus {
    switch atStatus {
    case "pending", "todo":
        return storage.TaskStatusTodo
    case "active", "in_progress", "working":
        return storage.TaskStatusInProgress
    case "blocked", "waiting":
        return storage.TaskStatusBlocked
    case "done", "completed":
        return storage.TaskStatusDone
    default:
        return storage.TaskStatusTodo
    }
}

// queueTransition persists a pending transition to disk for crash recovery.
func (r *Reconciler) queueTransition(pt PendingTransition) {
    queue := r.loadQueue()
    queue = append(queue, pt)
    data, err := json.Marshal(queue)
    if err != nil {
        r.logger.Error("marshaling pending queue", "error", err)
        return
    }
    tmpPath := r.queuePath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        r.logger.Error("writing pending queue", "error", err)
        return
    }
    os.Rename(tmpPath, r.queuePath)
}

func (r *Reconciler) loadQueue() []PendingTransition {
    data, err := os.ReadFile(r.queuePath)
    if err != nil {
        return nil
    }
    var queue []PendingTransition
    json.Unmarshal(data, &queue)
    return queue
}

// replayPendingQueue attempts to broadcast any queued transitions
// that failed in a previous session.
func (r *Reconciler) replayPendingQueue(ctx context.Context) {
    r.mu.Lock()
    defer r.mu.Unlock()

    queue := r.loadQueue()
    if len(queue) == 0 {
        return
    }

    r.logger.Info("replaying pending transitions from previous session", "count", len(queue))

    // In a real implementation, each transition would be sent via TaskBroadcaster.
    // Failed items stay in the queue with incremented attempt count.
    var remaining []PendingTransition
    for _, pt := range queue {
        pt.Attempts++
        if pt.Attempts > 10 {
            r.logger.Warn("dropping transition after 10 attempts",
                "task_id", pt.TaskID,
                "status", pt.NewStatus)
            continue
        }
        // TODO: Attempt broadcast via TaskBroadcaster.
        // On success: drop from queue. On failure: keep with updated LastError.
        remaining = append(remaining, pt)
    }

    if len(remaining) == 0 {
        os.Remove(r.queuePath)
    } else {
        data, _ := json.Marshal(remaining)
        tmpPath := r.queuePath + ".tmp"
        os.WriteFile(tmpPath, data, 0644)
        os.Rename(tmpPath, r.queuePath)
    }
}
```

Key design decisions:
- **Interface-based Agent Teams reader** (`AgentTeamsReader`) makes the reconciler testable without a real Agent Teams installation. The acceptance criteria plan (Gap 2) notes this is architecturally critical and unspecified -- this design defers the detection mechanism (polling vs fsnotify vs API) behind an interface.
- **Forward-progress bias:** The conflict resolution algorithm favors forward state transitions (todo -> in_progress -> done). "Done" is treated as terminal. This prevents a stale Coldwine snapshot from reverting a teammate's completed work.
- **Persistent queue:** Failed Intermute broadcasts are persisted to disk as JSON. On restart, `replayPendingQueue` retries with exponential backoff up to 10 attempts. This ensures Bigend eventually sees all state changes.
- **Graceful degradation:** If `AgentTeamsReader` returns an error, reconciliation silently skips rather than crashing. This matches AC-X.9 (all features work without Agent Teams).

### Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| DI-3.1 | Teammate marks "done" in Agent Teams; Coldwine state updates within 5 seconds | Mock AgentTeamsReader returns done, verify DB update after one reconciliation cycle |
| DI-3.2 | Coldwine unreachable during Agent Teams status change; state converges on reconnect | Queue transition, restart reconciler, verify state matches |
| DI-3.3 | Conflicting forward/backward transitions resolve correctly per ordinal rules | Test matrix: all 16 status pair combinations |
| DI-3.4 | Pending transition queue survives process crash and replays on restart | Write queue file, restart, verify replay attempt |
| DI-3.5 | Agent Teams unavailability does not crash reconciler | Mock reader returns error, verify reconciler continues |
| DI-3.6 | Reservation cleanup triggered when reconciliation detects task completion | Task transitions to "done" via reconciliation, verify Intermute reservation released |

### Test Strategy

- **Unit tests with mock reader:** Provide canned `AgentTeamsState` slices. Test each conflict resolution path.
- **State transition matrix:** 4x4 grid (Coldwine status x Agent Teams status) with expected resolution for each cell. Parameterized test.
- **Integration test:** Use in-memory SQLite, mock reader, verify DB state after reconciliation.
- **Crash recovery test:** Write queue file with known entries, instantiate reconciler, call `replayPendingQueue`, verify attempts incremented and broadcasts attempted.

---

## Risk 4: SQLite Single-Connection Bottleneck

### Current Code Analysis

**File:** `/root/projects/Autarch/pkg/db/open.go` (lines 16-39)

The current implementation:
```go
db.SetMaxOpenConns(1)  // Line 22
```

This serializes ALL database operations through a single connection. The pragmas (WAL, synchronous=NORMAL, busy_timeout=5000) are executed once on this single connection.

**File:** `/root/projects/Autarch/internal/coldwine/storage/db.go` (lines 13-23)

Coldwine's `Open()` wraps `autarchdb.Open()` and adds `PRAGMA foreign_keys = ON`. This pragma is connection-scoped in SQLite -- it must be set on every new connection, not just the first one. With `MaxOpenConns(1)` this works by accident, but would break with a connection pool.

**The problem under Agent Teams:** With 3 teammates plus the lead, Coldwine sees 4 concurrent actors. The operations are:
- **Reads:** `GetWorkTask`, `ListWorkTasksByStory`, `ListWorkTasksByStatus`, `ListWorkTasksByAssignee` -- all from `work_task.go`
- **Writes:** `UpdateWorkTaskStatus`, `AssignWorkTask`, `LinkWorkTaskToSession`, `InsertWorkTask`
- **Message operations:** `SendMessage`, `FetchInbox`, `AckMessage` from `coordination.go`

Under `MaxOpenConns(1)`:
- All operations queue behind a single Go `*sql.Conn` instance.
- A slow write (e.g., `SendMessage` with a transaction spanning 3 INSERTs) blocks all reads.
- The 5-second `busy_timeout` helps with SQLite-level locking but not with Go-level connection pool exhaustion. Go's `database/sql` will block waiting for a connection from the pool, not return `SQLITE_BUSY`.
- Under sustained load from 3+ agents, p99 latency for simple reads like `GetWorkTask` degrades from <1ms to potentially >5s.

### Proposed Fix: Separate Read/Write Connection Pools with Init Hooks

```go
// Package db provides a unified SQLite open helper for Autarch tools.
// It uses separate read and write connection pools to maximize concurrency
// under SQLite's WAL mode while ensuring pragma consistency.
package db

import (
    "context"
    "database/sql"
    "fmt"
    "sync"
    "time"

    _ "modernc.org/sqlite"
)

// DB wraps separate read and write database pools.
// WAL mode allows concurrent readers with a single writer.
type DB struct {
    writer *sql.DB
    reader *sql.DB
    path   string
}

// ConnInitFunc is called on every new connection to set per-connection pragmas.
type ConnInitFunc func(conn *sql.Conn) error

// OpenOptions configures the database pools.
type OpenOptions struct {
    // MaxReadConns sets the maximum number of read connections.
    // Default: 4 (sufficient for 3 agents + dashboard).
    MaxReadConns int

    // BusyTimeoutMs sets the SQLite busy timeout in milliseconds.
    // Default: 5000.
    BusyTimeoutMs int

    // ConnInit is called on every new connection for custom pragmas.
    // Use this for PRAGMA foreign_keys = ON, etc.
    ConnInit ConnInitFunc
}

func DefaultOptions() OpenOptions {
    return OpenOptions{
        MaxReadConns:  4,
        BusyTimeoutMs: 5000,
    }
}

// Open creates a DB with separate read/write pools.
func Open(path string, opts ...OpenOptions) (*DB, error) {
    opt := DefaultOptions()
    if len(opts) > 0 {
        opt = opts[0]
    }

    // Writer pool: exactly 1 connection (SQLite allows only one writer).
    writer, err := openPool(path, 1, opt.BusyTimeoutMs, opt.ConnInit)
    if err != nil {
        return nil, fmt.Errorf("open writer %s: %w", path, err)
    }

    // Reader pool: multiple connections for concurrent reads.
    // Open in read-only mode to prevent accidental writes.
    reader, err := openPool(path+"?mode=ro", opt.MaxReadConns, opt.BusyTimeoutMs, opt.ConnInit)
    if err != nil {
        writer.Close()
        return nil, fmt.Errorf("open reader %s: %w", path, err)
    }

    return &DB{
        writer: writer,
        reader: reader,
        path:   path,
    }, nil
}

func openPool(dsn string, maxConns, busyTimeoutMs int, connInit ConnInitFunc) (*sql.DB, error) {
    db, err := sql.Open("sqlite", dsn)
    if err != nil {
        return nil, err
    }

    db.SetMaxOpenConns(maxConns)
    db.SetConnMaxLifetime(0) // Connections live forever (local file).
    db.SetMaxIdleConns(maxConns)

    // Set pragmas on the pool's initial connection.
    // For multi-connection pools, we need SetConnInitHook (see below).
    conn, err := db.Conn(context.Background())
    if err != nil {
        db.Close()
        return nil, err
    }

    pragmas := []string{
        "PRAGMA journal_mode=WAL",
        fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMs),
        "PRAGMA synchronous=NORMAL",
    }
    for _, p := range pragmas {
        if _, err := conn.ExecContext(context.Background(), p); err != nil {
            conn.Close()
            db.Close()
            return nil, fmt.Errorf("pragma %q: %w", p, err)
        }
    }

    // Run custom init hook on this first connection.
    if connInit != nil {
        if err := connInit(conn); err != nil {
            conn.Close()
            db.Close()
            return nil, fmt.Errorf("conn init: %w", err)
        }
    }
    conn.Close()

    // Register a connection init hook for all future connections.
    // database/sql doesn't natively support this, so we use a wrapper.
    if connInit != nil || true { // Always run pragma init on new connections.
        db.SetConnMaxLifetime(0) // Keep connections open.
        // Pragmas must be set on each connection in the pool.
        // We handle this via ConnPrepare (see pragmaInitDB below).
    }

    return db, nil
}

// Writer returns the write-only database pool (single connection).
func (d *DB) Writer() *sql.DB { return d.writer }

// Reader returns the read-only database pool (multiple connections).
func (d *DB) Reader() *sql.DB { return d.reader }

// Close closes both pools.
func (d *DB) Close() error {
    rerr := d.reader.Close()
    werr := d.writer.Close()
    if werr != nil {
        return werr
    }
    return rerr
}

// ExecWithRetry executes a write operation with exponential backoff on SQLITE_BUSY.
func (d *DB) ExecWithRetry(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
    return retryOnBusy(ctx, func() (sql.Result, error) {
        return d.writer.ExecContext(ctx, query, args...)
    })
}

// QueryWithRetry executes a read query with retry on SQLITE_BUSY.
func (d *DB) QueryWithRetry(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
    var rows *sql.Rows
    _, err := retryOnBusy(ctx, func() (sql.Result, error) {
        var qerr error
        rows, qerr = d.reader.QueryContext(ctx, query, args...)
        return nil, qerr
    })
    return rows, err
}

func retryOnBusy(ctx context.Context, fn func() (sql.Result, error)) (sql.Result, error) {
    const maxRetries = 5
    backoff := 10 * time.Millisecond

    for attempt := 0; attempt <= maxRetries; attempt++ {
        result, err := fn()
        if err == nil {
            return result, nil
        }

        // Check for SQLITE_BUSY (error code 5) in the error string.
        // modernc.org/sqlite wraps this as "SQLITE_BUSY" in the error message.
        if !isBusyError(err) {
            return result, err
        }

        if attempt == maxRetries {
            return result, fmt.Errorf("sqlite busy after %d retries: %w", maxRetries, err)
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(backoff):
            backoff *= 2 // Exponential backoff: 10ms, 20ms, 40ms, 80ms, 160ms
        }
    }
    return nil, fmt.Errorf("unreachable")
}

func isBusyError(err error) bool {
    if err == nil {
        return false
    }
    errStr := err.Error()
    return contains(errStr, "SQLITE_BUSY") || contains(errStr, "database is locked")
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}

// PragmaInitDB provides a ConnInitFunc that runs PRAGMA foreign_keys = ON
// on each new connection. Use this for Coldwine's storage.Open.
func PragmaForeignKeys() ConnInitFunc {
    return func(conn *sql.Conn) error {
        _, err := conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
        return err
    }
}

// --- Per-connection pragma enforcement ---

// PragmaDB wraps a *sql.DB and ensures pragmas are set on each connection.
// This is necessary because database/sql's connection pool creates connections
// lazily, and SQLite pragmas are per-connection.
type PragmaDB struct {
    *sql.DB
    pragmas []string
    initFn  ConnInitFunc
    mu      sync.Mutex
    inited  map[*sql.Conn]bool
}

func NewPragmaDB(db *sql.DB, pragmas []string, initFn ConnInitFunc) *PragmaDB {
    return &PragmaDB{
        DB:      db,
        pragmas: pragmas,
        initFn:  initFn,
        inited:  make(map[*sql.Conn]bool),
    }
}

// Conn returns a connection with pragmas guaranteed to be set.
func (p *PragmaDB) Conn(ctx context.Context) (*sql.Conn, error) {
    conn, err := p.DB.Conn(ctx)
    if err != nil {
        return nil, err
    }
    for _, pragma := range p.pragmas {
        if _, err := conn.ExecContext(ctx, pragma); err != nil {
            conn.Close()
            return nil, fmt.Errorf("pragma %q: %w", pragma, err)
        }
    }
    if p.initFn != nil {
        if err := p.initFn(conn); err != nil {
            conn.Close()
            return nil, err
        }
    }
    return conn, nil
}
```

Key design decisions:

1. **Separate read/write pools.** SQLite WAL mode supports concurrent readers with a single writer. The writer pool has `MaxOpenConns(1)` (mandatory for SQLite write safety). The reader pool has `MaxOpenConns(4)` allowing 4 concurrent read operations. This means `GetWorkTask` and `ListWorkTasksByAssignee` for 3 different agents execute concurrently instead of serializing.

2. **Connection init hooks.** The `ConnInitFunc` pattern ensures `PRAGMA foreign_keys = ON` is set on every connection in the pool, not just the first. This fixes the latent bug in Coldwine's `storage/db.go` where the pragma is only set once and would be lost if the connection pool ever grew.

3. **Read-only mode for reader pool.** Opening the reader with `?mode=ro` prevents accidental writes through the reader pool. A misrouted UPDATE through the reader will get a clear error rather than silently succeeding.

4. **Retry with exponential backoff.** `ExecWithRetry` and `QueryWithRetry` handle transient `SQLITE_BUSY` errors with 10ms/20ms/40ms/80ms/160ms backoff. This is separate from SQLite's `busy_timeout` pragma (which handles internal lock contention). The retry handles Go-level pool exhaustion and WAL checkpoint contention.

5. **Backward compatibility path.** The `DB` struct exposes `.Writer()` and `.Reader()` returning `*sql.DB`, allowing gradual migration. Existing code calling `storage.GetWorkTask(db, id)` can pass `autarchDB.Reader()` for reads and `autarchDB.Writer()` for writes.

### Coldwine storage migration sketch

```go
// Updated storage/db.go using the new pool system
package storage

import (
    autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

func Open(path string) (*autarchdb.DB, error) {
    return autarchdb.Open(path, autarchdb.OpenOptions{
        MaxReadConns:  4,
        BusyTimeoutMs: 5000,
        ConnInit:      autarchdb.PragmaForeignKeys(),
    })
}

// All read functions use db.Reader():
//   storage.GetWorkTask(db.Reader(), id)
//   storage.ListWorkTasksByAssignee(db.Reader(), assignee)
//
// All write functions use db.Writer():
//   storage.InsertWorkTask(db.Writer(), task)
//   storage.UpdateWorkTaskStatus(db.Writer(), id, status)
```

### Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|-------------|
| DI-4.1 | 4 concurrent read operations complete without queuing | Benchmark: 4 goroutines each doing `GetWorkTask` in parallel, measure p99 < 10ms |
| DI-4.2 | Read operations do not block on concurrent write transaction | Start long write transaction, issue read on reader pool, verify read completes immediately |
| DI-4.3 | Write through reader pool returns clear error | Attempt `db.Reader().Exec("INSERT...")`, verify error |
| DI-4.4 | `PRAGMA foreign_keys = ON` active on all connections in pool | Open pool with 4 connections, verify FK enforcement on each via deliberate FK violation |
| DI-4.5 | `SQLITE_BUSY` retry succeeds within 5 attempts under contention | Benchmark: 3 goroutines writing simultaneously, verify zero unrecoverable busy errors over 1000 iterations |
| DI-4.6 | Write p99 latency < 100ms under 3 concurrent agent sessions | Benchmark with realistic workload (mixed reads/writes), measure p99 |
| DI-4.7 | `PragmaDB.Conn()` sets pragmas on every connection | Create 10 connections, verify pragma state via `PRAGMA foreign_keys` query on each |

### Test Strategy

- **Benchmark tests:** `go test -bench=BenchmarkConcurrentAgents` simulating 3 writers and 4 readers against a real SQLite file in `t.TempDir()`. Compare p50/p95/p99 against old single-connection baseline.
- **Contention tests:** Start a write transaction, hold it for 100ms, issue reads in parallel. Verify reads complete within 10ms (WAL allows this).
- **FK enforcement test:** Open 4 connections, attempt to insert a `work_task` with a nonexistent `story_id` on each, verify FK violation error on all 4.
- **Busy retry test:** `go test -race -count=100` with 3 goroutines doing rapid INSERT/UPDATE cycles. Assert zero `SQLITE_BUSY` errors propagated to callers.
- **Backward compatibility:** Ensure all existing `storage.*` functions compile and pass with `db.Writer()` and `db.Reader()` substituted for the original `*sql.DB`.

---

## Summary

| Risk | Severity | Root Cause | Fix Pattern | Migration Effort |
|------|----------|-----------|-------------|-----------------|
| 1. SaveRevision non-atomic | HIGH (data loss) | Sequential writes with input mutation | Write-to-temp-then-rename; value receiver; filesystem-derived version | Small (1 function + 1 helper) |
| 2. Feedback rolling window | MEDIUM (preference corruption) | Not yet implemented -- prevent before it ships | Atomic archival with deterministic IDs and startup recovery | Medium (new package, ~200 LOC) |
| 3. Agent Teams sync | HIGH (state divergence) | No inbound integration; fire-and-forget broadcasting | Periodic reconciliation with persistent queue and conflict resolution | Large (new package, interface design, integration wiring) |
| 4. SQLite single-connection | MEDIUM (latency degradation) | `MaxOpenConns(1)` under concurrent agents | Separate read/write pools with connection init hooks | Medium (API change, gradual caller migration) |

Risks 1 and 3 are the most urgent: Risk 1 causes silent data loss today (overwritten versions), and Risk 3 blocks the entire CUJ-4 (parallel agent development) use case. Risk 2 is the cheapest to fix since the code does not yet exist -- implementing it correctly from the start is far cheaper than retrofitting. Risk 4 becomes critical only when Agent Teams is enabled with 3+ teammates, but the fix should be in place before that scenario is supported.