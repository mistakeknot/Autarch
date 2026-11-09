// scripts/install-home-your-move.sh --go and --rollback when the plugin is not running (mk-hd2ks) or when `bb plugin install` fails on
// the client after the server did the work (mk-fg2io). A copy of the script runs inside a throwaway git repo against a stub bb whose
// state lives in files. Never live: no real bb, data directory, state directory or plugin is touched.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, utimesSync, writeFileSync, copyFileSync } from "node:fs";
import { tmpdir, userInfo } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const SCRIPT = resolve(__dirname, "../../../scripts/install-home-your-move.sh");
const isMk = userInfo().username === "mk";
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

const git = (cwd: string, ...args: string[]) =>
  spawnSync("git", ["-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.invalid", ...args], { cwd, encoding: "utf8" });

// The real moves DDL (migrations.ts), so the read-only export is tested against the real column set.
const MOVES_DDL = `
CREATE TABLE moves (
  task_id TEXT NOT NULL, generation INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('script','pr','read','context')),
  payload_json TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open','claimed','closed')),
  opened_by TEXT NOT NULL CHECK (opened_by IN ('card','pick')),
  opened_at TEXT NOT NULL, claimed_at TEXT, claimed_by TEXT, closed_at TEXT, closed_by TEXT, evidence TEXT,
  report_state TEXT CHECK (report_state IS NULL OR report_state IN ('succeeded','failed','no-report')),
  report_json TEXT, report_at TEXT, report_deadline_at TEXT, skipped_at TEXT, hidden_by TEXT, hidden_at TEXT, last_checked_at TEXT, last_error TEXT,
  PRIMARY KEY (task_id, generation)
);
CREATE INDEX moves_state ON moves(state, opened_at);`;

type InstallMode = "ok" | "reset-after-enable" | "reset-nothing" | "reset-stale-dist" | "reset-other-source";

interface RigOpts { schema?: number | null; moves?: boolean; plugin?: "running" | "disabled"; install?: InstallMode }

