# Acceptance Criteria Plan -- User Flow Analysis & Gap Review

**Analyzed:** 2026-02-06
**Source:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Analyzer:** Spec Flow Analyzer (UX Flow & Requirements Engineering)

---

## Executive Summary

The acceptance criteria plan is thorough in its CUJ-internal coverage and commendably identifies critical implementation gaps (glob overlap, Agent Teams bridge, signal deduplication, transport ambiguity). However, the analysis reveals **23 missing or under-specified user flows**, **14 unaddressed edge cases**, and **8 gaps in the degradation matrix**. The most significant structural weakness is the absence of inter-CUJ transition flows -- the plan describes what happens inside each CUJ well but does not specify the user experience of moving between them, particularly in failure or partial-completion scenarios.

---

## Phase 1: User Flow Overview

### Complete User Journey Map

```
                          First Launch
                               |
                    ./dev autarch tui
                               |
                    +----------+----------+
                    |                     |
              New User             Returning User
                    |                     |
              Onboarding           Resume Sprint?
                    |              /           \
                    v            Yes            No
              CUJ-1: PRD        |              |
              Creation     Resume Sprint   New Sprint
                    |              \           /
                    |               +----+----+
                    v                    v
              CUJ-1: 8-Phase Sprint Flow
              (Vision -> Acceptance)
                    |
        +-----------+-----------+
        |           |           |
   Research    Task Gen      Export
   (CUJ-3)    (CUJ-2)       (.md/.yaml)
        |           |
        v           v
   CUJ-3:      CUJ-2: Task
   Validation   Breakdown
        |           |
        |     +-----+-----+
        |     |           |
        |  With Agent   Without
        |  Teams        Agent Teams
        |     |           |
        |     v           v
        |  CUJ-4:    Manual tmux
        |  Parallel   Sessions
        |  Dev            |
        |     |           |
        +-----+-----+----+
              |
              v
         CUJ-5: Mission Control
         (Observes all above)
```

### Flow 1: CUJ-1 Internal -- Research-Enriched PRD Creation

**Entry Points:**
1. `./dev autarch tui` (full onboarding)
2. `./dev autarch tui --tool=gurgeh` (direct to Gurgeh tab)
3. `./dev autarch tui --skip-onboard` (dashboard, then navigate to Gurgeh)
4. Resume interrupted sprint

**Sub-flows:**
1. Kickoff -> codebase scan (blocking) + Pollard research (async)
2. 8-phase sprint: Vision -> Problem -> Users -> Features -> CUJs -> Requirements -> Scope -> Acceptance
3. Per-phase: Proposal -> Accept/Chat-Revise -> Consistency Check -> Confidence Update -> Advance
4. Research triage: Finding appears -> Navigate to Pollard tab -> Sidebar filter -> Select finding -> Agent triage -> Action (Accept/Reject/Defer/Deep Dive)
5. Handoff: Export spec / Generate tasks / Deep research

**Decision Points:**
- Accept or revise each phase draft
- Triage each research finding (4 actions)
- Choose handoff action post-sprint
- Skip unreviewed findings (affects confidence) or review them

### Flow 2: CUJ-2 Internal -- PRD to Task Breakdown

**Entry Points:**
1. CUJ-1 handoff ("Generate Tasks")
2. `./dev coldwine init` from CLI
3. Navigate to Coldwine tab with completed spec

**Sub-flows:**
1. Read spec -> Propose hierarchy -> Review -> Accept/Edit/Reject per item
2. Register file patterns as reservations (Intermute or local SQLite)
3. Optionally spawn Agent Team -> Populate shared task list
4. Plan approval gating for teammates

### Flow 3: CUJ-3 Internal -- Continuous Research Validation

**Entry Points:**
1. CUJ-1 handoff ("Deep Research")
2. Pollard watch cycle triggers automatically
3. User manually triggers research from Pollard tab

**Sub-flows:**
1. Watch cycle -> Hunter runs -> New findings detected
2. Contradiction detection -> Signal emission -> Sidebar indicator
3. Deep Dive request -> Targeted research -> Results return
4. Feedback loop: Agent reads feedback.yaml -> Preferences applied -> Rankings adjusted

### Flow 4: CUJ-4 Internal -- Parallel Agent Development

**Entry Points:**
1. CUJ-2 handoff (spawn Agent Team from hierarchy)
2. Manual team creation from Coldwine tab

**Sub-flows:**
1. Spawn team -> Lead assigns tasks -> Teammates self-claim
2. Claim -> Reserve files -> Work -> Complete -> Release
3. Conflict detection -> Blocked status -> Reassignment
4. Handoff messaging between teammates

### Flow 5: CUJ-5 Internal -- Multi-Project Mission Control

**Entry Points:**
1. `./dev bigend` (web mode)
2. Bigend tab in unified TUI
3. `./dev autarch tui --tool=bigend`

