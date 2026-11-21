// The id mk's own words are recorded under when the ask has a real option named "other" (bead mk-1poq7).
// It is reserved: the Home box picks the first form no real option of the ask uses, so free text can never select a real option.
export const OWN_WORDS_BASE = "mk-own-words";
const OWN_WORDS_RE = /^mk-own-words(-\d+)?$/;

/** True for an id of the reserved own-words form. */
export const isOwnWordsForm = (id: string): boolean => OWN_WORDS_RE.test(id);

/** The first reserved id that none of the ask's real options uses. */
export function freeOwnWordsId(options: readonly { id: string }[] | undefined): string {
  const used = new Set((options ?? []).map((o) => o.id));
  if (!used.has(OWN_WORDS_BASE)) return OWN_WORDS_BASE;
  for (let n = 2; ; n++) if (!used.has(`${OWN_WORDS_BASE}-${n}`)) return `${OWN_WORDS_BASE}-${n}`;
}
