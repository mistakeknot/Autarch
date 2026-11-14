// mk-cqnab.1 PR3 stage 1: the pick handler's evidence and pure evaluation. Nothing here writes, sends or
// schedules. An evaluation is an observation; a later stage owns capture, outbox rows and delivery.
import { z } from "zod";
import { authorClass, type ClassifyContext } from "./conversation.js";
import type { Ask, AskOption } from "./model.js";
import { ON_PICK_DENY, type OnPick } from "./onpick.js";
import { isOwnWordsForm } from "./ownwords.js";
import { readStoredAsk } from "./askbody.js";
import { commentBody, pickWrites } from "./cardwrites.js";
import { readSubjectHead, type HeadEvidence, type HeadReader } from "./pickhead.js";
import { ownerCommentCutoff, type TargetEvidence } from "./picktarget.js";
import type { PickActionRow, PickEvidenceSnapshot, Store } from "./store.js";
import { HOME_NOTE_TRAILER } from "./tasks.js";

export interface OwnerCommentInput {
  generation: number;
  previousObservedAt: string | null;
  comments: unknown;
  complete: boolean;
  observedAt: string;
  context: ClassifyContext;
  /** Every card-write id Home may have posted on this task (all generations): complete marker lines only. */
  expectedWriteIds: readonly string[];
  /** Every handler-status operation id Home may have posted on this task. */
  expectedStatusIds: readonly string[];
  /** Exact rendered persisted payloads. A marker alone is not proof of Home authorship. */
  expectedHomeBodies?: readonly string[];
  noteFence: "absent" | "pending" | "failed" | "recorded" | "unknown";
}
export interface OwnerCommentEvidence {
  result: "pass" | "fail" | "unknown";
  reason: string;
  observed_at: string;
  cutoff: string | null;
  comments: { id: string; created_at: string }[];
  owner_words?: { id: string; text: string }[];
  limitations: readonly string[];
}

// Validate at the evidence boundary too: malformed/incomplete reads cannot prove absence.
const evidenceComment = z.object({
  id: z.string().min(1), body: z.string(), createdAt: z.string().refine(v => Number.isFinite(Date.parse(v))),
  kind: z.enum(["user", "agent", "system"]), threadId: z.string().nullable(),
});
export const COMMENT_LIMITATIONS = ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"] as const;
/** The complete status-comment marker line for a handler status operation. Distinct from the card-write marker. */
export const PICK_STATUS_MARKER = "home-pick-status:";

export type PickHandlerStatus = "pending" | "already relayed" | "manual handling required" | "delivery uncertain";

/** Read-only projection; status_op remains a reserved identity, never an outbox operation. */
export function readPickHandlerStatus(store: Store, decisionId: string): PickHandlerStatus | null {
  try {
    return store.atomically(() => {
      const action = store.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(decisionId) as PickActionRow | undefined;
      if (!action) return null;
      const work = store.obligationsFor(decisionId);
      // Read all three operation histories in one snapshot. A receipt or fallback is informational:
      // neither its success nor its failure establishes whether the target accepted the relay.
      const evidence = [action.relay_op, action.fallback_op, action.receipt_op].map(op =>
        work.filter(o => op !== null && o.op === op).map(row => ({ row, attempts: store.attempts(row.id) })));
      const relays = evidence[0];
      if (!action.relay_op || relays.length > 1) return "delivery uncertain";
      const relay = relays[0];
      if (relay) {
        if (relay.row.kind !== "pick-relay" || relay.row.recipient !== action.target_thread) return "delivery uncertain";
        const accepted = relay.attempts.some(a => a.state === "delivered" && typeof a.evidence === "string"
          && a.evidence.trim().length > 0 && a.evidence !== "sent" && a.evidence !== a.handle);
        if (accepted) return "already relayed";
        if (relay.attempts.some(a => a.state !== "not-delivered")
          || relay.row.attempt !== relay.attempts.length
          || ["done", "sending", "queued", "uncertain"].includes(relay.row.state)) return "delivery uncertain";
      }
      if (action.disposition === "fallback" || action.disposition === "cancelled") return "manual handling required";
      if (action.disposition === "pending" || action.disposition === "automatic") return "pending";
      return "delivery uncertain";
    });
  } catch {
    return "delivery uncertain";
  }
}

