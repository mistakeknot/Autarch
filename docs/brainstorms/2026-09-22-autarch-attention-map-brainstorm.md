---
artifact_type: brainstorm
bead: none
stage: discover
---

# Autarch in BB: an estate map and a Mycroft rail

**Date:** 2026-09-22, revised 2026-09-23 after review
**Status:** design rulings made by mk through structured questions, one at a time. Two prototypes
were rendered and exercised. A six-agent flux-drive review returned **risky** (4 P0, 12 P1, 9 P2;
all 16 synthesized findings grounded; report at
`docs/research/flux-drive/2026-09-22-autarch-attention-map-brainstorm-661d4ab8/summary.md`).
mk ruled on the contested findings (see **Review rulings** below), and the rest are folded in as
constraints. Nothing is built.
**Thread:** BB `thr_xg8t59tfba`, a handoff from codex `thr_dunwxeab5c`. Writing this file makes
that thread the editor of this checkout for now. The Autarch Claude thread `thr_nn4veeieqr` stays
paused, and nothing here is committed.

**Prototypes** (BB thread storage for `thr_xg8t59tfba`, `reports/`):

| File | What it settled |
|---|---|
| `autarch-attention-prototype.html` | Four layouts compared; mk chose **map + escalation side rail** |
| `autarch-map-rail-v2.html` | The rulings below, rendered: four lenses, pinned nudge, semantic zoom, the Mycroft composer and bootstrap |

## What We're Building

A page inside BB, backed by an independent Autarch/Mycroft service, that shows mk's whole estate
(~98 gardens) as **one persistent map** beside **one escalation rail**. Mycroft coordinates agents
as BB threads. What needs mk (questions, blockers, review gates, decisions and dispatch proposals)
climbs to the rail, and the map explains the *shape* around those asks.

- **The map** places gardens in fixed regions: x is the pace layer (fast probe to slow doctrine)
  and y is the ecosystem band. Four lenses repaint the same positions:
  - **attention:** what waits on you, plus a soft glow on gardens that moved but that you haven't
    seen yet
  - **allocation:** agent labor per layer and Mycroft's proposed moves
  - **dependencies:** blast radius, with blocker propagation
  - **neglect:** fade by days untouched, plus a ring for the eight core documents
- **Semantic zoom:** enter a garden to see its core-document shelf (mission, vision, PRD, CUJs,
  roadmap, backlog, personas, philosophy), its outcomes and ready beads, its BB threads, and
  Mycroft's proposal together with the alternatives it weighed.
- **Cadence (ruling 9):** attention is the only default lens, and a live blocker automatically
  draws its known downstream edges. The other lenses are on demand.
- **The rail** is the escalation queue (ordered by who is stalled) plus a **Mycroft composer**
  scoped to the current selection. The composer is a router with one-shot answers and holds no
  chat (ruling 10). Changes and **new projects are bootstrapped by Mycroft from the
  rail**, reusing `clavain:project-onboard`, and a new garden appears in its region once onboarded.

## Why This Approach

**The rail answers "what needs me"; only a map answers "what is the estate doing".** mk asked the
map to earn its place with four jobs a list cannot do:

- **Gestalt and spatial memory.** Positions never re-rank, so gardens are found by place, not by
  reading. This is the Stellaris galaxy-map half of the walk; the outliner half already exists.
- **Neglect and absence.** A queue of asks can never show what *isn't* happening. Fading makes
  pressure ambient instead of modal (`sofg-dread-floor-ambient-pressure-not-modal`), which matches
  quiet-unless-attention-worthy (autarch-01).
- **Blast radius.** A blocker upstream visibly dims its dependents. The prototype uses the live
  rig-health signal that the deployed `ic` predates intercore's HEAD, so Autarch, Clavain, cujgel
  and interflux all show as at risk.
- **Allocation and Mycroft's reasoning.** Pace layers should move at their own speeds (autarch-03),
  so a map with a pace axis shows whether labor matches that. Mycroft's proposals are drawn as
  moves, and a proposal held by an upstream blocker says so.

**One map with lenses, not one screen per question.** This follows
`shadow-empire-one-map-switchable-lenses` and `eu5-map-modes-as-lens`: several facts share one
substrate, so the positions stay put and only the paint changes. The rail stays visible beside the
map because the two are cross-referenced on nearly every visit
(`sofg-forced-split-pane-cross-reference-tabs`). The human sits at the top of the escalation
ladder rather than being a flat notification target (`gastown-severity-routed-escalation`).
Global counters sit on the same sheet as the map (`twilight-struggle-single-sheet-status-strip`).
Agents are rendered as presence on nodes (`flowith-live-cursor-presence`).

**The service plus a BB plugin**, as argued earlier in this thread: BB owns threads, the tracker
owns tasks, and Mycroft owns assignment, which it records as beads claims. Autarch is an overlay
(ruling 12). Records are linked by stable IDs.

