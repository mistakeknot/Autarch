# TUI race conditions review

**Source:** Agent result from session b817bfb0
**Task ID:** a65fa2f
**Date:** 2026-02-05

---

Now I have a thorough understanding of the codebase. Let me compile the review.

---

# Race Condition Review: Autarch TUI Layer and Acceptance Criteria Plan

I have read the plan at `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md` and cross-referenced it against the existing code in the arbiter orchestrator, broker, Bigend aggregator, persistence layer, and TUI components. Here are the findings, ordered by severity.

---

## RACE 1: ArbiterView reads and mutates `v.state` without synchronisation -- while the orchestrator mutates it from background goroutines

**Severity: CRITICAL**

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go`

The `ArbiterView` holds a raw `*SprintState` pointer at line 28:

```go
state *arbiter.SprintState
```

The `Init()` method (line 141) and `StartWithInput()` (line 153) set this pointer from inside a `tea.Cmd` closure -- which Bubble Tea calls asynchronously. Meanwhile, `View()` (line 406) reads `v.state` on every render tick, `Update()` (line 166) reads it on every keystroke, and `acceptDraft()` (line 255) mutates `v.state.Sections` and passes `v.state` to `o.Advance()`.

Here is the unfolding:

1. User presses Ctrl+A to accept a draft. `acceptDraft()` calls `o.Advance(ctx, v.state)` (line 271).
2. Inside `Advance()`, when the phase reaches `PhaseUsers`, the orchestrator fires a background goroutine (line 328): `go func() { o.runQuickScanBackground(bgCtx) }()`.
3. That goroutine eventually writes to `o.state.ResearchCtx` and `o.state.Findings` (line 1062-1064, 1079-1080). But `v.state` in the TUI is a **different pointer** -- it was set from the return value of `o.Start()`, which returned a clone.
4. Except no -- look again. `acceptDraft()` passes `v.state` directly (not a clone) to `Advance()`. And `Advance()` mutates the `state` argument in-place: `state.Phase = phases[i+1]` (line 316), `state.Sections[state.Phase] = draft` (line 346), `state.Conflicts = conflicts` (line 300). This is the same pointer the TUI reads in `View()`.
5. So while the goroutine from step 2 is mutating the orchestrator's internal `o.state`, the TUI's `v.state` is being mutated by `Advance()` without any lock, and `View()` is reading it at ~60 FPS simultaneously.

The `go test -race` mandate in the institutional learnings (line 352 of the plan) exists precisely because this pattern was discovered before, documented as `arbiter-state-pointer-escape`. The `State()` method on the orchestrator does return a deep clone -- but the TUI does not use it. The TUI directly holds and mutates a bare pointer.

**What the user sees:** Imagine the confidence score says "42%" and then, for one frame, it flickers to "0%" because `Advance()` is mid-write on the `Confidence` struct fields. Or worse -- the `Sections` map gets written to while `updateDocPanel()` iterates it, causing a `concurrent map read and map write` panic. The TUI crashes with a stack trace that means nothing to the user. This is not hypothetical; this is the Go runtime detecting a map race and choosing to terminate the process.

**What the plan should add:**
- AC-1.X: "ArbiterView must never hold a direct `*SprintState` pointer. It must obtain snapshots via `Orchestrator.State()` and refresh on each `Update()` cycle. All arbiter tests must pass `go test -race`."
- The `Advance()` method should not accept an external `*SprintState` at all -- it should work on its internal state under the mutex, and return a clone.

---

## RACE 2: `Advance()` accepts and mutates an external `*SprintState` while the orchestrator's internal state drifts

**Severity: CRITICAL**

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go`, lines 283-412

`Advance()` takes `state *SprintState` as a parameter and mutates it extensively (setting `Phase`, `Sections`, `Conflicts`, `Confidence`, `UpdatedAt`). But it also fires `runQuickScanBackground()` which later acquires `o.mu` and mutates `o.state` -- the orchestrator's own pointer.

Now look at `ChatAcceptDraft()` (line 1187):

