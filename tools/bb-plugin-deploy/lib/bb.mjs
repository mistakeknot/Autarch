// The one module that talks to the bb CLI (plan 2.4). Every call runs with BB_SERVER_URL taken from the bound
// instance's runtime file, BB_HOST_DAEMON_PORT cleared, a fixed PATH and no shell.
import { spawn } from "node:child_process";
import { accessSync, constants, statSync } from "node:fs";
import { dirname } from "node:path";
import { ToolError, fixedPath, run } from "./util.mjs";

/** The minimal environment for every child process the tool starts. Extra keys are BPD_* and BB_SERVER_URL only. */
export function childEnv({ serverUrl, lockFd, extra = {}, binDir = testBinDir() } = {}) {
  const env = { PATH: fixedPath(binDir ? [binDir] : []), HOME: process.env.HOME ?? "/", LANG: process.env.LANG ?? "C.UTF-8", LC_ALL: process.env.LC_ALL ?? "C.UTF-8", TZ: process.env.TZ ?? "UTC", TMPDIR: process.env.TMPDIR ?? "/tmp" };
  if (serverUrl) env.BB_SERVER_URL = serverUrl;
  if (lockFd !== undefined && lockFd !== null) env.BPD_LOCK_FD = String(lockFd);
  for (const [k, v] of Object.entries(extra)) {
    if (/^BPD_[A-Z0-9_]+$/.test(k) && typeof v === "string") env[k] = v;
  }
  return env;
}

/** Directory of the resolved bb binary (when absolute), put on every child's PATH so a bare `bb` in a plugin command or export finds the same bb the tool uses. */
function testBinDir() {
  const b = bbBinary();
  return b.startsWith("/") ? dirname(b) : undefined;
}

const isExec = (f) => {
  try {
    accessSync(f, constants.X_OK);
    return statSync(f).isFile();
  } catch {
    return false;
  }
};

/**
 * The bb binary, in order: BB_PLUGIN_DEPLOY_BB (absolute; tests), BB_CLI (absolute), ~/.local/bin/bb, `bb` on the
 * caller's PATH, then bare `bb` on the fixed PATH. The children still get the fixed PATH; only the lookup of bb
 * itself uses the caller's.
 */
export function bbBinary(env = process.env) {
  const abs = (v) => (typeof v === "string" && v.startsWith("/") ? v : null);
  if (abs(env.BB_PLUGIN_DEPLOY_BB)) return env.BB_PLUGIN_DEPLOY_BB;
  if (abs(env.BB_CLI) && isExec(env.BB_CLI)) return env.BB_CLI;
  if (abs(env.HOME) && isExec(`${env.HOME}/.local/bin/bb`)) return `${env.HOME}/.local/bin/bb`;
  for (const d of (env.PATH ?? "").split(":")) {
    if (d.startsWith("/") && isExec(`${d}/bb`)) return `${d}/bb`;
  }
  return "bb";
}

/**
 * @param {{serverUrl:string, pluginId:string, lockFd?:number|null, bin?:string, log?:(s:string)=>void}} cfg
 */
export function makeBb(cfg) {
  const { serverUrl, pluginId, lockFd = null, log = () => {} } = cfg;
  if (!serverUrl) throw new ToolError("internal: bb module needs the bound serverUrl");
  const bin = cfg.bin ?? bbBinary();
  const binDir = bin.startsWith("/") ? dirname(bin) : undefined;
  const env = () => childEnv({ serverUrl, lockFd, binDir });
  const fds = () => (lockFd !== null && lockFd !== undefined ? [lockFd] : []);

  function call(args, { allowFail = false, timeoutMs = 120_000 } = {}) {
    log(`bb ${args.join(" ")}`);
    const r = run([bin, ...args], { env: env(), fds: fds(), timeoutMs });
    if (r.error && r.error.code === "ENOENT") throw new ToolError("bb CLI not found on PATH");
    if (r.code !== 0 && !allowFail) throw new ToolError(`bb ${args.slice(0, 3).join(" ")} failed (exit ${r.code}): ${(r.stderr || r.stdout).trim().slice(0, 400)}`);
    return r;
  }
  const json = (args, opts) => {
    const r = call(args, opts);
    try {
      return JSON.parse(r.stdout);
    } catch {
      throw new ToolError(`bb ${args.slice(0, 3).join(" ")} did not print JSON`);
    }
  };

  const api = {
    serverUrl,
    call,
    /** The directory a plugin is installed from, or null when bb reports none. */
    sourceDir() {
      const r = call(["plugin", "source", pluginId, "--json"], { allowFail: true });
      if (r.code !== 0) return null;
      let v;
      try {
        v = JSON.parse(r.stdout);
      } catch {
        return null;
      }
      const m = typeof v?.resolved === "string" ? /^path:(\/.*)$/.exec(v.resolved) : null;
      return m ? m[1] : null;
    },
    /** The plugin's list entry or null. */
    entry() {
      const v = json(["plugin", "list", "--json"]);
      const hits = (v.plugins ?? []).filter((p) => p.id === pluginId);
      return hits.length === 1 ? hits[0] : null;
    },
    /** Legacy plugin_running: exactly one entry for the id, enabled and running. */
    isRunning() {
      const v = json(["plugin", "list", "--json"]);
      const hits = (v.plugins ?? []).filter((p) => p.id === pluginId);
      return hits.length === 1 && hits[0].enabled === true && hits[0].status === "running";
    },
    isEnabled() {
      const e = api.entry();
      return e !== null && e.enabled === true;
    },
    disable: () => call(["plugin", "disable", pluginId]),
    enable: () => call(["plugin", "enable", pluginId]),
    install: (dir) => call(["plugin", "install", "--yes", dir]),
    /**
     * `bb plugin dev` as an asynchronous child, so the caller's event loop (and its signal handlers) keeps running while
     * it does. Returns {done: Promise<{code, signal, stdout, stderr, error?}>, stop()}. stop() asks it to end, and kills
     * it if it has not within 5 s.
     */
    dev(dir, { timeoutMs = 600_000 } = {}) {
      log(`bb plugin dev ${dir}`);
      const stdio = ["ignore", "pipe", "pipe"];
      for (const n of fds()) {
        for (let i = 3; i < n; i++) stdio[i] ??= "ignore";
        stdio[n] = n;
      }
      const child = spawn(bin, ["plugin", "dev", dir], { env: env(), stdio });
      let stdout = "";
      let stderr = "";
      const keep = (cur, chunk) => (cur + chunk).slice(-64 * 1024);
      child.stdout.on("data", (c) => (stdout = keep(stdout, c)));
      child.stderr.on("data", (c) => (stderr = keep(stderr, c)));
      const timer = setTimeout(() => child.kill("SIGKILL"), timeoutMs);
      let hard = null;
      const done = new Promise((resolveDone) => {
        let error;
        child.once("error", (e) => {
          error = e.code === "ENOENT" ? new ToolError("bb CLI not found on PATH") : e;
        });
        child.once("close", (code, signal) => {
          clearTimeout(timer);
          if (hard) clearTimeout(hard);
          resolveDone({ code, signal, stdout, stderr, error });
        });
      });
      return {
        done,
        stop() {
          if (child.exitCode !== null || child.signalCode !== null) return;
          child.kill("SIGTERM");
          hard ??= setTimeout(() => child.kill("SIGKILL"), 5_000);
        },
      };
    },
    configValues() {
      return json(["plugin", "config", pluginId, "--json"]).values ?? {};
    },
    setConfig: (key, value) => call(["plugin", "config", pluginId, "set", key, value]),
  };
  return api;
}
