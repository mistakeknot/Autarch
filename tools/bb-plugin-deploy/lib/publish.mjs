// Status publication (plan 2.5): status.json, result.json and the pending update request, in the shapes the
// plugin's updates.ts reads. status.json is always written last; unknown state is withheld, never guessed.
import { existsSync, readFileSync, renameSync } from "node:fs";
import { join } from "node:path";
import { git, gitTry } from "./gitops.mjs";
import { SHA40, ToolError, run, utcStamp, writeFileAtomic } from "./util.mjs";

export const statusPath = (d) => join(d, "status.json");
export const resultPath = (d) => join(d, "result.json");

/** A bare mirror, fetch-only: the tool computes "latest" from this and never from the working builds. */
export function fetchMirror({ stateDir, repo, branch, lockFd }) {
  const mirror = join(stateDir, "mirror.git");
  if (!existsSync(mirror)) git(undefined, ["init", "--bare", "--quiet", mirror], { lockFd });
  const r = git(mirror, ["fetch", "--quiet", "--no-tags", repo, `+refs/heads/${branch}:refs/remotes/origin/${branch}`], { allowFail: true, lockFd });
  if (r.code !== 0) return { mirror, latest: null, error: (r.stderr || r.stdout).trim().slice(0, 300) };
  const latest = gitTry(mirror, ["rev-parse", `refs/remotes/origin/${branch}`]);
  return { mirror, latest: latest !== null && SHA40.test(latest) ? latest : null };
}

/** {count, subjects} for installed..latest, or null when installed is not in the mirror (cannot be computed). */
export function behindOf({ mirror, installed, latest }) {
  if (installed === latest) return { count: 0, subjects: [] };
  const has = git(mirror, ["cat-file", "-e", `${installed}^{commit}`], { allowFail: true });
  if (has.code !== 0) return null;
  const count = Number(git(mirror, ["rev-list", "--count", `${installed}..${latest}`]));
  if (!Number.isInteger(count) || count < 0 || count > 100000) return null;
  const subj = git(mirror, ["log", "--format=%s", "-n", "200", `${installed}..${latest}`]);
  const subjects = subj ? subj.split("\n").map((s) => s.slice(0, 300)) : [];
  return { count, subjects };
}

function readJson(p) {
  try {
    return JSON.parse(readFileSync(p, "utf8"));
  } catch {
    return null;
  }
}

/** Atomically move status.json aside so the plugin shows no button and no stale claim. */
export function withholdStatus(stateDir) {
  const p = statusPath(stateDir);
  if (!existsSync(p)) return null;
  const to = `${p}.withheld-${utcStamp()}`;
  renameSync(p, to);
  return to;
}

/**
 * Write status.json LAST in a publication. `installed` is the live commit (null/unknown withholds); `checked_at` is
 * never earlier than a result.finished_at, so a fresh status always outranks an older result in updateInfo.
 */
export function publishStatus({ stateDir, installed, latest, behind, now = new Date() }) {
  if (!installed || !SHA40.test(installed) || !latest || !SHA40.test(latest) || !behind) {
    return { withheld: withholdStatus(stateDir) ?? "nothing to withhold", written: false };
  }
  const res = readJson(resultPath(stateDir));
  let at = now.getTime();
  const fin = res && typeof res.finished_at === "string" ? Date.parse(res.finished_at) : NaN;
  if (!Number.isNaN(fin) && fin >= at) at = fin + 1000;
  const status = { installed, latest, count: behind.count, subjects: behind.subjects, checked_at: new Date(at).toISOString() };
  writeFileAtomic(statusPath(stateDir), JSON.stringify(status, null, 2) + "\n");
  return { written: true, status };
}

/** result.json for a runner deploy, or an agent deploy of the sha a pending click names. Never for rollbacks. */
export function writeResult({ stateDir, sha, ok, message, now = new Date() }) {
  if (!SHA40.test(sha)) throw new ToolError("internal: result needs a full sha");
  const res = { sha, ok, finished_at: now.toISOString(), message: String(message).slice(0, 2000) };
  writeFileAtomic(resultPath(stateDir), JSON.stringify(res, null, 2) + "\n");
  return res;
}
export const readResultFile = (stateDir) => readJson(resultPath(stateDir));

/** The pending click: settings_kv[key] in the plugin's own database, read-only. {sha, clicked_at} or null. */
export function readUpdateRequest({ db, key }) {
  if (!existsSync(db)) return null;
  const r = run(["sqlite3", "-readonly", db, `SELECT value FROM settings_kv WHERE key='${key.replace(/'/g, "''")}'`], { timeoutMs: 30_000 });
  if (r.code !== 0 || !r.stdout.trim()) return null;
  try {
    const v = JSON.parse(r.stdout.trim());
    return v && typeof v.sha === "string" && SHA40.test(v.sha) && typeof v.clicked_at === "string" ? { sha: v.sha, clicked_at: v.clicked_at } : null;
  } catch {
    return null;
  }
}

/** Whether a finished deploy writes result.json: runner always, agent only for the sha a click names. */
export function resultWanted({ mode, sha, request }) {
  if (mode === "runner") return true;
  if (mode === "agent") return request !== null && request !== undefined && request.sha === sha;
  return false;
}
