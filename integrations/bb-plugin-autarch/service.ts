// The Home decision service: file, pick once, reconcile the ruling file, wake and feed.
// Every state change goes through one Store transaction; this layer maps outcomes to
// HTTP-shaped results and never holds state of its own.
import { isOwnWordsForm } from "./ownwords.js";
import { ImageStore, textSha, validateImages, type ImageIn, type PathsResult } from "./otherimages.js";
import { createHash, randomUUID } from "node:crypto";
import { buildFeed, renderFeed, type Feed } from "./feed.js";
import { cardFingerprint, askingThread, parseCard, toV1, type Card } from "./cards.js";
import { PUSH_TIMEOUT_MS, type Notifier, type PushRequest } from "./notifier.js";
import { blockersByThread, askRows, listPushes, timeToAnswer, type PushListing, type TimeToAnswer } from "./pushtiming.js";
import { checkHoldEnd, endMs, holdEndPhrase, parseStoredEnd, type HoldEnd } from "./holdend.js";
import { OPEN_STATUSES, type Task, type TaskCommentRow, type TasksClient } from "./tasks.js";

/** A hold copied all the way down, so a caller that edits what it got cannot change the cached record. */
const copyHold = (h: Hold): Hold => ({ ...h, ...(h.end ? { end: "event" in h.end ? { event: { ...h.end.event } } : { ...h.end } } : {}) });
const NEEDS_END = "a hold needs its end: pass --until DATE (for example 2026-11-14), or an event with --event-kind, --event-ref and --event-label";
/** A vizier hold on a card; n tells one hold from a replacement made in the same millisecond. A new hold names its end
 *  (bead mk-j5ipa.11); an old one has none and is never released by time. */
export type Hold = { reason: string; by: string; at: string; n?: string; end?: HoldEnd };
/** mk set the card aside to do after the fast ones (bead mk-8741). The card stays open and unruled. */
export type Later = { at: string; by: string };
import { readAsk, UNREADABLE_REFUSAL as UNREADABLE_PICK } from "./askbody.js";
import { identity, normalizedJson, parseAsk, parseExistingAsk, revision, semanticKey, type Ask, type Question } from "./model.js";
import { pickWrites } from "./cardwrites.js";

// Canonical JSON stays unchanged; delivery JSON also carries delivery and display attributes.
function deliveryJson(ask: Ask): string {
  if (!ask.evidence?.length && !ask.group && !ask.stopped) return normalizedJson(ask);
  const body = JSON.parse(normalizedJson(ask));
  if (ask.stopped) {
    body.urgency = ask.urgency;
    body.stopped = ask.stopped;
  }
  if (ask.evidence?.length) body.evidence = ask.evidence;
  if (ask.group) body.group = ask.group;
  return JSON.stringify(body);
}
import { estateRoot, fsUuidOfDev, pinRoot, renderRuling, rulingPath, writeRuling, type PinnedRoot, type Ruling } from "./ruling.js";
import { appendRuling } from "./ledger.js";
import { canonicalOnPick } from "./model.js";
import { TargetEvidenceError, validatePickTarget, type TargetEvidence } from "./picktarget.js";
import { evaluatePickRuntime, type PickEvidenceOptions } from "./pickhandler.js";
import { capturePick, pickFenceToken, productionPickMode, recordPickEvaluation } from "./pickcapture.js";
import type { HeadReader } from "./pickhead.js";
import type { ThreadRow } from "./delegation.js";
import type { DecisionInput, ObligationInput, ObligationRow, PickInput, PickRow, Store } from "./store.js";

export interface ProjectInfo {
  name: string;
  root: string;
  dev: number | bigint | string;
  ino: number | bigint | string;
}

export interface ServiceDeps {
  pickHandlerMode?: unknown;
  /** Isolated runtime injection only. Production wiring never supplies this dependency. */
  pickCaptureRuntime?: { mode: () => string };
  tasks?: () => TasksClient;
  readTargetEvidence?: (thread: string) => Promise<TargetEvidence>;
  store: Store;
  projects: () => Promise<ProjectInfo[]>;
  now?: () => string;
  newId?: () => string;
  env?: Record<string, string | undefined>;
  /** Called after a pick commits, so a delivery loop can run without waiting for its timer. */
  nudge?: () => void;
  /** The vizier's rulings.jsonl. Each ruled pick appends one line there (idempotent on the pick id); omitted, none is written. */
  rulingsLedger?: string | null;
  /** Where the Other box's images are kept. Omitted, a request that carries images is refused. */
  images?: ImageStore;
  /** Sends the phone push for a blocker ask (bead mk-tv70n.7). Omitted, no push is made. */
  notifier?: Notifier;
  /** How long a push may take before it is given up on (default 10 s). The filing never waits for it. */
  pushTimeoutMs?: number;
  /** Where a failed push is logged (default console.error). */
  log?: (msg: string) => void;
  /** Who is told about an undelivered card whose asker is unknown (bead mk-mn7yz). Omitted, no one is. Delegation sets it when it is given a thread lister. */
  threadSource?: NoticeThreads;
}

/** The threads a notice about an undelivered card with no known asker can go to. */
export interface NoticeThreads {
  /** Every thread, fresh, archived and deleted ones included. May reject. */
  list: () => Promise<ThreadRow[]>;
  /** The live vizier thread, or null when none can be confirmed. */
  vizier: () => Promise<string | null>;
}


export type FileResult =
  | { ok: true; status: 200 | 201; decision_id: string; mentioned?: true; related?: string }
  | { ok: false; status: number; exit: 1 | 2 | 3; error: string };

export type PickResult =
  | { ok: true; status: 200 | 201; pick: PickRow }
  | { ok: false; status: number; error: string };

type Row = Record<string, unknown> & {
  id: string;
  request_id: string;
  kind: string;
  project: string;
  thread: string;
  subject: string;
  body_json: string;
  revision: string;
  root_dev: string | null;
  root_ino: string | null;
  root_fsid: string | null;
  project_root: string | null;
  supersedes: string | null;
  owner_thread: string | null;
  withdrawn_at: string | null;
  resolved_at: string | null;
};

const RECONCILE_MS = 30_000;
/** Failed blocker pushes retried per ask before giving up, and the first backoff (doubling, capped at an hour). */
const MAX_PUSH_ATTEMPTS = 6;
const PUSH_RETRY_BASE_MS = 30_000;
/** Thrown inside the push when the ask closed before the notifier was called; not a failure. */
class PushSkipped extends Error {}
/** Pushes one reconcile pass starts at most. */
const MAX_PUSHES_PER_PASS = 50;
/** Rows the retry query hands to JS in one pass (all already eligible by the database's reckoning). */
const PUSH_SCAN_ROWS = 200;
export const ROUTING_PREFIX = "routing: ";
const OBSERVED_BACKOFF_MS = 5_000;
const CLOSED_BACKOFF_MS = 30_000;
const MAX_BACKOFF_MS = 300_000;
const UNROUTABLE_AFTER_MS = 60_000;
const APPROVAL_DEFAULT_TTL = 24 * 3600;
const APPROVAL_MAX_TTL = 7 * 24 * 3600;
const fail = (status: number, error: string, exit: 1 | 2 | 3 = status >= 500 ? 3 : 1): FileResult => ({ ok: false, status, exit, error });
const bad = (error: string): FileResult => ({ ok: false, status: 400, exit: 2, error });
export const OTHER_OPTION_ID = "other";
/** True for either id a free-text answer is recorded under (the reserved form is in ownwords.ts, bead mk-1poq7). */
export const isOwnWordsId = (id: string): boolean => id === OTHER_OPTION_ID || isOwnWordsForm(id);
/** The virtual option of a multi-pick ask: one pick that answers every question. Never in the ask; mk only. */
export const ANSWERS_OPTION_ID = "answers";
export const OTHER_MAX = 20000;
/** True when the comment's last non-empty line is exactly the marker `home-note: <id>`, with no trimming beyond a trailing carriage return (so n1 never matches n10). */
export function hasNoteMarker(body: string, noteId: string): boolean {
  const lines = body.split("\n").map((l) => l.replace(/\r$/, "")).filter((l) => l !== ""); // literal: only a trailing CR is dropped
  return lines.length > 0 && lines[lines.length - 1] === `home-note: ${noteId}`;
}
/** mk's free text: trimmed, plain, 1..OTHER_MAX characters; null when empty or too long. */
/** A multi-pick answer: every question has exactly one of its options. The words read "id: label" per question, and mark a pick that differs from the recommendation. */
export function answersOf(
  questions: readonly Question[],
  answers: Readonly<Record<string, string>> | undefined,
  note: string | undefined,
): { ok: true; words: string; lines: string[]; json: string } | { ok: false; error: string } {
  if (answers === undefined || answers === null || typeof answers !== "object") return { ok: false, error: "answers are required: one option for every question" };
  const known = new Set(questions.map((q) => q.id));
  for (const k of Object.keys(answers)) if (!known.has(k)) return { ok: false, error: `unknown question ${JSON.stringify(k)}` };
  // Labels and the note are unverified text: one line each, so none can pose as another answer.
  const flat = (t: string) => t.replace(/\s+/g, " ").trim();
  const parts: string[] = [];
  const lines: string[] = [];
  const chosen: string[][] = [];
  for (const q of questions) {
    const pickId = answers[q.id];
    const o = q.options.find((x) => x.id === pickId);
    if (!o) return { ok: false, error: `question ${q.id} needs one of its options` };
    const rec = q.recommendation ? q.options.find((x) => x.id === q.recommendation) : undefined;
    const changed = rec !== undefined && rec.id !== o.id ? ` (changed; recommended was ${flat(rec.label)})` : "";
    parts.push(`${q.id}: ${flat(o.label)}${changed}`);
    lines.push(`- ${q.id}: ${flat(o.label)} [${o.id}]${changed}`);
    chosen.push([q.id, o.id]);
  }
  let words = parts.join("; ");
  const text = note === undefined || note.trim() === "" ? null : cleanOther(note);
  if (note !== undefined && note.trim() !== "" && text === null) return { ok: false, error: `a note of 1-${OTHER_MAX} characters is required` };
  if (text !== null) {
    words += `; note: ${flat(text)}`;
    lines.push(`Note: ${flat(text)}`);
  }
  return { ok: true, words, lines, json: JSON.stringify([chosen, text]) };
}

export interface PickParams {
  option_id: string;
  revision: string;
  by: string;
  reason: string | null;
  answer_form: "option" | "own-words" | "answers";
  answers: [string, string][] | null;
  image_digest: string | null;
}
/**
 * Version 2 of the pick-parameter digest (mk-cqnab.1). Canonical JSON framing of the exact reason as received,
 * option, revision, by, answer form, answer values and image digest. Version 1 digests (bare SHA-256 hex or
 * `img1:`) stay valid only together with the stored words; see `pickRetryMatches`.
 */
export function pickParamsDigest(p: PickParams): string {
  const answers = p.answers === null ? null : [...p.answers].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
  const frame = JSON.stringify(["home-pick-params", 2, p.option_id, p.revision, p.by, p.reason, p.answer_form, answers, p.image_digest]);
  return `p2:${createHash("sha256").update(frame, "utf8").digest("hex")}`;
}

/** A retry settles only on the same parameters. A legacy row must match its old digest and its stored words. */
export function pickRetryMatches(existing: Pick<PickRow, "params_hash" | "reason">, digest: string, legacyDigest: string, storedReason: string | null): boolean {
  const h = existing.params_hash;
  if (h === null) return false;
  if (h.startsWith("p2:")) return h === digest;
  return h === legacyDigest && existing.reason === storedReason;
}

export function cleanOther(s: string | undefined): string | null {
  const t = (s ?? "").trim();
  return t === "" || [...t].length > OTHER_MAX ? null : t;
}
const optionLabelOf = (o: { label: string }) => o.label;
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** One project's slice of `stats`. */
export interface ProjectStats {
  filed: number;
  filed_per_week: Record<string, number>;
  picks: number;
  picks_by: Record<string, number>;
  open: number;
  oldest_open_age_ms: number | null;
}

export function isoWeek(ms: number): string {
  const d = new Date(ms);
  const day = (d.getUTCDay() + 6) % 7; // Monday = 0
  const thursday = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() - day + 3);
  const year = new Date(thursday).getUTCFullYear();
  const week = Math.floor((thursday - Date.UTC(year, 0, 1)) / 86_400_000 / 7) + 1;
  return `${year}-W${String(week).padStart(2, "0")}`;
}

function median(sorted: number[]): number | null {
  if (sorted.length === 0) return null;
  const m = sorted.length >> 1;
  return sorted.length % 2 ? sorted[m]! : (sorted[m - 1]! + sorted[m]!) / 2;
}

/** The shared validator's message is a vector Go also checks; Home says what mk or the vizier can do about a missing Request line. */
function shownParseError(err: string): string {
  return err === "missing Request line" ? "Request line missing: re-file the ask or restore the Request line" : err;
}

export class Service {
  private readonly entryGuards = new WeakMap<NonNullable<PickInput["guard"]>, string>();
  readonly store: Store;
  private readonly deps: ServiceDeps;
  private timer: ReturnType<typeof setInterval> | null = null;
  /** Asks with a push in flight (by oldest row id), so two filings of one ask never push twice at once. */
  private pushing = new Set<string>();
  private pushPending = new Set<Promise<void>>();
  private threadSource: NoticeThreads | undefined;

  constructor(deps: ServiceDeps) {
    this.deps = deps;
    this.store = deps.store;
    this.threadSource = deps.threadSource;
    const mode = productionPickMode(deps.pickHandlerMode);
    this.store.atomically(() => {
      this.db.prepare("UPDATE pick_handler_control SET mode = ?, epoch = epoch + 1, updated_at = ? WHERE id = 1 AND mode <> ?").run(mode, this.now(), mode);
    });
  }

  /** Where a notice about an undelivered card with no known asker is sent (bead mk-mn7yz). Delegation sets this from the host's thread list. */
  useThreadSource(src: NoticeThreads): void {
    this.threadSource = src;
  }

  private now(): string {
    return (this.deps.now ?? (() => new Date().toISOString()))();
  }
  private newId(): string {
    return (this.deps.newId ?? (() => `dec_${randomUUID().replace(/-/g, "").slice(0, 16)}`))();
  }
  private get db() {
    return this.store.db;
  }
  private row(id: string): Row | undefined {
    return this.store.decision(id) as Row | undefined;
  }
  private askOf(d: Row): Ask {
    const ask = readAsk(d.body_json, {}, d.source);
    if (!ask) throw new Error("unreadable stored ask");
    return ask;
  }
  private labelOf(d: Row, optionId: string): string {
    return readAsk(d.body_json, {}, d.source)?.options?.find((o) => o.id === optionId)?.label ?? optionId;
  }
  private replacementOf(id: string): string | undefined {
    return (this.db.prepare("SELECT id FROM decisions WHERE supersedes = ?").get(id) as { id: string } | undefined)?.id;
  }

  // ---- filing ---------------------------------------------------------------

  /** The service clock, for the layers built on it. */
  time(): string {
    return this.now();
  }

  /** A fresh decision id from the service's source. */
  mintId(): string {
    return this.newId();
  }

  /** Let a delivery loop run without waiting for its timer. */
  nudge(): void {
    this.deps.nudge?.();
  }

  /** Who owns a machine blocker: the ask's own owner, else the machineOwners setting for its class. */
  machineOwner(ask: Ask): string | null {
    if (ask.kind !== "machine" || !ask.machine) return null;
    if (ask.machine.owner_thread) return ask.machine.owner_thread;
    try {
      const map = JSON.parse(this.store.setting("machineOwners") ?? "{}") as Record<string, unknown>;
      const t = map[ask.machine.class];
      return typeof t === "string" && t !== "" ? t : null;
    } catch {
      return null;
    }
  }

