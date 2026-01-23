# Vauxhall vs Agent Deck Parity Audit (TUI + Web)

**Date:** 2026-01-22
**Scope:** TUI + Web parity audit against Agent Deck (docs + quick-run attempt)

## Sources
- Agent Deck marketing site and README (features, shortcuts, CLI, config)
- Agent Deck README sections for CLI, groups, MCP, worktrees, status, notifications, updates

## Method & Constraints
- Attempted a “quick run” locally, but direct network installs/clones are blocked in this environment. This audit is based on docs + README only. (No local runtime validation.)

## High-Level Summary
Agent Deck emphasizes fast multi-session management in a single TUI: fuzzy search with status filters, hierarchical groups, MCP toggles with auto-restart + socket pooling, tmux status-bar notifications for waiting sessions, and a full CLI (sessions/groups/worktrees/status).

Vauxhall already exceeds Agent Deck on cross-tool aggregation (Praude/Tandemonium/MCP Agent Mail) and has a web UI, but is missing several of Agent Deck’s workflow accelerators and automation surfaces.

## Parity Gaps (Priority Order)
1. **Search + status filters (TUI + Web)** — Agent Deck’s fuzzy search + status tokens are core to navigating many sessions fast.
2. **Hierarchical grouping of sessions/projects** — Agent Deck supports groups + subgroups and move/delete semantics.
3. **MCP manager parity** — Agent Deck supports global/local scope, auto-restart, and socket pooling to reduce MCP process load.
4. **tmux status-bar notifications** — Agent Deck surfaces waiting sessions with hotkeys directly in tmux.
5. **CLI coverage** — Agent Deck exposes session, MCP, group, status, worktree, and try commands with JSON output.

## Detailed Parity Matrix

### TUI
- **Session list + status detection:** Partial parity. Vauxhall lists sessions and detects status, but lacks Agent Deck’s search/status filters and notification bar.
- **Session actions:** Partial parity. Vauxhall supports new/rename/fork/restart/attach, but lacks delete, group move, and CLI-driven flows.
- **Groups / hierarchy:** Missing. Agent Deck supports nested groups and group management commands.
- **MCP manager:** Partial parity. Vauxhall has per-project toggles; Agent Deck adds global scope, auto-restart, and pooled MCPs for memory efficiency.
- **Search + filters:** Missing. Agent Deck offers fuzzy search and status filters via symbols.
- **Waiting-session notifications:** Missing. Agent Deck integrates with tmux status bar with hotkey mapping.

### Web
Agent Deck does not emphasize a web UI; parity here is “surface the same operational accelerators in a web context.” That implies:
- Global search + status filters
- Grouped project/session hierarchy
- MCP manager with global/local scope + pool indicators
- Status widgets for “waiting sessions” and quick navigation

### CLI
- **Session CLI:** Missing. Agent Deck has start/stop/restart/attach/show/current, plus fork with title/group flags.
- **MCP CLI:** Missing. Agent Deck has list/attach/detach with global + restart flags.
- **Group CLI:** Missing. Agent Deck has group list/create/delete/move with parent/force flags.
- **Worktree CLI:** Missing. Agent Deck integrates worktrees with add/list/info/cleanup.
- **Status CLI:** Missing. Agent Deck provides compact/verbose/JSON status output.
- **Try/experiments CLI:** Missing. Agent Deck supports quick “try” sessions with configurable location.

## Vauxhall Advantages (Non-Parity)
- Cross-tool aggregation: Praude PRDs + Tandemonium tasks + MCP Agent Mail threads.
- Web UI and dashboard beyond TUI workflows.
- Project discovery based on repo structure (.praude/.tandemonium) rather than manual session grouping.

## Recommended Next Work (Parity-Oriented)
1. **TUI search + status filter tokens** (mirror `/`, `!/@/#/$` filtering).
2. **Session grouping hierarchy** (projects list becomes groups + projects + sessions).
3. **MCP manager v2** (global/local scope, auto-restart toggle, pooled MCP indicator + config).
4. **tmux status-bar notifications** for waiting sessions with hotkey jumps.
5. **CLI surface** (session/mcp/group/worktree/status/try with JSON output).

## Open Questions
- Do we want group hierarchy to be project-centric (projects as leaves) or session-centric (sessions as leaves with optional project grouping)?
- How should MCP “global” be defined in Vauxhall (per-user config vs. global config per project root)?
- Should worktree management live in Vauxhall or stay in Tandemonium?

