import { mkdirSync, statSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { checkIntervalMs, checkRequest, DEFAULT_CHECK_INTERVAL_MS, parseRequest, parseSpan, runnerBusy, updateInfo } from "../updates.js";
import { tmpDir } from "./helpers.js";

const A = "a".repeat(40);
const B = "b".repeat(40);
const status = { installed: A, latest: B, count: 2, subjects: ["one", "two"], checked_at: "2026-10-07T05:00:00Z" };
let t: ReturnType<typeof tmpDir>;
afterEach(() => t?.cleanup());
function dir(files: Record<string, unknown> = {}): string {
  t = tmpDir();
  const d = join(t.dir, "u");
  mkdirSync(d);
  for (const [k, v] of Object.entries(files)) writeFileSync(join(d, k), typeof v === "string" ? v : JSON.stringify(v));
  return d;
}

describe("updateInfo", () => {
  it("shows nothing without a status file or with a malformed one", () => {
    expect(updateInfo(dir(), undefined).status).toBeNull();
    expect(updateInfo(dir({ "status.json": "{nope" }), undefined).available).toBe(false);
    expect(updateInfo(dir({ "status.json": { ...status, latest: "xyz" } }), undefined).status).toBeNull();
  });
  it("offers an update when main is ahead", () => {
    const i = updateInfo(dir({ "status.json": status }), undefined);
    expect(i).toMatchObject({ available: true, pending: false });
  });
  it("offers nothing when installed is latest", () => {
    expect(updateInfo(dir({ "status.json": { ...status, latest: A, count: 0 } }), undefined).available).toBe(false);
  });
  it("is pending after a click until the runner reports that sha", () => {
    const req = JSON.stringify({ sha: B, clicked_at: "2026-10-07T05:01:00Z" });
    expect(updateInfo(dir({ "status.json": status }), req)).toMatchObject({ pending: true, available: false });
    const done = { sha: B, ok: false, finished_at: "2026-10-07T05:05:00Z", message: "rolled back" };
    expect(updateInfo(dir({ "status.json": status, "result.json": done }), req)).toMatchObject({ pending: false });
  });
  it("a successful result settles the request even before status catches up; a failed one stays retryable", () => {
    const req = JSON.stringify({ sha: B, clicked_at: "2026-10-07T05:01:00Z" });
    const ok = { sha: B, ok: true, finished_at: "2026-10-07T05:05:00Z", message: "done" };
    expect(updateInfo(dir({ "status.json": status, "result.json": ok }), req)).toMatchObject({ pending: false, available: false });
    expect(updateInfo(dir({ "status.json": status, "result.json": { ...ok, ok: false } }), req)).toMatchObject({ pending: false, available: true });
  });
  it("a status checked after a successful result takes precedence", () => {
    const ok = { sha: B, ok: true, finished_at: "2026-10-07T04:00:00Z", message: "done" };
    expect(updateInfo(dir({ "status.json": status, "result.json": ok }), undefined).available).toBe(true);
  });
  it("a result with an unparseable date is ignored", () => {
    expect(updateInfo(dir({ "status.json": status, "result.json": { sha: B, ok: true, finished_at: "soon", message: "x" } }), undefined).result).toBeNull();
  });
  it("ignores a result older than the click", () => {
    const req = JSON.stringify({ sha: B, clicked_at: "2026-10-07T05:10:00Z" });
    const old = { sha: B, ok: false, finished_at: "2026-10-07T05:05:00Z", message: "x" };
    expect(updateInfo(dir({ "status.json": status, "result.json": old }), req).pending).toBe(true);
  });
});

describe("checkRequest", () => {
  const info = () => updateInfo(dir({ "status.json": status }), undefined);
  it("accepts only the latest offered sha", () => {
    expect(checkRequest(info(), B, "t")).toEqual({ ok: true, request: { sha: B, clicked_at: "t" } });
    expect(checkRequest(info(), A, "t").ok).toBe(false);
    expect(checkRequest(info(), "HEAD", "t").ok).toBe(false);
  });
  it("refuses with no status or a pending request", () => {
    expect(checkRequest(updateInfo(dir(), undefined), B, "t").ok).toBe(false);
    const p = updateInfo(dir({ "status.json": status }), JSON.stringify({ sha: B, clicked_at: "t" }));
    expect(checkRequest(p, B, "t").ok).toBe(false);
  });
  it("parseRequest rejects junk", () => {
    expect(parseRequest("junk")).toBeNull();
    expect(parseRequest(JSON.stringify({ sha: "x", clicked_at: "t" }))).toBeNull();
  });
});

describe("check interval", () => {
  it("parses systemd spans", () => {
    expect(parseSpan("5min")).toBe(300_000);
    expect(parseSpan("1h 30min")).toBe(5_400_000);
    expect(parseSpan("90")).toBe(90_000);
    expect(parseSpan("1d")).toBe(86_400_000);
    expect(parseSpan("2 weeks")).toBe(1_209_600_000);
    expect(parseSpan("500ms")).toBe(500);
    expect(parseSpan("1500us")).toBe(2);
    expect(parseSpan("1M")).toBe(Math.round(30.44 * 86_400_000));
    expect(parseSpan("1m")).toBe(60_000);
    expect(parseSpan("soon")).toBeNull();
    expect(parseSpan("5 fortnights")).toBeNull();
  });
  it("reads OnUnitActiveSec from the timer file, else the named default", () => {
    const d = dir({ "a.timer": "[Timer]\nOnBootSec=2min\nOnUnitActiveSec=10min\n", "b.timer": "[Timer]\nOnBootSec=2min\n" });
    expect(checkIntervalMs(join(d, "a.timer"))).toBe(600_000);
    expect(checkIntervalMs(join(d, "b.timer"))).toBe(DEFAULT_CHECK_INTERVAL_MS);
    expect(checkIntervalMs(join(d, "missing.timer"))).toBe(DEFAULT_CHECK_INTERVAL_MS);
  });
  it("updateInfo carries the interval, the server time, the log path and the unit", () => {
    const d = dir({ "status.json": status, "t.timer": "OnUnitActiveSec=7min" });
    expect(updateInfo(d, undefined, { now: "2026-10-23T18:32:00Z", timerFile: join(d, "t.timer") })).toMatchObject({ now: "2026-10-23T18:32:00Z", interval_ms: 420_000, log_path: join(d, "runner.log"), unit: "autarch-home-update.service", busy: false });
  });
});

describe("runner lock", () => {
  const lockLine = (d: string) => {
    const st = statSync(join(d, "lock"), { bigint: true });
    const major = ((st.dev >> 8n) & 0xfffn) | ((st.dev >> 32n) << 12n);
    const minor = (st.dev & 0xffn) | ((st.dev >> 12n) & ~0xffn);
    return `3: FLOCK  ADVISORY  WRITE 1985457 ${major.toString(16).padStart(2, "0")}:${minor.toString(16).padStart(2, "0")}:${st.ino} 0 EOF\n`;
  };
  it("is busy only when /proc/locks shows an flock on the lock file's inode", () => {
    const d = dir({ lock: "" });
    writeFileSync(join(d, "held"), lockLine(d));
    writeFileSync(join(d, "other"), "3: FLOCK  ADVISORY  WRITE 1 103:05:1 0 EOF\n4: POSIX  ADVISORY  READ 2 00:00:2 0 EOF\n");
    expect(runnerBusy(join(d, "lock"), join(d, "held"))).toBe(true);
    expect(runnerBusy(join(d, "lock"), join(d, "other"))).toBe(false);
    expect(runnerBusy(join(d, "nolock"), join(d, "held"))).toBe(false);
    expect(runnerBusy(join(d, "lock"), join(d, "no-proc"))).toBe(false);
  });
  it("updateInfo reports busy only for a pending request", () => {
    const d = dir({ "status.json": status, lock: "" });
    writeFileSync(join(d, "held"), lockLine(d));
    const req = JSON.stringify({ sha: B, clicked_at: "2026-10-07T05:01:00Z" });
    const env = { now: "2026-10-07T05:02:00Z", procLocks: join(d, "held") };
    expect(updateInfo(d, req, env).busy).toBe(true);
    expect(updateInfo(d, undefined, env).busy).toBe(false);
  });
});

describe("installed matches result.json at once", () => {
  const ok = { sha: B, ok: true, finished_at: "2026-10-07T05:05:00Z", message: "installed" };
  it("a successful result newer than status.json becomes the installed commit", () => {
    const i = updateInfo(dir({ "status.json": status, "result.json": ok }), undefined);
    expect(i.status).toMatchObject({ installed: B, latest: B, count: 0, subjects: [] });
    expect(i.available).toBe(false);
  });
  it("a status written after the result stands as written", () => {
    const i = updateInfo(dir({ "status.json": status, "result.json": { ...ok, finished_at: "2026-10-07T04:00:00Z" } }), undefined);
    expect(i.status).toMatchObject({ installed: A, count: 2 });
  });
  it("a failed result does not change the installed commit", () => {
    expect(updateInfo(dir({ "status.json": status, "result.json": { ...ok, ok: false } }), undefined).status?.installed).toBe(A);
  });
  it("a result for an older commit than latest moves installed to it and keeps the rest listed", () => {
    const C = "c".repeat(40);
    const i = updateInfo(dir({ "status.json": status, "result.json": { ...ok, sha: C } }), undefined);
    expect(i.status).toMatchObject({ installed: C, latest: B, count: 2 });
  });
});
