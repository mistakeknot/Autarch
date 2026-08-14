---
artifact_type: melange-synthesis
method: flux-melange
target: docs
target_description: >-
  Autarch docs tree — the why-card format spec (product-card-format.md), the drafter
  procedure (card-drafter.md), the 5-repo drafting measurement, the brainstorm carrying
  13 rulings, and the proposed goal to confirm one card then build the phase-2 TUI.
goal: >-
  Review docs/proposals/2026-08-13-goal-card-loop-phase2.md before any of it runs. Is the
  sequencing right — confirm one card, then TUI? Is unc-rancher the right first subject?
  Does the six-field format survive a real human confirming it, given success drafted 0/5?
  Are the DONE WHEN clauses checkable, and does any encode a false assumption? What would be
  expensive to discover only after the TUI reads the format? Attack the premise too: is a
  per-project why-card the right instrument for overwhelm across 82 projects, or ceremony
  added to an estate that already has 2270 plans nobody reads?
weights: balanced
rounds_run: 4
halt_reason: DRY
total_fusions: 1
emergent_findings: 4
runtime: claude
date: 2026-08-13
---

# The eye of distance: card loop phase 2

Four rounds, 47 findings, 33 clusters, one fused lens, halted DRY at round 3 (yield 0).
This is the synthesis, re-scored from the raw ledger.

## What I changed on re-score

The per-round scores were triage estimates made by agents that, in several cases, could not
read the documents they were scoring against. I re-scored the merged ledger and made four
material changes:

**Two findings move from refuted to upheld, because their refutation was an artifact of a
tooling gap, not a defect in the claim.** `Sylveste/apps/Autarch` is its own git repo; its
local HEAD is `7c02ffd`, which is **2 ahead and 8 behind `origin/main`**. Commit `38715de`
— the one that lands `product-card-format.md`, `card-drafter.md`, and the measurement — is
not an ancestor of local HEAD, so `find` returns nothing and every lens that tried to verify
a quote against those files came back empty-handed. The objects are present; only the
worktree is behind. `git show 38715de:docs/reference/product-card-format.md` reads fine.

