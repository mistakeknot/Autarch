import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { framed, sdkAdapter, WakeLoop } from "../wakes.js";
import { ask, makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";
import { archived, econnrefused, FakeSdk, netError } from "./wakes-helpers.js";

let env: Env;
let svc: Service;
let sdk: FakeSdk;
let loop: WakeLoop;
let q = 0;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  sdk = new FakeSdk();
  loop = new WakeLoop(svc, sdk);
  q = 0;
});
afterEach(() => env.cleanup());

const rev = (id: string) => (svc.store.decision(id) as { revision: string }).revision;
/** A picked instruction ask for `thread`: returns the wake obligation. */
async function wake(thread = "thr-a", service: Service = svc) {
  const r = await service.file(ask(env, { thread, question: `Question number ${++q}?`, subject: `s${q}` }), {});
  if (!r.ok) throw new Error(r.error);
  const p = service.pick(r.decision_id, "project", rev(r.decision_id), `pick-${r.decision_id}`, "mk", "home");
  if (!p.ok) throw new Error(p.error);
  return service.store.obligationsFor(r.decision_id).find((o) => o.kind === "wake")!;
}
const row = (id: string) => svc.store.obligation(id)!;
const states = (id: string) => svc.store.attempts(id).map((a) => a.state);
const restart = () => {
  const s = env.open();
  svc = s;
  loop = new WakeLoop(s, sdk);
  return loop;
};
const advance = (ms: number) => (env.clock.t += ms);
const framedOf = (id: string, n: number) => framed(row(id).op ?? row(id).id, n, row(id).payload ?? "");
/** The service died after `threadId` queued the row but before its id was stored. */
function crashAfterQueue(id: string, opts: { cancel?: boolean } = {}) {
  const c = svc.store.claim(id, row(id).attempt);
  if (!c.ok) throw new Error("claim");
  const r = sdk.inject(row(id).recipient!, framedOf(id, c.attempt));
  if (opts.cancel) sdk.cancel(r.id);
  return { n: c.attempt, queued: r };
}

// A delegated wake, then mk overrides it: returns the wake and the void notice.
async function delegated(thread = "thr-a") {
  const dele = verifiedDelegation(svc);
  dele.setDelegation({ vizierThreadId: "thr-vizier", projects: ["Autarch"], dailyCap: 5 }, {});
  dele.markSeen("mk", dele.latestSettingsItem()!);
  const r = await svc.file(ask(env, { thread, question: `Question number ${++q}?`, subject: `s${q}` }), {});
  if (!r.ok) throw new Error(r.error);
  const ruled = dele.rule(r.decision_id, "project", "reversible and low risk", { threadId: "thr-vizier" });
  if (!ruled.ok) throw new Error(ruled.error);
  const w = svc.store.obligationsFor(r.decision_id).find((o) => o.kind === "wake")!;
  return { dele, id: r.decision_id, w, notice: () => svc.store.obligationsFor(r.decision_id).find((o) => o.kind === "void-notice") };
}

describe("wake sends", () => {
  it("sends exactly one message per row with the exact snapshotted payload and its op marker, queue-if-active", async () => {
    const a = await wake("thr-a");
    const b = await wake("thr-b");
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
    expect(sdk.sent[0]).toEqual({ threadId: "thr-a", input: `[home-msg ${a.op} #1] ${a.payload}`, mode: "queue-if-active" });
    expect(sdk.sent[1]).toEqual({ threadId: "thr-b", input: `[home-msg ${b.op} #1] ${b.payload}`, mode: "queue-if-active" });
    expect(row(a.id).state).toBe("done");
    expect(svc.store.attempts(a.id)[0]).toMatchObject({ state: "delivered", evidence: "m1" });
  });

  it("a wake added during a send stays pending until the next drain", async () => {
    const x = await wake("thr-a");
    const open = sdk.block();
    const p = loop.drain();
    await Promise.resolve();
    const late = await wake("thr-b");
    open();
    await p;
    expect(row(x.id).state).toBe("done");
    expect(row(late.id).state).toBe("pending");
    expect(sdk.sent).toHaveLength(1);
    await loop.drain();
    expect(row(late.id).state).toBe("done");
  });

  it("overlapping nudges serialize the drains and never double-send within an attempt", async () => {
    const a = await wake("thr-a");
    const b = await wake("thr-b");
    const open = sdk.block();
    const ps = [loop.nudge(), loop.nudge(), loop.nudge()];
    await Promise.resolve();
    open();
    await Promise.all(ps);
    expect(sdk.maxRunning).toBe(1);
    expect(sdk.sent).toHaveLength(2);
    expect([row(a.id).state, row(b.id).state]).toEqual(["done", "done"]);
  });

  it("a late failure after done changes nothing", async () => {
    const a = await wake("thr-a");
    sdk.fallback = { kind: "queue" };
    await loop.drain();
    const handle = svc.store.attempts(a.id)[0]!.handle!;
    sdk.dispatch(handle);
    await loop.onQueueEvent({ type: "dispatched", id: handle, threadId: "thr-a" });
    expect(row(a.id).state).toBe("done");
    await loop.onQueueEvent({ type: "cancelled", id: handle, threadId: "thr-a" });
    await loop.onThreadGone("thr-a");
    expect(row(a.id).state).toBe("done");
    expect(states(a.id)).toEqual(["delivered"]);
  });
});

