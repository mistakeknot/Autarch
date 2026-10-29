"""Per-cutoff BM25 retrieval over the normalised ledger (plan R1, R3).

Everything is computed from rows strictly earlier than the cutoff: the document
count, document frequencies and average length are all taken over that prefix, so
adding later rows to the ledger cannot change a historical ranking (tested).
Rows that name any card key of the asking decision's family are removed before
ranking and from the statistics (the leak guard). Card metadata is used for
linkage only; it is never part of the text that is ranked or shown to the model.
"""

from __future__ import annotations

import hashlib
import math
import re
from bisect import bisect_left
from dataclasses import dataclass, field
from datetime import datetime
from typing import Iterable

from .ledger import Row

TOKEN_RE = re.compile(r"[a-z0-9][a-z0-9_]+")
STOPWORDS = frozenset(
    "the and for are but not you all any can had her was one our out has have this that with from they "
    "will would there their what about which when your said each she how its into than then them these "
    "some could been were more also only other should must may per via is it of to in on at as be by or "
    "an if so we do no".split()
)


def tokenize(text: str) -> list[str]:
    return [t for t in TOKEN_RE.findall(text.lower()) if t not in STOPWORDS]


@dataclass(frozen=True)
class Params:
    k1: float = 1.5
    b: float = 0.75
    k_citable: int = 12
    k_context: int = 4
    max_chars: int = 600

    def as_dict(self) -> dict:
        return {"k1": self.k1, "b": self.b, "k_citable": self.k_citable, "k_context": self.k_context, "max_chars": self.max_chars}


@dataclass(frozen=True)
class Hit:
    row: Row
    score: float
    text: str  # truncated for the model
    cut: bool


@dataclass
class Retrieval:
    citable: list[Hit]
    context: list[Hit]
    cutoff: datetime
    available: int  # rows before the cutoff
    leak_excluded: int
    truncated: bool  # any shown row text was cut at max_chars
    corpus_hash: str
    cand_hash: str
    terms: list[str] = field(default_factory=list)

    def hits(self) -> list[Hit]:
        return self.citable + self.context


class Retriever:
    def __init__(self, rows: Iterable[Row], params: Params | None = None) -> None:
        self.params = params or Params()
        self.rows = sorted(rows, key=lambda r: (r.ts, r.line))
        self._ts = [r.ts for r in self.rows]
        self._len: list[int] = []
        self._prefix_len = [0]
        self._postings: dict[str, list[tuple[int, int]]] = {}
        self._doc_terms: list[dict[str, int]] = []
        chain = ""
        self._chain: list[str] = []
        for i, row in enumerate(self.rows):
            toks = tokenize(row.text)
            tf: dict[str, int] = {}
            for t in toks:
                tf[t] = tf.get(t, 0) + 1
            self._doc_terms.append(tf)
            self._len.append(len(toks))
            self._prefix_len.append(self._prefix_len[-1] + len(toks))
            for term, n in tf.items():
                self._postings.setdefault(term, []).append((i, n))
            chain = hashlib.sha256((chain + row.sha256).encode()).hexdigest()
            self._chain.append(chain)

    def _df(self, term: str, prefix: int, excluded: list[int]) -> int:
        post = self._postings.get(term)
        if not post:
            return 0
        lo, hi = 0, len(post)
        while lo < hi:  # first posting with doc index >= prefix
            mid = (lo + hi) // 2
            if post[mid][0] < prefix:
                lo = mid + 1
            else:
                hi = mid
        df = lo
        for i in excluded:
            if term in self._doc_terms[i]:
                df -= 1
        return df

    def search(self, query: str, cutoff: datetime, family_cards: frozenset[str] = frozenset()) -> Retrieval:
        p = self.params
        prefix = bisect_left(self._ts, cutoff)  # strictly before the cutoff
        excluded = [i for i in range(prefix) if family_cards and (self.rows[i].cards & family_cards)]
        excluded_set = set(excluded)
        n_docs = prefix - len(excluded)
        total_len = self._prefix_len[prefix] - sum(self._len[i] for i in excluded)
        terms = sorted(set(tokenize(query)))
        scores: dict[int, float] = {}
        if n_docs > 0 and total_len > 0:
            avgdl = total_len / n_docs
            for term in terms:
                df = self._df(term, prefix, excluded)
                if df <= 0:
                    continue
                idf = math.log(1.0 + (n_docs - df + 0.5) / (df + 0.5))
                for i, f in self._postings[term]:
                    if i >= prefix:
                        break
                    if i in excluded_set:
                        continue
                    denom = f + p.k1 * (1.0 - p.b + p.b * self._len[i] / avgdl)
                    scores[i] = scores.get(i, 0.0) + idf * f * (p.k1 + 1.0) / denom
        ranked = sorted(scores.items(), key=lambda kv: (-kv[1], -kv[0]))
        citable: list[Hit] = []
        context: list[Hit] = []
        truncated = False
        for i, score in ranked:
            row = self.rows[i]
            bucket, cap = (citable, p.k_citable) if row.citable else (context, p.k_context)
            if len(bucket) >= cap:
                if len(citable) >= p.k_citable and len(context) >= p.k_context:
                    break
                continue
            cut = len(row.text) > p.max_chars
            truncated = truncated or cut
            bucket.append(Hit(row=row, score=score, text=row.text[: p.max_chars], cut=cut))
        base = self._chain[prefix - 1] if prefix else ""
        corpus_hash = hashlib.sha256(
            (base + "|" + ",".join(sorted(self.rows[i].id for i in excluded))).encode()
        ).hexdigest()
        cand_hash = hashlib.sha256(
            "|".join(h.row.sha256 for h in citable + context).encode()
        ).hexdigest()
        return Retrieval(
            citable=citable,
            context=context,
            cutoff=cutoff,
            available=prefix,
            leak_excluded=len(excluded),
            truncated=truncated,
            corpus_hash=corpus_hash,
            cand_hash=cand_hash,
            terms=terms,
        )
