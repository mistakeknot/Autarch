// mk-jb5ul: a vitest worker answers the runner's task-update replies only when its event loop turns. A file of
// synchronous tests (each one a spawnSync) used to run back to back on microtasks without ever turning the loop, so under
// host load a reply sat unread for over 60 s and the worker reported `Timeout calling "onTaskUpdate"` with every test green.
// The shared setup file (__tests__/setup/turn-event-loop.ts) turns the loop once after each test; this proves it does.
import { setTimeout as realSetTimeout } from "node:timers";
import { describe, expect, it } from "vitest";

describe("the event loop turns between tests", () => {
  let ticked = false;
  it("schedules a macrotask and returns without waiting for it", () => {
    realSetTimeout(() => { ticked = true; }, 0);
    expect(ticked).toBe(false);
  });
  it("sees that macrotask already run before this test starts", () => {
    expect(ticked).toBe(true);
  });
});
