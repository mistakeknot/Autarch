# Autarch

Catch up on work across your projects and agent sessions, inspect the questions
they left, and return to the original conversation to answer.

Bare `autarch` opens the daily catch-up: recent commit subjects and dated agent
reports. Undated working-tree changes are counted separately. The existing
mission-control, PRD, orchestration, and research tools remain available.

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
# Build and open the daily catch-up (Go 1.25+, tmux)
GOWORK=off go build -mod=readonly -o autarch ./cmd/autarch
./autarch
./autarch --since 24h  # explicit window; leaves the last-visit stamp unchanged

# Build all tools
go build ./cmd/...

# Tool dashboard
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

| In the daily catch-up | Key |
|---|---|
| Scroll recent changes | ↑ / ↓ |
| Review questions | `a`, then Enter for full evidence |
| Open the selected question's terminal seat | Enter from the evidence view |
| Resume saved history when its agent is stopped or replaced | `s` from the evidence view |
| Inspect every thread, including unread ones | `t`, then `i` for evidence |
| Open project rows | Tab |
| Refresh / return / quit | `r` / Esc / `q` |

Questions come from the trailing 4 MiB of Claude Code and Codex transcripts
linked by conversation IDs in tmux session names. Autarch distinguishes a
question visible in a current pane from saved history and uncertain waits.
An ordinary later reply is preserved for review; only an explicit structured
answer confirms resolution. Seats without IDs and unread sources remain
visible in coverage. Autarch sends no answers; resume opens the agent CLI
without a new prompt. `autarch threads --json` exposes the same transcript
evidence, including its private text, for local inspection.

## Tech stack

- **Language:** Go 1.25+
- **TUI:** Bubble Tea + Lip Gloss (Tokyo Night palette)
- **Web:** net/http + htmx + Tailwind
- **Database:** SQLite (WAL mode, pure Go driver)

## Prerequisites

- macOS or Linux
- Go 1.25+
- tmux (for the daily catch-up and Coldwine)

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
