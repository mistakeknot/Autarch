import { expect, it, vi } from "vitest";
import { resolveReminderTarget, type ReminderThreadRow } from "../followthrough-target.js";
import { resolveWakeTarget } from "../wake-target.js";
const owner: ReminderThreadRow = { id: "thr_fixture", title: "Worker", projectId: "proj_fixture", createdAt: 1, archivedAt: 10, deletedAt: null };
const next: ReminderThreadRow = { ...owner, id: "thr_next", createdAt: 11, archivedAt: null };
it.each([
  ["live owner", [{ ...owner, archivedAt: null }], "thr_fixture"],
  ["successor", [owner, next], "thr_next"],
  ["deleted owner", [{ ...owner, archivedAt: null, deletedAt: 10 }, next], "thr_next"],
  ["missing owner", [next], null],
  ["missing project", [{ ...owner, projectId: null }, next], null],
  ["missing title", [{ ...owner, title: null }, next], null],
  ["cross project", [owner, { ...next, projectId: "proj_other" }], null],
  ["old title", [owner, { ...next, createdAt: 9 }], null],
  ["equal time", [owner, { ...next, createdAt: 10 }], null],
  ["missing created time", [owner, { ...next, createdAt: null }], null],
  ["collision", [owner, next, { ...next, id: "thr_other" }], null],
  ["cross project collision", [owner, next, { ...next, id: "thr_other", projectId: "proj_other" }], null],
] as [string, ReminderThreadRow[], string | null][])("%s", async (_, rows, target) => {
  expect(await resolveReminderTarget("thr_fixture", async () => rows)).toBe(target);
});
it("lookup failure is digest-only", async () => {
  expect(await resolveReminderTarget("thr_fixture", async () => { throw Error("offline"); })).toBeNull();
});
it("ordinary wake and notice fixtures retain byte-identical vizier fallback", async () => {
  const vizier = vi.fn(async () => ({ ok: true as const, id: "thr_vizier", via: "stored" as const }));
  const fixtures = [[], [owner, next], [owner, { ...next, createdAt: 1 }], [owner, next, { ...next, id: "thr_other" }]];
  const results = [];
  for (const rows of fixtures) results.push(await resolveWakeTarget("thr_fixture", async () => rows, vizier));
  expect(JSON.stringify(results)).toBe('["thr_vizier","thr_next","thr_next","thr_vizier"]');
});
