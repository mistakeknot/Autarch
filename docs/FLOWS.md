# Autarch: Project & Module Interaction Flows

> Comprehensive guide to how Gurgeh, Coldwine, Pollard, and Bigend interact — from new-project onboarding through ongoing execution, research, and mission control.

---

## 1. Architecture Overview

```
                          +---------------------------+
                          |         BIGEND             |
                          |    Mission Control         |
                          |  (Web + TUI dashboard)     |
                          +-----+----------+----------+
                                |          |
                     WebSocket  |          |  discovery.Scanner
                     events     |          |  (project enumeration)
                                |          |
     +----------+----------+---+----------+---+----------+----------+
     |                     EVENT SPINE                               |
     |              (~/.autarch/events.db)                           |
     |  SQLite-backed durable log: Initiative/Epic/Story/Task/Run    |
     +---+-------------------+-------------------+------------------+
         |                   |                   |
         v                   v                   v
   +-----------+      +------------+      +------------+
   |  GURGEH   |      |  COLDWINE  |      |  POLLARD   |
   |  PRD Gen  |      |  Task Orch |      |  Research  |
   |  Arbiter  |      |  Epics     |      |  Hunters   |
   +-----------+      +------------+      +------------+
         |                   |                   |
         +-------------------+-------------------+
                             |
                     +-------+-------+
                     |   INTERMUTE   |
                     | REST+WS+Embed |
                     | Spec, Insight |
                     | CUJ, Session  |
                     | Messaging,    |
                     | Reservations, |
                     | Heartbeats    |
                     +---------------+
```

**Communication layers:**

| Layer | Transport | Purpose | Backing |
|-------|-----------|---------|---------|
| **Event Spine** | `pkg/events` | Append-only audit log (no subscribers) | SQLite (`~/.autarch/events.db`) |
| **Intermute** | `pkg/intermute` | Reactive coordination: entity CRUD, agent messaging/threading, file reservations, heartbeats, cursor-based event sourcing, WebSocket broadcast | REST + WebSocket + embedded |
| **Contract Types** | `pkg/contract` | Shared entity definitions | Go types (compile-time) |

> **Note:** The Event Spine is a **passive write-only log** — events are recorded but not subscribed to. Intermute is the **reactive** system with cursor-based event sourcing and WebSocket broadcast (`internal/ws/hub.go`). A future bridge will forward Event Spine writes to Intermute for unified reactivity.

---

## 2. New Project Flow

A new project moves through Gurgeh (spec creation) → Coldwine (task generation) → agents (execution), with Pollard providing research at multiple stages and Bigend observing throughout.

```mermaid
flowchart TD
    A[User runs autarch or gurgeh] --> B[Kickoff View]
    B -->|project name + description| D{Arbiter Spec Sprint}
    B -->|optional: scan repo| B1[Codebase scan → pre-fill vision]
    B1 --> D
    D -->|Phase 1| D0[Vision draft]
    D0 -->|accept/revise| D1[Problem draft]
    D1 -->|accept/revise| D2[Users draft]
    D2 -->|accept/revise| D3[Features & Goals draft]
    D3 -->|triggers| R[Pollard Quick Scan]
    R -->|GitHub + HN findings| D3
    D3 -->|accept/revise| D3b[Requirements draft]
    D3b -->|accept/revise| D4[Scope & Assumptions draft]
    D4 -->|accept/revise| D5[CUJs draft]
    D5 -->|accept/revise| D6[Acceptance Criteria draft]
    D6 --> DH{Handoff Options}
    DH -->|Export Spec| E[Spec Summary View]
    DH -->|Deep Research| PR[Pollard Deep Scan]
    DH -->|Generate Tasks| TC[Coldwine Task Gen]
    E -->|review complete spec| F[Epic Review View]
    F -->|review/edit epics| G[Task Review View]
    G -->|review/edit tasks| H[Onboarding Complete → Dashboard]
```

### Step-by-step

1. **Kickoff** (`internal/tui/unified_app.go` — `ModeOnboarding`)
   - User provides project name, description, and path.
   - `UnifiedApp` creates an `Initiative` via `pkg/contract`.
   - Emits `EventInitiativeCreated` on the event spine.

