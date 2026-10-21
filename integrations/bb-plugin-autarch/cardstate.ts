// What state a card key is in, for the deep link /plugins/autarch/home/<KEY>.
import type Database from "better-sqlite3";

export type CardState = { found: false } | { found: true; state: string; title: string };

/** The newest live card with this key (a deleted card is not reported). An open card wins over a closed one with the same key. */
export function cardStateOf(db: Database.Database, key: string): CardState {
  const row = db
    .prepare("SELECT state, title FROM cards WHERE card_key = ? AND deleted_at IS NULL ORDER BY (state = 'open') DESC, updated_at DESC LIMIT 1")
    .get(key) as { state: string; title: string | null } | undefined;
  return row ? { found: true, state: row.state, title: row.title ?? "" } : { found: false };
}
