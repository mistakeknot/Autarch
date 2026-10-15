// The Learn deck (bead mk-uoiwp): eight Markdown cards shipped with the plugin, off by default. The only state is in
// Home's settings table (`settings_kv`): `learn.enabled` and one `learn.card.<id>` per card. No schema change.
// Nothing here rules, closes or starts anything.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

export type LearnMark = "got" | "later";
export type LearnCard = { id: string; title: string; body: string; mark: LearnMark | null };
export type LearnSettings = { setting(key: string): string | undefined; setSetting(key: string, value: string): void };

const ENABLED = "learn.enabled";
const CARD_FILE = /^(\d{2}-[a-z0-9-]+)\.md$/;

/** The cards in file-name order. A file that is not `NN-name.md`, or has no `# Title` first line, is skipped. */
export function loadDeck(dir: string): Array<{ id: string; title: string; body: string }> {
  let names: string[];
  try {
    names = readdirSync(dir).sort();
  } catch {
    return [];
  }
  const out: Array<{ id: string; title: string; body: string }> = [];
  for (const n of names) {
    const m = CARD_FILE.exec(n);
    if (!m) continue;
    let text: string;
    try {
      text = readFileSync(join(dir, n), "utf8").replace(/\r\n?/g, "\n");
    } catch {
      continue;
    }
    const first = text.split("\n", 1)[0] ?? "";
    if (!first.startsWith("# ") || first.length < 3) continue;
    out.push({ id: m[1]!, title: first.slice(2).trim(), body: text.slice(first.length).trim() });
  }
  return out;
}

export const learnEnabled = (s: LearnSettings): boolean => s.setting(ENABLED) === "1";

export function learnView(s: LearnSettings, dir: string): { enabled: boolean; cards: LearnCard[] } {
  const enabled = learnEnabled(s);
  if (!enabled) return { enabled, cards: [] };
  const cards = loadDeck(dir).map((c) => {
    const v = s.setting(`learn.card.${c.id}`);
    return { ...c, mark: v === "got" || v === "later" ? v : null } satisfies LearnCard;
  });
  return { enabled, cards };
}

export type LearnSet = { enabled?: boolean | undefined; card?: string | undefined; mark?: LearnMark | "clear" | undefined };

export function learnSet(s: LearnSettings, dir: string, i: LearnSet): { ok: true } | { ok: false; status: number; error: string } {
  if (i.enabled === undefined && i.card === undefined) return { ok: false, status: 400, error: "nothing to set" };
  if (i.card !== undefined) {
    if (i.mark === undefined) return { ok: false, status: 400, error: "a card needs a mark" };
    if (!loadDeck(dir).some((c) => c.id === i.card)) return { ok: false, status: 404, error: "no such card" };
  }
  if (i.enabled !== undefined) s.setSetting(ENABLED, i.enabled ? "1" : "0");
  if (i.card !== undefined) s.setSetting(`learn.card.${i.card}`, i.mark === "clear" ? "" : i.mark!);
  return { ok: true };
}
