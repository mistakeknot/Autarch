"""judge.py against a fake transport. No network, no key, no credentials module."""

import json
import math
import os
from datetime import datetime, timedelta, timezone

import pytest

from tools.rulingjudge import judge, sel as selmod
from tools.rulingjudge.ask import parse_ask
from tools.rulingjudge.ledger import normalise_lines
from tools.rulingjudge.retrieve import Retriever

ROOT = os.environ.get("CLAVAIN_SELECTED_ROOT") or os.path.expanduser("~/.claude/plugins/cache/interagency-marketplace/clavain/0.6.331")
try:
    SEL = selmod.load(ROOT)
except selmod.SelectorUnavailable:  # pragma: no cover
    pytest.skip("selector not available", allow_module_level=True)

JV, C, Q = SEL.jev, SEL.contract, SEL.questions
T0 = datetime(2026, 1, 1, tzinfo=timezone.utc)
CUT = T0 + timedelta(days=10)
# Synthetic secret shapes, assembled at run time so no literal looks like a credential.
SECRET = "sk-" + "test-" + "0123456789abcdef" + "SECRETKEY"
AWS_SHAPED = "AKIA" + "IOSFODNN7" + "EXAMPLE"
TOKEN_ASSIGN = "tok" + "en=" + "abcdef" + "123456"


def rows():
    raw = []
    for i, (by, text) in enumerate([
        ("mk", "Ruling: ship small docs fixes without a review gate, pick option alpha"),
        ("mk", "Ruling: colour palette changes are cosmetic, choose beta"),
        ("vizier", "Delegated note about alpha docs fixes"),
    ]):
        raw.append(json.dumps({"id": f"r{i}", "ts": (T0 + timedelta(hours=i + 1)).isoformat().replace("+00:00", "Z"), "by": by, "ruling": text}).encode())
    return normalise_lines(raw).rows


def make_ask(options=None, **kw):
    body = {
        "question": "Should the docs fix ship now?",
        "subject": "docs fix",
        "project": "demo",
        "options": options or [
            {"id": "alpha", "kind": "ruling-only", "label": "Ship small docs fixes"},
            {"id": "beta", "kind": "needs-context", "label": "Colour palette change"},
            {"id": "gamma", "kind": "instruction", "label": "Run the thing", "instruction": "do it"},
        ],
        "recommendation": "alpha",
    }
    body.update(kw)
    return parse_ask("d1", "3", json.dumps(body), (T0 + timedelta(days=9)).isoformat())


def retrieval(ask):
    return Retriever(rows()).search(ask.search_text(), CUT)


def response(probs, choice=None, conf=None):
    choice = choice or max(probs, key=probs.get)
    conf = probs[choice] if conf is None else conf
    return JV.JevResponse(
        model=JV.PINNED_MODEL,
        select=JV.ChoiceAnswer(choice=choice, confidence=conf, probabilities=tuple(probs.items())),
        fits=(),
        usage=JV.Usage(input_tokens=100, output_tokens=5),
    )


def fake(result):
    calls = []

    def transport(call):
        calls.append(call)
        return result(call) if callable(result) else result

    transport.calls = calls
    return transport


def ok(probs, **kw):
    return JV.JevOk(response=response(probs, **kw), attempts=1, http_status=200, latency_ms=12)


def fail(reason, detail, status=None):
    return JV.JevFailure(reason=reason, detail=detail, attempts=1, http_status=status, latency_ms=7)


def run(result, cfg=None, ask=None):
    cfg = cfg or judge.JudgeConfig()
    ask = ask or make_ask()
    ret = retrieval(ask)
    prep = judge.prepare(SEL, ask, ret, cfg)
    assert isinstance(prep, judge.Prepared), prep
    return judge.execute(SEL, ask, ret, prep, cfg, fake(result)), prep, ask


def probs_for(prep, winner, p=0.95):
    rest = (1 - p) / (len(prep.cand_ids))
    d = {cid: rest for cid in prep.cand_ids}
    d[Q.ESCALATE_ID] = rest
    d[winner] = p
    return d


def with_probs(winner, p=0.95, **kw):
    return lambda call: ok(probs_for_call(call, winner, p), **kw)


def probs_for_call(call, winner, p):
    ids = [cid for cid, _ in call.battery.select.criteria]
    rest = (1 - p) / (len(ids) - 1)
    d = {i: rest for i in ids}
    d[winner] = p
    return d


# ---- request building -------------------------------------------------------------

def test_candidates_are_ruling_and_needs_context_plus_send_back():
    ask = make_ask()
    prep = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    assert set(prep.cand_ids) == {"alpha", "beta", judge.SEND_BACK}
    assert "gamma" not in prep.id_to_option.values()


