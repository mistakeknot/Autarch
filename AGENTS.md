# Vauxpraudemonium - Development Guide

Unified monorepo for AI agent development tools: Vauxhall, Praude, and Tandemonium.

## Quick Reference

| Tool | Purpose | Entry Point |
|------|---------|-------------|
| **Vauxhall** | Multi-project agent mission control (web + TUI) | `./dev vauxhall` |
| **Praude** | TUI-first PRD generation and validation | `./dev praude` |
| **Tandemonium** | Task orchestration for human-AI collaboration | `./dev tandemonium` |

| Item | Value |
|------|-------|
| Language | Go 1.24+ |
| Module | `github.com/mistakeknot/vauxpraudemonium` |
| TUI Framework | Bubble Tea + lipgloss |
| Web Framework | net/http + htmx + Tailwind |
| Database | SQLite (WAL mode) |

## Project Status

### Done
- Monorepo structure with shared TUI package
- All three tools build and run
- Tokyo Night color palette standardized

### In Progress
- Vauxhall TUI mode
- MCP Agent Mail integration

### TODO
- Migrate TUI components to use shared `pkg/tui`
- Remote host support for Vauxhall
- Cross-tool coordination features

---

## Project Structure

```
Vauxpraudemonium/
├── cmd/
│   ├── vauxhall/           # Vauxhall entry point
│   ├── praude/             # Praude entry point
│   └── tandemonium/        # Tandemonium entry point
├── internal/
│   ├── vauxhall/           # Vauxhall-specific code
│   │   ├── aggregator/     # Data aggregation
│   │   ├── agentmail/      # MCP Agent Mail integration
│   │   ├── claude/         # Claude session detection
│   │   ├── config/         # Configuration
│   │   ├── discovery/      # Project scanner
│   │   ├── tmux/           # tmux client with caching
│   │   ├── tui/            # Bubble Tea TUI
│   │   └── web/            # HTTP server + templates
│   ├── praude/             # Praude-specific code
│   │   ├── agents/         # Agent profile management
│   │   ├── brief/          # Brief composer
│   │   ├── cli/            # CLI commands
│   │   ├── config/         # Configuration
│   │   ├── git/            # Git auto-commit
│   │   ├── project/        # Project detection
│   │   ├── research/       # Research outputs
│   │   ├── scan/           # Codebase scanner
│   │   ├── specs/          # PRD schema, validation
│   │   ├── suggestions/    # Staged updates
│   │   └── tui/            # Bubble Tea TUI
│   └── tandemonium/        # Tandemonium-specific code
│       ├── agent/          # Agent adapters
│       ├── cli/            # CLI commands
│       ├── config/         # Configuration
│       ├── git/            # Git/worktree management
│       ├── project/        # Project detection
│       ├── specs/          # Task schema
│       ├── storage/        # SQLite storage
│       ├── tmux/           # tmux integration
│       └── tui/            # Bubble Tea TUI
├── pkg/
│   ├── agenttargets/       # Shared run-target registry/resolver
│   └── tui/                # Shared TUI styles (Tokyo Night)
│       ├── colors.go       # Color palette
│       ├── styles.go       # Base styles
│       └── components.go   # StatusIndicator, AgentBadge, etc.
├── mcp-client/             # TypeScript MCP client
├── mcp-server/             # TypeScript MCP server
├── prototypes/             # Experimental code
├── docs/
│   ├── vauxhall/           # Vauxhall docs
│   ├── praude/             # Praude docs
│   └── tandemonium/        # Tandemonium docs
├── dev                     # Unified dev script
├── go.mod
└── go.sum
```

---

## Development Setup

### Prerequisites
- Go 1.24+
- tmux (for session management)
- Node.js (for MCP TypeScript components)

### Build & Run

```bash
# Build all
go build ./cmd/...

# Build and run individual tools
./dev vauxhall           # Web mode (default)
./dev vauxhall --tui     # TUI mode
./dev praude             # TUI mode
./dev praude list        # CLI mode
./dev tandemonium        # TUI mode
./dev tandemonium list   # CLI mode

# Test all
go test ./...

# Test specific package
go test ./internal/vauxhall/tmux -v
```

### Configuration

**Shared agent targets** (global + per-project overrides):

- Global: `~/.config/vauxpraudemonium/agents.toml`
- Project: `.praude/agents.toml`
- Compat: `.praude/config.toml` `[agents]` (used if `.praude/agents.toml` missing)

