// scripts/home-ui.sh install_build against a stateful stub bb and temp directories. Never live (mk-4i8rd).
//
// Two behaviours of the real `bb`, both seen on the live deploy path:
//  1. `bb plugin install` of a path plugin leaves the plugin enabled and running, so the `bb plugin enable` that follows reloads it; bb
//     stops each service within 5 s and marks the plugin degraded when one misses the window.
//  2. `bb plugin install` can exit non-zero on the client (a reset connection while it waits) after the server has finished the install.
// The stub models both: HUI_INSTALL picks the install outcome, HUI_LEFT the state the install leaves, HUI_RELOAD=degrade makes an enable on
// a running plugin degrade it. The rollback subcommand is the entry point: it calls install_build on the recorded previous build.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, utimesSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const SCRIPT = resolve(__dirname, "../../../scripts/home-ui.sh");
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

const DIST = ["app.js", "app.css", "app.meta.json"];
const OLD = new Date(Date.now() - 3600_000);

function plugin(root: string, name: string) {
  const d = join(root, name);
  mkdirSync(join(d, "node_modules"), { recursive: true });
  mkdirSync(join(d, "dist"));
  writeFileSync(join(d, "package.json"), "{}");
  writeFileSync(join(d, "migrations.ts"), "export const CODE_VERSION = 3;\n");
  for (const f of DIST) {
    const p = join(d, "dist", f);
    writeFileSync(p, f === "app.meta.json" ? '{"pluginId":"autarch"}' : "old");
    utimesSync(p, OLD, OLD);
  }
  return d;
}

type Env = { HUI_INSTALL?: string; HUI_LEFT?: string; HUI_RELOAD?: string; HUI_FAIL_FOR?: string; HOME_UI_RECHECK_WAIT?: string; HOME_UI_RECHECK_INTERVAL?: string };   // HUI_RELOAD: degrade = an enable on a running plugin degrades it; stuck = every enable leaves it degraded

function rig() {
  const root = mkdtempSync(join(tmpdir(), "hui-ib-"));
  dirs.push(root);
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT)");
  db.prepare("INSERT INTO schema_meta VALUES ('schema_version', '3')").run();
  db.close();
  const cur = plugin(root, "cur");
  const state = join(root, "state");
  mkdirSync(state);
  writeFileSync(join(state, "mode"), "running\n");
  writeFileSync(join(state, "source"), `path:${cur}\n`);
  const calls = join(root, "calls");
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh
S=${state}
echo "$@" >> ${calls}
case "$1 $2" in
  "plugin source") echo "autarch"; echo "  resolved: path:${cur}" ;;
  "plugin disable") echo disabled > $S/mode ;;
  "plugin install")
    dir=$4
    [ "$HUI_FAIL_FOR" = "$dir" ] && exit 1
    case "$HUI_INSTALL" in
      nothing) exit 1 ;;
      half) echo built > "$dir/dist/.app.js.tmp" && mv "$dir/dist/.app.js.tmp" "$dir/dist/app.js"; exit 1 ;;
    esac
    for f in app.js app.css; do echo built > "$dir/dist/.$f.tmp" && mv "$dir/dist/.$f.tmp" "$dir/dist/$f"; done
    echo '{"pluginId":"autarch"}' > "$dir/dist/.app.meta.json.tmp" && mv "$dir/dist/.app.meta.json.tmp" "$dir/dist/app.meta.json"
    echo "path:$dir" > $S/source
    echo "\${HUI_LEFT:-running}" > $S/mode
    [ "$HUI_INSTALL" = client-error ] && exit 1
    exit 0 ;;
  "plugin enable")
    if [ "$HUI_RELOAD" = stuck ] || { [ "$(cat $S/mode)" = running ] && [ "$HUI_RELOAD" = degrade ]; }; then echo degraded > $S/mode; else echo running > $S/mode; fi ;;
  "plugin list")
    m=$(cat $S/mode); src=$(cat $S/source)
    if [ "$m" = disabled ]; then e=false; st=stopped; else e=true; st=$m; fi
    echo "{\\"plugins\\":[{\\"id\\":\\"autarch\\",\\"enabled\\":$e,\\"status\\":\\"$st\\",\\"source\\":\\"$src\\"}]}" ;;
  *) exit 0 ;;
