### Findings Index
- P0 | S1 | "The rail" | Agent-authored proposal text is rendered as live HTML with embedded action buttons — a prompt-injection-to-click primitive
- P1 | S2 | "The rail" | "Answer", "Approve", "Authorise" and "Approve bootstrap" share one visual/interaction pattern despite very different authority levels
- P1 | S3 | "The rail" | No audit trail is designed for approvals or overrides, including "Override & approve"
- P1 | S4 | "The rail" | Bootstrap creates real external resources (private repo, CI onboarding) behind a single click, with no review step between draft and creation
- P1 | S5 | "Order of work" | Approval-driven dispatch (T1) is scheduled before the one-coordinator claim exists, and bbDev/bbOps coordinators already run outside Mycroft's suggestion flow
- P2 | S6 | "The rail" | Composer defaults to "whole estate" scope, widening blast radius of an ambiguous prompt
- P2 | S7 | "The service plus a BB plugin" | Remote exposure through BB Connect is not addressed as a distinct threat surface for the approve/override actions

Verdict: needs-changes

## Summary

This is a discover-stage brainstorm with two exercised HTML prototypes; nothing is built, so no finding here blocks continued design work. But the design's central mechanism — a rail where agent-authored text (questions, proposals, holds) is rendered inline and answered/approved with one click, reused for authority ranging from "reply to a question" to "create a private GitHub repo and wire CI" to "override a blocker" — has a trust-boundary problem that the current refusals list does not cover. The prototype (`autarch-map-rail-v2.html`) confirms the concern is not hypothetical: agent-generated strings are injected via `innerHTML` together with live `<button data-approve>`/`<button data-bootok>` markup, so the rendering path already treats agent output as trusted UI, not as untrusted content to escape. Before this reaches an implementation plan, the design needs: (1) a rule that agent-authored text is rendered as inert text, never as markup with embedded actions; (2) visually and mechanically distinct approval tiers (answer / approve-dispatch / authorise-bootstrap / override) so a single reflexive click cannot cross authority levels; (3) a persisted, non-bypassable audit record for every approve and every override, distinct from the prototype's ephemeral toast; and (4) a stated claim mechanism gating T1 dispatch, not only autonomous tiers, given that unmanaged coordinator threads already exist.

## Issues Found

### S1 — P0 — Agent-authored proposal text is rendered as live HTML with embedded action buttons

