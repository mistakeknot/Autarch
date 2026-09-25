---
artifact_type: onepager
distills: docs/brainstorms/2026-09-24-one-place-in-aleph-brainstorm.md
---

# One place in Aleph

**Thesis.** mk directs about 98 projects through agents, but attention leaks
out of Aleph. The worst leaks are the estate-wide picture and decisions
buried in chat. The fix is one Home in Aleph, where attention comes in and
intention goes out. This is not a merger: Autarch's tools come in only where
they serve attention.

**How it works.**
- **An Aleph plugin, not a separate app.** The plugin provides a Home tab,
  sidebar badges and a decisions inbox. Its host part runs `autarch serve`:
  one Go service that consolidates the Bigend daemon, Gurgeh, Signals and
  MCP, and has no UI of its own.
- **Home** has two parts:
  - the estate map;
  - the rail: decisions owed and Mycroft's proposals.
- **Decisions are owed, then ruled.**
  - An owed decision is a hub-tracker bead.
  - Answering closes the bead and writes a stamped ruling file.
  - A companion proposes beads for prose decisions that were never filed. It
    never shows a guess as real.
- **Continuations.** Each option on a decision bead carries one of four
  things:
  - a command, which runs when mk picks the option. It is shown exactly and
    frozen by hash, and costs zero tokens.
  - a brief, which runs in a fresh, small thread.
  - needs-context, which wakes the asker.
  - ruling-only, which just records the ruling.

  Answers are batched per thread. A pick re-checks the continuation first
  and turns into a re-ask if the continuation has gone stale. A failed
  continuation reopens as a decision owed.
- **Fire-and-forget decisions and a feed.** Continuations own the
  follow-through. A turn-start hook injects rulings relevant to the project:
  project rulings and the thread's own answers, one line each.
- **Rulings are signed files.** Each one reuses the card's ratification
  block and is signed with Home's own key, whose public half is in the
  Uqbar's `allowed_signers`. Unsigned files render as proposed. Rulings
  about one project go in that project's `docs/decisions/`. Estate-wide
  rulings go in an **Uqbar**, a private repo convention any Aleph user can
  set up.
- **The index is Lattice,** grown to meet the Ultan thesis. CanonGraph stays
  read-only until Lattice reaches parity, then it retires.
- **Owners:**
  - Aleph core owns the mechanics.
  - Clavain owns policy.
  - Mycroft owns initiative.
  - Autarch owns attention.
- **Build order:**
  1. Decisions.
  2. Badges.
  3. Lattice grown toward the Ultan thesis.
  4. The map.
  5. Mycroft's proposals.
  6. The companion.

**Lineage.** This widens the ruled
[attention map](2026-09-22-autarch-attention-map.md), and it corrects the
[options doc](2026-09-24-autarch-in-aleph-options.md). Mycroft does not
enforce Aleph's roadmap, because Aleph core already does (mk-a4o0.1–.4,
mk-rpnv.12). It also follows Aleph's mission: a page renders state for free,
where a chat spends a wake on every look.

**Refusals.**
- No merger of Autarch into Aleph.
- No port of bb.
- No world truth in Autarch.
- No guessed decision shown as real.
- No companion as the only surface.
- No fork patches for the sidebar.

**Top open calls.**
1. Keeping the feed hook fast: a per-project cache invalidated on bead
   close or an Uqbar commit.
2. Whether a decisions rail works across all 98 projects without any
   focus lens.

**Status.** Discover. Refined with mk on 2026-09-24 and 25 (20 decisions).
CUJs autarch-07 (decide and continue) and 09 (the net) are validated.
- v1 is decisions and the estate picture. Rig health and PRs come
  after the trial.
- The trial passes if Home catches things, under the existing kill rule.
- Focus is cut from v1 (2026-09-25). Its CUJ, autarch-08, is parked until
  Mycroft dispatches on its own (T2).
- A flux-drive review is held under the quota note.
