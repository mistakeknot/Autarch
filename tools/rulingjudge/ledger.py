"""Normalise the rulings ledger (JSONL, several row shapes) into citable and context rows.

Rules (plan R2):
  * timestamp precedence: `ts`, then `date` (a date-only value counts as available
    from the next day 00:00Z), then `answered_at`; rows with none are skipped and counted;
  * duplicate ids keep the earliest row, the rest are counted;
  * only rows with by == "mk" are citable; every other row is context only;
  * rows carrying a post-hoc annotation (an outcome or digest field filled in after
    the ruling) are excluded from replay and counted.

The ledger file is opened O_RDONLY and never written.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
from collections import Counter
from dataclasses import dataclass
from datetime import date, datetime, timedelta, timezone
from typing import Any, Iterable

UTC = timezone.utc

CARD_KEY_RE = re.compile(r"\b[A-Z][A-Z0-9]{1,7}-\d{1,6}\b")
_DATE_ONLY_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")

TEXT_FIELDS = ("subject", "question", "ruling", "words", "quote", "basis", "summary", "note", "reason")
# A value in any of these means a later annotation was written onto the row.
POST_HOC_FIELDS = ("mk_outcome", "mk_pick", "delta", "digest_read_at", "digest_listed_at")

SHAPES = ("row-v2", "outcome", "home-pick", "approval", "delegated", "ruling", "record", "unknown")
CLASSES = ("mk-home", "mk-ruling", "vizier-delegated", "vizier", "approval", "unknown")


@dataclass(frozen=True)
class Row:
    id: str
    line: int
    sha256: str
    ts: datetime
    ts_source: str
    by: str | None
    shape: str
    cls: str
    text: str
    pick: str | None
    cards: frozenset[str]
    citable: bool

    def cite_ref(self) -> dict[str, str]:
        """Ledger ids repeat, so a citation pins the id, the time and the line's hash."""
        return {"id": self.id, "ts": self.ts.isoformat().replace("+00:00", "Z"), "sha256": self.sha256}


@dataclass(frozen=True)
class Skip:
    reason: str  # malformed | not-object | no-id | no-timestamp | post-hoc


@dataclass
class Ledger:
    rows: list[Row]  # sorted by (ts, line)
    stats: Counter


def extract_card_keys(*values: Any) -> frozenset[str]:
    """Card keys from a singular `card`, a plural `cards` list, and free text."""
    found: set[str] = set()
    for value in values:
        if isinstance(value, str):
            found.update(CARD_KEY_RE.findall(value))
        elif isinstance(value, (list, tuple)):
            for item in value:
                if isinstance(item, str):
                    found.update(CARD_KEY_RE.findall(item))
    return frozenset(found)


def parse_instant(value: Any) -> datetime | None:
    """A full ISO timestamp as UTC; a naive value is read as UTC. Date-only returns None."""
    if not isinstance(value, str):
        return None
    text = value.strip()
    if not text or _DATE_ONLY_RE.match(text):
        return None
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=UTC)
    return parsed.astimezone(UTC)


def parse_date_only(value: Any) -> datetime | None:
    """A bare date is conservatively available from the next day at 00:00Z."""
    if not isinstance(value, str) or not _DATE_ONLY_RE.match(value.strip()):
        return None
    try:
        day = date.fromisoformat(value.strip())
    except ValueError:
        return None
    return datetime(day.year, day.month, day.day, tzinfo=UTC) + timedelta(days=1)


def availability(obj: dict[str, Any]) -> tuple[datetime, str] | None:
    for key in ("ts", "date", "answered_at"):
        raw = obj.get(key)
        instant = parse_instant(raw)
        if instant is not None:
            return instant, key
        day = parse_date_only(raw)
        if day is not None:
            return day, f"{key}-date-only"
    return None


