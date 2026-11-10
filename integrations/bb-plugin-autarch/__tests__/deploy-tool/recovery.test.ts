import { afterEach, describe, expect, it } from "vitest";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { recoveryBlock, bareRecovery } from "../../../../tools/bb-plugin-deploy/lib/recovery.mjs";
import { PLUGIN_DIR, PLUGIN_ID, Rig, waitFor, type CliResult } from "./rig";

let rig: Rig;
const procs: ChildProcess[] = [];
const holderState = (): string => (existsSync(`${lockPath()}.state`) ? readFileSync(`${lockPath()}.state`, "utf8").trim() : "no marker");
afterEach(() => {
  for (const p of procs.splice(0)) p.kill("SIGKILL");
  rig?.cleanup();
});

const deployArgs = (sha: string) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", "agent"];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const lockPath = () => join(rig.stateDir, "lock");
const exited = (p: ChildProcess): Promise<number | null> => new Promise((res) => p.once("exit", (code) => res(code)));
const pausedFile = () => join(rig.instances[0].stateDir, "paused");
const releaseFile = () => join(rig.instances[0].stateDir, "release");
const sleepMs = (ms: number) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

/** Env for a bash that must find the fake `bb` on PATH. */
const bashEnv = (extra: Record<string, string | undefined> = {}) => ({ ...rig.env(extra), PATH: `${rig.stubDir}:${process.env.PATH ?? ""}` });

/** The block as the tool prints it: from a real successful deploy of Y, which anchors W (the build bb ran before). */
function deployedBlock(): { block: string; W: string; Y: string; wRoot: string; yRoot: string } {
  const wRoot = rig.liveRoot() ?? "";
  const Y = rig.commit({ "plug/app.txt": "v2\n" });
  const r = rig.cli(deployArgs(Y));
  expect(r.status, msg(r)).toBe(0);
  const rec = r.json?.recovery as { lastResort?: string; bare?: string } | undefined;
  expect(rec?.lastResort).toBeTruthy();
  expect(rig.installedSha()).toBe(Y);
  return { block: rec?.lastResort ?? "", W: rig.W, Y, wRoot, yRoot: rig.liveRoot() ?? "" };
}

/** The runner's lock pattern followed by an install of `$2` and an installed.sha write of `$3`. */
const RUNNER_INSTALL = `exec 9>"$1"; flock -n 9 || exit 0
export BB_SERVER_URL="$4"
bb plugin install --yes "$2" && printf '%s\\n' "$3" > "$5/installed.sha"`;

function runnerTick(installDir: string, sha: string): number | null {
  const r = spawnSync("bash", ["-c", RUNNER_INSTALL, "runner", lockPath(), installDir, sha, rig.instances[0].serverUrl, rig.stateDir], { encoding: "utf8", env: bashEnv() });
  return r.status;
}

/** Run a script text with bash, not waiting. `pre` runs first in the same shell (fd setup). */
function start(pre: string, block: string, env: Record<string, string | undefined> = {}): { p: ChildProcess; done: Promise<number | null>; out: () => string } {
  let buf = "";
  const p = spawn("bash", ["-c", `${pre}\n${block}`], { env: bashEnv(env), stdio: ["ignore", "pipe", "pipe"] });
  p.stdout?.on("data", (d: Buffer) => (buf += d.toString()));
  p.stderr?.on("data", (d: Buffer) => (buf += d.toString()));
  procs.push(p);
  return { p, done: exited(p), out: () => buf };
}

