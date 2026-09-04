# Layer view — execution-grade plan (cujgel consume)

Written 2026-09-03 under intercore goal `fdaae66d` ("Write the nativity thesis for the estate's own graph"), whose DONE WHEN asks that the first reshaping slice have an execution-grade plan that lands through the card guard without an override. This file landing is that check; the build it describes is its own goal, proposed as the successor.

**Spec:** `docs/cujs/autarch-03-seasonal-reshaping.json` · version **1.4** · status validated · sha256 `72c37c4404d1984e22be1f2f19a266f8e7741cb301e44d3fd101ad4dbeb7bba0`
**Thesis in force:** `docs/research/2026-09-03-ultan-nativity-thesis.md` (ruling 7: CanonGraph stays the read-only transitional source until Ultan answers the ten queries)
**Slice:** autarch-03 step 1, the project half only: "Review the estate's shape through the pace lens: which gardens are fast-layer probes churning as they should, which are sedimenting toward doctrine, which are misfiled". Build surfaces named by the step: `CanonGraph projects_in_layer / serving_map (read-only)` and `layer view in the door (new)`. The theme-lifecycle half of the step is OUT: there is no theme object until Ultan is built.
**OUT:** writing to any graph; the in-context retune (step 2); reranking (step 3); delegation scopes (step 4); Ultan itself; salon; the waiting-on-me axis; the stamp flow.

## mental_model (verbatim — the constraint solver)

> Pace layers: fast layers innovate and churn, slow layers stabilize and constrain — and the estate's health is the layers moving at their own speeds, not one speed. Reshaping is the gardener's seasonal work: it happens rarely, deliberately, and mostly by *retuning* rather than rebuilding. The interruption budget is a first-class tuned number, adjusted when it drifts — Stellaris's own history says sprawl formulas cannot fix late-game slog; only trustworthy delegation plus classed interrupts do, which makes this journey (not more ranking math) the answer to the slog resentment.

## Closed decisions in force (not re-litigated)

- Containment/layer structure comes from the estate's own graph; CanonGraph (`projects_in_layer` / `serving_map`) is the transitional read source until that exists (autarch-03, mk 2026-09-01). → This slice reads CanonGraph, read-only, through the same HTTP path every other client uses, and is written so the source can be swapped for Ultan behind one interface.
- Leaving CanonGraph is a sovereignty-and-nativity ruling, not a defect fix; parity is the floor (autarch-03). → Nothing here works around CanonGraph's shape; the view shows what the graph holds and what the local gardens add, and no more.
- Autarch holds no state; world edits flow through it as mk's acts (autarch-03 retune decision, amended 2026-09-03 on delivery classes). → The layer view writes nothing anywhere, not even a cache. Every open re-reads.
- Files are truth, the graph is an index; the ruling is the unit of a write (thesis rulings 1 and 2). → Out of scope for a read-only view, but the interface in WI-2 returns rows tagged with their source so a later Ultan reader can tag proposed rows.
- Never run graph commands against the live service; the Kùzu store is single-writer (CanonGraph decisions 2026-07-14, thesis §CanonGraph as it is). → HTTP only. No CLI, no file access to `~/.canongraph`.
- Quiet-unless-attention-worthy and orientation-before-obligation (autarch-01) apply to the door as a whole. → The layer view is reached by a key from the rows or the briefing, never shown first, and asks for no decision.

## Invariants verified at plan time (2026-09-03 09:00)

- `go test -race ./internal/door` → `ok … 5.345s` (exit 0).
- `go vet ./internal/door ./cmd/autarch` → clean (exit 0).
- Live graph on 2026-09-03: `projects_in_layer` answers for every letter A–L and X (G and L return zero rows), 51 rows in total; `serving_map` returns 5 rows, all machine `zklw`. The MCP tool result arrives as text content holding JSON of the form `{"result":{"query_id":"…","rows":[…]}}`.
- Token file: `~/.config/canongraph/canongraph.env` exists on this machine, one line, `CG_AUTH_TOKEN=…`. The endpoint recorded in the graph's own serving edge is `http://100.78.63.67:3943/mcp` (tailnet-only).
- Autarch has no Project row in the graph (`resolve` → `is_new: true`); the view must render "no row in the graph" for gardens that exist locally but not in the catalog, as a fact, not an error.
- The card at `docs/why.md` is `status: confirmed`; this plan file landing without `AUTARCH_CARD_OVERRIDE` set is the guard passing.

