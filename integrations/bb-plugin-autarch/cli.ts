// `bb home ...`: the command line agents use to file asks and report blocker progress.
//
// This file is handed the operations it may call. It has no way to read or change the
// delegation settings, and no verb here touches them [D-16]: only the RPC path, driven by mk's
// own browser, can. Exit codes: 2 usage or validation, 3 not filed, 5 already ruled, replaced
// or closed, 1 anything else.
import { cliCommand, defineCli, type PluginCliContext, type PluginCliResult } from "@get-bb/plugin-sdk";
import type { Asks, LifecycleResult } from "./asks.js";
import type { Catchup } from "./catchup.js";
import { parseAsk } from "./model.js";
import type { PickResult, Service } from "./service.js";

export const REQUEST_LIMIT = 16 * 1024;
const DAY_MS = 86_400_000;

export interface HomeCliParts {
  svc: Service;
  asks: Asks;
  catchup: Catchup;
  /** Rule on a decision as the vizier thread; the caller decides who may. */
  rule: (decisionId: string, optionId: string, reason: string, ctx: { threadId?: string }) => PickResult;
  /** True when the thread is the configured vizier thread. */
  isVizier: (threadId: string | undefined) => boolean;
}

const out = (v: unknown): PluginCliResult => ({ exitCode: 0, stdout: JSON.stringify(v) });
const err = (exitCode: number, message: string): PluginCliResult => ({ exitCode, stderr: `${message}\n` });
const exitFor = (status: number) => (status === 409 ? 5 : status === 400 ? 2 : status >= 500 ? 3 : 1);

function lifecycle(r: LifecycleResult): PluginCliResult {
  return r.ok ? out({ ok: true, replay: r.replay }) : err(exitFor(r.status), r.error);
}

export function homeCli(p: HomeCliParts) {
  const { svc } = p;
  const state = (id: string): string | undefined => {
    const d = svc.store.decision(id) as { project: string } | undefined;
    if (!d) return undefined;
    return svc.recent(d.project, 100_000).find((x) => x.id === id)?.status;
  };

  return defineCli({
    name: "home",
    summary: "File asks with mk and report blocker progress",
    usageErrorExitCode: 2,
    commands: {
      ask: cliCommand({
        summary: "File an ask (decide, steps or machine) as JSON",
        options: {
          request: { type: "string", required: true, stdin: true, description: "The ask as a JSON object, at most 16 KiB." },
        },
        async run({ options }, ctx: PluginCliContext) {
          if (Buffer.byteLength(options.request, "utf8") > REQUEST_LIMIT) return err(2, "request is over 16 KiB");
          let raw: unknown;
          try {
            raw = JSON.parse(options.request);
          } catch {
            return err(2, "request is not valid JSON");
          }
          if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return err(2, "request must be a JSON object");
          const req = { ...(raw as Record<string, unknown>) };
          if (ctx.threadId) {
            if (req.thread !== undefined && req.thread !== ctx.threadId) return err(2, "thread conflicts with caller context");
            req.thread = ctx.threadId;
            if (req.asker === undefined) req.asker = "thread";
          } else if (req.asker !== "mycroft") {
            return err(2, "outside a thread only asker mycroft may file");
          }
          let requestId: string | undefined;
          try {
            requestId = parseAsk(req).request_id;
          } catch (e) {
            return err(2, e instanceof Error ? e.message : String(e));
          }
          let r;
          try {
            r = await svc.file(req, { threadId: ctx.threadId });
          } catch (e) {
            return err(3, `not-filed: ${e instanceof Error ? e.message : String(e)}`);
          }
          if (!r.ok) {
            if (r.exit === 3) return err(3, `not-filed: ${r.error}`);
            return err(r.status === 409 ? 5 : r.exit, r.error);
          }
          return out({ id: r.decision_id, request_id: requestId, mentioned: r.mentioned === true });
        },
      }),

      get: cliCommand({
        summary: "Look up a filed ask by request id or decision id",
        options: {
          "request-id": { type: "string", description: "The request id printed by ask." },
          id: { type: "string", description: "A decision id." },
        },
        constraints: [{ kind: "exactly-one", options: ["request-id", "id"] }],
        run({ options }) {
          if (options["request-id"] !== undefined) {
            const reg = svc.store.registry(options["request-id"]);
            if (!reg) return err(1, "unknown request id");
            return out({ result: reg.result, decision_id: reg.decision_id, state: state(reg.decision_id) });
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
          json: { type: "boolean", description: "Print JSON (the only format)." },
        },
        run({ options }) {
          const rows = svc
            .owed()
            .filter((d) => (options.asker ? d.asker === options.asker : true) && (options.project ? d.project === options.project : true))
            .map((d) => ({ id: d.id, subject: d.subject, project: d.project, thread: d.thread, asker: d.asker, filed_at: d.filed_at }));
          return { exitCode: 0, stdout: JSON.stringify(rows) };
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

