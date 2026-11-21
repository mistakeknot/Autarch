#!/usr/bin/env bash
# PR-iteration helper only; full merge-head checks remain required (mk-ykk4s.7.4.7).
set -euo pipefail
if [[ $# -lt 2 || $# -gt 3 || ${3:---print} != --print && ${3:---print} != --run ]]; then
  echo 'usage: affected-checks.sh BASE HEAD [--print|--run]' >&2
  exit 2
fi
exec python3 - "$@" <<'PY'
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys

base, head = sys.argv[1:3]
run = len(sys.argv) == 4 and sys.argv[3] == '--run'
root = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], text=True).strip()
os.chdir(root)
env = dict(os.environ, GOWORK='off', GOMAXPROCS='8')
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
    if name.endswith('.md'):
        continue
    if not p.is_file():
        reason = 'missing input in checkout: ' + f
        break
    if f.endswith('.go'):
        go_files.append(f)
    elif f.startswith(plugin) and f.endswith(('.ts', '.tsx')):
        ts_files.append(f[len(plugin):])
    else:
        reason = 'unmapped input: ' + f
        break

packages = []
if not reason and go_files:
    try:
        raw = subprocess.check_output(['go', 'list', '-mod=readonly', '-deps', '-test', '-json', './...'], env=env, text=True)
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

mode = 'full' if reason else 'affected' if go_files or ts_files else 'none'
print('mode: ' + mode, flush=True)
print(('full: ' + reason) if reason else 'reason: ' + ('mapped source changes' if mode == 'affected' else 'docs-only or no changes; nothing to run'), flush=True)
print('Go packages: ' + ('./...' if reason else ' '.join(packages) or '(none)'), flush=True)
print('vitest files: ' + ('(full suite)' if reason else ' '.join(shlex.quote(f) for f in ts_files) or '(none)'), flush=True)
commands = []
if mode != 'none':
    commands.append(('.', ['go', 'build', '-mod=readonly', './...']))
    if reason or packages:
        commands.append(('.', ['go', 'test', '-mod=readonly', '-race', '-p', '8', *(['./...'] if reason else packages)]))
    if reason or ts_files:
        commands.append((plugin, ['node_modules/.bin/tsc', '-p', 'tsconfig.json']))
        # Related selects importers; explicitly changed test files also run themselves.
        tests = [f for f in ts_files if '.test.' in Path(f).name or '.spec.' in Path(f).name]
        if reason:
            commands.append((plugin, ['node_modules/.bin/vitest', 'run', '--maxWorkers=2']))
        else:
            commands.append((plugin, ['node_modules/.bin/vitest', 'related', '--run', '--passWithNoTests', '--maxWorkers=2', *['./' + f for f in ts_files]]))
            if tests:
                commands.append((plugin, ['node_modules/.bin/vitest', 'run', '--maxWorkers=2', *['./' + f for f in tests]]))
for directory, command in commands:
    print('command: (cd ' + shlex.quote(directory) + ' && GOWORK=off ' + shlex.join(command) + ')', flush=True)
    if run:
        subprocess.run(['nice', '-n', '19', 'ionice', '-c3', *command], cwd=directory, env=env, check=True)
PY
