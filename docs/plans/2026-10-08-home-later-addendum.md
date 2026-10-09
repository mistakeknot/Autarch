# Addendum: "Later" on every open Home card (beads mk-8741, mk-yjp7)

Status: plan, before any code. Same plugin (`integrations/bb-plugin-autarch`) and branch family as the Home UX pass.

## What is wanted
One tap puts any open card in a **Later** group below the active cards. The card stays open and unruled. It is not in the "Waiting on you" count (it is shown beside it, labelled). **Move back** undoes it. Optionally the filer can file a card as Later.

## Storage: no schema change, no migration
Home already keeps vizier holds in one `settings_kv` value (`holds`, `service.ts` hold/unhold/readHolds), keyed by task id, with no table or column. Later uses the same shape: a `later` settings_kv value, `{ [task_id]: { at, by } }`, read fail-open (a malformed entry is not a Later). So q712 B (additive schema, backup first, separate migration review) is **not triggered**. If review disagrees and wants a column, that becomes a separate, reviewed migration PR; I would stop and escalate.

## Semantics (mirrors Your-move Later, `moveselect.ts` `skipped_at` -> `later`)
- **Later is mk's own deferral, hold is the vizier's.** A hold refuses `rule` (409) and clears on a new script. Later never refuses a pick: mk can still rule a Later card directly, and ruling it clears its Later entry. Later and hold are independent; a held card is shown as held, whether or not it is also Later.
- A Later card is excluded from `waiting.total` and reported as `later: N` ("N for later") next to the existing "updates to read" and "on hold" figures. `WAITING_DEFINITION` gains the sentence. Badge, tab, strip and `bb home stats` stay equal because they all read `waitingNow`.
- A Later card's Your-move rows follow the card: a card set to Later leaves the moves count too (no double counting; the move stays visible in the Later group).
- New RPCs, additive: `later { task_id }` and `unlater { task_id }`, both idempotent, both publishing `home-queue-changed`. `bb home later|unlater <card>` CLI mirrors `hold|unhold`.
- UI: a **Later** button on every open card (`min-h-11 sm:min-h-0`), a "Later (N)" group below "Needs you now" and "Your move", each row with **Move back**. Order inside the group: when it was set to Later, oldest first.
- **Filer flag (e)**: a card body may carry a fenced ```` ```home-later ```` block (like `home-move`); the poll sets Later once on first sight of the card, never again after a Move back (the entry is recorded as "filer-set" so a poll cannot re-apply it). Small, optional, and the first thing to drop if the PR grows.

## Tests first
`later.test.ts` (service: later/unlater idempotent, ruling clears it, held+later, malformed value fails open, no schema diff: `user_version` and table list unchanged); `waiting.test.ts` (Later excluded from total, shown as `later`, moves on a Later card excluded, equal everywhere); UI (button on every open card, group order, Move back, tap-target class); filer flag (applied once, not after Move back).

## PR split
Recommendation: **two PRs.** PR 1 = C1-C4 (the Home UX pass, now at review round 7); PR 2 = Later, on its own branch off PR 1's head (it touches `waiting.ts`, `asks.tsx` and `app.tsx`, which PR 1 rewrites), reviewed cross-lab separately. Doing Later in PR 1 would restart a review that has been converging for seven rounds. I will not start Later code until PR 1 is open, unless the coordinator says otherwise.

## Not doing
Reordering inside Later by hand; snooze-until-a-time; Later for catch-up reading (it already has Mark seen).
