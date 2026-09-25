# Synthesis Report: Autarch Attention Map Brainstorm Review

**Review Date:** 2026-09-23  
**Stage:** Discover (brainstorm + prototype)  
**Run UUID:** ca9b9839-7e95-4b79-9b7d-971f282e8a91  
**Protocol Version:** 1.0

---

## Verdict Summary

| Agent | Findings | Verdict | Status |
|-------|----------|---------|--------|
| fd-architecture | 7 (2 P0, 3 P1, 2 P2) | needs-changes | NEEDS_ATTENTION |
| fd-decisions | 8 (0 P0, 4 P1, 4 P2) | needs-changes | NEEDS_ATTENTION |
| fd-perception | 7 (0 P0, 4 P1, 3 P2) | needs-changes | NEEDS_ATTENTION |
| fd-safety | 7 (1 P0, 4 P1, 2 P2) | needs-changes | NEEDS_ATTENTION |
| fd-systems | 6 (0 P0, 3 P1, 3 P2) | needs-changes | NEEDS_ATTENTION |
| fd-user-product | 9 (1 P0, 3 P1, 5 P2) | needs-changes | NEEDS_ATTENTION |

**Overall Verdict:** `risky` (4 P0 findings across 3 agents)  
**Gate Status:** `FAIL`

---

## Critical Blocking Issues (P0 — Gate Failures)

### 1. Prompt Injection via Trusted Rendering (fd-safety S1)

Agent-authored proposal text is rendered as live HTML with embedded action buttons via `innerHTML`, creating a prompt-injection-to-click primitive. In the prototype (`autarch-map-rail-v2.html:280,252`), Mycroft's proposal text and hold explanations directly embed `<button>` controls inside templated strings, allowing a compromised proposal to relabel or hide approval controls.

**Reactions:** fd-architecture agrees with partial independent coverage; adds evidence that this is a concrete failure mode of the missing Go/TypeScript data contract.

**Blocking:** Cannot merge without rule that agent text is inert content only; action buttons embedded separately.

---

### 2. State Boundary Violation (fd-architecture F1)

The design rule "Autarch holds no state" is contradicted by existing code (`internal/door/preferences.go`, `internal/door/display.go`, `internal/mycroft/escalate/escalate.go:DecisionQueue`). The brainstorm then proposes five new state items (pins, last-visit stamps, seen-glow, weighed alternatives, composer history) without per-item ruling on ownership.

**Blocking:** Cannot plan state allocation without first resolving which existing state is "preference" (allowed) vs. "held" (forbidden).

---

### 3. Dispatch Substrate Rewrite Not Named (fd-architecture F2)

The brainstorm states "Mycroft coordinates agents as BB threads" but existing code spawns agents purely through tmux sessions (`internal/mycroft/spawn/spawner.go:40`, `internal/mycroft/patrol/source.go`), with zero BB-thread integration. This is a rewrite of Mycroft's dispatch substrate, not an extension, and scope is never named.

**Reactions:** fd-safety agrees; elevates to threat surface concern (rail approve/override buttons have no verified path to invoke spawning). fd-decisions agrees as ordering dependency.

**Blocking:** Cannot schedule T1 dispatch approval or shape the rail's call path without first resolving dispatch substrate.

---

### 4. Data Incompleteness Undermines Core Job (fd-user-product F1)

Map placement data is real for only 51/98 gardens; core-doc shelf data for 20/29 named gardens. At launch, most positions/rings will be fallback or unscanned, directly undermining the map's #1 job (gestalt/spatial memory) and the trust needed for daily use.

**Supporting Evidence:** fd-perception convergence on MAP-001, GRAPH-001, DOCS-001; timeline peer-findings.

**Blocking:** Cannot ship spatial memory system without confidence signals or data-completeness SLA.

---

## Important Issues (P1 — Needs Changes)

### Coordination & Escalation Gaps
- **Two escalation queues** (fd-architecture F4, fd-safety S5): in-process `DecisionQueue` (Go/TUI) vs. BB-plugin rail queue; approval in one won't clear the other; double-dispatch at T1.
- **Three coordinator threads** (fd-systems MULTIPLE_COORDINATORS_AMBIGUITY): bbDev, bbOps, Mycroft composer with no priority rules; approval from one + hold from another = rejection loops.

### No Success Criteria or Exit Mechanisms
- **No graduation test** (fd-decisions NO-SUCCESS-CRITERIA, fd-user-product F2): "Earn its space" is aspirational; no metrics or transition rule.
- **No kill criterion** (fd-decisions MISSING-KILL): "If unused by X date, we stop" is never stated.
- **Three loops lack throttles** (fd-systems ESCALATION_FEEDBACK, ESTATE_SPRAWL, NEGLECT_SIGNAL_DECAY): escalation load amplifies, bootstrap sprawl unbounded, neglect signal offers no recovery.

### Scope & Frame Issues
- **All four jobs selected with no prioritization** (fd-decisions ALL-OF-ABOVE): Only attention is a daily concern; three others are seasonal, not persistent.
- **Composer duplicates BB's interface** (fd-architecture F5, fd-user-product F4): No stated reason to exist.
- **Data model issues** (fd-perception UNTOUCHED-001): Thread activity ≠ tending; neglect signal is noisy.

