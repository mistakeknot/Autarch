import { createHash } from "node:crypto";
import { readAsk } from "./askbody.js";
import { canonicalOnPick } from "./model.js";
import { evaluationDeadline, hasAcceptedPickEvidence, PICK_POLICY_VERSION, pickActionOps } from "./pickhandler.js";
import type { TargetEvidence } from "./picktarget.js";
import { MESSAGE_DUE_SQL, type ObligationInput, type ObligationRow, type PickActionRow, type Store } from "./store.js";
import { ledgerId } from "./ledger.js";

export function productionPickMode(value: unknown): "off" | "shadow" {
  if (value === undefined) return "off";
  if (value !== "off" && value !== "shadow") throw new Error("pickHandlerMode must be off or shadow in PR3");
  return value;
}

const sha = (text: string) => createHash("sha256").update(text).digest("hex");

/** Deterministic framing from persisted identity; caller text is never interpolated. */
export function pickRelayPayload(store: Store, pickId: string): string {
  const action = store.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ?").get(pickId) as PickActionRow | undefined;
  const pick = action && store.pickByPickId(action.pick_id);
  const decision = action && store.decision(action.decision_id);
  const ask = decision && typeof decision.body_json === "string" && decision.source === "card"
    ? readAsk(decision.body_json, {}, "card") : null;
  const option = ask?.options?.find(o => o.id === pick?.option_id);
  const declaration = option?.on_pick;
  if (!action || !pick || !declaration || action.relay_text === null || sha(action.relay_text) !== action.relay_sha256
    || canonicalOnPick(declaration) !== action.declaration_json) throw new Error("protected obligation conflict");
  const subject = declaration.subject;
  const key = store.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(action.task_id) as { card_key: string | null } | undefined;
  return [
    "Home pick relay",
    `Card: ${JSON.stringify(key?.card_key ?? action.task_id)}; decision: ${JSON.stringify(action.decision_id)}; generation: ${action.generation}`,
    `Ruling: ${ledgerId(pickId)}`,
    subject.kind === "none" ? "Subject: no artifact; head check not applicable" : `Subject: ${subject.repo}#${subject.number}; filed head: ${subject.head}`,
    `The owner picked option ${JSON.stringify(pick.option_id)}; the text below was written by ${JSON.stringify(action.asker)}.`,
    `Target: ${JSON.stringify(action.target_thread)} ${JSON.stringify(action.target_title)}; project: ${JSON.stringify(action.target_project_name)}; instruction SHA-256: ${action.relay_sha256}`,
    `Text bytes: ${Buffer.byteLength(action.relay_text, "utf8")}`,
    "BEGIN FILED TEXT", action.relay_text, "END FILED TEXT",
    "This is not merge, deploy, release or publish authorization.",
  ].join("\n");
}

export type SendGuardResult = { result: "permitted" | "refused" | "unknown"; reason: string };
export type RelayPreparation = { result: "enqueued" | "fallback" | "no-relay" | "not-applied"; reason: string };

export type PickFallbackDelivery = "none-confirmed" | "pending" | "queued" | "uncertain";
/** Ordinary (never protected) fallback messages a caller renders inside the fallback transaction. */
export interface PickFallbackOutbox { wake: string | null; notices: { recipient: string; payload: string }[] }
export const RULING_FAILURE_EVENT = "pick-ruling-file-failure";

/**
 * The guarded relay claim. Every mutable guard is compared in the same IMMEDIATE transaction as the attempt claim: the
 * stored action (not a caller object) names target, payload and operation; generation, revision, card fingerprint,
 * disposition, capture mode/epoch, note fences, the original deadline and the ruling-file failure evidence must all
 * still hold. Receipts are not claimable before their delivery stage exists.
 */
