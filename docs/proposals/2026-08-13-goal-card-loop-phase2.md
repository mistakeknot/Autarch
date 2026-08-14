---
artifact_type: goal-proposal
status: reviewed and amended — running
date: 2026-08-13
project: Autarch
reviewed_by: flux-melange (4 rounds, 47 findings, halt DRY)
review: docs/research/flux-melange/autarch-goal-card-loop-phase2/2026-08-13-synthesis.md
depends_on:
  - Sylveste/apps/Autarch/docs/reference/product-card-format.md
  - Sylveste/apps/Autarch/docs/reference/card-drafter.md
  - Sylveste/apps/Autarch/docs/analysis/2026-08-13-card-drafter-measurement.md
  - Sylveste/apps/Autarch/docs/brainstorms/2026-08-13-autarch-universal-interface-brainstorm.md
  - dotfiles/common/.local/bin/card-check.py
  - dotfiles/common/.claude/hooks/guard-plan-needs-card.sh
---

# Proposed goal: take one project through the full card loop, then build phase 2

> **Amended 2026-08-13 after review.** The goal below is the version that was
> reviewed; the version being run is in § Amendments. Six changes, five of them
> from findings the review upheld. Read § Amendments first — the original is
> kept because two of its clauses turned out to be false and the record of that
> is worth more than a clean document.

## The goal text as proposed (verbatim, superseded by § Amendments)

> Take one project through the full card loop, then build phase 2. Format at
> `Sylveste/apps/Autarch/docs/reference/product-card-format.md`, drafter procedure at
> `card-drafter.md`, measurement at `docs/analysis/2026-08-13-card-drafter-measurement.md`
> (all in `38715de`).
>
> Start with `unc-rancher`: it is a real GSV games-slate item, so its card is also the
> funding page's sales copy, which is the coupling this design rests on. Draft its
> `docs/why.md` per the drafter procedure, then mk confirms it — that is the step nothing
> has yet exercised, since every permit-path test so far runs against fixtures I wrote
> rather than a card mk ratified.
>
> Record what the confirm step actually costs in minutes and what it changed, because
> ruling 12 commits the estate to doing it 82 times and that number has never been
> measured.
>
> Then answer the question one real card raises and the five drafted ones could not: are
> these the right six fields? If mk finds themselves wanting to say something the format
> has no slot for, or leaving a slot empty that should not exist, that is a format finding
> and it is cheaper now than after the TUI reads it.
>
> Then build phase 2: the thin tmux-native door on `pkg/tui`, rendering **project** rows —
> not session rows, per decision 2 and ruling 13, so jeddnet's claude and codex sessions
> are one row with an agent count. Order by ruling 11: funded, then pins, then weakest
> card first, reading `strength.score` from `card-check.py --json`. Coverage is stated as
> a fraction with every unresolvable session named, never dropped — intermux reports 1 of
> 21 and discloses no gap, which is the failure this row is replacing.
>
> **OUT:** reviving Bigend, Coldwine or Pollard; the funding mechanism itself; backfilling
> cards beyond the one confirmed; the sibling's `test-config-invariants` regression at
> `0b25045`; Autarch's local `main` divergence.
>
> **DONE WHEN:** `unc-rancher` has a confirmed `docs/why.md` that `card-check.py` exits 0
> on; the plan gate has been observed permitting a real plan into that repo and refusing
> one into a repo without a card, both from a live session rather than a fixture; the
> confirm-step cost and any format findings are written into the brainstorm as rulings with
> reasons; the TUI renders all 21 sessions as project rows with a stated coverage fraction;
> ranking matches ruling 11 against real card strengths; a project whose card is absent,
> provisional, invalid and confirmed are four visibly distinct row states; suites green and
> mutations dead on both machines.

## What already shipped (the ground this goal stands on)

Landed and verified in Autarch `38715de` and dotfiles `8e35b11`:

