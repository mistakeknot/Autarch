---
artifact_type: reference
bead: none
stage: build
---

# The product card — `docs/why.md`

**Version:** 1
**Status:** normative for phase 1
**Brainstorm:** `docs/brainstorms/2026-08-13-autarch-universal-interface-brainstorm.md`

A card states who a project is for, what hurts, and how anyone would know it
worked. One per project, in-repo, at `docs/why.md`. It is read three ways: by mk
at a glance in a switcher row, by an agent before it writes a plan, and — for
slate items — by a stranger deciding whether to fund it.

## Why a file and not a database row

The card travels with the code, diffs in review, and survives Autarch. An agent
working inside a repo can read it without knowing Autarch exists. The derived
index used for ranking is a projection of these files, never the source.

## Shape

YAML frontmatter carries the machine-readable card. The prose body below it is
free-form and is what the funding page renders as sales copy.

```yaml
---
artifact_type: card
card_version: 1
project: unc-rancher
status: provisional          # provisional | confirmed
confirmed_by: null           # required when status is confirmed
confirmed_at: null           # ISO date, required when status is confirmed
line: "Ranchers who lose stock to gates left open"   # <= 48 chars
fields:
  persona:
    state: drafted           # drafted | declined | confirmed
    value: "Small-herd owner running fence line alone"
    evidence:
      - { path: "docs/cujs/2026-05-02-gate-check.md:14", scope: journey }
  pain:
    state: confirmed
    value: "A missed open gate costs a day of riding to find strays"
    evidence:
      - { path: "README.md:8", scope: project }
  cuj:
    state: confirmed
    ref: "uncrancher-01-walk-the-fence"   # null when the CUJ file carries no stable ID
    path: "docs/cujs/2026-05-02-gate-check.md"
  success:
    state: declined
    reason: "No baseline exists — nothing in the repo records how often stock is lost today"
    needs: "One season of gate-state observations, or mk's own number"
  guardrail:
    state: declined
    reason: "Cannot state what must not get worse without a success metric to bound"
    needs: "Resolution of success first"
decisions:
  - "docs/decisions/2026-05-11-offline-first.md"
---
```

## The six fields

| Field | What it answers | Drafting difficulty |
|---|---|---|
| `line` | The glanceable why, shown in the row | Derived from `persona` + `pain` |
| `persona` | Who this is for | Usually inferable |
| `pain` | What hurts today | Usually inferable |
| `cuj` | The journey, by reference | Inferable only if a CUJ file exists |
| `success` | How anyone knows it worked | Rarely inferable |
| `guardrail` | What must not get worse | Rarely inferable |

`decisions` is a list of paths, not a state-carrying field. It is a link set, and
an empty one is not a defect — most projects legitimately have no ADR yet.

**`line` requires both `persona` and `pain` to be non-declined.** A who without a
why, or a why without a who, is a description, and the row column exists to
answer both. This rule is strict on purpose and it has a measured cost: jeddnet
has a crisp, citable, project-scope pain and no stated audience anywhere in the
repo, so its row shows nothing. That is the true state of jeddnet and the column
should say so.

**`cuj.ref` may be null.** The estate's 126 CUJs are not one format: `ravenous`
and `uncrancher` carry cujgel JSON with a stable `cuj_id`; `shadow-work` carries
prose markdown with a title and no ID. When no stable ID exists, `ref` is null and
`path` alone identifies the journey.

## The three drafting rules

The drafter's whole job is refusing to invent. Three rules, in decreasing order
of how mechanically they can be enforced. All three were derived from — not
assumed before — the five-repo measurement in
`docs/analysis/2026-08-13-card-drafter-measurement.md`.

### 1. The citation rule (hard, machine-checked)

> **A field in state `drafted` MUST carry at least one `evidence` entry naming an
> in-repo path. A field with no citable source MUST be `declined`.**

This separates a drafted card from a plausible-sounding invention. An agent that
cannot point at the line it read the persona from did not read it — it wrote it.

### 2. The scope rule (hard, machine-checked)

> **`success` and `guardrail` in state `drafted` MUST cite at least one evidence
> entry with `scope: project`.**

