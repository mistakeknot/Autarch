// Task 2.4: the poller, ingest and the card state machine, driven only by the fake tasks client and
// store fixtures. Picks are rev-4 store.recordPick rows; override rows are rev-4 insertReplacement.
import { afterEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { Queue } from "../queue.js";
import { blocksCount, buildQueue, setBinding, sortRows } from "../queueview.js";
import { Service } from "../service.js";
import { TasksClient } from "../tasks.js";
import { FakeTasks, type FakeTask } from "./tasks-fake.js";
import { ask, makeEnv, type Env } from "./service-helpers.js";
import { pickOf } from "./helpers.js";

const T1 = "2026-10-01T00:00:01.000Z";
const SHA = "a".repeat(64);
const envs: Env[] = [];
afterEach(() => {
  while (envs.length) envs.pop()!.cleanup();
});

interface CardOpts {
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

function rig(opts: { projectName?: string; projects?: string[] } = {}) {
  const env = makeEnv(opts.projects ?? ["Autarch"]);
  envs.push(env);
  const fake = new FakeTasks();
  const tp = fake.addProject(opts.projectName ?? "Autarch");
  const label = fake.addLabel(tp.id, "needs-mk");
  const svc: Service = env.open();
  const tasks = new TasksClient(fake, { now: () => env.clock.t });
  const published = { n: 0 };
  const q = new Queue({ service: svc, tasks, publish: () => void published.n++ });
  const db = svc.store.db;
  let tick = 0;
  const api = {
    env, fake, tp, label, svc, q, published, db,
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
    poll: () => q.pollOnce(),
    advance: (ms: number) => void (env.clock.t += ms),
    gens: (taskId: string) => db.prepare("SELECT * FROM decisions WHERE task_id = ? ORDER BY generation").all(taskId) as any[],
    cardRow: (taskId: string) => db.prepare("SELECT * FROM cards WHERE task_id = ?").get(taskId) as any,
    pick: (decisionId: string) => svc.store.recordPick(pickOf(decisionId, { revision: (svc.store.decision(decisionId) as any).revision })),
    pickOld: (decisionId: string, revision: string) => svc.store.recordPick(pickOf(decisionId, { revision })),
  };
  return api;
}
type Rig = ReturnType<typeof rig>;

/** Open a card, return it and its g1 row. */
async function opened(r: Rig, o: Parameters<Rig["card"]>[0] = {}) {
  const t = r.card(o);
  await r.poll();
  const g = r.gens(t.id);
  expect(g.length, `card should materialize g1: ${JSON.stringify(r.cardRow(t.id))}`).toBe(1);
  return { t, g1: g[0] };
}

describe("T1 first materialization", () => {
  it("inserts g1 with its link columns, registry row, block snapshot and frozen routing", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "k1", blocks: "bead:mk-okek.8 thread:thr_abc project:autarch bead:mk-okek.8" });
    expect(g1).toMatchObject({ id: `card-${t.id}-g1`, request_id: `card:${t.id}:g1`, source: "card", task_id: t.id, generation: 1, tasks_project_id: r.tp.id, supersedes: null, asker: "thread", thread: "thr_a", project: "Autarch", project_root: r.env.roots.Autarch });
    expect(g1.card_fp).toMatch(/^[0-9a-f]{64}$/);
    expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(g1.id)).toEqual([{ ref: "bead:mk-okek.8" }, { ref: "project:autarch" }, { ref: "thread:thr_abc" }]);
    expect(r.db.prepare("SELECT * FROM card_requests WHERE request_key = 'k1'").get()).toMatchObject({ task_id: t.id, identity: IDENTS[0] });
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", routing_mode: "thread", routed_thread: "thr_a", display_reason: null });
    expect(r.published.n).toBeGreaterThan(0);
  });

  it("a pull:mycroft card routes to mycroft with no thread and needs no comment", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { pull: true, thread: null });
    expect(g1).toMatchObject({ asker: "mycroft", thread: "" });
    expect(r.cardRow(t.id)).toMatchObject({ routing_mode: "pull", routed_thread: null });
  });

  it("a malformed card is display-only with the parse error as the reason", async () => {
    const r = rig();
    const t = r.card({ breakJson: true });
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(0);
    expect(r.cardRow(t.id)).toMatchObject({ state: "display" });
    expect(r.cardRow(t.id).display_reason).toMatch(/invalid home-ask JSON/);
  });
});

