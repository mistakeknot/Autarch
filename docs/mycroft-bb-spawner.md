# Mycroft card workers as bb threads (bead mk-omogp.1)

## What changed

- `internal/mycroft/spawn` no longer opens tmux sessions. The tmux path is **removed**, not kept behind a flag:
  nothing else used it, and a second spawn path would be a second place for a seat to be chosen.
- `BBSpawner` implements the existing `scheduler.AgentSpawner` interface, so tests still stub it. `Spawn`:
  1. resolves the seat from Clavain routing (`ic --json route dispatch --policy=<routing.yaml> --role=routine-execution`);
  2. runs `bb thread create --project --environment --provider --model --reasoning-level --title --prompt-file`;
  3. runs `bb tasks attach <card> --thread <id>` (the card is required and is read first, its description goes into the brief; if the attach or the receipt write fails the thread is stopped and the error says whether the stop worked);
  4. appends a receipt line (role, profile, model, effort, policy hash, thread, bead, card) to `spawn-receipts.jsonl`
     and puts the same facts in the dispatch log reason.
- Mycroft holds **no seat table**. The only map is backend name to bb provider id (`claude` to `claude-code`,
  `codex` to `codex`), which is transport naming. Role defaults to `routine-execution`; routing returns Sonnet 5.5 at high
  today. Elevated work would use `deep-execution`; a frontier-required answer is refused here.
- **Fail closed**: an unset policy, an `ic` error, an unreadable or incomplete answer, a wrong role, a frontier-required
  answer or an unknown backend returns an error before any `bb` call. The caller logs the failure; nothing starts.
- **T1 only**: the patrol loop refuses tier T2/T3 with this spawner. At T1 Mycroft only suggests. `mycroft spawn <bead>
  --card KEY --title T` prints the thread title and routed seat and starts nothing; `--approve` starts it.
- Configuration: `AUTARCH_BB_PROJECT`, `AUTARCH_BB_ENVIRONMENT`, and `CLAVAIN_ROUTING_POLICY` or `CLAVAIN_SELECTED_ROOT`.
- `fleet.BeadView` gains an optional `Card` field (the task-board key).

`bb tasks dispatch --preset` was considered and not used: a preset stores a model and effort inside bb, which is a second seat table.

## What Aleph core would need to provide for T2 (not built here)

**Atomic card claim.** `bb tasks update` has `--status` and no compare-and-set, no assignee and no claim; `bb tasks attach`
adds a thread without checking that none is attached. Two Mycroft instances, or Mycroft and a human, can both start a worker
on one card. Needed: `bb tasks claim <key> --thread <id>` that succeeds for exactly one caller (conditional on status and no
current claimant), or an `--expect-status` guard on `update`. Until then Mycroft can only check `bb tasks threads <key>` and
race.

**Quota headroom.** `bb pool status --json` (and `account list`) already carry, per account: provider, enabled, priority,
`limitWindows` (slot, window length, utilization, reset time, observation time and source; filled in for Codex accounts
too), the Claude five-hour and seven-day utilization and reset fields, per-family weekly figures (only `fable` was filled
in at the time of writing), and pool-level `inFlight` and `accepting`. The status output seen on the host when this was
written also showed `activeAccountIds` and a per-account `usageRestriction`; the schema is the host's, so check it
before depending on those two. That is enough to read headroom per account. Missing:
- The Sonnet, Opus and Haiku family figures were null; the seat Mycroft needs (Sonnet) has no family number of its own.
- No pool-level answer ("headroom for provider P, family F, over the next N hours") and no reservation, so two dispatchers
  read the same headroom and both spend it. Needed: a headroom endpoint with a reserve/release call.
- Utilization is observed, not live (`observedAt` can be minutes old); a freshness bound is needed.
- The quota budget for a backlog run needs a stated policy (what fraction of the weekly window the runner may spend),
  which is a decision for mk and the vizier, not core.
