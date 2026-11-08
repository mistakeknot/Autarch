// A card for every plain operator message in the vizier thread: the stateless check (bead mk-ppvma.4).
// Synthetic data only.
//
// How two overlapping runs cannot file one message twice: every run goes through one process-wide queue
// (`singleFlight`), so a run's marker search and its card creation are never interleaved with another run's, in the
// same instance or in a second instance built by a plugin reload. Across processes and restarts the only guard is the
// marker line on the card itself, searched across every status (closed cards included) before a card is made. The
// "overlapping runs" tests below drive both.
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadRow } from "../delegation.js";
import { TasksClient } from "../tasks.js";
import {
  classify,
  errCode,
  gapMarker,
  hasMessageMarker,
  messageDescription,
  messageTitle,
  VizierMessages,
  VIZIER_MESSAGE_LABEL,
  WINDOW_MS,
  type MessageEvent,
} from "../vizierMessages.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const PREFIX = "Masaq' | vizier";
const VIZ = "thr_viz";
const OLD = "thr_old";
const HOUR = 3_600_000;
const T0 = 400 * 24 * HOUR; // the host clock when the setting is first seen on
const SECRET = "SECRET-OPERATOR-WORDS";
/** The host's own page cap for thread events, written out here so a change to the plugin's constant cannot move it. */
const HOST_EVENT_LIMIT = 100;

const text = (t: string) => [{ type: "text", text: t }];
/** A test event knows which thread it is in (default: the vizier thread); `at` is when the host stamped it. */
type TestEvent = MessageEvent & { threadId?: string };
const evt = (seq: number, t: string, data: Partial<NonNullable<MessageEvent["data"]>> = {}, at = T0 + 2000, threadId?: string): TestEvent => ({
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
  /** The host clock, in ms. */
  now: number;
  /** Set to make the next event reads fail. */
  failReads?: boolean;
  /** Every event read, when a test wants to see what was fetched. */
  readLog?: { threadId: string; beforeSeq?: number }[];
  /** Passed to the watcher as the time the service first saw the setting on. */
  observedAt?: number;
  /** Runs inside the first event read, before it returns: a thread that changes while the read is pending. */
  duringRead?: () => void;
}

function build(r: Rig, w: World) {
  const logs: string[] = [];
  const make = () =>
    new VizierMessages({
      tasks: new TasksClient(r.fake, { now: () => r.env.clock.t }),
      store: r.svc.store,
      threads: {
        listRows: async () => w.rows,
        events: async (a) => {
          w.readLog?.push({ threadId: a.threadId, ...(a.beforeSeq !== undefined ? { beforeSeq: a.beforeSeq } : {}) });
          const hook = w.duringRead;
          w.duringRead = undefined;
          await Promise.resolve();
          hook?.();
          if (w.failReads) throw new Error(`thread ${a.threadId} cannot be read ${SECRET}`);
          const all = w.events.filter((e) => (e.threadId ?? VIZ) === a.threadId && (a.beforeSeq === undefined || e.seq < a.beforeSeq));
          // The host rejects a page above its cap whole (threads.ts: "Thread event limit cannot exceed 100").
          if (a.limit > HOST_EVENT_LIMIT) throw new Error(`invalid_request: Thread event limit cannot exceed ${HOST_EVENT_LIMIT}`);
          return [...all].sort((x, y) => y.seq - x.seq).slice(0, a.limit);
        },
      },
      config: () => ({ enabled: w.enabled, titlePrefix: PREFIX, project: w.project }),
      now: () => w.now,
      ...(w.observedAt !== undefined ? { observedAt: w.observedAt } : {}),
      log: { info: (m) => logs.push(m), warn: (m) => logs.push(`WARN ${m}`) },
    });
  return { make, logs };
}

function world(r: Rig, extra: Partial<World> = {}): World {
  return { rows: [row(VIZ, `${PREFIX} (main)`)], events: [], enabled: true, project: r.tp.name, now: T0, ...extra };
}
const cards = (r: Rig) => r.fake.tasks.filter((t) => t.description.includes("home-vizier-message:"));
const gaps = (r: Rig) => r.fake.tasks.filter((t) => t.description.includes("home-vizier-gap:"));
const saved = (r: Rig) => JSON.parse(r.svc.store.setting("vizierMessagePoll") ?? "null") as { onAt: number; lastRunAt?: number } | null;

/** Switch on at T0 with an empty thread (records the switch-on time), then move the clock on. */
async function started(r: Rig, w: World) {
  const b = build(r, w);
  const vm = b.make();
  await vm.tick();
  w.now = T0 + 10_000;
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
    expect(classify(evt(10, "retry", { retryOfRequestId: "req_1" })).kind).toBe("skip");
    expect(classify(evt(11, "system note", { systemMessageKind: "interrupt" })).kind).toBe("skip");
    expect(classify({ id: "evt_x", seq: 12, type: "turn/started", data: { initiator: "user", input: text("hi") } }).kind).toBe("skip");
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
    expect(t.description).toContain("Read it, then close this card.");
    expect(t.title).toBe("Vizier message: can you check the nightly run");
    expect(r.fake.labels.filter((l) => t.labelIds.includes(l.id)).map((l) => l.name)).toEqual([VIZIER_MESSAGE_LABEL]);
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

  it("files one card per message, oldest first, across more than one page of events", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    for (let i = 1; i <= 230; i++) w.events.push(i === 1 || i === 230 ? evt(i, i === 1 ? "first" : "last") : evt(i, "[bb system] noise", { initiator: "system" }));
    w.readLog = [];
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: first", "Vizier message: last"]);
    expect(w.readLog.length).toBeGreaterThan(1); // the window was paged
  });

  it("never asks the host for a page above its cap of 100 events", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "needs a card"));
    w.readLog = [];
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: needs a card"]);
    expect(logs.join("\n")).not.toContain("invalid_request");
  });

  it("reaches a message 4900 events back, so 100-event pages cover as much of the window as 200-event pages did", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "the oldest"));
    for (let i = 2; i <= 4900; i++) w.events.push(evt(i, "[bb system] noise", { initiator: "system" }, T0 + 20_000));
    w.now = T0 + 30_000;
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: the oldest"]);
    expect(logs.join("\n")).not.toContain("more than");
  });

  it("files nothing for a message without a timestamp (when in doubt, no card)", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    const e = evt(1, "no stamp");
    delete e.createdAt;
    w.events.push(e);
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
  });
});

