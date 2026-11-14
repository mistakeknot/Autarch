import Database from "better-sqlite3";
import { afterEach, expect, it } from "vitest";
import { readFileSync, rmSync } from "node:fs";
import { envs } from "./card-rig.js";
import { recoveryRig } from "./pickhandler-stage4-rig.js";
import { loadV6 } from "./v6build.js";
import { FakeSdk } from "./wakes-helpers.js";
import { WakeLoop } from "../wakes.js";

const temps: { cleanup(): void }[] = [];
afterEach(() => {
  while (envs.length) rmSync(envs.pop()!.dir, { recursive: true, force: true });
  while (temps.length) temps.pop()!.cleanup();
});

it("actual earned receipt remains inert in v6 after draining the started relay", async () => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "earned-rollback")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const sdk = new FakeSdk(); await new WakeLoop(r.svc, sdk).drain();
  const receipt = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-receipt")!;
  expect(receipt).toMatchObject({ state: "pending", handler_next_try_at: r.env.now(), next_try_at: "9999-12-31T23:59:59.999Z" });
  expect(r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")?.state).toBe("done");
  const v6 = await loadV6();
  const old = new v6.Store(new Database(r.env.file), { now: r.env.now });
  try {
    const service = Object.create(r.svc); Object.defineProperty(service, "store", { value: old });
    r.svc.store.insertObligations(g.id, [{ id: "earned-legacy", kind: "wake", recipient: "thr_legacy", op: "earned-legacy", payload: "ordinary positive" }]);
    await new v6.WakeLoop(service, sdk).drain();
    expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_target", "thr_legacy"]);
    expect(old.attempts(receipt.id)).toEqual([]);
    expect(old.obligation(receipt.id)).toEqual(receipt);
  } finally { old.close(); }
});

