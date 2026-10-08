// bb-plugin-autarch — a BB plugin frontend entry.
//
// Compiled by `bb plugin build` into dist/app.js + dist/app.css. React and
// @get-bb/plugin-sdk/app are provided by the BB app at load time (never bundled),
// so this file must be loaded by BB, not imported directly.
//
// The components under components/ui/ are YOURS: vendored source (shadcn
// model), edit freely. Add more from the BB registry with
// `npx shadcn add @bb/<name>` (see components.json) — dropdowns, tables,
// the full shadcn set, version-matched to this BB install. Run
// `npm install` once before `bb plugin build`.
import { useCallback, useEffect, useMemo, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { definePluginApp, useBbNavigate, useRealtime, useRpc } from "@get-bb/plugin-sdk/app";
import type { rpcContract, Todo } from "./server";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Icon } from "@/components/ui/icon";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { OVERLAY_PANEL_ID, OVERLAY_PATH, OverlayPanel } from "./ui/overlay.js";
import { AsksPanel, PickController, pickOutcome } from "./ui/asks.js";
import { ConversationProvider, type ConversationApi, type ConversationData } from "./ui/conversation.js";
import { YourMovePanel, type MoveHandlers } from "./ui/yourmove.js";
import type { MoveViewGroups } from "./moveview.js";
import type { AsksData } from "./ui/asks.js";
import { CatchupPanel, SeenTracker, snapshotIds } from "./ui/catchup.js";
import type { CatchupEntry } from "./ui/catchup.js";
import { MapPlaceholder } from "./ui/map-placeholder.js";
import type { Lens } from "./ui/map-placeholder.js";
import { BlocksPanel, QueueRefresher, type RootRunHooks } from "./ui/blocks.js";
import type { QueueView } from "./ui/blocks.js";
import { BindingsPanel, SettingsPanel } from "./ui/settings.js";
import { keyAction, layoutStack, stackReducer, StackView } from "./ui/stack.js";
import type { Panel, StackState } from "./ui/stack.js";
import { HOME_SOURCE } from "./ui/identity.js";
import { ThreadPanel, VizierPanel } from "./ui/vizier.js";