esac
`);
  chmodSync(bb, 0o755);
  // A backup the rollback subcommand accepts: the previous build is `cur`.
  const backup = join(data, "home-backups", "T");
  mkdirSync(backup, { recursive: true });
  writeFileSync(join(backup, "previous-source.txt"), cur + "\n");
  const bdb = new Database(join(backup, "data.db"));
  bdb.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT)");
  bdb.close();
  const run = (args: string[], e: Env = {}) =>
    spawnSync("bash", [SCRIPT, ...args], {
      env: { PATH: process.env.PATH!, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2", HOME_UI_RECHECK_WAIT: "2", HOME_UI_RECHECK_INTERVAL: "1", ...e },
      encoding: "utf8",
      timeout: 60_000,
    });
  const log = () => (existsSync(calls) ? readFileSync(calls, "utf8").trim().split("\n").filter(Boolean) : []);
  const count = (re: RegExp) => log().filter((c) => re.test(c)).length;
  const mode = () => readFileSync(join(state, "mode"), "utf8").trim();
  return { root, data, cur, backup, run, log, count, mode };
}

describe("home-ui.sh install_build: the install leaves the plugin running", () => {
  it("does not reload a plugin the install left running (the reload degrades it past bb's 5 s stop window)", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_RELOAD: "degrade" });
    expect(out.stderr).not.toContain("did not come back running");
    expect(out.status).toBe(0);
    expect(r.count(/^plugin install /)).toBe(1);
    expect(r.count(/^plugin enable /)).toBe(0);
    expect(r.mode()).toBe("running");
  });

  it("still enables a plugin the install left disabled, exactly once", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_LEFT: "disabled" });
    expect(out.status).toBe(0);
    expect(r.count(/^plugin enable autarch$/)).toBe(1);
    expect(r.mode()).toBe("running");
  });

  it("still enables a plugin the install left degraded, exactly once", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_LEFT: "degraded" });
    expect(out.status).toBe(0);
    expect(r.count(/^plugin enable autarch$/)).toBe(1);
    expect(r.mode()).toBe("running");
  });

  it("a plugin that stays degraded after the one enable is still a failure (rollback maps it to exit 5)", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_LEFT: "degraded", HUI_RELOAD: "stuck" });
    expect(out.status).toBe(5);
    expect(out.stderr).toContain("did not come back running");
    expect(r.count(/^plugin enable autarch$/)).toBe(1);
  });
});

describe("home-ui.sh install_build: the install exits non-zero on the client", () => {
  it("carries on when the server finished the install (running, from this plugin, frontend rebuilt)", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "client-error", HUI_RELOAD: "degrade" });
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("exited");
    expect(out.stderr).toContain("completed the install");
    expect(r.count(/^plugin install /)).toBe(1);   // the failing install is not run a second time
    expect(r.count(/^plugin enable /)).toBe(0);
    expect(r.mode()).toBe("running");
  });

  it("carries on when the server finished the install and left the plugin disabled, with one enable", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "client-error", HUI_LEFT: "disabled" });
    expect(out.status).toBe(0);
    expect(r.count(/^plugin install /)).toBe(1);
    expect(r.count(/^plugin enable autarch$/)).toBe(1);
  });

  it("a real failure (nothing was installed) is still exit 5, Home stays disabled, no enable", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "nothing" });
    expect(out.status).toBe(5);
    expect(out.stderr).toContain("install of");
    expect(out.stderr).toContain("failed; Home is disabled");
    expect(r.count(/^plugin enable /)).toBe(0);
    expect(r.mode()).toBe("disabled");
  });

  it("zero-padded recheck knobs (08, 09) are read as decimal: the failure is still exit 5, not a bash octal abort", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "nothing", HOME_UI_RECHECK_WAIT: "08", HOME_UI_RECHECK_INTERVAL: "09" });
    expect(out.stderr).not.toContain("value too great for base");
    expect(out.status).toBe(5);
    expect(out.stderr).toContain("failed; Home is disabled");
    expect(r.mode()).toBe("disabled");
  }, 30_000); // the smallest octal-invalid wait is 8 s: the bounded poll really runs that long

  it("a half-written frontend (app.js rebuilt, the rest old) is not a finished install", () => {
    const r = rig();
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "half" });
    expect(out.status).toBe(5);
    expect(r.count(/^plugin enable /)).toBe(0);
  });
});

describe("home-ui.sh install_build: the stand-in bb never outlives the install", () => {
  it("a failed preview install still restores the previous build with the real bb", () => {
    const r = rig();
    const n = plugin(r.root, "new");
    const out = r.run(["preview", "--path", n], { HUI_FAIL_FOR: n, HUI_INSTALL: "ok" });
    expect(out.status).toBe(5);
    const installs = r.log().filter((c) => c.startsWith("plugin install"));
    expect(installs).toEqual([`plugin install --yes ${n}`, `plugin install --yes ${r.cur}`]);
    expect(r.mode()).toBe("running");
  });
});

// A signal that arrives while install_build is answering for the install with a stand-in bb must not leave the stand-in in charge of
// the exit-trap restore: the restore has to reinstall the previous build with the real bb. The signal is sent by a DEBUG trap loaded
// through BASH_ENV, exactly when the unchanged install-result line (window 1) or the unchanged enable line (window 2) is about to run
// against /bin/true or /bin/false.
describe("home-ui.sh install_build: a signal while the stand-in bb is in place", () => {
  function hook(root: string, pattern: string) {
    const f = join(root, "inject.sh");
    writeFileSync(
      f,
      `set -T
