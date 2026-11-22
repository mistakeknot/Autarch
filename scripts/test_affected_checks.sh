#!/usr/bin/env bash
# All fixture history is append-only. Scratch artifacts are ignored before planning.
set -euo pipefail
script=$(realpath "$(dirname "$0")/affected-checks.sh")
root=$(git rev-parse --show-toplevel)
scratch_root=${AFFECTED_CHECKS_SCRATCH:-$root/build/affected-checks}
mkdir -p "$scratch_root"
scratch=$(mktemp -d "$scratch_root/affected-fixture.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
# Exercise comment tokenization without executing the selector's main program.
python3 - "$script" <<'PYLEX'
import ast
from pathlib import Path
import sys
source = Path(sys.argv[1]).read_text().split("<<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
func = next(n for n in ast.parse(source).body if isinstance(n, ast.FunctionDef) and n.name == 'go_plugin_input')
ns = {}
exec(compile(ast.Module(body=[func], type_ignores=[]), 'go-scan-fixture', 'exec'), ns)
scan = ns['go_plugin_input']
for text, expected in [
    ('// scanner for integrations', False),
    ('/* bb-plugin-autarch */', False),
    ('const note = "third-party integrations"', False),
    (r'const note = "SaaS integrations\n"', False),
    ('const a = "git"; const b = "third-party integrations"', False),
    ('// cd integrations && git ls-files', False),
    ('const command = "cd integrations && git ls-files"', True),
    ('const command = "git ls-files integrations"', True),
    ('const command = "--dir=integrations"', True),
    ('const command = "cd integrations; ls"', True),
    ('const command = "$(ls integrations)"', True),
    ('const command = "ls integrations*"', True),
    ('const command = "cd integrations&&ls"', True),
    ('const command = "tar czf out.tgz integrations"', True),
    ('const command = "cat >integrations"', True),
    ('const command = "cp integrations backup"', True),
    ('const command = "rg TODO integrations"', True),
    ('const command = "reader <integrations"', True),
    ('const command = "writer >integrations"', True),
    ('const note = "run the test for integrations"', True),
    ('const path = "../../integrations"', True),
    ('const path = `integrations`', True),
    ('const path = "//bb-plugin-autarch"', True),
    ('const path = `/*name*/integrations`', True),
    (r'const path = "integrations\\value"', True),
    (r'const path = "..\\integrations"', True),
    ('//go:embed integrations/plugin/file.json', True),
    ('/* //go:embed integrations/plugin/file.json */', False),
]:
    assert scan(text) == expected, text
PYLEX
cd "$scratch"
git init -q
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
git -c user.name=Fixture -c user.email=fixture commit -qm initial
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
  git -c user.name=Fixture -c user.email=fixture commit -qm change
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
git -c user.name=Fixture -c user.email=fixture commit -qm delete
verify 'missing input in checkout'
mv leaf/old.go leaf/new.go
git add leaf/old.go leaf/new.go
git -c user.name=Fixture -c user.email=fixture commit -qm rename
verify 'renamed input'
AFFECTED_NO_RELATED=yes check integrations/bb-plugin-autarch/orphan.ts 'no related tests for'
[[ $plan == *'vitest run --maxWorkers=2)'* ]]
# A source read without an import still schedules the audited readers.
check integrations/bb-plugin-autarch/read-by-static-scan.ts 'source readers:'
[[ $plan == *'./docs/wiki'* && $plan != *'./internal/door'* && $plan == *'./__tests__/no-exec.test.ts'* ]]
# Non-path prose in a Go-only package must not poison later plugin-only plans.
mkdir -p cmd/autarch
for style in line block prose rawprose escapedprose; do
  case $style in
    line) printf 'package fixture\n// scanner for integrations\n' > cmd/autarch/prose_test.go ;;
    block) printf 'package fixture\n/* scanner for\n bb-plugin-autarch integrations\n*/\n' > cmd/autarch/prose_test.go ;;
    prose) printf 'package fixture\nconst note = "third-party integrations"\n' > cmd/autarch/prose_test.go ;;
    rawprose) printf 'package fixture\nconst note = `third-party integrations`\n' > cmd/autarch/prose_test.go ;;
    escapedprose) printf '%s\n' 'package fixture' 'const note = "SaaS integrations\n"' > cmd/autarch/prose_test.go ;;
  esac
  git add cmd/autarch/prose_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm prose
  check integrations/bb-plugin-autarch/value.ts 'mode: affected'
  go_plan=$(printf '%s\n' "$plan" | sed -n '/^Go packages:/p')
  [[ $go_plan != *'./cmd/autarch'* ]]
  printf 'package fixture\n' > cmd/autarch/prose_test.go
  git add cmd/autarch/prose_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm prose-complete
