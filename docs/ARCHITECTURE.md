# Autarch Architecture

> System overview, data flow, and shared infrastructure

This document provides a technical overview of the Autarch monorepo architecture. For tool-specific details, see the AGENTS.md files in each tool's docs folder.

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              BIGEND                                          │
│                         (Mission Control)                                    │
│         Observes all tools and agents - READ ONLY aggregation               │
│                                                                              │
│   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│   │   Gurgeh     │  │   Coldwine   │  │   Pollard    │  │    tmux      │   │
│   │   Specs      │  │    Tasks     │  │   Insights   │  │  Sessions    │   │
│   └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘   │
└──────────┼─────────────────┼─────────────────┼─────────────────┼────────────┘
           │                 │                 │                 │
           │                 │                 │                 │
    ┌──────┴──────┐   ┌──────┴──────┐   ┌──────┴──────┐         │
    │  .gurgeh/   │   │ .coldwine/  │   │  .pollard/  │         │
    │   specs/    │◄──┤   specs/    │◄──┤   insights/ │         │
    │  research/  │   │   plan/     │   │   sources/  │         │
    └─────────────┘   └─────────────┘   └─────────────┘         │
           │                 │                 │                 │
           └─────────────────┴────────┬────────┴─────────────────┘
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

## Tool Responsibilities

| Tool | Scope | Reads From | Writes To | UI |
|------|-------|------------|-----------|-----|
| **Bigend** | Multi-project mission control | All `.*/` dirs, tmux, Intermute | Nothing (read-only) | Web + TUI |
| **Gurgeh** | PRD generation & validation | Codebase, `.pollard/insights/` | `.gurgeh/specs/` | TUI |
| **Coldwine** | Task orchestration | `.gurgeh/specs/`, `.pollard/` | `.coldwine/specs/` | TUI |
| **Pollard** | Research intelligence | External APIs, codebase | `.pollard/sources/`, `.pollard/insights/` | CLI |

### Data Flow: PRD → Task → Research

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           DATA FLOW                                       │
└──────────────────────────────────────────────────────────────────────────┘

User Requirements
       │
       ▼
┌─────────────┐
│   GURGEH    │ ──────────────────────────────────────────┐
│   (PRDs)    │                                           │
│             │  1. Guided interview                       │
│             │  2. Generate PRD spec                      │
│             │  3. Auto-commit to git                     │
└──────┬──────┘                                           │
       │                                                  │
       │ .gurgeh/specs/PRD-001.yaml                       │
       │                                                  │
       ▼                                                  │
┌─────────────┐                                           │
│  COLDWINE   │ ◄─────────────────────────────────────────┤
│   (Tasks)   │                                           │
│             │  1. Read PRDs                              │
│             │  2. Scan codebase                          │
│             │  3. Generate epics/stories/tasks           │
│             │  4. Manage worktrees                       │
└──────┬──────┘                                           │
       │                                                  │
       │ .coldwine/specs/epic-001.yaml                    │
       │                                                  │
       ▼                                                  │
┌─────────────┐                                           │
│  POLLARD    │ ◄─────────────────────────────────────────┘
│  (Research) │
│             │  1. Analyze PRD content
│             │  2. Run relevant hunters
│             │  3. Generate insights
│             │  4. Link to features
└──────┬──────┘
       │
       │ .pollard/insights/
       │
       ▼
