// Task 2.5: the card write-back worker (plan 1.3.8): mirror comment, idempotent unlabel, backoff.
import { afterEach, describe, expect, it } from "vitest";
import { commentBody } from "../cardwrites.js";
import { cleanupEnvs, opened, pollN, rig } from "./card-rig.js";

afterEach(cleanupEnvs);
const writes = (r: any) => r.db.prepare("SELECT kind, state, attempt, last_error FROM card_writes ORDER BY kind").all();
const labels = (r: any, t: any) => r.fake.tasks.find((x: any) => x.id === t.id).labelIds;

describe("card write-back", () => {
  it("a pick inserts the comment and unlabel rows with the pick; a poll runs them", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.mkPick(g1.id)).toMatchObject({ ok: true });
    expect(writes(r).map((w: any) => [w.kind, w.state])).toEqual([["comment", "pending"], ["unlabel", "pending"]]);
    await r.poll();
    expect(writes(r).map((w: any) => w.state)).toEqual(["done", "done"]);
    expect(labels(r, t)).toEqual([]);
    expect(r.fake.callsOf("createComment")[0]!.input).toMatchObject({ taskId: t.id, notify: false });
    expect(r.fake.comments.at(-1)!.body).toContain("Ruled: Per project (Home, advisory; ruled by: mk)");
  });

  it("the comment renders pick.by: a delegated ruling says ruled by: vizier", async () => {
    const r = rig();
    r.enableDelegation();
    const { g1 } = await opened(r);
    r.vizierPick(g1.id);
    await r.poll();
    expect(r.fake.comments.at(-1)!.body).toContain("ruled by: vizier");
    expect(r.fake.comments.at(-1)!.body).toContain("Reason: reversible and routine");
  });

  it("the unlabel keeps other labels and is idempotent; a lost comment response is not posted twice", async () => {
    const r = rig();
    const other = r.fake.addLabel(r.tp.id, "bug");
    const { t, g1 } = await opened(r);
    r.fake.tasks.find((x) => x.id === t.id)!.labelIds.push(other.id);
    r.mkPick(g1.id);
    r.fake.lostResponses.push({ method: "createComment", nth: 1 });
    await r.poll();
    expect(writes(r).find((w: any) => w.kind === "comment")).toMatchObject({ state: "pending", attempt: 1 });
    expect(labels(r, t)).toEqual([other.id]);
    await pollN(r, 2);
    expect(r.fake.comments.filter((c) => c.body.includes("Ruled:"))).toHaveLength(1);
    // run the unlabel again by hand: nothing to remove, no second updateTask
    const before = r.fake.callsOf("updateTask").length;
    r.db.prepare("UPDATE card_writes SET state = 'pending' WHERE kind = 'unlabel'").run();
    await r.writer.drain();
    expect(r.fake.callsOf("updateTask").length).toBe(before);
    expect(labels(r, t)).toEqual([other.id]);
  });

  it("a failing unlabel backs off and blocks neither the wake nor the ruling file", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.fake.failures.push({ method: "updateTask", nth: 1, error: new Error("tasks is down") });
    r.mkPick(g1.id);
    await r.poll();
    expect(writes(r).find((w: any) => w.kind === "unlabel")).toMatchObject({ state: "pending", attempt: 1, last_error: expect.stringContaining("tasks is down") });
    const obs = r.svc.store.obligationsFor(g1.id);
    expect(obs.find((o) => o.kind === "ruling-file")).toMatchObject({ state: "done" });
    expect(obs.find((o) => o.kind === "wake")).toMatchObject({ state: "pending" });
    await r.poll(); // not yet due
    expect(labels(r, t)).not.toEqual([]);
    await pollN(r, 2);
    expect(labels(r, t)).toEqual([]);
  });

  it("a re-added label never re-queues a ruled card", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.mkPick(g1.id);
    await pollN(r, 2);
    r.edit(t, { labelIds: [r.label.id] });
    await pollN(r, 3);
    expect(r.gens(t.id)).toHaveLength(1);
    expect(r.cardRow(t.id).state).toBe("ruled");
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE kind = 'unlabel'").get()).toEqual({ n: 1 });
  });

  it("the comment body names the write for idempotency", () => {
    expect(commentBody("cw-x", { option_label: "A", by: "mk", generation: 2 })).toBe("Ruled: A (Home, advisory; ruled by: mk) generation 2\n\nhome-write: cw-x");
  });
});
