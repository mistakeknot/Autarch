// The real-bb driver (Tasks 1.11, 2.12): the scenarios against a bb server the harness launched and proved it
// owns (launcher.ts, ownership.ts), inside `bwrap --unshare-net`. The RealEnv is filled from that launch.
// Cards are filed with `autarch needs-mk file`; `bb home ask` is retired and is not used anywhere.
// Every bb call runs with a scrubbed environment, so an ambient BB_* can never redirect it.
import assert from "node:assert/strict";
import Database from "better-sqlite3";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium, type Browser, type Page } from "playwright-core";
import { FeedCaches } from "../feed.js";
import { makeTarget, resolveBin, rigExecSync, rigRpc, rigSpawn, type RigTarget } from "./rigexec.js";
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

/** The tasks plugin's createProject answer: the project, bare or under `project`. */
function projectIdOf(r: unknown): string {
  const o = r as { id?: string; project?: { id?: string } };
  const id = o.id ?? o.project?.id;
  if (typeof id !== "string") throw new Error(`tasks createProject returned no project id: ${JSON.stringify(r)}`);
  return id;
}

export class Real {
  readonly threads: string[] = [];
  private tmp = mkdtempSync(join(tmpdir(), "autarch-e2e-real-"));
  private project = "";
  /** The tasks project the filer files into (created in setup, through rigRpc). */
  private tasksPrefix = "";
  private tasksProjectId = "";
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
    this.tasksProjectId = projectIdOf(await rigRpc(this.target, "tasks", "createProject", { name: `Autarch ${this.runId}`, prefix: this.tasksPrefix, color: "#6b7280" }));
    this.serve = await startOwnServe({ autarchPath: this.build.autarch_path, dir: join(this.tmp, "serve"), parent: this.parent, home: join(this.tmp, "serve-home") });
    this.bb(["plugin", "install", this.build.plugin_dir, "--yes"]);
    const s = pluginSettings(this.serve, this.build.autarch_path);
    for (const [k, v] of Object.entries(s)) this.bb(["plugin", "config", "autarch", "set", k, Array.isArray(v) ? v.join(",") : String(v)]);
    this.bb(["plugin", "reload", "autarch"]);
    await this.bindToHome(this.tasksProjectId);
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
  /**
   * A tasks project reaches Home only once someone confirms its binding to a Home project (real bb: with serve not
   * suggesting one for a differently named tracker project, every card stays display-only, "project mismatch").
   * The confirm is the product's own setBinding RPC, retried while the plugin's serve project list loads.
   */
  async bindToHome(tasksProjectId: string, home = "Autarch"): Promise<void> {
    let last = "";
    for (let i = 0; i < 40; i++) {
      try {
        const r = await this.rpc<{ ok: boolean; error?: string }>("setBinding", { tasks_project_id: tasksProjectId, state: "confirmed", home_project: home });
        if (r.ok) return;
        last = r.error ?? JSON.stringify(r);
      } catch (e) {
        last = String(e);
      }
      await sleep(500);
    }
    throw new Error(`could not confirm the binding of ${tasksProjectId} to ${home}: ${last}`);
  }
  async waitFor<T>(what: string, fn: () => T | undefined | false, ms = 30_000): Promise<T> {
    const end = Date.now() + ms;
    for (;;) {
      const v = fn();
      if (v) return v;
      if (Date.now() > end) {
        const st = await this.rpc<{ status: unknown }>("queue", {}).then((q) => JSON.stringify(q.status), (e) => `unreadable: ${e}`);
        throw new Error(`timed out after ${ms} ms waiting for ${what} (queue status: ${st})`);
      }
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
      // The ruling is written back to the card (a user-kind "Ruled:" comment); check-e2e's `comment_by` is an id, so it
      // carries that comment's tasks id.
      const card_id = /^card-(.+)-g\d+$/.exec(decision)?.[1];
      assert.ok(card_id, `decision ${decision} is not a card generation`);
      let comment_by: string | undefined;
      for (let i = 0; i < 60 && !comment_by; i++) {
        const { comments } = await rigRpc<{ comments: { id: string; body: string }[] }>(this.target, "tasks", "listComments", { taskId: card_id });
        comment_by = comments.find((c) => c.body.startsWith("Ruled:"))?.id;
        if (!comment_by) await sleep(500);
      }
      assert.ok(comment_by, `no Ruled: comment on card ${card_id}`);
      return {
        threads: [t],
        card_id,
        comment_by,
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
      await this.bindToHome(projectIdOf(await rigRpc(this.target, "tasks", "createProject", { name: `Autarch second ${this.runId}`, prefix: second, color: "#6b7280" })));
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

    // Finding r5-1, on the owned server. v3 is installed and enabled by setup, so the scenario puts a legacy schema-2 database
    // in its place while it is disabled. The "running v2 instance" is the real a9853e2 Store (extracted from git) in a child
    // process that holds data.db open and answers the pick: bb does not host it, because installing a second plugin build
    // under the same id would need a v2 plugin build the rig does not make. That is the plan's holder, one process removed.
    // GAP (UNVERIFIED): because bb does not host the v2 instance, bb's "failed candidate keeps the previous v2 instance running"
    // path (PREVIOUS_INSTANCE_KEPT) is not exercised here. Tracked as bead "real-bb upgrade canary with bb-hosted v2 plugin instance".
    "upgrade-quiesce": async () => {
      const dataDir = join(this.env.data, "plugins", "autarch");
      const dbFile = join(dataDir, "data.db");
      const fileSha = (): string => sha(Buffer.concat(["", "-wal"].map((x) => (existsSync(dbFile + x) ? readFileSync(dbFile + x) : Buffer.alloc(0)))));
      const backups = (): string[] => readdirSync(dataDir).filter((n) => /^home-v2-backup-.*\.db$/.test(n));
      const status = (): string => this.bb(["plugin", "status", "autarch"], { allowFail: true }).stdout;
      const logs = (): string => { const r = this.bb(["plugin", "logs", "autarch"], { allowFail: true }); return r.stdout + r.stderr; };
      const REFUSAL = /home-refused:quiesce-required|another connection holds data\.db/;
      const migrated = (): boolean => { try { return this.db("SELECT 1 FROM migration_log WHERE version >= 3").length > 0; } catch { return false; } };

      // 1. v3 off, its database moved aside, a legacy v2 database made by the v2 build and held open by the v2 instance.
      this.bb(["plugin", "disable", "autarch"]);
      await this.waitFor("v3 to stop (its database closed)", () => !/running/i.test(status()) || undefined, 30_000).catch(() => undefined);
      const aside = join(dataDir, `.e2e-v3-aside-${sha(this.runId).slice(0, 8)}`);
      mkdirSync(aside);
      for (const n of readdirSync(dataDir).filter((n) => /^data\.db/.test(n))) renameSync(join(dataDir, n), join(aside, n));
      const v2dir = join(this.tmp, "v2src");
      mkdirSync(v2dir, { recursive: true });
      const top = rigExecSync("git", ["rev-parse", "--show-toplevel"], { cwd: dirname(fileURLToPath(import.meta.url)) }).stdout.trim();
      for (const f of ["store.ts", "migrations.ts"]) {
        const src = rigExecSync("git", ["-C", top, "show", `a9853e2:integrations/bb-plugin-autarch/${f}`]);
        assert.equal(src.code, 0, `git show a9853e2:${f}: ${src.stderr}`);
        writeFileSync(join(v2dir, f), src.stdout);
      }
      writeFileSync(join(v2dir, "holder.ts"), readFileSync(join(dirname(fileURLToPath(import.meta.url)), "v2-holder.src")));
      symlinkSync(join(dirname(fileURLToPath(import.meta.url)), "..", "node_modules"), join(v2dir, "node_modules"));
      const v2 = rigSpawn(process.execPath, ["--import", "tsx", join(v2dir, "holder.ts"), dbFile]);
      const lines: string[] = [];
      let buf = "";
      v2.stdout!.on("data", (d) => { buf += d; const parts = buf.split("\n"); buf = parts.pop()!; lines.push(...parts); });
      const next = (what: string, ms = 60_000) => this.waitFor(what, () => lines.shift(), ms);
      let v2Closed = false;
      const closeV2 = async () => { if (v2Closed) return; v2Closed = true; v2.stdin!.write("close\n"); await this.waitFor("the v2 instance to close", () => v2.exitCode !== null || undefined, 15_000).catch(() => v2.kill("SIGKILL")); };
      try {
        const ready = await next("the v2 instance to seed and hold data.db");
        assert.match(ready, /^ready /, ready);
        const seeded = JSON.parse(ready.slice(6)) as { schema_version: string | number; decisions: number };
        assert.equal(String(seeded.schema_version), "2");
        assert.equal(seeded.decisions, 2);
        const hasLog = this.db("SELECT 1 FROM sqlite_master WHERE name = 'migration_log'")[0]; // this.db closes its connection
        assert.equal(hasLog, undefined, "the legacy database already has a migration_log");

        // 2. Enable v3 while the holder is open: refused, nothing written.
        const shaBefore = fileSha();
        const enabled = this.bb(["plugin", "enable", "autarch"], { allowFail: true });
        await this.waitFor("the quiesce refusal in the plugin log", () => REFUSAL.test(logs()) || undefined, 60_000);
        const refusedLog = logs().split("\n").filter((l) => REFUSAL.test(l)).slice(-1)[0] ?? "";
        const statusDuring = status().trim().split("\n")[0] ?? "";
        this.bb(["plugin", "disable", "autarch"], { allowFail: true }); // stops the plugin's own retry
        const shaAfter = fileSha();
        const backupFiles = backups().length;
        assert.ok(!migrated(), "the database migrated while the holder was open");

        // 3. The v2 instance still answers: pick a legacy ask through it.
        v2.stdin!.write("pick legacy-1\n");
        const picked = JSON.parse(await next("the v2 pick")) as { ok: boolean };

        // 4. Release the holder, then disable (done) and enable: v3 migrates, the other legacy ask is listed.
        await closeV2();
        this.bb(["plugin", "enable", "autarch"]);
        await this.waitFor("v3 to migrate", () => migrated() || undefined, 60_000).catch((e: Error) => { throw new Error(`${e.message}\n--- plugin status: ${status().trim()}\n--- plugin log tail:\n${logs().split("\n").slice(-25).join("\n")}`); });
        let legacyListed = false;
        let listErr = "";
        let owedIds: string[] = [];
        for (let i = 0; i < 60 && !legacyListed; i++) {
          try {
            const l = await this.rpc<{ owed: { id: string }[] }>("listAsks");
            owedIds = l.owed.map((o) => o.id);
            legacyListed = owedIds.includes("legacy-2");
          } catch (e) { listErr = String(e); /* the plugin may still be starting */ }
          if (!legacyListed) await sleep(500);
        }
        const row = this.db<{ version: number; backup_path: string | null }>("SELECT version, backup_path FROM migration_log WHERE version >= 3")[0];
        const pickRows = this.db("SELECT 1 FROM picks WHERE decision_id = 'legacy-1'").length;
        assert.ok(row?.backup_path && existsSync(row.backup_path), "the migration wrote no backup file");
        assert.ok(legacyListed, `legacy-2 is not listed after the migration (owed: ${JSON.stringify(owedIds)}; last listAsks error: ${listErr})`);
        assert.equal(pickRows, 1, "the pick made through the v2 instance did not survive the migration");
        return {
          quiesce_refused: REFUSAL.test(refusedLog),
          db_sha256_unchanged: shaBefore === shaAfter,
          backup_files: backupFiles,
          legacy_ask_picked: picked.ok === true,
          migrated_after_enable: true,
          // UNVERIFIED, stated plainly: this proves quiesce refusal, backup and migration against a standalone v2 Store process.
          // It does NOT prove bb's activation-failure-keeps-the-previous-v2-plugin-instance-running behaviour.
          v2_instance_hosted_by: "standalone-store-process",
          unverified_gap: "bb-hosted-v2-instance-keeps-running-on-failed-candidate",
          threads: [] as string[], // no thread is created here; the acceptance jq reads evidence.threads from every record
          enable_exit: enabled.code,
          status_during_refusal: statusDuring,
          refusal_log: refusedLog,
          backup_after_migrate: row.backup_path,
          legacy_listed_after_migrate: legacyListed,
          v2_pick_survived: pickRows === 1,
        };
      } finally {
        await closeV2().catch(() => undefined);
        v2.kill("SIGKILL");
      }
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