## Key Decisions (mk, 2026-09-22)

1. **The page has two elements: map + escalation side rail.** Chosen after the four-layout
   prototype.
2. **The map's jobs, all four:** gestalt and spatial memory; neglect and absence; blast radius and
   dependencies; allocation and Mycroft's reasoning. Seasonal reshaping on the map is not a v1 job.
3. **Position means fixed regions that you can nudge.** x = pace layer and y = ecosystem, both
   from the graph (CanonGraph now, Ultan later). Within its cell a garden can be nudged and stays
   pinned; the pin is a preference, like `door.yaml` pins. Dependencies, neglect and agents are
   overlays and never move a node.
4. **Semantic zoom, and core documents always visible.** The eight-document shelf extends the
   product card. Missing or drifted documents are a neglect signal.
5. **The map is an instrument, with approval in place.** The estate view is read-only; the only
   gesture that writes is the pin-nudge. At garden zoom, Mycroft's proposal can be approved or
   redirected in place, which is the same act as the rail card. There is no drag-to-dispatch.
6. **The rail doubles as the Mycroft composer,** styled after Cursor and BB. Changes and new
   projects are bootstrapped through it, and the map selection sets the prompt's scope.
7. **Time is shown as since-last-visit plus neglect fade.** The glow settles once a garden has been
   seen. **Scrub-back replay goes on the backlog.**
8. **Order of work:** first a read-only attention and outcomes view, then approval-driven dispatch
   at T1. Autonomous tiers come only after a one-coordinator claim exists.

## Review rulings (mk, 2026-09-23)

mk ruled on each contested finding separately. For these questions the options were
deliberately **not** marked "recommended", because the review found that all four rulings of
2026-09-22 had followed the agent's recommended option. Where mk asked for help, the agent's
reasoning was given before the ruling.

9. **Lens cadence: events appear automatically, conditions only on demand.** Supersedes
   ruling 2's framing of all four lenses as daily. Attention is the only default lens.
   - When a live blocker exists, the attention lens automatically draws that blocker's
     downstream edges. It draws **only edges with known provenance**, labelled with graph
     coverage (for example "known dependents: 4 · graph covers 5 edges estate-wide"), and the
     overlay clears itself when the blocker clears. It never adds anything to the rail.
   - Allocation, dependencies and neglect are on-demand lenses and serve as the seasonal visit.
     There is no separate seasonal mode.
   - The principle: a blocker is an *event* and earns automatic display; allocation and neglect
     are slow *conditions* and wait to be asked for.
10. **The composer is a router with one-shot answers.** Supersedes ruling 6's composer.
    - Requests that change the world become a Mycroft proposal card: claim, then approve, then a
      BB thread, where the conversation continues.
    - Questions ("why is X quiet?") get a single evidence card with sources and no history.
    - Autarch and Mycroft keep no chat state. The reason the composer exists is the two things
      BB's composer lacks: estate context from the map selection, and routing through Mycroft's
      claim so the same work cannot be dispatched twice.
11. **Success and kill criterion: the caught-something log.**
    - During a two-week read-only trial, mk taps "map caught this" whenever the *map* rather
      than the rail shows something that would otherwise have been missed: a cascade, a
      neglected garden, or misplaced labor.
    - With zero or one catches, the daily map is cut back to rail plus briefing, and the lenses
      survive only as a seasonal view.
    - The trial's result is recorded as an ADR.
12. **The overlay rule replaces "Autarch holds no state".** The rule's origin (autarch-01: "the
    Mayor lost its data 22 times being a world instead of an overlay") was about truth, not
    bytes.
    - Restated: *Autarch is an overlay. It never owns world truth: everything it shows about the
      estate is re-derived from the owning system (BB, beads, git, the graph). The only things it
      keeps are mk's own: preferences and personal evidence.*
    - **Deletion test:** delete `~/.autarch` entirely. If anything about the world becomes
      wrong, that item was misplaced.
    - Pins, the last-visit stamp, the seen set and the catch log live in `~/.autarch`, which the
      TUI door and the BB page share.
    - `escalate.DecisionQueue` fails the test and moves to BB interactions and beads. Mycroft's
      claims, assignments and weighed alternatives are world truth and live in **beads**: the
      claim is the coordinator lock, and its rationale is the approval audit trail.

### Balancing loops (mk, 2026-09-23; fd-systems findings)

13. **Proposals are budgeted per layer, per period.** Only Mycroft's *own* proposals count
    against the budget. Agent questions and blockers are never throttled, because throttling them
    would hide stalls. Fast layers get a daily budget and slow layers a weekly one; the numbers are
    set in planning. This follows autarch-03: "the interruption budget is a first-class tuned
    number, adjusted when it drifts."
