import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, chmodSync, symlinkSync, rmSync, readdirSync, utimesSync, statSync, lstatSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { ReminderConsumer, reminderActionId, reminderDataDir, parseReminderRequest, type ReminderPick, type ReminderSources } from "../followthrough-reminder.js";
import { FakeSdk } from "./wakes-helpers.js";
const unlinkHook = vi.hoisted(() => ({ before: undefined as (() => void) | undefined }));
const renameHook = vi.hoisted(() => ({ before: undefined as ((from: string, to: string) => void) | undefined }));
vi.mock("node:fs", async importOriginal => {
  const fs = await importOriginal<typeof import("node:fs")>();
  return { ...fs, renameSync(from: Parameters<typeof fs.renameSync>[0], to: Parameters<typeof fs.renameSync>[1]) {
    renameHook.before?.(String(from), String(to));
    fs.renameSync(from, to);
  }, unlinkSync(path: Parameters<typeof fs.unlinkSync>[0]) {
    if (String(path).endsWith("/consumer.lock")) unlinkHook.before?.();
    fs.unlinkSync(path);
  } };
});
let dir: string, now: number, sdk: FakeSdk, pick: ReminderPick, consumer: ReminderConsumer;
const request = (date = "2026-11-25") => ({ version: 1, pick_id: "pick_fixture", generation: 1, revision: "revision_fixture", pacific_date: date, expires_at: new Date(now + 3600000).toISOString(), reason: "overdue-outcome" });
const paths = () => join(dir, "plugins/autarch/followthrough");
const rows = () => [{ id: "thr_fixture", title: "Worker", projectId: "proj_fixture", createdAt: 1, archivedAt: null, deletedAt: null }];
const sources = (): ReminderSources => ({ readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now });
const open = () => new ReminderConsumer(dir, sources());
function publish(r = request()) { const id = reminderActionId(r); writeFileSync(join(paths(), "requests", `${id}.json`), JSON.stringify(r), { mode: 0o600 }); return id; }
function result(id: string) { return JSON.parse(readFileSync(join(paths(), "receipts", `${id}.json`), "utf8")); }
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "reminder-fixture-")); now = Date.parse("2026-11-25T18:00:00Z"); sdk = new FakeSdk();
  pick = { pickId: "pick_fixture", generation: 1, revision: "revision_fixture", option: "Proceed", owner: "thr_fixture", eligible: true, protected: false, delivery: "confirmed", cancelled: false };
  mkdirSync(join(dir, "plugins/autarch"), { recursive: true, mode: 0o755 });
  consumer = open();
});
afterEach(() => { consumer.close(); rmSync(dir, { recursive: true, force: true }); });
it.each([{}, { experimental_dataDir: "" }, { get experimental_dataDir() { throw Error("missing"); } }, { experimental_dataDir: "relative" }])("missing/empty/throwing/relative data directory is report mode", host => {
  expect(reminderDataDir({ server: host })).toBeNull(); expect(sdk.sent).toEqual([]);
});
it("usable getter retains the exact host directory", () => expect(reminderDataDir({ server: { experimental_dataDir: dir } })).toBe(dir));
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
  // November 26 UTC is still November 25 in Pacific time.
  vi.spyOn(sdk, "acceptedEvents").mockResolvedValue([{ id: "event_fixture", text: input, createdAt: Date.parse("2026-11-26T07:59:59Z") }]);
  now += 86400000; const id = publish(request("2026-11-26")); await consumer.tick();
  expect(result(id)).toMatchObject({ status: "accepted", attempted: true });
  expect(send).toHaveBeenCalledTimes(2);
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
  registerReminderService({ server: Object.create(null, Object.getOwnPropertyDescriptors(host)), background: { service } }, sources);
  expect(service).not.toHaveBeenCalled(); expect(sources).not.toHaveBeenCalled(); expect(sdk.sent).toEqual([]);
});
it("eligible service starts with the host path and the fixture SDK", async () => {
  consumer.close(); let start!: (signal: AbortSignal) => Promise<void>;
  registerReminderService({ server: { experimental_dataDir: dir }, background: { service: (_: string, value: { start: typeof start }) => { start = value.start; } } }, () => ({ readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now }));
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
  await consumer.tick(); expect(send).toHaveBeenCalledTimes(1);
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
it("claim and lifetime reservation are durable before the SDK is called", async () => {
  const id = publish(); vi.spyOn(sdk, "send").mockImplementation(async () => {
    expect(result(id)).toMatchObject({ status: "claimed", attempted: true });
    const journal = readdirSync(join(paths(), "receipts")).find(f => f.startsWith("generation-"))!;
    expect(JSON.parse(readFileSync(join(paths(), "receipts", journal), "utf8")).entries).toHaveLength(1);
    throw Error("process lost response");
  });
  await consumer.tick(); expect(result(id).status).toBe("unknown");
});
it("unsafe directory change during fresh host lookup prevents dispatch", async () => {
  consumer.close(); consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => { chmodSync(paths(), 0o755); return rows(); }, sdk, now: () => now });
  publish(); await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("request swapped for a symlink during host lookup prevents dispatch", async () => {
  const id = publish(); const path = join(paths(), "requests", `${id}.json`);
  consumer.close(); let swapped = false;
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => { if (!swapped) { swapped = true; rmSync(path); symlinkSync("/fixture", path); } return rows(); }, sdk, now: () => now });
  await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it.each(["symlink", "oversize"])("unsafe receipt %s prevents dispatch", async kind => {
  const id = publish(); const path = join(paths(), "receipts", `${id}.json`);
  if (kind === "symlink") symlinkSync("/fixture", path);
  else writeFileSync(path, " ".repeat(16385), { mode: 0o600 });
  await consumer.tick(); expect(sdk.sent).toEqual([]);
});
it("cancelled wake with affirmative nondelivery can reach a proven successor after owner retirement", async () => {
  consumer.close(); pick.delivery = "none-confirmed"; pick.cancelled = true;
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => [{ ...rows()[0]!, archivedAt: 10 }, { ...rows()[0]!, id: "thr_next", createdAt: 11 }], sdk, now: () => now });
  publish({ ...request(), reason: "confirmed-nondelivery" }); await consumer.tick(); expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_next"]);
});
it("fresh readiness is signaled only for a locked safe spool", async () => {
  await consumer.tick(); expect(JSON.parse(readFileSync(join(paths(), "readiness.json"), "utf8"))).toEqual({ version: 1, uid: process.getuid!(), ready_at: new Date(now).toISOString(), expires_at: new Date(now + 45000).toISOString() });
  expect(sdk.sent).toEqual([]);
});
it("host-created 0755 ancestors remain unchanged", async () => {
  expect(consumer.active).toBe(true); publish(); await consumer.tick(); expect(sdk.sent).toHaveLength(1);
});
it("an earlier-day queued advisory prevents another reminder stacking", async () => {
  sdk.fallback = { kind: "queue" }; publish(); await consumer.tick();
  now += 86400000; publish(request("2026-11-26")); await consumer.tick(); expect(sdk.sent).toHaveLength(1);
});
it("cleanup touches only consumer temporaries and leaves producer publishes alone", () => {
  consumer.close(); const id = publish(); const suffix = "00000000-0000-4000-8000-000000000000";
  for (const folder of [paths(), join(paths(), "receipts")]) writeFileSync(join(folder, `.consumer-tmp-${suffix}`), "{}", { mode: 0o600 });
  const producer = join(paths(), "requests", `.tmp-${suffix}`); writeFileSync(producer, "partial", { mode: 0o600 });
  const namedProducer = join(paths(), "requests", `.request-tmp-${suffix}`); writeFileSync(namedProducer, "partial", { mode: 0o600 });
  consumer = open(); expect(consumer.active).toBe(true);
  for (const folder of [paths(), join(paths(), "receipts")]) expect(readdirSync(folder).filter(f => f.startsWith(".consumer-tmp-"))).toEqual([]);
  expect(readFileSync(producer, "utf8")).toBe("partial"); expect(readFileSync(namedProducer, "utf8")).toBe("partial");
  expect(readdirSync(join(paths(), "requests"))).toContain(`${id}.json`);
});
it("dead-process exclusive lock can be recovered without spawning", () => {
  consumer.close(); writeFileSync(join(paths(), "consumer.lock"), JSON.stringify({ pid: 2147483647, token: "fixture" }), { mode: 0o600 });
  consumer = open(); expect(consumer.active).toBe(true);
});

