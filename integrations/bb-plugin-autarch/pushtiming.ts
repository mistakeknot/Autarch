// Read-only views over the blocker push (bead mk-tv70n.7): which decisions of one ask have already pushed, and how
// long mk took to answer after the filing and after the push.
//
// "One ask" is every generation of a card (the rows sharing a task_id; an edit, a supersede or a reopen only adds a
// generation) or, for an ask filed before cards, the chain of rows linked by supersedes (which is linear: supersedes
// is unique).
//
// The first answer is read from the real schema as follows: a decide ask is answered by its earliest picks.picked_at
// (picks has one row per decision, so across generations the earliest); a steps or machine ask has no pick and is
// answered when it is marked resolved, decisions.resolved_at (written by the asks resolve verb).

import type Database from "better-sqlite3";

interface Row {
  id: string;
  kind: string;
  task_id: string | null;
  supersedes: string | null;
  filed_at: string;
  resolved_at: string | null;
  urgency: string | null;
  push_sent_at: string | null;
  push_id: string | null;
  push_capped: number | null;
}

const COLS = "id, kind, task_id, supersedes, filed_at, resolved_at, urgency, push_sent_at, push_id, push_capped";

/** Every row of the same ask as decisionId, oldest filing first. Empty when the decision is unknown. */
export function askRows(db: Database.Database, decisionId: string): Row[] {
  const me = db.prepare(`SELECT ${COLS} FROM decisions WHERE id = ?`).get(decisionId) as Row | undefined;
  if (!me) return [];
  if (me.task_id) return db.prepare(`SELECT ${COLS} FROM decisions WHERE task_id = ? ORDER BY filed_at, id`).all(me.task_id) as Row[];
  // A chain: up to the root through supersedes, then down through the rows that supersede it.
  return db
    .prepare(
      `WITH RECURSIVE up(id, supersedes) AS (
         SELECT id, supersedes FROM decisions WHERE id = ?
         UNION ALL SELECT d.id, d.supersedes FROM decisions d JOIN up ON d.id = up.supersedes
       ), root(id) AS (SELECT id FROM up WHERE supersedes IS NULL),
       down(id) AS (
         SELECT id FROM root
         UNION ALL SELECT d.id FROM decisions d JOIN down ON d.supersedes = down.id
       )
       SELECT ${COLS.split(", ").map((c) => `d.${c}`).join(", ")} FROM decisions d JOIN down ON d.id = down.id ORDER BY d.filed_at, d.id`,
    )
    .all(decisionId) as Row[];
}

/** True when some row of this ask already recorded a push (sent, capped or deduped by the notifier). */
export function alreadyPushed(db: Database.Database, decisionId: string): boolean {
  return askRows(db, decisionId).some((r) => r.push_sent_at !== null);
}

export interface TimeToAnswer {
  filed_at: string;
  push_sent_at: string | null;
  push_id: string | null;
  push_capped: boolean | null;
  /** The earliest answer: a pick for a decide ask, resolved_at for a steps or machine ask. Null while unanswered. */
  answered_at: string | null;
  answered_by: "pick" | "resolved" | null;
  filed_to_answer_ms: number | null;
  push_to_answer_ms: number | null;
}

const ms = (from: string | null, to: string | null): number | null => {
  if (!from || !to) return null;
  const a = Date.parse(from);
  const b = Date.parse(to);
  return Number.isNaN(a) || Number.isNaN(b) ? null : b - a;
};

const earliest = (xs: (string | null)[]): string | null => xs.filter((x): x is string => x !== null).sort((a, b) => Date.parse(a) - Date.parse(b))[0] ?? null;

/** Time to answer for the ask a decision belongs to; undefined when the decision is unknown. */
export function timeToAnswer(db: Database.Database, decisionId: string): TimeToAnswer | undefined {
  const rows = askRows(db, decisionId);
  if (rows.length === 0) return undefined;
  const pushed = rows.find((r) => r.push_sent_at !== null);
  const pickable = rows.some((r) => r.kind === "decide");
  let answered_at: string | null;
  let answered_by: TimeToAnswer["answered_by"];
  if (pickable) {
    const marks = rows.map(() => "?").join(",");
    const picks = db.prepare(`SELECT picked_at FROM picks WHERE decision_id IN (${marks})`).all(...rows.map((r) => r.id)) as { picked_at: string }[];
    answered_at = earliest(picks.map((p) => p.picked_at));
    answered_by = answered_at ? "pick" : null;
  } else {
    answered_at = earliest(rows.map((r) => r.resolved_at));
    answered_by = answered_at ? "resolved" : null;
  }
  const filed_at = rows[0]!.filed_at;
  const push_sent_at = pushed?.push_sent_at ?? null;
  return {
    filed_at,
    push_sent_at,
    push_id: pushed?.push_id ?? null,
    push_capped: pushed ? pushed.push_capped === 1 : null,
    answered_at,
    answered_by,
    filed_to_answer_ms: ms(filed_at, answered_at),
    push_to_answer_ms: ms(push_sent_at, answered_at),
  };
}

export interface PushListing extends TimeToAnswer {
  decision_id: string;
  card_key: string | null;
  project: string;
  kind: string;
}

/** One entry per blocker ask (the first generation that carries urgency), newest filing first. */
export function listPushes(db: Database.Database, limit = 100): PushListing[] {
  const heads = db
    .prepare(
      `SELECT d.id, d.project, d.kind, c.card_key FROM decisions d LEFT JOIN cards c ON c.task_id = d.task_id
       WHERE d.urgency = 'blocker'
         AND d.id = (SELECT x.id FROM decisions x WHERE x.urgency = 'blocker' AND COALESCE(x.task_id, x.id) = COALESCE(d.task_id, d.id) ORDER BY x.filed_at, x.id LIMIT 1)
       ORDER BY d.filed_at DESC, d.id DESC LIMIT ?`,
    )
    .all(Math.max(1, Math.min(limit, 1000))) as { id: string; project: string; kind: string; card_key: string | null }[];
  const out: PushListing[] = [];
  for (const h of heads) {
    const t = timeToAnswer(db, h.id);
    if (t) out.push({ decision_id: h.id, card_key: h.card_key, project: h.project, kind: h.kind, ...t });
  }
  return out;
}
