"""Metrics from a manifest and result rows (plan R5, R6, R8).

Headline numbers count only the first decision of each family, as n/N with a Wilson 95%
interval. "Meets 0.95" needs a lower bound of at least 0.90. Repeats go in their own table.
Held-other and sent-back-other are reported separately; send-back is exploratory.
"""

from __future__ import annotations

import hashlib
import math
from collections import Counter
from dataclasses import dataclass
from typing import Iterable

from .manifest import Entry, Manifest
from .store import parse_ts

Z95 = 1.959963984540054
TARGET = 0.95
FLOOR = 0.90


def wilson(k: int, n: int, z: float = Z95) -> tuple[float, float]:
    if n <= 0:
        return (0.0, 1.0)
    p = k / n
    d = 1 + z * z / n
    centre = (p + z * z / (2 * n)) / d
    half = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return (max(0.0, centre - half), min(1.0, centre + half))


@dataclass(frozen=True)
class Ratio:
    k: int
    n: int

    @property
    def point(self) -> float | None:
        return self.k / self.n if self.n else None

    @property
    def interval(self) -> tuple[float, float]:
        return wilson(self.k, self.n)

    def meets(self, target: float = TARGET, floor: float = FLOOR) -> bool:
        """A point estimate at the target is not evidence; the lower bound has to reach the floor."""
        return bool(self.n) and self.k / self.n >= target and self.interval[0] >= floor

    def text(self) -> str:
        if not self.n:
            return "n/a (0/0)"
        lo, hi = self.interval
        return f"{self.k}/{self.n} = {self.k / self.n:.3f} (95% CI {lo:.3f}-{hi:.3f})"


def hold_class(reason: str | None) -> str:
    r = reason or "unknown"
    return r.split(":")[0] + (":" + r.split(":")[1] if r.startswith(("gate:", "egress:")) and ":" in r else "")


def rows_for(results: Iterable[dict], manifest: Manifest, config_hash: str, mode: str, same_manifest: bool = True) -> dict[str, dict]:
    """One terminal row per decision for this configuration and mode (and manifest, for backtests).
    Live rows were stamped with the label snapshot of their day, so they are matched on decision id."""
    out: dict[str, dict] = {}
    for r in results:
        if (not same_manifest or r.get("manifest_id") == manifest.id) and r.get("config_hash") == config_hash and r.get("mode") == mode:
            out.setdefault(r["decision_id"], r)
    return out


def correct(row: dict, e: Entry) -> bool:
    return row.get("verdict") == "decided" and e.pick_kind == "option" and row.get("option") == e.pick_option


def baseline_correct(row: dict, e: Entry) -> bool:
    return e.pick_kind == "option" and bool(row.get("recommendation")) and row.get("recommendation") == e.pick_option


def structure_coverage(asks: Iterable) -> dict:
    """How much structural evidence the asks carry, so a reader can see what the holds rest on.

    `reversible` is a Go bool with omitempty: an absent value means false or never set, so it is
    reported but is neither a hold nor a permission. A home-move block is read from the moves
    table. A root-run block leaves no structured trace in data.db, so it is reported as
    unestablished: only the text signal and the production-root gate stand in for it."""
    asks = [a for a in asks if a is not None]
    opts = [o for a in asks for o in a.options if o.kind != "instruction"]
    return {
        "asks": len(asks),
        "options": len(opts),
        "reversible_true": sum(1 for o in opts if o.reversible is True),
        "reversible_false": sum(1 for o in opts if o.reversible is False),
        "reversible_absent": sum(1 for o in opts if o.reversible is None),
        "card_move": sum(1 for a in asks if a.move),
        "root_run_text_signal": sum(1 for a in asks if a.root_run),
        "root_run_structured": "unestablished",
    }


