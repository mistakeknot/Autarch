# Flux-drive perception review: Autarch estate map + Mycroft rail

**Agent:** interflux:fd-perception
**Evidence:** brainstorm 2026-09-22 (discover stage); supporting docs (Ultan thesis, thread registry probe, layer-view plan); prototype v2 HTML
**Scope:** map/territory confusion, information quality, temporal reasoning, perceptual bias
**Status:** design rulings made, two prototypes rendered, nothing built or committed

---

## Findings Index

- P1 | MAP-001 | "Ecosystem bands" | 47/98 gardens lack ecosystem data; map will show placement without stating confidence
- P1 | GRAPH-001 | "Dependency edges" | 5 documented edges vs ~98 gardens; blast radius and coupling model fundamentally incomplete
- P1 | DOCS-001 | "Core-document discovery" | Filename scan already found false missing (Autarch backlog); backlogs live off-repo; metadata staleness signals neglect falsely
- P1 | UNTOUCHED-001 | "Untouched conflates signals" | Thread activity ≠ tending; newer(last_commit, last_thread_activity) is unstated model of "visit"
- P2 | GLOW-001 | "Glow settlement and change blindness" | Glow settles on view without edit; if garden changes later, mk might not notice gradual divergence
- P2 | FADE-001 | "Ambient pressure without explicit alert" | Neglect fade follows mk's autarch-01 rule (claim, not verified); slow change blindness risk
- P2 | PARADIGM-001 | "Layer semantics shift at Ultan migration" | Current layer ≈ "where it lives"; Ultan ruling 4: layer ≈ "speed it moves at"; historical placement will be reinterpreted

**Verdict: needs-changes**

---

## Summary

The design places 98 gardens on a fixed estate map using three axes (pace layer, ecosystem, spatial nudge) that draw from sparse, incomplete sources. The core risk is that mk will observe the map and believe its placement is fact-based when it is in fact a composite model with unstated confidence levels and underlying data gaps. Ecosystem placement lacks 47/98 gardens; dependency edges cover only 5 serving relationships; core documents use a filename heuristic already marked as error-prone; and the temporal signal ("untouched") conflates several independent facts (commits, threads, tending). When spatial position encodes authority, and the underlying data is incomplete or multi-sourced, the map becomes a confident-looking falsehood rather than an honest model.

---

## Issues Found

### 1. **P1 | MAP-001: Ecosystem placement lacks 47/98 gardens; reifies guess as fact**

**Evidence:**
- Brainstorm open question 1: "CanonGraph has an `ecosystem` field, but only 51 projects have rows, and Autarch itself has none. Gardens without a row need a stated fallback region, not a guess."
- Ultan thesis: "51 projects on layers A through X, total 51" across 13 layers. Prototype data shows only 29 named gardens scanned; remaining ~69 auto-generated as "quiet garden N."
- Layer-view plan confirms: gardens without a graph row must render "no row in the graph" as a fact. But the map design places all 98 gardens on the canvas.

**Failure scenario:**
- mk looks at the map and sees Autarch in the "Tools & personal" ecosystem region.
- mk later decides to reorganize that region.
- mk discovers Autarch's ecosystem assignment was a fallback guess (or wasn't there), not a deliberate placement.
- Or mk misses that a garden was auto-placed in an ecosystem it doesn't belong to, because the position is silent about confidence.

**Temporal angle:**
- Once a garden appears in a position, that position becomes reified (stored in localStorage as a pin preference if mk nudges it).
- If ecosystem data arrives later (e.g., from Ultan), the old guess persists as a pinned position.

**Impact:** mk builds decisions on ecosystem proximity ("these three are in the research layer, let me group them") without knowing which assignments are data-backed vs. guessed.

---

### 2. **P1 | GRAPH-001: Dependency graph with 5 edges is too sparse to show true coupling; blast radius is false**

