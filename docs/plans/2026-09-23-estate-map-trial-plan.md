# Estate map read-only trial slice: execution-grade plan

> Goal (mk, 2026-09-23): plan the read-only trial slice of the Autarch estate map from
> `docs/brainstorms/2026-09-22-autarch-attention-map-brainstorm.md` (rulings 1–16).
> Revision 2 (2026-09-23) folds in governed plan review round 1 (review-fable, verdict FAIL,
> 18 findings). Each change cites its finding as `[R1-n]`.
> Revision 3 folds in round 2 (FAIL: 2 new P1, 5 P2, 3 P3), cited as `[R2-n]`.
> Revision 4 folds in round 3 (FAIL: 1 new P1, 3 P2, 3 P3), cited as `[R3-n]`.
> Revision 5 folds in round 4 (PASS-WITH-FIXES: 2 P2, 4 P3), cited as `[R4-n]`.
> Revision 6 folds in the independent cross-lab review mk requested (review-astra, gpt-6-astra
> via the BB account pool; FAIL: 9 P1, 8 P2), cited as `[A-n]`. It also records two mk rulings
> made on 2026-09-23 in response: **the kill rule is binding, with a manual flip** (`[mk-K]`),
> and **a catch needs a reason and is deduplicated** (`[mk-C]`).
> Revision 7 folds in Astra round 2 (FAIL: 5 P1, 5 P2; 8 of 17 prior findings resolved, and
> both mk rulings accepted), cited as `[A2-n]`.
> Revision 8 folds in Astra round 3 (FAIL: 5 P1, 2 P2; 7 more resolved), cited as `[A3-n]`.
> Revision 9 folds in Astra round 4 (FAIL: 2 P1; 13 more resolved), cited as `[A4-n]`.
> Revision 10 folds in Astra round 5 (**PASS-WITH-FIXES**: 1 P2, 1 P3), cited as `[A5-n]`.

## Context

mk runs about 98 BB-visible gardens (discovery finds about 180 under the four roots [A-5]) and wants one BB page: an estate map beside an
escalation rail. A six-agent review returned the design as *risky*. Almost every P0 lives in
dispatch and bootstrap, and mk ruled that the map must **earn its place in a two-week trial**
(ruling 11, the caught-something log). This slice builds only what the trial needs, read-only:

- the attention lens, plus blocker edges drawn automatically, only where their provenance is
  known (ruling 9)
- a read-only rail
- confidence encoding (known, guessed, unknown)
- the catch-up briefing
- the "map caught this" log

Rulings enforced by construction:

- **No action surfaces.** There is no dispatch, bootstrap, composer, answer or approve.
- **Deep links only.** Every rail item links into BB, where acting stays BB's job.
- **Overlay rule (ruling 12):** `Build` writes nothing.
  - The only writes are these mk-initiated personal-state commands, and each writes only
    mk-owned files under `~/.autarch` [A2-9]:
    - `estate visit`
    - `estate catch`
    - `estate catch reject`
    - `estate trial --start`
    - `estate trial --close`
    - `estate trial --override`
  - The plugin writes no BB setting on its own [R1-6].

## Decision context (routing)

```json
{"reasons":["unresolved-success-criteria"],
 "rationale":"The map's value is decided by a 2-week user-evidence trial with a kill rule; the slice is read-only and changes no authority boundary",
 "domain":"agent systems / product","investigation_active":false}
```

- **Author:** claude-opus-5-5.
- **Review:** `ic --json route dispatch --role=plan-review --producer-identity=claude-opus-5-5`.
  - Round 1 was requested on Astra by mk. Astra had no quota (`quota_exhausted`, reset
    2026-09-26 02:36), so the round ran on the policy's default seat, review-fable.

## Architecture (one aggregator, one renderer)

```
autarch estate --json   (Go; the single aggregator, testable, also usable from the TUI later)
   ├─ BB:        bb thread list --json; bb thread interactions list <id> --json (pending only); bb project list --json; bb host list --json
   ├─ beads:     bd -C <tracker> --readonly --sandbox list --json --limit 0 --status open,in_progress,blocked   (one call per tracker, 3 s each, 4 workers)
   ├─ rig health:~/.claude/health/*.json  status=fail, evaluated, not stale → blocker
   ├─ git:       door.IndexSessions + door.Movements (briefing), git log -1 (untouched)
   ├─ graph:     door.GraphSource (layer-view WI-1/2): placement only
   └─ edges:     beads cross-garden blocked_by  +  go.mod require of a mistakeknot module
        ↓  JSON contract autarch.estate/v1 (versioned; one shared fixture checked by Go and Zod)
BB plugin (integrations/bb-plugin-autarch): host entry runs `autarch estate …` via execFile
   → server RPC → navPanel page (SVG map + rail + briefing), React text only
```

The Go CLI reads BB, rather than the plugin reading `bb.sdk` directly, for three reasons:

- There is one model with one set of tests.
- It keeps parity with the TUI door.
- The only cost is a snapshot about 0.3–2 s old, which is fine for a pull page that refetches
  on realtime events.

## Work items

Rules for every Go WI:
- Package `internal/estate` (new) unless stated otherwise.
- Follow the door's threads pattern: options struct, `With…`, a bounded worker pool, and fakes
  on PATH as in `internal/door/sessions_test.go`.
- `go test -race`.
- Every exec has a context timeout and an output cap, matching `door.ReadProductBacklog`
  (`internal/door/product.go:244`, 5 s and 4 MiB) [R1-18].
- Every source failure becomes an `unchecked` `SourceStatus`, never a crash and never a silent
  omission.
- For `bb … --json`, output shaped as `{"ok":false,…}` instead of an array counts as that
  source's failure [R1-15].

### WI-0: Prerequisite, layer-view WI-1 and WI-2 (existing plan)

Execute `docs/plans/2026-09-03-layer-view-plan.md` WI-1 (`internal/door/graph.go`: `GraphSource`,
`HTTPGraph`, `LoadGraphEnv`) and WI-2 (`internal/door/layers.go`: `ReadLayers`, `MatchGarden`)
exactly as written. Its closed decisions stay binding: HTTP only, never the live service in
tests, and nothing written.

**Acceptance:** `go test -race -run 'Graph|Layers' ./internal/door`.

**Door changes this plan adds (with tests, in `internal/door`) [A-1, A-2, A-9, A-15]:**
- `NormalizeName`, exported
- `IndexSessions(ctx, …)` returning a skipped count
- the git runner uses `GIT_OPTIONAL_LOCKS=0`, `CommandContext` with `WaitDelay`, and an output
  cap

The existing door callers are updated, and their tests still pass.

WI-3 to WI-5 of that plan (the TUI layer screen) are **not** required. This plan reuses that
plan's writes-nothing *technique* but not its test, because the test lives in its excluded
WI-3 [R1-18].

### WI-1: `internal/estate/model.go`, the contract types

**Wire format [R2-5].** The structs below omit tags for brevity. The contract is:
- Every field has a `json:"snake_case"` tag, as in the layer plan (for example
  `bb_project_ids`, `last_touched`, `read_at`).
- Times are RFC 3339 strings.
- `Item.Age` is serialized as `age_seconds` (an int), and `SourceStatus.Took` as `took_ms`
  (an int), through custom field types or marshal methods.
- The schema file pins these names. The fixture must use them.