describe("no batching [E-4]", () => {
  async function two() {
    sdk.fallback = { kind: "queue" };
    const a = await wake("thr-a");
    const b = await wake("thr-a");
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
    return { a, b, qa: svc.store.attempts(a.id)[0]!.handle!, qb: svc.store.attempts(b.id)[0]!.handle! };
  }

  it("overriding A removes only A's row; B dispatches and ends done, across a restart", async () => {
    const { a, b, qa, qb } = await two();
    svc.store.voidObligation(a.id);
    await loop.reconcile();
    expect(sdk.deletes).toEqual([qa]);
    expect(states(a.id)).toEqual(["not-delivered"]);
    sdk.dispatch(qb);
    restart();
    await loop.reconcile();
    expect(row(b.id).state).toBe("done");
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
  });

  it("a dispatch racing the removal leaves A delivered, not lost", async () => {
    const { a, qa } = await two();
    svc.store.voidObligation(a.id);
    sdk.dispatch(qa);
    await loop.reconcile();
    expect(row(a.id).state).toBe("done");
    expect(states(a.id)).toEqual(["delivered"]);
  });
});

describe("crash before binding [E-5]", () => {
  it("a cancelled row is uncertain, and never resent until Resend", async () => {
    const a = await wake();
    crashAfterQueue(a.id, { cancel: true });
    restart();
    await loop.start(AbortSignal.abort());
    expect(row(a.id).state).toBe("uncertain");
    await loop.drain();
    expect(sdk.sent).toHaveLength(0);
    expect(loop.resend(a.id, 1, "click-1")).toMatchObject({ ok: true });
    await loop.drain();
    expect(sdk.sent).toHaveLength(1);
  });

  it("a still-queued row is bound and ends done on dispatch", async () => {
    const a = await wake();
    const { queued } = crashAfterQueue(a.id);
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("queued");
    expect(svc.store.attempts(a.id)[0]!.handle).toBe(queued.id);
    sdk.dispatch(queued.id);
    await loop.onQueueEvent({ type: "dispatched", id: queued.id, threadId: "thr-a" });
    expect(row(a.id).state).toBe("done");
    expect(sdk.sent).toHaveLength(0);
  });
});

describe("ambiguous failure, no crash [F-1]", () => {
  it("queues Q, throws a network error; mk cancels Q: not-delivered, undeliverable, never resent", async () => {
    const a = await wake();
    sdk.script = [{ kind: "queue-throw", err: netError() }];
    await loop.drain();
    expect(row(a.id).state).toBe("queued");
    const handle = svc.store.attempts(a.id)[0]!.handle!;
    sdk.cancel(handle);
    await loop.reconcile();
    expect(states(a.id)).toEqual(["not-delivered"]);
    expect(row(a.id).state).toBe("undeliverable");
    await loop.drain();
    expect(sdk.sent).toHaveLength(1);
  });

  it("with (c) and (d) unavailable the row is uncertain and nothing is sent until Resend", async () => {
    const a = await wake();
    sdk.caps = { state: false, list: false, del: false, events: false };
    sdk.script = [{ kind: "throw", err: netError() }];
    await loop.drain();
    expect(row(a.id).state).toBe("uncertain");
    await loop.drain();
    await loop.reconcile();
    expect(sdk.sent).toHaveLength(1);
    expect(loop.resend(a.id, 1, "c1")).toMatchObject({ ok: true });
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
  });

  it("a proven pre-acceptance rejection returns the row to pending and the retry sends once", async () => {
    const a = await wake();
    sdk.script = [{ kind: "throw", err: econnrefused() }];
    await loop.drain();
    expect(row(a.id)).toMatchObject({ state: "pending" });
    expect(row(a.id).last_error).toContain("ECONNREFUSED");
    expect(states(a.id)).toEqual(["not-delivered"]);
    await loop.drain();
    expect(sdk.sent).toHaveLength(1);
    advance(24 * 3600_000);
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
    expect(row(a.id).state).toBe("done");
  });

  it("an archived thread rejection is not-delivered and undeliverable", async () => {
    const a = await wake();
    sdk.script = [{ kind: "throw", err: archived() }];
    await loop.drain();
    expect(row(a.id).state).toBe("undeliverable");
    expect(states(a.id)).toEqual(["not-delivered"]);
  });
});

