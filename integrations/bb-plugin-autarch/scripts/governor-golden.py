#!/usr/bin/env python3
"""Generate portable policy fixtures from a supplied quota-governor checkout.

Usage: python3 scripts/governor-golden.py SCRIPT TEST_FILE OUTPUT
The upstream unit suite uses temporary files and fake runners. CLI comparisons
run main(--dry-run --json) with deterministic time, pool and state adapters.
No live governor state or commands are used.
"""
import contextlib
import copy
import hashlib
import importlib.util
import io
import json
import os
import re
import sys
import tempfile
import unittest

script, tests, output = sys.argv[1:]
spec = importlib.util.spec_from_file_location("upstream_tests", tests)
t = importlib.util.module_from_spec(spec)
spec.loader.exec_module(t)
m = t.M
assert os.path.realpath(m.__file__) == os.path.realpath(script)
cases = []
omit = {"state_dir", "operator_toml", "storage_root", "samples_path", "bb_host"}

def clean(x):
    if isinstance(x, dict):
        return {k: clean(v) for k, v in x.items() if k not in omit}
    if isinstance(x, (list, tuple)):
        return [clean(v) for v in x]
    if isinstance(x, str):
        return re.sub(r"\b[A-Z]{2,10}-\d+\b", "operator", x)
    return x

for name in ("choose", "apply_floor", "pooled_pace", "recent_pace", "edit_operator"):
    original = getattr(m, name)
    def wrap(*args, _name=name, _fn=original, **kwargs):
        result = _fn(*args, **kwargs)
        args = list(args)
        if _name == "recent_pace" and (len(args) < 4 or args[3] is None):
            args = args[:3] + [m.load_samples(args[2], args[1])]
        cfg_index = {"choose": 3, "pooled_pace": 2, "recent_pace": 2}.get(_name)
        if cfg_index is not None:
            args[cfg_index] = {k: v for k, v in args[cfg_index].items()
                               if k not in omit and v != m.DEFAULTS.get(k)}
        cases.append({"kind": _name, "args": clean(copy.deepcopy(args)), "result": clean(result)})
        return result
    setattr(m, name, wrap)
with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
    result = unittest.TextTestRunner(stream=io.StringIO()).run(unittest.defaultTestLoader.loadTestsFromModule(t))
if not result.wasSuccessful():
    raise RuntimeError(result.errors + result.failures)

# Capture public CLI dry-run decisions, beyond the upstream function assertions.
now = t.NOW
with tempfile.TemporaryDirectory() as d:
    cfg = copy.deepcopy(m.DEFAULTS)
    cfg.update(state_dir=d, samples_path=os.path.join(d, "samples.jsonl"), pace_mode="cumulative")
    m.load_config = lambda: cfg
    m.time.time = lambda: now / 1000
    m.bb_json = lambda *a, **k: {"accounts": accounts}
    m.load_state = lambda c: state
    for cur in (0, 1, 2):
        for streak in (0, 1):
            for util in (.1, .5, .9):
                for floor in (None, {"level": 1, "until": "2099-01-01T00:00:00Z", "source": "operator"}):
                    accounts = [t.acct(util, .5)]
                    state = {"level": cur, "good_streak": streak}
                    if floor:
                        json.dump(floor, open(os.path.join(d, "floor.json"), "w"))
                    elif os.path.exists(os.path.join(d, "floor.json")):
                        os.unlink(os.path.join(d, "floor.json"))
                    out = io.StringIO()
                    with contextlib.redirect_stdout(out):
                        assert m.main(["--dry-run", "--json"]) == 0
                    cases.append({"kind": "cli", "args": [state, accounts, now, floor], "result": json.loads(out.getvalue())})

with open(output, "w") as f:
    f.write('{"source_sha256":' + json.dumps(hashlib.sha256(open(script, "rb").read()).hexdigest()) + ',\n"upstream_tests":' + str(result.testsRun) + ',\n"cases":[\n')
    f.write(',\n'.join(json.dumps(c, allow_nan=False, separators=(',', ':')) for c in cases))
    f.write('\n]}\n')
