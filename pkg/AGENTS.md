# Shared Packages

Shared Go packages used across Autarch tools.

## Package Overview

| Package | Purpose |
|---------|---------|
| `agenttargets` | Run-target registry/resolver for agent commands |
| `contract` | Cross-tool entity types (Initiative, Epic, Story, Task, Run, Outcome) |
| `discovery` | Project discovery and tool detection |
| `events` | Event spine for cross-tool communication (SQLite) |
| `plan` | Plan file parsing |
| `shell` | Shell integration utilities |
| `toolpane` | Tool pane interface |
| `tui` | Shared TUI styles (Tokyo Night palette) |

---

## contract

Shared entity types forming the unified data contract between tools.

**Import:** `github.com/mistakeknot/autarch/pkg/contract`

### Entity Hierarchy

```
Initiative (high-level product/feature)
└── Epic (large body of work)
    └── Story (user story)
        └── Task (implementable unit)
            └── Run (agent execution)
                └── Outcome (result)
```

### Types

| Type | Description | Source Tool |
|------|-------------|-------------|
| `Initiative` | High-level product/feature initiative | Gurgeh (Spec) |
| `Epic` | Large body of work, links to Initiative | Coldwine |
| `Story` | User story within an epic | Coldwine |
| `Task` | Implementable unit of work | Coldwine |
| `Run` | Agent working on a task | Coldwine/Bigend |
| `Outcome` | Result of an agent run | Coldwine |
| `InsightLink` | Pollard insight → Initiative/Feature link | Pollard |

### Status Enums

```go
// Initiative/Epic/Story status
StatusDraft, StatusOpen, StatusInProgress, StatusDone, StatusClosed

// Task status
TaskStatusTodo, TaskStatusInProgress, TaskStatusBlocked, TaskStatusDone

// Run state
RunStateWorking, RunStateWaiting, RunStateBlocked, RunStateDone

// Complexity (t-shirt sizing)
ComplexityXS, ComplexityS, ComplexityM, ComplexityL, ComplexityXL
```

### Cross-Tool References

- `Epic.FeatureRef` → Links to Gurgeh spec ID
- `Task.WorktreeRef` → Git worktree path
- `Task.SessionRef` → Agent session ID
- `InsightLink` → Connects Pollard insights to features

### Usage

```go
import "github.com/mistakeknot/autarch/pkg/contract"

task := contract.Task{
    ID:         "task-001",
    StoryID:    "story-001",
    Title:      "Implement login form",
    Status:     contract.TaskStatusTodo,
    Priority:   1,
    SourceTool: contract.SourceColdwine,
    CreatedAt:  time.Now(),
    UpdatedAt:  time.Now(),
}

// Validate
if err := contract.Validate(task); err != nil {
    log.Fatal(err)
}
```

---

## events

Event spine for cross-tool communication. Events are stored in SQLite at `~/.autarch/events.db`.

**Import:** `github.com/mistakeknot/autarch/pkg/events`

### Event Types

| Category | Events |
|----------|--------|
| Initiative | `initiative_created`, `initiative_updated`, `initiative_closed` |
| Epic | `epic_created`, `epic_updated`, `epic_closed` |
| Story | `story_created`, `story_updated`, `story_closed` |
| Task | `task_created`, `task_assigned`, `task_started`, `task_blocked`, `task_completed` |
| Run | `run_started`, `run_waiting`, `run_completed`, `run_failed` |
| Outcome | `outcome_recorded` |
| Insight | `insight_linked` |

### Writing Events

```go
import "github.com/mistakeknot/autarch/pkg/events"

writer, err := events.NewWriter()
if err != nil {
    log.Fatal(err)
}
defer writer.Close()

// Emit an event
err = writer.Emit(events.EventTaskStarted, events.EntityTask, "task-001",
    events.SourceColdwine, map[string]interface{}{
        "assignee": "claude",
        "worktree": "/path/to/worktree",
    })
```

### Reading Events

```go
reader, err := events.NewReader()
if err != nil {
    log.Fatal(err)
}
defer reader.Close()

// Query with filters
filter := events.NewEventFilter().
    WithEventTypes(events.EventTaskStarted, events.EventTaskCompleted).
    WithSourceTools(events.SourceColdwine).
    WithSince(time.Now().Add(-24 * time.Hour)).
    WithLimit(50)

eventList, err := reader.Query(filter)
```

