// A card for every plain message the operator types in the vizier thread (bead mk-ppvma.4).
//
// The plugin polls the vizier thread's `client/turn/requested` events, keeps the ones an operator typed, and files
// one plain tracker card per message over the tasks plugin's RPC (never a spawned process). The card is "read and
// close": the operator's words quoted, a link to the thread message, and a line saying to close it once read.
// The plugin does not judge whether a message needs a decision.
//
// Idempotency: a message is identified by its event id (and its request id, so a retried turn is the same message).
// The id is written to `settings_kv` only after the card exists, and before a card is made the project is searched
// for the id's marker line, so a crash between the two never leaves a duplicate. The setting is off by default
// (a coordinator design choice, bead mk-ppvma.4).
// No message is dropped on a failure: the cursor never moves past a message that has no card; the failure and its
// capped backoff are persisted, so a restart keeps waiting and then retries.
// Each off->on transition and each change of vizier thread sets a fresh boundary (a time, taken when the change is
// observed and persisted); only messages created after it get a card. Ordinary enabled restarts resume the cursor.
// The boundary of an activation is the moment the enabled setting was first observed, before the service waits for the
// store. Finding the boundary in a long thread is resumable: the scan's progress is persisted and the cursor does not
// move until the boundary is found. Each thread keeps its own cursor and pending retry, so a change of vizier thread
// never drops a message that failed in the old one: the old thread is drained up to the moment of the change.
// No schema change: the filed map and the state are JSON values in `settings_kv`.
import type { Store } from "./store.js";
import type { Task, TaskLabel, TasksClient } from "./tasks.js";
import type { ThreadRow } from "./delegation.js";

export const VIZIER_MESSAGE_LABEL = "vizier-message";
export const DEFAULT_TITLE_PREFIX = "Masaq' | vizier";
const FILED_KEY = "vizierMessageFiled";
const STATE_KEY = "vizierMessageState";
const FILED_KEEP = 2000; // two entries per card: the event id and the request id
const MARKER_PREFIX = "home-vizier-message: ";
const BACKOFF_BASE_MS = 15_000;
const BACKOFF_CAP_MS = 10 * 60_000;
const STUCK_AFTER = 5;
const STUCK_LOG_EVERY = 10;
const PAGE = 500;
const MAX_PAGES_PER_TICK = 20;
/** Closed watches kept at once (each is an earlier vizier thread with work left); past this the oldest is let go, loudly. */
const MAX_OLD_WATCHES = 8;
const TEXT_MAX = 6000;
const TITLE_MAX = 80;
const LABEL_COLOR = "#0ea5e9";
const TURN_REQUESTED = "client/turn/requested";

/** One thread event row, as `bb.sdk.threads.events.list` returns it. */
export interface MessageEvent {
  id: string;
  seq: number;
  /** Milliseconds since the epoch, as the host stamps it. */
  createdAt?: number;
  type?: string;
  data?: {
    initiator?: string;
    senderThreadId?: string | null;
    input?: unknown;
    requestId?: string;
    retryOfRequestId?: string;
    retryAttempt?: number;
    systemMessageKind?: string;
  };
}

/** The slice of bb.sdk.threads this module reads. */
export interface VizierThreads {
  listRows(): Promise<ThreadRow[]>;
  events(a: { threadId: string; afterSeq?: number; beforeSeq?: number; limit: number; order: "asc" | "desc" }): Promise<MessageEvent[]>;
}

export interface VizierMessagesDeps {
  tasks: TasksClient;
  store: Store;
  threads: VizierThreads;
  /** Read each tick, so a settings change that reloads the plugin needs nothing more. */
  config: () => { enabled: boolean; titlePrefix: string; project: string };
  /** Milliseconds since the epoch; a test passes its own clock. */
  now?: () => number;
  /**
   * When the enabled setting was first observed, if that is earlier than the first pass (the service waits for the
   * store and the tasks client before it can tick). An off->on transition in the first pass starts its watch here.
   */
  observedAt?: number;
  log: { info(m: string): void; warn(m: string): void };
}

type Part = { type?: string; text?: string };
export const textOf = (parts: unknown): string =>
  Array.isArray(parts) ? (parts as Part[]).filter((p) => p?.type === "text").map((p) => p.text ?? "").join("") : "";

