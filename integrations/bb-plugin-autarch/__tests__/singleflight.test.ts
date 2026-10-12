import { describe, expect, it } from "vitest";
import { singleFlight } from "../ui/singleflight.js";

/** A read the test finishes by hand, recording which reads started and in what order. */
function harness() {
  const started: number[] = [];
  const finish: Array<(fail?: boolean) => void> = [];
  const go = singleFlight(() => {
    started.push(started.length);
    return new Promise<void>((resolve, reject) => finish.push((fail) => (fail ? reject(new Error("x")) : resolve())));
  });
  const tick = () => new Promise((r) => setTimeout(r, 0));
  return { go, started, finish, tick };
}

describe("singleFlight", () => {
  it("runs one read at a time and queues a single read for any number of calls made meanwhile", async () => {
    const { go, started, finish, tick } = harness();
    go();
    go();
    go();
    go();
    expect(started).toEqual([0]);
    finish[0]();
    await tick();
    expect(started).toEqual([0, 1]);
    finish[1]();
    await tick();
    expect(started).toEqual([0, 1]);
  });
  it("a read slower than the poll still lands, and the poll keeps reading after it", async () => {
    const { go, started, finish, tick } = harness();
    for (let i = 0; i < 5; i++) go(); // poll ticks while the first read is slow
    finish[0]();
    await tick();
    go(); // a tick during the queued read
    finish[1]();
    await tick();
    expect(started).toEqual([0, 1, 2]);
  });
  it("a failed read does not stop later reads", async () => {
    const { go, started, finish, tick } = harness();
    go();
    finish[0](true);
    await tick();
    go();
    expect(started).toEqual([0, 1]);
  });
});
