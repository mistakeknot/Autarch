# Unified Autarch TUI Design

## Overview

Refactor Autarch into a unified TUI with Intermute as the central state backend. All tools (Bigend, Gurgeh, Coldwine, Pollard) become views into shared domain state stored in Intermute.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Autarch TUI                               │
│  ┌─────────┬─────────┬─────────┬─────────┐                      │
│  │ Bigend  │ Gurgeh  │Coldwine │ Pollard │  ← Tab bar (1-4)     │
│  └─────────┴─────────┴─────────┴─────────┘                      │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                                                             ││
│  │                    Active View                              ││
│  │                                                             ││
│  └─────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────┐│
│  │ Command Palette (Ctrl+P)                                    ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
        │                    ▲
        │ REST mutations     │ WebSocket events
        ▼                    │
┌─────────────────────────────────────────────────────────────────┐
│                      Intermute Server                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Event Log (SQLite)                     │   │
│  │  ┌─────────┬─────────┬─────────┬─────────┬─────────────┐ │   │
│  │  │ Agents  │Messages │  Specs  │  Tasks  │  Insights   │ │   │
│  │  └─────────┴─────────┴─────────┴─────────┴─────────────┘ │   │
│  └──────────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Materialized Views (current state)           │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Domain Model

All state lives in Intermute. The domain is unified across what were separate tools:

### Core Entities

```go
// Spec (was PRD in Gurgeh)
type Spec struct {
    ID          string
    Project     string
    Title       string
    Vision      string
    Users       string
    Problem     string
    Status      SpecStatus  // draft, research, validated, archived
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// Epic (was in Coldwine)
type Epic struct {
    ID          string
    Project     string
    SpecID      string      // Links to parent Spec
    Title       string
    Status      EpicStatus  // open, in_progress, done
    Stories     []Story
}

// Story
type Story struct {
    ID                  string
    EpicID              string
    Title               string
    AcceptanceCriteria  []string
    Status              StoryStatus
}

// Task (execution unit)
type Task struct {
    ID          string
    StoryID     string
    Title       string
    Agent       string      // Assigned agent
    SessionID   string      // tmux session
    Status      TaskStatus  // pending, running, blocked, done
}

// Insight (was in Pollard)
type Insight struct {
    ID          string
    Project     string
    SpecID      string      // Optional link to Spec
    Source      string      // hunter name
    Category    string      // competitor, pattern, technology
    Title       string
    Body        string
    URL         string
    Score       float64
    CreatedAt   time.Time
}

// Session (was in Bigend)
type Session struct {
    ID          string
    Project     string
    Name        string
    Agent       string      // claude, codex, etc.
    TaskID      string      // Optional link to Task
    Status      string      // running, idle, error
    StartedAt   time.Time
}
```

### Event Types

Extend Intermute's event log with domain events:

```go
const (
    // Existing
    EventMessageCreated EventType = "message.created"
    EventAgentHeartbeat EventType = "agent.heartbeat"

    // Specs
    EventSpecCreated    EventType = "spec.created"
    EventSpecUpdated    EventType = "spec.updated"
    EventSpecArchived   EventType = "spec.archived"

    // Epics/Stories/Tasks
    EventEpicCreated    EventType = "epic.created"
    EventStoryCreated   EventType = "story.created"
    EventTaskCreated    EventType = "task.created"
    EventTaskAssigned   EventType = "task.assigned"
    EventTaskCompleted  EventType = "task.completed"

    // Insights
    EventInsightCreated EventType = "insight.created"
    EventInsightLinked  EventType = "insight.linked"

    // Sessions
    EventSessionStarted EventType = "session.started"
    EventSessionStopped EventType = "session.stopped"
)
```

## Intermute API Extensions

### New REST Endpoints

```
# Specs
POST   /api/specs                    Create spec
GET    /api/specs                    List specs (with filters)
GET    /api/specs/{id}               Get spec
PUT    /api/specs/{id}               Update spec
DELETE /api/specs/{id}               Archive spec

# Epics
POST   /api/epics                    Create epic
GET    /api/epics?spec={specId}      List epics for spec
GET    /api/epics/{id}               Get epic with stories

# Stories
POST   /api/stories                  Create story
PUT    /api/stories/{id}             Update story

# Tasks
POST   /api/tasks                    Create task
GET    /api/tasks?status=pending     List tasks
PUT    /api/tasks/{id}               Update task
POST   /api/tasks/{id}/assign        Assign to agent

# Insights
POST   /api/insights                 Create insight
GET    /api/insights?spec={specId}   List insights
POST   /api/insights/{id}/link       Link to spec

# Sessions
GET    /api/sessions                 List sessions
POST   /api/sessions                 Create session
DELETE /api/sessions/{id}            Stop session
```

### WebSocket Events

All mutations emit events on WebSocket for real-time updates:

```json
{
    "type": "spec.updated",
    "project": "autarch",
    "data": {
        "id": "spec-001",
        "status": "validated"
    },
    "cursor": 12345
}
```

## Autarch TUI Structure

### Navigation

- **Tab bar**: `[1:Bigend] [2:Gurgeh] [3:Coldwine] [4:Pollard]`
- **Command palette**: `Ctrl+P` opens fuzzy finder for all actions
- **Global keys**:
  - `1-4`: Switch tabs
  - `Ctrl+P`: Command palette
  - `?`: Help
  - `q`: Quit

### Views

Each tab is a view into the shared state:

**Bigend View** (Sessions + Overview)
```
┌─ Sessions ─────────────────────┬─ Details ──────────────────────┐
│ ▾ autarch (3 active)           │ Session: claude-main           │
│   ├─ claude-main [running]     │ Project: /root/projects/Autarch│
│   ├─ codex-research [idle]     │ Task: Implement spec API       │
│   └─ claude-tests [running]    │ Started: 2m ago                │
│ ▸ jawncloud (1 active)         │                                │
└────────────────────────────────┴────────────────────────────────┘
```