---

## Reaction Analysis

**18 reactions across 6 agents:** 13 agree, 2 partially-agree, 3 distinguish/refine; 0 disagreements.

**Convergence Pattern:** Multiple agents independently flagged:
- Coordinator-claim & escalation-queue gaps (fd-architecture, fd-safety, fd-systems via code, threat, dynamics lenses)
- Dispatch-substrate mismatch (fd-architecture, fd-safety, fd-decisions)
- Data-completeness gaps (fd-user-product, fd-perception, timeline)

This is **systemic document-wide validation**, not echo-chamber agreement.

**Lorenzen Move Validation:** All defense moves cite evidence (valid). All distinction moves specify boundaries (valid). One reactive addition within 2-per-agent cap.

---

## Sycophancy Analysis

**None detected.** All agents engaged substantively:
- fd-architecture reactions strengthen peer findings with code-level evidence
- fd-decisions reactions reframe from decision-process lens
- fd-perception reactions add independent information-quality observations
- fd-safety reactions extend peer findings with threat-surface implications
- fd-systems reactions relate findings to feedback-loop dynamics
- fd-user-product reactions cross-validate from user-flow lens

**Conformity rate:** 100% engagement, but all reactions add independent framing or evidence.

---

## Stemma & Convergence Groups

| Group | Findings | Convergence | Root Cause |
|-------|----------|-------------|-----------|
| Data Contract | S1, F6 | 2 agents | Go/TypeScript boundary undefined; no schema to enforce trust rules |
| State Boundary | F1 | 1 agent | "Holds no state" rule contradicted by code; five new items unowned |
| Dispatch Substrate | F2, F4, S5 | 3 agents | Tmux vs BB threads; substrate rewrite unscoped; coordinator ambiguity |
| Data Completeness | F1, MAP-001, GRAPH-001, UNTOUCHED-001 | 4 agents | 51/98 gardens placement missing; 5 edges vs ~98 gardens; activity as proxy fails |
| Feedback Loops | ESCALATION_FEEDBACK, ESTATE_SPRAWL, NEGLECT_SIGNAL_DECAY, MISSING-KILL | 4 agents | Three loops amplify without throttles, completion signals, or recovery paths |
| Scope & Frame | ALL-OF-ABOVE, F5 | 2 agents | Four jobs without ranking; three are seasonal not daily; composer unexplained |

---

## Diverse Perspectives

**fd-architecture:** "The map+rail idea is sound, but architectural connective tissue is missing—which state lives where, which packages get rewritten vs. extended. Fix that and most other findings resolve."

**fd-safety:** "The rail's central mechanism (agent text + one-click actions ranging from 'reply' to 'create private repo + CI') has a trust-boundary problem. Without a schema and audit trail, this is a hand grenade with an unpulled pin."

**fd-user-product:** "Ship the one job (attention) that works with the data we have. The other three lenses are seasonal concerns; three of four are scope creep. The estate's positions are missing for half the projects—spatial memory system needs confidence signals."

**fd-systems:** "Three loops are running open. In isolation each is a design choice; together they create a system that accumulates obligations without exit, which collapses into fatigue. Add balancing mechanisms at every scale."

**fd-decisions:** "The document makes four structural choices under time pressure, anchored by agent-recommended options mk accepted as-is. Add a kill criterion, a success metric, and a decision record for why this frame works. 103 prior plans with no recorded verdict—don't repeat mistakes."

**fd-perception:** "The document narrates a single confident model (BB threads, GraphSource, place-based layers) without surfacing divergence from reality—tmux vs BB, unbuilt plan vs interface, semantic drift at Ultan. Build memory system on solid foundation; verify the map against reality first."

---

## Files at Risk

- `internal/door/preferences.go:16` — State boundary violation (P0)
- `internal/mycroft/spawn/spawner.go:40` — Dispatch substrate (P0)
- `internal/mycroft/escalate/escalate.go:65` — Queue ambiguity (P1)
- `autarch-map-rail-v2.html:280,252` — HTML injection (P0)
- `docs/plans/2026-09-03-layer-view-plan.md` — WI-1/2 unimplemented (P1)
- `internal/tui/views/mycroft.go:37` — Dispatcher/queue conflict (P1)

---

## Summary

The brainstorm's core idea—a spatial map + escalation rail with multi-lens views—is sound and well-grounded in prior art. The design is built on three shaky foundations:

1. **Missing Connective Tissue:** "Autarch holds no state" is false; "Mycroft uses BB threads" assumes a rewrite; "GraphSource" is unbuilt. Document treats all as settled fact.

2. **Insufficient Data:** Placement/neglect data are real for half the estate. Core job (gestalt/spatial memory) is undermined unless confidence signals are added.

3. **Unbounded Loops:** Three loops (escalation, bootstrap sprawl, neglect decay) amplify without throttles, completion signals, or recovery paths. System will starve under load.

**Gate:** FAIL — Four P0 findings. Plan should reconcile model (document) against substrate (code), establish data-completeness gates and success criteria, add kill/pivot triggers and loop throttles.

**Recommended Next:** Use write-plan to address all P0 issues. Defer three seasonal lenses (allocation, dependencies, neglect-fade); ship attention lens first once foundational questions are resolved.
