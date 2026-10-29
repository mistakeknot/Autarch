"""Synthetic data.db and manifest rules."""

import json
import os
import sqlite3
from datetime import datetime, timedelta, timezone

import pytest

from tools.rulingjudge import dbro, manifest as mf, store as st

T0 = datetime(2026, 2, 1, tzinfo=timezone.utc)
CUT = T0 + timedelta(days=100)


def ts(days):
    return (T0 + timedelta(days=days)).isoformat().replace("+00:00", ".000Z")


SCHEMA = """
CREATE TABLE decisions (id TEXT PRIMARY KEY, revision TEXT, body_json TEXT, filed_at TEXT, delegable INT DEFAULT 1,
  withdrawn_at TEXT, supersedes TEXT UNIQUE, task_id TEXT, identity TEXT, semantic_key TEXT);
CREATE TABLE picks (decision_id TEXT PRIMARY KEY, option_id TEXT, revision TEXT, picked_at TEXT, reason TEXT);
CREATE TABLE cards (task_id TEXT PRIMARY KEY, card_key TEXT, request_identity TEXT);
"""


def body(n=0):
    return json.dumps({"question": f"synthetic question {n}", "subject": f"s{n}", "project": "p",
                       "options": [{"id": "a", "kind": "ruling-only", "label": "A"}, {"id": "b", "kind": "ruling-only", "label": "B"}]})


def make_db(path, specs):
    c = sqlite3.connect(path)
    c.executescript(SCHEMA)
    for s in specs:
        c.execute("INSERT INTO decisions (id,revision,body_json,filed_at,withdrawn_at,supersedes,task_id,identity,semantic_key) VALUES (?,?,?,?,?,?,?,?,?)",
                  (s["id"], s.get("rev", "1"), s.get("body", body(s["day"])), ts(s["day"]), s.get("withdrawn"), s.get("supersedes"),
                   s.get("task", "t-" + s["id"]), s.get("identity", "i-" + s["id"]), s.get("sem", "k-" + s["id"])))
        c.execute("INSERT OR IGNORE INTO cards VALUES (?,?,?)", (s.get("task", "t-" + s["id"]), s.get("card", "ABC-" + s["id"][1:]), s.get("req", "r-" + s["id"])))
        if "pick" in s:
            c.execute("INSERT INTO picks VALUES (?,?,?,?,?)", (s["id"], s["pick"], s.get("pick_rev", s.get("rev", "1")), ts(s["day"] + 1), s.get("reason")))
    c.commit()
    c.close()
    return path


@pytest.fixture
def db(tmp_path):
    return str(tmp_path / "data.db")


def build(db, specs, cutoff=CUT):
    make_db(db, specs)
    conn = dbro.open_ro(db)
    return mf.build_manifest(dbro.load_decisions(conn), cutoff)


# ---- read-only opener -------------------------------------------------------------

@pytest.mark.parametrize("sql", [
    "INSERT INTO picks VALUES ('x','a','1','2026-01-01',NULL)",
    "UPDATE picks SET option_id='z'",
    "DELETE FROM picks",
    "CREATE TABLE evil (x)",
    "DROP TABLE picks",
    "ALTER TABLE picks ADD COLUMN y",
    "ATTACH DATABASE ':memory:' AS m",
    "ATTACH DATABASE '/tmp/rulingjudge-should-not-exist.db' AS m",
    "PRAGMA query_only = OFF",
    "PRAGMA writable_schema = ON",
    "VACUUM",
    "CREATE TEMP TABLE t (x)",
])
def test_writes_are_refused(db, sql):
    make_db(db, [{"day": 1, "id": "d1", "pick": "a"}])
    conn = dbro.open_ro(db)
    with pytest.raises(sqlite3.Error):
        conn.execute(sql)
        conn.commit()
    assert conn.execute("SELECT count(*) FROM picks").fetchone() == (1,)
    assert not os.path.exists("/tmp/rulingjudge-should-not-exist.db")


def test_reads_work_and_file_is_unchanged(db):
    make_db(db, [{"day": 1, "id": "d1", "pick": "a"}])
    before = open(db, "rb").read()
    conn = dbro.open_ro(db)
    assert dbro.load_decisions(conn)[0].pick.option_id == "a"
    conn.close()
    assert open(db, "rb").read() == before


def test_missing_db_is_an_error_not_a_creation(tmp_path):
    with pytest.raises(sqlite3.Error):
        dbro.open_ro(str(tmp_path / "nope.db")).execute("SELECT 1 FROM sqlite_master").fetchall()
        dbro.open_ro(str(tmp_path / "nope.db")).execute("SELECT * FROM decisions")
    assert not (tmp_path / "nope.db").exists()


# ---- manifest rules ---------------------------------------------------------------