const STATUS_TEXTS = ["pending", "already relayed", "manual handling required", "delivery uncertain"] as const;

/** Reader contract for future status writers. PR3 never persists or posts these bodies. */
function knownStatusBody(body: string, ids: readonly string[]): boolean {
  return ids.some(id => body === `${PICK_STATUS_MARKER} ${id}`
    || STATUS_TEXTS.some(status => body === `Pick handler: ${status}\n\n${PICK_STATUS_MARKER} ${id}`));
}

export function evaluateOwnerComments(input: OwnerCommentInput): OwnerCommentEvidence {
  let cutoff: string | null = null;
  const comments: OwnerCommentEvidence["comments"] = [];
  const ownerWords: { id: string; text: string }[] = [];
  const fenced = ["pending", "failed", "recorded"].includes(input.noteFence);
  const result = (value: OwnerCommentEvidence["result"], reason: string): OwnerCommentEvidence => ({
    result: fenced ? "fail" : value, reason: fenced ? "home-note-fence" : reason,
    observed_at: input.observedAt, cutoff, comments, owner_words: ownerWords, limitations: COMMENT_LIMITATIONS,
  });
  // A note's posting state never clears its permanent generation fence.
  if (!fenced && input.noteFence !== "absent") return result("unknown", "note-fence-unavailable");
  try { cutoff = ownerCommentCutoff(input.generation, input.previousObservedAt); }
  catch { return result("unknown", "comment-cutoff-unavailable"); }
  if (!input.complete || !Number.isFinite(Date.parse(input.observedAt))) return result("unknown", "comments-unavailable");
  const parsed = z.array(evidenceComment).safeParse(input.comments);
  if (!parsed.success) return result("unknown", "comments-unavailable");
  const writes = new Set(input.expectedWriteIds.map(id => `home-write: ${id}`));
  const statuses = new Set(input.expectedStatusIds.map(id => `${PICK_STATUS_MARKER} ${id}`));
  for (const c of parsed.data) {
    comments.push({ id: c.id, created_at: c.createdAt });
    if (authorClass({ kind: c.kind, author_id: c.threadId }, input.context) !== "mk") continue;
    if (cutoff !== null && Date.parse(c.createdAt) <= Date.parse(cutoff)) continue;
    const lines = c.body.split(/\r?\n/);
    const homeWrite = lines.some(l => writes.has(l));
    const homeStatus = lines.some(l => statuses.has(l));
    if (!HOME_NOTE_TRAILER.test(c.body) && ((homeWrite && input.expectedHomeBodies?.includes(c.body))
      || (homeStatus && knownStatusBody(c.body, input.expectedStatusIds)))) continue;
    ownerWords.push({ id: c.id, text: c.body });
  }
  ownerWords.sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  return result(ownerWords.length ? "fail" : "pass", ownerWords.length ? "owner-comment" : "no-owner-comments");
}

// ---- Four checks -------------------------------------------------------------------------------

/** Bumped whenever a rule below changes meaning; an evaluation under another version is excluded. */
export const PICK_POLICY_VERSION = 1;
/** PR3 ships no live path. Stage/PR5 wiring flips this only with the reviewed owner-decision control. */
export const LIVE_DELIVERY_AVAILABLE = false as const;

