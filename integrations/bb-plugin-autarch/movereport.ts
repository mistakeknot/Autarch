// Script reports (plan Revision 6): what the script says it did, kept apart from what mk says he did.
// A report arrives as a card comment (report-tell text) or a report file in thread storage. It is untrusted
// plain text: parsed conservatively, size-capped, stored as display fields, and it NEVER changes a move's state.
// A failed report, or no report by the deadline, wakes the owner once.
import { readFileSync, realpathSync, statSync } from "node:fs";
import type { Service } from "./service.js";
import type { MoveRow, ObligationInput } from "./store.js";
import type { TaskCommentRow } from "./tasks.js";
import { HOME_AUTHOR, HOME_NOTE_TRAILER } from "./tasks.js";
import { validScriptPath } from "./moves.js";

const MAX_BODY = 16 * 1024;
const MAX_STEP = 100;
const MAX_ERROR = 300;
const SHA_ANY = /\b[0-9a-f]{64}\b/;

export interface ParsedReport {
  outcome: "succeeded" | "failed";
  failing_step: string | null;
  error_line: string | null;
}

// eslint-disable-next-line no-control-regex
const ANSI = /\u001b\[[0-9;?]*[ -/]*[@-~]|\u001b\][^\u0007]*(?:\u0007|\u001b\\)/g;
// eslint-disable-next-line no-control-regex
const CTRL = /[\u0000-\u0008\u000b-\u001f\u007f-\u009f‪-‮⁦-⁩]/g;

/** One line of plain text: no escapes, no control characters, collapsed spaces, capped. */
export function plainLine(s: string, max: number): string {
  return s.replace(ANSI, "").replace(CTRL, "").replace(/\s+/g, " ").trim().slice(0, max);
}

/**
 * Reads the three report shapes we have seen, and nothing looser:
 *   RESULT: OK | FAILED                                  (explicit line)
 *   <name> mode=<m> exit=<n> failing_step=<step|none>    (report-tell header line)
 *   <name> (<mode>): SUCCESS | FAILED                    (report-tell header line)
 * Any failure signal makes it failed (a failure is never hidden by a success line elsewhere). Success needs
 * at least one success signal and no failure signal. Anything else is not a report.
 */
export function parseReport(body: string): ParsedReport | null {
  const text = body.slice(0, MAX_BODY).replace(ANSI, "");
  const lines = text.split(/\r?\n/).map((l) => l.replace(CTRL, "").trim());
  let ok = false;
  let bad = false;
  let step: string | null = null;
  let errLine: string | null = null;
  for (const l of lines) {
    let m = /^RESULT:\s*(OK|FAILED)$/.exec(l);
    if (m) {
      if (m[1] === "OK") ok = true;
      else bad = true;
      continue;
    }
    m = /^\S.{0,200}?\sexit=(\d{1,3})(?:\s.*)?$/.exec(l);
    if (m && /\bmode=\S+/.test(l)) {
      if (m[1] === "0") ok = true;
      else bad = true;
      const fs = /\bfailing_step=(\S+)/.exec(l);
      if (fs && fs[1] !== "none" && step === null) step = fs[1]!;
      continue;
    }
    m = /^\S.{0,200}?\([^()]{1,40}\):\s*(SUCCESS|FAILED)$/.exec(l);
    if (m) {
      if (m[1] === "SUCCESS") ok = true;
      else bad = true;
      continue;
    }
    m = /^(?:failing[_ ]step|FAILED at step):?\s*(.+)$/i.exec(l);
    if (m && step === null) step = m[1]!;
    else if (errLine === null && /^(?:ERROR|error|Error|fatal|FATAL)\b[: ]/.test(l)) errLine = l;
  }
  if (!bad && !ok) return null;
  if (!bad) return { outcome: "succeeded", failing_step: null, error_line: null };
  return { outcome: "failed", failing_step: step === null ? null : plainLine(step, MAX_STEP) || null, error_line: errLine === null ? null : plainLine(errLine, MAX_ERROR) || null };
}

