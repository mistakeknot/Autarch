# Autarch - Development Guide

Unified monorepo for AI agent development tools: Bigend, Gurgeh, Coldwine, and Pollard.

## Quick Reference

| Tool | Purpose | Entry Point |
|------|---------|-------------|
| **Bigend** | Multi-project agent mission control (web + TUI) | `./dev bigend` |
| **Gurgeh** | TUI-first PRD generation and validation | `./dev gurgeh` |
| **Coldwine** | Task orchestration for human-AI collaboration | `./dev coldwine` |
| **Pollard** | Continuous research intelligence (hunters + reports) | `go run ./cmd/pollard` |

| Item | Value |
|------|-------|
| Language | Go 1.24+ |
| Module | `github.com/mistakeknot/autarch` |
| TUI Framework | Bubble Tea + lipgloss |
| Web Framework | net/http + htmx + Tailwind |
| Database | SQLite (WAL mode) |

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

**Key integration points:**
- `pkg/autarch/` - Client for Intermute domain API
- `internal/embedded/` - Wrapper for embedded Intermute server
- Domain types (Spec, Epic, Story, Task, Insight, Session) defined in Intermute

## Project Status

### Done
- Monorepo structure with shared TUI package
- All four tools build and run
- Tokyo Night color palette standardized
- Pollard hunters implemented (GitHub, HackerNews, arXiv, Competitor)
- Pollard general-purpose hunters (OpenAlex, PubMed, USDA, Legal, Economics, Wiki)
- Pollard report generation (landscape, competitive, trends, research)
- Pollard API for Gurgeh/Coldwine integration
- Pollard GetInsightsForFeature/GenerateResearchBrief for agent context

### In Progress
- Bigend TUI mode
- Intermute messaging (file-based, transitioning to HTTP)

### TODO
- Migrate TUI components to use shared `pkg/tui`
- Remote host support for Bigend
- Cross-tool coordination features
- Pollard integration into Bigend daemon

---

## Project Structure

```
Autarch/
├── cmd/
│   ├── bigend/           # Bigend entry point
│   ├── gurgeh/             # Gurgeh entry point
│   ├── coldwine/        # Coldwine entry point
│   └── pollard/            # Pollard entry point
├── internal/
│   ├── bigend/           # Bigend-specific code
│   │   ├── aggregator/     # Data aggregation
│   │   ├── agentmail/      # MCP Agent Mail integration
│   │   ├── claude/         # Claude session detection
│   │   ├── config/         # Configuration
│   │   ├── discovery/      # Project scanner
│   │   ├── tmux/           # tmux client with caching
│   │   ├── tui/            # Bubble Tea TUI
│   │   └── web/            # HTTP server + templates
│   ├── gurgeh/             # Gurgeh-specific code
│   │   ├── agents/         # Agent profile management
│   │   ├── brief/          # Brief composer
│   │   ├── cli/            # CLI commands
│   │   ├── config/         # Configuration
│   │   ├── git/            # Git auto-commit
│   │   ├── project/        # Project detection
│   │   ├── research/       # Research outputs
│   │   ├── scan/           # Codebase scanner
│   │   ├── specs/          # PRD schema, validation
│   │   ├── suggestions/    # Staged updates
│   │   └── tui/            # Bubble Tea TUI
│   ├── coldwine/        # Coldwine-specific code
│   │   ├── agent/          # Agent adapters
│   │   ├── cli/            # CLI commands
│   │   ├── config/         # Configuration
│   │   ├── git/            # Git/worktree management
│   │   ├── project/        # Project detection
│   │   ├── specs/          # Task schema
│   │   ├── storage/        # SQLite storage
│   │   ├── tmux/           # tmux integration
│   │   └── tui/            # Bubble Tea TUI
│   └── pollard/            # Pollard-specific code
│       ├── api/            # Programmatic API for integration
│       ├── cli/            # CLI commands
│       ├── config/         # Configuration
│       ├── hunters/        # Research agents (github, hackernews, arxiv, competitor)
│       ├── insights/       # Synthesized findings
│       ├── patterns/       # Implementation patterns
│       ├── reports/        # Markdown report generation
│       ├── sources/        # Raw collected data types
│       └── state/          # SQLite state management
├── pkg/                    # Shared packages (see pkg/AGENTS.md)
│   ├── agenttargets/       # Run-target registry/resolver
│   ├── contract/           # Cross-tool entity types
│   ├── events/             # Event spine (SQLite)
│   └── tui/                # TUI styles (Tokyo Night)
├── mcp-client/             # TypeScript MCP client
├── mcp-server/             # TypeScript MCP server
├── prototypes/             # Experimental code
├── docs/
│   ├── bigend/           # Bigend docs
│   ├── gurgeh/             # Gurgeh docs
│   └── coldwine/        # Coldwine docs
├── dev                     # Unified dev script
├── go.mod
└── go.sum
```

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