/** Stable reason codes. Renaming one is a policy-version change. */
export const PICK_REASON_CODES = [
  // recommendation
  "recommended", "no-recommendation", "recommendation-not-an-option", "recommendation-malformed", "not-recommended", "revision-mismatch", "virtual-option", "ask-unreadable",
  // no extra words or conditions
  "no-extra-words", "ordinary-reason", "own-words", "answers", "images", "images-unavailable", "no-unconditional-declaration",
  "no-owner-comments", "owner-comment", "home-note-fence", "note-fence-unavailable", "comment-cutoff-unavailable", "comments-unavailable",
  // reversible
  "declared-reversible", "declared-irreversible", "no-declaration", "option-kind-excluded", "approval-option", "root-run-card", "move-card",
  "card-exclusions-unavailable", "privileged-text", "relay-text-missing",
  // subject head
  "head-unchanged", "head-changed", "pr-not-open", "subject-none", "subject-invalid", "head-evidence-missing", "head-evidence-mismatch",
  "head-unavailable", "head-timeout", "head-malformed", "evaluation-timeout", "none-contradicted-by-artifact", "artifact-evidence-unavailable",
  // supplementary exclusions
  "not-card-source", "not-owner-pick", "generation-withdrawn", "generation-superseded", "generation-resolved", "generation-unavailable",
  "request-unregistered", "request-registration-unavailable", "binding-unconfirmed", "binding-unavailable", "no-asking-thread",
  "target-outside-scope", "target-project-changed", "target-not-live", "target-unavailable", "policy-version-mismatch",
  // activation (never part of the proposal)
  "handler-off", "shadow-observation-only", "proposal-fallback", "mode-epoch-changed", "owner-enablement-missing", "cell-not-enabled", "live-unavailable-in-this-build",
] as const;
export type PickReason = (typeof PICK_REASON_CODES)[number];
const REASONS: ReadonlySet<string> = new Set(PICK_REASON_CODES);

export type CheckResult = "pass" | "fail" | "unknown" | "not-applicable";
export interface CheckOutcome { result: CheckResult; reasons: string[] }
export type PickCheckName = "recommendation" | "no-extra-words" | "reversible" | "subject-head";
export type PickCell = "instruction:github-pr" | "instruction:none" | "ruling-only:github-pr" | "ruling-only:none";
export type HandlerMode = "off" | "shadow" | "live";
export interface TargetCheck { result: "pass" | "fail" | "unknown"; reason: string; observed_at: string | null }

/** Immutable snapshots and trusted context only. Nothing here is caller-supplied authority. */
export interface PickEvaluationInput {
  policy_version: number;
  evaluated_at: string;
  decision: {
    id: string;
    source: string;
    task_id: string | null;
    generation: number | null;
    revision: string;
    state: "open" | "withdrawn" | "superseded" | "resolved" | "unknown";
    request_registered: boolean | null;
    binding: "confirmed" | "unconfirmed" | "unknown";
    asker: string;
    /** The stored generation body read through the source-aware stored-card parser; null when unreadable. */
    ask: Ask | null;
    /** Card-level structural exclusions; null is unavailable evidence. */
    exclusions: { root_run: boolean | null; move: boolean | null; artifact: boolean | null };
  };
  pick: { pick_id: string; option_id: string; revision: string; by: string; reason: string | null; image_count: number | null; picked_at: string };
  comments: OwnerCommentEvidence;
  head: HeadEvidence | null;
  target: TargetCheck;
  control: { mode: HandlerMode; epoch: number; capture_epoch: number | null; enabled_cells: readonly string[]; owner_enablement: boolean };
}
export interface PickEvaluation {
  policy_version: number;
  decision_id: string;
  pick_id: string;
  generation: number | null;
  selected_option: string;
  recommended_option: string | null;
  checks: Record<PickCheckName, CheckOutcome>;
  /** Supplementary exclusions: any one makes the proposal a fallback. */
  exclusions: { code: string }[];
  cell: PickCell | null;
  /** The proposal: what the four checks and exclusions say. Never delivery ownership by itself. */
  outcome: "automatic" | "fallback";
  /** Whether this build would let the proposal own delivery. Always false in PR3. */
  activation: { mode: HandlerMode; permitted: false; reason: string };
  evidence_times: { evaluated_at: string; comments_observed_at: string | null; head_observed_at: string | null; target_observed_at: string | null };
  limitations: readonly string[];
  /** Lossless observed facts, not authority; absent on older evaluations. */
  evidence?: PickObservedEvidence;
}

export interface PickObservedEvidence {
  comments?: OwnerCommentEvidence;
  head?: HeadEvidence | null;
}

/** The fixture-shared trim set: ASCII space, tab, CR and LF only. Every other character is a word. */
export const REASON_TRIM = /^[ \t\r\n]*$/;
const VIRTUAL_IDS = new Set(["other", "answers"]);
const DASHES = /[\u2010-\u2015\u2212\ufe58\ufe63\uff0d]/g;
const DENY = ON_PICK_DENY.map(p => new RegExp(p, "i"));

