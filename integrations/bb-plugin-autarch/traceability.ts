// Traceability measures for `bb home stats` (bead mk-8y30): how much thread work ends on a tasks card, and how much
// of the open backlog has gone quiet. Read-only: it lists tasks and threads and reads each thread's final output; it
// writes nothing and wakes no one. A failed read reports that part as an error instead of a partial number.
import { OPEN_STATUSES, type Task, type TaskProject } from "./tasks.js";

export const STALE_DAYS = 14;
const DAY_MS = 86_400_000;
const OUTPUT_CONCURRENCY = 8;
const TOP_PROJECTS = 10;

/** A tasks key such as PROJ-24. Only prefixes of real tracker projects count, so "GPT-6" or "SHA-256" do not. */
const KEY_RE = /\b([A-Z][A-Z0-9]{0,9})-(\d+)\b/g;
/** The opt-out a thread gives when its work needs no card: `no-card: <reason>` starting a line. A bare `no-card:` does not count. */
const NO_CARD_RE = /(?:^|\n)[ \t]*no-card:[ \t]*\S/i;

const ENDED = new Set(["idle", "error"]);

export type Ending = "task_key" | "no_card" | "neither";

export function classifyOutput(text: string, prefixes: ReadonlySet<string>): Ending {
  for (const m of text.matchAll(KEY_RE)) if (prefixes.has(m[1]!)) return "task_key";
  return NO_CARD_RE.test(text) ? "no_card" : "neither";
}

export interface ThreadLike {
  id: string;
  status: string;
  updatedAt: number;
  parentThreadId?: string | null;
}

export interface ThreadPageArgs {
  limit: number;
  offset: number;
  includeHidden: true;
  archived: boolean;
}

/** Live then archived threads, every page, deleted rows dropped. Any failed page rejects; a partial list is never returned. */
export async function listAllThreads(list: (a: ThreadPageArgs) => Promise<(ThreadLike & { deletedAt?: unknown })[]>): Promise<ThreadLike[]> {
  const rows = new Map<string, ThreadLike>();
  for (const archived of [false, true]) {
    let offset = 0;
    for (let page = 0; ; page++) {
      if (page >= 2000) throw new Error("thread list did not end");
      const got = await list({ limit: 100, offset, includeHidden: true, archived });
      if (got.length === 0) break;
      for (const t of got) if (!t.deletedAt) rows.set(t.id, { id: t.id, status: t.status, updatedAt: t.updatedAt, parentThreadId: t.parentThreadId ?? null });
      // Advance by what came back: a host that caps the page size skips nothing. Offset paging is not a snapshot: a
      // thread updated mid-read can shift rows and be missed or seen twice (deduplicated). The measure is a rate over
      // thousands of threads, so it accepts that rather than failing every read taken while threads are busy.
      offset += got.length;
    }
  }
  return [...rows.values()];
}

export interface TraceabilityDeps {
  listProjects(): Promise<TaskProject[]>;
  /** Open tasks of one project, all pages. */
  listOpenTasks(projectId: string): Promise<Task[]>;
  /** Live and archived threads; deleted ones excluded. */
  listThreads(): Promise<ThreadLike[]>;
  /** The thread's final output, or null when it has none. */
  output(threadId: string): Promise<string | null>;
}

interface Endings {
  ended: number;
  task_key: number;
  no_card: number;
  neither: number;
  /** (task_key + no_card) / ended; null when no thread ended in the window. */
  share: number | null;
}

const endings = (): Endings => ({ ended: 0, task_key: 0, no_card: 0, neither: 0, share: null });
const settle = (e: Endings) => ({ ...e, share: e.ended > 0 ? (e.task_key + e.no_card) / e.ended : null });
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

async function mapLimit<T, R>(items: readonly T[], limit: number, f: (x: T) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length);
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      out[i] = await f(items[i]!);
    }
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
  return out;
}

export async function traceability(deps: TraceabilityDeps, opts: { since: string; now: string }) {
  const nowMs = Date.parse(opts.now);
  const sinceMs = Date.parse(opts.since);
  let projects: TaskProject[];
  try {
    projects = await deps.listProjects();
  } catch (e) {
    const error = `tasks projects: ${message(e)}`;
    return { threads: { error }, stale_open_tasks: { error } };
  }
  const prefixes = new Set(projects.map((p) => p.prefix));
  const [threads, stale] = await Promise.all([threadEndings(deps, prefixes, sinceMs), staleTasks(deps, projects, nowMs)]);
  return { threads, stale_open_tasks: stale };
}

/** Threads updated inside the window that have ended (idle or error; starting, active, pending and stopping count as
 *  running); root threads and child threads counted apart. */
async function threadEndings(deps: TraceabilityDeps, prefixes: ReadonlySet<string>, sinceMs: number) {
  try {
    const rows = (await deps.listThreads()).filter((t) => t.updatedAt >= sinceMs);
    const done = rows.filter((t) => ENDED.has(t.status));
    const kinds = await mapLimit(done, OUTPUT_CONCURRENCY, async (t) => classifyOutput((await deps.output(t.id)) ?? "", prefixes));
    const all = endings();
    const root = endings();
    const child = endings();
    done.forEach((t, i) => {
      for (const e of [all, t.parentThreadId ? child : root]) {
        e.ended++;
        e[kinds[i]!]++;
      }
    });
    return { ...settle(all), running: rows.length - done.length, root: settle(root), child: settle(child) };
  } catch (e) {
    return { error: `threads: ${message(e)}` };
  }
}

/** Open tasks across every tracker project, and those with no update in STALE_DAYS or more. */
async function staleTasks(deps: TraceabilityDeps, projects: TaskProject[], nowMs: number) {
  try {
    const cutoff = nowMs - STALE_DAYS * DAY_MS;
    let open = 0;
    let stale = 0;
    const per: { project: string; open: number; stale: number }[] = [];
    for (const p of projects) {
      const tasks = (await deps.listOpenTasks(p.id)).filter((t) => (OPEN_STATUSES as readonly string[]).includes(t.status));
      const s = tasks.filter((t) => Date.parse(t.updatedAt) <= cutoff).length;
      open += tasks.length;
      stale += s;
      if (s > 0) per.push({ project: p.prefix, open: tasks.length, stale: s });
    }
    per.sort((a, b) => b.stale - a.stale || a.project.localeCompare(b.project));
    return { days: STALE_DAYS, open, stale, share: open > 0 ? stale / open : null, top_projects: per.slice(0, TOP_PROJECTS) };
  } catch (e) {
    return { error: `tasks: ${message(e)}` };
  }
}