2. **Arbiter Spec Sprint** (`internal/gurgeh/arbiter/orchestrator.go`)
   - Eight-phase propose-first flow: Vision → Problem → Users → Features/Goals → Requirements → Scope/Assumptions → CUJs → Acceptance Criteria.
   - Replaces the legacy interview state machine.
   - Each phase produces a `SectionDraft` with 2–3 alternative phrasings (`Options`).
   - User can `AcceptDraft()` or `ReviseDraft()` with tracked edit history.
   - **Consistency check** (`arbiter/consistency/`) validates cross-section coherence after each advance. 🔧 Currently covers user-feature alignment (1 of 4 planned conflict types: `GoalFeature`, `ScopeCreep`, `Assumption`).
   - **Confidence scoring** (`arbiter/confidence/`) rates completeness (20%), consistency (25%), specificity (20%), research (20%), assumptions (15%).
   - At the Features/Goals phase, a **quick scan** fires Pollard's Ranger adapter to fetch GitHub + HN results. 🔧 Default is `stubScanner{}` (placeholder); real scanner must be injected via `SetScanner()`.

4. **Spec Summary** — User reviews the complete `Spec` (`internal/gurgeh/specs/schema.go`).

5. **Epic Generation** — Spec → `[]EpicProposal` with stories, acceptance criteria, risks, and estimates (`internal/coldwine/epics/types.go`).

6. **Task Generation** (`internal/coldwine/tasks/generate.go`)
   - `Generator.GenerateFromEpics()` walks each epic → each story → produces `TaskProposal` entries.
   - Task types: `implementation`, `test`, `documentation`, `review`, `setup`, `research`.
   - `ResolveCrossEpicDependencies()` links tasks across epic boundaries.
   - `BuildDependencyGraph()` produces a DAG for execution ordering.

7. **Transition to Dashboard** — `UnifiedApp` switches from `ModeOnboarding` to `ModeDashboard`. Bigend begins aggregation.

---

## 3. Ongoing Project Flow

Once a project has a spec, epics, and tasks, Coldwine manages execution while Bigend monitors.

```mermaid
flowchart LR
    A[Task: todo] -->|assign agent| B[Task: in_progress]
    B -->|spawn worktree| C[Run: working]
    C -->|agent completes| D[Run: done]
    D -->|record outcome| E[Outcome]
    E -->|all tasks done?| F{Epic complete?}
    F -->|yes| G[Epic: done]
    F -->|no| A
    C -->|agent blocked| H[Run: blocked]
    H -->|user unblocks| C
```

### Task Assignment

1. **Ready tasks** — `tasks.GetReadyTasks()` returns tasks with all dependencies met.
2. **Agent resolution** — `pkg/agenttargets/resolver.go` resolves agent name to command:
   - Resolution order: **project registry** → **global registry** → **auto-detected** (claude, codex, gemini).
   - Returns `ResolvedTarget` with command, args, env, and source context.
3. **Worktree creation** — Each task gets an isolated git worktree (via Coldwine coordination).
4. **Session management** — Bigend's tmux integration tracks active sessions.

### Run Lifecycle

| State | Meaning | Detected By | Status |
|-------|---------|-------------|--------|
| `working` | Agent actively producing output | Pane activity patterns | 🔧 Observed via tmux |
| `waiting` | Agent waiting for user input | Prompt detection | 🔧 Observed via tmux |
| `blocked` | Agent hit an obstacle | Repetition / error patterns | 🔧 Observed via tmux |
| `stalled` | No activity for extended period | Activity timeout | 🔧 Observed via tmux |
| `done` | Task completed | Exit detection | 🔧 Observed via tmux |
| `error` | Agent crashed or failed | Error pattern matching | 🔧 Observed via tmux |

State detection uses NudgeNik-style heuristics in `internal/bigend/statedetect/` — capturing tmux pane content and matching against known patterns with confidence scores.

