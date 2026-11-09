// scripts/home-ui.sh --engine tool|legacy, against a stub bb, a fake tool and temp directories. Never live: the installed build, the state
// directory, the data directory and every build are made here, and the "tool" that gets exec'd is a stand-in that only records what it saw.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const HUI = resolve(__dirname, "../../../scripts/home-ui.sh");
const PLUGIN_REL = "integrations/bb-plugin-autarch";
const dirs: string[] = [];
const procs: ChildProcess[] = [];
afterEach(() => {
  procs.splice(0).forEach((p) => p.kill("SIGKILL"));
  dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true }));
});

const git = (cwd: string, ...args: string[]) =>
  spawnSync("git", ["-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.invalid", ...args], { cwd, encoding: "utf8" });

// The stand-in tool: records argv, the lock descriptor it inherited, and whether the lock was held while it ran.
const FAKE_TOOL = `import { appendFileSync, fstatSync, statSync } from "node:fs";
import { spawnSync } from "node:child_process";
const sd = process.argv[process.argv.indexOf("--state-dir") + 1];
const fd = Number(process.env.BPD_LOCK_FD);
let inherited = false;
try { const a = fstatSync(fd), b = statSync(sd + "/lock"); inherited = a.ino === b.ino && a.dev === b.dev; } catch {}
const held = spawnSync("flock", ["-n", sd + "/lock", "true"]).status !== 0;
appendFileSync(process.env.FAKE_OUT, JSON.stringify({ argv: process.argv.slice(2), lockFd: process.env.BPD_LOCK_FD ?? null, inherited, held, bb: process.env.BB_PLUGIN_DEPLOY_BB ?? null }) + "\\n");
process.exit(Number(process.env.FAKE_EXIT ?? 0));
`;
const PLUGIN_FILE = {
  format: "bb-plugin-deploy/plugin@1",
  pluginId: "autarch",
  pluginDir: PLUGIN_REL,
  controlPaths: ["tools/bb-plugin-deploy/**", "scripts/home-gates.sh", "scripts/home-ui.sh", `${PLUGIN_REL}/deploy.json`, "**/AGENTS.md"],
};

function rig(opts: { noState?: boolean; stamped?: boolean } = {}) {
  const root = mkdtempSync(join(tmpdir(), "hengine-"));
  dirs.push(root);
  const st = join(root, "state");
  if (!opts.noState) mkdirSync(st);
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT)");
  db.prepare("INSERT INTO schema_meta VALUES ('schema_version', '3')").run();
  db.close();
  // The installed build: a git checkout of the repo, with the tool, the plugin file and the control scripts committed.
  const inst = join(root, "inst");
  const put = (rel: string, text: string) => {
    mkdirSync(join(inst, rel, ".."), { recursive: true });
    writeFileSync(join(inst, rel), text);
  };
  put("tools/bb-plugin-deploy/cli.mjs", FAKE_TOOL);
  put("tools/bb-plugin-deploy/lib/x.mjs", "export const x = 1;\n");
  put(`${PLUGIN_REL}/deploy.json`, JSON.stringify(PLUGIN_FILE, null, 2) + "\n");
  put(`${PLUGIN_REL}/migrations.ts`, "export const CODE_VERSION = 3;\n");
  put(`${PLUGIN_REL}/package.json`, "{}");
  put("scripts/home-gates.sh", "#!/bin/bash\nexit 0\n");
  put("scripts/home-ui.sh", "#!/bin/bash\nexit 0\n");
  put("AGENTS.md", "x\n");
  git(inst, "init", "-q");
  git(inst, "add", "-A");
  git(inst, "commit", "-q", "-m", "installed");
  const head = git(inst, "rev-parse", "HEAD").stdout.trim();
  const tree = git(inst, "rev-parse", "HEAD^{tree}").stdout.trim();
  if (opts.stamped) {
    mkdirSync(join(inst, ".bb-plugin-deploy"));
    writeFileSync(join(inst, ".bb-plugin-deploy", "stamp.json"), JSON.stringify({ format: "bb-plugin-deploy/stamp@1", pluginId: "autarch", commit: head, tree }));
  }
  const calls = join(root, "calls");
  const out = join(root, "tool-out");
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh
echo "$@" >> ${calls}
case "$1 $2" in
  "plugin source") echo "autarch"; echo "  resolved: path:${inst}/${PLUGIN_REL}" ;;
  "plugin list") echo '{"plugins":[{"id":"autarch","enabled":true,"status":"running"}]}' ;;
  *) exit 0 ;;
