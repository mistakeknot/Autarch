// The phone layout of Home (bead mk-yah6): an inbox with full-screen cards, three bottom tabs (Waiting, Later,
// Since you left) and one sticky bar with the card's main action. It reuses AskCard unchanged, so every
// pick, note and safety line behaves as on the desktop; this file only arranges them and picks which
// ones are shown. UI only: the lists come from the same data the desktop queue reads.
import { useEffect, useState, type ReactNode } from "react";
import { ActionButton } from "./buttons.js";
import { glance } from "./glance.js";
import { ageText, AskCard, isDestructiveOption, type AsksData, type OnLater, type OnNote, type OnPick, type OwedAsk } from "./asks.js";

export type InboxTab = "waiting" | "later" | "since";

export interface InboxRow {
  key: string;
  /** What the chip says: Decide. */
  chip: string;
  project: string;
  age: string;
  title: string;
  /** How long a play or walk card says it takes ("About 2 min"); unset for every other row. */
  time?: string;
  ask?: OwedAsk;
}

const askRow = (o: OwedAsk, nowMs: number): InboxRow => {
  const time = glance(o)?.time ?? undefined;
  return { key: `ask:${o.id}`, chip: "Decide", project: o.project, age: ageText(o.filed_at, nowMs), title: o.subject, ...(time ? { time } : {}), ask: o };
};

/** The two lists. Waiting is the owed decisions that are neither held nor set aside; Later is the cards mk set aside. */
export function inboxRows(asks: AsksData, nowMs: number): { waiting: InboxRow[]; later: InboxRow[] } {
  const live = asks.owed.filter((o) => !o.held);
  const waiting = live.filter((o) => !o.later).map((o) => askRow(o, nowMs));
  const later = live.filter((o) => o.later).map((o) => askRow(o, nowMs));
  return { waiting, later };
}

/** True while the screen is phone-sized (below Tailwind's sm breakpoint). */
export function useIsPhone(): boolean {
  const q = "(max-width: 639px)";
  const [phone, setPhone] = useState(() => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia(q).matches);
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const m = window.matchMedia(q);
    const on = () => setPhone(m.matches);
    on();
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);
  return phone;
}

const CHIP = "rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground";

function Row({ r, onOpen }: { r: InboxRow; onOpen: () => void }) {
  const rec = r.ask?.ask.options.find((o) => o.id === r.ask?.ask.recommendation);
  return (
    <li className="m-0 list-none border-b border-border">
      <button type="button" className="flex min-h-16 w-full flex-col items-stretch gap-1 px-4 py-3 text-left" onClick={onOpen} data-inbox-row={r.key}>
        <span className="flex items-center justify-between gap-2">
          <span className="flex min-w-0 items-center gap-2">
            <span className={CHIP}>{r.chip}</span>
            {r.ask?.key ? <span className="font-mono text-xs text-muted-foreground" data-row-key>{r.ask.key}</span> : null}
          </span>
          <span className="text-xs text-muted-foreground">{r.age}</span>
        </span>
        <span className="line-clamp-2 text-sm font-medium [overflow-wrap:anywhere]">{r.title}</span>
        {r.project || r.time ? <span className="text-xs text-muted-foreground">{[r.project, r.time].filter(Boolean).join(" · ")}</span> : null}
        {rec ? <span className="line-clamp-1 text-xs text-muted-foreground" data-row-rec>{`Rec: ${rec.label}`}</span> : null}
      </button>
    </li>
  );
}