### Subscriptions (Real-time)

```go
store, err := events.NewStore()
if err != nil {
    log.Fatal(err)
}
defer store.Close()

filter := events.NewEventFilter().
    WithEntityTypes(events.EntityTask)

sub := store.Subscribe(filter)
defer sub.Close()

for event := range sub.Channel {
    fmt.Printf("Event: %s on %s\n", event.EventType, event.EntityID)
}
```

### Intermute Bridge (Cross-Tool Coordination)

The `IntermuteBridge` forwards local events to Intermute for cross-tool visibility. This enables tools like Bigend, Gurgeh, and Coldwine to broadcast their events via Intermute's coordination layer.

**Initialization:**

```go
import "github.com/mistakeknot/autarch/pkg/events"
import "github.com/mistakeknot/autarch/pkg/intermute"

// Create bridge with Intermute client
bridge := events.NewIntermuteBridge(
    client,              // MessageSender (intermute.Client or mock)
    "autarch",           // Intermute project
    "coldwine-agent",    // Agent identifier
)

// Optional: set specific recipients
bridge.WithRecipients([]string{"bigend-agent", "gurgeh-agent"})

// Attach to writer for automatic forwarding
writer.AttachBridge(bridge)
```

**Key Behaviors:**

| Behavior | Details |
|----------|---------|
| **Forwarding** | Events forwarded after local storage (non-blocking) |
| **Message Format** | Event serialized to JSON with metadata |
| **Importance** | Events with 'failed', 'blocked', 'error' marked "high"; others "normal" |
| **Error Handling** | Bridge errors logged but don't fail local emission (graceful degradation) |
| **Timeout** | 5-second timeout for bridge operations |

**Payload Structure:**

```json
{
  "event_id": 42,
  "event_type": "task_completed",
  "entity_type": "task",
  "entity_id": "task-123",
  "source_tool": "coldwine",
  "payload": { /* custom event data */ },
  "project_path": "/path/to/project",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Database Location

- Default: `~/.autarch/events.db`
- Schema auto-migrates on first connection
- WAL mode for concurrent access

---

## tui

Shared TUI styles using Tokyo Night color palette.

**Import:** `github.com/mistakeknot/autarch/pkg/tui`

### Colors

```go
ColorPrimary   = "#7aa2f7"  // Blue
ColorSecondary = "#bb9af7"  // Purple
ColorSuccess   = "#9ece6a"  // Green
ColorWarning   = "#e0af68"  // Yellow
ColorError     = "#f7768e"  // Red
ColorMuted     = "#565f89"  // Gray
```

### Components

```go
// Status indicators
tui.StatusIndicator("running")  // "● RUNNING" (green)
tui.StatusIndicator("waiting")  // "○ WAITING" (yellow)
tui.StatusIndicator("idle")     // "◌ IDLE" (gray)
tui.StatusIndicator("error")    // "✗ ERROR" (red)

// Agent badges
tui.AgentBadge("claude")  // Orange badge
tui.AgentBadge("codex")   // Teal badge

// Priority badges
tui.PriorityBadge(0)  // "P0" (red)
tui.PriorityBadge(1)  // "P1" (yellow)
```

---

## agenttargets

Registry for agent run targets (claude, codex, etc.).

**Import:** `github.com/mistakeknot/autarch/pkg/agenttargets`

### Configuration

Global: `~/.config/autarch/agents.toml`
Per-project: `.gurgeh/agents.toml`

```toml
[targets.claude]
command = "claude"
args = ["--print"]

[targets.codex]
command = "codex"
args = ["--approval-mode", "full-auto"]
```

### Usage

```go
import "github.com/mistakeknot/autarch/pkg/agenttargets"

registry := agenttargets.NewRegistry()
if err := registry.LoadGlobal(); err != nil {
    log.Fatal(err)
}
if err := registry.LoadProject(".gurgeh/agents.toml"); err != nil {
    log.Fatal(err)
}

target, ok := registry.Get("claude")
if ok {
    cmd := exec.Command(target.Command, target.Args...)
    // ...
}
```
