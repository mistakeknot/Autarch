// The `moves` RPC read model: the grouped rows with what the panel needs to render them. Commands are derived
// from the validated payload by the safe builder (moves.ts), never from card text, so the panel only displays them.
import { groupMoves, type MoveGroups } from "./moveselect.js";
import { scriptCommands, type MoveCommand } from "./moves.js";
import type { Service } from "./service.js";
import type { MoveRow } from "./store.js";

export interface MoveReportView {
  outcome: "succeeded" | "failed" | "no-report";
  failing_step: string | null;
  error_line: string | null;
  report_link: string | null;
  reported_at: string | null;
  deadline_at: string | null;
}

export interface MoveView {
  task_id: string;
  generation: number;
  kind: MoveRow["kind"];
  state: MoveRow["state"];
  title: string;
  owner: string | null;
  opened_at: string;
  claimed_at: string | null;
  skipped_at: string | null;
  /** True while mk has set the whole card aside (Later on the card): "Move back" undoes it. */
  card_later?: boolean;
  checked_at: string | null;
  report_deadline_at: string | null;
  /** pr and read: the link. context: null. */
  url: string | null;
  /** pr: the optional merge-card facts (what merging does, review verdict and link, why it is mk's). */
  pr: { summary: string | null; verdict: string | null; review_url: string | null; why: string | null } | null;
  /** context: the text mk is asked for. */
  need: string | null;
  script: { path: string; sha256: string } | null;
  commands: MoveCommand[];
  report: MoveReportView | null;
  closed_at: string | null;
  closed_by: string | null;
  evidence: string | null;
}

export interface MoveViewGroups {
  yourMove: MoveView[];
  reported: MoveView[];
  later: MoveView[];
  hidden: MoveView[];
  audit: MoveGroups["audit"];
}

const obj = (s: string | null): Record<string, unknown> | null => {
  if (!s) return null;
  try {
    const v = JSON.parse(s) as unknown;
    return v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
};
const str = (v: unknown): string | null => (typeof v === "string" ? v : null);

export function viewOf(svc: Service, m: MoveRow): MoveView {
  const p = obj(m.payload_json) ?? {};
  const card = svc.store.db.prepare("SELECT title FROM cards WHERE task_id = ?").get(m.task_id) as { title: string | null } | undefined;
  const dec = svc.store.db.prepare("SELECT owner_thread, thread FROM decisions WHERE task_id = ? AND generation = ?").get(m.task_id, m.generation) as { owner_thread: string | null; thread: string | null } | undefined;
  let script: MoveView["script"] = null;
  let commands: MoveCommand[] = [];
  const s = p.script as { path?: unknown; sha256?: unknown; args?: unknown; recover?: { path?: unknown; sha256?: unknown } | null } | undefined;
  if (m.kind === "script" && s && typeof s.path === "string" && typeof s.sha256 === "string") {
    script = { path: s.path, sha256: s.sha256 };
    const rec = s.recover && typeof s.recover.path === "string" && typeof s.recover.sha256 === "string" ? { path: s.recover.path, sha256: s.recover.sha256 } : null;
    commands = scriptCommands({ path: s.path, sha256: s.sha256, args: Array.isArray(s.args) ? s.args.filter((a): a is string => typeof a === "string") : [], recover: rec });
  }
  const r = obj(m.report_json);
  let report: MoveReportView | null = null;
  if (m.report_state === "succeeded" || m.report_state === "failed") {
    report = {
      outcome: m.report_state,
      failing_step: str(r?.failing_step),
      error_line: str(r?.error_line),
      report_link: str(r?.report_link),
      reported_at: str(r?.reported_at) ?? m.report_at,
      deadline_at: null,
    };
  } else if (m.report_state === "no-report") {
    report = { outcome: "no-report", failing_step: null, error_line: null, report_link: null, reported_at: null, deadline_at: m.report_deadline_at };
  }
  return {
    task_id: m.task_id,
    generation: m.generation,
    kind: m.kind,
    state: m.state,
    title: card?.title ?? m.task_id,
    owner: dec?.owner_thread ?? dec?.thread ?? null,
    opened_at: m.opened_at,
    claimed_at: m.claimed_at,
    skipped_at: m.skipped_at,
    card_later: svc.laterOf(m.task_id) !== null,
    checked_at: m.kind === "script" ? svc.checkedAt(m.task_id, m.generation) : null,
    report_deadline_at: m.report_deadline_at,
    url: str(p.url),
    need: str(p.need),
    pr: m.kind === "pr" ? { summary: str(p.summary), verdict: str(p.verdict), review_url: str(p.review_url), why: str(p.why) } : null,
    script,
    commands,
    report,
    closed_at: m.closed_at,
    closed_by: m.closed_by,
    evidence: m.evidence,
  };
}

export function moveViews(svc: Service, rows: readonly MoveRow[] = svc.store.moves()): MoveViewGroups {
  const g = groupMoves(rows, new Set(Object.keys(svc.laters())));
  const v = (list: MoveRow[]) => list.map((m) => viewOf(svc, m));
  return { yourMove: v(g.yourMove), reported: v(g.reported), later: v(g.later), hidden: v(g.hidden), audit: g.audit };
}