export function claimPickRelay(store: Store, id: string, attempt: number, now: string): { ok: true; attempt: number } | { ok: false } {
  return store.atomically(() => {
    const row = store.obligation(id);
    if (!row || row.kind !== "pick-relay" || row.attempt !== attempt || row.op === null) return { ok: false } as const;
    const action = store.db.prepare("SELECT * FROM pick_actions WHERE relay_op = ? AND decision_id = ?").get(row.op, row.decision_id) as PickActionRow | undefined;
    if (!action || action.target_thread === null) return { ok: false } as const;
    let payload: string;
    try { payload = pickRelayPayload(store, action.pick_id); } catch { return { ok: false } as const; }
    if (row.payload !== payload || row.recipient !== action.target_thread) return { ok: false } as const;
    const res = store.db.prepare(`UPDATE obligations AS o SET state = 'sending', attempt = o.attempt + 1, resend_permit = 0, updated_at = @now
      WHERE o.id = @id AND o.attempt = @attempt AND o.kind = 'pick-relay' AND ${MESSAGE_DUE_SQL}
        AND o.payload = @payload AND o.recipient = @target AND o.after_id IS NULL AND o.resend_permit = 0
        AND o.allow_reroute = 0 AND o.allow_batch = 0 AND o.require_message_id = 1
        AND EXISTS (SELECT 1 FROM pick_actions p WHERE p.pick_id = @pick_id AND p.relay_op = o.op AND p.decision_id = o.decision_id
          AND p.target_thread = o.recipient AND p.capture_mode = 'live' AND p.disposition = 'automatic'
          AND p.policy_version = @policy AND julianday(p.evaluation_deadline_at) > julianday(@now)
          AND EXISTS (SELECT 1 FROM pick_handler_control c WHERE c.id = 1 AND c.mode = 'live' AND c.epoch = p.capture_epoch)
          AND EXISTS (SELECT 1 FROM picks k WHERE k.pick_id = p.pick_id AND k.decision_id = p.decision_id AND k.revision = p.revision)
          AND EXISTS (SELECT 1 FROM decisions d WHERE d.id = p.decision_id AND d.task_id = p.task_id AND d.generation = p.generation
            AND d.revision = p.revision AND d.card_fp = p.card_fp AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
            AND NOT EXISTS (SELECT 1 FROM decisions newer WHERE newer.supersedes = d.id))
          AND NOT EXISTS (SELECT 1 FROM pick_note_fences f WHERE f.decision_id = p.decision_id OR (f.task_id = p.task_id AND f.generation = p.generation))
          AND EXISTS (SELECT 1 FROM obligations r WHERE r.op = p.ruling_op AND r.decision_id = p.decision_id AND r.kind = 'ruling-file'
            AND r.attempt = 0 AND r.state IN ('pending','done'))
          AND NOT EXISTS (SELECT 1 FROM events e WHERE e.decision_id = p.decision_id AND e.type = '${RULING_FAILURE_EVENT}'))`)
      .run({ id, attempt, now, payload, target: action.target_thread, pick_id: action.pick_id, policy: PICK_POLICY_VERSION });
    if (res.changes !== 1) return { ok: false } as const;
    store.db.prepare("INSERT INTO attempts(obligation_id, n, state, at) VALUES (?, ?, 'sending', ?)").run(id, attempt + 1, now);
    return { ok: true, attempt: attempt + 1 } as const;
  });
}

/** Strict insert for ordinary fallback rows: an existing declared identity must be identical, never ignored or replaced. */
function insertOrdinaryStrict(store: Store, decisionId: string, rows: ObligationInput[], now: string): void {
  for (const input of rows) {
    if (input.kind === "pick-relay" || input.kind === "pick-receipt") throw new Error("protected obligations require guarded insertion");
    const expected = { decision_id: decisionId, kind: input.kind, recipient: input.recipient ?? null, payload: input.payload ?? null, op: input.op ?? null, after_id: null };
    const existing = store.db.prepare("SELECT * FROM obligations WHERE id = ? OR op = ?").all(input.id, input.op ?? null) as Record<string, unknown>[];
    if (existing.length) {
      if (existing.length !== 1 || existing[0]!.id !== input.id || Object.entries(expected).some(([key, value]) => existing[0]![key] !== value)) throw new Error("pick fallback obligation conflict");
      continue;
    }
    store.db.prepare(`INSERT INTO obligations(id, decision_id, kind, recipient, payload, op, after_id, next_try_at, updated_at)
      VALUES (@id, @decision_id, @kind, @recipient, @payload, @op, NULL, @now, @now)`).run({ id: input.id, ...expected, now });
  }
}

/**
 * Terminal fallback for a live action. Unlike the automatic transition, it requires only a non-terminal disposition:
 * an expired deadline, a changed epoch or an invalid generation must still release the work. In one transaction it
 * moves the disposition (never reviving a terminal one), voids an unattempted relay, records the outcome and inserts
 * the ordinary asker wake (no after_id) and information-only notices. A confirmed relay is left to receipt handling;
 * a possibly accepted one is preserved and reported as such.
 */
