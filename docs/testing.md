# Affected checks for PR iterations

PR-iteration runs may use `bash scripts/affected-checks.sh BASE HEAD --print`
to inspect the plan, then `--run` to execute it. Python 3 and the existing Go
and plugin toolchains are required; the helper never installs dependencies.
The diff uses `BASE...HEAD`, while dependency analysis and checks use the
current checkout. Check out the revision you intend to test before using it.
Historical ranges can also be used as selection inputs on a fixed checkout
for timing comparisons; those measurements do not test the historical tree.

Go source changes select their packages and reverse dependencies, including
imports used only by tests. Go builds always cover `./...` for non-doc runs.
Plugin source changes keep whole-program typechecking and use Vitest 3's
`related --run`; changed tests also run explicitly. Documentation-only changes
run nothing. Shared inputs, instruction files, configs, migrations, fixtures,
data, scripts, CI files, and anything unmapped fall back to full checks.
The plan prints the fallback reason. Runs stop on the first failed command.

The full suite (`go test -race ./...`, plugin tsc and full vitest) is still
required at the exact head sent for merge. Existing CI checks are unchanged.
Affected runs are never qualification evidence and do not replace the two
fresh-guest full-suite passes at one exact commit.

Run the mapping and execution fixtures with `bash scripts/test_affected_checks.sh`.
They use append-only temporary Git repositories in a git-ignored scratch directory
in the checkout (`build/affected-checks`), or `AFFECTED_CHECKS_SCRATCH`.
They cover reverse dependencies, test-only imports, fallback cases,
docs-only changes, plugin selection, explicit test runs and failure propagation.
