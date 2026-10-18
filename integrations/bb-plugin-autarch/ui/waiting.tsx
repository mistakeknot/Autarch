// The one count on screen: "Waiting on you: N", its parts and what sits beside it (bead mk-yjp7). The Queue tabs carry the numbers; "What this counts" lives beside them.
import type { Waiting } from "../waiting.js";

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** What sits beside the total, in order, each with where its button jumps to. */
function besideItems(w: Waiting): { to: WaitingJump; text: string }[] {
  const items: { to: WaitingJump; text: string }[] = [];
  if (w.updates > 0) items.push({ to: "updates", text: `${plural(w.updates, "update")} to read` });
  if (w.held > 0) items.push({ to: "held", text: `${w.held} on hold` });
  if (w.later > 0) items.push({ to: "later", text: `${w.later} for later` });
  return items;
}

export function waitingSummary(w: Waiting): { headline: string; parts: string; beside: string[] } {
  const parts = [w.decide > 0 ? `${w.decide} to decide` : null, w.moves > 0 ? plural(w.moves, "move") : null, w.notices > 0 ? plural(w.notices, "notice") : null].filter((x): x is string => x !== null);
  return {
    headline: `Waiting on you: ${w.total}`,
    parts: parts.length > 0 ? parts.join(", ") : "nothing",
    beside: besideItems(w).map((i) => i.text),
  };
}

/** The sidebar badge: the same total, "!" when serve is not ready or a machine blocker has no owner, nothing at zero. */
export function badgeCount(w: Pick<Waiting, "total">, o: { blocked: boolean; unowned: boolean }): string | null {
  if (o.blocked || o.unowned) return "!";
  return w.total > 0 ? String(w.total) : null;
}

export type WaitingJump = "decide" | "moves" | "updates" | "held" | "later";
