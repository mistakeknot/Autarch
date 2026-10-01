// The one wrapper (plan A7, Task 2.11). Everything under e2e/ that spawns a process, makes an HTTP
// request or calls an RPC does it here, so every call is built from nothing and checked against the rig:
//   - the child environment is built from an empty object, never from process.env;
//   - a target (URL + data dir + nonce) is refused when it is not loopback, has no explicit port, uses any
//     ambient port (from the environment and every bb-app-runtime.json), or does not exist (no default);
//   - a `bb` PATH shim execs the real bb only when the URL and the data-dir nonce match this rig,
//     otherwise it exits 97 and logs the call, so an indirect autarch -> bb call cannot reach another server;
//   - rigRpc is the only e2e RPC path and calls assertOwned() first.
// A refusal opens no socket: it happens before any spawn or request.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { randomUUID } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { homedir, tmpdir, userInfo } from "node:os";
import { delimiter, dirname, join } from "node:path";

export const SHIM_EXIT = 97;
export const NONCE_FILE = ".home-e2e-nonce";

export class RefusedError extends Error {
  constructor(readonly reason: string, message: string) {
    super(`rig refused: ${message}`);
    this.name = "RefusedError";
  }
}

// ---- target classification ---------------------------------------------------------------------

/** localhost, 127/8, ::1, ::ffff:127.x (any spelling WHATWG URL normalises to), 0.0.0.0 and ::. */
export function isLoopbackHost(hostname: string): boolean {
  let h = hostname.toLowerCase().replace(/^\[|\]$/g, "").replace(/\.$/, "");
  if (h === "localhost" || h.endsWith(".localhost")) return true;
  if (h === "::1" || h === "::" || h === "0.0.0.0") return true;
  if (/^127(\.\d{1,3}){3}$/.test(h)) return true;
  const mapped = /^::ffff:(.+)$/.exec(h);
  if (mapped) {
    const rest = mapped[1]!;
    if (/^127(\.\d{1,3}){3}$/.test(rest) || rest === "0.0.0.0") return true;
    const hex = /^([0-9a-f]{1,4}):([0-9a-f]{1,4})$/.exec(rest); // ::ffff:7f00:1
    if (hex) return (parseInt(hex[1]!, 16) >> 8) === 127 || (hex[1] === "0" && hex[2] === "0");
  }
  return false;
}

const PORT_KEYS = /port/i;
function collectPorts(v: unknown, into: Set<number>, key = ""): void {
  if (typeof v === "number" && Number.isInteger(v) && v > 0 && v < 65536 && PORT_KEYS.test(key)) into.add(v);
  else if (typeof v === "string") {
    if (PORT_KEYS.test(key) && /^\d{1,5}$/.test(v)) into.add(Number(v));
    try {
      const u = new URL(v);
      if (u.port) into.add(Number(u.port));
    } catch {
      /* not a URL */
    }
  } else if (Array.isArray(v)) v.forEach((x) => collectPorts(x, into, key));
  else if (v && typeof v === "object") for (const [k, x] of Object.entries(v)) collectPorts(x, into, k);
}

