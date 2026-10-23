// Catch-up: the facts mk has not seen yet, ranked [V2] [V9] [V10].
//
// Facts come from the store: turn failures the plugin recorded for threads that have filed
// asks, wakes that could not be delivered, open asks, delegated rulings (pinned until mk's
// account has a seen row), and routine items (mk's rulings, closed blockers) collapsed per
// project. Vizier notes follow the facts. `markAllSeen(snapshot)` marks exactly the ids the
// panel rendered expanded and visible; nothing else, ever [D-12].
import { randomUUID } from "node:crypto";
import type { Delegation } from "./delegation.js";
import type { Service } from "./service.js";

/** A `notice` changes what Home does (a vizier adoption, a delegation-settings change): the UI acknowledges it, dwell never does. */
export type CatchupKind = "failure" | "owed" | "delegated" | "routine" | "note" | "notice";
export interface CatchupItem {
  /** The seen-row key; for a routine group, `routine:<project>`. */
  item: string;
  kind: CatchupKind;
  project?: string;
  at: string;
  text: string;
  decision?: string;
  /** A routine group's own seen-row keys. */
  members?: string[];
  /** One line per member, in the same order, so a routine group can be read before it is marked. */
  lines?: string[];
  /** A routine group's unseen count in full; `members` and `lines` carry only the newest of them (see capRoutine). */
  total?: number;
  /** A note's cited facts. */
  cites?: string[];
}

const MK = "mk";
const NOTE_MAX = 1000;
/** Most members (and lines) one routine group sends, newest first; the rest is counted in `total` and shows after these are marked. */
export const ROUTINE_GROUP_MAX = 50;
/** Most members the routine groups send together, so a catch-up reply stays small however many rulings have piled up. */
export const ROUTINE_TOTAL_MAX = 300;

/** Trims each group to its newest members within the per-group and total budgets, keeping members and lines paired and in order. Groups arrive newest first, so the budget goes to them in that order. */
export function capRoutine(groups: CatchupItem[], perGroup = ROUTINE_GROUP_MAX, total = ROUTINE_TOTAL_MAX): void {
  let left = total;
  for (const g of groups) {
    const members = g.members ?? [];
    const keep = Math.max(0, Math.min(members.length, perGroup, left));
    g.total = members.length;
    g.members = keep === 0 ? [] : members.slice(-keep);
    g.lines = keep === 0 ? [] : (g.lines ?? []).slice(-keep);
    left -= keep;
  }
}

interface Dec {
  id: string;
  project: string;
  subject: string;
  thread: string;
  body_json: string;
  filed_at: string;
}

export class Catchup {
  constructor(
    private readonly svc: Service,
    private readonly dele: Delegation,
  ) {}

  private get store() {
    return this.svc.store;
  }
  private get db() {
    return this.store.db;
  }
  private dec(id: string): Dec | undefined {
    return this.store.decision(id) as Dec | undefined;
  }
  private title(id: string): string {
    const d = this.dec(id);
    if (!d) return id;
    if (d.subject) return d.subject;
    try {
      return String(JSON.parse(d.body_json).question ?? id).slice(0, 80);
    } catch {
      return id;
    }
  }

  // ---- facts recorded by the plugin --------------------------------------------

  /** Record a failed run, only for a thread that has filed asks; a repeated request id is ignored. */
  recordTurnFailed(threadId: string, requestId: string): boolean {
    const d = this.db.prepare("SELECT project FROM decisions WHERE thread = ? ORDER BY filed_at DESC, rowid DESC LIMIT 1").get(threadId) as { project: string } | undefined;
    if (!d) return false;
    const seen = this.db.prepare("SELECT 1 FROM events WHERE type = 'turn-failed' AND json_extract(detail_json, '$.request_id') = ?").get(requestId);
    if (seen) return false;
    this.store.recordEvent("turn-failed", null, { thread: threadId, request_id: requestId, project: d.project });
    return true;
  }

  factExists(id: string): boolean {
    if (this.dec(id) || this.store.obligation(id)) return true;
    const m = /^(ruling|filed|closed|failed|undeliverable):(.+)$/.exec(id);
    if (!m) return false;
    if (m[1] === "failed") return !!this.db.prepare("SELECT 1 FROM events WHERE type = 'turn-failed' AND seq = ?").get(Number(m[2]));
    if (m[1] === "undeliverable") return !!this.store.obligation(m[2]!);
    return !!this.dec(m[2]!);
  }

  /** A vizier's note. Refused unless every cited id is an existing fact. */
  addNote(text: string, cites: string[]): { ok: true; id: string } | { ok: false; error: string } {
    const t = (text ?? "").trim();
    if (t === "" || t.length > NOTE_MAX) return { ok: false, error: `a note is 1-${NOTE_MAX} characters` };
    if (!Array.isArray(cites) || cites.length === 0) return { ok: false, error: "a note must cite at least one fact" };
    const unknown = cites.filter((c) => !this.factExists(c));
    if (unknown.length > 0) return { ok: false, error: `unknown fact: ${unknown.join(", ")}` };
    const id = `note_${randomUUID().replace(/-/g, "").slice(0, 12)}`;
    this.db.prepare("INSERT INTO notes(id, at, text, cites_json) VALUES (?, ?, ?, ?)").run(id, this.svc.time(), t, JSON.stringify(cites));
    return { ok: true, id };
  }

  // ---- the ranked list ------------------------------------------------------------

