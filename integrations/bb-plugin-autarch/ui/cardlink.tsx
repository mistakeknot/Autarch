// Card-key deep links (bead mk-a51hu): the key in the path, the link to it, and the notice for a key that is not an open card.
import type { CardState } from "../cardstate.js";
import type { AsksData, OwedAsk } from "./asks.js";

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

const OPTION = /^[a-z0-9-]{1,32}(?![\s\S])/;
export type CardPreselect = { key: string; option: string };
export type CardOpenTarget = { key: string; option?: string };

/** A Cujgel return link opens the card; it never answers it. */
export function cardOptionHref(key: string, optionId: string): string {
  const canonical = cardKeyFrom(key);
  if (canonical === null || !OPTION.test(optionId)) throw new Error("Invalid card key or option id");
  return `${cardHref(canonical)}/option/${encodeURIComponent(optionId)}`;
}

/** Keep the legacy key parser unchanged. Even an invalid option opens its card. */
export function cardOpenTargetFrom(subPath: string | undefined): CardOpenTarget | null {
  const key = cardKeyFrom(subPath);
  if (key !== null) return { key };
  if (!subPath) return null;
  const match = /^([^/]+)\/option(?:\/([\s\S]*))?$/.exec(subPath.replace(/^\/+|\/+$/g, ""));
  if (!match) return null;
  const card = cardKeyFrom(match[1]);
  if (card === null) return null;
  let option = match[2] ?? "";
  try { option = decodeURIComponent(option); } catch { /* invalid ids still open their card */ }
  return { key: card, option };
}

/** Membership and exclusions are checked again against the displayed generation. */
export function cardOptionSelection(option: string | undefined, card: OwedAsk | null): { option: string | null; notice: string | null } {
  if (option === undefined) return { option: null, notice: null };
  if (!card || card.held || card.unreadable) return { option: null, notice: "No option was preselected: this card is not open for an answer." };
  const choice = OPTION.test(option) ? card.ask.options.find(o => o.id === option) : undefined;
  if (!choice) return { option: null, notice: "That option is not available. Choose an answer here." };
  if (Object.hasOwn(choice, "approval") || Object.hasOwn(choice, "on_pick")) return { option: null, notice: "That option must be chosen here; it was not preselected." };
  return { option, notice: null };
}

export function CardOptionNotice({ notice }: { notice: string | null }) {
  return notice ? <p role="status" data-card-option-notice className="m-2 rounded border border-border bg-muted p-3 text-sm sm:m-4">{notice}</p> : null;
}

/** null while the lookup runs. */
export type CardLookup = CardState | "error" | null;

/** Looks one key up through the plugin's `cardState` call (passed in as `cardState`) and hands the result to `onResult`. The returned function cancels it: a result that arrives after the cancel (a poll, or a switch to another key) is dropped. */
export function startCardLookup(
  cardState: (input: { key: string }) => Promise<unknown>,
  key: string,
  onResult: (key: string, result: Exclude<CardLookup, null>) => void,
): () => void {
  let live = true;
  void (cardState({ key }) as Promise<{ found: boolean; state?: string; title?: string }>).then(
    (r) => { if (live) onResult(key, r.found ? { found: true, state: r.state ?? "unknown", title: r.title ?? "" } : { found: false }); },
    () => { if (live) onResult(key, "error"); },
  );
  return () => { live = false; };
}

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

/**
 * The asks model for the focused view: the one card, with nothing else of Home's (runbooks, stalled rows, other
 * decisions' approvals) beside it. The recorded approvals of this card's own decision stay, so Revoke is reachable here.
 */
export function focusedAsks(asks: AsksData, card: OwedAsk): AsksData {
  const approvals = (asks.approvals ?? []).filter((a) => a.decision_id === card.id);
  return { owed: [card], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], ...(approvals.length > 0 ? { approvals } : {}), delegation: asks.delegation, machineOwners: asks.machineOwners };
}