describe("a second run, a restart and overlapping runs make no second card", () => {
  it("does not file the same message again on a second run over the same window", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "once"));
    await vm.tick();
    await vm.tick();
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("does not file again after a restart (a new instance, a new service), even if the card was closed", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "close me after reading"));
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    cards(r)[0]!.status = "done"; // the operator read it and closed it
    r.restart();
    w.now += 60_000;
    const again = build(r, w).make();
    await again.tick();
    expect(cards(r)).toHaveLength(1);
    cards(r)[0]!.status = "canceled";
    await again.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("makes no second card when the response to the create was lost (the next run finds the marker)", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "lost response"));
    r.fake.lostResponses.push({ method: "createTask", nth: 1 });
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(logs.some((l) => l.startsWith("WARN") && l.includes("not filed"))).toBe(true);
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });

  it("makes one card from overlapping runs, in one instance and across two instances", async () => {
    const r = rig();
    const w = world(r);
    const { vm, make } = await started(r, w);
    const second = make(); // as after a plugin reload that left the first still finishing a run
    w.events.push(evt(1, "double"));
    await Promise.all([vm.tick(), vm.tick(), second.tick(), vm.tick(), second.tick()]);
    expect(cards(r)).toHaveLength(1);
  });

  it("treats two events with the same request id as one message", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "same request", { requestId: "req_1" }), { ...evt(2, "same request", { requestId: "req_1" }), id: "evt_other" });
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });
});