  items(): CatchupItem[] {
    const seen = (item: string) => this.store.hasSeen(MK, item);
    const failures: CatchupItem[] = [];
    for (const e of this.db.prepare("SELECT seq, at, detail_json FROM events WHERE type = 'turn-failed' ORDER BY seq DESC").all() as { seq: number; at: string; detail_json: string }[]) {
      const item = `failed:${e.seq}`;
      if (seen(item)) continue;
      const d = JSON.parse(e.detail_json) as { thread?: string; project?: string; request_id?: string };
      failures.push({ item, kind: "failure", project: d.project, at: e.at, text: `A run failed on thread ${d.thread ?? "?"} (request ${d.request_id ?? "?"}).` });
    }
    for (const o of this.svc.undeliverable()) {
      const item = `undeliverable:${o.id}`;
      if (seen(item)) continue;
      failures.push({
        item,
        kind: "failure",
        decision: o.decision_id,
        project: this.dec(o.decision_id)?.project,
        at: o.updated_at,
        text: `A ${o.kind} message to thread ${o.recipient ?? "?"} could not be delivered (${o.op ?? o.id}).`,
      });
    }
    failures.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));

    const owed: CatchupItem[] = this.svc.owed().map((d) => ({
      item: `owed:${d.id}`,
      kind: "owed" as const,
      project: d.project,
      at: d.filed_at as string,
      decision: d.id,
      text: `Waiting for you: ${d.subject || this.title(d.id)}`,
    }));

    const delegated: CatchupItem[] = this.dele.pinned().map((p) =>
      p.decision
        ? { item: p.item, kind: "delegated" as const, at: p.at, decision: p.decision, project: this.dec(p.decision)?.project, text: `The vizier ruled on ${this.title(p.decision)}.` }
        : { item: p.item, kind: "notice" as const, at: p.at, text: p.text ?? "Delegation settings changed; delegated rulings stay suspended until you see this." },
    );

    const groups = new Map<string, CatchupItem>();
    const ats = new Map<string, string[]>(); // each group's member times, parallel to members and lines
    const add = (project: string, member: string, at: string, line: string) => {
      const g = groups.get(project) ?? { item: `routine:${project}`, kind: "routine" as const, project, at, text: "", members: [] as string[], lines: [] as string[] };
      g.members!.push(member);
      g.lines!.push(line);
      (ats.get(project) ?? ats.set(project, []).get(project)!).push(at);
      if (at > g.at) g.at = at;
      groups.set(project, g);
    };
    for (const p of this.db.prepare(`SELECT k.decision_id, k.picked_at, d.project FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE k."by" = 'mk' ORDER BY k.picked_at, k.rowid`).all() as { decision_id: string; picked_at: string; project: string }[]) {
      const item = `ruling:${p.decision_id}`;
      if (!seen(item)) add(p.project, item, p.picked_at, `You ruled on ${this.title(p.decision_id)}.`);
    }
    for (const e of this.db.prepare("SELECT e.decision_id, e.at, d.project FROM events e JOIN decisions d ON d.id = e.decision_id WHERE e.type IN ('resolved','withdrawn') ORDER BY e.seq").all() as { decision_id: string; at: string; project: string }[]) {
      const item = `closed:${e.decision_id}`;
      if (!seen(item)) add(e.project, item, e.at, `Closed: ${this.title(e.decision_id)}.`);
    }
    const routine = [...groups.values()].sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
    for (const g of routine) g.text = `${g.members!.length} routine update${g.members!.length === 1 ? "" : "s"} in ${g.project}`;
    // Rulings are read before closures, so a group is not yet in time order; the cap keeps the newest, so order first (stable).
    for (const g of routine) {
      const t = ats.get(g.project!)!;
      const order = t.map((_, i) => i).sort((a, b) => (t[a]! < t[b]! ? -1 : t[a]! > t[b]! ? 1 : a - b));
      g.members = order.map((i) => g.members![i]!);
      g.lines = order.map((i) => g.lines![i]!);
    }
    capRoutine(routine);

    const notes: CatchupItem[] = [];
    for (const n of this.db.prepare("SELECT id, at, text, cites_json FROM notes ORDER BY at, rowid").all() as { id: string; at: string; text: string; cites_json: string }[]) {
      const item = `note:${n.id}`;
      if (seen(item)) continue;
      notes.push({ item, kind: "note", at: n.at, text: `vizier's note: ${n.text}`, cites: JSON.parse(n.cites_json) as string[] });
    }
    const notices = delegated.filter((d) => d.kind === "notice");
    return [...notices, ...failures, ...owed, ...delegated.filter((d) => d.kind !== "notice"), ...routine, ...notes];
  }

  /**
   * Mark exactly the ids in the snapshot that are, right now, unseen items the panel could have
   * shown: never an open ask (it stays owed), a collapsed group's id, or anything unknown.
   */
  markAllSeen(snapshot: string[]): string[] {
    const valid = new Set<string>();
    for (const i of this.items()) {
      if (i.kind === "owed" || i.kind === "notice") continue;
      if (i.kind === "routine") for (const m of i.members ?? []) valid.add(m);
      else valid.add(i.item);
    }
    const marked: string[] = [];
    this.db
      .transaction(() => {
        for (const id of snapshot) {
          if (!valid.has(id) || marked.includes(id)) continue;
          this.store.markSeen(MK, id);
          marked.push(id);
        }
      })
      .immediate();
    return marked;
  }
}
