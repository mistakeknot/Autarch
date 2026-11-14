import { afterEach, expect, it, vi } from "vitest";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { envs } from "./card-rig.js";
import { recoveryRig } from "./pickhandler-stage4-rig.js";
import { FakeSdk, econnrefused } from "./wakes-helpers.js";
import { WakeLoop } from "../wakes.js";
import { readPickHandlerStatus } from "../pickhandler.js";
import { claimPickRelay, reconcilePickReceipts } from "../pickcapture.js";
import Database from "better-sqlite3";
import { loadV6 } from "./v6build.js";
import { appendRuling } from "../ledger.js";

const temps: { cleanup(): void }[] = [];
afterEach(() => {
  vi.useRealTimers(); vi.restoreAllMocks();
  while (envs.length) rmSync(envs.pop()!.dir, { recursive: true, force: true });
  while (temps.length) temps.pop()!.cleanup();
});
async function prepared() {
  const f = await recoveryRig(); temps.push(f.temp);
  expect((await f.r.validatedMkPick(f.g.id, "recovery-pick")).ok).toBe(true);
  await f.r.svc.pickWorkIdle();
  const relay = f.r.svc.store.obligationsFor(f.g.id).find(o => o.kind === "pick-relay")!;
  expect(relay).toBeDefined();
  return { ...f, relay };
}

it("append before acknowledgment reopens and replays one byte-identical static ruling row", async () => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  const pause = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  expect((await r.validatedMkPick(g.id, "append-replay")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const ledger = join(temp.dir, "rulings.jsonl");
  appendRuling(ledger, r.svc.rulingLedgerPick(g.id)!);
  const original = readFileSync(ledger, "utf8");
  expect(r.svc.store.obligationsFor(g.id).find(o => o.kind === "ruling-file")).toMatchObject({ state: "pending", attempt: 0 });
  pause.mockRestore(); r.restart();
  r.svc.reconcile(g.id); r.svc.reconcile(g.id);
  expect(readFileSync(ledger, "utf8")).toBe(original);
  expect(original.trim().split("\n")).toHaveLength(1);
  expect(r.svc.store.obligationsFor(g.id).find(o => o.kind === "ruling-file")).toMatchObject({ state: "done", attempt: 0 });
  await r.svc.pickWorkIdle();
});

it("v6 Resend permit on an actual uncertain relay cannot authorize a new-loop blind retry", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk();
  sdk.send = async input => { sdk.sent.push(input); return { delivery: "sent" }; };
  await new WakeLoop(r.svc, sdk).drain();
  const v6 = await loadV6();
  const old = new v6.Store(new Database(r.env.file), { now: r.env.now });
  try {
    expect(old.resend(relay.id, 1, "old-click")).toEqual({ ok: true, replay: false });
    expect(old.obligation(relay.id)).toMatchObject({ resend_permit: 1, next_try_at: "9999-12-31T23:59:59.999Z" });
  } finally { old.close(); }
  const resumed = new WakeLoop(r.svc, sdk);
  await resumed.drain(); await resumed.drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "delivery uncertain" });
});

it.each([false, true])("provable pre-acceptance retry rechecks owner fences within the original budget (note=%s)", async note => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "throw", err: econnrefused() });
  const loop = new WakeLoop(r.svc, sdk); await loop.drain();
  expect(r.svc.store.attempts(relay.id)[0].state).toBe("not-delivered");
  const due = r.svc.store.obligation(relay.id)!.handler_next_try_at!;
  const action = r.db.prepare("SELECT evaluation_deadline_at FROM pick_actions WHERE decision_id = ?").get(g.id) as { evaluation_deadline_at: string };
  expect(Date.parse(due)).toBeLessThan(Date.parse(action.evaluation_deadline_at));
  r.env.clock.t = Date.parse(due);
  if (note) expect(r.svc.note({ decision_id: g.id }, "Wait", "retry-note", { deferWake: true }).ok).toBe(true);
  await loop.drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(note ? 1 : 2);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(note ? 1 : 2);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: note ? "manual handling required" : "already relayed" });
});

it("Home note during the final PR head read wins the send claim without changing the filed head", async () => {
  const head = "a".repeat(40);
  const { r, g, temp } = await recoveryRig({ subject: { kind: "github-pr", repo: "example/repo", number: 7, head } }); temps.push(temp);
  const evaluate = r.svc.pickEvaluation.bind(r.svc);
  let noteAtHead = false;
  const readHead = vi.fn(async () => {
    if (noteAtHead) expect(r.svc.note({ decision_id: g.id }, "Wait for review", "head-race-note", { deferWake: true }).ok).toBe(true);
    return { state: "OPEN", headRefOid: head };
  });
  vi.spyOn(r.svc, "pickEvaluation").mockImplementation((id, tasks, opts) => evaluate(id, tasks, { ...opts, readHead }));
  expect((await r.validatedMkPick(g.id, "head-race-pick")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-relay")).toHaveLength(1);
  noteAtHead = true;
  const sdk = new FakeSdk(); await new WakeLoop(r.svc, sdk).drain();
  expect(readHead.mock.calls.length).toBeGreaterThanOrEqual(2);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toEqual([]);
  const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")!;
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "manual handling required" });
});

