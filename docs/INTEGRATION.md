# Cross-Tool Integration Guide

> How Autarch tools communicate and share data

This guide covers data flow between Autarch tools and integration with Intermute for cross-tool coordination.

**Related Documentation:**
- [COMPOUND_INTEGRATION.md](COMPOUND_INTEGRATION.md) - Compound Engineering patterns (multi-agent review, knowledge capture, Claude Code plugin)

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                           BIGEND                                     │
│                      (Mission Control)                               │
│         Observes all tools - READ ONLY aggregation                  │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│    GURGEH     │      │   COLDWINE    │      │   POLLARD     │
│   (PRDs)      │─────▶│   (Tasks)     │◀─────│  (Research)   │
│               │      │               │      │               │
│ .gurgeh/specs │      │.coldwine/specs│      │ .pollard/     │
└───────┬───────┘      └───────┬───────┘      └───────┬───────┘
        │                      │                      │
        │                      │                      │
        └──────────────────────┼──────────────────────┘
                               │
                        ┌──────┴──────┐
                        │  INTERMUTE  │
                        │(Coordination)│
                        │             │
                        │ - Messaging │
                        │ - Events    │
                        │ - Reservations│
                        └─────────────┘
```

---

## Tool Communication Patterns

### File-Based Integration (Always Works)

Tools read each other's data files directly:

| From | To | What | Location |
|------|-----|------|----------|
| Gurgeh | Coldwine | PRD specs for task generation | `.gurgeh/specs/*.yaml` |
| Pollard | Gurgeh | Research context for PRDs | `.pollard/insights/` |
| Pollard | Coldwine | Research brief for tasks | `.pollard/reports/` |
| All | Bigend | Status and stats | All `.*/` directories |

### Intermute Integration (Optional, Enhanced)

When Intermute is available, tools get:
- Real-time event notifications
- Cross-agent messaging
- File reservation coordination
- Activity streams

---

## Data Flow: PRD → Task → Research

### 1. PRD Creation (Gurgeh)

```
User Input → Gurgeh TUI → .gurgeh/specs/PRD-001.yaml
                              │
                              ├─► Git auto-commit
                              │
                              └─► (Optional) Intermute Spec sync
```

### 2. Task Generation (Coldwine)

```
.gurgeh/specs/PRD-001.yaml
         │
         ▼
    Coldwine Init
         │
         ├─► Scan codebase
         │
         ├─► Generate epics/stories
         │
         └─► .coldwine/specs/epic-001.yaml
                  │
                  └─► (Optional) Intermute Task broadcast
```

### 3. Research Enrichment (Pollard)

```
PRD Vision + Problem
         │
         ▼
    Pollard Scan
         │
         ├─► GitHub implementations
         ├─► HackerNews trends
         ├─► Academic papers
         │
         └─► .pollard/insights/
                  │
                  └─► (Optional) Intermute Insight publish
```

---

## Gurgeh → Coldwine Integration

### How It Works

Coldwine reads `.gurgeh/specs/` to understand product context when generating tasks.

```go
// Coldwine reads PRDs during init
prdPath := filepath.Join(projectPath, ".gurgeh", "specs")
prds, _ := gurgSpecs.LoadSummaries(prdPath)

// PRD context informs epic/story generation
for _, prd := range prds {
    epic := generateEpicFromPRD(prd)
    // ...
}
```

### Data Mapping

| Gurgeh Field | Coldwine Usage |
|--------------|----------------|
| `features[]` | Epic generation |
| `requirements[]` | Story breakdown |
| `files_to_modify[]` | Task file hints |
| `critical_user_journeys[]` | Acceptance criteria |

### Manual Linking

Tasks can reference PRD IDs:

```yaml
# .coldwine/specs/task-001.yaml
id: task-001
prd_ref: PRD-001  # Links to Gurgeh PRD
title: "Implement login form"
```

---

## Gurgeh → Pollard Integration

### Triggering Research from PRD

```go
import "github.com/mistakeknot/autarch/internal/pollard/api"

scanner, _ := api.NewScanner(projectPath)

// Research based on PRD content
result, _ := scanner.ResearchForPRD(ctx,
    prd.Vision,
    prd.Problem,
    prd.Requirements,
)

// Or research user personas
result, _ := scanner.ResearchUserPersonas(ctx,
    personas,   // ["developer", "PM"]
    painpoints, // ["slow builds", "unclear requirements"]
)
```

### Intelligent Hunter Selection

Pollard can analyze PRD content and suggest relevant hunters:

```go
selections := scanner.SuggestHunters(vision, problem, requirements)
// Returns: [
//   {Name: "github-scout", Score: 0.9, Reasoning: "OSS implementations"},
//   {Name: "hackernews", Score: 0.7, Reasoning: "Industry discourse"},
// ]
```

---

## Coldwine → Pollard Integration

### Research for Epic Planning

```go
import "github.com/mistakeknot/autarch/internal/pollard/api"

