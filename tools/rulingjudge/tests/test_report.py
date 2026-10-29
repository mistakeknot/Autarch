"""Metrics with known answers. All data synthetic."""

import math

import pytest

from tools.rulingjudge import report as rp
from tools.rulingjudge.manifest import Entry, Manifest


def entry(i, pick="a", kind="option", first=True, split="dev", sub="tune", picked="2026-03-01T00:00:00Z", fam=None):
    return Entry(decision_id=f"d{i}", revision="1", filed_at=f"2026-02-{(i % 28) + 1:02d}T00:00:00Z", pick_option=pick, pick_kind=kind,
                 pick_reason="raw words" if kind == "other" else None, picked_at=picked, body_sha256="x", family=fam or f"f{i}", first=first,
                 family_cards=(), delegable=1, split=split, sub=sub)


def row(i, verdict="decided", option="a", conf=0.95, rec=None, reason=None, choice=None, mode="m", cfg="c", ts="2026-02-15T00:00:00Z", cite_ok=True):
    return {"cite_ok": cite_ok, "decision_id": f"d{i}", "manifest_id": "M", "config_hash": cfg, "mode": mode, "verdict": verdict, "option": option,
            "confidence": conf, "recommendation": rec, "reason": reason or verdict, "choice": choice or option, "ts": ts}


def mani(entries):
    return Manifest(id="M", label_cutoff="x", entries=entries)


def ev(entries, rows, **kw):
    return rp.evaluate(mani(entries), rows, "c", "m", "dev", **kw)


# ---- Wilson -----------------------------------------------------------------------

def test_wilson_known_values():
    lo, hi = rp.wilson(95, 100)
    assert lo == pytest.approx(0.8882, abs=1e-3) and hi == pytest.approx(0.9784, abs=1e-3)
    lo, hi = rp.wilson(0, 10)
    assert lo == 0.0 and hi == pytest.approx(0.2775, abs=1e-3)
    lo, hi = rp.wilson(10, 10)
    assert hi == pytest.approx(1.0) and lo == pytest.approx(0.7225, abs=1e-3)
    assert rp.wilson(0, 0) == (0.0, 1.0)


def test_meets_needs_lower_bound_not_just_point():
    assert not rp.Ratio(19, 20).meets()  # point 0.95, CI lower ~0.76
    assert not rp.Ratio(95, 100).meets()  # lower 0.888 < 0.90
    assert rp.Ratio(190, 200).meets()  # lower ~0.91
    assert not rp.Ratio(0, 0).meets()
    assert not rp.Ratio(90, 100).meets()  # point below target


# ---- evaluation -------------------------------------------------------------------

def test_coverage_precision_counts():
    es = [entry(i, "a") for i in range(10)]
    rows = [row(i, "decided", "a") for i in range(6)] + [row(6, "decided", "b"), row(7, "held", None, reason="low_confidence"),
            row(8, "held", None, reason="gate:merge"), row(9, "sent_back", None, reason="send_back")]
    r = ev(es, rows)
    assert (r.decided.k, r.decided.n) == (7, 10)
    assert (r.precision.k, r.precision.n) == (6, 7)
    assert r.holds == {"low_confidence": 1, "gate:merge": 1}


def test_only_first_of_family_in_headline_and_repeats_separate():
    es = [entry(1, "a"), entry(2, "a", first=False, fam="f1"), entry(3, "a", first=False, fam="f1")]
    rows = [row(1, "decided", "a"), row(2, "decided", "a"), row(3, "decided", "b")]
    r = ev(es, rows)
    assert (r.precision.k, r.precision.n) == (1, 1)
    assert r.repeats == {"total": 2, "decided": 2, "correct": 1}