┌─────────────┐
│   BIGEND    │
│ (Aggregates)│
│             │  1. Discover projects
│             │  2. Aggregate tool stats
│             │  3. Monitor agents
│             │  4. Real-time updates
└─────────────┘
```

---

## Local-Only Default

Autarch is **local-only by default**. All tools function without any remote services running. Optional local services (Intermute coordination server and the Signals server) run on `127.0.0.1` and are not required for core workflows.

The only expected outbound traffic is from **research hunters** (Pollard) when explicitly invoked.

---

## Directory Structure

```
autarch/
├── cmd/                        # Entry points
│   ├── autarch/               # Unified CLI (future)
│   ├── bigend/                # Mission control
│   ├── coldwine/              # Task orchestration
│   ├── gurgeh/                # PRD generation
│   └── pollard/               # Research CLI
│
├── internal/                   # Tool-specific code
│   ├── bigend/
│   │   ├── aggregator/        # Data aggregation + WebSocket
│   │   ├── discovery/         # Project scanner
│   │   ├── tmux/              # tmux client
│   │   ├── tui/               # Bubble Tea TUI
│   │   └── web/               # HTTP + htmx
│   │
│   ├── coldwine/
│   │   ├── agents/            # Agent coordination
│   │   ├── intermute/         # Intermute bridge
│   │   ├── planner/           # Task generation
│   │   ├── storage/           # YAML persistence
│   │   ├── tui/               # Bubble Tea TUI
│   │   └── worktree/          # Git worktree management
│   │
│   ├── gurgeh/
│   │   ├── agents/            # PRD generation agents
│   │   ├── intermute/         # Intermute bridge
│   │   ├── spec/              # Spec parsing/validation
│   │   ├── storage/           # YAML persistence
│   │   └── tui/               # Bubble Tea TUI
│   │
│   └── pollard/
│       ├── api/               # Programmatic Scanner API
│       ├── cli/               # Cobra CLI commands
│       ├── config/            # YAML configuration
│       ├── hunters/           # Research agents
│       ├── insights/          # Synthesized findings
│       ├── intermute/         # Intermute bridge
│       ├── reports/           # Markdown generation
│       ├── research/          # Research coordinator
│       ├── sources/           # Raw data management
│       └── state/             # SQLite state
│
├── pkg/                        # Shared packages
│   ├── agenttargets/          # Agent target configuration
│   ├── autarch/               # Unified client (WebSocket)
│   ├── contract/              # Cross-tool entity types
│   ├── discovery/             # Project discovery
│   ├── events/                # Event spine (SQLite)
│   ├── intermute/             # Intermute client wrapper
│   ├── plan/                  # Plan file parsing
│   ├── shell/                 # Shell context helpers
│   └── tui/                   # Shared TUI components
│
├── docs/                       # Documentation
│   ├── ARCHITECTURE.md        # This file
│   ├── INTEGRATION.md         # Cross-tool integration
│   ├── QUICK_REFERENCE.md     # Command cheat sheet
│   ├── WORKFLOWS.md           # End-user guides
│   ├── bigend/                # Bigend docs
│   ├── coldwine/              # Coldwine docs
│   ├── gurgeh/                # Gurgeh docs
│   └── pollard/               # Pollard docs
│
└── dev                         # Build/run script
```

---

## Shared Infrastructure

### pkg/contract - Cross-Tool Entity Types

Defines the unified data contract between all tools:

```go
// Core entities
type Initiative struct { ... }  // High-level initiatives (maps to Gurgeh Spec)
type Epic struct { ... }        // Feature groupings
type Story struct { ... }       // User stories
type Task struct { ... }        // Implementation tasks
type Run struct { ... }         // Agent execution attempts
type Outcome struct { ... }     // Run results

// Status enums
type Status string      // draft, open, in_progress, done, closed
type TaskStatus string  // todo, in_progress, blocked, done
type RunState string    // working, waiting, blocked, done
type Complexity string  // xs, s, m, l, xl

// Source identification
type SourceTool string  // gurgeh, coldwine, pollard, bigend
```

### pkg/events - Event Spine

Local SQLite-based event log with optional Intermute bridge:

```go
// Event types
EventInitiativeCreated, EventInitiativeUpdated, EventInitiativeClosed
EventEpicCreated, EventEpicUpdated, EventEpicClosed
EventStoryCreated, EventStoryUpdated, EventStoryClosed
EventTaskCreated, EventTaskAssigned, EventTaskStarted, EventTaskBlocked, EventTaskCompleted
EventRunStarted, EventRunWaiting, EventRunCompleted, EventRunFailed
EventOutcomeRecorded
EventInsightLinked