export function fallbackPickAction(store: Store, pickId: string, reason: string, now: string,
  render: (action: PickActionRow, delivery: PickFallbackDelivery) => PickFallbackOutbox, evaluation: unknown = null): boolean {
  return store.atomically(() => {
    const action = store.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ?").get(pickId) as PickActionRow | undefined;
    if (!action || action.capture_mode !== "live" || (action.disposition !== "pending" && action.disposition !== "automatic")) return false;
    const relay = action.relay_op === null ? undefined
      : store.db.prepare("SELECT * FROM obligations WHERE op = ? AND kind = 'pick-relay' AND decision_id = ?").get(action.relay_op, action.decision_id) as ObligationRow | undefined;
    const attempts = relay ? store.attempts(relay.id) : [];
    if (relay && attempts.some(a => hasAcceptedPickEvidence(store, relay, a))) return false;
    const states = attempts.map(a => a.state);
    const delivery: PickFallbackDelivery = states.includes("uncertain") || states.includes("delivered")
      || (relay && (relay.state === "done" || relay.state === "uncertain" || relay.attempt !== attempts.length)) ? "uncertain"
      : states.includes("queued") ? "queued" : states.includes("sending") ? "pending" : "none-confirmed";
    const applied = store.db.prepare(`UPDATE pick_actions SET disposition = 'fallback', updated_at = @now,
        evaluation_json = CASE WHEN @evaluation IS NOT NULL AND evaluation_json IS NULL AND julianday(evaluation_deadline_at) > julianday(@now) THEN @evaluation ELSE evaluation_json END,
        evaluated_at = CASE WHEN @evaluation IS NOT NULL AND evaluation_json IS NULL AND julianday(evaluation_deadline_at) > julianday(@now) THEN @now ELSE evaluated_at END
      WHERE pick_id = @pick_id AND capture_mode = 'live' AND disposition IN ('pending','automatic')`)
      .run({ pick_id: pickId, now, evaluation: evaluation === null ? null : JSON.stringify(evaluation) }).changes === 1;
    if (!applied) return false;
    if (relay && delivery === "none-confirmed" && relay.voided_at === null) store.voidObligation(relay.id);
    store.recordEvent("pick-fallback", action.decision_id, { pick_id: pickId, generation: action.generation,
      policy_version: action.policy_version, reason, delivery, relay: relay?.id ?? null, at: now, evaluation });
    const out = render(action, delivery);
    const rows: ObligationInput[] = [];
    if (out.wake !== null && action.asker !== "" && action.fallback_op) {
      rows.push({ id: `ob:${action.fallback_op}`, kind: "wake", recipient: action.asker, op: action.fallback_op, payload: out.wake });
    }
    for (const n of out.notices) {
      if (n.recipient === action.asker || n.recipient === action.target_thread) continue;
      rows.push({ id: `ob:${action.fallback_op}:notice:${n.recipient}`, kind: "notice", recipient: n.recipient, op: `${action.fallback_op}:notice:${n.recipient}`, payload: n.payload });
    }
    insertOrdinaryStrict(store, action.decision_id, rows, now);
    return true;
  });
}

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
    // Caller snapshots carry no authority. Read immutable values under the same
    // write lock as the CAS and outbox insert; retain CAS checks for mutable state.
    const stored = store.db.prepare("SELECT * FROM pick_actions WHERE pick_id = ?").get(action.pick_id) as PickActionRow | undefined;
    const mutable = new Set(["evaluation_json", "evaluation_started_at", "evaluated_at", "disposition", "updated_at"]);
    if (!stored || Object.entries(stored).some(([key, value]) => !mutable.has(key) && Reflect.get(action, key) !== value)) return false;
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
    for (const obligation of obligations) insertProtectedObligation(store, stored, obligation, now);
    return true;
  });
}

