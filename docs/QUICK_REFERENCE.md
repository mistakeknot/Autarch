# Autarch Quick Reference

> Compact cheat sheet for all Autarch tools

---

## Starting Tools

| Command | Description |
|---------|-------------|
| `./dev bigend` | Mission control web dashboard |
| `./dev bigend --tui` | Mission control TUI |
| `./dev gurgeh` | PRD generation TUI |
| `./dev gurgeh list` | List PRDs (CLI) |
| `./dev coldwine` | Task orchestration TUI |
| `./dev coldwine list` | List tasks (CLI) |
| `go run ./cmd/pollard` | Research CLI |
| `go build ./cmd/...` | Build all binaries |

---

## Repo Hooks

| Command | Description |
|---------|-------------|
| `./scripts/hooks/install-git-hooks.sh` | Install repo git hooks (updates `docs/plans/STATUS.md` on commit) |
| `./dev autarch plan-status` | Generate plan status report manually |

---

## Gurgeh Commands

| Command | Description |
|---------|-------------|
| `gurgeh` | Launch TUI |
| `gurgeh init` | Initialize `.gurgeh/` |
| `gurgeh list` | List all PRDs |
| `gurgeh show <id>` | Show PRD details |
| `gurgeh run <brief>` | Run agent with brief |
| `gurgeh run <brief> --agent=claude` | Run with specific agent |

### Gurgeh TUI Keys

| Key | Action |
|-----|--------|
| `n` | New PRD |
| `e` | Edit selected PRD |
| `d` | Delete PRD |
| `r` | Refresh list |
| `Enter` | View PRD details |
| `?` | Show help |
| `Ctrl+C` | Quit |

---

## Coldwine Commands

| Command | Description |
|---------|-------------|
| `coldwine` | Launch TUI |
| `coldwine init` | Initialize + generate from PRDs |
| `coldwine scan` | Re-scan repo |
| `coldwine list` | List all tasks |
| `coldwine list --status=todo` | Filter by status |
| `coldwine show <id>` | Show task details |
| `coldwine start <id>` | Start task (creates worktree) |
| `coldwine complete <id>` | Complete task |
| `coldwine status` | Current status |

### Coldwine TUI Keys

| Key | Action |
|-----|--------|
| `A` | Accept ALL proposals |
| `e` | Edit selected |
| `d` | Delete selected |
| `s` | Start task |
| `c` | Complete task |
| `g` | Toggle grouped view |
| `tab` | Cycle task type |
| `R` | Regenerate proposals |
| `?` | Show help |
| `Ctrl+C` | Quit |

### Task States

```
todo → in_progress → review → done
         ↓
       blocked
```

---

## Pollard Commands

| Command | Description |
|---------|-------------|
| `pollard init` | Initialize `.pollard/` |
| `pollard` | Show status |
| `pollard scan` | Run all enabled hunters |
| `pollard scan --hunter <name>` | Run specific hunter |
| `pollard scan --dry-run` | Show what would run |
| `pollard report` | Generate landscape report |
| `pollard report --type competitive` | Competitive analysis |
| `pollard report --type trends` | Industry trends |
| `pollard report --type research` | Academic papers |
| `pollard report --stdout` | Output to terminal |
| `pollard propose` | Generate research agendas |
| `pollard hunter list` | List available hunters |

### Hunters

| Hunter | Purpose | Auth |
|--------|---------|------|
| `github-scout` | OSS implementations | Optional |
| `hackernews` | Industry trends | None |
| `arxiv` | CS/ML papers | None |
| `competitor-tracker` | Competitor changes | None |
| `openalex` | All academic disciplines | Optional |
| `pubmed` | Medical/biomedical | Optional |
| `usda-nutrition` | Food/nutrition | **Required** |
| `legal` | Court decisions | **Required** |
| `economics` | Economic indicators | None |
| `wiki` | Entity lookup | None |

---

## Bigend Commands

| Command | Description |
|---------|-------------|
| `bigend` | Web dashboard (port 8099) |
| `bigend --tui` | TUI mode |
| `bigend --scan-root <path>` | Override scan root |

### Bigend TUI Keys

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate |
| `Enter` | View details |
| `a` | Attach to session |
| `k` | Kill session |
| `Ctrl+R` | Refresh |
| `Tab` / `Ctrl+Left/Right` | Switch tabs |
| `?` | Show help |
| `Ctrl+C` | Quit |

### Web Routes

| Route | Description |
|-------|-------------|
| `/` | Dashboard |
| `/projects` | Project list |
| `/projects/:path` | Project detail |
| `/agents` | Agent list |
| `/sessions` | tmux sessions |
| `/api/state` | Full state JSON |

---

## Universal TUI Keys

