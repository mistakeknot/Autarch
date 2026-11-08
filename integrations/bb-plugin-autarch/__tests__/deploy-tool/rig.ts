// Shared rig for the deploy-tool tests. Everything is a temp directory: a fixture upstream git repo that carries a copy of
// the tool, a fake bb (a stub script with per-instance state), a fake bb data directory with a live "instance" process and
// an installed build the tool runs from. Nothing here touches a live bb, a live updater or an installed plugin.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync, chmodSync, appendFileSync, renameSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import Database from "better-sqlite3";

export const REPO_ROOT = resolve(__dirname, "../../../..");
export const TOOL_SRC = join(REPO_ROOT, "tools", "bb-plugin-deploy");
export const PLUGIN_ID = "fixture";
export const PLUGIN_DIR = "plug";

export type Json = Record<string, unknown>;
export interface Call { args: string[]; url: string | null; hostPort: string | null }
export interface CliResult { status: number | null; stdout: string; stderr: string; json: Json | null }

/** The fake bb. CommonJS on purpose: it lives in a temp dir with no package.json. State is per BB_SERVER_URL. */
const BB_STUB = String.raw`#!${process.execPath}
const fs = require("fs");
const path = require("path");
const url = process.env.BB_SERVER_URL || null;
const args = process.argv.slice(2);
let reg = {};
try { reg = JSON.parse(fs.readFileSync(path.join(__dirname, "instances.json"), "utf8")); } catch (e) { reg = {}; }
const st = url && reg[url];
if (!st) {
  fs.appendFileSync(path.join(__dirname, "stray.jsonl"), JSON.stringify({ args, url }) + "\n");
  console.error("fake bb: no instance for " + url);
  process.exit(1);
}
const sp = path.join(st, "state.json");
const S = JSON.parse(fs.readFileSync(sp, "utf8"));
const save = () => fs.writeFileSync(sp, JSON.stringify(S));
fs.appendFileSync(path.join(st, "calls.jsonl"), JSON.stringify({ args, url, hostPort: process.env.BB_HOST_DAEMON_PORT === undefined ? null : process.env.BB_HOST_DAEMON_PORT }) + "\n");
let inj = {};
const ip = path.join(st, "inject.json");
try { inj = JSON.parse(fs.readFileSync(ip, "utf8")); } catch (e) { inj = {}; }
const fail = (m) => { console.error(m); process.exit(1); };
const appendOn = (point) => { for (const a of (inj.appendOn && inj.appendOn[point]) || []) fs.appendFileSync(a.file, a.text); };
const running = () => S.enabled && !(inj.neverRunMatch && S.source && S.source.includes(inj.neverRunMatch));
const [a0, a1, a2, a3, a4] = args;
if (a0 === "home" && a1 === "moves") {
  if (inj.fail && inj.fail.moves) fail("moves failed");
  console.log(JSON.stringify({ moves: [] }));
  process.exit(0);
}
if (a0 !== "plugin") fail("fake bb: unsupported " + args.join(" "));
if (inj.fail && inj.fail[a1]) fail("injected failure for plugin " + a1);
const pause = () => {
  if (inj.pauseAfter !== a1) return;
  delete inj.pauseAfter;
  fs.writeFileSync(ip, JSON.stringify(inj));
  fs.writeFileSync(path.join(st, "paused"), String(process.pid));
  while (!fs.existsSync(path.join(st, "release"))) Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 25);
};
if (a1 === "source") {
  if (a2 !== S.pluginId) fail("unknown plugin " + a2);
  if (!S.source) fail("no source");
  if (args.includes("--json")) console.log(JSON.stringify({ requested: "path:" + S.source, resolved: "path:" + S.source, engines: { bb: ">=0.0.0" }, installedAt: "2026-01-01T00:00:00.000Z", history: [] }));
  else console.log(S.pluginId + "\n  requested: path:" + S.source + "\n  resolved: path:" + S.source + "\n  engines.bb: >=0.0.0");
} else if (a1 === "list") {
  const status = running() ? "running" : "stopped";
  if (args.includes("--json")) console.log(JSON.stringify({ plugins: [{ id: S.pluginId, version: "0.1.0", enabled: S.enabled, status, source: "path:" + S.source, rootDir: S.source }] }));
  else console.log(S.pluginId + "@0.1.0  " + status + "\n    source: path:" + S.source);
} else if (a1 === "disable") {
  S.enabled = false; save(); pause();
} else if (a1 === "enable") {
  S.enabled = true; save(); appendOn("enable"); pause();
} else if (a1 === "install") {
  const dir = args[args.length - 1];
  if (inj.failInstallMatch && dir.includes(inj.failInstallMatch)) fail("injected install failure for " + dir);
  S.source = dir; save(); appendOn("install"); pause();
} else if (a1 === "dev") {
  appendOn("dev");
} else if (a1 === "config") {
  if (a3 === "--json") console.log(JSON.stringify({ values: S.config }));
  else if (a3 === "set") { S.config[a4] = args[5]; save(); appendOn("config"); appendOn("config:" + a4); pause(); }
  else fail("fake bb: unsupported config action");
} else fail("fake bb: unsupported plugin command " + a1);
`;

