// The deploy record (plan 2.5): what is installed, what it replaces, and a capped history. Written atomically,
// only under the destination lock. Live resolution asks bb where the plugin runs from and trusts that over the file.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { buildCommit, lockfileSha, newBuildDir, readStamp, verifyStamp } from "./build.mjs";
import { git, gitTry } from "./gitops.mjs";
import { SHA40, ToolError, isObj, nowIso, refuse, writeFileAtomic } from "./util.mjs";

export const RECORD_FORMAT = "bb-plugin-deploy/record@1";
export const HISTORY_CAP = 200;
const EVENTS = ["install", "rollback", "pause", "fail", "observed-external-install"];

export const recordPath = (stateDir) => join(stateDir, "record.json");
export const installedShaPath = (stateDir) => join(stateDir, "installed.sha");

export function emptyRecord(pluginId) {
  return { format: RECORD_FORMAT, pluginId, installed: null, previous: null, history: [] };
}

export function readRecord(stateDir, pluginId) {
  const p = recordPath(stateDir);
  if (!existsSync(p)) return emptyRecord(pluginId);
  let v;
  try {
    v = JSON.parse(readFileSync(p, "utf8"));
  } catch (e) {
    throw new ToolError(`${p} is not valid JSON: ${e.message}`);
  }
  if (!isObj(v) || v.format !== RECORD_FORMAT || v.pluginId !== pluginId || !Array.isArray(v.history)) throw new ToolError(`${p} is not a ${RECORD_FORMAT} record for ${pluginId}`);
  return v;
}

export function writeRecord(stateDir, rec) {
  if (rec.history.length > HISTORY_CAP) rec.history = rec.history.slice(-HISTORY_CAP);
  writeFileAtomic(recordPath(stateDir), JSON.stringify(rec, null, 2) + "\n");
}

export function addHistory(rec, event, commit, detail = "") {
  if (!EVENTS.includes(event)) throw new ToolError(`internal: unknown history event ${event}`);
  rec.history.push({ at: nowIso(), event, commit: commit ?? null, detail: String(detail).slice(0, 500) });
  if (rec.history.length > HISTORY_CAP) rec.history = rec.history.slice(-HISTORY_CAP);
}

/** installed.sha stays for the legacy runner (plan 2.5). Never written when the installed commit is unknown. */
export function writeInstalledSha(stateDir, commit) {
  if (!SHA40.test(commit ?? "")) return false;
  writeFileAtomic(installedShaPath(stateDir), `${commit}\n`);
  return true;
}
export function readInstalledSha(stateDir) {
  try {
    const s = readFileSync(installedShaPath(stateDir), "utf8").trim();
    return SHA40.test(s) ? s : null;
  } catch {
    return null;
  }
}

/** The build root of a plugin directory: the plugin dir minus the plugin's relative path in the repo. */
export function buildRootOf(sourceDir, pluginDirRel) {
  const suffix = `/${pluginDirRel}`;
  return sourceDir.endsWith(suffix) ? sourceDir.slice(0, -suffix.length) : null;
}

/**
 * What is running, according to bb. states: "stamped" (a tool build), "git" (a legacy checkout with a clean plugin dir),
 * "unknown" (anything else). `reason` explains an unknown.
 */