> **Note:** These are **observed states** (Bigend scrapes tmux panes), not **managed states** (agents don't self-report). Intermute provides `POST /api/agents/{id}/heartbeat` for active state management, but Autarch tools don't call it yet. 📋 Planned: agents send structured status via Intermute messaging, replacing tmux scraping.

### Completion Tracking

- Each `Run` produces an `Outcome` (success/failure + artifacts + summary).
- Events propagate up: `EventTaskCompleted` → check if epic is done → `EventEpicClosed` → check if initiative is done → `EventInitiativeClosed`.
- Bigend's aggregator enriches project state with task/epic/pollard stats via `enrichWithTaskStats()`, `enrichWithGurgStats()`, `enrichWithPollardStats()`.

---

## 4. Research Flow (Pollard)

Pollard provides continuous research intelligence across multiple domains.

```mermaid
flowchart TD
    A[pollard scan] --> B[Hunter Registry]
    B --> C1[GitHub Scout]
    B --> C2[HackerNews]
    B --> C3[arXiv]
    B --> C4[OpenAlex]
    B --> C5[PubMed]
    B --> C6[Agent Hunter]
    B --> C7[...10+ more]
    C1 & C2 & C3 & C4 & C5 & C6 & C7 --> D[4-Stage Pipeline]
    D -->|1| E[Fetch: gather raw sources]
    E -->|2| F[Synthesize: extract insights]
    F -->|3| G[Score: relevance + quality]
    G -->|4| H[Rank: prioritize findings]
    H --> I[Sources + Insights stored in .pollard/]
    I -->|link| J[InsightLink → Initiative/Feature]
    I -->|report| K[pollard report: landscape/competitive]
```

### Hunter Interface (`internal/pollard/hunters/hunter.go`)

```go
type Hunter interface {
    Name() string
    Hunt(ctx context.Context, cfg HunterConfig) (*HuntResult, error)
}
```

All 12 hunters implement this interface. The `DefaultRegistry()` returns:

| Category | Hunters | API Keys? |
|----------|---------|-----------|
| **Tech** | GitHub Scout, HackerNews, arXiv, Competitor Tracker | GitHub optional |
| **Academic** | OpenAlex (multi-domain), PubMed | No |
| **Domain** | USDA (agriculture), Legal (CourtListener), Economics, Wiki | USDA/Court keys |
| **Agent** | Agent Hunter (primary research mechanism) | Agent command |
| **Docs** | Context7 (framework documentation) | No |

### Pipeline Options

The `PipelineOptions` struct controls agent-native synthesis:

```go
type PipelineOptions struct {
    FetchREADME      bool          // Fetch GitHub READMEs
    Synthesize       bool          // Run AI synthesis
    SynthesizeLimit  int           // Max items to synthesize
    AgentCmd         string        // Agent command for synthesis
    AgentParallelism int           // Concurrent agent invocations
    AgentTimeout     time.Duration // Per-agent timeout
}
```

### Insight Linking

Pollard insights connect to the entity hierarchy via `InsightLink` (`pkg/contract/types.go`):

```go
type InsightLink struct {
    InsightID    string
    InitiativeID string
    FeatureRef   string
    LinkedAt     time.Time
    LinkedBy     string // "pollard", "user", "gurgeh"
}
```

This allows Gurgeh specs and Coldwine tasks to reference specific research findings.

---

## 5. Mission Control Flow (Bigend)

Bigend is the read-heavy aggregation layer — it discovers projects, monitors agents, and presents a unified dashboard.

```mermaid
flowchart TD
    A[Bigend starts] --> B[discovery.Scanner]
    B -->|scan ~/projects/| C[Project list]
    A --> D[tmux session detection]
    D --> E[Session list + state]
    A --> F[Intermute WebSocket]
    F --> G[Real-time entity events]
    C & E & G --> H[Aggregator.Refresh]
    H --> I[Enrich: task stats]
    H --> J[Enrich: gurgeh stats]
    H --> K[Enrich: pollard stats]
    I & J & K --> L[Aggregated State]
    L --> M[Web UI: htmx + Tailwind]
    L --> N[TUI: Bubble Tea dashboard]
```

### Aggregator State (`internal/bigend/aggregator/aggregator.go`)

The `State` struct is the central read model:

```go
type State struct {
    Projects   []discovery.Project
    Agents     []Agent
    Sessions   []TmuxSession
    Colonies   []Colony
    MCP        map[string][]mcp.ComponentStatus
    Activities []Activity
    UpdatedAt  time.Time
}
```

**Refresh cycle:**
1. `discovery.Scanner` enumerates projects under configured paths.
2. tmux client lists sessions; `statedetect.Detector` classifies each as working/waiting/blocked/stalled.
3. Intermute WebSocket delivers real-time events (task completions, agent messages).
4. Enrichment methods pull tool-specific stats from each project's local data.
5. Colony detection scans git worktrees + convention markers; `/proc` scan is Linux-only.

**Event handlers** (`Aggregator.On()`) allow reactive updates — e.g., when a task completes, the dashboard refreshes immediately.

---

## 6. Module Interaction Map

How each `pkg/` package connects the tools:

```
pkg/contract/       pkg/events/         pkg/intermute/
  types.go            types.go            client.go
  ┌─────────┐         ┌──────────┐        ┌──────────────┐
  │Initiative│         │Event     │        │Spec CRUD     │
  │Epic      │◄───────►│EventType │        │Epic CRUD     │
  │Story     │  used   │EventFilter│       │Task CRUD     │
  │Task      │  by     │Subscription│      │CUJ CRUD      │
  │Run       │  all    │EventBus  │        │Insight CRUD  │
  │Outcome   │  tools  │          │        │Session CRUD  │
  │InsightLink│        └──────────┘        │Agent ops     │
  └─────────┘              │               │WebSocket     │
       │                   │               └──────────────┘
       │                   │                     │
       ▼                   ▼                     ▼
  pkg/agenttargets/   pkg/tui/            pkg/discovery/
  resolver.go         shelllayout.go      scanner.go
  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
  │Resolve agent │    │3-pane layout │    │Find projects │
  │by name       │    │Sidebar+Doc+  │    │in filesystem │
  │project→global│    │Chat          │    │Detect tools  │
  │→auto-detect  │    │Focus mgmt   │    │present       │
  └──────────────┘    │Tokyo Night   │    └──────────────┘
                      └──────────────┘
```

### Dependency Graph

| Package | Used By |
|---------|---------|
| `pkg/contract` | All tools (shared entity types) |
| `pkg/events` | All tools (emit events; subscribe not yet implemented) |
| `pkg/intermute` | Gurgeh (publish specs), Coldwine (task assignment), Bigend (aggregate), Pollard (insight linking) |
| `pkg/tui` | Gurgeh TUI, Coldwine TUI, Bigend TUI, Unified app |
| `pkg/agenttargets` | Coldwine (resolve agent for task), Bigend (display agent info) |
| `pkg/discovery` | Bigend (enumerate projects) |
| `pkg/autarch` | Unified TUI app (`internal/tui/unified_app.go`) — client library wrapping all tools |

---

## 7. Agent Integration

### Resolution (`pkg/agenttargets/resolver.go`)

```go
type Resolver struct {
    global, project, detected Registry
}
// Resolution: project → global → detected (claude, codex, gemini)
```

Agents are resolved by name with context (`global`, `project`, `spawn`). The resolver checks project-local config first, then global config, then auto-detected agents on the system.

### Agent Lifecycle

1. **Resolve** — `Resolver.Resolve(ctx, name)` → `ResolvedTarget` (command, args, env).
2. **Spawn** — Coldwine creates a tmux session + git worktree for the task.
3. **Monitor** — Bigend's `statedetect.Detector` reads tmux pane content to classify agent state.
4. **Communicate** — Intermute provides agent messaging: `SendMessage()`, `AgentMessages()`, inbox enrichment.
5. **Complete** — Agent finishes → `Run` state transitions to `done` → `Outcome` recorded.

### Intermute Agent Operations

```go
// Agent-aware operations from pkg/intermute/client.go
client.ListAgentsEnriched()    // All agents with inbox counts
client.AgentMessages(agentID)  // Messages for specific agent
client.AgentReservations(id)   // File reservations held by agent
client.SendMessage(msg)        // Send to agent inbox
client.Reserve(path, agentID)  // Reserve file for exclusive edit
```

### Intermute Capabilities (Full)

Beyond entity CRUD, Intermute provides infrastructure that Autarch tools can leverage:

| Capability | Endpoint/Mechanism | Used by Autarch? |
|------------|--------------------|------------------|
| **Entity CRUD** | `POST/GET /api/{specs,epics,stories,tasks,insights,cujs,sessions}` | ✅ Gurgeh, Coldwine |
| **Agent messaging** | `POST /api/messages`, inbox with threading (`thread_id`) | 📋 Planned |
| **File reservations** | `POST /api/reservations` (exclusive/shared, TTL, glob patterns) | 📋 Planned for Coldwine |
| **Heartbeats** | `POST /api/agents/{id}/heartbeat` | 📋 Planned |
| **Cursor-based events** | `GET /api/events?cursor=N` (append-only event log) | 📋 Planned bridge |
| **WebSocket broadcast** | `ws://` hub broadcasts entity + event changes | 🔧 Bigend partial |
| **Agent enrichment** | `ListAgentsEnriched()` — agents with inbox counts, reservations | 📋 Planned |

### File Reservation Model

Intermute's file reservation system enables safe multi-agent concurrent editing:

```
POST /api/reservations
{
  "path": "internal/gurgeh/**/*.go",  // glob pattern supported
  "agent_id": "agent-123",
  "mode": "exclusive",                // or "shared"
  "ttl_seconds": 300                  // auto-expires
}
```

Agents check reservations before editing. Coldwine can reserve file paths when spawning agents, enabling safe parallel work without git worktree overhead for non-conflicting tasks.

### Brief Generation

At the end of the Arbiter spec sprint, Gurgeh generates a **research brief** that Pollard can consume. This bridges spec creation and research:

- The Arbiter's Features/Goals phase triggers a quick scan that produces research queries.
- `research.Coordinator` (`internal/gurgeh/research/`) manages the handoff.
- The unified app passes the coordinator through view factories so research context flows from spec sprint → spec summary → task detail.

---

## 8. Data Model

### Entity Hierarchy

```
Initiative (project-level intent)
  ├── Epic (major feature area)
  │     ├── Story (user-facing capability)
  │     │     ├── Task (atomic work item)
  │     │     │     ├── Run (single agent execution)
  │     │     │     │     └── Outcome (result + artifacts)
  │     │     │     └── Run ...
  │     │     └── Task ...
  │     └── Story ...
  └── Epic ...

InsightLink (cross-reference: Insight ↔ Initiative/Feature)
```

### Key Types (from `pkg/contract/types.go`)

| Entity | Key Fields | Status Values |
|--------|-----------|---------------|
| `Initiative` | ID, Title, Status, Priority, SourceTool, ProjectPath | draft, open, in_progress, done, closed |
| `Epic` | ID, InitiativeID, FeatureRef, Title, Priority | draft, open, in_progress, done, closed |
| `Story` | ID, EpicID, Title, Complexity, Assignee | draft, open, in_progress, done, closed |
| `Task` | ID, StoryID, Title, WorktreeRef, SessionRef | todo, in_progress, blocked, done |
| `Run` | ID, TaskID, AgentName, AgentProgram, WorktreePath | working, waiting, blocked, done |
| `Outcome` | ID, RunID, Summary, Success, Artifacts | (terminal — no status) |
| `InsightLink` | InsightID, InitiativeID, FeatureRef, LinkedBy | (link — no status) |

### Gurgeh-Specific Types

**Spec** (`internal/gurgeh/specs/schema.go`):

| Field | Type | Purpose |
|-------|------|---------|
| `StrategicContext` | struct | Vision, market position |
| `UserStory` | struct | As-a / I-want / So-that |
| `Goals` | `[]Goal` | ID + description + metric + target |
| `NonGoals` | `[]NonGoal` | ID + description + rationale |
| `Assumptions` | `[]Assumption` | ID + description + impact-if-false + confidence + decay fields |
| `Hypotheses` | `[]Hypothesis` | Falsifiable "if X then Y" per feature, time-boxed |
| `StructuredRequirements` | `[]Requirement` | Given/When/Then format with constraints |
| `Version` | `int` | Spec version number (incremented on revision) |
| `CriticalUserJourneys` | `[]CriticalUserJourney` | Steps + success criteria + linked requirements |
| `MarketResearch` | `[]MarketResearchItem` | Competitive intelligence |

**Arbiter SprintState** (`internal/gurgeh/arbiter/types.go`):

| Field | Type | Purpose |
|-------|------|---------|
| `Phase` | enum | Current sprint phase (0–7) |
| `Sections` | `map[Phase]*SectionDraft` | Draft per phase with alternatives |
| `Conflicts` | `[]Conflict` | Cross-section inconsistencies |
| `Confidence` | `ConfidenceScore` | Weighted 5-dimension score |
| `ResearchCtx` | `*QuickScanResult` | GitHub + HN findings from Ranger |

### Coldwine-Specific Types

**Epic** (`internal/coldwine/epics/types.go`) — lighter-weight than contract epics, used during generation:

```go
type Epic struct {
    ID, Title, Summary string
    Status   Status   // todo, in_progress, review, blocked, done
    Priority Priority // p0, p1, p2, p3
    AcceptanceCriteria, Risks []string
    Estimates string
    Stories   []Story
}
```

**TaskProposal** (`internal/coldwine/tasks/generate.go`) — pre-commit task representation:

```go
type TaskProposal struct {
    ID, EpicID, StoryID, Title, Description string
    Type         TaskType // implementation, test, documentation, review, setup, research
    Priority     epics.Priority
    Dependencies []string
    Ready, Edited bool
}
```

---

## 9. TUI Architecture

### Unified Shell Layout (`pkg/tui/shelllayout.go`)

All TUIs share a 3-pane Cursor-style layout:

```
┌──────────┬────────────────────────┬──────────────┐
│          │                        │              │
│ Sidebar  │      Document          │    Chat      │
│          │      (main view)       │              │
│  Nav     │                        │  Transcript  │
│  items   │                        │  + input     │
│          │                        │              │
└──────────┴────────────────────────┴──────────────┘
     20%            50%                   30%

Focus cycles: Sidebar → Document → Chat (Tab key)
Minimum width: 100 columns
```

### Unified App Modes (`internal/tui/unified_app.go`)

| Mode | Purpose | Views |
|------|---------|-------|
| `ModeOnboarding` | New-project wizard | Kickoff → Interview → Spec Summary → Epic Review → Task Review |
| `ModeDashboard` | Ongoing work | Tab bar: Bigend, Gurgeh, Coldwine, Pollard |

The app uses **view factories** (injected functions) to create views, enabling testing and decoupling:

```go
createKickoffView      func() View
createInterviewView    func([]InterviewQuestion, *research.Coordinator) View
createSpecSummaryView  func(*SpecSummary, *research.Coordinator) View
createEpicReviewView   func([]epics.EpicProposal) View
createTaskReviewView   func([]tasks.TaskProposal) View
createTaskDetailView   func(tasks.TaskProposal, *research.Coordinator) View
createDashboardViews   func(*autarch.Client) []View
```

### Tokyo Night Theme (`pkg/tui/`)

All TUIs use a consistent color palette from the shared `pkg/tui` package, providing visual coherence across Gurgeh, Coldwine, Pollard, and Bigend interfaces.

---

## 10. File System Layout

### Per-Project Directories

```
project-root/
├── .gurgeh/
│   ├── spec.json          # Current PRD (Spec schema)
│   ├── sprint.json        # Arbiter SprintState (if active)
│   ├── specs/
│   │   ├── *.yaml         # Spec files
│   │   └── history/       # Versioned snapshots ({id}_v{N}.yaml + _rev.yaml)
│   ├── drafts/            # Section draft history
│   └── research/          # Research briefs
│
├── .coldwine/
│   ├── epics.json         # Epic proposals
│   ├── tasks.json         # Task proposals
│   ├── state.db           # SQLite: task state, runs, outcomes
│   └── worktrees/         # Git worktree metadata
│
├── .pollard/
│   ├── config.yaml        # Hunter configuration
│   ├── sources/           # Raw source data per hunter
│   ├── insights/          # Extracted insights (JSON)
│   ├── reports/           # Generated reports (Markdown)
│   └── watch/             # Watch mode state (last_scan.json)
```

### Global Directories

```
~/.autarch/
├── events.db              # Event Spine (SQLite)
├── config.yaml            # Global configuration
├── agents/                # Agent registry (global)
└── intermute/             # Intermute connection state
```

---

## 11. Event Spine ↔ Intermute Relationship

The Event Spine and Intermute are **complementary, not competing** systems:

```
  Tool (Gurgeh, Coldwine, etc.)
       │
       ├──► Event Spine (pkg/events)     ← Append-only SQLite log
       │         │                          No subscribers, no broadcast
       │         │  ┌──── planned ────┐
       │         └──► Bridge          │  ← Forward events to Intermute
       │              └───────────────┘
       │
       └──► Intermute (pkg/intermute)   ← Reactive coordination
                  │                        Cursor-based event sourcing
                  ├──► WebSocket hub       Real-time broadcast
                  ├──► Agent messaging     Threaded inboxes
                  └──► File reservations   Exclusive/shared locks
```

**Current state**: Tools write to both systems independently. Event Spine is passive (no `Subscribe()`). Intermute provides all reactive features.

**Planned bridge**: `pkg/events/store.go` will forward `Emit()` calls to Intermute's message API, unifying the event flow without duplicating infrastructure.

---

## 12. Signal System

Autarch uses a distributed signal system to detect when specs go stale. Each tool emits its own signal types; Bigend aggregates them.

```mermaid
flowchart TD
    P[Pollard] -->|competitor_shipped| S[pkg/signals]
    P -->|research_invalidation| S
    G[Gurgeh] -->|assumption_decayed| S
    G -->|hypothesis_stale| S
    G -->|spec_health_low| S
    C[Coldwine] -->|execution_drift| S
    S --> B[Bigend Signal Panel]
    S --> E[Event Spine: signal_raised]
```

### Signal Types

| Signal | Source | Trigger | Severity |
|--------|--------|---------|----------|
| `competitor_shipped` | Pollard | Watch mode detects new competitor release | warning |
| `research_invalidation` | Pollard | New findings contradict spec assumption | critical |
| `assumption_decayed` | Gurgeh | Assumption age exceeds DecayDays without validation | warning |
| `hypothesis_stale` | Gurgeh | Hypothesis past timebox, still untested | warning |
| `spec_health_low` | Gurgeh | Missing goals/requirements or majority low-confidence assumptions | critical |
| `execution_drift` | Coldwine | Task duration >3x estimate or >2 agent failures on same story | warning/critical |

### Emitter Files

| Tool | File |
|------|------|
| Pollard | `internal/pollard/signals/emitter.go` |
| Gurgeh | `internal/gurgeh/signals/emitter.go` |
| Coldwine | `internal/coldwine/signals/emitter.go` |

Signals are checked **on spec load** (no background process). Bigend aggregates all signals in `internal/bigend/tui/signals.go`.

Signals can also be streamed to local subscribers via the standalone WebSocket server:

```bash
go run ./cmd/signals serve --addr 127.0.0.1:8092
```

Publish from tools via `POST /api/signals` (local-only). Override the server URL with `AUTARCH_SIGNALS_URL`.

---

## 13. Spec Evolution & Versioning

Every spec mutation creates a `SpecRevision` stored as a file snapshot:

```
.gurgeh/specs/history/{spec_id}_v{N}.yaml       # full snapshot
.gurgeh/specs/history/{spec_id}_v{N}_rev.yaml    # revision metadata (author, trigger, changes)
.gurgeh/specs/{id}.yaml                          # current version (unchanged)
```

### Key Types (`internal/gurgeh/specs/evolution.go`)

- **SpecRevision**: version, author ("user"/"arbiter"/"pollard"), trigger ("manual"/"signal:competitive"/"agent_recommendation"), changes
- **Change**: field, before, after, reason, insight_ref

### Assumption Confidence Decay

Assumptions have `ValidatedAt`, `DecayDays` (default 30), and `LinkedInsight` fields. Confidence drops one level (high→medium→low) when age exceeds DecayDays without validation. Checked on spec load.

### CLI Commands

```bash
gurgeh history <spec-id>          # Show changelog
gurgeh diff <spec-id> v1 v2       # Structured diff between versions
```

---

## 14. Phase-Specific Deep Research

The Arbiter sprint now triggers Pollard research at each phase transition, not just at Features/Goals:

| Arbiter Phase | Pollard Hunters | Depth | Output |
|--------------|----------------|-------|--------|
| Vision | github-scout, hackernews | Quick (~30s) | Market landscape |
| Problem | arxiv-scout, openalex | Balanced (~2min) | Academic validation |
| Features/Goals | competitor-tracker, github-scout | Deep (~5min) | Prior art, gaps |
| Requirements | github-scout | Balanced (~2min) | Feasibility check |

Config: `internal/gurgeh/arbiter/research_phases.go` (`DefaultResearchPlan`)
API: `internal/pollard/api/targeted.go` (`RunTargetedScan`)

---

## 15. Competitor Watch Mode

Pollard can run continuous monitoring:

```bash
pollard watch            # Continuous loop (interval from config)
pollard watch --once     # Single cycle (cron-friendly)
```

Config in `.pollard/config.yaml`:
```yaml
watch:
  enabled: true
  interval: 24h
  hunters: [competitor-tracker, hackernews-trendwatcher]
  notify_on: [competitor_shipped, new_entrant]
```

State stored in `.pollard/watch/last_scan.json`. Each cycle diffs against previous findings and emits signals.

---

## 16. Agent-Powered Feature Ranking

```bash
gurgeh prioritize <spec-id>     # Rank features by what to build next
```

Synthesizes signals, research findings, and execution state into ranked recommendations:

```
Input: Spec + Signals + Research → Agent Prompt → JSON → RankedItems
```

Each `RankedItem` has: feature_id, title, rank, reasoning (2-3 sentences), confidence.

Implementation: `internal/gurgeh/prioritize/` (ranker.go, prompt.go)

---

## 17. Coordination Infrastructure (API Surfaces)

All coordination APIs are **local-only by default** (loopback binds enforced).

### Pollard Research API

```bash
go run ./cmd/pollard serve --addr 127.0.0.1:8090
```

**Endpoints:**
- `GET /health`
- `POST /api/scan`
- `POST /api/scan/targeted`
- `POST /api/research`
- `GET /api/jobs/{id}`
- `GET /api/jobs/{id}/result`
- `GET /api/insights`
- `GET /api/hunters`

### Gurgeh Spec API (read-only)

```bash
go run ./cmd/gurgeh serve --addr 127.0.0.1:8091
```

**Endpoints:**
- `GET /health`
- `GET /api/specs`
- `GET /api/specs/{id}`
- `GET /api/specs/{id}/requirements`
- `GET /api/specs/{id}/cujs`
- `GET /api/specs/{id}/hypotheses`
- `GET /api/specs/{id}/history`

List params: `offset`, `limit`, `include_archived` (legacy `cursor` supported as offset alias).

### Signals WebSocket Server

```bash
go run ./cmd/signals serve --addr 127.0.0.1:8092
```

**Endpoints:**
- `GET /ws` (optional filter: `?types=competitor_shipped,assumption_decayed`)
- `POST /api/signals` (publish a signal; local-only)

---

## Cross-References

- [AGENTS.md](../AGENTS.md) — Development setup and conventions
- [docs/ARCHITECTURE.md](ARCHITECTURE.md) — System architecture overview
- [docs/INTEGRATION.md](INTEGRATION.md) — Intermute integration details
- [docs/WORKFLOWS.md](WORKFLOWS.md) — End-user task guides
- [docs/bigend/AGENTS.md](bigend/AGENTS.md) — Bigend development guide
- [docs/gurgeh/AGENTS.md](gurgeh/AGENTS.md) — Gurgeh development guide
- [docs/coldwine/AGENTS.md](coldwine/AGENTS.md) — Coldwine development guide
- [docs/pollard/AGENTS.md](pollard/AGENTS.md) — Pollard development guide
