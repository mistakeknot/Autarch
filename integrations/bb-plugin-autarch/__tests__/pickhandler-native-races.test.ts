import { afterEach, expect, it } from "vitest";
import { Worker } from "node:worker_threads";
import { rmSync } from "node:fs";
import { join } from "node:path";
import { recoveryRig } from "./pickhandler-stage4-rig.js";
import { envs } from "./card-rig.js";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { FakeSdk } from "./wakes-helpers.js";
import { WakeLoop } from "../wakes.js";

const temps: { cleanup(): void }[] = [];
afterEach(() => {
  while (envs.length) rmSync(envs.pop()!.dir, { recursive: true, force: true });
  while (temps.length) temps.pop()!.cleanup();
});

async function race(data: { file: string; now: string; decision: string; relay?: string; operation: "capture" | "claim" }) {
  const barrier = new SharedArrayBuffer(4);
  let ready = 0;
  const workers = [0, 1].map(() => new Worker(new URL("./fixtures/pickhandler-race.mjs", import.meta.url), { workerData: { ...data, barrier } }));
  try {
    return await Promise.all(workers.map(worker => new Promise<unknown>((resolve, reject) => {
      let result: unknown;
      worker.on("error", reject);
      worker.on("message", (message: { ready?: boolean; result?: unknown }) => {
        if (message.ready && ++ready === 2) { Atomics.store(new Int32Array(barrier), 0, 1); Atomics.notify(new Int32Array(barrier), 0, 2); }
        if ("result" in message) result = message.result;
      });
      worker.on("exit", code => code === 0 && result !== undefined ? resolve(result) : reject(new Error(`race worker exit ${code}; result present: ${result !== undefined}`)));
    })));
  } finally { await Promise.all(workers.map(worker => worker.terminate())); }
}

it("two independently opened SQLite connections race the same declared capture identity", async () => {
  const { r, g, temp } = await recoveryRig({ svc: { pickCaptureRuntime: undefined } }); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "racing-capture")).ok).toBe(true);
  expect(r.db.prepare("SELECT pick_id FROM pick_actions").all()).toEqual([]);
  const results = await race({ file: r.env.file, now: r.env.now(), decision: g.id, operation: "capture" });
  expect(results).toHaveLength(2); expect(results[0]).toEqual(results[1]);
  expect(r.db.prepare("SELECT pick_id FROM pick_actions").all()).toEqual([{ pick_id: "racing-capture" }]);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-relay")).toEqual([]);
});

it("two independently opened SQLite connections race one declared relay claim", async () => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "racing-claim")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")!;
  expect(relay).toBeDefined();
  const results = await race({ file: r.env.file, now: r.env.now(), decision: g.id, relay: relay.id, operation: "claim" });
  expect(results).toEqual(expect.arrayContaining([{ ok: true, attempt: 1 }, { ok: false }]));
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
  expect(r.svc.store.obligation(relay.id)).toMatchObject({ attempt: 1, state: "sending", next_try_at: "9999-12-31T23:59:59.999Z" });
});

