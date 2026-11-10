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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { definePluginApp, useBbNavigate, useRealtime, useRpc } from "@get-bb/plugin-sdk/app";
import type { rpcContract } from "./server";
import { AsksPanel, PickController, imagesSig, pickOutcome, laterOutcome, withoutPicked, withLater } from "./ui/asks.js";
import { DocReaderHost, type DocReaderApi, type DocResultT } from "./ui/docreader.js";
import type { OtherImage } from "./ui/other.js";
import { setTimingSink, timed } from "./ui/clicktiming.js";
import { ConversationProvider, type ConversationApi, type ConversationData } from "./ui/conversation.js";
import type { AsksData } from "./ui/asks.js";
import { MobileInbox, useIsPhone } from "./ui/inbox.js";
import { ProjectFilterBar, useProjectFilter } from "./ui/projectfilter.js";
import { COUNTS_FOLLOW_FILTER, filterAsks, filterQueue, hiddenIn, projectChoices, shownCount } from "./ui/projectfilterstore.js";
import { IdeaDesk, IdeaDigest, type IdeaApi } from "./ui/ideas.js";
import type { IdeaProject, IdeaView } from "./ideas.js";
import { CatchupPanel, markedRows, markPlan, markResultText, normalizeCatchup, readableCatchup, SeenTracker, settleCatchup } from "./ui/catchup.js";
import { NoticeBanner } from "./ui/notices.js";
import { singleFlight, type Flight } from "./ui/singleflight.js";
import { rpcOutcome } from "./movehandlers.js";
import { ActionError, actionErrorText, useActionError } from "./ui/actionerror.js";
import { railBadge, railCount } from "./ui/waiting.js";
import type { Waiting } from "./waiting.js";
import type { CatchupEntry } from "./ui/catchup.js";
import { MapPlaceholder } from "./ui/map-placeholder.js";
import type { Lens } from "./ui/map-placeholder.js";
import { BlocksPanel, QueueRefresher, type RootRunHooks } from "./ui/blocks.js";
import type { QueueView } from "./ui/blocks.js";
import { blocksCount, withoutOwed } from "./ui/blocks.js";
import { CardKeyNotice, cardKeyFrom, focusedAsks, startCardLookup, type CardLookup } from "./ui/cardlink.js";
import { BindingsPanel, SettingsPanel } from "./ui/settings.js";
import { UpdateMenu, updateLabel, updatePollMs, type UpdateInfoView } from "./ui/update.js";
import { keyAction, layoutStack, ROOT_PANEL, stackReducer, StackView } from "./ui/stack.js";
import { forgetClassicTabs, HomeTabs, QueueTabs, type QueueTab } from "./ui/tabs.js";
import type { Panel, StackState } from "./ui/stack.js";
import { HOME_SOURCE } from "./ui/identity.js";
import { POLL_MS, startPoll } from "./ui/poll.js";
import { ArchivePanel, type ArchiveApi } from "./ui/archive.js";
import { LearnPanel, LearnSwitch, latestOnly, type LearnCardView } from "./ui/learn.js";
import { TellVizier, ThreadPanel } from "./ui/vizier.js";

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

/** The reader's one call: the server decides what may be read, so a failure comes back as its refusal text. */
function useDocReaderApi(rpc: ReturnType<typeof useRpc<typeof rpcContract>>): DocReaderApi {
  const nav = useBbNavigate();
  return useMemo(
    () => ({
      read: (path, sha256) => rpc.call("readDoc", { path, ...(sha256 !== undefined ? { sha256 } : {}) }).then((r) => r as DocResultT),
      // The server checks the path and names the target; bb's own file panel shows it, read-only.
      openFile: async (path) => {
        const r = (await rpc.call("openTarget", { path })) as { ok: true; target: { kind: "host"; hostId: string; path: string } } | { ok: false; error: string };
        if (!r.ok) return r;
        return nav.experimental_openFilePreview({ target: r.target, location: null }) ? { ok: true } : { ok: false, error: "bb could not open the file panel here" };
      },
    }),
    [rpc, nav],
  );
}

