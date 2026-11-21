#!/usr/bin/env bash
# All fixture history is append-only. Scratch artifacts are ignored before planning.
set -euo pipefail
script=$(realpath "$(dirname "$0")/affected-checks.sh")
root=$(git rev-parse --show-toplevel)
scratch_root=${AFFECTED_CHECKS_SCRATCH:-$root/build/affected-checks}
mkdir -p "$scratch_root"
scratch=$(mktemp -d "$scratch_root/affected-fixture.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
cd "$scratch"
git init -q
git config user.name Fixture
git config user.email fixture
mkdir -p .checks/bin leaf consumer other testconsumer nested integrations/bb-plugin-autarch/node_modules/.bin
printf '.checks/\nnode_modules/\n' > .gitignore
printf 'module fixture\n\ngo 1.25\n' > go.mod
printf 'package leaf\n' > leaf/leaf.go
printf 'package consumer\nimport _ "fixture/leaf"\n' > consumer/consumer.go
printf 'package other\n' > other/other.go
printf 'package testconsumer\n' > testconsumer/main.go
printf 'package testconsumer_test\nimport _ "fixture/leaf"\n' > testconsumer/main_test.go
printf 'module nested\n\ngo 1.25\n' > nested/go.mod
printf 'package nested\n' > nested/main.go
printf 'export const value = 1;\n' > integrations/bb-plugin-autarch/value.ts
printf 'export {};\n' > integrations/bb-plugin-autarch/value.test.ts
printf 'package leaf\n' > leaf/deleted.go
printf 'package leaf\n// rename sentinel\n' > leaf/old.go
printf 'Notes\n' > README.md
export AFFECTED_LOG="$scratch/.checks/commands.log"
export AFFECTED_REAL_NODE=$(command -v node)
export AFFECTED_REAL_GO=$(command -v go)
cat > .checks/bin/go <<'SH'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$AFFECTED_LOG"
if [[ $1 == list ]]; then exec "$AFFECTED_REAL_GO" "$@"; fi
if [[ ${AFFECTED_FAIL:-} == yes && $1 == test ]]; then exit 7; fi
SH
cat > .checks/bin/node <<'SH'
#!/usr/bin/env bash
if [[ $1 == */affected-related.mjs ]]; then
  if [[ ${AFFECTED_NO_RELATED:-} == yes ]]; then echo '[]'; else echo '["__tests__/docs.test.ts","value.test.ts","related.test.ts","__tests__/docs.test.ts"]'; fi
else
  exec "$AFFECTED_REAL_NODE" "$@"
fi
SH
for bin in tsc vitest; do
  cat > "integrations/bb-plugin-autarch/node_modules/.bin/$bin" <<'SH'
#!/usr/bin/env bash
printf '%s %s\n' "${0##*/}" "$*" >> "$AFFECTED_LOG"
SH
  chmod +x "integrations/bb-plugin-autarch/node_modules/.bin/$bin"
done
chmod +x .checks/bin/{go,node}
export PATH="$scratch/.checks/bin:$PATH"
git add .
git commit -qm initial
cases=0
verify() {
  plan=$(bash "$script" HEAD~1 HEAD --print)
  [[ $plan == *"$1"* ]] || { printf '%s\n' "$plan"; exit 1; }
  cases=$((cases + 1))
}
check() {
  mkdir -p "$(dirname "$1")"
  printf '\n' >> "$1"
  git add "$1"
  git commit -qm change
  verify "$2"
}
check leaf/leaf.go 'mode: affected'
[[ $plan == *' fixture/consumer fixture/leaf fixture/testconsumer'* ]]
[[ $plan != *'fixture/other'* ]]
[[ $plan == *'./docs/wiki'* && $plan == *'./internal/door'* && $plan == *'./tests/orchcompat'* ]]
[[ $plan == *'Go changes require cross-language binary checks'* ]]
grep -q 'go list -buildvcs=false' "$AFFECTED_LOG"
check integrations/bb-plugin-autarch/value.ts 'vitest run --maxWorkers=2'
go_plan=$(printf '%s\n' "$plan" | sed -n '/^Go packages:/p')
[[ $go_plan == 'Go packages: ./docs/wiki ./internal/homeask ./scripts/verify' ]]
for reader in no-exec.test.ts seams.test.ts write-scope.test.ts rigexec.test.ts docs.test.ts build-identity.test.ts design-roles.test.tsx; do
  [[ $plan == *"$reader"* ]]
done
check integrations/bb-plugin-autarch/value.test.ts './value.test.ts'
bash "$script" HEAD~1 HEAD --run > /dev/null
grep -q 'tsc -p tsconfig.json' "$AFFECTED_LOG"
[[ $(grep -c '^vitest run ' "$AFFECTED_LOG") == 1 ]]
# Test the real Vitest parser against every generated test command.
printf '%s\n' "$plan" > .checks/plan.txt
python3 - "$scratch" <<'PYTEST'
import json
from pathlib import Path
import shlex
import sys
p = Path(sys.argv[1]) / '.checks'
commands = []
for line in (p / 'plan.txt').read_text().splitlines():
    if 'command:' in line and 'node_modules/.bin/vitest' in line:
        args = shlex.split(line.split(' && GOWORK=off ', 1)[1][:-1])
        commands.append(['vitest', *args[1:]])
(p / 'parser-input.json').write_text(json.dumps(commands))
PYTEST
(cd "$root/integrations/bb-plugin-autarch" && "$AFFECTED_REAL_NODE" --input-type=module - "$scratch/.checks/parser-input.json" <<'JSTEST'
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseCLI } from 'vitest/node';
const commands = JSON.parse(readFileSync(process.argv[2], 'utf8'));
assert.equal(commands.length, 1);
for (const command of commands) {
  const parsed = parseCLI([...command]);
  const files = command[1] === 'related' ? parsed.options.related : parsed.filter;
  assert.ok(files.length > 0);
}
assert.equal(commands[0][1], 'run');
const files = parseCLI([...commands[0]]).filter;
assert.equal(new Set(files).size, files.length);
assert.ok(files.includes('./value.test.ts'));
assert.ok(files.includes('./related.test.ts'));
assert.equal(files.filter(f => f === './__tests__/docs.test.ts').length, 1);
JSTEST
)
check README.md 'mode: none'
check go.mod 'full:'
for file in integrations/bb-plugin-autarch/package-lock.json AGENTS.md integrations/bb-plugin-autarch/tsconfig.json migrations.ts data.json unknown.txt testdata/sample.go scripts/probe.sh .github/probe.yml Makefile data/sample.ts fixtures/sample.ts; do
  check "$file" 'full:'
