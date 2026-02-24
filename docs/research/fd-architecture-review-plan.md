# Architecture Review: Acceptance Criteria Plan

**Reviewer:** Architecture Reviewer (Claude Opus 4.6)
**Date:** 2026-02-05
**Plan reviewed:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Source files examined:** 18 files across `pkg/`, `internal/`, and `docs/`

---

## Architecture Assessment

### Components Affected

The acceptance criteria plan touches all five major subsystems plus two shared packages:

| Component | Role in Plan | Boundary Status |
|-----------|-------------|-----------------|
| **Gurgeh** (`internal/gurgeh/`) | CUJ-1: PRD creation, spec versioning, arbiter sprint | Boundaries respected |
| **Pollard** (`internal/pollard/`) | CUJ-1/CUJ-3: Research hunters, findings, watch cycles | Boundaries respected |
| **Coldwine** (`internal/coldwine/`) | CUJ-2/CUJ-4: Task hierarchy, agent coordination, reservations | **Boundary breach risk** (see Issue 1) |
| **Bigend** (`internal/bigend/`) | CUJ-5: Dashboard aggregation, WebSocket events | Boundaries respected |
| **Intermute** (`../Intermute`) | Cross-cutting: Reservations, events, messaging | External dependency |
| **pkg/signals** | Cross-cutting: Typed alerts | **Transport path ambiguous** (see Issue 3) |
| **pkg/intermute** | Cross-cutting: Client wrapper | Boundaries respected |

### Boundary Compliance Summary

The plan generally respects the established module boundaries documented in `AGENTS.md` and `docs/ARCHITECTURE.md`. Each tool writes to its own data directory (`.gurgeh/`, `.coldwine/`, `.pollard/`), reads from others via file-based integration, and uses Intermute for real-time coordination. Bigend remains read-only as documented.

However, three architectural concerns merit attention: the Coldwine-Agent Teams bridge introduces a new integration surface that is both unspecified and potentially coupling, the signal transport path contradicts the documented architecture, and the Coldwine local reservation system duplicates Intermute's reservation responsibility.

---

## Specific Issues

### Issue 1: Coldwine-Agent Teams Bridge Is Architecturally Unspecified and Risks Tight Coupling

**Location:** CUJ-4 (AC-4.8), Open Question 8, Gap 2 in Research Insights

**Problem:** The plan states "Coldwine acquires Intermute file reservations automatically when a teammate claims a task from the shared task list" but does not specify the detection mechanism. This is not merely a missing implementation detail -- it is the architectural linchpin of CUJ-4 and determines whether Coldwine couples tightly to Claude Code Agent Teams internals.

The three options mentioned in Open Question 8 have fundamentally different architectural consequences:

- **Polling `~/.claude/teams/` task file:** Introduces file-watching dependency on an external, experimental system's internal state format. If Agent Teams changes its storage format, Coldwine breaks. Creates a coupling from `internal/coldwine/` to an external process's internal files.
- **Event hook / API wrapping:** Requires Agent Teams to expose an event interface. Since Agent Teams is experimental and Autarch has no control over its API surface, this is fragile.
- **Coldwine as the task-claim API:** Coldwine could be the authority that teammates call to claim tasks (via MCP tools), then Coldwine acquires reservations. This inverts the dependency: teammates call Coldwine, rather than Coldwine polling Agent Teams.

The existing code confirms the gap. Looking at `/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go`, Coldwine's Intermute integration is purely outbound (broadcasting task events). There is no inbound event handling from Agent Teams. The `internal/coldwine/storage/coordination.go` `ReservePaths()` function does exact string matching on paths (line 258: `WHERE path = ? AND released_ts IS NULL AND expires_ts > ?`) with no glob overlap detection.

