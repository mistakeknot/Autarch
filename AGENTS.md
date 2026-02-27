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
| [docs/solutions/](docs/solutions/) | Solved problems by category — **check before debugging!** |

### Topic References

| Document | Purpose |
|----------|---------|
| [docs/reference/compound-engineering.md](docs/reference/compound-engineering.md) | Multi-agent review, knowledge compounding, SpecFlow analysis |
| [docs/reference/claude-code-ecosystem.md](docs/reference/claude-code-ecosystem.md) | Hooks, plugins, MCP Agent Mail, flux-drive, agent types |
| [docs/reference/lessons-learned.md](docs/reference/lessons-learned.md) | TUI/Bubble Tea, Go patterns, agent coordination, testing gotchas |

---

## Architecture

### Layer Model (L1/L2/L3)

Autarch follows Demarch's 3-layer architecture:

```
L3: Autarch (apps)     — TUI views, user interaction, dashboard
     ↓ calls
L2: Clavain (os)       — Policy layer, sprint lifecycle, gates, phase rules
     ↓ calls
L1: Intercore (core)   — Kernel, state machine, runs, coordination locks
```

**Intent routing**: Policy-governing mutations (sprint create, advance, gate enforcement) go through L2 (`pkg/clavain` → `clavain-cli`). Reads and metadata writes can call L1 directly (`pkg/intercore` → `ic`).

**Graceful degradation**: When `clavain-cli` is not on `PATH`, `clavain.New()` returns `ErrUnavailable`. All callsites check `if cclient != nil` and fall back to direct `ic` calls.

### Project Structure

```
Autarch/
├── cmd/                        # Entry points: autarch, autarch-mcp, bigend, coldwine, gurgeh, pollard, signals
├── internal/                   # Tool-specific code (bigend/, coldwine/, gurgeh/, pollard/, tui/)
├── pkg/                        # Shared packages (see `ls pkg/` — key ones below)
├── docs/                       # Per-tool docs at docs/{bigend,gurgeh,coldwine,pollard}/AGENTS.md
└── dev                         # Build/run script
```

**Key shared packages (pkg/):**
- `autarch` — unified client (HTTP→Intermute), `clavain` — L2 CLI wrapper, `intercore` — L1 CLI wrapper
- `tui` — shared TUI (styles, Shell/Split layouts, ChatPanel, CommandPicker, Composer)
- `db` — SQLite helpers (WAL, `MaxOpenConns(1)`, 5s busy timeout), `contract` — cross-tool entity types
- Full listing: `ls pkg/` (23 packages)

### Key Architectural Facts

- **Single TUI implementation**: `UnifiedApp` is the only app shell. All views render inside it.
- **Onboarding lives in GurgehView**: `GurgehOnboardingView` + `gurgeh_helpers.go` in `views/`. `GurgehView` delegates to onboarding or spec browser.
- **4 dashboard tabs**: Bigend(0), Gurgeh(1), Coldwine(2), Pollard(3). Signals is an overlay (`/sig`), not a tab.
- **Overlay render order**: palette → chat settings → signals → help. Each intercepts keys when visible.
- **Log pane**: Always created. Ctrl+L toggles. Auto-shows during scan, auto-hides after 3s.
- **Slash command aliases**: `/b`=back, `/p`=palette, `/g`=group, `/m`=model, `/r`=refresh, `/big`=bigend, `/gur`=gurgeh, `/cold`=coldwine, `/pol`=pollard, `/sig`=signals, `/logs`(`/log`,`/l`)=toggle log pane — check `GlobalCommands()` in `pkg/tui/command_picker.go` before adding new ones.

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

# Unified TUI (recommended)
./dev autarch tui                    # Full onboarding flow
./dev autarch tui --skip-onboard     # Direct to dashboard
./dev autarch tui --tool=gurgeh      # Jump to specific tab
./dev autarch tui --inline           # Inline mode (preserves scrollback)