it("ordinary startup backfill repairs missing legacy writes while relay delivery stays uncertain", async () => {
  const { r, g, task, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.send = async input => { sdk.sent.push(input); return { delivery: "sent" }; };
  await new WakeLoop(r.svc, sdk).drain();
  // A legacy pick missing its ordinary write records; protected action/outbox
  // records are retained. Store construction must still perform its old backfill.
  r.db.prepare("DELETE FROM card_writes WHERE decision_id = ?").run(g.id);
  const attempts = r.svc.store.attempts(relay.id);
  r.restart(); await r.writer.drain();
  expect(r.svc.store.attempts(relay.id)).toEqual(attempts);
  expect(r.fake.comments.filter(c => c.body.startsWith("Ruled:"))).toHaveLength(1);
  expect(task.labelIds).toEqual([]);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "delivery uncertain" });
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
});

it.each(["recipient", "text", "attempt", "obligation", "event_id"])("accepted-event provenance with mismatched %s cannot confirm or earn a receipt", async field => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  await new WakeLoop(r.svc, sdk).drain();
  const attempt = r.svc.store.attempts(relay.id)[0];
  r.svc.store.updateAttempt(relay.id, attempt.n, "delivered", { evidence: attempt.handle });
  const proof = { recipient: relay.recipient, text: sdk.sent[0].input, attempt: attempt.n, obligation: relay.id, event_id: attempt.handle };
  r.svc.store.recordEvent("pick-relay-accepted", g.id, { ...proof, [field]: field === "attempt" ? 99 : "wrong" });
  r.restart();
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "delivery uncertain" });
  reconcilePickReceipts(r.svc.store, r.env.now());
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toEqual([]);
  r.svc.store.recordEvent("pick-relay-accepted", g.id, proof);
  reconcilePickReceipts(r.svc.store, r.env.now());
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "already relayed" });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
});

it("a queue-handle-only placeholder is uncertain and cannot earn a receipt after restart", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  await new WakeLoop(r.svc, sdk).drain();
  const attempt = r.svc.store.attempts(relay.id)[0];
  // Legacy dispatched evidence can contain just the queue handle, with no durable host-event proof.
  r.svc.store.updateAttempt(relay.id, attempt.n, "delivered", { evidence: attempt.handle });
  r.restart();
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "delivery uncertain" });
  reconcilePickReceipts(r.svc.store, r.env.now());
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toEqual([]);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
});

it.each(["off", "epoch"].flatMap(change => ["pending", "unsent", "claiming", "queued", "uncertain", "confirmed"].map(phase => ({ change, phase }))))(
  "$change invalidation at $phase prevents new action attempts while ordinary Ruled writes survive", async ({ change, phase }) => {
    const f = await recoveryRig(); temps.push(f.temp);
    const { r, g } = f;
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    if (phase === "pending") {
      const evaluate = r.svc.pickEvaluation.bind(r.svc);
      vi.spyOn(r.svc, "pickEvaluation").mockImplementation(async (...args) => { await gate; return evaluate(...args); });
    }
    expect((await r.validatedMkPick(g.id, "phase-pick")).ok).toBe(true);
    if (phase !== "pending") await r.svc.pickWorkIdle();
    const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay");
    const sdk = new FakeSdk();
    const loop = new WakeLoop(r.svc, sdk);
    if (phase === "claiming") expect(claimPickRelay(r.svc.store, relay!.id, 0, r.env.now())).toEqual({ ok: true, attempt: 1 });
    if (phase === "queued") { sdk.script.push({ kind: "queue" }); await loop.drain(); }
    if (phase === "uncertain") {
      sdk.send = async input => { sdk.sent.push(input); return { delivery: "sent" }; };
      await loop.drain();
    }
    if (phase === "confirmed") await loop.drain();
    const before = sdk.sent.filter(s => s.threadId === "thr_target").length;
    r.db.prepare(change === "off" ? "UPDATE pick_handler_control SET mode = 'off', epoch = epoch + 1" : "UPDATE pick_handler_control SET epoch = epoch + 1").run();
    r.svc.expirePickEvaluations();
    release(); await r.svc.pickWorkIdle();
    await loop.drain(); await loop.drain();
    await r.writer.drain();
    expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(before);
    expect(r.fake.comments.filter(c => c.body.startsWith("Ruled:"))).toHaveLength(1);
    expect(r.fake.comments.filter(c => c.body.includes("home-pick-status:"))).toEqual([]);
    expect(r.db.prepare("SELECT kind FROM card_writes WHERE decision_id = ? ORDER BY kind").all(g.id)).toEqual([{ kind: "comment" }, { kind: "unlabel" }]);
    if (phase === "queued") expect(sdk.deletes).toHaveLength(1);
    const view = r.svc.readPickAction({ decision_id: g.id, generation: 1 });
    expect(view).toMatchObject({ handler_status: phase === "confirmed" ? "already relayed" : ["claiming", "uncertain"].includes(phase) ? "delivery uncertain" : "manual handling required" });
    if (relay) expect(r.svc.store.attempts(relay.id)).toHaveLength(phase === "unsent" ? 0 : 1);
  },
);

