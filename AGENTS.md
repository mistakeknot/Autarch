# Autarch - Development Guide

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
| Database | SQLite (WAL mode) |

## Documentation Map

| Document | Purpose |
|----------|---------|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System overview and data flow |
| [docs/INTEGRATION.md](docs/INTEGRATION.md) | Cross-tool + Intermute integration |
| [docs/COMPOUND_INTEGRATION.md](docs/COMPOUND_INTEGRATION.md) | Compound Engineering patterns |
| [docs/WORKFLOWS.md](docs/WORKFLOWS.md) | End-user task guides |
| [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) | Command cheat sheet |
| [docs/tui/SHORTCUTS.md](docs/tui/SHORTCUTS.md) | TUI keyboard shortcut conventions |
| [docs/plans/INDEX.md](docs/plans/INDEX.md) | Planning documents index |
| [docs/VISION.md](docs/VISION.md) | Strategic vision and coordination infrastructure |
| [docs/brainstorms/](docs/brainstorms/) | Design brainstorms |
| [docs/solutions/](docs/solutions/) | Solved problems by category (9 docs) - **check before debugging!** |

## Related Repositories

| Repo | Relationship | Location |
|------|--------------|----------|
| Intermute | Dependency (embedded server, domain API) | `/root/projects/Intermute` |

**Before starting Autarch work:**
```bash
# 1. Check Intermute for uncommitted changes
cd /root/projects/Intermute && git status

# 2. If changes exist, commit and push
git add -A && git commit -m "..." && git push

# 3. Update Autarch's go.mod to latest
cd /root/projects/Autarch
go get github.com/mistakeknot/intermute@latest
go mod tidy
```

## Project Status

### Done
- Monorepo structure with shared TUI package
- All four tools build and run
- Tokyo Night color palette standardized
- Pollard hunters (tech + general-purpose)
- Pollard report generation and API
- Intermute bridges for all tools
- Unified shell layout (Sidebar + ShellLayout) in `pkg/tui`
- 9 views migrated to Cursor-style 3-pane layout
- Gurgeh Arbiter subsystem (sprint state, consistency, confidence)
- Signal system: `pkg/signals/` + per-tool emitters (Pollard, Gurgeh, Coldwine)
- Spec evolution: versioned snapshots, assumption confidence decay
- Outcome hypotheses: falsifiable "if X then Y" per feature
- Structured requirements: Given/When/Then format
- Phase-specific deep research: Pollard targeted scans per Arbiter phase
- Competitor watch mode: `pollard watch [--once]`
- Agent-powered ranking: `gurgeh prioritize <spec-id>`
- Pollard HTTP API server (local-only)
- Gurgeh read-only Spec API (local-only)
- Signals WebSocket server + publish endpoint (local-only)
- Bigend colony detection (git worktrees + markers + /proc on Linux)
- Chat-focused TUI with Ctrl+ keybindings (no single-letter shortcuts)
- Slash commands with fuzzy finder (`/` opens picker)
- Deterministic tech stack detection with verbatim evidence
- **Unified TUI access:** `autarch tui` as single entry point with `--tool` flag for direct tab access
- **Inline mode:** `--inline` flag preserves terminal scrollback with log pane
- **Sprint persistence:** Auto-save and resume interrupted sprints (state saved to `.gurgeh/sprints/`)
- **Doc panel scrolling:** PgUp/PgDn scroll doc panel while chat is focused
- **Cached phase exploration:** All 8 arbiter phases cached in initial exploration for instant transitions

### In Progress
- Bigend TUI mode
- Intermute messaging (REST + WebSocket + embedded in-process; domain entities: Spec, Insight, CUJ, Epic, Session)
- **Unified TUI navigation** ([plan](docs/plans/2026-02-05-unified-tui-navigation-design.md)): 3-phase plan
  - Phase 1: Always-visible tabs (Bigend, Gurgeh, Coldwine, Pollard) + slash commands (`/big`, `/gur`, `/cold`, `/pol`) + `Ctrl+Left/Right` cycling
  - Phase 2: Gurgeh absorbs onboarding flow (large refactor)
  - Phase 3: Signals overlay (`Ctrl+Shift+S`)

### TODO
- Remote host support for Bigend (deferred; local-only default)
- Pollard integration into Bigend daemon
- Agent-powered ranking: wire real LLM agent call (currently uses goal-order placeholder)
- Intermute request/response pattern (async agent-to-Pollard queries)
- Bigend agent intelligence: agent reviews signals, surfaces suggestions/alerts/questions (needs separate design)

## Operating Principles

- Local-only by default: servers bind to loopback; remote/multi-host deferred; non-loopback requires explicit opt-in + auth

---

## Project Structure

```
Autarch/
├── cmd/                        # Entry points
│   ├── bigend/                # Mission control
│   ├── coldwine/              # Task orchestration
│   ├── gurgeh/                # PRD generation
│   └── pollard/               # Research CLI
├── internal/                   # Tool-specific code
│   ├── bigend/                # See docs/bigend/AGENTS.md
│   ├── coldwine/              # See docs/coldwine/AGENTS.md
│   ├── gurgeh/                # See docs/gurgeh/AGENTS.md
│   └── pollard/               # See docs/pollard/AGENTS.md
├── pkg/                        # Shared packages
│   ├── agenttargets/          # Run-target registry
│   ├── autarch/               # Unified client
│   ├── contract/              # Cross-tool types
│   ├── discovery/             # Project discovery
│   ├── events/                # Event spine
│   ├── intermute/             # Intermute client
│   ├── signals/               # Typed alerts (cross-tool)
│   └── tui/                   # Shared TUI styles
├── docs/                       # Documentation
└── dev                         # Build/run script
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for full directory structure.

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
./dev gurgeh export PRD-001          # Export spec
./dev coldwine status                # Task status
./dev bigend                         # Web server
./dev pollard scan                   # Run research

# Test
go test ./...
go test ./internal/<pkg> -v  # Specific package
```

