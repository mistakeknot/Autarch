// Catch-up: what happened while mk was away, marked seen only when actually seen.
import { useEffect, useRef } from "react";
import { ActionButton } from "./buttons.js";

export type CatchupEntry = {
  item: string;
  kind: "failure" | "owed" | "delegated" | "routine" | "note" | "notice";
  at: string;
  text: string;
  project?: string;
  decision?: string;
  members?: string[];
  /** One line per routine member, shown when the group is opened. */
  lines?: string[];
  cites?: string[];
};

export const SEEN_AFTER_MS = 1000;

/** Items that change what Home does (they suspend delegated rulings). An older server sends them as `delegated`, so the id decides too. */
const NOTICE_ID = /^(vizier-adopted|delegation-settings):/;
export function isNoticeItem(e: { item: string; kind: string }): boolean {
  return e.kind === "notice" || NOTICE_ID.test(e.item);
}
/** Whatever the server's version, a notice arrives as kind "notice": acknowledged on purpose, never marked by dwell or Mark all. */
export function normalizeCatchup(items: CatchupEntry[]): CatchupEntry[] {
  return items.map((e) => (e.kind !== "notice" && isNoticeItem(e) ? { ...e, kind: "notice" as const } : e));
}

type Timer = { ids: string[]; remaining: number; startedAt: number | null; handle: ReturnType<typeof setTimeout> | null };

/** Marks an expanded item after 1 s of continuous visibility; hidden or unfocused pauses the clock. */
export class SeenTracker {
  private active = false;
  private timers = new Map<string, Timer>();
  private shown = new Set<string>();
  constructor(private markSeen: (id: string) => void) {}

  expand(id: string, members?: string[]): void {
    if (id.startsWith("owed:") || NOTICE_ID.test(id) || this.timers.has(id)) return; // owed items are answered and notices acknowledged, never seen
    const t: Timer = { ids: members ?? [id], remaining: SEEN_AFTER_MS, startedAt: null, handle: null };
    this.timers.set(id, t);
    if (this.active && this.shown.has(id)) this.run(id, t);
  }

  /** Whether an item is on screen; an item is NOT visible until the first observer report. */
  isVisible(id: string): boolean {
    return this.shown.has(id);
  }

