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
  private hidden = new Set<string>();
  constructor(private markSeen: (id: string) => void) {}

  expand(id: string, members?: string[]): void {
    if (id.startsWith("owed:") || this.timers.has(id)) return; // owed items are answered, never seen
    const t: Timer = { ids: members ?? [id], remaining: SEEN_AFTER_MS, startedAt: null, handle: null };
    this.timers.set(id, t);
    if (this.active && !this.hidden.has(id)) this.run(id, t);
  }

  /** Whether an item is on screen; items never reported are treated as visible. */
  isVisible(id: string): boolean {
    return !this.hidden.has(id);
  }

  /** An item leaving the viewport pauses its clock; returning resumes it. */
  setVisible(id: string, visible: boolean): void {
    if (visible === !this.hidden.has(id)) return;
    if (visible) this.hidden.delete(id);
    else this.hidden.add(id);
    const t = this.timers.get(id);
    if (!t || !this.active) return;
    if (visible) this.run(id, t);
    else this.pause(t);
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
      if (active) {
        if (!this.hidden.has(id)) this.run(id, t);
      } else this.pause(t);
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

const observers = new WeakMap<Element, IntersectionObserver>();

/** A ref callback that reports viewport visibility; without IntersectionObserver every item counts as visible. */
function watchVisibility(item: string, report?: (item: string, visible: boolean) => void) {
  return (el: HTMLDivElement | null) => {
    if (!report || !el || typeof IntersectionObserver === "undefined") return;
    if (observers.has(el)) return;
    const o = new IntersectionObserver((entries) => {
      for (const en of entries) report(item, en.isIntersecting);
    });
    observers.set(el, o);
    o.observe(el);
  };
}

export function CatchupPanel({
  items,
  expanded,
  onToggle,
  onOverride,
  onMarkAll,
  onVisibility,
}: {
  items: CatchupEntry[];
  expanded: Set<string>;
  onToggle: (item: string, members?: string[]) => void;
  onOverride: (decision: string) => void;
  onMarkAll: () => void;
  /** Reports each item entering or leaving the viewport. */
  onVisibility?: (item: string, visible: boolean) => void;
}) {
  return (
    <div className="space-y-2 p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase text-muted-foreground">Catch-up</h2>
        <button type="button" className="text-xs underline" onClick={onMarkAll}>mark all seen</button>
      </div>
      {items.length === 0 ? <p className="text-sm text-muted-foreground">You are caught up.</p> : null}
      {items.map((e) => (
        <div key={e.item} ref={watchVisibility(e.item, onVisibility)} className="rounded border border-border p-3 text-sm" data-kind={e.kind} data-expanded={expanded.has(e.item) ? "true" : "false"}>
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
