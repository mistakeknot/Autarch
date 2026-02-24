# Agent-Native Architecture Review: schmux

**Reviewer**: Claude Sonnet 4.5
**Date**: 2026-02-11
**Repository**: /tmp/schmux
**Scope**: Multi-agent orchestration tool CLI ↔ Dashboard parity assessment

---

## Executive Summary

schmux demonstrates **strong agent-native fundamentals** with nearly complete action parity between CLI and HTTP API. The architecture treats agents as first-class citizens through environment variable injection, auto-provisioned instruction files, and a clean signaling protocol. However, the system stops short of full agent-native design—agents can *execute* via the orchestrator but cannot *discover* capabilities or *initiate* orchestration themselves. This is intentional: schmux's philosophy positions humans as coordinators, not agents.

**Key Strengths**:
- Complete CLI/API parity (spawn, list, attach, dispose)
- Environment variable injection for agent self-awareness
- Auto-provisioning of instruction files with signaling protocol
- NudgeNik meta-agent pattern for LLM-interpreting-LLM output
- Workspace-as-primitive design (git directories, not abstractions)

**Key Gaps** (by design, not oversight):
- No agent API for discovering quick launch presets or available targets
- No agent-triggered workspace creation or disposal
- Dashboard-only features (diff viewer, VS Code launcher)
- Static instruction files (no runtime context injection)

**Verdict**: **Agent-accessible but not agent-native** — schmux enables agents to work effectively but deliberately preserves human coordination. For an orchestrator, this is appropriate; for Autarch's agent development tools, these gaps reveal patterns worth learning from.

---

## 1. Action Parity Assessment

### Capability Map

| User Action | UI Location | CLI Command | HTTP API | Agent Tool | Status |
|-------------|-------------|-------------|----------|------------|--------|
| **Session Management** |
| Spawn session | Dashboard spawn form | `schmux spawn -t <target>` | `POST /api/spawn` | Via CLI or API | ✅ Full parity |
| List sessions | Dashboard home | `schmux list` | `GET /api/sessions` | Via CLI or API | ✅ Full parity |
| Attach to session | Click "Attach" | `schmux attach <id>` | N/A (tmux only) | Via CLI | ✅ Full parity |
| Dispose session | Click "Dispose" | `schmux dispose <id>` | `POST /api/sessions/<id>/dispose` | Via CLI or API | ✅ Full parity |
| Resume session | Dashboard spawn (resume flag) | `schmux spawn --resume` | `POST /api/spawn` (resume: true) | Via CLI or API | ✅ Full parity |
| **Workspace Management** |
| Create workspace | Implicit on spawn | Implicit on spawn | Implicit via `POST /api/spawn` | Via spawn | ✅ Full parity |
| Scan workspaces | Dashboard refresh | N/A (automatic) | `POST /api/workspaces/scan` | Via API | ⚠️ Dashboard-triggered only |
| Dispose workspace | Click "Dispose" | N/A | `POST /api/workspaces/<id>/dispose` | Via API | ⚠️ No CLI command |
| Refresh overlay | Dashboard button | `schmux refresh-overlay <id>` | `POST /api/workspaces/<id>/refresh-overlay` | Via CLI or API | ✅ Full parity |
| View git diff | Dashboard diff viewer | N/A | `GET /api/diff/<id>` | Via API | ⚠️ Dashboard-only UI |
| Open in VS Code | Dashboard button | `schmux code <id>` | `POST /api/open-vscode/<id>` | Via CLI or API | ✅ Full parity |
| Sync from main | Dashboard dropdown | N/A | `POST /api/workspaces/<id>/linear-sync-from-main` | Via API | ⚠️ Dashboard-only |
| Sync to main | Dashboard dropdown | N/A | `POST /api/workspaces/<id>/linear-sync-to-main` | Via API | ⚠️ Dashboard-only |
| **Configuration** |
| View config | Dashboard settings | N/A | `GET /api/config` | Via API | ⚠️ No CLI command |
| Update config | Dashboard settings | Manual edit | `POST /api/config` | Via API | ⚠️ No CLI command |
| Manage secrets | Dashboard settings | Manual edit | `POST /api/auth/secrets`, `POST /api/models/<id>/secrets` | Via API | ⚠️ No CLI command |
| **Discovery** |
| List quick launch | Dashboard spawn form | N/A | N/A | ❌ Not exposed | ❌ No agent access |
| List run targets | Dashboard spawn form | N/A | `GET /api/detect-tools` | Via API | ⚠️ Detection only, not user targets |
| List models | Dashboard model picker | N/A | `GET /api/models` | Via API | ⚠️ Read-only |
| Check NudgeNik status | Dashboard session card | N/A | `GET /api/hasNudgenik`, `GET /api/askNudgenik/<id>` | Via API | ✅ Programmatic |
| List recent branches | Dashboard spawn form | N/A | `GET /api/recent-branches` | Via API | ⚠️ Read-only |
| List PRs | Dashboard PR tab | N/A | `GET /api/prs`, `POST /api/prs/refresh` | Via API | ⚠️ Read-only |
| Checkout PR | Dashboard PR tab | N/A | `POST /api/prs/checkout` | Via API | ⚠️ Dashboard-driven |