  /** The owner's one notice, built for the filing's own transaction. */
  private ownerNotice(decisionId: string, ask: Ask, owner: string | null): ObligationInput[] {
    if (ask.kind !== "machine" || !owner) return [];
    return [
      {
        id: `ob:${decisionId}:owner`,
        kind: "owner",
        recipient: owner,
        op: `owner:${decisionId}`,
        payload: [
          `Machine blocker ${decisionId} (${ask.machine?.class ?? ""}) is yours: ${ask.machine?.detail ?? ""}`,
          `Question: ${ask.question}`,
          `Before acting, confirm with \`bb home get --id ${decisionId}\` that this blocker is still open.`,
        ].join("\n"),
      },
    ];
  }

  file(req: unknown, ctx: { threadId?: string } = {}): Promise<FileResult> {
    return this.fileInner(req, ctx);
  }

  private async fileInner(req: unknown, ctx: { threadId?: string } = {}): Promise<FileResult> {
    let ask: Ask;
    try {
      ask = parseExistingAsk(req);
    } catch (e) {
      return bad(message(e));
    }
    if (ask.asker === "thread" && ctx.threadId !== undefined && ctx.threadId !== ask.thread) {
      return bad("the asking thread does not match the calling thread");
    }
    const id = identity(ask);
    const requestId = ask.request_id!;

    const known = this.store.registry(requestId);
    if (known) return this.replay(known, id);

    // A registered legacy request can replay, but every new filing needs current delivery metadata.
    try {
      ask = parseAsk(req);
    } catch (e) {
      return bad(message(e));
    }


    // Scope: the estate Uqbar (G-1), or a project that serve resolves.
    let root: PinnedRoot;
    try {
      if (ask.project === "estate") {
        const dir = estateRoot(this.deps.env ?? process.env);
        if (dir !== ask.project_root) return bad("project_root must be the Uqbar directory for an estate decision");
        root = pinRoot(dir);
      } else {
        let projects: ProjectInfo[];
        try {
          projects = await this.deps.projects();
        } catch {
          return fail(503, "not-filed: project resolution unavailable", 3);
        }
        const p = projects.find((x) => x.name === ask.project);
        if (!p) return bad(`unknown project ${JSON.stringify(ask.project)}`);
        if (p.root !== ask.project_root) return bad("project_root does not match the root serve resolves for this project");
        root = { path: p.root, dev: String(p.dev), ino: String(p.ino), fsid: fsUuidOfDev(String(p.dev)) };
      }
    } catch (e) {
      return bad(message(e));
    }

    const base: DecisionInput = {
      id: this.newId(),
      request_id: requestId,
      identity: id,
      revision: revision(ask),
      semantic_key: semanticKey(ask),
      subject: ask.subject ?? "",
      kind: ask.kind,
      ask_key: ask.ask_key,
      project: ask.project,
      project_root: root.path,
      root_dev: root.dev,
      root_ino: root.ino,
      root_fsid: root.fsid ?? null,
      asker: ask.asker,
      thread: ask.thread ?? "",
      owner_thread: this.machineOwner(ask),
      body_json: deliveryJson(ask),
      supersedes: ask.supersedes || null,
      delegable: true,
      urgency: ask.urgency ?? null,
    };

    if (ask.supersedes) {
      const r = this.replace(base, ask, id);
      if (r.ok && r.status === 201) this.pushFor(base.id);
      return r;
    }
    if (ask.mention_of) return this.mentionOf(ask, id, ask.mention_of, requestId);

    if (ask.asker === "thread" && base.semantic_key !== "") {
      const twin = this.db
        .prepare(
          `SELECT d.id FROM decisions d
           WHERE d.semantic_key = ? AND d.project = ? AND d.thread <> ?
             AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
             AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
             AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
           ORDER BY d.filed_at, d.id LIMIT 1`,
        )
        .get(base.semantic_key, ask.project, ask.thread) as { id: string } | undefined;
      if (twin) return this.mentionOf(ask, id, twin.id, requestId);
    }

    const res = this.store.insertDecision({ ...base, obligations: this.ownerNotice(base.id, ask, base.owner_thread ?? null) });
    if (!res.inserted) return this.replay(res.existing, id);
    this.pushFor(base.id);
    const related = this.db
      .prepare(
        `SELECT d.id FROM decisions d WHERE d.ask_key = ? AND d.project = ? AND d.id <> ?
           AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
         ORDER BY d.filed_at, d.id LIMIT 1`,
      )
      .get(ask.ask_key, ask.project, base.id) as { id: string } | undefined;
    if (related) {
      this.store.recordEvent("related", base.id, { related: related.id });
      return { ok: true, status: 201, decision_id: base.id, related: related.id };
    }
    return { ok: true, status: 201, decision_id: base.id };
  }

  private replay(known: { identity: string; result: string; decision_id: string }, id: string): FileResult {
    if (known.identity !== id) return fail(409, "request id reused for a different decision");
    // A refile of an ask whose first push failed tries the push again; pushFor is silent once any push is recorded.
    if (known.result !== "mention") this.pushFor(known.decision_id);
    return known.result === "mention"
      ? { ok: true, status: 200, decision_id: known.decision_id, mentioned: true }
      : { ok: true, status: 200, decision_id: known.decision_id };
  }

  private replace(base: DecisionInput, ask: Ask, id: string): FileResult {
    const prevId = base.supersedes!;
    const res = this.store.insertReplacement(base, "replace");
    if (res.ok) return { ok: true, status: 201, decision_id: res.decision_id };
    if ("existing" in res) return this.replay(res.existing, id);
    const prev = this.row(prevId);
    switch (res.reason) {
      case "already-ruled": {
        const pick = this.store.pick(prevId);
        return fail(409, `already ruled: ${JSON.stringify(prev && pick ? this.labelOf(prev, pick.option_id) : "")}`);
      }
      case "already-superseded":
        return fail(409, `already superseded by ${this.replacementOf(prevId)}`);
      case "withdrawn":
        return fail(409, "withdrawn");
      default:
        if (!prev) return fail(404, "unknown decision");
        if (prev.kind !== "decide" || ask.kind !== "decide") return fail(409, "only decide asks can be superseded");
        return fail(409, "not replaceable");
    }
  }

  private mentionOf(ask: Ask, id: string, targetId: string, requestId: string): FileResult {
    if (ask.asker !== "thread") return bad("only a thread can mention a decision");
    const t = this.row(targetId);
    if (!t) return fail(409, "unknown");
    if (t.withdrawn_at) return fail(409, "withdrawn");
    if (this.replacementOf(targetId)) return fail(409, "superseded");
    if (this.store.pick(targetId) || t.resolved_at) return fail(409, "picked");
    if (t.thread === ask.thread) return fail(409, "same thread");
    if (t.project !== ask.project) return fail(409, "different project");
    const res = this.store.insertMention({ request_id: requestId, identity: id, decision_id: targetId, thread: ask.thread! });
    if (!res.inserted) return this.replay(res.existing, id);
    return { ok: true, status: 201, decision_id: targetId, mentioned: true };
  }

  // ---- picking --------------------------------------------------------------

  /**
   * mk-cqnab.1 PR3: read-only pick-handler evaluation from persisted state, the Tasks comments API, host target
   * metadata and the bounded head reader. It records nothing and sends nothing; a later stage captures and acts.
   */
  async pickEvaluation(decisionId: string, tasks: () => TasksClient, opts: PickEvidenceOptions & { readHead?: HeadReader } = {}) {
    const images = this.deps.images;
    return evaluatePickRuntime({
      store: this.store,
      listComments: (taskId) => tasks().listComments(taskId),
      ...(this.deps.readTargetEvidence ? { readTargetEvidence: this.deps.readTargetEvidence } : {}),
      ...(opts.readHead ? { readHead: opts.readHead } : {}),
      // An image set is stored under the ordinary wake op; with no image store none could be accepted.
      imageCount: (op) => (images ? images.expectedCount(op) : 0),
      now: () => this.now(),
    }, decisionId, { ...(opts.deadlineAt ? { deadlineAt: opts.deadlineAt } : {}), ...(opts.captureEpoch !== undefined ? { captureEpoch: opts.captureEpoch } : {}) });
  }

  /** Async owner-entry guard. Frozen presentation is never silently refreshed. */
  async refreshOwnerPick(decisionId: string, tasks: () => TasksClient): Promise<string | null> {
    const d = this.row(decisionId);
    const ask = d && readAsk(d.body_json, {}, d.source);
    if (!d || d.source !== "card" || !ask?.options?.some(o => o.on_pick) || this.store.pick(decisionId)) return null;
    const observed: Record<string, TargetEvidence> = {};
    try {
      const client = tasks();
      const state = await client.taskState(String(d.task_id));
      if (state.state !== "labelled") throw new Error("card is no longer open and labelled");
      const card = parseCard(state.task.description);
      const thread = askingThread(await client.listComments(String(d.task_id)));
      if (thread !== d.thread || cardFingerprint({ title: state.task.title, card, thread, tasksProject: state.task.projectId }) !== d.card_fp) throw new Error("card changed; refresh the card");
      const binding = this.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(state.task.projectId) as { home_project: string; state: string } | undefined;
      if (binding?.state !== "confirmed" || binding.home_project !== d.project) throw new Error("confirmed binding unavailable");
      const row = this.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'on-pick-targets' ORDER BY seq LIMIT 1").get(decisionId) as { detail_json: string } | undefined;
      if (!row || !this.deps.readTargetEvidence) throw new Error("target snapshot or host evidence unavailable");
      const frozen = JSON.parse(row.detail_json) as { asker: TargetEvidence; targets: Record<string, TargetEvidence> };
      const blocks = (this.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(decisionId) as { ref: string }[]).map(b => b.ref);
      const expected = [frozen.asker, ...Object.values(frozen.targets)];
      for (const previous of expected) {
        const id = previous.thread_id;
        const current = observed[id] ?? await this.deps.readTargetEvidence(id);
        observed[id] = current;
        if (id !== thread && !blocks.includes(`thread:${id}`)) throw new Error("target outside frozen generation scope");
        if (current.thread_id !== id || current.archived || current.deleted) throw new Error("target inaccessible or no longer live");
        if (current.project_id !== previous.project_id) throw new Error("target project changed; refile the declaration");
        if (current.title !== previous.title || current.project_name !== previous.project_name) throw new Error("target presentation changed; refresh by refiling the card");
      }
      for (const option of ask.options ?? []) if (option.on_pick && frozen.targets[option.id!]?.thread_id !== option.on_pick.target_thread) throw new Error("target declaration snapshot mismatch");
      return null;
    } catch (e) {
      const error = `on_pick: ${message(e)}`;
      this.store.recordEvent("on-pick-target-refresh", decisionId, { observed, error });
      return error;
    }
  }

  /** Evidence is scoped to one synchronous commit callback, never a reusable permission flag. */
  async withValidatedPick(decisionId: string, commit: (guard: PickInput["guard"]) => PickResult, tasks = this.deps.tasks): Promise<PickResult> {
    const d = this.row(decisionId);
    const ask = d && readAsk(d.body_json, {}, d.source);
    if (!d || d.source !== "card" || !ask?.options?.some(o => o.on_pick) || this.store.pick(decisionId)) return commit(undefined);
    const snapshot = () => JSON.stringify({
      decision: this.row(decisionId),
      card: this.db.prepare("SELECT * FROM cards WHERE task_id = ?").get(String(d.task_id)),
      binding: this.db.prepare("SELECT * FROM project_bindings WHERE tasks_project_id = ?").get(String(d.tasks_project_id)),
      blocks: this.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(decisionId),
      targets: this.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'on-pick-targets' ORDER BY seq").all(decisionId),
    });
    const before = snapshot();
    if (!tasks) return { ok: false, status: 409, error: "on_pick: Tasks unavailable" };
    const error = await this.refreshOwnerPick(decisionId, tasks);
    if (error) return { ok: false, status: 409, error };
    const guard = () => before === snapshot() ? null : { status: 409, error: "on_pick: Home state changed during target validation" };
    this.entryGuards.set(guard, decisionId);
    try { return commit(guard); }
    finally { this.entryGuards.delete(guard); }
  }