- **The card format** — `docs/why.md` with YAML frontmatter; six slots (`line`, `persona`,
  `pain`, `cuj`, `success`, `guardrail`); every field in state `drafted | declined |
  confirmed`; `status: confirmed` requires no field left `drafted`.
- **Three drafting rules.** R1 citation (machine-checked): a `drafted` field cites an
  in-repo path or is declined. R2 scope (machine-checked): `success`/`guardrail` drafted
  must cite at least one `scope: project` entry. R3 outcome (soft, drafter judgment only,
  explicitly labelled unenforceable): a metric names a direction or threshold, not a count.
- **The validator** `card-check.py` — four verdicts on four exit codes: 0 confirmed,
  1 provisional, 2 invalid, 3 absent. Emits `strength.score` (confirmed fields only, six
  slots) so a ranker never infers absent-vs-invalid from a score.
- **The gate** `guard-plan-needs-card.sh` — PreToolUse on `Edit|Write|MultiEdit`, matching
  only `docs/plans/*.md`, rooted from the written path rather than `$PWD`. Denies on
  provisional / invalid / absent / unknown-exit-code. Overrides logged with a reason;
  empty reason refused. Live on both machines.
- **49 assertions green on both machines; 30 mutations: 29 killed, 0 survived, 1 proven
  equivalent.**

## The estate this is aimed at

82 projects, 2,270 plans, 831 brainstorms, 388 PRDs, 126 CUJs, 36 decisions, 3 personas,
**0 metrics**. Diagnosed as a request-shape failure, not a discipline failure: agents were
never asked for a why, so they never wrote one.

## Findings from the measurement that constrain this goal

Five repos spanning the material gradient (shadow-work 427 plans → jawnomicon 0):

| Repo | line | persona | pain | cuj | success | guardrail |
|---|:--:|:--:|:--:|:--:|:--:|:--:|
| shadow-work | D | D | D | D | — | D |
| jeddnet | — | — | D | — | — | D |
| uncrancher | D | D | D | D | — | — |
| ravenous | D | D | D | D | — | D |
| jawnomicon | — | D | — | — | — | D |
| **drafted** | 3/5 | 4/5 | 3/5 | 3/5 | **0/5** | 4/5 |
| **guessed** | 0 | 0 | 0 | 0 | **0** | 0 |

- **`success` drafted 0/5.** No repo carries a baseline. This is the field the whole
  premise depends on, and it is the one an agent cannot produce.
- **Misattribution, not invention, is the danger.** The citation rule alone would have
  produced a *wrong* success metric in 4 of 5 repos — each offers one that cites real text
  and is only wrongly scoped. Misattribution survives review; invention does not. R2 exists
  because of this.
- **Artifact count does not predict yield.** jawnomicon at 0 plans drafted 2 fields;
  jeddnet at 29 drafted 2. shadow-work at 427 drafted 5. The gradient is not monotonic.

## Rulings this goal is bound by

- **Decision 2 / ruling 13** — a project with N agents is ONE row. The prototype rendering
  jeddnet twice contradicted a settled ruling rather than raising a new question.
