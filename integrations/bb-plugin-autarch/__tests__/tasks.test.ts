import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { OPEN_STATUSES, TasksClient, TasksError } from "../tasks";
import { FakeTasks } from "./tasks-fake";

function setup() {
  const fake = new FakeTasks();
  let now = 1_000_000;
  const client = new TasksClient(fake, { now: () => now });
  return { fake, client, advance: (ms: number) => (now += ms) };
}

describe("label cache", () => {
  it("serves a project's labels from cache for 60 s, then re-reads", async () => {
    const { fake, client, advance } = setup();
    const p = fake.addProject("Autarch");
    fake.addLabel(p.id, "needs-mk");
    await client.listLabels(p.id);
    advance(59_000);
    await client.listLabels(p.id);
    expect(fake.callsOf("listLabels")).toHaveLength(1);
    advance(2_000);
    await client.listLabels(p.id);
    expect(fake.callsOf("listLabels")).toHaveLength(2);
  });

  it("caches per project", async () => {
    const { fake, client } = setup();
    const a = fake.addProject("A");
    const b = fake.addProject("B");
    fake.addLabel(a.id, "needs-mk");
    await client.listLabels(a.id);
    await client.listLabels(b.id);
    expect(fake.callsOf("listLabels")).toHaveLength(2);
  });

  it("maps needs-mk to every label id carrying that name", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const l1 = fake.addLabel(p.id, "needs-mk");
    const l2 = fake.addLabel(p.id, "needs-mk");
    fake.addLabel(p.id, "needs-mk-not");
    fake.addLabel(p.id, "Needs-MK");
    expect((await client.needsMkLabelIds(p.id)).sort()).toEqual([l1.id, l2.id].sort());
  });

  it("a cached project with no needs-mk label is re-read, so a label created later is seen at once (real bb: the first poll can precede the label)", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    expect(await client.needsMkLabelIds(p.id)).toEqual([]);
    const l = fake.addLabel(p.id, "needs-mk");
    expect(await client.needsMkLabelIds(p.id)).toEqual([l.id]);
  });

  it("fresh bypasses the cache and replaces it at once", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const old = fake.addLabel(p.id, "needs-mk");
    expect(await client.needsMkLabelIds(p.id)).toEqual([old.id]);
    fake.labels = [];
    const recreated = fake.addLabel(p.id, "needs-mk");
    expect(await client.needsMkLabelIds(p.id)).toEqual([old.id]); // still cached
    expect(await client.needsMkLabelIds(p.id, { fresh: true })).toEqual([recreated.id]);
    expect(await client.needsMkLabelIds(p.id)).toEqual([recreated.id]); // cache replaced
    expect(fake.callsOf("listLabels")).toHaveLength(2);
  });

  it("a failed label read surfaces and leaves the cache as it was", async () => {
    const { fake, client, advance } = setup();
    const p = fake.addProject("Autarch");
    const l = fake.addLabel(p.id, "needs-mk");
    await client.listLabels(p.id);
    fake.failures.push({ method: "listLabels", nth: 2, error: new Error("down") });
    await expect(client.listLabels(p.id, { fresh: true })).rejects.toBeInstanceOf(TasksError);
    expect((await client.needsMkLabelIds(p.id)).sort()).toEqual([l.id]);
    advance(61_000);
    fake.failures.push({ method: "listLabels", nth: 3, error: new Error("down") });
    await expect(client.listLabels(p.id)).rejects.toBeInstanceOf(TasksError);
  });
});

describe("listTasks pagination", () => {
  it("follows nextCursor to the end with limit 500", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    for (let i = 0; i < 1200; i++) fake.addTask(p.id);
    const tasks = await client.listTasks({ projectId: p.id, statuses: OPEN_STATUSES });
    expect(tasks).toHaveLength(1200);
    expect(new Set(tasks.map((t) => t.id)).size).toBe(1200);
    const calls = fake.callsOf("listTasks");
    expect(calls).toHaveLength(3);
    expect(calls.map((c) => c.input.cursor)).toEqual([undefined, "500", "1000"]);
    expect(calls.every((c) => c.input.limit === 500)).toBe(true);
    expect(calls[0]!.input.statuses).toEqual(["backlog", "todo", "in_progress", "in_review"]);
  });

  it("passes label ids and works without a project (cross-project)", async () => {
    const { fake, client } = setup();
    const a = fake.addProject("A");
    const b = fake.addProject("B");
    const la = fake.addLabel(a.id, "needs-mk");
    const lb = fake.addLabel(b.id, "needs-mk");
    fake.addTask(a.id, { labelIds: [la.id] });
    fake.addTask(b.id, { labelIds: [lb.id] });
    fake.addTask(b.id);
    const tasks = await client.listTasks({ labelIds: [la.id, lb.id] });
    expect(tasks).toHaveLength(2);
    expect(fake.callsOf("listTasks")[0]!.input.projectId).toBeUndefined();
  });

  it("a failure on page 2 rejects and returns nothing partial", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    for (let i = 0; i < 700; i++) fake.addTask(p.id);
    fake.failures.push({ method: "listTasks", nth: 2, error: new Error("boom") });
    await expect(client.listTasks({ projectId: p.id })).rejects.toThrow(TasksError);
  });

  it("a cursor that repeats is an error, not a loop", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    fake.addTask(p.id);
    fake.rawOutput.set("listTasks", (real: any) => ({ ...real, nextCursor: "same" }));
    await expect(client.listTasks({ projectId: p.id })).rejects.toThrow(/cursor/);
  });
});

