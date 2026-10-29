import json
import os
import stat
import threading
from datetime import datetime, timedelta, timezone

import pytest

from tools.rulingjudge import store as st

T0 = datetime(2026, 3, 1, 12, 0, tzinfo=timezone.utc)
KEY = st.Key("d1", "shadow", "cfg1")


@pytest.fixture
def s(tmp_path):
    return st.Store(str(tmp_path / "judge"))


def mode(p):
    return stat.S_IMODE(os.stat(p).st_mode)


def test_dirs_and_files_are_private(s):
    with s.locked():
        s.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
        s.write_terminal(KEY, {"verdict": "held"}, T0)
        s.subdir("requests")
    assert mode(s.base) == 0o700 and mode(os.path.join(s.base, "requests")) == 0o700
    for f in ("attempts.jsonl", "results.jsonl", "judge.lock"):
        assert mode(os.path.join(s.base, f)) == 0o600


def test_append_requires_lock(s):
    with pytest.raises(st.NotLocked):
        s.log_attempt(KEY, error_class="x", operational=True)
    with pytest.raises(st.NotLocked):
        s.write_terminal(KEY, {"verdict": "held"})


def test_second_lock_is_refused_not_blocked(s):
    with s.locked():
        other = st.Store(s.base)
        with pytest.raises(st.LockBusy):
            with other.locked():
                pass
    with other.locked():  # released
        pass


def test_lock_is_exclusive_across_threads(s):
    seen = []
    with s.locked():
        def attempt():
            try:
                with st.Store(s.base).locked():
                    seen.append("got")
            except st.LockBusy:
                seen.append("busy")
        t = threading.Thread(target=attempt)
        t.start(); t.join()
    assert seen == ["busy"]


def test_attempts_are_append_only_and_bounded(s):
    with s.locked():
        s.log_attempt(KEY, error_class="http_5xx", operational=True, http_status=503, text="x" * 1000, now=T0)
        s.log_attempt(KEY, error_class="http_5xx", operational=True, http_status=503, text="again", now=T0 + timedelta(minutes=20))
    rows = s.attempts(KEY)
    assert len(rows) == 2 and all(len(r["excerpt"]) <= 200 for r in rows)


def test_excerpt_never_keeps_a_bearer_token(s):
    with s.locked():
        s.log_attempt(KEY, error_class="transport", operational=True, text="Authorization: " + "Bear" + "er " + "abcdefghijklmnop" + "1234567890", now=T0)
    assert "abcdefghijklmnop" not in open(s.attempts_path).read()


def test_one_terminal_row_per_key(s):
    with s.locked():
        assert s.write_terminal(KEY, {"verdict": "decided", "option": "a"}, T0)
        assert not s.write_terminal(KEY, {"verdict": "held", "reason": "later"}, T0)
        assert s.write_terminal(st.Key("d1", "backtest", "cfg1"), {"verdict": "held"}, T0)  # another mode
        assert s.write_terminal(st.Key("d1", "shadow", "cfg2"), {"verdict": "held"}, T0)  # another config
    assert len(s.results()) == 3
    assert [r["verdict"] for r in s.results() if KEY.matches(r)] == ["decided"]


def test_retry_schedule_15m_1h_4h_then_give_up(s):
    with s.locked():
        assert s.due(KEY, T0).action == "try"
        s.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
        assert s.due(KEY, T0 + timedelta(minutes=14)).action == "wait"
        assert s.due(KEY, T0 + timedelta(minutes=15)).action == "try"
        t1 = T0 + timedelta(minutes=15)
        s.log_attempt(KEY, error_class="http_5xx", operational=True, now=t1)
        assert s.due(KEY, t1 + timedelta(minutes=59)).action == "wait"
        assert s.due(KEY, t1 + timedelta(hours=1)).action == "try"
        t2 = t1 + timedelta(hours=1)
        s.log_attempt(KEY, error_class="rate_limited", operational=True, now=t2)
        assert s.due(KEY, t2 + timedelta(hours=3, minutes=59)).action == "wait"
        assert s.due(KEY, t2 + timedelta(hours=4)).action == "try"
        t3 = t2 + timedelta(hours=4)
        s.log_attempt(KEY, error_class="timeout", operational=True, now=t3)
        assert s.due(KEY, t3 + timedelta(hours=3)).action == "wait"  # stays at 4h
        d = s.due(KEY, T0 + timedelta(hours=24))
        assert d.action == "give_up" and d.failures == 4
        assert s.give_up(KEY, None, T0 + timedelta(hours=24))
        assert s.due(KEY, T0 + timedelta(hours=30)).action == "terminal"
        assert [r["reason"] for r in s.results()] == ["provider-unavailable"]


