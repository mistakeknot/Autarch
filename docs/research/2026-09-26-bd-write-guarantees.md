---
artifact_type: research
bead: mk-okek
stage: design
---
# What `bd` guarantees for Home's writes

Checked on 2026-09-26 against `bd` 1.1.2 (`20e493e56`). This answers review-3 findings C-1 and
C-2 on the [Home plan](../plans/2026-09-26-home-serve-and-decisions-plan.md), and backs its
Task 1.2 step 0.

**Setup.** A throwaway database made with `bd init --prefix mk` in `/tmp`, using the embedded
Dolt engine. The hub runs an external Dolt server on a local port, which was not touched. The
create and update code is shared between the two modes, but the kill timings below are specific
to the embedded engine.

## 1. A duplicate `--id` overwrites; it is not rejected (C-1 confirmed)

| Step | Result |
|---|---|
| `bd create --id mk-htest00001 …` | exit 0 |
| Same command again | exit 0, no error, no warning |
| Record a pick in metadata, add `home:pending`, `bd close` | closed, pick present |
| Same `create` again, as a delayed first attempt would be | **exit 0. The bead is open again, `close_reason` is null, the title is replaced, and the metadata is back to the filing version: the pick is gone.** Labels are merged, so `home:pending` survives. |

So a deterministic `--id` does not make filing idempotent. A late duplicate create erases a
recorded pick and reopens the decision.

## 2. One `bd update` with metadata and labels is not atomic (C-2 confirmed)

`bd update <id> --metadata @pick.json --add-label home:pending` takes about 350 ms. The test
sent it SIGKILL after a random delay of 160–380 ms, 200 times, and read the bead back after
each kill.

- 7 kills (at 161, 169, 174, 193, 200, 231 and 263 ms) left the pick **in metadata with no
  `home:pending` label**. That state persisted: it was read back by a fresh `bd show`.
- No kill left the label without the metadata.
- The other results were either all written or nothing written.

So metadata commits before labels, and a crash between the two is real and durable.

## 3. There is no conditional write (C-3)

`bd update --help` has no compare-and-set, expected-version or if-match flag. The only
other write path is `bd sql`, which is raw SQL against the database.

## 4. Labels are not validated

`bd` accepts any label without error:
- an empty label is silently skipped;
- `a,b` becomes two labels;
- a newline truncates the label;
- a 400-character label and a label with a space are stored as given.

Home must build labels itself from validated parts.

## What follows for the plan

The hub bead cannot be the only record of a pick if picks must survive retries and crashes.
Neither create-if-absent nor an all-or-nothing update is available through `bd`. The options,
for mk to rule on:
- a local SQLite log owned by `autarch serve` that records picks first, with beads as a copy
  that recovery can repair;
- conditional SQL through `bd sql`;
- a change to `bd` upstream.
