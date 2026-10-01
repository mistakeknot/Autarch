// The Home store: every state change is one SQLite transaction with a conditional
// write, so two connections to one file (two bb instances, a retried request) can
// never both win. There is no upsert path [C-1] [D-7].
import type Database from "better-sqlite3";
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
  asker: string;
  thread: string;
  owner_thread?: string | null;
  body_json: string;
  supersedes?: string | null;
  delegable?: boolean;
  /** Inserted in the filing's own transaction (an owner notice), so it exists exactly when the decision does. */
  obligations?: ObligationInput[];
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

export interface PickRow extends Required<Omit<PickInput, "reason" | "params_hash" | "approval">> {
  reason: string | null;
  params_hash: string | null;
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
  AND o.next_try_at <= @now
  AND o.voided_at IS NULL
  AND (o.after_id IS NULL OR EXISTS (SELECT 1 FROM obligations a WHERE a.id = o.after_id AND ${FINISHED("a")}))
  AND (o.resend_permit = 1 OR NOT EXISTS (
        SELECT 1 FROM attempts t WHERE t.obligation_id = o.id AND t.state IN ('sending','queued','uncertain')))
)`;

export class Store {
  private readonly now: () => string;
  private readonly hook: StoreHook;

  constructor(
    readonly db: Database.Database,
    opts: StoreOptions = {},
  ) {
    this.now = opts.now ?? iso;
    this.hook = opts.hook ?? (() => {});
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
                    json_object('option_id', p.option_id, 'by', p."by", 'generation', d.generation),
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

  /** Run fn in a BEGIN IMMEDIATE transaction: the write lock is taken up front. */
  private tx<T>(fn: () => T): T {
    return this.db.transaction(fn).immediate();
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
          `UPDATE obligations SET attempt = attempt + 1, last_error = @error, next_try_at = @next, updated_at = @now
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
           project, project_root, root_dev, root_ino, asker, thread, owner_thread, body_json, supersedes,
           delegable, filed_at, updated_at)
         VALUES (@id, @request_id, @identity, @revision, @semantic_key, @subject, @kind, @ask_key,
           @project, @project_root, @root_dev, @root_ino, @asker, @thread, @owner_thread, @body_json,
           @supersedes, @delegable, @at, @at)`,
      )
      .run({
        ask_key: null,
        project_root: null,
        root_dev: null,
        root_ino: null,
        owner_thread: null,
        supersedes: null,
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
  insertReplacement(d: DecisionInput, mode: "replace" | "override" = "replace"): ReplacementResult {
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
             project, project_root, root_dev, root_ino, asker, thread, owner_thread, body_json, supersedes,
             delegable, filed_at, updated_at)
           SELECT @id, @request_id, @identity, @revision, @semantic_key, @subject, @kind, @ask_key,
             @project, @project_root, @root_dev, @root_ino, @asker, @thread, @owner_thread, @body_json,
             p.id, MIN(@delegable, p.delegable), @at, @at
           ${gate}`,
        )
        .run({
          ask_key: null,
          project_root: null,
          root_dev: null,
          root_ino: null,
          owner_thread: null,
          ...d,
          prev: prevId,
          delegable: d.delegable === false ? 0 : 1,
          at,
        });
      if (ins.changes === 1) {
        this.db
          .prepare(
            `INSERT INTO mentions(decision_id, thread, request_id, at)
             SELECT ?, thread, request_id, at FROM mentions WHERE decision_id = ?`,
          )
          .run(d.id, prevId);
        this.event(mode === "replace" ? "replaced" : "overridden", d.id, { supersedes: prevId });
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
  recordPick(p: PickInput, obligations: ObligationInput[] = []): PickResult {
    const { approval, guard, ...pickParams } = p;
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
        next_try_at: o.next_try_at ?? now,
        now,
      }).changes;
      this.hook("obligation-row");
    }
    return n;
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
           next_try_at = COALESCE(@next_try_at, next_try_at),
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

  dueMessages(now: string = this.now()): ObligationRow[] {
    return this.db
      .prepare(`SELECT o.* FROM obligations o WHERE ${DUE} ORDER BY o.next_try_at, o.rowid`)
      .all({ now }) as ObligationRow[];
  }

  /** Re-send an uncertain row once per click [I-1]. */
  resend(id: string, attempt: number, clickId: string): { ok: true; replay: boolean } | { ok: false; reason: "stale" } {
    return this.tx(() => {
      const res = this.db
        .prepare(
          `UPDATE obligations SET state = 'pending', resend_permit = 1, resend_click = @click, updated_at = @now
           WHERE id = @id AND state = 'uncertain' AND attempt = @attempt AND voided_at IS NULL`,
        )
        .run({ id, attempt, click: clickId, now: this.now() });
      if (res.changes === 1) return { ok: true, replay: false } as const;
      const row = this.obligation(id);
      if (row && row.resend_click === clickId) return { ok: true, replay: true } as const;
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
           WHERE o.id = @id AND o.attempt = @attempt AND ${DUE}`,
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
      if (opts.closeOnFailure && db) {
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
      opts.log?.warn(`store not ready, retrying in ${delay} ms: ${lastError}`);
      timer = setTimeout(attempt, delay);
      delay = Math.min(delay * 2, max);
    }
  };
  attempt();

  return {
    ready: () => current !== null,
    error: () => lastError,
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