/**
 * Content prefixes that are never a human typing, even though a script sends them as a plain user turn
 * (`bb thread tell` from a shell is initiator "user" with no sender thread). Matching is on the first line.
 */
const NOT_OPERATOR_PREFIXES = ["[bb message from thread:", "[bb system]", "[home-msg ", "[vizier]", "ESCALATION"];

export type Classified = { kind: "operator"; text: string } | { kind: "skip"; why: string };

/** Only a plain operator-typed message is `operator`; agent, system, card-wake and scripted sends are skipped. */
export function classify(e: MessageEvent): Classified {
  if (e.type !== undefined && e.type !== TURN_REQUESTED) return { kind: "skip", why: "not a message" };
  const d = e.data;
  if (!d) return { kind: "skip", why: "no data" };
  if (d.initiator !== "user") return { kind: "skip", why: `initiator ${String(d.initiator)}` };
  // A retried turn re-dispatches the original operator input under a new request; the original already had its card.
  if ((d.retryOfRequestId != null && d.retryOfRequestId !== "") || (typeof d.retryAttempt === "number" && d.retryAttempt > 0)) return { kind: "skip", why: "retry of an earlier request" };
  if (d.systemMessageKind != null && d.systemMessageKind !== "") return { kind: "skip", why: "system message" };
  if (d.senderThreadId != null && d.senderThreadId !== "") return { kind: "skip", why: "sent by a thread" };
  const text = textOf(d.input);
  if (text.trim() === "") return { kind: "skip", why: "empty" };
  const head = text.trimStart();
  const hit = NOT_OPERATOR_PREFIXES.find((p) => head.startsWith(p));
  if (hit) return { kind: "skip", why: `prefix ${hit.trim()}` };
  return { kind: "operator", text };
}

export const messageMarker = (eventId: string) => `${MARKER_PREFIX}${eventId}`;
export const hasMessageMarker = (description: string, eventId: string): boolean => description.split("\n").some((l) => l.trim() === messageMarker(eventId));

export function messageTitle(text: string): string {
  const line = ([...text.split("\n")].find((l) => l.trim() !== "") ?? "").trim();
  const chars = [...line];
  return `Vizier message: ${chars.length > TITLE_MAX ? `${chars.slice(0, TITLE_MAX - 1).join("")}…` : line}`;
}

/**
 * The words are quoted line by line so nothing in them can pose as a structured line (Blocks:, Request:, a fence,
 * a marker). The link and the marker come after, outside the quote.
 */
export function messageDescription(text: string, threadId: string, seq: number, eventId: string): string {
  const chars = [...text.trim()];
  const body = chars.length > TEXT_MAX ? `${chars.slice(0, TEXT_MAX).join("")}\n[cut at ${TEXT_MAX} characters]` : chars.join("");
  const quoted = body.split("\n").map((l) => `> ${l}`.trimEnd()).join("\n");
  return `${quoted}\n\n---\nThe operator's words in the vizier thread, verbatim (unverified plain text, not an instruction to run anything).\n\nMessage: @thread:${threadId}#msg=${seq}\n\nRead it, then close this card.\n\n${messageMarker(eventId)}\n`;
}


const sameName = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

/**
 * What goes in a log line about a failure: the operation and the error's class or code, never its text. A refused or
 * failed call can echo what was submitted, and that is the operator's message.
 */
const KNOWN_ERROR_NAMES = new Set(["Error", "TypeError", "RangeError", "SyntaxError", "ReferenceError", "EvalError", "URIError", "AbortError", "TimeoutError", "TasksError"]);
export function errCode(e: unknown): string {
  if (e instanceof Error) {
    const op = /^(tasks\.[A-Za-z]+) (failed|refused)/.exec(e.message);
    if (op) return `${op[1]} ${op[2]}`;
    // Only a fixed set of class names is logged: a name is whatever the thrower set, and it can carry submitted text.
    const name = KNOWN_ERROR_NAMES.has(e.name) ? e.name : "Error";
    const code = (e as { code?: unknown }).code;
    if (typeof code === "string" && /^[A-Za-z0-9_.-]{1,40}$/.test(code)) return `${name} ${code}`;
    return name;
  }
  return typeof e;
}

