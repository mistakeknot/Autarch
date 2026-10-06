// The wake loop [A-15] [B-4] [C-8] [F-1] [F-4] [G-1]: delivers the message obligations
// (wakes, notices) into bb threads, exactly one message per row, and reconciles every
// attempt whose outcome is not known. A failure never returns a row to pending unless the
// request provably never reached the server; everything else is an unknown outcome that is
// reconciled against the recipient's queued rows and accepted turn events, and otherwise
// left `uncertain` until mk picks Resend.
import type { AttemptRow, ObligationRow, ObligationState } from "./store.js";
import type { Service } from "./service.js";

export type SendResult = { delivery: "sent"; messageId?: string } | { delivery: "queued"; queuedMessageId: string };
export interface QueuedRow {
  id: string;
  text: string;
}
export interface AcceptedEvent {
  id: string;
  text: string;
}
export type QueuedRowState = "queued" | "dispatched" | "cancelled" | "unknown";

/** The slice of bb.sdk.threads the loop touches. Any method may throw when the host lacks it. */
export interface WakeSdk {
  send(a: { threadId: string; input: string; mode: "queue-if-active" }): Promise<SendResult>;
  queuedState(threadId: string, id: string): Promise<QueuedRowState>;
  queuedList(threadId: string): Promise<QueuedRow[]>;
  queuedDelete(threadId: string, id: string): Promise<void>;
  acceptedEvents(threadId: string): Promise<AcceptedEvent[]>;
}

/** The exact text of one attempt's message: marker plus the snapshotted payload. */
export function framed(op: string, n: number, payload: string): string {
  return `[home-msg ${op} #${n}] ${payload}`;
}

export type QueueEvent = { type: "dispatched" | "cancelled"; id: string; threadId?: string; text?: string };
export const RECONCILE_MS = 60_000;
const BACKOFF_BASE_MS = 30_000;
const BACKOFF_MAX_MS = 3_600_000;

// (e) Step 0: the SDK documents no error shapes for threads.send. Only these are treated as
// proof that the request never reached the server; everything else is an unknown outcome.
const PRE_ACCEPTANCE_CODES = new Set(["ECONNREFUSED", "ENOTFOUND", "EAI_AGAIN"]);
const GONE_CODES = new Set(["thread_archived", "thread_deleted", "thread_not_found"]);

function errInfo(err: unknown): { code: string; status: number; message: string } {
  const e = (err ?? {}) as { code?: unknown; status?: unknown; statusCode?: unknown; message?: unknown; cause?: { code?: unknown } };
  const code = typeof e.code === "string" ? e.code : typeof e.cause?.code === "string" ? e.cause.code : "";
  const status = Number(e.status ?? e.statusCode ?? 0) || 0;
  return { code, status, message: typeof e.message === "string" ? e.message : String(err) };
}
export function isGone(err: unknown): boolean {
  const i = errInfo(err);
  return GONE_CODES.has(i.code) || /thread (is )?(archived|deleted)/i.test(i.message);
}
export function isPreAcceptance(err: unknown): boolean {
  const i = errInfo(err);
  return PRE_ACCEPTANCE_CODES.has(i.code) || (i.status >= 400 && i.status < 500 && i.status !== 408 && i.status !== 429 ? true : false);
}

const sleep = (ms: number, signal?: AbortSignal) =>
  new Promise<void>((resolve) => {
    if (signal?.aborted) return resolve();
    const t = setTimeout(done, ms);
    function done() {
      clearTimeout(t);
      signal?.removeEventListener("abort", done);
      resolve();
    }
    signal?.addEventListener("abort", done, { once: true });
  });

export class WakeLoop {
  private running: Promise<void> | null = null;
  private again = false;
  private readonly inflight = new Set<string>();

  constructor(
    readonly svc: Service,
    readonly sdk: WakeSdk,
  ) {}

  private get store() {
    return this.svc.store;
  }
  private get db() {
    return this.store.db;
  }

  // ---- drains ------------------------------------------------------------------

  /** One drain at a time; a call during a drain schedules exactly one more. */
  drain(): Promise<void> {
    if (this.running) {
      this.again = true;
      return this.running;
    }
    const run = (async () => {
      try {
        do {
          this.again = false;
          await this.pass();
        } while (this.again);
      } finally {
        this.running = null;
      }
    })();
    this.running = run;
    return run;
  }
  nudge(): Promise<void> {
    return this.drain();
  }

  private async pass(): Promise<void> {
    // No await before the snapshot when there is nothing to reconcile: a row filed while a
    // send is in flight belongs to the next drain.
    const cands = this.candidates();
    if (cands.length) await this.reconcileRows(cands);
    // Rows never attempted go before a manual Resend (stable, so id order holds within each).
    const due = this.store.dueMessages();
    for (const row of [...due.filter((r) => r.resend_permit !== 1), ...due.filter((r) => r.resend_permit === 1)]) {
      const c = this.store.claim(row.id, row.attempt);
      if (!c.ok) continue;
      await this.sendOne(row, c.attempt);
    }
  }

