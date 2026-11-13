// `bb home ...`: look-ups and the lifecycle of asks filed before cards. New asks are cards, filed with
// `autarch needs-mk file`; `bb home ask` is retired.
//
// This file is handed the operations it may call. It has no way to read or change the
// delegation settings, and no verb here touches them [D-16]: only the RPC path, driven by mk's
// own browser, can. Exit codes: 2 usage or validation, 3 not filed, 5 already ruled, replaced
// or closed, 1 anything else.
import { homeMethods, verbSummary } from "./contract.js";
import { groupMoves } from "./moveexport.js";
import { cliCommand, defineCli, type PluginCliResult } from "@get-bb/plugin-sdk";
import type { Asks, LifecycleResult } from "./asks.js";
import type { Catchup } from "./catchup.js";
import type { Waiting } from "./waiting.js";
import { readSettled } from "./settled.js";
import { buildQueue } from "./queueview.js";
import type { PickResult, Service } from "./service.js";
import type { TargetEvidence } from "./picktarget.js";

export const REQUEST_LIMIT = 16 * 1024;
const DAY_MS = 86_400_000;

export interface HomeCliParts {
  readTargetEvidence?: (threadId: string) => Promise<TargetEvidence>;
  svc: Service;
  asks: Asks;
  catchup: Catchup;
  /** The one waiting-on-mk count (waiting.ts); omitted, stats reports no `waiting` block. */
  waiting?: () => Waiting;
  /** Rule on a decision as the vizier thread; the caller decides who may. */
  rule: (decisionId: string, optionId: string, reason: string, ctx: { threadId?: string }) => PickResult | Promise<PickResult>;
  /** Hand the vizier role to another thread. Only the current vizier may; the caller decides who. */
  /** Sync stored-id recheck run right before a vizier write. */
  stillVizier?: (threadId: string | undefined) => boolean;
  handoff?: (to: string, ctx: { threadId?: string }) => Promise<{ ok: true; from: string; to: string } | { ok: false; status: number; error: string }>;
  /** Confirm a tasks project's binding to a Home project, as the vizier (who is logged). The caller decides who may. */
  bind?: (tasksProject: string, homeProject: string, by: string) => Promise<{ ok: true; tasks_project_id: string } | { ok: false; status: number; error: string }>;
  /** Remove a binding, as the vizier. */
  unbind?: (tasksProject: string, by: string) => Promise<{ ok: true; tasks_project_id: string; was: { home_project: string; state: string } } | { ok: false; status: number; error: string }>;
  /** True when the thread is the vizier, as the one resolver says (stored id, else the pinned-title fallback). */
  isVizier: (threadId: string | undefined) => boolean | Promise<boolean>;
  /** Thread endings and stale open tasks over the window (traceability.ts); omitted, stats reports only Home's own counts. */
  traceability?: (since: string) => Promise<unknown>;
}

const out = (v: unknown): PluginCliResult => ({ exitCode: 0, stdout: JSON.stringify(v) });
const err = (exitCode: number, message: string): PluginCliResult => ({ exitCode, stderr: `${message}\n` });
export const ASK_MOVED = "moved: use `autarch needs-mk file`";
const CARD_CLOSES = "card asks close through tasks";

/** Q4 default, one function: which owed rows `list --pull mycroft` shows. A card filed with pull:mycroft
 *  and a pre-card ask filed with asker mycroft both carry asker "mycroft" in the stored ask. */
export function isPulled(d: Record<string, unknown>, who: "mycroft"): boolean {
  return d.asker === who;
}

const exitFor = (status: number) => (status === 409 ? 5 : status === 400 ? 2 : status >= 500 ? 3 : 1);

function lifecycle(r: LifecycleResult): PluginCliResult {
  if (!r.ok && r.error === CARD_CLOSES) return err(2, CARD_CLOSES);
  return r.ok ? out({ ok: true, replay: r.replay }) : err(exitFor(r.status), r.error);
}

