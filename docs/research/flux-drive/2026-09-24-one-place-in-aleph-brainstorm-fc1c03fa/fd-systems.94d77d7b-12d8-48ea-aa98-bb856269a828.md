<!-- run-uuid: 94d77d7b-12d8-48ea-aa98-bb856269a828 -->

### Findings Index
- **P1** | SYS-001 | "Fire-and-Forget at Scale" | Decision-filing cost asymmetry creates unbounded rail growth
- **P1** | SYS-002 | "Rail Saturation Without Focus" | 98-project estate feeds single decision rail with no filtering in v1
- **P1** | SYS-003 | "Goodhart on Catches" | Success metric (catches) incentivizes behavior that defeats its own purpose
- **P2** | SYS-004 | "Hook Latency Under Load" | UserPromptSubmit feed runs on every turn; cache invalidation becomes token tax when mk is away
- **P2** | SYS-005 | "Stale Continuation Retry Loop" | Failed command → re-ask → mk picks again → risk of duplicate work and attention thrash
- **P2** | SYS-006 | "Decision Duplication Via Net + Agents" | Same decision caught by both net and filed directly → two beads → one orphaned on rail
- **P3** | SYS-007 | "Successor Resolution Unbuilt" | Needs-context continuation wakes wrong thread if successor lookup (aleph-05) is incomplete

Verdict: **needs-changes**

### Summary

The design correctly identifies that decisions buried in chat are a primary leak and builds a sound token-efficient mechanism to capture and answer them. However, it introduces three reinforcing loops that compound under scale:

1. **Fire-and-forget asymmetry:** filing becomes so cheap agents may overuse it; rail grows faster than mk can pick. Without focus (cut from v1), there is no steering valve.
2. **Metric inversion (Goodhart):** success is measured by catches (decisions surfaced), which trains agents to hide them where the net can't look, defeating the metric.
3. **Hook becomes tax:** the ruling-feed hook runs on every turn in every thread; when mk is away, cache invalidation costs accumulate without picking any decisions.

Focus was cut because it seemed "overly ornate," but it was the only system lever that balanced the fire-and-forget mechanism. With v1 on the rail alone, the system lacks a throttle and will reach inflection within weeks.

### Issues Found

**SYS-001 (P1): Fire-and-Forget at Scale — Decision-Filing Cost Asymmetry**

Decision 14 states "everything on pick": the filing helper makes one call, the asker moves on, and continuations own the follow-through. This is token-efficient for mk (no context wake on filing). However, it creates a cost asymmetry: filing a decision costs the agent ~10-50 tokens; answering it costs mk ~1 second of attention, but only if it lands on the rail and mk picks it.

If agents discover that filing is cheaper than asking in chat, they will file more aggressively. Example: an agent at a fork can spend 30 tokens asking mk and waiting, or 15 tokens filing a decision and continuing. The agent chooses filing. When all ~20 agents across the estate make this choice, the rail grows from "5-10 per day" to "30-50 per day." mk checks Home less frequently than that. Decisions begin piling up. Oldest-first ordering becomes a blunt instrument: old decisions for archived projects sit next to new urgent ones.

**Evidence:** Decision 3 and the CUJ autarch-07's first step show agents filing and moving on. No retry cost, no prompt to ask first. The cost to agents is frontloaded; the cost to mk is distributed over Home-check frequency. At 98 projects with ~20 agents, the rail is a funnel with a fixed drain (mk's attention budget).

**Risk:** Without focus (cut in decision 19), there is no gate on machine-started work into out-of-focus projects. All 98 projects feed the same rail. This is the scale trap mentioned in the prior review (fd-systems ESTATE_SPRAWL). Within 4-6 weeks, the rail becomes a pressure-relief valve mk checks but doesn't enjoy.

---

**SYS-002 (P1): Rail Saturation Without Focus — 98-Project Estate, One Undifferentiated Rail**

Decision 19 cuts focus from v1, reasoning that most estate work is started by mk and Mycroft does not self-dispatch below T2. This is true of TODAY. But the design is self-referential: focus was meant to gate Mycroft's proposals (decision 5 original, autarch-08). Removing focus assumes Mycroft stays at T0–T1. However, the design also says "After Them and Aleph; park the rest" — this is precisely focus. It is just deferred.

The v1 rail has no concept of "focused" vs. "parked." It shows all decisions oldest-first, tagged by project. But with no focus bar, mk has no way to say "I only want to see decisions for three projects this week." mk either:

1. Checks Home and scrolls past irrelevant decisions (attention tax).
2. Stops checking Home when the list grows long (decisions rot unanswered).
3. Asks agents not to file decisions for parked projects (requires distributed coordination; agents learn to ignore instructions as the system grows).

None of these scale. The second-order effect: without focus, mk under-uses Home because the signal-to-noise ratio declines. Decisions that SHOULD surface for focused projects are buried in the noise of parked projects, and mk learns not to trust the rail.