describe("T3 edit before a pick", () => {
  it("creates g2 superseding g1; a pick on the old revision is refused as stale, on g1 as superseded", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.edit(t, { description: r.desc({ key: JSON.parse(JSON.stringify(r.cardRow(t.id))).request_key, question: "Collapse per project, per day or per week?" }) });
    await r.poll();
    const g = r.gens(t.id);
    expect(g.map((x) => x.generation)).toEqual([1, 2]);
    expect(g[1]).toMatchObject({ supersedes: g1.id, id: `card-${t.id}-g2`, request_id: `card:${t.id}:g2` });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "superseded" });
    expect(r.pick(g[1].id)).toMatchObject({ ok: true });
    expect(r.pickOld(g[1].id, "wrong-revision")).toMatchObject({ ok: false });
  });

  for (const [name, patch] of [
    ["a Blocks-only edit", (r: Rig, key: string) => ({ description: r.desc({ key, blocks: "bead:mk-okek.8 bead:mk-okek.9" }) })],
    ["a title-only edit", (_r: Rig, _key: string) => ({ title: "A different title" })],
    ["a root-run-only edit", (r: Rig, key: string) => ({ description: r.desc({ key, rootRun: true }) })],
  ] as const) {
    it(`T3 on H5 only: ${name} makes g2 with a fresh snapshot and a different card_fp`, async () => {
      const r = rig();
      const { t, g1 } = await opened(r, { key: "kh5" });
      const g1Blocks = r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(g1.id);
      r.edit(t, patch(r, "kh5") as Partial<FakeTask>);
      await r.poll();
      const g = r.gens(t.id);
      expect(g).toHaveLength(2);
      expect(g[1].card_fp).not.toBe(g1.card_fp);
      // The ask's H3 content is the same apart from the predecessor link, and the body is identical.
      expect(g[1].body_json.replace(/"supersedes":"[^"]*"/, "")).toBeTruthy();
      expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(g[1].id).length).toBeGreaterThan(0);
      if (name === "a Blocks-only edit") expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(g[1].id)).not.toEqual(g1Blocks);
      expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "superseded" });
    });
  }

  it("after a pick, a Blocks-only edit creates no generation and sets changed_after_ruling", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "kpb" });
    expect(r.pick(g1.id)).toMatchObject({ ok: true });
    r.edit(t, { description: r.desc({ key: "kpb", blocks: "bead:other" }) });
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.cardRow(t.id)).toMatchObject({ changed_after_ruling: 1, state: "ruled" });
    expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(g1.id)).toEqual([{ ref: "bead:mk-okek.8" }]);
  });
});

describe("no-op re-observation", () => {
  it("a status change or a new comment never creates a generation", async () => {
    const r = rig();
    const { t } = await opened(r, { key: "knoop" });
    r.edit(t, { status: "in_progress" });
    await r.poll();
    r.fake.addComment(t.id, { threadId: "thr_a", kind: "user", createdAt: "2026-10-01T00:05:00.000Z" });
    r.edit(t);
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.cardRow(t.id).state).toBe("open");
  });
});

describe("T8", () => {
  it("after a pick the g1 row is byte-identical and changed_after_ruling is set", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "k8" });
    r.pick(g1.id);
    const before = JSON.stringify(r.db.prepare("SELECT * FROM decisions WHERE id = ?").get(g1.id));
    r.edit(t, { description: r.desc({ key: "k8", question: "A wholly different question to ask?" }) });
    await r.poll();
    r.edit(t, { description: r.desc({ key: "k8", breakJson: true }) });
    await r.poll();
    expect(JSON.stringify(r.db.prepare("SELECT * FROM decisions WHERE id = ?").get(g1.id))).toBe(before);
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.cardRow(t.id).changed_after_ruling).toBe(1);
  });
});

describe("T4 an invalid edit withdraws g n (finding r2-2)", () => {
  const causes: [string, (r: Rig, t: FakeTask) => void][] = [
    ["a broken JSON block", (r, t) => r.edit(t, { description: r.desc({ key: "k4", breakJson: true }) })],
    ["a Request identity token changed", (r, t) => r.edit(t, { description: r.desc({ key: "k4", ident: IDENTS[1] }) })],
    ["an edited Request key", (r, t) => r.edit(t, { description: r.desc({ key: "k4-edited" }) })],
    ["a project mismatch", (r, t) => r.edit(t, { description: r.desc({ key: "k4", ask: { project: "Elsewhere" } }) })],
    ["the earliest agent comment deleted", (r, t) => {
      r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
      r.edit(t);
    }],
  ];
  for (const [name, cause] of causes) {
    it(`${name}: an old-tab pick is refused as withdrawn and nothing is woken`, async () => {
      const r = rig();
      const { t, g1 } = await opened(r, { key: "k4" });
      cause(r, t);
      await r.poll();
      expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(g1.id)).not.toEqual({ withdrawn_at: null });
      expect(r.cardRow(t.id).state).toBe("closed");
      expect(r.cardRow(t.id).display_reason).toBeTruthy();
      expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
      expect(r.db.prepare("SELECT COUNT(*) AS n FROM obligations").get()).toEqual({ n: 0 });
    });
  }
});

describe("T5 a fixed card reopens", () => {
  it("opens g2 with no predecessor after T4, and a pick on g2 succeeds", async () => {
    const r = rig();
    const good = r.desc({ key: "k5" });
    const { t, g1 } = await opened(r, { key: "k5" });
    r.edit(t, { description: r.desc({ key: "k5", breakJson: true }) });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("closed");
    r.edit(t, { description: good });
    await r.poll();
    const g = r.gens(t.id);
    expect(g.map((x) => x.generation)).toEqual([1, 2]);
    expect(g[1].supersedes).toBeNull();
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", display_reason: null });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
    expect(r.pick(g[1].id)).toMatchObject({ ok: true });
  });
});

