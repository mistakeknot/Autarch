---
artifact_type: plan
bead: mk-okek
stage: design
requirements:
  - S0: one `autarch serve` (build step 0)
  - S1: decisions owed, picked and ruled (build step 1)
---
# Home, steps 0 and 1: `autarch serve` and decisions

> **For Claude:** REQUIRED SUB-SKILL: Use clavain:executing-plans to implement this plan task-by-task.

**Bead:** `mk-okek` (Home epic, hub tracker, filed 2026-09-26 under G-0). S0 is `mk-okek.1`
and S1 is `mk-okek.2`; S1 depends on S0.

**Rulings (mk, 2026-09-26):**
- G-5 accepted (P-7 feed via `agents.configure`; P-5 wake scope). G-9: mark undeliverable in v1;
  successor resolution deferred.
- **Command picks are cut from v1, and signing is deferred to step 3 (Lattice).** An option can
  carry a pre-written **instruction**. Picking it wakes the asking agent with exactly that text,
  and the agent does the work under its own sandbox and gates. The feed is built from Home's own
  pick records, not from ruling files, so a hand-copied file never reaches an agent. Ruling files
  stay as the human-readable record, unsigned. This is recorded as a ruling in the brainstorm and
  revises `[D14]`, `[D16]`, `[D18]`, `[D21]` and `[D22]` for v1.
  - mk's reason: "I just want to context switch less and think more deeply about
    product/design/taste problems (and QAing what I should be QAing)." Home running commands as
    mk was where both reviews found the defects, and it saved tokens, which is no longer the
    governing aim.
  - This supersedes G-2 (Home's key), G-7 (no commands without a key) and G-8 (failed-command
    follow-ups). The revert command from `[D22]` goes with the command kind.

**Goal:** mk sees the decisions agents owe mk in one Aleph rail and picks an option. The pick is
recorded once, even when retried or interrupted, and the answer (with the asker's pre-written
instruction, if any) reaches the asking thread at least once. Each pick leaves a ruling file.
All of this is served by one loopback Go service.

**Architecture:**
- `autarch serve` is one loopback HTTP service. It mounts the existing Bigend daemon, Gurgeh
  and Signals handlers under path prefixes. It shares one Signals broker between them, and adds
  a token check, an Origin check and a decisions API.
- One `serve` process owns decisions. It takes a process-lifetime lock before it listens.
  Every write to a decision bead goes through that process, so the bead's metadata has one
  writer.
- MCP moves into the same binary as `autarch mcp`, still speaking stdio.
- Decisions live only as `decision` beads in the hub tracker, reached through `bd`.
- Home executes nothing. A pick is one metadata write plus a list of **obligations** (close the
  bead, write the ruling file, wake the asker). One reconcile function carries out obligations
  and is used by pick, by retries and by crash recovery.
- A thin bb plugin (`integrations/bb-plugin-autarch/`, entry `server.ts`) does four things:
  - starts the service;
  - renders the rail;
  - delivers wakes and the feed;
  - registers `bb home ask`.
  The browser never talks to the service directly; the plugin mediates every call.

**Tech Stack:**
- Go 1.25: `net/http` ServeMux patterns, cobra, `os.Root`, `syscall.Flock`.
- `bd` 1.1.2 against the hub Dolt tracker.
- The bb plugin SDK: TypeScript, `bb.server` entry, React panel, `testing/fake-sdk.ts`.

**Prior learnings / inputs:**
- [Brainstorm](../brainstorms/2026-09-24-one-place-in-aleph-brainstorm.md), decisions 13–23.
  They are cited here as `[Dn]` and are binding, except where mk's rulings above and under G-5
  narrow them. Decision 23 records the 2026-09-26 cut.
- [One-pager](../onepagers/2026-09-24-one-place-in-aleph.md).
- CUJs [autarch-07](../cujs/autarch-07-decide-and-continue.json) and autarch-09. autarch-07 still
  describes command continuations and signing; it is revised when G-6 walks it (see G-6).
- [Thread Organizer assessment](../research/assess-bb-thread-organizer.md) (SDK primitives).
- [Estate-map trial plan](2026-09-23-estate-map-trial-plan.md), WI-5. It planned
  `integrations/bb-plugin-autarch/` but never built it; this plan creates it, and WI-5 later
  adds its map panel to the same plugin.

---

## Decision context (routing)

```json
{"reasons":["foundational-invariants","broad-consequences"],
 "rationale":"Plan for autarch serve consolidation and the Home decisions queue: the decision-bead schema, pick records and wake obligations are a shared protocol for every agent, Mycroft and the rail, and picks carry mk's approval of instructions that agents then act on.",
 "investigation_active":false,"domain":"agent"}
```

- **Planning role:**
  - Planning resolved to planning-astra (gpt-6-astra), with fallback planning-opus.
  - This plan was authored by **claude-opus-5-5**, the running session and the policy's declared
    fallback seat. It was not authored by Astra.
- **Review role:**
  - Review resolved to review-astra (gpt-6-astra, xhigh), from a different lab.
  - review-opus is excluded (`producer_model_conflict`).
  - Dispatch: `$CLAVAIN_SELECTED_ROOT/scripts/dispatch.sh --role plan-review
    --producer-identity=claude-opus-5-5`, with `CLAVAIN_DECISION_CONTEXT` pointing at
    `thread-storage/thr_awr853efiy/home-plan-decision.json`.
- **Status:**
  - Round 1: **needs-rework**, 22 findings (18 P1, 4 P2), folded and cited `[A-n]`.
    Receipt: gpt-6-astra, xhigh, codex, 8m51s. Review at
    `thread-storage/thr_awr853efiy/home-plan-review.md`.
  - Round 2: **needs-rework**, 19 findings (18 P1, 1 P2), cited `[B-n]`. Most concentrated in
    Home running commands as mk exactly once. mk cut command picks and signing rather than
    grow that machinery (Rulings above). Review at
    `thread-storage/thr_awr853efiy/home-plan-review2.md`.
  - Round 3 on this revision is required before execution, because the plan is foundational.
  - Astra flagged two things as unverified in both rounds: the GitHub repository identity and
    the zklw CI status (`zklw-ci status`). Both remain open prerequisites for execution.

### Where each round-2 finding landed

| Finding | Theme | Disposition |
|---|---|---|
| B-1 | Replay after an interrupted pick | Pick request id (P-4); retries return the recorded pick |
| B-2 | Incomplete state machine | Removed: no execution states. Obligations only (P-4, Task 1.4) |
| B-3 | Stale re-ask stranded | Reconcile lists every bead with an unfinished obligation, open or closed (Task 1.4) |
| B-4 | Ack deletes a newer wake | Ack names `(bead, obligation)`; the label is a projection (P-5, Task 1.4) |
| B-5 | Lock released while a child survives | Removed: Home starts no processes |
| B-6 | Precondition before evidence | Removed with commands |
| B-7 | argv defeats the freeze | Removed with commands |
| B-8 | `FindOp` + `Create` not atomic | Explicit deterministic bead id plus one writer (P-3a, Tasks 1.2, 1.4) |
| B-9 | Supersession bypasses ownership | Supersession under the owner, as an obligation (Task 1.4) |
| B-10 | Follow-up exceeds option limit | Removed with failed-command follow-ups |
| B-11 | Feed fields not bound to the signed file | Removed with signing; the feed reads Home's pick record (P-8) |
| B-12 | Feed cache leaks another thread's answers | Thread answers keyed by thread, selected at read time (Tasks 1.5, 1.9) |
| B-13 | Interrupted and undeliverable cards invisible | No interrupted state; undeliverable listed regardless of age (Tasks 1.5, 1.9) |
| B-14 | Unsupported stdin transport; exit codes | `--request-stdin` named option; exit-code error type (Tasks 1.7, 1.9) |
| B-15 | Mycroft dispatch is a fake success | Mycroft files ruling-only decisions; no dispatch action in v1 (Task 1.8) |
| B-16 | Step 0 imports step 1 | `Config.Decisions` added only in Task 1.6 (Task 0.2) |
| B-17 | Tracker-down start cannot serve | Listen first; reconcile retries; mutations 503 until done (Task 0.2, Task 1.6) |
| B-18 | Criterion 14 passes with no evidence | Output path passed explicitly; exact scenario set and commit checked (criteria 12, 13) |
| B-19 | Pollard's publisher stranded | Standalone Signals stays for Pollard, documented (Task 0.6) |
| A-19 residue | `filepath.Clean` in writes | Ruling files written through `os.Root` (Task 1.3) |
| A-11 residue | D15 helper follow-up unnamed | Filed as a child bead at execution (Task 1.11, Out of scope) |

