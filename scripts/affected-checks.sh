#!/usr/bin/env bash
# PR-iteration helper only; full merge-head checks remain required (mk-ykk4s.7.4.7).
set -euo pipefail
if [[ $# -lt 2 || $# -gt 3 || ${3:---print} != --print && ${3:---print} != --run ]]; then
  echo 'usage: affected-checks.sh BASE HEAD [--print|--run]' >&2
  exit 2
fi
exec python3 - "$(cd "$(dirname "$0")" && pwd)" "$@" <<'PY'
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys

script_dir = Path(sys.argv[1])
base, head = sys.argv[2:4]
run = len(sys.argv) == 5 and sys.argv[4] == '--run'
root = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], text=True).strip()
os.chdir(root)
env = dict(os.environ, GOWORK='off', GOMAXPROCS='8')
actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
requested = subprocess.check_output(['git', 'rev-parse', '--verify', head + '^{commit}'], text=True).strip()
if actual != requested or subprocess.check_output(['git', 'status', '--porcelain']):
    print('refused: HEAD must match the requested head and the checkout must be clean', flush=True)
    sys.exit(2)
readers = json.loads((script_dir / 'affected-readers.json').read_text())
files = subprocess.check_output(['git', 'diff', '--name-only', '-z', base + '...' + head]).decode().split('\0')
files = [f for f in files if f]
plugin = 'integrations/bb-plugin-autarch/'
reason = ''
go_files, ts_files = [], []
for f in files:
    p = Path(f)
    parts = {s.lower() for s in p.parts}
    name = p.name.lower()
    if (name in {'go.mod', 'go.sum', 'package.json', 'package-lock.json', 'dev', 'makefile', 'gnumakefile',
                 'agents.md', 'claude.md', 'coding_standards.md', 'skill.md', 'philosophy.md', 'instructions.md'}
        or any(s in parts for s in {'scripts', '.github', 'testdata', 'fixtures', 'data', 'agents', '.claude', '.codex', '.agents'})
        or any(s in name for s in {'tsconfig', 'vitest', 'oxlint', 'migration'})
        or name.startswith(('agents.', 'claude.')) or name.endswith('.mk')):
        reason = 'unmapped or shared input: ' + f
        break
    if not p.is_file():
        reason = 'missing input in checkout: ' + f
        break
    if name.endswith('.md'):
        if f not in readers['unread_markdown']:
            reason = 'Markdown is a test input or has no audited exemption: ' + f
            break
        continue
    if f.endswith('.go'):
        go_files.append(f)
    elif f.startswith(plugin) and f.endswith(('.ts', '.tsx')):
        ts_files.append(f[len(plugin):])
    else:
        reason = 'unmapped input: ' + f
        break

if subprocess.check_output(['git', 'diff', '--name-only', '--diff-filter=R', '-M', base + '...' + head]):
    reason = 'renamed input requires full checks'
packages = []
if not reason and go_files:
    try:
        raw = subprocess.check_output(['go', 'list', '-buildvcs=false', '-mod=readonly', '-deps', '-test', '-json', './...'], env=env, text=True)
        decoder = json.JSONDecoder()
        objects = []
        while raw.strip():
            raw = raw.lstrip()
            obj, end = decoder.raw_decode(raw)
            objects.append(obj)
            raw = raw[end:]
        roots = {o['ImportPath']: o for o in objects if not o.get('DepOnly') and not o.get('ForTest') and not o['ImportPath'].endswith('.test')}
        by_dir = {str(Path(o['Dir']).resolve()): p for p, o in roots.items()}
        changed = {by_dir[str(Path(f).parent.resolve())] for f in go_files}
        for p in roots:
            variants = [o for o in objects if o['ImportPath'] in {p, p + '.test'} or o.get('ForTest') == p]
            if any(changed.intersection(d.split(' [')[0] for d in [o['ImportPath'], *o.get('Deps', [])]) for o in variants):
                packages.append(p)
        if not changed.issubset(packages):
            raise ValueError('changed package not selected')
        packages.sort()
    except (subprocess.CalledProcessError, KeyError, ValueError) as err:
        reason = 'Go package mapping unavailable: ' + type(err).__name__

