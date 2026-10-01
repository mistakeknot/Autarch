// Root runs, display only (plan 1.5, Task 2.8). Home shows the command mk would paste into Aleph's
// decision-panel adapter and the read-only status command to paste (no automatic status read until mk rules on D-1). It never executes anything, never writes
// Aleph state, and makes no approval claim: the card text can be written by any same-uid agent, so
// everything read from a card, from a script file and from the runner's HTTP answer is untrusted.
// Slice A is "run by paste, not authenticated"; approval is a later slice's passkey page, not here.
//
// File reads here are O_RDONLY only (write-scope.test.ts); there is no process execution (no-exec.test.ts).
import { createHash } from "node:crypto";
import { closeSync, constants, fstatSync, lstatSync, openSync, readSync } from "node:fs";
import { askingThread, parseCard, type RootRun, type TaskComment } from "./cards.js";

export const BADGE = "run by paste, not authenticated";
export const MAX_SCRIPT_BYTES = 256 * 1024;

/** Aleph's grammars, stricter than the card's own (plan 1.2) where the Aleph interface is stricter. */
const SET_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;
const THREAD_RE = /^thr_[a-z0-9]{1,64}$/;
const TASK_ID_RE = /^[0-9A-HJKMNP-TV-Z]{26}$/;
const PROJECT_ID_RE = /^[A-Za-z0-9:_.-]{1,128}$/;
const SHA_RE = /^[0-9a-f]{64}$/;
const COMMENT_ID_MAX = 128;
const CANON_TIME = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/;

export type ScriptState = "match" | "mismatch" | "unreadable" | "invalid";
export interface ScriptCheck {
  state: ScriptState;
  actual: string | null;
  why?: string;
}

/** Open the script O_RDONLY|O_NOFOLLOW after lstat, require a regular file of at most 256 KiB, hash it. */
export function scriptState(path: string, expectedSha: string): ScriptCheck {
  if (!SHA_RE.test(expectedSha) || !path.startsWith("/")) return { state: "invalid", actual: null, why: "the tuple is not valid" };
  let fd: number | null = null;
  try {
    const l = lstatSync(path);
    if (!l.isFile()) return { state: "unreadable", actual: null, why: "not a regular file (a symlink counts)" };
    if (l.size > MAX_SCRIPT_BYTES) return { state: "unreadable", actual: null, why: "larger than 256 KiB" };
    fd = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW);
    const s = fstatSync(fd);
    if (!s.isFile() || s.ino !== l.ino || s.dev !== l.dev) return { state: "unreadable", actual: null, why: "the file changed while it was opened" };
    const buf = Buffer.alloc(MAX_SCRIPT_BYTES + 1);
    let n = 0;
    for (;;) {
      const r = readSync(fd, buf, n, buf.length - n, null);
      if (r === 0) break;
      n += r;
      if (n > MAX_SCRIPT_BYTES) return { state: "unreadable", actual: null, why: "larger than 256 KiB" };
    }
    const actual = createHash("sha256").update(buf.subarray(0, n)).digest("hex");
    return actual === expectedSha ? { state: "match", actual } : { state: "mismatch", actual };
  } catch (e) {
    return { state: "unreadable", actual: null, why: `cannot read: ${(e as NodeJS.ErrnoException).code ?? "error"}` };
  } finally {
    if (fd !== null) {
      try {
        closeSync(fd);
      } catch {
        /* nothing to do */
      }
    }
  }
}

/** Single-quote one shell argument. Refuses a quote, control characters, non-ASCII and the empty string. */
export function shQuote(s: string): string {
  if (s.length === 0) throw new Error("cannot quote an empty argument");
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (ch === "'" || c < 0x20 || c > 0x7e) throw new Error("argument has a quote, control character or non-ASCII character");
  }
  return `'${s}'`;
}

/** The card file the adapter reads. Home does not write it; mk saves the card's JSON there first. */
export const cardFileName = (taskId: string): string => `card-${taskId}.json`;

/**
 * Q1 (pending mk; default b = a paste command only): the one place that builds the command mk pastes.
 * `todo-add --from-card <file>` reads task.id, projectId and the root-run block from the saved card,
 * takes owner_thread from the earliest agent comment, re-pins the script bytes and never executes.
 */