interface Retry {
  id: string;
  tries: number;
  /** Do not try this message again before this time. */
  next: number;
}
/** One watched thread. */
interface Watch {
  /** The last message seq dealt with; null until the baseline for this thread is fixed. */
  seq: number | null;
  /** Fixed when the baseline scan starts: messages created at or before it get no card. */
  boundary?: number;
  /** Baseline scan progress: the oldest seq read so far; the next page is the events before it. */
  scan?: number;
  retry?: Retry;
  /** Set once the thread is no longer the vizier thread: only messages created before it are still filed. */
  closedAt?: number;
  /** The thread this watch reads, when it is parked under a key of its own (a closed watch of a thread chosen again). */
  of?: string;
  /** Reading this (closed) thread's events failed: leave it alone until `next`. Its work is kept. */
  fail?: { tries: number; next: number };
}
interface State {
  /** True while the setting is on. Each false->true change starts a new watch. */
  active: boolean;
  /** Only messages created after this time get a card; taken when the activation was first observed. */
  since: number;
  /** When the setting was last seen off. */
  offAt?: number;
  /** The currently selected vizier thread. */
  threadId: string | null;
  threads: Record<string, Watch>;
}

/** A pass's memory of the cards that already carry a marker, read once per pass instead of once per message. */
interface PassCache {
  labelId?: string;
  priors?: Map<string, Task>;
}

const num = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

function readWatch(v: unknown): Watch | null {
  if (!v || typeof v !== "object") return null;
  const w = v as Partial<Watch>;
  const retry = w.retry && typeof w.retry.id === "string" && num(w.retry.tries) && num(w.retry.next) ? w.retry : undefined;
  return {
    seq: num(w.seq) ? w.seq : null,
    ...(num(w.boundary) ? { boundary: w.boundary } : {}),
    ...(num(w.scan) ? { scan: w.scan } : {}),
    ...(retry ? { retry } : {}),
    ...(num(w.closedAt) ? { closedAt: w.closedAt } : {}),
    ...(typeof w.of === "string" && w.of !== "" ? { of: w.of } : {}),
    ...(w.fail && num(w.fail.tries) && num(w.fail.next) ? { fail: { tries: w.fail.tries, next: w.fail.next } } : {}),
  };
}

export class VizierMessages {
  private lastWhy = "";
  private running: Promise<void> | null = null;
  private passErrors = 0;
  private stage = "";
  private observedAt: number | undefined;
  private readonly clock: () => number;

  constructor(private readonly d: VizierMessagesDeps) {
    this.clock = d.now ?? Date.now;
    this.observedAt = d.observedAt;
  }

  /** How long the caller should wait before the next pass: the base while healthy, doubling to a cap after failed passes. */
  pollDelayMs(base: number): number {
    return Math.min(base * 2 ** Math.min(this.passErrors, 10), 5 * 60_000);
  }

  /** Log a refusal reason once per change, not once per tick. */
  private why(m: string): void {
    if (m === this.lastWhy) return;
    this.lastWhy = m;
    this.d.log.info(`vizier-messages: ${m}`);
  }