Round-1 findings `[A-n]` stay cited where their fixes survive. Those that concerned commands,
the runner, the freeze, revert or signing (A-2, A-3, A-5, A-6, A-7, A-9, A-18's crash
scenarios, A-21's follow-up) are removed along with that machinery.

## Plan-level decisions (settled here; reviewers, push on these)

- **P-1: Gurgeh goes multi-project by path, not by process.**
  - `/gurgeh/{project}/…` resolves `{project}` through `projects.Resolve` (Task 0.2). It builds
    a `gurgeh/server.Server` for that root lazily and caches it.
  - An uninitialized project returns 404 `{"error":"gurgeh not initialized"}`. The service
    never runs `EnsureInitialized`, because that writes into the project.
- **P-2: MCP attaches as a subcommand, not as a shim.**
  - `autarch-mcp` is spawned per session by the agent host and reads project files directly.
  - A stdio shim that proxies to HTTP would need HTTP endpoints for PRDs and tasks that don't
    exist. It would add a hop and a failure mode with no user-visible gain.
  - So step 0 moves MCP into the one binary as `autarch mcp` and keeps `cmd/autarch-mcp` as a
    two-line alias. Step 1 adds one MCP tool, `autarch_file_decision`, which files through the
    service like the CLI.
  - A shim is revisited when the service holds state MCP needs.
- **P-3: The service has a token and an Origin check even on loopback** `[A-22]`.
  - A pick sends mk's approval to an agent. A loopback-only guard does not stop a browser page
    (through DNS rebinding or a form post) or any local process from reaching it.
  - Every route except `GET /health` needs:
    - `Authorization: Bearer <token>`. The token lives in `~/.autarch/serve.token` (0600) and is
      created on first run. It is accepted **only** as a header. There is no `?token=`
      fallback, including on WebSocket routes.
    - A `Host` header that is `127.0.0.1:<port>` or `localhost:<port>`.
    - No `Origin` header, or an `Origin` in `--allow-origin`. The allowlist is empty by
      default, so any browser-originated request is refused with 403, WebSocket upgrades
      included. This also covers Bigend's `CheckOrigin` that accepts every origin
      (`internal/bigend/daemon/server.go:380`), because the middleware rejects the request
      before it reaches that handler.
  - The rail reaches the service only through plugin RPC, never through a browser WebSocket.
  - Existing standalone servers are unchanged.
  - The token is the same-user boundary accepted in `[D21]`: an agent running as mk can read
    it. What it stops is a browser page or another user.
- **P-3a: One decisions owner and one writer** `[A-1]` `[B-8]`.
  - `serve.Run` takes a non-blocking `flock` on `~/.autarch/decisions.lock` and holds it for the
    life of the process, before it listens.
  - A second `serve` fails fast with `another autarch serve owns decisions (pid N)`, even when
    it was given a different port.
  - Every decision write — file, pick, ack, supersede, reconcile — goes through the owner's API
    and runs under one service mutex. `autarch decide file`, the MCP tool, `bb home ask` and
    Mycroft are all API clients. None of them writes to the tracker directly.
- **P-4: A pick is a record plus obligations, and it is idempotent** `[B-1]` `[B-2]`.
  - Owed means an open bead with label `home:decision` and no `pick` in its metadata.
  - The rail sends a `pick_id` it generated for that click. A pick writes, in one `bd update`,
    the `pick` record (pick id, option, revision, time) and its obligations, and adds
    `home:pending`.
  - A retry with the same `pick_id` returns the recorded pick. A different `pick_id` on a bead
    that already has a pick returns 409 `already ruled`.
  - Obligations are `close`, `ruling-file`, `wake` and `close-superseded`. Each has an id. The
    reconcile function carries out the ones not done; each is idempotent. The label
    `home:pending` is a projection: present exactly when some obligation is neither done nor
    undeliverable.
  - Nothing else is state. There is no running, interrupted, stale or failed state, because
    Home runs nothing.
- **P-5: Wakes are scoped, per obligation and at least once** `[A-4]` `[A-11]` `[A-15]` `[B-4]`.
  - Wakes go only to **instruction** and **needs-context** picks. Ruling-only outcomes reach the
    asking thread through the feed, which includes the thread's own answers (Task 1.5). Ruled
    by mk under G-5.
  - The plugin drains pending wakes and sends one message per thread `[D13]`. It then acks each
    `(bead, obligation)` pair it sent. Acking one obligation never touches another, so a wake
    added while a send is in flight stays pending. At worst a wake is sent twice; it is never
    lost.
  - A send that bb rejects (the thread is archived or deleted) marks that obligation
    `undeliverable`. The rail lists every undeliverable wake, however old, with its answer and
    instruction so mk can hand it on, until mk dismisses it. There is no retry loop.
    Resolving a successor thread is deferred (G-9, ruled).
- **P-6: Needs-context is the default, and instructions are text** `[D22]` (revised by the
  2026-09-26 ruling).
  - Kinds are `instruction`, `needs-context` and `ruling-only`. An option with no kind and no
    instruction is `needs-context`. An option with an `instruction` and no kind is
    `instruction`. An instruction on another kind is rejected.
  - An instruction is plain text of at most 2,000 characters, written by the asker when filing.
    The rail shows it exactly. The wake carries it exactly. Home does not parse, run or check
    it; the agent acts on it under its own sandbox and permission gates.
  - Only an asker with a thread can offer `instruction` or `needs-context`. Mycroft files
    ruling-only options (Task 1.8).
- **P-7: The feed goes through `bb.agents.configure`, not a per-turn hook** (open question 5
  candidate) `[A-11]`.
  - The service exposes a consumer-agnostic `GET /api/decisions/feed`.
  - The plugin injects it at session start and resume from an async cache (Task 1.9).
  - The Clavain `UserPromptSubmit` hook `[D17]` is deferred until the trial shows that rulings
    made mid-session and arriving late are costly.
  - **This departs from the letter of `[D17]`, and with P-5 it narrows which answers wake a
    thread. mk accepted both under G-5 on 2026-09-26.**
- **P-8: The feed comes from Home's pick records, not from files** (replaces signing; mk,
  2026-09-26).
  - A feed line is built only from a closed decision bead whose metadata has a `pick` that Home
    wrote. It reads the picked option's label from the options stored at filing, never from the
    ruling file.
  - A bead closed by hand without a Home pick never reaches the feed. A ruling file written by
    hand reaches no agent, because nothing reads ruling files.
  - An agent could still edit bead metadata with `bd` as mk. That is the same-user risk
    accepted in `[D21]`; signing in step 3 is the upgrade path.
- **P-9: Home does not commit ruling files.**
  - It writes `docs/decisions/<file>.md` into the project's working tree.
  - Committing is left to the project's normal flow: autosync, or the next agent commit.
  - This keeps Autarch from writing git history, which is world state.

## Must-Haves

**Truths:**
- `autarch serve` answers `GET /health` with every mounted subsystem's status. It refuses
  `--addr 0.0.0.0:…`, returns 401 without the token and 403 for an Origin not allowed. A second
  `serve` fails fast.
- Existing clients keep working under prefixes:
  - `/bigend/api/sessions`;
  - `/gurgeh/{project}/api/specs`;
  - `/signals/ws`, which carries Gurgeh's signals from the same process.
- An agent inside a bb thread runs `bb home ask` (or `autarch decide file`) and one `decision`
  bead appears in the hub tracker, even if the call is retried or races another retry. The bead
  carries the question, the revision, the options (each with an id, a kind and any
  instruction), the project and the asking thread. If Home or the tracker is down, the agent is
  told whether nothing was filed (exit 3) or whether the outcome is unknown and a retry is safe
  (exit 4).
- The rail lists owed decisions across projects. It shows each option's label and kind, each
  instruction exactly with "sent to <thread> as written", the recommendation, undeliverable
  wakes, and "tracker down since …" when the tracker is unreachable.
- A pick is recorded once, including across a retry or SIGKILL of `serve`. The bead closes, a
  ruling file is written, and for instruction and needs-context picks the asking thread
  receives one message carrying the picked label and the exact instruction.
- New sessions in a project receive that project's recent rulings, plus the thread's own
  answers, as label only, from Home's pick records.
- Mycroft's suggestions and out-of-allowlist dispatches appear on the same rail as ruling-only
  decisions. `DecisionQueue` holds no private list. No option claims to dispatch.

**Artifacts:**
- `internal/serve/serve.go` exports `New(Config) (*Service, error)`, `(*Service).Handler()` and
  `(*Service).Run(ctx)`. `internal/serve/projects.go` exports `Resolver` and `Resolve(name)`.
- `cmd/autarch/serve.go` provides `serveCmd()`. `cmd/autarch/mcp.go` provides `mcpCmd()`.
  `cmd/autarch/decide.go` provides `decideCmd()` with `file`, `list` and `stats`.
  `cmd/autarch/exit.go` provides `ExitError{Code}`.
- In `internal/decisions/`:
  - `model.go`: `Decision`, `Option`, `Kind`, `Pick`, `Obligation`, `Validate`, `Revision`,
    `BeadID`.
  - `tracker.go`: the `Tracker` interface, `BDTracker`, `ErrTrackerDown`, `ErrOutcomeUnknown`,
    `ErrExists`.
  - `ruling.go`: `Ruling`, `WriteRuling`, `RulingPath`.
  - `lock.go`: `AcquireOwnerLock`.
  - `service.go`: `Service`, with `File`, `Owed`, `Pick`, `Reconcile`, `Recent`,
    `Undeliverable`, `Feed`, `Wakes`, `Ack` and `Stats`.
  - `client.go`: `Client`, the HTTP client every filer uses.
  - `api.go`: `Routes(mux, svc)`.
- `internal/mycroft/escalate/escalate.go`: `DecisionQueue` backed by a `decisions.Filer`.
- `internal/homee2e/`: the scenario harness behind build tag `homee2e`.
- `integrations/bb-plugin-autarch/`: `package.json`, `server.ts`, `contract.ts`, `feed.ts`,
  `app.tsx`, `__tests__/`, `README.md`.

**Key links:**
- `serve.Run` runs: `EnsureLocalOnly` → `AcquireOwnerLock` → `Listen` → `Reconcile` in the
  background, retried every 30 s until it has completed once. Reads and `/health` work at once;
  decision writes return 503 until the first reconcile completes `[B-17]`.
- `serve.New` creates one `signals.Broker`, gives it to `signals.NewServer`, and gives
  `broker.Publish` to every Gurgeh server it builds.
- `decisions.Service.Pick` runs, under the mutex: `Get` → same `pick_id`? return it → one
  `Update` writing `pick` + obligations + `home:pending` → `Reconcile(bead)`.
- `Reconcile(bead)` carries out, in order: `close-superseded`, `close`, `ruling-file`. Then one
  `Update` marks them done and recomputes `home:pending`. `wake` obligations stay pending until
  the plugin acks them.
