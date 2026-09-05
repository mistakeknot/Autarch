# Autarch

Catch up on work across your projects and agent sessions, inspect the questions
they left, and return to the original conversation to answer.

Bare `autarch` opens the daily catch-up: recent commit subjects and dated agent
reports. Undated working-tree changes are counted separately. The existing
mission-control, PRD, orchestration, and research tools remain available.

`autarch project [path]` opens the project's product HUD: persona, primary
journey, success criteria, guardrails, roadmap, backlog, and decision sources.
It reads the existing files and Beads tracker. Declared intent, dated plans,
and live work stay distinguishable; Autarch does not assign an alignment score.

## What this does

Running multiple AI agents across multiple projects creates a coordination problem: which agent is working on what, which specs are current, which tasks are blocked, and what's happening in the competitive landscape. Autarch provides four tools that each solve a piece of this:

| Tool | What it does | Interface |
|------|-------------|-----------|
| **Bigend** | Multi-project agent mission control | Web + TUI |
| **Gurgeh** | PRD generation and validation | TUI |
| **Coldwine** | Task orchestration for human-AI collaboration | TUI |
| **Pollard** | Continuous research intelligence (tech, medicine, law, economics) | CLI |

## Who this is for

Developers running the Demarch agent stack (Clavain, Intermute, Intercore) who want visibility and control over multi-project, multi-agent workflows. Autarch is the operational dashboard layer.

## Quick start

```bash
# Build and open the daily catch-up (Go 1.25+, tmux)
GOWORK=off go build -mod=readonly -o autarch ./cmd/autarch
./autarch
./autarch project .   # product context for this checkout

# Build all tools
go build ./cmd/...

# Tool dashboard
./dev autarch tui

# Individual tools
./dev bigend          # Mission control (web mode)
./dev bigend --tui    # Mission control (TUI mode)
./dev gurgeh          # PRD generation
./dev coldwine        # Task orchestration

# Pollard (research intelligence)
go run ./cmd/pollard init
go run ./cmd/pollard scan
go run ./cmd/pollard report
```

Open `autarch` with no arguments. It looks back to your last visit, or 24 hours
on your first visit. Click **Time range** or press `w` to choose last visit,
24 hours, 3 days, 7 days, or 30 days. The current window stays visible.

Click **View** or press `v` to compare **Cozy** (roomier summaries) and
**Compact** (more projects at a glance). Press `d` to switch immediately.
Cozy is the initial view; your choice is saved in `~/.autarch/display.yaml`
and also applies to the product HUD. Click the header tabs or use `1`–`4`
to navigate. Press `?` for the in-app guide.

For scripts or an explicit inspection window, `autarch --since 24h` remains
available and leaves the last-visit stamp unchanged.

| In the daily catch-up | Key |
|---|---|
| Switch Cozy / Compact | `d`, or `v` to choose |
| Choose a time range | `w` |
| Open the guide | `?` |
| Scroll recent changes | ↑ / ↓ |
| Review questions | `a`, then Enter for full evidence |
| Open the selected question's terminal seat | Enter from the evidence view |
| Resume saved history when its agent is stopped or replaced | `s` from the evidence view |
| Inspect every thread, including unread ones | `t`, then `i` for evidence |
| Open project rows | Tab |
| Open the selected project's product HUD | `i` from project rows |
| Refresh / return / quit | `r` / Esc / `q` |

Questions come from the trailing 4 MiB of Claude Code and Codex transcripts
linked by conversation IDs in tmux session names. Autarch distinguishes a
question visible in a current pane from saved history and uncertain waits.
An ordinary later reply is preserved for review; only an explicit structured
answer confirms resolution. Seats without IDs and unread sources remain
visible in coverage. Autarch sends no answers; resume opens the agent CLI
without a new prompt. `autarch threads --json` exposes the same transcript
evidence, including its private text, for local inspection.

In the project HUD, `1`–`6` or Tab switches Brief, Roadmap, Backlog, Journeys,
Decisions, and Foundation. Arrows scroll, `o` opens source files or their folder in Zed,
`r` refreshes, and Esc returns. The sources are `docs/why.md`,
`docs/roadmap.md`, JSON or Markdown files in `docs/cujs/`, and the card's
decision references. File modification times are labeled separately from
dates declared inside documents. Card and journey states are source declarations,
not measurements of product success.

The backlog uses a bounded, read-only `bd list` from the nearest `.beads`
tracker. A shared ancestor tracker is filtered by the card's project name
(the lowercased directory name if the card is unavailable); this scope is
shown in the view. Unlabeled work is excluded from a shared scope. Explicit
`spec_id` references are displayed without inferring CUJ alignment. Missing
files, unreadable sources, and failed backlog reads remain visible.

## Project onboarding

Choose a project from the Projects tab, press `i`, then **`6 Foundation`**.
Autarch looks for mission, vision, philosophy, personas, critical user journeys,
roadmap, architecture decision records, backlog, and design systems/standards.
It reuses conventional root and `docs/canon/` documents, product-card context,
journeys, decision folders, and the project's live Beads scope.

Press **`n`** to read the onboarding brief and **`c`** to copy it for your
chosen coding agent. The brief includes source paths, search limits, existing
product-card declarations, unresolved needs, and concrete questions for each
area. It asks the agent to reuse evidence, draft proposals as provisional,
and bring consequential choices to you through structured questions. Esc
returns to Foundation; `r` refreshes after the project documents change.

This is the starting point for onboarding: discovery and a portable working
brief. It does not author or approve the foundation itself or launch an agent.
“Sources found” means files or a tracker were readable, not that the content
is complete, current, or agreed. Empty and unreadable sources remain distinct.
Custom or inherited document locations may need to be supplied in the
onboarding conversation; the brief lists exactly where Autarch looked.

## Tech stack

- **Language:** Go 1.25+
- **TUI:** Bubble Tea + Lip Gloss (Tokyo Night palette)
- **Web:** net/http + htmx + Tailwind
- **Database:** SQLite (WAL mode, pure Go driver)

## Prerequisites

- macOS or Linux
- Go 1.25+
- tmux (for the daily catch-up and Coldwine)

## Project structure

```
cmd/           Entry points (bigend, gurgeh, coldwine, pollard)
internal/      Tool-specific code
pkg/tui/       Shared TUI styles (Tokyo Night)
pkg/contract/  Cross-tool entity types
pkg/events/    Event spine (SQLite)
```

## Intermute integration

Autarch auto-registers with the Intermute coordination service when `INTERMUTE_URL` is set:

```bash
export INTERMUTE_URL="http://localhost:7338"
```

Bigend handles session I/O; Intermute provides coordination and messaging.

## License

MIT
