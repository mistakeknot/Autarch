// The rollback export of the retired Your move rows (`bb home moves`): the stored rows, grouped as the old view grouped
// them. Home opens, shows and closes no move any more (beads mk-1k8pg and mk-j5ipa.10); this only reads what is stored.
// A pure function over `moves` rows: nothing here reads the clock, the network or the database.
import type { MoveRow } from "./store.js";

export interface MoveGroups {
  /** Open, unclaimed, not skipped, not hidden. */
  yourMove: MoveRow[];
  /** Claimed, or a report is present, but still open. */
  reported: MoveRow[];
  /** Skipped with "Later / skip". */
  later: MoveRow[];
  /** Hidden by mk's own attention preference. */
  hidden: MoveRow[];
  /** Closed moves, newest first, with who/what closed them. */
  audit: { task_id: string; generation: number; kind: MoveRow["kind"]; closed_at: string | null; closed_by: string | null; evidence: string | null }[];
}

const byOpened = (a: MoveRow, b: MoveRow) => (a.opened_at < b.opened_at ? -1 : a.opened_at > b.opened_at ? 1 : a.task_id < b.task_id ? -1 : a.task_id > b.task_id ? 1 : a.generation - b.generation);

/**
 * Precedence for a live (not closed) move: hidden, then skipped (Later), then reported (claimed or a report is present), else Your move.
 */
export function groupMoves(rows: readonly MoveRow[]): MoveGroups {
  const g: MoveGroups = { yourMove: [], reported: [], later: [], hidden: [], audit: [] };
  for (const m of rows) {
    if (m.state === "closed") continue;
    if (m.hidden_at !== null || m.hidden_by !== null) g.hidden.push(m);
    else if (m.skipped_at !== null) g.later.push(m);
    else if (m.state === "claimed" || m.report_state !== null) g.reported.push(m);
    else g.yourMove.push(m);
  }
  for (const k of ["yourMove", "reported", "later", "hidden"] as const) g[k].sort(byOpened);
  g.audit = rows
    .filter((m) => m.state === "closed")
    .sort((a, b) => ((b.closed_at ?? "") < (a.closed_at ?? "") ? -1 : (b.closed_at ?? "") > (a.closed_at ?? "") ? 1 : 0))
    .map((m) => ({ task_id: m.task_id, generation: m.generation, kind: m.kind, closed_at: m.closed_at, closed_by: m.closed_by, evidence: m.evidence }));
  return g;
}
