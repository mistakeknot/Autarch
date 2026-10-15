// The Learn tab (bead mk-uoiwp): the shipped deck of Markdown cards, each with "Got it" and "Later", and the switch that
// turns the tab on. Display and two settings only; nothing here rules, closes or starts anything.
import { ActionButton } from "./buttons.js";
import { Markdown } from "./markdown.js";

export type LearnCardView = { id: string; title: string; body: string; mark: "got" | "later" | null };
export type LearnApi = {
  load: () => Promise<{ enabled: boolean; cards: LearnCardView[] }>;
  setEnabled: (enabled: boolean) => Promise<void>;
  mark: (card: string, mark: "got" | "later" | "clear") => Promise<void>;
};

/** Cards still to read come first (no mark, then "Later"); "Got it" cards sink to the end, in deck order within each group. */
export function orderDeck(cards: readonly LearnCardView[]): LearnCardView[] {
  const rank = (c: LearnCardView) => (c.mark === "got" ? 2 : c.mark === "later" ? 1 : 0);
  return cards.map((c, i) => [c, i] as const).sort((a, b) => rank(a[0]) - rank(b[0]) || a[1] - b[1]).map(([c]) => c);
}

export function LearnPanel({ cards, busy = false, onMark }: { cards: readonly LearnCardView[]; busy?: boolean; onMark: (card: string, mark: "got" | "later" | "clear") => void }) {
  const left = cards.filter((c) => c.mark !== "got").length;
  return (
    <section className="flex flex-col gap-4 p-4" aria-label="Learn" data-learn-panel>
      <p className="m-0 text-sm text-muted-foreground" data-learn-count>{left === 0 ? "You have been through every card." : `${left} of ${cards.length} cards left.`}</p>
      {orderDeck(cards).map((c) => (
        <article key={c.id} className="flex flex-col gap-2 rounded-md border border-border p-3" data-learn-card={c.id} data-learn-mark={c.mark ?? "none"}>
          <h3 className="m-0 text-base font-semibold">{c.title}</h3>
          <Markdown text={c.body} />
          <div className="flex flex-wrap gap-2">
            <ActionButton tone="recommended" selected={c.mark === "got"} disabled={busy} onClick={() => onMark(c.id, c.mark === "got" ? "clear" : "got")} data-learn-got>Got it</ActionButton>
            <ActionButton selected={c.mark === "later"} disabled={busy} onClick={() => onMark(c.id, c.mark === "later" ? "clear" : "later")} data-learn-later>Later</ActionButton>
          </div>
        </article>
      ))}
    </section>
  );
}

/** The Settings switch. Off by default: the tab does not show until this is on. */
export function LearnSwitch({ enabled, busy = false, onChange }: { enabled: boolean; busy?: boolean; onChange: (enabled: boolean) => void }) {
  return (
    <section className="flex flex-col gap-1 p-4" aria-label="Learn tab" data-learn-switch>
      <label className="flex min-h-11 items-center gap-2 text-sm sm:min-h-0">
        <input type="checkbox" checked={enabled} disabled={busy} onChange={(e) => onChange(e.target.checked)} />
        Show the Learn tab
      </label>
      <p className="m-0 text-xs text-muted-foreground">A short deck that explains how Home and the estate work. Off until you turn it on.</p>
    </section>
  );
}

/**
 * Wraps a loader so only the response of the most recent call is delivered: an older read that finishes late
 * must not overwrite a newer one (the tab would come back after it was turned off).
 */
export function latestOnly<T>(load: () => Promise<T>, deliver: (v: T) => void): () => void {
  let seq = 0;
  return () => {
    const mine = ++seq;
    void load().then((v) => { if (mine === seq) deliver(v); }, () => {});
  };
}