def test_paired_comparison_and_baseline_on_decided():
    es = [entry(i, "a") for i in range(1, 6)]
    rows = [
        row(1, "decided", "a", rec="a"),  # both
        row(2, "decided", "a", rec="b"),  # judge only
        row(3, "held", None, rec="a", reason="low_confidence"),  # baseline only
        row(4, "decided", "b", rec="b"),  # neither
        row(5, "decided", "a", rec=None),  # no recommendation: out of the pairing
    ]
    r = ev(es, rows)
    # A hold or a send-back is not the judge being wrong, so it adds nothing to either side of the pairing.
    assert r.paired == {"both": 1, "judge_only": 1, "neither": 1}
    assert (r.baseline_on_decided.k, r.baseline_on_decided.n) == (1, 3)  # decided with rec: d1 ok, d2 wrong, d4 wrong


def test_held_and_sent_back_rows_do_not_enter_the_paired_comparison():
    es = [entry(i, "a") for i in range(1, 4)]
    rows = [row(1, "held", None, rec="a", reason="gate:merge"), row(2, "sent_back", None, rec="a"), row(3, "held", None, rec="b", reason="no-cite")]
    r = ev(es, rows)
    assert r.paired == {} and (r.baseline_on_decided.k, r.baseline_on_decided.n) == (0, 0)


def test_curve_leaves_off_rows_without_a_citable_ruling():
    es = [entry(i, "a") for i in range(3)]
    rows = [row(0, "held", None, conf=0.97, choice="a", reason="no-cite", cite_ok=False),
            row(1, "decided", "a", conf=0.96),
            {k: v for k, v in row(2, "decided", "a", conf=0.96).items() if k != "cite_ok"}]  # older row, no field
    c = rp.curve(list(zip(rows, es)))
    assert all(p["decided"] == 1 and p["correct"] == 1 for p in c if p["t"] <= 0.95)
    assert c[0]["judged"] == 3


def test_structure_coverage_counts_reversible_values_move_and_root_run():
    from tools.rulingjudge.ask import parse_ask
    import json
    opts = [{"id": "a", "kind": "ruling-only", "label": "A", "reversible": True},
            {"id": "b", "kind": "ruling-only", "label": "B"},
            {"id": "c", "kind": "instruction", "label": "C"}]
    body = json.dumps({"question": "q", "subject": "s", "project": "p", "options": opts})
    asks = [parse_ask("d1", "1", body, "2026-02-01T00:00:00Z", 1, True), None]
    c = rp.structure_coverage(asks)
    assert c["asks"] == 1 and c["options"] == 2
    assert (c["reversible_true"], c["reversible_false"], c["reversible_absent"]) == (1, 0, 1)
    assert c["card_move"] == 1 and c["root_run_structured"] == "unestablished"
    r = ev([entry(1, "a")], [row(1)], structure=c)
    assert r.as_dict()["structure"] == c and "reversible true=1" in r.text() and "unestablished" in r.text()


def test_other_picks_held_vs_sent_back_reported_separately():
    es = [entry(1, "other", "other"), entry(2, "other", "other"), entry(3, "other", "other"), entry(4, "other", "other")]
    rows = [row(1, "held", None, reason="gate:credential"), row(2, "sent_back", None), row(3, "decided", "a"), row(4, "held", None, reason="low_confidence")]
    r = ev(es, rows)
    assert r.other == {"total": 4, "held": 2, "sent_back": 1, "decided_wrongly": 1}
    assert (r.precision.k, r.precision.n) == (0, 1)  # a decided option on an `other` pick is wrong


def test_curve_is_monotone_and_uses_model_choice():
    es = [entry(i, "a") for i in range(8)]
    confs = [0.99, 0.97, 0.95, 0.92, 0.85, 0.75, 0.65, 0.55]
    rows = [row(i, "decided" if c >= 0.9 else "held", "a" if c >= 0.9 else None, conf=c, choice="a" if i != 3 else "b", reason="low_confidence") for i, c in enumerate(confs)]
    c = {p["t"]: p for p in rp.curve([(r, e) for r, e in zip(rows, es)])}
    assert c[0.5]["decided"] == 8 and c[0.5]["correct"] == 7
    assert c[0.9]["decided"] == 4 and c[0.9]["correct"] == 3
    assert c[0.99]["decided"] == 1 and c[0.99]["correct"] == 1
    dec = [p["decided"] for p in sorted(c.values(), key=lambda p: p["t"])]
    assert dec == sorted(dec, reverse=True)


