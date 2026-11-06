// The blocker phone push (bead mk-tv70n.7): the urgency field, the once-per-ask notifier call, the push columns and the
// time-to-answer read. The notifier is a fake; the RPC wrapper is tested against a stubbed callRpc.
import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { join } from "node:path";
import { CODE_VERSION, MIGRATIONS, migrate } from "../migrations.js";
import { identity, normalizedJson, parseAsk, revision } from "../model.js";
import { PUSH_METHOD, PUSH_PLUGIN_ID, RpcNotifier, type Notifier, type PushRequest, type PushResult } from "../notifier.js";
import type { Service } from "../service.js";
import { timeToAnswer } from "../pushtiming.js";
import { Store } from "../store.js";
import { decision, tmpDir } from "./helpers.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { ask, makeEnv, T0, type Env } from "./service-helpers.js";

class FakeNotifier implements Notifier {
  calls: PushRequest[] = [];
  mode: "ok" | "throw" | "hang" | "capped" | "deduped" | "garbage" = "ok";
  async notify(req: PushRequest): Promise<PushResult> {
    this.calls.push(req);
    if (this.mode === "throw") throw new Error("push-notifications is not installed");
    if (this.mode === "hang") return new Promise<PushResult>(() => {});
    if (this.mode === "garbage") return {} as PushResult;
    const base = { id: `push-${this.calls.length}`, at: "2026-09-26T14:03:05.000Z" };
    return this.mode === "capped" ? { ...base, capped: true } : this.mode === "deduped" ? { ...base, deduped: true } : base;
  }
}

describe("urgency on the ask", () => {
  let env: Env;
  beforeEach(() => void (env = makeEnv(["Autarch"])));
  afterEach(() => env.cleanup());
  const ok = (over: Record<string, unknown>) => {
    try {
      return parseAsk(ask(env, over));
    } catch {
      return undefined;
    }
  };

  it("accepts blocker, treats absence as unset, rejects any other value", () => {
    expect(ok({ urgency: "blocker" })?.urgency).toBe("blocker");
    expect(ok({})?.urgency).toBeUndefined();
    for (const bad of ["high", "Blocker", "", 1, true]) expect(ok({ urgency: bad })).toBeUndefined();
  });

  it("is not part of the identity, revision or normalized body", () => {
    const a = ok({ urgency: "blocker" })!;
    const b = ok({})!;
    expect(identity(a)).toBe(identity(b));
    expect(revision(a)).toBe(revision(b));
    expect(normalizedJson(a)).toBe(normalizedJson(b));
  });
});