it("dispatch winning invalidation cancellation retains late positive evidence and earns only one receipt", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  const loop = new WakeLoop(r.svc, sdk); await loop.drain();
  const handle = r.svc.store.attempts(relay.id)[0].handle!;
  vi.spyOn(sdk, "queuedDelete").mockImplementation(async () => { sdk.dispatch(handle); throw new Error("already dispatched"); });
  r.db.prepare("UPDATE pick_handler_control SET mode = 'off', epoch = epoch + 1").run();
  await loop.drain(); await loop.drain();
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "already relayed" });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
});

it("receipt insertion failure after accepted send repairs after reopening without another action", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk();
  const prepare = r.db.prepare.bind(r.db);
  const fault = vi.spyOn(r.db, "prepare").mockImplementation(sql => {
    if (sql.includes("INSERT INTO obligations(id, decision_id, kind") && sdk.sent.length) throw new Error("receipt insertion fault");
    return prepare(sql);
  });
  await expect(new WakeLoop(r.svc, sdk).drain()).rejects.toThrow("receipt insertion fault");
  fault.mockRestore();
  expect(r.svc.store.attempts(relay.id)).toMatchObject([{ state: "delivered", evidence: sdk.accepted[0].id }]);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toEqual([]);
  r.restart();
  const loop = new WakeLoop(r.svc, sdk);
  await loop.drain(); await loop.drain();
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(sdk.sent.filter(s => s.input.includes("already relayed; do not act"))).toHaveLength(1);
});

it("receipt declared-identity conflict survives reopen without overwriting it or repeating the action", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); await new WakeLoop(r.svc, sdk).drain();
  const receipt = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-receipt")!;
  r.db.prepare("UPDATE obligations SET payload = 'conflicting receipt' WHERE id = ?").run(receipt.id);
  const saved = r.svc.store.obligation(receipt.id);
  expect(() => reconcilePickReceipts(r.svc.store, r.env.now())).toThrow("protected obligation conflict");
  r.restart();
  expect(() => reconcilePickReceipts(r.svc.store, r.env.now())).toThrow("protected obligation conflict");
  expect(r.svc.store.obligation(receipt.id)).toEqual(saved);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "already relayed" });
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
});

it("accepted-event persistence failure rolls back confirmation then recovers exact provenance after reopen", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  const loop = new WakeLoop(r.svc, sdk); await loop.drain();
  const handle = r.svc.store.attempts(relay.id)[0].handle!;
  sdk.dispatch(handle); sdk.accepted[0].id = handle;
  const record = r.svc.store.recordEvent.bind(r.svc.store);
  const fault = vi.spyOn(r.svc.store, "recordEvent").mockImplementation((type, decision, detail) => {
    if (type === "pick-relay-accepted") throw new Error("event persistence fault");
    return record(type, decision, detail);
  });
  await loop.reconcile();
  expect(r.svc.store.attempts(relay.id)[0].state).not.toBe("delivered");
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toEqual([]);
  fault.mockRestore(); r.restart();
  const resumed = new WakeLoop(r.svc, sdk);
  await resumed.reconcile(); await resumed.drain(); await resumed.drain();
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "already relayed", facts: { delivery: { state: "confirmed", host_message_id: handle } } });
  expect(r.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'pick-relay-accepted'").all(g.id)).toHaveLength(1);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
});