- **Ruling 11 (mk's own pick)** — funded first, then pins, then weakest card first. Only
  `confirmed` fields count toward strength. Known cost, accepted: the worst-documented
  project sits permanently on top, "which may just be a nag."
- **Ruling 12** — backfill on first touch, never bulk. Two independent arguments: the
  prototype showed a signal firing on 100% of rows carries no information; the measurement
  showed bulk drafting manufactures uniform `provisional`.

## What this review was for

The goal had not been started. Nothing in it was committed. The review was told to treat
every clause as revisable, including the ordering (confirm-then-TUI), the choice of
`unc-rancher` as the first subject, the DONE WHEN list, and the OUT list.

---

# Amendments after review

Four rounds, 47 findings, 33 surfaced, halted DRY. Six changes. Two were done before this
document was amended, because they were defects rather than wording.

## Done already

**A1 — `strength()` no longer credits `line` on an unratified card.**
`card-check.py` counted `line` toward the confirmed total whenever it was set, four lines
below a docstring saying only `confirmed` counts. Consequence nobody had stated: ruling 11
sorts the unfunded tail *weakest first*, an absent card scores 0, and a drafter-only card
scored 1 — so **running the drafter moved a project down the queue, below every project
nobody had touched.** The ranking function produced the exact inversion ruling 11 exists to
prevent. Fixed; `line` now counts only when `status == confirmed`.

The suite did not miss this — it *ratified* it. `test_drafted_fields_do_not_raise_strength`
asserted `score == 2` on a provisional card with the comment "line + cuj = 2 confirmed".
The expectation was written from the implementation instead of from the rule, and 29 killed
mutants agreed with it, because no mutation targeted that line. A mutation score is bounded
by the mutations you thought to write. Both are now fixed, plus a mutation that reintroduces
the bug.

**A2 — local Autarch `main` merged with `origin/main`.**
Local HEAD was 2 ahead / 8 behind and contained none of this goal's `depends_on`: no
`product-card-format.md`, no `card-drafter.md`, no measurement, and decisions 11–13 absent
from the local brainstorm. DONE WHEN said "write format findings into the brainstorm as
rulings" — which would have authored a merge conflict against the very commit the OUT list
waved off. Both conflicts resolved to `origin/main`; the 17 lines it drops are open
questions 1–4, which it re-adds struck through with their closing reasons.

## Changes to the goal text

**A3 — the sequencing is now a gate, not a paragraph.** The prose argued
format-finding-before-TUI; DONE WHEN was seven semicolon-joined clauses with no ordering, so
an agent could satisfy all seven having built the TUI first. Split into Gate A and Gate B.

**A4 — one DONE WHEN clause that can come back false.** Every original clause was a
mechanical artifact check — *renders, exits, matches, observed permitting*. None could
return false if the apparatus did nothing for the overwhelm it exists to address. The goal
posed the ceremony-vs-instrument question in its own text and then equipped itself with
seven criteria that could not answer it either way.

**A5 — the format finding is priced.** `card-check.py` hard-pins `card_version` to 1 with no
migration path, so a version bump flips the just-confirmed card from exit 0 to exit 2. The
goal solicits a format finding and would have been destroyed by getting one.

**A6 — open call for mk (see below).** Second pilot subject.

## The amended goal

> Take one project through the full card loop, then build phase 2. Format at
> `docs/reference/product-card-format.md`, drafter at `card-drafter.md`, measurement at
> `docs/analysis/2026-08-13-card-drafter-measurement.md` — all now present in the local tree
> after the `origin/main` merge.
>
> **Gate A — the card. Must close before Gate B opens.**
> Draft `unc-rancher`'s `docs/why.md` per the drafter procedure, dry-running `card-check.py`
> first: a fresh card omitting `success`/`guardrail` returns INVALID (exit 2), not
> PROVISIONAL, so the drafter must write declined stubs and that is worth catching before
> mk is in the room rather than during the confirm step live. Then mk confirms it — the step
> nothing has yet exercised, since every permit-path test so far runs against fixtures I
> wrote rather than a card mk ratified.
>
> Record the confirm-step cost as session time including any interruption, since the single
> `confirmed_at` timestamp cannot reconstruct fragmented effort after the fact. Record
> `strength.score`'s confirmed/declined breakdown and whether the `success`/`guardrail`
> decline reasons carry real content — exit 0 alone cannot distinguish a card whose six
> slots were genuinely sourced from one that is a third hollow by construction, because the
> declined branch requires only two non-empty strings.
>
> Answer the format question one real card raises: are these the right six fields? Any
> finding lands in the brainstorm as a ruling with a reason. **If a finding implies a
> `card_version` bump, state in the same ruling what happens to the card just confirmed** —
> the validator hard-pins version 1 and would flip it from CONFIRMED to INVALID on its next
> check, and re-ratification is not currently budgeted.
>
> **Gate A closes with mk's explicit sign-off.** Gate B does not open before it.
>
> **Gate B — the door.** The thin tmux-native surface on `pkg/tui`, rendering **project**
> rows per decision 2 and ruling 13, so jeddnet's claude and codex sessions are one row with
> an agent count. Inspect what `pkg/tui` already contains first — it is not a bare package.
> Order by ruling 11: funded, then pins, then weakest card first, reading `strength.score`
> from `card-check.py --json`. State coverage as a fraction with every unresolvable session
> named — and state it on **both** axes, sessions resolved and cards confirmed out of the
> estate, because the row exists to replace intermux's silent 1-of-21 and must not reproduce
> that silence on the axis the goal is actually about.
>
> **The falsifiable one.** A week after the card is confirmed, mk names one thing they did
> differently because of it — or records that they did not. A "no" lands in the brainstorm
> as a ruling with the same weight as a yes. This is the only clause here that can fail on
> the premise, and it is what separates this goal from the 2,270 plans it was written to
> explain.
>
> **OUT:** reviving Bigend, Coldwine or Pollard; the funding mechanism; backfilling cards
> beyond the pilot; the sibling's `test-config-invariants` regression at `0b25045`;
> tightening R3's hollow-card permit (deferred deliberately — it changes what a confirm
> costs, and measuring the current cost is the point).
>
> **DONE WHEN:** Gate A — `unc-rancher` has a confirmed `docs/why.md` that `card-check.py`
> exits 0 on; the confirm-step cost, the strength breakdown, and any format finding are
> written into the brainstorm as rulings with reasons; mk has signed off. Gate B — the plan
> gate has been observed permitting a real plan into that repo and refusing one into a repo
> without a card, both from a live session rather than a fixture; the TUI renders project
> rows with coverage stated on both axes; ranking matches ruling 11 against real card
> strengths; absent, provisional, invalid and confirmed are four visibly distinct row
> states; suites green and mutations dead on both machines. Then — the week-later question,
> answered either way.