function abandonedLock(value: string, age = 0) {
  consumer.close(); const path = join(paths(), "consumer.lock");
  writeFileSync(path, value, { mode: 0o600 }); const at = new Date(now - age); utimesSync(path, at, at);
}
function takeoverDirectory(name: string, owner?: string, age = 0) {
  const path = join(paths(), name);
  mkdirSync(path, { mode: 0o700 });
  if (owner !== undefined) writeFileSync(join(path, "owner.json"), owner, { mode: 0o600 });
  const at = new Date(now - age); utimesSync(path, at, at);
}
async function runReminderOnce() {
  const abort = new AbortController(), log: string[] = [];
  let start: ((signal: AbortSignal) => Promise<void>) | undefined;
  registerReminderService({
    server: { experimental_dataDir: dir },
    log: { info(message) { log.push(message); abort.abort(); } },
    background: { service(_name, definition) { start = definition.start; } },
  }, sources);
  await start!(abort.signal);
  return log;
}
it.each(["{}", "null", "[]", '{"pid":0}'])("an old JSON lock without positive owner evidence stays refused: %s", text => {
  abandonedLock(text, 60001);
  consumer = open();
  expect(consumer.active).toBe(false);
  expect(consumer.state).toBe("lock-owner-unavailable");
  now += 86400000;
  consumer.close();
  consumer = open();
  expect(consumer.state).toBe("lock-owner-unavailable");
  expect(readFileSync(join(paths(), "consumer.lock"), "utf8")).toBe(text);
});
it.each([
  ["missing owner", undefined],
  ["invalid owner schema", '{"pid":0,"token":"fixture"}'],
  ["missing start", '{"pid":2147483647,"token":"fixture"}'],
] as const)("logs a receipt-free invalid takeover gate for %s", async (_name, owner) => {
  abandonedLock(JSON.stringify({ pid: 2147483647, token: "fixture" }));
  takeoverDirectory(".consumer-takeover", owner);
  publish();
  expect(await runReminderOnce()).toEqual(["followthrough reminders: state=takeover-gate-invalid"]);
  expect(readdirSync(join(paths(), "receipts"))).toEqual([]);
  expect(sdk.sent).toEqual([]);
  expect(readdirSync(paths())).toContain(".consumer-takeover");
});
it("logs an unsuccessful gate release and recovers that exact gate on the next run", async () => {
  abandonedLock(JSON.stringify({ pid: 2147483647, token: "fixture" }));
  publish();
  renameHook.before = (from, to) => {
    if (from.endsWith("/.consumer-takeover") && to.includes("/.consumer-takeover-")) throw Error("fixture rename failure");
  };
  try {
    expect(await runReminderOnce()).toEqual(["followthrough reminders: state=takeover-gate-release-failed"]);
    expect(readdirSync(join(paths(), "receipts"))).toEqual([]);
    expect(sdk.sent).toEqual([]);
    expect(readdirSync(paths())).toContain(".consumer-takeover");
  } finally {
    renameHook.before = undefined;
  }
  consumer = open();
  expect(consumer.active).toBe(true);
  expect(readdirSync(paths())).not.toContain(".consumer-takeover");
  await consumer.tick();
  expect(sdk.sent).toHaveLength(1);
});