describe("delivery evidence [F-4]", () => {
  it("a diagnostic message quoting the marker does not complete the attempt", async () => {
    const a = await wake();
    crashAfterQueue(a.id, { cancel: true });
    sdk.accepted.push({ id: "diag", threadId: "thr-a", text: `see the log: [home-msg ${a.op} #1] was logged` });
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("uncertain");
  });

  it("an unbound queued row whose text was edited but kept the marker leaves it uncertain", async () => {
    const a = await wake();
    const c = svc.store.claim(a.id, 0);
    if (!c.ok) throw new Error("claim");
    sdk.inject("thr-a", `${framedOf(a.id, 1)}\nplus mk's edit`);
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("uncertain");
    expect(svc.store.attempts(a.id)[0]!.handle).toBeNull();
  });

  it("two accepted events with the exact framed input leave it uncertain; exactly one completes it", async () => {
    const a = await wake();
    const c = svc.store.claim(a.id, 0);
    if (!c.ok) throw new Error("claim");
    sdk.accepted.push({ id: "e1", threadId: "thr-a", text: framedOf(a.id, 1) }, { id: "e2", threadId: "thr-a", text: framedOf(a.id, 1) });
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("uncertain");
    sdk.accepted.pop();
    await loop.reconcile();
    expect(row(a.id).state).toBe("done");
    expect(svc.store.attempts(a.id)[0]).toMatchObject({ state: "delivered", evidence: "e1" });
  });
});

describe("resend through the real drain [G-1] [F-2]", () => {
  async function uncertain() {
    const a = await wake();
    sdk.caps = { state: false, list: false, del: false, events: false };
    sdk.script = [{ kind: "throw", err: netError() }];
    await loop.drain();
    expect(row(a.id).state).toBe("uncertain");
    return a;
  }

  it("sends attempt 2 exactly once", async () => {
    const a = await uncertain();
    expect(loop.resend(a.id, 1, "c1")).toMatchObject({ ok: true, replay: false });
    await loop.drain();
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
    expect(sdk.sent[1]!.input).toContain("#2]");
    expect(row(a.id).state).toBe("done");
  });

  it("a restart after Resend but before the drain still gives exactly one attempt 2", async () => {
    const a = await uncertain();
    loop.resend(a.id, 1, "c1");
    restart();
    await loop.start(AbortSignal.abort());
    expect(row(a.id)).toMatchObject({ state: "pending", resend_permit: 1 });
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
  });

  it("a restart after the claim but before the send ends uncertain and never makes a third attempt", async () => {
    const a = await uncertain();
    loop.resend(a.id, 1, "c1");
    expect(svc.store.claim(a.id, row(a.id).attempt)).toMatchObject({ ok: true, attempt: 2 });
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("uncertain");
    expect(states(a.id)).toEqual(["uncertain", "uncertain"]);
    await loop.drain();
    expect(sdk.sent).toHaveLength(1);
  });

  it("a repeated Resend while the row is pending is refused", async () => {
    const a = await uncertain();
    expect(loop.resend(a.id, 1, "c1")).toMatchObject({ ok: true });
    expect(loop.resend(a.id, 1, "c2")).toEqual({ ok: false, reason: "stale" });
  });

  it("attempt 1 later reconciling delivered completes the row and removes a queued attempt 2", async () => {
    const a = await uncertain();
    sdk.caps = { state: true, list: true, del: true, events: true };
    loop.resend(a.id, 1, "c1");
    sdk.fallback = { kind: "queue" };
    await loop.drain();
    expect(row(a.id).state).toBe("queued");
    const q2 = svc.store.attempts(a.id)[1]!.handle!;
    sdk.accepted.push({ id: "late", threadId: "thr-a", text: framedOf(a.id, 1) });
    await loop.reconcile();
    expect(row(a.id).state).toBe("done");
    expect(states(a.id)).toEqual(["delivered", "not-delivered"]);
    expect(sdk.deletes).toEqual([q2]);
  });

  it("[H-1] startup reconciliation, and one while another send blocks the drain, keep the Resent row pending with its permit", async () => {
    const a = await uncertain();
    loop.resend(a.id, 1, "c1");
    await loop.reconcile();
    expect(row(a.id)).toMatchObject({ state: "pending", resend_permit: 1 });
    const other = await wake("thr-b");
    const open = sdk.block();
    const p = loop.drain();
    await Promise.resolve();
    await loop.reconcile();
    expect(row(a.id)).toMatchObject({ state: "pending", resend_permit: 1 });
    open();
    await p;
    expect(row(other.id).state).toBe("done");
    expect(sdk.sent.filter((s) => s.input.includes(`${a.op} #2]`))).toHaveLength(1);
  });

  it("[I-1] a lost Resend response then a retry with the same click id: no permit, no attempt 3; a new click is stale", async () => {
    const a = await uncertain();
    loop.resend(a.id, 1, "c1");
    sdk.script = [{ kind: "throw", err: netError() }];
    await loop.drain();
    expect(row(a.id)).toMatchObject({ state: "uncertain", attempt: 2, resend_permit: 0 });
    expect(loop.resend(a.id, 1, "c1")).toMatchObject({ ok: true, replay: true });
    restart();
    expect(loop.resend(a.id, 1, "c1")).toMatchObject({ ok: true, replay: true });
    expect(row(a.id)).toMatchObject({ state: "uncertain", resend_permit: 0 });
    await loop.drain();
    expect(sdk.sent).toHaveLength(2);
    expect(loop.resend(a.id, 1, "c9")).toEqual({ ok: false, reason: "stale" });
  });

  it("Resend on a voided row is refused", async () => {
    const a = await uncertain();
    svc.store.voidObligation(a.id);
    expect(loop.resend(a.id, 1, "c1")).toEqual({ ok: false, reason: "stale" });
  });
});

