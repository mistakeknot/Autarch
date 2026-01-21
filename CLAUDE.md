# Vauxpraudemonium

> See `AGENTS.md` for comprehensive development guide.

## Overview

Unified monorepo for AI agent development tools:
- **Vauxhall**: Multi-project agent mission control (web + TUI)
- **Praude**: TUI-first PRD generation and validation
- **Tandemonium**: Task orchestration for human-AI collaboration

## Quick Commands

```bash
# Build and run
./dev vauxhall --tui    # Vauxhall TUI mode
./dev vauxhall          # Vauxhall web mode
./dev praude            # Praude TUI
./dev tandemonium       # Tandemonium TUI

# Build all
go build ./cmd/...

# Test
go test ./...
```

## Key Paths

| Path | Purpose |
|------|---------|
| `cmd/` | Entry points for each tool |
| `internal/{tool}/` | Tool-specific code |
| `pkg/tui/` | Shared TUI styles (Tokyo Night) |
| `docs/{tool}/` | Tool-specific documentation |

## Design Decisions (Do Not Re-Ask)

- Module: `github.com/mistakeknot/vauxpraudemonium`
- Shared TUI package with Tokyo Night colors
- Bubble Tea for all TUIs
- htmx + Tailwind for Vauxhall web
- SQLite for local state (read-only to external DBs)
- tmux integration via CLI commands
