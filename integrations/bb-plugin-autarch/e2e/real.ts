// The real-bb driver (Task 1.11): the same scenarios against an ISOLATED bb server.
// The server is never started here. The operator starts it on its own data dir, port and HOME
// and names it in the environment:
//   HOME_E2E_BB            http://127.0.0.1:<port>          the isolated server
//   HOME_E2E_BB_HOST_PORT  the isolated server's host daemon port
//   HOME_E2E_BB_DATA       the isolated server's data dir
//   HOME_E2E_BB_HOME       the isolated server's HOME (optional)
//   HOME_E2E_BB_CLI        the bb executable (optional, default `bb` on PATH)
// Every bb call runs with a scrubbed environment, so an ambient BB_* can never redirect it.
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { chromium, type Browser, type Page } from "playwright-core";
import { FeedCaches } from "../feed.js";
import { freePort, pluginSettings, preflight, startOwnServe, statProject, type BuildFile, type Loaded, type OwnServe, type Readers } from "./preflight.js";

const sha = (b: string | Buffer) => createHash("sha256").update(b).digest("hex");
const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));
export type Evidence = Record<string, unknown>;

export interface RealEnv {
  url: string;
  hostPort: string;
  data: string;
  home: string;
  cli: string;
}

/** Refuse anything that is not clearly a separate, loopback, throwaway server. */
export function readRealEnv(env: NodeJS.ProcessEnv): RealEnv {
  const url = env.HOME_E2E_BB;
  const hostPort = env.HOME_E2E_BB_HOST_PORT;
  const data = env.HOME_E2E_BB_DATA;
  if (!url || !hostPort || !data) throw new Error("real-bb needs HOME_E2E_BB, HOME_E2E_BB_HOST_PORT and HOME_E2E_BB_DATA (an isolated bb server)");
  const u = new URL(url);
  if (u.hostname !== "127.0.0.1" && u.hostname !== "localhost") throw new Error(`HOME_E2E_BB must be loopback, not ${u.hostname}`);
  if (env.BB_SERVER_URL && new URL(env.BB_SERVER_URL).origin === u.origin) throw new Error("HOME_E2E_BB is the ambient bb server; refusing");
  if (env.BB_DATA_DIR && env.BB_DATA_DIR === data) throw new Error("HOME_E2E_BB_DATA is the ambient bb data dir; refusing");
  if (env.BB_HOST_DAEMON_PORT && env.BB_HOST_DAEMON_PORT === hostPort) throw new Error("HOME_E2E_BB_HOST_PORT is the ambient host daemon port; refusing");
  if (!existsSync(join(data, "bb.db"))) throw new Error(`HOME_E2E_BB_DATA ${data} has no bb.db`);
  if (data.includes(".bb-machines")) throw new Error("HOME_E2E_BB_DATA looks like a live machine's data; refusing");
  return { url: u.origin, hostPort, data, home: env.HOME_E2E_BB_HOME ?? join(data, "..", "home"), cli: env.HOME_E2E_BB_CLI ?? "bb" };
}

export class Real {
  readonly threads: string[] = [];
  private tmp = mkdtempSync(join(tmpdir(), "autarch-e2e-real-"));
  private project = "";
  private serve?: OwnServe;
  private browser?: Browser;
  readonly root: string;
  readonly parent: string;

  constructor(readonly env: RealEnv, readonly runId: string, readonly build: BuildFile) {
    this.parent = join(this.tmp, "projects");
    this.root = join(this.parent, "Autarch");
  }

  // ---- bb -------------------------------------------------------------------------
  bbEnv(threadId?: string): NodeJS.ProcessEnv {
    return {
      PATH: process.env.PATH,
      HOME: this.env.home,
      BB_SERVER_URL: this.env.url,
      BB_HOST_DAEMON_PORT: this.env.hostPort,
      BB_DATA_DIR: this.env.data,
      ...(threadId ? { BB_THREAD_ID: threadId } : {}),
    };
  }
  bb(args: string[], o: { thread?: string; input?: string; allowFail?: boolean } = {}): { code: number; stdout: string; stderr: string } {
    const r = spawnSync(this.env.cli, args, { env: this.bbEnv(o.thread), input: o.input, encoding: "utf8", timeout: 120_000 });
    const out = { code: r.status ?? 1, stdout: r.stdout ?? "", stderr: r.stderr ?? "" };
    if (out.code !== 0 && !o.allowFail) throw new Error(`bb ${args.join(" ")} failed (${out.code}): ${out.stderr || out.stdout}`);
    return out;
  }
  private rpc<T = unknown>(method: string, input: unknown = null): T {
    const f = join(this.tmp, `rpc-${method}.json`);
    writeFileSync(f, JSON.stringify(input));
    return JSON.parse(this.bb(["plugin", "rpc", "call", "autarch", method, "--input-file", f, "--json"]).stdout) as T;
  }

