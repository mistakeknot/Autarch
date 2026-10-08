import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { checkRequest, parseRequest, updateInfo } from "../updates.js";
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
