# Mycroft worker spawner

Mycroft starts workers selected by a coordinator. It has no patrol loop,
work selector, autonomy tiers, suggestion cards or TUI tab.

## Spawn a worker

Build with `go build -o mycroft ./cmd/mycroft`, then use:

```sh
mycroft spawn BEAD --card CARD --title 'Work title'
mycroft spawn BEAD --card CARD --title 'Work title' --approve
```

The first command prints the thread title and routed seat without starting a
worker. The second starts a bb thread and attaches the existing card. Set
`AUTARCH_BB_PROJECT` and `AUTARCH_BB_ENVIRONMENT` to the existing bb project and
environment. Set `CLAVAIN_ROUTING_POLICY`, or `CLAVAIN_SELECTED_ROOT`, for routing.

`internal/mycroft/spawn` resolves the `routine-execution` role through Clavain
routing at dispatch time. Mycroft holds no model or effort table. Unknown
backends, unresolved seats, incomplete routing answers and frontier-required
answers fail before any bb call.

The spawner reads the card description into the worker brief, calls
`bb thread create`, attaches the card with `bb tasks attach`, and appends the
seat and worker facts to `spawn-receipts.jsonl`. A failed attachment or receipt
write stops the thread; the error reports whether stopping succeeded. An
uncertain create result requires reconciliation before retrying.

`mycroft spawn --approve` opens `decisions.db` before routing or starting
anything. It records the approval in `dispatch_log` with action `suggest`
(the existing stored value), outcome `accepted`, and the seat, thread, bead
and card details. Unresolved plans and failed starts are logged with outcome
`failure`. A later log-write error names the running thread.

Data files default to `.autarch/mycroft/`; `MYCROFT_DATA_DIR` overrides that
directory. The SQLite schema is unchanged, including unused legacy tables.
Coordinators remain responsible for work selection, concurrency and approval.

## Run a pre-dispatch check

`internal/mycroft/predispatch.Run` exposes the former scheduler check independently.
A coordinator can run it through the CLI before choosing whether to spawn:

```sh
mycroft pre-dispatch --bead BEAD --agent AGENT
```

Configure `.autarch/mycroft/config.yaml` (or the overridden data directory):

```yaml
pre_dispatch_check:
  command: [./check-work, --ready]
  timeout: 30s
```

The command executes directly as argv, without a shell, inheriting the caller's
working directory and environment. `MYCROFT_BEAD_ID` and `MYCROFT_AGENT` are
replaced with the proposed assignment. A missing config or empty command
succeeds. The timeout defaults to 30 seconds, including nonpositive configured
values. Exit zero succeeds; nonzero exit, timeout or start failure returns a
nonzero CLI exit and a reason bounded to 240 runes. The check starts no worker,
files no cards, and writes no dispatch outcome. It is an explicit separate
call; `spawn --approve` retains its existing behavior.

Timeout cancellation uses Go's command context to kill the check process.
The extracted implementation has no process-group kill; callers should use
checks that do not leave background descendants.
