// The catch-up feed: what a thread is told about rulings when it starts or resumes.
// Only labels, ids, times and who ruled: never a question or an instruction, so the
// feed cannot smuggle text between threads [E-1].
import type Database from "better-sqlite3";

export interface FeedLine {
  decision: string;
  label: string;
  picked_at: string;
  by: string;
  project: string;
  status: "effective" | "void";
  supersedes?: string;
  /** For a void line: the replacement at the end of the chain, awaiting mk. */
  tip?: string;
}

export interface Feed {
  own: FeedLine[];
  project: FeedLine[];
}

export const FEED_BUDGET = 4096;
const MAX_LABEL_UNITS = 80;
const MAX_LINES = 10;
const OWN_DAYS = 30;
const PROJECT_DAYS = 14;
const DAY = 86_400_000;

const HEADER = "Rulings mk has made (newest first):";
const NOTE = "If your question is already answered above, do not file it again.";

const CONTROL = new RegExp("[\\u0000-\\u001f\\u007f-\\u009f\\u2028\\u2029]", "g");

/** Control characters stripped, then clipped to 80 UTF-16 units without splitting a surrogate pair. */
export function clipLabel(label: string): string {
  const s = label.replace(CONTROL, "");
  if (s.length <= MAX_LABEL_UNITS) return s;
  let cut = s.slice(0, MAX_LABEL_UNITS);
  const last = cut.charCodeAt(cut.length - 1);
  if (last >= 0xd800 && last <= 0xdbff) cut = cut.slice(0, -1);
  return cut;
}

export function renderLine(l: FeedLine): string {
  if (l.status === "void") return `void ${l.decision}: overridden, awaiting mk (${l.tip ?? ""})`;
  const at = `${l.picked_at.slice(0, 16)}Z`;
  return `ruled ${at} "${clipLabel(l.label)}" (${l.decision})${l.by === "vizier" ? " [vizier]" : ""}${
    l.supersedes ? `, supersedes ${l.supersedes}` : ""
  }`;
}

/** Header and filing note first, then own lines, then project lines; whole lines only, oldest dropped first. */
export function renderFeed(feed: Feed, budget: number = FEED_BUDGET): string {
  const parts = [HEADER];
  let used = HEADER.length + 1 + NOTE.length;
  let full = true;
  for (const l of [...feed.own, ...feed.project]) {
    if (!full) break;
    const text = renderLine(l);
    if (used + 1 + text.length > budget) {
      full = false;
      break;
    }
    parts.push(text);
    used += 1 + text.length;
  }
  parts.push(NOTE);
  return parts.join("\n");
}

interface Row {
  id: string;
  project: string;
  thread: string;
  supersedes: string | null;
  body_json: string;
  option_id: string;
  by: string;
  picked_at: string;
  replaced: number;
}

function labelOf(body: string, optionId: string): string {
  try {
    const o = (JSON.parse(body).options ?? []).find((x: { id: string }) => x.id === optionId);
    return o?.label ?? optionId;
  } catch {
    return optionId;
  }
}

/** The end of the supersedes chain that starts at id's replacement. */
function chainTip(db: Database.Database, id: string): { id: string; picked: boolean; withdrawn: boolean } {
  let cur = id;
  const seen = new Set<string>();
  for (;;) {
    seen.add(cur);
    const r = db.prepare("SELECT id FROM decisions WHERE supersedes = ?").get(cur) as { id: string } | undefined;
    if (!r || seen.has(r.id)) break;
    cur = r.id;
  }
  const d = db.prepare("SELECT withdrawn_at FROM decisions WHERE id = ?").get(cur) as { withdrawn_at: string | null };
  const picked = !!db.prepare("SELECT 1 FROM picks WHERE decision_id = ?").get(cur);
  return { id: cur, picked, withdrawn: !!d.withdrawn_at };
}

/** Whose rulings a thread sees as its own: the asking thread; a thread named by a `thread:<id>` Blocks ref
 *  on the card; and, for rows filed before cards (task_id NULL) only, a thread that mentioned the ask. */
const OWN_MATCH = `(d.thread = @thread
       OR EXISTS (SELECT 1 FROM decision_blocks b WHERE b.decision_id = d.id AND b.ref = 'thread:' || @thread)
       OR (d.task_id IS NULL AND EXISTS (SELECT 1 FROM mentions m WHERE m.decision_id = d.id AND m.thread = @thread)))`;

