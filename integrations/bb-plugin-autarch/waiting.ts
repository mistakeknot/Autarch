// One count of what is waiting on mk (bead mk-yjp7). It is computed from Home's own state, never from BB Tasks
// status (a task stays in backlog after mk picks, which made other lists overcount). Reading catch-up and held cards
// are shown beside the count and are not part of it.
import type { Catchup, CatchupItem } from "./catchup.js";
import type { Delegation } from "./delegation.js";
import { groupMoves } from "./moveselect.js";
import type { Service } from "./service.js";

/** Shown on screen beside the number so a mismatch with another list can be explained. */
export const WAITING_DEFINITION =
  "Counts open decisions (not on hold), open moves nobody has claimed, and notices you must acknowledge. Does not count updates to read, held cards, cards you set aside for later, or moves you already claimed or skipped.";

export interface WaitingNotice {
  item: string;
  at: string;
  text: string;
}
export interface Waiting {
  total: number;
  /** Owed decisions that are not on hold. */
  decide: number;
  /** Open, unclaimed, unskipped, unhidden moves on cards with no owed decision of their own. */
  moves: number;
  /** Unseen items that change what Home does. */
  notices: number;
  noticeItems: WaitingNotice[];
  /** Unseen catch-up rows that are reading only (shown beside the count). */
  updates: number;
  /** Owed decisions and open moves on hold (shown beside the count). */
  held: number;
  /** Open cards mk set aside for later (shown beside the count, never in it). A card with a decision and a move is one. */
  later: number;
  suspended: boolean;
  definition: string;
}

export function waitingNow(svc: Service, dele: Delegation, catchup: Catchup): Waiting {
  const owed = svc.owed();
  const live = owed.filter((d) => !svc.holdOf(d.task_id as string | null));
  const heldTasks = new Set<string>();
  for (const d of owed) if (d.task_id && svc.holdOf(d.task_id as string)) heldTasks.add(d.task_id as string);
  // A hold beats Later: a held card is counted in `held` only.
  const laterTasks = new Set<string>();
  const isLater = (t: string | null) => !!t && !svc.holdOf(t) && !!svc.laterOf(t);
  const active = live.filter((d) => {
    if (!isLater(d.task_id as string | null)) return true;
    laterTasks.add(d.task_id as string);
    return false;
  });
  const decideTasks = new Set(active.map((d) => d.task_id as string | null).filter((t): t is string => !!t));

  const moves = new Set<string>();
  for (const m of groupMoves(svc.store.moves()).yourMove) {
    if (svc.holdOf(m.task_id)) {
      heldTasks.add(m.task_id);
      continue;
    }
    if (isLater(m.task_id)) {
      laterTasks.add(m.task_id);
      continue;
    }
    if (!decideTasks.has(m.task_id)) moves.add(m.task_id);
  }

  const all: CatchupItem[] = catchup.items();
  const noticeItems = all.filter((i) => i.kind === "notice").map((i) => ({ item: i.item, at: i.at, text: i.text }));
  const updates = all.filter((i) => i.kind !== "owed" && i.kind !== "notice").length;
  const decide = active.length;
  return {
    total: decide + moves.size + noticeItems.length,
    decide,
    moves: moves.size,
    notices: noticeItems.length,
    noticeItems,
    updates,
    held: heldTasks.size,
    later: laterTasks.size,
    suspended: dele.suspendedNow(),
    definition: WAITING_DEFINITION,
  };
}
