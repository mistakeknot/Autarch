import { afterEach, expect, it, vi } from "vitest";
import { cleanupEnvs, rig } from "./card-rig.js";
import { TasksClient } from "../tasks.js";
import type { ServiceDeps } from "../service.js";
import { capturePick, recordPickEvaluation } from "../pickcapture.js";
import { Store, type PickActionRow } from "../store.js";
import { Delegation } from "../delegation.js";
import Database from "better-sqlite3";

afterEach(cleanupEnvs);

it.each([false, true])("note nudge observes committed obligation and fence with no open transaction (deferred=%s)", async deferred => {
  const nudge = vi.fn();
  const { r, g } = await participating({ nudge });
  const reader = new Database(r.env.file, { readonly: true });
  const observations: unknown[] = [];
  nudge.mockClear();
  nudge.mockImplementation(() => observations.push({
    inTransaction: r.db.inTransaction,
    obligations: reader.prepare("SELECT kind FROM obligations WHERE decision_id = ? AND kind = 'note'").all(g.id),
    fences: reader.prepare("SELECT note_id, posting_state FROM pick_note_fences WHERE decision_id = ?").all(g.id),
  }));
  try {
    if (deferred) {
      const result = r.svc.note({ decision_id: g.id }, "Wait for review", "notify-note", { deferWake: true });
      if (!result.ok) throw new Error(result.error);
      expect(nudge).not.toHaveBeenCalled();
      expect(reader.prepare("SELECT posting_state FROM pick_note_fences WHERE decision_id = ?").get(g.id)).toEqual({ posting_state: "pending" });
      expect(result.commit()).toEqual({ replay: false, woke: true });
    } else {
      expect(r.svc.note({ decision_id: g.id }, "Wait for review", "notify-note")).toMatchObject({ ok: true, replay: false, woke: true });
    }
    expect(nudge).toHaveBeenCalledTimes(1);
    expect(observations).toEqual([{
      inTransaction: false,
      obligations: [{ kind: "note" }],
      fences: [{ note_id: "notify-note", posting_state: "recorded" }],
    }]);
  } finally {
    reader.close();
  }
});

it("throwing direct note nudge propagates without rolling back the obligation or fence", async () => {
  const nudge = vi.fn();
  const { r, g } = await participating({ nudge });
  const reader = new Database(r.env.file, { readonly: true });
  const failure = new Error("notification unavailable");
  nudge.mockClear();
  nudge.mockImplementation(() => { throw failure; });
  try {
    expect(() => r.svc.note({ decision_id: g.id }, "Wait for review", "throw-note")).toThrow(failure);
    expect(r.db.inTransaction).toBe(false);
    expect(reader.prepare("SELECT kind FROM obligations WHERE decision_id = ? AND kind = 'note'").all(g.id)).toEqual([{ kind: "note" }]);
    expect(reader.prepare("SELECT note_id, posting_state FROM pick_note_fences WHERE decision_id = ?").all(g.id)).toEqual([{ note_id: "throw-note", posting_state: "recorded" }]);
    expect(r.svc.note({ decision_id: g.id }, "Wait for review", "throw-note")).toMatchObject({ ok: true, replay: true });
    expect(nudge).toHaveBeenCalledTimes(1);
  } finally {
    reader.close();
  }
});

async function participating(deps: Partial<ServiceDeps> = {}, target = "thr_target", blocks = "thread:thr_target") {
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }), ...deps } });
  r.enableDelegation();
  const t = r.card({ blocks, ask: { recommendation: "a", options: [
    { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: true,
      on_pick: { schema: 1, target_thread: target, reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } } },
    { id: "b", label: "Wait", kind: "ruling-only" },
  ] } });
  await r.poll();
  expect(r.gens(t.id), JSON.stringify(r.cardRow(t.id))).toHaveLength(1);
  return { r, t, g: r.gens(t.id)[0] };
}

