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
| `Ctrl+X` | Delete (context-dependent) |
| `Ctrl+S` | Scan (in kickoff) |

### Slash Commands

Type `/` in the chat composer to open a fuzzy finder for commands. Use `↑`/`↓` to navigate, `Tab` or `Enter` to select, `Esc` to dismiss.

#### Global Commands (work everywhere)

| Command | Alias | Action |
|---------|-------|--------|
| `/help` | `/h` | Show help overlay |
| `/quit` | `/q`, `/exit` | Quit the application |
| `/settings` | `/config` | Open chat settings |
| `/model` | `/m` | Toggle model selector |
| `/palette` | `/p` | Open command palette (dashboard) |
| `/refresh` | `/r` | Refresh current view |
| `/back` | `/b` | Go back / cancel |

#### Kickoff View

| Command | Alias | Action |
|---------|-------|--------|
| `/scan` | `/s` | Scan current directory |
| `/new` | `/n` | Focus on new project input |
| `/delete` | `/d` | Delete selected project |

#### Sprint / Arbiter Views

| Command | Alias | Action |
|---------|-------|--------|
| `/accept` | `/a` | Accept current draft |
| `/1`, `/2`, `/3` | - | Select alternative option |

> **Editing drafts:** Chat with the agent to refine content, or open the spec file directly in your editor.

#### Epic / Task Review Views

| Command | Alias | Action |
|---------|-------|--------|
| `/accept` | `/a` | Accept all proposals |
| `/delete` | `/d` | Delete selected item |
| `/edit` | `/e` | Edit selected (epics) |
| `/group` | `/g` | Toggle grouped view (tasks) |
| `/type` | `/t` | Cycle task type (tasks) |
| `/regen` | `/regenerate` | Regenerate proposals (epics) |

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

Bigend has no tool-specific environment variables. Configure it with flags (`--port`, default 8099; `--host`; `--config`; `--scan-root`; `--tui`; `--daemon`) and `~/.config/bigend/config.toml`. For shared settings, see [Autarch-wide](#autarch-wide) and [Intermute (Cross-Tool)](#intermute-cross-tool) below; these tables are not an exhaustive list of environment variables read by dependencies.

### Autarch-wide

Shared packages read these variables when their theme, signals or dispatch functionality is used.

| Variable | Default | Accepted values / effect |
|----------|---------|--------------------------|
| `AUTARCH_THEME` | `auto` | Case-insensitive, whitespace-trimmed: `auto`, `tokyo`/`tokyo-night`, `mocha`, `macchiato`, `latte`/`light`, `nord`, `plain`/`none`/`no-color`/`nocolor`. Empty or unknown values use automatic detection: Tokyo Night for dark backgrounds, Catppuccin Latte for light backgrounds. Color disabling takes precedence. |
| `AUTARCH_NO_COLOR` | Unset (defer to `NO_COLOR`) | Case-insensitive, whitespace-trimmed: `1`/`true`/`yes`/`on` disable colors; `0`/`false`/`no`/`off` force colors on even with `NO_COLOR` set. Other values defer to `NO_COLOR`. |
| `NO_COLOR` | Unset (colors enabled) | Any set value, including empty, disables colors unless overridden by a recognized `AUTARCH_NO_COLOR` value. |
| `AUTARCH_SIGNALS_URL` | `http://127.0.0.1:8092` | Whitespace-trimmed signals server address; empty uses the default. The client accepts HTTP/HTTPS URLs and prepends `http://` when neither scheme is present. |
| `AUTARCH_DISPATCH_BACKEND` | `subscription-cli` | Backend preference; defined values are `subscription-cli` and `api`. Any nonempty string overrides project configuration without validation or trimming. |
| `AUTARCH_DISPATCH_AGENT` | Empty (auto-detect) | `claude` or `codex`; empty leaves project configuration or auto-detection in effect. Any nonempty string overrides project configuration without validation or trimming; unsupported names fail at dispatch. |

Dispatch defaults above apply when project `agents.toml` supplies no preference; nonempty environment overrides take precedence.

### Intermute (Cross-Tool)

| Variable | Description |
|----------|-------------|
| `INTERMUTE_URL` | Server URL |
| `INTERMUTE_API_KEY` | Auth token |
| `INTERMUTE_PROJECT` | Project scope |
| `INTERMUTE_AGENT_NAME` | Registration name override (whitespace-trimmed); defaults to the caller's agent name, or the tool name with `RegisterTool`. |
| `INTERMUTE_HEARTBEAT_INTERVAL` | Registration heartbeat interval; default `30s`. Accepts Go duration strings such as `10s` or `1m`; empty or invalid strings retain the default. Use a positive duration (zero or negative durations panic when the ticker starts). |

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