**Sub-flows:**
1. Project discovery -> Metrics aggregation -> Dashboard render
2. Real-time WebSocket updates
3. Drill-down: Project -> Tool tab (Gurgeh/Coldwine/Pollard)
4. Agent session state detection (Agent Teams or heuristic)

---

## Phase 2: Flow Permutations Matrix

### CUJ-1 Permutation Matrix

| Dimension | Variant A | Variant B | Variant C | Covered in AC? |
|-----------|-----------|-----------|-----------|----------------|
| **Entry** | Fresh start | Resume sprint | From existing research | A: Yes, B: Partially (resume mentioned in AGENTS.md but no AC), C: Yes (AC StartWithResearch) |
| **Research** | Pollard available | Pollard unavailable | Partial (some hunters fail) | A: Yes, B: No AC, C: AC-1.16 (rate limit only) |
| **Agent pane** | Agent connected | Agent unavailable | Agent disconnects mid-triage | A: Yes, B: AC-X.6, C: **No AC** |
| **Findings** | 0 findings | 1-10 findings | 50+ findings | All implied but **no pagination AC** |
| **Confidence** | >70% | 50-70% | <50% | Only >70% has AC-1.13; warning at <50% is "recommended" but no AC |
| **Terminal** | >=120 cols | <120 cols | Resize during sprint | A: AC-X.3, B: AC-X.4, C: **No AC** |
| **Intermute** | Available | Unavailable | Crashes mid-sprint | A: Implied, B: AC-X.5, C: **No AC** |

### CUJ-2 Permutation Matrix

| Dimension | Variant A | Variant B | Variant C | Covered in AC? |
|-----------|-----------|-----------|-----------|----------------|
| **Spec input** | Complete spec | Incomplete spec (some phases empty) | Spec with 0 CUJs | A: Yes, B: **No AC**, C: Negative test only |
| **Agent Teams** | Enabled | Disabled | Becomes unavailable mid-flow | A: AC-2.8, B: AC-2.10, C: **No AC** |
| **Intermute** | Available | Unavailable | Crashes during reservation | A: Yes, B: **No AC (should be covered by degradation)**, C: **No AC** |
| **Hierarchy depth** | 1 level (just tasks) | 4 levels (initiative/epic/story/task) | 100+ tasks | A: Implied, B: Implied, C: **No AC for scale** |

### CUJ-3 Permutation Matrix

| Dimension | Variant A | Variant B | Variant C | Covered in AC? |
|-----------|-----------|-----------|-----------|----------------|
| **Timing** | Watch cycle | Manual trigger | Phase-triggered | A: AC-3.1, B: AC-3.5, C: AC-1.12 |
| **Feedback file** | Fresh (no history) | Existing (50 decisions) | Corrupted | A: Implied, B: AC-3.9, C: Negative test |
| **Global prefs** | None | Exists, no conflict | Exists, conflicts with project | A: Implied, B: Implied, C: AC-3.7 |
| **Contradiction** | None | Single | Duplicate (same tuple) | A: N/A, B: AC-3.3, C: AC-3.4a |
| **Finding volume** | 0 per cycle | 1-5 | 100+ | A: Implied, B: Implied, C: **No AC** |

### CUJ-4 Permutation Matrix

| Dimension | Variant A | Variant B | Variant C | Covered in AC? |
|-----------|-----------|-----------|-----------|----------------|
| **Team size** | 1 teammate | 3 teammates | Max teammates | A: Implied, B: Implied, C: **No AC for limits** |
| **File conflicts** | No overlap | Exact path overlap | Glob subset/superset overlap | A: AC-4.1, B: AC-4.2, C: **Gap 1 blocking** |
| **TTL** | Normal completion | TTL expiry during work | TTL renewal | A: AC-4.4a, B: Negative test, C: **No AC** |
| **Network** | Stable | Intermute drops mid-work | Agent Teams drops mid-work | A: Implied, B: **No AC**, C: **No AC** |

### CUJ-5 Permutation Matrix

| Dimension | Variant A | Variant B | Variant C | Covered in AC? |
|-----------|-----------|-----------|-----------|----------------|
| **Projects** | 1 project | 3+ projects | 0 projects (empty scan) | A: Implied, B: AC-5.1, C: **No AC** |
| **Mode** | Web (:8099) | TUI | Both simultaneously | A: Implied, B: Implied, C: **No AC** |
| **Agent Teams** | Active | Inactive | Becomes active during session | A: AC-5.7, B: AC-5.8, C: **No AC** |
| **WebSocket** | Connected | Disconnected | Reconnect after failure | A: AC-5.3, B: **No AC**, C: **No AC** |

---

## Phase 3: Missing Elements & Gaps

### Category: Inter-CUJ Transitions

**Gap T-1: CUJ-1 to CUJ-2 handoff UX is under-specified**