describe("stale selection [H-2]", () => {
  it("an override voids W while X's send blocks: W's claim fails and nothing is sent for it", async () => {
    const x = await wake("thr-a");
    const { dele, id, w } = await delegated("thr-b");
    const open = sdk.block();
    const p = loop.drain();
    await Promise.resolve();
    expect(dele.override(id, {})).toMatchObject({ ok: true });
    open();
    await p;
    expect(row(x.id).state).toBe("done");
    expect(sdk.sent.some((s) => s.input.includes(String(w.op)))).toBe(false);
    expect(svc.store.attempts(w.id)).toHaveLength(0);
  });

  it("the same for a voided owner notice O", async () => {
    const x = await wake("thr-a");
    const o = await wake("thr-b");
    const open = sdk.block();
    const p = loop.drain();
    await Promise.resolve();
    svc.store.voidObligation(o.id);
    open();
    await p;
    expect(row(x.id).state).toBe("done");
    expect(svc.store.attempts(o.id)).toHaveLength(0);
  });
});

describe("queued rows and events [D-9]", () => {
  async function queued(thread = "thr-a") {
    const a = await wake(thread);
    sdk.fallback = { kind: "queue" };
    await loop.drain();
    expect(row(a.id)).toMatchObject({ state: "queued" });
    return { a, handle: svc.store.attempts(a.id)[0]!.handle! };
  }

  it("queued then message.dispatched is done", async () => {
    const { a, handle } = await queued();
    sdk.dispatch(handle);
    await loop.onQueueEvent({ type: "dispatched", id: handle, threadId: "thr-a" });
    expect(row(a.id).state).toBe("done");
  });

  it("queued then thread.archived is undeliverable", async () => {
    const { a } = await queued();
    await loop.onThreadGone("thr-a");
    expect(states(a.id)).toEqual(["not-delivered"]);
    expect(row(a.id).state).toBe("undeliverable");
  });

  it("queued then message.cancelled is undeliverable", async () => {
    const { a, handle } = await queued();
    sdk.cancel(handle);
    await loop.onQueueEvent({ type: "cancelled", id: handle, threadId: "thr-a" });
    expect(row(a.id).state).toBe("undeliverable");
  });

  it("restart while queued, then the row dispatches: done", async () => {
    const { a, handle } = await queued();
    restart();
    await loop.start(AbortSignal.abort());
    expect(row(a.id).state).toBe("queued");
    sdk.dispatch(handle);
    await loop.onQueueEvent({ type: "dispatched", id: handle, threadId: "thr-a" });
    expect(row(a.id).state).toBe("done");
  });

  it("an event that arrives before the send resolves is not lost", async () => {
    const a = await wake();
    sdk.script = [
      {
        kind: "queue",
        before: async (r) => {
          sdk.dispatch(r.id);
          await loop.onQueueEvent({ type: "dispatched", id: r.id, threadId: "thr-a", text: r.text });
        },
      },
    ];
    await loop.drain();
    expect(row(a.id).state).toBe("done");
    expect(states(a.id)).toEqual(["delivered"]);
  });

  it("cancelled during downtime: undeliverable with the row read; uncertain without it and with no marker", async () => {
    const { a, handle } = await queued();
    sdk.cancel(handle);
    restart();
    await loop.reconcile();
    expect(row(a.id).state).toBe("undeliverable");

    const b = await wake("thr-b");
    sdk.fallback = { kind: "queue" };
    await loop.drain();
    const hb = svc.store.attempts(b.id)[0]!.handle!;
    sdk.cancel(hb);
    sdk.caps.state = false;
    sdk.caps.events = false;
    restart();
    await loop.reconcile();
    expect(row(b.id).state).toBe("uncertain");
    const before = sdk.sent.length;
    await loop.drain();
    expect(sdk.sent).toHaveLength(before);
  });

  it("periodic reconciliation moves a still-queued row to done once the fake dispatches it without an event", async () => {
    const { a, handle } = await queued();
    sdk.dispatch(handle);
    const wait: number[] = [];
    const ac = new AbortController();
    await loop.start(ac.signal, async (ms) => {
      wait.push(ms);
      if (wait.length > 1) ac.abort();
    });
    expect(wait[0]).toBe(60_000);
    expect(row(a.id).state).toBe("done");
  });
});

