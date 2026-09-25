---
artifact_type: onepager
distills: docs/brainstorms/2026-09-22-autarch-attention-map-brainstorm.md
---

# Autarch in BB: estate map + Mycroft rail

**Thesis.** mk runs about 98 gardens through agents in BB. An escalation rail says what needs mk;
a persistent estate map shows what the estate is doing around those asks. Mycroft coordinates the
agents as BB threads, and anything that needs mk climbs to the rail. The map has to *earn* its
place, and a two-week trial decides whether it does.

**How it works.**
- The page is a BB plugin backed by an independent Autarch/Mycroft service. BB owns threads,
  beads owns tasks and Mycroft's claims, and Autarch is an **overlay**: it never owns world truth,
  and deleting `~/.autarch` makes nothing in the world wrong.
- **Map:** fixed regions, with the pace layer on x and the ecosystem on y. You can nudge a garden
  within its cell and it stays pinned. Every placement, edge and document shows known, guessed or
  unknown.
- **Cadence:** attention is the only default lens. A live blocker automatically draws its
  known-provenance downstream edges. Allocation, dependencies and neglect are on-demand lenses,
  and they serve as the seasonal visit.
- **Zoom:** entering a garden shows its core-document shelf, its outcomes and beads, its threads,
  and Mycroft's proposal with the alternatives it weighed.
- **Rail:** the escalation queue plus a composer that works as a **router**. Changes become
  proposal cards (claim, approve, BB thread); questions get one-shot evidence cards. Agent text is
  inert. Approval weight scales with blast radius, and every approval is recorded with its beads
  claim.

**Lineage.** It builds on the walk (autarch-01), pace layers (autarch-03), the Gas Town escalation
ladder, and the one-map-many-lenses design of Shadow Empire and EU5. The rail stays next to the
map, following Shadows of Forbidden Gods. The Meadowsyn experiments are available as rendering
studies. The overlay rule restates autarch-01's "the Mayor lost its data 22 times being a world".

**Refusals.**
- No drag-to-dispatch.
- No chat state in Autarch.
- No world truth in Autarch.
- No confident rendering of guessed data.
- No dispatch surface of any kind before the beads claim exists.
- No scrub-back replay in v1.

**Top three open calls.**
1. The budget numbers per layer. The loops themselves are ruled: per-layer proposal budgets,
   probe expiry, "parked" as the single resting state, and one queue with layer tags.
2. Where placement and edges come from beyond CanonGraph's 51 rows and 5 edges.
3. How pins behave when the Ultan migration changes what a layer means.

**Status.** Discover stage, revised after a flux-drive review (verdict: risky, with the contested
findings ruled on 2026-09-23). The prerequisites are named as work: `GraphSource`, a BB-thread
spawner and source for Mycroft, the Go↔TypeScript data contract, and the beads claim. Next is the
read-only trial slice with the caught-something log.
