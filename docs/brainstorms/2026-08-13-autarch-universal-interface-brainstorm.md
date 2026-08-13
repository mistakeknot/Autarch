---
artifact_type: brainstorm
bead: none
stage: discover
---

# Autarch as the universal interface — one door, and the why behind each room

**Date:** 2026-08-13
**Status:** design captured, not planned
**Approach chosen:** A + C graft — product card and gate first (no UI), thin tmux-native door second, Bigend web as the public season surface third

## What We're Building

One entry point to every agent session mk runs, where each row is a **project**
rather than a session, and each row carries three things at once: live session
state, the project's stated why, and what outsiders have pledged against it.

The why is a **product card** — persona, pain point, CUJ, success metric,
guardrail — living in-repo as files so it travels with the code, diffs, and
survives Autarch. An agent drafts it on first sight from the repo itself and
marks it `PROVISIONAL`; mk's job is approval and correction, not authorship.
Nothing blocks exploration. `/write-plan` refuses to run until the card is
confirmed, and overrides are logged rather than forbidden.

Ranking is not a heuristic mk invented about themselves. It is fed by the
funded-attention mechanism on generalsystemsventures.com
(`gsvdotcom/docs/brainstorms/2026-08-12-funded-attention-brainstorm.md`): dollars
and donor agent-labor pledged against specific projects, in two columns that
never convert into each other.

Sessions attach to project rows. Attaching is `tmux switch-client` — all 21
sessions already live on one tmux server, so no session moves, nothing is
re-parented, and every terminal app keeps working as a client.

Coldwine and Pollard stay parked. Gurgeh's PRD machinery is the natural home for
card drafting but is not a phase-1 dependency.

## Why This Approach

**Because the overwhelm compounds, and one surface cuts two links.** Mk named all
three failures as feeding each other: can't find it → don't revisit it → forget
why it exists → can't rank it → work on whatever was last touched → the other 20
rot. Finding and remembering collapse into a single action if the switcher row
*is* the product card. A project with no stated why then shows that gap every
time mk opens the door, instead of never.

**Because the estate's PM vacuum is structural, not a discipline problem.**
Measured 2026-08-12 across 82 projects: 2,270 plans, 831 brainstorms, 388 PRDs,
126 CUJs, **36 decision records, 3 personas, 0 metrics documents**. Checked
fairly against PRD contents rather than directory names — of 375 unique PRDs,
14% state a success metric, 5% a pain point, 2% a CUJ, 1% a persona, 0.3% a
guardrail. Agents produce plans prolifically because `/write-plan` exists, is
invoked constantly, and yields a file. Nothing in the loop ever asks who the work
is for, so nothing answers. Autarch's own ratio is 103 plans to 1 ADR, and the
repo root already carries `OVERPLANNING_DETECTOR.md` and
`PREVENT_OVER_PLANNING_README.md` — this was diagnosed once and treated as a
discipline failure. It is a request-shape failure.

**Because funded attention supplies the one thing Autarch could not honestly
generate.** Any ranking function invented here would be mk's taste dressed as a
metric. Strangers pledging against specific projects is an external signal.
Three couplings follow, all load-bearing: the product card becomes the funding
page's sales copy, so the artifacts the estate has 3 of become the artifacts
without which the page is blank; "one bounded unit of attention" needs a receipt
generated from real session logs, and Autarch is already watching them; and the
public written veto is an ADR with an audience, which is the first thing that has
ever *required* a decision record.

**Because the gate must outlive the UI.** Putting the `/write-plan` gate in a
Clavain hook rather than inside Autarch means it fires whether or not the door is
open. A gate that only works when you are looking at it is a badge, and 2,270
plans are the evidence that ambient badges lose to momentum.

**Because Bigend is dormant, not absent — but reviving it first is the wrong
order.** `internal/bigend` (12,666 LOC) already has `statedetect/detector.go`,
`statedetect/nudge.go`, a daemon, an aggregator, and both web and TUI surfaces.
The classifier exists. But the repo does not currently build (`go mod tidy`
needed), discovery sees 1 of 21 sessions, and the vision doc already calls its
filesystem-scan-plus-tmux-scrape approach transitional. Proving the card format
against real projects costs days; opening a dormant 12.7k-LOC component inside a
155k-LOC repo costs an unknown. Do the cheap proof first, then spend the unknown
on a surface whose requirements are already settled.

## Key Decisions