  // ---- setup ----------------------------------------------------------------------
  /** The scratch project, bb project, the harness's own serve, the plugin install, then the preflight. */
  async setup(): Promise<Loaded> {
    mkdirSync(join(this.root, "docs", "decisions"), { recursive: true });
    for (const a of [["init", "-q"], ["config", "user.email", "e2e@example.invalid"], ["config", "user.name", "e2e"]]) execFileSync("git", ["-C", this.root, ...a]);
    writeFileSync(join(this.root, "README.md"), "scratch\n");
    execFileSync("git", ["-C", this.root, "add", "."]);
    execFileSync("git", ["-C", this.root, "commit", "-q", "-m", "init"]);
    const created = JSON.parse(this.bb(["project", "create", "--name", "Autarch", "--root", this.root, "--json"]).stdout) as { id: string };
    this.project = created.id;
    this.serve = await startOwnServe({ autarchPath: this.build.autarch_path, dir: join(this.tmp, "serve"), parent: this.parent, home: join(this.tmp, "serve-home") });
    this.bb(["plugin", "install", this.build.plugin_dir, "--yes"]);
    const s = pluginSettings(this.serve, this.build.autarch_path);
    for (const [k, v] of Object.entries(s)) this.bb(["plugin", "config", "autarch", "set", k, Array.isArray(v) ? v.join(",") : String(v)]);
    this.bb(["plugin", "reload", "autarch"]);
    const readers: Readers = {
      health: async () => {
        for (let i = 0; i < 40; i++) {
          const h = this.rpc<{ source_sha256: string | null; build: { exe_sha256?: string } | null; projects: never; projects_error: string | null }>("health");
          if (h.build?.exe_sha256 || h.projects_error) return h as never;
          await sleep(500);
        }
        return this.rpc("health");
      },
      homeSource: async () => {
        const p = await this.page();
        await p.goto(`${this.env.url}/plugins/autarch/home`);
        const el = p.locator("[data-home-source]").first();
        await el.waitFor({ timeout: 30_000 });
        return el.getAttribute("data-home-source");
      },
      autarchVersion: async () => JSON.parse(execFileSync(this.build.autarch_path, ["version", "--json"], { encoding: "utf8" })) as { sha256: string },
    };
    return preflight(this.build, [statProject("Autarch", this.root)], readers);
  }

  private async page(): Promise<Page> {
    this.browser ??= await chromium.launch();
    const ctx = this.browser.contexts()[0] ?? (await this.browser.newContext({ viewport: { width: 1400, height: 900 } }));
    return ctx.pages()[0] ?? ctx.newPage();
  }

