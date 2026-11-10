// One count of what is waiting on mk (beads mk-yjp7, mk-j5ipa.9). It is computed from Home's own state, never from BB Tasks
// status (a task stays in backlog after mk picks, which made other lists overcount). It is the open decision cards that
// need a pick now (waitingcount.ts). Notices, reading catch-up, held cards and Later cards are named beside it and are not
// part of it.
import type { Catchup, CatchupItem } from "./catchup.js";
import type { Delegation } from "./delegation.js";
import type { Service } from "./service.js";
import { waitingLine } from "./waitingcount.js";

/** Shown on screen beside the number so a mismatch with another list can be explained. */
export const WAITING_DEFINITION =
  "Counts open decisions that need your pick now. Does not count cards on hold or set aside for later, notices to acknowledge, or updates to read; those are named beside it.";

export interface WaitingNotice {
  item: string;
  at: string;
  text: string;
}
export interface Waiting {
  /** Decisions that need a pick now: the same number as `decide`, kept under its old name for readers of `bb home stats`. Notices and moves are named below, never added in. */
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
  /** The count with held and Later by name, a zero part left out: "10 · 12 on hold · 8 later". */
  line: string;
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

  // Held and Later are counted over the owed decision cards only, the same population the phone and the Later tab list, so the
  // named counts agree on every screen. A move-only card is counted nowhere (moves are being retired), so `moves` stays 0.
  const moves = new Set<string>();

  const all: CatchupItem[] = catchup.items();
  const noticeItems = all.filter((i) => i.kind === "notice").map((i) => ({ item: i.item, at: i.at, text: i.text }));
  const updates = all.filter((i) => i.kind !== "owed" && i.kind !== "notice").length;
  const decide = active.length;
  return {
    total: decide,
    decide,
    moves: moves.size,
    notices: noticeItems.length,
    noticeItems,
    updates,
    held: heldTasks.size,
    later: laterTasks.size,
    suspended: dele.suspendedNow(),
    definition: WAITING_DEFINITION,
    line: waitingLine({ pick: decide, held: heldTasks.size, later: laterTasks.size }),
  };
}
