// bb-plugin-autarch — a BB plugin backend entry.
//
// The default export is a factory that receives the plugin API. BB supplies
// the tiny defineRpcContract runtime helper; the API type remains type-only.
//
// The Home store serves the Home page (app.tsx, over RPC), the `bb home` CLI command
// (cli.ts) and the skill in skills/home/SKILL.md that tells agents how to use it. The
// example todo list in bb.storage.kv serves the Example todos page only. A write
// publishes a realtime signal so every open page refetches.
import { randomUUID } from "node:crypto";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { defineRpcContract, type BbPluginApi } from "@get-bb/plugin-sdk";
import { z } from "zod";
import { homeMethods } from "./contract.js";
import { sourceSha256 } from "./scripts/source-hash.mjs";
import { Asks } from "./asks.js";
import { Catchup } from "./catchup.js";
import { homeCli } from "./cli.js";
import { Delegation } from "./delegation.js";
import { exportEvents } from "./export.js";
import { FeedCaches } from "./feed.js";
import { parseAsk } from "./model.js";
import { ServeClient, tokenReader } from "./serve.js";
import { CardWriter } from "./cardwrites.js";
import { Queue } from "./queue.js";
import { rootRun } from "./rootrun.js";
import { buildQueue, removeBinding, setBinding } from "./queueview.js";
import { Service } from "./service.js";
import { TasksClient, type PluginsLike } from "./tasks.js";
import { createStoreHandle, type Store, type StoreHandle } from "./store.js";
import { sdkAdapter, WakeLoop, type ThreadsLike, type WakeSdk } from "./wakes.js";

const todoSchema = z.object({
  id: z.string(),
  title: z.string(),
  done: z.boolean(),
  createdAt: z.string(),
});
export type Todo = z.infer<typeof todoSchema>;

// Both schemas run at the wire boundary. Handler input/output are inferred
// from the shared contract; app.tsx imports only its type.
export const rpcContract = defineRpcContract({
  ...homeMethods,
  todos_list: {
    input: z.null(),
    output: z.object({ todos: z.array(todoSchema) }),
  },
  todos_add: {
    input: z.object({ title: z.string().trim().min(1).max(200) }),
    output: todoSchema,
  },
  todos_set_done: {
    input: z.object({ id: z.string(), done: z.boolean() }),
    output: todoSchema,
  },
  todos_remove: {
    input: z.object({ id: z.string() }),
    output: z.object({ removed: z.boolean() }),
  },
});

/** Realtime channel app.tsx listens on; the payload is the todo count. */
const TODOS_CHANGED = "todos-changed";

/** Nightly export cadence. */
const EXPORT_EVERY_MS = 24 * 60 * 60 * 1000;

/**
 * Store wiring (Task 1.2). The factory never throws on a locked or busy database (a refused migration is the exception: it throws): the
 * handle stays not-ready and retries, so surfaces registered later report "not ready"
 * instead of the plugin failing to load. The nightly export writes segments outside
 * the plugin folder.
 */
export function wireStore(bb: BbPluginApi): StoreHandle {
  // closeOnFailure: a retry that fails after the open must not leave its connection behind.
  const handle = createStoreHandle(() => bb.storage.database(), { log: bb.log, closeOnFailure: true });
  bb.onDispose(() => handle.dispose());
  // A refused migration fails activation: bb keeps the previous instance ("reload failed") and nothing migrates later.
  const refused = handle.refusal();
  if (refused) throw refused;
  bb.background.service("home-export", {
    async start(signal) {
      while (!signal.aborted) {
        await new Promise<void>((resolve) => {
          const timer = setTimeout(resolve, EXPORT_EVERY_MS);
          signal.addEventListener(
            "abort",
            () => {
              clearTimeout(timer);
              resolve();
            },
            { once: true },
          );
        });
        if (signal.aborted || !handle.ready()) continue;
        try {
          exportEvents(handle.store());
        } catch (e) {
          bb.log.warn(`export failed: ${e instanceof Error ? e.message : String(e)}`);
        }
      }
    },
  });
  return handle;
}