/** Conversation on the card: unread counts (polled), the conversation read, and the read mark. All display; no write but the read mark. */
function useConversationApi(rpc: ReturnType<typeof useRpc<typeof rpcContract>>): ConversationApi {
  const [unread, setUnread] = useState<Record<string, number>>({});
  const refetch = useCallback(() => {
    rpc.call("conversationUnread").then((r) => setUnread((r as { unread: Record<string, number> }).unread ?? {}), () => {});
  }, [rpc]);
  useEffect(() => {
    refetch();
    return startPoll(refetch, POLL_MS);
  }, [refetch]);
  useEffect(() => {
    setTimingSink((t) => void rpc.call("clickTiming", t).catch(() => {}));
    return () => setTimingSink(null);
  }, [rpc]);
  return useMemo(
    () => ({
      unread,
      load: (task_id) => timed("conversation", rpc.call("conversation", { task_id })).then((d) => d as unknown as ConversationData),
      markSeen: (task_id, through) => void rpc.call("markConversationSeen", { task_id, through }).then(refetch, () => {}),
    }),
    [rpc, unread, refetch],
  );
}

/** The same panel, opened beside a thread by the thread-panel action: this thread's cards are pinned. */
function BlocksThreadPanel({ threadId }: { threadId: string }) {
  const { rpc, queue, error, refetch } = useQueue(threadId);
  const note = useNote(rpc, refetch);
  const { error: actionError, act } = useActionError();
  const nav = useBbNavigate();
  const picks = useMemo(() => new PickController(() => crypto.randomUUID()), []);
  const rootRun = useRootRuns(rpc);
  const conversation = useConversationApi(rpc);
  const docs = useDocReaderApi(rpc);
  if (queue === null) return <EmptyState>{error ?? "Loading…"}</EmptyState>;
  return (
    <ConversationProvider value={conversation}><DocReaderHost api={docs}>
    <ActionError message={actionError} />
    <BlocksPanel
      rootRun={rootRun}
      onBind={(b) => act("Binding the project", rpc.call("setBinding", b), refetch)}
      data={queue}
      nowMs={Date.now()}
      thread={threadId}
      onNote={(decision_id, text, images) => note({ decision_id }, text, images)}
      onOpen={(t) => nav.toThread(t)}
      onPick={(decision_id, option_id, revision, reason, images, answers) => pickOutcome(picks.send((req) => rpc.call("pick", req) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}), ...(images && images.length > 0 ? { images } : {}), ...(answers ? { answers } : {}) }, refetch)).finally(refetch)}
    />
    </DocReaderHost></ConversationProvider>
  );
}

/** A read run now, on the poll and on each returned call, one at a time (see singleFlight). Unmounting stops it, and a
 * reply that lands after that is dropped. When `enabled` is false nothing runs and the returned call does nothing. */
function usePolledRead(run: (live: () => boolean) => Promise<unknown>, deps: readonly unknown[], enabled = true): () => void {
  const flight = useRef<Flight | null>(null);
  useEffect(() => {
    if (!enabled) return;
    const f = singleFlight(run);
    flight.current = f;
    f.go();
    const stopPoll = startPoll(f.go, POLL_MS);
    return () => {
      stopPoll();
      f.stop();
      if (flight.current === f) flight.current = null;
    };
    // run is rebuilt each render; deps say when it changes.
  }, [...deps, enabled]);
  return useCallback(() => flight.current?.go(), []);
}

