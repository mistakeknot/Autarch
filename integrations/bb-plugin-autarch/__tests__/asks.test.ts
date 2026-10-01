import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import type { Service } from "../service.js";
import { Store } from "../store.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { ask, makeEnv, T0, type Env } from "./service-helpers.js";

let env: Env;
let svc: Service;
let asks: Asks;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  asks = new Asks(svc);
});
afterEach(() => env.cleanup());

const HOUR = 3_600_000;
const stepsAsk = (thread: string, subject: string, steps = ["do a", "do b"], over: Record<string, unknown> = {}) =>
  ask(env, { kind: "steps", options: undefined, steps, thread, subject, question: `steps for ${subject}`, ...over });
const machineAsk = (over: Record<string, unknown> = {}, machine: Record<string, unknown> = {}) =>
  ask(env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "runner is down", ...machine }, thread: "thr-a", subject: "ci: runner down", question: "the CI runner is down", ...over });
const fileId = async (req: Record<string, unknown>) => {
  const r = await svc.file(req, {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};
const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
const done = (id: string, pickId = `pk-${id}`) => svc.pick(id, "done", rev(id), pickId, "mk", "home");
const wakesFor = (thread: string, s: Service = svc) => s.wakes().filter((w) => w.recipient === thread);
const obs = (id: string, s: Service = svc) => s.store.obligationsFor(id);

describe("the runbook", () => {
  it("merges every thread's open steps, grouped by thread in filing order", async () => {
    const a1 = await fileId(stepsAsk("thr-a", "s1"));
    const b1 = await fileId(stepsAsk("thr-b", "s2", ["only b"]));
    env.clock.t += 1000;
    const a2 = await fileId(stepsAsk("thr-a", "s3", ["later a"]));
    const rb = asks.runbook();
    expect(rb.map((g) => [g.thread, g.items.map((i) => i.id)])).toEqual([["thr-a", [a1, a2]], ["thr-b", [b1]]]);
    expect(rb[0]!.items[0]!.steps).toEqual(["do a", "do b"]);
    done(a1);
    expect(asks.runbook().map((g) => [g.thread, g.items.map((i) => i.id)])).toEqual([["thr-b", [b1]], ["thr-a", [a2]]]);
  });

  it("marking one thread's steps done wakes only that thread, once across a retry", async () => {
    const a = await fileId(stepsAsk("thr-a", "s1"));
    await fileId(stepsAsk("thr-b", "s2", ["only b"]));
    const first = done(a, "click-1");
    expect(first).toMatchObject({ ok: true, status: 201 });
    expect(done(a, "click-1")).toMatchObject({ ok: true, status: 200 });
    const w = wakesFor("thr-a");
    expect(w).toHaveLength(1);
    expect(w[0]!.op).toBe(`steps-done:${a}:thr-a`);
    expect(w[0]!.payload).toContain("mk finished your steps");
    expect(w[0]!.payload).toContain("do a");
    expect(w[0]!.payload).toContain("do b");
    expect(wakesFor("thr-b")).toEqual([]);
    expect(obs(a).map((o) => o.kind)).toEqual(["steps-done"]);
  });

  it("only the option done is valid on a steps ask", async () => {
    const a = await fileId(stepsAsk("thr-a", "s1"));
    expect(svc.pick(a, "yes", rev(a), "pk", "mk", "home")).toMatchObject({ ok: false, status: 400 });
    expect(svc.store.pick(a)).toBeUndefined();
  });

  it("a steps ask with two mentioners: three steps-done rows in one transaction, none on a retry [F-6]", async () => {
    const a = await fileId(stepsAsk("thr-a", "same"));
    for (const t of ["thr-b", "thr-c"]) expect(await svc.file(stepsAsk(t, "same"), {})).toMatchObject({ mentioned: true, decision_id: a });
    // a crash between the second and third row leaves nothing
    let n = 0;
    const crashing = env.open({
      store: new Store(new Database(env.file), {
        now: env.now,
        hook: (step) => {
          if (step === "obligation-row" && ++n === 2) throw new Error("crash");
        },
      }),
    });
    expect(() => crashing.pick(a, "done", rev(a), "click-1", "mk", "home")).toThrow("crash");
    expect(svc.store.pick(a)).toBeUndefined();
    expect(obs(a)).toEqual([]);
    expect(done(a, "click-1")).toMatchObject({ ok: true });
    expect(obs(a).map((o) => o.op).sort()).toEqual([`steps-done:${a}:thr-a`, `steps-done:${a}:thr-b`, `steps-done:${a}:thr-c`]);
    expect(done(a, "click-1")).toMatchObject({ ok: true, status: 200 });
    expect(obs(a)).toHaveLength(3);
    // after a restart, one message for each thread
    const again = env.open();
    expect(again.wakes().map((w) => w.recipient).sort()).toEqual(["thr-a", "thr-b", "thr-c"]);
  });
});

describe("the machine lane", () => {
  it("a blocker with an owner is in the lane, not in Asks, and wakes the owner", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    const l = asks.lists();
    expect(l.lane.map((e) => e.id)).toEqual([m]);
    expect(l.asks.map((e) => e.id)).toEqual([]);
    const w = wakesFor("thr-own");
    expect(w).toHaveLength(1);
    expect(w[0]!.op).toBe(`owner:${m}`);
    expect(w[0]!.payload).toContain(`bb home get --id ${m}`);
    expect(w[0]!.payload).toContain("still open");
    expect(wakesFor("thr-a")).toEqual([]);
  });

  it("the owner notice is in the filing's own transaction: a crash after the decision row leaves no blocker", async () => {
    let boom = false;
    const crashing = env.open({});
    (crashing.store as unknown as { hook: (s: string) => void }).hook = (step: string) => {
      if (boom && step === "obligation-row") throw new Error("crash");
    };
    boom = true;
    await expect(crashing.file(machineAsk({}, { owner_thread: "thr-own" }), {})).rejects.toThrow("crash");
    boom = false;
    expect(svc.store.db.prepare("SELECT COUNT(*) AS n FROM decisions").get()).toEqual({ n: 0 });
    expect(svc.store.db.prepare("SELECT COUNT(*) AS n FROM requests").get()).toEqual({ n: 0 });
  });

  it("a retry of the filing after the blocker resolved inserts no stale owner notice", async () => {
    const req = machineAsk({}, { owner_thread: "thr-own" });
    const m = await fileId(req);
    asks.resolve(m, "runner replaced", { threadId: "thr-own" });
    // as if the notice had never been written
    svc.store.db.prepare("DELETE FROM obligations WHERE op = ?").run(`owner:${m}`);
    const again = await svc.file(req, {});
    expect(again).toMatchObject({ ok: true, decision_id: m });
    expect(obs(m).some((o) => o.op === `owner:${m}`)).toBe(false);
  });

  it("a blocker with no owner is in Asks as an unowned machine blocker and wakes nobody", async () => {
    const m = await fileId(machineAsk());
    const l = asks.lists();
    expect(l.lane).toEqual([]);
    expect(l.asks).toEqual([expect.objectContaining({ id: m, label: "unowned machine blocker" })]);
    expect(svc.wakes()).toEqual([]);
  });

  it("the machineOwners setting supplies the owner, stored at filing so a later change does not move it", async () => {
    svc.store.setSetting("machineOwners", JSON.stringify({ ci: "thr-own" }));
    const m = await fileId(machineAsk());
    expect(asks.lists().lane.map((e) => e.id)).toEqual([m]);
    expect(wakesFor("thr-own")).toHaveLength(1);
    svc.store.setSetting("machineOwners", JSON.stringify({ ci: "thr-other" }));
    expect(asks.progress(m, "still on it", { threadId: "thr-own" })).toMatchObject({ ok: true });
    expect(asks.progress(m, "x", { threadId: "thr-other" })).toMatchObject({ ok: false, status: 403 });
    expect(asks.resolve(m, "fixed", { threadId: "thr-own" })).toMatchObject({ ok: true });
    expect(wakesFor("thr-a").map((w) => w.op)).toEqual([`resolve:${m}:thr-a`]);
  });

  it("an owned blocker older than 24 h is promoted to Asks as stalled; progress resets the clock", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    const m2 = await fileId(machineAsk({ subject: "ci: second", question: "second runner down" }, { owner_thread: "thr-own" }));
    env.clock.t = T0 + 23 * HOUR;
    expect(asks.progress(m2, "working", { threadId: "thr-own" })).toMatchObject({ ok: true });
    env.clock.t = T0 + 25 * HOUR;
    const l = asks.lists();
    expect(l.asks.map((e) => [e.id, e.label])).toEqual([[m, "stalled"]]);
    expect(l.lane.map((e) => e.id)).toEqual([m2]);
    env.clock.t = T0 + 48 * HOUR + 1000;
    expect(asks.lists().asks.map((e) => e.id).sort()).toEqual([m, m2].sort());
  });

  it("progress needs the owner thread and an open blocker, and appends an event", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    expect(asks.progress(m, "n", { threadId: "thr-a" })).toMatchObject({ ok: false, status: 403 });
    expect(asks.progress(m, "n", {})).toMatchObject({ ok: false, status: 403 });
    expect(asks.progress("nope", "n", { threadId: "thr-own" })).toMatchObject({ ok: false, status: 404 });
    expect(asks.progress(m, "half done", { threadId: "thr-own" })).toMatchObject({ ok: true });
    expect(svc.store.events().some((e) => e.type === "progress" && e.decision_id === m && e.detail_json.includes("half done"))).toBe(true);
    asks.resolve(m, undefined, { threadId: "thr-own" });
    expect(asks.progress(m, "late", { threadId: "thr-own" })).toMatchObject({ ok: false, status: 409 });
  });
});

