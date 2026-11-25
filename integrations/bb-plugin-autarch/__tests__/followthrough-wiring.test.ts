import Database from "better-sqlite3";
import { afterEach, expect, it, vi } from "vitest";
import { mkdtempSync, mkdirSync, existsSync, readdirSync, chmodSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createFakePluginHost, makeQueueEntry } from "@get-bb/plugin-sdk/testing";
import { wireHome } from "../server.js";
import { createStoreHandle } from "../store.js";
import { registerReminderService } from "../followthrough-reminder.js";
import { FakeSdk } from "./wakes-helpers.js";
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
  await f.harness.emitThreadEvent(event, { entry: makeQueueEntry({ id: "queue_fixture", content: [{ type: "text", text: `[followthrough ${"a".repeat(64)}] Advisory: fixture` }] }) });
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
