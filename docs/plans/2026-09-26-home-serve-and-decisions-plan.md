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

**Rulings (mk, 2026-09-26):** G-5 accepted (P-7 feed via `agents.configure`; P-5 wake scope).
G-7: recommended option (no command picks without a key). G-8: original options plus *retry*.
G-9: mark undeliverable in v1; successor resolution deferred.

**Goal:** mk sees the decisions agents owe mk in one Aleph rail and picks an option. The pick
runs exactly the approved snapshot, at most once, and leaves evidence that survives a crash. It
writes a signed ruling, and the answer reaches the asking thread. All of this is served by one
loopback Go service.

**Architecture:**
- `autarch serve` is one loopback HTTP service. It mounts the existing Bigend daemon, Gurgeh
  and Signals handlers under path prefixes. It shares one Signals broker between them, and adds
  a token check, an Origin check and a decisions API.
- One `serve` process owns decisions. It holds a process-lifetime lock taken before recovery and
  before it listens.
- MCP moves into the same binary as `autarch mcp`, still speaking stdio.
- Decisions live only as `decision` beads in the hub tracker, reached through `bd`.
- Every pick or revert is an **attempt**. An attempt has an id, a claim written to the tracker
  before anything runs, and an evidence directory under `~/.autarch/attempts/<id>/`. Crash
  recovery reads the claim and the evidence, and never re-runs.
- A thin bb plugin (`integrations/bb-plugin-autarch/`, entry `server.ts`) does four things:
  - starts the service;
  - renders the rail;
  - delivers wakes and the feed;
  - registers `bb home ask`.
  The browser never talks to the service directly; the plugin mediates every call.

**Tech Stack:**
- Go 1.24: `net/http` ServeMux patterns, cobra, `os/exec`, `syscall.Flock`.
- `bd` 1.1.2 against the hub Dolt tracker.
- `ssh-keygen -Y sign/verify`.
- The bb plugin SDK: TypeScript, `bb.server` entry, React panel, `testing/fake-sdk.ts`.

**Prior learnings / inputs:**
- [Brainstorm](../brainstorms/2026-09-24-one-place-in-aleph-brainstorm.md), decisions 13–22.
  They are cited here as `[Dn]` and are binding, except for the narrowings of `[D17]` and `[D18]`
  that mk ruled on under G-5 and G-8.
- [One-pager](../onepagers/2026-09-24-one-place-in-aleph.md).
- CUJs [autarch-07](../cujs/autarch-07-decide-and-continue.json) and autarch-09.
- [Thread Organizer assessment](../research/assess-bb-thread-organizer.md) (SDK primitives).
- [Estate-map trial plan](2026-09-23-estate-map-trial-plan.md), WI-5. It planned
  `integrations/bb-plugin-autarch/` but never built it; this plan creates it, and WI-5 later
  adds its map panel to the same plugin.

---

## Decision context (routing)