function runtimeFiles(root: string, depth: number): string[] {
  const out: string[] = [];
  const walk = (dir: string, d: number) => {
    let entries;
    try {
      entries = readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    for (const e of entries) {
      if (e.isFile() && e.name === "bb-app-runtime.json") out.push(join(dir, e.name));
      else if (e.isDirectory() && d < depth && !e.isSymbolicLink()) walk(join(dir, e.name), d + 1);
    }
  };
  walk(root, 0);
  return out;
}

/** Ports the ambient bb uses: from the environment and from every bb-app-runtime.json under ~/.bb and ~/.bb-machines/*. */
export function ambientPorts(env: NodeJS.ProcessEnv = process.env, homes: (string | undefined)[] = [env.HOME, safeHome(), homedir()]): Set<number> {
  const ports = new Set<number>();
  for (const k of ["BB_SERVER_URL", "BB_HOST_DAEMON_PORT", "BB_SERVER_PORT", "HOME_E2E_BB"]) {
    const v = env[k];
    if (!v) continue;
    if (/^\d{1,5}$/.test(v)) ports.add(Number(v));
    else
      try {
        const u = new URL(v);
        if (u.port) ports.add(Number(u.port));
      } catch {
        /* ignore */
      }
  }
  for (const home of new Set(homes.filter((h): h is string => !!h))) {
    for (const base of [join(home, ".bb"), ...machineDirs(home)]) {
      for (const f of runtimeFiles(base, 3)) {
        try {
          collectPorts(JSON.parse(readFileSync(f, "utf8")), ports);
        } catch {
          /* unreadable or not JSON: skip, the other sources still apply */
        }
      }
    }
  }
  return ports;
}
function safeHome(): string | undefined {
  try {
    return userInfo().homedir;
  } catch {
    return undefined;
  }
}
function machineDirs(home: string): string[] {
  const base = join(home, ".bb-machines");
  try {
    return readdirSync(base, { withFileTypes: true }).filter((e) => e.isDirectory()).map((e) => join(base, e.name));
  } catch {
    return [];
  }
}

/** Why a (url, hostPort) must be refused, or null when it is an acceptable rig target. */
export function refusal(url: string | undefined, hostPort: string | undefined, ambient: Set<number>): { reason: string; message: string } | null {
  if (!url) return { reason: "no-environment", message: "no target URL: there is no default bb server" };
  let u: URL;
  try {
    u = new URL(url);
  } catch {
    return { reason: "bad-url", message: `${url} is not a URL` };
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return { reason: "bad-url", message: `${url} is not http(s)` };
  if (!isLoopbackHost(u.hostname)) return { reason: "not-loopback", message: `${u.hostname} is not loopback` };
  if (!u.port) return { reason: "default-port", message: `${url} has no explicit port: defaults are refused` };
  const port = Number(u.port);
  if (ambient.has(port)) return { reason: "ambient-port", message: `${u.hostname}:${port} is an ambient bb port` };
  if (hostPort !== undefined) {
    if (!/^\d{1,5}$/.test(hostPort)) return { reason: "bad-port", message: `host daemon port ${hostPort} is not a port` };
    if (ambient.has(Number(hostPort))) return { reason: "ambient-port", message: `host daemon port ${hostPort} is an ambient bb port` };
  }
  return null;
}

// ---- target -----------------------------------------------------------------------------------------

export interface OwnershipProof {
  launcher_pid: number;
  socket_owner_pid: number;
  listen_inode: number;
  listen_inode_in_owned_set: boolean;
  owned_set: number[];
}

export interface RigTarget {
  readonly url: string;
  readonly dataDir: string;
  readonly home: string;
  readonly hostPort: string;
  readonly nonce: string;
  /** The directory holding the `bb` shim. */
  readonly shimDir: string;
  readonly shimLog: string;
  /** Real mode: rigRpc needs the socket-ownership proof first (Task 2.12 step 3). Fake mode does not. */
  readonly requireProof: boolean;
  proof?: OwnershipProof;
}
const created = new WeakSet<object>();

export interface TargetOptions {
  url?: string;
  dataDir?: string;
  home?: string;
  hostPort?: string;
  /** The real bb the shim execs once the target matches. */
  realBb: string;
  requireProof?: boolean;
  nonce?: string;
  /** Test seam; defaults to the real ambient set. */
  ambient?: Set<number>;
}

/** Validate a target, write its nonce into the data dir, and install the `bb` shim. Throws RefusedError. */
export function makeTarget(o: TargetOptions): RigTarget {
  const bad = refusal(o.url, o.hostPort, o.ambient ?? ambientPorts());
  if (bad) throw new RefusedError(bad.reason, bad.message);
  if (!o.dataDir || !o.home) throw new RefusedError("no-environment", "a data dir and a HOME are required: there is no default");
  if (/\.bb-machines(\/|$)|(^|\/)\.bb(\/|$)/.test(o.dataDir)) throw new RefusedError("ambient-data", `${o.dataDir} looks like a live bb data dir`);
  const nonce = o.nonce ?? randomUUID();
  mkdirSync(o.dataDir, { recursive: true });
  mkdirSync(o.home, { recursive: true });
  writeFileSync(join(o.dataDir, NONCE_FILE), nonce);
  const shimDir = mkdtempSync(join(tmpdir(), "rig-shim-"));
  const shimLog = join(shimDir, "shim.log");
  writeFileSync(shimLog, "");
  const target: RigTarget = { url: new URL(o.url!).origin, dataDir: o.dataDir, home: o.home, hostPort: o.hostPort ?? "", nonce, shimDir, shimLog, requireProof: o.requireProof ?? false };
  installShim(target, o.realBb);
  created.add(target);
  return target;
}

/** Refuses unless the target was made by makeTarget, still passes classification, still holds its nonce, and (real mode) is proven. */
export function assertOwned(t: RigTarget, ambient: Set<number> = ambientPorts()): void {
  if (!t || !created.has(t)) throw new RefusedError("not-a-target", "not a rig target");
  const bad = refusal(t.url, t.hostPort || undefined, ambient);
  if (bad) throw new RefusedError(bad.reason, bad.message);
  let nonce = "";
  try {
    nonce = readFileSync(join(t.dataDir, NONCE_FILE), "utf8");
  } catch {
    /* absent */
  }
  if (nonce !== t.nonce) throw new RefusedError("nonce", `the data dir ${t.dataDir} does not hold this rig's nonce`);
  if (t.requireProof && t.proof?.listen_inode_in_owned_set !== true) throw new RefusedError("unproven", "the server's socket ownership is not proven yet");
}

const sq = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`;

/** The shim is a shell script with the rig identity baked in, so a child cannot change what it expects. */
function installShim(t: RigTarget, realBb: string): void {
  const script = `#!/bin/sh
# rig bb shim: execs the real bb only for this rig's server (plan A7); otherwise exit ${SHIM_EXIT}.
url=${sq(t.url)}; data=${sq(t.dataDir)}; port=${sq(t.hostPort)}; nonce=${sq(t.nonce)}; log=${sq(t.shimLog)}; real=${sq(realBb)}
have=$(cat "$data/${NONCE_FILE}" 2>/dev/null)
if [ "$BB_SERVER_URL" != "$url" ] || [ "$BB_DATA_DIR" != "$data" ] || [ "$have" != "$nonce" ] || { [ -n "$port" ] && [ "$BB_HOST_DAEMON_PORT" != "$port" ]; }; then
  printf '%s REFUSED url=%s data=%s argv=%s\\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "\${BB_SERVER_URL:-}" "\${BB_DATA_DIR:-}" "$*" >> "$log"
  echo "bb shim: refused, this call does not target the rig ($BB_SERVER_URL)" >&2
  exit ${SHIM_EXIT}
fi
exec "$real" "$@"
`;
  const f = join(t.shimDir, "bb");
  writeFileSync(f, script);
  chmodSync(f, 0o755);
}

/** Lines the shim logged for refused calls. */
export function shimRefusals(t: RigTarget): string[] {
  return existsSync(t.shimLog) ? readFileSync(t.shimLog, "utf8").split("\n").filter(Boolean) : [];
}

// ---- environment ------------------------------------------------------------------------------------------

/** First directory on PATH holding an executable `name`. */
export function resolveBin(name: string, pathVar = process.env.PATH ?? ""): string | undefined {
  for (const d of pathVar.split(delimiter)) {
    if (!d) continue;
    const f = join(d, name);
    if (existsSync(f)) return f;
  }
  return undefined;
}

let throwaway: string | undefined;
const throwawayHome = () => (throwaway ??= mkdtempSync(join(tmpdir(), "rig-nohome-")));

/** For a call with no target there is no default server: its `bb` is a shim that always refuses. */
let refuseDir: string | undefined;
function refusingShimDir(): string {
  if (!refuseDir) {
    refuseDir = mkdtempSync(join(tmpdir(), "rig-noshim-"));
    writeFileSync(join(refuseDir, "bb"), `#!/bin/sh\necho "bb shim: refused, no rig target (there is no default server)" >&2\nexit ${SHIM_EXIT}\n`);
    chmodSync(join(refuseDir, "bb"), 0o755);
  }
  return refuseDir;
}

