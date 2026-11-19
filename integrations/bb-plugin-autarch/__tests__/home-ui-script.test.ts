// scripts/home-ui.sh against a stub bb and a temp data directory. Never live. The deploy subcommand fetches origin, so it is
// covered below through an extracted transaction body with fake Git, build, plugin and service commands.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const SCRIPT = resolve(__dirname, "../../../scripts/home-ui.sh");
const dirs: string[] = [];
const CLI_HELPER = resolve(__dirname, "../../../scripts/home-cli.sh");

// Exercise the real deploy/rollback bodies and CLI helpers with shell stand-ins. No live services or bb.
function cliRig(mode = "ok") {
  const root = mkdtempSync(join(tmpdir(), "hui-cli-"));
  dirs.push(root);
  const sha = "a".repeat(40), old = "b".repeat(40);
  const bin = join(root, ".local/bin/autarch");
  mkdirSync(join(root, ".local/bin"), { recursive: true });
  const binary = (rev: string) => `#!/bin/bash
if [ "$1" = version ]; then echo 'revision ${rev} modified=false sha256 sample'; exit; fi
[ "${rev}" != "${sha}" ] || [ "$MODE" != postfail ] || exit 1
echo '{"counts":0}'
`;
  writeFileSync(bin, binary(old)); chmodSync(bin, 0o755);
  writeFileSync(join(root, "new"), binary(mode === "wrong" ? old : mode === "unstamped" ? "" : sha));
  const script = readFileSync(SCRIPT, "utf8");
  const body = script.slice(script.lastIndexOf("case $SUB in\ncheck)"));
  const harness = join(root, "run");
  writeFileSync(harness, `#!/bin/bash
set -euo pipefail
MKHOME=$HOME; REPO=$HOME; BUILDS=$HOME/builds; PLUGIN_REL=plugin; COMMIT=${sha}
BB=true; PLUGIN_CODE=0; PATH_ARG=; RESTORE_DATA=0
ST=$HOME/state; mkdir -p "$ST" "$BUILDS/build-${sha.slice(0, 12)}/plugin" "$HOME/backup" "$HOME/previous"
die() { echo "$*" >&2; exit 1; }
stamp_warning() { echo "$*"; }
git() { case "$*" in *status*) [ "$MODE" != dirty ] || echo "?? stray"; [ "$MODE" != vcsfail ];; *'rev-parse HEAD'*) echo "$COMMIT";; *--show-toplevel*) echo "$BUILDS/build-${sha.slice(0, 12)}";; esac; }
npm() { :; }
go() { printf '%s\\n' "$*" > "$HOME/go-args"; [ "$MODE" != buildfail ] || return 1; cp "$HOME/new" "$6"; chmod +x "$6"; }
fixture_systemctl() { echo "$*" >> "$HOME/unit"; if [ "$MODE" = queryfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; if [ "$2" = restart ] && [ "$MODE" = restartfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; [ "$MODE" != missing ] || { echo not-found; return; }; if [ "$2" = restart ]; then
    if [ "$MODE" != stale ] || ! grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then
      local pid; pid=$(cat "$HOME/pid"); pid=$((pid+1)); echo "$pid" > "$HOME/pid"
      local target="$HOME/.local/bin/autarch"
      if [ "$MODE" = deleted ] && grep -q "revision ${sha}" "$target"; then target="$target (deleted)"; fi
      mkdir -p "$HOME/proc/$pid"; ln -s "$target" "$HOME/proc/$pid/exe"
    fi
  fi
  if [ "$2" = show ]; then case "$*" in *MainPID*) cat "$HOME/pid";; *) echo loaded;; esac; fi; }
curl() { if [ "$MODE" = healthfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; echo '{"status":"ok"}'; }
gate_ui_only() { :; }
backup() { echo "$HOME/previous" > "$HOME/backup/previous-source.txt"; touch "$HOME/backup/data.db"; echo "$HOME/backup"; }
install_build() { echo "$1" >> "$HOME/plugin-log"; }
rollback_cmd() { echo "rollback $1"; }
[ -f "$HOME/pid" ] || echo 10 > "$HOME/pid"
mkdir -p "$HOME/proc/10"
[ -L "$HOME/proc/10/exe" ] || ln -s "$HOME/.local/bin/autarch (deleted)" "$HOME/proc/10/exe"
HOME_UI_PROC_ROOT=$HOME/proc
HOME_UI_SYSTEMCTL=fixture_systemctl
systemctl() { echo "unexpected ambient systemctl" >&2; return 99; }
source '${CLI_HELPER}'
SUB=$1; BACKUP_ARG=$HOME/backup
${body}
`);
  const run = (sub = "deploy") => spawnSync("bash", [harness, sub], { env: { ...process.env, HOME: root, MODE: mode, HOME_UI_WAIT: "1" }, encoding: "utf8" });
  return { root, bin, sha, old, run, version: () => spawnSync(bin, ["version"], { encoding: "utf8" }).stdout };
}

