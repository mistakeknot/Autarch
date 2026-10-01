// Task 2.5 integration tests moved from 2.4 (finding r4-5): the crash window, the immutable snapshot
// and T9/T10/T11 end to end, over the fake tasks plugin and a real writeRuling into a temp root.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { chmodSync } from "node:fs";
import { cleanupEnvs, opened, pollN, rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const rulings = (root: string) => {
  const dir = join(root, "docs", "decisions");
  return existsSync(dir) ? readdirSync(dir) : [];
};

describe("crash window (finding r2-3)", () => {
  async function vizierPicked() {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    return { r, t, g1 };
  }
  const labelled = (r: any, t: any) => r.fake.tasks.find((x: any) => x.id === t.id).labelIds.length > 0;

  it("the unlabel landed on tasks but its record did not: an override then keeps g2 open across 10 polls", async () => {
    const { r, t, g1 } = await vizierPicked();
    r.fake.lostResponses.push({ method: "updateTask", nth: 1 }); // the effect lands, the response is lost
    await r.poll();
    expect(labelled(r, t)).toBe(false);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'unlabel'").get()).toEqual({ state: "pending" });
    expect(r.cardRow(t.id).home_unlabelled_at).toBeNull();
    const o = r.dele.override(g1.id, {});
    expect(o).toMatchObject({ ok: true, decision_id: `card-${t.id}-g2` });
    await pollN(r, 10);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(`card-${t.id}-g2`)).toEqual({ withdrawn_at: null });
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'unlabel'").get()).toEqual({ state: "skipped" });
    expect(labelled(r, t)).toBe(true);
    expect(r.mkPick(`card-${t.id}-g2`)).toMatchObject({ ok: true });
  });

  it("restart before the override, then the override", async () => {
    const { r, t, g1 } = await vizierPicked();
    r.fake.lostResponses.push({ method: "updateTask", nth: 1 });
    await r.poll();
    r.restart();
    expect(r.dele.override(g1.id, {})).toMatchObject({ ok: true });
    await pollN(r, 10);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(labelled(r, t)).toBe(true);
  });

  it("restart after the override", async () => {
    const { r, t, g1 } = await vizierPicked();
    r.fake.lostResponses.push({ method: "updateTask", nth: 1 });
    await r.poll();
    expect(r.dele.override(g1.id, {})).toMatchObject({ ok: true });
    r.restart();
    await pollN(r, 10);
    expect(r.cardRow(t.id).state).toBe("open");
    expect(labelled(r, t)).toBe(true);
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("the unlabel succeeds and is recorded, then an override relabels the card and it stays open", async () => {
    const { r, t, g1 } = await vizierPicked();
    await r.poll();
    expect(labelled(r, t)).toBe(false);
    expect(r.cardRow(t.id)).toMatchObject({ state: "ruled" });
    expect(r.cardRow(t.id).home_unlabelled_at).not.toBeNull();
    r.dele.override(g1.id, {});
    await pollN(r, 10);
    expect(labelled(r, t)).toBe(true);
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", home_unlabelled_at: null });
  });
});

describe("immutable snapshot", () => {
  it("pick, failed ruling write, card edit, restart, retry: the original text is written", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { question: "Original question?" });
    const root = r.env.roots.Autarch!;
    chmodSync(root, 0o555); // the ruling write fails
    const picked = r.mkPick(g1.id);
    expect(picked).toMatchObject({ ok: true });
    expect(r.svc.store.obligationsFor(g1.id).find((o) => o.kind === "ruling-file")).toMatchObject({ state: "pending" });
    expect(rulings(root)).toEqual([]);
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), question: "A rewritten question?" }) });
    await r.poll();
    expect(r.gens(t.id)).toHaveLength(1); // T8: no generation after a pick
    r.restart();
    chmodSync(root, 0o755);
    r.advance(3_700_000);
    r.svc.reconcileAll();
    const files = rulings(root);
    expect(files).toHaveLength(1);
    const text = readFileSync(join(root, "docs", "decisions", files[0]!), "utf8");
    expect(text).toContain("Original question?");
    expect(text).not.toContain("A rewritten question?");
    expect(text).toContain(`card_id: "${t.id}"`);
    expect(text).toContain("generation: 1");
  });
});

describe("T9/T10/T11 end to end", () => {
  it("an mk override of a vizier pick survives done, unlabel and an invalid edit, and mk's pick on it succeeds", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await r.poll();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await pollN(r, 3);
    const key = String(r.cardRow(t.id).request_key);
    r.edit(t, { description: r.desc({ key, breakJson: true }) });
    await pollN(r, 2);
    r.edit(t, { labelIds: [] });
    await pollN(r, 2);
    r.edit(t, { status: "done" });
    await pollN(r, 2);
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });
    expect(r.gens(t.id)).toHaveLength(2);
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
    expect(rulings(r.env.roots.Autarch!)).toHaveLength(2);
  });
});
