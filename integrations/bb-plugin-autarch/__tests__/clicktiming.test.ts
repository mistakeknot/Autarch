import { afterEach, describe, expect, it, vi } from "vitest";
import { clickStarted, paintDone, setTimingSink, timed, type ClickTiming } from "../ui/clicktiming.js";

afterEach(() => setTimingSink(null));

describe("click timing (bead mk-yaq27)", () => {
  it("timed reports how long an RPC took, returns its value, and reports a failure too", async () => {
    const seen: ClickTiming[] = [];
    setTimingSink((t) => seen.push(t));
    expect(await timed("conversation", new Promise((r) => setTimeout(() => r(7), 30)))).toBe(7);
    await expect(timed("setViewing", Promise.reject(new Error("x")))).rejects.toThrow("x");
    await vi.waitFor(() => expect(seen.map((t) => t.kind)).toEqual(["conversation", "setViewing"]));
    expect(seen[0]!.ms).toBeGreaterThanOrEqual(25);
  });

  it("paint is reported once per click and never without one; a throwing sink does not break the click", async () => {
    const seen: ClickTiming[] = [];
    setTimingSink((t) => seen.push(t));
    paintDone();
    clickStarted();
    paintDone();
    paintDone();
    await vi.waitFor(() => expect(seen).toHaveLength(1));
    expect(seen[0]!.kind).toBe("paint");
    setTimingSink(() => {
      throw new Error("sink");
    });
    clickStarted();
    expect(() => paintDone()).not.toThrow();
  });
});
