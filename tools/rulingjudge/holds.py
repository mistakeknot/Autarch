"""Deterministic holds (plan R4). They cost no tokens and never reach the model.

An ask is held, not judged, when any of these is true: it is not delegable, an option
carries an approval or is explicitly irreversible, it has a root-run or a move block, a
gate word list matches, or the question or an option is too long to send whole.
The word lists live in this one table; each class has a test. They are provisional until
the labelled-set audit measures their recall.
"""

from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass

from .ask import Ask

MAX_QUESTION = 6000
MAX_OPTION = 1800  # a candidate description may hold at most 2,000 characters

GATES: dict[str, tuple[str, ...]] = {
    "spend": (
        r"\bspend(ing)?\b", r"\$\s?\d", r"\bbilling\b", r"\bbill(ed|s)?\b", r"\bpurchase[sd]?\b", r"\bpay(ment|ments|ing)?\b",
        r"\bpaid\b", r"\bsubscription\b", r"\brenew(al|als|ed)?\b", r"\bbudget\b", r"\binvoice[sd]?\b", r"\bprice[sd]?\b",
        r"\bcredits?\b", r"\bregistration fee\b", r"\bplan upgrade\b", r"\bupgrade (the )?plan\b",
    ),
    "credential": (
        r"\bcredentials?\b", r"\bsecrets?\b", r"\bapi[ _-]?keys?\b", r"\btokens?\b", r"\bpasswords?\b", r"\bpassphrases?\b",
        r"\bprivate keys?\b", r"\boauth\b", r"\bssh keys?\b", r"\.env\b", r"\b1password\b", r"\bkeychain\b",
    ),
    "publication": (
        r"\bpublish(ed|es|ing)?\b", r"\breleases?\b", r"\breleased\b", r"\btag(s|ged|ging)?\b", r"\bpublic(ly)?\b",
        r"\bmerg(e|es|ed|ing)\b", r"\bpull request\b", r"\bannounce(ment|d)?\b", r"\blaunch(es|ed|ing)?\b", r"\bgo live\b",
        r"\bpush(es|ed|ing)? to (main|master)\b", r"\bmake (it )?public\b",
    ),
    "production_root": (
        r"\bdeploy(s|ed|ing|ment|ments)?\b", r"\broot[- ](run|script|shell|install)\b", r"\bsudo\b", r"\brestart(s|ed|ing)?\b",
        r"\binstall(s|ed|ing|ation)?\b", r"\bproduction\b", r"\bprod\b", r"\bsystemd\b", r"\bcron(job)?\b", r"\blive (site|data)\b",
        r"\bmigrat(e|es|ion|ions)\b",
    ),
    "permission": (
        r"\bpermissions?\b", r"\bsettings\.json\b", r"\ballow[- ]?(list|line|rule)s?\b", r"\bauto[- ]mode\b", r"\bhooks?\b",
        r"\bsudoers\b", r"\bgrant(s|ed|ing)? access\b", r"\baccess control\b", r"\bacl\b", r"\bdeny[- ]?list\b",
    ),
    "privacy_destination": (
        r"\bprivacy\b", r"\bpii\b", r"\bnew (destination|service|vendor|provider|endpoint|domain)s?\b", r"\bthird[- ]party\b",
        r"\bupload(s|ed|ing)?\b", r"\bexternal (service|api|host)s?\b", r"\bexpos(e|es|ed|ing)\b", r"\bdns\b", r"\bcloudflare\b",
        r"\bsend(s|ing)? (it |this |them |the \w+ )?to\b",
    ),
    "destructive": (
        r"\bdelet(e|es|ed|ing|ion)\b", r"\bforce[- ]push\b", r"\bdrop (table|database)\b", r"\bwipe[sd]?\b", r"\bpurge[sd]?\b",
        r"\bdestroy(s|ed|ing)?\b", r"\boverwrit(e|es|ing|ten)\b", r"\breset --hard\b", r"\birreversibl[ey]\b", r"\bcannot be undone\b",
        r"\btruncat(e|es|ed|ing)\b", r"\bprun(e|es|ed|ing)\b", r"\brewrit(e|ing) history\b", r"\brm -r",
    ),
    "signing_authority": (
        r"\bsign(s|ed|ing)?[- ]?off\b", r"\bsigning\b", r"\bsignatures?\b", r"\bgpg\b", r"\bcertificates?\b", r"\bauthori[sz](e|ed|ation|ing)\b",
        r"\bauthority\b", r"\bstanding approvals?\b", r"\bdelegat(e|ed|ion|ing)\b", r"\btrust (root|anchor|store)\b", r"\bkeep-list\b",
    ),
}
_COMPILED = {name: [re.compile(p, re.IGNORECASE) for p in pats] for name, pats in GATES.items()}


def gate_table_hash() -> str:
    return hashlib.sha256(json.dumps(GATES, sort_keys=True).encode()).hexdigest()


@dataclass(frozen=True)
class Holds:
    reasons: tuple[str, ...]

    @property
    def held(self) -> bool:
        return bool(self.reasons)

    @property
    def primary(self) -> str | None:
        return self.reasons[0] if self.reasons else None


def gate_hits(text: str) -> list[str]:
    return [name for name, pats in _COMPILED.items() if any(p.search(text) for p in pats)]


def ask_gate_text(ask: Ask) -> str:
    parts = [ask.subject, ask.question] + [f"{o.label}\n{o.instruction or ''}" for o in ask.options]
    return "\n".join(p for p in parts if p)


def evaluate(ask: Ask) -> Holds:
    reasons: list[str] = []
    if not ask.delegable:
        reasons.append("not-delegable")
    if any(o.approval for o in ask.options):
        reasons.append("approval-option")
    if any(o.reversible is False for o in ask.options):
        reasons.append("irreversible-option")
    if ask.root_run:
        reasons.append("root-run")
    if ask.move:
        reasons.append("move-block")
    reasons.extend(f"gate:{name}" for name in gate_hits(ask_gate_text(ask)))
    if len(ask.question) > MAX_QUESTION:
        reasons.append("truncated-question")
    if any(len(o.label) + len(o.instruction or "") > MAX_OPTION for o in ask.options):
        reasons.append("truncated-option")
    return Holds(tuple(reasons))
