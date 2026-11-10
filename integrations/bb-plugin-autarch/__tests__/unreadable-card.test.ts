// An unreadable card body must not blank the inbox (bead mk-j5ipa.14): one bad body_json row sits among good cards, and
// every reader still answers, with the bad card marked in its place, not answerable, and its asker told once.
import Database from "better-sqlite3";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { buildQueue } from "../queueview.js";
import { wireHome, type HomeConfig } from "../server.js";
import type { ServeClient } from "../serve.js";
import { createStoreHandle } from "../store.js";
import { cleanupEnvs, opened, rig, type Rig } from "./card-rig.js";
import { ask } from "./service-helpers.js";

afterEach(cleanupEnvs);

const BAD = "{not json";
const cfg: HomeConfig = { serveAddr: "127.0.0.1:8110", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" };
const serve = { projects: async () => [], health: async () => ({ build: { commit: "abc" } }), healthy: async () => true } as unknown as ServeClient;

function fakeBb() {
  const disposers: (() => void)[] = [];
  const bb = {
    log: { info: vi.fn(), warn: vi.fn(), error: vi.fn() },
    onDispose: (fn: () => void) => disposers.push(fn),
    background: { service: () => undefined },
    events: { on: () => undefined },
    agents: { configure: () => undefined },
    cli: { register: () => undefined },
    sdk: { threads: {} },
    realtime: { publish: vi.fn() },
  };
  return { bb: bb as never, dispose: () => disposers.forEach((d) => d()) };
}

/** Writes a bad body the way outside damage would: the immutability trigger guards the product, so this test database drops it first. */
function corrupt(r: Rig, ...ids: string[]) {
  r.db.exec("DROP TRIGGER IF EXISTS decisions_ask_immutable");
  for (const id of ids) r.db.prepare("UPDATE decisions SET body_json = ? WHERE id = ?").run(BAD, id);
}

/** Three open cards, the middle one's stored body corrupted after it was filed. */
async function threeWithOneBad(r: Rig) {
  const a = await opened(r, { key: "k-a", title: "Card A", ident: "0123456789abcdef" });
  const b = await opened(r, { key: "k-b", title: "Card B", ident: "fedcba9876543210", question: "Second question?" });
  const c = await opened(r, { key: "k-c", title: "Card C", ident: "00112233445566aa", question: "Third question?" });
  corrupt(r, b.g1.id);
  return { a, b, c };
}

describe("an unreadable card body (one bad row among good ones)", () => {
  it("the queue still lists every card; the bad one is marked unreadable and carries no options", async () => {
    const r = rig();
    const { a, b, c } = await threeWithOneBad(r);
    const q = buildQueue(r.svc, new Asks(r.svc));
    expect(q.rows.map((x) => x.decision_id).sort()).toEqual([a.g1.id, b.g1.id, c.g1.id].sort());
    const bad = q.rows.find((x) => x.decision_id === b.g1.id)!;
    expect(bad).toMatchObject({ unreadable: true, task_id: b.t.id, ask: { question: "", options: [] } });
    for (const good of q.rows.filter((x) => x.decision_id !== b.g1.id)) {
      expect(good.unreadable).toBeUndefined();
      expect(good.ask!.options.length).toBe(2);
    }
  });

  it("an unreadable ask filed before cards, in the legacy list, does not break the queue either", async () => {
    const r = rig();
    await opened(r, { key: "k-a" });
    const filed = await r.svc.file(ask(r.env), { threadId: "thr-a" });
    if (!filed.ok) throw new Error(filed.error);
    corrupt(r, filed.decision_id);
    const q = buildQueue(r.svc, new Asks(r.svc));
    expect(q.rows).toHaveLength(1);
    expect(q.legacy.owed.map((o) => o.id)).toEqual([filed.decision_id]);
  });

  it("listAsks answers with every owed card, and the bad one is marked with its key", async () => {
    const r = rig();
    const { a, b, c } = await threeWithOneBad(r);
    const f = fakeBb();
    try {
      const home = wireHome(f.bb, createStoreHandle(() => new Database(r.env.file), {}), cfg, { serve });
      const res = await home.handlers.listAsks(null);
      expect(res.owed.map((o) => o.id).sort()).toEqual([a.g1.id, b.g1.id, c.g1.id].sort());
      const bad = res.owed.find((o) => o.id === b.g1.id)!;
      expect(bad).toMatchObject({ unreadable: true, key: r.cardRow(b.t.id).card_key, task_id: b.t.id, ask: { question: "", options: [] } });
      for (const good of res.owed.filter((o) => o.id !== b.g1.id)) {
        expect(good.unreadable).toBeUndefined();
        expect(good.ask.options).toHaveLength(2);
      }
    } finally {
      f.dispose();
    }
  });

  it("the steps list and the machine lists keep answering when one of their asks is unreadable", async () => {
    const r = rig();
    const steps = await r.svc.file(ask(r.env, { kind: "steps", options: undefined, steps: ["one"], subject: "s1", question: "steps one" }), { threadId: "thr-a" });
    const steps2 = await r.svc.file(ask(r.env, { kind: "steps", options: undefined, steps: ["two"], subject: "s2", question: "steps two", thread: "thr-b" }), { threadId: "thr-b" });
    const mach = await r.svc.file(ask(r.env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "runner is down" }, subject: "m1", question: "ci down" }), { threadId: "thr-a" });
    const mach2 = await r.svc.file(ask(r.env, { kind: "machine", options: undefined, machine: { class: "ci", detail: "disk full" }, subject: "m2", question: "disk", thread: "thr-b" }), { threadId: "thr-b" });
    for (const f of [steps, steps2, mach, mach2]) if (!f.ok) throw new Error(f.error);
    corrupt(r, ...[steps, mach].map((f) => (f as { decision_id: string }).decision_id));
    const asks = new Asks(r.svc);
    const runbook = asks.runbook();
    expect(runbook.flatMap((g) => g.items.map((i) => i.subject)).sort()).toEqual(["s1", "s2"]);
    const lists = asks.lists();
    expect([...lists.lane, ...lists.asks].map((e) => e.subject).sort()).toEqual(["m1", "m2"]);
  });

  it("the unreadable card still counts as waiting on mk: counts come from the owed rows, not from the body", async () => {
    const r = rig();
    await threeWithOneBad(r);
    expect(r.svc.owed()).toHaveLength(3);
  });

  it("a pick on the unreadable card is refused with 409, not a crash; the good cards still take picks", async () => {
    const r = rig();
    const { a, b } = await threeWithOneBad(r);
    r.db.prepare("UPDATE cards SET root_state = 'verified' WHERE task_id IN (?, ?)").run(a.t.id, b.t.id);
    const rev = (id: string) => (r.svc.store.decision(id) as { revision: string }).revision;
    const refused = r.svc.pick(b.g1.id, "a", rev(b.g1.id), "p-bad", "mk", "home");
    expect(refused).toMatchObject({ ok: false, status: 409 });
    expect((refused as { error: string }).error).toMatch(/could not be read/);
    expect(r.svc.pick(a.g1.id, "a", rev(a.g1.id), "p-good", "mk", "home")).toMatchObject({ ok: true });
  });

  it("the vizier cannot rule an unreadable card: refused 409, nothing thrown", async () => {
    const r = rig();
    r.enableDelegation();
    const { b } = await threeWithOneBad(r);
    r.db.prepare("UPDATE decisions SET delegable = 1 WHERE id = ?").run(b.g1.id);
    const res = r.dele.rule(b.g1.id, "a", "reversible and routine", { threadId: "thr_viz" });
    expect(res).toMatchObject({ ok: false, status: 409 });
  });

  it("tells the asking thread to file it again, once for the card generation, not on every poll", async () => {
    const r = rig();
    const { b } = await threeWithOneBad(r);
    for (let i = 0; i < 3; i++) {
      await r.poll();
      r.advance(61_000);
    }
    const notes = r.svc.store.obligationsFor(b.g1.id).filter((o) => o.kind === "notice");
    expect(notes).toHaveLength(1);
    expect(notes[0]).toMatchObject({ recipient: "thr_a" });
    expect(notes[0]!.payload).toContain("could not be read");
    expect(notes[0]!.payload).toContain("file it again");
    expect(notes[0]!.payload).toContain(String(r.cardRow(b.t.id).card_key));
  });

  it("a readable card is never told about: no notice is queued for the good cards", async () => {
    const r = rig();
    const { a, c } = await threeWithOneBad(r);
    await r.poll();
    for (const g of [a.g1.id, c.g1.id]) expect(r.svc.store.obligationsFor(g).filter((o) => o.kind === "notice")).toHaveLength(0);
  });
});
