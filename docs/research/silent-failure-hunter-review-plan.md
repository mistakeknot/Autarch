# Silent Failure Hunter: Acceptance Criteria Plan Review

**Audited:** 2026-02-06
**Scope:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md` + supporting codebase
**Focus:** Five failure scenarios specified by reviewer, plus emergent issues discovered during audit

---

## Executive Summary

The acceptance criteria plan is thorough in its positive-path coverage but has **systemic gaps in error observability**. The plan's "Negative/Failure Path Testing" section (lines 366-371) is dangerously thin -- five bullet points for five CUJs, each describing a single failure mode when dozens exist. More critically, the **existing codebase already contains multiple silent failure patterns** that the plan neither acknowledges nor proposes to fix. The confidence score, which is the primary quality signal to users, is structurally incapable of reflecting hunter failures or Intermute outages.

**Critical findings: 14 | High findings: 9 | Medium findings: 6**

---

## Finding 1: Hunter Failures Do Not Reduce Confidence Score

**Severity:** CRITICAL
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 856-908 (`researchQuality`)
**Plan Reference:** AC-1.8, AC-1.9, CUJ-1 negative path (line 367)

### The Problem

The `researchQuality()` function computes research confidence from three factors: finding count (30%), source diversity (30%), and average relevance (40%). When a hunter fails, the coordinator sends a `HunterErrorMsg` to the TUI (line 172 of `coordinator.go`) but **the error state of individual hunters is never reflected in the confidence calculation**.

Consider this scenario:
- 3 hunters are configured: github-scout, arxiv-scout, hackernews-trendwatcher
- github-scout returns 5 findings (relevance 0.7 avg)
- arxiv-scout FAILS with a rate limit error
- hackernews-trendwatcher FAILS with a network timeout

The confidence score calculates:
- countScore = min(5/10, 1.0) = 0.5
- diversityScore = min(1/3, 1.0) = 0.33
- avgRelevance = 0.7
- Research = 0.3*0.5 + 0.3*0.33 + 0.4*0.7 = 0.53

The user sees 53% research confidence. This looks reasonable. But 2 of 3 hunters failed entirely. The user has **no indication** that the confidence score represents only 33% of the intended research coverage. The plan says "confidence adjusts" on hunter failure (line 367) but the code has no mechanism for this.

### Hidden Errors

The `HuntResult.Errors` field (line 93 of `hunter.go`) accumulates per-query errors within a hunter, but the `HuntResult.Success()` method checks `len(r.Errors) == 0`. A hunter that succeeds on 1 of 5 queries reports `Success() == false` yet still returns partial results. These partial results flow into the confidence calculation as if they were complete.

### User Impact

Users make high-stakes decisions based on confidence scores -- "Is my PRD ready for export?" A 53% research score when 67% of hunters failed is actively misleading. Users will export PRDs believing they have adequate research coverage when entire research domains (academic papers, trending discussions) were never successfully queried.

### Plan Gap

AC-1.8 says "Confidence score displays four components (Completeness, Consistency, Specificity, Research) and updates within 2 seconds of triage action." It does not require that the Research component reflect hunter failure rate. AC-1.9 mentions "unreviewed high-relevance findings reduce score" but says nothing about failed hunters.

### Recommendation

Add acceptance criteria:
- **AC-1.18**: When a hunter fails, Research confidence incorporates the failure: `adjustedResearch = rawResearch * (successfulHunters / totalHunters)`. Hunter failure status visible in confidence tooltip.
- **AC-1.19**: Confidence score tooltip or detail view shows per-hunter status (success/failed/rate-limited) with error messages.
- **AC-1.20**: A hunter that returns partial results (some queries succeeded, some failed) reports both success count and failure count; confidence reflects partial coverage.

---

## Finding 2: Signal Broker Silently Drops Signals on Slow Subscribers

**Severity:** CRITICAL
**Location:** `/root/projects/Autarch/pkg/signals/broker.go` lines 51-55
**Plan Reference:** AC-3.4, AC-3.4a, AC-5.3

### The Problem

```go
select {
case sub.ch <- sig:
default:
    // Drop if subscriber is slow.
}
```

This is a textbook silent failure. When a subscriber's channel buffer (64 signals) fills, new signals are permanently dropped with zero logging, zero metrics, zero user feedback. The plan already identified this (line 205: "broker.go line 51-54 silently drops signals when subscriber buffers fill") but **proposes no acceptance criterion to prevent or detect it**.

### Hidden Errors

This catch-all `default` branch can swallow:
- `SignalResearchInvalidation` (critical severity) -- research contradicting PRD assumptions
- `SignalCompetitorShipped` (warning severity) -- competitor releases
- `SignalAssumptionDecayed` (warning severity) -- assumptions losing validity
- `SignalSpecHealthLow` (critical severity) -- spec quality degradation

Every signal type defined in the system can be silently dropped here. A `SignalResearchInvalidation` is **critical severity** by definition (line 44 of `internal/pollard/signals/emitter.go`), yet it can vanish without a trace.

### User Impact

The Bigend dashboard (AC-5.3) relies on WebSocket signal delivery for real-time updates. If signals are dropped, the dashboard shows stale data. Worse, CUJ-3 depends on `SignalResearchInvalidation` to alert users when research contradicts PRD assumptions. If this signal is dropped, the user continues building on invalidated assumptions.

### Recommendation

Add acceptance criteria:
- **AC-3.4e**: Signal broker logs a warning when a signal is dropped due to subscriber backpressure, including signal type, subscriber identity, and buffer fullness.
- **AC-3.4f**: Dropped signal count is exposed as a metric visible in the Signals tab or Bigend dashboard.
- **AC-3.4g**: Critical-severity signals (research_invalidation, spec_health_low) are NEVER dropped -- they use a separate unbuffered delivery path with blocking semantics and a configurable timeout.

---

## Finding 3: Intermute Unreachable During Reservation -- Silent Degradation Hides Unsafe State

**Severity:** CRITICAL
**Location:** `/root/projects/Autarch/internal/gurgeh/intermute/sync.go` lines 39-42
**Plan Reference:** AC-X.5, AC-4.1, AC-4.2, Degradation Matrix (line 428)

### The Problem

When the Intermute client is nil (Intermute unreachable), `PRDSyncer.SyncPRD()` returns an empty `Spec{}` and nil error:

```go
func (s *PRDSyncer) SyncPRD(ctx context.Context, prd *specs.PRD) (intermute.Spec, error) {
    if s.client == nil {
        return intermute.Spec{}, nil
    }
    // ...
}
```

The same pattern appears in:
- `internal/pollard/intermute/publisher.go` line 42-44: `PublishFinding` returns empty Insight, nil error
- `internal/pollard/intermute/publisher.go` line 53-55: `PublishFindings` returns nil, nil
- `internal/pollard/research/coordinator.go` line 253: `publishToIntermute` silently returns when client is nil

These are all "graceful degradation" by design (comments say "no-op when nil"). But the degradation is **invisible to the user**. When Intermute is down:

1. File reservations silently become no-ops
2. Research findings are never published to the cross-tool visibility layer
3. Spec sync never happens
4. No signal broadcasts occur

The plan's AC-X.5 says tools should display "Intermute unavailable" and continue. But the current code returns success (nil error) when Intermute is nil, so the TUI layer has no way to know it should display a warning.

### Degradation Matrix Gap

The plan's degradation matrix (line 428) describes "Agent Teams ON + Intermute OFF" as: "Task claims work but no file reservation enforcement. Teammates proceed with 'unprotected' warning." But the current code doesn't generate this "unprotected" warning because the nil-client path returns success.

### Recommendation

Add acceptance criteria:
- **AC-X.5a**: When Intermute client is nil, all operations return a sentinel error (e.g., `ErrOffline` which already exists at `pkg/intermute/client.go` line 22) distinguishable from operational errors. Callers can decide to log-and-continue, but the decision is explicit.
- **AC-X.5b**: TUI displays a persistent "Intermute offline -- reservations disabled" indicator when Intermute is unreachable. This indicator is visible in all tabs, not just when a reservation is attempted.
- **AC-X.5c**: When a reservation would have been acquired but Intermute is offline, the log pane shows "WARNING: File reservation skipped (Intermute offline) -- concurrent edits not protected for [paths]".
- **AC-X.5d**: Confidence score includes an "Integration" component that degrades when Intermute is offline, warning users that cross-tool coordination is disabled.

---

## Finding 4: WebSocket Disconnect During Active Sprint -- No Reconnection or State Reconciliation

**Severity:** CRITICAL
**Location:** `/root/projects/Autarch/pkg/signals/broker.go` lines 60-81 (`ServeWS`)
**Plan Reference:** AC-5.3, AC-5.6, AC-5.7

### The Problem

The WebSocket handler in `ServeWS` has this error handling:

```go
case sig := <-sub.sub.ch:
    if err := wsjson.Write(ctx, conn, sig); err != nil {
        return  // Silent return on write error
    }
