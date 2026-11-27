# Layer view — execution-grade plan (cujgel consume)

Written 2026-09-03 under intercore goal `fdaae66d` ("Write the nativity thesis for the estate's own graph"), whose DONE WHEN asks that the first reshaping slice have an execution-grade plan that lands through the card guard without an override. This file landing is that check; the build it describes is its own goal, proposed as the successor.

**Amended 2026-11-27: the source is the estate index, not CanonGraph.** Ruling 7's condition has been met. Ultan was built as Lattice's estate index over the Uqbar canon checkout (Lattice `docs/estate-index.md`), it answers the ten queries at parity, and CanonGraph is being retired (Sylveste `ops/canongraph/retirement.md`, step 5, "Repoint Autarch's graph source"). No code from WI-1..4 had been written, so this amendment changes the plan and not a build. WI-1 is now a stdio client for the `estate` MCP server. The token file, HTTP, SSE and the tailnet endpoint are gone. The `GraphSource` interface, WI-2's reading logic, the screen and the acceptance counts stay as they were. The 2026-09-03 text for the replaced parts is in git history at this file's previous commit.

**Spec:** `docs/cujs/autarch-03-seasonal-reshaping.json` · version **1.4** · status validated · sha256 `72c37c4404d1984e22be1f2f19a266f8e7741cb301e44d3fd101ad4dbeb7bba0`
**Thesis in force:** `docs/research/2026-09-03-ultan-nativity-thesis.md` (ruling 7: CanonGraph stays the read-only transitional source until Ultan answers the ten queries; met 2026-11-27, see the amendment above)
**Slice:** autarch-03 step 1, the project half only: "Review the estate's shape through the pace lens: which gardens are fast-layer probes churning as they should, which are sedimenting toward doctrine, which are misfiled". Build surfaces named by the step: `CanonGraph projects_in_layer / serving_map (read-only)`, now served by the estate index under the same query ids, and `layer view in the door (new)`. The theme-lifecycle half of the step stays OUT for this slice: the estate index has a `theme_reach` query, but theme lifecycle is its own slice.
**OUT:** writing to any graph or to the Uqbar; the in-context retune (step 2); reranking (step 3); delegation scopes (step 4); changes to Lattice itself; salon; the waiting-on-me axis; the stamp flow.

## mental_model (verbatim — the constraint solver)

> Pace layers: fast layers innovate and churn, slow layers stabilize and constrain — and the estate's health is the layers moving at their own speeds, not one speed. Reshaping is the gardener's seasonal work: it happens rarely, deliberately, and mostly by *retuning* rather than rebuilding. The interruption budget is a first-class tuned number, adjusted when it drifts — Stellaris's own history says sprawl formulas cannot fix late-game slog; only trustworthy delegation plus classed interrupts do, which makes this journey (not more ranking math) the answer to the slog resentment.

## Closed decisions in force (not re-litigated)

