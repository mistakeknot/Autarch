"""The frozen evaluation manifest and the chronological split (plan R5, R8).

The manifest fixes which decisions are evaluated and what their labels are, at one label
cutoff. A pick counts only if its revision matches the decision's. Withdrawn, superseded,
unpicked and unparseable decisions are excluded and counted. The id is a hash of the content,
so a changed label gives a new manifest.
"""

from __future__ import annotations

import hashlib
import json
import os
from dataclasses import dataclass, field
from datetime import datetime

from .ask import parse_ask
from .dbro import DecisionRec
from .families import DecisionRef, build_families
from .ledger import extract_card_keys
from .store import Store, UnsafePath, iso, utcnow

DEV_SHARE = 0.5
TUNE_SHARE = 0.7
EXCLUSIONS = ("withdrawn", "superseded", "no_pick", "pick_revision_mismatch", "after_label_cutoff", "unparseable", "no_filed_at")


class FreezeError(RuntimeError):
    pass


@dataclass(frozen=True)
class Entry:
    decision_id: str
    revision: str
    filed_at: str
    pick_option: str
    pick_kind: str  # option | other
    pick_reason: str | None  # raw text, kept as-is for `other` picks
    picked_at: str
    body_sha256: str
    family: str
    first: bool
    family_cards: tuple[str, ...]
    delegable: int
    split: str | None = None  # dev | test | None (repeat whose first was excluded)
    sub: str | None = None  # tune | validation inside dev

    def as_dict(self) -> dict:
        return dict(self.__dict__)


@dataclass
class Manifest:
    id: str
    label_cutoff: str
    entries: list[Entry]
    excluded: dict[str, int] = field(default_factory=dict)
    created_at: str = ""

    def by_id(self, decision_id: str) -> Entry | None:
        return next((e for e in self.entries if e.decision_id == decision_id), None)

    def firsts(self, split: str | None = None, sub: str | None = None) -> list[Entry]:
        return [e for e in self.entries if e.first and (split is None or e.split == split) and (sub is None or e.sub == sub)]

    def repeats(self, split: str | None = None) -> list[Entry]:
        return [e for e in self.entries if not e.first and (split is None or e.split == split)]

    def to_json(self) -> str:
        return json.dumps({"id": self.id, "label_cutoff": self.label_cutoff, "created_at": self.created_at,
                           "excluded": self.excluded, "entries": [e.as_dict() for e in self.entries]}, sort_keys=True, indent=1)

    @staticmethod
    def from_json(text: str) -> "Manifest":
        d = json.loads(text)
        entries = [Entry(**{**e, "family_cards": tuple(e["family_cards"])}) for e in d["entries"]]
        return Manifest(id=d["id"], label_cutoff=d["label_cutoff"], entries=entries, excluded=d["excluded"], created_at=d.get("created_at", ""))


def _content_id(label_cutoff: str, entries: list[Entry]) -> str:
    blob = [label_cutoff] + [[e.decision_id, e.revision, e.pick_option, e.pick_reason, e.picked_at, e.body_sha256, e.family, e.first, e.split, e.sub] for e in entries]
    return hashlib.sha256(json.dumps(blob, sort_keys=True).encode()).hexdigest()


def build_refs(decisions: list[DecisionRec], asks: dict) -> list[DecisionRef]:
    """Family-linking facts per decision. Card keys come from the card record and the ask text,
    for linkage only; they are never model input."""
    refs = []
    for d in decisions:
        if d.filed_at is None:
            continue
        keys = set(extract_card_keys(d.card_key))
        if asks.get(d.id):
            keys |= set(asks[d.id].body_cards)
        refs.append(DecisionRef(id=d.id, filed_at=d.filed_at, task_id=d.task_id, supersedes=d.supersedes, identity=d.identity,
                                request_identity=d.request_identity, semantic_key=d.semantic_key, card_keys=frozenset(keys)))
    return refs


def parse_asks(decisions: list[DecisionRec]) -> dict:
    return {d.id: parse_ask(d.id, d.revision, d.body_json, d.filed_at_raw, d.delegable, d.card_move) for d in decisions}


