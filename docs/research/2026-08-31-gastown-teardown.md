# Gas Town / Wasteland teardown — the factory next door

```yaml
artifact_type: teardown
evidence_class: mixed — primary (repo README, yegge.ai docs) for architecture;
  secondary (search synthesis) for the v1.0 retrospective, Medium bot-walled;
  primary fetch of a third-party critique (fkxx.substack.com)
sources_current_as_of: 2026-08-31   # gastown pushed 2026-08-19, 17.9k stars
relation_to_estate: >
  Gas Town runs on tmux + beads (bd) + git worktrees + Claude Code — mk's exact
  substrate. mk's `bd` tracker IS Yegge's beads. The estate is a proto-Gas-Town
  without the orchestration layer; stealing is unusually cheap, and the
  commoditization signal is unusually loud.
```

## What they are

**Gas Town** (github.com/steveyegge/gastown, Go, 17.9k★, active): a
multi-agent workspace manager. A Town (`~/gt/`) holds Rigs (project
containers wrapping git repos). The **Mayor** is the chief-of-staff agent;
**Polecats** are workers ("persistent identity, ephemeral sessions");
**Crew** is the human's own workspace; a three-tier watchdog watches everything
(**Witness** per rig, **Deacon** patrolling all rigs, **Dogs** dispatched for
maintenance); the **Refinery** is a Bors-style merge queue with verification
gates. Work is beads; bundles are **Convoys** (a `mountain` label adds
autonomous stall detection); multi-step workflows are **Molecules** poured from
TOML **Formulas**. **Seance** lets a session interrogate its predecessors via
`.events.jsonl`. **"Landing the Plane"** is the mandatory session-close ritual.
Scale claim: 4–10 agents is chaos unmanaged; Gas Town makes 20–30 comfortable;
**Gas City** (declarative successor, cloud) claims hundreds.

**Wasteland** (github.com/steveyegge/wasteland, Go, **19★, dormant since
March**): the federation protocol — towns post wanted items, claim work across
towns, and validators issue multi-dimensional **stamps** (portable reputation)
over DoltHub. The network layer is aspiration, not evidence: even Yegge can't
bootstrap it yet. Single-town value first.

## The retrospective (his own honesty, secondary-sourced)

- The "22-nose Clown Show": the Mayor suffered massive data loss twenty-two
  times over weeks. Fix: move state into Dolt. **Lesson: the coordinator must
  hold no state.** (The door already obeys this — it transcribes card-check
  and tmux, owns nothing. Keep it that way forever.)
- "Serial killer sprees": something killed random workers mid-job.
  **Supervision itself can be the hazard** — watchdogs need guardrails.
- Resilience emerged from interweaving, not correctness: many overlapping
  agents "power through almost any error."
- Community arc: ~50 contributors, ~100 PRs, 12 days to stabilize v1.0.

## Lessons mapped to the journey steps / Stellaris axes

| Lesson | Gas Town mechanism | Confirms / extends |
|---|---|---|
| Interrupts climb a ladder, pre-triaged | Escalation: severity-routed (P0/P1/P2) beads through Deacon → Mayor → **Overseer (the human)** | Confirms Stellaris delivery-classes (B/H); adds shape: the human is the *top of an escalation hierarchy*, not a notification target |
| Stuck-detection is a role, not a feature | Witness/Deacon/Dogs — entire agents dedicated to watching agents; convoy stall detection | Confirms the waiting-on-me axis; extends Claude-Squad scraping to institutional form |
| Orientation can be conversational | Seance: query predecessor sessions for "what did you find?" | Extends orientation-on-entry (A/G): the catch-up briefing can interrogate the dead, not just list events. mk already has session archives (cass) to build this on |
| Parking is a ritual, not a habit | "Landing the Plane" — structured close-out phase | Journey step 8, institutionalized; mk's session-completion protocol is the same instinct with less enforcement |
| Merges are infrastructure, not ceremony | Refinery: batched, verification-gated, bisecting queue; failed MRs isolated and re-dispatched | The landing saga (PR #15's three-day dance) as a solved queueing problem |
| Work state lives in the ledger, never the context window | beads + git-worktree "hooks"; "durable memory plus parallelism win"; humans audit the ledger | Why cards/beads on disk are the right substrate; the door's statelessness is correct |
| Dispatch needs a governor | Scheduler: capacity-capped polecat dispatch vs rate limits | Future Mycroft concern, noted |

## What NOT to take

- **The factory frame.** The sharpest critique (fkxx.substack.com, "What Gas
  Town Got Right (And What It Can't See)"): Gas Town's unit is tasks completed,
  its measure is throughput — "it answers *how to build faster*, not *how to
  think better*." There is "a layer above the factory" of crystallized
  understanding — and that layer is precisely what Autarch's cards build. The
  most-cited critique of the biggest project in this space names, as its blind
  spot, the thing this estate is already building. "The prompt is not the
  spec. The conversation is not the architecture."
- **The full role bureaucracy at solo scale.** Mayor/Deacon/Dogs earn their
  keep at 20–30 concurrent agents with 50 human contributors. mk's estate is
  one gardener; the ladder shape transfers, the org chart does not.
- **Wasteland federation.** 19 stars, dormant. Ignore — though its *stamps*
  (multi-dimensional attestation of work) rhyme with card ratification: mk's
  confirmation is a stamp, human-issued, already shipped.

## Sources

- https://github.com/steveyegge/gastown (README, primary)
- https://yegge.ai/gastown (his docs, primary)
- https://github.com/steveyegge/wasteland (primary, metadata)
- Retrospective fragments via search synthesis of
  steve-yegge.medium.com "Gas Town: from Clown Show to v1.0" and
  "Welcome to the Wasteland: A Thousand Gas Towns" (Medium bot-walled;
  secondary)
- https://fkxx.substack.com/p/what-gas-town-got-right-and-what (critique)
- https://thenewstack.io/steve-yegges-ai-agent-orchestration-project-gas-town-comes-to-the-cloud-and-brings-the-wasteland-with-it/ (Gas City context; paywalled shell, title-level only)
