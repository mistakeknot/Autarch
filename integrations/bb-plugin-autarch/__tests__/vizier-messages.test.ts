// A card for every plain operator message in the vizier thread (bead mk-ppvma.4). Synthetic data only.
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadRow } from "../delegation.js";
import { TasksClient } from "../tasks.js";
import {
  classify,
  errCode,
  hasMessageMarker,
  messageDescription,
  messageTitle,
  VizierMessages,
  VIZIER_MESSAGE_LABEL,
  type MessageEvent,
} from "../vizierMessages.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const PREFIX = "Masaq' | vizier";
const VIZ = "thr_viz";

const text = (t: string) => [{ type: "text", text: t }];
/** A test event knows which thread it is in (default: the vizier thread); `at` is when the host stamped it. */
type TestEvent = MessageEvent & { threadId?: string };
const evt = (seq: number, t: string, data: Partial<NonNullable<MessageEvent["data"]>> = {}, at = 2000, threadId?: string): TestEvent => ({
  id: threadId ? `evt_${threadId}_${seq}` : `evt_${seq}`,
  seq,
  createdAt: at,
  type: "client/turn/requested",
  data: { initiator: "user", senderThreadId: null, input: text(t), ...data },
  ...(threadId ? { threadId } : {}),
});
const row = (id: string, title: string, o: Partial<ThreadRow> = {}): ThreadRow => ({ id, title, pinnedAt: 1, archivedAt: null, deletedAt: null, ...o });

interface World {
  rows: ThreadRow[];
  events: TestEvent[];
  enabled: boolean;
  project: string;
  /** The host clock, in ms. The setting is first seen on at 1000; "new" test events are stamped 2000. */
  now: number;
  /** Runs inside the thread lookup, to put a message in the gap between activation and the first read. */
  onList?: () => void;
}

function build(r: Rig, w: World) {
  const logs: string[] = [];
  const make = () =>
    new VizierMessages({
      tasks: new TasksClient(r.fake, { now: () => r.env.clock.t }),
      store: r.svc.store,
      threads: {
        listRows: async () => {
          const rows = w.rows;
          const hook = w.onList;
          w.onList = undefined;
          hook?.();
          return rows;
        },
        events: async (a) => {
          const all = w.events.filter((e) => (e.threadId ?? VIZ) === a.threadId && (a.afterSeq === undefined || e.seq > a.afterSeq) && (a.beforeSeq === undefined || e.seq < a.beforeSeq));
          const sorted = [...all].sort((x, y) => (a.order === "asc" ? x.seq - y.seq : y.seq - x.seq));
          return sorted.slice(0, a.limit);
        },
      },
      config: () => ({ enabled: w.enabled, titlePrefix: PREFIX, project: w.project }),
      now: () => w.now,
      log: { info: (m) => logs.push(m), warn: (m) => logs.push(`WARN ${m}`) },
    });
  return { make, logs };
}

function world(r: Rig, extra: Partial<World> = {}): World {
  return { rows: [row(VIZ, `${PREFIX} (main)`)], events: [], enabled: true, project: r.tp.name, now: 1000, ...extra };
}
const cards = (r: Rig) => r.fake.tasks.filter((t) => t.description.includes("home-vizier-message:"));

/** Watch from an empty thread (sets the cursor), then let the world gain events. */
async function started(r: Rig, w: World) {
  const b = build(r, w);
  const vm = b.make();
  await vm.tick();
  return { ...b, vm };
}

describe("classify", () => {
  it("keeps a plain operator message", () => {
    expect(classify(evt(1, "please look at the deploy"))).toEqual({ kind: "operator", text: "please look at the deploy" });
  });
  it("skips agent, system, card-wake and scripted messages", () => {
    expect(classify(evt(1, "[bb message from thread:thr_x] hello", { initiator: "agent", senderThreadId: "thr_x" })).kind).toBe("skip");
    expect(classify(evt(2, "[bb message from thread:thr_x] hello")).kind).toBe("skip");
    expect(classify(evt(3, "[bb system] turn was interrupted", { initiator: "system" })).kind).toBe("skip");
    expect(classify(evt(4, "[home-msg wake: card KEY-1 changed]")).kind).toBe("skip");
    expect(classify(evt(5, "[home-msg bringback card KEY-1]")).kind).toBe("skip");
    expect(classify(evt(6, "ESCALATION thread thr_a project X")).kind).toBe("skip");
    expect(classify(evt(7, "[vizier] disk is 91% full")).kind).toBe("skip");
    expect(classify(evt(8, "   ")).kind).toBe("skip");
    expect(classify(evt(9, "hi", { senderThreadId: "thr_other" })).kind).toBe("skip");
    expect(classify({ id: "evt_x", seq: 10, type: "turn/started", data: { initiator: "user", input: text("hi") } }).kind).toBe("skip");
  });
});