esac
`);
  chmodSync(bb, 0o755);
  const env = (extra: Record<string, string> = {}): Record<string, string> => ({
    PATH: process.env.PATH!, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2", HOME_UI_STATE_DIR: st, FAKE_OUT: out, ...extra,
  });
  const log = () => (existsSync(calls) ? readFileSync(calls, "utf8") : "");
  const seen = (): any[] => (existsSync(out) ? readFileSync(out, "utf8").trim().split("\n").filter(Boolean).map((l) => JSON.parse(l)) : []);
  const run = (args: string[], extra: Record<string, string> = {}) => spawnSync("bash", [HUI, ...args], { env: env(extra), encoding: "utf8" });
  return { root, st, inst, data, head, tree, bb, calls, out, put, env, log, seen, run, lock: join(st, "lock") };
}
type Rig = ReturnType<typeof rig>;

const SHA = "a".repeat(40);
const TREE = "b".repeat(40);
const TOOL_REFUSED = 10;
const legacyHint = /use --engine legacy/;

/** Another process (its own open file description) holding the lock until killed or `secs` pass. */
function holder(lock: string, secs = 60): Promise<ChildProcess> {
  const p = spawn("bash", ["-c", 'exec 8>>"$1"; flock -n 8 && echo held; sleep "$2"', "holder", lock, String(secs)], { stdio: ["ignore", "pipe", "ignore"] });
  procs.push(p);
  return new Promise((ok, no) => {
    p.stdout!.once("data", () => ok(p));
    p.once("exit", () => no(new Error("holder exited before it held the lock")));
  });
}

// A candidate build for the legacy gates: the installed plugin directory, copied (so its migrations.ts is identical).
function legacyPlugin(r: Rig, name: string) {
  const d = join(r.root, name);
  cpSync(join(r.inst, PLUGIN_REL), d, { recursive: true });
  mkdirSync(join(d, "node_modules"), { recursive: true });
  return d;
}

describe("home-ui.sh --engine: the legacy path", () => {
  it("with no flag, check still runs the legacy gates, never reads the installed tool and takes no lock", async () => {
    const r = rig();
    const n = legacyPlugin(r, "new");
    // The installed build here is not even a tool build: legacy never looks. And a held lock does not stop a legacy check.
    await holder(r.lock);
    const out = r.run(["check", "--path", n]);
    expect(out.status, out.stderr).toBe(0);
    expect(out.stdout).toContain("gate ok");
    expect(r.seen()).toEqual([]);
    expect(out.stderr).not.toContain("--engine tool");
  });

  it("--engine legacy is the same as no flag", () => {
    const r = rig();
    const n = legacyPlugin(r, "new");
    const a = r.run(["check", "--path", n]);
    const b = r.run(["check", "--path", n, "--engine", "legacy"]);
    expect([b.status, b.stdout, b.stderr]).toEqual([a.status, a.stdout, a.stderr]);
    expect(r.seen()).toEqual([]);
  });

  it("legacy preview still takes the lock and backs up, and never execs the tool", () => {
    const r = rig();
    const n = legacyPlugin(r, "new");
    const out = r.run(["preview", "--path", n]);
    expect(out.status, out.stderr).toBe(0);
    expect(r.log()).toContain("plugin dev " + n);
    expect(r.seen()).toEqual([]);
  });

  it("rollback never calls the tool, and refuses --engine altogether", () => {
    const r = rig();
    const backup = join(r.root, "backup");
    mkdirSync(backup);
    writeFileSync(join(backup, "data.db"), "x");
    writeFileSync(join(backup, "previous-source.txt"), r.inst + "/" + PLUGIN_REL + "\n");
    const ok = r.run(["rollback", backup]);
    expect(ok.status).toBe(0);
    expect(ok.stdout).toContain("rolled back to");
    expect(r.seen()).toEqual([]);
    for (const e of ["tool", "legacy"]) {
      const no = r.run(["rollback", backup, "--engine", e]);
      expect(no.status).toBe(64);
      expect(no.stderr).toContain("never calls the tool");
    }
    expect(r.run(["migrate-check", "--engine", "tool"]).status).toBe(64);
    expect(r.seen()).toEqual([]);
  });
});

describe("home-ui.sh --engine tool: delegation", () => {
  it("deploy execs the installed tool with the flags, the data and state dirs, and the inherited lock", () => {
    const r = rig();
    const out = r.run(["deploy", "--commit", SHA, "--engine", "tool", "--expect-tree", TREE], { FAKE_EXIT: "0" });
    expect(out.status, out.stderr).toBe(0);
    const [call, ...rest] = r.seen();
    expect(rest).toEqual([]);
    expect(call.argv).toEqual([
      "deploy", "--plugin-file", `${r.inst}/${PLUGIN_REL}/deploy.json`, "--bb-data", r.data, "--state-dir", r.st,
      "--commit", SHA, "--class", "plugin-deploy", "--mode", "agent", "--expect-tree", TREE,
    ]);
    expect(call.lockFd).toMatch(/^\d+$/);
    expect(call.inherited).toBe(true);   // the descriptor is the lock file itself
    expect(call.held).toBe(true);        // and the lock is held while the tool runs
    expect(call.bb).toBe(r.bb);
    expect(r.log()).not.toMatch(/disable|install/);   // the wrapper itself changed nothing
    expect(readFileSync(r.lock, "utf8")).toBe("");
  });

  it("the tool's exit code is the wrapper's exit code", () => {
    const r = rig();
    expect(r.run(["deploy", "--commit", SHA, "--engine", "tool"], { FAKE_EXIT: "20" }).status).toBe(20);
    expect(r.run(["check", "--commit", SHA, "--engine", "tool"], { FAKE_EXIT: "10" }).status).toBe(10);
  });

  it("check execs `check --commit C --class plugin-deploy`, under the lock (the legacy check takes none)", () => {
    const r = rig();
    const out = r.run(["check", "--commit", SHA, "--engine", "tool"]);
    expect(out.status, out.stderr).toBe(0);
    const [call] = r.seen();
    expect(call.argv).toEqual(["check", "--plugin-file", `${r.inst}/${PLUGIN_REL}/deploy.json`, "--bb-data", r.data, "--state-dir", r.st, "--commit", SHA, "--class", "plugin-deploy"]);
    expect(call.inherited).toBe(true);
    expect(call.held).toBe(true);
  });

  it("preview execs `preview` with --commit or --path, never both", () => {
    const r = rig();
    expect(r.run(["preview", "--commit", SHA, "--engine", "tool"]).status).toBe(0);
    expect(r.run(["preview", "--path", "/some/build", "--engine", "tool"]).status).toBe(0);
    const [a, b] = r.seen();
    expect(a.argv.slice(-4)).toEqual(["--commit", SHA, "--mode", "agent"]);
    expect(b.argv.slice(-4)).toEqual(["--path", "/some/build", "--mode", "agent"]);
    expect(a.held && b.held && a.inherited && b.inherited).toBe(true);
    for (const bad of [[], ["--commit", SHA, "--path", "/x"]]) {
      const no = r.run(["preview", "--engine", "tool", ...bad]);
      expect(no.status).toBe(64);
      expect(no.stderr).toContain("exactly one of --commit and --path");
    }
  });

  it("works for a stamped installed build (stamp = HEAD and its tree)", () => {
    const r = rig({ stamped: true });
    expect(r.run(["deploy", "--commit", SHA, "--engine", "tool"]).status).toBe(0);
    expect(r.seen()).toHaveLength(1);
  });

  it("does not need a stamp's extra files, node_modules or .git to be in the closure", () => {
    const r = rig();
    r.put("node_modules/pkg/index.js", "x");   // untracked and ignored by the walk
    r.put(`${PLUGIN_REL}/node_modules/pkg/index.js`, "x");
    r.put("README.md", "outside the closure\n");   // untracked but not a control path
    expect(r.run(["check", "--commit", SHA, "--engine", "tool"]).status).toBe(0);
  });

  it("waits for a lock another process holds, and gives up with 75 and no tool run", async () => {
    const r = rig();
    await holder(r.lock);
    for (const sub of ["deploy", "check", "preview"]) {
      const out = r.run([sub, "--commit", SHA, "--engine", "tool"], { HOME_UI_LOCK_WAIT: "1" });
      expect(out.status).toBe(75);
      expect(out.stderr).toContain("another deploy holds the lock");
    }
    expect(r.seen()).toEqual([]);
  });

  it("refuses when no destination lock could be taken (no state dir)", () => {
    const r = rig({ noState: true });
    const out = r.run(["deploy", "--commit", SHA, "--engine", "tool"]);
    expect(out.status).toBe(TOOL_REFUSED);
    expect(out.stderr).toMatch(legacyHint);
    expect(out.stderr).toContain("destination lock was not taken");
    expect(r.seen()).toEqual([]);
  });
});

describe("home-ui.sh --engine tool: refusals", () => {
  const refused = (r: Rig, why: RegExp, sub = "deploy") => {
    const out = r.run([sub, "--commit", SHA, "--engine", "tool"]);
    expect(out.status).toBe(TOOL_REFUSED);
    expect(out.stderr).toMatch(legacyHint);
    expect(out.stderr).toMatch(why);
    expect(r.seen()).toEqual([]);   // the installed tool never ran
    expect(r.log()).not.toMatch(/disable|install|dev /);
  };

  it("an unknown --engine value is a usage error before anything is read", () => {
    const r = rig();
    for (const sub of ["check", "deploy", "preview"]) {
      const out = r.run([sub, "--commit", SHA, "--engine", "nope"]);
      expect(out.status).toBe(64);
      expect(out.stderr).toContain("--engine must be tool or legacy");
    }
    expect(r.run(["check", "--engine"]).status).not.toBe(0);   // a flag with no value stops, as every other flag does
    expect(r.log()).toBe("");
    expect(r.seen()).toEqual([]);
  });

  it("refuses a tool file that differs from the commit", () => {
    const r = rig();
    r.put("tools/bb-plugin-deploy/lib/x.mjs", "export const x = 2; // tampered\n");
    refused(r, /tools\/bb-plugin-deploy\/lib\/x\.mjs differs/);
  });

  it("refuses a tampered cli.mjs", () => {
    const r = rig();
    r.put("tools/bb-plugin-deploy/cli.mjs", FAKE_TOOL + "// extra\n");
    refused(r, /cli\.mjs differs/);
  });

  it("refuses a changed control script (home-gates.sh, home-ui.sh)", () => {
    for (const f of ["scripts/home-gates.sh", "scripts/home-ui.sh"]) {
      const r = rig();
      r.put(f, "#!/bin/bash\necho changed\n");
      refused(r, new RegExp(`${f.replace(/[./]/g, "\\$&")} differs`));
    }
  });

  it("refuses an extra file under a closure directory", () => {
    const r = rig();
    r.put("tools/bb-plugin-deploy/lib/extra.mjs", "x");
    refused(r, /extra file tools\/bb-plugin-deploy\/lib\/extra\.mjs/);
  });

  it("refuses an extra file that a control glob matches anywhere (an AGENTS.md)", () => {
    const r = rig();
    r.put("sub/AGENTS.md", "x");
    refused(r, /extra file sub\/AGENTS\.md/);
  });

  it("refuses a closure file that is a symlink, and a symlinked closure directory", () => {
    const r = rig();
    rmSync(join(r.inst, "tools/bb-plugin-deploy/lib/x.mjs"));
    symlinkSync("/etc/hostname", join(r.inst, "tools/bb-plugin-deploy/lib/x.mjs"));
    refused(r, /x\.mjs is a symlink/);
    const r2 = rig();
    renameSync(join(r2.inst, "tools/bb-plugin-deploy/lib"), join(r2.root, "lib-moved"));
    symlinkSync(join(r2.root, "lib-moved"), join(r2.inst, "tools/bb-plugin-deploy/lib"));
    refused(r2, /symlink/);
  });

  it("refuses a missing closure file", () => {
    const r = rig();
    rmSync(join(r.inst, "tools/bb-plugin-deploy/lib/x.mjs"));
    refused(r, /x\.mjs is missing/);
  });

  it("refuses an installed plugin file that differs from the commit, even if it is still valid", () => {
    const r = rig();
    r.put(`${PLUGIN_REL}/deploy.json`, JSON.stringify({ ...PLUGIN_FILE, controlPaths: ["nothing/**"] }) + "\n");
    refused(r, /deploy\.json differs/);
  });

  it("refuses a stamp that disagrees with the build's HEAD", () => {
    const r = rig({ stamped: true });
    writeFileSync(join(r.inst, ".bb-plugin-deploy", "stamp.json"), JSON.stringify({ commit: SHA, tree: r.tree }));
    refused(r, /stamp disagrees/);
    const r2 = rig({ stamped: true });
    writeFileSync(join(r2.inst, ".bb-plugin-deploy", "stamp.json"), JSON.stringify({ commit: r2.head, tree: TREE }));
    refused(r2, /stamp disagrees/);
  });

  it("refuses an installed build that is not a git checkout", () => {
    const r = rig();
    rmSync(join(r.inst, ".git"), { recursive: true });
    refused(r, /not a git checkout/);
  });

  it("refuses when the installed tool is missing", () => {
    const r = rig();
    rmSync(join(r.inst, "tools/bb-plugin-deploy/cli.mjs"));
    refused(r, /no deploy tool/);
    const r2 = rig();
    rmSync(join(r2.inst, "tools"), { recursive: true });
    refused(r2, /no deploy tool/, "check");
  });

  it("refuses when the tool is a symlink", () => {
    const r = rig();
    rmSync(join(r.inst, "tools/bb-plugin-deploy/cli.mjs"));
    symlinkSync(join(r.root, "elsewhere.mjs"), join(r.inst, "tools/bb-plugin-deploy/cli.mjs"));
    writeFileSync(join(r.root, "elsewhere.mjs"), FAKE_TOOL);
    refused(r, /no deploy tool/);
  });

  it("refuses when the plugin file is missing", () => {
    const r = rig();
    rmSync(join(r.inst, PLUGIN_REL, "deploy.json"));
    refused(r, /plugin file is missing/);
  });

  it("refuses an invalid plugin file", () => {
    for (const bad of ["not json", "{}", JSON.stringify({ ...PLUGIN_FILE, format: "other" }), JSON.stringify({ ...PLUGIN_FILE, controlPaths: [] }), JSON.stringify({ ...PLUGIN_FILE, pluginId: "" })]) {
      const r = rig();
      r.put(`${PLUGIN_REL}/deploy.json`, bad);
      refused(r, /plugin file is invalid/);
    }
  });

  it("refuses when bb cannot name the installed build, or it is not the plugin's own directory", () => {
    const r = rig();
    writeFileSync(r.bb, "#!/bin/sh\nexit 1\n");
    refused(r, /cannot tell the installed Home build/);
    const r2 = rig();
    writeFileSync(r2.bb, `#!/bin/sh\ncase "$1 $2" in "plugin source") echo "  resolved: path:${r2.inst}/tools";; esac\n`);
    refused(r2, /does not end in/);
  });

  it("refuses an uncommitted change on an unstamped build, such as a mode change", () => {
    const r = rig();
    chmodSync(join(r.inst, "scripts/home-gates.sh"), 0o755);   // same bytes, different mode: only git status sees it
    git(r.inst, "config", "core.fileMode", "true");
    refused(r, /uncommitted change in the closure: scripts\/home-gates\.sh/);
  });
});

