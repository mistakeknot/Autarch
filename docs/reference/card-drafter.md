---
artifact_type: reference
bead: none
stage: build
---

# The card drafter

**Version:** 1
**Format:** `docs/reference/product-card-format.md`
**Measurement that produced these rules:** `docs/analysis/2026-08-13-card-drafter-measurement.md`
**Validator:** `card-check.py` (dotfiles `common/.local/bin/`)

You have been asked to draft a product card for a repo, or you hit the plan gate
and need one. This is the procedure. Follow it literally — the rules below were
derived from a five-repo measurement, and the three near-misses they catch all
read as correct answers.

**Your job is not to fill in six fields. Your job is to refuse to invent.** A
card with two drafted fields and four honest declines is a good card. A card with
six plausible fields is the thing this format exists to prevent.

## Procedure

1. **Read `README.md` first, in full.** The measurement found READMEs carry
   project-scope architecture invariants that no plan ever states. `jawnomicon`
   has zero plans, zero PRDs and zero CUJs, and its README still yields a
   drafted, project-scope guardrail. Do not skip to `docs/`.
2. **Read `docs/cujs/` if it exists.** Take `actor` for `persona` and
   `mental_model` for `pain`. Note the format: cujgel JSON carries a stable
   `cuj_id`; prose markdown CUJs do not, and then `cuj.ref` is null.
3. **Read `docs/decisions/`.** These become the `decisions` link list. They are
   also the most likely place a real guardrail is written down.
4. **Skim `docs/prds/` for metric language,** and apply rule 2 below ruthlessly
   to everything you find there.
5. **Do not read `docs/plans/`.** Plans state what will be done, never who for.
   The estate has 2,270 of them and 3 personas; reading 455 plan files is how you
   burn an hour and draft nothing. If a plan is the only place a fact appears, it
   is not project scope anyway.
6. **Write the card**, then **run the validator** and fix what it refuses:
   ```
   card-check.py <repo-root>          # 0 confirmed, 1 provisional, 2 invalid, 3 absent
   ```
   A freshly drafted card should come back `1 provisional`. If it comes back
   `2 invalid`, you broke a rule — read the message, it names which.
7. **Never set `status: confirmed`.** Only mk does that. The validator refuses a
   confirmed card that still has drafted fields (R3), so you cannot do it by
   accident, but do not try.

## The three rules

### 1. Cite or decline (hard — the validator enforces it)

Every `drafted` field carries at least one evidence entry naming an in-repo path.
If you cannot point at the line you read it from, you did not read it — you wrote
it. Decline instead.

```yaml
evidence:
  - { path: "README.md:11-14", scope: project }
```

### 2. Scope must match (hard — the validator enforces it)

`success` and `guardrail` need at least one evidence entry with `scope: project`.
Declare the scope of everything you cite: `project`, `epic`, `journey`,
`external`.

This rule exists because rule 1 is not enough. **4 of the 5 measured repos offer
a citation-passing wrong answer for `success`:**

- A cujgel `success_condition` is a per-journey user-recognition test — *"the real
  user recognizes this as theirs."* It is `scope: journey`. It is not a project
  metric, however much it sounds like one.
- A PRD section headed "Success Metrics (Epic Definition of Done)" is
  `scope: epic`. Those criteria can be excellent and still not be the project's.

Misattribution survives review in a way invention does not: the citation
resolves, the quoted text is real, and only the scope is wrong.

### 3. Outcome, not inventory (soft — nothing enforces this but you)

A success metric names a direction or a threshold. A count of things that exist
is inventory.

`jeddnet/README.md` states "204 scored items and 56 second-turn Sassho probes"
and a "1130-test partition." Project scope. Citable. Number-shaped. Not a success
metric — nothing in the repo says what score would count as the instrument
working. Both hard rules pass it. Only this question catches it.

No check can be written for this one, so it is stated as an instruction rather
than pretended into a rule.

## How to decline well

```yaml
success:
  state: declined
  reason: "README:18 and README:32 give item counts (204 scored, 56 probes) and a
    CI partition (1130 tests). Both are project-scope and citable, and both are
    inventory rather than outcome — nothing states what score counts as the
    instrument working."
  needs: "A threshold or direction: what result would make MênisBench v1 a success?"
```

`reason` says what you looked at and why it did not qualify. `needs` says what
would settle it — that is the question mk answers, so make it answerable in one
line.

**A decline may cite evidence, and that is its strongest form.**
`shadow-work/README.md:32` says *"No Win Conditions: Like Dwarf Fortress, success
is subjective."* The project states at project scope that its success is
undefinable. "The repo says there is no metric" and "I could not find a metric"
are different claims and must not render identically.

## What to expect

From the measurement, per field, across five repos:

| Field | Drafted | Note |
|---|:--:|---|
| `persona` | 4/5 | Usually inferable, most often from a CUJ `actor` |
| `guardrail` | 4/5 | More draftable than expected — READMEs state invariants |
| `pain` | 3/5 | Inferable when the README says what is wrong today |
| `cuj` | 3/5 | Only where `docs/cujs/` exists |
| `line` | 3/5 | Requires persona **and** pain to both be non-declined |
| `success` | **0/5** | Expect to decline this. It is the correct output. |

If you draft a success metric, stop and re-read rule 2 and rule 3. You are
probably about to commit one of the four near-misses.

**Artifact count does not predict yield.** `shadow-work` has 455 plans and
declines `success`; `jawnomicon` has no `docs/` artifacts at all and drafts a
guardrail. Do not assume a thin repo is not worth drafting, or that a thick one
will be easy.

## Do not bulk-draft

Ruling 12: cards are drafted **on first touch, never in bulk**. Drafting 82 cards
produces 82 unreviewed provisional cards — a backlog wearing a rigor costume, and
a `why` column that is uniformly `provisional` and therefore just as invisible as
one that is uniformly `missing`. If you have been asked to draft cards for many
projects at once, say this and ask which one is actually being worked on.
