"""Decision families (plan R1).

Two asks can be the same decision filed twice: a card re-filed after a revision, a
supersession, the same identity, a request identity shared by two cards, or an
identical semantic key. Union-find joins them. Headline metrics count only the
first decision of each family; later ones are "already settled" repeats. The family's
card keys feed the leak guard: any ledger row that names one is never retrieved.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from typing import Iterable


@dataclass(frozen=True)
class DecisionRef:
    id: str
    filed_at: datetime
    task_id: str | None = None
    supersedes: str | None = None
    identity: str | None = None
    request_identity: str | None = None
    semantic_key: str | None = None
    card_keys: frozenset[str] = frozenset()


class _UnionFind:
    def __init__(self) -> None:
        self.parent: dict[str, str] = {}

    def add(self, x: str) -> None:
        self.parent.setdefault(x, x)

    def find(self, x: str) -> str:
        root = x
        while self.parent[root] != root:
            root = self.parent[root]
        while self.parent[x] != root:
            self.parent[x], x = root, self.parent[x]
        return root

    def union(self, a: str, b: str) -> None:
        ra, rb = self.find(a), self.find(b)
        if ra != rb:
            self.parent[max(ra, rb)] = min(ra, rb)


@dataclass
class Families:
    family_of: dict[str, str]
    members: dict[str, list[str]]  # chronological
    card_keys: dict[str, frozenset[str]]
    first: dict[str, str]  # family id -> first decision id
    links: dict[str, int] = field(default_factory=dict)  # which rule joined how many pairs

    def is_first(self, decision_id: str) -> bool:
        return self.first[self.family_of[decision_id]] == decision_id

    def cards_of(self, decision_id: str) -> frozenset[str]:
        return self.card_keys[self.family_of[decision_id]]

    def earlier_in_family(self, decision_id: str) -> list[str]:
        mem = self.members[self.family_of[decision_id]]
        return mem[: mem.index(decision_id)]


def _blank(value: str | None) -> bool:
    return value is None or not value.strip()


def build_families(decisions: Iterable[DecisionRef]) -> Families:
    decs = list(decisions)
    uf = _UnionFind()
    by_id = {d.id: d for d in decs}
    for d in decs:
        uf.add(d.id)
    links = {"task": 0, "supersedes": 0, "identity": 0, "request_identity": 0, "semantic_key": 0}

    def join_by(attr: str, name: str) -> None:
        seen: dict[str, str] = {}
        for d in decs:
            value = getattr(d, attr)
            if _blank(value):
                continue
            if value in seen:
                if uf.find(seen[value]) != uf.find(d.id):
                    links[name] += 1
                uf.union(seen[value], d.id)
            else:
                seen[value] = d.id

    join_by("task_id", "task")
    join_by("identity", "identity")
    join_by("request_identity", "request_identity")
    join_by("semantic_key", "semantic_key")
    for d in decs:
        if d.supersedes and d.supersedes in by_id:
            if uf.find(d.supersedes) != uf.find(d.id):
                links["supersedes"] += 1
            uf.union(d.supersedes, d.id)

    groups: dict[str, list[DecisionRef]] = {}
    for d in decs:
        groups.setdefault(uf.find(d.id), []).append(d)
    family_of: dict[str, str] = {}
    members: dict[str, list[str]] = {}
    card_keys: dict[str, frozenset[str]] = {}
    first: dict[str, str] = {}
    for group in groups.values():
        group.sort(key=lambda d: (d.filed_at, d.id))
        fid = group[0].id
        members[fid] = [d.id for d in group]
        card_keys[fid] = frozenset().union(*(d.card_keys for d in group))
        first[fid] = fid
        for d in group:
            family_of[d.id] = fid
    return Families(family_of=family_of, members=members, card_keys=card_keys, first=first, links=links)
