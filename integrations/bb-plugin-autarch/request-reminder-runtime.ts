import { z } from "zod";
import type { BbPluginApi } from "@get-bb/plugin-sdk";
import { readAsk } from "./askbody.js";
import { VIZIER_TITLE_PREFIX } from "./delegation.js";
import { THREAD_EVENT_PAGE_CAP, threadEventLimit } from "./hostLimits.js";
import { confirmedRequestDelivery, type RequestEvent } from "./request-reminder-evidence.js";
import { RequestReminders, type ReminderRequestSource, type RequestRoute } from "./request-reminders.js";
import type { Store } from "./store.js";
import { TasksClient } from "./tasks.js";
import type { WakeSdk } from "./wakes.js";

export const REQUEST_SIGNAL_TOOL = "autarch_request_signal";
const PICK_CURSOR = "request-reminders:pick-cursor";
const TICK_MS = 15000;
const READ_LIMIT = 100;
const EVENT_PAGES = 5;
const THREAD_ID = /^thr_[a-zA-Z0-9_-]{1,120}$/;
const EVENT_TYPES = ["client/turn/requested", "client/turn/rejected", "turn/input/accepted"] as const;

interface PickSource {
  pick_id: string;
  picked_at: string;
  option_id: string;
  body_json: string;
  project: string;
  decision: string;
  task_id: string | null;
  thread: string;
  owner_thread: string | null;
  recipient: string | null;
  op: string | null;
  eligible: number;
}

// Proven user-authored instruction picks only. Notices, mirrored comments,
// protected routing and automated rulings have their own producers.
const PICK_SOURCE = `SELECT p.pick_id, p.picked_at, p.option_id, d.body_json,
    d.project, d.id AS decision, d.task_id, d.thread, d.owner_thread, w.recipient, w.op,
    CASE WHEN p."by" = 'mk' AND p.surface IN ('home', 'overlay') AND d.source = 'card'
          AND p.revision = d.revision AND d.withdrawn_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM decisions successor WHERE successor.supersedes = d.id)
          AND d.resolved_at IS NULL AND w.id IS NOT NULL AND w.voided_at IS NULL
          AND w.recipient = d.thread AND c.deleted_at IS NULL AND c.status <> 'canceled'
          AND NOT EXISTS (SELECT 1 FROM obligations x WHERE x.decision_id = d.id
                          AND x.kind IN ('pick-relay', 'pick-receipt'))
         THEN 1 ELSE 0 END AS eligible
    FROM picks p JOIN decisions d ON d.id = p.decision_id
    LEFT JOIN cards c ON c.task_id = d.task_id
    LEFT JOIN obligations w ON w.op = ('wake:' || d.id || ':' || p.pick_id) AND w.kind = 'wake'`;

function instruction(row: PickSource): ReminderRequestSource | null {
  const since = Date.parse(row.picked_at);
  if (!row.eligible || !row.task_id || !row.op || row.op.length > 200 || !row.recipient
      || !THREAD_ID.test(row.recipient) || !Number.isSafeInteger(since)) return null;
  const selected = readAsk(row.body_json, {}, "card")?.options?.find(option => option.id === row.option_id);
  if (selected?.kind !== "instruction" || !selected.instruction || selected.on_pick) return null;
  return { id: row.pick_id, ref: row.op, since, recipient: row.recipient,
    project: row.project, decision: row.decision, task: row.task_id,
    explicitOwner: row.owner_thread === row.recipient };
}

function readSources(store: Store): ReminderRequestSource[] {
  const cursor = store.setting(PICK_CURSOR);
  if (cursor === undefined) {
    const row = store.db.prepare("SELECT COALESCE(MAX(seq), 0) AS seq FROM events").get() as { seq: number };
    store.setSetting(PICK_CURSOR, String(row.seq));
    return [];
  }
  if (!/^\d+$/.test(cursor) || !Number.isSafeInteger(Number(cursor))) throw Error("invalid reminder source cursor");
  const rows = store.db.prepare(
    "SELECT seq, decision_id FROM events WHERE seq > ? AND type = 'picked' ORDER BY seq LIMIT 100",
  ).all(Number(cursor)) as { seq: number; decision_id: string }[];
  if (rows.length) store.setSetting(PICK_CURSOR, String(rows[rows.length - 1]!.seq));
  const lookup = store.db.prepare(`${PICK_SOURCE} WHERE p.decision_id = ?`);
  return rows.flatMap(({ decision_id }) => {
    const row = lookup.get(decision_id) as PickSource | undefined;
    const source = row && instruction(row);
    return source ? [source] : [];
  });
}