**Evidence:** In `autarch-map-rail-v2.html`, Mycroft's proposal and rail item text are inserted via `innerHTML` and directly embed action controls inside the same string as the agent's own words, e.g. line 280:
```
say('mycroft',`Proposal for <b>${name}</b>: place in <b>Tools & personal · Probe</b> ... I'd run <code>clavain:project-onboard</code>: private repo, PROVISIONAL product card drafted from your line, beads + baseline checks, zklw-ci onboard. <br><button class="primary" data-bootok>Approve bootstrap</button> <button data-bootno>Change placement</button>`)
```
and line 252, the "Override & approve" button is produced inside the same templated string as the hold explanation (line 142: `hold:'intercore is blocked upstream (ic provenance). Mycroft holds this dispatch until it clears or you override.'`).

**Failure scenario:** The rail's stated job is to show "an agent-authored proposal, or text inside a question" (per the brief) — this is exactly what's rendered. If any upstream input that reaches a proposal's text field is attacker-influenced (a compromised MCP tool result, a malicious README summarized into a bootstrap proposal, a crafted bead title, or simply a buggy/compromised dispatched agent), that text is not just displayed, it is markup with live handlers in the same trust zone as the composer's own buttons. A crafted string can: (a) relabel or duplicate the real approve/override button to mislead mk about what a click does, (b) suppress or bury the `hold` text that explains why an override is dangerous, or (c) since BB plugins are described as full-trust and the page is served through BB Connect, potentially inject markup that isn't just visual (an `onclick`, an `<a>`, or embedded script if any future revision loosens the string-to-DOM path) — the current build already treats agent strings as HTML fragments, so the sanitization boundary that would prevent this does not exist yet even in prototype form. This is the concrete instance of "text inside a question tricking mk into approving something."

### S2 — P1 — Authority is conflated across one button style

**Evidence:** The same `class="primary"` single-click button pattern is used for: "Answer" (reply to a question, `bb thread interactions answer`), "Authorise" (bd init/bootstrap, line 128), "Approve" (dispatch a Mycroft suggestion, claims a bead and spawns a thread, line 129/303), and "Approve bootstrap" (creates a private repo, drafts a product card, runs `zklw-ci onboard`, line 280/309). "Override & approve" (line 252) is the same button with a relabeled string when a `hold` is present — not a separate, harder-to-reach control.

**Failure scenario:** mk operates the rail at pace, clicking through an escalation queue ordered by "who is stalled." A user trained by dozens of low-stakes "Answer" clicks will apply the same reflexive click to "Approve bootstrap" or "Override & approve," which are irreversible-adjacent (repo creation, CI wiring, bypassing an upstream blocker) rather than conversational. The design explicitly distinguishes "approving a dispatch" from "what the dispatched agent later does" only in prose ("Result returns as a review gate") — the UI gives no such distinction at click time.

### S3 — P1 — No audit trail for approvals or overrides

**Evidence:** The prototype's only record of an approval is `toast(...)` (lines 302-303, 309) — a transient UI notification, not a persisted record. Neither the Key Decisions nor Open Questions sections mention logging who approved what, when, under which hold/alternatives context, or recording the override reason. Compare to the design's own citation of `gastown-severity-routed-escalation` (human at top of escalation ladder) — that pattern typically presumes the escalation and its resolution are recorded.

**Failure scenario:** "Override & approve" bypasses an explicit upstream blocker (e.g., "intercore is blocked upstream ... Mycroft holds this dispatch until it clears or you override"). Without a durable, queryable audit record distinct from the dispatched thread's own logs, there is no way to later answer "why did this run despite the block" or to detect a pattern of overrides masking a systemic problem (e.g., the blocker check itself misfiring). This is also the accountability backstop for S1 and S2: if a misleading click did occur, the audit trail is what would surface it.

### S4 — P1 — Bootstrap creates real external resources behind one click, no review gate between draft and creation

**Evidence:** Line 280: a single "Approve bootstrap" click triggers, in sequence, `clavain:project-onboard`: a private repo, a PROVISIONAL product card "drafted from your line," beads, baseline checks, and `zklw-ci onboard`. The product card is described as PROVISIONAL (i.e., agent-drafted, not yet verified) but is created together with the repo and CI wiring in the same action, not reviewed before the repo exists.

**Failure scenario:** This conflicts with the project's own global policy ("create repos private... require review gates and human acceptance") in spirit if not in mechanism: the repo and CI onboarding are real, hard-to-fully-undo side effects (a GitHub repo now exists, even if private; `zklw-ci onboard` registers the project into fleet automation) triggered from a drafted, unverified description. If the placement or name Mycroft inferred is wrong (a plausible failure given Open Question 1: "Gardens without a row need a stated fallback region, not a guess"), mk has already created the artifact before seeing the final card, only able to "Change placement" after the fact via composer text, not via undoing the repo/CI action.

### S5 — P1 — Approval-driven dispatch is scheduled before the coordinator claim exists

**Evidence:** Decision 8: "first a read-only attention and outcomes view, then approval-driven dispatch at T1. Autonomous tiers come only after a one-coordinator claim exists." Open Question 5 independently flags that `bbDev | coordinator` and `bbOps | coordinator` threads already exist, with no claim mechanism yet.

**Failure scenario:** Human approval at T1 is not itself a claim mechanism — it only gates whether mk consents to a single proposed dispatch, it does not prevent two independently-running coordinator threads (bbDev, bbOps) or two separately-timed rail suggestions from targeting the same bead/garden. mk approving a "suggest" card in the rail has no visibility into whether `bbDev | coordinator` is concurrently acting on the same target, since those coordinators are shown only as generic "active" threads (line 145), not cross-referenced against Mycroft's suggestion queue. Sequencing T1 dispatch ahead of the claim mechanism risks double dispatch (two agents editing/claiming the same bead) well before "autonomous tiers," contrary to the apparent intent of the ordering.

### S6 — P2 — Composer defaults to whole-estate scope

**Evidence:** Line 264: `${scope?scope:'whole estate'}` — when no garden is selected, the Mycroft composer's scope is the entire ~98-garden estate.

**Failure scenario:** An ambiguous or hastily-typed composer prompt sent with no selection is interpreted estate-wide rather than rejected or requiring explicit scope confirmation, widening blast radius for what may have been intended as a narrow request. Lower severity because the resulting action still requires a subsequent approve click and produces a proposal for review, not direct execution.

### S7 — P2 — Remote exposure via BB Connect not addressed as a distinct surface

**Evidence:** Per the task brief, Autarch is accessed by mk remotely through BB Connect at a public URL. The brainstorm treats "local-only by default" as satisfied by the underlying Autarch/Mycroft service binding to loopback, but the actual interactive surface (the approve/override/bootstrap buttons) is reached through BB's own remote session, not through the loopback service directly.

**Failure scenario:** This is not a new hole in BB's auth model, but it changes the calculus for S1-S4: any UI-redress or button-relabeling achieved via S1 is now reachable over the public BB Connect URL rather than only from a local terminal, and a stolen/replayed BB session would grant the same one-click repo-creation and override authority described in S3/S4. The document doesn't state whether BB Connect enforces step-up auth (e.g., re-confirmation) for the highest-authority actions (bootstrap, override) versus routine answers — worth an explicit ruling given the stated local-only-by-default policy is about the *service*, not this *session*.

## Improvements

1. Render agent-authored proposal/question text as escaped text nodes only; keep action buttons in a separate, statically-templated region of the DOM that the agent's string can never reach syntactically. This closes S1 without waiting for a "which model produced the text" trust argument.
2. Tier the rail's buttons by authority, not just by color: "Answer" (reply) stays low-friction; "Approve" (dispatch), "Authorise" (bd init), "Approve bootstrap" (repo+CI creation) and "Override & approve" (bypass a blocker) should require a distinct confirmation affordance (e.g., a second click on a named consequence, or a short type-to-confirm for override/bootstrap) so the click cost scales with blast radius.
3. Persist every approve/override as a record independent of toast/local UI state — who, what proposal text and hold reason, timestamp, and the resulting thread/repo/bead ID — queryable later, satisfying the design's own escalation-ladder lineage.
4. Split "Approve bootstrap" into "review the drafted product card" and "create repo + onboard CI" as two gated steps, or at minimum show the final drafted card before any external resource is created.
5. State explicitly whether T1 approval-driven dispatch is itself gated on a claim/lock (even a lightweight one, e.g. a bead field) before it ships, rather than deferring all claim-mechanism work to "autonomous tiers." Given bbDev/bbOps coordinators already exist and run outside this flow, the risk is present at T1, not just beyond it.
6. Add an explicit ruling on BB Connect session requirements (e.g., step-up confirmation) for bootstrap/override actions specifically, distinguishing them from read/answer actions.

```
--- VERDICT ---
STATUS: warn
FILES: 0 changed
FINDINGS: 7 (P0: 1, P1: 4, P2: 2)
SUMMARY: The rail's core mechanism renders agent-authored text as live actionable HTML and reuses one button pattern across answer/approve/authorise/override/bootstrap, so a compromised or careless proposal string can mislead an approval and a reflexive click can cross authority levels with no audit trail; findings are addressable in the write-plan stage before any code ships.
---
```
<!-- flux-drive:complete -->
