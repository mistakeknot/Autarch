// The summoned overlay (Task 1.12): owed decisions, one line to the vizier, a count strip. No map, no history.
// Aleph desktop opens `/plugins/autarch/home-overlay` in its always-on-top window (`overlay-settings.json`
// target `{pluginId: "autarch", panelId: OVERLAY_PANEL_ID}`); the web build shows the same panel in-page.
import { AsksPanel, type AsksData, type OnPick } from "./asks.js";
import type { CatchupEntry } from "./catchup.js";
import { VizierPanel } from "./vizier.js";

/**
 * Stable and documented (README). Aleph opens `/plugins/<pluginId>/<panelId>`, and a navPanel is routed at its
 * `path`, so the overlay's `panelId` is the path; the registration id is the same string. Matches `^[A-Za-z0-9_-]{1,64}$`.
 */
export const OVERLAY_PANEL_ID = "home-overlay";
export const OVERLAY_PATH = OVERLAY_PANEL_ID;

/** What the overlay hides from Asks: it shows owed decisions only; stalled rows are counted in the strip, not listed. */
const OWED_ONLY: Partial<AsksData> = { runbook: [], lane: [], asks: [], undeliverable: [], uncertain: [], failures: [], approvals: undefined };

export function overlayCounts(data: AsksData, catchup: CatchupEntry[]): { undeliverable: number; delegated: number } {
  // A wake is an obligation of kind "wake" (not a notice, steps-done or ruling-file); a delegated ruling carries its
  // decision (a settings change is also kind "delegated" but names none).
  return {
    undeliverable: data.undeliverable.filter((o) => o.kind === "wake").length,
    delegated: catchup.filter((c) => c.kind === "delegated" && c.decision !== undefined).length,
  };
}

/**
 * Only the decision items rendered here count as seen, and decisions are answered, never "seen", so this
 * view never calls markSeen/markAllSeen and the catch-up counts stay owed until Home's catch-up shows them.
 */
export function OverlayPanel({ data, catchup, onPick, onOpen }: { data: AsksData; catchup: CatchupEntry[]; onPick: OnPick; onOpen: (thread: string) => void }) {
  const c = overlayCounts(data, catchup);
  return (
    <div className="flex h-full min-h-0 flex-col" data-overlay-panel={OVERLAY_PANEL_ID}>
      <div className="flex gap-4 border-b border-border px-4 py-2 text-xs text-muted-foreground">
        <span>undeliverable wakes: <b data-overlay-count="undeliverable">{c.undeliverable}</b></span>
        <span>delegated rulings unseen: <b data-overlay-count="delegated">{c.delegated}</b></span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <AsksPanel data={{ ...data, ...OWED_ONLY }} onPick={onPick} onOpen={onOpen} />
      </div>
      <div className="h-48 border-t border-border">
        <VizierPanel threadId={data.delegation.settings.vizierThreadId} compact />
      </div>
    </div>
  );
}
