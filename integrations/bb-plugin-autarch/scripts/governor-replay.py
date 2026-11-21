#!/usr/bin/env python3
"""Replay a sanitized account-read race through an upstream script, offline.

Usage: python3 scripts/governor-replay.py SCRIPT legacy|fixed
No commands or live state are read or written by the loaded script.
"""
import contextlib
import io
import json
import math
from pathlib import Path
import runpy
import sys
from unittest.mock import patch

script, expected = sys.argv[1:]
assert expected in ("legacy", "fixed")
fixture = json.loads((Path(__file__).parent.parent / "__tests__/fixtures/governor-zero-pace.json").read_text())
m = runpy.run_path(script)
g = m["main"].__globals__
clock = [fixture["now"]]
def accounts(*args):
    clock[0] = fixture["readCompletedAt"]
    return {"accounts": fixture["accounts"]}
g.update(bb_json=accounts, load_config=lambda: m["DEFAULTS"],
         load_state=lambda cfg: {"level": 0}, read_floor=lambda *args: None,
         load_samples=lambda *args: fixture["samples"])
output = io.StringIO()
with patch.object(m["time"], "time", lambda: clock[0] / 1000), contextlib.redirect_stdout(output):
    assert m["main"](["--dry-run", "--json"]) == 0
result = json.loads(output.getvalue())
pace, note = fixture["result" if expected == "legacy" else "completedResult"]
assert math.isclose(result["pace"], pace, abs_tol=1e-12), result
assert result["note"] == note, result
assert result["new_level"] == (0 if expected == "legacy" else 1), result
print(f"{expected}: pace={result['pace']:.12f}, level={result['new_level']}; matches recorded Python/TypeScript evidence")
