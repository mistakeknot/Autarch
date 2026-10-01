// Task 2.8a test 3 (A11, finding r2-5): a populated v2 database, created only through the a9853e2 build's own
// entry points, is reopened under v3 and drains in place through the legacy lane.
import { chmodSync, existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import type { Delegation } from "../delegation.js";
import { buildQueue } from "../queueview.js";
import type { Service } from "../service.js";
import { makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";
import { populateV2, startV2Home, type V2Fixture } from "./v2-home.js";

let env: Env;
let svc: Service;
let dele: Delegation;
let asks: Asks;
let fx: V2Fixture;
let run: (argv: string[], ctx?: { threadId?: string }) => Promise<{ exitCode: number; stdout?: string; stderr?: string }>;

beforeEach(async () => {
  env = makeEnv(["Autarch"]);
  const v2 = await startV2Home(env);
  expect(v2.ready()).toBe(true);
  fx = await populateV2(env, v2);
  v2.close();
  // v3 time starts after every v2 timestamp, so backed-off obligations are due and nothing is stalled.
  env.clock.t = Date.now() + 120_000;
  svc = env.open(); // migrates v2 -> v3 with a verified backup
  asks = new Asks(svc);
  dele = verifiedDelegation(svc);
  const cli = homeCli({
    svc,
    asks,
    catchup: new Catchup(svc, dele),
    rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
    isVizier: (t) => t !== undefined && dele.settings().vizierThreadId === t,
  });
  run = (argv, ctx = {}) => Promise.resolve(cli.run(argv, ctx));
});
afterEach(() => {
  try {
    chmodSync(env.roots.Autarch!, 0o755);
    chmodSync(join(env.roots.Autarch!, "docs", "decisions"), 0o755);
  } catch {
    /* gone */
  }
  env.cleanup();
});

const queue = () => buildQueue(svc, asks, {});
const openIds = () => {
  const q = queue().legacy;
  return [...q.owed.map((o) => o.id), ...q.runbook.flatMap((g) => g.items.map((i) => i.id)), ...q.machine.lane.map((m) => m.id), ...q.machine.asks.map((m) => m.id)].sort();
};
const rulingFiles = () => {
  const dir = join(env.roots.Autarch!, "docs", "decisions");
  return existsSync(dir) ? readdirSync(dir) : [];
};

describe("cutover: a populated v2 database drains in place under v3", () => {
  it("the fixture really is v2 data: schema 3 now, with a backup, and legacy rows (no card)", () => {
    expect(svc.store.db.prepare("SELECT value FROM schema_meta WHERE key='schema_version'").get()).toEqual({ value: 3 });
    const log = svc.store.db.prepare("SELECT backup_path FROM migration_log").all() as { backup_path: string }[];
    expect(log).toHaveLength(1);
    expect(existsSync(log[0]!.backup_path)).toBe(true);
    const rows = svc.store.db.prepare("SELECT id, source, task_id FROM decisions").all() as { id: string; source: string; task_id: string | null }[];
    expect(rows.length).toBeGreaterThanOrEqual(7);
    for (const r of rows) expect(r).toMatchObject({ source: "home", task_id: null });
  });

  it("every outstanding ask is in the legacy group with its rev-4 rendering data; the withdrawn, picked and delegated ones are not", () => {
    const q = queue();
    expect(openIds()).toEqual([fx.decide, fx.mycroft, fx.steps, fx.machine].sort());
    expect(q.rows).toEqual([]);
    const decide = q.legacy.owed.find((o) => o.id === fx.decide)!;
    expect(decide).toMatchObject({ project: "Autarch", thread: "thr-a", subject: "autarch/catch-up: collapse order", mentions: 2 });
    expect(decide.ask).toMatchObject({ kind: "decide", options: [expect.objectContaining({ id: "project" }), expect.objectContaining({ id: "day" }), expect.objectContaining({ id: "ask" })] });
    expect(q.legacy.owed.find((o) => o.id === fx.mycroft)!.ask).toMatchObject({ asker: "mycroft" });
    expect(q.legacy.runbook).toEqual([expect.objectContaining({ thread: "thr-s", items: [expect.objectContaining({ id: fx.steps, steps: ["do a", "do b"] })] })]);
    expect(q.legacy.machine.lane).toEqual([expect.objectContaining({ id: fx.machine, owner: "thr-own", detail: "runner is down" })]);
    for (const gone of [fx.withdrawn, fx.picked, fx.delegated]) expect(openIds()).not.toContain(gone);
  });

  it("the drain counter equals the number of open legacy asks", () => {
    expect(queue().legacy.count).toBe(4);
    expect(queue().legacy.count).toBe(openIds().length);
  });

  it("the machine ask's progress event is kept and shown (the ask is fresh, so it is in the lane and not stalled)", () => {
    const ev = svc.store.db.prepare("SELECT at, detail_json FROM events WHERE type='progress' AND decision_id = ?").all(fx.machine) as { at: string; detail_json: string }[];
    expect(ev).toHaveLength(1);
    expect(JSON.parse(ev[0]!.detail_json)).toMatchObject({ thread: "thr-own", note: "restarting the runner" });
    const lane = queue().legacy.machine.lane.find((m) => m.id === fx.machine)!;
    expect(lane).toMatchObject({ owner: "thr-own" });
    expect(queue().legacy.machine.asks).toEqual([]);
  });

  it("progress works on the legacy machine ask (owner only); resolve and withdraw work on legacy ids", async () => {
    expect((await run(["progress", fx.machine, "--note", "again"], { threadId: "thr-own" })).exitCode).toBe(0);
    expect((await run(["progress", fx.machine], { threadId: "thr-x" })).exitCode).not.toBe(0);
    // progress is for machine asks only
    expect((await run(["progress", fx.decide], { threadId: "thr-a" })).exitCode).not.toBe(0);
    expect((await run(["resolve", fx.machine, "--note", "fixed"], { threadId: "thr-own" })).exitCode).toBe(0);
    expect(openIds()).not.toContain(fx.machine);
    const w = (await run(["withdraw", fx.steps], { threadId: "thr-s" }));
    expect(w.exitCode).not.toBe(0);
  });

  it("the lifecycle commands refuse a card id: exit 2, card asks close through tasks", async () => {
    svc.store.db.prepare("INSERT INTO cards(task_id, state) VALUES ('01J0000000000000000000000A', 'open')").run();
    svc.store.db
      .prepare("INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at, task_id, generation, source) VALUES ('card-gen-1','req-c','ident-c','rev-c','sem-c','s','decide','Autarch','thread','thr-a','{}','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z','01J0000000000000000000000A',1,'card')")
      .run();
    for (const verb of ["progress", "resolve", "withdraw"]) {
      const r = await run([verb, "card-gen-1"], { threadId: "thr-a" });
      expect(r.exitCode, `${verb}: ${r.stderr}`).toBe(2);
      expect(r.stderr).toContain("card asks close through tasks");
    }
  });

  it("reconcile writes the pending ruling file", () => {
    expect(rulingFiles().filter((f) => f.endsWith(`${fx.picked}.md`))).toEqual([]);
    expect(svc.store.obligationsFor(fx.picked).find((o) => o.kind === "ruling-file")).toMatchObject({ state: "pending" });
    chmodSync(env.roots.Autarch!, 0o755);
    chmodSync(join(env.roots.Autarch!, "docs", "decisions"), 0o755);
    svc.reconcileAll();
    expect(svc.store.obligationsFor(fx.picked).find((o) => o.kind === "ruling-file")).toMatchObject({ state: "done" });
    expect(rulingFiles().filter((f) => f.endsWith(`${fx.picked}.md`))).toHaveLength(1);
  });

  it("the wake feed still reaches the mentioned threads: mk's pick on the ask they mentioned shows in their feed", () => {
    chmodSync(env.roots.Autarch!, 0o755);
    chmodSync(join(env.roots.Autarch!, "docs", "decisions"), 0o755);
    const rev = (svc.store.decision(fx.decide) as { revision: string }).revision;
    expect(svc.pick(fx.decide, "day", rev, "pick-cutover-1", "mk", "home")).toMatchObject({ ok: true });
    for (const t of ["thr-a", "thr-b", "thr-c"]) expect(svc.feed("Autarch", t).text, t).toContain(fx.decide);
    // a plain option wakes nobody: the feed (read at the thread's next turn) is how the mentioners learn
    expect(svc.store.obligationsFor(fx.decide).filter((o) => o.kind === "wake")).toEqual([]);
  });

  it("an override of the vizier pick works and the replacement is a legacy ask", () => {
    const o = dele.override(fx.delegated, {});
    expect(o).toMatchObject({ ok: true, replay: false });
    if (!o.ok) return;
    expect(openIds()).toContain(o.decision_id);
    expect(svc.store.decision(o.decision_id)).toMatchObject({ source: "home", task_id: null, supersedes: fx.delegated });
    expect(queue().legacy.count).toBe(5);
    // the override is idempotent
    expect(dele.override(fx.delegated, {})).toMatchObject({ ok: true, replay: true, decision_id: o.decision_id });
  });

  it("list --pull mycroft includes the legacy mycroft ask", async () => {
    const r = await run(["list", "--pull", "mycroft", "--json"]);
    expect(r.exitCode).toBe(0);
    const rows = JSON.parse(r.stdout!) as { id: string; task_id: string | null }[];
    expect(rows.map((x) => x.id)).toEqual([fx.mycroft]);
    expect(rows[0]!.task_id).toBeNull();
  });

  it("bb home ask stays retired, so no new legacy row is created", async () => {
    const before = (svc.store.db.prepare("SELECT COUNT(*) AS n FROM decisions").get() as { n: number }).n;
    const r = await run(["ask", "--request", "{}"], { threadId: "thr-a" });
    expect(r.exitCode).toBe(2);
    expect((svc.store.db.prepare("SELECT COUNT(*) AS n FROM decisions").get() as { n: number }).n).toBe(before);
  });
});