```

When a WebSocket write fails (client disconnected, network interruption), the handler silently returns. No logging. No reconnection attempt. No state reconciliation.

The WebSocket accept also swallows errors:

```go
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
if err != nil {
    return  // No logging, no error response
}
```

### User Impact for CUJ-5

The Bigend dashboard (CUJ-5) depends on WebSocket for real-time updates. When a WebSocket disconnects:
1. The dashboard freezes showing the last known state
2. The user sees no indication that updates have stopped
3. Any signals emitted during the disconnect window are permanently lost (they fill the subscriber buffer and get dropped per Finding 2)
4. Stale state detection (AC-5.6) cannot work if the dashboard itself is stale

### Active Sprint Impact

During an active Arbiter sprint, the research coordinator sends `HunterStartedMsg`, `HunterUpdateMsg`, `HunterCompletedMsg`, `HunterErrorMsg` as Bubble Tea messages. These are in-process and not affected by WebSocket. However, if the user is monitoring the sprint from the Bigend dashboard (a common CUJ-5 scenario), all of these updates are delivered via WebSocket. A disconnect during the critical "first findings arriving" window (AC-1.2: within 60 seconds) means the user sees no research activity at all.

### Recommendation

Add acceptance criteria:
- **AC-5.3a**: WebSocket handler logs connection and disconnection events with client identifier and duration.
- **AC-5.3b**: Client-side WebSocket implementation includes automatic reconnection with exponential backoff (1s, 2s, 4s, max 30s).
- **AC-5.3c**: On reconnection, client requests full state snapshot to reconcile any signals missed during disconnect.
- **AC-5.3d**: Dashboard displays "Connection lost -- reconnecting..." indicator when WebSocket is disconnected.
- **AC-5.3e**: Signals emitted during a client disconnect are buffered server-side (up to configurable limit) and replayed on reconnection.

---

## Finding 5: feedback.yaml Corruption -- Inadequate Crash Recovery

**Severity:** HIGH
**Location:** Plan line 369, Plan line 248, Plan line 76-78
**Plan Reference:** AC-3.6, AC-3.9

### The Problem

The plan identifies this risk (line 248): "Feedback rolling window (AC-3.9) lacks crash recovery. Archive-then-truncate is not atomic. Process crash between archive write and truncation causes duplicate entries."

But the acceptance criteria for this are weak:
- AC-3.9 says "Add 60 decisions, verify window size and archive presence" -- this only tests the happy path
- The negative test (line 369) says "Feedback.yaml malformed -- agent logs warning, starts with empty preferences, doesn't overwrite" -- but this only covers READ of corrupted data, not WRITE corruption

### Missing Failure Modes

1. **Write corruption during triage**: If the process crashes while appending to `feedback.yaml`, the file may contain a partial YAML entry. The next read will fail to parse the entire file, losing ALL previous decisions (not just the corrupted one).

2. **Concurrent write from TUI and agent**: AC-3.6 says the agent reads `feedback.yaml` on session start. AC-1.6 says triage actions append to `feedback.yaml`. If a TUI triage and an agent triage happen simultaneously, the file can be corrupted.

3. **Rolling window archive race**: The archive operation reads the full file, writes the archive, then truncates the original. A crash between write and truncate duplicates entries. A crash during truncate loses entries.

4. **YAML bomb in feedback**: The plan's security finding F5 (line 237) mentions YAML bomb attacks via recursive anchors. The acceptance criteria have no test for this.

### The SaveSprintState Comparison

The plan already uses `AtomicWriteFile` (in `internal/file/atomic.go`) for spec revision snapshots. This is good. But `SaveSprintState` in `internal/gurgeh/arbiter/persistence.go` line 33 uses `os.WriteFile` directly -- NOT atomic. A crash during sprint state save corrupts the sprint file.

```go
if err := os.WriteFile(path, data, 0644); err != nil {
    return fmt.Errorf("write state: %w", err)
}
```

Similarly, `saveSnapshot` in `internal/pollard/watch/watcher.go` line 213 uses `os.WriteFile` -- not atomic.

### Recommendation

Add acceptance criteria:
- **AC-3.9a**: feedback.yaml writes use atomic write-to-temp-then-rename (same as spec revision snapshots).
- **AC-3.9b**: Corrupted feedback.yaml is detected on read; the last valid entry is identified and the file is repaired. The corrupted portion is preserved in a `.corrupted` backup file for debugging.
- **AC-3.9c**: Rolling window archive operation is atomic: archive is written, then original is atomically replaced with the truncated version in a single rename. No crash window exists where data can be lost or duplicated.
- **AC-3.9d**: Concurrent triage operations use file locking (already available via `internal/file/lock_unix.go`) to prevent interleaved writes.
- **AC-PERSISTENCE.1**: All file writes to persistent state (sprint state, feedback, watch snapshots, config) use `AtomicWriteFile` or equivalent. Audit all uses of `os.WriteFile` in production code paths.

---

## Finding 6: Orchestrator Swallows Research Publish Errors with `_, _`

**Severity:** CRITICAL
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 280, 1030, 1083

### The Problem

```go
// Line 280 - StartWithResearch
for _, f := range pollardFindings {
    _, _ = o.research.PublishInsight(ctx, specID, f)
}

