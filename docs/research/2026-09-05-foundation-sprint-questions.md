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

### Active goal: finish the product brainstorm with the user

The user reaffirmed: “please continue with brainstorming all the
ambiguity/design/product decisions as your next goal with me”.

Continue the dialogue through the consequential product, design, and
interaction decisions for the agreed project collaborator and its first
proving journey. For each decision, provide a concrete scenario, explain
meaningful tradeoffs, ask through the structured question tool, and record the
answer with its scope. Carry existing rulings forward instead of reopening
them. Recommendations and unanswered questions remain explicitly provisional.

The brainstorm is ready to conclude when the consequential choices for that
journey are answered, or the user explicitly agrees to defer a choice with
clear boundaries, and the user agrees to the resulting design synthesis.
New ambiguities discovered during that synthesis return to the dialogue.
This goal does not itself advance the sprint into implementation planning.

Track this work in `Sylveste-fuwn`. The thread goal widget still contains the
older unfinished daily-catch-up goal and cannot replace it through the
available goal-creation tool. Do not mark that goal complete merely to change
the widget.

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

Within those areas, the remaining dialogue must also cover:

- The first visit and return visit: what appears first, how the collaborator
  is opened, and how conversation, documents, and project status share space.
- Agent/model choice: who selects the runtime and model, whether switching
  is suggested or automatic, and what happens when a chosen model is unavailable.
- Context: project boundaries, inherited or cross-project evidence, and what
  the human can inspect, correct, or exclude from a handoff.
- Draft review and persistence: what is editable, what constitutes agreement,
  how changed decisions supersede earlier ones, and how source conflicts are
  resolved without erasing their provenance.
- Work handoff and return: what starts execution through Clavain, how work
  already running in an external session is represented, and which evidence
  establishes the result.
- Interruption and recovery: pending questions, declined or deferred answers,
  stopping work, reconnecting to sessions, and unavailable agents.
- Attention and resource preferences: visible progress, notifications,
  background activity, and any user-chosen limits on time or spend.
- Success and scope: the pilot project's actual delivered work, the human
  walkthrough, supported project types, and explicit first-slice exclusions.

The collaborator's product role and first proving journey are now agreed;
see the September 5 ruling below. The subsequent Flere assessment also settled
the intended native runtime as Flere with deep Clavain integration. Detailed
interaction, model policy, onboarding completion, and remaining rows stay open.

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

### Tradeoffs and embedded-agent exploration

The user asked to explore the tradeoffs, then asked whether Codex, Pi, or
another custom agent could be integrated inside Autarch to help. This is a
candidate onboarding approach under discussion; the user has not selected an
onboarding completion bar or an agent runtime.

The discussion separated coverage, depth, and file layout. The assistant's
recommendation was to examine every foundation area initially, then deepen
the areas needed for the next milestone while leaving other gaps explicit.
That recommendation remains unconfirmed. The tradeoff is less initial burden
at the cost of needing a useful way to revisit deferred decisions.

Read-only integration research on September 5 found:

- Autarch's [Codex backend](../../pkg/agenttargets/backend_codex.go) invokes
  `codex exec`, streams stderr, and reads a final output file. Its
  [stream contract](../../pkg/agenttargets/stream.go) does not provide a
  bidirectional structured-question response path. This is useful prior art,
  not evidence of embedded Foundation conversation support.
- [Codex App Server](https://learn.chatgpt.com/docs/app-server) is documented
  for embedding Codex in a custom client, with history, streamed events,
  approvals, and thread resume. Structured user-input requests are documented
  as experimental. Protocol capabilities and account/model access need
  validation against the selected installed version before implementation.
- [Pi RPC](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md)
  supports a separate process with JSON messages and events, including
  extension dialogs and correlated responses. Its
  [SDK guidance](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md)
  recommends RPC when integrating from another language. The local Flere
  checkout also contains a Pi-derived agent package and RPC documentation;
  runtime compatibility with Autarch has not been tested.

The product shape explored was a project collaborator presented in Autarch,
powered by a replaceable agent runtime, with Clavain retaining workflow policy
and files retaining project truth. It could inspect existing sources, discuss
contradictions and gaps, prepare drafts, and connect agreed direction to
roadmap and backlog work. Product agreement remains a human act. The user
subsequently agreed to the role and scope described below; no engine was chosen.

Codex offers a direct route to its existing agent behavior; Pi offers a
customizable runtime; building a new agent loop would add substantial scope.
The recommendation is to define the product interaction before selecting an
engine. Switching engines would require an explicit transfer of evidence,
rulings, and open work; provider session histories are not assumed portable.

The earlier original-session answering preference remains the default for
existing external sessions. Embedded onboarding could support direct answers
in new Autarch-hosted sessions; the exact behavior remains to be agreed.

The structured role question led to a comparison of a foundations guide, a
full project collaborator, and an estate-wide coordinator. The recommendation
was refined to distinguish the eventual product role from its first proving
journey. No implementation plan or application code was added.

### Agreed direction — September 5

The user answered **“concurred”** to the recommendation:

> A full project collaborator as the eventual role, with foundation onboarding
> as its first complete journey.

The recommendation the user agreed to includes:

- The collaborator is useful throughout a project, beyond initial setup.
- Autarch provides the product conversation and delegates detailed execution
  through Clavain, bringing back progress, evidence, and decisions needing the
  human. Entering an execution session remains available when useful.
- The first proving journey connects foundation onboarding to one real piece
  of work, testing whether a decision carries through to an outcome.
- Estate awareness can provide shared standards and related-project context;
  it does not grant authority to redirect other projects or their agents.

This agreement does not select Codex versus Pi, authorize automatic model
switching, resolve the earlier onboarding completion question, or advance the
sprint to implementation planning. Those are separate decisions.

Next structured question sent; no answer recorded: should the project
collaborator appear as one continuing conversation with visible agent/model
handoffs, or separate agent conversations linked by explicit handoffs?
Continuity reduces navigation but requires clear explanation of transferred
context; separate conversations preserve agent-native histories more clearly.
Either design carries agreed decisions and source files forward. The
recommendation is one continuing project conversation; it remains unconfirmed.

After the user's request to continue the full brainstorm, the same unresolved
choice was presented as a concrete scenario: discuss mission with Codex, then
switch to Pi for roadmap work. Options are a continuous project conversation
with a visible handoff, a separate linked conversation with a prepared brief,
or choosing whether to continue or branch at each switch. No answer recorded.

## Sprint progress (Sylveste-fuwn)

The user additionally requested assessment of Flere as Autarch's default native
agent. [The Flere assessment](assess-flere-autarch-native-agent.md) found that
raw RPC questions work, while the Clavain execution adapter and restricted
worker are unfinished. After reviewing those findings, the user selected:
**“Flere as intended native default, with deep Clavain integration”.**
The intended runtime and integration depth are now product rulings; runtime
activation remains subject to the onboarding-to-delivery proof.

The next structured question compares a visible guided sprint, with
conversation alongside artifacts and decisions, against a conversation-led
experience with sprint details opened on demand. No answer recorded. The
earlier conversation-continuity decision also remains unresolved; distinguish
model changes within Flere from handoffs between agent runtimes.

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