Every evidence entry declares the scope of what it cites: `project`, `epic`,
`journey`, or `external`. This rule exists because the citation rule alone is
**not sufficient** — the measurement found that 4 of 5 repos offer a
citation-passing *wrong* answer for `success`:

- `success_condition` on a cujgel CUJ is a per-journey user-recognition test
  ("the real user recognizes this as theirs"), not a project outcome measure.
  Lifting it produces a metric that reads correctly and is scoped wrongly.
- A PRD's "Success Metrics (Epic Definition of Done)" section is scoped to that
  epic. `shadow-work/docs/prds/2026-04-19-shadowbench-v01-release.md:238` states
  five genuinely measurable criteria — for the epic `shadow-work-6o5q`, not for
  shadow-work.

Misattribution is more dangerous than invention, because it survives review: the
citation resolves, the quoted text is real, and only the scope is wrong.

### 3. The outcome rule (soft, drafter judgment)

> **A success metric must name a direction or a threshold. A count of things that
> exist is inventory, not outcome.**

This one resists mechanization and is stated as a drafter instruction rather than
a check. It exists because jeddnet defeats both hard rules: `README.md:18` and
`README.md:32` state "204 scored items and 56 second-turn Sassho probes" and a
"1130-test partition" — project-scope, citable, number-shaped, and not a success
metric. Nothing in the repo says what score would count as the instrument
working.

Where a check cannot be written, the honest move is to say so rather than to
pretend the soft rule is enforced.

### What the rules produce

`success` came back `declined` in **5 of 5** repos. That is the correct output,
not a failure of the drafter. A guessed success metric is the exact fake rigor
the card exists to prevent: indistinguishable from a real one at a glance, and it
is the number a funding page would quote to a stranger.

## The three field states

- **`drafted`** — an agent inferred it and cited its source. Not yet trustworthy.
- **`declined`** — the agent refused. Carries `reason` (why it could not be
  drafted) and `needs` (what would settle it), and **may** carry `evidence`. A
  declined field is a recorded unknown, which is a stronger artifact than a wrong
  known.

  A decline can be *evidenced*, and that is the strongest form of it.
  `shadow-work/README.md:32` reads "**No Win Conditions**: Like Dwarf Fortress,
  success is subjective" — the project states at project scope that its success is
  undefinable. "The repo says there is no metric" and "I could not find a metric"
  are different claims and must not render identically.
- **`confirmed`** — mk ratified it, having read the draft or written it fresh.

`declined` is deliberately a projection of cujgel's
`ambiguity_ledger.open_questions` — "genuinely undecided; the agent SHOULD ask,
not guess." The vocabulary already existed in the estate and is not reinvented
here.

## Card status

`status: confirmed` requires **no field in state `drafted`**. Every field must be
either `confirmed` or `declined`. Confirming a card whose fields mk never touched
is the rubber stamp this design exists to prevent, so it is refused mechanically
rather than discouraged in prose.

A card may be confirmed with `success` and `guardrail` declined. That is mk
saying, on the record, "I do not yet know how I would tell if this worked" —
which is true of most of the estate and is worth saying out loud.

## Relationship to cujgel

**The card references a CUJ by ID; it never inlines one.** `cujgel.schema.json`
requires `mental_model` and `success_condition` and carries an
`ambiguity_ledger`, `implied_features`, and `evidence` citations into raw session
captures. Copying that into the card would create two sources of truth for one
journey and make the card unglanceable — a full CUJ is a page, and the card is a
row.

The card is an index into the estate's existing artifacts, not a replacement for
them. `cuj.ref` holds a `cuj_id`; `cuj.path` holds the file so the row can open
it.

## Absent, provisional, confirmed

- **Absent** — no `docs/why.md`. The state of all 82 projects as of 2026-08-13.
- **Provisional** — a card exists, drafted by an agent, unreviewed.
- **Confirmed** — mk has ratified it.

The gate (`guard-plan-needs-card.sh`) refuses plan-writing on absent and
provisional, permits on confirmed, and logs every override with its reason.
Nothing else is blocked — exploration, code, tests, and commits are untouched.
