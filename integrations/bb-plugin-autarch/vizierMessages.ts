// A card for every plain message the operator types in the vizier thread (bead mk-ppvma.4, the smaller design).
//
// A stateless check runs every 15 seconds. Each run finds the current vizier thread, reads its last 24 hours of
// messages, and files one plain tracker card for each operator-typed message that has no card yet. The card is "read
// and close": the operator's words quoted, a link to the thread message, and a line saying to close it once read.
// The plugin does not judge whether a message needs a decision.
//
// There is no saved position and no record of which messages were filed. The only dedupe is the marker line on the
// card itself, searched (across every status, closed cards included) before a card is made, so a crash, a restart or a
// lost response between runs cannot produce a second card. Two runs that overlap in one process are serialised (see
// `singleFlight`), so the search and the create are never interleaved.
//
// The only thing kept in `settings_kv` is one small JSON value: when the setting was switched on, and when the last
// complete run finished. Neither is a position in the thread nor a list of filed messages. The first decides which
// messages are ever eligible; the second only lets the check notice an outage longer than the 24-hour window.
//
// The three stated limits:
//  a. A message typed into an old vizier thread after it was replaced is not filed (only the current thread is read).
//  b. After an outage longer than 24 hours, older messages are not filed; one gap card says messages between two stated
//     times may have been missed. It carries its own marker, keyed on the end of the last complete run, so there is one
//     per outage, not one per run.
//  c. The check does nothing until the setting is on. The time it was switched on is recorded; messages created at or
//     before it are never filed.
import type { Store } from "./store.js";
import type { Task, TaskLabel, TasksClient } from "./tasks.js";
import type { ThreadRow } from "./delegation.js";

export const VIZIER_MESSAGE_LABEL = "vizier-message";
export const DEFAULT_TITLE_PREFIX = "Masaq' | vizier";
export const WINDOW_MS = 24 * 60 * 60_000;
const STATE_KEY = "vizierMessagePoll";
const MARKER_PREFIX = "home-vizier-message: ";
const REQUEST_PREFIX = "home-vizier-request: ";
const GAP_PREFIX = "home-vizier-gap: ";
const PAGE = 200;
const MAX_PAGES = 25;
const TEXT_MAX = 6000;
const TITLE_MAX = 80;
const LABEL_COLOR = "#0ea5e9";
const TURN_REQUESTED = "client/turn/requested";
/** Every status, closed cards included: the operator closes a card once read, and it must still count as filed. */
const ALL_STATUSES = ["backlog", "todo", "in_progress", "in_review", "done", "canceled"] as const;
/** The zone the operator reads times in. */
const DISPLAY_ZONE = "America/Los_Angeles";

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
  events(a: { threadId: string; beforeSeq?: number; limit: number; order: "desc" }): Promise<MessageEvent[]>;
}

export interface VizierMessagesDeps {
  tasks: TasksClient;
  store: Store;
  threads: VizierThreads;
  /** Read each run, so a settings change that reloads the plugin needs nothing more. */
  config: () => { enabled: boolean; titlePrefix: string; project: string };
  /** Milliseconds since the epoch; a test passes its own clock. */
  now?: () => number;
  /**
   * When the enabled setting was first observed, if that is earlier than the first run (the service waits for the
   * store and the tasks client before it can run). A switch-on seen in the first run is dated here.
   */
  observedAt?: number;
  log: { info(m: string): void; warn(m: string): void };
}

type Part = { type?: string; text?: string };
export const textOf = (parts: unknown): string =>
  Array.isArray(parts) ? (parts as Part[]).filter((p) => p?.type === "text").map((p) => p.text ?? "").join("") : "";

