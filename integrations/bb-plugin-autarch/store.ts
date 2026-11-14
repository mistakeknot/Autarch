// The Home store: every state change is one SQLite transaction with a conditional
// write, so two connections to one file (two bb instances, a retried request) can
// never both win. There is no upsert path [C-1] [D-7].
import type Database from "better-sqlite3";
import { BackupNotVerifiedError, QuiesceRequiredError } from "./backup.js";
import { migrate, SchemaTooNewError, type MigrateOptions } from "./migrations.js";

export type ObligationState =
  | "pending"
  | "sending"
  | "queued"
  | "uncertain"
  | "done"
  | "undeliverable"
  | "dismissed";
export type AttemptState = "sending" | "queued" | "delivered" | "not-delivered" | "uncertain";

export type PickHandlerMode = "off" | "shadow" | "live";
export type PickHandlerCell = "instruction:github-pr" | "instruction:none" | "ruling-only:github-pr" | "ruling-only:none";
export interface PickActionRow {
  pick_id: string;
  decision_id: string;
  task_id: string;
  generation: number;
  revision: string;
  card_fp: string;
  declaration_json: string | null;
  declaration_sha256: string | null;
  relay_text: string | null;
  relay_sha256: string | null;
  asker: string;
  /** References this generation's existing decision_blocks snapshot. */
  blocks_decision_id: string;
  target_thread: string | null;
  target_title: string | null;
  target_project_id: string | null;
  target_project_name: string | null;
  capture_mode: PickHandlerMode;
  capture_epoch: number;
  policy_version: number;
  captured_at: string;
  evaluation_deadline_at: string;
  evaluation_json: string | null;
  evaluation_started_at: string | null;
  evaluated_at: string | null;
  disposition: "pending" | "automatic" | "fallback" | "cancelled";
  ruling_op: string | null;
  relay_op: string | null;
  fallback_op: string | null;
  receipt_op: string | null;
  status_op: string | null;
  updated_at: string | null;
}
export interface PickNoteFenceRow {
  decision_id: string;
  task_id: string;
  generation: number;
  note_id: string;
  created_at: string;
  posting_state: "pending" | "recorded" | "failed";
  posting_attempted_at: string | null;
  posted_at: string | null;
  comment_id: string | null;
  last_error: string | null;
}
export interface PickHandlerControlRow {
  id: 1;
  mode: PickHandlerMode;
  epoch: number;
  owner_enablement_json: string | null;
  reviewed_build: string | null;
  reviewed_scope_json: string | null;
  enabled_cells_json: string;
  updated_at: string | null;
}

export interface DecisionInput {
  id: string;
  request_id: string;
  identity: string;
  revision: string;
  semantic_key: string;
  subject: string;
  kind: string;
  ask_key?: string | null;
  project: string;
  project_root?: string | null;
  root_dev?: string | null;
  root_ino?: string | null;
  root_fsid?: string | null;
  asker: string;
  thread: string;
  owner_thread?: string | null;
  body_json: string;
  supersedes?: string | null;
  delegable?: boolean;
  /** Inserted in the filing's own transaction (an owner notice), so it exists exactly when the decision does. */
  obligations?: ObligationInput[];
  /** Card generations carry their link at INSERT (plan 1.3.4); a home row leaves these unset. */
  source?: "home" | "card";
  task_id?: string | null;
  generation?: number | null;
  tasks_project_id?: string | null;
  card_fp?: string | null;
  /** "blocker" asks for a phone push at filing; null is unset. */
  urgency?: "blocker" | null;
  at?: string;
}

export interface RegistryRow {
  request_id: string;
  identity: string;
  result: "decision" | "mention";
  decision_id: string;
  thread: string | null;
  at: string;
}

export interface MentionInput {
  request_id: string;
  identity: string;
  decision_id: string;
  thread: string;
  at?: string;
}

export interface PickInput {
  decision_id: string;
  pick_id: string;
  option_id: string;
  revision: string;
  by: string;
  surface: "home" | "overlay" | "cli";
  reason?: string | null;
  picked_at?: string;
  params_hash?: string | null;
  /** Mint this record in the pick's transaction; the caller passes it only for mk's pick. */
  approval?: ApprovalMint;
  /**
   * Runs inside the pick's write transaction, before the insert, while no other writer
   * can commit. A refusal is returned as `guard` and nothing is written. Limits that
   * depend on other rows (a daily cap) belong here, not before the transaction.
   */
  guard?: () => { status: number; error: string } | null;
  capture?: () => void;
}

export type MoveState = "open" | "claimed" | "closed";

export interface MoveRow {
  task_id: string;
  generation: number;
  kind: "script" | "pr" | "read" | "context";
  payload_json: string;
  state: MoveState;
  opened_by: "card" | "pick";
  opened_at: string;
  claimed_at: string | null;
  claimed_by: string | null;
  closed_at: string | null;
  closed_by: string | null;
  evidence: string | null;
  report_state: "succeeded" | "failed" | "no-report" | null;
  report_json: string | null;
  report_at: string | null;
  report_deadline_at: string | null;
  skipped_at: string | null;
  hidden_by: string | null;
  hidden_at: string | null;
  last_checked_at: string | null;
  last_error: string | null;
}

export interface CommentInput {
  id: string;
  kind: string;
  authorName: string;
  authorId: string | null;
  body: string;
  createdAt: string;
}
export interface CommentRow {
  task_id: string;
  comment_id: string;
  kind: string;
  author_name: string;
  author_id: string | null;
  body: string;
  created_at: string;
  fetched_at: string;
}

export interface ApprovalMint {
  approval_id: string;
  kind: string;
  target: string;
  identity: string;
  expires_at: string;
}

export type ApprovalStatus = "recorded" | "expired" | "revoked" | "absent";

export interface ApprovalRow {
  approval_id: string;
  decision_id: string;
  option_id: string;
  pick_id: string;
  account: string;
  kind: string;
  target: string;
  identity: string;
  minted_at: string;
  expires_at: string;
  revoked_at: string | null;
  authorizing: number;
}

/** Read-only join for the pick handler (mk-cqnab.1 PR3). Existing rows only; no new persistent field. */
export interface PickEvidenceSnapshot {
  decision: {
    card_fp: string | null;
    id: string; request_id: string; source: string; task_id: string | null; generation: number | null; revision: string; body_json: string;
    thread: string; owner_thread: string | null; project: string; tasks_project_id: string | null; filed_at: string;
    withdrawn_at: string | null; resolved_at: string | null; superseded_by: string | null;
  };
  pick: PickRow | null;
  /** Generation n-1 of the same card, with Home's first-observation time; null when absent or for generation 1. */
  previous: { id: string; filed_at: string } | null;
  generations: { decision_id: string; pick_id: string | null }[];
  note_fences: { note_id: string; posting_state: string }[] | null;
  control: { mode: string; epoch: number; enabled_cells_json: string; owner_enablement_json: string | null } | null;
  target_snapshot_json: string | null;
  blocks: string[];
  binding: { home_project: string; state: string } | null;
  request_registered: boolean;
  move: boolean;
  vizier: string | null;
}

export interface PickRow extends Required<Omit<PickInput, "reason" | "params_hash" | "approval" | "guard" | "capture">> {
  reason: string | null;
  params_hash: string | null;
}

