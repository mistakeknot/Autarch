// The Queue's project filter, without the screen (bead mk-sufr9): which cards a ticked-out project hides, and what is kept
// on this device. The store holds the projects mk ticked OUT, so a project that shows up later is included by default.
// Cards with no project are never hidden: there is nothing to untick for them.
import type { AsksData } from "./asks.js";
import type { QueueView } from "./blocks.js";

/** Where this device keeps the choice. */
export const PROJECT_FILTER_KEY = "home.projectFilter";

/**
 * The one switch for the counts. Off: the Queue tab, the badge and the "Waiting on you" number count every project, and the
 * lists below say how many cards the filter hides. On: those numbers follow the filter. Flip it here and nothing else.
 */
export const COUNTS_FOLLOW_FILTER = false;

/** The number a tab or badge shows, given what it counts across all projects and how many of those the filter hides. */
export function shownCount(total: number, hidden: number): number {
  return COUNTS_FOLLOW_FILTER ? Math.max(0, total - hidden) : total;
}

export type ProjectChoice = { name: string; count: number };
type Carries = { project: string | null | undefined };
/** Takes a getter because reading localStorage itself throws where storage is blocked. */
export type StorageGetter = () => Pick<Storage, "getItem" | "setItem" | "removeItem"> | null | undefined;

const hides = (c: Carries, excluded: ReadonlySet<string>): boolean => !!c.project && excluded.has(c.project);

export function shownIn<T extends Carries>(cards: readonly T[], excluded: ReadonlySet<string>): T[] {
  return excluded.size === 0 ? [...cards] : cards.filter((c) => !hides(c, excluded));
}

export function hiddenIn(cards: readonly Carries[], excluded: ReadonlySet<string>): number {
  return excluded.size === 0 ? 0 : cards.filter((c) => hides(c, excluded)).length;
}

/** Every project that has a card, once, with its card count, sorted by name without regard to case. */
export function projectChoices(cards: readonly Carries[]): ProjectChoice[] {
  const counts = new Map<string, number>();
  for (const c of cards) if (c.project) counts.set(c.project, (counts.get(c.project) ?? 0) + 1);
  return [...counts].map(([name, count]) => ({ name, count })).sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }) || (a.name < b.name ? -1 : 1));
}

export function toggleProject(excluded: ReadonlySet<string>, name: string): Set<string> {
  const next = new Set(excluded);
  if (!next.delete(name)) next.add(name);
  return next;
}

export function loadExcluded(storage: StorageGetter): Set<string> {
  try {
    const raw = storage()?.getItem(PROJECT_FILTER_KEY);
    if (!raw) return new Set();
    const parsed: unknown = JSON.parse(raw);
    const list = parsed && typeof parsed === "object" ? (parsed as { excluded?: unknown }).excluded : undefined;
    return new Set(Array.isArray(list) ? list.filter((x): x is string => typeof x === "string") : []);
  } catch {
    // storage blocked, or a stored value that is not ours: start with every project included
    return new Set();
  }
}

/** An empty set removes the key, so "All projects" leaves nothing behind. */
export function saveExcluded(storage: StorageGetter, excluded: ReadonlySet<string>): void {
  try {
    const s = storage();
    if (!s) return;
    if (excluded.size === 0) s.removeItem(PROJECT_FILTER_KEY);
    else s.setItem(PROJECT_FILTER_KEY, JSON.stringify({ excluded: [...excluded].sort() }));
  } catch {
    // storage blocked or full: the choice holds for this visit only
  }
}

/** The decide and Later lists: the owed cards of an excluded project leave; every other part of the data stays. */
export function filterAsks(data: AsksData, excluded: ReadonlySet<string>): AsksData {
  return excluded.size === 0 ? data : { ...data, owed: shownIn(data.owed, excluded) };
}

/** The Blocked on others list: rows and legacy asks of an excluded project leave, and the legacy count drops by what left. */
export function filterQueue(q: QueueView, excluded: ReadonlySet<string>): QueueView {
  if (excluded.size === 0) return q;
  const owed = shownIn(q.legacy.owed, excluded);
  return { ...q, rows: shownIn(q.rows, excluded), legacy: { ...q.legacy, owed, count: q.legacy.count - (q.legacy.owed.length - owed.length) } };
}

export function hiddenText(n: number): string {
  return `${n} hidden by the project filter`;
}