// Usage
writer, _ := events.NewWriter(dbPath)
bridge := events.NewIntermuteBridge(client, "project", "agent")
writer.AttachBridge(bridge)  // Events auto-forward to Intermute
writer.Write(event)
```

### pkg/tui - Shared TUI Components

Tokyo Night color theme and reusable Bubble Tea components:

```go
// Colors
ColorBg, ColorBgDark, ColorBgLight   // Backgrounds
ColorFg, ColorFgDim                   // Foregrounds
ColorPrimary, ColorSecondary          // Accents
ColorSuccess, ColorWarning, ColorError // Status
ColorClaude, ColorCodex, ColorAider   // Agent badges

// Styles
BaseStyle, ContentStyle, CardStyle, HeaderStyle, FooterStyle
StatusRunning, StatusWaiting, StatusIdle, StatusError
SelectedStyle, UnselectedStyle, TabStyle, ActiveTabStyle

// Components
ChatPanel    // Agent chat display
DocPanel     // Document viewer
SplitLayout  // Two-pane layout
Composer     // Input composition
```

### pkg/intermute - Intermute Client Wrapper

HTTP client for Intermute coordination server:

```go
client, _ := intermute.NewClient(nil)  // Uses env vars

// Agent operations
agents, _ := client.ListAgentsEnriched(ctx)
agent, _ := client.GetAgent(ctx, "agent-name")

// Message operations
messages, _ := client.AgentMessages(ctx, "agent-id", 50)

// Reservation operations
reservations, _ := client.ActiveReservations(ctx)

// WebSocket for real-time events
client.Connect(ctx)
client.On("task.*", handler)
client.Subscribe(ctx, "task.created", "task.completed")
```

### pkg/discovery - Project Discovery

Scans directories to find Autarch-enabled projects:

```go
scanner := discovery.NewScanner([]string{"~/projects"})
projects, _ := scanner.Scan()

// Each project has:
type Project struct {
    Path       string
    Name       string
    HasGurgeh  bool      // .gurgeh/ exists
    HasColdwine bool     // .coldwine/ exists
    HasPollard  bool     // .pollard/ exists
    GurgStats   *GurgStats
    ColdStats   *ColdStats
    PollStats   *PollStats
}
```

---

## Communication Patterns

### 1. File-Based (Always Works)

Tools read each other's data directories directly:

| Source | Target | Data | Location |
|--------|--------|------|----------|
| Gurgeh | Coldwine | PRD specs | `.gurgeh/specs/*.yaml` |
| Pollard | Gurgeh | Research insights | `.pollard/insights/` |
| Pollard | Coldwine | Research briefs | `.pollard/reports/` |
| All | Bigend | Status and stats | All `.*/` directories |

### 2. Event-Based (Optional)

When Intermute is configured, tools emit events:

```
Gurgeh   ──► spec.created, spec.updated
Coldwine ──► task.created, task.status_changed, task.assigned
Pollard  ──► insight.created, insight.linked
```

Bigend subscribes to all events via WebSocket for real-time updates.

### 3. Graceful Degradation

All Intermute integrations work without Intermute configured:

```go
// Nil client = no-op operations
syncer := intermute.NewPRDSyncer(nil, "project")
spec, err := syncer.SyncPRD(ctx, prd)
// Returns: empty spec, nil error (silent success)
```

---

## Agent State Detection

Bigend detects AI agent state from tmux sessions:

```
┌─────────────────────────────────────────────────────────────────┐
│                    STATE DETECTION FLOW                          │
└─────────────────────────────────────────────────────────────────┘

tmux pane content
       │
       ├──► Contains "waiting for input"  ────► WAITING (yellow)
       │
       ├──► Contains "error"/"failed"     ────► BLOCKED (red)
       │
       ├──► Active output (< 30s ago)     ────► WORKING (green)
       │
       ├──► No output for 5+ minutes      ────► STALLED (gray)
       │
       └──► Contains "completed"/"done"   ────► DONE (blue)
