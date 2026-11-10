// Bead mk-mn7yz (follow-up to mk-j5ipa.15): a card stuck before delivery whose asking thread is unknown (still waiting for the
// asking thread, unroutable, or a project-binding mismatch) used to tell no one. After the second failed try it now tells the
// card's project coordinator (the one unarchived thread titled "<Project> | coordinator"), or the vizier when there is none or
// more than one. It uses the same notice obligation and the same once-per-card op as the asking-thread notice, and nothing mk
// sees changes.
import { afterEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Delegation, type ThreadRow } from "../delegation.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const row = (id: string, title: string | null, over: Partial<ThreadRow> = {}): ThreadRow => ({ id, title, pinnedAt: null, archivedAt: null, deletedAt: null, ...over });

/** A rig whose notice recipients come from a mutable thread list and a fixed vizier. */
function threadRig(threads: ThreadRow[], vizier: string | null = "thr_viz") {
  const state = { threads, vizier, listFails: false, lists: 0 };
  const r = rig({
    svc: {
      threadSource: {
        list: async () => {
          state.lists++;
          if (state.listFails) throw new Error("threads unavailable");
          return state.threads;
        },
        vizier: async () => state.vizier,
      },
    },
  });
  return { r, state };
}

const noticesFor = (r: Rig, taskId: string) => r.db.prepare("SELECT * FROM obligations WHERE kind = 'notice' AND op LIKE ?").all(`undelivered:${taskId}%`) as any[];

/** A card with no asking thread, polled twice so the second failed try has happened. */
async function unknownAsker(r: Rig, over: Parameters<Rig["card"]>[0] = {}) {
  const t = r.card({ thread: null, title: "Collapse per project or per day?", ...over });
  await r.poll();
  r.advance(6_000);
  await r.poll();
  return t;
}