  pick(decisionId: string, optionId: string, rev: string, pickId: string, by: string, surface: "home" | "overlay" | "cli", reason?: string, guard?: PickInput["guard"], images?: readonly ImageIn[], answers?: Readonly<Record<string, string>>, entryGuard?: PickInput["guard"]): PickResult {
    const d = this.row(decisionId);
    if (!d) return { ok: false, status: 404, error: "unknown decision" };
    if (d.kind === "steps") return this.pickSteps(d, optionId, rev, pickId, by, surface);
    if (d.kind !== "decide") return { ok: false, status: 400, error: "only decide and steps asks take a pick" };
    const ask = readAsk(d.body_json, {}, d.source);
    if (!ask) return UNREADABLE_PICK;
    if (revision(ask) !== d.revision) return { ok: false, status: 500, error: "stored revision does not match the stored decision" };
    if (d.source === "card") {
      // Revision 3: an ask on a project with no verified root cannot be ruled, by anyone, until the root is bound.
      const c = this.db.prepare("SELECT root_state FROM cards WHERE task_id = ?").get(String(d.task_id)) as { root_state: string | null } | undefined;
      if (c?.root_state !== "verified") return { ok: false, status: 403, error: "Project not bound: add a root to rule this" };
      // A held card is greyed out; a page opened before the hold must not rule it.
      if (this.holdOf(String(d.task_id))) return { ok: false, status: 409, error: "this card is on hold; the vizier releases it when its script or question is current" };
    }
    // A real option with the id "other" wins: it is picked as a normal option. mk's free-text answer (which always carries
    // his words in `reason` from the Home box) is refused for that ask, with a clear error.
    const realOther = ask.options?.some((o) => o.id === OTHER_OPTION_ID) === true;
    if (realOther && optionId === OTHER_OPTION_ID && by === "mk" && surface === "home" && reason !== undefined && reason.trim() !== "") {
      return { ok: false, status: 400, error: `this ask has its own option with id "other", so "Answer with this" is not available here; pick that option or use Ask / note` };
    }
    const isAnswers = optionId === ANSWERS_OPTION_ID && (ask.questions?.length ?? 0) > 0 && !ask.options?.some((o) => o.id === ANSWERS_OPTION_ID);
    let multi: { words: string; lines: string[]; json: string } | null = null;
    if (isAnswers) {
      if (by !== "mk") return { ok: false, status: 403, error: "only mk can answer a multi-pick ask" };
      if (d.source !== "card" && !d.project_root) return { ok: false, status: 403, error: "this decision has no verified project root" };
      const built = answersOf(ask.questions!, answers, reason);
      if (!built.ok) return { ok: false, status: 400, error: built.error };
      multi = built;
    } else if (answers !== undefined) return { ok: false, status: 400, error: "answers go only with a multi-pick ask" };
    // The Home box sends "other", or "mk-own-words" when the ask has its own option named "other", so mk can always answer in their own words.
    const isOther = (optionId === OTHER_OPTION_ID && !realOther) || (isOwnWordsForm(optionId) && !ask.options?.some((o) => o.id === optionId));
    const otherText = isOther ? cleanOther(reason) : null;
    if (isOther) {
      // mk's own words (plan: Other box). A virtual option: never in the ask, never delegable, mk only.
      if (by !== "mk") return { ok: false, status: 403, error: "only mk can answer with their own words" };
      if (otherText === null) return { ok: false, status: 400, error: `an answer of 1-${OTHER_MAX} characters is required` };
      if (d.source !== "card" && !d.project_root) return { ok: false, status: 403, error: "this decision has no verified project root" };
    }
    if (images && images.length > 0 && !isOther && !isAnswers) return { ok: false, status: 400, error: "images go only with your own answer" };
    const imgs = validateImages(images);
    if (!imgs.ok) return { ok: false, status: 400, error: imgs.error };
    if (imgs.files.length > 0 && !this.deps.images) return { ok: false, status: 503, error: "images are not available; nothing was sent" };
    const option = isAnswers
      ? ({ id: ANSWERS_OPTION_ID, label: "Answers", kind: "ruling-only", reversible: false } as NonNullable<Ask["options"]>[number])
      : isOther
      ? ({ id: optionId, label: "Other (mk's own answer)", kind: "ruling-only", reversible: false } as NonNullable<Ask["options"]>[number])
      : ask.options?.find((o) => o.id === optionId);
    if (!option) return { ok: false, status: 400, error: `unknown option ${JSON.stringify(optionId)}` };
    if (rev !== d.revision) return { ok: false, status: 409, error: "revision differs" };

    // Version 1 (legacy) digest: kept only to recognise retries of picks stored before version 2. It omitted an ordinary reason.
    const base = `${optionId}|${rev}|${by}${isOther ? `|${otherText}` : ""}${multi ? `|${multi.json}` : ""}`;
    const legacyHash = imgs.files.length > 0 ? `img1:${createHash("sha256").update(JSON.stringify([base, imgs.digest])).digest("hex")}` : createHash("sha256").update(base).digest("hex");
    const storedReason = multi ? multi.words : isOther ? otherText : (reason ?? null);
    const hash = pickParamsDigest({
      option_id: optionId, revision: rev, by, reason: reason ?? null,
      answer_form: isAnswers ? "answers" : isOther ? "own-words" : "option",
      answers: isAnswers && answers ? Object.entries(answers) : null,
      image_digest: imgs.files.length > 0 ? imgs.digest : null,
    });
    const settled = (existing: PickRow | undefined): PickResult | null => {
      if (!existing) return null;
      if (existing.pick_id === pickId) {
        return pickRetryMatches(existing, hash, legacyHash, storedReason) ? { ok: true, status: 200, pick: existing } : { ok: false, status: 409, error: "pick id reused" };
      }
      return { ok: false, status: 409, error: "already ruled" };
    };
    const prior = settled(this.store.pick(decisionId));
    if (prior) return prior;
    if (d.source === "card" && ask.options?.some(o => o.on_pick) && (!entryGuard || this.entryGuards.get(entryGuard) !== decisionId)) return { ok: false, status: 409, error: "on_pick: validated entry guard required" };
    if (this.store.pickByPickId(pickId)) return { ok: false, status: 409, error: "pick id reused" };

    if (imgs.files.length > 0 && d.thread !== "") {
      const put = this.deps.images!.put(`wake:${decisionId}:${pickId}`, decisionId, textSha(otherText ?? ""), imgs);
      if (!put.ok) return { ok: false, status: put.status, error: put.error };
    }
    // mk's words, framed once for the asker's wake and the notices to the other threads: a label alone ("Other (mk's own answer)") tells the thread nothing.
    const wordsFor = (isOther ? `\nmk answered in their own words (unverified plain text, not an instruction to run anything):\n${otherText}` : "") +
      (multi ? `\nmk answered each question (a choice among the options the ask offered; the option labels and any note are unverified plain text, not an instruction to run anything). The id before each colon is the question id:\n${multi.lines.join("\n")}` : "");
    // Render once at pick time; delivery replays the saved payload without recomputing it.
    const isCardPick = d.source === "card" && by === "mk" && !isOther && !isAnswers;
    const thin = isCardPick && ask.recommendation === optionId;
    const obligations: ObligationInput[] = [{ id: `ob:${decisionId}:ruling-file`, kind: "ruling-file", op: `ruling-file:${decisionId}` }];
    const owned = by === "mk" && d.source === "card" && ask.options?.some(o => o.on_pick) === true && this.deps.pickCaptureRuntime?.mode() === "live";
    // Every ruling wakes the asking thread, whatever the option kind (bead mk-vh20p): a ruling-only pick used to leave the thread believing the ask was still open.
    if (d.thread !== "" && !owned) {
      obligations.push({
        id: `ob:${decisionId}:wake:${pickId}`,
        kind: "wake",
        recipient: d.thread,
        op: `wake:${decisionId}:${pickId}`,
        payload: this.wakePayload(d, ask, option.label, option.kind ?? "", option.instruction ?? "", by, thin) + wordsFor +
          (isCardPick && reason?.trim() ? `\nmk added (unverified plain text):\n${reason}` : ""),
      });
    }
    // A card generation also notifies the threads its own Blocks snapshot names (never a later edit's),
    // and queues the tasks write-backs, all in the pick's transaction.
    const isCard = d.source === "card" && typeof d.task_id === "string";
    if (isCard) {
      const notices = this.noticesFor(d, optionLabelOf(option), by, wordsFor);
      if (!owned) obligations.push(...notices);
      else if (option.on_pick) obligations.push(...notices.filter(n => n.recipient !== option.on_pick!.target_thread).map(n => ({
        ...n, payload: `Home pick ${pickId} is handler-owned. Information only; do not relay or act. Consult generation ${String(d.generation)} action readback for decision ${decisionId}.`,
      })));
    }
    const cardWrites = isCard
      ? pickWrites(
          { id: decisionId, task_id: String(d.task_id) },
          { option_id: optionId, option_label: option.label, by, generation: Number(d.generation), picked_at: this.now(), reason: multi ? multi.words : isOther ? otherText : (reason ?? null) },
        )
      : [];
    // Only mk's own pick mints, and it does so in the pick's transaction. A record is not an authorization.
    let approval: PickInput["approval"];
    if (by === "mk" && option.approval) {
      const ttl = option.approval.ttl && option.approval.ttl > 0 ? option.approval.ttl : APPROVAL_DEFAULT_TTL;
      approval = {
        approval_id: `apr:${this.newId()}`,
        kind: option.approval.kind,
        target: option.approval.target,
        identity: option.approval.identity,
        expires_at: new Date(Date.parse(this.now()) + Math.min(ttl, APPROVAL_MAX_TTL) * 1000).toISOString(),
      };
    }
    let res;
    try {
      res = this.store.recordPick(
        { decision_id: decisionId, pick_id: pickId, option_id: optionId, revision: rev, by, surface, reason: storedReason, params_hash: hash, approval, guard: () => entryGuard?.() ?? guard?.() ?? null,
          ...(owned ? { capture: () => { capturePick(this.store, decisionId, "live", this.now()); } } : {}) },
        obligations,
        cardWrites,
      );
    } catch (e) {
      if (/UNIQUE/.test(message(e))) return { ok: false, status: 409, error: "pick id reused" };
      throw e;
    }
    if (!res.ok) {
      switch (res.reason) {
        case "guard":
          return { ok: false, status: res.refusal.status, error: res.refusal.error };
        case "superseded":
          return { ok: false, status: 409, error: `superseded by ${this.replacementOf(decisionId)}` };
        case "withdrawn":
          return { ok: false, status: 409, error: "withdrawn" };
        case "already-ruled":
          return settled(res.existing) ?? { ok: false, status: 409, error: "already ruled" };
        default:
          return { ok: false, status: 409, error: "revision differs" };
      }
    }
    try {
      this.reconcile(decisionId);
    } finally {
      this.deps.nudge?.();
    }
    // Ruling a card ends its time in Later.
    if (isCard) this.unlater(String(d.task_id));
    if (this.deps.pickHandlerMode === "shadow" && isCard && ask.options?.some(o => o.on_pick)) {
      const failed = (error: unknown) => {
        try { this.store.recordEvent("pick-collection-failure", decisionId, { pick_id: pickId, error: message(error) }); }
        catch { this.deps.log?.("pick collection failure could not be recorded"); }
      };
      try {
        const action = capturePick(this.store, decisionId, "shadow", this.now());
        const fenceToken = pickFenceToken(this.store, action);
        const tasks = this.deps.tasks;
        if (!tasks) throw new Error("Tasks unavailable for shadow collection");
        void this.pickEvaluation(decisionId, tasks, { deadlineAt: action.evaluation_deadline_at, captureEpoch: action.capture_epoch }).then(result => {
          if (!result) throw new Error("shadow evaluation unavailable");
          recordPickEvaluation(this.store, action, result, this.now(), fenceToken);
        }).catch(failed);
      } catch (error) { failed(error); }
    }
    return { ok: true, status: 201, pick: this.store.pick(decisionId)! };
  }

  /**
   * An open ask whose stored body cannot be read is shown to mk marked "could not be read" (bead mk-j5ipa.14), and its asking
   * thread is told once to file it again. The obligation id carries the decision id, which is per card generation, so a refresh
   * or a later poll inserts nothing new (insertObligations ignores a known id) and a re-filed generation is told once more.
   */
  tellUnreadable(): number {
    let told = 0;
    for (const d of this.owed()) {
      if (readAsk(d.body_json, {}, d.source)) continue;
      const taskId = d.task_id ? String(d.task_id) : null;
      const asker = (taskId ? (this.db.prepare("SELECT asking_thread FROM cards WHERE task_id = ?").get(taskId) as { asking_thread: string | null } | undefined)?.asking_thread : null) || d.thread;
      if (!asker) continue;
      const key = this.cardKey(taskId) ?? d.id;
      told += this.store.insertObligations(d.id, [
        {
          id: `ob:${d.id}:unreadable`,
          kind: "notice",
          recipient: asker,
          op: `unreadable:${d.id}`,
          payload: `Home could not read the card ${key} (${d.id}), so mk sees "${key} could not be read" and cannot answer it. Nothing was ruled. Please file it again, with a new request id, so mk can answer it. This is not merge, deploy, release or publish authorization.`,
        },
      ]);
    }
    if (told > 0) this.deps.nudge?.();
    return told;
  }

  /** One notice per distinct thread in the generation's Blocks snapshot, except the asking thread. */
  private noticesFor(d: Row, label: string, by: string, words = ""): ObligationInput[] {
    const refs = this.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(d.id) as { ref: string }[];
    const threads = [...new Set(refs.map((r) => r.ref).filter((r) => r.startsWith("thread:")).map((r) => r.slice("thread:".length)))].filter((t) => t !== "" && t !== d.thread);
    return threads.map((t) => ({
      id: `ob:${d.id}:notice:${t}`,
      kind: "notice",
      recipient: t,
      op: `notice:${d.id}:${t}`,
      payload: `${by === "vizier" ? "The vizier ruled on" : by === "auto" ? "Home closed, on evidence and not by mk's click," : "mk ruled on"} the card ${d.id}${d.subject ? ` (${d.subject})` : ""}, which names your thread: picked ${JSON.stringify(label)}. This is not merge, deploy, release or publish authorization. Look the card up with \`bb home get --id ${d.id}\`.${words}`,
    }));
  }

  /** Read a recorded approval. Always `authorizing: false`; there is no way to spend or consume one. */
  approvalCheck(kind: string, target: string, identity: string) {
    const r = this.store.checkApproval(kind, target, identity);
    return {
      status: r.status,
      authorizing: false as const,
      ...(r.approval ? { approval_id: r.approval.approval_id, minted_at: r.approval.minted_at, expires_at: r.approval.expires_at, revoked_at: r.approval.revoked_at } : {}),
    };
  }

  revokeApproval(approvalId: string) {
    return this.store.revokeApproval(approvalId);
  }

  /** "Done" on a steps ask: one steps-done notice per recipient (asker and mentioners), all in the pick's transaction. */
  private pickSteps(d: Row, optionId: string, rev: string, pickId: string, by: string, surface: "home" | "overlay" | "cli"): PickResult {
    if (optionId !== "done") return { ok: false, status: 400, error: "a steps ask takes only the option done" };
    if (rev !== d.revision) return { ok: false, status: 409, error: "revision differs" };
    const hash = createHash("sha256").update(`${optionId}|${rev}|${by}`).digest("hex");
    const settled = (existing: PickRow | undefined): PickResult | null => {
      if (!existing) return null;
      if (existing.pick_id === pickId) return existing.params_hash === hash ? { ok: true, status: 200, pick: existing } : { ok: false, status: 409, error: "pick id reused" };
      return { ok: false, status: 409, error: "already done" };
    };
    const prior = settled(this.store.pick(d.id));
    if (prior) return prior;
    if (this.store.pickByPickId(pickId)) return { ok: false, status: 409, error: "pick id reused" };
    const ask = readAsk(d.body_json, {}, d.source);
    if (!ask) return UNREADABLE_PICK;
    const threads = [...new Set([d.thread, ...this.store.mentions(d.id).map((m) => m.thread)].filter((t) => t !== ""))];
    const obligations: ObligationInput[] = threads.map((t) => ({
      id: `ob:${d.id}:steps-done:${t}`,
      kind: "steps-done",
      recipient: t,
      op: `steps-done:${d.id}:${t}`,
      payload: t === d.thread
        ? `mk finished your steps ${d.id}${d.subject ? ` (${d.subject})` : ""}: ${(ask.steps ?? []).length} ${(ask.steps ?? []).length === 1 ? "step" : "steps"}. Look the card up with \`bb home get --id ${d.id}\`.`
        : [
            `mk finished your steps ${d.id}${d.subject ? ` (${d.subject})` : ""}.`,
            ...(ask.steps ?? []).map((s, i) => `${i + 1}. ${s}`),
          ].join("\n"),
    }));
    let res;
    try {
      res = this.store.recordPick({ decision_id: d.id, pick_id: pickId, option_id: optionId, revision: rev, by, surface, reason: null, params_hash: hash }, obligations);
    } catch (e) {
      if (/UNIQUE/.test(message(e))) return { ok: false, status: 409, error: "pick id reused" };
      throw e;
    }
    if (!res.ok) {
      if (res.reason === "already-ruled") return settled(res.existing) ?? { ok: false, status: 409, error: "already done" };
      return { ok: false, status: 409, error: res.reason === "withdrawn" ? "withdrawn" : res.reason };
    }
    this.deps.nudge?.();
    return { ok: true, status: 201, pick: this.store.pick(d.id)! };
  }

  /** A snapshot taken at pick time, so the wake never depends on rows that may change later. */
  private wakePayload(d: Row, ask: Ask, label: string, kind: string, instruction: string, by: string, thin: boolean): string {
    const lines: string[] = [];
    if (by === "vizier") {
      lines.push(
        `The vizier ruled on your decision ${d.id} in mk's place (you marked this option reversible). mk may override it. This is not merge, deploy, release or publish authorization.`,
      );
    }
    if (by === "auto") {
      lines.push("Home closed this card automatically because independent evidence shows the action was done. mk did not click, and this is not mk's ruling or any authorization.");
    }
    const over = this.store.overriddenRuling(d.id);
    if (over) lines.push(`This supersedes the vizier's ruling on ${over.decision_id}.`);
    lines.push(`Ruling on ${d.id}${d.subject ? ` (${d.subject})` : ""}: ${by} picked ${JSON.stringify(label)}.`);
    const points = thin ? Array.from(ask.question ?? "") : [];
    const cut = thin && points.length > 300;
    const question = cut ? points.slice(0, 300).join("") + "…" : ask.question;
    lines.push(`Question: ${question}`);
    if (cut) lines.push(`Look the card up with \`bb home get --id ${d.id}\`.`);
    if (kind === "instruction") lines.push(`Instruction: ${instruction}`);
    else if (kind === "needs-context") lines.push("This choice needs more context from you before you act.");
    return lines.join("\n");
  }

  // ---- reconcile ------------------------------------------------------------

