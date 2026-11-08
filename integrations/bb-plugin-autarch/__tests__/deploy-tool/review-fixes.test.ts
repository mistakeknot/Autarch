import { afterEach, describe, expect, it } from "vitest";
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { bbBinary } from "../../../../tools/bb-plugin-deploy/lib/bb.mjs";
import { validatePluginObject } from "../../../../tools/bb-plugin-deploy/lib/pluginfile.mjs";
import { Rig, defaultPlugin, waitFor, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, mode = "agent") => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const stateOf = () => rig.instances[0].stateDir;
const exited = (p: { once: (e: "exit", f: (c: number | null) => void) => unknown }): Promise<number | null> => new Promise((res) => p.once("exit", (c) => res(c)));

describe("a control-path pause stops before any build command runs", () => {
  const buildLog = (): string => join(rig.stateDir, "build-ran.log");
  const plugin = { build: [["bash", "-c", 'echo ran >> "$1/build-ran.log"', "x", "{stateDir}"]] };

  it("a paused deploy runs no build step and leaves no build directory", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const before = rig.buildDirs().length;
    const Y = rig.commit({ "scripts/home-gates.sh": `${readFileSync(join(rig.upstream, "scripts", "home-gates.sh"), "utf8")}\n# changed\n` });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(20);
    expect(existsSync(buildLog())).toBe(false);
    expect(rig.buildDirs()).toHaveLength(before);
  });

  it("an unpaused deploy does run the build step", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(readFileSync(buildLog(), "utf8")).toBe("ran\n");
  });
});

describe("the in-progress transaction is on disk before the first mutating call", () => {
  it("record.json names the transaction, its phase and the anchored previous build while bb is mid-install", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ pauseAfter: "install" });
    const p = rig.spawnCli(deployArgs(Y));
    const done = exited(p);
    expect(waitFor(() => existsSync(join(stateOf(), "paused")), 20_000)).toBe(true);
    const mid = rig.record() as { previous: { commit: string } | null; transaction?: { state: string; phase: string; commit: string; previous: { commit: string } } };
    expect(mid.transaction).toMatchObject({ state: "mutating", phase: "install", commit: Y });
    expect(mid.transaction?.previous.commit).toBe(rig.W);
    expect(mid.previous?.commit).toBe(rig.W);
    writeFileSync(join(stateOf(), "release"), "");
    expect(await done).toBe(0);
    expect((rig.record() as { transaction?: unknown }).transaction).toBeUndefined();
  }, 60_000);

  it("after a crash mid-deploy, reconcile keeps the interrupted deploy's anchor as the previous build", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ pauseAfter: "install" });
    const p = rig.spawnCli(deployArgs(Y));
    const done = exited(p);
    expect(waitFor(() => existsSync(join(stateOf(), "paused")), 20_000)).toBe(true);
    p.kill("SIGKILL");
    await done;
    // bb now runs Y's build as a plain install; the interrupted record is still there
    expect((rig.record() as { transaction?: unknown }).transaction).toBeDefined();
    writeFileSync(join(stateOf(), "release"), "");
    rig.install(rig.makeBuild(Y, "git"));
    const r = rig.cli(["record", "--reconcile"]);
    expect(r.status, msg(r)).toBe(0);
    const after = rig.record() as { installed: { commit: string }; previous: { commit: string }; transaction?: unknown; history: { detail: string }[] };
    expect(after.installed.commit).toBe(Y);
    expect(after.previous.commit).toBe(rig.W);
    expect(after.transaction).toBeUndefined();
    expect(after.history.at(-1)?.detail).toMatch(/interrupted deploy/);
  }, 60_000);
});

describe("a failed record write after a healthy install is rolled back", () => {
  it("installed.sha that cannot be written makes the deploy exit 30 with the previous build live", () => {
    const sabotage = { kind: "check", name: "sabotage", run: ["bash", "-c", "rm -f {stateDir}/installed.sha; mkdir {stateDir}/installed.sha"] };
    rig = new Rig({ installedKind: "stamped", plugin: { health: [{ kind: "running", timeoutSec: 3 }, sabotage] } });
    const wSource = rig.source();
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(rig.source()).toBe(wSource);
    expect((rig.record() as { installed: { commit: string } }).installed.commit).toBe(rig.W);
  });
});