/** Called only under the transition write lock. Declared identities and exact saved payload are compared before INSERT. */
function insertProtectedObligation(store: Store, action: PickActionRow, input: ObligationInput, now: string, receiptRequiresId: 0 | 1 = 1): void {
  const relay = input.kind === "pick-relay";
  const receipt = input.kind === "pick-receipt";
  if ((!relay && !receipt) || input.op !== (relay ? action.relay_op : action.receipt_op)
    || input.recipient !== (relay ? action.target_thread : action.asker) || typeof input.payload !== "string" || input.after_id) throw new Error("protected obligation conflict");
  if (relay && input.payload !== pickRelayPayload(store, action.pick_id)) throw new Error("protected obligation conflict");
  const expected = { id: input.id, decision_id: action.decision_id, kind: input.kind,
    recipient: input.recipient, payload: input.payload, op: input.op, after_id: null,
    next_try_at: "9999-12-31T23:59:59.999Z", allow_reroute: relay ? 0 : 1, allow_batch: 0, require_message_id: relay ? 1 : receiptRequiresId };
  const existing = store.db.prepare("SELECT * FROM obligations WHERE id = ? OR op = ?").all(input.id, input.op) as Record<string, unknown>[];
  if (existing.length) {
    if (receipt && existing.length === 1 && existing[0].recipient !== action.asker) {
      // The immutable creator still names the asker; only an audited informational route may differ.
      const route = store.db.prepare(`SELECT 1 FROM events WHERE decision_id = ? AND type = 'rerouted'
        AND json_extract(detail_json, '$.obligation') = ? AND json_extract(detail_json, '$.to') = ?`).get(action.decision_id, input.id, existing[0].recipient);
      if (route) expected.recipient = String(existing[0].recipient);
    }
    if (existing.length !== 1 || Object.entries(expected).some(([key, value]) => existing[0][key] !== value)) throw new Error("protected obligation conflict");
    return;
  }
  store.db.prepare(`INSERT INTO obligations(id, decision_id, kind, recipient, payload, op, after_id,
    next_try_at, handler_next_try_at, allow_reroute, allow_batch, require_message_id, updated_at)
    VALUES (@id, @decision_id, @kind, @recipient, @payload, @op, @after_id,
    @next_try_at, @due, @allow_reroute, @allow_batch, @require_message_id, @now)`)
    .run({ ...expected, due: input.next_try_at ?? now, now });
}

/** Reconstruct earned receipts from authoritative attempts, including after a crash or deadline. */
export function reconcilePickReceipts(store: Store, now: string): void {
  store.atomically(() => {
    const actions = store.db.prepare("SELECT * FROM pick_actions WHERE capture_mode = 'live'").all() as PickActionRow[];
    for (const action of actions) {
      const relay = store.db.prepare("SELECT id FROM obligations WHERE op = ? AND decision_id = ? AND kind = 'pick-relay'").get(action.relay_op, action.decision_id) as { id: string } | undefined;
      const relayRow = relay && store.obligation(relay.id);
      const accepted = relayRow && store.attempts(relayRow.id).find(a => hasAcceptedPickEvidence(store, relayRow, a));
      if (!accepted || !action.receipt_op) continue;
      const payload = ["Home pick: already relayed; do not act",
        `Target: ${JSON.stringify(action.target_title)}; project: ${JSON.stringify(action.target_project_name)}`,
        `Instruction SHA-256: ${action.relay_sha256}`, `Ruling: ${ledgerId(action.pick_id)}`,
        `Host accepted ID: ${accepted.evidence}`,
        `Readback: bb home pick-action --decision ${action.decision_id} --generation ${action.generation} --json`].join("\n");
      // Earned informational receipts need no durable-ID gate. Preserve the existing stricter
      // transition API default for previously declared protected operations and their exact replay.
      insertProtectedObligation(store, action, { id: `ob:${action.receipt_op}`, kind: "pick-receipt", op: action.receipt_op, recipient: action.asker, payload }, now, 0);
    }
  });
}

export function claimPickReceipt(store: Store, id: string, attempt: number, now: string): { ok: true; attempt: number } | { ok: false } {
  return store.atomically(() => {
    reconcilePickReceipts(store, now);
    const applied = store.db.prepare(`UPDATE obligations AS o SET state = 'sending', attempt = attempt + 1, updated_at = @now
      WHERE id = @id AND attempt = @attempt AND kind = 'pick-receipt' AND ${MESSAGE_DUE_SQL}
        AND allow_batch = 0 AND allow_reroute = 1 AND require_message_id = 0
        AND EXISTS (SELECT 1 FROM pick_actions p WHERE p.receipt_op = o.op AND p.decision_id = o.decision_id AND p.capture_mode = 'live')`)
      .run({ id, attempt, now }).changes;
    if (applied !== 1) return { ok: false } as const;
    store.db.prepare("INSERT INTO attempts(obligation_id, n, state, at) VALUES (?, ?, 'sending', ?)").run(id, attempt + 1, now);
    return { ok: true, attempt: attempt + 1 } as const;
  });
}
