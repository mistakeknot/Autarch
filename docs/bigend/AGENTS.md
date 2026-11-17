# Bigend - Development Guide

> Multi-project AI agent mission control dashboard (web + TUI)

Named after Hubertus Bigend from William Gibson's *Pattern Recognition* - a Belgian advertising magnate who appears to know everything happening across his global network.

## Quick Reference

| Item | Value |
|------|-------|
| Entry Point | `cmd/bigend/main.go` |
| Web Port | 8099 (default) |
| Config | `~/.config/bigend/config.toml` |
| TUI Framework | Bubble Tea + lipgloss |
| Web Framework | net/http + htmx + Tailwind |

```bash
# Run commands
./dev bigend           # Web mode (default) - http://localhost:8099
./dev bigend --tui     # TUI mode
go build ./cmd/bigend  # Build binary
```

---

## Key Paths

| Path | Purpose |
|------|---------|
| `cmd/bigend/` | Entry point with CLI flags |
| `internal/bigend/aggregator/` | Core data aggregation + WebSocket events |
| `internal/bigend/agentcmd/` | Agent command resolver |
| `internal/bigend/coldwine/` | Coldwine (task) reader |
| `internal/bigend/config/` | TOML configuration |
| `internal/bigend/discovery/` | Project scanner |
| `internal/bigend/mcp/` | MCP server/client management |
| `internal/bigend/statedetect/` | Agent state detection (NudgeNik-style) |
| `internal/bigend/tmux/` | tmux client with caching |
| `internal/bigend/tui/` | Bubble Tea TUI application |
| `internal/bigend/web/` | HTTP server + htmx templates |

---

## Architecture

```
                    ┌─────────────────────────────┐
                    │         Bigend              │
                    │    (Mission Control)        │
                    └──────────┬──────────────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
    ┌─────────────────┐ ┌───────────┐ ┌─────────────────┐
    │   Aggregator    │ │    Web    │ │      TUI        │
    │  (Data Hub)     │ │  Server   │ │  (Bubble Tea)   │
    └────────┬────────┘ └─────┬─────┘ └────────┬────────┘
             │                │                │
             │    ┌───────────┴───────────┐    │
             │    │   HTTP + WebSocket    │    │
             │    │   htmx templates      │    │
             │    └───────────────────────┘    │
             │                                 │
    ┌────────┴─────────────────────────────────┴────────┐
    │                   Aggregator State                │
    │  Projects | Agents | Sessions | Activities | MCP  │
    └────────────────────────┬──────────────────────────┘
                             │
     ┌───────────┬───────────┼───────────┬───────────┐
     ▼           ▼           ▼           ▼           ▼
┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐
│ Gurgeh  │ │Coldwine │ │Intermute│ │  tmux   │ │   MCP   │
│ Specs   │ │ Tasks   │ │ Agents  │ │Sessions │ │ Servers │
└─────────┘ └─────────┘ └─────────┘ └─────────┘ └─────────┘
```

### Data Sources

| Source | Location | Data Retrieved |
|--------|----------|----------------|
| **Gurgeh** | `.gurgeh/specs/*.yaml` | PRD summaries, status counts |
| **Coldwine** | `.coldwine/specs/*.yaml` | Task stats (todo/in_progress/done) |
| **Pollard** | `.pollard/` | Source/insight counts, reports |
| **Intermute** | HTTP API | Registered agents, messages, reservations |
| **tmux** | `tmux list-sessions` | Active sessions, pane content |
| **MCP** | `mcp-server/`, `mcp-client/` | Component run status |

Session discovery uses `internal/bigend/tmux/`: the aggregator lists tmux sessions
and detects agent types from session names. The aggregator calls
`internal/bigend/statedetect/` to enrich sessions with agent state.
It does not read Claude's configuration for session IDs.

### Aggregator

The `Aggregator` (`internal/bigend/aggregator/aggregator.go`) is the central data hub:

```go
type Aggregator struct {
    scanner         *discovery.Scanner      // Project discovery
    tmuxClient      tmuxAPI                 // tmux operations
    stateDetector   *statedetect.Detector   // Agent state detection
    intermuteClient *intermute.Client       // Agent coordination
    mcpManager      *mcp.Manager            // MCP component lifecycle
    resolver        *agentcmd.Resolver      // Agent launch commands
    handlers        map[string][]EventHandler // WebSocket handlers
    state           State                   // Aggregated view
}
```

**Key Methods:**
- `Refresh(ctx)` - Full rescan of all data sources
- `GetState()` - Current aggregated state
- `ConnectWebSocket(ctx)` - Real-time Intermute events
- `NewSession(name, path, agentType)` - Create tmux session
- `StartMCP(ctx, path, component)` - Start MCP server/client

---

## Data Flow

### Refresh Cycle