```json
{"reasons":["foundational-invariants","broad-consequences"],
 "rationale":"Plan for autarch serve consolidation and the Home decisions queue: picks run commands as mk and write signed rulings (authority semantics), and the decision-bead schema is a shared protocol for every agent, Mycroft and the rail.",
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
- **Status:** review-astra, round 1, verdict **needs-rework**: 22 findings (18 P1, 4 P2), all
  folded into this revision and cited `[A-n]`.
  - Receipt: gpt-6-astra, xhigh, codex. 8m51s, 2,135,072 tokens in, 16,122 out, 50 commands.
  - The `.verdict` sidecar mis-parsed; the review body's needs-rework governs.
  - Astra flagged two things as unverified: the GitHub repository identity and the zklw CI
    status (`zklw-ci status`). Both remain open prerequisites for execution.
  - The review is at `thread-storage/thr_awr853efiy/home-plan-review.md`.
  - A second review-astra pass on this revision is required before execution, because the plan
    is foundational.

### Where each finding landed

| Finding | Theme | Folded into |
|---|---|---|
| A-1 | Two `serve` processes | P-3a, Task 0.4, Task 1.5 |
| A-2, A-3 | Run-once across crashes; attempt states | P-4, bead schema, Tasks 1.4, 1.5 |
| A-4 | Obligations lost at close | P-5, Tasks 1.2, 1.5 |
| A-5 | Revert semantics | Task 1.5 (Revert) |
| A-6 | Shell text is not a frozen command | P-10, Task 1.1, Task 1.4 |
| A-7 | Precondition runs at listing; env silently dropped | P-10, Task 1.1, Task 1.5 |
| A-8 | `shown_hash` is not the whole decision | Bead schema, Task 1.1, Task 1.7 |
| A-9 | Unsigned rulings run commands | P-8, G-7 |
| A-10 | Filing is not idempotent; degraded start | Tasks 1.2, 1.5, 1.7, 1.8 |
| A-11 | Wake scope | P-5, P-7, G-5 |
| A-12 | Wrong plugin SDK surface | Task 1.10 |
| A-13 | Mycroft escalations are never produced | Task 1.9 |
| A-14 | Two Signals brokers | Task 0.3 |
| A-15 | Rotated or archived asking threads | P-5, Task 1.10, G-9 |
| A-16 | No way to list recent rulings | Tasks 1.6, 1.7, 1.10 |
| A-17 | Manifest ordering | exec manifest, task dependencies |
| A-18 | e2e cannot prove crash safety | Task 1.11 (harness), Task 1.12 |
| A-19 | Project resolution and symlinks | Task 0.2 (`projects.go`), Tasks 1.3, 1.5 |
| A-20 | Recent query and stats basis | Tasks 1.2, 1.6 |
| A-21 | Stale shown too late; follow-up loses options | Tasks 1.5, 1.6, 1.10, G-8 |
| A-22 | `?token=` and Origin | P-3, Task 0.2 |

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
  - So step 0 does three things:
    - moves MCP into the one binary as `autarch mcp`;
    - keeps `cmd/autarch-mcp` as a two-line alias;
    - adds one MCP tool, `autarch_file_decision`, which files through the same code as the CLI.
  - A shim is revisited when the service holds state MCP needs.
- **P-3: The service has a token and an Origin check even on loopback** `[A-22]`.
  - The pick route runs commands as mk. A loopback-only guard does not stop a browser page
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
- **P-3a: One decisions owner** `[A-1]`.
  - `serve.Run` takes a non-blocking `flock` on `~/.autarch/decisions.lock` and holds it for the
    life of the process. It does this before `Recover` and before `Listen`.
  - A second `serve` fails fast with `another autarch serve owns decisions (pid N)`, even when
    it was given a different port.
  - Inside the owning process, `Pick`, `Revert` and `Recover` also take the service mutex.
- **P-4: Every pick is an attempt, and its claim comes first** `[A-2]` `[A-3]` `[D21]`.
  - Owed means an open bead with label `home:decision`.
  - A pick first writes a **claim** to the tracker in one `bd update`: `pick.attempt` (random
    id), `option_id`, `revision`, `state: claimed`, the attempt directory path, and label
    `+home:running`. Nothing runs before the claim is stored.
  - The attempt directory holds `started` (written and fsynced immediately before the
    command starts), then `outcome.json` and `output.log`.
  - Evidence is never deleted. A new pick on the same bead is a new attempt, and its ruling
    carries `supersedes`. Earlier attempts stay in `history[]`.
  - Recovery reconciles from the claim plus the evidence, per the table in Task 1.5. It never
    re-runs a command.
- **P-5: Obligations are recorded before close, and wakes are scoped** `[A-4]` `[A-11]`
  `[A-15]`.
  - After the final ruling, the pick writes `state: finalizing` with an `obligations` list, then
    closes the bead, then carries out the obligations. Each obligation has a stable op id
    stored as label `home:op:<id>` on whatever it creates, so re-running it finds the existing
    result instead of creating a second one.
  - Wakes (`home:wake-pending`) go only to **needs-context picks and re-asks**. Command and
    ruling-only outcomes reach the asking thread through the feed, which includes the thread's
    own answers (Task 1.6). Ruled by mk under G-5.
  - The plugin drains pending wakes and sends one message per thread `[D13]`, then removes the
    label. At worst a wake is sent twice, and it is never lost.
  - A send that bb rejects (the thread is archived or deleted) marks the wake
    `home:undeliverable`. The rail then shows the answer so mk can hand it on. There is no retry
    loop. Resolving a successor thread is deferred (G-9, ruled).
- **P-6: Needs-context is the default** `[D22]`.
  - The helper infers `needs-context` when an option has no kind and no command. It rejects
    `kind: command` without a command, and a command on any other kind.
  - This reconciles `[D18]` ("a kind on every option") with `[D22]`: every stored option has an
    explicit kind.
- **P-7: The feed goes through `bb.agents.configure`, not a per-turn hook** (open question 5
  candidate) `[A-11]`.
  - The service exposes a consumer-agnostic `GET /api/decisions/feed`.
  - The plugin injects it at session start and resume from an async cache (Task 1.10).
  - The Clavain `UserPromptSubmit` hook `[D17]` is deferred until the trial shows that rulings
    made mid-session and arriving late are costly.
  - **This departs from the letter of `[D17]`, and with P-5 it narrows which answers wake a
    thread. mk accepted both under G-5 on 2026-09-26.** The mid-session gap is a trial
    measurement; a per-turn hook is added only if it proves costly.
- **P-8: Without a signing key, commands do not run** `[A-9]` (replaces the earlier "write
  unsigned"; ruled by mk under G-7).
  - With no key, the rail disables command options and points to G-2.
  - Needs-context and ruling-only picks still work. Their rulings are written unsigned and
    render as proposed `[D16]`.
  - The feed carries only rulings whose signature verifies.
  - `/health` reports `"signing":"missing-key"` and the rail shows a banner.
- **P-9: Home does not commit ruling files.**
  - It writes `docs/decisions/<file>.md` and its `.sig` into the project's working tree.
  - Committing is left to the project's normal flow: autosync, or the next agent commit.
  - This keeps Autarch from writing git history, which is world state.
- **P-10: Commands take two constrained forms** `[A-6]` `[A-7]`.
  - `argv`: a list, run with no shell. `argv[0]` is resolved against the frozen `PATH` at file
    time, and the resolved absolute path is part of the hash.
  - `script`: a file inside `dir`, plus a `deps` list of other files it reads. At pick, a
    snapshot of the script whose hash matches is copied into the attempt directory and run as
    `bash <snapshot>` with cwd `dir`. `deps` are hashed and re-checked, but they are read in
    place, so a file changed between check and use is a residual race. The rail discloses this
    as "reads <deps> at run time".
  - Free shell text is accepted only as `needs-context`, shown as "not runnable".
  - The same rules apply to `run`, `revert` and `precondition`.
  - An `env` key outside the allowlist is a validation error, not silently dropped. The rail
    shows the effective env.

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
  bead appears in the hub tracker, even if the call is retried. The bead carries:
  - the question and the revision;
  - the options, each with an id and a kind;
  - the frozen command hashes;
  - the project and the asking thread.
  If the tracker is down, the agent is told whether nothing was filed (exit 3) or whether the
  outcome is unknown and a retry is safe (exit 4).
- The rail lists owed decisions across projects. It shows:
  - each command exactly: argv or script, directory, effective env, precondition, deps, and the
    words "runs as mk";
  - the recommendation;
  - stale (detected at listing without running anything), interrupted, failed and
    undeliverable marks;
  - "tracker down since …" when the tracker is unreachable.
- A pick runs a command at most once, including across SIGKILL of `serve`. A stale hash or a
  failed precondition turns the pick into a re-ask. A failure files a follow-up that keeps the
  original options plus retry. A crash leaves the decision owed and interrupted, with its
  evidence kept.
- Every pick writes a Markdown ruling with the ratification block plus the Home fields and the
  approved snapshot. Command rulings are always signed, and `autarch decide verify` checks
  them.
- Needs-context answers and re-asks to one thread arrive as one message. New sessions in a
  project receive that project's recent verified rulings, plus the thread's own answers, as
  label and outcome only.
- Mycroft's suggestions and out-of-allowlist dispatches appear on the same rail, and picking
  "dispatch" runs `mycroft dispatch`. `DecisionQueue` holds no private list.

**Artifacts:**
- `internal/serve/serve.go` exports `New(Config) (*Service, error)`, `(*Service).Handler()` and
  `(*Service).Run(ctx)`. `internal/serve/projects.go` exports `Resolver` and `Resolve(name)`.
- `cmd/autarch/serve.go` provides `serveCmd()`. `cmd/autarch/mcp.go` provides `mcpCmd()`.
  `cmd/autarch/decide.go` provides `decideCmd()` with `file`, `list`, `verify` and `stats`.
- In `internal/decisions/`:
  - `model.go`: `Decision`, `Option`, `Command` (`Argv`, `Script`, `Deps`), `Kind`, `Validate`,
    `Revision`.
  - `freeze.go`: `Freeze`, `CheckStale` (non-executing).
  - `tracker.go`: the `Tracker` interface, `BDTracker`, `ErrTrackerDown`, `ErrOutcomeUnknown`.
  - `ruling.go`: `Ruling`, `WriteRuling`, `RulingPath`.
  - `sign.go`: `Signer`, `Sign`, `Verify`.
  - `attempt.go`: `Attempt`, `AttemptDir`, the state constants.
  - `run.go`: `Runner`.
  - `lock.go`: `AcquireOwnerLock`.
  - `service.go`: `Service`, with `File`, `Owed`, `Recent`, `Pick`, `Revert`, `Recover`,
    `Feed`, `Wakes` and `AckWakes`.
  - `api.go`: `Routes(mux, svc)`.
- `~/.autarch/attempts/<attempt>/` with `claim.json`, `started`, `outcome.json`,
  `output.log` and the script snapshot.
- `internal/mycroft/escalate/escalate.go`: `DecisionQueue` backed by a `decisions.Filer`.
- `cmd/mycroft/main.go`: a `dispatchCmd` (`mycroft dispatch --bead --agent`).
- `internal/homee2e/`: the scenario harness behind build tag `homee2e`.
- `integrations/bb-plugin-autarch/`: `package.json`, `server.ts`, `contract.ts`, `feed.ts`,
  `app.tsx`, `__tests__/`, `README.md`.

**Key links:**
- `serve.Run` runs: `EnsureLocalOnly` → `AcquireOwnerLock` → `Recover` (retried every 30 s
  until it has completed once) → `Listen`. Picks return 503 until `Recover` has completed.
- `serve.New` creates one `signals.Broker`, gives it to `signals.NewServer`, and gives
  `broker.Publish` to every Gurgeh server it builds.
- `decisions.Service.Pick` runs, in this order:
  1. claim (`bd update`: metadata plus `+home:running`);
  2. `CheckStale` and the precondition;
  3. `WriteRuling(state=running)`;
  4. write and fsync `started`;
  5. `Runner.Run`;
  6. write `outcome.json`;
  7. `WriteRuling(final)` and sign;
  8. `state=finalizing` with obligations;
  9. `bd close`;
  10. carry out obligations.
  Moving step 5 before step 1 or step 4 breaks run-once.
- `autarch decide file`, `bb home ask`, the MCP tool and Mycroft all call
  `decisions.Service.File`. There is one validation path.
- The plugin host holds the token. The panel calls RPC only.

---

## Step 0: one `autarch serve`

Step 0 finishes, and its tests pass, before any step 1 task starts `[A-17]`.

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
- `Config{Addr, ProjectDirs []string, TokenPath string, AllowOrigins []string, Decisions
  *decisions.Service}`.
  - The default `Addr` is `127.0.0.1:8110`. It is new, so it does not collide with a running
    Bigend daemon on 8100 during the transition.
  - `Decisions` may be nil in step 0.
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
  `{"status":"ok","bigend":…,"gurgeh":{"projects":N},"signals":…,"decisions":…,"signing":…}`.
- `auth.go` implements P-3:
  - `LoadOrCreateToken(path)` makes 32 random bytes, hex-encoded, and writes the file with mode
    0600. It refuses a token file whose mode is looser than 0600.
  - The middleware checks, in order: `Origin` (absent or allowlisted, else 403), `Host` (else
    403), then the bearer header with `subtle.ConstantTimeCompare` (else 401).
  - There is no query-string token.
- `Run(ctx)`:
  1. `netguard.EnsureLocalOnly(Addr)`;
  2. from Task 1.7 on, `AcquireOwnerLock` and `Recover` (P-3a);
  3. `ListenAndServe`;
  4. `Shutdown` with a 5 s timeout when `ctx` is done.

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
  `--daemon` path near `:345`), `internal/gurgeh/cli/commands/serve.go` and
  `internal/signals/cli/serve.go`.
  - Each keeps working unchanged, and prints one stderr line:
    `deprecated: use "autarch serve" (mounted at /<prefix>/)`.
  - Nothing is removed in this plan.
- Modify: `./dev` to add `./dev serve`, which runs `go run ./cmd/autarch serve`.
- Modify: `AGENTS.md` (the tool command reference) to document `autarch serve`, its prefixes,
  the token and the Origin allowlist.

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
- `type`: `decision`.
- `title`: the question, clipped to 120 runes.
- `description`: the question in full.
- `labels`:
  - always `home:decision`;
  - `project:<name>` (or `project:estate` for estate-wide);
  - `asker:<thread-id|mycroft>`;
  - `home:op:<request-id>` for idempotent filing `[A-10]`;
  - state labels, per the table in Task 1.5: `home:running`, `home:interrupted`, `home:stale`,
    `home:failed-followup`, `home:wake-pending`, `home:undeliverable` and `home:revert-failed`.
- `metadata` is one key, `home`, whose value is versioned JSON:

```json
{"v":2,
 "project":"Autarch","project_dir":"/home/mk/projects/Autarch",
 "asking_thread":"thr_abc123","asker":"thread",
 "request_id":"req_7f3a…",
 "revision":"sha256:…",
 "recommendation":"merge",
 "options":[
   {"id":"merge","label":"Merge feat/bb-catchup","kind":"command",
    "command":{"argv":["git","merge","--ff-only","feat/bb-catchup"],"argv0":"/usr/bin/git",
               "dir":"/home/mk/projects/Autarch",
               "env":{"PATH":"/usr/bin:/bin"},
               "revert":{"argv":["git","reset","--hard","ORIG_HEAD"],"argv0":"/usr/bin/git"},
               "precondition":{"argv":["git","diff","--quiet"],"argv0":"/usr/bin/git"}},
    "hash":"sha256:…"},
   {"id":"wait","label":"Wait for review","kind":"needs-context"},
   {"id":"never","label":"Never merge it","kind":"ruling-only"}],
 "follow_up_of":"", "supersedes":"",
 "pick":{"attempt":"att_9c1e…","option_id":"merge","revision":"sha256:…",
         "state":"running","dir":"/home/mk/.autarch/attempts/att_9c1e…",
         "ruling":"docs/decisions/2026-09-26-merge-feat-bb-catchup.md",
         "at":"…","obligations":[]},
 "history":[]}
