import Database from "better-sqlite3";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { existsSync, statSync } from "node:fs";
import { join } from "node:path";
import { createStoreHandle, Store } from "../store.js";
import { decision, openStore, pickOf, tmpDir } from "./helpers.js";

let t: ReturnType<typeof tmpDir>;
let file: string;
beforeEach(() => {
  t = tmpDir();
  file = join(t.dir, "data.db");
});
afterEach(() => t.cleanup());

const count = (s: Store, table: string) =>
  (s.db.prepare(`SELECT COUNT(*) c FROM ${table}`).get() as { c: number }).c;

describe("filing", () => {
  it("files the same request_id once and returns the registry row", () => {
    const s = openStore(file);
    const d = decision();
    expect(s.insertDecision(d)).toEqual({ inserted: true, decision_id: d.id });
    const again = s.insertDecision({ ...d, id: "other-id" });
    expect(again.inserted).toBe(false);
    if (!again.inserted) expect(again.existing.decision_id).toBe(d.id);
    expect(count(s, "decisions")).toBe(1);
    expect(count(s, "requests")).toBe(1);
  });

  it("two connections to one file filing one request give one row and both see its id", () => {
    const a = openStore(file);
    const b = openStore(file);
    const d = decision();
    const ra = a.insertDecision(d);
    const rb = b.insertDecision({ ...d, id: "loser" });
    expect(ra.inserted).toBe(true);
    expect(rb.inserted).toBe(false);
    if (!rb.inserted) expect(rb.existing.decision_id).toBe(d.id);
    expect(count(a, "decisions")).toBe(1);
  });

  it("a filing and a mention sharing a request_id leave one requests row; the loser gets the winner's result", () => {
    const a = openStore(file);
    const b = openStore(file);
    const d = decision();
    a.insertDecision(d);
    const m = b.insertMention({ request_id: d.request_id, identity: "x", decision_id: d.id, thread: "t2" });
    expect(m.inserted).toBe(false);
    if (!m.inserted) expect(m.existing.result).toBe("decision");
    expect(count(a, "requests")).toBe(1);
    expect(count(a, "mentions")).toBe(0);

    const first = b.insertMention({ request_id: "m1", identity: "i", decision_id: d.id, thread: "t2" });
    expect(first.inserted).toBe(true);
    const asDecision = a.insertDecision(decision({ request_id: "m1" }));
    expect(asDecision.inserted).toBe(false);
    if (!asDecision.inserted) expect(asDecision.existing.result).toBe("mention");
  });
});

describe("recordPick", () => {
  it("two connections racing on one decision: exactly one wins", () => {
    const a = openStore(file);
    const b = openStore(file);
    const d = decision();
    a.insertDecision(d);
    const ra = a.recordPick(pickOf(d.id), [{ id: "o1", kind: "notify" }]);
    const rb = b.recordPick(pickOf(d.id, { option_id: "b" }), [{ id: "o2", kind: "notify" }]);
    expect([ra.ok, rb.ok].filter(Boolean)).toHaveLength(1);
    expect(rb).toMatchObject({ ok: false, reason: "already-ruled" });
    expect(count(a, "picks")).toBe(1);
    expect(count(a, "obligations")).toBe(1);
    expect(a.events().map((e) => e.type)).toEqual(["filed", "picked"]);
  });

  it("refuses superseded, withdrawn and stale decisions and writes nothing", () => {
    const s = openStore(file);
    const a = decision();
    s.insertDecision(a);
    const b = decision({ supersedes: a.id, asker: a.asker, thread: a.thread, project: a.project });
    expect(s.insertReplacement(b)).toMatchObject({ ok: true });
    expect(s.recordPick(pickOf(a.id), [{ id: "o", kind: "n" }])).toMatchObject({ ok: false, reason: "superseded" });

    const w = decision();
    s.insertDecision(w);
    s.db.prepare("UPDATE decisions SET withdrawn_at = 'x' WHERE id = ?").run(w.id);
    expect(s.recordPick(pickOf(w.id))).toMatchObject({ ok: false, reason: "withdrawn" });

    const st = decision();
    s.insertDecision(st);
    expect(s.recordPick(pickOf(st.id, { revision: "old" }))).toMatchObject({ ok: false, reason: "stale" });
    expect(count(s, "picks")).toBe(0);
    expect(count(s, "obligations")).toBe(0);
  });

  it("refuses a second decision with the same supersedes at the unique index", () => {
    const s = openStore(file);
    const a = decision();
    s.insertDecision(a);
    const mk = () => decision({ supersedes: a.id, asker: a.asker, thread: a.thread, project: a.project });
    expect(s.insertReplacement(mk())).toMatchObject({ ok: true });
    expect(s.insertReplacement(mk())).toEqual({ ok: false, reason: "already-superseded" });
    expect(() =>
      s.db
        .prepare(
          `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, supersedes, filed_at, updated_at)
           VALUES ('z','zr','i','r','s','s','decide','p','a','t','{}',?, 'x','x')`,
        )
        .run(a.id),
    ).toThrow(/UNIQUE/);
  });
});

