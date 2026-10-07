import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({
  ThreadChat: (p: { threadId: string; variant?: string; permissionPolicy?: string }) => (
    <div data-testid="thread-chat" data-thread={p.threadId} data-variant={p.variant} data-policy={p.permissionPolicy} />
  ),
}));

import { OVERLAY_PANEL_ID, OVERLAY_PATH, OverlayPanel, overlayCounts } from "../ui/overlay.js";
import type { AsksData, OwedAsk } from "../ui/asks.js";
import type { CatchupEntry } from "../ui/catchup.js";

const ask = (id = "dec1"): OwedAsk => ({
  id,
  project: "Autarch",
  thread: "thr-a",
  subject: "Ship it?",
  asker: "thread",
  filed_at: "2026-09-29T10:00:00Z",
  revision: "rev1",
  ask: { question: "Which day?", options: [{ id: "day", label: "Day", kind: "instruction", reversible: true, instruction: "go" }] },
});
const data = (over: Partial<AsksData> = {}): AsksData => ({
  owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [],
  delegation: { settings: { vizierThreadId: "thr-viz" }, suspended: false }, machineOwners: {}, ...over,
});
const ruling = (item: string): CatchupEntry => ({ ...entry("delegated", item), decision: item });
const entry = (kind: CatchupEntry["kind"], item: string): CatchupEntry => ({ item, kind, at: "x", text: item });

describe("overlay identity", () => {
  it("has a stable panel id and path that Aleph's validation accepts", () => {
    expect(OVERLAY_PANEL_ID).toBe("home-overlay");
    expect(OVERLAY_PATH).toBe("home-overlay");
    expect(OVERLAY_PANEL_ID).toMatch(/^[A-Za-z0-9_-]{1,64}$/);
    expect(OVERLAY_PATH).toMatch(/^[A-Za-z0-9_-]{1,64}$/);
  });
});

describe("overlay counts", () => {
  it("counts undeliverable wakes and delegated rulings not yet seen", () => {
    const c = overlayCounts(data({ undeliverable: [{ id: "o1", kind: "wake" }, { id: "o2", kind: "wake" }, { id: "o3", kind: "notice" }, { id: "o4", kind: "steps-done" }] }),
      [ruling("d1"), entry("routine", "r1"), ruling("d2"), entry("delegated", "settings")],
    );
    expect(c).toEqual({ undeliverable: 2, delegated: 2 });
  });
});

describe("OverlayPanel", () => {
  const noop = () => {};
  it("renders owed decisions with pick buttons, the compact vizier input and the count strip", () => {
    const html = renderToStaticMarkup(
      <OverlayPanel data={data({ owed: [ask()], undeliverable: [{ id: "o1", kind: "wake" }] })} catchup={[ruling("d1")]} onPick={noop} onOpen={noop} />,
    );
    expect(html).toContain('data-decision="dec1"');
    expect(html).toContain("Day");
    expect(html).toContain('data-testid="thread-chat"');
    expect(html).toContain('data-thread="thr-viz"');
    expect(html).toContain('data-variant="compact"');
    expect(html).toContain('data-overlay-count="undeliverable">1');
    expect(html).toContain('data-overlay-count="delegated">1');
  });
  it("lists only owed decisions: stalled rows are counted, not shown", () => {
    const html = renderToStaticMarkup(
      <OverlayPanel
        data={data({ owed: [ask()], undeliverable: [{ id: "o1", kind: "wake", recipient: "thr-a" }], failures: [{ id: "f1" }], uncertain: [{ id: "u1" }], lane: [{ id: "l1", subject: "Lane", thread: "t", owner: "o", detail: "d", updated_at: "x" }] })}
        catchup={[]}
        onPick={noop}
        onOpen={noop}
      />,
    );
    expect(html).toContain('data-decision="dec1"');
    expect(html).not.toMatch(/Stalled|Undeliverable|Ruling file failed|Uncertain|Waiting|Lane/);
  });
  it("has no map, history or catch-up list", () => {
    const html = renderToStaticMarkup(<OverlayPanel data={data()} catchup={[entry("delegated", "d1")]} onPick={noop} onOpen={noop} />);
    expect(html).not.toMatch(/Map|Catch-up|mark all seen/);
    expect(html).toContain("Nothing needs you.");
  });
  it("says so when no vizier thread is set", () => {
    const html = renderToStaticMarkup(<OverlayPanel data={data({ delegation: { settings: {}, suspended: false } })} catchup={[]} onPick={noop} onOpen={noop} />);
    expect(html).not.toContain("thread-chat");
    expect(html).toContain("No vizier thread is set");
  });
});
