# Tandemonium Go TUI Execute-Only MVP Design

**Date:** 2026-01-11
**Status:** Proposed
**Scope:** Full in-place rewrite to Go, TUI-first, Execute-only MVP, macOS + Linux

## Decisions (Confirmed)

- **Rewrite target:** Go implementation replaces Rust/Tauri in-place.
- **Frontend:** TUI-first (Bubble Tea). Minimal CLI for bootstrap/diagnostics only.
- **Persistence:** SQLite (WAL) for runtime state + YAML specs committed to git.
- **Platform:** macOS + Linux for MVP; Windows/WSL later.
- **Product phase:** Execute-only (multi-agent orchestration + review). Planning/Refine deferred.
- **Alias:** `tandemonium` is the binary; `tand` is an installer/shell alias (not guaranteed).

## Repo Layout & Build

```
cmd/
  tandemonium/           # Primary binary
  tand/                  # Thin alias binary (optional)
internal/
  agent/                 # Agent lifecycle, health, prompt injection
  config/                # TOML config + env/flag overrides
  git/                   # Worktrees, branch naming, merge
  review/                # Review queue + diff inspection
  storage/               # SQLite + YAML specs
  tmux/                  # Sessions, pipe-pane, streaming
  tui/                   # Bubble Tea models, views, keymaps
scripts/
  install-alias.sh       # Optional installer for `tand`
.tandemonium/
  state.db
  sessions/
  specs/
```

Go module at repo root. Remove Rust artifacts once Go scaffolding lands.

## Execution Architecture (TUI-first)

- TUI is the primary process; it opens config, then SQLite (WAL), then discovers tmux sessions.
- Each task starts a git worktree, launches a tmux session in that worktree, and attaches
  `pipe-pane` to `.tandemonium/sessions/<session-id>.log` with metadata in `.meta`.
- A streaming reader tails logs, renders output (ANSI stripped) and runs completion/blocker
  heuristics plus a magic-string fallback for completion.
- Fleet view is default. Focus view opens a single agent stream. Review queue lists completed
  tasks and allows file-by-file diff review using git.
- Approve merges the worktree branch into the default branch; no auto-commit aside from
  specs/plans (per spec behavior).

## Data Model & Storage

- **Specs:** `.tandemonium/specs/TAND-xxx.yaml` committed to git on `ready`.
- **Runtime:** `.tandemonium/state.db` holds task states, session state, review queue, and
  last-read offsets for log streaming.
- **Recovery:** Rebuild DB from specs + tmux sessions if state is corrupted.

## Config (TOML)

Layered config: user (`~/.config/tandemonium/config.toml`), project (`.tandemonium/config.toml`),
then env/flags.

MVP keys:
- `general.max_agents`
- `tmux.session_prefix`
- `git.branch_strategy`
- `git.auto_commit_specs`
- `coding_agent.auto_accept`
- `logging.level`

## Risks & Mitigations

- **Scope creep into planning/refine:** keep Execute-only entry points and hide planning UI.
- **tmux dependency:** document macOS + Linux requirement; fail fast with clear errors.
- **Spec/DB drift:** provide a `tand doctor` and `tand recover` path early.

## Next Steps (Implementation)

- Scaffold Go module and package layout.
- Implement config loading and `.tandemonium/` initialization.
- Implement tmux session lifecycle + streaming contract.
- Implement SQLite state schema + migrations.
- Build Fleet view + Focus view + Review queue.
- Add minimal CLI (`init`, `status`, `doctor`, `recover`, `cleanup`).
- Remove Rust/Tauri code and update README.