describe("replacement vs pick across connections", () => {
  it("pick first: the replacement is refused already-ruled and no row exists", () => {
    const a = openStore(file);
    const b = openStore(file);
    const d = decision();
    a.insertDecision(d);
    b.recordPick(pickOf(d.id));
    const r = decision({ supersedes: d.id, asker: d.asker, thread: d.thread, project: d.project });
    expect(a.insertReplacement(r)).toEqual({ ok: false, reason: "already-ruled" });
    expect(a.decision(r.id)).toBeUndefined();
    expect(a.registry(r.request_id)).toBeUndefined();
  });

  it("replacement first: the pick returns superseded", () => {
    const a = openStore(file);
    const b = openStore(file);
    const d = decision();
    a.insertDecision(d);
    a.insertReplacement(decision({ supersedes: d.id, asker: d.asker, thread: d.thread, project: d.project }));
    expect(b.recordPick(pickOf(d.id))).toMatchObject({ ok: false, reason: "superseded" });
  });

  it("refuses non-decide kinds and mismatched asker, and keeps non-delegable non-delegable", () => {
    const s = openStore(file);
    const m = decision({ kind: "machine" });
    s.insertDecision(m);
    expect(s.insertReplacement(decision({ supersedes: m.id, asker: m.asker, thread: m.thread, project: m.project }))).toEqual({
      ok: false,
      reason: "not-replaceable",
    });
    const d = decision({ delegable: false });
    s.insertDecision(d);
    expect(s.insertReplacement(decision({ supersedes: d.id, asker: "someone-else", thread: d.thread, project: d.project }))).toEqual({
      ok: false,
      reason: "not-replaceable",
    });
    const r = decision({ supersedes: d.id, asker: d.asker, thread: d.thread, project: d.project });
    expect(s.insertReplacement(r)).toMatchObject({ ok: true });
    expect((s.decision(r.id) as { delegable: number }).delegable).toBe(0);
  });

  it("withdrawn predecessor is refused withdrawn", () => {
    const s = openStore(file);
    const d = decision();
    s.insertDecision(d);
    s.db.prepare("UPDATE decisions SET withdrawn_at = 'x' WHERE id = ?").run(d.id);
    expect(s.insertReplacement(decision({ supersedes: d.id, asker: d.asker, thread: d.thread, project: d.project }))).toEqual({
      ok: false,
      reason: "withdrawn",
    });
  });
});

describe("override with mentions", () => {
  it("copies mention rows, keeps original registry rows, and walks to the vizier ruling", () => {
    const s = openStore(file);
    const a = decision();
    s.insertDecision(a);
    s.insertMention({ request_id: "R1", identity: "i1", decision_id: a.id, thread: "B" });
    s.insertMention({ request_id: "R2", identity: "i2", decision_id: a.id, thread: "C" });
    // replace-mode refuses an override target without a vizier pick
    const o1 = decision({ supersedes: a.id, asker: a.asker, thread: a.thread, project: a.project });
    expect(s.insertReplacement(o1, "override")).toEqual({ ok: false, reason: "not-replaceable" });
    s.recordPick(pickOf(a.id, { by: "vizier" }));
    const o = decision({ supersedes: a.id, asker: a.asker, thread: a.thread, project: a.project });
    expect(s.insertReplacement(o, "override")).toMatchObject({ ok: true });
    expect(s.mentions(o.id).map((m) => m.thread)).toEqual(["B", "C"]);
    expect(s.mentions(a.id).map((m) => m.thread)).toEqual(["B", "C"]);
    const again = s.insertMention({ request_id: "R1", identity: "i1", decision_id: a.id, thread: "B" });
    expect(again.inserted).toBe(false);
    if (!again.inserted) expect(again.existing).toMatchObject({ result: "mention", decision_id: a.id });
    expect(s.overriddenRuling(o.id)?.decision_id).toBe(a.id);
    expect(s.overriddenRuling(a.id)).toBeNull();
  });
});

