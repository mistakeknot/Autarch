---
artifact_type: brainstorm
bead: none
stage: discover
---

# One place in Aleph to direct attention, intention and thinking

Builds on: [attention map brainstorm](2026-09-22-autarch-attention-map-brainstorm.md)
and its [one-pager](../onepagers/2026-09-22-autarch-attention-map.md), and on the
[Autarch-in-Aleph options](../onepagers/2026-09-24-autarch-in-aleph-options.md).
It corrects that options doc's A2 in two places: the goal, and Mycroft's role.

## What we're building

A single place inside Aleph where mk directs the estate of about 98
projects. The goal is not to fold Autarch into Aleph (mk, 2026-09-24:
"I'm okay with autarch not folding into aleph, I really just want one place
to direct my attention/intention/cognition").

- **Attention (in).** The two primary leaks are the estate-wide picture and
  decisions owed. The secondary leaks are rig health and jobs, and PRs, CI and
  Sonnerie.
- **Intention (out).** mk answers decisions, sets focus (for example "this week:
  After Them and Aleph; park the rest"), and talks to a companion that turns
  intent into beads, threads or rulings for approval.

**Shape: approach 1, a Home page.** The one place is an **Aleph plugin, not a
separate application**:

- A full-page Home tab with the estate map, the rail and a focus bar.
- The rail holds decisions owed and Mycroft's proposals.
- The plugin's host part starts the **Autarch service**: the existing Go
  engine with no UI of its own.
- The service builds the estate picture and runs Mycroft's proposals.
- The Autarch TUI remains optional.
- The plugin is developed in the Autarch repo and installed into Aleph for
  the trial. Whether to bundle it into Aleph's own `plugins/` is decided after
  the trial.

**Which Autarch tools come in:** only those that serve attention.

- Bigend, with the catch-up and the estate map, becomes the map.
- Mycroft's proposals, filtered by focus, appear on the rail.
- Coldwine's outcomes appear when you open one project's view.

Gurgeh and Pollard stay as tools that agents call through the CLI and MCP.

## Why this approach

- **It has already been designed and ruled on.** The attention map design,
  after a flux-drive review, chose a bb plugin backed by an independent
  Autarch/Mycroft service. This design widens that plugin from one page to
  mk's home in Aleph.
- **The map costs nothing to look at.** Approach 2, companion first, would
  spend a wake and tokens on every look, which is the waste Aleph's mission
  counts. A page renders state it already has.
- **The fork stays thin.** The work is a plugin plus a Go service. Approach 3,
  folding everything into the sidebar, needed carried patches and could not
  show 98 projects.

## Key decisions

1. **The goal is one place, not a merger** (mk). Autarch's tools come in only
   where they serve attention or intention.
2. **Four owners, one job each:**
   - *Aleph core* owns the mechanics: threads, pool, waits and wakes. This is
     already in progress as mk-a4o0.1–.4 and mk-rpnv.12.
   - *Clavain* owns policy: routing, review and gates.
   - *Mycroft* owns initiative: proposing and claiming work.
   - *Autarch* owns attention: presenting the estate and recording mk's
     rulings.

   This withdraws the options doc's "Mycroft enforces the Aleph roadmap". That
   would have duplicated the work already in progress in Aleph core, and it
   contradicts Autarch's vision ("Clavain owns planning, policy, review and
   execution").
3. **A decision owed is a bead in the hub tracker** (mk accepted the
   recommendation):
   - It records the question, the options, the agent's recommendation, the
     project label and the asking thread's id.
   - Answering it in Home closes the bead and delivers the answer to the
     asking thread, or to that thread's successor if it has rotated.
   - Reasons: decisions have to survive thread rotation (today they survive
     only when a handoff copies them by hand), Autarch owns no world state,
     and the attention map rulings already record approvals with a beads
     claim.
4. **An extraction net that only proposes.** The companion scans thread
   endings for prose decisions that have no bead and offers to file them. It
   never shows a guessed decision as real.
5. **Intention = decide + set focus + talk** (mk chose options 2 and 3; focus cut from v1, decision 19):
   - Focus is a declared record that coordinators and Mycroft read.
   - The companion is the rail's composer: a pinned thread that wakes only
     when mk speaks to it.
6. **Success after a two-week trial** (mk):
   - *(Cut, decision 19.)* *Focus shows up:* after mk sets focus, coordinators' proposals and
     Mycroft's picks follow it, and parked projects stay quiet.
   - *Catches:* Home surfaces things mk would have missed. These are counted
     in the existing caught-something log, which uses the trial plan's kill
     rule.
7. **Build order** (mk: "Home on beads first"):
   1. The decisions inbox and focus, built on beads and stamped files.
   2. Sidebar badges.
   3. Lattice grown to meet the Ultan thesis, as the map's source.
   4. The map.
   5. Mycroft's proposals, filtered by focus.
   6. The companion.

   The badges and a focus filter in the sidebar are borrowed from approach 3.
   They are included only if they fit the plugin API without fork patches;
   otherwise they wait.
8. *(Cut from v1 on 2026-09-25; see decision 19.)* **Focus is a pinned "estate focus" bead in the hub tracker** (mk):
   - It holds focused and parked project lists.
   - Home edits it.
   - A Clavain SessionStart hook injects one line for the current project:
     focused, or parked with "finish claimed work, propose nothing new". It
     stays silent for neutral projects.
   - Mycroft filters its proposals by it.
   - Every change is also written as a stamped ruling file, per Ultan ruling 1
     (files are truth). This part is a recommendation recorded here; mk has
     not objected.
9. **A decision is owed, then ruled.** An owed decision is pending work, so
   it is a bead. When mk answers, the bead closes and a stamped decision
   record is written to a file, which Ultan indexes. This reconciles decision
   beads with Ultan rulings 1 and 2 (`Sylveste/apps/Autarch/docs/research/2026-09-03-ultan-nativity-thesis.md`).
10. **Autarch is one Go application, but it is the engine, not the place**
    (mk asked about "a Go mega-application"):
    - The existing unified `autarch` binary becomes the service.
    - Its four separate servers (the Bigend daemon, the Gurgeh server, the
      Signals server and `autarch-mcp`) consolidate into one `autarch serve`,
      which the plugin starts.
    - It reads the estate index (Lattice); it does not contain one.
    - A separate Go app as the *place* is refused: it would be a second place
      unless it held threads, and holding threads means rebuilding bb.
11. **Grow Lattice to meet the Ultan thesis** (mk, 2026-09-24). Ultan is
    the requirements thesis (2026-09-03). Lattice (`interverse/lattice`,
    remote `mistakeknot/interweave`) is the index that already exists. It
    already follows the finding-aid rule, and the 2026-09-09
    source-and-graph-ownership decision chose it: "Autarch presents, Lattice
    indexes; no estate-wide graph service required". One index, no new build.
    CanonGraph was ruled transitional on 2026-09-01. It stays read-only, its
    sylveste profile is exported into files, and it retires once Lattice
    reaches parity on what the map and the decision lanes need. The
    memory-lanes policy is then updated.
12. **Rulings files live in two places** (mk, 2026-09-24):
    - A ruling about one project goes in that project's `docs/decisions/`.
    - Estate-level rulings (focus, themes and rulings across projects) go in
      an **Uqbar**: a private repo with a documented layout (`focus/`,
      `themes/`, `rulings/`).

    Uqbar is a convention, so any Aleph user can set one up; Home finds it
    through a setting. The name comes from Borges' "Tlön, Uqbar, Orbis
    Tertius", where a country known only through an encyclopedia entry comes
    to overwrite the world. Here the written record is what coordinators
    obey. Creating mk's Uqbar repo is mk's call at planning. (The name
    "canon" was avoided because it collides with CanonGraph during its
    retirement.)
13. **Answers are delivered through continuations on the decision bead**
    (mk, 2026-09-24, choosing the most token-efficient design):
    - **Why:** the cost is not in delivering the answer but in the context
      that wakes up to act on it. Resuming a 150k-token asker after its cache
      has expired re-reads about 150k tokens.
    - **Continuations:** when filing, the asker gives each option a
      continuation of one of three kinds:
      - a **command**, run deterministically by the service within what mk
        approved (zero tokens);
      - a **brief**, run by a fresh, small thread whose only context is the
        bead;
      - **needs-context**, which wakes the original thread (or its
        successor).
    - **Batching:** answers to one thread go out in a single wake.
    - **A note, not a wake:** the asker gets a short note on its next natural
      turn.
    - **To verify in planning:** whether `bb thread queue` can deliver that
      note without waking the thread. If not, that is a small Aleph addition.
14. **Every continuation command runs when mk picks its option** (mk,
    2026-09-24, "everything on pick"). mk's pick is the whole approval.
    - This amends the attention-map ruling "approval weight scales with blast
      radius" for continuations.
    - Two safeguards keep the pick an informed approval, without adding a
      step:
      - the option shows the exact command, not only its label;
      - the command is frozen by hash when the bead is filed, so what was
        shown is what runs.
15. **v1 scope and shipping** (mk):
    - v1's rail carries the primary leaks: decisions, focus and the estate
      picture. Rig health and PRs/CI/Sonnerie join after the trial.
    - The plugin is installed from the Autarch repo during the trial.
      Whether to bundle it into Aleph is ruled on the trial's evidence.
    - Engineering defaults for planning:
      - a Clavain helper, so filing a decision takes one call, plus an
        instruction in the shared agent guidance;
      - decisions only in the hub tracker, falling back to a plugin store if
        beads fails the trial;
      - panels kept thin over RPC.
16. **A ruling is a signed file** (mk, 2026-09-24, option A4):
    - **Format:** one Markdown file per ruling, reusing the existing card
      `ratification` block (`ruled_by`, `ruled_at`, `ruling`,
      `transcribed_by`, `session_id`, `source`, `supersedes: path@commit`).
    - **Fields added for Home:** `bead`, `asking_thread`, `options_shown`
      (with the choice marked), `continuation` (kind and hash) and `state`.
    - **Signing:** Home signs every ruling it writes from a pick
      (`ssh-keygen -Y sign`, Home's key). Lattice and the hooks treat unsigned
      files as **proposed**. This makes Ultan ruling 2 ("only mk moves a fact
      to ruled") structural, and fixes the thesis's "asserted actors" limit
      from the first day.
    - **Layout:** Uqbar holds `rulings/YYYY-MM-DD-<slug>.md`, plus
      `focus/current.md`, where each change supersedes the previous commit.
      Git history is the focus timeline.
    - **Measuring focus** is a query over that timeline: which focus commit
      was current when each proposal was made.
    - **Rejected:**
      - an append-only ledger: concurrent appends conflict across machines;
      - git itself as the ledger: rulings stop being documents;
      - bb events as the ledger: breaks "files are truth";
      - a passkey tap on every pick: Home access already equals shell access.
17. **Decisions are fire-and-forget, and rulings reach agents through a feed**
    (mk, 2026-09-24, options B4 and B5):
    - Filing a decision with continuations hands off that branch. The asker
      needs no answer unless it marked the decision `needs-context`.
    - One Clavain `UserPromptSubmit` hook injects, at the start of each turn,
      the rulings relevant to that project only:
      - a focus change;
      - new rulings about this project;
      - answers to this thread's own decisions.
    - One line per item, read through a fast, cached path. It must never
      grow into another `bd prime`.
    - This is also how focus reaches coordinators in the middle of a session.
    - **Checked:** `bb thread queue` cannot deliver without waking the
      thread. Its modes are only `auto` and `steer`, and a message queued to
      an idle thread is sent immediately (`apps/server/src/services/threads/queued-messages.ts`).
      Plugin waits hold a message back, but it is still delivered as its own
      turn.
    - **Gap:** providers without Clavain hooks. An Aleph "attach to next
      turn" queue mode waits until they are a real share of mk's threads.
18. **Rulings from the autarch-07 walk-through** (mk, 2026-09-24,
    [autarch-07](../cujs/autarch-07-decide-and-continue.json) now
    validated):
    - **A fourth continuation kind, ruling-only.** The filing helper
      requires a kind on every option. Ruling-only records the ruling, and
      the feed tells the thread. Agents learn to write continuations from
      the helper's prompts and worked examples in the Clavain guidance.
    - **Stale decisions have two guards.** Home marks a decision stale on
      the rail. Every pick also re-checks the continuation's hash and
      preconditions; if they fail, the pick becomes a re-ask.
    - **A failed continuation reopens a decision.** The ruling stands with
      state `failed`, and a new decision is owed: retry, pick another
      option, or hand it to a thread. There is one queue.
    - **Signing key.** Home has its own ed25519 key on zklw, used for
      nothing else. Its public key goes in an `allowed_signers` file
      committed to the Uqbar, so every machine gets it through git. To
      rotate, add the new key with `valid-after` and expire the old one.
19. **Focus is cut from v1** (mk, 2026-09-25: "let's cut it for now and
    then determine later if we need it").
    - **Why:** walking autarch-08 grew focus into a bead, a signed file,
      two hooks, a Mycroft filter and an adherence query. mk called it
      "overly ornate/intricate ceremony". A leaner version would gate
      machine-started turns and nudge only on new work, but it had little
      to steer: most estate work starts with mk, and Mycroft does not
      dispatch on its own below T2.
    - **What changes:** the focus bar and focus bead go. Focus lines in
      the SessionStart and feed hooks go. `focus/` goes from the Uqbar
      layout. Build step 1 is decisions only. The trial is judged on
      "catches" alone. The feed hook still carries project rulings and a
      thread's own answers.
    - **Parked, not deleted:**
      [autarch-08](../cujs/autarch-08-focus-shows-up.json) keeps the lean
      design (one list, a turn gate, a nudge on new work) as a draft.
      Revisit it when Mycroft reaches T2, or if the trial shows the walk
      needs a weekly lens.
20. **The net, validated** (mk, 2026-09-25,
    [autarch-09](../cujs/autarch-09-the-net-catches.json)):
    - It reads thread endings only. A rotation's handoff counts as an
      ending, but handoff files and commits are not scanned.
    - There is no mute threshold up front. Dismissals and catches are
      logged, and the threshold is set from those real numbers. The net
      ships at step 6, after the v1 trial, so its own first weeks serve as
      its trial.

## Open questions

1. *(Resolved: see decisions 8 and 12.)*
2. *(Resolved: decisions 13 and 14.)*
3. *(Resolved: decision 18.)* Teaching continuations.
4. *(Resolved: decision 18.)* Home's signing key.
5. **Hook latency.** The feed runs on every turn, so it needs a
   per-project cache that is invalidated when a bead closes or Uqbar
   receives a commit.

## Review

A flux-drive review of this document is held under the bbOps quota note
(hold Opus reviews unless a gate needs one). It should run before
`/clavain:write-plan`.
