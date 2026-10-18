// An in-memory stand-in for the tasks plugin's RPC surface (tasks@0.1.2), shaped like
// bb.sdk.plugins.callRpc: it validates the returned value with the caller's outputSchema
// the way the host does, and records every call so tests can assert the route taken.

import { z } from "zod";
import type { PluginsLike } from "../tasks.js";

export interface FakeProject {
  id: string;
  name: string;
  prefix: string;
  nextTaskNumber: number;
  color: string;
  folderId: string | null;
  linkedBbProjectId: string | null;
  createdAt: string;
}
export interface FakeLabel {
  id: string;
  projectId: string;
  name: string;
  color: string;
}
export interface FakeTask {
  id: string;
  projectId: string;
  number: number;
  key: string;
  title: string;
  description: string;
  status: string;
  priority: string;
  dueDate: string | null;
  parentTaskId: string | null;
  position: number;
  createdAt: string;
  updatedAt: string;
  labelIds: string[];
}
export interface FakeComment {
  id: string;
  taskId: string;
  kind: string;
  authorName: string;
  presetName: string | null;
  threadId: string | null;
  body: string;
  notifiedCount: number;
  createdAt: string;
  threadTitle: string | null;
  provider: null;
}

export interface RpcCall {
  pluginId: string;
  method: string;
  input: any;
}

let seq = 0;
/** A 26-character Crockford-base32 ULID-shaped id, unique per call. */
export function ulid(): string {
  const n = String(++seq);
  return "01" + "0".repeat(24 - n.length) + n;
}

export class FakeTasks implements PluginsLike {
  projects: FakeProject[] = [];
  labels: FakeLabel[] = [];
  tasks: FakeTask[] = [];
  comments: FakeComment[] = [];
  calls: RpcCall[] = [];
  /** Fail the nth (1-based) call of a method, counted across the fake's lifetime. */
  failures: { method: string; nth: number; error: Error }[] = [];
  /** Replace a method's raw output, to exercise response validation. */
  rawOutput = new Map<string, (real: unknown) => unknown>();
  /** Run the nth call of a method for real, then reject it: the effect landed but the response was lost. */
  refuseUpdates = false;
  lostResponses: { method: string; nth: number }[] = [];
  private counts = new Map<string, number>();

  addProject(name: string): FakeProject {
    const p: FakeProject = { id: ulid(), name, prefix: name.slice(0, 4).toUpperCase(), nextTaskNumber: 1, color: "#fff", folderId: null, linkedBbProjectId: null, createdAt: "2026-10-01T00:00:00.000Z" };
    this.projects.push(p);
    return p;
  }
  addLabel(projectId: string, name: string): FakeLabel {
    const l: FakeLabel = { id: ulid(), projectId, name, color: "#f00" };
    this.labels.push(l);
    return l;
  }
  addTask(projectId: string, over: Partial<FakeTask> = {}): FakeTask {
    const p = this.projects.find((x) => x.id === projectId)!;
    const number = p.nextTaskNumber++;
    const t: FakeTask = {
      id: ulid(), projectId, number, key: `${p.prefix}-${number}`, title: `task ${number}`, description: "", status: "todo", priority: "none",
      dueDate: null, parentTaskId: null, position: number, createdAt: "2026-10-01T00:00:00.000Z", updatedAt: "2026-10-01T00:00:00.000Z", labelIds: [], ...over,
    };
    this.tasks.push(t);
    return t;
  }
  addComment(taskId: string, over: Partial<FakeComment> = {}): FakeComment {
    const c: FakeComment = {
      id: ulid(), taskId, kind: "agent", authorName: "You", presetName: null, threadId: "thr_a", body: "x", notifiedCount: 0,
      createdAt: "2026-10-01T00:00:00.000Z", threadTitle: null, provider: null, ...over,
    };
    this.comments.push(c);
    return c;
  }
  callsOf(method: string): RpcCall[] {
    return this.calls.filter((c) => c.method === method);
  }

