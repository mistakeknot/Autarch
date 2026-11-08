// One command's context: the plugin file, the resolved directories, the destination lock (taken before anything
// reads the installed build), the bound bb instance, the live resolution and the reconciled record. Also the trust
// rule for the engine itself (plan 2.2) and the two kinds of mk evidence (review, confirmed first run).
import { existsSync, readFileSync, realpathSync } from "node:fs";
import { join } from "node:path";
import { makeBb } from "./bb.mjs";
import { bindInstance, resolveDirs } from "./context.mjs";
import { git, gitTry, verifyClosure, verifyClosureAgainstHead } from "./gitops.mjs";
import { acquireLock } from "./lock.mjs";
import { loadPluginFile, template } from "./pluginfile.mjs";
import { readRecord, reconcile, resolveLive, writeInstalledSha, writeRecord } from "./record.mjs";
import { SHA40, ToolError, isObj, refuse, run } from "./util.mjs";

export const REVIEW_FORMAT = "bb-plugin-deploy/review-evidence@1";
export const FIRST_RUN_OPTION = "confirm-first-run";

/**
 * Lock first, then bind the instance, then look at what is live.
 * @param {{pluginFile:string, flags:object, env?:NodeJS.ProcessEnv, cliPath:string, engineRoot:string, log?:(s:string)=>void, write?:boolean}} a
 */
export function openSession({ pluginFile, flags, env = process.env, cliPath, engineRoot, log = () => {}, write = true }) {
  const plugin = loadPluginFile(pluginFile);
  const dirs = resolveDirs({ pluginId: plugin.pluginId, flags, env });
  const lock = acquireLock(dirs.stateDir, env);
  try {
    const bind = bindInstance({ bbData: dirs.bbData, pluginId: plugin.pluginId, env });
    const bb = makeBb({ serverUrl: bind.serverUrl, pluginId: plugin.pluginId, lockFd: lock.fd, log });
    const s = { plugin, pluginFile, dirs, stateDir: dirs.stateDir, bbData: dirs.bbData, lock, lockFd: lock.fd, bind, serverUrl: bind.serverUrl, bb, env, log, cliPath, engineRoot, rec: null, live: null, reconciled: null };
    refreshLive(s, { write });
    return s;
  } catch (e) {
    lock.release();
    throw e;
  }
}

export const closeSession = (s) => s.lock.release();

/** Re-read what bb says is live and bring the record in line with it. */
export function refreshLive(s, { write = true } = {}) {
  s.live = resolveLive({ bb: s.bb, plugin: s.plugin });
  const rec = readRecord(s.stateDir, s.plugin.pluginId);
  const r = reconcile(rec, s.live);
  s.rec = r.record;
  s.reconciled = r;
  if (write && r.changed) {
    writeRecord(s.stateDir, s.rec);
    writeInstalledSha(s.stateDir, s.live.commit);
  }
  return s.live;
}

const safeReal = (p) => {
  try {
    return realpathSync(p);
  } catch {
    return null;
  }
};

/** Placeholder values common to every phase. */
export function baseVars(s, extra = {}) {
  const eng = s.engineRoot;
  return {
    bbData: s.bbData,
    stateDir: s.stateDir,
    installedRoot: s.trustedRoot ?? s.live?.buildRoot ?? eng,
    installedPluginDir: s.live?.sourceDir ?? join(eng, s.plugin.pluginDir),
    installedCommit: s.live?.commit ?? s.rec?.installed?.commit,
    class: "none",
    ...extra,
  };
}

export function mkScriptCommand(s, args) {
  return ["node", s.cliPath, ...args, "--plugin-file", s.pluginFile, "--bb-data", s.bbData].join(" ");
}

/** The first sqlite data entry is Home's own database; confirmed-first-run evidence lives there. */
function homeDb(s) {
  const e = s.plugin.data.find((d) => d.kind === "sqlite");
  return e ? template(e.path, baseVars(s)) : null;
}

/**
 * A Citadel decision on which mk picked "confirm-first-run", naming exactly `commit` in its body. Read-only.
 * Design choice: the evidence argument is the decision id; the schema read is decisions + picks.
 */
