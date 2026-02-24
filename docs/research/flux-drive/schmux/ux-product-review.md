# schmux UX & Product Review for Autarch

**Reviewer:** Flux-drive User & Product Reviewer
**Date:** 2026-02-11
**Repository Reviewed:** /tmp/schmux
**Target:** Autarch (Go + Bubble Tea TUI monorepo for AI agent development tools)

---

## Executive Summary

schmux is a multi-agent orchestration tool built around **tmux sessions + web dashboard + git-native workflow**. Its most innovative features are **NudgeNik** (LLM-based agent state interpretation) and **Agent Signaling Protocol** (dual-format status signaling). The product philosophy emphasizes human-in-the-loop coordination, avoiding autonomous agent-to-agent communication.

**Key insight:** schmux solves **attention allocation** — helping developers know where to focus when running many agents in parallel. This is directly relevant to Autarch's multi-tool orchestration problem.

**Recommendation:** Autarch should adopt the **signaling philosophy** (agents declare state, not just log output) and the **interpretation layer concept** (LLM reads agent output to classify needs). Both can enhance Coldwine task orchestration and Bigend dashboard without requiring schmux's tmux dependency.

---

## 1. NudgeNik: LLM-Based Agent State Interpretation

### What It Does

NudgeNik reads the **last 100 lines of terminal output** from an agent and classifies its state using an LLM oneshot call.

**States:**
- Blocked (needs permission/authorization)
- Waiting (has questions, needs clarification)
- Working (actively making progress)
- Done (completed all work)

**Philosophy:** "The future isn't binary orchestration — it's interpretation and judgment."

### Technical Implementation

```go
// internal/nudgenik/nudgenik.go
func AskForSession(ctx context.Context, cfg *config.Config, sess state.Session) (Result, error) {
    // 1. Capture last 100 lines from tmux session
    content := tmux.CaptureLastLines(ctx, sess.TmuxSession, 100, false)

    // 2. Extract latest agent response (strips old output)
    extracted := ExtractLatestFromCapture(content)

    // 3. Send to LLM with structured prompt
    response := oneshot.ExecuteTarget(ctx, cfg, targetName, Prompt + extracted,
                                      oneshot.SchemaNudgeNik, timeout)

    // 4. Parse JSON result: {state, confidence, evidence, summary}
    return ParseResult(response)
}
```

**Prompt structure:**
- "You are analyzing the last response from a coding agent."
- "Choose exactly ONE state from: Needs Authorization | Needs Feature Clarification | Needs User Testing | Completed"
- Stylistic rules: "Do NOT use 'agent', 'model', 'system', or 'it'. Begin directly with the situation."

**Fallback behavior:**
- If agent hasn't signaled directly in 5+ minutes → NudgeNik kicks in
- If agent supports direct signaling → NudgeNik is skipped (saves compute)

### Evidence Quality

**Strengths:**
- Handles ambiguity that binary state machines miss (real software development is messy)
- Cheap to run (oneshot LLM call, ~1-2s)
- Works with any agent output format (terminal text is universal interface)
- Stylistic rules keep output professional (no anthropomorphization)

**Weaknesses:**
- Requires last 100 lines to be representative (can misfire if agent dumps logs)
- Timeout adds latency to status updates (15s max)
- No multi-turn context — each analysis is independent
- False negatives when agent is "thinking" (no output = no state change)

### Applicability to Autarch

**High value for:**
- **Coldwine task orchestration:** Classify task agent state without manual polling
- **Bigend dashboard:** Show task status glanceable indicators
- **Pollard:** Classify research agent progress (working vs needs input)

**Integration path:**
- Autarch already has interclode (Codex dispatch) — add NudgeNik-style classifier for Task agents
- Use Intermute signals to broadcast state changes to Bigend dashboard
- Create `pkg/nudge/` package with pluggable backends (LLM, heuristic rules, direct signals)

**Design adaptation:**
- schmux uses tmux capture → Autarch could use transcript files or in-memory buffers
- schmux uses oneshot LLM calls → Autarch could batch analyze multiple tasks or use streaming
- schmux extracts "latest response" → Autarch needs task-specific extraction logic (Codex vs Claude transcripts differ)

---

## 2. Agent Signaling Protocol: Structured Status Communication

### What It Does

Agents **explicitly signal their state** instead of relying on output parsing. schmux supports two formats:

**Bracket-based markers (recommended):**
```
--<[schmux:completed:Implementation finished]>--
--<[schmux:needs_input:Should I delete these 5 files?]>--
--<[schmux:needs_testing:Please try the new search feature]>--
--<[schmux:error:Build failed - missing dependency]>--
--<[schmux:working:]>--
```