describe("blocker lifecycle [D-11]", () => {
  it("resolve before promotion and after promotion both remove it and wake the asker once across a retry", async () => {
    const m1 = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    const r1 = asks.resolve(m1, "fixed", { threadId: "thr-own" }, "req-1");
    expect(r1).toMatchObject({ ok: true, replay: false });
    expect(asks.resolve(m1, "fixed", { threadId: "thr-own" }, "req-1")).toMatchObject({ ok: true, replay: true });
    expect(asks.resolve(m1, "again", { threadId: "thr-own" }, "req-2")).toMatchObject({ ok: false, status: 409 });
    expect(wakesFor("thr-a").filter((w) => w.op === `resolve:${m1}:thr-a`)).toHaveLength(1);
    const m2 = await fileId(machineAsk({ subject: "ci: two", question: "two down" }, { owner_thread: "thr-own" }));
    env.clock.t += 30 * HOUR;
    expect(asks.lists().asks.map((e) => e.id)).toEqual([m2]);
    expect(asks.resolve(m2, undefined, { threadId: "thr-a" }, "req-3")).toMatchObject({ ok: true });
    const l = asks.lists();
    expect([...l.asks, ...l.lane]).toEqual([]);
    expect(wakesFor("thr-a").filter((w) => w.op === `resolve:${m2}:thr-a`)).toHaveLength(1);
  });

  it("resolve and progress from an unrelated thread are refused and change nothing", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    expect(asks.resolve(m, "x", { threadId: "thr-zzz" })).toMatchObject({ ok: false, status: 403 });
    expect(asks.resolve(m, "x", {})).toMatchObject({ ok: false, status: 403 });
    expect(asks.withdraw(m, { threadId: "thr-own" })).toMatchObject({ ok: false, status: 403 });
    expect(svc.store.decision(m)).toMatchObject({ resolved_at: null, withdrawn_at: null });
    expect(asks.resolve("nope", "x", { threadId: "thr-a" })).toMatchObject({ ok: false, status: 404 });
    const s = await fileId(stepsAsk("thr-a", "s1"));
    expect(asks.resolve(s, "x", { threadId: "thr-a" })).toMatchObject({ ok: false, status: 409 });
  });

  it("withdraw by the asker wakes the owner (whose notice was delivered) and every mentioner, not the asker", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    expect(await svc.file(machineAsk({ thread: "thr-c" }, { owner_thread: "thr-own" }), {})).toMatchObject({ mentioned: true });
    const owner = obs(m).find((o) => o.op === `owner:${m}`)!;
    const c = svc.store.claim(owner.id, 0);
    if (!c.ok) throw new Error("claim");
    svc.store.updateAttempt(owner.id, c.attempt, "delivered");
    svc.store.transition(owner.id, "sending", "done", c.attempt);
    expect(asks.withdraw(m, { threadId: "thr-a" }, "w-1")).toMatchObject({ ok: true, replay: false });
    expect(asks.withdraw(m, { threadId: "thr-a" }, "w-1")).toMatchObject({ ok: true, replay: true });
    expect(svc.store.decision(m)).toMatchObject({ withdrawn_at: expect.any(String) });
    const ownerWake = obs(m).find((o) => o.op === `withdraw:${m}:thr-own`)!;
    expect(ownerWake).toMatchObject({ recipient: "thr-own", after_id: owner.id });
    expect(svc.wakes().map((w) => w.op).sort()).toEqual([`withdraw:${m}:thr-c`, `withdraw:${m}:thr-own`]);
    expect(asks.progress(m, "x", { threadId: "thr-own" })).toMatchObject({ ok: false, status: 409 });
    expect(asks.lists().lane).toEqual([]);
  });

  it("recipients survive a restart", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    await svc.file(machineAsk({ thread: "thr-c" }, { owner_thread: "thr-own" }), {});
    const again = new Asks(env.open());
    expect(again.resolve(m, "done", { threadId: "thr-own" })).toMatchObject({ ok: true });
    expect(again.svc.wakes().map((w) => w.recipient).sort()).toEqual(["thr-a", "thr-c"]);
  });

  it("get reports resolved_at and withdrawn_at", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    expect(asks.get(m)).toMatchObject({ id: m, resolved_at: null, withdrawn_at: null, owner_thread: "thr-own" });
    asks.resolve(m, "ok", { threadId: "thr-own" });
    expect(asks.get(m)!.resolved_at).toBe(env.now());
    expect(asks.get("nope")).toBeUndefined();
  });
});

