import { describe, expect, it } from "vitest";
import { startCardLookup } from "../ui/cardlink.js";

function deferred() {
  let resolve!: (v: unknown) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<unknown>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}
const tick = () => new Promise((r) => setTimeout(r, 0));

describe("startCardLookup", () => {
  it("asks cardState for the key and reports a found card", async () => {
    const calls: unknown[] = [];
    const got: unknown[] = [];
    startCardLookup((i) => (calls.push(i), Promise.resolve({ found: true, state: "done", title: "T" })), "DEMO-1", (k, r) => got.push([k, r]));
    await tick();
    expect(calls).toEqual([{ key: "DEMO-1" }]);
    expect(got).toEqual([["DEMO-1", { found: true, state: "done", title: "T" }]]);
  });

  it("reports a missing card, and an error", async () => {
    const got: unknown[] = [];
    startCardLookup(() => Promise.resolve({ found: false }), "DEMO-2", (k, r) => got.push([k, r]));
    startCardLookup(() => Promise.reject(new Error("x")), "DEMO-3", (k, r) => got.push([k, r]));
    await tick();
    expect(got).toEqual([["DEMO-2", { found: false }], ["DEMO-3", "error"]]);
  });

  it("drops a result that arrives after the lookup was cancelled (a key switch or a newer poll)", async () => {
    const d = deferred();
    const got: unknown[] = [];
    const cancel = startCardLookup(() => d.promise, "DEMO-4", (k, r) => got.push([k, r]));
    cancel();
    d.resolve({ found: true, state: "done" });
    await tick();
    expect(got).toEqual([]);
  });

  it("drops a late error after cancel", async () => {
    const d = deferred();
    const got: unknown[] = [];
    startCardLookup(() => d.promise, "DEMO-5", (k, r) => got.push([k, r]))();
    d.reject(new Error("late"));
    await tick();
    expect(got).toEqual([]);
  });

  it("a poll that re-runs the lookup asks again, and only its own answer lands", async () => {
    const first = deferred();
    const second = deferred();
    const queue = [first, second];
    let asked = 0;
    const got: unknown[] = [];
    const call = () => queue[asked++].promise;
    const cancelFirst = startCardLookup(call, "DEMO-6", (k, r) => got.push([k, r]));
    cancelFirst();
    startCardLookup(call, "DEMO-6", (k, r) => got.push([k, r]));
    first.resolve({ found: true, state: "open", title: "stale" });
    second.resolve({ found: true, state: "done", title: "fresh" });
    await tick();
    expect(asked).toBe(2);
    expect(got).toEqual([["DEMO-6", { found: true, state: "done", title: "fresh" }]]);
  });

  it("defaults a missing state and title", async () => {
    const got: unknown[] = [];
    startCardLookup(() => Promise.resolve({ found: true }), "DEMO-7", (k, r) => got.push([k, r]));
    await tick();
    expect(got).toEqual([["DEMO-7", { found: true, state: "unknown", title: "" }]]);
  });
});
