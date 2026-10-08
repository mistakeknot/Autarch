// The Your move grouping (plan Revision 2 and 6): a pure function over `moves` rows. Nothing here
// reads the clock, the network or the database, and nothing is closed or changed.
import type { MoveRow } from "./store.js";

export interface MoveGroups {
  /** Open, unclaimed, not skipped, not hidden: what mk owes. */
  yourMove: MoveRow[];
  /** Claimed, or a report is present, but still open: reported done, not verified. */
  reported: MoveRow[];
  /** Skipped with "Later / skip": visible, never closed. */
  later: MoveRow[];
  /** Hidden by mk's own attention preference. */
  hidden: MoveRow[];
  /** Closed moves, newest first, with who/what closed them. */
  audit: { task_id: string; generation: number; kind: MoveRow["kind"]; closed_at: string | null; closed_by: string | null; evidence: string | null }[];
}

const byOpened = (a: MoveRow, b: MoveRow) => (a.opened_at < b.opened_at ? -1 : a.opened_at > b.opened_at ? 1 : a.task_id < b.task_id ? -1 : a.task_id > b.task_id ? 1 : a.generation - b.generation);

/**
 * Precedence for a live (not closed) move: hidden, then skipped (Later), then reported (claimed or a
 * report is present), else Your move. A hidden or skipped row is still counted by its own group.
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