```

Agent detection by session/pane name patterns:
- `claude`, `anthropic` → Claude badge
- `codex`, `openai` → Codex badge
- `aider` → Aider badge
- `cursor` → Cursor badge

---

## Data Persistence

| Tool | Storage | Location |
|------|---------|----------|
| Gurgeh | YAML files | `.gurgeh/specs/*.yaml` |
| Coldwine | YAML files | `.coldwine/specs/*.yaml` |
| Pollard | YAML + SQLite | `.pollard/sources/`, `.pollard/state.db` |
| Events | SQLite | `~/.autarch/events.db` |
| Bigend | None (read-only) | Aggregates from others |

### YAML Schema Example (Coldwine Epic)

```yaml
id: epic-001
prd_ref: PRD-001
title: "User Authentication System"
description: "Implement OAuth2 and JWT authentication"
status: open
priority: 1
stories:
  - id: story-001
    title: "Login flow"
    tasks:
      - id: task-001
        title: "Implement login form"
        status: todo
```

---

## Build System

### Development Script (`./dev`)

```bash
./dev bigend           # Build and run Bigend web
./dev bigend --tui     # Build and run Bigend TUI
./dev gurgeh           # Build and run Gurgeh TUI
./dev coldwine         # Build and run Coldwine TUI
```

### Direct Go Commands

```bash
go build ./cmd/...     # Build all binaries
go test ./...          # Run all tests
go run ./cmd/pollard   # Run Pollard CLI
```

### Binary Outputs

```bash
./autarch              # Unified CLI (future)
./bigend               # Mission control
./coldwine             # Task orchestration
./gurgeh               # PRD generation
./pollard              # Research CLI
```

---

## Configuration

### Global Config (`~/.config/autarch/`)

```toml
# agents.toml - Global agent targets
[agents.claude]
command = "claude"
args = ["--print"]

[agents.codex]
command = "codex"
```

### Tool-Specific Config

| Tool | Config File | Purpose |
|------|-------------|---------|
| Bigend | `~/.config/bigend/config.toml` | Scan roots, server settings |
| Gurgeh | `.gurgeh/config.toml` | Agent profiles |
| Coldwine | `.coldwine/config.toml` | Worktree settings |
| Pollard | `.pollard/config.yaml` | Hunter configuration |

### Environment Variables

| Variable | Tool | Purpose |
|----------|------|---------|
| `INTERMUTE_URL` | All | Intermute server URL |
| `INTERMUTE_API_KEY` | All | Intermute authentication |
| `INTERMUTE_PROJECT` | All | Project scope |
| `VAUXHALL_PORT` | Bigend | Web server port (default: 8099) |
| `GITHUB_TOKEN` | Pollard | GitHub API access |

---

## Testing Strategy

### Unit Tests

```bash
# Test specific packages
go test ./internal/pollard/hunters -v
go test ./pkg/contract -v
go test ./pkg/events -v
```

### Integration Tests

```bash
# Full stack test (requires Intermute)
export INTERMUTE_URL=http://localhost:8080
go test ./... -tags=integration
```

### TUI Testing

TUI components use Bubble Tea's testing utilities:

```go
func TestModel(t *testing.T) {
    m := NewModel()
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
    // Assert state changes
}
```

---

## Related Documentation

- [INTEGRATION.md](./INTEGRATION.md) - Cross-tool integration details
- [WORKFLOWS.md](./WORKFLOWS.md) - End-user task guides
- [QUICK_REFERENCE.md](./QUICK_REFERENCE.md) - Command cheat sheet
- [bigend/AGENTS.md](./bigend/AGENTS.md) - Bigend developer guide
- [pollard/AGENTS.md](./pollard/AGENTS.md) - Pollard developer guide
- [coldwine/AGENTS.md](./coldwine/AGENTS.md) - Coldwine developer guide
- [gurgeh/AGENTS.md](./gurgeh/AGENTS.md) - Gurgeh developer guide
