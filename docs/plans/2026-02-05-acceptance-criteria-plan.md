# Acceptance Criteria Plan for Autarch PRD

## Enhancement Summary

**Deepened on:** 2026-02-05
**Research agents used:** 11 (agent-native architecture, spec flow analyzer, security sentinel, performance oracle, architecture strategist, data integrity guardian, pattern recognition, best practices researcher, Agent Teams docs researcher, learnings researcher, code simplicity reviewer)

### Key Improvements
1. Split timing-based criteria into correctness (deterministic) + performance (benchmarked) to eliminate flaky tests
2. Added negative/failure-path criteria for every CUJ (hunter failure, spec corruption, feedback corruption, TTL expiry during work, project disappearing)
3. Added CUJ transition criteria (CUJ-1→2, CUJ-1→3, CUJ-2→4, CUJ-3+4 interaction)
4. Identified critical implementation gap: Intermute `Reserve()` has no glob overlap detection — CUJ-4 isolation is untestable without it
5. Added degradation matrix covering all 4 combinations of Agent Teams × Intermute availability
6. Added race condition and atomicity criteria for concurrent agent writes (spec versioning, feedback rolling window, signal deduplication)
7. Added institutional learnings: `-race` flag mandatory, error message routing verification, phase propagation testing

### New Considerations Discovered
- Coldwine ↔ Agent Teams bridge mechanism (polling vs events vs wrapping) is architecturally critical and unspecified
- Signal transport path ambiguity: Signals server vs Intermute broadcasting are separate systems with no bridge
- `SaveRevision` has non-atomic two-file writes and mutates input spec version as side effect
- WebSocket origin pattern `*` in Bigend allows cross-origin connections from any website on same machine
- Feedback YAML files are writable by any local process and directly influence agent behavior (preference poisoning risk)

---

## Overview

This plan defines testable acceptance criteria for each CUJ in the Autarch PRD. Criteria are observable, measurable, and map directly to the success conditions specified in each user journey.

---

## Prior Context (Approved Phases)

### Vision
A system for producing excellent, research-informed PRDs that remain the canonical source of truth throughout development. Pollard provides strategic research and competitive intelligence. Gurgeh transforms that research into detailed, high-quality specifications with versioned history and structured revision tracking. Coldwine and Bigend ensure specs stay synchronized with execution, preventing drift between what was specified and what gets built.

### Problem
Modern AI-assisted development is fragmented: research evaporates in chat windows, specifications drift from reality, task breakdowns are manual and disconnected from specs, agents work without broader context, and observation requires manually checking multiple sources. Each gap loses context, forgets decisions, and duplicates work.

### Users
Solo developers who leverage AI coding assistants (Claude, Codex, Cursor, Aider) to build products faster—turning ideas into working software through natural language collaboration rather than traditional line-by-line coding.

### Features
Four integrated tools: Pollard (multi-domain research hunters for GitHub, arXiv, HackerNews, PubMed, OpenAlex, etc. that enrich all Arbiter sprint phases), Gurgeh (TUI-first PRD generation with 8-phase Arbiter sprints where each phase can trigger targeted research, plus consistency checking and confidence scoring), Coldwine (task orchestration bridging Claude Code Agent Teams for agent lifecycle with Intermute for file reservation enforcement and cross-tool event broadcasting), and Bigend (mission control dashboard for observing all agents/projects across web and TUI modes, reading Agent Teams config for authoritative session state).

### CUJs

**CUJ-1: Research-Enriched PRD Creation** — A solo developer with a product idea opens the unified TUI (`./dev autarch tui`), triggering Gurgeh's Kickoff phase which scans the codebase for existing structure, tech stack, and dependencies. The Arbiter Sprint then guides them through 8 phases, with Pollard research enriching each: Vision triggers market trend hunters (GitHub Scout, HackerNews), Problem triggers academic research (arXiv, OpenAlex) to validate uniqueness, Users triggers community analysis to validate personas, Features triggers competitor tracking for parity analysis, CUJs triggers workflow pattern extraction from similar products, Requirements triggers implementation pattern search, Scope triggers inverse research ("what did competitors NOT build in v1?"), and Acceptance triggers test pattern discovery. As hunters return findings, the doc pane's "Research Findings" section updates with relevance-ranked insights (⚔📊👤 icons), the sidebar badge increments for high-relevance items, and the log pane streams hunter activity. The user can select any finding to expand details inline, or ask the chat pane "how does this affect our vision?" to get contextual synthesis. The consistency engine flags conflicts while a running confidence score (0-100%) shows PRD strength. Success: validated PRD saved to `.gurgeh/specs/` in under 25 minutes with confidence >70%, research coverage >80%, and no unresolved blockers.

**CUJ-2: PRD to Task Breakdown** — With a completed PRD, the developer initializes Coldwine which reads the spec and proposes an Initiative → Epic → Story → Task hierarchy derived from PRD features. The user accepts or edits proposals through the TUI, then optionally spawns a Claude Code Agent Team—Coldwine populates the team's shared task list from its hierarchy and the lead assigns tasks to teammates. Before a teammate begins work, Coldwine acquires Intermute file reservations for the task's target paths (using glob patterns like `internal/auth/**/*.go`). Each reservation has a TTL and prevents other agents from claiming overlapping paths. Plan approval gating ensures teammates propose their approach before modifying code; the lead reviews against spec intent. Success: dependency DAG computed, tasks ready for agent assignment within 5-10 minutes; Agent Teams optional (manual session management still supported).

**CUJ-3: Continuous Research Validation** — As the developer iterates on their PRD, Pollard monitors for research invalidation signals. When new findings contradict PRD assumptions (e.g., a competitor ships a feature assumed to be unique), Pollard emits a `SignalResearchInvalidation` that surfaces as a sidebar attention indicator (● with warning color) rather than a disruptive modal. The doc pane's Research Findings section highlights the invalidating insight at the top with an explanation of which assumption it affects. The developer can trigger deep research at any phase—Pollard auto-selects relevant hunters based on phase content and synthesizes insights with relevance scores (0.0-1.0) linked to specific spec sections. The chat pane provides context: "New competitor finding may affect your 'unique differentiator' claim—want me to analyze alternatives?" After reviewing findings and updating assumptions, re-running consistency checks shows improved confidence. Success: insights linked to spec sections, assumption confidence increased, PRD quality >80%.