```go
o.mu.Lock()
// ... mark current draft accepted ...
state := o.state.Clone()
statePtr := &state
o.mu.Unlock()

updated, err := o.Advance(ctx, statePtr) // works on clone

o.mu.Lock()
if updated != nil {
    o.state = updated  // replaces internal state with Advance result
}
o.mu.Unlock()
```

This is the correct pattern -- clone before, replace after. But `ArbiterView.acceptDraft()` (line 255) does none of this:

```go
v.orchestrator.AcceptDraft(v.state)  // no lock
newState, err := v.orchestrator.Advance(context.Background(), v.state)  // no lock
v.state = newState  // bare assignment
```

Two code paths, two completely different concurrency contracts, operating on the same orchestrator. If the TUI path and the chat path ever run concurrently (e.g., user has both a chat agent and keyboard interaction in the same session), they will tear each other apart.

**What the plan should add:**
- AC-X.Y: "Only one code path may mutate sprint state at a time. `Advance()` must be called exclusively through the orchestrator's mutex-protected methods."

---

## RACE 3: Signal broker silently drops signals under load -- no backpressure, no deduplication, no persistence

**Severity: HIGH**

**File:** `/root/projects/Autarch/pkg/signals/broker.go`, lines 51-54

```go
select {
case sub.ch <- sig:
default:
    // Drop if subscriber is slow.
}
```

The plan correctly identifies this at "Gap 3" (line 203-207) but AC-3.4a merely says "Signal deduplication prevents repeat alerts." The deeper problem is not deduplication -- it is that signals are lost with no trace. No counter, no log, no metric. A `SignalResearchInvalidation` of severity `critical` is silently discarded because the TUI was busy rendering a long doc panel.

**What happens to the user:** Pollard discovers a competitor has shipped your "unique differentiator." A `critical` signal is emitted. The broker tries to push it. The subscriber's 64-element channel is full because the TUI is processing a batch of `insight.created` events from a deep dive. The signal vanishes. The user continues building on an invalidated assumption. They discover the problem only after shipping. The confidence score remains cheerfully at 78%.

**Mitigation in the plan:**
- Increase channel buffer? No -- that just defers the problem by 64 more signals.
- The correct approach is a ring buffer with overflow notification: when a signal is dropped, set a flag that the subscriber can check, and log the drop. Even better, make `critical` signals blocking (with a short timeout) and non-critical ones fire-and-forget.
- The `Stream()` method (line 106-115) has the same pattern in reverse: it writes to `out` without a `select`-with-default, meaning it blocks indefinitely if the consumer is slow. A slow Bigend dashboard could back-pressure the entire signal pipeline.

**What the plan should add:**
- AC-3.4b: "Critical signals (severity=critical) must not be silently dropped. Broker must log or queue overflow events with a bounded ring buffer."
- AC-3.4c: "`Stream()` must use a select with context cancellation to prevent goroutine leaks when consumers disconnect."

---

## RACE 4: `SaveRevision` has non-atomic two-file writes and mutates input spec

**Severity: HIGH**

**File:** `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`, lines 41-81

```go
func SaveRevision(root string, spec *Spec, ...) (*SpecRevision, error) {
    version := spec.Version + 1
    spec.Version = version     // SIDE EFFECT: mutates the caller's spec

    // Write 1: snapshot YAML
    os.WriteFile(snapPath, data, 0644)

    // Write 2: revision metadata
    os.WriteFile(revPath, revData, 0644)
}
```

Three problems here:

1. **Version number side effect:** `spec.Version = version` mutates the caller's spec regardless of whether the writes succeed. If the snapshot write succeeds but the revision write fails, the caller holds a spec with version N+1 but no corresponding revision file exists. The next call produces version N+2, and version N+1 is a phantom.

2. **Non-atomic writes:** If the process dies between the two `WriteFile` calls, you get a snapshot without revision metadata. The `LoadHistory()` function only looks for `_rev.yaml` files, so the orphaned snapshot is invisible -- a ghost version that consumes disk but is unreachable.