```go
const Schema = "autarch.estate/v1"

type Snapshot struct {
    Schema   string
    ReadAt   time.Time
    Window   Window
    Briefing Briefing
    Gardens  []Garden
    Items    []Item
    Edges    []Edge
    Sources  []SourceStatus
    Trial    TrialState
}

type Window struct { Since time.Time; Source string; Err string }  // from door.Window's (since, source, err) [R1-8]

type Briefing struct {                                              // [R1-8]
    Complete bool             // see WI-3; gates advancing the visit watermark [A2-5]
    Gardens  []GardenMovement // only gardens that moved in the window, newest Latest first
    Updated  []string         // Item IDs whose known Age says they were created/updated since Window.Since [A-11]
    Partial  []string         // garden Keys whose movement read failed (GardenMovement.Err) [A-9]
    SessionsSkipped int       // transcript files/dirs IndexSessions could not read [A-9]
}
type GardenMovement struct { Garden string /* Key [R4-6] */; Commits int; Subjects []string; Dirty int; Sessions int; Latest time.Time; Err string }

type Garden struct {
    Key, Name, Root   string
    BBProjectIDs      []string   // union of every BB project mapped to this garden [R1-4]
    Layer, Ecosystem  string
    Placement         Confidence
    Agents            *int       // non-archived BB threads with status "active" mapped here; tmux not counted [R1-16]; nil when BB is unchecked [A-9]
    LastTouched       *time.Time
    Untouched         Confidence
}

type Confidence string // "known" | "guessed" | "unknown"

type Item struct {
    ID, Kind, Garden, Title, Detail, Source string
    Age   *time.Duration // nil = unknown; never 0-as-unknown [A-11]
    Link  Link
}   // every item in a snapshot is current; there is no Live flag on items [A-4]
type Link struct { Kind string; Ref string; Tracker string; WorkDir string } // Kind: "bb-thread" | "bead" | "health"; beads: Tracker = binding identity [A-6], WorkDir = referring checkout used for bd -C [A4-1]

type Edge struct { From, To, Provenance, Ref string; Active bool } // From/To are garden Keys [R4-6]; Provenance: "beads:blocked_by" | "gomod:require"; Active: see WI-3 [A-4]

type SourceStatus struct { Name string; OK bool; Err string; Took time.Duration; Coverage string }

type TrialState struct {
    Verdict            string     // not-started | running | ended:awaiting-review | keep | cut:rail-only [mk-K]
    Start, End         *time.Time
    Day, Days          int
    Catches            int        // eligible, deduplicated catches in [start, end) [mk-C]
    Rejected           int
    ClosedAt           *time.Time
    Override           *Override  // set only by `trial --override` [mk-K]
    MapEnabledExpected *bool      // false after a closed cut:rail-only without override [mk-K]
}
type Override struct { ADR, Reason string }
```

- **Garden identity [R3-1]:**
  - `Name` is the directory basename, from `door.DiscoverProjects`.
  - `Key` is `<root-alias>.<name>`, and it is unique and stable across roots. The root aliases
    are `projects`, `core`, `interverse` and `checkouts` for the four default roots. An
    `estate.yaml` `roots:` entry may name its own alias.
  - **Alias derivation [R4-1]:**
    - A root's alias is its explicit alias if one is given, otherwise the basename of the root
      path. `--root P` flags always use the basename.
    - Two roots with the same alias are a config error: `estate: UNCHECKED alias collision`,
      exit 2.
    - `door.DiscoverProjects` is called **once per root**, so every garden knows the root it
      came from. Deduplication across roots uses the symlink-resolved `Root`, and the first
      root in list order wins. So `interverse/interlens`, a symlink to
      `projects/linsenkasten`, stays `projects.linsenkasten`. Dropped duplicates are counted
      in coverage.
  - `door.Movement` carries `Root`. It is mapped to a Key by `Root` [R4-6].
  - Keys must match `^[A-Za-z0-9._-]{1,64}$`. A garden whose key does not match is dropped
    from the map and counted in coverage.
  - `catch --garden` takes a `Key`.
  - Today the four default roots share 10 names (for example `interflux` under both
    `projects` and `interverse`). **Any name-based match that hits more than one garden
    attributes nothing, unless `estate.yaml` `prefer: {<name>: <key>}` picks one.** That
    covers the hub label, the go.mod `<m>` and graph placement.
  - Estate does its own **collect-all** normalized match over gardens. WI-0 adds one exported
    helper, `door.NormalizeName(s string) string`, a wrapper over the layer plan's unexported
    `normalizeName`, so both packages share one normalization [A-15]. Estate never calls
    `door.MatchGarden`, which returns only the first match and so cannot detect ambiguity
    [R4-3]. Ambiguous hits are counted in coverage as `ambiguous: N`.
  - **Path matching** (threads and go.mod files) compares cleaned absolute paths on a separator
    boundary. `/p/foo` is under `/p/foo` and `/p/foo/x`, and never under `/p/foobar` [A-3].
- **Placement confidence [R1-18]:**
  - **known:** the raw garden name equals a graph row's `project` exactly, before any
    normalization. `designation` is not used, because `MatchGarden` has no normalized
    counterpart for it [R3-6].
  - **guessed:** no exact match, but exactly one graph row matches under `door.NormalizeName`
    [A-15].
  - **unknown:** no row, or more than one. The garden renders in an "unplaced" band, never in a
    guessed cell.
  - **Duplicate graph rows** for one project collapse when their layer and ecosystem agree. When
    they conflict, placement is `unknown` and coverage counts `conflicting rows: N` [A-15].
- **`Item.Kind`** is `question | approval | blocker`.
- **`Item.Garden`** is a real garden key, or one of the pseudo-gardens `hub` or `rig`, or `""`.
  An empty garden means the item could not be attributed; it still appears in the rail and is
  never dropped [R1-3, R1-4, R2-9].
- **Zod enums [R2-6]:** only Autarch-owned vocabularies are enums: `Item.Kind`, `Confidence`,
  `Link.Kind`, `Edge.Provenance` and `TrialState.Verdict`. Values from BB, such as the approval
  subject kind, stay free strings, so a new BB value cannot kill the page.
- **Item identity [A-6].** `Item.ID` is namespaced, stable across snapshots, and never
  truncated:
  - `bb:<threadId>:<interactionId>` for a pending interaction
  - `bb:<threadId>:error` for an errored thread
  - `beads:<trackerKey>:<issueId>`, where `trackerKey` identifies the **physical tracker**,
    independent of garden attribution [A2-2]:
    - `hub` for `/home/mk/hub`
    - otherwise `t-` plus the first 10 hex characters of the SHA-256 of the tracker identity
      (see `ReadBeads`)
  - `health:<check>`

  IDs must match `^[A-Za-z0-9._:-]{1,160}$`. An item whose ID would not match is dropped and
  counted in coverage. Items are deduplicated by ID. A bead's `Link` carries `Tracker`, the
  binding identity (`dolt://host:port/db`, or a canonical local path), and `WorkDir`, the
  absolute referring checkout, so an issue ID is never ambiguous across trackers [A5-2].
- **Display text only is truncated [A-6].** `Title` and `Detail` are plain text, truncated to
  500 runes on a rune boundary, and never carry markup. IDs, `Ref`s, `Tracker`s, `Root`s and
  Keys are never truncated.
- **`Snapshot.Clone()`** deep-copies every slice and pointer, as the concurrency rule requires.
- **Contract files [R1-14, A-14]:**
  - `docs/reference/estate-v1.schema.json`
  - **Generated golden snapshots, not one hand-written fixture.** `TestContractGoldens` runs
    `Build` over fakes in four states: empty, partial (some sources unchecked), all-failed,
    and populated (edges, ambiguity, nullable fields, and a bead whose `tracker` is a Dolt
    binding with a distinct absolute `work_dir` [A5-2]). It writes each result to
    `integrations/bb-plugin-autarch/test/fixtures/estate-<state>.json` under `-update`, and
    otherwise diffs against the committed files. Go drift therefore fails a Go test.
  - **Goldens are reproducible [A2-10, A3-6]:**
    - `Options.Now` is fixed at `2026-01-01T00:00:00Z`, and durations come from a fake clock.
    - A test-only `Options.PathRewrite` maps each temp root to `/fixture/<alias>` **at
      discovery time, before any tracker binding, hash or ID is derived**, so path-derived
      `trackerKey`s are stable.
    - **Every serialized array is sorted deterministically:**
      - `Gardens` by Key
      - `Items` by rail group, then Age (nil last), then ID
      - `Edges` by From, To, Provenance and Ref
      - `Sources` by Name
      - `Briefing.Gardens` by Latest descending, then Key
      - `Briefing.Updated` and `Briefing.Partial` lexically
    - A test generates the goldens in two different temp roots and asserts byte-identical
      output.
  - Each golden validates against the JSON schema in Go. **The Vitest Zod test parses every
    golden**, so real Go output, not a lookalike, is what the Zod mirror accepts.
  - **Null and empty rules:**
    - Every slice is initialized non-nil and marshals as `[]`, never `null`.
    - Nullable fields (`LastTouched`, `Agents`, `Age`, `Start`, `End`, `ClosedAt`, `Override`)
      marshal as JSON `null` and are never omitted.
    - No field uses `omitempty`.
  - The Zod mirror is `.strict()`. An unknown or missing field is a mismatch, so adding a field
    requires a `v2` schema string. Negative tests cover a missing field, a wrong type, an extra
    field and a wrong `schema` string.

### WI-2: `internal/estate/sources.go`, the readers (one function per source, each independently fake-able)

**`~/.autarch/estate.yaml` schema [A-16].** It is optional, and every key is optional:

```yaml
roots:                     # present → replaces the defaults; absent → the defaults below
  - {path: ~/projects, alias: projects}
trackers:                  # absent → hub + every discovered repo with .beads
  mode: add                # add | replace   (replace = hub + exactly these paths)
  paths: [~/projects/foo]
health: {ic-provenance: core.intercore}   # check name → garden Key
prefer: {interflux: projects.interflux}   # ambiguous name OR remote → garden Key
bb: {host_id: host_pda34naxgq, include_personal: true}
```

