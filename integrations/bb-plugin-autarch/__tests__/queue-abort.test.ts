// mk-laqhx: bb stops a service with a 5 s window (a reload that misses it leaves the plugin "degraded: service
// home-queue did not stop"). The poller's run loop must therefore return soon after the abort signal fires, even when
// a poll is in flight and an RPC never answers, and it must not start new work after the abort. Time is scaled down:
// the hung RPC below stands for a handler that runs for tens of seconds.
import { afterEach, describe, expect, it } from "vitest";
import { Queue } from "../queue.js";
import { TasksClient, type PluginsLike } from "../tasks.js";
import { FakeTasks } from "./tasks-fake.js";
import { makeEnv, type Env } from "./service-helpers.js";

const envs: Env[] = [];
afterEach(() => {
  while (envs.length) envs.pop()!.cleanup();
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
});