  /** Write the ruling file for a picked decision, then mark its obligation done. Failures back off. */
  reconcile(decisionId: string): void {
    const ob = this.store.obligationsFor(decisionId).find((o) => o.kind === "ruling-file");
    if (!ob || ob.state !== "pending") return;
    try {
      const d = this.row(decisionId)!;
      const pick = this.store.pick(decisionId);
      if (!pick) throw new Error("no pick recorded");
      const ask = this.askOf(d);
      const option = ask.options?.find((o) => o.id === pick.option_id);
      const estate = d.project === "estate";
      const ruling: Ruling = {
        decision_id: d.id,
        pick_id: pick.pick_id,
        revision: pick.revision,
        asking_thread: d.thread,
        subject: d.subject || ask.question.slice(0, 80),
        options_shown: (ask.options ?? []).map((o) => ({ id: o.id!, label: o.label })),
        picked: pick.option_id,
        ruled_by: pick.by === "vizier" ? "vizier" : pick.by === "auto" ? "auto" : "mk",
        ruled_at: pick.picked_at,
        ruling: `Picked ${JSON.stringify(option?.label ?? pick.option_id)} for: ${ask.question}`,
        source: `autarch-home:${pick.surface}`,
      };
      if (option?.kind === "instruction" && option.instruction) ruling.instruction = option.instruction;
      if (pick.by === "vizier" && pick.reason) ruling.delegated_reason = pick.reason;
      if (pick.by === "auto" && pick.reason) ruling.auto_evidence = pick.reason;
      if (isOwnWordsId(pick.option_id) && pick.reason) ruling.ruling = `mk answered in their own words for: ${ask.question} -- ${pick.reason.replace(/\s+/g, " ")}`;
      if (pick.option_id === ANSWERS_OPTION_ID && pick.reason) ruling.ruling = `mk answered each question for: ${ask.question} -- ${pick.reason.replace(/\s+/g, " ")}`;
      if (d.supersedes) ruling.supersedes = d.supersedes;
      if (d.source === "card" && typeof d.task_id === "string") {
        // The stored generation body is the snapshot; the live card is never consulted.
        ruling.card_id = d.task_id;
        const key = (this.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(d.task_id) as { card_key: string | null } | undefined)?.card_key;
        if (key) ruling.card_key = key;
        ruling.generation = Number(d.generation);
      }
      const threads = this.store.mentions(d.id).map((m) => m.thread);
      if (threads.length > 0) ruling.mentions = threads;
      // The ledger comes first: it does not depend on the project having a root, so a pick on an unbound project still
      // reaches it. A retry after a later failure finds its line and writes nothing.
      if (this.deps.rulingsLedger) {
        appendRuling(this.deps.rulingsLedger, {
          pick_id: pick.pick_id,
          picked_at: pick.picked_at,
          by: pick.by,
          surface: pick.surface,
          card: ruling.card_key ?? d.id,
          subject: ruling.subject,
          option_id: pick.option_id,
          option_label: option?.label ?? pick.option_id,
          reason: pick.reason,
          ...(option?.on_pick ? { declaration: {
            decision_id: d.id, pick_id: pick.pick_id, generation: Number(d.generation), revision: String(d.revision),
            on_pick_sha256: createHash("sha256").update(canonicalOnPick(option.on_pick)).digest("hex"),
            kind: option.kind ?? "ruling-only",
            pr: option.on_pick.subject.kind === "github-pr" ? `${option.on_pick.subject.repo}#${option.on_pick.subject.number}` : null,
            head: option.on_pick.subject.kind === "github-pr" ? option.on_pick.subject.head : null,
            bead: option.on_pick.ruling.bead, under: option.on_pick.ruling.under,
          } } : {}),
        });
      }
      const target = rulingPath({ scope: estate ? "estate" : "project", decision_id: d.id, subject: ruling.subject, date: pick.picked_at.slice(0, 10) });
      writeRuling({ path: d.project_root ?? "", dev: d.root_dev ?? "", ino: d.root_ino ?? "", fsid: d.root_fsid ?? null }, target.dirs, target.file, renderRuling(ruling));
      this.store.transition(ob.id, "pending", "done", ob.attempt);
    } catch (e) {
      const delay = Math.min(30_000 * 2 ** Math.min(ob.attempt, 20), 3_600_000);
      this.store.failObligation(ob.id, ob.attempt, message(e), new Date(Date.parse(this.now()) + delay).toISOString());
    }
  }

  reconcileAll(): void {
    for (const o of this.store.dueRulingFiles(this.now())) this.reconcile(o.decision_id);
    this.reconcilePushes();
  }

  start(): void {
    if (this.timer) return;
    this.sweepAbandonedOverrides();
    this.timer = setInterval(() => this.reconcileAll(), RECONCILE_MS);
    this.timer.unref();
    this.reconcilePushes(); // a push lost to a restart is retried now, not after the first interval
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }

  ready(): boolean {
    return true;
  }

  // ---- readers --------------------------------------------------------------

  wakes(): ObligationRow[] {
    return this.store.dueMessages(this.now());
  }

  /**
   * Messages that could not be delivered and still need a person. A row is settled, and left out, when it was cancelled
   * (voided: a bring-back or an override), or the answer was delivered again: a later wake or bring-back notice to the
   * SAME recipient on the same decision reached `done`. Later means it finished at or after the failure and was inserted
   * after the failed row; insertion order alone is not completion order. A message to another thread, an information
   * notice, and a batched digest are not evidence: a notice is `done` the moment it is merged into a digest, before the
   * digest is delivered. The row itself stays as it is: this is a read, not a state change (bead mk-j6eqk).
   */
  undeliverable(): ObligationRow[] {
    return this.db
      .prepare(
        `SELECT o.* FROM obligations o
         WHERE o.state = 'undeliverable'
           AND o.voided_at IS NULL
           AND NOT EXISTS (
             SELECT 1 FROM obligations l
             WHERE l.decision_id = o.decision_id AND l.id <> o.id
               AND l.recipient IS NOT NULL AND l.recipient = o.recipient
               AND l.state = 'done'
               AND l.updated_at >= o.updated_at AND l.rowid > o.rowid
               AND (l.kind = o.kind OR (o.kind IN ('wake','void-notice') AND l.kind IN ('wake','void-notice'))))
         ORDER BY o.rowid`,
      )
      .all() as ObligationRow[];
  }

  failures(): ObligationRow[] {
    return this.db
      .prepare("SELECT * FROM obligations WHERE kind = 'ruling-file' AND state = 'pending' AND last_error IS NOT NULL ORDER BY rowid")
      .all() as ObligationRow[];
  }

  private rerouteTried(o: ObligationRow): boolean {
    return !!this.db
      .prepare("SELECT 1 FROM events WHERE decision_id = ? AND type IN ('rerouted','reroute-failed') AND json_extract(detail_json, '$.obligation') = ? LIMIT 1")
      .get(o.decision_id, o.id);
  }

  dismiss(decisionId: string, obligationId: string): { ok: true } | { ok: false; reason: string } {
    const o = this.store.obligation(obligationId);
    if (!o || o.decision_id !== decisionId) return { ok: false, reason: "unknown" };
    if (o.state !== "undeliverable") return { ok: false, reason: "not-undeliverable" };
    // An answer stranded by an archived or gone thread is dismissed only after a reroute was tried and recorded.
    const recoverable = !!o.recipient && o.voided_at == null && (o.kind === "wake" || o.kind === "notice");
    if (recoverable && /archived|thread gone/.test(o.last_error ?? "") && !this.rerouteTried(o)) return { ok: false, reason: "not-rerouted" };
    const r = this.store.transition(o.id, "undeliverable", "dismissed", o.attempt);
    return r.ok ? { ok: true } : { ok: false, reason: r.reason };
  }

  // ---- holds: the vizier greys a card out while its script or question is superseded. Kept in one settings_kv value
  // (no schema change). A hold hides the card from "needs you now"; it never closes, rules or edits the card.
  // The parsed list is kept (outside a transaction) while the store's settings stamp is the same, so a poll that asks about every card parses
  // the stored value once, not once per card. Readers get copies: nothing outside can change what the next one sees.
  private holdCache: { stamp: string; holds: Record<string, Hold> } | undefined;
  /** Every stored hold, expired or not (the sweep needs the expired ones to tell their holders). */
  private rawHolds(): Record<string, Hold> {
    const stamp = this.store.settingsStamp();
    if (stamp === null) return this.parseHolds();
    if (this.holdCache?.stamp === stamp) return this.holdCache.holds;
    const holds = this.parseHolds();
    this.holdCache = { stamp, holds };
    return holds;
  }
  private parseHolds(): Record<string, Hold> {
    try {
      const v = JSON.parse(this.store.setting("holds") ?? "{}");
      if (!v || typeof v !== "object" || Array.isArray(v)) return {};
      // Fail open: an entry without a reason, a writer and a readable time is not a hold. An end that is not exactly
      // one readable date or event is dropped, leaving a hold with no end.
      const ok: Record<string, Hold> = {};
      for (const [id, h] of Object.entries(v as Record<string, unknown>)) {
        const e = h as { reason?: unknown; by?: unknown; at?: unknown; n?: unknown; end?: unknown } | null;
        if (e && typeof e.reason === "string" && e.reason.trim() !== "" && typeof e.by === "string" && typeof e.at === "string" && !Number.isNaN(Date.parse(e.at))) {
          const end = parseStoredEnd(e.end);
          ok[id] = { reason: e.reason, by: e.by, at: e.at, ...(typeof e.n === "string" ? { n: e.n } : {}), ...(end ? { end } : {}) };
        }
      }
      return ok;
    } catch {
      return {};
    }
  }
  /** True once a dated end has passed. An event end, or no end, never passes by time. */
  private holdExpired(h: Hold, nowMs: number): boolean {
    const at = endMs(h.end);
    return at !== null && at <= nowMs;
  }
  /** The holds that are in force now: a hold whose date has passed is gone for every reader, before any sweep runs. */
  private readHolds(): Record<string, Hold> {
    const raw = this.rawHolds();
    const nowMs = Date.parse(this.now());
    let live = raw;
    for (const [id, h] of Object.entries(raw)) {
      if (!this.holdExpired(h, nowMs)) continue;
      if (live === raw) live = { ...raw };
      delete live[id];
    }
    return live;
  }
  holds(): Record<string, Hold> {
    return Object.fromEntries(Object.entries(this.readHolds()).map(([id, h]) => [id, copyHold(h)]));
  }
  holdOf(taskId: string | null | undefined): Hold | null {
    const h = taskId ? this.rawHolds()[taskId] : undefined;
    return h && !this.holdExpired(h, Date.parse(this.now())) ? copyHold(h) : null;
  }
  /** The task id of a card named by its task id or its tasks key (PROJ-24). */
  cardTaskId(ref: string): string | null {
    const r = this.db.prepare("SELECT task_id FROM cards WHERE (task_id = ? OR card_key = ?) AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1").get(ref, ref) as { task_id: string } | undefined;
    return r?.task_id ?? null;
  }
  /**
   * Hold a card. A new hold names its end: `end` is `{ until }` (a date or a full time) or `{ event: { kind, ref, label } }`.
   * Changing a hold that is still in force needs none: it keeps the end it has, or takes the one given. A hold whose date
   * passed is not in force, so a replacement over it must name an end, and its holder is told about the first one.
   */
  hold(ref: string, reason: string, by: string, end?: HoldEnd): { ok: true; task_id: string } | { ok: false; error: string } {
    const why = reason.trim();
    if (!why) return { ok: false, error: "a hold needs a reason" };
    if (why.length > 500 || /[\r\n]/.test(why)) return { ok: false, error: "the reason is one line of at most 500 characters" };
    const id = this.cardTaskId(ref);
    if (!id) return { ok: false, error: "no such card" };
    const at = this.now();
    const nowMs = Date.parse(at);
    let named: HoldEnd | undefined;
    if (end !== undefined) {
      const c = checkHoldEnd(end, nowMs);
      if (!c.ok) return { ok: false, error: c.error };
      named = c.end;
    }
    const told = this.store.atomically((): { error: string } | { woke: number } => {
      const h = { ...this.rawHolds() };
      const prev = h[id];
      const live = prev && !this.holdExpired(prev, nowMs) ? prev : undefined;
      const keep = named ?? live?.end;
      if (!keep && !live) return { error: NEEDS_END };
      const woke = prev && !live ? this.tellHolder(id, prev, `its date passed (${holdEndPhrase(prev.end, nowMs)})`) : 0;
      h[id] = { reason: why, by, at, n: randomUUID(), ...(keep ? { end: keep } : {}) };
      this.store.setSetting("holds", JSON.stringify(h));
      return { woke };
    });
    if ("error" in told) return { ok: false, error: told.error };
    if (told.woke > 0) this.deps.nudge?.();
    return { ok: true, task_id: id };
  }
  /** Tell the thread that set a hold that it ended, once: the op names the hold, so a repeat inserts nothing. Returns rows inserted. */
  private tellHolder(taskId: string, h: Hold, how: string): number {
    const key = this.cardKey(taskId) ?? taskId;
    const op = `hold-end:${taskId}:${h.n ?? h.at}`;
    const payload = `Home card ${key}: your hold ended (${how}). The card is back at mk's pick; nothing was ruled or closed. Your reason was (plain text): ${h.reason}`;
    return this.store.insertObligations(`hold:${taskId}`, [{ id: `ob:${op}`, kind: "hold-ended", recipient: h.by, op, payload }]);
  }
  /**
   * A hold with an event end is over: the thread that watches the event says so, naming the event (its kind and ref). A
   * report for an event the card is not held for does nothing, so a retried report cannot release a later hold on another event.
   * Date holds end by their date, old ones by unhold.
   */
  endHold(ref: string, by: string, eventKind: string, eventRef: string): { ok: true; task_id: string; ended: boolean; held_for?: string } | { ok: false; error: string } {
    const id = this.cardTaskId(ref);
    if (!id) return { ok: false, error: "no such card" };
    const out = this.store.atomically(() => {
      const raw = { ...this.rawHolds() };
      const h = raw[id];
      if (!h) return { ok: true as const, task_id: id, ended: false, woke: 0 };
      if (!h.end || !("event" in h.end)) return { ok: false as const, error: h.end ? "this hold ends on its date; wait for it, or use unhold" : "this hold has no end set; only unhold releases it" };
      if (h.end.event.kind !== eventKind || h.end.event.ref !== eventRef) return { ok: true as const, task_id: id, ended: false, woke: 0, held_for: h.end.event.ref };
      const woke = this.tellHolder(id, h, `${h.end.event.label} (reported by ${by})`);
      delete raw[id];
      this.store.setSetting("holds", JSON.stringify(raw));
      return { ok: true as const, task_id: id, ended: true, woke };
    });
    if (!out.ok) return out;
    if (out.woke > 0) this.deps.nudge?.();
    return { ok: true, task_id: out.task_id, ended: out.ended, ...("held_for" in out ? { held_for: out.held_for } : {}) };
  }
  /** Release every hold whose date has passed and tell each holder once. The record goes in the same transaction as the notice. */
  sweepExpiredHolds(): number {
    const nowMs = Date.parse(this.now());
    const due = Object.entries(this.rawHolds()).filter(([, h]) => this.holdExpired(h, nowMs));
    if (due.length === 0) return 0;
    let swept = 0;
    let woke = 0;
    for (const [id, seen] of due) {
      woke += this.store.atomically(() => {
        const raw = { ...this.rawHolds() };
        const h = raw[id];
        // Another writer may have replaced or removed it since the read: release only the hold that was seen.
        if (!h || h.n !== seen.n || h.at !== seen.at || !this.holdExpired(h, nowMs)) return 0;
        const n = this.tellHolder(id, h, `its date passed (${holdEndPhrase(h.end, nowMs)})`);
        delete raw[id];
        this.store.setSetting("holds", JSON.stringify(raw));
        swept++;
        return n;
      });
    }
    if (woke > 0) this.deps.nudge?.();
    return swept;
  }
  unhold(ref: string): { ok: true; task_id: string; was: boolean } | { ok: false; error: string } {
    const id = this.cardTaskId(ref);
    if (!id) return { ok: false, error: "no such card" };
    // Read the stored list, not the live one: writing the live list back would drop another card's expired hold before its holder was told.
    const res = this.store.atomically(() => {
      const h = { ...this.rawHolds() };
      const nowMs = Date.parse(this.now());
      const prev = h[id];
      const was = prev !== undefined && !this.holdExpired(prev, nowMs);
      let woke = 0;
      if (prev) {
        // A date that already passed but was not swept yet: the holder still hears it ended, once, in this transaction.
        if (!was) woke = this.tellHolder(id, prev, `its date passed (${holdEndPhrase(prev.end, nowMs)})`);
        delete h[id];
        this.store.setSetting("holds", JSON.stringify(h));
      }
      return { ok: true as const, task_id: id, was, woke };
    });
    if (res.woke > 0) this.deps.nudge?.();
    return { ok: true, task_id: res.task_id, was: res.was };
  }
  // ---- later: mk sets an open card aside to do after the fast ones. Kept in one settings_kv value keyed by task id
  // (no schema change). Unlike a hold it never refuses a pick and it is mk's own choice; ruling the card clears it.
  private laterCache: { stamp: string; laters: Record<string, Later> } | undefined;
  private readLaters(): Record<string, Later> {
    const stamp = this.store.settingsStamp();
    if (stamp === null) return this.parseLaters();
    if (this.laterCache?.stamp === stamp) return this.laterCache.laters;
    const laters = this.parseLaters();
    this.laterCache = { stamp, laters };
    return laters;
  }
  private parseLaters(): Record<string, Later> {
    try {
      const v = JSON.parse(this.store.setting("later") ?? "{}");
      if (!v || typeof v !== "object" || Array.isArray(v)) return {};
      // Fail open: an entry without a writer and a readable time is not a Later.
      const ok: Record<string, Later> = {};
      for (const [id, l] of Object.entries(v as Record<string, unknown>)) {
        const e = l as { at?: unknown; by?: unknown } | null;
        if (e && typeof e.by === "string" && typeof e.at === "string" && !Number.isNaN(Date.parse(e.at))) ok[id] = { at: e.at, by: e.by };
      }
      return ok;
    } catch {
      return {};
    }
  }
  laters(): Record<string, Later> {
    return Object.fromEntries(Object.entries(this.readLaters()).map(([id, l]) => [id, { ...l }]));
  }
  laterOf(taskId: string | null | undefined): Later | null {
    const l = taskId ? this.readLaters()[taskId] : undefined;
    return l ? { ...l } : null;
  }
  /** Task ids set aside as whole cards and not on hold: a hold beats Later everywhere it is shown or counted. */
  laterTasks(): Set<string> {
    const holds = this.readHolds();
    return new Set(Object.keys(this.readLaters()).filter((t) => !holds[t]));
  }
  later(ref: string, by: string): { ok: true; task_id: string; was: boolean } | { ok: false; error: string } {
    const id = this.cardTaskId(ref);
    if (!id) return { ok: false, error: "no such card" };
    // One transaction per read-modify-write, so two connections adding different cards cannot drop each other's.
    return this.store.atomically(() => {
      const l = this.laters();
      const was = id in l;
      if (!was) {
        l[id] = { at: this.now(), by };
        this.store.setSetting("later", JSON.stringify(l));
      }
      return { ok: true as const, task_id: id, was };
    });
  }
  unlater(ref: string): { ok: true; task_id: string; was: boolean } | { ok: false; error: string } {
    const id = this.cardTaskId(ref);
    if (!id) return { ok: false, error: "no such card" };
    return this.store.atomically(() => {
      const l = this.laters(); // a copy: the cached read must not be mutated in place
      const was = id in l;
      if (was) {
        delete l[id];
        this.store.setSetting("later", JSON.stringify(l));
      }
      return { ok: true as const, task_id: id, was };
    });
  }
  /** A comment posted after the hold that carries a sha256 (a new script) makes the card live again. */
  async releaseHoldOnSha(taskId: string, comments: () => Promise<TaskCommentRow[]>): Promise<boolean> {
    const hold = this.holdOf(taskId);
    if (!hold) return false;
    let list: TaskCommentRow[];
    try {
      list = await comments();
    } catch {
      return false; // unreadable now: stay held, try on the next change
    }
    if (!list.some((c) => c.kind !== "system" && Date.parse(c.createdAt) > Date.parse(hold.at) && /\b[0-9a-f]{64}\b/.test(c.body))) return false;
    // The vizier may have replaced the hold while the comments were read: clear only the hold those comments were judged against.
    const now = this.holdOf(taskId);
    if (now?.at !== hold.at || now?.n !== hold.n) return false;
    this.unhold(taskId);
    return true;
  }

