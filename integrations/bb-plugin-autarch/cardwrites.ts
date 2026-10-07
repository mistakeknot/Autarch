// Write-back of a picked card to tasks (plan 1.3.8): a mirror comment and the needs-mk unlabel on a
// pick, and a relabel on an override. Rows live in `card_writes`, never `obligations`, are inserted in
// the pick's own transaction, and are executed here, outside it, through the TasksClient (never a
// spawned process). A failed write backs off and retries; it never blocks the wake or the ruling.
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

/** The comment body. The marker line makes the post idempotent across a lost response. */
export function commentBody(id: string, payload: Partial<PickPayload>): string {
  const label = payload.option_label ?? payload.option_id ?? "?";
  const gen = payload.generation == null ? "" : ` generation ${payload.generation}`;
  const lines = [`Ruled: ${label} (Home, advisory; ruled by: ${payload.by ?? "mk"})${gen}`];
  if (payload.by === "vizier" && payload.reason) lines.push(`Reason: ${payload.reason}`);
  if (payload.option_id === "other" && payload.reason) lines.push("", "mk's answer (plain text):", payload.reason);
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

export class CardWriter {
  private running = false;

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

  /** Run every due write once, serially. Never throws; a failure is recorded on its row. */
  async drain(): Promise<{ done: number; failed: number }> {
    const out = { done: 0, failed: 0 };
    if (this.running) return out;
    this.running = true;
    try {
      this.skipSuperseded();
      for (const r of this.due()) {
        if (r.kind === "relabel" && this.blockedByUnlabel(r)) continue;
        try {
          await this.run(r);
          out.done++;
        } catch (e) {
          this.fail(r, e);
          out.failed++;
        }
      }
      await this.drainMoveCloses(out);
    } finally {
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
      try {
        // A newer live move on the card means the card is still owed.
        if (this.db.prepare("SELECT 1 FROM moves WHERE task_id = ? AND state <> 'closed'").get(r.task_id)) {
          settle(r, "skipped", "card has another live move");
          continue;
        }
        const st = await this.tasks.taskState(r.task_id);
        if (st.state === "deleted") {
          settle(r, "skipped", "task deleted");
          continue;
        }
        const labels = await this.tasks.listLabels(st.task.projectId, { fresh: true });
        const filed = labels.some((l) => l.name === HOME_MOVE_LABEL && st.task.labelIds.includes(l.id));
        if (!filed) settle(r, "skipped", "not filed by Home (no mk-move label)");
        else {
          if (st.task.status !== "done" && st.task.status !== "canceled") await this.tasks.closeTask(r.task_id);
          settle(r, "done", null);
        }
        out.done++;
      } catch (e) {
        const delay = Math.min(BACKOFF_MS * 2 ** Math.min(r.attempt, 20), MAX_BACKOFF_MS);
        this.db
          .prepare("UPDATE move_closes SET attempt = attempt + 1, last_error = ?, next_try_at = ?, updated_at = ? WHERE task_id = ? AND generation = ? AND state = 'pending'")
          .run(message(e), new Date(Date.parse(this.now()) + delay).toISOString(), this.now(), r.task_id, r.generation);
        out.failed++;
      }
    }
  }

  private async run(r: WriteRow): Promise<void> {
    const payload = JSON.parse(r.payload ?? "{}") as Partial<PickPayload>;
    if (r.kind === "comment") {
      const marker = `${MARKER} ${r.id}`;
      const existing = await this.tasks.listComments(r.task_id);
      if (!existing.some((c) => c.body.includes(marker))) await this.tasks.createComment(r.task_id, commentBody(r.id, payload));
      this.settle(r.id, "done", null);
      return;
    }
    const st = await this.tasks.taskState(r.task_id);
    if (r.kind === "unlabel") {
      if (st.state === "labelled") {
        const ids = await this.tasks.needsMkLabelIds(st.task.projectId, { fresh: true });
        await this.tasks.setLabels(r.task_id, st.task.labelIds.filter((l) => !ids.includes(l)));
      }
      this.db.prepare("UPDATE cards SET home_unlabelled_at = COALESCE(home_unlabelled_at, ?) WHERE task_id = ?").run(this.now(), r.task_id);
      this.settle(r.id, "done", null);
      return;
    }
    // relabel: only while the override is still mk's open question.
    const open = this.db
      .prepare("SELECT 1 FROM decisions d WHERE d.id = ? AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)")
      .get(r.decision_id);
    if (!open || st.state === "deleted") return this.settle(r.id, "skipped", open ? "task deleted" : "override no longer open");
    if (st.state === "unlabelled") {
      const ids = await this.tasks.needsMkLabelIds(st.task.projectId, { fresh: true });
      if (ids.length === 0) throw new Error("the tasks project has no needs-mk label");
      await this.tasks.setLabels(r.task_id, [...st.task.labelIds, ids[0]!]);
    }
    this.db.prepare("UPDATE cards SET home_unlabelled_at = NULL WHERE task_id = ?").run(r.task_id);
    this.settle(r.id, "done", null);
  }
}