**Gurgeh View** (Specs)
```
┌─ Specs ────────────────────────┬─ Details ──────────────────────┐
│ ▾ draft (2)                    │ SPEC-001: Unified TUI          │
│   ├─ SPEC-001: Unified TUI     │ Status: draft                  │
│   └─ SPEC-002: API Gateway     │ Vision: Single entry point...  │
│ ▾ validated (1)                │ ────────────────────────────── │
│   └─ SPEC-003: Auth System     │ Epics: 3 | Insights: 5         │
└────────────────────────────────┴────────────────────────────────┘
```

**Coldwine View** (Epics/Tasks)
```
┌─ Epics ────────────────────────┬─ Stories ───────────────────────┐
│ ▾ EPIC-001: Core Domain (3/5)  │ STORY-001: Define models       │
│   ├─ STORY-001 [done]          │ Status: done                   │
│   ├─ STORY-002 [in_progress]   │ Acceptance:                    │
│   └─ STORY-003 [pending]       │   ✓ Spec model defined         │
│ ▸ EPIC-002: API Layer (0/4)    │   ✓ Epic model defined         │
└────────────────────────────────┴────────────────────────────────┘
```

**Pollard View** (Research)
```
┌─ Insights ─────────────────────┬─ Details ──────────────────────┐
│ ▾ competitor (12)              │ Warp Terminal Architecture     │
│   ├─ Warp Terminal Arch...     │ Source: github-scout           │
│   └─ Cursor IDE Plugins...     │ Score: 0.89                    │
│ ▾ pattern (8)                  │ ────────────────────────────── │
│   └─ Event Sourcing in Go      │ Related Spec: SPEC-001         │
└────────────────────────────────┴────────────────────────────────┘
```

### Command Palette

Fuzzy search over all available actions:

```
┌─ Command Palette ──────────────────────────────────────────────┐
│ > new spec                                                     │
│ ────────────────────────────────────────────────────────────── │
│   New Spec                      Create a new specification     │
│   New Epic                      Create a new epic              │
│   New Session                   Start a new agent session      │
│   Run Research                  Execute Pollard hunters        │
└────────────────────────────────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Intermute Domain Extensions
1. Add domain models to `internal/core/domain.go`
2. Add domain event types
3. Extend schema with domain tables
4. Add REST handlers for specs, epics, tasks, insights, sessions
5. Add domain events to WebSocket broadcast

### Phase 2: Autarch Client Library
1. Create `pkg/autarch/client.go` - typed client for Intermute domain API
2. Add WebSocket subscription with event callbacks
3. Add local caching for offline resilience (optional)

### Phase 3: Unified TUI Shell
1. Create `internal/tui/app.go` - main application model
2. Add tab bar component
3. Add command palette component
4. Add base view interface

### Phase 4: Port Views
1. Port Bigend view (sessions)
2. Port Gurgeh view (specs)
3. Port Coldwine view (epics/tasks)
4. Port Pollard view (insights)

### Phase 5: Cross-View Navigation
1. Add jump-to commands (spec → epics, task → session)
2. Add contextual actions in command palette
3. Add keyboard shortcuts for common flows

## Migration Path

1. **Intermute first**: Build domain API, keep existing tools working
2. **Parallel operation**: New TUI works alongside old tools during transition
3. **Data migration**: Script to import existing `.gurgeh/`, `.coldwine/`, `.pollard/` into Intermute
4. **Deprecate old tools**: Once TUI is stable, remove separate binaries

## File Structure

```
Autarch/
├── cmd/
│   └── autarch/main.go           # Single binary
├── internal/
│   └── tui/
│       ├── app.go                # Main app model
│       ├── tabs.go               # Tab bar component
│       ├── palette.go            # Command palette
│       ├── views/
│       │   ├── bigend.go         # Sessions view
│       │   ├── gurgeh.go         # Specs view
│       │   ├── coldwine.go       # Epics/tasks view
│       │   └── pollard.go        # Insights view
│       └── components/
│           ├── list.go           # Grouped list
│           ├── detail.go         # Detail pane
│           └── input.go          # Text input
├── pkg/
│   └── autarch/
│       └── client.go             # Intermute domain client
└── docs/
    └── plans/
        └── 2026-01-25-unified-autarch-tui-design.md

Intermute/
├── internal/
│   ├── core/
│   │   ├── models.go             # Existing
│   │   └── domain.go             # New domain models
│   ├── http/
│   │   ├── handlers_agents.go    # Existing
│   │   ├── handlers_messages.go  # Existing
│   │   ├── handlers_specs.go     # New
│   │   ├── handlers_epics.go     # New
│   │   ├── handlers_tasks.go     # New
│   │   ├── handlers_insights.go  # New
│   │   └── handlers_sessions.go  # New
│   └── storage/
│       └── sqlite/
│           ├── schema.sql        # Extended
│           └── domain.go         # Domain queries
```

## Open Questions

1. **Project scoping**: Should all domain entities be project-scoped like messages?
   - **Recommendation**: Yes, consistent with existing Intermute design

2. **File storage**: Where do briefs, research files, etc. live?
   - **Option A**: Keep in filesystem, Intermute stores metadata only
   - **Option B**: Store content in Intermute (blob column or external storage)
   - **Recommendation**: Option A for now, keeps Intermute lightweight

3. **Offline mode**: Should Autarch work without Intermute?
   - **Recommendation**: No - keep it simple, Intermute is always required

4. **Authentication**: How do multiple users access the same Intermute?
   - **Recommendation**: Use existing project-scoped API keys, add user identity later
