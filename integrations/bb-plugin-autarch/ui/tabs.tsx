// The Home tab row: one line that scrolls sideways when the screen is narrow, with tap-height buttons on phones.
import type { ReactNode } from "react";
import type { Panel } from "./stack.js";

export type TabId = "queue" | "asks" | "blocks" | "catchup" | "ideas" | "learn" | "vizier" | "map" | "settings";

export function homeTabs(classic: boolean, learn = false): ReadonlyArray<readonly [TabId, Panel["kind"], string]> {
  return [
    ["queue", "decision", "Queue"],
    ...(classic ? ([["asks", "decision", "Asks"], ["blocks", "decision", "Blocking"], ["catchup", "catchup", "Catch-up"]] as const) : []),
    ["ideas", "ideas", "Ideas"],
    ...(learn ? ([["learn", "learn", "Learn"]] as const) : []),
    ["vizier", "vizier", "Vizier"],
    ["map", "map", "Map"],
    ["settings", "settings", "Settings"],
  ];
}

export function HomeTabs({ classic, learn = false, waiting, ideas, onOpen, onToggleClassic, onTodos, extra }: { extra?: ReactNode; classic: boolean; learn?: boolean | undefined; waiting?: number | undefined; ideas?: number | undefined; onOpen: (p: { id: string; kind: Panel["kind"]; title: string }) => void; onToggleClassic: () => void; onTodos: () => void }) {
  return (
    <nav className="flex gap-3 overflow-x-auto whitespace-nowrap border-b border-border px-4 py-1 text-sm sm:py-2" data-home-tabs>
      {homeTabs(classic, learn).map(([id, kind, title]) => (
        <button key={id} type="button" className="min-h-11 shrink-0 underline-offset-2 hover:underline sm:min-h-0" onClick={() => onOpen({ id, kind, title })}>
          {id === "queue" && waiting !== undefined ? `${title} (${waiting})` : id === "ideas" && ideas !== undefined ? `${title} (${ideas})` : title}
        </button>
      ))}
      {extra}
      <button type="button" className="ml-auto min-h-11 shrink-0 text-muted-foreground sm:min-h-0" aria-pressed={classic} onClick={onToggleClassic}>
        Classic tabs
      </button>
      <button type="button" className="min-h-11 shrink-0 text-muted-foreground sm:min-h-0" onClick={onTodos}>
        Todos
      </button>
    </nav>
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
export function QueueTabs({ tab, counts, onTab, definition = null }: { tab: QueueTab; counts: Record<QueueTab, number> | null; onTab: (t: QueueTab) => void; definition?: string | null }) {
  return (
    <div className="border-b border-border px-2 sm:px-4">
    <div className="flex flex-wrap items-center gap-x-3">
    <div role="tablist" aria-label="Queue" className="flex min-w-0 flex-1 gap-3 overflow-x-auto whitespace-nowrap" data-queue-tabs>
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
          className={`min-h-11 shrink-0 px-1 text-sm underline-offset-4 ${tab === id ? "font-semibold underline" : "text-muted-foreground hover:underline"}`}
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
