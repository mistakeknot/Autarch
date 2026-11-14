// mk-cqnab.1.1: producer serialization and terminal evidence regressions.
// All cards, targets and owner words below are synthetic fixtures.
import { afterEach, expect, it, vi } from "vitest";
import { cleanupEnvs, rig } from "./card-rig.js";
import { TasksClient } from "../tasks.js";
import { pickFenceToken, recordPickEvaluation } from "../pickcapture.js";
import type { PickActionRow } from "../store.js";

afterEach(cleanupEnvs);

async function shadowPick(reason?: string) {
  const r = rig({ svc: {
    pickHandlerMode: "shadow",
    readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Synthetic target",
      project_id: "synthetic-project", project_name: "Synthetic project", archived: false,
      deleted: false, observed_at: "2026-01-01T00:00:00.000Z" }),
  } });
  r.enableDelegation();
  const task = r.card({ blocks: "thread:thr_synthetic_target", ask: {
    recommendation: "a", options: [
      { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: true,
        on_pick: { schema: 1, target_thread: "thr_synthetic_target", reversible: true,
          subject: { kind: "none" }, ruling: { bead: null, under: [] } } },
      { id: "b", label: "Wait", kind: "ruling-only" },
    ],
  } });
  await r.poll();
  expect(r.gens(task.id)).toHaveLength(1);
  const decision = r.gens(task.id)[0];
  const pickId = "synthetic-shadow-pick";
  expect((await r.validatedMkPick(decision.id, pickId, "a", reason)).ok).toBe(true);
  const action = () => r.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ?").get(pickId) as PickActionRow;
  // pickWorkIdle tracks live preparation only. Wait for the actual shadow write,
  // irrespective of whether its stored JSON is currently the defective wrapper.
  await vi.waitFor(() => {
    expect(r.db.prepare("SELECT * FROM events WHERE decision_id = ? AND type = 'pick-collection-failure'").all(decision.id)).toEqual([]);
    expect(action()).toBeDefined();
    expect(action().evaluated_at).not.toBeNull();
    expect(action().evaluation_json).not.toBeNull();
  });
  return { r, decision, pickId, action };
}

it.each([
  { name: "automatic proposal", reason: undefined, outcome: "automatic", words: "pass" },
  { name: "owner-words fallback", reason: "Wait for independent review. 完整条件", outcome: "fallback", words: "fail" },
] as const)("production shadow $name stores a bare evaluation and reads the full original action", async ({ reason, outcome, words }) => {
  const { r, decision, pickId, action } = await shadowPick(reason);
  const saved = action();
  const stored = JSON.parse(saved.evaluation_json!);
  const view = r.svc.readPickAction({ pick_id: pickId });
  expect(view).toMatchObject({ ok: true, kind: "action", observation_only: true,
    ordinary_continuation: true, handler_status: "manual handling required", live_delivery_available: false });
  if (view.kind !== "action") throw new Error("full original action unavailable");
  // readPickAction invokes the private production evaluationSchema.parse. Full
  // action + parsed JSON equivalence exercises that schema without exporting or copying it.
  expect(stored).not.toHaveProperty("input");
  expect(stored).not.toHaveProperty("evaluation");
  expect(stored).toMatchObject({ pick_id: pickId, decision_id: decision.id, generation: decision.generation,
    outcome, activation: { mode: "shadow", permitted: false, reason: "shadow-observation-only" } });
  expect(view.snapshot.evaluation_json).toBe(saved.evaluation_json);
  expect(view.facts.evaluation).toEqual(stored);
  expect(view.facts.initial_evaluation).toBeNull();
  expect(view.facts.checks).toEqual(stored.checks);
  expect(view.facts.checks).toMatchObject({ recommendation: { result: "pass" },
    "no-extra-words": { result: words }, reversible: { result: "pass" }, "subject-head": { result: "not-applicable" } });
  expect(view.facts.deadline.outcome).toBe("met");
  expect(view).toEqual(r.svc.readPickAction({ decision_id: decision.id, generation: decision.generation }));
  if (reason) expect(view.facts.owner_words).toContainEqual({ kind: "ordinary-reason", text: reason });
  expect(r.svc.store.obligationsFor(decision.id).some(o => o.kind === "pick-relay" || o.kind === "pick-receipt")).toBe(false);
});

it.each(["fallback", "cancelled"] as const)("recordPickEvaluation refuses %s without bound terminal evidence and preserves the action", async to => {
  const { r, decision, pickId, action } = await shadowPick();
  // Isolate terminal acceptance from the shadow wrapper defect using the real
  // evaluator and the supported nonterminal write, never a fabricated result.
  const result = await r.svc.pickEvaluation(decision.id, () => new TasksClient(r.fake), {
    deadlineAt: action().evaluation_deadline_at, captureEpoch: action().capture_epoch,
  });
  expect(result).not.toBeNull();
  const evaluation = result!.evaluation;
  expect(recordPickEvaluation(r.svc.store, action(), evaluation, r.env.now(), pickFenceToken(r.svc.store, action()))).toBe(true);
  const before = action();
  const evidence = () => ({
    events: r.db.prepare("SELECT * FROM events ORDER BY seq").all(),
    obligations: r.db.prepare("SELECT * FROM obligations ORDER BY id").all(),
    attempts: r.db.prepare("SELECT * FROM attempts ORDER BY obligation_id, n").all(),
    writes: r.db.prepare("SELECT * FROM card_writes ORDER BY id").all(),
  });
  const originalEvidence = evidence();
  const originalView = r.svc.readPickAction({ pick_id: pickId });
  expect(originalView).toMatchObject({ kind: "action", handler_status: "manual handling required", ordinary_continuation: true });
  expect(r.db.prepare("SELECT * FROM events WHERE type = 'pick-fallback'").all()).toEqual([]);
  const changes = r.db.prepare("SELECT total_changes() AS n").get();

  expect(recordPickEvaluation(r.svc.store, before, evaluation, r.env.now(), pickFenceToken(r.svc.store, before),
    { from: "pending", to })).toBe(false);

  expect(action()).toEqual(before);
  expect(evidence()).toEqual(originalEvidence);
  expect(r.db.prepare("SELECT total_changes() AS n").get()).toEqual(changes);
  expect(r.svc.readPickAction({ pick_id: pickId })).toEqual(originalView);
  expect(r.svc.readPickAction({ decision_id: decision.id, generation: decision.generation })).toEqual(originalView);
});
