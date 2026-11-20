// @vitest-environment jsdom
// The one count on screen (bead mk-yjp7): the strip, the tab and the sidebar badge all read the same
// `waiting` figure, and the strip says what it counts so a mismatch with another list can be explained.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import type { Waiting } from "../waiting.js";
import { badgeCount, railBadge } from "../ui/waiting.js";
import { QueueTabs } from "../ui/tabs.js";
import { homeTabs, HomeOverflow } from "../ui/tabs.js";

const W: Waiting = {
  total: 5,
  line: "5 · 2 on hold",
  decide: 5,
  notices: 1,
  noticeItems: [{ item: "vizier-adopted:3", at: "2026-10-07T12:00:00Z", text: "Citadel adopted thr_viz as the vizier thread; delegated rulings stay suspended until you see this." }],
  updates: 3,
  held: 2,
  later: 0,
  suspended: true,
  definition: "Counts open decisions that need your pick now. Does not count cards on hold or set aside for later, notices to acknowledge, or updates to read; those are named beside it.",
};
const noop = () => {};

describe("merged queue header",()=>{
 it("has no repeated summary line or count-help row",()=>{
  const html=renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={{decide:5,later:0,blocks:2,catchup:3}} more={<HomeOverflow onOpen={noop} definition={W.definition}/>}/>);
  expect(html).not.toContain("data-waiting-line");
  expect(html).toContain("Needs you now (5)");
  expect(html).toContain(W.definition);
  expect(html).toContain("data-home-overflow");
 });
});

describe("every surface shows the same number", () => {
  it("the badge is the decision-card count, not the total", () => {
    expect(badgeCount({ count: 4 }, { blocked: false, unowned: false })).toBe("4");
  });
  it("the badge keeps '!' for a blocked serve or an unowned machine blocker, and hides at zero", () => {
    expect(badgeCount({ count: 4 }, { blocked: true, unowned: false })).toBe("!");
    expect(badgeCount({ count: 4 }, { blocked: false, unowned: true })).toBe("!");
    expect(badgeCount({ count: 0 }, { blocked: false, unowned: false })).toBeNull();
  });
  it("the rail badge (experimental_useBadge) carries the same number, '!' as a warning, null at zero", () => {
    expect(railBadge({ count: 4 }, { blocked: false, unowned: false })).toEqual({ count: 4, label: "4 waiting on you" });
    expect(railBadge({ count: 4 }, { blocked: true, unowned: false })).toEqual({ text: "!", tone: "warning", label: "Citadel needs attention" });
    expect(railBadge({ count: 0 }, { blocked: false, unowned: true })).toEqual({ text: "!", tone: "warning", label: "Citadel needs attention" });
    expect(railBadge({ count: 0 }, { blocked: false, unowned: false })).toBeNull();
  });
  it("the Queue tab leaves counts to its named sub-tabs", () => {
    const html = renderToStaticMarkup(<HomeOverflow onOpen={noop} />);
    expect(html).not.toContain("Queue (");
    expect(homeTabs()[0]![2]).toBe("Queue");
  });
  it("the Ideas tab sits before Archive and Vizier without repeating its panel count", () => {
    expect(homeTabs().map((t) => t[2])).toEqual(["Queue", "Ideas", "Archive", "Vizier", "Settings"]);
    const html = renderToStaticMarkup(<HomeOverflow onOpen={noop} />);
    expect(html).not.toContain("Ideas (");
    expect(renderToStaticMarkup(<HomeOverflow onOpen={noop} />)).not.toContain("Ideas (");
  });
  it("the tab row has no Classic tabs switch and no separate Asks, Blocking or Catch-up tab: the Queue holds them", () => {
    const html = renderToStaticMarkup(<HomeOverflow learn onOpen={noop} />);
    expect(html).not.toContain("Classic tabs");
    expect(html).not.toContain("aria-pressed");
    for (const id of ["asks", "blocks", "catchup"]) expect(homeTabs(true).some((t) => t[0] === id)).toBe(false);
  });
});
