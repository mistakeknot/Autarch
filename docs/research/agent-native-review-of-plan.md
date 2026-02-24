# Agent-Native Architecture Review of Acceptance Criteria Plan

**Reviewer:** Agent-Native Architecture Reviewer (Claude Opus 4.6)
**Date:** 2026-02-06
**Subject:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Supporting material reviewed:**
- `/root/projects/Autarch/pkg/mcp/server.go` (Go MCP server, 8 registered tools)
- `/root/projects/Autarch/pkg/mcp/handlers.go` (tool handler implementations)
- `/root/projects/Autarch/autarch-plugin/.claude-plugin/plugin.json` (plugin manifest)
- `/root/projects/Autarch/gurgeh-plugin/.claude-plugin/plugin.json` (Flux Drive plugin)
- `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (sprint orchestrator)
- `/root/projects/Autarch/internal/gurgeh/arbiter/tui/arbiter_view.go` (TUI key handlers)
- `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` (sprint state, phases, confidence)
- `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` (hardcoded phase-to-hunter map)
- `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` (spec versioning, history)
- `/root/projects/Autarch/pkg/intermute/types.go` (all entity types: Spec, Epic, Story, Task, Insight, etc.)
- `/root/projects/Autarch/docs/research/acceptance-criteria-2026-02-05/deep-dive-mcp-tool-surface.md` (38-tool design)
- `/root/projects/Autarch/docs/research/acceptance-criteria-2026-02-05/agent-native-architecture-skill.md` (prior analysis)

---

## Summary

The acceptance criteria plan is thorough in describing **what a user sees and does** through the TUI but systematically underspecifies what an agent can do programmatically. The plan's own "Agent-Native Architecture Assessment" section (lines 253-262) identifies this gap, and the companion research document (`deep-dive-mcp-tool-surface.md`) proposes a comprehensive 38-tool catalog with context injection -- but that design has not been incorporated back into the acceptance criteria. As a result, the plan defines 5 CUJs with ~55 acceptance criteria, of which exactly 2 (AC-1.17, AC-4.7) mention MCP tools. The existing Go MCP server at `pkg/mcp/server.go` registers 8 tools, but only 3 align with actual CUJ operations (list PRDs, get PRD, update task). The remaining 5 are either placeholders (`autarch_research` returns a stub) or utility operations. The agent cannot start a sprint, advance a phase, accept a draft, export a spec, triage a finding, init a task breakdown, view confidence, emit a signal, or query reservations through MCP tools.

**Verdict: NEEDS WORK.** The plan describes a TUI-first system with an agent bolt-on rather than an agent-native application. The 38-tool design exists as a research artifact but needs to become binding acceptance criteria.

---

## Capability Map

The following table maps every user-facing action identified in the plan to its MCP tool status. "Existing" means the tool exists in `pkg/mcp/server.go`. "Designed" means the tool appears in the 38-tool catalog (`deep-dive-mcp-tool-surface.md`) but is not implemented or referenced by an AC. "Missing" means neither exists.

| # | UI Action | Plan Reference | Existing MCP Tool | Designed MCP Tool | In an AC? | Status |
|---|-----------|---------------|-------------------|-------------------|-----------|--------|
| 1 | Start kickoff / codebase scan | AC-1.1 | None | None | No | MISSING |
| 2 | View codebase scan results | AC-1.1 | None | None | No | MISSING |
| 3 | Advance to next sprint phase | AC-1.12 | None | None | No | MISSING |
| 4 | Revert to previous phase | Orchestrator.Revert() | None | None | No | MISSING |
| 5 | Accept draft (Ctrl+A) | AC-1.7 context | None | None | No | MISSING |
| 6 | Edit/revise draft (Ctrl+E, Enter) | AC-1.7 context | None | autarch_update_spec | No | MISSING |
| 7 | Select alternative phrasing (/1 /2 /3) | TUI only | None | None | No | MISSING |
| 8 | View confidence score | AC-1.8 | None | autarch_get_confidence | No | MISSING |
| 9 | Export spec to file | AC-1.10 | None | None | No | MISSING |
| 10 | List specs | AC-1.10 context | autarch_list_prds | autarch_list_specs | No | PARTIAL |
| 11 | Get spec by ID | AC-1.10 context | autarch_get_prd | autarch_get_spec | No | PARTIAL |
| 12 | View Pollard findings in sidebar | AC-1.5 | None | autarch_list_insights | No | MISSING |
| 13 | Triage finding (accept/reject/defer) | AC-1.6 | None | autarch_update_insight (triage) | AC-1.17 | PARTIAL |
| 14 | Accept finding edit to spec | AC-1.7 | None | None | No | MISSING |
| 15 | Request Deep Dive | AC-3.5 | None | None | No | MISSING |
| 16 | View research status in sidebar | AC-1.5 | None | None | No | MISSING |
| 17 | Phase-appropriate hunter trigger | AC-1.12 | None | autarch_trigger_research | No | MISSING |
| 18 | Run ad-hoc research | CUJ-3 | autarch_research (STUB) | autarch_trigger_research | No | STUB |
| 19 | View spec version history | AC-1.15 | None | None | No | MISSING |
| 20 | Diff spec versions | AC-1.15 | None | None | No | MISSING |
| 21 | Init task breakdown | AC-2.1 | None | None | No | MISSING |
| 22 | Accept/edit/reject task proposal | AC-2.2 | None | None | No | MISSING |
| 23 | View dependency DAG | AC-2.6 | None | None | No | MISSING |
| 24 | Transition task state | AC-2.7 | autarch_update_task | autarch_update_task | No | EXISTS |
| 25 | List tasks | AC-2.3 context | autarch_list_tasks | autarch_list_tasks | No | EXISTS |
| 26 | Spawn Agent Team | AC-2.8 | None | None | No | MISSING |
| 27 | Approve/reject teammate plan | AC-2.9 | None | None | No | MISSING |
| 28 | Acquire file reservation | AC-4.1 | None | autarch_reserve_files | AC-4.7 | PARTIAL |
| 29 | Release file reservation | AC-4.4 | None | autarch_release_reservation | AC-4.7 | PARTIAL |
| 30 | View reservation status/holder | AC-4.3 | None | autarch_list_reservations | No | MISSING |
| 31 | Check if path is reserved | AC-4.2 context | None | autarch_check_reservation | No | MISSING |
| 32 | Send agent-to-agent message | AC-4.5 | autarch_send_message | autarch_send_message | No | EXISTS |
| 33 | View dashboard metrics | AC-5.2 | autarch_project_status (BASIC) | autarch_refresh_context | No | PARTIAL |
| 34 | Drill into project | AC-5.4 | None | None | No | MISSING |
| 35 | View team structure | AC-5.7 | None | autarch_list_agents | No | MISSING |
| 36 | View agent session states | AC-5.5 | None | autarch_get_agent | No | MISSING |
| 37 | Report agent status | AC-5.6 | None | autarch_report_status | No | MISSING |
| 38 | Complete task explicitly | AC-4.4, AC-5.6 | None | autarch_complete_task | No | MISSING |
| 39 | View/dismiss signals | AC-3.4 | None | autarch_list_signals, autarch_dismiss_signal | No | MISSING |
| 40 | Emit signal | AC-3.4 | None | autarch_emit_signal | No | MISSING |
| 41 | Register as agent | CUJ-4, CUJ-5 | None | autarch_register_agent | No | MISSING |
| 42 | Heartbeat/status report | CUJ-4, CUJ-5 | None | autarch_heartbeat, autarch_report_status | No | MISSING |
| 43 | Refresh context mid-session | Long sessions | None | autarch_refresh_context | No | MISSING |
| 44 | List available capabilities | Discoverability | None | autarch_list_capabilities | No | MISSING |
| 45 | Fetch inbox messages | CUJ-4 | None | autarch_fetch_inbox | No | MISSING |
| 46 | Claim task (self-assign) | AC-2.8 | None | autarch_claim_task | No | MISSING |
| 47 | Resume sprint from saved state | Orchestrator.Resume() | None | None | No | MISSING |
| 48 | Handoff option selection | Post-sprint | None | None | No | MISSING |

**Summary: 5 of 48 actions have existing MCP tools. 2 of 48 are referenced in acceptance criteria. The remainder are either designed-but-not-AC'd (18) or completely missing (23).**

---

## Findings

### Critical Issues (Must Fix)

#### 1. Only 2 of 55 Acceptance Criteria Reference MCP Tools

- **Location:** Plan lines 92-183 (all AC sections)
- **Impact:** An agent cannot complete any CUJ end-to-end without the TUI. The plan verifies TUI behavior but does not verify that agents can perform the same actions.
- **Evidence:** AC-1.17 mentions "MCP tools" for findings. AC-4.7 mentions "MCP tools" for reservations. Every other AC describes TUI-observed behavior only.
- **Fix:** For each CUJ, add an AC requiring that the entire journey can be completed via MCP tool calls alone. The 38-tool catalog in `docs/research/acceptance-criteria-2026-02-05/deep-dive-mcp-tool-surface.md` provides the design -- it must be promoted from research to binding criteria. Specifically:
  - AC-CUJ1-AGENT: Sprint creation through export achievable via `autarch_create_spec`, phase advance tools, `autarch_get_confidence`, and `autarch_export_spec` (or file write to `.gurgeh/specs/`).
  - AC-CUJ2-AGENT: Task breakdown achievable via `autarch_list_specs`, `autarch_create_task`, `autarch_update_task`, `autarch_reserve_files`.
  - AC-CUJ3-AGENT: Research validation achievable via `autarch_list_insights`, `autarch_trigger_research`, `autarch_emit_signal`.
  - AC-CUJ4-AGENT: Parallel development achievable via `autarch_claim_task`, `autarch_reserve_files`, `autarch_report_status`, `autarch_complete_task`.
  - AC-CUJ5-AGENT: Project status observable via `autarch_refresh_context`, `autarch_list_agents`, `autarch_list_signals`.

#### 2. "7 entities x 4 operations" CRUD Audit is Incomplete

- **Location:** Plan line 257 identifies this gap; research doc `agent-native-architecture-skill.md` lines 131-156 provides the audit
- **Impact:** Agents cannot create, read, update, or delete most entities. The Intermute client (`pkg/intermute/types.go`) defines 11 entity types. The plan mentions 7 entities. The existing MCP server (`pkg/mcp/server.go`) has CRUD coverage for approximately 1.5 entities (partial Spec read, partial Task update).
- **CRUD coverage of existing Go MCP tools vs the 7 plan entities:**

| Entity | Create | Read | Update | Delete | MCP Coverage |
|--------|--------|------|--------|--------|-------------|
| Spec | - | autarch_get_prd (read-only) | - | - | 1/4 |
| Finding/Insight | - | - | - | - | 0/4 |
| Task | - | autarch_list_tasks | autarch_update_task | - | 2/4 |
| Reservation | - | - | - | - | 0/4 |
| Signal | - | - | - | - | 0/4 |
| Team/Agent | - | - | - | - | 0/4 |
| Confidence | - | - | - | N/A | 0/3 |

- **Fix:** The 38-tool catalog covers this completely. Promote tools #1-#33 from the research doc into ACs. At minimum for v1, implement full CRUD for Spec, Insight, Task, and Reservation (the four entities agents interact with most). Signal and Team can follow.

#### 3. No Context Injection Specified in Any Acceptance Criterion

- **Location:** Plan line 259 identifies this gap; research doc `deep-dive-mcp-tool-surface.md` lines 386-469 provides a complete context injection design
- **Impact:** When an agent connects via MCP, it does not know what sprint phase is active, what the confidence score is, what findings are pending, what reservations are held, or what tools are available. Every agent interaction starts from zero context, making the agent pane useless for anything beyond generic chat.
- **Evidence:** The orchestrator (`internal/gurgeh/arbiter/orchestrator.go`) exposes `State()` returning a `SprintState` with phase, confidence, findings, and conflicts. This state is consumed by the TUI (`arbiter_view.go` line 162: `v.syncStateSnapshot()`) but there is no equivalent injection into agent system prompts.
- **Fix:** Add AC-CTX.1 through AC-CTX.4 per the prior analysis:
  - AC-CTX.1: On MCP session initialization, the server injects current spec ID, phase, confidence breakdown, and pending finding count into the agent's context (via the `autarch_refresh_context` tool or system prompt template from `deep-dive-mcp-tool-surface.md` lines 388-441).
  - AC-CTX.2: `autarch_refresh_context` tool exists and returns a selective state snapshot.
  - AC-CTX.3: Domain vocabulary (phase, finding, triage, reservation, signal, confidence) is defined in the system prompt so the agent never misunderstands user requests.
  - AC-CTX.4: Available capabilities are mapped to natural language ("when user says 'advance to next phase', use `autarch_advance_phase`").

#### 4. Stall Detection as Primary Completion Mechanism is an Anti-Pattern

- **Location:** AC-5.6 (plan line 166)
- **Impact:** The plan defines agent completion detection as "agent silent for >5 minutes." This means reservation auto-release (AC-4.4), task state transitions, and dashboard status all depend on a heuristic that cannot distinguish between a stalled agent, a thinking agent, and a network-disconnected agent. Open Question #4 (plan line 386) partially addresses this but does not promote the fix into an AC.
- **Evidence:** The `deep-dive-mcp-tool-surface.md` design includes `autarch_report_status` (tool #35) and `autarch_complete_task` (tool #18) as explicit completion/progress signaling. Neither appears in any AC.
- **Fix:**
  - AC-5.6 should be revised: "Agents signal completion via `autarch_complete_task`. Heuristic stall detection (configurable timeout: 5 min code, 15 min research) serves as crash-recovery fallback only."
  - Add AC-COMP.1: `autarch_complete_task` tool exists and triggers reservation release + task state transition.
  - Add AC-COMP.2: `autarch_report_status` tool exists; calling it resets the stall timer.
  - Add AC-COMP.3: The stall detection mechanism is documented as a fallback for crashed agents, not a primary signal.

### Warnings (Should Fix)

#### 5. Phase-to-Hunter Mapping is Hardcoded Business Logic

- **Location:** `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` lines 15-50
- **Impact:** The mapping "Vision -> github-scout + hackernews, Problem -> arxiv + openalex, Features -> competitor-tracker + github-scout, Requirements -> github-scout" is hardcoded in Go. An agent cannot override or extend this mapping. Adding a new hunter or changing the strategy for a phase requires a code change and rebuild.
- **AC-1.12** encodes this mapping as a pass/fail test: "Phase-appropriate hunters trigger automatically." This treats the mapping as correct-by-definition rather than configurable.
- **Fix:** Split AC-1.12 into:
  - AC-1.12a: Primitives exist to list available hunters, trigger a specific hunter, and get phase context.
  - AC-1.12b: Default phase-to-hunter mapping is defined in a configuration file (`.gurgeh/research-config.yaml` or similar), not in compiled Go code. The agent can read and propose modifications to this mapping.

#### 6. Spec Versioning Has No MCP Equivalent

- **Location:** AC-1.15, `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` lines 42-105
- **Impact:** `SaveRevision`, `LoadHistory`, and `LoadRevisionSpec` are internal Go functions callable from the CLI (`gurgeh history`, `gurgeh diff`) and the TUI but have no MCP tool equivalent. An agent cannot inspect spec version history or compute diffs programmatically.
- **Fix:** Add `autarch_spec_history` (list versions) and `autarch_spec_diff` (compare two versions) MCP tools, or at minimum ensure agents can read the `.gurgeh/specs/history/` YAML files directly and document this path in the system prompt.

#### 7. `autarch_research` Handler is a Stub

- **Location:** `/root/projects/Autarch/pkg/mcp/handlers.go` lines 220-243
- **Impact:** The handler returns `{"status": "queued", "message": "Research request queued"}` without actually invoking Pollard. An agent calling this tool gets no results and no way to check progress. This violates the "rich output that helps agent verify success" principle.
- **Fix:** Either implement the handler to invoke Pollard research (even if async), or remove the tool and replace it with the properly designed `autarch_trigger_research` from the 38-tool catalog.

#### 8. No Composability Test

- **Location:** Plan line 261 identifies this; no AC addresses it
- **Impact:** There is no acceptance criterion verifying that a new feature can be built by composing existing MCP primitives without code changes. This is the core test of an agent-native architecture.
- **Fix:** Add at least one composability AC:
  - AC-C.1: A "weekly research digest" (summarize all findings grouped by phase) can be achieved by an agent using `autarch_list_insights`, `autarch_get_spec`, and file write tools. No new Go code required.

#### 9. Agent-Native Test Category is Missing

- **Location:** Plan lines 322-370 (Test Categories)
- **Impact:** The test plan defines Manual, Integration, Unit, and Race Condition test categories. None of these verify agent capability. A test suite that passes all these categories could still produce a system where agents cannot perform basic operations.
- **Fix:** Add "Agent Capability Testing" as a test category:
  - For each CUJ, script the journey as a sequence of MCP tool calls and verify the same end state as a TUI walkthrough.
  - Parity tests: automated verification that every tool in the capability map exists and is documented.
  - Context parity tests: verify agent system prompt contains current phase, confidence, and finding count.

#### 10. Designed 38-Tool Surface is Not Referenced

- **Location:** `docs/research/acceptance-criteria-2026-02-05/deep-dive-mcp-tool-surface.md`
- **Impact:** This is an excellent, comprehensive design that addresses most of the agent-native gaps. But it exists only as a research artifact. The acceptance criteria plan does not reference it, import its tool names, or require its implementation. Without promotion to ACs, there is no mechanism to ensure it gets built.
- **Fix:** Create an "Agent Tool Surface" AC section that references the 38-tool catalog and requires at least the P0 tools (Spec CRUD, Insight CRUD, Task CRUD, Reservation CRUD, `refresh_context`, `report_status`, `complete_task`, `list_capabilities`) to be implemented and passing integration tests.

### Observations (Consider)

#### 11. Two MCP Server Implementations

The codebase has both a Go MCP server (`pkg/mcp/server.go`) and a TypeScript MCP server (`mcp-server/src/server.ts`). The 38-tool design recommends unifying on the Go server. This is the right call, but the TypeScript server's `claim_task` and `complete_task` tools have better designs (idempotent claiming, validation on completion) that should be ported to the Go implementation rather than lost.

#### 12. CUJ Transition Criteria Lack Agent Paths

Plan lines 361-364 define CUJ transition criteria (e.g., "Completing PRD export surfaces handoff options"). These transitions are described as TUI affordances ("surfaces handoff options") with no agent equivalent. An agent completing CUJ-1 should be able to programmatically discover that CUJ-2 (task breakdown) and CUJ-3 (research monitoring) are now available.

#### 13. Feedback Preferences System is Agent-Friendly but Isolated

The `.pollard/feedback.yaml` system (AC-3.6 through AC-3.9) is a good example of improvement-over-time design. However, it only applies to Pollard research. The plan does not specify equivalent learning mechanisms for Gurgeh (sprint preferences, phase depth preferences) or Coldwine (task estimation accuracy, common blocking patterns).

#### 14. Entity Count is Underspecified

The plan says "7 entities" but the Intermute types file (`pkg/intermute/types.go`) defines 11 entity types: Spec, Epic, Story, Task, Insight, Session, CriticalUserJourney, AcceptanceCriterion, Agent, Message, Reservation. The plan's "7 entities" appears to collapse Epic/Story into Task, Session into Team, and omit CriticalUserJourney and AcceptanceCriterion. The CRUD audit should be explicit about which entities are in scope and which are deferred.

#### 15. SaveRevision Atomicity Has Been Fixed

The plan's Research Insights section (line 243) flags `SaveRevision` as having "non-atomic two-file writes." However, `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` now uses `fileutil.AtomicWriteFile` for both the snapshot and revision metadata, with rollback on metadata write failure (lines 88-102). The plan's concern may be outdated.

---

## Detailed CUJ-by-CUJ Agent Parity Analysis

### CUJ-1: Research-Enriched PRD Creation

**TUI actions identified (from `arbiter_view.go`):**
1. Start sprint (`Init()` -> `orchestrator.Start()`)
2. View proposed draft (doc panel renders `SectionDraft.Content`)
3. Accept draft (Ctrl+A -> `acceptDraft()` -> `orchestrator.AcceptDraft()`)
4. Edit draft (Ctrl+E puts content in composer; Enter submits -> `orchestrator.ReviseDraft()`)
5. Select alternative (/1 /2 /3 -> `selectOption()`)
6. View confidence score (rendered in doc panel from `ConfidenceScore`)
7. Advance phase (auto after accept, or `orchestrator.Advance()`)
8. Revert phase (`orchestrator.Revert()`)
9. View research findings (Pollard tab)
10. Triage finding (accept/reject/defer in agent pane)
11. Export spec (handoff option -> `orchestrator.ExportSpec()`)
12. View spec history (`gurgeh history` CLI)
13. View consistency conflicts (rendered in doc panel)
14. Resume saved sprint (`orchestrator.Resume()`)

**Agent MCP equivalents:**
- Action 1: No tool. Agent cannot start a sprint.
- Action 2: No tool. Agent cannot read the current draft. Could read `.gurgeh/sprints/` YAML but path not documented.
- Action 3: No tool. Agent cannot accept a draft.
- Action 4: No tool. Agent cannot revise a draft.
- Action 5: No tool. Agent cannot select alternatives.
- Action 6: No tool. `autarch_get_confidence` is designed but not implemented.
- Action 7: No tool.
- Action 8: No tool.
- Action 9: No tool. `autarch_list_insights` is designed but not implemented.
- Action 10: Partially designed. AC-1.17 mentions it.
- Action 11: No tool.
- Action 12: No tool.
- Action 13: No tool.
- Action 14: No tool.

**Verdict: 0 of 14 actions are fully agent-accessible today. 2 of 14 are partially designed.**

### CUJ-2: PRD to Task Breakdown

**Agent MCP equivalents:**
- Init breakdown: No tool.
- Accept/edit/reject proposals: No tool.
- View dependency DAG: No tool.
- Task state transitions: `autarch_update_task` EXISTS.
- List tasks: `autarch_list_tasks` EXISTS.
- Spawn Agent Team: No tool.
- Approve/reject plan: No tool.

**Verdict: 2 of 7 actions are agent-accessible (list and update tasks).**

### CUJ-3: Continuous Research Validation

**Agent MCP equivalents:**
- Watch cycle management: No tool.
- View findings with `affects` metadata: No tool.
- Deep Dive request: No tool.
- Read feedback on session start: Can read `.pollard/feedback.yaml` directly (shared workspace -- good).
- Signal emission/viewing: No tool.

**Verdict: 0 of 5 actions are fully agent-accessible via MCP. 1 via direct file read.**

### CUJ-4: Parallel Agent Development

**Agent MCP equivalents:**
- Acquire reservation: Designed (AC-4.7) but not implemented.
- Release reservation: Designed (AC-4.7) but not implemented.
- View reservation status: No tool.
- Send message: `autarch_send_message` EXISTS.
- Claim task: No tool.
- Complete task: No tool.
- Report status: No tool.

**Verdict: 1 of 7 actions is agent-accessible (send message). 2 more designed but not implemented.**

### CUJ-5: Multi-Project Mission Control

**Agent MCP equivalents:**
- View project metrics: `autarch_project_status` EXISTS (basic).
- Drill into project: No tool.
- View team structure: No tool.
- View agent states: No tool.
- Stall detection: No explicit tool; heuristic only.

**Verdict: 1 of 5 actions partially accessible.**

---

## Open Question Verdicts

| # | Question | Plan Recommendation | Agent-Native Verdict |
|---|----------|--------------------|--------------------|
| 1 | Confidence threshold for export | Advisory only | AGREE. Do not gate agent actions artificially. |
| 2 | Button fallback reasoning | Mandatory dropdown | AGREE. Reasoning capture is essential for feedback loop. |
| 3 | Deep Dive timeout | Configurable, partial results | AGREE. Partial completion pattern. |
| 4 | Stall detection sensitivity | Configurable + report_status | PARTIALLY AGREE. Must promote `report_status` and `complete_task` from "recommendation" to required AC. |
| 5 | MCP tool surface area | Start minimal | DISAGREE. Minimal means agents cannot complete CUJs. Full CRUD for Spec, Insight, Task, Reservation is the v1 floor. |
| 6 | Task list ownership | Flatten to leaf tasks | AGREE. Simple primitives with context. |
| 7 | Token budget | Show estimated cost | AGREE. Agents need same info as users. |
| 8 | Bridge mechanism | Needs resolution | AGREE. Add: must be exposed as MCP tools, not just internal Go interfaces. |
| 9 | Glob overlap semantics | Must resolve | AGREE. Reservation tools are meaningless without overlap detection. |
| 10 | Signal transport | Choose one path | AGREE. Whichever path, add MCP layer on top. |

---

## Recommendations (Prioritized)

### P0: Foundational (blocks agent use entirely)

1. **Promote 38-tool catalog to ACs.** At minimum, add AC section requiring: Spec CRUD (4 tools), Insight CRUD (4 tools), Task CRUD + claim + complete (6 tools), Reservation CRUD (4 tools), `autarch_refresh_context`, `autarch_report_status`, `autarch_list_capabilities`. Total: 21 tools. Reference the existing design in `deep-dive-mcp-tool-surface.md`.

2. **Add context injection ACs.** Require that MCP session start injects current phase, confidence, pending findings, and available tools. Use the template from `deep-dive-mcp-tool-surface.md` lines 388-441.

3. **Replace heuristic completion with explicit signaling.** Revise AC-5.6 and AC-4.4 to use `autarch_complete_task` as primary, stall detection as crash-recovery fallback.

4. **Add agent-completion CUJ tests.** For each CUJ, an AC requiring that the journey can be completed via MCP tools alone, with the same end state as the TUI walkthrough.

### P1: Validates Architecture

5. **Add composability test.** At least one AC where a novel outcome is achieved by composing existing primitives without new code.

6. **Add Agent Capability Testing category.** Automated parity tests, context parity tests, and at least 3 "surprise tests" (open-ended domain requests).

7. **Move phase-to-hunter mapping to configuration.** Split AC-1.12 into primitives (list hunters, trigger hunter) and configurable defaults.

8. **Add improvement-over-time for Gurgeh and Coldwine.** `.gurgeh/context.md` for sprint preferences, `.coldwine/learnings.md` for task estimation accuracy.

### P2: Polish

9. **Add spec versioning MCP tools** (`autarch_spec_history`, `autarch_spec_diff`).

10. **Implement or remove `autarch_research` stub.** Currently misleads agents.

11. **Document CUJ transition paths for agents.** After completing CUJ-1, the agent should be able to discover that CUJ-2 and CUJ-3 are available.

12. **Explicit entity scope.** Clarify which of the 11 Intermute entity types are in the "7 entities" and which are deferred.

---

## What is Working Well

- **Shared workspace architecture.** File-based persistence in `.gurgeh/`, `.coldwine/`, `.pollard/` using YAML is exactly the agent-native shared workspace pattern. Both TUI and agents can read/write the same files. AC-X.7 verifies no binary blobs. This is a solid foundation.

- **Research feedback loop design.** The `.pollard/feedback.yaml` system with rolling window, cross-session learning, and global preferences (AC-3.6 through AC-3.9) is a textbook implementation of the improvement-over-time principle.

- **Degradation matrix.** The 2x2 matrix (Agent Teams on/off x Intermute on/off) with testable configurations per cell is excellent systems thinking. AC-X.5 and AC-X.9 cover graceful degradation.

- **38-tool catalog as research artifact.** The design in `deep-dive-mcp-tool-surface.md` is comprehensive, well-structured, and addresses every gap identified in this review. The authorization matrix, response envelope standard, and context injection template are production-ready designs. The gap is purely one of promotion from research to binding criteria.

- **Atomic spec versioning.** `SaveRevision` in `evolution.go` now uses `fileutil.AtomicWriteFile` with rollback on partial failure and file locking to prevent concurrent version collision. This addresses the concern raised in the plan's Research Insights section.

- **Sprint state persistence and resume.** The orchestrator supports `SaveSprintState`, `LoadSprintState`, `ListSprints`, and `Resume`. This enables the partial-completion pattern for sprints -- though it needs MCP tool exposure.

- **Deep copy for concurrency safety.** `SprintState.Clone()` with per-field deep copy (map cloning, slice copying) prevents data races between the orchestrator and TUI. This is well-tested and enables safe concurrent access patterns that agents would also benefit from.

---

## Agent-Native Score

- **5 of 48 user-facing capabilities have existing MCP tools** (10%)
- **2 of 55 acceptance criteria reference MCP tools** (4%)
- **0 of 5 CUJs are completable by an agent without the TUI** (0%)
- **0 of 5 CUJs have context injection specified** (0%)
- **1 of 7 entities has partial CRUD coverage via MCP** (14%)
- **A comprehensive 38-tool design exists but is not referenced by any AC**

**Verdict: NEEDS WORK.** The foundation (shared workspace, feedback loop, degradation matrix) is solid. The design (38-tool catalog, context injection template) is thorough. The gap is in the acceptance criteria, which verify TUI behavior exclusively and do not require any of the designed agent capabilities to be implemented.
