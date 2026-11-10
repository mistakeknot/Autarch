// The hold and later lists are read and parsed once per change, not once per card per call (bead mk-j5ipa.3).
// Output must be the same as before: this file checks counts of reads and parses, and that the list, queue and
// waiting results match what the stored values say when each is parsed by hand.
import { afterEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { buildQueue } from "../queueview.js";
import { waitingNow } from "../waiting.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(() => {
  vi.restoreAllMocks();
  cleanupEnvs();
});

const CARDS = 30;
const STALE = 45;

/** 30 open cards, about half held and a few set aside for later, plus stale entries for closed cards. */
async function fixture() {
  const r = rig();
  const ids: string[] = [];
  for (let i = 0; i < CARDS; i++) ids.push(r.card({ key: `pl-${i}`, ident: (i + 1).toString(16).padStart(16, "0"), title: `Card ${i}` }).id);
  await r.poll();
  const holds: Record<string, unknown> = {};
  const laters: Record<string, unknown> = {};
  ids.forEach((id, i) => {
    if (i % 2 === 0) holds[id] = { reason: `superseded script ${i} ` + "x".repeat(200), by: "thr_viz", at: "2026-10-07T00:00:00.000Z", n: `n-${i}` };
    else if (i % 3 === 0) laters[id] = { at: "2026-10-08T00:00:00.000Z", by: "mk" };
  });
  for (let i = 0; i < STALE; i++) holds[`gone-${i}`] = { reason: "closed card " + "y".repeat(240), by: "thr_viz", at: "2026-10-01T00:00:00.000Z", n: `g-${i}` };
  r.svc.store.setSetting("holds", JSON.stringify(holds));
  r.svc.store.setSetting("later", JSON.stringify(laters));
  return { r, ids, holds, laters };
}

/** What listAsks does for each card's hold and later. */
const listCall = (r: Rig) => r.svc.owed().map((d) => ({ id: d.id, held: r.svc.holdOf(d.task_id as string | null), later: r.svc.laterOf(d.task_id as string | null) }));

function spyReads(r: Rig) {
  const seen = { reads: 0, parses: 0 };
  const real = r.svc.store.setting.bind(r.svc.store);
  vi.spyOn(r.svc.store, "setting").mockImplementation((k: string) => {
    if (k === "holds" || k === "later") seen.reads++;
    return real(k);
  });
  const parse = JSON.parse;
  vi.spyOn(JSON, "parse").mockImplementation((s: string, ...a: []) => {
    if (typeof s === "string" && s.length > 5_000 && (s.includes("superseded script") || s.includes("closed card"))) seen.parses++;
    return parse(s, ...a);
  });
  return seen;
}

describe("holds and laters are not re-read per card", () => {
  it("a list call over many cards reads and parses each stored list at most once", async () => {
    const { r } = await fixture();
    const seen = spyReads(r);
    const rows = listCall(r);
    expect(rows).toHaveLength(CARDS);
    expect(seen.reads).toBeLessThanOrEqual(2); // one hold list, one later list
    expect(seen.parses).toBeLessThanOrEqual(1);
  });

  it("a second call with nothing changed reads and parses nothing more", async () => {
    const { r } = await fixture();
    listCall(r);
    const seen = spyReads(r);
    listCall(r);
    expect(seen.reads).toBe(0);
    expect(seen.parses).toBe(0);
  });

  it("the queue and the waiting count each read the lists at most once per call", async () => {
    const { r } = await fixture();
    const seen = spyReads(r);
    buildQueue(r.svc, new Asks(r.svc));
    waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele));
    expect(seen.parses).toBeLessThanOrEqual(1);
    expect(seen.reads).toBeLessThanOrEqual(2);
  });
});