### Analysis

**Strong Parity Areas**:
1. **Core session operations** — spawn, list, attach, dispose all have CLI + API + documented behavior
2. **Workspace overlays** — refresh command exists in CLI, API, and docs
3. **VS Code integration** — both CLI (`schmux code`) and API (`POST /api/open-vscode`)
4. **Resume functionality** — supported across all interfaces (CLI flag, API field, UI checkbox)

**Weak Parity Areas**:
1. **Git workflow sync** — "sync from main" and "sync to main" are dashboard-only (no CLI commands)
2. **Workspace disposal** — API exists, no CLI command (only session disposal via CLI)
3. **Configuration management** — dashboard has full UI, CLI requires manual file editing
4. **Discovery** — no agent API to list quick launch presets, user-defined targets, or workspace configs

**Orphan Features** (UI/API without CLI):
- Workspace scan (`POST /api/workspaces/scan`) — dashboard button only
- Git sync operations (linear-sync-from-main, linear-sync-to-main) — dashboard dropdowns only
- Config CRUD (`GET /api/config`, `POST /api/config`) — dashboard settings UI only
- PR discovery (`GET /api/prs`, `POST /api/prs/refresh`, `POST /api/prs/checkout`) — dashboard PR tab only

**Why This Matters**:
Agents working via CLI have full spawn/dispose/attach capabilities but cannot:
- Discover available quick launch presets for a workspace
- Trigger git sync operations programmatically
- Modify config (repos, targets, quick launch) without manual file editing
- Discover or initiate PR review workflows

This is **philosophically consistent** with schmux's "human is the coordinator" stance (see `docs/PHILOSOPHY.md`), but it means agents cannot autonomously orchestrate other agents or adapt to workspace-specific tooling.

---

## 2. Agent Signaling and Context Injection

### Environment Variables (Full Agent Self-Awareness)

schmux injects three environment variables into every spawned session:

```bash
SCHMUX_ENABLED=1              # Agent can detect orchestrator presence
SCHMUX_SESSION_ID=myproj-abc-xyz12345  # Unique session identifier
SCHMUX_WORKSPACE_ID=myproj-abc         # Workspace identifier
```

**Location**: `internal/session/manager.go:289-291` and `394-396`
**Injection Point**: Before tmux session creation, merged into command environment

**Verdict**: ✅ **Excellent** — agents have full self-awareness. Can conditionally enable signaling, log session context, or adapt behavior based on orchestration.

**Example Usage**:
```python
import os

def signal_schmux(state: str, message: str = ""):
    if os.environ.get("SCHMUX_ENABLED") == "1":
        print(f"--<[schmux:{state}:{message}]>--")

signal_schmux("completed", "Tests passing")
```

### Instruction File Auto-Provisioning

schmux auto-creates/updates agent instruction files on workspace creation:

- **Claude**: `.claude/CLAUDE.md`
- **Codex**: `.codex/AGENTS.md`
- **Gemini**: `.gemini/instructions.md`

**Content**: Markdown block with schmux signaling syntax, state definitions, examples
**Location**: `internal/provision/provision.go:75-135`
**Markers**: `<!-- SCHMUX:BEGIN -->` and `<!-- SCHMUX:END -->` for idempotent updates

