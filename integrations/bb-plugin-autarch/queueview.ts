// The `queue` read model (plan 1.4) and the project-binding write (plan 1.3.6). Pure reads over the
// Home store, plus one write path for bindings that only the RPC layer calls. The vizier's `rule`
// path (cli.ts, delegation.ts) holds no reference to anything exported here that writes.
import type Database from "better-sqlite3";
import { COUNTED } from "./cards.js";
import { ROUTING_PREFIX, type Service } from "./service.js";
import { OPEN_STATUSES } from "./tasks.js";
import { parseAsk } from "./model.js";
import type { Asks } from "./asks.js";

/**
 * Q3 (pending mk): the Blocks count is direct refs only, so a card that blocks a card that blocks
 * five things counts one. Unknown ref kinds are shown but not counted. The one place to change if
 * mk rules for transitive counting.
 */
export function blocksCount(refs: readonly string[]): number {
  const seen = new Set<string>();
  for (const r of refs) {
    const at = r.indexOf(":");
    if (at > 0 && COUNTED.has(r.slice(0, at))) seen.add(r);
  }
  return seen.size;
}

export type BindingState = "suggested" | "confirmed" | "rejected";
export type BindingResult = { ok: true; state: BindingState } | { ok: false; status: number; error: string };

/**
 * Q5 (pending mk): mk confirms or rejects a tasks-project to Home-project binding once, in Settings.
 * A name match is only ever `suggested` (the poller). This is the single write path to a binding's
 * state; it is reachable from the RPC handler only, never from the CLI or the vizier's rule path.
 * `home_project`, when given, must be a project serve knows. It may re-point an unconfirmed row.
 */
export function setBinding(
  db: Database.Database,
  i: { tasks_project_id: string; state: "confirmed" | "rejected"; home_project?: string },
  ctx: { now: string; knownProjects: readonly string[]; record: (type: string, detail: unknown) => void },
): BindingResult {
  const row = db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(i.tasks_project_id) as { home_project: string; state: BindingState } | undefined;
  if (i.home_project !== undefined && !ctx.knownProjects.includes(i.home_project)) return { ok: false, status: 400, error: `unknown Home project ${JSON.stringify(i.home_project)}` };
  const home = i.home_project ?? row?.home_project;
  if (home === undefined) return { ok: false, status: 404, error: "no binding for this tasks project; give home_project" };
  if (i.state === "confirmed" && !ctx.knownProjects.includes(home)) return { ok: false, status: 400, error: `unknown Home project ${JSON.stringify(home)}` };
  db.prepare(
    `INSERT INTO project_bindings(tasks_project_id, home_project, state, suggested_at, confirmed_at) VALUES (@id, @home, @state, @at, @confirmed)
     ON CONFLICT(tasks_project_id) DO UPDATE SET home_project = @home, state = @state, confirmed_at = @confirmed`,
  ).run({ id: i.tasks_project_id, home, state: i.state, at: ctx.now, confirmed: i.state === "confirmed" ? ctx.now : null });
  ctx.record(i.state === "confirmed" ? "binding-confirmed" : "binding-rejected", { tasks_project_id: i.tasks_project_id, home_project: home, was: row?.state ?? null });
  return { ok: true, state: i.state };
}

/**
 * Remove a binding row (the vizier's `bb home unbind`), returning the tasks project to unbound. The
 * poller may suggest it again; a suggestion never opens a delegation. Recorded with who removed it.
 */
export function removeBinding(
  db: Database.Database,
  i: { tasks_project_id: string },
  ctx: { record: (type: string, detail: unknown) => void },
): { ok: true; was: { home_project: string; state: BindingState } } | { ok: false; status: number; error: string } {
  const row = db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(i.tasks_project_id) as { home_project: string; state: BindingState } | undefined;
  if (!row) return { ok: false, status: 404, error: "no binding for this tasks project" };
  db.prepare("DELETE FROM project_bindings WHERE tasks_project_id = ?").run(i.tasks_project_id);
  ctx.record("binding-removed", { tasks_project_id: i.tasks_project_id, home_project: row.home_project, was: row.state });
  return { ok: true, was: row };
}

