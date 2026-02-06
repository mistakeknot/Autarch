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
