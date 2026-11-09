// Shared helpers: typed errors with exit codes, argv-array process spawning, atomic writes, hashing, time.
// Zero dependencies. Nothing here ever runs a shell.
import { spawnSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import { closeSync, fsyncSync, mkdirSync, openSync, renameSync, writeSync, readFileSync, existsSync } from "node:fs";
import { dirname, join } from "node:path";

export const PROTOCOL = "bb-plugin-deploy/1";

/** Exit codes (plan 2.3): 10 refused before any install, 20 needs mk, 30/31 after install began. */
export const EXIT = Object.freeze({
  OK: 0,
  ERROR: 1,
  USAGE: 2,
  REFUSED: 10,
  NEEDS_MK: 20,
  ROLLED_BACK: 30,
  ROLLBACK_UNHEALTHY: 31,
  LOCK_BUSY: 75,
});

export class ToolError extends Error {
  /** @param {string} message @param {number} [code] @param {object} [extra] */
  constructor(message, code = EXIT.ERROR, extra = {}) {
    super(message);
    this.name = "ToolError";
    this.code = code;
    this.extra = extra;
  }
}
export const refuse = (message, extra) => new ToolError(message, EXIT.REFUSED, extra);
export const needsMk = (message, extra) => new ToolError(message, EXIT.NEEDS_MK, extra);

export const nowIso = () => new Date().toISOString();
/** A compact UTC stamp usable in a directory name: 20260102T030405Z. */
export const utcStamp = (d = new Date()) => d.toISOString().replace(/[-:]/g, "").replace(/\.\d+Z$/, "Z");
export const sha256 = (buf) => createHash("sha256").update(buf).digest("hex");
export const SHA40 = /^[0-9a-f]{40}$/;

/** Fixed, minimal search path for every child: the bb directory, node's directory, then the system ones. */
export function fixedPath(extraDirs = []) {
  const dirs = [...extraDirs, dirname(process.execPath), "/usr/local/bin", "/usr/bin", "/bin"];
  return [...new Set(dirs.filter(Boolean))].join(":");
}

/**
 * Run a program with an argv array and no shell. `fds` lists parent file descriptors that the child gets at the
 * SAME number (the lock descriptor travels this way; the environment alone is never trusted).
 * @returns {{code:number|null, signal:string|null, stdout:string, stderr:string, error?:Error}}
 */
export function run(argv, opts = {}) {
  const { cwd, env, input, fds = [], timeoutMs, maxBuffer = 64 * 1024 * 1024 } = opts;
  if (!Array.isArray(argv) || argv.length === 0 || argv.some((a) => typeof a !== "string")) throw new ToolError("internal: run() needs an argv array of strings");
  const stdio = ["pipe", "pipe", "pipe"];
  const wanted = [...new Set(fds)].filter((n) => Number.isInteger(n) && n > 2);
  const top = wanted.length ? Math.max(...wanted) : 2;
  for (let i = 3; i <= top; i++) stdio[i] = wanted.includes(i) ? i : "ignore";
  const r = spawnSync(argv[0], argv.slice(1), { cwd, env, input, stdio, encoding: "utf8", timeout: timeoutMs, maxBuffer, killSignal: "SIGKILL" });
  return { code: r.status, signal: r.signal, stdout: r.stdout ?? "", stderr: r.stderr ?? "", error: r.error };
}

/** Atomic file write: temp file in the same directory, fsync, rename. */
export function writeFileAtomic(path, data, mode = 0o600) {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  const tmp = `${path}.tmp.${process.pid}.${randomBytes(4).toString("hex")}`;
  const fd = openSync(tmp, "w", mode);
  try {
    writeSync(fd, data);
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
  renameSync(tmp, path);
}

export function readJsonFile(path) {
  if (!existsSync(path)) return null;
  try {
    return JSON.parse(readFileSync(path, "utf8"));
  } catch {
    return null;
  }
}

export const isObj = (v) => v !== null && typeof v === "object" && !Array.isArray(v);
export const joinPath = join;
