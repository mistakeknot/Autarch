// The steps runbook, the machine lane, and blocker lifecycle [D-11] [E-8] [H-5].
//
// Ownership is stored on the decision at filing (the ask's owner, else the
// machineOwners setting for its class), so a later settings change never moves a
// blocker that already has an owner. Every lifecycle change is one transaction.
import { parseAsk } from "./model.js";
import type { ObligationInput } from "./store.js";
import type { Service } from "./service.js";

type Ctx = { threadId?: string };
type Fail = { ok: false; status: number; error: string };
export type LifecycleResult = { ok: true; replay: boolean } | Fail;

interface Row {
  id: string;
  kind: string;
  task_id?: string | null;
  subject: string;
  thread: string;
  project: string;
  owner_thread: string | null;
  filed_at: string;
  updated_at: string;
  resolved_at: string | null;
  withdrawn_at: string | null;
  body_json: string;
}

export interface RunbookGroup {
  thread: string;
  items: { id: string; subject: string; question: string; steps: string[]; filed_at: string }[];
}
export interface LaneEntry {
  id: string;
  subject: string;
  thread: string;
  owner: string | null;
  detail: string;
  updated_at: string;
  label?: "unowned machine blocker" | "stalled";
}

/** Q6 default, one function: the legacy lifecycle (get/progress/resolve/withdraw, runbook, lane) is for
 *  pre-v3 asks only. A card row (task_id set) closes through its tasks card, never here. */
export function legacyOnlyRefusal(d: { task_id?: string | null }): Fail | undefined {
  return d.task_id ? { ok: false, status: 400, error: "card asks close through tasks" } : undefined;
}

export const STALL_MS = 24 * 3_600_000;

export class Asks {
  constructor(readonly svc: Service) {}

  private get db() {
    return this.svc.store.db;
  }
  private row(id: string): Row | undefined {
    return this.svc.store.decision(id) as Row | undefined;
  }
  private msNow(): number {
    return Date.parse(this.svc.time());
  }
  private open(kind: string): Row[] {
    return this.db
      .prepare(
        `SELECT d.* FROM decisions d WHERE d.kind = ? AND d.task_id IS NULL AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
           AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
         ORDER BY d.filed_at, d.rowid`,
      )
      .all(kind) as Row[];
  }

  get(id: string) {
    const d = this.row(id);
    if (!d || legacyOnlyRefusal(d)) return undefined;
    return {
      id: d.id,
      kind: d.kind,
      subject: d.subject,
      thread: d.thread,
      project: d.project,
      owner_thread: d.owner_thread,
      filed_at: d.filed_at,
      updated_at: d.updated_at,
      resolved_at: d.resolved_at,
      withdrawn_at: d.withdrawn_at,
    };
  }

  // ---- readers ----------------------------------------------------------------

  /** Every thread's open steps, grouped by thread in filing order. */
  runbook(): RunbookGroup[] {
    const groups = new Map<string, RunbookGroup>();
    for (const d of this.open("steps")) {
      const ask = parseAsk(JSON.parse(d.body_json));
      const g = groups.get(d.thread) ?? { thread: d.thread, items: [] };
      g.items.push({ id: d.id, subject: d.subject, question: ask.question, steps: ask.steps ?? [], filed_at: d.filed_at });
      groups.set(d.thread, g);
    }
    return [...groups.values()];
  }

  /** The machine lane (owned, fresh) and the machine asks that need mk (unowned, or stalled past 24 h). */
  lists(): { lane: LaneEntry[]; asks: LaneEntry[] } {
    const lane: LaneEntry[] = [];
    const asks: LaneEntry[] = [];
    const now = this.msNow();
    for (const d of this.open("machine")) {
      const ask = parseAsk(JSON.parse(d.body_json));
      const e: LaneEntry = { id: d.id, subject: d.subject, thread: d.thread, owner: d.owner_thread, detail: ask.machine?.detail ?? "", updated_at: d.updated_at };
      if (!d.owner_thread) asks.push({ ...e, label: "unowned machine blocker" });
      else if (now - Date.parse(d.updated_at) > STALL_MS) asks.push({ ...e, label: "stalled" });
      else lane.push(e);
    }
    return { lane, asks };
  }

  // ---- lifecycle --------------------------------------------------------------

  private replayed(type: string, id: string, requestId: string | undefined): boolean {
    if (!requestId) return false;
    return !!this.db
      .prepare("SELECT 1 FROM events WHERE type = ? AND decision_id = ? AND json_extract(detail_json, '$.request_id') = ?")
      .get(type, id, requestId);
  }

  private machine(id: string): { d: Row } | Fail {
    const d = this.row(id);
    if (!d) return { ok: false, status: 404, error: "unknown decision" };
    const refused = legacyOnlyRefusal(d);
    if (refused) return refused;
    return { d };
  }

