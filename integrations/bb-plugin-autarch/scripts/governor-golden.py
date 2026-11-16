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
from datetime import datetime, timezone

script, tests, output = sys.argv[1:]
spec = importlib.util.spec_from_file_location("upstream_tests", tests)
t = importlib.util.module_from_spec(spec)
spec.loader.exec_module(t)
m = t.M
assert os.path.realpath(m.__file__) == os.path.realpath(script)
cases = []
omit = {"state_dir", "operator_toml", "storage_root", "samples_path", "bb_host"}
replacements = {}

def clean(x):
    if isinstance(x, dict):
        return {k: clean(v) for k, v in x.items() if k not in omit}
    if isinstance(x, (list, tuple)):
        return [clean(v) for v in x]
    if isinstance(x, str):
        for old, new in sorted(replacements.items(), key=lambda kv: -len(kv[0])):
            x = x.replace(old, new)
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

# Observe actual upstream step tests, including their temporary card mocks and
# fake command failures. Capture boundary outcomes so TS can replay decisions
# against the same approvals, file results, failed applies and reconciliations.
original_step = m.step
def capture_step(cfg, now, st, accounts, run=m.sh, dry=False):
    for k in omit:
        if isinstance(cfg.get(k), str) and cfg[k]:
            replacements[cfg[k]] = "governed" if k == "bb_host" else "/fixture/" + k
    before = copy.deepcopy(st)
    floor = m.read_floor(cfg, now)
    samples = m.load_samples(cfg, now)
    events, originals = [], {}
    for name in ("card_answer", "file_card", "apply_operator", "apply_bb_limit", "append_inbox"):
        fn = originals[name] = getattr(m, name)
        def observe(*args, _name=name, _fn=fn, **kwargs):
            event = {"kind": _name}
            events.append(event)
            if _name == "apply_operator":
                event.update(level=args[1]["level"], force=kwargs.get("force", False))
            elif _name == "apply_bb_limit":
                event["level"] = args[1]["level"]
            try:
                value = _fn(*args, **kwargs)
                event["result"] = value
                return value
            except Exception as e:
                event["error"] = str(e)
                raise
        setattr(m, name, observe)
    try:
        decision, state, rc = original_step(cfg, now, st, accounts, run, dry)
    finally:
        for name, fn in originals.items():
            setattr(m, name, fn)
    overrides = {k: v for k, v in cfg.items() if k not in omit and v != m.DEFAULTS.get(k)}
    cases.append({"kind": "step", "args": clean([before, accounts, now, floor, samples, dry, overrides, events]),
                  "result": clean({"decision": decision, "state": state, "rc": rc})})
    return decision, state, rc
m.step = capture_step
with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
    result = unittest.TextTestRunner(stream=io.StringIO()).run(unittest.defaultTestLoader.loadTestsFromModule(t))
if not result.wasSuccessful():
    raise RuntimeError(result.errors + result.failures)

# Capture public CLI dry-run decisions, beyond the upstream function assertions.
now = t.NOW
with tempfile.TemporaryDirectory() as d:
    cfg = copy.deepcopy(m.DEFAULTS)
    cfg.update(state_dir=d, samples_path=os.path.join(d, "samples.jsonl"))
    m.load_config = lambda: cfg
    m.time.time = lambda: now / 1000
    m.bb_json = lambda *a, **k: {"accounts": accounts}
    m.load_state = lambda c: state
    for pace_mode, history in [("cumulative", "none"), ("recent", "rising"), ("recent", "flat"), ("recent", "short"), ("recent", "reset"), ("recent", "stale")]:
        cfg["pace_mode"] = pace_mode
        for cur in (0, 1, 2):
          for streak in (0, 1):
            for util in (.1, .5, .9):
              for floor in (None, {"level": 1, "until": "2099-01-01T00:00:00Z", "source": "operator"}):
                    accounts = [t.acct(util, .5)]
                    state = {"level": cur, "good_streak": streak}
                    reset = accounts[0]["sevenDayResetAt"]
                    hours = {"rising": 2.9, "flat": 2.9, "short": 1, "reset": 2.9, "stale": 12, "none": 0}[history]
                    baseline = util if history == "flat" else max(0, util - .06)
                    sample_reset = reset - t.WEEK if history == "reset" else reset
                    rows = [] if history == "none" else [{"id": "a", "provider": "claude", "observed_at": now - int(hours * t.H), "sevenDayUtilization": baseline, "sevenDayResetAt": sample_reset}]
                    with open(cfg["samples_path"], "w") as f:
                        for row in rows:
                            f.write(json.dumps(row) + "\n")
                    if floor:
                        json.dump(floor, open(os.path.join(d, "floor.json"), "w"))
                    elif os.path.exists(os.path.join(d, "floor.json")):
                        os.unlink(os.path.join(d, "floor.json"))
                    out = io.StringIO()
                    with contextlib.redirect_stdout(out):
                        assert m.main(["--dry-run", "--json"]) == 0
                    cases.append({"kind": "cli", "args": [state, accounts, now, floor, pace_mode, m.load_samples(cfg, now)], "result": json.loads(out.getvalue())})

# Python is the oracle for accepted ISO floor forms, including invalid dates.
for until in ("2099-11-20", "20991120", "2099-11-20T00:00:00+0000", "20991120T000000+00", "2099-W47-5", "2099W475", "2099-W47", "2099-11-20 00:00:00.123456+00:00", "2099-11-20T12.5", "2099-11-20T00:00:00+0260", "2099-11-20T00:00:00+00:00:00.5", "2099-11-20T00:00:00-05:30:15.5", "2099-02-30", "2099-11-20T24:00", "2099-W54-1", "2099-11", "soon"):
    with tempfile.TemporaryDirectory() as d:
        cfg = dict(m.DEFAULTS, state_dir=d)
        raw = {"level": 1, "until": until, "source": "operator"}
        with open(os.path.join(d, "floor.json"), "w") as f:
            json.dump(raw, f)
        times = [now]
        try:
            parsed = datetime.fromisoformat(until.replace("Z", "+00:00"))
            if parsed.tzinfo is None:
                parsed = parsed.replace(tzinfo=timezone.utc)
            expiry = parsed.timestamp() * 1000
            times += [expiry - 1, expiry, expiry + 1]
        except ValueError:
            pass
        for observed in times:
            cases.append({"kind": "read_floor", "args": [raw, observed], "result": m.read_floor(cfg, observed)})

with open(output, "w") as f:
    f.write('{"source_sha256":' + json.dumps(hashlib.sha256(open(script, "rb").read()).hexdigest()) + ',\n"upstream_tests":' + str(result.testsRun) + ',\n"cases":[\n')
    f.write(',\n'.join(json.dumps(c, allow_nan=False, separators=(',', ':')) for c in cases))
    f.write('\n]}\n')
