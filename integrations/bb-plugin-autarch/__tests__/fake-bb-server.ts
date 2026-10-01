// The fake `bb` server for the filing scenarios (Task 2.12). The built `autarch needs-mk file` binary calls
// `bb tasks ...` and `bb home get --request`; the stand-in bb (fake-bb.mjs) forwards both here over loopback.
//   - `bb tasks ...` is translated into FakeTasks RPCs, with the output shapes of the real tasks plugin CLI
//     (plugins/tasks/cli/index.ts): project list {projects}, label list {labels}, list {tasks,nextCursor,limit},
//     show {task,comments}, create {task,...}, comment {comment}; a missing task is "task not found: <id>".
//   - `bb home ...` is the real plugin CLI over the scenario's real Service.
// Faults are injected per tasks verb: the effect lands first, then the answer is dropped or never sent.
import { chmodSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import http from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { z } from "zod";
import type { FakeTasks, FakeTask } from "./tasks-fake.js";

const here = dirname(fileURLToPath(import.meta.url));
export interface CliOut {
  exitCode: number;
  stdout: string;
  stderr: string;
}
export type Fault = "drop-after-commit" | "hang-after-commit";
export interface FaultSpec {
  verb: "create" | "comment" | "show" | "list";
  kind: Fault;
  /** 1-based call of the verb to hit; default the next one. */
  nth?: number;
}

const any = z.any();
const ok = (v: unknown): CliOut => ({ exitCode: 0, stdout: JSON.stringify(v), stderr: "" });
const fail = (message: string, code = 1): CliOut => ({ exitCode: code, stdout: JSON.stringify({ error: message }), stderr: `${message}\n` });

/** Parse `--flag value` pairs and positionals; a repeated flag collects its values. */
function parse(args: string[]): { pos: string[]; flags: Map<string, string[]> } {
  const pos: string[] = [];
  const flags = new Map<string, string[]>();
  for (let i = 0; i < args.length; i++) {
    const a = args[i]!;
    if (a === "--json") continue;
    if (a.startsWith("--")) flags.set(a.slice(2), [...(flags.get(a.slice(2)) ?? []), args[++i] ?? ""]);
    else pos.push(a);
  }
  return { pos, flags };
}
const csv = (vs: string[] | undefined) => (vs ?? []).flatMap((v) => v.split(",")).map((v) => v.trim()).filter(Boolean);

export class TasksCli {
  constructor(readonly fake: FakeTasks, private readonly readFile: (path: string) => string = (p) => readFileSync(p, "utf8")) {}

  private call(method: string, input: unknown): Promise<any> {
    return this.fake.callRpc({ pluginId: "tasks", method, input, outputSchema: any });
  }
  private project(address: string | undefined) {
    const a = (address ?? "").toLowerCase();
    return this.fake.projects.find((p) => p.id.toLowerCase() === a || p.prefix.toLowerCase() === a || p.name.toLowerCase() === a);
  }
  private task(address: string): FakeTask | undefined {
    return this.fake.tasks.find((t) => t.id === address || t.key.toLowerCase() === address.toLowerCase());
  }
  private view(t: FakeTask) {
    return { ...t, labels: t.labelIds.map((id) => this.fake.labels.find((l) => l.id === id)?.name ?? id), agentsWorking: 0 };
  }

  /** Run `bb tasks <argv>`. `threadId` is BB_THREAD_ID. */
  async run(argv: string[], threadId?: string): Promise<CliOut> {
    try {
      return await this.dispatch(argv, threadId);
    } catch (e) {
      return fail(e instanceof Error ? e.message : String(e));
    }
  }

  private async dispatch(argv: string[], threadId?: string): Promise<CliOut> {
    const [verb, ...rest] = argv;
    if (verb === "project" || verb === "label") {
      const [sub, ...args] = rest;
      const { flags } = parse(args);
      if (verb === "project" && sub === "list") return ok({ projects: (await this.call("listProjects", {})).projects });
      const p = this.project(flags.get("project")?.[0]);
      if (verb === "label" && !p) return fail(`project not found: ${flags.get("project")?.[0] ?? ""}`);
      if (verb === "label" && sub === "list") return ok({ labels: (await this.call("listLabels", { projectId: p!.id })).labels });
      if (verb === "label" && sub === "create") return ok(await this.call("createLabel", { projectId: p!.id, name: flags.get("name")?.[0] }));
      return fail(`unsupported: bb tasks ${verb} ${sub ?? ""}`);
    }
    const { pos, flags } = parse(rest);
    switch (verb) {
      case "list": {
        const p = flags.has("project") ? this.project(flags.get("project")![0]) : undefined;
        if (flags.has("project") && !p) return fail(`project not found: ${flags.get("project")![0]}`);
        const statuses = csv(flags.get("status"));
        const limit = Number(flags.get("limit")?.[0] ?? 50);
        const r = await this.call("listTasks", { projectId: p?.id, statuses: statuses.length ? statuses : undefined, search: flags.get("search")?.[0], limit, cursor: flags.get("cursor")?.[0] });
        return ok({ tasks: (r.tasks as FakeTask[]).map((t) => this.view(t)), nextCursor: r.nextCursor, limit });
      }
      case "show": {
        const t = this.task(pos[0] ?? "");
        if (!t) return fail(`task not found: ${pos[0] ?? ""}`);
        return ok({ task: this.view(t), comments: (await this.call("listComments", { taskId: t.id })).comments, attachments: [], taskThreads: [], pullRequests: [] });
      }
      case "create": {
        const p = this.project(flags.get("project")?.[0]);
        if (!p) return fail(`project not found: ${flags.get("project")?.[0] ?? ""}`);
        const labelIds: string[] = [];
        for (const name of csv(flags.get("label"))) {
          const l = this.fake.labels.find((x) => x.projectId === p.id && x.name.toLowerCase() === name.toLowerCase());
          if (!l) return fail(`label not found: ${name}`);
          labelIds.push(l.id);
        }
        const file = flags.get("description-file")?.[0];
        const description = file !== undefined ? this.readFile(file) : (flags.get("description")?.[0] ?? "");
        const r = await this.call("createTask", { projectId: p.id, title: flags.get("title")?.[0], description, labelIds });
        return ok({ task: r.task, attachments: [], failedAttachments: [] });
      }
      case "comment": {
        const t = this.task(pos[0] ?? "");
        if (!t) return fail(`task not found: ${pos[0] ?? ""}`);
        const body = flags.get("body")?.[0];
        if (body === undefined) return fail("missing required --body or --body-file");
        return ok({ comment: (await this.call("createComment", { taskId: t.id, body, threadId: threadId || undefined })).comment });
      }
      default:
        return fail(`unsupported: bb tasks ${verb ?? ""}`);
    }
  }
}

export type HomeRun = (argv: string[], threadId?: string) => Promise<{ exitCode: number; stdout: string; stderr: string }>;

/** The loopback server plus the `bb` wrapper script that reaches it. */
export class FakeBbServer {
  readonly cli: TasksCli;
  private server: http.Server | undefined;
  private readonly faults: (FaultSpec & { seen: number })[] = [];
  private readonly counts = new Map<string, number>();
  /** Every `bb` call the binary made, in order: the first two argv words. */
  readonly log: string[] = [];
  url = "";
  bin = "";

  constructor(readonly fake: FakeTasks, private readonly home: HomeRun) {
    this.cli = new TasksCli(fake);
  }
  inject(f: FaultSpec): void {
    this.faults.push({ ...f, seen: 0 });
  }
  callsOf(verb: string): number {
    return this.counts.get(verb) ?? 0;
  }

  /** Write the wrapper into `dir` and listen on an ephemeral loopback port. */
  async start(dir: string): Promise<void> {
    const wrapper = join(dir, "bin", "bb");
    mkdirSync(dirname(wrapper), { recursive: true });
    writeFileSync(wrapper, `#!/bin/sh\nexec ${process.execPath} ${join(here, "..", "e2e", "fake-bb.mjs")} "$@"\n`);
    chmodSync(wrapper, 0o755);
    this.bin = wrapper;
    this.server = http.createServer((req, res) => {
      let text = "";
      req.on("data", (d) => (text += d));
      req.on("end", async () => {
        const { group, argv, threadId } = JSON.parse(text) as { group: string; argv: string[]; threadId?: string };
        this.log.push(`${group} ${argv[0] ?? ""}`);
        if (group !== "tasks") {
          const r = await this.home(argv, threadId);
          return void res.end(JSON.stringify(r));
        }
        const verb = argv[0] ?? "";
        const n = (this.counts.get(verb) ?? 0) + 1;
        this.counts.set(verb, n);
        const r = await this.cli.run(argv, threadId);
        const f = this.faults.find((x) => x.verb === verb && (x.nth === undefined ? x.seen === 0 : x.nth === n));
        if (f) {
          f.seen++;
          if (f.kind === "drop-after-commit") return void req.socket.destroy();
          return; // hang-after-commit: never answered; the caller's timeout fires
        }
        res.end(JSON.stringify(r));
      });
    });
    await new Promise<void>((r) => this.server!.listen(0, "127.0.0.1", r));
    this.url = `http://127.0.0.1:${(this.server.address() as { port: number }).port}/`;
  }

  async stop(): Promise<void> {
    this.server?.closeAllConnections?.();
    await new Promise<void>((r) => (this.server ? this.server.close(() => r()) : r()));
    this.server = undefined;
  }
}