// Line 1030 - runQuickScanSync
_, _ = o.research.PublishInsight(ctx, state.SpecID, ResearchFinding{...})

// Line 1083 - runQuickScanBackground
_, _ = o.research.PublishInsight(ctx, specID, ResearchFinding{...})
```

The `_, _` pattern is the Go equivalent of an empty catch block. Every error from `PublishInsight` -- network failures, authentication errors, malformed data, Intermute server errors -- is silently discarded. There is no logging, no metric, no user feedback.

This is particularly dangerous at line 280 where ALL findings in a loop have their publish errors discarded. If Intermute is returning 500 errors, every single finding fails silently. The user sees findings in the TUI (they're stored locally) but they never reach Intermute for cross-tool visibility. The Bigend dashboard shows no research activity. Other tools see no insights. The user has no way to know.

### Recommendation

- **AC-RESEARCH.1**: All `PublishInsight` calls log errors at warning level with the finding title and error message.
- **AC-RESEARCH.2**: If >50% of publish attempts fail in a single run, a summary warning is displayed in the log pane: "N of M research findings failed to publish to Intermute."
- **AC-RESEARCH.3**: No production code uses the `_, _` pattern to discard both return value and error from any function that can fail. All such instances must be replaced with explicit error handling (log, metric, or propagation).

---

## Finding 7: Intermute Publisher Swallows Individual Publish Errors

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/pollard/intermute/publisher.go` lines 52-67