describe("an undelivered card whose asker is unknown", () => {
  it("tells the project's one live coordinator, after the second failed try and not before", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator"), row("thr_other", "Other | coordinator")]);
    const t = r.card({ thread: null, title: "Collapse per project or per day?" });
    await r.poll();
    expect(r.cardRow(t.id).check_attempts).toBe(1);
    expect(noticesFor(r, t.id)).toHaveLength(0);
    r.advance(6_000);
    await r.poll();
    expect(r.cardRow(t.id).check_attempts).toBe(2);
    const n = noticesFor(r, t.id);
    expect(n).toHaveLength(1);
    expect(n[0]).toMatchObject({ recipient: "thr_coord", kind: "notice" });
  });

  it("names the card key, the generation, the reason and what the receiver can do", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    const text = String(noticesFor(r, t.id)[0].payload);
    expect(text).toContain(r.svc.cardKey(t.id) ?? t.id);
    expect(text).toMatch(/generation 1/);
    expect(text).toContain("waiting for asking thread");
    expect(text).toMatch(/claim/i);
    expect(text).toMatch(/rebind/i);
    expect(text).toMatch(/re-file/i);
    // Worded as a past event with its time, like the asking-thread notice: a batched digest can leave later.
    expect(text).toMatch(/\d{4}-\d{2}-\d{2}T/);
  });

  it("tells the vizier when the project has no coordinator", async () => {
    const { r } = threadRig([row("thr_other", "Other | coordinator"), row("thr_x", "Autarch | mk-1 some executor")]);
    const t = await unknownAsker(r);
    const n = noticesFor(r, t.id);
    expect(n).toHaveLength(1);
    expect(n[0].recipient).toBe("thr_viz");
  });

  it("tells the vizier when the project has two live coordinators", async () => {
    const { r } = threadRig([row("thr_c1", "Autarch | coordinator"), row("thr_c2", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    const n = noticesFor(r, t.id);
    expect(n).toHaveLength(1);
    expect(n[0].recipient).toBe("thr_viz");
  });

  it("never tells an archived coordinator: a project with only an archived one goes to the vizier", async () => {
    const { r } = threadRig([row("thr_old", "Autarch | coordinator", { archivedAt: 1 })]);
    const t = await unknownAsker(r);
    const n = noticesFor(r, t.id);
    expect(n).toHaveLength(1);
    expect(n[0].recipient).toBe("thr_viz");
  });

  it("never tells a deleted coordinator either", async () => {
    const { r } = threadRig([row("thr_gone", "Autarch | coordinator", { deletedAt: 1 })]);
    const t = await unknownAsker(r);
    expect(noticesFor(r, t.id).map((o) => o.recipient)).toEqual(["thr_viz"]);
  });

  it("ignores an archived coordinator beside one live one", async () => {
    const { r } = threadRig([row("thr_old", "Autarch | coordinator", { archivedAt: 1 }), row("thr_new", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    expect(noticesFor(r, t.id)[0].recipient).toBe("thr_new");
  });

  it("tells no one, and retries on the next poll, when the thread list cannot be read", async () => {
    const { r, state } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    state.listFails = true;
    const t = await unknownAsker(r);
    expect(noticesFor(r, t.id)).toHaveLength(0);
    state.listFails = false;
    r.advance(300_000);
    await r.poll();
    expect(noticesFor(r, t.id)).toMatchObject([{ recipient: "thr_coord" }]);
  });

  it("tells no one when there is no coordinator and no vizier", async () => {
    const { r } = threadRig([], null);
    const t = await unknownAsker(r);
    expect(noticesFor(r, t.id)).toHaveLength(0);
  });

  it("tells the receiver once across many polls and a restart", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    for (let i = 0; i < 4; i++) {
      r.advance(300_000);
      await r.poll();
    }
    expect(r.cardRow(t.id).check_attempts).toBeGreaterThan(3);
    expect(noticesFor(r, t.id)).toHaveLength(1);
    r.restart();
    r.advance(300_000);
    await r.poll();
    expect(noticesFor(r, t.id)).toHaveLength(1);
  });

  it("does not list every thread again on later polls once the notice is queued", async () => {
    const { r, state } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    const after = state.lists;
    expect(after).toBeGreaterThan(0);
    for (let i = 0; i < 3; i++) {
      r.advance(300_000);
      await r.poll();
    }
    expect(r.cardRow(t.id).check_attempts).toBeGreaterThan(3);
    expect(state.lists).toBe(after);
  });

  it("tells a re-filed card again: a new card is a new generation with its own notice", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const first = await unknownAsker(r, { title: "First" });
    const second = r.card({ thread: null, title: "Second (re-filed)" });
    await r.poll();
    r.advance(6_000);
    await r.poll();
    expect(noticesFor(r, first.id)).toHaveLength(1);
    expect(noticesFor(r, second.id)).toHaveLength(1);
    expect(noticesFor(r, second.id)[0].recipient).toBe("thr_coord");
  });

  it("voids the queued notice once the card is delivered, like the asking-thread notice", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    expect(noticesFor(r, t.id)).toHaveLength(1);
    r.fake.addComment(t.id, { threadId: "thr_a", createdAt: "2026-10-01T00:00:02.000Z" });
    r.advance(300_000);
    await r.poll();
    expect(r.cardRow(t.id).state).toBe("open");
    expect(noticesFor(r, t.id)[0].voided_at).not.toBeNull();
  });

  it("changes nothing mk sees: the card stays listed under Not delivered with its reason", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    const t = await unknownAsker(r);
    const list = new Asks(r.svc).lists().notDelivered;
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ id: `undelivered:${t.id}`, reason: "waiting for asking thread", thread: "" });
    expect(r.svc.owed()).toHaveLength(0);
  });
});

describe("a known asker", () => {
  it("is still told, and neither the coordinator nor the vizier is", async () => {
    const { r } = threadRig([row("thr_coord", "Autarch | coordinator")]);
    r.env.down.value = true; // serve is down: the root cannot be verified, so the card is not delivered, and its asker is known
    const t = r.card({ title: "Collapse per project or per day?" });
    await r.poll();
    r.advance(6_000);
    await r.poll();
    const n = noticesFor(r, t.id);
    expect(n).toHaveLength(1);
    expect(n[0].recipient).toBe("thr_a");
  });
});

describe("wired through Delegation", () => {
  it("finds the coordinator in the host's thread list and the vizier by its pinned title", async () => {
    const threads: ThreadRow[] = [row("thr_viz9", "Masaq' | vizier", { pinnedAt: 1 })];
    const r = rig();
    new Delegation(r.svc, async () => threads);
    const a = await unknownAsker(r, { title: "No coordinator yet" });
    expect(noticesFor(r, a.id)[0].recipient).toBe("thr_viz9");
    threads.push(row("thr_coord9", "Autarch | coordinator"));
    const b = await unknownAsker(r, { title: "Now there is one" });
    expect(noticesFor(r, b.id)[0].recipient).toBe("thr_coord9");
  });
});