- `autarch decide file`, `bb home ask`, the MCP tool and Mycroft all call
  `POST /api/decisions`, which calls `decisions.Service.File`. There is one validation path
  and one writer.
- The plugin host holds the token. The panel calls RPC only.

---

## Step 0: one `autarch serve`

Step 0 finishes, and its tests pass, before any step 1 task starts `[A-17]`. No step 0 file
imports `internal/decisions` `[B-16]`.

### Task 0.1: expose the existing servers as handlers

**Files:**
- Modify: `internal/bigend/daemon/server.go:38-86`. Add `Handler() http.Handler`, returning
  `s.mux` (routes are already set in `NewServer`).
- Modify: `internal/gurgeh/server/server.go:25-48`. Add `Handler()`, which calls `routes()`
  once through a `sync.Once`. `ListenAndServe` uses the same once.
- Modify: `pkg/signals/server.go:21-54`. Same pattern: `Handler()` plus `sync.Once`.
- Test: `internal/bigend/daemon/handler_test.go`, `internal/gurgeh/server/handler_test.go`,
  `pkg/signals/handler_test.go`.

**Steps:**
1. Write failing tests. Each builds the server, wraps `Handler()` in `httptest.NewServer`, and
   asserts `GET /health` returns 200. The Gurgeh test uses a temp root with an empty specs
   directory. The Signals test also asserts `Handler()` is idempotent: calling it twice does not
   panic on duplicate route registration.
2. Run `go test -race ./internal/bigend/daemon/ ./internal/gurgeh/server/ ./pkg/signals/`.
   Expect FAIL (`Handler undefined`).
3. Implement.
4. Rerun and expect PASS.
5. Commit: `serve: expose bigend, gurgeh and signals as handlers`.

<verify>
- run: `go test -race ./internal/bigend/daemon/ ./internal/gurgeh/server/ ./pkg/signals/`
  expect: exit 0
</verify>

### Task 0.2: `internal/serve`, one mux, and project resolution

**Files:**
- Create: `internal/serve/serve.go`, `internal/serve/auth.go`, `internal/serve/gurgeh.go`,
  `internal/serve/projects.go`.
- Test: `internal/serve/serve_test.go`, `internal/serve/serve_bind_test.go`,
  `internal/serve/projects_test.go`.

**Design:**
- `Config{Addr, ProjectDirs []string, TokenPath string, AllowOrigins []string}`. There is no
  decisions field in step 0; Task 1.6 adds it `[B-16]`.
  - The default `Addr` is `127.0.0.1:8110`. It is new, so it does not collide with a running
    Bigend daemon on 8100 during the transition.
- `projects.go` `[A-19]`:
  - `Resolver` is built from the Bigend project list. Each root goes through
    `filepath.EvalSymlinks` and must stay inside one of the configured `ProjectDirs` (also
    resolved).
  - `Resolve(name)` maps a base name to one resolved root. A base name shared by two roots is
    an error (`ambiguous project "x": /a/x, /b/x`), never a silent first match.
  - The list is re-read at most every 30 s, so a retargeted symlink is noticed.
- Mounts:
  - `/bigend/` uses `http.StripPrefix("/bigend", daemon.Handler())`.
  - `/signals/` uses `StripPrefix` over the Signals handler (Task 0.3 makes it shared).
  - `/gurgeh/{project}/` uses `Resolve` (P-1) and caches `gurgeh.Server` per resolved root with
    a mutex. An unknown project returns 404, and so does one without a `.gurgeh` directory.
- `GET /health` needs no token. It returns
  `{"status":"ok","bigend":…,"gurgeh":{"projects":N},"signals":…}`. Task 1.6 adds `decisions`.
- `auth.go` implements P-3:
  - `LoadOrCreateToken(path)` makes 32 random bytes, hex-encoded, and writes the file with mode
    0600. It refuses a token file whose mode is looser than 0600.
  - The middleware checks, in order: `Origin` (absent or allowlisted, else 403), `Host` (else
    403), then the bearer header with `subtle.ConstantTimeCompare` (else 401).
  - There is no query-string token.
- `Run(ctx)`:
  1. `netguard.EnsureLocalOnly(Addr)`;
  2. an optional `BeforeListen func() error` hook (Task 1.6 sets it to take the owner lock);
  3. `Listen`, then `Serve`;
  4. an optional `AfterListen func(ctx)` hook, run in a goroutine (Task 1.6 sets it to the
     reconcile loop);
  5. `Shutdown` with a 5 s timeout when `ctx` is done.

**Steps:**
1. Write failing tests:
   - `/health` returns 200 without a token.
   - `/bigend/api/sessions` returns 401 without the token, and 200 with it.
   - `?token=<valid>` with no header returns 401 `[A-22]`.
   - A bad `Host` (`evil.example:8110`) returns 403.
   - `Origin: http://evil.example` with a valid token returns 403, on a plain route and on a
     WebSocket upgrade to `/signals/ws`.
   - A WebSocket upgrade to `/signals/ws` with the bearer header and no Origin succeeds.
   - `/gurgeh/demo/api/specs` returns 200 for an initialized temp project and 404 for an unknown
     one.
   - `Resolve` refuses duplicate base names, refuses a root whose symlink points outside
     `ProjectDirs`, and follows a symlink retargeted between two calls once the list is
     re-read.
   - The token file is created with mode 0600, and a 0644 token file is refused.
   - A `BeforeListen` error stops `Run` before it binds (the port stays free).
   - Bind test: `Run` with `0.0.0.0:0` returns the netguard error. `[::]:0` likewise.
2. Run and expect FAIL.
3. Implement.
4. Run and expect PASS.
5. Commit: `serve: one loopback mux with token and origin checks`.

<verify>
- run: `go test -race ./internal/serve/`
  expect: exit 0
- run: `go test -race -run 'Bind' ./internal/serve/`
  expect: exit 0
- run: `! go list -deps ./internal/serve/ | grep -q internal/decisions`
  expect: exit 0
</verify>

### Task 0.3: one Signals broker, and Gurgeh publishes into it `[A-14]`

**Files:**
- Modify: `internal/gurgeh/server/server.go`. `New(root)` stays. Add
  `WithPublisher(func(signals.Signal))`, an option the caller sets. `refreshSignals` (`:132`)
  uses the publisher when set, and keeps today's `psignals.NewClient(psignals.DefaultServerURL())`
  path otherwise, so the standalone Gurgeh server is unchanged.
- Modify: `internal/serve/serve.go`. `New` calls `signals.NewBroker()` once and passes it to
  `signals.NewServer(broker)`. Each Gurgeh server built by the resolver gets
  `WithPublisher(broker.Publish)`.
- Test: `internal/serve/signals_test.go`, `internal/gurgeh/server/publisher_test.go`.

**Steps:**
1. Write failing tests:
   - The Gurgeh unit test: with a publisher set, a spec update calls it and makes no HTTP call
     (the default URL points at a closed port and must not be dialled).
   - The serve test: subscribe to `/signals/ws` with the token, trigger a Gurgeh spec update
     through `/gurgeh/demo/…`, and receive the signal within 2 s.
2. Implement, rerun, commit: `serve: one signals broker shared with gurgeh`.

<verify>
- run: `go test -race ./internal/serve/ ./internal/gurgeh/server/`
  expect: exit 0
</verify>

### Task 0.4: the `autarch serve` command

**Files:**
- Create: `cmd/autarch/serve.go`.
- Modify: `cmd/autarch/main.go:74-88` (`root.AddCommand(serveCmd())`).
- Test: `cmd/autarch/serve_test.go`.