/**
 * Content prefixes that are never a human typing, even though a script sends them as a plain user turn
 * (`bb thread tell` from a shell is initiator "user" with no sender thread). Matching is on the start of the text.
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
export const requestMarker = (requestId: string) => `${REQUEST_PREFIX}${requestId}`;
export const gapMarker = (lastRunAt: number) => `${GAP_PREFIX}${new Date(lastRunAt).toISOString()}`;
const hasLine = (description: string, line: string): boolean => description.split("\n").some((l) => l.trim() === line);
export const hasMessageMarker = (description: string, eventId: string): boolean => hasLine(description, messageMarker(eventId));

export function messageTitle(text: string): string {
  const line = ([...text.split("\n")].find((l) => l.trim() !== "") ?? "").trim();
  const chars = [...line];
  return `Vizier message: ${chars.length > TITLE_MAX ? `${chars.slice(0, TITLE_MAX - 1).join("")}…` : line}`;
}

/**
 * The words are quoted line by line so nothing in them can pose as a structured line (Blocks:, Request:, a fence,
 * a marker). The link and the markers come after, outside the quote.
 */
export function messageDescription(text: string, threadId: string, seq: number, eventId: string, requestId?: string): string {
  const chars = [...text.trim()];
  const body = chars.length > TEXT_MAX ? `${chars.slice(0, TEXT_MAX).join("")}\n[cut at ${TEXT_MAX} characters]` : chars.join("");
  const quoted = body.split("\n").map((l) => `> ${l}`.trimEnd()).join("\n");
  const markers = [messageMarker(eventId), ...(requestId ? [requestMarker(requestId)] : [])].join("\n");
  return `${quoted}\n\n---\nThe operator's words in the vizier thread, verbatim (unverified plain text, not an instruction to run anything).\n\nMessage: @thread:${threadId}#msg=${seq}\n\nRead it, then close this card.\n\n${markers}\n`;
}