  private async sendOne(row: ObligationRow, n: number): Promise<void> {
    const key = `${row.id}#${n}`;
    const threadId = row.recipient ?? "";
    const input = framed(row.op ?? row.id, n, row.payload ?? "");
    this.inflight.add(key);
    let res: SendResult | undefined;
    let failure: unknown;
    let failed = false;
    try {
      res = await this.sdk.send({ threadId, input, mode: "queue-if-active" });
    } catch (err) {
      failed = true;
      failure = err;
    } finally {
      this.inflight.delete(key);
    }
    if (!failed && res) {
      if (res.delivery === "sent") {
        this.store.updateAttempt(row.id, n, "delivered", { evidence: res.messageId ?? "sent" });
      } else {
        this.store.updateAttempt(row.id, n, "queued", { handle: res.queuedMessageId });
        this.applyRecordedEvent(row.id, n, res.queuedMessageId);
      }
      this.settle(row.id);
      await this.reconcileRows([row.id]);
      return;
    }
    const message = errInfo(failure).message;
    if (isGone(failure)) {
      this.store.updateAttempt(row.id, n, "not-delivered", { error: message });
      this.settle(row.id, "gone");
    } else if (isPreAcceptance(failure)) {
      this.store.updateAttempt(row.id, n, "not-delivered", { error: message });
      this.settle(row.id, "retry", message, n);
    } else {
      this.store.updateAttempt(row.id, n, "uncertain", { error: message });
      this.settle(row.id);
      await this.reconcileRows([row.id]);
    }
  }

  // ---- row state follows its attempts --------------------------------------------

  /**
   * Derive the row's state from its attempts. `hint` says what an all-not-delivered row means:
   * `gone` (removed or thread gone) is undeliverable, `retry` is pending with a delay.
   */
  private settle(id: string, hint: "gone" | "retry" | "none" = "none", error?: string, attemptN?: number): void {
    const row = this.store.obligation(id);
    if (!row || row.state === "done" || row.state === "dismissed") return;
    const atts = this.store.attempts(id);
    let to: ObligationState | undefined;
    let fields: { last_error?: string; next_try_at?: string } = {};
    if (atts.some((a) => a.state === "delivered")) to = "done";
    else if (atts.some((a) => a.state === "sending")) to = "sending";
    else if (atts.some((a) => a.state === "queued")) to = "queued";
    else if (atts.some((a) => a.state === "uncertain")) to = "uncertain";
    else if (atts.length > 0) {
      if (row.state === "pending" || row.state === "undeliverable") return;
      if (hint === "retry" && !row.voided_at) {
        to = "pending";
        const delay = Math.min(BACKOFF_BASE_MS * 2 ** Math.max(0, (attemptN ?? row.attempt) - 1), BACKOFF_MAX_MS);
        fields = { last_error: error ?? "", next_try_at: new Date(Date.parse(this.svc.time()) + delay).toISOString() };
      } else {
        to = "undeliverable";
        // A failure the card shows: the target rotated, was archived, or the send was cancelled.
        const errs = [...atts].reverse().map((a) => a.error).filter((e): e is string => !!e);
        const why = error ?? errs.find((e) => !/^cancel/i.test(e)) ?? errs[0] ?? "";
        fields = { last_error: `${why || "not delivered"} (recipient ${row.recipient ?? "?"})` };
      }
    }
    if (!to || to === row.state) return;
    // A row holding a resend permit stays pending [H-1]; the store refuses uncertain over it.
    if (row.resend_permit === 1 && to !== "done") return;
    this.store.transition(id, row.state, to, row.attempt, fields);
  }

  // ---- events ----------------------------------------------------------------------

  private applyRecordedEvent(id: string, n: number, handle: string): void {
    const ev = this.store.queueEvent(handle);
    if (!ev) return;
    if (ev.type === "dispatched") this.store.updateAttempt(id, n, "delivered", { evidence: handle });
    else if (ev.type === "cancelled") this.store.updateAttempt(id, n, "not-delivered", { error: "cancelled" });
  }

  private attemptsByHandle(handle: string): AttemptRow[] {
    return this.db.prepare("SELECT * FROM attempts WHERE handle = ?").all(handle) as AttemptRow[];
  }

  /** message.dispatched / message.cancelled: recorded first, then applied [D-9]. */
  async onQueueEvent(e: QueueEvent): Promise<void> {
    this.store.recordQueueEvent(e.id, e.type);
    for (const a of this.attemptsByHandle(e.id)) {
      if (a.state === "delivered" || a.state === "not-delivered") continue;
      this.applyRecordedEvent(a.obligation_id, a.n, e.id);
      this.settle(a.obligation_id, "gone");
    }
  }

