# Autarch

> See `AGENTS.md` for comprehensive development guide.

## Overview

Unified monorepo for AI agent development tools:
- **Bigend**: Multi-project agent mission control (web + TUI)
- **Gurgeh**: TUI-first PRD generation and validation
- **Coldwine**: Task orchestration + sprint runs (Epics/Runs mode toggle, inline/split layouts)
- **Pollard**: General-purpose research intelligence (tech, medicine, law, economics, etc.)

## Quick Commands

```bash
# Unified TUI (recommended)
./dev autarch tui                    # Full onboarding flow
./dev autarch tui --skip-onboard     # Direct to dashboard
./dev autarch tui --tool=gurgeh      # Jump to Gurgeh tab
./dev autarch tui --inline           # Inline mode (preserves scrollback)

# Standalone CLI operations (no TUI)
./dev gurgeh list                    # List specs
./dev gurgeh export PRD-001          # Export spec to briefs
./dev coldwine status                # Show task status
./dev bigend                         # Web server mode

# Pollard CLI
go run ./cmd/pollard init           # Initialize .pollard/
go run ./cmd/pollard scan           # Run all hunters
go run ./cmd/pollard scan --hunter github-scout
go run ./cmd/pollard scan --hunter openalex   # Multi-domain academic
go run ./cmd/pollard scan --hunter pubmed     # Medical research
go run ./cmd/pollard report         # Generate landscape report
go run ./cmd/pollard report --type competitive
go run ./cmd/pollard watch --once    # Single competitor watch cycle
go run ./cmd/pollard watch           # Continuous monitoring

# API servers (local-only by default)
go run ./cmd/pollard serve --addr 127.0.0.1:8090   # Pollard research API
go run ./cmd/gurgeh serve --addr 127.0.0.1:8091    # Gurgeh spec API (read-only)
go run ./cmd/signals serve --addr 127.0.0.1:8092   # Signals WS server

# Gurgeh spec quality
go run ./cmd/gurgeh history <spec-id>       # Spec revision changelog
go run ./cmd/gurgeh diff <spec-id> v1 v2    # Structured version diff
go run ./cmd/gurgeh prioritize <spec-id>    # Agent-powered feature ranking

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
| `pkg/clavain/` | Clavain CLI wrapper (L2: sprint, gates, artifacts) |
| `pkg/intercore/` | Intercore CLI wrapper (L1: runs, state, coordination) |
| `docs/{tool}/` | Tool-specific documentation |
| `pkg/signals/` | Cross-tool signal types |
| `.coldwine/` | Coldwine data directory (state.db, specs, sessions) |
| `.pollard/` | Pollard data directory (sources, insights, reports) |
| `.pollard/watch/` | Competitor watch state |
| `.gurgeh/specs/history/` | Spec version snapshots |

## Workflow Discipline

**Review before implement:** When asked to "review", "look at", or "check" a plan/implementation, do NOT start implementing. First use the requested workflow tools to analyze and discuss. Only implement after explicit approval.

**Verify end-to-end before completion:** After making changes to data flow, exploration, or TUI features, verify the full user-facing result—don't assume success from tool execution alone. Run the actual flow and confirm data displays correctly.

**Pause after major refactoring:** For multi-file refactoring, pause after changes to run the affected flow manually. Confirm data propagates correctly through the pipeline before continuing.

## Execution Mode
codex-first: true

## Concurrency Rules

- Never return pointers to internal mutable state from synchronized methods. `State()` returns deep-copied snapshots via `Clone()`. All types crossing goroutine boundaries need `Clone()` methods.
- Run tests with `-race` flag.

## Bubble Tea Rules

- In parent `Update()` methods, never swallow messages that child views need. Default to fall-through. Only return early for messages exclusively owned by the parent. Error messages must always reach the view layer.
- Never use `[]rune` slicing on ANSI-styled strings for visual-column operations. Use `ansi.Truncate`/`TruncateLeft` from `charmbracelet/x/ansi`. Grep for `[]rune` + `lipgloss.Width` — that combination is always a bug.
- Always subtract chrome dimensions (header, footer, sidebar, padding) from `WindowSizeMsg` before passing to child views. Children must only know about their allocated space. Available Width = Terminal Width - Parent Horizontal Padding.

## Workflow Rules

- Reproduce bugs before planning fixes. Phase 0 (reproduction + failing test) is mandatory before any multi-phase fix plan. If bug cannot be reproduced, document as could-not-reproduce and close.

## Design Decisions (Do Not Re-Ask)

- Module: `github.com/mistakeknot/autarch`
- Shared TUI package with Tokyo Night colors
- Bubble Tea for all TUIs
- htmx + Tailwind for Bigend web
- SQLite for local state (read-only to external DBs)
- Local-only by default: servers bind to loopback; remote/multi-host deferred; non-loopback requires explicit opt-in + auth
- tmux integration via CLI commands
- Pollard tech hunters use free API tiers (no auth required)
- Pollard general-purpose hunters: some require API keys (USDA, CourtListener)
- Intermute for cross-tool coordination (REST + WebSocket + embedded in-process; first-class Spec, Insight, CUJ entities)
- Legacy tool names (Vauxhall/Praude/Tandemonium) still work via aliases