- A leading `~` is expanded. Every path must then be absolute.
- A `prefer:` or `health:` target that is not a discovered Key is a config error.
- An unknown key, a malformed YAML file or an invalid value is a **config error**:
  `estate: CONFIG <message>`, exit 2. That covers `--since` values that `door.Window` rejects
  (it returns a zero time with an error; `visit.go:68`).
- Config errors are distinct from source failures. A source failure is only ever an
  `unchecked` `SourceStatus`.

**Roots and host [R2-4].** Garden roots come from `estate.yaml` `roots:`. The default is:
- `~/projects`
- `~/projects/Sylveste/core`
- `~/projects/Sylveste/interverse`
- `~/bb-thread-migration-20260921/checkouts`

`door.DiscoverProjects` yields the direct children of each root. `--root` flags replace the
list. The plugin passes no `--root`, so `estate.yaml` or the default governs. The local host
ID is the `bb host list --json` entry whose `name` equals `os.Hostname()` (today `zklw` →
`host_pda34naxgq`), and it can be overridden with `estate.yaml` `bb: {host_id: …}`. If
neither resolves, rule 1 below is skipped and coverage says so.

**`ReadBB(ctx, run Runner, includePersonal bool)`**
- Drop threads with `archivedAt` or `deletedAt` set.
- **Personal threads [R1-4, R2-1]:** the plain `bb thread list --json` already includes them.
  There is no such flag as `--include-personal`. They are kept by default. `estate.yaml`
  `bb: {include_personal: false}` drops them client-side, on
  `environmentProviderId == "personal-workspace"`.
- List interactions only for threads where `hasPendingInteraction` is true, and keep only
  records with `resolvedAt == null` [R1-5].
- **Mapping, from `packages/domain/src/pending-interactions.ts` [R1-5]:**

  | Source | Kind | Title |
  |---|---|---|
  | `payload.kind == "user_question"` | question | `payload.questions[0].prompt`; Detail adds `+N more questions` when there are several. **One item per interaction**, not per question [A-10] |
  | `payload.kind == "approval"` | approval | `approval: <subject.kind>` |
  | `payload.kind == "plugin"` | question | `payload.title` |
  | any other `payload.kind` | question | detail reads `unknown interaction kind <k>` |
  | thread `status == "error"` | blocker | — |

  Approval `subject.kind` today is one of `command | file_change | permission_grant | plan | tool_use`.
  It reaches the contract only inside `Title`, and it is not an enum (see WI-1) [R2-6].

- **Timestamp units [A-10]:** BB timestamps are epoch **milliseconds**, beads `updated_at`
  is RFC 3339, and health `ran_at_epoch` is epoch **seconds**. Each reader converts
  explicitly, and a test pins each unit.
- **Thread activity** is the **newer** of the valid `latestAttentionAt` and `updatedAt`
  values. An idle→active transition updates only `updatedAt` (`threads.ts:2157`), so taking
  the newer one keeps resumed work from looking neglected. It is used for Untouched (always
  `guessed` confidence) and for errored-thread Age [A2-6].

**Mapping threads and BB projects to gardens [R1-4], in precedence order:**
1. The thread's `environmentHostId` is the local host and its `environmentPath` (resolved with
   `filepath.EvalSymlinks` when it exists [R4-5]; otherwise the rule falls through to 2) is at or under
   a garden `Root`. That garden wins; the longest root prefix takes it.
2. Otherwise, match the normalized `gitRemoteUrl` of the thread's project against every
   garden's remote, collecting all hits [A-3]. Normalization:
   - convert `git@host:owner/repo` to `host/owner/repo`
   - strip the scheme, `user@` and a trailing `.git` or `/`
   - lowercase the host and owner, keeping the repo name's case

   Exactly one hit takes the garden. Several hits are ambiguous: the item stays unattributed
   unless `prefer:` names one of them. About 14 remotes are shared today (for example
   `projects.interbrowse` and `interverse.interbrowse`). BB projects union into a garden's
   `BBProjectIDs` only when their remote match is unambiguous or preferred.
3. Otherwise the item gets `Garden: ""`.
- Names are never matched on their own.
- Coverage reports the counts, for example `bb: 41 threads → 29 gardens, 12 unattributed`.

**`ReadBeads(ctx, trackers []string, run Runner)` [R1-2, R1-3]**
- Trackers: `/home/mk/hub` plus every discovered repo with its own `.beads`, plus any
  configured `trackers.paths`.
- **Physical tracker identity [A2-2]:**
  - Follow `.beads/redirect`, if present, to the target directory.
  - **The identity is the resolved backend binding [A3-1]:**
    - For a server-backed tracker, it is `dolt://<host>:<port>/<dolt_database>`. The host and
      port come from `metadata.json` `dolt_server_host`, or `127.0.0.1` when that is null, and
      from `.beads/dolt-server.port`.
    - For an embedded or local store, it is the canonical (symlink-resolved) path of the
      resolved beads directory.
    - `project_id` is **supporting metadata only**. It never merges trackers on its own.
  - Trackers with the same binding are **one** tracker, queried once, and its issues appear
    once.
  - **Binding identity and command directory are separate [A4-1].**
    - The binding is only an identity.
    - `bd -C <dir>` always runs from a **referring checkout**: the lexically first
      symlink-resolved garden root (or configured path) whose `.beads` resolves to that
      binding. It never runs from the redirect target, which can resolve differently (from
      `tracker-binding-uncrancher` itself, `bd where` reports `/home/mk/.beads`). The hub runs
      from `/home/mk/hub`.
    - Before querying, the reader runs `bd -C <dir> --readonly --sandbox where --json` in the
      same worker. If the reported tracker path differs from the redirect-resolved
      expectation, that tracker is **unchecked** (`resolution mismatch`) and nothing is
      attributed to it.
    - Every bead `Link` carries `Tracker` (the binding) and `WorkDir` (the referring
      checkout). The rail's copy action is `bd -C <WorkDir> show <id>`.
  - Trackers that share a `project_id` but have **different** bindings stay distinct and are
    each queried. Coverage counts them as `same-project distinct stores: N`, and nothing is
    merged. Example: `ops` (port 44267) and `auraken-ops` (port 45293).
  - Genuine aliases: `uncrancher-scene` and `uncrancher-claude-mod` both redirect to
    `tracker-binding-uncrancher`.
  - The gardens that reference a tracker are its **referrers**.
- Each tracker has a 3 s deadline covering both of its calls, and 4 trackers run at once
  [A5-1].
- Every call is `bd -C <t> --readonly --sandbox …`, matching `product.go:268`. `--global` is
  never used.
- **Two calls per tracker, sharing one 3 s tracker deadline [R2-2, A5-1]:** the `where`
  resolution preflight, then the `list` call below. `list` gets only the time remaining, and
  it does not run if resolution failed or mismatched. Both calls sit inside the pool deadline.
  The `list` call is:
  `bd -C <t> --readonly --sandbox list --json --limit 0 --status open,in_progress,blocked`.
  Everything below is derived client-side from that one result. The hub returns about 493 rows
  in 0.21 s.
  - **Blockers:** issues whose **stored** `status == "blocked"`. Dependency readiness is not
    computed [A-10].
  - **Approvals:** open or in-progress issues whose `labels` include `decision-gate` or
    `human-in-the-loop`. The same ID is never emitted twice; approval wins.
  - **Dependencies:** each issue's `dependencies[]` entries `{issue_id, depends_on_id, type}`
    with `type == "blocks"` are kept for `ReadEdges`. The upstream issue is usually in the same
    result set. If it isn't (it may be closed, deferred, hidden or in another tracker), the
    endpoint is **unresolved** and counted as `unresolved deps: N`. It is not labelled
    cross-tracker [A-10].
- **Age** comes from `updated_at`.
- **Age for the other sources [R3-4]:**
  - a BB interaction: its `createdAt`
  - an errored thread: the thread-activity timestamp above [A2-6]
  - a health item: `ran_at_epoch`

  If the source value is absent or in the future, `Age` is `nil`. The rail shows "age
  unknown", the item sorts last in its group, and it is never counted as updated [A-11].
