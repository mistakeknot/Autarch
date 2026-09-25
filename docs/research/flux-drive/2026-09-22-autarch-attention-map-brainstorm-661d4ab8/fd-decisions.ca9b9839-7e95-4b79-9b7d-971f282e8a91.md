# Flux-Drive Decision Quality Review
## Autarch Attention Map — 2026-09-22 Brainstorm

**Review Date:** 2026-09-23
**Stage:** Discover (brainstorm + prototype)
**Run UUID:** ca9b9839-7e95-4b79-9b7d-971f282e8a91

---

### Findings Index

- P1 | ANCHORING | "Key Decisions" | Agent-recommended options accepted without re-evaluation; "recommended" framing anchors all four rulings
- P1 | ALL-OF-ABOVE | "Map's Jobs" | All four jobs selected with no prioritization; creates unfocused scope and sour-spot risk
- P1 | MISSING-KILL | "Why This Approach" | No kill criterion stated; no "if unused by X date, we stop" decision
- P1 | NO-SUCCESS-CRITERIA | "Map's Jobs" | "Earn its space" is aspirational; no metrics for what success looks like
- P2 | PREMATURE-LOCK | "Position Means Fixed Regions" | Pace layer × ecosystem axis locks in before validating this visualization helps mk make decisions
- P2 | OVERPLANNING-PATTERN | "Prior Art" | 103 prior plans + 18 Meadowsyn experiments with no recorded verdict; current design adds four jobs without feedback loop
- P2 | MISSING-SIGNPOST | "Open Questions" | No pre-committed decision trigger; no "if X at date Y, pivot to Z" rule stated
- P2 | UNDERSPEC-ORDER | "Order of Work" | Sequential phases given ("attention first, then approval") but not which of four jobs within attention slice

**Verdict:** needs-changes

---

## Summary

The brainstorm makes structural design choices under time pressure using agent-recommended options that mk accepted largely as-is. The framing ("recommended") anchors four key rulings. The scope decision—all four map jobs selected simultaneously—creates risk of building a feature that does many things adequately rather than one or two things well. Critically, the document lacks:

1. **A kill criterion** — when to stop investing if the map doesn't earn its space
2. **Success metrics** — what "earn its space" actually measures
3. **Job prioritization** — which of the four jobs is load-bearing, and which are nice-to-have
4. **A decision trigger** — what specific outcome (or lack thereof) causes a pivot
5. **Feedback loop closure** — how to learn from the 18 prior Meadowsyn experiments and 103 plans without verdicts

The placement decision (pace layer × ecosystem) is reversible in implementation (can build attention lens first with simpler layout) but the conceptual lock-in is expensive: if this axis doesn't help mk navigate the estate, the entire map frame is wrong, not just the UI.

---

## Issues Found

### 1. **Anchoring on Agent-Recommended Options** [P1]

**Location:** "Key Decisions (mk, 2026-09-22)" section; all eight rulings.

**Evidence:**
- Task context states: "mk accepted the recommended option for position, zoom, acting and time"
- All four layout rulings follow this pattern: agent proposes options, one marked "(Recommended)", mk accepts
- No re-evaluation or re-framing of alternatives documented
- Document does not show mk's reasoning *against* the other options

