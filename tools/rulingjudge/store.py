"""Judge state on disk: an append-only attempts log and one terminal result per key (plan R9, R10).

Everything lives under one state directory (default ~/.local/state/autarch/judge/), mode 0700,
files 0600. A run holds an exclusive flock on judge.lock. A crash can leave a partial last line;
it is moved to a `.recovered` file on the next open, never parsed and never silently dropped.
"""

from __future__ import annotations

import fcntl
import json
import os
import pwd
import stat
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, Iterable, Iterator

from .redact import MAX_EXCERPT, excerpt

DEFAULT_DIR = "~/.local/state/autarch/judge"
BACKOFF = (timedelta(minutes=15), timedelta(hours=1), timedelta(hours=4))
GIVE_UP_AFTER = timedelta(hours=24)
UTC = timezone.utc


class UnsafePath(ValueError):
    """A path outside the state directory, or one that aliases a protected source file."""


class LockBusy(RuntimeError):
    """Another judge run holds the lock."""


class NotLocked(RuntimeError):
    """A write was attempted without holding the run lock."""


def _real_home() -> str:
    """The account's home from the user database, so neither HOME nor a command line moves it."""
    return pwd.getpwuid(os.getuid()).pw_dir


def default_dir() -> str:
    return os.path.join(_real_home(), DEFAULT_DIR.removeprefix("~/"))


def permitted_root() -> str:
    """The only tree outputs may live under: the state directory under the real home. Nothing the
    process is started with (an environment variable, a flag) changes it. Tests substitute this
    function with a fixture; there is no production switch."""
    return os.path.realpath(default_dir())


def _same_file(a: str, b: str) -> bool:
    try:
        return os.path.samefile(a, b)
    except OSError:
        return os.path.realpath(a) == os.path.realpath(b)


def check_path(path: str, base: str, forbidden: Iterable[str] = ()) -> str:
    """The resolved path, or UnsafePath. Symlinks and hard links to a forbidden file are refused."""
    real_base = os.path.realpath(base)
    real = os.path.realpath(path)
    if real != real_base and not real.startswith(real_base + os.sep):
        raise UnsafePath("path resolves outside the judge state directory")
    for f in forbidden:
        if f and (os.path.realpath(f) == real or (os.path.exists(real) and os.path.exists(f) and _same_file(real, f))):
            raise UnsafePath("path aliases a protected source file")
    return real


def utcnow() -> datetime:
    return datetime.now(UTC)


def iso(ts: datetime) -> str:
    return ts.astimezone(UTC).isoformat().replace("+00:00", "Z")