  /** thread.archived / thread.deleted: queued attempts there are not delivered; rows undeliverable. */
  async onThreadGone(threadId: string): Promise<void> {
    const rows = this.db
      .prepare(
        `SELECT DISTINCT o.id FROM obligations o JOIN attempts a ON a.obligation_id = o.id
         WHERE o.recipient = ? AND a.state = 'queued'`,
      )
      .all(threadId) as { id: string }[];
    for (const { id } of rows) {
      for (const a of this.store.attempts(id)) if (a.state === "queued") this.store.updateAttempt(id, a.n, "not-delivered", { error: "thread gone" });
      this.settle(id, "gone");
    }
  }

  // ---- resend ------------------------------------------------------------------------

  resend(id: string, attempt: number, clickId: string) {
    return this.store.resend(id, attempt, clickId);
  }

  // ---- reconciliation [D-9] [E-5] [F-4] --------------------------------------------

  private candidates(): string[] {
    return (
      this.db
        .prepare(
          `SELECT DISTINCT o.id FROM obligations o JOIN attempts a ON a.obligation_id = o.id
           WHERE a.state IN ('sending','queued','uncertain') AND o.state <> 'dismissed' ORDER BY o.rowid`,
        )
        .all() as { id: string }[]
    ).map((r) => r.id);
  }

  async reconcile(): Promise<void> {
    await this.reconcileRows(this.candidates());
  }

  private async reconcileRows(ids: string[]): Promise<void> {
    for (const id of ids) {
      try {
        await this.reconcileRow(id);
      } catch {
        /* a failed lookup leaves the attempt as it is; the next pass tries again */
      }
    }
  }

  private async reconcileRow(id: string): Promise<void> {
    const row = this.store.obligation(id);
    if (!row || row.state === "dismissed" || !row.recipient) return;
    const thread = row.recipient;
    for (const a of this.store.attempts(id)) {
      if (a.state !== "sending" && a.state !== "queued" && a.state !== "uncertain") continue;
      if (this.inflight.has(`${id}#${a.n}`)) continue;
      const cur = this.store.obligation(id)!;
      if (cur.voided_at && a.handle && a.state === "queued") {
        await this.removeQueued(id, a, thread);
        continue;
      }
      if (a.handle) await this.checkHandle(id, a, thread);
      else await this.searchUnbound(cur, a, thread);
    }
    // A delivered attempt makes the row done and any still-queued attempt is removed.
    const after = this.store.attempts(id);
    if (after.some((a) => a.state === "delivered")) {
      for (const a of after) if (a.state === "queued" && a.handle) await this.removeQueued(id, a, thread);
    }
    this.settle(id, "gone");
  }

  private async lookup(thread: string, handle: string): Promise<QueuedRowState> {
    let st: QueuedRowState = "unknown";
    try {
      st = await this.sdk.queuedState(thread, handle);
    } catch {
      st = "unknown";
    }
    if (st !== "unknown") return st;
    const ev = this.store.queueEvent(handle);
    if (ev) return ev.type === "dispatched" ? "dispatched" : "cancelled";
    try {
      if ((await this.sdk.queuedList(thread)).some((r) => r.id === handle)) return "queued";
    } catch {
      /* unavailable */
    }
    return "unknown";
  }

  private async checkHandle(id: string, a: AttemptRow, thread: string): Promise<void> {
    const st = await this.lookup(thread, a.handle!);
    if (st === "dispatched") this.store.updateAttempt(id, a.n, "delivered", { evidence: a.handle });
    else if (st === "cancelled") this.store.updateAttempt(id, a.n, "not-delivered", { error: "cancelled" });
    else if (st === "queued") {
      if (a.state !== "queued") this.store.updateAttempt(id, a.n, "queued");
    } else {
      // The row is gone or unreadable: an accepted event with exactly the framed input is the
      // only other evidence; without it the outcome is unknown.
      const row = this.store.obligation(id)!;
      const want = framed(row.op ?? row.id, a.n, row.payload ?? "");
      try {
        const hits = (await this.sdk.acceptedEvents(thread)).filter((e) => e.text === want);
        if (hits.length === 1) {
          this.store.updateAttempt(id, a.n, "delivered", { evidence: hits[0]!.id });
          return;
        }
      } catch {
        /* unavailable */
      }
      if (a.state !== "uncertain") this.store.updateAttempt(id, a.n, "uncertain");
    }
  }

