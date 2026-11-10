// @vitest-environment jsdom
// The phone's 5-second Undo (bead mk-j5ipa.13): a one-tap pick is held on the phone for five seconds under a
// "sending in 5 s · Undo" bar. The pick goes through the existing answer path only when the bar closes; Undo drops it and the
// card is back, unanswered. Driven the way mk drives it: press the real buttons and let the clock run.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { DecideQueue, type AsksData, type OwedAsk } from "../ui/asks.js";
import { MobileInbox } from "../ui/inbox.js";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const NOW = Date.parse("2026-11-09T12:00:00Z");
const ask = (id: string, over: Partial<OwedAsk> = {}): OwedAsk => ({
  id,
  project: "Autarch",
  thread: `thr-${id}`,
  subject: `Subject ${id}`,
  asker: "thread",
  filed_at: "2026-11-09T07:00:00Z",
  revision: `rev-${id}`,
  task_id: `task-${id}`,
  ask: { question: `Question ${id}`, recommendation: "a", options: [{ id: "a", label: "Answer A", kind: "ruling-only", reversible: true }, { id: "b", label: "Answer B", kind: "ruling-only", reversible: true }] },
  ...over,
});
const asks = (owed: OwedAsk[]): AsksData => ({ owed, runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], delegation: { settings: {}, suspended: false }, machineOwners: {} });
const A = ask("a"), B = ask("b"), C = ask("c");

let host: HTMLDivElement;
let root: Root;
beforeEach(() => {
  vi.useFakeTimers();
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.useRealTimers();
});

const q = (sel: string) => host.querySelector(sel) as HTMLElement | null;
const click = async (el: Element | null) => { expect(el).not.toBeNull(); await act(async () => { (el as HTMLElement).click(); }); };
const tick = (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });
const detail = () => q("[data-inbox-detail]")?.getAttribute("data-inbox-detail") ?? null;
const bar = () => q("[data-pick-hold]");

const onPick = vi.fn(() => ({ ok: true }));
const render = (owed: OwedAsk[], initial?: { tab?: "waiting" | "later"; open?: string }, pick: unknown = onPick) =>
  act(() => root.render(<MobileInbox asks={asks(owed)} waiting={owed.filter((o) => !o.later).length} onPick={pick as never} onOpen={() => {}} onNote={() => {}} onLater={() => {}} sinceNode={null} sinceCount={0} onDesktop={() => {}} now={NOW} {...(initial ? { initial } : {})} />));
beforeEach(() => onPick.mockClear());

describe("a one-tap pick on the phone is held for five seconds", () => {
  it("nothing is sent before 5 s, the bar says so at once, and the pick is sent once at 5 s", async () => {
    render([A, B, C], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    expect(onPick).not.toHaveBeenCalled();
    expect(bar()?.textContent).toMatch(/sending in 5 s/);
    expect(bar()?.querySelector("[data-pick-undo]")?.textContent).toBe("Undo");
    await tick(4999);
    expect(onPick).not.toHaveBeenCalled();
    await tick(1);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick).toHaveBeenCalledWith("a", "a", "rev-a");
    expect(bar()).toBeNull();
  });

  it("the bar counts the seconds down", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    await tick(2000);
    expect(bar()?.textContent).toMatch(/sending in 3 s/);
  });

  it("an option tapped on the card is held the same way", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-inbox-detail] [data-option=b]"));
    expect(onPick).not.toHaveBeenCalled();
    expect(bar()).not.toBeNull();
    await tick(5000);
    expect(onPick).toHaveBeenCalledWith("a", "b", "rev-a");
  });

  it("Undo sends nothing, closes the bar and leaves the card unanswered where it was", async () => {
    render([A, B, C], { open: "ask:b" });
    await click(q("[data-bar-main]"));
    await click(q("[data-pick-undo]"));
    await tick(10000);
    expect(onPick).not.toHaveBeenCalled();
    expect(bar()).toBeNull();
    await click(q("[data-inbox-back]"));
    expect(Array.from(host.querySelectorAll("[data-inbox-row]")).map((r) => r.getAttribute("data-inbox-row"))).toEqual(["ask:a", "ask:b", "ask:c"]);
    expect(q("[data-inbox-row='ask:b']")?.textContent).toContain("Decide");
  });

  it("the card stays counted and listed, marked Sending, until the pick is sent", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    await click(q("[data-inbox-back]"));
    expect(q("[data-inbox-title]")?.textContent).toBe("Waiting on you · 2");
    expect(q("[data-inbox-row='ask:a']")?.textContent).toContain("Sending");
    expect(q("[data-inbox-row='ask:b']")?.textContent).toContain("Decide");
  });

  it("the bar survives the move to the next card and sends exactly once", async () => {
    render([A, B, C], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    expect(detail()).toBe("ask:b");
    expect(bar()).not.toBeNull();
    await tick(5000);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(detail()).toBe("ask:b");
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
  });

  it("the bar shows over the list as well", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    await click(q("[data-inbox-back]"));
    expect(bar()).not.toBeNull();
  });

  it("Undo is a 44 px target and the bar does not animate", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    expect(q("[data-pick-undo]")?.className).toContain("min-h-11");
    expect(bar()?.className).not.toMatch(/transition|animate|duration/);
  });
});