export function pasteCommand(i: { set: string; task_id: string }): string {
  return `todo-add --set ${shQuote(i.set)} --from-card ${shQuote(cardFileName(i.task_id))}`;
}

/** The run item Aleph builds from the card, shown so mk can see what will be pinned. */
export function itemJson(rr: RootRun, ownerThread: string, taskId: string, attempt: number): string {
  return JSON.stringify({ run_as: "mk", script: rr.script, script_sha256: rr.sha256, owner_thread: ownerThread, run_timeout_s: rr.timeout, label: `bbtask:${taskId}:${attempt}` });
}

/** Nanoseconds since the epoch of a canonical UTC time, or null for anything else. */
function canonNs(s: string): bigint | null {
  const m = CANON_TIME.exec(s);
  if (!m) return null;
  const [y, mo, d, h, mi, se] = [1, 2, 3, 4, 5, 6].map((k) => Number(m[k]));
  const ms = Date.UTC(y!, mo! - 1, d!, h!, mi!, se!);
  const back = new Date(ms);
  if (back.getUTCFullYear() !== y || back.getUTCMonth() !== mo! - 1 || back.getUTCDate() !== d || back.getUTCHours() !== h || back.getUTCMinutes() !== mi || back.getUTCSeconds() !== se) return null;
  return BigInt(ms) * 1_000_000n + BigInt((m[7] ?? "").padEnd(9, "0") || "0");
}

/**
 * The owner thread as Aleph will read it, or why it cannot be named. Aleph refuses a malformed or tied
 * earliest agent comment, so Home refuses to show a command in those cases too; the thread itself is
 * askingThread(), and it must agree with the strict, numeric ordering used here.
 */
function ownerThread(comments: readonly TaskComment[]): { ok: true; thread: string } | { ok: false; why: string } {
  const agents = comments.filter((c) => c.kind === "agent");
  if (agents.length === 0) return { ok: false, why: "no agent comment names the asking thread" };
  const keyed: { c: TaskComment; ns: bigint }[] = [];
  for (const c of agents) {
    if (typeof c.id !== "string" || c.id.length === 0 || c.id.length > COMMENT_ID_MAX) return { ok: false, why: "an agent comment has an empty or oversized id" };
    const ns = typeof c.createdAt === "string" ? canonNs(c.createdAt) : null;
    if (ns === null) return { ok: false, why: "an agent comment has a createdAt that is not canonical UTC" };
    keyed.push({ c, ns });
  }
  const min = keyed.reduce((a, b) => (b.ns < a ? b.ns : a), keyed[0]!.ns);
  const earliest = keyed.filter((k) => k.ns === min);
  if (earliest.length !== 1) return { ok: false, why: "the earliest agent comment is tied" };
  const thread = earliest[0]!.c.threadId;
  if (thread === null || !THREAD_RE.test(thread)) return { ok: false, why: "the earliest agent comment's thread id is missing or not a lowercase thr_ id" };
  if (askingThread(comments) !== thread) return { ok: false, why: "askingThread disagrees with the strict ordering" };
  return { ok: true, thread };
}

// ---- status: a read-only GET on the runner, answer treated as untrusted display data -------------------

export const RUNNER_ADDR = "127.0.0.1:8744";
const MAX_ATTEMPTS = 5;
const MAX_STATUS_BYTES = 16 * 1024;
const PHASES = new Set(["launching", "running", "finalizing"]);
const TERMINALS = new Set(["exited", "killed", "timeout", "launch_failed", "interrupted"]);
export const REASONS = new Set(["no-scope", "ready-timeout", "start-not-confirmed", "left-processes", "supervisor-died", "other"]);

export type RunStatus =
  | { kind: "none" }
  | { kind: "unavailable"; why: string }
  | {
      kind: "run";
      attempt: number;
      item: string;
      phase: string | null;
      terminal: boolean | string | null;
      exec_report: string | null;
      exit: number | null;
      signal: string | null;
      reason: string | null;
      started: string | null;
      ended: string | null;
      log_complete: boolean | null;
      owner_alive: boolean | null;
    };