# Readers bypass import graphs. Keep the audited, explicit list shrink-safe: new
# direct readers force full checks until the manifest has been reviewed.
if (reason or go_files or ts_files):
    import re
    audit_go = re.compile(r'os\.ReadFile|os\.Open|filepath\.Walk|exec\.Command\("go"')
    audit_ts = re.compile('readFileSync|readdirSync|\\bexecFile(?:Sync)?\\b|\\bspawn(?:Sync)?\\b|copyFileSync|cpSync|new\\s+Worker\\s*\\(|fs/promises|v2build|v6build|hash-golden-helper|deploy-tool/rig|static-scan|["\']\\./rig(?:\\.js)?["\']')
    tracked = subprocess.check_output(['git', 'ls-files', '-z'], text=True).split('\0')
    for path in [Path(f) for f in tracked if f.endswith('_test.go')]:
        if any(x in path.parts for x in ('node_modules', 'build', '.git', 'vendor')):
            continue
        package = './' if path.parent == Path('.') else './' + path.parent.as_posix()
        if audit_go.search(path.read_text()) and package not in readers['go']:
            reason = 'unlisted source reader: ' + path.as_posix()
            break
    for path in [Path(f) for f in tracked if f.startswith(plugin + '__tests__/') and '.test.' in f]:
        if path.suffix in ('.ts', '.tsx') and audit_ts.search(path.read_text()) and path.relative_to(plugin).as_posix() not in readers['vitest']:
            reason = 'unlisted source reader: ' + path.as_posix()
            break

selected_tests = set(readers['vitest'])
selected_tests.update(f for f in ts_files if '.test.' in Path(f).name or '.spec.' in Path(f).name)
full_vitest = bool(reason or go_files)
vitest_reason = 'Go changes require cross-language binary checks' if go_files else 'full fallback'
if ts_files and not full_vitest:
    # Probe each input separately: a tested file must not mask an orphan source.
    for file in ts_files:
        try:
            raw = subprocess.check_output(['nice', '-n', '19', 'ionice', '-c3', 'node', str(script_dir / 'affected-related.mjs'), './' + file], cwd=plugin, env=env, text=True)
            selected = json.loads(raw)
            if not isinstance(selected, list) or not selected:
                full_vitest = True
                vitest_reason = 'no related tests for ' + file
                break
            for spec in selected:
                path = Path(spec)
                if not path.is_absolute():
                    path = Path(plugin).resolve() / path
                selected_tests.add(path.resolve().relative_to(Path(plugin).resolve()).as_posix())
        except (OSError, subprocess.CalledProcessError, ValueError, TypeError):
            full_vitest = True
            vitest_reason = 'related test discovery unavailable for ' + file
            break
if reason or go_files or ts_files:
    packages = sorted(set(packages) | {p for p, entry in readers['go'].items() if reason or go_files or 'plugin' in entry['sides']})
mode = 'full' if reason else 'affected' if go_files or ts_files else 'none'
print('mode: ' + mode, flush=True)
print(('full: ' + reason) if reason else 'reason: ' + ('mapped source changes' if mode == 'affected' else 'docs-only or no changes; nothing to run'), flush=True)
print('Go packages: ' + ('./...' if reason else ' '.join(packages) or '(none)'), flush=True)
vitest_plan = '(full suite: ' + vitest_reason + ')' if full_vitest else ' '.join(shlex.quote(f) for f in sorted(selected_tests)) if ts_files else '(none)'
print('vitest files: ' + vitest_plan, flush=True)
if mode != 'none':
    print('source readers: ' + ' '.join(readers['vitest']), flush=True)
commands = []
if mode != 'none':
    commands.append(('.', ['go', 'build', '-mod=readonly', './...']))
    if reason or packages:
        commands.append(('.', ['go', 'test', '-mod=readonly', '-race', '-p', '8', *(['./...'] if reason else packages)]))
    if mode != 'none':
        commands.append((plugin, ['node_modules/.bin/tsc', '-p', 'tsconfig.json']))
        if full_vitest:
            commands.append((plugin, ['node_modules/.bin/vitest', 'run', '--maxWorkers=2']))
        else:
            commands.append((plugin, ['node_modules/.bin/vitest', 'run', '--maxWorkers=2', *['./' + f for f in sorted(selected_tests)]]))
for directory, command in commands:
    print('command: (cd ' + shlex.quote(directory) + ' && GOWORK=off ' + shlex.join(command) + ')', flush=True)
    if run:
        subprocess.run(['nice', '-n', '19', 'ionice', '-c3', *command], cwd=directory, env=env, check=True)
PY
