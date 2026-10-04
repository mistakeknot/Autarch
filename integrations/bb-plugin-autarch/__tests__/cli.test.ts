import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { parseAsk } from "../model.js";
import { homeCli } from "../cli.js";
import { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { ask, makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";

const VIZ = "thr-vizier";
let env: Env;
let svc: Service;
let dele: Delegation;
let run: (argv: string[], ctx?: { threadId?: string }) => Promise<{ exitCode: number; stdout?: string; stderr?: string }>;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  dele = verifiedDelegation(svc);
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

describe("bb home ask is retired", () => {
  it("exits 2 naming the card filer, from any context, and files nothing", async () => {
    for (const ctx of [{}, { threadId: "thr-a" }]) {
      for (const argv of [["ask", "--request", json(ask(env))], ["ask", "--request", json(mycroftAsk())], ["ask"]]) {
        const r = await run(argv, ctx);
        expect(r.exitCode).toBe(2);
        expect(r.stderr).toContain("moved");
        expect(r.stderr).toContain("autarch needs-mk file");
      }
    }
    expect(count()).toBe(0);
  });
});

describe("bb home get, list, stats, feed", () => {
  it("get --request-id and --request return a decision's and a mention's result", async () => {
    const own = ask(env, { thread: "thr-a" });
    const id = await filed({ thread: "thr-a" });
    const key = parseAsk(own).request_id!;
    for (const flag of ["--request-id", "--request"]) {
      const g = await run(["get", flag, key]);
      expect(JSON.parse(g.stdout!)).toMatchObject({ result: "decision", decision_id: id, state: "open" });
    }
    const g = await run(["get", "--request", key]);

    const other = ask(env, { thread: "thr-b" });
    const m = await svc.file(other, { threadId: "thr-b" });
    expect(m.ok && m.mentioned).toBe(true);
    const g2 = await run(["get", "--request", parseAsk(other).request_id!]);
    expect(JSON.parse(g2.stdout!)).toMatchObject({ result: "mention", decision_id: id });
    // E-6: the recovery read carries identity and scope.
    const first = JSON.parse(g.stdout!) as { identity: string; thread: string; project: string };
    expect(first.identity).toMatch(/^[0-9a-f]{64}$/);
    expect(first.thread).toBe("thr-a");
    expect(first.project).toBe((svc.store.decision(id) as { project: string }).project);
    expect(JSON.parse(g2.stdout!)).toMatchObject({ thread: "thr-b" });
  });

  it("get takes --json like every other home command (the Go filer always passes it; real bb rejects an undeclared flag)", async () => {
    const absent = await run(["get", "--request", "no-such-key", "--json"]);
    expect(absent.exitCode).toBe(0);
    expect(JSON.parse(absent.stdout!)).toMatchObject({ status: "absent" });
    expect((await run(["get", "--card", "no-such-card", "--json"])).exitCode).toBe(1);
  });

  it("get --id reports lifecycle state; unknown ids exit 1; no selector exits 2", async () => {
    const id = await filed();
    expect(JSON.parse((await run(["get", "--id", id])).stdout!)).toMatchObject({ id, state: "open" });
    const rev = (svc.store.decision(id) as { revision: string }).revision;
    svc.pick(id, "day", rev, "pk1", "mk", "home");
    expect(JSON.parse((await run(["get", "--id", id])).stdout!)).toMatchObject({ id, state: "picked" });
    expect((await run(["get", "--id", "dec-nope"])).exitCode).toBe(1);
    expect(JSON.parse((await run(["get", "--request-id", "nope"])).stdout!)).toEqual({ status: "absent" });
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

describe("cards in the CLI (Task 2.7)", () => {
  const cardCli = async (o: Parameters<typeof opened>[1] = {}) => {
    const r = rig();
    const c = await opened(r, o);
    const s = r.svc;
    const d = verifiedDelegation(s);
    const cli = homeCli({
      svc: s,
      asks: new Asks(s),
      catchup: new Catchup(s, d),
      rule: (id, option, reason, ctx) => d.rule(id, option, reason, ctx),
      isVizier: () => false,
    });
    return { r, c, run: (argv: string[], ctx: { threadId?: string } = {}) => Promise.resolve(cli.run(argv, ctx)) };
  };
  afterEach(() => cleanupEnvs());

  it("progress, resolve and withdraw refuse a card row with exit 2 and change nothing", async () => {
    const { r, c, run: go } = await cardCli();
    const before = JSON.stringify(r.svc.store.decision(c.g1.id));
    for (const verb of ["progress", "resolve", "withdraw"]) {
      const x = await go([verb, c.g1.id], { threadId: "thr_a" });
      expect(x.exitCode).toBe(2);
      expect(x.stderr).toContain("card asks close through tasks");
    }
    expect(JSON.stringify(r.svc.store.decision(c.g1.id))).toBe(before);
  });

  it("get --card reads the card and its current generation; an unknown card exits 1", async () => {
    const { c, run: go } = await cardCli();
    const x = await go(["get", "--card", c.t.id]);
    expect(x.exitCode).toBe(0);
    expect(JSON.parse(x.stdout!)).toMatchObject({ task_id: c.t.id, state: "open", decision_id: c.g1.id, generation: 1, decision_state: "open" });
    expect((await go(["get", "--card", "nope"])).exitCode).toBe(1);
  });

  it("get --request resolves a card key through card_requests, and a legacy key through the registry", async () => {
    const { r, c, run: go } = await cardCli({ key: "key-card-1" });
    const x = await go(["get", "--request", "key-card-1"]);
    expect(JSON.parse(x.stdout!)).toMatchObject({ status: "registered", task_id: c.t.id, request_key: "key-card-1", decision_id: c.g1.id });
    expect(JSON.parse(x.stdout!).identity).toEqual(expect.any(String));
    const absent = await go(["get", "--request", "missing"]);
    expect(absent.exitCode).toBe(0);
    expect(JSON.parse(absent.stdout!)).toEqual({ status: "absent" });

    const legacyEnv = makeEnv(["Autarch"]);
    try {
      const s = legacyEnv.open();
      const a = ask(legacyEnv, { thread: "thr-a" });
      const f = await s.file(a, {});
      if (!f.ok) throw new Error(f.error);
      const d = verifiedDelegation(s);
      const cli = homeCli({ svc: s, asks: new Asks(s), catchup: new Catchup(s, d), rule: () => ({ ok: false, status: 400, error: "x" }) as never, isVizier: () => false });
      const g = await cli.run(["get", "--request", parseAsk(a).request_id!], {});
      expect(JSON.parse(g.stdout!)).toMatchObject({ status: "legacy", result: "decision", decision_id: f.decision_id });
    } finally {
      legacyEnv.cleanup();
    }
  });

  it("get --request exits 3 while Home is not ready, for every key, so the filer creates nothing", async () => {
    const { r, run: go } = await cardCli({ key: "key-card-2" });
    const spy = vi.spyOn(r.svc, "ready").mockReturnValue(false);
    for (const key of ["key-card-2", "missing"]) {
      const x = await go(["get", "--request", key]);
      expect(x.exitCode).toBe(3);
      expect(x.stdout ?? "").not.toContain("absent");
    }
    spy.mockRestore();
  });

  it("list --pull mycroft keeps pull cards and legacy asker-mycroft rows, and drops thread asks", async () => {
    const { r, c, run: go } = await cardCli({ pull: true });
    const t = await opened(r, { key: "key-thread" });
    const ma = ask(r.env, { asker: "mycroft", subject: "legacy pull", question: "Legacy pull?", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] });
    delete ma.thread;
    const lf = await r.svc.file(ma, {});
    if (!lf.ok) throw new Error(lf.error);
    const all = JSON.parse((await go(["list"])).stdout!) as { id: string }[];
    expect(all.map((x) => x.id)).toEqual(expect.arrayContaining([c.g1.id, t.g1.id, lf.decision_id]));
    const pulled = JSON.parse((await go(["list", "--pull", "mycroft"])).stdout!) as { id: string; task_id: string | null }[];
    expect(pulled.map((x) => x.id).sort()).toEqual([c.g1.id, lf.decision_id].sort());
    expect(pulled.find((x) => x.id === c.g1.id)!.task_id).toBe(c.t.id);
  });
});

describe("bb home binding", () => {
  it("reads the binding row, or nulls when the tasks project is unbound", async () => {
    svc.store.db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state) VALUES ('tp-1', 'shadow-work', 'confirmed')").run();
    const hit = await run(["binding", "tp-1"]);
    expect(hit.exitCode).toBe(0);
    expect(JSON.parse(hit.stdout!)).toEqual({ tasks_project_id: "tp-1", home_project: "shadow-work", state: "confirmed" });
    expect(JSON.parse((await run(["binding", "tp-none"])).stdout!)).toEqual({ tasks_project_id: "tp-none", home_project: null, state: null });
  });
});