describe("which thread", () => {
  it("files nothing when no thread matches, and says why", async () => {
    const r = rig();
    const w = world(r, { rows: [row("thr_a", "Something else")] });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello", {}, T0 + 5000, "thr_a"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain("no pinned, unarchived thread");
  });

  it("files nothing when two threads match, and says why", async () => {
    const r = rig();
    const w = world(r, { rows: [row("thr_a", `${PREFIX} one`), row("thr_b", `${PREFIX} two`)] });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello", {}, T0 + 5000, "thr_a"), evt(1, "hello", {}, T0 + 5000, "thr_b"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(logs.join("\n")).toContain("2 pinned, unarchived threads match");
    expect(w.readLog).toBeUndefined();
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

  it("files nothing while no project is configured, or the project does not exist", async () => {
    const r = rig();
    const w = world(r, { project: "" });
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(logs.join("\n")).toContain("no tasks project is configured");
    w.project = "NOPE";
    await vm.tick();
    expect(logs.join("\n")).toContain('no tasks project matches "NOPE"');
    expect(cards(r)).toHaveLength(0);
    w.project = r.tp.name;
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });
});

describe("limit (c): off until switched on, and the switch-on time", () => {
  it("files nothing, reads nothing and keeps no record while the setting is off", async () => {
    const r = rig();
    const w = world(r, { enabled: false });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "hello"));
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(saved(r)).toBeNull();
    expect(r.fake.calls).toHaveLength(0);
    expect(w.readLog).toBeUndefined();
  });

  it("when turned off after an enabled run, reads only its own settings key, resets it, and touches no thread or task", async () => {
    const r = rig();
    const w = world(r, { rows: [row(OLD, `${PREFIX} (v1)`)] });
    const { vm } = await started(r, w);
    expect(saved(r)).toEqual({ onAt: T0, lastRunAt: T0 });
    w.enabled = false;
    w.readLog = [];
    const calls = r.fake.calls.length;
    const reads = vi.spyOn(r.svc.store, "setting");
    const writes = vi.spyOn(r.svc.store, "setSetting");
    await vm.tick();
    expect(reads.mock.calls).toEqual([["vizierMessagePoll"]]);
    expect(writes.mock.calls).toEqual([["vizierMessagePoll", "null"]]);
    expect(saved(r)).toBeNull();
    expect(w.readLog).toHaveLength(0);
    expect(r.fake.calls).toHaveLength(calls);
    expect(cards(r)).toHaveLength(0);
    // and a further disabled run writes nothing at all
    reads.mockClear();
    writes.mockClear();
    await vm.tick();
    expect(reads.mock.calls).toEqual([["vizierMessagePoll"]]);
    expect(writes.mock.calls).toEqual([]);
  });

  it("records the switch-on time, and never files a message at or before it", async () => {
    const r = rig();
    const w = world(r, { events: [evt(1, "typed an hour before", {}, T0 - HOUR), evt(2, "typed exactly at switch-on", {}, T0)] });
    const { vm } = await started(r, w);
    expect(saved(r)?.onAt).toBe(T0);
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    w.events.push(evt(3, "typed after", {}, T0 + 1));
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed after"]);
  });

  it("dates the switch-on from when the service first saw the setting on, so a message typed during start-up counts", async () => {
    const r = rig();
    const w = world(r, { observedAt: T0 - 5000, events: [evt(1, "before the setting was seen", {}, T0 - 9000), evt(2, "typed while the plugin came up", {}, T0 - 2000)] });
    const vm = build(r, w).make();
    await vm.tick();
    expect(saved(r)?.onAt).toBe(T0 - 5000);
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed while the plugin came up"]);
  });

  it("files only what arrives after the setting is turned on again, not what was typed while it was off", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "typed while on", {}, T0 + 12_000));
    w.now = T0 + 20_000;
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    w.enabled = false;
    w.now = T0 + 30_000;
    await vm.tick();
    expect(saved(r)).toBeNull();
    w.events.push(evt(2, "typed while off", {}, T0 + 40_000));
    w.now = T0 + 60_000;
    w.enabled = true;
    await vm.tick();
    expect(saved(r)?.onAt).toBe(T0 + 60_000);
    expect(cards(r)).toHaveLength(1);
    w.events.push(evt(3, "typed after turning it on", {}, T0 + 61_000));
    w.now = T0 + 70_000;
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed while on", "Vizier message: typed after turning it on"]);
  });
});

describe("limit (a): an old vizier thread after it is replaced", () => {
  it("files nothing typed into the old thread after it was replaced, and still files the new thread's", async () => {
    const r = rig();
    const w = world(r, { rows: [row(OLD, `${PREFIX} (v1)`)] });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "typed in the old thread while it was current", {}, T0 + 12_000, OLD));
    w.now = T0 + 15_000;
    await vm.tick();
    expect(cards(r).map((t) => t.description)).toEqual([expect.stringContaining(`@thread:${OLD}#msg=1`)]);
    // the old thread is replaced: archived, and a new pinned thread takes the prefix
    w.rows = [row(OLD, `${PREFIX} (v1)`, { archivedAt: 9 }), row(VIZ, `${PREFIX} (v2)`)];
    w.events.push(evt(2, "typed in the old thread after the replacement", {}, T0 + 20_000, OLD), evt(1, "typed in the new thread", {}, T0 + 21_000));
    w.now = T0 + 30_000;
    w.readLog = [];
    await vm.tick();
    const made = cards(r).map((t) => t.title);
    expect(made).toEqual(["Vizier message: typed in the old thread while it was current", "Vizier message: typed in the new thread"]);
    expect(w.readLog.every((x) => x.threadId === VIZ)).toBe(true); // the old thread is never read again
  });
});