it.each(["queued", "response-lost", "confirmed"])("actual subprocess restart at %s preserves delivery history without a new action", async phase => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "restart-phase-pick")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")!;
  const sdk = new FakeSdk();
  if (phase === "queued") sdk.script.push({ kind: "queue" });
  if (phase === "response-lost") {
    // Acceptance survives the lost response, but cannot be read until after restart.
    sdk.caps.events = false;
    const send = sdk.send.bind(sdk);
    let first = true;
    sdk.send = async input => {
      const accepted = await send(input);
      if (first) { first = false; throw new Error("accepted response lost"); }
      return accepted;
    };
  }
  await new WakeLoop(r.svc, sdk).drain();
  const deadline = r.db.prepare("SELECT evaluation_deadline_at FROM pick_actions WHERE decision_id = ?").get(g.id);
  const saved = r.svc.store.attempts(relay.id);
  const savedObligations = r.svc.store.obligationsFor(g.id);
  const acceptedBeforeRestart = sdk.accepted.map(event => ({ ...event }));
  const assertUnconfirmedBoundary = (readback: unknown) => {
    expect(readback).toMatchObject({
      ok: true, handler_status: phase === "queued" ? "pending" : "delivery uncertain",
      ordinary_continuation: false,
      facts: { delivery: { state: phase === "queued" ? "queued" : "uncertain", host_message_id: null } },
    });
    expect(r.svc.store.attempts(relay.id)).toEqual(saved);
    expect(saved).toHaveLength(1);
    expect(saved[0]).toMatchObject({ obligation_id: relay.id, n: 1, state: phase === "queued" ? "queued" : "uncertain", evidence: null });
    expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-relay")).toEqual([r.svc.store.obligation(relay.id)]);
    expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toEqual([]);
    expect(sdk.sent).toHaveLength(1);
    expect(sdk.sent[0].threadId).toBe("thr_target");
    expect(sdk.accepted).toEqual(acceptedBeforeRestart);
    if (phase === "queued") {
      expect(saved[0].handle).toEqual(expect.any(String));
      expect(saved[0].handle).not.toBe("");
      expect(sdk.rows.get(saved[0].handle!)).toEqual({ id: saved[0].handle, threadId: "thr_target", text: sdk.sent[0].input, state: "queued" });
      expect(sdk.accepted).toEqual([]);
    } else {
      expect(saved[0].handle).toBeNull();
      expect(sdk.caps.events).toBe(false);
      expect(sdk.accepted).toHaveLength(1);
      expect(sdk.accepted[0]).toEqual({ id: expect.any(String), threadId: "thr_target", text: sdk.sent[0].input });
      expect(sdk.accepted[0].id).not.toBe("");
    }
  };
  if (phase !== "confirmed") assertUnconfirmedBoundary(r.svc.readPickAction({ decision_id: g.id, generation: 1 }));
  const run = await promisify(execFile)(process.execPath, [fileURLToPath(new URL("./fixtures/pickhandler-restart.mjs", import.meta.url)), r.env.file, r.env.now(), g.id, join(temp.dir, "rulings.jsonl")], { cwd: fileURLToPath(new URL("../", import.meta.url)) });
  expect(run.stdout, run.stderr).not.toBe("");
  const result = JSON.parse(run.stdout);
  expect(result.action.evaluation_deadline_at).toBe(Object.values(deadline!)[0]);
  expect(r.svc.store.attempts(relay.id)).toEqual(saved);
  expect(result.readback.handler_status).toBe(phase === "confirmed" ? "already relayed" : phase === "queued" ? "pending" : "delivery uncertain");
  if (phase !== "confirmed") {
    expect(result.obligations).toEqual(savedObligations);
    assertUnconfirmedBoundary(result.readback);
  }
  if (phase === "queued") sdk.dispatch(saved[0].handle!);
  if (phase === "response-lost") sdk.caps.events = true;
  const acceptedRelay = sdk.accepted.find(event => event.threadId === "thr_target");
  const loop = new WakeLoop(r.svc, sdk);
  await loop.reconcile(); await loop.drain(); await loop.drain();
  expect(sdk.sent.filter(s => s.threadId === "thr_target")).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toHaveLength(1);
  expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({ handler_status: "already relayed" });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-receipt")).toHaveLength(1);
  if (phase !== "confirmed") {
    expect(acceptedRelay).toBeDefined();
    expect(sdk.accepted.filter(event => event.threadId === "thr_target")).toEqual([acceptedRelay]);
    expect(r.svc.store.attempts(relay.id)[0]).toMatchObject({ n: 1, state: "delivered", evidence: acceptedRelay!.id });
    expect(r.svc.readPickAction({ decision_id: g.id, generation: 1 })).toMatchObject({
      ordinary_continuation: false, facts: { delivery: { state: "confirmed", host_message_id: acceptedRelay!.id } },
    });
    const receipt = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-receipt")!;
    expect(receipt).toMatchObject({ op: result.action.receipt_op, recipient: result.action.asker, state: "done" });
    expect(receipt.payload).toContain(acceptedRelay!.id);
    expect(sdk.sent.filter(send => send.threadId === result.action.asker)).toHaveLength(1);
  }
});