**Suggestion:** Define an `AgentTeamsClient` interface in `pkg/contract/` (not `pkg/` directly, since this is a cross-tool contract type) that abstracts the detection mechanism. This follows the existing pattern where `ResearchProvider` in `/root/projects/Autarch/internal/gurgeh/arbiter/intermute.go` abstracts the Intermute research integration. The recommended approach is option 3 (Coldwine as the claim API via MCP tools) because:

1. It avoids coupling Coldwine to Agent Teams internals
2. It matches the existing project pattern where tools expose MCP operations (see `autarch-plugin/` and AC-4.7's MCP tool requirement)
3. It degrades gracefully: when Agent Teams is unavailable, users call the same MCP tools manually

Add AC-4.11:
> When a teammate claims a task via the `autarch_claim_task` MCP tool, Coldwine acquires Intermute file reservations for the task's target paths within 2 seconds.

This resolves the bridge mechanism by making it explicit and testable.

---

### Issue 2: Dual Reservation Systems Create Redundancy and Inconsistency Risk

**Location:** CUJ-4 (AC-4.1 through AC-4.4), Coldwine storage layer

**Problem:** There are two independent reservation systems:

1. **Coldwine's local reservations** in `/root/projects/Autarch/internal/coldwine/storage/coordination.go` using SQLite (`.coldwine/state.db`)
2. **Intermute's reservations** via `/root/projects/Autarch/pkg/intermute/client.go` `Reserve()` method, which calls the external Intermute server

The plan's AC-4.1 says "File reservation acquired before task work begins; reservation logged with TTL in Intermute" but Coldwine already has its own `ReservePaths()` function (line 239 of `coordination.go`) that creates local reservations in SQLite. If both are active, there is no synchronization between them.

Critically, neither system performs glob overlap detection:
- Coldwine's `ReservePaths()` checks `WHERE path = ?` (exact match only)
- Intermute's `Reserve()` (per the plan's Gap 1) does a simple INSERT with no overlap check

This means AC-4.2 ("Overlapping reservation request rejected") is untestable in both systems.

**Suggestion:** The plan correctly identifies this as a blocking gap (Gap 1). The resolution should be:

1. **Intermute owns reservation enforcement** (it is the documented coordination layer per `docs/ARCHITECTURE.md`). Add glob overlap detection to Intermute's `Reserve()` -- this is a change to the Intermute repo at `/root/projects/Intermute`.

2. **Coldwine's local `ReservePaths()` becomes the fallback** for when Intermute is unavailable (the "Intermute OFF" cell in the degradation matrix). Document explicitly that the local system does NOT do glob overlap detection and state the trade-off.

3. Do not attempt to synchronize two reservation stores. This follows the existing pattern where each tool has a nil-client graceful degradation mode (see `internal/coldwine/intermute/broadcaster.go` line 59-61: `if b.sender == nil { return nil }`).

---

### Issue 3: Signal Transport Path Contradicts Documented Architecture

**Location:** CUJ-3 (AC-3.4), Research Insights Gap 4, Open Question 10

**Problem:** The plan says signals "broadcast through Intermute" (Requirements section), but the actual code in `/root/projects/Autarch/pkg/signals/broker.go` is a self-contained in-memory fan-out with its own WebSocket server. It has no connection to Intermute. Meanwhile, Bigend's aggregator at `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` only subscribes to Intermute WebSocket events (lines 176-191). It does not subscribe to the Signals server.

This means:
- **`SignalResearchInvalidation` published via `pkg/signals/Broker.Publish()` will never reach Bigend** because Bigend only listens to Intermute events.
- **Signal types in `pkg/signals/signal.go`** (8 signal types including `SignalCompetitorShipped`, `SignalResearchInvalidation`, etc.) exist in a package that Bigend does not consume.

The two systems serve overlapping but distinct purposes:
- `pkg/signals/` - Typed domain alerts with dismissal state
- `pkg/intermute/` - Generic domain events (`spec.created`, `task.updated`, etc.)