export interface ExecOptions {
  /** The rig the call belongs to. Without one, no BB_* variable exists and the shim refuses every bb call. */
  target?: RigTarget;
  /** Passed only when a scenario sets it. */
  threadId?: string;
  cwd?: string;
  input?: string;
  timeoutMs?: number;
  /** Extra variables (never BB_*, PATH or HOME). */
  env?: Record<string, string>;
  /** Untargeted calls only: the HOME to use instead of a throwaway directory (e.g. a Go build cache). */
  home?: string;
  stdio?: "pipe" | "inherit-stderr";
}

/** Built from nothing. PATH = shim dir, /usr/bin, /bin, then the directories of the resolved bb and node. */
export function rigEnv(o: ExecOptions = {}): Record<string, string> {
  const extra = o.env ?? {};
  for (const k of Object.keys(extra)) if (/^BB_/.test(k) || k === "PATH" || k === "HOME") throw new RefusedError("env-override", `${k} cannot be set through env`);
  const dirs: string[] = [];
  dirs.push(o.target ? o.target.shimDir : refusingShimDir());
  dirs.push("/usr/bin", "/bin");
  const bb = resolveBin("bb");
  if (bb) dirs.push(dirname(bb));
  dirs.push(dirname(process.execPath));
  const env: Record<string, string> = { PATH: [...new Set(dirs)].join(delimiter), HOME: o.target ? o.target.home : (o.home ?? throwawayHome()), ...extra };
  if (o.target) {
    env.BB_SERVER_URL = o.target.url;
    env.BB_DATA_DIR = o.target.dataDir;
    if (o.target.hostPort) env.BB_HOST_DAEMON_PORT = o.target.hostPort;
    if (o.threadId) env.BB_THREAD_ID = o.threadId;
  }
  return env;
}