# Build and run individual tools
./dev bigend           # Web mode (default)
./dev bigend --tui     # TUI mode
./dev gurgeh             # TUI mode
./dev gurgeh list        # CLI mode
./dev coldwine        # TUI mode
./dev coldwine list   # CLI mode

# Test all
go test ./...

# Test specific package
go test ./internal/bigend/tmux -v
```

### Configuration

**Shared agent targets** (global + per-project overrides):

- Global: `~/.config/autarch/agents.toml`
- Project: `.gurgeh/agents.toml`
- Compat: `.gurgeh/config.toml` `[agents]` (used if `.gurgeh/agents.toml` missing)

Example:
```toml
[targets.codex]
command = "codex"
args = []

[targets.claude]
command = "claude"
args = []
```

**Bigend** (`~/.config/bigend/config.toml`):
```toml
[server]
port = 8099
host = "0.0.0.0"

[discovery]
scan_roots = ["~/projects"]
scan_interval = "30s"
```

**Gurgeh** (`.gurgeh/config.toml`):
```toml
[agents.claude]
command = "claude"
args = ["--print", "--dangerously-skip-permissions"]

[agents.codex]
command = "codex"
args = ["--approval-mode", "full-auto"]
```

**Coldwine** (`.coldwine/config.toml`):
```toml
[tui]
confirm_approve = true