**Evidence:**
- Brainstorm open question 2: "`serving_map` holds 5 edges, and the prototype's edges are illustrative. The candidate sources are go.mod and package manifests, bead cross-references, and edges mk declares."
- Ultan thesis: "Five `serves` edges, every one on zklw."
- Layer-view plan: "Live graph on 2026-09-03: `serving_map` returns 5 rows, all machine `zklw`."
- Prototype HTML (line 115): `const EDGES=[...]` hardcoded 14 illustrative edges; data source unresolved.

**Failure scenario:**
- mk views the dependencies lens and sees intercore blocking Autarch, Clavain, cujgel, interflux as "at risk."
- mk prioritizes unblocking intercore.
- A week later, mk discovers that project X (not shown as at-risk) is deeply coupled to intercore through go.mod imports, and the unblock of intercore did not fix X.
- mk's mental model of the estate's coupling was 5 edges wide; reality is much wider.

**Signal/noise angle:**
- The map presents a confident causal narrative (blast radius) with insufficient evidence.
- This is narrative fallacy: the story of "upstream blocker → visible downstream risk" is compelling but built on incomplete data.

**Temporal angle:**
- The missing edges will accumulate over time. As mk makes dispatch decisions based on the 5-edge model, unmodeled coupling will create surprises.

**Impact:** mk's mental model of the estate's bottlenecks is systematically incomplete. Decisions to reorganize or dispatch will miss real critical paths.

---

### 3. **P1 | DOCS-001: Core-document presence is a filename scan with known false-misses; metadata staleness signals neglect falsely**

**Evidence:**
- Brainstorm open question 3: "A filename scan on zklw found documents for 20 of 29 named gardens. Nine have no canonical path on zklw. Backlogs often live in the workspace tracker rather than in the repo; Autarch has no `.beads` directory, so the scan marks its backlog missing even though `Sylveste-fuwn` exists."
- Brainstorm decision 4: "Missing or drifted documents are a neglect signal." (But: missing ≠ drifted ≠ unstored.)
- Layer-view plan WI-2: `LayerRow` carries no document presence data; the scan is a separate step not yet wired into the map.

**Failure scenario:**
- mk opens the map, enters Autarch zoom, sees the core-document shelf with backlog marked "missing" (orange dashed border).
- mk thinks: "Autarch's backlog is neglected."
- Reality: Autarch's backlog lives in Sylveste-fuwn (workspace tracker), not in a `.beads` directory in Autarch's repo.
- mk believes a valid garden has a neglect problem when the problem is metadata staleness (the scan knows the wrong path).

**Temporal angle:**
- The per-garden document-location mapping doesn't exist yet (brainstorm open question 3).
- As long as it doesn't, the scan will produce false misses whenever a garden stores documents off-repo (workspace tracker, external wiki, etc.).
- These false misses will age into mk's mental model of neglect over time.

**Signal/noise angle:**
- "Missing document" should mean "we looked and found no evidence it exists."
- But the scan means "we looked in the canonical paths and found nothing," which is metadata staleness, not neglect.
- mk will interpret orange (missing) as amber (neglect) when it should be gray (unknown).

**Impact:** mk will invest effort maintaining document locations (or reorganizing gardens) when the real signal is that the scan's fallback paths are wrong. Neglect signal is corrupted.

---

### 4. **P1 | UNTOUCHED-001: "Untouched" conflates thread activity with tending; the temporal model is unstated**

**Evidence:**
- Brainstorm decision 7: "Time is shown as since-last-visit plus neglect fade. The glow settles once a garden has been seen."
- Brainstorm open question 4: "What counts as 'untouched'. The prototype uses the newer of the last commit and the last BB thread activity. Is thread activity alone enough evidence of tending?"
- Prototype HTML line 113: `const untouched=p=>(p.a||p.m)?0:p.days;` — if agents (p.a) or moves (p.m), return 0 (touched); else return days untouched.

