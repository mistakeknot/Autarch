/** Reminder-only host projection. Ordinary wake resolution deliberately keeps its old contract. */
export interface ReminderThreadRow {
  id: string;
  title: string | null;
  projectId: string | null;
  createdAt: number | null;
  archivedAt: number | null;
  deletedAt: number | null;
  hidden?: boolean;
}
export const reminderThreadRow = (t: ReminderThreadRow): ReminderThreadRow => ({
  id: t.id, title: t.title ?? null, projectId: t.projectId ?? null, createdAt: t.createdAt ?? null,
  archivedAt: t.archivedAt ?? null, deletedAt: t.deletedAt ?? null, hidden: t.hidden ?? false,
});
export async function resolveReminderTarget(owner: string | null, list: () => Promise<ReminderThreadRow[]>): Promise<string | null> {
  try {
    const rows = (await list()).filter(r => !r.hidden);
    if (new Set(rows.map(r => r.id)).size !== rows.length) return null;
    const live = (r: ReminderThreadRow) => r.archivedAt === null && r.deletedAt === null;
    const mine = rows.find(r => r.id === owner);
    if (!mine) return null;
    if (live(mine)) return mine.id;
    const retired = mine.archivedAt ?? mine.deletedAt;
    if (!mine.title || !mine.projectId || retired === null || !Number.isFinite(retired)) return null;
    // Even a cross-project title collision is ambiguous; never choose by list order.
    const candidates = rows.filter(r => live(r) && r.title === mine.title);
    if (candidates.length !== 1) return null;
    const candidate = candidates[0]!;
    return candidate.projectId === mine.projectId && candidate.createdAt !== null
      && Number.isFinite(candidate.createdAt) && candidate.createdAt > retired ? candidate.id : null;
  } catch { return null; }
}
