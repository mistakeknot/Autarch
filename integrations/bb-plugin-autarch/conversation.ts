// Conversation on the card. Everything here is DISPLAY: comments are mirrored from the tasks card, shown as plain
// text, and never read as a command. No code path in this file (or anywhere) parses a comment for Request:, home-ask or
// home-move blocks, rulings or reports; a comment cannot close, rule or alter a card. The author class comes from the
// id the tasks plugin recorded, never from the text.
import type { Service } from "./service.js";
import type { CommentRow, Store } from "./store.js";
import { HOME_NOTE_TRAILER, type TaskCommentRow } from "./tasks.js";
import { parseAsk } from "./model.js";

export const SELECTED_POLL_MS = 15_000;
export const OPEN_POLL_MS = 60_000;
export const OPEN_POLL_MAX = 25;
const CONCURRENCY = 2;
/** The selected card is polled while the UI has asked for it within this window. */
const SELECTION_TTL_MS = 2 * 60_000;

export type AuthorClass = "mk" | "owner" | "vizier" | "other";

const HOME_TRAILER = HOME_NOTE_TRAILER; // the only text-derived fact: it labels, it never grants anything

export interface ClassifyContext {
  owner: string | null;
  vizier: string | null;
}

/** mk, the card's owner thread, the vizier thread, or other: from the recorded author id and kind only. */
export function authorClass(c: Pick<CommentRow, "kind" | "author_id">, ctx: ClassifyContext): AuthorClass {
  if (c.author_id !== null && c.author_id !== "") {
    if (ctx.owner !== null && c.author_id === ctx.owner) return "owner";
    if (ctx.vizier !== null && c.author_id === ctx.vizier) return "vizier";
    return "other";
  }
  return c.kind === "user" ? "mk" : "other";
}

export interface CommentView {
  id: string;
  author_class: AuthorClass;
  author_name: string;
  /** Plain text. The Home trailer is removed; everything else is as written. */
  body: string;
  created_at: string;
  /** Posted from Home's Reply box: shown as "posted from Home (unattested)". Only ever true for the mk class. */
  home_posted: boolean;
  unread: boolean;
}

export interface ConversationView {
  task_id: string;
  comments: CommentView[];
  unread: number;
  /** True when the last read failed or none has succeeded: the UI says "comments may be stale". */
  stale: boolean;
  last_ok_at: string | null;
  /** Each option's full instruction text for the collapsible, shown only behind the conversation. */
  options: { id: string; label: string; kind: string; instruction: string | null }[];
}

const key = (c: { created_at: string; comment_id: string }) => `${c.created_at}#${c.comment_id}`;

export function unreadCounts(store: Store, taskIds: readonly string[]): Record<string, number> {
  const out: Record<string, number> = {};
  for (const id of taskIds) {
    const seen = store.seenThrough(id);
    const n = store.comments(id).filter((c) => (seen === null || key(c) > seen) && !(c.author_id === null && c.kind === "user")).length;
    if (n > 0) out[id] = n;
  }
  return out;
}

