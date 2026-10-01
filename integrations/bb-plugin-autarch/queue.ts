// The card poller (plan 1.3.2): reads tasks, ingests changed cards, and keeps three independent
// retry sets (unresolved cards, routing revalidation of open cards, routing-closed cards). It
// never spawns a process; every read goes through the TasksClient. A failed or partial read
// changes nothing (T12) and only flips the health to "degraded: tasks".
import type { CardWriter } from "./cardwrites.js";
import type { Service } from "./service.js";
import { OPEN_STATUSES, type Task, type TasksClient } from "./tasks.js";

export const POLL_MS = 5_000;
export const RETRY_BATCH = 20;
export const REVALIDATE_BATCH = 20;
export const CLOSED_BATCH = 5;

export interface QueueDeps {
  service: Service;
  tasks: TasksClient;
  /** Called after a poll that changed anything; the server publishes `home-queue-changed`. */
  publish?: () => void;
  pollMs?: number;
  /** Runs the due card write-backs after each poll, whether or not the poll's reads succeeded. */
  writer?: CardWriter;
}

export interface QueueStatus {
  health: "ok" | "degraded: tasks";
  last_error: string | null;
  last_poll_at: string | null;
  open_cards: number;
  /** Worst case between two routing checks of one open card, at the current count. */
  routing_worst_case_age_ms: number;
}

const digestOf = (t: Task) => `${t.updatedAt}|${t.status}|${[...t.labelIds].sort().join(",")}`;

export class Queue {
  private readonly pollMs: number;
  private readonly digest = new Map<string, string>();
  private health: QueueStatus["health"] = "ok";
  private lastError: string | null = null;
  private lastPollAt: string | null = null;
  private running = false;

  constructor(private readonly deps: QueueDeps) {
    this.pollMs = deps.pollMs ?? POLL_MS;
  }

  status(): QueueStatus {
    const open = this.deps.service.openThreadCardCount();
    return {
      health: this.health,
      last_error: this.lastError,
      last_poll_at: this.lastPollAt,
      open_cards: open,
      routing_worst_case_age_ms: Math.ceil(open / REVALIDATE_BATCH) * this.pollMs,
    };
  }

  private degrade(e: unknown): void {
    this.health = "degraded: tasks";
    this.lastError = e instanceof Error ? e.message : String(e);
  }

  /** One poll. Returns whether anything changed. Never throws. */
  async pollOnce(): Promise<{ changed: boolean; ok: boolean }> {
    if (this.running) return { changed: false, ok: this.health === "ok" };
    this.running = true;
    try {
      return await this.poll();
    } catch (e) {
      this.degrade(e);
      return { changed: false, ok: false };
    } finally {
      try {
        await this.deps.writer?.drain();
      } catch {
        /* drain records its own failures */
      }
      this.running = false;
    }
  }

  private async poll(): Promise<{ changed: boolean; ok: boolean }> {
    const { service, tasks } = this.deps;

    // ---- reads: all of them before the first write, so a failure changes nothing (T12) ----
    const listed = new Map<string, Task>();
    const names = new Map<string, string>();
    const goneCards: { id: string; why: "closed" | "unlabelled" | "deleted"; status?: string }[] = [];
    try {
      const projects = await tasks.listProjects();
      for (const p of projects) {
        names.set(p.id, p.name);
        const ids = await tasks.needsMkLabelIds(p.id);
        if (ids.length === 0) continue;
        for (const t of await tasks.listTasks({ projectId: p.id, statuses: OPEN_STATUSES, labelIds: ids })) listed.set(t.id, t);
      }
      // Known cards missing from the list: a fresh read decides deleted, unlabelled, closed or still there.
      const gone = goneCards;
      for (const k of service.knownOpenCards()) {
        if (listed.has(k.task_id)) continue;
        const st = await tasks.taskState(k.task_id);
        if (st.state === "deleted") gone.push({ id: k.task_id, why: "deleted" });
        else if (st.state === "unlabelled") gone.push({ id: k.task_id, why: "unlabelled", status: st.task.status });
        else if ((OPEN_STATUSES as readonly string[]).includes(st.task.status)) listed.set(st.task.id, st.task);
        else gone.push({ id: k.task_id, why: "closed", status: st.task.status });
      }
    } catch (e) {
      this.degrade(e);
      return { changed: false, ok: false };
    }

    // ---- writes ----
    let changed = false;
    let unavailable: string | null = null;
    const note = (r: { changed: boolean; unavailable?: string }) => {
      if (r.changed) changed = true;
      if (r.unavailable) unavailable = r.unavailable;
    };

    for (const g of goneCards) {
      if (service.cardGone(g.id, g.why, g.status).changed) changed = true;
      this.digest.delete(g.id);
    }

    const all = [...listed.values()];
    service.observeCards(all);
    try {
      const serve = await service.serveProjects();
      for (const t of all) {
        const name = names.get(t.projectId);
        if (name !== undefined) service.suggestBinding(t.projectId, name, serve);
      }
    } catch {
      /* serve down: no binding is suggested, and materialization waits as root unverified */
    }

    const comments = (id: string) => () => tasks.listComments(id);
    const seen = new Set<string>();
    const ingest = async (t: Task, commit: boolean) => {
      if (seen.has(t.id)) return;
      seen.add(t.id);
      const r = await service.ingestCard(t, { comments: comments(t.id) });
      note(r);
      if (commit && !r.unavailable) this.digest.set(t.id, digestOf(t));
    };
    for (const t of all) if (this.digest.get(t.id) !== digestOf(t)) await ingest(t, true);
    // Unresolved-retry set, independent of the digest.
    for (const id of service.dueObserved(RETRY_BATCH)) {
      const t = listed.get(id);
      if (t) await ingest(t, false);
    }
    // Routing revalidation of open cards, then the closed ones, so open cards keep their bound.
    for (const id of service.routingDue(REVALIDATE_BATCH)) {
      if (seen.has(id)) continue;
      note(await service.revalidateRouting(id, comments(id)));
    }
    for (const id of service.dueRoutingClosed(CLOSED_BATCH)) {
      const t = listed.get(id);
      if (!t || seen.has(id)) continue;
      seen.add(id);
      note(await service.ingestCard(t, { comments: comments(id) }));
      service.bumpClosedRetry(id);
    }
    // Forget cards that left the list so a later return is ingested again.
    for (const id of [...this.digest.keys()]) if (!listed.has(id)) this.digest.delete(id);

    this.lastPollAt = service.time();
    if (unavailable) this.degrade(unavailable);
    else {
      this.health = "ok";
      this.lastError = null;
    }
    if (changed) this.deps.publish?.();
    return { changed, ok: unavailable === null };
  }

  /** Poll until the signal aborts. */
  async run(signal: AbortSignal, sleep: (ms: number, signal: AbortSignal) => Promise<void>): Promise<void> {
    while (!signal.aborted) {
      await this.pollOnce();
      await sleep(this.pollMs, signal);
    }
  }
}
