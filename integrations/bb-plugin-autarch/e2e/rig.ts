// The fake-mode rig: the real plugin wiring (wireHome) over a real database file and real
// project directories, with only bb itself faked (its CLI proxy, events and thread SDK).
import type { ChildProcess } from "node:child_process";
import { rigExec, rigExecSync, rigSpawn } from "./rigexec.js";
import Database from "better-sqlite3";
import { chmodSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import http from "node:http";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ServeClient } from "../serve.js";
import { createStoreHandle, type StoreHandle } from "../store.js";
import { wireHome, type HomeConfig } from "../server.js";
import { FakeSdk } from "../__tests__/wakes-helpers.js";
import { OPTIONS } from "../__tests__/service-helpers.js";

const here = dirname(fileURLToPath(import.meta.url));
export const PLUGIN_DIR = join(here, "..");
export { OPTIONS, FakeSdk };

type CliResult = { exitCode?: number; stdout?: string; stderr?: string };
type Cli = { run(argv: string[], ctx: object): Promise<CliResult> | CliResult };
type Service = { start(signal: AbortSignal): Promise<void> };

export const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export async function waitFor<T>(what: string, fn: () => T | undefined | false | Promise<T | undefined | false>, ms = 10_000): Promise<T> {
  const end = Date.now() + ms;
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() > end) throw new Error(`timed out after ${ms} ms waiting for ${what}`);
    await sleep(50);
  }
}

export interface Child {
  proc: ChildProcess;
  out: Promise<string>;
  stdout: () => string;
  exit: Promise<{ code: number | null; signal: NodeJS.Signals | null }>;
}

export function spawnChild(args: string[]): Child {
  const proc = rigSpawn(process.execPath, ["--import", "tsx", join(here, "child.ts"), ...args]);
  let out = "";
  proc.stdout!.on("data", (d) => (out += d));
  const exit = new Promise<{ code: number | null; signal: NodeJS.Signals | null }>((r) => proc.on("exit", (code, signal) => r({ code, signal })));
  return { proc, exit, stdout: () => out, out: exit.then(() => out) };
}

export class Rig {
  readonly dir = mkdtempSync(join(tmpdir(), "autarch-e2e-"));
  readonly file = join(this.dir, "data.db");
  readonly projectsFile = join(this.dir, "projects.json");
  readonly roots: Record<string, string> = {};
  readonly sdk = new FakeSdk();
  private readonly events = new Map<string, ((p: unknown) => unknown)[]>();
  private readonly services = new Map<string, Service>();
  private cli: Cli | undefined;
  private disposers: (() => void)[] = [];
  private abort = new AbortController();
  private bbServer: http.Server | undefined;
  handle!: StoreHandle;
  home!: ReturnType<typeof wireHome>;
  /** Fault applied to the next `ask` through the fake bb. */
  fault: "none" | "drop-response-after-commit" | "hang-response" = "none";
  private hanging = false;
  bbBin = "";
  bbUrl = "";
  readonly warnings: string[] = [];

  constructor(names = ["Autarch", "P", "Q"]) {
    const list = names.map((name) => {
      const root = join(this.dir, "projects", name);
      mkdirSync(join(root, "docs", "decisions"), { recursive: true });
      rigExecSync("git", ["init", "-q", root]);
      const st = lstatSync(root);
      this.roots[name] = root;
      return { name, root, dev: st.dev, ino: st.ino };
    });
    writeFileSync(this.projectsFile, JSON.stringify(list));
  }

  /** Wire the plugin over the database file; the handle retries opening while it is locked. */
  wire(): void {
    this.abort = new AbortController();
    this.events.clear();
    this.services.clear();
    const cfg: HomeConfig = { serveAddr: "127.0.0.1:1", serveTokenFile: "/nonexistent", serveProjectDirs: [], autarchBin: "autarch", rulingsLedger: null };
    const serve = { projects: async () => JSON.parse(readFileSync(this.projectsFile, "utf8")), health: async () => ({}), healthy: async () => true } as unknown as ServeClient;
    this.handle = createStoreHandle(() => new Database(this.file, { timeout: 200 }), { initialDelayMs: 300, maxDelayMs: 300, closeOnFailure: true });
    const bb = {
      log: { info: () => {}, warn: (m: string) => this.warnings.push(m), error: (m: string) => this.warnings.push(m) },
      onDispose: (fn: () => void) => this.disposers.push(fn),
      background: { service: (name: string, s: Service) => this.services.set(name, s) },
      events: { on: (name: string, fn: (p: unknown) => unknown) => this.events.set(name, [...(this.events.get(name) ?? []), fn]) },
      agents: { configure: () => {} },
      cli: { register: (c: Cli) => (this.cli = c) },
      sdk: { threads: {} },
    };
    this.home = wireHome(bb as never, this.handle, cfg, { serve, sdk: this.sdk });
  }

