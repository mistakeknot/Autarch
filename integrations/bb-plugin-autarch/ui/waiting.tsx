// The rail badge: the count of cards waiting on mk, "!" when serve is not ready or a machine blocker has no owner (bead mk-yjp7). The Queue tabs carry the numbers.
/** The sidebar badge: the count of decision cards waiting on mk, "!" when serve is not ready or a machine blocker has no owner, nothing at zero. */
export function badgeCount(w: { count: number }, o: { blocked: boolean; unowned: boolean }): string | null {
  if (o.blocked || o.unowned) return "!";
  return w.count > 0 ? String(w.count) : null;
}

/** What the host draws on the rail icon (`experimental_useBadge`): the same number as `badgeCount`, "!" as a warning, null at zero. */
export type RailBadge = { count: number; label: string } | { text: string; tone: "warning"; label: string };
export function railBadge(w: { count: number }, o: { blocked: boolean; unowned: boolean }): RailBadge | null {
  const n = badgeCount(w, o);
  if (n === null) return null;
  if (n === "!") return { text: "!", tone: "warning", label: "Citadel needs attention" };
  return { count: w.count, label: `${w.count} waiting on you` };
}

/** The number behind the badge: the Needs you now count (`waiting.decide`), the same number the tab shows, so held, Later and blocked-on-others cards never raise it. Before `waiting` loads, the live owed asks that are not held or in Later. */
export function railCount(decide: number | undefined, owed: { held?: unknown; later?: unknown }[] | undefined): number {
  return decide ?? owed?.filter((o) => !o.held && !o.later).length ?? 0;
}