def test_option_semantics_reach_the_model():
    ask = make_ask(options=[
        {"id": "alpha", "kind": "ruling-only", "label": "Ship", "reversible": True},
        {"id": "beta", "kind": "needs-context", "label": "Hold", "instruction": "wait for the owner"},
    ])
    prep = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    body = json.loads(prep.admitted.body)
    descs = " ".join(c["description"] for c in body["candidates"])
    assert "wait for the owner" in descs and '"reversible":true' in descs


def test_recommendation_only_when_configured():
    ask = make_ask()
    off = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    on = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig(include_recommendation=True))
    assert b"recommends" not in off.admitted.body
    assert b"recommends" in on.admitted.body


def test_structural_hold_spends_nothing():
    ask = make_ask(options=[{"id": "alpha", "kind": "ruling-only", "label": "Ship", "approval": {"kind": "merge"}},
                            {"id": "beta", "kind": "ruling-only", "label": "No"}])
    out = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    assert isinstance(out, judge.Outcome) and out.verdict == "held" and out.reason == "approval-option"


def test_egress_refusal_is_a_hold_with_rule_ids():
    ask = make_ask(question="the build log shows " + AWS_SHAPED)
    out = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    assert isinstance(out, judge.Outcome) and out.reason.startswith("egress:")
    assert "AKIA" not in repr(out)


def test_no_candidate_options_is_a_hold():
    ask = make_ask(options=[{"id": "g", "kind": "instruction", "label": "x", "instruction": "y"}])
    out = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    assert out.verdict == "held" and out.reason == "no-candidate-options"


def test_awkward_option_ids_get_safe_candidate_ids():
    ask = make_ask(options=[{"id": "has space!", "kind": "ruling-only", "label": "Ship small docs fixes"},
                            {"id": "send_back", "kind": "ruling-only", "label": "Colour palette"}])
    prep = judge.prepare(SEL, ask, retrieval(ask), judge.JudgeConfig())
    assert set(prep.id_to_option.values()) == {"has space!", "send_back"}
    assert all(judge.ID_RE.match(c) for c in prep.cand_ids)
    assert prep.cand_ids.count(judge.SEND_BACK) == 1


# ---- verdicts ----------------------------------------------------------------------

def test_decided_with_cite():
    out, prep, ask = run(with_probs("alpha"))
    assert out.verdict == "decided" and out.option == "alpha"
    assert out.cite_method == "bm25-post"
    assert out.cite_ref["id"] == "r0" and len(out.cite_ref["sha256"]) == 64
    assert out.input_tokens == 100 and out.output_tokens == 5


def test_cite_is_never_a_non_mk_row():
    out, *_ = run(with_probs("alpha"))
    assert out.cite_ref["id"] != "r2"  # r2 is a vizier row: context only


def test_low_confidence_holds():
    out, *_ = run(with_probs("alpha", p=0.6))
    assert out.verdict == "held" and out.reason == "low_confidence"


def test_threshold_boundary():
    cfg = judge.JudgeConfig(t=0.8)
    out, *_ = run(with_probs("alpha", p=0.85), cfg=cfg)
    assert out.verdict == "decided"


def test_send_back_uses_its_own_threshold():
    cfg = judge.JudgeConfig(t=0.5, t_sb=0.95)
    out, *_ = run(with_probs("send_back", p=0.9), cfg=cfg)
    assert out.verdict == "held" and out.reason == "low_confidence"
    out, *_ = run(with_probs("send_back", p=0.97), cfg=cfg)
    assert out.verdict == "sent_back" and out.cite_ref is None


def test_option_threshold_not_loosened_by_lower_t_sb():
    cfg = judge.JudgeConfig(t=0.95, t_sb=0.5)
    out, *_ = run(with_probs("alpha", p=0.9), cfg=cfg)
    assert out.verdict == "held"


def test_escalate_is_held():
    out, *_ = run(with_probs(Q.ESCALATE_ID, p=0.97))
    assert out.verdict == "held" and out.reason == "jev_escalated"


def test_no_citable_row_is_a_hold():
    ask = make_ask(options=[{"id": "zeta", "kind": "ruling-only", "label": "Quantum zebra"},
                            {"id": "eta", "kind": "ruling-only", "label": "Plasma yak"}])
    out, *_ = run(with_probs("zeta"), ask=ask)
    assert out.verdict == "held" and out.reason == "no-cite"


# ---- failure mapping ---------------------------------------------------------------

@pytest.mark.parametrize("reason,detail,status,cls", [
    (C.FallbackReason.RATE_LIMITED, JV.FailureDetail.STATUS, 429, "rate_limited"),
    (C.FallbackReason.HTTP_ERROR, JV.FailureDetail.STATUS, 503, "http_5xx"),
    (C.FallbackReason.HTTP_ERROR, JV.FailureDetail.STATUS, 500, "http_5xx"),
    (C.FallbackReason.TIMEOUT, JV.FailureDetail.DEADLINE, None, "timeout"),
    (C.FallbackReason.HTTP_ERROR, JV.FailureDetail.CONNECT_ERROR, None, "transport"),
])
def test_operational_errors_are_retryable(reason, detail, status, cls):
    out, *_ = run(fail(reason, detail, status))
    assert out.operational and not out.terminal and out.verdict is None
    assert out.error_class == cls and out.http_status == status


