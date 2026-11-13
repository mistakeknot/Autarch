// One guarded read of a stored ask body (bead mk-j5ipa.14). A body that no longer parses (bad JSON, or a shape parseExistingAsk
// refuses) must cost one card its answer buttons, not blank the whole inbox: every reader goes through readAsk and
// shows the card marked "could not be read". The stored row is never changed.
import { parseExistingAsk, parseStoredCardAsk, type Ask } from "./model.js";

/** What a screen shows in place of the ask text and options of an unreadable card. */
export const UNREADABLE_ASK = { question: "", options: [] as never[] };

/** The answer to a pick or a ruling on a card whose body cannot be read: nothing is recorded, the asker files it again. */
export const UNREADABLE_REFUSAL = { ok: false, status: 409, error: "this card could not be read, so it cannot be answered; the asking agent is told to file it again" } as const;

/** The decoded ask, or null when the stored body cannot be read. */
export function readAsk(bodyJson: string, overrides: Record<string, unknown> = {}, source: unknown = "home"): Ask | null {
  try {
    const parse = source === "card" ? parseStoredCardAsk : parseExistingAsk;
    return parse({ ...JSON.parse(bodyJson), ...overrides });
  } catch {
    return null;
  }
}

/** Source must come from the persisted decision row, never the body or caller. */
export function readStoredAsk(body: string, source: "card" | "home"): Ask | null {
  return readAsk(body, {}, source);
}

/** The `ask` field of an owed row, spread into it: the decoded ask, or the placeholder plus `unreadable: true`. */
export function owedAsk(bodyJson: string, source: unknown = "home"): { ask: Ask; unreadable?: undefined } | { ask: typeof UNREADABLE_ASK; unreadable: true } {
  const ask = readAsk(bodyJson, {}, source);
  return ask ? { ask } : { ask: UNREADABLE_ASK, unreadable: true };
}