done
# Shell/flag context must be in the same literal as the space-delimited path.
for command_text in 'cd integrations && git ls-files' 'git ls-files integrations' '--dir=integrations' \
  'cd integrations; ls' '$(ls integrations)' 'ls integrations*' 'cd integrations&&ls' \
  'tar czf out.tgz integrations' 'cat >integrations' 'cp integrations backup' 'rg TODO integrations'; do
  printf 'package fixture\nimport "os/exec"\nvar command = exec.Command("sh", "-c", "%s")\n' "$command_text" > cmd/autarch/shell_test.go
  git add cmd/autarch/shell_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm shell-input
  check integrations/bb-plugin-autarch/value.ts 'Go reader missing plugin side: cmd/autarch/shell_test.go'
  printf 'package fixture\n' > cmd/autarch/shell_test.go
  git add cmd/autarch/shell_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm shell-input-complete
done
# Common command words can occur in prose; conservatively require full checks.
printf 'package fixture\nconst note = "run the test for integrations"\n' > cmd/autarch/command_prose_test.go
git add cmd/autarch/command_prose_test.go
git -c user.name=Fixture -c user.email=fixture commit -qm command-prose
check integrations/bb-plugin-autarch/value.ts 'Go reader missing plugin side: cmd/autarch/command_prose_test.go'
printf 'package fixture\n' > cmd/autarch/command_prose_test.go
git add cmd/autarch/command_prose_test.go
git -c user.name=Fixture -c user.email=fixture commit -qm command-prose-complete
# A plugin read added to either an existing Go-only reader or a new package
# must remain visible to a later TS-only change.
for package in cmd/autarch newpackage; do
  mkdir -p "$package"
  prefix=..
  if [[ $package == cmd/autarch ]]; then prefix=../..; fi
  printf 'package fixture\nimport ("os"; "testing")\nfunc TestPlugin(t *testing.T) { if _, err := os.ReadFile("%s/integrations/bb-plugin-autarch/value.ts"); err != nil { t.Fatal(err) } }\n' "$prefix" > "$package/plugin_test.go"
  git add "$package/plugin_test.go"
  git -c user.name=Fixture -c user.email=fixture commit -qm plugin-reader
  check integrations/bb-plugin-autarch/value.ts "Go reader missing plugin side: $package/plugin_test.go"
  printf 'package fixture\n' > "$package/plugin_test.go"
  git add "$package/plugin_test.go"
  git -c user.name=Fixture -c user.email=fixture commit -qm fixture-reader-complete
done
# A helper can read plugin input even when no test calls the audited Go APIs.
mkdir -p cmd/autarch
printf 'package fixture\nimport ("os"; "testing")\nfunc checkPlugin(t *testing.T, p string) { if _, err := os.ReadFile(p); err != nil { t.Fatal(err) } }\n' > cmd/autarch/helper.go
for style in literal segments prefix raw helper; do
  if [[ $style == literal ]]; then
    printf 'package fixture\nimport "testing"\nfunc TestPlugin(t *testing.T) { checkPlugin(t, "../../integrations/bb-plugin-autarch/value.ts") }\n' > cmd/autarch/plugin_test.go
  elif [[ $style == segments || $style == raw ]]; then
    segment='"integrations"'
    if [[ $style == raw ]]; then segment='`integrations`'; fi
    printf 'package fixture\nimport ("testing"; "path/filepath")\nfunc TestPlugin(t *testing.T) { checkPlugin(t, filepath.Join("..", "..", %s, "bb-plugin-" + "autarch", "value.ts")) }\n' "$segment" > cmd/autarch/plugin_test.go
  elif [[ $style == prefix ]]; then
    printf 'package fixture\nimport "testing"\nfunc TestPlugin(t *testing.T) { root := "../../integrations"; checkPlugin(t, root + "/" + "bb-plugin-" + "autarch/value.ts") }\n' > cmd/autarch/plugin_test.go
  else
    printf 'package fixture\nimport ("os"; "testing")\nfunc checkPlugin(t *testing.T, _ string) { if _, err := os.ReadFile("../../integrations/bb-plugin-autarch/value.ts"); err != nil { t.Fatal(err) } }\n' > cmd/autarch/helper.go
    printf 'package fixture\nimport "testing"\nfunc TestPlugin(t *testing.T) { checkPlugin(t, "") }\n' > cmd/autarch/plugin_test.go
  fi
  git add cmd/autarch/helper.go cmd/autarch/plugin_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm helper-reader
  reader_path=cmd/autarch/plugin_test.go
  if [[ $style == helper ]]; then reader_path=cmd/autarch/helper.go; fi
  check integrations/bb-plugin-autarch/value.ts "Go reader missing plugin side: $reader_path"
  printf 'package fixture\n' > cmd/autarch/plugin_test.go
  git add cmd/autarch/plugin_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm fixture-helper-complete
  if [[ $style == helper ]]; then
    printf 'package fixture\nimport ("os"; "testing")\nfunc checkPlugin(t *testing.T, p string) { if _, err := os.ReadFile(p); err != nil { t.Fatal(err) } }\n' > cmd/autarch/helper.go
    git add cmd/autarch/helper.go
    git -c user.name=Fixture -c user.email=fixture commit -qm helper-neutral
  fi
