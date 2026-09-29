## Acceptance Criteria

1. The whole Go tree builds and every test passes with the race detector.
   ```check
   go build ./cmd/... && go test -race ./...
   ```
2. `autarch serve` refuses a non-loopback address.
   ```check
   go test -race -run 'Bind' ./internal/serve/ ./cmd/autarch/
   ```
3. The service mounts Bigend, Gurgeh and Signals under prefixes behind the token, Host and
   Origin checks, with no query-string token. `/health` is open. Projects resolve without
   ambiguity or symlink escape and report root identity. Gurgeh's signals reach `/signals/ws`
   through one broker. `serve` has no decision route or writer and imports no decisions code; a failing
   `go list` fails the check `[D-15]`.
   ```check
   go test -race ./internal/serve/ ./internal/gurgeh/server/ && go test -race -run 'ReadOnly' ./internal/serve/ && deps=$(go list -deps ./internal/serve/) && ! printf '%s\n' "$deps" | grep -qE 'internal/(decisions|homeask)'
   ```
4. `autarch mcp` serves the existing MCP tools plus `autarch_file_decision`, and `autarch-mcp`
   still works.
   ```check
   go test -race ./cmd/autarch/ -run MCP && go test -race ./pkg/mcp/
   ```
5. Options have ids, one of three kinds, bounded instructions and a reversible mark with its
   refusals. Identity, revision and semantic key cover the scope, approval tokens are refused
   in any text, and Go and TS agree on every vector.
   ```check
   go test -race ./internal/homeask/ -run 'Validate|Revision|Identity|Semantic|Vectors' && cd integrations/bb-plugin-autarch && npm test -- __tests__/model.test.ts
   ```
6. The store files once per request id without upsert, records a pick only by conditional
   update, refuses superseded or stale picks, keeps obligations fenced by state and attempt,
   never tears a pick from its obligations under SIGKILL, and exports events append-only in
   immutable per-store segments outside the checkout, idempotent across a crash `[D-13]`.
   Requests are one registry; mentions are keyed by decision and thread; obligations are one
   outbox row per message with a snapshotted recipient, payload, unique per-recipient `op`
   and `after_id`, a fan-out inserts its whole recipient set in one transaction, each send
   is its own attempt row, a Resend permit is consumed with the attempt it creates, and
   `voided_at` suppresses sending without changing a row's delivery state `[E-2]` `[E-3]`
   `[E-8]` `[F-2]` `[F-6]` `[G-1]` `[G-2]`. Migrations are staged (expand-only, `min_reader_version`), a
   failed activation after a migration leaves the old instance working, and a locked database
   initializes degraded and recovers without a reload `[D-7]` `[D-10]`.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/store.test.ts __tests__/store-crash.test.ts __tests__/export.test.ts __tests__/migrations.test.ts
   ```
7. Ruling files carry the ratification block and the Home fields, are idempotent, and are
   written only under the root identity saved at filing, refusing symlinks, a replaced root and
   a retargeted root.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/ruling.test.ts
   ```