/** The fixture's fake gates script: its answer for a phase is the content of <stateDir>/gate-<phase> (default ok). */
const GATES_SH = String.raw`#!/usr/bin/env bash
phase="$1"; st="$2"; mode=ok
[ -f "$st/gate-$phase" ] && mode=$(cat "$st/gate-$phase")
env | grep '^BPD_MARKER_' | sort >> "$st/marker-env.log" || true
case "$mode" in
  ok) printf '{"gate":"%s","ok":true,"reason":""}\n' "$phase"; exit 0;;
  refuse) printf '{"gate":"%s","ok":false,"reason":"refused by test"}\n' "$phase"; exit 10;;
  needs) printf '{"gate":"%s","ok":false,"reason":"needs mk by test"}\n' "$phase"; exit 20;;
  *) echo boom >&2; exit 1;;
esac
`;

const BUILD_JS = `import { writeFileSync } from "node:fs";\nwriteFileSync("built.txt", "built\\n");\n`;

export const gateEntry = (name: string, phase: string, marker?: string): Json => ({ name, run: ["bash", "{installedRoot}/scripts/home-gates.sh", phase, "{stateDir}"], ...(marker ? { marker } : {}) });

export function defaultPlugin(upstream: string): Json {
  const gate = gateEntry;
  return {
    format: "bb-plugin-deploy/plugin@1",
    pluginId: PLUGIN_ID,
    repo: upstream,
    branch: "main",
    pluginDir: PLUGIN_DIR,
    build: [["node", "build.mjs"]],
    data: [{ name: "data.db", kind: "sqlite", path: `{bbData}/plugins/${PLUGIN_ID}/data.db` }],
    export: [{ name: "moves", run: ["bb", "home", "moves", "--json"], required: true }],
    preflight: [gate("gates", "preflight")],
    preInstall: [gate("schema-gate", "preInstall", "schema")],
    preEnable: [gate("pre-enable", "preEnable")],
    postEnablePreConfig: [gate("post-enable", "postEnable")],
    configAfterEnable: [{ key: "feature", value: "on", requiresMarker: "schema" }],
    health: [{ kind: "running", timeoutSec: 3 }],
    classes: ["plugin-deploy"],
    controlPaths: ["tools/bb-plugin-deploy/**", "scripts/home-gates.sh", `${PLUGIN_DIR}/deploy.json`, "**/AGENTS.md"],
    retention: { builds: 5 },
    updateRequest: { kind: "settings-kv", db: `{bbData}/plugins/${PLUGIN_ID}/data.db`, key: "update_request" },
  };
}