const outcome = (fails: string[], unknowns: string[], passReason: string): CheckOutcome =>
  fails.length ? { result: "fail", reasons: [...fails, ...unknowns] } : unknowns.length ? { result: "unknown", reasons: unknowns } : { result: "pass", reasons: [passReason] };

const CELLS: readonly PickCell[] = ["instruction:github-pr", "instruction:none", "ruling-only:github-pr", "ruling-only:none"];
function cellOf(kind: string, subject: string): PickCell | null { return CELLS.find(c => c === `${kind}:${subject}`) ?? null; }
function relayKind(o: AskOption): string { return o.kind || (o.instruction ? "instruction" : "ruling-only"); }

function checkRecommendation(i: PickEvaluationInput, ask: Ask, opt: AskOption | undefined, virtual: boolean): CheckOutcome {
  const rec: unknown = ask.recommendation;
  if (rec !== undefined && typeof rec !== "string") return { result: "unknown", reasons: ["recommendation-malformed"] };
  if (i.pick.revision !== i.decision.revision) return { result: "fail", reasons: ["revision-mismatch"] };
  if (virtual) return { result: "fail", reasons: ["virtual-option"] };
  if (!rec) return { result: "fail", reasons: ["no-recommendation"] };
  if (!ask.options?.some(o => o.id === rec)) return { result: "fail", reasons: ["recommendation-not-an-option"] };
  return opt && opt.id === rec ? { result: "pass", reasons: ["recommended"] } : { result: "fail", reasons: ["not-recommended"] };
}

function checkWords(i: PickEvaluationInput, opt: AskOption | undefined, virtual: boolean): CheckOutcome {
  const fails: string[] = [];
  const unknowns: string[] = [];
  if (virtual) fails.push(i.pick.option_id === "answers" ? "answers" : "own-words");
  else if (i.pick.reason !== null && !REASON_TRIM.test(i.pick.reason)) fails.push("ordinary-reason");
  if (i.pick.image_count === null || !Number.isSafeInteger(i.pick.image_count) || i.pick.image_count < 0) unknowns.push("images-unavailable");
  else if (i.pick.image_count > 0) fails.push("images");
  if (!virtual && !opt?.on_pick) fails.push("no-unconditional-declaration");
  if (i.comments.result === "fail") fails.push(REASONS.has(i.comments.reason) ? i.comments.reason : "owner-comment");
  else if (i.comments.result !== "pass") unknowns.push(REASONS.has(i.comments.reason) ? i.comments.reason : "comments-unavailable");
  return outcome(fails, unknowns, "no-extra-words");
}

function checkReversible(i: PickEvaluationInput, opt: AskOption | undefined): CheckOutcome {
  if (!opt?.on_pick) return { result: "fail", reasons: ["no-declaration"] };
  const fails: string[] = [];
  const unknowns: string[] = [];
  const kind = relayKind(opt);
  if (kind !== "instruction" && kind !== "ruling-only") fails.push("option-kind-excluded");
  if (Object.hasOwn(opt, "approval")) fails.push("approval-option");
  const { root_run, move } = i.decision.exclusions;
  if (root_run === true) fails.push("root-run-card");
  if (move === true) fails.push("move-card");
  if (root_run === null || move === null) unknowns.push("card-exclusions-unavailable");
  const relay = kind === "instruction" ? opt.instruction : opt.on_pick.notification;
  if (typeof relay !== "string" || relay === "") fails.push("relay-text-missing");
  // Screen an NFKC, dash-folded copy; the filed payload is never rewritten.
  const screened = [opt.label, opt.instruction ?? "", opt.on_pick.notification ?? ""].join("\n").normalize("NFKC").replace(DASHES, "-");
  if (DENY.some(re => re.test(screened))) fails.push("privileged-text");
  if (opt.on_pick.reversible !== true) fails.push("declared-irreversible");
  return outcome(fails, unknowns, "declared-reversible");
}

