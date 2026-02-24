# Agent-native architecture skill

**Source:** Agent result from session b817bfb0
**Task ID:** a004fb5
**Date:** 2026-02-05

---

Now I have a thorough understanding of both the skill principles and the plan. Let me compile my full analysis.

---

## Agent-Native Architecture Analysis of the Autarch Acceptance Criteria Plan

### Methodology

Every principle from the agent-native-architecture skill was applied systematically against every acceptance criterion and open question in the plan. The analysis covers: Parity, Granularity, Composability, Emergent Capability, Improvement Over Time (including Self-Modification), plus the implementation-level disciplines of CRUD Completeness, Dynamic Capability Discovery, Completion Signals, Partial Completion, Context Injection, Shared Workspace, Action Parity, and Agent-Native Testing.

---

### 1. PARITY: "Whatever the user can do through the UI, the agent should be able to achieve through tools."

**Current State: FAILING.** The plan acknowledges this in its own "Agent-Native Architecture Assessment" section (lines 253-262), but the acceptance criteria do not remediate it. Only 2 of 7 entities (Findings, Reservations) have even partial MCP tools. The TUI exposes dozens of actions with no agent equivalent.

**Specific Gaps per CUJ:**

| TUI Action | AC Reference | Agent MCP Tool? | Status |
|------------|-------------|-----------------|--------|
| Start kickoff / codebase scan | AC-1.1 | None | MISSING |
| Advance sprint phase | AC-1.12 | None | MISSING |
| View/update confidence score | AC-1.8, AC-1.9 | None (Open Question #5 recommends read-only) | MISSING |
| Export spec to file | AC-1.10 | None | MISSING |
| Accept/edit spec change from finding | AC-1.7 | None | MISSING |
| Triage finding (accept/reject/defer) | AC-1.6 | AC-1.17 mentions it | PARTIAL |
| Request Deep Dive | AC-3.5 | None | MISSING |
| Init task breakdown | AC-2.1 | None | MISSING |
| Accept/edit/reject task proposal | AC-2.2 | None | MISSING |
| View dependency DAG | AC-2.6 | None | MISSING |
| Transition task state | AC-2.7 | None | MISSING |
| Spawn Agent Team | AC-2.8 | None | MISSING |
| Approve/reject teammate plan | AC-2.9 | None | MISSING |
| Acquire/release file reservation | AC-4.1, AC-4.4 | AC-4.7 mentions it | PARTIAL |
| View dashboard metrics | AC-5.2 | None | MISSING |
| Drill into project | AC-5.4 | None | MISSING |
| View team structure | AC-5.7 | None | MISSING |

**Recommendation:** Add a new acceptance criterion section "AC-P: Agent Parity" with a capability map table. Every row in the table above marked MISSING needs either (a) an MCP tool or (b) composition of existing primitives documented in the system prompt. The skill is explicit: "When adding any UI capability, ask: can the agent achieve this outcome? If not, add the necessary tools or primitives."

**Specific new ACs needed:**

- **AC-P.1**: A capability map exists as a maintained artifact mapping every TUI action to the agent tool(s) that achieve the same outcome.
- **AC-P.2**: For each CUJ, an agent can complete the entire journey using only MCP tools (no TUI required). Verification: script the CUJ as a sequence of MCP tool calls, assert same end state as TUI walkthrough.
- **AC-P.3**: Adding a new TUI action without a corresponding agent capability (or explicit "N/A" with justification) fails code review.

---

### 2. GRANULARITY: "Prefer atomic primitives. Features are outcomes achieved by an agent operating in a loop."

**Current State: MIXED.** The plan makes some correct choices (file-based persistence in YAML is inherently agent-friendly), but several acceptance criteria encode workflow logic that should be prompt-defined.

**Specific Violations:**

- **AC-1.12** (phase-appropriate hunters trigger automatically) encodes the Vision-to-GitHub-Scout mapping as a hardcoded rule. This is the "analyze_and_organize" anti-pattern. The skill says: "To change how a feature behaves, do you edit prose or refactor code?" Currently the answer is "refactor code" (edit `research_phases.go`).

  **Recommendation:** Split into two ACs:
  - AC-1.12a: Primitives exist -- `list_available_hunters`, `trigger_hunter(hunter_name, context)`, `get_phase_context`. These are Dynamic Capability Discovery tools.
  - AC-1.12b: Default phase-to-hunter mapping is defined in a prompt/config file (not hardcoded Go), so agents or users can customize it.

- **AC-1.14** (GoalFeature consistency check) hardcodes one of four consistency check types. The skill would recommend: provide `run_consistency_check(check_type)` as a primitive and define the GoalFeature logic in a prompt section, allowing new check types to be added as prompts.

  **Recommendation:** Add AC-1.14a: New consistency check types can be added by writing a prompt section without code changes.

- **AC-2.1** (Coldwine reads spec and proposes hierarchy) bundles the decision logic of hierarchy generation into a single tool call. The skill says domain tools should "represent one conceptual action from the user's perspective."

  **Recommendation:** Ensure the hierarchy proposal is decomposed: `read_spec`, `propose_initiative`, `propose_epic(initiative_id)`, `propose_story(epic_id)`, `propose_task(story_id)`. The agent composes them. A convenience tool `propose_full_hierarchy` is acceptable as a shortcut but must not be a gate.

---

### 3. COMPOSABILITY: "With atomic tools and parity, you can create new features just by writing new prompts."

**Current State: NOT TESTED.** No acceptance criterion verifies that a new feature can be composed from existing primitives without code changes. The plan's own assessment (line 261) identifies this gap.

**The Test:** "Can you add a new feature by writing a new prompt section, without adding new code?"

**Concrete examples the plan should include:**

- **AC-C.1**: A "weekly research digest" feature (summarize all findings from the past week, grouped by phase, with trend analysis) can be implemented as a prompt using existing `list_findings`, `read_spec`, and `write_file` tools. No new code required.
- **AC-C.2**: A "spec comparison" feature (compare two specs and identify divergence) can be implemented by prompting the agent to use `read_file` on two spec YAML files and produce a diff. No new tool required.
- **AC-C.3**: A "task re-estimation" feature (review completed tasks and adjust estimates for remaining ones) composes `list_tasks`, `read_task`, `update_task`. Prompt-only.

**Recommendation:** Add AC-C.1 through AC-C.3 as composability verification criteria. Each must pass the test: "an agent given only the existing MCP tool set and a new prompt section achieves a novel outcome."

---

### 4. EMERGENT CAPABILITY: "The agent can accomplish things you didn't explicitly design for."

**Current State: NOT TESTED.** The plan has no "surprise test" -- no acceptance criterion where an agent is given an open-ended request within the Autarch domain and assessed on whether it can figure out a reasonable approach.

**The Skill's Test:** "Give the agent an open-ended request relevant to your domain. Can it figure out a reasonable approach, operating in a loop until it succeeds?"

**Recommended ACs:**

- **AC-E.1**: Given the prompt "Cross-reference my PRD with the competitive landscape and identify claims that are no longer defensible", the agent uses `read_spec`, `list_findings`, and its judgment to produce a useful analysis without a dedicated "defensibility analysis" feature.
- **AC-E.2**: Given the prompt "Which of my tasks are risky based on the research findings?", the agent composes `list_tasks`, `list_findings`, and reasoning to flag tasks affected by research invalidation -- even though no `cross_reference_tasks_and_findings` tool exists.
- **AC-E.3**: If the agent responds with "I don't have a feature for that" to any reasonable domain request, this is a parity or granularity failure that must be fixed.

---

### 5. IMPROVEMENT OVER TIME: "Agent-native applications get better through accumulated context and prompt refinement."

**Current State: PARTIAL.** The plan includes `.pollard/feedback.yaml` (AC-3.6 through AC-3.9) and global preferences (AC-3.7), which is the "context.md pattern" applied to research triage. This is good. However, improvement mechanisms are limited to Pollard only.

**Gaps:**

- **No `context.md` equivalent for Gurgeh.** The agent learns nothing from past sprints. Prior spec decisions, recurring patterns, user preferences for phase depth -- none of this accumulates.

  **Recommendation:** Add AC-IMP.1: On sprint completion, the agent updates `.gurgeh/context.md` with decisions made, patterns observed, and user preferences. On session start, the agent reads this file.

- **No `context.md` equivalent for Coldwine.** Task estimation accuracy, common dependency patterns, typical approval feedback -- none of this improves over time.

  **Recommendation:** Add AC-IMP.2: Coldwine maintains `.coldwine/learnings.md` updated by the agent with task estimation accuracy (predicted vs actual duration), common blocking patterns, and approval feedback themes.

- **No self-modifying prompts.** The plan does not envision agents editing their own system prompts or behavior descriptions. This is the "advanced" tier from the skill and may be deferred, but should be acknowledged.

  **Recommendation:** Add to "Deferred Features" or "Original Intent": agents can propose modifications to their own prompt sections based on accumulated feedback, subject to approval gates.

---

### 6. CRUD COMPLETENESS: "Every entity has create, read, update, AND delete."

**Current State: CRITICALLY INCOMPLETE.** The plan identifies 7 entities. Here is the audit:

| Entity | Create | Read | Update | Delete | Notes |
|--------|--------|------|--------|--------|-------|
| Spec | Via TUI only | Via CLI `gurgeh list/export` | Via TUI only | None | 0/4 MCP tools |
| Finding | Pollard auto-creates | AC-1.17 `list_findings` | AC-1.17 `triage_finding` | None | 2/4 MCP tools |
| Task | AC-2.1 via Coldwine | None | AC-2.7 (state transition, TUI only) | None | 0/4 MCP tools |
| Reservation | AC-4.7 `reserve_paths` | None (can query Intermute API) | None | AC-4.7 `release_paths` | 2/4 MCP tools |
| Signal | Auto-emitted | None | None | None | 0/4 MCP tools |
| Team | Via Agent Teams | AC-5.7 (Bigend reads config) | None | None | 0/4 MCP tools |
| Confidence | Computed | AC-1.8 (TUI display only) | None (implicit from triage) | N/A | 0/4 MCP tools |

**Recommendation:** Add AC-CRUD.1: Every entity has full CRUD exposed as MCP tools. Minimum tool set:

```
Spec:        autarch_create_spec, autarch_read_spec, autarch_update_spec, autarch_delete_spec
Finding:     autarch_create_finding, autarch_list_findings, autarch_update_finding, autarch_delete_finding
Task:        autarch_create_task, autarch_read_task, autarch_update_task, autarch_delete_task
Reservation: autarch_reserve_paths, autarch_list_reservations, autarch_update_reservation, autarch_release_paths
Signal:      autarch_emit_signal, autarch_list_signals, autarch_acknowledge_signal, autarch_dismiss_signal
Team:        autarch_create_team, autarch_read_team, autarch_update_team (reassign), N/A delete
Confidence:  autarch_get_confidence (read-only is acceptable; it's computed)
```

The plan's Open Question #5 recommends "Start minimal in v1 (findings + reservations + confidence query), expand based on usage." The skill disagrees with minimalism here -- the skill's CRUD Completeness principle says "If any operation is missing, users will eventually ask for it and the agent will fail." The v1 minimum should be full CRUD for at least Spec, Finding, Task, and Reservation. Signal and Team can follow.

---

### 7. DYNAMIC CAPABILITY DISCOVERY vs STATIC TOOL MAPPING

**Current State: STATIC.** AC-1.12 hardcodes phase-to-hunter mappings. The plan describes 8 phases, each with specific hunters. This is the "static tool mapping" anti-pattern from the skill.

**The skill says:** "Build a meta-tool that discovers what's available, and a generic tool that can access anything."

**Recommendation:** Replace static mappings with:
- `autarch_list_available_hunters()` -- returns all registered hunters with capabilities
- `autarch_trigger_research(hunter_name, query, context)` -- triggers any hunter
- `autarch_get_phase_context(phase_name)` -- returns what the phase needs

The default phase-to-hunter mapping lives in a config/prompt, not in Go code. This is AC-1.12a/b from the Granularity section above.

---

### 8. COMPLETION SIGNALS: "Agent has explicit `complete_task` tool (not heuristic detection)."

**Current State: ANTI-PATTERN.** AC-5.6 defines stall detection as "agent silent for >5 minutes." This is explicitly called out as an anti-pattern in the skill under "Heuristic completion detection."

The skill states: "Detecting agent completion through heuristics (consecutive iterations without tool calls, checking for expected output files). This is fragile. Fix: Require agents to explicitly signal completion through a `complete_task` tool."

**Specific AC violations:**

- **AC-5.6** uses heuristic stall detection as the primary mechanism. The plan's Open Question #4 partially addresses this by recommending a `report_status` MCP tool, but does not replace the heuristic with an explicit signal.

- **AC-4.4** (reservation auto-releases on task completion) relies on detecting completion, but no AC specifies HOW completion is detected. If it's heuristic, reservations may linger or release prematurely.

**Recommendations:**

- **AC-5.6-REVISED**: Agents signal completion via `autarch_complete_task(task_id, summary, status)`. Stall detection is a BACKUP for crashed/hung agents (not the primary mechanism). Rename to "stall recovery" not "stall detection."
- **AC-COMP.1**: `autarch_complete_task` tool exists with parameters `task_id: string`, `summary: string`, `status: "success" | "partial" | "blocked"`.
- **AC-COMP.2**: `autarch_report_progress` tool exists for long-running tasks to reset the stall timer without completing.
- **AC-COMP.3**: Heuristic stall detection is documented as a crash-recovery mechanism, not a completion signal. Default timeout: 5 min code, 15 min research (per Open Question #4).
- **AC-4.4-REVISED**: Reservation releases on explicit `complete_task` call, agent disconnect, or stall-recovery timeout (not "task completion" detected heuristically).

---

### 9. PARTIAL COMPLETION: "Multi-step tasks track progress for resume."

**Current State: NOT ADDRESSED.** The plan has no acceptance criteria for partial completion or task resumption. The "Out for v1" section excludes Agent Teams session resumption, but does not address single-agent partial completion.

**Gaps:**

- If a sprint is interrupted mid-phase (terminal closed, crash), there is no AC for resuming from the last completed phase.
- If a Coldwine task breakdown is interrupted, there is no checkpoint mechanism.
- The skill says: "Checkpoint saved with current state. Resume continues from where it left off, not from beginning."

**Recommendations:**

- **AC-PC.1**: Sprint state persists after each phase completion. If the TUI is restarted, the sprint resumes from the last completed phase, not from kickoff.
- **AC-PC.2**: Task breakdown state persists incrementally. Partial hierarchies are recoverable.
- **AC-PC.3**: Agent progress is tracked with status (pending/in_progress/completed/failed/skipped) per sub-task. This is visible in Bigend.

---

### 10. DYNAMIC CONTEXT INJECTION: "System prompt includes what exists."

**Current State: NOT ADDRESSED.** No acceptance criterion specifies what context is injected into agent system prompts. The plan describes an "agent pane" that connects to the user's coding agent, but does not specify what the agent knows about Autarch state.

**The skill says:** "The user's context IS the agent's context." Inject: available resources, current state, capabilities mapping, and domain vocabulary.

**Specific gaps:**

- Agent does not know which sprint phase is active
- Agent does not know current confidence score
- Agent does not know which findings are pending review
- Agent does not know which tasks are blocked
- Agent does not know which file reservations are active
- Agent does not know available MCP tools by user vocabulary

**Recommendations:**

- **AC-CTX.1**: On agent session start, system prompt includes: current spec ID and phase, confidence score breakdown, count of pending findings by category, list of available MCP tools with user-vocabulary descriptions.
- **AC-CTX.2**: `autarch_refresh_context` tool exists. Calling it returns current state snapshot (spec phase, confidence, pending findings, active reservations, task status summary). For long sessions where state changes.
- **AC-CTX.3**: Domain vocabulary section in system prompt defines "finding", "triage", "sprint", "phase", "reservation", "signal", "confidence" in user terms. Agent never responds with "I don't understand what you mean by phase."
- **AC-CTX.4**: System prompt includes available capabilities mapped to natural language: "When the user says 'advance to the next phase', use `autarch_advance_phase`. When they say 'what's my confidence?', use `autarch_get_confidence`."

---

### 11. SHARED WORKSPACE: "Agent and user work in same data space."

**Current State: GOOD.** The plan's file-based persistence (`.gurgeh/`, `.coldwine/`, `.pollard/`) using YAML is exactly the shared workspace pattern. Both the TUI and agent can read/write the same files. AC-X.7 verifies no binary blobs.

**One gap:** AC-1.7 (edit preview) implies the TUI mediates spec changes from findings. The agent should also be able to apply finding suggestions directly to spec YAML without going through the TUI.

**Recommendation:** Add AC-SW.1: Agent can read and write `.gurgeh/specs/*.yaml` directly. Spec changes made by the agent via MCP tools are visible in the TUI on next refresh.

---

### 12. SELF-MODIFICATION

**Current State: NOT ADDRESSED (appropriate for v1).** The skill identifies self-modification as "advanced" and says "Start with a non-self-modifying prompt-native agent. Add self-modification when you need it."

**Recommendation:** No new ACs needed for v1. Add to "Original Intent / Deferred Features":
- Agents can propose edits to their own system prompt sections based on accumulated feedback
- Approval gates required for prompt self-modification
- Git-based versioning of prompt changes

---

### 13. AGENT-NATIVE TESTING

**Current State: NOT ADDRESSED.** The plan's Test Categories section (lines 322-370) uses traditional test categories (Manual, Integration, Unit, Race Condition). None of the agent-native testing patterns from the skill are present.

**Missing test types:**

- **"Can Agent Do It?" tests**: For each CUJ, can an agent complete it via MCP tools alone?
- **"Surprise tests"**: Open-ended requests within the Autarch domain
- **Parity tests**: Automated verification that every TUI action has a corresponding tool
- **Context parity tests**: Agent sees what the TUI shows
- **Natural language variation tests**: Multiple phrasings for same request all work

**Recommendations:**

- **AC-TEST.1**: Add "Agent Capability Testing" section. For each CUJ, a test exists where an agent completes the journey using only MCP tools.
- **AC-TEST.2**: Add "Parity Testing" section. Automated test reads the capability map and verifies every tool exists and is documented in the system prompt.
- **AC-TEST.3**: Add "Emergent Capability Testing" section. At least 3 open-ended domain requests are tested. Agent must not respond "I don't have a feature for that."
- **AC-TEST.4**: Context parity test verifies agent system prompt contains current spec ID, phase, confidence, and pending finding count.

---

### 14. PRODUCT IMPLICATIONS: Progressive Disclosure and Approval Patterns

**Current State: PARTIALLY ADDRESSED.** AC-1.7 (edit preview with confirmation before applying) is a good approval pattern -- high-stakes actions (modifying spec) require confirmation. However, the plan does not consistently apply approval levels to stakes.

**Gaps:**

- Finding triage (accept/reject/defer) has no approval gate -- appropriate since it's reversible.
- Reservation acquisition has no approval gate -- appropriate since it's mechanical.
- Spec export (AC-1.10) has no approval gate -- this creates an immutable artifact and should confirm.
- Task state transitions (AC-2.7) have no approval gate -- transitioning to "done" should confirm.

**Recommendation:** Add AC-APPR.1: Approval requirements match stakes and reversibility. Destructive or irreversible agent actions (spec export, task completion, reservation force-release) require confirmation. Reversible actions (triage, status query, finding list) proceed without confirmation.

---

### 15. SPECIFIC AC-BY-AC RECOMMENDATIONS

Below are all acceptance criteria that fall short of agent-native principles, with specific recommendations:

**AC-1.1** (Kickoff scan <10s): The plan already identifies this target as unrealistic. No agent-native issue.

**AC-1.6** (Agent triage via natural language): GOOD -- this is agent-native. However, it only specifies TUI agent pane. Add: "and via MCP tool `autarch_triage_finding`."

**AC-1.8** (Confidence score display): Violates parity. Agent cannot query confidence programmatically. Add: `autarch_get_confidence(spec_id)` tool returning the four-component breakdown.

**AC-1.12** (Phase-appropriate hunters): Violates granularity and dynamic capability discovery. See section 7 above.

**AC-1.17** (MCP tools for findings): GOOD starting point but incomplete. Only covers list and triage. Missing: create (for injecting external findings), update (for modifying relevance), delete (for removing duplicates).

**AC-2.2** (Task review with accept/edit/reject): Violates parity. These actions are TUI-only. Add corresponding MCP tools.

**AC-2.7** (Task state transitions): Violates parity. State transitions are TUI-only. Add `autarch_transition_task(task_id, new_state)`.

**AC-3.5** (Deep Dive trigger): Violates parity. Only described as TUI action. Add `autarch_deep_dive(finding_id)` MCP tool.

**AC-3.6** (Agent reads feedback.yaml on start): GOOD -- this is the context.md pattern. But verify it's in the system prompt, not just a convention.

**AC-4.7** (MCP tools for reservations): GOOD. Covers acquire and release. Missing: list (what's currently reserved?) and query (is this path available?).

**AC-5.2** (Dashboard metrics display): Violates parity. Agent cannot query metrics. Add `autarch_get_project_metrics(project_path)`.

**AC-5.6** (Stall detection): ANTI-PATTERN. See section 8 above. Replace heuristic with explicit completion signaling.

**AC-X.6** (Button fallback when agent unavailable): Appropriate -- progressive disclosure from agent to manual UI.

**AC-X.9** (All features work without Agent Teams): GOOD -- graceful degradation.

---

### 16. OPEN QUESTIONS: Agent-Native Verdicts

**Open Question #1 (Confidence threshold for export):** The plan's recommendation ("Advisory only -- always permit export") aligns with the parity principle. Agents should have the same capability as users; don't gate actions artificially. **Agree.**

**Open Question #2 (Button fallback reasoning):** The plan's recommendation (mandatory dropdown) is pragmatic. For agent-native, the richer approach is to ensure the feedback loop works even in degraded mode. **Agree.**

**Open Question #3 (Deep Dive timeout):** The plan's recommendation (configurable, partial results) aligns with the partial completion pattern from the skill. Never discard progress. **Agree.**

**Open Question #4 (Stall detection sensitivity):** The plan's recommendation (configurable per-task-type + `report_status` tool) is correct but does not go far enough. The primary mechanism should be `complete_task`, not stall detection. **Partially agree -- needs explicit completion tool as primary, stall detection as fallback only.**

**Open Question #5 (MCP tool surface area):** The plan recommends "Start minimal in v1." The skill says this is wrong: "If any operation is missing, users will eventually ask for it and the agent will fail." Full CRUD for core entities (Spec, Finding, Task, Reservation) is the v1 minimum. Signal and Team can follow. **Disagree with minimal approach.**

**Open Question #6 (Task list ownership):** The plan's recommendation (flatten to leaf tasks with context prefix) aligns with the granularity principle -- simple primitives with context, not complex hierarchies. Agent Teams gets flat claims; agents that need hierarchy use Coldwine's MCP tools. **Agree.**

**Open Question #7 (Token budget):** The plan's recommendation (show estimated cost) aligns with the context injection principle -- agents should have the same information as users. **Agree.**

**Open Question #8 (Bridge mechanism):** The plan correctly identifies this as blocking. The skill adds: define an `AgentTeamsClient` interface (as the plan suggests) and ensure it's exposed as MCP tools, not just internal Go interfaces. **Agree, strengthen with MCP exposure.**

**Open Question #9 (Glob overlap semantics):** Pure implementation issue, not agent-native architecture. But it blocks the parity principle (reservation tools can't work correctly without it). **Must resolve before MCP reservation tools are meaningful.**

**Open Question #10 (Signal transport):** The plan recommends signals publish through Intermute with `signal.` prefix. The agent-native angle: whichever path is chosen, ensure `autarch_list_signals` and `autarch_subscribe_signals` MCP tools expose signals to agents regardless of transport. **Agree, add MCP layer on top.**

---

### 17. SUMMARY OF NEW ACCEPTANCE CRITERIA NEEDED

| Category | New ACs | Priority |
|----------|---------|----------|
| Agent Parity (AC-P) | AC-P.1 capability map, AC-P.2 agent-only CUJ completion, AC-P.3 parity enforcement | P0 -- foundational |
| CRUD Completeness (AC-CRUD) | AC-CRUD.1 full CRUD for 7 entities | P0 -- foundational |
| Completion Signals (AC-COMP) | AC-COMP.1 `complete_task` tool, AC-COMP.2 `report_progress` tool, AC-COMP.3 stall detection as fallback | P0 -- replaces anti-pattern |
| Context Injection (AC-CTX) | AC-CTX.1 session start context, AC-CTX.2 `refresh_context` tool, AC-CTX.3 vocabulary, AC-CTX.4 capability mapping | P0 -- enables all agent use |
| Composability (AC-C) | AC-C.1 through AC-C.3 prompt-only features | P1 -- validates architecture |
| Emergent Capability (AC-E) | AC-E.1 through AC-E.3 surprise tests | P1 -- validates architecture |
| Partial Completion (AC-PC) | AC-PC.1 sprint resume, AC-PC.2 task breakdown resume, AC-PC.3 progress tracking | P1 -- reliability |
| Improvement Over Time (AC-IMP) | AC-IMP.1 Gurgeh context.md, AC-IMP.2 Coldwine learnings.md | P1 -- long-term value |
| Agent-Native Testing (AC-TEST) | AC-TEST.1 through AC-TEST.4 | P1 -- verification |
| Dynamic Discovery | AC-1.12a/b hunter discovery | P2 -- flexibility |
| Shared Workspace | AC-SW.1 agent direct spec editing | P2 -- parity |
| Approval Patterns | AC-APPR.1 stakes-matched approval | P2 -- product quality |

---

### 18. THE ULTIMATE TEST

The skill's "ultimate test" is: *Describe an outcome to the agent that's within your application's domain but that you didn't build a specific feature for. Can it figure out how to accomplish it?*

Under the current plan, the answer is **no**. An agent asked "Compare my current PRD's competitive assumptions against the latest research findings and flag anything that needs updating" would fail because:
1. No MCP tool to read the spec programmatically
2. No MCP tool to list findings with metadata
3. No MCP tool to update spec sections
4. No context injection telling the agent what tools are available

After implementing the P0 recommendations above, this request would succeed by composing `autarch_read_spec`, `autarch_list_findings(filter: {category: "competitive"})`, and `autarch_update_spec(section, content)`. That is the difference between a TUI-with-agent-pane and a genuinely agent-native system.