- **f-013** (validator never inspects the prose body, which the format doc says is the
  funding page's sales copy) was refuted for lack of a readable source. Lines 26–27 of that
  file, verbatim: *"YAML frontmatter carries the machine-readable card. The prose body below
  it is free-form and is what the funding page renders as sales copy."* **Upheld.**
- **f-002** (the format doc's state taxonomy is 3-valued while `card-check.py` is 4-valued)
  was refuted on the same basis. `grep -i invalid` over the full 202-line document returns
  **zero hits**; the closing section *"Absent, provisional, confirmed"* enumerates exactly
  three states and says the gate "refuses plan-writing on absent and provisional, permits on
  confirmed" — `invalid` is not in the doc's vocabulary at all, yet DONE WHEN requires the
  TUI render it as one of four distinct row states. **Upheld.**

**Three findings are now execution-proven rather than argued.** I ran `card-check.py`
against synthetic cards:

- **f-014 / f-035** (a card with all five fields `declined`, `line: null`,
  `status: confirmed` exits 0): confirmed. Output —
  `{"verdict": "confirmed", "code": 0, ..., "strength": {"score": 0, "of": 6, "confirmed": 0,
  "drafted": 0, "declined": 5}}`, exit 0. The plan gate's only silent permit, on a card with
  zero content and a self-attested `confirmed_by: "definitely-mk"`.
- **f-001** (a fully-provisional, drafter-only card scores `strength.score: 1`): confirmed.
  A card with `status: provisional`, persona and pain merely `drafted`, and `line` set
  returns `"strength": {"score": 1, ..., "confirmed": 1, "drafted": 2, "declined": 3}` at
  exit 1. **Nothing on that card is confirmed.**
- **f-032 / f-035** (unc-rancher's fresh card validates INVALID, not PROVISIONAL): confirmed.
  A card omitting the `success` and `guardrail` keys — the exact shape the measurement
  predicts for uncrancher — returns `{"verdict": "invalid", "code": 2, "reason": "fields
  missing entirely: success, guardrail"}`, exit 2. Under `guard-plan-needs-card.sh` that is a
  hard BLOCKED.

**One finding is corrected downward.** f-033 claimed the four-row-states DONE WHEN clause
cannot be satisfied because only CONFIRMED and ABSENT will be populated at completion. A
sixth card already exists on disk that no lens found:
`/Users/sma/projects/After-Them/docs/why.md`, untracked, `card-check.py` exit 1, three
fields drafted with real project-scope citations. PROVISIONAL is reachable with real data.
INVALID still has no honest path at completion. The clause is 3/4 satisfiable, not 2/4 —
risk drops from 6 to 4, novelty holds.

**One convergent cluster collapsed on a false shared premise, and nobody re-asked the
question.** See §5.

---

## 1. Novelty × Risk Frontier

The front is degenerate at the corner: with `risk.product` capped at 9 (blast 3 × likelihood
3), three findings sit at (novelty 3, risk 9) and dominate everything else. The informative
structure is on the shoulders, so those lead, per the brief.

### Shoulder A — max novelty, mid risk: the diagnosis may be wrong

**f-010** · novelty **3** · risk **6** (blast 3 × likelihood 2) · heat 18 · taste −1 ·
severity P1 (reference only)
Lens: `fd-infoarch-ceremony-risk`

> `docs/why.md` is a new, **eighth** required artifact type in an estate whose 388 PRDs
> already carry persona/pain/cuj/success/guardrail-shaped fields at 14% / 5% / 2% / 1% /
> 0.3% compliance. Agents *were* asked for a why before and mostly didn't answer — a
> different failure mode from the brainstorm's "agents were never asked for a why, so they
> never wrote one." The proposal never compares the new-file approach against raising or
> surfacing existing PRD/CUJ compliance.

This is the sharpest answer available to the premise attack, and it is the only finding that
challenges the goal's *diagnosis* rather than its execution. The proposal states the
diagnosis at line 83 as settled — "a request-shape failure, not a discipline failure" — and
the compliance numbers it cites in the same breath are evidence against that framing, not
for it. Blast 3 because if the diagnosis is wrong the entire instrument is wrong. Likelihood
2, not 3, because the card carries a mechanism the PRD sections never had: a PreToolUse gate
that blocks plan-writing. That mechanism difference is real and is `fd-infoarch-ceremony-
risk`'s own declared failure mode ("may generalize too readily from 'this estate has unread
artifacts' to 'this artifact will also go unread'"). The finding is not refuted by that — it
is bounded by it.

The cheap version of this test costs nothing and is not in scope: before drafting
unc-rancher's card, read whether unc-rancher's *existing* PRDs and CUJs already answer four
of the six fields. The measurement says they do — persona, pain, cuj and line all drafted
**from in-repo citations**. The drafter is not producing new knowledge; it is relocating
knowledge that was already on disk into a file the gate can read. Whether that relocation is
the intervention, or whether a reader pointed at the existing files would do, is the
question f-010 says the goal never asks.

### Shoulder B — mid novelty, max risk: the sequencing is not enforced

**f-031** · novelty **2** · risk **9** (blast 3 × likelihood 3) · heat 18 · taste −1 ·
severity P0 (reference only)
Lens: `fd-timberframe-load-path` (converges with `fd-datamodel-format-durability` f-029)

> DONE WHEN is a flat, unordered, semicolon-separated checklist that never makes the
> confirm-step format-finding ruling a precondition for starting phase 2 (`pkg/tui`). An
> executing agent can satisfy every clause while building the TUI before or concurrently
> with recording the format finding — contradicting the goal's own stated rationale that the
> finding must land "cheaper now than after the TUI reads it."

This is the direct answer to north-star question 1. **The sequencing the proposal argues for
in prose is not encoded anywhere a checker can see.** Proposal lines 34–37 make the
argument; line 39 says "Then build phase 2"; lines 50–57 flatten all seven criteria into one
list joined by semicolons with no ordering language. `fd-datamodel-format-durability` reached
the same place from the schema side (f-029) and added the mechanism that makes it expensive:
`card-check.py:118-121` pins `card_version` to exactly 1 with a hard `CardError` and no
migration path anywhere in either reference doc. Two readers, one hard-pinned version, no
grandfather clause.

Blast 3: this is the scribe-rule/square-rule decision. Once `pkg/tui` reads the card shape,
the format is fitted to its mate and re-cutting costs a scarf repair in two consumers.
Likelihood 3: nothing in the text prevents it, and the goal is written as one continuous
stretch of work with a single DONE WHEN gate at the end.

The fix is one word of sequencing in the DONE WHEN list. It is the cheapest high-blast fix
in this report.

### The ridge — (novelty 3, risk 9), all three co-maximal

**f-009** · heat **27** · taste −2 · `fd-infoarch-ceremony-risk`

> Every DONE WHEN clause is a mechanical/artifact check — validator exit codes, gate
> behavior, TUI row rendering, suite and mutation status. **None can come back false if the
> card-and-gate apparatus does nothing for the overwhelm it exists to address.** The goal can
> complete 100% while the premise question it poses in its own text remains entirely
> untested.

Risk decomposition: blast 3 (the loss is the whole goal's evidentiary value), likelihood 3
(this is a property of the text as written, not a runtime hazard — it is already true).
Searching the DONE WHEN list for *overwhelm*, *revisit*, *remember*, *use*, *actually*
returns nothing; every verb is *renders*, *exits*, *matches*, *observed permitting*. The
proposal's own closing section says "The review should treat every clause as revisable" and
poses the ceremony-vs-instrument question as live — and then hands that question a
completion criterion set that cannot answer it either way.

**f-001** · heat **27** · taste −2 · `fd-datamodel-format-durability` (× `fd-hci-ratification-cost`)

> `strength()` adds an unconditional +1 to the confirmed count whenever the top-level `line`
> string is set, regardless of whether the persona/pain fields backing it are drafted or
> confirmed.

Execution-proven above. The consequence nobody stated: ruling 11 orders the unfunded tail
**weakest card first**. An absent card scores 0. A drafter-only, entirely unratified card
scores 1. **Running the drafter on a project moves it *down* the priority queue, below
projects nobody has touched.** That is precisely the inversion ruling 11 was written to
prevent, and the docstring on the function that causes it says so:

> *"Only `confirmed` counts. A `drafted` field is an agent's unreviewed guess and a
> `declined` field is a recorded unknown — neither is a project that knows what it is for,
> and letting either count would make a bulk drafter look like progress."*

Four lines later: `if card.get("line"): counts["confirmed"] += 1`. Blast 3 (corrupts the one
ranking mechanism ruling 11 specifies, across all 82 rows, and the TUI is about to harden on
it). Likelihood 3 (fires automatically; `line` drafted 3/5 in the measurement).

**f-035** · heat **27** · taste −1 · fusion of `fd-hci-ratification-cost` ×
`fd-assay-mark-integrity`

> `success` and `guardrail` will be **declined**, not drafted-then-confirmed, on
> unc-rancher's very first card as a structural necessity, not an available exploit someone
> might choose. DONE WHEN #1 will pass on a card that is one-third hollow by construction.

Execution-proven above: omitting the keys yields INVALID (exit 2), so the drafter *must*
write declined stubs, and the declined branch (`card-check.py:172-179`) requires only two
non-empty strings — no evidence list, no scope, unlike R1's citation requirement and R2's
project-scope requirement for drafted fields. Blast 3, likelihood 3. This is the answer to
north-star question 3: **the six-field format survives a real human confirming it precisely
because two of the six fields cost nothing to dispose of**, and DONE WHEN #1 ("a confirmed
`docs/why.md` that `card-check.py` exits 0 on") cannot tell that outcome from a card where
all six slots were genuinely sourced.

### Novelty without risk

Two findings carry high novelty at low risk and are worth reading for what they notice
rather than what they threaten.

**f-043** · novelty 3 · risk 3 (blast 3 × likelihood 1) · `fd-subak-flow-allocation` —
Mycroft, a T2/T3 auto-dispatch fleet orchestrator **shipped in this same repo**
(`cmd/mycroft`, documented in Autarch's own `CLAUDE.md`), appears nowhere in the proposal or
the brainstorm. `grep -i mycroft` over both returns zero. Autonomous multi-project dispatch
can trigger several "first touch" plan-writes across different repos in one session, and
ruling 12's on-demand trigger plus the global per-write Clavain hook have no mechanism to
queue, batch, or coalesce simultaneous confirm demands — reproducing bulk-shaped pressure
through a design that explicitly rejected bulk. Likelihood 1 only because T2/T3 must actually
be running during the pilot.

**f-033** · novelty 3 · risk 4 (corrected) · `fd-timberframe-load-path` — the four-row-states
clause is satisfiable only partially with real data. Corrected above: 3 of 4 states are
reachable (After-Them supplies a real PROVISIONAL), INVALID has no honest path at completion
without deliberately corrupting a card, which is itself out of scope.

---

## 2. Top Fusions

One pair was fused: `fd-hci-ratification-cost` × `fd-assay-mark-integrity`, generated as
`fd-fused-ratification-toll-integrity`. Its stated weld: *"hallmarking presumes an
independent assay office whose warden is indifferent to what the test cost the maker; this
estate collapses maker, office and warden into mk, so nothing but the price mk chose to pay
separates a real mark from a hollow one."* Seven fusion-sourced findings resulted (three
from the generated lens, four from the round-1 and round-2 fusion probes). Four are genuinely
emergent; one is a null.

### f-040 · heat 18 · taste −2 · the two DONE WHEN clauses are not jointly stable

Parents: `fd-hci-ratification-cost` × `fd-assay-mark-integrity`.

> The goal explicitly solicits a "format finding" from the confirm session and frames getting
> one as cheap validation. But `card-check.py` hard-pins `card_version` to exactly 1 with no
> per-card version stamp — so a resulting version bump flips unc-rancher's just-confirmed
> card from CONFIRMED (exit 0) to INVALID (exit 2) on its very next check, with zero cost
> budgeted for re-ratification and no way to distinguish a deliberately-superseded mark from
> rot.

*intersection_justification*: "fd-hci-ratification-cost alone would only note 're-confirming
isn't priced'; fd-assay-mark-integrity alone would only note 'no version stamp records what a
mark was tested against.' The fused claim is the causal chain neither reaches solo: this
goal's own DONE WHEN success on the format-finding clause can retroactively void its own
DONE WHEN success on the confirmed-card clause."

Evidence: `card-check.py:118-121` raises on `card_version != 1` with no version-aware
grandfather path; `guard-plan-needs-card.sh:76-82` branches purely on exit code, turning exit
2 into a hard BLOCKED with no distinction for "was confirmed, now stale by design." I
verified both. This is the single best argument for f-031's sequencing fix: the goal is
built to produce an event that destroys its own first deliverable.

### f-024 · heat 18 · taste −1 · the gate never reads the number it computes

Parents: `fd-hci-ratification-cost` × `fd-assay-mark-integrity` (round-1 fusion probe).

> `guard-plan-needs-card.sh` — the one enforcement point that exists today, and the one this
> goal's own DONE WHEN tests — branches purely on `card-check.py`'s exit code and never reads
> or thresholds on `strength.score`. A card confirmed with real content and a card confirmed
> by declining all five fields both produce exit 0, both silently permit a plan write, and
> neither prints an advisory.

Evidence, verified: `grep -n 'json\|strength' guard-plan-needs-card.sh` returns **nothing**.
The script never invokes the validator with `--json`, even though `emit()` computes
`strength` for exactly this purpose and annotates it with "a ranker reading only `score`
cannot tell no-card from card-is-lying." The gate is the ranker's blind twin. This is the
finding that promotes f-014 from "future TUI display defect" to "live enforcement defect."

### f-041 · heat 18 · taste −2 · the gate narrates a fact its schema cannot support

Parents: `fd-hci-ratification-cost` × `fd-assay-mark-integrity`.

> A fatigued, interrupted, partially-ratified confirm session collapses into the same
> `status: provisional` as a card no one has ever opened, and the gate's exit-1 denial text
> asserts an unverifiable "drafted by an agent" narrative over that ambiguous state — while
> the single `confirmed_at` timestamp gives the goal's own minutes-measurement no way to
> reconstruct fragmented real-world effort after the fact.

Evidence, verified: `guard-plan-needs-card.sh:77` hardcodes `"(drafted by an agent, not yet
confirmed by mk)"` into every exit-1 message, and nothing in `card-check.py`'s schema records
who drafted anything. `status` is a two-value enum; there is no per-field or per-session
timestamp besides the single `confirmed_at` set only on full confirmation.

*intersection_justification*: the cost-side parent explains why the ambiguous state is
*common* (ruling 12's "on first touch" confirms will be interrupted, not clean sittings); the
integrity-side parent explains what a downstream reader is wrongly licensed to believe. Note
the second-order damage: this directly corrupts the goal's headline deliverable, the
confirm-step cost in minutes, because a fragmented session leaves no trace to reconstruct.

### f-023 · heat 18 · the two lenses' errors are not opposite

Parents: `fd-hci-ratification-cost` × `fd-assay-mark-integrity` (round-1 PROBE-DISAGREEMENT).

> f-007 (R3 is too strict, all-or-nothing, in tension with ruling 12's gradualism) is
> refuted; f-014 (R3+R4 are too permissive) holds. They are not opposite errors on the same
> rule — f-007 misreads a *project-level* rollout ruling as a *field-level* one, and its own
> source lens's axiom about all-or-nothing gates causing rubber-stamping actually predicts
> f-014's exploit rather than supporting f-007's fix.

The emergent content is the second half: `fd-hci-ratification-cost` axiom 3 ("all-or-nothing
approval gates are prone to end-of-session fatigue and rubber-stamping") is the *mechanism*
by which `fd-assay-mark-integrity`'s exploit gets taken. The remedy f-007 implied — loosening
R3 — would not close f-014's hole and would further erode what `confirmed` means. A single
lens holding either axiom lands on the wrong prescription.

### f-039 · heat 12 · the cost clause prices only the path nobody will take

Parents: `fd-hci-ratification-cost` × `fd-assay-mark-integrity`.

> DONE WHEN's cost-recording clause prices only the expensive proper-path branch (draft + mk
> confirm) and never reads `guard-plan-needs-card.sh`'s override log as comparison-cost
> evidence, even though the override path costs one logged sentence and is the branch most
> likely to govern real load across the other 81 repos.

The fused lens is explicit about why neither parent gets here alone: "the cheap path's
existence changes what the expensive path's measured minutes are entitled to certify about
the other 81 repos." Verified: `grep AUTARCH_CARD_OVERRIDE` over the proposal returns zero
hits, and the override branch permits on any non-empty string.

### Negative result — f-034: the pair is mined out

**f-034 · novelty 0 · heat 0.** The round-2 re-probe of the same disagreement re-derived
f-023/f-024's conclusion from the code and reports it matches "verbatim in conclusion." This
is the null: **`fd-hci-ratification-cost` × `fd-assay-mark-integrity`, second pass —
exhausted.** It is the strongest single piece of evidence for the DRY halt, and it is
correctly scored at novelty 0 rather than counted as yield.

### Fusions never attempted

Only one pair was fused across four rounds. `fd-datamodel-format-durability` ×
`fd-infoarch-ceremony-risk` (schema durability × does-the-artifact-get-read),
`fd-timberframe-load-path` × `fd-subak-flow-allocation` (irreversible assembly × delivery vs
ordering), and `fd-archival-backlog-appraisal` × `fd-assay-mark-integrity` (coverage of a
backlog × integrity of a single mark) were never intersected. These are not negative results;
they are unexplored. The second and third in particular look live: subak's "an ordering is
not an allocation" against timberframe's "raising day is a single irreversible event" is
exactly the shape of the phase-2 TUI question.

---

## 3. Taste Calls

The ledger carried `taste: 0` on all 47 findings — no agent exercised the axis. These are my
assignments on re-score.

### Preserve

**f-047 · taste +2 · `honest-decline`** — the format's willingness to let a card reach
`confirmed` with `success` and `guardrail` declined is the best thing in this design. From
`product-card-format.md` § Card status:

> *"A card may be confirmed with success and guardrail declined. That is mk saying, on the
> record, 'I do not yet know how I would tell if this worked' — which is true of most of the
> estate and is worth saying out loud."*

Every mechanical finding in §1 argues for tightening the declined branch. **Do not tighten it
into unusability.** The finding f-047 makes — that the real unblocking action for
unc-rancher's missing fields is two short honest stubs, not "originating a metric from
nothing" — is only true because declined is cheap. Raise its evidentiary floor (a scope, a
date, a `blocked_by`), do not raise its price.

**f-002 · taste +2 · `separation-of-verdict-from-score`** — the elegance this finding is
defending, not a smell. `card-check.py`'s comment: *"Absent and invalid cards score 0 of 6 —
but the verdict, not the score, is what says which. A ranker reading only `score` cannot tell
'no card' from 'card is lying'; it must not have to, so both fields are always present."*
That is a correctly-drawn line. The defect is that the normative format doc never learned
about it — three states in the prose, four in the code, four in the DONE WHEN.

**f-025 / f-046 · taste +1 · `corrected-cost`** — the adjudication's own axiom, *"a refuted
finding still carries information: the corrected, cheaper reading of the mechanism is itself
a finding worth recording,"* paid off here. The corrected reading (declined satisfies R3
permanently; `needs` is unenforced free text; `validate()` re-reads disk state fresh each
call so there are no session semantics and nothing forces one sitting) is more useful than
the refutation.

### Fix

**f-001 · taste −2 · `contradicts-own-contract`** — a function whose docstring forbids
exactly what its last four lines do. The worst kind of defect: the intent is documented, in
place, correct, and violated in the same scroll.

**f-009 · taste −2 · `unfalsifiable-gate`** — a DONE WHEN list that cannot come back false on
the question its own goal text poses.

**f-040 · taste −2 · `self-voiding-criteria`** — two completion criteria that are individually
checkable and jointly unstable over time.

**f-041 · taste −2 · `asserted-provenance`** — a user-facing message that narrates authorship
the data model does not record. Cheap to fix (soften the string), and worth fixing because
the number this goal exists to produce depends on the state that message describes.

**f-018 · taste −1 · `out-list-collides-with-done-when`** — the OUT list declares "Autarch's
local `main` divergence" out of scope, while DONE WHEN requires format findings be "written
into the brainstorm as rulings with reasons." The local brainstorm has decisions 1–10 and no
rulings 11, 12, or 13; those live only on `origin/main`. An executing agent writing ruling 14
into the local file is authoring a merge conflict against the very commit whose absence the
OUT list waves off.

**f-015 · taste −1 · `lossy-aggregate`** — `strength.score` averages six non-fungible slots
unweighted. A card honestly confirmed everywhere except the permanently-undraftable `success`
scores 5/6, indistinguishable from a card at 5/6 because an easy field hasn't been touched.
The one field the measurement calls "the field the whole premise depends on" is worth
exactly as much as `line`, which is derived from two others.

**f-030 / f-042 · taste −1 · `vacuously-satisfiable`** — "ranking matches ruling 11 against
real card strengths" is satisfiable by placing unc-rancher (funded tier, where score plays no
role) above 81 rows tied at zero. The comparative logic between two differently-scored
non-zero cards is never exercised.

**f-028 · taste −1 · `wrong-denominator`** — the proposal names silently-dropped coverage as
the failure it is replacing, then applies the discipline to session enumeration (21) and
never to the axis the goal is about (cards confirmed, out of 82).

**f-032 · taste −1 · `indistinguishable-failure-modes`** — `missing = [f for f in FIELDS if f
not in fields]` runs before any per-field state logic, so the validator cannot distinguish
"field never attempted" from "field the drafter forgot to stub." Both are INVALID.

**f-038 · taste −1 · `no-tripwire`** — 30 registered tests, all named after frontmatter
rules, none touching prose. A regression that silently truncates every card's prose in the
estate passes the whole suite and surfaces as a blank sales-copy panel in front of a stranger
with money.

---

## 4. Convergence Spine

High-confidence, low-novelty. These are commodity: multiple independent lenses reached them,
you can trust them, and none of them is the interesting part of this report.

| Cluster | Findings | Lenses | What you can rely on |
|---|---|---|---|
| Declined clears the gate | f-014, f-023, f-034 | assay, hci, fusion ×2 | `declined` is invisible to R3's drafted-only check. Five declined fields + `line: null` + `status: confirmed` exits 0. Execution-proven. |
| Override is free and unmeasured | f-008, f-039, f-045 | hci, subak, fusion | `AUTARCH_CARD_OVERRIDE="wip"` always permits. Nothing in scope tracks override rate against the published confirm cost. |
| Coverage discipline on the wrong axis | f-012, f-019, f-028 | infoarch, archival, datamodel | The stated-fraction rule binds the 21 sessions, never the 82 projects or the N confirmed cards. |
| Validator never reads the sales copy | f-013, f-036 | assay, datamodel | `split_frontmatter` returns `lines[1:i]`; `main()` passes `text` in exactly once and never touches it again. Confirmed exit 0 with an empty body is in the test suite deliberately. |
| No reappraisal cadence | f-022, f-044 | archival, subak | `confirmed_by` and `confirmed_at` are truthiness checks. No date comparison exists anywhere in the file. A card confirmed today carries identical weight in six months. |
| Format finding not gated before the TUI | f-029, f-031 | datamodel, timberframe | Flat DONE WHEN list, no ordering language. Also §1 Shoulder B. |
| Ranking clause trivially satisfiable | f-030, f-042 | datamodel, subak | 81 rows tied at strength 0 by construction, under this goal's own OUT list. |
| Declined is cheap and permanent | f-025, f-046, f-047 | datamodel, adjudication | No session semantics, no cross-field ordering enforcement, `needs` never parsed. The chained-origination fear is unfounded. |

**One caution about this spine.** The ninth convergent cluster —
`c-uncrancher-best-case-generalization`, with f-005 (`fd-hci-ratification-cost`), f-011
(`fd-infoarch-ceremony-risk`) and f-017 (`fd-assay-mark-integrity`) — had **three
independent lenses converge and all three are refuted.** They shared a factual premise:
that uncrancher is "tied for the highest field-yield of the five sampled repos." It is not.
The proposal's own table gives shadow-work and ravenous 5/6 each and uncrancher 4/6.
Convergence measured agreement between lenses, not agreement with the table. Treat the spine
as high-confidence about *what the lenses believe*, and check the cheapest fact in any spine
row before acting on it.

---

## 5. Live Disagreements

### The flagged one — closed, but by a lens warned against exactly this

The controller carried one open disagreement into the halt:
`c-uncrancher-chained-originations`, f-006 versus f-025 — does confirming unc-rancher's card
force a sequential success-then-guardrail origination chain in one sitting?

Round 3 dispatched the `adjudication` lens, which produced f-046 and f-047 in favor of f-025,
then the round returned yield 0 and the loop halted DRY. But `adjudication`'s own failure-mode
card names the risk it then ran: *"Restates what one prior lens already established without
adding independent verification, inflating apparent confidence without new evidence."* f-046
cites the same two line ranges as f-025, quotes the same doc sentence, and reaches the same
conclusion. It is a restatement.

**I am closing it with independent evidence rather than with the adjudicator's word.** I ran
the validator. A card with all five fields declined and `status: confirmed` exits 0 — which
directly proves declined satisfies R3 permanently, no ordering, no session semantics. f-025
is correct, f-006 is refuted, and f-047's corrective is the useful residue: the real
unblocking action for unc-rancher is two short declined stubs, each one `reason` sentence and
one `needs` sentence, modeled by the format doc's own worked example for that exact project.

### The unflagged one — is unc-rancher the right first subject?

**This is the live disagreement, and the loop never registered it as one.** North-star
question 2 now has **no upheld finding on either side.**

Three lenses independently argued no (f-005, f-011, f-017), all on the same false premise
about field-yield ranking, and all three were refuted together. But each carried a second,
separable argument that the refutation did not touch:

- `fd-hci-ratification-cost` axiom 6: *"The representativeness of the pilot subject (its
  resourcing, incentives, existing material) bounds how far its measured cost can be
  generalized."*
- `fd-infoarch-ceremony-risk` axiom 4: *"A pilot validated on an atypically well-incentivized
  or well-resourced instance (funded, externally visible) does not establish that a mechanism
  works for the representative, unfunded case it is ultimately meant to serve."*
- `fd-fused-ratification-toll-integrity` axiom 5: *"A pilot run on a subject whose maker
  independently wants the mark measures the toll where nobody is evading it and certifies a
  property only that subject has — one measurement, two failed generalizations."*

The proposal's stated reason for choosing unc-rancher is exactly this property: *"it is a
real GSV games-slate item, so its card is also the funding page's sales copy, which is the
coupling this design rests on."* That coupling is the strongest reason to pick it (it is the
only subject where the card has an external reader who will notice if it lies) and the
strongest reason to distrust its cost number (it is the only subject where mk wants the card
to exist for reasons independent of the gate). Both are true. Neither was tested.

This is an unresolved taste call, and it is mk's to make. Two shapes, both cheap:

1. **Keep unc-rancher, add a second subject at the far end.** jawnomicon drafts 2/6 with 0
   plans; its confirm session is structurally different (mostly-decline) and no baseline for
   it exists. Two data points bracket the estate instead of anchoring one end of it. Cost:
   one more confirm session, and it directly stresses the declined branch that f-035 says is
   the load path.
2. **Keep the n=1 and stop generalizing from it.** Drop "ruling 12 commits the estate to
   doing it 82 times and that number has never been measured" as the stated rationale, and
   record the confirm cost as what it is: the cost for the one project mk most wanted a card
   for. Cost: zero, but ruling 12 stays unpriced.

I recommend (1). The measurement already established that artifact count does not predict
yield, which means a single sample cannot be assumed representative on the dimension that
matters, and the second session is the cheapest thing in this entire review that produces new
information.

---

## If you read one thing

**f-009** — heat 27, taste −2, `fd-infoarch-ceremony-risk`. Tied at the corner with f-001
and f-035 on heat and with f-001 on |taste|; broken on survivability. Patching `strength()`
leaves f-009 intact. Adding one falsifiable DONE WHEN clause would have surfaced f-001 during
the pilot.

> The goal poses the ceremony-vs-instrument question in its own text and then equips itself
> with seven completion criteria, none of which can return false on it.

The fix is one clause and it does not need to be sophisticated. Something a session can
check: *"a week after the card is confirmed, mk names one thing they did differently because
of it — or records that they did not, and that goes into the brainstorm as a ruling with the
same weight as a yes."* A criterion that can fail is the only thing separating this goal from
the 2,270 plans it was written to explain.

---

## Appendix: Spice Trail

**Round 0 — assay. 22 findings, 5 lenses in 2 dispatch waves, novel_cluster_rate 0.86.**
Adjacent tier: `fd-datamodel-format-durability` (schema/API contract durability),
`fd-hci-ratification-cost` (repeated-ratification ergonomics), `fd-infoarch-ceremony-risk`
(documentation-estate strategy). Distant tier: `fd-assay-mark-integrity` (guild hallmarking
metrology), `fd-archival-backlog-appraisal` (archival appraisal and finding aids). The
distant pair earned their slots immediately — assay-mark-integrity produced f-013/f-014/f-015
(the hollow-mark family, the highest-yield vein in the run) and archival produced f-018, the
ground-truth check that found the local tree 8 commits behind. Yield 22, novel cluster rate
0.86: nearly every finding opened a new cluster, which is what a well-spread seed looks like.

**Round 1 — probe. 4 directives, 11 findings, novel_cluster_rate 0.64.**
- `PROBE-DISAGREEMENT` (no lens) — f-007 versus f-014 read the same R3 in opposite
  directions. **This was the highest-value directive in the run.** It did not pick a winner;
  it produced f-023 (the errors are not opposite) and f-024 (the exit code's real consumer is
  the plan gate, not a hypothetical TUI ranker) — the finding that moved the hollow-card
  problem from future to present tense.
- `DEEPEN` ×2 on `fd-datamodel-format-durability` — risk-6 unconfirmed findings needing
  confirm-or-refute. Produced f-025 through f-030: the adjudication of f-006, the stale worked
  example, the missing confirmed example, and the two DONE WHEN defects (f-029, f-030).
- `STEER-WIDE` on `fd-timberframe-load-path` — fired because novel_cluster_rate 0.86 ≥ 0.6,
  the widening-still-pays threshold. Timber-frame scribe-rule reasoning produced f-031 (the
  flat DONE WHEN list, §1 Shoulder B), f-032 (the fresh card is INVALID), and f-033 (four row
  states). Three findings, three new clusters, one of them a frontier shoulder. The widening
  paid.

**Round 2 — probe. 4 directives, 12 findings, novel_cluster_rate 0.50.**
- `PROBE-DISAGREEMENT` again — produced f-034, the null. The pair had been mined out in round
  1 and the controller had no way to know until it spent the slot.
- `DEEPEN` on `fd-datamodel-format-durability` — produced f-036/f-037/f-038, the prose-body
  family. f-036 is where the local-tree gap bit: the agent confirmed the mechanism by reading
  code and running the test suite, then had to record that the attribution to
  `product-card-format.md` §Shape *"could not be verified, because that file does not exist
  anywhere in the local tree."* Correct and careful, and wrong only because `git show` was
  never tried.
- `FUSE` on `fd-hci-ratification-cost` × `fd-assay-mark-integrity` (shared_heat 2,
  complementarity 2, redundancy 3) — generated `fd-fused-ratification-toll-integrity` and
  produced f-039, f-040, f-041, three findings no single parent reaches. The fused lens's own
  hard constraint (report only what needs both parents) held: all three carry real
  intersection justifications.
- `STEER-WIDE` on `fd-subak-flow-allocation` — fired at novel_cluster_rate 0.64 ≥ 0.6.
  Balinese irrigation produced f-042 (ranking trivial at zero, converging with f-030 from a
  completely different direction), f-043 (Mycroft — the only lens to look outside the
  proposal at what else runs in this repo), f-044 and f-045. Its axiom *"an ordering is not an
  allocation"* is the sharpest single sentence any lens brought to the phase-2 question.

**Round 3 — probe. 1 directive, 2 findings, novel_cluster_rate 0.00. HALT: DRY.**
Only `PROBE-DISAGREEMENT` remained eligible; the `adjudication` lens produced f-046 and f-047
into an existing cluster and opened nothing new. Gain history 11 → 6 → 6 → 0 with novel
cluster rate 0.86 → 0.64 → 0.50 → 0.00. The halt is real convergence, not a budget clamp: 19
of 30 slots spent, 11 remaining and unspendable on anything the controller could see.

**Fusion stats:** 1 pair attempted, 4 emergent findings, 1 null on the re-probe.
**Failed probes:** 0 across all four rounds.

---

## Caveats

**The reviewed documents were not in the reviewed worktree.** `Sylveste/apps/Autarch` local
HEAD `7c02ffd` is 2 ahead / 8 behind `origin/main`. `product-card-format.md`,
`card-drafter.md`, and the measurement exist only at `38715de` on `origin/main`. Multiple
lenses recorded "file does not exist" and either refuted or softened findings on that basis;
none tried `git show`. I lifted the clamp for `product-card-format.md` and it flipped two
findings from refuted to upheld (f-013, f-002). **`card-drafter.md` was never read by any
lens in any round, including by me** — the drafter procedure is a `depends_on` of the goal
and is entirely unreviewed. Anything the procedure specifies about writing declined stubs for
undraftable fields — the exact mechanism f-032 and f-035 turn on — is unassessed.

**North-star question 2 has no upheld finding on either side.** Three lenses converged on
"unc-rancher is the wrong first subject," all three were refuted on a shared factual error
about field-yield ranking, and the separable representativeness argument each carried was
never re-tested. See §5.

**The phase-2 target was never inspected.** `pkg/tui` already contains 48 files — chat
panels, agent selectors, chat settings, a chat stream. The proposal describes building "the
thin tmux-native door on `pkg/tui`" as though onto a bare package. No lens read a line of it.
Whether the project-row door is a new surface or a change to an existing one, and what
already renders session state there, is unknown to this review.

**A sixth card exists on disk and no lens found it.**
`/Users/sma/projects/After-Them/docs/why.md`, untracked, `card-check.py` exit 1, three fields
drafted with real project-scope citations, `line: null` with an inline comment explaining the
R4 block. It is outside the five measured repos. It corrects f-033 and it raises a question
this review cannot answer: it was drafted by something, recently, into a repo the measurement
never covered, and ruling 12 says backfill on first touch, never bulk.

**Region never reached: the funding-page reader.** The fused lens named "the downstream buyer
— the funding-page stranger with money — relying on a mark the maker also struck" as a
primitive, and f-013/f-036/f-037/f-038 circle it, but no lens examined the GSV funding page,
what it renders, or what happens to it when a card's prose body is empty. That is the one
place in this design where a party other than mk relies on the mark.

**Region never reached: the 21-to-82 mapping.** f-012, f-019 and f-028 all note the
denominator problem; none establishes what the actual mapping is. Whether 21 live tmux
sessions cover 21 distinct projects, how many of the 82 are dormant, and what "all 21 sessions
as project rows" resolves to after decision 2's one-row-per-project collapse are unmeasured.

**Fusion coverage is thin.** One pair fused out of 21 available from seven base lenses. Three
promising pairs are named in §2 and were never attempted.

**Taste was unscored by every agent.** All 47 ledger rows carry `taste: 0` and
`taste_kind: null`. §3 is entirely my re-score; treat it as one reader's calibration, not
convergent signal.

**Severity is reference only.** The ledger's P0/P1/P2/P3 labels are triage-grade and were not
re-derived. Rank by heat.
