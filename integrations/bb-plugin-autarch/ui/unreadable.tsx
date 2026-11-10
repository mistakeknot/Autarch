// A card whose stored ask could not be read (bead mk-j5ipa.14): it keeps its place, says so with its key, links to the card,
// and has nothing to answer. No motion and no modal; the link is a plain anchor at tap height.
import { cardHref } from "./cardlink.js";

/** "<KEY> could not be read", or "This ask could not be read" for an ask with no card key. */
export const unreadableLabel = (key: string | null | undefined): string => `${key || "This ask"} could not be read`;

export function UnreadableCard({ id, cardKey }: { id: string; cardKey?: string | null }) {
  return (
    <article className="min-w-0 rounded-lg border border-border bg-card p-4" data-decision={id} data-unreadable>
      <h3 className="text-sm font-medium [overflow-wrap:anywhere]" data-unreadable-title>{unreadableLabel(cardKey)}</h3>
      <p className="mt-1 text-xs text-muted-foreground">Home could not read this card, so it cannot be answered here. The asking agent was told to file it again.</p>
      {cardKey ? (
        <a className="mt-2 inline-flex min-h-11 items-center text-sm underline" href={cardHref(cardKey)} data-open-card>Open card</a>
      ) : null}
    </article>
  );
}
