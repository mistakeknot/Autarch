// Write-back of a picked card to tasks (plan 1.3.8): a mirror comment and the needs-mk unlabel on a
// pick, and a relabel on an override. Rows live in `card_writes`, never `obligations`, are inserted in
// the pick's own transaction, and are executed here, outside it, through the TasksClient (never a
// spawned process). A failed write backs off and retries; it never blocks the wake or the ruling.
import { isOwnWordsForm } from "./ownwords.js";
import type { CardWriteInput } from "./store.js";
import type { TasksClient } from "./tasks.js";
import type Database from "better-sqlite3";

/** Q2 ruling (mk question 2): Home may remove needs-mk and post a mirror comment after a pick. One switch each. */
export const HOME_UNLABELS_ON_PICK = true;
export const HOME_COMMENTS_ON_PICK = true;
/** Q2 ruling: Home may re-add needs-mk on an override. */
export const HOME_RELABELS_ON_OVERRIDE = true;

const BACKOFF_MS = 30_000;
const MAX_BACKOFF_MS = 3_600_000;
const MARKER = "home-write:";
/** The label a card carries when Home filed it for a move; only such cards are closed when the move closes. */
export const HOME_MOVE_LABEL = "mk-move";

export interface PickPayload {
  option_id: string;
  option_label: string;
  by: string;
  generation: number | null;
  picked_at: string;
  reason: string | null;
}

/** The rows a pick of a card generation inserts with the pick. The comment renders `by` from the pick. */
export function pickWrites(d: { id: string; task_id: string }, p: PickPayload): CardWriteInput[] {
  const payload = JSON.stringify(p);
  const out: CardWriteInput[] = [];
  if (HOME_COMMENTS_ON_PICK) out.push({ id: `cw-${d.id}-comment`, task_id: d.task_id, kind: "comment", payload });
  if (HOME_UNLABELS_ON_PICK) out.push({ id: `cw-${d.id}-unlabel`, task_id: d.task_id, kind: "unlabel", payload });
  return out;
}

/** The row an override generation inserts, so mk's new question is visible again. */
export function overrideWrites(d: { id: string; task_id: string }, supersedes: string): CardWriteInput[] {
  return HOME_RELABELS_ON_OVERRIDE ? [{ id: `cw-${d.id}-relabel`, task_id: d.task_id, kind: "relabel", payload: JSON.stringify({ supersedes }) }] : [];
}

/** The row a bring-back generation inserts: relabel, and reopen the task if it was closed. */
export function bringBackWrites(d: { id: string; task_id: string }, supersedes: string): CardWriteInput[] {
  return [{ id: `cw-${d.id}-bringback`, task_id: d.task_id, kind: "relabel", payload: JSON.stringify({ supersedes, bringback: true }) }];
}

/** The comment body. The marker line makes the post idempotent across a lost response. */
export function commentBody(id: string, payload: Partial<PickPayload>): string {
  const label = payload.option_label ?? payload.option_id ?? "?";
  const gen = payload.generation == null ? "" : ` generation ${payload.generation}`;
  const lines = [`Ruled: ${label} (Home, advisory; ruled by: ${payload.by ?? "mk"})${gen}`];
  if (payload.by === "vizier" && payload.reason) lines.push(`Reason: ${payload.reason}`);
  if (payload.by === "auto" && payload.reason) lines.push(`Closed automatically, not by mk's click. Evidence: ${payload.reason}`);
  if ((payload.option_id === "other" || isOwnWordsForm(payload.option_id ?? "")) && payload.reason) lines.push("", "mk's answer (plain text):", payload.reason);
  if (payload.option_id === "answers" && payload.reason) lines.push("", "mk's answers (plain text):", payload.reason);
  lines.push("", `${MARKER} ${id}`);
  return lines.join("\n");
}

interface WriteRow {
  id: string;
  task_id: string;
  decision_id: string;
  kind: "comment" | "unlabel" | "relabel";
  payload: string | null;
  attempt: number;
}

const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** Thrown inside a drain when the service was told to stop; the write it interrupted stays owed, not failed. */
class Stopped extends Error {}

/**
 * Tasks a writer in this process is working on, keyed by database file and task. A drain told to stop leaves its one sent write
 * to land, and a writer built by a restarted service (its own `running` flag is fresh) must not touch that task meanwhile: a
 * comment still on its way is not there to find yet, and a label write still on its way is not in the task state it reads. A
 * claim is held while the drain reads or writes for its task; a stop ends a read at once (see `read`), so only a write already
 * sent keeps it. A claim older than CLAIM_TTL_MS is ignored, so a write that never answers cannot hold its task for good.
 */
