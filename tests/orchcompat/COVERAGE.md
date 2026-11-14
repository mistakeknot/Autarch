# Aleph pack S2 (AU): coverage table

Characterization tests that pin today's behavior of `autarch needs-mk`, home-ask/v2, the Citadel plugin's
answer routing and `autarch serve`. They pass at Autarch baseline `b17577fd4062dc57c6839833e759767b8329ca57`
and at main `d9213f8`. They test through the real `autarch` binary, a fake `bb` on PATH, temp HOME and data
directories and an ephemeral loopback port. Every `autarch` subprocess the suite starts, help probes
included, runs with an explicit environment: a temporary HOME, a fixed PATH (the fake `bb` first where a test
files cards), and only the variables the test sets itself. No ambient `AUTARCH_*`, `BB_*` or `XDG_*` variable
reaches it. The one process that sees the runner's environment is the `go build` of `autarch` at suite start
(skipped when `ORCHCOMPAT_AUTARCH` names a prebuilt binary), which runs no `autarch` code. The "serve not
running" case uses a loopback port the test opened and closed itself. Nothing touches a live bb server, Citadel
database, GitHub or thread storage.

The fake `bb` is the test binary re-run through a PATH wrapper. Under `go test -race` that binary is race-built,
and the race runtime sleeps one second at every process exit by default, which made each fake `bb` call cost a
second (about 11 minutes for the package on two CPUs). The wrapper sets `GORACE=atexit_sleep_ms=0` for the fake
`bb` only; it still runs race-instrumented and no test is skipped under `-race`. Every subprocess wait has a
local deadline (60 s) and the HTTP client a 20 s timeout, so a hung child fails its own test.

Go suite: `tests/orchcompat/`. TypeScript suite: `integrations/bb-plugin-autarch/__tests__/orchcompat/`.

| area | behavior pinned | status | test file | notes |
| --- | --- | --- | --- | --- |
| needs-mk file | card title, body, labels, home-ask/v2 block, identity and request key | covered | needsmk_file_test.go | independent oracles for the request key and description |
| needs-mk file | routing comment `Asked from this thread (Request KEY).` | covered | needsmk_file_test.go | |
| needs-mk file | replay by the same thread, same ask from two threads, reused request, explicit request key | covered | needsmk_file_test.go | |
| needs-mk file | comment failure then repair; create failure; lost create answer then replay | covered | needsmk_file_test.go | exit 4 (outcome unknown) |
| needs-mk file | legacy registry refusal, registered card gone or changed, unparseable card, earliest canonical card across pages | covered | needsmk_file_test.go | |
| needs-mk file | per-key lock file under the runtime dir | covered | needsmk_file_test.go | |
| needs-mk file | root-run block and script bytes in the request | covered | needsmk_file_test.go | |
| needs-mk file | usage errors (exit 2), no bb call before the checks pass | covered | needsmk_file_args_test.go | |
| needs-mk file | owner thread, filing preconditions, warnings, existing label not recreated | covered | needsmk_file_args_test.go | |
| needs-mk file | project alias rewritten to the bound name | covered | needsmk_file_args_test.go | |
| needs-mk file | Home down (exit 3) | covered | needsmk_file_args_test.go | |
| needs-mk file | move filing | covered | needsmk_file_args_test.go | |
| needs-mk adopt-move | attach a move to a filed card, conflicting move refused | covered | needsmk_file_args_test.go | |
| needs-mk footer | four sections, cursor file, per-caller cursor, caller selection, database lookup, empty database, never writes the database | covered | needsmk_footer_test.go | |
| home-ask/v2 | every field rule, accepted and rejected examples, validation errors at the CLI (exit 2) | covered | needsmk_file_args_test.go | |
| home-ask/v2 `urgency` field and `--urgency` flag | blocker accepted where the build has the field; absent where it does not | covered (build-aware) | needsmk_urgency_test.go | the baseline lacks the field, main has it; each test skips on the other side, so each side runs one |
| exit code 5 (already ruled) | mapped in `cmd/autarch/exit.go` | not covered | none | `needs-mk file` can return `ErrAlreadyRuled` from `internal/homeask/update.go` when refiling a picked card or when a pick races an update |
| autarch serve | startup line, token file (64 hex, mode 0600, dir 0700), token reuse, startup refusals | covered | serve_test.go | |
| autarch serve | default scan root, config candidate order, `~` expansion in scan_roots, `--project-dir` override, bad TOML fatal | covered | serve_test.go | |
| autarch serve | default token file, default address 127.0.0.1:8110 (asserted from `--help`, no bind to that port) | covered | serve_test.go | |
| autarch serve | `/health`, `/projects`, guard order (Origin 403, Host 403, token 401), allowed origin, gurgeh mount | covered | serve_test.go | ephemeral port only |
| Citadel plugin routing | a card routes to the thread that wrote the earliest agent comment; later claim never reroutes; owner first; createdAt order; id tie-break; user comments ignored | covered | answer-routing.test.ts | |
| Citadel plugin routing | unclaimed card: waiting, then unroutable after 60 s; late agent comment opens it | covered | answer-routing.test.ts | |
| Citadel plugin routing | frozen route: earlier comment or no agent comment invalidates | covered | answer-routing.test.ts | |
| Citadel plugin routing | pull card has routing mode pull | covered | answer-routing.test.ts | |
| Citadel plugin delivery | wake envelope `[home-msg <op> #n] <payload>`, mode queue-if-active | covered | answer-routing.test.ts | |
| Citadel plugin delivery | archived recipient: successor, vizier, stall, reroute, reroute-failed, hop limit | covered | answer-routing.test.ts | |
| Citadel plugin mirror | Ruled comment, notify false, label removed, task closes unless a thread is working | covered | answer-routing.test.ts | |
| answer routing, bb side | a comment from mk on a card delivered to the thread that wrote the card's latest agent comment | not covered | none | the delivery is done by bb itself and needs a live bb server; the plugin's observable half (earliest agent comment, frozen at open) is pinned instead |
| live services | real bb server, live Citadel database, GitHub, real thread storage | not covered | none | by design; every test uses fakes and temp directories |
