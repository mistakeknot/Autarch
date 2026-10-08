import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { parseAsk } from "../model.js";
import { homeCli } from "../cli.js";
import { Delegation } from "../delegation.js";
import { removeBinding, setBinding } from "../queueview.js";
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

  it("stats carries the traceability block over the same window when it is wired", async () => {
    const seen: string[] = [];
    const cli = homeCli({
      svc,
      asks: new Asks(svc),
      catchup: new Catchup(svc, dele),
      rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
      isVizier: () => false,
      traceability: async (since) => {
        seen.push(since);
        return { threads: { ended: 1 } };
      },
    });
    const s = JSON.parse((await Promise.resolve(cli.run(["stats", "--since", "3d", "--json"], {}))).stdout!);
    expect(s).toHaveProperty("picks_by");
    expect(s.traceability).toEqual({ threads: { ended: 1 } });
    expect(seen).toEqual([new Date(Date.parse(svc.time()) - 3 * 86_400_000).toISOString()]);
    expect(JSON.parse((await run(["stats", "--json"])).stdout!)).not.toHaveProperty("traceability");
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

  it("list --json carries the card's tasks key, and null for a legacy ask", async () => {
    const { r, c, run: go } = await cardCli({ pull: true });
    const ma = ask(r.env, { asker: "mycroft", subject: "legacy key", question: "Legacy?", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] });
    delete ma.thread;
    const lf = await r.svc.file(ma, {});
    if (!lf.ok) throw new Error(lf.error);
    const rows = JSON.parse((await go(["list", "--json"])).stdout!) as { id: string; key: string | null }[];
    const expected = (r.svc.store.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(c.t.id) as { card_key: string | null }).card_key;
    expect(rows.find((x) => x.id === c.g1.id)!.key).toBe(expected);
    expect(rows.find((x) => x.id === lf.decision_id)!.key).toBeNull();
  });
});

describe("bb home list shows cards Home flags", () => {
  afterEach(() => cleanupEnvs());
  it("includes a card that lost its Request line, with its reason, and leaves it out of --pull", async () => {
    const r = rig();
    const c = await opened(r, { key: "key-flag" });
    const s = r.svc;
    const d = verifiedDelegation(s);
    const cli = homeCli({ svc: s, asks: new Asks(s), catchup: new Catchup(s, d), rule: (id, option, reason, ctx) => d.rule(id, option, reason, ctx), isVizier: () => false });
    const go = (argv: string[]) => Promise.resolve(cli.run(argv, {}));
    r.edit(c.t, { description: c.t.description.replace(/^Request: .*\n/m, "") });
    await r.poll();
    const rows = JSON.parse((await go(["list", "--json"])).stdout!) as { id: string; task_id: string; display_only?: boolean; display_reason?: string }[];
    const flagged = rows.find((x) => x.task_id === c.t.id);
    expect(flagged).toMatchObject({ id: `card:${c.t.id}`, display_only: true, thread: ["thr", "a"].join("_") });
    expect(flagged!.display_reason).toMatch(/^Request line missing/);
    expect(JSON.parse((await go(["list", "--pull", "mycroft"])).stdout!)).toEqual([]);
  });
});

