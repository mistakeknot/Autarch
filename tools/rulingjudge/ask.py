"""A filed card decision, parsed from decisions.body_json.

The body is a home-ask object. Options always carry id, kind and label; `instruction`,
`reversible` and `approval` appear only when set (the Go struct omits empty values), so
an absent `reversible` carries no evidence either way: it neither holds nor permits.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

from .ledger import UTC, extract_card_keys, parse_instant

# The card description is not kept in data.db. A home-move block shows up there as a `moves` row
# opened by the card, which the loader reads (dbro). A root-run block leaves no structured trace, so
# `root_run` below is only a text signal and is reported as unestablished, never as proven absent.
ROOT_RUN_MARKERS = ("```root-run", "root-run:")


@dataclass(frozen=True)
class Option:
    id: str
    kind: str
    label: str
    instruction: str | None = None
    reversible: bool | None = None
    approval: dict | None = None

    def semantics(self) -> dict[str, Any]:
        out: dict[str, Any] = {"id": self.id, "kind": self.kind, "label": self.label}
        if self.instruction:
            out["instruction"] = self.instruction
        if self.reversible is not None:
            out["reversible"] = self.reversible
        if self.approval:
            out["approval"] = self.approval
        return out


@dataclass(frozen=True)
class Ask:
    decision_id: str
    revision: str
    question: str
    subject: str
    project: str
    options: tuple[Option, ...]
    recommendation: str | None
    delegable: bool
    root_run: bool
    move: bool
    filed_at: datetime
    kind: str = "decide"
    body_sha256: str = ""
    body_cards: frozenset[str] = field(default_factory=frozenset)

    def option_ids(self) -> list[str]:
        return [o.id for o in self.options]

    def search_text(self) -> str:
        """Text used as the retrieval query: what the ask says, never card metadata."""
        parts = [self.subject, self.question] + [f"{o.label} {o.instruction or ''}" for o in self.options]
        return "\n".join(p for p in parts if p)


def _str(value: Any) -> str:
    return value if isinstance(value, str) else ""


def parse_option(raw: Any) -> Option | None:
    if not isinstance(raw, dict) or not isinstance(raw.get("id"), str) or not raw["id"]:
        return None
    reversible = raw.get("reversible")
    approval = raw.get("approval")
    return Option(
        id=raw["id"],
        kind=_str(raw.get("kind")),
        label=_str(raw.get("label")),
        instruction=_str(raw.get("instruction")) or None,
        reversible=reversible if isinstance(reversible, bool) else None,
        approval=approval if isinstance(approval, dict) and approval else None,
    )


def parse_ask(decision_id: str, revision: str, body_json: str, filed_at: str, delegable: int = 1, card_move: bool = False) -> Ask | None:
    """None when the body is not a usable decide ask."""
    import hashlib

    try:
        body = json.loads(body_json)
    except ValueError:
        return None
    if not isinstance(body, dict):
        return None
    opts = tuple(o for o in (parse_option(r) for r in body.get("options") or []) if o)
    filed = parse_instant(filed_at)
    if not opts or filed is None:
        return None
    question = _str(body.get("question"))
    rec = body.get("recommendation")
    low = question.lower()
    return Ask(
        decision_id=decision_id,
        revision=revision,
        question=question,
        subject=_str(body.get("subject")),
        project=_str(body.get("project")),
        options=opts,
        recommendation=rec if isinstance(rec, str) and rec else None,
        delegable=bool(delegable),
        root_run=bool(body.get("root_run")) or any(m in low for m in ROOT_RUN_MARKERS),
        move=bool(card_move),
        filed_at=filed.astimezone(UTC),
        kind=_str(body.get("kind")) or "decide",
        body_sha256=hashlib.sha256(body_json.encode()).hexdigest(),
        body_cards=extract_card_keys(question, body.get("subject")),
    )
