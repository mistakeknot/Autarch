// Bead mk-j5ipa.15 (B7): a card stuck in "observed" (it was filed but never reached Decide) is listed read-only under
// Stalled as "Not delivered yet: <subject>" with its reason. After the second failed try the asking thread is told once,
// through the plugin's own notice obligation. When the card is delivered it leaves Stalled and shows under Decide by itself.
import { afterEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { buildQueue } from "../queueview.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const stuck = async (r: Rig, over: Parameters<Rig["card"]>[0] = {}) => {
  r.env.down.value = true; // serve is down: the root cannot be verified, so the card is not delivered
  const t = r.card({ title: "Collapse per project or per day?", ...over });
  await r.poll();
  return t;
};
const notices = (r: Rig) => r.db.prepare("SELECT * FROM obligations WHERE kind = 'notice' AND payload LIKE '%not delivered%'").all() as any[];

describe("listing", () => {
  it("lists a stuck card with its subject and reason, outside the queue and outside Waiting on you", () => {
    return (async () => {
      const r = rig();
      const t = await stuck(r);
      const list = r.svc.notDelivered();
      expect(list).toHaveLength(1);
      expect(list[0]).toMatchObject({ id: `undelivered:${t.id}`, subject: "Collapse per project or per day?", thread: "thr_a" });
      expect(list[0]!.reason).toMatch(/^root unverified: /);
      const lists = new Asks(r.svc).lists();
      expect(lists.notDelivered).toHaveLength(1);
      // Not a decision to make: not owed, not in the queue payload.
      expect(r.svc.owed()).toHaveLength(0);
      expect(JSON.stringify(buildQueue(r.svc, new Asks(r.svc)))).not.toContain("Not delivered");
      const q = buildQueue(r.svc, new Asks(r.svc));
      expect(Object.keys(q.legacy.machine).sort()).toEqual(["asks", "lane"]);
      expect(q.legacy.count).toBe(0);
      expect(q.rows).toHaveLength(0);
    })();
  });

  it("lists nothing when no card is stuck, and keeps a card that has no asking thread yet", async () => {
    const r = rig();
    await r.poll();
    expect(r.svc.notDelivered()).toEqual([]);
    const t = r.card({ thread: null });
    await r.poll();
    expect(r.svc.notDelivered()).toMatchObject([{ id: `undelivered:${t.id}`, reason: "waiting for asking thread", thread: "" }]);
  });
});

describe("telling the asking thread", () => {
  it("tells it once, after the second failed try and not before", async () => {
    const r = rig();
    const t = await stuck(r);
    expect(r.cardRow(t.id).check_attempts).toBe(1);
    expect(notices(r)).toHaveLength(0);
    r.advance(6_000);
    await r.poll();
    expect(r.cardRow(t.id).check_attempts).toBe(2);
    const n = notices(r);
    expect(n).toHaveLength(1);
    expect(n[0]).toMatchObject({ recipient: "thr_a", kind: "notice" });
    expect(n[0].payload).toContain(t.id);
    for (let i = 0; i < 4; i++) {
      r.advance(300_000);
      await r.poll();
    }
    expect(r.cardRow(t.id).check_attempts).toBeGreaterThan(3);
    expect(notices(r)).toHaveLength(1);
  });

  it("does not tell a thread again after a restart", async () => {
    const r = rig();
    await stuck(r);
    r.advance(6_000);
    await r.poll();
    r.restart();
    r.advance(300_000);
    await r.poll();
    expect(notices(r)).toHaveLength(1);
  });
});

describe("delivery after a notice was queued", () => {
  it("never sends the queued notice once the card is delivered, even after the batching window", async () => {
    const r = rig();
    const t = await stuck(r);
    r.advance(6_000);
    await r.poll();
    expect(notices(r)).toHaveLength(1);
    r.env.down.value = false;
    r.advance(300_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    r.advance(2 * 3_600_000); // well past the notice batching window
    const due = r.svc.store.dueMessagesBatched().filter((o) => o.op === `undelivered:${t.id}`);
    expect(due).toEqual([]);
    expect(notices(r)[0].voided_at).not.toBeNull();
  });
  it("words every notice as a past event, so a batched digest sent after delivery is still true", async () => {
    const r = rig();
    r.env.batch.noticeWindowMs = 3_600_000; // production batching: notices to one thread leave as one digest
    r.restart(); // the Store reads the window when it is built
    const a = await stuck(r, { title: "First question" });
    const b = r.card({ title: "Second question" });
    await r.poll();
    r.advance(6_000);
    await r.poll();
    // Notices wait out the window, then two due for one thread become a digest holding the text while the cards are still undelivered.
    r.advance(3_600_000 + 1_000);
    const made = r.svc.store.batchDueNotices();
    expect(made).toHaveLength(1);
    const digestId = made[0]!.id;
    expect(String(made[0]!.payload)).toContain(a.id);
    expect(String(made[0]!.payload)).toContain(b.id);
    r.env.down.value = false;
    r.advance(300_000);
    await r.poll();
    expect(r.cardRow(a.id).state).toBe("open");
    expect(r.cardRow(b.id).state).toBe("open");
    // The digest survives delivery (it is a separate row) and is sent later with its original text, which must still be true.
    const row = r.svc.store.obligation(digestId)!;
    expect(row.state).toBe("pending");
    const text = String(row.payload);
    expect(text).not.toMatch(/cannot see it yet|is not delivered/);
    expect(text).toMatch(/was not delivered/);
    expect(text).toMatch(/\d{4}-\d{2}-\d{2}T/); // when it was true
    expect(text).toMatch(/may have changed since/);
  });
  it("still tells once, and not again, when a card stalls a second time after delivery", async () => {
    const r = rig();
    const t = await stuck(r);
    r.advance(6_000);
    await r.poll();
    r.env.down.value = false;
    r.advance(300_000);
    await r.poll();
    r.env.down.value = true;
    r.card({ title: "other" });
    for (let i = 0; i < 4; i++) {
      r.advance(300_000);
      await r.poll();
    }
    expect(notices(r).filter((n) => n.op === `undelivered:${t.id}`)).toHaveLength(1);
  });
});

describe("delivery", () => {
  it("moves the card to Decide by itself, and out of the Not delivered list", async () => {
    const r = rig();
    const t = await stuck(r);
    r.advance(6_000);
    await r.poll();
    expect(r.svc.owed()).toHaveLength(0);
    r.env.down.value = false;
    r.advance(300_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    expect(r.svc.notDelivered()).toEqual([]);
    expect(new Asks(r.svc).lists().notDelivered).toEqual([]);
    expect(r.svc.owed().map((o) => o.task_id)).toEqual([t.id]);
    expect(buildQueue(r.svc, new Asks(r.svc)).rows).toHaveLength(1);
  });
});