def build_manifest(decisions: list[DecisionRec], label_cutoff: datetime, now: datetime | None = None) -> Manifest:
    superseded = {d.supersedes for d in decisions if d.supersedes}
    asks = parse_asks(decisions)
    fams = build_families(build_refs(decisions, asks))
    excluded = {k: 0 for k in EXCLUSIONS}
    pre: list[Entry] = []
    for d in decisions:
        ask = asks[d.id]
        if d.filed_at is None:
            excluded["no_filed_at"] += 1
        elif d.withdrawn:
            excluded["withdrawn"] += 1
        elif d.id in superseded:
            excluded["superseded"] += 1
        elif d.pick is None:
            excluded["no_pick"] += 1
        elif d.pick.revision != d.revision:
            excluded["pick_revision_mismatch"] += 1
        elif d.pick.picked_at is None or d.pick.picked_at > label_cutoff:
            excluded["after_label_cutoff"] += 1
        elif ask is None:
            excluded["unparseable"] += 1
        else:
            kind = "option" if d.pick.option_id in ask.option_ids() else "other"
            pre.append(Entry(
                decision_id=d.id, revision=d.revision, filed_at=iso(d.filed_at), pick_option=d.pick.option_id, pick_kind=kind,
                pick_reason=d.pick.reason, picked_at=iso(d.pick.picked_at), body_sha256=ask.body_sha256,
                family=fams.family_of[d.id], first=fams.is_first(d.id), family_cards=tuple(sorted(fams.cards_of(d.id))), delegable=d.delegable,
            ))
    entries = assign_splits(pre)
    cutoff = iso(label_cutoff)
    return Manifest(id=_content_id(cutoff, entries), label_cutoff=cutoff, entries=entries, excluded=excluded, created_at=iso(now or utcnow()))


def assign_splits(entries: list[Entry]) -> list[Entry]:
    """Earlier half of first-of-family decisions is dev (70/30 tune/validation); the rest is test.
    Repeats follow their family."""
    firsts = sorted((e for e in entries if e.first), key=lambda e: (e.filed_at, e.decision_id))
    dev_n = (len(firsts) + 1) // 2
    tune_n = round(dev_n * TUNE_SHARE)
    where: dict[str, tuple[str, str | None]] = {}
    for i, e in enumerate(firsts):
        if i < dev_n:
            where[e.family] = ("dev", "tune" if i < tune_n else "validation")
        else:
            where[e.family] = ("test", None)
    out = []
    for e in entries:
        split, sub = where.get(e.family, (None, None))
        out.append(Entry(**{**e.as_dict(), "split": split, "sub": sub}))
    return out


# ---- persistence and the freeze ---------------------------------------------------

def _write_new(path: str, text: str) -> None:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        os.write(fd, text.encode())
        os.fsync(fd)
    finally:
        os.close(fd)


def save_manifest(store: Store, m: Manifest) -> str:
    path = store.path(f"manifest-{m.id[:16]}.json")
    if not os.path.exists(path):
        _write_new(path, m.to_json())
    return path


def load_manifest(store: Store, manifest_id: str) -> Manifest:
    return Manifest.from_json(open(store.path(f"manifest-{manifest_id[:16]}.json")).read())


COHORT_FILE = "test-cohort.json"


def _test_ids(m: Manifest) -> set[str]:
    return {e.decision_id for e in m.entries if e.split == "test"}


def _load_cohort(store: Store) -> list[dict]:
    path = store.path(COHORT_FILE)
    if not os.path.exists(path):
        return []
    return json.load(open(path)).get("freezes", [])


def _cohort_conflict(store: Store, m: Manifest, config_hash: str) -> str | None:
    """The test cohort belongs to the experiment, not to one manifest. A new label cutoff or a
    newly picked decision gives another manifest, but its test decisions that were already frozen
    or evaluated under a different configuration cannot be reused with changed settings. Same
    configuration (a retry or a replay) is always allowed."""
    ids = _test_ids(m)
    for f in _load_cohort(store):
        if f["config_hash"] != config_hash and ids & set(f["decisions"]):
            return "test decisions were already frozen with a different configuration"
    for r in store.results():
        if str(r.get("mode", "")).endswith("-test") and r.get("decision_id") in ids and r.get("config_hash") != config_hash:
            return "test decisions were already evaluated with a different configuration"
    return None