| # | Decision | Ruling |
|---|---|---|
| 1 | The overwhelm | All three failures — finding, remembering, ranking — and they compound. Autarch must cut more than one link. |
| 2 | Unit of the row | **Project**, not session. Sessions are ephemeral and take their context to the grave; projects are durable. Sessions attach to project rows. |
| 3 | Enforcement stance | **Agent drafts, marks PROVISIONAL, mk confirms; `/write-plan` gates on confirmation.** Exploration never blocked. Hard gate on all agent work rejected — it would block 79 of 82 projects on day one, which is how gates get disabled. Overrides logged, not forbidden. |
| 4 | Where the why lives | **In-repo files**, with a derived index for ranking. Central-only storage would make a project's why invisible to the agent working inside it. |
| 5 | Gate location | **Clavain hook, not inside Autarch.** The gate must fire with the door closed. |
| 6 | Ranking input | **Funded attention** from GSV — pledged dollars and donor agent-labor, two non-converting columns — rather than a self-generated staleness heuristic. |
| 7 | Autarch's own success metric | Every funded item's bounded unit is **delivered and evidenced within its season, with the retrospective generated from session logs rather than written from memory**. Guardrail: a public veto with no written reason, or a hand-authored retrospective, is a miss. |
| 8 | Scope of revival | **Bigend and Gurgeh in, Coldwine and Pollard parked.** Bigend's web view is phase 3, as the public season page. |
| 9 | Attach mechanism | `tmux switch-client`. All 21 sessions are already on one server (`/private/tmp/tmux-501/default`, 27 clients across 5 terminal apps). Nothing is migrated; terminal apps remain clients. |
| 10 | Phasing | Phase 1 card + drafting agent + gate, no UI. Phase 2 thin tmux-native TUI on `pkg/tui`. Phase 3 Bigend web as public surface. |

**Prior art, in-tree.** `cujgel` (`/Users/sma/projects/cujgel`) already defines a
CUJ schema with `actor`, `trigger`, `mental_model`, `ambiguity_ledger`,
`implied_features`, `success_condition`, `provenance`, and 126 CUJs exist across
the estate. The card should adopt or subset this rather than invent a rival
format. Gurgeh is already "PRD generation and validation." Bigend is already
"multi-project agent mission control." No external prior art search was run for
this brainstorm: paseo and bb were evaluated earlier in the same session and
ruled out for this purpose — both manage their own agent processes, so adopting
either means abandoning long-lived tmux sessions and the `--resume` continuity
tied to their UUIDs, to buy a capability tmux already provides. Revisit paseo
separately and only for mobile access.

**Advisory shipped-state check.** Autarch has no `.beads` database; the nearest
is `Sylveste/.beads`. No epic search was run. `/clavain:strategy` Phase 0.5 must
run the enforced `subsume | supersede | orthogonal` check against the Sylveste
corpus before this becomes an epic — `2026-02-26-bigend-multi-project-brainstorm.md`
and its PRD are the obvious overlap candidates and were not read for this doc.

## Open Questions

1. **What exactly is on the card?** Persona, pain, CUJ, success metric, guardrail
   is the working set. Adopt cujgel's schema wholesale, subset it, or define a
   smaller card that references a cujgel CUJ by ID? The card is read at a glance
   in a TUI row; a full cujgel CUJ is not glanceable.
2. **What can an agent honestly draft?** Persona and pain are inferable from a
   repo. A success metric mostly is not — an agent guessing one produces exactly
   the fake rigor this is meant to prevent. Does the drafter leave metrics blank
   and say so, and does a blank metric block `/write-plan` the same as a missing
   card?
3. **Backfill policy for 82 projects.** Draft cards for all of them at once, or
   only on first touch? Bulk drafting produces 82 provisional cards mk will never
   review, which is a new backlog wearing a rigor costume.
4. **What does the row rank by when nothing is pledged?** Funded attention covers
   25 slate items. The other 47-plus projects need an ordering, and that ordering
   is back to being mk's taste. Explicit manual pinning may be more honest than a
   formula.
5. **Cross-machine scope.** zklw runs sessions too. Same door, or separate space?
   The tmux-server unification argument does not cross the network.
6. **The 1-of-21 blindness.** `intermux` reports one session, mis-parses it, calls
   it crashed while its PID is alive, and discloses no coverage gap. Whatever
   enumerates the fleet must state coverage as a fraction and name what it could
   not parse. Unresolved: whether `[` versus `]` in session names carries meaning
   — mk's convention to rule on — and whether the terminal-app prefix should be
   retired from the naming contract once the door exists.
7. **Coupling risk.** This ties an unfinished 155k-LOC internal tool to a public
   mechanism with money in it. If Autarch phase 2 slips, does the funding page
   still work? Phase 1 is deliberately UI-free partly to answer yes — but that
   answer should be stated as a constraint, not assumed.
8. **Prompt-injection surface.** GSV open question 4 — donor agents submitting
   work packets — becomes an Autarch concern the moment Autarch displays or
   routes those artifacts. Untrusted content reaching a tool mk drives all day is
   a different threat model than untrusted content on a website.