**Note:** Standalone TUI modes (`./dev gurgeh`, `./dev coldwine`, `./dev bigend --tui`) are deprecated. Use `autarch tui --tool=X` instead.

### Plan Status Hook

The plan status report is the single source of truth and is regenerated on every commit via a repo pre-commit hook.

```bash
# Install repo git hooks (one-time)
./scripts/hooks/install-git-hooks.sh

# Generate manually if needed
./dev autarch plan-status --output docs/plans/STATUS.md
```

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

See tool-specific AGENTS.md files for tool configuration.

---

## TUI Keybindings

When adding or editing shortcuts, review
[docs/tui/SHORTCUTS.md](docs/tui/SHORTCUTS.md).

### Design Philosophy

**Chat-focused TUI:** All TUIs are built around a chat composer with a 50/50 split layout (context pane + chat pane). Keybindings use `Ctrl+` combinations to avoid conflicts with typing. Single-letter shortcuts are avoided. Function keys work as fallbacks for external keyboards.

**Chat-first editing:** There is no "edit mode" — users refine content by chatting with the agent ("make it more focused on developers") or by opening spec files directly in their editor. This keeps the TUI simple and conversation-centric.

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
| `Ctrl+X` | Delete (context-dependent) |
| `Ctrl+S` | Scan (in kickoff) |
| `PgUp` / `Ctrl+↑` | Scroll doc panel up (works from chat) |
| `PgDn` / `Ctrl+↓` | Scroll doc panel down (works from chat) |

### Slash Commands (Fuzzy Finder)

Type `/` in the chat composer to open a fuzzy command picker. Use `↑`/`↓` to navigate, `Tab` or `Enter` to select, `Esc` to dismiss.

**Global Commands:** `/help`, `/quit`, `/settings`, `/model`, `/palette`, `/refresh`, `/back`

**Tool Switching (planned — Phase 1):** `/bigend` (`/big`), `/gurgeh` (`/gur`), `/coldwine` (`/cold`), `/pollard` (`/pol`), `/signals` (`/sig`)

**View-Specific:** Commands like `/scan`, `/accept` vary by view. The picker shows only commands available in the current context.

See [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) for the complete command list.

---

## Shared Packages (pkg/)

| Package | Purpose |
|---------|---------|
| `contract` | Cross-tool entity types (Initiative, Epic, Story, Task, Run, Outcome) |
| `events` | Event spine for communication (SQLite at `~/.autarch/events.db`) |
| `intermute` | Intermute client wrapper (agents, messages, reservations) |
| `signals` | Typed alerts: competitor shipped, assumption decayed, execution drifted |
| `tui` | Shared TUI: styles, layouts (Shell, Split), ChatPanel, CommandPicker, Composer |
| `agenttargets` | Run-target registry/resolver |
| `discovery` | Project discovery |

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for details.

---

## Code Conventions

- Use `internal/` for tool-specific, `pkg/` for shared code
- Error handling: `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` with structured fields
- No external dependencies for core functionality
- SQLite: read-only connections to external DBs

### Testing
- TDD for behavior changes
- Small unit tests over broad integration tests
- Run targeted tests: `go test ./internal/<pkg> -v`

### End-to-End Data Flow Verification

When modifying data pipelines (especially exploration → artifacts → UI), verify:

1. **Data produced correctly at source** - Check logs or intermediate output
2. **Data transforms correctly at each step** - Inspect struct fields between transforms
3. **Data displays correctly in UI** - Actually run the TUI and look at the screen

**Common failure mode:** Tool execution succeeds but results don't propagate to UI. The pipeline completes without error, but the view shows default values ("Unknown Project").

### Integration Test Patterns

For TUI features that involve data pipelines:
- Test the full message flow, not just individual functions
- Verify tea.Msg reaches the view layer and triggers re-render
- For CLI → TUI flows, trace: subprocess output → parsing → state update → view render

Example verification for exploration flow:
```bash
# Run with --inline to see logs while UI renders
autarch tui --inline

# Check that exploration results appear in spec view, not "Unknown Project"
```

### TUI Design Principles

Chat is the primary input surface. Typing safety: no single-key shortcuts that fire during text entry. Discoverability via slash commands. Ctrl+ only for shortcuts. Minimal modes (avoid vim-style modal traps).

### Coding Standards (from Oracle Review)

- Use phase constants, not integer literals, for array indices.
- Focus-aware keyboard routing: switch on `shell.Focus()`, not `component.Focused()`.
- Error observability: non-fatal errors log to stderr with `warning:` prefix. Never silently swallow errors.

### TUI Input Patterns

- Shell layout owns focus state — use `shell.Focus()` not `component.Focused()` for routing decisions.
- Use `msg.String()` for key matching instead of `key.Matches()` for raw key events.
- Avoid Ctrl+J (= LF/Enter in terminals).

### Spec Phase Ordering

Canonical order: Vision > Problem > Users > Features > CUJs > Requirements > Scope > Acceptance. Ordering must reflect information dependencies — later phases consume earlier phase outputs.

### Debugging

**Before debugging, check solutions:**
```bash
# Browse by category
ls docs/solutions/

# Search by keyword in content
grep -r "race condition" docs/solutions/

# Search by YAML frontmatter tags
grep -l "tags:.*concurrency" docs/solutions/**/*.md

# Use learnings-researcher agent (in Claude Code)
# Automatically filters by tags, category, module, symptoms
```

The `docs/solutions/` directory contains documented solutions organized by category:

