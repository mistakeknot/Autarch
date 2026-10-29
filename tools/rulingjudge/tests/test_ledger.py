"""Ledger normaliser tests. Every row below is synthetic (invented for these tests)."""

import json
import os
from datetime import datetime, timezone

import pytest

from tools.rulingjudge import ledger as L

UTC = timezone.utc


def lines(*objs):
    return [json.dumps(o).encode() for o in objs]


def one(obj):
    led = L.normalise_lines(lines(obj))
    assert len(led.rows) == 1, led.stats
    return led.rows[0]


# One synthetic row of each shape.
ROW_V2 = {"row_id": "r-1", "ts": "2026-01-02T03:04:05Z", "by": "mk", "question": "q?", "ruling": "do A", "pick": "a"}
HOME_PICK = {"id": "h-1", "ts": "2026-01-02T03:04:05Z", "by": "mk", "card": "ABC-12", "option": "go", "words": "go ahead"}
DELEGATED = {"id": "d-1", "ts": "2026-01-02T03:04:05Z", "by": "vizier (delegated by ruling x)", "basis": "b", "quote": "q"}
VIZIER = {"id": "v-1", "ts": "2026-01-02T03:04:05Z", "by": "vizier", "ruling": "do B"}
APPROVAL = {"id": "p-1", "ts": "2026-01-02T03:04:05Z", "by": "mk", "pr": "x/y#1", "head": "abc", "approval": "merge"}
RECORD = {"id": "s-1", "ts": "2026-01-02T03:04:05Z", "by": "mk", "summary": "a summary"}


@pytest.mark.parametrize(
    "obj,shape,cls,citable",
    [
        (ROW_V2, "row-v2", "mk-ruling", True),
        (HOME_PICK, "home-pick", "mk-home", True),
        (DELEGATED, "delegated", "vizier-delegated", False),
        (VIZIER, "ruling", "vizier", False),
        (APPROVAL, "approval", "approval", True),
        (RECORD, "record", "mk-ruling", True),
    ],
)
def test_each_shape(obj, shape, cls, citable):
    row = one(obj)
    assert (row.shape, row.cls, row.citable) == (shape, cls, citable)


def test_only_by_mk_is_citable():
    for by in ("vizier", "pending-mk", "vizier (delegated)", None, "MK", "mk-bot"):
        obj = {"id": "x", "ts": "2026-01-02T00:00:00Z", "ruling": "r"}
        if by is not None:
            obj["by"] = by
        assert one(obj).citable is False, by
    assert one({"id": "x", "ts": "2026-01-02T00:00:00Z", "ruling": "r", "by": "mk"}).citable


def test_timestamp_precedence_ts_over_date():
    row = one({"id": "x", "ts": "2026-03-01T10:00:00+02:00", "date": "2026-03-05", "by": "mk", "ruling": "r"})
    assert row.ts == datetime(2026, 3, 1, 8, 0, tzinfo=UTC)
    assert row.ts_source == "ts"


def test_date_only_available_next_day_midnight():
    row = one({"id": "x", "date": "2026-03-05", "by": "mk", "ruling": "r"})
    assert row.ts == datetime(2026, 3, 6, 0, 0, tzinfo=UTC)
    assert row.ts_source == "date-date-only"


def test_date_only_in_ts_field_also_conservative():
    row = one({"id": "x", "ts": "2026-03-05", "by": "mk", "ruling": "r"})
    assert row.ts == datetime(2026, 3, 6, tzinfo=UTC)


def test_naive_timestamp_read_as_utc():
    assert one({"id": "x", "ts": "2026-03-05T01:02:03", "by": "mk", "ruling": "r"}).ts == datetime(
        2026, 3, 5, 1, 2, 3, tzinfo=UTC
    )


def test_duplicate_ids_keep_earliest():
    a = {"id": "dup", "ts": "2026-02-01T00:00:00Z", "by": "mk", "ruling": "later"}
    b = {"id": "dup", "ts": "2026-01-01T00:00:00Z", "by": "mk", "ruling": "earlier"}
    led = L.normalise_lines(lines(a, b))
    assert [r.text for r in led.rows] == ["earlier"]
    assert led.stats["duplicate_ids"] == 1


def test_malformed_and_unusable_rows_are_counted_not_raised():
    raw = [
        b"{not json",
        b"[1,2]",
        b"",
        json.dumps({"ts": "2026-01-01T00:00:00Z"}).encode(),  # no id
        json.dumps({"id": "n"}).encode(),  # no timestamp
        json.dumps({"id": "ok", "ts": "2026-01-01T00:00:00Z", "by": "mk", "ruling": "r"}).encode(),
    ]
    led = L.normalise_lines(raw)
    assert [r.id for r in led.rows] == ["ok"]
    s = led.stats
    assert (s["malformed"], s["not_object"], s["no_id"], s["no_timestamp"], s["blank"]) == (1, 1, 1, 1, 1)


@pytest.mark.parametrize("field", ["mk_outcome", "mk_pick", "delta", "digest_read_at"])
def test_post_hoc_annotations_excluded(field):
    obj = {"id": "x", "ts": "2026-01-01T00:00:00Z", "by": "mk", "question": "q", field: "later value"}
    led = L.normalise_lines(lines(obj))
    assert led.rows == []
    assert led.stats["post_hoc"] == 1


def test_empty_post_hoc_field_is_kept():
    assert one({"id": "x", "ts": "2026-01-01T00:00:00Z", "by": "mk", "ruling": "r", "mk_outcome": None})


def test_rows_sorted_by_time():
    led = L.normalise_lines(
        lines(
            {"id": "b", "ts": "2026-02-01T00:00:00Z", "by": "mk", "ruling": "r"},
            {"id": "a", "ts": "2026-01-01T00:00:00Z", "by": "mk", "ruling": "r"},
        )
    )
    assert [r.id for r in led.rows] == ["a", "b"]


def test_card_keys_from_card_cards_and_text():
    row = one(
        {
            "id": "x",
            "ts": "2026-01-01T00:00:00Z",
            "by": "mk",
            "card": "AAA-1",
            "cards": ["BBB-22", "ccc-3", 5],
            "ruling": "see CCC-33 and not lowercase abc-9",
        }
    )
    assert row.cards == frozenset({"AAA-1", "BBB-22", "CCC-33"})


def test_cite_ref_pins_line_hash():
    row = one(ROW_V2)
    ref = row.cite_ref()
    assert ref["id"] == "r-1" and ref["ts"] == "2026-01-02T03:04:05Z" and len(ref["sha256"]) == 64


def test_load_ledger_opens_read_only(tmp_path, monkeypatch):
    path = tmp_path / "rulings.jsonl"
    path.write_bytes(b"\n".join(lines(ROW_V2, VIZIER)) + b"\n")
    flags = []
    real_open = os.open

    def spy(p, f, *a, **k):
        flags.append(f)
        return real_open(p, f, *a, **k)

    monkeypatch.setattr(L.os, "open", spy)
    led = L.load_ledger(str(path))
    assert len(led.rows) == 2
    assert flags == [os.O_RDONLY]
