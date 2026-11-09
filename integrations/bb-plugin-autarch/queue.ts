// The card poller (plan 1.3.2): reads tasks, ingests changed cards, and keeps three independent
// retry sets (unresolved cards, routing revalidation of open cards, routing-closed cards). It
// never spawns a process; every read goes through the TasksClient. A failure in the first read
// phase (projects, task lists, fresh re-reads of missing cards) changes nothing (T12) and only flips
// the health to "degraded: tasks". The poll is not one transaction: gone cards are written before
// per-card comment reads, so a later comment-read failure leaves earlier card writes in place, and
// the writer drain runs after every poll on its own.
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
  /** Bumped by rebind: an ingest that began before it must not record its (stale) digest. */
  private rebindEpoch = 0;
  private health: QueueStatus["health"] = "ok";
  private lastError: string | null = null;
  private lastPollAt: string | null = null;
  private running = false;

  constructor(private readonly deps: QueueDeps) {
    this.pollMs = deps.pollMs ?? POLL_MS;
  }

  /**
   * A binding changed: forget the digests of the tasks project's cards so the next poll re-evaluates them
   * (a card shown as a project mismatch is otherwise re-read only when the card itself is edited).
   */
  rebind(cardTaskIds: readonly string[]): void {
    this.rebindEpoch++;
    for (const id of cardTaskIds) this.digest.delete(id);
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

  /**
   * One poll. Returns whether anything changed. Never throws. With a signal, the poll hands it to every tasks read, starts
   * no further step once it is aborted, and skips the writer drain (owed write-backs stay owed for the next run).
   */
  async pollOnce(signal?: AbortSignal): Promise<{ changed: boolean; ok: boolean }> {
    if (this.running) return { changed: false, ok: this.health === "ok" };
    this.running = true;
    try {
      return await this.poll(signal);
    } catch (e) {
      if (signal?.aborted) return { changed: false, ok: this.health === "ok" }; // stopped, not failed: health is left as it was
      this.degrade(e);
      return { changed: false, ok: false };
    } finally {
      try {
        if (!signal?.aborted) await this.deps.writer?.drain();
      } catch {
        /* drain records its own failures */
      }
      this.running = false;
    }
  }

  private async poll(signal?: AbortSignal): Promise<{ changed: boolean; ok: boolean }> {
    const { service, tasks } = this.deps;
    /** Checked between steps: once the service is told to stop, no new read or write begins. */
    const stopped = () => signal?.aborted === true;

    // ---- reads: all of them before the first write, so a failure changes nothing (T12) ----
    const listed = new Map<string, Task>();
    const names = new Map<string, string>();
    const goneCards: { id: string; why: "closed" | "unlabelled" | "deleted"; status?: string }[] = [];
    try {
      const projects = await tasks.listProjects(signal);
      for (const p of projects) {
        if (stopped()) return { changed: false, ok: this.health === "ok" };
        names.set(p.id, p.name);
        const ids = await tasks.needsMkLabelIds(p.id, { signal });
        if (ids.length === 0) continue;
        for (const t of await tasks.listTasks({ projectId: p.id, statuses: OPEN_STATUSES, labelIds: ids, signal })) listed.set(t.id, t);
      }
      // Known cards missing from the list: a fresh read decides deleted, unlabelled, closed or still there.
      const gone = goneCards;
      const known = new Set(service.knownOpenCards().map((k) => k.task_id));
      // A card with a live move is re-read too, whatever its label or cached status, so a card that left before its
      // moves closed (or was unlabelled first) still has them closed on a fresh read (bead mk-t7656).
      const moveOnly = new Set(service.liveMoveCards().filter((id) => !known.has(id)));
      // So is an open override card (mk-cggjk): cached unlabelled and still open, a later cancel or delete is only
      // seen on a fresh read.
      // Only OPEN cards are re-read. A card closed here whose task is then unlabelled and only status-reopened
      // stays closed, by design: the queue is "a tasks card labelled needs-mk" (docs/plans/2026-09-30-home-on-bb-tasks-plan.md:31),
      // so an unlabelled task is outside it until the label is put back (bead mk-oss7o).
      const overrideOnly = new Set(service.openOverrideCards().filter((id) => !known.has(id) && !moveOnly.has(id)));
      for (const id of [...known, ...moveOnly, ...overrideOnly]) {
        if (listed.has(id)) continue;
        if (stopped()) return { changed: false, ok: this.health === "ok" };
        const st = await tasks.taskState(id, signal);
        const open = st.state !== "deleted" && (OPEN_STATUSES as readonly string[]).includes(st.task.status);
        if (st.state === "deleted") gone.push({ id, why: "deleted" });
        else if (st.state === "unlabelled") {
          if (!((moveOnly.has(id) || overrideOnly.has(id)) && open)) gone.push({ id, why: "unlabelled", status: st.task.status }); // a move-only or override-only re-read: already handled
        } else if (open) listed.set(st.task.id, st.task);
        else gone.push({ id, why: "closed", status: st.task.status });
      }
    } catch (e) {
      if (stopped()) return { changed: false, ok: this.health === "ok" }; // an aborted read is a stop, not a tasks failure
      this.degrade(e);
      return { changed: false, ok: false };
    }
    if (stopped()) return { changed: false, ok: this.health === "ok" };

    // ---- writes ----
    let changed = false;
    let unavailable: string | null = null;
    const note = (r: { changed: boolean; unavailable?: string }) => {
      if (r.changed) changed = true;
      if (r.unavailable) unavailable = r.unavailable;
    };

    /** Once stopped, the writes already made stand (the poll is not one transaction); nothing more starts. */
    const stoppedNow = () => {
      if (!stopped()) return false;
      if (changed) this.deps.publish?.();
      return true;
    };

    for (const g of goneCards) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
      if (service.cardGone(g.id, g.why, g.status).changed) changed = true;
      this.digest.delete(g.id);
    }

    const all = [...listed.values()];
    if (stoppedNow()) return { changed, ok: this.health === "ok" };
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

    const comments = (id: string) => () => tasks.listComments(id, signal);
    const seen = new Set<string>();
    const ingest = async (t: Task, commit: boolean) => {
      if (seen.has(t.id)) return;
      seen.add(t.id);
      const epoch = this.rebindEpoch;
      const r = await service.ingestCard(t, { comments: comments(t.id) });
      note(r);
      if (commit && !r.unavailable && epoch === this.rebindEpoch) this.digest.set(t.id, digestOf(t));
    };
    for (const t of all) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
      if (this.digest.get(t.id) !== digestOf(t)) await ingest(t, true);
    }
    // Unresolved-retry set, independent of the digest.
    for (const id of service.dueObserved(RETRY_BATCH)) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
      const t = listed.get(id);
      if (t) await ingest(t, false);
    }
    // Routing revalidation of open cards, then the closed ones, so open cards keep their bound.
    for (const id of service.routingDue(REVALIDATE_BATCH)) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
      if (seen.has(id)) continue;
      note(await service.revalidateRouting(id, comments(id)));
    }
    // A held card whose task did not change this poll can still have a new comment with a script digest.
    for (const id of Object.keys(service.holds())) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
      if (seen.has(id) || !listed.has(id)) continue;
      if (await service.releaseHoldOnSha(id, comments(id))) note({ changed: true });
    }
    for (const id of service.dueRoutingClosed(CLOSED_BATCH)) {
      if (stoppedNow()) return { changed, ok: this.health === "ok" };
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

  /**
   * Poll until the signal aborts. bb gives a stopping service 5 s before it marks the plugin degraded, and a poll can
   * be inside an RPC that is slow or never answers, so the loop returns as soon as the signal fires; an in-flight
   * poll is told to stop (the signal reaches its reads, it starts no new step) and its tail is left to finish alone.
   */
  async run(signal: AbortSignal, sleep: (ms: number, signal: AbortSignal) => Promise<void>): Promise<void> {
    const aborted = new Promise<void>((resolve) => {
      if (signal.aborted) resolve();
      else signal.addEventListener("abort", () => resolve(), { once: true });
    });
    while (!signal.aborted) {
      await Promise.race([this.pollOnce(signal), aborted]);
      if (signal.aborted) return;
      await sleep(this.pollMs, signal);
    }
  }
}