it("actual subprocess restart preserves the original absolute deadline and expires without a send", async () => {
  const { r, g, relay } = await prepared();
  const original = r.db.prepare("SELECT evaluation_deadline_at FROM pick_actions WHERE decision_id = ?").get(g.id);
  r.advance(120_000);
  const execution = promisify(execFile)(process.execPath, [fileURLToPath(new URL("./fixtures/pickhandler-restart.mjs", import.meta.url)), r.env.file, r.env.now(), g.id], { cwd: fileURLToPath(new URL("../", import.meta.url)) });
  const run = await execution;
  expect(run.stdout, `restart exit=${execution.child.exitCode}, signal=${execution.child.signalCode}, stderr: ${run.stderr}`).not.toBe("");
  const result = JSON.parse(run.stdout);
  expect(result.action.evaluation_deadline_at).toBe(Object.values(original!)[0]);
  expect(result.action.disposition).toBe("fallback");
  expect(result.obligations.find((o: { id: string }) => o.id === relay.id).voided_at).not.toBeNull();
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
});

it.each(["acceptedEvents", "queuedState"] as const)("hung %s reconciliation releases ordinary progress before the read finishes", async method => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  const loop = new WakeLoop(r.svc, sdk);
  await loop.drain();
  const handle = r.svc.store.attempts(relay.id)[0].handle!;
  sdk.dispatch(handle);
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const probe = method === "acceptedEvents"
    ? vi.spyOn(sdk, "acceptedEvents").mockImplementation(async () => { await gate; return sdk.accepted; })
    : vi.spyOn(sdk, "queuedState").mockImplementation(async () => { await gate; return "dispatched"; });
  r.svc.store.insertObligations(g.id, [{ id: "ordinary-progress", kind: "wake", recipient: "thr_other", op: "ordinary-progress", payload: "Ordinary progress" }]);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  let settled = false;
  const pending = loop.drain().then(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(0);
  expect(probe).toHaveBeenCalled();
  r.advance(120_000); r.svc.expirePickEvaluations();
  await vi.advanceTimersByTimeAsync(120_000);
  try {
    expect(settled).toBe(true);
    expect(sdk.sent.some(s => s.threadId === "thr_other")).toBe(true);
  } finally { release(); await pending; probe.mockRestore(); }
  await loop.reconcile();
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("already relayed");
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  const query = { decision_id: g.id, generation: 1 };
  expect(r.svc.readPickAction(query)).toMatchObject({ kind: "action", handler_status: "already relayed",
    ordinary_continuation: false, facts: { delivery: { state: "confirmed", host_message_id: sdk.accepted[0].id } } });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  r.restart();
  expect(r.svc.readPickAction(query)).toMatchObject({ kind: "action", handler_status: "already relayed",
    facts: { delivery: { state: "confirmed", host_message_id: sdk.accepted[0].id } } });
  const resumed = new WakeLoop(r.svc, sdk);
  await resumed.reconcile();
  await resumed.drain();
  await resumed.drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
  const receipts = r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt");
  expect(receipts).toHaveLength(1);
  expect(receipts[0].state).toBe("done");
  expect(sdk.sent.filter(s => s.input.includes("already relayed; do not act"))).toHaveLength(1);
});

it("accepted-event proof remains distinct when its ID equals the old queue handle", async () => {
  const { r, g, relay } = await prepared();
  const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
  const loop = new WakeLoop(r.svc, sdk);
  await loop.drain();
  const handle = r.svc.store.attempts(relay.id)[0].handle!;
  sdk.dispatch(handle);
  sdk.accepted[0].id = handle;
  await loop.reconcile();
  expect(r.svc.store.attempts(relay.id)[0]).toMatchObject({ state: "delivered", evidence: handle });
  expect(readPickHandlerStatus(r.svc.store, g.id)).toBe("already relayed");
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  const query = { decision_id: g.id, generation: 1 };
  expect(r.svc.readPickAction(query)).toMatchObject({ kind: "action", handler_status: "already relayed",
    ordinary_continuation: false, facts: { delivery: { state: "confirmed", host_message_id: handle } } });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  const provenance = r.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'pick-relay-accepted'").all(g.id);
  expect(provenance).toHaveLength(1);
  expect(JSON.parse(String(Reflect.get(provenance[0]!, "detail_json")))).toMatchObject({
    obligation: relay.id, attempt: 1, event_id: handle, recipient: relay.recipient, text: sdk.sent[0].input,
  });
  r.restart();
  expect(r.svc.readPickAction(query)).toMatchObject({ kind: "action", handler_status: "already relayed",
    facts: { delivery: { state: "confirmed", host_message_id: handle } } });
  const resumed = new WakeLoop(r.svc, sdk);
  await resumed.reconcile(); await resumed.drain(); await resumed.drain();
  expect(r.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'pick-relay-accepted'").all(g.id)).toEqual(provenance);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  expect(sdk.sent.filter(s => s.input.includes("already relayed; do not act"))).toHaveLength(1);
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
});
