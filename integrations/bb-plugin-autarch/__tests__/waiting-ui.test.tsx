// The one count on screen (bead mk-yjp7): the strip, the tab and the sidebar badge all read the same
// `waiting` figure, and the strip says what it counts so a mismatch with another list can be explained.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import type { Waiting } from "../waiting.js";
import { badgeCount, WaitingStrip, waitingSummary } from "../ui/waiting.js";
import { homeTabs, HomeTabs } from "../ui/tabs.js";

const W: Waiting = {
  total: 7,
  decide: 5,
  moves: 1,
  notices: 1,
  noticeItems: [{ item: "vizier-adopted:3", at: "2026-10-07T12:00:00Z", text: "Home adopted thr_viz as the vizier thread; delegated rulings stay suspended until you see this." }],
  updates: 3,
  held: 2,
  later: 0,
  suspended: true,
  definition: "Counts open decisions (not on hold), open moves nobody has claimed, and notices you must acknowledge.",
};
const noop = () => {};

describe("waiting strip", () => {
  it("leads with the total and splits it into its parts, then names what is beside it", () => {
    expect(waitingSummary(W)).toEqual({
      headline: "Waiting on you: 7",
      parts: "5 to decide, 1 move, 1 notice",
      beside: ["3 updates to read", "2 on hold"],
    });
    const html = renderToStaticMarkup(<WaitingStrip waiting={W} onJump={noop} />);
    expect(html).toContain("Waiting on you: 7");
    expect(html).toContain("5 to decide, 1 move, 1 notice");
    expect(html).toContain("3 updates to read");
    expect(html).toContain("2 on hold");
  });

  it("prints the definition so a mismatch with another list is explainable", () => {
    const html = renderToStaticMarkup(<WaitingStrip waiting={W} onJump={noop} />);
    expect(html).toContain("What this counts");
    expect(html).toContain("not on hold");
  });

  it("singular and zero read correctly", () => {
    expect(waitingSummary({ ...W, total: 1, decide: 1, moves: 0, notices: 0, updates: 1, held: 0 })).toEqual({ headline: "Waiting on you: 1", parts: "1 to decide", beside: ["1 update to read"] });
    const s = waitingSummary({ ...W, total: 0, decide: 0, moves: 0, notices: 0, updates: 0, held: 0 });
    expect(s.headline).toBe("Waiting on you: 0");
    expect(s.parts).toBe("nothing");
    expect(s.beside).toEqual([]);
  });
});

describe("every surface shows the same number", () => {
  it("the badge is the total", () => {
    expect(badgeCount(W, { blocked: false, unowned: false })).toBe("7");
  });
  it("the badge keeps '!' for a blocked serve or an unowned machine blocker, and hides at zero", () => {
    expect(badgeCount(W, { blocked: true, unowned: false })).toBe("!");
    expect(badgeCount(W, { blocked: false, unowned: true })).toBe("!");
    expect(badgeCount({ ...W, total: 0 }, { blocked: false, unowned: false })).toBeNull();
  });
  it("the Queue tab carries the total", () => {
    const html = renderToStaticMarkup(<HomeTabs classic={false} waiting={W.total} onOpen={noop} onToggleClassic={noop} onTodos={noop} />);
    expect(html).toContain("Queue (7)");
    expect(homeTabs(false)[0]![2]).toBe("Queue");
  });
  it("the Ideas tab sits between Queue and Vizier and carries the open-idea count", () => {
    expect(homeTabs(false).map((t) => t[2])).toEqual(["Queue", "Ideas", "Vizier", "Map", "Settings"]);
    const html = renderToStaticMarkup(<HomeTabs classic={false} waiting={W.total} ideas={3} onOpen={noop} onToggleClassic={noop} onTodos={noop} />);
    expect(html).toContain("Ideas (3)");
    expect(renderToStaticMarkup(<HomeTabs classic={false} onOpen={noop} onToggleClassic={noop} onTodos={noop} />)).not.toContain("Ideas (");
  });
});
