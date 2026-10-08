import { describe, expect, it } from "vitest";
import type { Task, TaskProject } from "../tasks.js";
import { classifyOutput, listAllThreads, traceability, type ThreadLike, type ThreadPageArgs, type TraceabilityDeps } from "../traceability.js";

const NOW = "2026-10-08T00:00:00.000Z";
const SINCE = "2026-09-24T00:00:00.000Z";
const DAY = 86_400_000;
const ago = (days: number) => new Date(Date.parse(NOW) - days * DAY).toISOString();

const project = (prefix: string): TaskProject => ({ id: `p_${prefix}`, name: prefix, prefix, linkedBbProjectId: null });
const task = (projectId: string, n: number, status: Task["status"], updatedAt: string): Task => ({
  id: `${projectId}_${n}`,
  projectId,
  number: n,
  key: `K-${n}`,
  title: "t",
  description: "",
  status,
  priority: "none",
  dueDate: null,
  parentTaskId: null,
  position: n,
  createdAt: updatedAt,
  updatedAt,
  labelIds: [],
});
const thread = (id: string, status: string, updatedAt: string, parentThreadId: string | null = null): ThreadLike => ({ id, status, updatedAt: Date.parse(updatedAt), parentThreadId });

describe("classifyOutput", () => {
  const prefixes = new Set(["AUTA", "OPS", "SHDW2"]);
  it("counts a key only when its prefix is a tracker project", () => {
    expect(classifyOutput("Done; recorded on AUTA-24.", prefixes)).toBe("task_key");
    expect(classifyOutput("closed SHDW2-7", prefixes)).toBe("task_key");
    expect(classifyOutput("Ran GPT-6 against SHA-256 fixtures in W-41", prefixes)).toBe("neither");
    expect(classifyOutput("see auta-24", prefixes)).toBe("neither");
  });
  it("counts no-card only with a reason", () => {
    expect(classifyOutput("Answered the question.\nno-card: one-off Q&A", prefixes)).toBe("no_card");
    expect(classifyOutput("No-Card: trivial", prefixes)).toBe("no_card");
    expect(classifyOutput("no-card:", prefixes)).toBe("neither");
    expect(classifyOutput("", prefixes)).toBe("neither");
  });
  it("counts no-card only as a marker starting a line", () => {
    expect(classifyOutput("  no-card: indented", prefixes)).toBe("no_card");
    expect(classifyOutput("not-no-card: reason", prefixes)).toBe("neither");
    expect(classifyOutput("Threads should give no-card: <reason> when trivial.", prefixes)).toBe("neither");
  });
  it("a key wins over no-card", () => {
    expect(classifyOutput("no-card: x, but OPS-3 updated", prefixes)).toBe("task_key");
  });
});