The plan mentions a CUJ-1->2 transition test ("Completing PRD export surfaces handoff options") but does not specify:
- What happens if the user selects "Generate Tasks" but Coldwine is not initialized
- Whether the user stays in the Gurgeh tab or switches to Coldwine
- Whether the spec is automatically passed or the user must re-select it
- What happens if the spec export fails mid-handoff

Impact: Users will experience a jarring or broken transition between the two most important sequential flows.

Current ambiguity: The `GetHandoffOptions` method in `orchestrator.go` returns 4 options (research, tasks, spec, export) but the TUI handling of selection and tab switching is not covered by any AC.

**Gap T-2: CUJ-1 to CUJ-3 activation is implicit**

AC says "Research monitoring activation after sprint completion" but:
- Is the Pollard watch started automatically or does the user opt in?
- What is the initial watch interval -- does it inherit from the sprint or use the 24h default?
- If the user closes the TUI after CUJ-1, does monitoring continue? (It should, since Pollard watch is a background process, but this is not stated.)

Impact: Users may not realize research monitoring is active, or may expect it when it is not.

**Gap T-3: CUJ-2 to CUJ-4 spawn path is undefined**

The transition from "tasks ready" to "Agent Team spawned" requires:
- Selecting which tasks to assign to the team
- Configuring team size
- Confirming token cost implications (per open question 7)
- The actual spawn mechanism (Coldwine UI button? CLI command? Slash command?)

None of these steps have acceptance criteria.

**Gap T-4: CUJ-3 + CUJ-4 interaction during parallel development**

The plan mentions "Research invalidation during parallel development surfaces as blocked indicator on affected tasks" but:
- How does the system map a research invalidation signal to specific tasks? The signal has `AffectedField` (a spec field like "goals") but tasks are linked to epics/stories, not directly to spec fields.
- Does the blocked indicator require the lead to pause the teammate, or is it informational only?
- What if a teammate has already started work on the invalidated task?

Impact: The most complex real-world scenario (research changes during active development) has the least specification.

**Gap T-5: CUJ-5 interaction with active flows**

The plan describes Bigend as read-only observational, but:
- Can the user interact with a project from Bigend (e.g., drill down and start triaging findings)?
- Can the user spawn an Agent Team from Bigend's mission control view?
- What happens when the user drills into a project -- does it navigate within the unified TUI (tab switch) or open a new context?

AC-5.4 says "Drilling into project navigates to appropriate tool tab" but this implies Bigend is part of the unified TUI. The web mode at `:8099` cannot switch TUI tabs.

### Category: Error States & Recovery

**Gap E-1: Agent pane disconnects mid-triage**

AC-X.6 covers agent unavailability (button fallback) but not:
- What happens to an in-progress triage conversation when the agent disconnects
- Whether partial triage state is preserved
- Whether the button fallback can complete an operation that was started conversationally

Impact: Loss of work during triage is frustrating and could lead to inconsistent feedback.yaml state.

**Gap E-2: Sprint interruption and resume**

The AGENTS.md documents sprint persistence (auto-save to `.gurgeh/sprints/`) and the code implements `SaveSprintState`/`LoadSprintState`, but no AC covers:
- Resuming a sprint after a crash
- Resuming with stale research data
- Resuming when the codebase has changed since the sprint started
- Multiple interrupted sprints (which to resume?)

Impact: This is a core "unhappy path" for the primary CUJ-1 flow. Power failures, SSH disconnects, and terminal crashes are common for solo developers.

**Gap E-3: Concurrent TUI + CLI access**

The plan assumes single-user single-session, but a user could:
- Have the TUI open while running `gurgeh export` from CLI
- Have two TUI instances (one per tmux pane) pointing at the same project
- Run `coldwine init` from CLI while the TUI is showing Gurgeh handoff

The YAML and SQLite files are shared. No locking or conflict detection is specified for concurrent local access.

Impact: File corruption or conflicting state, especially for the non-atomic operations identified in the plan's own research.

**Gap E-4: Intermute process crash during reservation hold**

The plan covers TTL expiry and explicit release but not:
- Intermute crashes while an agent holds reservations -- do reservations survive restart? (They should, since they are in SQLite with WAL mode.)
- Intermute restarts with a different state (e.g., database loss) -- agents believe they hold reservations that no longer exist
- What if Coldwine queries Intermute for reservations and gets an empty set because Intermute was restarted?

Impact: Silent reservation loss could allow the exact file conflicts that CUJ-4 exists to prevent.

**Gap E-5: SQLite BUSY under concurrent agent writes**

The plan identifies this risk ("MaxOpenConns(1) in `pkg/db/open.go`") but provides no AC to verify behavior under contention. The `<100ms` p99 write target exists in the timing summary but has no corresponding AC.

Impact: With 3+ teammates writing to `.coldwine/state.db`, SQLITE_BUSY errors will surface as mysterious failures in task state transitions.

### Category: Data Integrity

