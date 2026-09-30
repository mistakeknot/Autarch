import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { ask, makeEnv, type Env } from "./service-helpers.js";

const VIZ = "thr-vizier";
let env: Env;
let svc: Service;
let dele: Delegation;
let run: (argv: string[], ctx?: { threadId?: string }) => Promise<{ exitCode: number; stdout?: string; stderr?: string }>;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  dele = new Delegation(svc);
  const cli = homeCli({
    svc,
    asks: new Asks(svc),
    catchup: new Catchup(svc, dele),
    rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
    isVizier: (t) => t !== undefined && dele.settings().vizierThreadId === t,
  });
  run = (argv, ctx = {}) => Promise.resolve(cli.run(argv, ctx));
});
afterEach(() => env.cleanup());

const json = (o: unknown) => JSON.stringify(o);
const filed = async (over: Record<string, unknown> = {}) => {
  const r = await svc.file(ask(env, over), {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
};
const count = () => (svc.store.db.prepare("SELECT COUNT(*) AS n FROM decisions").get() as { n: number }).n;
const mycroftAsk = (over: Record<string, unknown> = {}) => {
  const a = ask(env, { asker: "mycroft", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }], ...over });
  delete a.thread;
  return a;
};
const enable = () => {
  dele.setDelegation({ vizierThreadId: VIZ, projects: ["Autarch"], dailyCap: 5 }, {});
  dele.markSeen("mk", dele.latestSettingsItem()!);
};

describe("bb home ask", () => {
  it("outside a thread without asker mycroft is refused and files nothing", async () => {
    const r = await run(["ask", "--request", json(ask(env))]);
    expect(r.exitCode).toBe(2);
    expect(count()).toBe(0);
  });

  it("outside a thread accepts asker mycroft", async () => {
    const r = await run(["ask", "--request", json(mycroftAsk())]);
    expect(r.exitCode).toBe(0);
    expect(JSON.parse(r.stdout!)).toMatchObject({ mentioned: false });
    expect(count()).toBe(1);
  });

  it("inside a thread uses ctx.threadId when the request names none", async () => {
    const a = ask(env);
    delete a.thread;
    delete a.asker;
    const r = await run(["ask", "--request", json(a)], { threadId: "thr-ctx" });
    expect(r.exitCode).toBe(0);
    const out = JSON.parse(r.stdout!);
    expect(out.mentioned).toBe(false);
    expect(typeof out.request_id).toBe("string");
    expect((svc.store.decision(out.id) as { thread: string }).thread).toBe("thr-ctx");
  });

  it("a request thread that differs from ctx.threadId exits 2 [D-16]", async () => {
    const r = await run(["ask", "--request", json(ask(env, { thread: "thr-a" }))], { threadId: "thr-b" });
    expect(r.exitCode).toBe(2);
    expect(r.stderr).toContain("thread conflicts with caller context");
    expect(count()).toBe(0);
  });

  it("a matching thread is fine", async () => {
    expect((await run(["ask", "--request", json(ask(env, { thread: "thr-a" }))], { threadId: "thr-a" })).exitCode).toBe(0);
  });

  it("exits 2 on invalid JSON, a non-object, and a request over 16 KiB", async () => {
    expect((await run(["ask", "--request", "{nope"], { threadId: "t" })).exitCode).toBe(2);
    expect((await run(["ask", "--request", "[1]"], { threadId: "t" })).exitCode).toBe(2);
    const big = ask(env, { question: "q".repeat(17 * 1024) });
    expect((await run(["ask", "--request", json(big)], { threadId: "thr-a" })).exitCode).toBe(2);
    expect((await run(["ask"], { threadId: "thr-a" })).exitCode).toBe(2);
  });

  it("exit 2 on a validation failure from the service", async () => {
    const r = await run(["ask", "--request", json(ask(env, { project: "Nope" }))], { threadId: "thr-a" });
    expect(r.exitCode).toBe(2);
  });

  it("exit 3 when not filed", async () => {
    env.down.value = true;
    const r = await run(["ask", "--request", json(ask(env))], { threadId: "thr-a" });
    expect(r.exitCode).toBe(3);
    expect(r.stderr).toContain("not-filed");
  });

  it("exit 5 when the decision it replaces is already ruled", async () => {
    const id = await filed();
    const rev = (svc.store.decision(id) as { revision: string }).revision;
    svc.pick(id, "day", rev, "pk1", "mk", "home");
    const r = await run(["ask", "--request", json(ask(env, { question: "Changed?", supersedes: id }))], { threadId: "thr-a" });
    expect(r.exitCode).toBe(5);
  });
});