describe("the held pick is still sent when the page goes away", () => {
  it("pagehide before 5 s sends it once, and the timer does not send it again", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    await tick(1000);
    act(() => { window.dispatchEvent(new Event("pagehide")); });
    expect(onPick).toHaveBeenCalledTimes(1);
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
  });

  it("the page turning hidden sends it once", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    const set = (v: string) => Object.defineProperty(document, "visibilityState", { configurable: true, get: () => v });
    set("hidden");
    act(() => { document.dispatchEvent(new Event("visibilitychange")); });
    set("visible");
    expect(onPick).toHaveBeenCalledTimes(1);
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
  });

  it("leaving the phone inbox (Full view, unmount) sends it once", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]"));
    act(() => root.render(<p>desktop</p>));
    expect(onPick).toHaveBeenCalledTimes(1);
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
  });
});

describe("a second tap while a pick is held", () => {
  it("on another card sends the first now (it cannot be undone any more) and holds the second", async () => {
    render([A, B, C], { open: "ask:a" });
    await click(q("[data-bar-main]")); // a held; b opens
    await click(q("[data-bar-main]")); // b
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick).toHaveBeenLastCalledWith("a", "a", "rev-a");
    await tick(5000);
    expect(onPick).toHaveBeenCalledTimes(2);
    expect(onPick).toHaveBeenLastCalledWith("b", "a", "rev-b");
  });

  it("on the same card does not replace the first", async () => {
    render([A, B], { open: "ask:a" });
    await click(q("[data-bar-main]")); // a held, b opens
    await click(q("[data-inbox-back]"));
    await click(q("[data-inbox-row='ask:a']"));
    await click(q("[data-inbox-detail] [data-option=b]"));
    await tick(5000);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick).toHaveBeenCalledWith("a", "a", "rev-a");
  });
});

describe("a send that fails", () => {
  it("says so in the bar and keeps the card", async () => {
    render([A, B], { open: "ask:a" }, vi.fn(() => ({ ok: false, error: "stale revision" })));
    await click(q("[data-bar-main]"));
    await tick(5000);
    expect(q("[data-pick-hold-failure]")?.textContent).toContain("stale revision");
    expect(q("[data-pick-undo]")).toBeNull();
  });
});

describe("picks that keep their existing flow", () => {
  it("a destructive option still asks for a second tap and then sends at once, with no bar", async () => {
    const D = ask("d", { ask: { question: "Q", recommendation: "a", options: [{ id: "a", label: "Answer A", kind: "ruling-only", reversible: true }, { id: "x", label: "Reject it", kind: "ruling-only" }] } });
    render([D], { open: "ask:d" });
    await click(q("[data-option=x]"));
    expect(onPick).not.toHaveBeenCalled();
    await click(q("[data-option=x]"));
    expect(onPick).toHaveBeenCalledWith("d", "x", "rev-d");
    expect(bar()).toBeNull();
  });

  it("a confirmed destructive answer on a card with a held pick replaces the held pick: only the later choice is sent", async () => {
    const D = ask("d", { ask: { question: "Q", recommendation: "a", options: [{ id: "a", label: "Answer A", kind: "ruling-only", reversible: true }, { id: "x", label: "Reject it", kind: "ruling-only" }] } });
    render([D], { open: "ask:d" });
    await click(q("[data-option=a]")); // held
    await click(q("[data-option=x]"));
    await click(q("[data-option=x]")); // confirmed: sent at once
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(onPick).toHaveBeenCalledWith("d", "x", "rev-d");
    expect(bar()).toBeNull();
  });

  it("typed words sent on a card with a held pick replace the held pick", async () => {
    render([A], { open: "ask:a" });
    await click(q("[data-option=b]")); // held
    const box = q("[data-inbox-detail] [data-other-text]") as HTMLTextAreaElement;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(box, "my own words");
      box.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await click(q("[data-inbox-detail] [data-other-answer]"));
    await tick(10000);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect((onPick.mock.calls[0] as unknown[] | undefined)?.[1]).toBe("other");
  });

  it("an immediate answer on another card leaves the held pick alone", async () => {
    const D = ask("d", { ask: { question: "Q", recommendation: "a", options: [{ id: "a", label: "Answer A", kind: "ruling-only", reversible: true }, { id: "x", label: "Reject it", kind: "ruling-only" }] } });
    render([A, D], { open: "ask:a" });
    await click(q("[data-bar-main]")); // a held, d opens
    await click(q("[data-option=x]"));
    await click(q("[data-option=x]"));
    expect(onPick).toHaveBeenCalledTimes(1);
    await tick(5000);
    expect(onPick).toHaveBeenCalledTimes(2);
    expect(onPick).toHaveBeenLastCalledWith("a", "a", "rev-a");
  });

  it("typed words sent as the answer go at once, with no bar", async () => {
    render([A], { open: "ask:a" });
    const box = q("[data-inbox-detail] [data-other-text]") as HTMLTextAreaElement;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(box, "my own words");
      box.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await click(q("[data-inbox-detail] [data-other-answer]"));
    await tick(0);
    expect(onPick).toHaveBeenCalledTimes(1);
    expect(bar()).toBeNull();
  });
});

describe("desktop is unchanged", () => {
  it("a desktop pick is sent at once", async () => {
    const pick = vi.fn(() => ({ ok: true }));
    act(() => root.render(<DecideQueue owed={[A, B]} onPick={pick as never} onOpen={() => {}} nowMs={NOW} onLater={(() => {}) as never} />));
    await click(q("[data-option=a]"));
    expect(pick).toHaveBeenCalledTimes(1);
    expect(bar()).toBeNull();
  });
});
