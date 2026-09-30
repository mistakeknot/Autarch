# Scenario harness

Fake mode (`npm run e2e -- --mode fake --run-id <uuid> --out <file>`) runs eleven scenarios
against the real plugin wiring, a real database file, real project directories and the real
`autarch decide file` binary. Only bb is faked (CLI proxy, events, thread SDK).
`--out` must be outside the git worktree.

Accept the output with `node scripts/check-e2e.mjs <file> --mode fake --run-id <uuid>
--product-commit <sha> --scenarios <list>`. The checker rejects partial, duplicated, stale,
dirty or mixed-run output.

Real-bb mode drives an ISOLATED bb server that the operator started on its own data dir, port
and HOME (never a live machine). Name it in the environment: `HOME_E2E_BB` (loopback URL),
`HOME_E2E_BB_HOST_PORT`, `HOME_E2E_BB_DATA`, optionally `HOME_E2E_BB_HOME` and `HOME_E2E_BB_CLI`.
`e2e/real.ts` refuses the ambient server, a non-loopback host or a `.bb-machines` data dir, and
runs every bb call with a scrubbed environment. Run it from a clean checkout of the commit:

    node scripts/build-identity.mjs --out build.json
    npm run e2e -- --mode real-bb --build build.json --install --run-id "$(uuidgen)" --out <file outside the tree>

The harness installs the build's plugin by path, starts its own `autarch serve`, verifies the loaded
plugin, app, daemon and autarch identities, runs `answer-instruction`, `queued-then-archived`,
`ask-cli-proxy` and `vizier-chat` (Playwright, chromium), archives exactly the scratch threads it
recorded and writes `<out>.cleanup.json`. Scratch threads take one real turn that fails at once with
"Not logged in" (no credentials in the isolated HOME), which is what gives bb an execution model to
wake. `queued-then-archived` needs an active turn, so without model credentials it fails honestly.