**Gap D-1: Feedback rolling window crash recovery has no AC**

Negative test says "Feedback.yaml malformed -- agent logs warning, starts with empty preferences, doesn't overwrite." But:
- What about the archive-then-truncate atomicity? A crash between writing `feedback-archive/` and truncating `feedback.yaml` creates duplicates.
- No AC verifies that duplicates are detected and handled on next startup.
- No AC verifies concurrent triage sessions don't interleave YAML writes.

Impact: Gradually corrupted preferences that degrade research ranking quality over time.

**Gap D-2: Spec export includes all deferred findings but not rejected ones**

AC-1.11 says "Exported spec includes 'Future Considerations' section containing all deferred findings with original reasoning." But:
- Are rejected findings included anywhere in the export for auditability?
- If a finding was accepted then later rejected (via a subsequent finding that contradicts it), what happens?
- If the user defers 50 findings, does the Future Considerations section become unmanageably long?

Impact: Loss of decision history (rejection reasoning) limits the utility of the feedback loop.

**Gap D-3: Agent Teams / Coldwine state reconciliation**

The plan notes "If a teammate marks done in Agent Teams while Coldwine is unreachable, states diverge. No conflict detection or resolution mechanism defined." This has no AC and no negative test.

Impact: Task status becomes unreliable -- the primary value proposition of Coldwine.

### Category: Performance & Scalability

**Gap P-1: No AC for large codebases**

AC-1.1 targets "within 10 seconds" but the plan's own research says this is unrealistic for 144K LOC repos. The revised target splits structural scan (<5s) from LLM exploration (<90s) but no AC reflects this split.

Impact: Test will fail for the primary use case (real codebases, not test fixtures).

**Gap P-2: No AC for high finding volume**

The plan implies bounded finding lists but:
- What if a full Pollard scan returns 200+ findings? Is there pagination?
- What if all 200 are high-relevance (>0.8)? The badge would show `Pollard (200)`.
- Performance of the sidebar rendering with hundreds of findings is not tested.

Impact: Information overload defeats the purpose of research enrichment.

**Gap P-3: No AC for WebSocket reconnection**

AC-5.3 covers "within 2 seconds" latency but:
- What happens when the WebSocket connection drops?
- Is there automatic reconnection with backoff?
- What state is lost during the disconnection period?

Impact: Any network hiccup makes the mission control dashboard stale with no recovery path.

### Category: Security

**Gap S-1: Reservation ownership verification has no AC**

The plan identifies F2 (HIGH) but no AC requires ownership verification. `ForceReleaseReservation` in `coordination.go` takes an `id` parameter but no `owner` -- any caller can release any reservation.

Impact: The core isolation guarantee of CUJ-4 is unenforceable.

**Gap S-2: YAML bomb protection has no AC**

The plan identifies F5 (MEDIUM) but no AC verifies schema validation or size limits on feedback.yaml parsing.

Impact: A single malformed YAML file can crash the agent or cause unbounded memory consumption.

### Category: Accessibility & Usability

**Gap U-1: Screen reader support unmentioned**

The plan targets solo developers but does not mention accessibility. The TUI uses lipgloss styling, icons (bullet, triangle, circle), and color-coded status indicators.

Impact: Visually impaired developers cannot use the tool.

**Gap U-2: No AC for inline mode (`--inline`)**

The `--inline` flag "preserves terminal scrollback" but no AC verifies:
- Log pane behavior in inline mode
- Research findings rendering in inline mode
- Whether all CUJ-1 flows work identically in inline mode vs fullscreen

Impact: Inline mode is documented as a feature but untested.

**Gap U-3: No AC for the chat-first editing paradigm**

The plan and AGENTS.md emphasize "chat-first editing" (no edit mode, users refine by chatting) but no AC verifies:
- That revisions via chat actually update the phase content
- That multi-phase propagation (via `PropagateChanges`) works correctly
- That the user can see what changed after a chat revision

The `ProcessChatMessage` method in `orchestrator.go` does all of this, but the test coverage is through code, not through ACs.

---

## Phase 4: Critical Questions Requiring Clarification

### Critical (Blocks implementation or creates data/security risks)

**Q1: How does Coldwine detect Agent Teams task claims?**
Already identified as Open Question 8 and Gap 2. The plan recommends file-watching `~/.claude/teams/` but no AC exists. This is the linchpin of CUJ-4.

Why it matters: Without this, CUJ-4 is untestable and the Coldwine-Agent Teams bridge is fictional.
Assumption if unanswered: File-watching with 1-second polling, mockable via `AgentTeamsClient` interface.

**Q2: What is the canonical signal transport path?**
Already identified as Open Question 10 and Gap 4. The `Emitter.ResearchInvalidation()` at `/root/projects/Autarch/internal/pollard/signals/emitter.go` creates a `Signal` struct, but the Broker at `/root/projects/Autarch/pkg/signals/broker.go` is in-memory only with no bridge to Intermute. The Bigend aggregator at `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` connects to Intermute WebSocket (line 166-188) but does not subscribe to the Signals server.

