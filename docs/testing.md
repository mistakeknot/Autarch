# Affected checks for PR iterations

PR iterations may use `bash scripts/affected-checks.sh BASE HEAD --print` to
inspect the plan, then `--run` to execute it. The checkout must be clean,
including untracked files, and checked-out HEAD must equal the HEAD argument.
A mismatch is refused with exit 2; historical ranges cannot be timed against
a different checkout. Python 3, Linux nice/ionice, Go and the installed plugin
toolchain are required. The helper never installs dependencies.

Go changes select their packages and reverse dependencies, including test-only
imports. Only metadata discovery (`go list`) disables VCS stamping. Builds
always cover `./...` for non-doc runs. Any Go change also runs full Vitest:
the build-identity tests compile and inspect the real Go binary.

Plugin changes retain whole-program tsc and `vitest related --run`, plus an
explicit run of changed test files. A metadata-only Vitest 3 query checks each
changed source separately. An empty or failed query selects full Vitest and
prints why, so a tested neighbor cannot hide an untested source. The query uses
the pinned Vitest specification API; an unavailable API fails safely to full.

## Tests outside import graphs

[scripts/affected-readers.json](../scripts/affected-readers.json) is the explicit
reader manifest, always run for any non-documentation change. It deliberately
over-includes direct filesystem/process readers rather than assume a read is
only a fixture. It includes:

- Go's root rune-width scan, docs/wiki's Go/TS/Markdown figure checks,
  internal/door's binary build, scripts/verify, tests/orchcompat and every Go
  package found by the filesystem/build-reader audit.
- All static-scan consumers (no-exec, write-scope, seams, rigexec), docs.test,
  build-identity, UI source readers, schema/vector readers and script/deploy
  consumers. Shared deploy fixtures copy and execute repository sources.

The audit searches tracked Go tests for os.ReadFile, os.Open, filepath.Walk
and exec.Command("go"), and Vitest tests for readFileSync, readdirSync,
execFileSync, spawnSync, copyFileSync, cpSync and static-scan. A new direct
reader missing from the manifest selects full checks. Review indirect helper
reads when maintaining this list; import graphs alone do not establish coverage.

Shared inputs, configs, migrations, data, fixtures, scripts, CI files, unknown
files, deleted/renamed files and unmappable Go packages select full checks.
Markdown is exempt only when explicitly audited in the manifest. In particular,
docs/wiki/src pages and the plugin README and overview are test inputs and
select full checks, as do other Markdown files without an exemption.

## Merge checks and fixtures

The full suite (`go test -race ./...`, plugin tsc and full vitest) is still
required at the exact head sent for merge. Existing CI checks are unchanged.
Affected runs are never qualification evidence and do not replace the two
fresh-guest full-suite passes at one exact commit. Runs stop at the first failure.

Run `bash scripts/test_affected_checks.sh` for append-only fixture repositories
in the checkout's ignored `build/affected-checks`, or `AFFECTED_CHECKS_SCRATCH`.
They cover package graphs, reader coverage, fallback rules, orphan selection,
actual Vitest argument parsing, execution/failure propagation, and checkout
refusals. Only each fixture's own directory is removed after use.
