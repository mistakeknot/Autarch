// How long a decision-card click takes on mk's own screen (bead mk-yaq27): click to painted card, and each RPC the
// click starts. Display-only: the numbers go to the plugin log through one RPC and nothing reads them back into the UI.

export interface ClickTiming {
  kind: "paint" | "setViewing" | "conversation" | "pick" | "later";
  ms: number;
  width: number;
}

let sink: ((t: ClickTiming) => void) | null = null;
let clickAt: number | null = null;

export function setTimingSink(fn: ((t: ClickTiming) => void) | null): void {
  sink = fn;
}

const now = (): number => (typeof performance !== "undefined" ? performance.now() : Date.now());
const width = (): number => (typeof window !== "undefined" ? Math.round(window.innerWidth) : 0);

function report(kind: ClickTiming["kind"], ms: number): void {
  try {
    sink?.({ kind, ms: Math.round(ms), width: width() });
  } catch {
    /* timing must never break a click */
  }
}

/** Called in the click handler, before the selection state changes. */
export function clickStarted(): void {
  clickAt = now();
}

/** Called from an effect after the selected card has rendered; reports once the browser has painted it. */
export function paintDone(): void {
  if (clickAt === null) return;
  const start = clickAt;
  clickAt = null;
  if (typeof requestAnimationFrame !== "function") return report("paint", now() - start);
  requestAnimationFrame(() => requestAnimationFrame(() => report("paint", now() - start)));
}

/** Wraps an RPC promise: reports how long it took to settle, and returns it unchanged. */
export function timed<T>(kind: ClickTiming["kind"], p: Promise<T>): Promise<T> {
  const start = now();
  const done = () => report(kind, now() - start);
  p.then(done, done);
  return p;
}
