---
artifact_type: analysis
bead: none
stage: build
---

# What a drafter can honestly produce — five repos, measured

**Date:** 2026-08-13
**Answers:** open question 2 of `docs/brainstorms/2026-08-13-autarch-universal-interface-brainstorm.md`
**Format under test:** `docs/reference/product-card-format.md`

Open question 2 asked what an agent can honestly draft, and warned that a guessed
success metric produces exactly the fake rigor the card exists to prevent. That is
a claim about behaviour, so it was measured rather than argued.

## Method

Five repos chosen to span the material gradient — from an estate-record 455 plans
to a repo with no `docs/` artifacts at all:

| Repo | plans | PRDs | CUJs | ADRs |
|---|---:|---:|---:|---:|
| `shadow-work` | 455 | 27 | 13 | 1 |
| `jeddnet` | 38 | 0 | 0 | 5 |
| `uncrancher` | 18 | 0 | 7 | 0 |
| `ravenous` | 8 | 0 | 6 | 0 |
| `jawnomicon` | 0 | 0 | 0 | 0 |

For each repo, each of the six card fields was drafted under the citation rule
(cite an in-repo path or decline) and recorded as **drafted**, **declined**, or
**guessed**. A *guessed* field is one where the drafter produced a value it could
not honestly stand behind — the failure mode that matters, since a guessed
success metric is indistinguishable from a real one at a glance and is the number
a funding page would quote to a stranger.

Guesses are counted **explicitly**, including near-misses caught before the value
was written down. Counting only successes would make the drafter look better than
it is.

## Per-field results

`D` = drafted with citation · `—` = declined · **bold** = the interesting case

| Repo | line | persona | pain | cuj | success | guardrail |
|---|:--:|:--:|:--:|:--:|:--:|:--:|
| `shadow-work` | D | D | D | D | **—** | D |
| `jeddnet` | **—** | **—** | D | — | **—** | D |
| `uncrancher` | D | D | D | D | **—** | **—** |
| `ravenous` | D | D | D | D | **—** | D |
| `jawnomicon` | **—** | D | **—** | — | **—** | D |
| **drafted** | 3/5 | 4/5 | 3/5 | 3/5 | **0/5** | 4/5 |
| **declined** | 2/5 | 1/5 | 2/5 | 2/5 | **5/5** | 1/5 |
| **guessed** | 0 | 0 | 0 | 0 | **0** | 0 |

Zero fields were written as guesses. Four were caught as near-misses before being
written, and those are the whole result — see below.

## The four near-misses

Each of these is a value the drafter was one step from writing, that would have
passed review, and that is wrong.

**1–2. `ravenous`, `uncrancher` — journey `success_condition` read as project
success.** Both carry cujgel CUJs whose `success_condition` is prose that sounds
exactly like a success metric: *"mk plays a live session, gets pause-and-zoomed on
a quarrel in a ward they'd neglected, and recognizes the moment as the city
grabbing them by the collar."* Per `cujgel.schema.json` that field is a
user-**recognition** test for one journey — "the real user recognizes this as
theirs." It is not a project outcome measure. Caught by the scope rule.

**3. `shadow-work` — an epic's Definition of Done read as the project's.**
`docs/prds/2026-04-19-shadowbench-v01-release.md:238` has a section literally
titled "Success Metrics" with five genuinely measurable criteria (artifacts
published within a 2-hour window; an outside reader reproduces scores in <60
minutes; ≥50% of methodology questions answered within 24h). All real, all
excellent, and all scoped to the epic `shadow-work-6o5q`. Caught by the scope
rule.

**4. `jeddnet` — inventory counted as outcome.** `README.md:18` and `README.md:32`
state "204 scored items and 56 second-turn Sassho probes" and a "1130-test
partition: 1055 portable tests… and 75 exact readiness tests." Project scope,
citable, number-shaped. Defeats both hard rules. It is a count of things that
exist, and nothing in the repo says what score would count as MênisBench working.
Caught only by asking whether the number names a direction.