3. **No file locking:** Two concurrent phase completions (imagine the chat agent and the TUI both accepting drafts simultaneously on the same sprint) both call `SaveRevision` with the same spec. Both read `spec.Version` as N, both compute N+1, and both write `{spec}_v{N+1}.yaml`. The second write silently overwrites the first. You lose a version with no indication.

**What the user sees:** "I accepted all 8 phases but `gurgeh history` only shows 5 versions." Or worse: the diff between v3 and v4 shows changes that were actually in v5, because v4's snapshot was clobbered.

**Mitigation:**
- Write-to-temp-then-rename (atomic on POSIX).
- Compute version from existing files on disk, not from the input spec's mutable field.
- Use `os.O_EXCL` to detect collisions instead of silently overwriting.

**What the plan should add:**
- AC-1.15 should explicitly require: "No duplicate version numbers under concurrent phase completions. `SaveRevision` must use atomic file operations (write-to-temp, rename) and detect version collisions."

---

## RACE 5: Aggregator `GetState()` returns a shallow copy of mutable slices

**Severity: HIGH**

**File:** `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 694-698

```go
func (a *Aggregator) GetState() State {
    a.mu.RLock()
    defer a.mu.RUnlock()
    return a.state
}
```

This returns a `State` by value, which copies the struct. But Go copies struct fields shallowly. `State.Projects` is `[]discovery.Project` -- the slice header is copied but the underlying array is shared. When `handleIntermuteEvent()` later calls `a.enrichWithGurgStats(a.state.Projects)` (line 278) and mutates `projects[i].GurgStats` inside a goroutine, it mutates the same `Project` structs that an HTTP handler received earlier from `GetState()` and is iterating over in its template rendering.

This is exactly the pointer-escape pattern from the institutional learnings but in a different package. The `State()` method on `Orchestrator` does deep cloning via `Clone()`. The `GetState()` on `Aggregator` does not.

**What the user sees:** The Bigend dashboard shows "Spec Count: 3" for a project, then the template panics mid-render because `projects[i].GurgStats` was nil when the template began and non-nil when it tried to read a field. Or: the dashboard shows stale data interleaved with fresh data in the same HTTP response, creating a Frankenstein view where project A shows the stats of project B.

**What the plan should add:**
- AC-5.3 should require: "`GetState()` returns a deep copy. Concurrent WebSocket event handling must not mutate state visible to in-flight HTTP responses."

---

## RACE 6: `refreshForEvent()` spawns goroutines that acquire the mutex while other goroutines are already enriching

**Severity: MEDIUM-HIGH**

**File:** `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 267-310

```go
case strings.HasPrefix(eventType, "spec."):
    go func() {
        a.mu.Lock()
        a.enrichWithGurgStats(a.state.Projects)
        a.state.UpdatedAt = time.Now()
        a.mu.Unlock()
    }()

case strings.HasPrefix(eventType, "insight."):
    go func() {
        a.mu.Lock()
        a.enrichWithPollardStats(a.state.Projects)
        a.state.UpdatedAt = time.Now()
        a.mu.Unlock()
    }()
```

Each event spawns a new goroutine. If events arrive rapidly (which they will during a deep dive that produces dozens of `insight.created` events), you get N goroutines all contending for the same mutex, each doing I/O (walking directories, reading YAML files) while holding the write lock. The mutex is not the problem -- the problem is that `enrichWithGurgStats` does filesystem I/O while holding `a.mu.Lock()`. Every `GetState()` call (which takes `a.mu.RLock()`) will block behind all of them.

The Bigend dashboard freezes. The user clicks refresh. Nothing happens for 3 seconds. They click again. Now there are two pending refreshes queued behind the enrichment goroutines. The WebSocket terminal streaming handler at `/ws/terminal/` also calls `GetState()` to verify the session exists (line 425) -- so the live terminal view freezes too.

**What the plan should add:**
- AC-5.3 should include: "WebSocket event processing must not hold the aggregator write lock during I/O operations. Enrichment should be done outside the lock, then results applied under the lock."

---

## RACE 7: Feedback YAML concurrent writes from rapid triage

**Severity: MEDIUM**

**File:** Not yet implemented (plan references `.pollard/feedback.yaml`, only in `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`)

