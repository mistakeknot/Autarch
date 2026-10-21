// Card-key deep links (bead mk-a51hu): the key in the path, the link to it, and the notice for a key that is not an open card.
import type { CardState } from "../cardstate.js";

const KEY = /^[A-Z][A-Z0-9]{1,11}-\d{1,7}$/;

/** The card key in the rest of the path (`DEMO-1046`, with slashes or a lowercase prefix allowed), or null when it is not exactly one key. */
export function cardKeyFrom(subPath: string | undefined): string | null {
  if (!subPath) return null;
  let s: string;
  try { s = decodeURIComponent(subPath); } catch { return null; }
  s = s.replace(/^\/+|\/+$/g, "").toUpperCase();
  return KEY.test(s) ? s : null;
}

/** The host-relative link that opens Home on this card. */
export function cardHref(key: string): string {
  return `/plugins/autarch/home/${key}`;
}

/** null while the lookup runs. */
export type CardLookup = CardState | "error" | null;

export function CardKeyNotice({ cardKey, lookup }: { cardKey: string; lookup: CardLookup }) {
  if (lookup === null) return null;
  const text =
    lookup === "error"
      ? `${cardKey} could not be looked up.`
      : lookup.found
        ? `${cardKey}: no such open card. It is ${lookup.state}${lookup.title ? `: ${lookup.title}` : ""}.`
        : `${cardKey}: no such open card (no card with this key).`;
  return (
    <p role="status" data-card-notice={cardKey} className="m-2 rounded border border-border bg-muted p-3 text-sm [overflow-wrap:anywhere] sm:m-4">
      {text}
    </p>
  );
}
