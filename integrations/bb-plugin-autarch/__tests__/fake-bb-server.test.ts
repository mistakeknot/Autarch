// The `bb tasks` emulation the filing scenarios stand on: output shapes of the real tasks plugin CLI.
import { describe, expect, it } from "vitest";
import { TasksCli } from "./fake-bb-server.js";
import { FakeTasks } from "./tasks-fake.js";

const setup = () => {
  const fake = new FakeTasks();
  const p = fake.addProject("Autarch");
  fake.addLabel(p.id, "needs-mk");
  return { fake, p, cli: new TasksCli(fake, () => "Which?\n\nRequest: k sha256:0123456789abcdef\n") };
};
const j = (o: { stdout: string }) => JSON.parse(o.stdout);

describe("TasksCli", () => {
  it("lists projects and labels", async () => {
    const { cli, p } = setup();
    expect(j(await cli.run(["project", "list", "--json"])).projects.map((x: { id: string }) => x.id)).toEqual([p.id]);
    expect(j(await cli.run(["label", "list", "--project", p.prefix, "--json"])).labels[0].name).toBe("needs-mk");
  });
  it("creates a labelled card from a description file, finds it by search, shows it with its comments", async () => {
    const { cli, p, fake } = setup();
    const made = await cli.run(["create", "--project", p.id, "--title", "T", "--description-file", "/x", "--label", "needs-mk", "--json"]);
    expect(made.exitCode).toBe(0);
    const id = j(made).task.id;
    expect(fake.tasks[0]!.labelIds).toHaveLength(1);
    const found = j(await cli.run(["list", "--project", p.id, "--status", "backlog,todo", "--search", "Request: k", "--limit", "50", "--json"]));
    expect(found.tasks.map((t: { id: string }) => t.id)).toEqual([id]);
    expect(found.nextCursor).toBeNull();
    const c = j(await cli.run(["comment", id, "--body", "hi", "--json"], "thr_a")).comment;
    expect(c).toMatchObject({ kind: "agent", threadId: "thr_a" });
    const shown = j(await cli.run(["show", id, "--json"]));
    expect(shown.task.description).toContain("Request: k");
    expect(shown.comments).toHaveLength(1);
  });
  it("a comment without a thread is a user comment", async () => {
    const { cli, p, fake } = setup();
    const t = fake.addTask(p.id);
    expect(j(await cli.run(["comment", t.id, "--body", "x"])).comment.kind).toBe("user");
  });
  it("a missing task is `task not found: <id>`, the text the Go filer's notFoundRe matches", async () => {
    const { cli } = setup();
    const r = await cli.run(["show", "01NOPE", "--json"]);
    expect(r.exitCode).toBe(1);
    expect(r.stderr).toBe("task not found: 01NOPE\n");
    expect(/not found|no such task|unknown task|does not exist/i.test(r.stderr)).toBe(true);
  });
  it("an unknown label or project fails without creating anything", async () => {
    const { cli, p, fake } = setup();
    expect((await cli.run(["create", "--project", p.id, "--title", "T", "--label", "nope"])).exitCode).toBe(1);
    expect((await cli.run(["create", "--project", "ZZZ", "--title", "T"])).exitCode).toBe(1);
    expect(fake.tasks).toHaveLength(0);
  });
});
