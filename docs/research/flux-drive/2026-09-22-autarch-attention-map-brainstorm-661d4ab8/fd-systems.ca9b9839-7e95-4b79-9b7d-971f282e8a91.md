---
review_type: fd-systems
reviewed_artifact: "Autarch in BB: an estate map and a Mycroft rail"
date: 2026-09-23
findings_count: 6
severity_breakdown: "P1: 3, P2: 3"
---

## Findings Index

- P1 | ESCALATION_FEEDBACK | "What We're Building" | Mycroft proposal-approval loop creates a reinforcing feedback cycle that can amplify escalation load rather than reduce it
- P1 | ESTATE_SPRAWL | "Why This Approach" | Bootstrapping new projects from the rail lowers barrier to entry with no balancing mechanism; Goodhart's Law on "8/8 docs" creates false signal stability
- P1 | NEGLECT_SIGNAL_DECAY | "The map" | Neglect fade and since-last-visit glow settle independently, risking "quiet by design" being indistinguishable from "abandoned"; no recovery path for stalled slow-layer gardens
- P2 | BLOCKER_PROPAGATION_VISIBILITY | "Key Decisions (Decision 8)" | Blocker propagation is named as a focus area but the map surface doesn't guarantee visibility into cascades; nested stalls could hide behind per-layer views
- P2 | PACE_LAYER_CONTEXT_SWITCHING | "Prior art" (autarch-03) | Pace-layer separation prevents context switching, but Mycroft's rail aggregates decisions across all layers with one ordering ("who is stalled"); fast-layer projects can starve slow-layer approvals
- P2 | MULTIPLE_COORDINATORS_AMBIGUITY | "Open Questions" (item 5) | Three existing coordinator threads (bbDev, bbOps, + proposed Mycroft composer) lack explicit priority rules; approval from one and hold from another creates rejection loops and unclear escalation paths

Verdict: needs-changes

---

## Summary

The design elegantly separates watching from decision-making and introduces spatial memory to interrupt fatigue. However, it assumes Mycroft's proposals are passive inputs to the rail, when the feedback structure suggests they become an active load generator. More critically, three reinforcing loops are missing balancing mechanisms:

1. **Mycroft amplifies rather than dampens:** proposal approval → mk acts → creates new state → Mycroft detects new blocker → new proposal climbs rail
2. **New project bootstrap has no estate-size valve:** lower cost → more gardens → more pace layers to coordinate → more blocker cascades
3. **Neglect signal conflates abandonment with peace:** slow-layer gardens that are operating normally fade visually the same way as ones that are genuinely stuck, and there's no mechanism to recover them once the glow settles

The blocker cascade visibility, pace-layer decision pacing, and coordinator contention are open questions, but they are downstream of the feedback structure. Once the feedback loops are visible, the architecture decisions (map vs rail prioritization, proposal generation budgets, glow decay mechanics) become addressable.

---

## Issues Found

### 1. P1 | Mycroft Proposal-Approval Loop: A Reinforcing Cycle
**Section:** "What We're Building", plus the role of Mycroft escalation in autarch-01 and autarch-03

**Evidence & Causal Chain:**
- The design states: "Anything that needs mk climbs to the rail" and "Mycroft coordinates agents as BB threads".
- "New projects are bootstrapped by Mycroft from the rail, reusing `clavain:project-onboard`".
- Mycroft's role across the CUJs: T0 observe → T1 suggest → T2/T3 auto-dispatch, with escalation gates at each tier.

**The loop structure:**
1. mk approves a Mycroft proposal at T1 (e.g., spin up a new garden, delegate a task to an agent)
2. That action creates state: a new garden on the map, a new thread, new work in flight
3. Mycroft observes the new state: new blocker (dependency on the just-created garden), new question (does this garden need a coordinator?), new proposal (should we escalate to a coordinator thread?)
4. The new proposal climbs the rail again

**Why this is a systems issue, not just a detail:**
The design frames the rail as a *queue* to clear, with order determined by "who is stalled". But if Mycroft's proposals create the conditions for new proposals, the rail is not a queue being emptied — it's a queue where each approval adds more items. Under load (98 gardens, fast-layer churn creating new interdependencies), the rail could grow faster than mk can clear it. Worse, the glow that mk experiences is not "overwhelm reduced" but "overwhelm displaced from the map to the rail".

