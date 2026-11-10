// The one definition of "waiting on you" (bead mk-j5ipa.9): the open decision cards that need a pick now, which are the
// ones neither on hold nor set aside for Later. Held and Later cards are counted beside it by name and never in it; a card
// that is both is held only. Notices and updates to read are named elsewhere and are not part of it either.
// Pure and dependency-free so the server (waiting.ts) and every screen (rail badge, Queue tab, phone inbox) share it.

export interface WaitingCounts {
  /** Cards that need a pick now. */
  pick: number;
  /** Cards on hold. */
  held: number;
  /** Cards set aside for Later, not held. */
  later: number;
}

/** Count open decision cards by what they need from mk. A hold beats Later. */
export function owedCounts(owed: ReadonlyArray<{ held?: unknown; later?: unknown }>): WaitingCounts {
  const c: WaitingCounts = { pick: 0, held: 0, later: 0 };
  for (const o of owed) {
    if (o.held) c.held++;
    else if (o.later) c.later++;
    else c.pick++;
  }
  return c;
}

/** "12 on hold · 8 later": the held and Later parts by name, a part that is zero left out; empty when both are. */
export function extrasText(c: Pick<WaitingCounts, "held" | "later">): string {
  return [c.held > 0 ? `${c.held} on hold` : "", c.later > 0 ? `${c.later} later` : ""].filter(Boolean).join(" · ");
}

/** "10 · 12 on hold · 8 later": the count first, then the extras. */
export function waitingLine(c: WaitingCounts): string {
  return [String(c.pick), extrasText(c)].filter(Boolean).join(" · ");
}
