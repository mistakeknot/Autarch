# Vauxpraudemonium

> Unified monorepo for AI agent development tools. See `AGENTS.md` for full development guide.

## Overview

Vauxpraudemonium combines three complementary tools for AI-assisted software development:
- **Vauxhall**: Multi-project agent mission control dashboard (web + TUI)
- **Praude**: TUI-first PRD generation and validation CLI
- **Tandemonium**: Task orchestration for human-AI collaboration

## Status

In development (not yet deployed)

## Quick Commands

```bash
# Build all
go build ./cmd/...

# Build individual tools
go build -o vauxhall ./cmd/vauxhall
go build -o praude ./cmd/praude
go build -o tandemonium ./cmd/tandemonium

# Run Vauxhall (web mode)
./vauxhall

# Run Vauxhall (TUI mode)
./vauxhall --tui

# Run praude
./praude

# Run Tandemonium
./tandemonium

# Test all
go test ./...
```

## Project Structure

```
/
├── cmd/
│   ├── vauxhall/       # Vauxhall entry point
│   ├── praude/         # Praude entry point
│   └── tandemonium/    # Tandemonium entry point
├── internal/
│   ├── vauxhall/       # Vauxhall-specific code
│   ├── praude/         # Praude-specific code
│   └── tandemonium/    # Tandemonium-specific code
├── pkg/
│   └── tui/            # Shared TUI styles and components
├── mcp-client/         # TypeScript MCP client
├── mcp-server/         # TypeScript MCP server
├── prototypes/         # Experimental code
└── docs/               # Documentation
```

## Design Decisions (Do Not Re-Ask)

- Monorepo structure with shared Go module
- Shared TUI package (`pkg/tui`) for consistent styling across all tools
- Tokyo Night color palette for TUI components
- Each tool maintains its own internal structure but shares dependencies
- Module path: `github.com/mistakeknot/vauxpraudemonium`
- Go + Bubble Tea for TUI components
- Web stack (Vauxhall): Go backend + htmx + Tailwind (server-rendered, minimal JS)
- SQLite for local state aggregation
- Discovers projects by scanning for `.praude/` and `.tandemonium/` directories
- Connects to MCP Agent Mail SQLite DBs directly (read-only)
- tmux integration via `tmux` CLI commands