**Suggestion:** Adopt the plan's own recommendation (Open Question 10, Option A): **Signals publish through Intermute as messages with a `signal.` prefix.** This means:

1. Add a `SignalBridge` in `pkg/signals/` that wraps `pkg/intermute.Client` and publishes signals as Intermute messages with subjects like `signal.research_invalidation`.
2. Bigend's aggregator already handles `*` wildcard events from Intermute (line 171), so `signal.*` events would be automatically dispatched.
3. The in-memory `Broker` in `pkg/signals/broker.go` remains useful for in-process subscribers (the TUI) but is not the transport to Bigend.

This avoids introducing a second WebSocket subscription in Bigend and follows the existing pattern where Intermute is the canonical cross-tool transport.

Add AC-3.11:
> `SignalResearchInvalidation` emitted by Pollard reaches Bigend's dashboard within 5 seconds via Intermute event transport. The signal appears in the activity feed with type `signal.research_invalidation`.

---

### Issue 4: SaveRevision Atomicity Is Better Than Plan Claims (But Still Has a Gap)

**Location:** CUJ-1 (AC-1.15), Research Insights "Data Integrity Risks"

**Problem:** The plan's Research Insights section states "SaveRevision has non-atomic two-file writes and mutates input spec version as side effect." This is partially incorrect based on the actual code at `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`.

The code shows:
1. **File locking is present** (line 53-62): `fileutil.LockFile()` provides serialized access per spec ID, preventing concurrent writers from picking the same version.
2. **Atomic file writes are used** (line 88, 98): `fileutil.AtomicWriteFile()` uses write-to-temp-then-rename (confirmed in `/root/projects/Autarch/internal/file/atomic.go`), which is crash-safe on Linux/ext4.
3. **Best-effort rollback exists** (line 99): If the revision metadata write fails, the snapshot file is removed.
4. **Version mutation is on a copy** (line 79): `snapshot := *spec` creates a shallow copy, and `snapshot.Version = version` mutates the copy, not the original. However, the original `spec.Version` field IS NOT mutated -- the plan's claim is wrong.

The remaining gap is:
- If the process crashes between snapshot write (line 88) and revision metadata write (line 98), you get an orphaned snapshot without metadata. The `LoadHistory()` function (line 153) only reads `_rev.yaml` files, so the orphaned snapshot is invisible but wastes disk space.

**Suggestion:** The plan should correct the SaveRevision assessment. The actual concern is narrower than stated: crash-between-two-writes leaving orphaned snapshots. This is a minor data integrity issue, not a correctness one. AC-1.15 as written ("verify 3 snapshots exist") is adequate, but consider adding a recovery check:

> AC-1.15a: `gurgeh history` reports correct version count even after simulated crash between snapshot and metadata writes.

---

### Issue 5: Research Coverage Target (>80%) Contradicts Code Reality (4 of 8 Phases)

**Location:** CUJ-1 (AC-1.13), Research Insights "Performance Analysis"

**Problem:** AC-1.13 requires "research coverage >80%" but `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` only defines research configurations for 4 of 8 phases: Vision, Problem, FeaturesGoals, and Requirements. The `ResearchConfigForPhase()` function (line 53) returns `nil` for Users, CUJs, ScopeAssumptions, and AcceptanceCriteria.

The plan's own Timing Thresholds table (line 315) acknowledges this: "Research coverage: >60% (4 of 8 phases have configs)." But AC-1.13 still says >80%.

The `researchQuality()` function in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (line 856) computes research quality based on finding count, source diversity, and average relevance -- not phase coverage. A single comprehensive GitHub Scout run could produce >80% "research quality" even though only 1 of 8 phases triggered research.

**Suggestion:** The plan should reconcile the two numbers. Options:
1. Change AC-1.13 to use the actual `researchQuality()` formula: >60% research quality score
2. Define "research coverage" explicitly as the percentage of phases with at least one linked finding (not hunter trigger rate)
3. Add research configs for the missing 4 phases (Users -> community hunters, CUJs -> workflow hunters, Scope -> inverse research, AC -> test pattern hunters) as AC-1.12 already expects