**Failure scenario:**
- Clavain has a thread activity today (someone ran an agent, or mk read a thread about Clavain).
- Clavain has no commits in 7 days.
- The map shows Clavain as "touched today" (glow active, days untouched = 0).
- mk thinks: "Clavain is being tended."
- Reality: The thread activity was a bot message or a read-through; no actual work happened; Clavain is stalled.
- OR: Clavain has a commit from 7 days ago and a thread activity 2 hours ago. The "last-visit" model says mk tended it; reality might be mk read a status update.

**Temporal reasoning angle:**
- The map uses "newer of (last commit, last thread activity)" as a proxy for "last visit."
- But mk hasn't stated what "visit" means: is it tending, reading, being aware of?
- BB thread activity is a lagging indicator (something happened in the past) and a leading indicator (an agent is thinking about it now).
- Commits are clear (work happened), but threads are ambiguous.
- This is a silent model of time, and it conflates two independent signals.

**Stratification angle:**
- The design tries to distinguish "moved but unseen" (glow) from "untouched" (fade).
- But "moved" means "agent labor or explicit moves by mk" (p.m flag), not "someone worked on the code."
- A garden with active agent work (p.a=1) but zero commits still shows as "touched."
- The design doesn't distinguish "active agents" from "active humans"; both darken the same way.

**Impact:** mk's sense of which gardens are actually stalled vs. busy will be false for gardens with active agent work but no code progress, or thread activity without agent dispatch.

---

### 5. **P2 | GLOW-001: Glow settles on view, creating change blindness if garden diverges later**

**Evidence:**
- Brainstorm decision 7: "Time is shown as since-last-visit plus neglect fade. The glow settles once a garden has been seen."
- Brainstorm refusal: "Scrub-back replay goes on the backlog."
- Prototype HTML line 166: `const S={lens:'attention',sel:null,zoom:null,seen:new Set()};` — seen set is localStorage-persistent.