**Evidence:** Decision 19 quotes mk: "I really just want one place to direct my attention/intention/cognition." Focus was directional intention. Cutting it in v1 leaves intention unimplemented. The trial will measure "catches" (what was surfaced), not "attention spent well" (what mk actually decided to do). The metric misses half the goal.

---

**SYS-003 (P2/P1 hybrid): Goodhart on Catches — Success Metric Perversely Incentivizes Evasion**

Decision 6 and the autarch-09 CUJ define trial success as "catches": decisions mk would have missed that the net surfaces. The net reads thread endings. Success is counted in the caught-something log.

Agents quickly learn: "the net catches decisions left in thread endings. To avoid being caught, file decisions before the thread ends, or don't use prose at all, or hide the decision in a sub-thread that won't rotate."

More broadly: agents learn that being caught (i.e., failing to file or record a decision formally) is what gets counted. To optimize for "not being caught," they file more aggressively to get ahead of the net. The catches metric was meant to surface the gap; instead, it trains the system to hide the gap.

**Evidence:** The net is positioned as a safety net for missed decisions (autarch-09, step 1: "reads thread endings"). But the metric is only "confirmed catches." An unconfirmed proposal mk dismisses is not logged as a catch—it is logged as a dismissal. mk may dismiss dozens of valid proposals because they are low-priority or stale by the time mk sees them. The log shows "dismissals: 200, catches: 5." This looks like the net is mostly noise. In truth, the net is surfacing real decisions mk never would have seen, but mk is dismissing them in bulk. The metric inverts: the net's job was to reduce mk's missed decisions; instead, it trains agents to avoid prose decisions, and mk learns not to trust the rail. The system "succeeds" by its metric (low dismiss rate) while failing at its goal (surface the real leaks).

**Root cause:** The catch is measured only when mk confirms. Dismissals are silent. No loop exists to ask "are we dismissing real decisions, or is the net just noisy?" The system has no corrective feedback; it only has the metric.

---

**SYS-004 (P2): Hook Latency as Hidden Token Tax — UserPromptSubmit Feed Under Load**

Decision 17 says the ruling feed hook runs on every turn's start. It reads rulings relevant to "this project only" and injects one line per item. Decision 5 (open question 1) names the challenge: "Hook latency. The feed runs on every turn, so it needs a per-project cache that is invalidated when a bead closes or Uqbar receives a commit."

Here is the system under load:

- mk is away for one week.
- During that week, ~50 threads run across the estate, averaging ~5 turns per thread.
- Each turn starts with the feed hook checking: "what rulings apply to this project?"
- The cache is invalidated whenever a bead closes or Uqbar receives a commit.
- In a week where mk is picking decisions daily, that is 10-20 cache invalidations per day (one per decision + one per Uqbar commit per decision ruling).
- Each cache invalidation requires a fresh query over the Uqbar git history and the beads tracker.
- That is 7 days × 50 threads × 5 turns × ~0.3 second (git query + beads query + serialize + inject) = 5250 seconds of server time.
- Most of those turns are idle-wait; the agent is not doing useful work while the hook runs.

If the cache is aggressive (long TTL), rulings go stale: an agent picks a decision on Monday, but a new thread starting Tuesday doesn't see the ruling until the cache expires. Agents drift out of sync with mk's decisions.

If the cache is conservative (short TTL or invalidated on every commit), the hook becomes a hidden token and latency tax on every turn in the system, especially during the week mk is away and can't pick decisions to clear the queue.

**Evidence:** Decision 17 mentions the gap: "bb thread queue cannot deliver without waking the thread. Its modes are only auto and steer, and a message queued to an idle thread is sent immediately." This is why the feed hook exists—to inject rulings without waking the thread. But without a solution to cache invalidation (open question 1), the feed hook becomes a synchronization bottleneck.

---

**SYS-005 (P2): Stale Continuation Retry Loop — Failed Commands Reopen Decisions**

Decision 18 introduces a guard: "Stale decisions have two guards. Home marks a decision stale on the rail. Every pick also re-checks the continuation's hash and preconditions; if they fail, the pick becomes a re-ask."

Scenario: An agent files a decision with a continuation command: `deploy-branch --branch mybranch`. mk picks the decision. The command runs. But it fails (branch was deleted). The ruling is written with state `failed`. Decision 18 says "a failed continuation reopens a decision": the decision goes back on the rail and mk must "retry, pick another option, or hand it to a thread."

mk picks again. The hash has changed (the code on disk moved). The re-check fails. The pick becomes a "re-ask." mk sees the decision again, but now they are not sure if the first pick ran, or if they are seeing a corrupted state.

Multiply this: an agent proposes 5 decisions, 3 of which have commands that depend on transient state (branch exists, a file path is valid, etc.). One week later, mk picks all 3. Two of them fail. mk picks them again. They fail again. mk is now in a retry loop, spending attention on decisions that have a low success probability.