### The Problem

```go
func (p *Publisher) PublishFindings(ctx context.Context, findings []research.Finding) ([]intermute.Insight, error) {
    if p.client == nil {
        return nil, nil
    }
    var results []intermute.Insight
    for _, finding := range findings {
        insight, err := p.PublishFinding(ctx, finding)
        if err != nil {
            // Log but continue with other findings
            continue
        }
        results = append(results, insight)
    }
    return results, nil
}
```

The comment says "Log but continue" but there is NO logging. The `continue` silently skips failed findings. The function returns a nil error even if ALL findings failed to publish. The caller receives an empty slice and a nil error -- indistinguishable from "no findings to publish."

### User Impact

This means the Pollard-to-Intermute pipeline can silently lose 100% of its data without any component reporting an error. The coordinator at line 269 of `coordinator.go` logs failures with `log.Printf("warn: ...")`, but this publisher does not.

### Recommendation

- **AC-PUBLISH.1**: `PublishFindings` logs each individual failure with finding ID and error.
- **AC-PUBLISH.2**: `PublishFindings` returns an error when ALL findings fail (total failure vs partial).
- **AC-PUBLISH.3**: Return value includes counts of successful and failed publishes so callers can make informed decisions.

---

## Finding 8: `StartWithScan` Silently Skips Failed Phase Re-generation

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 235-237

### The Problem

```go
for phase, pd := range phaseMap {
    if pd == nil {
        continue
    }
    draft, err := o.generator.GenerateDraft(ctx, phase, projectCtx, userInput, o.state.Findings, pd)
    if err != nil {
        continue // best-effort: fall back to draft without evidence
    }
    o.state.Sections[phase] = draft
}
```

When `GenerateDraft` fails for a phase, the `continue` silently skips that phase. The user sees the original draft (generated without scan evidence) but has no indication that the scan evidence was NOT incorporated. The comment says "best-effort: fall back to draft without evidence" but the user is never told this happened.

This is a quality degradation that directly affects AC-1.1 ("Kickoff completes codebase scan and displays results in doc pane"). The scan completed, results were collected, but they were silently not used for specific phases.

### Recommendation

- **AC-1.1a**: If scan evidence fails to incorporate into a phase draft, the log pane shows a warning identifying which phase and why.
- **AC-1.1b**: Phases using fallback (non-evidence-enriched) drafts are visually distinguished in the TUI (e.g., "Draft (no scan evidence)" vs "Draft (enriched)").

---

## Finding 9: Coordinator `publishToIntermute` Spawns Unbounded Background Goroutines

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/pollard/research/coordinator.go` lines 264-273

### The Problem

```go
go func() {
    for _, finding := range allFindings {
        insight := mapFindingToInsight(finding, project)
        _, err := client.CreateInsight(ctx, insight)
        if err != nil {
            log.Printf("warn: failed to publish finding to Intermute: %v", err)
        }
    }
}()
```

Every call to `processHuntResult` spawns a new goroutine to publish findings. With N hunters x M queries, this can create many concurrent goroutines all making HTTP requests to Intermute. There is no:
- Goroutine pool or semaphore limiting concurrency
- Context cancellation check inside the loop (the goroutine outlives the caller)
- Wait mechanism for completion
- Cleanup on coordinator shutdown

If Intermute is slow (e.g., 3-second timeout per request, 50 findings), these goroutines can accumulate. If the research run is cancelled, these goroutines continue running. They hold the `ctx` from the cancelled run, which may already be done, causing every `CreateInsight` to fail immediately -- but they still iterate through all findings.

### Recommendation

- **AC-GOROUTINE.1**: Background publishing uses a bounded worker pool (e.g., 3 concurrent publishes).
- **AC-GOROUTINE.2**: Background goroutines respect context cancellation between iterations.
- **AC-GOROUTINE.3**: Coordinator shutdown waits for all background goroutines to complete (or times out after 10 seconds).

---

## Finding 10: Sprint Persistence Uses Non-Atomic Write

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/persistence.go` line 33