it("mk-j8u9k: raw synchronous participating pick cannot omit the validated entry guard", async () => {
  const { r, g } = await participating();
  expect(r.mkPick(g.id).ok).toBe(false);
  expect(r.svc.store.pick(g.id)).toBeUndefined();
});

async function ownerPick(r: Awaited<ReturnType<typeof participating>>["r"], g: { id: string; revision: string }, id = "owner-pick") {
  return r.svc.withValidatedPick(g.id, guard => r.svc.pick(g.id, "a", g.revision, id, "mk", "home", undefined, undefined, undefined, undefined, guard), () => new TasksClient(r.fake));
}

it("shadow capture follows every legacy pick effect", async () => {
  const { r, g } = await participating({ ...{ pickHandlerMode: "shadow" } });
  expect((await ownerPick(r, g)).ok).toBe(true);
  expect(r.db.prepare("SELECT capture_mode, ruling_op FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual({ capture_mode: "shadow", ruling_op: `ruling-file:${g.id}` });
  expect(r.svc.store.obligationsFor(g.id).map(o => o.kind).sort()).toEqual(["notice", "ruling-file", "wake"]);
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE decision_id = ?").get(g.id)).toEqual({ n: 2 });
});

it("production configuration cannot enable live in PR3", async () => {
  expect(() => rig({ svc: { ...{ pickHandlerMode: "live" } } })).toThrow("off or shadow");
});

it.each(["fallback", "cancelled", "deadline", "epoch", "note", "generation"])("carried1 no-revive-for-automatic: late evaluation after %s cannot write", async change => {
  const { r, g } = await participating({ pickHandlerMode: "shadow" });
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const original = r.svc.pickEvaluation.bind(r.svc);
  let finished!: () => void;
  const done = new Promise<void>(resolve => { finished = resolve; });
  vi.spyOn(r.svc, "pickEvaluation").mockImplementation(async (...args) => {
    const result = await original(...args);
    await gate;
    finished();
    return result;
  });
  expect((await ownerPick(r, g)).ok).toBe(true);
  if (change === "fallback" || change === "cancelled") r.db.prepare("UPDATE pick_actions SET disposition = ? WHERE decision_id = ?").run(change, g.id);
  if (change === "deadline") r.advance(120_001);
  if (change === "epoch") r.db.prepare("UPDATE pick_handler_control SET epoch = epoch + 1 WHERE id = 1").run();
  if (change === "note") r.svc.note({ decision_id: g.id }, "Wait", "new-fence", { deferWake: true });
  if (change === "generation") expect(r.dele.bringBack(String(g.task_id), g.id, null, {}).ok).toBe(true);
  release();
  await done;
  await Promise.resolve();
  expect(r.db.prepare("SELECT evaluation_json FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual({ evaluation_json: null });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "pick-relay")).toHaveLength(0);
});

it("isolated dormant capture owns routing atomically and reserves the target Blocks path", async () => {
  const { r, g } = await participating({ ...{ pickCaptureRuntime: { mode: () => "live" } } });
  expect((await ownerPick(r, g)).ok).toBe(true);
  expect(r.db.prepare("SELECT capture_mode FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual({ capture_mode: "live" });
  expect(r.svc.store.obligationsFor(g.id).map(o => o.kind)).toEqual(["ruling-file"]);
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE decision_id = ?").get(g.id)).toEqual({ n: 2 });
});

it("isolated dormant capture owns a selected option without a declaration", async () => {
  const { r, g } = await participating({ ...{ pickCaptureRuntime: { mode: () => "live" } } });
  const result = await r.svc.withValidatedPick(g.id, guard => r.svc.pick(g.id, "b", g.revision, "no-declaration", "mk", "home", undefined, undefined, undefined, undefined, guard));
  expect(result.ok).toBe(true);
  expect(r.db.prepare("SELECT declaration_json, target_thread, capture_mode FROM pick_actions WHERE decision_id = ?").get(g.id)).toEqual({ declaration_json: null, target_thread: null, capture_mode: "live" });
  expect(r.svc.store.obligationsFor(g.id).map(o => o.kind)).toEqual(["ruling-file"]);
});

it.each(["pick-relay", "pick-receipt"])("carried2 strict protected dedup and carried3 permanent declared identities share the transition transaction: %s", async kind => {
  const { r, g } = await participating({ pickCaptureRuntime: { mode: () => "live" } });
  r.db.prepare(`UPDATE pick_handler_control SET mode = 'live', epoch = 1, owner_enablement_json = '{}', reviewed_build = 'test-build', reviewed_scope_json = '{}', enabled_cells_json = '["instruction:none"]' WHERE id = 1`).run();
  expect((await ownerPick(r, g)).ok).toBe(true);
  const action = r.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(g.id) as PickActionRow;
  const relay = { id: "protected-message", kind, op: (kind === "pick-relay" ? action.relay_op : action.receipt_op)!, recipient: kind === "pick-relay" ? "thr_target" : "thr_a", payload: "immutable filed payload" };
  const forged = { ...action, ...(kind === "pick-relay" ? { relay_op: "forged" } : { receipt_op: "forged" }) };
  expect(recordPickEvaluation(r.svc.store, forged, {}, r.env.now(), 0, { from: "pending", to: "automatic", obligations: [{ ...relay, op: "forged" }] })).toBe(false);
  expect(recordPickEvaluation(r.svc.store, action, {}, r.env.now(), 0, { from: "pending", to: "automatic", obligations: [relay] })).toBe(true);
  expect(r.svc.store.obligation(relay.id)).toMatchObject({ payload: relay.payload, next_try_at: "9999-12-31T23:59:59.999Z", allow_reroute: kind === "pick-relay" ? 0 : 1, allow_batch: 0, require_message_id: 1 });
  const saved = r.svc.store.obligation(relay.id);
  expect(recordPickEvaluation(r.svc.store, action, {}, r.env.now(), 0, { from: "automatic", to: "automatic", obligations: [relay] })).toBe(true);
  expect(r.svc.store.obligation(relay.id)).toEqual(saved);
  expect(capturePick(r.svc.store, g.id, "live", r.env.now()).pick_id).toBe(action.pick_id);
  expect(() => recordPickEvaluation(r.svc.store, action, { changed: true }, r.env.now(), 0, { from: "automatic", to: "automatic", obligations: [{ ...relay, payload: "changed" }] })).toThrow("protected obligation conflict");
  expect(r.db.prepare("SELECT evaluation_json FROM pick_actions WHERE pick_id = ?").get(action.pick_id)).toEqual({ evaluation_json: "{}" });
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM obligations WHERE id = ? OR op = ?").get(relay.id, relay.op)).toEqual({ n: 1 });
  expect(r.svc.store.obligation(relay.id)).toEqual(saved);
  r.db.prepare("UPDATE pick_actions SET disposition = 'fallback' WHERE pick_id = ?").run(action.pick_id);
  expect(recordPickEvaluation(r.svc.store, action, {}, r.env.now(), 0, { from: "automatic", to: "automatic", obligations: [relay] })).toBe(false);
  expect(r.svc.store.obligation(relay.id)).toEqual(saved);
});

it("mk-j8u9k: validated entry rejects owner changes between evidence and transaction", async () => {
  const { r, g } = await participating();
  const result = await r.svc.withValidatedPick(g.id, guard => {
    r.db.prepare("UPDATE decisions SET owner_thread = ? WHERE id = ?").run("thr_changed", g.id);
    return r.svc.pick(g.id, "a", g.revision, "race", "mk", "home", undefined, undefined, undefined, undefined, guard);
  }, () => new TasksClient(r.fake));
  expect(result.ok).toBe(false);
  expect(r.svc.store.pick(g.id)).toBeUndefined();
});

it("carried note fence: pending note is permanent before deferred Tasks posting", async () => {
  const { r, g } = await participating();
  const result = r.svc.note({ decision_id: g.id }, "Wait for review", "note-one", { deferWake: true });
  expect(result.ok).toBe(true);
  expect(r.db.prepare("SELECT note_id, posting_state FROM pick_note_fences WHERE decision_id = ?").all(g.id))
    .toEqual([{ note_id: "note-one", posting_state: "pending" }]);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "note")).toHaveLength(0);
});

it("carried2 strict dedup: changed note words under the same declared note key conflict before posting", async () => {
  const { r, g } = await participating();
  expect(r.svc.note({ decision_id: g.id }, "Wait for review", "note-one", { deferWake: true }).ok).toBe(true);
  const conflict = r.svc.note({ decision_id: g.id }, "Proceed now", "note-one", { deferWake: true });
  expect(conflict).toMatchObject({ ok: false, status: 409 });
});

it("mk-j8u9k: an arbitrary synchronous guard is not target evidence", async () => {
  const { r, g } = await participating();
  expect(r.svc.pick(g.id, "a", g.revision, "forged", "mk", "home", undefined, () => null).ok).toBe(false);
  expect(r.svc.store.pick(g.id)).toBeUndefined();
});

it("carried3: exact deferred note retries preserve the declared fence and recorded posting evidence", async () => {
  const { r, g } = await participating();
  const note = () => r.svc.note({ decision_id: g.id }, "Wait for review", "stable-note", { deferWake: true });
  const first = note();
  expect(first.ok).toBe(true);
  if (!first.ok) throw new Error(first.error);
  expect(first.commit().woke).toBe(true);
  const rows = () => r.db.prepare("SELECT * FROM pick_note_fences WHERE decision_id = ? AND note_id = ?").all(g.id, "stable-note");
  const saved = rows();
  expect(saved).toMatchObject([{ posting_state: "recorded" }]);
  const retry = note();
  if (!retry.ok) throw new Error(retry.error);
  expect(retry.commit().replay).toBe(true);
  expect(rows()).toEqual(saved);
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "note")).toHaveLength(1);
});

it("carried note fence: failed Tasks posting retains the fence without waking", async () => {
  const { r, g } = await participating();
  const note = r.svc.note({ decision_id: g.id }, "Wait", "failed-note", { deferWake: true });
  if (!note.ok) throw new Error(note.error);
  expect(note.commit("Tasks unavailable").woke).toBe(false);
  expect(r.db.prepare("SELECT posting_state FROM pick_note_fences WHERE decision_id = ? AND note_id = ?").get(g.id, "failed-note")).toEqual({ posting_state: "failed" });
  expect(r.svc.store.obligationsFor(g.id).filter(o => o.kind === "note")).toHaveLength(0);
  const retry = r.svc.note({ decision_id: g.id }, "Wait", "failed-note", { deferWake: true });
  if (!retry.ok) throw new Error(retry.error);
  expect(retry.commit().woke).toBe(true);
  expect(r.db.prepare("SELECT posting_state FROM pick_note_fences WHERE decision_id = ? AND note_id = ?").get(g.id, "failed-note")).toEqual({ posting_state: "recorded" });
});

it("mk-j8u9k: fresh external target evidence must be bound to the exact Home owner at commit", async () => {
  const { r, g } = await participating();
  const tasks = new TasksClient(r.fake);
  expect(await r.svc.refreshOwnerPick(g.id, () => tasks)).toBeNull();
  r.db.prepare("UPDATE decisions SET owner_thread = ? WHERE id = ?").run("thr_changed", g.id);
  expect(r.svc.pick(g.id, "a", g.revision, "stale-owner", "mk", "home", undefined, () => null).ok).toBe(false);
  expect(r.svc.store.pick(g.id)).toBeUndefined();
});

it.each(["off", "shadow"].flatMap(mode => ["title", "project", "name", "archived", "deleted", "unavailable"].map(field => ({ mode, field }))))("mk-j8u9k: actual vizier delegation validates $field in $mode without consuming cap", async ({ mode, field }) => {
  let stale = false;
  const { r, g } = await participating({ pickHandlerMode: mode, readTargetEvidence: async id => { if (stale && field === "unavailable") throw new Error("unavailable"); return { schema: 1, thread_id: id, title: stale && field === "title" ? "Changed" : "Work", project_id: stale && field === "project" ? "changed" : "project", project_name: stale && field === "name" ? "Changed" : "Project", archived: stale && field === "archived", deleted: stale && field === "deleted", observed_at: "2026-01-01T00:00:00Z" }; } });
  const dele = new Delegation(r.svc, async () => [{ id: "thr_viz", title: "Vizier", archivedAt: null, pinnedAt: 1 }]);
  dele.applyProjectList(["Autarch"]);
  stale = true;
  expect((await dele.ruleResolved(g.id, "a", "routine", { threadId: "thr_viz" })).ok).toBe(false);
  expect(r.svc.store.pick(g.id)).toBeUndefined();
  expect(r.svc.store.obligationsFor(g.id)).toHaveLength(0);
  stale = false;
  expect((await dele.ruleResolved(g.id, "a", "routine", { threadId: "thr_viz" })).ok).toBe(true);
  expect(r.svc.store.pick(g.id)?.by).toBe("vizier");
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM pick_actions WHERE capture_mode = 'live'").get()).toEqual({ n: 0 });
});

it.each(["insert", "evaluation"])("shadow %s failure preserves API success and every legacy effect", async failure => {
  const { r, g } = await participating({ pickHandlerMode: "shadow" });
  if (failure === "insert") vi.spyOn(r.svc.store, "pickEvidenceSnapshot").mockImplementation(() => { throw new Error("collection unavailable"); });
  else vi.spyOn(r.svc, "pickEvaluation").mockRejectedValue(new Error("evaluation unavailable"));
  expect((await ownerPick(r, g)).ok).toBe(true);
  await Promise.resolve();
  await Promise.resolve();
  expect(r.svc.store.obligationsFor(g.id).map(o => o.kind).sort()).toEqual(["notice", "ruling-file", "wake"]);
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE decision_id = ?").get(g.id)).toEqual({ n: 2 });
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM events WHERE decision_id = ? AND type = 'pick-collection-failure'").get(g.id)).toEqual({ n: 1 });
});

it("off never captures or evaluates, including exact retry", async () => {
  const { r, g } = await participating();
  const evaluate = vi.spyOn(r.svc, "pickEvaluation");
  expect((await ownerPick(r, g)).ok).toBe(true);
  expect(await ownerPick(r, g)).toMatchObject({ ok: true, status: 200 });
  expect(evaluate).not.toHaveBeenCalled();
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM pick_actions").get()).toEqual({ n: 0 });
});

it("dormant capture cannot reconstruct an historical off pick as live", async () => {
  const { r, g } = await participating();
  expect((await ownerPick(r, g)).ok).toBe(true);
  expect(() => capturePick(r.svc.store, g.id, "live", r.env.now())).toThrow("new owner pick transaction");
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM pick_actions").get()).toEqual({ n: 0 });
});

it("note task generation selection and fence reservation share the write transaction", async () => {
  const { r, t, g } = await participating();
  expect((await ownerPick(r, g)).ok).toBe(true);
  const atomic = r.svc.store.atomically.bind(r.svc.store);
  let latest = "";
  vi.spyOn(r.svc.store, "atomically").mockImplementationOnce(fn => {
    const back = r.dele.bringBack(t.id, g.id, null, {});
    if (!back.ok) throw new Error(back.error);
    latest = back.decision_id;
    return atomic(fn);
  });
  expect(r.svc.note({ task_id: t.id }, "Wait", "generation-note", { deferWake: true }).ok).toBe(true);
  expect(r.db.prepare("SELECT decision_id FROM pick_note_fences WHERE note_id = ?").all("generation-note")).toEqual([{ decision_id: latest }]);
});

it("carried2: note operation identity survives a new generation without accepting changed words", async () => {
  const { r, t, g } = await participating();
  expect(r.svc.note({ task_id: t.id }, "Wait", "task-note").ok).toBe(true);
  expect((await ownerPick(r, g)).ok).toBe(true);
  expect(r.dele.bringBack(t.id, g.id, null, {}).ok).toBe(true);
  expect(r.svc.note({ task_id: t.id }, "Changed", "task-note")).toMatchObject({ ok: false, status: 409 });
  expect(r.svc.note({ task_id: t.id }, "Wait", "task-note")).toMatchObject({ ok: true, replay: true });
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM pick_note_fences WHERE note_id = ?").get("task-note")).toEqual({ n: 1 });
});

it("dormant target equals asker has no duplicate wake; other Blocks receive only readback information", async () => {
  const { r, g } = await participating({ pickCaptureRuntime: { mode: () => "live" } }, "thr_a", "thread:thr_a thread:thr_other");
  expect((await ownerPick(r, g)).ok).toBe(true);
  const messages = r.svc.store.obligationsFor(g.id).filter(o => o.kind !== "ruling-file");
  expect(messages).toHaveLength(1);
  expect(messages[0]).toMatchObject({ recipient: "thr_other", kind: "notice" });
  expect(messages[0].payload).toContain("Information only; do not relay or act");
  expect(messages[0].payload).not.toContain("Use blue.");
});

it("dormant capture Store hook failure rolls back pick, action, writes and obligations together", async () => {
  const { r, g } = await participating();
  const store = new Store(r.db, { now: r.env.now, hook: step => { if (step === "pick-captured") throw new Error("capture crash"); } });
  const svc = r.env.open({ store, tasks: () => new TasksClient(r.fake), pickCaptureRuntime: { mode: () => "live" }, readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) });
  await expect(svc.withValidatedPick(g.id, guard => svc.pick(g.id, "a", g.revision, "rollback", "mk", "home", undefined, undefined, undefined, undefined, guard))).rejects.toThrow("capture crash");
  expect(store.pick(g.id)).toBeUndefined();
  expect(store.obligationsFor(g.id)).toHaveLength(0);
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM pick_actions").get()).toEqual({ n: 0 });
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_writes WHERE decision_id = ?").get(g.id)).toEqual({ n: 0 });
});

it.each(["owner", "binding", "target-snapshot", "withdrawn"])("mk-j8u9k: actual vizier path rejects Home %s change after awaited evidence with no cap consumption", async field => {
  const { r, g } = await participating();
  const dele = new Delegation(r.svc, async () => [{ id: "thr_viz", title: "Vizier", archivedAt: null, pinnedAt: 1 }]);
  dele.applyProjectList(["Autarch"]);
  const refresh = r.svc.refreshOwnerPick.bind(r.svc);
  vi.spyOn(r.svc, "refreshOwnerPick").mockImplementation(async (...args) => {
    const result = await refresh(...args);
    if (field === "owner") r.db.prepare("UPDATE decisions SET owner_thread = 'thr_changed' WHERE id = ?").run(g.id);
    if (field === "binding") r.db.prepare("UPDATE project_bindings SET state = 'suggested'").run();
    if (field === "target-snapshot") r.svc.store.recordEvent("on-pick-targets", g.id, { changed: true });
    if (field === "withdrawn") r.svc.store.withdrawDecision(g.id, "changed");
    return result;
  });
  expect((await dele.ruleResolved(g.id, "a", "routine", { threadId: "thr_viz" })).ok).toBe(false);
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM picks WHERE \"by\" = 'vizier'").get()).toEqual({ n: 0 });
  expect(r.svc.store.obligationsFor(g.id)).toHaveLength(0);
});
