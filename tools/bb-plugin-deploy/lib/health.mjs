// Health kinds (plan 2.3): running, log-line, process-count, check, setting-equals. Log baselines are byte offsets
// snapshotted at install, enable and config so an old process's lines never satisfy or fail a later check.
import { readFileSync, readdirSync, statSync } from "node:fs";
import { runCheck } from "./checks.mjs";
import { template } from "./pluginfile.mjs";
import { nowIso } from "./util.mjs";

export const pollMs = (env = process.env) => {
  const n = Number(env.BPD_POLL_MS);
  return Number.isFinite(n) && n >= 10 && n <= 5000 ? Math.floor(n) : 1000;
};
export function sleepSync(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

/** Poll fn() until it returns a truthy value or the timeout passes. Always tries once. */
export function pollUntil(fn, timeoutSec, env = process.env) {
  const end = Date.now() + timeoutSec * 1000;
  for (;;) {
    const v = fn();
    if (v) return v;
    if (Date.now() >= end) return null;
    sleepSync(pollMs(env));
  }
}

const sizeOf = (p) => {
  try {
    return statSync(p).size;
  } catch {
    return 0;
  }
};

/** Record the end of every log-line file whose `since` is `point`. Mutates and returns `baselines`. */
export function snapshotBaselines(health, vars, point, baselines) {
  for (const h of health) {
    if (h.kind !== "log-line" || h.since !== point) continue;
    const file = template(h.file, vars);
    baselines[h.name] = { file, offset: sizeOf(file), time: nowIso(), point };
  }
  return baselines;
}

function readFrom(file, offset) {
  try {
    const buf = readFileSync(file);
    return buf.subarray(offset > buf.length ? 0 : offset).toString("utf8");
  } catch {
    return "";
  }
}

function countProcesses(re) {
  let n = 0;
  for (const d of readdirSync("/proc")) {
    if (!/^[0-9]+$/.test(d) || Number(d) === process.pid) continue;
    try {
      const cmd = readFileSync(`/proc/${d}/cmdline`, "utf8").split("\0").join(" ").trim();
      if (cmd && re.test(cmd)) n++;
    } catch {
      /* exited */
    }
  }
  return n;
}

/**
 * Evaluate the health list in order; stops at the first failure.
 * @returns {{ok:boolean, results:{name:string, ok:boolean, detail:string}[]}}
 */
export function runHealth({ health, vars, bb, stateDir, serverUrl, lockFd, baselines, env = process.env, log = () => {} }) {
  const results = [];
  const done = (name, ok, detail) => {
    results.push({ name, ok, detail });
    return ok;
  };
  for (const h of health) {
    const t = h.timeoutSec ?? (h.kind === "running" ? 30 : 30);
    if (h.kind === "running") {
      const ok = pollUntil(() => bb.isRunning(), t, env);
      if (!done("running", !!ok, ok ? "enabled and running" : `not running after ${t}s`)) break;
    } else if (h.kind === "check") {
      const r = runCheck({ entry: h, vars, stateDir, serverUrl, lockFd, log });
      if (!done(h.name, r.status === "pass", `${r.status}${r.detail ? `: ${r.detail}` : ""}`)) break;
    } else if (h.kind === "setting-equals") {
      const want = template(h.value, vars);
      const ok = pollUntil(() => String(bb.configValues()[h.key]) === want, t, env);
      if (!done(`setting ${h.key}`, !!ok, ok ? `${h.key} = ${want}` : `${h.key} is not ${want}`)) break;
    } else if (h.kind === "process-count") {
      const re = new RegExp(h.pattern);
      let last = -1;
      const ok = pollUntil(() => ((last = countProcesses(re)) === h.count ? true : null), t, env);
      if (!done(h.name, !!ok, ok ? `${h.count} process(es) match` : `${last} process(es) match ${h.pattern}, want ${h.count}`)) break;
    } else if (h.kind === "log-line") {
      const base = baselines[h.name];
      if (!base) {
        done(h.name, false, `no baseline was taken at ${h.since}`);
        break;
      }
      const fails = (h.fail ?? []).map((p) => new RegExp(p));
      const pass = new RegExp(h.pattern);
      let verdict = null;
      pollUntil(() => {
        const text = readFrom(base.file, base.offset);
        const f = fails.find((re) => re.test(text));
        if (f) return (verdict = { ok: false, detail: `failure line matched ${f} after ${h.since}` });
        if (pass.test(text)) return (verdict = { ok: true, detail: `saw ${pass} after ${h.since}` });
        return null;
      }, t, env);
      if (!done(h.name, verdict?.ok === true, verdict?.detail ?? `no ${pass} line within ${t}s after ${h.since}`)) break;
    }
  }
  return { ok: results.length >= health.length && results.every((r) => r.ok), results };
}