**Second-order consequence:**
If mk feels relief looking at the quiet map (the glow settles, the neglect fades), but the rail grows, mk's actual interrupt load has not decreased; it has moved to a new location. The daily walk's goal — "orientation before obligation" — is threatened by an obligation that now arrives silently, because the map is quiet.

**Missing causal reasoning:**
The one-pager calls this "what needs me climbs to the rail", but it doesn't specify the *rate* at which proposals are generated. A budget or a growth-rate cap on Mycroft proposals would make the feedback visible: "Mycroft can hold N proposals in the rail before T1 dispatch gates open."

---

### 2. P1 | Estate Sprawl: Bootstrapping with No Estate-Size Budget
**Section:** "Why This Approach" (universal interface) and autarch-03 on estate governance

**Evidence & Causal Chain:**
- Decision 1 from the universal interface: "All three failures — finding, remembering, ranking — and they compound."
- The design: "Changes and new projects are bootstrapped by Mycroft from the rail, reusing `clavain:project-onboard`".
- Autarch-03's mental model: "The interruption budget is a first-class tuned number, adjusted when it drifts."

**The loop:**
1. Bootstrapping from the rail lowers the cost of starting a new garden (no manual onboarding, Mycroft handles it)
2. Lower cost → more gardens get started, because "why not add it to the estate"
3. More gardens → more pace layers to coordinate → more potential blockers → more Mycroft proposals
4. More Mycroft proposals → what was a *bounded* estate becomes an *unbounded* one

**Why this is a systems issue, not just a feature creep problem:**
Autarch-03 identifies pace-layer mismatches as a source of overwhelm and lists "sprawl penalties as failed governor" (Stellaris F) as something the seasonal reshaping must address. Yet the bootstrap mechanism makes sprawl *free* to mk in the moment of approval. The seasonal reshaping ("archive, rerank, retire") is designed to correct sprawl *after the fact*, but it runs rarely (seasonal boundaries), while bootstrap runs constantly. The feedback is delayed and weak: mk only sees sprawl consequences when reshaping.

Worse, the "8 core documents" signal (attention, allocation, dependencies, neglect lenses all show document completeness as a proxy for health) creates a Goodhart effect: mk may start gardens with minimal plans to avoid the "8/8" nag, shifting the signal from "health" to "document compliance". The universal interface's ruling 11 ("weakest card first") attempts to make gaps visible, but if mk can bootstrap a garden with a PROVISIONAL card and only visit seasonal reshaping once per season, the garden can grow undocumented for months.

**Missing balancing mechanism:**
The estate needs an explicit size constraint or a decay mechanic. Either: (a) Mycroft can only propose N new gardens per season before requiring explicit approval for more, or (b) gardens that are not visited within T days are automatically marked for archival, creating pressure to maintain engagement with what exists rather than expand infinitely.

---

### 3. P1 | Neglect Signal Decay: "Quiet by Design" Becomes Invisible
**Section:** "The map" and decision 7 ("Time is shown as since-last-visit plus neglect fade")

**Evidence & Causal Chain:**
- Design: "Untouched gardens fade" and "Scrub-back replay goes on the backlog."
- Decision 7: "Time is shown as since-last-visit plus neglect fade. The glow settles once a garden has been seen."
- Autarch-01: "Leaving without acting is a successful walk, not an aborted one."

**The problem:**
1. mk opens the map and sees the since-last-visit glow on gardens that moved
2. mk visits those gardens (the glow settles) or decides to skip them (the glow is now reset by attention, not by action)
3. Meanwhile, a slow-layer garden that is operating *normally* (no urgent needs, steady state) has not been visited in two weeks and is fading
4. The fade is *cumulative*: a slow-layer garden that is at peace in month 1 is darker in month 2, darker still in month 3, until it is visually indistinguishable from one that is genuinely broken

**Why this is a systems issue:**
The glow is meant to surface *change*, and the fade is meant to surface *abandonment*. But the fade has no mechanism to say "this is quiet because it's healthy, not quiet because we forgot". Over time, slow-layer gardens (which autarch-03 says should move at their own pace) become invisible on the map. If mk accidentally forgets about one, the signal for "come look at me" only fires if the garden moves or blocks something upstream. A silent failure in a slow-layer garden could cascade into a crisis before the map notices.

**The temporal asymmetry:**
- Fast-layer gardens get constantly refreshed (new glow) → stay bright → easy to navigate
- Slow-layer gardens fade predictably → harder to see → easy to miss when they do need attention
- The pace-layer architecture (autarch-03) is supposed to let slow layers move slowly, but the visual design biases attention toward fast layers

