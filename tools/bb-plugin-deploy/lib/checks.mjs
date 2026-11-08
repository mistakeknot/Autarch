// Check protocol (plan 2.3): argv, no shell, fixed PATH. Exit 0 pass, 10 refused, 20 needs mk, anything else an error.
// stdout JSON lines {"gate","ok","reason"} are copied into the report. Markers are written only after a pass.
import { existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { childEnv } from "./bb.mjs";
import { templateArgv } from "./pluginfile.mjs";
import { EXIT, ToolError, needsMk, nowIso, refuse, run, writeFileAtomic } from "./util.mjs";

export const markersDir = (stateDir) => join(stateDir, "markers");
export const markerEnvName = (name) => `BPD_MARKER_${name.toUpperCase()}`;

export function markerEnv(stateDir) {
  const out = {};
  const d = markersDir(stateDir);
  if (existsSync(d)) for (const f of readdirSync(d)) if (/^[A-Za-z0-9_]{1,64}$/.test(f)) out[markerEnvName(f)] = "1";
  return out;
}
export function clearMarkers(stateDir) {
  rmSync(markersDir(stateDir), { recursive: true, force: true });
  mkdirSync(markersDir(stateDir), { recursive: true, mode: 0o700 });
}
export const hasMarker = (stateDir, name) => existsSync(join(markersDir(stateDir), name));

export function parseGates(stdout) {
  const gates = [];
  for (const line of stdout.split("\n")) {
    const t = line.trim();
    if (!t.startsWith("{")) continue;
    try {
      const v = JSON.parse(t);
      if (v && typeof v.gate === "string" && typeof v.ok === "boolean") gates.push({ gate: v.gate.slice(0, 100), ok: v.ok, reason: typeof v.reason === "string" ? v.reason.slice(0, 500) : "" });
    } catch {
      /* not a gate line */
    }
  }
  return gates;
}

/**
 * Run one check entry ({name, run, marker?}). Never throws for a failing check; returns the outcome.
 * @returns {{name:string, status:"pass"|"refused"|"needs-mk"|"error", code:number|null, gates:object[], detail:string}}
 */
export function runCheck({ entry, vars, stateDir, serverUrl, lockFd, log = () => {} }) {
  const argv = templateArgv(entry.run, vars);
  log(`check ${entry.name}: ${argv.join(" ")}`);
  const env = childEnv({ serverUrl, lockFd, extra: { ...markerEnv(stateDir), BPD_PHASE_CHECK: entry.name.toUpperCase().replace(/[^A-Z0-9_]/g, "_") } });
  const r = run(argv, { cwd: vars.targetRoot ?? vars.installedRoot ?? stateDir, env, fds: lockFd ? [lockFd] : [], timeoutMs: 10 * 60_000 });
  const gates = parseGates(r.stdout);
  const status = r.code === 0 ? "pass" : r.code === EXIT.REFUSED ? "refused" : r.code === EXIT.NEEDS_MK ? "needs-mk" : "error";
  const firstBad = gates.find((g) => !g.ok);
  const detail = r.error ? String(r.error.message) : firstBad ? `${firstBad.gate}: ${firstBad.reason}` : (r.stderr || r.stdout).trim().slice(-300);
  if (entry.marker) {
    const p = join(markersDir(stateDir), entry.marker);
    if (status === "pass") writeFileAtomic(p, `${nowIso()}\n`);
    else rmSync(p, { force: true });
  }
  return { name: entry.name, status, code: r.code, gates, detail };
}

/** Run entries in order; stops at the first one that does not pass. */
export function runChecks(entries, ctx) {
  const results = [];
  for (const entry of entries) {
    const r = runCheck({ ...ctx, entry });
    results.push(r);
    if (r.status !== "pass") break;
  }
  return results;
}

/** The ToolError for the first failing result, or null when every result passed. */
export function failureOf(results, phase) {
  const bad = results.find((r) => r.status !== "pass");
  if (!bad) return null;
  const msg = `${phase} check ${bad.name} ${bad.status === "refused" ? "refused" : bad.status === "needs-mk" ? "needs mk" : "failed"}: ${bad.detail}`;
  if (bad.status === "refused") return refuse(msg, { results });
  if (bad.status === "needs-mk") return needsMk(msg, { results });
  return new ToolError(msg, EXIT.ERROR, { results });
}