describe("the push on a legacy ask", () => {
  let env: Env;
  let n: FakeNotifier;
  let logs: string[];
  const open = (over = {}) => env.open({ notifier: n, log: (m: string) => void logs.push(m), pushTimeoutMs: 40, ...over });
  beforeEach(() => {
    env = makeEnv(["Autarch"]);
    n = new FakeNotifier();
    logs = [];
  });
  afterEach(() => env.cleanup());
  const file = async (svc: Service, over: Record<string, unknown> = {}) => {
    const r = await svc.file(ask(env, over), {});
    if (!r.ok) throw new Error(r.error);
    return r.decision_id;
  };
  const row = (svc: Service, id: string) => svc.store.db.prepare("SELECT urgency, push_sent_at, push_id, push_capped FROM decisions WHERE id = ?").get(id) as Record<string, unknown>;
  const machine = (over: Record<string, unknown> = {}) => ({ kind: "machine", options: undefined, machine: { class: "ci", detail: "runner is down" }, subject: "ci: runner down", question: "the CI runner is down", ...over });
  const steps = (over: Record<string, unknown> = {}) => ({ kind: "steps", options: undefined, steps: ["do a"], subject: "s", question: "steps", ...over });

  it("pushes once for a decide ask with urgency blocker and records the result", async () => {
    const svc = open();
    const id = await file(svc, { urgency: "blocker" });
    await svc.drainPushes();
    expect(n.calls).toHaveLength(1);
    expect(n.calls[0]).toMatchObject({ kind: "blocker", dedupeKey: id });
    expect(n.calls[0]!.body).toContain("Collapse routine catch-up");
    expect(row(svc, id)).toEqual({ urgency: "blocker", push_sent_at: "2026-09-26T14:03:05.000Z", push_id: "push-1", push_capped: 0 });
  });

  it("does not push without urgency, on any kind", async () => {
    const svc = open();
    await file(svc, {});
    await file(svc, machine());
    await file(svc, steps());
    await svc.drainPushes();
    expect(n.calls).toEqual([]);
  });

  it("a machine ask without urgency never pushes, with urgency blocker it does", async () => {
    const svc = open();
    const a = await file(svc, machine());
    const b = await file(svc, machine({ thread: "thr-b", subject: "ci: other", question: "the other CI runner is down" , urgency: "blocker" }));
    const c = await file(svc, steps({ urgency: "blocker" }));
    await svc.drainPushes();
    expect(n.calls).toHaveLength(2);
    expect(row(svc, a).push_sent_at).toBeNull();
    expect(row(svc, b).push_id).not.toBeNull();
    expect(row(svc, c).push_id).not.toBeNull();
  });

  it("an ask refiled with the same request does not push again", async () => {
    const svc = open();
    const req = { urgency: "blocker", request_id: "req-1" };
    const a = await file(svc, req);
    await svc.drainPushes();
    const b = await file(svc, req);
    await svc.drainPushes();
    expect(b).toBe(a);
    expect(n.calls).toHaveLength(1);
  });

  it("a notifier that throws is swallowed and logged; the ask is filed and a later refile retries", async () => {
    const svc = open();
    n.mode = "throw";
    const id = await file(svc, { urgency: "blocker", request_id: "req-1" });
    await svc.drainPushes();
    expect(row(svc, id).push_sent_at).toBeNull();
    expect(logs.join("\n")).toContain("not installed");
    n.mode = "ok";
    await file(svc, { urgency: "blocker", request_id: "req-1" });
    await svc.drainPushes();
    expect(n.calls).toHaveLength(2);
  });

  it("a notifier that hangs times out without delaying the filing", async () => {
    const svc = open();
    n.mode = "hang";
    const t0 = Date.now();
    const id = await file(svc, { urgency: "blocker" });
    expect(Date.now() - t0).toBeLessThan(35);
    await svc.drainPushes();
    expect(logs.join("\n")).toContain("timed out");
    expect(row(svc, id).push_sent_at).toBeNull();
  });

  it("a result with no id is a failure; a capped result is a success that records push_capped", async () => {
    const svc = open();
    n.mode = "garbage";
    const g = await file(svc, { urgency: "blocker" });
    await svc.drainPushes();
    expect(row(svc, g).push_sent_at).toBeNull();
    n.mode = "capped";
    const c = await file(svc, { urgency: "blocker", subject: "other", question: "Another question?" });
    await svc.drainPushes();
    expect(row(svc, c)).toMatchObject({ push_id: "push-2", push_capped: 1 });
    n.mode = "deduped";
    const d = await file(svc, { urgency: "blocker", subject: "third", question: "A third question?" });
    await svc.drainPushes();
    expect(row(svc, d)).toMatchObject({ push_capped: 0 });
    expect(row(svc, d).push_id).toBe("push-3");
  });

  it("a notifier that throws synchronously cannot fail the filing", async () => {
    const svc = open({ notifier: { notify: () => { throw new Error("sync boom"); } } });
    const id = await file(svc, { urgency: "blocker" });
    await svc.drainPushes();
    expect(row(svc, id).push_sent_at).toBeNull();
    expect(logs.join("\n")).toContain("sync boom");
  });

  it("with no notifier configured a blocker ask files and nothing happens", async () => {
    const svc = env.open();
    const id = await file(svc, { urgency: "blocker" });
    expect(row(svc, id).urgency).toBe("blocker");
    expect(row(svc, id).push_sent_at).toBeNull();
  });
});

