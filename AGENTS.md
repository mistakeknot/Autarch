# Vauxhall - Development Guide

## Quick Reference

| Item | Value |
|------|-------|
| Language | Go 1.24+ |
| Web Framework | net/http + html/template + htmx |
| CSS | Tailwind CSS (CDN for dev) |
| Database | SQLite (WAL mode) |
| Default Port | 8099 |

## Project Status

### Done
- Initial project structure

### In Progress
- Core architecture design

### TODO
- [ ] Project discovery (scan for .praude/, .tandemonium/)
- [ ] Praude integration (read PRD specs)
- [ ] Tandemonium integration (read tasks, messages)
- [ ] MCP Agent Mail integration (read inboxes)
- [ ] tmux session detection
- [ ] Web dashboard UI
- [ ] Live terminal streaming (websocket)
- [ ] Agent activity timeline

---

## Architecture

### Core Concept

Vauxhall is a **read-mostly aggregator** that discovers and monitors existing project tooling:

```
┌─────────────────────────────────────────────────────────────┐
│                     Vauxhall Web UI                         │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────────┐   │
│  │Projects │ │ Agents  │ │ Tasks   │ │ Terminal Viewer │   │
│  │ List    │ │ Status  │ │ Board   │ │ (Live Stream)   │   │
│  └─────────┘ └─────────┘ └─────────┘ └─────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Vauxhall Server (Go)                     │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────────┐    │
│  │   Discovery  │ │  Aggregator  │ │  WebSocket Hub   │    │
│  │   Scanner    │ │   (SQLite)   │ │  (tmux streams)  │    │
│  └──────────────┘ └──────────────┘ └──────────────────┘    │
└─────────────────────────────────────────────────────────────┘
         │                   │                    │
         ▼                   ▼                    ▼
┌─────────────┐    ┌─────────────────┐    ┌─────────────┐
│  Filesystem │    │  Project DBs    │    │    tmux     │
│  .praude/   │    │  (read-only)    │    │  sessions   │
│  .tandemon/ │    │  - state.db     │    │             │
│             │    │  - agent_mail   │    │             │
└─────────────┘    └─────────────────┘    └─────────────┘
```

### Data Sources

| Source | Location | Data |
|--------|----------|------|
| Praude | `.praude/specs/*.yaml` | PRDs, CUJs, requirements |
| Praude | `.praude/research/*.md` | Agent research outputs |
| Tandemonium | `.tandemonium/specs/*.yaml` | Tasks, states |
| Tandemonium | `.tandemonium/state.db` | Messages, reservations |
| MCP Agent Mail | `~/.agent_mail/` or project `.agent_mail/` | Cross-project messages |
| tmux | `tmux list-sessions` | Active sessions |
| tmux | `tmux capture-pane` | Terminal output |

### Key Entities

```
Project
├── path: string (absolute path to project root)
├── name: string (directory name or from config)
├── has_praude: bool
├── has_tandemonium: bool
├── prds: []PRD (from .praude/specs/)
├── tasks: []Task (from .tandemonium/specs/)
└── agents: []Agent (detected from sessions + mail)

Agent
├── name: string (e.g., "BlueLake", "GreenCastle")
├── program: string (claude-code, codex-cli, etc.)
├── model: string
├── project: *Project
├── task: *Task (current task if any)
├── session: *TmuxSession (if active)
├── last_active: time.Time
└── inbox_count: int (unread messages)

TmuxSession
├── name: string (session name)
├── created: time.Time
├── last_activity: time.Time
├── window_count: int
├── attached: bool
└── agent: *Agent (if detected)
```

---

## Directory Structure

```
Vauxhall/
├── cmd/
│   └── vauxhall/
│       └── main.go           # Entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Configuration loading
│   ├── discovery/
│   │   └── scanner.go        # Project discovery
│   ├── praude/
│   │   └── reader.go         # Read Praude specs
│   ├── tandemonium/
│   │   └── reader.go         # Read Tandemonium data
│   ├── agentmail/
│   │   └── reader.go         # Read MCP Agent Mail
│   ├── tmux/
│   │   └── client.go         # tmux CLI wrapper
│   ├── aggregator/
│   │   └── aggregator.go     # Combine all data sources
│   └── web/
│       ├── server.go         # HTTP server
│       ├── handlers.go       # Route handlers
│       ├── websocket.go      # Terminal streaming
│       └── templates/
│           ├── layout.html
│           ├── dashboard.html
│           ├── projects.html
│           ├── agents.html
│           ├── tasks.html
│           └── terminal.html
├── static/
│   ├── css/
│   │   └── app.css           # Custom styles
│   └── js/
│       └── terminal.js       # xterm.js integration
├── go.mod
├── go.sum
├── CLAUDE.md
└── AGENTS.md
```

---

## Development Setup

### Prerequisites

- Go 1.24+
- tmux (for session management)
- Projects with .praude/ or .tandemonium/ directories

### Run Locally

```bash
cd ~/projects/Vauxhall
go run ./cmd/vauxhall --scan-root ~/projects
```

