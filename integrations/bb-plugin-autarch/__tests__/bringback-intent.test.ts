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
  r.env.clock.t = Date.parse(r.svc.store.pick(g1.id)!.picked_at) + 120_000;
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

it("bounds and falls back through card title, decision subject and Untitled card", async () => {
  const r = rig(); const { t } = await opened(r);
  r.db.prepare("UPDATE cards SET title = '' WHERE task_id = ?").run(t.id);
  expect(r.svc.cardNoticeLabel(t.id, "Decision subject")).toBe(`${t.key}: Decision subject`);
  expect(r.svc.cardNoticeLabel(t.id, "")).toBe(`${t.key}: Untitled card`);
  r.db.prepare("UPDATE cards SET title = ? WHERE task_id = ?").run("x".repeat(200), t.id);
  expect(r.svc.cardNoticeLabel(t.id, "subject")).toBe(`${t.key}: ${"x".repeat(120)}`);
});

it.each([{ confirmed: true }, { by: "waiting-sweep" }])("accepts a declared RPC caller: %j", caller => {
  expect(homeMethods.bringBack.input.safeParse({ task_id: "fixture", decision_id: "decision", ...caller }).success).toBe(true);
});
it("refuses a payload declaring both callers", () => {
  expect(homeMethods.bringBack.input.safeParse({ task_id: "fixture", decision_id: "decision", confirmed: true, by: "waiting-sweep" }).success).toBe(false);
});