describe("the cache never serves a stale list", () => {
  it("sees a hold or later written through this service, and its removal", async () => {
    const { r, ids } = await fixture();
    const odd = ids[1];
    expect(r.svc.holdOf(odd)).toBeNull();
    r.svc.hold(odd, "new reason", "thr_viz", { until: "2099-01-05" });
    expect(r.svc.holdOf(odd)).toMatchObject({ reason: "new reason" });
    r.svc.unhold(odd);
    expect(r.svc.holdOf(odd)).toBeNull();
    expect(r.svc.laterOf(odd)).toBeNull();
    r.svc.later(odd, "mk");
    expect(r.svc.laterOf(odd)).toMatchObject({ by: "mk" });
    r.svc.unlater(odd);
    expect(r.svc.laterOf(odd)).toBeNull();
  });

  it("sees a value written straight to the store, including a bad one", async () => {
    const { r, ids } = await fixture();
    expect(r.svc.holdOf(ids[0])).not.toBeNull();
    r.svc.store.setSetting("holds", "not json");
    expect(r.svc.holdOf(ids[0])).toBeNull();
    expect(r.svc.holds()).toEqual({});
  });

  it("sees a hold written by another connection to the same database", async () => {
    const { r, ids } = await fixture();
    const other = r.env.open();
    expect(r.svc.holdOf(ids[1])).toBeNull(); // warm the cache
    other.hold(ids[1], "from the other process", "thr_viz", { until: "2099-01-05" });
    expect(r.svc.holdOf(ids[1])).toMatchObject({ reason: "from the other process" });
    other.later(ids[1], "mk");
    expect(r.svc.laterOf(ids[1])).toMatchObject({ by: "mk" });
  });

  it("forgets a write that rolled back", async () => {
    const { r, ids } = await fixture();
    expect(r.svc.holdOf(ids[1])).toBeNull();
    expect(() =>
      r.svc.store.atomically(() => {
        r.svc.store.setSetting("holds", JSON.stringify({ [ids[1]]: { reason: "r", by: "v", at: "2026-10-07T00:00:00.000Z" } }));
        expect(r.svc.holdOf(ids[1])).not.toBeNull();
        throw new Error("abort");
      }),
    ).toThrow("abort");
    expect(r.svc.holdOf(ids[1])).toBeNull();
  });

  it("sees a value changed with plain SQL on the same connection", async () => {
    const { r, ids } = await fixture();
    expect(r.svc.laterOf(ids[3])).not.toBeNull();
    expect(r.svc.holdOf(ids[0])).not.toBeNull();
    r.svc.store.db.prepare("UPDATE settings_kv SET value = '{}' WHERE key = 'later'").run();
    r.svc.store.db.prepare("DELETE FROM settings_kv WHERE key = 'holds'").run();
    expect(r.svc.laterOf(ids[3])).toBeNull();
    expect(r.svc.holdOf(ids[0])).toBeNull();
  });

  it("forgets a write that a plain transaction on the connection rolled back", async () => {
    const { r, ids } = await fixture();
    expect(r.svc.holdOf(ids[1])).toBeNull();
    const t = r.svc.store.db.transaction(() => {
      r.svc.store.setSetting("holds", JSON.stringify({ [ids[1]]: { reason: "r", by: "v", at: "2026-10-07T00:00:00.000Z" } }));
      expect(r.svc.holdOf(ids[1])).not.toBeNull();
      throw new Error("abort");
    });
    expect(() => t()).toThrow("abort");
    expect(r.svc.holdOf(ids[1])).toBeNull();
    expect(r.svc.holdOf(ids[0])).not.toBeNull();
  });

  it("hands out copies, so a caller that edits a result does not change later reads", async () => {
    const { r, ids } = await fixture();
    const h = r.svc.holdOf(ids[0])!;
    h.reason = "edited";
    delete r.svc.holds()[ids[0]];
    delete r.svc.laters()[ids[3]];
    expect(r.svc.holdOf(ids[0])!.reason).not.toBe("edited");
    expect(r.svc.holdOf(ids[0])).not.toBeNull();
    expect(r.svc.laterOf(ids[3])).not.toBeNull();
  });
});

describe("output is unchanged", () => {
  it("every list, queue and waiting result matches the stored lists parsed by hand", async () => {
    const { r, ids, holds, laters } = await fixture();
    const rows = listCall(r);
    for (const row of rows) {
      const d = r.svc.store.decision(row.id) as { task_id: string };
      const want = holds[d.task_id] as { reason: string; by: string; at: string; n: string } | undefined;
      expect(row.held).toEqual(want ? { reason: want.reason, by: want.by, at: want.at, n: want.n } : null);
      expect(row.later).toEqual((laters[d.task_id] as object | undefined) ?? null);
    }
    const heldIds = ids.filter((_, i) => i % 2 === 0);
    const q = buildQueue(r.svc, new Asks(r.svc));
    expect(q.rows.filter((x) => x.held).map((x) => x.task_id).sort()).toEqual([...heldIds].sort());
    const w = waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele));
    expect(w.held).toBe(heldIds.length);
    expect(w.later).toBe(ids.filter((_, i) => i % 2 === 1 && i % 3 === 0).length);
    expect(w.decide).toBe(CARDS - heldIds.length - w.later);
    expect(r.svc.laterTasks()).toEqual(new Set(Object.keys(laters)));
    // A fresh service (nothing cached) gives the same answers.
    r.restart();
    expect(listCall(r)).toEqual(rows);
    expect(waitingNow(r.svc, r.dele, new Catchup(r.svc, r.dele))).toEqual(w);
  });
});

describe("isOwed", () => {
  it("agrees with owed() for an open ask, a picked one, a withdrawn one, a replaced one and an unknown id", async () => {
    const r = rig();
    const mk = (i: number) => r.card({ key: `io-${i}`, ident: (i + 1).toString(16).padStart(16, "0") });
    const [a, b, c] = [mk(0), mk(1), mk(2)];
    await r.poll();
    const dec = (t: { id: string }) => (r.gens(t.id)[0] as { id: string }).id;
    const d = mk(3);
    r.mkPick(dec(b));
    r.svc.store.withdrawDecision(dec(c), "test");
    await r.poll();
    // A replacement row (as a legacy ask's replace leaves it) takes the first generation out of owed() and is owed itself.
    const g1 = dec(d);
    const proto = r.db.prepare("SELECT * FROM decisions WHERE id = ?").get(g1) as Record<string, unknown>;
    const cols = Object.keys(proto);
    const v: Record<string, unknown> = { ...proto, id: "io-replacement", request_id: "req-io-r", identity: "ident-io-r", semantic_key: "key-io-r", generation: 2, supersedes: g1 };
    r.db.prepare(`INSERT INTO decisions (${cols.join(",")}) VALUES (${cols.map(() => "?").join(",")})`).run(...cols.map((c) => v[c]));
    const g2 = "io-replacement";
    const open = new Set(r.svc.owed().map((x) => x.id));
    expect(open.has(dec(a))).toBe(true);
    const all = [dec(a), dec(b), dec(c), g1, g2, "no-such-decision"];
    for (const id of all) expect(r.svc.isOwed(id)).toBe(open.has(id));
    expect(all.map((id) => r.svc.isOwed(id))).toEqual([true, false, false, false, true, false]);
  });
});