const oldTakeoverAge = 86400001;
const takeoverStage = ".consumer-takeover-00000000-0000-4000-8000-000000000000";
const retiredGate = `.consumer-takeover-retired-${"a".repeat(64)}`;
it("cleans only old takeover leftovers and keeps the live gate and symlink", () => {
  consumer.close();
  const owner = JSON.stringify({ pid: 2147483647, token: "fixture", start: "fixture-start" });
  takeoverDirectory(takeoverStage, owner, oldTakeoverAge);
  takeoverDirectory(retiredGate, owner, oldTakeoverAge);
  const freshRetired = `.consumer-takeover-retired-${"b".repeat(64)}`;
  takeoverDirectory(freshRetired, owner);
  const freshGate = ".consumer-takeover";
  takeoverDirectory(freshGate, JSON.stringify({ pid: process.pid, token: "fixture", start: "fixture-live-start" }));
  const malformedName = ".consumer-takeover-00000000-0000-4000-8000-00000000000";
  takeoverDirectory(malformedName, owner, oldTakeoverAge);
  const outside = join(dir, "fixture"); mkdirSync(outside);
  const link = join(paths(), `.consumer-takeover-retired-${"c".repeat(64)}`);
  symlinkSync(outside, link);
  consumer = new ReminderConsumer(dir, { ...sources(), processIdentity: pid => pid === process.pid ? "fixture-live-start" : null });
  expect(consumer.active).toBe(true);
  const remaining = readdirSync(paths());
  expect(remaining).not.toContain(takeoverStage);
  expect(remaining).not.toContain(retiredGate);
  expect(remaining).toEqual(expect.arrayContaining([freshGate, freshRetired, malformedName]));
  expect(lstatSync(link).isSymbolicLink()).toBe(true);
  expect(readdirSync(outside)).toEqual([]);
});
it("preserves a tombstone while a live staged contender could still use it", () => {
  consumer.close();
  const owner = JSON.stringify({ pid: process.ppid, token: "fixture", start: "fixture-live-start" });
  takeoverDirectory(takeoverStage, owner, oldTakeoverAge);
  takeoverDirectory(retiredGate, owner, oldTakeoverAge);
  consumer = new ReminderConsumer(dir, { ...sources(), processIdentity: () => "fixture-live-start" });
  expect(consumer.active).toBe(true);
  expect(readdirSync(paths())).toEqual(expect.arrayContaining([takeoverStage, retiredGate]));
});
it("same-process PID with no held consumer is stale", () => {
  abandonedLock(JSON.stringify({ pid: process.pid, token: "fixture" })); consumer = open(); expect(consumer.active).toBe(true);
});
it.each(["", "{partial"])("broken old lock is recoverable: %j", text => {
  abandonedLock(text, 60001); consumer = open(); expect(consumer.active).toBe(true);
});
it("a fresh partial lock is protected during its 60-second initialization grace", () => {
  abandonedLock("{"); consumer = open(); expect(consumer.active).toBe(false);
  consumer.close(); now += 60001; consumer = open(); expect(consumer.active).toBe(true);
});
it("a reused live PID with a different boot/start identity is stale", () => {
  abandonedLock(JSON.stringify({ pid: process.ppid, token: "fixture", start: "fixture-old-start" }));
  consumer = open(); expect(consumer.active).toBe(true);
});
it("legacy PID-only lock owned by another live process expires after the grace period", () => {
  abandonedLock(JSON.stringify({ pid: process.ppid, token: "fixture" }), 60001);
  consumer = open(); expect(consumer.active).toBe(true);
});
it("takeover compares the parsed inode and cannot unlink a newer live holder", () => {
  abandonedLock(JSON.stringify({ pid: process.ppid, token: "fixture", start: "fixture-old-start" }));
  let holder: ReminderConsumer | undefined, token = "";
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now,
    processIdentity: (pid: number) => {
      if (pid === process.ppid && !holder) {
        rmSync(join(paths(), "consumer.lock")); holder = open();
        token = JSON.parse(readFileSync(join(paths(), "consumer.lock"), "utf8")).token;
      }
      return "fixture-current-start";
    },
  });
  try { expect(consumer.active).toBe(false); expect(holder?.active).toBe(true); expect(JSON.parse(readFileSync(join(paths(), "consumer.lock"), "utf8")).token).toBe(token); }
  finally { holder?.close(); }
});
it("serializes stale takeover when a rival reaches the stale lock's final unlink", () => {
  abandonedLock(JSON.stringify({ pid: 2147483647, token: "fixture" }));
  let rival: ReminderConsumer | undefined;
  unlinkHook.before = () => {
    unlinkHook.before = undefined;
    rival = open();
  };
  try {
    consumer = open();
    expect(rival).toBeDefined();
    expect([consumer, rival!].filter(c => c.active)).toHaveLength(1);
  } finally {
    unlinkHook.before = undefined;
    rival?.close();
  }
});
it("recovers a dead takeover guard and leaves a tombstone that cannot be overwritten", () => {
  abandonedLock(JSON.stringify({ pid: 2147483647, token: "fixture" }));
  const guardName = ".consumer-takeover";
  const guard = join(paths(), guardName);
  const owner = JSON.stringify({ pid: 2147483647, token: "fixture", start: "fixture-start" });
  mkdirSync(guard, { mode: 0o700 });
  writeFileSync(join(guard, "owner.json"), owner, { mode: 0o600 });
  consumer = open();
  expect(consumer.active).toBe(true);
  const guards = readdirSync(paths()).filter(name => name.startsWith(guardName));
  expect(guards).toEqual([expect.stringMatching(/^\.consumer-takeover-retired-[a-f0-9]{64}$/)]);
  expect(readFileSync(join(paths(), guards[0]!, "owner.json"), "utf8")).toBe(owner);
});
it("keeps the stale lock if the new consumer cannot read its own identity", () => {
  const stale = JSON.stringify({ pid: 2147483647, token: "fixture" });
  abandonedLock(stale);
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now, processIdentity: () => null });
  expect(consumer.state).toBe("lock-owner-unavailable");
  expect(readFileSync(join(paths(), "consumer.lock"), "utf8")).toBe(stale);
});
it.each([{}, { future: true }])("protects an old live child lock with extra metadata %j", async extra => {
  const child = spawn(process.execPath, ["-e", "process.stdin.resume()"], { stdio: ["pipe", "ignore", "ignore"] });
  try {
    await once(child, "spawn");
    // The fixture process has a single-word comm, so split from field 1 and read field 22 directly.
    const startTicks = readFileSync(`/proc/${child.pid}/stat`, "utf8").split(/\s+/)[21];
    const boot = readFileSync("/proc/sys/kernel/random/boot_id", "utf8").trim();
    expect(startTicks).toMatch(/^\d+$/);
    const lock = JSON.stringify({ pid: child.pid, token: "fixture", start: `${boot}:${startTicks}`, ...extra });
    abandonedLock(lock, 60001);
    consumer = open();
    expect(consumer.state).toBe("lock-held");
    expect(readFileSync(join(paths(), "consumer.lock"), "utf8")).toBe(lock);
  } finally {
    if (child.pid && child.exitCode === null && child.signalCode === null) {
      const exit = once(child, "exit");
      child.kill();
      await exit;
    }
  }
});
it("a late earlier-day queue acceptance uses today's slot across restart and pruning", async () => {
  sdk.fallback = { kind: "queue" }; publish(); await consumer.tick();
  now += 86400000; const id = publish(request("2026-11-26")); sdk.dispatch("q1", now);
  await consumer.tick(); expect(sdk.sent).toHaveLength(1); expect(result(id)).toMatchObject({ status: "day-used", attempted: false });
  for (const file of readdirSync(join(paths(), "receipts"))) if (/^[a-f0-9]{64}\.json$/.test(file)) rmSync(join(paths(), "receipts", file));
  consumer.close(); consumer = open(); await consumer.tick(); expect(sdk.sent).toHaveLength(1);
  now += 86400000; publish(request("2026-11-27")); await consumer.tick(); expect(sdk.sent).toHaveLength(2);
});
it("a send accepted before Pacific midnight keeps that date when its reply crosses midnight", async () => {
  now = Date.parse("2026-11-26T07:59:59Z");
  const send = vi.spyOn(sdk, "send").mockImplementationOnce(async ({ threadId, input }) => {
    sdk.accepted.push({ id: "event_fixture", threadId, text: input, createdAt: now });
    now += 2000;
    return { delivery: "sent", messageId: "fixture_message" };
  });
  const first = publish(); await consumer.tick();
  expect(result(first)).toMatchObject({ status: "accepted", resolved_date: "2026-11-25" });
  const second = publish(request("2026-11-26")); await consumer.tick();
  expect(send).toHaveBeenCalledTimes(2);
  expect(result(second).status).toBe("accepted");
});
it("deliberate reminder cancellation never becomes permission to re-nag", async () => {
  sdk.fallback = { kind: "queue" }; publish(); await consumer.tick(); sdk.cancel("q1");
  const state = vi.spyOn(sdk, "queuedState"); now += 86400000; publish(request("2026-11-26")); await consumer.tick();
  expect(sdk.sent).toHaveLength(1); expect(state).not.toHaveBeenCalled();
});
it.each([".tmp-", ".consumer-tmp-"])("wrong-mode stray temporary does not disable setup: %s", async prefix => {
  consumer.close(); const path = join(paths(), "receipts", `${prefix}00000000-0000-4000-8000-000000000000`);
  writeFileSync(path, "{}", { mode: 0o644 }); consumer = open(); expect(consumer.active).toBe(true);
  publish(); await consumer.tick(); expect(sdk.sent).toHaveLength(1); expect(statSync(path).mode & 0o777).toBe(0o644);
});
it("legacy EPERM identity lookup cannot strand an old PID-only lock", () => {
  abandonedLock(JSON.stringify({ pid: process.ppid, token: "fixture" }), 60001);
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now,
    processIdentity: pid => { if (pid === process.ppid) throw Object.assign(Error("fixture"), { code: "EPERM" }); return "fixture-current-start"; },
  });
  expect(consumer.active).toBe(true);
});
it("a matching foreign boot/start identity stays locked even after the grace period", () => {
  abandonedLock(JSON.stringify({ pid: process.ppid, token: "fixture", start: "fixture-current-start" }), 60001);
  consumer = new ReminderConsumer(dir, { readPick: () => pick, listThreads: async () => rows(), sdk, now: () => now, processIdentity: () => "fixture-current-start" });
  expect(consumer.active).toBe(false); expect(consumer.state).toBe("lock-held");
});
it("an oversized broken lock ages out without accepting or fully reading it", () => {
  abandonedLock("{".repeat(16385), 60001); consumer = open(); expect(consumer.active).toBe(true);
});