describe("limit (a), mid-run: the thread changes while its messages are being read", () => {
  it("files nothing and keeps the heartbeat when the thread is replaced during the read", async () => {
    const r = rig();
    const w = world(r, { rows: [row(OLD, `${PREFIX} (v1)`)] });
    const { vm, logs } = await started(r, w);
    const before = saved(r);
    w.events.push(evt(1, "typed in the old thread, after the replacement", {}, T0 + 12_000, OLD));
    w.now = T0 + 15_000;
    w.duringRead = () => {
      w.rows = [row(OLD, `${PREFIX} (v1)`, { archivedAt: 9 }), row(VIZ, `${PREFIX} (v2)`)];
    };
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(saved(r)).toEqual(before);
    expect(logs.join("\n")).toContain("the vizier thread changed while the messages were being read");
    // the next run reads the new thread only
    w.events.push(evt(1, "typed in the new thread", {}, T0 + 21_000));
    w.now = T0 + 30_000;
    w.readLog = [];
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed in the new thread"]);
    expect(w.readLog.every((x) => x.threadId === VIZ)).toBe(true);
    expect(saved(r)!.lastRunAt).toBe(T0 + 30_000);
  });

  it("files nothing and keeps the heartbeat when the thread is archived during the read and none replaces it", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    const before = saved(r);
    w.events.push(evt(1, "typed just before it was archived", {}, T0 + 12_000));
    w.now = T0 + 15_000;
    w.duringRead = () => {
      w.rows = [row(VIZ, `${PREFIX} (main)`, { archivedAt: 9 })];
    };
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(saved(r)).toEqual(before);
    expect(logs.join("\n")).toContain("the vizier thread changed");
  });

  it("files nothing and keeps the heartbeat when a second match appears during the read", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    const before = saved(r);
    w.events.push(evt(1, "typed while the selection turned ambiguous", {}, T0 + 12_000));
    w.now = T0 + 15_000;
    w.duringRead = () => {
      w.rows = [row(VIZ, `${PREFIX} (main)`), row("thr_b", `${PREFIX} (second)`)];
    };
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(saved(r)).toEqual(before);
    expect(logs.join("\n")).toContain("2 pinned, unarchived threads match");
    expect(logs.join("\n")).toContain("the vizier thread changed");
  });

  it("files normally when the selection does not change during the read", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "an ordinary message", {}, T0 + 12_000));
    w.now = T0 + 15_000;
    w.duringRead = () => {
      w.rows = [row(VIZ, `${PREFIX} (main)`), row("thr_x", "Something else")];
    };
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
  });
});

describe("limit (b): an outage longer than 24 hours", () => {
  it("does not file older messages, and files one gap card saying which stretch may have been missed", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    expect(saved(r)?.lastRunAt).toBe(T0);
    // the check is down for 30 hours; meanwhile the operator types
    w.events.push(evt(1, "typed early in the outage", {}, T0 + 2 * HOUR), evt(2, "typed late in the outage", {}, T0 + 29 * HOUR));
    w.now = T0 + 30 * HOUR;
    await vm.tick();
    // the message inside the last 24 hours is filed; the older one is not
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed late in the outage"]);
    expect(gaps(r)).toHaveLength(1);
    const g = gaps(r)[0]!;
    expect(g.description).toContain(gapMarker(T0));
    expect(g.description).toContain("may have been missed");
    expect(g.description).toMatch(/P[DS]T/); // times are given in the operator's zone
    expect(g.description).toContain(new Date(T0).toISOString().slice(0, 16)); // the start of the stretch (UTC after it)
    expect(g.description).toContain(new Date(T0 + 30 * HOUR - WINDOW_MS).toISOString().slice(0, 16)); // its end: where the 24 hours begin
    expect(g.description).toContain(`@thread:${VIZ}`);
    expect(g.labelIds).toHaveLength(1);
    expect(logs.join("\n")).toContain("filed");
    // and no more gap cards on later runs, in this instance or a restarted one
    w.now += 15_000;
    await vm.tick();
    r.restart();
    await build(r, w).make().tick();
    expect(gaps(r)).toHaveLength(1);
    expect(saved(r)?.lastRunAt).toBe(w.now);
  });

  it("files no gap card for a stretch shorter than 24 hours", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "typed during a short outage", {}, T0 + 2 * HOUR));
    w.now = T0 + 20 * HOUR;
    await vm.tick();
    expect(gaps(r)).toHaveLength(0);
    expect(cards(r)).toHaveLength(1); // still inside the window, so still filed
  });

  it("files one gap card per outage, not per run, and a later outage gets its own", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.now = T0 + 30 * HOUR;
    await vm.tick();
    await vm.tick();
    w.now += 15_000;
    await vm.tick();
    expect(gaps(r)).toHaveLength(1);
    w.now += 40 * HOUR;
    await vm.tick();
    await vm.tick();
    expect(gaps(r)).toHaveLength(2);
    expect(new Set(gaps(r).map((g) => g.description.split("\n").find((l) => l.startsWith("home-vizier-gap:")))).size).toBe(2);
  });

  it("holds the gap card back while a message in the window cannot be filed, and states it on a later run", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, "late in the outage", {}, T0 + 29 * HOUR));
    w.now = T0 + 30 * HOUR;
    r.fake.failures.push({ method: "createTask", nth: 1, error: new Error(`createTask echoed: ${SECRET}`) });
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    expect(gaps(r)).toHaveLength(0);
    expect(saved(r)?.lastRunAt).toBe(T0); // the heartbeat did not advance on an incomplete run
    w.now += 15_000;
    await vm.tick();
    expect(cards(r)).toHaveLength(1);
    expect(gaps(r)).toHaveLength(1);
    expect(gaps(r)[0]!.description).toContain(gapMarker(T0)); // still the original stretch, not a shorter one
  });

  it("does not advance the heartbeat when no vizier thread can be found, so a long silence becomes a gap", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.rows = [];
    w.now = T0 + 5 * HOUR;
    await vm.tick();
    expect(saved(r)?.lastRunAt).toBe(T0);
    w.rows = [row(VIZ, `${PREFIX} (main)`)];
    w.now = T0 + 30 * HOUR;
    await vm.tick();
    expect(gaps(r)).toHaveLength(1);
  });
});

