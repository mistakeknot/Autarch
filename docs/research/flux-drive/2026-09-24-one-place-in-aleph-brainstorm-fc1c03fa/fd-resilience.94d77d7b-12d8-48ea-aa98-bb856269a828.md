<!-- run-uuid: 94d77d7b-12d8-48ea-aa98-bb856269a828 -->

### Findings Index
- P2 | R1 | Decision 18 (Home signing key) | Single point of failure: signing key not backed up; rotation procedure risks bricking in-flight decisions
- P1 | R2 | Decision 13 (Continuations) | Half-run continuations: no recovery defined when command runs but ruling write fails
- P2 | R3 | Open Question 5 (Hook latency) | Feed hook failure is undefended: crashes or Uqbar loss leaves turn start hanging without graceful degradation
- P1 | R4 | Decision 3 (Hub tracker beads) | Hub tracker loss is unrecoverable: no fallback when Dolt at 127.0.0.1:3311 fails; decisions become inaccessible
- P2 | R5 | Decision 10 (autarch serve consolidation) | autarch serve is a new single point of failure; no degraded mode when process crashes; Home shows nothing
- P1 | R6 | Decision 17 (Fire-and-forget) | Missing backpressure: continuations run with no ack, so race between conclusion and feed delivery leaves agent and mk in divergent state
- P2 | R7 | Decision 16 (Ruled files) | Signed rulings asymptotically accumulate in Uqbar; no retention or compaction strategy for 98 projects over time

Verdict: needs-changes

### Summary

The design concentrates responsibility for the estate's decision surface into a few fragile points with no fallback. Home's signing key, the autarch serve process, and the hub tracker are each capable of losing in-flight work or rendering the entire decision subsystem inaccessible. The feed hook runs on every prompt but has no graceful degradation when Uqbar or Lattice are unreachable, risking a blocked turn. Continuations are fire-and-forget with no ack, so a ruling write failure leaves agents believing decisions closed when the decision is actually still open—a consistency gap. The design is optimized for the happy path and needs antifragility built in at each hard dependency.

### Issues Found

R1. **P2: Home signing key is a single point of failure without backup or recovery.** Decision 18 specifies Home's ed25519 key lives only on zklw and is used for nothing else; rotation adds new key with `valid-after` and expires old; no mention of backup, escrow, or recovery if the key is lost or the machine dies. If zklw goes down and the key is not replicated, all in-flight decisions whose continuations have already run are stranded: their rulings cannot be signed and the beads remain open.

R2. **P1: Continuation execution and ruling write are not atomic; failure after command runs loses the decision outcome.** Decision 13 says the command runs, the brief starts, or needs-context wakes the thread, then "the ruling is written as a signed file...and the bead closes." No mention of what happens if the command succeeds but the ruling write fails (key unavailable, Uqbar unreachable, disk full). The bead stays open, the continuation re-runs on retry, and the command may execute twice. Agents that ran fire-and-forget continuations do not learn of the failure unless they poll the bead.

R3. **P2: Feed hook has no graceful degradation; cache invalidation failures will block or hang turn starts.** Open question 5 states the hook needs a per-project cache invalidated when a bead closes or Uqbar receives a commit, but does not specify what happens if: (1) the cache layer fails; (2) Uqbar is unreachable during invalidation; (3) Lattice query fails; (4) the hook exceeds a time budget. Hooks that block turn starts risk starving the entire estate. No mention of timeout, fallback to stale cache, or silent skip if the hook is unavailable.

R4. **P1: Hub tracker (Dolt at 127.0.0.1:3311) has no redundancy; loss is unrecoverable.** Decision 3 records decisions as beads in the hub tracker. No mention of backup, standby, or recovery if the Dolt instance fails. If 127.0.0.1:3311 is down, the decision-filing helper fails, Clavain hooks cannot inject the feed, and Home cannot query the decisions inbox. The filing helper does not degrade to a fallback (plugin store mentioned in decision 15 as a contingency, but not wired). Agents awaiting decisions see nothing; decisions in flight have no visibility.