def test_non_operational_attempt_does_not_start_the_clock(s):
    with s.locked():
        s.log_attempt(KEY, error_class="http_4xx", operational=False, now=T0)
        assert s.due(KEY, T0 + timedelta(days=3)).action == "try"


def test_pending_is_eligible_minus_terminal_not_a_time_cursor(s):
    with s.locked():
        s.write_terminal(st.Key("d2", "shadow", "cfg1"), {"verdict": "held"}, T0)
        s.log_attempt(st.Key("d1", "shadow", "cfg1"), error_class="timeout", operational=True, now=T0)  # an old stranded error
    assert s.pending(["d1", "d2", "d3"], "shadow", "cfg1") == ["d1", "d3"]
    assert s.pending(["d2"], "shadow", "cfg2") == ["d2"]  # config change: not terminal for the new config


def test_partial_last_line_is_recovered_not_parsed(s):
    with s.locked():
        s.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    with open(s.attempts_path, "ab") as fh:
        fh.write(b'{"decision_id": "d9", "mode": "sha')  # crash mid-write
    assert len(st.read_jsonl(s.attempts_path)) == 1  # readers ignore it even before recovery
    with s.locked():
        pass
    assert open(s.attempts_path + ".recovered", "rb").read().startswith(b'{"decision_id": "d9"')
    assert open(s.attempts_path, "rb").read().endswith(b"\n")
    assert mode(s.attempts_path + ".recovered") == 0o600
    with s.locked():
        s.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    assert len(s.attempts()) == 2


def test_partial_results_line_recovered_and_key_not_terminal(s):
    with open(s.results_path, "wb") as fh:
        fh.write(json.dumps({"decision_id": "d1", "mode": "shadow", "config_hash": "cfg1", "verdict": "held"}).encode() + b"\n")
        fh.write(b'{"decision_id": "d1", "mode": "shadow", "config_hash": "cfg2", "verd')
    with s.locked():
        assert s.has_terminal(KEY)
        assert not s.has_terminal(st.Key("d1", "shadow", "cfg2"))
    assert os.path.exists(s.results_path + ".recovered")


def test_garbage_line_in_the_middle_is_skipped_not_fatal(s):
    with open(s.attempts_path, "wb") as fh:
        fh.write(b'not json\n{"decision_id":"d1","mode":"shadow","config_hash":"cfg1","operational":true,"ts":"2026-03-01T12:00:00Z"}\n')
    assert len(s.attempts()) == 1


def test_nan_cannot_be_written(s):
    with s.locked():
        with pytest.raises(ValueError):
            s.write_terminal(KEY, {"verdict": "decided", "confidence": float("nan")})
    assert s.results() == []


# ---- path checks (R10) -------------------------------------------------------------

def test_path_outside_state_dir_is_refused(s, tmp_path):
    with pytest.raises(st.UnsafePath):
        st.check_path(str(tmp_path / "elsewhere.jsonl"), s.base)
    with pytest.raises(st.UnsafePath):
        s.path("../escape.jsonl")


def test_symlink_to_protected_file_is_refused(tmp_path):
    src = tmp_path / "data.db"
    src.write_text("x")
    base = tmp_path / "judge"
    base.mkdir()
    (base / "out.jsonl").symlink_to(src)
    with pytest.raises(st.UnsafePath):
        st.check_path(str(base / "out.jsonl"), str(base), [str(src)])


def test_symlink_pointing_outside_is_refused(tmp_path):
    outside = tmp_path / "other"
    outside.write_text("x")
    base = tmp_path / "judge"
    base.mkdir()
    (base / "link").symlink_to(outside)
    with pytest.raises(st.UnsafePath):
        st.check_path(str(base / "link"), str(base))


def test_hard_link_to_protected_file_is_refused(tmp_path):
    src = tmp_path / "rulings.jsonl"
    src.write_text("x")
    base = tmp_path / "judge"
    base.mkdir()
    os.link(src, base / "alias.jsonl")
    with pytest.raises(st.UnsafePath):
        st.check_path(str(base / "alias.jsonl"), str(base), [str(src)])


