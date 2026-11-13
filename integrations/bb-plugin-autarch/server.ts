// bb-plugin-autarch — a BB plugin backend entry.
//
// The default export is a factory that receives the plugin API. BB supplies
// the tiny defineRpcContract runtime helper; the API type remains type-only.
//
// The Home store serves the Home page (app.tsx, over RPC), the `bb home` CLI command
// (cli.ts) and the skill in skills/home/SKILL.md that tells agents how to use it. A write
// publishes a realtime signal so every open page refetches.
import { postNoteOnce } from "./notepost.js";
import { threadEventLimit } from "./hostLimits.js";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { defineRpcContract, type BbPluginApi } from "@get-bb/plugin-sdk";
import { homeMethods } from "./contract.js";
import { sourceSha256 } from "./scripts/source-hash.mjs";
import { Asks } from "./asks.js";
import { Catchup } from "./catchup.js";
import { waitingNow } from "./waiting.js";
import { homeCli } from "./cli.js";
import { ledgerPath } from "./ledger.js";
import { Delegation, type ThreadRow } from "./delegation.js";
import { exportEvents } from "./export.js";
import { FeedCaches } from "./feed.js";
import { owedAsk } from "./askbody.js";
import { ServeClient, tokenReader } from "./serve.js";
import { CardWriter } from "./cardwrites.js";
import { Queue } from "./queue.js";
import { touchTrigger } from "./trigger.js";
import { checkRequest, updateInfo } from "./updates.js";
import { ghViewReal } from "./ghview.js";
import { MergeRecheck } from "./mergerecheck.js";
import { CommentPoller, conversationView, openTaskIds, unreadCounts } from "./conversation.js";
import { readDoc, resolveOpenTarget } from "./docread.js";
import { rootRun } from "./rootrun.js";
import { buildQueue, removeBinding, setBinding } from "./queueview.js";
import { RpcNotifier } from "./notifier.js";
import { isoWeek, Service } from "./service.js";
import { Ideas } from "./ideas.js";
import { DEFAULT_TITLE_PREFIX, VizierMessages, type MessageEvent } from "./vizierMessages.js";
import { archiveView, type ArchiveQuery } from "./archive.js";
import { learnSet, learnView, type LearnSet } from "./learn.js";
import { OPEN_STATUSES, TasksClient, type PluginsLike } from "./tasks.js";
import { listAllThreads, traceability, type ThreadLike } from "./traceability.js";
import { cardStateOf } from "./cardstate.js";
import { createStoreHandle, type Store, type StoreHandle } from "./store.js";
import { defaultImagesRoot, ImageStore } from "./otherimages.js";
import { sdkAdapter, WakeLoop, type ThreadsLike, type WakeSdk } from "./wakes.js";
// Evidence has a separate store and a read-only RPC; it never uses Other-box attachments.
// Keep this import beside that store's existing wiring.

import { readEvidence } from "./evidencestore.js";

// Both schemas run at the wire boundary. Handler input/output are inferred
// from the shared contract; app.tsx imports only its type.
export const rpcContract = defineRpcContract({
  ...homeMethods,
});

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
  /** Where the update runner writes status.json and result.json (read-only here). */
  updateDir?: string;
  /** The runner's systemd timer file, read for its check interval; unset means the default user-unit path. */
  updateTimerFile?: string;
  /** The rulings ledger file; null turns it off. Unset means the default path (see ledger.ts). */
  rulingsLedger?: string | null;
  /** Ship dark: a card for each plain operator message in the vizier thread is off unless this is true. */
  vizierMessageCards?: boolean;
  /** Title prefix of the vizier thread; empty means the default. */
  vizierTitlePrefix?: string;
  /** Tasks project (id, prefix or name) that receives those cards; empty files nothing. */
  vizierCardProject?: string;
}

/** The three settings the vizier message watcher follows. */
export type VizierSettings = Pick<HomeConfig, "vizierMessageCards" | "vizierTitlePrefix" | "vizierCardProject">;
/**
 * Where those three settings are read, once per watcher tick. A rejection makes the tick treat the watcher as off.
 * Without a source the watcher follows the values in `HomeConfig` for the life of the process.
 */