export function verifyFirstRun(s, evidence, commit) {
  if (!/^[A-Za-z0-9_.:-]{1,100}$/.test(evidence ?? "")) throw refuse("--confirmed-first-run needs a decision id");
  const db = homeDb(s);
  if (!db || !existsSync(db)) throw refuse("cannot read the confirmed-first-run evidence: Home's database is not available");
  const sql = `SELECT d.body_json AS body, p.option_id AS opt, p."by" AS who FROM decisions d JOIN picks p ON p.decision_id = d.id WHERE d.id = '${evidence}' AND d.withdrawn_at IS NULL`;
  const r = run(["sqlite3", "-readonly", "-json", db, sql], { timeoutMs: 30_000 });
  let rows = [];
  try {
    rows = r.stdout.trim() ? JSON.parse(r.stdout) : [];
  } catch {
    rows = [];
  }
  const row = rows[0];
  if (!row) throw refuse(`no picked decision ${evidence} in Home's database`);
  if (row.opt !== FIRST_RUN_OPTION || row.who !== "mk") throw refuse(`decision ${evidence} does not carry mk's "${FIRST_RUN_OPTION}" pick`);
  if (!String(row.body).includes(commit)) throw refuse(`decision ${evidence} does not name the exact target commit ${commit}`);
  return { evidence, commit };
}

/** The runner's review condition (plan 2.2). */
export function requireReviewEvidence(s, sha) {
  const p = join(s.stateDir, "review-evidence", `${sha}.json`);
  const msg = `no review evidence for ${sha}; ask the coordinator to review it or install by mk script`;
  if (!existsSync(p)) throw refuse(msg);
  let v;
  try {
    v = JSON.parse(readFileSync(p, "utf8"));
  } catch {
    throw refuse(msg);
  }
  if (!isObj(v) || v.format !== REVIEW_FORMAT || v.commit !== sha || v.verdict !== "approve") throw refuse(msg);
  return v;
}

/**
 * The trust rule (plan 2.2). Outside mk-script mode the installed engine, plugin file and checks run, never the
 * target's. Returns {kind: "installed"|"bootstrap", toolCommit, firstRun?}.
 */
export function engineTrust(s, { mode, commit, expectTree, confirmedFirstRun }) {
  const eng = safeReal(s.engineRoot);
  const installed = s.live?.state !== "unknown" && s.live?.buildRoot ? safeReal(s.live.buildRoot) : null;
  const paths = s.plugin.controlPaths;
  const pf = safeReal(s.pluginFile);
  if (eng && installed && eng === installed) {
    if (s.live.state === "stamped") verifyClosure({ root: eng, commit: s.live.commit, controlPaths: paths });
    else verifyClosureAgainstHead({ root: eng, controlPaths: paths });
    if (!pf || !pf.startsWith(`${eng}/`)) throw refuse("the plugin file is not part of the installed build; the installed plugin file must be used");
    s.trustedRoot = s.live.buildRoot;
    return { kind: "installed", toolCommit: s.live.stamp?.toolCommit ?? s.live.commit };
  }
  let firstRun = null;
  if (confirmedFirstRun) firstRun = verifyFirstRun(s, confirmedFirstRun, commit);
  if (mode !== "mk-script" && !firstRun) {
    throw refuse(`the tool is not running from the installed build; use --mode mk-script, or --confirmed-first-run <decision> naming ${commit}`);
  }
  if (!eng) throw refuse("cannot resolve the tool's own location");
  const head = gitTry(eng, ["rev-parse", "HEAD"]);
  if (head === null || !SHA40.test(head)) throw refuse("a clone engine must be a git checkout");
  if (firstRun && mode !== "mk-script" && head !== commit) throw refuse(`a first run must use the engine of the target commit: engine HEAD ${head.slice(0, 12)}, target ${commit.slice(0, 12)}`);
  const tree = git(eng, ["rev-parse", "HEAD^{tree}"]);
  if (expectTree && tree !== expectTree && head === commit) throw refuse(`engine tree ${tree} differs from --expect-tree ${expectTree}`);
  verifyClosureAgainstHead({ root: eng, controlPaths: paths });
  const status = gitTry(eng, ["status", "--porcelain", "--untracked-files=all"]);
  if (status !== "") throw refuse("the clone engine is not clean");
  if (!pf || !pf.startsWith(`${eng}/`)) throw refuse("the plugin file must come from the verified clone engine");
  s.trustedRoot = eng;
  return { kind: "bootstrap", toolCommit: head, firstRun };
}

export { ToolError };