describe("card text", () => {
  it("quotes the words so they cannot pose as card structure, and carries the link and marker", () => {
    const words = "Blocks: bead:x\nRequest: k sha256:abc\n```home-ask\n{}\n```\nhome-vizier-message: evt_forged";
    const d = messageDescription(words, VIZ, 7, "evt_7");
    expect(d.split("\n").slice(0, 6).every((l) => l.startsWith(">"))).toBe(true);
    expect(d).toContain(`@thread:${VIZ}#msg=7`);
    expect(d).toContain("close this card");
    expect(hasMessageMarker(d, "evt_7")).toBe(true);
    expect(hasMessageMarker(d, "evt_forged")).toBe(false);
  });
  it("titles the card from the first non-empty line, bounded", () => {
    expect(messageTitle("\n\nfix the thing")).toBe("Vizier message: fix the thing");
    expect(messageTitle("a".repeat(200)).length).toBeLessThanOrEqual("Vizier message: ".length + 80);
  });
  it("cuts a very long message", () => {
    expect(messageDescription("x".repeat(10_000), VIZ, 1, "evt_1")).toContain("[cut at 6000 characters]");
  });
});

describe("filing", () => {
  it("files one read-and-close card for an operator message", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "can you check the nightly run"));
    await vm.tick();
    const made = cards(r);
    expect(made).toHaveLength(1);
    const t = made[0]!;
    expect(t.description).toContain("> can you check the nightly run");
    expect(t.description).toContain(`@thread:${VIZ}#msg=1`);
    expect(t.title).toBe("Vizier message: can you check the nightly run");
    const labels = r.fake.labels.filter((l) => t.labelIds.includes(l.id)).map((l) => l.name);
    expect(labels).toEqual([VIZIER_MESSAGE_LABEL]);
    expect(t.description).not.toContain("home-ask");
    expect(t.description).not.toMatch(/^Request:/m);
  });

  it("files nothing for agent, system and card-wake messages, then files the operator's next one", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(
      evt(1, "[bb message from thread:thr_x] done", { initiator: "agent", senderThreadId: "thr_x" }),
      evt(2, "[bb system] interrupted", { initiator: "system" }),
      evt(3, "[home-msg wake: card KEY-9 changed]"),
    );
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    w.events.push(evt(4, "a real message"));
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("files one card per message, in order, and none twice on later ticks", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "first"), evt(2, "second"));
    await vm.tick();
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: first", "Vizier message: second"]);
  });

  it("makes no backfill: messages before the setting was first on get no card", async () => {
    const r = rig();
    const w = world(r, { events: [evt(1, "old one", {}, 500), evt(2, "old two", {}, 900)] });
    const { vm } = await started(r, w);
    expect(cards(r)).toHaveLength(0);
    w.events.push(evt(3, "new one"));
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: new one"]);
  });
});

