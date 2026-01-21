# Vauxhall Agent Handoff

> Mission control dashboard for monitoring AI coding agents across projects

## Quick Start

```bash
cd ~/projects/Vauxhall
go build ./cmd/vauxhall
./vauxhall --scan-root ~/projects
# Server runs at http://0.0.0.0:8099
```

## Project Location

```
~/projects/Vauxhall/
```

## What This Is

Vauxhall is a web dashboard that aggregates data from multiple AI agent tooling systems:

| Source | What it provides |
|--------|------------------|
| **Praude** (`.praude/`) | PRD specs, requirements, CUJs |
| **Tandemonium** (`.tandemonium/`) | Tasks, agent messages, file reservations |
| **MCP Agent Mail** | Cross-project agent coordination |
| **tmux** | Active terminal sessions |

The goal: see all your AI agents across all projects in one place, watch their terminal output live, and understand what they're working on.

## Current State (M0 Complete)

**What's built:**
- Project discovery - scans directories for `.praude/`, `.tandemonium/`, `.agent_mail/`
- Web server - Go + htmx + Tailwind on port 8099
- Basic templates - dashboard, projects, agents, sessions views
- Configuration - TOML config file support, CLI flags

**What's NOT built yet:**
- tmux session listing
- Praude YAML parsing
- Tandemonium SQLite reading
- MCP Agent Mail reading
- Live terminal streaming
- Activity feed

## Key Files

| File | Purpose |
|------|---------|
| `cmd/vauxhall/main.go` | Entry point, CLI flags, server startup |
| `internal/config/config.go` | Configuration loading (TOML) |
| `internal/discovery/scanner.go` | Scans for projects with tooling |
| `internal/aggregator/aggregator.go` | Combines all data sources (mostly stubs) |
| `internal/web/server.go` | HTTP server, routes, template rendering |
| `internal/web/templates/*.html` | htmx + Tailwind templates |
| `docs/roadmap.md` | Detailed milestone breakdown |
| `AGENTS.md` | Full architecture and conventions |

## Next Milestone: M1 (tmux Integration)

**Goal:** Show all tmux sessions in the dashboard and identify which ones have AI agents.

### Tasks

1. **Create `internal/tmux/client.go`**
   - Function to list all tmux sessions
   - Parse output of `tmux list-sessions -F "#{session_name}|#{session_created}|#{session_windows}|#{session_attached}"`
   - Return `[]TmuxSession` structs

2. **Create `internal/tmux/detector.go`**
   - Heuristics to identify agent sessions:
     - Session name contains "claude", "codex", "agent"
     - Window title patterns
     - Check CWD of session against known project paths
   - Function: `DetectAgent(session TmuxSession) *AgentInfo`

3. **Update `internal/aggregator/aggregator.go`**
   - Call tmux client in `Refresh()`
   - Populate `state.Sessions` with real data
   - Link sessions to projects by CWD matching

4. **Update `internal/web/templates/sessions.html`**
   - Show real session data
   - Add agent indicator badges
   - Link to project if detected

### tmux Commands Reference

```bash
# List sessions with format
tmux list-sessions -F "#{session_name}|#{session_created}|#{session_windows}|#{session_attached}"

# Get session CWD (for project linking)
tmux display-message -t SESSION_NAME -p "#{pane_current_path}"

# Capture pane output (for M5)
tmux capture-pane -t SESSION_NAME -p -S -100
```

### Data Types (already defined in aggregator.go)

```go
type TmuxSession struct {
    Name         string    `json:"name"`
    Created      time.Time `json:"created"`
    LastActivity time.Time `json:"last_activity"`
    WindowCount  int       `json:"window_count"`
    Attached     bool      `json:"attached"`
    AgentName    string    `json:"agent_name,omitempty"`
}
```

## Code Conventions

- **Packages:** All code in `internal/` (not a library)
- **Errors:** Wrap with context: `fmt.Errorf("failed to list sessions: %w", err)`
- **Logging:** Use `log/slog` with structured fields
- **Templates:** Go html/template with htmx attributes (`hx-get`, `hx-target`, etc.)
- **No frameworks:** htmx + vanilla JS only, no React/Vue
- **SQLite:** Read-only connections to external DBs

## Testing

```bash
# Build
go build ./cmd/vauxhall

# Run with specific scan root
./vauxhall --scan-root ~/projects

# Run with custom port
./vauxhall --port 9000

# Check if it finds projects
# Should see: level=INFO msg="refresh complete" projects=N agents=0
```

## Related Projects on This Server

| Project | Location | Relevance |
|---------|----------|-----------|
| **Praude** | `~/projects/praude` | PRD management TUI - read its `.praude/specs/*.yaml` |
| **Tandemonium** | `~/projects/Tandemonium` | Task orchestration - read its `.tandemonium/state.db` |

Both have `.praude/` or `.tandemonium/` directories that Vauxhall should discover.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     Vauxhall Web UI                         │
│         http://0.0.0.0:8099 (htmx + Tailwind)              │
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

## After M1

Once tmux integration works, the next milestones can be parallelized:

- **M2:** Praude integration (parse YAML specs)
- **M3:** Tandemonium integration (query SQLite)
- **M4:** MCP Agent Mail integration (query agents/messages)

Then:
- **M5:** Live terminal streaming (WebSocket + xterm.js)
- **M6:** Activity feed (aggregate events)

See `docs/roadmap.md` for full details.

## Git Status

```bash
cd ~/projects/Vauxhall
git log --oneline
# 5d404f3 docs: add detailed roadmap
# 21b7e31 feat: initial Vauxhall project scaffolding
```

Clean working tree, all changes committed.

## Questions?

Read these files:
- `AGENTS.md` - Full architecture, API endpoints, data models
- `docs/roadmap.md` - All milestones with task breakdowns
- `CLAUDE.md` - Quick reference