Why it matters: AC-3.4 says "visible in Signals tab" and the CUJ-3+4 transition says "surfaces as blocked indicator on affected tasks." Neither is achievable without a defined transport path.
Assumption if unanswered: Signals publish through Intermute as messages with `signal.` type prefix.

**Q3: What happens to reservations when Intermute crashes and restarts?**
The SQLite database should persist through process restarts (WAL mode), but:
- Is the database location fixed or configurable?
- Do agents re-validate their held reservations on reconnect?
- Does Coldwine have a "reconciliation" flow to sync reservation state?

Why it matters: Without reconciliation, reservation state can diverge from reality after any process restart.
Assumption if unanswered: SQLite persists; no reconciliation; agents trust their local state.

**Q4: What is the AC for glob overlap detection?**
Gap 1 is identified as blocking but the recommended AC-4.2a through AC-4.2c are described in prose, not added to the AC table. The current `ReservePaths` at `/root/projects/Autarch/internal/coldwine/storage/coordination.go` (line 239-306) uses exact string matching (`WHERE path = ?`), not glob expansion.

Why it matters: Without this, the entire reservation system provides a false sense of security.
Assumption if unanswered: Implementation will use `doublestar.Match()` or similar for glob expansion before reservation check.

### Important (Significantly affects UX or maintainability)

**Q5: What is the sprint resume flow after TUI crash or network disconnect?**
The code has `Resume()` in orchestrator.go and `SaveSprintState` auto-saves on phase transitions. But:
- Is resume offered automatically on next launch?
- How does the user choose between multiple interrupted sprints?
- What if the saved state references an exploration session that has expired?

Why it matters: SSH disconnects are extremely common for the target audience (solo developers with remote servers). Sprint resume is a daily occurrence, not an edge case.
Assumption if unanswered: List interrupted sprints on launch; user selects; exploration re-runs if session expired.

**Q6: What happens when a user switches tabs mid-sprint?**
The unified TUI has 4 tabs (Bigend, Gurgeh, Coldwine, Pollard). During an active Gurgeh sprint:
- Does the sprint auto-pause when the user switches to Pollard?
- Does the sprint continue in the background?
- What if the user switches to Coldwine and tries to init tasks from an incomplete sprint?

Why it matters: Tab switching is the primary navigation pattern (Phase 1 of unified TUI navigation plan). Users will switch tabs frequently.
Assumption if unanswered: Sprint continues; state preserved; Coldwine init rejects incomplete specs.

**Q7: How are high-relevance finding notifications delivered in non-Pollard tabs?**
AC-1.3 and AC-1.4 describe the Pollard badge and pulse, but:
- Is the badge visible when the user is in the Gurgeh tab?
- Is it visible in the Coldwine tab?
- The header tabs show `Pollard (3)` -- is this always rendered regardless of current tab?

Why it matters: The user spends most of their time in Gurgeh during CUJ-1. If findings are only visible after tab-switching, they will be missed.
Assumption if unanswered: Badge is always visible in the tab header bar, which is rendered in all views.

**Q8: What is the feedback.yaml schema and what are its size bounds?**
The plan says "rolling window of last 50 decisions" (AC-3.9) and "~500 tokens on session start." But:
- What is the YAML schema? (Fields: action, reasoning, timestamp, finding_id, relevance_score, affected_sections?)
- Is 50 decisions the hard cap, or is it configurable?
- What is the max file size before performance degrades?
- Is the archive file ever pruned?

Why it matters: This file is the persistence layer for the entire feedback loop (CUJ-3). Its schema is the contract between the agent, the triage system, and the preference engine.
Assumption if unanswered: Schema matches the prose description; 50 is configurable via `.pollard/config.yaml`; archive grows unbounded.