```

- A `script` command replaces `argv` with `{"script":"scripts/x.sh","deps":["config/y"]}` plus
  their hashes (P-10).
- `revision` is sha256 over the canonical decision: question, option ids, labels, kinds, command
  hashes and recommendation `[A-8]`. Any edit changes it.
- `pick` is absent until mk picks. `history[]` holds every earlier `pick`, unchanged.
- `obligations` entries are `{op, kind: wake|reask|followup, done}`.
- **Metadata never carries the asker's free text into the feed.** The feed reads only the picked
  option's label and the state `[D21]`.

### Task 1.1: model, validation, revision and the hash freeze

**Files:**
- Create: `internal/decisions/model.go`, `internal/decisions/freeze.go`.
- Test: `internal/decisions/model_test.go`, `internal/decisions/freeze_test.go`.

**Design:**
- `Kind` is one of `command`, `needs-context` or `ruling-only`.
- `Validate`, per P-6 and P-10:
  - no kind and no command means `needs-context`;
  - `command` needs exactly one of `Argv` or `Script`, and an absolute `Dir` equal to or inside
    `project_dir`;
  - a free-text `run` string is accepted only on `needs-context` and marked not runnable;
  - no other kind may carry a command;
  - `env` keys must be in the allowlist (`PATH`, `HOME`, `LANG`, `TZ`, `AUTARCH_*`); any other
    key is an error naming it `[A-7]`;
  - option ids are unique, `[a-z0-9-]{1,32}`, and default to a slug of the label;
  - 2–6 options;
  - labels are non-empty and ≤ 80 runes;
  - `recommendation` names an existing option id.
- `Freeze(cmd)` resolves `argv[0]` against the frozen `PATH` and returns a sha256 over
  canonical JSON of: the resolved `argv0` and the argv, or the script path, its sha256 and each
  `deps` path with its sha256; `filepath.Clean(dir)`; `env` sorted by key; and the frozen
  `revert` and `precondition`. Script and deps paths must resolve (after `EvalSymlinks`) inside
  `dir`.
- `Revision(d)` hashes the canonical decision `[A-8]`.
- `CheckStale(opt)` is **non-executing** `[A-7]` `[A-21]`. It recomputes the hash and checks that
  `argv0`, the script and the deps still exist. It returns `ErrStale{Reason}` on any difference.
  It never runs the precondition, which runs only inside a pick (Task 1.5).

**Test cases:**
- Validation: every rule above, plus inference of needs-context.
- An env key `LD_PRELOAD` is rejected with its name in the error.
- The same inputs give the same hash, and env key order doesn't matter.
- Editing the script or a dep changes the hash. Replacing `/usr/bin/git` in a temp `PATH`
  changes the resolved `argv0` and makes the option stale.
- A script or dep that escapes `dir` (`../x.sh`, or a symlink out) is rejected at validation.
- `CheckStale` never executes: a precondition whose argv would create a marker file leaves no
  marker.
- Editing an option label changes `Revision`.

Commit: `decisions: option model, validation, revision and hash freeze`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Validate|Freeze|Stale|Revision'`
  expect: exit 0
</verify>

### Task 1.2: the `bd` tracker

Depends on Task 1.1 (it stores `Decision`) `[A-17]`.

**Files:**
- Create: `internal/decisions/tracker.go`.
- Test: `internal/decisions/tracker_test.go`, using a fake `bd` script on PATH via
  `t.Setenv("PATH")`, as in `internal/door/product_test.go:99`. The fake records its argv to a
  file and prints canned JSON.

**Design:**
- `BDTracker{HubDir, Timeout, LabelPrefix}` runs `bd` with `cmd.Dir = HubDir`, a 5 s timeout per
  call and a 4 MiB output cap, matching `internal/door/product.go:244-296`. `LabelPrefix`
  defaults to `home` (Task 1.12 sets `hometest`).
- `FindOp(op)` runs `bd list -l home:op:<op> --all -n 0 --json` and returns the bead, if any.
- `Create(d, op)` runs
  `bd create --title … -d … -t decision -l home:decision,project:X,asker:Y,home:op:<op> --metadata @tmp.json --json`.
  Metadata goes through a temp file, never argv.
- `Get(id)` runs `bd show id --json`.
- `ListOwed()` runs `bd list -l home:decision -s open -n 0 --json`.
- `ListRecent(project, since)` runs
  `bd list -l home:decision -l project:X --all --closed-after <since> --sort closed -n 0 --json`
  `[A-20]`.
- `ListByLabel(label, all bool)` supports recovery and wakes.
- `Update(id, meta, add, remove []string)` runs `bd update id --metadata @tmp.json --add-label …
  --remove-label …` as one call.
- `Close(id, reason)` and `Reopen(id)` run `bd close --reason` and `bd reopen`.
- Errors:
  - `ErrTrackerDown`: a connection error (`connection refused`, `dial tcp`, `dolt`) **before**
    any write was sent. The tracker records `downSince` and clears it on the next success.
  - `ErrOutcomeUnknown`: a timeout or connection error on a write call, where the write may
    have landed `[A-10]`.
  - Other failures surface as-is.