describe("obligations", () => {
  const setup = () => {
    const s = openStore(file);
    const d = decision();
    s.insertDecision(d);
    return { s, d };
  };

  it("transition with a stale attempt or state changes nothing; done never goes back; voided never re-enters", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [{ id: "o1", kind: "notify" }]);
    expect(s.transition("o1", "pending", "sending", 5)).toEqual({ ok: false, reason: "lost-race" });
    expect(s.transition("o1", "queued", "done", 0)).toEqual({ ok: false, reason: "lost-race" });
    expect(s.obligation("o1")?.state).toBe("pending");
    expect(s.transition("o1", "pending", "done", 0)).toEqual({ ok: true });
    expect(s.transition("o1", "done", "pending", 0)).toEqual({ ok: false, reason: "final" });
    expect(s.obligation("o1")?.state).toBe("done");

    s.insertObligations(d.id, [{ id: "o2", kind: "notify" }]);
    s.voidObligation("o2");
    expect(s.transition("o2", "pending", "pending", 0)).toEqual({ ok: false, reason: "voided" });
    expect(s.transition("o2", "pending", "sending", 0)).toEqual({ ok: false, reason: "voided" });
    expect(s.obligation("o2")?.state).toBe("pending");
  });

  it("voiding a done row leaves it done", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [{ id: "o1", kind: "notify" }]);
    s.transition("o1", "pending", "done", 0);
    s.voidObligation("o1");
    expect(s.obligation("o1")).toMatchObject({ state: "done" });
    expect(s.obligation("o1")?.voided_at).not.toBeNull();
  });

  it("dueMessages withholds a row until its after_id row is finished and never returns voided rows", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [
      { id: "first", kind: "notify" },
      { id: "second", kind: "notify", after_id: "first" },
      { id: "file", kind: "ruling-file" },
    ]);
    expect(s.dueMessages().map((o) => o.id)).toEqual(["first"]);
    s.transition("first", "pending", "done", 0);
    expect(s.dueMessages().map((o) => o.id)).toEqual(["second"]);
    s.voidObligation("second");
    expect(s.dueMessages()).toEqual([]);
  });

  it("a voided predecessor with no unsettled attempt counts as finished; with a queued attempt it does not", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [
      { id: "p", kind: "notify" },
      { id: "n", kind: "notify", after_id: "p" },
    ]);
    const c = s.claim("p", 0);
    expect(c).toEqual({ ok: true, attempt: 1 });
    s.updateAttempt("p", 1, "queued", { handle: "row-1" });
    s.transition("p", "sending", "queued", 1);
    s.voidObligation("p");
    expect(s.dueMessages()).toEqual([]);
    s.updateAttempt("p", 1, "not-delivered");
    expect(s.dueMessages().map((o) => o.id)).toEqual(["n"]);
  });

  it("attempts and the permit", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [{ id: "o", kind: "notify" }]);
    expect(s.claim("o", 0)).toEqual({ ok: true, attempt: 1 });
    s.updateAttempt("o", 1, "uncertain", { error: "timeout" });
    s.transition("o", "sending", "uncertain", 1);
    expect(s.dueMessages()).toEqual([]);

    // resend: once per click id
    expect(s.resend("o", 1, "click-1")).toEqual({ ok: true, replay: false });
    expect(s.resend("o", 1, "click-1")).toEqual({ ok: true, replay: true });
    expect(s.resend("o", 1, "click-2")).toEqual({ ok: false, reason: "stale" });
    expect(s.obligation("o")).toMatchObject({ state: "pending", resend_permit: 1 });
    // due only while the permit is set; reconciling never derives uncertain over it
    expect(s.dueMessages().map((o) => o.id)).toEqual(["o"]);
    expect(s.transition("o", "pending", "uncertain", 1)).toEqual({ ok: false, reason: "permit" });
    expect(s.obligation("o")?.state).toBe("pending");

    // claim clears the permit with the attempt insert; both attempts are kept
    expect(s.claim("o", 1)).toEqual({ ok: true, attempt: 2 });
    expect(s.obligation("o")).toMatchObject({ state: "sending", resend_permit: 0, attempt: 2 });
    s.updateAttempt("o", 2, "queued", { handle: "r" });
    expect(s.attempts("o").map((a) => [a.n, a.state])).toEqual([
      [1, "uncertain"],
      [2, "queued"],
    ]);
    // delivered is never changed by a later event
    expect(s.updateAttempt("o", 2, "delivered", { evidence: "evt-1" })).toBe(true);
    expect(s.updateAttempt("o", 2, "uncertain")).toBe(false);
    expect(s.updateAttempt("o", 2, "not-delivered")).toBe(false);
    expect(s.attempts("o")[1]).toMatchObject({ state: "delivered", evidence: "evt-1" });
  });

  it("without the permit a pending row with an uncertain attempt is not due", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [{ id: "o", kind: "notify" }]);
    s.claim("o", 0);
    s.updateAttempt("o", 1, "uncertain");
    s.db.prepare("UPDATE obligations SET state = 'pending' WHERE id = 'o'").run();
    expect(s.dueMessages()).toEqual([]);
  });

  it("claim after voided_at is set changes nothing; a lost claim reports not ok", () => {
    const { s, d } = setup();
    s.insertObligations(d.id, [{ id: "o", kind: "notify" }]);
    s.voidObligation("o");
    expect(s.claim("o", 0)).toEqual({ ok: false });
    expect(s.attempts("o")).toEqual([]);
    s.insertObligations(d.id, [{ id: "p", kind: "notify" }]);
    expect(s.claim("p", 0)).toEqual({ ok: true, attempt: 1 });
    expect(s.claim("p", 0)).toEqual({ ok: false });
  });

  it("a recipient set inserts all rows in one transaction and a retry inserts none", () => {
    const { s, d } = setup();
    const rows = ["a", "b", "c"].map((th) => ({ id: `id-${th}`, kind: "steps-done", recipient: th, op: `steps-done:${d.id}:${th}` }));
    expect(s.insertObligations(d.id, rows)).toBe(3);
    expect(s.insertObligations(d.id, rows.map((r) => ({ ...r, id: `${r.id}-again` })))).toBe(0);
    expect(count(s, "obligations")).toBe(3);
  });

  it("queue events that arrive before the row is bound are kept once", () => {
    const { s } = setup();
    s.recordQueueEvent("row-9", "dispatched", "t1");
    s.recordQueueEvent("row-9", "cancelled", "t2");
    expect(s.queueEvent("row-9")).toMatchObject({ type: "dispatched" });
  });
});