  /** Start the wake loop (the home-wakes background service). */
  startWakes(): void {
    void this.services.get("home-wakes")!.start(this.abort.signal);
  }

  emit(name: string, payload: unknown): void {
    for (const fn of this.events.get(name) ?? []) void fn(payload);
  }

  async run(argv: string[], threadId?: string) {
    const r = await this.cli!.run(argv, threadId ? { threadId } : {});
    return { exitCode: r.exitCode ?? 0, stdout: r.stdout ?? "", stderr: r.stderr ?? "" };
  }

  /** File an ask through the plugin CLI, as bb would. */
  async ask(threadId: string, req: Record<string, unknown>) {
    return this.run(["ask", "--request", JSON.stringify(req)], threadId);
  }

  askBody(over: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      v: 1,
      kind: "decide",
      project: "Autarch",
      project_root: this.roots.Autarch,
      asker: "thread",
      thread: "thr-a",
      question: "Collapse routine catch-up items per project or per day?",
      subject: "autarch/catch-up: collapse order",
      options: OPTIONS,
      ...over,
    };
  }

  /** A read-only view of the database for evidence. */
  read<T = Record<string, unknown>>(sql: string, ...args: unknown[]): T[] {
    const db = new Database(this.file, { readonly: true, timeout: 2000 });
    try {
      return db.prepare(sql).all(...args) as T[];
    } finally {
      db.close();
    }
  }

  /** Serve the plugin CLI to the fake bb binary over loopback. */
  async serveBb(): Promise<void> {
    const wrapper = join(this.dir, "bin", "bb");
    mkdirSync(dirname(wrapper), { recursive: true });
    writeFileSync(wrapper, `#!/bin/sh\nexec ${process.execPath} ${join(here, "fake-bb.mjs")} "$@"\n`);
    chmodSync(wrapper, 0o755);
    this.bbBin = dirname(wrapper);
    this.bbServer = http.createServer((req, res) => {
      let text = "";
      req.on("data", (d) => (text += d));
      req.on("end", async () => {
        const { argv, threadId } = JSON.parse(text) as { argv: string[]; threadId?: string };
        // A hang lasts through the recovery read that follows, until the next ask.
        if (argv[0] === "ask") {
          this.hanging = this.fault === "hang-response";
        }
        const fault = argv[0] === "ask" ? this.fault : this.hanging && argv[0] === "get" ? "hang-response" : "none";
        if (argv[0] === "ask") this.fault = "none";
        const r = await this.run(argv, threadId);
        if (fault === "drop-response-after-commit") return void req.socket.destroy();
        if (fault === "hang-response") return; // never answered; the caller's timeout fires
        res.end(JSON.stringify(r));
      });
    });
    await new Promise<void>((r) => this.bbServer!.listen(0, "127.0.0.1", r));
    this.bbUrl = `http://127.0.0.1:${(this.bbServer!.address() as { port: number }).port}/`;
  }

  /** Stop the plugin (as bb unloading it) but keep the directory. */
  stop(): void {
    this.abort.abort();
    for (const d of this.disposers) d();
    this.disposers = [];
    try {
      this.handle?.dispose();
    } catch {
      /* not open */
    }
    this.bbServer?.closeAllConnections?.();
    this.bbServer?.close();
    this.bbServer = undefined;
  }

  cleanup(): void {
    this.stop();
    for (const r of Object.values(this.roots)) {
      try {
        chmodSync(join(r, "docs", "decisions"), 0o755);
      } catch {
        /* gone */
      }
    }
    rmSync(this.dir, { recursive: true, force: true });
  }

  /** The ruling file name in a project's docs/decisions, if one exists. */
  ruling(project: string): string | undefined {
    const d = join(this.roots[project]!, "docs", "decisions");
    return existsSync(d) ? readdirSync(d).find((f) => f.endsWith(".md")) : undefined;
  }
}

/** Run a command without blocking this process's event loop (the fake bb is served from it). */
export function runAsync(cmd: string, args: string[], o: { cwd?: string; input?: string; threadId?: string; target?: import("./rigexec.js").RigTarget }): Promise<{ code: number | null; stdout: string; stderr: string }> {
  return rigExec(cmd, args, o);
}