**Test cases:**
- The argv for each call, including `--closed-after` and `--sort closed`.
- Metadata passes through `@file`, and the file is removed afterwards.
- A connection-refused fake on `list` gives `ErrTrackerDown` with `DownSince()` set; the next
  success clears it.
- A fake that sleeps past the timeout on `create` gives `ErrOutcomeUnknown`.
- A 5 MiB output is truncated with an error.

Commit: `decisions: bd tracker with op labels and outcome-unknown`.

<verify>
- run: `go test -race ./internal/decisions/ -run Tracker`
  expect: exit 0
</verify>

### Task 1.3: ruling files and signing

**Files:**
- Create: `internal/decisions/ruling.go`, `internal/decisions/sign.go`.
- Test: `internal/decisions/ruling_test.go`, `internal/decisions/sign_test.go`.

**Ruling format** `[D16]`: YAML frontmatter, then a short body.

```markdown
---
artifact_type: ruling
ratification:
  ruled_by: mk
  ruled_at: 2026-09-26T14:03:00Z
  ruling: "Merge feat/bb-catchup"
  transcribed_by: autarch-home
  session_id: <asking thread>
  source: bead:mk-xxxx
  supersedes: ""            # the previous attempt's ruling path, if any
home:
  bead: mk-xxxx
  attempt: att_9c1e…
  revision: "sha256:…"
  asking_thread: thr_abc123
  approved:                 # the snapshot mk approved [A-8]
    options: [{id: merge, label: "Merge feat/bb-catchup", kind: command}, …]
    picked: merge
    command: {argv0: /usr/bin/git, argv: [git, merge, --ff-only, feat/bb-catchup],
              dir: /home/mk/projects/Autarch, env: {PATH: /usr/bin:/bin}, hash: "sha256:…"}
  state: done               # running | done | failed | interrupted | stale | reverted | revert-failed
  outcome: {exit: 0, tail: "…last 2 KiB of output…", evidence: /home/mk/.autarch/attempts/att_9c1e…}
---
# <question>
```

**Rules:**
- `RulingPath`:
  - A project ruling goes to `<project_dir>/docs/decisions/YYYY-MM-DD-<slug>.md`.
  - An estate ruling goes to `$AUTARCH_UQBAR_DIR/rulings/…`. If the Uqbar is unset, filing
    `project:estate` is refused at `File` time, with the message
    `estate-wide decisions need an Uqbar (see G-1)`.
  - Slug collisions get `-2`, `-3` and so on. Each attempt gets its own file.
- **Symlink-safe writes** `[A-19]`: the root and `docs/decisions` are resolved with
  `EvalSymlinks` and must stay inside the resolved project root. The file is written to a temp
  name in the same directory and renamed. The write is refused if the target already exists as
  a symlink.
- `WriteRuling` writes, then signs. A state change rewrites the file and re-signs it.
- `Signer{KeyPath}`:
  - `Sign(path)` runs `ssh-keygen -Y sign -f KeyPath -n autarch-ruling path` and produces
    `path.sig`.
  - `Verify(path, allowedSigners)` runs
    `ssh-keygen -Y verify -f allowedSigners -I home@autarch -n autarch-ruling -s path.sig < path`.
  - A missing key gives `ErrNoKey`. For non-command rulings the caller writes the file unsigned
    and removes any stale `.sig` (P-8).
- The default key is `~/.config/autarch/home_ed25519`, overridable with `AUTARCH_HOME_KEY`.

**Test cases:**
- Round trip: write the frontmatter, parse it, get the same values, including `approved`.
- Slug collisions.
- A path escape is refused, and so is a `docs/decisions` symlink pointing outside the root.
- Signing uses a temp key made with `ssh-keygen -t ed25519 -N '' -f`:
  - sign, then verify, gives OK;
  - flipping one byte makes verify fail;
  - a key not in `allowed_signers` fails;
  - a missing key gives `ErrNoKey`, and there is no `.sig`.

Commit: `decisions: signed ruling files`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Ruling|Sign'`
  expect: exit 0
</verify>

### Task 1.4: attempts and the continuation runner

Depends on Task 1.1 `[A-17]`.

**Files:**
- Create: `internal/decisions/attempt.go`, `internal/decisions/run.go`,
  `internal/decisions/lock.go`.
- Test: `internal/decisions/attempt_test.go`, `internal/decisions/run_test.go`,
  `internal/decisions/lock_test.go`.

**Design:**
- `NewAttempt(root)` makes `att_<16 hex>` and its directory `~/.autarch/attempts/<id>/`
  (0700). `MarkStarted` writes `started` and fsyncs the file and the directory. `WriteOutcome`
  writes `outcome.json` the same way. Nothing in an attempt directory is ever deleted.
- `Runner.Run(ctx, attempt, cmd) Outcome{Exit, Tail, Err}`:
  - `argv`: `exec.Command(argv0, argv[1:]...)`, no shell. It refuses to start if `argv0` no
    longer hashes the same.
  - `script`: copies the script into the attempt directory, checks the copy's hash against the
    frozen hash, and runs `bash <snapshot>` with cwd `dir`. It re-checks deps just before
    start.
  - The env is exactly the frozen env. Nothing else is inherited.
  - It uses its own process group, so a timeout kills the whole group. The timeout defaults to
    10 minutes.
  - Output streams to `output.log` (capped at 16 MiB); the tail keeps the last 2 KiB.
  - It is injected into `Service`, so pick tests use a fake.
- `AcquireOwnerLock(path)` `[A-1]` takes `syscall.Flock(LOCK_EX|LOCK_NB)`, writes the pid into
  the file, and returns a release function. On `EWOULDBLOCK` it returns `ErrOwned{PID}`.

**Test cases:**
- Exit codes.
- The env is limited: `env` output contains no `BB_` or `ANTHROPIC_` variables.
- `dir` is honoured.
- An argv containing `;` or `$(…)` is passed as a literal argument, not interpreted.
- A script edited after freezing is refused before start; the snapshot, not the live file, is
  what runs.
- A timeout kills the child and its grandchild (`sleep 60 & wait`).
- The output tail is capped, and `output.log` holds the full stream up to the cap.
- `started` exists before the child's first byte of output (the child writes a marker; the test
  compares mtimes).
- `AcquireOwnerLock` in a second **process** (the test re-execs itself) returns `ErrOwned`.

Commit: `decisions: attempts, owner lock and continuation runner`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Attempt|Runner|Lock'`
  expect: exit 0
</verify>

### Task 1.5: the service — file, pick, revert, recover

Depends on Tasks 1.1–1.4 and on `internal/serve/projects.go` (Task 0.2), injected as an
interface so `decisions` does not import `serve`.

**Files:**
- Create: `internal/decisions/service.go`.
- Test: `internal/decisions/service_test.go`, using an in-memory `fakeTracker` that implements
  `Tracker` and can inject `ErrTrackerDown` or `ErrOutcomeUnknown` on any call, plus the fake
  runner and a temp signer.

**`File(ctx, req)`** `[A-10]` `[A-19]`:
1. Resolve `req.Project` through the resolver. `project_dir` must equal the resolved root.
2. `Validate`, freeze every command option, compute `Revision`.
3. `FindOp(req.RequestID)`. If a bead exists, return it: filing is idempotent.
4. `Create`.
5. Errors: `ErrTrackerDown` before create gives the fixed message
   `hub tracker unreachable since <t>: nothing was filed; ask mk in chat instead` `[D21]`.
   `ErrOutcomeUnknown` gives `hub tracker did not confirm: the decision may have been filed;
   re-running the same command is safe`.
6. If `req.Supersedes` names an open bead from the same asker, close it with reason
   `superseded by <id>` (the re-ask path).

**States and labels** `[A-3]`. Every transition is one `bd update` unless it says close or
reopen.