/** The todo list, kept current by the server's "todos-changed" signal. */
function useTodos() {
  const rpc = useRpc<typeof rpcContract>();
  const [todos, setTodos] = useState<Todo[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const report = useCallback((cause: unknown) => {
    setError(cause instanceof Error ? cause.message : String(cause));
  }, []);
  const refetch = useCallback(() => {
    rpc.call("todos_list").then((result) => {
      setTodos(result.todos);
      setError(null);
    }, report);
  }, [rpc, report]);
  useEffect(() => {
    refetch();
  }, [refetch]);
  // server.ts publishes after every write — from this page or another window —
  // so the list never goes stale.
  useRealtime("todos-changed", refetch);
  return { rpc, todos, error, report, refetch };
}

function TodoRow({
  todo,
  onToggle,
  onRemove,
}: {
  todo: Todo;
  onToggle: (done: boolean) => void;
  onRemove: () => void;
}) {
  return (
    <li className="flex items-center gap-3 py-2.5 text-sm">
      <Checkbox
        checked={todo.done}
        onCheckedChange={(checked) => onToggle(checked === true)}
        aria-label={`Mark "${todo.title}" ${todo.done ? "not done" : "done"}`}
      />
      <span
        className={cn(
          "min-w-0 flex-1 truncate",
          todo.done && "text-muted-foreground line-through",
        )}
      >
        {todo.title}
      </span>
      <span className="hidden font-mono text-xs text-muted-foreground sm:inline">
        {todo.id}
      </span>
      <Button
        variant="ghost"
        size="icon"
        className="size-7 text-muted-foreground hover:text-foreground"
        aria-label={`Remove "${todo.title}"`}
        onClick={onRemove}
      >
        <Icon name="Trash2" className="size-4" />
      </Button>
    </li>
  );
}

/** The dashed box BB's own list pages use for loading and empty states. */
function EmptyState({ children }: { children: ReactNode }) {
  return (
    <div
      role="status"
      className="rounded-lg border border-dashed border-border px-4 py-6 text-center text-sm text-muted-foreground"
    >
      {children}
    </div>
  );
}

// Tailwind classes compile against the host theme's live CSS variables —
// derive colors from the theme tokens, never hardcoded grays. The frame
// (scrolling page, centered column) matches BB's own nav-panel pages.
function TodosPage() {
  const { rpc, todos, error, report, refetch } = useTodos();
  const [title, setTitle] = useState("");
  const [pending, setPending] = useState(false);
  const add = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const next = title.trim();
    if (next === "" || pending) return;
    setPending(true);
    try {
      await rpc.call("todos_add", { title: next });
      setTitle("");
      refetch();
    } catch (cause) {
      report(cause);
    } finally {
      setPending(false);
    }
  };
  const doneCount = todos?.filter((todo) => todo.done).length ?? 0;
  return (
    <div className="h-full min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto box-border w-full max-w-3xl px-4 pb-4 pt-3 md:px-5 md:pt-4">
        <p className="text-sm text-muted-foreground">
          A small example list, managed on this page only. Agents reach Home with{" "}
          <code>bb home</code>.
        </p>
        <form onSubmit={add} className="mt-4 flex items-center gap-2">
          <Input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="What needs doing?"
            aria-label="New todo"
          />
          <Button type="submit" disabled={pending || title.trim() === ""}>
            <Icon name="Plus" className="size-4" />
            Add
          </Button>
        </form>
        {error === null ? null : (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {error}
          </p>
        )}
        <div className="mt-4">
          {todos === null ? (
            <EmptyState>Loading todos…</EmptyState>
          ) : todos.length === 0 ? (
            <EmptyState>
              Nothing to do. Add one above.
            </EmptyState>
          ) : (
            <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card px-4">
              {todos.map((todo) => (
                <TodoRow
                  key={todo.id}
                  todo={todo}
                  onToggle={(done) => {
                    rpc
                      .call("todos_set_done", { id: todo.id, done })
                      .then(refetch, report);
                  }}
                  onRemove={() => {
                    rpc
                      .call("todos_remove", { id: todo.id })
                      .then(refetch, report);
                  }}
                />
              ))}
            </ul>
          )}
        </div>
        {todos !== null && todos.length > 0 ? (
          <p className="mt-2 text-xs text-muted-foreground">
            {doneCount} of {todos.length} done
          </p>
        ) : null}
      </div>
    </div>
  );
}

const POLL_MS = 10_000;

/** The queue refetches on the server's "home-queue-changed" signal and, as a fallback, every POLL_MS. */
function useQueue(thread?: string) {
  const rpc = useRpc<typeof rpcContract>();
  const [queue, setQueue] = useState<QueueView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const refetch = useCallback(() => {
    rpc.call("queue", thread === undefined ? {} : { thread }).then(
      (q) => {
        setQueue(q as unknown as QueueView);
        setError(null);
      },
      (cause) => setError(cause instanceof Error ? cause.message : String(cause)),
    );
  }, [rpc, thread]);
  const refresher = useMemo(() => new QueueRefresher(refetch, POLL_MS), [refetch]);
  useRealtime("home-queue-changed", refresher.onEvent);
  useEffect(() => {
    refetch();
    return refresher.start();
  }, [refetch, refresher]);
  return { rpc, queue, error, refetch };
}

/** Root-run views, loaded on demand: one tasks read and one file read per click, never on the refresh poll. */
function useRootRuns(rpc: ReturnType<typeof useRpc<typeof rpcContract>>): RootRunHooks {
  const [views, setViews] = useState<RootRunHooks["views"]>({});
  const load = useCallback(
    (taskId: string) => {
      rpc.call("rootRun", { task_id: taskId }).then(
        (r) => {
          const res = r as unknown as { ok: true; view: never } | { ok: false; error: string };
          setViews((v) => ({ ...v, [taskId]: res.ok ? res.view : { error: res.error } }));
        },
        (cause) => setViews((v) => ({ ...v, [taskId]: { error: cause instanceof Error ? cause.message : String(cause) } })),
      );
    },
    [rpc],
  );
  return { views, load };
}