function checkSubject(i: PickEvaluationInput, opt: AskOption | undefined): CheckOutcome {
  const subject: OnPick["subject"] | undefined = opt?.on_pick?.subject;
  if (!subject) return { result: "unknown", reasons: ["no-declaration"] };
  const h = i.head;
  if (subject.kind === "none") {
    if (Object.keys(subject).length !== 1) return { result: "unknown", reasons: ["subject-invalid"] };
    if (i.decision.exclusions.artifact === true) return { result: "fail", reasons: ["none-contradicted-by-artifact"] };
    if (i.decision.exclusions.artifact === null) return { result: "unknown", reasons: ["artifact-evidence-unavailable"] };
    if (!h) return { result: "unknown", reasons: ["head-evidence-missing"] };
    return h.result === "not-applicable" ? { result: "not-applicable", reasons: ["subject-none"] } : { result: "unknown", reasons: ["head-evidence-mismatch"] };
  }
  if (!h) return { result: "unknown", reasons: ["head-evidence-missing"] };
  if (h.result === "not-applicable") return { result: "unknown", reasons: ["head-evidence-mismatch"] };
  if (h.result === "pass") return h.head === subject.head && h.state === "OPEN" ? { result: "pass", reasons: ["head-unchanged"] } : { result: "unknown", reasons: ["head-evidence-mismatch"] };
  const reason = REASONS.has(h.reason) ? h.reason : "head-unavailable";
  return h.result === "fail" ? { result: "fail", reasons: [reason] } : { result: "unknown", reasons: [reason] };
}

function exclusionsOf(i: PickEvaluationInput): { code: string }[] {
  const out: string[] = [];
  const d = i.decision;
  if (i.policy_version !== PICK_POLICY_VERSION) out.push("policy-version-mismatch");
  if (d.source !== "card") out.push("not-card-source");
  // Only a new owner pick can enter live handling; vizier rulings and evidence auto-closes cannot.
  if (i.pick.by !== "mk") out.push("not-owner-pick");
  if (d.state === "withdrawn") out.push("generation-withdrawn");
  else if (d.state === "superseded") out.push("generation-superseded");
  else if (d.state === "resolved") out.push("generation-resolved");
  else if (d.state !== "open" || d.generation === null || !Number.isSafeInteger(d.generation) || d.generation < 1) out.push("generation-unavailable");
  if (d.request_registered === false) out.push("request-unregistered");
  else if (d.request_registered !== true) out.push("request-registration-unavailable");
  if (d.binding === "unconfirmed") out.push("binding-unconfirmed");
  else if (d.binding !== "confirmed") out.push("binding-unavailable");
  if (!d.asker) out.push("no-asking-thread");
  if (i.target.result !== "pass") {
    const known = ["target-outside-scope", "target-project-changed", "target-not-live"];
    out.push(i.target.result === "fail" && known.includes(i.target.reason) ? i.target.reason : "target-unavailable");
  }
  return out.map(code => ({ code }));
}

function activationOf(i: PickEvaluationInput, proposal: "automatic" | "fallback", cell: PickCell | null): PickEvaluation["activation"] {
  const mode = i.control.mode;
  const deny = (reason: PickReason) => ({ mode, permitted: false as const, reason });
  if (mode === "off") return deny("handler-off");
  if (mode !== "live") return deny("shadow-observation-only");
  if (proposal !== "automatic" || cell === null) return deny("proposal-fallback");
  if (i.control.capture_epoch === null || i.control.capture_epoch !== i.control.epoch) return deny("mode-epoch-changed");
  if (!i.control.owner_enablement) return deny("owner-enablement-missing");
  if (!i.control.enabled_cells.includes(cell)) return deny("cell-not-enabled");
  // Every guard passes, yet this build has no live path.
  return deny("live-unavailable-in-this-build");
}

