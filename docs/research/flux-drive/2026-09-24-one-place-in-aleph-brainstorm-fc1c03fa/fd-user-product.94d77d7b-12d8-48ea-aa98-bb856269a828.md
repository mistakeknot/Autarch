<!-- run-uuid: 94d77d7b-12d8-48ea-aa98-bb856269a828 -->

### Findings Index
- P1 | UP-1 | "Build order / rail" | Rail-at-98-projects UX is an admitted open question, not resolved, and the one filter that would have addressed it (focus) was cut
- P1 | UP-2 | "Decision 14, everything on pick" | Uniform "everything on pick" removes graduated caution exactly where decision volume is highest, with no confirm step beyond text on a skimmed rail
- P1 | UP-3 | "Decision 6 / build order" | The trial's "catches" success criterion is measured before the net — the mechanism built to catch missed decisions — exists, and the doc never states when the two-week clock starts relative to the six build steps
- P1 | UP-4 | "Decision 18, continuation kind on every option" | Mandatory continuation classification on every option is estate-wide filing ceremony with no stated default or validation step for a misclassified (e.g. wrongly-"command") continuation
- P2 | UP-5 | "Decision 15 vs 19" | Decision 15's v1 scope text ("decisions, focus and the estate picture") was never updated after decision 19 cut focus, and lacks the cut-annotation convention used elsewhere
- P2 | UP-6 | "What we're building / build order" | v1 as sequenced (steps 1-4) delivers about half of the five capabilities the opening section promises; "talk to a companion" and "set focus" are absent from what a v1 user actually gets
- P2 | UP-7 | "autarch-07 step 4" | No loop-breaker for a decision that keeps re-staling every time mk goes to pick it
- P2 | UP-8 | "autarch-07 step 7" | Only continuation *failure* reopens a decision; a successful-but-wrong pick (human error under rail-skimming) has no described undo/reversal path
- P2 | UP-9 | "autarch-09 step 3" | Dismissal dedup ("the same passage is not proposed again") has no stated matching strategy, so a paraphrased re-mention across a thread rotation can re-trigger the exact alert fatigue the net exists to avoid
- P3 | UP-10 | "Decision 20 vs autarch-09 step 1" | Wording tension between "handoff files... are not scanned" and "closing turns, which include the handoff a rotation writes" is easy to misread as scanning the handoff file itself

Verdict: needs-changes

### Summary
Primary user: mk, sole operator of ~98 projects, whose job here is "answer a morning's worth of decisions from Home in a few minutes" (autarch-07's own success condition) without chasing threads. The decide-and-continue flow (autarch-07) is well-built for that job in isolation — a decision card with exact commands, frozen hashes, and a stale-guard is a genuinely good design for one decision at a time. The weak point is what happens at the estate's actual scale: the rail's UX at ~98 projects is left as an open question rather than answered, the removal of focus (decision 19) removes the one filter that would have kept that rail navigable, and "everything on pick" (decision 14) trades graduated caution for speed exactly where a long, skimmed queue makes mis-clicks likeliest. The trial's success bar ("catches") is also measured against a catching mechanism (the net) that the build order places after the trial ends, which weakens what the two-week trial can actually prove about v1's value. None of this blocks the design outright — the core decide/continue loop is sound and validated by autarch-07 — but the scale and trial-validity gaps should be resolved before `/clavain:write-plan`.

### Issues Found

UP-1. P1: Rail-at-98-projects UX is an admitted open question, not resolved. The onepager's own "Top open calls" lists "Whether a decisions rail works across all 98 projects without any focus lens" as unresolved, and CUJ autarch-07 step 2 specifies only "oldest first, tagged with its project" for sort/filter — no search, grouping, or volume cap. Decision 19 then cuts the one mechanism (focus) that would have scoped the rail down to what mk is currently steering. If decision volume across 98 projects is more than a handful at any time, "answers a morning's worth... in a few minutes" (the CUJ's success condition) is untested by the design as written.

UP-2. P1: "Everything on pick" (decision 14) explicitly amends the earlier "approval weight scales with blast radius" rule, so a trivial command and a destructive one get the identical safeguard: the option shows the exact command, frozen by hash. That's sufficient for careful reading of one card, but the whole point of a rail is fast triage across many items — decision fatigue from a long, oldest-first queue (see UP-1) is precisely the condition under which a skimmed exact-command display gets skimmed too. There's no described second-look step (e.g., a distinct confirm for irreversible/high-blast-radius commands) even though the design still tracks blast radius conceptually elsewhere in the lineage (attention-map ruling).

UP-3. P1: Decision 6 counts "catches" via "the existing caught-something log, which uses the trial plan's kill rule," and decision 20 ships the extraction net — the component explicitly built to surface missed prose decisions — "at step 6, after the v1 trial, so its own first weeks of use serve as its trial." That means the v1 two-week trial (whatever build steps are complete by then) is judged on catches from a system that doesn't yet include the net. The doc never states which build step the two-week clock starts at, so it's unclear whether v1's "catches" criterion is even testable, or is really just measuring the decisions-inbox itself for the trial window — worth stating explicitly so the trial doesn't get judged against a capability (the net) that literally isn't built yet.

UP-4. P1: Decision 18 makes a continuation kind mandatory on every option ("the filing helper requires a continuation kind on every option"), with agents expected to "learn to write continuations from the helper's prompts and worked examples." Across an estate where many different agents across ~98 projects file decisions, this is recurring classification overhead with real consequences if gotten wrong — a wrongly-labeled "command" continuation runs unattended on mk's pick (decision 14, "everything on pick"), and there's no stated validation/lint step at filing time (only a hash freeze, which locks in a mistake rather than catching it) nor a safe default (e.g., falling back to ruling-only or needs-context) for an agent that's unsure.

### Improvements

UP-5. Decision 15's v1-scope bullet ("v1's rail carries the primary leaks: decisions, focus and the estate picture") should get the same "(Cut, decision 19.)" annotation used elsewhere in the doc (e.g. under decision 6, decision 8) so a planner reading decisions in order doesn't carry stale scope forward.

UP-6. State explicitly, near "What we're building," which of the five listed capabilities (estate picture, decisions, decide, set focus, talk to companion) v1 actually ships by which build step — as written, a reader could believe v1 delivers "one place to direct attention/intention/cognition" when steps 1-4 deliver only decisions + map, with focus cut and companion deferred to step 6.

UP-7. Add a guard for chronic staleness in autarch-07: if the same decision re-stales every time mk opens it (fast-moving project), the stale-guard (step 4) as described has no exit other than the same re-ask loop — worth a small escalation (e.g., after N re-asks, surface as its own decision: "this keeps going stale, want to hand it to a thread instead?").

UP-8. Add an explicit path for a successful-but-regretted pick (not a continuation failure) — right now only "failed" reopens a decision; a human misclick under rail-skimming (UP-2) that runs a fine-but-wrong command has no described recovery beyond whatever the command itself allows.

UP-9. Specify (even loosely) how "the same passage" is matched for net dismissals — exact text, thread+turn id, or fuzzy similarity — since prose decisions are frequently paraphrased across a rotation's handoff turns, and a naive exact-match dedup will under-suppress exactly the noisy case the mute-threshold work (decision 20) is trying to measure.

UP-10. Clarify decision 20's "handoff files and commits are not scanned" against autarch-07 CUJ step 1's "closing turns, which include the handoff a rotation writes" — both can be true (closing chat turns vs. a separate committed handoff file) but the phrasing invites a planner to conflate them.
<!-- flux-drive:complete -->
