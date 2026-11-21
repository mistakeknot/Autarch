import { afterEach, describe, expect, it, vi } from "vitest";
import { execFile } from "node:child_process";
import { readSubjectHead, type PickSubject } from "../pickhead.js";

vi.mock("node:child_process", () => ({
  execFile: vi.fn((_file: string, _argv: string[], _options: unknown, callback: (error: Error | null, stdout: string) => void) => {
    callback(null, JSON.stringify({ state: "OPEN", headRefOid: "a".repeat(40) }));
  }),
}));

const T0 = Date.parse("2026-01-01T00:00:00.000Z");
const sha = "a".repeat(40);
const subject: PickSubject = { kind: "github-pr", repo: "example/project", number: 12, head: sha };
const deadlineAt = new Date(T0 + 120_000).toISOString();
const now = () => T0;

afterEach(() => vi.useRealTimers());
describe("bounded subject head evidence", () => {
  it("passes the shrinking deadline through the audited helper to execFile", async () => {
    vi.mocked(execFile).mockClear();
    let clock = T0;
    const options = { deadlineAt: new Date(T0 + 1500).toISOString(), now: () => clock };
    expect(await readSubjectHead(subject, options)).toMatchObject({ result: "pass" });
    clock += 750;
    expect(await readSubjectHead(subject, options)).toMatchObject({ result: "pass" });
    for (const [index, timeout] of [1500, 750].entries()) {
      expect(execFile).toHaveBeenNthCalledWith(index + 1, "gh", ["pr", "view", "12", "--repo", "example/project", "--json", "state,headRefOid"], {
        timeout, maxBuffer: 65536,
        env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
      }, expect.any(Function));
    }
  });
  it("production adapter uses fixed gh argv, bounded output and no shell", async () => {
    expect(await readSubjectHead(subject, { deadlineAt, now })).toMatchObject({ result: "pass" });
    expect(execFile).toHaveBeenCalledWith("gh", ["pr", "view", "12", "--repo", "example/project", "--json", "state,headRefOid"], {
      timeout: 20_000, maxBuffer: 65536,
      env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
    }, expect.any(Function));
  });
  it("none is not applicable and never calls a head reader", async () => {
    const read = vi.fn();
    expect(await readSubjectHead({ kind: "none" }, { deadlineAt, now, read })).toMatchObject({ result: "not-applicable", reason: "subject-none", head: null });
    expect(read).not.toHaveBeenCalled();
  });
  it("requires OPEN and the exact full filed SHA", async () => {
    const read = vi.fn().mockResolvedValue({ state: "OPEN", headRefOid: sha });
    expect(await readSubjectHead(subject, { deadlineAt, now, read })).toMatchObject({ result: "pass", reason: "head-unchanged", head: sha, state: "OPEN" });
    expect(read).toHaveBeenCalledWith("example/project", 12, 20_000);
  });
  it.each(["CLOSED", "MERGED"])("refuses %s even with the filed head", async state => {
    expect(await readSubjectHead(subject, { deadlineAt, now, read: async () => ({ state, headRefOid: sha }) })).toMatchObject({ result: "fail", reason: "pr-not-open" });
  });
  it("refuses changed full head", async () => {
    expect(await readSubjectHead(subject, { deadlineAt, now, read: async () => ({ state: "OPEN", headRefOid: "b".repeat(40) }) })).toMatchObject({ result: "fail", reason: "head-changed" });
  });
  it.each([null, {}, { state: "OPEN", headRefOid: "aaaaaaa" }, { state: "OPEN", headRefOid: "A".repeat(40) }, { state: "unknown", headRefOid: sha }])("malformed response is unknown: %j", async response => {
    expect(await readSubjectHead(subject, { deadlineAt, now, read: async () => response })).toMatchObject({ result: "unknown", reason: "head-malformed" });
  });
  it.each([
    { ...subject, repo: "--repo=elsewhere" }, { ...subject, repo: "example/project/path" },
    { ...subject, number: 0 }, { ...subject, number: 1.5 }, { ...subject, number: Number.MAX_SAFE_INTEGER + 1 },
    { ...subject, head: "a".repeat(7) },
  ])("invalid subject never invokes reader: %j", async invalid => {
    const read = vi.fn();
    expect(await readSubjectHead(invalid, { deadlineAt, now, read })).toMatchObject({ result: "unknown", reason: "subject-invalid" });
    expect(read).not.toHaveBeenCalled();
  });
  it("caps the subprocess budget by the remaining absolute deadline", async () => {
    const read = vi.fn().mockResolvedValue({ state: "OPEN", headRefOid: sha });
    await readSubjectHead(subject, { deadlineAt: new Date(T0 + 750).toISOString(), now, read });
    expect(read).toHaveBeenCalledWith("example/project", 12, 750);
  });
  it.each([new Date(T0).toISOString(), new Date(T0 - 1).toISOString(), "invalid"])("does not read at expired/invalid deadline %s", async deadline => {
    const read = vi.fn();
    expect(await readSubjectHead(subject, { deadlineAt: deadline, now, read })).toMatchObject({ result: "unknown", reason: "evaluation-timeout" });
    expect(read).not.toHaveBeenCalled();
  });
  it("bounds a hung reader independently at the original deadline", async () => {
    vi.useFakeTimers(); vi.setSystemTime(T0);
    const pending = readSubjectHead(subject, { deadlineAt: new Date(T0 + 750).toISOString(), read: () => new Promise(() => {}) });
    await vi.advanceTimersByTimeAsync(750);
    expect(await pending).toMatchObject({ result: "unknown", reason: "evaluation-timeout", observed_at: new Date(T0 + 750).toISOString() });
    expect(vi.getTimerCount()).toBe(0);
  });
  it("bounds a hung reader to twenty seconds even with longer allowance", async () => {
    vi.useFakeTimers(); vi.setSystemTime(T0);
    const pending = readSubjectHead(subject, { deadlineAt, read: () => new Promise(() => {}) });
    await vi.advanceTimersByTimeAsync(20_000);
    expect(await pending).toMatchObject({ result: "unknown", reason: "head-timeout" });
  });
  it("refuses a result received at the deadline", async () => {
    let clock = T0;
    const result = await readSubjectHead(subject, { deadlineAt, now: () => clock, read: async () => {
      clock += 120_000;
      return { state: "OPEN", headRefOid: sha };
    } });
    expect(result).toMatchObject({ result: "unknown", reason: "evaluation-timeout" });
  });
  it("reader failure is unknown without leaking subprocess output", async () => {
    expect(await readSubjectHead(subject, { deadlineAt, now, read: async () => { throw new Error("private transport details"); } })).toMatchObject({ result: "unknown", reason: "head-unavailable" });
  });
});
