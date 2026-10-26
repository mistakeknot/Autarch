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
  `Activities`, optional `Kernel`, with `UpdatedAt`. It is served as JSON and over a
  websocket, and rendered by the TUI and the web server.
- `discovery.Project` carries `TaskStats`, `PollardStats`, `GurgStats` and
  `InterspectStats`: counts of work per project (the "items").
- Web routes today: `/`, `/projects`, `/agents`, `/sessions`, `/api/agents`, `/api/refresh`,
  `/ws/terminal/`. Templates are htmx + Tailwind (design decision for Bigend web).

## Gaps

- No per-bead or per-card item list in the state: only counts. Belts need item identity
  and age, so the feed needs a thin item list (id, project, status, age, owner).
- No "flow" signal: items entering and leaving a stage per interval. It can be computed
  from consecutive `UpdatedAt` snapshots, so no new store is needed to start.
- Cards live in the Citadel plugin, not in Bigend. Reading them means a read-only call
  through the Home query interface, kept optional.

## Proposed first scene (web first)

One page, `/colony`, server-rendered with htmx polling or the existing websocket:

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

1. Add an item list and flow counters to the aggregator state, with tests.
2. Add `/colony` with the SVG scene and a fixture-driven render test.
3. Run it against live data and review with the agents running.