```
                   Refresh()
                      │
    ┌─────────────────┼─────────────────┐
    │                 │                 │
    ▼                 ▼                 ▼
┌─────────┐    ┌───────────┐    ┌───────────┐
│  Scan   │    │   Load    │    │   Load    │
│Projects │    │  Agents   │    │ Sessions  │
└────┬────┘    └─────┬─────┘    └─────┬─────┘
     │               │                │
     ▼               ▼                ▼
┌─────────┐    ┌───────────┐    ┌───────────┐
│ Enrich  │    │ Intermute │    │   tmux    │
│ Stats   │    │   API     │    │  client   │
└────┬────┘    └─────┬─────┘    └─────┬─────┘
     │               │                │
     └───────────────┼────────────────┘
                     ▼
              ┌────────────┐
              │   State    │
              │  Updated   │
              └────────────┘
```

### WebSocket Events (Real-time)

When connected to Intermute WebSocket, events trigger targeted refreshes:

| Event Prefix | Refresh Action |
|--------------|----------------|
| `spec.*`, `epic.*`, `story.*`, `task.*` | Gurgeh stats |
| `agent.*`, `message.*` | Agent list |
| `insight.*` | Pollard stats |
| `reservation.*` | Activity log only |

---

## Configuration

### Global Config (`~/.config/bigend/config.toml`)

```toml
[server]
port = 8099
host = "127.0.0.1"

[discovery]
scan_roots = ["~/projects", "/root/projects"]
scan_interval = "30s"

[agents.claude]
command = "claude"
args = []

[agents.codex]
command = "codex"
args = []

[mcp.server]
command = "npm"
args = ["run", "dev"]
workdir = ""  # Relative to project, defaults to mcp-server/

[mcp.client]
command = "npm"
args = ["run", "dev"]
workdir = ""  # Relative to project, defaults to mcp-client/
```

### Project-Specific

Projects can have their own agent configs in `.gurgeh/agents.toml`:

```toml
[targets.claude]
command = "claude"
args = ["--model", "sonnet"]
```

---

## Adding a New Data Source

To add a new data source to Bigend:

### 1. Create Reader Package

```go
// internal/bigend/newtool/reader.go
package newtool

type Reader struct {
    projectPath string
}

func NewReader(projectPath string) *Reader {
    return &Reader{projectPath: projectPath}
}

func (r *Reader) Exists() bool {
    _, err := os.Stat(filepath.Join(r.projectPath, ".newtool"))
    return err == nil
}

func (r *Reader) GetStats() (*Stats, error) {
    // Read and parse data files
}
```

### 2. Add Stats to Discovery Project

```go
// internal/bigend/discovery/scanner.go
type Project struct {
    // ... existing fields
    HasNewTool   bool       `json:"has_newtool"`
    NewToolStats *NewToolStats `json:"newtool_stats,omitempty"`
}

type NewToolStats struct {
    // Your metrics
}
```

### 3. Enrich in Aggregator

```go
// internal/bigend/aggregator/aggregator.go
func (a *Aggregator) enrichWithNewToolStats(projects []discovery.Project) {
    for i := range projects {
        if !projects[i].HasNewTool {
            continue
        }
        reader := newtool.NewReader(projects[i].Path)
        stats, err := reader.GetStats()
        if err != nil {
            slog.Warn("failed to read newtool stats", "error", err)
            continue
        }
        projects[i].NewToolStats = stats
    }
}
```

### 4. Call Enrichment in Refresh

```go
func (a *Aggregator) Refresh(ctx context.Context) error {
    // ... existing code
    a.enrichWithTaskStats(projects)
    a.enrichWithGurgStats(projects)
    a.enrichWithPollardStats(projects)
    a.enrichWithNewToolStats(projects)  // Add here
    // ...
}
```

### 5. Add Web Template

```html
<!-- internal/bigend/web/templates/partials/newtool_card.html -->
{{ if .HasNewTool }}
<div class="card">
    <h3>NewTool</h3>
    <!-- Display stats -->
</div>
{{ end }}
```

---

## Agent State Detection

Bigend uses NudgeNik-style state detection to determine what agents are doing:

### States

| State | Description |
|-------|-------------|
| `working` | Actively processing (tool calls, code generation) |
| `waiting` | Waiting for user input |
| `blocked` | Error or stuck state |
| `stalled` | No activity for extended period |
| `done` | Task completed |
| `unknown` | Cannot determine state |

### Detection Sources

| Source | Method |
|--------|--------|
| `pattern` | Regex matching on terminal output |
| `repetition` | Detecting repeated output (stuck loops) |
| `activity` | Time since last activity |
| `llm` | (Future) LLM-based classification |

### Implementation