/** The server publishes no realtime channel for Home, so the page polls and refetches after each action. */
function useHomeData() {
  const rpc = useRpc<typeof rpcContract>();
  const [asks, setAsks] = useState<AsksData | null>(null);
  const [catchup, setCatchup] = useState<CatchupEntry[]>([]);
  // The catch-up read can fail alone (an oversized or cut-off reply); the rest of the panel stays up with a line saying so.
  const [catchupError, setCatchupError] = useState<string | null>(null);
  const [waiting, setWaiting] = useState<Waiting | null>(null);
  const [health, setHealth] = useState<{ ready: boolean } | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Reads overlap (poll, realtime signal, after an action); one runs at a time, so an older reply never overwrites a
  // newer snapshot.
  const refetch = usePolledRead(
    (live) =>
      Promise.all([rpc.call("listAsks"), settleCatchup(rpc.call("catchup")), rpc.call("health"), rpc.call("waiting").catch(() => null)]).then(
        ([a, c, h, w]) => {
          if (!live()) return;
          setAsks(a as unknown as AsksData);
          if (c.ok) {
            setCatchup(normalizeCatchup(c.items));
            setCatchupError(null);
          } else setCatchupError(c.error);
          setWaiting(w as Waiting | null);
          setHealth(h as { ready: boolean });
          setError(null);
        },
        (cause) => {
          if (live()) setError(cause instanceof Error ? cause.message : String(cause));
        },
      ),
    [rpc],
  );
  // The totals move when a card is answered, a move is claimed or a hold changes, not only on the poll.
  useRealtime("home-queue-changed", refetch);
  const dropOwed = useCallback((id: string) => setAsks((a) => withoutPicked(a, id)), []);
  const setLater = useCallback((taskId: string, on: boolean) => setAsks((a) => withLater(a, taskId, on)), []);
  return { rpc, asks, catchup, catchupError, waiting, health, error, refetch, dropOwed, setLater };
}

/** Posting a note on a card. A note id is stable per (target, text) until it is accepted, so a retry cannot post the comment twice. */
function useNote(rpc: ReturnType<typeof useRpc<typeof rpcContract>>, onChanged: () => void) {
  const noteIds = useMemo(() => new Map<string, string>(), []);
  return useCallback(
    async (target: { task_id: string } | { decision_id: string }, text: string, images?: OtherImage[]) => {
      const key = `${JSON.stringify(target)}|${text}|${imagesSig(images)}`;
      const note_id = noteIds.get(key) ?? crypto.randomUUID().slice(0, 32);
      noteIds.set(key, note_id);
      const r = await rpcOutcome(rpc.call("note", { ...target, text, note_id, ...(images && images.length > 0 ? { images } : {}) }));
      if (r.ok) {
        noteIds.delete(key);
        onChanged();
      }
      return r;
    },
    [rpc, noteIds, onChanged],
  );
}