/** Conversation on the card: unread counts (polled), the conversation read, and the read mark. All display; no write but the read mark. */
function useConversationApi(rpc: ReturnType<typeof useRpc<typeof rpcContract>>): ConversationApi {
  const [unread, setUnread] = useState<Record<string, number>>({});
  const refetch = useCallback(() => {
    rpc.call("conversationUnread").then((r) => setUnread((r as { unread: Record<string, number> }).unread ?? {}), () => {});
  }, [rpc]);
  useEffect(() => {
    refetch();
    const t = setInterval(refetch, POLL_MS);
    return () => clearInterval(t);
  }, [refetch]);
  return useMemo(
    () => ({
      unread,
      load: (task_id) => rpc.call("conversation", { task_id }).then((d) => d as unknown as ConversationData),
      markSeen: (task_id, through) => void rpc.call("markConversationSeen", { task_id, through }).then(refetch, () => {}),
    }),
    [rpc, unread, refetch],
  );
}

/** The same panel, opened beside a thread by the thread-panel action: this thread's cards are pinned. */
function BlocksThreadPanel({ threadId }: { threadId: string }) {
  const { rpc, queue, error, refetch } = useQueue(threadId);
  const noteOnly = useMoves(rpc, refetch, false);
  const nav = useBbNavigate();
  const picks = useMemo(() => new PickController(() => crypto.randomUUID()), []);
  const rootRun = useRootRuns(rpc);
  const conversation = useConversationApi(rpc);
  if (queue === null) return <EmptyState>{error ?? "Loading…"}</EmptyState>;
  return (
    <ConversationProvider value={conversation}>
    <BlocksPanel
      rootRun={rootRun}
      onBind={(b) => void rpc.call("setBinding", b).then(refetch, () => {})}
      data={queue}
      nowMs={Date.now()}
      thread={threadId}
      onNote={(decision_id, text) => noteOnly.note({ decision_id }, text)}
      onOpen={(t) => nav.toThread(t)}
      onPick={(decision_id, option_id, revision, reason) => pickOutcome(picks.send((req) => rpc.call("pick", req) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}) }, refetch)).finally(refetch)}
    />
    </ConversationProvider>
  );
}

/** The server publishes no realtime channel for Home, so the page polls and refetches after each action. */
function useHomeData() {
  const rpc = useRpc<typeof rpcContract>();
  const [asks, setAsks] = useState<AsksData | null>(null);
  const [catchup, setCatchup] = useState<CatchupEntry[]>([]);
  const [health, setHealth] = useState<{ ready: boolean } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const refetch = useCallback(() => {
    Promise.all([rpc.call("listAsks"), rpc.call("catchup"), rpc.call("health")]).then(
      ([a, c, h]) => {
        setAsks(a as unknown as AsksData);
        setCatchup((c as { items: CatchupEntry[] }).items);
        setHealth(h as { ready: boolean });
        setError(null);
      },
      (cause) => setError(cause instanceof Error ? cause.message : String(cause)),
    );
  }, [rpc]);
  useEffect(() => {
    refetch();
    const t = setInterval(refetch, POLL_MS);
    return () => clearInterval(t);
  }, [refetch]);
  return { rpc, asks, catchup, health, error, refetch };
}

