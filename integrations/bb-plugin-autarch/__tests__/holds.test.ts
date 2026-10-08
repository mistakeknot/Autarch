import { afterEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import { buildQueue } from "../queueview.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

const SHA = "a".repeat(64);

describe("holds (vizier greys a card out)", () => {
  afterEach(() => cleanupEnvs());

  it("hold needs a one-line reason and a real card; the queue row carries it; unhold clears it", async () => {
    const r = rig();
    const { t } = await opened(r);
    expect(r.svc.hold(t.id, "  ", "thr_viz")).toMatchObject({ ok: false });
    expect(r.svc.hold(t.id, "two\nlines", "thr_viz")).toMatchObject({ ok: false });
    expect(r.svc.hold("no-such", "why", "thr_viz")).toMatchObject({ ok: false, error: "no such card" });
    expect(r.svc.hold(t.id, "script superseded", "thr_viz")).toEqual({ ok: true, task_id: t.id });
    const key = r.svc.cardKey(t.id)!;
    expect(r.svc.cardTaskId(key)).toBe(t.id);
    const row = buildQueue(r.svc, new Asks(r.svc)).rows.find((x) => x.task_id === t.id)!;
    expect(row.held).toMatchObject({ reason: "script superseded", by: "thr_viz" });
    expect(r.svc.unhold(key)).toEqual({ ok: true, task_id: t.id, was: true });
    expect(buildQueue(r.svc, new Asks(r.svc)).rows.find((x) => x.task_id === t.id)!.held).toBeNull();
    expect(r.svc.unhold(t.id)).toMatchObject({ ok: true, was: false });
  });

  it("a held card cannot be picked, even from a page opened before the hold", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.svc.hold(t.id, "superseded", "thr_viz");
    expect(r.mkPick(g1.id)).toMatchObject({ ok: false, status: 409 });
    expect(r.gens(t.id)).toHaveLength(1);
    r.svc.unhold(t.id);
    expect(r.mkPick(g1.id, "p-after")).toMatchObject({ ok: true });
  });

  it("a hold replaced while its comments were being read is not cleared by the old reading", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.env.clock.t = Date.parse("2026-10-02T00:00:00.000Z");
    r.svc.hold(t.id, "first", "thr_viz");
    const later = new Date(Date.parse(r.svc.holdOf(t.id)!.at) + 60_000).toISOString();
    const released = await r.svc.releaseHoldOnSha(t.id, async () => {
      r.env.clock.t += 120_000;
      r.svc.hold(t.id, "second", "thr_viz");
      return [{ id: "c1", taskId: t.id, kind: "agent" as const, authorName: "a", threadId: "thr_a", body: `bash /x ${SHA}`, createdAt: later }];
    });
    expect(released).toBe(false);
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "second" });
  });

  it("a held setting that is not JSON means no holds", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.svc.store.setSetting("holds", "not json");
    expect(r.svc.holdOf(t.id)).toBeNull();
    r.svc.store.setSetting("holds", "[1]");
    expect(r.svc.holdOf(t.id)).toBeNull();
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: "bad" }));
    expect(r.svc.holdOf(t.id)).toBeNull();
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "", by: "v", at: "2026-10-07T00:00:00.000Z" } }));
    expect(r.svc.holdOf(t.id)).toBeNull();
    r.svc.store.setSetting("holds", JSON.stringify({ [t.id]: { reason: "r", by: "v", at: "never" } }));
    expect(r.svc.holdOf(t.id)).toBeNull();
  });

  it("a later non-system comment with a sha256 releases the hold; an earlier one, a system one or no sha does not", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.fake.addComment(t.id, { kind: "agent", threadId: "thr_a", body: `old script ${SHA}`, createdAt: "2026-10-01T00:00:02.000Z" });
    r.env.clock.t = Date.parse("2026-10-02T00:00:00.000Z");
    r.svc.hold(t.id, "script superseded", "thr_viz");
    const at = r.svc.holdOf(t.id)!.at;
    const later = new Date(Date.parse(at) + 60_000).toISOString();
    r.fake.addComment(t.id, { kind: "system", threadId: null, body: `sys ${SHA}`, createdAt: later });
    r.fake.addComment(t.id, { kind: "agent", threadId: "thr_a", body: "no digest here", createdAt: later });
    r.edit(t);
    r.advance(61_000);
    await r.poll();
    expect(r.svc.holdOf(t.id)).not.toBeNull();
    r.fake.addComment(t.id, { kind: "agent", threadId: "thr_a", body: `new script bash /x.sh sha256 ${SHA}`, createdAt: later });
    // no edit(t): the task's updatedAt is unchanged, so only the held-card sweep can see the comment
    r.advance(61_000);
    await r.poll();
    expect(r.svc.holdOf(t.id)).toBeNull();
  });

  it("bb home hold / unhold are vizier only and `list` marks the held ask", async () => {
    const r = rig();
    const { t } = await opened(r);
    r.enableDelegation();
    const cli = homeCli({ svc: r.svc, asks: new Asks(r.svc), catchup: new Catchup(r.svc, r.dele), rule: (id, o, why, ctx) => r.dele.rule(id, o, why, ctx), isVizier: (th) => th === "thr_viz" });
    const go = (argv: string[], threadId?: string) => Promise.resolve(cli.run(argv, threadId ? { threadId } : {}));
    const denied = await go(["hold", t.id, "--reason", "x"], "thr_a");
    expect(denied.exitCode).not.toBe(0);
    expect(denied.stderr).toMatch(/only the vizier/);
    expect(r.svc.holdOf(t.id)).toBeNull();
    expect((await go(["hold", t.id], "thr_viz")).exitCode).not.toBe(0);
    const ok = await go(["hold", r.svc.cardKey(t.id)!, "--reason", "superseded"], "thr_viz");
    expect(ok.exitCode).toBe(0);
    expect(r.svc.holdOf(t.id)).toMatchObject({ reason: "superseded", by: "thr_viz" });
    const list = JSON.parse((await go(["list", "--json"])).stdout!) as { task_id: string; held: { reason: string } | null }[];
    expect(list.find((x) => x.task_id === t.id)!.held).toMatchObject({ reason: "superseded" });
    expect((await go(["unhold", t.id], "thr_a")).exitCode).not.toBe(0);
    const un = await go(["unhold", t.id], "thr_viz");
    expect(JSON.parse(un.stdout!)).toMatchObject({ ok: true, was_held: true });
    expect(r.svc.holdOf(t.id)).toBeNull();
  });
});