@dataclass
class Evaluation:
    split: str
    sub: str | None
    n_firsts: int
    n_judged: int
    decided: Ratio  # coverage: decided / judged
    precision: Ratio  # correct / decided
    holds: Counter
    paired: dict
    curve: list[dict]
    other: dict
    repeats: dict
    baseline_on_decided: Ratio
    structure: dict | None = None

    def as_dict(self) -> dict:
        return {
            "split": self.split, "sub": self.sub, "firsts": self.n_firsts, "judged": self.n_judged,
            "coverage": [self.decided.k, self.decided.n], "precision": [self.precision.k, self.precision.n],
            "precision_ci": list(self.precision.interval), "meets_0_95": self.precision.meets(),
            "holds": dict(self.holds), "paired": self.paired, "curve": self.curve, "other": self.other,
            "repeats": self.repeats, "baseline_on_decided": [self.baseline_on_decided.k, self.baseline_on_decided.n],
            "structure": self.structure,
        }

    def text(self) -> str:
        lines = [
            f"split={self.split}{'/' + self.sub if self.sub else ''}  first-of-family decisions: {self.n_firsts}, judged: {self.n_judged}",
            f"coverage (decided / judged): {self.decided.text()}",
            f"precision on decided:       {self.precision.text()}   meets 0.95 (CI lower >= 0.90): {self.precision.meets()}",
            f"baseline recommendation on the same decided subset: {self.baseline_on_decided.text()}",
            "paired (decided entries with a recommendation): " + ", ".join(f"{k}={v}" for k, v in self.paired.items()),
            "holds by class: " + (", ".join(f"{k}={v}" for k, v in sorted(self.holds.items())) or "none"),
            "other-picks: " + ", ".join(f"{k}={v}" for k, v in self.other.items()),
            "already settled (repeats, excluded from the headline): " + ", ".join(f"{k}={v}" for k, v in self.repeats.items()),
        ]
        if self.structure:
            c = self.structure
            lines.append(
                f"structural evidence over {c['asks']} asks / {c['options']} non-instruction options: reversible true={c['reversible_true']} "
                f"false={c['reversible_false']} absent={c['reversible_absent']} (not a hold, not a permission); card move blocks={c['card_move']}; "
                f"root-run blocks: {c['root_run_structured']} in data.db (text signal on {c['root_run_text_signal']})")
        lines.append("coverage-precision curve (confidence >= t, citation-backed):")
        for p in self.curve:
            lines.append(f"  t={p['t']:.2f}  coverage={p['decided']}/{p['judged']}  precision={Ratio(p['correct'], p['decided']).text()}")
        return "\n".join(lines)


CURVE_T = (0.5, 0.6, 0.7, 0.8, 0.85, 0.9, 0.95, 0.99)


def curve(pairs: list[tuple[dict, Entry]], ts: Iterable[float] = CURVE_T) -> list[dict]:
    """Coverage and precision as the confidence floor rises, from the model's own choice.
    Only rows that reached the model (they carry a choice and a confidence) and that a citable
    ruling backs (`cite_ok`) can be on the curve: a no-cite row is never a decision at any threshold.
    A row without the field (written before it existed) is left off rather than assumed eligible."""
    out = []
    scored = [(r, e) for r, e in pairs if r.get("confidence") is not None and r.get("choice") and r.get("cite_ok") is True]
    for t in ts:
        picked = [(r, e) for r, e in scored if r["confidence"] >= t and r["choice"] not in ("send_back", "escalate")]
        ok = sum(1 for r, e in picked if e.pick_kind == "option" and r["choice"] == e.pick_option)
        out.append({"t": t, "judged": len(pairs), "decided": len(picked), "correct": ok})
    return out


