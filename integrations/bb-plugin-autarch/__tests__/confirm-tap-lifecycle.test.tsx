// Runs ConfirmTap's real hooks (arm, timeout, blur, key change, unmount) against a tiny hook runtime, since no DOM
// library is installed here, then checks each call site hands ConfirmTap the right inputs. Bead mk-qrp5d.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// A minimal hook runtime: state slots and effects kept per mounted instance, re-rendered synchronously on setState.
const rt = vi.hoisted(() => ({ slots: [] as unknown[], effects: [] as { deps?: unknown[]; cleanup?: (() => void) | void }[], i: 0, e: 0, rerender: () => {}, active: false }));
vi.mock("react", async (orig) => {
  const actual = await orig<typeof import("react")>();
  return {
    ...actual,
    useState: (init: unknown) => {
      if (!rt.active) return actual.useState(init as never);
      const k = rt.i++;
      if (!(k in rt.slots)) rt.slots[k] = init;
      const set = (v: unknown) => { rt.slots[k] = typeof v === "function" ? (v as (p: unknown) => unknown)(rt.slots[k]) : v; rt.rerender(); };
      return [rt.slots[k], set];
    },
    useEffect: (fn: () => (() => void) | void, deps?: unknown[]) => {
      if (!rt.active) return actual.useEffect(fn, deps);
      const k = rt.e++;
      const prev = rt.effects[k];
      const same = prev && deps && prev.deps && deps.every((d, n) => Object.is(d, prev.deps![n]));
      if (same) return;
      queueMicrotaskSync(() => { if (typeof prev?.cleanup === "function") prev.cleanup(); rt.effects[k] = { deps, cleanup: fn() }; });
    },
  };
});
const pending: (() => void)[] = [];
function queueMicrotaskSync(f: () => void) { pending.push(f); }

import { CONFIRM_MS, ConfirmTap, type ConfirmProps } from "../ui/confirmtap.js";

type Props = { needsConfirm: boolean; resetKey?: string; onConfirmed: () => void };

function mount(initial: Props) {
  rt.slots = []; rt.effects = []; let props = initial; let last!: ConfirmProps; rt.active = true;
  let gen = 0;
  const render = () => {
    rt.i = 0; rt.e = 0;
    // A setState during render starts a newer render; like React, drop what the abandoned outer render produced.
    const mine = ++gen;
    ConfirmTap({ ...props, children: (p: ConfirmProps) => { if (mine === gen) last = p; return null; } });
    // ConfirmTap's own setState-during-render (key change) re-renders until stable.
    const todo = pending.splice(0); todo.forEach((f) => f());
  };
  rt.rerender = render; render();
  return {
    get p() { return last; },
    setProps(next: Partial<Props>) { props = { ...props, ...next }; render(); },
    unmount() { rt.effects.forEach((x) => typeof x?.cleanup === "function" && x.cleanup()); rt.active = false; },
  };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => { rt.active = false; vi.useRealTimers(); });

describe("ConfirmTap lifecycle", () => {
  it("the first tap arms and runs nothing; the second runs once and disarms", () => {
    const run = vi.fn(); const m = mount({ needsConfirm: true, resetKey: "a", onConfirmed: run });
    expect(m.p.armed).toBe(false);
    m.p.onClick(); expect(m.p.armed).toBe(true); expect(run).not.toHaveBeenCalled();
    m.p.onClick(); expect(m.p.armed).toBe(false); expect(run).toHaveBeenCalledTimes(1);
    m.unmount();
  });

  it("an action that needs no confirmation runs on the first tap", () => {
    const run = vi.fn(); const m = mount({ needsConfirm: false, onConfirmed: run });
    m.p.onClick(); expect(run).toHaveBeenCalledTimes(1); expect(m.p.armed).toBe(false);
    m.unmount();
  });

  it("disarms after CONFIRM_MS, not before, and the next tap re-arms instead of running", () => {
    const run = vi.fn(); const m = mount({ needsConfirm: true, onConfirmed: run });
    m.p.onClick();
    vi.advanceTimersByTime(CONFIRM_MS - 1); expect(m.p.armed).toBe(true);
    vi.advanceTimersByTime(1); expect(m.p.armed).toBe(false);
    m.p.onClick(); expect(run).not.toHaveBeenCalled(); expect(m.p.armed).toBe(true);
    m.unmount();
  });

  it("disarms on blur", () => {
    const run = vi.fn(); const m = mount({ needsConfirm: true, onConfirmed: run });
    m.p.onClick(); m.p.onBlur(); expect(m.p.armed).toBe(false);
    m.p.onClick(); expect(run).not.toHaveBeenCalled();
    m.unmount();
  });

  it("disarms when the card or revision changes, and a tap then arms rather than records", () => {
    const run = vi.fn(); const m = mount({ needsConfirm: true, resetKey: "d1|r1", onConfirmed: run });
    m.p.onClick(); expect(m.p.armed).toBe(true);
    m.setProps({ resetKey: "d1|r2" }); expect(m.p.armed).toBe(false);
    m.p.onClick(); expect(run).not.toHaveBeenCalled();
    m.unmount();
  });

  it("clears its timer on unmount", () => {
    const m = mount({ needsConfirm: true, onConfirmed: vi.fn() });
    m.p.onClick(); expect(vi.getTimerCount()).toBe(1);
    m.unmount(); expect(vi.getTimerCount()).toBe(0);
  });
});