## Work items

Package `internal/door`, module `github.com/mistakeknot/autarch`. Follow the threads screen (`threads.go`, `threads_view.go`, `threads_model.go`, `cmd/autarch/threads.go`) as the shape to copy: options struct, `With…` constructor, a goroutine that reads and posts messages, a screen constant, a key that opens it and returns, a subcommand that prints the same rows. Every timeout below is a constant with a comment saying why.

### WI-1 — `internal/door/graph.go`: a read-only client for the graph's MCP endpoint

serves: autarch-03 step 1 build surface "CanonGraph projects_in_layer / serving_map (read-only)"; closed decisions "CanonGraph is the transitional read source" and "never run graph commands against the live service".

Types and functions, exactly these signatures:

```go
// GraphSource answers named queries; CanonGraph today, Ultan later.
type GraphSource interface {
    Query(ctx context.Context, queryID string, params map[string]any) ([]map[string]any, error)
}

// GraphEnv is what the token file yields.
type GraphEnv struct{ URL, Token string }

// LoadGraphEnv reads KEY=value lines. CG_AUTH_TOKEN is required; CG_URL is
// optional and defaults to DefaultGraphURL. A missing file is an error the
// caller renders as unchecked, never a crash.
func LoadGraphEnv(path string) (GraphEnv, error)

const DefaultGraphURL = "http://100.78.63.67:3943/mcp"

// graphCallTimeout bounds one HTTP round trip. The graph lives on zklw over
// Tailscale; two seconds is the budget the recall hook already runs on.
const graphCallTimeout = 2 * time.Second

type HTTPGraph struct {
    Env    GraphEnv
    Client *http.Client // nil means http.DefaultClient with graphCallTimeout applied per call
    sessionID string   // Mcp-Session-Id echoed back once initialize returns one
}

func NewHTTPGraph(env GraphEnv) *HTTPGraph
func (g *HTTPGraph) Query(ctx context.Context, queryID string, params map[string]any) ([]map[string]any, error)
```

`Query` performs at most three POSTs to `Env.URL`, each with headers `Content-Type: application/json`, `Accept: application/json, text/event-stream`, `Authorization: Bearer <token>`, and `Mcp-Session-Id: <id>` once known. Bodies, JSON-RPC 2.0:

