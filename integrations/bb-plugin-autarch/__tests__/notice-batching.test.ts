// Home's `notice` messages to one thread leave as one digest per hourly window; answers and actions stay immediate.
import { afterEach, describe, expect, it } from "vitest";
import { WakeLoop } from "../wakes.js";
import { cleanupEnvs, opened, rig, type Rig } from "./card-rig.js";
import { archived, econnrefused, FakeSdk } from "./wakes-helpers.js";

afterEach(cleanupEnvs);
const HOUR = 3_600_000;

/** A rig whose store batches notices per hour, and a wake loop over a fake SDK. */
function batched(resolve?: (gone: string) => Promise<string | null>) {
  const r = rig();
  r.env.batch.noticeWindowMs = HOUR;
  r.restart();
  const sdk = new FakeSdk();
  let loop = new WakeLoop(r.svc, sdk, resolve);
  const restart = () => {
    r.restart();
    loop = new WakeLoop(r.svc, sdk, resolve);
  };
  return { r, sdk, drain: () => loop.drain(), restart };
}
const to = (sdk: FakeSdk, thread: string) => sdk.sent.filter((s) => s.threadId === thread);
const notices = (r: Rig, decision: string) => r.svc.store.obligationsFor(decision).filter((o) => o.kind === "notice");

/** A card asked by thr_a that names thr_b, picked now. */
async function pickedCard(r: Rig, blocks = "thread:thr_a thread:thr_b") {
  const { g1 } = await opened(r, { blocks });
  expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
  return g1.id as string;
}

