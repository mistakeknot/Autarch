import { createHash } from "node:crypto";
import { z } from "zod";
import type { Store } from "./store.js";
import type { RequestDelivery } from "./request-reminder-evidence.js";
import type { SendResult, WakeSdk } from "./wakes.js";

const PREFIX = "request-reminders:v1:request:";
const END = "request-reminders:v1:request;";
const REPLY_PREFIX = "request-reminders:v1:reply:";
const MINUTE = 60_000;
const CLOCK = { owner: [9, 27], coordinator: [14, 42] } as const;
const digest = (parts: unknown[]) => createHash("sha256").update(JSON.stringify(parts)).digest("hex");
const id = z.string().min(1).max(200).regex(/^[a-zA-Z0-9:_-]+$/);
const time = z.number().int().nonnegative();
const sourceSchema = z.object({
  id, ref: id, since: time, recipient: id, project: z.string().min(1).max(200), decision: id, task: id,
  explicitOwner: z.boolean().optional(),
}).strict();
export type ReminderRequestSource = z.infer<typeof sourceSchema>;
const deliverySchema = z.object({ id, at: time }).strict();
const sendSchema = z.object({ key: id, target: id, epoch: time,
  step: z.enum(["transfer", "reminder", "escalation"]),
  result: z.enum(["pending", "unknown", "sent", "queued", "cancelled"]),
  handle: id.optional(),
}).strict();
const recordSchema = z.object({
  source: sourceSchema,
  recipient: id,
  ref: id,
  since: time,
  epoch: time,
  transfer: z.object({ from: id, to: id, at: time }).strict().nullable(),
  delivery: deliverySchema.nullable(),
  role: z.enum(["owner", "coordinator"]).nullable(),
  answer: z.object({ actor: id, comment: id, at: time }).strict().nullable(),
  sends: z.array(sendSchema),
}).strict();
type RequestRecord = z.infer<typeof recordSchema>;
type Attempt = z.infer<typeof sendSchema>;

export interface RequestRoute {
  project: string;
  role: keyof typeof CLOCK;
  next: string | null;
}

export interface RequestReminderSources {
  now(): number;
  sources(): ReminderRequestSource[];
  eligible(source: ReminderRequestSource): boolean;
  deliver(thread: string, ref: string, since: number): Promise<RequestDelivery | null>;
  route(thread: string, project: string, explicitOwner?: boolean): Promise<RequestRoute | null>;
  reply(task: string, comment: string, actor: string, since: number): Promise<boolean>;
  enabled(source: ReminderRequestSource): boolean;
  send: WakeSdk["send"];
  removeQueued: WakeSdk["queuedDelete"];
  queuedList?: WakeSdk["queuedList"];
}

/** Uses Home's own durable settings table with private, versioned rows. Every
 * state mutation is serialized by its SQLite transaction. Awaited observations
 * can change a record only if the same ownership epoch is still current.
 */
export class RequestReminders {
  private cursor = PREFIX;
  constructor(private readonly store: Store, private readonly s: RequestReminderSources) {}

  private get(request: string): RequestRecord | null {
    const raw = this.store.setting(PREFIX + request);
    return raw === undefined ? null : recordSchema.parse(JSON.parse(raw));
  }

  private put(record: RequestRecord): void {
    this.store.setSetting(PREFIX + record.source.id, JSON.stringify(record));
  }

  private durable<T>(write: () => T): T {
    // FULL protects a pre-send claim even across a host power loss. Switch
    // only for this synchronous transaction on Home's shared connection.
    const db = this.store.db;
    const synchronous = db.pragma("synchronous", { simple: true }) as number;
    db.pragma("synchronous = FULL");
    try { return db.transaction(write).immediate(); }
    finally { db.pragma(`synchronous = ${synchronous}`); }
  }

  private rows(): RequestRecord[] {
    // Keyset pagination visits a bounded batch even with old answered rows.
    const rows = this.store.db.prepare(
      `SELECT key, value FROM settings_kv WHERE key > ? AND key < ?
        AND (json_extract(value, '$.answer') IS NULL
          OR EXISTS (SELECT 1 FROM json_each(settings_kv.value, '$.sends') WHERE
                     json_extract(json_each.value, '$.result') IN ('queued', 'unknown', 'pending')))
        ORDER BY key LIMIT 100`,
    ).all(this.cursor, END) as { key: string; value: string }[];
    this.cursor = rows.length === 100 ? rows[rows.length - 1]!.key : PREFIX;
    return rows.map(row => recordSchema.parse(JSON.parse(row.value)));
  }