### The Problem

```go
if err := os.WriteFile(path, data, 0644); err != nil {
    return fmt.Errorf("write state: %w", err)
}
```

Sprint state is persisted with `os.WriteFile`, which is NOT atomic. A crash or power loss during write produces a corrupted file. The existing `AtomicWriteFile` utility in `internal/file/atomic.go` exists and is used for spec revision snapshots but NOT for sprint state saves.

Sprint state saves happen frequently:
- On every phase advance (line 241, 423)
- On every chat message (line 1166)
- On every draft revision (line 1256)
- On background scan completion (line 1097)

Each of these is a corruption opportunity. A corrupted sprint file means the user loses their entire sprint progress (which can be 25+ minutes of work according to AC-1.13).

### Same Issue in Watch Snapshot

`saveSnapshot` in `/root/projects/Autarch/internal/pollard/watch/watcher.go` line 213 also uses `os.WriteFile`.

### Recommendation

- **AC-PERSISTENCE.1** (repeated from Finding 5): All persistent state writes use `AtomicWriteFile`.
- **AC-PERSISTENCE.2**: Sprint state includes a checksum. On load, checksum is verified. Corrupted files trigger a clear error message with recovery instructions ("Sprint file corrupted. Last known good state is [X]. Run `gurgeh history` to check revision snapshots.").

---

## Finding 11: `saveLocked()` Swallows Errors to stderr

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 157-164

### The Problem

```go
func (o *Orchestrator) saveLocked() {
    if o.state == nil {
        return
    }
    if err := SaveSprintState(o.state); err != nil {
        fmt.Fprintf(os.Stderr, "warning: failed to save sprint state: %v\n", err)
    }
}
```

This is called from at least 8 locations in the orchestrator. When the save fails:
1. The error is printed to stderr (not the TUI log pane, not a structured logger)
2. The calling function receives no indication of failure
3. The user continues working, believing their state is persisted
4. On crash, they lose all work since the last successful save

`fmt.Fprintf(os.Stderr, ...)` in a TUI application goes to a stream the user never sees (the TUI owns the terminal). This is effectively an empty catch block with a console.log that nobody reads.

The same pattern appears at:
- Line 182: `fmt.Fprintf(os.Stderr, "warning: Intermute spec creation failed: %v\n", err)`
- Line 996: `fmt.Fprintf(os.Stderr, "warning: phase research failed for %s: %v\n", ...)`
- Line 1023: `fmt.Fprintf(os.Stderr, "warning: quick scan failed: %v\n", err)`

### Recommendation

- **AC-ERROR.1**: All error logging in TUI-mode code uses the log pane handler, not stderr. Errors written to stderr are invisible to users.
- **AC-ERROR.2**: Save failures trigger a visible indicator in the TUI (e.g., "Unsaved changes" warning in the header).
- **AC-ERROR.3**: After a save failure, the orchestrator retries once. If the retry fails, the TUI displays a persistent warning.

---

## Finding 12: `LoadVisionContext` Silently Skips All Errors

**Severity:** MEDIUM
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 759-797

### The Problem

```go
func (o *Orchestrator) LoadVisionContext() *VisionContext {
    entries, err := os.ReadDir(specsDir)
    if err != nil {
        return nil  // Silent: directory doesn't exist or permission error
    }
    for _, entry := range entries {
        // ...
        data, err := os.ReadFile(specsDir + "/" + entry.Name())
        if err != nil {
            continue  // Silent: file read error
        }
        var spec specs.Spec
        if err := yaml.Unmarshal(data, &spec); err != nil {
            continue  // Silent: corrupt spec YAML
        }
        // ...
    }
    return nil  // Silent: no vision spec found
}
```

Every error path returns nil silently. A corrupt vision spec file produces the same result as "no vision spec exists." The user gets no vertical consistency checks (comparing current sprint against the vision) without knowing why.

### User Impact

If a vision spec exists but is corrupted (e.g., partial YAML from a non-atomic write), the user loses all vertical consistency checking with no error message. They might wonder why the consistency engine isn't flagging conflicts.

### Recommendation

- **AC-CONSISTENCY.1**: When a vision spec file exists but fails to parse, log a warning identifying the file and error.
- **AC-CONSISTENCY.2**: When vertical consistency is disabled due to missing/corrupt vision context, the confidence detail view explains why.

---

## Finding 13: Signal Store `Count()` Swallows Database Errors