- **Attribution:**
  - A tracker with exactly **one** referrer garden: its issues belong to that garden.
  - A tracker with **several** referrers, or a configured path outside every garden: an issue
    belongs to the one referrer, or the one garden, that its labels name after `prefer:` is
    applied. Otherwise it is unattributed (`""`) and counted as `shared-tracker: N` [A2-2].
  - A hub issue belongs to the garden whose name equals one of its labels, following the
    door's `--label <project>` scoping (`product.go:269-276`). If no label matches, or if its
    labels match **more than one** distinct garden after `prefer:` is applied, it belongs to
    the pseudo-garden `hub` and is counted as `ambiguous` [A-3]. Hub issues are never
    dropped.
- Keep every issue's garden attribution, not only emitted items', so that the edge reader can
  place upstream issues.

**`ReadHealth(ctx, dir string, now time.Time)` [R1-12, A2-3]**
- Use `Lstat` and read only regular files of at most 256 KiB. Symlinks, FIFOs, devices and
  oversized files are skipped and counted.
- Skip files that have no `check` field (for example `facts.json`).
- A file that fails to parse becomes a `SourceStatus` error.
- An item is emitted only when `status == "fail" && evaluated == true && now < ran_at_epoch + 2*interval_seconds`.
- A failed record that has gone stale, or that was not evaluated, is counted in coverage. It
  is not emitted as an item.
- **Garden attribution** comes from the static map `health:` in `~/.autarch/estate.yaml`.
  Unmapped checks go to the pseudo-garden `rig`.
- Deleting `estate.yaml` resets every preference to its default [R2-9]:
  - health attribution falls to `rig`
  - `roots:` and `trackers:` return to their defaults
  - `include_personal` returns to true
  - the host ID is resolved from the hostname
  - `prefer:` empties, so ambiguous names attribute nothing [R4-4]

  None of that makes the world wrong, so it passes the deletion test.

