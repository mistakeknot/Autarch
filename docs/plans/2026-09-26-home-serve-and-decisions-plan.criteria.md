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