export interface HomeConfig {
  serveAddr: string;
  serveTokenFile: string;
  serveProjectDirs: string[];
  autarchBin: string;
}

let sourceHash: string | null = null;
/** sha256 over the plugin root's sources (excluding identity.json, dist and node_modules), computed once at first use. */
function pluginSource(): string | null {
  if (sourceHash === null) {
    try {
      sourceHash = sourceSha256(dirname(fileURLToPath(import.meta.url)));
    } catch {
      return null;
    }
  }
  return sourceHash;
}

const FEED_REFRESH_MS = 30_000;
const SERVE_CHECK_MS = 15_000;
const MK = "mk";

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    if (signal.aborted) return resolve();
    const timer = setTimeout(resolve, ms);
    signal.addEventListener("abort", () => (clearTimeout(timer), resolve()), { once: true });
  });

interface Parts {
  store: Store;
  svc: Service;
  asks: Asks;
  dele: Delegation;
  catchup: Catchup;
  loop: WakeLoop;
  caches: FeedCaches;
}

/**
 * Home wiring (Task 1.7). Everything that needs the store waits for the handle to be ready;
 * until then RPC handlers refuse with "not ready" and the feed injects nothing. Returns the
 * RPC handlers so the caller registers them together with the other contract methods.
 */
export function wireHome(bb: BbPluginApi, handle: StoreHandle, cfg: HomeConfig, deps: { serve?: ServeClient; sdk?: WakeSdk } = {}) {
  const serve = deps.serve ?? new ServeClient({ addr: cfg.serveAddr, readToken: tokenReader(cfg.serveTokenFile) });
  let parts: Parts | null = null;
  const need = (): Parts => {
    if (!parts) throw new Error(`home store not ready${handle.error() ? `: ${handle.error()}` : ""}`);
    return parts;
  };

  handle.onReady((store) => {
    if (parts) return;
    // The nudge closes over `parts` so a pick or override wakes the loop and refreshes the feed.
    const nudge = () => {
      parts?.caches.invalidate();
      void parts?.loop.nudge();
    };
    const svc = new Service({ store, projects: () => serve.projects(), nudge });
    const dele = new Delegation(svc);
    const asks = new Asks(svc);
    const catchup = new Catchup(svc, dele);
    const loop = new WakeLoop(svc, deps.sdk ?? sdkAdapter(bb.sdk.threads as unknown as ThreadsLike));
    const caches = new FeedCaches(() => store.db, () => Date.now());
    parts = { store, svc, asks, dele, catchup, loop, caches };
    svc.start();
  });
  bb.onDispose(() => parts?.svc.stop());

  // The one write path for a binding, shared by mk's RPC and the vizier's CLI. `by` is logged on the event.
  const applyBinding = async (i: { tasks_project_id: string; state: "confirmed" | "rejected"; home_project?: string }, by: string) => {
    const p = need();
    const known = (await p.svc.serveProjects()).map((x) => x.name);
    const r = setBinding(p.store.db, i, { now: p.svc.time(), knownProjects: known, record: (type, detail) => p.store.recordEvent(type, null, { ...(detail as object), by }) });
    if (r.ok) afterBindingChange(i.tasks_project_id);
    return r;
  };
  const afterBindingChange = (tasksProjectId: string) => {
    const p = need();
    const ids = p.store.db.prepare("SELECT task_id FROM cards WHERE project_id = ?").all(tasksProjectId) as { task_id: string }[];
    queueRef?.rebind(ids.map((x) => x.task_id));
    p.caches.invalidate();
    bb.realtime.publish("home-queue-changed", {});
  };
  const resolveTasksProject = async (ref: string): Promise<{ ok: true; id: string } | { ok: false; status: number; error: string }> => {
    if (!tasksRef) return { ok: false, status: 503, error: "tasks unavailable" };
    let projects;
    try {
      projects = await tasksRef.listProjects();
    } catch {
      return { ok: false, status: 503, error: "tasks unavailable" };
    }
    const hits = projects.filter((x) => x.id === ref || x.prefix.toLowerCase() === ref.toLowerCase() || x.name.toLowerCase() === ref.toLowerCase());
    if (hits.length === 0) return { ok: false, status: 404, error: `no tasks project ${JSON.stringify(ref)}` };
    if (hits.length > 1) return { ok: false, status: 400, error: `${JSON.stringify(ref)} matches ${hits.length} tasks projects; use the id` };
    return { ok: true, id: hits[0]!.id };
  };

  // ---- CLI ----------------------------------------------------------------------
  const lazy = <T extends object>(pick: (p: Parts) => T): T =>
    new Proxy({} as T, { get: (_t, k) => (pick(need()) as Record<string | symbol, unknown>)[k] });
  bb.cli.register(
    homeCli({
      svc: lazy((p) => p.svc),
      asks: lazy((p) => p.asks),
      catchup: lazy((p) => p.catchup),
      rule: (id, option, reason, ctx) => need().dele.rule(id, option, reason, ctx),
      bind: async (ref, home, by) => {
        const t = await resolveTasksProject(ref);
        if (!t.ok) return t;
        const r = await applyBinding({ tasks_project_id: t.id, state: "confirmed", home_project: home }, by);
        return r.ok ? { ok: true as const, tasks_project_id: t.id } : r;
      },
      unbind: async (ref, by) => {
        const t = await resolveTasksProject(ref);
        if (!t.ok) return t;
        const p = need();
        const r = removeBinding(p.store.db, { tasks_project_id: t.id }, { record: (type, detail) => p.store.recordEvent(type, null, { ...(detail as object), by }) });
        if (!r.ok) return r;
        afterBindingChange(t.id);
        return { ok: true as const, tasks_project_id: t.id, was: r.was };
      },
      isVizier: (t) => t !== undefined && parts !== null && parts.dele.settings().vizierThreadId === t,
    }),
  );

  // ---- events -------------------------------------------------------------------
  const guard = (name: string, fn: () => unknown) => {
    try {
      const r = fn();
      if (r instanceof Promise) r.catch((e) => bb.log.warn(`${name}: ${e instanceof Error ? e.message : String(e)}`));
    } catch (e) {
      bb.log.warn(`${name}: ${e instanceof Error ? e.message : String(e)}`);
    }
  };
  bb.events.on("message.dispatched", ({ entry }) => guard("message.dispatched", () => parts?.loop.onQueueEvent({ type: "dispatched", id: entry.id })));
  bb.events.on("message.cancelled", ({ entry }) => guard("message.cancelled", () => parts?.loop.onQueueEvent({ type: "cancelled", id: entry.id })));
  bb.events.on("thread.archived", ({ thread }) => guard("thread.archived", () => parts?.loop.onThreadGone(thread.id)));
  bb.events.on("thread.deleted", ({ thread }) => guard("thread.deleted", () => parts?.loop.onThreadGone(thread.id)));
  bb.events.on("turn.failed", (e) => guard("turn.failed", () => parts?.catchup.recordTurnFailed(e.threadId, e.requestId)));

  // ---- feed injection -------------------------------------------------------------
  bb.agents.configure((ctx) => {
    const text = parts?.caches.configure(ctx.project.name, ctx.thread.id);
    return text === undefined ? { tools: [], skills: [] } : { tools: [], skills: [], instructions: text };
  });

  // ---- background -----------------------------------------------------------------
  bb.background.service("home-wakes", {
    async start(signal) {
      while (!signal.aborted && !parts) await sleep(1000, signal);
      if (parts) await parts.loop.start(signal);
    },
  });
  let queueRef: Queue | null = null;
  let tasksRef: TasksClient | null = null;
  // Card poller (Task 2.4). Read-only toward tasks, over plugins.callRpc; never spawns the bb CLI.
  bb.background.service("home-queue", {
    async start(signal) {
      while (!signal.aborted && !parts) await sleep(1000, signal);
      // bb.sdk.plugins.callRpc({ pluginId, method, input, signal, outputSchema }) is the SDK's own
      // PluginsArea (plugin-sdk bundled-types, PluginRpcArgs); the guard covers a host without it.
      const plugins = (bb.sdk as { plugins?: PluginsLike }).plugins;
      if (!parts || signal.aborted) return;
      if (!plugins) {
        console.error("home-queue: bb.sdk.plugins is unavailable, cards are not polled");
        return;
      }
      const tasks = (tasksRef = new TasksClient(plugins));
      const queue = (queueRef = new Queue({
        service: parts.svc,
        tasks,
        writer: new CardWriter(parts.svc.store.db, tasks, () => parts!.svc.time()),
        publish: () => {
          parts?.caches.invalidate();
          bb.realtime.publish("home-queue-changed", {});
        },
      }));
      await queue.run(signal, sleep);
    },
  });
  bb.background.service("home-feed-refresh", {
    async start(signal) {
      while (!signal.aborted) {
        await sleep(FEED_REFRESH_MS, signal);
        guard("feed-refresh", () => parts?.caches.refresh());
      }
    },
  });
  // Plan 1.3.6: until serve's project list has been checked against the delegation settings, delegation is
  // refused. Retried until it succeeds; read-only toward serve, and serve is not started from here.
  bb.background.service("home-delegation-check", {
    async start(signal) {
      while (!signal.aborted && !parts) await sleep(1000, signal);
      while (!signal.aborted && parts) {
        try {
          if (await parts.dele.verifyProjects(() => serve.projects())) return;
        } catch (e) {
          bb.log.warn(`delegation check: ${e instanceof Error ? e.message : String(e)}`);
        }
        await sleep(SERVE_CHECK_MS, signal);
      }
    },
  });

  // ---- RPC ------------------------------------------------------------------------
  const handlers = {
    async listAsks(_: null) {
      const p = need();
      const rows = p.svc.owed().map((d) => ({
        id: d.id,
        project: d.project,
        thread: d.thread,
        key: p.svc.cardKey(d.task_id as string | null),
        subject: d.subject,
        asker: d.asker,
        filed_at: d.filed_at,
        revision: d.revision,
        ask: parseAsk(JSON.parse(d.body_json)),
        mentions: p.store.mentions(d.id).length,
      }));
      let machineOwners: unknown = {};
      try {
        machineOwners = JSON.parse(p.store.setting("machineOwners") ?? "{}");
      } catch {
        /* unreadable: show none */
      }
      const item = p.dele.latestSettingsItem();
      return {
        owed: rows,
        runbook: p.asks.runbook(),
        ...p.asks.lists(),
        undeliverable: p.svc.undeliverable(),
        failures: p.svc.failures(),
        uncertain: p.svc.wakes().filter((o) => o.state === "uncertain"),
        approvals: p.store.liveApprovals(),
        delegation: { settings: p.dele.settings(), suspended: item !== undefined && !p.store.hasSeen(MK, item) },
        machineOwners,
      };
    },
    async queue(i: { thread?: string }) {
      const p = need();
      let serveProjects: string[] = [];
      try {
        serveProjects = (await p.svc.serveProjects()).map((x) => x.name).sort();
      } catch {
        /* serve down: no picker choices until it is back */
      }
      return { ...buildQueue(p.svc, p.asks, i.thread === undefined ? {} : { thread: i.thread }), serve_projects: serveProjects, status: queueRef?.status() ?? null };
    },
    async rootRun(i: { task_id: string }) {
      const p = need();
      // Only a card Home already knows; the id is never used to read anything else.
      const known = p.store.db.prepare("SELECT 1 AS x FROM cards WHERE task_id = ? AND deleted_at IS NULL").get(i.task_id);
      if (!known) return { ok: false as const, error: "unknown card" };
      if (!tasksRef) return { ok: false as const, error: "tasks unavailable" };
      try {
        const st = await tasksRef.taskState(i.task_id);
        if (st.state === "deleted") return { ok: false as const, error: "card deleted in tasks" };
        const comments = await tasksRef.listComments(i.task_id);
        return { ok: true as const, view: await rootRun({ task: { id: st.task.id, projectId: st.task.projectId, description: st.task.description }, comments }) };
      } catch {
        return { ok: false as const, error: "tasks unavailable" };
      }
    },
    async setBinding(i: { tasks_project_id: string; state: "confirmed" | "rejected"; home_project?: string }) {
      return applyBinding(i, MK);
    },
    async listRecent(i: { project: string; limit: number }) {
      return { recent: need().svc.recent(i.project, i.limit) };
    },
    async pick(i: { decision_id: string; option_id: string; revision: string; pick_id: string; reason?: string; surface?: "home" | "overlay" }) {
      return need().svc.pick(i.decision_id, i.option_id, i.revision, i.pick_id, MK, i.surface ?? "home", i.reason);
    },
    async dismiss(i: { decision_id: string; obligation_id: string }) {
      return need().svc.dismiss(i.decision_id, i.obligation_id);
    },
    async override(i: { decision_id: string }) {
      const r = need().dele.override(i.decision_id, {});
      if (r.ok) need().caches.invalidate();
      return r;
    },
    async revokeApproval(i: { approval_id: string }) {
      return need().svc.revokeApproval(i.approval_id);
    },
    async setDelegation(i: { vizierThreadId: string; projects: string[]; dailyCap: number }) {
      return need().dele.setDelegation(i, {});
    },
    async resend(i: { id: string; attempt: number; click_id: string }) {
      const p = need();
      const r = p.loop.resend(i.id, i.attempt, i.click_id);
      if (r.ok) void p.loop.nudge();
      return r;
    },
    /** Home reports which ask is on screen. Only the last one is kept, in this plugin's own store; `bb home viewing` reads it. */
    async setViewing(i: { decision_id: string | null }) {
      const p = need();
      if (i.decision_id === null) {
        p.store.setSetting("viewing", "null");
        return { ok: true };
      }
      if (!p.svc.owed().some((d) => d.id === i.decision_id)) return { ok: false, error: "not an open ask" };
      p.store.setSetting("viewing", JSON.stringify({ decision_id: i.decision_id, at: p.svc.time() }));
      return { ok: true };
    },
    async markSeen(i: { item: string }) {
      need().dele.markSeen(MK, i.item);
      return { ok: true };
    },
    async markAllSeen(i: { ids: string[] }) {
      return { marked: need().catchup.markAllSeen(i.ids) };
    },
    async catchup(_: null) {
      return { items: need().catchup.items() };
    },
    async stats(i: { days: number }) {
      const p = need();
      return p.svc.stats(new Date(Date.parse(p.svc.time()) - i.days * 86_400_000).toISOString());
    },
    async health(_: null) {
      let projects: unknown;
      let projectsError: string | undefined;
      try {
        projects = await serve.projects();
      } catch (e) {
        projectsError = e instanceof Error ? e.message : String(e);
      }
      let build: unknown = null;
      try {
        build = (await serve.health()).build ?? null;
      } catch {
        /* serve down: build stays null */
      }
      return { ready: handle.ready(), error: handle.error(), source_sha256: pluginSource(), build, projects: projects ?? null, projects_error: projectsError ?? null };
    },
  };
  // bb validates an RPC result as a JSON value, and an `undefined` member fails the whole call
  // (found by the real-bb run: listAsks failed on an ask with no machine block). Round-trip
  // every result through JSON so absent members are dropped, as they would be on the wire.
  const jsonSafe = <F extends (i: never) => Promise<unknown>>(f: F): F =>
    (async (i: never) => {
      const r = await f(i);
      return r === undefined ? null : JSON.parse(JSON.stringify(r));
    }) as F;
  const safe = Object.fromEntries(Object.entries(handlers).map(([k, f]) => [k, jsonSafe(f as never)])) as unknown as typeof handlers;
  return { handlers: safe };
}