**Strengths**:
1. ✅ Agents automatically know how to signal without prior configuration
2. ✅ Markdown format preserves agent-native instruction files (doesn't clobber user content)
3. ✅ Idempotent updates via markers (can refresh instructions across schema changes)

**Weaknesses**:
1. ❌ **Static content** — no runtime context injection (e.g., "you are in workspace X with repos Y, quick launch presets Z")
2. ❌ **No capability discovery** — agents learn signaling syntax but not what actions are available
3. ❌ **Generic instructions** — same content for all workspaces, doesn't reflect workspace-specific config

**Example Content** (`internal/provision/provision.go:21-72`):
```markdown
## Schmux Status Signaling

This workspace is managed by schmux. Signal your status to help the user monitor your progress.

### How to Signal

Output this marker **on its own line** in your response:
--<[schmux:state:message]>--

### Available States

| State | When to Use |
|-------|-------------|
| `completed` | Task finished successfully |
| `needs_input` | Waiting for user confirmation, approval, or choice |
| `needs_testing` | Implementation ready for user to test |
| `error` | Something failed that needs user attention |
| `working` | Starting new work (clears previous status) |
```

**Missing Context**:
- What quick launch presets are available in this workspace?
- What run targets are configured (claude, codex, custom tools)?
- What repos are accessible?
- What recent branches or PRs exist?

**Comparison to Autarch**:
Autarch's Intermute and agent context layers inject **runtime state** into system prompts (available specs, recent tasks, cross-tool signals). schmux's provisioning is a **static template**, closer to a README than a dynamic context layer.

**Recommendation for Autarch**:
Adopt schmux's **marker-based idempotent updates** (e.g., `<!-- AUTARCH:BEGIN -->` blocks in AGENTS.md) but extend with **runtime context injection** (available specs, tasks, signals, capabilities).

---

## 3. Signaling Protocol (Agent → Orchestrator Communication)

### Bracket Marker Syntax

Agents signal status via **on-its-own-line markers**:

```
--<[schmux:state:message]>--
```

**States**: `needs_input`, `needs_testing`, `completed`, `error`, `working`
**Parser**: `internal/signal/signal.go:87-115` (regex-based, ANSI-stripping, timestamp injection)
**Consumption**: Dashboard updates session card with state + message

**Strengths**:
1. ✅ **Simple protocol** — no API calls, no file writes, just stdout
2. ✅ **ANSI-aware** — strips terminal escape sequences before parsing (handles cursor movement, colors)
3. ✅ **Unobtrusive** — markers are stripped from terminal output, invisible to users
4. ✅ **Multi-line robust** — requires signals on their own line (prevents false matches in code blocks or docs)

**Weaknesses**:
1. ❌ **One-way only** — agents signal *to* orchestrator but cannot receive responses
2. ❌ **No structured data** — `message` is a string, not JSON (limits rich payloads)
3. ❌ **No acknowledgment** — agents don't know if signals were received
4. ❌ **Brittle regex** — `(?m)^[⏺•\-\*\s]*--<\[schmux:(\w+):([^\]]*)\]>--[ \t]*\r*$` handles bullets and whitespace but could break with creative markdown

**Example Workflow**:
1. User spawns Claude session with "Add user authentication"
2. Claude reads `.claude/CLAUDE.md`, sees signaling instructions
3. Claude outputs `--<[schmux:working:]>--` at start
4. Claude implements feature, runs tests
5. Claude outputs `--<[schmux:needs_testing:Please try logging in]>--`
6. Dashboard shows session card: "Needs User Testing: Please try logging in"
7. User attaches, tests feature, approves
8. Claude outputs `--<[schmux:completed:Authentication added]>--`

**Alternative Signaling Method: OSC 777** (documented but not implemented):
`docs/agent-signaling.md:236-247` mentions OSC 777 escape sequences for terminal-based signaling. Not used in practice (bracket markers are sufficient for text-based agents).

**Verdict**: ✅ **Solid for status signaling**, but ❌ **not a full agent communication channel**. Agents can broadcast state but cannot query orchestrator, request resources, or coordinate with other agents.

---

## 4. NudgeNik (Meta-Agent Pattern)

### Concept: LLM Interpreting LLM Output

NudgeNik is a **meta-agent** — it reads terminal output from other agents and uses an LLM to classify their state.

**Architecture**:
1. Session manager captures last N lines from tmux session (`tmux capture-pane`)
2. NudgeNik extracts "latest response" (strips prompts, isolates agent output)
3. Sends agent output + classification prompt to an LLM (configurable target: claude, codex, model)
4. Parses JSON response with `state`, `confidence`, `evidence`, `summary`
5. Updates session state in dashboard

**Location**: `internal/nudgenik/nudgenik.go:19-186`
**Prompt**: `internal/nudgenik/nudgenik.go:19-52` (system prompt with state definitions)
**States**: "Needs Authorization", "Needs Feature Clarification", "Needs User Testing", "Completed"

**Example Flow**:
```
Agent Output:
"I've implemented login. Should I proceed with OAuth integration or use basic auth?"

NudgeNik Prompt:
"You are analyzing the last response from a coding agent. Choose ONE state: Needs Authorization, Needs Feature Clarification, Needs User Testing, Completed."

NudgeNik Response (JSON):
{
  "state": "Needs Feature Clarification",
  "confidence": "high",
  "evidence": ["Agent is asking for design decision", "Multiple implementation paths mentioned"],
  "summary": "Implementation is blocked on OAuth vs basic auth decision"
}

Dashboard Display:
Session card shows: "Needs Feature Clarification: Implementation is blocked on OAuth vs basic auth decision"
```

**Strengths**:
1. ✅ **Robust fallback** — works even if agents don't emit signals (or emit wrong signals)
2. ✅ **Human-centric summaries** — prompt explicitly forbids "agent", "model", "it" (e.g., "Implementation is blocked..." not "The agent is blocked...")
3. ✅ **Configurable target** — can use any detected tool or model (not hardcoded to one LLM)
4. ✅ **Extensible** — prompt can be updated to detect new states or patterns

**Weaknesses**:
1. ❌ **Expensive** — every status check costs an LLM call (15s timeout per `nudgenik.go:54`)
2. ❌ **Latency** — not real-time (polls on user request or dashboard load)
3. ❌ **No memory** — each NudgeNik call is stateless (doesn't know previous states or conversation history)
4. ❌ **Signal override** — if agent emits bracket signals, NudgeNik still runs (wastes tokens)

**Comparison to Autarch**:
Autarch's **Arbiter** (multi-LLM consensus for spec validation) is similar — meta-agent pattern for interpreting agent work. Key difference: Arbiter is **multi-turn** and **stateful** (tracks proposals, evidence, confidence over time), while NudgeNik is **one-shot** and **stateless**.

**Recommendation for Autarch**:
Consider **hybrid signaling** — agents emit structured signals when confident, meta-agent interprets ambiguous output. NudgeNik's "stylistic rules" (no anthropomorphizing) could improve Arbiter output clarity.

---

## 5. Tool Design: Primitives vs Workflows

### Spawn as Primitive

`schmux spawn` is a **primitive** — accepts data, minimal logic:

```bash
# CLI
schmux spawn -t claude -p "Add authentication" -r myproject -b feature-auth

# API
POST /api/spawn
{
  "repo": "git@github.com:user/myproject.git",
  "branch": "feature-auth",
  "targets": { "claude": 1 },
  "prompt": "Add authentication"
}
```

**What it does**:
1. Resolve repo URL from config (if `-r` flag used)
2. Create or find workspace (git clone or worktree)
3. Provision instruction files (`.claude/CLAUDE.md`)
4. Inject environment variables (`SCHMUX_ENABLED=1`)
5. Launch agent in tmux session
6. Return session ID

**Decision Points**:
- ✅ **Data-driven**: Repo, branch, target, prompt are inputs (not decisions)
- ✅ **No hidden logic**: Doesn't decide which target to use, which model, or how to split work
- ✅ **Composable**: CLI wraps API wraps session manager (clean layers)

**Verdict**: ✅ **Excellent primitive design** — spawn is a noun (session creation), not a verb (orchestration strategy).

### Quick Launch as Workflow (But Exposed as Data)

Quick launch presets encode **workflows** (target + prompt combinations) but are stored as **data** in config:

```json
{
  "quick_launch": [
    {
      "name": "Review Changes",
      "target": "claude-sonnet",
      "prompt": "Review these changes for bugs and style issues"
    }
  ]
}
```

**Problem**: Agents cannot discover or trigger quick launch presets programmatically.

**Current State**:
- ✅ Dashboard renders quick launch buttons (click to spawn)
- ✅ API supports `POST /api/spawn` with `quick_launch_name` field
- ❌ No API to list available quick launch presets (`GET /api/quick-launch`)
- ❌ No API to list workspace-specific presets (stored in `<workspace>/.schmux/config.json`)

**Impact**:
Agents working via CLI/API cannot:
- Discover what quick launch presets exist
- Adapt behavior based on repo-specific tooling
- Trigger orchestration workflows (e.g., "spawn 3 reviewers" preset)

**Recommendation for Autarch**:
Expose quick launch presets as **agent-readable data** (GET endpoint or injected into instruction files). Autarch's **CUJ (Capability Understanding Journals)** pattern could formalize this: every workspace publishes a JSON manifest of available actions.

---

## 6. Shared Workspace Architecture

### Git Directories as Ground Truth

schmux workspaces are **real git directories**, not abstractions:

```
~/schmux-workspaces/
├── myproject-001/          # Workspace (git worktree)
│   ├── .git                # Git metadata (worktree link)
│   ├── .schmux/
│   │   └── config.json     # Workspace-specific quick launch
│   ├── .claude/
│   │   └── CLAUDE.md       # Provisioned instructions
│   └── src/                # Project files
├── myproject-002/          # Another workspace
│   └── ...
```

**Strengths**:
1. ✅ **Transparent** — users can `cd` into workspaces, use git CLI, edit files directly
2. ✅ **No sync layer** — agents and users work in the same directories (not isolated sandboxes)
3. ✅ **Tool-agnostic** — VS Code, IDEs, and CLI tools work normally (no special integration needed)
4. ✅ **Git-native** — diffs, commits, branches are real git operations (not schmux abstractions)

**Weaknesses**:
1. ⚠️ **Concurrent write conflicts** — multiple agents in same workspace can clobber each other's changes (no locking)
2. ⚠️ **No workspace versioning** — no snapshots or rollback (relies on git)
3. ⚠️ **Local-only** — workspaces are filesystem directories (not portable or remoteable without extra setup)

**Comparison to Autarch**:
Autarch stores specs in `.gurgeh/specs/`, tasks in `.coldwine/tasks/`, but doesn't formalize "workspace" as a concept. schmux's **workspace overlays** (auto-copy `.env`, configs) could inspire Autarch's **multi-project context injection** (e.g., auto-detect `.gurgeh/`, `.coldwine/` dirs and inject available specs/tasks into agent prompts).

**Recommendation for Autarch**:
Adopt **workspace-scoped config files** (`.autarch/config.json`) that agents can discover at runtime. Use schmux's `.gitignore` check pattern to prevent accidental tracking of overlay files.

---

## 7. Discovery Gaps

### What Agents Cannot See

Agents working via CLI or API **cannot programmatically discover**:

1. **Quick Launch Presets** (global or workspace-specific)
   - No `GET /api/quick-launch` endpoint
   - No `GET /api/workspaces/<id>/config` endpoint
   - Must manually parse `.schmux/config.json` files

2. **User-Defined Run Targets**
   - `GET /api/detect-tools` only lists detected tools (claude, codex, gemini)
   - User targets (defined in `run_targets` config) not exposed via API
   - Agents cannot discover custom tools or validate target names before spawn

3. **Workspace-Specific Tooling**
   - No API to query "what can I do in this workspace?"
   - No manifest of available commands, scripts, or presets
   - Agents must infer capabilities from filesystem inspection

4. **Recent Branches and PRs**
   - `GET /api/recent-branches` exists but only lists branch names (no metadata on quick launch or repo-specific context)
   - `GET /api/prs` exists but agents cannot trigger PR checkout (`POST /api/prs/checkout` requires workspace setup first)

**Why This Matters**:
A fully agent-native system would allow agents to:
- Query: "What quick launch presets exist for this repo?"
- Adapt: "This workspace has a 'Run Tests' preset, I'll use that instead of guessing the command"
- Coordinate: "I'll spawn a reviewer using the 'Review: Kimi' preset"

**Current Workaround**:
Agents can read config files directly (`.schmux/config.json` is JSON), but this is:
- ❌ **Fragile** — no schema validation, breaks if config format changes
- ❌ **Incomplete** — workspace-specific configs are scattered across multiple files
- ❌ **Undocumented** — no API contract for config file structure

**Recommendation for Autarch**:
Create **agent-readable capability manifests**:
- `GET /api/workspaces/<id>/capabilities` → lists quick launch, run targets, overlays
- Inject capabilities into instruction files (runtime context, not static template)
- Formalize as **CUJ pattern** (Capability Understanding Journals)

---

## 8. Dashboard-Only Features

### Features Without CLI/API Equivalents

| Feature | Dashboard UI | CLI | API | Agent Access |
|---------|--------------|-----|-----|--------------|
| **Git Sync Operations** |
| Sync from main | Dropdown on git status | ❌ No | `POST /api/workspaces/<id>/linear-sync-from-main` | Via API only |
| Sync to main | Dropdown on git status | ❌ No | `POST /api/workspaces/<id>/linear-sync-to-main` | Via API only |
| **Diff Viewing** |
| Built-in diff viewer | Workspace row button | ❌ No | `GET /api/diff/<id>` | Via API (raw data) |
| External diff tool | Dropdown on diff viewer | ❌ No | `POST /api/diff-external/<id>` | Via API only |
| **PR Workflows** |
| List PRs | PR tab | ❌ No | `GET /api/prs` | Via API (read-only) |
| Refresh PR list | PR tab button | ❌ No | `POST /api/prs/refresh` | Via API only |
| Checkout PR | PR tab button | ❌ No | `POST /api/prs/checkout` | Via API only |
| **Configuration** |
| Edit config | Settings UI | Manual file edit | `POST /api/config` | Via API only |
| Manage secrets | Settings UI | Manual file edit | `POST /api/auth/secrets`, `POST /api/models/<id>/secrets` | Via API only |

**Analysis**:
These features are **orchestrator-centric** (humans use dashboard to coordinate agents) rather than **agent-accessible** (agents use tools to coordinate themselves).

**Why Dashboard-Only?**
1. **Git sync** — sensitive operations (rebase, cherry-pick) require human judgment
2. **Diff viewing** — visual comparison is inherently human-centric
3. **PR workflows** — discovery and review are human decisions
4. **Config management** — global settings affect all agents (requires human oversight)

**Is This a Problem?**
**For schmux's philosophy, no** — "The human is always the coordinator" (see `docs/PHILOSOPHY.md:24`).
**For Autarch's tools, yes** — agents building agents need to discover capabilities, trigger workflows, and coordinate autonomously.

**Recommendation for Autarch**:
Distinguish **user-facing tools** (Bigend dashboard) from **agent-facing tools** (Gurgeh CLI, Intermute API). User tools can be dashboard-only; agent tools must have CLI/API parity.

---

## 9. Lessons for Autarch

### What to Adopt

1. **Environment Variable Injection**
   - ✅ `AUTARCH_ENABLED=1`, `AUTARCH_SPEC_ID`, `AUTARCH_TASK_ID` in every agent spawn
   - Enables agents to conditionally adapt behavior based on orchestration context

2. **Marker-Based Instruction Provisioning**
   - ✅ `<!-- AUTARCH:BEGIN -->` blocks in AGENTS.md for idempotent updates
   - Allows schema evolution without clobbering user content

3. **Bracket Signal Syntax for Agent Status**
   - ✅ `--<[autarch:state:message]>--` for agent → orchestrator communication
   - Simple, stdout-based, ANSI-aware, unobtrusive

4. **Meta-Agent Pattern (NudgeNik)**
   - ✅ LLM-interpreting-LLM for ambiguous agent output
   - Complements Autarch's Arbiter (multi-LLM consensus)

5. **Workspace Overlays (Auto-Copy Ignored Files)**
   - ✅ `.gitignore` check pattern prevents accidental tracking
   - Useful for `.env`, secrets, local configs

6. **Git Directories as Primitives**
   - ✅ No abstraction layer — agents work in real git repos
   - Transparent, tool-agnostic, git-native

### What to Improve Upon

1. **Runtime Context Injection**
   - ❌ schmux provisions static templates — no "you have specs X, Y, Z available"
   - ✅ Autarch should inject runtime state (specs, tasks, signals) into instruction files or system prompts

2. **Agent-Readable Capability Manifests**
   - ❌ schmux has no `GET /api/workspaces/<id>/capabilities` endpoint
   - ✅ Autarch should expose available actions (specs, tasks, tools) via API

3. **Bidirectional Signaling**
   - ❌ schmux signals are one-way (agent → orchestrator)
   - ✅ Autarch should allow orchestrator → agent communication (e.g., "your spec was merged", "task was reassigned")

4. **Structured Signal Payloads**
   - ❌ schmux signals are `state:message` strings
   - ✅ Autarch should use JSON payloads for rich data (e.g., `{"state": "completed", "spec_id": "PRD-001", "confidence": 0.95}`)

5. **Quick Launch Discoverability**
   - ❌ schmux quick launch is dashboard-only (no agent API)
   - ✅ Autarch should expose presets programmatically (GET endpoints, injected into context)

6. **Concurrent Access Coordination**
   - ❌ schmux allows multiple agents in same workspace (no locking or conflict resolution)
   - ✅ Autarch should use file reservations (Agent Mail pattern) or operational transforms

---

## 10. Recommendations for Autarch

### Immediate Actions (Low-Hanging Fruit)

1. **Add Environment Variables to Agent Spawns**
   - Inject `AUTARCH_ENABLED=1`, `AUTARCH_TOOL=gurgeh|coldwine|pollard`, `AUTARCH_SPEC_ID`, `AUTARCH_TASK_ID`
   - Location: `pkg/agenttargets/backend_claude.go`, `backend_codex.go` (when spawning subagents)

2. **Adopt Bracket Signal Syntax**
   - Define `--<[autarch:state:data]>--` protocol for agent status
   - States: `working`, `needs_input`, `completed`, `error`, `spec_updated`, `task_completed`
   - Parser in `pkg/signals/` (similar to schmux's `internal/signal/`)

3. **Create Workspace-Scoped Config Files**
   - `.autarch/config.json` in project root with:
     - Available specs (from `.gurgeh/specs/`)
     - Active tasks (from `.coldwine/tasks/`)
     - Suggested prompts or quick launch presets
   - Auto-detect and inject into agent context at spawn time

4. **Expose Capability Discovery API**
   - `GET /api/specs` (list available specs in current project)
   - `GET /api/tasks` (list active tasks)
   - `GET /api/signals` (recent cross-tool signals)
   - `GET /api/capabilities` (what actions are available in this tool)

### Medium-Term Enhancements

5. **Runtime Instruction Injection**
   - Extend Gurgeh/Coldwine/Pollard to inject runtime context into AGENTS.md or system prompts:
     - "You have access to specs: PRD-001, PRD-002, PRD-003"
     - "Recent tasks: Task-123 (in-progress), Task-124 (completed)"
     - "Signals: Gurgeh published spec PRD-001, Coldwine completed task for feature X"
   - Use marker blocks (`<!-- AUTARCH:CONTEXT:BEGIN -->`) for idempotent updates

6. **Bidirectional Signal Channel**
   - Extend Intermute to support orchestrator → agent messaging:
     - "Your spec PRD-001 was merged by Coldwine"
     - "Task reassigned from you to Agent B"
     - "User feedback on your work: 'looks good, ship it'"
   - Use WebSocket or file-based message queue

7. **Meta-Agent for Autarch (Inspired by NudgeNik)**
   - Create "Arbiter Light" — single-LLM interpreter for agent output
   - Use case: When agent doesn't emit signals or emits ambiguous output
   - Prompt: "Analyze this agent's response and classify as: working, blocked, needs_review, completed"

### Long-Term Vision

8. **Agent-Native by Default**
   - **Principle**: Every dashboard feature must have a CLI command AND an API endpoint AND agent documentation
   - **Enforcement**: CI check that verifies CLI/API/docs parity (inspired by schmux's `docs/api.md` gate)
   - **Example**: Bigend dashboard shows "Export Spec" button → must have `gurgeh export` CLI + `POST /api/specs/<id>/export` API

9. **Capability Understanding Journals (CUJ)**
   - Formalize schmux's "quick launch" pattern as **CUJ spec**:
     - Every tool publishes a JSON manifest of available actions
     - Agents discover capabilities at runtime (not hardcoded in prompts)
     - Orchestrator (Intermute) aggregates CUJs across tools
   - Example CUJ for Gurgeh:
     ```json
     {
       "tool": "gurgeh",
       "capabilities": [
         { "action": "list_specs", "description": "List all specs in project" },
         { "action": "export_spec", "args": ["spec_id"], "description": "Export spec to briefs" },
         { "action": "prioritize", "args": ["spec_id"], "description": "AI-powered feature ranking" }
       ]
     }
     ```

10. **Multi-Agent Coordination Primitives**
    - Extend Agent Mail to support:
      - **File locks** (exclusive access to spec or task)
      - **Work claims** (agent reserves a task, releases on completion)
      - **Event streams** (agents subscribe to spec updates, task completions)
    - Enables agents to coordinate without human intervention

---

## 11. Anti-Patterns Observed in schmux

### 1. Context Starvation

**Problem**: Agents don't know what resources exist or what they can do.

**Example**:
- User spawns Claude in workspace with quick launch presets `["Run Tests", "Fix Tests"]`
- Claude's instruction file (`.claude/CLAUDE.md`) says "Signal your status" but not "You have quick launch presets available"
- Claude cannot discover or reference presets in its responses

**Why It Happens**:
Static instruction provisioning (template, not runtime context).

**Autarch Risk**:
If Gurgeh agents don't know what specs exist, Coldwine agents don't know what tasks are available.

**Mitigation**:
Inject runtime context (available specs, tasks, signals) into system prompts or instruction files.

### 2. One-Way Signaling

**Problem**: Agents can broadcast status but cannot receive responses or updates.

**Example**:
- Agent outputs `--<[schmux:needs_input:Should I use OAuth or basic auth?]>--`
- Dashboard shows "Needs Authorization: Should I use OAuth or basic auth?"
- User decides "OAuth" but agent has no way to receive this decision (must attach to tmux session manually)

**Why It Happens**:
Signaling protocol is stdout-based (no return channel).

**Autarch Risk**:
If agents can't receive feedback or orchestration commands, they can't adapt autonomously.

**Mitigation**:
Add bidirectional channel (WebSocket, file-based queue, or Intermute messaging).

### 3. Dashboard-Only Workflows

**Problem**: Features exist in UI but not as agent-accessible tools.

**Example**:
- User can click "Sync from main" on dashboard to rebase workspace
- Agent working via CLI cannot trigger this operation (no `schmux sync-from-main` command)
- Agent cannot discover this capability (not documented in instruction file or API)

**Why It Happens**:
schmux philosophy positions humans as coordinators (dashboard is for orchestration, CLI is for execution).

**Autarch Risk**:
If Bigend dashboard has features that Gurgeh/Coldwine/Pollard agents cannot trigger, agents become second-class citizens.

**Mitigation**:
Enforce CLI/API/dashboard parity for all non-human-judgment features (export, prioritize, status checks).

### 4. Hidden Configuration

**Problem**: Agents must read raw config files (no schema, no validation, no documentation).

**Example**:
- User defines quick launch preset in `~/.schmux/config.json`
- Agent wants to know "what presets exist?" — must parse JSON manually
- Config format could change (breaking agent integration) with no API contract

**Why It Happens**:
schmux doesn't expose config as agent-readable data (no `GET /api/config` for non-authenticated users, no discovery API).

**Autarch Risk**:
If `.gurgeh/specs/`, `.coldwine/tasks/` are opaque to agents, they cannot adapt to project structure.

**Mitigation**:
Expose config/state as agent-readable API (`GET /api/specs`, `GET /api/tasks`, `GET /api/capabilities`).

---

## 12. Scoring

### Agent-Native Criteria (schmux)

| Criterion | Score | Evidence |
|-----------|-------|----------|
| **Action Parity** | 7/10 | Core operations (spawn, list, attach, dispose) have full CLI/API parity. Git sync, diff viewing, PR workflows are dashboard-only. |
| **Context Parity** | 3/10 | Agents get environment variables (SCHMUX_*) and static instruction files but no runtime context (quick launch, available targets, workspace state). |
| **Shared Workspace** | 9/10 | Git directories are transparent, no sync layer, tool-agnostic. Minor deduction for no concurrent access coordination. |
| **Primitive Tools** | 9/10 | Spawn, dispose, refresh-overlay are data-driven primitives. Quick launch encodes workflows but is stored as data. |
| **Dynamic Context** | 2/10 | Instruction files are static templates. No runtime injection of available resources, capabilities, or workspace state. |
| **Discoverability** | 4/10 | Agents can detect schmux via `SCHMUX_ENABLED=1` but cannot discover quick launch, user targets, or workspace configs programmatically. |
| **Bidirectional Signaling** | 3/10 | Agents signal status (one-way) but cannot receive orchestration commands or feedback. |
| **Documentation for Agents** | 8/10 | Instruction files auto-provisioned with signaling syntax, examples, state definitions. Missing: capability discovery, runtime context. |

**Overall**: **6.1/10** — schmux enables agents to execute effectively but stops short of agent-native autonomy. This is **intentional** per philosophy (humans coordinate agents), not an oversight.

### Autarch Comparison (Estimated)

| Criterion | schmux | Autarch (Current) | Autarch (Target) |
|-----------|--------|-------------------|------------------|
| **Action Parity** | 7/10 | 5/10 (Bigend dashboard has features without CLI/API) | 9/10 (enforce parity) |
| **Context Parity** | 3/10 | 6/10 (Intermute injects signals, but not specs/tasks) | 9/10 (inject specs, tasks, capabilities) |
| **Shared Workspace** | 9/10 | 8/10 (git repos, but no workspace config files yet) | 9/10 (add `.autarch/config.json`) |
| **Primitive Tools** | 9/10 | 7/10 (some tools encode workflows, e.g., `gurgeh onboard`) | 9/10 (refactor to data-driven) |
| **Dynamic Context** | 2/10 | 5/10 (Intermute signals, but not full state injection) | 9/10 (runtime context in system prompts) |
| **Discoverability** | 4/10 | 3/10 (no agent API for specs, tasks, capabilities) | 9/10 (CUJ pattern, GET endpoints) |
| **Bidirectional Signaling** | 3/10 | 4/10 (Intermute has signals, but no agent → orchestrator replies) | 8/10 (WebSocket or queue for replies) |
| **Documentation for Agents** | 8/10 | 6/10 (AGENTS.md exists, but not auto-updated) | 9/10 (marker-based updates + runtime context) |
| **Overall** | 6.1/10 | 5.5/10 | 9.0/10 |

---

## 13. Conclusion

### What schmux Does Right

1. **Full CLI/API parity for core operations** — spawn, list, attach, dispose all work across interfaces
2. **Environment variable injection** — agents have full self-awareness (SCHMUX_ENABLED, SESSION_ID, WORKSPACE_ID)
3. **Auto-provisioned instruction files** — agents automatically learn signaling protocol
4. **Git directories as primitives** — transparent, tool-agnostic, no abstraction layer
5. **NudgeNik meta-agent** — LLM-interpreting-LLM for ambiguous output (fallback when signals fail)
6. **Workspace overlays** — auto-copy `.env`, configs with `.gitignore` safety check

### What schmux Deliberately Omits

1. **Agent discovery APIs** — no way to list quick launch, user targets, or workspace configs programmatically
2. **Runtime context injection** — instruction files are static templates, not dynamic
3. **Bidirectional signaling** — agents broadcast status but cannot receive orchestration commands
4. **Concurrent access coordination** — multiple agents in same workspace with no locking or conflict resolution
5. **Agent-triggered orchestration** — agents cannot spawn other agents, modify config, or discover capabilities

**Why?** schmux's philosophy: "The human is always the coordinator" (not agent-to-agent autonomy).

### What Autarch Should Learn

1. **Adopt** — environment variables, marker-based provisioning, bracket signals, workspace overlays, git primitives
2. **Improve** — runtime context injection, capability manifests, bidirectional signaling, agent discovery APIs
3. **Enforce** — CLI/API/dashboard parity for all agent-facing features (schmux has this for spawn/dispose but not git sync or diff viewing)

### Final Verdict

**schmux is agent-accessible but not agent-native.**

It provides the tools for agents to work effectively under human coordination but deliberately stops short of agent autonomy. For an orchestrator targeting human-in-the-loop workflows, this is correct. For Autarch (tools for building agents), the gaps are opportunities:

- **Context starvation** → inject runtime state (specs, tasks, capabilities)
- **One-way signals** → add orchestrator → agent messaging
- **Hidden workflows** → expose quick launch / presets via API
- **Dashboard-only features** → enforce CLI/API parity

schmux is a **strong reference architecture** for multi-agent orchestration. Autarch should adopt its strengths (env vars, signals, git primitives) and extend where schmux's philosophy ends (agent autonomy, discovery, bidirectionality).

---

## 14. Next Steps for Autarch Team

1. **Review this document** with focus on sections 9-11 (recommendations, anti-patterns, scoring)
2. **Prioritize quick wins** (environment variables, bracket signals, workspace config files)
3. **Design Capability Understanding Journals** (CUJ spec for tool discovery)
4. **Enforce parity** (CI check for CLI/API/dashboard equivalence on agent-facing features)
5. **Prototype bidirectional signaling** (WebSocket or file-based queue for orchestrator → agent messages)
6. **Extend Intermute** to aggregate capabilities across tools (Gurgeh specs, Coldwine tasks, Pollard insights)

---

## Appendix: Key Files Reviewed

| File | Purpose | Lines | Key Insights |
|------|---------|-------|--------------|
| `docs/api.md` | HTTP API contract | 972 | Full spawn/dispose/list endpoints, no discovery APIs for quick launch or user targets |
| `docs/PHILOSOPHY.md` | Product philosophy | 200 | "Human is always the coordinator" — explains dashboard-only features |
| `docs/targets.md` | Run target system | 268 | Promptable vs command targets, models, quick launch |
| `docs/cli.md` | CLI reference | 442 | Full spawn/list/attach/dispose parity with API |
| `docs/workspaces.md` | Workspace architecture | 300 | Git directories, overlays, .gitignore checks |
| `internal/signal/signal.go` | Signaling protocol | 145 | Bracket marker regex, ANSI stripping, state validation |
| `internal/provision/provision.go` | Instruction provisioning | 244 | Marker-based updates, static template content |
| `internal/nudgenik/nudgenik.go` | Meta-agent | 187 | LLM-interpreting-LLM, state classification prompt |
| `internal/session/manager.go` | Session spawning | ~800 | Environment variable injection, tmux integration |
| `internal/dashboard/handlers.go` | API implementation | 3819 | SpawnRequest, QuickLaunchName, workspace operations |
| `cmd/schmux/spawn.go` | CLI spawn command | 235 | Target validation, workspace resolution, JSON output |

Total lines reviewed: ~7000+ (excluding UI frontend)

---

**Document Status**: Complete
**Review Duration**: ~2 hours
**Confidence**: High (comprehensive API, docs, and implementation analysis)
**Recommended for**: Autarch architecture team, Intermute design, multi-tool coordination