describe("the push on a card", () => {
  afterEach(cleanupEnvs);
  const setup = () => {
    const n = new FakeNotifier();
    const logs: string[] = [];
    const r = rig({ svc: { notifier: n, log: (m: string) => void logs.push(m), pushTimeoutMs: 40 } });
    return { r, n, logs };
  };

  it("pushes once when a card with urgency blocker is filed, with the card key as the dedupe key", async () => {
    const { r, n } = setup();
    const { t, g1 } = await opened(r, { ask: { urgency: "blocker" } });
    await r.svc.drainPushes();
    expect(n.calls).toHaveLength(1);
    expect(n.calls[0]).toMatchObject({ kind: "blocker", dedupeKey: r.svc.cardKey(t.id) });
    expect(r.db.prepare("SELECT urgency, push_id, push_capped FROM decisions WHERE id = ?").get(g1.id)).toEqual({ urgency: "blocker", push_id: "push-1", push_capped: 0 });
    await r.poll();
    await r.svc.drainPushes();
    expect(n.calls).toHaveLength(1);
  });

  it("a card without urgency does not push", async () => {
    const { r, n } = setup();
    await opened(r, {});
    await r.svc.drainPushes();
    expect(n.calls).toEqual([]);
  });

  it("an edit of a pushed card (new generation) does not push again", async () => {
    const { r, n } = setup();
    const { t } = await opened(r, { ask: { urgency: "blocker" } });
    await r.svc.drainPushes();
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), question: "A rewritten question?", ask: { urgency: "blocker" } }) });
    await r.poll();
    await r.svc.drainPushes();
    expect(r.gens(t.id).length).toBeGreaterThan(1);
    expect(n.calls).toHaveLength(1);
  });

  it("an edit after a failed first push tries again", async () => {
    const { r, n } = setup();
    n.mode = "throw";
    const { t } = await opened(r, { ask: { urgency: "blocker" } });
    await r.svc.drainPushes();
    expect(n.calls).toHaveLength(1);
    n.mode = "ok";
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), question: "A rewritten question?", ask: { urgency: "blocker" } }) });
    await r.poll();
    await r.svc.drainPushes();
    expect(n.calls).toHaveLength(2);
    expect(r.gens(t.id).some((g) => g.push_id === "push-2")).toBe(true);
  });

  it("a notifier failure never fails the card's materialization", async () => {
    const { r, n, logs } = setup();
    n.mode = "throw";
    const { t } = await opened(r, { ask: { urgency: "blocker" } });
    await r.svc.drainPushes();
    expect(r.cardRow(t.id).state).toBe("open");
    expect(logs.join("\n")).toContain("not installed");
  });

  it("a card with an invalid urgency is not materialized as a decision", async () => {
    const { r, n } = setup();
    const t = r.card({ ask: { urgency: "high" } });
    await r.poll();
    await r.svc.drainPushes();
    expect(r.gens(t.id)).toHaveLength(0);
    expect(n.calls).toEqual([]);
  });
});

