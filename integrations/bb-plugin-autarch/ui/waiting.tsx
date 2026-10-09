// The one count on screen: "Waiting on you: N", its parts, what sits beside it, and what it counts (bead mk-yjp7).
import type { Waiting } from "../waiting.js";
import { ActionButton } from "./buttons.js";

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

export function waitingSummary(w: Waiting): { headline: string; parts: string; beside: string[] } {
  const parts = [w.decide > 0 ? `${w.decide} to decide` : null, w.moves > 0 ? plural(w.moves, "move") : null, w.notices > 0 ? plural(w.notices, "notice") : null].filter((x): x is string => x !== null);
  return {
    headline: `Waiting on you: ${w.total}`,
    parts: parts.length > 0 ? parts.join(", ") : "nothing",
    beside: [w.updates > 0 ? `${plural(w.updates, "update")} to read` : null, w.held > 0 ? `${w.held} on hold` : null].filter((x): x is string => x !== null),
  };
}

/** The sidebar badge: the same total, "!" when serve is not ready or a machine blocker has no owner, nothing at zero. */
export function badgeCount(w: Pick<Waiting, "total">, o: { blocked: boolean; unowned: boolean }): string | null {
  if (o.blocked || o.unowned) return "!";
  return w.total > 0 ? String(w.total) : null;
}

export type WaitingJump = "decide" | "moves" | "updates" | "held";

export function WaitingStrip({ waiting: w, onJump }: { waiting: Waiting; onJump: (to: WaitingJump) => void }) {
  const s = waitingSummary(w);
  return (
    <div className="border-b border-border px-2 py-2 sm:px-4" data-waiting-strip>
      <p className="m-0 text-sm [overflow-wrap:anywhere]">
        <b data-waiting-total>{s.headline}</b>
        <span className="text-muted-foreground">{` — ${s.parts}`}</span>
      </p>
      {s.beside.length > 0 ? (
        <p className="m-0 mt-1 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground" data-waiting-beside>
          <span>Not in that number:</span>
          {w.updates > 0 ? <ActionButton tone="quiet" className="min-h-11 sm:min-h-0 !px-1 !py-0" onClick={() => onJump("updates")}>{s.beside[0]}</ActionButton> : null}
          {w.held > 0 ? <ActionButton tone="quiet" className="min-h-11 sm:min-h-0 !px-1 !py-0" onClick={() => onJump("held")}>{`${w.held} on hold`}</ActionButton> : null}
        </p>
      ) : null}
      <details className="mt-1 text-xs text-muted-foreground" data-waiting-definition>
        <summary className="flex min-h-11 cursor-pointer items-center sm:min-h-0">What this counts</summary>
        <p className="m-0 mt-1 [overflow-wrap:anywhere]">{w.definition}</p>
      </details>
    </div>
  );
}
