// The Home tab row: one line that scrolls sideways when the screen is narrow, with tap-height buttons on phones.
import type { ReactNode } from "react";
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

export type TabId = "queue" | "ideas" | "learn" | "archive" | "vizier" | "map" | "settings";

/** The Queue holds asks, Later, blocked cards and catch-up as its own tabs; the row has no separate Asks, Blocking or Catch-up tab. */
export function homeTabs(learn = false): ReadonlyArray<readonly [TabId, Panel["kind"], string]> {
  return [
    ["queue", "decision", "Queue"],
    ["ideas", "ideas", "Ideas"],
    ...(learn ? ([["learn", "learn", "Learn"]] as const) : []),
    ["archive", "archive", "Archive"],
    ["vizier", "vizier", "Vizier"],
    ["map", "map", "Map"],
    ["settings", "settings", "Settings"],
  ];
}

export function HomeTabs({ learn = false, waiting, ideas, onOpen, onTodos, extra }: { extra?: ReactNode; learn?: boolean | undefined; waiting?: number | undefined; ideas?: number | undefined; onOpen: (p: { id: string; kind: Panel["kind"]; title: string }) => void; onTodos: () => void }) {
  return (
    <>
    <nav className="flex gap-3 overflow-x-auto whitespace-nowrap border-b border-border px-4 py-1 text-sm sm:py-2" data-home-tabs>
      {homeTabs(learn).map(([id, kind, title]) => (
        <button key={id} type="button" className="min-h-11 shrink-0 underline-offset-2 hover:underline sm:min-h-0" onClick={() => onOpen({ id, kind, title })}>
          {id === "queue" && waiting !== undefined ? `${title} (${waiting})` : id === "ideas" && ideas !== undefined ? `${title} (${ideas})` : title}
        </button>
      ))}
      <button type="button" className="ml-auto min-h-11 shrink-0 text-muted-foreground sm:min-h-0" onClick={onTodos}>
        Todos
      </button>
    </nav>
    {extra ? <div className="border-b border-border px-4 py-1 sm:py-2" data-home-extra>{extra}</div> : null}
    </>
  );
}

export type QueueTab = "decide" | "later" | "blocks" | "catchup";

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
export function QueueTabs({ tab, counts, onTab, definition = null }: { tab: QueueTab; counts: Record<QueueTab, number> | null; onTab: (t: QueueTab) => void; definition?: string | null }) {
  return (
    <div className="border-b border-border px-2 sm:px-4">
    <div className="flex flex-wrap items-center gap-x-3">
    <div role="tablist" aria-label="Queue" className="flex min-w-0 flex-1 flex-wrap gap-x-3 sm:flex-nowrap sm:overflow-x-auto" data-queue-tabs>
      {QUEUE_TABS.map(([id, label]) => (
        <button
          key={id}
          type="button"
          role="tab"
          id={`queue-tab-${id}`}
          aria-selected={tab === id}
          aria-controls="queue-tabpanel"
          tabIndex={tab === id ? 0 : -1}
          data-queue-tab={id}
          className={`min-h-11 shrink-0 whitespace-nowrap px-1 text-sm underline-offset-4 ${tab === id ? "font-semibold underline" : "text-muted-foreground hover:underline"}`}
          onClick={() => onTab(id)}
          onKeyDown={(e) => {
            const to = nextQueueTab(tab, e.key);
            if (to === null) return;
            e.preventDefault();
            onTab(to);
            document.getElementById(`queue-tab-${to}`)?.focus();
          }}
        >
          {counts ? `${label} (${counts[id]})` : label}
        </button>
      ))}
    </div>
    {definition ? (
      <details className="shrink-0 text-xs text-muted-foreground open:basis-full" data-waiting-definition>
        <summary className="flex min-h-11 cursor-pointer items-center">What this counts</summary>
        <p className="m-0 mb-2 [overflow-wrap:anywhere]">{definition}</p>
      </details>
    ) : null}
    </div>
    </div>
  );
}