const claims = new Map<string, { token: symbol; since: number }>();
const CLAIM_TTL_MS = 120_000;
/** Per task: how many writes landed after their claim had been taken over. A relabel that began before one must not settle. */
const lateLandings = new Map<string, number>();
type Claim = { release: () => void; lost: () => boolean; key: string; epoch: number };

export class CardWriter {
  private running = false;
  /** The stop signal of the drain in progress (drains never overlap). Reads carry it; a write in flight is never cut. */
  private cur?: Claim;
  private signal: AbortSignal | undefined;

  constructor(
    private readonly db: Database.Database,
    private readonly tasks: TasksClient,
    private readonly now: () => string,
  ) {}

  private settle(id: string, state: "done" | "skipped", note: string | null): void {
    this.db.prepare("UPDATE card_writes SET state = ?, last_error = ?, updated_at = ? WHERE id = ? AND state = 'pending'").run(state, note, this.now(), id);
  }

  private fail(r: WriteRow, e: unknown): void {
    const delay = Math.min(BACKOFF_MS * 2 ** Math.min(r.attempt, 20), MAX_BACKOFF_MS);
    this.db
      .prepare("UPDATE card_writes SET attempt = attempt + 1, last_error = ?, next_try_at = ?, updated_at = ? WHERE id = ? AND state = 'pending'")
      .run(message(e), new Date(Date.parse(this.now()) + delay).toISOString(), this.now(), r.id);
  }

  /** An unlabel whose generation has been overridden must never run: the override relabels. */
  private skipSuperseded(): void {
    this.db
      .prepare(
        `UPDATE card_writes SET state = 'skipped', last_error = 'superseded by an override', updated_at = @now
         WHERE state = 'pending' AND kind = 'unlabel'
           AND EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = card_writes.decision_id)`,
      )
      .run({ now: this.now() });
  }

  private due(): WriteRow[] {
    return this.db
      .prepare("SELECT id, task_id, decision_id, kind, payload, attempt FROM card_writes WHERE state = 'pending' AND next_try_at <= ? ORDER BY rowid")
      .all(this.now()) as WriteRow[];
  }

  private blockedByUnlabel(r: WriteRow): boolean {
    return !!this.db.prepare("SELECT 1 FROM card_writes WHERE task_id = ? AND kind = 'unlabel' AND state = 'pending'").get(r.task_id);
  }

  /** Throws Stopped once the drain's signal has aborted: checked after every await and before every write, so none starts late. */
  private gate(): void {
    if (this.signal?.aborted) throw new Stopped("stopped");
  }

  /**
   * Await a read, but give up the moment the drain is told to stop: the read may never answer (the host need not honour the
   * signal), and the drain must not sit on its task for it. A read that answers late is dropped; it changes nothing.
   */
  private read<T>(p: Promise<T>): Promise<T> {
    const s = this.signal;
    if (!s) return p;
    if (s.aborted) {
      p.catch(() => {});
      return Promise.reject(new Stopped("stopped"));
    }
    return new Promise<T>((resolve, reject) => {
      const onAbort = () => reject(new Stopped("stopped"));
      s.addEventListener("abort", onAbort, { once: true });
      p.then(
        (v) => (s.removeEventListener("abort", onAbort), resolve(v)),
        (e) => (s.removeEventListener("abort", onAbort), reject(e)),
      );
    });
  }

  /** Take the task, or return null when another writer in this process holds it. The claim gives it back with `release`. */
  private claim(taskId: string): Claim | null {
    const key = `${this.db.name}\0${taskId}`;
    const held = claims.get(key);
    if (held && Date.now() - held.since < CLAIM_TTL_MS) return null;
    const mine = { token: Symbol(key), since: Date.now() };
    claims.set(key, mine);
    return {
      key,
      epoch: lateLandings.get(key) ?? 0,
      lost: () => claims.get(key)?.token !== mine.token,
      release: () => {
        if (claims.get(key)?.token === mine.token) claims.delete(key);
      },
    };
  }

  /**
   * Called when a write the tasks plugin accepted has landed. If this writer's claim was taken over meanwhile (the TTL ran out on
   * a hung write), the other writer may have judged the task by what it looked like before this write and settled; so the write
   * is recorded as late, a relabel in progress will not settle, and relabels already done are put back to be re-verified (a
   * relabel reads the task afresh and checks its override is still open, so a re-run is safe).
   */
  private landed(taskId: string): void {
    const c = this.cur;
    if (!c || !c.lost()) return;
    lateLandings.set(c.key, (lateLandings.get(c.key) ?? 0) + 1);
    this.db
      .prepare("UPDATE card_writes SET state = 'pending', attempt = 0, next_try_at = ?, last_error = 'to re-verify after a late write', updated_at = ? WHERE task_id = ? AND kind = 'relabel' AND state = 'done'")
      .run(this.now(), this.now(), taskId);
  }