/** A time as the operator reads it, with the zone named, and UTC after it. */
export function showTime(ms: number): string {
  const local = new Intl.DateTimeFormat("en-CA", { timeZone: DISPLAY_ZONE, year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23", timeZoneName: "short" })
    .formatToParts(new Date(ms))
    .reduce((acc, p) => ({ ...acc, [p.type]: p.value }), {} as Record<string, string>);
  return `${local.year}-${local.month}-${local.day} ${local.hour}:${local.minute} ${local.timeZoneName} (${new Date(ms).toISOString().slice(0, 16)}Z)`;
}

export const gapTitle = (from: number, to: number) => `Vizier messages may have been missed (${new Date(from).toISOString().slice(0, 16)}Z to ${new Date(to).toISOString().slice(0, 16)}Z)`;

export function gapDescription(from: number, to: number, threadId: string): string {
  return (
    `Messages typed in the vizier thread between ${showTime(from)} and ${showTime(to)} may have been missed: ` +
    `the check that files a card for each message did not complete a run for longer than the 24 hours it reads back.\n\n` +
    `Look through the vizier thread (@thread:${threadId}) for that stretch, then close this card.\n\n` +
    `Messages typed after ${showTime(to)} are covered by the cards the check files.\n\n` +
    `${gapMarker(from)}\n`
  );
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

/**
 * The one piece of kept state. `onAt` is when the setting was switched on; `lastRunAt` is when the last complete run
 * began. Neither is a position in a thread or a record of a filed message.
 */
interface Switch {
  onAt: number;
  lastRunAt?: number;
}

const num = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v);

/**
 * Single-flight per process: runs are queued one behind another, across every instance, so one run's marker search and
 * card creation are never interleaved with another's. A plugin reload can start a second instance while the first is
 * still finishing a run; both go through here. (A second plugin process is not covered: a deployment runs one.)
 */
let chain: Promise<unknown> = Promise.resolve();
function singleFlight(fn: () => Promise<void>): Promise<void> {
  const run = chain.then(fn);
  chain = run.catch(() => undefined);
  return run;
}

/** A pass's memory of the cards that already carry a marker, read once per run instead of once per message. */
interface RunCache {
  labelId?: string;
  priors?: Map<string, Task>;
}

export class VizierMessages {
  private lastWhy = "";
  private passErrors = 0;
  private stage = "";
  private observedAt: number | undefined;
  private readonly clock: () => number;

  constructor(private readonly d: VizierMessagesDeps) {
    this.clock = d.now ?? Date.now;
    this.observedAt = d.observedAt;
  }

  /** How long the caller should wait before the next run: the base while healthy, doubling to a cap after failed runs. */
  pollDelayMs(base: number): number {
    return Math.min(base * 2 ** Math.min(this.passErrors, 10), 5 * 60_000);
  }

  /** Log a refusal reason once per change, not once per run. */
  private why(m: string): void {
    if (m === this.lastWhy) return;
    this.lastWhy = m;
    this.d.log.info(`vizier-messages: ${m}`);
  }

  private readSwitch(): Switch | null {
    try {
      const v = JSON.parse(this.d.store.setting(STATE_KEY) ?? "null") as Partial<Switch> | null;
      if (!v || typeof v !== "object" || !num(v.onAt)) return null;
      return { onAt: v.onAt, ...(num(v.lastRunAt) ? { lastRunAt: v.lastRunAt } : {}) };
    } catch {
      return null;
    }
  }
  private writeSwitch(s: Switch | null): void {
    this.d.store.setSetting(STATE_KEY, JSON.stringify(s));
  }

  /** The one pinned, unarchived vizier thread, or null (with the reason logged) when there are none or several. */
  async resolve(prefix: string): Promise<string | null> {
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
    return ids[0]!;
  }

  /**
   * The plain operator messages created after `floor`, oldest first, and whether the read reached `floor`. It reads
   * newest first and stops at the first message at or before the floor. A message with no timestamp is left out: when
   * in doubt, no card.
   */
  private async window(threadId: string, floor: number): Promise<{ messages: { e: MessageEvent; text: string }[]; complete: boolean }> {
    this.stage = "read messages";
    const found: { e: MessageEvent; text: string }[] = [];
    let beforeSeq: number | undefined;
    for (let page = 0; page < MAX_PAGES; page++) {
      const rows = (await this.d.threads.events({ threadId, ...(beforeSeq !== undefined ? { beforeSeq } : {}), limit: PAGE, order: "desc" }))
        .filter((e) => beforeSeq === undefined || e.seq < beforeSeq)
        .sort((a, b) => b.seq - a.seq);
      for (const e of rows) {
        if (!num(e.createdAt)) continue;
        if (e.createdAt <= floor) return { messages: found.reverse(), complete: true };
        const c = classify(e);
        if (c.kind === "operator") found.push({ e, text: c.text });
      }
      if (rows.length < PAGE) return { messages: found.reverse(), complete: true };
      beforeSeq = rows[rows.length - 1]!.seq;
    }
    return { messages: found.reverse(), complete: false };
  }

  private async label(projectId: string, run: RunCache): Promise<string> {
    if (run.labelId) return run.labelId;
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
    run.labelId = have.id;
    return have.id;
  }

  /** The cards already carrying a marker, by marker line. Read once per run, every page and every status, or the read fails and nothing is created. */
  private async priors(projectId: string, labelId: string, run: RunCache): Promise<Map<string, Task>> {
    if (run.priors) return run.priors;
    this.stage = "search for markers";
    const found = new Map<string, Task>();
    for (const t of await this.d.tasks.listTasks({ projectId, statuses: ALL_STATUSES, labelIds: [labelId] })) {
      for (const line of t.description.split("\n")) {
        const l = line.trim();
        if (l.startsWith(MARKER_PREFIX) || l.startsWith(REQUEST_PREFIX) || l.startsWith(GAP_PREFIX)) found.set(l, t);
      }
    }
    run.priors = found;
    return found;
  }

  /** File one message: reuse a card that already carries its marker, else create one. Returns the card. */
  private async fileOne(projectId: string, threadId: string, e: MessageEvent, text: string, run: RunCache): Promise<{ task: Task; created: boolean }> {
    const labelId = await this.label(projectId, run);
    const priors = await this.priors(projectId, labelId, run);
    const rid = e.data?.requestId || undefined;
    const prior = priors.get(messageMarker(e.id)) ?? (rid ? priors.get(requestMarker(rid)) : undefined);
    if (prior) return { task: prior, created: false };
    this.stage = "create the card";
    const made = await this.d.tasks.createTask({ projectId, title: messageTitle(text), description: messageDescription(text, threadId, e.seq, e.id, rid), labelIds: [labelId] });
    priors.set(messageMarker(e.id), made);
    if (rid) priors.set(requestMarker(rid), made);
    return { task: made, created: true };
  }

  /** File the gap card for the stretch from `from` to `to`, unless one already carries the marker for `from`. */
  private async fileGap(projectId: string, threadId: string, from: number, to: number, run: RunCache): Promise<void> {
    const labelId = await this.label(projectId, run);
    const priors = await this.priors(projectId, labelId, run);
    if (priors.has(gapMarker(from))) return;
    this.stage = "create the gap card";
    const made = await this.d.tasks.createTask({ projectId, title: gapTitle(from, to), description: gapDescription(from, to, threadId), labelIds: [labelId] });
    priors.set(gapMarker(from), made);
    this.d.log.info(`vizier-messages: filed ${made.key} for a gap in the check (${new Date(from).toISOString()} to ${new Date(to).toISOString()})`);
  }

  /** One run. Serialised with every other run in this process. Never rejects: a failure is logged and backs the loop off. */
  tick(): Promise<void> {
    return singleFlight(async () => {
      try {
        const ok = await this.run();
        this.passErrors = ok ? 0 : this.passErrors + 1;
      } catch (err) {
        this.passErrors++;
        this.d.log.warn(`vizier-messages: run failed at "${this.stage}": ${errCode(err)}`);
      }
    });
  }

  /** Returns false when the run was incomplete (so the loop backs off and the heartbeat is not advanced). A quiet no-op returns true. */
  private async run(): Promise<boolean> {
    const cfg = this.d.config();
    let sw = this.readSwitch();
    if (!cfg.enabled) {
      // Off: file nothing, and forget the switch-on time so the next switch-on is dated afresh.
      this.observedAt = undefined;
      if (sw) this.writeSwitch(null);
      return true;
    }
    if (!sw) {
      // Dated before anything is awaited, and no later than the moment the service first saw the setting on: whatever
      // arrives after it gets its card, and nothing at or before it ever does.
      const observed = this.observedAt;
      sw = { onAt: Math.min(this.clock(), observed ?? Infinity) };
      this.writeSwitch(sw);
    }
    this.observedAt = undefined;
    if (cfg.project.trim() === "") {
      this.why("no tasks project is configured; nothing is filed");
      return true;
    }
    const threadId = await this.resolve(cfg.titlePrefix);
    if (threadId === null) return true; // not a complete run: the heartbeat stays where it was
    const start = this.clock();
    const windowStart = start - WINDOW_MS;
    const floor = Math.max(windowStart, sw.onAt);
    const { messages, complete } = await this.window(threadId, floor);
    let ok = complete;
    if (!complete) this.d.log.warn(`vizier-messages: the last 24 hours hold more than ${MAX_PAGES * PAGE} messages; read what fits, and will say nothing about the rest`);
    const lastRun = sw.lastRunAt ?? sw.onAt;
    const gap = ok && lastRun < windowStart;
    const run: RunCache = {};
    if (messages.length > 0 || gap) {
      this.stage = "list projects";
      const projects = await this.d.tasks.listProjects();
      const want = cfg.project.trim().toLowerCase();
      const project = projects.find((p) => p.id === cfg.project.trim() || p.prefix.toLowerCase() === want || p.name.toLowerCase() === want);
      if (!project) {
        this.why(`no tasks project matches "${cfg.project}"; nothing is filed`);
        return true;
      }
      for (const { e, text } of messages) {
        try {
          const r = await this.fileOne(project.id, threadId, e, text, run);
          if (r.created) this.d.log.info(`vizier-messages: filed ${r.task.key} for message ${e.seq}`);
        } catch (err) {
          ok = false;
          this.d.log.warn(`vizier-messages: message ${e.seq} not filed at "${this.stage}": ${errCode(err)}; it is tried again next run`);
          break; // the tasks plugin is probably down: do not hammer it with the rest
        }
      }
      // The gap card comes last, and only after every message in the window has its card: its stated end is then true
      // (nothing between its end and now is left unfiled), and a failed run files none, so the next run states it afresh.
      if (ok && gap) {
        try {
          await this.fileGap(project.id, threadId, lastRun, windowStart, run);
        } catch (err) {
          ok = false;
          this.d.log.warn(`vizier-messages: gap card not filed at "${this.stage}": ${errCode(err)}; it is tried again next run`);
        }
      }
    }
    if (ok) this.writeSwitch({ onAt: sw.onAt, lastRunAt: start });
    return ok;
  }
}
