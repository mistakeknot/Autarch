// Reader for a coordinator's `settled-for-mk.md` (bead mk-p02ft): the asks it settled itself instead of sending to mk,
// so `bb home stats --settled-file PATH` can show them beside the ask counts. Read-only: it opens one file O_RDONLY |
// O_NOFOLLOW | O_NONBLOCK (so a FIFO is refused after fstat instead of blocking the open), writes nothing and touches no Home table. An entry is a line after the `#` header that starts with
// `YYYY-MM-DD |`; every other line is ignored.
import { constants as fsc, closeSync, fstatSync, openSync, readSync } from "node:fs";
import { basename, isAbsolute } from "node:path";
import { isoWeek } from "./service.js";

export const SETTLED_FILE = "settled-for-mk.md";
export const SETTLED_MAX_BYTES = 1024 * 1024;
const ENTRY = /^(\d{4}-\d\d-\d\d) \|/;

export function countSettled(text: string): { total: number; dates: string[] } {
  const dates: string[] = [];
  for (const line of text.split(/\r?\n/)) {
    const m = ENTRY.exec(line);
    if (m) dates.push(m[1]!);
  }
  return { total: dates.length, dates };
}

export type SettledResult =
  | { ok: true; path: string; total: number; count: number; first: string | null; last: string | null; per_week: Record<string, number> }
  | { ok: false; path: string; error: string };

/** Entries in the file; `since` (an ISO time) keeps those dated on or after its UTC day. `total` is always the whole file. */
export function readSettled(path: string, since?: string): SettledResult {
  const fail = (error: string): SettledResult => ({ ok: false, path, error });
  if (!isAbsolute(path)) return fail("the path must be absolute");
  if (basename(path) !== SETTLED_FILE) return fail(`the file must be named ${SETTLED_FILE}`);
  let text: string;
  let fd = -1;
  try {
    fd = openSync(path, fsc.O_RDONLY | fsc.O_NOFOLLOW | fsc.O_NONBLOCK);
    const st = fstatSync(fd);
    if (!st.isFile()) return fail("not a regular file");
    if (st.size > SETTLED_MAX_BYTES) return fail("file too large");
    const buf = Buffer.alloc(st.size);
    let off = 0;
    while (off < buf.length) {
      const n = readSync(fd, buf, off, buf.length - off, off);
      if (n === 0) break;
      off += n;
    }
    text = buf.subarray(0, off).toString("utf8");
  } catch {
    return fail("cannot read the file");
  } finally {
    if (fd >= 0) closeSync(fd);
  }
  const { total, dates } = countSettled(text);
  const from = since?.slice(0, 10);
  const inWindow = (from ? dates.filter((d) => d >= from) : dates).sort();
  const per_week: Record<string, number> = {};
  for (const d of inWindow) {
    const ms = Date.parse(d);
    if (!Number.isNaN(ms)) per_week[isoWeek(ms)] = (per_week[isoWeek(ms)] ?? 0) + 1;
  }
  return { ok: true, path, total, count: inWindow.length, first: inWindow[0] ?? null, last: inWindow[inWindow.length - 1] ?? null, per_week };
}