describe("override interleavings, live [D-3] [G-2]", () => {
  const noticeSends = () => sdk.sent.filter((s) => s.input.includes("[home-msg void:"));

  it("(a) the send blocks, the override lands, the send succeeds: delivered, done with voided_at, one notice", async () => {
    const { dele, id, w } = await delegated();
    const open = sdk.block();
    const p = loop.drain();
    await Promise.resolve();
    dele.override(id, {});
    open();
    await p;
    await loop.drain();
    expect(row(w.id)).toMatchObject({ state: "done" });
    expect(row(w.id).voided_at).not.toBeNull();
    expect(states(w.id)).toEqual(["delivered"]);
    expect(noticeSends()).toHaveLength(1);
  });

  it("(b) W is queued, the override lands: the queued attempt is removed and W is never sent again; one notice", async () => {
    sdk.fallback = { kind: "queue" };
    const { dele, id, w } = await delegated();
    await loop.drain();
    const handle = svc.store.attempts(w.id)[0]!.handle!;
    dele.override(id, {});
    await loop.drain();
    expect(sdk.deletes).toEqual([handle]);
    expect(states(w.id)).toEqual(["not-delivered"]);
    await loop.drain();
    expect(sdk.sent.filter((s) => s.input.includes(String(w.op)))).toHaveLength(1);
    expect(noticeSends()).toHaveLength(1);
  });

  it("(c) the asker already acted: one notice, and a restart sends no second one", async () => {
    const { dele, id, w } = await delegated();
    await loop.drain();
    expect(row(w.id).state).toBe("done");
    dele.override(id, {});
    await loop.drain();
    restart();
    await loop.start(AbortSignal.abort());
    await loop.drain();
    expect(noticeSends()).toHaveLength(1);
  });

  it("(d) a crash just before and just after the removal call: still one notice", async () => {
    sdk.fallback = { kind: "queue" };
    const { dele, id, w } = await delegated();
    await loop.drain();
    const handle = svc.store.attempts(w.id)[0]!.handle!;
    dele.override(id, {});
    // just after the removal: the row is gone in the fake, the store never heard
    sdk.cancel(handle);
    restart();
    await loop.drain();
    expect(states(w.id)).toEqual(["not-delivered"]);
    expect(noticeSends()).toHaveLength(1);
  });

  it("(d') a crash just before the removal: after restart the row is removed by handle", async () => {
    sdk.fallback = { kind: "queue" };
    const { dele, id, w } = await delegated();
    await loop.drain();
    const handle = svc.store.attempts(w.id)[0]!.handle!;
    dele.override(id, {});
    restart();
    await loop.drain();
    expect(sdk.deletes).toEqual([handle]);
    expect(noticeSends()).toHaveLength(1);
  });
});

describe("sdkAdapter against the real threads.send contract", () => {
  it("sends input as an array of text parts, not a string (bb rejects strings; found by the real-bb run)", async () => {
    const seen: unknown[] = [];
    const threads = {
      send: async (a: { input: unknown }) => {
        seen.push(a.input);
        return { delivery: "sent" };
      },
      queuedMessages: { list: async () => [], delete: async () => undefined },
      events: { list: async () => [] },
    };
    await sdkAdapter(threads as never).send({ threadId: "thr_x", input: "hello", mode: "queue-if-active" });
    expect(seen).toEqual([[{ type: "text", text: "hello" }]]);
  });
});
