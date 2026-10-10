# Autarch - Development Guide

## Canonical References
1. [`PHILOSOPHY.md`](../../PHILOSOPHY.md) — direction for ideation and planning decisions.
2. `CLAUDE.md` — implementation details, architecture, testing, and release workflow.

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
| Language | Go 1.25+ |
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

## autarch serve

`autarch serve` (or `./dev serve`) is the one consolidated, read-only, loopback service. It
replaces running the standalone Bigend, Gurgeh and Signals servers by hand; those still work and
print a deprecation notice.

| Item | Value |
|------|-------|
| Default address | `127.0.0.1:8110` (`--addr`; non-loopback is refused) |
| `/bigend/` | Bigend daemon API |
| `/signals/` | One in-process Signals broker (`/signals/ws`); Gurgeh spec updates publish into it |
| `/gurgeh/{project}/` | Gurgeh API for one project; unknown or non-`.gurgeh` project is 404 |
| `GET /api/projects` | Resolved project roots as `[{name, root, dev, ino}]`; the only route `serve` adds |
| `GET /health` | No token; status, project count, build revision and executable SHA-256 |

- **Projects:** `--project-dir` (repeatable) names scan roots whose child directories are the
  projects; the default is Bigend's discovery roots. Roots are symlink-resolved and must stay
  inside a scan root; a shared base name is an ambiguity error. The list is re-read at most every 30 s.
- **Token:** every route except `/health` needs `Authorization: Bearer <token>`. The token lives in
  `--token-file` (default `~/.autarch/serve.token`, created mode 0600; a looser mode is refused).
  There is no query-string token, and the startup line prints the file path, never the token.
- **Origin allowlist:** a request with an `Origin` header is 403 unless it is listed with
  `--allow-origin` (repeatable; default none). A `Host` that is not loopback is also 403.
- **No decisions:** `serve` holds no decisions code and no `/api/decisions` route.
- **Pollard:** Pollard's watcher still publishes to the standalone Signals server on 8092
  (`internal/signals/cli/serve.go`, not deprecated); it does not reach the `serve` broker yet.
- **Mycroft:** needs `autarch serve` running (loopback, token in `~/.autarch/serve.token`;
  override with `AUTARCH_SERVE_URL` / `AUTARCH_SERVE_TOKEN_FILE`) to file asks into Home. It asks
  serve for project roots and files nothing (fails closed) when serve is down.
- **MCP:** `autarch mcp --project <dir>` runs the MCP server; `autarch-mcp` is an alias.

## Home asks

Agents ask mk by filing a `needs-mk` card in tasks with `autarch needs-mk file`; `bb home ask` is
retired (exits 2, "moved"). `bb home get`, `list`, `feed` and `stats` read; `bb home progress`,
`resolve` and `withdraw` are for asks filed before cards and refuse a card. The bb plugin never
starts `autarch serve`; run it yourself.

A card can carry what mk owes: `autarch needs-mk file --move move.json` (a home-move/v1 file: kind script, pr,
read or context; `--ask-file` may be left out) labels it `mk-move`, and `autarch needs-mk adopt-move --card ID
--move move.json` attaches a move to a card this thread already filed. Both validate with the parser Home uses;
nothing in the file is executed.

`autarch needs-mk footer` prints what is open for mk, read-only from Home's `data.db`: To decide, Parked (Later),
new picks since this caller's cursor, and undelivered obligations. The cursor is per caller (`--caller`, default
`$BB_THREAD_ID`) under `$XDG_STATE_HOME/autarch/needs-mk-footer/`; `--peek` leaves it alone; `--db` or
`AUTARCH_HOME_DB` overrides the database.

Each decision pick also appends one line to the vizier rulings ledger (`~/.local/state/vizier/rulings.jsonl`, or
`AUTARCH_RULINGS_LEDGER`; `off` disables it). The entry id is `home-<pick_id>`, so it never matches a hand-logged
`q<number>`, and a retried pick finds its line instead of adding a second. Picks made before this shipped are not
backfilled.

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
