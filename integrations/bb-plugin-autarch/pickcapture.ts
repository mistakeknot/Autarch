import { createHash } from "node:crypto";
import { readAsk } from "./askbody.js";
import { canonicalOnPick } from "./model.js";
import { evaluationDeadline, PICK_POLICY_VERSION, pickActionOps } from "./pickhandler.js";
import type { TargetEvidence } from "./picktarget.js";
import type { ObligationInput, PickActionRow, Store } from "./store.js";

export function productionPickMode(value: unknown): "off" | "shadow" {
  if (value === undefined) return "off";
  if (value !== "off" && value !== "shadow") throw new Error("pickHandlerMode must be off or shadow in PR3");
  return value;
}

const sha = (text: string) => createHash("sha256").update(text).digest("hex");

/** Immutable PR2 snapshots. No delivery, ledger append or historical reconstruction. */
export function capturePick(store: Store, decisionId: string, mode: "shadow" | "live", at: string): PickActionRow {
  const inPickTransaction = store.db.inTransaction;
  return store.atomically(() => {
    const snap = store.pickEvidenceSnapshot(decisionId);
    if (!snap?.pick || !snap.control) throw new Error("pick capture evidence unavailable");
    const d = snap.decision;
    const p = snap.pick;
    const ask = readAsk(d.body_json, {}, d.source);
    if (d.source !== "card" || !d.task_id || !d.generation || !ask?.options?.some(o => o.on_pick)) throw new Error("pick capture requires a participating generation");
    const option = ask.options.find(o => o.id === p.option_id);
    const declaration = option?.on_pick ? canonicalOnPick(option.on_pick) : null;
    const relay = option?.on_pick ? (option.kind === "instruction" ? option.instruction : option.on_pick.notification) ?? null : null;
    const targets = snap.target_snapshot_json ? JSON.parse(snap.target_snapshot_json) as { targets: Record<string, TargetEvidence> } : null;
    const target = targets?.targets[p.option_id];
    const ops = pickActionOps(decisionId, p.pick_id);
    const row = {
      pick_id: p.pick_id, decision_id: decisionId, task_id: d.task_id, generation: d.generation,
      revision: d.revision, card_fp: d.card_fp, declaration_json: declaration,
      declaration_sha256: declaration === null ? null : sha(declaration), relay_text: relay,
      relay_sha256: relay === null ? null : sha(relay), asker: d.thread, blocks_decision_id: decisionId,
      target_thread: target?.thread_id ?? null, target_title: target?.title ?? null,
      target_project_id: target?.project_id ?? null, target_project_name: target?.project_name ?? null,
      capture_mode: mode, capture_epoch: snap.control.epoch, policy_version: PICK_POLICY_VERSION,
      captured_at: at, evaluation_deadline_at: evaluationDeadline(at),
      ruling_op: `ruling-file:${decisionId}`, relay_op: ops.relay, fallback_op: ops.fallback,
      receipt_op: ops.receipt, status_op: ops.status,
    };
    const existing = store.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ? OR decision_id = ?").all(p.pick_id, decisionId) as PickActionRow[];
    if (existing.length) {
      const saved = existing[0];
      // Capture time and its deadline belong to the first observation, not its retry.
      const expected = { ...row, captured_at: saved.captured_at, evaluation_deadline_at: saved.evaluation_deadline_at };
      if (existing.length !== 1 || Object.entries(expected).some(([key, value]) => Reflect.get(saved, key) !== value)) throw new Error("pick capture identity conflict");
      return saved;
    }
    if (mode === "live" && (!inPickTransaction || p.by !== "mk" || store.db.prepare("SELECT 1 FROM events WHERE decision_id = ? AND type = 'picked'").get(decisionId))) throw new Error("live capture requires a new owner pick transaction");
    for (const op of [row.ruling_op, row.relay_op, row.fallback_op, row.receipt_op, row.status_op]) {
      if (store.db.prepare("SELECT pick_id FROM pick_actions WHERE ruling_op = ? OR relay_op = ? OR fallback_op = ? OR receipt_op = ? OR status_op = ?").get(op, op, op, op, op)) throw new Error("pick capture operation conflict");
    }
    const keys = Object.keys(row);
    store.db.prepare(`INSERT INTO pick_actions (${keys.join(",")}) VALUES (${keys.map(k => `@${k}`).join(",")})`).run(row);
    return store.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ?").get(p.pick_id) as PickActionRow;
  });
}

/** A permanent note insertion advances this token regardless of its posting result. */
export function pickFenceToken(store: Store, action: PickActionRow): number {
  return (store.db.prepare("SELECT COUNT(*) AS n FROM pick_note_fences WHERE decision_id = ? OR (task_id = ? AND generation = ?)").get(action.decision_id, action.task_id, action.generation) as { n: number }).n;
}

