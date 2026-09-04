# Catch-up briefing — execution-grade plan (cujgel consume)

Written 2026-09-01 and held outside the repo until 2026-09-02: the plan guard
refuses plan files while a project's card is absent or provisional, and Autarch
had no card. It landed the moment mk ratified `docs/why.md`, with no override.
The build it describes shipped on 2026-09-01 as commit 20b84e2.

**Spec:** `docs/cujs/autarch-01-daily-walk.json` · version **1.3** · status validated ·
sha256 `27be5b5dd1f61e6ff2959ce5722f60a65377ab344b02f3e1ac7fb99c7bbb5024`
**Goal:** intercore goal `da757a12` "Ship the catch-up briefing from autarch-01" (2026-09-01)
**Slice:** step 1 of the walk (arrive to orientation), plus the exit half of step 5
(close Autarch owing nothing). Steps 2–4 are touched only where the briefing must
sit beside them (layout). OUT: waiting-on-me scraping, salon, graph.

## mental_model (verbatim — the constraint solver)

> The walk is the watching/waiting half of the gardening loop — 'lots of context
> switching and deep diving and then watching/waiting for intervention points.'
> Stellaris gets stillness free by freezing the world; mk's world never halts, so
> the door must stop *the gardener* instead: orientation completes before any
> obligation is allowed to speak. The estate is scanned like an outliner, not read
> like an inbox — at ~98 projects the default is quiet, and a row earns its place
> on the walk. Leaving without acting is a successful walk, not an aborted one.

## Closed decisions in force (not re-litigated)

- The door transcribes; it owns no state (HUD ruling). → The briefing is computed
  from sources on disk at open time. The one client-local fact it keeps is *when mk
  last opened it*, which is a preference-class local (same class as pins), not
  world state.
- Orientation-before-obligation. → The briefing is the opening view; verdict rows
  (obligations) are one keystroke away, never first.
- Quiet-unless-attention-worthy. → Gardens with no movement are omitted from the
  briefing, not listed as "0".