describe("the printed last-resort recovery block", () => {
  it("is printed with real values on a deploy report and names the lock, the anchored build and the record write", () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, wRoot } = deployedBlock();
    expect(block).toContain("bash -s <<'RECOVER'");
    expect(block).toContain(rig.instances[0].serverUrl);
    expect(block).toContain(wRoot);
    expect(block).toContain(rig.stateDir);
    expect(block).toContain("flock -w");
    expect(block).toContain("installed.sha");
    expect(block).toContain('bb plugin enable "$P"');
    expect(block).toContain(`P='${PLUGIN_ID}'`);
  });

  it("restores the anchored build, waits for it to run and writes installed.sha atomically", () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W, wRoot } = deployedBlock();
    rig.clearCalls();
    const r = spawnSync("bash", ["-c", block], { encoding: "utf8", env: bashEnv() });
    expect(r.status, r.stderr + r.stdout).toBe(0);
    expect(rig.mutations()).toEqual(["disable", "install", "enable"]);
    expect(rig.source()).toBe(join(wRoot, PLUGIN_DIR));
    expect(rig.installedSha()).toBe(W);
    expect(existsSync(join(rig.stateDir, `installed.sha.tmp.${r.pid}`))).toBe(false);
  });

  it("a bare variant has no lock and says why that is a risk", () => {
    const text = bareRecovery({ serverUrl: "http://127.0.0.1:1", pluginId: "p", anchoredBuild: "/b", pluginDirRel: "plug", rollbackLeaves: "disabled" });
    expect(text).toMatch(/RISK/);
    expect(text).not.toMatch(/flock/);
    expect(text).not.toMatch(/bb plugin enable/);
  });

  it("with rollbackLeaves disabled the block does not enable and does not poll for running", () => {
    const text = recoveryBlock({ serverUrl: "http://127.0.0.1:1", pluginId: "p", anchoredBuild: "/b", pluginDirRel: "plug", stateDir: "/s", rollbackLeaves: "disabled" });
    expect(text).not.toMatch(/bb plugin enable/);
    expect(text).not.toMatch(/is not running/);
  });

  it("quotes values: a path with a quote or a space cannot break out of the block", () => {
    const text = recoveryBlock({ serverUrl: "http://x", pluginId: "p'q", anchoredBuild: "/a b/c'd", pluginDirRel: "plug", stateDir: "/s t" });
    expect(text).toContain(`P='p'\\''q'`);
    expect(text).toContain(`B='/a b/c'\\''d'`);
    expect(text).toContain("ST='/s t'");
    expect(spawnSync("bash", ["-n"], { input: text, encoding: "utf8" }).status).toBe(0);
  });
});

describe("the block holds the destination lock from before the disable to after the write", () => {
  it("excludes a runner tick and makes a tool tick wait; nothing installs between the disable and the record write", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W, Y, wRoot } = deployedBlock();
    // Y is wanted again (the user clicked it): both a runner and the tool would act on it as soon as they may
    rig.approve(Y);
    rig.setRequest(Y, "2026-01-02T00:00:00.000Z");
    rig.clearCalls();
    rig.inject({ pauseAfter: "enable" });
    const rec = start("", block);
    expect(waitFor(() => existsSync(pausedFile()), 20_000)).toBe(true);
    expect(rig.mutations()).toEqual(["disable", "install", "enable"]);

    // a runner tick (non-blocking flock on fd 9) exits quietly: no bb call, installed.sha untouched
    const before = rig.calls().length;
    const shaBefore = rig.installedSha();
    expect(runnerTick(join(rig.liveRoot() ?? "", PLUGIN_DIR), Y)).toBe(0);
    expect(rig.calls().length).toBe(before);
    expect(rig.installedSha()).toBe(shaBefore);

    // the tool's own tick waits for the lock instead of acting
    const tick = rig.spawnCli(["update-tick"], { engineRoot: wRoot, env: { BPD_LOCK_WAIT_SEC: "120" } });
    const tickDone = exited(tick);
    sleepMs(700);
    expect(tick.exitCode).toBeNull();
    expect(rig.calls().length).toBe(before);

    writeFileSync(releaseFile(), "");
    expect(await rec.done, rec.out()).toBe(0);
    expect(await tickDone).toBe(0);

    // order of mutations: the block's disable, install W, enable; only then the tool's own work on Y
    const muts = rig.calls().filter((c) => c.args[0] === "plugin" && ["disable", "install", "enable"].includes(c.args[1] ?? ""));
    expect(muts.slice(0, 3).map((c) => c.args[1])).toEqual(["disable", "install", "enable"]);
    expect(muts[1].args.at(-1)).toBe(join(wRoot, PLUGIN_DIR));
    const yInstalls = muts.map((c, i) => ({ c, i })).filter(({ c }) => c.args[1] === "install" && c.args.at(-1) !== join(wRoot, PLUGIN_DIR));
    expect(yInstalls.length).toBeGreaterThan(0);
    expect(Math.min(...yInstalls.map((x) => x.i))).toBeGreaterThanOrEqual(3);
    expect(rig.installedSha()).toBe(Y);
    expect(W).not.toBe(Y);
  }, 120_000);

  it("after the block ends, installed.sha is the anchored commit and the next runner tick may act", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W, Y, wRoot } = deployedBlock();
    rig.inject({ pauseAfter: "enable" });
    const rec = start("", block);
    expect(waitFor(() => existsSync(pausedFile()), 20_000)).toBe(true);
    expect(runnerTick(join(rig.liveRoot() ?? "", PLUGIN_DIR), Y)).toBe(0);
    expect(rig.source()).toBe(join(wRoot, PLUGIN_DIR));
    writeFileSync(releaseFile(), "");
    expect(await rec.done, rec.out()).toBe(0);
    expect(rig.installedSha()).toBe(W);
    // the lock is free: a runner tick now runs
    const yDir = join(rig.makeBuild(Y, "git"), PLUGIN_DIR);
    expect(runnerTick(yDir, Y)).toBe(0);
    expect(rig.source()).toBe(yDir);
    expect(rig.installedSha()).toBe(Y);
  }, 120_000);
});

