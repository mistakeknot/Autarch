import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { singleFlight } from "../ui/singleflight.js";

/** Reads the test finishes by hand. Each read applies its value to `state` only while it is live, as the hooks do. */
function harness(timeoutMs = 1000) {
  const finish: Array<(value: string | Error) => void> = [];
  let state = "";
  const f = singleFlight(
    (live) =>
      new Promise<string>((resolve, reject) => finish.push((v) => (v instanceof Error ? reject(v) : resolve(v)))).then((v) => {
        if (live()) state = v;
      }),
    timeoutMs,
  );
  return { f, finish, state: () => state };
}
const flush = () => vi.advanceTimersByTimeAsync(0);

describe("singleFlight", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("runs one read at a time and queues a single read for any number of calls made meanwhile", async () => {
    const { f, finish, state } = harness();
    for (let i = 0; i < 4; i++) f.go();
    expect(finish).toHaveLength(1);
    finish[0]("first");
    await flush();
    expect(state()).toBe("first");
    expect(finish).toHaveLength(2);
    finish[1]("second");
    await flush();
    expect(state()).toBe("second");
    expect(finish).toHaveLength(2);
  });
  it("a read slower than the poll still lands its state, and the queued read lands after it", async () => {
    const { f, finish, state } = harness(60_000);
    f.go();
    for (let i = 0; i < 3; i++) {
      await vi.advanceTimersByTimeAsync(10_000);
      f.go(); // poll ticks while the first read is slow
    }
    finish[0]("slow");
    await flush();
    expect(state()).toBe("slow");
    finish[1]("newer");
    await flush();
    expect(state()).toBe("newer");
  });
  it("a failed read releases the lock and runs the read queued behind it", async () => {
    const { f, finish, state } = harness();
    f.go();
    f.go();
    finish[0](new Error("x"));
    await flush();
    expect(finish).toHaveLength(2);
    finish[1]("after");
    await flush();
    expect(state()).toBe("after");
  });
  it("a read that never settles is given up after the timeout, and its late reply is ignored", async () => {
    const { f, finish, state } = harness(1000);
    f.go();
    f.go();
    await vi.advanceTimersByTimeAsync(1000);
    expect(finish).toHaveLength(2); // the queued read ran
    finish[1]("fresh");
    await flush();
    finish[0]("stale"); // the hung read finally answers
    await flush();
    expect(state()).toBe("fresh");
    f.go();
    expect(finish).toHaveLength(3); // polls keep working
  });
  it("stop drops the reply in flight and starts no queued or new read", async () => {
    const { f, finish, state } = harness();
    f.go();
    f.go();
    f.stop();
    finish[0]("late");
    await vi.advanceTimersByTimeAsync(5000);
    f.go();
    expect(finish).toHaveLength(1);
    expect(state()).toBe("");
  });
});