**Severity:** MEDIUM
**Location:** `/root/projects/Autarch/internal/gurgeh/signals/store.go` lines 127-131

### The Problem

```go
func (s *Store) Count(specID string) int {
    var count int
    _ = s.db.QueryRow(...).Scan(&count)
    return count
}
```

The `_ =` discards the database query error. If the database is corrupted, locked, or the table doesn't exist, `Count()` returns 0. This means the Pollard tab badge (AC-1.3) shows 0 active signals even when the database is broken. The user sees "no signals" when the truth is "signals unknown."

### Recommendation

- **AC-SIGNALS.1**: `Count()` returns `(int, error)`. Callers handle the error explicitly. Badge displays "?" or an error indicator when signal count is unknown.

---

## Finding 14: `EmitAll` Uses String Matching for Error Classification

**Severity:** MEDIUM
**Location:** `/root/projects/Autarch/internal/gurgeh/signals/store.go` lines 162-172

### The Problem

```go
func (s *Store) EmitAll(sigs []signals.Signal) error {
    for _, sig := range sigs {
        if err := s.Emit(sig); err != nil {
            if !strings.Contains(err.Error(), "UNIQUE constraint") {
                return err
            }
        }
    }
    return nil
}
```

Error classification by string matching (`strings.Contains(err.Error(), "UNIQUE constraint")`) is fragile. If the SQLite driver changes its error message format, or if a different SQLite error happens to contain "UNIQUE constraint" in its message, the classification breaks. This could cause:
1. Non-unique errors being silently swallowed (if they happen to contain the string)
2. Unique constraint violations being treated as fatal (if the message format changes)

### Recommendation

- **AC-SIGNALS.2**: Use typed error checking (e.g., SQLite error codes) instead of string matching for error classification.

---

## Finding 15: Signals Store `Emit` Uses INSERT OR IGNORE -- Deduplication is Silent

**Severity:** MEDIUM
**Location:** `/root/projects/Autarch/internal/gurgeh/signals/store.go` lines 69-86

### The Problem

```go
func (s *Store) Emit(sig signals.Signal) error {
    _, err := s.db.Exec(`INSERT OR IGNORE INTO signals ...`, ...)
    // ...
}
```

`INSERT OR IGNORE` means duplicate signals are silently discarded at the database level. This is the deduplication mechanism for AC-3.4a. However:

1. The caller has no way to know if the signal was a duplicate or a new insertion
2. There is no logging of duplicate detection
3. No metrics track dedup hit rate

While deduplication is correct behavior, **silent deduplication prevents debugging**. If signals are being generated with incorrect `(spec_id, type, affected_field)` tuples, the dedup silently hides the problem.

### Recommendation

- **AC-3.4a-EXT**: Emit returns a `(wasNew bool, err error)` tuple. Callers can log dedup events for debugging. The dedup hit rate is available as a diagnostic metric.

---

## Finding 16: `rand.Read` Errors Discarded in Signal ID Generation

**Severity:** MEDIUM
**Location:**
- `/root/projects/Autarch/internal/gurgeh/signals/emitter.go` lines 117-121
- `/root/projects/Autarch/internal/pollard/signals/emitter.go` lines 50-54
- `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` line 167 (per grep)
- `/root/projects/Autarch/internal/pollard/watch/watcher.go` lines 163-167

### The Problem

```go
func generateID() string {
    b := make([]byte, 8)
    _, _ = rand.Read(b)
    return "sig-" + hex.EncodeToString(b)
}
```

All four signal ID generators discard the error from `rand.Read`. On systems where `/dev/urandom` is unavailable or the entropy pool is exhausted, `rand.Read` can fail. The result would be signal IDs generated from a zero-filled byte array: `sig-0000000000000000`. This would cause:
1. All signals to have the same ID
2. The UNIQUE constraint on the signals table to reject all but the first signal
3. Effective loss of all signal functionality

While this is extremely unlikely on a modern Linux system, discarding the error violates the principle that all error paths must be handled. In Go, `crypto/rand.Read` panics on failure in the standard library, but the `_, _ =` pattern prevents the compiler from warning about unused errors.

### Recommendation

- **AC-UTIL.1**: Signal ID generation checks rand.Read error and panics on failure (this is the correct behavior for crypto/rand -- a system with no randomness source is fundamentally broken).

---