@pytest.mark.parametrize("reason,detail,status,cls", [
    (C.FallbackReason.HTTP_ERROR, JV.FailureDetail.STATUS, 400, "http_4xx"),
    (C.FallbackReason.HTTP_ERROR, JV.FailureDetail.STATUS, 404, "http_4xx"),
    (C.FallbackReason.CREDENTIAL_REJECTED, JV.FailureDetail.STATUS, 401, "credential_rejected"),
    (C.FallbackReason.INVALID_RESPONSE, JV.FailureDetail.BAD_JSON, 200, "malformed"),
    (C.FallbackReason.MODEL_MISMATCH, JV.FailureDetail.MODEL, 200, "malformed"),
])
def test_terminal_provider_errors(reason, detail, status, cls):
    out, *_ = run(fail(reason, detail, status))
    assert out.terminal and out.verdict == "held" and out.reason == "provider-error" and out.error_class == cls


def test_transport_exception_is_operational_and_redacted():
    def boom(call):
        raise RuntimeError("connect failed Authorization: Bearer " + SECRET)

    out, *_ = run(boom)
    assert out.operational and out.error_class == "transport"
    assert SECRET not in (out.excerpt or "") and len(out.excerpt) <= 200


def test_unexpected_transport_result_is_terminal_malformed():
    out, *_ = run(lambda call: {"nope": 1})
    assert out.terminal and out.reason == "provider-error" and out.error_class == "malformed"


# ---- response validation -----------------------------------------------------------

def bad_probs(call, mutate):
    d = probs_for_call(call, "alpha", 0.95)
    return mutate(d)


@pytest.mark.parametrize("name,mutate,defect", [
    ("nan", lambda d: {**d, "beta": float("nan")}, "non-finite"),
    ("inf", lambda d: {**d, "beta": math.inf}, "non-finite"),
    ("neg", lambda d: {**d, "beta": -0.2}, "out-of-range"),
    ("big", lambda d: {**d, "beta": 1.5}, "out-of-range"),
    ("missing", lambda d: {k: v for k, v in d.items() if k != "beta"}, "probability-keys"),
    ("extra", lambda d: {**d, "ghost": 0.0}, "probability-keys"),
])
def test_invalid_probabilities_hold(name, mutate, defect):
    out, *_ = run(lambda call: ok(bad_probs(call, mutate), choice="alpha", conf=0.95))
    assert out.terminal and out.reason == "provider-error" and out.excerpt == defect


def test_choice_outside_candidates_holds():
    out, *_ = run(lambda call: ok(probs_for_call(call, "alpha", 0.95), choice="ghost", conf=0.95))
    assert out.reason == "provider-error" and out.excerpt == "choice-outside-candidates"


@pytest.mark.parametrize("conf", [float("nan"), 1.2, -0.1])
def test_bad_confidence_holds(conf):
    out, *_ = run(lambda call: ok(probs_for_call(call, "alpha", 0.95), choice="alpha", conf=conf))
    assert out.reason == "provider-error"


def test_malformed_object_holds():
    class Weird:
        pass

    r = JV.JevOk(response=Weird(), attempts=1, http_status=200, latency_ms=1)
    out, *_ = run(r)
    assert out.reason == "provider-error" and out.error_class == "malformed"


# ---- config, secrets ---------------------------------------------------------------

def test_config_hash_moves_with_every_knob():
    base = judge.JudgeConfig().config_hash(SEL)
    assert base == judge.JudgeConfig().config_hash(SEL)
    for cfg in (judge.JudgeConfig(t=0.8), judge.JudgeConfig(t_sb=0.8), judge.JudgeConfig(include_recommendation=True),
                judge.JudgeConfig(retrieval=judge.Params(k_citable=5))):
        assert cfg.config_hash(SEL) != base


def test_key_never_appears_in_outcomes():
    out, *_ = run(fail(C.FallbackReason.HTTP_ERROR, JV.FailureDetail.STATUS, 400))
    assert SECRET not in json.dumps(out.__dict__, default=str)


def test_selector_load_fails_closed(tmp_path, monkeypatch):
    monkeypatch.delenv("CLAVAIN_SELECTED_ROOT", raising=False)
    with pytest.raises(selmod.SelectorUnavailable):
        selmod.load(None)
    with pytest.raises(selmod.SelectorUnavailable):
        selmod.load(str(tmp_path))


def test_redact_bounds_and_scrubs():
    from tools.rulingjudge.redact import excerpt

    s = excerpt("x" * 500 + " " + TOKEN_ASSIGN + " Bearer " + SECRET)
    assert len(s) <= 200
    assert SECRET not in excerpt("Bearer " + SECRET) and TOKEN_ASSIGN not in excerpt(TOKEN_ASSIGN)
