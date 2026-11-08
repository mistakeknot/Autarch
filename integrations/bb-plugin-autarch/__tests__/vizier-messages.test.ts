// A card for every plain operator message in the vizier thread (bead mk-ppvma.4). Synthetic data only.
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ThreadRow } from "../delegation.js";
import { TasksClient } from "../tasks.js";
import {
  classify,
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
const evt = (seq: number, t: string, data: Partial<NonNullable<MessageEvent["data"]>> = {}): MessageEvent => ({
  id: `evt_${seq}`,
  seq,
  type: "client/turn/requested",
  data: { initiator: "user", senderThreadId: null, input: text(t), ...data },
});
const row = (id: string, title: string, o: Partial<ThreadRow> = {}): ThreadRow => ({ id, title, pinnedAt: 1, archivedAt: null, deletedAt: null, ...o });

interface World {
  rows: ThreadRow[];
  events: MessageEvent[];
  enabled: boolean;
  project: string;
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
          const all = w.events.filter((e) => (a.afterSeq === undefined ? true : e.seq > a.afterSeq));
          const sorted = [...all].sort((x, y) => (a.order === "asc" ? x.seq - y.seq : y.seq - x.seq));
          return sorted.slice(0, a.limit);
        },
      },
      config: () => ({ enabled: w.enabled, titlePrefix: PREFIX, project: w.project }),
      log: { info: (m) => logs.push(m), warn: (m) => logs.push(`WARN ${m}`) },
    });
  return { make, logs };
}

function world(r: Rig, extra: Partial<World> = {}): World {
  return { rows: [row(VIZ, `${PREFIX} (main)`)], events: [], enabled: true, project: r.tp.name, ...extra };
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
    const w = world(r, { events: [evt(1, "old one"), evt(2, "old two")] });
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
    r.svc.store.setSetting("vizierMessageCursor", JSON.stringify({ threadId: VIZ, seq: 0 }));
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
    // restart: a new instance over the same store and the same tasks
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
    const w = world(r, { rows: [row("thr_a", "Something else")], events: [evt(1, "hello")] });
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
    expect(r.svc.store.setting("vizierMessageCursor")).toBeUndefined();
  });

  it("files only what arrives after it is turned on", async () => {
    const r = rig();
    const w = world(r, { enabled: false });
    const { vm } = await started(r, w);
    w.events.push(evt(1, "typed while off"));
    await vm.tick();
    w.enabled = true;
    await vm.tick();
    expect(cards(r)).toHaveLength(0);
    w.events.push(evt(2, "typed while on"));
    await vm.tick();
    expect(cards(r).map((t) => t.title)).toEqual(["Vizier message: typed while on"]);
  });
});
