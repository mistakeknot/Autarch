"""Command line behaviour against a synthetic data.db and ledger, a real selector and a fake transport.

Every fixture here is synthetic. No credential is loaded and nothing leaves the process.
"""

import glob
import io
import json
import os
import sqlite3
import stat
from datetime import datetime, timedelta, timezone

import pytest

from tools.rulingjudge import cli, judge, manifest as mf, report as rp, sel as selmod
from tools.rulingjudge.store import iso, parse_ts
from tools.rulingjudge.tests.test_manifest import T0, make_db

ROOT = os.environ.get("CLAVAIN_SELECTED_ROOT") or os.path.expanduser("~/.claude/plugins/cache/interagency-marketplace/clavain/0.6.331")
try:
    SEL = selmod.load(ROOT)
except selmod.SelectorUnavailable:  # pragma: no cover
    pytest.skip("selector unavailable", allow_module_level=True)
JV = SEL.jev


def ledger_file(path):
    rows = []
    for i, text in enumerate(["ship small docs fixes without a gate", "colour palette changes are cosmetic", "synthetic question handling rule"]):
        rows.append({"id": f"r{i}", "ts": (T0 - timedelta(days=30 - i)).isoformat().replace("+00:00", "Z"), "by": "mk", "ruling": text})
    with open(path, "w") as fh:
        for r in rows:
            fh.write(json.dumps(r) + "\n")
    return str(path)


def body(n):
    return json.dumps({"question": f"synthetic question {n}: should docs fixes ship?", "subject": f"s{n}", "project": "p", "recommendation": "a",
                       "options": [{"id": "a", "kind": "ruling-only", "label": "Ship small docs fixes"},
                                   {"id": "b", "kind": "ruling-only", "label": "Colour palette change"}]})


def specs(n=10):
    return [{"id": f"d{i}", "day": i * 2, "pick": "a" if i % 2 else "b", "body": body(i)} for i in range(n)]


@pytest.fixture
def env(tmp_path, monkeypatch):
    monkeypatch.setenv("CLAVAIN_SELECTED_ROOT", ROOT)
    monkeypatch.delenv("RULINGJUDGE_DB", raising=False)
    db = make_db(str(tmp_path / "data.db"), specs())
    led = ledger_file(tmp_path / "rulings.jsonl")
    state = str(tmp_path / "state")
    return {"db": db, "ledger": led, "state": state, "tmp": tmp_path}