  // ---- helpers ----------------------------------------------------------------------
  /**
   * A scratch thread titled with the run id. It gets a real turn that fails at once with "Not logged
   * in" (the isolated HOME holds no credentials, so no model runs and nothing is spent); that turn is
   * what stores the thread's execution model, without which bb refuses a later wake.
   */
  spawn(name: string): string {
    const f = join(this.tmp, `prompt-${name}.txt`);
    writeFileSync(f, "scratch thread: do nothing\n");
    const t = JSON.parse(this.bb(["thread", "spawn", "--project", this.project, "--title", `e2e ${this.runId} ${name}`, "--prompt-file", f, "--provider", "claude-code", "--json"]).stdout) as { id: string };
    this.threads.push(t.id);
    return t.id;
  }
  askBody(thread: string, subject: string, over: Record<string, unknown> = {}): Record<string, unknown> {
    return {
      v: 1,
      kind: "decide",
      project: "Autarch",
      project_root: this.root,
      asker: "thread",
      thread,
      question: "Collapse routine catch-up items per project or per day?",
      subject,
      options: [
        { id: "project", label: "Collapse per project", kind: "instruction", reversible: true, instruction: "On branch feat/x, group per project, run npm test, commit locally, and report." },
        { id: "day", label: "Collapse per day", kind: "ruling-only" },
      ],
      ...over,
    };
  }
  /** `bb home ask --request-stdin` from inside `thread`. */
  ask(thread: string, subject: string): string {
    const r = this.bb(["home", "ask", "--request-stdin"], { thread, input: JSON.stringify(this.askBody(thread, subject)) });
    return (JSON.parse(r.stdout) as { id: string }).id;
  }
  db<T = Record<string, unknown>>(sql: string, ...a: unknown[]): T[] {
    const db = new Database(join(this.env.data, "plugins", "autarch", "data.db"), { readonly: true, timeout: 2000 });
    try {
      return db.prepare(sql).all(...a) as T[];
    } finally {
      db.close();
    }
  }
  async waitFor<T>(what: string, fn: () => T | undefined | false, ms = 30_000): Promise<T> {
    const end = Date.now() + ms;
    for (;;) {
      const v = fn();
      if (v) return v;
      if (Date.now() > end) throw new Error(`timed out after ${ms} ms waiting for ${what}`);
      await sleep(250);
    }
  }
  events(thread: string): { id: string; type: string; data?: { input?: { type?: string; text?: string }[] } }[] {
    return JSON.parse(this.bb(["thread", "log", thread, "--json", "--all"]).stdout);
  }
  async pick(decision: string, option: string, pick_id: string) {
    const listed = this.rpc<{ owed: { id: string; revision: string }[] }>("listAsks");
    const d = listed.owed.find((o) => o.id === decision);
    assert.ok(d, `decision ${decision} is not owed`);
    return this.rpc<{ ok: boolean; status: number }>("pick", { decision_id: decision, option_id: option, revision: d.revision, pick_id });
  }
  /** The feed text for (project, thread), from a read-only view of the real database. Not observed inside a thread. */
  feedText(thread: string): string {
    const db = new Database(join(this.env.data, "plugins", "autarch", "data.db"), { readonly: true });
    try {
      return new FeedCaches(() => db, () => Date.now()).configure("Autarch", thread) ?? "";
    } finally {
      db.close();
    }
  }
  rulingSha(): string {
    const dir = join(this.root, "docs", "decisions");
    const names = readdirSync(dir).filter((n) => n.endsWith(".md")).sort();
    assert.ok(names.length > 0, "no ruling file");
    return sha(readFileSync(join(dir, names[names.length - 1]!)));
  }

