# Home e2e, real-bb run on an isolated bb server (Task 1.11)

Historical record of one run. It is not acceptance and authorizes nothing (gates G-3, G-6 stand:
the real trial on mk's server is not part of this run).

- Product commit: `e5cd4688f57049b6441b1708880c4e6c60b14546` (clean detached worktree of that commit; tree `9201f2c2bb18c9f664e964edcb2fc3f18ac52241`, dirty:false)
- Run id: `3601e12a-8a66-47af-b930-5a2112e7f136`
- Evidence: `/home/mk/.autarch/home-e2e/e5cd4688f57049b6441b1708880c4e6c60b14546/3601e12a-8a66-47af-b930-5a2112e7f136.jsonl` (sha256 in `2026-09-29-home-e2e-real-bb.json`), cleanup record `/home/mk/.autarch/home-e2e/e5cd4688f57049b6441b1708880c4e6c60b14546/3601e12a-8a66-47af-b930-5a2112e7f136.jsonl.cleanup.json`
- Isolated server: own data dir, port and HOME under `/tmp/autarch-e2e-iso.*`, loopback only. It was torn down
  afterwards. The live autarch.getbb.app server, its data and the hub Dolt were not touched. No API keys
  were used; scratch threads run one turn that fails with "Not logged in" (no credentials in the isolated HOME).

## Commands

    node scripts/build-identity.mjs --out /tmp/e2e-run-build.json --scratch /tmp/e2e-run-build
    HOME_E2E_BB=http://127.0.0.1:33143 HOME_E2E_BB_HOST_PORT=37761 HOME_E2E_BB_DATA=$ISO/data HOME_E2E_BB_HOME=$ISO/home \
      HOME_E2E_BB_CLI=/home/mk/.bb-machines/autarch.getbb.app/npm/bin/bb \
      npm run e2e -- --mode real-bb --build /tmp/e2e-run-build.json --install --run-id 3601e12a-8a66-47af-b930-5a2112e7f136 --out /home/mk/.autarch/home-e2e/e5cd4688f57049b6441b1708880c4e6c60b14546/3601e12a-8a66-47af-b930-5a2112e7f136.jsonl
    node scripts/check-e2e.mjs /home/mk/.autarch/home-e2e/e5cd4688f57049b6441b1708880c4e6c60b14546/3601e12a-8a66-47af-b930-5a2112e7f136.jsonl --mode real-bb --run-id 3601e12a-8a66-47af-b930-5a2112e7f136 --product-commit e5cd4688f57049b6441b1708880c4e6c60b14546 \
      --build /tmp/e2e-run-build.json --scenarios answer-instruction,ask-cli-proxy,queued-then-archived,vizier-chat

Result: all four scenarios PASS; `check-e2e: ok (4 scenarios, run 3601e12a-8a66-47af-b930-5a2112e7f136)`, exit 0. Cleanup archived the 5 recorded
threads, 0 failed (ids in the cleanup record).

## Results

| scenario | result | threads | evidence (threads omitted) |
|---|---|---|---|
| answer-instruction | PASS | 1 | `{"decision":"dec_aa1a310520e940c8","pick_id":"pick-dec_aa1a310520e940c8","ruling_sha256":"...","wake_state":"done","wake_count":1,"feed_line":"ruled 2026-09-30T07:00Z \"Collapse per project\" (dec_aa1a310520e940c8)"}` |
| queued-then-archived | PASS | 1 | `{"decision":"dec_4f17a1d7ce624a44","wake_state":"undeliverable","listed":true}` |
| ask-cli-proxy | PASS | 2 | `{"decision":"dec_949827762d4e4e32","env_absent_exit":0,"env_conflict_exit":2}` |
| vizier-chat | PASS | 1 | `{"message_id":"evt_6t9ktwr96a","screenshot_sha256":"..."}` |

Caveats: `feed_line` is computed by the real FeedCaches over the isolated server's real database, read-only; it was
not observed rendered inside a thread. `queued-then-archived` gets its queued wake by racing a `bb thread tell`
turn (which fails fast with no credentials) against the pick; the driver retries up to 6 fresh threads and fails
if no wake queues (it queued in this run).

## Failures found and fixed on the way

1. RPC results contained `undefined` and did not serialize (fixed in 7fa10df, `server.ts` jsonSafe).
2. `threads.send` input must be an array of text parts, not a string (fixed in 9741826, `wakes.ts`).
3. First real run (commit 128d9c8, run 351bc6c1-ca6f-45a1-a946-855ed4d00cf5): 1 PASS, 3 FAIL. answer-instruction
   picked while the scratch thread's turn was active (wake queued, not sent); queued-then-archived saw the wake sent;
   ask-cli-proxy expected exit 0 for a conflicting BB_THREAD_ID.
   - Checker schema corrected: `autarch decide file` exits 2 on a conflicting BB_THREAD_ID by design (decide.go,
     `TestDecideThreadConflictExitsTwoAndRunsNothing`), so `env_conflict_exit` is now eq(2) (48a15c2, with a test).
   - Driver: settle threads before ask/pick, race a queued wake, scope decide-file rows to the run (1b77768, e5cd468).
4. Second run (commit 1b77768, run 5befe35b-c7ac-4839-adf6-1bf1c437a556): settle read the wrong JSON path and timed
   out on two scenarios; ask-cli-proxy matched rows from a previous run. Fixed in e5cd468. Run 3 above passed.

## Follow-up beads (children of mk-okek): FILED under mk-okek

- mk-okek.8: Clavain Stop hook and filing helper
- mk-okek.9: signals clients reach the consolidated broker
- mk-okek.10: `bd create --id` overwrites an existing bead (bd upstream)
- mk-okek.11: optional hub copy
- mk-okek.12: approvals in Aleph core
- mk-okek.13: G-15 host-attested caller for plugin CLI and RPC
- mk-okek.14: G-16 core approval authentication and audit contract
- mk-okek.15: rewrite of the stale "bb autarch" todo docs. This was in fact already rewritten in
  9bd3c2a (`bb home` replaces the removed todo CLI), so mk's coordinator may close .15.