export interface CardWriteInput {
  id: string;
  task_id: string;
  kind: "comment" | "unlabel" | "relabel";
  payload: string;
}

export interface ObligationInput {
  id: string;
  kind: string;
  recipient?: string | null;
  payload?: string | null;
  op?: string | null;
  after_id?: string | null;
  next_try_at?: string;
}

export interface ObligationRow {
  id: string;
  decision_id: string;
  kind: string;
  state: ObligationState;
  attempt: number;
  recipient: string | null;
  payload: string | null;
  op: string | null;
  after_id: string | null;
  voided_at: string | null;
  resend_permit: number;
  resend_click: string | null;
  last_error: string | null;
  next_try_at: string;
  updated_at: string;
  allow_reroute: number;
  allow_batch: number;
  require_message_id: number;
  handler_next_try_at: string | null;
}

export interface AttemptRow {
  id: number;
  obligation_id: string;
  n: number;
  state: AttemptState;
  handle: string | null;
  evidence: string | null;
  error: string | null;
  at: string;
}

export type InsertResult =
  | { inserted: true; decision_id: string }
  | { inserted: false; existing: RegistryRow };
export type ReplacementRefusal = "not-replaceable" | "already-ruled" | "withdrawn" | "already-superseded";
export type ReplacementResult =
  | { ok: true; decision_id: string }
  | { ok: false; existing: RegistryRow }
  | { ok: false; reason: ReplacementRefusal };
export type PickRefusal = "already-ruled" | "superseded" | "withdrawn" | "stale";
export type PickResult =
  | { ok: true; pick_id: string }
  | { ok: false; reason: PickRefusal; existing?: PickRow }
  | { ok: false; reason: "guard"; refusal: { status: number; error: string } };

/** Test seam: called between statements, so a crash test can SIGKILL there. */
export type StoreHook = (step: string) => void;

export interface StoreOptions {
  now?: () => string;
  hook?: StoreHook;
  /** Milliseconds; SQLite busy_timeout. Default 2000. */
  busyTimeoutMs?: number;
  /** Milliseconds; a thread hears Home's `notice` messages at most once per window, as one digest. Default one hour; 0 turns batching off. */
  noticeWindowMs?: number;
  migrate?: MigrateOptions;
  /** bb.log: the migration outcome line and the handle's readiness messages. */
  log?: { warn(msg: string): void; info(msg: string): void };
}

const iso = () => new Date().toISOString();

/** SQL for "this obligation row is finished" (alias `a`). */
const FINISHED = (a: string) => `(
  ${a}.state IN ('done','undeliverable','dismissed','uncertain')
  OR (${a}.voided_at IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM attempts t WHERE t.obligation_id = ${a}.id AND t.state IN ('sending','queued')))
)`;

/** SQL for "this message row is due right now" (alias `o`, parameter @now). */
const DUE = `(
  o.kind <> 'ruling-file'
  AND o.state = 'pending'
  AND ((o.kind IN ('pick-relay','pick-receipt') AND o.handler_next_try_at IS NOT NULL AND o.handler_next_try_at <= @now)
    OR (o.kind NOT IN ('pick-relay','pick-receipt') AND o.next_try_at <= @now))
  AND o.voided_at IS NULL
  AND (o.after_id IS NULL OR EXISTS (SELECT 1 FROM obligations a WHERE a.id = o.after_id AND ${FINISHED("a")}))
  AND (o.resend_permit = 1 OR NOT EXISTS (
        SELECT 1 FROM attempts t WHERE t.obligation_id = o.id AND t.state IN ('sending','queued','uncertain')))
)`;

/** The message due predicate (alias `o`, parameter @now), shared with the guarded handler claim so neither branch drifts. */
export const MESSAGE_DUE_SQL = DUE;

/** Home messages that are information for another thread and not an answer or an action: these batch per recipient. */
export const BATCHED_KIND = "notice";
export const DIGEST_KIND = "notice-digest";
export const NOTICE_WINDOW_MS = 3_600_000;

export class Store {
  private readonly now: () => string;
  private readonly hook: StoreHook;
  private readonly noticeWindowMs: number;

  constructor(
    readonly db: Database.Database,
    opts: StoreOptions = {},
  ) {
    this.now = opts.now ?? iso;
    this.hook = opts.hook ?? (() => {});
    this.noticeWindowMs = opts.noticeWindowMs ?? NOTICE_WINDOW_MS;
    if (String(db.pragma("journal_mode", { simple: true })).toLowerCase() !== "wal") {
      db.pragma("journal_mode = WAL");
    }
    db.pragma("foreign_keys = ON");
    db.pragma(`busy_timeout = ${opts.busyTimeoutMs ?? 2000}`);
    migrate(db, { ...opts.migrate, log: opts.migrate?.log ?? opts.log });
    this.backfillCardWrites();
  }

  /**
   * Startup invariant (plan 1.3.8 "What stays"): every picked card generation has its comment and
   * unlabel write. A pick commits them in one transaction (Task 2.5), so this only repairs a build
   * that picked without them. Idempotent; returns the number of rows inserted.
   */
  backfillCardWrites(): number {
    const has = this.db.prepare("SELECT 1 FROM sqlite_master WHERE type='table' AND name='card_writes'").get();
    if (!has) return 0;
    const now = this.now();
    return this.tx(
      () =>
        this.db
          .prepare(
            `INSERT OR IGNORE INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at)
             SELECT 'cw-' || d.id || '-' || k.kind, d.task_id, d.id, k.kind,
                    json_object('option_id', p.option_id, 'by', p."by", 'generation', d.generation, 'picked_at', p.picked_at, 'reason', p.reason),
                    'pending', @now, @now
             FROM decisions d
             JOIN picks p ON p.decision_id = d.id
             JOIN (SELECT 'comment' AS kind UNION ALL SELECT 'unlabel') k
             WHERE d.source = 'card' AND d.task_id IS NOT NULL`,
          )
          .run({ now }).changes,
    );
  }

  get storeId(): string {
    return this.setting("store_id") as string;
  }

  setting(key: string): string | undefined {
    const row = this.db.prepare("SELECT value FROM settings_kv WHERE key = ?").get(key) as
      | { value: string }
      | undefined;
    return row?.value;
  }

  setSetting(key: string, value: string): void {
    this.db
      .prepare(
        "INSERT INTO settings_kv(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
      )
      .run(key, value);
  }

  private stampQuery: Database.Statement | undefined;

  /**
   * A token for a parsed copy of a setting, or null when no copy may be kept. While it is the same string, nothing
   * has written to the file since: `total_changes()` moves with every row this connection inserts, updates or deletes
   * (a write that is later rolled back still counts, so it is never mistaken for no change), and `data_version` moves
   * with every commit by another connection. Inside a transaction a read can see writes that may yet roll back, so
   * there is no token and the caller reads the value afresh.
   */
  settingsStamp(): string | null {
    if (this.db.inTransaction) return null;
    this.stampQuery ??= this.db.prepare("SELECT total_changes() AS c, (SELECT data_version FROM pragma_data_version) AS v");
    const r = this.stampQuery.get() as { c: number; v: number };
    return `${r.c}:${r.v}`;
  }

