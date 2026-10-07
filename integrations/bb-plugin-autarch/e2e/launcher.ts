// The `--owned-server` launcher (Task 2.12 part B, plan A7 steps 1-6). The harness starts its own bb-app from a COPY
// of the installed app, on an empty temp data dir, then proves it owns the listening socket before any request
// reaches it. It runs only inside `bwrap --unshare-net` (requireNetns is checked by the harness before this runs).
//
// Fresh-data-dir facts that shaped this file (Aleph's diagnosis, mk-2ayy): the data dir must be writable with room to
// spare (a tiny tmpfs makes the migrator abort with SQLITE_FULL, and drizzle masks that as "no transaction is
// active"), so the launcher refuses a data dir with less than MIN_FREE_BYTES free. A fresh bb has no `tasks`
// plugin installed; the launcher installs the bundled one (after the socket proof, so no request precedes it).
import { cpSync, existsSync, mkdirSync, mkdtempSync, readdirSync, realpathSync, statfsSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import type { ChildProcess } from "node:child_process";
import { checkRuntimeFile, listeners, nonceRoundTrip, OwnershipError, ownedSet, proveSocket, type RigEvidence } from "./ownership.js";
import { freePort } from "./preflight.js";
import { makeTarget, rigExec, rigSpawn, type RigTarget } from "./rigexec.js";

export const MIN_FREE_BYTES = 50 * 1024 * 1024;
const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export class LauncherError extends Error {
  constructor(message: string) {
    super(`launcher: ${message}`);
    this.name = "LauncherError";
  }
}

/** The app must be a copy: never the live install under ~/.bb-machines or ~/.bb, and it must hold the entry points. */
export function checkApp(app: string): { app: string; bbApp: string; bbCli: string } {
  let real: string;
  try {
    real = realpathSync(app);
  } catch {
    throw new LauncherError(`HOME_E2E_BB_APP ${app} does not exist`);
  }
  if (/(^|\/)\.bb(-dev|-machines)?(\/|$)/.test(real)) throw new LauncherError(`${real} is inside a live bb directory: the launcher runs a copy, never the installed app`);
  const bbApp = join(real, "dist", "bb-app.js");
  const bbCli = join(real, "dist", "bb.js");
  if (!existsSync(join(real, "package.json")) || !existsSync(bbApp) || !existsSync(bbCli)) throw new LauncherError(`${real} is not a bb-app package (package.json, dist/bb-app.js, dist/bb.js)`);
  return { app: real, bbApp, bbCli };
}

/** The data dir must be empty and have room: a full or tiny dir aborts the first migration with a masked error. */
export function checkDataDir(dir: string, minFree = MIN_FREE_BYTES): number {
  if (readdirSync(dir).length > 0) throw new LauncherError(`data dir ${dir} is not empty`);
  const s = statfsSync(dir);
  const free = Number(s.bavail) * Number(s.bsize);
  if (free < minFree) throw new LauncherError(`data dir ${dir} has ${free} bytes free, under the ${minFree} the first migration needs (a smaller dir fails with SQLITE_FULL, shown by drizzle as "cannot rollback")`);
  return free;
}

/**
 * The plugin build toolchain (esbuild, tailwind: ~30 MB of node_modules) is downloaded with npm on the first plugin
 * install, and the netns has no network, so the owned data dir would never build the autarch plugin. A seed is a
 * pre-fetched toolchain-<pins> directory copied into <data>/plugins before the server starts: code only, no data, and
 * no network. Refuses anything that is not such a directory or that lives in a live bb directory's data (the seed is
 * read, never written).
 */
export function seedToolchain(dataDir: string, seed: string): string {
  const real = realpathSync(seed);
  if (!/^toolchain-[\w.-]+$/.test(basename(real))) throw new LauncherError(`toolchain seed ${real} is not a toolchain-<pins> directory`);
  if (!existsSync(join(real, ".bb-toolchain.json")) || !existsSync(join(real, "node_modules", "esbuild"))) throw new LauncherError(`toolchain seed ${real} is incomplete (.bb-toolchain.json, node_modules/esbuild)`);
  const dest = join(dataDir, "plugins", basename(real));
  mkdirSync(join(dataDir, "plugins"), { recursive: true });
  cpSync(real, dest, { recursive: true });
  return dest;
}

export interface OwnedServer {
  target: RigTarget;
  base: string;
  app: string;
  bbCli: string;
  bbApp: string;
  launcher: ChildProcess;
  rig: Omit<RigEvidence, "stopped">;
  /** Stop through `bb-app stop`, then require every pid of the owned set gone (SIGKILL after 10 s). */
  stop(): Promise<{ stopped: boolean; killed: number[]; stop_exit: number | null }>;
}

const alive = (pid: number) => existsSync(`/proc/${pid}`);

export async function launchOwned(o: { app: string; netnsIsolated: boolean; startTimeoutMs?: number; toolchainSeed?: string }): Promise<OwnedServer> {
  const { app, bbApp, bbCli } = checkApp(o.app);
  const base = mkdtempSync(join(tmpdir(), "autarch-e2e-owned-"));
  const dataDir = join(base, "data");
  const home = join(base, "home");
  mkdirSync(dataDir, { recursive: true });
  checkDataDir(dataDir);
  if (o.toolchainSeed) seedToolchain(dataDir, o.toolchainSeed);
  const port = await freePort();
  const hostPort = await freePort();
  const target = makeTarget({ url: `http://127.0.0.1:${port}`, dataDir, home, hostPort: String(hostPort), realBb: bbCli, requireProof: true });
  // makeTarget wrote the nonce file: the data dir is "empty" apart from it, which is the rig's own marker.
  const launcher = rigSpawn("node", [bbApp, "start", "--data-dir", dataDir, "--server-bind-host", "127.0.0.1", "--server-port", String(port), "--host-daemon-port", String(hostPort), "--bundled"], { target, stdout: "ignore" });
  const L = launcher.pid;
  if (!L) throw new LauncherError("the launcher did not start");
  let exited: number | null | undefined;
  launcher.once("exit", (c) => (exited = c));
  const stop = async () => {
    const before = ownedSet(L);
    const r = await rigExec("node", [bbApp, "stop", "--data-dir", dataDir], { target, timeoutMs: 60_000 }).catch(() => ({ code: null as number | null }));
    const end = Date.now() + 10_000;
    while (Date.now() < end && before.some(alive)) await sleep(200);
    const left = [...new Set([...before, ...ownedSet(L)])].filter(alive);
    for (const p of left) {
      try {
        process.kill(p, "SIGKILL");
      } catch {
        /* gone */
      }
    }
    if (left.length) await sleep(500);
    return { stopped: [...before].every((p) => !alive(p)), killed: left, stop_exit: r.code };
  };
  try {
    // Step 3: wait for a LISTEN socket on the port (a read of /proc, no request), then prove it is ours.
    const deadline = Date.now() + (o.startTimeoutMs ?? 120_000);
    while (!listeners().some((e) => e.port === port)) {
      if (exited !== undefined) throw new LauncherError(`the launcher exited (${exited}) before listening`);
      if (Date.now() > deadline) throw new LauncherError(`nothing listened on port ${port} within the timeout`);
      await sleep(200);
    }
    const proof = proveSocket(L, port);
    // Step 4: the runtime file (written just after the listener) names exactly the launcher and the port.
    let runtimePid = -1;
    for (;;) {
      try {
        runtimePid = checkRuntimeFile(dataDir, L, port);
        break;
      } catch (e) {
        if (!(e instanceof OwnershipError) || !/cannot read/.test(e.message) || Date.now() > deadline) throw e;
        await sleep(200);
      }
    }
    target.proof = proof;
    // A fresh bb has no tasks plugin: install the bundled one, then step 5 (the nonce round trip) once it answers.
    const inst = await rigExec("bb", ["plugin", "install", "builtin:tasks", "--yes"], { target, timeoutMs: 120_000 });
    if (inst.code !== 0) throw new LauncherError(`installing the bundled tasks plugin failed (${inst.code}): ${inst.stderr || inst.stdout}`);
    let ok = false;
    for (let last = ""; !ok; await sleep(500)) {
      try {
        ok = await nonceRoundTrip(target);
        if (!ok) throw new OwnershipError(`no tasks project e2e-${target.nonce} in the tasks data.db`);
      } catch (e) {
        last = e instanceof Error ? e.message : String(e);
        if (e instanceof OwnershipError || Date.now() > deadline) throw new LauncherError(`nonce round trip failed: ${last}`);
      }
    }
    return { target, base, app, bbCli, bbApp, launcher, rig: { ...proof, runtime_file_pid: runtimePid, nonce_in_tasks_db: true, netns_isolated: o.netnsIsolated }, stop };
  } catch (e) {
    await stop().catch(() => undefined);
    throw e;
  }
}