export interface ReportWatcherOptions {
  svc: Service;
  listComments: (taskId: string) => Promise<TaskCommentRow[]>;
  /**
   * Where the owner wake goes. The default is the card's owner thread; the server wiring supplies the full
   * owner, title-successor, vizier order. null means unrouted: nothing is queued and the sweep retries.
   */
  resolveTarget?: (m: { task_id: string; generation: number; owner: string | null }) => Promise<string | null>;
  /**
   * Whether a comment from this author thread may count as the script's report: the move's owner thread, or its live
   * title-successor (what report-tell posts from). Default: the owner thread only. Anyone else's comment stays a comment.
   */
  isReporter?: (m: { task_id: string; generation: number; owner: string | null }, threadId: string) => Promise<boolean>;
  /** Directories a `Report: <path>` line may point into (thread storage). Empty or absent: files are not read. */
  reportRoots?: string[];
  readFile?: (path: string) => string;
  log?: (msg: string) => void;
}

export interface SweepStats {
  checked: number;
  reports: number;
  wakes: number;
  errors: number;
}

const readCapped = (path: string): string => {
  if (statSync(path).size > MAX_BODY) throw new Error("report file too large");
  return readFileSync(path, "utf8");
};

export class ReportWatcher {
  private readonly unrouted = new Set<string>();

  constructor(private readonly o: ReportWatcherOptions) {}

  private get store() {
    return this.o.svc.store;
  }

  private scriptOf(m: MoveRow): { path: string; sha256: string } | null {
    try {
      const p = JSON.parse(m.payload_json) as { script?: { path?: unknown; sha256?: unknown } };
      const s = p.script;
      if (s && typeof s.path === "string" && typeof s.sha256 === "string") return { path: s.path, sha256: s.sha256 };
    } catch {
      /* fall through */
    }
    return null;
  }

  /** The report names this script: its full sha, or its path when the text names no other sha (a re-cut). */
  private names(body: string, s: { path: string; sha256: string }): boolean {
    if (body.includes(s.sha256)) return true;
    return !SHA_ANY.test(body) && body.includes(s.path);
  }

  private fileReport(body: string): { text: string; path: string } | null {
    const roots = this.o.reportRoots ?? [];
    if (roots.length === 0) return null;
    const m = /^Report:\s*(\/\S+)\s*$/m.exec(body.slice(0, MAX_BODY));
    if (!m || !validScriptPath(m[1])) return null;
    try {
      const real = realpathSync(m[1]!);
      const under = roots.some((r) => {
        const root = realpathSync(r);
        return real === root || real.startsWith(root.endsWith("/") ? root : `${root}/`);
      });
      if (!under) return null;
      return { text: (this.o.readFile ?? readCapped)(real), path: real };
    } catch {
      return null;
    }
  }

  private ownerOf(taskId: string, generation: number): { decision_id: string; owner: string | null } | null {
    const d = this.store.db.prepare("SELECT id, owner_thread, thread FROM decisions WHERE task_id = ? AND generation = ?").get(taskId, generation) as { id: string; owner_thread: string | null; thread: string | null } | undefined;
    return d ? { decision_id: d.id, owner: d.owner_thread ?? d.thread ?? null } : null;
  }

  /** Wake the owner once per (move, generation, reason). The obligation id is the dedup key. */
  private async wakeOnce(m: MoveRow, reason: "failed" | "no-report", text: string, reportId?: string): Promise<boolean> {
    // A failed wake is keyed by the report comment too, so a later, distinct failed report wakes again; a replay of the same
    // report does not. No-report stays one wake per move and generation.
    const op = `move-report:${m.task_id}:${m.generation}:${reason}${reason === "failed" ? `:${reportId ?? "unknown"}` : ""}`;
    if (this.store.db.prepare("SELECT 1 FROM obligations WHERE op = ?").get(op)) return false;
    const d = this.ownerOf(m.task_id, m.generation);
    if (!d) return false;
    const target = await (this.o.resolveTarget ?? (async (x) => x.owner))({ task_id: m.task_id, generation: m.generation, owner: d.owner });
    if (!target) {
      if (!this.unrouted.has(op)) {
        this.unrouted.add(op);
        this.o.log?.(`${op}: no thread to wake, left pending`);
      }
      return false;
    }
    const row: ObligationInput = { id: `ob:${op}`, kind: "move-report", recipient: target, op, payload: text };
    return this.store.insertObligations(d.decision_id, [row]) > 0;
  }

