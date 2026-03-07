# Autarch - Development Guide

## Canonical References
1. [`PHILOSOPHY.md`](../../PHILOSOPHY.md) — direction for ideation and planning decisions.
2. `CLAUDE.md` — implementation details, architecture, testing, and release workflow.

## Philosophy Alignment Protocol
Review [`PHILOSOPHY.md`](../../PHILOSOPHY.md) during:
- Intake/scoping
- Brainstorming
- Planning
- Execution kickoff
- Review/gates
- Handoff/retrospective

For brainstorming/planning outputs, add two short lines:
- **Alignment:** one sentence on how the proposal supports the module's purpose within Demarch's philosophy.
- **Conflict/Risk:** one sentence on any tension with philosophy (or 'none').

If a high-value change conflicts with philosophy, either:
- adjust the plan to align, or
- create follow-up work to update `PHILOSOPHY.md` explicitly.


Unified monorepo for AI agent development tools: Bigend, Gurgeh, Coldwine, and Pollard.

## Quick Reference

| Tool | Purpose | CLI | Docs |
|------|---------|-----|------|
| **Autarch** | Unified TUI (all tools in tabs) | `./dev autarch tui` | This file |
| **Bigend** | Multi-project agent mission control | `./dev bigend` (web) | [docs/bigend/](docs/bigend/AGENTS.md) |
| **Gurgeh** | PRD generation and validation | `./dev gurgeh list` (CLI) | [docs/gurgeh/](docs/gurgeh/AGENTS.md) |
| **Coldwine** | Task orchestration | `./dev coldwine status` (CLI) | [docs/coldwine/](docs/coldwine/AGENTS.md) |
| **Pollard** | Research intelligence (hunters + reports) | `./dev pollard scan` (CLI) | [docs/pollard/](docs/pollard/AGENTS.md) |

**Recommended:** Use `./dev autarch tui` for all TUI access. Standalone TUI modes are deprecated.

| Item | Value |
|------|-------|
| Language | Go 1.24+ |
| Module | `github.com/mistakeknot/autarch` |
| TUI Framework | Bubble Tea + lipgloss |
| Web Framework | net/http + htmx + Tailwind |
| Database | SQLite (WAL mode, pure Go via `modernc.org/sqlite`) |

## Topic Guides

| Topic | File | Covers |
|-------|------|--------|
| Architecture | [agents/architecture.md](agents/architecture.md) | Layer model, project structure, key architectural facts |
| Development | [agents/development.md](agents/development.md) | Prerequisites, build & run, configuration |
| Conventions | [agents/conventions.md](agents/conventions.md) | Code style, testing, concurrency, TUI design, debugging |
| Environment | [agents/environment.md](agents/environment.md) | Environment variables by tool |
| Integration | [agents/integration.md](agents/integration.md) | Cross-tool integration, brief vs task |
| Arbiter Sprint | [agents/arbiter-sprint.md](agents/arbiter-sprint.md) | 8-phase PRD flow, consistency engine, confidence scoring |
| TUI Keybindings | [agents/tui-keybindings.md](agents/tui-keybindings.md) | Universal keys, slash commands |
| Git Workflow | [agents/git-workflow.md](agents/git-workflow.md) | Commit messages, session completion |

## Documentation Map

| Document | Purpose |
|----------|---------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System overview and data flow |
| [docs/INTEGRATION.md](docs/INTEGRATION.md) | Cross-tool + Intermute integration |
| [docs/WORKFLOWS.md](docs/WORKFLOWS.md) | End-user task guides |
| [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) | Command cheat sheet |
| [docs/tui/SHORTCUTS.md](docs/tui/SHORTCUTS.md) | TUI keyboard shortcut conventions |
| [docs/plans/INDEX.md](docs/plans/INDEX.md) | Planning documents index |
| [docs/VISION.md](docs/VISION.md) | Strategic vision and coordination infrastructure |
| [docs/solutions/](docs/solutions/) | Solved problems by category -- **check before debugging!** |

### Topic References

| Document | Purpose |
|----------|---------|
| [docs/reference/compound-engineering.md](docs/reference/compound-engineering.md) | Multi-agent review, knowledge compounding, SpecFlow analysis |
| [docs/reference/claude-code-ecosystem.md](docs/reference/claude-code-ecosystem.md) | Hooks, plugins, MCP Agent Mail, flux-drive, agent types |
| [docs/reference/lessons-learned.md](docs/reference/lessons-learned.md) | TUI/Bubble Tea, Go patterns, agent coordination, testing gotchas |

## Tool-Specific Documentation

| Tool | Developer Guide | Related |
|------|-----------------|---------|
| Bigend | [docs/bigend/AGENTS.md](docs/bigend/AGENTS.md) | [roadmap.md](docs/bigend/roadmap.md) |
| Gurgeh | [docs/gurgeh/AGENTS.md](docs/gurgeh/AGENTS.md) | |
| Coldwine | [docs/coldwine/AGENTS.md](docs/coldwine/AGENTS.md) | |
| Pollard | [docs/pollard/AGENTS.md](docs/pollard/AGENTS.md) | [HUNTERS.md](docs/pollard/HUNTERS.md), [API.md](docs/pollard/API.md) |

## Design Decisions (Do Not Re-Ask)

- Module: `github.com/mistakeknot/autarch`
- Shared TUI package with Tokyo Night colors
- Bubble Tea for all TUIs; htmx + Tailwind for Bigend web
- SQLite for local state (read-only to external DBs)
- Local-only by default: servers bind to loopback; remote deferred
- tmux integration via CLI commands
- Pollard tech hunters use free API tiers (no auth required)
- Intermute for cross-tool coordination
- L2 (Clavain) for policy-governing sprint mutations; L1 (Intercore) for kernel state
- Legacy tool names (Vauxhall/Praude/Tandemonium) still work via aliases
