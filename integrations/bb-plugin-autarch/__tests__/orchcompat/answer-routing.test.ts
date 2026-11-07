import { afterEach, describe, expect, it } from "vitest";
import { WakeLoop } from "../../wakes.js";
import { archived, FakeSdk } from "../wakes-helpers.js";
import { cleanupEnvs, opened, rig, T1 } from "../card-rig.js";

afterEach(cleanupEnvs);

const HOUR = 3_600_000;
const wakeOf = (r: ReturnType<typeof rig>, decisionId: string) =>
  r.svc.store.obligationsFor(decisionId).find((o) => o.kind === "wake")!;

describe("answer routing: which thread a card's ruling goes to", () => {
  it("an unclaimed card with one agent comment routes to the thread that wrote it", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const row = r.cardRow(t.id);
    expect(row.asking_thread).toBe("thr_a");
    expect(row.routed_thread).toBe("thr_a");
    expect(row.routing_mode).toBe("thread");
    expect(row.state).toBe("open");
    expect(g1.generation).toBe(1);
  });

  it("a ruling creates a wake for the routed thread and the wake loop delivers it with the home-msg envelope", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const p = r.mkPick(g1.id);
    expect(p.ok).toBe(true);
    const wake = wakeOf(r, g1.id);
    expect(wake.recipient).toBe("thr_a");
    expect(wake.state).toBe("pending");
    expect(wake.payload).toBe(
      `Ruling on ${g1.id}: mk picked "Per project".\nQuestion: Collapse per project or per day?\nInstruction: Group per project, run tests, report.`,
    );
    const sdk = new FakeSdk();
    await new WakeLoop(r.svc, sdk).drain();
    expect(sdk.sent).toEqual([{ threadId: "thr_a", input: `[home-msg ${wake.op} #1] ${wake.payload}`, mode: "queue-if-active" }]);
    expect(wakeOf(r, g1.id).state).toBe("done");
  });

  it("a claim by another thread after the card opened does not move the route", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.fake.addComment(t.id, { threadId: "thr_claimer", createdAt: "2026-10-01T00:30:00.000Z" });
    r.advance(3 * HOUR);
    await r.poll();
    const row = r.cardRow(t.id);
    expect(row.routed_thread).toBe("thr_a");
    expect(row.state).toBe("open");
    expect(r.gens(t.id)).toHaveLength(1);
    r.mkPick(g1.id);
    expect(wakeOf(r, g1.id).recipient).toBe("thr_a");
  });

  it("with a claim already on the card at first sight, the earliest agent comment still wins", async () => {
    const r = rig();
    const t = r.card({ thread: "thr_owner" });
    r.fake.addComment(t.id, { threadId: "thr_claimer", createdAt: "2026-10-01T05:00:00.000Z" });
    await r.poll();
    expect(r.cardRow(t.id).routed_thread).toBe("thr_owner");
    expect(r.cardRow(t.id).asking_thread).toBe("thr_owner");
  });

  it("when the owner's comment is the first agent comment the card routes to the owner", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    r.fake.addComment(t.id, { threadId: "thr_owner", createdAt: T1 });
    r.fake.addComment(t.id, { threadId: "thr_helper", createdAt: "2026-10-01T00:00:09.000Z" });
    r.advance(HOUR);
    await r.poll();
    expect(r.cardRow(t.id).routed_thread).toBe("thr_owner");
  });

  it("the earliest comment is chosen by creation time, whatever order the comments were added in", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    r.fake.addComment(t.id, { threadId: "thr_late", createdAt: "2026-10-01T09:00:00.000Z" });
    r.fake.addComment(t.id, { threadId: "thr_early", createdAt: "2026-10-01T01:00:00.000Z" });
    r.advance(HOUR);
    await r.poll();
    expect(r.cardRow(t.id).routed_thread).toBe("thr_early");
  });

  it("comments with the same creation time are ordered by comment id", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    const first = r.fake.addComment(t.id, { threadId: "thr_one", createdAt: T1 });
    const second = r.fake.addComment(t.id, { threadId: "thr_two", createdAt: T1 });
    expect(first.id < second.id).toBe(true);
    r.advance(HOUR);
    await r.poll();
    expect(r.cardRow(t.id).routed_thread).toBe("thr_one");
  });

  it("user comments never route a card", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    r.fake.addComment(t.id, { kind: "user", threadId: null, createdAt: T1 });
    r.fake.addComment(t.id, { kind: "user", threadId: "thr_user_side", createdAt: "2026-10-01T00:00:02.000Z" });
    await r.poll();
    const row = r.cardRow(t.id);
    expect(row.state).toBe("observed");
    expect(row.routed_thread).toBeNull();
    expect(r.gens(t.id)).toHaveLength(0);
  });
});