done
check data/sample.ts 'unmapped or shared input: data/sample.ts'
for file in docs/wiki/src/page.md integrations/bb-plugin-autarch/README.md integrations/bb-plugin-autarch/PLUGIN_OVERVIEW.md docs/unknown.md; do
  check "$file" 'Markdown is a test input'
done
check nested/main.go 'Go package mapping unavailable'
# Physical deletion and rename add commits; no reference moves backwards.
rm leaf/deleted.go
git add leaf/deleted.go
git commit -qm delete
verify 'missing input in checkout'
mv leaf/old.go leaf/new.go
git add leaf/old.go leaf/new.go
git commit -qm rename
verify 'renamed input'
AFFECTED_NO_RELATED=yes check integrations/bb-plugin-autarch/orphan.ts 'no related tests for'
[[ $plan == *'vitest run --maxWorkers=2)'* ]]
# A source read without an import still schedules the audited readers.
check integrations/bb-plugin-autarch/read-by-static-scan.ts 'source readers:'
[[ $plan == *'./docs/wiki'* && $plan != *'./internal/door'* && $plan == *'./__tests__/no-exec.test.ts'* ]]
# A plugin read added to either an existing Go-only reader or a new package
# must remain visible to a later TS-only change.
for package in cmd/autarch newpackage; do
  mkdir -p "$package"
  prefix=..
  if [[ $package == cmd/autarch ]]; then prefix=../..; fi
  printf 'package fixture\nimport ("os"; "testing")\nfunc TestPlugin(t *testing.T) { if _, err := os.ReadFile("%s/integrations/bb-plugin-autarch/value.ts"); err != nil { t.Fatal(err) } }\n' "$prefix" > "$package/plugin_test.go"
  git add "$package/plugin_test.go"
  git commit -qm plugin-reader
  check integrations/bb-plugin-autarch/value.ts 'Go reader missing plugin side'
  printf 'package fixture\n' > "$package/plugin_test.go"
  git add "$package/plugin_test.go"
  git commit -qm fixture-reader-complete
done
# New readers fail closed pending an explicit manifest audit.
mkdir -p integrations/bb-plugin-autarch/__tests__
printf '/* readFileSync */\n' > integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
git add .
git commit -qm reader
verify 'unlisted source reader'
for snippet in 'new Worker(' 'execFile(' 'execFileSync(' 'spawn(' 'node:fs/promises' 'readFile(' 'createReadStream(' 'exec(' 'execSync(' 'fork(' 'existsSync(' 'statSync(' './v2build.js' './v6build.js' './hash-golden-helper.js' './deploy-tool/rig.js' './static-scan.js' "'./rig.js'"; do
  printf '// %s\n' "$snippet" > integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
  git add integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
  git commit -qm mediated
  verify 'unlisted source reader'
done
if AFFECTED_FAIL=yes bash "$script" HEAD~1 HEAD --run > /dev/null 2>&1; then exit 1; fi
printf 'dirty\n' >> README.md
if bash "$script" HEAD~1 HEAD --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
git add README.md
git commit -qm tracked
printf 'untracked\n' > untracked.txt
if bash "$script" HEAD~1 HEAD --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
git add untracked.txt
git commit -qm untracked
if bash "$script" HEAD~2 HEAD~1 --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
printf 'PASS: %s mapping cases, reader coverage, orphan fallback, execution, parser and three checkout refusals\n' "$cases"
