// Client for the bb tasks plugin (0.1.2), over plugins.callRpc. Reads, plus the two Home writes
// (a non-notifying comment and a label-set replacement). Never spawns the bb CLI.
// Every response is validated here as well as by the host; a rejected call, a schema failure or a
// repeating cursor is a TasksError. Callers treat a TasksError as "unavailable", never as "absent".

import { z } from "zod";

export const TASKS_PLUGIN_ID = "tasks";
export const NEEDS_MK_LABEL = "needs-mk";
export const LABEL_TTL_MS = 60_000;
export const HOME_AUTHOR = "Home";
/** The trailer Home's own note path appends to a comment it posts. Used to label (and to never read as a report); it grants nothing. */
export const HOME_NOTE_TRAILER = /\n\nhome-note: [A-Za-z0-9_-]{1,64}\s*$/;
export const PAGE_LIMIT = 500;
export const OPEN_STATUSES = ["backlog", "todo", "in_progress", "in_review"] as const;

const id = z.string().min(1);
const statusSchema = z.enum(["backlog", "todo", "in_progress", "in_review", "done", "canceled"]);

const projectSchema = z.object({ id, name: z.string(), prefix: z.string(), linkedBbProjectId: z.string().nullable() });
const labelSchema = z.object({ id, projectId: id, name: z.string(), color: z.string() });
const taskSchema = z.object({
  id,
  projectId: id,
  number: z.number().int(),
  key: z.string(),
  title: z.string(),
  description: z.string(),
  status: statusSchema,
  priority: z.string(),
  dueDate: z.string().nullable(),
  parentTaskId: id.nullable(),
  position: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
  labelIds: z.array(id),
});
const commentSchema = z.object({
  id,
  taskId: id,
  kind: z.enum(["user", "agent", "system"]),
  authorName: z.string(),
  threadId: z.string().nullable(),
  body: z.string(),
  createdAt: z.string(),
});

export type TaskProject = z.infer<typeof projectSchema>;
export type TaskLabel = z.infer<typeof labelSchema>;
export type Task = z.infer<typeof taskSchema>;
export type TaskCommentRow = z.infer<typeof commentSchema>;

export class TasksError extends Error {
  constructor(message: string, readonly cause?: unknown) {
    super(message);
    this.name = "TasksError";
  }
}

export interface PluginsLike {
  callRpc<T>(args: { pluginId: string; method: string; input?: unknown; signal?: AbortSignal; outputSchema: z.ZodType<T> }): Promise<T>;
}

export type TaskState = { state: "deleted" } | { state: "unlabelled"; task: Task } | { state: "labelled"; task: Task };

export interface ListTasksQuery {
  projectId?: string;
  statuses?: readonly string[];
  labelIds?: readonly string[];
  signal?: AbortSignal;
}

export class TasksClient {
  private readonly now: () => number;
  private readonly labelCache = new Map<string, { at: number; labels: TaskLabel[] }>();

  constructor(private readonly plugins: PluginsLike, opts: { now?: () => number } = {}) {
    this.now = opts.now ?? Date.now;
  }

  private async call<T>(method: string, input: unknown, schema: z.ZodType<T>, signal?: AbortSignal): Promise<T> {
    try {
      const raw = await this.plugins.callRpc<unknown>({ pluginId: TASKS_PLUGIN_ID, method, input, signal, outputSchema: z.unknown() });
      return schema.parse(raw);
    } catch (e) {
      throw new TasksError(`tasks.${method} failed: ${e instanceof Error ? e.message : String(e)}`, e);
    }
  }

  async listProjects(signal?: AbortSignal): Promise<TaskProject[]> {
    return (await this.call("listProjects", {}, z.object({ projects: z.array(projectSchema) }), signal)).projects;
  }

  /** Labels of one project, cached 60 s. `fresh` bypasses the cache; a failed read leaves the cache untouched. */
  async listLabels(projectId: string, opts: { fresh?: boolean; signal?: AbortSignal } = {}): Promise<TaskLabel[]> {
    const hit = this.labelCache.get(projectId);
    if (!opts.fresh && hit && this.now() - hit.at < LABEL_TTL_MS) return hit.labels;
    const { labels } = await this.call("listLabels", { projectId }, z.object({ labels: z.array(labelSchema) }), opts.signal);
    this.labelCache.set(projectId, { at: this.now(), labels });
    return labels;
  }

  /** Every label id in the project named needs-mk (a project may carry several). */
  async needsMkLabelIds(projectId: string, opts: { fresh?: boolean; signal?: AbortSignal } = {}): Promise<string[]> {
    const ids = (await this.listLabels(projectId, opts)).filter((l) => l.name === NEEDS_MK_LABEL).map((l) => l.id);
    if (ids.length > 0 || opts.fresh) return ids;
    // The label can be created after the first poll saw the project; an empty answer is never trusted from cache.
    return (await this.listLabels(projectId, { ...opts, fresh: true })).filter((l) => l.name === NEEDS_MK_LABEL).map((l) => l.id);
  }