export interface QueueRow {
  /** The open decision id, or `card:<task id>` for a display-only card. */
  id: string;
  decision_id: string | null;
  task_id: string;
  card_key: string | null;
  tasks_project_id: string | null;
  binding_state: BindingState | null;
  project: string | null;
  title: string;
  refs: { ref: string; counted: boolean }[];
  blocks_count: number;
  created_at: string;
  thread: string | null;
  pinned: boolean;
  /** Present only on an open generation: the ask, as stored. */
  ask: { question: string; recommendation?: string; options: { id: string; label: string; kind: string; reversible?: boolean; instruction?: string }[] } | null;
  revision: string | null;
  mentions: number;
  /** Why the card is display-only (no pick buttons); null on an open generation. */
  display_reason: string | null;
  display_only: boolean;
  /** The generation of the vizier-ruled generation this one overrides. */
  overrides_generation: number | null;
  changed_after_ruling: boolean;
  root: { state: string | null; reason: string | null };
}

export interface QueueData {
  rows: QueueRow[];
  legacy: {
    count: number;
    owed: { id: string; project: string; thread: string; subject: string; filed_at: string; revision: string; mentions: number; ask: unknown }[];
    runbook: ReturnType<Asks["runbook"]>;
    machine: ReturnType<Asks["lists"]>;
  };
  bindings: { tasks_project_id: string; home_project: string; state: BindingState; suggested_at: string | null; confirmed_at: string | null }[];
  /** Tasks projects with open cards and no binding row (no name match), for mk to bind by hand. */
  unbound: { tasks_project_id: string; cards: number; targets: string[] }[];
  inactive_projects: string[];
}

const ms = (iso: string | null | undefined): number => {
  const t = Date.parse(iso ?? "");
  return Number.isNaN(t) ? Number.MAX_SAFE_INTEGER : t;
};

/** Plan 1.4 order: derived blocks count, highest first; createdAt, oldest first; id. */
export function compareRows(a: QueueRow, b: QueueRow): number {
  return b.blocks_count - a.blocks_count || ms(a.created_at) - ms(b.created_at) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
}

/** Pinned rows (those naming the current thread) first, each group in plan 1.4 order. */
export function sortRows(rows: QueueRow[]): QueueRow[] {
  return [...rows].sort((a, b) => Number(b.pinned) - Number(a.pinned) || compareRows(a, b));
}

type CardRow = Record<string, any>;

