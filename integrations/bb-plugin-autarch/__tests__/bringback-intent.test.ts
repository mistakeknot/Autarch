import { afterEach, expect, it } from "vitest";
import { homeMethods } from "../contract.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

afterEach(cleanupEnvs);

it.each([undefined, false])("requires literal confirmation in the RPC contract: %s", confirmed => {
  expect(homeMethods.bringBack.input.safeParse({ task_id: "fixture", decision_id: "decision", ...(confirmed === undefined ? {} : { confirmed }) }).success).toBe(false);
});

it("refuses unconfirmed Archive reopening even more than a minute after the pick", async () => {
  const r = rig(); const { t, g1 } = await opened(r);
  r.mkPick(g1.id);
  r.env.clock.t = "2099-01-01T00:00:00.000Z";
  expect(r.dele.archiveBringBack(t.id, g1.id, "thr_a", false)).toMatchObject({ ok: false, status: 409 });
  expect(r.gens(t.id)).toHaveLength(1);
});

it.each(["unknown", "merge-recheck"] as const)("does not attribute %s to the Archive button", async origin => {
  const r = rig(); const { t, g1 } = await opened(r);
  r.mkPick(g1.id);
  const out = r.dele.bringBack(t.id, g1.id, "thr_a", {}, undefined, origin);
  expect(out.ok).toBe(true);
  const notice = r.svc.store.obligationsFor(g1.id).find(o => o.op === `bringback:${g1.id}`)!.payload;
  expect(notice).not.toContain("Archive button");
  const event = r.db.prepare("SELECT detail_json FROM events WHERE type='brought-back'").get() as { detail_json: string };
  expect(JSON.parse(event.detail_json).origin).toBe(origin);
});
