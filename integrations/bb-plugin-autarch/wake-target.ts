import type { ThreadRow, VizierResolution } from "./delegation.js";

export type RerouteDiagnostic = {
  reason: "no-target" | "resolver-unavailable" | "resolver-error" | "same-thread" | "retarget-race" | "reroute-not-allowed" | "row-closed"
    | "no-live-target" | "thread-list-error" | "vizier-list-unavailable" | "vizier-error";
  error?: string;
  ownerState?: "live" | "archived" | "deleted" | "missing";
  liveSuccessors?: number;
  vizierStatus?: number;
};
export type ReportReroute = (detail: RerouteDiagnostic) => void;

/** One bounded message line, never a stack or credentials from an SDK error. */
export function rerouteError(err: unknown): string {
  const message = typeof err === "string" ? err
    : err && typeof err === "object" && "message" in err && typeof err.message === "string" ? err.message : "lookup failed";
  return message.trim().split(/[\r\n]/, 1)[0]!.trim()
    .replace(/https?:\/\/\S+/gi, "[url]")
    .replace(/\bBearer\s+\S+/gi, "Bearer [redacted]")
    .replace(/\b([\w-]*(?:token|password|secret|api[_-]?key|authorization))\s*[:=].*$/gi, "$1=[redacted]")
    .slice(0, 240);
}

/** Same owner / unique live title-successor / vizier order as the thread-only caller. */
export async function resolveWakeTarget(
  owner: string | null,
  list: () => Promise<ThreadRow[]>,
  vizier: () => Promise<VizierResolution | null | undefined>,
  report?: ReportReroute,
): Promise<string | null> {
  const live = (r: ThreadRow) => r.archivedAt == null && r.deletedAt == null;
  let stage: "thread-list-error" | "vizier-error" = "thread-list-error";
  try {
    const rows = await list();
    const mine = owner ? rows.find(r => r.id === owner) : undefined;
    if (mine && live(mine)) return mine.id;
    let successors = 0;
    if (mine?.title) {
      const succ = rows.filter(r => live(r) && r.title === mine.title);
      successors = succ.length;
      if (succ.length === 1) return succ[0]!.id;
    }
    stage = "vizier-error";
    const v = await vizier();
    if (v && v.ok) return v.id;
    report?.({ reason: v && !v.ok && v.status === 503 ? "vizier-list-unavailable" : "no-live-target",
      ownerState: !mine ? "missing" : mine.deletedAt != null ? "deleted" : "archived", liveSuccessors: successors,
      ...(v && !v.ok ? { vizierStatus: v.status } : {}) });
    return null;
  } catch (err) {
    report?.({ reason: stage, error: rerouteError(err) });
    return null;
  }
}