  private async searchUnbound(row: ObligationRow, a: AttemptRow, thread: string): Promise<void> {
    const want = framed(row.op ?? row.id, a.n, row.payload ?? "");
    let queued: QueuedRow[] | undefined;
    let events: AcceptedEvent[] | undefined;
    try {
      queued = (await this.sdk.queuedList(thread)).filter((r) => r.text === want);
    } catch {
      /* unavailable */
    }
    try {
      events = (await this.sdk.acceptedEvents(thread)).filter((e) => e.text === want);
    } catch {
      /* unavailable */
    }
    const q = queued ?? [];
    const e = events ?? [];
    if (q.length === 1 && e.length === 0) {
      this.store.updateAttempt(row.id, a.n, "queued", { handle: q[0]!.id });
      this.applyRecordedEvent(row.id, a.n, q[0]!.id);
    } else if (e.length === 1 && q.length === 0) {
      this.store.updateAttempt(row.id, a.n, "delivered", { evidence: e[0]!.id });
    } else if (a.state !== "uncertain") {
      this.store.updateAttempt(row.id, a.n, "uncertain");
    }
  }

  /** Remove a queued attempt's row where the host allows; a lost race is settled by reading it. */
  private async removeQueued(id: string, a: AttemptRow, thread: string): Promise<void> {
    try {
      await this.sdk.queuedDelete(thread, a.handle!);
      this.store.updateAttempt(id, a.n, "not-delivered", { error: "removed" });
      return;
    } catch {
      /* fall through: read what happened to it */
    }
    const st = await this.lookup(thread, a.handle!);
    if (st === "dispatched") this.store.updateAttempt(id, a.n, "delivered", { evidence: a.handle });
    else if (st === "cancelled") this.store.updateAttempt(id, a.n, "not-delivered", { error: "cancelled" });
  }

  // ---- lifecycle -------------------------------------------------------------------

  /**
   * Reconcile at start, then drain and reconcile every 60 s until aborted. An already
   * aborted signal runs only the startup reconciliation.
   */
  async start(signal: AbortSignal, wait: (ms: number) => Promise<void> = (ms) => sleep(ms, signal)): Promise<void> {
    await this.reconcile();
    if (signal.aborted) return;
    await this.drain();
    while (!signal.aborted) {
      await wait(RECONCILE_MS);
      if (signal.aborted) break;
      await this.drain();
    }
  }
}

// ---- the real SDK --------------------------------------------------------------------

type Part = { type?: string; text?: string };
const textOf = (parts: unknown): string =>
  Array.isArray(parts) ? (parts as Part[]).filter((p) => p?.type === "text").map((p) => p.text ?? "").join("") : "";

/** The slice of bb.sdk.threads the adapter uses. */
export interface ThreadsLike {
  send(a: { threadId: string; input: readonly { type: "text"; text: string }[]; mode?: string }): Promise<unknown>;
  queuedMessages: {
    list(a: { threadId: string }): Promise<unknown>;
    delete(a: { threadId: string; queuedMessageId: string }): Promise<unknown>;
  };
  events: { list(a: { threadId: string; types?: readonly string[]; limit?: string; order?: "asc" | "desc" }): Promise<unknown> };
}

/**
 * bb.sdk.threads as a WakeSdk. Step 0 found no read-one-queued-row call, so a row's state is
 * `queued` when the thread's list holds it and `unknown` otherwise; the loop settles unknown
 * from recorded queue events and accepted turn events.
 */
export function sdkAdapter(threads: ThreadsLike): WakeSdk {
  const list = async (threadId: string) => {
    const rows = (await threads.queuedMessages.list({ threadId })) as { id: string; content?: unknown }[];
    return rows.map((r) => ({ id: r.id, text: textOf(r.content) }));
  };
  return {
    async send(a) {
      const r = (await threads.send({ threadId: a.threadId, input: [{ type: "text", text: a.input }], mode: a.mode })) as {
        delivery: "sent" | "queued";
        queuedMessage?: { id: string };
        messageId?: string;
      };
      if (r.delivery === "queued" && r.queuedMessage?.id) return { delivery: "queued", queuedMessageId: r.queuedMessage.id };
      return { delivery: "sent", ...(r.messageId ? { messageId: r.messageId } : {}) };
    },
    async queuedState(threadId, id) {
      return (await list(threadId)).some((r) => r.id === id) ? "queued" : "unknown";
    },
    queuedList: list,
    async queuedDelete(threadId, id) {
      await threads.queuedMessages.delete({ threadId, queuedMessageId: id });
    },
    async acceptedEvents(threadId) {
      const rows = (await threads.events.list({ threadId, types: ["client/turn/requested"], limit: "500", order: "desc" })) as {
        id: string;
        data?: { input?: unknown };
      }[];
      return rows.map((r) => ({ id: r.id, text: textOf(r.data?.input) }));
    },
  };
}
