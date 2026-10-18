// "Needs your eyes": items that change what Home does, put above the queue instead of in a collapsed catch-up row
// (bead mk-yjp7). Acknowledging is a deliberate click, never a dwell, because it lifts the delegation suspension.
import { ActionButton } from "./buttons.js";

export interface NoticeView {
  item: string;
  at: string;
  text: string;
}

export function NoticeBanner({ notices, suspended, onAcknowledge }: { notices: NoticeView[]; suspended: boolean; onAcknowledge: (item: string) => void }) {
  if (notices.length === 0) return null;
  return (
    <section className="rounded-lg border border-destructive p-3" role="region" aria-label="Needs your eyes" data-section="needs-eyes">
      <h2 className="m-0 text-xs font-semibold uppercase text-destructive">{`Needs your eyes (${notices.length})`}</h2>
      {suspended ? <p className="mb-0 mt-1 text-sm font-medium" data-suspended>Delegated rulings are suspended until you acknowledge this.</p> : null}
      <ul className="m-0 mt-2 list-none space-y-2 p-0">
        {notices.map((n) => (
          <li key={n.item} className="flex flex-wrap items-center gap-2 text-sm" data-notice={n.item}>
            <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{n.text}</span>
            <ActionButton tone="recommended" onClick={() => onAcknowledge(n.item)} data-acknowledge={n.item}>Acknowledge</ActionButton>
          </li>
        ))}
      </ul>
    </section>
  );
}
