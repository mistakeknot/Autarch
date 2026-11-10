// A hold's end (bead mk-j5ipa.11): every new hold names the date or the event that ends it. Pure helpers, no I/O,
// shared by the service (which stores and expires holds) and the screens (which show the end).

/** A hold ends at a date (an ISO date or a full ISO time) or when a named event is reported. Exactly one. */
export type HoldEnd = { until: string } | { event: { kind: string; ref: string; label: string } };

const DAY_MS = 86_400_000;
const DATE_ONLY = /^\d{4}-\d{2}-\d{2}$/;
const ISO_TIME = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(Z|[+-]\d{2}:\d{2})$/;

/** True when the YYYY-MM-DD names a day that exists: Date.parse would roll 2027-02-29 over to March 2. */
function realDay(day: string): boolean {
  const [y, m, d] = day.split("-").map(Number) as [number, number, number];
  const t = new Date(Date.UTC(y, m - 1, d));
  return t.getUTCFullYear() === y && t.getUTCMonth() === m - 1 && t.getUTCDate() === d;
}

/** The moment an `until` value stops holding: a date alone holds through that whole UTC day; a full time is exact. NaN when unreadable. */
export function untilMs(until: string): number {
  const t = ISO_TIME.exec(until);
  if (!DATE_ONLY.test(until) && !t) return NaN;
  if (!realDay(t ? t[1]! : until)) return NaN;
  if (t && (Number(t[2]) > 23 || Number(t[3]) > 59 || Number(t[4] ?? 0) > 59)) return NaN;
  const ms = Date.parse(until);
  if (Number.isNaN(ms)) return NaN;
  return DATE_ONLY.test(until) ? ms + DAY_MS : ms;
}

const KIND = /^[a-z0-9][a-z0-9-]{0,39}$/;
const oneLine = (s: string, max: number) => s.trim() !== "" && s.length <= max && !/[\r\n]/.test(s);

/** What mk (or the vizier) typed, checked for a NEW hold. A plain sentence says what to pass. */
export function checkHoldEnd(end: unknown, nowMs: number): { ok: true; end: HoldEnd } | { ok: false; error: string } {
  const e = end as { until?: unknown; event?: unknown } | null;
  if (!e || typeof e !== "object") return { ok: false, error: "a hold needs its end: pass --until DATE (for example 2026-11-14), or an event with --event-kind, --event-ref and --event-label" };
  const hasUntil = e.until !== undefined;
  const hasEvent = e.event !== undefined;
  if (hasUntil && hasEvent) return { ok: false, error: "name one end: a date (--until) or an event, not both" };
  if (hasUntil) {
    if (typeof e.until !== "string") return { ok: false, error: "--until is a date such as 2026-11-14 or a full time such as 2026-11-14T18:00:00Z" };
    const ms = untilMs(e.until);
    if (Number.isNaN(ms)) return { ok: false, error: "--until is a date such as 2026-11-14 or a full time such as 2026-11-14T18:00:00Z" };
    if (ms <= nowMs) return { ok: false, error: "--until is already past; name a date that is still ahead" };
    return { ok: true, end: { until: e.until } };
  }
  if (hasEvent) {
    const v = e.event as { kind?: unknown; ref?: unknown; label?: unknown } | null;
    if (!v || typeof v !== "object" || typeof v.kind !== "string" || typeof v.ref !== "string" || typeof v.label !== "string") return { ok: false, error: "an event end needs --event-kind, --event-ref and --event-label" };
    if (!KIND.test(v.kind)) return { ok: false, error: "--event-kind is lowercase words joined by hyphens, such as pr-merge" };
    if (!oneLine(v.ref, 200)) return { ok: false, error: "--event-ref is one line of at most 200 characters" };
    if (!oneLine(v.label, 200)) return { ok: false, error: "--event-label is one line of at most 200 characters; it is what mk reads on the card" };
    return { ok: true, end: { event: { kind: v.kind, ref: v.ref.trim(), label: v.label.trim() } } };
  }
  return { ok: false, error: "a hold needs its end: pass --until DATE (for example 2026-11-14), or an event with --event-kind, --event-ref and --event-label" };
}

/** A stored end, read leniently: anything that is not exactly one readable end is "no end" (the hold stays, never released). */
export function parseStoredEnd(raw: unknown): HoldEnd | undefined {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return undefined;
  const e = raw as { until?: unknown; event?: unknown };
  if (e.until !== undefined && e.event !== undefined) return undefined;
  if (typeof e.until === "string") return Number.isNaN(untilMs(e.until)) ? undefined : { until: e.until };
  const v = e.event as { kind?: unknown; ref?: unknown; label?: unknown } | null | undefined;
  if (v && typeof v === "object" && typeof v.kind === "string" && v.kind !== "" && typeof v.ref === "string" && typeof v.label === "string") return { event: { kind: v.kind, ref: v.ref, label: v.label } };
  return undefined;
}

/** When a dated end passes, in ms; null for an event end or no end (those are never released by time). */
export function endMs(end: HoldEnd | undefined): number | null {
  return end && "until" in end ? untilMs(end.until) : null;
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** The end as one short phrase for a card: "until Nov 14", "until PR 114 merges", or "no end set" for an old hold. */
export function holdEndPhrase(end: HoldEnd | null | undefined, nowMs: number): string {
  const e = parseStoredEnd(end);
  if (!e) return "no end set";
  if ("event" in e) return `until ${e.event.label.trim() !== "" ? e.event.label : `${e.event.kind} ${e.event.ref}`}`;
  const d = new Date(Date.parse(e.until));
  const sameYear = d.getUTCFullYear() === new Date(nowMs).getUTCFullYear();
  return `until ${MONTHS[d.getUTCMonth()]} ${d.getUTCDate()}${sameYear ? "" : `, ${d.getUTCFullYear()}`}`;
}