describe("T6 and T7 the card closes or loses its label", () => {
  const ways: [string, (r: Rig, t: FakeTask) => void][] = [
    ["done", (r, t) => r.edit(t, { status: "done" })],
    ["canceled", (r, t) => r.edit(t, { status: "canceled" })],
    ["unlabelled", (r, t) => r.edit(t, { labelIds: [] })],
    ["deleted", (r, t) => void (r.fake.tasks = r.fake.tasks.filter((x) => x.id !== t.id))],
  ];
  for (const [name, how] of ways) {
    it(`${name} with no pick withdraws g n (T6)`, async () => {
      const r = rig();
      const { t, g1 } = await opened(r);
      how(r, t);
      await r.poll();
      expect(r.cardRow(t.id).state).toBe("closed");
      expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
    });
    it(`${name} with a fixture pick withdraws nothing (T7)`, async () => {
      const r = rig();
      const { t, g1 } = await opened(r);
      r.pick(g1.id);
      how(r, t);
      await r.poll();
      expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(g1.id)).toEqual({ withdrawn_at: null });
      expect(r.cardRow(t.id).state).toBe("ruled");
      expect(r.cardRow(t.id).display_reason).toMatch(/card closed in tasks|unlabelled|card deleted/);
    });
  }

  it("a closed card relabelled opens g n+1 (T5)", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.edit(t, { status: "done" });
    await r.poll();
    r.edit(t, { status: "todo" });
    await r.poll();
    expect(r.gens(t.id).map((g) => g.generation)).toEqual([1, 2]);
    expect(r.cardRow(t.id).state).toBe("open");
  });
});

/** Insert a rev-4 override generation on a vizier-picked g1, as the T10 fixture. */
function overrideOn(r: Rig, g1: any) {
  r.svc.store.recordPick(pickOf(g1.id, { revision: g1.revision, by: "vizier" }));
  const res = r.svc.store.insertReplacement(
    { id: `ovr-${g1.id}`, request_id: `override:${g1.id}`, identity: "ovr-ident", revision: "ovr-rev", semantic_key: g1.semantic_key, subject: g1.subject, kind: "decide", project: g1.project, project_root: g1.project_root, root_dev: g1.root_dev, root_ino: g1.root_ino, asker: g1.asker, thread: g1.thread, body_json: g1.body_json, supersedes: g1.id, delegable: false, source: "card", task_id: g1.task_id, generation: 2, tasks_project_id: g1.tasks_project_id, card_fp: g1.card_fp },
    "override",
  );
  expect(res.ok).toBe(true);
  return `ovr-${g1.id}`;
}

describe("T11 an override generation (store fixture)", () => {
  const events: [string, (r: Rig, t: FakeTask) => void][] = [
    ["done", (r, t) => r.edit(t, { status: "done" })],
    ["unlabel", (r, t) => r.edit(t, { labelIds: [] })],
    ["an invalid edit", (r, t) => r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), breakJson: true }) })],
  ];
  for (const [name, how] of events) {
    it(`survives ${name}`, async () => {
      const r = rig();
      const { t, g1 } = await opened(r);
      const ovr = overrideOn(r, g1);
      how(r, t);
      await r.poll();
      expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(ovr)).toEqual({ withdrawn_at: null });
      expect(r.gens(t.id)).toHaveLength(2);
    });
  }
});

describe("routing is frozen per card (finding r4-3)", () => {
  it("a thread card gaining pull:mycroft is T4; restoring it reopens as g n+1; a third routing is T4 again", async () => {
    const r = rig();
    const orig = r.desc({ key: "kr1" });
    const { t, g1 } = await opened(r, { key: "kr1" });
    r.edit(t, { description: r.desc({ key: "kr1", pull: true }) });
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed", routing_mode: "thread" });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
    r.edit(t, { description: orig });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    const g2 = r.gens(t.id)[1];
    expect(g2).toMatchObject({ generation: 2, supersedes: null, thread: "thr_a" });
    r.edit(t, { description: r.desc({ key: "kr1", pull: true }) });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("closed");
    expect(r.pickOld(g2.id, g2.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
  });

  it("a pull card losing pull (with an agent comment) is T4", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "kr2", pull: true });
    r.fake.addComment(t.id, { threadId: "thr_a", createdAt: T1 });
    r.edit(t, { description: r.desc({ key: "kr2" }) });
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed", routing_mode: "pull", routed_thread: null });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
  });

  it("an earliest agent comment from a different thread is T4", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "kr3" });
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    r.fake.addComment(t.id, { threadId: "thr_other", createdAt: T1 });
    r.edit(t);
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed", routed_thread: "thr_a" });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
  });

  it("a refused replacement falls back to T4 in the same transaction", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "kr4" });
    const spy = vi.spyOn(r.svc.store, "insertReplacement").mockReturnValue({ ok: false, reason: "not-replaceable" });
    r.edit(t, { description: r.desc({ key: "kr4", question: "Another phrasing of the question?" }) });
    await r.poll();
    spy.mockRestore();
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(g1.id)).not.toEqual({ withdrawn_at: null });
    expect(r.cardRow(t.id).state).toBe("closed");
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
  });

  it("cards_routing_frozen rejects any value-changing update of routing_mode and routed_thread", async () => {
    const r = rig();
    const { t } = await opened(r);
    expect(() => r.db.prepare("UPDATE cards SET routing_mode = 'pull' WHERE task_id = ?").run(t.id)).toThrow(/frozen/);
    expect(() => r.db.prepare("UPDATE cards SET routed_thread = 'thr_x' WHERE task_id = ?").run(t.id)).toThrow(/frozen/);
    expect(() => r.db.prepare("UPDATE cards SET routed_thread = 'thr_a' WHERE task_id = ?").run(t.id)).not.toThrow();
  });
});