describe("idempotency", () => {
  it("does not file a replayed or duplicate event twice", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "once"));
    await vm.tick();
    // the same event comes back, as from a replay, with a stale cursor
    r.svc.store.setSetting("vizierMessageState", JSON.stringify({ ...JSON.parse(r.svc.store.setting("vizierMessageState")!), seq: 0 }));
    await vm.tick();
    // and the same id again under a new seq
    w.events.push({ ...evt(2, "once"), id: "evt_1" });
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("makes no second card when the process dies after filing and before the record", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "crash me"));
    // the card is created, then the write that records it fails, as if the process had died
    const spy = vi.spyOn(r.svc.store, "atomically").mockImplementation(() => {
      throw new Error("crash before record");
    });
    await vm.tick();
    spy.mockRestore();
    expect(cards(r)).toHaveLength(1);
    expect(r.svc.store.setting("vizierMessageFiled")).toBeUndefined();
    // the card was closed meanwhile; the marker search still finds it
    cards(r)[0]!.status = "done";
    // restart (after the retry wait): a new instance over the same store and the same tasks
    w.now += 60_000;
    const again = build(r, w).make();
    await again.tick();
    expect(cards(r)).toHaveLength(1);
    expect(JSON.parse(r.svc.store.setting("vizierMessageFiled")!)).toEqual({ evt_1: cards(r)[0]!.id });
  });

  it("finds the card after a lost createTask response instead of making a second", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "lost response"));
    r.fake.lostResponses.push({ method: "createTask", nth: 1 });
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(logs.some((l) => l.startsWith("WARN") && l.includes("not filed"))).toBe(true);
    w.now += 60_000;
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("does not run two passes at once into two cards", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "double"));
    await Promise.all([vm.tick(), vm.tick(), vm.tick()]);
    expect(cards(r)).toHaveLength(1);
  });
});

describe("which thread", () => {
  it("files nothing when no thread matches, and says why", async () => {
    const r = rig();
    const w = world(r, { rows: [row("thr_a", "Something else")], events: [evt(1, "hello", {}, 500)] });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(2, "hello again"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain("no pinned, unarchived thread");
  });

  it("files nothing when two threads match, and says why", async () => {
    const r = rig();
    const w = world(r, { rows: [row("thr_a", `${PREFIX} one`), row("thr_b", `${PREFIX} two`)] });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain("2 pinned, unarchived threads match");
  });

  it("ignores an unpinned, archived or deleted thread with the prefix", async () => {
    const r = rig();
    const w = world(r, {
      rows: [row(VIZ, `${PREFIX} (main)`), row("thr_b", `${PREFIX} old`, { archivedAt: 5 }), row("thr_c", `${PREFIX} gone`, { deletedAt: 5 }), row("thr_d", `${PREFIX} loose`, { pinnedAt: null })],
    });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(cards(r)[0]!.description).toContain(`@thread:${VIZ}#msg=1`);
  });

  it("files nothing while no project is configured", async () => {
    const r = rig();
    const w = world(r, { project: "" });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain("no tasks project is configured");
  });

  it("does not file into a project that does not exist", async () => {
    const r = rig();
    const w = world(r, { project: "NOPE" });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain('no tasks project matches "NOPE"');
  });
});

describe("the setting", () => {
  it("files nothing and reads no thread while it is off", async () => {
    const r = rig();
    const w = world(r, { enabled: false });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(r.svc.store.setting("vizierMessageState")).toBeUndefined();
    expect(r.fake.calls).toHaveLength(0);
  });

  it("files only what arrives after it is turned on", async () => {
    const r = rig();
    const w = world(r, { enabled: false });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "typed while off", {}, 1500));
    await vm.tick();
    w.now = 3000;
    w.enabled = true;
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    w.now = 3500;
    w.events.push(evt(2, "typed while on", {}, 3200));
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed while on"]);
  });
});

type TestWatch = { seq: number | null; boundary?: number; scan?: number; retry?: { id: string; tries: number; next: number }; closedAt?: number };
type TestState = { active: boolean; since: number; threadId: string | null; threads: Record<string, TestWatch> };
const rawState = (r: Rig) => JSON.parse(r.svc.store.setting("vizierMessageState")!) as TestState;
/** The state with the selected thread's cursor and retry lifted to the top, for the plain assertions. */
const state = (r: Rig) => {
  const s = rawState(r);
  const w: TestWatch = (s.threadId ? s.threads[s.threadId] : undefined) ?? { seq: null };
  return { ...s, seq: w.seq, retry: w.retry };
};
const SECRET = "SECRET-OPERATOR-WORDS";
/** Make the next n calls of a method fail with an error that echoes the operator's words. */
const failN = (r: Rig, method: string, n: number) => {
  const done = r.fake.callsOf(method).length;
  for (let i = 1; i <= n; i++) r.fake.failures.push({ method, nth: done + i, error: new Error(`${method} echoed: ${SECRET}`) });
};