  progress(id: string, note: string | undefined, ctx: Ctx, requestId?: string): LifecycleResult {
    const m = this.machine(id);
    if (!("d" in m)) return m;
    const d = m.d;
    if (!ctx.threadId || d.owner_thread !== ctx.threadId) return { ok: false, status: 403, error: "only the owner thread reports progress" };
    if (d.kind !== "machine") return { ok: false, status: 409, error: "not a machine blocker" };
    if (this.replayed("progress", id, requestId)) return { ok: true, replay: true };
    if (d.resolved_at || d.withdrawn_at) return { ok: false, status: 409, error: d.resolved_at ? "resolved" : "withdrawn" };
    const at = this.svc.time();
    this.db.transaction(() => {
      this.db.prepare("UPDATE decisions SET updated_at = ? WHERE id = ?").run(at, id);
      this.svc.store.recordEvent("progress", id, { thread: ctx.threadId, note: note ?? "", request_id: requestId ?? null });
    }).immediate();
    return { ok: true, replay: false };
  }

  resolve(id: string, note: string | undefined, ctx: Ctx, requestId?: string): LifecycleResult {
    const m = this.machine(id);
    if (!("d" in m)) return m;
    const d = m.d;
    if (!ctx.threadId || (ctx.threadId !== d.owner_thread && ctx.threadId !== d.thread)) {
      return { ok: false, status: 403, error: "only the owner or the asking thread resolves a blocker" };
    }
    if (d.kind !== "machine") return { ok: false, status: 409, error: "not a machine blocker" };
    if (this.replayed("resolved", id, requestId)) return { ok: true, replay: true };
    return this.close(d, "resolved", note, ctx.threadId, requestId);
  }

  withdraw(id: string, ctx: Ctx, requestId?: string): LifecycleResult {
    const m = this.machine(id);
    if (!("d" in m)) return m;
    const d = m.d;
    if (!ctx.threadId || ctx.threadId !== d.thread) return { ok: false, status: 403, error: "only the asking thread withdraws a blocker" };
    if (d.kind !== "machine") return { ok: false, status: 409, error: "not a machine blocker" };
    if (this.replayed("withdrawn", id, requestId)) return { ok: true, replay: true };
    return this.close(d, "withdrawn", undefined, ctx.threadId, requestId);
  }

  /** Close the blocker, void the owner notice, and fan out the notices, all in one transaction. */
  private close(d: Row, how: "resolved" | "withdrawn", note: string | undefined, by: string, requestId: string | undefined): LifecycleResult {
    const store = this.svc.store;
    const verb = how === "resolved" ? "resolve" : "withdraw";
    const at = this.svc.time();
    const run = this.db.transaction((): LifecycleResult => {
      const col = how === "resolved" ? "resolved_at" : "withdrawn_at";
      const upd = this.db.prepare(`UPDATE decisions SET ${col} = ?, updated_at = ? WHERE id = ? AND resolved_at IS NULL AND withdrawn_at IS NULL`).run(at, at, d.id);
      if (upd.changes !== 1) {
        const cur = this.row(d.id)!;
        return { ok: false, status: 409, error: cur.resolved_at ? "already resolved" : "already withdrawn" };
      }
      store.recordEvent(how, d.id, { thread: by, note: note ?? "", request_id: requestId ?? null });
      const mentioners = store.mentions(d.id).map((x) => x.thread);
      const ownerRow = store.obligationsFor(d.id).find((o) => o.op === `owner:${d.id}`);
      if (ownerRow) store.voidObligation(ownerRow.id);
      const text = (role: string) =>
        [
          `The machine blocker ${d.id}${d.subject ? ` (${d.subject})` : ""} you ${role} was ${how}${by ? ` by ${by}` : ""}.`,
          ...(note ? [`Note: ${note}`] : []),
        ].join("\n");
      const rows: ObligationInput[] = [];
      const seen = new Set<string>();
      const add = (thread: string, role: string, afterId?: string) => {
        if (!thread || seen.has(thread)) return;
        seen.add(thread);
        rows.push({
          id: `ob:${d.id}:${verb}:${thread}`,
          kind: verb,
          recipient: thread,
          op: `${verb}:${d.id}:${thread}`,
          payload: text(role),
          ...(afterId ? { after_id: afterId } : {}),
        });
      };
      if (how === "resolved") add(d.thread, "filed");
      for (const t of mentioners) add(t, "mentioned");
      // An owner who was already told (any attempt) hears the outcome after that notice; a
      // notice that was never tried is simply voided [H-5].
      if (ownerRow && d.owner_thread && d.owner_thread !== by && store.attempts(ownerRow.id).length > 0) {
        add(d.owner_thread, "own", ownerRow.id);
      }
      store.insertObligations(d.id, rows);
      return { ok: true, replay: false };
    });
    const out = run.immediate();
    if (out.ok) this.svc.nudge();
    return out;
  }
}
