---
artifact_type: assessment
date: 2026-09-25
bead: none
verdict: inspire-only
decision_status: recommended
scope: dilipgv/bb-plugin-command-center as prior art for Home (one place in Aleph)
---

# bb-plugin-command-center as prior art for Home

## Recommendation

**Inspire-only.** Borrow three ideas and no code:
- the content-script sidebar badge;
- the shape of the agent `ask` command;
- the stall rule.

The repo has no license, so its code cannot be copied. Its central design
choice is also the opposite of ours: a question is a notification, and the
answer is a chat reply.

## What it is

- **Source:** <https://github.com/dilipgv/bb-plugin-command-center>, read
  at commit `4829214`. It had 2 stars, was last updated 2026-09-25, and has
  no license.
- **Stack:** a bb plugin (`server.ts` at 5.5k lines, `app.tsx` at 1.6k)
  with its own SQLite. It requires bb >= 0.37 and SDK >= 0.4.34.
- **The board:** Queue → In progress → In review → Done, plus a derived
  **Needs you** lane. Only the user moves a card to Done.
- **Agents ask with a CLI:**
  `bb command-center ask|review --task … --question … --asked-by … [--wait]`.
  A bundled `user-inbox` skill tells agents to use it instead of ending a
  turn with "let me know".
- **Chief:** a global Chief, a project chief per project, and task
  architects beneath them, for routing queued work. Each project opts in;
  otherwise work goes to a single direct worker thread.
- **Other features:**
  - a sidebar count badge;
  - a reading view for each card;
  - archiving that cascades to the card's worker threads;
  - macOS notifications via `osascript`;
  - voice capture.

## Against our design

| Topic | Command Center | Home (brainstorm) |
|---|---|---|
| **A question** | Notification only. No options. The user answers in the asking thread's chat, which wakes the asker. | A decision bead with options, each carrying a continuation (decisions 13 and 14). A pick can run a frozen command without waking the asker. |
| **Record of the answer** | None durable; the card is dismissed. | A signed ruling file plus a feed (decisions 16 and 17). |
| **State** | Plugin-local SQLite. | Hub-tracker beads across the estate (decision 3). |
| **Dispatch** | Chief hierarchy, opt-in per project. | Mycroft T2/T3, after the trial (build step 5). |
| **Badge** | Content script (`nav-badge.ts`). | Build step 2, and only without fork patches. |

**The answer model is a deliberate counter-choice.** Its README argues that
acting on a mishearing is worse than one extra tap. Their model is cheaper
to build and to learn. Ours pays for continuations so that most answers
don't wake a thread. That bet has to show up in the trial's
decisions-per-week and time-to-pick counts (decision 21). If most
continuations turn out to be needs-context, their model was the right one.

## Borrow

1. **The badge without a fork patch.**
   - `navPanel` has no badge option, so `nav-badge.ts` is page code that
     finds the plugin's sidebar row by its label and appends a count span.
   - It re-adds the span every 8 seconds because React re-renders wipe it.
   - It strips its own trailing digits before matching the label.
   - The count comes from one `attention` RPC that runs a single `COUNT`.
   - This fits build step 2's "no fork patches" condition.
   - It is fragile because it matches on visible text. First check whether
     the current SDK has gained a badge option.
2. **The filing helper's shape.** Use one agent-facing command with
   `--asked-by` and `--wait`, plus a skill that tells agents to file instead
   of ending a turn with a question. Our helper adds options and
   continuation kinds on top.
3. **The stall rule** (`lib/stall.ts`).
   - Work in progress is stalled when its last activity is older than a
     threshold. Last activity is the newest agent comment or task update,
     falling back to the card's creation time.
   - It is a pure function, so it is testable. It is a candidate source of
     catches for the map.

## Leave

- **Chief.** It overlaps Mycroft's dispatch. Its per-project opt-in with a
  direct single worker otherwise is worth recalling at T2.
- **Plugin-local SQLite.** Our decisions span the estate.
- **`osascript` notifications.** They only work on macOS, and zklw is
  Linux.
- **Voice.**

The code was read as untrusted; none of it was run or installed. Trying the
plugin would need mk to approve an install.
