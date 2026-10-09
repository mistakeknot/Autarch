---
name: home
description: Ask mk for a decision by filing a needs-mk card with `autarch needs-mk file`, and read what is waiting with the `bb home` CLI. Use when you need a decision from mk, or when you want to see what is waiting for a ruling.
---

# Home

The Autarch plugin's Home page collects what is waiting for mk. mk's questions are cards in
tasks, labelled `needs-mk`. You file one; Home shows it, records mk's pick, and wakes the asking
thread with the ruling.

## Asking mk

File a card with `autarch needs-mk file` (run it with `--help` for the exact options). Do not use
`bb home ask`: it is retired and exits 2 with "moved". Give the question, the options and a
`Blocks:` line naming what is waiting. Options default to needs-context. Give an option an
`instruction` when you know what you would do if mk picks it, and mark it reversible only if
undoing it is cheap and local; never for a push, merge, deploy or release.

## Reading

| Command | Effect |
| --- | --- |
| `bb home get --card <task id>` | The card, its current generation and its state. |
| `bb home get --request <key>` | Look up a filed card by its request key (pre-card asks by their request id). |
| `bb home list` | List the asks waiting for a ruling, plus cards Home shows flagged (`display_only: true` with a `display_reason`, e.g. a missing Request line); `--pull mycroft` narrows to what Mycroft pulls and omits flagged cards. |
| `bb home feed` | The recent-rulings feed a thread sees. |
| `bb home stats` | Picks, delegation and filing counts over a window, plus `traceability`: the share of threads that ended in the window whose final output names a tasks key (a real tracker prefix, e.g. `PROJ-24`) or has a line starting `no-card: <reason>`, split into root and child threads; and the open tasks across every tracker project with no update in 14+ days. A failed read shows as `error` on that part. |

`bb home rule`, `bb home note`, `bb home bind` and `bb home unbind` are for the vizier thread only.

Home knows the vizier as the stored vizier thread (mk sets it in Settings). When that is unset, archived
or gone, Home adopts the single pinned, unarchived thread titled "Masaq' | vizier..." instead (the rule
`vizier-tell` uses), records it, and keeps delegated rulings suspended until mk has seen the change. With
no match or several, the vizier-only commands refuse. At a handoff the current vizier runs
`bb home handoff <thr_id>` to name its successor (a live thread other than itself); that is recorded and
does not suspend delegation.

## Vizier: file, update, close

An ask is a tasks card labelled `needs-mk`, so the vizier works one with three commands. Home picks
up every change on its next queue refresh.

| To | Run | Notes |
| --- | --- | --- |
| File | `autarch needs-mk file --project <tasks project> --title <t> --ask-file <json> [--blocks bead:ID] [--request <key>]` | Run from the vizier thread (`$BB_THREAD_ID` required). Re-running the same `--request` is safe. A project with no confirmed Home binding files anyway and prints a `warning:` line on stderr; its card shows flagged until `bb home bind` (vizier) or mk's Bind. |
| Update | `bb tasks comment <KEY> --body <text>`; `bb tasks update <KEY> --title/--description-file/--priority` | Comment to add facts or answer mk. Do not remove the `needs-mk` label: that drops the card from Home. |
| Close | `bb tasks update <KEY> --status done` (or `canceled` when the ask no longer applies) | Home lists only backlog, todo, in_progress and in_review cards, so a closed card leaves the queue. Add a `bb tasks comment` first saying why. Close an answered card only after the ruling reached the asking thread. |

Exit codes of `file`: 2 usage or refused, 3 Home or tasks unavailable (nothing created), 4 card
created but the routing comment failed (re-run the same command), 5 already ruled. Look an ask up
with `bb home get --request <key>` before filing a duplicate. `bb home progress|resolve|withdraw`
do not work on cards (exit 2, "card asks close through tasks").

## For asks filed before cards

Asks filed before cards existed drain in place. Only these commands touch them, and they refuse a
card with exit 2, "card asks close through tasks":

| Command | Effect |
| --- | --- |
| `bb home progress <id>` | Report progress on a machine blocker you own. |
| `bb home resolve <id>` | Resolve a machine blocker. |
| `bb home withdraw <id>` | Withdraw a machine blocker you filed. |

## Rules

- Look up an ask with `bb home get` before filing a duplicate.
- Never guess an id; take it from `bb home list` or from the filing output.
- Change asks only through the card filer and `bb home`. Do not edit the plugin's `data.db`.
- A non-zero exit of 2 means the request was malformed or the command moved: fix it, do not retry unchanged.