describe("no message is lost to a transient failure", () => {
  it("keeps retrying a message that will not file, with a persisted backoff, and files it when the outage ends", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, `needs a card ${SECRET}`), evt(2, "and a later one"));
    failN(r, "createTask", 14); // far past any give-up count
    await vm.tick();
    expect(state(r).seq).toBe(0);
    expect(state(r).retry).toMatchObject({ id: "evt_1", tries: 1 });
    // inside the wait nothing is tried again, in this process or after a restart
    const calls = r.fake.callsOf("createTask").length;
    await vm.tick();
    await build(r, w).make().tick();
    expect(r.fake.callsOf("createTask").length).toBe(calls);
    // the wait grows and is capped
    for (let i = 0; i < 13; i++) {
      w.now += 11 * 60_000;
      await vm.tick();
    }
    expect(cards(r)).toHaveLength(0);
    expect(state(r).seq).toBe(0);
    expect(state(r).retry!.tries).toBe(14);
    expect(state(r).retry!.next - w.now).toBeLessThanOrEqual(10 * 60_000);
    expect(logs.filter((l) => l.includes("STUCK on message 1"))).toHaveLength(1); // at try 5, then every 10th
    // the outage ends
    w.now += 11 * 60_000;
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual([expect.stringContaining("needs a card"), "Vizier message: and a later one"]);
    expect(state(r).retry).toBeUndefined();
    expect(state(r).seq).toBe(2);
  });

  it("does not move past a message when the marker search keeps failing", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "search me"));
    failN(r, "listTasks", 12);
    for (let i = 0; i < 12; i++) {
      await vm.tick();
      w.now += 11 * 60_000;
    }
    expect(cards(r)).toHaveLength(0);
    expect(state(r).seq).toBe(0);
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(state(r).seq).toBe(1);
  });

  it("logs the failed step and error class, never the error text", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, `private ${SECRET}`));
    failN(r, "createTask", 2);
    await vm.tick();
    // a pass-level failure too
    w.now += 60_000;
    failN(r, "listProjects", 1);
    await vm.tick();
    const all = logs.join("\n");
    expect(all).not.toContain(SECRET);
    expect(all).toMatch(/not filed at "create the card" \(try 1/);
    expect(all).toContain("tasks.createTask failed");
    expect(all).toMatch(/pass failed at "list projects": tasks\.listProjects failed/);
  });

  it("polls less often while passes keep failing, and normally again after one works", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    expect(vm.pollDelayMs(15_000)).toBe(15_000);
    failN(r, "listProjects", 3);
    await vm.tick();
    await vm.tick();
    expect(vm.pollDelayMs(15_000)).toBe(60_000);
    await vm.tick();
    await vm.tick();
    expect(vm.pollDelayMs(15_000)).toBe(15_000);
  });

  it("searches for markers once per pass, not once per message", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "a"), evt(2, "b"), evt(3, "c"));
    await vm.tick();
    expect(cards(r)).toHaveLength(3);
    expect(r.fake.callsOf("listTasks")).toHaveLength(1);
  });
});

describe("the watch boundary", () => {
  it("starts a fresh watch each time the setting goes on, and resumes after an ordinary restart", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "first"));
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    // off, reload, a message arrives while off, on again
    w.enabled = false;
    await vm.tick();
    expect(state(r).active).toBe(false);
    w.events.push(evt(2, "typed while off", {}, 3000));
    w.now = 4000;
    w.enabled = true;
    const again = build(r, w).make();
    await again.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: first"]);
    // an ordinary enabled restart resumes: a message that came in while the plugin was down gets its card
    w.events.push(evt(3, "typed during a restart", {}, 5000));
    w.now = 6000;
    await build(r, w).make().tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: first", "Vizier message: typed during a restart"]);
  });

  it("gives a card to a message that arrives between activation and the first read", async () => {
    const r = rig();
    const w = world(r, { events: [evt(1, "history", {}, 500)] });
    w.onList = () => w.events.push(evt(2, "arrived during discovery", {}, 1001));
    const vm = build(r, w).make();
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: arrived during discovery"]);
  });

  it("gives a card to a message that arrives in the new thread before its first poll after a rotation", async () => {
    const r = rig();
    const w = world(r, { events: [evt(1, "in the old thread", {}, 500, VIZ)] });
    const { vm } = await started(r, w);
    expect(state(r).threadId).toBe(VIZ);
    // the old thread is archived and a new one takes the title; its messages are stamped by the host
    w.rows = [row(VIZ, `${PREFIX} (old)`, { archivedAt: 2000 }), row("thr_new", `${PREFIX} (new)`, { pinnedAt: 2500 })];
    w.events.push(
      evt(1, "typed before it was the vizier thread", {}, 2000, "thr_new"),
      evt(2, "typed once it was", {}, 2600, "thr_new"),
    );
    w.now = 3000;
    await vm.tick();
    expect(state(r).threadId).toBe("thr_new");
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed once it was"]);
    expect(cards(r)[0]!.description).toContain("@thread:thr_new#msg=2");
    w.events.push(evt(3, "and then another", {}, 3100, "thr_new"));
    await vm.tick();
    expect(cards(r)).toHaveLength(2);
  });
});

