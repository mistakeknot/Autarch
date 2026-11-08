// Host, data-dir and instance binding (plan 2.4). The data directory comes only from a flag, an environment
// variable or the host file; there is no discovery. Two differing candidates are refused.
import { existsSync, readFileSync, readdirSync, realpathSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { ToolError, isObj, refuse } from "./util.mjs";

export const hostFilePath = (pluginId, env = process.env) => join(env.XDG_CONFIG_HOME ?? join(env.HOME ?? homedir(), ".config"), "bb-plugin-deploy", `${pluginId}.json`);

function readHostFile(path) {
  if (!existsSync(path)) return null;
  let v;
  try {
    v = JSON.parse(readFileSync(path, "utf8"));
  } catch (e) {
    throw new ToolError(`host file ${path} is not valid JSON: ${e.message}`);
  }
  if (!isObj(v)) throw new ToolError(`host file ${path} must be a JSON object`);
  const known = ["bbData", "stateDir", "buildsRoot", "backupsRoot", "report"];
  for (const k of Object.keys(v)) if (!known.includes(k)) throw new ToolError(`host file ${path}: unknown key ${k}`);
  for (const k of known.slice(0, 4)) if (v[k] !== undefined && (typeof v[k] !== "string" || !v[k].startsWith("/"))) throw new ToolError(`host file ${path}: ${k} must be an absolute path`);
  if (v.report !== undefined) {
    if (!isObj(v.report) || !Array.isArray(v.report.command) || v.report.command.length === 0 || !v.report.command.every((a) => typeof a === "string") || typeof v.report.title !== "string") throw new ToolError(`host file ${path}: report needs {command: argv array, title: string}`);
  }
  return v;
}

/**
 * Resolve every directory the tool uses without needing bb, so the lock can be taken first.
 * @param {{pluginId:string, flags:{bbData?:string,stateDir?:string,hostFile?:string}, env?:NodeJS.ProcessEnv}} a
 */
export function resolveDirs({ pluginId, flags, env = process.env }) {
  const hostFile = flags.hostFile ?? hostFilePath(pluginId, env);
  const host = readHostFile(hostFile);
  const abs = (p, what) => {
    if (!p.startsWith("/")) throw new ToolError(`${what} must be an absolute path`);
    return resolve(p);
  };
  const cands = [];
  if (flags.bbData) cands.push(["--bb-data", abs(flags.bbData, "--bb-data")]);
  if (env.HOME_BB_DATA) cands.push(["HOME_BB_DATA", abs(env.HOME_BB_DATA, "HOME_BB_DATA")]);
  if (env.BB_PLUGIN_DEPLOY_BB_DATA) cands.push(["BB_PLUGIN_DEPLOY_BB_DATA", abs(env.BB_PLUGIN_DEPLOY_BB_DATA, "BB_PLUGIN_DEPLOY_BB_DATA")]);
  if (host?.bbData) cands.push([`host file ${hostFile}`, abs(host.bbData, "host file bbData")]);
  const distinct = [...new Set(cands.map(([, p]) => p))];
  if (distinct.length === 0) throw refuse("no bb data directory: pass --bb-data, set HOME_BB_DATA or BB_PLUGIN_DEPLOY_BB_DATA, or write the host file (no discovery is done)");
  if (distinct.length > 1) throw refuse(`two different bb data directories were given: ${cands.map(([s, p]) => `${s}=${p}`).join(", ")}`);
  const bbData = distinct[0];
  const stateDir = resolve(flags.stateDir ?? host?.stateDir ?? join(env.XDG_STATE_HOME ?? join(env.HOME ?? homedir(), ".local", "state"), "bb-plugin-deploy", pluginId));
  const buildsRoot = resolve(host?.buildsRoot ?? join(stateDir, "builds"));
  const backupsRoot = resolve(host?.backupsRoot ?? join(bbData, "home-backups"));
  return { bbData, stateDir, buildsRoot, backupsRoot, hostFile, host };
}

/** Does any live process have `needle` (or its realpath) as an argument? Walks /proc/<pid>/cmdline. */
export function pidNamesEntry(pid, entryPath) {
  let real = entryPath;
  try {
    real = realpathSync(entryPath);
  } catch {
    /* the entry may not exist on this host */
  }
  try {
    const args = readFileSync(`/proc/${pid}/cmdline`, "utf8").split("\0");
    return args.includes(entryPath) || args.includes(real);
  } catch {
    return false;
  }
}

/**
 * Verify the data directory belongs to a live bb instance and return its server URL.
 * @returns {{serverUrl:string, runtime:object, pluginRoot:string}}
 */
export function bindInstance({ bbData, pluginId, env = process.env }) {
  const rtPath = join(bbData, "bb-app-runtime.json");
  if (!existsSync(rtPath)) throw refuse(`${bbData} has no bb-app-runtime.json: it is not a live bb data directory`);
  let rt;
  try {
    rt = JSON.parse(readFileSync(rtPath, "utf8"));
  } catch (e) {
    throw refuse(`${rtPath} is not valid JSON: ${e.message}`);
  }
  if (!isObj(rt) || !Number.isInteger(rt.pid) || typeof rt.entryPath !== "string") throw refuse(`${rtPath} lacks pid or entryPath`);
  if (typeof rt.serverUrl !== "string" || !/^https?:\/\//.test(rt.serverUrl)) throw refuse(`${rtPath} has no serverUrl: refusing to guess which bb instance to talk to`);
  if (!pidNamesEntry(rt.pid, rt.entryPath)) throw refuse(`pid ${rt.pid} from ${rtPath} is not a live process running ${rt.entryPath}: the data directory is stale`);
  if (env.BB_SERVER_URL && env.BB_SERVER_URL !== rt.serverUrl) throw refuse(`inherited BB_SERVER_URL=${env.BB_SERVER_URL} differs from the runtime file's ${rt.serverUrl}`);
  const pluginRoot = join(bbData, "plugins", pluginId);
  let ok = false;
  try {
    ok = statSync(pluginRoot).isDirectory();
  } catch {
    /* absent */
  }
  if (!ok) throw refuse(`${pluginRoot} does not exist: plugin ${pluginId} has no data in this instance`);
  return { serverUrl: rt.serverUrl, runtime: rt, pluginRoot };
}

/** The build directories under buildsRoot/<pluginId>, newest name last. */
export function listBuildDirs(buildsRoot, pluginId) {
  const dir = join(buildsRoot, pluginId);
  try {
    return readdirSync(dir, { withFileTypes: true }).filter((d) => d.isDirectory() && !d.isSymbolicLink() && /^[0-9a-f]{12}-\d{8}T\d{6}Z$/.test(d.name)).map((d) => join(dir, d.name)).sort();
  } catch {
    return [];
  }
}