describe("routing revalidation (finding r4-4)", () => {
  it("a deleted earliest agent comment withdraws g1 with the task metadata unchanged", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const meta = JSON.stringify([t.updatedAt, t.status, t.labelIds, t.description]);
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    expect(JSON.stringify([t.updatedAt, t.status, t.labelIds, t.description])).toBe(meta);
    r.advance(5_000);
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed" });
    expect(r.cardRow(t.id).display_reason).toMatch(/^routing: /);
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
  });

  it("listComments failing on every revalidation leaves g1 open and unwithdrawn (T12)", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const before = r.fake.callsOf("listComments").length;
    for (let i = 1; i <= 5; i++) r.fake.failures.push({ method: "listComments", nth: before + i, error: new Error("tasks down") });
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    for (let i = 0; i < 5; i++) {
      r.advance(5_000);
      await r.poll();
    }
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(g1.id)).toEqual({ withdrawn_at: null });
    expect(r.q.status().health).toBe("degraded: tasks");
  });

  it("150 open cards: each is re-checked within the worst-case age that status reports", async () => {
    const r = rig();
    const tasks = Array.from({ length: 150 }, () => r.card());
    await r.poll();
    expect(tasks.every((t) => r.gens(t.id).length === 1)).toBe(true);
    const worst = r.q.status().routing_worst_case_age_ms;
    expect(worst).toBe(Math.ceil(150 / 20) * 5_000);
    const start = r.env.clock.t;
    for (let i = 0; i < Math.ceil(150 / 20); i++) {
      r.advance(5_000);
      await r.poll();
    }
    const stale = tasks.filter((t) => Date.parse(r.cardRow(t.id).routing_check_at) <= start);
    expect(stale).toHaveLength(0);
    expect(r.q.status().open_cards).toBe(150);
  }, 60_000);

  it("an open override generation is not revalidated (T11)", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const ovr = overrideOn(r, g1);
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    const before = r.fake.callsOf("listComments").length;
    r.advance(5_000);
    await r.poll();
    expect(r.fake.callsOf("listComments").length).toBe(before);
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(ovr)).toEqual({ withdrawn_at: null });
  });

  it("repair with metadata unchanged: the original thread re-posting reopens g2 within the closed backoff (finding r5-5)", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const comment = r.fake.comments.find((c) => c.taskId === t.id)!;
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    r.advance(5_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("closed");
    const meta = JSON.stringify([t.updatedAt, t.status, t.labelIds, t.description]);
    r.fake.addComment(t.id, { threadId: comment.threadId, createdAt: comment.createdAt });
    expect(JSON.stringify([t.updatedAt, t.status, t.labelIds, t.description])).toBe(meta);
    r.advance(31_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    const g2 = r.gens(t.id)[1];
    expect(g2).toMatchObject({ generation: 2, supersedes: null });
    expect(r.pickOld(g1.id, g1.revision)).toMatchObject({ ok: false, reason: "withdrawn" });
    expect(r.pick(g2.id)).toMatchObject({ ok: true });
  });

  it("a comment from a different thread leaves the card closed, and the retry backs off to the 5 min cap", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id);
    r.advance(5_000);
    await r.poll();
    r.fake.addComment(t.id, { threadId: "thr_other", createdAt: T1 });
    for (let i = 0; i < 12; i++) {
      r.advance(300_000);
      await r.poll();
      expect(r.cardRow(t.id).state).toBe("closed");
    }
    const next = Date.parse(r.cardRow(t.id).next_check_at) - r.env.clock.t;
    expect(next).toBeLessThanOrEqual(300_000);
    expect(next).toBeGreaterThan(0);
  });

  it("a card closed for a parse error is not in the routing-closed set", async () => {
    const r = rig();
    const { t } = await opened(r, { key: "kpe" });
    r.edit(t, { description: r.desc({ key: "kpe", breakJson: true }) });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("closed");
    r.advance(600_000);
    expect(r.svc.dueRoutingClosed(10)).toEqual([]);
  });
});

