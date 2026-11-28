/** The request and provider-acceptance rows exposed by the BB thread events API. */
export interface RequestEvent {
  type: string;
  threadId: string;
  seq: number;
  createdAt: number;
  data: {
    requestId?: string | undefined;
    clientRequestId?: string;
    originPluginId?: string | undefined;
    wakeRef?: string;
    retryOfRequestId?: string;
    retryAttempt?: number;
    inputGroups?: readonly unknown[];
    wakeSources?: readonly unknown[];
    wakeSourcesTruncated?: boolean;
  };
}

export interface RequestDelivery {
  id: string;
  at: number;
}

/** An outbound request row is only a routing witness. The clock starts at the
 * provider's acceptance, on the intended recipient's own timeline.
 */
export function confirmedRequestDelivery(
  events: readonly RequestEvent[], recipient: string, ref: string, createdAt: number,
): RequestDelivery | null {
  const threadEvents = events.filter(e => e.threadId === recipient && Number.isSafeInteger(e.seq)
    && Number.isSafeInteger(e.createdAt) && e.createdAt >= createdAt);
  const rejected = new Set(threadEvents.filter(e => e.type === "client/turn/rejected").map(e => e.data.requestId));
  const requests = new Map<string, RequestEvent>();
  let original: string | undefined;
  for (const e of threadEvents) {
    const d = e.data;
    if (e.type !== "client/turn/requested" || d.wakeRef !== ref || d.originPluginId !== "autarch") continue;
    if (!d.requestId || requests.has(d.requestId) || (d.inputGroups?.length ?? 0) > 1
        || (d.wakeSources?.length ?? 0) > 1 || d.wakeSourcesTruncated
        || (d.retryOfRequestId === undefined) !== (d.retryAttempt === undefined)
        || d.retryAttempt !== undefined && (!Number.isSafeInteger(d.retryAttempt) || d.retryAttempt < 2)) return null;
    const root = d.retryOfRequestId ?? d.requestId;
    if (!root || original !== undefined && original !== root) return null;
    original = root;
    requests.set(d.requestId, e);
  }
  if (!original) return null;
  let earliest: number | undefined;
  for (const e of threadEvents) {
    const id = e.data.clientRequestId;
    if (e.type !== "turn/input/accepted" || !id || rejected.has(id)) continue;
    const request = requests.get(id);
    if (!request || e.seq <= request.seq || e.createdAt < request.createdAt) continue;
    earliest = Math.min(earliest ?? e.createdAt, e.createdAt);
  }
  return earliest === undefined ? null : { id: original, at: earliest };
}