| Category | Purpose | Example Topics |
|----------|---------|----------------|
| `runtime-errors/` | Concurrency, panics, crashes | Pointer escape, race conditions |
| `ui-bugs/` | TUI rendering, message routing | ANSI splicing, swallowed messages |
| `patterns/` | Architecture, design decisions | Oracle reviews, sprint architecture |
| `workflow-issues/` | Process, methodology learnings | Over-planning, debugging approach |
| `integration/` | Cross-tool, API issues | Intermute, external services |

Each file has YAML frontmatter for searchability:
```yaml
---
title: "Descriptive title"
category: runtime-errors
tags: [concurrency, race-condition, deep-copy]
module: internal/gurgeh/arbiter
symptom: "Observable error or behavior"
root_cause: "Technical explanation"
date_resolved: "YYYY-MM-DD"
---
```

**After fixing a non-trivial bug**, run `/workflows:compound` to capture:
- Problem symptoms and error messages
- Investigation steps tried
- Root cause analysis
- The fix applied (with code examples)
- Prevention strategies

### Known Issues & Gotchas

**"Unknown Project" in Spec View:**
Exploration succeeds but results don't display. Root cause: `extractString()` can't find expected keys in the `map[string]any` returned by Claude Code. The keys depend on Claude's output format, which varies.

**Fix:** Check the actual keys in `exploreResult` before assuming field names. Log the raw result during debugging:
```go
slog.Debug("exploration result", "keys", maps.Keys(exploreResult))
```

### Key Data Flow: Exploration → Spec View

```
exploration.Explore(ctx, path)
  │
  ▼ returns map[string]any (raw JSON from Claude Code)
  │
exploration.MergeIntoArtifacts(exploreResult, nil)
  │
  ▼ returns *agent.PhaseArtifacts
  │
extractString(exploreResult, "project_name") // May fail if key missing!
extractVisionSummary(artifacts)
extractProblemSummary(artifacts)
extractUsersSummary(artifacts)
  │
  ▼ builds agent.ScanResult
  │
CodebaseScanResultMsg{Result: result}
  │
  ▼ sent via progressChan
  │
unified_app.Update() receives msg
  │
  ▼ updates state, transitions to spec view
  │
spec view renders PhaseArtifacts
```

**Breakpoints for debugging:**
- `internal/gurgeh/exploration/explore.go` - Raw Claude output
- `internal/gurgeh/exploration/merge.go` - Artifact construction
- `internal/tui/unified_app.go:scanCodebase()` - ScanResult building
- `internal/tui/unified_app.go:Update()` - Message handling

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

See [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) for complete list.

---

## Integration Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                           BIGEND                                 │
│                      (Mission Control)                           │
│         Observes all tools - READ ONLY aggregation              │
└───────────────────────────────┬─────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│    GURGEH     │      │   COLDWINE    │      │   POLLARD     │
│   (PRDs)      │─────▶│   (Tasks)     │◀─────│  (Research)   │
│               │      │               │      │               │
│ .gurgeh/specs │      │.coldwine/specs│      │ .pollard/     │
└───────────────┘      └───────────────┘      └───────────────┘
                                │
                         ┌──────┴──────┐
                         │  INTERMUTE  │
                         │(Coordination)│
                         └─────────────┘
```

**Key Integrations:**
- Gurgeh → Coldwine: PRDs generate tasks (via Briefs or direct import)
- Gurgeh → Pollard: Research enriches PRDs
- Coldwine → Pollard: Research informs implementation
- Bigend → All: Read-only aggregation
- Intermute: Cross-tool messaging and coordination

### Brief vs Task (Concept Clarification)

These are **intentionally separate** concepts with different purposes:

| Aspect | Gurgeh Brief | Coldwine Task |
|--------|--------------|---------------|
| **Purpose** | Agent instruction | Orchestration tracking |
| **Fields** | 3 (title, outcome, criteria) | 12+ (status, assignee, worktree, session...) |
| **Storage** | Markdown in `.gurgeh/briefs/` | SQLite in `.coldwine/` |
| **Lifecycle** | Stateless (create → execute → discard) | Stateful (pending → in_progress → completed) |
| **Consumer** | Claude Code (`claude -p "$(cat brief.md)"`) | Coldwine TUI/orchestrator |

**Mental model:**
> **Brief** = "What Claude Code should do" (instruction for agent execution)
> **Task** = "How to track what Claude Code is doing" (orchestration state)

The shared fields (title, outcome, criteria) form the **handoff contract** - they must match so Task can track Brief execution. This is intentional separation of concerns, not duplication.

**Integration flow:**
```
Gurgeh Spec → Decompose → Briefs (.md) → Import → Coldwine Tasks (SQLite)
```

See [docs/brainstorms/2026-02-04-briefs-vs-tasks-unification-brainstorm.md](docs/brainstorms/2026-02-04-briefs-vs-tasks-unification-brainstorm.md) for the design decision rationale.

See [docs/INTEGRATION.md](docs/INTEGRATION.md) for details.

---

## Arbiter Spec Sprint (NEW)

**Primary workflow for PRD creation:** Propose-first 8-phase flow with integrated research and confidence scoring.

### Quick Start

In Gurgeh TUI, start a new sprint to begin:

```
Phase 1: Vision
  ├─ Arbiter proposes project vision
  ├─ You accept or chat to refine ("focus more on...")
  └─ ✓ Quick Ranger scan triggers automatically

Phase 2: Problem
  ├─ Arbiter proposes problem statement
  └─ Consistency check (passes → continue)

Phase 3: Users
  ├─ Arbiter proposes user personas
  ├─ (Ranger scan results inform proposals)
  └─ You accept or chat to refine

Phase 4: Features + Goals
  ├─ Arbiter proposes feature list
  └─ You accept or chat to refine

Phase 5: Critical User Journeys
  ├─ CUJs flow naturally from users + features
  └─ You accept or chat to refine

Phase 6: Requirements
  ├─ Requirements derived from CUJs
  └─ Given/When/Then format

