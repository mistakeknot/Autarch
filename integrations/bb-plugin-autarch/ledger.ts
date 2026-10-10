// The vizier's rulings ledger: one JSON object per line, appended when a pick is ruled.
//
// The vizier logs its own rulings by hand with ids q1, q2, ... Every entry written here has the id
// `home-<pick_id>`, which can never be a q-number, and carries the pick id so a retry of the same pick
// finds its line instead of adding a second one.
import { closeSync, fstatSync, mkdirSync, openSync, readFileSync, readSync, writeSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

export interface LedgerPick {
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