## Open call for mk

**A6 — a second pilot subject?** The review's live disagreement. Three lenses argued
`unc-rancher` is the wrong first subject; all three were refuted on a shared factual error
(it drafts 4/6, not the best of the five — shadow-work and ravenous draft 5/6). But each
carried a separable argument the refutation never touched: a pilot run on the one subject
whose maker independently wants the card measures the toll where nobody is evading it.

Both things are true at once. The funding-page coupling is the strongest reason to pick
`unc-rancher` — it is the only subject with an external reader who will notice if the card
lies — and the strongest reason to distrust its cost number.

Adding `jawnomicon` (2/6 drafted, 0 plans) brackets the estate instead of anchoring one end
of it, and stresses the declined branch the review calls the load path. It costs mk a second
confirm session. Not taken unilaterally: it spends mk's attention, and ruling 12's price is
what the goal exists to measure.

## Findings recorded, not acted on

- **f-010** (the premise): the estate's 388 PRDs already carry persona/pain/cuj/success-shaped
  fields at 14% / 5% / 2% / 1% compliance. Agents *were* asked before and mostly didn't
  answer — a different failure from "never asked". Bounded, not refuted: the card has a
  PreToolUse gate the PRD sections never had. Worth a ruling with a reason before phase 3.
- **f-024**: `guard-plan-needs-card.sh` never reads `strength.score` — it branches on exit
  code alone, so a richly-sourced card and an all-declined one both permit silently.
- **f-043**: `cmd/mycroft` (T2/T3 auto-dispatch, shipped in this repo) appears nowhere in the
  brainstorm. Autonomous multi-repo dispatch can trigger several first-touch confirms at once.
- **f-013 / f-038**: the validator never reads the prose body — the part the funding page
  renders as sales copy. 30 tests, none touching prose.
- **`card-drafter.md` was never read by any lens**, including the synthesizer. It is a
  `depends_on` and is entirely unreviewed.
