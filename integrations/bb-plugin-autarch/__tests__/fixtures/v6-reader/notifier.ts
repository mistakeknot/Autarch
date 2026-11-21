// The phone push for a blocker card (bead mk-tv70n.7). The service talks to a Notifier; the concrete one calls the
// push-notifications plugin over plugins.callRpc (never the bb CLI). A push is best effort: every failure (the
// plugin missing, an RPC error, a timeout, a result that is not the expected shape) is thrown to the caller as an
// Error, and the service swallows and logs it. Filing a card never waits on, or fails because of, a push.

import { z } from "zod";
import type { PluginsLike } from "./tasks.js";

export interface PushRequest {
  kind: "blocker";
  title: string;
  body: string;
  thread?: string;
  /** The card key. The notifier side drops a second push with the same key. */
  dedupeKey: string;
}

export interface PushResult {
  id: string;
  at: string;
  /** True when nothing was sent because the daily cap was reached. Still a success. */
  capped?: boolean;
  /** True when the notifier side had already handled this dedupeKey. Still a success. */
  deduped?: boolean;
}

export interface Notifier {
  notify(req: PushRequest): Promise<PushResult>;
}

export const PUSH_TIMEOUT_MS = 10_000;
export const PUSH_PLUGIN_ID = "push-notifications";
export const PUSH_METHOD = "notifications.notify";

// The host sets the source to this plugin's id; extra result fields (day, source, outcome, cap, sentToday, delivery) are ignored.
const resultSchema = z.object({ id: z.string().min(1), at: z.string().min(1), capped: z.boolean().optional(), deduped: z.boolean().optional() }).passthrough();

/** Calls notifications.notify on the push-notifications plugin. The input carries no source: the host fills it in. */
export class RpcNotifier implements Notifier {
  constructor(
    private readonly plugins: PluginsLike,
    private readonly opts: { timeoutMs?: number } = {},
  ) {}

  async notify(req: PushRequest): Promise<PushResult> {
    const input = {
      kind: req.kind,
      title: req.title,
      body: req.body,
      ...(req.thread ? { threadId: req.thread } : {}),
      dedupeKey: req.dedupeKey,
    };
    const signal = AbortSignal.timeout(this.opts.timeoutMs ?? PUSH_TIMEOUT_MS);
    const raw = await this.plugins.callRpc<unknown>({ pluginId: PUSH_PLUGIN_ID, method: PUSH_METHOD, input, signal, outputSchema: z.unknown() });
    const r = resultSchema.parse(raw);
    return { id: r.id, at: r.at, ...(r.capped === true ? { capped: true } : {}), ...(r.deduped === true ? { deduped: true } : {}) };
  }
}
