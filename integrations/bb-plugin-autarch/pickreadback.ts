// Generation-specific observation only. This module never invokes reconciliation or delivery.
import { z } from "zod";
import { readAsk } from "./askbody.js";
import { canonicalOnPick } from "./model.js";
import { createHash } from "node:crypto";
import { ledgerId } from "./ledger.js";
import { isOwnWordsForm } from "./ownwords.js";
import { LIVE_DELIVERY_AVAILABLE, PICK_POLICY_VERSION, type PickEvaluation } from "./pickhandler.js";
import { pickFactSheet, pickFactsJson, type PickDelivery, type PickFacts } from "./pickfacts.js";
import type { Store, PickActionRow, PickHandlerControlRow } from "./store.js";
import { PickActionQuerySchema, type PickActionQuery } from "./pickquery.js";
export type { PickActionQuery } from "./pickquery.js";
const object = z.record(z.string(), z.unknown());
const jsonObject = (raw: string) => object.parse(JSON.parse(raw));
const sha = (text: string) => createHash("sha256").update(text).digest("hex");
const check = z.object({ result: z.enum(["pass", "fail", "unknown", "not-applicable"]), reasons: z.array(z.string()) });
const observedEvidence = z.object({
  comments: z.object({ result: z.enum(["pass", "fail", "unknown"]), reason: z.string(), observed_at: z.string(), cutoff: z.string().nullable(),
    comments: z.array(z.object({ id: z.string(), created_at: z.string() })), owner_words: z.array(z.object({ id: z.string(), text: z.string() })).optional(), limitations: z.array(z.string()) }).optional(),
  head: z.object({ result: z.enum(["pass", "fail", "unknown", "not-applicable"]), reason: z.string(), observed_at: z.string(), head: z.string().nullable(), state: z.string().nullable() }).nullable().optional(),
});
const evaluationSchema: z.ZodType<PickEvaluation> = z.object({
  policy_version: z.number().int(), decision_id: z.string(), pick_id: z.string(), generation: z.number().nullable(),
  selected_option: z.string(), recommended_option: z.string().nullable(),
  checks: z.object({ recommendation: check, "no-extra-words": check, reversible: check, "subject-head": check }),
  exclusions: z.array(z.object({ code: z.string() })), cell: z.enum(["instruction:none", "instruction:github-pr", "ruling-only:none", "ruling-only:github-pr"]).nullable(),
  outcome: z.enum(["automatic", "fallback"]), activation: z.object({ mode: z.enum(["off", "shadow", "live"]), permitted: z.literal(false), reason: z.string() }),
  evidence_times: z.object({ evaluated_at: z.string(), comments_observed_at: z.string().nullable(), head_observed_at: z.string().nullable(), target_observed_at: z.string().nullable() }),
  limitations: z.array(z.string()),
  evidence: observedEvidence.optional(),
}).passthrough();