const GIT_ENV = { GIT_AUTHOR_NAME: "t", GIT_AUTHOR_EMAIL: "t@example.invalid", GIT_COMMITTER_NAME: "t", GIT_COMMITTER_EMAIL: "t@example.invalid", GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1" };

export function git(cwd: string, args: string[]): string {
  const r = spawnSync("git", args, { cwd, encoding: "utf8", env: { PATH: process.env.PATH ?? "", HOME: "/nonexistent", ...GIT_ENV } });
  if (r.status !== 0) throw new Error(`git ${args.join(" ")} failed: ${r.stderr}`);
  return r.stdout.trim();
}

export interface Instance { name: string; dir: string; serverUrl: string; stateDir: string; entry: string; proc: ChildProcess; bbData: string }

export interface RigOptions {
  /** Merged over defaultPlugin(). Applied to the committed plugin file (so it is part of the installed build). */
  plugin?: Json;
  /** Skip creating the installed build (a fresh host with nothing installed). */
  noInstalled?: boolean;
  /** "git": a clean checkout of W with no stamp (legacy); "stamped": a tool build. */
  installedKind?: "git" | "stamped";
  /** Extra files committed in the first (W) commit. */
  files?: Record<string, string>;
}

export class Rig {
  readonly root: string;
  readonly upstream: string;
  readonly bbData: string;
  readonly stateDir: string;
  readonly buildsRoot: string;
  readonly backupsRoot: string;
  readonly stubDir: string;
  readonly stub: string;
  readonly instances: Instance[] = [];
  W = "";
  private readonly extraProcs: ChildProcess[] = [];

  constructor(readonly opts: RigOptions = {}) {
    this.root = mkdtempSync(join(tmpdir(), "bpd-"));
    this.upstream = join(this.root, "upstream");
    this.bbData = join(this.root, "bbdata");
    this.stateDir = join(this.root, "state");
    this.buildsRoot = join(this.stateDir, "builds");
    this.backupsRoot = join(this.bbData, "home-backups");
    this.stubDir = join(this.root, "stub");
    this.stub = join(this.stubDir, "bb");
    mkdirSync(this.stubDir, { recursive: true });
    mkdirSync(this.stateDir, { recursive: true });
    writeFileSync(this.stub, BB_STUB);
    chmodSync(this.stub, 0o755);
    this.initUpstream();
    this.addInstance("a", this.bbData);
    this.seedData(this.bbData);
    if (!opts.noInstalled) this.install(this.makeBuild(this.W, opts.installedKind ?? "git"));
  }

  // ---- upstream repo ---------------------------------------------------------------------------------------------
  private initUpstream(): void {
    mkdirSync(this.upstream, { recursive: true });
    git(this.upstream, ["init", "-q", "-b", "main"]);
    cpSync(TOOL_SRC, join(this.upstream, "tools", "bb-plugin-deploy"), { recursive: true, filter: (p) => !p.includes("node_modules") });
    const plug = join(this.upstream, PLUGIN_DIR);
    mkdirSync(plug, { recursive: true });
    mkdirSync(join(this.upstream, "scripts"), { recursive: true });
    writeFileSync(join(plug, "build.mjs"), BUILD_JS);
    writeFileSync(join(plug, "package.json"), JSON.stringify({ name: "fixture", private: true, type: "module" }));
    writeFileSync(join(plug, "app.txt"), "v1\n");
    writeFileSync(join(this.upstream, ".gitignore"), `${PLUGIN_DIR}/built.txt\nnode_modules/\n`);
    writeFileSync(join(this.upstream, "scripts", "home-gates.sh"), GATES_SH);
    chmodSync(join(this.upstream, "scripts", "home-gates.sh"), 0o755);
    const plugin = { ...defaultPlugin(this.upstream), ...(this.opts.plugin ?? {}) };
    writeFileSync(join(plug, "deploy.json"), JSON.stringify(plugin, null, 2));
    for (const [rel, text] of Object.entries(this.opts.files ?? {})) {
      mkdirSync(resolve(this.upstream, rel, ".."), { recursive: true });
      writeFileSync(join(this.upstream, rel), text);
    }
    git(this.upstream, ["add", "-A"]);
    git(this.upstream, ["commit", "-q", "-m", "W"]);
    this.W = git(this.upstream, ["rev-parse", "HEAD"]);
  }

  /** Commit files (path -> content; null deletes) on main of the upstream and return the new sha. */
  commit(files: Record<string, string | null>, msg = "change"): string {
    for (const [rel, text] of Object.entries(files)) {
      const p = join(this.upstream, rel);
      if (text === null) rmSync(p, { force: true });
      else {
        mkdirSync(resolve(p, ".."), { recursive: true });
        writeFileSync(p, text);
      }
    }
    git(this.upstream, ["add", "-A"]);
    git(this.upstream, ["commit", "-q", "-m", msg, "--allow-empty"]);
    return git(this.upstream, ["rev-parse", "HEAD"]);
  }
  tree(sha: string): string {
    return git(this.upstream, ["rev-parse", `${sha}^{tree}`]);
  }

  // ---- builds ---------------------------------------------------------------------------------------------------
  /** A clone of the upstream at `sha`. "stamped" also writes the tool's stamp. Returns the build root. */
  private buildSeq = 0;
  makeBuild(sha: string, kind: "git" | "stamped" = "stamped", name?: string): string {
    const dir = join(this.buildsRoot, PLUGIN_ID, name ?? `${sha.slice(0, 12)}-20200101T00${String(this.buildSeq++).padStart(4, "0")}Z`);
    mkdirSync(resolve(dir, ".."), { recursive: true });
    git(this.root, ["clone", "-q", "--no-hardlinks", this.upstream, dir]);
    git(dir, ["checkout", "-q", "--detach", sha]);
    if (kind === "stamped") {
      mkdirSync(join(dir, ".bb-plugin-deploy"), { recursive: true });
      writeFileSync(join(dir, ".bb-plugin-deploy", "stamp.json"), JSON.stringify({ format: "bb-plugin-deploy/stamp@1", pluginId: PLUGIN_ID, commit: sha, tree: this.tree(sha), builtAt: "2026-01-01T00:00:00.000Z", toolCommit: sha, buildCommands: [["node", "build.mjs"]], lockSha256: null }, null, 2));
    }
    return dir;
  }
  /** Make bb (instance `a` unless named) report `buildRoot/plug` as its source and enable it. */
  install(buildRoot: string, inst: Instance = this.instances[0]): void {
    const s = this.readState(inst);
    s.source = join(buildRoot, PLUGIN_DIR);
    s.enabled = true;
    this.writeState(inst, s);
  }
  /** The build root bb currently reports for instance a (null when nothing is installed). */
  liveRoot(inst: Instance = this.instances[0]): string | null {
    const src = this.readState(inst).source;
    return typeof src === "string" ? src.slice(0, -(PLUGIN_DIR.length + 1)) : null;
  }
  buildDirs(): string[] {
    const r = spawnSync("ls", ["-1", join(this.buildsRoot, PLUGIN_ID)], { encoding: "utf8" });
    return r.status === 0 ? r.stdout.split("\n").filter(Boolean).map((n) => join(this.buildsRoot, PLUGIN_ID, n)) : [];
  }

  // ---- fake bb instances -----------------------------------------------------------------------------------------
  private readState(inst: Instance): Record<string, unknown> {
    return JSON.parse(readFileSync(join(inst.stateDir, "state.json"), "utf8")) as Record<string, unknown>;
  }
  private writeState(inst: Instance, s: Record<string, unknown>): void {
    writeFileSync(join(inst.stateDir, "state.json"), JSON.stringify(s));
  }
  private register(): void {
    const reg: Record<string, string> = {};
    for (const i of this.instances) reg[i.serverUrl] = i.stateDir;
    writeFileSync(join(this.stubDir, "instances.json"), JSON.stringify(reg));
  }

  /** A second (or first) live fake instance: a real process whose argv names `entry`, a runtime file and a fake bb state. */
  addInstance(name: string, bbData: string, port = 40000 + this.instances.length): Instance {
    const dir = join(this.root, `inst-${name}`);
    mkdirSync(dir, { recursive: true });
    mkdirSync(join(bbData, "plugins", PLUGIN_ID), { recursive: true });
    const entry = join(dir, "bb-app-entry.cjs");
    writeFileSync(entry, "setTimeout(() => {}, 600000);\n");
    const proc = spawn(process.execPath, [entry], { stdio: "ignore" });
    const serverUrl = `http://127.0.0.1:${port}`;
    const inst: Instance = { name, dir, serverUrl, stateDir: join(dir, "bbstate"), entry, proc, bbData };
    mkdirSync(inst.stateDir, { recursive: true });
    this.writeState(inst, { pluginId: PLUGIN_ID, source: null, enabled: false, config: {} });
    writeFileSync(join(bbData, "bb-app-runtime.json"), JSON.stringify({ entryPath: entry, pid: proc.pid, serverUrl, startedAt: "2026-01-01T00:00:00.000Z", surface: "test", version: "0.0.0" }));
    this.instances.push(inst);
    this.register();
    return inst;
  }
  /** Rewrite instance a's runtime file (for missing-serverUrl tests). */
  rewriteRuntime(inst: Instance, patch: Record<string, unknown>): void {
    const p = join(inst.bbData, "bb-app-runtime.json");
    const cur = JSON.parse(readFileSync(p, "utf8")) as Record<string, unknown>;
    const next = { ...cur, ...patch };
    for (const [k, v] of Object.entries(next)) if (v === undefined) delete next[k];
    writeFileSync(p, JSON.stringify(next));
  }

  calls(inst: Instance = this.instances[0]): Call[] {
    const p = join(inst.stateDir, "calls.jsonl");
    return existsSync(p) ? readFileSync(p, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l) as Call) : [];
  }
  /** Calls that change bb state, as "disable"/"install"/"enable"/"config" words. */
  mutations(inst: Instance = this.instances[0]): string[] {
    return this.calls(inst)
      .filter((c) => c.args[0] === "plugin" && (["disable", "enable", "install"].includes(c.args[1] ?? "") || (c.args[1] === "config" && c.args[3] === "set")))
      .map((c) => (c.args[1] === "config" ? "config" : (c.args[1] ?? "")));
  }
  clearCalls(inst: Instance = this.instances[0]): void {
    rmSync(join(inst.stateDir, "calls.jsonl"), { force: true });
  }
  inject(patch: Json, inst: Instance = this.instances[0]): void {
    const p = join(inst.stateDir, "inject.json");
    const cur = existsSync(p) ? (JSON.parse(readFileSync(p, "utf8")) as Json) : {};
    writeFileSync(p, JSON.stringify({ ...cur, ...patch }));
  }
  state(inst: Instance = this.instances[0]): Record<string, unknown> {
    return this.readState(inst);
  }
  /** Where the fake bb currently runs from, as a plugin dir. */
  source(inst: Instance = this.instances[0]): string | null {
    const s = this.readState(inst).source;
    return typeof s === "string" ? s : null;
  }

  // ---- data and state --------------------------------------------------------------------------------------------
  seedData(bbData: string, schema = 3): string {
    const p = join(bbData, "plugins", PLUGIN_ID, "data.db");
    mkdirSync(join(bbData, "plugins", PLUGIN_ID), { recursive: true });
    const db = new Database(p);
    db.exec("CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE IF NOT EXISTS t (x TEXT); CREATE TABLE IF NOT EXISTS settings_kv (key TEXT PRIMARY KEY, value TEXT)");
    db.prepare("INSERT OR REPLACE INTO schema_meta VALUES ('schema_version', ?)").run(String(schema));
    db.prepare("INSERT INTO t VALUES ('before')").run();
    db.close();
    return p;
  }
  get dataDb(): string {
    return join(this.bbData, "plugins", PLUGIN_ID, "data.db");
  }
  setRequest(sha: string, clickedAt: string): void {
    const db = new Database(this.dataDb);
    db.prepare("INSERT OR REPLACE INTO settings_kv VALUES ('update_request', ?)").run(JSON.stringify({ sha, clicked_at: clickedAt }));
    db.close();
  }
  /** The raw pending click row, as the plugin stores it (undefined when none). */
  requestRaw(): string | undefined {
    const db = new Database(this.dataDb, { readonly: true });
    try {
      const row = db.prepare("SELECT value FROM settings_kv WHERE key='update_request'").get() as { value: string } | undefined;
      return row?.value;
    } finally {
      db.close();
    }
  }
  /** Make the upstream repository unreachable (renamed away). */
  breakUpstream(): void {
    renameSync(this.upstream, `${this.upstream}-gone`);
  }
  addDecision(id: string, body: string, option: string, by: string): void {
    const db = new Database(this.dataDb);
    db.exec("CREATE TABLE IF NOT EXISTS decisions (id TEXT PRIMARY KEY, body_json TEXT, withdrawn_at TEXT); CREATE TABLE IF NOT EXISTS picks (decision_id TEXT, option_id TEXT, \"by\" TEXT)");
    db.prepare("INSERT INTO decisions VALUES (?, ?, NULL)").run(id, body);
    db.prepare("INSERT INTO picks VALUES (?, ?, ?)").run(id, option, by);
    db.close();
  }
  /** Make the fake gates script answer `mode` ("ok" | "refuse" | "needs" | "fail") for a phase. */
  gate(phase: string, mode: string): void {
    writeFileSync(join(this.stateDir, `gate-${phase}`), mode);
  }
  approve(sha: string): void {
    mkdirSync(join(this.stateDir, "review-evidence"), { recursive: true });
    writeFileSync(join(this.stateDir, "review-evidence", `${sha}.json`), JSON.stringify({ format: "bb-plugin-deploy/review-evidence@1", commit: sha, verdict: "approve" }));
  }
  readJson(rel: string): Json | null {
    const p = join(this.stateDir, rel);
    return existsSync(p) ? (JSON.parse(readFileSync(p, "utf8")) as Json) : null;
  }
  record(): Json {
    return this.readJson("record.json") as Json;
  }
  installedSha(): string | null {
    const p = join(this.stateDir, "installed.sha");
    return existsSync(p) ? readFileSync(p, "utf8").trim() : null;
  }
  appendLog(file: string, text: string): void {
    appendFileSync(file, text);
  }

  // ---- running the tool ------------------------------------------------------------------------------------------
  env(extra: Record<string, string | undefined> = {}): Record<string, string> {
    const base: Record<string, string | undefined> = { PATH: process.env.PATH, HOME: this.root, BB_PLUGIN_DEPLOY_BB: this.stub, BPD_POLL_MS: "20", BPD_LOCK_WAIT_SEC: "5", ...extra };
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(base)) if (v !== undefined) out[k] = v;
    return out;
  }
  /** Args to point the tool at this rig. */
  common(inst: Instance = this.instances[0], root?: string): string[] {
    const r = root ?? this.liveRoot(inst) ?? "";
    return ["--plugin-file", join(r, PLUGIN_DIR, "deploy.json"), "--bb-data", inst.bbData, "--state-dir", this.stateDir, "--json"];
  }
  /** Run the tool from `engineRoot` (default: the build bb currently runs). */
  cli(args: string[], o: { engineRoot?: string; env?: Record<string, string | undefined>; inst?: Instance; pluginRoot?: string; noCommon?: boolean; stdio?: "pipe" } = {}): CliResult {
    const inst = o.inst ?? this.instances[0];
    const eng = o.engineRoot ?? this.liveRoot(inst);
    if (!eng) throw new Error("no engine root: nothing installed");
    const argv = [join(eng, "tools", "bb-plugin-deploy", "cli.mjs"), ...args, ...(o.noCommon ? ["--json"] : this.common(inst, o.pluginRoot ?? eng))];
    const r = spawnSync(process.execPath, argv, { encoding: "utf8", env: this.env(o.env), timeout: 120_000 });
    return { status: r.status, stdout: r.stdout, stderr: r.stderr, json: parse(r.stdout) };
  }
  /** Spawn (not wait) the tool, for the lock tests. */
  spawnCli(args: string[], o: { engineRoot?: string; env?: Record<string, string | undefined>; inst?: Instance } = {}): ChildProcess {
    const inst = o.inst ?? this.instances[0];
    const eng = o.engineRoot ?? this.liveRoot(inst);
    if (!eng) throw new Error("no engine root: nothing installed");
    const p = spawn(process.execPath, [join(eng, "tools", "bb-plugin-deploy", "cli.mjs"), ...args, ...this.common(inst, eng)], { env: this.env(o.env), stdio: ["ignore", "pipe", "pipe"] });
    this.extraProcs.push(p);
    return p;
  }

  cleanup(): void {
    for (const p of this.extraProcs) p.kill("SIGKILL");
    for (const i of this.instances) i.proc.kill("SIGKILL");
    rmSync(this.root, { recursive: true, force: true });
  }
}

export function parse(text: string): Json | null {
  try {
    const v: unknown = JSON.parse(text);
    return v !== null && typeof v === "object" && !Array.isArray(v) ? (v as Json) : null;
  } catch {
    return null;
  }
}

/** Wait (blocking, bounded) until `pred` is true. */
export function waitFor(pred: () => boolean, ms = 10_000, step = 25): boolean {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (pred()) return true;
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, step);
  }
  return pred();
}
