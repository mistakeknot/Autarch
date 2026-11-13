import type { MergeGroup } from "../group.js";

/** Put members at their first visible member's position. Stable sort preserves
 * incoming order for ties; absent members and filtered cards never block display. */
export function orderedGroups<T extends { ask: { group?: MergeGroup } }>(rows: readonly T[]): T[] {
 const groups = new Map<string, T[]>();
 for (const row of rows) {
  const key = row.ask.group?.key;
  if (key !== undefined) {
   const members = groups.get(key) ?? [];
   members.push(row); groups.set(key, members);
  }
 }
 const out: T[] = [];
 for (const row of rows) {
  const key = row.ask.group?.key;
  if (key === undefined) out.push(row);
  else {
   const members = groups.get(key);
   if (members) { out.push(...members.sort((a,b) => a.ask.group!.order - b.ask.group!.order)); groups.delete(key); }
  }
 }
 return out;
}