describe("the outbox [E-8]", () => {
  it("resolving before the drain voids the owner notice and gives each recipient one notice with its own payload", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    await svc.file(machineAsk({ thread: "thr-c" }, { owner_thread: "thr-own" }), {});
    const ownerRow = obs(m).find((o) => o.op === `owner:${m}`)!;
    asks.resolve(m, "runner replaced", { threadId: "thr-own" });
    expect(svc.store.obligation(ownerRow.id)!.voided_at).toBe(env.now());
    const due = svc.wakes();
    expect(due.map((w) => w.recipient).sort()).toEqual(["thr-a", "thr-c"]);
    expect(due.some((w) => w.op === `owner:${m}`)).toBe(false);
    expect(obs(m).some((o) => o.op === `resolve:${m}:thr-own`)).toBe(false);
    const pa = due.find((w) => w.recipient === "thr-a")!.payload!;
    const pc = due.find((w) => w.recipient === "thr-c")!.payload!;
    expect(pa).not.toBe(pc);
    for (const p of [pa, pc]) {
      expect(p).toContain(m);
      expect(p).toContain("runner replaced");
    }
    // a restart changes nothing
    const again = env.open();
    expect(obs(m, again).map((o) => [o.op, o.payload, o.voided_at])).toEqual(obs(m).map((o) => [o.op, o.payload, o.voided_at]));
  });

  it("the same with withdraw", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    await svc.file(machineAsk({ thread: "thr-c" }, { owner_thread: "thr-own" }), {});
    asks.withdraw(m, { threadId: "thr-a" });
    expect(svc.wakes().map((w) => w.op).sort()).toEqual([`withdraw:${m}:thr-c`]);
    expect(obs(m).find((o) => o.op === `owner:${m}`)!.voided_at).not.toBeNull();
  });

  it("an owner notice already done is followed by the resolve notice", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    const owner = obs(m).find((o) => o.op === `owner:${m}`)!;
    const c = svc.store.claim(owner.id, 0);
    if (!c.ok) throw new Error("claim");
    svc.store.updateAttempt(owner.id, c.attempt, "delivered");
    svc.store.transition(owner.id, "sending", "done", c.attempt);
    asks.resolve(m, "fixed", { threadId: "thr-a" });
    const r = obs(m).find((o) => o.op === `resolve:${m}:thr-own`)!;
    expect(r).toMatchObject({ recipient: "thr-own", after_id: owner.id });
    expect(svc.wakes().map((w) => w.op)).toContain(`resolve:${m}:thr-own`);
  });

  it("late owner delivery: uncertain owner notice, resolve sent first, then the owner notice arrives late [H-5]", async () => {
    const m = await fileId(machineAsk({}, { owner_thread: "thr-own" }));
    const owner = obs(m).find((o) => o.op === `owner:${m}`)!;
    const c = svc.store.claim(owner.id, 0);
    if (!c.ok) throw new Error("claim");
    svc.store.updateAttempt(owner.id, c.attempt, "uncertain");
    expect(svc.store.transition(owner.id, "sending", "uncertain", c.attempt)).toEqual({ ok: true });
    asks.resolve(m, "fixed", { threadId: "thr-a" });
    const r = obs(m).find((o) => o.op === `resolve:${m}:thr-own`)!;
    expect(r.after_id).toBe(owner.id);
    expect(svc.wakes().map((w) => w.op)).toContain(`resolve:${m}:thr-own`);
    // the owner notice is delivered late
    expect(svc.store.transition(owner.id, "uncertain", "done", c.attempt)).toEqual({ ok: true });
    expect(owner.payload).toContain(`bb home get --id ${m}`);
    expect(owner.payload).toContain("still open");
    expect(asks.get(m)).toMatchObject({ resolved_at: expect.any(String) });
  });
});

describe("card rows are outside the legacy lifecycle (Task 2.7, Q6)", () => {
  const REFUSED = { ok: false, status: 400, error: "card asks close through tasks" };

  it("refuses progress, resolve and withdraw on a card row, hides it from get, and leaves it untouched", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const a = new Asks(r.svc);
    const before = r.svc.store.decision(g1.id);
    expect(a.progress(g1.id, "x", { threadId: "thr_a" })).toEqual(REFUSED);
    expect(a.resolve(g1.id, "x", { threadId: "thr_a" })).toEqual(REFUSED);
    expect(a.withdraw(g1.id, { threadId: "thr_a" })).toEqual(REFUSED);
    expect(a.get(g1.id)).toBeUndefined();
    expect(r.svc.store.decision(g1.id)).toEqual(before);
    cleanupEnvs();
  });

  it("legacy rows keep working beside card rows", async () => {
    const m = await fileId(machineAsk());
    expect(asks.get(m)).toBeDefined();
    expect(asks.lists().asks.map((e) => e.id)).toContain(m);
  });
});