describe("bb home viewing", () => {
  afterEach(() => cleanupEnvs());
  it("answers null until Home shows an ask, then the last ask shown with its key, and null again once it is closed", async () => {
    const r = rig();
    const c = await opened(r, { key: "key-view" });
    const d = verifiedDelegation(r.svc);
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, d), rule: (id, option, reason, ctx) => d.rule(id, option, reason, ctx), isVizier: () => false });
    const go = (argv: string[]) => Promise.resolve(cli.run(argv, {}));
    const view = async () => JSON.parse((await go(["viewing", "--json"])).stdout!) as { viewing: { decision_id: string; key: string | null; task_id: string; subject: string } | null; reason?: string };
    expect((await view()).viewing).toBeNull();
    r.svc.store.setSetting("viewing", JSON.stringify({ decision_id: c.g1.id, at: "2026-10-07T10:00:00.000Z" }));
    const v = (await view()).viewing!;
    expect(v).toMatchObject({ decision_id: c.g1.id, task_id: c.t.id });
    const key = (r.svc.store.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(c.t.id) as { card_key: string | null }).card_key;
    expect(v.key).toBe(key);
    r.svc.store.setSetting("viewing", JSON.stringify({ decision_id: "no-such", at: "2026-10-07T10:00:00.000Z" }));
    expect((await view()).viewing).toBeNull();
    r.svc.store.setSetting("viewing", "not json");
    expect((await view()).viewing).toBeNull();
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

describe("bb home bind / unbind (vizier only, logged)", () => {
  beforeEach(() => enable());
  const bindCli = () => {
    const db = svc.store.db;
    const rec = (by: string) => (type: string, detail: unknown) => svc.store.recordEvent(type, null, { ...(detail as object), by });
    const cli = homeCli({
      svc,
      asks: new Asks(svc),
      catchup: new Catchup(svc, dele),
      rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
      isVizier: (t) => t !== undefined && dele.settings().vizierThreadId === t,
      bind: async (ref, home, by) => {
        const r = setBinding(db, { tasks_project_id: ref, state: "confirmed", home_project: home }, { now: svc.time(), knownProjects: ["Autarch", "shadow-work"], record: rec(by) });
        return r.ok ? { ok: true as const, tasks_project_id: ref } : r;
      },
      unbind: async (ref, by) => {
        const r = removeBinding(db, { tasks_project_id: ref }, { record: rec(by) });
        return r.ok ? { ok: true as const, tasks_project_id: ref, was: r.was } : r;
      },
    });
    return (argv: string[], ctx: { threadId?: string } = {}) => Promise.resolve(cli.run(argv, ctx));
  };
  const events = (type: string) => (svc.store.db.prepare("SELECT at, detail_json FROM events WHERE type = ?").all(type) as { at: string; detail_json: string }[]).map((e) => ({ at: e.at, ...JSON.parse(e.detail_json) }));

  it("refuses every thread but the vizier, and writes nothing", async () => {
    const r = bindCli();
    for (const ctx of [{}, { threadId: "thr-a" }]) {
      for (const argv of [["bind", "tp-1", "shadow-work"], ["unbind", "tp-1"]]) {
        const res = await r(argv, ctx);
        expect(res.exitCode).toBe(1);
        expect(res.stderr).toMatch(/only the vizier/);
      }
    }
    expect(svc.store.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
  });

  it("binds confirmed, logging who and when, and unbind undoes it, logging the same", async () => {
    const r = bindCli();
    const b = await r(["bind", "tp-1", "shadow-work"], { threadId: VIZ });
    expect(b.exitCode).toBe(0);
    expect(svc.store.db.prepare("SELECT home_project, state FROM project_bindings").get()).toEqual({ home_project: "shadow-work", state: "confirmed" });
    expect(events("binding-confirmed")).toMatchObject([{ tasks_project_id: "tp-1", home_project: "shadow-work", by: VIZ }]);
    expect(events("binding-confirmed")[0]!.at).toBeTruthy();
    const u = await r(["unbind", "tp-1"], { threadId: VIZ });
    expect(u.exitCode).toBe(0);
    expect(JSON.parse(u.stdout!).was).toEqual({ home_project: "shadow-work", state: "confirmed" });
    expect(svc.store.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
    expect(events("binding-removed")).toMatchObject([{ tasks_project_id: "tp-1", was: "confirmed", by: VIZ }]);
  });

  it("an unknown Home project exits 2, and unbinding an unbound project exits 1", async () => {
    const r = bindCli();
    expect((await r(["bind", "tp-1", "nowhere"], { threadId: VIZ })).exitCode).toBe(2);
    expect((await r(["unbind", "tp-1"], { threadId: VIZ })).exitCode).toBe(1);
  });

  it("is unavailable, not silent, when the plugin gave the CLI no binder", async () => {
    const res = await run(["bind", "tp-1", "Autarch"], { threadId: VIZ });
    expect(res.exitCode).toBe(1);
    expect(res.stderr).toMatch(/not available/);
  });
});

describe("bb home handoff", () => {
  const handoffCli = (isVizier: (t: string | undefined) => boolean) => {
    const calls: { to: string; ctx: { threadId?: string } }[] = [];
    const cli = homeCli({
      svc,
      asks: new Asks(svc),
      catchup: new Catchup(svc, dele),
      rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
      isVizier,
      handoff: async (to, ctx) => {
        calls.push({ to, ctx });
        return ctx.threadId === "thr_old" ? { ok: true as const, from: "thr_old", to } : { ok: false as const, status: 403, error: "only the vizier thread may hand off" };
      },
    });
    return { calls, run: (argv: string[], ctx: { threadId?: string } = {}) => Promise.resolve(cli.run(argv, ctx)) };
  };

  it("passes the caller's thread and prints the move; a refusal carries its message", async () => {
    const h = handoffCli(() => false);
    const ok = await h.run(["handoff", "thr_new"], { threadId: "thr_old" });
    expect(ok.exitCode).toBe(0);
    expect(JSON.parse(ok.stdout!)).toEqual({ ok: true, from: "thr_old", to: "thr_new" });
    const no = await h.run(["handoff", "thr_new"], { threadId: "thr_x" });
    expect(no.exitCode).not.toBe(0);
    expect(no.stderr).toMatch(/only the vizier thread may hand off/);
    expect(h.calls.map((c) => c.ctx.threadId)).toEqual(["thr_old", "thr_x"]);
  });

  it("an awaited async isVizier gates bind, unbind, rule and note", async () => {
    const h = handoffCli(() => false);
    for (const argv of [["rule", "d1", "o", "--reason", "r"], ["note", "hello"]]) {
      const r = await h.run(argv, { threadId: "thr_x" });
      expect(r.exitCode).toBe(1);
      expect(r.stderr).toMatch(/only the vizier/);
    }
  });

  it("note rechecks the stored vizier right before writing: a handoff during the async check ends its authority", async () => {
    let stored = "thr_old";
    const cli = homeCli({
      svc,
      asks: new Asks(svc),
      catchup: new Catchup(svc, dele),
      rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
      isVizier: async (t) => {
        await Promise.resolve();
        const ok = t === stored;
        stored = "thr_new"; // the handoff lands after the check resolves
        return ok;
      },
      stillVizier: (t) => t === stored,
    });
    const r = await Promise.resolve(cli.run(["note", "hello"], { threadId: "thr_old" }));
    expect(r.exitCode).toBe(1);
    expect(r.stderr).toMatch(/only the vizier/);
  });
});

describe("home report", () => {
  const SHA = "a".repeat(64);
  const open = (sha = SHA) => svc.store.openMove({ task_id: "T1", generation: 1, kind: "script", payload: { script: { path: "/x/s.sh", sha256: sha, args: [], recover: null } }, opened_by: "card" } as never);
  const rep = (...a: string[]) => run(["report", "--script-path", "/x/s.sh", ...a]);
  it("records an ok report on the open script row and leaves the move open", async () => {
    open();
    const r = await rep("--script-sha256", SHA, "--result", "ok", "--log", "/tmp/s.log");
    expect(r.exitCode).toBe(0);
    expect(JSON.parse(r.stdout!)).toEqual({ ok: true, matched: 1 });
    const m = svc.store.move("T1", 1)!;
    expect(m.state).toBe("open");
    expect(m.report_state).toBe("succeeded");
  });
  it("a failed report keeps the step and the log link", async () => {
    open();
    await rep("--script-sha256", SHA, "--result", "failed", "--step", "migrate", "--log", "/tmp/s.log");
    const m = svc.store.move("T1", 1)!;
    expect(m.report_state).toBe("failed");
    expect(JSON.parse(m.report_json!)).toMatchObject({ failing_step: "migrate", report_link: "/tmp/s.log" });
  });
  it("an unmatched sha changes nothing and says so", async () => {
    open();
    const r = await rep("--script-sha256", "b".repeat(64), "--result", "ok");
    expect(JSON.parse(r.stdout!)).toEqual({ ok: true, matched: 0 });
    expect(svc.store.move("T1", 1)!.report_state).toBeNull();
  });
  it("a retry of the same run is a no-op even after a later report; a new run with the same outcome is its own report", async () => {
    open();
    await rep("--script-sha256", SHA, "--result", "failed", "--step", "s", "--report-id", "run1");
    await rep("--script-sha256", SHA, "--result", "ok", "--report-id", "run2");
    await rep("--script-sha256", SHA, "--result", "failed", "--step", "s", "--report-id", "run1"); // delayed retry
    expect(svc.store.move("T1", 1)!.report_state).toBe("succeeded");
    await rep("--script-sha256", SHA, "--result", "failed", "--step", "s", "--report-id", "run3");
    expect(svc.store.move("T1", 1)!.report_state).toBe("failed");
    expect(JSON.parse(svc.store.move("T1", 1)!.report_json!).comment_id).toContain("run3");
  });
  it("a wrong path matches nothing", async () => {
    open();
    const r = await run(["report", "--script-path", "/other.sh", "--script-sha256", SHA, "--result", "ok"]);
    expect(JSON.parse(r.stdout!).matched).toBe(0);
  });
  it("refuses a bad sha, a bad result and an unsafe log path", async () => {
    expect((await rep("--script-sha256", "zz", "--result", "ok")).exitCode).toBe(2);
    expect((await rep("--script-sha256", SHA, "--result", "maybe")).exitCode).toBe(2);
    expect((await rep("--script-sha256", SHA, "--result", "ok", "--log", "rel; rm -rf /")).exitCode).toBe(2);
  });
});
