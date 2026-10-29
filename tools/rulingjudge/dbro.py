"""Read-only access to the Citadel data.db (plan R8, R10).

The connection is opened with mode=ro, query_only is switched on, and an authorizer then
denies everything except reads, so an INSERT/UPDATE/DELETE/CREATE/ATTACH or PRAGMA fails
even if a future change loosened the open flags.
"""

from __future__ import annotations

import sqlite3
from dataclasses import dataclass
from datetime import datetime
from urllib.parse import quote

from .ledger import UTC, parse_instant

_ALLOWED = {sqlite3.SQLITE_SELECT, sqlite3.SQLITE_READ, sqlite3.SQLITE_FUNCTION}
if hasattr(sqlite3, "SQLITE_RECURSIVE"):
    _ALLOWED.add(sqlite3.SQLITE_RECURSIVE)


def _authorizer(action, *_args):
    return sqlite3.SQLITE_OK if action in _ALLOWED else sqlite3.SQLITE_DENY


def open_ro(path: str) -> sqlite3.Connection:
    uri = "file:" + quote(path) + "?mode=ro"
    conn = sqlite3.connect(uri, uri=True)
    conn.execute("PRAGMA query_only = ON")
    conn.set_authorizer(_authorizer)
    return conn


@dataclass(frozen=True)
class PickRec:
    option_id: str
    revision: str
    picked_at: datetime | None
    reason: str | None


@dataclass(frozen=True)
class DecisionRec:
    id: str
    revision: str
    body_json: str
    filed_at: datetime | None
    filed_at_raw: str
    delegable: int
    withdrawn: bool
    supersedes: str | None
    task_id: str | None
    identity: str | None
    semantic_key: str | None
    card_key: str | None
    request_identity: str | None
    pick: PickRec | None
    card_move: bool = False  # the card carried a home-move block (a moves row opened by the card)


QUERY = """
SELECT d.id, d.revision, d.body_json, d.filed_at, d.delegable, d.withdrawn_at, d.supersedes,
       d.task_id, d.identity, d.semantic_key, c.card_key, c.request_identity,
       p.option_id, p.revision, p.picked_at, p.reason,
       EXISTS (SELECT 1 FROM moves m WHERE m.task_id = d.task_id AND m.generation = d.generation AND m.opened_by = 'card')
FROM decisions d
LEFT JOIN cards c ON c.task_id = d.task_id
LEFT JOIN picks p ON p.decision_id = d.id
ORDER BY d.filed_at, d.id
"""


class SchemaError(RuntimeError):
    """The database lacks the tables or columns that carry the structural flags."""


def load_decisions(conn: sqlite3.Connection) -> list[DecisionRec]:
    """Decisions with their labels and structural flags.

    A card's home-move block is stored as a `moves` row opened by the card for the decision's
    (task_id, generation). Rows opened by a pick are consequences of the answer, so they are not
    read: they would leak the label. If the table or the generation column is missing the flag
    cannot be established, and the load fails rather than treating its absence as "no move".
    """
    out: list[DecisionRec] = []
    try:
        rows = conn.execute(QUERY).fetchall()
    except sqlite3.OperationalError as exc:
        raise SchemaError(f"cannot establish the structural flags (moves table / generation column): {exc}") from exc
    for r in rows:
        filed = parse_instant(r[3])
        pick = None
        if r[12] is not None:
            at = parse_instant(r[14])
            pick = PickRec(option_id=r[12], revision=r[13], picked_at=at.astimezone(UTC) if at else None, reason=r[15])
        out.append(DecisionRec(
            id=r[0], revision=r[1], body_json=r[2], filed_at=filed.astimezone(UTC) if filed else None, filed_at_raw=r[3],
            delegable=r[4], withdrawn=r[5] is not None, supersedes=r[6], task_id=r[7], identity=r[8],
            semantic_key=r[9], card_key=r[10], request_identity=r[11], pick=pick, card_move=bool(r[16]),
        ))
    return out