## Finding 17: Watch Cycle Errors Only Go to stderr

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/pollard/watch/watcher.go` lines 104-105, 113-114

### The Problem

```go
if _, err := w.RunOnce(ctx); err != nil {
    fmt.Fprintf(os.Stderr, "watch cycle error: %v\n", err)
}
```

The continuous watcher logs errors to stderr and continues. In a TUI context, stderr is invisible. In a background daemon context (e.g., running via systemd), this might go to journald, but there's no structured logging, no error categorization, and no alerting.

The watcher runs on a 24-hour (or shorter) cycle. If the first cycle fails and every subsequent cycle fails (e.g., permanent network issue), the user gets zero research validation (CUJ-3) with zero notification.

### Recommendation

- **AC-3.1a**: Watch cycle errors are emitted as signals (`SignalWatchError`) so they appear in the Signals tab and Bigend dashboard.
- **AC-3.1b**: After N consecutive watch cycle failures (configurable, default 3), a critical-severity signal is emitted.

---

## Finding 18: Advance Fallback Chain Obscures Root Cause

**Severity:** HIGH
**Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` lines 356-422

### The Problem

The `advanceInternal` method has a triple-nested fallback chain for generating phase content:

1. Try cached extraction
2. If cache exists but missing this phase, try context-aware generation
3. If that fails, try full exploration
4. If that fails, try template generation
5. If that fails, return error

When fallback #2 fails and #3 succeeds, the user gets content generated by a more expensive, less targeted method -- but has no idea why. The error from #2 is swallowed:

```go
genContent, err := exploration.GeneratePhaseFromContext(...)
if err != nil {
    // Fallback to full exploration if context-aware generation fails
    genContent, err = exploration.GeneratePhase(...)
```

This means:
- If the Claude Code session expired, the user gets a different (potentially lower quality) generation method silently
- If there's a permission error, it's masked by the fallback
- If there's a rate limit, it's hidden

### Recommendation

- **AC-ADVANCE.1**: Each fallback in the generation chain logs the reason for fallback (what failed and why).
- **AC-ADVANCE.2**: The TUI indicates when a phase was generated by a fallback method (e.g., "Generated (fallback: context-aware generation failed)").

---

## Finding 19: `LoadHistory` Silently Skips Unreadable and Corrupt Revision Files

**Severity:** MEDIUM
**Location:** `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` lines 170-179

### The Problem

```go
for _, e := range entries {
    // ...
    data, err := os.ReadFile(filepath.Join(dir, name))
    if err != nil {
        continue  // Silent skip
    }
    var rev SpecRevision
    if err := yaml.Unmarshal(data, &rev); err != nil {
        continue  // Silent skip
    }
    revisions = append(revisions, rev)
}
```

When `gurgeh history <spec-id>` (AC-1.15) lists versions, corrupt or unreadable revision files are silently skipped. The user sees versions 1, 2, 4, 5 and has no idea that version 3 exists but is corrupt. This is especially problematic because:
- `gurgeh diff <spec-id> v1 v3` would fail with "version not found" when the file actually exists but is corrupt
- The user might think they only have 4 revisions when they have 5

### Recommendation

- **AC-1.15a**: `gurgeh history` reports corrupt revision files as warnings, including filename and error.

---

## Plan-Level Gap Analysis

### The "Negative/Failure Path Testing" Section is Dangerously Thin

The plan's negative testing section (lines 366-371) covers exactly 5 scenarios -- one per CUJ. Each is a single sentence. For a system with 12+ async components, 4 external dependencies (Intermute, Claude Code, various APIs, filesystem), and concurrent agents, this is woefully inadequate.

**Missing negative test scenarios:**

1. **Multiple hunters fail simultaneously** -- Does the sprint stall? Does the TUI hang?
2. **Intermute connection drops mid-reservation** -- Is the reservation in an inconsistent state?
3. **SQLite WAL file grows unbounded** -- What happens when disk fills?
4. **Agent Teams teammate crashes during plan approval** -- Is the approval state recoverable?
5. **WebSocket reconnects during signal emission** -- Are in-flight signals delivered?
6. **Concurrent phase advances from TUI and agent** -- Race condition on state
7. **Pollard watch finds contradicting research during active sprint** -- Signal delivery during active editing
8. **Spec export while phase generation is in progress** -- Partial spec exported
9. **Feedback.yaml exceeds rolling window during concurrent triage** -- Race between archive and append
10. **Config.yaml deleted while watcher is running** -- Config reload behavior

### The Degradation Matrix Has Untested Cells

The plan acknowledges (line 430) that "Agent Teams ON + Intermute OFF" is currently untested. This is the most dangerous configuration because Agent Teams creates multiple concurrent agents that NEED Intermute's reservation enforcement. Without it, file conflicts are guaranteed.

### Confidence Score is the Primary User-Facing Quality Signal -- But It Hides Problems

The confidence score (AC-1.8) is the number users look at to decide "is my PRD good enough?" But:
- It doesn't reflect hunter failures (Finding 1)
- It doesn't reflect Intermute availability (Finding 3)
- It doesn't reflect signal delivery failures (Finding 2)
- It doesn't reflect corrupted data (Findings 5, 12)

The plan should add a "Confidence Score Integrity" criterion:
- **AC-CONFIDENCE.1**: Every factor that reduces PRD quality is reflected in the confidence score or an adjacent indicator. The score never gives a false sense of security.

