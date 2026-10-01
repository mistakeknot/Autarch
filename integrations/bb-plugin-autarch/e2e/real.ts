// The real-bb driver (Tasks 1.11, 2.12): the scenarios against a bb server the harness launched and proved it
// owns (launcher.ts, ownership.ts), inside `bwrap --unshare-net`. The RealEnv is filled from that launch.
// Cards are filed with `autarch needs-mk file`; `bb home ask` is retired and is not used anywhere.
// Every bb call runs with a scrubbed environment, so an ambient BB_* can never redirect it.
import assert from "node:assert/strict";
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { chromium, type Browser, type Page } from "playwright-core";
import { FeedCaches } from "../feed.js";
import { makeTarget, resolveBin, rigExecSync, rigRpc, type RigTarget } from "./rigexec.js";
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
  /** The tasks project the filer files into (created in setup, through rigRpc). */
  private tasksPrefix = "";
  private serve?: OwnServe;
  private browser?: Browser;
  readonly root: string;
  readonly parent: string;

  /** `owned` is the harness-launched, ownership-proven target (launcher.ts); without it the target is built lazily (tests). */
  constructor(readonly env: RealEnv, readonly runId: string, readonly build: BuildFile, owned?: RigTarget) {
    this.tgt = owned;
    this.parent = join(this.tmp, "projects");
    this.root = join(this.parent, "Autarch");
  }

  // ---- bb -------------------------------------------------------------------------
  /** The rig target: built lazily so a refusal (ambient port, no data dir) surfaces at the first call. */
  private tgt?: RigTarget;
  get target(): RigTarget {
    return (this.tgt ??= makeTarget({ url: this.env.url, dataDir: this.env.data, home: this.env.home, hostPort: this.env.hostPort, realBb: resolveBin(this.env.cli) ?? this.env.cli, requireProof: true }));
  }
  bb(args: string[], o: { thread?: string; input?: string; allowFail?: boolean } = {}): { code: number; stdout: string; stderr: string } {
    const r = rigExecSync("bb", args, { target: this.target, threadId: o.thread, input: o.input, timeoutMs: 120_000 });
    const out = { code: r.code ?? 1, stdout: r.stdout, stderr: r.stderr };
    if (out.code !== 0 && !o.allowFail) throw new Error(`bb ${args.join(" ")} failed (${out.code}): ${out.stderr || out.stdout}`);
    return out;
  }
  private rpc<T = unknown>(method: string, input: unknown = null): Promise<T> {
    return rigRpc<T>(this.target, "autarch", method, input);
  }

  // ---- setup ----------------------------------------------------------------------
  /** The scratch project, bb project, the harness's own serve, the plugin install, then the preflight. */
  async setup(): Promise<Loaded> {
    mkdirSync(join(this.root, "docs", "decisions"), { recursive: true });
    for (const a of [["init", "-q"], ["config", "user.email", "e2e@example.invalid"], ["config", "user.name", "e2e"]]) rigExecSync("git", ["-C", this.root, ...a]);
    writeFileSync(join(this.root, "README.md"), "scratch\n");
    rigExecSync("git", ["-C", this.root, "add", "."]);
    rigExecSync("git", ["-C", this.root, "commit", "-q", "-m", "init"]);
    const created = JSON.parse(this.bb(["project", "create", "--name", "Autarch", "--root", this.root, "--json"]).stdout) as { id: string };
    this.project = created.id;
    this.tasksPrefix = `E${sha(this.runId).slice(0, 9).toUpperCase()}`;
    await rigRpc(this.target, "tasks", "createProject", { name: `Autarch ${this.runId}`, prefix: this.tasksPrefix, color: "#6b7280" });
    this.serve = await startOwnServe({ autarchPath: this.build.autarch_path, dir: join(this.tmp, "serve"), parent: this.parent, home: join(this.tmp, "serve-home") });
    this.bb(["plugin", "install", this.build.plugin_dir, "--yes"]);
    const s = pluginSettings(this.serve, this.build.autarch_path);
    for (const [k, v] of Object.entries(s)) this.bb(["plugin", "config", "autarch", "set", k, Array.isArray(v) ? v.join(",") : String(v)]);
    this.bb(["plugin", "reload", "autarch"]);
    // `bb thread spawn` resolves a default model from the provider catalog, which a fresh server fills a few seconds
    // after it starts (and only when it can find the claude CLI). Wait for it rather than fail the first scenario.
    await this.waitFor("the claude-code model catalog", () => this.bb(["provider", "models", "claude-code"], { allowFail: true }).code === 0, 90_000);
    const readers: Readers = {
      health: async () => {
        for (let i = 0; i < 40; i++) {
          const h = await this.rpc<{ source_sha256: string | null; build: { exe_sha256?: string } | null; projects: never; projects_error: string | null }>("health");
          if (h.build?.exe_sha256 || h.projects_error) return h as never;
          await sleep(500);
        }
        return await this.rpc("health");
      },
      homeSource: async () => {
        const p = await this.page();
        await p.goto(`${this.env.url}/plugins/autarch/home`);
        const el = p.locator("[data-home-source]").first();
        await el.waitFor({ timeout: 30_000 });
        return el.getAttribute("data-home-source");
      },
      autarchVersion: async () => JSON.parse(rigExecSync(this.build.autarch_path, ["version", "--json"], { home: join(this.tmp, "serve-home") }).stdout) as { sha256: string },
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
  /** Wait until the thread has no active turn (its status is neither starting nor running). */
  async settle(thread: string): Promise<void> {
    await this.waitFor(`thread ${thread} to stop being active`, () => {
      const st = (JSON.parse(this.bb(["thread", "show", thread, "--json"]).stdout) as { thread?: { status?: string } }).thread?.status ?? "";
      return !/^(starting|running)$/i.test(st) && st !== "";
    }, 60_000);
  }
  /** The ask file `autarch needs-mk file --ask-file` reads: the decision's question and options. */
  askFile(name: string, question: string): string {
    const f = join(this.tmp, `ask-${name}.json`);
    writeFileSync(
      f,
      JSON.stringify({
        question,
        subject: question,
        project: "Autarch",
        project_root: this.root,
        options: [
          { id: "project", label: "Collapse per project", kind: "instruction", reversible: true, instruction: "On branch feat/x, group per project, run npm test, commit locally, and report." },
          { id: "day", label: "Collapse per day", kind: "ruling-only" },
        ],
      }),
    );
    return f;
  }
  /** `autarch needs-mk file` from inside `thread` (BB_THREAD_ID), filing a card through `bb tasks`. Replaces the retired `bb home ask`. */
  file(thread: string | undefined, key: string, subject: string, project = this.tasksPrefix): { code: number; stderr: string; card?: string } {
    const args = ["needs-mk", "file", "--project", project, "--title", subject, "--ask-file", this.askFile(key, subject), "--request", key, "--blocks", `thread:${thread ?? "none"}`];
    const r = rigExecSync(this.build.autarch_path, args, { target: this.target, threadId: thread, cwd: this.root, timeoutMs: 90_000 });
    let card: string | undefined;
    try {
      card = (JSON.parse(r.stdout.trim().split("\n").pop() ?? "") as { card?: string }).card;
    } catch {
      /* no card on a failed run */
    }
    return { code: r.code ?? 1, stderr: r.stderr, card };
  }
  /** File a card from `thread` and wait for the poller to open its decision; returns the decision id `card-<card>-g1`. */
  async ask(thread: string, subject: string): Promise<string> {
    const r = this.file(thread, `${this.runId}-${sha(subject).slice(0, 8)}`, subject);
    assert.equal(r.code, 0, `needs-mk file exited ${r.code}: ${r.stderr}`);
    const decision = `card-${r.card}-g1`;
    await this.waitFor(`the poller to open ${decision}`, () => this.db("SELECT 1 FROM decisions WHERE id = ?", decision).length > 0, 60_000);
    return decision;
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
    const listed = await this.rpc<{ owed: { id: string; revision: string }[] }>("listAsks");
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
      await this.settle(t);
      const decision = await this.ask(t, "e2e: collapse order");
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

    // A wake queues only while its thread has an active turn. A scratch thread has no credentials, so a
    // turn started with `bb thread tell` fails within moments; the pick races that turn, over fresh
    // threads, and the scenario fails if no wake ever queues.
    "queued-then-archived": async () => {
      let last = "none";
      for (let attempt = 1; attempt <= 6; attempt++) {
        const t = this.spawn(`queued-then-archived-${attempt}`);
        await this.settle(t);
        const decision = await this.ask(t, `e2e: queued wake ${attempt}`);
        const listed = (await this.rpc<{ owed: { id: string; revision: string }[] }>("listAsks")).owed.find((o) => o.id === decision);
        assert.ok(listed, `decision ${decision} is not owed`);
        this.bb(["thread", "tell", t, `e2e ${this.runId} keep busy`]);
        const r = await this.rpc<{ ok: boolean; status: number }>("pick", { decision_id: decision, option_id: "project", revision: listed.revision, pick_id: `pick-${decision}` });
        assert.equal(r.status, 201);
        const state = await this.waitFor("the wake to settle", () => this.db<{ state: string }>("SELECT state FROM obligations WHERE decision_id = ? AND kind = 'wake'", decision)[0]?.state.match(/^(queued|done|undeliverable)$/)?.[0]);
        last = state;
        if (state !== "queued") continue;
        this.bb(["thread", "archive", t]);
        await this.waitFor("the wake to become undeliverable", () => this.db("SELECT 1 FROM obligations WHERE decision_id = ? AND kind = 'wake' AND state = 'undeliverable'", decision).length > 0);
        const shown = (await this.rpc<{ undeliverable: { decision_id: string }[] }>("listAsks")).undeliverable.some((o) => o.decision_id === decision);
        return { threads: [t], decision, wake_state: "undeliverable", listed: shown };
      }
      throw new Error(`no wake queued in 6 attempts (last settled as ${last}): a queued delivery needs an active turn, and a credential-less scratch turn ends too fast`);
    },

    // Replaces rev-4 `ask-cli-proxy`. The filer requires a thread: BB_THREAD_ID absent exits 2, and a replay of
    // the same Request key from another thread exits 2 without a second card.
    "filer-from-thread": async () => {
      const t = this.spawn("filer-from-thread");
      const other = this.spawn("filer-from-thread-other");
      const key = `${this.runId}-filer`;
      const first = this.file(t, key, "e2e: file from a thread");
      assert.equal(first.code, 0, `filer exit ${first.code}: ${first.stderr}`);
      const shown = JSON.parse(this.bb(["tasks", "show", first.card!, "--json"]).stdout) as { comments: { kind?: string; threadId?: string | null }[] };
      const agent = shown.comments.filter((c) => c.kind === "agent");
      const matches = agent.length === 1 && agent[0]!.threadId === t;
      assert.ok(matches, "the card's agent comment does not carry the filing thread");
      const absent = this.file(undefined, `${key}-absent`, "e2e: file with BB_THREAD_ID absent");
      assert.equal(absent.code, 2, `BB_THREAD_ID absent must exit 2: ${absent.stderr}`);
      const replay = this.file(other, key, "e2e: file from a thread");
      assert.equal(replay.code, 2, `a replay from another thread must exit 2: ${replay.stderr}`);
      return { threads: [t, other], card: first.card, comment_thread_matches: matches, env_absent_exit: absent.code, other_thread_replay_exit: replay.code };
    },

    // A card the poller shows without the page reloading: a marker set on `window` before the filing must survive
    // until the new card's subject is on screen, so no navigation happened in between.
    "poller-refresh": async () => {
      const t = this.spawn("poller-refresh");
      await this.settle(t);
      const p = await this.page();
      await p.goto(`${this.env.url}/plugins/autarch/home`);
      await p.locator("[data-home-source]").first().waitFor({ timeout: 30_000 });
      await p.evaluate((m) => ((window as unknown as Record<string, string>).__e2eNoReload = m), this.runId);
      const subject = `e2e: refresh ${this.runId}`;
      const r = this.file(t, `${this.runId}-refresh`, subject);
      assert.equal(r.code, 0, `needs-mk file exited ${r.code}: ${r.stderr}`);
      await p.getByText(subject).first().waitFor({ timeout: 60_000 });
      const marker = await p.evaluate(() => (window as unknown as Record<string, string>).__e2eNoReload);
      assert.equal(marker, this.runId, "the page reloaded: the window marker is gone");
      return { threads: [t], card: r.card, panel_updated_without_reload: true };
    },

    // A card with no agent comment is observed and not routed; a later agent comment carrying a thread routes it
    // (the comment changes no task field, so updatedAt does not move: the poller must read comments to see it).
    "comment-arrives-later-real": async () => {
      const t = this.spawn("comment-arrives-later");
      await this.settle(t);
      const key = `${this.runId}-late`;
      const ask = JSON.stringify({ schema: "home-ask/v2", project: "Autarch", project_root: this.root, question: "e2e: a late comment", subject: "e2e: a late comment", options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] });
      const desc = join(this.tmp, "late-card.md");
      writeFileSync(desc, `e2e: a late comment\n\nBlocks: thread:${t}\nRequest: ${key} sha256:${sha(key).slice(0, 16)}\n\n\`\`\`home-ask\n${ask}\n\`\`\`\n`);
      this.bb(["tasks", "label", "create", "--project", this.tasksPrefix, "--name", "needs-mk"], { allowFail: true });
      const made = JSON.parse(this.bb(["tasks", "create", "--project", this.tasksPrefix, "--title", "e2e: a late comment", "--description-file", desc, "--label", "needs-mk", "--json"]).stdout) as { id?: string; key?: string; task?: { id?: string; key?: string } };
      const id = made.id ?? made.task?.id;
      const taskKey = made.key ?? made.task?.key;
      assert.ok(id && taskKey, `tasks create returned no id: ${JSON.stringify(made)}`);
      const row = (): { state: string; routed_thread: string | null } | undefined => this.db<{ state: string; routed_thread: string | null }>("SELECT state, routed_thread FROM cards WHERE task_id = ?", id)[0];
      await this.waitFor("the poller to observe the card", () => row()?.state === "observed", 60_000);
      const before = (JSON.parse(this.bb(["tasks", "show", taskKey!, "--json"]).stdout) as { task?: { updatedAt?: string }; updatedAt?: string });
      const stamp = before.task?.updatedAt ?? before.updatedAt;
      const c = rigExecSync("bb", ["tasks", "comment", taskKey!, "--body", `asked from ${t}`, "--json"], { target: this.target, threadId: t, timeoutMs: 60_000 });
      assert.equal(c.code, 0, `tasks comment failed: ${c.stderr}`);
      const open = await this.waitFor("the late comment to route the card", () => (row()?.state === "open" ? row() : undefined), 60_000);
      assert.equal(open.routed_thread, t);
      const after = (JSON.parse(this.bb(["tasks", "show", taskKey!, "--json"]).stdout) as { task?: { updatedAt?: string }; updatedAt?: string });
      return { threads: [t], card: id, routed_thread: open.routed_thread, updated_at_before: stamp, updated_at_after: after.task?.updatedAt ?? after.updatedAt };
    },

    // Cards in two tracker projects both reach the one panel.
    "cross-project-panel": async () => {
      const t = this.spawn("cross-project");
      await this.settle(t);
      const second = `F${sha(this.runId).slice(0, 9).toUpperCase()}`;
      await rigRpc(this.target, "tasks", "createProject", { name: `Autarch second ${this.runId}`, prefix: second, color: "#6b7280" });
      const a = this.file(t, `${this.runId}-xp-a`, `e2e: project one ${this.runId}`);
      const b = this.file(t, `${this.runId}-xp-b`, `e2e: project two ${this.runId}`, second);
      assert.equal(a.code, 0, `file into ${this.tasksPrefix}: ${a.stderr}`);
      assert.equal(b.code, 0, `file into ${second}: ${b.stderr}`);
      const p = await this.page();
      await p.goto(`${this.env.url}/plugins/autarch/home`);
      await p.getByText(`e2e: project one ${this.runId}`).first().waitFor({ timeout: 60_000 });
      await p.getByText(`e2e: project two ${this.runId}`).first().waitFor({ timeout: 60_000 });
      const rows = this.db<{ project_id: string }>("SELECT project_id FROM cards WHERE task_id IN (?, ?) AND state = 'open'", a.card, b.card);
      assert.equal(rows.length, 2, "both cards are open");
      return { threads: [t], cards: rows.length, projects: new Set(rows.map((r) => r.project_id)).size };
    },

    "vizier-chat": async () => {
      const v = this.spawn("vizier");
      const set = await this.rpc<{ ok: boolean }>("setDelegation", { vizierThreadId: v, projects: ["Autarch"], dailyCap: 5 });
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