**CUJ-4: Parallel Agent Development** — A developer managing multiple features spawns a Claude Code Agent Team from the Coldwine task hierarchy. The lead assigns tasks to teammates, which self-claim unblocked work from the shared task list. Coldwine bridges between Agent Teams (agent lifecycle) and Intermute (resource enforcement): when a teammate claims a task, Coldwine acquires Intermute file reservations for the task's target paths before the teammate begins work. If another teammate attempts to claim overlapping files, Intermute rejects the reservation—the teammate sees "blocked" status and the lead reassigns to non-overlapping work. Teammates coordinate via Agent Teams' native messaging for handoffs (e.g., "auth module ready for integration"), while Coldwine broadcasts task lifecycle events through Intermute for Bigend visibility. Reservation TTLs auto-expire on task completion, teammate shutdown, or stall detection. When Agent Teams is unavailable, Coldwine falls back to manual session management with tmux. Success: no concurrent edits to same files, clear blocking visibility, reservation cleanup after completion.

**CUJ-5: Multi-Project Mission Control** — A developer running 3+ Autarch-enabled projects opens Bigend (web at `:8099` or TUI). Bigend discovers all projects by scanning for `.gurgeh/`, `.coldwine/`, `.pollard/` directories and Agent Teams config at `~/.claude/teams/`, aggregates metrics (spec completion %, task progress, agent activity, research coverage), and streams real-time updates via WebSocket. When Agent Teams is active, Bigend reads team config for authoritative session state (lead + teammates), supplementing `last_active_at` heuristics with native team membership and task assignment data. Drilling into a project shows active PRD phases, task breakdowns, research coverage per phase, team structure, and agent session states (working/waiting/blocked/stalled/done/error). Success: unified view with <2s update latency, accurate state detection >95% when Agent Teams active (>90% with heuristics alone).

### Requirements

**Data & Persistence** — All tools maintain local-first storage in dedicated directories (`.gurgeh/`, `.coldwine/`, `.pollard/`) using YAML for human-readable artifacts and SQLite with WAL mode for high-frequency operations. Specs, sprints, and research findings persist as versioned YAML enabling git-based history. Signal deduplication uses a `(spec_id, type, affected_field)` unique constraint to prevent alert fatigue. Task state transitions (todo → in_progress → blocked → done) and agent sessions are tracked in normalized SQLite tables with foreign key relationships linking tasks to epics, stories, and sessions.

**Integration & Coordination** — Two complementary coordination layers: Claude Code Agent Teams handles agent lifecycle (spawning teammates, inter-agent messaging, plan approval gating, shared task list with dependency tracking), while Intermute (`../Intermute`) handles resource enforcement and cross-tool coordination (file reservations with glob-pattern locking, domain entity CRUD via REST with `{ok, data, meta, error}` envelope, task lifecycle event broadcasting via WebSocket). Coldwine bridges between them—translating Agent Teams task claims into Intermute file reservations and broadcasting task events for Bigend visibility. Cross-tool signals (competitor_shipped, research_invalidation, execution_drift, vision_drift) broadcast through Intermute at info/warning/critical severity levels. Agent-to-agent messaging uses Agent Teams' native mailbox when available, falling back to Intermute inbox-based messaging with thread grouping, importance levels, and acknowledgment flags.

**Research Enrichment** — Each Arbiter phase triggers phase-appropriate hunters: Vision activates GitHub Scout and HackerNews for market trends, Problem activates arXiv and OpenAlex for academic validation, Features activates competitor tracking for parity analysis. Pipeline modes (quick/balanced/deep) control synthesis depth with configurable parallelism and 2-minute per-item timeouts. Relevance scoring uses weighted factors (engagement, citations, recency, query match) with domain-specific half-lives—7 days for trends, 90 days for repos, 365 days for academic research. Insights link bidirectionally to spec sections via `linked_features` and `initiative_ref` fields.

**Research Suggestion UX** — Pollard findings surface through all four TUI panes using existing component patterns: (1) **Doc pane** displays a collapsible "Research Findings" section within the current phase view, showing relevance-ranked insights with icons (⚔ competitive, 📊 trends, 👤 user) and brief summaries—selecting a finding expands inline detail; (2) **Sidebar** shows research status via dynamic icons (● running, ◐ partial results, ✓ complete) with a badge count of high-relevance findings requiring attention; (3) **Chat pane** receives research context as system messages when findings arrive, enabling the user to ask follow-up questions like "tell me more about competitor X" or "how does this affect our approach?"; (4) **Log pane** streams hunter activity ("GitHub Scout: found 3 trending repos", "arXiv: 2 relevant papers") for transparency without interrupting the main workflow. High-relevance findings (score >0.8) trigger a subtle attention indicator in the sidebar rather than modal interruptions.

**Isolation & Performance** — Intermute file reservations provide task isolation: agents acquire reservations on glob patterns (e.g., `internal/auth/**/*.go`) with configurable TTLs before modifying files. Overlapping reservations are rejected, surfacing as "blocked" status in the TUI. This enforces what Agent Teams only recommends—Agent Teams advises "avoid file conflicts" as a best practice but provides no enforcement mechanism. Reservations auto-release on task completion, teammate shutdown, or TTL expiry. Session state tracks via Agent Teams' native team config when available, supplemented by `last_active_at` heuristic stall detection. Agent Teams' plan approval gating provides an additional isolation checkpoint—teammates must have their approach approved before modifying code. TUI refresh targets 2-second intervals, git commit scanning runs every 60 seconds, and Pollard watch cycles default to 24 hours with per-hunter overrides (2-6 hours for fast sources). All servers bind to loopback by default; remote access requires explicit opt-in with authentication.

### Scope + Assumptions