function rig(opts: RigOpts = {}) {
  const root = mkdtempSync(join(tmpdir(), "ihfp-"));
  dirs.push(root);
  // A throwaway checkout holding a copy of the script: REPO is derived from the script's own location.
  const repo = join(root, "repo");
  const plugin = join(repo, "integrations", "bb-plugin-autarch");
  mkdirSync(join(repo, "scripts"), { recursive: true });
  mkdirSync(join(plugin, "dist"), { recursive: true });
  mkdirSync(join(plugin, "node_modules"));
  copyFileSync(SCRIPT, join(repo, "scripts", "install-home-your-move.sh"));
  writeFileSync(join(plugin, "migrations.ts"), "export const CODE_VERSION = 4;\n");
  writeFileSync(join(plugin, "dist", "app.js"), "old build\n");
  const hourAgo = new Date(Date.now() - 3_600_000);
  utimesSync(join(plugin, "dist", "app.js"), hourAgo, hourAgo);
  writeFileSync(join(repo, ".gitignore"), "dist/\nnode_modules/\n");
  git(repo, "init", "-q");
  git(repo, "add", "-A");
  git(repo, "commit", "-q", "-m", "x");
  const head = git(repo, "rev-parse", "HEAD").stdout.trim();
  // bb's data directory and the plugin's data.db.
  const data = join(root, "bbdata");
  const pdata = join(data, "plugins", "autarch");
  mkdirSync(pdata, { recursive: true });
  const schema = opts.schema === undefined ? 4 : opts.schema;
  const db = new Database(join(pdata, "data.db"));
  db.exec("CREATE TABLE migration_log (version INTEGER)");
  if (schema !== null) {
    db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT)");
    db.prepare("INSERT INTO schema_meta VALUES ('schema_version', ?)").run(String(schema));
    db.prepare("INSERT INTO migration_log VALUES (?)").run(schema);
  }
  if (opts.moves !== false) {
    db.exec(MOVES_DDL);
    const ins = db.prepare(
      "INSERT INTO moves (task_id, generation, kind, payload_json, state, opened_by, opened_at, claimed_at, claimed_by, evidence) VALUES (?,?,?,?,?,?,?,?,?,?)",
    );
    ins.run("TASK-1", 1, "script", JSON.stringify({ path: "/x/y.sh", note: 'quote " and unicode é' }), "open", "card", "2026-11-01T00:00:00Z", null, null, null);
    ins.run("TASK-2", 1, "pr", JSON.stringify({ url: "https://example.invalid/pr/2" }), "claimed", "pick", "2026-11-02T00:00:00Z", "2026-11-02T01:00:00Z", "mk", "said so");
    ins.run("TASK-2", 2, "read", "{}", "closed", "card", "2026-11-03T00:00:00Z", null, null, null);
  }
  db.close();
  const st = join(root, "state");
  mkdirSync(st);
  // The stub bb: its whole state is in files, so each call sees what the previous one did.
  const ctl = join(root, "ctl");
  mkdirSync(ctl);
  writeFileSync(join(ctl, "state"), opts.plugin === "disabled" ? "false disabled\n" : "true running\n");
  writeFileSync(join(ctl, "source"), plugin + "\n");
  writeFileSync(join(ctl, "install-mode"), (opts.install ?? "ok") + "\n");
  const calls = join(root, "calls");
  const bb = join(root, "bb");
  writeFileSync(
    bb,
    `#!/bin/sh
C=${ctl}
echo "$@" >> ${calls}
read EN STATUS < $C/state
case "$1 $2" in
  "plugin list")
    printf '{"plugins":[{"id":"autarch","enabled":%s,"status":"%s","source":"path:%s","rootDir":"%s"}]}\\n' "$EN" "$STATUS" "$(cat $C/source)" "$(cat $C/source)" ;;
  "home moves")
    if [ "$EN $STATUS" = "true running" ]; then echo '{"moves":[{"task_id":"FROM-BB"}],"groups":{}}'
    else echo "error: unknown command 'home'" >&2; exit 1; fi ;;
  "plugin disable") echo "false disabled" > $C/state ;;
  "plugin enable") echo "true running" > $C/state ;;
  "plugin install")
    echo "Installing bb-plugin-autarch@0.1.0"
    case "$(cat $C/install-mode)" in
      ok) touch ${join(plugin, "dist", "app.js")} ;;
      reset-after-enable) sleep 0.2; touch ${join(plugin, "dist", "app.js")}; echo "true running" > $C/state; echo "Error: fetch failed: read ECONNRESET" >&2; exit 1 ;;
      reset-nothing) echo "Error: fetch failed: read ECONNRESET" >&2; exit 1 ;;
      reset-stale-dist) echo "true running" > $C/state; echo "Error: fetch failed: read ECONNRESET" >&2; exit 1 ;;
      reset-other-source) sleep 0.2; touch ${join(plugin, "dist", "app.js")}; echo "true running" > $C/state; echo "/elsewhere/plugin" > $C/source; echo "Error: fetch failed: read ECONNRESET" >&2; exit 1 ;;
    esac ;;
  *) exit 0 ;;
esac
`,
  );
  chmodSync(bb, 0o755);
  const out = join(root, "out");
  mkdirSync(out);
  const prev = join(root, "prev-plugin");
  mkdirSync(prev);
  const env = (extra: Record<string, string> = {}): Record<string, string> => ({
    PATH: process.env.PATH!, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_STATE_DIR: st, HOME_YOUR_MOVE_OUT_DIR: out,
    HOME_UI_LOCK_WAIT: "1", HOME_YOUR_MOVE_RECHECK_WAIT: "3", HOME_YOUR_MOVE_RECHECK_INTERVAL: "1", ...extra,
  });
  const inst = (args: string[], extra: Record<string, string> = {}) =>
    spawnSync("bash", [join(repo, "scripts", "install-home-your-move.sh"), ...args], { env: env(extra), encoding: "utf8" });
  const calledList = () => (existsSync(calls) ? readFileSync(calls, "utf8").split("\n").filter(Boolean) : []);
  const outDir = () => { const s = readdirSync(out)[0]; return s ? join(out, s) : ""; };
  const exportJson = () => { try { return JSON.parse(readFileSync(join(outDir(), "moves-before.json"), "utf8")); } catch { return null; } };
  const dbMoves = () => {
    const d = new Database(join(pdata, "data.db"), { readonly: true });
    try { return d.prepare("SELECT * FROM moves ORDER BY opened_at DESC, task_id, generation").all(); } finally { d.close(); }
  };
  return { root, repo, head, plugin, prev, ctl, inst, calledList, outDir, exportJson, dbMoves, state: () => readFileSync(join(ctl, "state"), "utf8").trim() };
}

