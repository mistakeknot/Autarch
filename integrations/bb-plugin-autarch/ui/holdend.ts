import { holdEndPhrase, type HoldEnd } from "../holdend.js";

/** The end of a hold as text for a card: "until Nov 14", "until <event label>", or "no end set" for an old hold. */
export const holdEndText = (h: { end?: HoldEnd | null }, nowMs: number): string => holdEndPhrase(h.end, nowMs);