export function conversationView(svc: Service, taskId: string, vizier: string | null): ConversationView {
  const store = svc.store;
  const dec = store.db.prepare("SELECT body_json, owner_thread, thread FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(taskId) as
    | { body_json: string; owner_thread: string | null; thread: string | null }
    | undefined;
  const ctx: ClassifyContext = { owner: dec?.owner_thread ?? dec?.thread ?? null, vizier };
  const seen = store.seenThrough(taskId);
  const comments = store.comments(taskId).map((c): CommentView => {
    const cls = authorClass(c, ctx);
    const home = cls === "mk" && HOME_TRAILER.test(c.body);
    return {
      id: c.comment_id,
      author_class: cls,
      author_name: c.author_name,
      body: home ? c.body.replace(HOME_TRAILER, "") : c.body,
      created_at: c.created_at,
      home_posted: home,
      // mk's own comments are never unread to mk.
      unread: cls !== "mk" && (seen === null || key(c) > seen),
    };
  });
  let options: ConversationView["options"] = [];
  if (dec) {
    try {
      options = (parseAsk(JSON.parse(dec.body_json)).options ?? []).map((o) => ({ id: o.id ?? "", label: o.label, kind: o.kind ?? "ruling-only", instruction: typeof o.instruction === "string" && o.instruction.trim() !== "" ? o.instruction : null }));
    } catch {
      options = [];
    }
  }
  const poll = store.commentPoll(taskId);
  return {
    task_id: taskId,
    comments,
    unread: comments.filter((c) => c.unread).length,
    stale: !poll || poll.last_error !== null,
    last_ok_at: poll?.last_ok_at ?? null,
    options,
  };
}

export interface CommentPollerOptions {
  store: Store;
  listComments: (taskId: string) => Promise<TaskCommentRow[]>;
  /** Cards with an open move or an open ask. */
  openTaskIds: () => string[];
  clock?: () => number;
  log?: (msg: string) => void;
  onChanged?: () => void;
}

/**
 * Reads comments from the tasks adapter. The selected card every 15 s; cards with an open move or ask every 60 s, at most 25
 * per cycle (oldest-read first, so the rest are reached next cycle), two at a time. A failure is recorded as "may be stale"
 * and changes nothing else: no card, move or decision is touched here.
 */
export class CommentPoller {
  private selected: { id: string; at: number } | null = null;
  private lastOpen = Number.NEGATIVE_INFINITY;
  private readonly clock: () => number;
  readonly stats = { reads: 0, errors: 0 };

  constructor(private readonly o: CommentPollerOptions) {
    this.clock = o.clock ?? Date.now;
  }

  /** The UI has the conversation of this card open. */
  select(taskId: string): void {
    this.selected = { id: taskId, at: this.clock() };
  }

  /** Reads one card now (after mk posts a reply, so it shows without waiting for the next tick). Never throws. */
  async readNow(taskId: string): Promise<void> {
    await this.readOne(taskId);
  }

  private async readOne(taskId: string): Promise<void> {
    this.stats.reads++;
    try {
      const rows = await this.o.listComments(taskId);
      const n = this.o.store.upsertComments(
        taskId,
        rows.map((r) => ({ id: r.id, kind: r.kind, authorName: r.authorName, authorId: r.threadId, body: r.body, createdAt: r.createdAt })),
      );
      if (n > 0) this.o.onChanged?.();
    } catch (e) {
      this.stats.errors++;
      const msg = e instanceof Error ? e.message : String(e);
      this.o.store.markCommentsFailed(taskId, msg);
      this.o.log?.(`comments ${taskId}: ${msg}`);
    }
  }

  private async all(ids: string[]): Promise<void> {
    let i = 0;
    const worker = async () => {
      while (i < ids.length) await this.readOne(ids[i++]!);
    };
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, ids.length) }, worker));
  }

  /** One 15 s tick: the selected card, plus the open cards when 60 s have passed. Never throws. */
  async tick(): Promise<void> {
    try {
      const now = this.clock();
      const ids: string[] = [];
      if (this.selected && now - this.selected.at <= SELECTION_TTL_MS) ids.push(this.selected.id);
      if (now - this.lastOpen >= OPEN_POLL_MS) {
        this.lastOpen = now;
        const open = [...new Set(this.o.openTaskIds())].filter((t) => !ids.includes(t));
        const attempt = (t: string) => Date.parse(this.o.store.commentPoll(t)?.last_attempt_at ?? "") || 0;
        open.sort((a, b) => attempt(a) - attempt(b) || (a < b ? -1 : 1));
        ids.push(...open.slice(0, OPEN_POLL_MAX));
      }
      await this.all(ids);
    } catch (e) {
      this.o.log?.(`comment poll: ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  async run(signal: AbortSignal, sleep: (ms: number, signal: AbortSignal) => Promise<void>): Promise<void> {
    while (!signal.aborted) {
      await this.tick();
      await sleep(SELECTED_POLL_MS, signal);
    }
  }
}

/** Task ids with an open ask or a live move. */
export function openTaskIds(svc: Service): string[] {
  const asks = svc.store.db.prepare("SELECT task_id FROM cards WHERE state = 'open'").all() as { task_id: string }[];
  const moves = svc.store.moves().filter((m) => m.state !== "closed").map((m) => ({ task_id: m.task_id }));
  return [...asks, ...moves].map((r) => r.task_id);
}