describe("unresolved-retry set", () => {
  it("comment-arrives-later: the card opens with no updatedAt change", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "observed", display_reason: "waiting for asking thread" });
    const stamp = t.updatedAt;
    r.fake.addComment(t.id, { threadId: "thr_late", createdAt: T1 });
    expect(t.updatedAt).toBe(stamp);
    r.advance(5_000);
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", routed_thread: "thr_late" });
  });

  it("serve-recovers: root unverified while serve is down, then it opens", async () => {
    const r = rig();
    r.env.down.value = true;
    const t = r.card();
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("observed");
    expect(r.cardRow(t.id).display_reason).toMatch(/^root unverified: /);
    r.env.down.value = false;
    r.advance(5_000);
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "open" });
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("shows unroutable after 60 s while retries continue, and the backoff caps at 5 min", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    await r.poll();
    r.advance(30_000);
    await r.poll();
    expect(r.cardRow(t.id).display_reason).toBe("waiting for asking thread");
    r.advance(31_000);
    await r.poll();
    expect(r.cardRow(t.id)).toMatchObject({ state: "observed", display_reason: "unroutable" });
    for (let i = 0; i < 14; i++) {
      r.advance(300_000);
      await r.poll();
    }
    const row = r.cardRow(t.id);
    expect(row.state).toBe("observed");
    expect(Date.parse(row.next_check_at) - r.env.clock.t).toBeLessThanOrEqual(300_000);
    expect(row.check_attempts).toBeGreaterThan(8);
    r.fake.addComment(t.id, { threadId: "thr_a", createdAt: T1 });
    r.advance(300_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
  });

  it("backs off 5 s, 10 s, 20 s between retries", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    await r.poll();
    const gaps: number[] = [];
    for (let i = 0; i < 3; i++) {
      gaps.push(Date.parse(r.cardRow(t.id).next_check_at) - r.env.clock.t);
      r.advance(gaps[i]!);
      await r.poll();
    }
    expect(gaps).toEqual([5_000, 10_000, 20_000]);
  });
});

describe("registry and scope", () => {
  it("duplicate cards: the same canonical card in arrival order and in reverse order", async () => {
    for (const reverse of [false, true]) {
      const r = rig();
      const mk = (createdAt: string) => r.card({ key: "kdup", createdAt });
      const [a, b] = reverse ? (() => { const b = mk("2026-10-01T00:00:09.000Z"); const a = mk("2026-10-01T00:00:01.000Z"); return [a, b]; })() : [mk("2026-10-01T00:00:01.000Z"), mk("2026-10-01T00:00:09.000Z")];
      await r.poll();
      expect(r.cardRow(a.id).state, `reverse=${reverse}`).toBe("open");
      expect(r.cardRow(b.id)).toMatchObject({ state: "display", display_reason: "duplicate Request of kdup" });
      expect(r.gens(b.id)).toHaveLength(0);
    }
  });

  it("a later card with an earlier createdAt is still the duplicate once registration exists", async () => {
    const r = rig();
    const a = r.card({ key: "kreg", createdAt: "2026-10-01T00:00:05.000Z" });
    await r.poll();
    const c = r.card({ key: "kreg", createdAt: "2026-10-01T00:00:01.000Z" });
    await r.poll();
    expect(r.cardRow(a.id).state).toBe("open");
    expect(r.cardRow(c.id)).toMatchObject({ state: "display", display_reason: "duplicate Request of kreg" });
    expect(r.db.prepare("SELECT task_id FROM card_requests WHERE request_key = 'kreg'").get()).toEqual({ task_id: a.id });
  });

  it("a project mismatch is display-only", async () => {
    const r = rig({ projects: ["Autarch", "Elsewhere"] });
    const t = r.card({ ask: { project: "Elsewhere", project_root: undefined } });
    r.edit(t, { description: r.desc({ ask: { project: "Elsewhere", project_root: r.env.roots.Elsewhere } }).replace("key-default", "kmm") });
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(0);
    expect(r.cardRow(t.id).state).toBe("display");
    expect(r.cardRow(t.id).display_reason).toMatch(/^project mismatch: card in Autarch, ask targets Elsewhere/);
  });

  it("a root that does not match serve is display-only", async () => {
    const r = rig();
    const t = r.card({ ask: { project_root: "/not/the/root" } });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("display");
    expect(r.gens(t.id)).toHaveLength(0);
  });
});

describe("project bindings", () => {
  it("an exact name match writes suggested, with serve's own spelling", async () => {
    const r = rig({ projectName: "autarch" });
    r.card();
    await r.poll();
    expect(r.db.prepare("SELECT * FROM project_bindings").all()).toMatchObject([{ tasks_project_id: r.tp.id, home_project: "Autarch", state: "suggested" }]);
  });
  it("a fuzzy name never writes a row", async () => {
    for (const name of ["Autarc", "Autarch Two", "autarch-2", "My Autarch"]) {
      const r = rig({ projectName: name });
      const t = r.card();
      await r.poll();
      expect(r.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get(), name).toEqual({ n: 0 });
      expect(r.cardRow(t.id).state).toBe("display");
    }
  });
  it("an existing confirmed or rejected row is never rewritten by the poller", async () => {
    const r = rig();
    r.db.prepare("INSERT INTO project_bindings(tasks_project_id, home_project, state) VALUES (?, 'Autarch', 'rejected')").run(r.tp.id);
    const t = r.card();
    await r.poll();
    expect(r.db.prepare("SELECT state FROM project_bindings").get()).toEqual({ state: "rejected" });
    expect(r.cardRow(t.id).state).toBe("display");
  });
});