  /** Run fn in a BEGIN IMMEDIATE transaction: the write lock is taken up front. */
  private tx<T>(fn: () => T): T {
    return this.db.transaction(fn).immediate();
  }

  /** Run fn as one BEGIN IMMEDIATE transaction (a savepoint when already inside one). */
  atomically<T>(fn: () => T): T {
    return this.tx(fn);
  }

  /**
   * Withdraw an unpicked, unresolved decision (card T4/T6). Returns false when nothing changed:
   * unknown, already withdrawn, resolved or picked.
   */
  withdrawDecision(id: string, reason: string, at: string = this.now()): boolean {
    return this.tx(() => {
      const r = this.db
        .prepare(
          `UPDATE decisions SET withdrawn_at = @at, updated_at = @at
           WHERE id = @id AND withdrawn_at IS NULL AND resolved_at IS NULL
             AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = decisions.id)`,
        )
        .run({ id, at });
      if (r.changes === 1) this.event("withdrawn", id, { reason });
      return r.changes === 1;
    });
  }

  private event(type: string, decisionId: string | null, detail: unknown = {}): void {
    this.db
      .prepare("INSERT INTO events(at, type, decision_id, detail_json) VALUES (?, ?, ?, ?)")
      .run(this.now(), type, decisionId, JSON.stringify(detail));
  }

  /** Record an event outside any other transaction (for example a `related` note). */
  recordEvent(type: string, decisionId: string | null, detail: unknown = {}): void {
    this.event(type, decisionId, detail);
  }

  pickByPickId(pickId: string): PickRow | undefined {
    return this.db.prepare("SELECT * FROM picks WHERE pick_id = ?").get(pickId) as PickRow | undefined;
  }

  /** Pending ruling-file obligations whose retry time has come. */
  dueRulingFiles(now: string = this.now()): ObligationRow[] {
    return this.db
      .prepare(
        `SELECT * FROM obligations WHERE kind = 'ruling-file' AND state = 'pending' AND next_try_at <= ?
         ORDER BY next_try_at, rowid`,
      )
      .all(now) as ObligationRow[];
  }

  /** A failed attempt on a pending row: count it, keep the error, and set the next try. CAS on attempt. */
  failObligation(id: string, attempt: number, error: string, nextTryAt: string): boolean {
    return (
      this.db
        .prepare(
          `UPDATE obligations SET attempt = attempt + 1, last_error = @error,
             next_try_at = CASE WHEN kind IN ('pick-relay','pick-receipt') THEN next_try_at ELSE @next END,
             handler_next_try_at = CASE WHEN kind IN ('pick-relay','pick-receipt') THEN @next ELSE handler_next_try_at END,
             updated_at = @now
           WHERE id = @id AND state = 'pending' AND attempt = @attempt`,
        )
        .run({ id, attempt, error, next: nextTryAt, now: this.now() }).changes === 1
    );
  }

  /** Record that an account has seen an item (a ruling, a settings change). Idempotent. */
  markSeen(account: string, itemId: string, at: string = this.now()): void {
    this.db.prepare("INSERT OR IGNORE INTO seen(account, item_id, seen_at) VALUES (?, ?, ?)").run(account, itemId, at);
  }

  hasSeen(account: string, itemId: string): boolean {
    return !!this.db.prepare("SELECT 1 FROM seen WHERE account = ? AND item_id = ?").get(account, itemId);
  }

  registry(requestId: string): RegistryRow | undefined {
    return this.db.prepare("SELECT * FROM requests WHERE request_id = ?").get(requestId) as
      | RegistryRow
      | undefined;
  }

  decision(id: string): Record<string, unknown> | undefined {
    return this.db.prepare("SELECT * FROM decisions WHERE id = ?").get(id) as
      | Record<string, unknown>
      | undefined;
  }

  /**
   * mk-cqnab.1 PR3: one consistent read of everything the pick handler evaluates. Read-only (a deferred
   * transaction takes no write lock). `previous.filed_at` is Home's own clock at ingest of generation n-1
   * (insertDecision stamps `at ?? now()`, and card generations never pass `at`): Home's first observation.
   * Nullable members are unavailable evidence, never absence.
   */
  pickEvidenceSnapshot(decisionId: string): PickEvidenceSnapshot | null {
    return this.db.transaction((): PickEvidenceSnapshot | null => {
      const d = this.db.prepare(
        `SELECT d.id, d.request_id, d.source, d.task_id, d.generation, d.revision, d.body_json, d.thread, d.owner_thread, d.project,
                d.tasks_project_id, d.filed_at, d.withdrawn_at, d.resolved_at, d.card_fp,
                (SELECT r.id FROM decisions r WHERE r.supersedes = d.id) AS superseded_by
           FROM decisions d WHERE d.id = ?`,
      ).get(decisionId) as PickEvidenceSnapshot["decision"] | undefined;
      if (!d) return null;
      const pick = this.pick(decisionId) ?? null;
      const generation = typeof d.generation === "number" ? d.generation : null;
      const previous = d.task_id !== null && generation !== null && generation > 1
        ? (this.db.prepare("SELECT id, filed_at FROM decisions WHERE task_id = ? AND generation = ?").get(d.task_id, generation - 1) as { id: string; filed_at: string } | undefined) ?? null
        : null;
      const generations = d.task_id === null ? [] : (this.db.prepare(
        "SELECT d.id AS decision_id, k.pick_id FROM decisions d LEFT JOIN picks k ON k.decision_id = d.id WHERE d.task_id = ? ORDER BY d.generation",
      ).all(d.task_id) as { decision_id: string; pick_id: string | null }[]);
      const optional = <T>(read: () => T): T | null => { try { return read(); } catch { return null; } };
      // Note fences and control are PR2 tables: an older schema reads as unavailable, not as absent.
      const note_fences = optional(() => this.db.prepare(
        "SELECT note_id, posting_state FROM pick_note_fences WHERE decision_id = ? OR (task_id = ? AND generation = ?) ORDER BY decision_id, note_id",
      ).all(d.id, d.task_id, generation) as { note_id: string; posting_state: string }[]);
      const control = optional(() => (this.db.prepare(
        "SELECT mode, epoch, enabled_cells_json, owner_enablement_json FROM pick_handler_control WHERE id = 1",
      ).get() as PickEvidenceSnapshot["control"] | undefined) ?? null);
      const target = this.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'on-pick-targets' ORDER BY seq LIMIT 1").get(d.id) as { detail_json: string } | undefined;
      const binding = d.tasks_project_id === null ? null
        : (this.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(d.tasks_project_id) as { home_project: string; state: string } | undefined) ?? null;
      const registry = this.db.prepare("SELECT 1 FROM requests WHERE request_id = ? AND decision_id = ? AND result = 'decision'").get(d.request_id, d.id) !== undefined;
      const cardRequest = d.task_id === null ? false : this.db.prepare("SELECT 1 FROM card_requests WHERE task_id = ? LIMIT 1").get(d.task_id) !== undefined;
      const move = d.task_id === null || generation === null ? false
        : this.db.prepare("SELECT 1 FROM moves WHERE task_id = ? AND generation = ?").get(d.task_id, generation) !== undefined;
      return {
        decision: d,
        pick,
        previous,
        generations,
        note_fences,
        control,
        target_snapshot_json: target?.detail_json ?? null,
        blocks: (this.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(d.id) as { ref: string }[]).map(b => b.ref),
        binding,
        request_registered: registry && cardRequest,
        move,
        vizier: this.setting("vizierThreadId") ?? null,
      };
    }).deferred();
  }

