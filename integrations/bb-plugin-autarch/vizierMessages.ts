// A card for every plain message the operator types in the vizier thread (bead mk-ppvma.4).
//
// The plugin polls the vizier thread's `client/turn/requested` events, keeps the ones an operator typed, and files
// one plain tracker card per message over the tasks plugin's RPC (never a spawned process). The card is "read and
// close": the operator's words quoted, a link to the thread message, and a line saying to close it once read.
// The plugin does not judge whether a message needs a decision.
//
// Idempotency: a message is identified by its event id. The id is written to `settings_kv` only after the card
// exists, and before a card is made the project is searched for the id's marker line, so a crash between the
// two never leaves a duplicate. The setting is off by default (a coordinator design choice, bead mk-ppvma.4).
// No schema change: the filed map and the cursor are JSON values in `settings_kv`.
import type { Store } from "./store.js";
import type { Task, TaskLabel, TasksClient } from "./tasks.js";
import type { ThreadRow } from "./delegation.js";

export const VIZIER_MESSAGE_LABEL = "vizier-message";
export const DEFAULT_TITLE_PREFIX = "Masaq' | vizier";
const FILED_KEY = "vizierMessageFiled";
const CURSOR_KEY = "vizierMessageCursor";
const FILED_KEEP = 1000;
const PAGE = 500;
const MAX_PAGES_PER_TICK = 20;
const GIVE_UP_AFTER = 10;
const TEXT_MAX = 6000;
const TITLE_MAX = 80;
const LABEL_COLOR = "#0ea5e9";
const TURN_REQUESTED = "client/turn/requested";

/** One thread event row, as `bb.sdk.threads.events.list` returns it. */
export interface MessageEvent {
  id: string;
  seq: number;
  type?: string;
  data?: { initiator?: string; senderThreadId?: string | null; input?: unknown };
}

/** The slice of bb.sdk.threads this module reads. */
export interface VizierThreads {
  listRows(): Promise<ThreadRow[]>;
  events(a: { threadId: string; afterSeq?: number; limit: number; order: "asc" | "desc" }): Promise<MessageEvent[]>;
}

export interface VizierMessagesDeps {
  tasks: TasksClient;
  store: Store;
  threads: VizierThreads;
  /** Read each tick, so a settings change that reloads the plugin needs nothing more. */
  config: () => { enabled: boolean; titlePrefix: string; project: string };
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
  if (d.senderThreadId != null && d.senderThreadId !== "") return { kind: "skip", why: "sent by a thread" };
  const text = textOf(d.input);
  if (text.trim() === "") return { kind: "skip", why: "empty" };
  const head = text.trimStart();
  const hit = NOT_OPERATOR_PREFIXES.find((p) => head.startsWith(p));
  if (hit) return { kind: "skip", why: `prefix ${hit.trim()}` };
  return { kind: "operator", text };
}

export const messageMarker = (eventId: string) => `home-vizier-message: ${eventId}`;
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
const errText = (e: unknown) => (e instanceof Error ? e.message : String(e));

export class VizierMessages {
  private readonly failures = new Map<string, number>();
  private lastWhy = "";
  private running: Promise<void> | null = null;

  constructor(private readonly d: VizierMessagesDeps) {}

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
  private readCursor(): { threadId: string; seq: number } | null {
    try {
      const v = JSON.parse(this.d.store.setting(CURSOR_KEY) ?? "null") as { threadId?: unknown; seq?: unknown } | null;
      return v && typeof v.threadId === "string" && typeof v.seq === "number" ? { threadId: v.threadId, seq: v.seq } : null;
    } catch {
      return null;
    }
  }
  private writeCursor(threadId: string, seq: number): void {
    this.d.store.setSetting(CURSOR_KEY, JSON.stringify({ threadId, seq }));
  }