The plan describes feedback YAML as a simple append-only file with a rolling window of 50 entries (AC-3.9). But:

1. The plan says the MCP tools (`autarch_triage_finding` in AC-1.17) and the TUI agent pane (AC-1.6) can both write to `feedback.yaml`. If both paths trigger simultaneously -- user triages via keyboard while the agent processes a natural language command -- both will read the file, parse YAML, append an entry, and write it back. One write will be lost.

2. The rolling window (archive-then-truncate) is explicitly called out as non-atomic in the plan's "Data Integrity Risks" section. But the plan adds no AC to address it.

**What the user sees:** "I rejected 'consumer-focused results' but Pollard keeps surfacing them." Because the triage decision was silently overwritten by a concurrent agent triage. The feedback loop (AC-3.6 through AC-3.8) fails silently.

**What the plan should add:**
- AC-3.9a: "Concurrent triage operations must use file locking (`flock`) or a single-writer channel to prevent YAML corruption."
- AC-3.9b: "Archive-then-truncate must be atomic: write new file, rename to archive, then truncate in a single locked operation."

---

## RACE 8: WebSocket terminal streaming checks session existence without holding state across the connection

**Severity: MEDIUM**

**File:** `/root/projects/Autarch/internal/bigend/web/server.go`, lines 416-508

```go
state := s.agg.GetState()
var found bool
for _, session := range state.Sessions {
    if session.Name == sessionName {
        found = true
        break
    }
}
if !found {
    http.NotFound(w, r)
    return
}
// ... accept WebSocket ...
// ... stream terminal output indefinitely ...
```

The session existence check is a point-in-time snapshot. The session can be killed between the check and the WebSocket accept. More subtly, the session can be *renamed* (via `RenameSession`) between the check and the first `CapturePane` call. The ticker loop will then fail on every tick because the session name no longer exists, but the error path returns immediately without sending a proper WebSocket close frame -- the client just sees the connection drop.

Also: the 100ms ticker (line 466) has no cancellation mechanism beyond `ctx.Done()`. If the HTTP context is not properly cancelled when the client disconnects, this goroutine leaks, calling `CapturePane` every 100ms on a dead connection.

**What the user sees:** The terminal streaming view shows "session ended or capture failed" and goes blank. Not the end of the world, but combined with the `OriginPatterns: []string{"*"}` (line 440), it means any website open in the user's browser can connect to this WebSocket and trigger these goroutine leaks repeatedly.

---

## RACE 9: `runQuickScanBackground` uses stale `specID` after dropping the lock

**Severity: MEDIUM**

**File:** `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go`, lines 1033-1094

```go
o.mu.Lock()
// ... extract topic and specID from o.state ...
specID := o.state.SpecID
o.mu.Unlock()

// ... do the scan (takes 10-30 seconds) ...

// Use specID to publish
o.research.PublishInsight(ctx, specID, ...)
```

The `specID` is captured before the scan starts. If, during the 10-30 seconds the scan takes, the user starts a new sprint (which calls `Start()` and sets `o.state` to a new `SprintState` with a different `SpecID`), the background goroutine publishes the insight to the **old** spec. The findings then get written back to `o.state.Findings` (line 1079) -- which is now the **new** sprint's state. The new sprint receives findings from the old sprint's topic.

**What the user sees:** You start a sprint about "real-time collaboration," quickly abandon it, start a new sprint about "payment processing," and thirty seconds later the sidebar shows "3 new findings: Real-time collaboration frameworks on GitHub."

**What the plan should add:**
- The background goroutine should check whether `o.state.ID` still matches the sprint ID that started the scan before writing results back.

---

## RACE 10: Bigend aggregator `enrichWithGurgStats` mutates the `projects` slice in-place from WebSocket goroutines

**Severity: MEDIUM**

**File:** `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go`, lines 276-281

```go
go func() {
    a.mu.Lock()
    a.enrichWithGurgStats(a.state.Projects)
    a.state.UpdatedAt = time.Now()
    a.mu.Unlock()
}()
```

