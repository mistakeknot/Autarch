import Database from "better-sqlite3";
import { afterEach, expect, it, vi } from "vitest";
import { Store } from "../store.js";
import { RequestReminders, type ReminderRequestSource, type RequestReminderSources } from "../request-reminders.js";
import type { WakeSdk } from "../wakes.js";
import { tmpDir } from "./helpers.js";
import { join } from "node:path";

const minute = 60_000;
const original = 100_000;
const acceptedAt = 200_000;
const owner = "thr_owner";
const successor = "thr_successor";
const coordinator = "thr_coordinator";
const vizier = "thr_vizier";
const project = "proj_project";

const connections: Database.Database[] = [];
afterEach(() => {
  for (const db of connections.splice(0)) if (db.open) db.close();
});

function fixture(initial = owner, file = ":memory:") {
  let db = new Database(file);
  connections.push(db);
  let store = new Store(db);
  const source: ReminderRequestSource = {
    id: "pick-one", ref: "wake:d1:pick-one", since: original, recipient: initial, project,
    decision: "d1", task: "task1",
  };
  const second = { ...source, id: "pick-two", ref: "wake:d2:pick-two", decision: "d2" };
  const routes = {
    [owner]: { project, role: "owner" as const, next: coordinator },
    [successor]: { project, role: "owner" as const, next: coordinator },
    [coordinator]: { project, role: "coordinator" as const, next: vizier },
  };
  const sources = vi.fn(() => [source, second]);
  const deliver = vi.fn(async (thread: string, ref: string, since: number) =>
    thread === initial && ref.startsWith("wake:") && since === original ? { id: ref, at: acceptedAt } : null);
  const send = vi.fn<WakeSdk["send"]>(async () => ({ delivery: "sent" }));
  const removeQueued = vi.fn(async () => {});
  const reply = vi.fn(async (task: string, comment: string, actor: string) => task === source.task && comment === "comment-one" && actor === initial);
  const route = vi.fn<RequestReminderSources["route"]>(async (tid, _project, explicitOwner) => {
    const found = routes[tid as keyof typeof routes];
    return found?.role === "coordinator" && explicitOwner ? { ...found, role: "owner" } : found ?? null;
  });
  const queuedList = vi.fn<WakeSdk["queuedList"]>(async () => []);
  let now = acceptedAt;
  let enabled = true;
  const create = () => new RequestReminders(store, {
    now: () => now, sources, eligible: () => true, deliver,
    route, send, removeQueued, queuedList, reply, enabled: () => enabled,
  });
  return { create, source, second, sources, routes, deliver, send, reply, removeQueued, route, queuedList,
    store: () => store,
    time(value: number) { now = value; },
    observationOnly() { enabled = false; },
    restart() {
      db.close();
      db = new Database(file);
      connections.push(db);
      store = new Store(db);
    },
    close() { if (db.open) db.close(); },
  };
}

it.each([
  [owner, 9, 27, coordinator],
  [coordinator, 14, 42, vizier],
] as const)("times reminders and one escalation from delivery for %s", async (target, reminder, escalation, next) => {
  const f = fixture(target);
  f.sources.mockReturnValue([f.source]);
  const runner = f.create();
  f.time(acceptedAt + reminder * minute - 1);
  await runner.tick();
  expect(f.send).not.toHaveBeenCalled();
  f.time(acceptedAt + reminder * minute);
  await runner.tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  expect(f.send.mock.calls[0]).toEqual([expect.objectContaining({ threadId: target })]);
  f.time(acceptedAt + escalation * minute - 1);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  f.time(acceptedAt + escalation * minute);
  await f.create().tick();
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(2);
  expect(f.send.mock.calls[1]).toEqual([expect.objectContaining({ threadId: next })]);
});

it("records a reply from the recipient and leaves a separate request on the same task open", async () => {
  const f = fixture();
  const runner = f.create();
  await runner.tick();
  expect(await runner.answer(f.source.id, successor, "comment-one")).toBe(false);
  expect(await runner.answer(f.source.id, owner, "comment-one")).toBe(true);
  expect(await runner.answer(f.second.id, owner, "comment-one")).toBe(false);
  f.time(acceptedAt + 9 * minute);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  expect(f.send.mock.calls[0]).toEqual([expect.objectContaining({ input: expect.stringContaining(f.second.id) })]);
});

it("collects delivery and replies while sends are disabled", async () => {
  const f = fixture();
  f.observationOnly();
  f.time(acceptedAt + 60 * minute);
  const runner = f.create();
  await runner.tick();
  expect(await runner.answer(f.source.id, owner, "comment-one")).toBe(true);
  expect(f.send).not.toHaveBeenCalled();
});

it("holds a transfer until the actual successor accepts the new delivery", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  const runner = f.create();
  await runner.tick();
  expect(await runner.transfer(f.source.id, successor, vizier)).toBe(false);
  expect(await runner.transfer(f.source.id, owner, successor)).toBe(true);
  expect(f.send).toHaveBeenCalledTimes(1);
  f.time(acceptedAt + 40 * minute);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  const handoffAt = acceptedAt + 40 * minute;
  f.deliver.mockImplementation(async (thread, ref) => thread === successor && ref.startsWith("reminder-transfer:")
    ? { id: "transfer-request", at: handoffAt } : null);
  f.time(handoffAt + 9 * minute - 1);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  f.time(handoffAt + 9 * minute);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(2);
  expect(f.send.mock.calls[1]).toEqual([expect.objectContaining({ threadId: successor })]);
});

