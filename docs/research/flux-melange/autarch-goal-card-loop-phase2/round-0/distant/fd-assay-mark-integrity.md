# fd-assay-mark-integrity — round 0

## Findings Index
- [P0] confirmed-never-validates-sales-copy — card-check.py parses only the frontmatter; the prose body it is used to sell unc-rancher with is never inspected (§card-check.py validate/split_frontmatter)
- [P0] declined-everything-exits-confirmed — five declines + null `line` + a self-attested `confirmed_by` passes exit 0 with zero real content (§card-check.py R3/R4)
- [P1] strength-score-erases-the-load-bearing-gap — averaging six slots hides whether the missing field is `success` or anything else (§card-check.py strength())
- [P1] cuj-evidence-shape-undemonstrated — R1 requires evidence on a drafted `cuj`, but no doc or sample ever shows one passing (§product-card-format.md Shape / card-drafter.md step 2)
- [P1] unc-rancher-is-the-richest-available-ore — the chosen sample drafts 4/6, tied for best in the 5-repo measurement (§proposal "Start with unc-rancher")

## Findings

### confirmed-never-validates-sales-copy
- **Severity:** P0
- **Where:** `dotfiles/common/.local/bin/card-check.py` `split_frontmatter()` (lines 55-70) and `validate()` (lines 110-214); `Sylveste/apps/Autarch/docs/reference/product-card-format.md` §"Shape"
- **What:** `card-check.py` parses only the YAML block between the two `---` delimiters (`split_frontmatter` returns `lines[1:i]` and nothing else); `validate()` operates exclusively on that parsed dict. Nothing in the tool ever reads or checks anything after the closing `---`. A `docs/why.md` with a fully `confirmed` frontmatter (all five fields `state: confirmed`, non-empty values, `confirmed_by`/`confirmed_at` set) and an empty or absent prose body still validates cleanly and exits 0 — the sole silent-permit verdict for the plan gate, and the verdict the proposal's DONE WHEN clause treats as the test's success condition. But per the format doc's own description, the prose body — not the frontmatter — "is what the funding page renders as sales copy." `confirmed` therefore certifies frontmatter field-presence and citation form and is silent on the one artifact unc-rancher's card was chosen specifically because it doubles as.
- **Evidence:** `card-check.py:55-70` (frontmatter extraction stops at the second `---`); `card-check.py:110-214` (validate() never touches `text` past the frontmatter slice); `product-card-format.md` §Shape: "The prose body below it is free-form and is what the funding page renders as sales copy."
- **Suggestion:** Add a minimal check (even a non-empty-body floor) gating `confirmed` on the prose existing, or state explicitly in the DONE WHEN clause that "confirmed" does not cover the sales copy before unc-rancher's card is used to raise money.

### declined-everything-exits-confirmed
- **Severity:** P0
- **Where:** `card-check.py` R3 block (lines 197-209) and R4 `line` check (lines 185-195); `strength()` (lines 217-238)
- **What:** R3 only checks that no field is `drafted`; `declined` is a fully legal terminal state for every one of the five fields given a `reason` and `needs` (lines 172-179). R4's persona/pain consistency check fires only `if line:` (line 187) — a null `line` skips it entirely. So `status: confirmed`, all five fields `declined`, `line: null`, and any string in `confirmed_by`/`confirmed_at` (the validator never checks that string is actually mk) parses cleanly and returns exit 0. `strength()` on the same card scores 0/6. The license to write a plan into `docs/plans/*.md` is obtainable with zero real content and zero human involvement — an agent hitting the gate could write this card itself and self-attest `confirmed_by: "mk"`.
- **Evidence:** `card-check.py:197-209` (`drafted = [n for n in FIELDS if states[n] == "drafted"]`; raises only if non-empty); `card-check.py:185-195` (`if line:` guards the whole R4 block, so a null line bypasses it); `card-drafter.md` "Never set `status: confirmed`. Only mk does that." is prose instruction only, not enforced anywhere in `card-check.py`.
- **Suggestion:** Require at least one field in state `confirmed` (not merely "no field drafted") for `status: confirmed`, and/or require `line` to be non-null whenever status is confirmed so R4 cannot be bypassed by omission.