type RpcResult = { ok?: boolean; status?: number; error?: string };
/** A send becomes an outcome the card can show; a thrown error or a non-ok result is a failure with its reason. */
async function rpcOutcome(send: Promise<unknown>): Promise<{ ok: boolean; error?: string }> {
  try {
    const r = (await send) as RpcResult | null;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? `failed${r?.status ? ` (${r.status})` : ""}` };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

/** Your move: the grouped read model, refetched on the queue signal and on the poll. */
function useMoves(rpc: ReturnType<typeof useRpc<typeof rpcContract>>, onChanged: () => void, load = true) {
  const [moves, setMoves] = useState<MoveViewGroups | null>(null);
  const refetch = useCallback(() => {
    rpc.call("moves").then((m) => setMoves(m as unknown as MoveViewGroups), () => {});
  }, [rpc]);
  useRealtime("home-queue-changed", () => {
    if (load) refetch();
  });
  useEffect(() => {
    if (!load) return;
    refetch();
    const t = setInterval(refetch, POLL_MS);
    return () => clearInterval(t);
  }, [refetch, load]);
  // A note id is stable per (target, text) until it is accepted, so a retry cannot post the comment twice.
  const noteIds = useMemo(() => new Map<string, string>(), []);
  const note = useCallback(
    async (target: { task_id: string } | { decision_id: string }, text: string) => {
      const key = `${JSON.stringify(target)}|${text}`;
      const note_id = noteIds.get(key) ?? crypto.randomUUID().slice(0, 32);
      noteIds.set(key, note_id);
      const r = await rpcOutcome(rpc.call("note", { ...target, text, note_id }));
      if (r.ok) {
        noteIds.delete(key);
        onChanged();
      }
      return r;
    },
    [rpc, noteIds, onChanged],
  );
  const handlers: MoveHandlers = useMemo(
    () => ({
      onCheck: (m) => rpcOutcome(rpc.call("checkMove", { task_id: m.task_id, generation: m.generation })).finally(refetch),
      onClaim: (m) => rpcOutcome(rpc.call("claimMove", { task_id: m.task_id, generation: m.generation })).finally(refetch),
      onSkip: (m) => rpcOutcome(rpc.call("skipMove", { task_id: m.task_id, generation: m.generation })).finally(refetch),
      onNote: (m, text) => note({ task_id: m.task_id }, text),
    }),
    [rpc, refetch, note],
  );
  return { moves, handlers, refetch, note };
}

function HomePage() {
  const { rpc, asks, catchup, error, refetch } = useHomeData();
  const nav = useBbNavigate();
  const blocks = useQueue();
  const rootRuns = useRootRuns(rpc);
  const yourMove = useMoves(rpc, () => {});
  const conversation = useConversationApi(rpc);
  const [stack, setStack] = useState<StackState>({ panels: [{ id: "queue", kind: "decision", title: "Queue" }], width: "third" });
  // Q: one ranked queue is the default (mk picked A on AUTA-17); the old Asks / Blocking / Catch-up tabs stay behind this setting.
  const [classic, setClassic] = useState(() => { try { return localStorage.getItem("home.classicTabs") === "1"; } catch { return false; } });
  const toggleClassic = () => setClassic((c) => { const n = !c; try { localStorage.setItem("home.classicTabs", n ? "1" : "0"); } catch { /* storage unavailable: the choice lasts this session */ } return n; });
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [lens, setLens] = useState<Lens>("attention");
  const picks = useMemo(() => new PickController(() => crypto.randomUUID()), []);
  const tracker = useMemo(() => new SeenTracker((item) => void rpc.call("markSeen", { item }).then(refetch, () => {})), [rpc, refetch]);
  const onVisibility = useCallback((item: string, visible: boolean) => tracker.setVisible(item, visible), [tracker]);
  const refetchAll = () => {
    refetch();
    blocks.refetch();
  };
  /** Open a thread and this plugin's blocks panel beside it; the host declines (false) where there is no side panel. */
  const openBeside = (thread: string) => {
    nav.toThread(thread);
    nav.openThreadPanel({ actionId: "home-blocks" });
  };
  const dispatch = (a: Parameters<typeof stackReducer>[1]) => setStack((s) => stackReducer(s, a));
  const push = (panel: Panel) => dispatch({ type: "push", panel });

  useEffect(() => {
    const sync = () => tracker.setActive(document.visibilityState === "visible" && document.hasFocus());
    sync();
    document.addEventListener("visibilitychange", sync);
    window.addEventListener("focus", sync);
    window.addEventListener("blur", sync);
    return () => {
      document.removeEventListener("visibilitychange", sync);
      window.removeEventListener("focus", sync);
      window.removeEventListener("blur", sync);
      tracker.setActive(false);
    };
  }, [tracker]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const editable = !!el && (el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName));
      const intent = keyAction(e.key, { editable });
      if (!intent) return;
      if (intent.type === "close") dispatch({ type: "close" });
      else if (intent.type === "width") dispatch(intent);
      else if (intent.type === "lens") setLens((["attention", "allocation", "dependencies", "neglect"] as const)[intent.lens - 1]!);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const toggle = (item: string, members?: string[]) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(item)) {
        next.delete(item);
        tracker.collapse(item);
      } else {
        next.add(item);
        tracker.expand(item, members);
      }
      return next;
    });
  };

  const render = (panel: Panel): ReactNode => {
    switch (panel.id) {
      case "queue":
        return (
          <>
            {([
              ["Needs you now", { id: "asks", kind: "decision", title: "Asks" }],
              ["Blocked on others", { id: "blocks", kind: "decision", title: "Blocking", hideOwed: true }],
              ["Since you left", { id: "catchup", kind: "catchup", title: "Catch-up" }],
            ] as const).map(([heading, p]) => (
              <section key={p.id} aria-label={heading} className="border-b border-border">
                <h2 className="px-4 pt-3 text-xs font-semibold uppercase text-muted-foreground">{heading}</h2>
                {render(p)}
              </section>
            ))}
          </>
        );
      case "asks":
        return asks === null ? (
          <EmptyState>{error ?? "Loading asks…"}</EmptyState>
        ) : (
          <>
          {yourMove.moves ? <YourMovePanel data={yourMove.moves} handlers={yourMove.handlers} /> : null}
          <AsksPanel
            data={asks}
            onNote={(decision_id, text) => yourMove.note({ decision_id }, text)}
            onDismiss={(decision_id, obligation_id) => void rpc.call("dismiss", { decision_id, obligation_id }).then(refetch, () => {})}
            onOpen={(thread) => push({ id: `thread:${thread}`, kind: "thread", title: thread, ref: thread })}
            onRevoke={(approval_id) => void rpc.call("revokeApproval", { approval_id }).then(refetch, () => {})}
            onPick={(decision_id, option_id, revision, reason) => {
              return pickOutcome(picks.send((req) => rpc.call("pick", req) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}) }, refetchAll)).finally(refetchAll);
            }}
          />
          </>
        );
      case "blocks":
        return blocks.queue === null ? (
          <EmptyState>{blocks.error ?? "Loading blocking cards…"}</EmptyState>
        ) : (
          <BlocksPanel
            rootRun={rootRuns}
            onBind={(b) => void rpc.call("setBinding", b).then(refetchAll, () => {})}
            data={panel.hideOwed && asks ? withoutOwed(blocks.queue, asks) : blocks.queue}
            nowMs={Date.now()}
            onNote={(decision_id, text) => yourMove.note({ decision_id }, text)}
            onOpen={openBeside}
            onPick={(decision_id, option_id, revision, reason) => {
              return pickOutcome(picks.send((req) => rpc.call("pick", req) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}) }, refetchAll)).finally(refetchAll);
            }}
          />
        );
      case "catchup":
        return (
          <CatchupPanel
            items={catchup}
            expanded={expanded}
            onToggle={toggle}
            onOverride={(decision_id) => void rpc.call("override", { decision_id }).then(refetch, () => {})}
            onVisibility={onVisibility}
            onMarkAll={() => {
              const ids = snapshotIds(catchup, expanded, new Set(catchup.filter((c) => tracker.isVisible(c.item)).map((c) => c.item)));
              if (ids.length > 0) void rpc.call("markAllSeen", { ids }).then(refetch, () => {});
            }}
          />
        );
      case "vizier":
        return <VizierPanel threadId={asks?.delegation.settings.vizierThreadId} />;
      case "settings":
        return asks === null ? null : (
          <>
            <SettingsPanel delegation={asks.delegation} machineOwners={asks.machineOwners} onSave={(v) => void rpc.call("setDelegation", v).then(refetch, () => {})} />
            {blocks.queue === null ? null : (
              <BindingsPanel
                bindings={blocks.queue.bindings}
                unbound={blocks.queue.unbound ?? []}
                serveProjects={blocks.queue.serve_projects ?? []}
                inactive={blocks.queue.inactive_projects}
                legacyCount={blocks.queue.legacy.count}
                onBind={(b) => void rpc.call("setBinding", b).then(refetchAll, () => {})}
              />
            )}
          </>
        );
      case "map":
        return <MapPlaceholder lens={lens} onLens={setLens} />;
      default:
        return panel.ref ? <ThreadPanel threadId={panel.ref} /> : null;
    }
  };

  return (
    <ConversationProvider value={conversation}>
    <div className="flex h-full min-h-0 flex-1 flex-col" data-home-source={HOME_SOURCE}>
      <nav className="flex gap-3 border-b border-border px-4 py-2 text-sm">
        {(
          [
            ["queue", "decision", "Queue"],
            ...(classic ? ([["asks", "decision", "Asks"], ["blocks", "decision", "Blocking"], ["catchup", "catchup", "Catch-up"]] as const) : []),
            ["vizier", "vizier", "Vizier"],
            ["map", "map", "Map"],
            ["settings", "settings", "Settings"],
          ] as const
        ).map(([id, kind, title]) => (
          <button key={id} type="button" className="underline-offset-2 hover:underline" onClick={() => push({ id, kind, title })}>
            {title}
          </button>
        ))}
        <button type="button" className="ml-auto text-muted-foreground" aria-pressed={classic} onClick={toggleClassic}>
          Classic tabs
        </button>
        <button type="button" className="text-muted-foreground" onClick={() => nav.toPluginPanel("example-todos")}>
          Todos
        </button>
      </nav>
      <StackView placed={layoutStack(stack)} render={render} onExpand={push} />
    </div>
    </ConversationProvider>
  );
}