/** Pure: same input, same output; no reads, writes, clocks or randomness. */
export function evaluatePick(input: PickEvaluationInput): PickEvaluation {
  const ask = input.decision.ask;
  const optionId = input.pick.option_id;
  const real = ask?.options?.find(o => o.id === optionId);
  const virtual = !real && (VIRTUAL_IDS.has(optionId) || isOwnWordsForm(optionId));
  const unknown = (reason: string): CheckOutcome => ({ result: "unknown", reasons: [reason] });
  const checks: Record<PickCheckName, CheckOutcome> = ask
    ? {
        recommendation: checkRecommendation(input, ask, real, virtual),
        "no-extra-words": checkWords(input, real, virtual),
        reversible: virtual ? { result: "fail", reasons: ["no-declaration"] } : checkReversible(input, real),
        "subject-head": checkSubject(input, real),
      }
    : { recommendation: unknown("ask-unreadable"), "no-extra-words": unknown("ask-unreadable"), reversible: unknown("ask-unreadable"), "subject-head": unknown("ask-unreadable") };
  const exclusions = exclusionsOf(input);
  const declared = real?.on_pick;
  const cell = declared ? cellOf(relayKind(real), declared.subject.kind) : null;
  const eligible = checks.recommendation.result === "pass" && checks["no-extra-words"].result === "pass" && checks.reversible.result === "pass"
    && (checks["subject-head"].result === "pass" || checks["subject-head"].result === "not-applicable") && exclusions.length === 0 && cell !== null;
  const proposal = eligible ? "automatic" : "fallback";
  const rec = typeof ask?.recommendation === "string" && ask.recommendation !== "" ? ask.recommendation : null;
  return {
    policy_version: PICK_POLICY_VERSION,
    decision_id: input.decision.id,
    pick_id: input.pick.pick_id,
    generation: input.decision.generation,
    selected_option: optionId,
    recommended_option: rec,
    checks,
    exclusions,
    cell,
    outcome: proposal,
    activation: activationOf(input, proposal, cell),
    evidence_times: {
      evaluated_at: input.evaluated_at,
      comments_observed_at: input.comments.observed_at ?? null,
      head_observed_at: input.head?.observed_at ?? null,
      target_observed_at: input.target.observed_at,
    },
    limitations: input.comments.limitations ?? COMMENT_LIMITATIONS,
    evidence: structuredClone({ comments: input.comments, head: input.head }),
  };
}

// ---- Runtime evidence ------------------------------------------------------------------------------
// Read-only collection from persisted state, Tasks comments, host target metadata and the bounded head
// reader. The caller cannot supply authority: every fact comes from the Store snapshot or a fresh read.

export const EVALUATION_WINDOW_MS = 120_000;
/** Absolute deadline basis. A later stage persists it as pick_actions.evaluation_deadline_at at capture. */
export function evaluationDeadline(capturedAt: string): string | null {
  const t = Date.parse(capturedAt);
  return Number.isFinite(t) ? new Date(t + EVALUATION_WINDOW_MS).toISOString() : null;
}

/** Stable per-pick operation keys with distinct suffixes; stage 2 stores them in pick_actions. */
export function pickActionOps(decisionId: string, pickId: string) {
  const base = `pick-action:${decisionId}:${pickId}`;
  return { ruling: `${base}:ruling`, relay: `${base}:relay`, fallback: `${base}:fallback`, receipt: `${base}:receipt`, status: `${base}:status` };
}

export interface PickEvidenceSources {
  store: Pick<Store, "pickEvidenceSnapshot">;
  /** The existing TasksClient.listComments contract; a throw is unavailable evidence. */
  listComments: (taskId: string) => Promise<unknown>;
  readTargetEvidence?: (thread: string) => Promise<TargetEvidence>;
  /** Injected for tests; production uses the audited ghview head reader through readSubjectHead. */
  readHead?: HeadReader;
  /** Count of images accepted with the pick under the ordinary wake op; null when unknown. */
  imageCount?: (op: string) => number | null;
  now: () => string;
}
export interface PickEvidenceOptions {
  /** Detached observations as reads finish. Persistence, if desired, belongs to the caller. */
  observed?: (evidence: PickObservedEvidence) => void;
  /** Persisted absolute deadline (stage 2: pick_actions.evaluation_deadline_at). Default: pick time + 120 s. */
  deadlineAt?: string;
  /** Capture epoch of the action record; null before capture exists. */
  captureEpoch?: number | null;
}

type FrozenTargets = { asker?: TargetEvidence; targets?: Record<string, TargetEvidence> };