**Missing mechanism:**
The design needs either (a) a "hearbeat" signal per garden (mk explicitly marks "I checked this and it's at peace"), or (b) a visual distinction between "faded because untouched" and "faded because I checked it and nothing changed". Without it, a slow-layer garden that's humming along normally has the same visual urgency as one that's stuck.

---

### 4. P2 | Blocker Propagation: Visibility Without Certainty
**Section:** Key Decisions, item 8: "The rail doubles as the Mycroft composer" and the dependency lens

**Evidence & Causal Chain:**
- Design: "A blocker upstream visibly dims its dependents."
- Open question 2 (focus): "Blocker propagation holding dispatches: could it deadlock or starve?"
- Decision 8: "The map is an instrument, with approval in place. The estate view is read-only; the only gesture that writes is the pin-nudge."

**The question arises:**
The map shows blocker cascades visually (upstream blocker dims dependents). But the map is read-only. If mk sees a cascade on the map and wants to know *why* project A is blocked, does mk have to (a) zoom into A to see Mycroft's proposal, (b) check the rail to see if A's blocker is listed there, or (c) both? If the cascade is deep (A blocks B blocks C blocks D), and D is blocked at T1 but B's blocker is a slow-layer question waiting for seasonal reshaping, mk may not see the connection from the map view alone.