export interface ExecResult {
  code: number | null;
  stdout: string;
  stderr: string;
}

function guard(o: ExecOptions): void {
  if (o.target) assertTargetShape(o.target);
}
function assertTargetShape(t: RigTarget): void {
  if (!created.has(t)) throw new RefusedError("not-a-target", "not a rig target");
}

/** Resolve a command: a bare `bb` is always the shim; other bare names resolve on the rig PATH. */
function command(cmd: string, env: Record<string, string>): string {
  if (cmd.includes("/")) return cmd;
  return resolveBin(cmd, env.PATH) ?? cmd;
}

export function rigExecSync(cmd: string, args: string[], o: ExecOptions = {}): ExecResult {
  guard(o);
  const env = rigEnv(o);
  const r = spawnSync(command(cmd, env), args, { cwd: o.cwd, env, input: o.input, encoding: "utf8", timeout: o.timeoutMs, maxBuffer: 64 << 20 });
  return { code: r.status, stdout: r.stdout ?? "", stderr: r.stderr ?? "" };
}

export async function rigExec(cmd: string, args: string[], o: ExecOptions = {}): Promise<ExecResult> {
  guard(o);
  return new Promise((resolve, reject) => {
    const env = rigEnv(o);
    const p = spawn(command(cmd, env), args, { cwd: o.cwd, env, stdio: ["pipe", "pipe", "pipe"] });
    let stdout = "";
    let stderr = "";
    const timer = o.timeoutMs ? setTimeout(() => p.kill("SIGKILL"), o.timeoutMs) : undefined;
    p.stdout.on("data", (d) => (stdout += d));
    p.stderr.on("data", (d) => (stderr += d));
    p.on("error", (e) => {
      if (timer) clearTimeout(timer);
      reject(e);
    });
    p.on("close", (code) => {
      if (timer) clearTimeout(timer);
      resolve({ code, stdout, stderr });
    });
    p.stdin.on("error", () => undefined); // the child may exit without reading its stdin
    p.stdin.end(o.input ?? "");
  });
}

/** A long-lived child (a launcher, a serve, a scenario child). Pipes stdin and stdout; stderr is inherited. */
export function rigSpawn(cmd: string, args: string[], o: ExecOptions & { stdout?: "pipe" | "ignore" } = {}): ChildProcess {
  guard(o);
  const env = rigEnv(o);
  return spawn(command(cmd, env), args, { cwd: o.cwd, env, stdio: [o.stdout === "ignore" ? "ignore" : "pipe", o.stdout ?? "pipe", "inherit"] });
}

// ---- RPC and HTTP --------------------------------------------------------------------------------------------

/** The only e2e RPC path: `bb plugin rpc call <plugin> <method>` on the owned target, through the shim. */
export async function rigRpc<T = unknown>(t: RigTarget, plugin: string, method: string, input: unknown = null, o: { threadId?: string } = {}): Promise<T> {
  assertOwned(t);
  const dir = mkdtempSync(join(t.home, "rpc-"));
  const f = join(dir, `${method}.json`);
  writeFileSync(f, JSON.stringify(input));
  const r = await rigExec("bb", ["plugin", "rpc", "call", plugin, method, "--input-file", f, "--json"], { target: t, threadId: o.threadId, timeoutMs: 120_000 });
  if (r.code !== 0) throw new Error(`rpc ${plugin}.${method} failed (${r.code}): ${r.stderr || r.stdout}`);
  return JSON.parse(r.stdout) as T;
}

/** The only e2e `fetch`. Refuses anything that is not a loopback, non-ambient, explicit-port URL. */
export async function rigFetch(url: string, init?: RequestInit, ambient: Set<number> = ambientPorts()): Promise<Response> {
  const bad = refusal(url, undefined, ambient);
  if (bad) throw new RefusedError(bad.reason, bad.message);
  return fetch(url, init);
}