  /** The one pinned, unarchived vizier thread, or null (with the reason logged) when there are none or several. */
  async resolve(prefix: string): Promise<string | null> {
    if (prefix.trim() === "") {
      this.why("the title prefix is empty; nothing is filed");
      return null;
    }
    const rows = await this.d.threads.listRows();
    const ids = [...new Set(rows.filter((t) => t.archivedAt == null && t.deletedAt == null && t.pinnedAt != null && typeof t.title === "string" && t.title.startsWith(prefix)).map((t) => t.id))].sort();
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

  private async label(projectId: string): Promise<TaskLabel> {
    const find = (ls: TaskLabel[]) => ls.find((l) => sameName(l.name, VIZIER_MESSAGE_LABEL));
    const have = find(await this.d.tasks.listLabels(projectId, { fresh: true }));
    if (have) return have;
    try {
      await this.d.tasks.createLabel(projectId, VIZIER_MESSAGE_LABEL, LABEL_COLOR);
    } catch (e) {
      const again = find(await this.d.tasks.listLabels(projectId, { fresh: true }));
      if (again) return again;
      throw e;
    }
    const made = find(await this.d.tasks.listLabels(projectId, { fresh: true }));
    if (!made) throw new Error(`label ${VIZIER_MESSAGE_LABEL} was not created`);
    return made;
  }

  /** File one message: reuse a card that already carries its marker, else create one. Returns the card. */
  private async fileOne(projectId: string, threadId: string, e: MessageEvent, text: string): Promise<Task> {
    const lab = await this.label(projectId);
    // A crash or lost response after creation leaves a card whose id was never recorded: find it by marker first.
    const prior = (await this.d.tasks.listTasks({ projectId, labelIds: [lab.id] })).find((t) => hasMessageMarker(t.description, e.id));
    return (
      prior ??
      (await this.d.tasks.createTask({ projectId, title: messageTitle(text), description: messageDescription(text, threadId, e.seq, e.id), labelIds: [lab.id] }))
    );
  }

  private record(threadId: string, e: MessageEvent, taskId: string | null): void {
    this.d.store.atomically(() => {
      if (taskId !== null) {
        const f = this.readFiled();
        f[e.id] = taskId;
        const keys = Object.keys(f);
        for (const k of keys.slice(0, Math.max(0, keys.length - FILED_KEEP))) delete f[k];
        this.d.store.setSetting(FILED_KEY, JSON.stringify(f));
      }
      this.writeCursor(threadId, e.seq);
    });
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
    if (!cfg.enabled) return;
    try {
      if (cfg.project.trim() === "") {
        this.why("no tasks project is configured; nothing is filed");
        return;
      }
      const threadId = await this.resolve(cfg.titlePrefix);
      if (threadId === null) return;
      let cursor = this.readCursor();
      if (!cursor || cursor.threadId !== threadId) {
        // First enable (or a different vizier thread): no backfill. Start after the newest message there is now.
        const newest = await this.d.threads.events({ threadId, limit: 1, order: "desc" });
        const seq = newest.reduce((m, e) => Math.max(m, e.seq), 0);
        this.writeCursor(threadId, seq);
        this.d.log.info(`vizier-messages: watching ${threadId} from message ${seq}; earlier messages get no card`);
        cursor = { threadId, seq };
      }
      const projects = await this.d.tasks.listProjects();
      const want = cfg.project.trim().toLowerCase();
      const project = projects.find((p) => p.id === cfg.project.trim() || p.prefix.toLowerCase() === want || p.name.toLowerCase() === want);
      for (let page = 0; page < MAX_PAGES_PER_TICK; page++) {
        const rows = (await this.d.threads.events({ threadId, afterSeq: cursor.seq, limit: PAGE, order: "asc" })).filter((e) => e.seq > cursor!.seq).sort((a, b) => a.seq - b.seq);
        if (rows.length === 0) return;
        for (const e of rows) {
          const c = classify(e);
          if (c.kind === "skip") {
            this.writeCursor(threadId, e.seq);
            cursor = { threadId, seq: e.seq };
            continue;
          }
          if (this.readFiled()[e.id]) {
            this.writeCursor(threadId, e.seq); // a replayed event: already filed
            cursor = { threadId, seq: e.seq };
            continue;
          }
          if (!project) {
            this.why(`no tasks project matches "${cfg.project}"; nothing is filed`);
            return;
          }
          try {
            const task = await this.fileOne(project.id, threadId, e, c.text);
            this.record(threadId, e, task.id);
            this.failures.delete(e.id);
            this.d.log.info(`vizier-messages: filed ${task.key} for message ${e.seq}`);
          } catch (err) {
            const n = (this.failures.get(e.id) ?? 0) + 1;
            this.failures.set(e.id, n);
            this.d.log.warn(`vizier-messages: message ${e.seq} not filed (try ${n}): ${errText(err)}`);
            if (n < GIVE_UP_AFTER) return; // keep the cursor; the next tick retries this message
            this.d.log.warn(`vizier-messages: giving up on message ${e.seq} (${e.id}) after ${n} tries; it gets no card`);
            this.record(threadId, e, null);
            this.failures.delete(e.id);
          }
          cursor = { threadId, seq: e.seq };
        }
        if (rows.length < PAGE) return;
      }
    } catch (err) {
      this.d.log.warn(`vizier-messages: pass failed: ${errText(err)}`);
    }
  }
}
