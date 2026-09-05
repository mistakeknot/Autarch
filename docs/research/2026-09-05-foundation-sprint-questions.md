---
artifact_type: research
status: dialogue-in-progress
bead: Sylveste-fuwn
date: 2026-09-05
---

# Project foundation onboarding: sprint research and open decisions

## User direction

“let's treat this as a clavain sprint, starting with brainstorming, so we can
clarify all unknowns/ambiguities before creating a plan; let's proceed with
this as the next goal”

The next goal is to establish how Autarch helps each project develop and
maintain its mission, vision, philosophy, personas, critical user journeys,
roadmap, architectural decisions, backlog, and design standards. The user
wants consequential ambiguities clarified before a new implementation plan.
The existing Foundation view and September 4 pilot draft are inputs to that
discussion, not approval of a target workflow.

This is an in-progress research record, not a completed brainstorm or plan.
Questions use the available structured question tool, one decision at a time.

## Verified starting point

Source baseline: Autarch main at `51905a7`.

- [Foundation discovery](../../internal/door/foundation.go) inventories nine
  areas and prepares an agent brief. Source presence does not establish
  approval, freshness, completeness, or successful onboarding. One source may
  cover several areas, and shared or custom sources may exist outside the
  scanner's conventions.
- [The delivery verification](2026-09-04-foundation-verification.md) records
  that canonical authoring, approval, agent launch, and estate rollout remain
  outside the delivered slice.
- Clavain's `project-onboard` skill already combines repository introspection,
  an evidence-prefilled interview, mission, philosophy, conventions, and
  personas with Interpath generation of vision, PRD, roadmap, and CUJs.
  Its current exceptions, such as skipping CUJs for some libraries, are
  existing behavior to evaluate rather than approved product requirements.
- Interpath's `artifact-gen` skill already connects product artifacts to
  project context and Beads. Evaluate that integration before adding another
  independent generator.
- [Gurgeh onboarding](../../internal/tui/onboarding.go) has an older
  interview-to-spec-to-epics-to-tasks flow. It provides prior art, but its spec
  scope and orchestration role do not settle this onboarding journey.

## Decisions to preserve

- [The confirmed product card](../why.md) establishes the solo gardener
  persona, context-loss pain, daily-walk journey, and source-of-record
  boundary. Its project-wide success measure remains deliberately declined.
  A proposed onboarding acceptance test must not silently fill that field.
- [The September 3 rulings](2026-09-03-ultan-nativity-thesis.md) establish that
  files are truth, agents propose, and the human rules. No new graph is needed
  to resolve the onboarding product questions.
- [The card contract](../reference/product-card-format.md) distinguishes
  provisional evidence-backed drafts from human confirmation, including
  recorded unknowns and declined decisions.
- [The architecture](../autarch-vision.md) places policy-governing
  orchestration and model routing in Clavain; Autarch presents the human
  interface. The interaction and integration details remain open.
- The user selected showing questions and supporting context in Autarch,
  then opening the original session to answer. Preserve this default while
  clarifying how switching models affects continuation.
- The user selected both Cozy and Compact presentation options. That choice
  does not settle the onboarding workflow or required artifact depth.
- [The wider vision](2026-08-31-vision-capture.md) includes an estate beyond
  software repositories alone. This sprint must clarify its initial project
  scope without silently replacing that vision.

## Open decisions

| Decision | What needs clarification |
|---|---|
| Meaning of onboarded | Required substance and connections; acceptable unknowns; whether separate documents matter. |
| Sprint outcome | Working capability, actual pilot projects, or estate-wide onboarding; the decision this should make easier. |
| Project diversity | Apps, libraries, infrastructure, research, creative projects, and non-repository interests; applicability rules. |
| Interaction | How Autarch presents drafts, questions, source sessions, and progress through a Clavain-led workflow. |
| Review granularity | Whole-foundation review versus individual consequential claims; how to handle contradictions and declined answers. |
| Shared foundations | How ecosystem mission, philosophy, and design standards are inherited, overridden, and attributed. |
| Model continuity | When to resume the original session versus use another model; what context and decisions must transfer. |
| Maintenance | Which changes reopen review, and how much interruption is appropriate. |
| Pilot and evidence | First project and contrasting follow-ups; the human walkthrough that demonstrates a useful result. |

An observed source conflict belongs in this discussion: root development
guidance still recommends the older unified four-tool TUI, while the confirmed
product card describes the garden HUD. Source reconciliation should expose
that difference instead of silently blending incompatible directions.

## Current dialogue

First structured question sent; no answer recorded:

**What should count as a project being “onboarded”?** We currently have source
discovery and a working brief; this sprint needs to define the substantive
result beyond that.

1. Every relevant foundation area is agreed, consistent, and connected to next
   work; inapplicable areas have an explicit reason. Recommended, not selected.
2. A small core is agreed first; remaining foundation areas can be completed
   progressively.
3. All nine areas must have separate, complete, approved documents.

Silence does not select an option. After the dialogue resolves the consequential
choices, compare concrete approaches and capture a brainstorm for agreement
before proceeding to strategy or a new implementation plan.

## Sprint progress (Sylveste-fuwn)

- [ ] Step 1: Brainstorm — Understand phase; research gathered, dialogue open.
- [ ] Step 2: Strategy
- [ ] Step 3: Write Plan
- [ ] Step 4: Plan Review
- [ ] Step 5: Execute
- [ ] Step 6: Test & Verify
- [ ] Step 7: Quality Gates
- [ ] Step 8: Resolve
- [ ] Step 9: Reflect
- [ ] Step 10: Ship
