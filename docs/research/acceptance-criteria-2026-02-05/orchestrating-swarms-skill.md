# Orchestrating swarms skill

**Source:** Agent result from session b817bfb0
**Task ID:** a04f8f6
**Date:** 2026-02-05

---

I now have all the information needed to produce a thorough analysis. Here are my findings.

---

## Swarm Orchestration Analysis: Acceptance Criteria Plan vs. Skill Patterns

### 1. Does the Plan's Agent Teams Integration Match the Skill's Recommended Patterns?

**Partial match with significant structural gaps.**

The plan correctly identifies the core primitives from the skill -- Leader, Teammate, Task, Inbox, Message, and Backend -- and maps them to Coldwine concepts. However, the mapping has several misalignments:

**What matches well:**
- **Leader/Teammate roles** (CUJ-4): The plan's "lead assigns tasks to teammates who self-claim unblocked work" directly mirrors the skill's Pattern 3 (Self-Organizing Swarm) and Pattern 6 (Coordinated Multi-File Refactoring). AC-2.8 and AC-4.8 correctly describe this pattern.
- **Plan approval gating** (AC-2.9): Maps directly to the skill's Pattern 5 (Plan Approval Workflow) and the `approvePlan`/`rejectPlan` operations. This is well-specified.
- **Task dependencies** (AC-2.6): The skill's `addBlockedBy` / auto-unblock mechanism (Pattern 2: Pipeline) maps to the plan's dependency DAG.
- **Graceful degradation** (AC-2.10, AC-4.10, AC-X.9): The tmux fallback when Agent Teams unavailable aligns with the skill's tmux spawn backend.

**What diverges or is incomplete:**

| Skill Pattern | Plan Coverage | Gap |
|---|---|---|
| `spawnTeam` creates `~/.claude/teams/{name}/` + task dir | Not specified in any AC | No AC verifies the team file structure is created or compatible with Coldwine's hierarchy |
| Task file structure at `~/.claude/tasks/{team}/N.json` is flat (id, subject, status, owner, blockedBy) | Plan uses Initiative->Epic->Story->Task hierarchy | Open Question #6 acknowledges this but no AC tests the flattening or round-trip |
| `idle_notification` auto-sent when teammate stops | Not referenced | No AC tests idle detection from Agent Teams' perspective; AC-5.6 only covers heuristic stall detection |
| `requestShutdown` -> `approveShutdown` -> `cleanup` sequence | AC-4.9 says "shut down teammate, verify reservation cleared" | No AC verifies the full 3-step graceful shutdown sequence from the skill |
| Teammate `write` vs `broadcast` distinction | AC-4.5 mentions "teammate messaging" generically | No AC tests the directed `write` vs. `broadcast` semantics or the cost implications |
| Backend auto-detection (in-process vs tmux vs iterm2) | Plan assumes tmux | No AC verifies behavior across different spawn backends, particularly `in-process` which is the default on most systems |
| Environment variables (`CLAUDE_CODE_TEAM_NAME`, `CLAUDE_CODE_AGENT_ID`, etc.) | Not mentioned | No AC verifies that Coldwine can read or use these env vars to identify which teammate is making a reservation request |

### 2. Missing Orchestration Patterns

The skill document defines several patterns that the plan either lacks or under-specifies:

**A. Task Queue and Backpressure**

The skill's Pattern 3 (Self-Organizing Swarm) has workers that "race to claim tasks, naturally load-balance." The plan's CUJ-4 describes teammates who "self-claim unblocked work" but has **no backpressure mechanism**:

- **No AC for concurrent claim races.** Two teammates simultaneously claiming the same task via `TaskUpdate` is a race condition. The skill relies on file-level atomicity of `~/.claude/tasks/{team}/N.json`. The plan does not test this.
- **No AC for overloaded task queue.** If Coldwine generates 50 tasks and only 3 teammates exist, there is no mechanism to throttle task creation, batch teammate spawning, or signal to the lead that the pool is saturated.
- **No idle/retry loop.** The skill's swarm prompt includes an explicit idle detection loop: "Wait 30 seconds, try again, up to 3 times, then exit." The plan has no equivalent AC for teammate behavior when no tasks are available.

**B. Graceful Shutdown**

This is the most significant gap. The skill explicitly documents:

1. `requestShutdown` for all teammates
2. Wait for `shutdown_approved` messages
3. Verify no active members
4. Only then `cleanup`