  list(actor: string): { request: string; decision: string; task: string; accepted: boolean }[] {
    const rows = this.store.db.prepare(
      `SELECT value FROM settings_kv WHERE key > ? AND key < ?
        AND json_extract(value, '$.recipient') = ? AND json_extract(value, '$.answer') IS NULL
        ORDER BY key LIMIT 100`,
    ).all(PREFIX, END, actor) as { value: string }[];
    return rows.map(row => recordSchema.parse(JSON.parse(row.value)))
      .filter(row => this.s.eligible(row.source))
      .map(row => ({ request: row.source.id, decision: row.source.decision, task: row.source.task, accepted: row.delivery !== null }));
  }

  async answer(request: string, actor: string, comment: string): Promise<boolean> {
    const row = await this.actorRequest(request, actor);
    if (!row?.delivery || row.recipient !== actor || !this.s.eligible(row.source)) return false;
    if (row.answer) return row.answer.actor === actor && row.answer.comment === comment;
    if (!await this.s.reply(row.source.task, comment, actor, row.delivery.at)) return false;
    const recorded = this.durable(() => {
      const fresh = this.get(request);
      if (!fresh?.delivery || fresh.epoch !== row.epoch || !this.s.eligible(fresh.source)) return false;
      if (fresh.answer) return fresh.answer.actor === actor && fresh.answer.comment === comment;
      if (this.store.setting(REPLY_PREFIX + comment) !== undefined) return false;
      this.store.setSetting(REPLY_PREFIX + comment, request);
      this.put({ ...fresh, answer: { actor, comment, at: this.s.now() } });
      return true;
    });
    if (recorded) await this.cancelQueues(request);
    return recorded;
  }

  async transfer(request: string, actor: string, target: string): Promise<boolean> {
    const row = await this.actorRequest(request, actor);
    if (!row || row.answer || !this.s.eligible(row.source)) return false;
    if (row.transfer?.from === actor && row.transfer.to === target) return true;
    if (row.recipient !== actor || !row.delivery || actor === target) return false;
    const route = await this.s.route(target, row.source.project, row.source.explicitOwner);
    if (!route || route.project !== row.source.project) return false;
    const claimed = this.durable(() => {
      const fresh = this.get(request);
      if (!fresh?.delivery || fresh.epoch !== row.epoch || fresh.answer || !this.s.eligible(fresh.source)) return null;
      const epoch = fresh.epoch + 1;
      const key = digest([request, epoch, "transfer"]);
      const ref = `reminder-transfer:${key}`;
      const since = this.s.now();
      const attempt: Attempt = { key, target, epoch, step: "transfer", result: "pending" };
      this.put({ ...fresh, recipient: target, transfer: { from: actor, to: target, at: since },
        epoch, ref, since, role: null, delivery: null, sends: [...fresh.sends, attempt] });
      return { attempt, ref };
    });
    if (!claimed) return false;
    await this.dispatch(request, claimed.attempt, claimed.ref,
      `Home request ${request} for decision ${row.source.decision} was transferred to you by ${actor}. Read the original Home ruling. Link your reply to this request with the Home request signal tool.`);
    return true;
  }

  private async cancelQueues(request: string): Promise<void> {
    const row = this.get(request);
    if (!row) return;
    const ended = row.answer !== null || !this.s.eligible(row.source);
    for (const attempt of row.sends) {
      if ((!ended && attempt.epoch === row.epoch) || attempt.result === "cancelled" || attempt.result === "sent") continue;
      try {
        const known = attempt.handle;
        if (known) {
          await this.s.removeQueued(attempt.target, known);
        } else if (this.s.queuedList) {
          // A response can be lost after the host enqueues the message.
          const prefix = `[followthrough ${attempt.key}] Advisory:`;
          const rows = (await this.s.queuedList(attempt.target)).filter(entry => entry.text.startsWith(prefix));
          if (rows.length !== 1) continue;
          await this.s.removeQueued(attempt.target, rows[0]!.id);
        } else continue;
      } catch { continue; }
      this.finish(request, attempt.key, "cancelled");
    }
  }