def test_store_refuses_a_log_that_aliases_a_source(tmp_path):
    src = tmp_path / "data.db"
    src.write_text("x")
    base = tmp_path / "judge"
    base.mkdir()
    (base / "results.jsonl").symlink_to(src)
    with pytest.raises(st.UnsafePath):
        st.Store(str(base), forbidden=[str(src)])


def test_existing_loose_dir_is_tightened(tmp_path):
    base = tmp_path / "judge"
    base.mkdir(mode=0o755)
    st.Store(str(base))
    assert mode(str(base)) == 0o700


# ---- review fixes: the recovery file, the permitted root, the give-up row -------------

def partial(base):
    """Leave a torn last line in attempts.jsonl, as a crash mid-append would."""
    with open(os.path.join(base, "attempts.jsonl"), "ab") as fh:
        fh.write(b'{"torn":')


def test_recovery_still_works_and_is_private(s):
    with s.locked():
        s.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    partial(s.base)
    with s.locked():
        pass
    rec = os.path.join(s.base, "attempts.jsonl.recovered")
    assert open(rec, "rb").read() == b'{"torn":\n' and mode(rec) == 0o600
    assert len(s.attempts(KEY)) == 1


def test_recovered_symlink_to_a_protected_file_is_refused(tmp_path):
    src = tmp_path / "data.db"
    src.write_text("original")
    store = st.Store(str(tmp_path / "judge"), forbidden=[str(src)])
    with store.locked():
        store.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    partial(store.base)
    os.symlink(src, os.path.join(store.base, "attempts.jsonl.recovered"))
    with pytest.raises(st.UnsafePath):
        with store.locked():
            pass
    assert src.read_text() == "original"
    assert open(store.attempts_path, "rb").read().endswith(b'{"torn":')  # nothing was cut away


def test_recovered_hard_link_to_a_protected_file_is_refused(tmp_path):
    src = tmp_path / "rulings.jsonl"
    src.write_text("original")
    store = st.Store(str(tmp_path / "judge"), forbidden=[str(src)])
    with store.locked():
        store.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    partial(store.base)
    os.link(src, os.path.join(store.base, "attempts.jsonl.recovered"))
    with pytest.raises(st.UnsafePath):
        with store.locked():
            pass
    assert src.read_text() == "original"


def test_recovered_symlink_to_an_outside_file_is_refused(tmp_path):
    outside = tmp_path / "outside.txt"
    outside.write_text("keep")
    store = st.Store(str(tmp_path / "judge"))
    with store.locked():
        store.log_attempt(KEY, error_class="timeout", operational=True, now=T0)
    partial(store.base)
    os.symlink(outside, os.path.join(store.base, "attempts.jsonl.recovered"))
    with pytest.raises(st.UnsafePath):
        with store.locked():
            pass
    assert outside.read_text() == "keep"


def test_a_base_outside_the_permitted_root_is_refused_before_anything_is_made(tmp_path, monkeypatch):
    root = tmp_path / "root"
    root.mkdir()
    monkeypatch.setenv("RULINGJUDGE_STATE_ROOT", str(root))
    elsewhere = tmp_path / "elsewhere" / "judge"
    with pytest.raises(st.UnsafePath):
        st.Store(str(elsewhere))
    assert not (tmp_path / "elsewhere").exists()
    st.Store(str(root / "judge"))  # inside the root is fine
    st.Store(str(root))  # and so is the root itself


def test_a_symlinked_base_is_refused_and_leaves_its_target_alone(tmp_path, monkeypatch):
    root = tmp_path / "root"
    root.mkdir()
    target = tmp_path / "target"
    target.mkdir(mode=0o755)
    os.symlink(target, root / "judge")
    monkeypatch.setenv("RULINGJUDGE_STATE_ROOT", str(root))
    with pytest.raises(st.UnsafePath):
        st.Store(str(root / "judge"))
    assert mode(str(target)) == 0o755 and os.listdir(target) == []


def test_default_root_is_the_state_directory(monkeypatch):
    monkeypatch.delenv("RULINGJUDGE_STATE_ROOT")
    assert st.permitted_root() == os.path.realpath(os.path.expanduser(st.DEFAULT_DIR))


def test_give_up_row_carries_provenance(s):
    prov = {"manifest_id": "M1", "split": "dev", "revision": "3", "corpus_hash": "ch", "cand_hash": "kh"}
    with s.locked():
        assert s.give_up(KEY, prov, T0)
    row = s.results()[0]
    assert row["verdict"] == "held" and row["reason"] == "provider-unavailable"
    assert all(row[k] == v for k, v in prov.items())