Phase 7: Scope + Assumptions
  ├─ Arbiter sets boundaries
  └─ You accept or chat to refine

Phase 8: Acceptance Criteria
  ├─ Arbiter generates AC for each CUJ
  └─ You finalize

▼ PRD Complete (Consistency checked, Confidence scored)

Handoff Options:
  ├─ Press R: Run full research (Pollard scan)
  ├─ Press T: Generate tasks (Coldwine)
  └─ Press E: Export (markdown/JSON)
```

### Workflow Details

**Consistency Engine:**
- Problem ↔ Users: Do users align with problem?
- Users ↔ Features: Do features address user needs?
- Features ↔ CUJs: Do CUJs demonstrate features?
- CUJs ↔ AC: Does AC validate each journey?
- AC ↔ Scope: Do AC respect boundaries?

**Confidence Scoring (0.0–1.0):**
- Clarity: Proposal text unambiguous
- Completeness: All required fields populated
- Coherence: Aligns with prior sections
- Feasibility: Technically achievable

Low-confidence proposals show warnings but don't block. Users can refine or accept.

**Quick Scan (Auto):**
- Triggers after Problem is accepted
- Ranger queries tech landscape + similar projects
- Results feed into Users/Features/Scope proposals

### Key Files

| Path | Purpose |
|------|---------|
| `internal/gurgeh/arbiter/orchestrator.go` | Sprint flow: Start → Advance → Accept → Revise → Handoff |
| `internal/gurgeh/arbiter/generator.go` | AI draft generation (propose-first) |
| `internal/gurgeh/arbiter/types.go` | Phase, SprintState, ConfidenceScore, Conflict types |
| `internal/gurgeh/arbiter/migrate.go` | Legacy Spec → SprintState migration |
| `internal/gurgeh/arbiter/consistency/` | Local adapter for cross-section validation |
| `internal/gurgeh/arbiter/confidence/` | Local adapter for 0.0-1.0 scoring |
| `internal/gurgeh/arbiter/intermute.go` | ResearchProvider interface + ResearchBridge (Intermute client wrapper) |
| `internal/gurgeh/arbiter/research_phases.go` | Phase → hunter mapping + query extractors |
| `internal/gurgeh/arbiter/deepscan.go` | Async deep scan handoff via Intermute messaging |
| `internal/gurgeh/specs/evolution.go` | Spec versioning: SaveRevision, LoadHistory, assumption decay |
| `internal/gurgeh/specs/diff.go` | Structured spec diff between versions |
| `internal/gurgeh/prioritize/ranker.go` | Agent-powered feature ranking |
| `internal/gurgeh/tui/sprint.go` | Bubble Tea sprint TUI view |

### Sprint Persistence

Sprints auto-save after each phase transition. Resume interrupted sprints:

```bash
# List in-progress sprints
ls .gurgeh/sprints/

# Resume via TUI (select from list)
./dev autarch tui --tool=gurgeh
# → Shows "Resume Sprint" option if in-progress sprints exist
```

State is saved to `.gurgeh/sprints/<sprint-id>.json` with all phase content, exploration cache, and current phase pointer.

### CLI Commands

```bash
# Start new sprint (TUI)
gurgeh sprint new

# From existing research
gurgeh sprint new --from-research insights.json

# Export completed PRD
gurgeh sprint export PRD-001 --format markdown
gurgeh sprint export PRD-001 --format json

# Spec evolution
gurgeh history <spec-id>              # Show spec revision changelog
gurgeh diff <spec-id> v1 v2           # Structured diff between versions

# Feature ranking
gurgeh prioritize <spec-id>           # Agent-powered ranked recommendations
```

### Typical Timing

20–40 minutes depending on domain complexity:
- Problem + Quick Scan: 7–15 min
- Users: 5–10 min
- Features+Goals: 3–5 min
- Scope+Assumptions: 2–3 min
- CUJs: 3–5 min
- Acceptance Criteria: 5–10 min

---

## Compound Engineering Integration

Autarch adopts patterns from the Compound Engineering Claude Code plugin:

### Multi-Agent Review

PRDs and research are validated by parallel review agents:

```bash
# PRD review with multi-agent validation
gurgeh review PRD-001 --gaps

# Reviewers: Completeness, CUJ Consistency, Acceptance Criteria, Scope Creep
```

### Knowledge Compounding

Solved problems are captured in `docs/solutions/` for future reference. Each solution compounds team knowledge—first occurrence takes research, subsequent occurrences take minutes.

**Current categories:** `runtime-errors/`, `ui-bugs/`, `patterns/`, `workflow-issues/`, `integration/`

```bash
# Before debugging - search existing solutions
grep -r "error message" docs/solutions/
grep -l "tags:.*bubble-tea" docs/solutions/**/*.md

# After fixing - capture the learning
/workflows:compound
```

**Key solutions documented:**
- Concurrency: State pointer escape, deep-copy patterns
- TUI: Message swallowing, ANSI-aware string operations, dimension mismatches
- Process: Reproduce before planning (over-engineering anti-pattern)
- Architecture: Oracle review findings, arbiter sprint patterns

### SpecFlow Gap Analysis

Detect specification gaps before implementation:

```go
analyzer := spec.NewSpecFlowAnalyzer()
result := analyzer.Analyze(spec)
// Gaps: missing_flow, unclear_criteria, edge_case, error_handling, etc.
```

### Claude Code Plugin

The `autarch-plugin/` directory provides Claude Code integration:

| Component | Purpose |
|-----------|---------|
| `/autarch:prd` | Create PRD (now uses Spec Sprint) |
| `/autarch:research` | Run Pollard research |
| `/autarch:tasks` | Generate epics from PRD |
| `/autarch:status` | Show project status |
| `autarch-mcp` | MCP server for AI agents |

See [docs/COMPOUND_INTEGRATION.md](docs/COMPOUND_INTEGRATION.md) for full details.

---

## Git Workflow

### Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: bigend, gurgeh, coldwine, pollard, tui, build
```

