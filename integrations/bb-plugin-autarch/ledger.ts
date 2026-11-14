// The vizier's rulings ledger: one JSON object per line, appended when a pick is ruled.
//
// The vizier logs its own rulings by hand with ids q1, q2, ... Every entry written here has the id
// `home-<pick_id>`, which can never be a q-number, and carries the pick id so a retry of the same pick
// finds its line instead of adding a second one.
import { closeSync, fstatSync, mkdirSync, openSync, readFileSync, readSync, writeSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

export interface LedgerPick {
  declaration?: { decision_id: string; pick_id: string; generation: number; revision: string; on_pick_sha256: string; kind: string; pr: string | null; head: string | null; bead: string | null; under: string[] };
  pick_id: string;
  picked_at: string;
  by: string;
  surface: string;
  /** The card key, else the decision id. */
  card: string;
  subject: string;
  option_id: string;
  option_label: string;
  /** The reason mk or the vizier gave with the pick. */
  reason?: string | null;
}

export const LEDGER_ENV = "AUTARCH_RULINGS_LEDGER";

/** Where the ledger lives: $AUTARCH_RULINGS_LEDGER, else ~/.local/state/vizier/rulings.jsonl. "off" disables it. */
export function ledgerPath(env: NodeJS.ProcessEnv = process.env): string | null {
  const v = env[LEDGER_ENV];
  if (v === "off") return null;
  return v && v !== "" ? v : join(homedir(), ".local", "state", "vizier", "rulings.jsonl");
}

export function ledgerId(pickId: string): string {
  return `home-${pickId}`;
}

function entryOf(p: LedgerPick) {
  const words = [p.option_label, p.reason ? p.reason.replace(/\s+/g, " ").trim() : ""].filter((s) => s !== "").join(": ");
  return {
    id: ledgerId(p.pick_id),
    ts: p.picked_at,
    by: p.by,
    surface: p.surface,
    card: p.card,
    subject: p.subject,
    option: p.option_id,
    words,
    ...(p.declaration ?? {}),
  };
}

function has(text: string, id: string): boolean {
  for (const line of text.split("\n")) {
    if (!line.includes(id)) continue;
    try {
      if ((JSON.parse(line) as { id?: unknown }).id === id) return true;
    } catch {
      /* a line the vizier is still writing is not ours */
    }
  }
  return false;
}

/**
 * Append the pick's ruling unless the ledger already has it. Returns true when a line was written.
 * One O_APPEND write of the whole line; a missing final newline (a hand edit) is repaired first so the new line
 * never joins the old one. Two writers racing on the same pick are not possible: the one reconcile loop serialises them.
 */
export function appendRuling(path: string, p: LedgerPick): boolean {
  const entry = entryOf(p);
  mkdirSync(dirname(path), { recursive: true });
  const fd = openSync(path, "a+");
  try {
    const size = fstatSync(fd).size;
    if (size > 0) {
      if (has(readFileSync(path, "utf8"), entry.id)) return false;
      const last = Buffer.alloc(1);
      readSync(fd, last, 0, 1, size - 1);
      if (last[0] !== 0x0a) writeSync(fd, "\n");
    }
    writeSync(fd, `${JSON.stringify(entry)}\n`);
    return true;
  } finally {
    closeSync(fd);
  }
}

export type LedgerReadback = "matching" | "missing" | "conflicting" | "disabled" | "unknown";

function sameJson(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    return a.every((v, i) => sameJson(v, b[i]));
  }
  if (a === null || b === null || typeof a !== "object" || typeof b !== "object") return false;
  const ka = Object.keys(a).sort();
  const kb = Object.keys(b).sort();
  if (ka.length !== kb.length || ka.some((k, i) => k !== kb[i])) return false;
  return ka.every((k) => sameJson((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]));
}

/**
 * Could an unparseable line be (part of) the selected row? Every JSON string literal in it is decoded, so an id spelled
 * with escapes is still recognised; a torn final literal counts when it decodes to a prefix of the id. Lines with no such
 * literal are someone else's and are neither repaired nor read as evidence.
 */
function mayNameId(line: string, id: string): boolean {
  const literal = /"((?:[^"\\]|\\.)*)("?)/g;
  for (let m = literal.exec(line); m !== null; m = literal.exec(line)) {
    const body = m[1] ?? "";
    const closed = m[2] === '"';
    let decoded: string | null = null;
    try {
      decoded = JSON.parse(`"${closed ? body : body.replace(/\\(u[0-9a-fA-F]{0,3})?$/, "")}"`) as string;
    } catch {
      decoded = null;
    }
    if (decoded === null) {
      if (body.includes("\\")) return true; // an escape we cannot decode: it may spell the id
      continue;
    }
    if (decoded === id || (!closed && decoded !== "" && id.startsWith(decoded))) return true;
    if (!closed) break;
  }
  return false;
}

/**
 * Focused readback of the selected pick's ruling: is the complete row appendRuling would write for this pick present,
 * readable and unique? "disabled" when the ledger is off, "missing" when no line carries the id, "conflicting" when a line
 * with the id differs from the expected row, "unknown" when the file cannot be read or a line that may be the selected row
 * does not parse. Read-only: nothing is repaired, locked or appended.
 */
export function readMatchingRuling(path: string | null, pick: LedgerPick): LedgerReadback {
  if (path === null) return "disabled";
  let text: string;
  try {
    text = readFileSync(path, "utf8");
  } catch (err) {
    return (err as NodeJS.ErrnoException).code === "ENOENT" ? "missing" : "unknown";
  }
  // The row as it is stored: undefined members vanish exactly as they do in appendRuling's JSON.stringify.
  const expected = JSON.parse(JSON.stringify(entryOf(pick))) as ReturnType<typeof entryOf>;
  let matched = false;
  let conflicting = false;
  for (const line of text.split("\n")) {
    if (line.trim() === "") continue;
    let row: unknown;
    try {
      row = JSON.parse(line);
    } catch {
      if (mayNameId(line, expected.id)) return "unknown";
      continue;
    }
    if (row === null || typeof row !== "object" || Array.isArray(row)) continue;
    if ((row as { id?: unknown }).id !== expected.id) continue;
    if (sameJson(row, expected)) matched = true;
    else conflicting = true;
  }
  if (conflicting) return "conflicting";
  return matched ? "matching" : "missing";
}
