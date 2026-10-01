// The owned-server proof (plan A7, steps 3-6 of Task 2.12's rig). The functions here are the harness's
// own checks; they read /proc and the data dir and open no socket. Order matters and is enforced by
// proveOwnedServer: the socket inode is matched against the descendants' /proc/<pid>/fd entries and the
// runtime file is cross-checked BEFORE the nonce round-trip (the first rigRpc).
import Database from "better-sqlite3";
import { existsSync, readdirSync, readFileSync, readlinkSync } from "node:fs";
import { join } from "node:path";
import { assertOwned, rigRpc, type OwnershipProof, type RigTarget } from "./rigexec.js";

export class OwnershipError extends Error {
  constructor(message: string) {
    super(`ownership: ${message}`);
    this.name = "OwnershipError";
  }
}

export interface TcpEntry {
  port: number;
  state: string;
  inode: number;
}

/** Parse /proc/net/tcp or tcp6 (hex local address:port, state, inode in column 10). */
export function parseProcNetTcp(text: string): TcpEntry[] {
  const out: TcpEntry[] = [];
  for (const line of text.split("\n").slice(1)) {
    const f = line.trim().split(/\s+/);
    if (f.length < 10) continue;
    const port = parseInt(f[1]!.split(":")[1] ?? "", 16);
    if (Number.isNaN(port)) continue;
    out.push({ port, state: f[3]!, inode: Number(f[9]) });
  }
  return out;
}

const readTcp = (proc: string): TcpEntry[] => ["tcp", "tcp6"].flatMap((n) => (existsSync(join(proc, "net", n)) ? parseProcNetTcp(readFileSync(join(proc, "net", n), "utf8")) : []));
export const listeners = (proc = "/proc"): TcpEntry[] => readTcp(proc).filter((e) => e.state === "0A");

/** Pids in /proc with their parent, from /proc/<pid>/stat (the comm field may hold spaces and parentheses). */
function parents(proc: string): Map<number, number> {
  const m = new Map<number, number>();
  for (const name of readdirSync(proc)) {
    if (!/^\d+$/.test(name)) continue;
    try {
      const stat = readFileSync(join(proc, name, "stat"), "utf8");
      const after = stat.slice(stat.lastIndexOf(")") + 2).split(" ");
      m.set(Number(name), Number(after[1])); // state, ppid
    } catch {
      /* exited */
    }
  }
  return m;
}

/** {L} plus every descendant of L, by ppid. L is included because the launcher may serve the port itself. */
export function ownedSet(launcher: number, proc = "/proc"): number[] {
  const par = parents(proc);
  const owned = new Set<number>([launcher]);
  for (let grew = true; grew; ) {
    grew = false;
    for (const [pid, ppid] of par) if (!owned.has(pid) && owned.has(ppid)) (owned.add(pid), (grew = true));
  }
  return [...owned].sort((a, b) => a - b);
}

/** inode -> pid for every socket:[N] fd link of the given pids. */
export function socketHolders(pids: number[], proc = "/proc"): Map<number, number> {
  const held = new Map<number, number>();
  for (const pid of pids) {
    let fds: string[];
    try {
      fds = readdirSync(join(proc, String(pid), "fd"));
    } catch {
      continue;
    }
    for (const fd of fds) {
      try {
        const m = /^socket:\[(\d+)\]$/.exec(readlinkSync(join(proc, String(pid), "fd", fd)));
        if (m && !held.has(Number(m[1]))) held.set(Number(m[1]), pid);
      } catch {
        /* closed meanwhile */
      }
    }
  }
  return held;
}

/**
 * Step 3: the LISTEN socket on `port` must be the only one, and its inode must be among the owned set's
 * fd links. Returns the proof (nothing is trusted from the runtime file here). Throws OwnershipError.
 */
export function proveSocket(launcher: number, port: number, proc = "/proc"): Omit<OwnershipProof, "launcher_pid"> & { launcher_pid: number } {
  const onPort = listeners(proc).filter((e) => e.port === port);
  if (onPort.length === 0) throw new OwnershipError(`no LISTEN socket on port ${port}`);
  const inodes = new Set(onPort.map((e) => e.inode));
  if (inodes.size !== 1) throw new OwnershipError(`more than one LISTEN socket on port ${port}: inodes ${[...inodes].join(",")}`);
  const inode = [...inodes][0]!;
  const owned = ownedSet(launcher, proc);
  const holder = socketHolders(owned, proc).get(inode);
  if (holder === undefined) throw new OwnershipError(`LISTEN inode ${inode} on port ${port} is held by no process of the owned set ${owned.join(",")}`);
  return { launcher_pid: launcher, socket_owner_pid: holder, listen_inode: inode, listen_inode_in_owned_set: true, owned_set: owned };
}