  /** An item leaving the viewport pauses its clock; returning resumes it. */
  setVisible(id: string, visible: boolean): void {
    if (visible === this.shown.has(id)) return;
    if (visible) this.shown.add(id);
    else this.shown.delete(id);
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
        if (this.shown.has(id)) this.run(id, t);
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

/**
 * What "Mark N seen" marks: exactly the rows the panel shows in full on screen. A row's text is its whole content, so
 * it only has to be on screen; a routine group needs to be opened (its lines listed) and on screen. An owed ask is
 * answered, never seen, and a notice is acknowledged on purpose, so neither is ever marked here [D-12].
 */
export function markPlan(items: CatchupEntry[], expanded: Set<string>, visible: Set<string>): { ids: string[]; count: number; routineLeft: number; reason: string | null } {
  const ids: string[] = [];
  let count = 0;
  let routineLeft = 0;
  for (const e of items) {
    if (e.kind === "owed" || isNoticeItem(e)) continue;
    if (e.kind === "routine") {
      if (!expanded.has(e.item) || !visible.has(e.item)) {
        routineLeft += 1;
        continue;
      }
      ids.push(...(e.members ?? [e.item]));
      count += (e.members ?? [e.item]).length;
      continue;
    }
    if (!visible.has(e.item)) continue;
    ids.push(e.item);
    count += 1;
  }
  const reason = count > 0 ? null : `Nothing to mark: no unread row is on screen.${routineLeft > 0 ? " Open a routine group to read it, then mark it." : ""}`;
  return { ids, count, routineLeft, reason };
}

/** The ids "mark all seen" may send. */
export function snapshotIds(items: CatchupEntry[], expanded: Set<string>, visible: Set<string>): string[] {
  return markPlan(items, expanded, visible).ids;
}

/** What the panel says after a Mark N seen click: how many, and what was left. Never empty. */
export function markResultText(marked: number, routineLeft: number): string {
  const left = routineLeft > 0 ? ` ${routineLeft} routine group${routineLeft === 1 ? "" : "s"} left: open ${routineLeft === 1 ? "it" : "one"} to mark it.` : "";
  return `Marked ${marked}.${left}`;
}

/**
 * Whether an element has been shown in full. One that fits the screen must be entirely inside it. One taller than the
 * screen can never be all in view at once, so it counts once both its top edge and its bottom edge have been in view
 * since it last came on screen; a tall row that has only been brushed or half scrolled is not "seen".
 */
export function displayedTracker(): (en: IntersectionObserverEntry) => boolean {
  let sawTop = false;
  let sawBottom = false;
  const SLACK = 1; // sub-pixel rounding only
  return (en) => {
    if (!en.isIntersecting) {
      sawTop = false;
      sawBottom = false;
      return false;
    }
    const r = en.rootBounds;
    const b = en.boundingClientRect;
    const clipped = en.intersectionRect ? b.height - en.intersectionRect.height : 0; // clipped by any ancestor, not just the root
    const ir = en.intersectionRect;
    const clippedW = ir && typeof b.width === "number" && typeof ir.width === "number" ? b.width - ir.width : 0; // cut off sideways
    if (clippedW > SLACK) return false; // part of the row is cut off sideways: no edge counts, tall or not
    if (!r) return clipped <= SLACK;
    if (b.height <= r.height + SLACK) return b.top >= r.top - SLACK && b.bottom <= r.bottom + SLACK && clipped <= SLACK;
    // An edge counts only when it is inside the root AND not cut off by an ancestor (the intersection reaches it).
    if (b.top >= r.top - SLACK && (!ir || ir.top <= b.top + SLACK)) sawTop = true;
    if (b.bottom <= r.bottom + SLACK && (!ir || ir.bottom >= b.bottom - SLACK)) sawBottom = true;
    return sawTop && sawBottom;
  };
}

/** The nearest ancestor that scrolls: the row is clipped by it, so "fully displayed" is measured against it, not the window. */
export function scrollParent(el: Element): Element | null {
  if (typeof getComputedStyle !== "function") return null;
  for (let p = el.parentElement; p; p = p.parentElement) {
    const oy = getComputedStyle(p).overflowY;
    if (oy === "auto" || oy === "scroll" || oy === "overlay") return p;
  }
  return null;
}

/** What a row shows; when it changes (a poll adds members), what was read of it no longer counts. */
export function rowContentKey(e: CatchupEntry): string {
  return [e.text, (e.members ?? []).join("|"), (e.lines ?? []).join("|")].join("\u0000");
}

/** Thresholds fine enough that a change in how much of a row is shown reports. */
const THRESHOLDS = Array.from({ length: 101 }, (_, i) => i / 100);

/**
 * Report whether an element is fully displayed; returns a cleanup that disconnects the observer
 * and reports the item not visible so its clock stops. Without IntersectionObserver the
 * item is reported visible.
 */
export function observeVisibility(el: Element, item: string, report: (item: string, visible: boolean) => void): () => void {
  let o: IntersectionObserver | undefined;
  let cancelled = false; // disconnect() does not clear entries already queued
  if (typeof IntersectionObserver === "undefined") {
    report(item, true);
  } else {
    const displayed = displayedTracker();
    o = new IntersectionObserver(
      (entries) => {
        if (cancelled) return;
        for (const en of entries) report(item, displayed(en));
      },
      { threshold: THRESHOLDS, root: scrollParent(el) },
    );
    o.observe(el);
  }
  return () => {
    cancelled = true;
    o?.disconnect();
    report(item, false);
  };
}

function CatchupRow({ e, expanded, onToggle, onOverride, onMarkOne, onVisibility }: {
  e: CatchupEntry;
  expanded: boolean;
  onToggle: (item: string, members?: string[]) => void;
  onOverride: (decision: string) => void;
  onMarkOne?: ((item: string, members?: string[]) => void) | undefined;
  onVisibility?: (item: string, visible: boolean) => void;
}) {
  const ref = useRef<HTMLDivElement | null>(null);
  const contentKey = rowContentKey(e);
  useEffect(() => {
    if (!onVisibility || !ref.current) return;
    return observeVisibility(ref.current, e.item, onVisibility);
  }, [e.item, contentKey, onVisibility]);
  // A routine group can be marked only once it is open and its lines are showing; every other row shows all of itself.
  const markable = e.kind !== "owed" && !isNoticeItem(e) && (e.kind !== "routine" || expanded);
  return (
    <div ref={ref} className="rounded border border-border p-3 text-sm" data-kind={e.kind} data-expanded={expanded ? "true" : "false"}>
      <button type="button" className="min-h-11 text-left [overflow-wrap:anywhere] sm:min-h-0" onClick={() => onToggle(e.item, e.members)}>{e.text}</button>
      {expanded && e.cites && e.cites.length > 0 ? <div className="text-xs text-muted-foreground">{`cites ${e.cites.join(", ")}`}</div> : null}
      {expanded && e.lines && e.lines.length > 0 ? (
        <ul className="m-0 mt-1 list-disc space-y-0.5 pl-5 text-xs text-muted-foreground" data-routine-lines>
          {e.lines.map((l, i) => <li key={i} className="[overflow-wrap:anywhere]">{l}</li>)}
        </ul>
      ) : null}
      <div className="mt-1 flex flex-wrap items-center gap-2">
        {e.kind === "delegated" && e.decision !== undefined ? (
          <ActionButton tone="quiet" onClick={() => onOverride(e.decision!)}>Override</ActionButton>
        ) : null}
        {markable && onMarkOne ? <ActionButton tone="quiet" onClick={() => onMarkOne(e.item, e.members)} data-mark-one={e.item}>Mark seen</ActionButton> : null}
      </div>
    </div>
  );
}

export function CatchupPanel({
  items,
  expanded,
  visible,
  result,
  onToggle,
  onOverride,
  onMarkAll,
  onMarkOne,
  onVisibility,
}: {
  items: CatchupEntry[];
  expanded: Set<string>;
  /** The rows on screen right now; decides what Mark N seen will mark. */
  visible?: Set<string>;
  /** What the last click did. */
  result?: string | null;
  onToggle: (item: string, members?: string[]) => void;
  onOverride: (decision: string) => void;
  onMarkAll: () => void;
  onMarkOne?: (item: string, members?: string[]) => void;
  /** Reports each item entering or leaving the viewport. */
  onVisibility?: (item: string, visible: boolean) => void;
}) {
  const plan = markPlan(items, expanded, visible ?? new Set());
  return (
    <div className="space-y-2 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-xs font-semibold uppercase text-muted-foreground">Catch-up</h2>
        <ActionButton tone="quiet" disabled={plan.count === 0} onClick={onMarkAll} data-mark-all>{`Mark ${plan.count} seen`}</ActionButton>
      </div>
      {plan.reason ? <p className="m-0 text-xs text-muted-foreground" data-mark-reason>{plan.reason}</p> : null}
      {result ? <p className="m-0 text-xs" role="status" data-mark-result>{result}</p> : null}
      {items.length === 0 ? <p className="text-sm text-muted-foreground">You are caught up.</p> : null}
      {items.map((e) => (
        <CatchupRow key={e.item} e={e} expanded={expanded.has(e.item)} onToggle={onToggle} onOverride={onOverride} onMarkOne={onMarkOne} onVisibility={onVisibility} />
      ))}
    </div>
  );
}