  /**
   * Run every due write once, serially. Never throws; a failure is recorded on its row. With a signal, the drain reads with
   * it and starts no further read or write once it aborts (a write already sent is left to land). A stop is not a failure:
   * the row it interrupted stays pending, with its attempt count and backoff untouched, for the next run.
   */
  async drain(signal?: AbortSignal): Promise<{ done: number; failed: number }> {
    const out = { done: 0, failed: 0 };
    if (this.running) return out;
    this.running = true;
    this.signal = signal;
    try {
      this.gate();
      this.skipSuperseded();
      for (const r of this.due()) {
        this.gate();
        if (r.kind === "relabel" && this.blockedByUnlabel(r)) continue;
        const claim = this.claim(r.task_id);
        if (!claim) continue; // another writer's write for this task has not landed yet
        this.cur = claim;
        try {
          await this.run(r);
          out.done++;
        } catch (e) {
          if (e instanceof Stopped || signal?.aborted) throw new Stopped("stopped");
          this.fail(r, e);
          out.failed++;
        } finally {
          this.cur = undefined;
          claim.release();
        }
      }
      await this.drainMoveCloses(out);
    } catch (e) {
      if (!(e instanceof Stopped)) throw e;
    } finally {
      this.signal = undefined;
      this.running = false;
    }
    return out;
  }

  /**
   * After a move closes: mark its tasks card done, but only a card Home filed (label mk-move). A stopgap card
   * the vizier filed is left for its owner. Idempotent per (task, generation); a failure backs off and retries.
   */
  private async drainMoveCloses(out: { done: number; failed: number }): Promise<void> {
    const rows = this.db.prepare("SELECT task_id, generation, attempt FROM move_closes WHERE state = 'pending' AND next_try_at <= ? ORDER BY rowid").all(this.now()) as { task_id: string; generation: number; attempt: number }[];
    const settle = (r: { task_id: string; generation: number }, state: "done" | "skipped", note: string | null) =>
      this.db.prepare("UPDATE move_closes SET state = ?, last_error = ?, updated_at = ? WHERE task_id = ? AND generation = ?").run(state, note, this.now(), r.task_id, r.generation);
    for (const r of rows) {
      this.gate();
      const claim = this.claim(r.task_id);
      if (!claim) continue; // another writer's write for this task has not landed yet
      this.cur = claim;
      try {
        // A later generation (a bring-back or an override) means mk is answering afresh: the old close is stale.
        if (this.db.prepare("SELECT 1 FROM decisions WHERE task_id = ? AND generation > ?").get(r.task_id, r.generation)) {
          settle(r, "skipped", "card was reopened by a later generation");
          continue;
        }
        // A newer live move on the card means the card is still owed.
        if (this.db.prepare("SELECT 1 FROM moves WHERE task_id = ? AND state <> 'closed'").get(r.task_id)) {
          settle(r, "skipped", "card has another live move");
          continue;
        }
        const st = await this.read(this.tasks.taskState(r.task_id, this.signal));
        this.gate();
        if (st.state === "deleted") {
          settle(r, "skipped", "task deleted");
          continue;
        }
        const labels = await this.read(this.tasks.listLabels(st.task.projectId, { fresh: true, signal: this.signal }));
        this.gate();
        const filed = labels.some((l) => l.name === HOME_MOVE_LABEL && st.task.labelIds.includes(l.id));
        if (!filed) settle(r, "skipped", "not filed by Home (no mk-move label)");
        else {
          if (st.task.status !== "done" && st.task.status !== "canceled") {
            await this.tasks.closeTask(r.task_id); // a write: no signal
            this.landed(r.task_id);
          }
          settle(r, "done", null);
        }
        out.done++;
      } catch (e) {
        if (e instanceof Stopped || this.signal?.aborted) throw new Stopped("stopped");
        const delay = Math.min(BACKOFF_MS * 2 ** Math.min(r.attempt, 20), MAX_BACKOFF_MS);
        this.db
          .prepare("UPDATE move_closes SET attempt = attempt + 1, last_error = ?, next_try_at = ?, updated_at = ? WHERE task_id = ? AND generation = ? AND state = 'pending'")
          .run(message(e), new Date(Date.parse(this.now()) + delay).toISOString(), this.now(), r.task_id, r.generation);
        out.failed++;
      } finally {
        this.cur = undefined;
        claim.release();
      }
    }
  }