**`ReadGraph(ctx, src door.GraphSource, gardens)`**
- It calls `src.Query` itself (the letter queries of the layer plan's WI-2) and does its own
  collect-all matching. It does **not** call `door.ReadLayers`, which also launches movement
  reads [A-2].
- It provides placement and **nothing else** [R1-1].
- Its deadline is **5 s** of the 8 s budget [R1-11].

**`ReadEdges`: `ScanGoMod(ctx, gardens)` during the go.mod window, plus `BeadsEdges(beadsResults, gardens)`, a pure function called at merge [R1-1, A2-3, A4-2]**
- `go.mod` reads follow the same regular-file rule, with a 256 KiB cap, and check `ctx`
  between files.
- CanonGraph `serving_map` rows are `{project, url, port, unit, machine}`: project→machine,
  with no garden→garden edge. They are therefore **not** an edge source.
- The two known-provenance sources are:
  - **`beads:blocked_by`:** a `dependencies[]` entry with `type == "blocks"` says issue A (in
    garden X) depends on issue B (in garden Y). When X ≠ Y and both are real gardens (not
    `hub`, `rig` or `""`), this yields the edge `Y → X`, with `Ref` set to `B→A`.
    - The edge is only drawn when both issues are in the same tracker result. Cross-tracker
      dependencies are counted in coverage, not drawn.
    - There are no extra calls [R2-2].
  - **`gomod:require`:** scan each garden's `go.mod` files to a depth of 2 below the root,
    excluding `vendor/`, `node_modules/`, `testdata/` and `.worktrees/`.
    - Do not descend into any subdirectory that has its own `.git`. That directory is a
      different garden, or no garden at all.
    - Attribute each go.mod file to the garden with the longest root prefix, the same rule
      used for threads. Each edge is emitted once. [R3-2]
    - `<m>` is the first path segment after `github.com/mistakeknot/`, matched
      case-insensitively against garden names. For example, `interbase/go` matches
      `interbase`, and `typhon/cmd/typhon-tui` matches `Typhon`.
    - A self-reference is ignored.
    - A match yields the edge `m → garden`, with `Ref` set to the go.mod path [R2-3].
- **Expected coverage is near zero.** A survey on 2026-09-23 found one cross-garden go.mod
  edge: `Clavain/cmd/clavain-cli` requires intercore, and intercore is a garden only through
  the `Sylveste/core` root. The live hub `blocks` dependencies found so far stay inside the
  campaign garden. Coverage states it plainly, for example
  `edges: 1 gomod, 0 beads — no graph edges`.
- **The expected path is the untested clause.** If a live run yields zero edges, or so few
  that no blocker has an edge, the cascade catch category is recorded as *untested* in the
  trial ADR. It is not counted for or against the map.

**Untouched**
- Take the newer of the last commit (`git -C <root> log -1 --format=%ct`, 2 s timeout, run
  inside the same per-garden worker as movement) and the garden's latest non-archived thread
  activity.
- `Movement.Latest` is not reused, because it is bounded by the window. It is used only for
  the briefing [R1-11].
- **Confidence follows the timestamp actually chosen [R3-5, A-9]:**
  - **known** when the chosen (newer) timestamp is a commit and the BB source was checked.
  - **guessed** when the chosen timestamp is thread activity (perception UNTOUCHED-001), or
    when a commit was chosen but BB was unchecked, because a newer thread may be missing.
  - **unknown** when neither exists, or when git failed for that garden. `LastTouched` is then
    nil.
- **Git is read-only for real [A-1].**
  - Every git exec in estate and in the door code it reuses (`GitMovement`, `briefing.go:81`
    and `:110`) runs with `GIT_OPTIONAL_LOCKS=0`, so `git status` never refreshes `.git/index`.
  - WI-2 adds that to the door's git runner, along with `exec.CommandContext`, a `WaitDelay`
    of 1 s and a 1 MiB output cap in place of uncapped `Output()` [A-2].

### WI-3: `internal/estate/build.go`, `Build(ctx, Options) Snapshot`

- Fan out all the sources concurrently. The total budget is **8 s**.
- **Per-source deadlines [R1-11]:**
  - graph 5 s
  - BB 4 s
  - beads 3 s per tracker, inside a 6 s pool deadline
  - health 1 s
  - git 5 s pool
- A source that is late becomes `unchecked`, and **partial results are kept** [A-2]:
  - trackers that finished are used
  - trackers still running at the pool deadline are cancelled and reported as `failed: N`
  - trackers never started are reported as `skipped: N`
- **Every reader is cancellable [A-2]:**
  - Every exec uses `exec.CommandContext` with `WaitDelay` of 1 s, so a grandchild holding
    the pipe cannot hang `Wait`.
  - **Descendant cleanup [A2-3, A3-3]:**
    - Each child starts in its own process group (`SysProcAttr.Setpgid`).
    - `Cancel` sends SIGKILL to the whole group (`-pgid`).
    - **After `Wait` returns on every path** (success, error, `ErrWaitDelay` or cancellation),
      the runner also sends SIGKILL to `-pgid`, ignoring `ESRCH`. A grandchild that outlived a
      cleanly exiting parent is killed too, not just its pipes closed (`os/exec` does not call
      `Cancel` after a normal exit).
    - Test: the parent exits 0 immediately, a grandchild sleeps 30 s holding stdout, and the
      test asserts via `kill(pid, 0)` that the grandchild is gone within 2 s.
  - **Local reads [A2-3, A3-2]:**
    - `LastTurn` keeps its existing 256 KiB tail (`transcript.go:26`). WI-2 changes it to
      read through `io.LimitReader(f, tail)` after the seek, so a file that grows during the
      read is not followed past the cap.
    - Every local read (transcripts, health, go.mod and discovery) runs on a worker
      goroutine. At its deadline the result is **abandoned** and counted as skipped.
    - `Build` never waits on abandoned work. **`autarch estate` is a one-shot process**: after
      it prints and exits, the OS reclaims abandoned goroutines and any blocked I/O. The plugin
      always runs it as a subprocess, never in-process, so abandoned work cannot accumulate.
    - Special files are rejected with `Lstat` before opening.
  - Output is capped. Over the cap, the reader stops reading, kills the process and reports
    `output cap`.
  - WI-2 changes the signature to
    `door.IndexSessions(ctx context.Context, root string, projects []Project, since time.Time) (idx map[string]SessionStat, skipped int, err error)` [A3-2].
    `ctx` is checked between directory entries, and existing callers pass
    `context.Background()` [A2-3].
- **Dependency-aware schedule, 8 s wall time from `ReadAt` [A2-3, A3-2].** Times are absolute
  deadlines measured from `ReadAt`:

  | Window | Stage |
  |---|---|
  | t = 0–1 s | **discovery**: roots, `.beads` redirects and bindings. It runs on a worker with a 1 s deadline. A root that has not returned is marked unchecked and its gardens are absent. |
  | t = 1–7 s, concurrently | graph (to 6 s), BB (to 5 s), beads pool (to 7 s), health (to 2 s), untouched `git log -1` pool (to 5 s) |
  | t = 1–3 s, then 3–7 s | transcripts, then git movement, which needs the transcript index. If transcripts time out, movement runs with a nil index. |
  | t = 1–4 s, concurrently with the above | **go.mod scan**, independent of beads [A4-2] |
  | t = 7–8 s | **reserved** for process-group cleanup, then merge. At merge, **beads edges are derived from every accepted tracker result**, and then active-edge traversal runs. No reader may run past 7 s. |

  The root context is 8 s. The full-`Build` timing test uses fakes that each sleep to their
  deadline, and asserts completion in under 8.5 s and that every stage's result was merged in
  its window. **A tracker that finishes at t = 5 s** (after the go.mod scan, before its own
  deadline) supplies both a blocker and a cross-garden dependency, and both appear in the
  snapshot with the edge active [A4-2]. Separate tests cover a stalled discovery and a transcript that grows during the
  read.
- **One merger [A-2].** Readers send results on channels to a single goroutine, the only one
  that writes the snapshot. `Build` returns a value built after every reader has returned or
  been cancelled. No reader holds a reference to it.
- **Window [A-12]:** `since, source, err := door.Window(opts.Since, door.DefaultVisitPath(), now)`.
  An error is a config error (WI-2). The page freezes `since` for its mount (WI-5).
- **`ReadAt` is the read-start watermark** [A2-5]: it is captured before any reader starts.
- **Briefing [R1-8, A-9]:** `idx, skipped, err := door.IndexSessions(ctx, transcriptsRoot, projects, since)`,
  then `door.Movements(ctx, projects, since, idx, …)`.
  - Transcripts do feed the `Sessions` count.
  - The skip count is reported in `Briefing.SessionsSkipped`. The door function returns it in
    place of today's silent skip (`briefing.go:196`), and an error becomes a `SourceStatus`.
  - A `Movement.Err` is kept in `GardenMovement.Err`, and that garden's Key is listed in
    `Briefing.Partial`. It is never silently dropped.
  - `Briefing.Updated` lists items whose non-nil `Age` is at most `ReadAt - Since`. The page
    labels this "updated since your last visit", not "new" [A-11].
  - **`Briefing.Complete` (a bool) [A2-5, A3-5]** is true only when every source that feeds
    the briefing or the rail was fully read:
    - discovery returned for every root
    - git movement read every garden (`Partial` is empty)
    - `IndexSessions` finished with `skipped == 0` and no error
    - BB was checked
    - every beads tracker was checked
    - health was checked, with no parse errors

    Only `graph` (placement) and the go.mod edge scan are exempt, because neither contributes
    to "updated since".
- **Edges (ruling 9) [A-4]:**
  - `Snapshot.Edges` is the **complete** known-edge set, used as-is by the dependencies lens.
  - An edge's `Active` field is recomputed on every snapshot. Origins are the gardens with at
    least one `blocker` item. Every edge reachable downstream from any origin is marked active
    by a breadth-first walk with a visited set, so cycles terminate and several origins union.
  - When a blocker clears, the next snapshot has no origin there, so its edges become inactive
    again. There is no inference beyond known edges.
- **Deletion-test invariant:** `Build` writes nothing. `TestBuildWritesNothing` works like
  this [R2-10, A-1]:
  - Set `HOME` to a `t.TempDir()` seeded with `.autarch/last-visit`, `.autarch/estate.yaml`,
    `.autarch/estate-trial.json` and `.autarch/estate-catches.jsonl`.
  - Create **two real git repositories** with the real `git` binary: one with a modified
    tracked file and a stale index stat, and one whose git dir sits outside HOME (`--separate-git-dir`).
    `bb` and `bd` stay fakes.
  - Snapshot HOME and both git directories (paths, sizes, mtimes and content hashes, including
    `.git/index`).
  - Run `Build`.
  - Assert that nothing changed and that no new path appeared.
  - A companion test runs the same setup **without** `GIT_OPTIONAL_LOCKS=0` and shows the index
    does change. That proves the test can see the write.
- **Concurrency tests (`-race`) [A-2]:**
  - a slow tracker pool that is partly skipped and partly failed
  - a slow `where` that leaves `list` only the remaining budget, and a failed `where` that
    runs no `list` [A5-1]
  - cancellation mid-read
  - a fake that forks a sleeping grandchild holding stdout
  - output over the cap
  - `Clone` isolation: mutating nested slices and pointers of a clone leaves the original
    unchanged
  - many parallel `Build` calls

### WI-4: `cmd/autarch/estate.go`, the CLI

Register the command after `threadsCmd()` in `cmd/autarch/main.go:74-88`.

**`autarch estate [--json] [--since D] [--root P]… [--graph-env F]`**
- Prints a snapshot. The text mode is a compact attention list, useful without BB.
- Exits 2 with `estate: UNCHECKED` only when **every** source failed.

**`autarch estate visit --at T` [A-12]**
- Writes `~/.autarch/last-visit` via `door.SaveVisit(path, T)`, where `T` is the **snapshot's
  `ReadAt`** that mk acknowledged, not the time Continue was pressed. Events between the read
  and the click therefore show in the next briefing.
- `--at` is required, and a value later than now is refused.
- On failure it prints `{ok:false,error}` and exits 1.
- Ruling 7's seen/fade lifecycle is **deferred**. The trial has only the frozen window and
  the "updated since" list.

**`autarch estate catch --garden G --lens L --category C --reason R --snapshot-read-at T --active-edges N [--item ID]…` [R1-13, R2-9, mk-C]**
- `--category` is one of `cascade | neglect | allocation | movement | other`.
- `catch` is allowed only while the trial is `running`. Otherwise it refuses with
  `not-running` or `closed` [A2-8].
- `--reason` is required, non-blank, and at most 500 characters. It completes the sentence
  "the rail alone would have missed this because…". Click location alone is **not** treated
  as proof of map credit [A-7].
- `--active-edges` is the number of active edges in the acknowledged snapshot. A `cascade`
  catch with 0 is recorded but marked `eligible:false, why:"no active edges"`.
- **Deduplication [mk-C, A-7]:** the key is `(garden, category, local date)`. The file is
  appended under `flock`. A second catch with the same key is **not appended**: the command
  returns `{ok:true, duplicate:true, id}`. So a double click or a retry is idempotent.
- Each line is `{id, ts, garden, lens, category, reason, item_ids, snapshot_read_at, active_edges, eligible, via:"map"}`,
  where `id` is a short hash of the dedupe key. The file is `~/.autarch/estate-catches.jsonl`,
  mode 0600.

**`autarch estate catch list` / `catch reject <id> --why W` [mk-C, A-7]**
- `list` prints every catch with its eligibility.
- `reject` appends `{reject:id, why, ts}` to the same file.
- This is the **adjudication pass**: mk reviews the log for ineligible entries before the
  trial closes.

**`autarch estate trial [--start | --close | --override --adr P --reason R] [--json]` [R1-9, mk-K]**
- **`--start`** creates `~/.autarch/estate-trial.json` `{start, days:14}` with `O_EXCL`, so
  of two concurrent starts exactly one succeeds. It refuses if the file already exists.
- **`--close`** is allowed only when `now >= end`. It computes the verdict and writes
  `{closed_at, verdict, counted_ids, rejected_ids, log_offset}` into the trial file once, where
  `log_offset` is the catch-log byte length at close. It then prints the obligation.
  **The score is frozen:**
  - After a close, `catch` and `catch reject` refuse with `trial closed`.
  - A later `trial` reports only the frozen counts [A2-7].
- **Serialization [A2-7]:**
  - `catch`, `catch reject`, `--start`, `--close` and `--override` all take the same
    exclusive `flock` on `~/.autarch/estate.lock`.
  - Trial-file updates are atomic: write a temp file in the same directory, `fsync`, then
    `rename`. A crash leaves either the old file or the new one.
- **`--override`** is allowed only after a `cut:rail-only` close. It records
  `{adr, reason}`. The ADR path must exist, and the reason must be non-blank.
- **Verdict:**
  - `not-started`: no trial file.
  - `running`: `now < end`.
  - `ended:awaiting-review`: `now >= end` but not closed yet. The chip asks mk to run
    `catch list` and then `trial --close`.
  - `cut:rail-only`: closed with **eligible, non-rejected, deduplicated** catches ≤ 1.
  - `keep`: closed with more than that.
- **Catches** are counted in `[start, end)`. Parse failures are reported and not counted.
- **The kill rule is binding [mk-K].** After a `cut:rail-only` close, `MapEnabledExpected` is
  `false`. Applying it is a trial-completion obligation (WI-6), and it is lifted only by
  `--override` with an ADR.
- A corrupt trial file is a config error: exit 2, and it is never treated as `not-started`
  [A-16].

**Mutation response contract (`--json`), used by every writing command [A2-9]:**

| Outcome | Output | Exit |
|---|---|---|
| success | `{"ok":true, ...}` (see below) | 0 |
| validation or state refusal | `{"ok":false,"error":{"code":C,"message":M}}` | 1 |
| config error | same envelope | 2 |

- The `ok:true` payloads:
  - `visit`: `{"ok":true,"at":T}`
  - `catch`: `{"ok":true,"id":I,"duplicate":bool,"eligible":bool}`
  - `catch reject`: `{"ok":true,"id":I}`
  - `trial --start`: `{"ok":true,"trial":TrialState}`
  - `trial --close` and `trial --override`: the same `trial` payload
- `C` is one of `invalid | exists | not-running | not-ended | closed | not-cut | no-adr | locked | io | config`.
- The host parses only this envelope. Anything else is an error envelope.

**Test clock**
- The hidden env var `AUTARCH_NOW` (RFC 3339) overrides `now` for `trial` and `catch` only.
  It is documented as test-only and read nowhere else.

**Tests**
- Fake `bb`, `bd` and `git` on PATH, plus a fake `GraphSource`.
- The JSON validates against the schema, and the exit codes are asserted.
- Every trial verdict is covered, including:
  - a seeded log record at `ts == end` is not counted, and a live catch at `now == end` is refused [A3-7]
  - a malformed JSONL line
  - a second `--start` refusing, and two concurrent `--start` calls with one winner
  - `--close` before the end refusing
  - `--override` without an ADR refusing
  - a corrupt trial file
- Catch cases [mk-C, A-7]:
  - a double click and a retry are deduplicated
  - the same garden in a different category counts separately
  - a blank reason is refused
  - a `cascade` catch with 0 active edges is ineligible
  - a rejected catch is not counted
  - a catch outside `running` refuses
  - `close` and `reject` racing under the lock (the frozen score is unchanged by a later
    reject, which refuses)
  - a crash simulated between temp write and rename leaves a valid trial file [A2-7]
  - every mutation's JSON envelope and exit code [A2-9]
- Config cases [A-16]: a malformed `estate.yaml`, an unknown key, a `prefer:` pointing at a
  missing Key, and an invalid `--since` all exit 2 with `CONFIG`. `-race`.

### WI-5: `integrations/bb-plugin-autarch/`, the BB plugin (read-only renderer)

Scaffold with `bb plugin new autarch`, then move the result into the repo path.

**Manifest and host entry:**
- The manifest declares `bb.host`.
- `host.ts` uses `experimental_defineHostEntry` with the methods `snapshot`, `visit`,
  `catch`, `catchList` and `trialStart`. Only `trial --start` is reachable from the page;
  `--close`, `--override` and `catch reject` are CLI-only, so mk runs them deliberately.
- Each method runs `execFile(autarchBin, fixedArgv)`: no shell, a 15 s timeout, a 4 MiB
  `maxBuffer`, and an abort on `context.signal` and `context.lifecycle.signal`
  (`host-contract.ts:111`) [A-13].
- **No page-to-host cancellation is claimed [A2-1].** BB's frontend RPC carries only input
  (`rpc-contract.ts:88`, `plugin-service.ts:1881`), so unmounting cannot reach the host.
  Instead, load is bounded on the server:
  - The server keeps **one in-flight `snapshot` host call per `since` value**. Concurrent
    RPCs join its promise, so page churn cannot multiply `autarch estate` processes.
  - Every call is bounded by the 15 s timeout.
  - An unmounted page discards the result.
- **Binary [R1-15, R2-7]:**
  - The plugin setting `autarchBin` defaults to `/home/mk/.local/bin/autarch`, an absolute
    path, because the host daemon's PATH may not include `~/.local/bin`.
  - That file is created by `go build -o ~/.local/bin/autarch ./cmd/autarch` (verification
    step 0) [A2-10]. It does not exist today, and `go install` would target `~/go/bin`.
  - Host entries have no settings API. The server reads `settings.get().autarchBin` and passes
    `bin` in every host-call payload. The host validates it as an absolute path to a regular,
    executable file before calling `execFile`.
  - A missing binary returns an error envelope.
- **Input validation:**
  - `garden` and `lens` match `^[A-Za-z0-9._-]{1,64}$`.
  - Each `item` matches `^[A-Za-z0-9._:-]{1,160}$` [A-6].
  - `category` is from the enum, `reason` is 1–500 characters, and `at`, `snapshotReadAt`
    and `since` are RFC 3339.

**Server and RPC:**
- `bb.rpc.register(contract, …)` proxies to `bb.hosts.experimental_client` with
  `hostId = bb.sdk.system.config().primaryHostId`. If `primaryHostId` is `null`, it returns
  an error envelope and does not throw [R1-15].
- It Zod-parses the snapshot. On a mismatch it returns an error envelope; it never renders a
  partial parse.
- **Realtime [R1-10, A-13]:** the server subscribes to backend events
  `interaction.pending`, `thread.active`, `thread.failed`, `thread.idle`, `thread.archived`
  and `thread.deleted`, and publishes `bb.realtime.publish("estate", {changed:true})`.
  `thread.active` is included because it clears an errored-thread blocker. There is no
  `thread:changed` event.
- **Refresh ordering [A-13]:** the frontend uses `useRealtime("estate", refetch)` plus a 60 s
  poll through **one single-flight fetcher**:
  - A trigger during a fetch sets a `pending` flag, and exactly one more fetch follows.
  - Each response carries a sequence number, and a response older than the last applied one
    is discarded.
  - Unmounting marks the fetcher dead, and a late response is ignored [A2-1].
  - Every snapshot fetch passes the mount's **frozen `since`** [A-12].

**Page (`app.slots.navPanel({id:"estate", title:"Estate", path:"estate"})`):**
- **Briefing overlay first** (orientation before obligation).
  - It renders `Snapshot.Briefing`, including the partial and skipped counts [A-9].
  - "Continue" calls `visit --at <the briefing snapshot's ReadAt>` [A-12], but **only when
    `Briefing.Complete` is true** [A2-5].
    - When the briefing was incomplete, Continue closes the overlay **without** advancing the
      watermark, and the chip reads "visit not saved: incomplete read". The next mount
      re-reads the same window, so nothing unread is acknowledged.
    - If the write fails, the chip shows "visit not saved" until a later complete Continue
      succeeds.
  - The mount's `since` is frozen at the first snapshot's `Window.Since` [A-12].
  - It is shown **once per page mount** and never re-opens on refetch or poll [R1-17].
- **Map layout, `layout(gardens, width, height)`: a pure function, unit-tested [A-5]:**
  - **Columns** are the layer letters in the graph's letter order. They are labelled as
    layers, **not "pace"**, because the letter→speed mapping is unresolved in the layer plan
    (line 170).
  - **Rows** are ecosystem bands, ordered alphabetically. An "unknown ecosystem" band comes
    last among the placed rows, and the **unplaced band** sits at the bottom.
  - **Fixed geometry [A2-4]:**
    - The node pitch is fixed at 28 px, and every band is exactly 4 node rows tall.
    - `cellWidth = max(56, viewportWidth / layerCount)` px, so every cell has at least 2 slots
      per row and `C = 4 × floor(cellWidth / 28) ≥ 8`. With 13 layers, a viewport narrower than
      728 px scrolls horizontally instead of shrinking cells [A3-4].
    - The map scrolls vertically.
    - **The unplaced band** is one cell spanning all columns, with the same 4-row, 28 px
      geometry, a capacity of `4 × floor(mapWidth / 28)`, and the same `+K` rule.
    - **Labels never extend beyond their node [A3-4].** Each node shows a 3-character
      abbreviation clipped to its 28 px square (`clipPath`). The full name is in an SVG
      `<title>` and in the selection card. No free-standing label text is drawn, so grid
      placement really does prevent collisions.
  - **Within a cell**, gardens are sorted by `Key` and fill slots row-major. When a cell has
    more than `C` gardens, the last slot becomes a `+K` chip, and the selection card lists the
    overflowed gardens. **Edges to an overflowed garden terminate at its cell's `+K` chip.**
  - **Stability guarantee, for a fixed viewport width:**
    - A refetch with the same gardens moves nothing.
    - Adding or removing a garden moves only the later-sorted siblings in its own cell.
    - Adding a new ecosystem inserts one band in alphabetical position, which shifts the bands
      below it down by exactly one band height and moves nothing within any cell.
    - A viewport resize reflows the cells, which is expected.
  - **Labels:** see the fixed-geometry rule above. There are clipped abbreviations only [A3-4].
  - Gardens `hub`, `rig` and `""` are not drawn on the map. They appear only in the rail.
- **Encodings:**
  - **Placement:** guessed is a dashed outline; unknown goes in the unplaced band.
  - **Attention (default lens):** a badge showing the garden's item count, coloured by its
    highest rail group. Active edges are drawn as solid arrows. The coverage label ("edges: 1
    gomod, 0 beads") is always visible.
  - **Movement:** a glow ring on gardens in `Briefing.Gardens`. It fades only with the frozen
    window; the seen/fade lifecycle is deferred [A-12].
  - **Allocation (pull):** node radius scales with `Agents` (0, 1, 2–3, 4+). `nil` renders as
    "?".
  - **Dependencies (pull):** every edge in `Snapshot.Edges`, with inactive ones dimmed.
  - **Neglect (pull):** node opacity by days since `LastTouched`, with steps at 14, 30 and 90
    days. `Untouched` confidence reuses the placement idiom: guessed is a dashed ring, unknown
    is hatched grey. The caption reads: "parking is not in the trial. A faded garden you
    parked on purpose is not a catch" [R1-16].
- **Density acceptance:** a 200-garden fixture renders with no overlapping node boxes and
  every garden reachable, as a `layout` unit test plus a jsdom test.
- **Stability tests [A2-4]:**
  - an addition below capacity moves only later siblings
  - an addition that crosses capacity turns the last slot into `+K`, and nothing earlier moves
  - a new ecosystem shifts only the lower bands by one band height
  - an edge to an overflowed garden ends at the `+K` chip
  - widths of 320, 728 and 1440 px: cells never have fewer than 8 slots, a narrow viewport
    scrolls horizontally, and every overflowed garden is reachable through `+K` [A3-4]
  - **browser check** (Playwright, in verification step 4): the rendered bounding boxes of
    node text lie within their node squares, with no overlaps, at 1440 px and at 728 px
- **Rail:** one queue with layer tags (ruling 16), in a fixed order [R1-7]:
  1. `Source == "bb"` and `Kind` is `question` or `approval` (an agent is stalled on mk)
  2. `Source == "bb"` and `Kind == "blocker"` (errored threads)
  3. `Source == "beads"` (blockers and `decision-gate`/`human-in-the-loop` approvals)
  4. `Source == "health"`

  The groups are keyed on `(Source, Kind)` [R3-3].

  - Within each group, sort by `Age` descending, then by `ID`.
  - The tag is `Garden.Layer`: "unplaced" when the placement is unknown, and "hub", "rig" or
    "unattributed" for the pseudo-gardens.
  - Each item's only action is **Open**. For BB items that is
    `useBbNavigate().toThread(id)`; for other items it copies `bd -C <WorkDir> show <id>` for beads, or the health file path [A-6].
  - **Rail Open never selects or highlights a garden on the map** [R1-13].
- **"Map caught this" [mk-C, A-7]:** the button exists **only** on the map's garden
  selection card, and that card opens only from a click on the map.
  - It opens a small form with a required **category** and a required **reason** ("the rail
    alone would have missed this because…").
  - Submit calls `catch` with the lens, category, reason, the garden's item IDs, the
    snapshot's `ReadAt` and its active-edge count.
  - The submit button is disabled while a request is in flight. A `duplicate:true` response
    shows "already logged today".
- **Source strip:** every `SourceStatus`, with coverage and `unchecked` flags.
- **Sidebar accessory:** the count of question and approval items.
- **Trial chip:** reads "trial not started", then "day N/14 · K catches", then "ended:
  review + close", then the verdict [R1-9].

**Rendering rules (S1):** all agent-sourced strings render as React text children. The page
never uses `dangerouslySetInnerHTML`, never uses Markdown for agent text, and keeps no button
inside agent-text containers.

**Setting `mapEnabled`** (boolean, default true). **The plugin never writes it** [R1-6].
- **While the trial is `not-started`**, the map is **not rendered** [A-8]. The page shows the
  rail, the briefing and a "Start the 14-day trial" button, which calls `trialStart`, an
  mk-initiated write to `~/.autarch`. The map cannot run indefinitely outside a trial.
- **If `mapEnabled=false`**, the page opens as rail plus briefing. A "Lenses (seasonal)"
  button renders the map on demand for that mount only [A-8].
- **Catch eligibility follows the trial state, not the setting [A2-8]:**
  - While the trial is `running`, a catch from a seasonal-opened map counts like any other.
  - The catch button is shown only while the trial is `running`.
  - After `ended:awaiting-review` or close, the button is hidden, and the CLI refuses with
    `not-running` or `closed`.
- **The binding cut [mk-K]:** when `Trial.MapEnabledExpected == false` but `mapEnabled` is
  still true, a persistent banner reads "Trial closed: cut:rail-only. Apply it with `bb plugin
  config autarch set mapEnabled false`, or record an override ADR". The map still renders
  until mk acts, because the plugin never writes the setting.

**Tests (Vitest):**
- **Zod:** the Go goldens (below) parse, and a wrong `schema` string is rejected [R1-14,
  A2-10].
- **`createFakePluginHost` RPC:**
  - a valid snapshot
  - a schema mismatch returns an error envelope
  - `primaryHostId: null` returns an error envelope
  - input validation rejects a bad `garden` or `item`
- **`experimental_createHostEntryHarness`:**
  - a fake `autarch` binary, reached through the `autarchBin` setting
  - a missing binary returns an error envelope
  - non-JSON output returns an error envelope
- **Contract:** every Go golden (`test/fixtures/estate-*.json`) parses under the `.strict()`
  Zod mirror. There are negative cases for a missing field, a wrong type and an extra field
  [A-14].
- **Refresh [A-13]:**
  - overlapping triggers produce at most two fetches
  - a stale response is discarded
  - after unmount, a late response is ignored
  - **server:** five concurrent `snapshot` RPCs with the same `since` produce exactly one host
    call [A2-1]
  - a `thread.active` event after a `thread.failed` event clears the blocker
- **`renderSlot` jsdom:**
  - The briefing shows first. Continue calls `visit` with the snapshot's `ReadAt`, not the
    current time, and a refetch does not re-show the briefing.
  - A `not-started` trial renders no map and shows a start button.
  - `mapEnabled=false` renders rail only, and the seasonal button renders the map.
  - The cut banner shows when `MapEnabledExpected` is false and `mapEnabled` is true.
  - The rail Open button calls `navigateCalls.toThread` and does not change the map
    selection.
  - The rail order matches the four `(Source, Kind)` groups, including a beads approval,
    which goes to group 3.
  - Unplaced gardens render in the unplaced band.
  - **Injection test:** an item title of `<button onclick=…>Approve</button>` renders as
    literal text, and zero extra buttons appear.
  - "Map caught this" appears only after a map click. It requires a category and a reason,
    a double submit sends one request, and it calls `catch` with the item IDs and the active
    edge count.

### WI-6: Trial operation (documented, not code)

`docs/reference/estate-trial.md` covers the following.
- **How to start:** the page's start button or `autarch estate trial --start`. The map is not
  shown until then.
- **What counts as a catch [mk-C]:** the *map* (a garden selected by clicking the map, under
  any lens) showed something mk would otherwise have missed, and the logged **reason** says
  why the rail alone would not have shown it.
  - At most one catch per garden, category and day.
  - What does not count:
    - rail-first discoveries
    - parked-garden fades
    - `cascade` catches with no active edges
    - anything mk rejects during review
- **Daily use:** open Estate at least once on each working day. Forgetting to log a catch
  cannot be repaired afterwards: the trial counts only logged catches. That is deliberate, so
  that recollection cannot inflate the result.
- **End of trial (binding) [mk-K]:**
  1. The chip reads "ended: review + close".
  2. mk runs `autarch estate catch list` and rejects ineligible entries with `catch reject`.
  3. mk runs `autarch estate trial --close`, which fixes the verdict.
  4. mk records the verdict, the catch log and the coverage lines (edges, placement, ambiguity)
     in `docs/adr/NNNN-estate-map-trial.md`.
  5. **If the verdict is `cut:rail-only`, applying it is part of closing the trial:** run
     `bb plugin config autarch set mapEnabled false`. The lenses stay reachable on demand
     through "Lenses (seasonal)".
     - The only way out is a written override: `trial --override --adr <path> --reason <why>`.
     - The banner persists until one of the two happens.

## Explicitly out of scope (trial slice)

- Pin-nudge. Positions are deterministic, from the Key-sorted grid.
- Ruling 7's seen/fade lifecycle. The trial has only the frozen window [A-12].
- A layer→pace mapping. Columns are labelled as layers [A-5].
- Park, probe expiry and budgets (rulings 13–15).
- Composer, dispatch, bootstrap, and answering or approving.
- The Mycroft BB spawner, and the beads coordinator claim.
- The allocation lens's Mycroft proposal curves. There is no Mycroft data in the trial.
- Seasonal reshaping, and scrub replay.
- CanonGraph-sourced edges. The graph has no garden→garden edges today.

## Verification

0. **Build once [A-17].** In the repo, run `go build -o ~/.local/bin/autarch ./cmd/autarch`.
   Every step below uses that exact binary, `B=~/.local/bin/autarch`. Rebuild it after any
   code change, and rerun steps 1 to 5.
1. **Tests.** `go vet ./... && go test -race ./internal/estate ./internal/door ./cmd/autarch`
   passes. This includes:
   - the schema and fixture tests
   - the writes-nothing test
   - the fake-source failure tests
   - the attribution tests: hub label, unattributed item, union of projects sharing a remote
   - **two same-named gardens** `projects.X` and `interverse.X` [R4-2]:
     - hub label `X` attributes to `hub` and counts `ambiguous: 1`
     - with `prefer: {X: interverse.X}` it attributes to `interverse.X`
     - a go.mod `<m> = X` emits no edge in the ambiguous case
   - an alias collision exits 2, and a symlinked duplicate root keeps the first root's key
   - the edge tests: cross-garden `blocked_by` only, go.mod, a three-garden chain, a cycle,
     two blocker origins, and a blocker clearing between snapshots [A-4]
   - remote ambiguity: a duplicate remote, labels that conflict across gardens, and the
     `/p/foo` versus `/p/foobar` path boundary [A-3]
   - item ID collisions across trackers, catch-input round trips, and IDs that are never
     truncated [A-6]
   - timestamp units, and missing or future ages [A-10, A-11]
   - an old `latestAttentionAt` with a recent `updatedAt` takes the newer one [A2-6]
   - tracker identity [A2-2]:
     - two redirect aliases are one tracker, queried once and attributed as shared
     - two distinct trackers with the same issue ID stay distinct
     - equal `project_id` with distinct server ports stays two trackers, and an equal
       binding merges [A3-1]
     - **real-`bd` regression [A4-1]:** a redirect target whose directory is not named
       `.beads` is queried from its referring checkout, and `bd where` confirms the target. A
       deliberately mismatched redirect yields `resolution mismatch` and no items. This test
       runs the installed `bd` binary against a temp embedded tracker, not a fake.
   - `Briefing.Complete` is false after a timed-out garden, **a failed health read**, or **a
     stalled discovery root**. In each case the UI skips `visit` on Continue, and the
     recovered read still shows the missed commits or health items because the watermark
     did not advance [A2-5, A3-5]
   - cancellation inside a transcript read is abandoned within its deadline, a FIFO in the
     health directory is skipped without hanging, a forked grandchild is killed, and a
     full `Build` over deadline-sleeping fakes finishes in under 8.5 s [A2-3]
   - the concurrency tests (WI-3), the real-git writes-nothing test, and the contract goldens
     diffed against the committed files
   - the trial state machine, catch deduplication and adjudication, and config errors
2. **Live, read-only on zklw.** Run `time $B estate --json | tee /tmp/estate.json`, then check:
   - **Schema [A-17]:**
     `npx --prefix integrations/bb-plugin-autarch ajv validate --spec=draft2020 -s docs/reference/estate-v1.schema.json -d /tmp/estate.json`
     succeeds, and so does `npm --prefix integrations/bb-plugin-autarch run check-snapshot -- /tmp/estate.json`,
     which parses the live file with the strict Zod mirror. `ajv-cli` is a devDependency.
   - **Timing is an acceptance gate, not an assumption [A-17]:** each of 3 consecutive runs
     completes under 8 s of wall time, the full `Build` included, and each source's `took_ms`
     is within its deadline. If this fails, the escalation clause applies before the trial.
   - every source is listed in `sources[]` with its coverage
   - the counts reconcile [R2-1]:
     - `bb thread list --json` threads with status `error` and without `archivedAt` match the
       errored-thread blockers
     - the `bd -C /home/mk/hub --readonly --sandbox list --json --limit 0 --status blocked`
       count matches the hub-sourced beads blockers
     - the live errored checkout thread under `~/bb-thread-migration-20260921/checkouts` is
       attributed to its garden, not `""`
   - one live item of each kind is spot-checked
   - the edge coverage line is recorded
3. **Deletion test by hand.** `mv ~/.autarch ~/.autarch.bak && $B estate --json >/dev/null`.
   - The command succeeds with a 24 h default window.
   - Health attribution falls back to `rig`.
   - Record `stat -c %Y .git/index` for three live gardens before and after, and check it is
     unchanged [A-1].
   - Then restore the directory.
4. **Plugin.** Using the binary from step 0 [R2-7, A-17], run
   `cd integrations/bb-plugin-autarch && npm test && bb plugin build`.
   - Run `bb plugin install . --yes` **only after mk confirms**, because the plugin is
     full-trust. Then `bb plugin dev`.
   - **Install checklist item:** before the start, the page shows no map and offers a start
     button. mk starts the trial, and the chip then reads "day 1/14" [R2-10, A-8].
   - Open the Estate page through BB Connect and check it against step 2's JSON:
     - item counts per rail group match
     - unplaced gardens are shown
     - edges carry their coverage label
     - the briefing does not re-open after 60 s
   - Take Playwright screenshots of the briefing, the map with the rail, and one blocker
     cascade (or the "0 edges" label, if there are none).
5. **Trial dry run** with `HOME` set to a temp directory and `AUTARCH_NOW` stepped by hand,
   using `$B`:
   - `trial` reports `not-started`.
   - Run `--start`; `trial` reports `running`, and a second `--start` refuses.
   - Log two catches on different days, one duplicate (not appended), and a `cascade` catch
     with `--active-edges 0` (ineligible).
   - Set `AUTARCH_NOW` to `end` and attempt a catch. It must be refused with `not-running`,
     and the log must be unchanged [A3-7].
   - The exclusion of a pre-existing `ts == end` record is covered separately by a unit test
     with a seeded log.
   - Past the end, `trial` reports `ended:awaiting-review`, and `--close` then gives `keep`.
   - In a fresh HOME, repeat with one eligible catch plus one that is rejected. `--close`
     gives `cut:rail-only` with `MapEnabledExpected=false`. `--override` without an existing
     ADR path refuses.
6. **Plan review.** Governed, independent frontier seat. Required fixes are folded in before
   execution. Round 1 was FAIL with 18 findings, and round 2 was FAIL with 10 findings; all of
   them are folded in above. Round 3 was FAIL with 7 findings, also folded in. Round 4 was
   **PASS-WITH-FIXES** (no P0/P1); its 6 fixes are folded in as `[R4-n]`.
   - The cross-lab review mk requested (review-astra, through the account pool) was **FAIL**,
     with 9 P1 and 8 P2 findings, all folded in as `[A-n]`, plus the two mk rulings.
   - Astra round 2 was **FAIL** with 5 P1 and 5 P2 findings, folded in as `[A2-n]`.
   - Astra round 3 was **FAIL** with 5 P1 and 2 P2 findings, folded in as `[A3-n]`.
   - Astra round 4 was **FAIL** with 2 P1 findings, folded in as `[A4-n]`.
   - **Astra round 5 was PASS-WITH-FIXES** (no P0/P1). Its P2 and P3 fixes are folded in as
     `[A5-n]`. **Plan review is passed.**
   - Full-estate timing and the implementation tests remain acceptance gates at execution.

## Execution preconditions (not code)

- **Checkout ownership:** reconcile with the paused @thread:thr_nn4veeieqr before any
  implementation edit.
- **CI:** the repo has ID 1140086114 and zklw-ci status `pending-inventory` (migration
  `mk-ag2s.18` is open). Tests run locally meanwhile, and no GitHub Actions dependency is
  added.
- **Escalation:**
  - If WI-0 shows that the CanonGraph read path is not usable from zklw, the trial proceeds
    with every garden **unplaced**. Placement is the hypothesis under test, so that outcome
    is recorded in the trial and not papered over.
  - Today there are 72 repo trackers plus the hub. Each healthy call measured about 0.06 s,
    but that is not evidence for the full `Build` [R3-7, A-17]. If verification step 2's
    timing gate fails, use `trackers: {mode: replace, paths: […]}` in
    `~/.autarch/estate.yaml`. That file is mk-owned preference, and there is no cache,
    because `Build` writes nothing. Report the reduced coverage in `sources[]` and rerun the
    gate.