Example:
```toml
[targets.codex]
command = "codex"
args = []

[targets.claude]
command = "claude"
args = []
```

**Vauxhall** (`~/.config/vauxhall/config.toml`):
```toml
[server]
port = 8099
host = "0.0.0.0"

[discovery]
scan_roots = ["~/projects"]
scan_interval = "30s"
```

**Praude** (`.praude/config.toml`):
```toml
[agents.claude]
command = "claude"
args = ["--print", "--dangerously-skip-permissions"]

[agents.codex]
command = "codex"
args = ["--approval-mode", "full-auto"]
```

**Tandemonium** (`.tandemonium/config.yml`):
```yaml
default_branch: main
worktree_root: .tandemonium/worktrees
```

---

## Tool-Specific Details

### Vauxhall

Mission control dashboard for monitoring AI agents across projects.

**Data Sources:**
| Source | Location | Data |
|--------|----------|------|
| Praude | `.praude/specs/*.yaml` | PRDs, requirements |
| Tandemonium | `.tandemonium/specs/*.yaml` | Tasks, states |
| MCP Agent Mail | `~/.agent_mail/` | Cross-project messages |
| tmux | `tmux list-sessions` | Active sessions |

**Key Features:**
- Web dashboard with htmx
- TUI mode with Bubble Tea
- tmux session detection with status (running/waiting/idle/error)
- Claude session ID detection
- Cached tmux data (2-second TTL)

### Praude

TUI-first PRD generation and validation CLI.

**Key Paths:**
- `.praude/specs/` - PRD YAML files (source of truth)
- `.praude/research/` - Market/competitive research
- `.praude/suggestions/` - Staged updates for review
- `.praude/briefs/` - Agent briefs (timestamped)

**Commands:**
```bash
praude              # Launch TUI
praude init         # Initialize .praude/
praude list         # List PRDs
praude show <id>    # Show PRD details
praude run <brief>  # Spawn agent with brief
```

### Tandemonium

Task orchestration with git worktree isolation.

**Key Paths:**
- `.tandemonium/tasks.yml` - Task definitions
- `.tandemonium/config.yml` - Configuration
- `.tandemonium/activity.log` - Audit log (JSONL)
- `.tandemonium/worktrees/` - Isolated git worktrees

**Task States:** `todo` → `in_progress` → `review` → `done` (or `blocked`)

**Commands:**
```bash
tandemonium              # Launch TUI
tandemonium init         # Initialize
tandemonium add "Title"  # Create task
tandemonium start <id>   # Start task (creates worktree)
tandemonium complete <id>
```

---

## Shared TUI Package

`pkg/tui` provides consistent styling across all tools.

**Colors (Tokyo Night):**
```go
ColorPrimary   = "#7aa2f7"  // Blue
ColorSecondary = "#bb9af7"  // Purple
ColorSuccess   = "#9ece6a"  // Green
ColorWarning   = "#e0af68"  // Yellow
ColorError     = "#f7768e"  // Red
ColorMuted     = "#565f89"  // Gray
```

**Components:**
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

## Code Conventions

- Use `internal/` for all tool-specific packages
- Use `pkg/` only for shared code across tools
- Error handling: wrap with `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` with structured fields
- No external dependencies for core functionality
- SQLite: read-only connections to external DBs

### Testing
- TDD for behavior changes
- Run targeted tests while iterating: `go test ./internal/<pkg> -v`
- Small unit tests over broad integration tests

---

## Environment Variables

| Variable | Tool | Default |
|----------|------|---------|
| `VAUXHALL_PORT` | Vauxhall | 8099 |
| `VAUXHALL_SCAN_ROOTS` | Vauxhall | ~/projects |
| `PRAUDE_CONFIG` | Praude | .praude/config.toml |
| `TANDEMONIUM_CONFIG` | Tandemonium | .tandemonium/config.yml |

---

## Git Workflow

### Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: vauxhall, praude, tandemonium, tui, build
```

### Landing a Session
1. Run tests: `go test ./...`
2. Commit changes with clear messages
3. Push to remote: `git push`
4. Create issues for remaining work

---

## Integration Points

### Praude → Tandemonium
- Tandemonium reads `.praude/specs/` for PRD context
- Tasks can reference PRD IDs

### Vauxhall → All
- Reads Praude specs, Tandemonium tasks, MCP Agent Mail
- Monitors tmux sessions across all projects
- Read-only aggregation (observes, doesn't control)

### MCP Agent Mail
- Cross-project agent coordination
- File reservations for conflict prevention
- Message routing between agents