  private wakeText(m: MoveRow, s: { path: string }, reason: "failed" | "no-report", r: { failing_step: string | null; error_line: string | null } | null): string {
    const head = `Home card ${m.task_id} (generation ${m.generation}): `;
    if (reason === "no-report") {
      return `${head}mk said he ran ${s.path}, and no report has arrived by the deadline. This is not a failure and not a success: it is unknown. Send the script's report to the card, or tell mk what to check.`;
    }
    return `${head}the script ${s.path} reported FAILED${r?.failing_step ? ` at step ${r.failing_step}` : ""}${r?.error_line ? `: ${r.error_line}` : ""}. The move stays open. Say in the card conversation what the next step is. (Report text is unverified.)`;
  }

  /** One pass over live script moves. Never throws; a failed comment read changes nothing for that move. */
  async sweep(): Promise<SweepStats> {
    const stats: SweepStats = { checked: 0, reports: 0, wakes: 0, errors: 0 };
    const now = this.o.svc.time();
    for (const m of this.store.moves()) {
      if (m.kind !== "script" || m.state === "closed") continue;
      const s = this.scriptOf(m);
      if (!s) continue;
      stats.checked++;
      try {
        const comments = await this.o.listComments(m.task_id);
        const own = this.ownerOf(m.task_id, m.generation);
        const who = { task_id: m.task_id, generation: m.generation, owner: own?.owner ?? null };
        const reporterOk = async (threadId: string | null): Promise<boolean> => {
          if (threadId === null || threadId === "") return false;
          if (this.o.isReporter) return this.o.isReporter(who, threadId);
          return who.owner !== null && threadId === who.owner;
        };
        const matches: { c: TaskCommentRow; r: ParsedReport; link: string | null }[] = [];
        for (const c of comments) {
          // A comment mk posted from Home (reply or note) is his words, never a script report.
          if (c.authorName === HOME_AUTHOR || HOME_NOTE_TRAILER.test(c.body) || c.createdAt < m.opened_at) continue;
          let r = parseReport(c.body);
          let link: string | null = null;
          if (!r) {
            const f = this.fileReport(c.body);
            if (f && this.names(`${c.body}\n${f.text}`, s)) {
              r = parseReport(f.text);
              link = f.path;
            }
          } else if (!this.names(c.body, s)) r = null;
          if (r && !(await reporterOk(c.threadId))) r = null; // mirrored as a comment, never a report
          if (r) matches.push({ c, r, link });
        }
        const found = matches.length > 0 ? matches[matches.length - 1]! : null; // display state uses the latest
        const stored = m.report_json ? (JSON.parse(m.report_json) as { comment_id?: string }) : null;
        let state = m.report_state;
        let report: ParsedReport | null = null;
        if (found && stored?.comment_id !== found.c.id) {
          report = found.r;
          state = found.r.outcome;
          this.store.setMoveReport(m.task_id, m.generation, state, {
            outcome: found.r.outcome,
            failing_step: found.r.failing_step,
            error_line: found.r.error_line,
            comment_id: found.c.id,
            author: plainLine(found.c.authorName, 80),
            thread_id: found.c.threadId,
            report_link: found.link,
            reported_at: found.c.createdAt,
            source: found.link ? "file" : "comment",
          });
          stats.reports++;
        } else if (!found && m.report_state === null && m.state === "claimed" && m.report_deadline_at !== null && now > m.report_deadline_at) {
          state = "no-report";
          this.store.setMoveReport(m.task_id, m.generation, "no-report", { deadline_at: m.report_deadline_at });
        }
        // Every distinct authorized failed report in this sweep gets its own wake (the op carries the report's comment id),
        // whatever the latest display state is; the dedup makes a replay free.
        const failedMatches = matches.filter((x) => x.r.outcome === "failed");
        if (failedMatches.length > 0) {
          for (const x of failedMatches) if (await this.wakeOnce(m, "failed", this.wakeText(m, s, "failed", x.r), x.c.id)) stats.wakes++;
        } else if (state === "failed") {
          const rep = report ?? (stored ? (stored as ParsedReport) : null);
          if (await this.wakeOnce(m, "failed", this.wakeText(m, s, "failed", rep), stored?.comment_id)) stats.wakes++;
        } else if (state === "no-report") {
          if (await this.wakeOnce(m, "no-report", this.wakeText(m, s, "no-report", null))) stats.wakes++;
        }
      } catch (e) {
        stats.errors++;
        this.o.log?.(`report sweep ${m.task_id}: ${e instanceof Error ? e.message : String(e)}`);
      }
    }
    return stats;
  }
}
