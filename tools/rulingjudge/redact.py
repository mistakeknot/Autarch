"""Bounded, redacted error excerpts (plan R9). Never holds a request body or a key."""

from __future__ import annotations

import re

MAX_EXCERPT = 200
_SECRET_RES = (
    re.compile(r"(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}"),
    re.compile(r"(?i)\b(api[_-]?key|token|secret|password|authorization)\b\s*[:=]\s*\S+"),
    re.compile(r"\b[A-Za-z0-9_-]{24,}\b"),  # any long opaque run
)


def excerpt(text: object, limit: int = MAX_EXCERPT) -> str:
    s = " ".join(str(text).split())
    for rx in _SECRET_RES:
        s = rx.sub("[redacted]", s)
    return s[:limit]