1. Once per `HTTPGraph` (guard with a `sync.Once`): `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"autarch","version":"door"}}}`. Read the `Mcp-Session-Id` response header if present. Then POST `{"jsonrpc":"2.0","method":"notifications/initialized"}` and ignore its body.
2. Per call: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"query","arguments":{"query_id":"<queryID>","params":<params or {}>}}}`.

Response decoding: the body is either plain JSON or SSE (`text/event-stream`). For SSE, concatenate the `data:` lines of the first event that parses as JSON. From the JSON-RPC result take `result.structuredContent.result.rows` if present; otherwise `result.content[0].text`, parse it as JSON, and take `.result.rows`. A JSON-RPC `error` member, a non-2xx status, or a body with neither shape is returned as an error prefixed `graph:`. Rows are `[]map[string]any` exactly as the graph returned them; no renaming.

Tests (`internal/door/graph_test.go`): an `httptest.Server` that asserts the bearer header, returns a session id on `initialize`, accepts `notifications/initialized`, and answers `tools/call` from a fixture map keyed by `query_id` + `layer` in both encodings (one test plain JSON, one SSE-framed). A third test points at a server that sleeps 3s and asserts `Query` returns an error within 2.5s. A fourth asserts `LoadGraphEnv` on `testdata/graph.env` (contents `CG_AUTH_TOKEN=test-token`) yields the default URL and the token, and on a missing path returns an error. No test touches the network or `~/.config`.

Acceptance: `go test -race -run 'Graph' ./internal/door` passes; `go vet ./internal/door` clean; `grep -n "canongraph\b" internal/door/graph.go` finds the word only in comments (the type names carry no vendor name, per the swap-for-Ultan intent).

### WI-2 — `internal/door/layers.go`: rows by layer, matched to local gardens

serves: autarch-03 step 1 (the pace lens over projects); thesis ruling 4's reading side (a last-tended time per node, here computed locally because CanonGraph carries none).

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

`ReadLayers` queries each letter concurrently (at most 4 in flight, a `sync.WaitGroup` and a buffered channel of size 4), each call under its own `graphCallTimeout`. Rows are built from the graph's `designation`, `project`, `status`, `ecosystem` fields (strings; a missing field is ""). `MatchGarden` compares `normalizeName(row.Project)` with `normalizeName(p.Name)` and returns the first match in `projects` order. For every matched row, `GitMovement(ctx, p, now.Add(-movementWindow))` supplies `Latest` (that function exists in `briefing.go`; call it with 8 workers, the same fan-out as `ReadThreads`). `onLayer` fires once per letter as it completes, so the screen can fill in progressively; it may be nil. Rows are sorted with `sort.SliceStable` by the index of `Layer` in `Layers`, then by `Designation`.

Tests (`internal/door/layers_test.go`): a fake `GraphSource` (a struct with a `map[string][]map[string]any` and an optional error per letter) drives `ReadLayers` over a `projects` slice built from `t.TempDir()` roots, two of which are `git init`ed with one commit each so `Latest` is non-zero for exactly those; assert the sort order, the counts (13 letters, two zero), the per-letter error surfacing for one letter, and that `normalizeName("After-Them") == normalizeName("after-them")`. No test reads `~/.autarch` or the network.

Acceptance: `go test -race -run 'Layers|MatchGarden|normalizeName' ./internal/door` passes; a row whose project has no local garden has `Garden == ""` and zero `Latest`; a letter whose query failed appears in `Errs` and contributes no rows.

### WI-3 — `internal/door/layers_view.go` + `layers_model.go`: the screen

serves: autarch-03 step 1 build surface "layer view in the door (new)"; autarch-01 closed decisions on orientation (reached by a key, never first) and on Autarch holding no state.

Model additions (in `model.go`, next to the threads fields): `screenLayers` appended to the `screen` enum after `screenThreads`; fields `layersOpts LayersOptions`, `layersOn bool`, `layers LayerSet`, `layersPending int` (letters still being read), `layersLoaded bool`, `layerOffset int`, `layersPrev screen`. Messages `layerMsg{layer string; rows []LayerRow; err error}` and `layersDoneMsg{set LayerSet}`.

```go
type LayersOptions struct {
    EnvPath string // token file; "" means ~/.config/canongraph/canongraph.env
    Source  GraphSource // nil means NewHTTPGraph(LoadGraphEnv(EnvPath)); tests inject a fake
}
func (m Model) WithLayers(o LayersOptions) Model
```

Keys, in `handleKey` directly after the threads block: when `m.layersOn`, `l` from the rows or the briefing sets `layersPrev = m.screen`, `m.screen = screenLayers`, and starts the read if `!layersLoaded` (the read is started once per open of Autarch; `r` re-reads it along with everything else). On the layers screen: `l`, `tab` and `esc` return to `layersPrev`; `up`/`down`/`pgup`/`pgdown` scroll by row; `r` falls through to the global re-read; `enter` on a row with a matched garden switches to that garden exactly as the rows screen does (call the existing enter path with the matched `Project`); `q` quits.

Render (`layersLines(rows int) []string`): a header `layers: 51 projects · 13 letters · 38 with a garden here · 13 unchecked` where the last count is rows whose letter failed, omitted when zero. Then per letter a group line `A  9 projects` (or `A  none` for an empty letter, `A  unchecked: <err>` for a failed one), then one line per row: designation, project, status (truncated with `ansi.Truncate` to the column width; never `[]rune` slicing), then the garden column: `garden <name> · last commit <age>` where age is `3d`, `12h`, `none in 30d`, or `no garden here` when unmatched, or `could not read` on `MoveErr`. While `layersPending > 0` the header ends with ` · reading N…`. Nothing is written to disk by any path in this file.

Footer: on the layers screen `{"↑/↓ move","enter switch","l back","r re-read","q quit"}`; elsewhere `"l layers"` is inserted before `"q quit"` exactly as `"t threads"` is (`model.go` around line 1040).

Tests (`internal/door/layers_view_test.go`): `TestLayersScreenToggleAndReturn` (from rows, `l` opens, `l` returns to rows; from briefing, `l` opens, `esc` returns to briefing); `TestLayersUncheckedShowsReason` (a fake source that errors for every letter renders the header with 13 unchecked and zero rows, and the reason line contains the error text); `TestLayersRowsRender` (fixture rows across three letters render group lines in `Layers` order and the garden column shows `no garden here` for an unmatched row); `TestLayersWritesNothing` (run the read and render with `HOME` pointed at `t.TempDir()`; assert the directory has no `.autarch` entry afterwards).

Acceptance: `go test -race ./internal/door` passes in full; the existing `TestTmuxCaptureSwitchClientAndZed` still passes (the footer change must not break its wait condition, which looks for `enter switch/open` on the rows footer).

### WI-4 — `cmd/autarch/layers.go`: `autarch layers [--json] [--layer X] [--env PATH]`

serves: the same step, as the shell-readable twin of the screen (the threads screen has `autarch threads`; the plan keeps that symmetry so the pace lens can be read in a pipe).

`layersCmd()` with flags `--json`, `--layer` (repeatable; default all of `Layers`), `--env` (default `""`, resolved to `~/.config/canongraph/canongraph.env`), `--root` (repeatable, default `~/projects`, resolved the way `door.go` resolves roots). It loads the projects the door would (call the same discovery `door.go` uses), builds `NewHTTPGraph`, runs `ReadLayers`, and prints the header and rows in the same text as the screen at width 160, or a JSON array of `layerJSON{Layer, Designation, Project, Status, Ecosystem, Garden, Root string; Latest string; MoveErr string}` (snake_case tags, `latest` RFC3339 or ""). Exit 2 when the token file cannot be read or every letter failed; exit 0 otherwise, with unchecked letters reported on stderr. Register in `cmd/autarch/main.go` after `threadsCmd()`.

Wiring in `cmd/autarch/door.go`: add `--graph-env PATH` (default ""), and `.WithLayers(door.LayersOptions{EnvPath: o.graphEnv})` after `.WithThreads(...)`.

Acceptance: `go build ./cmd/autarch` succeeds; `go vet ./cmd/autarch` clean; `autarch layers --json --env /nonexistent` exits 2 with a one-line stderr naming the path; on this machine `autarch layers --json | python3 -c 'import json,sys; r=json.load(sys.stdin); print(len(r), len({x["layer"] for x in r}))'` prints `51 11` (51 rows, 11 non-empty letters) within 30 seconds.

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

**[invented] (drift log, target zero):** none. Every constant above is either taken from an existing decision (2s from the recall hook decision; 8 workers and the footer shape from the threads screen) or recorded under auto_proceed.

**Executor note (capability routing):** this plan is written for a Sonnet executor with Opus validating against the acceptance lines above, not against its own judgment. If the executor cannot decode the MCP response in WI-1 after two attempts, stop and escalate; the fixture shapes are given, and a third guess is drift.

## Success audit plan

The spec's success_condition: "After a reshaping pass, mk's next week of walks feels lighter without anything important going dark — and when a signal annoys them, they fix it in the two seconds of the annoyance and recognize that as the Stellaris retune button they admired, working on their own estate." This slice is the reading half only; the retune button is step 2, out of scope. Translated two ways:

**Structured proxies (checkable at review):**

- met/unmet: every acceptance line above, run as written.
- met/unmet: a live render on this machine shows 51 rows across 11 letters, G and L marked `none`, and at least one row per populated letter carrying `last commit <age>`; the screen appears only after `l` is pressed.
- met/unmet: with the token file moved aside, the screen shows every letter unchecked with the path in the reason line, and the rest of Autarch is unaffected.
- met/unmet: `find ~/.autarch -newer <marker>` after a render lists nothing the layers path wrote (the door's own visit stamp excepted).
- unverifiable-by-machine: whether the rows read as the estate's pace.

**The recognition check (to mk, never automated away):** after one real render, two questions, one at a time. "Reading this by letter, which layer is misfiled: a probe sitting still on a fast letter, or something churning on a slow one?" And then: "Is this the pace lens you meant, or is the lens something the letters do not carry?" mk's words are quoted into the spec's provenance entry at v1.5.