  pick(decisionId: string): PickRow | undefined {
    return this.db.prepare(`SELECT * FROM picks WHERE decision_id = ?`).get(decisionId) as
      | PickRow
      | undefined;
  }

  mentions(decisionId: string): { decision_id: string; thread: string; request_id: string; at: string }[] {
    return this.db
      .prepare("SELECT * FROM mentions WHERE decision_id = ? ORDER BY thread")
      .all(decisionId) as never;
  }

  obligation(id: string): ObligationRow | undefined {
    return this.db.prepare("SELECT * FROM obligations WHERE id = ?").get(id) as
      | ObligationRow
      | undefined;
  }

  obligationsFor(decisionId: string): ObligationRow[] {
    return this.db
      .prepare("SELECT * FROM obligations WHERE decision_id = ? ORDER BY rowid")
      .all(decisionId) as ObligationRow[];
  }

  attempts(obligationId: string): AttemptRow[] {
    return this.db
      .prepare("SELECT * FROM attempts WHERE obligation_id = ? ORDER BY n")
      .all(obligationId) as AttemptRow[];
  }

  events(afterSeq = 0): { seq: number; at: string; type: string; decision_id: string | null; detail_json: string }[] {
    return this.db.prepare("SELECT * FROM events WHERE seq > ? ORDER BY seq").all(afterSeq) as never;
  }

  // ---- filing -------------------------------------------------------------

  /** File a decision. A request_id conflict returns the registry row and inserts nothing. */
  insertDecision(d: DecisionInput): InsertResult {
    return this.tx(() => {
      const at = d.at ?? this.now();
      const reg = this.db
        .prepare(
          `INSERT INTO requests(request_id, identity, result, decision_id, thread, at)
           VALUES (?, ?, 'decision', ?, ?, ?) ON CONFLICT(request_id) DO NOTHING`,
        )
        .run(d.request_id, d.identity, d.id, d.thread, at);
      if (reg.changes === 0) return { inserted: false, existing: this.registry(d.request_id)! };
      this.hook("registry-inserted");
      this.insertDecisionRow(d, at);
      if (d.obligations?.length) this.insertObligationRows(d.id, d.obligations, false);
      this.event("filed", d.id, { request_id: d.request_id, kind: d.kind });
      return { inserted: true, decision_id: d.id };
    });
  }

  private insertDecisionRow(d: DecisionInput, at: string): void {
    this.db
      .prepare(
        `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, ask_key,
           project, project_root, root_dev, root_ino, root_fsid, asker, thread, owner_thread, body_json, supersedes,
           delegable, filed_at, updated_at, source, task_id, generation, tasks_project_id, card_fp, urgency)
         VALUES (@id, @request_id, @identity, @revision, @semantic_key, @subject, @kind, @ask_key,
           @project, @project_root, @root_dev, @root_ino, @root_fsid, @asker, @thread, @owner_thread, @body_json,
           @supersedes, @delegable, @at, @at, @source, @task_id, @generation, @tasks_project_id, @card_fp, @urgency)`,
      )
      .run({
        ask_key: null,
        project_root: null,
        root_dev: null,
        root_ino: null,
        root_fsid: null,
        owner_thread: null,
        supersedes: null,
        source: "home",
        task_id: null,
        generation: null,
        tasks_project_id: null,
        card_fp: null,
        urgency: null,
        ...d,
        obligations: undefined,
        delegable: d.delegable === false ? 0 : 1,
        at,
      });
  }

  /** Record a mention of an existing decision. Same request_id conflict rule as insertDecision. */
  insertMention(m: MentionInput): InsertResult {
    return this.tx(() => {
      const at = m.at ?? this.now();
      const reg = this.db
        .prepare(
          `INSERT INTO requests(request_id, identity, result, decision_id, thread, at)
           VALUES (?, ?, 'mention', ?, ?, ?) ON CONFLICT(request_id) DO NOTHING`,
        )
        .run(m.request_id, m.identity, m.decision_id, m.thread, at);
      if (reg.changes === 0) return { inserted: false, existing: this.registry(m.request_id)! };
      this.db
        .prepare(
          `INSERT INTO mentions(decision_id, thread, request_id, at) VALUES (?, ?, ?, ?)
           ON CONFLICT(decision_id, thread) DO NOTHING`,
        )
        .run(m.decision_id, m.thread, m.request_id, at);
      this.event("mentioned", m.decision_id, { thread: m.thread, request_id: m.request_id });
      return { inserted: true, decision_id: m.decision_id };
    });
  }

  /**
   * Insert a replacement for d.supersedes [D-8]. Mode "replace": both are `decide`, the
   * predecessor is unpicked, unwithdrawn, unresolved, unreplaced and has the same asker,
   * thread and project. Mode "override" (Task 1.6): the predecessor instead has a
   * `by: vizier` pick.
   */
  insertReplacement(d: DecisionInput, mode: "replace" | "override" | "bringback" = "replace"): ReplacementResult {
    return this.tx(() => {
      const at = d.at ?? this.now();
      const prevId = d.supersedes;
      if (!prevId) return { ok: false, reason: "not-replaceable" } as const;
      const reg = this.db
        .prepare(
          `INSERT INTO requests(request_id, identity, result, decision_id, thread, at)
           VALUES (?, ?, 'decision', ?, ?, ?) ON CONFLICT(request_id) DO NOTHING`,
        )
        .run(d.request_id, d.identity, d.id, d.thread, at);
      if (reg.changes === 0) return { ok: false, existing: this.registry(d.request_id)! } as const;

      const pickCond =
        mode === "replace"
          ? "NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = p.id)"
          : mode === "bringback"
            ? "EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = p.id)"
            : `EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = p.id AND k."by" = 'vizier')`;
      const gate = `
        FROM decisions p
        WHERE p.id = @prev AND p.kind = 'decide' AND @kind = 'decide'
          AND p.asker = @asker AND p.thread = @thread AND p.project = @project
          AND p.withdrawn_at IS NULL AND p.resolved_at IS NULL
          AND ${pickCond}
          AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = p.id)`;
      const ins = this.db
        .prepare(
          `INSERT INTO decisions(id, request_id, identity, revision, semantic_key, subject, kind, ask_key,
             project, project_root, root_dev, root_ino, root_fsid, asker, thread, owner_thread, body_json, supersedes,
             delegable, filed_at, updated_at, source, task_id, generation, tasks_project_id, card_fp, urgency)
           SELECT @id, @request_id, @identity, @revision, @semantic_key, @subject, @kind, @ask_key,
             @project, @project_root, @root_dev, @root_ino, @root_fsid, @asker, @thread, @owner_thread, @body_json,
             p.id, MIN(@delegable, p.delegable), @at, @at, @source, @task_id, @generation, @tasks_project_id, @card_fp, @urgency
           ${gate}`,
        )
        .run({
          ask_key: null,
          project_root: null,
          root_dev: null,
          root_ino: null,
          root_fsid: null,
          owner_thread: null,
          source: "home",
          task_id: null,
          generation: null,
          tasks_project_id: null,
          card_fp: null,
          urgency: null,
          ...d,
          obligations: undefined,
          prev: prevId,
          delegable: d.delegable === false ? 0 : 1,
          at,
        });
      if (ins.changes === 1) {
        // Owner reopen paths retain the original display and scope. These are
        // historical evidence, not fresh host authority; the pick guard rereads.
        if (mode !== "replace" && d.source === "card") {
          const snapshot = this.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = 'on-pick-targets' ORDER BY seq LIMIT 1").get(prevId) as { detail_json: string } | undefined;
          if (snapshot) {
            this.db.prepare("INSERT OR IGNORE INTO decision_blocks(decision_id, ref) SELECT ?, ref FROM decision_blocks WHERE decision_id = ?").run(d.id, prevId);
            this.event("on-pick-targets", d.id, JSON.parse(snapshot.detail_json));
          }
        }
        this.db
          .prepare(
            `INSERT INTO mentions(decision_id, thread, request_id, at)
             SELECT ?, thread, request_id, at FROM mentions WHERE decision_id = ?`,
          )
          .run(d.id, prevId);
        this.event(mode === "replace" ? "replaced" : mode === "bringback" ? "brought-back" : "overridden", d.id, { supersedes: prevId });
        return { ok: true, decision_id: d.id } as const;
      }
      // Refused: undo the registry row, then say why, in this same transaction.
      this.db.prepare("DELETE FROM requests WHERE request_id = ?").run(d.request_id);
      const p = this.decision(prevId) as { withdrawn_at: string | null } | undefined;
      let reason: ReplacementRefusal = "not-replaceable";
      if (p) {
        if (p.withdrawn_at) reason = "withdrawn";
        else if (
          this.db.prepare("SELECT 1 FROM decisions WHERE supersedes = ?").get(prevId)
        )
          reason = "already-superseded";
        else if (mode === "replace" && this.pick(prevId)) reason = "already-ruled";
      }
      return { ok: false, reason } as const;
    });
  }

