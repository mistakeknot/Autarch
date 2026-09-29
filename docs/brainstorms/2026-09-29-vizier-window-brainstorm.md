---
artifact_type: brainstorm
bead: none
stage: discover
---

# The vizier window

## What We're Building

mk reads one thread, the vizier (`Masaq' | vizier`). Today it is one bb
thread among many, and it talks in chat prose. mk wants a surface they can
pull up quickly or put full screen.

**The vizier becomes Home.** The Home surface from
[one place in Aleph](../onepagers/2026-09-24-one-place-in-aleph.md) gains the
vizier's conversation. One place shows:

- **the map** (Autarch's estate map), which handles "what's running and is
  anything stuck";
- **the rail**, the decisions mk owes, picked in place;
- **catch-up**, "since you last looked";
- **the vizier's conversation**, for direction and discussion.

**A summoned overlay.** A global hotkey brings up a small window over
whatever mk is doing. It holds the owed decisions (pick in place) and one
input line to the vizier. It has no map and no history. It is built for
30-second check-ins.

The vizier serves three jobs: catch up, answer decisions, direct and
discuss (mk, 2026-09-29). Watching live work belongs to the map.

## Why This Approach

The Home plan (revision 3) already specifies the rail, picks, wakes and the
feed. A separate vizier app would split attention again, which is the leak
Home exists to close. Folding the vizier into Home keeps one data model:
the vizier's escalations are decision cards on the rail, not paragraphs in
chat. Its "still waiting on you" footers are the symptom this removes.

## Key Decisions

1. **The vizier is Home**, with the Autarch map. It is also available as a
   summoned overlay (mk, 2026-09-29).
2. **Catch-up is facts, then prose.** A factual list comes first, one line
   each and linked: rulings made, decisions opened and closed, runs finished
   or failed. A short vizier note follows only when something needs
   interpretation. This extends the plan's rule that the feed carries no
   agent prose: the facts keep that rule, and the note is marked as the
   vizier's.
3. **The overlay shows decisions and an input line**, nothing else.
4. **Delegated rulings are listed and can be overridden.** When the vizier
   rules in mk's place, it is recorded as a pick by the vizier. It appears
   in catch-up as "vizier ruled X on Y", with an Override button that
   reopens it as a decision for mk. Reversible actions go ahead at once.
   There is no veto window.
5. **Approach: A then B** (mk, 2026-09-29).
   - A: revise the Home plan to include the vizier.
   - B: a popped-out vizier window and a fixed-format catch-up in the
     vizier's messages, as interim pieces.

   mk ordered A first. Read literally, the interim pieces follow the
   revision rather than precede it.
6. **Out of scope:** a standalone vizier app, and live-work watching
   outside the map.

### From the review (fd-user-product, 2026-09-29)

7. **Override files a superseding decision.** In the plan a pick is
   written once, and a second pick returns 409. So an override files a new
   decision bead marked `supersedes` the old one (using the plan's
   `close-superseded`). The old pick stays immutable.
8. **Delegation has a boundary.** A pick records `by: vizier`. Only
   options the asker marks `reversible` when filing can be delegated.
   Anything irreversible, and Mycroft's out-of-allowlist dispatches, stay
   owed to mk. Overriding a ruling that was already acted on sends a second
   wake: "the prior ruling is void; undo or continue".
9. **Catch-up and the feed are separate surfaces.** The feed stays
   agent-facing and has no prose. Catch-up is for mk. Every vizier note
   cites the ids of the facts it interprets.
10. **Catch-up is ranked and collapsed.** The order is failures, then owed
    decisions, then delegated rulings. Routine items collapse per project,
    and there is a "mark all seen" action.
11. **The read marker is per account.** It advances only when an item has
    actually been shown on screen. Delegated rulings stay pinned until mk
    has seen them.
12. **The overlay also shows** a count strip (undeliverable wakes,
    delegated rulings not yet seen) and the vizier's last reply to the
    input line.

## Open Questions

1. **Can a bb plugin panel show a thread's chat** (the vizier's composer and
   messages)? If not, the vizier column needs an Aleph core change, or a
   plugin chat that drives the thread through `bb thread tell`. This could
   invalidate approach A, so a spike answers it first, before the plan is
   revised.
2. **Where the overlay lives.** A global hotkey needs Aleph desktop (native).
   Does Aleph desktop support a second, frameless, always-on-top window? Is
   the web build limited to an in-page overlay?
3. **Override semantics.** Overriding a ruling the asking thread has already
   acted on reopens the decision. Must the asker undo the action, or just
   take the new ruling going forward? It probably varies by the option's
   continuation. The plan's continuation kinds should say.
4. **"Since you last looked" needs a read marker.** It could be per device
   or per account. Does opening the overlay count as looking?
5. **Order check on decision 5:** confirm that "A then B" means the Home
   revision comes before the interim pop-out and catch-up format.
6. **Success measure.** A candidate: mk stops needing to scroll the vizier
   chat to find what's owed. Measured as time-to-pick and the count of
   decisions answered from the rail, not from chat, in line with the trial's
   decision 21. It needs a baseline taken from today's chat before the build.
   It also needs failure signals: the override rate, and the time from a
   ruling to its override.

## Prior Art

- Home plan revision 3 (`docs/plans/2026-09-26-home-serve-and-decisions-plan.md`)
  specifies the rail, picks, wakes, the feed and a `navPanel` Home panel with
  a badge.
- [bb-plugin-command-center](../research/assess-bb-plugin-command-center.md)
  (inspire-only) has a "Needs you" lane and a badge, and answers by chat
  reply. Our rail answers by pick.
- No epic covers a vizier surface. This is a revision to the Home epic, not
  a new epic, so no one-pager was written.
