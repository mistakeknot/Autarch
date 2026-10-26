# Bigend colony view: a game-style map of projects, beads, cards and agents

Bead mk-ogl3p. Status: scout and proposal. Nothing is built.

## Idea

Show the whole estate the way a strategy or factory game shows a base. Projects are
bases. Agents are units working at them. Beads and cards are items that move through
the base. Stuck or neglected work shows as a visible jam. Inspiration: the agent-graph
plugin idea and the Stellaris teardown in `docs/`.

## What Bigend already has

- `internal/bigend/colony` already detects "colonies" per discovered project: markers
  (`.colony`, `.agents`), git worktrees, and member processes. The name fits the metaphor.
- `aggregator.State` is the data feed: `Projects`, `Agents`, `Sessions`, `Colonies`, `MCP`,
  `Activities`, optional `Kernel`, with `UpdatedAt`. The TUI and the web templates render
  it. The web server exposes only `/api/agents` as JSON; its websocket (`/ws/terminal/`)
  streams terminal output, not state.
- `discovery.Project` carries `TaskStats`, `PollardStats`, `GurgStats` and
  `InterspectStats`: counts of work per project (the "items").
- Web routes today: `/`, `/projects`, `/agents`, `/sessions`, `/api/agents`, `/api/refresh`,
  `/ws/terminal/`. Templates are htmx + Tailwind (design decision for Bigend web).

## Gaps

- No per-bead or per-card item list in the state: only counts, and `TaskStats` counts
  Coldwine tasks, not beads. Belts need item identity and age, so the feed needs a thin
  item list (id, project, status, age, owner). Proposed bead source: a read-only
  `bd list --json` per discovered project that has a `.beads` directory, keyed by the
  project path. Coldwine counts stay a separate signal.
- No "flow" signal: items entering and leaving a stage per interval. It can be computed
  from consecutive `UpdatedAt` snapshots, so no new store is needed to start.
- Cards live in the Citadel plugin, not in Bigend. Reading them means a read-only call
  through the Home query interface, kept optional.

## Proposed first scene (web first)

One page, `/colony`, server-rendered with htmx polling of a fragment route (a state websocket would be new work):

- A base per project, sized by open work, coloured by neglect (age of the oldest
  open item).
- Units on a base for agents with an active session; idle agents dim.
- A belt per base: items queued by status; a jam is a belt whose oldest item exceeds a
  threshold, drawn in red.
- Click a base for the existing project detail page.

TUI second: same data as a character grid using the shared Tokyo Night styles.

## Open design questions (each a separate decision card, in order)

1. Web first or TUI first? Recommendation: web, because belts and units need 2D layout.
2. Do cards (Citadel) appear as items, or only beads? Recommendation: beads first,
   cards as a later optional source.
3. Drawing: plain SVG via htmx (no new dependency) or canvas? Recommendation: SVG.

## Steps after the answers

1. Add an item list and flow counters to the aggregator state, with tests. Accepted when
   a fixture of two snapshots yields the expected item ids, ages, per-stage entered and
   left counts, and a jam flag for an item past the threshold.
2. Add `/colony` with the SVG scene and a fixture-driven render test. Accepted when the
   rendered SVG for the fixture has one base per project, a unit per active agent, a belt
   item per open bead with its age, and a red belt exactly where the fixture has a jam.
3. Run it against live data and review with the agents running. Accepted when the page
   loads on live data, bases match the project list, and a known stale bead shows as a jam.