describe("Home CLI deploy transaction", () => {
  it("check reports the live revision and build stamp without installing or restarting", () => {
    const r = cliRig(), out = r.run("check");
    expect(out.status, out.stderr).toBe(0);
    expect(out.stdout).toContain(`live CLI revision: ${r.old}`);
    expect(out.stdout).toContain(`CLI build stamps revision ${r.sha} modified=false`);
    expect(r.version()).toContain(r.old);
    expect(existsSync(join(r.root, "unit"))).toBe(false);
    expect(existsSync(join(r.root, "plugin-log"))).toBe(false);
  });
  it("installs the target, saves the previous binary, and rollback restores it", () => {
    const r = cliRig();
    expect(r.run().status).toBe(0);
    expect(r.version()).toContain(r.sha);
    expect(readFileSync(join(r.root, "go-args"), "utf8")).toContain(`-buildvcs=false -ldflags -X main.buildRevision=${r.sha}`);
    expect(readFileSync(join(r.root, "backup/previous-cli.txt"), "utf8")).toContain(`autarch.prev-${r.old.slice(0, 12)}`);
    const rolled = r.run("rollback");
    expect(rolled.status, rolled.stderr).toBe(0);
    expect(r.version()).toContain(r.old);
  });
  for (const mode of ["wrong", "unstamped", "vcsfail", "dirty", "buildfail"]) it(`refuses ${mode} before installing either component`, () => {
    const r = cliRig(mode), out = r.run();
    expect(out.status).not.toBe(0);
    expect(out.stderr).toMatch(/CLI (build|worktree)|VCS status/);
    expect(r.version()).toContain(r.old);
    expect(existsSync(join(r.root, "plugin-log"))).toBe(false);
  });
  for (const mode of ["postfail", "healthfail", "restartfail", "stale", "deleted"]) it(`restores both components and restarts after ${mode}`, () => {
    const r = cliRig(mode), out = r.run();
    expect(out.status).not.toBe(0);
    expect(out.stderr).toContain("restor");
    expect(out.stdout).toContain("rollback");
    expect(r.version()).toContain(r.old);
    expect(readFileSync(join(r.root, "unit"), "utf8").match(/restart/g)?.length).toBe(2);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous");
  });
  it("restores the old executable even when service queries fail after the swap", () => {
    const r = cliRig("queryfail"), out = r.run();
    expect(out.status).not.toBe(0);
    expect(r.version()).toContain(r.old);
    expect(out.stderr).toContain("restoring CLI anyway");
    expect(readFileSync(join(r.root, "unit"), "utf8")).toContain("restart");
  });
  it("skips an absent user unit with a message", () => {
    const r = cliRig("missing"), out = r.run();
    expect(out.status).toBe(0);
    expect(out.stdout).toContain("unit does not exist");
    expect(readFileSync(join(r.root, "unit"), "utf8")).not.toContain("restart");
  });
});
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
  const run = (args: string[]) => spawnSync("bash", [SCRIPT, ...args], { env: { PATH: process.env.PATH!, HUI_FAIL_AFTER: failAfter, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2", HOME_UI_RECHECK_WAIT: "1", HOME_UI_RECHECK_INTERVAL: "1" }, encoding: "utf8" });
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

  it("--plugin-code allows plugin code outside ui/ but still refuses dependency, schema and migrations.ts changes", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "server.ts"), "plugin logic");
    const refused = r.run(["check", "--path", n]);
    expect(refused.status).toBe(3);
    const ok = r.run(["check", "--path", n, "--plugin-code"]);
    expect(ok.status).toBe(0);
    expect(ok.stdout).toContain("plugin code may differ");
    writeFileSync(join(n, "package.json"), '{"dependencies":{"x":"1"}}');
    const dep = r.run(["check", "--path", n, "--plugin-code"]);
    expect(dep.status).toBe(3);
    expect(dep.stderr).toContain("package.json differs");
    const s2 = plugin(r.root, "new2", 4);
    expect(r.run(["check", "--path", s2, "--plugin-code"]).status).toBe(3);
    const m = plugin(r.root, "new3", 3, "// changed\n");
    expect(r.run(["check", "--path", m, "--plugin-code"]).stderr).toContain("migrations.ts differs");
    expect(r.run(["preview", "--plugin-code"]).status).toBe(64);
  });

  it("--plugin-code refuses changed plugin code that contains schema statements", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "store.ts"), 'db.exec("CREATE TABLE gate_bypassed(x TEXT)");');
    const out = r.run(["check", "--path", n, "--plugin-code"]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("schema statement");
    writeFileSync(join(n, "store.ts"), "ok();");
    expect(r.run(["check", "--path", n, "--plugin-code"]).status).toBe(0);
  });

  it("--plugin-code scans added lines, including new directories and multi-line SQL, and ignores unchanged references", () => {
    const r = rig();
    writeFileSync(join(r.cur, "backup.ts"), 'q("SELECT value FROM schema_meta");\n');
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "backup.ts"), 'q("SELECT value FROM schema_meta");\n// comment only\n');
    expect(r.run(["check", "--path", n, "--plugin-code"]).status).toBe(0);
    mkdirSync(join(n, "lib"));
    writeFileSync(join(n, "lib", "x.ts"), "db.exec(`CREATE\n  TABLE t2(x)`);\n");
    const out = r.run(["check", "--path", n, "--plugin-code"]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("schema statement");
  });

  it("--plugin-code scans binary-looking files, lines starting with ++ and odd file names", () => {
    const r = rig();
    const bad = 'db.exec("CREATE TABLE bypass(x)");';
    for (const [name, body] of [["bin.js", `\0${bad}`], ["pp.js", `++c; ${bad}`], ["a b ui", bad]] as const) {
      const n = plugin(r.root, `n-${name.replace(/\W/g, "_")}`, 3);
      mkdirSync(join(n, "lib"), { recursive: true });
      writeFileSync(join(n, "lib", name), body);
      expect(r.run(["check", "--path", n, "--plugin-code"]).status, name).toBe(3);
    }
    const n2 = plugin(r.root, "n-sp", 3);
    mkdirSync(join(n2, "lib x"), { recursive: true });
    mkdirSync(join(n2, "lib x", "ui"));
    writeFileSync(join(n2, "lib x", "ui", "p.js"), bad);
    expect(r.run(["check", "--path", n2, "--plugin-code"]).status).toBe(3);
  });

  it("--plugin-code refuses any added, changed or dangling lockfile and .npmrc", () => {
    const r = rig();
    for (const f of ["package-lock.json", "npm-shrinkwrap.json", ".npmrc", "pnpm-lock.yaml", "yarn.lock"]) {
      const n = plugin(r.root, `n-${f}`, 3);
      writeFileSync(join(n, f), "new");
      const out = r.run(["check", "--path", n, "--plugin-code"]);
      expect(out.status, f).toBe(3);
      expect(out.stderr).toContain(f);
    }
    const d = plugin(r.root, "dangling", 3);
    symlinkSync("/nonexistent-lock", join(d, "package-lock.json"));
    expect(r.run(["check", "--path", d, "--plugin-code"]).status).toBe(3);
  });

  it("refuses to pick when no Home database or more than one is found", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const home = join(r.root, "h");
    mkdirSync(home);
    const env = { PATH: process.env.PATH!, HOME: home, HOME_BB_BIN: join(r.root, "bb"), HOME_UI_WAIT: "2" };
    expect(spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" }).stderr).toContain("found 0");
    for (const x of ["x", "y"]) { mkdirSync(join(home, x, "plugins", "autarch"), { recursive: true }); writeFileSync(join(home, x, "plugins", "autarch", "data.db"), ""); }
    expect(spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" }).stderr).toContain("found 2");
  });

  it("finds the Home database in a data directory five levels below the home directory", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const deep = join(r.root, "a", "b", "c", "data");
    mkdirSync(join(deep, "plugins", "autarch"), { recursive: true });
    writeFileSync(join(deep, "plugins", "autarch", "data.db"), "");
    const env = { PATH: process.env.PATH!, HOME: join(r.root, "a"), HOME_BB_BIN: join(r.root, "bb"), HOME_UI_WAIT: "2" };
    const out = spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" });
    expect(out.stderr).not.toContain("cannot pick one Home database");
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
  // R5: check lists every failed gate in one run; deploy and preview stay fail-fast.
  describe("accumulation", () => {
    it("every subset of {no node_modules, other schema, file outside ui/} is reported whole by check", () => {
      const names = ["no node_modules", "the build is schema", "files outside ui/"];
      for (let mask = 1; mask < 8; mask++) {
        const r = rig();
        const n = plugin(r.root, "new", mask & 2 ? 4 : 3);
        if (mask & 1) rmSync(join(n, "node_modules"), { recursive: true });
        if (mask & 4) writeFileSync(join(n, "server.ts"), "export {};\n");
        const out = r.run(["check", "--path", n]);
        expect(out.status, `mask ${mask}`).toBe(3);
        names.forEach((say, i) => { if (mask & (1 << i)) expect(out.stderr, `mask ${mask}: ${say}`).toContain(say); });
        expect(out.stderr).toContain("failed gate(s)");
        expect(r.log()).not.toContain("plugin disable");
      }
    });
    it("check names a build that has no package.json", () => {
      const r = rig();
      const n = plugin(r.root, "new", 3);
      rmSync(join(n, "package.json"));
      const out = r.run(["check", "--path", n]);
      expect(out.status).toBe(3);
      expect(out.stderr).toContain("is not a Home plugin build");
    });
    it("preview stops at the first failed gate", () => {
      const r = rig();
      const n = plugin(r.root, "new", 4);
      rmSync(join(n, "node_modules"), { recursive: true });
      const out = r.run(["preview", "--path", n]);
      expect(out.status).not.toBe(0);
      expect(out.stderr).toContain("no node_modules");
      expect(out.stderr).not.toContain("the build is schema");
    });
  });

});
