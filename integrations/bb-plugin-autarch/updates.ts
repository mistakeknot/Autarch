// Update availability for Home. An outside runner (a systemd user unit as mk) fetches Autarch main,
// writes status.json beside result.json in the update directory, and acts on the request row this
// plugin stores. The plugin only reads those two files and writes one request row to its own store;
// it never executes anything.
import { readFileSync } from "node:fs";
import { join } from "node:path";

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
  if (!str(v.sha, 40) || !SHA.test(v.sha) || typeof v.ok !== "boolean" || !str(v.finished_at, 40) || !str(v.message, 2000)) return null;
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

export interface UpdateInfo {
  status: UpdateStatus | null;
  result: UpdateResult | null;
  request: UpdateRequest | null;
  /** True when a newer main exists, no request is waiting for it and the last result is not for it. */
  available: boolean;
  pending: boolean;
}

export function updateInfo(dir: string, requestRaw: string | undefined): UpdateInfo {
  const status = readStatus(dir);
  const result = readResult(dir);
  const request = parseRequest(requestRaw);
  const newer = status !== null && status.count > 0 && status.latest !== status.installed;
  // A request is pending until the runner reports a result for that sha (or the installed commit is it).
  const pending = request !== null && status !== null && status.installed !== request.sha && !(result && result.sha === request.sha && Date.parse(result.finished_at) >= Date.parse(request.clicked_at));
  return { status, result, request, available: newer && !pending, pending };
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