And warns: "Will fail if teammates are still active."

The plan's AC-4.9 says "shut down teammate, verify reservation cleared, assign to new teammate" but does **not** test:

- What happens if a teammate rejects shutdown (`rejectShutdown` -- "Still working on task #3, need 5 more minutes")
- What happens if cleanup is called while teammates are still active
- What happens if a teammate crashes mid-work (skill says 5-minute heartbeat timeout, tasks reclaimed)
- Proper sequencing of reservation release BEFORE vs AFTER shutdown approval

**Recommended additions:**
- AC-4.11: Teammate that rejects shutdown continues working; lead receives rejection reason
- AC-4.12: `cleanup` fails gracefully if teammates still active; error message names active members
- AC-4.13: Crashed teammate's reservations auto-expire at TTL; tasks reclaimable after 5-minute heartbeat timeout

**C. Permission Requests**

The skill documents `permission_request` structured messages -- when a teammate needs Bash access or other sandboxed tools, it sends a permission request to the leader. The plan has no coverage for this. In a parallel development context, teammates need to run `go test`, `git commit`, etc. If sandbox restrictions block this, the lead must approve. No AC exists for this flow.

**D. Message Acknowledgment and Reliability**

The skill uses JSON inbox files where messages have a `read` flag. Coldwine's own coordination layer (`/root/projects/Autarch/internal/coldwine/storage/coordination.go`) has richer messaging with `AckRequired`, `read_ts`, and `ack_ts` fields. But there is no bridge between these two messaging systems:

- Agent Teams messages go to `~/.claude/teams/{name}/inboxes/{agent}.json`
- Coldwine messages go to SQLite tables (`messages` + `mailboxes`)
- Intermute events go to the Intermute REST/WebSocket API

AC-4.5 says "Teammate-to-teammate messaging delivers within 5 seconds via Agent Teams' native mailbox; Coldwine broadcasts task lifecycle events via Intermute for Bigend" -- but this describes **two separate, unbridged messaging paths**. No AC verifies that a message sent via Agent Teams' inbox is reflected in Coldwine's coordination layer, or vice versa.

### 3. How Should Coldwine's Bridge Mechanism Work Given the Skill's Primitives?

