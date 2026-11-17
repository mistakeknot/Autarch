---
name: autarch-verify
description: Run Autarch's advisory exact-head journey verifier for a base and head, with isolated offline builds and external evidence.
---

# Autarch verify

Requires Linux and `bwrap`. The verifier package is excluded from other hosts'
repository builds.

Source of requirements: bead mk-05q0j.3. This pilot is advisory. It grants no
merge, deployment or publication authority.

Use a clean, detached checkout of a pinned verifier commit. Run the driver there,
separately from the product checkout. It creates a detached product worktree at
the requested head; historical heads use their own harness, without overlays.

Before a heavy run, read the fleet's ops-hygiene heavy-run instructions, check
CPU pressure and ensure another suite is not running for this repository.
The driver serializes verifier runs across clones with flock and runs journeys
sequentially. Use nice 19, idle I/O and at most four workers.

Build the standard-library driver offline inside a network namespace, with an
owned temporary HOME and explicit Go cache. Keep its executable outside source
checkouts. Run it from the detached verifier checkout:

```sh
nice -n 19 ionice -c3 "$VERIFY_BIN" \
  --verifier-commit "$VERIFIER_SHA" --base "$BASE_SHA" --head "$HEAD_SHA" \
  --cache-root "$OFFLINE_CACHE" --app-copy "$OWNED_BB_APP" \
  --toolchain-seed "$BB_TOOLCHAIN_SEED" --cache-state cold --pr "$PR_URL" \
  --out "$EVIDENCE_DIR"
```

Preprovision these offline fixtures outside checkouts and live data directories:

- `node/<lockfile-sha256>-<node-version>-<ABI>-<platform>-<arch>`: npm cache.
- `gomod`: Go module cache; `go/<sha256-of-go-version-output>`: Go build cache.
- `playwright/<version>`: the browser cache for the head's Playwright version.
- A self-contained copy of the installed bb app, plus its offline
  `toolchain-<pins>` seed. The harness launches this copy with owned data and HOME
  inside `bwrap --unshare-net`. No operator server target is accepted.

Missing fixtures, unavailable namespaces and failed setup mean INCOMPLETE.
Never fetch dependencies to hide a cache miss. Never run the live bb CLI,
Home preview/deployment scripts, or point the verifier at live Home data.
All children use scrubbed environments. Output paths, including symlink
escapes, machine data directories and paths within a checkout, are refused.

`--flows` adds journeys. Required deferred journeys remain INCOMPLETE. Partial
map rows leave NO-FLOW coverage even if their journeys pass. The precedence is
FAIL, INCOMPLETE, VERIFIER-CHANGED, NO-FLOW, PASS. A product assertion timeout
after setup is FAIL; an outer journey deadline is INCOMPLETE. An edited verifier
or product harness cannot yield overall PASS.

Read both `report.json` and `report.md`. Evidence and the JSONL ledger stay
outside the repository. The ledger defaults to
`$XDG_STATE_HOME/autarch-verify/ledger.jsonl` (or the standard HOME state directory).
Supply `--suite-results` with JSON `{ "head": "<full SHA>", "results": { ... } }`
to record fresh same-head suite outcomes and skips; absent results stay unknown.
Repeat with `--cache-state warm` and a fresh output directory. Times include
setup and teardown; a label alone does not establish that caches were warm.

Run `go test -race ./scripts/verify` for driver tests. To exercise the three real
browser fault oracles, set `AUTARCH_VERIFY_ORACLE_FIXTURES` to an external JSON
file containing `cache`, `app` and `seed`, then run
`go test -race ./scripts/verify -run TestPickClickPlantedFaults -timeout 25m`.
The suite first requires a passing control, then plants each fault in a
standalone scratch repository and requires FAIL at the expected assertion.
An oracle skip leaves acceptance unverified. Run the full Go race suite and the
plugin typecheck, lint and vitest at the reported head as well.

A catch pair is comparable only when verifier manifest hashes and product
e2e tree hashes match. Historical heads without the scenario are not exercisable.
Passing regression checks never count as incremental catches. Keep cold, warm,
retry, incomplete and verifier-changed observations separate.