scanner, _ := api.NewScanner(projectPath)

// Research implementation patterns for an epic
result, _ := scanner.ResearchForEpic(ctx,
    epic.Title,       // "Authentication System"
    epic.Description, // "Implement OAuth2..."
)
```

### Getting Insights for Tasks

```go
// Get insights linked to a feature/epic
insights, _ := scanner.GetInsightsForFeature(ctx, "FEAT-001")

// Generate a research brief for agent context
brief, _ := scanner.GenerateResearchBrief(ctx, "FEAT-001")
// Returns markdown with relevant findings, recommendations
```

---

## Bigend Aggregation

Bigend reads from all tools but **never writes** to them:

```go
// internal/bigend/aggregator/aggregator.go

func (a *Aggregator) Refresh(ctx context.Context) error {
    // Scan for projects
    projects, _ := a.scanner.Scan()

    // Enrich with Coldwine task stats
    a.enrichWithTaskStats(projects)

    // Enrich with Gurgeh PRD stats
    a.enrichWithGurgStats(projects)

    // Enrich with Pollard stats
    a.enrichWithPollardStats(projects)

    // Load agents from Intermute
    agents := a.loadAgents()

    // Load tmux sessions
    sessions := a.loadTmuxSessions(projects)

    // ...
}
```

### What Bigend Reads

| Tool | Data | Location |
|------|------|----------|
| Gurgeh | PRD counts, statuses | `.gurgeh/specs/*.yaml` |
| Coldwine | Task stats | `.coldwine/specs/*.yaml` |
| Pollard | Source/insight counts, reports | `.pollard/` |
| tmux | Session list, pane content | `tmux list-sessions` |
| Intermute | Agents, messages | HTTP API |

---

## Intermute Integration

### Environment Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `INTERMUTE_URL` | Server URL | `http://localhost:8080` |
| `INTERMUTE_API_KEY` | Authentication | `secret-key-123` |
| `INTERMUTE_PROJECT` | Project scope | `autarch` |

### Coldwine → Intermute (Task Events)

**Component:** `internal/coldwine/intermute/broadcaster.go`

```go
// Create broadcaster
broadcaster := intermute.NewTaskBroadcaster(sender, "autarch", "coldwine-agent")
broadcaster.WithRecipients([]string{"bigend-agent"})

// Broadcast task lifecycle events
broadcaster.BroadcastCreated(ctx, task)
broadcaster.BroadcastStatusChange(ctx, task, storage.TaskStatusInProgress)
broadcaster.BroadcastAssigned(ctx, task, "claude-agent")
broadcaster.BroadcastBlocked(ctx, task, "waiting for API review")
broadcaster.BroadcastCompleted(ctx, task)
```

**Events Published:**
- `task.created`
- `task.status_changed`
- `task.assigned`
- `task.blocked` (importance: high)
- `task.completed`

**Payload Structure:**
```json
{
  "event_type": "task.status_changed",
  "task_id": "task-001",
  "story_id": "story-001",
  "title": "Implement login form",
  "status": "in_progress",
  "previous_status": "todo",
  "assignee": "claude-agent",
  "priority": 1
}
```

### Gurgeh → Intermute (PRD Sync)

**Component:** `internal/gurgeh/intermute/sync.go`

```go
// Create syncer
syncer := intermute.NewPRDSyncer(client, "autarch")

// Sync new PRD (creates Intermute Spec)
spec, _ := syncer.SyncPRD(ctx, prd)

// Update existing PRD
spec, _ := syncer.SyncPRDWithID(ctx, prd, existingSpecID)
```

**Status Mapping:**

| Gurgeh Status | Intermute Status |
|---------------|------------------|
| `draft` | `draft` |
| `approved` | `research` |
| `in_progress` | `validated` |
| `done` | `archived` |

### Pollard → Intermute (Research Publishing)

**Component:** `internal/pollard/intermute/publisher.go`

```go
// Create publisher
pub := intermute.NewPublisher(client, "autarch")

// Optionally link to a spec
pub = pub.WithSpecID("spec-123")

// Publish findings as Intermute Insights
insight, _ := pub.PublishFinding(ctx, finding)
insights, _ := pub.PublishFindings(ctx, findings) // Batch
```

**Category Mapping:**

| Finding Tag | Insight Category |
|-------------|------------------|
| Contains "competitive" | `competitive` |
| Contains "trend" | `trends` |
| Contains "user" | `user` |
| Default | `research` |

### Bigend → Intermute (WebSocket Events)

**Component:** `internal/bigend/aggregator/aggregator.go`

```go
// Connect to Intermute WebSocket for real-time events
a.ConnectWebSocket(ctx)

// Register handler for all events
a.On("*", func(evt Event) {
    // Update UI, refresh relevant data
})

// Events trigger targeted refreshes
// spec.* events → refresh Gurgeh stats
// task.* events → refresh Coldwine stats
// insight.* events → refresh Pollard stats
// agent.* events → refresh agent list
```

---

## Graceful Degradation

All Intermute integrations work **without Intermute configured**:

```go
// Nil client = no-op operations
syncer := intermute.NewPRDSyncer(nil, "autarch")
spec, err := syncer.SyncPRD(ctx, prd)
// Returns: empty spec, nil error

broadcaster := intermute.NewTaskBroadcaster(nil, "autarch", "agent")
err := broadcaster.BroadcastCreated(ctx, task)
// Returns: nil error (silently succeeds)

pub := intermute.NewPublisher(nil, "autarch")
insight, err := pub.PublishFinding(ctx, finding)
// Returns: empty insight, nil error
```

**Benefits:**
- Tools work standalone without Intermute
- No error handling needed for missing Intermute
- Easy local development without full stack

---

## Shared Packages

### pkg/intermute

Client wrapper for Intermute HTTP API:

```go
import "github.com/mistakeknot/autarch/pkg/intermute"

client, _ := intermute.NewClient(nil) // Uses env vars

// Agents
agents, _ := client.ListAgentsEnriched(ctx)
agent, _ := client.GetAgent(ctx, "agent-name")

// Messages
messages, _ := client.AgentMessages(ctx, "agent-id", 50)

// Reservations
reservations, _ := client.ActiveReservations(ctx)
reservations, _ := client.AgentReservations(ctx, "agent-id")

// WebSocket
client.Connect(ctx)
client.On("task.*", func(evt Event) { ... })
client.Subscribe(ctx, "task.created", "task.completed")
```

### pkg/events

Local event spine (SQLite) with optional Intermute bridge:

```go
import "github.com/mistakeknot/autarch/pkg/events"

// Create event writer
writer, _ := events.NewWriter(dbPath)

// Attach Intermute bridge for forwarding
bridge := events.NewIntermuteBridge(client, "autarch", "coldwine-agent")
bridge.WithRecipients([]string{"bigend-agent"})
writer.AttachBridge(bridge)

// Events now auto-forward to Intermute after local storage
writer.Write(evt)
```

### pkg/contract

Cross-tool entity types:

```go
import "github.com/mistakeknot/autarch/pkg/contract"

// Shared types used across tools
type Initiative struct { ... }  // High-level product initiatives
type Epic struct { ... }        // Feature groupings
type Story struct { ... }       // User stories
type Task struct { ... }        // Implementation tasks
type Run struct { ... }         // Execution attempts
type Outcome struct { ... }     // Results
```

---

## Testing Integration

### Unit Tests

```bash
# Test Gurgeh → Intermute sync
go test ./internal/gurgeh/intermute -v

# Test Coldwine → Intermute broadcast
go test ./internal/coldwine/intermute -v

# Test Pollard → Intermute publish
go test ./internal/pollard/intermute -v
```

### Integration Test (Full Stack)

```bash
# 1. Start Intermute server
cd /root/projects/Intermute && go run ./cmd/server

# 2. Set environment
export INTERMUTE_URL=http://localhost:8080
export INTERMUTE_PROJECT=test

# 3. Test tools
./dev gurgeh init
./dev gurgeh  # Create a PRD
./dev coldwine init  # Should see Intermute sync
./dev bigend  # Should show connected status
```

### Mocking Intermute

For unit tests, mock the interfaces:

```go
type mockSender struct {
    messages []ic.Message
}

func (m *mockSender) SendMessage(ctx context.Context, msg ic.Message) (ic.SendResponse, error) {
    m.messages = append(m.messages, msg)
    return ic.SendResponse{MessageID: "test-123"}, nil
}

func TestBroadcaster(t *testing.T) {
    mock := &mockSender{}
    broadcaster := NewTaskBroadcaster(mock, "project", "agent")
    broadcaster.BroadcastCreated(ctx, task)

    assert.Len(t, mock.messages, 1)
    assert.Contains(t, mock.messages[0].Subject, "task.created")
}
```

---

## Troubleshooting

### Tools Not Seeing Each Other's Data

1. Check data directories exist:
   ```bash
   ls -la .gurgeh/ .coldwine/ .pollard/
   ```

2. Verify file permissions

3. Run tool init commands:
   ```bash
   ./dev gurgeh init
   ./dev coldwine init
   go run ./cmd/pollard init
   ```

### Intermute Connection Failed

1. Check environment:
   ```bash
   echo $INTERMUTE_URL
   curl $INTERMUTE_URL/health
   ```

2. Check API key (if required):
   ```bash
   echo $INTERMUTE_API_KEY
   ```

3. Check project scope:
   ```bash
   echo $INTERMUTE_PROJECT
   ```

### Events Not Appearing

1. Check WebSocket connection in Bigend logs
2. Verify event subscriptions
3. Check Intermute server logs for errors
4. Ensure sender/publisher client is not nil

### Research Not Linked to PRDs

1. Ensure Pollard has `specID` set when publishing
2. Check insight category mapping
3. Verify PRD features have linkable identifiers