R5. **P2: autarch serve consolidation creates a new single point of failure with no degraded mode.** Decision 10 consolidates Bigend daemon, Gurgeh, Signals, and MCP into one `autarch serve` started by the plugin. If the process crashes, Home's map, decisions, and rail are all unavailable. No mention of: restart policy, health checks, stale data fallback, or graceful degradation (e.g., showing cached map from last successful run). Plugin startup or Home rendering likely assume serve is running; no defensive check.

R6. **P1: Fire-and-forget continuations have no ack; ruling write failure leaves async gap.** Decision 17 states decisions are "fire-and-forget"; the thread learns from the feed, not from a wake. This means: (1) the asker does not block waiting for the answer; (2) there is no ack back to the asker that the ruling was written. If the feed hook fails or is slow, the agent may attempt retry before the feed arrives, creating a race. If the ruling write fails, the agent has no direct notification; it learns only if it polls the bead or sees the feed re-raise the decision. This violates causality: the agent's work (e.g., a spawned thread) may have already committed based on the ruling, but mk sees the decision still open.

R7. **P2: Ruling files accumulate with no retention or compaction strategy.** Decision 16 specifies rulings go to `Uqbar/rulings/YYYY-MM-DD-<slug>.md` and `focus/current.md` (now cut), with git history as the timeline. Over time, this grows unbounded. No mention of: archiving old decisions, pruning irrelevant project-level rulings from the per-project `docs/decisions/`, or compaction. The estate has ~98 projects × several decisions/year = hundreds of files. Querying the feed becomes an O(n) walk over git history; Lattice indexing may not catch all ruling mutations. No SLA on feed query latency.

### Improvements

I1. **Backup Home signing key to escrow or a rotation ceremony.** Specify a key rotation and disaster recovery procedure: a second operational key in sealed storage on a separate machine, with automated restore if the primary is lost. Alternatively, a ceremony to re-sign old rulings if the key is rolled. This keeps the single-signing property while avoiding total loss.

I2. **Atomize continuation and ruling write; add idempotency.** After a command runs, write the ruling and close the bead in the same transaction, or with an idempotent retry loop. If the write fails, keep the bead open and re-raise it to the rail as a failed continuation (decision 18 mechanism). This prevents double-execution and ensures the bead state and the agent's state stay consistent.

I3. **Add timeout and fallback for the feed hook; do not block turn starts.** Give the hook a budget (e.g., 500ms). If it times out or errors, skip injecting the feed and log the miss; the next turn will retry. Or: inject a cached feed if available, marking it stale. This prevents the hook from becoming a single point of failure that blocks every turn.

I4. **Implement fallback for hub tracker loss.** Specify the fallback: either (1) write pending decisions to a plugin-local SQLite store and resync when tracker recovers (simplest), or (2) keep a hot standby tracker. Without this, loss of 127.0.0.1:3311 freezes the estate.

I5. **Add health checks and restart policy for autarch serve; implement graceful degradation.** The plugin should periodically ping the serve process. If it's down, Home should render the last cached map/decisions (with a "stale" badge) rather than error. Or: render a "serve unavailable, decisions cached, refresh to retry" message. This keeps Home usable during serve restarts.

I6. **Add backpressure or ack for continuation outcomes.** The asker should write its own ack to the bead (e.g., a "received" field in the bead) after it acts on a feed line. The feed hook can then check that continuations have been ack'd; if not, re-inject them. This closes the async gap between "ruling written" and "agent saw the feed."

I7. **Implement ruling file pruning and Lattice query optimization.** Define an archive strategy: move rulings older than N months to an archive branch, or compact them into summary files per project per quarter. Index only active rulings in Lattice, or add a "archived: true" field. This keeps the feed query fast and prevents the Uqbar from becoming a historical archive that slows every turn.
<!-- flux-drive:complete -->