describe("the plugin file must be a tracked, unmodified file of the installed build", () => {
  // deploy.json is deliberately not a control path here, so the closure check cannot be what refuses it.
  const plugin = { controlPaths: ["tools/bb-plugin-deploy/**", "scripts/home-gates.sh"] };

  it("an edited plugin file in the installed build is refused", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const pf = join(rig.liveRoot() ?? "", "plug", "deploy.json");
    const edited = JSON.parse(readFileSync(pf, "utf8")) as Record<string, unknown>;
    edited.retention = { builds: 99 };
    chmodSync(pf, 0o644);
    writeFileSync(pf, JSON.stringify(edited));
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/differs from/);
    expect(rig.mutations()).toEqual([]);
  });

  it("an untracked stand-in file in the installed build is refused", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const eng = rig.liveRoot() ?? "";
    const alt = join(eng, "plug", "alt.json");
    writeFileSync(alt, readFileSync(join(eng, "plug", "deploy.json")));
    const argv = [join(eng, "tools", "bb-plugin-deploy", "cli.mjs"), ...deployArgs(Y), "--plugin-file", alt, "--bb-data", rig.instances[0].bbData, "--state-dir", rig.stateDir, "--json"];
    const t = spawnSync(process.execPath, argv, { encoding: "utf8", env: rig.env() });
    expect(t.status, t.stdout + t.stderr).toBe(10);
    expect(t.stdout + t.stderr).toMatch(/not tracked/);
    expect(rig.mutations()).toEqual([]);
  });

  it("an untouched tracked plugin file deploys", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
  });
});

describe("reserved data and export names", () => {
  const base = () => defaultPlugin("/nonexistent") as Record<string, unknown>;
  const dataNamed = (name: string) => ({ ...base(), data: [{ name, kind: "file", path: "{bbData}/x" }] });
  const exportNamed = (name: string) => ({ ...base(), export: [{ name, run: ["true"], required: false }] });
  const problems = (o: unknown): string => JSON.stringify((validatePluginObject as (x: unknown) => unknown)(o));

  it.each(["manifest.json", "previous-source.txt", "export-moves.json", ".", ".."])("data name %s is rejected", (n) => {
    expect(problems(dataNamed(n))).toMatch(/reserved/);
  });
  it.each([".", ".."])("export name %s is rejected", (n) => {
    expect(problems(exportNamed(n))).toMatch(/reserved/);
  });
  it("ordinary names are accepted", () => {
    expect(problems(dataNamed("data.db"))).not.toMatch(/reserved/);
    expect(problems(exportNamed("moves"))).not.toMatch(/reserved/);
  });
});

describe("restore-data validates the backup manifest", () => {
  const backupOf = (): string => (rig.record() as { installed: { backup: string } }).installed.backup;
  const deployed = () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
  };
  const check = () => rig.cli(["restore-data", "--backup", backupOf(), "--check"]);

  it("a good backup passes", () => {
    deployed();
    expect(check().status).toBe(0);
  });
  it("a missing manifest is refused", () => {
    deployed();
    spawnSync("rm", ["-f", join(backupOf(), "manifest.json")]);
    expect(check().status).toBe(10);
  });
  it("a manifest for another plugin is refused", () => {
    deployed();
    const f = join(backupOf(), "manifest.json");
    const m = JSON.parse(readFileSync(f, "utf8")) as Record<string, unknown>;
    m.pluginId = "someone-else";
    writeFileSync(f, JSON.stringify(m));
    expect(check().status).toBe(10);
  });
  it("a data file that no longer matches its manifest digest is refused even if it is a valid database", () => {
    deployed();
    const db = join(backupOf(), "data.db");
    spawnSync("sqlite3", [db, "INSERT INTO t VALUES ('tampered')"]);
    const r = check();
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/digest/);
  });
  it("a previous-source.txt that disagrees with the manifest is refused", () => {
    deployed();
    writeFileSync(join(backupOf(), "previous-source.txt"), `${rig.root}\n`);
    const r = check();
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/disagrees/);
  });
});

