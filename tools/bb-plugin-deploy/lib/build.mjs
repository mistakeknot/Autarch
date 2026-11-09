// Immutable builds (plan 2.5 and 2.6): clone, verify, stamp, build, re-verify. A build directory is never reused.
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { childEnv } from "./bb.mjs";
import { cloneVerify, git } from "./gitops.mjs";
import { templateArgv } from "./pluginfile.mjs";
import { SHA40, ToolError, nowIso, refuse, run, sha256, utcStamp, writeFileAtomic } from "./util.mjs";

export const STAMP_FORMAT = "bb-plugin-deploy/stamp@1";
const STAMP_REL = join(".bb-plugin-deploy", "stamp.json");

/** The directory name for a new build. BPD_UTC_STAMP is a test hook that fixes the time part. */
export function newBuildDir(buildsRoot, pluginId, commit, env = process.env) {
  const stamp = env.BPD_UTC_STAMP && /^\d{8}T\d{6}Z$/.test(env.BPD_UTC_STAMP) ? env.BPD_UTC_STAMP : utcStamp();
  return join(buildsRoot, pluginId, `${commit.slice(0, 12)}-${stamp}`);
}

export function lockfileSha(buildRoot, pluginDir) {
  const p = join(buildRoot, pluginDir, "package-lock.json");
  return existsSync(p) ? sha256(readFileSync(p)) : null;
}

export function readStamp(buildRoot) {
  const p = join(buildRoot, STAMP_REL);
  if (!existsSync(p)) return null;
  try {
    const v = JSON.parse(readFileSync(p, "utf8"));
    return v && v.format === STAMP_FORMAT && SHA40.test(v.commit ?? "") && SHA40.test(v.tree ?? "") ? v : null;
  } catch {
    return null;
  }
}

/**
 * Re-check a stamped build before it is installed or trusted: stamp commit == HEAD, tree == HEAD^{tree}, no tracked
 * change, same lockfile. Returns the stamp or throws a refusal.
 */
export function verifyStamp(buildRoot, plugin) {
  const st = readStamp(buildRoot);
  if (!st) throw refuse(`${buildRoot} has no valid stamp`);
  if (st.pluginId !== plugin.pluginId) throw refuse(`${buildRoot} is stamped for ${st.pluginId}, not ${plugin.pluginId}`);
  const head = git(buildRoot, ["rev-parse", "HEAD"]);
  if (head !== st.commit) throw refuse(`${buildRoot}: stamp commit ${st.commit.slice(0, 12)} differs from HEAD ${head.slice(0, 12)}`);
  const tree = git(buildRoot, ["rev-parse", "HEAD^{tree}"]);
  if (tree !== st.tree) throw refuse(`${buildRoot}: stamp tree differs from HEAD tree`);
  const d = git(buildRoot, ["diff", "--quiet", "HEAD"], { allowFail: true });
  if (d.code !== 0) throw refuse(`${buildRoot}: tracked files differ from HEAD`);
  const lock = lockfileSha(buildRoot, plugin.pluginDir);
  if (lock !== st.lockSha256) throw refuse(`${buildRoot}: lockfile changed since the stamp`);
  return st;
}

/**
 * Clone, verify, stamp and build `commit` into a NEW directory.
 * @param {object} a
 * @returns {{root:string, pluginDir:string, commit:string, tree:string, stamp:object}}
 */
export function buildCommit({ plugin, commit, expectTree, dest, toolCommit, vars, lockFd, log = () => {}, beforeBuild = null }) {
  const v = cloneVerify({ repo: plugin.repo, commit, expectTree, dest, lockFd });
  const pluginDir = join(dest, plugin.pluginDir);
  if (!existsSync(pluginDir)) throw refuse(`plugin directory ${plugin.pluginDir} does not exist in ${commit.slice(0, 12)}`);
  // Runs after the clone is verified and before any build command (npm ci and the like), so a refusal or pause
  // never executes the commit's own code (plan 2.4).
  if (beforeBuild) beforeBuild({ root: dest, pluginDir, commit: v.commit, tree: v.tree });
  const bv = { ...vars, targetRoot: dest, targetPluginDir: pluginDir, buildDir: dest, targetCommit: commit };
  const cmds = plugin.build.map((a) => templateArgv(a, bv));
  const stamp = { format: STAMP_FORMAT, pluginId: plugin.pluginId, commit: v.commit, tree: v.tree, builtAt: nowIso(), toolCommit: toolCommit ?? null, buildCommands: cmds, lockSha256: lockfileSha(dest, plugin.pluginDir) };
  writeFileAtomic(join(dest, STAMP_REL), JSON.stringify(stamp, null, 2) + "\n");
  for (const argv of cmds) {
    log(`build: ${argv.join(" ")}`);
    const r = run(argv, { cwd: pluginDir, env: childEnv({ lockFd }), fds: lockFd ? [lockFd] : [], timeoutMs: 30 * 60_000 });
    if (r.code !== 0) throw new ToolError(`build command failed (${argv.join(" ")}, exit ${r.code}): ${(r.stderr || r.stdout).trim().slice(-600)}`);
  }
  const d = git(dest, ["diff", "--quiet", "HEAD"], { allowFail: true });
  if (d.code !== 0) throw new ToolError("the build changed tracked files; the build is not trusted");
  if (lockfileSha(dest, plugin.pluginDir) !== stamp.lockSha256) throw new ToolError("the build changed the lockfile; the build is not trusted");
  return { root: dest, pluginDir, commit: v.commit, tree: v.tree, stamp };
}