def _record_cohort(store: Store, m: Manifest, config_hash: str, frozen_at: str) -> None:
    """Idempotent per (manifest, configuration): a retry or a reconcile never adds a second record."""
    freezes = _load_cohort(store)
    if any(f["manifest_id"] == m.id and f["config_hash"] == config_hash for f in freezes):
        return
    freezes.append({"manifest_id": m.id, "config_hash": config_hash, "frozen_at": frozen_at, "decisions": sorted(_test_ids(m))})
    path = store.path(COHORT_FILE)
    tmp = store.path(COHORT_FILE + ".tmp")
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    try:
        os.write(fd, json.dumps({"freezes": freezes}, sort_keys=True).encode())
        os.fsync(fd)
    finally:
        os.close(fd)
    os.replace(tmp, path)


def _reconcile_cohort(store: Store, m: Manifest, config_hash: str, frozen_at: str) -> None:
    """Every path that finds an existing freeze goes through here. A freeze file with no cohort
    record (an interrupted write) gets its record; one that conflicts with another freeze or an
    evaluation is refused first, so a refused configuration is never written into the cohort."""
    conflict = _cohort_conflict(store, m, config_hash)
    if conflict:
        raise FreezeError(conflict)
    _record_cohort(store, m, config_hash, frozen_at)


def freeze_config(store: Store, m: Manifest, config_hash: str, now: datetime | None = None) -> str:
    """Record the tuned configuration before the test split runs. A different configuration after
    test results exist is refused (a failed test is a report, not another tuning round), for this
    manifest and for any later manifest whose test cohort overlaps an earlier freeze or evaluation.
    Runs under the run lock: the check, the cohort record and the freeze file are one step."""
    with store.held():
        return _freeze_config(store, m, config_hash, now)


def _freeze_config(store: Store, m: Manifest, config_hash: str, now: datetime | None) -> str:
    path = store.path(f"frozen-{m.id[:16]}.json")
    if os.path.exists(path):
        cur = json.load(open(path))
        if cur["config_hash"] != config_hash:
            raise FreezeError("manifest already frozen with a different configuration")
        _reconcile_cohort(store, m, config_hash, cur.get("frozen_at") or iso(now or utcnow()))
        return path
    if any(r.get("manifest_id") == m.id and str(r.get("mode", "")).endswith("-test") for r in store.results()):
        raise FreezeError("test results already exist for this manifest")
    conflict = _cohort_conflict(store, m, config_hash)
    if conflict:
        raise FreezeError(conflict)
    stamp = iso(now or utcnow())
    # The cohort record goes first: if the write is interrupted between the two, the cohort is
    # already protected and a retry finishes the freeze file. The other order left a freeze with no
    # cohort record, which a retry returned early on.
    _record_cohort(store, m, config_hash, stamp)
    _write_new(path, json.dumps({"manifest_id": m.id, "config_hash": config_hash, "frozen_at": stamp}, sort_keys=True))
    return path


def require_frozen(store: Store, m: Manifest, config_hash: str) -> None:
    """The test split's gate. It may repair a missing cohort record, so it runs under the run lock."""
    with store.held():
        _require_frozen(store, m, config_hash)


def _require_frozen(store: Store, m: Manifest, config_hash: str) -> None:
    path = store.path(f"frozen-{m.id[:16]}.json")
    if not os.path.exists(path):
        raise FreezeError("the test split needs a frozen configuration; freeze it after tuning on dev")
    cur = json.load(open(path))
    if cur["config_hash"] != config_hash:
        raise FreezeError("configuration differs from the frozen one")
    _reconcile_cohort(store, m, config_hash, cur.get("frozen_at") or "")