def parse_ts(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def read_jsonl(path: str) -> list[dict]:
    """Complete, parseable object lines only; a partial last line is ignored (recovery moves it)."""
    out: list[dict] = []
    try:
        with open(path, "rb") as fh:
            data = fh.read()
    except FileNotFoundError:
        return out
    for raw in data.split(b"\n")[:-1]:  # the last element is "" or a partial line
        if not raw.strip():
            continue
        try:
            obj = json.loads(raw)
        except ValueError:
            continue
        if isinstance(obj, dict):
            out.append(obj)
    return out


@dataclass(frozen=True)
class Key:
    decision_id: str
    mode: str
    config_hash: str

    def matches(self, row: dict) -> bool:
        return row.get("decision_id") == self.decision_id and row.get("mode") == self.mode and row.get("config_hash") == self.config_hash


@dataclass(frozen=True)
class Due:
    action: str  # try | wait | give_up | terminal
    not_before: datetime | None = None
    failures: int = 0


class Store:
    def __init__(self, base: str | None = None, forbidden: Iterable[str] = ()) -> None:
        root = permitted_root()
        self.base = os.path.realpath(os.path.expanduser(base)) if base else root
        self.forbidden = tuple(forbidden)
        # Validate before anything is created or chmodded: the base is inside the permitted root
        # and neither it nor a parent aliases a protected source file.
        check_path(self.base, root, self.forbidden)
        for f in self.forbidden:
            if f and os.path.exists(f) and os.path.isdir(self.base) and _same_file(self.base, f):
                raise UnsafePath("state directory aliases a protected source file")
        self._lock_fd: int | None = None
        os.makedirs(self.base, mode=0o700, exist_ok=True)
        os.chmod(self.base, 0o700)
        self.attempts_path = self.path("attempts.jsonl")
        self.results_path = self.path("results.jsonl")
        self.lock_path = self.path("judge.lock")

    def path(self, name: str) -> str:
        return check_path(os.path.join(self.base, name), self.base, self.forbidden)

    def subdir(self, name: str) -> str:
        p = self.path(name)
        os.makedirs(p, mode=0o700, exist_ok=True)
        os.chmod(p, 0o700)
        return p

    # ---- locking and recovery ------------------------------------------------------

    @contextmanager
    def locked(self) -> Iterator["Store"]:
        fd = os.open(self.lock_path, os.O_RDWR | os.O_CREAT, 0o600)
        os.fchmod(fd, 0o600)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError as exc:
            os.close(fd)
            raise LockBusy("another judge run holds judge.lock") from exc
        self._lock_fd = fd
        try:
            self.recover()
            yield self
        finally:
            self._lock_fd = None
            fcntl.flock(fd, fcntl.LOCK_UN)
            os.close(fd)

    @contextmanager
    def held(self) -> Iterator["Store"]:
        """The run lock, taken only if this store does not hold it already. For code that writes
        shared state (the freeze and its cohort record) and may be called with or without it."""
        if self._lock_fd is not None:
            yield self
        else:
            with self.locked():
                yield self

    def recover(self) -> dict[str, int]:
        """Move a partial last line of each log into <name>.recovered. Returns bytes moved by log."""
        moved: dict[str, int] = {}
        for log in (self.attempts_path, self.results_path):
            if not os.path.exists(log):
                continue
            with open(log, "rb+") as fh:
                data = fh.read()
                if not data or data.endswith(b"\n"):
                    continue
                cut = data.rfind(b"\n") + 1
                tail = data[cut:]
                rec = self._open_recovered(log)
                try:
                    os.write(rec, tail + b"\n")
                    os.fsync(rec)
                finally:
                    os.close(rec)
                fh.truncate(cut)
                fh.flush()
                os.fsync(fh.fileno())
                moved[os.path.basename(log)] = len(tail)
        return moved

    def _open_recovered(self, log: str) -> int:
        """Open <log>.recovered for append. The destination is checked for containment and forbidden
        identity, a symlink is refused at open, and a file with other links or a looser mode is
        refused before a byte is written."""
        dest = os.path.join(self.base, os.path.basename(log) + ".recovered")
        check_path(dest, self.base, self.forbidden)
        if os.path.islink(dest):
            raise UnsafePath("recovery file is a symlink")
        try:
            fd = os.open(dest, os.O_WRONLY | os.O_CREAT | os.O_APPEND | os.O_NOFOLLOW, 0o600)
        except OSError as exc:
            raise UnsafePath(f"recovery file cannot be opened safely: {type(exc).__name__}") from exc
        try:
            st = os.fstat(fd)
            if not stat.S_ISREG(st.st_mode) or st.st_nlink != 1:
                raise UnsafePath("recovery file is not a plain single-link file")
            for f in self.forbidden:
                if f and os.path.exists(f) and os.path.samestat(st, os.stat(f)):
                    raise UnsafePath("recovery file aliases a protected source file")
            os.fchmod(fd, 0o600)
        except BaseException:
            os.close(fd)
            raise
        return fd

    def _append(self, path: str, row: dict) -> None:
        if self._lock_fd is None:
            raise NotLocked("append requires the run lock")
        line = (json.dumps(row, sort_keys=True, ensure_ascii=False, allow_nan=False) + "\n").encode()
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
        try:
            os.fchmod(fd, 0o600)
            os.write(fd, line)
            os.fsync(fd)
        finally:
            os.close(fd)

    # ---- attempts ------------------------------------------------------------------

    def log_attempt(self, key: Key, *, error_class: str | None, operational: bool, http_status: int | None = None,
                    text: object = "", now: datetime | None = None, **extra: Any) -> dict:
        row = {
            "ts": iso(now or utcnow()),
            "decision_id": key.decision_id,
            "mode": key.mode,
            "config_hash": key.config_hash,
            "error_class": error_class,
            "operational": operational,
            "http_status": http_status,
            "excerpt": excerpt(text, MAX_EXCERPT),
        }
        row.update({k: v for k, v in extra.items() if k not in row})
        self._append(self.attempts_path, row)
        return row

    def attempts(self, key: Key | None = None) -> list[dict]:
        rows = read_jsonl(self.attempts_path)
        return [r for r in rows if key is None or key.matches(r)]

    # ---- results -------------------------------------------------------------------

    def results(self) -> list[dict]:
        return read_jsonl(self.results_path)

    def terminal_keys(self) -> set[tuple[str, str, str]]:
        return {(r.get("decision_id"), r.get("mode"), r.get("config_hash")) for r in self.results()}

    def has_terminal(self, key: Key) -> bool:
        return (key.decision_id, key.mode, key.config_hash) in self.terminal_keys()

    def terminal_rows(self, key: Key) -> list[dict]:
        return [r for r in self.results() if (r.get("decision_id"), r.get("mode"), r.get("config_hash")) == (key.decision_id, key.mode, key.config_hash)]

    def write_terminal(self, key: Key, row: dict, now: datetime | None = None) -> bool:
        """Append the one terminal row for this key. False (and no write) when one exists."""
        if self.has_terminal(key):
            return False
        out = dict(row)
        out.update(decision_id=key.decision_id, mode=key.mode, config_hash=key.config_hash, ts=iso(now or utcnow()))
        if "excerpt" in out and out["excerpt"] is not None:
            out["excerpt"] = excerpt(out["excerpt"], MAX_EXCERPT)
        self._append(self.results_path, out)
        return True

    # ---- retry schedule ------------------------------------------------------------

    def due(self, key: Key, now: datetime | None = None) -> Due:
        """What to do next for a decision: terminal rows end it; operational errors back off 15m/1h/4h
        and give up 24h after the first failure."""
        now = now or utcnow()
        if self.has_terminal(key):
            return Due("terminal")
        ops = [r for r in self.attempts(key) if r.get("operational")]
        if not ops:
            return Due("try")
        first = parse_ts(ops[0]["ts"])
        last = parse_ts(ops[-1]["ts"])
        if now - first >= GIVE_UP_AFTER:
            return Due("give_up", failures=len(ops))
        wait = BACKOFF[min(len(ops) - 1, len(BACKOFF) - 1)]
        not_before = last + wait
        if now < not_before:
            return Due("wait", not_before=not_before, failures=len(ops))
        return Due("try", failures=len(ops))

    def give_up(self, key: Key, provenance: dict | None = None, now: datetime | None = None) -> bool:
        """The terminal row for a decision whose provider never answered. It carries the same
        manifest, split, revision and retrieval provenance as any other row, so a report counts it."""
        row = dict(provenance or {})
        row.update(verdict="held", reason="provider-unavailable")
        return self.write_terminal(key, row, now)

    def pending(self, eligible: Iterable[str], mode: str, config_hash: str) -> list[str]:
        """Eligible decision ids with no terminal row, in the order given."""
        done = self.terminal_keys()
        return [d for d in eligible if (d, mode, config_hash) not in done]