export type VizierSettingsSource = () => Promise<VizierSettings>;

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
const VIZIER_MESSAGE_POLL_MS = 15_000;
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
export function wireHome(bb: BbPluginApi, handle: StoreHandle, cfg: HomeConfig, deps: { serve?: ServeClient; sdk?: WakeSdk; vizierSettings?: VizierSettingsSource } = {}) {
  const updateDir = cfg.updateDir ?? join(homedir(), ".local", "state", "autarch-home-update");
  const updateEnv = (now: string) => ({ now, ...(cfg.updateTimerFile ? { timerFile: cfg.updateTimerFile } : {}) });
  const serve = deps.serve ?? new ServeClient({ addr: cfg.serveAddr, readToken: tokenReader(cfg.serveTokenFile) });
  let parts: Parts | null = null;
  /** Every thread, paged: the vizier resolver needs title, pinnedAt, archivedAt and deletedAt. */
  let threadCache: { at: number; rows: ThreadRow[] } | null = null;
  const toRow = (t: ThreadRow): ThreadRow => ({ id: t.id, title: t.title ?? null, pinnedAt: t.pinnedAt ?? null, archivedAt: t.archivedAt ?? null, deletedAt: t.deletedAt ?? null });
  const listThreadRows = async (o: { cached?: boolean } = {}): Promise<ThreadRow[]> => {
    // Only the panel's read-only peek may be a few seconds old; every authority check lists fresh.
    if (o.cached && threadCache && Date.now() - threadCache.at < 3000) return threadCache.rows;
    const rows: ThreadRow[] = [];
    let offset = 0;
    for (let page = 0; page < 2000; page++) {
      const got = (await bb.sdk.threads.list({ limit: 100, offset, includeHidden: true })) as unknown as ThreadRow[];
      rows.push(...got.map(toRow));
      offset += got.length; // advance by what came back: a host that caps the page size skips nothing
      if (got.length === 0) {
        if (new Set(rows.map((r) => r.id)).size !== rows.length) throw new Error("thread list shifted while it was paged");
        threadCache = { at: Date.now(), rows };
        return rows;
      }
    }
    throw new Error("thread list did not end");
  };
  const getThreadRow = async (id: string): Promise<ThreadRow | null> => {
    const t = (await bb.sdk.threads.get({ threadId: id })) as unknown as ThreadRow | null | undefined;
    return t ? toRow(t) : null;
  };
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
    const images = new ImageStore(defaultImagesRoot());
    try {
      images.sweep();
    } catch (e) {
      bb.log.warn(`image sweep failed: ${e instanceof Error ? e.message : String(e)}`);
    }
    // The blocker phone push goes over plugins.callRpc to the push-notifications plugin; a host without
    // bb.sdk.plugins simply makes no push.
    const sdkPlugins = (bb.sdk as { plugins?: PluginsLike }).plugins;
    const svc = new Service({ store, images, projects: () => serve.projects(), nudge, ...(sdkPlugins ? { notifier: new RpcNotifier(sdkPlugins) } : {}), log: (m) => bb.log.warn(m), rulingsLedger: cfg.rulingsLedger !== undefined ? cfg.rulingsLedger : ledgerPath() });
    const dele = new Delegation(svc, listThreadRows, getThreadRow);
    const asks = new Asks(svc);
    const catchup = new Catchup(svc, dele);
    const loop = new WakeLoop(svc, deps.sdk ?? sdkAdapter(bb.sdk.threads as unknown as ThreadsLike), (gone) => resolveWakeTarget("", gone, true));
    const caches = new FeedCaches(() => store.db, () => Date.now());
    parts = { store, svc, asks, dele, catchup, loop, caches };
    svc.start();
  });
  bb.onDispose(() => parts?.svc.stop());

  // The one write path for a binding, shared by mk's RPC and the vizier's CLI. `by` is logged on the event.
  const applyBinding = async (i: { tasks_project_id: string; state: "confirmed" | "rejected"; home_project?: string }, by: string, stillAllowed?: () => Promise<boolean>, recheck?: () => boolean) => {
    const p = need();
    const known = (await p.svc.serveProjects()).map((x) => x.name);
    // The vizier's authority is re-read right before the write: a handoff or archive during the awaits above ends it.
    // The last await is the liveness read; the stored-id recheck and the write share its continuation.
    if (stillAllowed && (!(await stillAllowed()) || !recheck?.())) return { ok: false as const, status: 403, error: "only the vizier thread may bind a project" };
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
      waiting: () => waitingNow(need().svc, need().dele, need().catchup),
      rule: (id, option, reason, ctx) => need().dele.ruleResolved(id, option, reason, ctx),
      handoff: (to, ctx) => need().dele.handoff(to, ctx),
      bind: async (ref, home, by) => {
        const t = await resolveTasksProject(ref);
        if (!t.ok) return t;
        const r = await applyBinding({ tasks_project_id: t.id, state: "confirmed", home_project: home }, by, async () => (await need().dele.isLive(by, false)) === true, () => need().dele.isStoredVizier(by));
        return r.ok ? { ok: true as const, tasks_project_id: t.id } : r;
      },
      unbind: async (ref, by) => {
        const t = await resolveTasksProject(ref);
        if (!t.ok) return t;
        const p = need();
        const live = (await p.dele.isLive(by, false)) === true;
        if (!live || !p.dele.isStoredVizier(by)) return { ok: false as const, status: 403, error: "only the vizier thread may unbind a project" };
        const r = removeBinding(p.store.db, { tasks_project_id: t.id }, { record: (type, detail) => p.store.recordEvent(type, null, { ...(detail as object), by }) });
        if (!r.ok) return r;
        afterBindingChange(t.id);
        return { ok: true as const, tasks_project_id: t.id, was: r.was };
      },
      traceability: (since) =>
        traceability(
          {
            listProjects: () => (tasksRef ? tasksRef.listProjects() : Promise.reject(new Error("tasks unavailable"))),
            listOpenTasks: (projectId) => tasksRef!.listTasks({ projectId, statuses: OPEN_STATUSES }),
            listThreads: () => listAllThreads(async (a) => (await bb.sdk.threads.list(a)) as unknown as (ThreadLike & { deletedAt?: unknown })[]),
            output: async (threadId) => ((await bb.sdk.threads.output({ threadId })) as { output: string | null }).output,
          },
          { since, now: need().svc.time() },
        ),
      isVizier: async (t) => {
        if (t === undefined || parts === null) return false;
        const r = await parts.dele.resolveVizier();
        return r.ok && r.id === t;
      },
      stillVizier: (t) => parts !== null && parts.dele.isStoredVizier(t),
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
  // One desk for the process: its per-key serialisation only works when every RPC shares the same instance.
  let ideaDeskRef: { tasks: TasksClient; desk: Ideas } | null = null;
  const ideaDesk = (): Ideas | null => {
    if (!tasksRef || !parts) return null;
    if (!ideaDeskRef || ideaDeskRef.tasks !== tasksRef) ideaDeskRef = { tasks: tasksRef, desk: new Ideas({ tasks: tasksRef, store: parts.svc.store, now: () => new Date().toISOString(), nudge: () => void parts?.loop.nudge() }) };
    return ideaDeskRef.desk;
  };
  // A card for every plain operator message in the vizier thread: a stateless check every 15 s over the last 24 hours
  // (off unless the setting is on; see vizierMessages.ts). Its three settings are read afresh on every tick, so turning
  // it on or off, or changing the project or the title prefix, needs no plugin reload.
  const vizierNow = async (): Promise<VizierSettings> => (deps.vizierSettings ? deps.vizierSettings() : cfg);
  bb.background.service("home-vizier-messages", {
    async start(signal) {
      // The activation time is taken here, before the wait for the store and the tasks client: a message the operator
      // sends while those are still coming up is after the activation and gets its card.
      // A failed read here is carried into the watcher: its first tick takes it as that tick's failed read (nothing filed,
      // the saved switch-on time reset, one warning, the usual backoff), so settings that recover before the first tick
      // cannot reuse an activation saved before the failure.
      let startupFailure: { err: unknown } | null = null;
      const observedAt = await vizierNow().then(
        (v) => (v.vizierMessageCards === true ? Date.now() : undefined),
        (err: unknown) => {
          startupFailure = { err };
          return undefined;
        },
      );
      while (!signal.aborted && !(parts && tasksRef)) await sleep(1000, signal);
      if (!parts || !tasksRef || signal.aborted) return;
      const watcher = new VizierMessages({
        ...(observedAt !== undefined ? { observedAt } : {}),
        tasks: tasksRef,
        store: parts.svc.store,
        threads: {
          listRows: () => listThreadRows({ cached: false }),
          events: async (a) => {
            const rows: unknown = await bb.sdk.threads.events.list({
              threadId: a.threadId,
              types: ["client/turn/requested"],
              limit: threadEventLimit(a.limit),
              order: a.order,
              ...(a.beforeSeq !== undefined ? { beforeSeq: String(a.beforeSeq) } : {}),
            });
            return Array.isArray(rows) ? (rows as MessageEvent[]) : [];
          },
        },
        config: async () => {
          if (startupFailure) {
            const { err } = startupFailure;
            startupFailure = null;
            throw err;
          }
          const v = await vizierNow();
          return { enabled: v.vizierMessageCards === true, titlePrefix: v.vizierTitlePrefix || DEFAULT_TITLE_PREFIX, project: v.vizierCardProject || "" };
        },
        log: { info: (m) => bb.log.info(m), warn: (m) => bb.log.warn(m) },
      });
      while (!signal.aborted) {
        // Always ticked: a run with the setting off files nothing and only forgets the recorded switch-on time, so a
        // later turn-on is dated afresh.
        await watcher.tick();
        await sleep(watcher.pollDelayMs(VIZIER_MESSAGE_POLL_MS), signal);
      }
    },
  });
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
  // Merge recheck (bead mk-g1t73): a merge pick whose PR is still open five minutes later brings the card back.
  // Read-only `gh pr view`, no model; the bring-back is the Archive's own.
  bb.background.service("home-merge-recheck", {
    async start(signal) {
      while (!signal.aborted && !parts) await sleep(1000, signal);
      if (!parts || signal.aborted) return;
      const p = parts;
      const recheck = new MergeRecheck({
        db: p.store.db,
        ghView: ghViewReal,
        log: (m) => console.error(`home-merge-recheck: ${m}`),
        // The tasks client is set a moment after the stores; until then the read throws and the pick is retried next pass.
        taskState: (taskId) => (tasksRef ? tasksRef.taskState(taskId) : Promise.reject(new Error("tasks unavailable"))),
        bringBack: async (taskId, decisionId, pr) => {
          const cur = p.svc.currentGeneration(taskId);
          const recipient = cur && cur.thread ? await resolveWakeTarget(taskId, String(cur.thread), true) : null;
          const r = p.dele.bringBack(taskId, decisionId, recipient, {}, [
            `Citadel brought the decision ${decisionId} back on its own: mk picked a merge, and ${pr.url} was still not merged five minutes later.`,
            `The card is open again, so mk will be asked afresh. If the PR merges first, tell mk on the card. If you have already received mk's answer on the new card, that answer stands.`,
          ]);
          if (r.ok && !r.replay) {
            p.caches.invalidate();
            bb.realtime.publish("home-queue-changed", {});
          }
          return r.ok && !r.replay;
        },
      });
      await recheck.run(signal, sleep);
    },
  });
  // Conversation mirror: the selected card every 15 s, open cards every 60 s. Failures only mark "may be stale".
  let commentPoller: CommentPoller | null = null;
  bb.background.service("home-card-comments", {
    async start(signal) {
      while (!signal.aborted && !(parts && tasksRef)) await sleep(1000, signal);
      if (!parts || !tasksRef || signal.aborted) return;
      const p = parts;
      const tasks = tasksRef;
      const poller = (commentPoller = new CommentPoller({
        store: p.svc.store,
        listComments: (id) => tasks.listComments(id),
        openTaskIds: () => openTaskIds(p.svc),
        onChanged: () => bb.realtime.publish("home-queue-changed", {}),
        log: (m) => console.error(`home-card-comments: ${m}`),
      }));
      await poller.run(signal, sleep);
    },
  });
  // Owner thread, else the one live thread with the owner's title (its successor), else the vizier, else nobody.
  const resolveWakeTarget = async (_taskId: string, owner: string | null, fresh = false): Promise<string | null> => {
    const live = (r: ThreadRow) => r.archivedAt == null && r.deletedAt == null;
    try {
      // A wake the host refused as "archived" lists fresh: a cached row may still call the thread live.
      const rows = await listThreadRows({ cached: !fresh });
      const mine = owner ? rows.find((r) => r.id === owner) : undefined;
      if (mine && live(mine)) return mine.id;
      if (mine?.title) {
        const succ = rows.filter((r) => live(r) && r.title === mine.title);
        if (succ.length === 1) return succ[0]!.id;
      }
      const v = await parts?.dele.resolveVizier({ adopt: false });
      return v && v.ok ? v.id : null;
    } catch {
      return null;
    }
  };
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
  const learnDir = join(dirname(fileURLToPath(import.meta.url)), "learn");
  // Thread titles are a read-only peek, so they never block a read: the last known titles answer at once and one
  // background refresh (never two at a time) updates them for the next read. Before the first refresh lands the card
  // shows the thread id (bead mk-yaq27: listing every thread took 1.2-1.4 s and held the whole inbox read).
  let titleMap = new Map<string, string>();
  let titleAt = 0;
  let titleRefresh: Promise<void> | null = null;
  const threadTitles = async (): Promise<Map<string, string>> => {
    if (!titleRefresh && Date.now() - titleAt >= 3000) {
      titleRefresh = listThreadRows({ cached: true })
        .then((rows) => {
          titleMap = new Map(rows.filter((t) => t.title).map((t) => [t.id, t.title as string]));
          titleAt = Date.now();
        })
        .catch(() => {}) // show the last known titles, or ids
        .finally(() => {
          titleRefresh = null;
        });
    }
    return titleMap;
  };
  const SERVE_READ_TTL_MS = 20_000;
  let serveRead: { at: number; projects: unknown; projectsError: string | undefined; build: unknown } | undefined;
  const handlers = {
    async evidenceImage(i: { sha256: string }) {
      return readEvidence(i.sha256);
    },
    async listAsks(_: null) {
      const p = need();
      const titles = await threadTitles();
      const rows = p.svc.owed().map((d) => ({
        id: d.id,
        project: d.project,
        thread: d.thread,
        key: p.svc.cardKey(d.task_id as string | null),
        held: p.svc.holdOf(d.task_id as string | null),
        later: p.svc.laterOf(d.task_id as string | null),
        // A card filed the v2 way has no subject of its own; its title is what the phone list and detail show.
        subject: d.subject || p.svc.cardTitle(d.task_id as string | null) || "",
        thread_title: titles.get(d.thread) ?? null,
        asker: d.asker,
        filed_at: d.filed_at,
        revision: d.revision,
        task_id: d.task_id ?? null,
        ...owedAsk(d.body_json),
        mentions: p.store.mentions(d.id).length,
      }));
      let machineOwners: unknown = {};
      try {
        machineOwners = JSON.parse(p.store.setting("machineOwners") ?? "{}");
      } catch {
        /* unreadable: show none */
      }
      // A read-only peek: the panel shows the vizier thread but never adopts (adoption comes with its catch-up notice).
      const resolved = await p.dele.resolveVizier({ adopt: false });
      const settings = p.dele.settings();
      // When the resolver refuses, show no vizier thread rather than a stored id that may be archived or unknown.
      settings.vizierThreadId = resolved.ok ? resolved.id : undefined;
      return {
        owed: rows,
        runbook: p.asks.runbook(),
        ...p.asks.lists(),
        undeliverable: p.svc.undeliverable(),
        failures: p.svc.failures(),
        uncertain: p.svc.wakes().filter((o) => o.state === "uncertain"),
        approvals: p.store.liveApprovals(),
        delegation: { settings, suspended: p.dele.suspendedNow() },
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
      const q = buildQueue(p.svc, p.asks, i.thread === undefined ? {} : { thread: i.thread });
      // A Blocked row names its owner thread by title, as an Asks card does (bead mk-4mhv2).
      const titles = q.rows.some((r) => r.thread) ? await threadTitles() : new Map<string, string>();
      return { ...q, rows: q.rows.map((r) => ({ ...r, thread_title: r.thread ? titles.get(r.thread) ?? null : null })), serve_projects: serveProjects, status: queueRef?.status() ?? null };
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
    async conversation(i: { task_id: string }) {
      commentPoller?.select(i.task_id);
      const vizier = await parts?.dele.resolveVizier({ adopt: false }).then((v) => (v.ok ? v.id : null), () => null);
      return conversationView(need().svc, i.task_id, vizier ?? null);
    },
    async conversationUnread(_: null) {
      const s = need().svc;
      return { unread: unreadCounts(s.store, openTaskIds(s)) };
    },
    async readDoc(i: { path: string; sha256?: string }) {
      return readDoc(i.path, { sha256: i.sha256 });
    },
    async openTarget(i: { path: string }) {
      const r = resolveOpenTarget(i.path);
      if (!r.ok) return r;
      const cfg = (await bb.sdk.system.config()) as { primaryHostId?: string | null };
      if (!cfg.primaryHostId) return { ok: false as const, error: "bb has no primary host to open the file on" };
      return { ok: true as const, target: { kind: "host" as const, hostId: cfg.primaryHostId, path: r.path } };
    },
    async markConversationSeen(i: { task_id: string; through: string }) {
      need().svc.store.markConversationSeen(i.task_id, i.through);
      return { ok: true as const };
    },
    async later(i: { ref: string }) {
      const r = need().svc.later(i.ref, MK);
      if (r.ok && !r.was) bb.realtime.publish("home-queue-changed", {});
      return r;
    },
    async unlater(i: { ref: string }) {
      const r = need().svc.unlater(i.ref);
      if (r.ok && r.was) bb.realtime.publish("home-queue-changed", {});
      return r;
    },
    async pick(i: { decision_id: string; option_id: string; revision: string; pick_id: string; reason?: string; images?: { mime: string; data_base64: string }[]; answers?: Record<string, string>; surface?: "home" | "overlay" }) {
      return need().svc.pick(i.decision_id, i.option_id, i.revision, i.pick_id, MK, i.surface ?? "home", i.reason, undefined, i.images, i.answers);
    },
    async note(i: { task_id?: string; decision_id?: string; text: string; note_id: string; images?: { mime: string; data_base64: string }[] }) {
      // Comment first: the card comment is posted before any wake obligation exists. If the tasks write fails nothing is woken.
      const r = need().svc.note({ ...(i.task_id !== undefined ? { task_id: i.task_id } : {}), ...(i.decision_id !== undefined ? { decision_id: i.decision_id } : {}) }, i.text, i.note_id, { deferWake: true, ...(i.images ? { images: i.images } : {}) });
      if (!r.ok) return r;
      if (r.task_id !== null) {
        if (!tasksRef) return { ok: false as const, status: 503, error: "tasks unavailable; nothing was sent" };
        try {
          await postNoteOnce(tasksRef, r.task_id, i.note_id, r.body);
        } catch {
          return { ok: false as const, status: 502, error: "the card comment could not be posted, so nobody was woken; send it again" };
        }
        void commentPoller?.readNow(r.task_id);
      }
      const done = r.commit();
      bb.realtime.publish("home-queue-changed", {});
      return { ok: true as const, replay: done.replay, woke: done.woke };
    },
    async ideaProjects(_: null) {
      const d = ideaDesk();
      if (!d) return { ok: false as const, status: 503, error: "tasks unavailable" };
      try {
        return { ok: true as const, projects: await d.projects() };
      } catch (e) {
        return { ok: false as const, status: 502, error: `tasks unavailable: ${e instanceof Error ? e.message : String(e)}` };
      }
    },
    async ideas(_: null) {
      const d = ideaDesk();
      if (!d) return { ok: false as const, status: 503, error: "tasks unavailable" };
      try {
        return { ok: true as const, ...(await d.digest(isoWeek(Date.now()))) };
      } catch (e) {
        return { ok: false as const, status: 502, error: `tasks unavailable: ${e instanceof Error ? e.message : String(e)}` };
      }
    },
    async archive(i: ArchiveQuery) {
      return { ok: true as const, ...archiveView(need().svc.store.db, i) };
    },
    async learn(_: null) {
      return { ok: true as const, ...learnView(need().svc.store, learnDir) };
    },
    async learnSet(i: LearnSet) {
      const r = learnSet(need().svc.store, learnDir, i);
      if (r.ok) bb.realtime.publish("home-learn-changed", {});
      return r;
    },
    async fileIdea(i: { project_id: string; text: string; idea_id: string }) {
      const d = ideaDesk();
      if (!d) return { ok: false as const, status: 503, error: "tasks unavailable; nothing was filed" };
      const r = await d.file(i);
      if (r.ok) bb.realtime.publish("home-queue-changed", {});
      return r;
    },
    async actIdea(i: { task_id: string; action: "pursue" | "park" | "drop" }) {
      const d = ideaDesk();
      if (!d) return { ok: false as const, status: 503, error: "tasks unavailable; nothing was changed" };
      const r = await d.act(i);
      if (r.ok) bb.realtime.publish("home-queue-changed", {});
      return r;
    },
    async clearIdeaDigest(_: null) {
      const d = ideaDesk();
      if (!d) return { ok: false as const, status: 503, error: "tasks unavailable" };
      d.clearDigest(isoWeek(Date.now()));
      bb.realtime.publish("home-queue-changed", {});
      return { ok: true as const };
    },
    async dismiss(i: { decision_id: string; obligation_id: string }) {
      return need().svc.dismiss(i.decision_id, i.obligation_id);
    },
    async override(i: { decision_id: string }) {
      const r = need().dele.override(i.decision_id, {});
      if (r.ok) need().caches.invalidate();
      return r;
    },
    /** Archive tab: reopen a decided card as a new generation. The notice goes to the asking thread, or the vizier when it is gone. */
    async bringBack(i: { task_id: string; decision_id: string }) {
      const p = need();
      const cur = p.svc.currentGeneration(i.task_id);
      const recipient = cur && cur.thread ? await resolveWakeTarget(i.task_id, String(cur.thread), true) : cur ? await resolveWakeTarget(i.task_id, null, true) : null;
      const r = p.dele.bringBack(i.task_id, i.decision_id, recipient, {});
      if (r.ok) p.caches.invalidate();
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
      const o0 = performance.now();
      const open = p.svc.isOwed(i.decision_id);
      timingLog(`rpc-timing setViewing owed=${Math.round(performance.now() - o0)}ms`);
      if (!open) return { ok: false, error: "not an open ask" };
      p.store.setSetting("viewing", JSON.stringify({ decision_id: i.decision_id, at: p.svc.time() }));
      return { ok: true };
    },
    /** The panel's own timing of a card click, written to the plugin log so it can be read after mk's click (bead mk-yaq27). */
    async clickTiming(i: { kind: string; ms: number; width: number }) {
      bb.log.info(`click-timing ${i.kind} ${i.ms}ms width=${i.width}`);
      return { ok: true };
    },
    /** The state of the card with this key, for the link /plugins/autarch/home/<KEY>; reads one row. */
    async cardState(i: { key: string }) {
      return cardStateOf(need().store.db, i.key);
    },
    /** The runner's status and last result, and any waiting request. Reads two files; runs nothing. */
    async updateInfo() {
      const p = need();
      return updateInfo(updateDir, p.store.setting("update_request"), updateEnv(p.svc.time()));
    },
    /** A click only records {sha, clicked_at} in this plugin's store; the runner outside the plugin acts on it. */
    async requestUpdate(i: { sha: string }) {
      const p = need();
      const r = checkRequest(updateInfo(updateDir, p.store.setting("update_request"), updateEnv(p.svc.time())), i.sha, p.svc.time());
      if (!r.ok) return r;
      p.store.setSetting("update_request", JSON.stringify(r.request));
      touchTrigger(updateDir);
      return { ok: true, request: r.request };
    },
    async markSeen(i: { item: string }) {
      need().dele.markSeen(MK, i.item);
      bb.realtime.publish("home-queue-changed", {}); // acknowledging a notice changes the waiting count on every open surface
      return { ok: true };
    },
    async markAllSeen(i: { ids: string[] }) {
      const marked = need().catchup.markAllSeen(i.ids);
      if (marked.length > 0) bb.realtime.publish("home-queue-changed", {});
      return { marked };
    },
    async catchup(_: null) {
      return { items: need().catchup.items() };
    },
    async waiting(_: null) {
      const p = need();
      return waitingNow(p.svc, p.dele, p.catchup);
    },
    async stats(i: { days: number }) {
      const p = need();
      return p.svc.stats(new Date(Date.parse(p.svc.time()) - i.days * 86_400_000).toISOString());
    },
    async health(_: null) {
      // The serve reads change rarely and cost a round trip each, so a poll reuses them for a short while (bead mk-0zih8).
      const now = performance.now();
      if (!serveRead || now - serveRead.at > SERVE_READ_TTL_MS) {
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
        serveRead = { at: now, projects, projectsError, build };
      }
      const { projects, projectsError, build } = serveRead;
      return { ready: handle.ready(), error: handle.error(), source_sha256: pluginSource(), build, projects: projects ?? null, projects_error: projectsError ?? null };
    },
  };
  // bb validates an RPC result as a JSON value, and an `undefined` member fails the whole call
  // (found by the real-bb run: listAsks failed on an ask with no machine block). Round-trip
  // every result through JSON so absent members are dropped, as they would be on the wire.
  // Server-side time of each RPC (bead mk-0zih8): the handler's own wall time and the JSON round trip, next to the longest event-loop
  // stall seen in the last 10 s. With the panel's round trip (click-timing) this separates handler time from waiting in the
  // single bb process from the transport. Display-only: written to the plugin log, nothing reads it back.
  const lagWindow: { at: number; ms: number }[] = [];
  let lastTick = performance.now();
  // Time this plugin's own RPC handlers took in the current 10 s report window (bead mk-o2ur7.1).
  let busyMs = 0;
  let reportAt = performance.now();
  let reportOver50 = 0;
  let reportMax = 0;
  const lagTimer = setInterval(() => {
    const t = performance.now();
    const late = t - lastTick - 100;
    lastTick = t;
    if (late > 5) lagWindow.push({ at: t, ms: Math.round(late) });
    while (lagWindow.length > 0 && t - lagWindow[0]!.at > 10_000) lagWindow.shift();
    if (late > 50) reportOver50 += 1;
    if (late > reportMax) reportMax = Math.round(late);
    // One line per 10 s, with or without RPCs: the stall next to this plugin's own handler time. A stall in a window where
    // the plugin did no work comes from the bb process or another plugin. Display-only, like rpc-timing. Off unless AUTARCH_LOOP_LAG_LOG=1 (about 8,600 lines a day).
    if (t - reportAt >= 10_000) {
      if (process.env.AUTARCH_LOOP_LAG_LOG === "1") timingLog(`loop-lag10s max=${reportMax}ms over50=${reportOver50} autarch_busy=${Math.round(busyMs)}ms`);
      reportAt = t;
      busyMs = 0;
      reportOver50 = 0;
      reportMax = 0;
    }
  }, 100);
  lagTimer.unref?.();
  bb.onDispose(() => clearInterval(lagTimer));
  const loopStall = (): number => {
    const now = performance.now();
    return lagWindow.reduce((m, x) => (now - x.at > 10_000 ? m : Math.max(m, x.ms)), 0);
  };
  // A failing diagnostic log must never change an RPC's outcome.
  const timingLog = (msg: string): void => {
    try {
      bb.log.info(msg);
    } catch {
      /* display-only */
    }
  };
  const jsonSafe = <F extends (i: never) => Promise<unknown>>(name: string, f: F): F =>
    (async (i: never) => {
      const t0 = performance.now();
      const r = await f(i);
      const t1 = performance.now();
      const out = r === undefined ? null : JSON.parse(JSON.stringify(r));
      const t2 = performance.now();
      busyMs += t2 - t0;
      if (name !== "clickTiming") {
        timingLog(`rpc-timing ${name} handler=${Math.round(t1 - t0)}ms json=${Math.round(t2 - t1)}ms stall10s=${loopStall()}ms`);
      }
      return out;
    }) as F;
  const safe = Object.fromEntries(Object.entries(handlers).map(([k, f]) => [k, jsonSafe(k, f as never)])) as unknown as typeof handlers;
  return { handlers: safe };
}

export default async function plugin(bb: BbPluginApi) {
  bb.log.info("loaded");
  const handle = wireStore(bb);

  // Declarative settings — rendered in BB's settings UI and editable with
  // `bb plugin config autarch`. Add `secret: true` for values like API keys.
  // Settings are read once per load, except the three vizier message settings, which the watcher reads on every tick.
  // The Home surfaces: RPC handlers, the `bb home` CLI, wakes, feed injection, serve link.
  const homeSettings = bb.settings.define({
    serveAddr: { type: "string", label: "autarch serve address", default: "127.0.0.1:8110" },
    serveTokenFile: { type: "string", label: "autarch serve token file", default: "" },
    serveProjectDirs: { type: "string", label: "Project scan roots (comma separated; empty uses Bigend's)", default: "" },
    autarchBin: { type: "string", label: "autarch binary", default: "" },
    vizierMessageCards: { type: "boolean", label: "File a card for each message the operator types in the vizier thread", default: false },
    vizierTitlePrefix: { type: "string", label: "Vizier thread title prefix (the one pinned, unarchived thread that starts with it)", default: "Masaq' | vizier" },
    vizierCardProject: { type: "string", label: "Tasks project (id, prefix or name) that receives vizier message cards", default: "" },
  });
  const hs = await homeSettings.get();
  const vizierSettings: VizierSettingsSource = async () => {
    const v = await homeSettings.get();
    return {
      vizierMessageCards: v.vizierMessageCards === true,
      vizierTitlePrefix: typeof v.vizierTitlePrefix === "string" ? v.vizierTitlePrefix : "",
      vizierCardProject: typeof v.vizierCardProject === "string" ? v.vizierCardProject : "",
    };
  };
  const home = wireHome(bb, handle, {
    serveAddr: hs.serveAddr || "127.0.0.1:8110",
    serveTokenFile: hs.serveTokenFile || join(homedir(), ".autarch", "serve.token"),
    serveProjectDirs: (hs.serveProjectDirs || "").split(",").map((d: string) => d.trim()).filter(Boolean),
    autarchBin: hs.autarchBin || process.env.AUTARCH_BIN || "autarch",
    vizierMessageCards: hs.vizierMessageCards === true,
    vizierTitlePrefix: hs.vizierTitlePrefix || "",
    vizierCardProject: hs.vizierCardProject || "",
  }, { vizierSettings });

  bb.rpc.register(rpcContract, {
    ...home.handlers,
  });

  // Cleanup on reload/disable/shutdown; hooks run LIFO. The sanctioned place
  // to clear timers and close connections.
  bb.onDispose(() => {
    bb.log.info("disposed");
  });
}
