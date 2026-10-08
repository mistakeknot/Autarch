// `bb home ...`: look-ups and the lifecycle of asks filed before cards. New asks are cards, filed with
// `autarch needs-mk file`; `bb home ask` is retired.
//
// This file is handed the operations it may call. It has no way to read or change the
// delegation settings, and no verb here touches them [D-16]: only the RPC path, driven by mk's
// own browser, can. Exit codes: 2 usage or validation, 3 not filed, 5 already ruled, replaced
// or closed, 1 anything else.
import { cliCommand, defineCli, type PluginCliContext, type PluginCliResult } from "@get-bb/plugin-sdk";
import type { Asks, LifecycleResult } from "./asks.js";
import type { Catchup } from "./catchup.js";
import { parseAsk } from "./model.js";
import { buildQueue } from "./queueview.js";
import type { PickResult, Service } from "./service.js";

export const REQUEST_LIMIT = 16 * 1024;
const DAY_MS = 86_400_000;

export interface HomeCliParts {
  svc: Service;
  asks: Asks;
  catchup: Catchup;
  /** Rule on a decision as the vizier thread; the caller decides who may. */
  rule: (decisionId: string, optionId: string, reason: string, ctx: { threadId?: string }) => PickResult;
  /** Confirm a tasks project's binding to a Home project, as the vizier (who is logged). The caller decides who may. */
  bind?: (tasksProject: string, homeProject: string, by: string) => Promise<{ ok: true; tasks_project_id: string } | { ok: false; status: number; error: string }>;
  /** Remove a binding, as the vizier. */
  unbind?: (tasksProject: string, by: string) => Promise<{ ok: true; tasks_project_id: string; was: { home_project: string; state: string } } | { ok: false; status: number; error: string }>;
  /** True when the thread is the configured vizier thread. */
  isVizier: (threadId: string | undefined) => boolean;
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
    const c = svc.store.db.prepare("SELECT task_id, project_id, card_key, title, state, asking_thread, routing_mode FROM cards WHERE task_id = ?").get(taskId) as Record<string, unknown> | undefined;
    if (!c) return undefined;
    const g = svc.store.db.prepare("SELECT id, generation FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(taskId) as { id: string; generation: number } | undefined;
    return { ...c, decision_id: g?.id ?? null, generation: g?.generation ?? null, decision_state: g ? state(g.id) ?? null : null };
  };

  return defineCli({
    name: "home",
    summary: "Look up cards and asks; report progress on asks filed before cards",
    usageErrorExitCode: 2,
    commands: {
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
          request: { type: "string", description: "The request key: a card's, or a pre-card ask's request id." },
          "request-id": { type: "string", description: "Alias of --request." },
          id: { type: "string", description: "A decision id." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        constraints: [{ kind: "exactly-one", options: ["card", "request", "request-id", "id"] }],
        run({ options }) {
          if (options.card !== undefined) {
            const c = cardView(options.card);
            return c ? out(c) : err(1, "unknown card");
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
        summary: "List the asks waiting for a ruling",
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
            .map((d) => ({ id: d.id, subject: d.subject, project: d.project, thread: d.thread, asker: d.asker, filed_at: d.filed_at, task_id: d.task_id ?? null, key: svc.cardKey(d.task_id as string | null) }));
          // A card Home shows flagged (display only) is not owed a ruling but is still on mk's page: list it with its reason
          // so a coordinator can check its own card. Not pulled by anyone, and no project name until it is bound.
          const flagged = options.pull
            ? []
            : buildQueue(svc, p.asks)
                .rows.filter((r) => r.display_only && (options.asker ? options.asker === "thread" : true) && (options.project ? r.project === options.project : true))
                .map((r) => ({ id: r.id, subject: r.title, project: r.project, thread: r.thread ?? "", asker: "thread", filed_at: r.created_at, task_id: r.task_id, key: r.card_key, display_only: true, display_reason: r.display_reason, card_key: r.card_key }));
          return { exitCode: 0, stdout: JSON.stringify([...rows, ...flagged]) };
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
        summary: "Picks, delegation and filing counts over a window",
        options: {
          since: { type: "duration", defaultUnit: "d", default: 14 * DAY_MS, description: "Window, default 14d." },
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        run({ options }) {
          return out(svc.stats(new Date(Date.parse(svc.time()) - options.since).toISOString()));
        },
      }),

      feed: cliCommand({
        summary: "The recent-rulings feed a thread sees",
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
          if (!p.isVizier(ctx.threadId)) return err(1, "only the vizier thread may bind a project");
          if (!p.bind) return err(1, "binding is not available");
          const r = await p.bind(positionals.tasks_project, positionals.home_project, ctx.threadId!);
          return r.ok ? out({ ok: true, tasks_project_id: r.tasks_project_id, home_project: positionals.home_project, state: "confirmed" }) : err(exitFor(r.status), r.error);
        },
      }),

      unbind: cliCommand({
        summary: "Vizier only: remove a project binding (undoes `bind`)",
        positionals: [{ name: "tasks_project", description: "Tasks project: id, key prefix or name.", required: true }],
        async run({ positionals }, ctx) {
          if (!p.isVizier(ctx.threadId)) return err(1, "only the vizier thread may unbind a project");
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
        run({ positionals, options }, ctx) {
          if (!p.isVizier(ctx.threadId)) return err(1, "only the vizier thread may rule");
          const r = p.rule(positionals.id, positionals.option, options.reason, { threadId: ctx.threadId });
          return r.ok ? out({ ok: true, decision_id: positionals.id, option: r.pick.option_id }) : err(exitFor(r.status), r.error);
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

      note: cliCommand({
        summary: "Vizier only: leave a note for mk that cites existing facts",
        positionals: [{ name: "text", description: "The note.", required: true }],
        options: { cites: { type: "string", repeatable: true, split: ",", description: "Ids of the facts the note cites." } },
        run({ positionals, options }, ctx) {
          if (!p.isVizier(ctx.threadId)) return err(1, "only the vizier thread may leave a note");
          const r = p.catchup.addNote(positionals.text, options.cites);
          return r.ok ? out({ ok: true, id: r.id }) : err(2, r.error);
        },
      }),
    },
  });
}