it("never replays a reminder whose send result is unknown", async () => {
  const storage = tmpDir();
  const f = fixture(owner, join(storage.dir, "reminders.db"));
  try {
    f.sources.mockReturnValue([f.source]);
    f.send.mockRejectedValueOnce(new Error("lost response"));
    f.time(acceptedAt + 9 * minute);
    await f.create().tick();
    f.restart();
    await f.create().tick();
    expect(f.send).toHaveBeenCalledTimes(1);
  } finally { f.close(); storage.cleanup(); }
});

it("cancels a queued reminder if the answer arrives before the send response", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  let finishSend!: (result: Awaited<ReturnType<WakeSdk["send"]>>) => void;
  let startSend!: () => void;
  const enqueued = new Promise<Awaited<ReturnType<WakeSdk["send"]>>>(resolve => { finishSend = resolve; });
  const started = new Promise<void>(resolve => { startSend = resolve; });
  f.send.mockImplementationOnce(async () => { startSend(); return enqueued; });
  f.time(acceptedAt + 9 * minute);
  const running = f.create().tick();
  await started;
  expect(await f.create().answer(f.source.id, owner, "comment-one")).toBe(true);
  finishSend({ delivery: "queued", queuedMessageId: "queue-one" });
  await running;
  expect(f.removeQueued).toHaveBeenCalledWith(owner, "queue-one");
  f.time(acceptedAt + 27 * minute);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
});

it("uses coordinator timing after an explicitly assigned owner transfers", async () => {
  const f = fixture();
  f.sources.mockReturnValue([{ ...f.source, explicitOwner: true }]);
  const runner = f.create();
  await runner.tick();
  expect(await runner.transfer(f.source.id, owner, coordinator)).toBe(true);
  const acceptedHandoff = acceptedAt + minute;
  f.deliver.mockResolvedValue({ id: "request-2", at: acceptedHandoff });
  f.time(acceptedHandoff + 9 * minute);
  await runner.tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  f.time(acceptedHandoff + 14 * minute);
  await runner.tick();
  expect(f.send).toHaveBeenCalledTimes(2);
  f.time(acceptedHandoff + 27 * minute);
  await runner.tick();
  expect(f.send).toHaveBeenCalledTimes(2);
  f.time(acceptedHandoff + 42 * minute);
  await runner.tick();
  expect(f.send.mock.calls.at(-1)?.[0].threadId).toBe(vizier);
});

it("isolates malformed records while still visiting unrelated requests", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  f.store().setSetting("request-reminders:v1:request:000-invalid", "{broken");
  f.store().setSetting("request-reminders:v1:request:001-invalid", JSON.stringify({ sends: ["invalid"] }));
  f.time(acceptedAt + 9 * minute);
  await expect(f.create().tick()).resolves.toBe(false);
  expect(f.send).toHaveBeenCalledTimes(1);
});

it("accepts an opaque Tasks comment ID with punctuation without corrupting storage", async () => {
  const f = fixture();
  f.reply.mockResolvedValue(true);
  const runner = f.create();
  await runner.tick();
  await expect(runner.answer(f.source.id, owner, "comment.1")).resolves.toBe(true);
  f.time(acceptedAt + 9 * minute);
  await expect(runner.tick()).resolves.toBe(true);
  expect(f.send).toHaveBeenCalledTimes(1);
  expect(f.send.mock.calls[0]?.[0].input).toContain(f.second.id);
});

it("stops polling after claiming the final step, across service restarts", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  f.time(acceptedAt + 27 * minute);
  await f.create().tick();
  expect(f.send).toHaveBeenCalledTimes(1);
  f.route.mockClear();
  await f.create().tick();
  expect(f.route).not.toHaveBeenCalled();
});

it("cancels a uniquely marked queue entry after a lost send response", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  f.send.mockRejectedValueOnce(new Error("lost response"));
  f.time(acceptedAt + 9 * minute);
  await f.create().tick();
  f.queuedList.mockResolvedValue([{ id: "queue-found", text: f.send.mock.calls[0]![0].input }]);
  expect(await f.create().answer(f.source.id, owner, "comment-one")).toBe(true);
  expect(f.removeQueued).toHaveBeenCalledWith(owner, "queue-found");
});

it("still reminds the owner after the deadline when the next responsible agent is unavailable", async () => {
  const f = fixture();
  f.sources.mockReturnValue([f.source]);
  f.route.mockResolvedValueOnce({ project, role: "owner", next: null });
  f.time(acceptedAt + 27 * minute);
  const runner = f.create();
  await runner.tick();
  expect(f.send.mock.calls.map(([message]) => message.threadId)).toEqual([owner]);
  await runner.tick();
  expect(f.send.mock.calls.map(([message]) => message.threadId)).toEqual([owner, coordinator]);
});