  /** All pages. Any failure rejects; a partial read is never returned. */
  async listTasks(q: ListTasksQuery = {}): Promise<Task[]> {
    const out: Task[] = [];
    const seen = new Set<string>();
    let cursor: string | undefined;
    for (;;) {
      const input: Record<string, unknown> = { limit: PAGE_LIMIT };
      if (q.projectId !== undefined) input.projectId = q.projectId;
      if (q.statuses !== undefined) input.statuses = [...q.statuses];
      if (q.labelIds !== undefined) input.labelIds = [...q.labelIds];
      if (cursor !== undefined) input.cursor = cursor;
      const page = await this.call("listTasks", input, z.object({ tasks: z.array(taskSchema), nextCursor: z.string().nullable() }), q.signal);
      out.push(...page.tasks);
      if (page.nextCursor === null) return out;
      if (seen.has(page.nextCursor)) throw new TasksError(`tasks.listTasks repeated cursor ${page.nextCursor}`);
      seen.add(page.nextCursor);
      cursor = page.nextCursor;
    }
  }

  /** Comments ordered by (createdAt, id), whatever order the server used. */
  async listComments(taskId: string, signal?: AbortSignal): Promise<TaskCommentRow[]> {
    const { comments } = await this.call("listComments", { taskId }, z.object({ comments: z.array(commentSchema) }), signal);
    return [...comments].sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  }

  /** Post a comment that notifies no thread. Not idempotent: the caller checks listComments first. */
  async createComment(taskId: string, body: string, signal?: AbortSignal): Promise<void> {
    await this.call("createComment", { taskId, body, notify: false, allowEmptyBody: false }, z.object({ comment: z.object({ id }).passthrough() }).passthrough(), signal);
  }

  /** Replace a task's whole label set (updateTask semantics), attributed to Home. */
  async setLabels(taskId: string, labelIds: readonly string[], signal?: AbortSignal): Promise<void> {
    await this.call("updateTask", { taskId, labelIds: [...labelIds], authorName: HOME_AUTHOR }, z.object({}).passthrough(), signal);
  }

  /** One task, or null when it does not exist. */
  async getTask(taskId: string, signal?: AbortSignal): Promise<Task | null> {
    return (await this.call("getTask", { taskId }, z.object({ task: taskSchema.nullable() }), signal)).task;
  }

  /** File a task in a project (the Idea box). Not idempotent: the caller looks for its own marker first. */
  async createTask(a: { projectId: string; title: string; description: string; labelIds: readonly string[] }, signal?: AbortSignal): Promise<Task> {
    // The real RPC answers {ok:true,task} or {ok:false,error}; a refusal is an error here, never a task.
    const r = await this.call("createTask", { ...a, labelIds: [...a.labelIds] }, z.union([z.object({ ok: z.literal(true), task: taskSchema }).passthrough(), z.object({ ok: z.literal(false) }).passthrough()]), signal);
    if (!r.ok) throw new TasksError(`tasks.createTask refused: ${JSON.stringify((r as { error?: unknown }).error ?? null)}`);
    return r.task;
  }

  async createLabel(projectId: string, name: string, color: string, signal?: AbortSignal): Promise<void> {
    await this.call("createLabel", { projectId, name, color }, z.object({}).passthrough(), signal);
    this.labelCache.delete(projectId);
  }

  /** Set a task's status (updateTask semantics), attributed to Home. */
  async setStatus(taskId: string, status: "canceled" | "done", signal?: AbortSignal): Promise<void> {
    const r = await this.call("updateTask", { taskId, status, authorName: HOME_AUTHOR }, z.object({ ok: z.boolean().optional() }).passthrough(), signal);
    if (r.ok === false) throw new TasksError("tasks.updateTask refused");
  }

  /** Mark a task done (updateTask semantics), attributed to Home. Used only for cards Home filed. */
  async closeTask(taskId: string, signal?: AbortSignal): Promise<void> {
    await this.call("updateTask", { taskId, status: "done", authorName: HOME_AUTHOR }, z.object({}).passthrough(), signal);
  }

  /**
   * deleted (getTask null), unlabelled, or labelled. "Unlabelled" is decided against a fresh label
   * read, so a renamed or recreated needs-mk label is not mistaken for a removed one. Any failed
   * read throws rather than answering deleted or unlabelled.
   */
  async taskState(taskId: string, signal?: AbortSignal): Promise<TaskState> {
    const { task } = await this.call("getTask", { taskId }, z.object({ task: taskSchema.nullable() }), signal);
    if (task === null) return { state: "deleted" };
    const ids = await this.needsMkLabelIds(task.projectId, { fresh: true, signal });
    return task.labelIds.some((l) => ids.includes(l)) ? { state: "labelled", task } : { state: "unlabelled", task };
  }
}