  /** The tasks key of a card (for example PROJ-24), or null for a legacy ask or a card without one. */
  cardKey(taskId: string | null | undefined): string | null {
    if (!taskId) return null;
    const r = this.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(taskId) as { card_key: string | null } | undefined;
    return r?.card_key ?? null;
  }

  /** The title of a card, or null for a legacy ask or a card without one. */
  cardTitle(taskId: string | null | undefined): string | null {
    if (!taskId) return null;
    const r = this.db.prepare("SELECT title FROM cards WHERE task_id = ?").get(taskId) as { title: string | null } | undefined;
    return r?.title ?? null;
  }

  /** Whether this one decision is in `owed()`: the same conditions, one row read, nothing else loaded. */
  isOwed(decisionId: string): boolean {
    return !!this.db
      .prepare(
        `SELECT 1 FROM decisions d
         WHERE d.id = ? AND d.kind = 'decide' AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
           AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)`,
      )
      .get(decisionId);
  }

  /** Open decide asks with no pick, not withdrawn, resolved or replaced. */
  owed(): Row[] {
    return this.db
      .prepare(
        `SELECT d.* FROM decisions d
         WHERE d.kind = 'decide' AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
           AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
         ORDER BY d.filed_at, d.id`,
      )
      .all() as Row[];
  }

  recent(project: string, limit: number): { id: string; subject: string; kind: string; thread: string; filed_at: string; status: string }[] {
    const rows = this.db
      .prepare("SELECT * FROM decisions WHERE project = ? ORDER BY filed_at DESC, id ASC LIMIT ?")
      .all(project, limit) as Row[];
    return rows.map((d) => ({
      id: d.id,
      subject: d.subject,
      kind: d.kind,
      thread: d.thread,
      filed_at: d.filed_at as string,
      status: d.withdrawn_at
        ? "withdrawn"
        : this.replacementOf(d.id)
          ? "replaced"
          : this.store.pick(d.id)
            ? "picked"
            : d.resolved_at
              ? "resolved"
              : "open",
    }));
  }

  feed(project: string, thread: string): Feed & { text: string } {
    const f = buildFeed(this.db, project, thread, Date.parse(this.now()));
    return { ...f, text: renderFeed(f) };
  }

  /**
   * `stats` split by project (bead mk-p02ft), from the same tables and with no schema change. Filings and picks are
   * windowed by `since`; `open` and `oldest_open_age_ms` are the state now (the age is null with nothing open).
   */
  private statsByProject(since: string): Record<string, ProjectStats> {
    // No prototype: a project may be named `constructor` or `__proto__`.
    const out: Record<string, ProjectStats> = Object.create(null);
    const of = (project: string) => (out[project] ??= { filed: 0, filed_per_week: {}, picks: 0, picks_by: {}, open: 0, oldest_open_age_ms: null });
    for (const e of this.db
      .prepare("SELECT e.at AS at, d.project AS project FROM events e JOIN decisions d ON d.id = e.decision_id WHERE e.type IN ('filed','replaced') AND e.at >= ? ORDER BY e.seq")
      .all(since) as { at: string; project: string }[]) {
      const p = of(e.project);
      const w = isoWeek(Date.parse(e.at));
      p.filed++;
      p.filed_per_week[w] = (p.filed_per_week[w] ?? 0) + 1;
    }
    for (const r of this.db
      .prepare(`SELECT d.project AS project, k."by" AS by, COUNT(*) AS n FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE k.picked_at >= ? GROUP BY d.project, k."by"`)
      .all(since) as { project: string; by: string; n: number }[]) {
      const p = of(r.project);
      p.picks += r.n;
      p.picks_by[r.by] = r.n;
    }
    const now = Date.parse(this.now());
    for (const d of this.owed()) {
      const p = of(d.project as string);
      p.open++;
      const age = now - Date.parse(d.filed_at as string);
      if (p.oldest_open_age_ms === null || age > p.oldest_open_age_ms) p.oldest_open_age_ms = age;
    }
    return out;
  }

  /** The rail badge's number: Citadel cards waiting on mk (open, or shown flagged), not deleted. */
  decisionCount(): number {
    return (this.db.prepare("SELECT COUNT(*) AS n FROM cards WHERE labelled = 1 AND deleted_at IS NULL AND state IN ('open','display')").get() as { n: number }).n;
  }

  stats(since: string) {
    const count = (col: string) => {
      const out: Record<string, number> = {};
      for (const r of this.db.prepare(`SELECT ${col} AS k, COUNT(*) AS n FROM picks WHERE picked_at >= ? GROUP BY ${col}`).all(since) as { k: string; n: number }[]) out[r.k] = r.n;
      return out;
    };
    const filed_per_week: Record<string, number> = {};
    for (const e of this.db.prepare("SELECT at FROM events WHERE type IN ('filed','replaced') AND at >= ? ORDER BY seq").all(since) as { at: string }[]) {
      const w = isoWeek(Date.parse(e.at));
      filed_per_week[w] = (filed_per_week[w] ?? 0) + 1;
    }
    const lat = (
      this.db
        .prepare(`SELECT k.picked_at AS p, d.filed_at AS f FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE k."by" = 'mk' AND k.picked_at >= ?`)
        .all(since) as { p: string; f: string }[]
    )
      .map((r) => Date.parse(r.p) - Date.parse(r.f))
      .sort((a, b) => a - b);
    const p90 = lat.length ? lat[Math.max(0, Math.ceil(0.9 * lat.length) - 1)]! : null;
    const delegated = (this.db.prepare(`SELECT COUNT(*) AS n FROM picks WHERE "by" = 'vizier' AND picked_at >= ?`).get(since) as { n: number }).n;
    const over = this.db.prepare("SELECT at, decision_id FROM events WHERE type = 'overridden' AND at >= ?").all(since) as { at: string; decision_id: string }[];
    const gaps = over
      .map((e) => {
        const prev = (this.row(e.decision_id)?.supersedes ?? null) as string | null;
        const pk = prev ? this.store.pick(prev) : undefined;
        return pk ? Date.parse(e.at) - Date.parse(pk.picked_at) : null;
      })
      .filter((x): x is number => x !== null)
      .sort((a, b) => a - b);
    return {
      filed_per_week,
      per_project: this.statsByProject(since),
      picks_by: count(`"by"`),
      picks_by_surface: count("surface"),
      mk_latency_ms: { n: lat.length, median: median(lat), p90 },
      delegated,
      overrides: over.length,
      override_rate: delegated > 0 ? over.length / delegated : null,
      median_override_ms: median(gaps),
    };
  }

  // ---- cards (Task 2.4): ingest and the state machine, plan 1.3.4 ------------------------

  /** serve's project list, for bindings. Rejects when serve is down. */
  serveProjects(): Promise<ProjectInfo[]> {
    return this.deps.projects();
  }

  /**
   * Write `suggested` when the tasks project's name equals exactly one serve project name
   * (case-insensitive, with spaces, hyphens and underscores ignored: "After Them" is after-them; never fuzzy). No row otherwise; an existing row is never touched, because
   * only mk confirms or rejects a binding (plan 1.3.6).
   */
  suggestBinding(tasksProjectId: string, tasksProjectName: string, serve: ProjectInfo[]): void {
    const key = (n: string) => n.toLowerCase().replace(/[\s_-]+/g, "");
    const hits = serve.filter((p) => key(p.name) === key(tasksProjectName));
    if (hits.length !== 1) return;
    this.db
      .prepare("INSERT OR IGNORE INTO project_bindings(tasks_project_id, home_project, state, suggested_at) VALUES (?, ?, 'suggested', ?)")
      .run(tasksProjectId, hits[0]!.name, this.now());
  }

  /**
   * Record what the poller saw of each card (all of a poll's cards before any is materialized, so
   * the canonical card of a duplicated Request is order-independent). Never touches routing columns.
   */
  observeCards(tasks: readonly Task[]): void {
    const at = this.now();
    const up = this.db.prepare(
      `INSERT INTO cards(task_id, project_id, card_key, title, request_key, request_identity, created_at, updated_at, status, labelled, blocks_json, first_seen_at, last_seen_at, deleted_at)
       VALUES (@id, @project, @key, @title, @rkey, @rident, @created, @updated, @status, 1, @blocks, @at, @at, NULL)
       ON CONFLICT(task_id) DO UPDATE SET project_id = excluded.project_id, card_key = excluded.card_key, title = excluded.title,
         request_key = excluded.request_key, request_identity = excluded.request_identity, created_at = excluded.created_at,
         updated_at = excluded.updated_at, status = excluded.status, labelled = 1, blocks_json = excluded.blocks_json,
         last_seen_at = excluded.last_seen_at, deleted_at = NULL`,
    );
    this.store.atomically(() => {
      for (const t of tasks) {
        let rkey: string | null = null;
        let rident: string | null = null;
        let blocks: string | null = null;
        try {
          const c = parseCard(t.description);
          rkey = c.request.key;
          rident = c.request.identity;
          blocks = JSON.stringify(c.blocks.map((b) => b.ref));
        } catch {
          /* not a card yet: shown as display-only by ingestCard */
        }
        up.run({ id: t.id, project: t.projectId, key: t.key, title: t.title, rkey, rident, created: t.createdAt, updated: t.updatedAt, status: t.status, blocks, at });
      }
    });
  }

  /** Ids of cards that were seen labelled and open, for the poller's missing-card re-read. */
  knownOpenCards(): { task_id: string; project_id: string }[] {
    return this.db
      .prepare(`SELECT task_id, project_id FROM cards WHERE labelled = 1 AND deleted_at IS NULL AND status IN ('backlog','todo','in_progress','in_review')`)
      .all() as { task_id: string; project_id: string }[];
  }

  /**
   * mk-cggjk: open cards whose override is still mk's question and whose relabel is not pending, whatever their
   * cached label or status. knownOpenCards() skips a card cached unlabelled (cardGone keeps an override open
   * through a plain unlabel), so a later cancel or delete of that task was never read and the card stayed open
   * until a restart's sweep, which cannot see a task still cached as open. The poller re-reads these on a fresh
   * read; once cardGone closes the card it leaves this set, so the set stays bounded to live override questions.
   */
  openOverrideCards(): string[] {
    const rows = this.db.prepare("SELECT task_id FROM cards WHERE state = 'open' AND deleted_at IS NULL").all() as { task_id: string }[];
    return rows.map((r) => r.task_id).filter((id) => {
      const latest = this.currentGeneration(id);
      return !!latest && this.hasPick(id) && this.overrideOpen(latest) && !this.pendingRelabel(latest.id);
    });
  }

  /**
   * mk-cggjk: heals cards an older cardGone left stuck at state 'open' -- an unpicked, still-live
   * override on a task that is gone for good: deleted, or cached with a status outside the open set. The
   * cached label is not consulted: a pick's own unlabel+complete is cached unlabelled/done, and the override's
   * relabel (before it also reopened the task) left that cache as it was. Once status leaves the open set,
   * knownOpenCards() (and the poller's own listing) never surface the task again, so nothing would otherwise
   * revisit these rows. An override whose relabel write is still pending is skipped, exactly as cardGone does:
   * that write is about to reopen the task. Idempotent: run on every start(), a safe no-op once healed.
   */
  sweepAbandonedOverrides(): { swept: string[] } {
    const swept: string[] = [];
    const open = OPEN_STATUSES.map((s) => `'${s}'`).join(",");
    this.store.atomically(() => {
      const stuck = this.db
        .prepare(`SELECT task_id FROM cards WHERE state = 'open' AND (deleted_at IS NOT NULL OR status NOT IN (${open}))`)
        .all() as { task_id: string }[];
      for (const { task_id } of stuck) {
        const latest = this.currentGeneration(task_id);
        if (latest && this.hasPick(task_id) && this.overrideOpen(latest) && !this.pendingRelabel(latest.id)) {
          this.patchCard(task_id, { state: "closed", display_reason: "card closed in tasks" });
          swept.push(task_id);
        }
      }
    });
    return { swept };
  }

