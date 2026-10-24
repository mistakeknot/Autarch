// One count of what is waiting on mk (bead mk-yjp7). Home's own state decides it: owed decisions that are not on hold,
// open moves, and notices that change what Home does. Catch-up reading and held cards are shown beside it, never in it.
import { afterEach, describe, expect, it } from "vitest";
import { Catchup } from "../catchup.js";
import { waitingNow, WAITING_DEFINITION } from "../waiting.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

let n = 0;
/** A card with its own request identity so several can be open at once. */
const card = (r: Rig, o: Parameters<Rig["card"]>[0] = {}) => {
  n += 1;
  return r.card({ key: `w-${n}`, ident: n.toString(16).padStart(16, "0"), ...o });
};
const withMove = (t: { description: string }, move: object) => void (t.description += "```home-move\n" + JSON.stringify(move) + "\n```\n");
const prMove = (k: number) => ({ schema: "home-move/v1", kind: "pr", pr: { url: `https://github.com/o/r/pull/${k}` } });
const count = (r: Rig) => waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele));

describe("waiting on mk", () => {
  it("counts owed decisions, not held ones, and shows the held count beside it", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    const cards = Array.from({ length: 6 }, () => card(r));
    await r.poll();
    expect(r.svc.hold(cards[5]!.id, "waiting on the vendor", "mk")).toMatchObject({ ok: true });
    const w = count(r);
    expect(w).toMatchObject({ total: 5, decide: 5, moves: 0, notices: 0, held: 1 });
  });

  it("a move on a card that also has an owed decision counts once", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    const a = card(r);
    const b = card(r);
    withMove(a, prMove(1));
    withMove(b, prMove(2));
    await r.poll();
    // a keeps its owed decision AND a move; b's decision is picked, so only its move remains
    const gb = r.gens(b.id)[0];
    expect(r.mkPick(gb.id)).toMatchObject({ ok: true });
    const w = count(r);
    expect(w).toMatchObject({ decide: 1, moves: 1, total: 2 });
  });

  it("a claimed, skipped or held move is not waiting", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    const t = [card(r), card(r), card(r), card(r)];
    t.forEach((x, i) => withMove(x, prMove(i + 10)));
    await r.poll();
    for (const x of t) r.mkPick(r.gens(x.id)[0].id); // only the moves are owed now
    expect(count(r).moves).toBe(4);
    r.svc.store.claimMove(t[0]!.id, 1, "mk");
    r.svc.store.skipMove(t[1]!.id, 1);
    r.svc.hold(t[2]!.id, "later", "mk");
    expect(count(r)).toMatchObject({ moves: 1, held: 1 });
  });

  it("an unseen vizier adoption is one notice; acknowledging it lifts the suspension and the count", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    card(r);
    await r.poll();
    r.svc.store.recordEvent("vizier-adopted", null, { from: null, to: "thr_viz2", reason: "unset" });
    expect(r.dele.suspendedNow()).toBe(true);
    const w = count(r);
    expect(w).toMatchObject({ decide: 1, notices: 1, total: 2 });
    expect(w.noticeItems).toHaveLength(1);
    expect(w.noticeItems[0]!.text).toContain("thr_viz2");
    r.dele.markSeen("mk", w.noticeItems[0]!.item);
    expect(r.dele.suspendedNow()).toBe(false);
    expect(count(r)).toMatchObject({ notices: 0, total: 1 });
  });

  it("unseen catch-up reading is shown beside the count, never in it", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    card(r);
    await r.poll();
    const cu = new Catchup(r.svc, r.dele);
    expect(cu.recordTurnFailed("thr_a", "req-1")).toBe(true);
    cu.addNote("a note", [`failed:${(r.db.prepare("SELECT seq FROM events WHERE type='turn-failed'").get() as { seq: number }).seq}`]);
    const w = waitingNow(r.svc, r.dele, cu);
    expect(w.total).toBe(1);
    expect(w.updates).toBe(2);
  });

  it("states what it counts", () => {
    expect(WAITING_DEFINITION).toMatch(/decision/i);
    expect(WAITING_DEFINITION).not.toMatch(/move/i); // Home opens no moves (bead mk-1k8pg)
    expect(WAITING_DEFINITION).toMatch(/hold/i);
  });
});

describe("decisionCount (the rail badge)", () => {
  it("counts open cards, held ones included, and is zero with none", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    expect(r.svc.decisionCount()).toBe(0);
    const cards = Array.from({ length: 3 }, () => card(r));
    await r.poll();
    expect(r.svc.decisionCount()).toBe(3);
    expect(r.svc.hold(cards[2]!.id, "waiting on the vendor", "mk")).toMatchObject({ ok: true });
    expect(r.svc.decisionCount()).toBe(3);
  });
});

describe("decisionCount excludes what is not waiting", () => {
  it("counts display cards, not deleted, unlabelled or other states", async () => {
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    const cards = Array.from({ length: 6 }, () => card(r));
    await r.poll();
    expect(r.svc.decisionCount()).toBe(6);
    const db = (r.svc as unknown as { db: { prepare(q: string): { run(...a: unknown[]): unknown } } }).db;
    const set = (i: number, sql: string) => db.prepare(`UPDATE cards SET ${sql} WHERE task_id = ?`).run(cards[i]!.id);
    set(0, "state = 'display'");
    set(1, "deleted_at = '2026-10-23T00:00:00Z'");
    set(2, "labelled = 0");
    set(3, "state = 'closed'");
    set(4, "state = 'observed'");
    expect(r.svc.decisionCount()).toBe(2); // the display card and the untouched open one
  });
});

describe("railCount (the badge number)", () => {
  it("prefers the server count, then live owed asks, then the waiting decide count", async () => {
    const { railCount } = await import("../ui/waiting.js");
    expect(railCount(0, [{}, {}], 5)).toBe(0);
    expect(railCount(null, [{}, { held: true }, {}], 5)).toBe(2);
    expect(railCount(null, undefined, 5)).toBe(5);
    expect(railCount(null, undefined, undefined)).toBe(0);
  });
});

describe("one number everywhere", () => {
  it("the strip, the tab and the stats output show the same total; the badge shows the decision count", async () => {
    const { badgeCount, waitingSummary } = await import("../ui/waiting.js");
    const r = rig({ legacyMoves: true });
    r.enableDelegation();
    card(r);
    card(r);
    await r.poll();
    r.svc.store.recordEvent("vizier-adopted", null, { from: null, to: "thr_viz3", reason: "unset" });
    const w = count(r);
    expect(w.total).toBe(3);
    expect(badgeCount({ count: r.svc.decisionCount() }, { blocked: false, unowned: false })).toBe("2");
    expect(waitingSummary(w).headline).toBe("Waiting on you: 3");
  });
});
