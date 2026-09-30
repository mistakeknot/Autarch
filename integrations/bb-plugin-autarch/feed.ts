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

export function buildFeed(db: Database.Database, project: string, thread: string, nowMs: number): Feed {
  const sel = `SELECT d.id, d.project, d.thread, d.supersedes, d.body_json, k.option_id, k."by" AS by, k.picked_at,
       EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id) AS replaced
     FROM picks k JOIN decisions d ON d.id = k.decision_id
     WHERE d.project = @project AND k.picked_at >= @since`;
  const order = " ORDER BY k.picked_at DESC, d.id ASC LIMIT 400";
  const iso = (ms: number) => new Date(ms).toISOString();

  const mine = db
    .prepare(
      `${sel} AND (d.thread = @thread OR EXISTS (SELECT 1 FROM mentions m WHERE m.decision_id = d.id AND m.thread = @thread))${order}`,
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