**Flags:**
- `--addr` (default `127.0.0.1:8110`);
- `--project-dir` (repeatable; defaults to the Bigend daemon's discovery defaults);
- `--token-file` (default `~/.autarch/serve.token`);
- `--allow-origin` (repeatable; default none);
- `--hub-dir` (default `/home/mk/hub`, or `$AUTARCH_HUB_DIR`; used from step 1).

SIGINT and SIGTERM cancel the context. The startup line prints the address and the token file
path, never the token.

**Steps:**
1. Write failing tests: `serveCmd()` with `--addr 0.0.0.0:0` returns an error containing
   `local`. A second test starts on `127.0.0.1:0`, polls `/health`, and cancels.
2. Implement.
3. Run `go build ./cmd/... && go test -race ./cmd/autarch/ -run Serve`.
4. Commit: `autarch serve: one command for the consolidated service`.

<verify>
- run: `go build ./cmd/...`
  expect: exit 0
- run: `go test -race ./cmd/autarch/ -run Serve`
  expect: exit 0
</verify>

### Task 0.5: MCP into the one binary (P-2)

**Files:**
- Create: `cmd/autarch/mcp.go` (`autarch mcp --project <dir>`, which calls
  `mcp.NewServer(path).Run(ctx)`).
- Modify: `cmd/autarch/main.go` (`root.AddCommand(mcpCmd())`).
- Modify: `cmd/autarch-mcp/main.go`. Keep its flags and call the same function; add a stderr
  line: `autarch-mcp is an alias for "autarch mcp"`.
- Modify: `autarch-plugin/.claude-plugin/plugin.json`. The MCP server command becomes
  `autarch mcp`. **This is a ship-class file:** fd-safety reviews it, and the plugin is
  republished only with mk's approval.
- Test: `cmd/autarch/mcp_test.go`. It pipes an `initialize` request plus `tools/list` through
  stdin and asserts `autarch_list_prds` is in the result.

**Steps:** write the test, confirm it fails, implement, confirm it passes, then commit:
`mcp: serve from the autarch binary; autarch-mcp stays as an alias`.

<verify>
- run: `go test -race ./cmd/autarch/ -run MCP`
  expect: exit 0
</verify>

### Task 0.6: point the old entry points at `autarch serve`

**Files:**
- Modify: `cmd/bigend/main.go:104` (`runDaemon`), `cmd/autarch/main.go` (the Bigend
  `--daemon` path near `:345`) and `internal/gurgeh/cli/commands/serve.go`.
  - Each keeps working unchanged, and prints one stderr line:
    `deprecated: use "autarch serve" (mounted at /<prefix>/)`.
  - Nothing is removed in this plan.
- **Not** `internal/signals/cli/serve.go` `[B-19]`. Pollard's watcher publishes through an
  unauthenticated Signals client aimed at the standalone server on 8092. Pointing it at the
  consolidated endpoint would get 401. So the standalone Signals server stays, undeprecated,
  as Pollard's target. The fix — endpoint and token settings for external Signals clients —
  is a follow-up bead filed at execution (Task 1.11).
- Modify: `./dev` to add `./dev serve`, which runs `go run ./cmd/autarch serve`.
- Modify: `AGENTS.md` (the tool command reference) to document `autarch serve`, its prefixes,
  the token, the Origin allowlist, and that Pollard still publishes to the standalone Signals
  server.

**Steps:** there are no behavior tests beyond the existing ones. Run `go test -race ./cmd/...
./internal/gurgeh/... ./internal/signals/...`, then commit: `serve: deprecation notices on the
standalone servers`.

<verify>
- run: `go test -race ./cmd/... ./internal/gurgeh/... ./internal/signals/...`
  expect: exit 0
</verify>

**Pollard's server (8090) is not mounted.** The one-pager's list is Bigend, Gurgeh, Signals and
MCP. Pollard joins if the rail needs it.

---

## Step 1: decisions

### Bead schema (the shared protocol)

A decision owed is a hub-tracker bead with these properties:
- `id`: `mk-h<10 lowercase base32 chars>`, derived from the request id (Task 1.1) `[B-8]`.
- `type`: `decision`.
- `title`: the question, clipped to 120 runes.
- `description`: the question in full.
- `labels`:
  - always `home:decision`;
  - `project:<name>` (or `project:estate` for estate-wide);
  - `asker:<thread-id|mycroft>`;
  - `home:pending` while any obligation is open, and `home:undeliverable` while any wake is
    undeliverable and not dismissed. Both are projections of `pick.obligations`.
- `metadata` is one key, `home`, whose value is versioned JSON:

```json
{"v":3,
 "project":"Autarch","project_dir":"/home/mk/projects/Autarch",
 "asking_thread":"thr_abc123","asker":"thread",
 "request_id":"req_7f3a…",
 "revision":"sha256:…",
 "recommendation":"merge",
 "supersedes":"",
 "options":[
   {"id":"merge","label":"Merge feat/bb-catchup","kind":"instruction",
    "instruction":"Fast-forward main to feat/bb-catchup, run go test -race ./..., push, and report the result."},
   {"id":"wait","label":"Wait for review","kind":"needs-context"},
   {"id":"never","label":"Never merge it","kind":"ruling-only"}],
 "pick":{"pick_id":"pk_2b9e…","option_id":"merge","revision":"sha256:…","by":"home",
         "at":"2026-09-26T14:03:00Z",
         "ruling":"docs/decisions/2026-09-26-merge-feat-bb-catchup-mk-h3k2….md",
         "obligations":[
           {"id":"ob1","kind":"close","state":"done"},
           {"id":"ob2","kind":"ruling-file","state":"done"},
           {"id":"ob3","kind":"wake","state":"pending"}]}}
```

- `revision` is sha256 over the canonical decision: question, option ids, labels, kinds,
  instructions, recommendation and `supersedes` `[A-8]`. Any edit changes it.
- `pick` is absent until mk picks, and is written once. There is no `history[]`: nothing
  re-picks a bead.
- An obligation's `state` is `pending`, `done`, `undeliverable` or `dismissed` (the last two
  only for `wake`).
- **The feed never carries the asker's free text.** It reads only the picked option's label
  `[D21]`. The instruction goes back only to the thread that wrote it, in its wake.

### Task 1.1: model, validation, revision and bead id

**Files:**
- Create: `internal/decisions/model.go`.
- Test: `internal/decisions/model_test.go`.

**Design:**
- `Kind` is one of `instruction`, `needs-context` or `ruling-only` (P-6).
- `Validate`:
  - no kind and no instruction means `needs-context`; an instruction and no kind means
    `instruction`; an instruction on any other kind is an error;
  - an instruction is 1–2,000 characters after trimming, valid UTF-8, with no NUL;
  - option ids are unique, `[a-z0-9-]{1,32}`, and default to a slug of the label;
  - 2–6 options;
  - labels are non-empty and ≤ 80 runes;
  - `recommendation` names an existing option id;
  - an asker with no thread (`asker: mycroft`) may offer only `ruling-only`;
  - `request_id` is 1–128 characters of `[A-Za-z0-9:_.-]`.
- `Revision(d)` hashes the canonical decision `[A-8]`.
- `BeadID(requestID)` is `mk-h` plus the first 10 characters of lowercase base32 of
  sha256(`home-decision:` + request id). The same request id always gives the same bead id
  `[B-8]`. The prefix follows the hub's `mk` prefix, so `bd create` needs no `--force`.
- `Pick.Pending()` and `Pick.Undeliverable()` compute the two projected labels from the
  obligations.

**Test cases:**
- Validation: every rule above, plus inference of both default kinds.
- An instruction on a ruling-only option is rejected with the option id in the error.
- Mycroft offering `needs-context` is rejected.
- Editing an option label, an instruction or `supersedes` changes `Revision`. Reordering JSON
  keys does not.
- `BeadID` is stable across runs and differs for different request ids.
- The projections: an obligation list with one pending wake gives `pending`; all done gives
  none; one undeliverable wake gives `undeliverable` and not `pending`.

Commit: `decisions: option model, validation, revision and bead id`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Validate|Revision|BeadID|Projection'`
  expect: exit 0
</verify>

### Task 1.2: the `bd` tracker

Depends on Task 1.1 (it stores `Decision`) `[A-17]`.

**Files:**
- Create: `internal/decisions/tracker.go`.
- Test: `internal/decisions/tracker_test.go`, using a fake `bd` script on PATH via
  `t.Setenv("PATH")`, as in `internal/door/product_test.go:99`. The fake records its argv to a
  file and prints canned JSON.

**Step 0 (reproduce before relying on it)** `[B-8]`: in a throwaway directory, `bd init` a
scratch database and run `bd create --id mk-htest00001 …` twice. Record whether the second call
fails and with what message. If `bd` rejects the duplicate, `ErrExists` matches that message.
If it does not, stop and report: Task 1.4's idempotent filing then falls back to the owner
mutex alone, which does not cover a create that commits after its timeout, and that residual
goes to mk before execution continues. The scratch database is deleted afterwards; the hub is
never touched.

**Design:**
- `BDTracker{HubDir, Timeout, LabelPrefix}` runs `bd` with `cmd.Dir = HubDir`, a 5 s timeout per
  call and a 4 MiB output cap, matching `internal/door/product.go:244-296`. `LabelPrefix`
  defaults to `home` (Task 1.11 sets `hometest`).
- `Create(d)` runs
  `bd create --id <BeadID> --title … -d … -t decision -l home:decision,project:X,asker:Y --metadata @tmp.json --json`.
  Metadata goes through a temp file, never argv. A duplicate id gives `ErrExists`.
- `Get(id)` runs `bd show id --json`, and returns `ErrNotFound` for an unknown id.
- `ListOwed()` runs `bd list -l home:decision -s open -n 0 --json`.
- `ListByLabel(label)` runs `bd list -l <label> --all -n 0 --json`. It serves reconcile
  (`home:pending`) and the undeliverable list (`home:undeliverable`), open or closed `[B-3]`
  `[B-13]`.
- `ListRecent(since)` runs
  `bd list -l home:decision --all --closed-after <since> --sort closed -n 0 --json` `[A-20]`.
- `Update(id, meta, add, remove []string)` runs `bd update id --metadata @tmp.json --add-label …
  --remove-label …` as one call.
- `Close(id, reason)` runs `bd close --reason`.
- Errors:
  - `ErrTrackerDown`: a connection error (`connection refused`, `dial tcp`, `dolt`) **before**
    any write was sent. The tracker records `downSince` and clears it on the next success.
  - `ErrOutcomeUnknown`: a timeout or connection error on a write call, where the write may
    have landed `[A-10]`.
  - Other failures surface as-is.

**Test cases:**
- The argv for each call, including `--id`, `--closed-after` and `--sort closed`.
- Metadata passes through `@file`, and the file is removed afterwards.
- The fake's duplicate-id message (copied from step 0) gives `ErrExists`.
- A connection-refused fake on `list` gives `ErrTrackerDown` with `DownSince()` set; the next
  success clears it.
- A fake that sleeps past the timeout on `create` gives `ErrOutcomeUnknown`.
- A 5 MiB output is truncated with an error.

Commit: `decisions: bd tracker with explicit ids and outcome-unknown`.

<verify>
- run: `go test -race ./internal/decisions/ -run Tracker`
  expect: exit 0
</verify>

### Task 1.3: ruling files and the owner lock

Depends on Task 1.1.

**Files:**
- Create: `internal/decisions/ruling.go`, `internal/decisions/lock.go`.
- Test: `internal/decisions/ruling_test.go`, `internal/decisions/lock_test.go`.

**Ruling format** `[D16]` (unsigned in v1; signing arrives with Lattice in step 3):

