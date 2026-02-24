# Autarch

Four tools for the operational side of running AI agents: mission control, PRD generation, task orchestration, and research intelligence.

## What this does

Running multiple AI agents across multiple projects creates a coordination problem: which agent is working on what, which specs are current, which tasks are blocked, and what's happening in the competitive landscape. Autarch provides four tools that each solve a piece of this:

| Tool | What it does | Interface |
|------|-------------|-----------|
| **Bigend** | Multi-project agent mission control | Web + TUI |
| **Gurgeh** | PRD generation and validation | TUI |
| **Coldwine** | Task orchestration for human-AI collaboration | TUI |
| **Pollard** | Continuous research intelligence (tech, medicine, law, economics) | CLI |

## Who this is for

Developers running the Demarch agent stack (Clavain, Intermute, Intercore) who want visibility and control over multi-project, multi-agent workflows. Autarch is the operational dashboard layer.

## Quick start

```bash
# Build all tools
go build ./cmd/...

# Unified TUI (recommended)
./dev autarch tui

# Individual tools
./dev bigend          # Mission control (web mode)
./dev bigend --tui    # Mission control (TUI mode)
./dev gurgeh          # PRD generation
./dev coldwine        # Task orchestration

# Pollard (research intelligence)
go run ./cmd/pollard init
go run ./cmd/pollard scan
go run ./cmd/pollard report
```

## Tech stack

- **Language:** Go 1.24+
- **TUI:** Bubble Tea + Lip Gloss (Tokyo Night palette)
- **Web:** net/http + htmx + Tailwind
- **Database:** SQLite (WAL mode, pure Go driver)

## Prerequisites

- macOS or Linux
- Go 1.24+
- tmux (for Coldwine)

## Project structure

```
cmd/           Entry points (bigend, gurgeh, coldwine, pollard)
internal/      Tool-specific code
pkg/tui/       Shared TUI styles (Tokyo Night)
pkg/contract/  Cross-tool entity types
pkg/events/    Event spine (SQLite)
```

## Intermute integration

Autarch auto-registers with the Intermute coordination service when `INTERMUTE_URL` is set:

```bash
export INTERMUTE_URL="http://localhost:7338"
```

Bigend handles session I/O; Intermute provides coordination and messaging.

## License

MIT