**Why it matters:**
The word "recommended" is a high-confidence anchor. Framing as "Option A (Recommended)" + "Option B" + "Option C" makes Option A the default-to-accept path, exploiting status-quo bias. The decision appears made, not *chosen after critical weighing*. In a high-stakes, irreversible-in-practice architecture decision (the map's layout, time tracking, approval model), anchoring bias can lock in a choice that a fresh re-frame would reject.

**What's missing:**
- mk's stated reasons for accepting the recommended option *vs* the other options
- A statement like "I rejected Option B because..." or "Option A is better than C because..."
- Evidence that mk considered reversibility of each choice

**Lens:** Anchoring bias, Explore/Exploit (no evidence of exploring alternatives)

---

### 2. **"All of the Above" Job Selection Without Prioritization** [P1]

**Location:** "Key Decisions" ruling #2: "The map's jobs, all four"

**Evidence:**
- Four distinct jobs listed: gestalt/spatial memory, neglect/absence, blast radius, allocation/Mycroft
- Decision text: "all four" with no ordering
- "Seasonal reshaping on the map is not a v1 job" — implies deprioritization exists, but not *among the four chosen jobs*
- No statement like "Ship in this order: 1. Attention (gestalt + neglect), 2. Dependencies (blast radius), 3. Allocation"

**Why it matters:**
Committing to four jobs simultaneously creates a "sour spot" (Flux-drive term): each job competes for design real estate, implementation time, and cognitive load. The "one map, four lenses" framing sounds elegant, but without prioritization, the implementation risk is: build all four imperfectly rather than two excellently. This violates the explore/exploit principle: you're optimizing for feature completeness rather than learning which job actually helps mk navigate the estate.

**What's missing:**
- A prioritization order with rationale, e.g., "Attention first because it captures the urgent case; neglect second because it's a leading indicator"
- A hypothesis about which job is the minimum viable layer, and which are "nice-to-have but not this release"
- Acceptance criteria per job ("attention view succeeds if X, Y, Z")

**Lens:** All-of-Above trap (choosing all options without trade-offs), Explore/Exploit (no winner declared)

---

### 3. **No Kill Criterion Stated** [P1]

**Location:** Implicitly missing from entire "Why This Approach" and "Key Decisions" sections.

**Evidence:**
- Document frames the map as essential ("answers what is the estate doing")
- Zero mention of a kill trigger: "if we ship the map and mk doesn't use it, we..."
- The "Open Questions" list focuses on implementation details, not viability questions

**Why it matters:**
Without a kill criterion, the map becomes a sunk-cost trap: "we designed it, we shipped it, we'll keep maintaining it even though mk uses the rail 90% of the time and the map 10%." The document does not state *when* the map would be abandoned or de-prioritized. This is especially risky given the project history: 18 Meadowsyn experiments exist with "no recorded verdict" on whether visualization + spatial reasoning actually helps mk's estate navigation.

**What's missing:**
- A pre-committed kill criterion, e.g., "If mk visits the map on <20% of estate-view sessions in weeks 1–4, we deprioritize the four-lens design and pivot to attention-only"
- A time-boxed validation period, e.g., "We have 30 days to show that the map is used more than the rail"
- Success/failure thresholds stated upfront

**Lens:** Premature Commitment, Option Value, Sunk Cost avoidance

---

### 4. **Success Criteria Not Operationalized** [P1]

**Location:** "Why This Approach" section; specifically "The map's jobs" subsection.

**Evidence:**
- "mk asked the map to earn its place with four jobs a list cannot do"
- The four jobs are described philosophically (gestalt, spatial memory, neglect as ambient pressure)
- No quantitative or observable success metric stated
- Phrases like "should do" and "answers" are aspirational, not measurable

**Why it matters:**
"Earn its place" is too vague to trigger action. After shipping the attention view, how will mk know the map is succeeding? Possible metrics (not proposed in the doc):
- Visit frequency (% of estate-view sessions where map is opened)
- Decision impact (does the map catch a blocker the rail would have missed?)
- Time-to-recognition (does the spatial layout reduce time to find a garden compared to scrolling?)
- Neglect detection (does the fade view surface untended projects that would otherwise be missed?)

Without these, there's no way to declare "the map earned its space" or "the map failed, pivot to rail-only."

**What's missing:**
- At least one quantitative success metric per job, e.g., "Attention view succeeds if glows catch 80%+ of day-to-day navigation needs"
- A baseline measurement (what % of gardens does the rail capture as "needing mk"?)
- A sample-size rule for the validation period (e.g., "measure over 4 weeks of mk's actual use, min 50 estate-view sessions")

**Lens:** Uncertainty Management, N-ply Thinking (no forward pass to "what does success look like when it ships?")

---

### 5. **Premature Lock-In of Placement Axis** [P2]

**Location:** "Key Decisions" ruling #3: "Position means fixed regions that you can nudge."

**Evidence:**
- Pace layer on x-axis, ecosystem on y-axis, fixed regions with nudge-pins
- This architectural choice is presented as settled
- The two sources (CanonGraph now, Ultan later) are noted in the decision, not in the *rationale for why this axis*
- "Open Questions" #1 shows ecosystem field coverage is low ("only 51 projects have rows")

**Why it matters:**
The (pace, ecosystem) placement is a *frame* decision: if this frame doesn't match mk's mental model of the estate, the entire map is misaligned. This is a one-way door in practice (redesigning the axes after shipping is expensive and disorienting). The decision was made *before* validating that mk actually reasons about the estate in pace-layer terms, or that ecosystem bands are meaningful discriminators.

**Examples of frame mismatches:**
- What if mk's primary navigation is by *team* (who owns each garden) rather than by pace and ecosystem?
- What if the pace layer is correct but ecosystem classification is noisy (as open question #1 hints)?
- What if a different frame (e.g., risk level × priority) would be more actionable?

**What's missing:**
- Evidence that mk navigates the estate by [pace × ecosystem] mentally or linguistically
- A test of whether the (pace, ecosystem) frame is *the* right frame or just *a* frame
- A reversibility plan: if this axis proves wrong at ship time, what's the pivot path?

**Lens:** Kobayashi Maru (checking if the problem as framed is solvable), Cone of Uncertainty (this decision narrows the cone prematurely), Theory of Change (are we mapping causal chain correctly?)

---

### 6. **Unresolved Feedback Loop: 18 Meadowsyn Experiments, No Verdict** [P2]

**Location:** "Prior Art" section.

**Evidence:**
- 18 experiments listed in `Sylveste/apps/Meadowsyn`
- Explicitly noted: "no recorded verdict" on feasibility or utility
- Current design reuses rendering patterns from those experiments (e.g., `fixed-stars`, `semantic-zoom`)
- Project history: 103 prior plans + 1 ADR (vast planning, minimal learning recorded)

**Why it matters:**
This is an over-planning pattern (echoed in the project's `OVERPLANNING_DETECTOR.md`): extensive design work without decisive feedback. The current brainstorm risks perpetuating this: shipping the attention map without learning what made those 18 experiments succeed or fail. "Feasibility reviews" without verdicts don't tell you whether the visualization actually *helps mk make better decisions*.

**What's missing:**
- A compact synthesis of what those 18 experiments taught (even: "fixed-stars works for <100 nodes, fails >500 nodes")
- A hypothesis carried forward from Meadowsyn: "We learned X from experiments, so we'll test Y in the v1 map"
- A commitment to record the verdict this time: "After 4 weeks, we will write a 1-page 'what we learned' summary"

**Lens:** Sunk Cost (can't undo the 18 experiments, but can learn from them), Theory of Change (do we know why spatial visualization helps?)

---

### 7. **Missing Signpost Decision: No Pre-Committed Pivot Trigger** [P2]

**Location:** Implicitly missing from "Key Decisions" and "Open Questions" sections.

**Evidence:**
- "Open Questions" lists implementation blockers (ecosystem field coverage, document discovery, etc.)
- No decision rule stated, e.g., "If CanonGraph ecosystem field coverage remains <70%, we fallback to team-based grouping by X date"
- No escalation rule, e.g., "If mk doesn't use the map in week 1–2, we pause and interview why before shipping more lenses"

**Why it matters:**
Signposts are pre-committed decision triggers that let you pivot without retroactive debate. Without them, you're locked into a path until either: (1) a problem becomes undeniable (sunk cost trap), or (2) you fight about whether to pivot (coordination tax). A signpost moves the decision upfront.

**Example signpost for this map:**
- "If ecosystem field coverage is <60% by implementation start, we use [fallback strategy] by [date] and accept [trade-off]"
- "If ecosystem field coverage blocks the axis, we test a single-axis (pace-only) prototype before committing to two-axis design"

**What's missing:**
- Signpost rules for each open question (ecosystem, edges, documents, "untouched" definition, coordinator claim)
- A pre-committed decision: if X is not resolved by date Y, do Z (e.g., "ship attention-only" or "add team-based grouping as fallback")
- Escalation criteria for when a blocker is encountered mid-implementation

**Lens:** Signposts (Flux-drive term), Pre-commitment, N-ply Thinking

---

### 8. **Order of Work Underspecified: Which Job First Within Attention Slice?** [P2]

**Location:** "Key Decisions" ruling #8: "Order of work: first a read-only attention and outcomes view..."

**Evidence:**
- Sequential phases stated: "attention and outcomes view, then approval-driven dispatch, then autonomous tiers"
- But within "attention view," which of the four jobs is v1?
- No statement like "Ship just gestalt + spatial memory in attention lens v1; add neglect fade in v1.1"

**Why it matters:**
The decision to "ship attention first" is good, but incomplete. The attention lens could include:
- *Just* recent activity (minimal viable product)
- Recent activity + neglect fade (adds 2 jobs)
- Recent activity + neglect fade + blast-radius coloring (adds 3 jobs)

Without knowing which jobs live in v1 vs v1.1, the implementation team will guess, potentially building all four and shipping a bloated attention view that's slow or confusing.

**What's missing:**
- A clear job-by-job timeline, e.g., "v1.0: gestalt + spatial memory only; v1.1: add neglect fade; v1.2: add blast radius; v1.3: add allocation"
- Rationale for the order (e.g., "gestalt first because it's the 80% case; neglect second because it's a leading indicator for mk")
- Load-bearing jobs identified: which are must-have, which are nice-to-have?

**Lens:** Scope Creep, Explore/Exploit (which job teaches us the most before shipping the next?)

---

## Improvements

### Immediate (Before Planning)

1. **Create a success rubric for the map.** Propose at least one quantitative metric per job:
   - **Gestalt:** "Attention view is used on ≥50% of estate-view sessions in week 1–4"
   - **Neglect:** "Fade view surfaces ≥1 untended garden per week that mk didn't know about"
   - **Blast radius:** "Dependency overlay prevents ≥1 mis-dispatch per sprint"
   - **Allocation:** "Pace-layer grouping matches Mycroft's proposed move order ≥80% of the time"

2. **State a kill criterion.** Decide now: "If the map is used on <20% of sessions by end of week 2, we pivot to a simplified rail-only design and defer the map to a follow-up epic."

3. **Prioritize the four jobs.** Make a decision: "In v1.0, we ship [1–2 jobs]. Here's why [job A] is load-bearing and [job B] is nice-to-have." Commit to removing jobs from v1 if implementation gets tight.

4. **Document mk's reasoning, not just the decisions.** Go back to the structured questions: capture why the other options were rejected. Replace "recommended" with "mk chose X over Y because..."

5. **Design a minimal experiment to test the placement axis.** Before building the full map:
   - **Hypothesis:** "mk can find gardens faster by (pace, ecosystem) location than by reading a list"
   - **Test:** "Draw a static mock with 10 gardens placed on (pace, ecosystem) grid. Give mk 3 tasks ('find the slowest garden,' 'find the garden that needs deployment'), measure time-to-answer, compare to list view"
   - **Pass/fail:** "Task completion time is <2min per task; mk says 'spatial memory works'"
   - **Payoff:** "If this fails, the entire map frame is wrong; pivot before implementing"

### Before Implementation

6. **Close the Meadowsyn feedback loop.** Write a 1-page synthesis: "Of the 18 experiments, we learned: [fixed-stars works best for <200 nodes; semantic-zoom needs spring-repulsion to avoid overlap; ...]. We're applying lesson X to the v1 map by doing Y."

7. **Add signpost rules to each open question.** For each of the 6 open questions in the doc, propose a signpost:
   - "If ecosystem field coverage is <60%, we test with [fallback] by date [X]"
   - "If core-document discovery finds <50% of gardens by scan date, we require manual per-garden config"
   - Etc.

8. **Explicitly separate rulings from inferences from prior evidence.** Re-read "Why This Approach" and mark each claim:
   - **Ruling (mk):** "The map should use spatial memory as a primary UX feature"
   - **Inference (design):** "One map with lenses keeps positions stable, reducing reorientation cost"
   - **Prior evidence:** "Meadowsyn `fixed-stars` and Shadow Empire both use one substrate with switchable lenses"
   - This clarity helps reviewers spot where assumptions are made vs. where precedent exists.

---

## Questions for the Author

1. **Reversibility:** If the (pace, ecosystem) axis proves wrong in week 1 of dogfooding, can we switch to a simpler (pace-only) view in <2 days, or is the implementation locked in?

2. **Jevons Paradox:** If the map is so effective it shows mk 10x more problems, will the map create a new bottleneck (mk is overwhelmed) instead of solving the existing one (things fall through the cracks)?

3. **Measurement:** How will mk know on day 1 post-ship whether the map is being used? Is there a built-in analytics hook to measure visit frequency per lens?

4. **Fallback:** If the Mycroft composer discovers it needs approvals on the rail (not the map) 80% of the time, is the map then providing marginal value? What's the threshold for "this isn't earning its space"?

---

## Verdict Block

```
--- VERDICT ---
STATUS: warn
FILES: 0 changed
FINDINGS: 8 (P0: 0, P1: 4, P2: 4)
SUMMARY: Solid design framing, but missing kill criteria, success metrics, and job prioritization. Risk of anchoring on recommended options and overcommitting to four jobs simultaneously without feedback loop. Test the (pace, ecosystem) frame before locking implementation.
---
```

<!-- flux-drive:complete -->
