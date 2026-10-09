// "Later" on an open card (bead mk-8741): mk sets a card aside to do after the fast ones. The card stays open and unruled;
// it leaves "Waiting on you" and is counted beside it. Stored in settings_kv like holds, so there is no schema change.
import { afterEach, describe, expect, it } from "vitest";
import { Catchup } from "../catchup.js";
import { moveViews } from "../moveview.js";
import { waitingNow } from "../waiting.js";
import { cleanupEnvs, opened, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

let n = 100;
const card = (r: Rig, o: Parameters<Rig["card"]>[0] = {}) => {
  n += 1;
  return r.card({ key: `l-${n}`, ident: n.toString(16).padStart(16, "0"), ...o });
};
const withMove = (t: { description: string }, move: object) => void (t.description += "```home-move\n" + JSON.stringify(move) + "\n```\n");
const prMove = (k: number) => ({ schema: "home-move/v1", kind: "pr", pr: { url: `https://github.com/o/r/pull/${k}` } });
const count = (r: Rig) => waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele));

describe("later (store)", () => {
  it("sets a card aside, names who and when, and is idempotent", async () => {
    const r = rig();
    const { t } = await opened(r);
    expect(r.svc.laterOf(t.id)).toBeNull();
    const a = r.svc.later(t.id, "mk");
    expect(a).toMatchObject({ ok: true, task_id: t.id, was: false });
    const first = r.svc.laterOf(t.id)!;
    expect(first).toMatchObject({ by: "mk" });
    expect(Number.isNaN(Date.parse(first.at))).toBe(false);
    r.advance(5_000);
    expect(r.svc.later(t.id, "mk")).toMatchObject({ ok: true, was: true });
    expect(r.svc.laterOf(t.id)).toEqual(first); // a second tap does not move it
  });

  it("names a card by its tasks key, and refuses an unknown card", async () => {
    const r = rig();
    const { t } = await opened(r);
    const key = r.svc.cardKey(t.id)!;
    expect(r.svc.later(key, "mk")).toMatchObject({ ok: true, task_id: t.id });
    expect(r.svc.later("no-such", "mk")).toEqual({ ok: false, error: "no such card" });
    expect(r.svc.unlater("no-such")).toEqual({ ok: false, error: "no such card" });
  });

  it("move back clears it, and says whether there was anything to clear", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.later(t.id, "mk");
    expect(r.svc.unlater(t.id)).toEqual({ ok: true, task_id: t.id, was: true });
    expect(r.svc.laterOf(t.id)).toBeNull();
    expect(r.svc.unlater(t.id)).toEqual({ ok: true, task_id: t.id, was: false });
  });

  it("fails open: an unreadable or malformed value is no one's Later", async () => {
    const r = rig();
    const { t } = await opened(r);
    for (const bad of ["{not json", "[]", "7", JSON.stringify({ [t.id]: { by: "mk" } }), JSON.stringify({ [t.id]: { at: "never", by: "mk" } }), JSON.stringify({ [t.id]: { at: "2026-10-08T00:00:00.000Z" } })]) {
      r.svc.store.setSetting("later", bad);
      expect(r.svc.laterOf(t.id)).toBeNull();
      expect(r.svc.laters()).toEqual({});
    }
    // and a bad value does not stop a fresh Later from being set
    expect(r.svc.later(t.id, "mk")).toMatchObject({ ok: true });
    expect(r.svc.laterOf(t.id)).not.toBeNull();
  });

  it("keeps other cards' Later when one is moved back", async () => {
    const r = rig();
    const a = await opened(r);
    const b = await opened(r, { key: "other", ident: "fedcba9876543210" });
    r.svc.later(a.t.id, "mk");
    r.svc.later(b.t.id, "mk");
    r.svc.unlater(a.t.id);
    expect(Object.keys(r.svc.laters())).toEqual([b.t.id]);
  });

  it("survives a restart", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.later(t.id, "mk");
    r.restart();
    expect(r.svc.laterOf(t.id)).toMatchObject({ by: "mk" });
  });

  it("does not touch the card, its decision or its tasks labels", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const before = JSON.stringify(r.cardRow(t.id));
    r.svc.later(t.id, "mk");
    expect(JSON.stringify(r.cardRow(t.id))).toBe(before);
    expect(r.svc.owed().map((d) => d.id)).toContain(g1.id); // still owed, still open
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

describe("later and picking", () => {
  it("a Later card can still be ruled, and ruling it clears the Later", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.svc.later(t.id, "mk");
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect(r.svc.laterOf(t.id)).toBeNull();
  });

  it("a refused pick leaves the Later alone", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.svc.later(t.id, "mk");
    const bad = r.svc.pick(g1.id, "nope", (r.svc.store.decision(g1.id) as any).revision, "p-bad", "mk", "home");
    expect(bad).toMatchObject({ ok: false });
    expect(r.svc.laterOf(t.id)).not.toBeNull();
  });

  it("a hold wins over Later: the held card is refused as before, and Later does not unhold it", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.svc.later(t.id, "mk");
    r.svc.hold(t.id, "script superseded", "thr_viz");
    expect(r.mkPick(g1.id)).toMatchObject({ ok: false, status: 409 });
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    expect(r.svc.laterOf(t.id)).not.toBeNull();
  });
});