### Landing the Plane (Session Completion)

**MANDATORY WORKFLOW:**

1. File issues for remaining work
2. Run quality gates (if code changed)
3. Update issue status
4. **PUSH TO REMOTE** (mandatory):
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. Clean up stashes, prune branches
6. Verify all changes pushed
7. Hand off context for next session

**CRITICAL:** Work is NOT complete until `git push` succeeds.

---

## Tool-Specific Documentation

For detailed information about each tool, see:

| Tool | Developer Guide | Related |
|------|-----------------|---------|
| Bigend | [docs/bigend/AGENTS.md](docs/bigend/AGENTS.md) | [roadmap.md](docs/bigend/roadmap.md) |
| Gurgeh | [docs/gurgeh/AGENTS.md](docs/gurgeh/AGENTS.md) | |
| Coldwine | [docs/coldwine/AGENTS.md](docs/coldwine/AGENTS.md) | |
| Pollard | [docs/pollard/AGENTS.md](docs/pollard/AGENTS.md) | [HUNTERS.md](docs/pollard/HUNTERS.md), [API.md](docs/pollard/API.md) |

<!-- auracoil:begin -->
## Auracoil Review Notes

_This section is maintained by Auracoil (GPT-5.2 Pro reviewer). Do not edit manually._

### Review metadata
- **Last reviewed:** 2026-02-07
- **Reviewed commit:** d69fee4
- **Reviewer:** Auracoil (GPT-5.2 Pro)

### Scope
- Root AGENTS.md (719 lines) — full project overview, architecture, quick commands
- 38 source files provided as evidence (Go entrypoints, TS MCP server, Rust prototypes, docs, todos)

### Open verification items (from existing reviews)
- _Oracle review (2026-02-01) flagged potential drift/bugs around:_
  - Quick scan timing vs docs (phase trigger)
  - Targeted research wiring (`RunTargetedScan`) behavior
  - SprintView focus handling (Tab focus vs keystroke routing)
  - Review mapping consistency (Problem vs Users field mapping)

_Note: keep these as "verify current behavior" items (not asserted truths) until reconfirmed in code._
<!-- auracoil:end -->

## Key Architectural Facts
- **Single TUI implementation**: `UnifiedApp` is the only app shell (893 lines). `App` deleted in Phase 2b. <!-- intermem:42748734 -->
- **Onboarding lives in GurgehView**: `GurgehOnboardingView` (1075 lines) + `gurgeh_helpers.go` (366 lines) in `views/`. `GurgehView` is a container that delegates to onboarding or spec browser. <!-- intermem:357cb6a8 -->
- **Ctrl+1/2/3 freed up**: Removed from `internal/gurgeh/arbiter/tui/arbiter_view.go` (was alternative selection, redundant with `/1` `/2` `/3`) <!-- intermem:11d014d2 -->
- **Slash command aliases**: `/b`=back, `/p`=palette, `/g`=group, `/m`=model, `/r`=refresh, `/big`=bigend, `/gur`=gurgeh, `/cold`=coldwine, `/pol`=pollard, `/sig`=signals, `/logs`(`/log`,`/l`)=toggle log pane — check before adding new ones <!-- intermem:12b7bcd9 -->
- **Log pane always created**: `UnifiedApp` always creates `LogPane` + `LogHandler`. Ctrl+L toggles visibility. Auto-shows during scan, auto-hides after 3s. Bridge messages: `LogPaneAutoShowMsg`/`LogPaneScheduleAutoHideMsg` from GurgehOnboardingView → UnifiedApp. <!-- intermem:0d790779 -->
- **4 dashboard tabs**: Bigend(0), Gurgeh(1), Coldwine(2), Pollard(3) — Signals is an overlay (`/sig`), not a tab <!-- intermem:265f46d6 -->
- **Overlay render order in UnifiedApp**: palette → chat settings → signals → help. Each intercepts keys when visible. <!-- intermem:2b9bc692 -->

## Active Plans
- [unified-tui-navigation-design](docs/plans/2026-02-05-unified-tui-navigation-design.md): 3-phase plan for always-visible tabs + tool switching
  - Phase 1: DONE (commit 4c62720) — Tabs always visible + slash commands (`/big` `/gur` `/cold` `/pol`) + `Ctrl+Left/Right` cycling. Direct keybindings (Ctrl+N, Alt+N) dropped — unportable in BT v1 + tmux + macOS.
  - Phase 2: DONE — 3 sub-phases: 2a dead code cleanup (5 commits), 2b App/UnifiedApp merge (3 commits), 2c onboarding into Gurgeh (4 commits: f5e7a3d, c023fc0, 7b142be, bc77580). unified_app.go: 2200→893 lines.
  - Phase 3: DONE — SignalsOverlay component (1ae30ee) + wiring into UnifiedApp with `/sig` command (2301cd6). 2 Codex dispatches, both succeeded first try. Bead Autarch-n3e closed.
  - **All 3 phases complete.** unified-tui-navigation-design plan is fully implemented. <!-- intermem:5118d2dd -->

## Deferred Features
- **Bigend Agent Intelligence**: Agent reviews signals and surfaces suggestions/alerts/questions (needs separate design) <!-- intermem:fff2188f -->

## Interdoc Skill
- `interdoc` is a Claude Code skill/plugin, NOT a CLI tool <!-- intermem:d50130fb -->
- Invoked via `/interdoc:interdoc`, then follow the workflow manually <!-- intermem:70bce563 -->
- Don't try to run `interdoc` as a shell command <!-- intermem:6e91e10e -->

