import Database from "better-sqlite3";
import { afterEach, expect, it, vi } from "vitest";
import { mkdtempSync, mkdirSync, existsSync, readdirSync, chmodSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createFakePluginHost, makeQueueEntry } from "@get-bb/plugin-sdk/testing";
import { wireHome } from "../server.js";
import { createStoreHandle } from "../store.js";
import { reminderActionId, pacificDate, registerReminderService } from "../followthrough-reminder.js";
import { FakeSdk } from "./wakes-helpers.js";
import { decision, pickOf } from "./helpers.js";
import type { ServeClient } from "../serve.js";
const cleanup: (() => void | Promise<void>)[] = [];
afterEach(async () => { vi.useRealTimers(); for (const fn of cleanup.splice(0).reverse()) await fn(); });
const serve = { projects: async () => [], healthy: async () => true } as unknown as ServeClient;
function fixture() {
  const dir = mkdtempSync(join(tmpdir(), "reminder-wire-fixture-")); cleanup.push(() => rmSync(dir, { recursive: true, force: true }));
  mkdirSync(join(dir, "plugins/autarch"), { recursive: true, mode: 0o755 });
  const f = createFakePluginHost({ pluginId: "autarch", dataDir: dir }); cleanup.push(() => f.harness.dispose());
  const db = new Database(":memory:"); cleanup.push(() => { if (db.open) db.close(); });
  const handle = createStoreHandle(() => db, {});
  wireHome(f.bb, handle, { serveAddr: "127.0.0.1:1", serveTokenFile: "/fixture", serveProjectDirs: [], autarchBin: "autarch" }, { serve, sdk: new FakeSdk() });
  return { ...f, db, dir, handle };
}
it("wireHome starts the consumer from the SDK testing fake's server dataDir", async () => {
  const f = fixture();
  expect(f.harness.registrations.services.map(s => s.name)).toContain("home-followthrough-reminders");
  const run = f.harness.runService("home-followthrough-reminders"); await Promise.resolve();
  expect(existsSync(join(f.dir, "plugins/autarch/followthrough/readiness.json"))).toBe(true);
  run.controller.abort(); await run.done;
  expect(readdirSync(f.dir)).toEqual(["plugins"]);
});
it("setup failure retries on a later service tick without restart", async () => {
  vi.useFakeTimers(); const f = fixture();
  mkdirSync(join(f.dir, "plugins/autarch/followthrough"), { mode: 0o755 });
  const run = f.harness.runService("home-followthrough-reminders"); await Promise.resolve();
  expect(existsSync(join(f.dir, "plugins/autarch/followthrough/readiness.json"))).toBe(false);
  chmodSync(join(f.dir, "plugins/autarch/followthrough"), 0o700);
  await vi.advanceTimersByTimeAsync(15000);
  expect(existsSync(join(f.dir, "plugins/autarch/followthrough/readiness.json"))).toBe(true);
  run.controller.abort(); await run.done;
});
it.each(["message.dispatched", "message.cancelled"] as const)("%s for a reminder creates no Home row", async event => {
  const f = fixture(); const before = f.db.prepare("SELECT total_changes() AS n").get();
  await f.harness.emitThreadEvent(event, { entry: makeQueueEntry({ id: "queue_fixture", content: [{ type: "text", mentions: [], text: `[followthrough ${"a".repeat(64)}] Advisory: fixture` }] }) });
  expect(f.db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});
it.each([undefined, "", "relative"])("unusable server data directory logs exactly once: %s", value => {
  const info = vi.fn(), service = vi.fn(); registerReminderService({ server: { experimental_dataDir: value }, log: { info }, background: { service } }, () => null);
  expect(info).toHaveBeenCalledTimes(1); expect(service).not.toHaveBeenCalled();
});
it("throwing server getter logs once and stays off", () => {
  const info = vi.fn(), service = vi.fn(); registerReminderService({ server: { get experimental_dataDir() { throw Error("fixture"); } }, log: { info }, background: { service } }, () => null);
  expect(info).toHaveBeenCalledTimes(1); expect(service).not.toHaveBeenCalled();
});

it("unpaginated successor read cannot skip a collision at a moving offset boundary", async () => {
  vi.useFakeTimers(); const f = fixture(); const store = f.handle.store();
  f.db.prepare("INSERT INTO cards(task_id,title,state) VALUES ('task_fixture','Fixture','open')").run();
  store.insertDecision(decision({ id: "decision_fixture", source: "card", task_id: "task_fixture", card_fp: "fixture", generation: 1, thread: "thr_fixture" }));
  store.recordPick(pickOf("decision_fixture", { pick_id: "pick_fixture" }), [{ id: "wake_fixture", kind: "wake", op: "wake:decision_fixture:pick_fixture", recipient: "thr_fixture" }]);
  f.db.prepare("UPDATE obligations SET state = 'done', attempt = 1 WHERE id = 'wake_fixture'").run();
  f.db.prepare("INSERT INTO attempts(obligation_id,n,state,evidence,at) VALUES ('wake_fixture',1,'delivered','event_fixture','2026-01-01T00:00:01Z')").run();
  const owner = { id: "thr_fixture", title: "Worker", projectId: "proj_fixture", createdAt: 1, archivedAt: 10, deletedAt: null };
  const rows = [owner, ...Array.from({ length: 101 }, (_, n) => ({ ...owner, id: `thr_fixture_${n}`, title: "Other", archivedAt: null })), { ...owner, id: "thr_next", createdAt: 11, archivedAt: null }, { ...owner, id: "thr_collision", createdAt: 12, archivedAt: null }];
  const list = vi.fn(async (args: { offset?: number; limit?: number }) => args.limit ? rows.slice(args.offset ?? 0, (args.offset ?? 0) + args.limit) : rows);
  f.harness.sdk.stub("threads.list", list);
  const spool = join(f.dir, "plugins/autarch/followthrough"); mkdirSync(spool, { mode: 0o700 }); mkdirSync(join(spool, "requests"), { mode: 0o700 });
  const request = { version: 1, pick_id: "pick_fixture", generation: 1, revision: "rev-1", pacific_date: pacificDate(Date.now()), expires_at: new Date(Date.now() + 3600000).toISOString(), reason: "overdue-outcome" };
  writeFileSync(join(spool, "requests", `${reminderActionId(request)}.json`), JSON.stringify(request), { mode: 0o600 });
  const run = f.harness.runService("home-followthrough-reminders"); await vi.advanceTimersByTimeAsync(0);
  expect(list).toHaveBeenCalledExactlyOnceWith({ includeHidden: false });
  run.controller.abort(); await run.done;
});
it("ordinary queue events still record their existing Home evidence", async () => {
  const f = fixture(); const before = f.db.prepare("SELECT total_changes() AS n").get() as { n: number };
  await f.harness.emitThreadEvent("message.dispatched", { entry: makeQueueEntry({ id: "queue_fixture", content: [{ type: "text", mentions: [], text: "[home-msg fixture #1] ordinary" }] }) });
  expect((f.db.prepare("SELECT total_changes() AS n").get() as { n: number }).n).toBeGreaterThan(before.n);
});
