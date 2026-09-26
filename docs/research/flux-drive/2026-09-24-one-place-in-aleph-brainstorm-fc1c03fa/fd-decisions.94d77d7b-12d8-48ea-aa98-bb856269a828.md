<!-- run-uuid: 94d77d7b-12d8-48ea-aa98-bb856269a828 -->

### Findings Index
- P1 | DECISION-5-FORWARD-REF | "Key Decisions, #5" | Intention decision forward-references its own cut; confuses reading
- P1 | TRIAL-CRITERIA-VAGUE | "Key Decisions, #6" | Trial success ("catches") has no baseline, thresholds, or volume prediction
- P1 | KILL-RULE-UNDEFINED | "Key Decisions, #6" | References "trial plan's kill rule" but never states it in this document
- P1 | ULTAN-UNEXPLAINED | "Key Decisions, #11" | Lattice required to meet Ultan thesis; no explanation of thesis authority or binding nature
- P1 | AUTARCH-08-COMPLEXITY-TRAP | "Key Decisions, #5, #8, #19" | CUJ marked "validated" but drove design mk later called "overly ornate"; signals validation missed feasibility
- P2 | BUILD-ORDER-UNJUSTIFIED | "Key Decisions, #7" | "Home on beads first" stated but no rationale for this sequence
- P2 | TOKEN-COST-UNVALIDATED | "Key Decisions, #13" | "150k-token re-read" premise cited with no token logs or cache expiration evidence
- P2 | BUILD-CASCADE-UNCLEAR | "Key Decisions, #7" | Build order lists sequential steps; if Lattice not ready for step 3, cascade path undefined
- P2 | VALIDATION-DEFINITION-AMBIGUOUS | "Key Decisions, #18, companion: autarch-07/09" | "Validated" CUJs never defined (mk walk-through vs. end-to-end test vs. user feedback)
- P3 | MISSING-REJECTION-RATIONALE | "Key Decisions, all" | Options listed (option A, option B) but mk's reasoning against rejected options not recorded

Verdict: needs-changes

### Summary

This brainstorm refines a prior attention-map design by embedding it in Aleph as a plugin, making 20 decisions with two major amendments to prior rulings (decision 2, decision 14) and one scope cut (decision 19). The framing is sound: reuse the design already vetted, keep the fork thin, use beads for decisions. However, the document has three structural decision-quality gaps:

1. **Vague trial gate:** Success is judged on "catches," but with no baseline, threshold, or volume prediction—making the kill criterion non-actionable.
2. **Complexity trap surfaced mid-document:** autarch-08 was marked "validated" but drove a design mk later called "overly ornate," revealing that the CUJ validation didn't check implementation feasibility; this pattern appeared in the prior attention-map review and is repeating.
3. **Forward-referencing and undefined anchors:** Decision 5 is stated before its own cut (decision 19), and decision 11 anchors to an external thesis (Ultan) without explaining its authority.

Most decisions are reversible (pre-shipping, modular), but the trial-success criteria are too loose to trigger action.

### Issues Found

1. **P1: Decision 5 Forward-References Its Own Cut** — Decision 5 states "Intention = decide + set focus + talk" but decision 19 cuts focus from v1. The decision reads confusingly because it's stated as-decided before the cut is mentioned 14 lines later. Should read "decide + talk (with focus parked for v1; see decision 19)."