  private finish(request: string, key: string, result: Attempt["result"], handle?: string): void {
    this.store.db.transaction(() => {
      const row = this.get(request);
      if (!row) return;
      const active = row.sends.find(a => a.key === key);
      if (!active || active.result !== "pending" && !(["queued", "unknown"].includes(active.result) && result === "cancelled")) return;
      Object.assign(active, { result }, handle === undefined ? {} : { handle });
      this.put(row);
    }).immediate();
  }

  private async dispatch(request: string, attempt: Attempt, ref: string, payload: string): Promise<void> {
    let result: SendResult | null = null;
    try {
      result = await this.s.send({ threadId: attempt.target, mode: "queue-if-active", wakeCause: "retry", wakeRef: ref,
        // Reuse the reminder marker already excluded by the ordinary Home queue observer.
        input: `[followthrough ${attempt.key}] Advisory: ${payload}` });
    } catch { /* The remote outcome is unknown. The claim fences any replay. */ }
    this.finish(request, attempt.key, result?.delivery ?? "unknown",
      result?.delivery === "queued" ? result.queuedMessageId : undefined);
    await this.cancelQueues(request);
  }

  private async observe(row: RequestRecord, route: RequestRoute): Promise<void> {
    if (row.delivery) return;
    const delivery = await this.s.deliver(row.recipient, row.ref, row.since);
    if (!delivery || delivery.at < row.since || delivery.at > this.s.now()) return;
    this.durable(() => {
      const fresh = this.get(row.source.id);
      if (fresh && fresh.epoch === row.epoch && !fresh.delivery && this.s.eligible(fresh.source)) {
        this.put({ ...fresh, delivery, role: route.role });
      }
    });
  }

  private async actorRequest(request: string, actor: string): Promise<RequestRecord | null> {
    this.collect();
    const row = this.get(request);
    if (row?.recipient === actor && !row.delivery && this.s.eligible(row.source)) {
      const route = await this.s.route(actor, row.source.project, row.source.explicitOwner);
      if (route?.project === row.source.project) await this.observe(row, route);
      return this.get(request);
    }
    return row;
  }

  private async advance(row: RequestRecord): Promise<void> {
    const request = row.source.id;
    await this.cancelQueues(request);
    if (row.answer || !this.s.eligible(row.source)) return;
    const route = await this.s.route(row.recipient, row.source.project, row.source.explicitOwner);
    if (!route || route.project !== row.source.project) return;
    await this.observe(row, route);
    // Recheck the request under the same lock that writes the durable claim.
    const attempt = this.durable(() => {
      const fresh = this.get(request);
      if (!fresh?.delivery || !fresh.role || fresh.epoch !== row.epoch || fresh.answer
          || fresh.role !== route.role || !this.s.enabled(fresh.source) || !this.s.eligible(fresh.source)) return null;
      const elapsed = this.s.now() - fresh.delivery.at;
      const [reminder, escalation] = CLOCK[fresh.role];
      const step = elapsed >= escalation * MINUTE ? "escalation" : elapsed >= reminder * MINUTE ? "reminder" : null;
      const target = step === "escalation" ? route.next : fresh.recipient;
      if (!step || !target || step === "escalation" && target === fresh.recipient
          || fresh.sends.some(a => a.epoch === fresh.epoch && a.step === step)) return null;
      const claimed: Attempt = { key: digest([request, fresh.epoch, step]), target, epoch: fresh.epoch, step, result: "pending" };
      this.put({ ...fresh, sends: [...fresh.sends, claimed] });
      return claimed;
    });
    if (!attempt) return;
    const status = attempt.step === "escalation" ? ` is still unanswered by ${row.recipient}` : " is still awaiting your reply";
    await this.dispatch(request, attempt, `request-reminder:${attempt.key}`,
      `Home request ${request} for decision ${row.source.decision}${status}.`);
  }

  collect(): void {
    this.durable(() => {
      for (const raw of this.s.sources()) {
        const source = sourceSchema.safeParse(raw);
        if (!source.success) continue;
        const s = source.data;
        if (this.get(s.id) || !this.s.eligible(s)) continue;
        this.put({ source: s, recipient: s.recipient, ref: s.ref, since: s.since, epoch: 0, transfer: null,
          delivery: null, role: null, answer: null, sends: [] });
      }
    });
  }

  async tick(): Promise<boolean> {
    this.collect();
    let complete = true;
    for (const row of this.rows()) {
      try { await this.advance(row); }
      catch { complete = false; }
    }
    return complete;
  }
}
