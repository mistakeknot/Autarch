---
artifact_type: onboarding-draft
project: autarch
status: provisional
bead: Sylveste-fuwn
---
# Autarch foundation — first onboarding draft

This is a proposal for review, not a ratification of new mission, vision,
principles, or success measures. Existing confirmed card fields retain their
own provenance. The user's September 4 direction is to focus on getting each
project onboarded to mission, vision, roadmap, philosophy, ADRs, backlog,
CUJs, personas, and design systems/standards.

## Mission — proposed

Help one person keep an agent-heavy estate of projects purposeful and coherent
by making each project's intent, decisions, work, and open questions legible
across sessions.

Basis: the confirmed persona and pain in [the product card](../why.md), the
user's [HUD vision](../research/2026-08-31-vision-capture.md), and the current
onboarding priority. The wording above is newly drafted.

## Vision — proposed emphasis

Entering a project should recover its purpose and context: who it serves,
what experience it promises, which principles constrain it, what comes next,
and which decisions need the human. A new agent or model can continue from
that foundation without requiring the human to reconstruct the project from
old sessions. Moving among projects should feel like tending a connected
garden with different rhythms of change.

The [August vision capture](../research/2026-08-31-vision-capture.md) supplies
the garden/HUD direction. [The earlier application-layer vision](../autarch-vision.md)
supplies the separation between rendering, policy, and durable state. The
latest user feedback adds project-foundation onboarding as the immediate focus.
These emphases should be reconciled in the existing vision document after review.

## Philosophy — proposed synthesis of existing rulings

- Project files carry the foundation; Autarch reads and presents it. This
  follows the card's confirmed guardrail that Autarch holds no world state.
- A human ruling, an agent proposal, and an unresolved question have different
  authority. Preserve their sources and attribution.
- Recover existing context before asking the human to repeat it. Draft from
  evidence and ask about consequential gaps; see [the card drafting rules](../reference/product-card-format.md).
- Connect intent to a user journey and a concrete next unit of work. A pile of
  documents is insufficient evidence of that connection.
- Keep the foundation available across agents and models. Preserve the
  original evidence when a different model takes over.
- Keep the daily walk quiet. Onboarding is entered deliberately, and returning
  to a project should not create an obligation to clear an inbox.

## Persona — reuse the confirmed card

The primary persona is the solo gardener of many interrelated, agent-heavy,
terminal-native projects. Their problem includes losing context across
compacting sessions and becoming the manual carrier of context between projects.
This is already stated and confirmed in [docs/why.md](../why.md); it does not
need a fresh invented persona. A separate persona document should add useful
detail only if the onboarding review needs it.

## Critical user journey — onboarding proposal

**Actor:** the existing solo-gardener persona.
**Trigger:** a new project, a project without a clear foundation, or a return
to work that exposes conflicting direction.

1. Choose the project and read its existing foundation sources.
2. Recover relevant prior decisions and identify contradictions or missing context.
3. Have the selected agent prepare cited provisional drafts and a small batch
   of product/design questions.
4. Resolve those questions; record the human's rulings in the project files.
5. Connect the next roadmap outcome to a persona, journey, and bounded backlog work.
6. Refresh Autarch and verify that the agreed foundation is accessible to the
   next agent session.

**Proposed recognition condition:** the human and an incoming agent can explain
why the next unit of work matters, who it serves, and which principles and
decisions constrain it without reconstructing the project from old chats.
This is a journey proposal, not a confirmed project-wide success measure.

The four existing [tending-cadence journeys](../cujs/README.md) retain their
existing scope and validation declarations.

## Roadmap — proposed sequence under the new priority

1. **Autarch pilot:** resolve this draft's open calls and reconcile the existing
   sources. The Foundation view and portable brief support that work now.
2. **Representative projects:** apply the same conversation to a small set with
   different shapes, such as an app, a tool/library, and a research project.
   Establish how shared philosophy and design standards are referenced.
3. **Estate rollout:** onboard the remaining projects in deliberate batches,
   preserving existing decisions and carrying unresolved questions visibly.
4. **Continuity:** make newly accepted decisions and changes in direction flow
   back into the relevant roadmap, journeys, backlog, and agent context.

This sequence does not silently replace the existing dated roadmap. The
foundation pilot should reconcile it with the user's new priority explicitly.

## Architecture decisions — reuse and identify actual gaps

Reuse the decision references in the card and the existing ADR directory.
The current implementation has chosen a derived inventory and a portable
brief, with no automatic authoring or agent launch. Provider dispatch and
where to record inherited-source references still require a concrete design.
Write additional ADRs when those decisions are made, including alternatives
and consequences; do not backfill fictitious decisions.

## Backlog — current link

`Sylveste-fuwn` tracks this onboarding focus. Its immediate next unit is to
resolve the Autarch pilot's open calls, then record the reviewed foundation
in the appropriate existing project sources. The original catch-up work and
the separate native-graph epic retain their distinct acceptance criteria.

## Design systems and standards — proposed consolidation

Retain the existing Bubble Tea/Lip Gloss stack and Tokyo Night palette from
[AGENTS.md](../../AGENTS.md), plus the user-selected Cozy/Compact alternatives.
Carry forward keyboard access, visible controls, bounded reading areas, and
explicit distinction between source evidence and proposed work.

The project already has guidance in [agents/conventions.md](../../agents/conventions.md)
and `docs/tui/`. Some older guidance assumes a chat-first layout, while the
current daily catch-up serves quiet orientation. Onboarding should reconcile
that scope difference and name the applicable standards instead of declaring
that the project has no design guidance.

## Open calls for the human

- Does the proposed mission capture the project as a whole, including the
  longer-term garden experience, or overemphasize project management?
- What project-wide outcome would show that this is working? The existing
  success field is deliberately declined; it remains so until the user rules.
- Which principles/design standards are shared across projects, and where
  should project-specific departures be recorded?

No other project's canonical documents were changed by this pilot draft.
