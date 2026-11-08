// The destination lock (plan 2.1): <stateDir>/lock, a real flock shared with the runner and the legacy scripts.
//
// Inheritance is by file descriptor, not by pid. A flock belongs to the open file description, so a child of a
// holder shares it. The rule: take the candidate descriptor (BPD_LOCK_FD if set, else fd 9 which is the runner's),
// and if it is open and has the same device and inode as the lock file, run `flock -n <fd>` on it. Success means
// this process holds the lock through that descriptor. Failure means another description holds it, so wait on the
// same descriptor up to the cap. With no such descriptor, open our own and wait.
import { closeSync, mkdirSync, openSync, statSync } from "node:fs";
import { join } from "node:path";
import { EXIT, ToolError, run } from "./util.mjs";

export const LOCK_WAIT_CAP_SEC = 35 * 60;

/** The cap in seconds. BPD_LOCK_WAIT_SEC may lower it (used by tests); it can never raise it above 35 minutes. */
export function lockWaitSec(env = process.env) {
  const n = Number(env.BPD_LOCK_WAIT_SEC);
  return Number.isFinite(n) && n >= 0 && n < LOCK_WAIT_CAP_SEC ? Math.floor(n) : LOCK_WAIT_CAP_SEC;
}

const devIno = (st) => `${st.dev}:${st.ino}`;

/** The descriptor named by BPD_LOCK_FD (digits only) or else 9. */
export function candidateFd(env = process.env) {
  const raw = env.BPD_LOCK_FD;
  return raw !== undefined && /^[0-9]+$/.test(raw) && Number(raw) > 2 ? Number(raw) : 9;
}

/** True when descriptor `fd` of this process is open on the same file as `lockPath`. */
export function fdIsLockFile(fd, lockPath) {
  try {
    return devIno(statSync(`/proc/self/fd/${fd}`)) === devIno(statSync(lockPath));
  } catch {
    return false;
  }
}

/** flock(1) on a descriptor of this process, via a child that inherits it at the same number. */
function flockFd(fd, waitSec) {
  const argv = waitSec === 0 ? ["flock", "-n", String(fd)] : ["flock", "-w", String(waitSec), String(fd)];
  const r = run(argv, { fds: [fd], timeoutMs: (waitSec + 30) * 1000 });
  if (r.error && r.error.code === "ENOENT") throw new ToolError("flock(1) is required (util-linux) and was not found");
  return r.code === 0;
}

/** Best-effort owner hint for the timeout message; reads nothing that can fail the caller. */
function holderHint(lockPath) {
  try {
    const st = statSync(lockPath);
    const locks = run(["cat", "/proc/locks"]).stdout.split("\n");
    for (const line of locks) {
      const f = line.trim().split(/\s+/);
      if (f[1] === "FLOCK" && /:\d+$/.test(f[5] ?? "") && f[5].endsWith(`:${st.ino}`)) return `pid ${f[4]}`;
    }
  } catch {
    /* unknown */
  }
  return "pid unknown";
}

/**
 * Take (or inherit) the destination lock and hold it until release().
 * @returns {{fd:number, inherited:boolean, path:string, release():void}}
 */
export function acquireLock(stateDir, env = process.env) {
  mkdirSync(stateDir, { recursive: true, mode: 0o700 });
  const path = join(stateDir, "lock");
  const cap = lockWaitSec(env);
  const cand = candidateFd(env);
  let fd;
  let inherited = false;
  if (fdIsLockFile(cand, path)) {
    fd = cand;
    inherited = true;
  } else {
    fd = openSync(path, "a", 0o600);
  }
  // Both cases: try without waiting first (inherited success means held through that descriptor), then wait.
  let held = flockFd(fd, 0);
  if (!held) held = cap > 0 && flockFd(fd, cap);
  if (!held) {
    if (!inherited) closeSync(fd);
    throw new ToolError(`another deploy holds the lock (${holderHint(path)})`, EXIT.LOCK_BUSY);
  }
  return {
    fd,
    inherited,
    path,
    release() {
      if (!inherited) {
        try {
          closeSync(fd);
        } catch {
          /* already closed */
        }
      }
    },
  };
}