describe("store handle", () => {
  it("a database held by another connection is a quiesce refusal: not-ready, never retried (plan 8.9 item 1)", async () => {
    const holder = new Database(file);
    holder.pragma("journal_mode = WAL");
    holder.exec("BEGIN EXCLUSIVE");
    let opens = 0;
    const handle = createStoreHandle(() => (opens++, new Database(file)), {
      initialDelayMs: 20,
      maxDelayMs: 40,
      busyTimeoutMs: 20,
      closeOnFailure: true,
    });
    expect(handle.ready()).toBe(false);
    expect(handle.error()).toMatch(/another connection holds data\.db/);
    expect(handle.refusal()).toBeTruthy();
    expect(() => handle.store()).toThrow(/not ready/);
    holder.exec("ROLLBACK");
    holder.close();
    await new Promise((r) => setTimeout(r, 200));
    expect(opens).toBe(1);
    expect(handle.ready()).toBe(false);
    handle.dispose();
  });

  it("a transient open failure returns not-ready and opens after the cause clears without a reload", async () => {
    let fail = true;
    const handle = createStoreHandle(
      () => {
        if (fail) throw Object.assign(new Error("database is locked"), { code: "SQLITE_BUSY" });
        return new Database(file);
      },
      { initialDelayMs: 20, maxDelayMs: 40, busyTimeoutMs: 20, closeOnFailure: true },
    );
    expect(handle.ready()).toBe(false);
    expect(handle.error()).toMatch(/locked|busy/i);
    expect(handle.refusal()).toBeNull();
    expect(() => handle.store()).toThrow(/not ready/);
    fail = false;
    const deadline = Date.now() + 3000;
    while (!handle.ready() && Date.now() < deadline) await new Promise((r) => setTimeout(r, 20));
    expect(handle.ready()).toBe(true);
    expect(handle.error()).toBeNull();
    handle.dispose();
    handle.store().close();
  });
});

