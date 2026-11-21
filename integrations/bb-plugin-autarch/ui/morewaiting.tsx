// The phone's "More waiting" row (bead mk-j5ipa.12): the lists the desktop shows besides the cards to decide on, shown read-only
// on the phone under one row that is closed until tapped. It reads the data the phone already has (the same `AsksData` the
// desktop panel draws from) and the same item rendering, so no server route is added. Nothing here acts: no Dismiss, no Revoke, and a file path stays plain text (no read or open button).
import { ApprovalsList, buildAsksView, RefText, type ApprovalRecord, type AsksData, type ViewItem } from "./asks.js";

export type MoreKey = "stalled" | "steps" | "waiting" | "approvals";
export type MoreGroup = { key: MoreKey; title: string; count: number; items: ViewItem[]; approvals?: ApprovalRecord[] };

/**
 * The four lists, in the order mk chose: Stalled, Steps for you (the desktop's Runbook), Waiting on others (the desktop's
 * Waiting), Recorded approvals. A list with nothing in it is left out; `total` is the sum of what is shown. The decisions owed
 * are not here: they are the Waiting and Later lists above. None of these items carries a project, so the project filter
 * never hides one (cards with no project are never hidden), and the count is across all projects as the other counts are.
 */
export function moreWaiting(d: AsksData): { total: number; groups: MoreGroup[] } {
  // A list the data lacks counts as empty: the phone must not fail to draw because one list is missing.
  const sections = buildAsksView({ ...d, owed: [], runbook: d.runbook ?? [], lane: d.lane ?? [], asks: d.asks ?? [], undeliverable: d.undeliverable ?? [], failures: d.failures ?? [], uncertain: d.uncertain ?? [], notDelivered: d.notDelivered ?? [] });
  const items = (key: "stalled" | "runbook" | "waiting"): ViewItem[] => sections.find((s) => s.key === key)?.items ?? [];
  // The Stalled list is put together by buildAsksView, so the desktop's "Not delivered yet: <subject>" items (read-only) come through here too.
  const stalled = items("stalled");
  const approvals = d.approvals ?? [];
  const all: MoreGroup[] = [
    { key: "stalled", title: "Stalled", count: stalled.length, items: stalled },
    { key: "steps", title: "Steps for you", count: items("runbook").length, items: items("runbook") },
    { key: "waiting", title: "Waiting on others", count: items("waiting").length, items: items("waiting") },
    { key: "approvals", title: "Recorded approvals", count: approvals.length, items: [], approvals },
  ];
  const groups = all.filter((g) => g.count > 0);
  return { total: groups.reduce((n, g) => n + g.count, 0), groups };
}

/** The row: closed by default with the total; opened, each non-empty group with its count and items. Read-only. */
export function MoreWaitingRow({ groups, total, open, onToggle }: { groups: MoreGroup[]; total: number; open: boolean; onToggle: () => void }) {
  return (
    <section className="border-b border-border" data-inbox-more>
      <button type="button" className="flex min-h-11 w-full items-center justify-between gap-2 px-4 py-2 text-left text-sm text-muted-foreground" aria-expanded={open} aria-controls="inbox-more-list" onClick={onToggle} data-inbox-more-toggle>
        <span>{`More waiting · ${total}`}</span>
        <span aria-hidden="true">{open ? "Hide" : "Show"}</span>
      </button>
      {open ? (
        <div id="inbox-more-list" className="space-y-4 border-t border-border px-4 py-3" data-inbox-more-list>
          {groups.map((g) => (
            <section key={g.key} data-more-group={g.key}>
              <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`${g.title} · ${g.count}`}</h2>
              {g.approvals ? (
                <ApprovalsList approvals={g.approvals} bare />
              ) : (
                <ul className="m-0 list-none space-y-2 p-0">
                  {g.items.map((i) => (
                    <li key={i.id} className="rounded border border-border p-3 text-sm [overflow-wrap:anywhere]" data-more-item={i.id}>
                      <div><RefText text={i.title} docLinks={false} /></div>
                      {i.detail ? <div className="text-xs text-muted-foreground"><RefText text={i.detail} docLinks={false} /></div> : null}
                    </li>
                  ))}
                </ul>
              )}
            </section>
          ))}
        </div>
      ) : null}
    </section>
  );
}
