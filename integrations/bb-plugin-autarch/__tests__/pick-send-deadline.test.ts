import { afterEach, expect, it, vi } from "vitest";
import { rmSync } from "node:fs";
import { join } from "node:path";
import { envs, rig } from "./card-rig.js";
import { tmpDir } from "./helpers.js";
import { FakeSdk } from "./wakes-helpers.js";
import { WakeLoop } from "../wakes.js";
import { commentBody } from "../cardwrites.js";
import { TasksClient } from "../tasks.js";
import { readPickHandlerStatus } from "../pickhandler.js";
import type { SendGuardResult } from "../pickcapture.js";

const dirs: ReturnType<typeof tmpDir>[] = [];
afterEach(() => {
  vi.useRealTimers();
  while (envs.length) rmSync(envs.pop()!.dir, { recursive: true, force: true });
  while (dirs.length) dirs.pop()!.cleanup();
});

async function prepared(beforeSend: () => Promise<SendGuardResult> = async () => ({ result: "permitted", reason: "test seam" })) {
  const target = { title: "Work" };
  const dir = tmpDir(); dirs.push(dir);
  const r = rig({ svc: {
    rulingsLedger: join(dir.dir, "rulings.jsonl"),
    pickCaptureRuntime: { mode: () => "live", beforeSend },
    readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: target.title, project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }),
  } });
  r.enableDelegation();
  const task = r.card({ blocks: "thread:thr_target", ask: { recommendation: "a", options: [
    { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: true,
      on_pick: { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } } },
    { id: "b", label: "Wait", kind: "ruling-only" },
  ] } });
  await r.poll();
  const g = r.gens(task.id)[0];
  r.db.prepare(`UPDATE pick_handler_control SET mode = 'live', epoch = epoch + 1, owner_enablement_json = '{}', reviewed_build = 'test-build', reviewed_scope_json = '{}', enabled_cells_json = '["instruction:none"]'`).run();
  expect((await r.validatedMkPick(g.id, "send-budget-pick")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")!;
  expect(relay).toBeDefined();
  return { r, g, relay, task, target };
}

it("mk-7okdy saved Ruled payload is excluded but appended owner conditions are not", async () => {
  const { r, g, task } = await prepared();
  await r.writer.drain();
  const saved = r.db.prepare("SELECT id, payload FROM card_writes WHERE decision_id = ? AND kind = 'comment'").get(g.id) as { id: string; payload: string };
  const exact = commentBody(saved.id, JSON.parse(saved.payload));
  const comment = r.fake.comments.find(c => c.taskId === task.id && c.body === exact)!;
  expect(comment).toBeDefined();
  expect((await r.svc.pickEvaluation(g.id, () => new TasksClient(r.fake)))?.evaluation.checks["no-extra-words"].result).toBe("pass");
  comment.body += "\nOnly after another review.";
  expect((await r.svc.pickEvaluation(g.id, () => new TasksClient(r.fake)))?.evaluation.checks["no-extra-words"].result).toBe("fail");
});

it.each(["owner-comment", "card-edit", "ledger-conflict"])("each send refuses fresh %s evidence after preparation", async change => {
  const { r, g, task, relay } = await prepared();
  await r.writer.drain();
  if (change === "owner-comment") r.fake.addComment(task.id, { kind: "user", threadId: null, body: "Only after review", createdAt: "2020-01-01T00:00:00Z" });
  if (change === "card-edit") r.edit(task, { title: "Changed choice" });
  if (change === "ledger-conflict") vi.spyOn(r.svc, "rulingLedgerPick").mockReturnValue(null);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toEqual([]);
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-relay")).toHaveLength(1);
});

it("normal Ruled/unlabel/done writes leave the frozen relay eligible", async () => {
  const { r, task } = await prepared();
  await r.writer.drain();
  expect(task.status).toBe("done");
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
});

it("a dispatched queue handle is not durable relay confirmation", async () => {
  const { r, relay } = await prepared();
  const sdk = new FakeSdk();
  sdk.script.push({ kind: "queue" });
  const loop = new WakeLoop(r.svc, sdk);
  await loop.drain();
  const handle = r.svc.store.attempts(relay.id)[0].handle!;
  sdk.dispatch(handle);
  sdk.caps.events = false;
  await loop.onQueueEvent({ id: handle, type: "dispatched" });
  await loop.reconcile();
  expect(r.svc.store.attempts(relay.id)[0].state).toBe("uncertain");
  sdk.caps.events = true;
  await loop.reconcile();
  expect(r.svc.store.attempts(relay.id)[0]).toMatchObject({ state: "delivered", evidence: sdk.accepted[0].id });
});

it("target title changes refuse the immutable relay", async () => {
  const { r, target } = await prepared();
  target.title = "Changed";
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toEqual([]);
});

it("confirmed relay earns one informational receipt, preserving an audited retarget on replay", async () => {
  const { r, g } = await prepared();
  const sdk = new FakeSdk();
  const loop = new WakeLoop(r.svc, sdk);
  await loop.drain();
  const receipts = () => r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt");
  expect(receipts()).toHaveLength(1);
  const receipt = receipts()[0];
  expect(receipt).toMatchObject({ recipient: "thr_a", allow_reroute: 1, allow_batch: 0, require_message_id: 0 });
  expect(receipt.payload).toContain("already relayed; do not act");
  expect(receipt.payload).toContain(sdk.accepted[0].id);
  expect(r.svc.store.retarget(receipt.id, "thr_a", "thr_successor")).toBe(true);
  r.svc.store.recordEvent("rerouted", g.id, { obligation: receipt.id, from: "thr_a", to: "thr_successor" });
  r.advance(121_000);
  await loop.reconcile();
  expect(receipts()).toHaveLength(1);
  expect(receipts()[0]).toMatchObject({ recipient: "thr_successor", payload: receipt.payload, op: receipt.op });
  await loop.drain();
  expect(receipts()[0].state).toBe("done");
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(sdk.sent.filter(s => s.threadId === "thr_successor")).toHaveLength(1);
});

it("hung SDK send releases WakeLoop at the original deadline and retains late durable evidence", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk();
  const release = sdk.block();
  r.advance(119_000);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  let settled = false;
  const loop = new WakeLoop(r.svc, sdk);
  const pending = loop.drain().then(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(0);
  expect(sdk.sent).toHaveLength(1);
  r.svc.store.insertObligations(g.id, [{ id: "ordinary-progress", kind: "wake", recipient: "thr_other", op: "ordinary-progress", payload: "Ordinary progress" }]);
  r.advance(1000);
  await vi.advanceTimersByTimeAsync(1000);
  // Assert progress before awaiting a promise that the regression leaves unresolved.
  expect(settled).toBe(true);
  await pending;
  expect(r.svc.store.attempts(relay.id)[0].state).toBe("uncertain");
  await loop.drain();
  expect(sdk.sent.some(s => s.threadId === "thr_other")).toBe(true);
  release();
  await vi.advanceTimersByTimeAsync(0);
  await loop.reconcile();
  expect(r.svc.store.attempts(relay.id)[0]).toMatchObject({ state: "delivered", evidence: expect.any(String) });
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.obligation(relay.id)?.next_try_at).toBe("9999-12-31T23:59:59.999Z");
});