[review]
target_branch = ""
```

---

## Tool-Specific Details

### Bigend

Mission control dashboard for monitoring AI agents across projects.

**Data Sources:**
| Source | Location | Data |
|--------|----------|------|
| Gurgeh | `.gurgeh/specs/*.yaml` | PRDs, requirements |
| Coldwine | `.coldwine/specs/*.yaml` | Tasks, states |
| MCP Agent Mail | `~/.agent_mail/` | Cross-project messages |
| tmux | `tmux list-sessions` | Active sessions |

**Key Features:**
- Web dashboard with htmx
- TUI mode with Bubble Tea
- tmux session detection with status (running/waiting/idle/error)
- Claude session ID detection
- Cached tmux data (2-second TTL)

### Gurgeh

TUI-first PRD generation and validation CLI.

**Key Paths:**
- `.gurgeh/specs/` - PRD YAML files (source of truth)
- `.gurgeh/research/` - Market/competitive research
- `.gurgeh/suggestions/` - Staged updates for review
- `.gurgeh/briefs/` - Agent briefs (timestamped)

**Commands:**
```bash
gurgeh              # Launch TUI
gurgeh init         # Initialize .gurgeh/
gurgeh list         # List PRDs
gurgeh show <id>    # Show PRD details
gurgeh run <brief>  # Spawn agent with brief
```

### Coldwine

Task orchestration with git worktree isolation.

**Key Paths:**
- `.coldwine/specs/` - Epic/story YAML specs
- `.coldwine/plan/` - Exploration summary + init prompts
- `.coldwine/config.toml` - Configuration
- `.coldwine/activity.log` - Audit log (JSONL)
- `.coldwine/worktrees/` - Isolated git worktrees

**Task States:** `todo` → `in_progress` → `review` → `done` (or `blocked`)

**Commands:**
```bash
coldwine              # Launch TUI
coldwine init         # Initialize + generate epics/stories from scan
coldwine scan         # Re-scan repo and update exploration summary
coldwine status       # Show current task status
coldwine start <id>   # Start task (creates worktree)
coldwine stop <id>    # Stop task
```

### Pollard

Continuous research intelligence for product development. Named after Cayce Pollard from William Gibson's *Pattern Recognition*.

Pollard is a **general-purpose research system** that can research any domain: technology, medicine, law, economics, nutrition, and more.

**Hunters (Research Agents):**

*Tech-Focused (enabled by default):*
| Hunter | Purpose | API |
|--------|---------|-----|
| `github-scout` | Find relevant OSS implementations | GitHub Search API |
| `trend-watcher` | Track industry discourse | HackerNews Algolia API |
| `research-scout` | Track academic research | arXiv API |
| `competitor-tracker` | Monitor competitor changes | HTML scraping |

*General-Purpose (disabled by default, enable as needed):*
| Hunter | Purpose | API | Auth |
|--------|---------|-----|------|
| `openalex` | 260M+ academic works, all disciplines | OpenAlex | Email (optional) |
| `pubmed` | 37M+ biomedical/medical citations | NCBI E-utilities | API key (optional) |
| `usda-nutrition` | 1.4M+ foods, nutrients, allergens | USDA FoodData Central | API key (required) |
| `legal` | 9M+ US court decisions | CourtListener | API key (required) |
| `economics` | Global economic indicators | World Bank | None |
| `wiki` | Millions of entities, all domains | Wikipedia/Wikidata | None |

**Key Paths:**
- `.pollard/config.yaml` - Hunter configs and schedules
- `.pollard/state.db` - SQLite run history and freshness
- `.pollard/sources/github/` - Raw GitHub repo data
- `.pollard/sources/hackernews/` - Trend items
- `.pollard/sources/research/` - Academic papers (arXiv)
- `.pollard/sources/openalex/` - Multi-domain academic works
- `.pollard/sources/pubmed/` - Biomedical articles
- `.pollard/sources/nutrition/` - Food/nutrition data
- `.pollard/sources/legal/` - Court cases
- `.pollard/sources/economics/` - Economic indicators
- `.pollard/sources/wiki/` - Wikipedia/Wikidata entities
- `.pollard/insights/competitive/` - Competitor changes
- `.pollard/reports/` - Generated markdown reports

**Commands:**
```bash
pollard init                        # Initialize .pollard/
pollard scan                        # Run all enabled hunters
pollard scan --hunter github-scout  # Run specific hunter
pollard scan --hunter openalex      # Run OpenAlex (multi-domain academic)
pollard scan --hunter pubmed        # Run PubMed (medical research)
pollard scan --dry-run              # Show what would run
pollard report                      # Generate landscape report
pollard report --type competitive   # Competitive analysis
pollard report --type trends        # Industry trends
pollard report --type research      # Academic papers
pollard report --stdout             # Output to terminal
```

**API Integration:**
Gurgeh and Coldwine can trigger Pollard research via the API:
```go
import "github.com/mistakeknot/autarch/internal/pollard/api"

scanner := api.NewScanner(projectPath)
result, _ := scanner.ResearchForPRD(ctx, vision, problem, requirements)
result, _ := scanner.ResearchForEpic(ctx, epicTitle, description)
result, _ := scanner.ResearchUserPersonas(ctx, personas, painpoints)

// Get insights linked to a feature (for Coldwine)
insights, _ := scanner.GetInsightsForFeature(ctx, "FEAT-001")
brief, _ := scanner.GenerateResearchBrief(ctx, "FEAT-001")
```

**Environment Variables:**
| Variable | Hunter | Required |
|----------|--------|----------|
| `GITHUB_TOKEN` | github-scout | No (faster with) |
| `OPENALEX_EMAIL` | openalex | No (faster with) |
| `NCBI_API_KEY` | pubmed | No (faster with) |
| `USDA_API_KEY` | usda-nutrition | Yes |
| `COURTLISTENER_API_KEY` | legal | Yes |

**Rate Limits:**
| API | Unauthenticated | With Token/Email |
|-----|-----------------|------------------|
| GitHub | 60 req/hr | 5000 req/hr |
| HackerNews | Generous | N/A |
| arXiv | 1 req/3s | N/A |
| OpenAlex | 10 req/s | 100k/day (with email) |
| PubMed | 3 req/s | 10 req/s |
| USDA | N/A | 12k req/hr |
| CourtListener | N/A | Generous |
| World Bank | Polite use | N/A |
| Wikipedia | 5 req/s | N/A |

---

## Shared Packages

See [`pkg/AGENTS.md`](pkg/AGENTS.md) for detailed documentation on shared packages:

| Package | Purpose |
|---------|---------|
| `contract` | Cross-tool entity types (Initiative, Epic, Story, Task, Run, Outcome) |
| `events` | Event spine for cross-tool communication (SQLite at `~/.autarch/events.db`) |
| `tui` | Shared TUI styles (Tokyo Night palette) |
| `agenttargets` | Run-target registry/resolver |

---

## Code Conventions

- Use `internal/` for all tool-specific packages
- Use `pkg/` only for shared code across tools
- Error handling: wrap with `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` with structured fields
- No external dependencies for core functionality
- SQLite: read-only connections to external DBs

### Testing
- TDD for behavior changes
- Run targeted tests while iterating: `go test ./internal/<pkg> -v`
- Small unit tests over broad integration tests

---

## Environment Variables

| Variable | Tool | Default |
|----------|------|---------|
| `VAUXHALL_PORT` | Bigend | 8099 |
| `VAUXHALL_SCAN_ROOTS` | Bigend | ~/projects |
| `PRAUDE_CONFIG` | Gurgeh | .gurgeh/config.toml |
| `TANDEMONIUM_CONFIG` | Coldwine | .coldwine/config.toml |
| `GITHUB_TOKEN` | Pollard | (optional, faster rate limit) |
| `POLLARD_GITHUB_TOKEN` | Pollard | (alternative to GITHUB_TOKEN) |
| `OPENALEX_EMAIL` | Pollard | (optional, polite pool access) |
| `NCBI_API_KEY` | Pollard | (optional, faster PubMed) |
| `USDA_API_KEY` | Pollard | (required for usda-nutrition) |
| `COURTLISTENER_API_KEY` | Pollard | (required for legal) |

---

## Git Workflow

### Commit Messages
```
type(scope): description

Types: feat, fix, chore, docs, test, refactor
Scopes: bigend, gurgeh, coldwine, tui, build
```

### Landing a Session
1. Run tests: `go test ./...`
2. Commit changes with clear messages
3. Push to remote: `git push`
4. Create issues for remaining work

---

## Integration Points

### Gurgeh → Coldwine
- Coldwine reads `.gurgeh/specs/` for PRD context
- Tasks can reference PRD IDs

### Gurgeh → Pollard
- Gurgeh can trigger Pollard research during PRD creation
- `scanner.ResearchForPRD()` runs relevant hunters
- `scanner.ResearchUserPersonas()` for persona research
- Research results feed into PRD context

### Coldwine → Pollard
- Coldwine can trigger Pollard research for epics
- `scanner.ResearchForEpic()` runs hunters with epic context
- Patterns from Pollard inform implementation decisions

### Pollard → Gurgeh/Coldwine
- Insights link to Gurgeh Features
- Patterns link to Coldwine Epics
- Recommendations suggest feature priorities

### Bigend → All
- Reads Gurgeh specs, Coldwine tasks, Pollard insights
- Monitors tmux sessions across all projects
- Read-only aggregation (observes, doesn't control)
- Future: Runs Pollard hunters via daemon

### Intermute (Future)
- Cross-tool agent coordination layer
- Replaces MCP Agent Mail
- File-based messaging now, HTTP API planned
- Message format in `.pollard/inbox/` and `.pollard/outbox/`

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
