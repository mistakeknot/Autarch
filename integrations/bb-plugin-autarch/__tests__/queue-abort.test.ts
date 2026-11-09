// mk-laqhx: bb stops a service with a 5 s window (a reload that misses it leaves the plugin "degraded: service
// home-queue did not stop"). The poller's run loop must therefore return soon after the abort signal fires, even when
// a poll is in flight and an RPC never answers, and it must not start new work after the abort. Time is scaled down:
// the hung RPC below stands for a handler that runs for tens of seconds.
import { afterEach, describe, expect, it, vi } from "vitest";
import { Queue } from "../queue.js";
import { TasksClient, type PluginsLike } from "../tasks.js";
import { FakeTasks } from "./tasks-fake.js";
import { makeEnv, type Env } from "./service-helpers.js";
import { cleanupEnvs as cleanupCardEnvs, opened, pollN, rig as cardRig } from "./card-rig.js";

const envs: Env[] = [];
afterEach(() => {
  while (envs.length) envs.pop()!.cleanup();
  cleanupCardEnvs();
});

const realSleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    if (signal.aborted) return resolve();
    const timer = setTimeout(resolve, ms);
    signal.addEventListener("abort", () => (clearTimeout(timer), resolve()), { once: true });
  });

const never = new Promise<never>(() => {});
const within = async <T>(p: Promise<T>, ms: number): Promise<T | "timeout"> => {
  let t: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([p, new Promise<"timeout">((r) => (t = setTimeout(() => r("timeout"), ms)))]);
  } finally {
    clearTimeout(t);
  }
};

function rig(plugins: (fake: FakeTasks) => PluginsLike) {
  const env = makeEnv(["Autarch"]);
  envs.push(env);
  const fake = new FakeTasks();
  const tp = fake.addProject("Autarch");
  const label = fake.addLabel(tp.id, "needs-mk");
  const svc = env.open();
  const tasks = new TasksClient(plugins(fake), { now: () => env.clock.t });
  return { env, fake, tp, label, svc, tasks, queue: new Queue({ service: svc, tasks, pollMs: 20 }) };
}

describe("mk-laqhx: the card poller stops inside bb's stop window", () => {
  it("returns soon after the abort although an RPC in flight never answers and ignores the signal", async () => {
    const r = rig(() => ({ callRpc: () => never }));
    const ac = new AbortController();
    const run = r.queue.run(ac.signal, realSleep);
    await new Promise((res) => setTimeout(res, 150)); // the first poll is now stuck in its first RPC
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
  });

  it("hands the abort signal to the RPCs of a poll, so a host that honours it can cancel them", async () => {
    const seen: (AbortSignal | undefined)[] = [];
    const r = rig(() => ({
      callRpc: (a) => {
        seen.push(a.signal);
        return never;
      },
    }));
    const ac = new AbortController();
    void r.queue.run(ac.signal, realSleep);
    await new Promise((res) => setTimeout(res, 100));
    expect(seen.length).toBeGreaterThan(0);
    expect(seen.every((s) => s === ac.signal)).toBe(true);
    ac.abort();
  });

  it("starts no new RPC after the abort: a slow poll over several cards ends at the next step", async () => {
    const calls: { at: number; method: string }[] = [];
    let abortedAt = Infinity;
    const r = rig((fake) => ({
      callRpc: async (a) => {
        calls.push({ at: Date.now(), method: a.method });
        await new Promise((res) => setTimeout(res, 60));
        return fake.callRpc(a);
      },
    }));
    r.fake.addProject("Second");
    r.fake.addLabel(r.fake.projects[1]!.id, "needs-mk");
    const ac = new AbortController();
    const run = r.queue.run(ac.signal, realSleep);
    await new Promise((res) => setTimeout(res, 90)); // mid-poll: a few RPCs are done, more are due
    abortedAt = Date.now();
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    await new Promise((res) => setTimeout(res, 400)); // let any straggling poll show itself
    expect(calls.filter((c) => c.at > abortedAt + 5)).toEqual([]);
  });

  it("a stop is not a tasks failure: health stays ok and no write-back drain starts after the abort", async () => {
    let rpcs = 0;
    const r = rig(() => ({ callRpc: () => (rpcs++, never) }));
    let drains = 0;
    const queue = new Queue({ service: r.svc, tasks: r.tasks, pollMs: 20, writer: { drain: async () => void drains++ } as never });
    const ac = new AbortController();
    const run = queue.run(ac.signal, realSleep);
    await new Promise((res) => setTimeout(res, 100));
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    await new Promise((res) => setTimeout(res, 100));
    expect(queue.status().health).toBe("ok");
    expect(queue.status().last_error).toBeNull();
    expect(drains).toBe(0);
    expect(rpcs).toBe(1);
  });
});