  /** The nearest ancestor (via `supersedes`, starting at d's predecessor) with a vizier pick [H-3]. */
  overriddenRuling(decisionId: string): { decision_id: string; pick: PickRow } | null {
    const seen = new Set<string>();
    let cur = (this.decision(decisionId) as { supersedes: string | null } | undefined)?.supersedes;
    while (cur && !seen.has(cur)) {
      seen.add(cur);
      const pick = this.pick(cur);
      if (pick && pick.by === "vizier") return { decision_id: cur, pick };
      cur = (this.decision(cur) as { supersedes: string | null } | undefined)?.supersedes;
    }
    return null;
  }

  // ---- picks --------------------------------------------------------------

  /** Insert the pick and its obligations and one event in one transaction [C-3] [C-5]. */
  recordPick(p: PickInput, obligations: ObligationInput[] = [], cardWrites: CardWriteInput[] = []): PickResult {
    const { approval, guard, capture, ...pickParams } = p;
    return this.tx((): PickResult => {
      const at = p.picked_at ?? this.now();
      if (guard && !this.pick(p.decision_id)) {
        const refusal = guard();
        if (refusal) return { ok: false, reason: "guard", refusal };
      }
      const ins = this.db
        .prepare(
          `INSERT INTO picks(decision_id, pick_id, option_id, revision, "by", surface, reason, picked_at, params_hash)
           SELECT d.id, @pick_id, @option_id, d.revision, @by, @surface, @reason, @at, @params_hash
           FROM decisions d
           WHERE d.id = @decision_id AND d.revision = @revision
             AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
             AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
             AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)`,
        )
        .run({ reason: null, params_hash: null, ...pickParams, at });
      if (ins.changes === 1) {
        this.hook("pick-inserted");
        if (approval) this.mintApproval(p, approval, at);
        this.insertObligationRows(p.decision_id, obligations, false);
        this.hook("obligations-inserted");
        this.insertCardWriteRows(p.decision_id, cardWrites);
        this.hook("card-writes-inserted");
        capture?.();
        this.hook("pick-captured");
        // T9: a picked card generation moves its card to ruled in the same transaction.
        this.db
          .prepare("UPDATE cards SET state = 'ruled', display_reason = NULL, updated_at = @at WHERE task_id = (SELECT task_id FROM decisions WHERE id = @id AND source = 'card')")
          .run({ id: p.decision_id, at });
        this.event("picked", p.decision_id, { pick_id: p.pick_id, option_id: p.option_id, by: p.by });
        this.hook("event-inserted");
        return { ok: true, pick_id: p.pick_id };
      }
      const d = this.decision(p.decision_id) as
        | { withdrawn_at: string | null; resolved_at: string | null; revision: string }
        | undefined;
      const existing = this.pick(p.decision_id);
      if (d?.withdrawn_at) return { ok: false, reason: "withdrawn" };
      if (this.db.prepare("SELECT 1 FROM decisions WHERE supersedes = ?").get(p.decision_id))
        return { ok: false, reason: "superseded" };
      if (existing || d?.resolved_at) return { ok: false, reason: "already-ruled", existing };
      return { ok: false, reason: "stale" };
    });
  }

  // ---- moves: retired; the stored rows are only read back (bead mk-j5ipa.10) ----------------

  /** Every stored move, newest first; `bb home moves --json` reads this. */
  moves(): MoveRow[] {
    return this.db.prepare("SELECT * FROM moves ORDER BY opened_at DESC, task_id, generation").all() as MoveRow[];
  }

  // ---- card conversation mirror: display only, never an input to a decision ----------------

