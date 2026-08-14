# Contradiction adjudication — round 3

## Findings Index
- [P1] f-025-holds-f-006-refuted — R3 never forces a success→guardrail origination chain; declined is permanently valid and unordered (§product-card-format.md Shape / card-check.py R3)
- [P2] real-blocker-is-two-decline-stubs-not-origination — uncrancher's actual gap to confirmable is writing two short decline stubs, not "originating a metric from nothing" (§card-check.py R1/validate())

## Findings

### f-025-holds-f-006-refuted
- Severity: P1
- Where: `Sylveste/apps/Autarch/docs/reference/product-card-format.md` § Card status (git show 38715de); `dotfiles/common/.local/bin/card-check.py:172-179` (declined branch), `:198-209` (R3 ratified)
- What: f-006's claim — that confirming uncrancher's card forces a sequential, one-sitting chain of "originate success from nothing, then originate a dependent guardrail" — does not hold against either the code or the format doc. f-025's refutation is correct on all three of its sub-claims, independently verified:
  1. **R3 only blocks on `drafted`.** `card-check.py:198-205` raises `CardError` only when a field's state is `"drafted"` at confirm time; `declined` never appears in that list. A field left `declined` satisfies R3 permanently.
  2. **The format doc says so in plain language.** `product-card-format.md` § Card status states outright: *"A card may be confirmed with `success` and `guardrail` declined. That is mk saying, on the record, 'I do not yet know how I would tell if this worked' — which is true of most of the estate and is worth saying out loud."* This is not an inference — it's the spec's own worked-out position, stated immediately after defining R3.
  3. **`needs` is unenforced free text.** The worked example's `guardrail.needs: "Resolution of success first"` reads as a dependency, but `validate()`'s declined branch (`:172-179`) only checks that `reason` and `needs` are each non-empty strings — it never parses or cross-references their content. Nothing in `card-check.py` reads one field's `needs` value to gate another field's state. A card with `guardrail: confirmed` beside `success: declined` (or vice versa) passes validation exactly as f-025 states.
  4. **No session semantics.** `validate()` re-reads whatever's on disk at each invocation; there is no notion of "the same sitting." Fields can be declined weeks apart and `status: confirmed` set whenever the drafted-count reaches zero, in any order, across any number of edits.
- Evidence: `card-check.py:172-179`, `:198-209`; `product-card-format.md` § Card status (quoted above, confirmed via `git show 38715de:docs/reference/product-card-format.md`).
- Suggestion: n/a — this is the adjudication, not a design defect. If the goal proposal or brainstorm cites f-006's "sequential chain" framing anywhere as a cost estimate for the confirm step, correct it: the mechanical floor for confirming uncrancher's card (once the two missing fields are stubbed, see next finding) is two independent, unordered declines, not an origination chain.

### real-blocker-is-two-decline-stubs-not-origination
- Severity: P2
- Where: `dotfiles/common/.local/bin/card-check.py:131-137` (missing-field hard error), `:172-179` (declined branch requires only `reason` + `needs`); proposal measurement table (uncrancher row: success `—`, guardrail `—`)
- What: A settled fact this round already established that uncrancher's success/guardrail were *never drafted* (not even declined) and that this triggers the "fields missing entirely" hard error, producing INVALID rather than PROVISIONAL on first run. What the f-006/f-025 dispute clarifies on top of that: the actual unblocking action is not "originate a metric from nothing" (f-006's framing) — it is writing two short, honest `declined` stubs (a `reason` sentence and a `needs` sentence each, per the worked example's own template) for fields the measurement already shows have no basis in the repo. `declined` requires no `evidence`, no citation, and no real value (`R1` only binds `drafted`). This is materially cheaper than f-006's "origination chain" framing suggests, and it is exactly what the format doc's own uncrancher worked example already models — the goal's confirm step for these two fields is transcription of a known non-answer, not invention.
- Evidence: `card-check.py:131-137`, `:172-179`; product-card-format.md worked YAML example (`success.state: declined`, `guardrail.state: declined`); proposal measurement table row `uncrancher | D D D D — —`.
- Suggestion: If the proposal or brainstorm estimates confirm-step cost/time, model it as "two decline stubs + N confirms" rather than "resolve success, then resolve a dependent guardrail" — the latter overstates both the effort and the required ordering.

## Verdict
f-025 holds and f-006 is refuted: nothing in `card-check.py` or `product-card-format.md` forces sequential same-sitting origination of success then guardrail — declined is a permanently valid, independently-settable terminal state for both fields, and the format doc says exactly that. f-006's error is treating the worked example's narrative `needs` text as an enforced schema dependency when it is unread drafter rationale. The one live cost the dispute surfaces is smaller than either finding stated: uncrancher's real gap is two short, unordered decline stubs, not an origination chain.