```markdown
---
artifact_type: ruling
ratification:
  ruled_by: mk
  ruled_at: 2026-09-26T14:03:00Z
  ruling: "Merge feat/bb-catchup"
  transcribed_by: autarch-home
  session_id: <asking thread>
  source: bead:mk-h3k2…
  supersedes: ""            # the superseded decision's bead, if any
home:
  bead: mk-h3k2…
  pick_id: pk_2b9e…
  revision: "sha256:…"
  asking_thread: thr_abc123
  options_shown: [{id: merge, label: "Merge feat/bb-catchup", kind: instruction}, …]
  picked: merge
  instruction: "Fast-forward main to feat/bb-catchup, …"   # instruction picks only
---
# <question>

mk picked **Merge feat/bb-catchup**. Home recorded this pick; this file is the readable copy,
not the record agents read.
```

**Rules:**
- `RulingPath(d)`:
  - A project ruling goes to `docs/decisions/YYYY-MM-DD-<slug>-<bead>.md` under the project
    root. The bead id in the name makes it unique, so there is no collision search, and
    rewriting it after a crash writes the same file.
  - An estate ruling goes to `rulings/…` under `$AUTARCH_UQBAR_DIR`. If the Uqbar is unset,
    filing `project:estate` is refused at `File` time, with the message
    `estate-wide decisions need an Uqbar (see G-1)`.
- **Safe writes through `os.Root`** `[A-19]`: `WriteRuling` opens the resolved project root (or
  Uqbar) with `os.OpenRoot`, creates `docs/decisions` with `Root.MkdirAll`, writes a temp file
  with `Root.OpenFile(O_CREATE|O_EXCL|O_WRONLY)`, fsyncs it, and renames it with `Root.Rename`.
  `os.Root` refuses any path, including through a symlink, that leaves the root, so there is
  no `filepath.Clean` check to get wrong.
- `AcquireOwnerLock(path)` `[A-1]` takes `syscall.Flock(LOCK_EX|LOCK_NB)`, writes the pid into
  the file, and returns a release function. On `EWOULDBLOCK` it returns `ErrOwned{PID}`.

**Test cases:**
- Round trip: write the frontmatter, parse it, get the same values, including `options_shown`
  and `instruction`.
- Writing the same ruling twice produces one file with the same bytes.
- A `docs` or `docs/decisions` symlink pointing outside the root is refused, and nothing is
  written outside it.
- An instruction containing `---` and YAML-looking lines round-trips as a string and does not
  break the frontmatter.
- `AcquireOwnerLock` in a second **process** (the test re-execs itself) returns `ErrOwned`.

Commit: `decisions: ruling files and the owner lock`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Ruling|Lock'`
  expect: exit 0
</verify>

### Task 1.4: the service — file, pick, reconcile, ack

Depends on Tasks 1.1–1.3 and on `internal/serve/projects.go` (Task 0.2), injected as an
interface so `decisions` does not import `serve`.

**Files:**
- Create: `internal/decisions/service.go`.
- Test: `internal/decisions/service_test.go`, using an in-memory `fakeTracker` that implements
  `Tracker`, enforces unique ids like step 0 of Task 1.2 found, and can inject `ErrTrackerDown`
  or `ErrOutcomeUnknown` on any call — including "the write landed, then the call timed out".

Every method below runs under the service mutex. The process already holds the owner lock
(P-3a).

**`File(ctx, req)`** `[A-10]` `[A-19]` `[B-8]` `[B-9]`:
1. Resolve `req.Project` through the resolver. `project_dir` must equal the resolved root.
2. `Validate`, compute `Revision` and `BeadID(req.RequestID)`.
3. `Get(beadID)`. If the bead exists:
   - with the same revision, return it and run `Reconcile` on it (this finishes a supersession
     whose close did not land);
   - with a different revision, return 409 `request id reused for a different decision`.
4. If `req.Supersedes` is set, check the predecessor now: it must exist, be open, have the same
   asker and project, and have no `pick`. If it has been picked, return 409
   `already ruled: "<label>"` so the asker reads the answer instead of re-asking.
5. `Create`. With a supersession, the new bead's metadata carries a `supersede` record with one
   `close-superseded` obligation, and the label `home:pending`.
   - `ErrExists` means an earlier attempt landed: go to step 3's path.
   - `ErrOutcomeUnknown`: `Get` once more. Found: continue as step 3. Not found: return the
     "may have been filed; re-running the same command is safe" error. A retry with the same
     request id cannot create a second bead, because the id is the same.
   - `ErrTrackerDown` before create gives the fixed message
     `hub tracker unreachable since <t>: nothing was filed; ask mk in chat instead` `[D21]`.
6. `Reconcile` the new bead (closes the predecessor with reason `superseded by <id>`).

**`Pick(ctx, id, optionID, revision, pickID)`** `[B-1]`:
1. Refuse with 503 if the first reconcile has not completed since start.
2. `Get` the bead.
   - It has a `pick` with this `pick_id`: return the recorded pick, then `Reconcile`.
   - It has a `pick` with another id, or it is closed: 409 `already ruled`.
   - `revision` differs from the stored one: 409 `ErrRevisionMismatch`; the rail re-reads.
3. One `Update`: `pick{pick_id, option_id, revision, by: "home", at, ruling, obligations}` and
   `+home:pending`. Obligations are `close` and `ruling-file`, plus `wake` for instruction and
   needs-context picks. On `ErrOutcomeUnknown` return 503 `retry with the same pick id`; the
   retry takes step 2's first branch if the write landed.
4. `Reconcile(bead)`, then return the pick.

**`Reconcile(ctx, bead)`** `[B-3]` — the one function that carries out obligations:
1. `close-superseded`: `Get` the predecessor; if open, `Close(pred, "superseded by <id>")`.
2. `close`: if the bead is open, `Close(id, "ruled: <label>")`.
3. `ruling-file`: `WriteRuling` (idempotent: same path, same bytes).
4. One `Update` marks those done and sets `home:pending` and `home:undeliverable` from the
   projections.
- A failed step leaves its obligation pending and stops; the next reconcile resumes it.
- `wake` obligations are never marked done here; only `Ack` does that.

**`ReconcileAll(ctx)`** runs at startup and every 30 s until it has completed once, then every
5 minutes. It lists `ListByLabel(home:pending)` — open or closed, whatever the reason — and
reconciles each bead. It also lists open beads with a `pick` but no `home:pending` label (a
crash between write and label is impossible in one `Update`, so this list is expected empty;
a non-empty one is logged as a tracker anomaly and reconciled).

**`Wakes()`** returns pending `wake` obligations grouped by `asking_thread`. Each carries the bead
id, the obligation id, the question, the picked label, the kind and, for instruction picks, the
exact instruction. The question and instruction are the asker's own text going back to that
same asker, which does not break `[D21]`.

**`Ack(ctx, acks []{bead, obligation, undeliverable bool})`** `[B-4]`: for each pair, `Get` the
bead, set that one obligation to `done` or `undeliverable`, recompute the projections, and
`Update`. An obligation id that is not pending is a no-op. Other obligations on the same bead
are untouched.

**`Dismiss(ctx, bead, obligation)`** moves an undeliverable wake to `dismissed` after mk handed
it on.

**Test cases** (each tied to a CUJ autarch-07 step). Each asserts the final bead labels and
metadata, the ruling file, and what `Owed`, `Wakes` and `Feed` then return:
- Filing gives an open bead with id `BeadID(request_id)` and a revision. Filing again with the
  same request id returns the same bead and makes one `create` call.
- **Concurrent filing:** two goroutines file the same request; one bead, both get its id.
- **Delayed commit:** a `create` that lands and then times out; the retry returns the same bead
  and no second bead exists.
- A reused request id with a different body is refused 409.
- Tracker down before create gives the fixed message, and no `create` call.
- Supersession: filing B with `supersedes: A` closes A. With a fault on that `Close`, B exists
  with `home:pending`; re-filing B (same request id) closes A. A picked predecessor refuses B
  with `already ruled`.
- Picking an instruction option: the bead closes, the ruling file exists, and `Wakes()` returns
  one wake carrying the exact instruction.
- Picking ruling-only: no wake, and the feed carries it.
- **Retry:** the same `pick_id` twice gives one `pick` and one `Update` with a pick in it; a
  different `pick_id` gets 409.
- **Crash after the pick write:** the fake fails `Close`; `ReconcileAll` then closes the bead,
  writes the file and leaves one pending wake. A second `ReconcileAll` changes nothing.