  /** Mirrors a card's comments (insert-or-ignore on the comment id: a recorded comment is never rewritten). */
  upsertComments(taskId: string, rows: readonly CommentInput[]): number {
    const at = this.now();
    const stmt = this.db.prepare(
      `INSERT OR IGNORE INTO card_comments(task_id, comment_id, kind, author_name, author_id, body, created_at, fetched_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    );
    return this.tx(() => {
      let n = 0;
      for (const r of rows) n += stmt.run(taskId, r.id, r.kind, r.authorName, r.authorId, r.body, r.createdAt, at).changes;
      this.db
        .prepare(
          `INSERT INTO card_comment_polls(task_id, last_ok_at, last_attempt_at, last_error) VALUES (?, ?, ?, NULL)
           ON CONFLICT(task_id) DO UPDATE SET last_ok_at = excluded.last_ok_at, last_attempt_at = excluded.last_attempt_at, last_error = NULL`,
        )
        .run(taskId, at, at);
      return n;
    });
  }

  /** A failed read: remembered so the UI can say "comments may be stale". Touches nothing else. */
  markCommentsFailed(taskId: string, error: string): void {
    const at = this.now();
    this.db
      .prepare(
        `INSERT INTO card_comment_polls(task_id, last_ok_at, last_attempt_at, last_error) VALUES (?, NULL, ?, ?)
         ON CONFLICT(task_id) DO UPDATE SET last_attempt_at = excluded.last_attempt_at, last_error = excluded.last_error`,
      )
      .run(taskId, at, error.slice(0, 300));
  }

  comments(taskId: string): CommentRow[] {
    return this.db.prepare("SELECT * FROM card_comments WHERE task_id = ? ORDER BY created_at, comment_id").all(taskId) as CommentRow[];
  }

  commentPoll(taskId: string): { last_ok_at: string | null; last_attempt_at: string; last_error: string | null } | undefined {
    return this.db.prepare("SELECT last_ok_at, last_attempt_at, last_error FROM card_comment_polls WHERE task_id = ?").get(taskId) as never;
  }

  /** Where mk has read to (a `created_at#comment_id` key), or null when never opened. */
  seenThrough(taskId: string): string | null {
    return (this.db.prepare("SELECT seen_through FROM card_seen WHERE task_id = ?").get(taskId) as { seen_through: string } | undefined)?.seen_through ?? null;
  }

  /** Marks the conversation read through a key. Only moves forward. */
  markConversationSeen(taskId: string, through: string): void {
    this.db
      .prepare(
        `INSERT INTO card_seen(task_id, seen_through, seen_at) VALUES (?, ?, ?)
         ON CONFLICT(task_id) DO UPDATE SET seen_through = excluded.seen_through, seen_at = excluded.seen_at WHERE excluded.seen_through > card_seen.seen_through`,
      )
      .run(taskId, through, this.now());
  }

  private insertCardWriteRows(decisionId: string, rows: CardWriteInput[]): number {
    const now = this.now();
    const stmt = this.db.prepare(
      `INSERT OR IGNORE INTO card_writes(id, task_id, decision_id, kind, payload, state, next_try_at, updated_at)
       VALUES (@id, @task_id, @decision_id, @kind, @payload, 'pending', @now, @now)`,
    );
    let n = 0;
    for (const r of rows) n += stmt.run({ ...r, decision_id: decisionId, now }).changes;
    return n;
  }

  /** Insert card write-backs; a retry (same decision and kind) inserts none. Joins an open transaction. */
  insertCardWrites(decisionId: string, rows: CardWriteInput[]): number {
    return this.tx(() => this.insertCardWriteRows(decisionId, rows));
  }

  // ---- approvals (interim, never authorizing) -------------------------------

  private approvalEvent(type: "minted" | "checked" | "revoked", approvalId: string | null, detail: unknown): void {
    this.db
      .prepare("INSERT INTO approval_events(at, type, approval_id, detail_json) VALUES (?, ?, ?, ?)")
      .run(this.now(), type, approvalId, JSON.stringify(detail));
  }

  /** Runs inside the pick's transaction, after the pick row exists. */
  private mintApproval(p: PickInput, a: ApprovalMint, at: string): void {
    this.db
      .prepare(
        `INSERT INTO approvals(approval_id, decision_id, option_id, pick_id, account, kind, target, identity, minted_at, expires_at)
         VALUES (@approval_id, @decision_id, @option_id, @pick_id, @by, @kind, @target, @identity, @at, @expires_at)`,
      )
      .run({ ...a, decision_id: p.decision_id, option_id: p.option_id, pick_id: p.pick_id, by: p.by, at });
    this.approvalEvent("minted", a.approval_id, { decision_id: p.decision_id, kind: a.kind, target: a.target, identity: a.identity, expires_at: a.expires_at });
    this.hook("approval-minted");
  }

  /** Read the newest record for a tuple and append a `checked` event. Reads only; nothing is spent. */
  checkApproval(kind: string, target: string, identity: string): { status: ApprovalStatus; approval?: ApprovalRow } {
    return this.tx(() => {
      const row = this.db
        .prepare("SELECT * FROM approvals WHERE kind = ? AND target = ? AND identity = ? ORDER BY minted_at DESC, rowid DESC LIMIT 1")
        .get(kind, target, identity) as ApprovalRow | undefined;
      const status: ApprovalStatus = !row ? "absent" : row.revoked_at ? "revoked" : Date.parse(row.expires_at) <= Date.parse(this.now()) ? "expired" : "recorded";
      this.approvalEvent("checked", row?.approval_id ?? null, { kind, target, identity, status });
      return row ? { status, approval: row } : { status };
    });
  }

  revokeApproval(approvalId: string): { ok: true; already: boolean } | { ok: false; reason: "unknown" } {
    return this.tx(() => {
      const row = this.db.prepare("SELECT * FROM approvals WHERE approval_id = ?").get(approvalId) as ApprovalRow | undefined;
      if (!row) return { ok: false as const, reason: "unknown" as const };
      if (row.revoked_at) return { ok: true as const, already: true };
      this.db.prepare("UPDATE approvals SET revoked_at = ? WHERE approval_id = ? AND revoked_at IS NULL").run(this.now(), approvalId);
      this.approvalEvent("revoked", approvalId, { decision_id: row.decision_id });
      return { ok: true as const, already: false };
    });
  }

  /** Approvals that could still be acted on (not revoked, not expired), newest first, for Home. */
  liveApprovals(): ApprovalRow[] {
    return this.db
      .prepare("SELECT * FROM approvals WHERE revoked_at IS NULL AND expires_at > ? ORDER BY minted_at DESC, rowid DESC")
      .all(this.now()) as ApprovalRow[];
  }

  // ---- obligations --------------------------------------------------------

  private insertObligationRows(decisionId: string, rows: ObligationInput[], ignoreDup: boolean): number {
    // Handler operations need immutable-payload replay checks and their action CAS in the same
    // transaction. The legacy skip-duplicates path must never consume those identities.
    if (rows.some(o => o.kind === "pick-relay" || o.kind === "pick-receipt")) {
      throw new Error("protected obligations require guarded insertion");
    }
    const now = this.now();
    const stmt = this.db.prepare(
      `INSERT ${ignoreDup ? "OR IGNORE " : ""}INTO obligations(id, decision_id, kind, recipient, payload, op, after_id, next_try_at, updated_at)
       VALUES (@id, @decision_id, @kind, @recipient, @payload, @op, @after_id, @next_try_at, @now)`,
    );
    let n = 0;
    for (const o of rows) {
      n += stmt.run({
        recipient: null,
        payload: null,
        op: null,
        after_id: null,
        ...o,
        decision_id: decisionId,
        next_try_at: o.next_try_at ?? this.noticeSendAt(o, now),
        now,
      }).changes;
      this.hook("obligation-row");
    }
    return n;
  }

  /**
   * When a new row first goes out. A `notice` joins its recipient's open window (the latest still-future
   * time a pending notice waits for), else opens one that ends a window from now; everything else is due now.
   * The window lives in the pending rows' `next_try_at`, so a restart neither loses nor re-opens it.
   */
  private noticeSendAt(o: ObligationInput, now: string): string {
    if (o.kind !== BATCHED_KIND || !o.recipient || this.noticeWindowMs <= 0) return now;
    const open = this.db
      .prepare(
        `SELECT MIN(next_try_at) AS at FROM obligations
         WHERE kind = @kind AND recipient = @recipient AND state = 'pending' AND voided_at IS NULL AND next_try_at > @now`,
      )
      .get({ kind: BATCHED_KIND, recipient: o.recipient, now }) as { at: string | null };
    return open.at ?? new Date(Date.parse(now) + this.noticeWindowMs).toISOString();
  }

  /**
   * Hold back and merge the due information-only rows, in one transaction.
   * Spacing: a due `notice` or `notice-digest` whose recipient was sent one in the last window waits until that
   * window is over (this also covers a delayed drain, and digests retargeted to one successor).
   * One per pass: of several due rows for one recipient only the first stays due.
   * Merging: the due `notice` rows that share a recipient become one `notice-digest` row (exactly one message),
   * and the members are done with their text carried by the digest, so a crash leaves either the members or the
   * digest, never both and never neither. A lone due notice goes out as it is.
   * `skip` keeps a row out of a digest (it goes alone, still spaced). Returns the digests created.
   */
  batchDueNotices(skip: (row: ObligationRow) => boolean = () => false, now: string = this.now()): ObligationRow[] {
    if (this.noticeWindowMs <= 0) return [];
    return this.tx(() => {
      const classOf = (r: ObligationRow) => (r.kind === BATCHED_KIND || r.kind === DIGEST_KIND) && !!r.recipient && r.resend_permit !== 1;
      const lastSent = this.db.prepare(
        `SELECT MAX(a.at) AS at FROM attempts a JOIN obligations o ON o.id = a.obligation_id
         WHERE o.kind IN ('${BATCHED_KIND}','${DIGEST_KIND}') AND o.recipient = ? AND a.state IN ('sending','queued','uncertain','delivered')`,
      );
      const hold = this.db.prepare("UPDATE obligations SET next_try_at = @at, updated_at = @now WHERE id = @id AND state = 'pending'");
      const due: ObligationRow[] = [];
      for (const r of this.dueMessages(now).filter(classOf)) {
        const last = (lastSent.get(r.recipient!) as { at: string | null }).at;
        const free = last ? new Date(Date.parse(last) + this.noticeWindowMs).toISOString() : null;
        if (free && free > now) hold.run({ id: r.id, at: free, now });
        else due.push(r);
      }
      const byThread = new Map<string, ObligationRow[]>();
      for (const r of due) if (r.kind === BATCHED_KIND && !skip(r)) byThread.set(r.recipient!, [...(byThread.get(r.recipient!) ?? []), r]);
      const made: ObligationRow[] = [];
      for (const [recipient, members] of byThread) {
        if (members.length < 2) continue;
        const lead = members[0]!;
        const op = `${DIGEST_KIND}:${recipient}:${lead.id}`;
        const payload = [`${members.length} Home notices for this thread, batched (Home sends a thread at most one such message per hour):`, ...members.map((m, i) => `${i + 1}. ${m.payload ?? ""}`)].join("\n\n");
        this.insertObligationRows(lead.decision_id, [{ id: `ob:${op}`, kind: DIGEST_KIND, recipient, payload, op, next_try_at: now }], true);
        for (const m of members) {
          this.db
            .prepare("UPDATE obligations SET state = 'done', last_error = @note, updated_at = @now WHERE id = @id AND state = 'pending'")
            .run({ id: m.id, note: `batched into ob:${op}`, now });
        }
        this.hook("notice-batched");
        made.push(this.obligation(`ob:${op}`)!);
      }
      // One message per recipient per pass: a second due row for a recipient (two digests retargeted to one successor,
      // or a notice kept out of a digest) waits a window, and the pass after the first send holds it to that send.
      const seen = new Set<string>();
      for (const r of this.dueMessages(now).filter(classOf)) {
        if (!seen.has(r.recipient!)) seen.add(r.recipient!);
        else hold.run({ id: r.id, at: new Date(Date.parse(now) + this.noticeWindowMs).toISOString(), now });
      }
      return made;
    });
  }

  /** Insert a recipient set atomically; a retry (same `op`) inserts none [F-6]. Returns rows inserted. */
  insertObligations(decisionId: string, rows: ObligationInput[]): number {
    return this.tx(() => this.insertObligationRows(decisionId, rows, true));
  }

  /**
   * Conditional state change [E-2] [G-2]. Reports a lost race when the row is not in
   * `from` at `attempt`. `done` and `dismissed` are never left, a voided row never
   * enters pending or sending, and reconciliation never derives `uncertain` over a
   * resend permit [H-1].
   */
  transition(
    id: string,
    from: ObligationState,
    to: ObligationState,
    attempt: number,
    fields: { last_error?: string | null; next_try_at?: string } = {},
  ): { ok: true } | { ok: false; reason: "lost-race" | "final" | "voided" | "permit" } {
    if (from === "done" || from === "dismissed") return { ok: false, reason: "final" };
    const res = this.db
      .prepare(
        `UPDATE obligations SET state = @to, updated_at = @now,
           last_error = COALESCE(@last_error, last_error),
           next_try_at = CASE WHEN kind IN ('pick-relay','pick-receipt') THEN next_try_at
             ELSE COALESCE(@next_try_at, next_try_at) END,
           handler_next_try_at = CASE WHEN kind IN ('pick-relay','pick-receipt')
             THEN COALESCE(@next_try_at, handler_next_try_at) ELSE handler_next_try_at END,
           resend_permit = CASE WHEN @to IN ('done','dismissed') THEN 0 ELSE resend_permit END
         WHERE id = @id AND state = @from AND attempt = @attempt
           AND NOT (voided_at IS NOT NULL AND @to IN ('pending','sending'))
           AND NOT (resend_permit = 1 AND @to = 'uncertain')`,
      )
      .run({
        id,
        from,
        to,
        attempt,
        now: this.now(),
        last_error: fields.last_error ?? null,
        next_try_at: fields.next_try_at ?? null,
      });
    if (res.changes === 1) return { ok: true };
    const row = this.obligation(id);
    if (row && row.state === from && row.attempt === attempt) {
      if (row.voided_at && (to === "pending" || to === "sending")) return { ok: false, reason: "voided" };
      if (row.resend_permit === 1 && to === "uncertain") return { ok: false, reason: "permit" };
    }
    return { ok: false, reason: "lost-race" };
  }

  /** Point a message row at another thread after its recipient was archived; only while it still names `from`. */
  retarget(id: string, from: string, to: string): boolean {
    return this.db.prepare("UPDATE obligations SET recipient = @to, updated_at = @now WHERE id = @id AND recipient = @from AND allow_reroute = 1 AND state NOT IN ('done','dismissed')").run({ id, from, to, now: this.now() }).changes === 1;
  }

  /** Suppress a row without changing its state [G-2]; also clears any resend permit. */
  voidObligation(id: string): boolean {
    return (
      this.db
        .prepare(
          `UPDATE obligations SET voided_at = @now, resend_permit = 0, updated_at = @now
           WHERE id = @id AND voided_at IS NULL`,
        )
        .run({ id, now: this.now() }).changes === 1
    );
  }

  /** Dismiss (the operator gives up on a row): terminal, clears the permit. */
  dismiss(id: string): boolean {
    return (
      this.db
        .prepare(
          `UPDATE obligations SET state = 'dismissed', resend_permit = 0, updated_at = @now
           WHERE id = @id AND state NOT IN ('done','dismissed')`,
        )
        .run({ id, now: this.now() }).changes === 1
    );
  }

  /** Batch and space the information-only rows, then the rows due at that same instant (a row that falls due between the two calls waits for the next pass). */
  dueMessagesBatched(skip?: (row: ObligationRow) => boolean): ObligationRow[] {
    const now = this.now();
    this.batchDueNotices(skip, now);
    return this.dueMessages(now);
  }

  dueMessages(now: string = this.now()): ObligationRow[] {
    return this.db
      .prepare(`SELECT o.* FROM obligations o WHERE ${DUE}
        ORDER BY CASE WHEN o.kind IN ('pick-relay','pick-receipt') THEN o.handler_next_try_at ELSE o.next_try_at END, o.rowid`)
      .all({ now }) as ObligationRow[];
  }

  /** Re-send an uncertain row once per click [I-1]. */
  resend(id: string, attempt: number, clickId: string): { ok: true; replay: boolean } | { ok: false; reason: "stale" } {
    return this.tx(() => {
      const res = this.db
        .prepare(
          `UPDATE obligations SET state = 'pending', resend_permit = 1, resend_click = @click, updated_at = @now
           WHERE id = @id AND state = 'uncertain' AND attempt = @attempt AND voided_at IS NULL
             AND kind NOT IN ('pick-relay','pick-receipt')`,
        )
        .run({ id, attempt, click: clickId, now: this.now() });
      if (res.changes === 1) return { ok: true, replay: false } as const;
      const row = this.obligation(id);
      if (row && row.kind !== "pick-relay" && row.kind !== "pick-receipt" && row.resend_click === clickId) return { ok: true, replay: true } as const;
      return { ok: false, reason: "stale" } as const;
    });
  }

  /**
   * Claim a due row for sending [H-2]: re-checks the whole due predicate in the UPDATE,
   * inserts attempt n+1, sets `sending` and clears the permit. Send only on ok.
   */
  claim(id: string, attempt: number, now: string = this.now()): { ok: true; attempt: number } | { ok: false } {
    return this.tx(() => {
      const res = this.db
        .prepare(
          `UPDATE obligations AS o SET state = 'sending', attempt = o.attempt + 1, resend_permit = 0, updated_at = @now
           WHERE o.id = @id AND o.attempt = @attempt AND o.kind NOT IN ('pick-relay','pick-receipt') AND ${DUE}`,
        )
        .run({ id, attempt, now });
      if (res.changes !== 1) return { ok: false } as const;
      const n = attempt + 1;
      this.db
        .prepare("INSERT INTO attempts(obligation_id, n, state, at) VALUES (?, ?, 'sending', ?)")
        .run(id, n, now);
      return { ok: true, attempt: n } as const;
    });
  }

  /** Move an attempt on; `delivered` and `not-delivered` are never left [F-2]. */
  updateAttempt(
    obligationId: string,
    n: number,
    to: AttemptState,
    fields: { handle?: string | null; evidence?: string | null; error?: string | null } = {},
  ): boolean {
    return (
      this.db
        .prepare(
          `UPDATE attempts SET state = @to,
             handle = COALESCE(@handle, handle), evidence = COALESCE(@evidence, evidence), error = COALESCE(@error, error)
           WHERE obligation_id = @oid AND n = @n AND state NOT IN ('delivered','not-delivered')`,
        )
        .run({
          to,
          oid: obligationId,
          n,
          handle: fields.handle ?? null,
          evidence: fields.evidence ?? null,
          error: fields.error ?? null,
        }).changes === 1
    );
  }

  /** Remember a dispatch or cancel event that arrived before the send response bound the row [D-9]. */
  recordQueueEvent(queuedRow: string, type: string, at: string = this.now()): void {
    this.db
      .prepare("INSERT INTO queue_events(queued_row, type, at) VALUES (?, ?, ?) ON CONFLICT(queued_row) DO NOTHING")
      .run(queuedRow, type, at);
  }

  queueEvent(queuedRow: string): { queued_row: string; type: string; at: string } | undefined {
    return this.db.prepare("SELECT * FROM queue_events WHERE queued_row = ?").get(queuedRow) as never;
  }

  /** Clean stop: fold the WAL into data.db and truncate it, so a refused older build sees an untouched file. */
  close(): void {
    try {
      this.db.pragma("wal_checkpoint(TRUNCATE)");
    } finally {
      this.db.close();
    }
  }
}

// ---- degraded, retrying initialization [D-10] ---------------------------------

export interface StoreHandleOptions extends StoreOptions {
  /** First retry delay; doubles up to maxDelayMs. Default 5000 / 60000. */
  initialDelayMs?: number;
  maxDelayMs?: number;
  /** Close a database that failed to open a store (use when open() makes fresh connections). */
  closeOnFailure?: boolean;
}

export interface StoreHandle {
  ready(): boolean;
  error(): string | null;
  /** A refused migration (quiesce or unverified backup): permanent, no retry. The factory must throw it so bb keeps the previous instance. */
  refusal(): Error | null;
  /** The open store; throws while not ready. */
  store(): Store;
  onReady(fn: (s: Store) => void): void;
  dispose(): void;
}

/**
 * Never throws on a locked or busy database: the factory keeps its surfaces registered
 * in a not-ready state and retries, because releasing a lock does not reload a failed
 * factory. A schema newer than this code supports stays not-ready with its error.
 */
export function createStoreHandle(open: () => Database.Database, opts: StoreHandleOptions = {}): StoreHandle {
  let current: Store | null = null;
  let lastError: string | null = null;
  let refusal: Error | null = null;
  let timer: ReturnType<typeof setTimeout> | null = null;
  let disposed = false;
  let delay = opts.initialDelayMs ?? 5000;
  const max = opts.maxDelayMs ?? 60000;
  const listeners: ((s: Store) => void)[] = [];

  const attempt = () => {
    timer = null;
    if (disposed) return;
    let db: Database.Database | null = null;
    try {
      db = open();
      current = new Store(db, opts);
      lastError = null;
      opts.log?.info("store ready");
      for (const fn of listeners) fn(current);
      return;
    } catch (e) {
      lastError = e instanceof Error ? e.message : String(e);
      if (opts.closeOnFailure && db && !current) {
        try {
          db.close();
        } catch {
          /* already closed */
        }
      }
      if (e instanceof SchemaTooNewError) {
        opts.log?.warn(`store not ready (permanent until the plugin is upgraded): ${lastError}`);
        return;
      }
      if (e instanceof QuiesceRequiredError || e instanceof BackupNotVerifiedError) {
        // Not retried: each retry would run a fresh VACUUM INTO, and the plan says the plugin does not start (1.3.8, 8.9).
        refusal = e;
        opts.log?.warn(`store migration refused, plugin not started: ${lastError}`);
        return;
      }
      opts.log?.warn(`store not ready, retrying in ${delay} ms: ${lastError}`);
      timer = setTimeout(attempt, delay);
      delay = Math.min(delay * 2, max);
    }
  };
  attempt();

  return {
    ready: () => current !== null,
    error: () => lastError,
    refusal: () => refusal,
    store() {
      if (!current) throw new Error(`store not ready${lastError ? `: ${lastError}` : ""}`);
      return current;
    },
    onReady(fn) {
      if (current) fn(current);
      else listeners.push(fn);
    },
    dispose() {
      disposed = true;
      if (timer) clearTimeout(timer);
    },
  };
}