2. **P1: Trial Success Criteria Vague and Unactionable** — Decision 6 claims success is "catches" measured in "the existing caught-something log, which uses the trial plan's kill rule," but the document never defines: (a) what count of catches = success, (b) what baseline rate mk catches things today without Home, (c) what the kill rule is. "Catches" is 0–N; without thresholds, the trial gate cannot fire. Compare to prior attention-map review (fd-decisions-ca9b9839, issue #4), which flagged "Earn its place" as unoperationalized—this document repeats the pattern.

3. **P1: Kill Rule Referenced but Never Stated** — Six references to "the trial plan's kill rule" (decision 6, decision 19, decision 20) treat it as external knowledge, but it never appears in this document. Is it defined elsewhere (in the trial plan doc)? If so, this document should cite it by name/date and summarize it. If not, it's aspirational language masking missing decision.

4. **P1: Ultan Thesis Not Explained; Authority Unclear** — Decision 11 ("Grow Lattice to meet the Ultan thesis") cites a file (`Sylveste/apps/Autarch/docs/research/2026-09-03-ultan-nativity-thesis.md`) without explaining: (a) what Ultan is, (b) whether the thesis is binding or guidance, (c) why Lattice *must* meet it vs. *should* meet it. This anchors design to an external doc without justifying the anchor. Is Ultan mk's own requirements? A community standard? A prior decision from another project? The lack of context makes it hard to challenge or validate.

5. **P1: autarch-08 Marked "Validated" but Drove Overly Complex Design** — Decision 8 specifies focus as a pinned bead with Clavain hook, stamped files, Mycroft filter, and query system. Decision 19 cuts it because "walking autarch-08 grew focus into...overly ornate/intricate ceremony." This reveals that autarch-08 (marked "validated" in the intro) was not validated for *implementation feasibility*, only for conceptual soundness. This echoes the prior attention-map review (fd-decisions-ca9b9839, issue #1: "Anchoring on agent-recommended options"), where "recommended" options anchored decisions without weighing implementation cost. If autarch-08 is a CUJ validated by mk's walk-through, the validation gate is too soft to catch ceremony-creep.

6. **P2: Build Order Lacks Rationale** — Decision 7 states "Home on beads first" but never explains why this order (decisions + focus, then badges, then Lattice, then map, then Mycroft, then companion) is optimal. Why not start with Lattice, then map, then layer decisions on top? Why are badges (step 2) before the map (step 4) when decisions feed the map? No prioritization stated for why beads are critical-path and map is not.

7. **P2: Token-Cost Premise Unvalidated** — Decision 13 justifies continuations via RPC by claiming "the cost is not in delivering the answer but in the context that wakes up to act on it. Resuming a 150k-token asker after its cache has expired re-reads about 150k tokens." This is reasonable, but: (a) no token logs provided to show that cache expiration is a real problem, (b) no data on how often threads resume after cache expiration, (c) "150k tokens" is an assumption, not measured. If this turns out to be a micro-optimization (most threads don't resume), the continuation design could be overcomplicating the RPC layer.

8. **P2: Build-Order Cascade Unclear if Lattice Blocked** — Decision 7 lists sequential steps. If Lattice (step 3) is not ready before step 4 (the map) ships, can the map ship without Lattice? Can map and Lattice be parallel? The document does not state dependencies, so if Lattice stalls, the whole cascade is blocked or unclear.

9. **P2: "Validated" CUJs Never Defined** — Decisions 7, 18, and 20 reference autarch-07, autarch-08, and autarch-09 as "validated" without clarifying what that means. Does "validated" mean: (a) mk walked through the CUJ in a session and found it sensible? (b) end-to-end tested with a real agent and data? (c) user-tested with mk? The standard matters because autarch-08 was validated but then drove the overly-complex design cut in decision 19. If validation is only a walk-through, it's not sufficient to catch feasibility gaps.

10. **P3: Decisions Missing Rejection Rationale** — Most decisions (e.g., #1, #3, #4, #10, #12, #16, #17) list options but do not record mk's reasoning for rejecting the unchosen ones. Decision 16 lists rejected approaches ("Rejected: an append-only ledger, git as ledger, bb events, passkey tap") with brief rationale, which is excellent. Replicating this pattern for other decisions would make it easier to know whether alternatives were truly considered vs. dismissed without evaluation.

### Improvements

1. **Clarify Decision 5 by removing forward-reference:** Rewrite to "Intention = decide + talk (with focus parked for v1; see decision 19)." Remove "set focus" from the initial statement.

2. **Define trial success criteria now:** State in decision 6: "Success is ≥N catches per week" (pick a number based on the existing log's baseline rate); "Kill criterion: if catches <M per week by end of week 2, we reduce v1 to decisions-only and defer the rail to v1.1."

3. **State the kill rule in this document:** Copy or summarize "the trial plan's kill rule" into decision 6 so the gate is explicit, not external.

4. **Explain Ultan or move the dependency:** Either (a) add 1-2 sentences to decision 11 defining Ultan and why Lattice must meet it, or (b) reframe decision 11 as "Lattice remains the index; we will align it with Ultan thesis later if needed" (deprioritize it as a v1 blocker).

5. **Add rationale to Decision 7:** State why decisions + focus are critical-path (e.g., "beads are the data source for all downstream views; ship them first to unblock parallel development of map and rail"). This makes the cascade visible.

6. **Validate token-cost premise before shipping continuations:** Log a pre-implementation check: "Measure cache-miss rate and token cost of re-waking threads in current Autarch; if re-read cost is <10% of total session cost, simplify continuations to fire-and-forget without token optimization."

7. **Add build-order dependencies:** Create a simple DAG: "Decisions (blocks Map, Proposals); Lattice (blocks Map); Proposals (blocks only Companion). Badges and Companion can run in parallel after Decisions."

8. **Define "validated" for CUJs:** Add a note to the intro: "CUJs marked 'validated' have been walked through by mk and agreed to be feasible in concept; they are not end-to-end tested and may reveal complexity gaps during implementation (see decision 19)."

9. **Add rejection rationale to key decisions:** For decisions 2, 11, 12, 16, and 18, add 1–2 sentences of "rejected because..." for each unchosen option. Reuse the pattern from decision 16.

<!-- flux-drive:complete -->