describe("home-ui.sh --engine tool: usage", () => {
  const usage = (r: Rig, args: string[], why: RegExp) => {
    const out = r.run(args);
    expect(out.status).toBe(64);
    expect(out.stderr).toMatch(why);
    expect(r.seen()).toEqual([]);
    expect(r.log()).toBe("");
  };
  it("deploy and check need --commit, and take no --path", () => {
    const r = rig();
    usage(r, ["check", "--engine", "tool"], /needs --commit/);
    usage(r, ["deploy", "--engine", "tool"], /needs --commit/);
    usage(r, ["check", "--engine", "tool", "--commit", SHA, "--path", "/x"], /not --path/);
    usage(r, ["deploy", "--engine", "tool", "--commit", SHA, "--path", "/x"], /not --path/);
  });
  it("--plugin-code and --restore-data are not for the tool", () => {
    const r = rig();
    usage(r, ["deploy", "--engine", "tool", "--commit", SHA, "--plugin-code"], /legacy gate option/);
    usage(r, ["check", "--engine", "tool", "--commit", SHA, "--restore-data"], /belongs to rollback/);
  });
  it("--expect-tree is for the tool only and must be a full tree sha", () => {
    const r = rig();
    usage(r, ["deploy", "--commit", SHA, "--expect-tree", TREE], /--expect-tree applies to --engine tool only/);
    usage(r, ["deploy", "--commit", SHA, "--engine", "tool", "--expect-tree", "abc"], /40-hex tree sha/);
  });
  it("--commit still has to be a full sha", () => {
    const r = rig();
    usage(r, ["deploy", "--commit", "abc", "--engine", "tool"], /40-hex/);
  });
});