/** Sidebar badge: the owed count, or "!" when serve is not ready or a machine blocker has no owner. */
/** The summoned overlay's route (`/plugins/autarch/home-overlay`); it never marks anything seen. */
function OverlayPage() {
  const { rpc, asks, catchup, error, refetch } = useHomeData();
  const nav = useBbNavigate();
  const picks = useMemo(() => new PickController(() => crypto.randomUUID()), []);
  if (asks === null) return <EmptyState>{error ?? "Loading…"}</EmptyState>;
  return (
    <OverlayPanel
      data={asks}
      catchup={catchup}
      onOpen={(thread) => nav.toThread(thread)}
      onPick={(decision_id, option_id, revision, reason) => {
        return pickOutcome(picks.send((req) => rpc.call("pick", { ...req, surface: "overlay" }) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}) }, refetch)).finally(refetch);
      }}
    />
  );
}

/** The Queue shows Asks above Blocking: drop from Blocking what Asks already owes, so one ask is never answerable twice. */
function withoutOwed(q: QueueView, asks: { owed: { id: string }[] }): QueueView {
  const ids = new Set(asks.owed.map((o) => o.id));
  return { ...q, legacy: { ...q.legacy, owed: q.legacy.owed.filter((o) => !ids.has(o.id)) } };
}