export function resolveLive({ bb, plugin }) {
  const dir = bb.sourceDir();
  if (!dir) return { state: "unknown", reason: "bb reports no source for the plugin", sourceDir: null };
  const root = buildRootOf(dir, plugin.pluginDir);
  if (!root) return { state: "unknown", reason: `source ${dir} does not end in ${plugin.pluginDir}`, sourceDir: dir };
  const base = { sourceDir: dir, buildRoot: root, pluginDir: dir };
  const stamp = readStamp(root);
  if (stamp) {
    const head = gitTry(root, ["rev-parse", "HEAD"]);
    const tree = gitTry(root, ["rev-parse", "HEAD^{tree}"]);
    if (head !== stamp.commit || tree !== stamp.tree) return { ...base, state: "unknown", reason: "stamp disagrees with the build's HEAD or tree" };
    return { ...base, state: "stamped", commit: stamp.commit, tree: stamp.tree, stamp };
  }
  const head = gitTry(root, ["rev-parse", "HEAD"]);
  if (head === null || !SHA40.test(head)) return { ...base, state: "unknown", reason: "the source is not a git checkout" };
  const dirty = gitTry(root, ["status", "--porcelain", "--", plugin.pluginDir]);
  if (dirty !== "") return { ...base, state: "unknown", reason: "the live plugin directory has uncommitted changes" };
  return { ...base, state: "git", commit: head, tree: git(root, ["rev-parse", "HEAD^{tree}"]) };
}

/**
 * Bring the record in line with what is live. Pure with respect to disk: the caller writes the record.
 * @returns {{record:object, changed:boolean, external:boolean, unknown:boolean}}
 */
export function reconcile(rec, live) {
  if (live.state === "unknown") return { record: rec, changed: false, external: false, unknown: true };
  const cur = rec.installed;
  const entry = () => ({ commit: live.commit, tree: live.tree, buildRoot: live.buildRoot, pluginDir: live.pluginDir, installedAt: nowIso(), by: { mode: "observed", actor: "reconcile" }, backup: null, toolCommit: live.stamp?.toolCommit ?? null });
  if (!cur) {
    rec.installed = entry();
    addHistory(rec, "observed-external-install", live.commit, "first observation");
    return { record: rec, changed: true, external: true, unknown: false };
  }
  if (cur.commit === live.commit && cur.buildRoot === live.buildRoot) return { record: rec, changed: false, external: false, unknown: false };
  if (cur.commit === live.commit) {
    rec.installed = { ...cur, buildRoot: live.buildRoot, pluginDir: live.pluginDir, tree: live.tree };
    addHistory(rec, "observed-external-install", live.commit, "same commit, different build");
    return { record: rec, changed: true, external: true, unknown: false };
  }
  rec.previous = cur;
  rec.installed = entry();
  addHistory(rec, "observed-external-install", live.commit, `was ${cur.commit.slice(0, 12)}`);
  return { record: rec, changed: true, external: true, unknown: false };
}

/**
 * Previous anchoring (plan 2.5), before every tool install under the lock. Returns the anchored build
 * {commit, tree, buildRoot, pluginDir}. A dirty or unknown live install, or a failed rebuild, is a refusal.
 */
export function anchorPrevious({ plugin, live, buildsRoot, vars, toolCommit, lockFd, log = () => {} }) {
  if (live.state === "unknown") throw refuse(`cannot anchor the previous install: ${live.reason}`);
  if (live.state === "stamped") {
    try {
      verifyStamp(live.buildRoot, plugin);
    } catch (e) {
      throw refuse(`the live build is a tool build but no longer verifies (${e.message}); not anchoring it`);
    }
    return { commit: live.commit, tree: live.tree, buildRoot: live.buildRoot, pluginDir: live.pluginDir };
  }
  const dest = newBuildDir(buildsRoot, plugin.pluginId, live.commit);
  log(`anchoring previous: rebuilding ${live.commit.slice(0, 12)} immutably`);
  let b;
  try {
    b = buildCommit({ plugin, commit: live.commit, expectTree: live.tree, dest, toolCommit, vars, lockFd, log });
  } catch (e) {
    throw refuse(`could not rebuild the live commit ${live.commit.slice(0, 12)} for anchoring: ${e.message}`);
  }
  const liveLock = lockfileSha(live.buildRoot, plugin.pluginDir);
  if (liveLock !== b.stamp.lockSha256) throw refuse("the rebuilt previous has a different lockfile than the live build");
  return { commit: b.commit, tree: b.tree, buildRoot: b.root, pluginDir: b.pluginDir };
}