describe("v3 store level", () => {
  it("close() checkpoints with TRUNCATE: the -wal file is absent or empty", () => {
    const s = openStore(file);
    for (let i = 0; i < 20; i++) s.insertDecision(decision());
    s.close();
    const wal = `${file}-wal`;
    expect(!existsSync(wal) || statSync(wal).size === 0).toBe(true);
  });

  it("backfillCardWrites inserts the missing comment and unlabel writes for picked card generations, once", () => {
    const s = openStore(file);
    const ins = (id: string, extra: Record<string, unknown>) => {
      const base: Record<string, unknown> = {
        id, request_id: `req-${id}`, identity: "i", revision: "rev-1", semantic_key: id, subject: "s", kind: "decide",
        project: "autarch", asker: "a", thread: "t", body_json: "{}", filed_at: "x", updated_at: "x", ...extra,
      };
      const cols = Object.keys(base);
      s.db.prepare(`INSERT INTO decisions(${cols.join(",")}) VALUES (${cols.map((c) => "@" + c).join(",")})`).run(base);
    };
    const pick = (id: string) =>
      s.db.prepare("INSERT INTO picks(decision_id, pick_id, option_id, revision, \"by\", surface, picked_at) VALUES (?, ?, 'a', 'rev-1', 'vizier', 'home', 'x')").run(id, `p-${id}`);
    ins("card-T1-g1", { source: "card", task_id: "T1", generation: 1, tasks_project_id: "P", card_fp: "f" });
    ins("card-T2-g1", { source: "card", task_id: "T2", generation: 1, tasks_project_id: "P", card_fp: "f" }); // unpicked
    ins("card-T3-g1", { source: "card", task_id: "T3", generation: 1, tasks_project_id: "P", card_fp: "f" });
    ins("legacy", {});
    pick("card-T1-g1");
    pick("card-T3-g1");
    pick("legacy");
    s.db.prepare("INSERT INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at) VALUES ('keep','T3','card-T3-g1','comment','{}','done','t','t')").run();
    expect(s.backfillCardWrites()).toBe(3); // T1: comment + unlabel; T3: unlabel (comment exists)
    expect(s.backfillCardWrites()).toBe(0);
    const rows = s.db.prepare("SELECT task_id, decision_id, kind, state FROM card_writes ORDER BY decision_id, kind").all();
    expect(rows).toEqual([
      { task_id: "T1", decision_id: "card-T1-g1", kind: "comment", state: "pending" },
      { task_id: "T1", decision_id: "card-T1-g1", kind: "unlabel", state: "pending" },
      { task_id: "T3", decision_id: "card-T3-g1", kind: "comment", state: "done" },
      { task_id: "T3", decision_id: "card-T3-g1", kind: "unlabel", state: "pending" },
    ]);
  });

  it("opening a store runs the backfill invariant check", () => {
    const s = openStore(file);
    s.db.prepare(`INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, project, asker, thread, body_json, filed_at, updated_at, source, task_id, generation, tasks_project_id, card_fp)
                  VALUES ('card-T1-g1','r','i','rev-1','s','s','decide','autarch','a','t','{}','x','x','card','T1',1,'P','f')`).run();
    s.db.prepare("INSERT INTO picks(decision_id, pick_id, option_id, revision, \"by\", surface, picked_at) VALUES ('card-T1-g1','p','a','rev-1','mk','home','x')").run();
    s.close();
    const again = openStore(file);
    expect(count(again, "card_writes")).toBe(2);
  });
});