function HomeBadge() {
  const { asks, catchup, health } = useHomeData();
  const blocked = health !== null && !health.ready;
  const unowned = asks?.asks.some((a) => a.owner === null) ?? false;
  // Queue sections: owed asks plus catch-up items that still need a look (owed ones are already in the first count).
  const n = (asks?.owed.length ?? 0) + catchup.filter((c) => c.kind !== "owed" && c.kind !== "routine").length;
  if (blocked || unowned) return <span aria-label="needs attention">!</span>;
  return n > 0 ? <span>{n}</span> : null;
}

// The default export must be definePluginApp(...); BB interprets it after
// loading the bundle. navPanel adds a page to the left sidebar; register
// other UI under app.slots and composer actions, plus-menu rows, banners, or
// rich-text rules with app.composer.customize(...) (see the bb guide's
// plugins chapter).
export default definePluginApp((app) => {
  app.slots.navPanel({
    id: "autarch-home",
    title: "Home",
    icon: "House",
    path: "home",
    component: HomePage,
    experimental_sidebarAccessory: HomeBadge,
  });
  app.slots.navPanel({ id: OVERLAY_PANEL_ID, title: "Home overlay", icon: "House", path: OVERLAY_PATH, component: OverlayPage });
  app.slots.threadPanelAction({
    id: "home-blocks",
    title: "Blocking",
    icon: "House",
    component: ({ threadId }) => <BlocksThreadPanel threadId={threadId} />,
  });
  app.slots.navPanel({
    id: "example-todos",
    title: "Example todos",
    icon: "ListTodo",
    // Routed at /plugins/autarch/example-todos; the component receives the
    // remainder as `subPath` for deep links within the page.
    path: "example-todos",
    component: TodosPage,
  });
});