### strength-score-erases-the-load-bearing-gap
- **Severity:** P1
- **Where:** `card-check.py` `strength()` (lines 217-238); proposal DONE WHEN clause "ranking matches ruling 11 against real card strengths"
- **What:** `strength()` counts confirmed fields unweighted across six slots. The measurement found `success` drafts 0/5 across every repo tested and is, in the proposal's own words, "the field the whole premise depends on." A card honestly confirmed on persona/pain/cuj/guardrail/line but permanently `declined` on `success` (because no baseline can exist) scores 5/6 — indistinguishable from a card that's 5/6 for any other reason, e.g. mk simply hasn't gotten to `guardrail` yet. Ruling 11's "weakest card first" cannot tell "the estate's hardest field, correctly and permanently declined" from "an easy field nobody has touched," so the row most needing attention sinks to the same rank as a merely-unattended one.
- **Evidence:** `card-check.py:230-238` (`counts["confirmed"] += 1` per field, no per-field weight); `docs/analysis/2026-08-13-card-drafter-measurement.md` — `success` row: 0/5, "This is the field the whole premise depends on, and it is the one an agent cannot produce."
- **Suggestion:** Emit `strength.score` as two figures — confirmed-of-six, and a separate flag for whether `success` specifically is confirmed — before phase 2 wires the single scalar into row order.

### cuj-evidence-shape-undemonstrated
- **Severity:** P1
- **Where:** `card-check.py` R1 (lines 153-159, applies to every field in `FIELDS` including `cuj`, no name exemption unlike the R2/value checks two lines below); `product-card-format.md` §Shape (`cuj` shown only as `state: confirmed`); `card-drafter.md` step 2
- **What:** R1 (citation) applies uniformly to any `drafted` field, `cuj` included — only R2 (scope) explicitly exempts `cuj` (line 162). The format doc's only worked YAML example shows `cuj` as `state: confirmed` with `ref`/`path` keys and no `evidence` list; `card-drafter.md` step 2 tells the drafter to take `actor`→`persona` and `mental_model`→`pain` but never instructs attaching `evidence` to the `cuj` field itself. The measurement records `cuj` as drafted (`D`) in 3 of 5 repos, including uncrancher — the chosen first subject — but the one fully-reproduced sample card (jeddnet) has `cuj: declined`, so a drafted-and-validator-passing `cuj` has never actually been demonstrated end to end.
- **Evidence:** `card-check.py:153-159` (R1, no cuj exemption) vs. `card-check.py:169` (`if name != "cuj" and not spec.get("value")` — the *value* check is exempted, evidence is not); `product-card-format.md` `cuj:` block under §Shape; `docs/analysis/2026-08-13-card-drafter-measurement.md` per-repo table, uncrancher row: `cuj: D`.
- **Suggestion:** Before drafting unc-rancher's card, hand-build a drafted `cuj` entry (ref + path + a plausible evidence list) and run it through `card-check.py` to confirm the shape validates; add that worked example to the format doc.

### unc-rancher-is-the-richest-available-ore
- **Severity:** P1
- **Where:** proposal §"Start with `unc-rancher`"; `docs/analysis/2026-08-13-card-drafter-measurement.md` per-field table
- **What:** In the five-repo measurement, uncrancher drafts 4 of 6 slots (line, persona, pain, cuj — only success and guardrail declined), tied for best in the sample alongside ravenous (5/6) and well ahead of jeddnet and jawnomicon (2/6 each). The proposal's stated reason for picking unc-rancher — funded slate item, card doubles as sales copy — is real, but is never reconciled against the fact that it is also the easiest card in the known sample to fill in. The confirm-step cost this goal measures, and the format findings it produces, will both be measured close to the drafter's best case, not against a project like jeddnet or jawnomicon where mk confirming would be looking mostly at honest declines — plausibly the harder and more revealing case before ruling 12 commits the estate to doing this 82 times.
- **Evidence:** `docs/analysis/2026-08-13-card-drafter-measurement.md` per-field table — uncrancher: `D D D D — —`; jeddnet: `— — D — — D`; jawnomicon: `— D — — — D`.
- **Suggestion:** Confirm a second, thin card (jeddnet or jawnomicon) in the same pass and report both costs, or state explicitly in the brainstorm ruling that the measured confirm-cost is a best-case figure with a named correction expected for thin cards.

## Verdict
`card-check.py`'s exit-0 contract is weaker than every downstream consumer treats it as being: a card can reach `confirmed` with zero real content and no attester verification, and even a diligently confirmed card is never checked against the prose body that is its actual public use. Fix the two P0s before unc-rancher's card gates a plan or raises money; resolve the P1s (score compression, the cuj evidence gap, and the easy-ore sample) before ruling 12 generalizes this confirm step to 82 projects.