describe("incomplete reads and failures", () => {
  it("does not advance the heartbeat when the window holds more than the read can cover, and says so", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    for (let i = 1; i <= 5200; i++) w.events.push(evt(i, "[bb system] noise", { initiator: "system" }, T0 + 20_000));
    w.now = T0 + 30_000;
    await vm.tick();
    expect(saved(r)?.lastRunAt).toBe(T0);
    expect(logs.join("\n")).toContain("more than 5000 messages");
  });

  it("retries a message whose card failed on the next run, once, and does not stop later runs", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    w.events.push(evt(1, `needs a card ${SECRET}`), evt(2, "and a later one"));
    r.fake.failures.push({ method: "createTask", nth: 1, error: new Error(`createTask echoed: ${SECRET}`) });
    await vm.tick();
    expect(cards(r)).toHaveLength(0); // the run stops at the first failure
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual([`Vizier message: needs a card ${SECRET}`, "Vizier message: and a later one"]);
  });

  it("files nothing and writes no heartbeat when the thread cannot be read", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.failReads = true;
    w.now = T0 + 20_000;
    await vm.tick();
    expect(saved(r)?.lastRunAt).toBe(T0);
    expect(logs.join("\n")).toContain('failed at "read messages"');
    expect(logs.join("\n")).not.toContain(SECRET);
  });

  it("logs the failed step and error class, never the error text", async () => {
    const r = rig();
    const w = world(r);
    const { vm, logs } = await started(r, w);
    w.events.push(evt(1, `private ${SECRET}`));
    r.fake.failures.push({ method: "createTask", nth: 1, error: new Error(`createTask echoed: ${SECRET}`) });
    await vm.tick();
    r.fake.failures.push({ method: "listProjects", nth: r.fake.callsOf("listProjects").length + 1, error: new Error(`listProjects echoed: ${SECRET}`) });
    await vm.tick();
    const all = logs.join("\n");
    expect(all).not.toContain(SECRET);
    expect(all).toMatch(/not filed at "create the card": tasks\.createTask failed/);
    expect(all).toMatch(/run failed at "list projects": tasks\.listProjects failed/);
  });

  it("polls less often while runs keep failing, and normally again after one works", async () => {
    const r = rig();
    const w = world(r);
    const { vm } = await started(r, w);
    expect(vm.pollDelayMs(15_000)).toBe(15_000);
    w.failReads = true;
    await vm.tick();
    await vm.tick();
    expect(vm.pollDelayMs(15_000)).toBe(60_000);
    w.failReads = false;
    await vm.tick();
    expect(vm.pollDelayMs(15_000)).toBe(15_000);
  });

  it("reads the error class only from a fixed set of names", () => {
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
