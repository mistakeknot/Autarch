// The Home tab row: one line that scrolls sideways when the screen is narrow, with tap-height buttons on phones.
import type { Panel } from "./stack.js";

export type TabId = "queue" | "asks" | "blocks" | "catchup" | "vizier" | "map" | "settings";

export function homeTabs(classic: boolean): ReadonlyArray<readonly [TabId, Panel["kind"], string]> {
  return [
    ["queue", "decision", "Queue"],
    ...(classic ? ([["asks", "decision", "Asks"], ["blocks", "decision", "Blocking"], ["catchup", "catchup", "Catch-up"]] as const) : []),
    ["vizier", "vizier", "Vizier"],
    ["map", "map", "Map"],
    ["settings", "settings", "Settings"],
  ];
}

export function HomeTabs({ classic, waiting, onOpen, onToggleClassic, onTodos }: { classic: boolean; waiting?: number | undefined; onOpen: (p: { id: string; kind: Panel["kind"]; title: string }) => void; onToggleClassic: () => void; onTodos: () => void }) {
  return (
    <nav className="flex gap-3 overflow-x-auto whitespace-nowrap border-b border-border px-4 py-1 text-sm sm:py-2" data-home-tabs>
      {homeTabs(classic).map(([id, kind, title]) => (
        <button key={id} type="button" className="min-h-11 shrink-0 underline-offset-2 hover:underline sm:min-h-0" onClick={() => onOpen({ id, kind, title })}>
          {id === "queue" && waiting !== undefined ? `${title} (${waiting})` : title}
        </button>
      ))}
      <button type="button" className="ml-auto min-h-11 shrink-0 text-muted-foreground sm:min-h-0" aria-pressed={classic} onClick={onToggleClassic}>
        Classic tabs
      </button>
      <button type="button" className="min-h-11 shrink-0 text-muted-foreground sm:min-h-0" onClick={onTodos}>
        Todos
      </button>
    </nav>
  );
}