/** Conditional SQL owns the transition. Late evaluations never revive terminal work. */
export function recordPickEvaluation(store: Store, action: PickActionRow, evaluation: unknown, now: string, fenceToken: number,
  transition: { from: "pending" | "automatic"; to: PickActionRow["disposition"]; obligations?: ObligationInput[] } = { from: "pending", to: "pending" }): boolean {
  return store.atomically(() => {
    const obligations = transition.obligations ?? [];
    if (obligations.length && (action.capture_mode !== "live" || transition.to !== "automatic" || fenceToken !== 0)) return false;
    const applied = store.db.prepare(`UPDATE pick_actions
    SET evaluation_json = @evaluation, evaluated_at = @now, updated_at = @now, disposition = @to
    WHERE pick_id = @pick_id AND decision_id = @decision_id AND disposition = @from AND disposition IN ('pending','automatic')
      AND revision = @revision AND generation = @generation AND card_fp = @card_fp
      AND capture_mode = @capture_mode AND capture_epoch = @capture_epoch
      AND ruling_op IS @ruling_op AND relay_op IS @relay_op AND receipt_op IS @receipt_op
      AND fallback_op IS @fallback_op AND status_op IS @status_op
      AND target_thread IS @target_thread AND asker = @asker AND policy_version = @policy_version
      AND EXISTS (SELECT 1 FROM picks p WHERE p.pick_id = @pick_id AND p.decision_id = @decision_id AND p.revision = @revision)
      AND julianday(evaluation_deadline_at) > julianday(@now)
      AND EXISTS (SELECT 1 FROM pick_handler_control c WHERE c.id = 1 AND c.mode = @capture_mode AND c.epoch = @capture_epoch)
      AND EXISTS (SELECT 1 FROM decisions d WHERE d.id = @decision_id
        AND d.task_id = @task_id AND d.generation = @generation AND d.revision = @revision AND d.card_fp = @card_fp
        AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
        AND NOT EXISTS (SELECT 1 FROM decisions newer WHERE newer.supersedes = d.id))
      AND (SELECT COUNT(*) FROM pick_note_fences f WHERE f.decision_id = @decision_id OR (f.task_id = @task_id AND f.generation = @generation)) = @fenceToken
    `).run({ pick_id: action.pick_id, decision_id: action.decision_id, task_id: action.task_id,
      generation: action.generation, revision: action.revision, card_fp: action.card_fp,
      capture_mode: action.capture_mode, capture_epoch: action.capture_epoch,
      ruling_op: action.ruling_op, relay_op: action.relay_op, receipt_op: action.receipt_op,
      fallback_op: action.fallback_op, status_op: action.status_op,
      target_thread: action.target_thread, asker: action.asker, policy_version: action.policy_version,
      now, evaluation: JSON.stringify(evaluation), fenceToken, from: transition.from, to: transition.to }).changes === 1;
    if (!applied) return false;
    for (const obligation of obligations) insertProtectedObligation(store, action, obligation, now);
    return true;
  });
}

/** Called only under the transition write lock. Declared identities and exact saved payload are compared before INSERT. */
function insertProtectedObligation(store: Store, action: PickActionRow, input: ObligationInput, now: string): void {
  const relay = input.kind === "pick-relay";
  const receipt = input.kind === "pick-receipt";
  if ((!relay && !receipt) || input.op !== (relay ? action.relay_op : action.receipt_op)
    || input.recipient !== (relay ? action.target_thread : action.asker) || typeof input.payload !== "string" || input.after_id) throw new Error("protected obligation conflict");
  const expected = { id: input.id, decision_id: action.decision_id, kind: input.kind,
    recipient: input.recipient, payload: input.payload, op: input.op, after_id: null,
    next_try_at: "9999-12-31T23:59:59.999Z", allow_reroute: relay ? 0 : 1, allow_batch: 0, require_message_id: 1 };
  const existing = store.db.prepare("SELECT * FROM obligations WHERE id = ? OR op = ?").all(input.id, input.op) as Record<string, unknown>[];
  if (existing.length) {
    if (existing.length !== 1 || Object.entries(expected).some(([key, value]) => existing[0][key] !== value)) throw new Error("protected obligation conflict");
    return;
  }
  store.db.prepare(`INSERT INTO obligations(id, decision_id, kind, recipient, payload, op, after_id,
    next_try_at, handler_next_try_at, allow_reroute, allow_batch, require_message_id, updated_at)
    VALUES (@id, @decision_id, @kind, @recipient, @payload, @op, @after_id,
    @next_try_at, @due, @allow_reroute, @allow_batch, @require_message_id, @now)`)
    .run({ ...expected, due: input.next_try_at ?? now, now });
}
