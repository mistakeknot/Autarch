# Thread registry — execution-grade plan (cujgel consume)

**Spec:** `docs/cujs/autarch-01-daily-walk.json` · version **1.4** · status validated ·
sha256 `7941050ce0fd3854ea1c606b5b00df5902c922b5a16af98cd6d2ec289a7f692c`
**Goal:** intercore goal `cf9e8644` "Make Autarch the thread registry" (2026-09-02)
**Evidence:** `docs/research/2026-09-02-thread-registry-probe.md` (mk's note verbatim + the same-day measurement)
**Slice:** step 2 of the walk (the live-session column and the pivot), step 3's first half
(running / idle shell / no seat, without pane-content scraping), step 5 unchanged. Steps 1
and 4 touched only where the briefing's sessions count must stop lying.
OUT: pane-content scraping for the waiting-on-a-human state, alwe, salon, graph, Codex
transcript attribution, the stamp flow, focusing emulator windows.

## mental_model (verbatim — the constraint solver)

> The walk is the watching/waiting half of the gardening loop — 'lots of context
> switching and deep diving and then watching/waiting for intervention points.'
> Stellaris gets stillness free by freezing the world; mk's world never halts, so
> the door must stop *the gardener* instead: orientation completes before any
> obligation is allowed to speak. The estate is scanned like an outliner, not read
> like an inbox — at ~98 projects the default is quiet, and a row earns its place
> on the walk. Leaving without acting is a successful walk, not an aborted one.

## Closed decisions in force (not re-litigated)

- The door transcribes; it owns no state. → Threads are read from tmux and the transcripts
  at open time. Nothing about a thread is persisted by Autarch.
- Quiet-unless-attention-worthy. → The threads screen lists seats, not gardens; idle and
  seatless states are marked, never hidden.
- The row stays the garden (mk's axis correction, 01 ledger). → Garden rows keep their
  shape; threads get their own screen and feed the garden rows' live-thread count.
- Entry is tmux switch-client (shipped). → Unchanged in this slice.
- "Waiting-on-me detection comes from tmux scraping (Claude-Squad pattern)" (leaning). →
  Consistent: this slice reads tmux's pane command, not pane content; the content scrape
  stays the next slice.

## GATE rulings (mk, 2026-09-02, in the session that wrote this plan, before any build)

Asked one at a time against the probe document. Transcribed by Claude Fable 5.1
(claude-fable-5-1), session 5920c9b1-6a3f-4a7d-8566-e6067aaeaf01.

1. **Row unit** — offered: threads screen + garden rows (recommended); threads nested under
   garden rows; thread rows replace garden rows. **mk chose "Threads screen + garden rows".**
2. **Marks** — offered: left/right/full; two displays; historical only. **mk, verbatim:
   "[ left 3/4s of the screen, ] right 3/4s, [] center 3/4s (i use 3/4s on my macbook 14
   so i can get more horizontal space to review the terminal window".** Three overlapping
   three-quarter-width positions on one display: a real column.
3. **Entry** — offered: keep switch-client now and rule on window focus later
   (recommended); focus the thread's own window; both. **mk: "1, and let's explore
   standardizing to iterm2".** Entry unchanged here; "standardize on iTerm2" is recorded as
   the open question for the focus-window slice (it collapses three emulator adapters to one).

## Invariants verified at plan time (2026-09-02 16:40)

- `go test -race ./internal/door` green at 792bac2 (run before dispatch; executor re-runs first).
- Branch `cuj-lineage`, worktree `apps/Autarch/.claude/worktrees/door`; main is behind, not ahead.
- Live substrate on this machine: 39 tmux sessions, all panes at `~/projects`; 32 panes
  run a Claude Code version binary; 33 Claude transcripts resolve by id under
  `~/.claude/projects/*/<id>.jsonl`, 31 of them in `-Users-sma-projects`.
- Existing `ListSessions` (`internal/door/sessions.go:106`) parses exactly 5 `\x1f` fields.
  Adding a field changes the parser and its fixtures.
- Existing `IndexSessions` (`internal/door/briefing.go:165`) counts `.jsonl` by ModTime.

## Work items

### WI-1 — `internal/door/threads.go`: parse the seat, classify the pane
serves: step 2 (live session counts, pivot data), step 3 (running / idle / no seat); ruling 2.

```go
type Mark string
const (
    MarkNone   Mark = ""
    MarkLeft   Mark = "["   // left three-quarters of the display
    MarkRight  Mark = "]"   // right three-quarters
    MarkCenter Mark = "[]"  // center three-quarters
)

// Seat is what a tmux session name says about itself in mk's registry form:
//   <terminal><mark><topic> - <resume id>     e.g. "iterm]solwend - 21cc6bd2-…"
// A name in no such form is a seat with Topic = name and nothing else.
type Seat struct {
    Terminal string // "wezterm" | "iterm" | "rio" | ""
    Mark     Mark
    Topic    string // trimmed; "ushas/bridger", "jeddnet@codex"
    ResumeID string // "" when the name carries none
}

// Runtime is what the pane is running, from #{pane_current_command}.
type Runtime string
const (
    RuntimeClaude Runtime = "claude" // command matches ^\d+\.\d+\.\d+$ (the version binary)
    RuntimeCodex  Runtime = "codex"
    RuntimeKimi   Runtime = "kimi"
    RuntimeShell  Runtime = "shell"  // zsh bash fish sh: an idle seat
    RuntimeOther  Runtime = "other"
)

func ParseSeat(name string) Seat
func ClassifyPane(cmd string) (Runtime, version string) // version only for RuntimeClaude
```

- `ParseSeat` regex: `^(wezterm|iterm|rio)?(\[\]|\[|\])\s*(.+?)(?:\s+-\s+(\S+))?\s*$`.
  Double spaces before the id occur in the real data (`jeddnet -  f44f…`): `\s+` handles it.
  Trim the topic. No match → `Seat{Topic: name}`.
- `ClassifyPane`: version regex → claude; exact `codex` → codex; exact `kimi` → kimi;
  `zsh|bash|fish|sh` → shell; anything else → other with version "".

Acceptance: `TestParseSeatEveryShapeInTheNote` (table from the probe: all three marks, the
three emulators, no-terminal `[cujgel - id`, no-id `rio]garden-salon`, double-space
`jeddnet -  id`, plain `28`, `grey-area`, `kimifork`), `TestClassifyPane` (`2.1.258`→claude
2.1.258; `codex`; `kimi`; `zsh`→shell; `python3.11`→other; `node`→other).

### WI-2 — `internal/door/sessions.go`: read the pane command
serves: step 3 (running vs idle vs gone) — the substrate ruling.

- Add `Command string` to `TmuxSession`. Extend the `-F` format with a sixth field
  `#{pane_current_command}`; the parser requires exactly 6 fields. Update every fixture and
  test that builds raw lines. `TmuxSession` literals elsewhere keep compiling (new field zero).
- No other behavior change in this file. `ResolveSessions` by pane path stays as the first
  pass (it is correct when a pane is inside a garden).

Acceptance: `TestListSessionsParsesSixFields` (fixture output through the parser with a
malformed 5-field line rejected), existing sessions tests green.

### WI-3 — `internal/door/transcript.go`: the last real turn and the gardens touched
serves: step 2 (attribution of a root-launched thread to its gardens), step 1 (the sessions
count stops lying); probe findings 3 and 4.

```go
// FindTranscript locates <root>/*/<id>.jsonl; several matches → newest ModTime wins.
func FindTranscript(transcriptsRoot, id string) (path string, err error)

// LastTurn is the timestamp of the last entry whose type is "user" or "assistant".
// Bookkeeping rows (bridge-session, mode, permission-mode, last-prompt, atis-latch,
// system/*) carry no timestamp or are not turns and never count. Reads the trailing
// tailBytes (256<<10) only; zero time + nil error when no turn is in the tail.
func LastTurn(path string, tailBytes int64) (time.Time, error)

// GardenHit is one garden mentioned by absolute path inside a transcript.
type GardenHit struct { Root, Name string; Mentions int }

// Gardens counts mentions of every project root inside the trailing scanBytes
// (16<<20) of the transcript. A path under a nested project (Sylveste/apps/Autarch)
// credits the longest matching root only. Sorted by Mentions desc, then Name.
func Gardens(path string, projects []Project, scanBytes int64) ([]GardenHit, error)
```

- Implementation of `Gardens`: build one regexp per distinct estate root:
  `regexp.QuoteMeta(root) + "/[^\"'\\\\\\s]+"`; for each match take the matched path,
  walk project roots sorted longest-first, `path == r || strings.HasPrefix(path, r+"/")`
  → credit `r`. Stream with `bufio.Scanner` and a 4 MiB buffer (lines are long).
  Estate roots = the distinct `filepath.Dir` ancestors passed to `DiscoverProjects`; pass
  them in via `Project.Root` prefixes (the scan roots are known to `runDoor`; add a
  `Roots []string` parameter rather than inferring).
- `LastTurn`: seek to `size - tailBytes` (or 0), discard the partial first line, then for
  each line `json.Unmarshal` into `struct{ Type, Timestamp string }`, keep the last with
  Type in {user, assistant} and a parseable RFC3339 timestamp.

Acceptance: `TestFindTranscriptPicksNewest`, `TestLastTurnIgnoresBookkeepingTail` (fixture
whose last six lines are untimestamped bookkeeping after an assistant turn dated
2026-08-25), `TestGardensLongestRootWins` (fixture mentioning `/e/Sylveste/apps/Autarch/x`
and `/e/Sylveste/y` with both projects present → Autarch 1, Sylveste 1),
`TestGardensNoPathsIsEmptyNotError` (the taxes case).

### WI-4 — `internal/door/threads.go`: assemble threads, attribute, and count
serves: step 2 (the live-thread count per garden row; enter targets the right thread),
step 3; ruling 1.

```go
type Thread struct {
    Session  string // tmux name, verbatim
    Seat     Seat
    Runtime  Runtime
    Version  string
    Activity int64
    Path     string    // pane path (kept for ResolveSessions parity)
    Transcript string  // "" when none
    LastTurn time.Time // zero = unknown
    Gardens  []GardenHit
    Err      error     // transcript could not be read; the row says so
}

type ThreadSet struct {
    Threads    []Thread            // stable order: Activity desc, then Session
    ByRoot     map[string][]Thread // attributed threads per garden root, most recent first
    Pending    int                 // transcripts still being read
    Err        error               // tmux could not be listed: everything else is meaningless
}

// Attribute credits a thread to a garden when the pane path resolves there (first pass,
// as ResolveSessions) or, for a thread at no garden, to every garden holding at least
// minShare (0.2) of its mentions and always to its top garden. A thread with no
// transcript and no resolvable path is attributed to nothing and still listed.
func Attribute(threads []Thread, projects []Project, minShare float64) map[string][]Thread
```

- `SessionSet.Count(root)` and `SessionSet.Target(root)` remain the API the rows use;
  build the `SessionSet` from `ThreadSet` so rows and enter read attribution. Concretely:
  add `func (ts ThreadSet) Sessions() SessionSet` that fills `ByRoot` with the tmux
  sessions of attributed threads (most recent LastTurn first, Activity as tiebreak),
  `Total`, `Resolved` = threads attributed to at least one garden, `Unresolved` = the rest.
- Streaming: `ReadThreads(ctx, sessions []TmuxSession, projects []Project, roots []string,
  transcriptsRoot string, onThread func(Thread))` — 8 workers as `Movements`; each worker
  does `FindTranscript` → `LastTurn` → `Gardens` for claude threads with a resume id, and
  emits the thread; non-claude threads are emitted immediately with no transcript.

Acceptance: `TestAttributeRootLaunchedThreadLandsOnItsGardens` (pane at estate root,
gardens A 80 / B 15 / C 5 → A and B credited at minShare 0.2, not C),
`TestAttributeKeepsPanePathFirst`, `TestThreadSetSessionsPreservesCountAndTarget`
(enter target is the thread with the newest LastTurn), `TestReadThreadsEmitsEveryThread`
(non-claude and no-id threads included; a missing transcript is `Err`, not a drop).

### WI-5 — `internal/door/model.go` + `internal/door/threads_view.go`: the threads screen
serves: step 2 (the pivot one Tab away), step 3 (the three visible states), closed decision
orientation-before-obligation (the briefing stays first).

- New `screenThreads`. Keys: `t` toggles the threads screen from the briefing or the rows
  and returns to the previous screen; on the threads screen `tab` also returns; `↑/↓/j/k/g/G`
  move; `enter` runs `tmux switch-client -t <Session>` (mirror `switchToSession`, no
  Project required); `r` re-reads threads as well; `q` quits. In `LayoutAbove` the threads
  screen replaces the rows area while shown.
- Header line 1: `threads: N seats · N running (claude N · codex N · other N) · N idle
  shells · N no seat data` ; while reading: `· reading N…`. Header line 2 when tmux could
  not be listed: `threads: UNCHECKED (<err>)` in `styleErr`, and no rows.
- One line per thread, Activity desc:
  `<glyph> <term 7> <mark 2> <topic ≤24> <id 8> <state 12> <gardens…>`
  glyph: claude `◆`, codex/kimi `◇`, shell `○`, other `·`.
  state: `<version>` for claude with LastTurn within 3 days (`2.1.258 2h`), `idle 8d` in
  `styleFunded` when LastTurn is 3 days or older, `no transcript` when the id resolves to no
  file, `could not read` in `styleErr` on Err, `idle shell` for RuntimeShell, `no id` when
  the seat carries none. Gardens: names joined by `, `, truncated to width, top first.
- Footer keys on this screen: `↑/↓ move · enter switch · t rows · r re-read · q quit`.
- `WithBriefing` is not the switch for threads; add `WithThreads(ThreadsOptions{Roots,
  TranscriptsRoot, Now})` so rows-only tests are untouched. `Init` starts the read when on.
- Never `[]rune`-slice styled strings; use the existing `pad`/`truncate` on plain text and
  style afterwards.

Acceptance: `TestThreadsScreenToggleAndReturn`, `TestThreadsRenderThreeStatesVisiblyDistinct`
(running / idle Nd / idle shell / no transcript / could not read all present and distinct),
`TestThreadsScreenEnterTargetsSession`, `TestThreadsUncheckedShowsNoRows`; every existing
model test green under `-race`.

### WI-6 — `internal/door/registry.go`: the drift diff against a note file
serves: goal OUTCOME clause "drift between the note and tmux is shown, never silently
reconciled"; the one-time migration off the Apple Note.

```go
// ParseRegistryNote reads mk's note format: section lines starting with "——" are
// skipped, blank lines skipped, every other line is ParseSeat(line).
func ParseRegistryNote(r io.Reader) ([]Seat, error)

type Drift struct {
    Kind  string // "stale id" | "renamed" | "no seat" | "not in note"
    Topic string
    Note  string // what the note says
    Live  string // what tmux says
}

// DiffRegistry compares note seats to live threads by ResumeID first, then by Topic:
//   same id, different topic     → "renamed"   (rakes-of-the-new-sun → rakes-of-the-new-book)
//   same topic, different id     → "stale id"  (ushas/bridger 21434d6f vs ef9ad21a)
//   note seat with no live match → "no seat"   (shadewright, garden-salon, the 14 no-id topics)
//   live thread with no note seat→ "not in note" (ryan, spellswords@hermes, 28, 30, kimifork, mobile)
// A note line that shares an id with another note line (autarch/estate/cujgel) is one
// seat with several topics, not drift.
func DiffRegistry(note []Seat, live []Thread) []Drift
```

- `internal/door/testdata/session-note.txt` = the note verbatim from the probe document.
- Surface: `autarch --registry <file>` adds a `registry:` block at the top of the threads
  screen listing each drift as `<kind>: <topic> — note <x> · live <y>`; without the flag no
  block. Nothing is written anywhere.

Acceptance: `TestDiffRegistryFindsTheThreeKnownDrifts` (fixture live list built from the
probe's tmux table; expects stale id ushas/bridger, renamed rakes, no seat shadewright,
not-in-note ryan) and `TestDiffRegistrySharedIdIsNotDrift`.

### WI-7 — `internal/door/briefing.go`: the sessions count stops lying
serves: step 1 (the briefing must be true); probe finding 4.

- `IndexSessions` counts a transcript inside the window when `LastTurn(path, 256<<10)` is
  after `since`, not when ModTime is. `SessionStat.Latest` becomes the newest LastTurn.
  Files with no turn in the tail do not count. Keep the directory-based garden attribution
  here (this index is about per-garden directories; the threads pass covers root-launched
  threads).
- Acceptance: `TestIndexSessionsUsesLastTurnNotMtime` (a fixture file touched now whose
  last turn is 10 days old is not counted for a 24h window), existing briefing tests green.

### WI-8 — `cmd/autarch/door.go` + `cmd/autarch/threads.go`: flags and a plain listing
serves: DONE WHEN "a real render over the estate"; the validator's machine check.

- `--registry <file>` flag on the root and `door` commands; `runDoor` passes
  `door.ThreadsOptions{Roots: roots, TranscriptsRoot: door.DefaultTranscriptsRoot(),
  RegistryPath: o.registry}` via `WithThreads`.
- New subcommand `autarch threads [--root …] [--registry file] [--json]`: no TUI; runs the
  same `ReadThreads` to completion and prints one line per thread in the screen's column
  order, then the drift block when `--registry` is given. `--json` prints `[]Thread` plus
  `drift` as JSON. Exit 0 on success; exit 2 when tmux could not be listed (message on stderr).
- Acceptance: `go build ./cmd/autarch`; `autarch --help` lists `threads`;
  `autarch threads --json | jq '.threads | map(select(.runtime=="claude")) | length'` ≥ 30 on
  this machine (32 on 2026-09-02); `autarch threads --registry /Users/sma/.claude/jobs/cfe8ca13/tmp/session-note.txt`
  prints lines containing `stale id: ushas/bridger`, `renamed: rakes-of-the-new-sun`,
  `no seat: shadewright`.

### WI-9 — docs
- `docs/cujs/autarch-01-daily-walk.json` → v1.5: three GATE rulings above recorded as
  closed decisions in mk's words (rulings 1 and 3 as `{"decision": …, "confidence": "firm"}`,
  ruling 2 verbatim); the garden-vs-thread ledger question amended to say the ruling landed
  (row = garden, threads on their own screen); a new open question
  `{"gate":"ask_then_proceed","question":"Entry that focuses the thread's own emulator
  window; mk 2026-09-02: 'let's explore standardizing to iterm2'"}`; provenance entry for
  this build; run `~/projects/Sylveste/interverse/cujgel/bin/cujgel-validate
  docs/cujs/autarch-01-daily-walk.json` and record the new sha256 in this plan's re-entry
  section. `docs/cujs/README.md` gets a v1.5 line.
- This plan's Question ledger updated with anything invented during execution.

## Question ledger

| gate | question | disposition |
|---|---|---|
| GATE (mk) | row unit | **ruled: threads screen + garden rows** |
| GATE (mk) | marks | **ruled: left / right / center three-quarters of one display** |
| GATE (mk) | entry | **ruled: unchanged; explore standardizing on iTerm2 (next slice)** |
| ask_then_proceed | the 14 note topics with no id and no seat | provisional: shown once by the `--registry` diff as "no seat", not persisted; reversal = a `seats:` list in `~/.autarch/door.yaml` (preference-class, same as pins) |
| auto_proceed | idle threshold | 3 days; the four idle threads today are 6–19 days, the rest under 1 |
| auto_proceed | attribution share | top garden always, plus any garden ≥ 20% of mentions, over the trailing 16 MiB |
| `[invented]` 1 | `t` as the threads key and Tab returning | no ledger entry names the key; Tab already means "the other screen" |
| `[invented]` 2 | `autarch threads` plain subcommand | needed for a machine-checkable render; mirrors how `door` sits beside the root |
| `[invented]` 3 | drift order: stale id, renamed, not in note, then no seat | the screen caps the block at 8 lines; the note's own order put the 14 seatless topics ahead of the two lines mk would fix today |
| deviation (execution) | `Gardens` derives estate roots from the top-level project roots; `ReadThreads` accepts `roots` and does not use it | equivalent for attribution (only project roots are credited either way); the signature stays as planned |
| deviation (execution) | `--json` emits `{threads, drift}` with snake_case fields, not a bare `[]Thread` | one object carries both results; the WI-8 jq acceptance reads `.threads` |
| deviation (execution) | header says `N unmarked`, not `N no seat data` | shorter, and it names the fact: the session name carries no mark |
| deviation (execution) | `TestThreadsRenderStatesVisiblyDistinct` (not `…ThreeStates…`); adds `TestThreadsUpgradeGardenRowsAfterRead`, `TestThreadsRegistryBlockShowsDrift` | same claims, one more state (`no id`) covered |
| deviation (execution) | `TestTmuxCaptureSwitchClientAndZed` now also waits for the rows' footer key | the briefing names both gardens in its could-not-read line, so the old wait passed before Tab landed; a real flake under the full suite |
| execution note | the Sonnet executor finished WI-1..4 and stalled on a rate limit at WI-5; the frontier model finished WI-5..9; an Opus validator accepted every acceptance line (2026-09-02) | capability-routing doctrine: a stalled executor keeps frontier in the loop |

## Success audit

**Structured proxies**
1. `go test -race ./internal/door` passes with every named test present — run.
2. `autarch threads --json` on this machine lists ≥ 30 claude threads, each with a version, a
   non-zero LastTurn, and ≥ 1 garden except taxes — run and inspect.
3. The four idle threads (taxes, fluxrig, jawnbase, jetty/fissionchips) render `idle Nd` — inspect.
4. `--registry` on the note reproduces the three known drifts — run.
5. Garden rows show `◆N` for gardens that had `◆` blank before (solwend, zahro, uncrancher,
   jawnomicon at minimum) — inspect a real render.
6. Nothing under `~/.autarch` changes when the threads screen is used — `ls -la` before/after.

**Recognition check (never automated; to mk after the first real use)**
"Open autarch, press t. Is that your note? Could you delete the session list from the Apple
Note today, and what would you miss?" mk's words are the verdict; deleting the list is the
goal's DONE WHEN.

## Re-entry
Derived from spec v1.4 @ 7941050c. If the spec changes, stop the items whose serves-lines
cite changed material, diff, re-derive those only, and log it here. WI-9 itself moves the
spec to v1.5 after the build; record the hash and classify the event below.

**Event 1 — 2026-09-02, after the build (WI-9 itself):** spec moved 1.4 → 1.5, sha256
`376d4c3924723161835f7291176f4dcbe6252536ad2dc26e9fd2fb831142d77b`. Diff: the three GATE
rulings and the substrate decision recorded as firm closed decisions; the garden-vs-thread
open question removed (closed by ruling 1); two open questions added (window-focus entry
with the iTerm2 standardization, the 14 seatless note topics); a provenance entry.
Classification: no work item's serves-line cites material that moved as a constraint;
every change records what this plan already did or asks what the next slice must ask.
Provisionals: the seatless-topics provisional now has a ledger entry on the spec and stops
counting as invented, so the drift count is 2 (`t`/Tab keys; the plain subcommand), plus
the drift ordering invented during execution: 3. Nothing re-derived. Build shipped as
7063b58, 2c0b07b, ee1acf1, d059d92 on `cuj-lineage`; `go test -race ./internal/door` green
twice uncached; Opus validation ACCEPT on every acceptance line.