describe("retried turns", () => {
  const retry = { requestId: "req_2", retryOfRequestId: "req_1", retryAttempt: 1 };
  it("skips an event shaped like a retry of an earlier request", () => {
    const e = evt(2, "same words", retry);
    (e.data as Record<string, unknown>).source = "tell";
    expect(classify(e)).toEqual({ kind: "skip", why: "retry of an earlier request" });
  });
  it("files the original once even though the retry carries the same input", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "please do the thing", { requestId: "req_1" }), evt(2, "please do the thing", retry));
    await vm.tick();
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(state(r).seq).toBe(2);
  });
  it("treats a second event with an already-filed request id as the same message", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "once", { requestId: "req_9" }));
    await vm.tick();
    w.events.push(evt(2, "once", { requestId: "req_9" }));
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });
});

describe("finding where to start in a long thread", () => {
  // PAGE is 500 and a pass reads 20 pages, so 10,000 events newer than the boundary is exactly the scan limit.
  const LIMIT = 10_000;
  async function run(newer: number) {
    const r = rig();
    // seq 1 is history (before the activation); seqs 2..newer+1 are newer. Only the oldest and newest are messages.
    const events: TestEvent[] = [evt(1, "history", {}, 500)];
    for (let s = 2; s <= newer + 1; s++) {
      const first = s === 2;
      const last = s === newer + 1;
      events.push(first || last ? evt(s, first ? "oldest of the backlog" : "newest of the backlog", {}, 2000 + s) : evt(s, "agent turn", { initiator: "agent" }, 2000 + s));
    }
    const w = world(r, { events });
    const vm = build(r, w).make();
    await vm.tick();
    const afterFirst = rawState(r).threads[VIZ];
    const passes = [afterFirst];
    for (let i = 0; i < 6; i++) {
      w.now += 60_000;
      await vm.tick();
    }
    return { r, afterFirst, passes, titles: cards(r).map((t) => t.title) };
  }

  it("finds the boundary within one pass when the boundary is the very last event the pass reads", async () => {
    const { afterFirst, titles } = await run(LIMIT - 1);
    expect(afterFirst!.scan).toBeUndefined();
    expect(titles).toEqual(["Vizier message: oldest of the backlog", "Vizier message: newest of the backlog"]);
  });
  it("does not skip the backlog when it is exactly the scan limit", async () => {
    const { r, afterFirst, titles } = await run(LIMIT);
    // the first pass ran out of pages: nothing is claimed, only the progress is kept
    expect(afterFirst!.seq).toBeNull();
    expect(afterFirst!.scan).toBe(2);
    expect(titles).toEqual(["Vizier message: oldest of the backlog", "Vizier message: newest of the backlog"]);
    expect(state(r).seq).toBe(LIMIT + 1);
    expect(rawState(r).threads[VIZ]!.scan).toBeUndefined();
  });
  it("does not skip the backlog when it is past the scan limit, and keeps its progress across a restart", async () => {
    const r = rig();
    const events: TestEvent[] = [evt(1, "history", {}, 500)];
    for (let s = 2; s <= LIMIT + 6; s++) events.push(s === 2 ? evt(s, "oldest of the backlog", {}, 2002) : s === LIMIT + 6 ? evt(s, "newest of the backlog", {}, 9000) : evt(s, "agent turn", { initiator: "agent" }, 2000 + s));
    const w = world(r, { events });
    await build(r, w).make().tick();
    const first = rawState(r).threads[VIZ]!;
    expect(first.seq).toBeNull();
    expect(first.scan).toBeGreaterThan(5);
    expect(cards(r)).toHaveLength(0);
    // a restart picks up the scan where it stopped, with the same boundary
    for (let i = 0; i < 6; i++) {
      w.now += 60_000;
      await build(r, w).make().tick();
    }
    expect(rawState(r).threads[VIZ]!.boundary).toBe(first.boundary);
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: oldest of the backlog", "Vizier message: newest of the backlog"]);
  });
});

