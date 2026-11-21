#!/usr/bin/env bash
# Fixture history is append-only; every case compares its commit with its parent.
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
mkdir -p leaf consumer other testconsumer integrations/bb-plugin-autarch
printf 'module fixture\n\ngo 1.25\n' > go.mod
printf 'package leaf\n' > leaf/leaf.go
printf 'package consumer\nimport _ "fixture/leaf"\n' > consumer/consumer.go
printf 'package other\n' > other/other.go
printf 'package testconsumer\n' > testconsumer/main.go
printf 'package testconsumer_test\nimport _ "fixture/leaf"\n' > testconsumer/main_test.go
printf 'export const value = 1;\n' > integrations/bb-plugin-autarch/value.ts
printf 'export {};\n' > integrations/bb-plugin-autarch/value.test.ts
printf '{}\n' > integrations/bb-plugin-autarch/package-lock.json
printf 'Notes\n' > README.md
printf 'sample\n' > unknown.txt
git add .
git commit -qm initial
check() {
  local file=$1 expected=$2
  printf '\n' >> "$file"
  git add "$file"
  git commit -qm change
  plan=$(bash "$script" HEAD~1 HEAD --print)
  [[ $plan == *"$expected"* ]] || { printf '%s\n' "$plan"; exit 1; }
}
check leaf/leaf.go 'mode: affected'
[[ $plan == *'Go packages: fixture/consumer fixture/leaf'* ]]
[[ $plan == *'Go packages: fixture/consumer fixture/leaf fixture/testconsumer'* ]]
[[ $plan != *'Go packages: fixture/consumer fixture/leaf fixture/other'* ]]
check go.mod 'full:'
check integrations/bb-plugin-autarch/package-lock.json 'full:'
check integrations/bb-plugin-autarch/value.ts 'vitest related --run'
check integrations/bb-plugin-autarch/value.test.ts 'vitest run --maxWorkers=2 ./value.test.ts'
check README.md 'mode: none'
check unknown.txt 'full:'
for file in AGENTS.md integrations/bb-plugin-autarch/tsconfig.json migrations.ts data.json; do
  check "$file" 'full:'
done
# Fake executables exercise execution and failure propagation, without fetching dependencies.
mkdir -p fake-bin integrations/bb-plugin-autarch/node_modules/.bin
cat > fake-bin/go <<'SH'
#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$AFFECTED_LOG"
if [[ ${AFFECTED_FAIL:-} == yes && $1 == test ]]; then exit 7; fi
SH
cat > integrations/bb-plugin-autarch/node_modules/.bin/tsc <<'SH'
#!/usr/bin/env bash
printf 'tsc %s\n' "$*" >> "$AFFECTED_LOG"
SH
cat > integrations/bb-plugin-autarch/node_modules/.bin/vitest <<'SH'
#!/usr/bin/env bash
printf 'vitest %s\n' "$*" >> "$AFFECTED_LOG"
SH
chmod +x fake-bin/go integrations/bb-plugin-autarch/node_modules/.bin/{tsc,vitest}
export PATH="$scratch/fake-bin:$PATH" AFFECTED_LOG="$scratch/commands.log"
check integrations/bb-plugin-autarch/value.test.ts 'mode: affected'
bash "$script" HEAD~1 HEAD --run > /dev/null
grep -q 'go build -mod=readonly ./...' "$AFFECTED_LOG"
grep -q 'tsc -p tsconfig.json' "$AFFECTED_LOG"
grep -q 'vitest related --run' "$AFFECTED_LOG"
grep -q 'vitest run --maxWorkers=2 ./value.test.ts' "$AFFECTED_LOG"
# Verify actual Vitest 3 argument parsing, rather than only the fake executable's log.
printf '%s\n' "$plan" > "$scratch/plan.txt"
python3 - "$scratch" <<'PYTEST'
import json
from pathlib import Path
import shlex
import sys
p = Path(sys.argv[1])
commands = []
for line in (p / 'plan.txt').read_text().splitlines():
    if 'node_modules/.bin/vitest' in line:
        args = shlex.split(line.split(' && GOWORK=off ', 1)[1][:-1])
        commands.append(['vitest', *args[1:]])
(p / 'parser-input.json').write_text(json.dumps(commands))
PYTEST
(cd "$root/integrations/bb-plugin-autarch" && node --input-type=module - "$scratch/parser-input.json" <<'JSTEST'
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { parseCLI } from 'vitest/node';
const commands = JSON.parse(readFileSync(process.argv[2], 'utf8'));
assert.equal(commands.length, 2);
for (const command of commands) {
  const parsed = parseCLI([...command]);
  assert.deepEqual(command[1] === 'related' ? parsed.options.related : parsed.filter, ['./value.test.ts']);
}
JSTEST
)
check unknown.txt 'full:'
if AFFECTED_FAIL=yes bash "$script" HEAD~1 HEAD --run > /dev/null 2>&1; then
  echo 'execution failure was not propagated' >&2
  exit 1
fi
printf 'PASS: 11 mapping cases plus execution and failure propagation\n' 
