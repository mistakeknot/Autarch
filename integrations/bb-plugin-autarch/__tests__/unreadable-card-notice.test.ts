// The notice to the asker of an unreadable card (bead mk-j5ipa.14): one per card generation, none repeated by a poll or a
// restart, nothing written by a read, and delivered through the real wake loop.
import Database from "better-sqlite3";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Asks } from "../asks.js";
import { buildQueue } from "../queueview.js";
import { WakeLoop } from "../wakes.js";
import { cleanupEnvs, opened, pollN, rig, type Rig } from "./card-rig.js";
import { FakeSdk } from "./wakes-helpers.js";

afterEach(cleanupEnvs);

/** Writes a bad body the way outside damage would: the immutability trigger guards the product, so this test database drops it first. */
function corrupt(r: Rig, id: string) {
  r.db.exec("DROP TRIGGER IF EXISTS decisions_ask_immutable");
  r.db.prepare("UPDATE decisions SET body_json = ? WHERE id = ?").run("{not json", id);
}
const notices = (r: Rig, id: string) => r.svc.store.obligationsFor(id).filter((o) => o.kind === "notice");

describe("the unreadable-card notice", () => {
  it("a re-filed card (a second generation) is told once, and the first generation's notice is not repeated", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { key: "k-a", ident: "0123456789abcdef" });
    corrupt(r, g1.id);
    await pollN(r, 3);
    expect(notices(r, g1.id)).toHaveLength(1);
    // The asker files it again: the card is edited, which is a new generation with a body of its own.
    r.edit(t, { description: r.desc({ key: "k-a", question: "A changed question?" }) });
    await pollN(r, 2);
    const g2 = r.gens(t.id).find((g) => g.generation === 2)!;
    expect(g2).toBeDefined();
    expect(notices(r, g2.id)).toHaveLength(0); // a readable generation is never told about
    corrupt(r, g2.id);
    await pollN(r, 4);
    expect(notices(r, g2.id)).toHaveLength(1);
    expect(notices(r, g2.id)[0]!.id).not.toBe(notices(r, g1.id)[0]!.id);
    expect(notices(r, g1.id)).toHaveLength(1);
  });

  it("repeated polls and a restart queue no second notice for the same generation, and wake the loop once", async () => {
    const nudge = vi.fn();
    const r = rig({ svc: { nudge } });
    const { g1 } = await opened(r, { key: "k-a" });
    corrupt(r, g1.id);
    const before = nudge.mock.calls.length;
    await r.poll();
    expect(notices(r, g1.id)).toHaveLength(1);
    expect(nudge.mock.calls.length).toBe(before + 1);
    await pollN(r, 3);
    r.restart();
    await pollN(r, 3);
    expect(notices(r, g1.id)).toHaveLength(1);
    expect(r.svc.tellUnreadable()).toBe(0);
    expect(nudge.mock.calls.length).toBe(before + 1);
  });

  it("reading an unreadable card changes no stored row", async () => {
    const r = rig();
    const { g1 } = await opened(r, { key: "k-a" });
    await opened(r, { key: "k-b", ident: "fedcba9876543210" });
    corrupt(r, g1.id);
    const dump = () => {
      const db = r.db as Database.Database;
      return ["decisions", "cards", "obligations", "decision_blocks"].map((tb) => db.prepare(`SELECT * FROM ${tb} ORDER BY 1`).all());
    };
    const before = JSON.stringify(dump());
    const asks = new Asks(r.svc);
    buildQueue(r.svc, asks);
    asks.runbook();
    asks.lists();
    r.svc.owed();
    expect(JSON.stringify(dump())).toBe(before);
  });

  it("the notice goes out through the wake loop to the asking thread, once, and is done", async () => {
    const r = rig();
    const { g1 } = await opened(r, { key: "k-a" });
    corrupt(r, g1.id);
    await r.poll();
    const sdk = new FakeSdk();
    const loop = new WakeLoop(r.svc, sdk);
    await loop.nudge();
    expect(sdk.sent).toHaveLength(1);
    expect(sdk.sent[0]).toMatchObject({ threadId: "thr_a" });
    expect(sdk.sent[0]!.input).toContain("could not read the card");
    expect(sdk.sent[0]!.input).toContain("file it again");
    expect(r.svc.store.obligationsFor(g1.id).find((o) => o.kind === "notice")).toMatchObject({ state: "done" });
    await r.poll();
    await loop.nudge();
    expect(sdk.sent).toHaveLength(1);
  });
});