describe("notice batching: at most one message per thread per hour", () => {
  it("two notices inside the window leave as one digest at the window end; the wake is not held", async () => {
    const { r, sdk, drain } = batched();
    const a = await pickedCard(r);
    r.advance(10 * 60_000);
    const b = await pickedCard(r);
    await drain();
    expect(to(sdk, "thr_a")).toHaveLength(2); // both wakes to the asker go at once
    expect(to(sdk, "thr_b")).toHaveLength(0);
    r.advance(49 * 60_000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(0); // 59 minutes after the first notice
    r.advance(61_000);
    await drain();
    const sent = to(sdk, "thr_b");
    expect(sent).toHaveLength(1);
    expect(sent[0]!.input).toContain(a);
    expect(sent[0]!.input).toContain(b);
    expect(sent[0]!.input).toContain("2 Home notices");
    for (const id of [a, b]) {
      const n = notices(r, id)[0]!;
      expect(n.state).toBe("done");
      expect(n.last_error).toMatch(/^batched into ob:notice-digest:thr_b:/);
    }
    const digest = r.db.prepare("SELECT * FROM obligations WHERE kind = 'notice-digest'").all() as { state: string; recipient: string }[];
    expect(digest).toEqual([expect.objectContaining({ state: "done", recipient: "thr_b" })]);
    await drain();
    r.advance(2 * HOUR);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
  });

  it("a lone notice is delayed to the window end and goes out unchanged, not as a digest", async () => {
    const { r, sdk, drain } = batched();
    const a = await pickedCard(r);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(0);
    r.advance(HOUR);
    await drain();
    const sent = to(sdk, "thr_b");
    expect(sent).toHaveLength(1);
    expect(sent[0]!.input).toContain(`[home-msg notice:${a}:thr_b #1]`);
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM obligations WHERE kind = 'notice-digest'").get()).toEqual({ n: 0 });
  });

  it("a notice after a window closed opens the next one, so sends to a thread are an hour apart", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r);
    r.advance(HOUR);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(5 * 60_000);
    await pickedCard(r);
    await pickedCard(r);
    r.advance(HOUR - 1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(2);
    expect(to(sdk, "thr_b")[1]!.input).toContain("2 Home notices");
  });

  it("windows are per thread", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r, "thread:thr_a thread:thr_b");
    r.advance(30 * 60_000);
    await pickedCard(r, "thread:thr_a thread:thr_c");
    r.advance(30 * 60_000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    expect(to(sdk, "thr_c")).toHaveLength(0);
    r.advance(30 * 60_000);
    await drain();
    expect(to(sdk, "thr_c")).toHaveLength(1);
  });

  it("a restart inside the window neither loses nor resends the notices", async () => {
    const { r, sdk, drain, restart } = batched();
    await pickedCard(r);
    r.advance(20 * 60_000);
    await pickedCard(r);
    restart();
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(0);
    r.advance(40 * 60_000);
    restart();
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    restart();
    await drain();
    r.advance(HOUR);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
  });

  it("a notice that arrives while the window's batch sits unsent after a restart is not lost", async () => {
    const { r, sdk, drain, restart } = batched();
    await pickedCard(r);
    r.advance(HOUR);
    restart(); // the window ended while the service was down
    await pickedCard(r); // opens a window of its own
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(HOUR);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(2);
  });

  it("a voided notice is not in the digest", async () => {
    const { r, sdk, drain } = batched();
    const a = await pickedCard(r);
    await pickedCard(r);
    expect(r.svc.store.voidObligation(notices(r, a)[0]!.id)).toBe(true);
    r.advance(HOUR);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    expect(to(sdk, "thr_b")[0]!.input).not.toContain(a);
  });

  it("a drain that runs late still leaves the next message an hour after the last actual send", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r); // due at +1h
    r.advance(HOUR + 30 * 60_000);
    await pickedCard(r); // its window opens while the first is overdue and unsent: due at +2h30
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1); // the overdue first notice went at +1h30
    r.advance(60 * 60_000 - 1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(2);
  });

  it("a notice made due by a delayed drain waits out the hour since the previous send", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r); // A: due at +1h
    r.advance(HOUR);
    await drain(); // A goes at +1h
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(10 * 60_000);
    await pickedCard(r); // B: due at +2h10
    r.advance(10 * 60_000);
    // A second notice whose window had opened earlier than A's send (a stalled service) must still wait for A + 1h.
    const late = r.svc.store.db.prepare("UPDATE obligations SET next_try_at = @t WHERE kind = 'notice' AND state = 'pending'");
    late.run({ t: new Date(r.env.clock.t - 5 * 60_000).toISOString() });
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1); // due by its own clock, held until A's send + 1h
    r.advance(40 * 60_000 - 1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1);
    r.advance(1000);
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(2);
  });

  it("a send that failed before acceptance does not use up the hour", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r);
    await drain();
    r.advance(HOUR);
    sdk.script = [{ kind: "throw", err: econnrefused() }];
    await drain();
    expect(to(sdk, "thr_b")).toHaveLength(1); // tried, refused
    r.advance(10 * 60_000);
    await drain();
    r.advance(HOUR);
    await drain();
    expect(to(sdk, "thr_b").length).toBeGreaterThanOrEqual(2);
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM attempts a JOIN obligations o ON o.id = a.obligation_id WHERE o.kind = 'notice' AND a.state = 'delivered'").get()).toEqual({ n: 1 });
  });

  it("notices to two archived threads that resolve to one successor leave as one message", async () => {
    const { r, sdk, drain } = batched(async (gone) => (gone === "thr_b" || gone === "thr_c" ? "thr_s" : null));
    const a = await pickedCard(r, "thread:thr_a thread:thr_b");
    const b = await pickedCard(r, "thread:thr_a thread:thr_c");
    await drain(); // the wakes to thr_a
    r.advance(HOUR);
    sdk.script = [{ kind: "throw", err: archived() }, { kind: "throw", err: archived() }];
    await drain();
    const toS = to(sdk, "thr_s");
    expect(toS).toHaveLength(1);
    expect(toS[0]!.input).toContain(a);
    expect(toS[0]!.input).toContain(b);
    r.advance(3 * HOUR);
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(1);
  });

  it("digests to two archived threads that resolve to one successor do not both go in one pass", async () => {
    const { r, sdk, drain } = batched(async (gone) => (gone === "thr_b" || gone === "thr_c" ? "thr_s" : null));
    for (const t of ["thr_b", "thr_b", "thr_c", "thr_c"]) await pickedCard(r, `thread:thr_a thread:${t}`);
    await drain(); // the wakes to thr_a
    r.advance(HOUR);
    sdk.script = [{ kind: "throw", err: archived() }, { kind: "throw", err: archived() }];
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(1);
    r.advance(HOUR - 1000);
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(1);
    r.advance(1000);
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(2);
  });

  it("a notice retargeted to a successor that was just sent one waits for that successor's window", async () => {
    const { r, sdk, drain } = batched(async (gone) => (gone === "thr_b" ? "thr_s" : null));
    await pickedCard(r, "thread:thr_a thread:thr_s"); // due at +1h
    r.advance(30 * 60_000);
    await pickedCard(r, "thread:thr_a thread:thr_b"); // due at +1h30
    await drain();
    r.advance(30 * 60_000);
    await drain(); // thr_s gets its notice at +1h
    expect(to(sdk, "thr_s")).toHaveLength(1);
    r.advance(30 * 60_000);
    sdk.script = [{ kind: "throw", err: archived() }];
    await drain(); // refused by thr_b at +1h30: the next hop to thr_s is still inside its hour
    expect(to(sdk, "thr_b")).toHaveLength(1);
    expect(to(sdk, "thr_s")).toHaveLength(1);
    r.advance(30 * 60_000 - 1000);
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(1);
    r.advance(1000);
    await drain();
    expect(to(sdk, "thr_s")).toHaveLength(2);
  });

  it("a notice that falls due between the batching read and the send read of the clock does not slip past the spacing", async () => {
    const { r, sdk, drain } = batched();
    await pickedCard(r);
    await pickedCard(r);
    await drain(); // the wakes
    const T = r.env.clock.t;
    const ids = (r.db.prepare("SELECT id FROM obligations WHERE kind = 'notice' ORDER BY rowid").all() as { id: string }[]).map((x) => x.id);
    const set = r.db.prepare("UPDATE obligations SET next_try_at = ? WHERE id = ?");
    set.run(new Date(T - 1000).toISOString(), ids[0]); // overdue
    set.run(new Date(T + 1).toISOString(), ids[1]); // due 1 ms after the batching read
    let reads = 0;
    Object.defineProperty(r.env.clock, "t", { configurable: true, get: () => T + reads++ }); // each read of the clock is 1 ms later
    await drain();
    Object.defineProperty(r.env.clock, "t", { configurable: true, writable: true, value: T + 100 });
    expect(to(sdk, "thr_b")).toHaveLength(1);
  });

  it("with a window of 0 every notice goes at once", async () => {
    const r = rig();
    const sdk = new FakeSdk();
    const loop = new WakeLoop(r.svc, sdk);
    await pickedCard(r);
    await pickedCard(r);
    await loop.drain();
    expect(to(sdk, "thr_b")).toHaveLength(2);
  });
});
