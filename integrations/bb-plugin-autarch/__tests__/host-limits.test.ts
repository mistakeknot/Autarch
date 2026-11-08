import { describe, expect, it } from "vitest";
import { THREAD_EVENT_PAGE_CAP, threadEventLimit } from "../hostLimits.js";

describe("threadEventLimit", () => {
  it("holds a page to the host's cap of 100 events", () => {
    expect(THREAD_EVENT_PAGE_CAP).toBe(100);
    expect(threadEventLimit(200)).toBe("100");
    expect(threadEventLimit(500)).toBe("100");
    expect(threadEventLimit(100)).toBe("100");
  });
  it("passes smaller pages through and never goes below 1", () => {
    expect(threadEventLimit(50)).toBe("50");
    expect(threadEventLimit(0)).toBe("1");
    expect(threadEventLimit(-3)).toBe("1");
    expect(threadEventLimit(7.9)).toBe("7");
    expect(threadEventLimit(Number.NaN)).toBe("100");
  });
});
