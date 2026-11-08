// The one module that talks to the bb CLI (plan 2.4). Every call runs with BB_SERVER_URL taken from the bound
// instance's runtime file, BB_HOST_DAEMON_PORT cleared, a fixed PATH and no shell.
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

/** Directory of the BB_PLUGIN_DEPLOY_BB test binary, put on the children's PATH so a bare `bb` in a plugin command finds it. */
function testBinDir() {
  const b = process.env.BB_PLUGIN_DEPLOY_BB;
  return b && b.startsWith("/") ? dirname(b) : undefined;
}

/** The bb binary: BB_PLUGIN_DEPLOY_BB (an absolute path, used by tests) or `bb` on the fixed PATH. */
export function bbBinary(env = process.env) {
  return env.BB_PLUGIN_DEPLOY_BB && env.BB_PLUGIN_DEPLOY_BB.startsWith("/") ? env.BB_PLUGIN_DEPLOY_BB : "bb";
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
    dev: (dir) => call(["plugin", "dev", dir], { timeoutMs: 600_000 }),
    configValues() {
      return json(["plugin", "config", pluginId, "--json"]).values ?? {};
    },
    setConfig: (key, value) => call(["plugin", "config", pluginId, "set", key, value]),
  };
  return api;
}
