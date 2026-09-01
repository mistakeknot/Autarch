# Estate ontology — three axes, one missing object

```yaml
artifact_type: capture
evidence_class: primary (mk verbatim, 2026-08-31, mid-CUJ-walkthrough)
proposal_verbatim: >
  "hmm, let's think more about the organization of this; i feel like it's
  ecosystems -> platforms -> projects -> themes -> epics -> subtasks and then
  the salons are different pivot views of the themes?, but i'm not sure"
rulings:
  - "themes are a cross-cutting lattice, not a tree level (mk chose decisively
    via structured question; the overall shape remains 'i'm not sure' — treat
    the lattice ruling as firm, the rest as working model)"
  - "the estate surface has no name of its own — bare `autarch` opens the
    vantage; 'door' retired from product vocabulary (mk, same conversation)"
```

## The model (refined against mk's real estate)

mk's six levels are **two trees and a lattice, glued**:

- **Containment tree** — ecosystem → platform → project. Where things live.
  Files cleanly: jawnverse and interverse as ecosystems; Clavain+intercore,
  CanonGraph, the garden-salon substrate as platforms; the ~98 gardens as
  projects. Strain case: Sylveste (ecosystem, or container of ecosystems?) —
  suggests ecosystem/platform may be **roles on containers** rather than
  rigid levels. OPEN, low stakes.
- **Theme lattice** — first-class objects that *reach into* any projects and
  platforms they concern. RULED: cross-cutting, never nested under one
  project. Every real theme tested spans projects (house-style/voice, canon &
  memory lanes, the uncrancher×After-Them alignment model, "agents in the
  tmux estate"). Epics *live in* a project's tracker but can *serve* a theme
  — edge, not containment.
- **Work tree** — epic → subtask. Already fully modeled by beads; the estate
  model federates it, never re-models it.

Plus two attribute axes, not levels:

- **Pace layer** is a *property* of nodes (containment says where, pace says
  how fast, serving says what depends on what). Platforms are usually slow
  and probes fast, but slow projects (canon) and fast platform experiments
  exist — baking pace into the hierarchy would misfile them.
- **Serving edges** (CanonGraph `serving_map`) stay their own axis.

## Salons = the live view of a theme

Sharpening mk's "pivot views": the **theme is the durable object** (member
projects, canon, open questions, cadence); the **salon is its live view** —
the room while conversation is gathered. A theme persists with no salon open;
a salon without a theme is just chatter. The August archaeology re-read: the
~8–10 standing companion threads were salons that could never close, because
closing one destroyed the theme it was faking.

"Pivot" also applies to the vantage itself: the estate view pivots
**by container** (ecosystems → projects) or **by theme** — the Stellaris
outliner's tabs. This settles the garden-vs-thread axis question at depth:
*both, as two pivots of one estate.*

## Ownership map — who holds what

| Object | System of record |
|---|---|
| Ecosystems, platforms, projects, serving edges, pace properties | CanonGraph (exists today: `projects_in_layer`, `serving_map`) |
| Epics, subtasks | beads (exists today) |
| **Themes + their salons** | **nothing — the only missing object in the estate** |

Candidate homes for the theme object, undecided: a new CanonGraph entity
type; a garden-salon substrate object (fits the salon-interface ruling);
an Autarch-local read-model federating both. OPEN — ask before building.

## Consequences for the CUJs (batched for the post-walkthrough edit pass)

1. autarch-01 prose: "open the door" → "open Autarch"; the vantage gains
   by-container / by-theme pivots; the walk triages themes-with-live-salons
   and projects-with-waiting-agents as two reads of one estate.
2. autarch-02: the dive can enter a garden *or* a theme's salon.
3. autarch-04: cross-garden discourse gets a second router — themes route
   topically (a contradiction inside a theme's membership is high-relevance),
   serving/layer edges route structurally. The two compose.
4. The "which organ first" decomposition question now has a candidate answer
   shape: the theme object may be the briefing's natural unit (a briefing is
   "what moved, per theme, per garden").