describe("time to answer", () => {
  afterEach(cleanupEnvs);

  it("a decide card: filed, pushed, first pick, and the differences", async () => {
    const n = new FakeNotifier();
    const r = rig({ svc: { notifier: n } });
    const { t, g1 } = await opened(r, { ask: { urgency: "blocker" } });
    await r.svc.drainPushes();
    r.advance(90_000);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
    const tt = r.svc.timeToAnswer(g1.id)!;
    expect(tt).toMatchObject({ push_id: "push-1", push_capped: false, answered_by: "pick" });
    expect(tt.answered_at).toBe(r.svc.store.pick(g1.id)!.picked_at);
    expect(tt.filed_to_answer_ms).toBe(Date.parse(tt.answered_at!) - Date.parse(tt.filed_at));
    expect(tt.push_to_answer_ms).toBe(Date.parse(tt.answered_at!) - Date.parse("2026-09-26T14:03:05.000Z"));
    expect(r.svc.pushes()).toHaveLength(1);
    expect(r.svc.pushes()[0]).toMatchObject({ decision_id: g1.id, card_key: r.svc.cardKey(t.id), kind: "decide" });
  });

  it("an unanswered ask has no answer time", async () => {
    const r = rig({ svc: { notifier: new FakeNotifier() } });
    const { g1 } = await opened(r, { ask: { urgency: "blocker" } });
    expect(r.svc.timeToAnswer(g1.id)).toMatchObject({ answered_at: null, answered_by: null, filed_to_answer_ms: null, push_to_answer_ms: null });
  });

  it("a machine ask is answered when it is resolved", async () => {
    const env = makeEnv(["Autarch"]);
    try {
      const n = new FakeNotifier();
      const svc = env.open({ notifier: n });
      const asks = new Asks(svc);
      const f = await svc.file(ask(env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "down", owner_thread: "thr-own" }, subject: "ci", question: "CI down", urgency: "blocker" }), {});
      if (!f.ok) throw new Error(f.error);
      await svc.drainPushes();
      env.clock.t += 120_000;
      expect(asks.resolve(f.decision_id, "fixed", { threadId: "thr-own" })).toMatchObject({ ok: true });
      const tt = svc.timeToAnswer(f.decision_id)!;
      expect(tt.answered_by).toBe("resolved");
      expect(tt.filed_to_answer_ms).toBe(120_000);
      expect(tt.push_to_answer_ms).not.toBeNull();
    } finally {
      env.cleanup();
    }
  });

  it("an unknown decision has no time to answer; a card without urgency lists in no push listing", async () => {
    const r = rig({ svc: { notifier: new FakeNotifier() } });
    expect(r.svc.timeToAnswer("nope")).toBeUndefined();
    await opened(r, {});
    expect(r.svc.pushes()).toEqual([]);
  });
});

describe("the migration of an old database", () => {
  let dir: ReturnType<typeof tmpDir>;
  beforeEach(() => void (dir = tmpDir()));
  afterEach(() => dir.cleanup());

  it("v5 to v6 adds four nullable columns; an old decision row reads as unset", () => {
    const file = join(dir.dir, "data.db");
    const old = new Database(file);
    migrate(old, { codeVersion: 5, migrations: MIGRATIONS.filter((m) => m.version <= 5) });
    expect(old.prepare("SELECT COUNT(*) AS n FROM pragma_table_info('decisions') WHERE name IN ('urgency','push_sent_at','push_id','push_capped')").get()).toEqual({ n: 0 });
    const st = new Store(old, { now: () => "2026-09-26T14:03:00.000Z" });
    st.insertDecision(decision({ id: "old-1" }));
    old.close();

    const db = new Database(file);
    expect(CODE_VERSION).toBe(6);
    expect(migrate(db)).toMatchObject({ schemaVersion: 6 });
    const cols = db.prepare("SELECT name, \"notnull\" AS nn FROM pragma_table_info('decisions') WHERE name IN ('urgency','push_sent_at','push_id','push_capped') ORDER BY name").all();
    expect(cols).toEqual([
      { name: "push_capped", nn: 0 },
      { name: "push_id", nn: 0 },
      { name: "push_sent_at", nn: 0 },
      { name: "urgency", nn: 0 },
    ]);
    expect(db.prepare("SELECT urgency, push_sent_at, push_id, push_capped FROM decisions WHERE id = 'old-1'").get()).toEqual({ urgency: null, push_sent_at: null, push_id: null, push_capped: null });
    // The old row has no push and no answer: its time to answer reads without error.
    expect(timeToAnswer(db, "old-1")).toMatchObject({ push_sent_at: null, push_capped: null, answered_at: null });
    db.close();
  });
});