trap 'if [ "$BASHPID" = "$$" ] && { [ "\${BB:-}" = /bin/true ] || [ "\${BB:-}" = /bin/false ]; } && [[ "$BASH_COMMAND" == ${pattern} ]]; then trap - DEBUG; kill -TERM $$; fi' DEBUG
`,
    );
    return f;
  }
  for (const [name, pattern] of [
    ["the install-result line", '*\\"\\$BB\\"\\ plugin\\ install*'],
    ["the enable line", '*\\"\\$BB\\"\\ plugin\\ enable*'],
  ] as const) {
    it(`reinstalls the previous build with the real bb when interrupted at ${name}`, () => {
      const r = rig();
      const n = plugin(r.root, "new");
      const out = r.run(["preview", "--path", n], { BASH_ENV: hook(r.root, pattern) } as never);
      expect(out.status).toBe(130);
      const installs = r.log().filter((c) => c.startsWith("plugin install"));
      expect(installs).toEqual([`plugin install --yes ${n}`, `plugin install --yes ${r.cur}`]);
      expect(r.mode()).toBe("running");
    });
  }
});

// The server writes each frontend artifact by renaming a staged file over it, so a finished install leaves a different file at each
// path. A file that is merely dated in the future (a clock that stepped back, a restored backup) is "newer than the marker" without any
// install having run; it must not turn a failed install into a success.
describe("home-ui.sh install_build: a future-dated artifact is not evidence of an install", () => {
  const FUTURE = new Date(Date.now() + 3600_000);
  const dateAll = (cur: string, names: string[], when: Date) => names.forEach((f) => utimesSync(join(cur, "dist", f), when, when));

  it("a failed install that changed nothing, over future-dated artifacts, is still exit 5", () => {
    const r = rig();
    dateAll(r.cur, DIST, FUTURE);
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "nothing" });
    expect(out.status).toBe(5);
    expect(out.stderr).not.toContain("completed the install");
    expect(r.count(/^plugin enable /)).toBe(0);
    expect(r.mode()).toBe("disabled");
  });

  it("a partial install (app.js replaced; app.css and app.meta.json future-dated and unchanged) is still exit 5", () => {
    const r = rig();
    dateAll(r.cur, ["app.css", "app.meta.json"], FUTURE);
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "half" });
    expect(out.status).toBe(5);
    expect(r.count(/^plugin enable /)).toBe(0);
  });

  it("a finished install over future-dated artifacts is still recognised (every file was replaced)", () => {
    const r = rig();
    dateAll(r.cur, DIST, FUTURE);
    const out = r.run(["rollback", r.backup], { HUI_INSTALL: "client-error" });
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("completed the install");
  });
});