type FetchLike = (url: string | URL, init?: { signal?: AbortSignal; redirect?: "manual" | "error" | "follow"; headers?: Record<string, string> }) => Promise<Response>;
/**
 * D-1 (pending mk's ruling; plan 1.5 and 7): until mk rules, the default is option (b): Home shows a `todo-run --status`
 * paste command and makes NO request to the runner. The HTTP read in fetchRunStatus stays behind this one switch,
 * OFF; turning it on is a decision for mk, not a code change to slip in. A test pins it to false.
 */
export const AUTO_READ_RUNNER_STATUS = false;

export interface StatusOptions {
  fetch?: FetchLike;
  timeoutMs?: number;
  /** Opt in to the HTTP status read. Defaults to AUTO_READ_RUNNER_STATUS (false). */
  autoRead?: boolean;
}

/** The read-only command mk pastes to see a run's status (plan 1.5): `todo-run --status '<set>' '<item>'`, first attempt. */
export function statusCommand(set: string, taskId: string): string {
  return `todo-run --status ${shQuote(set)} ${shQuote(`bbtask-${taskId}-1`)}`;
}

const rec = (v: unknown): v is Record<string, unknown> => typeof v === "object" && v !== null && !Array.isArray(v);
const bool = (v: unknown): boolean | null => (typeof v === "boolean" ? v : null);
const time = (v: unknown): string | null => (typeof v === "string" && canonNs(v) !== null ? v : null);

function shape(r: Record<string, unknown>, attempt: number, item: string): Extract<RunStatus, { kind: "run" }> {
  const phase = r.phase === null || r.phase === undefined ? null : typeof r.phase === "string" && PHASES.has(r.phase) ? r.phase : "unknown";
  const terminal = r.terminal === null || r.terminal === undefined ? null : typeof r.terminal === "boolean" ? r.terminal : typeof r.terminal === "string" && TERMINALS.has(r.terminal) ? r.terminal : "unknown";
  const reason = r.reason === null || r.reason === undefined ? null : typeof r.reason === "string" && REASONS.has(r.reason) ? r.reason : "other";
  return {
    kind: "run",
    attempt,
    item,
    phase,
    terminal,
    exec_report: typeof r.exec_report === "string" && /^[a-z0-9_-]{1,32}$/.test(r.exec_report) ? r.exec_report : null,
    exit: typeof r.exit === "number" && Number.isInteger(r.exit) && r.exit >= 0 && r.exit <= 255 ? r.exit : null,
    signal: typeof r.signal === "string" && /^[A-Z0-9]{2,16}$/.test(r.signal) ? r.signal : null,
    reason,
    started: time(r.started),
    ended: time(r.ended),
    log_complete: bool(r.log_complete),
    owner_alive: bool(r.owner_alive),
  };
}

/**
 * GET http://127.0.0.1:8744/api/run/<set>/<item>.json for attempt 1, 2, ... until the runner says there is
 * no record. Anything but 200 or 404, an unreachable runner, a slow one, an oversized or ill-formed answer,
 * or a record that does not link back to this card is "unavailable". It never throws.
 */
export async function fetchRunStatus(set: string, taskId: string, opts: StatusOptions = {}): Promise<RunStatus> {
  const f = opts.fetch ?? (globalThis.fetch as FetchLike);
  let latest: Extract<RunStatus, { kind: "run" }> | null = null;
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    const item = `bbtask-${taskId}-${attempt}`;
    try {
      const res = await f(`http://${RUNNER_ADDR}/api/run/${encodeURIComponent(set)}/${encodeURIComponent(item)}.json`, { signal: AbortSignal.timeout(opts.timeoutMs ?? 2000), redirect: "manual" });
      if (res.status === 404) break;
      if (res.status !== 200) return { kind: "unavailable", why: `the runner answered ${res.status >= 100 && res.status < 600 ? res.status : "an unexpected status"}` };
      const text = await res.text();
      if (text.length > MAX_STATUS_BYTES) return { kind: "unavailable", why: "the runner's answer was too large" };
      const body: unknown = JSON.parse(text);
      if (!rec(body)) return { kind: "unavailable", why: "the runner's answer was not a record" };
      if (body.set !== set || body.item !== item || body.label !== `bbtask:${taskId}:${attempt}`) return { kind: "unavailable", why: "the runner's record does not link back to this card" };
      latest = shape(body, attempt, item);
    } catch {
      return { kind: "unavailable", why: "the runner could not be read" };
    }
  }
  return latest ?? { kind: "none" };
}