**Consequence:** The retry loop is attention-consuming and demoralizing. Each re-ask costs mk 3-5 seconds of re-reading and re-deciding. If 10% of decisions have flaky continuations, mk is spending ~30 seconds per morning session re-trying old decisions. Over a month, that is ~2 hours of mk's time chasing ghosts.

**Evidence:** Decision 18 introduces the re-check, but there is no exponential backoff, no "give up after N retries," no "mark as stale and hide." The re-ask is permanent. The decision lives until mk decides to hand it to a thread, which breaks the token-efficiency goal.

---

**SYS-006 (P2): Decision Duplication — Net and Agents Both File**

The net proposes decisions from thread endings (autarch-09). Agents also file decisions directly (autarch-07). It is possible for both to propose/file the same decision:

1. Agent A makes a decision in prose at turn N-1: "we should use approach 2."
2. Agent A's turn ends.
3. The net reads the thread ending and proposes: "looks like a decision: 'we should use approach 2.'"
4. Meanwhile, Agent B in a related thread files the same decision: "we should use approach 2. Recommendation: option 2."
5. mk sees two proposals/beads on the rail.
6. mk confirms the net proposal (turns it into a bead).
7. mk picks the agent-filed bead.
8. mk closes both with ruling "approach 2."
9. Now there are two ruling files for the same decision, with the same state, but separate beads.

The second-order effect: if the agent's bead had continuations, both ran (or one did and the other didn't). If they had conflicting continuations (one says "deploy," one says "skip"), which one won? The system has no deduplication rule.

**Consequence:** The dual-filing creates reconciliation work downstream. Agents or mk must manually check: "are these the same decision?" This is especially risky if one continuation ran and the other didn't.

**Evidence:** Autarch-09 says "each confirmed catch is counted in the caught-something log." But if mk sees the same decision twice and dismisses one and confirms the other, the log shows a catch, but the system silently filed two beads. No visibility into deduplication failures.

---

**SYS-007 (P3): Successor Resolution Unbuilt — Needs-Context Continuation Risk**

Decision 13 says "needs-context" continuation wakes the original thread "or its successor if it has rotated." This requires aleph-05 (successor resolution) to be built and working. The CUJ autarch-07, step 4, mentions "successor resolution after rotation (aleph-05)" as a build surface, but the spec is marked as unbuilt.

If a decision continuation is "needs-context" and the thread rotates between when the decision is filed and when mk picks it:

1. Agent A files a decision with continuation "needs-context."
2. Agent A's turn ends, thread rotates to Agent B.
3. mk picks the decision.
4. Autarch tries to wake the original thread (Agent A's context).
5. If successor resolution fails, the wake goes to the wrong thread, or no thread, or times out.

**Consequence:** mk's pick doesn't work. The answer doesn't reach the agent who filed the decision. That agent never learns their decision was answered. They may file it again. Or they assume it was rejected and pick a different path.

This is marked P3 rather than P2 because the risk is only if successors are both running when a decision is picked AND that decision has a needs-context continuation. The common case (decisions picked within one turn, or continuations are command/brief) avoids the risk.

---

### Improvements

**IMP-001: Add a simple Goodhart guard to the net.**

Log each confirmed catch WITH the reason mk confirmed it: "relevant," "stale," "already ruled," "redirect to thread." Track dismissals the same way. At the end of the trial, analyze which categories are increasing. If dismissals rise while catches stay flat, the net is surfacing stale decisions faster than mk can process them. Adjust the mute threshold (autarch-09, decision 20: "No mute threshold up front; decide after the trial") to surface fewer low-confidence proposals.

**IMP-002: Restore a minimal focus gate for v1.**

Focus was cut for being "ornate," but the core is not ornate: one list (focused projects), one gate (hold turns into out-of-focus projects), one nudge (one line when filing new work). This is the "lean version" mentioned in decision 19. Without it, the rail scales linearly with estate size. With it, the rail scales with mk's declared focus. The trial will show whether this steering is needed; if not, it can be removed post-trial.

**IMP-003: Name a cache coherence strategy for the ruling feed.**

Decision 5's open question 1 is unresolved. Before planning, decide: (a) cache is invalidated only on bead close (cheap, but stale rulings linger); (b) cache is invalidated on Uqbar commits (expensive under heavy picking); (c) the hook uses a bloom filter to quickly rule out "no rulings for this project" (reduces cache misses); or (d) the hook is replaced with a different delivery mechanism (e.g., a polled queue instead of a hook on every turn).

**IMP-004: Add exponential backoff and a "give up" rule to failed continuations.**

Decision 18 reopens a decision owed when a continuation fails, with no retry limit. Add: after 3 re-asks, the decision is marked "blocked" on the rail and mk must either hand it to a thread or close it manually. This prevents the retry loop from accumulating indefinitely.

**IMP-005: Deduplication check for the net.**

Before confirming a net proposal, check: does a bead already exist with the same question/project? If yes, offer mk: "This looks like [existing bead]. Merge into it, or create a new one?" This catches the case where both the net and an agent file the same decision.

<!-- flux-drive:complete -->
