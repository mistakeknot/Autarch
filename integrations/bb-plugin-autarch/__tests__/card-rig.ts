// Shared fixture for the Task 2.5 card tests: a fake tasks plugin, a poller and a card writer over one temp store.
import assert from "node:assert/strict";
import { CardWriter } from "../cardwrites.js";
import { Delegation } from "../delegation.js";
import { Queue } from "../queue.js";
import { Service } from "../service.js";
import { TasksClient } from "../tasks.js";
import { FakeTasks, type FakeTask } from "./tasks-fake.js";
import { makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";
import { pickOf } from "./helpers.js";

export const T1 = "2026-10-01T00:00:01.000Z";
export const SHA = "a".repeat(64);
export const envs: Env[] = [];
export function cleanupEnvs() {
  while (envs.length) envs.pop()!.cleanup();
}

export interface CardOpts {
  key?: string;
  ident?: string;
  question?: string;
  prose?: string;
  pull?: boolean;
  blocks?: string;
  rootRun?: boolean;
  ask?: Record<string, unknown>;
  breakJson?: boolean;
}
let keyN = 0;
const IDENTS = ["0123456789abcdef", "fedcba9876543210"];

function desc(env: Env, o: CardOpts = {}): string {
  const ask = {
    schema: "home-ask/v2",
    project: "Autarch",
    project_root: env.roots.Autarch,
    question: o.question ?? "Collapse per project or per day?",
    options: o.pull
      ? [
          { id: "a", label: "Per project", kind: "ruling-only" },
          { id: "b", label: "Per day", kind: "ruling-only" },
        ]
      : [
          { id: "a", label: "Per project", kind: "instruction", instruction: "Group per project, run tests, report.", reversible: true },
          { id: "b", label: "Per day", kind: "instruction", instruction: "Group per day, run tests, report.", reversible: true },
        ],
    ...(o.pull ? { pull: "mycroft" } : {}),
    ...(o.ask ?? {}),
  };
  const lines = [o.prose ?? "Which collapse order?", "", `Blocks: ${o.blocks ?? "bead:mk-okek.8"}`, `Request: ${o.key ?? "key-default"} sha256:${o.ident ?? IDENTS[0]}`, ""];
  lines.push("```home-ask", o.breakJson ? "{not json" : JSON.stringify(ask), "```");
  if (o.rootRun) lines.push("```root-run", "script:/opt/run.sh", `sha256:${SHA}`, "timeout:60", "set:s1", "```");
  return lines.join("\n") + "\n";
}

export function rig(opts: { projectName?: string; projects?: string[] } = {}) {
  const env = makeEnv(opts.projects ?? ["Autarch"]);
  envs.push(env);
  const fake = new FakeTasks();
  const tp = fake.addProject(opts.projectName ?? "Autarch");
  const label = fake.addLabel(tp.id, "needs-mk");
  const tasks = new TasksClient(fake, { now: () => env.clock.t });
  const published = { n: 0 };
  const build = () => {
    const svc: Service = env.open();
    const writer = new CardWriter(svc.store.db, tasks, env.now);
    const q = new Queue({ service: svc, tasks, writer, publish: () => void published.n++ });
    return { svc, writer, q, dele: verifiedDelegation(svc) };
  };
  let cur = build();
  let tick = 0;
  const api = {
    env, fake, tp, label, published,
    get svc() { return cur.svc; },
    get q() { return cur.q; },
    get writer() { return cur.writer; },
    get dele() { return cur.dele; },
    get db() { return cur.svc.store.db; },
    /** A new Service, Store and poller over the same database file, as after a process restart. */
    restart() { cur = build(); },
    /** Enable delegation for Autarch with a confirmed binding, and let mk see the settings change. */
    enableDelegation(state: "confirmed" | "suggested" | "rejected" = "confirmed", home = "Autarch") {
      const s = cur.dele.setDelegation({ vizierThreadId: "thr_viz", projects: ["Autarch"], dailyCap: 5 }, {});
      if (!s.ok) throw new Error(s.error);
      cur.dele.markSeen("mk", s.item);
      cur.svc.store.db.prepare("INSERT OR REPLACE INTO project_bindings(tasks_project_id, home_project, state, suggested_at) VALUES (?, ?, ?, ?)").run(tp.id, home, state, env.now());
    },
    /** A real Service.pick of option a on the current revision. */
    mkPick: (decisionId: string, pickId = `p-${decisionId}`, option = "a") => cur.svc.pick(decisionId, option, (cur.svc.store.decision(decisionId) as any).revision, pickId, "mk", "home"),
    vizierPick: (decisionId: string) => cur.dele.rule(decisionId, "a", "reversible and routine", { threadId: "thr_viz" }),
    desc: (o?: CardOpts) => desc(env, o),
    /** A labelled card with an agent comment from thr_a. */
    card(o: CardOpts & { thread?: string | null; title?: string; createdAt?: string } = {}): FakeTask {
      const key = o.key ?? `key-${++keyN}`;
      const t = fake.addTask(tp.id, { labelIds: [label.id], title: o.title ?? "Which collapse order?", description: desc(env, { ...o, key }), ...(o.createdAt ? { createdAt: o.createdAt } : {}) });
      if (o.thread !== null) fake.addComment(t.id, { threadId: o.thread ?? "thr_a", createdAt: T1 });
      return t;
    },
    /** Edit a card as tasks would: new description or fields and a bumped updatedAt. */
    edit(t: FakeTask, patch: Partial<FakeTask> = {}) {
      Object.assign(t, patch, { updatedAt: `2026-10-02T00:00:${String(++tick % 60).padStart(2, "0")}.${String(tick).padStart(3, "0")}Z` });
    },
    poll: () => cur.q.pollOnce(),
    advance: (ms: number) => void (env.clock.t += ms),
    gens: (taskId: string) => cur.svc.store.db.prepare("SELECT * FROM decisions WHERE task_id = ? ORDER BY generation").all(taskId) as any[],
    cardRow: (taskId: string) => cur.svc.store.db.prepare("SELECT * FROM cards WHERE task_id = ?").get(taskId) as any,
    pick: (decisionId: string) => cur.svc.store.recordPick(pickOf(decisionId, { revision: (cur.svc.store.decision(decisionId) as any).revision })),
    pickOld: (decisionId: string, revision: string) => cur.svc.store.recordPick(pickOf(decisionId, { revision })),
  };
  return api;
}
export type Rig = ReturnType<typeof rig>;

/** Open a card, return it and its g1 row. */
export async function opened(r: Rig, o: Parameters<Rig["card"]>[0] = {}) {
  const t = r.card(o);
  await r.poll();
  const g = r.gens(t.id);
  assert.equal(g.length, 1, `card should materialize g1: ${JSON.stringify(r.cardRow(t.id))}`);
  return { t, g1: g[0] };
}

/** Polls with a minute between them, so write-back backoffs elapse. */
export async function pollN(r: Rig, n: number) {
  for (let i = 0; i < n; i++) {
    await r.poll();
    r.advance(61_000);
  }
}