# Standalone CLI (no TUI)
./dev gurgeh list                    # List specs
./dev coldwine status                # Task status
./dev pollard scan                   # Run research

# Test
go test ./...
go test ./internal/<pkg> -v  # Specific package
```

**Note:** Standalone TUI modes (`./dev gurgeh`, `./dev coldwine`) are deprecated. Use `autarch tui --tool=X` instead.

### Configuration

**Shared agent targets** (global + per-project overrides):
- Global: `~/.config/autarch/agents.toml`
- Project: `.gurgeh/agents.toml`

```toml
[targets.claude]
command = "claude"
args = []

[targets.codex]
command = "codex"
args = []
```

---

## Code Conventions

- Use `internal/` for tool-specific, `pkg/` for shared code
- Error handling: `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` with structured fields
- No external dependencies for core functionality
- SQLite: read-only connections to external DBs
- Run tests with `-race` flag

### Testing
- TDD for behavior changes
- Small unit tests over broad integration tests
- Run targeted tests: `go test ./internal/<pkg> -v`

### Concurrency Rules

- Never return pointers to internal mutable state from synchronized methods. `State()` returns deep-copied snapshots via `Clone()`.
- All types crossing goroutine boundaries need `Clone()` methods.
- Bubble Tea threading model: `Update()` and `View()` always run on the same goroutine — pointer fields shared across Model value copies are safe without mutexes.

### TUI Design Principles

**Chat-focused TUI:** All TUIs are built around a chat composer with a 50/50 split layout. Keybindings use `Ctrl+` combinations — no single-letter shortcuts during text entry. Discoverability via slash commands.

**Chat-first editing:** No "edit mode" — users refine content by chatting with the agent. This keeps the TUI conversation-centric.

- Shell layout owns focus state — use `shell.Focus()` not `component.Focused()` for routing decisions
- Use `msg.String()` for key matching instead of `key.Matches()` for raw key events
- Avoid Ctrl+J (= LF/Enter in terminals)
- Use phase constants, not integer literals, for array indices
- Non-fatal errors log to stderr with `warning:` prefix; never silently swallow errors

### Debugging

**Before debugging, check solutions:**
```bash
ls docs/solutions/
grep -r "error message" docs/solutions/
grep -l "tags:.*bubble-tea" docs/solutions/**/*.md
```

| Category | Purpose |
|----------|---------|
| `runtime-errors/` | Concurrency, panics, crashes |
| `ui-bugs/` | TUI rendering, message routing |
| `patterns/` | Architecture, design decisions |
| `workflow-issues/` | Process, methodology learnings |
| `integration/` | Cross-tool, API issues |

**After fixing a non-trivial bug**, run `/clavain:compound` to capture the solution.

---

## Environment Variables

| Variable | Tool | Purpose |
|----------|------|---------|
| `VAUXHALL_PORT` | Bigend | Web port (default: 8099) |
| `VAUXHALL_SCAN_ROOTS` | Bigend | Project scan paths |
| `INTERMUTE_URL` | All | Intermute server URL |
| `INTERMUTE_API_KEY` | All | Intermute authentication |
| `INTERMUTE_PROJECT` | All | Project scope |
| `GITHUB_TOKEN` | Pollard | GitHub API (optional) |
| `USDA_API_KEY` | Pollard | USDA hunter (required) |
| `COURTLISTENER_API_KEY` | Pollard | Legal hunter (required) |

---

## Integration Overview

**Key Integrations:**
- Gurgeh → Coldwine: PRDs generate tasks (via Briefs or direct import)
- Gurgeh → Pollard: Research enriches PRDs
- Coldwine → Pollard: Research informs implementation
- Coldwine → Clavain/Intercore: Sprint lifecycle, phase gates, dispatch tracking
- Bigend → All: Read-only aggregation
- Intermute: Cross-tool messaging and coordination

### Brief vs Task (Concept Clarification)

| Aspect | Gurgeh Brief | Coldwine Task |
|--------|--------------|---------------|
| **Purpose** | Agent instruction | Orchestration tracking |
| **Fields** | 3 (title, outcome, criteria) | 12+ (status, assignee, worktree, session...) |
| **Storage** | Markdown in `.gurgeh/briefs/` | SQLite in `.coldwine/` |
| **Lifecycle** | Stateless (create → execute → discard) | Stateful (pending → in_progress → completed) |

**Mental model:**
> **Brief** = "What Claude Code should do" (instruction)
> **Task** = "How to track what Claude Code is doing" (orchestration state)

---

## Arbiter Spec Sprint

**Primary workflow for PRD creation:** Propose-first 8-phase flow with integrated research and confidence scoring.

### Phase Flow

```
Phase 1: Vision        → Arbiter proposes project vision
Phase 2: Problem       → Problem statement + consistency check
Phase 3: Users         → Personas (informed by Ranger scan)
Phase 4: Features      → Feature list + goals
Phase 5: CUJs          → Critical User Journeys from users + features
Phase 6: Requirements  → Given/When/Then format
Phase 7: Scope         → Boundaries + assumptions
Phase 8: Acceptance    → Criteria for each CUJ