// A process that holds the lock on this rig's lock file until the test ends. It WAITS for the lock (up to 30 s) rather than
// trying once: the old version tried once with `flock -n`, and the test's own readiness poll (`flock -n <lock> true`, which takes
// the lock for a few milliseconds) landed in the gap between the holder creating the file and locking it in about 12 of 15 idle
// repeats, so the holder reported "busy", held nothing, and the poll then saw a free lock for the whole 20 s. Readiness is now the
// holder's own "held" marker, which no probe can disturb; the lock is then confirmed held with one probe.
function startLockHolder(): void {
  const holder = spawn("bash", ["-c", 'exec 9>>"$1"; if flock -w 30 9; then echo held > "$1.state"; else echo busy > "$1.state"; fi; exec sleep 120', "holder", lockPath()], { stdio: ["ignore", "pipe", "ignore"] });
  procs.push(holder);
  expect(waitFor(() => holderState() === "held", 20_000), `holder state: ${holderState()}`).toBe(true);
  expect(spawnSync("flock", ["-n", lockPath(), "true"]).status, "the lock file is held").not.toBe(0);
}

describe("lock inheritance in the block", () => {
  const heldBy = (fd: number, extraEnv = "") => `exec ${fd}>>"$LOCKFILE"; flock -n ${fd} || exit 9\n${extraEnv}`;

  it("fd 9 inherited from a caller that holds the lock: no wait, and an independent fd 9 runner stays excluded until the write", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W, Y, wRoot } = deployedBlock();
    rig.inject({ pauseAfter: "enable" });
    const rec = start(heldBy(9), block, { LOCKFILE: lockPath(), BPD_LOCK_WAIT_SEC: "0" });
    expect(waitFor(() => existsSync(pausedFile()), 20_000)).toBe(true);
    const calls = rig.calls().length;
    expect(runnerTick(join(rig.liveRoot() ?? "", PLUGIN_DIR), Y)).toBe(0);
    expect(rig.calls().length).toBe(calls);
    writeFileSync(releaseFile(), "");
    expect(await rec.done, rec.out()).toBe(0);
    expect(rig.installedSha()).toBe(W);
    expect(rig.source()).toBe(join(wRoot, PLUGIN_DIR));
  }, 60_000);

  it("fd 17 named by BPD_LOCK_FD is inherited with no wait while a separate fd 9 runner stays excluded", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W, Y } = deployedBlock();
    rig.inject({ pauseAfter: "enable" });
    const rec = start(heldBy(17, "export BPD_LOCK_FD=17\n"), block, { LOCKFILE: lockPath(), BPD_LOCK_WAIT_SEC: "0" });
    expect(waitFor(() => existsSync(pausedFile()), 20_000)).toBe(true);
    const calls = rig.calls().length;
    expect(runnerTick(join(rig.liveRoot() ?? "", PLUGIN_DIR), Y)).toBe(0);
    expect(rig.calls().length).toBe(calls);
    writeFileSync(releaseFile(), "");
    expect(await rec.done, rec.out()).toBe(0);
    expect(rig.installedSha()).toBe(W);
  }, 60_000);

  it("a BPD_LOCK_FD with nothing behind it opens its own descriptor and gives up when another process holds the lock", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W } = deployedBlock();
    startLockHolder();
    rig.clearCalls();
    const r = spawnSync("bash", ["-c", `export BPD_LOCK_FD=17\n${block}`], { encoding: "utf8", env: bashEnv({ BPD_LOCK_WAIT_SEC: "1" }) });
    expect(r.status, r.stderr + r.stdout).toBe(75);
    expect(rig.calls()).toEqual([]);
    expect(rig.installedSha()).not.toBe(W);
  }, 60_000);

  it("a descriptor open on some other file is not the lock: the block opens its own and waits", () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block } = deployedBlock();
    startLockHolder();
    rig.clearCalls();
    const other = join(rig.root, "not-the-lock");
    const r = spawnSync("bash", ["-c", `exec 9>"${other}"\n${block}`], { encoding: "utf8", env: bashEnv({ BPD_LOCK_WAIT_SEC: "1" }) });
    expect(r.status, r.stderr + r.stdout).toBe(75);
    expect(rig.calls()).toEqual([]);
  }, 60_000);

  it("a non-numeric BPD_LOCK_FD and BPD_LOCK_WAIT_SEC are ignored, not evaluated", () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, W } = deployedBlock();
    const marker = join(rig.root, "pwned");
    const r = spawnSync("bash", ["-c", block], { encoding: "utf8", env: bashEnv({ BPD_LOCK_FD: `x;touch ${marker}`, BPD_LOCK_WAIT_SEC: `9999999;touch ${marker}` }) });
    expect(r.status, r.stderr + r.stdout).toBe(0);
    expect(existsSync(marker)).toBe(false);
    expect(rig.installedSha()).toBe(W);
  }, 60_000);
});

