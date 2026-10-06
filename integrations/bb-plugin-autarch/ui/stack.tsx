// The stack: panels open to the right, the newest at full size, older ones collapsed to spines.

export type PanelKind = "decision" | "thread" | "vizier" | "catchup" | "settings" | "map" | "runbook";
export type Panel = { id: string; kind: PanelKind; title: string; ref?: string };
export type WidthName = "quarter" | "third" | "half";
export type StackState = { panels: Panel[]; width: WidthName };
export type StackAction =
  | { type: "push"; panel: Panel }
  | { type: "close" }
  | { type: "width"; key: "[" | "]" };

const ORDER: WidthName[] = ["quarter", "third", "half"];
const CSS: Record<WidthName, string> = { quarter: "25%", third: "33.333%", half: "50%" };
export const SPINE_WIDTH = "34px";

export function stackReducer(s: StackState, a: StackAction): StackState {
  switch (a.type) {
    case "push":
      // An open panel moves to the top rather than duplicating.
      return { ...s, panels: [...s.panels.filter((p) => p.id !== a.panel.id), a.panel] };
    case "close":
      return { ...s, panels: s.panels.slice(0, -1) };
    case "width": {
      const i = ORDER.indexOf(s.width) + (a.key === "]" ? 1 : -1);
      return { ...s, width: ORDER[Math.min(ORDER.length - 1, Math.max(0, i))]! };
    }
  }
}

// The top panel takes at least its width setting and grows into the rest of the row, so one open panel never leaves
// two thirds of the screen empty.
export type Placed = { panel: Panel; collapsed: boolean; width: string; grow: boolean };

export function layoutStack(s: StackState): Placed[] {
  return s.panels.map((panel, i) => {
    const top = i === s.panels.length - 1;
    return { panel, collapsed: !top, width: top ? CSS[s.width] : SPINE_WIDTH, grow: top };
  });
}

/** Keyboard map; null means the key is not ours and must fall through. */
export type KeyIntent =
  | { type: "next" }
  | { type: "prev" }
  | { type: "open" }
  | { type: "close" }
  | { type: "lens"; lens: number }
  | { type: "width"; key: "[" | "]" };

export function keyAction(key: string, target: { editable: boolean }): KeyIntent | null {
  if (target.editable) return null;
  if (key === "j") return { type: "next" };
  if (key === "k") return { type: "prev" };
  if (key === "Enter") return { type: "open" };
  if (key === "Escape") return { type: "close" };
  if (key === "[" || key === "]") return { type: "width", key };
  if (/^[1-4]$/.test(key)) return { type: "lens", lens: Number(key) };
  return null;
}

export function StackView({ placed, render, onExpand }: { placed: Placed[]; render: (p: Panel) => React.ReactNode; onExpand: (p: Panel) => void }) {
  return (
    <div className="flex h-full min-h-0 flex-1">
      {placed.map(({ panel, collapsed, width, grow }) =>
        collapsed ? (
          <button
            key={panel.id}
            type="button"
            style={{ width, flex: "none" }}
            className="border-r border-border text-xs text-muted-foreground [writing-mode:vertical-rl]"
            onClick={() => onExpand(panel)}
            aria-label={`Open ${panel.title}`}
          >
            {panel.title}
          </button>
        ) : (
          <section key={panel.id} style={grow ? { minWidth: width, flex: "1 1 0%" } : { width, flex: "none" }} className="min-w-0 min-h-0 overflow-y-auto border-r border-border">
            {render(panel)}
          </section>
        ),
      )}
    </div>
  );
}
