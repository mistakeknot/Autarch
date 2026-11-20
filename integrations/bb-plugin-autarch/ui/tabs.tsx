// Home navigation wraps at narrow widths; every destination stays visible and tappable.
import { useId, useState, type ReactNode } from "react";
import * as Popover from "@radix-ui/react-popover";
import { usePortalScopeProps } from "../lib/portal-scope.js";
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

export function HomeTabs({ learn = false, onOpen, extra }: { extra?: ReactNode; learn?: boolean | undefined; waiting?: number | undefined; ideas?: number | undefined; onOpen: (p: { id: string; kind: Panel["kind"]; title: string }) => void }) {
  return (
    <>
    <nav className="flex flex-wrap gap-3 border-b border-border px-4 py-1 text-sm sm:py-2" data-home-tabs>
      {homeTabs(learn).map(([id, kind, title]) => (
        <button key={id} type="button" className="min-h-11 shrink-0 underline-offset-2 hover:underline sm:min-h-0" onClick={() => onOpen({ id, kind, title })}>
          {title}
        </button>
      ))}
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

/** Tap-accessible count help, with dismissal and focus restoration supplied by the shared popover primitive. */
function WaitingDefinition({ definition }: { definition: string }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const portalScope = usePortalScopeProps();
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label="What this counts"
          aria-expanded={open}
          aria-controls={open ? id : undefined}
          aria-describedby={open ? id : undefined}
          data-waiting-definition-toggle
          className="flex min-h-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-state-hover focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          style={{ minWidth: 44, minHeight: 44 }}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" aria-hidden="true">
            <circle cx="12" cy="12" r="9" />
            <path d="M12 11v6M12 7v1" />
          </svg>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          {...portalScope}
          id={id}
          aria-label="What this counts"
          tabIndex={-1}
          side="bottom"
          align="end"
          sideOffset={4}
          collisionPadding={8}
          className="rounded-md border border-border bg-background p-3 text-xs text-foreground shadow-md"
          style={{ width: "min(20rem, calc(100vw - 2rem))", overflowWrap: "anywhere", zIndex: 50 }}
          onOpenAutoFocus={(event) => event.preventDefault()}
          onEscapeKeyDown={(event) => event.stopPropagation()}
          data-waiting-definition
        >
          {definition}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** The Queue's own tabs: each paradigm gets a tab with its count, so no view makes mk scroll past one to reach another. */
// Below sm the tabs wrap to a second line rather than scroll, so no tab sits off-screen with no cue; a label never splits.
export function QueueTabs({ tab, counts, onTab, definition = null, filter = null }: { tab: QueueTab; counts: Record<QueueTab, number> | null; onTab: (t: QueueTab) => void; definition?: string | null; line?: string | null; filter?: ReactNode }) {
  const layout = typeof window === "undefined" ? null : new URLSearchParams(window.location.search).get("headerLayout");
  const compact = layout === "compact";
  return (
    <div className="border-b border-border px-2 sm:px-4">
    <div className="relative flex flex-wrap items-center gap-x-3 [&_[data-project-filter]]:static [&_[data-project-filter-popup]]:left-auto [&_[data-project-filter-popup]]:right-0 [&_[data-project-filter-popup]]:w-80 [&_[data-project-filter-popup]]:max-w-[calc(100vw-2rem)]">
    <div role="tablist" aria-label="Queue" className="flex min-w-0 flex-1 basis-full flex-wrap gap-x-3 sm:basis-0" data-queue-tabs>
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
          className={`min-h-11 shrink-0 whitespace-nowrap px-1 text-sm underline-offset-4 ${compact ? "rounded-full border border-border px-3 my-1" : ""} ${tab === id ? "font-semibold underline" : "text-muted-foreground hover:underline"}`}
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
    {filter}
    {definition ? <div className="flex shrink-0 items-center">
      {definition ? <WaitingDefinition definition={definition} /> : null}
    </div> : null}
    </div>
    </div>
  );
}