describe("failure and degraded paths of the block", () => {
  it("a failed verification (wrong source) exits 76 and leaves installed.sha unchanged", async () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, Y, yRoot } = deployedBlock();
    rig.inject({ pauseAfter: "install" });
    const rec = start("", block);
    expect(waitFor(() => existsSync(pausedFile()), 20_000)).toBe(true);
    // something else moves bb's source while the block is mid-way
    rig.install(yRoot);
    writeFileSync(releaseFile(), "");
    expect(await rec.done).toBe(76);
    expect(rec.out()).toMatch(/VERIFY FAILED: source is/);
    expect(rig.installedSha()).toBe(Y);
  }, 60_000);

  it("a missing state dir prints the warning and still recovers, writing no record", () => {
    rig = new Rig({ installedKind: "stamped" });
    const wRoot = rig.liveRoot() ?? "";
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.install(rig.makeBuild(Y, "git"));
    const gone = join(rig.root, "no-such-state");
    const block = recoveryBlock({ serverUrl: rig.instances[0].serverUrl, pluginId: PLUGIN_ID, anchoredBuild: wRoot, pluginDirRel: PLUGIN_DIR, stateDir: gone });
    const r = spawnSync("bash", ["-c", block], { encoding: "utf8", env: bashEnv() });
    expect(r.status, r.stderr + r.stdout).toBe(0);
    expect(r.stdout).toContain(`WARNING: no state dir ${gone}`);
    expect(rig.source()).toBe(join(wRoot, PLUGIN_DIR));
    expect(existsSync(gone)).toBe(false);
  }, 60_000);

  it("a failing bb step stops the block (set -e) before the record is written", () => {
    rig = new Rig({ installedKind: "stamped" });
    const { block, Y } = deployedBlock();
    rig.inject({ fail: { install: true } });
    const r = spawnSync("bash", ["-c", block], { encoding: "utf8", env: bashEnv() });
    expect(r.status).not.toBe(0);
    expect(rig.installedSha()).toBe(Y);
  }, 60_000);

  it("the block read back from a rolled-back deploy report is the same shape and works", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ neverRunMatch: Y.slice(0, 12) });
    const r = rig.cli(deployArgs(Y));
    expect([30, 31]).toContain(r.status);
    const rb = r.json?.recoveryBlock as { lastResort?: string } | undefined;
    expect(rb?.lastResort).toContain("bash -s <<'RECOVER'");
    expect(rb?.lastResort).toContain(rig.stateDir);
    const ran = spawnSync("bash", ["-c", rb?.lastResort ?? ""], { encoding: "utf8", env: bashEnv() });
    expect(ran.status, ran.stderr + ran.stdout).toBe(0);
    expect(readFileSync(join(rig.stateDir, "installed.sha"), "utf8").trim()).toBe(rig.W);
  }, 60_000);
});