The plan identifies this as its most critical architectural gap (Gap 2, Open Question #8). Here is a concrete analysis based on the skill's actual primitives:

**The fundamental problem:** Agent Teams' task state lives in JSON files (`~/.claude/tasks/{team}/N.json`). Coldwine's task state lives in SQLite (`.coldwine/state.db`). When a teammate calls `TaskUpdate({ taskId: "3", owner: "worker-1" })`, the JSON file is updated atomically, but Coldwine has no notification.

**Option A: File Watching (recommended by plan's research)**

Watch `~/.claude/tasks/{team}/*.json` via `fsnotify`. On change, parse the JSON, detect claim events, and call Intermute `Reserve()`.

Pros:
- Reactive, low latency (<1s)
- No modification to Agent Teams internals

Cons:
- `fsnotify` is platform-dependent and sometimes misses events (especially on NFS or in Docker)
- Polling fallback needed
- Race condition: file change detected, reservation acquired, but another teammate already started writing

**Option B: Prompt-Driven (skill's native pattern)**

Include reservation logic in the teammate's prompt itself. When spawning teammates, Coldwine injects instructions:

```
Before starting any task, call the MCP tool `autarch_reserve_paths` with the task's file patterns.
If reservation fails (blocked), do NOT proceed. Mark task blocked and try another.
After completing task, call `autarch_release_paths`.
```

This aligns with the skill's Pattern 3 (swarm workers with explicit claim/complete loops in prompts).

Pros:
- No external bridge mechanism needed
- Reservation is synchronous and before work begins (exactly what AC-4.8 requires)
- Natural fit with the skill's prompt-based coordination
- Teammates are self-coordinating, matching the swarm pattern

Cons:
- Relies on prompt compliance (teammates might forget/skip)
- No enforcement if teammate bypasses MCP tools
- Need MCP tools to be available (AC-4.7 covers this)

**Option C: Wrapper/Interposer**

Coldwine wraps the `TaskUpdate` call so that claiming a task first acquires reservations.

Pros:
- Atomic claim + reserve
- Enforcement, not just advice

Cons:
- Requires modifying Agent Teams' task system or providing an alternative endpoint
- Agent Teams is a Claude Code internal -- modifying it is fragile

**Recommendation: Option B (prompt-driven) as primary, Option A (file watching) as verification.**

The skill's entire model is prompt-driven coordination. Teammates are told what to do. Coldwine should:
1. Inject reservation instructions into teammate prompts at spawn time
2. Provide MCP tools (`autarch_reserve_paths`, `autarch_release_paths`, `autarch_check_reservation`)
3. Run a file watcher as a **verification layer** that logs warnings if a teammate starts work without holding a reservation

This produces two new acceptance criteria:
- AC-4.14: Teammate spawn prompt includes reservation instructions; teammate calls `autarch_reserve_paths` before first file modification
- AC-4.15: File watcher detects task claim in `~/.claude/tasks/` within 2 seconds; logs warning if no corresponding Intermute reservation exists

### 4. Best Patterns for Reservation Lifecycle Management in a Swarm Context

**Current state of reservations (confirmed from code):**

Intermute's `Reserve()` at `/root/projects/Intermute/internal/storage/sqlite/sqlite.go:813` performs a bare INSERT with **no overlap check** -- confirming the plan's Gap 1. Two agents can acquire `internal/auth/**/*.go` and `internal/**/*.go` simultaneously.

Coldwine's local `ReservePaths()` at `/root/projects/Autarch/internal/coldwine/storage/coordination.go:239` does exact string matching on the `path` column -- `WHERE path = ? AND released_ts IS NULL AND expires_ts > ?`. This catches only identical path strings, not overlapping globs.

Neither system detects glob overlaps.

**Recommended reservation lifecycle for swarms:**

```
Teammate spawns
  |
  v
TaskList() -> find pending task
  |
  v
TaskUpdate(claim) 
  |
  v
autarch_reserve_paths(task.file_patterns)
  |
  +--[GRANTED]-----> Begin work
  |                    |
  |                    v
  |                  autarch_release_paths() 
  |                    |
  |                    v
  |                  TaskUpdate(completed)
  |                    |
  |                    v
  |                  [Reservation auto-released on completion]
  |
  +--[CONFLICT]----> TaskUpdate(blocked, reason="files held by {holder}")
                       |
                       v
                     Try next pending task
                       |
                       v
                     [No tasks? idle_notification to lead]
```

**Key lifecycle events needing ACs:**

| Event | Current AC | Gap |
|---|---|---|
| Claim -> reserve | AC-4.8 | Missing: what if reserve fails but claim succeeded? Need rollback. |
| Complete -> release | AC-4.4a | Missing: release must happen before or atomically with completion |
| Shutdown -> release | AC-4.9 | Missing: what if release fails during shutdown? |
| TTL expiry during work | AC-4.4c | Missing: what signal does the working agent receive? Plan mentions 80% TTL warning but no AC |
| Crash -> reclaim | Not covered | Skill says 5-min heartbeat. Reservation has separate TTL. Need to test: crashed agent's reservation TTL < heartbeat timeout = another agent could claim files while crashed agent's task is still "in_progress" |
| Reservation renewal | Not covered | Coldwine's `RenewReservations()` exists but no AC tests it. Long-running tasks need periodic renewal. |

**Recommended additions:**
- AC-4.16: Failed reservation after successful task claim triggers automatic task unclaim (rollback)
- AC-4.17: Agent receives warning at 80% of reservation TTL; can renew via `autarch_renew_reservation`
- AC-4.18: Reservation TTL > heartbeat timeout (prevents file access gap between crash detection and task reclaim)
- AC-4.19: Two agents requesting overlapping globs (`internal/auth/**/*.go` vs `internal/**/*.go`) -- second request rejected with conflict detail (BLOCKED by Gap 1)

### 5. Are the Acceptance Criteria Sufficient to Test Swarm Coordination?

**Verdict: No. The criteria test the happy path and some failure paths, but miss the swarm-specific coordination challenges.**

**What is well covered:**
- Basic reservation acquire/release lifecycle (AC-4.1, AC-4.4)
- Conflict detection and blocking visibility (AC-4.2, AC-4.3)
- Messaging delivery timing (AC-4.5)
- MCP tool availability (AC-4.7)
- Automatic reservation on task claim (AC-4.8)
- Fallback to tmux (AC-4.10)
- Race condition testing for overlapping reservations (Race Condition section: 100 iterations)

**What is missing:**

**Swarm Coordination Gaps:**

| Category | Missing Coverage | Recommended AC |
|---|---|---|
| **Concurrent task claiming** | Two teammates race to claim same task | AC-4.20: Concurrent task claim resolves to single owner; loser retries on next pending task |
| **Graceful shutdown sequence** | Full requestShutdown -> approveShutdown -> cleanup flow | AC-4.11, AC-4.12, AC-4.13 (detailed above) |
| **Idle behavior** | Teammate with no available tasks | AC-4.21: Teammate with no claimable tasks sends idle_notification and waits; does not loop infinitely |
| **Reservation-claim atomicity** | Reserve fails after claim succeeds | AC-4.16 (detailed above) |
| **TTL renewal** | Long-running tasks | AC-4.17 (detailed above) |
| **TTL vs heartbeat ordering** | Crash recovery timing | AC-4.18 (detailed above) |
| **Glob overlap semantics** | Subset/superset pattern conflicts | AC-4.19 (BLOCKED by Gap 1) |
| **Prompt-driven coordination** | Teammate follows reservation instructions in prompt | AC-4.14 (detailed above) |
| **Verification layer** | File watcher detects unguarded work | AC-4.15 (detailed above) |
| **Backend portability** | in-process vs tmux behavior differences | AC-4.22: Reservation lifecycle works identically across in-process and tmux backends |
| **Team cleanup** | Resources cleaned after all work done | AC-4.23: After all tasks completed, `cleanup` removes team config and task files; Intermute reservations all released |
| **Message routing** | Agent Teams inbox vs Coldwine mailbox | AC-4.24: Critical task lifecycle events visible in both Agent Teams inbox (for teammates) and Intermute events (for Bigend) |
| **Spawn overhead** | Team creation + teammate spawn timing | AC-4.25: Full team spawn (create team + 3 teammates + task population) completes in <30s |

**Cross-CUJ Gaps Related to Swarm Coordination:**

The plan's "CUJ Transition Testing" section mentions CUJ-2->4 ("From task hierarchy to Agent Team spawn") but has no specific AC for the transition. The skill's lifecycle is:

1. Create team (`spawnTeam`)
2. Create tasks (`TaskCreate` from Coldwine hierarchy)
3. Spawn teammates
4. Work + coordinate
5. Shutdown
6. Cleanup

Steps 1-3 are the CUJ-2->4 transition, and they need an AC that verifies the full handoff: Coldwine hierarchy -> flattened Agent Teams tasks -> teammates spawned with reservation-aware prompts -> first task claimed.

### Summary of Findings

| Finding | Severity | Action |
|---|---|---|
| No glob overlap detection in Intermute `Reserve()` | **BLOCKING** | Must implement before CUJ-4 is testable (plan already identifies this) |
| No graceful shutdown sequence ACs | **HIGH** | Add AC-4.11 through AC-4.13 for the skill's shutdown protocol |
| Bridge mechanism unspecified | **HIGH** | Recommend prompt-driven (Option B) + file watcher verification; add AC-4.14, AC-4.15 |
| Reservation-claim atomicity untested | **HIGH** | Add AC-4.16 for rollback on failed reservation |
| No concurrent claim race testing | **MEDIUM** | Add AC-4.20 |
| TTL renewal not tested | **MEDIUM** | Add AC-4.17 |
| No idle/backpressure behavior tested | **MEDIUM** | Add AC-4.21 |
| Backend portability untested | **MEDIUM** | Add AC-4.22 |
| Three separate messaging paths unbridged | **MEDIUM** | Add AC-4.24 |
| Team cleanup verification missing | **LOW** | Add AC-4.23 |
| Environment variable usage unverified | **LOW** | Could be covered by AC-4.14 |

**Overall assessment:** The plan is thorough for a traditional multi-process coordination system, but it does not fully internalize the skill's prompt-driven, file-based, inbox-messaging model. The plan treats Agent Teams as an external system to integrate with, rather than recognizing that the skill's swarm patterns ARE the coordination mechanism. The recommended approach is to make teammates self-coordinating via MCP tools in their prompts, with Coldwine serving as the reservation authority (via Intermute) and the verification/monitoring layer, rather than trying to build an event-driven bridge that detects and reacts to Agent Teams state changes.