function HomePage({ subPath }: { subPath?: string }) {
  const { rpc, asks, catchup, catchupError, waiting, error, refetch, dropOwed, setLater } = useHomeData();
  const nav = useBbNavigate();
  const blocks = useQueue();
  const { excluded, setExcluded } = useProjectFilter();
  const rootRuns = useRootRuns(rpc);
  const note = useNote(rpc, refetch);
  const { error: actionError, act } = useActionError();
  const conversation = useConversationApi(rpc);
  const docs = useDocReaderApi(rpc);
  const [stack, setStack] = useState<StackState>({ panels: [ROOT_PANEL], width: "third" });
  useEffect(() => forgetClassicTabs(() => localStorage), []);
  // Phones get the inbox layout; "Full view" keeps the desktop layout on this device until the page is reopened.
  const phone = useIsPhone();
  const [fullView, setFullView] = useState(false);
  // The Idea box (bead mk-2zojo): a panel on the desktop queue, a header button on the phone.
  const [ideaOpen, setIdeaOpen] = useState(false);
  const [ideaTick, setIdeaTick] = useState(0);
  useRealtime("home-queue-changed", () => setIdeaTick((n) => n + 1));
  // The Learn deck (bead mk-uoiwp): off by default; the tab shows only when Settings turns it on.
  const [learn, setLearn] = useState<{ enabled: boolean; cards: LearnCardView[] }>({ enabled: false, cards: [] });
  const [learnBusy, setLearnBusy] = useState(false);
  const archiveApi = useMemo<ArchiveApi>(() => ({
    load: async (f, offset, limit) => {
      const r = (await rpc.call("archive", { ...(f.search ? { search: f.search } : {}), ...(f.project ? { project: f.project } : {}), ...(f.from ? { from: f.from } : {}), ...(f.to ? { to: f.to } : {}), ...(f.pick ? { pick: f.pick } : {}), offset, limit })) as { ok: boolean; rows?: never[]; total?: number; projects?: string[] };
      return { rows: r.rows ?? [], total: r.total ?? 0, projects: r.projects ?? [] };
    },
    bringBack: async (taskId, decisionId) => {
      const r = (await rpc.call("bringBack", { task_id: taskId, decision_id: decisionId })) as { ok: boolean; error?: string };
      if (r.ok) { refetch(); return null; }
      return r.error ?? "refused";
    },
  }), [rpc, refetch]);
  const loadLearn = useMemo(() => latestOnly(
    () => rpc.call("learn") as Promise<{ ok: boolean; enabled?: boolean; cards?: LearnCardView[] }>,
    (r) => { if (r.ok) setLearn({ enabled: r.enabled ?? false, cards: r.cards ?? [] }); },
  ), [rpc]);
  useEffect(loadLearn, [loadLearn]);
  useRealtime("home-learn-changed", loadLearn);
  const setLearnState = (args: { enabled?: boolean; card?: string; mark?: "got" | "later" | "clear" }) => { setLearnBusy(true); act("Saving the Learn setting", rpc.call("learnSet", args), () => { loadLearn(); setLearnBusy(false); }); };
  const [openIdeas, setOpenIdeas] = useState<number | undefined>(undefined);
  const ideaApi = useMemo<IdeaApi>(() => ({
    projects: async () => { const r = (await rpc.call("ideaProjects")) as { ok: boolean; projects?: IdeaProject[]; error?: string }; if (!r.ok) throw new Error(r.error ?? "the Idea box could not load projects"); return r.projects ?? []; },
    digest: async () => { const r = (await rpc.call("ideas")) as { ok: boolean; ideas?: IdeaView[]; show?: boolean; error?: string }; if (!r.ok) throw new Error(r.error ?? "the ideas could not load"); return { ideas: r.ideas ?? [], show: r.show ?? false }; },
    file: async (project_id, text, idea_id) => (await rpc.call("fileIdea", { project_id, text, idea_id })) as { ok: boolean; error?: string },
    act: async (task_id, action) => (await rpc.call("actIdea", { task_id, action })) as { ok: boolean; error?: string },
    clear: async () => { const r = (await rpc.call("clearIdeaDigest")) as { ok?: boolean; error?: string }; if (r && r.ok === false) throw new Error(r.error ?? "could not hide the digest"); },
  }), [rpc]);
  // The Ideas tab carries the open-idea count, as Queue carries the waiting count; no count when the read fails.
  useEffect(() => { void ideaApi.digest().then((d) => setOpenIdeas(d.ideas.length), () => setOpenIdeas(undefined)); }, [ideaApi, ideaTick]);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  // Rows on screen right now: React state (not just the tracker) so "Mark N seen" can say N.
  const [visible, setVisible] = useState<Set<string>>(new Set());
  const [markResult, setMarkResult] = useState<string | null>(null);
  const [lens, setLens] = useState<Lens>("attention");
  const picks = useMemo(() => new PickController(() => crypto.randomUUID()), []);
  const tracker = useMemo(() => new SeenTracker((item) => act("Marking it seen", rpc.call("markSeen", { item }), refetch)), [rpc, refetch, act]);
  const onVisibility = useCallback(
    (item: string, on: boolean) => {
      tracker.setVisible(item, on);
      setVisible((prev) => {
        if (prev.has(item) === on) return prev;
        const next = new Set(prev);
        if (on) next.add(item);
        else next.delete(item);
        return next;
      });
    },
    [tracker],
  );
  const notices = useMemo(() => catchup.filter((c) => c.kind === "notice"), [catchup]);
  // A "Waiting for you" row for an ask that is set aside (Later or on hold) is not waiting; its tab already lists it.
  const readable = useMemo(() => readableCatchup(catchup, asks?.owed ?? null), [catchup, asks]);
  const [queueTab, setQueueTab] = useState<QueueTab>("decide");
  const markIds = (rows: string[][], routineLeft: number) => {
    const ids = rows.flat();
    if (ids.length === 0) return;
    void rpc.call("markAllSeen", { ids }).then(
      (r) => {
        setMarkResult(markResultText(markedRows(rows, (r as { marked: string[] }).marked), routineLeft));
        refetch();
      },
      () => setMarkResult("That did not go through. Nothing was marked."),
    );
  };
  const refetchAll = () => {
    refetch();
    blocks.refetch();
  };
  /** Later and Move back: the card changes section the moment the call returns ok; the refetch confirms in the background. */
  const sendLater = (ref: string, on: boolean) =>
    laterOutcome(timed("later", rpc.call(on ? "later" : "unlater", { ref }))).then((o) => {
      if (o.ok) setLater(ref, on);
      refetchAll();
      return o;
    });
  /** A pick that went through takes its card off the screen at once; the refetch then confirms it in the background. */
  const sendPick = (decision_id: string, option_id: string, revision: string, reason?: string, images?: OtherImage[], answers?: Record<string, string>) =>
    pickOutcome(picks.send((req) => timed("pick", rpc.call("pick", req)) as never, { decision_id, option_id, revision, ...(reason !== undefined ? { reason } : {}), ...(images && images.length > 0 ? { images } : {}), ...(answers ? { answers } : {}) }, refetchAll)).then((o) => {
      if (o.ok) dropOwed(decision_id);
      return o;
    }).finally(refetchAll);
  /** Open a thread and this plugin's blocks panel beside it; the host declines (false) where there is no side panel. */
  const openBeside = (thread: string) => {
    nav.toThread(thread);
    nav.openThreadPanel({ actionId: "home-blocks" });
  };
  /** The one Revoke: the asks list and the card-link view both use it. */
  const revoke = (approval_id: string) => act("Revoking the approval", rpc.call("revokeApproval", { approval_id }), refetch);
  const dispatch = (a: Parameters<typeof stackReducer>[1]) => setStack((s) => stackReducer(s, a));
  // The key handler is registered once, so it reads the current stack through a ref.
  const stackRef = useRef(stack);
  stackRef.current = stack;
  const push = (panel: Panel) => dispatch({ type: "push", panel });
  const [update, setUpdate] = useState<UpdateInfoView | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [updateReadError, setUpdateReadError] = useState<string | null>(null);
  const loadUpdate = useCallback(() => void rpc.call("updateInfo", null).then(
    (v) => { setUpdate(v as UpdateInfoView); setUpdateReadError(null); },
    (e) => setUpdateReadError(actionErrorText("Reading the update status", e instanceof Error ? e.message : String(e))),
  ), [rpc]);
  const updatePoll = updatePollMs(update);
  useEffect(() => {
    loadUpdate();
    const t = setInterval(loadUpdate, updatePoll);
    return () => clearInterval(t);
  }, [loadUpdate, updatePoll]);
  const requestUpdate = (sha: string) => {
    setUpdateError(null);
    void rpc.call("requestUpdate", { sha }).then((r) => {
      const v = r as { ok: boolean; error?: string };
      if (!v.ok) setUpdateError(v.error ?? "the request was refused");
      loadUpdate();
    }, (e) => setUpdateError(String(e)));
  };

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
      const mapOnTop = stackRef.current.panels[stackRef.current.panels.length - 1]?.kind === "map";
      const intent = keyAction(e.key, { editable, mapOnTop });
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

  // The project filter (bead mk-sufr9) narrows the card lists only. Its lists are built here, before they reach the panels, and
  // the counts below stay on the unfiltered data unless COUNTS_FOLLOW_FILTER (projectfilterstore.ts) is turned on.
  const allBlocks = blocks.queue ? (asks ? withoutOwed(blocks.queue, asks) : blocks.queue) : null;
  const filterChoices = projectChoices([...(asks ? asks.owed.filter((o) => !o.held) : []), ...(allBlocks ? [...allBlocks.rows, ...allBlocks.legacy.owed] : [])]);
  const hiddenBy = {
    decide: asks ? hiddenIn(asks.owed.filter((o) => !o.held && !o.later), excluded) : 0,
    later: asks ? hiddenIn(asks.owed.filter((o) => !o.held && o.later), excluded) : 0,
    blocks: allBlocks ? hiddenIn(allBlocks.rows, excluded) + hiddenIn(allBlocks.legacy.owed, excluded) : 0,
    // On the phone the held cards sit under the On hold row of the waiting tab, so the filter's note counts them there too.
    heldOnPhone: asks ? hiddenIn(asks.owed.filter((o) => o.held), excluded) : 0,
  };
  const filterBar = (hidden: number) => <ProjectFilterBar choices={filterChoices} excluded={excluded} hidden={hidden} onChange={setExcluded} />;
  const HIDDEN_ALL = "Everything here is hidden by the project filter.";

  const render = (panel: Panel): ReactNode => {
    switch (panel.id) {
      case "queue":
        return (
          <>
            {notices.length > 0 ? (
              <div className="p-2 sm:p-4">
                <NoticeBanner notices={notices} suspended={waiting?.suspended ?? true} onAcknowledge={(item) => act("Marking the notice seen", rpc.call("markSeen", { item }), refetch)} />
              </div>
            ) : null}
            <QueueTabs tab={queueTab} onTab={setQueueTab} definition={waiting?.definition ?? null} line={waiting?.line ?? null} counts={waiting ? { decide: shownCount(waiting.decide, hiddenBy.decide), later: shownCount(asks ? asks.owed.filter((o) => o.later && !o.held).length : waiting.later, hiddenBy.later), blocks: shownCount(allBlocks ? blocksCount(allBlocks) : waiting.held, hiddenBy.blocks), catchup: waiting.updates } : null} />
            {queueTab !== "catchup" ? <div className="border-b border-border py-1" data-queue-filter>{filterBar(hiddenBy[queueTab === "blocks" ? "blocks" : queueTab])}</div> : null}
            <section role="tabpanel" id="queue-tabpanel" aria-labelledby={`queue-tab-${queueTab}`} data-queue-section={queueTab === "decide" ? "asks" : queueTab} className="border-b border-border">
              {queueTab === "decide"
                ? render({ id: "asks", kind: "decision", title: "Asks", part: "decide" })
                : queueTab === "later"
                  ? render({ id: "asks", kind: "decision", title: "Asks", part: "later" })
                  : queueTab === "blocks"
                    ? render({ id: "blocks", kind: "decision", title: "Blocking" })
                    : render({ id: "catchup", kind: "catchup", title: "Catch-up" })}
            </section>
          </>
        );
      case "asks":
        return asks === null ? (
          <EmptyState>{error ?? "Loading asks…"}</EmptyState>
        ) : (
          <>
          <AsksPanel
            data={filterAsks(asks, excluded)}
            hideHeld
            {...(panel.part ? { part: panel.part } : {})}
            {...((panel.part === "later" ? hiddenBy.later : hiddenBy.decide) > 0 ? { emptyText: HIDDEN_ALL } : {})}
            onNote={(decision_id, text, images) => note({ decision_id }, text, images)}
            onLater={sendLater}
            onDismiss={(decision_id, obligation_id) => act("Dismissing it", rpc.call("dismiss", { decision_id, obligation_id }), refetch)}
            onOpen={(thread) => push({ id: `thread:${thread}`, kind: "thread", title: thread, ref: thread })}
            onRevoke={revoke}
            onView={(decision_id) => void timed("setViewing", rpc.call("setViewing", { decision_id })).catch(() => {})}
            onPick={(decision_id, option_id, revision, reason, images, answers) => {
              return sendPick(decision_id, option_id, revision, reason, images, answers);
            }}
          />
          </>
        );
      case "blocks": {
        const blockView = allBlocks ? filterQueue(allBlocks, excluded) : null;
        return blockView === null ? (
          <EmptyState>{blocks.error ?? "Loading blocking cards…"}</EmptyState>
        ) : blockView.rows.length === 0 && blockView.legacy.count === 0 && hiddenBy.blocks > 0 ? (
          <p className="p-4 text-sm text-muted-foreground">{HIDDEN_ALL}</p>
        ) : (
          <BlocksPanel
            rootRun={rootRuns}
            onBind={(b) => act("Binding the project", rpc.call("setBinding", b), refetchAll)}
            data={blockView}
            nowMs={Date.now()}
            onNote={(decision_id, text, images) => note({ decision_id }, text, images)}
            onOpen={openBeside}
            onPick={(decision_id, option_id, revision, reason, images, answers) => {
              return sendPick(decision_id, option_id, revision, reason, images, answers);
            }}
          />
        );
      }
      case "catchup":
        return (
          <CatchupPanel
            loadError={catchupError}
            items={readable}
            expanded={expanded}
            visible={visible}
            result={markResult}
            onToggle={toggle}
            onOverride={(decision_id) => act("Overriding the ruling", rpc.call("override", { decision_id }), refetch)}
            onVisibility={onVisibility}
            onMarkAll={() => {
              const plan = markPlan(readable, expanded, visible);
              markIds(plan.rows, plan.routineLeft);
            }}
            onMarkOne={(item, members) => {
              setMarkResult(null);
              markIds([members ?? [item]], 0);
            }}
          />
        );
      case "ideas":
        return (
          <>
            <IdeaDigest api={ideaApi} refreshKey={ideaTick} onChanged={() => setIdeaTick((n) => n + 1)} />
            <IdeaDesk api={ideaApi} refreshKey={ideaTick} heading={false} onChanged={() => setIdeaTick((n) => n + 1)} />
          </>
        );
      case "archive":
        return <ArchivePanel api={archiveApi} />;
      case "learn":
        return <LearnPanel cards={learn.cards} busy={learnBusy} onMark={(card, mark) => setLearnState({ card, mark })} />;
      case "vizier":
        return <TellVizier threadId={asks?.delegation.settings.vizierThreadId} />;
      case "settings":
        return asks === null ? null : (
          <>
            <LearnSwitch enabled={learn.enabled} busy={learnBusy} onChange={(enabled) => setLearnState({ enabled })} />
            <SettingsPanel delegation={asks.delegation} machineOwners={asks.machineOwners} onSave={(v) => rpc.call("setDelegation", v).finally(refetch)} />
            {blocks.queue === null ? null : (
              <BindingsPanel
                bindings={blocks.queue.bindings}
                unbound={blocks.queue.unbound ?? []}
                serveProjects={blocks.queue.serve_projects ?? []}
                inactive={blocks.queue.inactive_projects}
                legacyCount={blocks.queue.legacy.count}
                onBind={(b) => act("Binding the project", rpc.call("setBinding", b), refetchAll)}
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

  // A card-key link (bead mk-a51hu): /plugins/autarch/home/<KEY>. An open card shows alone, with a way back to the list;
  // any other key gets a notice with the card's state, and the list stays under it.
  const linkKey = cardKeyFrom(subPath);
  const linked = linkKey !== null && asks !== null ? asks.owed.find((o) => o.key === linkKey) ?? null : null;
  const [lookup, setLookup] = useState<{ key: string; result: CardLookup } | null>(null);
  const needLookup = linkKey !== null && asks !== null && linked === null;
  useEffect(() => {
    if (!needLookup || linkKey === null) return;
    return startCardLookup((i) => rpc.call("cardState", i), linkKey, (key, result) => setLookup({ key, result }));
  }, [needLookup, linkKey, rpc, asks]); // each Home poll looks again, so a notice never goes stale
  const showAll = () => nav.toPluginPanel("home");
  // A click that did not go through (or an update status that could not be read) shows here, above the list.
  const actionLine = <ActionError message={actionError ?? updateReadError} />;
  if (linked !== null && asks !== null) {
    return (
      <ConversationProvider value={conversation}><DocReaderHost api={docs}>
        <div className="flex h-full min-h-0 flex-1 flex-col overflow-y-auto" data-home-source={HOME_SOURCE} data-card-link={linkKey}>
          <button type="button" className="min-h-11 border-b border-border px-4 text-left text-sm underline" onClick={showAll} data-card-link-back>‹ All cards</button>
          {actionLine}
          <AsksPanel
            data={focusedAsks(asks, linked)}
            onNote={(decision_id, text, images) => note({ decision_id }, text, images)}
            onLater={sendLater}
            onOpen={openBeside}
            onRevoke={revoke}
            onView={(decision_id) => void timed("setViewing", rpc.call("setViewing", { decision_id })).catch(() => {})}
            onPick={(decision_id, option_id, revision, reason, images, answers) => {
              return sendPick(decision_id, option_id, revision, reason, images, answers);
            }}
          />
        </div>
      </DocReaderHost></ConversationProvider>
    );
  }
  const keyNotice = linkKey !== null && lookup !== null && lookup.key === linkKey ? <CardKeyNotice cardKey={linkKey} lookup={lookup.result} /> : null;

  if (phone && !fullView && asks !== null) {
    return (
      <ConversationProvider value={conversation}><DocReaderHost api={docs}>
        <div className="flex h-full min-h-0 flex-1 flex-col" data-home-source={HOME_SOURCE}>
          {keyNotice}
          {actionLine}
          <MobileInbox
            asks={filterAsks(asks, excluded)}
            waiting={shownCount(waiting?.total ?? 0, hiddenBy.decide)}
            filter={{ node: filterBar, hidden: { waiting: hiddenBy.decide, later: hiddenBy.later }, heldHidden: hiddenBy.heldOnPhone, summaryOwed: COUNTS_FOLLOW_FILTER ? filterAsks(asks, excluded).owed : asks.owed, laterCount: shownCount(asks.owed.filter((o) => o.later && !o.held).length, hiddenBy.later) }}
            sinceCount={waiting ? waiting.updates + waiting.notices : readable.length + notices.length}
            sinceNode={
              <>
                {notices.length > 0 ? (
                  <div className="p-2">
                    <NoticeBanner notices={notices} suspended={waiting?.suspended ?? true} onAcknowledge={(item) => act("Marking the notice seen", rpc.call("markSeen", { item }), refetch)} />
                  </div>
                ) : null}
                {render({ id: "catchup", kind: "catchup", title: "Catch-up" })}
              </>
            }
            onOpen={openBeside}
            onNote={(decision_id, text, images) => note({ decision_id }, text, images)}
            onLater={sendLater}
            onPick={(decision_id, option_id, revision, reason, images, answers) => {
              return sendPick(decision_id, option_id, revision, reason, images, answers);
            }}
            onDesktop={() => setFullView(true)}
            onIdea={() => setIdeaOpen((o) => !o)}
            updateNode={updateLabel(update) ? <UpdateMenu info={update} onRequest={requestUpdate} error={updateError} /> : undefined}
            ideaNode={ideaOpen ? <><IdeaDigest api={ideaApi} refreshKey={ideaTick} /><IdeaDesk api={ideaApi} refreshKey={ideaTick} /></> : null}
          />
        </div>
      </DocReaderHost></ConversationProvider>
    );
  }
  return (
    <ConversationProvider value={conversation}><DocReaderHost api={docs}>
    <div className="flex h-full min-h-0 flex-1 flex-col" data-home-source={HOME_SOURCE}>
      {keyNotice}
      {actionLine}
      {phone && fullView ? <button type="button" className="min-h-11 border-b border-border px-4 text-left text-sm underline" onClick={() => setFullView(false)} data-inbox-return>‹ Back to the phone inbox</button> : null}
      <HomeTabs extra={updateLabel(update) ? <UpdateMenu info={update} onRequest={requestUpdate} error={updateError} /> : undefined} learn={learn.enabled} {...(waiting ? { waiting: waiting.total } : {})} ideas={openIdeas} onOpen={push} />
      <StackView placed={layoutStack(stack)} render={render} onExpand={push} />
    </div>
    </DocReaderHost></ConversationProvider>
  );
}

/** Rail badge (`experimental_useBadge`, which replaced the retired `experimental_sidebarAccessory`): the count of decision cards waiting on mk, "!" when serve is not ready or a machine blocker has no owner, nothing at zero. */
function useHomeBadge() {
  const { asks, waiting, health } = useHomeData();
  const blocked = health !== null && !health.ready;
  const unowned = asks?.asks.some((a) => a.owner === null) ?? false;
  // The badge is the Needs you now count, the same number as that tab.
  const count = railCount(waiting?.decide, asks?.owed);
  return railBadge({ count }, { blocked, unowned });
}

// The default export must be definePluginApp(...); BB interprets it after
// loading the bundle. navPanel adds a page to the left sidebar; register
// other UI under app.slots and composer actions, plus-menu rows, banners, or
// rich-text rules with app.composer.customize(...) (see the bb guide's
// plugins chapter).
export default definePluginApp((app) => {
  app.slots.navPanel({
    id: "autarch-home",
    title: "Citadel",
    icon: "Target",
    path: "home",
    component: HomePage,
    experimental_useBadge: useHomeBadge,
  });
  app.slots.threadPanelAction({
    id: "home-blocks",
    title: "Blocking",
    icon: "Target",
    component: ({ threadId }) => <BlocksThreadPanel threadId={threadId} />,
  });
});
