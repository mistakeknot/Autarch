"""Command line: backtest, freeze, shadow and report (plan tasks 9 and R5, R8, R10).

Shadow only. Verdicts go to the judge state directory and nowhere else; nothing in Citadel is
written. `--dry-run` builds every request and prints what would be sent, calling nothing and
loading no credential. The data.db is opened read-only and the ledger O_RDONLY.

Exit codes: 0 ok, 2 usage / unsafe path / selector unavailable, 3 credential unavailable,
4 another run holds the lock, 5 freeze rule, 6 the saved manifest no longer matches the database.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib
import json
import os
import re
import sys
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Any, Callable, TextIO

from . import dbro, judge, manifest as mf, report as rp, sel as selmod
from .ask import Ask
from .families import Families, build_families
from .ledger import UTC, load_ledger
from .retrieve import Params, Retrieval, Retriever
from .store import Key, LockBusy, Store, UnsafePath, iso, utcnow

DEFAULT_LEDGER = "~/.local/state/vizier/rulings.jsonl"
SAFE_NAME = re.compile(r"[^A-Za-z0-9_.-]")


class CliError(Exception):
    def __init__(self, message: str, code: int = 2) -> None:
        super().__init__(message)
        self.code = code


# ---- setup -------------------------------------------------------------------------

@dataclass
class Ctx:
    sel: selmod.Selector
    store: Store
    decisions: list[dbro.DecisionRec]
    by_id: dict[str, dbro.DecisionRec]
    asks: dict[str, Ask | None]
    retriever: Retriever
    cfg: judge.JudgeConfig
    cfg_hash: str
    db: str
    ledger: str


def make_cfg(a: argparse.Namespace) -> judge.JudgeConfig:
    return judge.JudgeConfig(
        t=a.t, t_sb=a.t_sb, include_recommendation=a.include_recommendation,
        retrieval=Params(k_citable=a.k_citable, k_context=a.k_context),
    )


def load_ctx(a: argparse.Namespace) -> Ctx:
    db = a.db or os.environ.get("RULINGJUDGE_DB")
    if not db:
        raise CliError("no data.db: pass --db or set RULINGJUDGE_DB")
    db = os.path.expanduser(db)
    ledger_path = os.path.expanduser(a.ledger or os.environ.get("RULINGJUDGE_LEDGER") or DEFAULT_LEDGER)
    try:
        sel = selmod.load()
    except selmod.SelectorUnavailable as exc:
        raise CliError(f"selector unavailable, nothing sent: {exc}") from exc
    try:
        store = Store(os.path.expanduser(a.state_dir) if a.state_dir else None, forbidden=(db, ledger_path))
    except (UnsafePath, OSError) as exc:
        raise CliError(f"unusable state directory: {type(exc).__name__}") from exc
    conn = dbro.open_ro(db)
    try:
        decisions = dbro.load_decisions(conn)
    except dbro.SchemaError as exc:
        raise CliError(f"structural flags unavailable, nothing sent: {exc}") from exc
    finally:
        conn.close()
    cfg = make_cfg(a)
    ledger = load_ledger(ledger_path)
    return Ctx(
        sel=sel, store=store, decisions=decisions, by_id={d.id: d for d in decisions}, asks=mf.parse_asks(decisions),
        retriever=Retriever(ledger.rows, cfg.retrieval), cfg=cfg, cfg_hash=cfg.config_hash(sel), db=db, ledger=ledger_path,
    )


def label_cutoff(ctx: Ctx, a: argparse.Namespace) -> datetime:
    if getattr(a, "label_cutoff", None):
        return datetime.fromisoformat(a.label_cutoff.replace("Z", "+00:00")).astimezone(UTC)
    picks = [d.pick.picked_at for d in ctx.decisions if d.pick and d.pick.picked_at]
    return max(picks) if picks else utcnow()


def get_manifest(ctx: Ctx, a: argparse.Namespace) -> mf.Manifest:
    if getattr(a, "manifest", None):
        try:
            return mf.load_manifest(ctx.store, a.manifest)
        except OSError as exc:
            raise CliError(f"no saved manifest {a.manifest}") from exc
    m = mf.build_manifest(ctx.decisions, label_cutoff(ctx, a))
    mf.save_manifest(ctx.store, m)
    return m


def default_transport_factory(sel: selmod.Selector) -> Callable[[Any], Any]:
    """The production transport: the selector's own credential loader and client. The key never
    passes through this module; a failure to load it stops the run."""
    cred_mod = importlib.import_module("clavain_selector.credentials")
    try:
        credential = cred_mod.load()
    except cred_mod.CredentialUnavailable as exc:
        raise CliError("credential unavailable; no request sent", 3) from exc
    return sel.jev.JevClient(credential).call


# ---- running ------------------------------------------------------------------------

@dataclass
class Item:
    decision_id: str
    cutoff: datetime
    family_cards: frozenset[str]
    split: str | None = None


def retrieve(ctx: Ctx, ask: Ask, item: Item) -> Retrieval:
    return ctx.retriever.search(ask.search_text(), item.cutoff, item.family_cards)


def inputs_sha256(ask: Ask, ret: Retrieval | None, item: Item) -> str:
    """One hash over everything the holds and the request builder read for this decision: every
    Ask field (the option semantics in order, the structural flags from the moves table and the
    root-run signal included), the retrieval cutoff and family, and the retrieval hashes. The
    configuration is not here; it is part of the result key."""
    payload = {
        "decision_id": ask.decision_id, "revision": ask.revision, "body_sha256": ask.body_sha256, "kind": ask.kind,
        "project": ask.project, "subject": ask.subject, "question": ask.question, "recommendation": ask.recommendation,
        "options": [o.semantics() for o in ask.options], "delegable": bool(ask.delegable), "root_run": bool(ask.root_run),
        "move": bool(ask.move), "filed_at": iso(item.cutoff), "family_cards": sorted(item.family_cards),
        "corpus_hash": ret.corpus_hash if ret is not None else None, "cand_hash": ret.cand_hash if ret is not None else None,
    }
    return hashlib.sha256(json.dumps(payload, sort_keys=True, ensure_ascii=False, separators=(",", ":")).encode()).hexdigest()


def provenance(ask: Ask | None, ret: Retrieval | None, item: Item, manifest_id: str) -> dict:
    """What every terminal row carries so a report can place it: manifest, split, revision and
    the retrieval facts. A hold written before or without a model call carries them too."""
    row: dict = {"manifest_id": manifest_id, "split": item.split}
    if ask is not None:
        row.update(revision=ask.revision, recommendation=ask.recommendation, body_sha256=ask.body_sha256,
                   delegable=bool(ask.delegable), filed_at=iso(item.cutoff), inputs_sha256=inputs_sha256(ask, ret, item))
    if ret is not None:
        row.update(corpus_hash=ret.corpus_hash, cand_hash=ret.cand_hash,
                   retrieval={"available": ret.available, "leak_excluded": ret.leak_excluded, "truncated": ret.truncated,
                              "citable": len(ret.citable), "context": len(ret.context)})
    return row


def result_row(out: judge.Outcome, ask: Ask, ret: Retrieval, item: Item, manifest_id: str) -> dict:
    return {
        **provenance(ask, ret, item, manifest_id),
        "verdict": out.verdict, "option": out.option, "reason": out.reason, "confidence": out.confidence, "choice": out.choice,
        "cite_ref": out.cite_ref, "cite_ok": out.cite_ok, "cite_method": out.cite_method, "probabilities": out.probabilities,
        "error_class": out.error_class, "excerpt": out.excerpt, "http_status": out.http_status,
        "input_tokens": out.input_tokens, "output_tokens": out.output_tokens, "latency_ms": out.latency_ms,
        "request_sha256": out.request_sha256, "cand_ids": out.cand_ids,
    }


def keep_request(ctx: Ctx, decision_id: str, prepared: judge.Prepared) -> str:
    folder = ctx.store.subdir("requests")
    path = ctx.store.path(os.path.join("requests", f"{SAFE_NAME.sub('_', decision_id)}-{ctx.cfg_hash[:8]}.json"))
    body = prepared.admitted.body
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        os.fchmod(fd, 0o600)
        os.write(fd, body if isinstance(body, bytes) else str(body).encode())
    finally:
        os.close(fd)
    assert os.path.dirname(path) == folder
    return path


def dry_run(ctx: Ctx, items: list[Item], keep: bool) -> Counter:
    tally: Counter = Counter()
    for item in items:
        ask = ctx.asks.get(item.decision_id)
        if ask is None:
            tally["held:unparseable"] += 1
            continue
        prepared = judge.prepare(ctx.sel, ask, retrieve(ctx, ask, item), ctx.cfg)
        if isinstance(prepared, judge.Outcome):
            tally["held:" + rp.hold_class(prepared.reason)] += 1
        else:
            tally["would_send"] += 1
            tally["wire_bytes"] += prepared.wire_bytes
            if keep:
                keep_request(ctx, item.decision_id, prepared)
    return tally


def run_live(ctx: Ctx, items: list[Item], mode: str, manifest_id: str, factory: Callable[[selmod.Selector], Callable[[Any], Any]],
             keep: bool, clock: Callable[[], datetime] = utcnow) -> Counter:
    """Judge each item once. Every row is stamped when it is appended (`clock()`), never with the
    scan-start time, so a pick made during a slow call is seen to precede the verdict. Retrieval
    cutoffs belong to the item and are not touched here."""
    tally: Counter = Counter()
    transport = None
    with ctx.store.locked() as store:
        for item in items:
            key = Key(item.decision_id, mode, ctx.cfg_hash)
            due = store.due(key, clock())
            if due.action == "terminal":
                tally[settle_terminal(ctx, store, key, item, manifest_id, clock) if mode.startswith("backtest-") else "already_done"] += 1
                continue
            if due.action == "wait":
                tally["waiting_backoff"] += 1
                continue
            ask = ctx.asks.get(item.decision_id)
            if due.action == "give_up":
                ret = retrieve(ctx, ask, item) if ask is not None else None
                store.give_up(key, provenance(ask, ret, item, manifest_id), clock())
                tally["gave_up"] += 1
                continue
            if ask is None:
                store.write_terminal(key, {"verdict": "held", "reason": "unparseable", **provenance(None, None, item, manifest_id)}, clock())
                tally["held:unparseable"] += 1
                continue
            ret = retrieve(ctx, ask, item)
            prepared = judge.prepare(ctx.sel, ask, ret, ctx.cfg)
            called = False
            if isinstance(prepared, judge.Outcome):
                out = prepared
            else:
                if keep:
                    keep_request(ctx, item.decision_id, prepared)
                if transport is None:
                    transport = factory(ctx.sel)
                out = judge.execute(ctx.sel, ask, ret, prepared, ctx.cfg, transport)
                called = True
            if called:
                # One event per completed provider attempt, written before any terminal row.
                store.log_attempt(key, error_class=out.error_class, operational=out.operational, http_status=out.http_status,
                                  text=out.excerpt, now=clock(), request_sha256=out.request_sha256, verdict=out.verdict)
            if out.operational:
                tally["operational_error"] += 1
                continue
            store.write_terminal(key, result_row(out, ask, ret, item, manifest_id), clock())
            tally[out.verdict or "none"] += 1
    return tally


def settle_terminal(ctx: Ctx, store: Store, key: Key, item: Item, manifest_id: str, clock: Callable[[], datetime]) -> str:
    """A decision already has its terminal row for this configuration. Under the same manifest that
    ends it. Under another manifest (a later label cutoff) the report only counts rows of its own
    manifest, so the old row is reused only when its `inputs_sha256` equals the one this run would
    send (see `inputs_sha256`); then it is copied under the new manifest and counted. A row with no
    hash, or a different one, is reported as incompatible, never dropped and never counted."""
    rows = store.terminal_rows(key)
    if any(r.get("manifest_id") == manifest_id for r in rows):
        return "already_done"
    ask = ctx.asks.get(item.decision_id)
    want = inputs_sha256(ask, retrieve(ctx, ask, item), item) if ask is not None else None
    for r in rows:
        if want is not None and r.get("inputs_sha256") == want:
            reused = {**r, "manifest_id": manifest_id, "split": item.split, "reused_from": r.get("manifest_id"),
                      "input_tokens": 0, "output_tokens": 0, "latency_ms": None}
            store.write_reuse(key, reused, clock())
            return "reused_from_other_manifest"
    return "incompatible_other_manifest"


def check_replay(ctx: Ctx, m: mf.Manifest, items: list[Item]) -> None:
    """A manifest freezes what was evaluated. Before any request is built, every item has to match
    the database's current ask on revision, body hash, delegability and filing time; otherwise an
    edited ask would be judged against its old label and cutoff."""
    drifted: list[str] = []
    for it in items:
        e = m.by_id(it.decision_id)
        d = ctx.by_id.get(it.decision_id)
        ask = ctx.asks.get(it.decision_id)
        if e is None or d is None or ask is None:
            drifted.append(f"{it.decision_id}: missing")
        elif ask.revision != e.revision:
            drifted.append(f"{it.decision_id}: revision")
        elif ask.body_sha256 != e.body_sha256:
            drifted.append(f"{it.decision_id}: body")
        elif bool(ask.delegable) != bool(e.delegable):
            drifted.append(f"{it.decision_id}: delegable")
        elif d.filed_at is None or iso(d.filed_at) != e.filed_at:
            drifted.append(f"{it.decision_id}: filed_at")
    if drifted:
        raise CliError(f"manifest {m.id[:16]} no longer matches the database ({len(drifted)} changed, first {drifted[0]}); nothing sent", 6)


# ---- commands -----------------------------------------------------------------------

def split_items(m: mf.Manifest, split: str, limit: int | None) -> list[Item]:
    chosen = sorted(m.firsts(split), key=lambda e: (e.filed_at, e.decision_id)) + sorted(m.repeats(split), key=lambda e: (e.filed_at, e.decision_id))
    if limit is not None:
        chosen = chosen[:limit]
    return [Item(e.decision_id, mf_parse(e.filed_at), frozenset(e.family_cards), split) for e in chosen]


def mf_parse(text: str) -> datetime:
    return datetime.fromisoformat(text.replace("Z", "+00:00")).astimezone(UTC)


def manifest_lines(m: mf.Manifest) -> list[str]:
    firsts, repeats = m.firsts(), m.repeats()
    fams = {e.family for e in m.entries}
    return [
        f"manifest {m.id[:16]} label_cutoff={m.label_cutoff}",
        f"entries={len(m.entries)} families={len(fams)} first-of-family={len(firsts)} repeats(already settled)={len(repeats)}",
        f"dev firsts={len(m.firsts('dev'))} (tune={len(m.firsts('dev', 'tune'))}, validation={len(m.firsts('dev', 'validation'))}) test firsts={len(m.firsts('test'))}",
        "excluded: " + ", ".join(f"{k}={v}" for k, v in m.excluded.items()),
        f"other-picks={sum(1 for e in m.entries if e.pick_kind == 'other')}",
    ]


def emit(out: TextIO, lines: list[str]) -> None:
    for line in lines:
        print(line, file=out)


def cmd_backtest(a: argparse.Namespace, out: TextIO, factory: Callable) -> int:
    ctx = load_ctx(a)
    m = get_manifest(ctx, a)
    if a.split == "test":
        try:
            mf.require_frozen(ctx.store, m, ctx.cfg_hash)
        except mf.FreezeError as exc:
            raise CliError(str(exc), 5) from exc
    items = split_items(m, a.split, a.limit)
    check_replay(ctx, m, items)
    emit(out, manifest_lines(m))
    emit(out, [f"config {ctx.cfg_hash[:16]} split={a.split} items={len(items)} dry_run={a.dry_run}"])
    if a.dry_run:
        tally = dry_run(ctx, items, a.keep_requests)
        sent = tally.pop("would_send", 0)
        wire = tally.pop("wire_bytes", 0)
        holds = {k[5:]: v for k, v in sorted(tally.items()) if k.startswith("held:")}
        emit(out, [f"would send: {sent} requests, {wire} bytes", f"holds (no tokens): {sum(holds.values())}"] + [f"  {k}: {v}" for k, v in holds.items()])
        return 0
    tally = run_live(ctx, items, f"backtest-{a.split}", m.id, factory, a.keep_requests)
    emit(out, ["run: " + ", ".join(f"{k}={v}" for k, v in sorted(tally.items()))])
    if tally.get("incompatible_other_manifest"):
        raise CliError(f"{tally['incompatible_other_manifest']} decision(s) already have a result under another manifest whose "
                       "frozen inputs differ from this run's; they are not counted here. Use a new configuration or the original manifest", 7)
    return 0


def cmd_freeze(a: argparse.Namespace, out: TextIO, _factory: Callable) -> int:
    ctx = load_ctx(a)
    m = get_manifest(ctx, a)
    try:
        with ctx.store.locked():
            path = mf.freeze_config(ctx.store, m, ctx.cfg_hash)
    except mf.FreezeError as exc:
        raise CliError(str(exc), 5) from exc
    emit(out, [f"frozen manifest {m.id[:16]} config {ctx.cfg_hash[:16]}", path])
    return 0


def shadow_items(ctx: Ctx, now: datetime, since_days: int, limit: int | None) -> list[Item]:
    fams: Families = build_families(mf.build_refs(ctx.decisions, ctx.asks))
    superseded = {d.supersedes for d in ctx.decisions if d.supersedes}
    floor = now - timedelta(days=since_days)
    items = []
    for d in ctx.decisions:
        if d.pick or d.withdrawn or d.id in superseded or d.filed_at is None or d.filed_at < floor or ctx.asks.get(d.id) is None:
            continue
        items.append(Item(d.id, now, frozenset(fams.cards_of(d.id)), "shadow"))
    return items[:limit] if limit is not None else items


def cmd_shadow(a: argparse.Namespace, out: TextIO, factory: Callable) -> int:
    if not a.once:
        raise CliError("shadow runs one pass at a time: pass --once")
    ctx = load_ctx(a)
    now = utcnow()
    m = mf.build_manifest(ctx.decisions, label_cutoff(ctx, a), now)
    mf.save_manifest(ctx.store, m)
    items = shadow_items(ctx, now, a.since_days, a.limit)
    emit(out, [f"shadow: {len(items)} unpicked asks filed in the last {a.since_days} days (label snapshot {m.id[:16]})"])
    if a.dry_run:
        tally = dry_run(ctx, items, a.keep_requests)
        emit(out, ["would send: %d requests" % tally.pop("would_send", 0)] + [f"  {k}: {v}" for k, v in sorted(tally.items()) if k != "wire_bytes"])
        return 0
    tally = run_live(ctx, items, "shadow", m.id, factory, a.keep_requests)
    emit(out, ["run: " + ", ".join(f"{k}={v}" for k, v in sorted(tally.items()))])
    return 0


def write_report(ctx: Ctx, name: str, text: str) -> str:
    path = ctx.store.path(name)  # inside the state directory, never a source file
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    try:
        os.fchmod(fd, 0o600)
        os.write(fd, text.encode())
    finally:
        os.close(fd)
    return path


def cmd_report(a: argparse.Namespace, out: TextIO, _factory: Callable) -> int:
    ctx = load_ctx(a)
    m = get_manifest(ctx, a)
    results = ctx.store.results()
    chunks: list[str] = []
    if a.live:
        live = rp.live_report(m, results, ctx.cfg_hash)
        chunks.append("live (shadow) verdicts, prospective only: " + ", ".join(f"{k}={v.text() if hasattr(v, 'text') else v}" for k, v in live.items()))
    elif a.ablation:
        h0 = make_hash(ctx, a, False)
        h1 = make_hash(ctx, a, True)
        res = rp.ablation(m, results, results, h0, h1)
        chunks.append(json.dumps(res, sort_keys=True, indent=1, default=str))
    else:
        scope = [ctx.asks.get(e.decision_id) for e in m.entries if e.split == a.split]
        ev = rp.evaluate(m, results, ctx.cfg_hash, f"backtest-{a.split}", a.split, a.sub, structure=rp.structure_coverage(scope))
        chunks.append(json.dumps(ev.as_dict(), sort_keys=True, indent=1) if a.json else ev.text())
    if a.audit_sample:
        chunks.append("`other` audit sample (raw words, mark each as equivalent to an option or not):")
        chunks += [f"  {e.decision_id}: {e.pick_reason!r}" for e in rp.other_audit_sample(m, a.audit_sample)]
    text = "\n".join(chunks)
    print(text, file=out)
    if a.out:
        print(f"written: {write_report(ctx, a.out, text + chr(10))}", file=out)
    return 0


def make_hash(ctx: Ctx, a: argparse.Namespace, include_recommendation: bool) -> str:
    cfg = judge.JudgeConfig(t=a.t, t_sb=a.t_sb, include_recommendation=include_recommendation, retrieval=Params(k_citable=a.k_citable, k_context=a.k_context))
    return cfg.config_hash(ctx.sel)


# ---- parser ---------------------------------------------------------------------------

def parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="rulingjudge", description="Shadow-only ruling judge.")
    p.add_argument("--db", help="Citadel data.db (opened read-only); or RULINGJUDGE_DB")
    p.add_argument("--ledger", help=f"rulings ledger (opened read-only); default {DEFAULT_LEDGER}")
    p.add_argument("--state-dir", help="judge state directory; default ~/.local/state/autarch/judge")
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--t", type=float, default=0.9, help="decide threshold")
    common.add_argument("--t-sb", type=float, default=0.9, help="send-back threshold (exploratory)")
    common.add_argument("--include-recommendation", action="store_true", help="show the asker's recommendation to the model")
    common.add_argument("--k-citable", type=int, default=12)
    common.add_argument("--k-context", type=int, default=4)
    common.add_argument("--manifest", help="reuse a saved manifest by id instead of building one")
    common.add_argument("--label-cutoff", help="ISO instant; picks after it are not labels (default: latest pick)")
    sub = p.add_subparsers(dest="cmd", required=True)

    bt = sub.add_parser("backtest", parents=[common], help="judge picked asks in a chronological split")
    bt.add_argument("--split", choices=("dev", "test"), required=True)
    bt.add_argument("--limit", type=int)
    bt.add_argument("--dry-run", action="store_true", help="build requests, call nothing, load no credential")
    bt.add_argument("--keep-requests", action="store_true", help="keep request bodies under the state directory")
    bt.set_defaults(fn=cmd_backtest)

    fz = sub.add_parser("freeze", parents=[common], help="freeze the tuned configuration before the test split")
    fz.set_defaults(fn=cmd_freeze)

    sh = sub.add_parser("shadow", parents=[common], help="judge new unpicked asks, log only")
    sh.add_argument("--once", action="store_true")
    sh.add_argument("--since-days", type=int, default=7)
    sh.add_argument("--limit", type=int)
    sh.add_argument("--dry-run", action="store_true")
    sh.add_argument("--keep-requests", action="store_true")
    sh.set_defaults(fn=cmd_shadow)

    rpt = sub.add_parser("report", parents=[common], help="metrics from the result log")
    rpt.add_argument("--split", choices=("dev", "test"), default="dev")
    rpt.add_argument("--sub", choices=("tune", "validation"))
    rpt.add_argument("--live", action="store_true", help="prospective shadow precision")
    rpt.add_argument("--ablation", action="store_true", help="recommendation on vs off (dev only)")
    rpt.add_argument("--audit-sample", type=int, default=0, help="print N `other` picks for the equivalence audit")
    rpt.add_argument("--json", action="store_true")
    rpt.add_argument("--out", help="file name inside the state directory")
    rpt.set_defaults(fn=cmd_report)
    return p


def main(argv: list[str] | None = None, *, transport_factory: Callable | None = None, out: TextIO | None = None) -> int:
    args = parser().parse_args(argv)
    out = out or sys.stdout
    factory = transport_factory or default_transport_factory
    try:
        return args.fn(args, out, factory)
    except CliError as exc:
        print(f"rulingjudge: {exc}", file=sys.stderr)
        return exc.code
    except UnsafePath as exc:
        print(f"rulingjudge: unsafe path: {exc}", file=sys.stderr)
        return 2
    except LockBusy as exc:
        print(f"rulingjudge: {exc}", file=sys.stderr)
        return 4


if __name__ == "__main__":
    sys.exit(main())
