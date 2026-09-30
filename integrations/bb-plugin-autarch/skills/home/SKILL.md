---
name: home
description: File asks with mk and report blocker progress with the `bb home` CLI. Use when you need a decision from mk, when you own a machine blocker, or when you want to see what is waiting for a ruling.
---

# Home

The Autarch plugin's Home page collects what is waiting for mk. Agents reach it with
`bb home`; run `bb home --help` for the exact options of each command.

## Commands

| Command | Effect |
| --- | --- |
| `bb home ask --request <json>` | File an ask (decide, steps or machine) as a JSON object of at most 16 KiB. |
| `bb home get <id>` | Look up a filed ask by request id or decision id. |
| `bb home list` | List the asks waiting for a ruling. |
| `bb home feed` | The recent-rulings feed a thread sees. |
| `bb home stats` | Picks, delegation and filing counts over a window. |
| `bb home progress <id>` | Report progress on a machine blocker you own. |
| `bb home resolve <id>` | Resolve a machine blocker. |
| `bb home withdraw <id>` | Withdraw a machine blocker you filed. |

`bb home rule` and `bb home note` are for the vizier thread only.

## Rules

- Look up an ask with `bb home get` before filing a duplicate.
- Never guess an id; take it from `bb home list` or from the filing output.
- Change asks only through `bb home`. Do not edit the plugin's `data.db`.
- A non-zero exit of 2 means the request was malformed: fix it, do not retry unchanged.