**Why this is a systems issue:**
The rail is ordered by "who is stalled". But if a deep cascade is stalled at the top layer (A holds B, which holds C, which holds D), and mk needs decisions in D, C, and B to unblock D, the rail may only show D as stalled — not the transitive dependency chain. Mycroft knows the chain (it's marked by blocker propagation), but mk's visibility into *why* D is stalled could be fragmented across the map, the rail, and the zoom view.

More subtly: if three gardens form a cycle (A blocks B, B blocks C, C blocks A), Mycroft may mark all three as blocked but offer no proposal to break the cycle. The cascade is visible, but the resolution path is not.

**Impact on decision velocity:**
If mk needs to see the full cascade to make a decision, but the map and rail surface different layers of it, mk may have to synthesize across three views (map, rail, zoom). The design's goal of "quieting the estate" is undercut by *context fragmentation*, which is a cousin of overwhelm.

---

### 5. P2 | Pace-Layer Decision Pacing: One Rail, Many Speeds
**Section:** Autarch-03 CUJ and the "pace layer" axis of the map

**Evidence & Causal Chain:**
- Autarch-03: "Pace layers: fast layers innovate and churn, slow layers stabilize and constrain — and the estate's health is the layers moving at their own speeds."
- The map: "x is the pace layer (fast probe to slow doctrine) and y is the ecosystem band."
- The rail: "ordered by who is stalled", with one global order

**The tension:**
1. Fast-layer gardens need decisions hourly or daily (e.g., "should we pivot this experiment?")
2. Slow-layer gardens need decisions weekly or monthly (e.g., "should we commit this to doctrine?")
3. Mycroft's rail aggregates both into one queue, ordered by "who is stalled"
4. If a slow-layer blocker is deep in the queue, but a fast-layer experiment is blocked and high in the queue, mk is context-switching between decision speeds

**Why this is a systems issue:**
The design correctly identifies that layers should move at their own speeds (autarch-03 ruling 1). But the rail collapses those speeds into one ordering. A fast-layer project experiencing a blocker will rise high in the rail and demand mk's attention. But mk must then switch to slow-layer mode to even *understand* the decision that's needed. This context switching is exactly what autarch-01 (the daily walk) is trying to prevent: "the walk is the watching/waiting half of the gardening loop".

The risk: mk optimizes for *throughput* on the rail (clear the queue) rather than *pacing* per layer. Slow-layer decisions get rushed, fast-layer experiments stall while mk thinks in slow mode.

**Missing mechanism:**
The rail needs either (a) per-layer sub-queues, so mk can batch fast-layer decisions separately from slow-layer ones, or (b) an estimated "context-switch cost" that Mycroft factored into its ordering. Without it, the rail will naturally filter toward fast-layer items (they're more urgent and clearer to decide), and slow-layer governance will starve.

---

### 6. P2 | Multiple Coordinators, Ambiguous Authority
**Section:** Open Questions, item 5 ("The one-coordinator claim") and the proposal approval flow

**Evidence & Causal Chain:**
- Open question 5: "`bbDev | coordinator` and `bbOps | coordinator` threads already exist. The claim mechanism (a bead claim or a BB plugin metadata lock) must exist before Mycroft dispatches beyond T1."
- The design: "Mycroft's proposal can be approved or redirected in place, which is the same act as the rail card."
- The rail: "new projects are bootstrapped through it, and the map selection sets the prompt's scope."

**The contention point:**
Suppose Mycroft proposes: "spin up a new garden in the fast-layer", and mk approves it via the rail. But the garden spans both DevOps and Ops concerns (it's a new CI/CD experiment). Which coordinator thread owns it? If bbDev coordinator claims it but then bbOps coordinator puts a hold on it (infrastructure capacity), what happens?

1. Mycroft sees the hold and marks the proposal blocked
2. The blocked proposal sits in the rail
3. mk sees it and approves it again (thinking the hold is outdated)
4. Rejection loop: bbOps blocks again, proposal re-emerges, mk approves again

**Why this is a systems issue:**
The design assumes one coordinator (mk via approval). But the existing threads (bbDev, bbOps) are also coordinators. The authority hierarchy is unclear: does mk's approval via Mycroft override a coordinator thread's hold? Or does the hold require mk to negotiate with the coordinator directly (outside the rail)?

If mk's approval can override, then coordinator threads lose real authority and become advisory. If holds block approval, then the rail is no longer an escalation-to-mk mechanism; it's an inter-coordinator negotiation board, and mk is a tie-breaker who doesn't understand the underlying constraint.

**Missing decision:**
The design needs to state explicitly: when Mycroft proposes a move that spans multiple coordinators, what is the resolution path? Is there a proposal *status* that says "waiting for coordinator sign-off" distinct from "waiting for mk approval"? Does mk see coordinator holds as a form of blocker (visual, on the map) or as an error state that prevents the proposal from reaching the rail?

---

## Improvements

1. **Add explicit proposal budget to Mycroft:** Define the rail's steady-state capacity as N active proposals per layer per time period. When the budget fills, Mycroft holds new proposals until mk clears some. This makes the feedback loop visible and prevents the rail from becoming an unbounded backlog.

2. **Introduce estate-size governance:** The seasonal reshaping (autarch-03) is the *correction* mechanism, but it's rare. Add a real-time signal: if the estate grows beyond M gardens per layer, new bootstrap proposals require explicit approval. Or: new gardens without a confirmed product card are marked provisional and auto-archive if untouched for T days.

3. **Distinguish "quiet by design" from "abandoned":** Add a heartbeat mechanism: when mk visits a slow-layer garden and finds it healthy but unchanged, mk explicitly marks it (e.g., "checked and at pace" button). The glow then reflects *attention state* not just *elapsed time*. Gardenswithin 2x the expected pace cycle can have a distinct color (healthy) vs. ones beyond it (at risk).

4. **Visualize blocker cascades on the rail itself:** Don't just show "who is stalled". Show the *depth* of the cascade (A blocks B blocks C) and the *longest path* to resolution. This makes the context-fragmentation cost visible: if mk needs to understand 4 layers to unblock 1 item, Mycroft should say so.

5. **Stratify the rail by pace layer:** The rail has sub-sections: fast-layer questions (decide hourly), slow-layer questions (decide weekly), and cross-layer questions (these need special attention). This surfaces the pace mismatch explicitly and lets mk batch decisions by mode.

6. **Clarify coordinator contention:** Define the proposal *state machine*: submitted → coordinator-review → mk-approval → executing → done. If a proposal needs multiple coordinators, show which ones have signed off and which haven't. If a hold is in place, show *who* is holding (mk, coordinator, blocker) so mk knows what action is needed.

---

## VERDICT ---
STATUS: warn
FILES: 0 changed
FINDINGS: 6 (P1: 3, P2: 3)
SUMMARY: The design correctly identifies attention-routing and pace-layer separation as key levers, but three reinforcing feedback loops lack balancing mechanisms: Mycroft proposals amplifying escalation load, new-project bootstrap creating unbounded estate sprawl, and neglect signals making healthy slow-layer gardens indistinguishable from abandoned ones. Blocker cascade visibility, pace-layer context switching, and coordinator authority contention are downstream issues that become addressable once the feedback structure is explicit. The map surface is at risk of being quiet while the rail grows.
---

<!-- flux-drive:complete -->