export default async function plugin(bb: BbPluginApi) {
  bb.log.info("loaded");
  const handle = wireStore(bb);

  // Declarative settings — rendered in BB's settings UI and editable with
  // `bb plugin config autarch`. Add `secret: true` for values like API keys.
  // Settings are read once per load: reload the plugin after changing one.
  const settings = bb.settings.define({
    showDone: {
      type: "boolean",
      label: "Show completed todos",
      default: true,
    },
  });
  const { showDone } = await settings.get();

  // The Home surfaces: RPC handlers, the `bb home` CLI, wakes, feed injection, serve link.
  const homeSettings = bb.settings.define({
    serveAddr: { type: "string", label: "autarch serve address", default: "127.0.0.1:8110" },
    serveTokenFile: { type: "string", label: "autarch serve token file", default: "" },
    serveProjectDirs: { type: "string", label: "Project scan roots (comma separated; empty uses Bigend's)", default: "" },
    autarchBin: { type: "string", label: "autarch binary", default: "" },
  });
  const hs = await homeSettings.get();
  const home = wireHome(bb, handle, {
    serveAddr: hs.serveAddr || "127.0.0.1:8110",
    serveTokenFile: hs.serveTokenFile || join(homedir(), ".autarch", "serve.token"),
    serveProjectDirs: (hs.serveProjectDirs || "").split(",").map((d: string) => d.trim()).filter(Boolean),
    autarchBin: hs.autarchBin || process.env.AUTARCH_BIN || "autarch",
  });

  // Namespaced key-value storage in bb.db (JSON values, up to 256KB each).
  // For bigger or relational data use bb.storage.database().
  async function readTodos(): Promise<Todo[]> {
    return (await bb.storage.kv.get<Todo[]>("todos")) ?? [];
  }
  async function writeTodos(todos: Todo[]): Promise<void> {
    await bb.storage.kv.set("todos", todos);
    // Ephemeral broadcast to every connected client; nothing is persisted.
    bb.realtime.publish(TODOS_CHANGED, { count: todos.length });
  }

  async function listTodos(): Promise<Todo[]> {
    const todos = await readTodos();
    return showDone ? todos : todos.filter((todo) => !todo.done);
  }
  async function addTodo(title: string): Promise<Todo> {
    const todo: Todo = {
      id: randomUUID().slice(0, 8),
      title,
      done: false,
      createdAt: new Date().toISOString(),
    };
    await writeTodos([...(await readTodos()), todo]);
    return todo;
  }
  async function setTodoDone(id: string, done: boolean): Promise<Todo | null> {
    const todos = await readTodos();
    const todo = todos.find((candidate) => candidate.id === id);
    if (todo === undefined) return null;
    todo.done = done;
    await writeTodos(todos);
    return todo;
  }
  async function removeTodo(id: string): Promise<boolean> {
    const todos = await readTodos();
    const remaining = todos.filter((todo) => todo.id !== id);
    if (remaining.length === todos.length) return false;
    await writeTodos(remaining);
    return true;
  }

  bb.rpc.register(rpcContract, {
    ...home.handlers,
    todos_list: async () => ({ todos: await listTodos() }),
    todos_add: ({ title }) => addTodo(title),
    todos_set_done: async ({ id, done }) => {
      const todo = await setTodoDone(id, done);
      if (todo === null) throw new Error(`No todo with id ${id}`);
      return todo;
    },
    todos_remove: async ({ id }) => ({ removed: await removeTodo(id) }),
  });

  // Cleanup on reload/disable/shutdown; hooks run LIFO. The sanctioned place
  // to clear timers and close connections.
  bb.onDispose(() => {
    bb.log.info("disposed");
  });

  // Long-lived background work: starts after load, gets an AbortSignal on
  // reload/disable/shutdown, and restarts with backoff if it crashes. Sleeps
  // must wake on abort — a plain setTimeout sleeps through the stop window
  // and the plugin reports "degraded (service did not stop)" on reload.
  // bb.background.service("worker", {
  //   async start(signal) {
  //     while (!signal.aborted) {
  //       await new Promise((resolve) => {
  //         const timer = setTimeout(resolve, 60_000);
  //         signal.addEventListener(
  //           "abort",
  //           () => { clearTimeout(timer); resolve(undefined); },
  //           { once: true },
  //         );
  //       });
  //     }
  //   },
  // });
}