The unified TUI (`./dev autarch tui`) serves as the primary interface, integrating all four tools through a consistent 3-pane layout: sidebar (navigation/filters), doc pane (content), and agent pane (conversational interaction with the user's coding agent). Kickoff triggers both a codebase scan (blocking, ~5 seconds) and Pollard research (async, non-blocking) simultaneously—users enter the sprint flow immediately while research runs in the background. Gurgeh's 8-phase Arbiter Sprint runs as a propose-first chat interface with consistency checking and confidence scoring.

**Pollard owns the research review workflow** through its standard 3-pane layout. The sidebar filters findings (Inbox/Accepted/Rejected/Deferred) and shows hunter status. The doc pane displays the selected finding's full content: summary, source, affected spec sections, and suggested edits. The agent pane enables conversational triage—the same coding agent (Claude Code, Codex, etc.) that helps write the spec now helps evaluate research against it. Users respond naturally ("accept but rephrase the edit", "reject—we're targeting a different market", "defer to v2", "dig deeper on this competitor") and the agent executes actions, capturing reasoning alongside decisions. The header tab shows a badge count (`Pollard (3)`) that pulses after 5 minutes for unreviewed high-relevance items (>0.8 score).

**Triage decisions persist to `.pollard/feedback.yaml`**—a structured log of actions, reasoning, relevance scores, and affected spec sections. The agent maintains an aggregated preferences summary (domain focus, excluded domains, hunter effectiveness, relevance threshold adjustments) updated as patterns emerge from decisions. On session start, the agent reads this compact log (~500 tokens) instead of replaying full conversation history, enabling cross-session learning without context bloat. Global preferences at `~/.autarch/pollard-preferences.yaml` carry user patterns across projects. The agent uses these preferences to pre-filter findings, adjust relevance rankings, and refine research queries.

Accepted findings apply agent-suggested edits to spec sections with user confirmation. Rejected findings log reasoning to the feedback file. Deferred findings auto-populate "Future Considerations" in exported specs. Deep Dive requests trigger targeted Pollard research. Confidence scoring includes research review coverage—unreviewed high-relevance findings reduce the Research percentage without hard-blocking progress.

**Coldwine bridges Agent Teams and Intermute** for parallel development. When Agent Teams is available, Coldwine populates the team's shared task list from its Initiative→Epic→Story→Task hierarchy and translates teammate task claims into Intermute file reservations. Plan approval gating ensures teammates propose their approach before modifying code. When Agent Teams is unavailable, Coldwine falls back to manual session management with tmux—the Intermute reservation and event broadcasting layer works identically in both modes. Bigend aggregates project status with real-time WebSocket updates, reading Agent Teams config (`~/.claude/teams/`) for authoritative session state when available. All servers bind to `127.0.0.1` by default.

**Out for v1:** automatic spec updates from research (all incorporation requires conversational confirmation), research invalidation auto-blocking phase advancement, remote/multi-host coordination, multi-repo orchestration, three of four consistency check types (only GoalFeature ships), context timeout guardrails, CommonKeys migration, nested Agent Teams (teammates cannot spawn sub-teams), and Agent Teams session resumption (experimental limitation—teammates lost on `/resume`).

**Assumptions:** users run Autarch locally with SQLite; file-based persistence commits to git while event spine does not; Intermute spawns automatically with graceful degradation; `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled for parallel development features (Coldwine falls back to manual session management if unavailable); research hunters use free API tiers with rate limits; agent pane connects to user's configured coding agent which reads `.pollard/feedback.yaml` on session start; feedback log uses rolling window (last 50 decisions) to bound file size; button-based fallback available if agent unavailable; users review high-relevance findings before finalizing phases (skipping permitted but reflected in confidence); terminal width ≥120 columns for 3-pane layout; single developer workflow without multi-user collaboration; Agent Teams token usage is significantly higher than single-session—users accept this tradeoff for parallel development.

---

## Acceptance Criteria

### CUJ-1: Research-Enriched PRD Creation

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-1.1 | Kickoff completes codebase scan and displays results in doc pane within 10 seconds | Timer from TUI launch to scan result render |
| AC-1.2 | Pollard research begins async during kickoff; first findings appear in Pollard tab inbox within 60 seconds | Timer from kickoff start to first finding visible |
| AC-1.3 | Header tab shows `Pollard (N)` badge within 5 seconds of findings arriving | Observe badge update after hunter returns |
| AC-1.4 | Badge pulses after 5 minutes for unreviewed high-relevance findings (>0.8 score) | Let finding sit unreviewed, observe pulse animation |
| AC-1.5 | Pollard 3-pane layout renders: sidebar (Inbox/Accepted/Rejected/Deferred + hunter status), doc pane (finding detail), agent pane (triage conversation) | Navigate to Pollard tab, verify all panes present |
| AC-1.6 | Agent pane accepts natural language triage ("reject—not our market") and logs action to `.pollard/feedback.yaml` | Issue triage command, verify YAML append with action + reasoning |
| AC-1.7 | Accept action opens edit preview showing affected spec section with suggested changes; user can modify before confirming | Accept finding, verify preview modal with editable diff |
| AC-1.8 | Confidence score displays four components (Completeness, Consistency, Specificity, Research) and updates within 2 seconds of triage action | Perform triage, observe score recalculation |
| AC-1.9 | Research component of confidence reflects review coverage (unreviewed high-relevance findings reduce score) | Leave findings unreviewed, verify Research % decreases |
| AC-1.10 | Completed PRD exports to `.gurgeh/specs/` with all 8 phases populated as valid YAML | Run export, verify file structure and schema |
| AC-1.11 | Exported spec includes "Future Considerations" section containing all deferred findings with original reasoning | Defer findings, export, verify section contents |
| AC-1.12 | Phase-appropriate hunters trigger automatically: Vision→GitHub Scout+HackerNews, Problem→arXiv+OpenAlex, Users→community analysis, Features→competitor tracking, CUJs→workflow patterns, Requirements→implementation patterns, Scope→inverse research, Acceptance→test patterns | Advance through all 8 phases, verify hunter selection in log pane |
| AC-1.13 | End-to-end PRD creation completes in under 25 minutes with confidence >70% and research coverage >80% | Timed full sprint run from kickoff through export |
| AC-1.14 | GoalFeature consistency check flags conflicts between Vision/Problem statements and proposed Features (e.g., feature not traceable to a stated goal) | Add feature unrelated to Vision, verify consistency warning with affected sections identified |
| AC-1.15 | Spec version snapshot saved to `.gurgeh/specs/history/` on each phase completion; `gurgeh history <spec-id>` lists all versions; `gurgeh diff <spec-id> v1 v2` shows structured diff | Complete 3 phases, verify 3 snapshots exist, run history and diff commands |
| AC-1.16 | Rate-limited hunter displays retry countdown in log pane and does not block other hunters | Trigger rate limit (rapid repeated scans), verify countdown display and parallel hunter progress |
| AC-1.17 | Agent can list, get, and triage findings via MCP tools (e.g., `autarch_list_findings`, `autarch_triage_finding`) without requiring the TUI | Invoke MCP tools from agent session, verify CRUD operations on findings |

### CUJ-2: PRD to Task Breakdown

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-2.1 | Coldwine `init` reads completed spec and proposes Initiative → Epic → Story → Task hierarchy within 30 seconds | Timer from init to hierarchy display |
| AC-2.2 | Task review TUI displays proposed hierarchy with accept/edit/reject actions available per item | Navigate hierarchy, verify action buttons/commands |
| AC-2.3 | Accepted tasks persist to `.coldwine/` (or `.tandemonium/`) as YAML conforming to contract schema | Accept tasks, verify file structure and field presence |
| AC-2.4 | Task file patterns register as Intermute file reservations with specified TTL | Accept task with glob patterns, query Intermute API for reservation |
| AC-2.5 | Reservation conflicts surface as "blocked" status in TUI within 2 seconds of conflict detection | Create overlapping reservation from second agent, observe blocked status |
| AC-2.6 | Dependency DAG visualization shows blocking relationships between tasks | Create tasks with dependencies, verify graph renders correctly |
| AC-2.7 | Task state transitions (todo → in_progress → blocked → done) persist to SQLite with timestamps | Transition task states, query database for records |
| AC-2.8 | When Agent Teams enabled, Coldwine populates team's shared task list from its hierarchy; teammates can self-claim unblocked tasks | Spawn team, verify task list matches Coldwine hierarchy, claim task as teammate |
| AC-2.9 | Plan approval gating: teammate enters plan mode before implementation; lead reviews and approves/rejects | Spawn teammate with plan-required, verify plan mode enforced, approve plan, verify implementation begins |
| AC-2.10 | When Agent Teams unavailable, Coldwine falls back to manual session management without error | Disable Agent Teams flag, run Coldwine init, verify full flow works with tmux-based sessions |

### CUJ-3: Continuous Research Validation

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-3.1 | Pollard watch cycle triggers at configured interval (default 24h, configurable 2-6h per hunter) | Set short interval for testing, observe trigger timing |
| AC-3.2 | New findings include `affects` field linking to relevant spec sections (Vision, Problem, Features, etc.) | Run watch, verify finding metadata contains section links |
| AC-3.3 | Findings contradicting accepted findings surface with warning icon in Pollard sidebar | Inject contradicting finding, observe warning indicator |
| AC-3.4 | `SignalResearchInvalidation` emits when contradiction detected; visible in Signals tab with severity level and affected spec section | Trigger contradiction, verify signal in Signals view with severity=warning and section link |
| AC-3.4a | Signal deduplication prevents repeat alerts for same `(spec_id, type, affected_field)` tuple | Trigger same contradiction twice, verify only one signal emits |
| AC-3.5 | Deep Dive action triggers targeted research on specific thread and returns findings within 2 minutes | Request deep dive, time to additional results |
| AC-3.6 | Agent reads `.pollard/feedback.yaml` on session start; applies preferences to finding pre-filtering and ranking | Start new session with existing feedback, verify rankings reflect past rejections |
| AC-3.7 | Global preferences at `~/.autarch/pollard-preferences.yaml` merge with project-specific feedback; global takes precedence for conflicts | Set conflicting global/project prefs, verify merge behavior |
| AC-3.8 | Agent uses feedback to refine research queries ("exclude consumer-focused results") | Reject consumer findings, verify subsequent queries exclude them |
| AC-3.9 | Feedback log maintains rolling window of last 50 decisions; older entries archived to `.pollard/feedback-archive/` | Add 60 decisions, verify window size and archive presence |

### CUJ-4: Parallel Agent Development

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-4.1 | File reservation acquired before task work begins; reservation logged with TTL in Intermute | Start task, query Intermute for reservation record |
| AC-4.2 | Overlapping reservation request rejected with blocking task ID within 1 second | Request overlap, time rejection and verify error includes blocker ID |
| AC-4.3 | TUI shows reservation holder (agent/task) and TTL remaining for blocked paths | View blocked task in Coldwine, verify holder and TTL display |
| AC-4.4 | Reservation auto-releases on: (a) task completion, (b) agent disconnect, (c) TTL expiry | Test each trigger independently: complete task, kill session, wait for TTL; verify reservation cleared in all cases |
| AC-4.5 | Teammate-to-teammate messaging delivers within 5 seconds via Agent Teams' native mailbox; Coldwine broadcasts task lifecycle events via Intermute for Bigend | Send teammate message, time receipt; verify Bigend receives Intermute event |
| AC-4.6 | Critical handoff messages require acknowledgment; unacknowledged messages surface in Bigend dashboard | Send critical message, verify ack tracking and dashboard indicator |
| AC-4.7 | Agent can acquire and release file reservations via MCP tools (e.g., `autarch_reserve_paths`, `autarch_release_paths`) without requiring the TUI | Invoke MCP tools from agent session, verify reservation lifecycle |
| AC-4.8 | Coldwine acquires Intermute file reservations automatically when a teammate claims a task from the shared task list | Teammate claims task, verify Intermute reservation created for task's file patterns before work begins |
| AC-4.9 | Teammate shutdown triggers reservation release; lead can reassign task to new teammate | Shut down teammate, verify reservation cleared, assign to new teammate, verify new reservation |
| AC-4.10 | Agent Teams fallback: when Agent Teams unavailable, parallel development works via manual tmux sessions with Intermute reservations | Disable Agent Teams, start two tmux sessions, verify reservation enforcement still works |

### CUJ-5: Multi-Project Mission Control

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-5.1 | Bigend discovers projects by recursively scanning for `.gurgeh/`, `.coldwine/`, `.pollard/` directories from configured root | Create projects in subdirectories, verify discovery |
| AC-5.2 | Dashboard displays per-project metrics: spec completion %, task progress (todo/in_progress/done), agent session status, research coverage | Open dashboard, verify all four metric categories present |
| AC-5.3 | WebSocket updates reflect state changes within 2 seconds of underlying file modification | Modify spec file, time dashboard update |
| AC-5.4 | Drilling into project navigates to appropriate tool tab (Gurgeh for spec, Coldwine for tasks, Pollard for research) | Click project drill-down, verify correct tab opens |
| AC-5.5 | Agent session states detect correctly >95% with Agent Teams (reading `~/.claude/teams/` config), >90% with heuristics alone | With Agent Teams: test 20+ transitions, verify >19 correct. Without: verify >18 correct |
| AC-5.6 | Stall detection triggers after configurable inactivity period (default: agent silent for >5 minutes) | Pause agent activity, verify stall detection timing |
| AC-5.7 | When Agent Teams active, Bigend displays team structure: lead identity, teammate names, task assignments, and per-teammate status | Spawn team with 3 teammates, verify Bigend shows lead + 3 teammates with correct task mappings |
| AC-5.8 | Bigend gracefully handles Agent Teams absence—falls back to heuristic session detection without error | Disable Agent Teams, open Bigend, verify dashboard renders with heuristic-based agent state |

### Cross-Cutting

| ID | Criterion | Verification Method |
|----|-----------|---------------------|
| AC-X.1 | All servers (Gurgeh, Pollard, Coldwine, Bigend, Signals) bind to `127.0.0.1` by default | Start each server, verify listening address |
| AC-X.2 | Non-loopback binding requires explicit `--addr` flag; attempt without flag rejected | Try `--addr 0.0.0.0` without flag, verify error |
| AC-X.3 | TUI 3-pane layout renders correctly at terminal width ≥120 columns | Test at 120, 150, 200 column widths |
| AC-X.4 | Terminal width <120 columns displays graceful degradation message | Resize to 80 columns, verify message |
| AC-X.5 | Intermute unavailability does not crash tools; features requiring coordination display "Intermute unavailable" and continue | Kill Intermute, verify tools continue with degraded message |
| AC-X.6 | Agent pane unavailability falls back to button-based triage in doc pane | Disconnect agent, verify button fallback appears |
| AC-X.7 | File-based persistence (`.gurgeh/`, `.coldwine/`, `.pollard/`) suitable for git commit; no binary blobs | Inspect directories, verify YAML/text only |
| AC-X.8 | Event spine at `~/.autarch/events.db` does not require git tracking | Verify .gitignore excludes events.db path |
| AC-X.9 | Agent Teams integration is additive: all features work without `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled | Disable flag, run full flow (kickoff → sprint → task breakdown → parallel work → dashboard), verify no errors |
| AC-X.10 | Agent Teams experimental limitations documented: no session resumption for teammates, one team per session, no nested teams, lead is fixed | Verify CLAUDE.md or help text mentions these constraints |

---

## Research Insights

### Critical Implementation Gaps

**Gap 1: Intermute `Reserve()` has no glob overlap detection (BLOCKING CUJ-4)**

The Intermute storage layer at `/root/projects/Intermute/internal/storage/sqlite/sqlite.go` performs a simple INSERT with no overlap check. Two agents can simultaneously acquire exclusive reservations on `internal/auth/**/*.go` and `internal/**/*.go` without rejection. AC-4.2 ("Overlapping reservation request rejected") is untestable because the underlying mechanism does not exist. The local `ReservePaths` in Coldwine's `coordination.go` does exact string matching on paths, not glob expansion.

*Recommended:* Add AC-4.2a through AC-4.2c requiring glob overlap detection with proper semantics, atomic serialized transactions, and test cases covering subset/superset/partial patterns.

**Gap 2: Coldwine ↔ Agent Teams bridge mechanism is unspecified**

AC-4.8 says "Coldwine acquires Intermute file reservations automatically when a teammate claims a task" but HOW Coldwine detects task claims is undefined. No polling, event hook, callback, or API-wrapping mechanism is described. This is the architectural linchpin of CUJ-4. The existing code in `internal/coldwine/intermute/broadcaster.go` handles outbound events but has no inbound Agent Teams integration.

*Recommended:* Add AC-4.11 specifying detection mechanism and latency target (<2 seconds from claim to reservation).

**Gap 3: Signal deduplication is purely in-memory with no persistence**

The Signals broker in `pkg/signals/broker.go` is a pure fan-out with no deduplication. The `(spec_id, type, affected_field)` unique constraint described in Requirements exists nowhere in code. Additionally, `broker.go` line 51-54 silently drops signals when subscriber buffers fill (`select { case sub.ch <- sig: default: }`).

*Recommended:* AC-3.4a needs server-side enforcement with persistent dedup state surviving process restart. Add AC-3.4b through AC-3.4d for atomicity and persistence.

**Gap 4: Signal transport path ambiguity**

Two separate WebSocket systems (Intermute events and Signals server) serve overlapping purposes. The plan says signals "broadcast through Intermute" but Signal types in `pkg/signals/signal.go` are published through the Signals broker, not Intermute. There is no bridge. Bigend's aggregator only connects to Intermute WebSocket, not the Signals server.

*Recommended:* Resolve transport path and add AC-3.11 specifying how `SignalResearchInvalidation` reaches Bigend.

### Performance Analysis

**Codebase scan (AC-1.1) target of <10s is unrealistic.** The actual `exploration.Explore()` function shells out to Claude Code with a comprehensive prompt. Current timeout is set to 10 minutes. For 144K LOC repos, Claude Code exploration takes 60-120 seconds. The <10s target is only achievable for the local `filepath.WalkDir` in `scan.go`, not the full LLM-powered exploration.

*Recommended:* Split into structural scan (<5s, local file walk) and LLM exploration (<90s with streaming partial results).

**Full PRD sprint <25 min with >80% research coverage is contradictory.** The documented sprint timing in AGENTS.md is 20-40 minutes *without* research. Only 4 of 8 phases have research configs in `research_phases.go`. Research adds 15-120s per hunter per phase plus user triage time.

*Recommended:* Sprint-only target: <25 min. Research-enriched target: <45 min. Define "research coverage" precisely against the `researchQuality()` weighted formula.

**Agent Teams spawning overhead is unaccounted.** Each teammate is a separate Claude Code session (5-15s spawn time, 3-5x token consumption). Missing thresholds: team spawn (<30s for 3 teammates), plan approval round-trip (<60s), context loading per teammate.

### Security Findings

**F1 (CRITICAL): No glob overlap detection in Intermute reservations** — see Gap 1 above.

**F2 (HIGH): No agent identity verification for reservations.** The reservation handler accepts `agent_id` from request body without verification. Any local process can impersonate any agent. `releaseReservation` performs no ownership check.

**F3 (HIGH): Teammates inherit full MCP access.** No role-based access control—a teammate can invoke `autarch_update_task` on any task, `autarch_triage_finding` on any finding, or `autarch_reserve_paths` on any path outside its assigned scope.

**F4 (HIGH): WebSocket terminal streaming has no auth.** The `/ws/terminal/{session_name}` endpoint at Bigend accepts all origins (`OriginPatterns: []string{"*"}`), streaming live tmux output containing potentially sensitive data.

**F5 (MEDIUM): Feedback YAML poisoning.** `.pollard/feedback.yaml` is parsed as YAML with contents directly influencing agent behavior. Possible attacks: YAML bombs (recursive anchors), preference poisoning ("exclude all security findings"), silent modification undetectable without checksums.

**F6 (MEDIUM): X-Forwarded-For header spoofing in Intermute auth.** The auth middleware trusts `X-Forwarded-For` for local request detection. If Intermute is exposed, this bypasses authentication.

*Recommended:* Phase 0 (blocking): implement glob overlap detection + reservation ownership verification. Phase 1 (before production): restrict WebSocket origins, add CSRF protection, validate feedback YAML schema. Phase 2 (before non-loopback): disable X-Forwarded-For trust, add authentication middleware.

### Data Integrity Risks

**Spec versioning (`SaveRevision`) has non-atomic writes.** Two files written sequentially (snapshot YAML + revision metadata) with no write-to-temp-then-rename. The function mutates `spec.Version` as a side effect even if file writes fail. Concurrent phase completions can produce duplicate version numbers.

**Feedback rolling window (AC-3.9) lacks crash recovery.** Archive-then-truncate is not atomic. Process crash between archive write and truncation causes duplicate entries. Concurrent triage sessions can interleave YAML writes.

**Agent Teams ↔ Coldwine task sync has no reconciliation.** If a teammate marks done in Agent Teams while Coldwine is unreachable, states diverge. No conflict detection or resolution mechanism defined.

**SQLite single-connection bottleneck under Agent Teams.** `MaxOpenConns(1)` in `pkg/db/open.go` serializes all reads and writes. With 3+ teammates writing to `.coldwine/state.db`, `SQLITE_BUSY` errors become likely despite 5s timeout.

### Agent-Native Architecture Assessment

The plan treats agents as TUI users rather than first-class citizens. Key gaps:

1. **CRUD completeness:** Only Findings and Reservations have partial MCP tools. Missing: Spec, Phase, Task, Signal, Team, Confidence operations.
2. **No explicit completion signaling:** AC-5.6 uses heuristic stall detection (agent silent >5 min) — an anti-pattern. Agents should call `complete_task` tool explicitly.
3. **No dynamic context injection:** Agent system prompts don't include current spec phase, confidence score, pending findings, or available tools.
4. **No composability test:** No criterion verifies a new feature can be added via prompt alone (composing existing MCP tools).

*Recommended:* Full entity CRUD audit (7 entities × 4 operations), `complete_task` tool, `refresh_context` tool, capability map, parity tests.

### Best Practices Findings

**Separate correctness from performance.** Every timing-based AC should split: (1) correctness test using event ordering with generous timeouts, (2) performance benchmark using p95 over multiple runs. This eliminates the primary source of flaky tests.

**Add Given-When-Then for complex criteria.** Simple criteria (AC-X.1) are fine as-is. Multi-step criteria like AC-4.8 benefit from explicit Given/When/Then structure with observable preconditions and postconditions.

**Add negative criteria per CUJ.** The plan has zero explicit failure-path criteria. Each CUJ needs at least one: hunter failure mid-sprint, invalid spec input to Coldwine, feedback corruption, TTL expiry during active work, project directory disappearing.

**Use fixture-based testing for feedback loops.** AC-3.6 through AC-3.8 should use known feedback histories with deterministic assertions, not LLM invocations.

**Add idempotency criteria.** Reservation acquire for already-reserved paths, Coldwine init on existing tasks, spec export should all be idempotent.

### Institutional Learnings Applied

From `docs/solutions/`:

1. **Race condition testing mandatory** (from `arbiter-state-pointer-escape`): All arbiter tests must run with `go test -race`. The `State()` method returns deep-copied snapshots to avoid data races with TUI refresh (~60 FPS).

2. **Error message routing** (from `swallowed-generation-error-msg`): Parent views must pass error messages to children. `GenerationErrorMsg` from arbiter must reach SprintView's chat panel, not be swallowed.

3. **Phase propagation** (from `spec-propagation-consistency-pattern`): When user edits a phase via chat, dependent phases should update automatically. This behavior exists but has no AC.

4. **Chat-first keybindings** (from `chat-first-tui-design`): Ctrl+ prefixes only (no single-letter shortcuts). 50/50 split layout. Slash command picker on `/`.

5. **Quick scan moved to Users phase** (from `oracle-review-issues`): Changes when research evidence is available — affects AC-1.12 hunter trigger sequence.

6. **Phase generation split** (from `prd-requirements-blank-on-generation`): Early phases extract from artifacts; later phases generate dynamically via Claude Code. AC should verify all 8 produce non-empty content.

---

## Timing Thresholds Summary

| Metric | Correctness Target | Performance Budget | Rationale |
|--------|-------------------|-------------------|-----------|
| Structural scan | Completes and renders | <5s (p95) | Local file walk only |
| LLM exploration | Completes with streaming | <90s (p95) | Claude Code exploration, stream partial results |
| First research finding | Finding visible after hunter | <60s (raw), <120s (scored) | Rate limits + synthesis |
| Badge update | Badge reflects finding count | <5s | Event-driven |
| Badge pulse for unreviewed | Pulse activates | 5 min | Nudge without pressure |
| Triage → confidence update | Score recalculates after triage | <2s | Local computation |
| Task hierarchy generation | Hierarchy displayed | <60s (p95) | LLM analysis of complex spec |
| Reservation conflict detection | Conflict surfaced | <2s | Local Intermute lookup |
| Reservation rejection response | Rejection with blocker ID | <1s | Indexed DB query |
| Deep Dive results | Results returned (partial OK) | <3 min, configurable | Network + rate limits |
| WebSocket state update (push) | State reflects change | <2s | Intermute WebSocket events |
| File-change detection | Change detected | <5s | fsnotify or polling |
| Agent message delivery | Message received | <5s (Agent Teams), <10s (Intermute) | Two transport layers |
| Stall detection | Stall detected | 5 min code, 15 min research, configurable | Per-task-type thresholds |
| Full PRD sprint (no research) | All 8 phases complete | <25 min | CUJ-1 success metric |
| Full PRD sprint (with research) | Phases + triage complete | <45 min | Research + triage adds ~20 min |
| Confidence threshold | Score computed correctly | >70% | CUJ-1 success metric |
| Research coverage | Coverage formula applied | >60% (4 of 8 phases have configs) | Only 4 phases mapped in `research_phases.go` |
| Team spawn (3 teammates) | All teammates responsive | <30s | Process spawn + context loading |
| Plan approval round-trip | Approval received | <60s | LLM analysis |
| SQLite write p99 | Write succeeds | <100ms | Under 3 concurrent agent sessions |

---

## Test Categories

### Manual Testing Required
- AC-1.4 (pulse animation timing and appearance)
- AC-1.5 (3-pane layout visual verification — verify 50/50 split, Ctrl+ keybindings only, slash command picker)
- AC-1.7 (edit preview UX flow)
- AC-1.13 (end-to-end timed sprint run)
- AC-5.2 (dashboard visual completeness)
- AC-X.3, AC-X.4 (terminal resize behavior)
- AC-X.6 (button fallback UX)

### Integration Testing
- AC-1.1, AC-1.2, AC-1.3 (timing across components — split correctness from benchmark)
- AC-1.6, AC-1.10, AC-1.11 (file persistence)
- AC-1.14 (GoalFeature consistency check)
- AC-1.15 (spec versioning and diff — verify atomic writes, no version duplication)
- AC-1.16 (rate limit degradation)
- AC-1.17 (MCP tool finding operations)
- AC-2.1 through AC-2.10 (Coldwine full flow + Agent Teams integration + fallback)
- AC-3.1 through AC-3.5, AC-3.4a (Pollard watch + deep dive + signal dedup — verify server-side persistence)
- AC-4.1 through AC-4.10 (Intermute reservations + Agent Teams bridge + MCP tools + fallback)
- AC-5.1, AC-5.3, AC-5.7, AC-5.8 (Bigend discovery + WebSocket + Agent Teams config + fallback)
- AC-X.9 (full flow without Agent Teams)
- Degradation matrix: all 4 combinations of Agent Teams × Intermute

### Unit Testing
- AC-1.9 (confidence calculation with review coverage)
- AC-3.6, AC-3.7 (preference loading and merging — use fixture feedback histories, no LLM invocation)
- AC-3.9 (rolling window logic — test crash recovery and concurrent write safety)
- AC-5.5 (state detection heuristics, 20+ samples; test both Agent Teams and heuristic paths)
- All arbiter packages: `go test -race ./internal/gurgeh/arbiter/...` (mandatory per institutional learnings)

### Race Condition Testing
- AC-4.2: Two agents simultaneously request overlapping reservations — 100 iterations, zero double-grants
- AC-1.15: Two concurrent phase completions — no duplicate version numbers
- AC-3.9: Concurrent triage operations — no YAML corruption
- AC-3.4a: Simultaneous signal emissions — dedup check and publish are atomic

### CUJ Transition Testing
- CUJ-1→2: Completing PRD export surfaces handoff options (generate tasks, start research watch)
- CUJ-1→3: Research monitoring activation after sprint completion
- CUJ-2→4: From task hierarchy to Agent Team spawn
- CUJ-3+4: Research invalidation during parallel development surfaces as blocked indicator on affected tasks

### Negative/Failure Path Testing
- CUJ-1: Hunter failure mid-sprint — sprint continues, failed hunter shows error in log, confidence adjusts
- CUJ-2: Coldwine init with spec having zero CUJs — clear error message, not empty hierarchy
- CUJ-3: Feedback.yaml malformed — agent logs warning, starts with empty preferences, doesn't overwrite
- CUJ-4: TTL expiry during active work — agent warned at 80% TTL, blocked after expiry
- CUJ-5: Project directory deleted while Bigend running — removed from dashboard on next scan, no crash

---

## Open Questions

1. **Confidence threshold for export:** Should there be a minimum confidence (e.g., >50%) before export is allowed, or always permitted with score as advisory?
   > *Research recommendation:* Advisory only — always permit export. Display warning banner when confidence <50%. Agents should have the same capability as users; don't gate actions artificially. (Agent-native principle)

2. **Button fallback completeness:** When agent pane unavailable, do button-based actions need to capture reasoning, or just the action (Accept/Reject/Defer)?
   > *Research recommendation:* Capture reasoning via mandatory dropdown (Wrong Market / Already Addressed / Defer to V2 / Other) but don't require free-text. Without reasoning, the feedback loop (AC-3.6) cannot learn preferences across sessions. (Best practices)

3. **Deep Dive timeout:** Is 2 minutes a hard limit, or should it be configurable? What happens if exceeded—partial results or failure?
   > *Research recommendation:* Configurable (default 2 min), returns partial results on timeout with "partial" flag. Never discard progress. Configure via `.pollard/config.yaml`. (Best practices, agent-native partial completion pattern)

4. **Stall detection sensitivity:** 5 minutes default may be too aggressive for agents doing deep research. Should this be configurable per-task-type?
   > *Research recommendation:* Configurable per-task-type: 5 min for code tasks, 15 min for research tasks. Supplement heuristic with explicit `report_status` MCP tool agents call proactively. (Performance, agent-native)

5. **MCP tool surface area:** Which Autarch operations need MCP tools for agent-native access? Current plan specifies findings triage and file reservations—should spec editing, phase navigation, and confidence queries also be exposed?
   > *Research recommendation:* Full CRUD for all 7 entities (Spec, Finding, Task, Reservation, Signal, Team, Confidence). Start minimal in v1 (findings + reservations + confidence query), expand based on usage. Add `autarch_refresh_context` and `autarch_complete_task` tools. (Agent-native CRUD completeness + Parity principle)

6. **Agent Teams task list ownership:** Coldwine has a richer hierarchy (Initiative→Epic→Story→Task) than Agent Teams' flat task list. Should Coldwine flatten its hierarchy into Agent Teams' format, or should teammates read Coldwine's hierarchy directly and only use Agent Teams' task list for claim/status tracking?
   > *Research recommendation:* Flatten to leaf-level Tasks with Epic/Story context as description prefix. Agent Teams' flat list for claim/status; Coldwine hierarchy accessible via MCP for agents that need the full structure. This avoids coupling teammates to Coldwine internals while preserving data richness. (Architecture, agent-native granularity)

7. **Agent Teams token budget:** Agent Teams uses significantly more tokens than single sessions. Should Coldwine expose estimated token cost before spawning a team, or let users decide based on their own budget awareness?
   > *Research recommendation:* Yes — show estimated cost. Agent Teams uses ~3-5x tokens for a 3-teammate setup. Simple estimate: task count × average task complexity × per-task token estimate. Users need this for informed decisions. (Agent-native principle: agents have same info as users)

8. **Coldwine ↔ Agent Teams bridge mechanism:** How does Coldwine detect teammate task claims? Polling the task list? Event hook? API wrapping?
   > *Research recommendation:* Needs resolution before CUJ-4 implementation. Polling at 1-2s introduces latency and race conditions. Event-based via file watching of `~/.claude/teams/` task file is more responsive but adds fsnotify dependency. Define interface adapter (`AgentTeamsClient`) mockable for unit tests. (Architecture, spec flow)

9. **Glob overlap semantics for reservation conflicts:** Does `internal/auth/**/*.go` conflict with `internal/auth/jwt/*.go`? Does `internal/**/*.go` conflict with `internal/auth/**/*.go`?
   > *Research recommendation:* Must resolve — current implementation uses exact string matching (not glob expansion), meaning subsets/supersets DON'T conflict. This is a serious correctness hole. Define: subset overlap detected, superset overlap detected, disjoint patterns permitted. (Security, data integrity)

10. **Signal transport: Signals server or Intermute?** Signals and Intermute events are architecturally distinct systems with no bridge. Which carries `SignalResearchInvalidation` to Bigend?
    > *Research recommendation:* Choose one canonical path and document it. Options: (a) Signals publish through Intermute as messages with `signal.` prefix, (b) Bigend subscribes to both, (c) bridge service. Option (a) is simplest. (Architecture)

---

## Dependencies

- Pollard hunters functional on free API tiers (GitHub, HackerNews, arXiv, OpenAlex, PubMed)
- Intermute runnable locally (Go binary or Docker)
- **Intermute glob overlap detection implemented** (BLOCKING: CUJ-4 isolation untestable without it — current `Reserve()` does simple INSERT with no overlap check)
- Claude Code with `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` enabled for parallel development testing (Agent Teams is additive; all other features testable without it)
- Claude Code or Codex CLI available for agent pane testing
- tmux available for Agent Teams split-pane mode and manual session fallback
- Terminal emulator supporting ≥120 column width
- SQLite available on test machine
- Git available for persistence verification
- **`go test -race` passing on all arbiter packages** (concurrency safety from institutional learnings)
- **AgentTeamsClient interface defined in `pkg/`** (shared between Coldwine and Bigend, mockable for unit tests)

## Degradation Matrix

| Scenario | Agent Teams ON | Agent Teams OFF |
|----------|---------------|-----------------|
| **Intermute ON** | Full capability (AC-2.8, AC-4.1-4.9, AC-5.7) | Manual sessions + Intermute reservations (AC-2.10, AC-4.10, AC-5.8) |
| **Intermute OFF** | **DEGRADED:** Task claims work but no file reservation enforcement. Teammates proceed with "unprotected" warning. Core CUJ-4 value lost. | **MINIMAL:** PRD creation + task breakdown work. No reservations, no signals broadcast, no dashboard coordination. |

Each cell is a testable configuration. AC-X.5 and AC-X.9 cover two cells; the "Agent Teams ON + Intermute OFF" cell is currently untested (see Research Insights Gap 4).
