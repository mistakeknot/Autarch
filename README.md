# Tandemonium

> **TUI-first task orchestration for human-AI collaboration**

Tandemonium is a terminal-native tool for solo developers. It orchestrates multiple AI coding sessions with git worktree isolation, tmux session persistence, and a structured review flow. The current roadmap is a **Go + Bubble Tea** TUI that focuses on **Execute-only** (multi-agent orchestration + review). Planning and PM refinement are deferred to later milestones.

## What It Does (MVP)

- Launches a TUI to manage active agent sessions
- Creates isolated git worktrees per task
- Streams tmux output for completion/blocker detection
- Provides a review queue for completed tasks
- Stores runtime state locally in SQLite (WAL) and task specs in YAML

## Tech Stack

- **Language:** Go 1.22+
- **TUI:** Bubble Tea + Lip Gloss
- **Persistence:** SQLite (WAL) + YAML specs in git
- **Process isolation:** tmux
- **Git:** native `git` commands

## Prerequisites

- macOS or Linux (tmux required)
- Git 2.23+ (worktrees)
- Go 1.24+
- tmux

## Quick Start (Development)

```bash
# Build
go build ./cmd/tandemonium

# Run
./tandemonium
```

## Configuration

Project config lives at `.tandemonium/config.toml` (TOML). Layering order:

1) `~/.config/tandemonium/config.toml`
2) `.tandemonium/config.toml`
3) env vars
4) CLI flags

Minimal example:

```toml
[general]
max_agents = 4

[git]
branch_strategy = "feature"

auto_commit_specs = true

[llm_summary]
# Optional: user-managed CLI for summaries (default example uses Claude)
command = "claude"
timeout_seconds = 30
```

## Local State

All state lives in `.tandemonium/`:

```
.tandemonium/
├── state.db         # SQLite runtime state (WAL)
├── specs/           # Task specs (YAML, committed)
└── sessions/        # tmux logs + metadata
```

## Repo Layout (Go)

```
cmd/
  tandemonium/        # Primary binary
internal/
  agent/              # Detection + agent logic
  cli/                # Cobra commands
  config/             # TOML config loader
  git/                # Worktrees + branch operations
  project/            # .tandemonium initialization
  review/             # Review queue
  storage/            # SQLite layer
  tmux/               # tmux session management
  tui/                # Bubble Tea models + views
prd/                  # Product requirements
```

## CLI

Primary binary is `tandemonium`. A `tand` alias may be provided by installers or user shell aliasing.

Current commands (stubs for MVP scaffolding):

```bash
tandemonium init
tandemonium status
tandemonium doctor
tandemonium recover
tandemonium cleanup
```

Mail helpers (MCP parity):

```bash
tandemonium mail summarize --thread <thread-id> --llm --examples
tandemonium mail summarize --dry-run --llm --json
```

`--dry-run` validates your `llm_summary.command` with synthetic input, without requiring a real thread.
See `docs/cli/mail.md` for the full mail command reference.
See `docs/cli/agent.md` for agent registry commands.

## Status

This repository is mid-transition to the Go/TUI execute-only MVP. Rust/Tauri artifacts have been removed. See:

- `prd/tandemonium-spec.md`
- `docs/plans/2026-01-11-go-tui-execute-mvp-design.md`
- `docs/plans/2026-01-11-go-tui-execute-mvp-implementation-plan.md`