describe("traceability", () => {
  const deps = (over: Partial<TraceabilityDeps> = {}): TraceabilityDeps => {
    const outputs: Record<string, string | null> = { a: "Finished AUTA-1", b: "no-card: chat", c: "all done", d: null, e: "OPS-2 closed", old: "AUTA-9", run: "AUTA-3" };
    return {
      listProjects: async () => [project("AUTA"), project("OPS")],
      listOpenTasks: async (id) =>
        id === "p_AUTA"
          ? [task(id, 1, "todo", ago(30)), task(id, 2, "in_progress", ago(20)), task(id, 3, "backlog", ago(1)), task(id, 4, "done", ago(40))]
          : [task(id, 1, "todo", ago(15)), task(id, 2, "in_review", ago(13.9)), task(id, 3, "todo", ago(14))],
      listThreads: async () => [
        thread("a", "idle", ago(1)),
        thread("b", "idle", ago(2)),
        thread("c", "error", ago(3), "a"),
        thread("d", "idle", ago(4), "a"),
        thread("e", "idle", ago(5), "b"),
        thread("old", "idle", ago(20)),
        thread("run", "active", ago(0)),
        thread("boot", "starting", ago(0)),
        thread("halt", "stopping", ago(0)),
      ],
      output: async (id) => outputs[id] ?? null,
      ...over,
    };
  };

  it("measures thread endings in the window and stale open tasks", async () => {
    const r = await traceability(deps(), { since: SINCE, now: NOW });
    expect(r.threads).toEqual({
      ended: 5,
      task_key: 2,
      no_card: 1,
      neither: 2,
      share: 3 / 5,
      running: 3,
      root: { ended: 2, task_key: 1, no_card: 1, neither: 0, share: 1 },
      child: { ended: 3, task_key: 1, no_card: 0, neither: 2, share: 1 / 3 },
    });
    // done tasks are dropped even if the source returns them; 13.9 days is not stale, exactly 14 and 15 are.
    expect(r.stale_open_tasks).toEqual({
      days: 14,
      open: 6,
      stale: 4,
      share: 4 / 6,
      top_projects: [
        { project: "AUTA", open: 3, stale: 2 },
        { project: "OPS", open: 3, stale: 2 },
      ],
    });
  });

  it("an empty window reports a null share, not zero", async () => {
    const r = await traceability(deps({ listThreads: async () => [], listOpenTasks: async () => [] }), { since: SINCE, now: NOW });
    expect(r.threads).toMatchObject({ ended: 0, share: null });
    expect(r.stale_open_tasks).toMatchObject({ open: 0, stale: 0, share: null, top_projects: [] });
  });

  it("a failed read reports that part as an error, never a partial count", async () => {
    let calls = 0;
    const r = await traceability(
      deps({
        output: async (id) => {
          if (id === "e") throw new Error("boom");
          return "AUTA-1";
        },
        listOpenTasks: async (id) => {
          if (calls++ > 0) throw new Error("page 2 failed");
          return [task(id, 1, "todo", ago(30))];
        },
      }),
      { since: SINCE, now: NOW },
    );
    expect(r.threads).toEqual({ error: "threads: boom" });
    expect(r.stale_open_tasks).toEqual({ error: "tasks: page 2 failed" });
  });

  it("without tracker projects both parts report the error", async () => {
    const r = await traceability(deps({ listProjects: async () => Promise.reject(new Error("tasks unavailable")) }), { since: SINCE, now: NOW });
    expect(r).toEqual({ threads: { error: "tasks projects: tasks unavailable" }, stale_open_tasks: { error: "tasks projects: tasks unavailable" } });
  });
});

describe("listAllThreads", () => {
  const row = (id: string, extra: Record<string, unknown> = {}) => ({ id, status: "idle", updatedAt: 1, parentThreadId: null, ...extra });
  const pager = (pages: { live: ReturnType<typeof row>[][]; archived: ReturnType<typeof row>[][] }, fail?: (a: ThreadPageArgs, n: number) => boolean) => {
    const calls: ThreadPageArgs[] = [];
    const list = async (a: ThreadPageArgs) => {
      calls.push(a);
      if (fail?.(a, calls.length)) throw new Error("page failed");
      const src = a.archived ? pages.archived : pages.live;
      // pages are addressed by offset; this host caps pages at 2 rows whatever limit is asked
      const flat = src.flat();
      return flat.slice(a.offset, a.offset + 2);
    };
    return { list, calls };
  };

  it("reads live then archived pages by offset until an empty page, drops deleted rows and duplicates", async () => {
    const { list, calls } = pager({ live: [[row("a"), row("b"), row("c", { parentThreadId: "a" })]], archived: [[row("d"), row("x", { deletedAt: 5 }), row("a")]] });
    const got = await listAllThreads(list);
    expect(got.map((t) => t.id).sort()).toEqual(["a", "b", "c", "d"]);
    expect(got.find((t) => t.id === "c")!.parentThreadId).toBe("a");
    expect(calls.map((c) => [c.archived, c.offset])).toEqual([
      [false, 0],
      [false, 2],
      [false, 3],
      [true, 0],
      [true, 2],
      [true, 3],
    ]);
    expect(calls.every((c) => c.includeHidden && c.limit === 100)).toBe(true);
  });

  it("a failed later page rejects, so traceability reports an error and no counts", async () => {
    const { list } = pager({ live: [[row("a"), row("b"), row("c")]], archived: [[row("d")]] }, (a) => a.archived && a.offset > 0);
    await expect(listAllThreads(list)).rejects.toThrow("page failed");
    const r = await traceability(
      { listProjects: async () => [project("AUTA")], listOpenTasks: async () => [], listThreads: () => listAllThreads(list), output: async () => "AUTA-1" },
      { since: SINCE, now: NOW },
    );
    expect(r.threads).toEqual({ error: "threads: page failed" });
  });
});