## Hook System Knowledge (see [hooks-learnings.md](hooks-learnings.md) for details)
- **`updatedInput` is REPLACE not merge** — must pass ALL original tool_input fields <!-- intermem:cc3eed0d -->
- **Bug #15897**: Multiple PreToolUse hooks cause `updatedInput` to be silently dropped (last hook wins) <!-- intermem:e67e725f -->
- **Conflicting plugins**: hookify + tool-time both have catch-all PreToolUse hooks that nuke `updatedInput` <!-- intermem:f32a64c2 -->
- **Subagents don't inherit CLAUDE.md** — instructions in parent CLAUDE.md don't reach Task subagents <!-- intermem:71c3c3f1 -->
- **Explore/Plan agents are read-only** — no Write tool; only general-purpose and specialized reviewers can write files <!-- intermem:7260e413 -->
- hookify disabled (`false` in settings.json) — it has no rules and its catch-all PreToolUse nukes `updatedInput` <!-- intermem:3d4bafe4 -->
- **Hooks are cached at session start** — changes to settings.json/hooks.json don't take effect until next session <!-- intermem:d3621dba -->
- **hook.sh IS re-read from disk each invocation** — only the hooks.json registration is cached, so editing hook.sh works mid-session <!-- intermem:d66c3045 -->
- **Plugin version resolution is unpredictable** — Claude Code may load 0.2.1 from cache instead of 0.3.0 or local; always sync ALL cached versions; removed 0.2.0 to reduce confusion <!-- intermem:6aad9bc9 -->
- **Local plugins need BOTH `localPlugins` AND `enabledPlugins`** — `localPlugins` tells Claude Code where the plugin directory is, but `enabledPlugins` controls whether it actually loads. Without the `enabledPlugins` entry, the plugin is invisible (no skills, no commands, no agents). For local-only plugins (not on a marketplace), use just the plugin name as the key: `"gurgeh-plugin": true` <!-- intermem:bf1a20b1 -->
- **`localPlugins` is undocumented and unreliable** — the CORRECT way to permanently install a local plugin is: (1) add it to a marketplace's `marketplace.json` with a git URL source, (2) `claude plugin marketplace update <name>`, (3) `claude plugin install plugin@marketplace`. This goes through the full pipeline: cache → installed_plugins.json → enabledPlugins. The `localPlugins` key + bare `enabledPlugins` name didn't work; marketplace install with `name@marketplace` format did. <!-- intermem:c61dc13d -->