/** Bound a read to the original allowance. Tasks/host reads have no cancellation API:
 * an underlying read-only request may finish later, but its result is ignored. */
export async function readBeforeDeadline<T>(read: () => Promise<T>, deadlineAt: string, now: () => string): Promise<T> {
  const remaining = Date.parse(deadlineAt) - Date.parse(now());
  if (!Number.isFinite(remaining) || remaining <= 0) throw new Error("evaluation-timeout");
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const timeout = new Promise<never>((_resolve, reject) => {
      timer = setTimeout(() => reject(new Error("evaluation-timeout")), remaining);
    });
    const result = await Promise.race([read(), timeout]);
    // Also reject a late result if the event loop delivers it before the overdue timer.
    if (!(Date.parse(now()) < Date.parse(deadlineAt))) throw new Error("evaluation-timeout");
    return result;
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}

async function targetCheck(src: PickEvidenceSources, snap: PickEvidenceSnapshot, optionId: string, decl: OnPick | undefined, deadlineAt: string): Promise<TargetCheck> {
  const unknown = (): TargetCheck => ({ result: "unknown", reason: "target-unavailable", observed_at: null });
  if (!decl || !snap.target_snapshot_json) return unknown();
  let frozen: TargetEvidence | undefined;
  try { frozen = (JSON.parse(snap.target_snapshot_json) as FrozenTargets).targets?.[optionId]; } catch { return unknown(); }
  if (!frozen || frozen.thread_id !== decl.target_thread) return unknown();
  const asker = snap.decision.thread;
  if (decl.target_thread !== asker && !snap.blocks.includes(`thread:${decl.target_thread}`)) return { result: "fail", reason: "target-outside-scope", observed_at: src.now() };
  if (!src.readTargetEvidence) return unknown();
  let current: TargetEvidence;
  const read = src.readTargetEvidence;
  try { current = await readBeforeDeadline(() => read(decl.target_thread), deadlineAt, src.now); } catch { return unknown(); }
  const at = current.observed_at && Number.isFinite(Date.parse(current.observed_at)) ? current.observed_at : src.now();
  if (current.thread_id !== decl.target_thread) return unknown();
  if (current.archived || current.deleted) return { result: "fail", reason: "target-not-live", observed_at: at };
  // N2: only the host thread/project relation counts; Tasks linkedBbProjectId plays no role.
  if (current.project_id !== frozen.project_id) return { result: "fail", reason: "target-project-changed", observed_at: at };
  if (current.title !== frozen.title || current.project_name !== frozen.project_name) return { result: "fail", reason: "target-presentation-changed", observed_at: at };
  return { result: "pass", reason: "target-live-in-scope", observed_at: at };
}

function fenceState(rows: { posting_state: string }[] | null): OwnerCommentInput["noteFence"] {
  if (rows === null) return "unknown";
  if (!rows.length) return "absent";
  for (const s of ["pending", "failed", "recorded"] as const) if (rows.some(r => r.posting_state === s)) return s;
  return "unknown";
}

function controlOf(snap: PickEvidenceSnapshot, captureEpoch: number | null): PickEvaluationInput["control"] {
  const c = snap.control;
  if (!c) return { mode: "off", epoch: 0, capture_epoch: captureEpoch, enabled_cells: [], owner_enablement: false };
  let cells: unknown = [];
  try { cells = JSON.parse(c.enabled_cells_json); } catch { cells = []; }
  const mode: HandlerMode = c.mode === "shadow" || c.mode === "live" ? c.mode : "off";
  return {
    mode, epoch: Number(c.epoch), capture_epoch: captureEpoch,
    enabled_cells: Array.isArray(cells) ? cells.filter((x): x is string => typeof x === "string") : [],
    owner_enablement: c.owner_enablement_json !== null,
  };
}