describe("later and the Waiting-on-you count", () => {
  it("a Later card leaves the count and is counted beside it, not in it", async () => {
    const r = rig();
    r.enableDelegation();
    const cards = Array.from({ length: 5 }, () => card(r));
    await r.poll();
    expect(count(r)).toMatchObject({ decide: 5, total: 5, later: 0 });
    r.svc.later(cards[0]!.id, "mk");
    r.svc.later(cards[1]!.id, "mk");
    expect(count(r)).toMatchObject({ decide: 3, moves: 0, total: 3, later: 2 });
    r.svc.unlater(cards[0]!.id);
    expect(count(r)).toMatchObject({ decide: 4, total: 4, later: 1 });
  });

  it("a held card is held, not Later: it is counted once, in held", async () => {
    const r = rig();
    r.enableDelegation();
    const cards = [card(r), card(r), card(r)];
    await r.poll();
    r.svc.later(cards[0]!.id, "mk");
    r.svc.hold(cards[0]!.id, "superseded", "thr_viz");
    r.svc.hold(cards[1]!.id, "superseded", "thr_viz");
    expect(count(r)).toMatchObject({ decide: 1, held: 2, later: 0, total: 1 });
  });

  it("a move on a Later card is not waiting either, and the card is one Later, not two", async () => {
    const r = rig();
    r.enableDelegation();
    const a = card(r);
    const b = card(r);
    withMove(a, prMove(21));
    withMove(b, prMove(22));
    await r.poll();
    r.mkPick(r.gens(a.id)[0].id);
    r.mkPick(r.gens(b.id)[0].id);
    expect(count(r)).toMatchObject({ decide: 0, moves: 2, total: 2, later: 0 });
    r.svc.later(a.id, "mk");
    expect(count(r)).toMatchObject({ moves: 1, total: 1, later: 1 });
  });

  it("a card with both an owed decision and a move is one Later", async () => {
    const r = rig();
    r.enableDelegation();
    const a = card(r);
    withMove(a, prMove(31));
    await r.poll();
    r.svc.later(a.id, "mk");
    expect(count(r)).toMatchObject({ decide: 0, moves: 0, total: 0, later: 1 });
  });

  it("a skipped move (Later / skip) is counted beside the number too, once", async () => {
    const r = rig();
    r.enableDelegation();
    const a = card(r);
    const b = card(r);
    withMove(a, prMove(41));
    withMove(b, prMove(42));
    await r.poll();
    r.mkPick(r.gens(a.id)[0].id);
    r.mkPick(r.gens(b.id)[0].id);
    r.svc.skipMove(a.id, 1);
    expect(count(r)).toMatchObject({ moves: 1, total: 1, later: 1 });
    r.svc.later(a.id, "mk"); // skipped and set aside: still one
    expect(count(r)).toMatchObject({ moves: 1, total: 1, later: 1 });
  });

  it("a move on a Later card shows in the Your move Later group, with the card flag, and leaves it on Move back", async () => {
    const r = rig();
    r.enableDelegation();
    const a = card(r);
    withMove(a, prMove(51));
    await r.poll();
    r.mkPick(r.gens(a.id)[0].id);
    expect(moveViews(r.svc).yourMove.map((m) => m.task_id)).toEqual([a.id]);
    r.svc.later(a.id, "mk");
    const v = moveViews(r.svc);
    expect(v.yourMove).toEqual([]);
    expect(v.later.map((m) => [m.task_id, m.card_later])).toEqual([[a.id, true]]);
    r.svc.unlater(a.id);
    expect(moveViews(r.svc).yourMove.map((m) => m.task_id)).toEqual([a.id]);
  });

  it("two connections to one database see each other's Later (every write re-reads inside a transaction)", async () => {
    const r = rig();
    const { t: a } = await opened(r);
    const { t: b } = await opened(r);
    const svc2 = r.env.open();
    r.svc.later(a.id, "mk");
    svc2.later(b.id, "mk");
    expect(Object.keys(r.svc.laters()).sort()).toEqual([a.id, b.id].sort());
    svc2.unlater(a.id);
    expect(Object.keys(r.svc.laters())).toEqual([b.id]);
  });

  it("a stale Later on a card with nothing open is not counted", async () => {
    const r = rig();
    r.enableDelegation();
    const a = card(r);
    await r.poll();
    r.svc.later(a.id, "mk");
    r.svc.store.setSetting("later", JSON.stringify({ ...r.svc.laters(), "gone-task": { at: r.env.now(), by: "mk" } }));
    expect(count(r).later).toBe(1);
  });

  it("the definition says what Later is", async () => {
    const { WAITING_DEFINITION } = await import("../waiting.js");
    expect(WAITING_DEFINITION).toMatch(/later/i);
  });
});