**OSC 777 escape sequences:**
```bash
printf '\x1b]777;notify;completed;Task done\x07'
printf '\x1b]777;notify;needs_input;Waiting for approval\x07'
```

**Key principle:** "Agents signal WHAT attention they need. schmux/dashboard controls HOW to notify the user."

### Automatic Provisioning

On session spawn, schmux auto-creates instruction files:
- Claude Code → `.claude/CLAUDE.md`
- Codex → `.codex/AGENTS.md`
- Gemini → `.gemini/GEMINI.md`

Content wrapped in markers for safe updates:
```markdown
<!-- SCHMUX:BEGIN -->
## Schmux Status Signaling
...instructions...
<!-- SCHMUX:END -->
```

**Environment variables injected:**
- `SCHMUX_ENABLED=1` — agent can detect schmux environment
- `SCHMUX_SESSION_ID=myproj-abc-xyz12345` — unique identifier
- `SCHMUX_WORKSPACE_ID=myproj-abc` — workspace context

### Signal Flow

1. Agent outputs signal (bracket marker or OSC 777)
2. tmux captures via pipe-pane to log file
3. schmux WebSocket reads log, detects signal via regex
4. Signal is validated (must match valid schmux state)
5. Session nudge state is updated
6. **Signal is stripped from output before browser display**
7. Dashboard broadcasts update to all connected clients

### Evidence Quality

**Strengths:**
- **Deterministic** — no LLM guessing, agent declares ground truth
- **Instant** — no 15s timeout, real-time updates
- **Cheap** — regex parsing, no API calls
- **Invisible** — users never see signal markers (stripped from terminal)
- **Opt-in** — works with or without agent support (NudgeNik fallback)
- **Standard format** — OSC 777 is recognized by terminals (VSCode, rxvt-unicode)

**Weaknesses:**
- Requires agent adoption (must output markers)
- Agents can forget to signal (silent failures)
- No validation that agent's declared state matches reality (can claim "completed" while tests fail)
- Bracket format is schmux-specific (not a standard)

### Applicability to Autarch

**Extremely high value for:**
- **Interclode dispatches:** Codex agents could output `--<[autarch:completed:...]>--` markers
- **Gurgeh spec generation:** Signal when spec is ready for validation
- **Coldwine task orchestration:** Tasks declare completion/blockage directly
- **Intermute coordination:** Standardize cross-tool signaling

**Integration path:**
- Define Autarch-native signal format: `--<[autarch:state:message]>--`
- Add provisioning to interclode dispatch script (inject into Codex AGENTS.md)
- Create `pkg/signals/parser.go` with regex detection + stripping
- Update Intermute to broadcast parsed signals as `SignalMsg` events
- Extend Bigend dashboard to display task signals as status badges

**Design adaptation:**
- schmux strips signals from terminal → Autarch could strip from transcript before UI display
- schmux uses WebSocket streaming → Autarch uses Intermute signals (already implemented)
- schmux environment vars → Autarch could inject via `AUTARCH_ENABLED=1`, `AUTARCH_TASK_ID=...`
- schmux auto-provisions `.claude/CLAUDE.md` → Autarch could provision via interclode `--inject-docs`

**Critical insight:** This separates **WHAT** (agent needs attention) from **HOW** (notification mechanism). Autarch's Intermute already handles the HOW — we need to standardize the WHAT.

---

## 3. Session Lifecycle & Workspace Persistence

### Session States

```
spawning → running → done → disposed
```

- **Spawning:** Creating workspace + starting agent
- **Running:** Agent actively working
- **Done:** Agent exited, session preserved for review
- **Disposed:** Removed from tracking, tmux session deleted

**Key design:** Sessions persist after agent exit. Workspace remains until explicitly disposed. This supports **post-mortem review** and **resumable work**.

### Workspace Management

**Git-native design:**
- Workspaces are real directories: `~/schmux-workspaces/myproject-001`, `myproject-002`
- Uses `git worktree` for efficiency (shared object store)
- Multiple agents can work in same workspace simultaneously
- Workspaces survive agent crashes/network disconnects

**Safety checks:**
- Cannot dispose workspace with uncommitted changes
- Cannot dispose workspace with unpushed commits
- Git status shown in dashboard (dirty indicator, ahead/behind, branch name)

### Applicability to Autarch

**Medium value — philosophy more than implementation:**
- Autarch doesn't use tmux (uses TUI), so session lifecycle differs
- Autarch doesn't need git worktrees (tasks are ephemeral, not long-running clones)
- Autarch's Coldwine tasks could adopt "done but not disposed" pattern (preserve output for review)