describe("a failed message survives a change of vizier thread", () => {
  const NEW = "thr_new";
  const rotate = (w: World) => {
    w.rows = [row(VIZ, `${PREFIX} (old)`, { archivedAt: 3000 }), row(NEW, `${PREFIX} (new)`, { pinnedAt: 3500 })];
  };

  it("retries it after the rotation and after a restart, and files nothing the old thread gets later", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "the one that failed", {}, 2000), evt(2, "behind it, same thread", {}, 2100));
    failN(r, "createTask", 1);
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(rawState(r).threads[VIZ]!.retry).toMatchObject({ id: "evt_1", tries: 1 });
    // the vizier thread changes while the message is waiting out its backoff
    rotate(w);
    w.now = 4000;
    w.events.push(evt(1, "typed in the new thread", {}, 4100, NEW));
    await vm.tick();
    expect(rawState(r).threadId).toBe(NEW);
    expect(rawState(r).threads[VIZ]!.retry).toMatchObject({ id: "evt_1" }); // still owed, not dropped
    expect(rawState(r).threads[VIZ]!.closedAt).toBe(4000);
    // the process restarts, the backoff ends: the old message is filed, then the one behind it, and the new thread's own
    w.events.push(evt(3, "typed in the old thread after the change", {}, 4200));
    w.now = 4000 + 60_000;
    await build(r, w).make().tick();
    expect(cards(r).map((t) => t.title).sort()).toEqual(["Vizier message: behind it, same thread", "Vizier message: the one that failed", "Vizier message: typed in the new thread"]);
    const old = cards(r).find((t) => t.title.endsWith("the one that failed"))!;
    expect(old.description).toContain(`@thread:${VIZ}#msg=1`);
    // the old thread is finished with and forgotten
    expect(Object.keys(rawState(r).threads)).toEqual([NEW]);
    // and nothing is filed twice
    await build(r, w).make().tick();
    expect(cards(r)).toHaveLength(3);
  });

  it("still files it when the process restarts before it ever sees the rotation, and across an off and on", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "the one that failed", {}, 2000));
    failN(r, "createTask", 1);
    await vm.tick();
    rotate(w);
    w.now = 4000;
    await build(r, w).make().tick(); // restart; sees the rotation inside its backoff
    // switched off and on again: the new watch is fresh, but the owed message is carried over
    w.enabled = false;
    w.now = 5000;
    await build(r, w).make().tick();
    w.enabled = true;
    w.now = 5000 + 60_000;
    await build(r, w).make().tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: the one that failed"]);
    expect(cards(r)[0]!.description).toContain(`@thread:${VIZ}#msg=1`);
  });

  it("does not let a message that cannot be filed in the old thread hold up the new one", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "stuck", {}, 2000));
    failN(r, "createTask", 1);
    await vm.tick();
    rotate(w);
    w.now = 4000;
    w.events.push(evt(1, "fresh in the new thread", {}, 4100, NEW));
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: fresh in the new thread"]);
  });
});

describe("what a log line says about an error", () => {
  it("names a known class and nothing else", () => {
    class Hostile extends Error {
      override name = `Hostile ${SECRET}`;
    }
    expect(errCode(new Hostile("x"))).toBe("Error");
    expect(errCode(Object.assign(new Hostile("x"), { code: "E_BUSY" }))).toBe("Error E_BUSY");
    expect(errCode(new TypeError(SECRET))).toBe("TypeError");
    expect(errCode(Object.assign(new Error(SECRET), { name: "TasksError" }))).toBe("TasksError");
    expect(errCode(`${SECRET}`)).toBe("string");
  });
});