- A stale revision is refused without a write.
- **Ack race:** a wake is fetched, then a second obligation is added to the same bead (a
  supersession's predecessor pick is refused, so the test uses a crafted second wake), then the
  first is acked: the second is still pending.
- Undeliverable, then dismiss, drops the bead from the undeliverable list.

Commit: `decisions: file, pick once, reconcile and per-obligation acks`.

<verify>
- run: `go test -race ./internal/decisions/ -run Service -count=3`
  expect: exit 0
</verify>

### Task 1.5: feed, recent, undeliverable and trial stats

Depends on Task 1.4. Merged into the same file, so it follows it in the manifest.

**Files:**
- Modify: `internal/decisions/service.go`.
- Test: `internal/decisions/feed_test.go`.

**Design:**
- `Recent(project, limit)` `[A-16]` returns decisions with a Home pick closed within 14 days,
  from `ListRecent`, each with the picked label, the kind and the ruling path.
- `Undeliverable()` returns every bead with `home:undeliverable`, however old, with its answer
  and instruction `[B-13]`.
- `Feed(since)` `[A-20]` `[B-12]` returns one object:
  `{projects: {<project>: [line…]}, threads: {<thread>: [line…]}}`.
  - `projects` holds, per project, up to 10 lines for beads with a Home `pick` (P-8) closed since
    `since` (default 14 days), newest first.
  - `threads` holds, per asking thread, up to 10 lines for its own picked decisions in any
    project.
  - The caller selects `projects[P] ∪ threads[T]` at read time, so no cache ever holds one
    thread's selection for another.
  - Each line reads `ruled YYYY-MM-DD "<label>" (<bead>)`. The label is quoted, stripped of
    newlines and control characters, and clipped to 80 runes. The line is clipped to 140.
  - A line never contains the question, the instruction or any metadata other than the label
    `[D21]`.
  - The whole object is cached for 30 s and invalidated by any pick or reconcile in this
    process.
- `Owed()` returns open beads with no pick, and the tracker status.
- `Stats(since)` reports decisions filed per ISO week, and the median and p90 of
  `pick.at − created_at` for picked decisions `[A-20]`.

**Test cases:**
- A label with `\n` or an injection-shaped string is flattened and clipped.
- Neither the question nor the instruction appears in feed output.
- A bead closed by hand with no Home `pick` is absent from the feed (P-8).
- A thread's own answer from another project appears under `threads[T]`, and not under
  `threads[U]`.
- Two threads in one project: selecting for T never includes U's own answers.
- `Undeliverable` includes a bead closed 60 days ago whose wake is undeliverable.
- Stats over fixed timestamps, using `pick.at`.

Commit: `decisions: feed, recent, undeliverable and trial stats`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Feed|Recent|Undeliverable|Owed|Stats'`
  expect: exit 0
</verify>

### Task 1.6: the HTTP API on `autarch serve`

**Files:**
- Create: `internal/decisions/api.go`, `internal/decisions/client.go`.
- Modify: `internal/serve/serve.go` (`Config` gains `Decisions http.Handler` and the
  `BeforeListen`/`AfterListen` hooks are set by the command; `/health` gains a `decisions`
  field) and `cmd/autarch/serve.go` (build a `decisions.Service` with `BDTracker{HubDir}` and the
  resolver; set `BeforeListen` to `AcquireOwnerLock` and `AfterListen` to the reconcile loop).
- Test: `internal/decisions/api_test.go`, using `httptest` against `Routes` with a fake tracker;
  `internal/serve/owner_test.go` for the lock and startup order.

**Routes** (all behind the P-3 middleware):

| Route | Behavior |
|---|---|
| `GET /api/decisions` | Returns `{owed:[…], undeliverable:[…], tracker:{up, down_since}, reconciled}`. When the tracker is down it returns 200 with `up:false`, never an empty "nothing owed" list. |
| `POST /api/decisions` | Files a decision. Takes `request_id`. 201 new, 200 existing, 409 reuse or already ruled, 422 validation, 503 tracker down (nothing filed), 504 outcome unknown. |
| `GET /api/decisions/recent?project=&limit=` | Returns `Recent`. |
| `POST /api/decisions/{id}/pick` | Takes `{option_id, revision, pick_id}`. 200 with the pick; 409 already ruled or revision differs; 503 tracker down, outcome unknown or not yet reconciled. |
| `GET /api/decisions/feed` | Returns the feed object. |
| `GET /api/decisions/wakes` | Returns pending wakes grouped by thread. |
| `POST /api/decisions/wakes/ack` | Takes `{acks:[{bead, obligation, undeliverable}]}`. |
| `POST /api/decisions/{id}/dismiss` | Takes `{obligation}`. |
| `GET /api/decisions/stats?since=` | Returns the trial stats. |

`Client` wraps these routes, reading the address from `$AUTARCH_SERVE_ADDR` (default
`127.0.0.1:8110`) and the token from `~/.autarch/serve.token`. It maps a refused connection to
`ErrHomeDown` (nothing was sent) and a timeout after the request was written to
`ErrOutcomeUnknown`.

**Test cases:**
- The status code for each route and error.
- The tracker-down shape.
- Pick returns 409 on a second `pick_id`, 200 on the same one, and 503 before the first
  reconcile.
- The body size is capped at 64 KiB.
- **Startup with the tracker down** `[B-17]`: `/health` and `GET /api/decisions` answer
  (`up:false`, `reconciled:false`) while pick and file return 503; clearing the fault lets the
  loop reconcile and writes succeed.
- Owner lock: two `serve` processes (the test re-execs the test binary) on different ports —
  the second exits non-zero with `owns decisions` before it binds, and its fake tracker records
  no calls.
- `Client` against a closed port gives `ErrHomeDown`.

Commit: `serve: decisions API, client and single owner`.

<verify>
- run: `go test -race ./internal/decisions/ ./internal/serve/`
  expect: exit 0
</verify>

### Task 1.7: the CLI helper, exit codes and the MCP tool

**Files:**
- Create: `cmd/autarch/decide.go`, `cmd/autarch/exit.go` and `cmd/autarch/decide_test.go`.
- Modify: `cmd/autarch/main.go`: `root.AddCommand(decideCmd())`, and the `os.Exit(1)` at `:91`
  becomes `os.Exit(exitCode(err))`, where an `ExitError{Code}` anywhere in the chain gives its
  code and any other error gives 1 `[B-14]`.
- Modify: `pkg/mcp/server.go` (add `autarch_file_decision`, with a write scope checked like the
  others at `:317`; it files through `decisions.Client`).

**Commands:**
- `autarch decide file` reads a JSON request on stdin (the bead schema's option list plus
  `question`, `project`, `recommendation`, and optional `request_id` and `supersedes`).
  - A missing `request_id` is sha256 of the thread plus the canonical request body, so a blind
    retry of the same request is idempotent `[A-10]`. `bb home ask` uses the same rule
    (Task 1.9), so the two surfaces agree `[B-8]`.
  - The thread comes from `--thread` or `$BB_THREAD_ID`. It is required unless `--asker mycroft`.
  - `project_dir` comes from `--project-dir` (default: the git root of the working directory),
    and must match the resolved project.
  - It posts through `decisions.Client`. On success it prints `{"bead":…,"request_id":…}`.
  - Exit 3: Home down or tracker down, nothing filed; the message says to ask mk in chat.
    Exit 4: outcome unknown, re-run the same command. Exit 2: validation error, naming the bad
    option and the rule. Exit 5: already ruled, printing the label.
- `autarch decide list` reads through the running service if `/health` answers, and otherwise
  directly through `BDTracker` (read-only).
- `autarch decide stats [--since 14d]` prints the trial stats.

There is **no pick in the CLI**. Picks happen only on the rail, so mk is the one picking
`[D14]`.

**Test cases** (the tests build the binary and run it as a subprocess, so exit codes are
real):
- Against an `httptest` service with a fake tracker: `file` posts once, and running it twice
  with the same stdin gives the same bead.
- Exit 3 with no service listening; exit 4 when the service returns 504; exit 2 on an
  instruction on a ruling-only option; exit 5 on a picked predecessor.
- MCP `tools/list` includes `autarch_file_decision`, and a call without the write scope is
  refused.

Commit: `decide: filing helper CLI, exit codes and MCP tool`.

<verify>
- run: `go test -race ./cmd/autarch/ -run Decide`
  expect: exit 0
- run: `go test -race ./pkg/mcp/`
  expect: exit 0
</verify>

### Task 1.8: Mycroft escalates through decisions `[A-13]` `[B-15]`

**Files:**
- Modify: `internal/mycroft/escalate/escalate.go:55-120`.
- Modify: `internal/mycroft/scheduler/orchestrator.go`: `suggest()` (`:97-110`) and the T2
  out-of-allowlist branch of `autoDispatchFiltered` (`:123-130`).
- Modify: `cmd/mycroft/main.go:98`: the production `runCmd` passes
  `WithEscalations(escalate.NewDecisionQueue(decisions.NewClient(…)))` to `NewOrchestrator`.
- Modify: `internal/tui/views/mycroft.go` (the badge).
- Test: `internal/mycroft/escalate/escalate_test.go`,
  `internal/mycroft/scheduler/escalate_wiring_test.go`, `cmd/mycroft/wiring_test.go`.

**Design:**
- `DecisionQueue` gets a `decisions.Filer` (the `File` and `Owed` subset of `Client`).
- `NewOrchestrator` gains an option `WithEscalations(*escalate.DecisionQueue)`. With it set:
  - `suggest()` calls `Add` next to its existing `logDispatch(… ActionSuggest …)`;
  - the T2 out-of-allowlist branch calls `Add` next to its existing log line.
- `Add(p)` files this decision:
  - asker `mycroft`; project from the bead's project;
  - request id `mycroft:<project>:<bead>:<agent>`, so each patrol cycle finds the same bead
    instead of filing another;
  - the question is `Mycroft suggests <agent> on <bead>: <title>. Should it?`;
  - options, both **ruling-only**: *yes, when Mycroft can dispatch* and *no, skip it*.
  - The rail notes on these cards: "records your ruling; Mycroft does not dispatch from it until
    step 5". Mycroft's current `Spawn` only opens an empty tmux session, so v1 offers no option
    that claims to dispatch. Mycroft reading its rulings and launching real work is step 5
    (Mycroft's proposals).
  - `Reasoning` goes in the bead description only. It never reaches the feed.
- `Len`, `All` and `HighestSeverity` read `Owed` filtered by `asker:mycroft`, cached for 10 s.
- `Get` and `Remove` are deleted if unused outside tests. Otherwise `Remove` becomes a no-op
  with a deprecation comment.
- The TUI badge shows `?` when Home or the tracker is down, instead of `0`.

**Test cases:**
- `Add` makes one `File` call with the right labels, request id and two ruling-only options; a
  second cycle with the same suggestion makes no new bead.
- An orchestrator at T1 fed a `FleetView` with one ready bead produces one decision bead (patrol
  → bead).
- The production constructor path in `cmd/mycroft` sets the escalation queue (the wiring test
  builds `runCmd`'s orchestrator with a fake client and checks one `File` call).
- `Len` reads through the filer. Home down makes `Len` return the last known count, and
  `Stale()` returns true.

Commit: `mycroft: escalations file ruling-only decision beads`.

<verify>
- run: `go test -race ./internal/mycroft/... ./cmd/mycroft/ ./internal/tui/views/`
  expect: exit 0
</verify>

### Task 1.9: the bb plugin (rail, wakes, feed, `bb home ask`) `[A-12]`

**Files** (created with `bb plugin new bb-plugin-autarch` under `integrations/`, then edited):
- `integrations/bb-plugin-autarch/package.json`, with `typecheck` (`tsc --noEmit`) and `test`
  scripts matching the scaffold.
- `contract.ts`: `defineRpcContract` with `listOwed`, `listRecent`, `pick`, `dismiss` and
  `health`.
- `server.ts`, the `bb.server` entry. It uses `rpc`, `cli`, `background`, `agents` and `sdk`. A
  `host.ts` is added only if a host-local capability turns out to be needed.
  - **Service supervisor** (background):
    - If `GET 127.0.0.1:8110/health` fails, it spawns `autarch serve` (the binary comes from
      the `AUTARCH_BIN` setting, default `autarch` on PATH).
    - Restarts are capped at 3 per 10 minutes. It never spawns a second copy while `/health`
      answers, and a spawn that exits with `owns decisions` counts as "running elsewhere".
    - It reads `~/.autarch/serve.token` and never sends it to the panel.
  - **RPC handlers** proxy to the decisions API with the token. The panel never opens a
    WebSocket to the service `[A-22]`.
  - **Wake loop** (background) `[A-15]` `[B-4]`:
    - Every 15 s, plus immediately after a pick, it reads `/api/decisions/wakes`.
    - For each thread it sends one message through
      `bb.sdk.threads.send({threadId, input, mode: "queue-if-active"})`. The message lists each
      answer as:
      ```
      Home: mk answered your decision <bead>: "<question, clipped to 200>"
      Picked: "<label>"
      Your instruction for this option, approved by mk's pick:
      <instruction, verbatim>
      ```
      A needs-context answer ends after the picked label with "You asked to hear back before
      acting; continue from here."
    - On success it acks exactly the `(bead, obligation)` pairs in that message. On a rejection
      that the thread is archived or deleted, it acks them with `undeliverable: true`. Other
      failures retry on the next tick.
  - **Feed** (`feed.ts`) `[B-12]`:
    - A background refresher fetches `GET /api/decisions/feed` every 30 s and keeps the whole
      object. One call covers every project and thread, so a thread never seen before has its
      project's lines ready.
    - `bb.agents.configure` is synchronous. For thread T in project P it returns
      `projects[P] ∪ threads[T]`, newest first, deduplicated by bead, with
      `{instructions, tools: [], skills: []}`.
    - The instructions are a fixed header, `Recent rulings in this project (label only):`, at
      most 10 lines, and a short note on filing: "When you need mk to decide, run
      `bb home ask --request-stdin` with one line of JSON. Options default to needs-context.
      Give an option an `instruction` when you already know what you would do if mk picks it;
      you'll receive it verbatim." The whole text is kept under the 4096-character cap,
      dropping the oldest lines first.
    - With an empty cache or the service down, it returns no instructions.
  - **CLI** `[B-14]`: `defineCli` registers `bb home ask` with a named option `request` declared
    `stdin: true`, so the agent runs `jq -c . request.json | bb home ask --request-stdin` (one
    line, at most 16 KiB, the proxy's limit). `--request '<json>'` also works. It takes the
    thread from `ctx.threadId`, derives `request_id` by the Go helper's rule if none is given,
    and calls `POST /api/decisions`. Outside a thread it is refused. It exits 3, 4 or 5 as the
    Go helper does.
- `app.tsx`:
  - A `navPanel` "Home" panel with an `experimental_sidebarAccessory` badge (the owed count, or
    `!` when the tracker is down).
  - The rail lists owed decisions grouped by project. Each shows the question, a link to the
    asking thread, then each option with:
    - its label and kind;
    - for an instruction, the full text and "sent to <thread> as written; the agent acts on it
      under its own permissions";
    - a recommendation mark.
  - An "Undeliverable" section lists every undeliverable wake with its answer and instruction,
    a copy button and a Dismiss button `[B-13]`.
  - Picking sends `{option_id, revision, pick_id}` from what was rendered; the panel generates
    `pick_id` once per click and reuses it on retry. A 409 re-reads the decision.
  - A "Recent" section from `listRecent` shows each ruling with its label and a link to the
    ruling file. It survives a panel reload.
  - Banners: "tracker down since …" and "starting: reconciling" (the 503 state).
- `__tests__/server.test.ts`, using the SDK's `testing/fake-sdk.ts`:
  - `configure` for two threads in one project returns the project lines to both and each
    thread's own lines only to that thread;
  - `configure` for a thread never seen before returns its project's lines;
  - it returns nothing with an empty cache, and stays under 4096 characters with 10 long lines;
  - the wake loop sends one message per thread containing the exact instruction, and acks the
    exact pairs sent; a wake added during the send is not acked;
  - a fake rejection for an archived thread acks as undeliverable and does not resend;
  - `bb home ask --request-stdin` outside a thread is refused, and inside one posts with
    `ctx.threadId` and the derived request id;
  - `listRecent` after a simulated reload returns the same rulings.
- `README.md`: build, the local install command (for mk) and settings.

**Steps:**
1. Scaffold the plugin with `bb plugin new`.
2. Write `contract.ts`, then the failing tests in `__tests__/`.
3. Write `server.ts` and `feed.ts` until the tests pass.
4. Write `app.tsx`.
5. Run `npm run typecheck`, `npm test` and `bb plugin build`. Each must exit 0.
6. Check the real CLI proxy: with the built plugin loaded in a dev bb (`bb plugin dev`, not
   installed), pipe a one-line request through `bb home ask --request-stdin` from a scratch
   thread and confirm the service received it. Record the transcript in the task's commit
   message body.
7. Commit: `bb-plugin-autarch: Home rail, wakes, feed and bb home ask`.

**Not in this task:** `bb plugin install`, which is mk's step (G-3). If `bb plugin dev` is not
available without installing, step 6 is recorded as not run and moves to G-6.

<verify>
- run: `cd integrations/bb-plugin-autarch && npm run typecheck && npm test`
  expect: exit 0
- run: `cd integrations/bb-plugin-autarch && bb plugin build`
  expect: exit 0
</verify>

### Task 1.10: the scenario harness `[A-18]` `[B-18]`

**Files:**
- Create: `internal/homee2e/harness_test.go` (build tag `homee2e`), `internal/homee2e/bdfault/`
  (a fault-injecting `bd` wrapper built by the harness), `internal/homee2e/README.md`.

**Design:**
- The harness builds `autarch`, starts `serve` as a real child process on `127.0.0.1:0` with a
  temp token, a temp home and a scratch git project.
- `bdfault` wraps a real or fake `bd`. `HOMEE2E_BD_FAULT` selects a fault: `down` (connection
  refused), `timeout-after-write=<subcommand>` (let the write land, then hang past the
  timeout), `kill-after=<subcommand>` (SIGKILL `serve` right after that call returns), or
  `fail-nth=N`.
- Two modes:
  - `HOMEE2E_MODE=fake` (default): `bd` is a file-backed fake that enforces unique ids, so it
    runs anywhere.
  - `HOMEE2E_MODE=hub`: the real hub tracker with `LabelPrefix=hometest` (Task 1.11).
- The harness takes its output path from the `-homee2e.out` test flag and fails if it is not
  given. It writes one JSON object per scenario:
  `{scenario, mode, commit, run_id, pass, beads, wakes, ruling_file, detail}`, where `commit` is
  `git rev-parse HEAD` and `run_id` is fresh per run. This one file is both the machine check
  and the human report.

**Scenarios** (names are exact; the checks in criteria 12 and 13 compare the set):
1. `answer-instruction`: file with an instruction option and pick it. The bead is closed, the
   ruling file exists, `wakes` holds one wake with the exact instruction, the feed shows the
   label only. Ack it; wakes is empty.
2. `pick-retry`: `timeout-after-write=update` on the pick; retry with the same `pick_id`. One
   `pick`, one wake.
3. `crash-after-pick`: `kill-after=update` on the pick. Restart `serve`. Within 35 s the bead is
   closed, the ruling file exists and exactly one wake is pending.
4. `two-serves`: start a second `serve` on another port. It exits non-zero with
   `owns decisions` and the first still answers.
5. `file-retry`: `timeout-after-write=create`, then re-run the same `decide file`. Exit 4, then
   exit 0, and one bead.
6. `tracker-down-at-start`: start with `down`. `/health` answers, `GET /api/decisions` shows
   `up:false`, pick returns 503. Clear the fault; within 35 s reconcile completes and a pick
   works.
7. `supersede`: file A, then B with `supersedes: A` under `fail-nth` on B's close of A. Re-run
   B's filing. A is closed and B is owed.

**Steps:** write the harness, run it in fake mode, fix what it finds, then commit
`homee2e: scenario harness for picks, retries and reconcile`.

<verify>
- run: `out=$(mktemp) && go test -tags homee2e -race ./internal/homee2e/ -count=1 -args -homee2e.out="$out" && jq -se --arg c "$(git rev-parse HEAD)" '(map(.scenario)|sort)==(["answer-instruction","crash-after-pick","file-retry","pick-retry","supersede","tracker-down-at-start","two-serves"]) and all(.pass==true and .commit==$c and .mode=="fake")' "$out"`
  expect: exit 0
</verify>

### Task 1.11: end-to-end check on zklw against the real hub, and follow-ups

This task has no new product code. It runs the Task 1.10 harness in hub mode, so the rail mk
uses is not polluted: every label is `hometest:*`.

**Steps:**
1. On zklw, from the repo root:
   `HOMEE2E_MODE=hub AUTARCH_HUB_DIR=/home/mk/hub go test -tags homee2e -race ./internal/homee2e/ -count=1 -args -homee2e.out=docs/research/2026-09-26-home-e2e-hub.jsonl`.
   Scenarios `answer-instruction`, `two-serves` and `supersede` run against the real tracker.
   The others inject faults into the `bd` wrapper and run in fake mode only, because they must
   not fault the shared hub.
2. The harness closes every `hometest:*` bead it created with reason `home e2e check`.
3. Write `docs/research/2026-09-26-home-e2e-check.md`: the command, the commit, a table built
   from the JSONL, and anything that failed and how it was fixed.
4. File two child beads of `mk-okek`, neither blocking this plan:
   - **Clavain filing helper and guidance** `[D15]` `[A-11]`: a Clavain wrapper that calls
     `bb home ask`, and the shared-guidance text on when to file and how to write an
     instruction. v1's guidance lives in the plugin's `configure` note.
   - **Signals clients reach the consolidated broker** `[B-19]`: endpoint and token settings
     for Pollard's publisher, after which the standalone Signals server can be deprecated.
   Record both ids in the e2e check document.

<verify>
- run: `jq -se '(map(.scenario)|sort)==(["answer-instruction","supersede","two-serves"]) and all(.pass==true and .mode=="hub")' docs/research/2026-09-26-home-e2e-hub.jsonl`
  expect: exit 0
</verify>

---

## Explicitly out of scope

- **Command picks, the continuation runner and revert commands** (cut by mk, 2026-09-26). An
  instruction replaces the command: the agent does the work under its own gates.
- **Signing ruling files**, deferred to step 3 (Lattice). Files are unsigned in v1; the feed
  reads Home's records instead.
- The brief continuation `[D22]`.
- A "QA this" decision kind. The brainstorm has none; it is raised to mk as an open question
  and not added without a ruling.
- Badges beyond the Home entry (step 2).
- Lattice (step 3), the map (step 4, estate-map WI-5 adds it to this plugin), Mycroft's
  proposals and dispatch from rulings (step 5), and the companion (step 6).
- The Clavain `UserPromptSubmit` feed hook (P-7) and the Clavain filing helper plus guidance
  text `[D15]`, filed as a follow-up in Task 1.11.
- Successor-thread resolution for rotated threads (deferred by mk under G-9).
- `bb.ui.requestInput` in-thread cards. The rail is the only picking surface in v1.
- Removing the standalone servers, and deprecating the standalone Signals server while Pollard
  depends on it.
- Mounting Pollard.
- A note without a wake. This is unverified, and P-5 wakes are acceptable for v1.

## Execution preconditions and mk gates

These are **not agent-completable**. Each is a checklist item for mk; none is a DONE WHEN.

- **G-0 (ruled 2026-09-26):** mk approved the Home epic `mk-okek` with S0 `mk-okek.1` and S1
  `mk-okek.2`. Still required before execution: a third review-astra pass on this revision
  that does not return needs-rework, and the zklw CI status for this repo
  (`zklw-ci status --repo mistakeknot/Autarch --json`).
- **G-1:** Create the Uqbar repo, which must be private, and set `AUTARCH_UQBAR_DIR`. Until then
  estate-wide decisions are refused, and project decisions still work.
- **G-2 (superseded 2026-09-26):** Home's signing key is not needed in v1.
- **G-3:** Install the plugin: `bb plugin install path:integrations/bb-plugin-autarch`.
- **G-4:** Republish `autarch-plugin`, because `plugin.json` changes in Task 0.5.
- **G-5 (ruled 2026-09-26)** `[A-11]`: mk accepted both departures. P-7: the feed is injected
  by `agents.configure` at session start or resume, instead of the per-turn hook in `[D17]`.
  P-5: wakes go only to instruction and needs-context picks; ruling-only outcomes reach the
  asking thread through the feed.
- **G-6:** Start the trial by walking autarch-07 on the real rail with a real decision, and
  revise autarch-07 to match: instructions instead of commands, unsigned rulings. Trial metrics
  come from `autarch decide stats`.
- **G-7 (superseded 2026-09-26):** there are no command picks to gate on a key.
- **G-8 (superseded 2026-09-26):** there are no failed commands to follow up. An agent whose
  instruction fails files a new decision, with `supersedes` if it replaces an open one.
- **G-9 (ruled 2026-09-26)** `[A-15]`: v1 marks a wake to an archived or deleted thread
  undeliverable and shows it on the rail. Following a Clavain handoff record to the successor
  thread is deferred; Home does not read handoff records in v1.

## Verification

- Whole tree: `go build ./cmd/... && go test -race ./...`.
- Bind: `go test -race -run Bind ./internal/serve/`.
- Plugin: `npm run typecheck`, `npm test` and `bb plugin build` in
  `integrations/bb-plugin-autarch`.
- Harness: `go test -tags homee2e -race ./internal/homee2e/` in fake mode (Task 1.10) and hub
  mode (Task 1.11), each checked against its exact scenario set.
- Real acceptance: G-6 (mk). Neither the harness nor any fixture replaces it.

## Acceptance Criteria

1. The whole tree builds and every test passes with the race detector.
   ```check
   go build ./cmd/... && go test -race ./...
   ```
2. `autarch serve` refuses a non-loopback address.
   ```check
   go test -race -run 'Bind' ./internal/serve/ ./cmd/autarch/
   ```
3. The service mounts Bigend, Gurgeh and Signals under prefixes behind the token, Host and
   Origin checks, with no query-string token. `/health` is open. Projects resolve without
   ambiguity or symlink escape. Gurgeh's signals reach `/signals/ws` through one broker. Step 0
   does not import the decisions package.
   ```check
   go test -race ./internal/serve/ ./internal/gurgeh/server/ && ! go list -deps ./internal/serve/ | grep -q internal/decisions
   ```
4. `autarch mcp` serves the existing MCP tools plus `autarch_file_decision`, and `autarch-mcp`
   still works.
   ```check
   go test -race ./cmd/autarch/ -run MCP && go test -race ./pkg/mcp/
   ```
5. Options have ids and one of three kinds, with needs-context as the default and instructions
   as bounded text. The revision covers the whole decision, and the bead id is derived from the
   request id.
   ```check
   go test -race ./internal/decisions/ -run 'Validate|Revision|BeadID|Projection'
   ```
6. The tracker writes decision beads through `bd` with explicit ids and metadata passed by
   file. A duplicate id, tracker down and outcome unknown are distinct errors, and nothing is
   spooled.
   ```check
   go test -race ./internal/decisions/ -run Tracker
   ```
7. Ruling files carry the ratification block and the Home fields, are written through
   `os.Root`, and are idempotent. A second process cannot take the owner lock.
   ```check
   go test -race ./internal/decisions/ -run 'Ruling|Lock'
   ```
8. Filing is idempotent under concurrency and delayed commits. A pick is recorded once per
   bead, a retry with the same pick id returns it, reconcile finishes any interrupted
   obligation whether the bead is open or closed, supersession is serialized and retryable, and
   acking one wake never clears another.
   ```check
   go test -race ./internal/decisions/ -run Service -count=3
   ```
9. The feed carries only the quoted, clipped label from Home pick records, selected per thread
   at read time. Undeliverable wakes are listed regardless of age.
   ```check
   go test -race ./internal/decisions/ -run 'Feed|Recent|Undeliverable|Owed|Stats'
   ```
10. The API and the single-owner rule hold: reads work and writes return 503 before the first
    reconcile, including with the tracker down at start; a second pick id gets 409; a second
    `serve` exits before it binds. The CLI returns exit codes 2, 3, 4 and 5 as a real process.
    ```check
    go test -race ./internal/decisions/ ./internal/serve/ && go test -race ./cmd/autarch/ -run Decide
    ```
11. Mycroft's suggestions and out-of-allowlist dispatches file ruling-only decision beads once
    per suggestion through the production constructor, and `DecisionQueue` keeps no private
    list.
    ```check
    go test -race ./internal/mycroft/... ./cmd/mycroft/ && ! grep -nE '^[[:space:]]+decisions[[:space:]]+\[\]PendingDecision' internal/mycroft/escalate/escalate.go
    ```
12. The bb plugin typechecks, its fake-SDK tests pass, and it builds.
    ```check
    cd integrations/bb-plugin-autarch && npm run typecheck && npm test && bb plugin build
    ```
13. The scenario harness passes exactly its seven scenarios in fake mode, at this commit. An
    empty, partial, duplicated or stale result fails.
    ```check
    out=$(mktemp) && go test -tags homee2e -race ./internal/homee2e/ -count=1 -args -homee2e.out="$out" && jq -se --arg c "$(git rev-parse HEAD)" '(map(.scenario)|sort)==(["answer-instruction","crash-after-pick","file-retry","pick-retry","supersede","tracker-down-at-start","two-serves"]) and all(.pass==true and .commit==$c and .mode=="fake")' "$out"
    ```
14. The hub-mode run passed exactly its three scenarios and cleaned up its beads.
    ```check
    jq -se '(map(.scenario)|sort)==(["answer-instruction","supersede","two-serves"]) and all(.pass==true and .mode=="hub")' docs/research/2026-09-26-home-e2e-hub.jsonl && [ "$(cd /home/mk/hub && bd list -l hometest:decision -s open -n 0 --json | jq length)" = "0" ]
    ```
15. mk walks autarch-07 on the installed rail (G-6). This is recorded by mk, and no command can
    substitute for it.