▼ PRD Complete → Handoff: Research (R), Tasks (T), Export (E)
```

**Consistency Engine:** Validates cross-section alignment (Problem↔Users, Users↔Features, Features↔CUJs, CUJs↔AC, AC↔Scope).

**Confidence Scoring (0.0–1.0):** Clarity, Completeness, Coherence, Feasibility. Low-confidence proposals show warnings but don't block.

### Key Files

| Path | Purpose |
|------|---------|
| `internal/gurgeh/arbiter/orchestrator.go` | Sprint flow: Start → Advance → Accept → Revise → Handoff |
| `internal/gurgeh/arbiter/generator.go` | AI draft generation (propose-first) |
| `internal/gurgeh/arbiter/types.go` | Phase, SprintState, ConfidenceScore, Conflict types |
| `internal/gurgeh/arbiter/consistency/` | Cross-section validation |
| `internal/gurgeh/arbiter/confidence/` | 0.0-1.0 scoring |
| `internal/gurgeh/arbiter/research_phases.go` | Phase → hunter mapping + query extractors |
| `internal/gurgeh/specs/evolution.go` | Spec versioning, assumption decay |

### Sprint Persistence

Sprints auto-save to `.gurgeh/sprints/<sprint-id>.json` with all phase content, exploration cache, and current phase pointer. Resume via `./dev autarch tui --tool=gurgeh`.

---

## TUI Keybindings

See [docs/tui/SHORTCUTS.md](docs/tui/SHORTCUTS.md) for full reference.

### Universal Keys

| Key | Action |
|-----|--------|
| `Ctrl+C` | Clear input (once) / Quit (twice) |
| `↓` / `↑` | Move down / up |
| `Enter` | Select / confirm |
| `Esc` | Cancel / go back |
| `Tab` | Cycle pane focus |
| `Ctrl+B` | Toggle sidebar |
| `Ctrl+G` | Model selector |
| `Ctrl+R` | Refresh |
| `Ctrl+A` | Accept (context-dependent) |
| `PgUp/PgDn` | Scroll doc panel (works from chat) |

### Slash Commands

Type `/` to open fuzzy command picker. Global: `/help`, `/quit`, `/settings`, `/model`, `/palette`, `/refresh`, `/back`. Tool switching: `/bigend`, `/gurgeh`, `/coldwine`, `/pollard`, `/signals`.

---

## Git Workflow

### Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: bigend, gurgeh, coldwine, pollard, tui, build
```

### Session Completion

1. Run quality gates (if code changed)
2. **Push to remote** (mandatory): `git pull --rebase && bd sync && git push`
3. Verify `git status` shows "up to date with origin"

---

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