export function buildQueue(svc: Service, asks: Asks, opts: { thread?: string } = {}): QueueData {
  const db = svc.store.db;
  const thread = opts.thread;
  const bindings = db.prepare("SELECT tasks_project_id, home_project, state, suggested_at, confirmed_at FROM project_bindings ORDER BY tasks_project_id").all() as QueueData["bindings"];
  const bstate = new Map(bindings.map((b) => [b.tasks_project_id, b.state]));
  const refsOf = (decisionId: string): string[] => (db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(decisionId) as { ref: string }[]).map((r) => r.ref);
  const shape = (refs: string[]) => refs.map((ref) => ({ ref, counted: blocksCount([ref]) === 1 }));
  const pinnedBy = (refs: string[], asking: string | null) => thread !== undefined && (asking === thread || refs.includes(`thread:${thread}`));
  const cardOf = (taskId: string) => db.prepare("SELECT * FROM cards WHERE task_id = ?").get(taskId) as CardRow | undefined;
  const rows: QueueRow[] = [];

  const owed = svc.owed();
  for (const d of owed.filter((x) => x.source === "card")) {
    const c = cardOf(String(d.task_id));
    const refs = refsOf(d.id);
    const over = svc.store.overriddenRuling(d.id);
    const prior = over ? (svc.store.decision(over.decision_id) as { generation: number | null } | undefined) : undefined;
    const ask = parseAsk(JSON.parse(d.body_json));
    const rootOk = c?.root_state === "verified";
    rows.push({
      id: d.id,
      decision_id: d.id,
      task_id: String(d.task_id),
      card_key: c?.card_key ?? null,
      tasks_project_id: (d.tasks_project_id as string | null) ?? null,
      binding_state: bstate.get(String(d.tasks_project_id)) ?? null,
      project: d.project,
      title: d.subject,
      refs: shape(refs),
      blocks_count: blocksCount(refs),
      created_at: c?.created_at ?? d.filed_at,
      thread: c?.asking_thread ?? (d.thread || null),
      pinned: pinnedBy(refs, c?.asking_thread ?? (d.thread || null)),
      ask: { question: ask.question, ...(ask.recommendation ? { recommendation: ask.recommendation } : {}), options: ask.options as never },
      revision: d.revision,
      mentions: svc.store.mentions(d.id).length,
      display_reason: null,
      display_only: false,
      overrides_generation: prior?.generation ?? null,
      changed_after_ruling: false,
      root: { state: rootOk ? "verified" : (c?.root_state ?? null), reason: rootOk ? null : (c?.root_reason ?? "root not verified") },
    });
  }

  const statuses = OPEN_STATUSES.map((s) => `'${s}'`).join(",");
  const display = db
    .prepare(
      `SELECT * FROM cards WHERE deleted_at IS NULL AND (
         (state IN ('display','closed') AND display_reason IS NOT NULL AND labelled = 1 AND status IN (${statuses}))
         OR (state = 'ruled' AND changed_after_ruling = 1 AND labelled = 1))
       ORDER BY created_at, task_id`,
    )
    .all() as CardRow[];
  for (const c of display) {
    let refs: string[] = [];
    try {
      const v = JSON.parse(c.blocks_json ?? "[]");
      if (Array.isArray(v)) refs = v.filter((x): x is string => typeof x === "string");
    } catch {
      /* unreadable: none */
    }
    const reason = c.state === "ruled" ? "changed after ruling" : String(c.display_reason).replace(ROUTING_PREFIX, "");
    rows.push({
      id: `card:${c.task_id}`,
      decision_id: null,
      task_id: c.task_id,
      card_key: c.card_key ?? null,
      tasks_project_id: c.project_id ?? null,
      binding_state: bstate.get(String(c.project_id)) ?? null,
      project: null,
      title: c.title ?? "",
      refs: shape(refs),
      blocks_count: blocksCount(refs),
      created_at: c.created_at ?? "",
      thread: c.asking_thread ?? null,
      pinned: pinnedBy(refs, c.asking_thread ?? null),
      ask: null,
      revision: null,
      mentions: 0,
      display_reason: reason,
      display_only: true,
      overrides_generation: null,
      changed_after_ruling: c.changed_after_ruling === 1,
      root: { state: c.root_state ?? null, reason: c.root_reason ?? null },
    });
  }

  const legacyOwed = owed.filter((x) => x.source !== "card");
  const runbook = asks.runbook();
  const machine = asks.lists();
  const unbound = (
    db
      .prepare(
        `SELECT project_id, COUNT(*) AS cards FROM cards WHERE labelled = 1 AND deleted_at IS NULL AND project_id IS NOT NULL
           AND status IN ('backlog','todo','in_progress','in_review')
           AND project_id NOT IN (SELECT tasks_project_id FROM project_bindings) GROUP BY project_id ORDER BY project_id`,
      )
      .all() as { project_id: string; cards: number }[]
  ).map((u) => {
    const asked = db.prepare("SELECT DISTINCT d.project FROM decisions d JOIN cards c ON c.task_id = d.task_id WHERE c.project_id = ? AND c.deleted_at IS NULL AND d.withdrawn_at IS NULL").all(u.project_id) as { project: string }[];
    const targets = asked.map((x) => x.project).sort();
    return { tasks_project_id: u.project_id, cards: u.cards, targets };
  });
  let inactive: string[] = [];
  try {
    const v = JSON.parse(svc.store.setting("delegation.projects_inactive") ?? "[]");
    if (Array.isArray(v)) inactive = v.filter((x): x is string => typeof x === "string");
  } catch {
    /* unreadable: none */
  }
  return {
    rows: sortRows(rows),
    legacy: {
      count: legacyOwed.length + runbook.reduce((n, g) => n + g.items.length, 0) + machine.lane.length + machine.asks.length,
      owed: legacyOwed.map((d) => ({ id: d.id, project: d.project, thread: d.thread, subject: d.subject, filed_at: String(d.filed_at), revision: d.revision, mentions: svc.store.mentions(d.id).length, ask: parseAsk(JSON.parse(d.body_json)) })),
      runbook,
      machine,
    },
    bindings,
    unbound,
    inactive_projects: inactive,
  };
}