/** Collect a PickEvaluationInput for a ruled decision; null when the decision or its pick does not exist. */
export async function collectPickEvidence(src: PickEvidenceSources, decisionId: string, opts: PickEvidenceOptions = {}): Promise<PickEvaluationInput | null> {
  const snap = src.store.pickEvidenceSnapshot(decisionId);
  if (!snap || !snap.pick) return null;
  const d = snap.decision;
  const p = snap.pick;
  const ask = readStoredAsk(d.body_json, d.source === "card" ? "card" : "home");
  const option = ask?.options?.find(o => o.id === p.option_id);
  const decl = option?.on_pick;
  const participating = ask?.options?.some(o => o.on_pick) === true;
  const generation = typeof d.generation === "number" ? d.generation : null;
  const deadlineAt = opts.deadlineAt ?? evaluationDeadline(p.picked_at) ?? new Date(0).toISOString();

  // Comments: the existing listComments API, the persisted generation fences and Home's prior observation.
  let comments: unknown = null;
  let complete = false;
  if (d.task_id !== null) {
    const taskId = d.task_id;
    try { comments = await readBeforeDeadline(() => src.listComments(taskId), deadlineAt, src.now); complete = true; } catch { complete = false; }
  }
  const observedAt = src.now();
  const writeIds = snap.generations.flatMap(g => pickWrites({ id: g.decision_id, task_id: d.task_id ?? "" }, { option_id: "", option_label: "", by: "", generation: null, picked_at: "", reason: null }))
    .filter(w => w.kind === "comment").map(w => w.id);
  const statusIds = snap.generations.filter(g => g.pick_id !== null).map(g => pickActionOps(g.decision_id, g.pick_id!).status);
  const ownerComments = evaluateOwnerComments({
    generation: generation ?? 0,
    previousObservedAt: generation === 1 ? null : snap.previous?.filed_at ?? null,
    comments, complete, observedAt,
    context: { owner: d.owner_thread ?? d.thread ?? null, vizier: snap.vizier },
    expectedWriteIds: writeIds, expectedStatusIds: statusIds,
    expectedHomeBodies: snap.home_comments.flatMap(w => {
      try { return [commentBody(w.id, JSON.parse(w.payload ?? "{}"))]; } catch { return []; }
    }),
    noteFence: fenceState(snap.note_fences),
  });
  opts.observed?.(structuredClone({ comments: ownerComments }));

  const head = decl ? await readSubjectHead(decl.subject, { deadlineAt, now: () => Date.parse(src.now()), ...(src.readHead ? { read: src.readHead } : {}) }) : null;
  opts.observed?.(structuredClone({ comments: ownerComments, head }));
  const target = await targetCheck(src, snap, p.option_id, decl, deadlineAt);

  let imageCount: number | null;
  try { imageCount = src.imageCount ? src.imageCount(`wake:${d.id}:${p.pick_id}`) : 0; } catch { imageCount = null; }
  const binding = snap.binding === null ? "unconfirmed" : snap.binding.state === "confirmed" && snap.binding.home_project === d.project ? "confirmed" : "unconfirmed";
  const noneSelected = decl?.subject.kind === "none";
  return {
    policy_version: PICK_POLICY_VERSION,
    evaluated_at: src.now(),
    decision: {
      id: d.id, source: d.source, task_id: d.task_id, generation, revision: d.revision,
      state: d.withdrawn_at ? "withdrawn" : d.superseded_by ? "superseded" : d.resolved_at ? "resolved" : "open",
      request_registered: snap.request_registered, binding, asker: d.thread ?? "", ask,
      // A stored declaration exists only because card ingest refused it alongside root-run, home-move and
      // (for `none`) artifact evidence. Without a declaration that proof is absent: unavailable, not false.
      exclusions: {
        root_run: participating ? false : null,
        move: snap.move ? true : participating ? false : null,
        artifact: noneSelected ? false : null,
      },
    },
    pick: { pick_id: p.pick_id, option_id: p.option_id, revision: p.revision, by: p.by, reason: p.reason, image_count: imageCount, picked_at: p.picked_at },
    comments: ownerComments,
    head,
    target,
    control: controlOf(snap, opts.captureEpoch ?? null),
  };
}

export async function evaluatePickRuntime(src: PickEvidenceSources, decisionId: string, opts: PickEvidenceOptions = {}): Promise<{ input: PickEvaluationInput; evaluation: PickEvaluation } | null> {
  const input = await collectPickEvidence(src, decisionId, opts);
  return input ? { input, evaluation: evaluatePick(input) } : null;
}
