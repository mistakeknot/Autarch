import assert from "node:assert/strict";
import { readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { recoveryRig } from "../../__tests__/pickhandler-stage4-rig.js";
import { envs } from "../../__tests__/card-rig.js";
import { FakeSdk, archived } from "../../__tests__/wakes-helpers.js";
import { WakeLoop } from "../../wakes.js";
import { LIVE_DELIVERY_AVAILABLE } from "../../pickhandler.js";
import type { PickActionRow } from "../../store.js";
import Database from "better-sqlite3";
import { loadV6 } from "../../__tests__/v6build.js";
import { TasksClient } from "../../tasks.js";

type Fixture = Awaited<ReturnType<typeof recoveryRig>>;
const action = (f: Fixture) => f.r.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(f.g.id) as PickActionRow;
const rows = (f: Fixture, kind: string) => f.r.svc.store.obligationsFor(f.g.id).filter(o => o.kind === kind);
const query = (f: Fixture) => ({ decision_id: f.g.id, generation: f.g.generation });
async function pick(f: Fixture, reason?: string) {
  assert.equal((await f.r.validatedMkPick(f.g.id, "scenario-pick", "a", reason)).ok, true);
  await f.r.svc.pickWorkIdle();
}
async function using(run: (f: Fixture) => Promise<Record<string, unknown>>, opts: Parameters<typeof recoveryRig>[0] = {}) {
  const f = await recoveryRig(opts);
  try {
    const evidence = await run(f);
    assert.equal(LIVE_DELIVERY_AVAILABLE, false);
    const status = f.g ? action(f)?.status_op : undefined;
    const statusOperations = f.g ? f.r.svc.store.obligationsFor(f.g.id).filter(o => o.op === status).length : 0;
    assert.equal(statusOperations, 0);
    assert.equal(f.r.fake.comments.filter(c => c.body.includes("home-pick-status:")).length, evidence.seeded_status_comments ?? 0);
    // The frozen producers declare these nine legacy obligation kinds. PR3 adds
    // exactly relay/receipt; no status operation may hide under an ordinary kind.
    const legacy = ["owner", "ruling-file", "wake", "notice", "steps-done", "hold-ended", "note", "void-notice", "notice-digest"];
    const producers = ["service.ts", "delegation.ts", "store.ts"].map(name => readFileSync(new URL(`../../__tests__/fixtures/v6-reader/${name}`, import.meta.url), "utf8")).join("\n");
    for (const kind of legacy) assert.ok(producers.includes(`kind: "${kind}"`) || producers.includes(`DIGEST_KIND = "${kind}"`));
    const allowed = new Set([...legacy, "pick-relay", "pick-receipt"]);
    if (f.g) for (const row of f.r.svc.store.obligationsFor(f.g.id)) {
      assert.ok(allowed.has(row.kind), `unexpected obligation kind ${row.kind}`);
      assert.ok(!row.payload?.includes("home-pick-status:"));
    }
    return { ...evidence, live_available: LIVE_DELIVERY_AVAILABLE, status_operations: statusOperations };
  } finally {
    while (envs.length) rmSync(envs.pop()!.dir, { recursive: true, force: true });
    f.temp.cleanup();
  }
}

/** Isolated dormant runtime only. These exercise Service/Store/WakeLoop, never real host sends. */
export const pickHandlerScenarios = {
  "pick-handler-target-project": async () => {
    const outcomes: boolean[] = [];
    for (const missing of [false, true]) {
      let changed = false;
      await using(async f => {
        changed = true;
        const result = await f.r.validatedMkPick(f.g.id);
        outcomes.push(result.ok);
        assert.equal(result.ok, false);
        assert.equal(f.r.svc.store.pick(f.g.id), undefined);
        assert.equal(rows(f, "pick-relay").length, 0);
        return {};
      }, { svc: { readTargetEvidence: async id => {
        if (changed && missing) throw new Error("project relation unavailable");
        return { schema: 1, thread_id: id, title: "Work", project_id: changed ? "changed" : "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" };
      } } });
    }
    return { rejected_relations: outcomes.filter(x => !x).length, live_available: LIVE_DELIVERY_AVAILABLE, status_operations: 0, host_ui_go: "native wrapper read-only proof" };
  },

  "pick-handler-shadow-failure": () => using(async f => {
    f.r.svc.store.pickEvidenceSnapshot = () => { throw new Error("capture unavailable"); };
    await pick(f);
    assert.ok(f.r.svc.store.pick(f.g.id));
    assert.deepEqual(f.r.svc.store.obligationsFor(f.g.id).map(o => o.kind).sort(), ["notice", "ruling-file", "wake"]);
    await f.r.writer.drain();
    assert.ok(f.r.fake.comments.some(c => c.body.startsWith("Ruled:")));
    assert.equal(f.task.labelIds.length, 0);
    return { committed_picks: 1, ordinary_wakes: rows(f, "wake").length, ruled_comments: f.r.fake.comments.filter(c => c.body.startsWith("Ruled:")).length, relay_rows: rows(f, "pick-relay").length };
  }, { svc: { pickCaptureRuntime: undefined, pickHandlerMode: "shadow" } }),

  "pick-handler-home-writeback": () => using(async f => {
    await pick(f); await f.r.writer.drain();
    const statusBody = `Pick handler: pending\n\nhome-pick-status: ${action(f).status_op}`;
    // N-status: seed only in fake Tasks; PR3 has no status writer.
    const comment = f.r.fake.addComment(f.task.id, { kind: "user", threadId: null, body: statusBody });
    const clean = await f.r.svc.pickEvaluation(f.g.id, () => new TasksClient(f.r.fake));
    assert.equal(clean?.evaluation.checks["no-extra-words"].result, "pass");
    comment.body += "\nOnly after review.";
    const dirty = await f.r.svc.pickEvaluation(f.g.id, () => new TasksClient(f.r.fake));
    assert.equal(dirty?.evaluation.checks["no-extra-words"].result, "fail");
    const note = f.r.svc.note({ decision_id: f.g.id }, "Wait for me", "scenario-note", { deferWake: true });
    assert.equal(note.ok, true);
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(sdk.sent.filter(s => s.input.includes("Home pick relay")).length, 0);
    return { marker_check: clean?.evaluation.checks["no-extra-words"].result, prose_check: dirty?.evaluation.checks["no-extra-words"].result, relay_sends: 0, ruled_comments: f.r.fake.comments.filter(c => c.body.startsWith("Ruled:")).length, seeded_status_comments: 1 };
  }),

  "pick-handler-ruling-failure": () => using(async f => {
    await pick(f);
    const ruling = rows(f, "ruling-file")[0];
    f.r.db.prepare("UPDATE obligations SET state = 'pending', attempt = 1, last_error = 'write failed', next_try_at = '2099-01-01T00:00:00Z' WHERE id = ?").run(ruling.id);
    f.r.svc.expirePickEvaluations();
    assert.equal(action(f).disposition, "fallback");
    assert.equal(rows(f, "wake")[0].after_id, null);
    f.r.db.prepare("UPDATE obligations SET state = 'done', last_error = NULL WHERE id = ?").run(ruling.id);
    f.r.svc.resumePickActions(); await f.r.svc.pickWorkIdle();
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(sdk.sent.filter(s => s.input.includes("Home pick relay")).length, 0);
    return { disposition: action(f).disposition, relay_sends: 0, independent_fallback: rows(f, "wake")[0].after_id === null, failure_attempts: f.r.svc.store.obligation(ruling.id)!.attempt };
  }),

  "pick-handler-deadline": () => using(async f => {
    const evaluate = f.r.svc.pickEvaluation.bind(f.r.svc);
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    f.r.svc.pickEvaluation = async (...args) => { await gate; return evaluate(...args); };
    assert.equal((await f.r.validatedMkPick(f.g.id, "scenario-pick")).ok, true);
    const deadline = action(f).evaluation_deadline_at;
    f.r.advance(120_000); f.r.svc.expirePickEvaluations();
    assert.equal(action(f).disposition, "fallback");
    release(); await f.r.svc.pickWorkIdle();
    f.r.restart();
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(action(f).evaluation_deadline_at, deadline);
    assert.equal(rows(f, "pick-relay").length, 0);
    return { deadline_preserved: action(f).evaluation_deadline_at === deadline, disposition: action(f).disposition, relay_rows: rows(f, "pick-relay").length };
  }),

  "pick-handler-old-reader": () => using(async f => {
    await pick(f);
    const v6 = await loadV6();
    const old = new v6.Store(new Database(f.r.env.file), { now: f.r.env.now, noticeWindowMs: 0 });
    try {
      const service = Object.create(f.r.svc);
      Object.defineProperty(service, "store", { value: old });
      const sdk = new FakeSdk(); const loop = new v6.WakeLoop(service, sdk, async () => "thr_successor");
      f.r.svc.store.insertObligations(f.g.id, [{ id: "legacy-control", kind: "wake", recipient: "thr_legacy", op: "legacy-control", payload: "ordinary exact 日本語\n\u0007" }]);
      const relay = rows(f, "pick-relay")[0];
      old.resend(relay.id, 0, "old-resend");
      await loop.drain(); await loop.drain();
      assert.deepEqual(sdk.sent.map(s => s.threadId), ["thr_legacy"]);
      assert.equal(sdk.sent[0].input, "[home-msg legacy-control #1] ordinary exact 日本語\n\u0007");
      assert.equal(old.obligation(relay.id)!.next_try_at, "9999-12-31T23:59:59.999Z");
      assert.throws(() => v6.parseExistingAsk(JSON.parse(String(f.r.svc.store.decision(f.g.id)!.body_json))), /unknown field/);
      return { protected_attempts: old.attempts(relay.id).length, legacy_sends: sdk.sent.length, sentinel: old.obligation(relay.id)!.next_try_at, rollback_limit: "drain started attempts before rollback" };
    } finally { old.close(); }
  }),
  "pick-handler-none": () => using(async f => {
    let headReads = 0;
    const evaluate = f.r.svc.pickEvaluation.bind(f.r.svc);
    f.r.svc.pickEvaluation = (id, tasks, opts) => evaluate(id, tasks, { ...opts, readHead: async () => { headReads++; throw new Error("none must not read a head"); } });
    await pick(f);
    const sdk = new FakeSdk(); const loop = new WakeLoop(f.r.svc, sdk);
    await loop.drain(); await loop.drain();
    const view = f.r.svc.readPickAction(query(f));
    assert.equal(view.kind, "action");
    assert.equal(action(f).disposition, "automatic");
    const subjectCheck = JSON.parse(action(f).evaluation_json!).checks["subject-head"].result;
    assert.equal(subjectCheck, "not-applicable"); assert.equal(headReads, 0);
    const target = sdk.sent.filter(s => s.threadId === "thr_target");
    assert.equal(target.length, 1); assert.match(target[0].input, /Blue was selected\./);
    const ledger = readFileSync(join(f.temp.dir, "rulings.jsonl"), "utf8").trim().split("\n");
    assert.equal(ledger.length, 1);
    return { target_sends: target.length, ruling_rows: ledger.length, receipt_rows: rows(f, "pick-receipt").length, subject_check: subjectCheck, head_reads: headReads };
  }, { rulingOnly: true }),

  "pick-handler-direct-target": () => using(async f => {
    assert.equal(f.g, undefined);
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(sdk.sent.length, 0);
    return { generations: f.r.gens(f.task.id).length, target_sends: sdk.sent.length, state: f.r.cardRow(f.task.id).state };
  }, { blocks: "thread:thr_other" }),

  "pick-handler-no-reroute": () => using(async f => {
    await pick(f);
    const sdk = new FakeSdk(); sdk.script.push({ kind: "throw", err: archived() });
    const loop = new WakeLoop(f.r.svc, sdk, async () => "thr_successor");
    await loop.drain(); await loop.drain();
    assert.equal(sdk.sent.filter(s => s.threadId === "thr_successor").length, 0);
    assert.equal(rows(f, "pick-relay")[0].recipient, "thr_target");
    return { successor_sends: 0, recipient: rows(f, "pick-relay")[0].recipient, attempts: f.r.svc.store.attempts(rows(f, "pick-relay")[0].id).length };
  }),

  "pick-handler-single-target": () => using(async f => {
    await pick(f);
    const sdk = new FakeSdk(); const loop = new WakeLoop(f.r.svc, sdk);
    await loop.drain(); await loop.drain();
    assert.equal(rows(f, "wake").length, 0);
    assert.equal(rows(f, "notice").filter(o => o.recipient === "thr_a").length, 0);
    assert.ok(rows(f, "notice").some(o => o.recipient === "thr_other" && o.payload?.includes("Information only")));
    assert.equal(sdk.sent.filter(s => s.input.includes("Home pick relay")).length, 1);
    return { relay_sends: 1, target_notices: 0, ordinary_wakes: rows(f, "wake").length, other_notices: rows(f, "notice").length };
  }, { target: "thr_a", blocks: "thread:thr_a thread:thr_other" }),

  "pick-handler-waits-watch": () => using(async f => {
    await pick(f);
    const before = f.r.svc.readPickAction(query(f));
    assert.equal(before.kind, "action"); assert.equal(before.ordinary_continuation, false);
    await f.r.writer.drain();
    const sdk = new FakeSdk(); const loop = new WakeLoop(f.r.svc, sdk);
    await loop.drain(); await loop.reconcile(); await loop.drain();
    f.r.svc.reconcile(f.g.id);
    const after = f.r.svc.readPickAction(query(f));
    assert.equal(after.kind, "action");
    assert.equal(after.handler_status, "already relayed"); assert.equal(after.ordinary_continuation, false);
    assert.equal(sdk.sent.filter(s => s.input.includes("Home pick relay")).length, 1);
    assert.equal(sdk.sent.filter(s => s.input.includes("already relayed; do not act")).length, 1);
    return { original_generation: f.g.generation, relay_sends: 1, receipt_sends: 1, handler_status: after.handler_status, consumer: "deferred PR4: external waits-watch" };
  }),

  "pick-handler-replay": () => using(async f => {
    await pick(f);
    const sdk = new FakeSdk();
    sdk.script.push({ kind: "queue-throw", err: new Error("response lost") });
    await new WakeLoop(f.r.svc, sdk).drain();
    const queued = [...sdk.rows.values()][0]; assert.ok(queued);
    sdk.dispatch(queued.id);
    f.r.restart();
    const loop = new WakeLoop(f.r.svc, sdk);
    await loop.reconcile(); await loop.drain(); await loop.drain();
    const view = f.r.svc.readPickAction(query(f));
    assert.equal(view.kind, "action"); assert.equal(view.handler_status, "already relayed");
    assert.equal(sdk.sent.filter(s => s.threadId === "thr_target").length, 1);
    return { relay_sends: 1, receipt_rows: rows(f, "pick-receipt").length, handler_status: view.handler_status, attempts: f.r.svc.store.attempts(rows(f, "pick-relay")[0].id).length };
  }),

  "pick-handler-fallback": () => using(async f => {
    const words = "日本語 مرحبا\n\u0007 Only after review.  ";
    await pick(f, words);
    assert.equal(action(f).disposition, "fallback");
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(rows(f, "pick-relay").length, 0);
    assert.ok(sdk.sent.some(s => s.threadId === "thr_a" && s.input.includes(words)));
    assert.match(rows(f, "wake")[0].payload!, /no-extra-words: fail/);
    return { relay_rows: rows(f, "pick-relay").length, words_preserved: sdk.sent.some(s => s.input.includes(words)), disposition: action(f).disposition };
  }),

  "pick-handler-off": () => using(async f => {
    await pick(f);
    const sdk = new FakeSdk(); sdk.script.push({ kind: "queue" });
    const loop = new WakeLoop(f.r.svc, sdk); await loop.drain();
    f.r.db.prepare("UPDATE pick_handler_control SET mode = 'off', epoch = epoch + 1").run();
    await loop.drain(); await loop.drain();
    assert.equal(sdk.sent.filter(s => s.threadId === "thr_target").length, 1);
    assert.equal(action(f).disposition, "fallback");
    assert.equal(f.r.svc.store.attempts(rows(f, "pick-relay")[0].id).length, 1);
    return { relay_sends: 1, attempts: 1, disposition: action(f).disposition, cancelled_handles: sdk.deletes.length };
  }),

  "pick-handler-lifecycle": () => using(async f => {
    await pick(f);
    const sdk = new FakeSdk(); let loop = new WakeLoop(f.r.svc, sdk);
    await loop.drain(); await loop.drain(); await f.r.writer.drain();
    const back = f.r.dele.bringBack(f.task.id, f.g.id, null, {}); assert.equal(back.ok, true);
    await f.r.writer.drain(); f.r.restart();
    loop = new WakeLoop(f.r.svc, sdk); await loop.drain();
    const view = f.r.svc.readPickAction(query(f));
    assert.equal(view.kind, "action"); assert.equal(view.handler_status, "already relayed");
    assert.equal(sdk.sent.filter(s => s.input.includes("Home pick relay")).length, 1);
    return { generations: f.r.gens(f.task.id).length, relay_sends: 1, original_status: view.handler_status };
  }),

  "pick-handler-cell-scope": () => using(async f => {
    f.r.db.prepare("UPDATE pick_handler_control SET enabled_cells_json = '[\"ruling-only:none\"]', epoch = epoch + 1, owner_enablement_json = '{\"fixture\":\"other cell\"}'").run();
    await pick(f);
    const sdk = new FakeSdk(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(rows(f, "pick-relay").length, 0); assert.equal(action(f).disposition, "fallback");
    assert.throws(() => f.r.db.prepare("UPDATE pick_handler_control SET enabled_cells_json = '[\"instruction:none\"]', epoch = epoch + 1").run(), /new owner evidence/);
    f.r.db.prepare("UPDATE pick_handler_control SET enabled_cells_json = '[\"instruction:none\"]', epoch = epoch + 1, owner_enablement_json = '{\"fixture\":\"new decision\"}'").run();
    f.r.svc.resumePickActions(); await f.r.svc.pickWorkIdle(); await new WakeLoop(f.r.svc, sdk).drain();
    assert.equal(rows(f, "pick-relay").length, 0);
    return { relay_rows: 0, disposition: action(f).disposition, replay_after_widening: rows(f, "pick-relay").length };
  }),
};