Option 3 is most consistent with AC-1.12, which already describes hunters for all 8 phases. The code in `research_phases.go` just needs to match.

---

### Issue 6: Degradation Matrix Has an Untested Cell That Is Architecturally Dangerous

**Location:** Degradation Matrix, AC-X.5, AC-X.9

**Problem:** The matrix correctly identifies four cells but the "Agent Teams ON + Intermute OFF" cell is marked "DEGRADED" with the note "Core CUJ-4 value lost." This cell is architecturally dangerous and untested.

In this scenario:
- Agent Teams spawns teammates who claim tasks
- Coldwine cannot acquire Intermute reservations (Intermute is down)
- Teammates proceed with "unprotected" warning
- File conflicts become possible

The plan acknowledges this but has no AC covering it. Meanwhile, the "Agent Teams OFF + Intermute ON" cell IS covered by AC-2.10 and AC-4.10.

**Suggestion:** Add explicit AC for the dangerous cell:

> AC-X.11: When Agent Teams is active but Intermute is unavailable, Coldwine logs a warning at startup and displays "Unprotected: file reservation enforcement unavailable" in the TUI header. Teammates can still claim tasks but reservation-dependent ACs (AC-4.1 through AC-4.4) are skipped.

> AC-X.12: When Agent Teams is active but Intermute is unavailable, file reservation MCP tools (`autarch_reserve_paths`, `autarch_release_paths`) return a clear error message explaining Intermute is required.

---

### Issue 7: Signal Deduplication Location Needs Architectural Decision

**Location:** CUJ-3 (AC-3.4a), Research Insights Gap 3

**Problem:** The plan correctly identifies that signal deduplication is purely in-memory in the Broker (line 51-54 of `broker.go`: `select { case sub.ch <- sig: default: }`). The `(spec_id, type, affected_field)` unique constraint exists in the Signal struct definition (`signal.go` line 37: `AffectedField string`) but is not enforced anywhere.

The architectural question is: where should deduplication live?

1. **In the Broker (`pkg/signals/`):** Adds state to what is currently a stateless fan-out. The Broker would need a dedup store (in-memory set or SQLite).
2. **In the publisher (each tool):** Each tool checks before publishing. Distributed dedup, harder to enforce.
3. **In Intermute:** If signals route through Intermute (per Issue 3 recommendation), Intermute can enforce the unique constraint server-side.

**Suggestion:** If the signal-through-Intermute transport is adopted (Issue 3), deduplication belongs in Intermute's message handling (Option 3). Intermute already has SQLite storage and can enforce the unique constraint atomically. This avoids adding stateful dedup to the stateless `pkg/signals/Broker`.

If signals remain separate from Intermute, use Option 1 with an in-memory LRU set in the Broker. The Broker is a singleton per process, so this is effective for single-process dedup. Add TTL-based expiry (e.g., suppress duplicate signals for 1 hour).

---

### Issue 8: `pkg/db/open.go` MaxOpenConns(1) Will Bottleneck Under Agent Teams

**Location:** Research Insights "Data Integrity Risks", CUJ-4

**Problem:** The plan notes that `MaxOpenConns(1)` serializes all SQLite reads and writes, and with 3+ teammates writing to `.coldwine/state.db`, `SQLITE_BUSY` errors become likely. This is a real concern but needs to be evaluated against Autarch's actual SQLite usage pattern.

Examining the architecture:
- Each tool has its own SQLite database (`.coldwine/state.db`, `.pollard/state.db`, `~/.autarch/events.db`)
- Agent Teams teammates are separate processes that would each open their own connection to `.coldwine/state.db`
- `MaxOpenConns(1)` limits connections within a single process, not across processes
- WAL mode (documented in `AGENTS.md`) allows concurrent readers but serializes writers

