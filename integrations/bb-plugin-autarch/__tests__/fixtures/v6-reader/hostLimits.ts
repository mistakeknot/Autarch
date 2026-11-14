// Limits the bb host enforces on the calls this plugin makes. A request over one is rejected whole (invalid_request),
// not trimmed, so a page size must never be asked for above these.

/**
 * The most thread events one `bb.sdk.threads.events.list` call may ask for (the host's THREAD_EVENT_LIST_PAGE_SIZE,
 * in its server-contract threads API: "Thread event limit cannot exceed 100").
 */
export const THREAD_EVENT_PAGE_CAP = 100;

/** `limit` as the string the events call takes, held to the host's page cap and to at least 1. */
export function threadEventLimit(limit: number): string {
  const n = Number.isFinite(limit) ? Math.trunc(limit) : THREAD_EVENT_PAGE_CAP;
  return String(Math.max(1, Math.min(n, THREAD_EVENT_PAGE_CAP)));
}