def run(env, *args, factory=None):
    out = io.StringIO()
    code = cli.main(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", env["state"], *args], transport_factory=factory, out=out)
    return code, out.getvalue()


def no_factory(_sel):
    raise AssertionError("the transport factory must not run")


def probs_for_call(call, winner, p=0.95):
    ids = [cid for cid, _ in call.battery.select.criteria]
    rest = (1 - p) / (len(ids) - 1)
    d = {i: rest for i in ids}
    d[winner] = p
    return d


def ok_response(call, winner="a", p=0.95):
    probs = probs_for_call(call, winner, p)
    return JV.JevOk(response=JV.JevResponse(
        model=JV.PINNED_MODEL,
        select=JV.ChoiceAnswer(choice=winner, confidence=p, probabilities=tuple(probs.items())),
        fits=(), usage=JV.Usage(input_tokens=100, output_tokens=5)), attempts=1, http_status=200, latency_ms=12)


def http_failure(status):
    return JV.JevFailure(reason=SEL.contract.FallbackReason.HTTP_ERROR, detail=JV.FailureDetail.STATUS, attempts=1, http_status=status, latency_ms=3)


class FakeFactory:
    def __init__(self, result):
        self.result = result
        self.calls = []

    def __call__(self, _sel):
        def transport(call):
            self.calls.append(call)
            return self.result(call)
        return transport


def fake_factory(result=ok_response):
    return FakeFactory(result)


def results(env):
    path = os.path.join(env["state"], "results.jsonl")
    if not os.path.exists(path):
        return []
    return [json.loads(line) for line in open(path)]


def mode_of(path):
    return stat.S_IMODE(os.stat(path).st_mode)


# ---- dry run ----------------------------------------------------------------------

def test_dry_run_builds_requests_and_calls_nothing(env):
    code, out = run(env, "backtest", "--split", "dev", "--dry-run", factory=no_factory)
    assert code == 0
    assert "would send" in out and "holds" in out and "first-of-family=10" in out
    assert results(env) == []


def test_dry_run_files_are_private_and_inside_the_state_dir(env):
    code, _ = run(env, "backtest", "--split", "dev", "--dry-run", "--keep-requests", factory=no_factory)
    assert code == 0
    folder = os.path.join(env["state"], "requests")
    files = os.listdir(folder)
    assert files and all(mode_of(os.path.join(folder, f)) == 0o600 for f in files)
    assert mode_of(env["state"]) == 0o700 and mode_of(folder) == 0o700
    assert all(json.loads(open(os.path.join(folder, f)).read()) for f in files)


def test_limit_applies(env):
    code, out = run(env, "backtest", "--split", "dev", "--limit", "2", "--dry-run", factory=no_factory)
    assert code == 0 and "items=2" in out


def test_source_files_are_not_modified(env):
    before = [open(env[k], "rb").read() for k in ("db", "ledger")]
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    run(env, "report", "--split", "dev")
    assert before == [open(env[k], "rb").read() for k in ("db", "ledger")]


# ---- live backtest -----------------------------------------------------------------

def test_backtest_writes_terminal_rows_and_is_idempotent(env):
    f = fake_factory()
    code, out = run(env, "backtest", "--split", "dev", factory=f)
    assert code == 0
    rows = results(env)
    first_count = len(rows)
    assert first_count == len(f.calls) + sum(1 for r in rows if r["verdict"] == "held")
    assert {r["mode"] for r in rows} == {"backtest-dev"}
    for r in rows:
        assert r["config_hash"] and r["manifest_id"]
    sent = [r for r in rows if r["verdict"] != "held"]
    assert sent and all(r["corpus_hash"] and r["cand_hash"] is not None for r in sent)
    calls = len(f.calls)
    code, out = run(env, "backtest", "--split", "dev", factory=f)
    assert code == 0 and len(results(env)) == first_count and len(f.calls) == calls


def test_result_rows_carry_the_model_choice(env):
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    decided = [r for r in results(env) if r["verdict"] == "decided"]
    assert decided and all(r["choice"] == "a" and r["option"] == "a" for r in decided)


def test_credential_unavailable_stops_with_code_3(env):
    def factory(_sel):
        raise cli.CliError("credential unavailable; no request sent", 3)

    code, _ = run(env, "backtest", "--split", "dev", factory=factory)
    assert code == 3
    assert results(env) == []


def test_operational_failure_logs_an_attempt_and_the_rerun_waits(env):
    def down(call):
        return http_failure(503)

    f = fake_factory(down)
    code, out = run(env, "backtest", "--split", "dev", "--limit", "3", factory=f)
    assert code == 0
    attempts_path = os.path.join(env["state"], "attempts.jsonl")
    attempts = [json.loads(line) for line in open(attempts_path)]
    assert attempts and all(a["operational"] for a in attempts)
    assert all(r["verdict"] == "held" for r in results(env))  # only structural holds are terminal
    calls = len(f.calls)
    code, out = run(env, "backtest", "--split", "dev", "--limit", "3", factory=f)
    assert len(f.calls) == calls and "waiting_backoff" in out


def test_held_decisions_cost_no_call(env):
    f = fake_factory()
    run(env, "backtest", "--split", "dev", factory=f)
    held = [r for r in results(env) if r["verdict"] == "held"]
    assert len(f.calls) == len(results(env)) - len(held)


# ---- test split gate ---------------------------------------------------------------

def test_test_split_is_refused_without_a_freeze(env):
    code, _ = run(env, "backtest", "--split", "test", "--dry-run", factory=no_factory)
    assert code == 5


def test_test_split_runs_after_freeze_and_refuses_a_changed_config(env):
    code, out = run(env, "freeze")
    assert code == 0 and "frozen" in out
    code, out = run(env, "backtest", "--split", "test", "--dry-run", factory=no_factory)
    assert code == 0
    code, _ = run(env, "backtest", "--split", "test", "--t", "0.8", "--dry-run", factory=no_factory)
    assert code == 5


def test_a_second_freeze_with_a_different_config_is_refused_once_test_results_exist(env):
    run(env, "freeze")
    run(env, "backtest", "--split", "test", factory=fake_factory())
    assert any(r["mode"] == "backtest-test" for r in results(env))
    code, _ = run(env, "freeze", "--t", "0.8")
    assert code == 5


# ---- shadow ------------------------------------------------------------------------

@pytest.fixture
def shadow_env(env):
    days_ago = (datetime.now(timezone.utc) - T0).days - 1
    env["db"] = make_db(str(env["tmp"] / "shadow.db"), specs(4) + [{"id": "s1", "day": days_ago, "body": body(91)}, {"id": "s2", "day": days_ago, "pick": "a", "body": body(92)}])
    return env


def test_shadow_requires_once(env):
    code, _ = run(env, "shadow", factory=no_factory)
    assert code == 2


def test_shadow_dry_run_counts_only_recent_unpicked(shadow_env):
    code, out = run(shadow_env, "shadow", "--once", "--dry-run", factory=no_factory)
    assert code == 0 and "shadow: 1 unpicked" in out
    assert results(shadow_env) == []


def test_shadow_logs_verdicts_in_shadow_mode_only(shadow_env):
    f = fake_factory()
    code, _ = run(shadow_env, "shadow", "--once", factory=f)
    assert code == 0
    rows = results(shadow_env)
    assert [r["decision_id"] for r in rows] == ["s1"] and rows[0]["mode"] == "shadow"


# ---- report and paths --------------------------------------------------------------

def test_report_prints_metrics_and_audit_sample(env):
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    code, out = run(env, "report", "--split", "dev", "--audit-sample", "3")
    assert code == 0 and "coverage" in out and "precision" in out and "audit sample" in out


def test_report_json_and_out_file(env):
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    code, out = run(env, "report", "--split", "dev", "--json", "--out", "report-dev.json")
    assert code == 0
    path = os.path.join(env["state"], "report-dev.json")
    assert mode_of(path) == 0o600
    assert json.loads(open(path).read())["split"] == "dev"


def test_report_ablation_and_live_run(env):
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    assert run(env, "report", "--ablation")[0] == 0
    assert run(env, "report", "--live")[0] == 0


def test_report_out_cannot_escape_the_state_dir(env):
    code, _ = run(env, "report", "--split", "dev", "--out", "../escaped.txt")
    assert code == 2
    assert not os.path.exists(os.path.join(os.path.dirname(env["state"]), "escaped.txt"))


def test_report_out_refuses_a_symlink_to_the_database(env):
    os.makedirs(env["state"], exist_ok=True)
    os.symlink(env["db"], os.path.join(env["state"], "alias.txt"))
    before = open(env["db"], "rb").read()
    code, _ = run(env, "report", "--split", "dev", "--out", "alias.txt")
    assert code == 2 and open(env["db"], "rb").read() == before


def test_state_dir_that_is_the_ledger_is_refused(env):
    out = io.StringIO()
    code = cli.main(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", env["ledger"], "report"], out=out)
    assert code == 2


# ---- failing closed -----------------------------------------------------------------

def test_missing_selector_sends_nothing(env, monkeypatch):
    monkeypatch.delenv("CLAVAIN_SELECTED_ROOT")
    code, _ = run(env, "backtest", "--split", "dev", factory=no_factory)
    assert code == 2 and results(env) == []


def test_missing_database_is_a_usage_error(env):
    out = io.StringIO()
    code = cli.main(["--ledger", env["ledger"], "--state-dir", env["state"], "report"], out=out)
    assert code == 2


def test_default_factory_is_never_used_by_the_tests():
    assert cli.default_transport_factory.__module__.endswith("cli")
    assert judge.CITE_METHOD == "bm25-post"


# ---- review fixes: replay drift, cohort, backdating, moves, state dir, provenance, attempts ----

def namespace(env, *args):
    return cli.parser().parse_args(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", env["state"], *args])


def saved_manifest_id(env):
    (path,) = glob.glob(os.path.join(env["state"], "manifest-*.json"))
    return json.load(open(path))["id"]


def edit_db(env, sql, *params):
    c = sqlite3.connect(env["db"])
    c.execute(sql, params)
    c.commit()
    c.close()


@pytest.mark.parametrize("column,value", [
    ("body_json", body(77)),
    ("revision", "2"),
    ("filed_at", "2026-01-01T00:00:00.000Z"),
    ("delegable", 0),
])
def test_replaying_a_saved_manifest_refuses_a_drifted_ask_before_any_send(env, column, value):
    run(env, "backtest", "--split", "dev", "--dry-run", factory=no_factory)
    mid = saved_manifest_id(env)
    edit_db(env, f"UPDATE decisions SET {column}=? WHERE id='d0'", value)
    f = fake_factory()
    code, _ = run(env, "backtest", "--split", "dev", "--manifest", mid, factory=f)
    assert code == 6 and f.calls == [] and results(env) == []
    code, _ = run(env, "backtest", "--split", "dev", "--manifest", mid, "--dry-run", factory=no_factory)
    assert code == 6


def test_replaying_an_unchanged_manifest_is_allowed(env):
    run(env, "backtest", "--split", "dev", "--dry-run", factory=no_factory)
    mid = saved_manifest_id(env)
    f = fake_factory()
    code, _ = run(env, "backtest", "--split", "dev", "--manifest", mid, factory=f)
    assert code == 0 and f.calls


def test_a_later_manifest_cannot_retune_an_already_frozen_test_cohort(env):
    assert run(env, "freeze")[0] == 0
    env["db"] = make_db(str(env["tmp"] / "more.db"), specs(11))  # a later pick moves the cutoff: another manifest, same test decisions
    code, _ = run(env, "freeze", "--t", "0.8")
    assert code == 5
    code, _ = run(env, "backtest", "--split", "test", "--t", "0.8", "--dry-run", factory=no_factory)
    assert code == 5
    code, out = run(env, "freeze")  # the same configuration is a retry, not a retune
    assert code == 0 and "frozen" in out


class Clock:
    def __init__(self, now):
        self.now = now

    def __call__(self):
        return self.now


def test_a_row_written_after_the_pick_is_stamped_after_the_pick(env):
    ns = namespace(env, "report")
    ctx = cli.load_ctx(ns)
    m = cli.get_manifest(ctx, ns)
    entry = m.by_id("d1")
    picked = parse_ts(entry.picked_at)
    clock = Clock(picked - timedelta(hours=1))

    def slow(_sel):
        def transport(call):
            clock.now = picked + timedelta(hours=1)  # the pick lands while the provider is thinking
            return ok_response(call)
        return transport

    item = cli.Item("d1", parse_ts(entry.filed_at), frozenset(entry.family_cards), None)
    cli.run_live(ctx, [item], "shadow", m.id, slow, False, clock=clock)
    (row,) = results(env)
    assert parse_ts(row["ts"]) > picked
    live = rp.live_report(m, ctx.store.results(), ctx.cfg_hash)
    assert live["retrospective"] == 1 and live["prospective"] == 0


def test_a_card_move_decision_is_held_and_costs_no_call(env):
    sp = specs(6)
    sp[2]["move"] = ("home", "card")
    env["db"] = make_db(str(env["tmp"] / "move.db"), sp)
    code, out = run(env, "backtest", "--split", "dev", "--dry-run", factory=no_factory)
    assert code == 0 and "move-block" in out
    f = fake_factory()
    run(env, "backtest", "--split", "dev", factory=f)
    assert all(c is not None for c in f.calls)
    moved = [r for r in results(env) if r["decision_id"] == "d2"]
    assert all(r["verdict"] == "held" and "move-block" in r["reason"] for r in moved)


def test_a_pick_opened_move_does_not_hold(env):
    sp = specs(6)
    sp[2]["move"] = ("home", "pick")
    env["db"] = make_db(str(env["tmp"] / "pickmove.db"), sp)
    code, out = run(env, "backtest", "--split", "dev", "--dry-run", factory=no_factory)
    assert code == 0 and "move-block" not in out


def test_a_database_without_the_moves_table_stops_with_code_2(env):
    c = sqlite3.connect(env["db"])
    c.execute("DROP TABLE moves")
    c.commit()
    c.close()
    for args in (("backtest", "--split", "dev", "--dry-run"), ("report",), ("freeze",)):
        code, _ = run(env, *args, factory=no_factory)
        assert code == 2
    assert results(env) == []


def test_a_state_dir_outside_the_permitted_root_is_refused_and_created_nowhere(env):
    elsewhere = str(env["tmp"].parent / "elsewhere-judge")
    out = io.StringIO()
    code = cli.main(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", elsewhere, "report"], out=out)
    assert code == 2 and not os.path.exists(elsewhere)


def test_the_state_root_variable_does_not_move_the_permitted_root(env, monkeypatch):
    monkeypatch.undo()  # the production root, not the fixture's
    monkeypatch.setenv("CLAVAIN_SELECTED_ROOT", ROOT)
    monkeypatch.setenv("RULINGJUDGE_STATE_ROOT", str(env["tmp"]))
    out = io.StringIO()
    code = cli.main(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", env["state"], "report"], out=out)
    assert code == 2 and not os.path.exists(env["state"])


def test_a_state_dir_symlinked_out_of_the_permitted_root_is_refused(env):
    outside = env["tmp"].parent / f"{env['tmp'].name}-outside"
    outside.mkdir()
    try:
        link = env["tmp"] / "linked"
        os.symlink(outside, link)
        out = io.StringIO()
        code = cli.main(["--db", env["db"], "--ledger", env["ledger"], "--state-dir", str(link), "report"], out=out)
        assert code == 2 and os.listdir(outside) == []
    finally:
        os.rmdir(outside)


def test_result_rows_say_whether_a_ruling_backs_the_choice(env):
    run(env, "backtest", "--split", "dev", factory=fake_factory())
    sent = [r for r in results(env) if r.get("choice")]
    assert sent and all("cite_ok" in r for r in sent)
    assert all(r["cite_ok"] is True for r in sent if r["verdict"] == "decided")


def test_provider_exhaustion_is_a_terminal_held_row_with_provenance_and_counts_in_the_report(env):
    ns = namespace(env, "backtest", "--split", "dev")
    ctx = cli.load_ctx(ns)
    m = cli.get_manifest(ctx, ns)
    (item,) = cli.split_items(m, "dev", 1)
    clock = Clock(datetime(2027, 1, 1, tzinfo=timezone.utc))
    down = fake_factory(lambda call: http_failure(503))
    t1 = cli.run_live(ctx, [item], "backtest-dev", m.id, down, False, clock=clock)
    assert t1["operational_error"] == 1 and results(env) == []
    clock.now += timedelta(hours=25)
    t2 = cli.run_live(ctx, [item], "backtest-dev", m.id, down, False, clock=clock)
    assert t2["gave_up"] == 1 and len(down.calls) == 1
    (row,) = results(env)
    assert row["verdict"] == "held" and row["reason"].startswith("provider-unavailable")
    assert row["manifest_id"] == m.id and row["split"] == "dev" and row["revision"] == "1"
    ev = rp.evaluate(m, ctx.store.results(), ctx.cfg_hash, "backtest-dev", "dev")
    assert ev.n_judged == 1 and ev.holds["provider-unavailable"] == 1


def test_a_completed_call_logs_one_attempt_before_its_terminal_row(env):
    f = fake_factory()
    run(env, "backtest", "--split", "dev", "--limit", "3", factory=f)
    attempts = [json.loads(line) for line in open(os.path.join(env["state"], "attempts.jsonl"))]
    rows = {r["decision_id"]: r for r in results(env)}
    assert len(attempts) == len(f.calls) >= 1
    for a in attempts:
        assert a["operational"] is False and a["error_class"] is None
        assert parse_ts(a["ts"]) <= parse_ts(rows[a["decision_id"]]["ts"])


def test_a_non_operational_provider_error_logs_one_attempt_and_one_held_row(env):
    bad = fake_factory(lambda call: http_failure(400))
    run(env, "backtest", "--split", "dev", "--limit", "3", factory=bad)
    attempts = [json.loads(line) for line in open(os.path.join(env["state"], "attempts.jsonl"))]
    sent = [r for r in results(env) if r.get("error_class") == "http_4xx"]
    assert len(attempts) == len(bad.calls) == len(sent) >= 1
    assert all(r["verdict"] == "held" and r["reason"] == "provider-error" for r in sent)
    assert all(a["operational"] is False and a["error_class"] == "http_4xx" for a in attempts)


def test_report_prints_the_structural_evidence_line(env):
    code, out = run(env, "report", "--split", "dev")
    assert code == 0 and "structural evidence" in out and "reversible true=" in out and "root-run blocks: unestablished" in out