it("N-status hung relay leaves ordinary Ruled writes independent and derives late success despite receipt failure", async () => {
  const { r, g, task, relay } = await prepared();
  const priorComments = r.fake.comments.filter(c => c.taskId === task.id).length;
  const reserved = r.db.prepare("SELECT status_op FROM pick_actions WHERE decision_id = ?").get(g.id);
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("pending");
  const sdk = new FakeSdk();
  const release = sdk.block();
  r.advance(119_000);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const loop = new WakeLoop(r.svc, sdk);
  let settled = false;
  const pending = loop.drain().then(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(0);
  expect(sdk.sent).toHaveLength(1);
  await r.writer.drain();
  expect(task.status).toBe("done");
  expect(r.db.prepare("SELECT kind, state FROM card_writes WHERE decision_id = ? ORDER BY kind").all(g.id)).toEqual([
    { kind: "comment", state: "done" }, { kind: "unlabel", state: "done" },
  ]);
  expect(r.fake.comments.filter(c => c.taskId === task.id)).toHaveLength(priorComments + 1);
  expect(r.fake.comments.filter(c => c.taskId === task.id && c.body.includes("Ruled:"))).toHaveLength(1);
  expect(r.fake.comments.some(c => c.body.includes("home-pick-status:"))).toBe(false);
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("delivery uncertain");
  r.advance(1000);
  await vi.advanceTimersByTimeAsync(1000);
  expect(settled).toBe(true);
  await pending;
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("delivery uncertain");
  release();
  await vi.advanceTimersByTimeAsync(0);
  await loop.reconcile();
  const receipt = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-receipt")!;
  expect(receipt).toBeDefined();
  r.db.prepare("UPDATE obligations SET state = 'undeliverable', last_error = 'receipt unavailable' WHERE id = ?").run(receipt.id);
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("already relayed");
  expect(r.svc.store.attempts(relay.id)[0].evidence).toBe(sdk.accepted[0].id);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.db.prepare("SELECT status_op FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual(reserved);
  expect(r.svc.store.obligationsFor(g.id).some(o => o.op === Object.values(reserved!)[0] || o.kind.includes("status"))).toBe(false);
});

it("N-status confirmed no-send fallback is manual and missing or malformed delivery evidence grants no authority", async () => {
  const { r, g, target, relay } = await prepared();
  target.title = "Changed";
  await new WakeLoop(r.svc, new FakeSdk()).drain();
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("manual handling required");
  r.db.prepare("UPDATE obligations SET state = 'done', attempt = 1 WHERE id = ?").run(relay.id);
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("delivery uncertain");
  expect(readPickHandlerStatus(r.svc.store, "absent")).toBeNull();
});

it.each([null, "", "sent", "queue-handle"])("N-status rejects delivered labels without durable evidence: %s", async evidence => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  r.db.prepare("UPDATE attempts SET evidence = ?, handle = 'queue-handle' WHERE obligation_id = ?").run(evidence, relay.id);
  const before = r.db.prepare("SELECT total_changes() AS n").get();
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("delivery uncertain");
  expect(r.db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it("N-status cancelled before send is manual; unavailable storage is uncertain", async () => {
  const { r, g } = await prepared();
  r.db.prepare("UPDATE pick_actions SET disposition = 'cancelled' WHERE decision_id = ?").run(g.id);
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("manual handling required");
  const read = vi.spyOn(r.svc.store, "obligationsFor").mockImplementation(() => { throw new Error("unavailable"); });
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("delivery uncertain");
  read.mockRestore();
});

it("a hung before-send hook consumes only the original remaining budget and late permission cannot send", async () => {
  let release!: (result: SendGuardResult) => void;
  const { r, relay } = await prepared(() => new Promise(resolve => { release = resolve; }));
  r.advance(119_000);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const sdk = new FakeSdk();
  let settled = false;
  const pending = new WakeLoop(r.svc, sdk).drain().then(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(0);
  r.advance(1000);
  await vi.advanceTimersByTimeAsync(1000);
  expect(settled).toBe(true);
  await pending;
  release({ result: "permitted", reason: "late" });
  await vi.advanceTimersByTimeAsync(0);
  expect(sdk.sent).toEqual([]);
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
});
