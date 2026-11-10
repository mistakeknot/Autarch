// @vitest-environment jsdom
// After a button press that resolves a card or moves it out of the list (an answer, Later, Move back), the next card
// on screen is the one that was directly below it, or the one above if it was last (bead mk-c0mka). Before this, the
// desktop Decide list fell back to the first card and the phone inbox fell back to the list.
// Driven the way mk drives it: press the real buttons, then deliver the refreshed data the page would deliver.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@get-bb/plugin-sdk/app", () => ({ ThreadChat: () => null }));

import { AsksPanel, DecideQueue, type AsksData, type OwedAsk } from "../ui/asks.js";
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
const LATER = { at: "2026-11-09T09:00:00Z", by: "mk" };

let host: HTMLDivElement;
let root: Root;
let scrolled: Element[];
beforeEach(() => {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  scrolled = [];
  // jsdom has no scrollIntoView; record who was brought into view.
  (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView = function (this: Element) { scrolled.push(this); };
});
afterEach(() => {
  act(() => root.unmount());
  host.remove();
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
});

const ok = () => ({ ok: true });
const settle = () => act(async () => { await Promise.resolve(); });
const q = (sel: string) => host.querySelector(sel) as HTMLElement | null;
const click = (el: Element | null) => { expect(el).not.toBeNull(); act(() => { (el as HTMLElement).click(); }); };

describe("desktop Decide list", () => {
  const props = { onPick: vi.fn(ok) as never, onOpen: () => {}, nowMs: NOW, onLater: vi.fn(ok) as never };
  const showing = () => q("[data-decide-list] [aria-current=true]")?.textContent ?? "";
  const render = (owed: OwedAsk[]) => act(() => root.render(<DecideQueue owed={owed} {...props} />));
  const A = ask("a"), B = ask("b"), C = ask("c"), D = ask("d");

  it("answering the middle card shows the card that was below it, not the first", async () => {
    render([A, B, C, D]);
    click(q("[data-decide-list] li:nth-child(2) button")); // select B
    expect(showing()).toContain("Subject b");
    click(q('[data-decision="b"] [data-option="a"]'));
    await settle();
    render([A, C, D]); // the refreshed list: B is resolved
    expect(showing()).toContain("Subject c");
    expect(q("[data-decision]")?.getAttribute("data-decision")).toBe("c");
  });

  it("answering the last card shows the card above it", () => {
    render([A, B, C]);
    click(q("[data-decide-list] li:nth-child(3) button"));
    render([A, B]);
    expect(showing()).toContain("Subject b");
  });

  it("answering the first card shows the card that was second", () => {
    render([A, B, C]);
    render([B, C]);
    expect(showing()).toContain("Subject b");
  });

  it("answering the only card leaves nothing on screen", () => {
    render([A]);
    render([]);
    expect(host.querySelector("[data-decision]")).toBeNull();
  });

  it("a card leaving the list that mk is not looking at changes nothing", () => {
    render([A, B, C, D]);
    click(q("[data-decide-list] li:nth-child(3) button")); // select C
    render([A, C, D]); // B went away
    expect(showing()).toContain("Subject c");
    render([A, C]); // D went away
    expect(showing()).toContain("Subject c");
  });

  it("the card is picked by position in the list as it was when the card left, even if another card arrived meanwhile", () => {
    render([A, B, C]);
    click(q("[data-decide-list] li:nth-child(2) button")); // select B
    render([A, B, D, C]); // a new card arrives under B while mk is reading it
    expect(showing()).toContain("Subject b");
    render([A, D, C]); // B resolved: the card below it is D
    expect(showing()).toContain("Subject d");
  });

  it("when the card below left too, the next one below it is used, then the one above", () => {
    render([A, B, C, D]);
    click(q("[data-decide-list] li:nth-child(2) button"));
    render([A, D]); // B and C both gone
    expect(showing()).toContain("Subject d");
    render([A]); // D, the last, gone
    expect(showing()).toContain("Subject a");
  });

  it("brings the new card's row into view without animation", () => {
    render([A, B, C]);
    click(q("[data-decide-list] li:nth-child(2) button"));
    scrolled = [];
    render([A, C]);
    const row = q("[data-decide-list] [aria-current=true]");
    expect(scrolled).toContain(row);
  });

  it("Later through the panel moves on to the card below, and the Later group keeps the card", async () => {
    const panel = (owed: OwedAsk[]) => act(() => root.render(<AsksPanel part="decide" data={asks(owed)} onPick={props.onPick} onOpen={() => {}} onLater={props.onLater} nowMs={NOW} />));
    panel([A, B, C]);
    click(q("[data-decide-list] li:nth-child(2) button"));
    click(q("[data-later-set=b]"));
    await settle();
    panel([A, ask("b", { later: LATER }), C]);
    expect(showing()).toContain("Subject c");
  });
});

describe("phone inbox", () => {
  const A = ask("a"), B = ask("b"), C = ask("c");
  const onPick = vi.fn(ok) as never;
  const onLater = vi.fn(ok) as never;
  const render = (owed: OwedAsk[], initial?: { tab?: "waiting" | "later"; open?: string }) =>
    act(() => root.render(<MobileInbox asks={asks(owed)} waiting={owed.filter((o) => !o.later).length} onPick={onPick} onOpen={() => {}} onNote={() => {}} onLater={onLater} sinceNode={null} sinceCount={0} onDesktop={() => {}} now={NOW} {...(initial ? { initial } : {})} />));
  const detail = () => q("[data-inbox-detail]")?.getAttribute("data-inbox-detail") ?? null;

  it("the sticky bar's main action opens the card below the answered one", async () => {
    render([A, B, C], { open: "ask:b" });
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:c");
  });

  it("the sticky bar's Later opens the card below", async () => {
    render([A, B, C], { open: "ask:a" });
    click(q("[data-bar-later]"));
    await settle();
    expect(detail()).toBe("ask:b");
  });

  it("answering the last card opens the one above it", async () => {
    render([A, B, C], { open: "ask:c" });
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:b");
  });

  it("answering the only card goes to the empty list", async () => {
    render([A], { open: "ask:a" });
    click(q("[data-bar-main]"));
    await settle();
    render([]);
    expect(detail()).toBeNull();
    expect(q("[data-inbox-empty]")?.textContent).toContain("Nothing is waiting on you.");
  });

  it("an answer pressed on the card itself, once the card leaves the list, opens the card below", async () => {
    render([A, B, C], { open: "ask:b" });
    click(q('[data-inbox-detail] [data-option="b"]'));
    await settle();
    render([A, C]);
    expect(detail()).toBe("ask:c");
  });

  it("Move back from the Later tab opens the next card in Later", async () => {
    const L1 = ask("l1", { later: LATER }), L2 = ask("l2", { later: LATER }), L3 = ask("l3", { later: LATER });
    render([L1, L2, L3], { tab: "later", open: "ask:l2" });
    click(q("[data-bar-later]"));
    await settle();
    expect(detail()).toBe("ask:l3");
  });

  it("the new card starts at its top, not where the last one was scrolled to", async () => {
    render([A, B, C], { open: "ask:a" });
    const scroller = q("[data-inbox-detail]")!.parentElement as HTMLElement;
    scroller.scrollTop = 400;
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:b");
    expect(scroller.scrollTop).toBe(0);
  });

  it("Later pressed on the card itself moves it to Later and opens the card below", async () => {
    render([A, B, C], { open: "ask:b" });
    click(q("[data-inbox-detail] [data-later-set=b]"));
    await settle();
    render([A, ask("b", { later: LATER }), C]); // B is now in Later, the Waiting list is A, C
    expect(detail()).toBe("ask:c");
  });

  it("the card below is chosen from the list as it is when the answer lands, not as it was at the press", async () => {
    let finish: (v: { ok: boolean }) => void = () => {};
    const slow = vi.fn(() => new Promise<{ ok: boolean }>((r) => { finish = r; })) as never;
    const D = ask("d");
    const renderSlow = (owed: OwedAsk[]) =>
      act(() => root.render(<MobileInbox asks={asks(owed)} waiting={owed.length} onPick={slow} onOpen={() => {}} onNote={() => {}} onLater={onLater} sinceNode={null} sinceCount={0} onDesktop={() => {}} now={NOW} initial={{ open: "ask:b" }} />));
    renderSlow([A, B, C]);
    click(q("[data-bar-main]")); // the answer is in flight
    renderSlow([A, B, D, C]); // a card arrives under B meanwhile
    await act(async () => { finish({ ok: true }); await Promise.resolve(); });
    expect(detail()).toBe("ask:d");
  });

  it("the card below opens with its own form: words typed on the card before it do not carry over", async () => {
    render([A, B, C], { open: "ask:a" });
    const box = q("[data-inbox-detail] [data-other] textarea") as HTMLTextAreaElement;
    act(() => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!.call(box, "words meant for card a");
      box.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect((q("[data-inbox-detail] [data-other] textarea") as HTMLTextAreaElement).value).toBe("words meant for card a");
    click(q("[data-bar-main]"));
    await settle();
    expect(detail()).toBe("ask:b");
    expect((q("[data-inbox-detail] [data-other] textarea") as HTMLTextAreaElement).value).toBe("");
  });

  it("Back still returns to the list", () => {
    render([A, B, C], { open: "ask:b" });
    click(q("[data-inbox-back]"));
    expect(detail()).toBeNull();
    expect(q('[data-inbox-row="ask:a"]')).not.toBeNull();
  });
});