8. Filing is idempotent and scoped; a pick is recorded once per decision; a reused pick id with
   other parameters is refused; a replacement blocks its predecessor's pick; one obligation's
   failure never blocks another decision; dismissal survives restart; mentions attach once,
   only by `mention_of` or semantic equality over a structured `subject` and every answer
   semantic (`ask_key` alone marks "possibly related"; unrelated asks never merge); a
   replacement never mentions, and `supersedes` with `mention_of` is refused; a retried filing
   returns its canonical identity while serve is down `[E-1]` `[E-7]` `[E-10]`; every
   supersede interleaving resolves in one transaction with an explicit outcome
   (`already-ruled`, `withdrawn`, `already-superseded`, `superseded`, `stale`) `[D-4]` `[D-8]`.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/service.test.ts
   ```
9. The feed carries only the quoted, clipped label from pick records, per thread, with own
   answers reserved within 4096 UTF-16 units; only effective rulings count, and a superseded
   ruling shows as void; a thread new to a project gets the shared project feed, and the
   caches are invalidated on override `[D-4]` `[D-17]`; a replacement ruling appears at once
   with `supersedes <old id>`; undeliverable wakes are listed regardless of age;
   stats report rail share, time-to-pick, override rate and ruling-to-override time.
   ```check
   cd integrations/bb-plugin-autarch && npm test -- __tests__/feed.test.ts __tests__/service.test.ts
   ```
10. Wakes complete only on `sent`, a dispatched queued row, or exactly one accepted event
    matching recipient, framed marker and snapshotted payload (a quoted marker, an edited
    unbound row or several matches do not count), become undeliverable on archive, delete or queue
    removal, survive restart while queued, drain serially, and never regress from done.
    Each obligation is its own message, never batched, and each send is its own attempt.
    Only a proven pre-acceptance rejection is retried automatically; any other failure,
    with or without a crash, including a crash before the row id is stored, is reconciled
    and otherwise `uncertain` with Resend and Dismiss; one Resend click makes exactly one new
    attempt through the real drain, across restarts and replays `[I-1]`, while earlier ones
    keep being reconciled. A voided wake is never sent again, and an override sends one void
    notice whatever the wake's state `[D-3]` `[D-9]` `[E-4]` `[E-5]` `[F-1]` `[F-2]` `[F-4]`
    `[G-1]` `[G-2]`. The
    `bb home` CLI and `autarch decide` return exit codes 2, 3, 4 and 5 as real processes, a
    thread conflict between `--thread` and `BB_THREAD_ID` exits 2 `[D-16]`, a found recovery
    read with the same identity and scope exits 0, and one with another identity exits 2
    `[E-6]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/wakes.test.ts __tests__/configure.test.ts __tests__/cli.test.ts __tests__/delegation.test.ts && cd ../.. && go test -race ./internal/homeask/ ./cmd/autarch/ -run 'Filer|Decide|Version'
    ```
11. Mycroft's suggestions and out-of-allowlist dispatches file ruling-only decisions once per
    suggestion through the production constructor, Mycroft never schedules a decision bead,
    and `DecisionQueue` keeps no private list.
    ```check
    go test -race ./internal/mycroft/... ./cmd/mycroft/ && ! grep -nE '^[[:space:]]+decisions[[:space:]]+\[\]PendingDecision' internal/mycroft/escalate/escalate.go
    ```
12. The bb plugin typechecks, all its fake-SDK tests pass, and it builds.
    ```check
    cd integrations/bb-plugin-autarch && npm run typecheck && npm test && bb plugin build
    ```
13. The fake-mode harness passes exactly its eleven scenarios in one run at this clean commit
    and tree, under an externally generated run id, with evidence that validates against each
    scenario's typed schema and is written outside the checkout; the checker rejects every
    rejection fixture, including a stale same-commit run and malformed evidence `[D-15]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/check-e2e.test.ts && rid=$(node -e 'console.log(crypto.randomUUID())') && out=$(mktemp -d)/e2e.jsonl && npm run e2e -- --mode fake --run-id "$rid" --out "$out" && node scripts/check-e2e.mjs "$out" --mode fake --run-id "$rid" --product-commit "$(git rev-parse HEAD)" --scenarios answer-instruction,crash-after-pick,delegated-override,file-retry,file-unknown,not-ready-at-start,pick-retry,queued-then-archived,ruling-file-blocked,supersede,two-writers
    ```
14. A fresh real-bb run, started by this check with a run id it generates itself, against
    the permitted isolated bb server named by `HOME_E2E_BB` (Task 1.11, or mk's server after
    G-3), exports HEAD and builds `autarch`, installs the export by path and reloads it,
    starts its own `autarch serve` from that build with a scratch token and a scratch
    discovery parent and points the plugin at both, resolves the scratch project through an
    authenticated request, verifies the loaded plugin source, the rendered app's source, the
    connected daemon and the `autarch` binary against the build, passes exactly its four
    scenarios at HEAD with clean evidence written outside the checkout, and archives every
    scratch thread it recorded. The checker rejects a stale plugin, stale frontend and stale
    daemon. The committed manifest and report are historical and not read. Without
    `HOME_E2E_BB` the check fails: unmet, not skipped `[D-14]` `[D-15]` `[E-11]` `[E-12]`
    `[F-7]` `[G-7]` `[H-6]` `[I-4]`.
    ```check
    test -n "$HOME_E2E_BB" && cd integrations/bb-plugin-autarch && npm test -- __tests__/build-identity.test.ts && rid=$(node -e 'console.log(crypto.randomUUID())') && d=$(mktemp -d) && b="$d/build.json" && e="$d/e2e.jsonl" && node scripts/build-identity.mjs --out "$b" && npm run e2e -- --mode real-bb --build "$b" --install --run-id "$rid" --out "$e" && node scripts/check-e2e.mjs "$e" --mode real-bb --run-id "$rid" --product-commit "$(git rev-parse HEAD)" --build "$b" --scenarios answer-instruction,ask-cli-proxy,queued-then-archived,vizier-chat && jq -se '[.[].evidence.threads[]] as $t | ($t|length) > 0 and all(.[]; .evidence.cleanup.archived == .evidence.threads)' "$e"
    ```
15. Structured asks, delegation and the Home tab hold: runbook, machine lane and mentions;
    machine blockers progress, resolve and withdraw, each notice an outbox row that a later
    resolve or withdraw voids or follows, an owner notice asks the owner to confirm the
    blocker is open (a resolve may overtake an uncertain one), and a completion wakes every
    recipient once `[H-5]`; `rule` from a caller claiming the vizier thread, including a
    forged matching `BB_THREAD_ID`, is held to every delegation limit (reversible, listed
    project, daily cap, no approval spec), woken with the non-authorization line and pinned;
    a direct `pick` RPC from any caller is recorded `by: mk` without delegation limits,
    which the test records as the admitted attribution bypass; authenticated human/vizier
    separation is G-15; delegation settings change only through the panel, pinned, and suspend
    delegation until seen; an override voids the old wake, sends one void notice to the
    asker in every wake state, keeps a delivered wake `done`, and makes the replacement and
    its successors non-delegable with their mentions kept, the void notice names the
    replacement and never overrides an answer already received, and every later answer
    names the overridden vizier ruling `[H-3]` `[H-4]`; catch-up ranking and pinning;
    the panel stack, `markAllSeen` over a rendered snapshot, and a read marker that needs the
    item expanded, visible and focused `[D-1]` `[D-2]` `[D-11]` `[D-12]` `[E-2]` `[E-13]`
    `[F-6]` `[F-8]` `[G-2]` `[G-4]`.
    ```check
    cd integrations/bb-plugin-autarch && npm test -- __tests__/asks.test.ts __tests__/delegation.test.ts __tests__/catchup.test.ts __tests__/ui.test.tsx
    ```
16. mk walks autarch-07 on the installed Home tab, including a delegated ruling and an
    override (G-6). This is recorded by mk, and no command can substitute for it.