The actual bottleneck is cross-process write contention, not in-process connection pooling. `MaxOpenConns(1)` is the correct setting for a single-process writer; the issue is that Agent Teams creates multiple writers to the same database file.

**Suggestion:** This is a real architectural concern but the plan misdiagnoses it. The fix is not increasing `MaxOpenConns` (which only affects per-process pooling). The fix is ensuring Coldwine is the single writer to `.coldwine/state.db` and teammates update state through Coldwine's MCP API rather than writing directly. This is consistent with Issue 1's recommendation (Coldwine as the claim API).

Add AC:
> AC-4.12: Under concurrent load (3 teammates updating task state simultaneously via MCP tools), no `SQLITE_BUSY` errors occur. Coldwine serializes writes through its single-process connection.

---

### Issue 9: The Plan's Confidence Score Components Do Not Match the Code

**Location:** CUJ-1 (AC-1.8)

**Problem:** AC-1.8 states the confidence score displays "four components (Completeness, Consistency, Specificity, Research)" but the `ConfidenceScore` struct in `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` (line 93-99) has FIVE components:

```go
type ConfidenceScore struct {
    Completeness float64 // 0-1, weight: 20%
    Consistency  float64 // 0-1, weight: 25%
    Specificity  float64 // 0-1, weight: 20%
    Research     float64 // 0-1, weight: 20%
    Assumptions  float64 // 0-1, weight: 15%
}
```

The `Assumptions` component (15% weight) is missing from AC-1.8. This means either the AC is incomplete or the code has an extra component. Looking at the `Total()` method (line 102-107), all five are used in the weighted sum.

**Suggestion:** Update AC-1.8 to reference five components: Completeness, Consistency, Specificity, Research, and Assumptions. The Assumptions component is especially important because it tracks assumption decay (`CheckAssumptionDecay` in `evolution.go`), which is a core differentiator of this system.

---

### Issue 10: AC-1.12 Hunter Sequence Contradicts Code and Institutional Learning

**Location:** CUJ-1 (AC-1.12), Institutional Learnings item 5

**Problem:** AC-1.12 says "Vision -> GitHub Scout+HackerNews, Problem -> arXiv+OpenAlex..." but the plan's own Institutional Learnings section (item 5, line 288) notes: "Quick scan moved to Users phase (from oracle-review-issues): Changes when research evidence is available -- affects AC-1.12 hunter trigger sequence."

The code in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (line 336) confirms: `if state.Phase == PhaseUsers { ... o.runQuickScanBackground(bgCtx) }` -- the quick scan triggers when advancing TO the Users phase, not at Vision.

Meanwhile, `research_phases.go` maps:
- Vision -> `github-scout`, `hackernews-trendwatcher`
- Problem -> `arxiv-scout`, `openalex`
- FeaturesGoals -> `competitor-tracker`, `github-scout`
- Requirements -> `github-scout`

There are no mappings for Users, CUJs, Scope, or Acceptance -- contradicting AC-1.12's claim of per-phase hunter selection for all 8 phases.

**Suggestion:** AC-1.12 should be split:

> AC-1.12a: Phase-specific hunters trigger for the 4 mapped phases: Vision (GitHub Scout, HackerNews), Problem (arXiv, OpenAlex), Features (Competitor Tracker, GitHub Scout), Requirements (GitHub Scout). Verify via log pane output.

> AC-1.12b: Quick scan triggers automatically when advancing to Users phase. Verify via log pane output showing scan activity.

> AC-1.12c (deferred to v2): Add hunter mappings for Users, CUJs, Scope, and Acceptance phases per the CUJ-1 description.

This makes the AC testable against the actual code rather than the aspirational description.

---

## Cross-Cutting Observations

### What the Plan Gets Right

