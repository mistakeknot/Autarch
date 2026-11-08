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
export function errCode(e: unknown): string {
  if (e instanceof Error) {
    const op = /^(tasks\.[A-Za-z]+) (failed|refused)/.exec(e.message);
    if (op) return `${op[1]} ${op[2]}`;
    const code = (e as { code?: unknown }).code;
    if (typeof code === "string" && /^[A-Za-z0-9_.-]{1,40}$/.test(code)) return `${e.name} ${code}`;
    return e.name;
  }
  return typeof e;
}

interface Retry {
  id: string;
  tries: number;
  /** Do not try this message again before this time. */
  next: number;
}
interface State {
  /** True while the setting is on. Each false->true change starts a new watch. */
  active: boolean;
  /** Only messages created after this time get a card; taken when the activation was first observed. */
  since: number;
  threadId: string | null;
  /** The last message seq dealt with in `threadId`; null until the baseline for that thread is fixed. */
  seq: number | null;
  retry?: Retry;
}

/** A pass's memory of the cards that already carry a marker, read once per pass instead of once per message. */
interface PassCache {
  labelId?: string;
  priors?: Map<string, Task>;
}

export class VizierMessages {
  private lastWhy = "";
  private running: Promise<void> | null = null;
  private passErrors = 0;
  private stage = "";
  private readonly clock: () => number;

  constructor(private readonly d: VizierMessagesDeps) {
    this.clock = d.now ?? Date.now;
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
      const v = JSON.parse(this.d.store.setting(STATE_KEY) ?? "null") as Partial<State> | null;
      if (!v || typeof v !== "object" || typeof v.since !== "number") return null;
      const retry = v.retry && typeof v.retry.id === "string" && typeof v.retry.tries === "number" && typeof v.retry.next === "number" ? v.retry : undefined;
      return { active: v.active === true, since: v.since, threadId: typeof v.threadId === "string" ? v.threadId : null, seq: typeof v.seq === "number" ? v.seq : null, ...(retry ? { retry } : {}) };
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
   * The seq to watch from: the newest message created at or before the boundary, so everything created after it
   * (including what arrived during discovery and startup) still gets its card. 0 when every message is newer.
   */
  private async baseline(threadId: string, boundary: number): Promise<number> {
    this.stage = "read the baseline";
    let beforeSeq: number | undefined;
    let newest = 0;
    for (let page = 0; page < MAX_PAGES_PER_TICK; page++) {
      const rows = (await this.d.threads.events({ threadId, ...(beforeSeq !== undefined ? { beforeSeq } : {}), limit: PAGE, order: "desc" }))
        .filter((e) => beforeSeq === undefined || e.seq < beforeSeq)
        .sort((a, b) => b.seq - a.seq);
      if (rows.length === 0) return 0;
      if (page === 0) newest = rows[0]!.seq;
      // A message with no timestamp is treated as old: when in doubt, no card for history.
      const old = rows.find((e) => typeof e.createdAt !== "number" || e.createdAt <= boundary);
      if (old) return old.seq;
      beforeSeq = rows[rows.length - 1]!.seq;
      if (rows.length < PAGE) return 0;
    }
    this.d.log.warn(`vizier-messages: more than ${MAX_PAGES_PER_TICK * PAGE} messages are newer than the boundary; watching from the newest`);
    return newest;
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

  /** Move the cursor past a message, and (for a filed one) record it, in one write. A pending retry for it is cleared. */
  private advance(st: State, e: MessageEvent, taskId: string | null): State {
    const next: State = { ...st, seq: e.seq };
    delete next.retry;
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
      if (st?.active) this.writeState({ ...st, active: false });
      return;
    }
    if (!st || !st.active) {
      // Taken before anything is awaited: whatever arrives after this moment gets its card.
      st = { active: true, since: this.clock(), threadId: null, seq: null };
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
    if (st.threadId !== threadId || st.seq === null) {
      // First watch, or a different vizier thread: the boundary is the later of the activation and the pin, so
      // nothing older gets a card but nothing created since then is skipped either.
      const seq = await this.baseline(threadId, Math.max(st.since, row.pinnedAt ?? 0));
      st = { active: true, since: st.since, threadId, seq };
      this.writeState(st);
      this.d.log.info(`vizier-messages: watching ${threadId} from message ${seq}; earlier messages get no card`);
    }
    this.stage = "list projects";
    const projects = await this.d.tasks.listProjects();
    const want = cfg.project.trim().toLowerCase();
    const project = projects.find((p) => p.id === cfg.project.trim() || p.prefix.toLowerCase() === want || p.name.toLowerCase() === want);
    const pass: PassCache = {};
    for (let page = 0; page < MAX_PAGES_PER_TICK; page++) {
      this.stage = "read messages";
      const cursor = st.seq!;
      const rows = (await this.d.threads.events({ threadId, afterSeq: cursor, limit: PAGE, order: "asc" })).filter((e) => e.seq > cursor).sort((a, b) => a.seq - b.seq);
      if (rows.length === 0) return;
      for (const e of rows) {
        const c = classify(e);
        const filed = this.readFiled();
        const rid = e.data?.requestId;
        if (c.kind === "skip" || filed[e.id] || (rid && filed[`req:${rid}`])) {
          st = this.advance(st, e, null); // not an operator message, or a replay of one already filed
          continue;
        }
        if (!project) {
          this.why(`no tasks project matches "${cfg.project}"; nothing is filed`);
          return;
        }
        if (st.retry?.id === e.id && this.clock() < st.retry.next) return; // still backing off
        try {
          const task = await this.fileOne(project.id, threadId, e, c.text, pass);
          st = this.advance(st, e, task.id);
          this.d.log.info(`vizier-messages: filed ${task.key} for message ${e.seq}`);
        } catch (err) {
          // The cursor stays on the message before this one: it is retried until it has a card, never skipped.
          const tries = (st.retry?.id === e.id ? st.retry.tries : 0) + 1;
          const wait = Math.min(BACKOFF_BASE_MS * 2 ** (tries - 1), BACKOFF_CAP_MS);
          st = { ...st, retry: { id: e.id, tries, next: this.clock() + wait } };
          this.writeState(st);
          this.d.log.warn(`vizier-messages: message ${e.seq} not filed at "${this.stage}" (try ${tries}, next in ${Math.round(wait / 1000)}s): ${errCode(err)}`);
          if (tries >= STUCK_AFTER && tries % STUCK_LOG_EVERY === STUCK_AFTER % STUCK_LOG_EVERY) this.d.log.warn(`vizier-messages: STUCK on message ${e.seq}: no later message gets a card until this one is filed`);
          return;
        }
      }
      if (rows.length < PAGE) return;
    }
  }
}
