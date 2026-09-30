import type { AcceptedEvent, QueuedRow, SendResult, WakeSdk } from "../wakes.js";

export type Behavior =
  | { kind: "sent" }
  | { kind: "queue"; before?: (row: FakeRow) => void | Promise<void> }
  | { kind: "queue-throw"; err: Error }
  | { kind: "throw"; err: Error };

export interface FakeRow {
  id: string;
  threadId: string;
  text: string;
  state: "queued" | "dispatched" | "cancelled";
}

export const econnrefused = () => Object.assign(new Error("connect ECONNREFUSED 127.0.0.1"), { code: "ECONNREFUSED" });
export const netError = () => new Error("socket hang up");
export const archived = () => Object.assign(new Error("thread is archived"), { code: "thread_archived" });

/** A fake of the slice of bb.sdk.threads that the wake loop touches. */
export class FakeSdk implements WakeSdk {
  rows = new Map<string, FakeRow>();
  accepted: (AcceptedEvent & { threadId: string })[] = [];
  sent: { threadId: string; input: string; mode: string }[] = [];
  script: Behavior[] = [];
  fallback: Behavior = { kind: "sent" };
  gate: Promise<void> | null = null;
  caps = { state: true, list: true, del: true, events: true };
  deletes: string[] = [];
  running = 0;
  maxRunning = 0;
  private n = 0;

  /** Block the next send until the returned function is called. */
  block(): () => void {
    let open!: () => void;
    this.gate = new Promise<void>((r) => (open = r));
    return open;
  }

  async send(a: { threadId: string; input: string; mode: "queue-if-active" }): Promise<SendResult> {
    this.sent.push(a);
    this.running++;
    this.maxRunning = Math.max(this.maxRunning, this.running);
    try {
      const g = this.gate;
      if (g) {
        this.gate = null;
        await g;
      }
      const b = this.script.shift() ?? this.fallback;
      if (b.kind === "throw") throw b.err;
      if (b.kind === "sent") {
        const id = `m${++this.n}`;
        this.accepted.push({ id, threadId: a.threadId, text: a.input });
        return { delivery: "sent", messageId: id };
      }
      const row = this.inject(a.threadId, a.input);
      if (b.kind === "queue-throw") throw b.err;
      await b.before?.(row);
      return { delivery: "queued", queuedMessageId: row.id };
    } finally {
      this.running--;
    }
  }

  /** A queued row that exists in the fake without any send having answered. */
  inject(threadId: string, text: string): FakeRow {
    const row: FakeRow = { id: `q${++this.n}`, threadId, text, state: "queued" };
    this.rows.set(row.id, row);
    return row;
  }
  dispatch(id: string): void {
    const r = this.rows.get(id)!;
    r.state = "dispatched";
    this.accepted.push({ id: `ev${++this.n}`, threadId: r.threadId, text: r.text });
  }
  cancel(id: string): void {
    this.rows.get(id)!.state = "cancelled";
  }

  async queuedState(threadId: string, id: string): Promise<"queued" | "dispatched" | "cancelled" | "unknown"> {
    if (!this.caps.state) throw new Error("unsupported");
    const r = this.rows.get(id);
    return r && r.threadId === threadId ? r.state : "unknown";
  }
  async queuedList(threadId: string): Promise<QueuedRow[]> {
    if (!this.caps.list) throw new Error("unsupported");
    return [...this.rows.values()].filter((r) => r.threadId === threadId && r.state === "queued").map((r) => ({ id: r.id, text: r.text }));
  }
  async queuedDelete(threadId: string, id: string): Promise<void> {
    if (!this.caps.del) throw new Error("unsupported");
    const r = this.rows.get(id);
    if (!r || r.threadId !== threadId || r.state !== "queued") throw new Error("queued message not found");
    r.state = "cancelled";
    this.deletes.push(id);
  }
  async acceptedEvents(threadId: string): Promise<AcceptedEvent[]> {
    if (!this.caps.events) throw new Error("unsupported");
    return this.accepted.filter((e) => e.threadId === threadId).map((e) => ({ id: e.id, text: e.text }));
  }
}
