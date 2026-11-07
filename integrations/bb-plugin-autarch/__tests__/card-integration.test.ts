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

  // mk-cggjk: a card whose override is never picked, on a task that goes away for good, must stop
  // showing as open -- nothing ever polls it again once its cached status leaves the open set, so
  // "open" forever is a permanent stuck state, not a transient one.
  it("mk-cggjk: an unpicked override on a task canceled for good moves the card off open, and a later mk pick still rules it", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await r.poll();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await pollN(r, 3);
    expect(r.cardRow(t.id).state).toBe("open"); // T10: the override is mk's pending decision

    r.edit(t, { status: "canceled" });
    await pollN(r, 5); // several polls: the task is gone, re-checking it changes nothing further

    expect(r.cardRow(t.id)).toMatchObject({ state: "closed", status: "canceled" });
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });

    // the override decision is still live: mk can pick it later and the card still rules correctly.
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
    expect(r.cardRow(t.id)).toMatchObject({ state: "ruled" });
  });

  // mk-cggjk sweep: the existing production rows were already written stuck at state 'open' by the
  // old bug, before this fix shipped. Force a card back into that exact pre-fix shape (cardGone
  // itself is fixed now, so it won't produce one) and confirm sweepAbandonedOverrides() -- run once
  // on every start() -- heals it the same way, and is a no-op on an already-healthy card.
  it("mk-cggjk sweep: start() heals a card left stuck open by the old bug, and leaves a healthy one alone", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await r.poll();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await pollN(r, 3);
    r.edit(t, { status: "canceled" });
    await pollN(r, 2);
    expect(r.cardRow(t.id).state).toBe("closed"); // the fixed cardGone already got this right

    r.db.prepare("UPDATE cards SET state = 'open' WHERE task_id = ?").run(t.id); // simulate a pre-fix stuck row
    expect(r.svc.sweepAbandonedOverrides()).toEqual({ swept: [t.id] });
    expect(r.cardRow(t.id).state).toBe("closed");
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });

    expect(r.svc.sweepAbandonedOverrides()).toEqual({ swept: [] }); // already closed: no-op

    const { t: t2, g1: g2 } = await opened(r);
    expect(r.vizierPick(g2.id)).toMatchObject({ ok: true });
    await r.poll();
    expect(r.cardRow(t2.id).state).toBe("ruled");
    expect(r.svc.sweepAbandonedOverrides()).toEqual({ swept: [] }); // a normal ruled card is untouched
    expect(r.cardRow(t2.id).state).toBe("ruled");
  });

  // Cross-lab review of PR #159, finding 1: a card closed here must come back if a person reopens its task,
  // or a genuinely reopened override question is hidden from mk for good.
  it("mk-cggjk: a closed override card opens again when a person reopens its canceled task", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await r.poll();
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    await pollN(r, 3);
    r.edit(t, { status: "canceled" });
    await pollN(r, 3);
    expect(r.cardRow(t.id).state).toBe("closed");

    r.edit(t, { status: "todo" }); // a person reopens it
    await pollN(r, 3);
    expect(r.cardRow(t.id)).toMatchObject({ state: "open", display_reason: null });
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
    expect(r.cardRow(t.id)).toMatchObject({ state: "ruled" });
  });

  // Finding 2: the real pre-fix stuck shapes. The vizier pick unlabels and completes the task, a poll caches it
  // unlabelled+done, and the override's relabel restored the label but (before this fix) did not reopen the task;
  // so the cache stays labelled=0 with a terminal status and nothing polls the card. A deleted task is the same
  // shape with deleted_at set and whatever status it had.
  it("mk-cggjk sweep: heals pre-fix rows cached unlabelled with a terminal status, and deleted ones", async () => {
    const r = rig();
    r.enableDelegation();
    const mk = async () => {
      const { t, g1 } = await opened(r);
      expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
      await pollN(r, 2); // unlabel+done lands, the next poll caches it unlabelled/done
      expect(r.cardRow(t.id)).toMatchObject({ labelled: 0, status: "done" });
      const o = r.dele.override(g1.id, {});
      if (!o.ok) throw new Error(o.error);
      await r.poll(); // drain the relabel
      return { t, o };
    };
    const a = await mk();
    const b = await mk();
    // The pre-fix relabel left the task done; the cache stayed labelled=0 / done, state open.
    for (const x of [a, b]) {
      r.edit(x.t, { status: "done" });
      r.db.prepare("UPDATE cards SET state = 'open', labelled = 0, status = 'done', display_reason = NULL WHERE task_id = ?").run(x.t.id);
    }
    r.db.prepare("UPDATE cards SET status = 'todo', deleted_at = ? WHERE task_id = ?").run(r.env.now(), b.t.id); // b: deleted, status never left the open set
    expect(r.svc.sweepAbandonedOverrides().swept.sort()).toEqual([a.t.id, b.t.id].sort());
    expect(r.cardRow(a.t.id).state).toBe("closed");
    expect(r.cardRow(b.t.id).state).toBe("closed");
    for (const x of [a, b]) expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(x.o.decision_id)).toEqual({ withdrawn_at: null });
  });

  // Finding 3: the startup sweep must honour the same pending-relabel guard as cardGone: an override whose
  // relabel has not run yet is still about to reopen its task, so a restart in that window must not close it.
  it("mk-cggjk sweep: start() leaves an override alone while its relabel is still pending", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 2);
    expect(r.cardRow(t.id)).toMatchObject({ labelled: 0, status: "done" });
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    expect(r.db.prepare("SELECT state FROM card_writes WHERE kind = 'relabel'").get()).toEqual({ state: "pending" });
    // Both cached shapes the sweep sees in this window: unlabelled+done (as polled) and labelled+done.
    for (const labelled of [0, 1]) {
      r.db.prepare("UPDATE cards SET labelled = ? WHERE task_id = ?").run(labelled, t.id);
      r.restart();
      r.svc.start();
      try {
        expect(r.cardRow(t.id).state, `labelled=${labelled}`).toBe("open");
      } finally {
        r.svc.stop();
      }
    }
    r.restart();
    r.svc.start();
    try {
      expect(r.cardRow(t.id).state).toBe("open");
    } finally {
      r.svc.stop();
    }
    await pollN(r, 3); // the relabel drains and reopens the task
    expect(r.fake.tasks.find((x) => x.id === t.id)!.status).toBe("todo");
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
  });

  // Finding 4: a relabel that backs off must not undo a person's deliberate cancel when it finally runs. The
  // task stays canceled, the override is not withdrawn, and the card leaves the open list (nothing polls it).
  it("mk-cggjk: a delayed override relabel does not reopen a task a person canceled meanwhile", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 2);
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    r.edit(t, { status: "canceled" }); // before the relabel has run
    await pollN(r, 4);
    expect(r.fake.tasks.find((x) => x.id === t.id)!.status).toBe("canceled");
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed" });
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
  });

  // Round 2 finding: a task deleted while its override relabel is pending. cardGone leaves the card open
  // (relabel pending), the writer then skips the write as "task deleted", and nothing polls a deleted task or
  // sweeps until the next restart -- so the card must be closed by the writer, without a restart.
  it("mk-cggjk: a task deleted while its override relabel is pending closes the card without a restart", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r);
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 2);
    const o = r.dele.override(g1.id, {});
    if (!o.ok) throw new Error(o.error);
    r.fake.tasks.splice(r.fake.tasks.findIndex((x) => x.id === t.id), 1); // deleted before the relabel has run
    await pollN(r, 6);
    expect(r.cardRow(t.id)).toMatchObject({ state: "closed" });
    expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });
    expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
  });

  // Round 3 finding: an override card whose task is unlabelled (cached labelled=0, still open) and then canceled
  // or deleted. knownOpenCards() skips labelled=0 rows and the sweep runs only at start, so nothing read the
  // terminal state: the card stayed open. Open override cards are now re-read whatever their cached label.
  for (const how of ["canceled", "deleted"] as const) {
    it(`mk-cggjk: an override card unlabelled, then ${how}, closes without a restart`, async () => {
      const r = rig();
      r.enableDelegation();
      const { t, g1 } = await opened(r);
      expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
      await pollN(r, 2);
      const o = r.dele.override(g1.id, {});
      if (!o.ok) throw new Error(o.error);
      await pollN(r, 3); // the relabel drains and reopens the task
      expect(r.fake.tasks.find((x) => x.id === t.id)!.status).toBe("todo");
      r.edit(t, { labelIds: [] }); // a person removes needs-mk; the task stays open
      await pollN(r, 2);
      expect(r.cardRow(t.id)).toMatchObject({ state: "open", labelled: 0 });
      if (how === "canceled") r.edit(t, { status: "canceled" });
      else r.fake.tasks.splice(r.fake.tasks.findIndex((x) => x.id === t.id), 1);
      await pollN(r, 6);
      expect(r.cardRow(t.id)).toMatchObject({ state: "closed" });
      expect(r.db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(o.decision_id)).toEqual({ withdrawn_at: null });
      expect(r.mkPick(o.decision_id)).toMatchObject({ ok: true });
    });
  }
});