describe("answer routing: a card nobody has claimed yet", () => {
  it("with no agent comment the card waits, then is called unroutable, and opens no generation", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("observed");
    expect(r.cardRow(t.id).display_reason).toBe("waiting for asking thread");
    expect(r.gens(t.id)).toHaveLength(0);
    r.advance(2 * HOUR);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("observed");
    expect(r.cardRow(t.id).display_reason).toBe("unroutable");
    expect(r.gens(t.id)).toHaveLength(0);
  });

  it("an agent comment that arrives later opens the card and routes it to that thread", async () => {
    const r = rig();
    const t = r.card({ thread: null });
    await r.poll();
    r.advance(2 * HOUR);
    await r.poll();
    expect(r.cardRow(t.id).display_reason).toBe("unroutable");
    r.fake.addComment(t.id, { threadId: "thr_late", createdAt: "2026-10-01T03:00:00.000Z" });
    r.advance(HOUR);
    await r.poll();
    const row = r.cardRow(t.id);
    expect(row.state).toBe("open");
    expect(row.routed_thread).toBe("thr_late");
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

describe("answer routing: the route is frozen when the card opens", () => {
  it("an agent comment dated before the asking thread's comment invalidates the generation", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.addComment(t.id, { threadId: "thr_early", createdAt: "2026-10-01T00:00:00.500Z" });
    r.advance(3 * HOUR);
    await r.poll();
    const row = r.cardRow(t.id);
    expect(row.asking_thread).toBe("thr_a");
    expect(row.routed_thread).toBe("thr_a");
    expect(row.state).toBe("closed");
    expect(row.display_reason).toBe("routing: the asking thread changed");
    const gens = r.gens(t.id);
    expect(gens).toHaveLength(1);
    expect(gens[0].withdrawn_at).not.toBeNull();
  });

  it("removing every agent comment invalidates the generation with its own reason", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.comments.splice(0, r.fake.comments.length);
    r.advance(3 * HOUR);
    await r.poll();
    const row = r.cardRow(t.id);
    expect(row.state).toBe("closed");
    expect(row.display_reason).toBe("routing: no agent comment names the asking thread");
    expect(r.gens(t.id)[0].withdrawn_at).not.toBeNull();
  });
});

describe("answer routing: a pull card", () => {
  it("a pull:mycroft card has routing mode pull and no routed thread", async () => {
    const r = rig();
    const { t } = await opened(r, { pull: true });
    const row = r.cardRow(t.id);
    expect(row.routing_mode).toBe("pull");
    expect(row.routed_thread).toBeNull();
    expect(row.state).toBe("open");
  });
});

describe("answer routing: an archived recipient", () => {
  async function ruled() {
    const r = rig();
    const { g1 } = await opened(r);
    r.mkPick(g1.id);
    const wake = wakeOf(r, g1.id);
    const events = (type: string) => r.svc.store.events().filter((e) => e.type === type);
    return { r, g1, wake, events };
  }

  it("goes to the successor the resolver names", async () => {
    const { r, g1, wake, events } = await ruled();
    const sdk = new FakeSdk();
    sdk.script = [{ kind: "throw", err: archived() }];
    await new WakeLoop(r.svc, sdk, async (gone) => (gone === "thr_a" ? "thr_successor" : null)).drain();
    expect(sdk.sent.map((s) => s.threadId)).toEqual(["thr_a", "thr_successor"]);
    expect(wakeOf(r, g1.id).state).toBe("done");
    expect(wakeOf(r, g1.id).recipient).toBe("thr_successor");
    const [ev] = events("rerouted");
    expect(ev.decision_id).toBe(g1.id);
    expect(JSON.parse(ev.detail_json)).toEqual({ obligation: wake.id, from: "thr_a", to: "thr_successor" });
  });

  it("goes to the vizier when the resolver falls back to it", async () => {
    const { r, g1 } = await ruled();
    const sdk = new FakeSdk();
    sdk.script = [{ kind: "throw", err: archived() }];
    await new WakeLoop(r.svc, sdk, async () => "thr_vizier").drain();
    expect(wakeOf(r, g1.id).recipient).toBe("thr_vizier");
    expect(wakeOf(r, g1.id).state).toBe("done");
  });

  it("stalls undeliverable on the original thread when no route exists", async () => {
    const { r, g1, wake } = await ruled();
    const sdk = new FakeSdk();
    sdk.script = [{ kind: "throw", err: archived() }];
    await new WakeLoop(r.svc, sdk, async () => null).drain();
    const row = wakeOf(r, g1.id);
    expect(row.state).toBe("undeliverable");
    expect(row.recipient).toBe("thr_a");
    expect(sdk.sent.map((s) => s.threadId)).toEqual(["thr_a"]);
    expect(wake.id).toBe(row.id);
  });

  it("a queued wake whose thread is later archived is rerouted, and with no route records reroute-failed", async () => {
    const a = await ruled();
    const sdkA = new FakeSdk();
    sdkA.fallback = { kind: "queue" };
    const loopA = new WakeLoop(a.r.svc, sdkA, async (gone) => (gone === "thr_a" ? "thr_successor" : null));
    await loopA.drain();
    expect(wakeOf(a.r, a.g1.id).state).toBe("queued");
    sdkA.fallback = { kind: "sent" };
    await loopA.onThreadGone("thr_a");
    await loopA.drain();
    expect(wakeOf(a.r, a.g1.id).recipient).toBe("thr_successor");
    expect(wakeOf(a.r, a.g1.id).state).toBe("done");
    expect(a.events("rerouted")).toHaveLength(1);

    const b = await ruled();
    const sdkB = new FakeSdk();
    sdkB.fallback = { kind: "queue" };
    const loopB = new WakeLoop(b.r.svc, sdkB, async () => null);
    await loopB.drain();
    await loopB.onThreadGone("thr_a");
    expect(wakeOf(b.r, b.g1.id).state).toBe("undeliverable");
    const [ev] = b.events("reroute-failed");
    expect(JSON.parse(ev.detail_json)).toMatchObject({ obligation: b.wake.id, from: "thr_a" });
  });

  it("a resolver that keeps naming archived threads is abandoned after a few hops", async () => {
    const { r, g1 } = await ruled();
    const sdk = new FakeSdk();
    sdk.fallback = { kind: "throw", err: archived() };
    let n = 0;
    await new WakeLoop(r.svc, sdk, async () => `thr_gone_${++n}`).drain();
    expect(wakeOf(r, g1.id).state).toBe("undeliverable");
    expect(sdk.sent.length).toBeLessThanOrEqual(5);
  });
});

describe("answer routing: the advisory mirror on the card", () => {
  const labels = (r: ReturnType<typeof rig>, id: string) => r.fake.tasks.find((x) => x.id === id)!.labelIds;

  it("a ruling posts the Ruled comment without notifying, removes the needs-mk label and closes the task", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(labels(r, t.id)).toHaveLength(1);
    r.mkPick(g1.id);
    await r.poll();
    const c = r.fake.comments.at(-1)!;
    expect(c.body).toBe(`Ruled: Per project (Home, advisory; ruled by: mk) generation 1\n\nhome-write: cw-${g1.id}-comment`);
    const create = r.fake.callsOf("createComment").at(-1)!;
    expect(create.input.notify).toBe(false);
    expect(create.input.taskId).toBe(t.id);
    expect(labels(r, t.id)).toEqual([]);
    expect(r.fake.tasks.find((x) => x.id === t.id)!.status).toBe("done");
  });

  it("a task with a working thread keeps its status but still loses the label", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.fake.threads.push({ taskId: t.id, liveStatus: "working" });
    r.mkPick(g1.id);
    await r.poll();
    expect(labels(r, t.id)).toEqual([]);
    expect(r.fake.tasks.find((x) => x.id === t.id)!.status).not.toBe("done");
  });
});
