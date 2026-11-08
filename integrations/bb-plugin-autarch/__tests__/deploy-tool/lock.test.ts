import { afterEach, describe, expect, it } from "vitest";
import { closeSync, existsSync, openSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { LOCK_WAIT_CAP_SEC, candidateFd, fdIsLockFile, lockWaitSec } from "../../../../tools/bb-plugin-deploy/lib/lock.mjs";
import { Rig, waitFor, type CliResult, type Json } from "./rig";

let rig: Rig;
const procs: ChildProcess[] = [];
afterEach(() => {
  for (const p of procs.splice(0)) p.kill("SIGKILL");
  rig?.cleanup();
});

const deployArgs = (sha: string, mode = "agent") => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const lockPath = () => join(rig.stateDir, "lock");

/** Resolve with the exit status once the child ends; registered immediately so the event cannot be missed. */
const exited = (p: ChildProcess): Promise<number | null> => new Promise((res) => p.once("exit", (code) => res(code)));

/** A separate process that holds the lock on fd 9 (the runner's way) until killed. */
function holder(): ChildProcess {
  const p = spawn("bash", ["-c", 'exec 9>"$1"; flock -n 9 && echo held; sleep 120', "holder", lockPath()], { stdio: ["ignore", "pipe", "ignore"] });
  procs.push(p);
  return p;
}
function holding(): ChildProcess {
  const p = holder();
  // the event loop is blocked by waitFor, so probe the lock itself: held once a non-blocking flock on it fails
  const heldNow = () => existsSync(lockPath()) && spawnSync("flock", ["-n", lockPath(), "true"]).status !== 0;
  expect(waitFor(heldNow, 5000)).toBe(true);
  return p;
}

/** The runner's exact lock pattern, then `body`. */
const runnerScript = (body: string): string => `exec 9>"$1"; flock -n 9 || exit 0\n${body}`;

/** Run bash with the rig's env; the tool's command line is appended by the caller via "$@" after the lock path. */
function bash(script: string, args: string[], env: Record<string, string | undefined> = {}) {
  const r = spawnSync("bash", ["-c", script, "bash", ...args], { encoding: "utf8", env: rig.env(env), timeout: 60_000 });
  return { status: r.status, stdout: r.stdout, stderr: r.stderr };
}
const cliArgs = (): string[] => {
  const root = rig.liveRoot() ?? "";
  return [join(root, "tools", "bb-plugin-deploy", "cli.mjs"), "record", ...rig.common()];
};

describe("lock helpers", () => {
  it("the wait cap is 35 minutes and the test hook can only lower it", () => {
    expect(LOCK_WAIT_CAP_SEC).toBe(2100);
    expect(lockWaitSec({})).toBe(2100);
    expect(lockWaitSec({ BPD_LOCK_WAIT_SEC: "5" })).toBe(5);
    expect(lockWaitSec({ BPD_LOCK_WAIT_SEC: "0" })).toBe(0);
    for (const v of ["2100", "99999", "-1", "abc", "", "1.5e9"]) expect(lockWaitSec({ BPD_LOCK_WAIT_SEC: v }), v).toBe(2100);
  });

  it("the candidate descriptor is BPD_LOCK_FD when it is digits above 2, else 9", () => {
    expect(candidateFd({})).toBe(9);
    expect(candidateFd({ BPD_LOCK_FD: "17" })).toBe(17);
    for (const v of ["2", "0", "x", "1 7", "-4", "", "17;id"]) expect(candidateFd({ BPD_LOCK_FD: v }), v).toBe(9);
  });

  it("a descriptor counts only when it is open on the lock file itself", () => {
    rig = new Rig({ installedKind: "stamped" });
    const other = join(rig.root, "other-file");
    writeFileSync(other, "");
    writeFileSync(lockPath(), "");
    const fdLock = openSync(lockPath(), "a");
    const fdOther = openSync(other, "a");
    try {
      expect(fdIsLockFile(fdLock, lockPath())).toBe(true);
      expect(fdIsLockFile(fdOther, lockPath())).toBe(false);
    } finally {
      closeSync(fdLock);
      closeSync(fdOther);
    }
    expect(fdIsLockFile(fdLock, lockPath())).toBe(false);
    expect(fdIsLockFile(9999, lockPath())).toBe(false);
  });
});

describe("one tool run excludes the runner and other tool runs", () => {
  it("while a deploy is mid-transaction, a runner tick exits without acting and a second tool run reports the holder", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ pauseAfter: "disable" });
    const deploy = rig.spawnCli(deployArgs(Y));
    const done = exited(deploy);
    const paused = join(rig.instances[0].stateDir, "paused");
    expect(waitFor(() => existsSync(paused), 20_000)).toBe(true);

    const runnerLog = join(rig.root, "runner.log");
    const sh = runnerScript(`echo ran >> "${runnerLog}"`);
    const tick = bash(sh, [lockPath()]);
    expect(tick.status).toBe(0);
    expect(existsSync(runnerLog)).toBe(false);

    const second = rig.cli(["rollback", "--to", "previous"], { env: { BPD_LOCK_WAIT_SEC: "1" } });
    expect(second.status, msg(second)).toBe(75);
    expect(msg(second)).toMatch(/another deploy holds the lock \(pid \d+\)/);

    const third = rig.cli(deployArgs(Y), { env: { BPD_LOCK_WAIT_SEC: "1" } });
    expect(third.status, msg(third)).toBe(75);

    writeFileSync(join(rig.instances[0].stateDir, "release"), "");
    expect(await done).toBe(0);
    expect(rig.installedSha()).toBe(Y);
    // the lock is free again: the runner pattern now runs
    expect(bash(sh, [lockPath()]).status).toBe(0);
    expect(existsSync(runnerLog)).toBe(true);
  }, 90_000);

  it("a run that waits for a holder proceeds once the holder lets go", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const h = holding();
    const waiter = rig.spawnCli(["record"], { env: { BPD_LOCK_WAIT_SEC: "60" } });
    const done = exited(waiter);
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 500);
    expect(waiter.exitCode).toBeNull();
    h.kill("SIGKILL");
    expect(await done).toBe(0);
  }, 90_000);

  it("a lock that stays held past the wait is reported as busy and nothing is mutated", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const t0 = Date.now();
    const r = rig.cli(deployArgs(Y), { env: { BPD_LOCK_WAIT_SEC: "1" } });
    expect(r.status, msg(r)).toBe(75);
    expect(Date.now() - t0).toBeLessThan(30_000);
    expect(rig.calls()).toEqual([]);
  });
});

