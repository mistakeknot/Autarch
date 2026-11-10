// Which card to show when the one on screen leaves a list (answered, set aside, moved by another surface).
// The card that was directly below it, else the one above it, else none. The position comes from the list as it
// was before the card left, so a card that arrived or went meanwhile does not change where mk lands.

/** `before` is the list while the card was still in it, `after` is the list now. Returns an id from `after`, or null. */
export function neighborAfterRemoval(before: readonly string[], removed: string, after: readonly string[]): string | null {
  const at = before.indexOf(removed);
  if (at < 0) return null;
  const still = new Set(after);
  for (let i = at + 1; i < before.length; i++) if (still.has(before[i]!)) return before[i]!;
  for (let i = at - 1; i >= 0; i--) if (still.has(before[i]!)) return before[i]!;
  return null;
}