`enrichWithGurgStats()` mutates `projects[i].GurgStats` and `projects[i].TaskStats` in-place. It receives `a.state.Projects` which is the live slice. If `Refresh()` is called concurrently (from the `/api/refresh` endpoint), it replaces `a.state` entirely (line 394-404), but the goroutine from `refreshForEvent` still holds a reference to the old `Projects` slice and continues mutating it. Those mutations are lost, but the old slice may also be held by an in-flight HTTP handler that got it from `GetState()` before the refresh. That handler now sees partially-enriched, partially-stale data.

---

## Summary Matrix

| Race | Severity | Component | Symptom | Plan AC Gap |
|------|----------|-----------|---------|-------------|
| 1 | CRITICAL | ArbiterView | Map panic / flickering UI | AC-1.X missing: state snapshot discipline |
| 2 | CRITICAL | Orchestrator.Advance | Silent state corruption between TUI and chat paths | AC-X.Y missing: single mutation path |
| 3 | HIGH | Broker.Publish | Critical signals silently dropped | AC-3.4b-3.4c missing |
| 4 | HIGH | SaveRevision | Phantom versions, clobbered snapshots | AC-1.15 underspecified |
| 5 | HIGH | Aggregator.GetState | Dashboard shows Frankenstein data | AC-5.3 underspecified |
| 6 | MEDIUM-HIGH | Aggregator.refreshForEvent | Dashboard freezes during enrichment | AC-5.3 underspecified |
| 7 | MEDIUM | feedback.yaml writes | Triage decisions silently lost | AC-3.9a-3.9b missing |
| 8 | MEDIUM | handleTerminalWS | Goroutine leaks, blank terminal view | AC-5.3 underspecified |
| 9 | MEDIUM | runQuickScanBackground | Findings from wrong sprint | No AC |
| 10 | MEDIUM | enrichWithGurgStats | Partially-enriched stale state | AC-5.3 underspecified |

---

## How to Induce These Failures for Testing

1. **Race 1:** Run `go test -race ./internal/gurgeh/arbiter/tui/...` while rapidly pressing Ctrl+A through all 8 phases. The race detector will fire on `v.state.Sections` map access.

2. **Race 3:** Create a subscriber, fill its channel by publishing 65 signals, then publish one `critical` signal. Assert it was not dropped.

3. **Race 4:** In a test, call `SaveRevision` twice concurrently with the same spec (same goroutine or two goroutines with a sync point). Assert that the resulting version numbers differ and both files exist.

4. **Race 5:** In a test, call `GetState()`, then immediately call `Refresh()` from another goroutine, then read from the returned state. With `-race`, the detector will fire on the shared `Projects` slice elements.

5. **Race 9:** Start a sprint, trigger advance to PhaseUsers (which launches background scan), then immediately call `Start()` with a new topic. Wait for background scan to complete. Assert that findings written to `o.state` match the *new* sprint's topic, not the old one.

---

## Architectural Recommendations

1. **Adopt the "snapshot-only" discipline project-wide.** The orchestrator already has `State()` returning a clone. The aggregator's `GetState()` must do the same. The TUI must never hold a mutable reference. This is not a "nice to have" -- it is the difference between "the TUI occasionally panics" and "the TUI is reliable."

2. **Make `Advance()` a method on the orchestrator, not a free function that takes state.** It should acquire the mutex, work on internal state, save, and return a clone. The current signature `Advance(ctx, *SprintState)` invites exactly the pointer-escape race that was already caught once.

3. **Add a generation counter (or sprint ID check) to background goroutines.** Before writing results back, verify the sprint is still the same one that initiated the work. This is the standard "cancellation token" pattern, applied to goroutines instead of timeouts.

4. **The signal broker needs at minimum an overflow counter.** Silent drops are antithetical to the plan's stated goal of "no unresolved blockers." A `critical` signal that gets silently dropped is an unresolved blocker that nobody knows about.

5. **`SaveRevision` needs `os.O_CREATE|os.O_EXCL` or write-to-temp-rename.** YAML file I/O has a nasty habit of looking atomic until two goroutines prove otherwise. This is the kind of bug that shows up once every 200 sprints and is impossible to reproduce.