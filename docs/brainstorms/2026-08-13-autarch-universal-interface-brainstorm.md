---
artifact_type: brainstorm
bead: none
stage: discover
---

# Autarch as the universal interface — one door, and the why behind each room

**Date:** 2026-08-13
**Status:** phase 1 built. Decisions 11–13 added and open questions 1–4 closed after a
prototype and a five-repo measurement.
**Approach chosen:** A + C graft — product card and gate first (no UI), thin tmux-native door second, Bigend web as the public season surface third

**Phase 1 artifacts**

| What | Where |
|---|---|
| Card format | `docs/reference/product-card-format.md` |
| Drafter procedure | `docs/reference/card-drafter.md` |
| Five-repo measurement | `docs/analysis/2026-08-13-card-drafter-measurement.md` |
| Validator | dotfiles `common/.local/bin/card-check.py` |
| Plan gate | dotfiles `common/.claude/hooks/guard-plan-needs-card.sh` |
| Suites + mutation run | dotfiles `common/.claude/hooks/tests/` |
| Prototype | `https://claude.ai/code/artifact/a6e982de-4eb1-4f5d-856f-dedc5309435a` |

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
| 11 | Row ordering | **Funded first, then pins, then weakest card first.** Mk's pick, 2026-08-13. The unfunded tail is ordered by how little of its card is real, so the estate's gaps rank themselves instead of a staleness formula standing in for taste. Only `confirmed` fields count toward strength. Known cost, accepted: the worst-documented project sits permanently on top. |
| 12 | Backfill | **On first touch, never in bulk.** Not a sequencing preference — a constraint. See below. |
| 13 | A project with N agents | **One row.** `jeddnet` runs a claude session and a codex session; that is one project with two attached sessions, and decision 2 already settled that the row is the project. The prototype rendering it as two sibling rows was the prototype contradicting decision 2, not a new question. The row shows an agent count; the card panel lists the sessions. |
| 14 | Confirm-session scope | **A design question surfaced while confirming a card is parked as a bead, never followed in-session.** Discovered by violating it — see Gate A below. Reading a why hard enough to write it truthfully is a good way to find live design holes, and following one is the correct engineering response and the wrong *session* response. At 82× the difference between parking and following is the difference between a week and a quarter. |
| 15 | What Autarch is for | **Markdown document review and editing, plus agent development guidance.** Mk, 2026-08-14, mid-Gate-A: *"Autarch needs to be really focused on markdown document review/editing along with agent development guidance."* Sharpens decision 1 from a symptom (overwhelm) to a mechanism. The card loop is not a feature of Autarch — it is the archetype of what Autarch does, and Gate A is the first end-to-end instance of it. See below. |

**Ruling 12 in full — why bulk backfill is a constraint and not a preference.**
Two independent measurements say the same thing, and neither was available when
open question 3 was written.

*From the prototype:* in the true present state every one of the 82 rows reads
`⚠ no why stated`. A signal that fires on 100% of rows carries no information —
the column is invisible within seconds. Bulk-drafting 82 cards does not fix that;
it replaces a uniform `missing` column with a uniform `provisional` one. The
column only starts working once the estate is genuinely mixed, which requires
adoption to be gradual.

*From the measurement:* `success` came back `declined` in 5 of 5 repos. A bulk
run over 82 projects would therefore produce ~82 cards each carrying a declined
success metric and an unreviewed persona — a backlog wearing a rigor costume, at
estate scale, exactly as open question 3 feared.

The format enforces this rather than asking for discipline: rule R3 refuses
`status: confirmed` while any field is still `drafted`, so a bulk drafter
*cannot* produce confirmed cards. Someone must read each one. That is the cost,
and it is the point.

**Ruling 15 in full — what the first confirm session revealed Autarch to be.**
Gate A ran end-to-end on one project, and every minute of it was one of two
activities: reviewing and editing a markdown document under a validator gate, or
deciding what agents should do next. Nothing else happened. The card is
markdown; the gate reads markdown; the drafter writes markdown; the thing the
gate protects is whether a human actually read it. The 2,270 plans and 388 PRDs
in the estate are the same substance at scale — the estate's problem is not that
it lacks documents but that nothing distinguishes a document someone read from
one an agent emitted.

That reframes the phase-2 TUI. A card panel is not a card panel; it is the first
instance of a **markdown review surface with a machine-checkable ratification
state**, and the six fields are one schema it happens to enforce. The second
instance is already sitting in the estate at 388×: PRD sections carrying
persona/pain/cuj/success at 14/5/2/1% compliance (finding f-010) — asked for,
mostly unanswered, never gated. Whether the TUI generalizes to those or stays
card-only is a phase-2 scoping question this ruling opens and does not close.

## Gate A — what the first confirm session found

Subject: `uncrancher`. Card drafted, mk reviewed, four format findings recorded,
**not ratified** — the session forked before ratification and the fork is the
result worth keeping.

*Findings that hold against the format:*