  // ---- scenarios ---------------------------------------------------------------------
  readonly scenarios: Record<string, () => Promise<Evidence>> = {
    "answer-instruction": async () => {
      const t = this.spawn("answer-instruction");
      const decision = this.ask(t, "e2e: collapse order");
      const pick_id = `pick-${decision}`;
      assert.equal((await this.pick(decision, "project", pick_id)).status, 201);
      await this.waitFor("the wake to finish", () => this.db("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'done'", decision).length > 0);
      const delivered = this.events(t).some((e) => e.type === "client/turn/requested" && (e.data?.input ?? []).some((p) => (p.text ?? "").includes("group per project") && (p.text ?? "").includes(decision)));
      assert.ok(delivered, "bb holds no turn carrying the ruling's instruction");
      const feed = this.feedText(t);
      const line = feed.split("\n").map((l) => l.trim()).find((l) => l.startsWith("ruled ") && l.includes(`(${decision})`));
      assert.ok(line, `no feed line for ${decision}`);
      assert.ok(!feed.includes("group per project"), "the instruction text leaked into the feed");
      return {
        threads: [t],
        decision,
        pick_id,
        ruling_sha256: this.rulingSha(),
        wake_state: "done",
        wake_count: this.db<{ n: number }>("SELECT COUNT(*) n FROM obligations WHERE decision_id = ? AND kind = 'wake'", decision)[0]!.n,
        feed_line: line,
      };
    },

    // A wake queues only when its thread has an active turn. A scratch thread with no credentials never
    // holds one, so bb answers `sent`; this scenario fails then instead of pretending.
    "queued-then-archived": async () => {
      const t = this.spawn("queued-then-archived");
      const decision = this.ask(t, "e2e: queued wake");
      assert.equal((await this.pick(decision, "project", `pick-${decision}`)).status, 201);
      const state = await this.waitFor("the wake to settle", () => this.db<{ state: string }>("SELECT state FROM obligations WHERE decision_id = ? AND kind = 'wake'", decision)[0]?.state.match(/^(queued|done|undeliverable)$/)?.[0]);
      if (state !== "queued") throw new Error(`the wake settled as ${state}, not queued: a queued delivery needs an active turn, and no turn can run on this server without model credentials`);
      this.bb(["thread", "archive", t]);
      await this.waitFor("the wake to become undeliverable", () => this.db("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'undeliverable'", decision).length > 0);
      const listed = this.rpc<{ undeliverable: { decision_id: string }[] }>("listAsks").undeliverable.some((o) => o.decision_id === decision);
      return { threads: [t], decision, wake_state: "undeliverable", listed };
    },

    "ask-cli-proxy": async () => {
      const t = this.spawn("ask-cli-proxy");
      const other = this.spawn("ask-cli-proxy-other");
      const decision = this.ask(t, "e2e: proxy ask from a thread");
      const req = (subject: string) => JSON.stringify({ v: 1, kind: "decide", question: "Collapse routine catch-up items per project or per day?", subject, options: this.askBody(t, subject).options });
      const decide = (subject: string, threadEnv?: string) =>
        spawnSync(this.build.autarch_path, ["decide", "file", "--thread", t], { cwd: this.root, env: { ...this.bbEnv(threadEnv), PATH: this.env.cli.includes("/") ? `${dirname(this.env.cli)}:${process.env.PATH}` : process.env.PATH }, input: req(subject), encoding: "utf8", timeout: 60_000 });
      const absent = decide("e2e: decide file, BB_THREAD_ID absent");
      const conflict = decide("e2e: decide file, BB_THREAD_ID conflicting", other);
      assert.equal(absent.status, 0, `absent: ${absent.stderr}`);
      assert.equal(conflict.status, 0, `conflict: ${conflict.stderr}`);
      const rows = this.db<{ thread: string | null }>("SELECT thread FROM decisions WHERE subject LIKE 'e2e: decide file%'");
      assert.equal(rows.length, 2);
      assert.ok(rows.every((r) => r.thread === t), "a decide-file ask was filed under another thread");
      return { threads: [t, other], decision, env_absent_exit: absent.status, env_conflict_exit: conflict.status };
    },

    "vizier-chat": async () => {
      const v = this.spawn("vizier");
      const set = this.rpc<{ ok: boolean }>("setDelegation", { vizierThreadId: v, projects: ["Autarch"], dailyCap: 5 });
      assert.ok(set.ok, JSON.stringify(set));
      const before = new Set(this.events(v).filter((e) => e.type === "client/turn/requested").map((e) => e.id));
      const p = await this.page();
      await p.goto(`${this.env.url}/plugins/autarch/home`);
      await p.getByRole("button", { name: "Vizier", exact: true }).click();
      const box = p.getByRole("textbox", { name: "Reply…" });
      await box.waitFor({ timeout: 30_000 });
      const marker = `vizier-chat ${this.runId}`;
      await box.click();
      await p.keyboard.type(marker);
      await p.getByRole("button", { name: /^Submit/ }).click();
      await p.getByText(marker).first().waitFor({ timeout: 30_000 });
      const shot = await p.screenshot();
      const ev = await this.waitFor("the composer message in bb", () => this.events(v).find((e) => e.type === "client/turn/requested" && !before.has(e.id) && (e.data?.input ?? []).some((x) => (x.text ?? "").includes(marker))));
      return { threads: [v], message_id: ev.id, screenshot_sha256: sha(shot) };
    },
  };

  // ---- cleanup -------------------------------------------------------------------------
  /** Archive exactly the threads the scenarios recorded, stop the harness's serve, remove its scratch dirs. */
  async cleanup(recorded: string[]): Promise<{ archived: string[]; failed: string[] }> {
    const archived: string[] = [];
    const failed: string[] = [];
    for (const id of new Set(recorded)) {
      const r = this.bb(["thread", "archive", id], { allowFail: true });
      (r.code === 0 || /already archived/i.test(r.stderr + r.stdout) ? archived : failed).push(id);
    }
    await this.browser?.close().catch(() => undefined);
    await this.serve?.stop();
    rmSync(this.tmp, { recursive: true, force: true });
    return { archived, failed };
  }
}

export { freePort };