- Containment/layer structure comes from the estate's own graph; CanonGraph (`projects_in_layer` / `serving_map`) was the transitional read source until that existed (autarch-03, mk 2026-09-01). → It exists now: this slice reads the estate index, read-only, through the same `estate` stdio MCP server every other session uses, behind the one `GraphSource` interface the plan always intended to swap.
- Leaving CanonGraph is a sovereignty-and-nativity ruling, not a defect fix; parity is the floor (autarch-03). → The estate index keeps CanonGraph's query ids, parameters and columns, so the rows WI-2 consumes are unchanged. The view shows what the index holds and what the local gardens add, and no more.
- Autarch holds no state; world edits flow through it as mk's acts (autarch-03 retune decision, amended 2026-09-03 on delivery classes). → The layer view writes nothing anywhere, not even a cache. Every open re-reads.
- Files are truth, the graph is an index; the ruling is the unit of a write (thesis rulings 1 and 2). → Out of scope for a read-only view. Rows come back exactly as the index returns them; tagging rows ruled or proposed is a later slice (the CLI's `query --with-state` exists for that).
- Never run graph commands against the live service (CanonGraph decisions 2026-07-14). → The estate index has no live service and no single-writer store: each read starts the `estate` MCP server as a child process over the local Uqbar checkout. Autarch still never reads the Uqbar's files itself, never calls `lattice.estate.uqbar` (no `sign`, no `init`), and never touches `~/.canongraph` or the CanonGraph endpoint.
- Quiet-unless-attention-worthy and orientation-before-obligation (autarch-01) apply to the door as a whole. → The layer view is reached by a key from the rows or the briefing, never shown first, and asks for no decision.

## Invariants verified at plan time (2026-09-03 09:00)

- `go test -race ./internal/door` → `ok … 5.345s` (exit 0).
- `go vet ./internal/door ./cmd/autarch` → clean (exit 0).
- Live graph on 2026-09-03 (CanonGraph): `projects_in_layer` answered for every letter A–L and X (G and L return zero rows), 51 rows in total; `serving_map` returned 5 rows.
- Estate index on 2026-11-27 (zklw, `LATTICE_UQBAR_ROOT=~/projects/uqbar`), one `estate` MCP process: initialize answered in 0.09s, and all 13 letters plus `serving_map` answered in 1.51s in total. Rows per letter: A 9, B 5, C 8, D 5, E 4, F 3, G 0, H 3, I 1, J 3, K 1, L 0, X 9, so 51 in total over 11 non-empty letters, the same as 2026-09-03. `projects_in_layer` rows carry `designation`, `project`, `status`, `ecosystem`. `serving_map` returns 5 rows, now all machine `zklw`, with columns `machine`, `port`, `project`, `unit`, `url`.
- Response shape: a `tools/call` result is `{"content":[{"type":"text","text":"<json>"}]}` with no `structuredContent`; the text parses to `{"query_id":"…","rows":[…],"warnings":[…]}` (no `result` wrapper). A tool failure is the same envelope with `"isError": true` and a plain-text reason; with no Uqbar configured the reason is `no Uqbar configured: set LATTICE_UQBAR_ROOT to mk's Uqbar checkout`. The `query` tool takes the query's parameters nested: `{"query_id":"projects_in_layer","params":{"layer":"A"}}`; passing them flat fails.
- Autarch has no Project record in the estate index (`resolve Autarch` → `entity_id: null`); the view must render "no row in the graph" for gardens that exist locally but not in the catalog, as a fact, not an error.
- The card at `docs/why.md` is `status: confirmed`; this plan file landing without `AUTARCH_CARD_OVERRIDE` set is the guard passing.

## Work items

Package `internal/door`, module `github.com/mistakeknot/autarch`. Follow the threads screen (`threads.go`, `threads_view.go`, `threads_model.go`, `cmd/autarch/threads.go`) as the shape to copy: options struct, `With…` constructor, a goroutine that reads and posts messages, a screen constant, a key that opens it and returns, a subcommand that prints the same rows. Every timeout below is a constant with a comment saying why.

### WI-1 — `internal/door/graph.go`: a read-only client for the `estate` MCP server

serves: autarch-03 step 1 build surface "CanonGraph projects_in_layer / serving_map (read-only)", now served by the estate index; closed decisions "the estate's own graph is the read source" and "never write the graph".

Types and functions, exactly these signatures:

```go
// GraphSource answers named queries. The estate index today; any later index
// keeps this interface.
type GraphSource interface {
    Query(ctx context.Context, queryID string, params map[string]any) ([]map[string]any, error)
}

// EstateEnv says where the estate server's code and canon live.
type EstateEnv struct{ Lattice, Uqbar string }

// LoadEstateEnv resolves the two paths: an explicit argument wins, then
// LATTICE_DIR / LATTICE_UQBAR_ROOT from the environment, then the defaults
// below under $HOME. Either directory missing, or `uv` absent from PATH, is
// an error that names the path; the caller renders it as unchecked, never a
// crash.
func LoadEstateEnv(lattice, uqbar string) (EstateEnv, error)

const (
    DefaultLatticeDir = "projects/Sylveste/interverse/lattice" // under $HOME
    DefaultUqbarDir   = "projects/uqbar"                       // under $HOME
)

// estateStartTimeout bounds starting the child and its initialize answer.
// A warm start measured 0.09s on 2026-11-27; uv may resolve the environment
// on a cold cache, so the budget is generous.
const estateStartTimeout = 10 * time.Second

// graphCallTimeout bounds one query. All 14 layer-view queries took 1.51s
// together on 2026-11-27; two seconds per call is the budget the recall hook
// already runs on.
const graphCallTimeout = 2 * time.Second

type EstateGraph struct {
    Env     EstateEnv
    Command []string // nil means EstateCommand(Env); tests point it at a fake server
    // unexported: mu sync.Mutex, cmd *exec.Cmd, stdin io.WriteCloser,
    // stdout *bufio.Reader, nextID int, started bool
}

// EstateCommand is the argv that starts the server:
// uv run --quiet --directory <Lattice> python -m lattice.estate.mcp_server
func EstateCommand(env EstateEnv) []string

func NewEstateGraph(env EstateEnv) *EstateGraph
func (g *EstateGraph) Query(ctx context.Context, queryID string, params map[string]any) ([]map[string]any, error)
func (g *EstateGraph) Close() error // kills the child if running; safe to call twice
```

Transport: newline-delimited JSON-RPC 2.0 over the child's stdin and stdout, one object per line, as `lattice.estate.mcp_server.serve` reads and writes it. The child's environment is the parent's plus `LATTICE_UQBAR_ROOT=<Env.Uqbar>`; its stderr is discarded. `Query` holds `mu` for the whole exchange, so calls are serialized on one process; ReadLayers may still have several in flight, and they queue.

1. On the first `Query`, or the first after a failure: start the child, then write `{"jsonrpc":"2.0","id":<n>,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"autarch","version":"door"}}}` and read one line, all under `estateStartTimeout`. Then write `{"jsonrpc":"2.0","method":"notifications/initialized"}`; the server answers notifications with nothing, so read nothing.
2. Per call: `{"jsonrpc":"2.0","id":<n>,"method":"tools/call","params":{"name":"query","arguments":{"query_id":"<queryID>","params":<params or {}>}}}`. The parameters are nested under `params`, never flat. `id` increments per request, and a response line whose `id` does not match is an error.

Response decoding: from the JSON-RPC `result` take `content[0].text` and parse it as JSON; return its `.rows`. If `result.isError` is true, return the text as the error. A JSON-RPC `error` member, an unparsable line, EOF, or a body with neither shape is an error. Every error is prefixed `graph:`. Rows are `[]map[string]any` exactly as the server returned them; no renaming. `.warnings` is not surfaced in this slice.

Failure handling: a call that hits its context deadline, an EOF, or a write error kills the child and marks the graph unstarted, so the next `Query` starts afresh and a hung server never wedges later reads. The reader runs in a goroutine per call so the deadline can fire while a line is pending.

Lifetime: whoever builds an `EstateGraph` closes it. The layers read (WI-3) and `autarch layers` (WI-4) each start one per read and `Close` it once `ReadLayers` returns, so no child outlives a read.

Tests (`internal/door/graph_test.go`): the fake server is the test binary itself, run as a helper process (`Command: []string{os.Args[0], "-test.run=TestHelperEstateServer", "--"}` with `GO_WANT_HELPER_ESTATE=1` in its environment), which answers from a fixture map keyed by `query_id` + `layer`. Cases:
- Rows decode from text content. The helper rejects a `tools/call` whose `params` are flat, so the nesting is asserted.
- Three queries reuse one process: the helper counts `initialize` and reports the count.
- An `isError: true` result returns an error containing its text and the `graph:` prefix.
- A helper that never answers `tools/call` makes `Query` return an error within 2.5s. The next `Query` starts a new process and succeeds.
- `Command: []string{"/nonexistent"}` returns an error, with no panic.
- `LoadEstateEnv`: with `HOME` at `t.TempDir()` and no directories, the error names the missing Lattice path; with both directories created and the environment variables unset, it returns the defaults; explicit arguments win over the environment.

No test touches the network, the real Uqbar, or `~/projects`.

Acceptance: `go test -race -run 'Graph|Estate' ./internal/door` passes; `go vet ./internal/door` clean; `grep -n "canongraph" internal/door/graph.go` finds nothing (the type names carry no vendor name).

### WI-2 — `internal/door/layers.go`: rows by layer, matched to local gardens

serves: autarch-03 step 1 (the pace lens over projects); thesis ruling 4's reading side (a last-tended time per node, here computed locally from git). The estate index does carry `last_tended` on records that set it, and has an `overdue_tending` query, but `projects_in_layer` returns neither, and most migrated records have no `last_tended` yet; reading it is a later slice.

```go
// Layers is the letters the catalog uses, in the order the view shows them.
var Layers = []string{"A","B","C","D","E","F","G","H","I","J","K","L","X"}

type LayerRow struct {
    Layer, Designation, Project, Status, Ecosystem string
    Garden   string    // local root name this row matched, or "" when the estate has no garden by that name
    Root     string    // matched garden's root
    Latest   time.Time // newest commit on any ref inside movementWindow; zero when none or unmatched
    MoveErr  error     // the garden could not be read
}

type LayerSet struct {
    Rows    []LayerRow // sorted by Layers order, then Designation
    Counts  map[string]int
    Errs    map[string]error // per layer letter: the query failed; the letter is unchecked
    ReadAt  time.Time
}

// movementWindow is how far back the layer view asks git for the last
// commit. Thirty days is a season's width for the fast layers; slower ones
// show "none in 30d", which is the fact the pace lens wants.
const movementWindow = 30 * 24 * time.Hour

func ReadLayers(ctx context.Context, src GraphSource, projects []Project, now time.Time, onLayer func(layer string, rows []LayerRow, err error)) LayerSet
func MatchGarden(name string, projects []Project) (Project, bool)
func normalizeName(s string) string // lowercase; drop every rune that is not a letter or digit
```

`ReadLayers` queries each letter concurrently (at most 4 in flight, a `sync.WaitGroup` and a buffered channel of size 4), each call under its own `graphCallTimeout`. `EstateGraph` serializes calls on one process, so the in-flight calls queue; at about 0.1s a query, the fourth waits well inside its two seconds, and the bound still protects a slower `GraphSource`. Rows are built from the graph's `designation`, `project`, `status`, `ecosystem` fields (strings; a missing field is ""). `MatchGarden` compares `normalizeName(row.Project)` with `normalizeName(p.Name)` and returns the first match in `projects` order. For every matched row, `GitMovement(ctx, p, now.Add(-movementWindow))` supplies `Latest` (that function exists in `briefing.go`; call it with 8 workers, the same fan-out as `ReadThreads`). `onLayer` fires once per letter as it completes, so the screen can fill in progressively; it may be nil. Rows are sorted with `sort.SliceStable` by the index of `Layer` in `Layers`, then by `Designation`.

Tests (`internal/door/layers_test.go`): a fake `GraphSource` (a struct with a `map[string][]map[string]any` and an optional error per letter) drives `ReadLayers` over a `projects` slice built from `t.TempDir()` roots, two of which are `git init`ed with one commit each so `Latest` is non-zero for exactly those; assert the sort order, the counts (13 letters, two zero), the per-letter error surfacing for one letter, and that `normalizeName("After-Them") == normalizeName("after-them")`. No test reads `~/.autarch` or the network.

Acceptance: `go test -race -run 'Layers|MatchGarden|normalizeName' ./internal/door` passes; a row whose project has no local garden has `Garden == ""` and zero `Latest`; a letter whose query failed appears in `Errs` and contributes no rows.

### WI-3 — `internal/door/layers_view.go` + `layers_model.go`: the screen

serves: autarch-03 step 1 build surface "layer view in the door (new)"; autarch-01 closed decisions on orientation (reached by a key, never first) and on Autarch holding no state.

Model additions (in `model.go`, next to the threads fields): `screenLayers` appended to the `screen` enum after `screenThreads`; fields `layersOpts LayersOptions`, `layersOn bool`, `layers LayerSet`, `layersPending int` (letters still being read), `layersLoaded bool`, `layerOffset int`, `layersPrev screen`. Messages `layerMsg{layer string; rows []LayerRow; err error}` and `layersDoneMsg{set LayerSet}`.

```go
type LayersOptions struct {
    Lattice string      // Lattice checkout; "" means LoadEstateEnv's resolution
    Uqbar   string      // Uqbar checkout; "" means LoadEstateEnv's resolution
    Source  GraphSource // nil means NewEstateGraph(LoadEstateEnv(Lattice, Uqbar)), closed when the read ends; tests inject a fake
}
func (m Model) WithLayers(o LayersOptions) Model
```

Keys, in `handleKey` directly after the threads block: when `m.layersOn`, `l` from the rows or the briefing sets `layersPrev = m.screen`, `m.screen = screenLayers`, and starts the read if `!layersLoaded` (the read is started once per open of Autarch; `r` re-reads it along with everything else). On the layers screen: `l`, `tab` and `esc` return to `layersPrev`; `up`/`down`/`pgup`/`pgdown` scroll by row; `r` falls through to the global re-read; `enter` on a row with a matched garden switches to that garden exactly as the rows screen does (call the existing enter path with the matched `Project`); `q` quits.

Render (`layersLines(rows int) []string`): a header `layers: 51 projects · 13 letters · 38 with a garden here · 13 unchecked` where the last count is rows whose letter failed, omitted when zero. Then per letter a group line `A  9 projects` (or `A  none` for an empty letter, `A  unchecked: <err>` for a failed one), then one line per row: designation, project, status (truncated with `ansi.Truncate` to the column width; never `[]rune` slicing), then the garden column: `garden <name> · last commit <age>` where age is `3d`, `12h`, `none in 30d`, or `no garden here` when unmatched, or `could not read` on `MoveErr`. While `layersPending > 0` the header ends with ` · reading N…`. If `LoadEstateEnv` fails, every letter is unchecked with its error, which names the missing path. Nothing is written to disk by any path in this file.

Footer: on the layers screen `{"↑/↓ move","enter switch","l back","r re-read","q quit"}`; elsewhere `"l layers"` is inserted before `"q quit"` exactly as `"t threads"` is (`model.go` around line 1040).

Tests (`internal/door/layers_view_test.go`): `TestLayersScreenToggleAndReturn` (from rows, `l` opens, `l` returns to rows; from briefing, `l` opens, `esc` returns to briefing); `TestLayersUncheckedShowsReason` (a fake source that errors for every letter renders the header with 13 unchecked and zero rows, and the reason line contains the error text); `TestLayersRowsRender` (fixture rows across three letters render group lines in `Layers` order and the garden column shows `no garden here` for an unmatched row); `TestLayersWritesNothing` (run the read and render with `HOME` pointed at `t.TempDir()`; assert the directory has no `.autarch` entry afterwards).

Acceptance: `go test -race ./internal/door` passes in full; the existing `TestTmuxCaptureSwitchClientAndZed` still passes (the footer change must not break its wait condition, which looks for `enter switch/open` on the rows footer).

### WI-4 — `cmd/autarch/layers.go`: `autarch layers [--json] [--layer X] [--lattice PATH] [--uqbar PATH]`

serves: the same step, as the shell-readable twin of the screen (the threads screen has `autarch threads`; the plan keeps that symmetry so the pace lens can be read in a pipe).

`layersCmd()` with flags `--json`, `--layer` (repeatable; default all of `Layers`), `--lattice` and `--uqbar` (default `""` each, resolved by `LoadEstateEnv`), `--root` (repeatable, default `~/projects`, resolved the way `door.go` resolves roots). It loads the projects the door would (call the same discovery `door.go` uses), builds `NewEstateGraph`, runs `ReadLayers`, closes the graph, and prints the header and rows in the same text as the screen at width 160, or a JSON array of `layerJSON{Layer, Designation, Project, Status, Ecosystem, Garden, Root string; Latest string; MoveErr string}` (snake_case tags, `latest` RFC3339 or ""). Exit 2 when `LoadEstateEnv` fails or every letter failed; exit 0 otherwise, with unchecked letters reported on stderr. Register in `cmd/autarch/main.go` after `threadsCmd()`.

Wiring in `cmd/autarch/door.go`: add `--estate-lattice PATH` and `--estate-uqbar PATH` (default "" each), and `.WithLayers(door.LayersOptions{Lattice: o.estateLattice, Uqbar: o.estateUqbar})` after `.WithThreads(...)`.

Acceptance: `go build ./cmd/autarch` succeeds; `go vet ./cmd/autarch` clean; `autarch layers --json --uqbar /nonexistent` exits 2 with a one-line stderr naming the path; on this machine `autarch layers --json | python3 -c 'import json,sys; r=json.load(sys.stdin); print(len(r), len({x["layer"] for x in r}))'` prints `51 11` (51 rows, 11 non-empty letters) within 30 seconds.

### WI-5 — docs: spec provenance and README line

serves: cujgel re-entry discipline (the spec records what was built from it).

After WI-1..4 land: append a `provenance` entry to `docs/cujs/autarch-03-seasonal-reshaping.json` naming this plan and the commits, bump `version` to `1.5`, run `cujgel-validate`, and add a one-paragraph `v1.5 (autarch-03 only)` note to `docs/cujs/README.md` before the "Validate with the cujgel plugin" line. Record on the spec, as a closed decision with mk's words, the answer to question 1 below if mk has ruled by then; otherwise leave it open.

Acceptance: `cujgel-validate docs/cujs/autarch-03-seasonal-reshaping.json` prints `OK`; `git log --oneline -1 -- docs/cujs/autarch-03-seasonal-reshaping.json` shows the provenance commit.

## Question ledger

**must_stop:** none. The nativity thesis closed the spec's only one on 2026-09-03.

**ask_then_proceed (to mk, at the render):**

1. Which letters are fast and which are slow? The catalog's letters A–L and X carry no speed in the graph, and the view therefore labels no layer fast or slow; it shows the last commit per row and lets the reading be mk's. If mk supplies the mapping at the render, it becomes a closed decision on the spec and WI-3 may add a per-letter cadence word. Until then the view is descriptive only. **Provisional answer: none; nothing is built on it.**
2. Season boundary marker (already open on the spec): untouched by this slice.

**auto_proceed (recorded):**

- Movement source for `Latest` is git only (`GitMovement`, commits on any ref inside 30 days). The transcript index is not consulted, so the column is labelled `last commit`, never `last moved`. Reversal: pass the briefing's `IndexSessions` result into `ReadLayers` and relabel.
- `movementWindow` = 30 days. Reversal: one constant.
- The layers screen's key is `l`. Reversal: one case label.
- `estateStartTimeout` = 10s (amendment 2026-11-27). The warm start measured 0.09s; the margin covers uv resolving the environment on a cold cache. Reversal: one constant.
- One `estate` child per read, closed when the read ends, rather than one per Autarch process (amendment 2026-11-27). A read costs one start of about 0.1s warm, and no child outlives a read, which keeps "Autarch holds no state" literal. Reversal: keep the graph on the model and close it on quit.
- Flag names `--lattice`/`--uqbar` (WI-4) and `--estate-lattice`/`--estate-uqbar` (door) (amendment 2026-11-27). Reversal: rename the flags.

**[invented] (drift log, target zero):** none. Every constant above is either taken from an existing decision (2s from the recall hook decision; 8 workers and the footer shape from the threads screen) or recorded under auto_proceed, including the three amendment entries.

**Executor note (capability routing):** this plan is written for a Sonnet executor with Opus validating against the acceptance lines above, not against its own judgment. If the executor cannot decode the MCP response in WI-1 after two attempts, stop and escalate; the fixture shapes are given, and a third guess is drift.

## Success audit plan

The spec's success_condition: "After a reshaping pass, mk's next week of walks feels lighter without anything important going dark — and when a signal annoys them, they fix it in the two seconds of the annoyance and recognize that as the Stellaris retune button they admired, working on their own estate." This slice is the reading half only; the retune button is step 2, out of scope. Translated two ways:

**Structured proxies (checkable at review):**

- met/unmet: every acceptance line above, run as written.
- met/unmet: a live render on this machine shows 51 rows across 11 letters, G and L marked `none`, and at least one row per populated letter carrying `last commit <age>`; the screen appears only after `l` is pressed.
- met/unmet: with `--estate-uqbar /nonexistent`, the screen shows every letter unchecked with the path in the reason line, and the rest of Autarch is unaffected.
- met/unmet: after quitting Autarch, `pgrep -f lattice.estate.mcp_server` lists no child that Autarch started.
- met/unmet: `find ~/.autarch -newer <marker>` after a render lists nothing the layers path wrote (the door's own visit stamp excepted).
- unverifiable-by-machine: whether the rows read as the estate's pace.

**The recognition check (to mk, never automated away):** after one real render, two questions, one at a time. "Reading this by letter, which layer is misfiled: a probe sitting still on a fast letter, or something churning on a slow one?" And then: "Is this the pace lens you meant, or is the lens something the letters do not carry?" mk's words are quoted into the spec's provenance entry at v1.5.