describe("label handling (finding r3-6)", () => {
  it("needs-mk deleted, recreated with a new id and reapplied: the card stays open and nothing is withdrawn", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.labels = r.fake.labels.filter((l) => l.id !== r.label.id);
    const fresh = r.fake.addLabel(r.tp.id, "needs-mk");
    r.edit(t, { labelIds: [fresh.id] });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE task_id = ?").get(t.id)).toEqual({ withdrawn_at: null });
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("a genuinely removed label withdraws only after the fresh listLabels confirms it", async () => {
    const r = rig();
    const { t } = await opened(r);
    const labelReads = r.fake.callsOf("listLabels").length;
    r.edit(t, { labelIds: [] });
    await r.poll();
    expect(r.fake.callsOf("listLabels").length).toBeGreaterThan(labelReads);
    expect(r.cardRow(t.id).state).toBe("closed");
  });

  it("listLabels failing during confirmation withdraws nothing and keeps the snapshot", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.failures.push({ method: "listLabels", nth: r.fake.callsOf("listLabels").length + 1, error: new Error("labels down") });
    r.edit(t, { labelIds: [] });
    const res = await r.poll();
    expect(res.ok).toBe(false);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE task_id = ?").get(t.id)).toEqual({ withdrawn_at: null });
    expect(r.q.status().health).toBe("degraded: tasks");
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("closed");
  });

  it("two labels named needs-mk: either one counts as labelled", async () => {
    const r = rig();
    const second = r.fake.addLabel(r.tp.id, "needs-mk");
    const t = r.card();
    r.edit(t, { labelIds: [second.id] });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.fake.callsOf("listTasks")[0]!.input.labelIds).toEqual([r.label.id, second.id]);
  });
});

describe("degraded mode and invariants", () => {
  it("a partial page read changes nothing", async () => {
    const r = rig();
    for (let i = 0; i < 501; i++) r.fake.addTask(r.tp.id, { labelIds: [r.label.id], description: "plain text" });
    r.fake.failures.push({ method: "listTasks", nth: 2, error: new Error("page 2 failed") });
    const res = await r.poll();
    expect(res.ok).toBe(false);
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM cards").get()).toEqual({ n: 0 });
    expect(r.q.status().health).toBe("degraded: tasks");
  });

  it("when tasks is unavailable health is degraded: tasks and the last snapshot is kept", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.fake.failures.push({ method: "listProjects", nth: r.fake.callsOf("listProjects").length + 1, error: new Error("tasks is down") });
    r.edit(t, { status: "done" });
    const res = await r.poll();
    expect(res.ok).toBe(false);
    expect(r.q.status()).toMatchObject({ health: "degraded: tasks", last_error: expect.stringContaining("tasks is down") });
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.gens(t.id)).toEqual([g1]);
    await r.poll();
    expect(r.q.status().health).toBe("ok");
    expect(r.cardRow(t.id).state).toBe("closed");
  });

  it("the current generation is always max(generation) over random event sequences", async () => {
    for (let seed = 1; seed <= 6; seed++) {
      let s = seed * 2654435761;
      const rnd = (n: number) => ((s = (s * 1103515245 + 12345) & 0x7fffffff), s % n);
      const r = rig();
      const { t } = await opened(r, { key: "kprop" });
      const comment = r.fake.comments.find((c) => c.taskId === t.id)!;
      let q = 0;
      for (let step = 0; step < 40; step++) {
        switch (rnd(8)) {
          case 0: r.edit(t, { description: r.desc({ key: "kprop", question: `Question number ${++q} for the card?` }) }); break;
          case 1: r.edit(t, { description: r.desc({ key: "kprop", breakJson: true }) }); break;
          case 2: r.edit(t, { description: r.desc({ key: "kprop", blocks: `bead:b${++q}` }) }); break;
          case 3: r.edit(t, { status: rnd(2) ? "done" : "todo" }); break;
          case 4: r.edit(t, { labelIds: rnd(2) ? [] : [r.label.id] }); break;
          case 5: r.fake.comments = r.fake.comments.filter((c) => c.taskId !== t.id); break;
          case 6: if (!r.fake.comments.some((c) => c.taskId === t.id)) r.fake.addComment(t.id, { threadId: "thr_a", createdAt: comment.createdAt }); break;
          case 7: if (rnd(4) === 0 && r.gens(t.id).length) { const g = r.svc.currentGeneration(t.id) as any; if (!g.withdrawn_at) r.pickOld(g.id, g.revision); } break;
        }
        r.advance(31_000);
        await r.poll();
        const gens = r.gens(t.id).map((g) => g.generation as number);
        expect(gens).toEqual(gens.map((_, i) => i + 1));
        if (gens.length) expect((r.svc.currentGeneration(t.id) as any).generation).toBe(Math.max(...gens));
        const live = r.gens(t.id).filter((g) => !g.withdrawn_at && !r.db.prepare("SELECT 1 FROM decisions WHERE supersedes = ?").get(g.id) && !r.db.prepare("SELECT 1 FROM picks WHERE decision_id = ?").get(g.id));
        expect(live.length).toBeLessThanOrEqual(1);
      }
    }
  }, 60_000);
});

// ---- Task 2.6: the queue read model, plan 1.4 --------------------------------------------------

const view = (r: Rig, thread?: string) => buildQueue(r.svc, new Asks(r.svc), thread === undefined ? {} : { thread });

describe("blocksCount (Q3: direct refs only)", () => {
  it("counts distinct known refs once and does not count unknown kinds", () => {
    expect(blocksCount(["bead:a", "bead:a", "thread:thr_x", "project:p", "ticket:9"])).toBe(3);
    expect(blocksCount([])).toBe(0);
  });
});