**Q9: What does "research coverage >80%" mean precisely?**
AC-1.13 targets ">80% research coverage" but the `researchQuality()` function at `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (line 856-908) uses a weighted formula: 30% finding count (capped at 10), 30% source diversity (capped at 3 sources), 40% average relevance. Only 4 of 8 phases have research configs in `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go`.

This means:
- Maximum diversity score is 1.0 (3+ sources out of 3 cap)
- Maximum count score is 1.0 (10+ findings)
- Average relevance of 0.5 (default for unscored findings) gives 0.2
- Total: 0.3 + 0.3 + 0.2 = 0.8 (exactly 80%)

To exceed 80%, you need high-relevance findings (avg >0.5). With random-quality free-tier API results, this is not guaranteed.

Why it matters: The >80% target may be unachievable with default scoring. The plan's own timing summary revises this to >60% ("4 of 8 phases have configs") but AC-1.13 still says >80%.
Assumption if unanswered: AC-1.13 target should be revised to >60% to match the timing summary, or the `researchQuality()` formula should be recalibrated.

### Nice-to-Have (Improves clarity but has reasonable defaults)

**Q10: Should the MCP tool surface be defined as ACs?**
Open Question 5 recommends "full CRUD for all 7 entities" but the plan only has AC-1.17 (finding tools) and AC-4.7 (reservation tools). Should each MCP tool have its own AC, or is "MCP tools exist and work" sufficient?

Assumption if unanswered: Only the two existing ACs; expand post-v1.

**Q11: What is the minimum number of 20+ transitions required for AC-5.5 validation?**
AC-5.5 says ">95% with Agent Teams (test 20+ transitions, verify >19 correct)" but:
- What transition types? (working->waiting, waiting->blocked, blocked->done, etc.)
- Must each transition type be tested, or just 20 arbitrary transitions?
- How is "correct" determined -- by human observation or automated oracle?

Assumption if unanswered: 20 arbitrary transitions covering all state values; correctness judged by human.

**Q12: Does the plan need ACs for the CLI-only workflows?**
Several features have CLI equivalents: `gurgeh history`, `gurgeh diff`, `gurgeh prioritize`, `pollard scan`, `coldwine status`. None of these have ACs. Are they covered by existing tool-specific tests or do they need acceptance criteria?

Assumption if unanswered: CLI workflows are covered by unit/integration tests in each tool's package; ACs focus on TUI flows.

---

## Phase 5: Degradation Matrix Completeness Analysis

### Current Matrix (from plan)

| Scenario | Agent Teams ON | Agent Teams OFF |
|----------|---------------|-----------------|
| **Intermute ON** | Full capability | Manual sessions + Intermute |
| **Intermute OFF** | DEGRADED | MINIMAL |

### Missing Dimensions

The matrix covers Agent Teams x Intermute but omits:

**1. Pollard availability dimension**
- Pollard hunters can fail independently (rate limits, network errors, API changes)
- Pollard being completely unavailable is different from individual hunter failures
- This affects CUJ-1 (research enrichment), CUJ-3 (validation), and CUJ-5 (research coverage metrics)

**2. Agent pane availability dimension**
- Agent connected vs disconnected affects CUJ-1 triage flow
- AC-X.6 covers fallback but the degradation is not in the matrix

**3. SQLite availability dimension**
- If `.coldwine/state.db` is locked or corrupted, CUJ-2 and CUJ-4 fail completely
- If `.gurgeh/specs/` is read-only (e.g., disk full), CUJ-1 export fails

**4. WebSocket availability dimension (for CUJ-5)**
- Intermute may be running but its WebSocket endpoint may be unreachable
- This is a partial degradation not captured by the binary ON/OFF matrix

### Expanded Matrix (Recommended)

| Intermute | Agent Teams | Pollard | Agent Pane | Affected CUJs | Severity |
|-----------|-------------|---------|------------|----------------|----------|
| ON | ON | ON | ON | None | Full capability |
| ON | ON | OFF | ON | CUJ-1,3 | Research degraded; sprint works |
| ON | ON | ON | OFF | CUJ-1 | Triage via buttons only |
| ON | OFF | ON | ON | CUJ-2,4 | Manual sessions; reservations work |
| ON | OFF | OFF | ON | CUJ-1,2,3,4 | No research, no agents; reservations work |
| OFF | ON | ON | ON | CUJ-4,5 | No reservations; agents coordinate but no enforcement |
| OFF | ON | OFF | ON | CUJ-1,3,4,5 | No research, no reservations |
| OFF | OFF | ON | ON | CUJ-2,4,5 | No reservations, no agents; manual only |
| OFF | OFF | OFF | OFF | All | PRD sprint only (no enrichment, no tasks, no monitoring) |

### Untested Cell: Agent Teams ON + Intermute OFF

The plan acknowledges this cell is untested. Specific behaviors that need definition:
- Teammates can claim tasks via Agent Teams shared list, but no reservation enforcement
- Coldwine should display "unprotected" warning per the matrix
- What does Bigend show for reservation state? Empty? "Intermute unavailable"?
- Does the lead get notified that isolation is not enforced?

---

## Phase 6: Code-Level Verification of Key Claims

### Claim: "SaveRevision has non-atomic two-file writes" -- PARTIALLY RESOLVED

The actual code at `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` (lines 42-105) shows:
1. File locking via `LockFile()` at line 53 -- this serializes concurrent writers
2. `AtomicWriteFile()` for both snapshot and revision metadata (lines 88, 98) -- these use write-to-temp-then-rename (verified in `/root/projects/Autarch/internal/file/atomic.go`)
3. Best-effort rollback: if metadata write fails, snapshot is removed (line 100)
4. Input spec is NOT mutated -- a `snapshot := *spec` copy is made (line 79)

The test at `/root/projects/Autarch/internal/gurgeh/specs/evolution_test.go` (lines 69-133) verifies concurrent writers produce unique versions.

**Assessment:** The plan's research insight about non-atomic writes appears to have been addressed by the code changes visible in the test file. The file locking and atomic writes are implemented. However, the rollback at line 100 (`os.Remove(snapPath)`) leaves a window where the snapshot exists but metadata does not -- if the process crashes after the snapshot write but before the metadata write. This is a minor residual risk.

### Claim: "Signal deduplication is purely in-memory" -- CONFIRMED

The broker at `/root/projects/Autarch/pkg/signals/broker.go` has zero deduplication logic. It is a pure fan-out with silent drop on slow subscribers (line 51-54). The `(spec_id, type, affected_field)` unique constraint described in Requirements exists nowhere in the codebase.

### Claim: "ReservePaths uses exact string matching" -- CONFIRMED

The Coldwine `ReservePaths` at `/root/projects/Autarch/internal/coldwine/storage/coordination.go` (line 258) uses `WHERE path = ?` for conflict detection. This means:
- `internal/auth/**/*.go` does NOT conflict with `internal/auth/jwt/*.go`
- `internal/**/*.go` does NOT conflict with `internal/auth/**/*.go`

This is a confirmed blocking gap for CUJ-4.

### Claim: "Bigend only connects to Intermute WebSocket, not Signals server" -- CONFIRMED

The aggregator at `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` (line 155-195) connects to Intermute via `a.intermuteClient.Connect()` and subscribes to Intermute event types. There is no subscription to the Signals broker's WebSocket endpoint. The Signals broker at `/root/projects/Autarch/pkg/signals/broker.go` has its own `ServeWS()` method (line 60-81) but no code calls this from Bigend.

### Claim: "Only 4 of 8 phases have research configs" -- CONFIRMED

The file at `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` configures research for:
1. PhaseVision (github-scout, hackernews-trendwatcher)
2. PhaseProblem (arxiv-scout, openalex)
3. PhaseFeaturesGoals (competitor-tracker, github-scout)
4. PhaseRequirements (github-scout)

Missing: PhaseUsers, PhaseCUJs, PhaseScopeAssumptions, PhaseAcceptanceCriteria.

This means AC-1.12 ("Phase-appropriate hunters trigger automatically" for all 8 phases) is unimplementable without adding research configs for the remaining 4 phases. The AC lists specific hunters for all 8 phases (Users->community analysis, CUJs->workflow patterns, Scope->inverse research, Acceptance->test patterns) but the code only supports 4.

---

## Phase 7: Recommended Next Steps

### Blocking (Must resolve before implementation)

1. **Add glob overlap detection AC** -- Formalize AC-4.2a through AC-4.2c as entries in the CUJ-4 table with concrete test cases:
   - `internal/auth/**/*.go` vs `internal/auth/jwt/*.go` = CONFLICT
   - `internal/auth/**/*.go` vs `internal/billing/**/*.go` = NO CONFLICT
   - `internal/**/*.go` vs `internal/auth/**/*.go` = CONFLICT (superset)

2. **Define Coldwine-Agent Teams bridge mechanism** -- Add AC-4.11 specifying the detection mechanism (file-watching, polling, or event hook) with latency target (<2s from claim to reservation).

3. **Resolve signal transport path** -- Choose option (a), (b), or (c) from Open Question 10 and add AC-3.11 verifying end-to-end signal delivery from Pollard emitter to Bigend dashboard.

4. **Add missing research phase configs** -- Either add hunter mappings for the 4 missing phases, or revise AC-1.12 to specify only the 4 implemented phases, or mark Users/CUJs/Scope/Acceptance as "manual research only" phases.

### High Priority (Should resolve before implementation starts)

5. **Add sprint resume ACs** -- At minimum: AC-1.18 (resume interrupted sprint from `.gurgeh/sprints/`), AC-1.19 (resume with stale exploration triggers re-scan), AC-1.20 (multiple interrupted sprints shown as selectable list).

6. **Add inter-CUJ transition ACs** -- Formalize the 4 transition tests in the "CUJ Transition Testing" section as numbered ACs with specific observable behaviors and tab switching expectations.

7. **Revise research coverage target** -- Change AC-1.13 from ">80%" to ">60%" to align with the timing summary, or recalibrate the `researchQuality()` formula.

8. **Add degradation ACs for Agent Teams ON + Intermute OFF** -- This is the only matrix cell with no corresponding AC. Add AC-X.11 requiring "unprotected" warning and graceful operation without reservation enforcement.

9. **Add concurrent access ACs** -- At minimum: AC-X.12 (two TUI instances pointing at same project don't corrupt state), AC-X.13 (CLI export during active TUI sprint doesn't corrupt spec file).

### Medium Priority (Should resolve before testing)

10. **Define feedback.yaml schema** -- Create a formal schema definition (JSON Schema or Go struct with YAML tags) and add AC-3.10 verifying schema conformance.

11. **Add pagination AC for findings** -- AC-1.18 (or similar): "Pollard sidebar supports scrolling through 100+ findings without performance degradation."

12. **Add WebSocket reconnection AC** -- AC-5.9: "Bigend automatically reconnects WebSocket within 5 seconds of disconnection with exponential backoff."

13. **Add SQLite contention AC** -- AC-4.11 (or similar): "3 concurrent teammate writes to `.coldwine/state.db` complete within 100ms p99 without SQLITE_BUSY errors."

### Low Priority (Can defer to v2)

14. Add accessibility ACs for screen reader compatibility.
15. Add inline mode ACs (AC-X.14: all CUJ-1 flows work in `--inline` mode).
16. Add MCP tool CRUD completeness audit (all 7 entities x 4 operations).
17. Add reservation ownership verification AC (security F2).
18. Add YAML bomb protection AC (security F5).

---

## Appendix A: Files Referenced in This Analysis

| File | Purpose | Key Findings |
|------|---------|--------------|
| `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md` | Source plan | 5 CUJs, 10 cross-cutting ACs, 10 open questions |
| `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` | Sprint flow control | Implements Start, Advance, Accept, Revise, Handoff, ChatMessage |
| `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` | Phase-hunter mapping | Only 4 of 8 phases configured |
| `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` | Spec versioning | Uses file locking and atomic writes (improved from plan's claim) |
| `/root/projects/Autarch/internal/gurgeh/specs/evolution_test.go` | Versioning tests | Concurrent write safety verified |
| `/root/projects/Autarch/pkg/signals/broker.go` | Signal fan-out | No deduplication, silent drop on slow subscribers |
| `/root/projects/Autarch/pkg/signals/signal.go` | Signal types | 8 types including ResearchInvalidation |
| `/root/projects/Autarch/internal/pollard/signals/emitter.go` | Signal creation | Creates Signal structs but no delivery path to Intermute/Bigend |
| `/root/projects/Autarch/internal/coldwine/storage/coordination.go` | Reservation system | Exact string matching (no glob expansion), no ownership verification on release |
| `/root/projects/Autarch/internal/coldwine/coordination/compat.go` | Coordination layer | Wraps storage.ReservePaths with request/response types |
| `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` | Dashboard aggregation | Connects to Intermute WebSocket only, not Signals server |
| `/root/projects/Autarch/internal/file/atomic.go` | Atomic file writes | Proper write-to-temp-then-rename with fsync |
| `/root/projects/Autarch/AGENTS.md` | Dev guide | Sprint timing 20-40 min without research |

---

## Appendix B: AC Gap Summary Table

| Gap ID | Category | Severity | Description | Recommended AC ID |
|--------|----------|----------|-------------|-------------------|
| T-1 | Transition | Important | CUJ-1->2 handoff UX unspecified | AC-T.1 |
| T-2 | Transition | Important | CUJ-1->3 activation implicit | AC-T.2 |
| T-3 | Transition | Important | CUJ-2->4 spawn path undefined | AC-T.3 |
| T-4 | Transition | Critical | CUJ-3+4 signal-to-task mapping undefined | AC-T.4 |
| T-5 | Transition | Nice-to-have | CUJ-5 drill-down behavior (web vs TUI) | AC-T.5 |
| E-1 | Error | Important | Agent disconnect mid-triage | AC-1.18 |
| E-2 | Error | Critical | Sprint resume after crash | AC-1.19-1.21 |
| E-3 | Error | Important | Concurrent TUI+CLI access | AC-X.12-13 |
| E-4 | Error | Critical | Intermute crash during held reservations | AC-4.11 |
| E-5 | Error | Important | SQLite BUSY under contention | AC-4.12 |
| D-1 | Data | Important | Feedback rolling window crash recovery | AC-3.10 |
| D-2 | Data | Nice-to-have | Rejected findings in export | AC-1.22 |
| D-3 | Data | Critical | Agent Teams / Coldwine state reconciliation | AC-4.13 |
| P-1 | Performance | Critical | Large codebase scan target unrealistic | Revise AC-1.1 |
| P-2 | Performance | Important | High finding volume pagination | AC-1.23 |
| P-3 | Performance | Important | WebSocket reconnection | AC-5.9 |
| S-1 | Security | Critical | Reservation ownership verification | AC-4.14 |
| S-2 | Security | Medium | YAML bomb protection | AC-3.11 |
| U-1 | Usability | Nice-to-have | Screen reader support | Defer |
| U-2 | Usability | Important | Inline mode ACs | AC-X.14 |
| U-3 | Usability | Important | Chat-first editing ACs | AC-1.24 |
| Matrix-1 | Degradation | Important | Agent Teams ON + Intermute OFF untested | AC-X.11 |
| Matrix-2 | Degradation | Important | Pollard dimension missing from matrix | Expand matrix |