| `pick.state` | Bead | Labels added / removed | Next |
|---|---|---|---|
| (none) | open | — | claimed |
| `claimed` | open | +`home:running` | stale, running, interrupted |
| `stale` | open | −`home:running`, +`home:stale` | (re-ask obligation; a later pick is a new attempt) |
| `running` | open | (keeps `home:running`) | finalizing, interrupted |
| `finalizing` | open, then closed | −`home:running` (at close) | closed |
| `interrupted` | open | −`home:running`, +`home:interrupted` | (owed; a later pick is a new attempt) |
| `reverting` | closed or open | +`home:running` | reverted, revert-failed, revert-interrupted |
| `reverted` | reopened | −`home:running` | (owed again) |
| `revert-failed` | unchanged | −`home:running`, +`home:revert-failed` | — |
| `revert-interrupted` | unchanged | −`home:running`, +`home:interrupted` | — |

`finalizing` records `outcome: done|failed` and the obligations.

**`Pick(ctx, id, optionID, revision)`** `[A-2]` `[A-8]`, under the service mutex (the owner lock
is already held by the process):
1. Refuse with 503 if `Recover` has not completed since start.
2. `Get` the bead. It must be open and not `home:running`. `revision` must equal the stored
   revision, else `ErrRevisionMismatch`: the rail must re-read.
3. For a command option with no signing key, refuse with `ErrNoKey` (P-8, G-7).
4. **Claim**: create the attempt directory, write `claim.json`, then one `Update` with
   `pick{attempt, option_id, revision, state: claimed, dir}` and `+home:running`. Move any
   previous `pick` into `history[]`.
5. For a command option: `CheckStale`, then run the precondition inside the attempt (recorded in
   `precondition.json`, 10 s timeout). On failure, set `state: stale` per the table, add a
   `reask` obligation, write a `stale` ruling, and return 409 `{reask: true}` `[A-7]`.
6. `WriteRuling(state=running)`, then `Update(state: running)`.
7. By kind:
   - **command:** `MarkStarted`, `Runner.Run`, `WriteOutcome`.
   - **needs-context** and **ruling-only:** nothing runs.
8. `WriteRuling(final)`, signed. `done` on exit 0 or a non-command kind; `failed` otherwise.
9. `Update(state: finalizing, outcome, obligations)`. Obligations:
   - needs-context: a `wake`;
   - command failure: a `followup` `[A-21]`. It keeps the original options and adds *retry*
     (the same command, re-frozen). It is labelled `home:failed-followup`, with op id
     `<attempt>:followup`;
   - a stale re-ask (step 5): a `reask`.
10. `Close(id, "ruled: <label>")`.
11. Carry out obligations, marking each `done` in metadata:
    - `wake` and `reask` add `home:wake-pending` to the bead. A `reask` wake carries a distinct
      payload: `option went stale: <reason>; file a replacement with supersedes=<id>`.
    - `followup` calls `File` with op id `<attempt>:followup`, so a retry finds the bead that
      was already created.

**`Revert(ctx, id, attempt, revision)`** `[A-5]`:
- Eligible only when that attempt's state is `done` (bead closed) or `interrupted` (bead open),
  its option has a `revert`, and the key exists.
- It uses the same machinery: a new attempt of kind revert, claim with `state: reverting`,
  `CheckStale` on the revert command, `MarkStarted`, run, outcome.
- Exit 0: write a `reverted` ruling that supersedes the original, then `Reopen` the bead once
  with a new revision, so it is owed again with its original options `[D22]`.
- Non-zero: `revert-failed` per the table. The bead is left as it was.

**`Recover(ctx)`** `[A-2]` `[A-4]`. It lists every bead with `home:running` (open or closed)
and every closed bead whose `pick.state` is `finalizing` or has an unfinished obligation. It
reconciles by evidence and never runs a command:

| Found | Evidence | Action |
|---|---|---|
| `claimed` | no `started` | `interrupted` (it never ran) |
| `running`, command | `started`, no `outcome.json` | `interrupted`; ruling rewritten `interrupted` with the evidence path |
| `running`, command | `outcome.json` | finish from step 8 using the recorded outcome |
| `running`, non-command | — | finish from step 8 |
| `finalizing` | bead open | close, then carry out obligations |
| closed | obligations not done | carry them out; each is idempotent by op id |
| `reverting` | no `started` or no outcome | `revert-interrupted` |
| `reverting` | outcome, exit 0 | finish the revert, including a missing `Reopen` |

It is idempotent. On `ErrTrackerDown`, `serve` retries it every 30 s and picks stay at 503.

**Test cases** (each tied to a CUJ autarch-07 step). Each asserts the final bead labels and
metadata, the ruling state, and what `Owed`, `Recent` and `Feed` then return:
- Filing gives an open bead with frozen hashes and a revision. Filing again with the same
  request id returns the same bead and makes one `create` call.
- Tracker down before create gives the fixed message, and no `create` call. A `create` that
  returns `ErrOutcomeUnknown` gives the "may have filed" message.
- Picking a command:
  - the claim `Update` happens before the runner is called;
  - `started` exists before the runner is called;
  - the ruling says `running` when the runner is called;
  - then `done`, the bead is closed, and no wake is pending (the feed carries it).
- A double pick (two goroutines) runs the fake runner once; the second returns `ErrNotOwed`.
- A revision mismatch is refused without a claim.
- An edited script gives 409 stale, no runner call, `home:stale`, and one `reask` wake.
- A failing precondition gives the same, and the precondition ran inside the attempt.
- A failing command gives a `failed` ruling plus one follow-up bead with the original options
  plus *retry*. Crashing after close and running `Recover` still gives exactly one follow-up.
- No key: a command pick is refused; a ruling-only pick writes an unsigned ruling.
- `Recover` over each row of the table gives the listed result. The runner is never called, and
  a second `Recover` is a no-op.
- Revert of a `done` pick runs the revert once and reopens the same bead. Revert with the wrong
  attempt or revision is refused. A crash between the revert's outcome and `Reopen` is finished
  by `Recover`.
- `ruling-only` makes no runner call.

Commit: `decisions: file, pick once, revert and crash recovery`.

<verify>
- run: `go test -race ./internal/decisions/ -run Service -count=3`
  expect: exit 0
</verify>

### Task 1.6: feed, wakes, recent and trial stats

**Files:**
- Modify: `internal/decisions/service.go`.
- Test: `internal/decisions/feed_test.go`.

**Design:**
- `Recent(project, limit)` `[A-16]` returns closed and recently picked decisions from
  `ListRecent`. Each carries the picked label, the state, the ruling path, whether its
  signature verifies, and whether revert is eligible (with the attempt and revision needed).
- `Owed()` also computes `stale` for each command option through `CheckStale`, which runs
  nothing, so the rail marks it before any pick `[A-21]`.
- `Feed(project, thread, since)` returns up to 10 lines, newest first `[A-20]`:
  - rulings in that project closed since `since` (default 14 days) **whose signature
    verifies** (P-8);
  - unioned with rulings on beads this thread asked, in any project;
  - each line reads `ruled YYYY-MM-DD "<label>" → <state> (<bead>)`.
- The label is quoted, stripped of newlines and control characters, and clipped to 80 runes. The
  line is clipped to 140.
- A line never contains the question, the reasoning or any metadata other than the label and
  the state `[D21]`.
- It uses a per-project cache with a 30 s TTL that is invalidated by any `Pick`, `Revert` or
  `Recover` in this process.
- `Wakes()` returns beads labelled `home:wake-pending` and not `home:undeliverable`, grouped by
  `asking_thread`. Each wake carries:
  - the bead id and kind (`answer` or `reask`);
  - the question;
  - the picked label;
  - the state;
  - the ruling path.
  The question is the asker's own text going back to that same asker, which does not break
  `[D21]`.
- `AckWakes(ids)` removes `home:wake-pending`. `MarkUndeliverable(ids)` swaps it for
  `home:undeliverable` `[A-15]`.
- `Stats(since)` reports decisions filed per ISO week, and the median and p90 of
  `pick.at − created_at` for picked decisions `[A-20]`.

