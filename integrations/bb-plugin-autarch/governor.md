# Quota governor

The plugin includes a ten minute, abortable quota governor service (bead
mk-z0lht.1). Installation defaults `quota_governor_mode` to `off`: it does not
sample quota, create governor tables, edit policy, set limits or write inboxes.
Citadel's Settings panel shows its status and updates through realtime events.

Configure the governed host with the `quota_governor_host` plugin setting;
its default is empty. Resolution uses the non-empty setting first, then
`QUOTA_GOVERNOR_HOST`, then `bb_host` in the JSON file named by
`QUOTA_GOVERNOR_CONFIG`. Empty or whitespace-only settings fall back to the
environment or file. A missing host leaves the service inert; the local hostname
must match. Plugin settings are read on each tick, so no server environment
change is needed. JSON overrides otherwise retain the script's shallow merge
semantics and policy defaults. State and floor default to
`$HOME/.local/state/quota-governor`, operator policy to
`$HOME/.config/aleph-orchestration/operator.toml`, and inbox storage to
`$BB_DATA_DIR/thread-storage` when set. Otherwise it discovers the real machine
thread-storage directories through the same `docRoots` helper used by the document
reader. Discovery runs only for live notification preflight after the mode and
host checks; off, shadow and other hosts do not need notification storage.
Missing or ambiguous roots require an explicit `storage_root`. Before policy is
applied, preflight requires a storage directory. Missing, malformed or unwritable
recipients are skipped, logged and shown in the panel. Zero writable recipients
is a warning and policy still applies, matching the script. An unavailable or
ambiguous storage root blocks the transition without applying or rolling back
operator policy or host limits.
`QUOTA_GOVERNOR_PROJECT_ROOT` overrides the protected card's project root;
its default is the Sylveste directory beneath the user's projects directory.
The card destination uses the same precedence: the non-empty
`quota_governor_card_project` plugin setting (default empty), then
`QUOTA_GOVERNOR_CARD_PROJECT`, then `card_project` in the config file.
An unset destination holds protected work and
reports the error without writing a card file. Malformed override JSON or a
non-object override is an error, never a fallback to policy defaults.

The supported SDK `bb.sdk.plugins.callRpc` handles local routing and
authentication. `account-pool`'s `account.list` returns weekly utilization,
reset and observation times. `concurrency-limit`'s `getConfiguration` and
`setConfiguration` preserve the global limit and every other host override.
No server URL, auth token or CLI fallback is necessary for these calls.

Use the plugin setting `quota_governor_mode` to stage cutover:

1. Run `shadow` for at least 24 hours and compare every level transition with
   the script in the panel. Shadow samples and persists simulated decisions
   only in the plugin's SQLite store. It reads the script state and floor,
   and never writes operator policy, concurrency limits, cards or inboxes.
2. Stop the legacy timer through the normal operator process.
3. Select `live`. An active or indeterminate timer or service state refuses live work.
   The first live run imports the script's state, including its protected card
   and pending reconciliation. Shadow state is separate from live state.
4. Retire the old timer and service in a separate operational change after
   observing live operation. This change does not stop or remove either unit.

Recent pace needs two hours of samples inside a three hour window. Startup
holds when pace is unknown. Samples use observation time, exclude expired
weekly windows and never combine opposite sides of a reset. An active
`floor.json` prevents loosening below its level and clears good readings at
that floor; expiry restores normal hysteresis. Floors that cut protected work
still require a card. Protected answers come from Home's `picks` table through
the card's task id and generation one decision id. Only picks attributed to the
human operator on the Home or overlay surface count; delegated, automatic and
CLI picks cannot authorize protected-work cuts. Declines hold twelve hours,
then renew the card. Card filing supplies a nonempty title and the vizier
thread's caller identity.

Operator changes keep backups, use bounded `execFile` calls, restore the file
and reactivate the old generation after an orchestration failure. Transition
failure rolls back operator and host limits; failed rollback persists a
pending reconciliation. Notices are appended after the new level is committed.
A compatibility `throttle.json` read or write failure is recorded and shown but
does not suppress those notices; the next live tick retries the export.
Delivery failures are logged and shown while delivery
continues to other recipients; they never roll back the committed level, so a
partial append cannot strand notices for a rolled-back policy. Preflight cannot
prevent later directory or permission changes. Pool failures do not refresh throttle freshness.
`throttle.json` retains the script's fields and is replaced atomically in
live mode. Coordinator/steward and vizier notices retain the throttle/lift
format. Inbox and card prose match the script except that its private ruling
identifier in the Undersketch path is replaced by the word "protected"; the
project root is resolved at runtime instead of embedding a private path.

Golden fixtures contain 1,270 observations from all 50 upstream Python tests,
216 deterministic `main(--dry-run --json)` calls (180 use recent pace;
144 retain non-empty history and 36 stale cases load empty history),
237 captured `step()` decisions, and 53 Python floor parsing
observations. Step replay checks card decisions and resulting policy, card and
pending-reconciliation state against the same recorded boundary successes and
failures. Human-facing apply diagnostics differ from the script and are checked
for failure status rather than exact text. The fixture records the
source SHA-256. Regenerate with the current source and its test file:

```sh
python3 scripts/governor-golden.py "$GOVERNOR_SCRIPT" "$GOVERNOR_TEST_FILE" __tests__/fixtures/governor-golden.json
```

Tests inject RPC, process and filesystem interfaces or use in-memory SQLite;
they do not change live policy, concurrency limits, inboxes or throttle files.