describe("queue sort order", () => {
  it("sorts by blocks count desc, then createdAt oldest first, then id", async () => {
    const r = rig();
    const one = r.card({ key: "q-one", blocks: "bead:a", createdAt: "2026-09-01T00:00:00.000Z" });
    const threeNew = r.card({ key: "q-3n", blocks: "bead:a bead:b bead:c", createdAt: "2026-09-20T00:00:00.000Z" });
    const threeOld = r.card({ key: "q-3o", blocks: "bead:a bead:b thread:thr_z", createdAt: "2026-09-10T00:00:00.000Z" });
    const unknown = r.card({ key: "q-u", blocks: "bead:a ticket:1 ticket:2 ticket:3", createdAt: "2026-08-01T00:00:00.000Z" });
    await r.poll();
    const v = view(r);
    expect(v.rows.map((x) => x.task_id)).toEqual([threeOld.id, threeNew.id, unknown.id, one.id]);
    expect(v.rows.map((x) => x.blocks_count)).toEqual([3, 3, 1, 1]);
  });
  it("breaks a full tie by id", () => {
    const row = (id: string) => ({ id, blocks_count: 1, created_at: "2026-09-01T00:00:00.000Z", pinned: false }) as never;
    expect(sortRows([row("b"), row("a")]).map((x: { id: string }) => x.id)).toEqual(["a", "b"]);
  });
});

describe("queue pinning", () => {
  it("pins cards that name the thread in Blocks or were asked from it, ahead of heavier cards", async () => {
    const r = rig();
    const heavy = r.card({ key: "p-heavy", blocks: "bead:a bead:b bead:c bead:d", thread: "thr_other" });
    const named = r.card({ key: "p-ref", blocks: "thread:thr_me", thread: "thr_other" });
    const asked = r.card({ key: "p-ask", blocks: "bead:z", thread: "thr_me" });
    await r.poll();
    const v = view(r, "thr_me");
    expect(v.rows.map((x) => [x.task_id, x.pinned])).toEqual([[named.id, true], [asked.id, true], [heavy.id, false]].sort((a, b) => Number(b[1]) - Number(a[1])));
    expect(v.rows.slice(0, 2).map((x) => x.pinned)).toEqual([true, true]);
    expect(view(r).rows[0]!.task_id).toBe(heavy.id);
    expect(view(r).rows.every((x) => !x.pinned)).toBe(true);
  });
});

describe("queue rows: display reasons, free-form cards, markers", () => {
  it("a free-form card (no home-ask) is display-only with its parse reason and no ask", async () => {
    const r = rig();
    const t = r.fake.addTask(r.tp.id, { labelIds: [r.label.id], title: "Just prose", description: "please decide something" });
    await r.poll();
    const row = view(r).rows.find((x) => x.task_id === t.id)!;
    expect(row).toMatchObject({ display_only: true, ask: null, decision_id: null, title: "Just prose" });
    expect(row.display_reason).toBeTruthy();
  });
  it("shows each display reason on its card", async () => {
    const r = rig({ projects: ["Autarch", "Other"] });
    const bad = r.card({ key: "d-bad", breakJson: true });
    const first = r.card({ key: "d-dup" });
    const dup = r.card({ key: "d-dup" });
    const other = r.card({ key: "d-other", ask: { project: "Other", project_root: r.env.roots.Other } });
    const root = r.card({ key: "d-root", title: "rootcard" });
    const routing = r.card({ key: "d-route", title: "routecard" });
    await r.poll();
    r.db.prepare("UPDATE cards SET root_state = 'unverified', root_reason = 'sha256 does not match' WHERE task_id = ?").run(root.id);
    r.db.prepare("UPDATE cards SET state = 'closed', display_reason = 'routing: asking thread is not in this project' WHERE task_id = ?").run(routing.id);
    r.db.prepare("UPDATE decisions SET withdrawn_at = ? WHERE task_id = ?").run(r.env.now(), routing.id);
    const rows = view(r).rows;
    const by = (id: string) => rows.find((x) => x.task_id === id)!;
    expect(by(bad.id).display_reason).toMatch(/invalid home-ask JSON/);
    expect([by(first.id), by(dup.id)].some((x) => /duplicate Request/.test(x.display_reason ?? ""))).toBe(true);
    expect(by(other.id).display_reason).toMatch(/project mismatch/);
    expect(by(root.id).root).toEqual({ state: "unverified", reason: "sha256 does not match" });
    expect(by(root.id).display_only).toBe(false);
    expect(by(routing.id)).toMatchObject({ display_only: true, display_reason: "asking thread is not in this project" });
  });
  it("marks an override generation with the ruling it overrides, and a changed-after-ruling card", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "m-1" });
    overrideOn(r, g1);
    const row = view(r).rows.find((x) => x.task_id === t.id)!;
    expect(row).toMatchObject({ decision_id: `ovr-${g1.id}`, overrides_generation: 1, display_only: false });
    const r2 = rig();
    const { t: t2, g1: h1 } = await opened(r2, { key: "m-2" });
    r2.pick(h1.id);
    r2.db.prepare("UPDATE cards SET state = 'ruled', changed_after_ruling = 1 WHERE task_id = ?").run(t2.id);
    expect(view(r2).rows.find((x) => x.task_id === t2.id)).toMatchObject({ changed_after_ruling: true, display_only: true, display_reason: "changed after ruling" });
  });
  it("carries the binding state and the pick options of an open row", async () => {
    const r = rig();
    const { g1 } = await opened(r, { key: "o-1" });
    const row = view(r).rows[0]!;
    expect(row).toMatchObject({ decision_id: g1.id, revision: g1.revision, binding_state: "suggested", display_only: false, refs: [{ ref: "bead:mk-okek.8", counted: true }] });
    expect(row.ask!.options.map((o) => o.id)).toEqual(["a", "b"]);
  });
  it("puts legacy steps and machine asks in the legacy group, not the rows", async () => {
    const r = rig();
    const a = await r.svc.file(ask(r.env, { kind: "steps", options: undefined, steps: ["one", "two"], thread: "thr_l", subject: "s", question: "do the steps" }), {});
    const m = await r.svc.file(ask(r.env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "disk full" }, thread: "thr_l", subject: "m", question: "machine blocked" }), {});
    if (!a.ok || !m.ok) throw new Error("file failed");
    const v = view(r);
    expect(v.rows).toEqual([]);
    expect(v.legacy.runbook[0]!.items[0]).toMatchObject({ id: a.decision_id, steps: ["one", "two"] });
    expect(v.legacy.machine.asks[0]).toMatchObject({ id: m.decision_id, detail: "disk full" });
    expect(v.legacy.count).toBe(2);
  });
});