**Integration path:**
- Add "completed tasks" archive in Coldwine (don't auto-delete task outputs)
- Preserve Codex agent transcripts after task completion
- Add "Review Completed" tab in Coldwine dashboard

**Key takeaway:** Don't auto-delete work artifacts. Let humans decide when to dispose.

---

## 4. Spawn Wizard UX: Prompt-First Design

### Design Philosophy

**Prompt-first single-page interface:**
- Large textarea at top for task description
- Slash commands in textarea: `/command`, `/resume`
- AI-powered branch naming (auto-generated from prompt)
- Parallel target configuration (select multiple agents)
- One-click "Engage" button

### Form State Persistence (3-Layer Model)

**Layer 1: Mode Logic (Entry Point)**
- URL parameters: `workspace_id` for existing workspace spawns
- React Router state: `repo`, `branch`, `prompt`, `nickname` for prefilled mode

**Layer 2: Session Storage (Active Draft)**
- Per-tab, survives page refresh
- Key: `spawn-draft-{workspace_id}` or `spawn-draft-fresh`
- Auto-saved as user types
- **Cleared on successful spawn**

**Layer 3: Local Storage (Long-term Memory)**
- Cross-tab, survives browser close
- `spawn-last-repo`, `spawn-last-target-counts`, `spawn-last-model-selection-mode`
- **Never auto-cleared**
- Updated on successful spawn (write-back pattern)

### Model Selection Modes

| Mode | Description | Behavior |
|------|-------------|----------|
| `single` | One agent only | Radio button (clicking deselects others) |
| `multiple` | Multiple agents | Toggles (0 or 1 per agent) |
| `advanced` | Full control | Counter buttons (0-10 per agent) |

### Evidence Quality

**Strengths:**
- **Prompt-first** reduces cognitive load (describe task before choosing tools)
- **3-layer persistence** prevents data loss while avoiding clutter
- **AI branch naming** saves time and enforces naming consistency
- **Slash commands** provide advanced features without cluttering default UI
- **Write-back to localStorage** learns user preferences without explicit settings

**Weaknesses:**
- Complex state management (3 layers with precedence rules)
- AI branch naming requires LLM call (adds latency)
- Mode switching can confuse users (single → multiple clears selections)
- Session storage draft is tab-specific (can't share across tabs)

### Applicability to Autarch

**High value for:**
- **Gurgeh spec creation:** Adopt prompt-first approach for new specs
- **Coldwine task spawning:** Use 3-layer persistence for task creation form
- **Bigend dashboard:** AI-powered naming for missions/sprints

**Integration path:**
- Add "Create Spec" modal in Gurgeh with prompt-first design
- Persist Gurgeh form state in session storage (survive crashes)
- Add AI naming for Coldwine tasks via interclode oneshot call
- Use localStorage for "last used tool" preferences (e.g., last Pollard hunter)

**Design adaptation:**
- schmux uses React Router state → Autarch uses Bubble Tea messages
- schmux uses browser storage APIs → Autarch could use `.autarch/state.json`
- schmux auto-saves on keystroke → Autarch could save on Ctrl+S or blur events
- schmux clears draft on success → Autarch should do the same (prevent stale data)

**Critical insight:** **Prompt-first** changes the mental model from "pick tool then describe task" to "describe task then pick tools." This is more natural and reduces tool-fixation bias.

---

## 5. Philosophy: Human as Coordinator

### Non-Goals (Explicitly Stated)

schmux is deliberately NOT:
- Agent-to-agent orchestration (human is always coordinator)
- A cloud service (local-only)
- A sandbox/container system (real filesystem)
- Batch/headless automation (interactive by design)
- Trying to hide git (git is foundation, not implementation detail)

### Design Implications

**Human-in-the-loop:**
- Dashboard shows real-time agent output (not hidden)
- Agents don't coordinate autonomously (no agent→agent messaging)
- User decides when to dispose workspaces (no auto-cleanup)
- Git workflow is explicit (branches, commits, diffs visible)

**Local-first:**
- No hosted infrastructure, no accounts
- Everything runs on one machine
- No multi-node coordination

### Evidence Quality

**Strengths:**
- Clear boundaries prevent scope creep
- "Human as coordinator" aligns with Clerky's development workflow
- Local-first simplifies deployment (no cloud dependencies)
- Git-native design leverages existing developer knowledge

**Weaknesses:**
- No autonomous agent coordination limits scalability
- Single-machine architecture prevents distributed work
- Interactive-only design excludes CI/CD integration
- Local-first prevents team collaboration features

### Applicability to Autarch

**High alignment with Autarch philosophy:**
- Autarch already emphasizes human-in-the-loop (Gurgeh validation, Coldwine task approval)
- Autarch is local-first (no cloud dependencies)
- Autarch uses git for project sync (not hidden)

**Integration path:**
- Document Autarch's non-goals explicitly (like schmux does)
- Avoid autonomous agent-to-agent orchestration in Coldwine
- Keep Intermute as message bus, not agent coordinator
- Preserve human approval gates in task flows

**Key takeaway:** Explicitly stating non-goals prevents feature requests that contradict core philosophy.

---

## 6. Quick Launch Presets & Cookbooks

### Quick Launch Design

**Schema:**
```json
{
  "quick_launch": [
    {
      "name": "Run Tests",
      "command": "npm test"
    },
    {
      "name": "Review Changes",
      "target": "claude-sonnet",
      "prompt": "Review these changes for bugs and style issues"
    }
  ]
}
```

**Rules:**
- Shell command: set `command`
- AI agent: set `target` + `prompt`
- Either/or: use command OR target+prompt, not both

**Global vs Workspace:**
- Global: `~/.schmux/config.json` (all repos)
- Workspace: `<workspace>/.schmux/config.json` (repo-specific)
- Merge behavior: workspace overrides global on name conflicts

### Evidence Quality

**Strengths:**
- **Reduces repetitive typing** for common operations
- **Workspace-specific presets** support per-project workflows
- **Simple JSON schema** is easy to hand-edit
- **One-click execution** from dashboard

**Weaknesses:**
- No variables/templating (can't pass arguments)
- No conditional logic (can't "run tests only if dirty")
- No chaining (can't "run tests then deploy")
- Merge behavior can surprise users (global preset silently overridden)

### Applicability to Autarch

**High value for:**
- **Gurgeh:** Quick launch for "Create Security Spec", "Create Performance Spec"
- **Coldwine:** Presets for "Review All Open PRs", "Run All Tests"
- **Pollard:** Presets for "Scan GitHub Repos", "Generate Competitor Report"
- **Bigend:** Dashboard quick actions

**Integration path:**
- Add `quick_launch` array to `.autarch/config.json`
- Parse presets at startup (similar to Gurgeh specs)
- Render as buttons in dashboard (Gurgeh tab, Coldwine tab, Pollard tab)
- Support both CLI commands and agent dispatches

**Design adaptation:**
- schmux uses web dropdown → Autarch could use Bubble Tea list selector
- schmux workspace config → Autarch could use `.autarch/workspace-config.json`
- schmux merges global+workspace → Autarch should document precedence clearly

**Critical insight:** Quick launch presets **encode team knowledge** — they're living documentation of "how we use this tool."

---

## 7. Git Workflow Sync (Sync from Main / Sync to Main)

### Features

**Sync from Main:**
- Brings commits from `origin/main` into current branch via **iterative cherry-pick**
- Handles both "behind" and "diverged" states
- Creates temporary WIP commit before syncing (preserves local changes)
- Aborts if conflicts detected

**Sync to Main:**
- Pushes branch commits directly to main via **fast-forward**
- Requires clean workspace state (no uncommitted changes, not behind main)
- Two workflow styles:
  - On-main workflow: push directly when already on main
  - Feature branch workflow: set upstream to main, sync locally after push

### Evidence Quality

**Strengths:**
- **Linear history** — no merge commits
- **Conflict detection** — aborts early, doesn't leave repo in bad state
- **WIP commit safety** — local changes preserved during sync
- **Dashboard-integrated** — available from git status dropdown

**Weaknesses:**
- Cherry-pick can fail silently if commits don't apply cleanly
- No automatic conflict resolution
- Sync to main requires manual upstream setup (not automatic)
- Fast-forward-only limits workflow flexibility

### Applicability to Autarch

**Low value — Autarch uses trunk-based development:**
- Autarch commits directly to main (no feature branches)
- Autarch doesn't need sync workflows
- Autarch's git operations are simpler (commit, push, done)

**Integration path:**
- None recommended — Autarch's git workflow is already optimized

**Key takeaway:** schmux solves a problem Autarch doesn't have (multi-branch coordination). Don't add features without user pain.

---

## Novel Concepts Autarch Should Adopt

### 1. Agent Signaling Protocol (CRITICAL)

**Problem it solves:** Agents log output but don't declare state. Humans must infer "is it done?" from terminal output.

**Autarch solution:**
- Define `--<[autarch:state:message]>--` marker format
- Add parser to `pkg/signals/parser.go`
- Inject signaling instructions via interclode `--inject-docs`
- Broadcast parsed signals via Intermute
- Display in Bigend dashboard as status badges

**User benefit:** Glanceable task status without reading transcripts.

**Adoption path:**
1. Write `pkg/signals/protocol.md` spec
2. Add provisioning to interclode dispatch script
3. Implement parser + stripper in `pkg/signals/`
4. Wire into Intermute signal bus
5. Update Bigend TUI to display signal-based status

---

### 2. NudgeNik-Style State Interpretation (HIGH VALUE)

**Problem it solves:** Agents don't always signal. Need fallback classification.

**Autarch solution:**
- Add `pkg/nudge/classifier.go` with LLM-based state detection
- Use as fallback when no signal received for N minutes
- Classify task agent state: Working | Needs Input | Needs Review | Completed | Error
- Store classification in Coldwine task metadata

**User benefit:** Attention allocation — know which tasks need review first.

**Adoption path:**
1. Define state taxonomy (reuse schmux states or customize)
2. Write prompt template (reference schmux's prompt engineering)
3. Implement classifier with timeout + error handling
4. Add to Coldwine task polling loop
5. Display classification in Coldwine dashboard

---

### 3. Prompt-First UI Design (MEDIUM VALUE)

**Problem it solves:** Tool-first design creates fixation bias ("I need Claude" vs "I need code review").

**Autarch solution:**
- Reorder Gurgeh spec creation: prompt → template → tools
- Add AI-powered naming for specs/tasks (via interclode oneshot)
- Persist form drafts to survive crashes (sessionStorage equivalent)
- Write back to config on success (learn preferences)

**User benefit:** Lower cognitive load, fewer abandoned workflows.

**Adoption path:**
1. Redesign Gurgeh spec creation modal (move prompt to top)
2. Add AI naming endpoint (interclode oneshot call)
3. Implement draft persistence (`.autarch/drafts.json`)
4. Add write-back on success (`.autarch/preferences.json`)

---

### 4. Quick Launch Presets (HIGH VALUE)

**Problem it solves:** Repetitive typing for common operations.

**Autarch solution:**
- Add `quick_launch` to `.autarch/config.json`
- Support CLI commands and agent dispatches
- Render as dashboard buttons (per-tool tab)
- Allow workspace-specific overrides

**User benefit:** One-click execution of common workflows.

**Adoption path:**
1. Define `quick_launch` schema in `pkg/agenttargets/config.go`
2. Parse at startup (merge global + workspace configs)
3. Render in Bubble Tea dashboard (list selector or buttons)
4. Wire to existing dispatch/CLI logic

---

### 5. Session Persistence Philosophy (MEDIUM VALUE)

**Problem it solves:** Work artifacts auto-deleted, preventing post-mortem review.

**Autarch solution:**
- Don't auto-delete Coldwine task outputs
- Add "Completed Tasks" archive in Coldwine
- Preserve Codex agent transcripts after completion
- Let users explicitly dispose when ready

**User benefit:** Can review what agents did after task completion.

**Adoption path:**
1. Change Coldwine task lifecycle: add "completed" state (not "disposed")
2. Preserve transcript files in `.autarch/tasks/completed/`
3. Add "Review Completed" tab in Coldwine dashboard
4. Add explicit "Dispose" action (with confirmation)

---

## Overlap with Existing Autarch Features

### Intermute Signal Bus (ALREADY IMPLEMENTED)

schmux uses WebSocket for real-time updates. Autarch already has Intermute for cross-tool signaling.

**Overlap:** Both solve agent→dashboard communication.

**Advantage:** Autarch's Intermute is more flexible (REST + WebSocket + in-process).

**Action:** Use Intermute to broadcast agent signals (no new infra needed).

---

### Interclode Agent Dispatch (ALREADY IMPLEMENTED)

schmux spawns agents via CLI exec. Autarch has interclode plugin for Codex dispatch.

**Overlap:** Both run external agents with prompts.

**Advantage:** Autarch's interclode includes `--inject-docs`, `--name`, `--dry-run` flags.

**Action:** Extend interclode to provision agent signaling instructions.

---

### Multi-Tool Dashboard (ALREADY IMPLEMENTED)

schmux has web dashboard with session monitoring. Autarch has Bigend TUI with 4 tabs.

**Overlap:** Both provide observability for multiple agent types.

**Advantage:** schmux streams terminal output in real-time (Autarch uses polling).

**Action:** Consider adding real-time transcript streaming to Bigend (via Intermute WebSocket).

---

### Git-Native Workflow (ALREADY IMPLEMENTED)

schmux uses git worktrees for workspaces. Autarch uses trunk-based development (direct to main).

**Overlap:** Both treat git as first-class citizen.

**Advantage:** Autarch's trunk-based flow is simpler (no branch management overhead).

**Action:** None — Autarch's git workflow is already optimized.

---

## What Autarch Should NOT Adopt

### 1. tmux Dependency (WRONG FIT)

schmux relies on tmux for session persistence. Autarch uses Bubble Tea TUI.

**Why not:** Autarch's TUI is the UI, not tmux. Adding tmux would add complexity without benefit.

**Alternative:** Use transcript files + Intermute for session state (already working).

---

### 2. Git Worktrees (NO USER PAIN)

schmux uses worktrees for multi-workspace efficiency. Autarch commits directly to main.

**Why not:** Autarch doesn't spawn multiple workspaces per project. No disk space problem to solve.

**Alternative:** Keep current trunk-based workflow.

---

### 3. Web Dashboard Architecture (INCOMPATIBLE)

schmux uses Go backend + React frontend + WebSocket. Autarch uses Bubble Tea TUI.

**Why not:** Autarch's TUI is terminal-native. Adding web UI would fragment user experience.

**Alternative:** Keep Bubble Tea TUI, optionally add Intermute REST API for external tools.

---

### 4. Workspace Overlays (NO USER PAIN)

schmux auto-copies `.env` files to workspaces. Autarch doesn't create workspaces.

**Why not:** Autarch agents run in-place (no workspace clones). No file copying needed.

**Alternative:** None — no problem to solve.

---

### 5. External Diff Tool Integration (LOW VALUE)

schmux launches Kaleidoscope/VS Code diffs from dashboard. Autarch users work in terminal.

**Why not:** Autarch users already have `git diff` in terminal. Adding GUI diff is scope creep.

**Alternative:** None — use existing git CLI tools.

---

## Feature Recommendations by Priority

### P0: Critical (Implement First)

1. **Agent Signaling Protocol** — Define `--<[autarch:state:message]>--` format, add parser, provision via interclode
2. **NudgeNik-Style Classifier** — LLM fallback for task state detection (Coldwine)
3. **Intermute Signal Integration** — Broadcast agent signals via existing Intermute bus

### P1: High Value (Next Phase)

4. **Quick Launch Presets** — One-click common operations (Gurgeh, Coldwine, Pollard)
5. **Prompt-First UI** — Reorder Gurgeh spec creation, add AI naming
6. **Session Persistence** — Don't auto-delete task outputs, add "Completed Tasks" archive

### P2: Nice to Have (Defer)

7. **Form Draft Persistence** — Save Gurgeh/Coldwine form state to survive crashes
8. **Real-Time Transcript Streaming** — Add WebSocket transcript feed to Bigend (optional)

### P3: Out of Scope (Don't Implement)

- tmux dependency
- Git worktrees
- Web dashboard (separate from TUI)
- Workspace overlays
- External diff tool integration
- Git workflow sync (Autarch uses trunk-based)

---

## Product Validation: Evidence Standards

### What schmux Validates

**User pain:**
- "Running multiple agents in parallel is tedious" → **Confirmed** (schmux exists to solve this)
- "Hard to know which agent needs attention" → **Confirmed** (NudgeNik solves this)
- "Repetitive typing for common operations" → **Confirmed** (quick launch solves this)

**Solution fit:**
- NudgeNik directly addresses attention allocation
- Agent signaling directly addresses state ambiguity
- Quick launch directly addresses repetitive typing

**Evidence quality:**
- **Anecdotal** (author's own workflow) — no user research cited
- **Dogfooding** (built by schmux, used by schmux) — high confidence in real-world use
- **Philosophy doc** (explicit non-goals) — clear product boundaries

### What Autarch Should Validate Before Adoption

**User pain in Autarch context:**
- Do Autarch users run multiple tasks in parallel? (check Coldwine usage)
- Do Autarch users struggle to know task status? (check support requests)
- Do Autarch users repeat the same commands? (check command history)

**Solution fit for Autarch:**
- Does agent signaling work with Codex agents? (test interclode with signal markers)
- Does NudgeNik classification work with task transcripts? (test on real Coldwine logs)
- Do quick launch presets reduce cognitive load? (test with 3-5 users)

**Recommended validation:**
1. **Signaling:** Run interclode dispatch with provisioned `.codex/AGENTS.md`, verify signals appear
2. **Classification:** Run NudgeNik-style classifier on 10 Coldwine task transcripts, measure accuracy
3. **Quick Launch:** Add 3 presets to `.autarch/config.json`, measure usage over 1 week

---

## Flow Analysis: Agent Signaling End-to-End

### Happy Path

1. User spawns Coldwine task via Bigend dashboard
2. Interclode dispatch provisions `.codex/AGENTS.md` with signaling instructions
3. Codex agent reads instructions, starts work
4. Agent outputs `--<[autarch:working:]>--` (state cleared)
5. Parser strips marker from transcript, broadcasts `SignalMsg{State: "working"}` via Intermute
6. Bigend dashboard receives signal, updates task status badge (🔄 Working)
7. Agent finishes implementation
8. Agent outputs `--<[autarch:completed:Feature implemented successfully]>--`
9. Parser broadcasts `SignalMsg{State: "completed", Message: "Feature implemented successfully"}`
10. Bigend dashboard updates badge (✓ Completed)
11. User reviews task output, marks as done

**Time to value:** Immediate (real-time status updates)

**User confidence:** High (agent declared completion, no guessing needed)

---

### Error Path: Agent Forgets to Signal

1. User spawns task
2. Agent starts work (no signal output)
3. Parser sees no signals for 5 minutes
4. NudgeNik classifier kicks in (LLM analyzes last 100 lines of transcript)
5. Classification result: "Waiting" (agent asked a question)
6. Broadcaster sends `SignalMsg{State: "waiting", Source: "llm"}`
7. Bigend dashboard shows ⚠ badge (Waiting, LLM-classified)
8. User reviews transcript, sees question, responds
9. Agent continues work

**Time to value:** 5-minute delay (classifier timeout)

**User confidence:** Medium (LLM guessed, might be wrong)

---

### Edge Case: Agent Claims Completion But Tests Failed

1. Agent outputs `--<[autarch:completed:All tests passing]>--`
2. Parser broadcasts completion signal
3. Bigend shows ✓ Completed badge
4. User reviews output, sees test failures in transcript
5. **Signal was wrong** — agent lied or misunderstood

**Mitigation:**
- Add validation layer (parse test output, compare with signal)
- Show "Agent declared completed" vs "Verified completed" states
- Use NudgeNik as second opinion (compare signal with LLM classification)

**Design insight:** Signals are **self-reported**, not verified. Treat as hints, not ground truth.

---

## User Impact Assessment

### Positive Impacts

**Agent Signaling:**
- **New users:** Easier onboarding (status is explicit, not inferred)
- **Advanced users:** Faster triage (scan badges, no transcript reading)
- **Occasional users:** Re-engagement (notifications when attention needed)

**NudgeNik Classification:**
- **All users:** Fallback when agents don't signal (no silent failures)
- **Power users:** Multi-task monitoring (dashboard shows all states at glance)

**Quick Launch Presets:**
- **New users:** Discover features (presets are living docs)
- **Advanced users:** Speed (one-click common operations)
- **Teams:** Shared knowledge (team presets in git)

### Negative Impacts

**Agent Signaling:**
- **Migration cost:** Existing agents don't signal (need provisioning retrofit)
- **False confidence:** Users trust signals even when wrong
- **Visual clutter:** Too many badges can overwhelm dashboard

**NudgeNik Classification:**
- **Latency:** 15s LLM call delays status updates
- **Cost:** LLM API calls add expense (mitigated by 5-minute throttle)
- **Inaccuracy:** LLM can misclassify agent state (especially "thinking" vs "done")

**Quick Launch Presets:**
- **Discovery:** Hidden in config file (users don't know they exist)
- **Maintenance:** Stale presets accumulate (no auto-cleanup)
- **Collision:** Workspace presets silently override global (confusion)

### User-Side Failure Modes

**Agent signaling fails:**
- Agent outputs signal but parser misses it (regex bug)
- Agent outputs signal but state is invalid (typo: `complted` instead of `completed`)
- Agent outputs signal but message contains newline (parser expects single line)

**Recovery:** NudgeNik fallback after 5 minutes (graceful degradation)

**NudgeNik fails:**
- LLM times out (15s max) → status stays stale
- LLM returns invalid JSON → parser error
- LLM misclassifies state → user sees wrong badge

**Recovery:** Manual transcript review (user clicks task to see full output)

**Quick launch fails:**
- Preset command fails (bad syntax) → error shown in terminal
- Preset prompt is outdated (stale instructions) → agent asks for clarification
- Preset targets wrong tool (tool not installed) → spawn fails

**Recovery:** Edit config file, fix preset, retry

---

## Detailed Evidence: schmux's Product Design Strengths

### 1. Philosophy-First Documentation

schmux leads with `docs/PHILOSOPHY.md`, not implementation details. This is **product thinking**, not engineering thinking.

**Why it matters:** Forces clarity on what problem schmux solves and for whom.

**Autarch equivalent:** `AGENTS.md` should lead with user problems, not architecture.

---

### 2. Explicit Non-Goals

schmux states what it will NOT be:
- Not agent-to-agent orchestration
- Not a cloud service
- Not batch automation

**Why it matters:** Prevents scope creep and manages user expectations.

**Autarch equivalent:** Add "Non-Goals" section to `AGENTS.md`.

---

### 3. Human-Centric Design Language

NudgeNik prompt rules:
- "Do NOT use 'agent', 'model', 'system', or 'it'"
- "Do NOT anthropomorphize"
- "Begin directly with the situation"

**Why it matters:** Keeps output professional and actionable (not cute).

**Autarch equivalent:** Apply same rules to Intermute signal messages.

---

### 4. Dual-Format Signaling

Supporting both bracket markers and OSC 777 shows **pragmatism over purity**.

**Why it matters:** Meets agents where they are (terminal output vs escape sequences).

**Autarch equivalent:** Support both explicit signals and heuristic detection (NudgeNik fallback).

---

### 5. Write-Back to Config

Spawn wizard saves successful configurations to localStorage (learns user preferences).

**Why it matters:** Reduces repetitive choices without explicit "Save Preferences" UI.

**Autarch equivalent:** Save last-used Gurgeh template, Coldwine task type, Pollard hunter.

---

## Recommendations Summary

### Adopt from schmux

1. **Agent Signaling Protocol** — `--<[autarch:state:message]>--` format (P0)
2. **NudgeNik-Style Classifier** — LLM fallback for state detection (P0)
3. **Prompt-First UI** — Reorder Gurgeh spec creation (P1)
4. **Quick Launch Presets** — One-click common operations (P1)
5. **Session Persistence** — Preserve task outputs for review (P1)

### Don't Adopt from schmux

1. **tmux Dependency** — Autarch uses Bubble Tea, not tmux
2. **Git Worktrees** — Autarch uses trunk-based development
3. **Web Dashboard** — Autarch TUI is terminal-native
4. **Workspace Overlays** — Autarch doesn't create workspaces
5. **Git Workflow Sync** — Autarch commits directly to main

### Validate Before Building

1. **User pain:** Do Autarch users struggle with task status visibility?
2. **Solution fit:** Does agent signaling work with Codex transcripts?
3. **Success metrics:** Define measurable improvement (e.g., "time to find blocked task" drops 50%)

---

## Final Verdict

**schmux is excellent product design masquerading as a developer tool.**

The key insight: **Agents need two communication channels:**
1. **Output channel** (terminal/transcript) — what they did
2. **State channel** (signals) — what they need

Most orchestration tools only have #1. schmux adds #2 via **explicit signaling** and **LLM interpretation**.

**For Autarch:** Adopt the signaling philosophy (agents declare state) and the interpretation layer (LLM classifies needs). Both can enhance Coldwine task orchestration without requiring schmux's tmux/web architecture.

**Implementation priority:** Start with agent signaling (deterministic, cheap, instant). Add NudgeNik classifier as fallback (graceful degradation when signals missing).

**Success criteria:**
- Bigend dashboard shows glanceable task status (no transcript reading required)
- Task triage time drops 50% (measured via user timing studies)
- Agent completion detection is 95%+ accurate (measured against ground truth test runs)

---

## Appendix: schmux Package Structure

```
internal/
  nudgenik/          # LLM-based state classification
    nudgenik.go      # AskForSession, AskForCapture, ParseResult

  signal/            # Agent signal parsing
    signal.go        # ParseSignals, ExtractAndStripSignals, MapStateToNudge

  provision/         # Agent instruction provisioning
    provision.go     # EnsureAgentInstructions, RemoveAgentInstructions

  session/           # Session management
    manager.go       # Spawn, Dispose, Attach

  workspace/         # Workspace management
    manager.go       # Create, Clone, Dispose

  dashboard/         # Web dashboard
    handlers.go      # HTTP API endpoints
    websocket.go     # Real-time terminal streaming

  tmux/              # tmux CLI wrapper
    tmux.go          # CaptureLastLines, NewSession, KillSession

  config/            # Configuration
    config.go        # Load, Validate, GetNudgenikTarget

  state/             # Session state
    state.go         # Session, Workspace, LastSignalAt
```

Autarch equivalent structure:
```
pkg/
  signals/           # Agent signal parsing (new)
    parser.go        # ParseSignals, StripSignals
    protocol.md      # Signal format spec

  nudge/             # State classification (new)
    classifier.go    # ClassifyState, ParseResult

  agenttargets/      # Agent dispatch (existing)
    config.go        # Add quick_launch field
    provision.go     # Add signaling instruction injection

  claude/            # Claude agent (existing)
    run.go           # Add signal detection

  intermute/         # Cross-tool signals (existing)
    signals.go       # Add SignalMsg type
```
