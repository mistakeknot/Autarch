// Straight policy port of quota-governor (bead mk-z0lht.1). IO lives in governor.ts.
export type Window = { windowMinutes?: number; utilization?: number | null; resetAt?: number | null; observedAt?: number | null };
export type Account = { id: string; provider: string; enabled: boolean; status: string; observedAt?: number | null; heldUntil?: number | null; usageRestriction?: unknown; sevenDayUtilization?: number | null; sevenDayResetAt?: number | null; limitWindows?: Window[] | null };
export type Samples = Record<string, [number, number, number][]>;
export type Operator = { per_coordinator_cap: number; host_ceiling: number; top_rung: number; priority_host_ceiling: number; overrides: Record<string, number> };
export type Level = { level: number; name: string; enter: number | null; exit: number | null; operator: Operator; bb_limit: number | "auto"; allowed_max: number | null; cuts_protected: boolean };
export type Config = typeof DEFAULTS;
export const DEFAULTS = {
  // Host and private locations are supplied at runtime, never embedded in the plugin.
  bb_host: "", state_dir: "", operator_toml: "", storage_root: "", card_project: "",
  stale_ms: 30 * 60 * 1000, pace_mode: "recent", recent_window_h: 3, min_span_h: 2,
  min_hours_to_reset: 1, same_window_ms: 10 * 60 * 1000, min_elapsed: .05,
  good_readings: 2, card_hold_declined_s: 12 * 3600,
  levels: [
    { level: 0, name: "open", enter: null, exit: null, operator: { per_coordinator_cap: 2, host_ceiling: 6, top_rung: 4, priority_host_ceiling: 14, overrides: { "Shadow Work |": 8, "Undersketch |": 5 } }, bb_limit: "auto", allowed_max: null, cuts_protected: false },
    { level: 1, name: "throttle", enter: 1.10, exit: .95, operator: { per_coordinator_cap: 1, host_ceiling: 4, top_rung: 2, priority_host_ceiling: 6, overrides: { "Shadow Work |": 1, "Undersketch |": 5 } }, bb_limit: "auto", allowed_max: 1, cuts_protected: false },
    { level: 2, name: "severe", enter: 1.35, exit: 1.20, operator: { per_coordinator_cap: 1, host_ceiling: 3, top_rung: 1, priority_host_ceiling: 4, overrides: { "Shadow Work |": 1, "Undersketch |": 3 } }, bb_limit: 12, allowed_max: 0, cuts_protected: true },
  ] as Level[],
  recipient_title: "\\| coordinator|steward", recipient_exclude: "^Masaq' \\| (vizier|mk channel|intercom)", vizier_title: "^Masaq' \\| vizier",
};
const WEEK = 168 * 3600000;
export const num = (x: unknown): x is number => typeof x === "number" && Number.isFinite(x);
export function usable(a: Account, now: number, stale: number): boolean {
  return a.provider === "claude" && a.enabled && a.status === "ready" && !(a.heldUntil && a.heldUntil > now) && !a.usageRestriction && a.observedAt != null && now - a.observedAt <= stale;
}
export function weekly(a: Account, now: number): [number, number | null] | null {
  const wins: Window[] = [];
  if (a.sevenDayUtilization != null) wins.push({ utilization: a.sevenDayUtilization, resetAt: a.sevenDayResetAt, windowMinutes: 10080 });
  wins.push(...(a.limitWindows ?? []));
  for (const w of wins) if (w.utilization != null && w.windowMinutes === 10080) return [w.utilization, w.resetAt ? Math.max(0, Math.min(1, 1 - (w.resetAt - now) / WEEK)) : null];
  return null;
}
export function weeklyReset(a: Account): number | null {
  if (a.sevenDayUtilization != null && a.sevenDayResetAt) return a.sevenDayResetAt;
  return a.limitWindows?.find(w => w.utilization != null && w.windowMinutes === 10080)?.resetAt ?? null;
}
export function weeklyObserved(a: Account): number | null {
  return a.limitWindows?.find(w => w.windowMinutes === 10080 && num(w.observedAt))?.observedAt ?? a.observedAt ?? null;
}
export type Part = { id: string; rate: number; sustainable: number; span_h: number };
export function measurePace(accounts: Account[], now: number, cfg: Config, samples: Samples, parts: Part[] = []): [number | null, string] {
  let skipped = 0;
  if (cfg.pace_mode === "cumulative") {
    const timed: [number, number][] = [];
    for (const a of accounts) {
      if (a.provider !== "claude" || !a.enabled) continue;
      if (!usable(a, now, cfg.stale_ms)) { skipped++; continue; }
      const w = weekly(a, now); if (w && w[1] != null) timed.push([w[0], w[1]]);
    }
    if (!timed.length) return [null, `no usable account with a timed weekly window (skipped ${skipped})`];
    return [timed.reduce((s, w) => s + w[0], 0) / Math.max(timed.reduce((s, w) => s + w[1], 0), cfg.min_elapsed * timed.length), `${timed.length} usable timed, ${skipped} skipped`];
  }
  let rate = 0, sust = 0, used = 0, nosamples = 0;
  for (const a of accounts) {
    if (a.provider !== "claude" || !a.enabled) continue;
    if (!usable(a, now, cfg.stale_ms)) { skipped++; continue; }
    const w = weekly(a, now), reset = weeklyReset(a);
    if (!w || reset == null || reset <= now) { skipped++; continue; }
    const u = w[0], obs = weeklyObserved(a);
    if (!num(obs) || obs < now - cfg.recent_window_h * 3600000 || obs > now) { nosamples++; continue; }
    const pts: [number, number][] = (samples[a.id] ?? []).filter(p => Math.abs(p[2] - reset) <= cfg.same_window_ms).map(p => [p[0], p[1]]);
    pts.push([obs, u]); pts.sort((a, b) => a[0] - b[0] || a[1] - b[1]);
    const span = (pts[pts.length - 1][0] - pts[0][0]) / 3600000;
    if (span < cfg.min_span_h) { nosamples++; continue; }
    const r = Math.max(0, pts[pts.length - 1][1] - pts[0][1]) / span;
    const s = Math.max(0, 1 - u) / Math.max(cfg.min_hours_to_reset, (reset - now) / 3600000);
    rate += r; sust += s; used++; parts.push({ id: a.id, rate: r, sustainable: s, span_h: span });
  }
  const note = `${used} with a recent rate, ${nosamples} without enough samples, ${skipped} skipped`;
  if (!used) return [null, `no usable account with ${cfg.min_span_h} h of samples in its weekly window (${note}): hold`];
  if (sust <= 0) return [null, `no remaining capacity on any account with samples (${note}): hold`];
  return [rate / sust, `${note}; recent ${(rate * 100).toFixed(2)} pts/h over sustainable ${(sust * 100).toFixed(2)} pts/h`];
}
export function choose(cur: number, pace: number | null, streak: number, cfg: Config): [number, number, string] {
  if (pace == null) return [cur, streak, "pace unknown: hold"];
  const enter = Math.max(0, ...cfg.levels.filter(l => l.enter != null && pace >= l.enter).map(l => l.level));
  if (enter > cur) return [enter, 0, `pace ${pace.toFixed(2)} reached level ${enter}`];
  const below = cfg.levels[cur].exit;
  if (cur > 0 && below != null && pace < below) {
    streak++;
    return streak >= cfg.good_readings ? [cur - 1, 0, `pace ${pace.toFixed(2)} below ${below.toFixed(2)} for ${streak} readings`] : [cur, streak, `pace ${pace.toFixed(2)} below exit ${below.toFixed(2)} (${streak} of ${cfg.good_readings} readings)`];
  }
  return [cur, 0, `pace ${pace.toFixed(2)} holds level ${cur}`];
}
export type Floor = { level: number; until: string; source: string };
// datetime.fromisoformat accepts basic/extended calendar and ISO-week dates,
// optional time, and colonless offsets. Validate components instead of relying
// on Date.parse's permissive rollover of invalid dates and rejection of basic ISO.
export function pythonIsoMillis(text: string): number | null {
  const date = /^(\d{4})(?:-(\d{2})-(\d{2})|(\d{2})(\d{2})|-?W(\d{2})(?:-?([1-7]))?)/.exec(text);
  if (!date) return null;
  const year = Number(date[1]);
  if (!year) return null;
  const utc = (y: number, m: number, d: number) => { const v = new Date(0); v.setUTCFullYear(y, m - 1, d); v.setUTCHours(0, 0, 0, 0); return v; };
  let day: Date;
  if (date[6]) {
    const week = Number(date[6]), weekday = Number(date[7] ?? 1);
    if (week < 1 || week > 53) return null;
    const jan4 = utc(year, 1, 4);
    day = new Date(jan4.getTime() + ((week - 1) * 7 + weekday - 1 - (jan4.getUTCDay() + 6) % 7) * 86400000);
    const thursday = new Date(day.getTime() + (4 - (day.getUTCDay() || 7)) * 86400000);
    if (thursday.getUTCFullYear() !== year) return null;
  } else {
    const month = Number(date[2] ?? date[4]), dom = Number(date[3] ?? date[5]);
    day = utc(year, month, dom);
    if (day.getUTCFullYear() !== year || day.getUTCMonth() !== month - 1 || day.getUTCDate() !== dom) return null;
  }
  const rest = text.slice(date[0].length);
  if (!rest) return day.getTime();
  // Python permits any single separator between the date and time.
  const time = /^(\d{2})(?:(:?)(\d{2})(?:\2(\d{2}))?)?(?:[.,](\d+))?(Z|[+-]\d{2}(?::?\d{2}(?::?\d{2})?)?(?:[.,]\d+)?)?$/.exec(rest.slice([...rest][0].length));
  if (!time) return null;
  const h = Number(time[1]), m = Number(time[3] ?? 0), s = Number(time[4] ?? 0);
  if (h > 23 || m > 59 || s > 59) return null;
  let offset = 0;
  if (time[6] && time[6] !== "Z") {
    const z = /^([+-])(\d{2})(?::?(\d{2})(?::?(\d{2}))?)?(?:[.,](\d+))?$/.exec(time[6])!;
    // Python normalizes minute/second overflow in offsets, but disallows a day or more.
    const seconds = Number(z[2]) * 3600 + Number(z[3] ?? 0) * 60 + Number(z[4] ?? 0);
    offset = seconds === 0 ? 0 : (seconds + Number(`0.${(z[5] ?? "0").slice(0, 6)}`)) * 1000;
    if (offset >= 86400000) return null;
    if (z[1] === "-") offset = -offset;
  }
  return day.getTime() + ((h * 60 + m) * 60 + s) * 1000 + Number(`0.${(time[5] ?? "0").slice(0, 6)}`) * 1000 - offset;
}
export function readFloor(f: unknown, now: number, cfg: Config): Floor | null {
  if (!f || typeof f !== "object") return null;
  const v = f as Floor;
  if (!Number.isInteger(v.level) || v.level < 0 || v.level >= cfg.levels.length || typeof v.until !== "string") return null;
  const until = pythonIsoMillis(v.until);
  if (until === null || now >= until) return null;
  return { level: v.level, until: v.until, source: String(v.source ?? "").slice(0, 120) };
}
export function applyFloor(cur: number, next: number, streak: number, why: string, floor: Floor | null): [number, number, string] {
  if (!floor) return [next, streak, why];
  if (next >= floor.level) return [next, next === floor.level ? 0 : streak, why];
  return [Math.max(cur, floor.level), 0, `${why}; floor level ${floor.level} until ${floor.until} holds it (${floor.source || "no source"})`];
}
export type Thread = { id: string; title?: string | null; providerId?: string; pinnedAt?: unknown; archivedAt?: unknown; deletedAt?: unknown };
export function recipients(threads: Thread[], cfg: Config): string[] {
  return threads.filter(t => !t.archivedAt && !t.deletedAt && t.providerId === "claude-code" && new RegExp(cfg.recipient_title).test(t.title ?? "") && !new RegExp(cfg.recipient_exclude).test(t.title ?? "")).map(t => t.id);
}
export function findVizier(threads: Thread[], cfg: Config): string | null {
  const found = threads.filter(t => !t.archivedAt && !t.deletedAt && t.pinnedAt && new RegExp(cfg.vizier_title).test(t.title ?? ""));
  return found.length === 1 ? found[0].id : null;
}
export function editOperator(text: string, op: Operator): string {
  const want = new Map(Object.entries({ "executors.per_coordinator_cap": op.per_coordinator_cap, "executors.host_ceiling": op.host_ceiling, "executors.priority.top_rung": op.top_rung, "executors.priority.host_ceiling": op.priority_host_ceiling }));
  const ov = new Map(Object.entries(op.overrides).map(([k, v]) => [`"${k}"`, v]));
  const out: string[] = []; let sec = "", hasOv = false;
  const ovSec = "executors.priority.top_rung_overrides";
  const flush = () => { for (const [k, v] of ov) out.push(`${k} = ${v}`); ov.clear(); };
  for (const line of text.split("\n")) {
    const m = /^\s*\[([^\]]+)\]\s*$/.exec(line);
    if (m) { if (sec === ovSec) flush(); sec = m[1].trim(); if (sec === ovSec) hasOv = true; out.push(line); continue; }
    const kv = /^(\s*)("?[^"=\s][^"=]*?"?)\s*=\s*(.*?)\s*$/.exec(line);
    if (kv && !line.trimStart().startsWith("#")) {
      const key = `${sec}.${kv[2]}`;
      if (want.has(key)) { out.push(`${kv[1]}${kv[2]} = ${want.get(key)}`); want.delete(key); continue; }
      if (sec === ovSec && ov.has(kv[2])) { out.push(`${kv[1]}${kv[2]} = ${ov.get(kv[2])}`); ov.delete(kv[2]); continue; }
    }
    out.push(line);
  }
  if (want.size) throw Error(`operator.toml lacks keys: ${[...want.keys()].sort().join(", ")}`);
  if (sec === ovSec || (!hasOv && ov.size)) {
    while (out.at(-1) === "") out.pop();
    if (sec !== ovSec) out.push("", `[${ovSec}]`);
    flush(); out.push("");
  }
  return out.join("\n");
}