describe("RpcNotifier over a stubbed callRpc", () => {
  const stub = (result: unknown | Error) => {
    const calls: Record<string, unknown>[] = [];
    const plugins = {
      callRpc: async (o: Record<string, unknown>) => {
        calls.push(o);
        if (result instanceof Error) throw result;
        return result;
      },
    };
    return { calls, plugins: plugins as never };
  };

  it("calls push-notifications notifications.notify with no source and maps the thread to threadId", async () => {
    const { calls, plugins } = stub({ id: "n1", at: "2026-11-06T00:00:00Z", day: "2026-11-06", source: "autarch", outcome: "sent", capped: false, deduped: false, cap: 5, sentToday: 1, delivery: {} });
    const res = await new RpcNotifier(plugins).notify({ kind: "blocker", title: "T", body: "B", thread: "thr_x", dedupeKey: "AUTA-1" });
    expect(res).toEqual({ id: "n1", at: "2026-11-06T00:00:00Z" });
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({ pluginId: PUSH_PLUGIN_ID, method: PUSH_METHOD, input: { kind: "blocker", title: "T", body: "B", threadId: "thr_x", dedupeKey: "AUTA-1" } });
    expect(calls[0]!.signal).toBeInstanceOf(AbortSignal);
    expect(Object.keys(calls[0]!.input as object)).not.toContain("source");
    expect(Object.keys(calls[0]!.input as object)).not.toContain("thread");
    expect(Object.keys(calls[0]!.input as object).sort()).toEqual(["body", "dedupeKey", "kind", "threadId", "title"]);
  });

  it("sends no url, and no threadId when the card has no asking thread", async () => {
    const { calls, plugins } = stub({ id: "n1", at: "t" });
    await new RpcNotifier(plugins).notify({ kind: "blocker", title: "T", body: "B", dedupeKey: "AUTA-2", url: "https://x.example/c" } as never);
    expect(Object.keys(calls[0]!.input as object).sort()).toEqual(["body", "dedupeKey", "kind", "title"]);
  });

  it("passes capped and deduped through as successes", async () => {
    expect(await new RpcNotifier(stub({ id: "n", at: "t", capped: true }).plugins).notify({ kind: "blocker", title: "T", body: "B", dedupeKey: "k" })).toEqual({ id: "n", at: "t", capped: true });
    expect(await new RpcNotifier(stub({ id: "n", at: "t", deduped: true }).plugins).notify({ kind: "blocker", title: "T", body: "B", dedupeKey: "k" })).toEqual({ id: "n", at: "t", deduped: true });
  });

  it("throws on an RPC error and on a malformed result", async () => {
    const req: PushRequest = { kind: "blocker", title: "T", body: "B", dedupeKey: "k" };
    await expect(new RpcNotifier(stub(new Error("plugin push-notifications not found")).plugins).notify(req)).rejects.toThrow("not found");
    await expect(new RpcNotifier(stub({ at: "t" }).plugins).notify(req)).rejects.toThrow();
    await expect(new RpcNotifier(stub("ok").plugins).notify(req)).rejects.toThrow();
  });
});

describe("bb home pushes and get --card", () => {
  afterEach(cleanupEnvs);
  const cliFor = (r: ReturnType<typeof rig>) => {
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: (id, option, reason, ctx) => r.dele.rule(id, option, reason, ctx), isVizier: () => false });
    return async (argv: string[]) => {
      const res = await Promise.resolve(cli.run(argv, {}));
      return { code: res.exitCode, out: res.stdout ? JSON.parse(res.stdout) : undefined };
    };
  };

  it("lists the blocker asks with time to answer; get --card carries time_to_answer only for a blocker card", async () => {
    const r = rig({ svc: { notifier: new FakeNotifier() } });
    const run = cliFor(r);
    const { t: plain } = await opened(r, {});
    const { t, g1 } = await opened(r, { ask: { urgency: "blocker" }, key: "key-blocker" });
    await r.svc.drainPushes();
    r.advance(30_000);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });

    const list = await run(["pushes", "--json"]);
    expect(list.code).toBe(0);
    expect(list.out.pushes).toHaveLength(1);
    expect(list.out.pushes[0]).toMatchObject({ decision_id: g1.id, push_id: "push-1", answered_by: "pick" });
    expect((await run(["pushes", "--limit", "0"])).code).toBe(2);

    const blocker = await run(["get", "--card", t.id]);
    expect(blocker.out.time_to_answer).toMatchObject({ push_id: "push-1", answered_by: "pick" });
    const other = await run(["get", "--card", plain.id]);
    expect(other.out.time_to_answer).toBeNull();
  });
});