export interface PickReadSources {
  store: Store; now: () => string;
  ledger: (decision: string) => PickFacts["ledger"];
  imageCount: (op: string) => number;
  mode: "off" | "shadow";
}
export type PickActionView = ReturnType<typeof readPickAction>;
export function readPickAction(sources: PickReadSources, input: PickActionQuery) {
  const query = PickActionQuerySchema.safeParse(input);
  if (!query.success) return { ok: false as const, kind: "invalid-query" as const, status: 400, error: "use original pick_id OR decision_id with exact positive generation", ordinary_continuation: false };
  let ordinary = false;
  try { return sources.store.db.transaction(() => readSnapshot(sources, query.data, value => { ordinary = value; })).deferred(); }
  catch (error) { return unavailable(`stored action evidence could not be read: ${error instanceof Error ? error.message : "unknown error"}`, ordinary); }
}
function unavailable(error: string, ordinary_continuation = false) {
  return { ok: false as const, kind: "unavailable" as const, status: 503, error, ordinary_continuation };
}
function readSnapshot(s: PickReadSources, query: PickActionQuery, routing: (ordinary: boolean) => void) {
  const { store } = s;
  const db = store.db;
  const pick = "pick_id" in query ? store.pickByPickId(query.pick_id) : store.pick(query.decision_id);
  const id = "decision_id" in query ? query.decision_id : pick?.decision_id;
  const d = id ? store.decision(id) : undefined;
  if (!d || !pick || ("generation" in query && d.generation !== query.generation)) return unavailable("original pick/generation unavailable");
  const action = db.prepare("SELECT * FROM pick_actions WHERE pick_id = ? AND decision_id = ?").get(pick.pick_id, pick.decision_id) as PickActionRow | undefined;
  const work = store.obligationsFor(pick.decision_id);
  // The ordinary wake is inserted in the pick transaction. Its declared operation is evidence of routing,
  // independent of today's mode, labels, closure or latest generation.
  const protectedWork = work.some(o => o.kind === "pick-relay" || o.kind === "pick-receipt");
  const ordinary = work.some(o => o.kind === "wake" && o.op === `wake:${pick.decision_id}:${pick.pick_id}`)
    && !protectedWork && action?.capture_mode !== "live";
  // Use the same conservative proof for explicit failures and thrown read/parse failures.
  routing(ordinary);
  const failed = (error: string) => unavailable(error, ordinary);
  const ask = typeof d.body_json === "string" ? readAsk(d.body_json, {}, d.source) : null;
  if (!ask) return failed("immutable ask unavailable");
  if (!action) {
    if (protectedWork) return failed("protected routing without captured action");
    if (!ask.options?.some(o => o.on_pick)) return { ok: true as const, kind: "no-action" as const, ordinary_continuation: true, pick_id: pick.pick_id, decision_id: pick.decision_id, generation: d.generation ?? null };
    return failed("participating capture unavailable; collection is not a successful shadow sample");
  }
  if (action.generation !== d.generation || action.revision !== pick.revision || action.card_fp !== d.card_fp) return failed("immutable identity conflict");
  const events = (db.prepare("SELECT seq, at, type, detail_json FROM events WHERE decision_id = ? ORDER BY seq").all(pick.decision_id) as { seq: number; at: string; type: string; detail_json: string }[])
    .map(e => ({ ...e, detail: jsonObject(e.detail_json) }));
  const selected = ask.options?.find(o => o.id === pick.option_id);
  const declaration = selected?.on_pick ?? null;
  if (action.declaration_json !== (declaration ? canonicalOnPick(declaration) : null)
    || (action.declaration_json !== null && sha(action.declaration_json) !== action.declaration_sha256)
    || (action.relay_text !== null && sha(action.relay_text) !== action.relay_sha256)) return failed("immutable declaration/text conflict");
  let evaluation: PickEvaluation | null = null;
  if (action.evaluation_json !== null) {
    evaluation = evaluationSchema.parse(JSON.parse(action.evaluation_json));
    if (evaluation.pick_id !== pick.pick_id || evaluation.decision_id !== pick.decision_id) return failed("evaluation identity conflict");
  }
  const operations = [action.ruling_op, action.relay_op, action.fallback_op, action.receipt_op].map(op => ({ op,
    rows: work.filter(o => op !== null && o.op === op).map(row => ({ row, attempts: store.attempts(row.id).map(a => ({ ...a, queue_event: a.handle ? store.queueEvent(a.handle) ?? null : null })) })),
  }));
  const relay = operations[1]!.rows;
  const attempts = relay.flatMap(r => r.attempts);
  const accepted = attempts.find(a => a.state === "delivered" && a.evidence?.trim() && a.evidence !== "sent" && a.evidence !== a.handle);
  let delivery: PickDelivery = "unknown";
  if (relay.length > 1 || relay.some(r => r.row.kind !== "pick-relay" || r.row.recipient !== action.target_thread)) return failed("relay operation conflict");
  if (accepted) delivery = "confirmed";
  else if (attempts.some(a => a.state === "uncertain" || a.state === "delivered") || relay.some(r => r.row.state === "uncertain" || r.row.state === "done" || r.row.attempt !== r.attempts.length)) delivery = "uncertain";
  else if (attempts.some(a => a.state === "queued") || relay.some(r => r.row.state === "queued")) delivery = "queued";
  else if (attempts.some(a => a.state === "sending") || relay.some(r => r.row.state === "sending")) delivery = "pending";
  else if (relay.length || action.capture_mode !== "live" || action.disposition === "fallback" || action.disposition === "cancelled") delivery = "none-confirmed";
  else if (action.disposition === "pending" || action.disposition === "automatic") delivery = "not-attempted";
  const observation = action.capture_mode !== "live";
  const ordinaryContinuation = (observation || action.disposition === "fallback" || action.disposition === "cancelled") && delivery === "none-confirmed";
  const status = delivery === "confirmed" ? "already relayed" : ["uncertain", "unknown"].includes(delivery) ? "delivery uncertain" : ordinaryContinuation ? "manual handling required" : "pending";
  const words: PickFacts["owner_words"][number][] = pick.reason === null || pick.reason === "" ? [] : [{
    kind: pick.option_id === "other" || isOwnWordsForm(pick.option_id) ? "own-words" : pick.option_id === "answers" ? "answers" : "ordinary-reason", text: pick.reason,
  }];
  const notes = events.filter(e => e.type === "home-note-reserved").map(e => {
    const note = z.object({ note_id: z.string(), body: z.string() }).parse(e.detail);
    const prefix = "mk's note (Home, advisory; plain text):\n";
    const suffix = `\n\nhome-note: ${note.note_id}`;
    const framed = note.body.startsWith(prefix) && note.body.endsWith(suffix);
    const text = framed ? note.body.slice(prefix.length, -suffix.length) : note.body;
    words.push({ kind: "note", id: note.note_id, text });
    return { ...note, text, framing: framed ? "decoded" : "unknown" };
  });
  const comments = new Map<string, string>();
  let head = evaluation?.evidence?.head;
  for (const evidence of [...events.filter(e => e.type === "pick-evidence-observed").map(e => e.detail.evidence), evaluation?.evidence]) {
    if (evidence === undefined) continue;
    const ev = observedEvidence.parse(evidence);
    for (const w of ev.comments?.owner_words ?? []) comments.set(w.id, w.text);
    if (ev.head) head = ev.head;
  }
  for (const [id, text] of comments) words.push({ kind: "comment", id, text });
  const ruling = operations[0]!.rows[0]?.row;
  const failure = events.find(e => e.type === "pick-ruling-file-failure");
  const control = db.prepare("SELECT * FROM pick_handler_control WHERE id = 1").get() as PickHandlerControlRow | undefined;
  if (!control) return failed("handler control unavailable");
  const parsedControl = { ...control, enabled_cells: z.array(z.string()).parse(JSON.parse(control.enabled_cells_json)),
    owner_enablement: control.owner_enablement_json === null ? null : jsonObject(control.owner_enablement_json),
    reviewed_scope: control.reviewed_scope_json === null ? null : JSON.parse(control.reviewed_scope_json) as unknown };
  const targetSnapshot = events.find(e => e.type === "on-pick-targets")?.detail;
  const askerProject = targetSnapshot?.asker ? z.object({ project_id: z.string() }).parse(targetSnapshot.asker).project_id : null;
  const imageOp = `wake:${pick.decision_id}:${pick.pick_id}`;
  const images = s.imageCount(imageOp);
  const facts: PickFacts = {
    observation_only: observation, handler_status: status,
    card_key: (db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(action.task_id) as { card_key: string | null } | undefined)?.card_key ?? null,
    card_title: null, question: ask.question, pick_id: pick.pick_id, decision_id: pick.decision_id, generation: action.generation,
    selected_option: pick.option_id, recommended_option: ask.recommendation || null, declaration, evaluation,
    target: action.target_thread === null || action.target_title === null || action.target_project_id === null || action.target_project_name === null ? null : { thread_id: action.target_thread, title: action.target_title, project_id: action.target_project_id, project_name: action.target_project_name },
    asker_project_id: askerProject, head: declaration?.subject.kind === "github-pr" ? { filed: declaration.subject.head, current: head?.head ?? null, state: head?.state ?? null } : null,
    ruling_id: ledgerId(pick.pick_id), ledger: s.ledger(pick.decision_id),
    ruling_file: { state: ruling?.state === "done" ? "written" : ruling?.attempt ? "failed" : ruling?.state === "pending" ? "pending" : "unknown", first_failure_at: failure?.at ?? null, failed_before: !!failure || (ruling?.attempt ?? 0) > 0 },
    deadline: { at: action.evaluation_deadline_at, outcome: action.evaluated_at && Date.parse(action.evaluated_at) < Date.parse(action.evaluation_deadline_at) ? "met" : Date.parse(s.now()) >= Date.parse(action.evaluation_deadline_at) ? "expired" : "pending" },
    delivery, host_message_id: accepted?.evidence ?? null, owner_words: words, images: images > 0 ? { count: images, op: imageOp } : null,
  };
  return { ok: true as const, kind: "action" as const, facts: pickFactsJson(facts), text: pickFactSheet(facts).text,
    snapshot: action, pick, ask, notes, target_evidence: targetSnapshot ?? null,
    blocks: db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(action.blocks_decision_id),
    observation_only: observation, ordinary_continuation: ordinaryContinuation, handler_status: status,
    current_control: parsedControl, configured_mode: s.mode, live_delivery_available: LIVE_DELIVERY_AVAILABLE,
    policy_version: PICK_POLICY_VERSION, live_unavailable_reason: "mk-6re1k and live enablement prerequisites remain outstanding",
    operations, events, event_history_complete: true, host_history_complete: false,
    image_contract: { op: imageOp, count: images > 0 ? images : null, absence_proves_no_images: false, transport: "ordinary-wake-images" },
    mode_evidence: { capture: { mode: action.capture_mode, epoch: action.capture_epoch }, current: { mode: control.mode, epoch: control.epoch, enabled_cells: parsedControl.enabled_cells },
      send: null, send_evidence: "consult recorded attempts/events; current control is not historical send evidence" },
    ruling_first_failure: { observed: !!failure || (ruling?.attempt ?? 0) > 0, at: failure?.at ?? null },
    optional_metadata_unavailable: ["card_title"],
  };
}