// Returning from run must also leave nothing running that mutates the store: a read answered after the abort changes
// nothing, and a write-back drain in progress starts no further write.
describe("mk-laqhx: a poll that outlives the stop changes nothing", () => {
  /** Make `method` wait on a gate while `when()` holds; resolves `reached` the first time a call waits. */
  function pause(fake: import("./tasks-fake.js").FakeTasks, method: string, when: () => boolean) {
    const orig = fake.callRpc.bind(fake);
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    let reached!: () => void;
    const hit = new Promise<void>((r) => (reached = r));
    fake.callRpc = (async (a: any) => {
      if (a.method === method && when()) {
        reached();
        await gate;
      }
      return orig(a);
    }) as typeof fake.callRpc;
    return { release, hit };
  }

  it("a comment read answered after the abort materializes no card, even over a restarted service", async () => {
    const r = cardRig();
    const t = r.card();
    const p = pause(r.fake, "listComments", () => true);
    const ac = new AbortController();
    const run = r.q.run(ac.signal, realSleep);
    await p.hit; // the first poll is inside the comment read of the new card
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    r.restart(); // bb starts the plugin again over the same database
    p.release(); // the old read now answers
    await new Promise((res) => setTimeout(res, 200));
    expect(r.gens(t.id)).toEqual([]);
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_requests").get()).toEqual({ n: 0 });
  });

  /** A service whose nth root lookup (1-based) waits on a gate. */
  function heldLookup(nth: number) {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    let asked!: () => void;
    const hit = new Promise<void>((r) => (asked = r));
    let n = 0;
    const r: ReturnType<typeof cardRig> = cardRig({ svc: { projects: async () => (++n === nth && (asked(), await gate), r.env.projects) } });
    return { r, release, hit };
  }

  it("the second root lookup of a poll (the one inside a card's materialization) answered after the abort writes no card", async () => {
    const h = heldLookup(2);
    const t = h.r.card();
    const ac = new AbortController();
    const run = h.r.q.run(ac.signal, realSleep);
    await h.hit; // the poll is inside the lookup that precedes the write of g1
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    h.release();
    await new Promise((res) => setTimeout(res, 200));
    expect(h.r.gens(t.id)).toEqual([]);
    expect(h.r.db.prepare("SELECT COUNT(*) AS n FROM card_requests").get()).toEqual({ n: 0 });
  });

  it("the serve project list answered after the abort suggests no binding", async () => {
    const h = heldLookup(1);
    h.r.card();
    const ac = new AbortController();
    const run = h.r.q.run(ac.signal, realSleep);
    await h.hit;
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    h.release();
    await new Promise((res) => setTimeout(res, 200));
    expect(h.r.db.prepare("SELECT COUNT(*) AS n FROM project_bindings").get()).toEqual({ n: 0 });
  });

  it("a write-back drain in progress at the stop starts no further write and records no failure", async () => {
    const r = cardRig();
    const { g1 } = await opened(r);
    r.mkPick(g1.id);
    let inDrain = false;
    const drain = r.writer.drain.bind(r.writer);
    r.writer.drain = (async (s?: AbortSignal) => {
      inDrain = true;
      try {
        return await (drain as any)(s);
      } finally {
        inDrain = false;
      }
    }) as typeof r.writer.drain;
    const p = pause(r.fake, "listComments", () => inDrain);
    const ac = new AbortController();
    const run = r.q.run(ac.signal, realSleep);
    await p.hit; // the drain is inside the read that precedes the mirror comment
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    p.release();
    await new Promise((res) => setTimeout(res, 200));
    expect(r.fake.callsOf("createComment")).toEqual([]);
    expect(r.fake.callsOf("updateTask")).toEqual([]);
    expect(r.db.prepare("SELECT kind, state, attempt, last_error FROM card_writes ORDER BY kind").all()).toEqual([
      { kind: "comment", state: "pending", attempt: 0, last_error: null },
      { kind: "unlabel", state: "pending", attempt: 0, last_error: null },
    ]);
  });
  it("a relabel stopped while its label write is in flight does not go on to reopen the task", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 2); // the unlabel lands and completes the task; the next poll caches it unlabelled and done
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    const p = pause(r.fake, "updateTask", () => true); // the relabel's label write waits
    const ac = new AbortController();
    const run = r.q.run(ac.signal, realSleep);
    await p.hit;
    ac.abort();
    expect(await within(run, 1_000)).not.toBe("timeout");
    p.release(); // the label write lands; the reopen that would follow must not be sent
    await new Promise((res) => setTimeout(res, 200));
    expect(r.fake.callsOf("updateTask").filter((c) => c.input.status === "todo")).toEqual([]);
    expect(r.db.prepare("SELECT state, attempt FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "pending", attempt: 0 });
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.status).toBe("done");
  });

  it("a restarted service's writer does not post a comment whose write is still in flight", async () => {
    const r = cardRig();
    const { g1 } = await opened(r);
    r.mkPick(g1.id);
    const p = pause(r.fake, "createComment", () => true);
    const old = r.writer.drain(); // sends the mirror comment; it has not landed
    await p.hit;
    r.restart(); // bb starts the plugin again over the same database, with a fresh writer
    await r.writer.drain();
    p.release();
    await old;
    const id = (r.db.prepare("SELECT id FROM card_writes WHERE kind = 'comment'").get() as { id: string }).id;
    expect(r.fake.comments.filter((c: any) => c.body.includes(id))).toHaveLength(1);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'comment'").get()).toEqual({ state: "done" });
  });
  it("a read that never answers does not hold its row after a stop: a restarted writer completes the card's writes", async () => {
    const r = cardRig();
    const { t, g1 } = await opened(r);
    r.mkPick(g1.id);
    let reads = 0;
    pause(r.fake, "listComments", () => ++reads === 1); // the first read never answers: the host ignores the abort
    const ac = new AbortController();
    const old = r.writer.drain(ac.signal);
    await new Promise((res) => setTimeout(res, 50));
    ac.abort();
    await new Promise((res) => setTimeout(res, 0)); // the stopped drain lets go of its row
    r.restart();
    expect(await within(r.writer.drain(), 1_000)).not.toBe("timeout");
    expect(r.fake.comments.filter((c: any) => c.body.includes("home-write:"))).toHaveLength(1);
    expect(r.db.prepare("SELECT kind, state FROM card_writes ORDER BY kind").all()).toEqual([
      { kind: "comment", state: "done" },
      { kind: "unlabel", state: "done" },
    ]);
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds).not.toContain(r.label.id);
    void old;
  });

  it("an override made while a stopped unlabel's label write is in flight still ends with the label on the task", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    const p = pause(r.fake, "updateTask", () => true); // the unlabel's label write waits
    const ac = new AbortController();
    const old = r.writer.drain(ac.signal);
    await p.hit;
    ac.abort();
    r.restart(); // bb starts the plugin again over the same database
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await r.writer.drain(); // the relabel must not run past the write still in flight
    p.release(); // the old label write lands
    await old;
    await r.writer.drain();
    await r.writer.drain();
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds).toContain(r.label.id);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "done" });
  });
  it("an accepted write from an expired claim that lands after the new writer settled the relabel does not leave the label off", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    const p = pause(r.fake, "updateTask", () => true); // the unlabel's label write is accepted and then waits
    const ac = new AbortController();
    const old = r.writer.drain(ac.signal);
    await p.hit;
    ac.abort();
    r.restart();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    const real = Date.now();
    const now = vi.spyOn(Date, "now").mockReturnValue(real + 121_000); // past the claim's TTL: the new writer may take the task
    try {
      await r.writer.drain(); // the task still shows the label (the old write has not landed): the relabel has nothing to write and settles
    } finally {
      now.mockRestore();
    }
    p.release(); // now the old write lands and removes the label
    await old;
    await r.writer.drain();
    await r.writer.drain();
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds).toContain(r.label.id);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "done" });
  });
  it("an accepted write from an expired claim that lands while the new writer's relabel holds a stale read does not let that relabel settle", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    const p = pause(r.fake, "updateTask", () => true);
    const ac = new AbortController();
    const old = r.writer.drain(ac.signal);
    await p.hit;
    ac.abort();
    r.restart();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    // The new writer's read of the task is answered (label still on), then held before the relabel goes on.
    const orig = r.fake.callRpc.bind(r.fake);
    let release!: () => void;
    const gate = new Promise<void>((res) => (release = res));
    let reached!: () => void;
    const hit = new Promise<void>((res) => (reached = res));
    let armed = true;
    r.fake.callRpc = (async (a: any) => {
      const v = await orig(a);
      if (armed && a.method === "getTask") {
        armed = false;
        const snapshot = structuredClone(v); // the fake hands out its live task object: a held read must be a copy
        reached();
        await gate;
        return snapshot;
      }
      return v;
    }) as typeof r.fake.callRpc;
    const now = vi.spyOn(Date, "now").mockReturnValue(Date.now() + 121_000);
    let nd: Promise<unknown>;
    try {
      nd = r.writer.drain();
      await hit;
    } finally {
      now.mockRestore();
    }
    p.release(); // the old write lands (the label comes off) while the new relabel holds its stale read
    await old;
    release();
    await nd;
    await r.writer.drain();
    await r.writer.drain();
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds).toContain(r.label.id);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "done" });
  });

  // Round 5: a write the tasks plugin accepted can still land, or be answered with a lost response, after its writer's claim was
  // taken over and the new writer settled. The repair is level-triggered: the latest decision's end state is derived again.

  /** The first call of `method` for which `when()` holds waits on a gate; once released it commits and then fails (a lost response). */
  function commitThenFail(fake: import("./tasks-fake.js").FakeTasks, method: string, when: () => boolean) {
    const orig = fake.callRpc.bind(fake);
    let release!: () => void;
    const gate = new Promise<void>((res) => (release = res));
    let reached!: () => void;
    const hit = new Promise<void>((res) => (reached = res));
    let armed = true;
    fake.callRpc = (async (a: any) => {
      if (armed && a.method === method && when()) {
        armed = false;
        reached();
        await gate;
        await orig(a);
        throw new Error("response lost");
      }
      return orig(a);
    }) as typeof fake.callRpc;
    return { release, hit };
  }

  /** Run `f` with Date.now() past the claim TTL, so a writer may take over a task another writer still holds. */
  async function pastTtl<T>(f: () => Promise<T>): Promise<T> {
    const now = vi.spyOn(Date, "now").mockReturnValue(Date.now() + 121_000);
    try {
      return await f();
    } finally {
      now.mockRestore();
    }
  }

  it("r5-1: a write that commits and then fails after the claim expired and the new writer settled the relabel does not leave the label off", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    const p = commitThenFail(r.fake, "updateTask", () => true); // the unlabel's label write: accepted late, response lost
    const old = r.writer.drain();
    await p.hit;
    r.restart(); // a new writer (bb restarted the plugin) over the same database
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await pastTtl(() => r.writer.drain()); // the label is still on: the relabel has nothing to write and settles
    p.release(); // the old write now commits (the label comes off) and its caller is told it failed
    await old;
    await r.writer.drain();
    await r.writer.drain();
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds).toContain(r.label.id);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "done" });
  });

  it("r5-2: a stopped relabel's reopen that lands after the override was picked and the new writer completed the task does not leave it open", async () => {
    const r = cardRig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 2); // the unlabel lands and completes the task
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    let writes = 0;
    const p = pause(r.fake, "updateTask", () => ++writes === 2); // the relabel's label write lands; its reopen waits
    const ac = new AbortController();
    const old = r.writer.drain(ac.signal);
    await p.hit;
    ac.abort();
    r.restart();
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true }); // mk picks the override: its unlabel completes the task again
    await pastTtl(() => r.writer.drain());
    expect(r.fake.tasks.find((x: any) => x.id === t.id)!.status).toBe("done");
    p.release(); // the old reopen now lands
    await old;
    await r.writer.drain();
    await r.writer.drain();
    const task = r.fake.tasks.find((x: any) => x.id === t.id)!;
    expect(task.status).toBe("done");
    expect(task.labelIds).not.toContain(r.label.id);
    expect(r.db.prepare("SELECT kind, state FROM card_writes WHERE decision_id = ? ORDER BY kind").all(o.decision_id)).toEqual(
      expect.arrayContaining([{ kind: "unlabel", state: "done" }]),
    );
  });

  it("r5-3 (known limitation): a mirror comment still in flight past the claim TTL may be posted twice, and the repair never posts a third", async () => {
    const r = cardRig();
    const { g1 } = await opened(r);
    r.mkPick(g1.id);
    let posts = 0;
    const p = pause(r.fake, "createComment", () => ++posts === 1); // only the first post waits
    const old = r.writer.drain();
    await p.hit;
    r.restart();
    await pastTtl(() => r.writer.drain()); // the comment is not there yet: the new writer posts its own
    p.release(); // the old one lands as well
    await old;
    await r.writer.drain();
    await r.writer.drain();
    const id = (r.db.prepare("SELECT id FROM card_writes WHERE kind = 'comment'").get() as { id: string }).id;
    expect(r.fake.comments.filter((c: any) => c.body.includes(id))).toHaveLength(2);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'comment'").get()).toEqual({ state: "done" });
  });
});