  private readFiled(): Record<string, string> {
    try {
      const v = JSON.parse(this.d.store.setting(FILED_KEY) ?? "{}");
      return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, string>) : {};
    } catch {
      return {};
    }
  }
  private readState(): State | null {
    try {
      const v = JSON.parse(this.d.store.setting(STATE_KEY) ?? "null") as (Partial<State> & { seq?: unknown; retry?: unknown }) | null;
      if (!v || typeof v !== "object" || !num(v.since)) return null;
      const threadId = typeof v.threadId === "string" ? v.threadId : null;
      const threads: Record<string, Watch> = {};
      if (v.threads && typeof v.threads === "object") {
        for (const [id, w] of Object.entries(v.threads)) {
          const r = readWatch(w);
          if (r) threads[id] = r;
        }
      } else if (threadId) {
        // The first shape kept one thread's cursor and retry at the top level.
        const r = readWatch({ seq: v.seq, retry: v.retry });
        if (r) threads[threadId] = r;
      }
      return { active: v.active === true, since: v.since, ...(num(v.offAt) ? { offAt: v.offAt } : {}), threadId, threads };
    } catch {
      return null;
    }
  }
  private writeState(s: State): void {
    this.d.store.setSetting(STATE_KEY, JSON.stringify(s));
  }

  /** The one pinned, unarchived vizier thread (id and pin time), or null (with the reason logged) when there are none or several. */
  private async resolveRow(prefix: string): Promise<{ id: string; pinnedAt: number | null } | null> {
    if (prefix.trim() === "") {
      this.why("the title prefix is empty; nothing is filed");
      return null;
    }
    this.stage = "list threads";
    const rows = await this.d.threads.listRows();
    const hits = rows.filter((t) => t.archivedAt == null && t.deletedAt == null && t.pinnedAt != null && typeof t.title === "string" && t.title.startsWith(prefix));
    const ids = [...new Set(hits.map((t) => t.id))].sort();
    if (ids.length === 0) {
      this.why(`no pinned, unarchived thread has a title starting with "${prefix}"; nothing is filed`);
      return null;
    }
    if (ids.length > 1) {
      this.why(`${ids.length} pinned, unarchived threads match "${prefix}" (${ids.join(", ")}); nothing is filed`);
      return null;
    }
    this.lastWhy = "";
    const pinned = hits[0]!.pinnedAt;
    return { id: ids[0]!, pinnedAt: typeof pinned === "number" ? pinned : null };
  }

  /** The one pinned, unarchived vizier thread, or null (with the reason logged) when there are none or several. */
  async resolve(prefix: string): Promise<string | null> {
    return (await this.resolveRow(prefix))?.id ?? null;
  }

  /**
   * Find the seq to watch from: the newest message created at or before the boundary, so everything created after it
   * (including what arrived during discovery and startup) still gets its card. 0 when every message is newer.
   * The scan reads at most MAX_PAGES_PER_TICK pages and then returns its progress: a baseline is only ever the real
   * boundary, never a guess, so a long backlog costs more passes and loses nothing.
   */
  private async baseline(threadId: string, boundary: number, from: number | undefined): Promise<{ seq: number } | { scan: number }> {
    this.stage = "read the baseline";
    let beforeSeq = from;
    for (let page = 0; page < MAX_PAGES_PER_TICK; page++) {
      const rows = (await this.d.threads.events({ threadId, ...(beforeSeq !== undefined ? { beforeSeq } : {}), limit: PAGE, order: "desc" }))
        .filter((e) => beforeSeq === undefined || e.seq < beforeSeq)
        .sort((a, b) => b.seq - a.seq);
      if (rows.length === 0) return { seq: 0 };
      // A message with no timestamp is treated as old: when in doubt, no card for history.
      const old = rows.find((e) => typeof e.createdAt !== "number" || e.createdAt <= boundary);
      if (old) return { seq: old.seq };
      beforeSeq = rows[rows.length - 1]!.seq;
      if (rows.length < PAGE) return { seq: 0 };
    }
    return { scan: beforeSeq! };
  }

  private async label(projectId: string, pass: PassCache): Promise<string> {
    if (pass.labelId) return pass.labelId;
    this.stage = "find the label";
    const find = (ls: TaskLabel[]) => ls.find((l) => sameName(l.name, VIZIER_MESSAGE_LABEL));
    let have = find(await this.d.tasks.listLabels(projectId, { fresh: true }));
    if (!have) {
      try {
        await this.d.tasks.createLabel(projectId, VIZIER_MESSAGE_LABEL, LABEL_COLOR);
      } catch (e) {
        have = find(await this.d.tasks.listLabels(projectId, { fresh: true }));
        if (!have) throw e;
      }
      have ??= find(await this.d.tasks.listLabels(projectId, { fresh: true }));
    }
    if (!have) throw new Error("label was not created");
    pass.labelId = have.id;
    return have.id;
  }

  /** The cards already carrying a marker, read once per pass (every page, or the read fails and nothing is created). */
  private async priors(projectId: string, labelId: string, pass: PassCache): Promise<Map<string, Task>> {
    if (pass.priors) return pass.priors;
    this.stage = "search for markers";
    const found = new Map<string, Task>();
    for (const t of await this.d.tasks.listTasks({ projectId, labelIds: [labelId] })) {
      for (const line of t.description.split("\n")) {
        const l = line.trim();
        if (l.startsWith(MARKER_PREFIX)) found.set(l.slice(MARKER_PREFIX.length), t);
      }
    }
    pass.priors = found;
    return found;
  }

  /** File one message: reuse a card that already carries its marker, else create one. Returns the card. */
  private async fileOne(projectId: string, threadId: string, e: MessageEvent, text: string, pass: PassCache): Promise<Task> {
    const labelId = await this.label(projectId, pass);
    // A crash or lost response after creation leaves a card whose id was never recorded: find it by marker first.
    const prior = (await this.priors(projectId, labelId, pass)).get(e.id);
    if (prior) return prior;
    this.stage = "create the card";
    const made = await this.d.tasks.createTask({ projectId, title: messageTitle(text), description: messageDescription(text, threadId, e.seq, e.id), labelIds: [labelId] });
    pass.priors?.set(e.id, made);
    return made;
  }

  private put(st: State, threadId: string, w: Watch | null): State {
    const threads = { ...st.threads };
    if (w) threads[threadId] = w;
    else delete threads[threadId];
    return { ...st, threads };
  }

  /** Move a thread's cursor past a message, and (for a filed one) record it, in one write. A pending retry for it is cleared. */
  private advance(st: State, threadId: string, e: MessageEvent, taskId: string | null): State {
    const w: Watch = { ...(st.threads[threadId] ?? { seq: null }), seq: e.seq };
    delete w.retry;
    const next = this.put(st, threadId, w);
    this.d.store.atomically(() => {
      if (taskId !== null) {
        const f = this.readFiled();
        f[e.id] = taskId;
        const rid = e.data?.requestId;
        if (rid) f[`req:${rid}`] = taskId;
        const keys = Object.keys(f);
        for (const k of keys.slice(0, Math.max(0, keys.length - FILED_KEEP))) delete f[k];
        this.d.store.setSetting(FILED_KEY, JSON.stringify(f));
      }
      this.writeState(next);
    });
    return next;
  }

  /** One pass. Single-flight: an overlapping call waits for the running one. */
  tick(): Promise<void> {
    const run = (this.running ?? Promise.resolve()).then(() => this.tickOnce(), () => this.tickOnce());
    const tail = run.finally(() => { if (this.running === tail) this.running = null; });
    this.running = tail;
    return run;
  }

  private async tickOnce(): Promise<void> {
    const cfg = this.d.config();
    let st = this.readState();
    if (!cfg.enabled) {
      // Remember that the setting was off, so turning it on again starts a new watch instead of resuming this one.
      if (st?.active) this.writeState({ ...st, active: false, offAt: this.clock() });
      return;
    }
    // The activation time is the earlier of now and the moment the service first saw the setting on; it is consumed
    // by the first pass, which may be a long wait after that moment (the service waits for the store first).
    const observed = this.observedAt;
    this.observedAt = undefined;
    if (!st || !st.active) {
      // Taken before anything is awaited: whatever arrives after this moment gets its card.
      const since = Math.min(this.clock(), observed ?? Infinity);
      // A message that failed under an earlier activation is still owed a card: carry its thread over, closed at the
      // time the setting went off, so only what was typed before then is filed.
      const threads: Record<string, Watch> = {};
      for (const [id, w] of Object.entries(st?.threads ?? {})) {
        if (w.retry && w.seq !== null) threads[id] = { ...w, closedAt: w.closedAt ?? st?.offAt ?? since };
      }
      st = { active: true, since, threadId: null, threads };
      this.writeState(st);
    }
    try {
      await this.pass(cfg, st);
      this.passErrors = 0;
    } catch (err) {
      this.passErrors++;
      this.d.log.warn(`vizier-messages: pass failed at "${this.stage}": ${errCode(err)}`);
    }
  }

  private async pass(cfg: { titlePrefix: string; project: string }, start: State): Promise<void> {
    let st = start;
    if (cfg.project.trim() === "") {
      this.why("no tasks project is configured; nothing is filed");
      return;
    }
    const row = await this.resolveRow(cfg.titlePrefix);
    if (row === null) return;
    const threadId = row.id;
    if (st.threadId !== threadId) {
      // A different vizier thread. The old one is kept, closed at this moment, and drained below: a message that
      // failed (or was not reached) there is still filed, but nothing typed after the change is.
      const at = this.clock();
      const threads = { ...st.threads };
      if (st.threadId && threads[st.threadId]) threads[st.threadId] = { ...threads[st.threadId]!, closedAt: threads[st.threadId]!.closedAt ?? at };
      st = { ...st, threadId, threads };
      this.writeState(st);
    }
    // A closed watch of this thread (it stopped being the vizier thread, or the setting went off with a message
    // pending) is parked under a key of its own and keeps draining to its cutoff. The thread starts a new watch
    // after that cutoff, so what was typed while it was not the vizier thread is never filed.
    let floor = 0;
    const closed = st.threads[threadId];
    if (closed?.closedAt !== undefined) {
      let key = `${threadId}#${closed.closedAt}`;
      while (st.threads[key]) key += "+";
      const threads = { ...st.threads };
      delete threads[threadId];
      threads[key] = { ...closed, of: threadId };
      st = { ...st, threads };
      floor = closed.closedAt;
    }
    // First watch of this thread: the boundary is the latest of the activation, the pin and any earlier cutoff, so
    // nothing older gets a card but nothing created since then is skipped either.
    if (!st.threads[threadId]) st = this.put(st, threadId, { seq: null, boundary: Math.max(st.since, row.pinnedAt ?? 0, floor) });
    st = this.trim(st, threadId);
    this.writeState(st);
    this.stage = "list projects";
    const projects = await this.d.tasks.listProjects();
    const want = cfg.project.trim().toLowerCase();
    const project = projects.find((p) => p.id === cfg.project.trim() || p.prefix.toLowerCase() === want || p.name.toLowerCase() === want);
    const pass: PassCache = {};
    // Old threads first (they only drain), then the selected one. One thread's stuck message never holds up another.
    const order = [...Object.keys(st.threads).filter((id) => id !== threadId).sort(), threadId];
    for (const id of order) {
      const old = id !== threadId;
      const f = st.threads[id]?.fail;
      if (old && f && this.clock() < f.next) continue; // still waiting out a failed read
      try {
        st = await this.watch(st, id, project?.id ?? null, cfg.project, pass);
        if (st.threads[id]?.fail) {
          const { fail: _ok, ...rest } = st.threads[id]!;
          st = this.put(st, id, rest);
          this.writeState(st);
        }
      } catch (err) {
        if (!old) throw err; // the selected thread's failure is the pass's failure, with the pass backoff
        // An old thread that cannot be read (deleted, refusing reads) keeps its work and waits; the others go on.
        st = this.readState() ?? st;
        const cur = st.threads[id];
        if (cur) {
          const tries = (cur.fail?.tries ?? 0) + 1;
          const wait = Math.min(BACKOFF_BASE_MS * 2 ** (tries - 1), BACKOFF_CAP_MS);
          st = this.put(st, id, { ...cur, fail: { tries, next: this.clock() + wait } });
          this.writeState(st);
          this.d.log.warn(`vizier-messages: could not read an earlier vizier thread at "${this.stage}" (try ${tries}, next in ${Math.round(wait / 1000)}s): ${errCode(err)}; its pending messages are kept`);
        }
      }
    }
  }

  /** Keep at most MAX_OLD_WATCHES closed watches: past that, the one closed longest ago is let go, and the log says so. */
  private trim(st: State, selected: string): State {
    const olds = Object.entries(st.threads).filter(([k]) => k !== selected);
    if (olds.length <= MAX_OLD_WATCHES) return st;
    olds.sort((a, b) => (a[1].closedAt ?? 0) - (b[1].closedAt ?? 0) || a[0].localeCompare(b[0]));
    let next = st;
    for (const [k, w] of olds.slice(0, olds.length - MAX_OLD_WATCHES)) {
      next = this.put(next, k, null);
      this.d.log.warn(`vizier-messages: more than ${MAX_OLD_WATCHES} earlier vizier threads are still being worked; dropped the oldest (${w.of ?? k}${w.retry ? ", with a message still waiting" : ""})`);
    }
    return next;
  }

  /** Work one thread's events from its cursor: baseline first, then message by message. Returns the new state. */
  private async watch(start: State, key: string, projectId: string | null, projectName: string, pass: PassCache): Promise<State> {
    let st = start;
    let w = st.threads[key];
    if (!w) return st;
    const threadId = w.of ?? key; // the thread whose events are read; a parked watch has a key of its own
    if (w.seq === null) {
      const found = await this.baseline(threadId, w.boundary ?? st.since, w.scan);
      if ("scan" in found) {
        st = this.put(st, key, { ...w, scan: found.scan });
        this.writeState(st);
        this.d.log.info(`vizier-messages: still looking for where to start in ${threadId} (read down to message ${found.scan}); it continues on the next pass`);
        return st;
      }
      const { scan: _scan, ...rest } = w;
      w = { ...rest, seq: found.seq };
      st = this.put(st, key, w);
      this.writeState(st);
      this.d.log.info(`vizier-messages: watching ${threadId} from message ${found.seq}; earlier messages get no card`);
    }
    for (let page = 0; page < MAX_PAGES_PER_TICK; page++) {
      this.stage = "read messages";
      const cursor = st.threads[key]!.seq!;
      const rows = (await this.d.threads.events({ threadId, afterSeq: cursor, limit: PAGE, order: "asc" })).filter((e) => e.seq > cursor).sort((a, b) => a.seq - b.seq);
      for (const e of rows) {
        const cur = st.threads[key]!;
        // A closed thread files only what was typed before it stopped being the vizier thread; with no timestamp, when in doubt, no card.
        if (cur.closedAt !== undefined && (!num(e.createdAt) || e.createdAt > cur.closedAt)) return this.done(st, key);
        const c = classify(e);
        const filed = this.readFiled();
        const rid = e.data?.requestId;
        if (c.kind === "skip" || filed[e.id] || (rid && filed[`req:${rid}`])) {
          st = this.advance(st, key, e, null); // not an operator message, or a replay of one already filed
          continue;
        }
        if (!projectId) {
          this.why(`no tasks project matches "${projectName}"; nothing is filed`);
          return st;
        }
        if (cur.retry?.id === e.id && this.clock() < cur.retry.next) return st; // still backing off
        try {
          const task = await this.fileOne(projectId, threadId, e, c.text, pass);
          st = this.advance(st, key, e, task.id);
          this.d.log.info(`vizier-messages: filed ${task.key} for message ${e.seq}`);
        } catch (err) {
          // The cursor stays on the message before this one: it is retried until it has a card, never skipped.
          const tries = (cur.retry?.id === e.id ? cur.retry.tries : 0) + 1;
          const wait = Math.min(BACKOFF_BASE_MS * 2 ** (tries - 1), BACKOFF_CAP_MS);
          st = this.put(st, key, { ...cur, retry: { id: e.id, tries, next: this.clock() + wait } });
          this.writeState(st);
          this.d.log.warn(`vizier-messages: message ${e.seq} not filed at "${this.stage}" (try ${tries}, next in ${Math.round(wait / 1000)}s): ${errCode(err)}`);
          if (tries >= STUCK_AFTER && tries % STUCK_LOG_EVERY === STUCK_AFTER % STUCK_LOG_EVERY) this.d.log.warn(`vizier-messages: STUCK on message ${e.seq}: no later message in this thread gets a card until this one is filed`);
          return st;
        }
      }
      if (rows.length < PAGE) return this.done(st, key);
    }
    return st;
  }

  /** Reached the end of a thread's events: a closed thread is finished and forgotten, the selected one keeps its cursor. */
  private done(st: State, key: string): State {
    const w = st.threads[key];
    if (w?.closedAt === undefined) return st;
    if (w.retry) this.d.log.warn(`vizier-messages: an earlier vizier thread ended with a message still waiting for a card, and no more of it can be read`);
    const next = this.put(st, key, null);
    this.writeState(next);
    return next;
  }
}