// ---- the view -----------------------------------------------------------------------------------------

export interface RootRunInput {
  task: { id: string; projectId: string; description: string };
  comments: readonly TaskComment[];
}
export interface RootRunProblem {
  field: string;
  why: string;
}
export interface RootRunView {
  /** False when the card has no root-run block: nothing to show. */
  present: boolean;
  badge: string;
  state: ScriptState | null;
  problems: RootRunProblem[];
  tuple: RootRun | null;
  actual_sha256: string | null;
  owner_thread: string | null;
  item_json: string | null;
  command: string | null;
  card_file: string | null;
  /** Null unless the opt-in HTTP read (AUTO_READ_RUNNER_STATUS, D-1) is on. */
  status: RunStatus | null;
  /** The paste command that shows the run's status (D-1 default b). */
  status_command: string | null;
}

const EMPTY: RootRunView = { present: false, badge: BADGE, state: null, problems: [], tuple: null, actual_sha256: null, owner_thread: null, item_json: null, command: null, card_file: null, status: null, status_command: null };

/** The first attempt Home shows in the item JSON. Aleph numbers attempts; this is for display only. */
const DISPLAY_ATTEMPT = 1;

/**
 * Plan 1.5: strict parse, check the script bytes, then decide whether the paste command renders.
 * It renders only when the state is match, set and thread are valid, task.id and projectId are valid.
 * Otherwise the view names each field that failed and carries no command.
 */
export async function rootRun(input: RootRunInput, opts: StatusOptions = {}): Promise<RootRunView> {
  const { task, comments } = input;
  let card;
  try {
    card = parseCard(task.description);
  } catch (e) {
    if (!/^```root-run[ \t]*$/m.test(task.description.replaceAll("\r\n", "\n"))) return { ...EMPTY };
    return { ...EMPTY, present: true, state: "invalid", problems: [{ field: "card", why: (e as Error).message }] };
  }
  const rr = card.root_run;
  if (rr === null) return { ...EMPTY };

  const problems: RootRunProblem[] = [];
  const check = scriptState(rr.script, rr.sha256);
  if (check.state !== "match") {
    const why =
      check.state === "mismatch" ? "the file's sha256 differs from the card's" : (check.why ?? "the script could not be checked");
    problems.push({ field: "script", why });
  }
  if (!SET_RE.test(rr.set)) problems.push({ field: "set", why: "must be lowercase letters, digits and dashes, starting with a letter or digit" });
  const owner = ownerThread(comments);
  if (!owner.ok) problems.push({ field: "owner_thread", why: owner.why });
  const idOk = TASK_ID_RE.test(task.id);
  if (!idOk) problems.push({ field: "task.id", why: "is not a 26-character task id" });
  if (!PROJECT_ID_RE.test(task.projectId)) problems.push({ field: "projectId", why: "must match [A-Za-z0-9:_.-]{1,128}" });

  const ok = problems.length === 0 && owner.ok;
  const statusable = idOk && SET_RE.test(rr.set);
  const status = statusable && (opts.autoRead ?? AUTO_READ_RUNNER_STATUS) ? await fetchRunStatus(rr.set, task.id, opts) : null;
  return {
    present: true,
    badge: BADGE,
    state: check.state,
    problems,
    tuple: rr,
    actual_sha256: check.state === "mismatch" ? check.actual : null,
    owner_thread: owner.ok ? owner.thread : null,
    item_json: ok ? itemJson(rr, owner.thread, task.id, DISPLAY_ATTEMPT) : null,
    command: ok ? pasteCommand({ set: rr.set, task_id: task.id }) : null,
    card_file: ok ? cardFileName(task.id) : null,
    status,
    status_command: statusable ? statusCommand(rr.set, task.id) : null,
  };
}