  async callRpc<T>(args: { pluginId: string; method: string; input?: unknown; signal?: AbortSignal; outputSchema: z.ZodType<T> }): Promise<T> {
    const input = (args.input ?? {}) as any;
    this.calls.push({ pluginId: args.pluginId, method: args.method, input });
    const n = (this.counts.get(args.method) ?? 0) + 1;
    this.counts.set(args.method, n);
    const f = this.failures.find((x) => x.method === args.method && x.nth === n);
    if (f) throw f.error;
    if (args.pluginId !== "tasks") throw new Error(`unknown plugin ${args.pluginId}`);
    let out = this.handle(args.method, input);
    if (this.lostResponses.some((x) => x.method === args.method && x.nth === n)) throw new Error(`${args.method}: response lost`);
    const raw = this.rawOutput.get(args.method);
    if (raw) out = raw(out);
    // The host validates; so does the fake. A schema failure is a rejected promise.
    return args.outputSchema.parse(out);
  }

  private handle(method: string, input: any): unknown {
    switch (method) {
      case "listProjects":
        return { projects: this.projects };
      case "listLabels":
        return { labels: this.labels.filter((l) => l.projectId === input.projectId) };
      case "getTask":
        return { task: this.tasks.find((t) => t.id === input.taskId) ?? null };
      case "listTasks": {
        let rows = this.tasks.filter(
          (t) =>
            (input.projectId === undefined || t.projectId === input.projectId) &&
            (input.statuses === undefined || input.statuses.includes(t.status)) &&
            (input.labelIds === undefined || t.labelIds.some((l: string) => input.labelIds.includes(l))) &&
            (input.search === undefined || `${t.title}\n${t.description}`.toLowerCase().includes(String(input.search).toLowerCase())),
        );
        const start = input.cursor === undefined ? 0 : Number(input.cursor);
        const limit = input.limit ?? 100;
        const page = rows.slice(start, start + limit);
        const next = start + limit < rows.length ? String(start + limit) : null;
        return { tasks: page, nextCursor: next };
      }
      case "createTask": {
        const p = this.projects.find((x) => x.id === input.projectId);
        if (!p) throw new Error("project not found");
        const t = this.addTask(p.id, { title: input.title, description: input.description ?? "", labelIds: [...(input.labelIds ?? [])], createdAt: `2026-10-03T00:00:${String(this.tasks.length % 60).padStart(2, "0")}.000Z` });
        return { ok: true, task: t };
      }
      case "createLabel": {
        if (this.labels.some((l) => l.projectId === input.projectId && l.name.toLowerCase() === String(input.name).toLowerCase())) throw new Error(`label name already in use: ${input.name}`);
        return { label: this.addLabel(input.projectId, input.name) };
      }
      case "listComments": {
        // Deliberately not in createdAt order: the client must sort.
        const rows = this.comments.filter((c) => c.taskId === input.taskId);
        return { comments: [...rows].reverse() };
      }
      case "createComment": {
        // The real CLI posts an agent comment from the calling thread; the RPC's own callers (the card writer) post as mk.
        const c = this.addComment(input.taskId, { kind: input.threadId ? "agent" : "user", authorName: "You", threadId: input.threadId ?? null, body: input.body, createdAt: `2026-10-03T00:00:${String(this.comments.length % 60).padStart(2, "0")}.000Z` });
        return { comment: c };
      }
      case "updateTask": {
        const t = this.tasks.find((x) => x.id === input.taskId);
        if (!t) throw new Error("task not found");
        if (this.refuseUpdates) return { ok: false, error: "refused" };
        if (input.labelIds !== undefined) t.labelIds = [...input.labelIds];
        if (input.status !== undefined) t.status = input.status;
        t.updatedAt = `2026-10-03T00:01:${String(this.calls.length % 60).padStart(2, "0")}.000Z`;
        return { task: t };
      }
      default:
        throw new Error(`fake does not implement ${method}`);
    }
  }
}