1. **Degradation matrix is excellent.** The 2x2 Agent Teams x Intermute matrix is a well-structured way to ensure all configurations are tested. This is the right level of analysis for a system with two optional coordination layers.

2. **Separation of correctness from performance.** The Timing Thresholds table (line 296) correctly splits each metric into "Correctness Target" (event ordering, generous timeout) and "Performance Budget" (p95 benchmark). This will eliminate flaky tests.

3. **Negative/failure path testing.** The plan explicitly includes failure scenarios for each CUJ (hunter failure, spec corruption, feedback corruption, TTL expiry, project disappearing). Many AC plans omit these.

4. **Institutional learnings are well-integrated.** The plan references specific solutions from `docs/solutions/` and translates them into concrete AC requirements (race flag, error message routing, phase propagation).

5. **SaveRevision is more robust than assessed.** The code uses file locking, atomic writes, and best-effort rollback -- the plan's research agents over-stated the fragility. This is a case where reading the actual code (which has been iteratively hardened) reveals better engineering than a static analysis would predict.

### Hidden Coupling Risks

1. **Coldwine -> Agent Teams file format dependency:** If Coldwine polls `~/.claude/teams/` files directly, it couples to an experimental feature's internal format. The recommended interface adapter (`AgentTeamsClient`) mitigates this.

2. **Pollard -> Gurgeh spec structure dependency:** Pollard's feedback YAML (`.pollard/feedback.yaml`) references spec sections by name. If Gurgeh's phase names change (which they did -- CUJs and Requirements were swapped per the `types.go` comment), feedback references become stale. Consider using phase IDs instead of names.

3. **Bigend -> Coldwine SQLite direct read:** Bigend reads Coldwine's SQLite via `coldwine.NewReader()` (aggregator.go line 416-419, 726-740). This creates a tight coupling to Coldwine's schema. If Coldwine changes its schema, Bigend breaks. The existing pattern of reading YAML files is safer; consider exposing task stats via Intermute queries instead.

### Circular Dependency Check

No circular dependencies detected in the module boundary analysis. The dependency graph is:

```
Bigend --reads--> Gurgeh, Coldwine, Pollard, Intermute (all read-only)
Coldwine --reads--> Gurgeh (specs), Pollard (insights), Intermute (reservations, events)
Gurgeh --reads--> Pollard (research), Intermute (specs, insights)
Pollard --writes--> .pollard/ only, reads nothing from other tools
```

Pollard has zero inbound dependencies from other tools at the code level, which is architecturally clean. All Pollard integration goes through `.pollard/` files or Intermute.

---

## Summary

**Overall architecture fit: Acceptable, with three issues requiring resolution before implementation.**

### Top 3 Changes to Improve the Plan

1. **Resolve the Coldwine-Agent Teams bridge mechanism (Issue 1).** This is the single biggest architectural gap. Recommend making Coldwine the authority via MCP tools (`autarch_claim_task`), avoiding polling of Agent Teams internals. Add AC-4.11 with an explicit detection mechanism and latency target.

2. **Unify the signal transport path (Issue 3).** Signals currently cannot reach Bigend because Bigend subscribes to Intermute events, not the separate Signals broker. Route signals through Intermute with a `signal.` prefix. Add AC-3.11 requiring end-to-end signal delivery verification.

3. **Reconcile research coverage targets with code reality (Issue 5 + Issue 10).** AC-1.13's >80% target and AC-1.12's 8-phase hunter mapping do not match the code's 4-phase research configuration. Either add the missing 4 phase configs or adjust the ACs to match reality.

### Secondary Improvements

4. Fix AC-1.8 to include all 5 confidence components (Issue 9)
5. Add AC for the "Agent Teams ON + Intermute OFF" degradation cell (Issue 6)
6. Clarify that Coldwine should be the single SQLite writer, not Agent Teams teammates (Issue 8)
7. Correct the SaveRevision atomicity assessment -- the code is more robust than claimed (Issue 4)