describe("inheritance is by descriptor, never by environment", () => {
  it("a runner holding fd 9 that starts the tool: the tool runs inherited, with no wait", () => {
    rig = new Rig({ installedKind: "stamped" });
    const node = process.execPath;
    const r = bash(runnerScript(`"${node}" "$2" "\${@:3}"`), [lockPath(), ...cliArgs()], { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status, r.stderr).toBe(0);
  });

  it("the same call from a process that does not hold the lock is busy when another process holds it", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    const r = bash(`"${process.execPath}" "$1" "\${@:2}"`, cliArgs(), { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status).toBe(75);
  });

  it("a holder on fd 17 with BPD_LOCK_FD=17 is inherited, with no wait", () => {
    rig = new Rig({ installedKind: "stamped" });
    const r = bash(`exec 17>"$1"; flock -n 17 || exit 9; export BPD_LOCK_FD=17; "${process.execPath}" "$2" "\${@:3}"`, [lockPath(), ...cliArgs()], { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status, r.stderr).toBe(0);
  });

  it("a forged BPD_LOCK_FD with no descriptor behind it opens its own and waits", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    const r = bash(`export BPD_LOCK_FD=17; "${process.execPath}" "$1" "\${@:2}"`, cliArgs(), { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status).toBe(75);
  });

  it("a BPD_LOCK_FD naming a descriptor on some other file is not trusted", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    const other = join(rig.root, "not-the-lock");
    const r = bash(`exec 17>"$1"; export BPD_LOCK_FD=17; "${process.execPath}" "$2" "\${@:3}"`, [other, ...cliArgs()], { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status).toBe(75);
  });

  it("fd 9 open on another file is not the lock either", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    const other = join(rig.root, "not-the-lock");
    const r = bash(`exec 9>"$1"; "${process.execPath}" "$2" "\${@:3}"`, [other, ...cliArgs()], { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status).toBe(75);
  });

  it("a forged BPD_LOCK_FD when the lock is free just takes the lock", () => {
    rig = new Rig({ installedKind: "stamped" });
    const r = bash(`export BPD_LOCK_FD=17; "${process.execPath}" "$1" "\${@:2}"`, cliArgs(), { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status, r.stderr).toBe(0);
  });

  it("a descriptor open on the lock file but not locked by this tree does not count as held", () => {
    rig = new Rig({ installedKind: "stamped" });
    holding();
    // fd 9 is open on the real lock file, but nobody in this process tree holds the flock: it must wait, then give up
    const r = bash(`exec 9>>"$1"; "${process.execPath}" "$2" "\${@:3}"`, [lockPath(), ...cliArgs()], { BPD_LOCK_WAIT_SEC: "0" });
    expect(r.status).toBe(75);
  });
});

describe("the tool's own subprocesses run inherited", () => {
  const innerRecord = (planFile: string): string[] => [
    "env",
    "BPD_LOCK_WAIT_SEC=1",
    "BB_PLUGIN_DEPLOY_BB={stateDir}/../stub/bb",
    "node",
    "{installedRoot}/tools/bb-plugin-deploy/cli.mjs",
    "record",
    "--plugin-file",
    planFile,
    "--bb-data",
    "{bbData}",
    "--state-dir",
    "{stateDir}",
    "--json",
  ];
  const plugin = (planFile: string): Json => ({ preflight: [{ name: "inner-tool", run: innerRecord(planFile) }] });

  it("a check that starts the tool again takes the inherited lock instead of waiting on itself", () => {
    rig = new Rig({ installedKind: "stamped", plugin: plugin("{installedPluginDir}/deploy.json") });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { env: { BPD_LOCK_WAIT_SEC: "30" } });
    expect(r.status, msg(r)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
  });

  it("the inner run really executes: a failing inner call stops the deploy", () => {
    rig = new Rig({ installedKind: "stamped", plugin: plugin("{installedPluginDir}/missing.json") });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { env: { BPD_LOCK_WAIT_SEC: "30" } });
    expect(r.status, msg(r)).toBe(1);
    expect(msg(r)).toMatch(/preflight check inner-tool failed/);
    expect(msg(r)).toMatch(/cannot read the plugin file/);
    expect(rig.mutations()).toEqual([]);
  });
});
