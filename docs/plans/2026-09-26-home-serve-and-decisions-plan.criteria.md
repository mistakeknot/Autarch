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