/** Step 4: `<data>/bb-app-runtime.json` must name exactly the launcher pid and the chosen port. It never identifies the socket. */
export function checkRuntimeFile(dataDir: string, launcher: number, port: number): number {
  let r: { pid?: unknown; serverUrl?: unknown };
  try {
    r = JSON.parse(readFileSync(join(dataDir, "bb-app-runtime.json"), "utf8"));
  } catch (e) {
    throw new OwnershipError(`cannot read bb-app-runtime.json: ${e instanceof Error ? e.message : String(e)}`);
  }
  if (r.pid !== launcher) throw new OwnershipError(`runtime file pid ${String(r.pid)} is not the launcher pid ${launcher}`);
  const rp = typeof r.serverUrl === "string" ? new URL(r.serverUrl).port : "";
  if (Number(rp) !== port) throw new OwnershipError(`runtime file port ${rp} is not the chosen port ${port}`);
  return r.pid;
}

/** The per-plugin DB (plugin-sdk backend-contract), never bb.db. */
export const tasksDbPath = (dataDir: string) => join(dataDir, "plugins", "tasks", "data.db");

/** Step 5, after steps 3-4: create `e2e-<nonce>` through rigRpc, then require that row in tasks' data.db, opened read-only. */
export function nonceProjectInput(nonce: string): { name: string; prefix: string; color: string } {
  // tasks createProject requires name, prefix (^[A-Z][A-Z0-9]{0,9}$) and color.
  const hex = nonce.replace(/[^0-9a-fA-F]/g, "").toUpperCase();
  return { name: `e2e-${nonce}`, prefix: `E${hex.slice(0, 9)}`, color: "#6b7280" };
}

export async function nonceRoundTrip(t: RigTarget): Promise<boolean> {
  assertOwned(t);
  const { name, prefix, color } = nonceProjectInput(t.nonce);
  await rigRpc(t, "tasks", "createProject", { name, prefix, color });
  const db = new Database(tasksDbPath(t.dataDir), { readonly: true, fileMustExist: true, timeout: 2000 });
  try {
    const row = db.prepare("SELECT 1 AS ok FROM projects WHERE name = ?").get(name);
    return row !== undefined;
  } finally {
    db.close();
  }
}

export interface RigEvidence extends OwnershipProof {
  runtime_file_pid: number;
  nonce_in_tasks_db: boolean;
  netns_isolated: boolean;
  stopped: boolean;
}

/**
 * Steps 3, 4, 5 in the required order. The nonce RPC cannot run first: assertOwned refuses an unproven
 * real-mode target, and the proof is recorded on the target only after steps 3 and 4 pass.
 */
export async function proveOwnedServer(t: RigTarget, launcher: number, port: number, o: { proc?: string; netnsIsolated: boolean }): Promise<Omit<RigEvidence, "stopped"> & { stopped?: boolean }> {
  const proof = proveSocket(launcher, port, o.proc);
  const runtimePid = checkRuntimeFile(t.dataDir, launcher, port);
  t.proof = proof;
  const nonceOk = await nonceRoundTrip(t);
  if (!nonceOk) throw new OwnershipError(`no tasks project e2e-${t.nonce} in ${tasksDbPath(t.dataDir)}`);
  return { ...proof, runtime_file_pid: runtimePid, nonce_in_tasks_db: true, netns_isolated: o.netnsIsolated };
}

// ---- network namespace ------------------------------------------------------------------------------------------

/**
 * Real-bb mode runs only inside `bwrap --unshare-net`. Refuses (before any spawn) when the namespace is the
 * outer one or unknown, or when any LISTEN socket already exists inside it. There is no fallback.
 */
export function requireNetns(env: NodeJS.ProcessEnv = process.env, proc = "/proc"): true {
  const outer = env.HOME_E2E_OUTER_NETNS;
  if (!outer) throw new OwnershipError("HOME_E2E_OUTER_NETNS is not set: run inside `bwrap --unshare-net` with the outer namespace recorded");
  const own = readlinkSync(join(proc, "self", "ns", "net"));
  if (own === outer) throw new OwnershipError(`running in the outer network namespace ${own}: refusing (no fallback to the shared namespace)`);
  const open = listeners(proc);
  if (open.length > 0) throw new OwnershipError(`a LISTEN socket already exists inside the namespace (port ${open[0]!.port})`);
  return true;
}