1. **R1 and R2 govern `drafted` and not `confirmed`.** Both live inside
   `if state == "drafted"`; the `confirmed` branch requires only a non-empty
   `value`. Proven by execution: the same field with the same journey-scope
   evidence is INVALID as `drafted` (*"a project metric needs project-scope
   evidence"*) and valid as `confirmed`. Enforcement switches off exactly when a
   human touches a field. **Ruling owed** — recommend applying R1 to `confirmed`
   too, or requiring `origin: mk` for values a human originated; hold R2 as-is.
2. **The drafter never opens the why-bearing file.** `uncrancher`'s why lives in
   `docs/ontology.md`, which is not a README, CUJ, decision log, or PRD — the
   four things the drafter's search path reads. The first card came back
   mechanics-level and mk rejected it on exactly that basis. The 5-repo
   measurement optimized the search path for *yield*, which systematically
   selects against *depth*, casting doubt on every `pain` that measurement drafted.
3. **`cuj` holds one ref; a why can span several journeys.** `uncrancher`'s spans
   three (compose, walk, porch). The field took the entry point and a prose note.
4. **No per-field provenance.** `confirmed_by`/`confirmed_at` are card-level only,
   so a value mk originated is indistinguishable from one they accepted. Worked
   around in prose, which the validator cannot read.

*The pricing error, which is mine and not the format's.* Ruling 12 already states
that R3 refuses `status: confirmed` while any field is `drafted`, so a bulk
drafter cannot produce confirmed cards. Having written that, I still planned Gate
A as *"set status, run the checker, exit 0"* — and the checker refused with
`status confirmed but still drafted: persona, pain, cuj, guardrail`. Ratification
is **not one yes**. It is one decision per field, each of which either accepts
text mk did not write or requires them to write their own. Any 82× estimate built
on a one-yes confirm session is low by roughly the field count.

*The session-shape finding (ruling 14).* Reading the why hard enough to write it
truthfully surfaced a live design hole in the subject project — a walking game
whose walk provably cannot affect its own roll. Following it was correct
engineering and produced two turns of genuine design work, now parked at
`uncrancher-2zd`. It was also the wrong session move, and nothing in the goal
said so. Hence ruling 14.

*What Gate A still needs:* mk's four field rulings on `docs/why.md`
(`persona`, `pain`, `cuj`, `guardrail` — each `confirmed` or `declined`), the
ruling owed on finding 1, and the A6 call on a second pilot subject.

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

1. ~~**What exactly is on the card?**~~ **Settled** —
   `docs/reference/product-card-format.md`. Six fields (`line`, `persona`,
   `pain`, `cuj`, `success`, `guardrail`) plus a `decisions` link list, in
   `docs/why.md` frontmatter. **The card references a cujgel CUJ by ID and never
   inlines one:** `cujgel.schema.json` requires `mental_model` and
   `success_condition` and carries an `ambiguity_ledger` — copying that in would
   create two sources of truth for one journey and make the card unglanceable.
   The `declined` state is deliberately a projection of cujgel's
   `ambiguity_ledger.open_questions`, so the vocabulary is reused rather than
   reinvented.
2. ~~**What can an agent honestly draft?**~~ **Measured, not argued** —
   `docs/analysis/2026-08-13-card-drafter-measurement.md`. Across five repos
   spanning 455 plans to zero: `persona` 4/5, `guardrail` 4/5, `pain` 3/5,
   `cuj` 3/5, `line` 3/5, **`success` 0/5**. Declining the success metric is the
   correct output in every repo measured.

   The finding that changed the design: **the citation rule alone would have
   produced a wrong success metric in 4 of 5 repos.** A cujgel `success_condition`
   is journey-scoped; a PRD's "Success Metrics (Epic Definition of Done)" is
   epic-scoped; jeddnet's item and test counts are project-scoped inventory rather
   than outcome. All three cite real text. Misattribution survives review in a way
   invention does not. Hence three rules — cite, scope-match, outcome-not-inventory
   — of which the first two are machine-checked and the third is honestly labelled
   as drafter judgment because no check can be written for it.

   A declined field does **not** block `/write-plan`; an unconfirmed card does.
   A card may be confirmed with `success` declined — that is mk saying on the
   record "I do not yet know how I would tell if this worked," which is true of
   most of the estate and worth saying out loud.
3. ~~**Backfill policy for 82 projects.**~~ **Settled — ruling 12 above.** On
   first touch, never in bulk, and the format enforces it rather than asking for
   discipline.
4. ~~**What does the row rank by when nothing is pledged?**~~ **Settled — ruling
   11.** Funded first (decision 6), then manual pins, then **weakest card first**:
   the unfunded tail is ordered by how little of its card is real, so the estate's
   own gaps rank themselves rather than a staleness formula standing in for mk's
   taste. `card-check.py --json` emits `strength.score` out of 6 for this, and
   **only `confirmed` fields count** — a drafted field is an unreviewed guess and a
   declined field is a recorded unknown, so neither can raise a project's rank.
   Without that, running the drafter across the estate would look like progress
   and quietly reorder the door: ruling 12's failure mode arriving as a scoreboard
   instead of as a backlog.

   Mk's ruling accepts a known cost, recorded here so it can be revisited against
   evidence: the worst-documented project sits permanently at the top of the
   unfunded tail, and may read as a nag rather than a prompt.
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