---

## Summary of All Recommended Acceptance Criteria Additions

| ID | Criterion | Category |
|----|-----------|----------|
| AC-1.18 | Hunter failure reflected in Research confidence component | Confidence integrity |
| AC-1.19 | Per-hunter status visible in confidence detail | Error visibility |
| AC-1.20 | Partial hunter results report success/failure counts | Error granularity |
| AC-1.1a | Scan evidence incorporation failure logged per phase | Fallback visibility |
| AC-1.1b | Fallback drafts visually distinguished | User awareness |
| AC-1.15a | Corrupt revision files reported as warnings | Error visibility |
| AC-3.4e | Signal drop logged with type and subscriber | Silent failure prevention |
| AC-3.4f | Dropped signal count exposed as metric | Observability |
| AC-3.4g | Critical signals never dropped | Data integrity |
| AC-3.4a-EXT | Dedup returns wasNew boolean | Debug visibility |
| AC-3.1a | Watch errors emitted as signals | Error visibility |
| AC-3.1b | Consecutive watch failures escalate severity | Alerting |
| AC-3.9a | Feedback writes use atomic operations | Crash safety |
| AC-3.9b | Corrupt feedback detected and repaired | Crash recovery |
| AC-3.9c | Archive operation is atomic | Crash safety |
| AC-3.9d | Concurrent triage uses file locking | Race prevention |
| AC-5.3a | WebSocket events logged | Observability |
| AC-5.3b | Auto-reconnection with backoff | Resilience |
| AC-5.3c | State reconciliation on reconnect | Data consistency |
| AC-5.3d | Disconnect indicator in dashboard | User awareness |
| AC-5.3e | Server-side signal buffering for disconnected clients | Data integrity |
| AC-X.5a | Nil Intermute returns sentinel error, not nil | Error honesty |
| AC-X.5b | Persistent offline indicator in TUI | User awareness |
| AC-X.5c | Skipped reservations logged with paths | Security visibility |
| AC-X.5d | Integration component in confidence | Confidence integrity |
| AC-PERSISTENCE.1 | All state writes use atomic operations | Crash safety |
| AC-PERSISTENCE.2 | Sprint state checksum verification | Corruption detection |
| AC-RESEARCH.1 | PublishInsight errors logged | Error visibility |
| AC-RESEARCH.2 | Bulk publish failure summary | Aggregate alerting |
| AC-RESEARCH.3 | No `_, _` pattern for fallible functions | Code quality |
| AC-PUBLISH.1 | Individual publish failures logged | Error visibility |
| AC-PUBLISH.2 | Total failure returns error | Error honesty |
| AC-PUBLISH.3 | Success/failure counts returned | Error granularity |
| AC-ERROR.1 | TUI errors go to log pane, not stderr | User visibility |
| AC-ERROR.2 | Save failure shows TUI indicator | User awareness |
| AC-ERROR.3 | Save retry with persistent warning | Resilience |
| AC-ADVANCE.1 | Fallback reasons logged | Debug visibility |
| AC-ADVANCE.2 | Fallback method indicated in TUI | User awareness |
| AC-GOROUTINE.1 | Bounded publishing worker pool | Resource safety |
| AC-GOROUTINE.2 | Background goroutines check cancellation | Resource cleanup |
| AC-GOROUTINE.3 | Shutdown waits for goroutines | Graceful shutdown |
| AC-CONFIDENCE.1 | All quality factors reflected in score | Confidence integrity |
| AC-SIGNALS.1 | Count returns error | Error honesty |
| AC-SIGNALS.2 | Typed error checking, not string matching | Robustness |
| AC-UTIL.1 | rand.Read failure panics | Fail-fast correctness |

---

## Priority Ordering

**Phase 0 (Before any CUJ testing):**
1. AC-PERSISTENCE.1 -- Atomic writes for all state (prevents data loss)
2. AC-ERROR.1 -- TUI errors go to log pane (enables all other error visibility)
3. AC-RESEARCH.3 -- Eliminate `_, _` pattern (prevents silent publish failures)
4. AC-3.4g -- Critical signals never dropped (prevents research invalidation loss)

**Phase 1 (Before CUJ-1 testing):**
5. AC-1.18 -- Hunter failure in confidence
6. AC-1.19 -- Per-hunter status visibility
7. AC-ADVANCE.1 -- Fallback chain logging
8. AC-X.5a/b/c -- Intermute offline indicators

**Phase 2 (Before CUJ-3/4/5 testing):**
9. AC-5.3a/b/c/d -- WebSocket resilience
10. AC-3.9a/b/c/d -- Feedback atomicity and crash recovery
11. AC-GOROUTINE.1/2/3 -- Bounded background publishing
12. AC-3.1a/b -- Watch error escalation

**Phase 3 (Before production):**
13. All remaining criteria
