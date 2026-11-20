// One counted Queue row, with Projects and every other destination under More.
import { useEffect, useRef, type ReactNode } from "react";
import { HEADER_CONTROL, HEADER_QUIET } from "./buttons.js";
import type { Panel } from "./stack.js";

/** The removed Classic tabs switch stored its choice here; nothing reads it now, so Home clears it on load. */
export const RETIRED_CLASSIC_KEY = "home.classicTabs";

/** Takes a getter because reading localStorage itself throws where storage is blocked. */
export function forgetClassicTabs(storage: () => Pick<Storage, "removeItem"> | null | undefined): void {
  try {
    storage()?.removeItem(RETIRED_CLASSIC_KEY);
  } catch {
    // storage unavailable or blocked: nothing to clear
  }
}

export type TabId = "queue" | "ideas" | "learn" | "archive" | "vizier" | "settings";

/** The Queue holds asks, Later, blocked cards and catch-up as its own tabs; the row has no separate Asks, Blocking or Catch-up tab. */
export function homeTabs(learn = false): ReadonlyArray<readonly [TabId, Panel["kind"], string]> {
  return [
    ["queue", "decision", "Queue"],
    ["ideas", "ideas", "Ideas"],
    ...(learn ? ([["learn", "learn", "Learn"]] as const) : []),
    ["archive", "archive", "Archive"],
    ["vizier", "vizier", "Vizier"],
    ["settings", "settings", "Settings"],
  ];
}

export function HomeTabs({ queue, extra }: { queue: ReactNode; extra?: ReactNode }) {
  return <>{queue}{extra ? <div className="border-b border-border px-4 py-1 sm:py-2" data-home-extra>{extra}</div> : null}</>;
}

export function HomeOverflow({ learn = false, onOpen, definition, active, ideas }: { ideas?: number | undefined; active?: string | undefined; definition?: string | null | undefined; learn?: boolean; onOpen: (p: { id: string; kind: Panel["kind"]; title: string }) => void }) {
  const menu = useRef<HTMLDetailsElement>(null);
  const destination = homeTabs(learn).find(([id]) => id !== "queue" && id === active)?.[2];
  useEffect(() => {
    const outside = (e: PointerEvent) => {
      if (menu.current && e.target instanceof Node && !menu.current.contains(e.target)) menu.current.open = false;
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, []);
  return <details ref={menu} className="relative shrink-0 text-xs" data-home-overflow onKeyDown={e => {
    if (e.key === "Escape" && e.currentTarget.open) {
      e.preventDefault();
      e.stopPropagation();
      e.currentTarget.open = false;
      e.currentTarget.querySelector("summary")?.focus();
    }
  }}>
    <summary className={`${destination ? `${HEADER_CONTROL} px-2 bg-transparent font-semibold underline underline-offset-4 text-foreground` : HEADER_QUIET}`}>{destination ? `More · ${destination}` : "More"}</summary>
    <div className="absolute right-0 z-30 flex w-40 flex-col rounded border border-border bg-background p-1 shadow-md">
      {homeTabs(learn).filter(([id]) => id !== "queue").map(([id, kind, title]) => <button type="button" aria-current={active === id ? "page" : undefined} className="min-h-11 px-3 text-left hover:bg-muted" key={id} onClick={e => {
        e.currentTarget.closest("details")?.removeAttribute("open");
        onOpen({ id, kind, title });
      }}>{id === "ideas" && ideas !== undefined ? `Ideas (${ideas})` : title}</button>)}
      {definition ? <p className="border-t border-border p-2 text-xs text-muted-foreground">{definition}</p> : null}
    </div>
  </details>;
}

export type QueueTab = "decide" | "later" | "blocks" | "catchup";

export const PHONE_QUEUE_LABELS: Record<QueueTab, string> = { decide: "Now", later: "Later", blocks: "Blocked", catchup: "Since left" };

export const QUEUE_TABS: ReadonlyArray<readonly [QueueTab, string]> = [["decide", "Needs you now"], ["later", "Later"], ["blocks", "Blocked on others"], ["catchup", "Since you left"]];

/** Arrow keys, Home and End move between tabs and select the one they land on, as the W3C tabs pattern says. */
export function nextQueueTab(tab: QueueTab, key: string): QueueTab | null {
  const i = QUEUE_TABS.findIndex(([id]) => id === tab);
  if (key === "ArrowRight") return QUEUE_TABS[(i + 1) % QUEUE_TABS.length]![0];
  if (key === "ArrowLeft") return QUEUE_TABS[(i + QUEUE_TABS.length - 1) % QUEUE_TABS.length]![0];
  if (key === "Home") return QUEUE_TABS[0]![0];
  if (key === "End") return QUEUE_TABS[QUEUE_TABS.length - 1]![0];
  return null;
}

/** The Queue's own tabs: each paradigm gets a tab with its count, so no view makes mk scroll past one to reach another. */
// Below sm the tabs wrap to a second line rather than scroll, so no tab sits off-screen with no cue; a label never splits.
export function QueueTabs({ tab, counts, onTab, filter = null, more = null, active = true, phone = false }: { phone?: boolean; tab: QueueTab; counts: Record<QueueTab, number> | null; onTab: (t: QueueTab) => void; filter?: ReactNode; more?: ReactNode; active?: boolean }) {
  return (
    <div className="border-b border-border px-2 sm:px-4">
    <div className="relative flex flex-wrap items-center gap-x-1 [&_[data-project-filter]]:static [&_[data-project-filter-popup]]:left-auto [&_[data-project-filter-popup]]:right-0 [&_[data-project-filter-popup]]:w-80 [&_[data-project-filter-popup]]:max-w-[calc(100vw-2rem)]">
    <div role="tablist" aria-label="Queue" className="contents [&_button]:min-h-11 [&_button]:text-xs sm:[&_button]:min-h-9 sm:[&_button]:text-sm" data-queue-tabs>
      {QUEUE_TABS.map(([id, label]) => (
        <button
          key={id}
          type="button"
          role="tab"
          id={`queue-tab-${id}`}
          aria-label={counts ? `${phone ? PHONE_QUEUE_LABELS[id] : label} (${counts[id]})` : phone ? PHONE_QUEUE_LABELS[id] : label}
          aria-selected={active && tab === id}
          aria-controls={active ? "queue-tabpanel" : undefined}
          tabIndex={tab === id ? 0 : -1}
          data-queue-tab={id}
          className={`${HEADER_CONTROL} px-0.5 sm:px-2 whitespace-nowrap underline-offset-4 ${active && tab === id ? "bg-transparent font-semibold underline text-foreground" : "bg-transparent text-muted-foreground hover:bg-state-hover hover:text-foreground"}`}
          onClick={() => onTab(id)}
          onKeyDown={(e) => {
            const to = nextQueueTab(tab, e.key);
            if (to === null) return;
            e.preventDefault();
            onTab(to);
            document.getElementById(`queue-tab-${to}`)?.focus();
          }}
        >
          {counts ? `${phone ? PHONE_QUEUE_LABELS[id] : label} (${counts[id]})` : phone ? PHONE_QUEUE_LABELS[id] : label}
        </button>
      ))}
    </div>
    {filter}
    {more}
    </div>
    </div>
  );
}
