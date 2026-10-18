---
artifact_type: assessment
date: 2026-09-26
bead: none
verdict: inspire-only
decision_status: recommended
scope: brsbl/bb-plugins thread-organizer as prior art for Home (one place in Aleph)
---

# Thread Organizer as prior art for Home

## Recommendation

**Inspire-only for code.** Its `package.json` says `UNLICENSED`, so its
code cannot be copied.

It is worth more to us than
[bb-plugin-command-center](assess-bb-plugin-command-center.md), for two
reasons:
- It is in the reviewed bb-community catalog.
- It shows three SDK primitives that answer parts of our design better than
  what we had planned:
  - agent instructions supplied by a plugin;
  - an approval card inside a thread;
  - a queued, retractable message to a thread.

Using it as the operator, alongside Home, is a separate choice. It does not conflict
with Home, and installing it needs mk.

## What it is

- **Source:** <https://github.com/brsbl/bb-plugins/tree/HEAD/plugins/thread-organizer>,
  read at commit `d882c56` (2026-09-26). It is version 0.1.x, with 4
  installs, and was published on 2026-09-18.
- **Stack:** a bb plugin (`server.ts` at 1.5k lines, `core.ts` and
  `sidebar-controller.ts` at about 580 each). It keeps its state in plugin
  KV and has no external service.
- **Sidebar sections become workflow stages:** Inbox, then stages the user
  names. The defaults are Planning, Spec Review, Building, Testing / Deploy,
  Handoff and On Hold.
- **Inbox:**
  - Idle unread threads collect in Inbox.
  - Starting work again returns a thread to its remembered stage.
  - Dragging a thread out clears it from Inbox without starting a turn.
- **Agents move their own threads** with `bb organizer phase <key>`, guided
  by a skill plus the live stage table. The plugin never classifies a thread
  itself. A rule that needs the user's intent, such as Handoff, cannot be
  inferred.
- **Entry prompts:**
  - When a thread lands in a stage, it receives that stage's prompt.
  - A prompt fires once per landing.
  - A thread can get the same stage's prompt at most once every 10 minutes,
    and at most 3 entry prompts in 30 minutes.
  - A queued prompt is retracted if the thread moves on before it is sent.
- **Configuration changes from the CLI** ask for approval inside the thread,
  showing the exact text to be stored. It says itself that this "is not an
  authorization boundary".
- **Scope:** only visible root threads. Child threads, forks and
  automations are left alone.

## What it tells us about our design

1. **The feed could be instructions, not a per-turn hook (open question 5).**
   - `bb.agents.configure(({thread, origin}) => ({instructions, skills}))`
     supplies instructions whenever a session starts or resumes.
   - Project rulings could arrive that way, from the service's cache. That
     means no hook on every turn and no latency question.
   - The gap: a ruling made in the middle of a session reaches other
     threads only at their next start or resume.
     - A thread's own answers already arrive: needs-context wakes it, and a
       command's outcome can be sent to it.
     - So a per-turn hook would only be needed if the gap proves costly
       in the trial.
   - This fits decision 17's "label and outcome only", and it puts the
     feed in Home's plugin rather than in Clavain's lane (ARCH-5).
2. **Needs-context delivery already exists.**
   - `bb.sdk.threads.send({mode: "queue-if-active"})` starts an idle thread
     or queues the message behind a running turn. It returns a queued
     message id.
   - `threads.queuedMessages.delete` retracts that message.
   - This is how a stale pick is withdrawn before it is sent (decision 14),
     and how batched answers per thread can be combined (decision 13).
   - It does not answer decision 13's open check: `send` always wakes the
     thread, so leaving a note without a wake is still unverified.
3. **An in-thread card is native.**
   - `bb.ui.requestInput({threadId, rendererId, payload, timeoutMs})`
     renders a custom card in a thread and waits for the answer.
   - This could replace the "ask in chat" fallback for when the tracker is
     down (decision 21), because the question still gets a card with
     options. It could also show an owed decision in the asking thread
     next to the rail.
   - The same limit applies to us as to them: whoever runs as the operator account can answer
     it. This matches the same-user risk mk accepted.
4. **The filing helper can be a plugin CLI.** `bb.cli.register` gives
   `bb home ask …` inside any thread, with the thread's id known. There is no
   separate binary to install on PATH, and filing from outside a thread is
   refused. They refuse the same case.
5. **Guards worth copying.** Their protections against prompt loops match
   our rule that a pick runs once:
   - once per landing;
   - a cooldown per thread and stage;
   - a window cap;
   - a retry at most every 30 s for up to a day;
   - a revision on every config save.

## Against Home

| Topic | Thread Organizer | Home |
|---|---|---|
| **Unit of attention** | A thread: unread and idle means Inbox. | A decision: owed means the rail. |
| **Scope** | Sidebar sections. | The estate: about 98 projects, from hub-tracker beads. |
| **Agent action** | Moves its own thread between stages. | Files a decision with options. |
| **Follow-through** | An entry prompt when a thread lands in a stage. | A continuation when mk picks an option. |
| **Record** | KV state; no history. | A signed ruling file and the feed. |

The two overlap only at "what needs me". Inbox answers "which threads
have something new to read". The rail answers "what have I been asked to
decide". Both can run together: an agent that files a decision and ends its
turn would also land in Inbox. That doubles the signal, but it does not
conflict.

## Leave

- **Stages that agents classify.** Stage tracking is Coldwine's territory,
  and Home v1 does not carry it.
- **Rewriting sidebar sections.** Home is a tab plus badges, and it makes no
  sidebar changes (decision 7).

The code was read as untrusted; none of it was run or installed.