const changedAnything = (calls: string[]) => calls.some((c) => /^plugin (disable|install|enable)\b/.test(c));

describe.skipIf(!isMk)("mk-hd2ks: --rollback and --go when the plugin is not running", () => {
  it("--rollback with the plugin disabled reads the moves from data.db (read only) and then restores the previous build", { timeout: 90_000 }, () => {
    const r = rig({ plugin: "disabled" });
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status, o.stdout + o.stderr).toBe(0);
    const exp = r.exportJson();
    expect(exp.moves).toEqual(r.dbMoves());
    expect(exp.moves).toHaveLength(3);
    expect(o.stdout).toContain("from data.db read only");
    const calls = r.calledList();
    expect(calls).toContain("plugin install --yes " + r.prev);
    expect(r.state()).toBe("true running");
  });

  it("--rollback with the plugin disabled and no readable moves table stays fatal and changes nothing", () => {
    const r = rig({ plugin: "disabled", moves: false });
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status).toBe(3);
    expect(o.stdout).toContain("ABORT");
    expect(changedAnything(r.calledList())).toBe(false);
  });

  it("--rollback with an unreadable schema (no schema_meta) and the plugin disabled stays fatal", () => {
    const r = rig({ plugin: "disabled", schema: null, moves: false });
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status).toBe(3);
    expect(changedAnything(r.calledList())).toBe(false);
  });

  it("--rollback with a RUNNING plugin whose export fails stays fatal: the data.db fallback is only for a plugin that is not running", () => {
    const r = rig({ plugin: "running" });
    writeFileSync(join(r.ctl, "state"), "true running\n");
    // make `bb home moves` fail while the plugin reads as running
    const bb = join(r.root, "bb");
    writeFileSync(bb, readFileSync(bb, "utf8").replace(`if [ "$EN $STATUS" = "true running" ]; then echo`, `if false; then echo`));
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status).toBe(3);
    expect(changedAnything(r.calledList())).toBe(false);
  });

  it("--rollback with a running plugin still takes the export through bb (unchanged)", { timeout: 90_000 }, () => {
    const r = rig({ plugin: "running" });
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status, o.stdout + o.stderr).toBe(0);
    expect(r.exportJson().moves).toEqual([{ task_id: "FROM-BB" }]);
    expect(r.calledList()).toContain("home moves --json");
  });

  it("--go with the plugin disabled (left so by an earlier failure) exports from data.db and installs", { timeout: 90_000 }, () => {
    const r = rig({ plugin: "disabled" });
    const o = r.inst(["--go", "--commit", r.head]);
    expect(o.status, o.stdout + o.stderr).toBe(0);
    expect(r.exportJson().moves).toEqual(r.dbMoves());
    expect(r.state()).toBe("true running");
  });
  it("--rollback with the plugin disabled and an EMPTY moves table writes an empty moves array (a counted zero, not a read failure)", { timeout: 90_000 }, () => {
    const r = rig({ plugin: "disabled" });
    const d = new Database(join(r.root, "bbdata", "plugins", "autarch", "data.db"));
    d.exec("DELETE FROM moves");
    d.close();
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status, o.stdout + o.stderr).toBe(0);
    expect(r.exportJson().moves).toEqual([]);
  });

  it("--rollback with the plugin enabled but not running (half-installed) also reads data.db", { timeout: 90_000 }, () => {
    const r = rig({ plugin: "disabled" });
    writeFileSync(join(r.ctl, "state"), "true error\n");
    const o = r.inst(["--rollback", "--previous-plugin", r.prev]);
    expect(o.status, o.stdout + o.stderr).toBe(0);
    expect(r.exportJson().moves).toEqual(r.dbMoves());
  });
});