- Ontology ruling (2026-09-01, docs/research/2026-08-31-estate-ontology.md): the
  surface has no name — bare `autarch` opens the vantage. → Root command runs the
  door; `autarch door` stays as an alias. Package `internal/door` is not renamed in
  this slice (the goal's DONE WHEN cites it).

## GATE ruling (mk, 2026-09-01, before any build)

Asked: what does the briefing read from? Options offered: git + Claude transcripts
(recommended), git only, git + alwe sessions (async), per-repo events ledger.
**mk: "1 then 3."** → This slice reads git (commits on any ref since the window
opened, plus uncommitted files) and Claude Code transcript directories (sessions
per garden). The follow-on slice adds alwe's session catalog as an async column
with a timeout. Recorded on the spec as a closed decision at v1.4.

## Invariants verified at plan time (2026-09-01 22:30)

- `go test -race ./internal/door` → `ok … 5.681s` (exit 0).
- `go vet ./internal/door ./cmd/autarch` → clean.
- Bare `autarch` today prints cobra help (root has no RunE) — wiring it is additive.
- Data-source probes on this machine:
  - tmux: every live session's active pane sits at `/Users/sma/projects` (estate
    root), so `session_activity` cannot attribute movement to a garden.
  - cass: `cass search --robot` returns `checkpoint_incomplete` (index degraded;
    the repair is `cass index --full`, which memory says must wait for a backup).
  - alwe: `alwe health` took >2 min to answer; local catalog healthy, cass stale.
    Unfit for a briefing that must land within a minute tonight; it is the "then 3"
    follow-on as an async column.
  - git: universal across the estate; `log --all --since` + `status --porcelain`.
  - Claude Code transcripts: `~/.claude/projects/<abs path with / and . → ->/*.jsonl`
    mtimes attribute agent sessions to gardens (nested dirs roll up by longest
    prefix). Last 24h: 46 files at the estate root, 1 under Sylveste — thin per
    garden today (the archaeology's scaffolding finding), honest, and cheap.

## Work items

### WI-1 — `internal/door/briefing.go`: what moved, per garden
serves: step 1 (catch-up briefing pane); closed decisions HUD-transcribes, quiet
default; GATE ruling "1".
- `Movement{Root, Name, Since, Commits, Dirty, Sessions, Latest, Err}`, `Moved()`.
- `GitMovement`: `log --all --since=<ISO8601> --format=%H%x1f%ct%x1f%s -n 200`,
  then `status --porcelain`. One 10s timeout per garden. Any failure → `Err`,
  never a zero.
- `IndexSessions`: one pass over the transcripts root; each directory attributed
  to the garden whose encoded root is its longest matching prefix (exact, or
  prefix + "-"); counts `*.jsonl` with ModTime after `since`. Unreadable root →
  error; garden with no directory → absent from the map (a real zero).
- `Movements`: 8-worker fan-out mirroring `CheckAll`; merges the session stat.
Acceptance: `TestEncodeTranscriptDir`, `TestIndexSessionsRollsUpNestedAndKeepsSiblingsApart`,
`TestGitMovementCountsOnlyTheWindow`, `TestGitMovementUnreadIsNeverQuiet`,
`TestMovementsReportsEveryGarden`.

### WI-2 — `internal/door/visit.go`: the last-visit stamp
serves: step 1 ("since the last visit"); Stellaris G (Empire Timeline park/load bridge).
- `~/.autarch/last-visit`, RFC3339. Written on quit. Absent → first visit, 24h.
  Malformed → first-visit window plus the error, stated in the header.
- `Window(override, visitPath, now)`; `--since` accepts durations (incl. `3d`)
  or RFC3339 and never writes the stamp.
Acceptance: `TestVisitRoundTrip`, `TestVisitAbsentIsFirstVisit`,
`TestVisitMalformedIsAnError`, `TestWindow`, `TestQuitSavesVisit`.

### WI-3 — `internal/door/model.go`: briefing view + both layouts
serves: step 1; open question "briefing alone vs above rows" (both built, decided
against a real render); closed decision orientation-before-obligation.
- `WithBriefing(BriefingOptions)`; `LayoutAlone` (default) | `LayoutAbove`;
  keys `b` layout, `tab` rows/briefing, `w` widen (24h→3d→7d→30d→stamp), `r`
  re-read. The briefing screen has no actions.
- Header: `since <t> · <dur> · <source> [· reading N…]` and
  `N of M gardens moved · commits · uncommitted · claude sessions` (or
  `claude sessions: UNCHECKED (<err>)`). One line per moved garden, newest first.
  `+N more`; `could not read: …`; `nothing moved since <t>` once all read.
- Title `AUTARCH` (door retired from product vocabulary).
Acceptance: `TestBriefingListsOnlyWhatMovedNewestFirst`,
`TestBriefingNothingMovedAndUnreadSessionsRoot`, `TestLayoutsAndScreens`,
`TestBriefingScreenHasNoActions`, `TestWidenCyclesAndReturns`; existing tests
unchanged and green under `-race`.

### WI-4 — `cmd/autarch/door.go` + `cmd/autarch/main.go`: bare `autarch` opens it
serves: ontology ruling "no surface name" (2026-09-01); step 1 build_surface.
- `doorOptions`, `addDoorFlags`, `runDoor`; root `RunE` + `Args: cobra.NoArgs`;
  `door` subcommand kept as alias. Flags `--since`, `--layout alone|above`.
Acceptance: `go build ./cmd/autarch`; `autarch --help` lists subcommands;
`go run ./cmd/autarch --since 48h` opens to the briefing over the real estate.

### WI-5 — docs
- This plan (blocked on the card guard); `docs/cujs/autarch-01-daily-walk.json` →
  v1.4 (source → closed decision in mk's words; layout → closed after the render
  decision), revalidated; README v1.4 note.

## Question ledger

| gate | question | disposition |
|---|---|---|
| GATE (mk) | what the briefing reads from | **asked before build; mk: "1 then 3"** — git + Claude transcripts now, alwe sessions async next. tmux excluded by evidence; cass deferred (degraded). |
| ask_then_proceed | briefing alone vs above rows | provisional: build both, default alone (orientation-before-obligation), mk rules against the render. Reversal = flip the default. |
| ask_then_proceed | decomposition order + transitional thread rendering | provisional: briefing first (this goal is that answer; most-exercised organ per archaeology). Thread rendering is out of slice. |
| auto_proceed | signal freshness | computed on open; `r`/`w` re-read; no background cadence. |
| `[invented]` 1 | last-visit stamp at `~/.autarch/last-visit`, written on quit; first visit = 24h | no ledger entry covers "how the door knows when you last came"; preference-class local, fails wide. |
| `[invented]` 2 | "moved" = commits on any ref + uncommitted files + transcript files touched; quiet omitted | the spec says "what moved"; this is the cheapest honest operationalization on today's substrate. |
| `[invented]` 3 | the briefing screen has no actions; `tab` to rows | keeps orientation free of obligation literally; reversible. |
| `[invented]` 4 | `w` window cycling + `--since` override | a stamp-only window makes a second open in five minutes empty; widening is the escape. |
| `[invented]` 5 | sessions column counts Claude Code only (Codex transcripts carry cwd inside the file) | header labels it `claude sessions` so the omission is visible. |

## Success audit

**Structured proxies**
1. Cold start opens to the briefing before any verdict row is visible — run it.
2. Only gardens that moved appear, newest first; quiet gardens absent — test.
3. The window (since when, how long, why) is in the header — test.
4. Gardens the door could not read are named, never dropped or shown as zero — test.
5. Open → read → quit with no action exits clean and advances the stamp — test.
6. Whole estate reported within ~a minute on this machine — timed run by hand.

**Recognition check (never automated; put to mk after the first real morning)**
"After you opened it this morning: did it tell you what moved before anything asked
you for a decision — and did it feel like loading a save from stillness, or like an
inbox?" Three consecutive mornings of use is the goal's DONE WHEN; mk's words are
the verdict.

## Follow-on (not this slice)
- "then 3": alwe session catalog as an async column with a timeout (bead candidate).
- Codex transcripts in the sessions count (needs cwd from inside the file).
- Autarch's own product card (docs/why.md) — the plan guard's real ask. Done
  2026-09-02: drafted, then ratified by mk (goal e961b2a6).

## Re-entry
Derived from spec v1.3 @ 27be5b5d. If the spec changes, stop the items whose
serves-lines cite changed material, diff, re-derive those only, and log it here.

**Event 1 — 2026-09-01, after the build (WI-5 itself):** spec moved 1.3 → 1.4,
sha256 `7941050ce0fd3854ea1c606b5b00df5902c922b5a16af98cd6d2ec289a7f692c`. Diff:
the GATE ruling recorded as a firm closed decision in mk's words; the last-visit
window recorded as a leaning closed decision; the source open question removed;
the layout question amended to say both layouts are built and `b` flips; a
provenance entry added. Classification: no work item's serves-line cites material
that moved as a constraint — every change records what this plan already did.
Provisionals: the layout provisional stands (the question is still open, to be
ruled against the render); `[invented]` 1 is now covered by the ledger and stops
counting as invented, so the drift count is 4. Nothing re-derived.
