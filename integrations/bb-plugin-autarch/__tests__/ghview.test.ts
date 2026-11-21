import { beforeEach, describe, expect, it, vi } from "vitest";
import { execFile } from "node:child_process";
import { ghHeadReal, ghViewReal } from "../ghview.js";

const response = vi.hoisted(() => ({ stdout: "", error: null as Error | null }));
vi.mock("node:child_process", () => ({
  execFile: vi.fn((_file: string, _argv: string[], _options: unknown, callback: (error: Error | null, stdout: string) => void) => {
    callback(response.error, response.stdout);
  }),
}));
beforeEach(() => {
  vi.clearAllMocks();
  response.error = null;
  response.stdout = JSON.stringify({ state: "OPEN", headRefOid: "a".repeat(40) });
});

describe("audited GitHub readers", () => {
  it.each([[120_000, 20_000], [750, 750]])("bounds head argv and budget %i to %i", async (budget, timeout) => {
    expect(await ghHeadReal("example/project", 12, budget)).toEqual(JSON.parse(response.stdout));
    expect(execFile).toHaveBeenCalledExactlyOnceWith("gh", ["pr", "view", "12", "--repo", "example/project", "--json", "state,headRefOid"], {
      timeout, maxBuffer: 65536,
      env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
    }, expect.any(Function));
  });
  it.each([
    ["--repo=elsewhere", 12], ["example/project/path", 12], ["", 12],
    ["example/project", 0], ["example/project", -1], ["example/project", 1.5],
    ["example/project", Number.MAX_SAFE_INTEGER + 1],
  ])("rejects invalid repo/number before execution: %s %i", async (repo, number) => {
    await expect(ghHeadReal(repo, number, 750)).rejects.toThrow();
    expect(execFile).not.toHaveBeenCalled();
  });
  it.each([0, -1, NaN, Infinity, 0.5])("rejects unsafe timeout %s before execution", async budget => {
    await expect(ghHeadReal("example/project", 12, budget)).rejects.toThrow();
    expect(execFile).not.toHaveBeenCalled();
  });
  it("propagates process failure without fabricating evidence", async () => {
    response.error = new Error("transport failed");
    await expect(ghHeadReal("example/project", 12, 750)).rejects.toThrow("transport failed");
  });
  it("rejects invalid JSON without fabricating evidence", async () => {
    response.stdout = "not JSON";
    await expect(ghHeadReal("example/project", 12, 750)).rejects.toThrow("invalid head response");
  });
  it("preserves the URL reader fields and fixed bounds", async () => {
    response.stdout = JSON.stringify({ state: "MERGED", mergedAt: "2026-01-01T00:00:00Z" });
    const url = "https://github.com/example/project/pull/12";
    expect(await ghViewReal(url)).toEqual(JSON.parse(response.stdout));
    expect(execFile).toHaveBeenCalledExactlyOnceWith("gh", ["pr", "view", url, "--json", "state,mergedAt"], {
      timeout: 20_000, maxBuffer: 65536,
      env: { ...process.env, GH_PROMPT_DISABLED: "1", NO_COLOR: "1" },
    }, expect.any(Function));
  });
  it("preserves URL validation before execution", async () => {
    await expect(ghViewReal("--help")).rejects.toThrow("not a pull request URL");
    expect(execFile).not.toHaveBeenCalled();
  });
});