def classify_shape(obj: dict[str, Any]) -> str:
    if "row_id" in obj:
        return "row-v2"
    if "mk_outcome" in obj:
        return "outcome"
    if "card" in obj and "option" in obj and "words" in obj:
        return "home-pick"
    if "approval" in obj or ("pr" in obj and "head" in obj):
        return "approval"
    if "basis" in obj and ("quote" in obj or "delegated" in obj):
        return "delegated"
    if "ruling" in obj:
        return "ruling"
    if "summary" in obj:
        return "record"
    return "unknown"


def _by(obj: dict[str, Any]) -> str | None:
    value = obj.get("by", obj.get("ruled_by"))
    return value.strip() if isinstance(value, str) and value.strip() else None


def classify_class(shape: str, by: str | None, obj: dict[str, Any]) -> str:
    if shape == "approval":
        return "approval"
    if by == "mk":
        return "mk-home" if shape == "home-pick" else "mk-ruling"
    if by and by.startswith("vizier"):
        if "delegated" in by or obj.get("delegated") or shape == "delegated":
            return "vizier-delegated"
        return "vizier"
    return "unknown"


def _text(obj: dict[str, Any]) -> str:
    parts = []
    for key in TEXT_FIELDS:
        value = obj.get(key)
        if isinstance(value, str) and value.strip():
            parts.append(value.strip())
    return "\n".join(parts)


def _pick(shape: str, obj: dict[str, Any]) -> str | None:
    value = obj.get("option") if shape == "home-pick" else obj.get("pick")
    return value if isinstance(value, str) and value else None


def normalise(obj: Any, raw: bytes, line_no: int) -> Row | Skip:
    if not isinstance(obj, dict):
        return Skip("not-object")
    row_id = obj.get("id", obj.get("row_id"))
    if not isinstance(row_id, str) or not row_id:
        return Skip("no-id")
    avail = availability(obj)
    if avail is None:
        return Skip("no-timestamp")
    if any(obj.get(field) not in (None, "", [], {}) for field in POST_HOC_FIELDS):
        return Skip("post-hoc")
    shape = classify_shape(obj)
    by = _by(obj)
    cls = classify_class(shape, by, obj)
    text = _text(obj)
    return Row(
        id=row_id,
        line=line_no,
        sha256=hashlib.sha256(raw).hexdigest(),
        ts=avail[0],
        ts_source=avail[1],
        by=by,
        shape=shape,
        cls=cls,
        text=text,
        pick=_pick(shape, obj),
        cards=extract_card_keys(obj.get("card"), obj.get("cards"), text),
        citable=by == "mk",
    )


def normalise_lines(lines: Iterable[bytes]) -> Ledger:
    stats: Counter = Counter()
    kept: dict[str, Row] = {}
    for line_no, raw in enumerate(lines, start=1):
        raw = raw.rstrip(b"\r\n")
        if not raw.strip():
            stats["blank"] += 1
            continue
        stats["lines"] += 1
        try:
            obj = json.loads(raw)
        except ValueError:
            stats["malformed"] += 1
            continue
        result = normalise(obj, raw, line_no)
        if isinstance(result, Skip):
            stats[result.reason.replace("-", "_")] += 1
            continue
        prior = kept.get(result.id)
        if prior is None:
            kept[result.id] = result
            continue
        stats["duplicate_ids"] += 1
        if (result.ts, result.line) < (prior.ts, prior.line):
            kept[result.id] = result
    rows = sorted(kept.values(), key=lambda row: (row.ts, row.line))
    stats["usable"] = len(rows)
    stats["citable"] = sum(1 for row in rows if row.citable)
    for row in rows:
        stats[f"shape:{row.shape}"] += 1
        stats[f"class:{row.cls}"] += 1
    return Ledger(rows=rows, stats=stats)


def load_ledger(path: str) -> Ledger:
    """Read the ledger. Opened O_RDONLY; nothing is ever written back."""
    fd = os.open(path, os.O_RDONLY)
    with os.fdopen(fd, "rb") as handle:
        return normalise_lines(handle)