**Test cases:**
- A label with `\n` or an injection-shaped string is flattened and clipped.
- The question never appears in feed output.
- An unsigned ruling is absent from the feed.
- A thread's own answer from another project appears in its feed.
- Two answers to one thread form one group; a re-ask wake has kind `reask`.
- Ack removes the label; undeliverable swaps it.
- `Owed` marks an option stale after its script is edited, and runs nothing.
- `Recent` reports revert eligibility only for `done` or `interrupted` picks with a revert.
- Stats over fixed timestamps, using `pick.at`.

Commit: `decisions: feed, wakes, recent and trial stats`.

<verify>
- run: `go test -race ./internal/decisions/ -run 'Feed|Wake|Recent|Owed|Stats'`
  expect: exit 0
</verify>

### Task 1.7: the HTTP API on `autarch serve`

**Files:**
- Create: `internal/decisions/api.go`.
- Modify: `internal/serve/serve.go` (mount it when `Config.Decisions != nil`; take the owner
  lock and run `Recover` in `Run`, P-3a) and `cmd/autarch/serve.go` (build a
  `decisions.Service` with `BDTracker{HubDir}`, the resolver, the default signer and the
  runner). `Config` gains `Decisions` here, not earlier `[A-17]`.
- Test: `internal/decisions/api_test.go`, using `httptest` against `Routes` with a fake tracker;
  `internal/serve/owner_test.go` for the lock.

**Routes** (all behind the P-3 middleware):

| Route | Behavior |
|---|---|
| `GET /api/decisions` | Returns `{owed:[…], tracker:{up, down_since}, signing, recovered}`. When the tracker is down it returns 200 with `up:false`, never an empty "nothing owed" list. |
| `POST /api/decisions` | Files a decision (the same as the CLI). Takes `request_id`. |
| `GET /api/decisions/recent?project=&limit=` | Returns `Recent` `[A-16]`. |
| `POST /api/decisions/{id}/pick` | Takes `{option_id, revision}`. Returns 200 with the ruling; 409 when stale (with `reask`), already picked, or the revision differs; 503 when the tracker is down or recovery has not completed. |
| `POST /api/decisions/{id}/revert` | Takes `{attempt, revision}`. |
| `GET /api/decisions/feed?project=&thread=` | Returns the feed lines. |
| `GET /api/decisions/wakes` | Returns pending wakes. |
| `POST /api/decisions/wakes/ack` | Takes `{ids, undeliverable}`. |
| `GET /api/decisions/stats?since=` | Returns the trial stats. |

**Test cases:**
- The status code for each route and error.
- The tracker-down shape.
- Pick returns 409 on a second call, and 503 before `Recover` completes.
- The body size is capped at 64 KiB.
- Owner lock: two `serve` processes (the test re-execs the test binary) on different ports —
  the second exits non-zero with `owns decisions` before it binds. A second `serve` on an
  occupied port also exits before running `Recover` (its fake tracker records no calls).

Commit: `serve: decisions API and single owner`.

<verify>
- run: `go test -race ./internal/decisions/ ./internal/serve/`
  expect: exit 0
</verify>

### Task 1.8: the CLI helper and the MCP tool

**Files:**
- Create: `cmd/autarch/decide.go` and `cmd/autarch/decide_test.go`.
- Modify: `cmd/autarch/main.go` (`root.AddCommand(decideCmd())`).
- Modify: `pkg/mcp/server.go` (add `autarch_file_decision`, with a write scope checked like the
  others at `:317`).

**Commands:**
- `autarch decide file` reads a JSON request on stdin (the bead schema's option list plus
  `question`, `project`, `recommendation`, and optional `request_id` and `supersedes`).
  - A missing `request_id` is generated from sha256 of the thread plus the request body, so a
    blind retry of the same request is idempotent `[A-10]`.
  - The thread comes from `--thread` or `$BB_THREAD_ID`. It is required unless `--asker mycroft`.
  - `project_dir` comes from `--project-dir` (default: the git root of the working directory),
    and must match the resolved project.
  - On success it prints `{"bead":…,"request_id":…}`.
  - Exit 3: tracker down, nothing filed. Exit 4: outcome unknown, re-run the same command.
    Exit 2: validation error, naming the bad option and the rule.
- `autarch decide list` reads through the running service if `/health` answers, and otherwise
  directly through `BDTracker`.
- `autarch decide verify <ruling.md> [--allowed-signers]` verifies a ruling; `allowed-signers`
  defaults to `$AUTARCH_UQBAR_DIR/allowed_signers`.
- `autarch decide stats [--since 14d]` prints the trial stats.

There is **no pick in the CLI**. Picks happen only on the rail, so mk is the one picking
`[D14]`.

**Test cases:**
- Fake `bd` on PATH gives the argv for `file`, including `home:op:<id>`.
- Running `file` twice with the same stdin makes one `create`.
- Exit 3 when the tracker is down on the lookup; exit 4 when `create` times out.
- Exit 2 on `kind: command` with no command, and on a disallowed env key.
- MCP `tools/list` includes `autarch_file_decision`, and a call without the write scope is
  refused.

Commit: `decide: filing helper CLI and MCP tool`.

<verify>
- run: `go test -race ./cmd/autarch/ -run Decide`
  expect: exit 0
- run: `go test -race ./pkg/mcp/`
  expect: exit 0
</verify>

### Task 1.9: Mycroft escalates through decisions, and `mycroft dispatch` `[A-13]`

**Files:**
- Modify: `internal/mycroft/escalate/escalate.go:55-120`.
- Modify: `internal/mycroft/scheduler/orchestrator.go`: `suggest()` (`:97-110`) and the T2
  out-of-allowlist branch of `autoDispatchFiltered` (`:123-130`). Add an exported
  `Dispatch(agent, bead)` that wraps `dispatchToAgent` (`:149`).
- Modify: `cmd/mycroft/main.go`: add `dispatchCmd`, registered in `init()` (`:426`). Mycroft's
  commands live in `cmd/mycroft`; `autarch` has no mycroft subcommand, so it goes here.
- Modify: `internal/tui/views/mycroft.go` (the badge).
- Test: `internal/mycroft/escalate/escalate_test.go`,
  `internal/mycroft/scheduler/escalate_wiring_test.go`, `cmd/mycroft/dispatch_test.go`.

**Design:**
- `DecisionQueue` gets a `decisions.Filer` (the `File` and `ListOwed` subset).
- `NewOrchestrator` gains an option `WithEscalations(*escalate.DecisionQueue)`. With it set:
  - `suggest()` calls `Add` next to its existing `logDispatch(… ActionSuggest …)`;
  - the T2 out-of-allowlist branch calls `Add` next to its existing log line.
- `Add(p)` files this decision:
  - asker `mycroft`; project from the bead's project, resolved as in Task 1.5;
  - request id `mycroft:<project>:<bead>:<agent>`, so each patrol cycle finds the same bead
    instead of filing another;
  - the question is `Dispatch <agent> on <bead>: <title>?`;
  - options:
    - *dispatch*: kind command, argv
      `["mycroft","dispatch","--bead",<bead>,"--agent",<agent>]` in the project root;
    - *skip*: ruling-only;
    - *look first*: needs-context. With no asking thread it records the ruling only in v1.
  - `Reasoning` goes in the bead description only. It never reaches the feed.
- `mycroft dispatch --bead --agent` loads one `FleetView` from `newSource()`, finds the agent and
  the bead, and refuses (exit 1, naming which) if either is gone or the bead is no longer
  ready. It then builds the orchestrator as `runCmd` does and calls `Dispatch` once.
- `Len`, `All` and `HighestSeverity` read `ListOwed` filtered by `asker:mycroft`, cached for
  10 s.
- `Get` and `Remove` are deleted if unused outside tests. Otherwise `Remove` becomes a no-op
  with a deprecation comment.
- The TUI badge shows `?` when the tracker is down, instead of `0`.

**Test cases:**
- `Add` makes one `File` call with the right labels and request id; a second cycle with the same
  suggestion makes no new bead.
- An orchestrator at T1 fed a `FleetView` with one ready bead produces one decision bead (patrol
  → bead).
