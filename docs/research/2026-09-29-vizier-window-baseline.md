---
artifact_type: research
bead: mk-schu
---

# Vizier window: success-measure baseline (OQ6)

Taken 2026-09-29 from the log of the current vizier thread (thr_dcuzkvjb39),
11:58 to 15:16 local, before anything is built. The archived predecessor
(thr_33wnq9k293) holds older history and was not read.

## What was counted

Turns in the thread that arrived as `client/turn/requested` with
`initiator: user`, excluding the spawn turn. The vizier thread is the only
place mk answers asks today, so every one of these came through chat.
Gap = time from the vizier's previous turn ending to that turn arriving.

## Numbers

| Measure | Baseline |
|---|---|
| Turns from mk or a script report, in 3h18m | 40 |
| Answered from the rail (no rail exists yet) | 0 of 40 |
| Median gap after the vizier's last turn | 1.6 min |
| Longest gap | 15.1 min |
| Gaps of 9 min or more | 5 |
| Vizier turns that ended with a question or an escalation | not counted separately (see limits) |

## Limits

- The gap is a poor proxy for time-to-pick. It includes script reports that
  arrive by `bb thread tell` (about a third of the 40), and mk's own
  conversation, which is not a pick. The vizier's turns rarely carry a
  machine-readable ask, so "time to pick" cannot be computed until decisions
  are structured (decision 25).
- Short gaps mean mk was already in the thread. The cost the window targets is
  the scroll to find what is owed, which this log cannot show.
- 3h18m on one day, during an active release, is one sample.

## Consequence for the measure

Use the baseline for one thing only: the count answered from the rail should
rise from 0. Time-to-pick should be measured after the build, from the pick
record's `asked_at` and `picked_at`, so it needs no log reconstruction. To
compare against today, record "scrolled to find it" by asking mk once, after
two weeks on the rail. The failure signals stay as in OQ6: override rate and
time from ruling to override, both zero today because delegated rulings are
not yet listed.
