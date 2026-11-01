// The Archive tab (bead mk-gc2xm): a read-only list of decided cards (state ruled or closed), newest first, with
// search and project / date / pick filters. Reads data.db only; nothing here rules, reopens or writes.
import type Database from "better-sqlite3";
import { z } from "zod";

export const ArchiveQuery = z.object({
  search: z.string().max(200).optional(),
  project: z.string().max(40).optional(),
  from: z.string().max(40).optional(),
  to: z.string().max(40).optional(),
  pick: z.string().max(200).optional(),
  limit: z.number().int().min(1).max(200).optional(),
  offset: z.number().int().min(0).optional(),
});
export type ArchiveQuery = z.infer<typeof ArchiveQuery>;

export type ArchiveRow = {
  task_id: string;
  card_key: string;
  title: string;
  project: string;
  state: string;
  decided_at: string;
  pick_label: string | null;
  option_id: string | null;
  by: string | null;
  reason: string | null;
  generation: number | null;
  /** Closed cards that Home never recorded a pick for. */
  no_pick: boolean;
};

export type ArchiveView = { rows: ArchiveRow[]; total: number; projects: string[]; offset: number; limit: number };

const like = (s: string) => `%${s.replace(/[\\%_]/g, (c) => `\\${c}`)}%`;

function optionLabel(bodyJson: string | null, optionId: string | null): string | null {
  if (!bodyJson || !optionId) return null;
  try {
    const body = JSON.parse(bodyJson) as { options?: { id?: string; label?: string }[] };
    return body.options?.find((o) => o.id === optionId)?.label ?? null;
  } catch {
    return null;
  }
}

export function archiveView(db: Database.Database, q: ArchiveQuery): ArchiveView {
  const limit = q.limit ?? 50;
  const offset = q.offset ?? 0;
  const where = ["c.state IN ('ruled','closed')"];
  const args: Record<string, string> = {};
  if (q.project) {
    where.push("c.card_key LIKE @proj ESCAPE '\\'");
    args.proj = `${q.project.replace(/[\\%_]/g, (c) => `\\${c}`)}-%`;
  }
  if (q.from) {
    where.push("COALESCE(p.picked_at, c.updated_at) >= @from");
    args.from = q.from;
  }
  if (q.to) {
    // A bare date (what the date input gives) means the whole day, so the bound is the next midnight, exclusive.
    const day = /^\d{4}-\d{2}-\d{2}$/.test(q.to) ? new Date(`${q.to}T00:00:00.000Z`) : null;
    if (day && !Number.isNaN(day.getTime())) {
      const next = day;
      next.setUTCDate(next.getUTCDate() + 1);
      where.push("COALESCE(p.picked_at, c.updated_at) < @to");
      args.to = next.toISOString();
    } else {
      where.push("COALESCE(p.picked_at, c.updated_at) <= @to");
      args.to = q.to;
    }
  }
  if (q.pick) {
    where.push("(p.option_id = @pickExact OR EXISTS (SELECT 1 FROM json_each(d.body_json, '$.options') o WHERE json_extract(o.value, '$.id') = p.option_id AND json_extract(o.value, '$.label') LIKE @pickLike ESCAPE '\\'))");
    args.pickExact = q.pick;
    args.pickLike = like(q.pick);
  }
  if (q.search) {
    where.push("(c.card_key LIKE @s ESCAPE '\\' OR c.title LIKE @s ESCAPE '\\' OR p.reason LIKE @s ESCAPE '\\' OR p.option_id LIKE @s ESCAPE '\\' OR json_extract(d.body_json, '$.question') LIKE @s ESCAPE '\\' OR EXISTS (SELECT 1 FROM json_each(d.body_json, '$.options') o WHERE json_extract(o.value, '$.id') = p.option_id AND json_extract(o.value, '$.label') LIKE @s ESCAPE '\\'))");
    args.s = like(q.search);
  }
  // The latest generation of each card; cards with no decision (closed in tasks before Home saw them) still list.
  const from = `FROM cards c
    LEFT JOIN decisions d ON d.task_id = c.task_id AND d.generation = (SELECT MAX(generation) FROM decisions WHERE task_id = c.task_id)
    LEFT JOIN picks p ON p.decision_id = d.id
    WHERE ${where.join(" AND ")}`;
  const total = (db.prepare(`SELECT COUNT(*) AS n ${from}`).get(args) as { n: number }).n;
  const raw = db
    .prepare(
      `SELECT c.task_id, c.card_key, c.title, c.state, COALESCE(p.picked_at, c.updated_at) AS decided_at,
              p.option_id, p."by" AS by, p.reason, d.generation, d.body_json
       ${from} ORDER BY decided_at DESC, c.card_key DESC LIMIT @limit OFFSET @offset`,
    )
    .all({ ...args, limit, offset }) as Array<Record<string, string | number | null>>;
  const rows: ArchiveRow[] = raw.map((r) => ({
    task_id: String(r.task_id),
    card_key: String(r.card_key),
    title: String(r.title ?? ""),
    project: String(r.card_key).split("-")[0] ?? "",
    state: String(r.state),
    decided_at: String(r.decided_at ?? ""),
    pick_label: optionLabel(r.body_json as string | null, r.option_id as string | null),
    option_id: (r.option_id as string | null) ?? null,
    by: (r.by as string | null) ?? null,
    reason: (r.reason as string | null) ?? null,
    generation: (r.generation as number | null) ?? null,
    no_pick: r.option_id == null,
  }));
  const projects = (
    db.prepare("SELECT DISTINCT substr(card_key, 1, instr(card_key, '-') - 1) AS p FROM cards WHERE state IN ('ruled','closed') AND instr(card_key, '-') > 0 ORDER BY p").all() as { p: string }[]
  ).map((x) => x.p);
  return { rows, total, projects, offset, limit };
}