## Interclode Plugin (Cross-AI Delegation)
- **Plugin**: `interclode@interagency-marketplace` — dispatch Codex agents from Claude Code <!-- intermem:26e85fd7 -->
- **Components**: `/interclode` command + `delegate` skill + `dispatch.sh` script <!-- intermem:85859490 -->
- **Codex CLI**: `codex exec -s workspace-write -C <dir> -o <output> "prompt"` with `run_in_background: true` <!-- intermem:d15b9d6e -->
- **v0.2.0**: `--inject-docs` auto-prepends CLAUDE.md/AGENTS.md; `--name` for template output paths; mandatory negative constraints in prompt template; single-task support <!-- intermem:a7e0dbaa -->
- **v0.2.1**: `--dry-run`, `--prompt-file`, `-i`/`--image` passthrough, `--inject-docs` default changed to claude-only (Codex reads AGENTS.md natively), retry/resume guidance, `--add-dir` docs, README.md <!-- intermem:e084c084 -->
- **v0.2.1+**: edge case fixes — `--` end-of-options, empty prompt file error, `{name}` without `--name` warning, no-docs-found note <!-- intermem:91b3f1ba -->
- **v0.2.2**: Step 0 fetches Codex CLI reference (developers.openai.com/codex/cli/reference/) at skill start; CLAUDE.md + AGENTS.md added; static CLI reference replaced with live fetch + fallback table <!-- intermem:03c1f8fb -->
- **v0.2.2+**: Live docs revealed `-a`/`--ask-for-approval` was a value flag missing from passthrough — fixed. Also made `--yolo`, `--search`, `--no-alt-screen` explicit. <!-- intermem:6025addf -->
- **Key insight**: Codex reads AGENTS.md natively from `-C` dir — `--inject-docs` only adds value for CLAUDE.md (Claude Code-specific instructions Codex wouldn't otherwise see) <!-- intermem:38132d36 -->
- **Key lesson**: Prompt quality is everything — include file paths, success criteria, and constraints <!-- intermem:3afe441a -->
- **GOCACHE fix**: Codex agents hit permission errors on `/root/.cache/go-build`; add `GOCACHE=/tmp/go-build-cache` to prompts <!-- intermem:51332d9c -->
- **Always verify independently**: Codex agents can report success while tests actually fail <!-- intermem:9df23e5e -->
- **Scope test commands**: Always use `-run TestPattern` or `-short` in test commands to avoid hanging integration tests (arbiter phase tests need live Claude CLI). Use `go test ./pkg/... -run TestFoo -v` not bare `go test ./... -v`. Codex agents stuck polling for hung tests will eat the full timeout. <!-- intermem:502a703f -->

## MCP Agent Mail
- **Server**: systemd service `mcp-agent-mail.service`, `http://127.0.0.1:8765/mcp/`, SQLite backend <!-- intermem:1fcdaf43 -->
- **MCP configured twice**: global `settings.json` (with auth token) AND Clavain `plugin.json` (no token) — both work, tools appear with both prefixes <!-- intermem:ba3bbdb7 -->
- **API works via plain HTTP POST**: `curl -s http://127.0.0.1:8765/mcp/ -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":"1","method":"tools/call","params":{"name":"...","arguments":{...}}}'` <!-- intermem:67a9e41f -->
- **Hook gracefully degrades**: 2s timeout on health check, silent exit 0 if Agent Mail unreachable <!-- intermem:c5268473 -->
- **Hooks are cached at session start** — editing hook.sh in Clavain source AND cache is needed for mid-version changes; new sessions pick up from cache automatically <!-- intermem:b6508f55 -->
- **All concurrent sessions must register** — the hook solves the bootstrap problem; without it, sessions work blind and can clobber each other's files <!-- intermem:9b14968d -->
- **File reservations are advisory** — they don't block edits, but report conflicts so agents can coordinate <!-- intermem:7a0c8b56 -->

## Flux Drive Skill
- **Moved from gurgeh-plugin to Clavain** — flux-drive is project-agnostic orchestration; T1 agents (`fd-*`) remain in gurgeh-plugin. Invoked as `/clavain:flux-drive`. Replaced `/clavain:deepen-plan` (the "fire 40+ agents" approach). <!-- intermem:5195187d -->
- **Generalized to any document or repo** — accepts file (plan, brainstorm, spec, ADR, README) or bare directory (repo review mode). LLM classifies on-the-fly, adapts review goal accordingly. <!-- intermem:caa67a30 -->
- **Repo review mode** — bare directory path → reads README + build files + key sources; writes summary.md to OUTPUT_DIR instead of modifying repo files. <!-- intermem:05ae2de0 -->
- **Always check codebase reality before profiling** — documents diverge from implementation. Step 1.0 in SKILL.md. <!-- intermem:08b9f9e6 -->
- **Front-load divergence context in agent prompts** — list actual file paths + line numbers so agents don't waste cycles on phantom code <!-- intermem:8087131f -->
- **Convergence tracking** — when N/M agents flag the same issue, that's high confidence signal. Include counts in issues checklist. <!-- intermem:9a56648d -->
- **T1 cross-project scoring** — domain expertise transfers (security, perf) but codebase knowledge doesn't. Score honestly, no tier bonus for wrong codebase. <!-- intermem:5314c8ee -->
- **3 test runs completed**: Autarch (Go/BT, same project), shadow-work (Rust/WASM, cross-project), Interforge (Tauri Rust+TS, cross-project with plan divergence) <!-- intermem:3ddde047 -->

## Lessons Learned
- Always check for keybinding conflicts before proposing new shortcuts — grep for the key combo first <!-- intermem:652cec23 -->
- Always check slash command alias collisions against `GlobalCommands()` in `pkg/tui/command_picker.go` <!-- intermem:b6a9d619 -->
- Estimate refactors by counting entangled state (struct fields, message types, handlers), not just "files to change" <!-- intermem:5fc0e631 -->
- **Ctrl+number keybindings don't work in Bubble Tea v1**: BT v1 doesn't negotiate the Kitty keyboard protocol, so terminals send bare digits for Ctrl+1-9. Use Alt+number instead (Alt prepends ESC byte, which BT v1 parses correctly). This applies even on Kitty-protocol terminals like Rio, Ghostty, WezTerm. <!-- intermem:4dc44f23 -->
- **Pre-existing test failures**: `docs/solutions` build failure (type assertion) and `TestCommandErrorWrapping` in coldwine CLI — not related to TUI changes <!-- intermem:8d68567a -->
- **`pipefail` + `grep -q`** = SIGPIPE trap. `grep -q` exits early → upstream `tail` gets SIGPIPE (141). With pipefail, pipeline fails. Fix: use `grep >/dev/null 2>&1` <!-- intermem:0f04480a -->
- **JSONL transcript lines are 10-100KB each** — use byte-based `tail -c` not line-based `tail -n` <!-- intermem:86b8f5c1 -->
- **lipgloss `Height()` is a floor, not a ceiling**: If content + padding exceeds `Height(n)`, the block silently expands. This breaks layout math — always verify `Height` matches actual content lines + padding. Caught this in onboarding header: `Height(3)` with `Padding(1,3)` + 2-line content (tabs+breadcrumb) → rendered 4 lines, causing 1-line overflow and terminal-default black leaking through. <!-- intermem:156c17c5 -->
- **Always test lipgloss layout math empirically**: Write a quick `go run` script that counts `strings.Count(rendered, "\n")+1` for each section and compares total to terminal height. Don't trust mental arithmetic with lipgloss Height/Padding interaction. <!-- intermem:a3c6bf52 -->
- **View height math must match unified_app's content padding**: Views using ShellLayout+SplitLayout (which pads to full height via `ensureSize`) MUST use `msg.Height - 4 - 2` (not just `-4`). The `-2` accounts for `contentStyle.Padding(1,3)` in unified_app.go. KickoffView and SprintView got this right; dashboard views (Gurgeh/Coldwine/Pollard/Bigend) didn't until ChatPanel made the overflow visible. <!-- intermem:f23469bd -->
- **Subagent output paths are relative to CWD, not input file**: When Task agents write files via `docs/research/...`, they resolve relative to the main session's CWD. Cross-project reviews write to the wrong project. Fix: derive OUTPUT_DIR from PROJECT_ROOT (nearest .git ancestor) and always resolve to absolute path. Applied in flux-drive SKILL.md. <!-- intermem:f67b4052 -->
- **Background agents overwrite same-named files silently**: Running flux-drive twice with overlapping agent names (e.g., fd-performance) overwrites the first run's output. Name output files with plan identifier or timestamp if preserving multiple runs. <!-- intermem:cd996f02 -->
- **Codex agents commit+push despite "Do NOT commit" in prompt**: All three agents in the 2026-02-06 interclode run committed and pushed. With `sandbox_mode=danger-full-access` and `approval_policy=never`, Codex ignores negative constraints about git ops. Check `git status` after dispatch — don't assume changes are unstaged. <!-- intermem:66623269 -->
- **Codex agents make unrelated cosmetic changes**: YAML security agent also refactored 4 dashboard view files with ShellLayout/ChatPanel additions — completely unrelated. Always `git diff --stat` before committing, revert unrelated files with `git checkout --`. <!-- intermem:db6667b9 -->
- **Arbiter phase tests are all integration tests**: Every test in `orchestrator_phase_test.go` calls `Advance()` → `GeneratePhase()` → `runClaude()`. They hang without a live Claude CLI. Only `TestConfidenceTotalWeightedCorrectly` and `TestConfidenceConflictsReduceConsistency` are true unit tests. Needs build tag isolation. <!-- intermem:485dbd5d -->
- **Scope Codex test commands to avoid hangs**: Always use `-run TestPattern` or `-short` in Codex prompts to avoid arbiter integration tests. E.g., `go test ./internal/tui/... -run TestGurgeh -v` not `go test ./internal/tui/... -v`. The 16v agent timed out (exit 144/SIGTERM) stuck polling for lingering `go test` processes that included hanging integration tests. <!-- intermem:2d9daa35 -->
- **`exec.CommandContext` kills process on context deadline**: Never pass a timeout context to `exec.CommandContext` for long-running servers — use `exec.Command` instead and manage lifecycle explicitly via `Process.Kill()`/`Process.Signal()`. The timeout context is fine for bounding a health-check poll, just not for the process itself. Found in Intermute manager: 30s startup timeout killed the server after 30s. <!-- intermem:c0f3a73f -->
- **Cross-agent convergence = high confidence**: When N/3 independent flux-drive agents flag the same finding, confidence scales with N. In schmux review, 3/3 agents independently flagged bracket signaling as #1 priority — that's much stronger than any single agent's rec. Always count convergence in synthesis docs. Single-agent findings should be labeled as such. <!-- intermem:05022373 -->
- **For "what to adopt" reviews, fd-architecture is the highest-value single agent**: It evaluates module boundaries and patterns, which is exactly what you need for cross-project inspiration. UX and agent-native add depth but take longer and need more source reading. If you can only run one agent, pick architecture. <!-- intermem:045a8a82 -->
- **Stale background agents from previous sessions still complete**: TaskOutput shows "running" for agents from ended sessions, but they continue executing and eventually complete. Check back later — don't re-launch duplicates. Agent outputs survive session boundaries. <!-- intermem:5daff73e -->

## PreToolUse Hook `updatedInput` Semantics
- **REPLACE, not merge**: `updatedInput` completely replaces `tool_input`. Must include ALL fields. <!-- intermem:071cef5a -->
- For Task tool: must include `prompt`, `description`, `subagent_type`, and optionally `model`, `max_turns`, `run_in_background`, `resume` <!-- intermem:2ca1cb77 -->
- Best pattern: capture full `ORIGINAL_INPUT=$(echo "$INPUT" | jq '.tool_input')` then `echo "$ORIGINAL_INPUT" | jq --arg prompt "$NEW_PROMPT" '. + {"prompt": $prompt}'` <!-- intermem:94744b22 -->

## Bug #15897: Multi-Hook `updatedInput` Aggregation
- When multiple PreToolUse hooks match the same tool, `updatedInput` from earlier hooks is overwritten by later hooks <!-- intermem:96b9aade -->
- Even a hook returning `{}` (no modification) will silently nuke a previous hook's `updatedInput` <!-- intermem:52e8e608 -->
- **Affected plugins on this server**:
  - `hookify` — catch-all PreToolUse (no matcher), returns `{}` for non-Bash/Edit tools
  - `tool-time` — `matcher: "*"`, logs events, returns nothing to stdout
  - `security-guidance` — `matcher: "Edit|Write|MultiEdit"`, only fires for file tools (not Task) <!-- intermem:9e6c2da6 -->
- **Workaround options**:
  1. Disable conflicting plugins during multi-agent workflows
  2. Merge all hook logic into a single hook script
  3. Wait for bug fix in Claude Code
  4. Use a different mechanism (modify skill prompts directly) <!-- intermem:2e434d9a -->

## `deepen-plan` Skill Agent Types
- `Task general-purpose` — most research agents (HAS Write) <!-- intermem:777d48da -->
- `Task Explore` — best-practices research (NO Write) <!-- intermem:9ccaabe3 -->
- `Task [agent-name]` — specialized reviewers (HAS Write) <!-- intermem:45520ec8 -->
- `plan_review` uses `@agent-*` reviewers — all have Write <!-- intermem:e7f9cbf7 -->

## Hook File Location
- hookify disabled in settings.json (`false`) — no rules configured, catch-all nukes `updatedInput` <!-- intermem:7a3c819e -->

## Plugin Version Resolution
- **Version selection is unpredictable**: In testing, CC loaded 0.2.1 instead of 0.3.0 or the local plugin <!-- intermem:3f6b6c52 -->
- **hook.sh is re-read from disk each invocation** (via `bash "$CLAUDE_PLUGIN_ROOT/hooks/hook.sh"`) — editing cached hook.sh takes effect mid-session <!-- intermem:79a7eb1c -->
- **settings.json / hooks.json changes are cached at session start** — require restart to take effect <!-- intermem:0b39319e -->
- **Workaround for cached versions**: sync hook.sh to ALL cached versions when making changes <!-- intermem:31a68ba9 -->
- **Diagnostic trick**: Add `echo "$0 $CLAUDE_PLUGIN_ROOT" >> /tmp/hook-identity.log` to identify which version is running <!-- intermem:f8ab17ca -->

## Research Results from Dead Session
- Saved to `docs/research/acceptance-criteria-2026-02-05/` (16 files, 344K) <!-- intermem:9bd03567 -->
- Extracted from session `b817bfb0` JSONL using Python parser <!-- intermem:5ab8853a -->
- Pattern: `<result>...</result>` inside `<task-notification>` blocks in user-type messages <!-- intermem:1935b7e2 -->
