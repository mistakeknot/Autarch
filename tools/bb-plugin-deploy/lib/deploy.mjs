// The ordered phases (plan 2.3, 2.5, 2.6): prepare/check, deploy with automatic recovery, rollback, preview, restore-data
// and status publication. Every function runs under the destination lock held by the session.
import { existsSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import { buildCommit, newBuildDir, verifyStamp } from "./build.mjs";
import { clearMarkers, failureOf, hasMarker, runChecks } from "./checks.mjs";
import { restoreData, makeBackup } from "./data.mjs";
import { gcBuilds } from "./gc.mjs";
import { git, controlDiff } from "./gitops.mjs";
import { pollUntil, runHealth, snapshotBaselines } from "./health.mjs";
import { template } from "./pluginfile.mjs";
import { bareRecovery, recoveryBlock } from "./recovery.mjs";
import { addHistory, anchorPrevious, writeInstalledSha, writeRecord, resolveLive } from "./record.mjs";
import { behindOf, fetchMirror, publishStatus, readUpdateRequest, resultWanted, withholdStatus, writeResult } from "./publish.mjs";
import { baseVars, engineTrust, mkScriptCommand, refreshLive, requireReviewEvidence } from "./session.mjs";
import { EXIT, SHA40, ToolError, needsMk, refuse } from "./util.mjs";

const MODES = ["agent", "runner", "mk-script"];

const asToolError = (e) => (e instanceof ToolError ? e : new ToolError(String(e?.message ?? e)));

/** The pending click request, when the plugin file says where to look. */
export function readRequest(s) {
  const u = s.plugin.updateRequest;
  if (!u) return null;
  return readUpdateRequest({ db: template(u.db, baseVars(s)), key: u.key });
}

/** Write result (if given) first, then status.json last. Unknown installed or latest withholds the status. */
export function publishAll(s, result) {
  if (result) writeResult({ stateDir: s.stateDir, ...result });
  const live = resolveLive({ bb: s.bb, plugin: s.plugin });
  s.live = live;
  const installed = live.state === "unknown" ? null : live.commit;
  let latest = null;
  let behind = null;
  let warning = null;
  try {
    const m = fetchMirror({ stateDir: s.stateDir, repo: s.plugin.repo, branch: s.plugin.branch, lockFd: s.lockFd });
    latest = m.latest;
    if (m.error) warning = `mirror fetch failed: ${m.error}`;
    if (installed && latest) behind = behindOf({ mirror: m.mirror, installed, latest });
  } catch (e) {
    warning = `status computation failed: ${e.message}`;
  }
  const out = publishStatus({ stateDir: s.stateDir, installed, latest, behind });
  return { ...out, warning };
}

function tryPublish(s, result) {
  try {
    return publishAll(s, result);
  } catch (e) {
    try {
      withholdStatus(s.stateDir);
    } catch {
      /* best effort */
    }
    return { written: false, warning: `status publication failed: ${e.message}` };
  }
}

function checkCtx(s, vars) {
  return { vars, stateDir: s.stateDir, serverUrl: s.serverUrl, lockFd: s.lockFd, log: s.log };
}

function staticDataDirCheck(s, vars) {
  for (const d of s.plugin.data) {
    if (d.kind !== "sqlite") continue;
    const p = resolve(template(d.path, vars));
    if (!p.startsWith(`${s.bbData}/`)) throw refuse(`data-dir mismatch: ${d.name} is ${p}, outside the bound bb data directory ${s.bbData}`);
  }
}

function validateArgs(s, { commit, cls, mode }) {
  if (!SHA40.test(commit ?? "")) throw refuse("--commit must be a full 40-hex sha");
  if (!MODES.includes(mode)) throw refuse(`--mode must be one of ${MODES.join(", ")}`);
  const classes = s.plugin.classes;
  if (cls === undefined || cls === null || cls === "") throw refuse("--class is required");
  if (mode !== "mk-script" && !classes.includes(cls)) throw refuse(classes.length === 0 ? `class ${cls} is not approved: this plugin file lists no classes, so only mk-script deploys are allowed` : `class ${cls} is not approved (approved: ${classes.join(", ")})`);
}

/**
 * Trust, ancestry, build and control-path diff, shared by check, prepare, deploy and preview.
 * @returns {{trust:object, target:object, control:string[]|null, firstRun:object|null, scratch:boolean}}
 */
export function prepareTarget(s, o, { scratch = false } = {}) {
  validateArgs(s, o);
  const { commit, cls, mode } = o;
  const trust = engineTrust(s, { mode, commit, expectTree: o.expectTree, confirmedFirstRun: o.confirmedFirstRun });
  const warnings = [];
  if (mode === "mk-script") {
    warnings.push("ancestry of origin/main not enforced in mk-script mode");
  } else {
    const m = fetchMirror({ stateDir: s.stateDir, repo: s.plugin.repo, branch: s.plugin.branch, lockFd: s.lockFd });
    if (m.error || !m.latest) throw refuse(`cannot fetch ${s.plugin.branch}: ${m.error ?? "no head"}`);
    const has = git(m.mirror, ["cat-file", "-e", `${commit}^{commit}`], { allowFail: true });
    if (has.code !== 0) throw refuse(`${commit} is not on ${s.plugin.branch}`);
    const anc = git(m.mirror, ["merge-base", "--is-ancestor", commit, `refs/remotes/origin/${s.plugin.branch}`], { allowFail: true });
    if (anc.code !== 0) throw refuse(`${commit} is not an ancestor of origin/${s.plugin.branch}`);
  }
  if (mode === "runner") requireReviewEvidence(s, commit);
  const vars0 = baseVars(s, { class: cls });
  const dest = scratch ? join(s.stateDir, "scratch", `${commit.slice(0, 12)}-${process.pid}`) : newBuildDir(s.dirs.buildsRoot, s.plugin.pluginId, commit, s.env);
  let target;
  try {
    target = buildCommit({ plugin: s.plugin, commit, expectTree: o.expectTree, dest, toolCommit: trust.toolCommit, vars: vars0, lockFd: s.lockFd, log: s.log });
  } catch (e) {
    if (scratch) rmSync(dest, { recursive: true, force: true });
    throw e;
  }
  const installedCommit = s.live?.state === "unknown" ? null : s.live?.commit;
  const control = controlDiff({ repoDir: target.root, from: installedCommit, to: commit, controlPaths: s.plugin.controlPaths });
  const needsPause = control === null || control.length > 0;
  if (needsPause && mode !== "mk-script" && !trust.firstRun) {
    const a = (installedCommit ?? "unknown").slice(0, 12);
    const msg = `deploy tooling changed between ${a} and ${commit.slice(0, 12)}: needs mk's reviewed install or a confirmed first run. To install by mk script:\n${mkScriptCommand(s, ["deploy", "--commit", commit, "--class", cls, "--mode", "mk-script"])}`;
    if (!scratch) {
      addHistory(s.rec, "pause", commit, msg.split("\n")[0]);
      writeRecord(s.stateDir, s.rec);
    }
    const err = needsMk(msg, { paused: true, control, target });
    if (scratch) rmSync(dest, { recursive: true, force: true });
    throw err;
  }
  return { trust, target, control, firstRun: trust.firstRun ?? null, scratch, warnings };
}

function targetVars(s, o, target) {
  return baseVars(s, { class: o.cls ?? "none", targetRoot: target.root, targetPluginDir: target.pluginDir, targetCommit: target.commit, buildDir: target.root });
}

function recoverTo(s, anchored) {
  const out = { ran: true, ok: false, detail: "" };
  try {
    try {
      s.bb.disable();
    } catch {
      /* the disable may already have happened */
    }
    s.bb.install(anchored.pluginDir);
    let healthy = true;
    if (s.plugin.rollbackLeaves === "enabled") {
      s.bb.enable();
      healthy = !!pollUntil(() => s.bb.isRunning(), 60, s.env);
      if (!healthy) out.detail = "the previous version did not come up running";
    }
    const src = s.bb.sourceDir();
    if (src !== anchored.pluginDir) {
      healthy = false;
      out.detail = `bb reports source ${src}, expected ${anchored.pluginDir}`;
    }
    out.ok = healthy;
  } catch (e) {
    out.detail = `recovery failed: ${e.message}`;
  }
  return out;
}

function recoveryText(s, anchored) {
  const args = { serverUrl: s.serverUrl, pluginId: s.plugin.pluginId, anchoredBuild: anchored.buildRoot, pluginDirRel: s.plugin.pluginDir, rollbackLeaves: s.plugin.rollbackLeaves };
  return { lastResort: recoveryBlock({ ...args, stateDir: s.stateDir }), bare: bareRecovery(args) };
}

function entryFor(target, o, backup, toolCommit, env) {
  return { commit: target.commit, tree: target.tree, buildRoot: target.root, pluginDir: target.pluginDir, installedAt: new Date().toISOString(), by: { mode: o.mode, actor: env.BPD_ACTOR || "unknown" }, backup: backup?.dir ?? null, toolCommit: toolCommit ?? null };
}

/** The whole deploy. Returns a report with `exit`; never throws for an expected failure. */
export function deploy(s, o) {
  const report = { command: "deploy", ok: false, exit: EXIT.ERROR, commit: o.commit, class: o.cls, mode: o.mode, gates: [], phases: [], warnings: [], message: "" };
  let mutated = false;
  let anchored = null;
  let target = null;
  let trust = null;
  let backup = null;
  const request = (() => {
    try {
      return readRequest(s);
    } catch {
      return null;
    }
  })();
  const wantResult = SHA40.test(o.commit ?? "") && resultWanted({ mode: o.mode, sha: o.commit, request });
  const phase = (name) => report.phases.push(name);
  const gates = (results) => {
    for (const r of results) for (const g of r.gates) report.gates.push({ check: r.name, ...g });
  };
  const runPhase = (name, entries, vars) => {
    phase(name);
    const results = runChecks(entries, checkCtx(s, vars));
    gates(results);
    const f = failureOf(results, name);
    if (f) throw f;
  };
  try {
    clearMarkers(s.stateDir);
    phase("prepare");
    const prep = prepareTarget(s, o);
    trust = prep.trust;
    target = prep.target;
    report.warnings.push(...prep.warnings);
    if (prep.firstRun) report.firstRun = prep.firstRun;
    if (prep.control && prep.control.length > 0) report.controlChanged = prep.control;
    const vars = targetVars(s, o, target);
    staticDataDirCheck(s, vars);
    runPhase("preflight", s.plugin.preflight, vars);
    runPhase("preInstall", s.plugin.preInstall, vars);
    phase("anchor-previous");
    anchored = anchorPrevious({ plugin: s.plugin, live: s.live, buildsRoot: s.dirs.buildsRoot, vars, toolCommit: trust.toolCommit, lockFd: s.lockFd, log: s.log });
    report.previous = anchored;
    phase("backup");
    backup = makeBackup({ plugin: s.plugin, vars, backupsRoot: s.dirs.backupsRoot, stateDir: s.stateDir, commit: target.commit, previousPluginDir: anchored.pluginDir, serverUrl: s.serverUrl, lockFd: s.lockFd, env: s.env, log: s.log });
    report.backup = backup.dir;
    verifyStamp(target.root, s.plugin);
  } catch (e) {
    return failBefore(s, report, asToolError(e), o, wantResult, target);
  }

  // ---- mutation begins here: the first mutating bb call is the disable --------------------------------------
  const baselines = {};
  try {
    mutated = true;
    phase("disable");
    s.bb.disable();
    phase("install");
    s.bb.install(target.pluginDir);
    snapshotBaselines(s.plugin.health, targetVars(s, o, target), "install", baselines);
    const vars = targetVars(s, o, target);
    runPhase("preEnable", s.plugin.preEnable, vars);
    phase("enable");
    s.bb.enable();
    snapshotBaselines(s.plugin.health, vars, "enable", baselines);
    runPhase("postEnablePreConfig", s.plugin.postEnablePreConfig, vars);
    phase("configAfterEnable");
    for (const c of s.plugin.configAfterEnable) {
      if (c.requiresMarker && !hasMarker(s.stateDir, c.requiresMarker)) throw new ToolError(`config ${c.key} requires marker ${c.requiresMarker}, which no passing check wrote`);
      s.bb.setConfig(c.key, template(c.value, vars));
    }
    snapshotBaselines(s.plugin.health, vars, "config", baselines);
    phase("health");
    const h = runHealth({ health: s.plugin.health, vars, bb: s.bb, stateDir: s.stateDir, serverUrl: s.serverUrl, lockFd: s.lockFd, baselines, env: s.env, log: s.log });
    report.health = h.results;
    if (!h.ok) throw new ToolError(`health failed: ${h.results.filter((r) => !r.ok).map((r) => `${r.name}: ${r.detail}`).join("; ")}`);
  } catch (e) {
    return failAfter(s, report, asToolError(e), o, wantResult, anchored, target);
  }

  // ---- success ------------------------------------------------------------------------------------------------
  const rec = s.rec;
  rec.previous = anchored;
  rec.installed = entryFor(target, o, backup, trust.toolCommit, s.env);
  addHistory(rec, "install", target.commit, `${o.mode}${o.cls ? ` ${o.cls}` : ""}${prepFirstRunNote(report)}`);
  writeRecord(s.stateDir, rec);
  writeInstalledSha(s.stateDir, target.commit);
  const pub = tryPublish(s, wantResult ? { sha: target.commit, ok: true, message: `deployed ${target.commit.slice(0, 12)} (${o.mode})` } : null);
  report.status = pub;
  if (pub.warning) report.warnings.push(pub.warning);
  try {
    report.gc = gcBuilds({ buildsRoot: s.dirs.buildsRoot, pluginId: s.plugin.pluginId, keepN: s.plugin.retention.builds, installedRoot: target.root, previousRoot: anchored.buildRoot, apply: true });
  } catch (e) {
    report.warnings.push(`gc failed: ${e.message}`);
  }
  report.ok = true;
  report.exit = EXIT.OK;
  report.installed = rec.installed;
  report.message = `deployed ${target.commit.slice(0, 12)}`;
  report.rollbackCommand = mkScriptCommand(s, ["rollback", "--to", "previous", "--mode", "mk-script"]);
  report.restoreDataCommand = mkScriptCommand(s, ["restore-data", "--backup", backup.dir, "--mode", "mk-script"]);
  report.recovery = recoveryText(s, anchored);
  return report;
}

const prepFirstRunNote = (report) => (report.firstRun ? ` (confirmed first run ${report.firstRun.evidence})` : "");

function failBefore(s, report, err, o, wantResult, target) {
  report.exit = err.code === EXIT.REFUSED || err.code === EXIT.NEEDS_MK ? err.code : EXIT.ERROR;
  report.message = err.message;
  report.recovery = { ran: false };
  if (err.extra?.results) report.checks = err.extra.results;
  try {
    if (!err.extra?.paused && SHA40.test(o.commit ?? "")) {
      addHistory(s.rec, "fail", o.commit, `before install: ${err.message.split("\n")[0]}`);
      writeRecord(s.stateDir, s.rec);
    }
  } catch {
    /* best effort */
  }
  const pub = tryPublish(s, wantResult ? { sha: o.commit, ok: false, message: err.message.split("\n")[0] } : null);
  if (pub.warning) report.warnings.push(pub.warning);
  if (target && target.root && existsSync(target.root)) report.unusedBuild = target.root;
  report.nothingInstalled = true;
  return report;
}

function failAfter(s, report, err, o, wantResult, anchored, target) {
  const rec = recoverTo(s, anchored);
  report.recovery = rec;
  report.exit = rec.ok ? EXIT.ROLLED_BACK : EXIT.ROLLBACK_UNHEALTHY;
  report.message = `${err.message}${rec.ok ? " — rolled back to " + anchored.commit.slice(0, 12) : ` — rollback to ${anchored.commit.slice(0, 12)} FAILED: ${rec.detail}`}`;
  report.rolledBackTo = anchored;
  if (err.extra?.results) report.checks = err.extra.results;
  try {
    s.rec.installed = { ...anchored, installedAt: new Date().toISOString(), by: { mode: "recovery", actor: s.env.BPD_ACTOR || "unknown" }, backup: null, toolCommit: null };
    addHistory(s.rec, "fail", target.commit, `after install: ${err.message.split("\n")[0]}; recovery ${rec.ok ? "ok" : "unhealthy"}`);
    writeRecord(s.stateDir, s.rec);
    if (rec.ok) writeInstalledSha(s.stateDir, anchored.commit);
  } catch (e) {
    report.warnings.push(`record write failed: ${e.message}`);
  }
  const pub = tryPublish(s, wantResult ? { sha: target.commit, ok: false, message: report.message.split("\n")[0] } : null);
  if (pub.warning) report.warnings.push(pub.warning);
  report.recoveryBlock = recoveryText(s, anchored);
  return report;
}

/** `check`: everything deploy does before it touches bb, on a scratch build, plus a control-path report. */
export function check(s, o) {
  const report = { command: "check", ok: false, exit: EXIT.ERROR, commit: o.commit, class: o.cls, mode: o.mode ?? "agent", gates: [], warnings: [], message: "" };
  let prep = null;
  try {
    prep = prepareTarget(s, { ...o, mode: report.mode }, { scratch: true });
    const vars = targetVars(s, o, prep.target);
    staticDataDirCheck(s, vars);
    clearMarkers(s.stateDir);
    for (const [name, entries] of [["preflight", s.plugin.preflight], ["preInstall", s.plugin.preInstall]]) {
      const results = runChecks(entries, checkCtx(s, vars));
      for (const r of results) for (const g of r.gates) report.gates.push({ check: r.name, ...g });
      const f = failureOf(results, name);
      if (f) throw f;
    }
    if (s.live.state === "unknown") throw refuse(`cannot anchor the previous install: ${s.live.reason}`);
    report.ok = true;
    report.exit = EXIT.OK;
    report.control = prep.control;
    report.warnings.push(...prep.warnings);
    report.message = `${o.commit.slice(0, 12)} passes the pre-install checks`;
  } catch (e) {
    const err = asToolError(e);
    report.exit = err.code === EXIT.REFUSED || err.code === EXIT.NEEDS_MK ? err.code : EXIT.ERROR;
    report.message = err.message;
  } finally {
    if (prep) rmSync(prep.target.root, { recursive: true, force: true });
  }
  return report;
}

/** `prepare`: build only. The build stays in buildsRoot, verified and stamped, for a later deploy to reuse as evidence. */
export function prepare(s, o) {
  const prep = prepareTarget(s, o);
  return { command: "prepare", ok: true, exit: EXIT.OK, commit: o.commit, build: prep.target.root, pluginDir: prep.target.pluginDir, control: prep.control, message: `built ${o.commit.slice(0, 12)}` };
}

function rollbackTarget(s, o) {
  if (o.build) {
    const b = resolve(o.build);
    const base = join(s.dirs.buildsRoot, s.plugin.pluginId);
    if (!b.startsWith(`${base}/`)) throw refuse(`--build must be a directory under ${base}`);
    const st = verifyStamp(b, s.plugin);
    return { commit: st.commit, tree: st.tree, buildRoot: b, pluginDir: join(b, s.plugin.pluginDir) };
  }
  const prev = s.rec.previous;
  if (!prev) throw refuse("the record has no previous build");
  if (!prev.buildRoot || !prev.buildRoot.startsWith(`${join(s.dirs.buildsRoot, s.plugin.pluginId)}/`)) throw refuse(`the previous install (${prev.commit?.slice(0, 12)}) is not a verified tool build; pass --build DIR`);
  verifyStamp(prev.buildRoot, s.plugin);
  return prev;
}

/** `rollback`: disable, install an immutable verified build, enable. Never writes result.json. */
export function rollback(s, o) {
  const report = { command: "rollback", ok: false, exit: EXIT.ERROR, gates: [], warnings: [], message: "" };
  let tgt;
  try {
    tgt = rollbackTarget(s, o);
    if (s.live.state === "unknown") throw refuse(`cannot roll back: ${s.live.reason}`);
  } catch (e) {
    const err = asToolError(e);
    report.exit = err.code === EXIT.REFUSED ? err.code : EXIT.ERROR;
    report.message = err.message;
    return report;
  }
  const from = s.rec.installed;
  const r = recoverTo(s, tgt);
  report.recovery = r;
  if (!r.ok) {
    report.exit = EXIT.ROLLBACK_UNHEALTHY;
    report.message = `rollback to ${tgt.commit.slice(0, 12)} did not become healthy: ${r.detail}`;
    tryPublish(s, null);
    return report;
  }
  s.rec.previous = from ?? null;
  s.rec.installed = { commit: tgt.commit, tree: tgt.tree, buildRoot: tgt.buildRoot, pluginDir: tgt.pluginDir, installedAt: new Date().toISOString(), by: { mode: o.mode ?? "agent", actor: s.env.BPD_ACTOR || "unknown" }, backup: null, toolCommit: null };
  addHistory(s.rec, "rollback", tgt.commit, `from ${from?.commit?.slice(0, 12) ?? "unknown"}`);
  writeRecord(s.stateDir, s.rec);
  writeInstalledSha(s.stateDir, tgt.commit);
  const pub = tryPublish(s, null);
  if (pub.warning) report.warnings.push(pub.warning);
  report.ok = true;
  report.exit = EXIT.OK;
  report.installed = s.rec.installed;
  report.message = `rolled back to ${tgt.commit.slice(0, 12)}`;
  return report;
}

/** `preview`: previewGate checks, install the preview build, run `bb plugin dev`, always restore the previous build. */
export function preview(s, o) {
  const report = { command: "preview", ok: false, exit: EXIT.ERROR, gates: [], warnings: [], message: "" };
  let dir = o.path ? resolve(o.path) : null;
  let anchored;
  try {
    if (dir) {
      verifyStamp(dir, s.plugin);
    } else {
      validateArgs(s, { commit: o.commit, cls: "preview", mode: "mk-script" });
      const trust = engineTrust(s, { mode: "mk-script", commit: o.commit, expectTree: o.expectTree });
      dir = newBuildDir(s.dirs.buildsRoot, s.plugin.pluginId, o.commit, s.env);
      buildCommit({ plugin: s.plugin, commit: o.commit, expectTree: o.expectTree, dest: dir, toolCommit: trust.toolCommit, vars: baseVars(s, { class: "preview" }), lockFd: s.lockFd, log: s.log });
    }
    const st = verifyStamp(dir, s.plugin);
    const vars = baseVars(s, { class: "preview", targetRoot: dir, targetPluginDir: join(dir, s.plugin.pluginDir), targetCommit: st.commit, buildDir: dir });
    clearMarkers(s.stateDir);
    const results = runChecks(s.plugin.previewGate, checkCtx(s, vars));
    for (const r of results) for (const g of r.gates) report.gates.push({ check: r.name, ...g });
    const f = failureOf(results, "previewGate");
    if (f) throw f;
    anchored = anchorPrevious({ plugin: s.plugin, live: s.live, buildsRoot: s.dirs.buildsRoot, vars, toolCommit: null, lockFd: s.lockFd, log: s.log });
    report.rollbackHint = `bb plugin install --yes ${anchored.pluginDir}`;
    s.bb.disable();
    s.bb.install(join(dir, s.plugin.pluginDir));
    s.bb.enable();
    s.bb.dev(join(dir, s.plugin.pluginDir));
    report.ok = true;
    report.exit = EXIT.OK;
    report.message = "preview ended";
  } catch (e) {
    const err = asToolError(e);
    report.exit = err.code === EXIT.REFUSED || err.code === EXIT.NEEDS_MK ? err.code : EXIT.ERROR;
    report.message = err.message;
  } finally {
    if (anchored) {
      const r = recoverTo(s, anchored);
      report.restored = r;
      if (!r.ok) {
        report.ok = false;
        report.exit = EXIT.ROLLBACK_UNHEALTHY;
        report.message += ` — restoring ${anchored.commit.slice(0, 12)} FAILED: ${r.detail}`;
      }
    }
  }
  return report;
}

/** `restore-data`: mk-only (unless --check). Port of home-ui.sh rollback --restore-data. */
export function restoreCommand(s, o) {
  const report = { command: "restore-data", ok: false, exit: EXIT.ERROR, warnings: [], message: "" };
  try {
    if (!o.check && o.mode !== "mk-script") throw refuse("restore-data replaces live data and is mk-only: pass --mode mk-script");
    if (!o.backup) throw refuse("--backup DIR is required");
    const vars = baseVars(s, {});
    const r = restoreData({ plugin: s.plugin, vars, backupDir: resolve(o.backup), check: !!o.check, bb: s.bb, log: s.log });
    report.ok = true;
    report.exit = EXIT.OK;
    report.plan = r.plan;
    report.previous = r.previous;
    report.message = o.check ? "restore-data check passed" : "data restored";
    if (!o.check) {
      refreshLive(s, { write: true });
      const pub = tryPublish(s, null);
      if (pub.warning) report.warnings.push(pub.warning);
    }
  } catch (e) {
    const err = asToolError(e);
    report.exit = err.code === EXIT.REFUSED ? err.code : EXIT.ERROR;
    report.message = err.message;
  }
  return report;
}