**The headline: the citation rule alone would have produced a wrong success
metric in 4 of 5 repos.** Misattribution is more dangerous than invention because
it survives review — the citation resolves, the quoted text is real, and only the
scope is wrong. This is why the format now carries three rules and not one.

## Four findings that change the design

**1. Artifact count does not predict card yield.** `jawnomicon` has zero plans,
zero PRDs and zero CUJs, and still yields a drafted, project-scope guardrail:
`README.md:17` — "Git-canonical creature files are the source of truth; Neo4j is a
rebuildable build product; games consume versioned published exports — never the
live DB." Meanwhile `shadow-work`, with 455 plans, declines `success`. READMEs
state architecture invariants; plans do not state why. The 2,270-plan number
diagnosed in the brainstorm is not merely unhelpful for cards — it is
*uncorrelated* with them.

**2. The estate's projects are rich and poor in mirror-image ways.** The games
(`ravenous`, `uncrancher`) have personas and journeys and no metrics. `jeddnet`
has directional guardrails and no stated audience. A single "coverage %" over the
six fields would average these into a meaningless middle. Coverage must be
reported per field.

**3. `shadow-work` declines `success` *with a citation*.** `README.md:32`:
"**No Win Conditions**: Like Dwarf Fortress, success is subjective." The project
states, at project scope, that its success is undefinable. This is the strongest
possible decline — an evidenced refusal rather than an absence — and the format
should let a decline cite its evidence, not only state a reason.

**4. The strict `line` rule blanks a project that has real material.** `jeddnet`
has a crisp project-scope pain (`README.md:11-14`: can a model reason about mênis
and acedia "without silently importing a modern therapeutic or biomedical
script?") and no stated audience anywhere. Under the persona-**and**-pain rule its
row shows nothing. Kept strict: a why with no who is a description. The cost is
recorded here so it can be overruled against the evidence rather than in the
abstract.

## What was deliberately not done

The five drafted cards were **not** written into their repos. Writing five
unreviewed `provisional` cards into five repos is a five-project version of the
bulk backfill this same work rules against (finding 1 in the brainstorm). They
live here as measurement output until mk rules on the format; landing them is one
command afterwards.

## Sample output — `jeddnet`

The richest decline in the set, reproduced in full because the reasons are the
product:

```yaml
---
artifact_type: card
card_version: 1
project: jeddnet
status: provisional
confirmed_by: null
confirmed_at: null
line: null
fields:
  persona:
    state: declined
    reason: "The README states the research question and the instrument in detail
      but never names who consumes the result. An academic audience is inferable
      from 'publication still requires independent source review' — inference, not
      citation."
    needs: "One line from mk: who reads a MênisBench result and does what with it."
  pain:
    state: drafted
    value: "Models import a modern therapeutic script into ancient anger"
    evidence:
      - { path: "README.md:11-14", scope: project }
  cuj:
    state: declined
    reason: "No docs/cujs/ directory. 38 plans and 5 decision records, no journey."
    needs: "One derived CUJ, or a ruling that a benchmark has no user journey."
  success:
    state: declined
    reason: "README:18 and README:32 give item counts (204 scored, 56 probes) and a
      CI partition (1130 tests). Both are project-scope and citable, and both are
      inventory rather than outcome — nothing states what score counts as the
      instrument working."
    needs: "A threshold or direction: what result would make MênisBench v1 a success?"
  guardrail:
    state: drafted
    value: "No result licenses a claim it was not built to support"
    evidence:
      - { path: "README.md:14-16", scope: project }
      - { path: "README.md:24-26", scope: project }
decisions:
  - "docs/decisions/2026-07-10-executable-protocol-freeze.md"
  - "docs/decisions/2026-07-14-owner-capability-trust.md"
  - "docs/decisions/2026-07-15-menisbench-v1-terminal-no-go.md"
---
```

Note that `guardrail` drafts cleanly here while `success` cannot. A project can
know what it must not claim long before it knows what it is trying to prove — and
the card is the first artifact in the estate that can say both.
