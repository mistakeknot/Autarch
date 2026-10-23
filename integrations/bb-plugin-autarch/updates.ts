// Update availability for Home. An outside runner (a systemd user unit as mk) fetches Autarch main,
// writes status.json beside result.json in the update directory, and acts on the request row this
// plugin stores. The plugin only reads those two files and writes one request row to its own store;
// it never executes anything.
import { readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

/** The systemd user units that run the update runner; the panel names them when a request looks stuck. */
export const UPDATE_UNIT = "autarch-home-update.service";
export const UPDATE_TIMER = "autarch-home-update.timer";
/** The timer's OnUnitActiveSec is read when the timer file is there; this is the interval it was installed with. */
export const DEFAULT_CHECK_INTERVAL_MS = 5 * 60_000;
export const defaultTimerFile = () => join(homedir(), ".config", "systemd", "user", UPDATE_TIMER);

export interface UpdateStatus {
  installed: string;
  latest: string;
  count: number;
  subjects: string[];
  checked_at: string;
}
export interface UpdateResult {
  sha: string;
  ok: boolean;
  finished_at: string;
  message: string;
}
export interface UpdateRequest {
  sha: string;
  clicked_at: string;
}

const SHA = /^[0-9a-f]{40}$/;
const str = (v: unknown, max: number): v is string => typeof v === "string" && v.length <= max;

function readJson(file: string): unknown {
  try {
    return JSON.parse(readFileSync(file, "utf8"));
  } catch {
    return null;
  }
}

/** The runner's status file, or null when it is absent, unreadable or malformed (no button then). */
export function readStatus(dir: string): UpdateStatus | null {
  const v = readJson(join(dir, "status.json")) as Record<string, unknown> | null;
  if (!v || typeof v !== "object") return null;
  if (!str(v.installed, 40) || !SHA.test(v.installed) || !str(v.latest, 40) || !SHA.test(v.latest)) return null;
  if (typeof v.count !== "number" || !Number.isInteger(v.count) || v.count < 0 || v.count > 100000) return null;
  if (!Array.isArray(v.subjects) || v.subjects.length > 200 || !v.subjects.every((s) => str(s, 300))) return null;
  if (!str(v.checked_at, 40) || Number.isNaN(Date.parse(v.checked_at))) return null;
  return { installed: v.installed, latest: v.latest, count: v.count, subjects: v.subjects as string[], checked_at: v.checked_at };
}

/** The runner's last result, or null. */
export function readResult(dir: string): UpdateResult | null {
  const v = readJson(join(dir, "result.json")) as Record<string, unknown> | null;
  if (!v || typeof v !== "object") return null;
  if (!str(v.sha, 40) || !SHA.test(v.sha) || typeof v.ok !== "boolean" || !str(v.finished_at, 40) || Number.isNaN(Date.parse(v.finished_at)) || !str(v.message, 2000)) return null;
  return { sha: v.sha, ok: v.ok, finished_at: v.finished_at, message: v.message };
}

export function parseRequest(raw: string | undefined): UpdateRequest | null {
  if (!raw) return null;
  try {
    const v = JSON.parse(raw) as Record<string, unknown>;
    if (v && str(v.sha, 40) && SHA.test(v.sha) && str(v.clicked_at, 40)) return { sha: v.sha, clicked_at: v.clicked_at };
  } catch {
    /* none */
  }
  return null;
}

/** systemd time span such as "5min", "300s", "1h 30min" or a bare number of seconds, in ms; null when unparseable. */
export function parseSpan(raw: string): number | null {
  const MIN = 60_000;
  const DAY = 24 * 60 * MIN;
  const unit: Record<string, number> = {
    us: 0.001, usec: 0.001, "μs": 0.001, ms: 1, msec: 1, s: 1000, sec: 1000, second: 1000, seconds: 1000,
    m: MIN, min: MIN, minute: MIN, minutes: MIN, h: 60 * MIN, hr: 60 * MIN, hour: 60 * MIN, hours: 60 * MIN,
    d: DAY, day: DAY, days: DAY, w: 7 * DAY, week: 7 * DAY, weeks: 7 * DAY,
    M: 30.44 * DAY, month: 30.44 * DAY, months: 30.44 * DAY, y: 365.25 * DAY, year: 365.25 * DAY, years: 365.25 * DAY,
  };
  const text = raw.trim();
  if (!/^(\d+(\.\d+)?\s*[a-zμ]*\s*)+$/i.test(text)) return null;
  let total = 0;
  for (const m of text.matchAll(/(\d+(?:\.\d+)?)\s*([a-zμ]*)/gi)) {
    const u = m[2]!;
    const mult = u === "" ? 1000 : (unit[u] ?? (u === "M" ? undefined : unit[u.toLowerCase()]));
    if (mult === undefined) return null;
    total += Number(m[1]) * mult;
  }
  return total > 0 ? Math.round(total) : null;
}

/** How often the runner's timer fires: its OnUnitActiveSec if readable, else the named default. */
export function checkIntervalMs(timerFile: string): number {
  try {
    const line = readFileSync(timerFile, "utf8").split("\n").map((l) => l.trim()).find((l) => /^OnUnitActiveSec\s*=/.test(l));
    const ms = line ? parseSpan(line.slice(line.indexOf("=") + 1)) : null;
    if (ms !== null) return ms;
  } catch {
    /* default */
  }
  return DEFAULT_CHECK_INTERVAL_MS;
}

/**
 * True while another process holds an flock on the runner's lock file (the runner holds it for the
 * whole tick, including the install). Reads /proc/locks; runs nothing. False when it cannot tell.
 */
export function runnerBusy(lockFile: string, procLocks = "/proc/locks"): boolean {
  try {
    const st = statSync(lockFile, { bigint: true });
    // Linux dev_t: major in bits 8-19 and 32+, minor in bits 0-7 and 12-31 (the form /proc/locks prints).
    const major = ((st.dev >> 8n) & 0xfffn) | ((st.dev >> 32n) << 12n);
    const minor = (st.dev & 0xffn) | ((st.dev >> 12n) & ~0xffn);
    return readFileSync(procLocks, "utf8").split("\n").some((line) => {
      const f = line.trim().split(/\s+/);
      if (f[1] !== "FLOCK") return false;
      const m = /^([0-9a-f]+):([0-9a-f]+):(\d+)$/i.exec(f[5] ?? "");
      return m !== null && BigInt(parseInt(m[1]!, 16)) === major && BigInt(parseInt(m[2]!, 16)) === minor && BigInt(m[3]!) === st.ino;
    });
  } catch {
    return false;
  }
}

export interface UpdateEnv {
  /** Server time as an ISO string; the panel measures ages against it so it needs no clock of its own. */
  now: string;
  timerFile?: string;
  procLocks?: string;
}

export interface UpdateInfo {
  status: UpdateStatus | null;
  result: UpdateResult | null;
  request: UpdateRequest | null;
  /** True when a newer main exists, no request is waiting for it and it is not already installed (a failed run stays available to retry). */
  available: boolean;
  pending: boolean;
  now: string;
  /** Milliseconds between runner checks. */
  interval_ms: number;
  /** The runner holds its lock: it is checking or installing right now. */
  busy: boolean;
  log_path: string;
  unit: string;
}

export function updateInfo(dir: string, requestRaw: string | undefined, env: UpdateEnv = { now: new Date().toISOString() }): UpdateInfo {
  const rawStatus = readStatus(dir);
  const result = readResult(dir);
  // The runner writes status.json before it installs, so right after an install it still names the old
  // commit. A successful result written after that status is newer evidence: show it as installed at once.
  const settled = rawStatus !== null && result !== null && result.ok && Date.parse(result.finished_at) > Date.parse(rawStatus.checked_at);
  const status = rawStatus && settled && result ? { ...rawStatus, installed: result.sha, ...(result.sha === rawStatus.latest ? { count: 0, subjects: [] } : {}) } : rawStatus;
  const request = parseRequest(requestRaw);
  const newer = status !== null && status.count > 0 && status.latest !== status.installed;
  // A request is pending until the runner reports a result for that sha (or the installed commit is it).
  const pending = request !== null && status !== null && status.installed !== request.sha && !(result && result.sha === request.sha && Date.parse(result.finished_at) >= Date.parse(request.clicked_at));
  // A successful result for the latest commit settles it even if status.json has not caught up yet (a status checked after the result wins).
  const installedNow = result !== null && result.ok && status !== null && result.sha === status.latest && Date.parse(result.finished_at) > Date.parse(status.checked_at);
  const interval_ms = checkIntervalMs(env.timerFile ?? defaultTimerFile());
  return { status, result, request, available: newer && !pending && !installedNow, pending, now: env.now, interval_ms, busy: pending && runnerBusy(join(dir, "lock"), env.procLocks), log_path: join(dir, "runner.log"), unit: UPDATE_UNIT };
}

export type RequestOutcome = { ok: true; request: UpdateRequest } | { ok: false; error: string };

/** Validate a click against the runner's status; the caller stores the returned row. */
export function checkRequest(info: UpdateInfo, sha: string, at: string): RequestOutcome {
  if (!SHA.test(sha)) return { ok: false, error: "not a full commit sha" };
  if (!info.status) return { ok: false, error: "no update status from the runner" };
  if (info.pending) return { ok: false, error: "an update is already requested" };
  if (!info.available || sha !== info.status.latest) return { ok: false, error: "that is not the latest commit offered" };
  return { ok: true, request: { sha, clicked_at: at } };
}
