// scripts/home-ui.sh against a stub bb and a temp data directory. Never live. The deploy subcommand fetches origin, so it is
// covered by the same gates through `check`/`preview`, not run here.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const SCRIPT = resolve(__dirname, "../../../scripts/home-ui.sh");
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

function plugin(root: string, name: string, version: number, extra = "") {
  const d = join(root, name);
  mkdirSync(join(d, "node_modules"), { recursive: true });
  writeFileSync(join(d, "package.json"), "{}");
  writeFileSync(join(d, "migrations.ts"), `export const CODE_VERSION = ${version};\n${extra}`);
  return d;
}

function rig(live = 3, failAfter = "") {
  const root = mkdtempSync(join(tmpdir(), "hui-"));
  dirs.push(root);
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE t (x TEXT)");
  db.prepare("INSERT INTO schema_meta VALUES ('schema_version', ?)").run(String(live));
  db.prepare("INSERT INTO t VALUES ('before')").run();
  db.close();
  const cur = plugin(root, "cur", live);
  const calls = join(root, "calls");
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh
echo "$@" >> ${calls}
case "$1 $2" in
  "plugin source") echo "autarch"; echo "  resolved: path:${cur}" ;;
  "plugin install") [ -n "$HUI_FAIL_AFTER" ] && [ "$(grep -c '^plugin install' ${calls})" -gt "$HUI_FAIL_AFTER" ] && exit 1; exit 0 ;;
  "plugin list") echo '{"plugins":[{"id":"autarch","enabled":true,"status":"running"}]}' ;;
  *) exit 0 ;;
esac
`);
  chmodSync(bb, 0o755);
  const run = (args: string[]) => spawnSync("bash", [SCRIPT, ...args], { env: { PATH: process.env.PATH!, HUI_FAIL_AFTER: failAfter, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2" }, encoding: "utf8" });
  const log = () => (existsSync(calls) ? readFileSync(calls, "utf8") : "");
  const backups = () => { try { return readdirSync(join(data, "home-backups")); } catch { return []; } };
  return { root, data, cur, run, log, backups };
}

describe("home-ui.sh gates", () => {
  it("check passes when the schema matches and migrations.ts is identical", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(0);
    expect(out.stdout).toContain("gate ok");
  });

  it("refuses a build whose schema differs from the live one, before any backup or bb change", () => {
    const r = rig(3);
    const n = plugin(r.root, "new", 4);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("NOT UI-only");
    expect(r.backups()).toEqual([]);
    expect(r.log()).not.toMatch(/disable|install/);
  });

  it("allows a ui/ change but refuses a change to any other file", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    mkdirSync(join(n, "ui"));
    writeFileSync(join(n, "ui", "a.tsx"), "x");
    expect(r.run(["check", "--path", n]).status).toBe(0);
    writeFileSync(join(n, "server.ts"), "writes the db");
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("server.ts");
  });

  it("refuses a build whose migrations.ts differs even at the same version", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3, "// changed\n");
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("migrations.ts differs");
  });

  it("fails closed on an unreadable database, a missing node_modules and a bad commit", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    rmSync(join(n, "node_modules"), { recursive: true });
    expect(r.run(["check", "--path", n]).stderr).toContain("node_modules");
    rmSync(join(r.data, "plugins", "autarch", "data.db"));
    const m = plugin(r.root, "new2", 3);
    expect(r.run(["check", "--path", m]).status).not.toBe(0);
    expect(r.run(["deploy", "--commit", "abc"]).status).toBe(64);
    expect(r.run(["deploy"]).status).toBe(64);
  });
});

describe("home-ui.sh preview and rollback", () => {
  it("backs up first, swaps to the preview build, runs dev, then restores the previous build", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(0);
    const [b] = r.backups();
    expect(b).toBeDefined();
    const dir = join(r.data, "home-backups", b);
    expect(readFileSync(join(dir, "previous-source.txt"), "utf8").trim()).toBe(r.cur);
    expect(new Database(join(dir, "data.db"), { readonly: true }).prepare("SELECT x FROM t").get()).toEqual({ x: "before" });
    const calls = r.log().trim().split("\n");
    const installs = calls.filter((c) => c.startsWith("plugin install"));
    expect(installs).toEqual([`plugin install --yes ${n}`, `plugin install --yes ${r.cur}`]);
    expect(calls).toContain(`plugin dev ${n}`);
    expect(calls.indexOf(`plugin dev ${n}`)).toBeLessThan(calls.lastIndexOf(`plugin install --yes ${r.cur}`));
    expect(out.stdout).toContain("rollback: bash");
    expect(out.stdout).toContain("live data");
  });

  it("exits nonzero when the preview cannot restore the previous build", () => {
    const r = rig(3, "1");
    const n = plugin(r.root, "new", 3);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(7);
    expect(out.stderr).toContain("restore failed");
  });

  it("rollback reinstalls the recorded build and restores data only when asked", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    r.run(["preview", "--path", n]);
    const dir = join(r.data, "home-backups", r.backups()[0]);
    const live = join(r.data, "plugins", "autarch", "data.db");
    const w = new Database(live); w.prepare("INSERT INTO t VALUES ('after')").run(); w.close();
    expect(r.run(["rollback", dir]).status).toBe(0);
    const count = () => { const d = new Database(live, { readonly: true }); try { return d.prepare("SELECT count(*) AS c FROM t").get(); } finally { d.close(); } };
    expect(count()).toEqual({ c: 2 });
    expect(r.run(["rollback", dir, "--restore-data"]).status).toBe(0);
    expect(count()).toEqual({ c: 1 });
    expect(r.run(["rollback", r.root]).status).not.toBe(0);
  });

  it("refuses to run as root", () => {
    if (process.getuid?.() !== 0) return;
    expect(spawnSync("bash", [SCRIPT, "check"], { encoding: "utf8" }).status).toBe(64);
  });
});