### Configuration

`~/.config/vauxhall/config.toml`:
```toml
[server]
port = 8099
host = "0.0.0.0"

[discovery]
scan_roots = ["~/projects", "~/work"]
scan_interval = "30s"
exclude_patterns = ["node_modules", ".git", "vendor"]

[tmux]
socket_path = ""  # default
```

---

## Web UI Design

### Dashboard (Home)

```
┌────────────────────────────────────────────────────────────────┐
│  Vauxhall                                    [Settings] [Help] │
├────────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌─ Active Agents ──────────────────────────────────────────┐  │
│  │                                                          │  │
│  │  🟢 BlueLake (claude-code)     praude      "TUI search"  │  │
│  │     └─ tmux: praude-dev        last: 2m ago              │  │
│  │                                                          │  │
│  │  🟢 GreenCastle (codex-cli)    tandemonium "Mail parity" │  │
│  │     └─ tmux: tand-work         last: 5m ago              │  │
│  │                                                          │  │
│  │  🟡 RedStone (claude-code)     smartedgar  "API routes"  │  │
│  │     └─ tmux: edgar-api         last: 1h ago (idle)       │  │
│  │                                                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
│  ┌─ Projects ───────────────────────────────────────────────┐  │
│  │                                                          │  │
│  │  praude         3 PRDs   2 tasks   1 agent active        │  │
│  │  tandemonium    5 PRDs   8 tasks   1 agent active        │  │
│  │  smartedgar     2 PRDs   4 tasks   1 agent idle          │  │
│  │  beads          0 PRDs   0 tasks   no agents             │  │
│  │                                                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
│  ┌─ Recent Activity ────────────────────────────────────────┐  │
│  │                                                          │  │
│  │  10:32  BlueLake committed "feat(tui): add search"       │  │
│  │  10:28  GreenCastle reserved files: internal/coord/*.go  │  │
│  │  10:15  BlueLake → GreenCastle: "Need API for inbox"     │  │
│  │  09:45  RedStone task TAND-003 → review                  │  │
│  │                                                          │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

### Project Detail

- PRD list (from Praude)
- Task board (from Tandemonium)
- Agent activity for this project
- File reservations
- Recent commits

### Agent Detail

- Current task and progress
- Message inbox/outbox
- File reservations held
- Terminal viewer (live stream from tmux)
- Activity timeline

### Terminal Viewer

- xterm.js for rendering
- WebSocket connection to tmux capture-pane
- Read-only by default
- Optional: send input to tmux (with confirmation)

---

## API Endpoints

### REST

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Dashboard |
| GET | `/projects` | Project list |
| GET | `/projects/:path` | Project detail |
| GET | `/agents` | Agent list |
| GET | `/agents/:name` | Agent detail |
| GET | `/tasks` | Cross-project task list |
| GET | `/sessions` | tmux session list |
| GET | `/api/refresh` | Trigger rescan |

### WebSocket

| Path | Description |
|------|-------------|
| `/ws/terminal/:session` | Stream tmux session output |
| `/ws/activity` | Live activity feed |

---

## Code Conventions

- Use `internal/` for all packages (not a library)
- Error handling: wrap with context using `fmt.Errorf("...: %w", err)`
- Logging: `log/slog` with structured fields
- Templates: Go html/template with htmx attributes
- No JavaScript frameworks; htmx + vanilla JS only
- SQLite: read-only connections to external DBs

---

## Integration Details

### Praude Integration

```go
// Read PRD spec
spec, err := praude.ReadSpec("/path/to/project/.praude/specs/PRD-001.yaml")

// List all PRDs
prds, err := praude.ListSpecs("/path/to/project/.praude/specs/")
```

### Tandemonium Integration

```go
// Open read-only connection to state.db
db, err := tandemonium.OpenDB("/path/to/project/.tandemonium/state.db")

// Query tasks
tasks, err := db.ListTasks()

// Query messages for agent
messages, err := db.InboxFor("BlueLake")
```

### tmux Integration

```go
// List sessions
sessions, err := tmux.ListSessions()

// Capture pane output
output, err := tmux.CapturePane("session-name", 0, 0, 100) // last 100 lines

// Stream output (for websocket)
ch, err := tmux.StreamPane("session-name", 0, 0)
```

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `VAUXHALL_PORT` | HTTP server port | 8099 |
| `VAUXHALL_HOST` | Bind address | 0.0.0.0 |
| `VAUXHALL_SCAN_ROOTS` | Comma-separated paths | ~/projects |
| `VAUXHALL_CONFIG` | Config file path | ~/.config/vauxhall/config.toml |

---

## Known Limitations

- Read-only: Vauxhall observes but doesn't control agents
- Single host: Currently only monitors local tmux sessions
- No auth: Assumes trusted local/tailscale network

## Future Ideas

- Remote host support (SSH to ethics-gradient, monitor its tmux)
- Agent control: start/stop/message agents from UI
- Notifications: alert when agent idle, task blocked, etc.
- Mobile-friendly UI for monitoring on the go