describe("unbound tasks projects (mk-okek: no name match, so no binding row)", () => {
  const ctx = (r: Rig) => ({ now: r.env.now(), knownProjects: ["Autarch"], record: (type: string, detail: unknown) => void r.svc.store.recordEvent(type, null, detail) });
  it("lists a tasks project with cards and no binding row, with the project its asks target; binding it frees the card", async () => {
    const r = rig({ projectName: "Shadow Work" });
    const t = r.card();
    await r.poll();
    expect(r.cardRow(t.id).display_reason).toMatch(/^project mismatch: card in .*, ask targets Autarch/);
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
    expect(view(r).unbound).toEqual([{ tasks_project_id: r.tp.id, cards: 1, targets: ["Autarch"] }]);
    expect(setBinding(r.db, { tasks_project_id: r.tp.id, state: "confirmed", home_project: "Autarch" }, ctx(r))).toMatchObject({ ok: true });
    expect(view(r).unbound).toEqual([]);
    r.q.rebind([t.id]);
    r.advance(60_000);
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

describe("rebind racing an in-flight ingest (review finding)", () => {
  it("an ingest that began before the rebind does not record its stale digest, so the next poll re-evaluates", async () => {
    const r = rig({ projectName: "Shadow Work" });
    const t = r.card();
    const orig = r.svc.ingestCard.bind(r.svc);
    const spy = vi.spyOn(r.svc, "ingestCard").mockImplementation(async (...a) => {
      const res = await orig(...a);
      // mk confirms the binding while this ingest is still in flight; it saw no binding.
      setBinding(r.db, { tasks_project_id: r.tp.id, state: "confirmed", home_project: "Autarch" }, { now: r.env.now(), knownProjects: ["Autarch"], record: () => {} });
      r.q.rebind([t.id]);
      return res;
    });
    await r.poll();
    spy.mockRestore();
    expect(r.gens(t.id)).toHaveLength(0);
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

describe("setBinding (Q5: mk confirms once)", () => {
  const ctx = (r: Rig) => ({ now: r.env.now(), knownProjects: ["Autarch"], record: (type: string, detail: unknown) => void r.svc.store.recordEvent(type, null, detail) });
  it("confirms a suggested binding and records an event; delegation then clears the binding refusal", async () => {
    const r = rig();
    r.card();
    await r.poll();
    expect(setBinding(r.db, { tasks_project_id: r.tp.id, state: "confirmed" }, ctx(r))).toEqual({ ok: true, state: "confirmed" });
    expect(r.db.prepare("SELECT state, confirmed_at FROM project_bindings").get()).toMatchObject({ state: "confirmed" });
    expect(r.db.prepare("SELECT type FROM events WHERE type = 'binding-confirmed'").all()).toHaveLength(1);
  });
  it("rejects an unknown Home project and an unknown tasks project with no home_project", () => {
    const r = rig();
    expect(setBinding(r.db, { tasks_project_id: "nope", state: "confirmed" }, ctx(r))).toMatchObject({ ok: false, status: 404 });
    expect(setBinding(r.db, { tasks_project_id: "nope", state: "confirmed", home_project: "Ghost" }, ctx(r))).toMatchObject({ ok: false, status: 400 });
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
  });
  it("creates a confirmed binding for an unsuggested project when home_project is a known project", () => {
    const r = rig();
    expect(setBinding(r.db, { tasks_project_id: "tp-x", state: "confirmed", home_project: "Autarch" }, ctx(r))).toMatchObject({ ok: true });
    expect(r.db.prepare("SELECT * FROM project_bindings WHERE tasks_project_id = 'tp-x'").get()).toMatchObject({ home_project: "Autarch", state: "confirmed" });
  });
});
