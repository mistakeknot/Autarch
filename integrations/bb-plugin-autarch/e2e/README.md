# Scenario harness

Fake mode (`npm run e2e -- --mode fake --run-id <uuid> --out <file>`) runs eleven scenarios
against the real plugin wiring, a real database file, real project directories and the real
`autarch decide file` binary. Only bb is faked (CLI proxy, events, thread SDK).
`--out` must be outside the git worktree.

Accept the output with `node scripts/check-e2e.mjs <file> --mode fake --run-id <uuid>
--product-commit <sha> --scenarios <list>`. The checker rejects partial, duplicated, stale,
dirty or mixed-run output.

Real-bb mode (loaded-identity preflight, Playwright, install into an isolated bb server) is
Task 1.11; `e2e/preflight.ts` and `scripts/build-identity.mjs` are its groundwork.