  private cardRow(taskId: string): Record<string, any> | undefined {
    return this.db.prepare("SELECT * FROM cards WHERE task_id = ?").get(taskId) as Record<string, any> | undefined;
  }
  /** The current generation is derived, never stored: max(generation). */
  currentGeneration(taskId: string): Row | undefined {
    return this.db.prepare("SELECT * FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(taskId) as Row | undefined;
  }
  private hasPick(taskId: string): boolean {
    return !!this.db.prepare("SELECT 1 FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE d.task_id = ?").get(taskId);
  }
  /** T11: the latest generation is an override of a vizier pick and is still mk's pending decision. */
  /** T10: an override generation is mk's open question; the card shows as open until mk picks. */
  reopenForOverride(taskId: string): void {
    this.patchCard(taskId, { state: "open", display_reason: null, changed_after_ruling: 0, next_check_at: null });
  }
  private overrideOpen(latest: Row): boolean {
    if (!latest.supersedes || latest.withdrawn_at || latest.resolved_at || this.store.pick(latest.id)) return false;
    // An override of a vizier pick, or a bring-back (request_id bringback:*) of any pick, is mk's open question.
    return this.store.pick(latest.supersedes)?.by === "vizier" || (latest.request_id.startsWith("bringback:") && !!this.store.pick(latest.supersedes));
  }
  /** A generation's own relabel (or bring-back) write has not yet run: it is still due to reopen the task. */
  private pendingRelabel(decisionId: string): boolean {
    return !!this.db.prepare("SELECT 1 FROM card_writes WHERE decision_id = ? AND kind = 'relabel' AND state = 'pending'").get(decisionId);
  }
  private sig(taskId: string): string {
    const c = this.cardRow(taskId);
    const g = this.currentGeneration(taskId);
    return JSON.stringify([c?.state, c?.display_reason, c?.changed_after_ruling, g?.generation, g?.withdrawn_at]);
  }
  private patchCard(taskId: string, set: Record<string, string | number | null>): void {
    const keys = Object.keys(set);
    if (keys.length === 0) return;
    this.db
      .prepare(`UPDATE cards SET ${keys.map((k) => `${k} = @${k}`).join(", ")}, updated_at = @__at WHERE task_id = @__id`)
      .run({ ...set, __at: this.now(), __id: taskId });
  }
  private isoPlus(ms: number): string {
    return new Date(Date.parse(this.now()) + ms).toISOString();
  }

  /** Ingest one observed card (T1-T5, T8, T11). Never throws for a tasks read failure: `unavailable` says so (T12). */
  async ingestCard(task: Task, ctx: { comments: () => Promise<TaskCommentRow[]> }): Promise<{ changed: boolean; unavailable?: string }> {
    if (!this.cardRow(task.id)) this.observeCards([task]);
    const before = this.sig(task.id);
    const unavailable = await this.ingestInner(task, ctx);
    const released = await this.releaseHoldOnSha(task.id, ctx.comments);
    const changed = released || before !== this.sig(task.id);
    return unavailable ? { changed, unavailable } : { changed };
  }

  private async ingestInner(task: Task, ctx: { comments: () => Promise<TaskCommentRow[]> }): Promise<string | void> {
    let card: Card | null = null;
    let parseErr = "";
    try {
      card = parseCard(task.description);
    } catch (e) {
      parseErr = message(e);
    }
    const row = this.cardRow(task.id)!;
    const latest = this.currentGeneration(task.id);
    if (!latest) return this.materializeNew(task, card, parseErr, ctx, row);

    if (this.hasPick(task.id)) {
      if (this.overrideOpen(latest)) {
        // T11. A card closed because its task went away for good (cardGone, sweepAbandonedOverrides) shows again
        // when a person reopens the task: it is listed open and labelled, and the override is still mk's question.
        if (row.state === "closed") this.patchCard(task.id, { state: "open", display_reason: null });
        return;
      }
      // T8: never a new generation after a pick.
      let changed = card === null;
      if (card) {
        try {
          changed = cardFingerprint({ title: task.title, card, thread: row.routed_thread ?? "", tasksProject: task.projectId }) !== latest.card_fp;
        } catch {
          changed = true;
        }
      }
      this.patchCard(task.id, { state: "ruled", ...(changed ? { changed_after_ruling: 1 } : {}) });
      return;
    }

    const ev = await this.evaluate(task, card, parseErr, ctx, row, latest);
    if ("unavailable" in ev) return ev.unavailable;
    if (!latest.withdrawn_at) {
      if (!ev.ok) this.invalidate(latest, ev.reason, ev.routing);
      else if (ev.fp !== latest.card_fp) this.replaceGeneration(task, ev, latest);
      else if (row.state !== "open") this.patchCard(task.id, { state: "open", display_reason: null });
    } else if (ev.ok) {
      this.reopenGeneration(task, ev, latest); // T5
    }
    // A closed card that is still invalid stays closed with its first reason.
  }

  private async evaluate(
    task: Task,
    card: Card | null,
    parseErr: string,
    ctx: { comments: () => Promise<TaskCommentRow[]> },
    row: Record<string, any>,
    latest: Row,
  ): Promise<{ ok: true; card: Card; thread: string; fp: string } | { ok: false; reason: string; routing?: boolean } | { unavailable: string }> {
    if (!card) return { ok: false, reason: shownParseError(parseErr) };
    const reg = this.db.prepare("SELECT * FROM card_requests WHERE task_id = ?").get(task.id) as { request_key: string; identity: string } | undefined;
    if (reg && (reg.request_key !== card.request.key || reg.identity !== card.request.identity)) return { ok: false, reason: "the Request line changed" };
    const project = String(card.ask.project ?? "");
    const mismatch = this.bindingMismatch(task, project);
    if (mismatch) return { ok: false, reason: mismatch };
    if (project !== latest.project || card.ask.project_root !== latest.project_root) return { ok: false, reason: "project or project_root changed" };
    const mode = card.pull === "mycroft" ? "pull" : "thread";
    if (mode !== row.routing_mode) return { ok: false, reason: `routing mode changed from ${row.routing_mode} to ${mode}`, routing: true };
    let thread = "";
    if (mode === "thread") {
      let comments: TaskCommentRow[];
      try {
        comments = await ctx.comments();
      } catch (e) {
        return { unavailable: message(e) };
      }
      thread = askingThread(comments);
      if (thread === "") return { ok: false, reason: "no agent comment names the asking thread", routing: true };
      if (thread !== row.routed_thread) return { ok: false, reason: "the asking thread changed", routing: true };
    }
    try {
      const fp = cardFingerprint({ title: task.title, card, thread, tasksProject: task.projectId });
      if (fp !== latest.card_fp || latest.withdrawn_at) await this.validateTargets(task, card, thread);
      return { ok: true, card, thread, fp };
    } catch (e) {
      return { ok: false, reason: message(e), ...(e instanceof TargetEvidenceError ? { routing: true } : {}) };
    }
  }

  /**
   * Null when the card may open. A confirmed binding must name the ask's project. A tracker project
   * with no binding, or only a name-match suggestion, fails open: the card shows in Home flagged
   * "project not bound" so a filed ask is never hidden; only mk confirms a binding. A rejected
   * binding still refuses.
   */
  private bindingMismatch(task: Task, project: string): string | null {
    const b = this.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(task.projectId) as
      | { home_project: string; state: string }
      | undefined;
    if (!b || b.state === "suggested") return null;
    if (b.state !== "rejected" && b.home_project === project) return null;
    return `project mismatch: card in ${b.home_project}, ask targets ${project}`;
  }

  private genInput(task: Task, card: Card, thread: string, fp: string, n: number, supersedes: string | null, root: { project_root: string; root_dev: string; root_ino: string; root_fsid: string | null }): DecisionInput {
    const requestId = `card:${task.id}:g${n}`;
    const ask = toV1({ pull: card.pull, ask: { ...card.ask, request_id: requestId, ...(supersedes ? { supersedes } : {}) } }, thread);
    return {
      id: `card-${task.id}-g${n}`,
      request_id: requestId,
      identity: identity(ask),
      revision: revision(ask),
      semantic_key: semanticKey(ask),
      subject: ask.subject ?? "",
      kind: ask.kind,
      ask_key: ask.ask_key,
      project: ask.project,
      project_root: root.project_root,
      root_dev: root.root_dev,
      root_ino: root.root_ino,
      root_fsid: root.root_fsid,
      asker: ask.asker,
      thread: ask.thread ?? "",
      owner_thread: null,
      body_json: deliveryJson(ask),
      supersedes,
      delegable: true,
      source: "card",
      task_id: task.id,
      generation: n,
      tasks_project_id: task.projectId,
      card_fp: fp,
      urgency: ask.urgency ?? null,
    };
  }

  private snapshotBlocks(decisionId: string, card: Card): void {
    const ins = this.db.prepare("INSERT OR IGNORE INTO decision_blocks(decision_id, ref) VALUES (?, ?)");
    for (const b of card.blocks) ins.run(decisionId, b.ref);
    const evidence = this.pickEvidence.get(card);
    if (evidence) this.store.recordEvent("on-pick-targets", decisionId, evidence);
  }

  private pickEvidence = new WeakMap<Card, { asker: TargetEvidence; targets: Record<string, TargetEvidence>; blocks: string[] }>();
  supportsOnPick(): boolean { return this.deps.readTargetEvidence !== undefined; }

  pickTargetPresentation(decisionId: string): Record<string, { target: TargetEvidence; cross_project: boolean }> {
    const row = this.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'on-pick-targets' ORDER BY seq LIMIT 1").get(decisionId) as { detail_json: string } | undefined;
    if (!row) return {};
    const snapshot = JSON.parse(row.detail_json) as { asker: TargetEvidence; targets: Record<string, TargetEvidence> };
    return Object.fromEntries(Object.entries(snapshot.targets).map(([id, target]) => [id, { target, cross_project: target.project_id !== snapshot.asker.project_id }]));
  }

  private async validateTargets(task: Task, card: Card, thread: string): Promise<void> {
    const options = (toV1(card, thread).options ?? []).filter(o => o.on_pick);
    if (!options.length) return;
    const binding = this.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(task.projectId) as { home_project: string; state: string } | undefined;
    const confirmed = binding?.state === "confirmed" && binding.home_project === card.ask.project;
    const blocks = card.blocks.map(b => b.ref);
    if (!thread || !confirmed) throw new Error("on_pick: asking thread and confirmed binding required");
    for (const o of options) {
      const target = o.on_pick!.target_thread;
      if (target !== thread && !blocks.includes(`thread:${target}`)) throw new Error("on_pick: target outside generation scope");
    }
    if (!this.deps.readTargetEvidence) throw new TargetEvidenceError("unavailable", "on_pick host target evidence");
    const asker = await this.deps.readTargetEvidence(thread);
    if (asker.thread_id !== thread || asker.archived || asker.deleted) throw new Error("on_pick: asker evidence unavailable");
    const targets: Record<string, TargetEvidence> = {};
    for (const o of options) {
      const id = o.on_pick!.target_thread;
      const target = id === thread ? asker : await this.deps.readTargetEvidence(id);
      const result = validatePickTarget({ asker: thread, askerProjectId: asker.project_id, blocks, bindingConfirmed: confirmed }, target);
      if (target.thread_id !== id || !result.ok) throw new Error(`on_pick: ${result.reason ?? "mismatched target"}`);
      targets[o.id!] = target;
    }
    this.pickEvidence.set(card, { asker, targets, blocks });
  }

  private markOpen(taskId: string): void {
    // A "not delivered" notice still waiting in the batching window would now be false: void it. The row stays, so the card is still told at most once.
    this.store.voidObligation(`ob:undelivered:${taskId}`);
    this.patchCard(taskId, { state: "open", display_reason: null, next_check_at: null, check_attempts: 0, routing_check_at: this.now() });
  }

  /**
   * "Ask / note" from the Other box: a message to the card's owner. It never rules, claims or closes anything.
   * The text is mk's, untrusted plain text, 1..OTHER_MAX characters. Idempotent per note_id.
   */
  note(target: { task_id?: string; decision_id?: string }, text: string, noteId: string, opts?: { deferWake?: false; images?: readonly ImageIn[] }): { ok: true; replay: boolean; body: string; woke: boolean; task_id: string | null } | { ok: false; status: number; error: string };
  note(target: { task_id?: string; decision_id?: string }, text: string, noteId: string, opts: { deferWake: true; images?: readonly ImageIn[] }): { ok: true; body: string; task_id: string | null; commit: (postingError?: string) => { replay: boolean; woke: boolean } } | { ok: false; status: number; error: string };
  note(target: { task_id?: string; decision_id?: string }, text: string, noteId: string, opts?: { deferWake?: boolean; images?: readonly ImageIn[] }): any {
    let notifyAfterCommit = false;
    const result = this.store.atomically(() => {
      const t = cleanOther(text);
      if (t === null) return { ok: false, status: 400, error: `a note of 1-${OTHER_MAX} characters is required` };
      if ((target.task_id === undefined) === (target.decision_id === undefined)) return { ok: false, status: 400, error: "name exactly one of task_id and decision_id" };
      type D = { id: string; owner_thread: string | null; thread: string | null; task_id: string | null };
      const d = (target.decision_id !== undefined
        ? this.db.prepare("SELECT id, owner_thread, thread, task_id FROM decisions WHERE id = ?").get(target.decision_id)
        : this.db.prepare("SELECT id, owner_thread, thread, task_id FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(target.task_id)) as D | undefined;
      if (!d) return { ok: false, status: 404, error: "no such card or ask" };
      const owner = d.owner_thread ?? d.thread ?? "";
      const key = d.task_id ?? d.id;
      const op = `note:${key}:${noteId}`;
      const imgs = validateImages(opts?.images);
      if (!imgs.ok) return { ok: false, status: 400, error: imgs.error };
      if (imgs.files.length > 0 && !this.deps.images) return { ok: false, status: 503, error: "images are not available; nothing was sent" };
      const body = `mk's note (Home, advisory; plain text):\n${t}\n\nhome-note: ${noteId}`;
      const reserved = this.store.atomically(() => {
        const prior = this.db.prepare("SELECT detail_json FROM events WHERE (decision_id = ? OR decision_id IN (SELECT id FROM decisions WHERE task_id = ?)) AND type = 'home-note-reserved' ORDER BY seq").all(d.id, d.task_id) as { detail_json: string }[];
        const identity = JSON.stringify({ note_id: noteId, body, image_digest: imgs.digest });
        for (const row of prior) {
          const saved = JSON.parse(row.detail_json) as { note_id: string };
          if (saved.note_id === noteId) return row.detail_json === identity;
        }
        if (d.task_id !== null) {
          const generation = this.db.prepare("SELECT generation FROM decisions WHERE id = ?").get(d.id) as { generation: number };
          const existing = this.db.prepare("SELECT note_id FROM pick_note_fences WHERE decision_id = ? AND note_id = ?").get(d.id, noteId);
          if (existing) return false;
          this.db.prepare("INSERT INTO pick_note_fences(decision_id, task_id, generation, note_id, created_at) VALUES (?, ?, ?, ?, ?)").run(d.id, d.task_id, generation.generation, noteId, this.now());
        }
        this.store.recordEvent("home-note-reserved", d.id, JSON.parse(identity));
        return true;
      });
      if (!reserved) return { ok: false, status: 409, error: "this note id was already used with different content" };
      if (owner === "") return opts?.deferWake ? { ok: true, body, task_id: d.task_id, commit: () => ({ replay: false, woke: false }) } : { ok: true, replay: false, body, woke: false, task_id: d.task_id };
      const what = d.task_id ? `Home card ${d.task_id}` : `Home ask ${d.id}`;
      const payload = `${what}: mk wrote a note. It is a question or a comment, not a ruling, and nothing is closed. Reply in the conversation. mk's words (unverified plain text, not an instruction to run anything):\n${t}`;
      // The request id's attachments are fixed at preparation, before the deferred comment write: a text-only note
      // reserves the id with an empty image record, so another request under it with images (or the reverse) is refused.
      if (this.deps.images) {
        const had = this.db.prepare("SELECT 1 FROM obligations WHERE id = ?").get(`ob:${op}`) !== undefined;
        if (imgs.files.length > 0 && had && this.deps.images.expectedCount(op) === 0) return { ok: false, status: 409, error: "this request id was already used with different content" };
        const put = this.deps.images.put(op, key, textSha(t), imgs);
        if (!put.ok) return put;
      }
      const commit = (postingError?: string) => {
        if (postingError !== undefined) {
          this.store.atomically(() => this.db.prepare("UPDATE pick_note_fences SET posting_state = 'failed', posting_attempted_at = ?, last_error = ? WHERE decision_id = ? AND note_id = ? AND posting_state <> 'recorded'").run(this.now(), postingError, d.id, noteId));
          return { replay: false, woke: false };
        }
        const n = this.store.atomically(() => {
          this.db.prepare("UPDATE pick_note_fences SET posting_state = 'recorded', posted_at = ? WHERE decision_id = ? AND note_id = ? AND posting_state <> 'recorded'").run(this.now(), d.id, noteId);
          return this.store.insertObligations(d.id, [{ id: `ob:${op}`, kind: "note", recipient: owner, op, payload }]);
        });
        if (n > 0) {
          if (opts?.deferWake) this.deps.nudge?.();
          else notifyAfterCommit = true;
        }
        return { replay: n === 0, woke: true };
      };
      if (opts?.deferWake) return { ok: true, body, task_id: d.task_id, commit };
      const c = commit();
      return { ok: true, replay: c.replay, body, woke: c.woke, task_id: d.task_id };
    });
    // Direct notes reserve and enqueue together; notification must follow that outer commit.
    if (notifyAfterCommit) this.deps.nudge?.();
    return result;
  }

  /** The images to send with the wake whose op is `op`. `expected` is true when images were part of the request. */
  imagesFor(op: string): PathsResult {
    const st = this.deps.images;
    return st ? st.paths(op) : { state: "none" };
  }

  /** T4: withdraw the latest generation, state closed. A routing cause enters the closed-card retry set. */
  private invalidate(latest: Row, reason: string, routing = false): void {
    this.store.atomically(() => {
      if (!latest.withdrawn_at) this.store.withdrawDecision(latest.id, reason);
      this.patchCard(String(latest.task_id), {
        state: "closed",
        display_reason: (routing ? ROUTING_PREFIX : "") + reason,
        check_attempts: 0,
        next_check_at: routing ? this.isoPlus(CLOSED_BACKOFF_MS) : null,
      });
    });
  }

  /** T3: a valid edit with a new H5 fingerprint before any pick. A refused replacement falls back to T4 in the same transaction. */
  private replaceGeneration(task: Task, ev: { card: Card; thread: string; fp: string }, latest: Row): void {
    let filed: string | null = null;
    this.store.atomically(() => {
      const n = Number(latest.generation) + 1;
      const input = this.genInput(task, ev.card, ev.thread, ev.fp, n, latest.id, { project_root: String(latest.project_root), root_dev: String(latest.root_dev), root_ino: String(latest.root_ino), root_fsid: latest.root_fsid == null ? null : String(latest.root_fsid) });
      const res = this.store.insertReplacement(input, "replace");
      if (!res.ok) {
        this.invalidate(latest, `replacement refused: ${"reason" in res ? res.reason : "request id in use"}`);
        return;
      }
      this.snapshotBlocks(input.id, ev.card);
      this.markOpen(task.id);
      filed = input.id;
    });
    // A later generation pushes only when no earlier one did (the first push failed, or there was none).
    if (filed) this.pushFor(filed);
  }

  /** T5: a closed card with no pick that is valid again opens g n+1 with no predecessor. */
  private reopenGeneration(task: Task, ev: { card: Card; thread: string; fp: string }, latest: Row): void {
    let filed: string | null = null;
    this.store.atomically(() => {
      const n = Number(latest.generation) + 1;
      const input = this.genInput(task, ev.card, ev.thread, ev.fp, n, null, { project_root: String(latest.project_root), root_dev: String(latest.root_dev), root_ino: String(latest.root_ino), root_fsid: latest.root_fsid == null ? null : String(latest.root_fsid) });
      const res = this.store.insertDecision(input);
      if (!res.inserted) throw new Error(`card generation ${input.id} request id is in use`);
      this.snapshotBlocks(input.id, ev.card);
      this.markOpen(task.id);
      filed = input.id;
    });
    if (filed) this.pushFor(filed);
  }

  private setDisplay(taskId: string, reason: string): void {
    this.patchCard(taskId, { state: "display", display_reason: reason, next_check_at: null, check_attempts: 0 });
  }

  /** T2: stay observed, back off 5 s doubling to 5 min, and say why. Returns the notice still being queued, when the second try just failed. */
  private setObserved(row: Record<string, any>, reason: string, thread = "", project = ""): Promise<void> | void {
    const attempts = Number(row.check_attempts ?? 0);
    const delay = Math.min(OBSERVED_BACKOFF_MS * 2 ** Math.min(attempts, 20), MAX_BACKOFF_MS);
    this.patchCard(row.task_id, { state: "observed", display_reason: reason, check_attempts: attempts + 1, next_check_at: this.isoPlus(delay), ...(thread !== "" ? { asking_thread: thread } : {}) });
    if (attempts + 1 >= 2) return this.tellNotDelivered(row, reason, thread || String(row.asking_thread ?? ""), project);
  }

  /**
   * After the second failed try the asking thread is told once that its card is not delivered (bead mk-j5ipa.15). It rides the
   * plugin's existing notice obligation and wake engine, so no new transport exists. The row's `op` is unique, so a later try, a restart
   * or a card that stalls again inserts nothing more. A card with no known asking thread tells the project's coordinator instead, or
   * the vizier when the project has none or several (bead mk-mn7yz); the same op keeps that to once per card too. A card is
   * undelivered only before its first generation exists, so the notice always speaks of generation 1 and a re-filed card is a new card.
   */
  private async tellNotDelivered(row: Record<string, any>, reason: string, thread: string, project: string): Promise<void> {
    const taskId = String(row.task_id);
    const op = `undelivered:${taskId}`;
    // Already told (or already queued): skip the thread lookup, which lists every thread and would run on each poll.
    if (this.store.obligation(`ob:${op}`)) return;
    const title = String(this.cardTitle(taskId) ?? "").slice(0, 120);
    let recipient = thread;
    let payload: string;
    if (thread !== "") {
      // Worded as a past event with its time: a batched digest can leave up to an hour later, after the card was delivered, and must still be true.
      payload = `At ${this.now()}, Home card ${taskId}${title ? ` (${title})` : ""} was not delivered: Home had tried twice and could not put it in front of mk (${reason}). That may have changed since: Home keeps trying, the card moves to Decide by itself once delivered, and Home tells you nothing more about it. Nothing needs to be re-filed unless the card is edited or deleted.`;
    } else {
      const to = await this.unknownAskerReceiver(project);
      if (!to) return; // none can be named now: the next poll tries again
      recipient = to.id;
      const key = this.cardKey(taskId) ?? taskId;
      const gen = Number((this.db.prepare("SELECT MAX(generation) AS g FROM decisions WHERE task_id = ?").get(taskId) as { g: number | null }).g ?? 0) + 1;
      payload = `At ${this.now()}, Home card ${key}${title ? ` (${title})` : ""}, generation ${gen}${project ? `, project ${project}` : ""}, was not delivered: Home had tried twice and could not put it in front of mk (${reason}). Home does not know which thread asked it, so you are told as ${to.role}. That may have changed since: Home keeps trying, the card moves to Decide by itself once delivered, and Home tells you nothing more about it. If it stays stuck you can claim it (comment on the card from the thread that should own it, so Home learns the asking thread), rebind its project (when the reason is a project-binding mismatch), or re-file it (edit or replace the card).`;
    }
    if (this.store.insertObligations(`card:${taskId}`, [{ id: `ob:${op}`, kind: "notice", recipient, op, payload }]) > 0) this.deps.nudge?.();
  }

  /** The one live "<Project> | coordinator" thread, else the vizier. Null when neither can be named now (including an unreadable thread list). */
  private async unknownAskerReceiver(project: string): Promise<{ id: string; role: string } | null> {
    const src = this.threadSource;
    if (!src) return null;
    try {
      if (project !== "") {
        const title = `${project} | coordinator`;
        const live = (await src.list()).filter((t) => t.archivedAt == null && t.deletedAt == null && (t.title ?? "").trim() === title);
        if (live.length === 1) return { id: live[0]!.id, role: `the ${project} coordinator` };
      }
      const v = await src.vizier();
      return v ? { id: v, role: "the vizier, because the project has no single live coordinator" } : null;
    } catch {
      return null;
    }
  }

  /** The cards held in observed, oldest first: filed but not yet delivered to Decide. Read-only; none is a decision owed. */
  notDelivered(): { id: string; subject: string; reason: string; thread: string }[] {
    const rows = this.db
      .prepare(
        `SELECT task_id, title, display_reason, asking_thread FROM cards
         WHERE state = 'observed' AND labelled = 1 AND deleted_at IS NULL AND status IN ('backlog','todo','in_progress','in_review')
         ORDER BY first_seen_at, task_id`,
      )
      .all() as { task_id: string; title: string | null; display_reason: string | null; asking_thread: string | null }[];
    return rows.map((r) => ({ id: `undelivered:${r.task_id}`, subject: r.title || r.task_id, reason: r.display_reason ?? "", thread: r.asking_thread ?? "" }));
  }

  private async materializeNew(task: Task, card: Card | null, parseErr: string, ctx: { comments: () => Promise<TaskCommentRow[]> }, row: Record<string, any>): Promise<string | void> {
    if (!card) return this.setDisplay(task.id, shownParseError(parseErr));
    // Read-only conflicts must win over a temporary root-resolution failure:
    // these cards cannot become deliverable and must not queue an undelivered notice.
    const checkRequest = () => {
      const key = card.request.key;
      const reserved = this.db.prepare("SELECT request_key, identity FROM card_requests WHERE task_id = ?").get(task.id) as { request_key: string; identity: string } | undefined;
      if (reserved && (reserved.request_key !== key || reserved.identity !== card.request.identity)) return { reason: "the Request line changed", registered: false };
      const reg = this.db.prepare("SELECT task_id, identity FROM card_requests WHERE request_key = ?").get(key) as { task_id: string; identity: string } | undefined;
      if (reg && reg.task_id !== task.id) return { reason: `duplicate Request of ${key}`, registered: false };
      if (reg && reg.identity !== card.request.identity) return { reason: "the Request line changed", registered: false };
      if (!reg) {
        const canon = this.db
          .prepare(
            `SELECT task_id FROM cards WHERE request_key = ? AND labelled = 1 AND deleted_at IS NULL
               AND status IN ('backlog','todo','in_progress','in_review') ORDER BY created_at, task_id LIMIT 1`,
          )
          .get(key) as { task_id: string } | undefined;
        if (canon && canon.task_id !== task.id) return { reason: `duplicate Request of ${key}`, registered: false };
      }

      return { reason: null, registered: !!reg };
    };
    const project = String(card.ask.project ?? "");
    const mismatch = this.bindingMismatch(task, project);
    if (mismatch) {
      // No binding row yet and serve unreachable: the binding could not be suggested, so wait.
      if (!this.db.prepare("SELECT 1 FROM project_bindings WHERE tasks_project_id = ?").get(task.projectId)) {
        try {
          await this.deps.projects();
        } catch (e) {
          return this.setObserved(row, `root unverified: ${message(e)}`, "", project);
        }
      }
      return this.setDisplay(task.id, mismatch);
    }

    // If root verification is unavailable, keep the existing undelivered notice
    // recipient without reserving an unverified request or materializing a card.
    const rootUnavailable = async (reason: string) => {
      let recipient = "";
      if (card.pull !== "mycroft") {
        try { recipient = askingThread(await ctx.comments()); } catch { /* still observed */ }
      }
      return this.setObserved(row, reason, recipient, project);
    };
    if (project === "estate") return this.setDisplay(task.id, "estate is not allowed on a card");
    const initial = checkRequest();
    if (initial.reason) return this.setDisplay(task.id, initial.reason);
    let projects: ProjectInfo[];
    try {
      projects = await this.deps.projects();
    } catch (e) {
      return rootUnavailable(`root unverified: ${message(e)}`);
    }
    const p = projects.find((x) => x.name === project);
    if (!p) return rootUnavailable(`root unverified: unknown project ${JSON.stringify(project)}`);
    if (p.root !== card.ask.project_root) return this.setDisplay(task.id, "project_root does not match the root serve resolves for this project");

    // Recheck after project resolution: another ingest may have reserved the key
    // during that await. No await separates this check from the insert.
    const current = checkRequest();
    if (current.reason) return this.setDisplay(task.id, current.reason);

    // Reserve the canonical Request before any routing read can fail or wait. A retry
    // can then find this card even when Tasks search is stale; registration does not
    // make a generation actionable. The insert survives partial routing failures.
    if (!current.registered) {
      this.db
        .prepare("INSERT INTO card_requests(request_key, task_id, identity, registered_at) VALUES (?, ?, ?, ?)")
        .run(card.request.key, task.id, card.request.identity, this.now());
    }

    let thread = "";
    if (card.pull !== "mycroft") {
      let comments: TaskCommentRow[];
      try {
        comments = await ctx.comments();
      } catch (e) {
        return message(e);
      }
      thread = askingThread(comments);
      if (thread === "") {
        const waited = Date.parse(this.now()) - Date.parse(String(row.first_seen_at));
        return this.setObserved(row, waited >= UNROUTABLE_AFTER_MS ? "unroutable" : "waiting for asking thread", "", project);
      }
    }

    let fp: string;
    let input: DecisionInput;
    try {
      await this.validateTargets(task, card, thread);
      fp = cardFingerprint({ title: task.title, card, thread, tasksProject: task.projectId });
      input = this.genInput(task, card, thread, fp, 1, null, { project_root: p.root, root_dev: String(p.dev), root_ino: String(p.ino), root_fsid: fsUuidOfDev(String(p.dev)) });
    } catch (e) {
      if (e instanceof TargetEvidenceError) return this.setObserved(row, `on_pick: ${message(e)}`, thread, project);
      return this.setDisplay(task.id, message(e));
    }
    this.store.atomically(() => {
      const res = this.store.insertDecision(input);
      if (!res.inserted) throw new Error(`card generation ${input.id} request id is in use`);
      this.snapshotBlocks(input.id, card);
      this.patchCard(task.id, {
        routing_mode: card.pull === "mycroft" ? "pull" : "thread",
        routed_thread: card.pull === "mycroft" ? null : thread,
        asking_thread: thread,
        root_state: "verified",
        root_reason: null,
      });
      this.markOpen(task.id);
    });
    this.pushFor(input.id);
  }

  /**
   * The blocker phone push (bead mk-tv70n.7). Called once a decision row is committed. It pushes only for a row with
   * urgency "blocker", and only when no row of the same ask (every generation of a card, or a supersedes chain) has
   * recorded a push: so a re-file, an edit or a supersede of a pushed ask is silent, and one whose first push failed
   * tries again. It starts the push and returns: the filing never waits for it, and nothing here throws.
   */
  private pushFor(decisionId: string): void {
    const notifier = this.deps.notifier;
    if (!notifier) return;
    let group = "";
    let req: PushRequest;
    try {
      const d = this.store.decision(decisionId) as Row | undefined;
      if (!d || d.urgency !== "blocker") return;
      const rows = askRows(this.db, decisionId);
      group = rows[0]?.id ?? decisionId;
      if (this.pushing.has(group) || !this.pushLive(d, rows) || !this.pushEligible(rows)) return;
      const taskId = d.task_id ? String(d.task_id) : null;
      // A legacy ask has no card: its key is the first generation of its supersedes chain, so every generation shares it.
      const key = this.cardKey(taskId) ?? taskId ?? group;
      const title = (this.cardTitle(taskId) || d.subject || String(readAsk(d.body_json, {}, d.source)?.question ?? "")).slice(0, 120);
      const ask = readAsk(d.body_json, {}, d.source);
      const clean = (text: string) => text.replace(/\p{Cc}/gu, " ").replace(/\s+/gu, " ").trim();
      const clip = (text: string, limit: number) => [...clean(text)].slice(0, limit).join("");
      // Reserve room for both pieces; code-point slicing never splits an astral character.
      const body = ask?.stopped
        ? `${clip(ask.question, 90)} Stopped: ${clip(ask.stopped, 200)}`
        : clip(String(ask?.question ?? ""), 300);
      req = { kind: "blocker", title, body, ...(d.thread ? { thread: d.thread } : {}), dedupeKey: key };
      this.pushing.add(group);
    } catch (e) {
      this.logPush(`push not started for ${decisionId}: ${message(e)}`);
      return;
    }
    const timeoutMs = this.deps.pushTimeoutMs ?? PUSH_TIMEOUT_MS;
    const run = (async () => {
      let timer: ReturnType<typeof setTimeout> | undefined;
      try {
        const res = await Promise.race([
          Promise.resolve().then(() => {
            // The ask may have been answered, withdrawn, resolved or superseded since the eligibility check above.
            if (!this.stillOpen(decisionId)) throw new PushSkipped();
            return notifier.notify(req);
          }),
          new Promise<never>((_, rej) => {
            timer = setTimeout(() => rej(new Error(`push timed out after ${timeoutMs} ms`)), timeoutMs);
          }),
        ]);
        if (!res || typeof res.id !== "string" || typeof res.at !== "string") throw new Error("push result has no id or at");
        this.db.prepare("UPDATE decisions SET push_sent_at = ?, push_id = ?, push_capped = ? WHERE id = ?").run(res.at, res.id, res.capped ? 1 : 0, decisionId);
        this.settle(askRows(this.db, decisionId));
      } catch (e) {
        if (e instanceof PushSkipped) return; // closed before the call started: nothing was sent and no try is spent
        this.logPush(`push for ${req.dedupeKey} failed: ${message(e)}`);
        try {
          this.db.prepare("UPDATE decisions SET push_attempts = COALESCE(push_attempts, 0) + 1, push_last_attempt_at = ? WHERE id = ?").run(this.now(), decisionId);
          this.pushEligible(askRows(this.db, decisionId)); // settles the ask when this was its last try
        } catch {
          /* the retry bookkeeping must not fail a filing */
        }
      } finally {
        if (timer) clearTimeout(timer);
        this.pushing.delete(group);
      }
    })();
    this.pushPending.add(run);
    void run.finally(() => this.pushPending.delete(run));
  }

  /**
   * Retries the pushes that failed or were lost (the plugin was missing, the call timed out, or the process died after
   * filing): every blocker ask whose latest generation is still open and whose group never recorded a push. It uses the
   * same stable dedupe key as the first try, so the notifier drops a duplicate. Bounded: at most MAX_PUSH_ATTEMPTS failed
   * tries per ask, spaced by an exponential backoff from the last failure. It starts pushes and returns; nothing here
   * blocks or fails a filing. Runs at start and on every reconcile pass.
   */
  reconcilePushes(): void {
    if (!this.deps.notifier) return;
    try {
      this.settlePicked();
      const q = this.retryQuery();
      const ids = this.db.prepare(q.sql).all(q.params) as { id: string }[];
      let started = 0;
      for (const { id } of ids) {
        if (started >= MAX_PUSHES_PER_PASS) break;
        const rows = askRows(this.db, id);
        if (rows.length === 0 || rows[rows.length - 1]!.id !== id) continue; // only the latest generation speaks for the ask
        if (!this.pushEligible(rows)) continue;
        this.pushFor(id);
        started++;
      }
    } catch (e) {
      this.logPush(`push reconciliation failed: ${message(e)}`);
    }
  }

  /**
   * Whether this row still speaks for an open ask: it is the latest generation of its ask (not superseded), is not
   * withdrawn or resolved, and has no pick. A stale replay or a retry after mk answered or withdrew must not send a
   * phone push for work already handled. Same conditions as the reconcile query.
   */
  private pushLive(d: Row, rows: { id: string }[]): boolean {
    if (rows[rows.length - 1]?.id !== d.id) return false;
    if (d.withdrawn_at || d.resolved_at) return false;
    return !this.db.prepare("SELECT 1 FROM picks WHERE decision_id = ?").get(d.id);
  }

  /** Re-reads the row and applies pushLive, for the moment just before the notifier is called. */
  private stillOpen(decisionId: string): boolean {
    const d = this.store.decision(decisionId) as Row | undefined;
    return !!d && this.pushLive(d, askRows(this.db, decisionId));
  }

  /** The retry candidates query (exposed to tests, which read its plan). */
  private retryQuery(): { sql: string; params: Record<string, string> } {
      // The database does the filtering: only asks that are eligible right now cross into JS, read through the partial
      // index decisions_blocker_pending (live, unsent, unspent blocker asks), so the pass costs in the live asks and
      // never in the historical or exhausted ones. The group rules are exact for cards (task_id) and row-level for
      // an ask filed before cards; pushEligible below is the exact check for both.
      const attempts = (alias: string) => `(SELECT COALESCE(SUM(g.push_attempts), 0) FROM decisions g WHERE g.task_id = ${alias}.task_id)`;
      const last = (alias: string) => `(SELECT MAX(g.push_last_attempt_at) FROM decisions g WHERE g.task_id = ${alias}.task_id)`;
      const now = Date.parse(this.now());
      const cutoff = (k: number) => new Date(now - Math.min(PUSH_RETRY_BASE_MS * 2 ** (k - 1), 3_600_000)).toISOString();
      const waits = Array.from({ length: MAX_PUSH_ATTEMPTS - 1 }, (_, i) => `(t.n = ${i + 1} AND t.last <= @c${i + 1})`).join(" OR ");
      const cutoffs = Object.fromEntries(Array.from({ length: MAX_PUSH_ATTEMPTS - 1 }, (_, i) => [`c${i + 1}`, cutoff(i + 1)]));
      const sql = `SELECT t.id FROM (
             SELECT d.id, d.filed_at,
                    CASE WHEN d.task_id IS NULL THEN COALESCE(d.push_attempts, 0) ELSE ${attempts("d")} END AS n,
                    CASE WHEN d.task_id IS NULL THEN d.push_last_attempt_at ELSE ${last("d")} END AS last
             FROM decisions d
             WHERE d.urgency = 'blocker' AND d.push_sent_at IS NULL AND COALESCE(d.push_attempts, 0) < ${MAX_PUSH_ATTEMPTS}
               AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
               AND NOT EXISTS (SELECT 1 FROM picks p WHERE p.decision_id = d.id)
               AND CASE WHEN d.task_id IS NULL
                        THEN NOT EXISTS (SELECT 1 FROM decisions s WHERE s.supersedes = d.id)
                        ELSE d.generation = (SELECT MAX(g.generation) FROM decisions g WHERE g.task_id = d.task_id)
                             AND NOT EXISTS (SELECT 1 FROM decisions s WHERE s.task_id = d.task_id AND s.push_sent_at IS NOT NULL) END
           ) t
           WHERE t.n < ${MAX_PUSH_ATTEMPTS} AND (t.n = 0 OR t.last IS NULL OR ${waits})
           ORDER BY t.filed_at, t.id LIMIT ${PUSH_SCAN_ROWS}`;
      return { sql, params: cutoffs };
  }

  /** Takes an ask that mk already answered (a pick) out of the retry index; it needs no push. */
  private settlePicked(): void {
    this.db
      .prepare(
        `UPDATE decisions SET push_attempts = ${MAX_PUSH_ATTEMPTS}
         WHERE urgency = 'blocker' AND push_sent_at IS NULL AND COALESCE(push_attempts, 0) < ${MAX_PUSH_ATTEMPTS} AND withdrawn_at IS NULL AND resolved_at IS NULL
           AND EXISTS (SELECT 1 FROM picks p WHERE p.decision_id = decisions.id)`,
      )
      .run();
  }

  /**
   * Settles the unpushed rows of an ask that will never be retried (spent, or another generation already pushed):
   * push_attempts goes to the limit, which takes them out of the retry index. The count is a bookkeeping value after this.
   */
  private settle(rows: { id: string }[]): void {
    if (rows.length === 0) return;
    this.db
      .prepare(`UPDATE decisions SET push_attempts = ${MAX_PUSH_ATTEMPTS} WHERE id IN (${rows.map(() => "?").join(",")}) AND push_sent_at IS NULL AND COALESCE(push_attempts, 0) < ${MAX_PUSH_ATTEMPTS}`)
      .run(...rows.map((r) => r.id));
  }

  /**
   * The one eligibility rule for a push, used by the filing and edit path (pushFor) and by the reconcile pass: no row
   * of the ask has recorded a push, fewer than MAX_PUSH_ATTEMPTS tries have failed across the ask, and the backoff
   * since the last failed try has elapsed. An ask that fails the first two tests for good is settled.
   */
  private pushEligible(rows: { id: string; push_sent_at: string | null }[]): boolean {
    if (rows.length === 0) return false;
    if (rows.some((r) => r.push_sent_at !== null)) {
      this.settle(rows);
      return false;
    }
    const tries = this.db
      .prepare(`SELECT COALESCE(SUM(push_attempts), 0) AS n, MAX(push_last_attempt_at) AS last FROM decisions WHERE id IN (${rows.map(() => "?").join(",")})`)
      .get(...rows.map((r) => r.id)) as { n: number; last: string | null };
    if (tries.n >= MAX_PUSH_ATTEMPTS) {
      this.settle(rows);
      return false;
    }
    if (tries.last) {
      const wait = Math.min(PUSH_RETRY_BASE_MS * 2 ** Math.max(0, tries.n - 1), 3_600_000);
      if (Date.parse(this.now()) < Date.parse(tries.last) + wait) return false;
    }
    return true;
  }

  private logPush(msg: string): void {
    try {
      (this.deps.log ?? ((m: string) => console.error(`autarch: ${m}`)))(msg);
    } catch {
      /* a failing logger must not fail a filing */
    }
  }

  /** Resolves when every push started so far has finished. For tests and a clean shutdown. */
  async drainPushes(): Promise<void> {
    while (this.pushPending.size > 0) await Promise.allSettled([...this.pushPending]);
  }

  /** Time to answer for the ask a decision belongs to (see pushtiming.ts), or undefined for an unknown decision. */
  timeToAnswer(decisionId: string): TimeToAnswer | undefined {
    return timeToAnswer(this.db, decisionId);
  }

  /** The blocker asks, newest first, each with its push and time to answer. */
  blockersByThread(days = 7): {thread: string; blocker_cards: number}[] {
    return blockersByThread(this.db, this.now(), days);
  }

  pushes(limit?: number): PushListing[] {
    return listPushes(this.db, limit);
  }

  /** T6/T7/T11: the card was closed, unlabelled or deleted in tasks. */
  cardGone(taskId: string, why: "closed" | "unlabelled" | "deleted", status?: string): { changed: boolean } {
    const before = this.sig(taskId);
    this.store.atomically(() => {
      const row = this.cardRow(taskId);
      if (!row) return;
      this.patchCard(taskId, {
        ...(status ? { status } : {}),
        ...(why === "closed" ? {} : { labelled: 0 }),
        ...(why === "deleted" ? { deleted_at: this.now() } : {}),
      });
      const reason = why === "closed" ? "card closed in tasks" : why === "unlabelled" ? "unlabelled" : "card deleted in tasks";
      const closedInTasks = why !== "unlabelled" || (status !== undefined && !(OPEN_STATUSES as readonly string[]).includes(status));
      const latest = this.currentGeneration(taskId);
      if (!latest) {
        this.patchCard(taskId, { state: "closed", display_reason: reason });
        return;
      }
      if (this.hasPick(taskId)) {
        if (!this.overrideOpen(latest)) {
          this.patchCard(taskId, { state: "ruled", display_reason: reason });
        } else if (closedInTasks && !this.pendingRelabel(latest.id)) {
          // T11, task gone for good: close the card so it stops showing as open forever (mk-cggjk --
          // nothing polls it again once its status leaves the open set). The override decision itself
          // stays un-withdrawn and pickable: a later pick still rules it (T9). A pending relabel write is
          // the override's own reopen-the-task action, not yet drained; its "unlabelled"/status-left-open
          // snapshot is the pick's own auto-complete echo, not a genuine external close, so it is left for
          // the relabel to resolve before this is judged again.
          this.patchCard(taskId, { state: "closed", display_reason: reason });
        } // else T11, task still open (merely unlabelled, or a relabel write is still pending): the override stays mk's open question (T10)
        return;
      }
      if (!latest.withdrawn_at) this.store.withdrawDecision(latest.id, reason);
      this.patchCard(taskId, { state: "closed", display_reason: reason, next_check_at: null });
    });
    return { changed: before !== this.sig(taskId) };
  }

  /** Observed cards whose retry time has come (the unresolved-retry set). */
  dueObserved(limit: number): string[] {
    return (
      this.db
        .prepare(`SELECT task_id FROM cards WHERE state = 'observed' AND labelled = 1 AND deleted_at IS NULL AND (next_check_at IS NULL OR next_check_at <= ?) ORDER BY next_check_at IS NOT NULL, next_check_at, task_id LIMIT ?`)
        .all(this.now(), limit) as { task_id: string }[]
    ).map((r) => r.task_id);
  }

  /** Open thread-mode cards to revalidate, the oldest check first. An override generation or a picked card is not in the set. */
  routingDue(limit: number): string[] {
    const rows = this.db
      .prepare(`SELECT task_id FROM cards WHERE state = 'open' AND routing_mode = 'thread' AND labelled = 1 AND deleted_at IS NULL ORDER BY routing_check_at IS NOT NULL, routing_check_at, task_id`)
      .all() as { task_id: string }[];
    const out: string[] = [];
    for (const r of rows) {
      const g = this.currentGeneration(r.task_id);
      if (!g || g.withdrawn_at || this.hasPick(r.task_id)) continue;
      out.push(r.task_id);
      if (out.length >= limit) break;
    }
    return out;
  }

  openThreadCardCount(): number {
    return this.routingDue(Number.MAX_SAFE_INTEGER).length;
  }

  /** Cards T4 closed for a routing cause that may reopen (T5), whose backoff has elapsed. */
  dueRoutingClosed(limit: number): string[] {
    const rows = this.db
      .prepare(
        `SELECT task_id FROM cards WHERE state = 'closed' AND display_reason LIKE '${ROUTING_PREFIX}%' AND labelled = 1 AND deleted_at IS NULL
           AND status IN ('backlog','todo','in_progress','in_review') AND (next_check_at IS NULL OR next_check_at <= ?)
         ORDER BY next_check_at IS NOT NULL, next_check_at, task_id`,
      )
      .all(this.now()) as { task_id: string }[];
    return rows.filter((r) => !this.hasPick(r.task_id)).slice(0, limit).map((r) => r.task_id);
  }

  /** Back off a routing-closed card that stayed closed after a retry: 30 s doubling to 5 min. */
  bumpClosedRetry(taskId: string): void {
    const row = this.cardRow(taskId);
    if (!row || row.state !== "closed") return;
    const attempts = Number(row.check_attempts ?? 0) + 1;
    this.patchCard(taskId, { check_attempts: attempts, next_check_at: this.isoPlus(Math.min(CLOSED_BACKOFF_MS * 2 ** Math.min(attempts, 20), MAX_BACKOFF_MS)) });
  }

  /**
   * Re-read the comments of an open thread-mode card and compare the asking thread to the frozen
   * one. A failed read changes nothing and does not count as a check (T12).
   */
  async revalidateRouting(taskId: string, comments: () => Promise<TaskCommentRow[]>): Promise<{ changed: boolean; unavailable?: string }> {
    const row = this.cardRow(taskId);
    const latest = this.currentGeneration(taskId);
    if (!row || !latest || latest.withdrawn_at || this.hasPick(taskId) || this.overrideOpen(latest) || row.routing_mode !== "thread") return { changed: false };
    let list: TaskCommentRow[];
    try {
      list = await comments();
    } catch (e) {
      return { changed: false, unavailable: message(e) };
    }
    const before = this.sig(taskId);
    const thread = askingThread(list);
    if (thread === "" || thread !== row.routed_thread) {
      this.invalidate(latest, thread === "" ? "no agent comment names the asking thread" : "the asking thread changed", true);
    } else {
      this.patchCard(taskId, { routing_check_at: this.now() });
    }
    return { changed: before !== this.sig(taskId) };
  }

}
