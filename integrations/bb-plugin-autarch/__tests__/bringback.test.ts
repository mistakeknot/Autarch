// Bring back (bead mk-gc2xm): a decided card reopens as the next generation of the same card.
import { afterEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { ArchiveRowView } from "../ui/archive.js";
import { HOME_MOVE_LABEL } from "../cardwrites.js";
import { cleanupEnvs, opened, pollN, rig } from "./card-rig.js";

afterEach(cleanupEnvs);
const labelled = (r: any, t: any) => r.fake.tasks.find((x: any) => x.id === t.id).labelIds.length > 0;

async function ruled(r = rig()) {
  const { t, g1 } = await opened(r);
  expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
  await pollN(r, 5);
  r.edit(t, { status: "done" });
  await pollN(r, 3);
  expect(r.cardRow(t.id).state).toBe("ruled");
  return { r, t, g1 };
}

describe("bring back", () => {
  it("reopens an mk-picked, done card as generation 2 of the same card, keeping the first pick", async () => {
    const { r, t, g1 } = await ruled();
    const out = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    expect(out).toMatchObject({ ok: true, replay: false });
    await pollN(r, 10);
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", home_unlabelled_at: null });
    expect(labelled(r, t)).toBe(true);
    expect(r.fake.tasks.find((x: any) => x.id === t.id)?.status).toBe("todo");
    const gens = r.gens(t.id);
    expect(gens).toHaveLength(2);
    expect(gens[1]?.supersedes).toBe(g1.id);
    expect(r.svc.store.pick(g1.id)).toMatchObject({ by: "mk" }); // the earlier pick is still there
    // the notice goes to the asking thread
    expect(r.svc.store.obligationsFor(g1.id).find((o) => o.op === `bringback:${g1.id}`)).toMatchObject({ recipient: "thr_a" });
    // and mk's new answer lands on generation 2
    if (!out.ok) throw new Error(out.error);
    expect(r.mkPick(out.decision_id, "p2")).toMatchObject({ ok: true });
  });

  it("carries the earlier card's Blocks threads to the new generation", async () => {
    const { r, t, g1 } = await ruled();
    const db = (r.svc as any).db;
    db.prepare("INSERT OR IGNORE INTO decision_blocks(decision_id, ref) VALUES (?, ?)").run(g1.id, "thread:thr_b");
    const out = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    if (!out.ok) throw new Error(out.error);
    const refs = db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(out.decision_id).map((x: any) => x.ref);
    const before = db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(g1.id).map((x: any) => x.ref);
    expect(refs.sort()).toEqual(before);
    expect(refs).toContain("thread:thr_b");
  });

  it("is idempotent: a second call replays and adds no generation", async () => {
    const { r, t, g1 } = await ruled();
    const a = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    const b = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    expect(a).toMatchObject({ ok: true, replay: false });
    expect(b).toMatchObject({ ok: true, replay: true });
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("sends the notice to the fallback recipient it is given, and none when there is none", async () => {
    const { r, t, g1 } = await ruled();
    r.dele.bringBack(t.id, g1.id, "thr_viz", {});
    expect(r.svc.store.obligationsFor(g1.id).find((o) => o.op === `bringback:${g1.id}`)).toMatchObject({ recipient: "thr_viz" });
    const two = await ruled(rig());
    two.r.dele.bringBack(two.t.id, two.g1.id, null, {});
    expect(two.r.svc.store.obligationsFor(two.g1.id).some((o) => (o.op ?? "").startsWith("bringback:"))).toBe(false);
  });

  it("also reopens a vizier-picked card", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 5);
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true });
    await pollN(r, 5);
    expect(r.cardRow(t.id).state).toBe("open");
  });

  it("refuses a card with no pick, a thread caller and an unknown card", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: false });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", { threadId: "thr_a" })).toMatchObject({ ok: false, status: 403 });
    expect(r.dele.bringBack("nope", "x", "thr_a", {})).toMatchObject({ ok: false, status: 404 });
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("a move close queued by the earlier ruling cannot shut the reopened card, queued before or after", async () => {
    const { r, t, g1 } = await ruled();
    const l = r.fake.addLabel(r.tp.id, HOME_MOVE_LABEL);
    r.fake.tasks.find((x: any) => x.id === t.id)!.labelIds.push(l.id);
    const seed = () => r.db.prepare("INSERT OR IGNORE INTO move_closes(task_id, generation, next_try_at, updated_at) VALUES (?, 1, '2000-01-01T00:00:00Z', '2000-01-01T00:00:00Z')").run(t.id);
    seed();
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true });
    seed(); // a late queue entry for generation 1 is dropped by the drain
    r.db.prepare("UPDATE move_closes SET state = 'pending'").run();
    await pollN(r, 10);
    expect(r.fake.tasks.find((x: any) => x.id === t.id)?.status).toBe("todo");
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.db.prepare("SELECT state FROM move_closes WHERE task_id = ?").get(t.id)).toEqual({ state: "skipped" });
  });

  it("a delayed retry after mk answered the new generation does not reopen it again", async () => {
    const { r, t, g1 } = await ruled();
    const a = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    if (!a.ok) throw new Error(a.error);
    expect(r.mkPick(a.decision_id, "p2")).toMatchObject({ ok: true });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true, replay: true, decision_id: a.decision_id });
    expect(r.dele.bringBack(t.id, "some-old-id", "thr_a", {})).toMatchObject({ ok: false, status: 409 });
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("a picked card is not touched by a later edit before it is brought back (T8 unchanged)", async () => {
    const { r, t, g1 } = await ruled();
    await pollN(r, 3);
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

describe("Archive row button", () => {
  const row = { task_id: "t1", card_key: "AUTA-1", title: "T", project: "AUTA", state: "ruled", decided_at: "2026-10-01T00:00:00Z", pick_label: "Per day", option_id: "a", by: "mk", reason: null, generation: 1, no_pick: false, decision_id: "card-t1-g1" };
  it("shows Bring back on a picked row, not on a row without a pick, and shows an error", () => {
    const html = renderToStaticMarkup(createElement(ArchiveRowView, { row, onBringBack: () => {}, error: "boom" }));
    expect(html).toContain("data-archive-bringback=\"AUTA-1\"");
    expect(html).toContain("boom");
    expect(renderToStaticMarkup(createElement(ArchiveRowView, { row: { ...row, no_pick: true, option_id: null, by: null }, onBringBack: () => {} }))).not.toContain("data-archive-bringback");
  });
});