done
# Paths in shell/regexp strings still couple a Go-only test to plugin inputs.
for shape in shell regexp; do
  if [[ $shape == shell ]]; then
    printf 'package fixture\nimport "os/exec"\nvar command = exec.Command("sh", "-c", "git ls-files integrations/")\n' > cmd/autarch/path_test.go
  else
    printf 'package fixture\nimport "regexp"\nvar filter = regexp.MustCompile("^integrations/")\n' > cmd/autarch/path_test.go
  fi
  git add cmd/autarch/path_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm path-context
  check integrations/bb-plugin-autarch/value.ts 'Go reader missing plugin side: cmd/autarch/path_test.go'
  printf 'package fixture\n' > cmd/autarch/path_test.go
  git add cmd/autarch/path_test.go
  git -c user.name=Fixture -c user.email=fixture commit -qm path-context-complete
done
# Go directives are build inputs, rather than discardable comments.
printf '{}\n' > integrations/bb-plugin-autarch/package.json
printf 'package fixture\nimport _ "embed"\n//go:embed integrations/bb-plugin-autarch/package.json\nvar embedded []byte\n' > embed.go
git add embed.go integrations/bb-plugin-autarch/package.json
git -c user.name=Fixture -c user.email=fixture commit -qm embed-input
check integrations/bb-plugin-autarch/value.ts 'Go reader missing plugin side: embed.go'
printf 'package fixture\n' > embed.go
git add embed.go
git -c user.name=Fixture -c user.email=fixture commit -qm embed-complete
# New readers fail closed pending an explicit manifest audit.
mkdir -p integrations/bb-plugin-autarch/__tests__
printf '/* readFileSync */\n' > integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
git add .
git -c user.name=Fixture -c user.email=fixture commit -qm reader
verify 'unlisted source reader'
for snippet in 'new Worker(' 'execFile(' 'execFileSync(' 'spawn(' 'node:fs/promises' 'readFile(' 'createReadStream(' 'exec(' 'execSync(' 'fork(' 'existsSync(' 'statSync(' './v2build.js' './v6build.js' './hash-golden-helper.js' './deploy-tool/rig.js' './static-scan.js' "'./rig.js'"; do
  printf '// %s\n' "$snippet" > integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
  git add integrations/bb-plugin-autarch/__tests__/new-reader.test.ts
  git -c user.name=Fixture -c user.email=fixture commit -qm mediated
  verify 'unlisted source reader'
done
if AFFECTED_FAIL=yes bash "$script" HEAD~1 HEAD --run > /dev/null 2>&1; then exit 1; fi
printf 'dirty\n' >> README.md
if bash "$script" HEAD~1 HEAD --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
git add README.md
git -c user.name=Fixture -c user.email=fixture commit -qm tracked
printf 'untracked\n' > untracked.txt
if bash "$script" HEAD~1 HEAD --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
git add untracked.txt
git -c user.name=Fixture -c user.email=fixture commit -qm untracked
if bash "$script" HEAD~2 HEAD~1 --print > .checks/refusal 2>&1; then exit 1; fi
grep -q 'refused:' .checks/refusal
printf 'PASS: %s mapping cases, reader coverage, orphan fallback, execution, parser and three checkout refusals\n' "$cases"