def test_curve_ignores_send_back_and_escalate_choices():
    es = [entry(1, "a"), entry(2, "a")]
    rows = [row(1, "sent_back", None, conf=0.99, choice="send_back"), row(2, "held", None, conf=0.99, choice="escalate")]
    assert all(p["decided"] == 0 for p in rp.curve(list(zip(rows, es))))


def test_results_from_other_configs_modes_or_manifests_are_ignored():
    es = [entry(1, "a")]
    other_cfg = row(1, "decided", "a", cfg="other")
    other_mode = row(1, "decided", "a", mode="shadow")
    other_man = {**row(1, "decided", "a"), "manifest_id": "Z"}
    r = ev(es, [other_cfg, other_mode, other_man])
    assert r.n_judged == 0
    assert ev(es, [row(1, "decided", "a")]).n_judged == 1


def test_sub_split_filter():
    es = [entry(1, "a", sub="tune"), entry(2, "a", sub="validation")]
    rows = [row(1), row(2)]
    assert ev(es, rows, sub="tune").n_judged == 1
    assert ev(es, rows, sub="validation").n_judged == 1
    assert ev(es, rows).n_judged == 2


def test_empty_is_safe():
    r = ev([], [])
    assert r.n_judged == 0 and r.precision.point is None and "n/a" in r.text()


def test_ablation_uses_dev_only():
    es = [entry(1, "a"), entry(2, "a", split="test", sub=None)]
    a = [row(1, cfg="c0"), row(2, cfg="c0")]
    b = [row(1, "decided", "b", cfg="c1"), row(2, cfg="c1")]
    out = rp.ablation(mani(es), a, b, "c0", "c1", mode="m")
    assert out["without_recommendation"]["judged"] == 1 and out["with_recommendation"]["judged"] == 1
    assert out["without_recommendation"]["precision"] == [1, 1] and out["with_recommendation"]["precision"] == [0, 1]


# ---- live ---------------------------------------------------------------------------

def test_live_counts_only_prospective_and_shows_unresolved():
    es = [entry(1, "a", picked="2026-03-01T00:00:00Z"), entry(2, "a", picked="2026-03-01T00:00:00Z"), entry(3, "a", picked="2026-03-01T00:00:00Z")]
    rows = [
        row(1, "decided", "a", mode="shadow", ts="2026-02-28T00:00:00Z"),  # before the pick: prospective, right
        row(2, "decided", "b", mode="shadow", ts="2026-03-02T00:00:00Z"),  # after the pick: retrospective
        row(3, "decided", "b", mode="shadow", ts="2026-02-28T00:00:00Z"),  # prospective, wrong
        row(9, "decided", "a", mode="shadow", ts="2026-02-28T00:00:00Z"),  # no pick yet
    ]
    out = rp.live_report(mani(es), rows, "c")
    assert (out["prospective"], out["retrospective"], out["unresolved"]) == (2, 1, 1)
    assert (out["precision"].k, out["precision"].n) == (1, 2)


# ---- audit sample -------------------------------------------------------------------

def test_audit_sample_is_reproducible_and_other_only():
    es = [entry(i, "other", "other") for i in range(60)] + [entry(100 + i, "a") for i in range(10)]
    m = mani(es)
    s1, s2 = rp.other_audit_sample(m), rp.other_audit_sample(m)
    assert [e.decision_id for e in s1] == [e.decision_id for e in s2] and len(s1) == 40
    assert all(e.pick_kind == "other" and e.pick_reason == "raw words" for e in s1)
    s3 = rp.other_audit_sample(Manifest(id="N", label_cutoff="x", entries=es))
    assert [e.decision_id for e in s3] != [e.decision_id for e in s1]  # a new manifest draws a new sample