it("manifest-verified v6 leaves captured relay and receipt sentinel inert while a legacy wake delivers", async () => {
  const { r, g, temp } = await recoveryRig(); temps.push(temp);
  expect((await r.validatedMkPick(g.id, "rollback-pick")).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const relay = r.svc.store.obligationsFor(g.id).find(o => o.kind === "pick-relay")!;
  expect(relay).toBeDefined();
  const sentinel = "9999-12-31T23:59:59.999Z";
  r.db.prepare(`INSERT INTO obligations(id,decision_id,kind,recipient,payload,op,next_try_at,handler_next_try_at,updated_at,allow_batch)
    VALUES ('receipt-sentinel',?,'pick-receipt','thr_a','already relayed; do not act','receipt-sentinel',?,?,?,0)`)
    .run(g.id, sentinel, r.env.now(), r.env.now());
  r.svc.store.insertObligations(g.id, [{ id: "legacy-control", kind: "wake", recipient: "thr_legacy", op: "legacy-control", payload: "legacy exact 日本語\n\u0007" }]);
  const v6 = await loadV6();
  const old = new v6.Store(new Database(r.env.file), { now: r.env.now, noticeWindowMs: 0 });
  const sdk = new FakeSdk();
  // The actual Service is structurally compatible with the frozen loop. Its store getter is overridden
  // through the inherited instance solely for the old-reader drill, without running concurrent senders.
  const service = Object.create(r.svc);
  Object.defineProperty(service, "store", { value: old });
  const loop = new v6.WakeLoop(service, sdk, async () => "thr_successor");
  try {
    await loop.drain();
    expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_legacy"]);
    expect(sdk.sent[0].input).toBe("[home-msg legacy-control #1] legacy exact 日本語\n\u0007");
    for (const id of [relay.id, "receipt-sentinel"]) {
      old.resend(id, 0, `resend-${id}`);
      expect(old.obligation(id)?.next_try_at).toBe(sentinel);
      expect(old.attempts(id)).toEqual([]);
    }
    await loop.drain();
    expect(sdk.sent).toHaveLength(1);
    const kinds = new Set(["wake", "ruling-file", "notice", "notice-digest", "steps-done", "hold-ended", "note", "owner", "idea", "void-notice", "resolve", "withdraw", "pick-relay", "pick-receipt"]);
    expect(r.svc.store.obligationsFor(g.id).every(o => kinds.has(o.kind))).toBe(true);
    const action = r.db.prepare("SELECT status_op FROM pick_actions WHERE decision_id = ?").get(g.id) as { status_op: string };
    expect(r.svc.store.obligationsFor(g.id).some(o => o.op === action.status_op)).toBe(false);
    expect(r.fake.comments.some(c => c.body.includes("home-pick-status:"))).toBe(false);
    expect(() => v6.parseExistingAsk(JSON.parse(String(r.svc.store.decision(g.id)!.body_json)))).toThrow('unknown field "on_pick"');
    // Closed inventory derived from the frozen producers: kind has no SQL enum,
    // so the frozen Service/Delegation/Store declarations are the legacy contract.
    const legacy = ["owner", "ruling-file", "wake", "notice", "steps-done", "hold-ended", "note", "void-notice", "notice-digest"];
    const producers = ["service.ts", "delegation.ts", "store.ts"].map(name => readFileSync(new URL(`./fixtures/v6-reader/${name}`, import.meta.url), "utf8")).join("\n");
    for (const kind of legacy) expect(producers.includes(`kind: "${kind}"`) || producers.includes(`DIGEST_KIND = "${kind}"`)).toBe(true);
    const closedKinds = new Set([...legacy, "pick-relay", "pick-receipt"]);
    expect(r.svc.store.obligationsFor(g.id).every(o => closedKinds.has(o.kind))).toBe(true);
    expect(r.db.prepare("SELECT kind FROM card_writes WHERE decision_id = ? ORDER BY kind").all(g.id)).toEqual([{ kind: "comment" }, { kind: "unlabel" }]);

    // Exercise successful v6 Resend, not just its stale/no-op branch. These are
    // synthetic unstarted rollback sentinels; no mixed-binary sending is claimed.
    for (const id of [relay.id, "receipt-sentinel"]) {
      r.db.prepare("UPDATE obligations SET state = 'uncertain' WHERE id = ?").run(id);
      expect(old.resend(id, 0, `uncertain-${id}`)).toEqual({ ok: true, replay: false });
      expect(old.obligation(id)).toMatchObject({ state: "pending", resend_permit: 1, next_try_at: sentinel });
    }
    r.svc.store.insertObligations(g.id, [{ id: "legacy-resend", kind: "wake", recipient: "thr_legacy", op: "legacy-resend", payload: "exact resend 日本語\n\u0007" }]);
    r.db.prepare("UPDATE obligations SET state = 'uncertain' WHERE id = 'legacy-resend'").run();
    expect(old.resend("legacy-resend", 0, "legacy-click")).toEqual({ ok: true, replay: false });
    await loop.drain();
    expect(sdk.sent.at(-1)?.input).toBe("[home-msg legacy-resend #1] exact resend 日本語\n\u0007");
    for (const id of [relay.id, "receipt-sentinel"]) expect(old.attempts(id)).toEqual([]);

    // Frozen stranded sweep reroutes the ordinary positive control in the same
    // drain while ignoring both new kinds despite their matching archived error.
    for (const id of [relay.id, "receipt-sentinel"]) r.db.prepare("UPDATE obligations SET state = 'undeliverable', last_error = 'thread archived' WHERE id = ?").run(id);
    r.svc.store.insertObligations(g.id, [{ id: "legacy-stranded", kind: "wake", recipient: "thr_gone", op: "legacy-stranded", payload: "stranded exact" }]);
    r.db.prepare("UPDATE obligations SET state = 'undeliverable', last_error = 'thread archived' WHERE id = 'legacy-stranded'").run();
    await loop.drain();
    expect(sdk.sent.at(-1)).toMatchObject({ threadId: "thr_successor", input: "[home-msg legacy-stranded #1] stranded exact" });
    expect(old.obligation(relay.id)?.recipient).toBe("thr_target");
    expect(old.obligation("receipt-sentinel")?.recipient).toBe("thr_a");
    for (const id of [relay.id, "receipt-sentinel"]) expect(old.attempts(id)).toEqual([]);
    expect(r.svc.store.obligationsFor(g.id).every(o => closedKinds.has(o.kind))).toBe(true);
    const exact = "queued ordinary 日本語\n\u0007  trailing  ";
    old.insertObligations(g.id, [{ id: "legacy-queued", kind: "wake", recipient: "thr_legacy", op: "legacy-queued", payload: exact }]);
    sdk.script.push({ kind: "queue" });
    await loop.drain();
    const queued = old.attempts("legacy-queued")[0];
    expect(queued.state).toBe("queued");
    expect(sdk.rows.get(queued.handle!)?.text).toBe(`[home-msg legacy-queued #1] ${exact}`);
    // Roll forward after the old drain finishes; reconcile the same bytes.
    sdk.dispatch(queued.handle!);
    await new WakeLoop(r.svc, sdk).reconcile();
    expect(r.svc.store.attempts("legacy-queued")[0].state).toBe("delivered");
    expect(r.svc.store.obligation("legacy-queued")?.payload).toBe(exact);
    expect(sdk.sent.filter(s => s.input.startsWith("[home-msg legacy-queued #"))).toHaveLength(1);
  } finally { old.close(); }
});