describe("once-per-commit backups", () => {
  it("a second backup for the same commit restores through the reference to the first", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { data: [{ name: "data.db", kind: "sqlite", path: "{bbData}/plugins/fixture/data.db", backup: "once-per-commit" }] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    // fail the first attempt after the backup so a second attempt at the same commit skips the copy
    rig.inject({ failInstallMatch: Y.slice(0, 12) });
    expect(rig.cli(deployArgs(Y)).status).toBe(30);
    rig.inject({ failInstallMatch: "" });
    // build directories are stamped to the second; let the stamp move on so the retry does not collide
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 1100);
    const second = rig.cli(deployArgs(Y));
    expect(second.status, msg(second)).toBe(0);
    const backup = (rig.record() as { installed: { backup: string } }).installed.backup;
    const m = JSON.parse(readFileSync(join(backup, "manifest.json"), "utf8")) as { data: { skipped?: string; ref?: string }[] };
    expect(m.data[0].skipped).toBe("once-per-commit");
    const r = rig.cli(["restore-data", "--backup", backup, "--check"]);
    expect(r.status, msg(r)).toBe(0);
  });
});

describe("log-line baselines are taken before the command they belong to", () => {
  it("a line written by bb install itself counts for since: install", () => {
    const plugin = { health: [{ kind: "running", timeoutSec: 3 }, { kind: "log-line", name: "ready", file: "{stateDir}/app.log", pattern: "ready", since: "install", timeoutSec: 1 }] };
    rig = new Rig({ installedKind: "stamped", plugin });
    rig.inject({ appendOn: { install: [{ file: join(rig.stateDir, "app.log"), text: "ready from install\n" }] } });
    writeFileSync(join(rig.stateDir, "app.log"), "");
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
  });
});

describe("the last-resort recovery block is printed on success", () => {
  it("a successful deploy's text output carries the recovery block", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const eng = rig.liveRoot() ?? "";
    const argv = [join(eng, "tools", "bb-plugin-deploy", "cli.mjs"), ...deployArgs(Y), "--plugin-file", join(eng, "plug", "deploy.json"), "--bb-data", rig.instances[0].bbData, "--state-dir", rig.stateDir];
    const t = spawnSync(process.execPath, argv, { encoding: "utf8", env: rig.env() });
    expect(t.status, t.stdout + t.stderr).toBe(0);
    expect(t.stdout).toContain("Last-resort recovery (if this tool cannot run):");
  });
});

describe("bb binary resolution", () => {
  const exe = (p: string): string => {
    mkdirSync(join(p, ".."), { recursive: true });
    writeFileSync(p, "#!/bin/sh\n");
    chmodSync(p, 0o755);
    return p;
  };
  it("prefers BB_PLUGIN_DEPLOY_BB, then BB_CLI, then ~/.local/bin/bb, then the caller's PATH, then bare bb", () => {
    rig = new Rig({ installedKind: "stamped" });
    const home = join(rig.root, "home");
    const pathBb = exe(join(rig.root, "pathdir", "bb"));
    const localBb = exe(join(home, ".local", "bin", "bb"));
    const cliBb = exe(join(rig.root, "cli", "bb"));
    const env = { HOME: home, PATH: join(rig.root, "pathdir"), BB_CLI: cliBb, BB_PLUGIN_DEPLOY_BB: "/explicit/bb" };
    expect(bbBinary(env)).toBe("/explicit/bb");
    expect(bbBinary({ ...env, BB_PLUGIN_DEPLOY_BB: undefined })).toBe(cliBb);
    expect(bbBinary({ ...env, BB_PLUGIN_DEPLOY_BB: undefined, BB_CLI: undefined })).toBe(localBb);
    expect(bbBinary({ HOME: join(rig.root, "nohome"), PATH: join(rig.root, "pathdir") })).toBe(pathBb);
    expect(bbBinary({ HOME: join(rig.root, "nohome"), PATH: "/nonexistent" })).toBe("bb");
    expect(bbBinary({ ...env, BB_PLUGIN_DEPLOY_BB: undefined, BB_CLI: join(rig.root, "missing", "bb") })).toBe(localBb);
  });
});