14. **Probes expire.** A new garden, including anything bootstrapped from the rail, starts in the
    Probe layer with an expiry. At expiry Mycroft asks whether to promote it to a slower layer,
    extend it, or park it. A probe nobody answers for parks itself, and nothing is deleted.
    Sprawl is allowed but has to be chosen again.
15. **"Parked" is the single resting concept.** A garden resting on purpose is parked, which is
    the same state probe expiry produces, and parked gardens never fade. The neglect lens
    therefore shows only unparked gardens going untended, and there is no separate "checked"
    mark to maintain.
16. **One rail queue with layer tags.** The rail is ordered by who is stalled, and each item shows
    its layer. The per-layer budgets (13) already cap how much fast-layer work arrives, so the
    rail is not split into sections or digests.

### Accepted from the review as constraints (no ruling needed)

- **Agent text is inert.** Proposal, question and answer text is escaped and rendered
  separately from the action controls; agent-authored text never shares markup with a button
  (S1).
- **Approval weight scales with blast radius.** "Answer" differs from "Approve dispatch",
  which differs from "Authorise `bd init`" and from "Create repo + CI". Actions that create
  external resources or override a hold need an explicit second step. Every approval and
  override is recorded with its claim in beads (S2, S3).
- **Bootstrap reviews before it creates.** The PROVISIONAL card and placement are reviewed
  before any repo or CI exists (S4).
- **No dispatch surface until the beads claim exists,** T1 included, because the bbDev and
  bbOps coordinators already run (S5).
- **Confidence is visible.** Placement, edges and document status each show known, guessed or
  unknown. A garden without a graph row sits in a labelled "unplaced" band, not a guessed cell
  (MAP-001, GRAPH-001, DOCS-001).
- **Prerequisites are named as work:**
  1. land the layer-view plan's `GraphSource` (WI-1 and WI-2; not in the tree yet)
  2. build a BB-thread `AgentSpawner` and `DataSource` for Mycroft, which is a rewrite of the
     tmux substrate, not an extension (F2, F3)
  3. define a Go-service to TypeScript-plugin data contract (F6)

## Prior art

- **Meadowsyn** (`Sylveste/apps/Meadowsyn`, 18 experiments including `cytoscape-graph`,
  `semantic-zoom`, `fixed-stars`, `loopy-signals`, `process-replay` and `unified-compose`). These
  are prototypes with feasibility reviews but **no recorded verdict**. They are reusable rendering
  studies, but Meadowsyn is aimed at a public audience, not the operator.
- **Jawnomicon** ruling: "the constellation should be the main element with a detail pane", the
  same shape as the map plus detail pane here.
- **Autarch door** (`internal/door`): garden rows, the threads screen, and the layer-view plan
  (`docs/plans/2026-09-03-layer-view-plan.md`) with its `GraphSource` interface, which the map's
  placement should read through.
- No shipped epic covers this. The advisory in-tree epic check was not run, because this checkout
  has no `.beads`.

## Open Questions

1. **Ecosystem bands.** CanonGraph has an `ecosystem` field, but only 51 projects have rows, and
   Autarch itself has none. Gardens without a row need a stated fallback region, not a guess.
2. **Dependency edges.** `serving_map` holds 5 edges, and the prototype's edges are illustrative.
   The candidate sources are go.mod and package manifests, bead cross-references, and edges mk
   declares.
3. **Core-document discovery.** A filename scan on zklw found documents for 20 of 29 named
   gardens. Nine have no canonical path on zklw. Backlogs often live in the workspace tracker
   rather than in the repo; Autarch has no `.beads` directory, so the scan marks its backlog
   missing even though `Sylveste-fuwn` exists. The shelf needs per-garden document locations, not
   filename heuristics, or it will report gaps that aren't real.
4. **What counts as "untouched".** The prototype uses the newer of the last commit and the last BB
   thread activity. Is thread activity alone enough evidence of tending?
5. **The one-coordinator claim.** `bbDev | coordinator` and `bbOps | coordinator` threads already
   exist. The claim mechanism (a bead claim or a BB plugin metadata lock) must exist before Mycroft
   dispatches beyond T1.
6. **Scale and legibility.** At ~98 gardens, dense cells collide. Should labels appear only in the
   lens that makes a garden relevant (as prototyped), or should cells aggregate beyond a density
   threshold?
7. **Backlog:** scrub-back replay (following Meadowsyn `process-replay`); seasonal reshaping on the
   map (dragging a garden across layers as the reshaping act).
8. **Balancing loops:** ruled as rulings 13–16. Still open: the budget numbers per layer, which
   are set in planning and tuned when they drift.
9. **Scope of the Ultan migration (fd-perception PARADIGM-001).** If layer comes to mean "how
   fast it should move" rather than "where it lives", positions will change meaning. Pins must
   survive a cell reassignment, or be explicitly reset.
10. **What becomes of the TUI door's threads screen** once the BB page covers the same ground
    (F8).