function stillEligible(store: Store, source: ReminderRequestSource): boolean {
  const row = store.db.prepare(`${PICK_SOURCE} WHERE p.pick_id = ?`).get(source.id) as PickSource | undefined;
  const current = row && instruction(row);
  return !!current && current.decision === source.decision && current.ref === source.ref
    && current.since === source.since && current.recipient === source.recipient
    && current.task === source.task && current.project === source.project && current.explicitOwner === source.explicitOwner;
}

type Threads = BbPluginApi["sdk"]["threads"];
async function providerDelivery(threads: Threads, threadId: string, ref: string, since: number) {
  const events: RequestEvent[] = [];
  let beforeSeq: number | undefined;
  for (let page = 0; page < EVENT_PAGES; page++) {
    const rows = await threads.events.list({
      threadId, types: EVENT_TYPES, order: "desc", limit: threadEventLimit(THREAD_EVENT_PAGE_CAP),
      ...(beforeSeq === undefined ? {} : { beforeSeq: String(beforeSeq) }),
    });
    const last = rows.at(-1);
    events.push(...rows as RequestEvent[]);
    if (rows.length < THREAD_EVENT_PAGE_CAP || last && last.createdAt < since) {
      return confirmedRequestDelivery(events, threadId, ref, since);
    }
    if (!last || beforeSeq !== undefined && last.seq >= beforeSeq) return null;
    beforeSeq = last.seq;
  }
  // The part of a busy thread's history we can see might omit another delivery.
  return null;
}

type AgentThread = Pick<Awaited<ReturnType<Threads["get"]>>, "id" | "visibility" | "archivedAt" | "deletedAt" | "providerId">;
function liveAgent(row: AgentThread | undefined): boolean {
  return !!row && THREAD_ID.test(row.id) && row.visibility === "visible"
    && row.archivedAt === null && row.deletedAt === null && !!row.providerId;
}

async function route(bb: BbPluginApi, store: Store, recipient: string, project: string, explicitOwner?: boolean): Promise<RequestRoute | null> {
  const thread = await bb.sdk.threads.get({ threadId: recipient });
  if (!liveAgent(thread) || thread.id !== recipient || !thread.projectId
      || thread.title?.startsWith(VIZIER_TITLE_PREFIX) || recipient === store.setting("vizierThreadId")) return null;
  const home = await bb.sdk.projects.get({ projectId: thread.projectId });
  if (home.name !== project || home.id !== thread.projectId) return null;
  const rows = await bb.sdk.threads.list({ projectId: home.id, includeHidden: false, limit: READ_LIMIT + 1 });
  if (rows.length > READ_LIMIT || new Set(rows.map(row => row.id)).size !== rows.length) return null;
  const coordinatorTitle = `${project} | coordinator`;
  const candidates = rows.filter(row => liveAgent(row) && row.title === coordinatorTitle && row.projectId === home.id);
  if (candidates.length !== 1) return thread.title === coordinatorTitle ? null : { project, role: "owner", next: null };
  const coordinator = candidates[0]!;
  if (coordinator.id !== recipient) return { project, role: "owner", next: coordinator.id };
  const vizier = store.setting("vizierThreadId");
  const next = vizier && THREAD_ID.test(vizier) && vizier !== recipient
    ? await bb.sdk.threads.get({ threadId: vizier }).catch(() => undefined)
    : undefined;
  return { project, role: explicitOwner ? "owner" : "coordinator",
    next: next && liveAgent(next) && next.id === vizier && next.title?.startsWith(VIZIER_TITLE_PREFIX) ? next.id : null };
}

const signal = z.discriminatedUnion("action", [
  z.object({ action: z.literal("list") }).strict(),
  z.object({ action: z.literal("reply"), request: z.string().min(1).max(160), comment: z.string().min(1).max(160) }).strict(),
  z.object({ action: z.literal("transfer"), request: z.string().min(1).max(160), to: z.string().regex(THREAD_ID) }).strict(),
]);