- `mycroft dispatch` with a fake spawner and a fake source calls `Spawn` once; with the bead
  missing it exits 1 and calls nothing (bead → outcome).
- `Len` reads through the filer. Tracker down makes `Len` return the last known count, and
  `Stale()` returns true.

Commit: `mycroft: escalations file decision beads; mycroft dispatch`.

<verify>
- run: `go test -race ./internal/mycroft/... ./cmd/mycroft/ ./internal/tui/views/`
  expect: exit 0
</verify>

### Task 1.10: the bb plugin (rail, wakes, feed, `bb home ask`) `[A-12]`

**Files** (created with `bb plugin new bb-plugin-autarch` under `integrations/`, then edited):
- `integrations/bb-plugin-autarch/package.json`, with `typecheck` (`tsc --noEmit`) and `test`
  scripts matching the scaffold.
- `contract.ts`: `defineRpcContract` with `listOwed`, `listRecent`, `pick`, `revert` and
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
  - **Wake loop** (background) `[A-15]`:
    - Every 15 s, plus immediately after a pick, it reads `/api/decisions/wakes`.
    - For each thread it sends one message through
      `bb.sdk.threads.send({threadId, input, mode: "queue-if-active"})`, listing that thread's
      answers and re-asks.
    - On success it acks. On a rejection that the thread is archived or deleted, it acks with
      `undeliverable: true`. Other failures retry on the next tick.
  - **Feed** (`feed.ts`):
    - A background refresher fetches `/api/decisions/feed` per project and thread every 30 s
      into an in-memory cache.
    - `bb.agents.configure` is synchronous. It reads only the cache and returns
      `{instructions, tools: [], skills: []}`.
    - The instructions are a fixed header, `Recent rulings in this project (label and outcome
      only):`, at most 10 lines, and a two-line note on filing: "When you need mk to decide,
      run `bb home ask` with JSON options. The default kind is needs-context; give an exact
      argv or script only when a pick should just run it." The whole text is kept under the
      4096-character cap, dropping the oldest lines first.
    - With an empty cache or the service down, it returns no instructions.
  - **CLI**: `defineCli` with `stdin: true` registers `bb home ask`. It reads the request from
    stdin, takes the thread from `ctx.threadId`, generates a `request_id` if none is given, and
    calls `POST /api/decisions`. Outside a thread it is refused. It exits 3 or 4 as the Go
    helper does.
- `app.tsx`:
  - A `navPanel` "Home" panel with an `experimental_sidebarAccessory` badge (the owed count, or
    `!` when the tracker is down).
  - The rail lists owed decisions grouped by project. Each shows the question, then each option
    with:
    - its label and kind;
    - for commands: the exact argv or script, `dir`, effective env, precondition, deps ("reads
      <deps> at run time"), "runs as mk in <dir>", and any revert;
    - a recommendation mark;
    - `stale`, `interrupted`, `failed follow-up` and `undeliverable` chips;
    - command options disabled with "needs Home key (G-2)" when signing is missing.
  - An undeliverable wake shows its answer so mk can hand it on.
  - Picking sends `{option_id, revision}` from what was rendered. A 409 re-reads the decision.
  - A "Recent" section from `listRecent` shows each ruling with a Revert button when eligible,
    and an "unsigned (proposed)" chip when its signature does not verify. It survives a panel
    reload.
  - Banners: "tracker down since …", "starting: recovering" (the 503 state) and "rulings
    unsigned: no Home key".
- `__tests__/server.test.ts`, using the SDK's `testing/fake-sdk.ts`:
  - `configure` returns cached lines, returns nothing with an empty cache, and stays under
    4096 characters with 10 long lines;
  - the wake loop sends one message per thread and acks;
  - a fake rejection for an archived thread acks as undeliverable and does not resend (the
    rotation case);
  - `bb home ask` outside a thread is refused, and inside one posts with `ctx.threadId`;
  - `listRecent` after a simulated reload returns the same rulings.
- `README.md`: build, the local install command (for mk) and settings.

**Steps:**
1. Scaffold the plugin with `bb plugin new`.
2. Write `contract.ts`, then the failing tests in `__tests__/`.
3. Write `server.ts` and `feed.ts` until the tests pass.
4. Write `app.tsx`.
5. Run `npm run typecheck`, `npm test` and `bb plugin build`. Each must exit 0.
6. Commit: `bb-plugin-autarch: Home rail, wakes, feed and bb home ask`.

**Not in this task:** `bb plugin install`, which is mk's step (G-3).

<verify>
- run: `cd integrations/bb-plugin-autarch && npm run typecheck && npm test`
  expect: exit 0
- run: `cd integrations/bb-plugin-autarch && bb plugin build`
  expect: exit 0
</verify>

### Task 1.11: the scenario harness `[A-18]`

**Files:**
- Create: `internal/homee2e/harness_test.go` (build tag `homee2e`), `internal/homee2e/bdfault/`
  (a fault-injecting `bd` wrapper built by the harness), `internal/homee2e/README.md`.

**Design:**
- The harness builds `autarch`, starts `serve` as a real child process on `127.0.0.1:0` with a
  temp token, a temp key (`AUTARCH_HOME_KEY`), a temp attempts root and a scratch git project.
- Commands used by scenarios write a **start marker**, bump an **invocation counter** file, and
  write a **completion marker** at the end. Run-once is asserted from the counter, never from
  a marker's absence alone.
- `bdfault` wraps a real or fake `bd`. `HOMEE2E_BD_FAULT` selects a fault: `down` (connection
  refused), `timeout-after-write` on a named subcommand, or `fail-nth=N`.
- Two modes:
  - `HOMEE2E_MODE=fake` (default): `bd` is a file-backed fake, so it runs anywhere.
  - `HOMEE2E_MODE=hub`: the real hub tracker with `LabelPrefix=hometest` (Task 1.12).
- Results are written as one JSON object per scenario to `$HOMEE2E_OUT` (default
  `$TMPDIR/homee2e.jsonl`): `{scenario, pass, counter, bead_state, labels, ruling_state,
  verified}`.

**Scenarios:**
1. **done:** pick a command. Counter is 1, ruling `done`, `autarch decide verify` passes with the
   real `ssh-keygen`, the bead is closed, the feed shows the label only.
2. **stale:** edit the script, then pick. 409 with `reask`, counter 0, `home:stale`, one re-ask
   wake.
3. **crash mid-run:** pick a `sleep 30` script, SIGKILL `serve` after the start marker, and
   check that the runner's process group is gone (`kill -0 -<pgid>` fails, killing it if the
   kernel left it). Restart. The bead is owed with `home:interrupted`, the counter stays 1, the
   completion marker is absent, and the evidence directory is intact.
4. **crash after close:** fail a command with `HOMEE2E_BD_FAULT=kill-after=close` (the wrapper
   SIGKILLs `serve` right after `bd close` returns). Restart. Exactly one follow-up bead exists.
5. **two serves:** start a second `serve` on another port. It exits non-zero with
   `owns decisions` and the first still answers.
6. **filing retry:** `timeout-after-write` on `create`, then re-run the same `decide file`. Exit
   4, then exit 0, and one bead.
7. **tracker down at start:** start with `down`. `/api/decisions/…/pick` returns 503; clear the
   fault; within 35 s recovery completes and picks work.

**Steps:** write the harness, run it in fake mode, fix what it finds, then commit
`homee2e: scenario harness for run-once and recovery`.

<verify>
- run: `go test -tags homee2e -race ./internal/homee2e/ -count=1`
  expect: exit 0
- run: `jq -s 'map(select(.pass|not))|length' "${HOMEE2E_OUT:-${TMPDIR:-/tmp}/homee2e.jsonl}"`
  expect: contains "0"
</verify>

### Task 1.12: end-to-end check on zklw against the real hub (agent-run, before handing to mk)

This task has no new code. It runs the Task 1.11 harness in hub mode, so the rail mk uses is not
polluted: every label is `hometest:*`.

