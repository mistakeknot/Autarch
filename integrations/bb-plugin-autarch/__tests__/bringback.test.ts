// Bring back (bead mk-gc2xm): a decided card reopens as the next generation of the same card.
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { ArchiveRowView } from "../ui/archive.js";
import { archiveView } from "../archive.js";
import { cleanupEnvs, opened, pollN, rig } from "./card-rig.js";

afterEach(() => { cleanupEnvs(); vi.unstubAllEnvs(); });
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
  it.each([false, true])("records only real picks across bring-back with earlier pick=%s", async earlierPick => {
    const r = rig();
    vi.stubEnv("AUTARCH_CARD_STATE_DIR", r.env.dir);
    vi.stubEnv("AUTARCH_NEEDSMK_PILOTS_OFF", "");
    writeFileSync(join(r.env.dir, "pilots.json"), JSON.stringify({ daycap: { enabled: true, until: "2099-01-01T00:00:00Z" } }));
    const { t, g1 } = await opened(r);
    if (earlierPick) expect(r.mkPick(g1.id, "earlier-answer")).toMatchObject({ ok: true });
    r.edit(t, { status: "done" });
    await pollN(r, 3);
    const before = earlierPick ? ["pick:earlier-answer"] : [];
    expect(r.svc.store.pilotMetrics().map(m => m.event_id)).toEqual(before);
    const out = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    if (!out.ok) throw new Error(out.error);
    expect(r.svc.store.pilotMetrics().map(m => m.event_id)).toEqual(before);
    await pollN(r, 5);
    expect(r.mkPick(out.decision_id, "fresh-round-answer")).toMatchObject({ ok: true });
    expect(r.svc.store.pilotMetrics().map(m => m.event_id)).toEqual([...before, "pick:fresh-round-answer"]);
  });
  it.each(["done", "canceled"])("reopens a %s card without a pick as a fresh round, preserving comments and retry identity", async status => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const comments = [...r.fake.comments];
    r.edit(t, { status });
    await pollN(r, 3);
    expect(r.cardRow(t.id).state).toBe("closed");
    expect(r.svc.store.decision(g1.id)).toMatchObject({ withdrawn_at: expect.any(String) });
    const row = archiveView(r.db, {}).rows.find(x => x.task_id === t.id)!;
    expect(row).toMatchObject({ no_pick: true, decision_id: g1.id });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", { threadId: "thr_a" })).toMatchObject({ ok: false, status: 403 });
    expect(r.dele.bringBack(t.id, "stale-generation", "thr_a", {})).toMatchObject({ ok: false, status: 409 });
    expect(r.gens(t.id)).toHaveLength(1);
    const out = r.dele.bringBack(t.id, row.decision_id!, "thr_a", {});
    expect(out).toMatchObject({ ok: true, replay: false });
    if (!out.ok) throw new Error(out.error);
    r.restart();
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true, replay: true, decision_id: out.decision_id });
    await pollN(r, 5);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(t.status).toBe("todo");
    expect(labelled(r, t)).toBe(true);
    expect(r.gens(t.id)).toHaveLength(2);
    expect(r.gens(t.id)[1]).toMatchObject({ id: out.decision_id, generation: 2, supersedes: g1.id, withdrawn_at: null });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
    expect(r.svc.store.pick(out.decision_id)).toBeUndefined();
    expect(r.fake.comments).toEqual(comments);
    expect(archiveView(r.db, {}).rows.some(x => x.task_id === t.id)).toBe(false);
    const notices = r.svc.store.obligationsFor(g1.id).filter(o => o.op === `bringback:${g1.id}`);
    expect(notices).toHaveLength(1);
    expect(notices[0]).toMatchObject({ recipient: "thr_a" });
    expect(notices[0]!.payload).toContain("No earlier pick");
    expect(r.mkPick(out.decision_id, "fresh-answer")).toMatchObject({ ok: true });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true, replay: true, decision_id: out.decision_id });
    expect(r.gens(t.id)).toHaveLength(2);
  });
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

  it("reopens an unanswered later round even when an older round had a pick", async () => {
    const { r, t, g1 } = await ruled();
    const second = r.dele.bringBack(t.id, g1.id, "thr_a", {});
    if (!second.ok) throw new Error(second.error);
    await pollN(r, 3);
    r.edit(t, { status: "done" });
    await pollN(r, 3);
    expect(r.cardRow(t.id).state).toBe("closed");
    expect(r.svc.store.pick(second.decision_id)).toBeUndefined();
    const third = r.dele.bringBack(t.id, second.decision_id, "thr_a", {});
    expect(third).toMatchObject({ ok: true });
    await pollN(r, 5);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.gens(t.id)).toHaveLength(3);
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

  it("refuses an open card with no pick, a thread caller and an unknown card", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: false });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", { threadId: "thr_a" })).toMatchObject({ ok: false, status: 403 });
    expect(r.dele.bringBack("nope", "x", "thr_a", {})).toMatchObject({ ok: false, status: 404 });
    expect(r.gens(t.id)).toHaveLength(1);
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
  it("shows Bring back on picked and no-pick rows, and shows an error", () => {
    const html = renderToStaticMarkup(createElement(ArchiveRowView, { row, onBringBack: () => {}, error: "boom" }));
    expect(html).toContain("data-archive-bringback=\"AUTA-1\"");
    expect(html).toContain("boom");
    const noPick = renderToStaticMarkup(createElement(ArchiveRowView, { row: { ...row, state: "closed", no_pick: true, option_id: null, by: null }, onBringBack: () => {} }));
    expect(noPick).toMatch(/<button\b[^>]*data-archive-bringback=[^>]*>Bring back<\/button>/);
  });
});

it.each(["done", "canceled"])("stops offering a brought-back generation after external task closure: %s", async status => {
  const { r, t, g1 } = await ruled();
  const out = r.dele.bringBack(t.id, g1.id, "thr_a", {});
  if (!out.ok) throw new Error(out.error);
  await pollN(r, 5);
  expect(r.svc.isOwed(out.decision_id)).toBe(true);
  r.edit(t, { status });
  await pollN(r, 3);
  expect(r.svc.owed().some(d => d.id === out.decision_id)).toBe(false);
  expect(r.svc.isOwed(out.decision_id)).toBe(false);
});