describe("bb home get, list, stats, feed", () => {
  it("get --request-id returns a decision's and a mention's result", async () => {
    const id = await filed({ thread: "thr-a" });
    const own = ask(env, { thread: "thr-a" });
    const g = await run(["get", "--request-id", (JSON.parse((await run(["ask", "--request", json(own)], { threadId: "thr-a" })).stdout!) as { request_id: string }).request_id]);
    expect(JSON.parse(g.stdout!)).toMatchObject({ result: "decision", decision_id: id, state: "open" });

    const m = await run(["ask", "--request", json(ask(env, { thread: "thr-b" }))], { threadId: "thr-b" });
    const mo = JSON.parse(m.stdout!);
    expect(mo.mentioned).toBe(true);
    const g2 = await run(["get", "--request-id", mo.request_id]);
    expect(JSON.parse(g2.stdout!)).toMatchObject({ result: "mention", decision_id: id });
  });

  it("get --id reports lifecycle state; unknown ids exit 1; no selector exits 2", async () => {
    const id = await filed();
    expect(JSON.parse((await run(["get", "--id", id])).stdout!)).toMatchObject({ id, state: "open" });
    const rev = (svc.store.decision(id) as { revision: string }).revision;
    svc.pick(id, "day", rev, "pk1", "mk", "home");
    expect(JSON.parse((await run(["get", "--id", id])).stdout!)).toMatchObject({ id, state: "picked" });
    expect((await run(["get", "--id", "dec-nope"])).exitCode).toBe(1);
    expect((await run(["get", "--request-id", "nope"])).exitCode).toBe(1);
    expect((await run(["get"])).exitCode).toBe(2);
  });

  it("list, stats and feed print JSON", async () => {
    const id = await filed();
    const l = JSON.parse((await run(["list", "--json"])).stdout!);
    expect(l.map((x: { id: string }) => x.id)).toEqual([id]);
    expect((await run(["list", "--asker", "mycroft", "--json"])).stdout).toBe("[]");
    expect(JSON.parse((await run(["stats", "--since", "14d", "--json"])).stdout!)).toHaveProperty("picks_by");
    const f = JSON.parse((await run(["feed", "--project", "Autarch", "--thread", "thr-a", "--json"])).stdout!);
    expect(f).toHaveProperty("own");
    expect((await run(["feed", "--json"])).exitCode).toBe(2);
  });
});

describe("rule and note (vizier only)", () => {
  it("rule from a non-vizier thread or from outside a thread is refused", async () => {
    enable();
    const id = await filed();
    expect((await run(["rule", id, "project", "--reason", "ok"], { threadId: "thr-x" })).exitCode).not.toBe(0);
    expect((await run(["rule", id, "project", "--reason", "ok"])).exitCode).not.toBe(0);
    expect(svc.store.pick(id)).toBeUndefined();
  });

  it("rule from the vizier picks once", async () => {
    enable();
    const id = await filed();
    const r = await run(["rule", id, "project", "--reason", "reversible and low risk"], { threadId: VIZ });
    expect(r.exitCode).toBe(0);
    expect(svc.store.pick(id)).toMatchObject({ by: "vizier" });
    expect((await run(["rule", id, "day", "--reason", "again"], { threadId: VIZ })).exitCode).toBe(5);
  });

  it("note is vizier only and every cited id must be an existing fact", async () => {
    enable();
    const id = await filed();
    expect((await run(["note", "--cites", id, "hello"], { threadId: "thr-x" })).exitCode).not.toBe(0);
    expect((await run(["note", "--cites", `${id},dec-nope`, "hello"], { threadId: VIZ })).exitCode).not.toBe(0);
    expect((await run(["note", "--cites", id, "hello"], { threadId: VIZ })).exitCode).toBe(0);
    expect((svc.store.db.prepare("SELECT COUNT(*) AS n FROM notes").get() as { n: number }).n).toBe(1);
  });
});

describe("progress, resolve, withdraw", () => {
  it("run through the caller's thread", async () => {
    const id = await filed({
      kind: "machine",
      thread: "thr-a",
      options: undefined,
      question: "Run the tests on Clavain",
      machine: { class: "test-host", detail: "needs a runner", owner_thread: "thr-own" },
    });
    expect((await run(["progress", id, "--note", "on it"], { threadId: "thr-a" })).exitCode).not.toBe(0);
    expect((await run(["progress", id, "--note", "on it"], { threadId: "thr-own" })).exitCode).toBe(0);
    expect((await run(["resolve", id, "--note", "done"], { threadId: "thr-own" })).exitCode).toBe(0);
    expect((await run(["resolve", id, "--note", "done again"], { threadId: "thr-own" })).exitCode).toBe(5);
  });
});

describe("no CLI verb changes delegation settings", () => {
  it("no verb, option or thread identity moves a setting", async () => {
    enable();
    const id = await filed();
    const snap = () => json([svc.store.db.prepare("SELECT * FROM settings_kv ORDER BY key").all(), svc.store.db.prepare("SELECT COUNT(*) AS n FROM events WHERE type = 'delegation-settings-changed'").get()]);
    const before = snap();
    const attempts: string[][] = [
      ["set-delegation", "--vizier-thread-id", "thr-evil"],
      ["delegation", "set", "--daily-cap", "999"],
      ["rule", id, "project", "--reason", "x", "--vizier-thread-id", "thr-evil"],
      ["note", "--cites", id, "x", "--daily-cap", "1000"],
      ["ask", "--request", json(ask(env, { vizierThreadId: "thr-evil" })), "--projects", "*"],
      ["stats", "--settings", "{}"],
    ];
    for (const ctx of [{ threadId: VIZ }, { threadId: "thr-evil" }, {}]) {
      for (const a of attempts) await run(a, ctx);
    }
    expect(snap()).toBe(before);
    const unknown = await run(["set-delegation", "--vizier-thread-id", "thr-evil"], { threadId: VIZ });
    expect(unknown.exitCode).toBe(2);
  });
});