/** The sticky bar: Later on the left, the card's main action on the right. Nothing is picked without the card above showing what it does. */
function DetailBar({ r, onPick, onLater, onDone }: { r: InboxRow; onPick: OnPick; onLater: OnLater; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async (f: () => unknown) => {
    setBusy(true);
    setError(null);
    try {
      const out = (await f()) as { ok?: boolean; error?: string } | null | undefined;
      if (out && out.ok === false) setError(out.error ?? "that did not go through");
      else onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  let later: { label: string; go: () => void } | null = null;
  let main: { label: string; go: () => void } | null = null;
  if (r.ask) {
    const a = r.ask;
    const rec = a.ask.options.find((o) => o.id === a.ask.recommendation);
    if (a.task_id) later = { label: a.later ? "Move back" : "Later", go: () => void run(() => onLater(a.task_id!, !a.later)) };
    if (rec && !isDestructiveOption(rec)) main = { label: rec.label, go: () => void run(() => onPick(a.id, rec.id, a.revision)) };
  }
  return (
    <div className="sticky bottom-0 border-t border-border bg-background px-3 py-2" data-detail-bar>
      <div className="flex gap-2">
        {later ? <ActionButton tone="quiet" className="min-h-12 flex-1" disabled={busy} onClick={later.go} data-bar-later>{later.label}</ActionButton> : null}
        {main ? <ActionButton tone="recommended" className="min-h-12 flex-[2]" disabled={busy} onClick={main.go} data-bar-main>{main.label}</ActionButton> : <span className="flex-[2] self-center text-xs text-muted-foreground" data-bar-hint>Choose an option above.</span>}
      </div>
      {error !== null ? <p role="alert" className="mt-1 text-xs font-medium text-destructive" data-bar-failure>{`That did not go through: ${error}`}</p> : null}
    </div>
  );
}

export function MobileInbox({ asks, waiting, onPick, onOpen, onNote, onLater, sinceNode, sinceCount, onDesktop, onIdea, ideaNode, updateNode, filter, now = Date.now(), initial }: {
  asks: AsksData;
  waiting: number;
  onPick: OnPick;
  onOpen: (thread: string) => void;
  onNote: OnNote;
  onLater: OnLater;
  /** The Since you left tab body: notices and the catch-up list, built by the page so it keeps its mark-seen rules. */
  sinceNode: ReactNode;
  sinceCount: number;
  onDesktop: () => void;
  /** The Idea box (bead mk-2zojo): a header button and, when open, its body in place of the list. */
  onIdea?: () => void;
  ideaNode?: ReactNode;
  /** The Home update control (bead mk-nbt6c): shown under the header on the list, never on a card's detail. */
  updateNode?: ReactNode;
  /**
   * The project filter (bead mk-sufr9). `asks` arrives already filtered; this carries what the filter adds: its control (given
   * how many cards it hides on the tab shown), how many it hides per tab, and the Later count across all projects.
   */
  filter?: { node: (hidden: number) => ReactNode; hidden: { waiting: number; later: number }; laterCount: number };
  now?: number;
  /** Where to start (tests render the detail view this way). */
  initial?: { tab?: InboxTab; open?: string };
}) {
  const [tab, setTab] = useState<InboxTab>(initial?.tab ?? "waiting");
  const [open, setOpen] = useState<string | null>(initial?.open ?? null);
  const rows = inboxRows(asks, now);
  const list = tab === "later" ? rows.later : rows.waiting;
  const current = open === null ? null : [...rows.waiting, ...rows.later].find((r) => r.key === open) ?? null;
  const laterCount = filter?.laterCount ?? rows.later.length;
  const hiddenHere = filter ? (tab === "later" ? filter.hidden.later : filter.hidden.waiting) : 0;
  const title = tab === "waiting" ? `Waiting on you · ${waiting}` : tab === "later" ? `Later · ${laterCount}` : "Since you left";
  const tabs: [InboxTab, string][] = [["waiting", `Waiting ${waiting}`], ["later", `Later ${laterCount}`], ["since", sinceCount > 0 ? `Since you left ${sinceCount}` : "Since you left"]];
  // A card that vanished (picked, or moved by another surface) closes its own detail instead of showing a stale one.
  useEffect(() => { if (open !== null && current === null) setOpen(null); }, [open, current]);
  return (
    <div className="flex h-full min-h-0 flex-1 flex-col" data-mobile-inbox>
      <header className="flex items-center justify-between gap-2 border-b border-border px-4 py-2">
        {current ? (
          <ActionButton tone="quiet" className="min-h-11" onClick={() => setOpen(null)} data-inbox-back>‹ Back</ActionButton>
        ) : (
          <h1 className="m-0 text-base font-semibold" data-inbox-title>{title}</h1>
        )}
        {current ? null : (
          <span className="flex items-center gap-1">
            {onIdea ? <ActionButton tone="quiet" className="min-h-11" onClick={onIdea} aria-expanded={ideaNode ? true : false} data-inbox-idea>Idea</ActionButton> : null}
            <ActionButton tone="quiet" className="min-h-11" onClick={onDesktop} data-inbox-desktop>Full view</ActionButton>
          </span>
        )}
      </header>
      {updateNode && !current ? <div className="border-b border-border px-4 py-2" data-inbox-update>{updateNode}</div> : null}
      {filter && !current && !ideaNode && tab !== "since" ? <div className="border-b border-border py-1" data-inbox-filter>{filter.node(hiddenHere)}</div> : null}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {ideaNode && !current ? (
          ideaNode
        ) : current ? (
          <div className="p-3" data-inbox-detail={current.key}>
            {current.ask ? <AskCard ask={current.ask} onPick={onPick} onOpen={onOpen} nowMs={now} onNote={onNote} onLater={onLater} /> : null}
          </div>
        ) : tab === "since" ? (
          sinceNode
        ) : list.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground" data-inbox-empty>{hiddenHere > 0 ? "Everything here is hidden by the project filter." : tab === "later" ? "Nothing set aside." : "Nothing is waiting on you."}</p>
        ) : (
          <ul className="m-0 p-0">{list.map((r) => <Row key={r.key} r={r} onOpen={() => setOpen(r.key)} />)}</ul>
        )}
      </div>
      {current ? (
        <DetailBar r={current} onPick={onPick} onLater={onLater} onDone={() => setOpen(null)} />
      ) : (
        <nav className="flex border-t border-border bg-background" data-inbox-tabs>
          {tabs.map(([id, label]) => (
            <button key={id} type="button" aria-current={tab === id ? "page" : undefined} className={`min-h-14 flex-1 px-1 text-sm ${tab === id ? "font-semibold underline" : "text-muted-foreground"}`} onClick={() => { setTab(id); if (ideaNode) onIdea?.(); }} data-inbox-tab={id}>{label}</button>
          ))}
        </nav>
      )}
    </div>
  );
}
