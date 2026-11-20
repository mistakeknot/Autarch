# Autarch

Autarch's main interface is **Citadel**, a decision-card and agent-oversight UI
that runs as a bb plugin. It is for people directing agents who need to review
work, answer questions and see what is blocked.

**Citadel** is the sidebar name; **Home** is the name retained in the plugin's
commands and storage.

## Citadel today

- **Queue** separates cards that need your decision, Later, work blocked on
  others, and changes since you left. Filter cards by project.
- Read a card's question, options, recommendation and evidence. Pick an answer,
  send a note or answer in your own words. Image evidence opens in a lightbox.
  On phones, inbox picks have a five-second Undo window.
- **Archive** lets you search earlier cards and bring a question back for a new
  round while keeping its history.
- **Ideas** lets you record an idea, then pursue, park or drop it.
- **Learn**, enabled in Settings, offers short reading pages with Got it and
  Later marks. **Vizier** opens the configured oversight thread's chat.
- **Settings** holds delegation and project bindings. Picks and updates are
  reflected across open pages.

A pick records a decision and tells the owning thread. Agents file cards with
`autarch needs-mk file`. Citadel does not itself merge, deploy, publish or spend.
Read [what Citadel does and never does](integrations/bb-plugin-autarch/learn/01-what-home-is.md)
and [how cards and options work](integrations/bb-plugin-autarch/learn/02-cards-and-options.md).

## Install and open Citadel

You need bb. Building the plugin from source also needs Node.js and npm.
From the `integrations/bb-plugin-autarch/` directory:

```sh
npm install
bb plugin build
bb plugin install .
```

Open **Citadel** in bb's sidebar. See the [plugin README](integrations/bb-plugin-autarch/README.md)
for installation, configuration and storage details, and the
[plugin overview](integrations/bb-plugin-autarch/PLUGIN_OVERVIEW.md) for its role.
The plugin reads project data through a local Autarch service; it does not start
that service. Build the Go CLI below and make it available as `autarch`, then
run `autarch serve` with the project and token configuration described in the
[local service reference](AGENTS.md#autarch-serve).

For source development, rebuild and reload after edits:

```sh
bb plugin build
bb plugin reload autarch
```

For an existing managed installation, the **Update available** control shows
commits and lets you request an update. The configured runner checks the commit,
backs up data, installs it and rolls back on failure. This needs the managed
update setup; a plain plugin install does not create that runner. The repository's
[Home deployment wrapper](scripts/home-ui.sh) and
[schema upgrade scripts](scripts/home-upgrade-v3.sh) document the operator
workflow. They can change live data and are not a generic first-install recipe.

Read the [Learn pages](integrations/bb-plugin-autarch/learn/) for cards, delegation,
checks, scripts and ideas. The [product foundation](docs/why.md) and
[roadmap](docs/roadmap.md) describe the broader project-visit direction; they are
planning context, not a list of shipped Citadel features.

## Go tools and local inspection

The repository also contains a Go CLI, terminal dashboards and research tools.
These remain available alongside the bb plugin. Building them needs Go 1.25+;
tmux is used by the daily catch-up and Coldwine.

| Tool | What it does | Interface |
|------|-------------|-----------|
| **Autarch** | Daily catch-up and a project's product HUD | CLI + TUI |
| **Bigend** | Multi-project agent mission control | Web + TUI |
| **Gurgeh** | PRD generation and validation | CLI + TUI |
| **Coldwine** | Task orchestration | CLI + TUI |
| **Pollard** | Research intelligence | CLI |
| **Mycroft** | Coordinator-selected worker spawning and pre-dispatch checks | CLI |

Mycroft has no patrol loop or TUI tab. See the
[worker spawner guide](docs/mycroft-bb-spawner.md).

### Build and run

From the repository root:

```sh
GOWORK=off go build -mod=readonly -o autarch ./cmd/autarch
./autarch
./autarch project .
go build ./cmd/...
./dev autarch tui
```

The development wrapper also exposes individual tools:

```sh
./dev bigend
./dev bigend --tui
./dev gurgeh
./dev coldwine
```

Pollard's CLI can initialize a project, scan and produce a report:

```sh
go run ./cmd/pollard init
go run ./cmd/pollard scan
go run ./cmd/pollard report
```

### Daily catch-up and project HUD

The catch-up shows recent commit subjects and dated agent reports. Undated
working-tree changes are counted separately. The project HUD reads product
context from local files and Beads without assigning an alignment score.

Open `autarch` with no arguments. It looks back to your last visit, or 24 hours
on your first visit. Click **Time range** or press `w` to choose last visit,
24 hours, 3 days, 7 days, or 30 days. The current window stays visible.

Click **View** or press `v` to compare **Cozy** (roomier summaries) and
**Compact** (more projects at a glance). Press `d` to switch immediately.
Cozy is the initial view; your choice is saved in the local display configuration
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

### Project onboarding

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

## Implementation

Citadel uses TypeScript, React and SQLite in the bb plugin. The Go tools use
Bubble Tea and Lip Gloss for TUIs, net/http with htmx and Tailwind for web
views, and SQLite with a pure Go driver.

| Path | Contents |
|------|----------|
| `integrations/bb-plugin-autarch/` | Citadel plugin, UI, storage and Learn pages |
| `internal/homeask/` | Decision-card filing and held-request handling |
| `scripts/` | Build, deployment and maintenance helpers |
| `cmd/`, `internal/` | Go entry points and tool implementations |
| `pkg/tui/` | Shared terminal styles |
| `pkg/contract/`, `pkg/events/` | Cross-tool types and event storage |

Bigend can register with Intermute when `INTERMUTE_URL` is set. It handles
session I/O while Intermute supplies coordination and messaging. See the
[integration guide](docs/INTEGRATION.md).

## License

MIT