export function homeCli(p: HomeCliParts) {
  const { svc } = p;
  const state = (id: string): string | undefined => {
    const d = svc.store.decision(id) as { project: string } | undefined;
    if (!d) return undefined;
    return svc.recent(d.project, 100_000).find((x) => x.id === id)?.status;
  };

  const cardView = (taskId: string) => {
    const c = svc.store.db.prepare("SELECT task_id, project_id, card_key, title, state, asking_thread, routing_mode FROM cards WHERE task_id = ?").get(taskId) as (Record<string, unknown> & { project_id: string }) | undefined;
    if (!c) return undefined;
    const g = svc.store.db.prepare("SELECT id, generation FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(taskId) as { id: string; generation: number } | undefined;
    // time_to_answer is additive: null for a card whose ask never carried urgency "blocker" and so never pushed.
    const urgent = g ? (svc.store.db.prepare("SELECT 1 AS x FROM decisions WHERE task_id = ? AND urgency = 'blocker' LIMIT 1").get(taskId) as { x: number } | undefined) : undefined;
    return { ...c, decision_id: g?.id ?? null, generation: g?.generation ?? null, decision_state: g ? state(g.id) ?? null : null, time_to_answer: g && urgent ? svc.timeToAnswer(g.id) ?? null : null };
  };

  return defineCli({
    name: "home",
    summary: "Look up cards and asks; report progress on asks filed before cards",
    usageErrorExitCode: 2,
    commands: {
      capabilities: cliCommand({
        summary: "Read supported card declaration and target lookup versions",
        options: { json: { type: "boolean", description: "Print JSON." } },
        run: () => out({ schema: 1, on_pick: p.readTargetEvidence && svc.supportsOnPick() ? [1] : [], target_lookup: p.readTargetEvidence ? [1] : [] }),
      }),
      target: cliCommand({
        summary: "Read host thread title and project evidence",
        options: { thread: { type: "string", required: true, description: "Concrete thread identifier." }, json: { type: "boolean", description: "Print JSON." } },
        async run({ options }) {
          if (!p.readTargetEvidence) return err(1, "unavailable: target lookup");
          try { return out(await p.readTargetEvidence(options.thread)); }
          catch (e) { return err(1, e instanceof Error ? e.message : "unavailable: target lookup"); }
        },
      }),
      ask: cliCommand({
        summary: "Retired: file a card with `autarch needs-mk file`",
        options: {
          request: { type: "string", stdin: true, description: "Ignored. `bb home ask` moved to `autarch needs-mk file`." },
        },
        run: () => err(2, ASK_MOVED),
      }),

      get: cliCommand({
        summary: "Look up a card, a filed ask by request key, or a legacy decision id",
        options: {
          card: { type: "string", description: "A tasks card (task) id." },
          project: { type: "string", description: "Tasks project id for --card; empty means no filter, independent of the working directory." },
          request: { type: "string", description: "The request key: a card's, or a pre-card ask's request id." },
          "request-id": { type: "string", description: "Alias of --request." },
          id: { type: "string", description: "A decision id." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        constraints: [{ kind: "exactly-one", options: ["card", "request", "request-id", "id"] }],
        run({ options }) {
          if (options.card !== undefined) {
            const c = cardView(options.card);
            return c && (!options.project || c.project_id === options.project) ? out(c) : err(1, options.project ? "unknown card in the requested project" : "unknown card");
          }
          const key = options.request ?? options["request-id"];
          if (key !== undefined) {
            // The registry answers exactly one of registered, legacy or absent. Not ready is exit 3, so the
            // filer never reads an unready service as "absent" and creates a card.
            if (!svc.ready()) return err(3, "home is not ready");
            const reg = svc.store.db.prepare("SELECT task_id, identity FROM card_requests WHERE request_key = ?").get(key) as { task_id: string; identity: string } | undefined;
            if (reg) {
              const c = cardView(reg.task_id);
              return out({ status: "registered", ...(c ?? {}), task_id: reg.task_id, identity: reg.identity, request_key: key });
            }
            const legacy = svc.store.registry(key);
            if (!legacy) return out({ status: "absent" });
            const project = (svc.store.decision(legacy.decision_id) as { project?: string } | undefined)?.project ?? null;
            return out({ status: "legacy", result: legacy.result, decision_id: legacy.decision_id, identity: legacy.identity, thread: legacy.thread, project, state: state(legacy.decision_id) });
          }
          const id = options.id!;
          const s = state(id);
          return s === undefined ? err(1, "unknown decision") : out({ id, state: s });
        },
      }),

      list: cliCommand({
        summary: verbSummary("listAsks"),
        options: {
          asker: { type: "enum", values: ["thread", "mycroft"], description: "Only asks from this asker." },
          project: { type: "string", description: "Only this project." },
          pull: { type: "enum", values: ["mycroft"], description: "Only asks Mycroft pulls, cards and pre-card rows alike." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        run({ options }) {
          const rows = svc
            .owed()
            .filter((d) => (options.asker ? d.asker === options.asker : true) && (options.project ? d.project === options.project : true) && (options.pull ? isPulled(d, options.pull) : true))
            .map((d) => ({ id: d.id, subject: d.subject, project: d.project, thread: d.thread, asker: d.asker, filed_at: d.filed_at, task_id: d.task_id ?? null, key: svc.cardKey(d.task_id as string | null), held: svc.holdOf(d.task_id as string | null) }));
          // A card Home shows flagged (display only) is not owed a ruling but is still on mk's page: list it with its reason
          // so a coordinator can check its own card. Not pulled by anyone, and no project name until it is bound.
          const flagged = options.pull
            ? []
            : buildQueue(svc, p.asks)
                .rows.filter((r) => r.display_only && (options.asker ? options.asker === "thread" : true) && (options.project ? r.project === options.project : true))
                .map((r) => ({ id: r.id, subject: r.title, project: r.project, thread: r.thread ?? "", asker: "thread", filed_at: r.created_at, task_id: r.task_id, key: r.card_key, display_only: true, display_reason: r.display_reason, card_key: r.card_key, held: r.held }));
          return { exitCode: 0, stdout: JSON.stringify([...rows, ...flagged]) };
        },
      }),

      pushes: cliCommand({
        summary: "List blocker asks (urgency blocker) with push time and time to answer; read-only",
        options: {
          "by-thread": { type: "boolean", description: "Count blocker cards by filing thread and first-generation filing time; task-less older asks are excluded. Read-only." },
          days: { type: "string", description: "Audit window in days (default 7, at most 365). Requires --by-thread." },
          limit: { type: "string", description: "Most recent N blocker asks (default 100, at most 1000)." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        run({ options }) {
          if (options["by-thread"]) {
            const days = options.days === undefined ? 7 : Number(options.days);
            if (!Number.isInteger(days) || days < 1 || days > 365) return err(2, "days must be an integer from 1 to 365");
            if (options.limit !== undefined) return err(2, "limit cannot be used with --by-thread");
            return out(svc.blockersByThread(days));
          }
          if (options.days !== undefined) return err(2, "days requires --by-thread");
          const n = options.limit === undefined ? undefined : Number(options.limit);
          if (n !== undefined && (!Number.isInteger(n) || n < 1)) return err(2, "limit must be a positive integer");
          return out({ pushes: svc.pushes(n) });
        },
      }),

      moves: cliCommand({
        summary: "Every stored Your move row with its claim, report and closing evidence (rollback export; nothing opens or closes a move any more)",
        options: { json: { type: "boolean", description: "Print JSON (the only format)." } },
        run() {
          const rows = svc.store.moves();
          return out({ moves: rows, groups: groupMoves(rows) });
        },
      }),

      viewing: cliCommand({
        summary: "The ask Home last showed mk (the last selection only; read from this machine's plugin store)",
        options: { json: { type: "boolean", description: "Print JSON (the only format)." } },
        run() {
          let raw: { decision_id?: string; at?: string } | null = null;
          try {
            raw = JSON.parse(svc.store.setting("viewing") ?? "null");
          } catch {
            raw = null;
          }
          if (!raw || !raw.decision_id) return out({ viewing: null, reason: "Home has not shown an ask yet, or nothing is open" });
          const d = svc.owed().find((x) => x.id === raw!.decision_id);
          if (!d) return out({ viewing: null, reason: "the ask last shown is no longer open", decision_id: raw.decision_id, at: raw.at ?? null });
          return out({ viewing: { decision_id: d.id, task_id: d.task_id ?? null, key: svc.cardKey(d.task_id as string | null), subject: d.subject, project: d.project, thread: d.thread, at: raw.at ?? null } });
        },
      }),

      stats: cliCommand({
        summary: verbSummary("stats"),
        options: {
          since: { type: "duration", defaultUnit: "d", default: 14 * DAY_MS, description: "Window, default 14d." },
          "settled-file": { type: "string", description: "Absolute path of a coordinator's settled-for-mk.md; adds settled_for_mk, its entries over the window, beside the ask counts." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        async run({ options }) {
          // The window obeys the same limits as the panel's `stats` method (whole days read downward, so 12h is under the minimum).
          const days = homeMethods.stats.input.safeParse({ days: Math.floor(options.since / DAY_MS) });
          if (!days.success) return err(2, "--since must be between 1d and 365d");
          const since = new Date(Date.parse(svc.time()) - options.since).toISOString();
          const settled = options["settled-file"];
          const stats = { ...svc.stats(since), ...(p.waiting ? { waiting: p.waiting() } : {}), ...(settled !== undefined ? { settled_for_mk: readSettled(settled, since) } : {}) };
          return out(p.traceability ? { ...stats, traceability: await p.traceability(since) } : stats);
        },
      }),

      feed: cliCommand({
        summary: verbSummary("listRecent"),
        options: {
          project: { type: "string", required: true, description: "Project name." },
          thread: { type: "string", description: "Thread id; defaults to the caller's." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        run({ options }, ctx) {
          const f = svc.feed(options.project, options.thread ?? ctx.threadId ?? "\u0000");
          return out(f);
        },
      }),

      progress: cliCommand({
        summary: "Report progress on a machine blocker you own",
        positionals: [{ name: "id", description: "Decision id.", required: true }],
        options: { note: { type: "string", description: "What happened." } },
        run: ({ positionals, options }, ctx) => lifecycle(p.asks.progress(positionals.id, options.note, { threadId: ctx.threadId })),
      }),
      resolve: cliCommand({
        summary: "Resolve a machine blocker",
        positionals: [{ name: "id", description: "Decision id.", required: true }],
        options: { note: { type: "string", description: "What resolved it." } },
        run: ({ positionals, options }, ctx) => lifecycle(p.asks.resolve(positionals.id, options.note, { threadId: ctx.threadId })),
      }),
      withdraw: cliCommand({
        summary: "Withdraw a machine blocker you filed",
        positionals: [{ name: "id", description: "Decision id.", required: true }],
        run: ({ positionals }, ctx) => lifecycle(p.asks.withdraw(positionals.id, { threadId: ctx.threadId })),
      }),

      binding: cliCommand({
        summary: "Read the Home project a tasks project is bound to (read-only; the filer checks an ask's project against it)",
        positionals: [{ name: "tasks_project_id", description: "Tasks project id.", required: true }],
        run({ positionals }) {
          const row = svc.store.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(positionals.tasks_project_id) as { home_project: string; state: string } | undefined;
          return out({ tasks_project_id: positionals.tasks_project_id, home_project: row?.home_project ?? null, state: row?.state ?? null });
        },
      }),

      bind: cliCommand({
        summary: "Vizier only: bind a tasks project to a Home project (confirmed, logged with who and when)",
        positionals: [
          { name: "tasks_project", description: "Tasks project: id, key prefix or name.", required: true },
          { name: "home_project", description: "Home project name, as `autarch serve` lists it.", required: true },
        ],
        async run({ positionals }, ctx) {
          if (!(await p.isVizier(ctx.threadId))) return err(1, "only the vizier thread may bind a project");
          if (!p.bind) return err(1, "binding is not available");
          const r = await p.bind(positionals.tasks_project, positionals.home_project, ctx.threadId!);
          return r.ok ? out({ ok: true, tasks_project_id: r.tasks_project_id, home_project: positionals.home_project, state: "confirmed" }) : err(exitFor(r.status), r.error);
        },
      }),

      hold: cliCommand({
        summary: "Vizier only: put a card on hold (greyed out with a reason, out of needs-you-now). A new hold names its end: --until DATE, or an event. When the date passes the card is back at mk's pick and you are told once; an event end is reported with `hold-ended`. A later comment carrying a sha256, or `unhold`, also makes it live",
        positionals: [{ name: "card", description: "Task id or tasks key (PROJ-24).", required: true }],
        options: {
          reason: { type: "string", required: true, description: "Why it is on hold (one line)." },
          until: { type: "string", description: "The date the hold ends: 2026-11-14 (holds through that UTC day) or a full time such as 2026-11-14T18:00:00Z." },
          "event-kind": { type: "string", description: "With --event-ref and --event-label: the kind of event that ends the hold, lowercase with hyphens (pr-merge)." },
          "event-ref": { type: "string", description: "The thing to watch (mistakeknot/Autarch#114)." },
          "event-label": { type: "string", description: "What mk reads on the card: the words after 'until' (PR 114 merges). One line." },
        },
        async run({ positionals, options }, ctx) {
          if (!(await p.isVizier(ctx.threadId)) || (p.stillVizier && !p.stillVizier(ctx.threadId))) return err(1, "only the vizier thread may hold a card");
          const named = [options["event-kind"], options["event-ref"], options["event-label"]].filter((v) => v !== undefined).length;
          if (named > 0 && named < 3) return err(1, "an event end needs --event-kind, --event-ref and --event-label together");
          if (options.until !== undefined && named > 0) return err(1, "name one end: --until, or an event, not both");
          const end = options.until !== undefined ? { until: String(options.until) } : named === 3 ? { event: { kind: String(options["event-kind"]), ref: String(options["event-ref"]), label: String(options["event-label"]) } } : undefined;
          const r = svc.hold(positionals.card, options.reason, ctx.threadId!, end);
          return r.ok ? out({ ok: true, task_id: r.task_id, held: true }) : err(1, r.error);
        },
      }),

      "hold-ended": cliCommand({
        summary: "Vizier only: report that the event a hold was waiting for has happened (name it with --event-kind and --event-ref). The card is back at mk's pick and the thread that held it is told once. Repeating it does nothing",
        positionals: [{ name: "card", description: "Task id or tasks key (PROJ-24).", required: true }],
        options: {
          "event-kind": { type: "string", description: "The kind of the event that happened, as given to the hold (--event-kind). Required." },
          "event-ref": { type: "string", description: "The ref of the event that happened, as given to the hold (--event-ref). Required: a report for any other event does nothing." },
        },
        async run({ positionals, options }, ctx) {
          if (!(await p.isVizier(ctx.threadId)) || (p.stillVizier && !p.stillVizier(ctx.threadId))) return err(1, "only the vizier thread may report a hold's end");
          if (typeof options["event-kind"] !== "string" || options["event-kind"].trim() === "" || typeof options["event-ref"] !== "string" || options["event-ref"].trim() === "") return err(1, "name the event that happened with --event-kind and --event-ref (the kind and ref the hold was set with)");
          const r = svc.endHold(positionals.card, ctx.threadId!, options["event-kind"].trim(), options["event-ref"].trim());
          return r.ok ? out({ ok: true, task_id: r.task_id, ended: r.ended, ...(r.held_for !== undefined ? { held_for: r.held_for } : {}) }) : err(1, r.error);
        },
      }),

      unhold: cliCommand({
        summary: "Vizier only: make a held card live again",
        positionals: [{ name: "card", description: "Task id or tasks key (PROJ-24).", required: true }],
        async run({ positionals }, ctx) {
          if (!(await p.isVizier(ctx.threadId)) || (p.stillVizier && !p.stillVizier(ctx.threadId))) return err(1, "only the vizier thread may release a hold");
          const r = svc.unhold(positionals.card);
          return r.ok ? out({ ok: true, task_id: r.task_id, was_held: r.was }) : err(1, r.error);
        },
      }),

      unbind: cliCommand({
        summary: "Vizier only: remove a project binding (undoes `bind`)",
        positionals: [{ name: "tasks_project", description: "Tasks project: id, key prefix or name.", required: true }],
        async run({ positionals }, ctx) {
          if (!(await p.isVizier(ctx.threadId))) return err(1, "only the vizier thread may unbind a project");
          if (!p.unbind) return err(1, "binding is not available");
          const r = await p.unbind(positionals.tasks_project, ctx.threadId!);
          return r.ok ? out({ ok: true, tasks_project_id: r.tasks_project_id, was: r.was }) : err(exitFor(r.status), r.error);
        },
      }),

      rule: cliCommand({
        summary: "Vizier only: rule on a delegable decision",
        positionals: [
          { name: "id", description: "Decision id.", required: true },
          { name: "option", description: "Option id.", required: true },
        ],
        options: { reason: { type: "string", required: true, description: "Why this option." } },
        async run({ positionals, options }, ctx) {
          if (!(await p.isVizier(ctx.threadId))) return err(1, "only the vizier thread may rule");
          const r = await p.rule(positionals.id, positionals.option, options.reason, { threadId: ctx.threadId });
          return r.ok ? out({ ok: true, decision_id: positionals.id, option: r.pick.option_id }) : err(exitFor(r.status), r.error);
        },
      }),

      handoff: cliCommand({
        summary: "Vizier only: hand the vizier role to another thread (recorded; the successor must be live)",
        positionals: [{ name: "thread", description: "The successor thread id, for example thr_abc123.", required: true }],
        async run({ positionals }, ctx) {
          if (!p.handoff) return err(1, "handoff is not available");
          const r = await p.handoff(positionals.thread, { threadId: ctx.threadId });
          return r.ok ? out({ ok: true, from: r.from, to: r.to }) : err(exitFor(r.status), r.error);
        },
      }),

      "approval check": cliCommand({
        summary: "Read whether mk recorded an approval for a tuple. A record is never an authorization.",
        options: {
          kind: { type: "enum", values: ["merge", "deploy", "release"], required: true, description: "merge, deploy or release." },
          target: { type: "string", required: true, description: "What was approved, for example owner/repo#12." },
          identity: { type: "string", required: true, description: "The exact head, digest or version that was approved." },
        },
        run({ options }) {
          // Reads only. Nothing here can spend a record, by design.
          return out(svc.approvalCheck(options.kind, options.target, options.identity));
        },
      }),

      report: cliCommand({
        summary: "Retired: Home keeps no Your move row to report against, so this records nothing and answers matched 0 (report-tell still calls it)",
        options: {
          "script-sha256": { type: "string", description: "Ignored." },
          "script-path": { type: "string", description: "Ignored." },
          result: { type: "string", description: "Ignored." },
          step: { type: "string", description: "Ignored." },
          log: { type: "string", description: "Ignored." },
          "report-id": { type: "string", description: "Ignored." },
        },
        run() {
          return out({ ok: true, matched: 0 });
        },
      }),

      note: cliCommand({
        summary: "Vizier only: leave a note for mk that cites existing facts",
        positionals: [{ name: "text", description: "The note.", required: true }],
        options: { cites: { type: "string", repeatable: true, split: ",", description: "Ids of the facts the note cites." } },
        async run({ positionals, options }, ctx) {
          if (!(await p.isVizier(ctx.threadId))) return err(1, "only the vizier thread may leave a note");
          // Same continuation as the write: a handoff during the checks above ends this thread's authority.
          if (p.stillVizier && !p.stillVizier(ctx.threadId)) return err(1, "only the vizier thread may leave a note");
          const r = p.catchup.addNote(positionals.text, options.cites);
          return r.ok ? out({ ok: true, id: r.id }) : err(2, r.error);
        },
      }),
    },
  });
}