```go
// internal/bigend/statedetect/detector.go
type Detector struct {
    patternRules []PatternRule
    history      map[string][]OutputSample
}

func (d *Detector) Detect(session, output, agentType string, lastActivity time.Time) Result {
    // 1. Check pattern matches
    // 2. Check for repetition
    // 3. Check activity staleness
    // 4. Return state with confidence
}
```

---

## tmux Integration

### Client with Caching

```go
// internal/bigend/tmux/client.go
type Client struct {
    cache     map[string]Session
    cacheTTL  time.Duration  // 2 seconds default
    cacheTime time.Time
}

func (c *Client) ListSessions() ([]Session, error) {
    if time.Since(c.cacheTime) < c.cacheTTL {
        return c.cachedSessions(), nil
    }
    // Execute tmux list-sessions and parse
}
```

### Session Operations

| Method | Description |
|--------|-------------|
| `ListSessions()` | List all sessions with metadata |
| `CapturePane(session, lines)` | Get recent terminal output |
| `NewSession(name, path, cmd)` | Create new session |
| `KillSession(name)` | Terminate session |
| `AttachSession(name)` | Attach (TUI mode) |
| `RenameSession(old, new)` | Rename session |

### Agent Detection

The detector matches sessions to agents and projects:

```go
// internal/bigend/tmux/detector.go
type Detector struct {
    projectPaths []string
}

func (d *Detector) EnrichSessions(sessions []Session) []EnrichedSession {
    // Call DetectAgent per session: match the name against agent patterns.
    // For a matched name, set AgentInfo.ProjectPath to the project matching its CWD.
    // Unmatched names get no AgentInfo, regardless of CWD.
}
```

---

## Testing

### Unit Tests

```bash
# Test tmux client
go test ./internal/bigend/tmux -v

# Test aggregator
go test ./internal/bigend/aggregator -v

# Test discovery
go test ./internal/bigend/discovery -v
```

### Integration Testing

```bash
# Start web server and verify
./dev bigend &
curl http://localhost:8099/health

# Check project discovery
curl http://localhost:8099/api/projects

# Check sessions
curl http://localhost:8099/api/sessions
```

### TUI Testing

```bash
# Run TUI mode (requires tmux)
./dev bigend --tui

# Verify keybindings work
# - j/k: Navigate sessions
# - Enter: View session details
# - q: Quit
```

---

## Web Interface

### Routes

| Route | Method | Description |
|-------|--------|-------------|
| `/` | GET | Dashboard |
| `/projects` | GET | Project list |
| `/projects/:path` | GET | Project detail |
| `/agents` | GET | Agent list |
| `/agents/:name` | GET | Agent detail |
| `/sessions` | GET | tmux session list |
| `/sessions/:name` | GET | Session detail + terminal |
| `/api/state` | GET | Full state JSON |
| `/ws` | WS | WebSocket for real-time updates |

### htmx Patterns

Templates use htmx for dynamic updates:

```html
<!-- Auto-refresh every 5 seconds -->
<div hx-get="/partials/sessions" hx-trigger="every 5s">
    {{ template "sessions" .Sessions }}
</div>

<!-- WebSocket connection for real-time -->
<div hx-ext="ws" ws-connect="/ws">
    <div id="activity-feed"></div>
</div>
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `INTERMUTE_URL` | - | Intermute server URL (required for agents) |
| `INTERMUTE_API_KEY` | - | Intermute authentication |
| `INTERMUTE_PROJECT` | - | Intermute project scope |

---

## Troubleshooting

### No Projects Found

```bash
# Check scan roots in config
cat ~/.config/bigend/config.toml

# Verify projects have expected markers
ls -la ~/projects/*/.gurgeh 2>/dev/null
ls -la ~/projects/*/.coldwine 2>/dev/null
ls -la ~/projects/*/.pollard 2>/dev/null
```

### tmux Sessions Not Detected

```bash
# Verify tmux is available
tmux -V

# List sessions manually
tmux list-sessions

# Check Bigend can parse output
go test ./internal/bigend/tmux -v -run TestListSessions
```

### Intermute Connection Failed

```bash
# Check environment
echo $INTERMUTE_URL

# Test connectivity
curl $INTERMUTE_URL/health

# Check logs for connection errors
./dev bigend 2>&1 | grep -i intermute
```

### Agent State Always Unknown

- Ensure session has agent name pattern (e.g., `project-claude`)
- Check pane capture is returning content
- Verify state detection patterns in `statedetect/patterns.go`

---

## Roadmap

See `docs/bigend/roadmap.md` for full milestone breakdown. Key upcoming:

| Milestone | Description |
|-----------|-------------|
| M5 | Live terminal streaming via WebSocket |
| M6 | Unified activity feed |
| M7 | Agent control (start/stop/message) |
| M8 | Multi-host support (SSH tunnels) |