def evaluate(manifest: Manifest, results: Iterable[dict], config_hash: str, mode: str, split: str, sub: str | None = None,
             structure: dict | None = None) -> Evaluation:
    rows = rows_for(results, manifest, config_hash, mode)
    firsts = manifest.firsts(split, sub)
    pairs = [(rows[e.decision_id], e) for e in firsts if e.decision_id in rows]
    decided = [(r, e) for r, e in pairs if r.get("verdict") == "decided"]
    ok = [(r, e) for r, e in decided if correct(r, e)]
    holds = Counter(hold_class(r.get("reason")) for r, _ in pairs if r.get("verdict") == "held")

    withrec = [(r, e) for r, e in decided if r.get("recommendation")]
    paired = Counter()
    for r, e in withrec:
        j, b = correct(r, e), baseline_correct(r, e)
        paired["both" if j and b else "judge_only" if j else "baseline_only" if b else "neither"] += 1
    base_k = sum(1 for r, e in decided if baseline_correct(r, e))
    base_n = sum(1 for r, e in decided if r.get("recommendation"))

    others = [(r, e) for r, e in pairs if e.pick_kind == "other"]
    other = {
        "total": len(others),
        "held": sum(1 for r, _ in others if r.get("verdict") == "held"),
        "sent_back": sum(1 for r, _ in others if r.get("verdict") == "sent_back"),
        "decided_wrongly": sum(1 for r, _ in others if r.get("verdict") == "decided"),
    }

    reps = [(rows[e.decision_id], e) for e in manifest.repeats(split) if e.decision_id in rows]
    repeats = {"total": len(reps), "decided": sum(1 for r, _ in reps if r.get("verdict") == "decided"),
               "correct": sum(1 for r, e in reps if correct(r, e))}
    return Evaluation(
        split=split, sub=sub, n_firsts=len(firsts), n_judged=len(pairs), decided=Ratio(len(decided), len(pairs)),
        precision=Ratio(len(ok), len(decided)), holds=holds, paired=dict(paired), curve=curve(pairs), other=other,
        repeats=repeats, baseline_on_decided=Ratio(base_k, base_n), structure=structure,
    )


def ablation(manifest: Manifest, without: Iterable[dict], with_rec: Iterable[dict], cfg_without: str, cfg_with: str, mode: str = "backtest-dev") -> dict:
    """Recommendation inclusion, dev set only. The test split is never used to choose it."""
    a = evaluate(manifest, without, cfg_without, mode, "dev")
    b = evaluate(manifest, with_rec, cfg_with, mode, "dev")
    return {"without_recommendation": a.as_dict(), "with_recommendation": b.as_dict()}


# ---- live (prospective) verdicts ---------------------------------------------------

def live_report(manifest: Manifest, results: Iterable[dict], config_hash: str, mode: str = "shadow") -> dict:
    """Only verdicts logged before mk's pick count. Later ones are retrospective, not evidence."""
    rows = rows_for(results, manifest, config_hash, mode, same_manifest=False)
    decided_total = sum(1 for r in rows.values() if r.get("verdict") == "decided")
    prospective = retrospective = unresolved = 0
    ok = n = 0
    for did, r in rows.items():
        e = manifest.by_id(did)
        if e is None:
            unresolved += 1
            continue
        if parse_ts(r["ts"]) >= parse_ts(e.picked_at):
            retrospective += 1
            continue
        prospective += 1
        if r.get("verdict") == "decided":
            n += 1
            ok += 1 if correct(r, e) else 0
    return {"logged": len(rows), "decided": decided_total, "unresolved": unresolved, "retrospective": retrospective,
            "prospective": prospective, "precision": Ratio(ok, n)}


# ---- the semantic-equivalence audit sample ----------------------------------------

def other_audit_sample(manifest: Manifest, n: int = 40) -> list[Entry]:
    """A reproducible sample of `other` picks (raw text kept) for a human to mark as equivalent
    to an option or not. Ordered by a hash of the manifest id and decision id."""
    others = [e for e in manifest.entries if e.pick_kind == "other"]
    others.sort(key=lambda e: hashlib.sha256(f"{manifest.id}:{e.decision_id}".encode()).hexdigest())
    return others[:n]
