import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, chmodSync, symlinkSync, rmSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { ReminderConsumer, reminderActionId, reminderDataDir, parseReminderRequest, type ReminderPick } from "../followthrough-reminder.js";
import { FakeSdk } from "./wakes-helpers.js";
let dir: string, now: number, sdk: FakeSdk, pick: ReminderPick, consumer: ReminderConsumer;
const request = (date = "2026-11-25") => ({ version: 1, pick_id: "pick_fixture", generation: 1, revision: "revision_fixture", pacific_date: date, expires_at: new Date(now + 3600000).toISOString(), reason: "overdue-outcome" });
const paths = () => join(dir, "plugins/autarch/followthrough");
const rows = () => [{ id: "thr_fixture", title: "Worker", projectId: "proj_fixture", createdAt: 1, archivedAt: null, deletedAt: null }];
const open = () => new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now });
function publish(r = request()) { const id = reminderActionId(r); writeFileSync(join(paths(), "requests", `${id}.json`), JSON.stringify(r), { mode: 0o600 }); return id; }
function result(id: string) { return JSON.parse(readFileSync(join(paths(), "receipts", `${id}.json`), "utf8")); }
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "reminder-fixture-")); now = Date.parse("2026-11-25T18:00:00Z"); sdk = new FakeSdk();
  pick = { pickId: "pick_fixture", generation: 1, revision: "revision_fixture", option: "Proceed", owner: "thr_fixture", eligible: true, protected: false, delivery: "confirmed", cancelled: false };
  consumer = open();
});
afterEach(() => { consumer.close(); rmSync(dir, { recursive: true, force: true }); });
it.each([{}, { experimental_dataDir: "" }, { get experimental_dataDir() { throw Error("missing"); } }, { experimental_dataDir: "relative" }])("missing/empty/throwing/relative data directory is report mode", host => {
  expect(reminderDataDir(host)).toBeNull(); expect(sdk.sent).toEqual([]);
});
it("usable getter retains the exact host directory", () => expect(reminderDataDir({ experimental_dataDir: dir })).toBe(dir));
it.each([
  { target: "thr_other" }, { text: "run something" }, { version: 2 }, { generation: 0 }, { pacific_date: "2026-02-30" }, { expires_at: "bad" }, { reason: "anything" }, { pick_id: "../../escape" }, { revision: "" },
])("strict request rejects %j", patch => expect(parseReminderRequest({ ...request(), ...patch }, now)).toBeNull());
it("expiry is bounded and stale dates are rejected", () => {
  expect(parseReminderRequest({ ...request(), expires_at: new Date(now + 86400001).toISOString() }, now)).toBeNull();
  expect(parseReminderRequest(request("2026-11-24"), now)).toBeNull();
});
it("exactly one advisory send, durable receipt, no instruction text", async () => {
  const id = publish(); await consumer.tick(); await consumer.tick();
  expect(sdk.sent).toHaveLength(1);
  expect(sdk.sent[0]).toEqual({ threadId: "thr_fixture", mode: "queue-if-active", wakeCause: "retry", wakeRef: `followthrough:${id}`, input: `[followthrough ${id}] Advisory: selected option "Proceed" still needs outcome review. Read the authenticated action: bb home pick-action --pick pick_fixture` });
  expect(result(id).status).toBe("accepted");
});
it.each(["eligible", "protected", "generation", "revision", "cancelled", "delivery"])("pre-send guard %s", async field => {
  publish(); Object.assign(pick, { [field]: ({ eligible: false, protected: true, generation: 2, revision: "changed", cancelled: true, delivery: "unknown" } as Record<string, unknown>)[field] });
  await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("rechecks current state after waiting for fresh route", async () => {
  consumer.close(); consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => { pick.eligible = false; return rows(); }, sdk, now: () => now });
  publish(); await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("lost-wake requires positive nondelivery and retired owner", async () => {
  publish({ ...request(), reason: "confirmed-nondelivery" }); pick.delivery = "none-confirmed";
  await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("failed send cannot enter legacy reroute or retry that day", async () => {
  const send = vi.spyOn(sdk, "send").mockRejectedValue({ code: "ECONNREFUSED" });
  const id = publish(); await consumer.tick(); consumer.close(); consumer = open(); await consumer.tick();
  expect(send).toHaveBeenCalledTimes(1); expect(result(id).status).toBe("rejected");
});
it("crash after durable claim is never dispatched again", async () => {
  const id = publish(); vi.spyOn(sdk, "send").mockImplementation(async () => { throw Error("ambiguous crash"); });
  await consumer.tick(); const receipt = result(id); receipt.status = "claimed";
  writeFileSync(join(paths(), "receipts", `${id}.json`), JSON.stringify(receipt), { mode: 0o600 });
  consumer.close(); consumer = open(); await consumer.tick(); expect(sdk.send).toHaveBeenCalledTimes(1);
});
it("unknown acceptance and missing history never grant a later retry", async () => {
  vi.spyOn(sdk, "send").mockRejectedValue(Error("ambiguous")); publish(); await consumer.tick();
  now += 86400000; publish(request("2026-11-26")); consumer.close(); consumer = open(); await consumer.tick(); expect(sdk.send).toHaveBeenCalledTimes(1);
});
it("positive exact accepted evidence reconciles unknown before a later advisory", async () => {
  const send = vi.spyOn(sdk, "send").mockRejectedValueOnce(Error("ambiguous")); publish(); await consumer.tick();
  const input = send.mock.calls[0]![0].input;
  vi.spyOn(sdk, "acceptedEvents").mockResolvedValue([{ id: "event_fixture", text: input }]);
  now += 86400000; publish(request("2026-11-26")); await consumer.tick(); expect(send).toHaveBeenCalledTimes(2);
});
it("three lifetime attempts survive restart and action receipt pruning", async () => {
  const send = vi.spyOn(sdk, "send").mockRejectedValue({ code: "ECONNREFUSED" });
  for (let n = 0; n < 4; n++) {
    const date = `2026-11-${25 + n}`; const id = publish(request(date)); await consumer.tick();
    expect(result(id).status).toBe(n === 3 ? "capped" : "rejected");
    for (const f of readdirSync(join(paths(), "receipts"))) if (/^[a-f0-9]{64}\.json$/.test(f)) rmSync(join(paths(), "receipts", f));
    consumer.close(); consumer = open(); now += 86400000;
  }
  expect(send).toHaveBeenCalledTimes(3);
});
it("exclusive consumer lock excludes a second instance", () => { const second = open(); expect(second.active).toBe(false); second.close(); expect(consumer.active).toBe(true); });
it.each(["requests", "receipts", ""])("wrong directory mode fails closed: %s", async sub => {
  publish(); chmodSync(join(paths(), sub), 0o755); await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("wrong UID fails closed", () => {
  consumer.close(); consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now }, process.getuid!() + 1);
  expect(consumer.active).toBe(false);
});
it.each(["mode", "symlink", "oversize"])("unsafe request %s", async kind => {
  const id = publish(), path = join(paths(), "requests", `${id}.json`);
  if (kind === "mode") chmodSync(path, 0o644);
  if (kind === "symlink") { rmSync(path); symlinkSync("/fixture", path); }
  if (kind === "oversize") writeFileSync(path, " ".repeat(16385));
  await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("symlink in any ancestor disables consumer", () => {
  consumer.close(); const alias = `${dir}-alias`; symlinkSync(dir, alias);
  const other = new ReminderConsumer(alias, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now });
  expect(other.active).toBe(false); other.close(); rmSync(alias);
});
it("unsafe existing receipt prevents dispatch", async () => {
  const id = publish(); writeFileSync(join(paths(), "receipts", `${id}.json`), "{}", { mode: 0o644 }); await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("expired request yields an expired receipt", async () => {
  const id = publish({ ...request(), expires_at: new Date(now - 1).toISOString() }); await consumer.tick(); expect(result(id).status).toBe("expired"); expect(sdk.sent).toEqual([]);
});
it("no route consumes no lifetime attempt", async () => {
  consumer.close(); consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => [], sdk, now: () => now });
  const id = publish(); await consumer.tick(); expect(result(id)).toMatchObject({ status: "no-route", attempted: false });
});

import { registerReminderService } from "../followthrough-reminder.js";
it.each([{}, { experimental_dataDir: "" }, { get experimental_dataDir() { throw Error("unavailable"); } }])("start gate registers no worker for unavailable getter", host => {
  const service = vi.fn(); const sources = vi.fn();
  registerReminderService(Object.assign(Object.create(null, Object.getOwnPropertyDescriptors(host)), { background: { service } }), sources);
  expect(service).not.toHaveBeenCalled(); expect(sources).not.toHaveBeenCalled(); expect(sdk.sent).toEqual([]);
});
it("eligible service starts with the host path and the fixture SDK", async () => {
  consumer.close(); let start!: (signal: AbortSignal) => Promise<void>;
  registerReminderService({ experimental_dataDir: dir, background: { service: (_: string, value: { start: typeof start }) => { start = value.start; } } }, () => ({ readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now }));
  consumer = open(); publish(); consumer.close();
  const abort = new AbortController(); const send = vi.spyOn(sdk, "send").mockImplementation(async () => { abort.abort(); return { delivery: "sent", messageId: "event_fixture" }; });
  await start(abort.signal); expect(send).toHaveBeenCalledTimes(1);
});
it("malformed existing receipt blocks all dispatch", async () => {
  const id = publish(); writeFileSync(join(paths(), "receipts", `${id}.json`), "{}", { mode: 0o600 }); await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("positive queue evidence reconciles unknown; a near marker does not", async () => {
  const send = vi.spyOn(sdk, "send").mockRejectedValueOnce(Error("ambiguous")); publish(); await consumer.tick();
  const input = send.mock.calls[0]![0].input;
  vi.spyOn(sdk, "queuedList").mockResolvedValueOnce([{ id: "queue_fixture", text: `${input} extra` }]);
  now += 86400000; publish(request("2026-11-26")); await consumer.tick(); expect(send).toHaveBeenCalledTimes(1);
  vi.mocked(sdk.queuedList).mockResolvedValue([{ id: "queue_fixture", text: input }]);
  await consumer.tick(); expect(send).toHaveBeenCalledTimes(2);
});
it("proven lost wake reaches only the new same-project successor", async () => {
  consumer.close(); pick.delivery = "none-confirmed";
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => [{ ...rows()[0]!, archivedAt: 10 }, { ...rows()[0]!, id: "thr_next", createdAt: 11 }], sdk, now: () => now });
  publish({ ...request(), reason: "confirmed-nondelivery" }); await consumer.tick(); expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_next"]);
});
it("consumer source exposes no HTTP, CLI, RPC write or credential", () => {
  const source = readFileSync(new URL("../followthrough-reminder.ts", import.meta.url), "utf8");
  expect(source).not.toMatch(/fetch\(|\.post\(|\.rpc\(|tokenReader|process\.env|rerouteStranded|resolveWakeTarget|addObligation/);
});