export function registerRequestReminders(bb: BbPluginApi, getStore: () => Store | null, sdk: WakeSdk): boolean {
  if (typeof bb.agents.registerTool !== "function" || !bb.sdk.plugins || !bb.sdk.threads.events) return false;
  const tasks = new TasksClient(bb.sdk.plugins);
  const enableFrom = process.env.AUTARCH_REQUEST_REMINDERS_FROM ?? "";
  const enabledAt = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/.test(enableFrom) ? Date.parse(enableFrom) : NaN;
  let state: { store: Store; reminders: RequestReminders } | null = null;
  const ready = () => {
    const store = getStore();
    if (!store) return null;
    if (state?.store === store) return state.reminders;
    const reminders = new RequestReminders(store, {
      now: Date.now,
      sources: () => readSources(store),
      eligible: source => stillEligible(store, source),
      deliver: (threadId, ref, since) => providerDelivery(bb.sdk.threads, threadId, ref, since),
      route: (threadId, project, explicitOwner) => route(bb, store, threadId, project, explicitOwner),
      async reply(task, comment, actor, since) {
        const rows = await tasks.listComments(task);
        return rows.some(r => r.id === comment && r.taskId === task && r.kind === "agent"
          && r.threadId === actor && Date.parse(r.createdAt) >= since && Date.parse(r.createdAt) <= Date.now());
      },
      enabled: source => source.since >= enabledAt && Date.now() >= enabledAt,
      send: args => sdk.send(args),
      removeQueued: (threadId, handle) => sdk.queuedDelete(threadId, handle),
      queuedList: threadId => sdk.queuedList(threadId),
    });
    state = { reminders, store };
    return reminders;
  };
  bb.agents.registerTool({
    name: REQUEST_SIGNAL_TOOL,
    description: "List Home requests assigned to your agent, link an existing task reply, or explicitly transfer a request to another agent.",
    instructions: "After replying on a Home request's task, link the exact comment ID to the request with action reply. Use action list to see your request IDs. Use action transfer with the live successor thread ID when handing off an unanswered request. The successor's clock starts only after its delivery is confirmed.",
    parameters: signal,
    async execute(input, ctx) {
      try {
        const reminders = ready();
        if (!reminders || !THREAD_ID.test(ctx.threadId) || ctx.signal.aborted) return JSON.stringify({ ok: false, error: "request evidence unavailable" });
        const actor = await bb.sdk.threads.get({ threadId: ctx.threadId });
        if (!liveAgent(actor) || actor.id !== ctx.threadId || actor.projectId !== ctx.projectId || ctx.signal.aborted) {
          return JSON.stringify({ ok: false, error: "request evidence unavailable" });
        }
        if (input.action === "list") {
          reminders.collect();
          return JSON.stringify({ ok: true, requests: reminders.list(ctx.threadId) });
        }
        if (input.action === "transfer") {
          const target = await bb.sdk.threads.get({ threadId: input.to });
          if (!liveAgent(target) || target.id !== input.to || target.projectId !== actor.projectId || ctx.signal.aborted) {
            return JSON.stringify({ ok: false, error: "transfer target is outside your live project" });
          }
        }
        const ok = input.action === "reply"
          ? await reminders.answer(input.request, ctx.threadId, input.comment)
          : await reminders.transfer(input.request, ctx.threadId, input.to);
        return JSON.stringify(ok ? { ok: true } : { ok: false, error: "request or evidence did not match your assignment" });
      } catch {
        return JSON.stringify({ ok: false, error: "request evidence unavailable" });
      }
    },
  });
  bb.background.service("home-request-reminders", {
    async start(signal) {
      let failed = false;
      while (!signal.aborted) {
        let complete = false;
        try {
          complete = await ready()?.tick() ?? true;
        } catch { /* A failed read or claim leaves reminders off for this tick. */ }
        if (!complete && !failed) bb.log.warn("request reminders: source evidence unavailable");
        failed = !complete;
        if (signal.aborted) break;
        await new Promise<void>(resolve => {
          const done = () => { clearTimeout(timer); signal.removeEventListener("abort", done); resolve(); };
          const timer = setTimeout(done, TICK_MS);
          signal.addEventListener("abort", done, { once: true });
        });
      }
    },
  });
  return true;
}