| Key | Action |
|-----|--------|
| `Ctrl+C` | Clear input (once) / Quit (twice) |
| `↓` / `↑` | Move down / up |
| `Enter` | Select / confirm |
| `Esc` | Cancel / go back |
| `Tab` | Cycle pane focus |
| `Ctrl+Left/Right` | Switch tabs |
| `Ctrl+PgUp/PgDn` | Switch tabs (fallback) |
| `Ctrl+F` | Search |
| `Ctrl+R` | Refresh |
| `Ctrl+P` | Command palette |
| `Ctrl+G` | Model selector |
| `Ctrl+A` | Accept (context-dependent) |
| `Ctrl+E` | Edit (context-dependent) |
| `Ctrl+X` | Delete (context-dependent) |
| `Ctrl+S` | Scan (in kickoff) |

### Slash Commands

Type these in the chat composer and press Enter:

| Command | Action |
|---------|--------|
| `/help` | Show help overlay |
| `/quit` | Quit the application |

> **Chat-focused design:** All shortcuts use `Ctrl+` combinations to avoid conflicts with typing in the chat composer. Function keys (F1-F12) still work as fallbacks for external keyboards. Slash commands work in any chat input.

---

## File Locations

### Gurgeh (`.gurgeh/`)

| Path | Contents |
|------|----------|
| `specs/` | PRD YAML files |
| `research/` | Market research |
| `suggestions/` | Staged updates |
| `briefs/` | Agent briefs |
| `config.toml` | Agent profiles |
| `agents.toml` | Agent targets |

### Coldwine (`.coldwine/`)

| Path | Contents |
|------|----------|
| `specs/` | Epic/story YAML |
| `plan/` | Exploration summary |
| `config.toml` | Configuration |
| `activity.log` | Audit log (JSONL) |
| `worktrees/` | Git worktrees |

### Pollard (`.pollard/`)

| Path | Contents |
|------|----------|
| `config.yaml` | Hunter config |
| `state.db` | Run history |
| `sources/github/` | GitHub repos |
| `sources/hackernews/` | HN items |
| `sources/research/` | arXiv papers |
| `sources/openalex/` | Academic works |
| `sources/pubmed/` | Medical articles |
| `insights/competitive/` | Competitor data |
| `reports/` | Generated reports |

### Global Config

| Path | Contents |
|------|----------|
| `~/.config/bigend/config.toml` | Bigend config |
| `~/.config/autarch/agents.toml` | Global agent targets |

---

## Environment Variables

### Bigend

| Variable | Default | Description |
|----------|---------|-------------|
| `VAUXHALL_PORT` | 8099 | Web port |
| `VAUXHALL_HOST` | 0.0.0.0 | Web host |
| `VAUXHALL_SCAN_ROOTS` | ~/projects | Scan paths |

### Intermute (Cross-Tool)

| Variable | Description |
|----------|-------------|
| `INTERMUTE_URL` | Server URL |
| `INTERMUTE_API_KEY` | Auth token |
| `INTERMUTE_PROJECT` | Project scope |

### Pollard

| Variable | Hunter | Required |
|----------|--------|----------|
| `GITHUB_TOKEN` | github-scout | No |
| `OPENALEX_EMAIL` | openalex | No |
| `NCBI_API_KEY` | pubmed | No |
| `USDA_API_KEY` | usda-nutrition | **Yes** |
| `COURTLISTENER_API_KEY` | legal | **Yes** |

### Legacy Names

| Variable | Tool |
|----------|------|
| `PRAUDE_CONFIG` | Gurgeh |
| `TANDEMONIUM_CONFIG` | Coldwine |
| `POLLARD_GITHUB_TOKEN` | Pollard |

---

## Common Workflows

### New Project Setup

```bash
./dev gurgeh init        # PRDs
go run ./cmd/pollard init # Research
./dev coldwine init      # Tasks
```

### Create PRD → Tasks

```bash
./dev gurgeh             # Create PRD
./dev coldwine init      # Generate tasks
./dev coldwine           # Review & accept
```

### Research → Report

```bash
go run ./cmd/pollard scan
go run ./cmd/pollard report --stdout
```

### Start Task with Worktree

```bash
./dev coldwine start <task-id>
# Work in .coldwine/worktrees/<id>/
./dev coldwine complete <task-id>
```

### Monitor Everything

```bash
./dev bigend             # http://localhost:8099
```

---

## Git Integration

### Auto-Commit Messages

| Action | Message |
|--------|---------|
| New PRD | `chore(gurgeh): add PRD-###` |
| Update PRD | `chore(gurgeh): update PRD-###` |
| Task change | `chore(coldwine): update task-###` |

### Worktree Cleanup

```bash
./dev coldwine cleanup   # Remove completed worktrees
```

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| No projects in Bigend | Check `scan_roots` config |
| Rate limited | Set API tokens, wait for reset |
| Tasks not showing | Run `coldwine init` |
| Agent not detected | Name session with "claude"/"codex" |
| Intermute errors | Check `INTERMUTE_URL` |

---

## Links

- **Workflows:** `docs/WORKFLOWS.md`
- **Architecture:** `docs/ARCHITECTURE.md`
- **Integration:** `docs/INTEGRATION.md`
- **Bigend:** `docs/bigend/AGENTS.md`
- **Pollard:** `docs/pollard/AGENTS.md`
- **Coldwine:** `docs/coldwine/AGENTS.md`
