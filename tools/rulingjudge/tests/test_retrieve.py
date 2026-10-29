"""Retrieval tests on a synthetic corpus."""

from datetime import datetime, timedelta, timezone

from tools.rulingjudge.ledger import normalise_lines
import json

from tools.rulingjudge.retrieve import Params, Retriever, tokenize

T0 = datetime(2026, 1, 1, tzinfo=timezone.utc)


def mk_rows(specs):
    """specs: (id, hours, by, text, extra) -> normalised rows."""
    raw = []
    for sid, hours, by, text, *extra in specs:
        obj = {"id": sid, "ts": (T0 + timedelta(hours=hours)).isoformat().replace("+00:00", "Z"), "by": by, "ruling": text}
        if extra:
            obj.update(extra[0])
        raw.append(json.dumps(obj).encode())
    return normalise_lines(raw).rows


CORPUS = [
    ("r1", 1, "mk", "merge approvals for deploy pipelines need a release gate"),
    ("r2", 2, "mk", "tune the dashboard colour palette"),
    ("r3", 3, "vizier", "deploy pipelines delegated rule"),
    ("r4", 4, "mk", "deploy pipeline merge rule second"),
    ("r5", 10, "mk", "deploy pipelines merge decision after cutoff"),
]


def ids(hits):
    return [h.row.id for h in hits]


def test_no_row_at_or_after_cutoff():
    r = Retriever(mk_rows(CORPUS))
    cutoff = T0 + timedelta(hours=4)  # r4 sits exactly at the cutoff
    res = r.search("deploy pipelines merge", cutoff)
    assert "r4" not in ids(res.hits()) and "r5" not in ids(res.hits())
    assert res.available == 3
    assert set(ids(res.citable)) <= {"r1", "r2"}
    assert ids(res.context) == ["r3"]


def test_family_rows_removed_and_dont_count_in_statistics():
    rows = mk_rows(CORPUS[:4] + [("r6", 5, "mk", "deploy pipelines merge, see ABC-1", {"cards": ["ABC-1"]})])
    r = Retriever(rows)
    res = r.search("deploy pipelines merge", T0 + timedelta(hours=9), frozenset({"ABC-1"}))
    assert "r6" not in ids(res.hits())
    assert res.leak_excluded == 1
    # Same answer as a corpus that never contained the row.
    ref = Retriever(mk_rows(CORPUS[:4])).search("deploy pipelines merge", T0 + timedelta(hours=9))
    assert [(h.row.id, round(h.score, 12)) for h in res.hits()] == [(h.row.id, round(h.score, 12)) for h in ref.hits()]


def test_text_reference_to_family_card_is_excluded():
    rows = mk_rows(CORPUS[:2] + [("r7", 3, "mk", "deploy rule noted on ABC-7 earlier")])
    res = Retriever(rows).search("deploy rule", T0 + timedelta(hours=9), frozenset({"ABC-7"}))
    assert "r7" not in ids(res.hits())


def test_future_rows_change_nothing():
    base = Retriever(mk_rows(CORPUS[:4]))
    more = Retriever(mk_rows(CORPUS + [("r8", 50, "mk", "deploy pipelines merge merge merge " * 5)]))
    cutoff = T0 + timedelta(hours=9)
    a = base.search("deploy pipelines merge gate", cutoff)
    b = more.search("deploy pipelines merge gate", cutoff)
    assert [(h.row.id, h.score) for h in a.hits()] == [(h.row.id, h.score) for h in b.hits()]
    assert a.corpus_hash == b.corpus_hash and a.cand_hash == b.cand_hash


def test_k_cap_and_truncation_flag():
    spec = [(f"c{i}", i, "mk", f"deploy rule number{i} " + "x" * 700 if i == 0 else f"deploy rule number{i}") for i in range(20)]
    res = Retriever(mk_rows(spec), Params(k_citable=5)).search("deploy rule", T0 + timedelta(hours=100))
    assert len(res.citable) == 5
    assert all(len(h.text) <= 600 for h in res.hits())
    cut_expected = any(h.cut for h in res.hits())
    assert res.truncated == cut_expected
    long = Retriever(mk_rows(spec[:1])).search("deploy rule", T0 + timedelta(hours=100))
    assert long.truncated and long.citable[0].cut and len(long.citable[0].text) == 600


def test_no_match_returns_empty():
    res = Retriever(mk_rows(CORPUS)).search("zebra", T0 + timedelta(hours=9))
    assert res.hits() == [] and not res.truncated


def test_ranking_prefers_more_relevant_row():
    res = Retriever(mk_rows(CORPUS)).search("palette colour", T0 + timedelta(hours=9))
    assert ids(res.citable)[0] == "r2"


def test_tokenize_drops_stopwords():
    assert tokenize("The deploy of a PR") == ["deploy", "pr"]
