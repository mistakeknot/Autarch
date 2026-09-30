// Catch-up: what happened while mk was away, marked seen only when actually seen.

export type CatchupEntry = {
  item: string;
  kind: "failure" | "owed" | "delegated" | "routine" | "note";
  at: string;
  text: string;
  project?: string;
  decision?: string;
  members?: string[];
  cites?: string[];
};

export const SEEN_AFTER_MS = 1000;

type Timer = { ids: string[]; remaining: number; startedAt: number | null; handle: ReturnType<typeof setTimeout> | null };

/** Marks an expanded item after 1 s of continuous visibility; hidden or unfocused pauses the clock. */
export class SeenTracker {
  private active = false;
  private timers = new Map<string, Timer>();
  constructor(private markSeen: (id: string) => void) {}

  expand(id: string, members?: string[]): void {
    if (id.startsWith("owed:") || this.timers.has(id)) return; // owed items are answered, never seen
    const t: Timer = { ids: members ?? [id], remaining: SEEN_AFTER_MS, startedAt: null, handle: null };
    this.timers.set(id, t);
    if (this.active) this.run(id, t);
  }

  collapse(id: string): void {
    const t = this.timers.get(id);
    if (t?.handle) clearTimeout(t.handle);
    this.timers.delete(id);
  }

  setActive(active: boolean): void {
    if (active === this.active) return;
    this.active = active;
    for (const [id, t] of this.timers) {
      if (active) this.run(id, t);
      else this.pause(t);
    }
  }

  private run(id: string, t: Timer): void {
    t.startedAt = Date.now();
    t.handle = setTimeout(() => {
      this.timers.delete(id);
      for (const m of t.ids) this.markSeen(m);
    }, t.remaining);
  }

  private pause(t: Timer): void {
    if (t.handle) clearTimeout(t.handle);
    if (t.startedAt !== null) t.remaining = Math.max(0, t.remaining - (Date.now() - t.startedAt));
    t.handle = null;
    t.startedAt = null;
  }
}

/** The ids "mark all seen" may send: only what is expanded and on screen, never an owed item. */
export function snapshotIds(items: CatchupEntry[], expanded: Set<string>, visible: Set<string>): string[] {
  const out: string[] = [];
  for (const e of items) {
    if (e.kind === "owed" || !expanded.has(e.item) || !visible.has(e.item)) continue;
    out.push(...(e.members ?? [e.item]));
  }
  return out;
}

export function CatchupPanel({
  items,
  expanded,
  onToggle,
  onOverride,
  onMarkAll,
}: {
  items: CatchupEntry[];
  expanded: Set<string>;
  onToggle: (item: string, members?: string[]) => void;
  onOverride: (decision: string) => void;
  onMarkAll: () => void;
}) {
  return (
    <div className="space-y-2 p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase text-muted-foreground">Catch-up</h2>
        <button type="button" className="text-xs underline" onClick={onMarkAll}>mark all seen</button>
      </div>
      {items.length === 0 ? <p className="text-sm text-muted-foreground">You are caught up.</p> : null}
      {items.map((e) => (
        <div key={e.item} className="rounded border border-border p-3 text-sm" data-kind={e.kind} data-expanded={expanded.has(e.item) ? "true" : "false"}>
          <button type="button" className="text-left" onClick={() => onToggle(e.item, e.members)}>{e.text}</button>
          {expanded.has(e.item) && e.cites && e.cites.length > 0 ? <div className="text-xs text-muted-foreground">{`cites ${e.cites.join(", ")}`}</div> : null}
          {e.kind === "delegated" && e.decision !== undefined ? (
            <button type="button" className="ml-2 text-xs underline" onClick={() => onOverride(e.decision!)}>Override</button>
          ) : null}
        </div>
      ))}
    </div>
  );
}