def test_exclusions_are_counted(db):
    specs = [
        {"day": 1, "id": "d1", "pick": "a"},
        {"day": 2, "id": "d2", "pick": "a", "withdrawn": ts(3)},
        {"day": 3, "id": "d3"},  # no pick
        {"day": 4, "id": "d4", "pick": "a", "rev": "2", "pick_rev": "1"},
        {"day": 5, "id": "d5", "pick": "a"},
        {"day": 6, "id": "d6", "pick": "a", "supersedes": "d5"},
        {"day": 7, "id": "d7", "pick": "a", "body": "not json"},
    ]
    m = build(db, specs)
    ids = {e.decision_id for e in m.entries}
    assert ids == {"d1", "d6"}
    assert m.excluded["withdrawn"] == 1 and m.excluded["no_pick"] == 1 and m.excluded["pick_revision_mismatch"] == 1
    assert m.excluded["superseded"] == 1 and m.excluded["unparseable"] == 1


def test_label_cutoff_excludes_late_picks(db):
    m = build(db, [{"day": 1, "id": "d1", "pick": "a"}, {"day": 50, "id": "d2", "pick": "a"}], cutoff=T0 + timedelta(days=10))
    assert [e.decision_id for e in m.entries] == ["d1"] and m.excluded["after_label_cutoff"] == 1


def test_other_pick_keeps_raw_text(db):
    m = build(db, [{"day": 1, "id": "d1", "pick": "other", "reason": "Please REDO this, with Logto auth"}])
    e = m.entries[0]
    assert e.pick_kind == "other" and e.pick_reason == "Please REDO this, with Logto auth"


def test_families_and_first_flags(db):
    specs = [
        {"day": 1, "id": "d1", "pick": "a", "identity": "same"},
        {"day": 5, "id": "d2", "pick": "a", "identity": "same"},
        {"day": 6, "id": "d3", "pick": "b"},
    ]
    m = build(db, specs)
    by = {e.decision_id: e for e in m.entries}
    assert by["d1"].first and not by["d2"].first and by["d3"].first
    assert by["d1"].family == by["d2"].family != by["d3"].family
    assert set(by["d2"].family_cards) >= {"ABC-1", "ABC-2"}


def test_splits_are_chronological_and_families_stay_together(db):
    specs = [{"day": d, "id": f"d{d}", "pick": "a"} for d in range(1, 21)]
    specs.append({"day": 30, "id": "rep", "pick": "a", "identity": "i-d2"})  # repeat of d2 (dev)
    m = build(db, specs)
    firsts = m.firsts()
    assert len(firsts) == 20
    dev = m.firsts("dev"); test = m.firsts("test")
    assert len(dev) == 10 and len(test) == 10
    assert max(e.filed_at for e in dev) < min(e.filed_at for e in test)
    assert len(m.firsts("dev", "tune")) == 7 and len(m.firsts("dev", "validation")) == 3
    assert max(e.filed_at for e in m.firsts("dev", "tune")) < min(e.filed_at for e in m.firsts("dev", "validation"))
    rep = m.by_id("rep")
    assert not rep.first and rep.split == "dev"


def test_manifest_id_tracks_labels(db, tmp_path):
    a = build(db, [{"day": 1, "id": "d1", "pick": "a"}])
    b = build(str(tmp_path / "other.db"), [{"day": 1, "id": "d1", "pick": "b"}])
    c = build(str(tmp_path / "third.db"), [{"day": 1, "id": "d1", "pick": "a"}])
    assert a.id != b.id and a.id == c.id


def test_save_load_roundtrip_is_private(db, tmp_path):
    m = build(db, [{"day": 1, "id": "d1", "pick": "a"}, {"day": 2, "id": "d2", "pick": "b"}])
    s = st.Store(str(tmp_path / "judge"))
    path = mf.save_manifest(s, m)
    assert os.stat(path).st_mode & 0o777 == 0o600
    assert mf.load_manifest(s, m.id).to_json() == m.to_json()
    assert mf.save_manifest(s, m) == path  # idempotent


# ---- freeze -----------------------------------------------------------------------

def test_test_split_needs_a_freeze_and_a_matching_config(db, tmp_path):
    m = build(db, [{"day": 1, "id": "d1", "pick": "a"}])
    s = st.Store(str(tmp_path / "judge"))
    with pytest.raises(mf.FreezeError):
        mf.require_frozen(s, m, "cfgA")
    mf.freeze_config(s, m, "cfgA")
    mf.require_frozen(s, m, "cfgA")
    with pytest.raises(mf.FreezeError):
        mf.require_frozen(s, m, "cfgB")
    with pytest.raises(mf.FreezeError):
        mf.freeze_config(s, m, "cfgB")  # no re-freeze with another config
    mf.freeze_config(s, m, "cfgA")  # same config is fine


def test_no_refreeze_after_test_results_exist(db, tmp_path):
    m = build(db, [{"day": 1, "id": "d1", "pick": "a"}])
    s = st.Store(str(tmp_path / "judge"))
    with s.locked():
        s.write_terminal(st.Key("d1", "backtest-test", "cfgA"), {"verdict": "held", "manifest_id": m.id})
    with pytest.raises(mf.FreezeError):
        mf.freeze_config(s, m, "cfgA")