export function buildFeed(db: Database.Database, project: string, thread: string, nowMs: number): Feed {
  const sel = `SELECT d.id, d.project, d.thread, d.supersedes, d.body_json, k.option_id, k."by" AS by, k.picked_at,
       EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id) AS replaced
     FROM picks k JOIN decisions d ON d.id = k.decision_id
     WHERE d.project = @project AND k.picked_at >= @since`;
  const order = " ORDER BY k.picked_at DESC, d.id ASC LIMIT 400";
  const iso = (ms: number) => new Date(ms).toISOString();

  const mine = db
    .prepare(
      `${sel} AND ${OWN_MATCH}${order}`,
    )
    .all({ project, thread, since: iso(nowMs - OWN_DAYS * DAY) }) as Row[];
  const own: FeedLine[] = [];
  for (const r of mine) {
    const base = { decision: r.id, label: clipLabel(labelOf(r.body_json, r.option_id)), picked_at: r.picked_at, by: r.by, project: r.project };
    if (!r.replaced) {
      own.push({ ...base, status: "effective", ...(r.supersedes ? { supersedes: r.supersedes } : {}) });
      continue;
    }
    const tip = chainTip(db, r.id);
    if (!tip.picked && !tip.withdrawn) own.push({ ...base, status: "void", tip: tip.id });
  }
  const ownIds = new Set(mine.map((r) => r.id));

  const rows = db.prepare(`${sel}${order}`).all({ project, since: iso(nowMs - PROJECT_DAYS * DAY) }) as Row[];
  const proj: FeedLine[] = [];
  for (const r of rows) {
    if (ownIds.has(r.id) || r.replaced) continue;
    proj.push({
      decision: r.id,
      label: clipLabel(labelOf(r.body_json, r.option_id)),
      picked_at: r.picked_at,
      by: r.by,
      project: r.project,
      status: "effective",
      ...(r.supersedes ? { supersedes: r.supersedes } : {}),
    });
  }
  return { own: own.slice(0, MAX_LINES), project: proj.slice(0, MAX_LINES) };
}

// ---- agent configuration text [D-17] ---------------------------------------------

const CONFIGURE_HEADER = "Recent rulings (label only):";
const CONFIGURE_NOTE = [
  "When you need mk, end your turn by filing a card: run `autarch needs-mk file` (`bb home ask` is retired).",
  "Options default to needs-context.",
  "Give an option an `instruction` when you know what you'd do if mk picks it, and mark it `reversible` only if undoing it is cheap and local; never for a push, merge, deploy or release.",
  "If your question is already answered above, do not file it again.",
].join(" ");

/**
 * The instructions a thread starts with: header, own lines, project lines, then the filing note.
 * Whole lines only, at most `budget` UTF-16 units; the oldest project lines go first, then own lines.
 */
export function renderConfigure(own: FeedLine[], project: FeedLine[], budget: number = FEED_BUDGET): string {
  const parts = [CONFIGURE_HEADER];
  let used = CONFIGURE_HEADER.length + 1 + CONFIGURE_NOTE.length;
  const mine = new Set(own.map((l) => l.decision));
  for (const l of [...own, ...project.filter((p) => !mine.has(p.decision))]) {
    const text = renderLine(l);
    if (used + 1 + text.length > budget) break;
    parts.push(text);
    used += 1 + text.length;
  }
  parts.push(CONFIGURE_NOTE);
  return parts.join("\n");
}

const NO_THREAD = "\u0000";

/**
 * Two caches over the store: project lines shared by every thread of a project, and own lines
 * per thread. A miss builds from the store, so a thread starting before any refresh still sees
 * its project's rulings. `invalidate` (after a pick or override) and `refresh` (every 30 s)
 * keep them current; `configure` itself is synchronous, as `bb.agents.configure` requires.
 */
export class FeedCaches {
  private readonly project = new Map<string, FeedLine[]>();
  private readonly own = new Map<string, FeedLine[]>();

  constructor(
    private readonly db: () => Database.Database,
    private readonly now: () => number,
  ) {}

  private projectLines(project: string): FeedLine[] {
    let v = this.project.get(project);
    if (!v) {
      v = buildFeed(this.db(), project, NO_THREAD, this.now()).project;
      this.project.set(project, v);
    }
    return v;
  }

  private ownLines(project: string, thread: string): FeedLine[] {
    const key = `${project}\u0001${thread}`;
    let v = this.own.get(key);
    if (!v) {
      v = buildFeed(this.db(), project, thread, this.now()).own;
      this.own.set(key, v);
    }
    return v;
  }

  /** Drop everything; the next configure rebuilds from the store. */
  invalidate(): void {
    this.project.clear();
    this.own.clear();
  }

  /** Rebuild every cache that has been asked for. */
  refresh(): void {
    const projects = [...this.project.keys()];
    const owns = [...this.own.keys()];
    this.invalidate();
    for (const p of projects) this.projectLines(p);
    for (const k of owns) {
      const [p, t] = k.split("\u0001") as [string, string];
      this.ownLines(p, t);
    }
  }

  /** Instructions for a thread of a project, or undefined when both caches are empty. */
  configure(project: string, thread: string): string | undefined {
    const own = this.ownLines(project, thread);
    const proj = this.projectLines(project);
    if (own.length === 0 && proj.length === 0) return undefined;
    return renderConfigure(own, proj);
  }
}