**Failure scenario:**
- mk opens the map on Monday and sees interflux glowing (moved since last visit).
- mk enters interflux zoom to review the outcomes, then exits.
- The glow settles (S.seen.add('interflux')).
- On Tuesday, interflux gets 3 new commits and a thread activity.
- mk opens the map Wednesday and does not see interflux glowing (it's already in seen).
- mk might miss that interflux changed again.
- (The design does not track "last view time per garden"; only "was it ever viewed in this session".)

**Perceptual bias angle:**
- Change blindness: mk is accustomed to the settled state and might not re-notice the garden if it changes.
- The glow is binary (on/off), not a timescale (glowing brighter if changed more recently).
- "Glow settles when seen" means mk can't re-glow it until the next session or until mk clears the seen set.

**Temporal reasoning angle:**
- The design doesn't model "last-seen timestamp." It models "ever seen in this session."
- This breaks down over multi-day work cycles where mk revisits gardens.
- If mk wants to know "has interflux changed since I last looked at it," the glow will not answer after the first look.

**Impact:** mk might miss gardens that changed after mk last reviewed them, because the signal (glow) is single-use and cannot re-activate.

---

### 6. **P2 | FADE-001: Ambient pressure through fade risks slow change blindness without explicit alert**

**Evidence:**
- Brainstorm design rationale: "A queue of asks can never show what *isn't* happening. Fading makes pressure ambient instead of modal (`sofg-dread-floor-ambient-pressure-not-modal`), which matches quiet-unless-attention-worthy (autarch-01)."
- Brainstorm decision: "Neglect: fade by days untouched, plus a ring for the eight core documents."
- Prototype HTML line 196: `if(S.lens==='neglect'){const u=untouched(p);op=u==null?.6:Math.max(.18,1-u/150)}` — opacity fades from 1.0 to 0.18 as days untouched increases from 0 to 150.

**Failure scenario:**
- mk looks at the map and sees all gardens. Over weeks, linsekasten fades as its untouched days grow from 20 → 50 → 80 → 100.
- mk doesn't notice the fade increasing because it's gradual (slow change blindness).
- At day 150, linsekasten is nearly invisible (18% opacity).
- mk finally notices and thinks "oh, I haven't tended linsekasten in months."
- But mk had no alert telling them the fade was increasing; only retrospective evidence of neglect.

**Perceptual bias angle:**
- Fade is ambient pressure (claim), not alerted pressure (fact).
- mk might habituate to the fade and stop noticing it entirely.
- The design offers no "gardens neglected for >60 days" counter or alert, only visual fade.
- This follows mk's autarch-01 rule ("quiet by default"), but it doesn't follow "orientation before obligation" (mk must orient to the neglect without being alerted).

**Temporal reasoning angle:**
- The fade is continuous (opacity changes every day), but mk's attention is episodic (mk opens the map once a day or less).
- mk might not notice the change between two views one week apart.
- The design lacks a "rate of change" signal (gardens that are fading faster than others).

**Impact:** mk might let gardens atrophy to invisibility without consciously deciding to park them, because the signal is ambient and lacks an explicit alert.

---

### 7. **P2 | PARADIGM-001: Layer semantics shift at Ultan migration; historical placement will be reinterpreted**

**Evidence:**
- Ultan thesis ruling 4: "any node (project, platform, theme, decision) carries a layer letter, an expected revisit cadence, and a last-tended timestamp... pace is an attribute axis, as the ontology note sketched, and it has time in it. ... a fast probe sedimenting into doctrine is a layer change with a stamp, not a move in a tree."
- Current CanonGraph: layer is a string on Project only; no time on any node; "layer letter only, cadence local" was the rejected alternative.
- Layer-view plan: reads CanonGraph `projects_in_layer` query; swappable for Ultan behind one interface.

**Failure scenario:**
- mk looks at the map today. Meadowsyn is in the fast probe layer (A or B), reflecting its recent churn.
- Ultan is built. The layer now means "expected revisit cadence" (probe = every 2 weeks, doctrine = every quarter).
- mk looks at a historical record (or a screenshot from today) and interprets Meadowsyn's placement as "this is where Meadowsyn should be revisited."
- But the old placement was "this is where Meadowsyn is moving actively," not "this is where it should slow down."
- The semantics changed, and the historical data is now misleading.

**Temporal reasoning angle:**
- Layer transitions are now "stamped world edits" (Ultan ruling 4, amended ruling 5).
- A layer change will become an event in the graph, not just a state change.
- The map (built from CanonGraph today) doesn't know about this future semantics shift.
- When mk moves a garden's layer in the current map (nudged pin), that's a localStorage preference, not a stamped world edit.
- After Ultan migration, that nudge will need to become a recorded decision.

**Impact:** mk's current mental model of "layer = where the garden is moving" will need to shift to "layer = how fast it should move." The map as currently designed doesn't prepare for this change in meaning.

---

## Improvements

### 1. **Explicit confidence levels on ecosystem and dependency edges**

Ground each garden's ecosystem placement in one of: (a) graph row, (b) heuristic/fallback, (c) unknown. When showing a garden, render a small icon (✓ | ⊗ | ?) or tooltip that states the source. For dependencies, show a count of documented edges vs. inferred edges, or label edges by source (serving_map | go.mod | declared).

**Rationale:** Removes the silent model. mk sees "this placement is a guess" and makes different decisions than if they thought it was fact.

### 2. **Separate "moved" from "tended" signals**

Split the fade calculation: keep "days since last commit" as the neglect signal. Add a separate "agents working" indicator (p.a flag from the prototype). Make "last thread activity" visible as a third signal (timestamp, not binary). This way mk can see: "garden X has agents working but no commits (stalled but active)" vs "garden X has commits but no agent labor (done but unvalidated)" vs "garden X has thread activity but neither (someone is reading, not doing)."

**Rationale:** "Untouched" currently conflates three independent signals. Splitting them gives mk better orientation.

### 3. **Per-garden document-location registry**

Build the per-garden fallback mapping before the map is deployed (complement to brainstorm open question 3). For each named garden, record the path(s) where each of the 8 core documents live (or "in workspace tracker" / "external" / "not yet created"). The scan then looks up the per-garden map before concluding "missing."

**Rationale:** Eliminates false misses. mk's neglect signal becomes accurate.

### 4. **Glow reset on re-change, or per-session glow age**

Instead of "glow settles once seen in session," track "last time mk viewed this garden" (not "ever seen"). On each new map view, gardens that changed since the last view re-glow. Or, show glow intensity as a function of "how long ago was it last seen" (new ≈ bright, old ≈ dim).

**Rationale:** Prevents change blindness. mk can see "I looked at this on Monday and it's glowing again on Wednesday, so it changed."

### 5. **Explicit "neglect alert" at configurable thresholds**

Add a counter or beacon (e.g., "3 gardens untouched >60d") to the header strip. Let mk set thresholds per layer (fast layers alert sooner; slow layers have longer budgets). This makes the ambient pressure explicit.

**Rationale:** Follows mk's autarch-01 "orientation before obligation" principle. mk knows the neglect situation without reading the fade.

### 6. **Document the temporal model of "untouched" before map launch**

Write a one-paragraph rule: "A garden is untouched if _____." Use it in the map legend. State: "Thread activity counts as _____ [tending/awareness/bot message]; commits count as _____." Let mk verify this matches their intent before v1 ships.

**Rationale:** Removes the silent model. mk knows what the fade means.

### 7. **Mark paradigm-shift risk in Ultan migration plan**

In the build plan for Ultan (out of scope here, but upcoming), include a note: "Layer semantics change from state (where garden is moving) to tempo (how fast it should move). Historical map screenshots will need reinterpretation." Prepare a migration narrative for mk.

**Rationale:** When the shift happens, mk won't be surprised that their old spatial intuition is now inaccurate.

---

## Findings Summary

| Severity | Count | Category | Blocking? |
|----------|-------|----------|-----------|
| P1 | 4 | Map/Territory Confusion, Signal/Noise | Yes |
| P2 | 3 | Perceptual Bias, Temporal Reasoning | Consider for v1 |

**Blocking findings (P1):** ecosystem placement without confidence, dependency graph too sparse for blast-radius accuracy, core documents marked missing falsely, "untouched" model unstated. Each silently converts a guess into apparent fact.

**Enhancement findings (P2):** glow settlement creates change blindness, fade is ambient without alert, layer semantics will shift at Ultan migration.

---

## Recommendation for Next Phase

The design is sound in direction (map + rail, persistent position, four lenses, approval-driven dispatch). The blind spots are not architectural; they are epistemic gaps where mk will observe the map and believe data that the map is actually uncertain about.

**Before implementation:**
1. Decide per-garden fallback rules for ecosystem and core-document paths (brainstorm open questions 1 and 3).
2. Resolve dependency-edge sources (question 2): commit to a hierarchy (serving_map first, then go.mod, then declared) and implement at least the first source completely.
3. Write the temporal model of "untouched" and verify with mk (my improvement 6).
4. Add confidence indicators to ecosystem and edges (my improvement 1).

**For v1 launch:**
- Document the glow behavior (single-use until next session) so mk knows to re-read gardens to see changes.
- Implement the neglect alert counter (my improvement 5) to make ambient pressure explicit.
- Mark the Ultan paradigm-shift risk in the follow-on plan (my improvement 7).

The map will be credible and useful if mk can see when it's uncertain. Right now, it will be confident-looking and wrong in small ways that accumulate over time.

---

--- VERDICT ---
STATUS: warn
FILES: 0 changed
FINDINGS: 7 (P0: 0, P1: 4, P2: 3)
SUMMARY: Map design is directionally sound but contains four silent epistemic gaps where mk will mistake incomplete data (sparse ecosystem, 5 documented edges, filename scan, conflated temporal signals) for complete fact. Add confidence layers before implementation.
---

<!-- flux-drive:complete -->
