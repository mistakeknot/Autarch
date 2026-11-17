// @vitest-environment jsdom
// The one count on screen (bead mk-yjp7): the strip, the tab and the sidebar badge all read the same
// `waiting` figure, and the strip says what it counts so a mismatch with another list can be explained.
import { renderToStaticMarkup } from "react-dom/server";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import type { Waiting } from "../waiting.js";
import { badgeCount, railBadge } from "../ui/waiting.js";
import { QueueTabs } from "../ui/tabs.js";
import { homeTabs, HomeTabs } from "../ui/tabs.js";

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

describe("waiting strip", () => {
  it("there is no ribbon repeating the tab row", () => {
    const html = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={{ decide: 5, later: 0, blocks: 2, catchup: 3 }} definition={W.definition} />);
    expect(html).not.toContain("data-waiting-strip");
    expect(html).not.toContain("Not in that number");
  });

  it("offers an info button instead of a text disclosure", () => {
    const html = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={null} definition={W.definition} />);
    expect(html).toContain('aria-label="What this counts"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).not.toContain("<details");
    expect(html).not.toContain("<summary");
  });
});

describe("waiting row layout", () => {
  it("lets tabs share a row with nonshrinking waiting text", () => {
    const host = document.createElement("div");
    host.innerHTML = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={null} line={W.line} definition={W.definition} />);
    const tabs = host.querySelector<HTMLElement>('[role="tablist"]')!;
    expect(tabs.style.flexBasis).toBe("");
    expect(tabs.classList.contains("flex-1")).toBe(true);
    expect(host.querySelector("[data-waiting-line]")!.classList.contains("shrink-0")).toBe(true);
    expect(tabs.nextElementSibling!.classList.contains("shrink-0")).toBe(true);
  });
  it.each([null, "5", ""])("omits an empty wrapper for line %s", (line) => {
    const host = document.createElement("div");
    host.innerHTML = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={null} line={line} />);
    expect(host.querySelector('[role="tablist"]')!.parentElement!.children).toHaveLength(1);
  });
  it.each([[W.line, null], [null, W.definition]])("keeps the wrapper when either element exists", (line, definition) => {
    const host = document.createElement("div");
    host.innerHTML = renderToStaticMarkup(<QueueTabs tab="decide" onTab={noop} counts={null} line={line} definition={definition} />);
    const row = host.querySelector('[role="tablist"]')!.parentElement!;
    expect(row.children).toHaveLength(2);
    expect(row.lastElementChild!.children).toHaveLength(1);
  });
});

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("waiting definition popover (mk-thfym)", () => {
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} });
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
    act(() => root.render(<QueueTabs tab="decide" onTab={noop} counts={null} line={W.line} definition={W.definition} />));
  });
  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  });
  const button = () => {
    const found = host.querySelector<HTMLButtonElement>('button[aria-label="What this counts"]');
    expect(found).not.toBeNull();
    return found!;
  };
  const toggle = async () => {
    await act(async () => { button().click(); await new Promise(resolve => setTimeout(resolve, 0)); });
  };

  it("toggles by click/tap, keeps the original definition and wires its accessible description", async () => {
    const control = button();
    expect(control.textContent?.trim()).toBe("");
    expect(control.tabIndex).toBe(0);
    expect(control.getAttribute("aria-expanded")).toBe("false");
    expect(control.hasAttribute("aria-controls")).toBe(false);
    await toggle();
    expect(control.getAttribute("aria-expanded")).toBe("true");
    const id = control.getAttribute("aria-controls");
    expect(id).toBeTruthy();
    expect(control.getAttribute("aria-describedby")).toBe(id);
    expect(document.getElementById(id!)?.textContent).toBe(W.definition);
    await toggle();
    expect(control.getAttribute("aria-expanded")).toBe("false");
    expect(control.hasAttribute("aria-controls")).toBe(false);
    expect(document.getElementById(id!)).toBeNull();
  });

  it("closes on Escape and returns keyboard focus to the info button", async () => {
    const control = button();
    act(() => control.focus());
    expect(document.activeElement).toBe(control);
    await toggle();
    const popup = document.getElementById(control.getAttribute("aria-controls")!);
    expect(popup).not.toBeNull();
    act(() => popup!.focus());
    await act(async () => {
      document.activeElement!.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      await new Promise(resolve => setTimeout(resolve, 0));
    });
    expect(control.getAttribute("aria-expanded")).toBe("false");
    expect(control.hasAttribute("aria-controls")).toBe(false);
    await vi.waitFor(() => expect(document.activeElement).toBe(control));
  });

  it("closes when the user clicks outside", async () => {
    await toggle();
    await act(async () => {
      document.body.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }));
      document.body.dispatchEvent(new MouseEvent("pointerup", { bubbles: true }));
      document.body.click();
      await new Promise(resolve => setTimeout(resolve, 0));
    });
    expect(button().getAttribute("aria-expanded")).toBe("false");
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
  it("the Queue tab carries the total", () => {
    const html = renderToStaticMarkup(<HomeTabs waiting={W.total} onOpen={noop} />);
    expect(html).toContain("Queue (5)"); // the decisions that need a pick; the notice is not added
    expect(homeTabs()[0]![2]).toBe("Queue");
  });
  it("the Ideas tab sits before Archive and Vizier and carries the open-idea count", () => {
    expect(homeTabs().map((t) => t[2])).toEqual(["Queue", "Ideas", "Archive", "Vizier", "Map", "Settings"]);
    const html = renderToStaticMarkup(<HomeTabs waiting={W.total} ideas={3} onOpen={noop} />);
    expect(html).toContain("Ideas (3)");
    expect(renderToStaticMarkup(<HomeTabs onOpen={noop} />)).not.toContain("Ideas (");
  });
  it("the tab row has no Classic tabs switch and no separate Asks, Blocking or Catch-up tab: the Queue holds them", () => {
    const html = renderToStaticMarkup(<HomeTabs learn onOpen={noop} />);
    expect(html).not.toContain("Classic tabs");
    expect(html).not.toContain("aria-pressed");
    for (const id of ["asks", "blocks", "catchup"]) expect(homeTabs(true).some((t) => t[0] === id)).toBe(false);
  });
});