**Steps:**
1. On zklw, from the repo root: `HOMEE2E_MODE=hub AUTARCH_HUB_DIR=/home/mk/hub
   HOMEE2E_OUT=$tmp/hub.jsonl go test -tags homee2e -race ./internal/homee2e/ -count=1`.
   Scenarios 1–3 and 5 run against the real tracker. Scenarios 4, 6 and 7 inject faults into the
   `bd` wrapper and are run in fake mode only, because they must not fault the shared hub.
2. The harness closes every `hometest:*` bead it created with reason `home e2e check`.
3. Record the transcript and the JSON results in `docs/research/2026-09-26-home-e2e-check.md`.

<verify>
- run: `test -s docs/research/2026-09-26-home-e2e-check.md`
  expect: exit 0
- run: `grep -c '"pass":true' docs/research/2026-09-26-home-e2e-check.md`
  expect: contains "4"
</verify>

---

## Explicitly out of scope

- The brief continuation `[D22]`.
- Badges beyond the Home entry (step 2).
- Lattice (step 3), the map (step 4, estate-map WI-5 adds it to this plugin), Mycroft's
  proposals (step 5) and the companion (step 6).
- The Clavain `UserPromptSubmit` feed hook (P-7) and the Clavain filing helper plus guidance
  text `[D15]`. Both are in the Clavain lane: a follow-up bead in Clavain adds a wrapper that
  calls `autarch decide file`, and adds the shared-guidance instruction. G-5 accepted P-7, so
  that follow-up is not a dependency of the feed.
- Successor-thread resolution for rotated threads (deferred by mk under G-9).
- `bb.ui.requestInput` in-thread cards. The rail is the only picking surface in v1.
- Removing the standalone servers.
- Mounting Pollard.
- A note without a wake. This is unverified, and P-5 wakes are acceptable for v1.

## Execution preconditions and mk gates

These are **not agent-completable**. Each is a checklist item for mk; none is a DONE WHEN.

- **G-0 (ruled 2026-09-26):** mk approved the Home epic `mk-okek` with S0 `mk-okek.1` and S1
  `mk-okek.2`. Still required before execution: a second review-astra pass on this revision,
  and the zklw CI status for this repo (`zklw-ci status --repo mistakeknot/Autarch --json`).
- **G-1:** Create the Uqbar repo, which must be private, and set `AUTARCH_UQBAR_DIR`. Until then
  estate-wide decisions are refused, and project decisions still work.
- **G-2:** Create Home's key on zklw:
  `ssh-keygen -t ed25519 -N '' -C home@autarch -f ~/.config/autarch/home_ed25519`.
  Commit its public half to `Uqbar/allowed_signers` as
  `home@autarch namespaces="autarch-ruling" <pubkey>`.
- **G-3:** Install the plugin: `bb plugin install path:integrations/bb-plugin-autarch`.
- **G-4:** Republish `autarch-plugin`, because `plugin.json` changes in Task 0.5.
- **G-5 (ruled 2026-09-26)** `[A-11]`: mk accepted both departures. P-7: the feed is injected
  by `agents.configure` at session start or resume, instead of the per-turn hook in `[D17]`.
  P-5: wakes go only to needs-context picks and re-asks; command and ruling-only outcomes reach
  the asking thread through the feed.
- **G-6:** Start the trial by walking autarch-07 on the real rail with a real decision. Trial
  metrics come from `autarch decide stats`.
- **G-7 (ruled 2026-09-26)** `[A-9]`: P-8 as written. Without a key, command picks are disabled;
  needs-context and ruling-only picks write unsigned, proposed rulings; the feed carries only
  verified rulings.
- **G-8 (ruled 2026-09-26)** `[A-21]`: `[D18]` is narrowed. A failed command's follow-up keeps
  the original options plus *retry*. Staleness shows at listing without running anything, and
  the precondition runs only at pick.
- **G-9 (ruled 2026-09-26)** `[A-15]`: v1 marks a wake to an archived or deleted thread
  undeliverable and shows it on the rail. Following a Clavain handoff record to the successor
  thread is deferred; Home does not read handoff records in v1.

## Verification

- Whole tree: `go build ./cmd/... && go test -race ./...`.
- Bind: `go test -race -run Bind ./internal/serve/`.
- Plugin: `npm run typecheck`, `npm test` and `bb plugin build` in
  `integrations/bb-plugin-autarch`.
- Harness: `go test -tags homee2e -race ./internal/homee2e/` in fake mode (Task 1.11) and hub
  mode (Task 1.12).
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
   ambiguity or symlink escape. Gurgeh's signals reach `/signals/ws` through one broker.
   ```check
   go test -race ./internal/serve/ ./internal/gurgeh/server/
   ```
4. `autarch mcp` serves the existing MCP tools plus `autarch_file_decision`, and `autarch-mcp`
   still works.
   ```check
   go test -race ./cmd/autarch/ -run MCP && go test -race ./pkg/mcp/
   ```
5. Options have ids and validate with needs-context as the default. Commands are argv or script
   only, disallowed env keys are rejected, and the revision covers the whole decision. Command
   hashes cover argv0, argv or script and deps, dir, env, revert and precondition. Staleness is
   checked without running anything.
   ```check
   go test -race ./internal/decisions/ -run 'Validate|Freeze|Stale|Revision'
   ```
6. The tracker writes decision beads through `bd` with metadata passed by file and an op label.
   Tracker down and outcome unknown are distinct errors, and nothing is spooled.
   ```check
   go test -race ./internal/decisions/ -run Tracker
   ```
7. Ruling files carry the ratification block, the Home fields and the approved snapshot. They
   are written symlink-safely, signed with `ssh-keygen -Y` and verified against
   `allowed_signers`.
   ```check
   go test -race ./internal/decisions/ -run 'Ruling|Sign'
   ```
8. Attempts are recorded before anything runs, commands run without a shell or from a verified
   snapshot, and a second process cannot take the owner lock.
   ```check
   go test -race ./internal/decisions/ -run 'Attempt|Runner|Lock'
   ```
9. A pick claims before it runs, runs at most once under concurrency, becomes a re-ask when
   stale or when the precondition fails, files exactly one follow-up on failure even across a
   crash, refuses commands without a key, and never re-runs on recovery. Revert reopens the
   same bead once.
   ```check
   go test -race ./internal/decisions/ -run Service -count=3
   ```
10. The feed carries only the quoted, clipped label and state of verified rulings, plus the
    thread's own answers. Wakes batch per thread, re-asks are distinct, and undeliverable wakes
    stop. Recent rulings report revert eligibility.
    ```check
    go test -race ./internal/decisions/ -run 'Feed|Wake|Recent|Owed|Stats'
    ```
11. The API and the single-owner rule hold: 503 before recovery, 409 on a second pick, and a
    second `serve` exits before it binds.
    ```check
    go test -race ./internal/decisions/ ./internal/serve/
    ```
12. Mycroft's suggestions and out-of-allowlist dispatches file decision beads once per
    suggestion, `mycroft dispatch` dispatches once, and `DecisionQueue` keeps no private list.
    ```check
    go test -race ./internal/mycroft/... ./cmd/mycroft/ && ! grep -nE '^[[:space:]]+decisions[[:space:]]+\[\]PendingDecision' internal/mycroft/escalate/escalate.go
    ```
13. The bb plugin typechecks, its fake-SDK tests pass, and it builds.
    ```check
    cd integrations/bb-plugin-autarch && npm run typecheck && npm test && bb plugin build
    ```
14. The scenario harness passes all seven scenarios in fake mode.
    ```check
    HOMEE2E_OUT=$(mktemp) && go test -tags homee2e -race ./internal/homee2e/ -count=1 && [ "$(jq -s 'map(select(.pass|not))|length' "$HOMEE2E_OUT")" = "0" ]
    ```
15. The hub-mode run passed its four scenarios (done, stale, crash mid-run, two serves) and
    cleaned up its beads.
    ```check
    test -s docs/research/2026-09-26-home-e2e-check.md && [ "$(cd /home/mk/hub && bd list -l hometest:decision -s open -n 0 --json | jq length)" = "0" ]
    ```
16. mk walks autarch-07 on the installed rail (G-6). This is recorded by mk, and no command can
    check it.