  private async run(r: WriteRow): Promise<void> {
    const payload = JSON.parse(r.payload ?? "{}") as Partial<PickPayload>;
    if (r.kind === "comment") {
      const marker = `${MARKER} ${r.id}`;
      const existing = await this.read(this.tasks.listComments(r.task_id, this.signal));
      this.gate();
      if (!existing.some((c) => c.body.includes(marker))) await this.tasks.createComment(r.task_id, commentBody(r.id, payload));
      this.settle(r.id, "done", null);
      return;
    }
    const st = await this.read(this.tasks.taskState(r.task_id, this.signal));
    this.gate();
    if (r.kind === "unlabel") {
      if (st.state === "labelled") {
        const ids = await this.read(this.tasks.needsMkLabelIds(st.task.projectId, { fresh: true, signal: this.signal }));
        this.gate();
        await this.tasks.setLabels(r.task_id, st.task.labelIds.filter((l) => !ids.includes(l)));
        this.landed(r.task_id);
        this.gate();
      }
      this.db.prepare("UPDATE cards SET home_unlabelled_at = COALESCE(home_unlabelled_at, ?) WHERE task_id = ?").run(this.now(), r.task_id);
      // A ruled card's task is done, unless a thread is working on it: that thread closes it at its DONE turn.
      // Only the unlabel a pick queues counts; one a move retirement or evidence close queues (move_closed_by) is
      // no completion evidence and leaves the task open. A failed read or a refused close throws, so the write
      // backs off and retries; a task already closed is left alone. The tasks plugin has no conditional close, so a
      // thread attached between the read and the close is not protected (a narrow window, accepted).
      const byPick = !(JSON.parse(r.payload ?? "{}") as { move_closed_by?: unknown }).move_closed_by;
      if (byPick && st.state !== "deleted" && st.task.status !== "done" && st.task.status !== "canceled") {
        const active = await this.read(this.tasks.activeThreadCount(r.task_id, this.signal));
        this.gate();
        if (active === 0) {
          await this.tasks.setStatus(r.task_id, "done");
          this.landed(r.task_id);
        }
      }
      this.settle(r.id, "done", null);
      return;
    }
    // relabel: only while the override is still mk's open question.
    const open = this.db
      .prepare("SELECT 1 FROM decisions d WHERE d.id = ? AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)")
      .get(r.decision_id);
    if (!open || st.state === "deleted") {
      // cardGone kept the card open while this relabel was pending; a deleted task is gone for good and nothing
      // polls it, so close the card here (the override stays pickable; startup's sweep is the backstop).
      if (open) this.db.prepare("UPDATE cards SET state = 'closed', display_reason = 'card closed in tasks' WHERE task_id = ? AND state = 'open'").run(r.task_id);
      return this.settle(r.id, "skipped", open ? "task deleted" : "override no longer open");
    }
    if (st.state === "unlabelled") {
      const ids = await this.read(this.tasks.needsMkLabelIds(st.task.projectId, { fresh: true, signal: this.signal }));
      this.gate();
      if (ids.length === 0) throw new Error("the tasks project has no needs-mk label");
      await this.tasks.setLabels(r.task_id, [...st.task.labelIds, ids[0]!]);
      this.landed(r.task_id);
      this.gate(); // the label landed; the reopen that follows is a second write and must not start after a stop
    }
    // A brought-back or overridden card's task was closed when it was ruled (including the pick's own
    // auto-complete, the "unlabel" branch above, which sets it done); it is open work for mk again. A task a
    // person canceled after that is a deliberate close and stays closed, whatever delayed this write; only a
    // bring-back (which may follow a canceled ruling) also reopens a canceled task. A person who marks the task
    // done in the same window cannot be told from the pick's own completion (accepted, narrow).
    const reopen = st.task.status === "done" || (r.id.endsWith("-bringback") && st.task.status === "canceled");
    if (reopen) {
      await this.tasks.setStatus(r.task_id, "todo");
      this.landed(r.task_id);
    }
    // A task left closed leaves nothing to answer: the card stops showing as open (the override stays pickable,
    // and the card reopens if a person reopens the task, ingestInner). Nothing polls a closed task again.
    else if (st.task.status === "canceled")
      this.db.prepare("UPDATE cards SET state = 'closed', display_reason = 'card closed in tasks', status = ? WHERE task_id = ? AND state = 'open'").run(st.task.status, r.task_id);
    // A write of another writer that held this task before its claim ran out landed after this relabel began: what was read
    // above may be stale, so the row stays pending and the next drain reads the task afresh.
    if (this.cur && (lateLandings.get(this.cur.key) ?? 0) !== this.cur.epoch) return;
    this.db.prepare("UPDATE cards SET home_unlabelled_at = NULL WHERE task_id = ?").run(r.task_id);
    this.settle(r.id, "done", null);
  }
}
