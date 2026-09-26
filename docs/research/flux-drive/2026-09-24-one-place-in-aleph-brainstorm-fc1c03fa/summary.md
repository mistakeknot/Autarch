# Flux-drive review: one place in Aleph (brainstorm)

- Run: `94d77d7b-12d8-48ea-aa98-bb856269a828`, 2026-09-25
- Input: `docs/brainstorms/2026-09-24-one-place-in-aleph-brainstorm.md`
- Agents: fd-architecture, fd-systems, fd-safety, fd-user-product, fd-decisions,
  fd-resilience. fd-perception was deferred. All ran on the session model
  (Opus 5.5), with no model routed per agent.
- Synthesis: intersynth:synthesize-review wrote `findings.json` (verdict
  risky). The orchestrator then checked the claims below by hand.
- Verdict after that check: **needs-changes**. One P0 stands. Nothing
  invalidates the design.

## Surviving findings

**P0, confirmed: Home's signing key sits in the same trust domain as every
agent** (SF1, SF2)
- Every agent on zklw runs as `mk` (19 of 19 claude/codex processes at
  review time), so any of them can read a key file in mk's home and sign a
  "ruled" file. This covers continuation commands too.
- `allowed_signers` lives in the Uqbar, and agents can push there, so an
  agent could also add its own key.
- As written, decision 18 does not deliver "no agent can make one look
  ruled". The signer needs a boundary agents cannot cross, for example:
  - a separate Unix user or service that signs only in response to a pick
    made in Home;
  - changes to `allowed_signers` accepted only when signed by mk's personal
    key.

**P1, confirmed: two decision queues** (ARCH-1)
- Mycroft's in-process `DecisionQueue`
  (`internal/mycroft/escalate/escalate.go`) is still separate from the bead
  design.
- The 09-22 review raised the same point.
- Decisions 2 and 3 need to say whether Mycroft's escalations become
  decision beads.

**P1, confirmed: what the hash freezes** (SF3, SF4)
- The hash covers the command string only. It does not cover cwd, env, or
  the scripts the command calls.
- No sandbox or per-project scope is stated.
- Decision 14 should name what is frozen: working directory, environment
  allowlist, and the hashes of any scripts referenced. It should also name
  what a continuation runs as.

**P1, confirmed: feed text is agent-authored** (SF5)
- Ruling lines from the feed are injected into other agents' turns.
- Decision 17 should inject only fields mk ruled on (the option label and
  the outcome), quoted and clipped, and never free text from the asker.

**P1, confirmed: running a continuation and writing its ruling are not
atomic** (R2)
- A crash between the two can run the command twice or leave it unrecorded.
- A pick needs an idempotency key, with the order write intent → run →
  record outcome.

**P1, plausible: hub-tracker loss** (R4)
- If Dolt at 3311 is down, the rail is empty and filing fails.
- The design should say what fails safe: filing falls back to asking in
  chat, and the rail shows "tracker down" rather than "nothing owed".

**P1, confirmed: `autarch serve` and step 1** (ARCH-2, ARCH-3)
- `autarch-mcp` speaks stdio, unlike the three HTTP daemons it would be
  merged with.
- Step 1 should state that it needs only the decisions RPC, not the whole
  consolidation.

**P2: rail volume at about 98 projects** (SYS-001, SYS-002, UP-1)
- This is real, and it is already open call 2.
- The recommendation to restore focus conflicts with ruling 19, so it is
  recorded here but not proposed as an action.
- A cheap measure fits the lean rule: count decisions filed and
  time-to-pick per week during the trial, and add a revisit trigger to
  decision 19.

**P2, downgraded: the source of catches** (UP-3)
- The net ships after the trial, but trial catches do not depend on it.
- The trial plan already logs manual `estate catch` entries from the map
  and catch-up (`[mk-C]`).
- Decision 6 should still name which v1 surfaces are expected to produce
  catches.

**P2: other findings**
- UP-4: no default when a continuation kind is misclassified.
- UP (no ID): no undo after a pick that succeeded but was wrong.
- ARCH-4, ARCH-5: preferences state, and the feed hook sitting in Clavain's
  lane.
- Stale wording in decision 5 and in the v1 scope of decision 15 after
  decision 19.

## Rejected

- **KILL-RULE-UNDEFINED.** The rule is in
  `docs/plans/2026-09-23-estate-map-trial-plan.md` (`[mk-K]`).
- **AUTARCH-08-COMPLEXITY-TRAP.** autarch-08 was never validated; its status
  is `draft`. The validation walk is where mk caught the ceremony, so the
  gate worked.
- **SYS-003 (the Goodhart point).** It assumes agents know about the net
  and avoid it. Nothing tells them, and the net only reads thread endings.
- **Synthesis gate "FAIL / risky".** It counted ARCH-1 as P0. It is a
  carried-forward reconciliation, not a flaw in the new design.