describe("listComments", () => {
  it("returns comments ordered by (createdAt, id) whatever the server order", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const t = fake.addTask(p.id);
    fake.addComment(t.id, { id: "01C0000000000000000000000Z", createdAt: "2026-10-01T10:02:00.000Z" });
    fake.addComment(t.id, { id: "01A0000000000000000000000Z", createdAt: "2026-10-01T10:00:00.000Z" });
    fake.addComment(t.id, { id: "01B0000000000000000000000Z", createdAt: "2026-10-01T10:00:00.000Z" });
    const got = await client.listComments(t.id);
    expect(got.map((c) => c.id)).toEqual(["01A0000000000000000000000Z", "01B0000000000000000000000Z", "01C0000000000000000000000Z"]);
  });
});

describe("taskState: deleted is not unlabelled", () => {
  it("tells deleted, unlabelled and labelled apart", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const label = fake.addLabel(p.id, "needs-mk");
    const labelled = fake.addTask(p.id, { labelIds: [label.id] });
    const bare = fake.addTask(p.id);
    expect((await client.taskState(labelled.id)).state).toBe("labelled");
    const u = await client.taskState(bare.id);
    expect(u.state).toBe("unlabelled");
    expect(u.state === "unlabelled" && u.task.id).toBe(bare.id);
    fake.tasks = fake.tasks.filter((t) => t.id !== labelled.id);
    expect(await client.taskState(labelled.id)).toEqual({ state: "deleted" });
  });

  it("a renamed or recreated label is not a lost label: it uses fresh label ids", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const old = fake.addLabel(p.id, "needs-mk");
    const task = fake.addTask(p.id, { labelIds: [old.id] });
    await client.needsMkLabelIds(p.id); // warm the cache with the old id
    fake.labels = [];
    const fresh = fake.addLabel(p.id, "needs-mk");
    fake.tasks[0]!.labelIds = [fresh.id];
    expect((await client.taskState(task.id)).state).toBe("labelled");
    fake.labels = [];
    fake.addLabel(p.id, "other");
    expect((await client.taskState(task.id)).state).toBe("unlabelled");
  });

  it("a failed label read throws instead of reporting unlabelled", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const task = fake.addTask(p.id);
    fake.failures.push({ method: "listLabels", nth: 1, error: new Error("down") });
    await expect(client.taskState(task.id)).rejects.toBeInstanceOf(TasksError);
  });

  it("a failed getTask throws instead of reporting deleted", async () => {
    const { fake, client } = setup();
    fake.failures.push({ method: "getTask", nth: 1, error: new Error("down") });
    await expect(client.taskState("01000000000000000000000000")).rejects.toBeInstanceOf(TasksError);
  });
});

describe("response validation", () => {
  it("refuses a task with an unknown status", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    fake.addTask(p.id);
    fake.rawOutput.set("listTasks", (real: any) => ({ ...real, tasks: real.tasks.map((t: any) => ({ ...t, status: "bogus" })) }));
    await expect(client.listTasks({ projectId: p.id })).rejects.toThrow(TasksError);
  });

  it("refuses a task missing labelIds", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const t = fake.addTask(p.id);
    fake.rawOutput.set("getTask", (real: any) => ({ task: { ...real.task, labelIds: undefined } }));
    await expect(client.taskState(t.id)).rejects.toThrow(TasksError);
  });

  it("refuses a comment of an unknown kind", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const t = fake.addTask(p.id);
    fake.addComment(t.id, { kind: "alien" });
    await expect(client.listComments(t.id)).rejects.toThrow(TasksError);
  });

  it("validates even when the host skips its own check", async () => {
    const lax = { callRpc: async () => ({ projects: [{ id: 1 }] }) } as any;
    await expect(new TasksClient(lax).listProjects()).rejects.toThrow(TasksError);
  });

  it("a rejected call becomes a TasksError that names the method", async () => {
    const { fake, client } = setup();
    fake.failures.push({ method: "listProjects", nth: 1, error: new Error("down") });
    await expect(client.listProjects()).rejects.toThrow(/listProjects/);
  });
});

describe("transport", () => {
  it("every call goes through callRpc({pluginId:'tasks'})", async () => {
    const { fake, client } = setup();
    const p = fake.addProject("Autarch");
    const l = fake.addLabel(p.id, "needs-mk");
    const t = fake.addTask(p.id, { labelIds: [l.id] });
    await client.listProjects();
    await client.listLabels(p.id);
    await client.listTasks({ labelIds: [l.id] });
    await client.listComments(t.id);
    await client.taskState(t.id);
    expect(fake.calls.length).toBeGreaterThanOrEqual(6);
    expect(fake.calls.every((c) => c.pluginId === "tasks")).toBe(true);
    expect(new Set(fake.calls.map((c) => c.method))).toEqual(new Set(["listProjects", "listLabels", "listTasks", "listComments", "getTask"]));
  });

  it("the source never spawns a process", () => {
    const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "..", "tasks.ts"), "utf8");
    expect(src).not.toMatch(/child_process|execFile|spawn\(|exec\(/);
  });
});
